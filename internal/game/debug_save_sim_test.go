//go:build debug

// Debug diagnostics are opt-in: run `go test -tags debug ./internal/game`.
package game

// Headless save-file simulation - a DEBUG MODULE, not a regression test.
// Loads a real save (default bin/saves/save1.json), steps the real-time
// monster path (AI + engagement flip + separation + combat interactions)
// with the party parked at the saved position, and reports what every
// monster near the party is doing - for diagnosing "monster frozen in
// front of the party" reports on live saves.
//
// Run with:
//
//	RAM_DEBUG_SIM=1 [RAM_DEBUG_SAVE=path.json] go test ./internal/game/ -run TestDebugSim_SaveFile -v
import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"testing"

	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

func TestDebugSim_SaveFile(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	t.Chdir("../..")

	savePath := os.Getenv("RAM_DEBUG_SAVE")
	if savePath == "" {
		savePath = "bin/saves/save1.json"
	}

	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := config.LoadSpellConfig("assets/spells.yaml"); err != nil {
		t.Fatalf("spells: %v", err)
	}
	if _, err := config.LoadWeaponConfig("assets/weapons.yaml"); err != nil {
		t.Fatalf("weapons: %v", err)
	}
	if _, err := config.LoadItemConfig("assets/items.yaml"); err != nil {
		t.Fatalf("items: %v", err)
	}
	if _, err := config.LoadLootTables("assets/loots.yaml"); err != nil {
		t.Fatalf("loots: %v", err)
	}
	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()

	prevTM, prevWM, prevQM := world.GlobalTileManager, world.GlobalWorldManager, quests.GlobalQuestManager
	defer func() {
		world.GlobalTileManager, world.GlobalWorldManager, quests.GlobalQuestManager = prevTM, prevWM, prevQM
	}()
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatalf("tiles: %v", err)
	}
	if err := world.GlobalTileManager.LoadSpecialTileConfig("assets/special_tiles.yaml"); err != nil {
		t.Fatalf("special tiles: %v", err)
	}
	if _, err := config.LoadTrapConfig("assets/traps.yaml"); err != nil {
		t.Fatalf("traps: %v", err)
	}
	monster.SetSizeClassHeights(cfg.Graphics.SizeClasses)
	monster.MustLoadMonsterConfig("assets/monsters.yaml")
	if err := character.LoadNPCConfig("assets/npcs.yaml"); err != nil {
		t.Fatalf("npcs: %v", err)
	}
	if _, err := config.LoadChampionConfig("assets/champions.yaml"); err != nil {
		t.Fatalf("champions: %v", err)
	}
	if err := PrimeChampions(cfg); err != nil {
		t.Fatalf("prime champions: %v", err)
	}
	if _, err := config.LoadLevelUpConfig("assets/level_up.yaml"); err != nil {
		t.Fatalf("level-up: %v", err)
	}
	monster.MustLoadHatesConfig("assets/hates.yaml")
	questConfig, err := quests.LoadQuestConfig("assets/quests.yaml")
	if err != nil {
		t.Fatalf("quests: %v", err)
	}
	quests.GlobalQuestManager = quests.NewQuestManager(questConfig)
	quests.GlobalQuestManager.InitializeStartingQuests()

	raw, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("read save: %v", err)
	}
	var save GameSave
	if err := json.Unmarshal(raw, &save); err != nil {
		t.Fatalf("parse save: %v", err)
	}

	wm := world.NewWorldManager(cfg)
	if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		t.Fatalf("map configs: %v", err)
	}
	if err := wm.LoadAllMaps(); err != nil {
		t.Fatalf("load maps: %v", err)
	}
	if err := wm.SwitchToMap(save.MapKey); err != nil {
		t.Fatalf("switch: %v", err)
	}
	world.GlobalWorldManager = wm

	// NewMMGame builds the same collision system, static NPC blocks and threaded
	// monster updater as the executable. We never call Draw, so this remains a
	// headless simulation.
	g := NewMMGame(cfg)
	defer g.Shutdown()
	g.appScreen = AppScreenInGame
	if err := g.applySave(wm, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}
	gl := g.gameLoop
	w := g.world
	camX, camY := g.camera.X, g.camera.Y
	tile := float64(cfg.World.TileSize)
	tps := cfg.GetTPS()
	t.Logf("save %s: map=%s player=(%.0f,%.0f) tile(%d,%d) turnBased=%v, %d monsters",
		savePath, save.MapKey, camX, camY, int(camX/tile), int(camY/tile), save.TurnBased, len(w.Monsters))

	// Watch every living monster within 12 tiles of the party. Some reports are
	// about a visibly stuck mob just beyond normal aggro range, so the old
	// eight-tile window silently omitted the useful actor.
	type watched struct {
		m            *monster.Monster3D
		startX, winX float64
		startY, winY float64
	}
	var watch []*watched
	for _, m := range w.Monsters {
		if m.IsAlive() && math.Hypot(m.X-camX, m.Y-camY) < tile*12 {
			watch = append(watch, &watched{m: m, startX: m.X, startY: m.Y, winX: m.X, winY: m.Y})
		}
	}
	for _, wt := range watch {
		m := wt.m
		t.Logf("WATCH %s hp=%d at (%.0f,%.0f) tile(%d,%d) dist=%.1ft engaged=%v wasAttacked=%v state=%v",
			m.Name, m.HitPoints, m.X, m.Y, int(m.X/tile), int(m.Y/tile),
			math.Hypot(m.X-camX, m.Y-camY)/tile, m.IsEngagingPlayer, m.WasAttacked, m.State)
	}

	// One-shot collision probes for the watched monsters before any ticks run.
	for _, wt := range watch {
		m := wt.m
		ent := g.collisionSystem.GetEntityByID(m.ID)
		if ent == nil {
			t.Logf("PROBE %s: NO COLLISION ENTITY", m.Name)
			continue
		}
		ownOK := g.collisionSystem.CanMoveToWithHabitat(m.ID, m.X, m.Y, m.HabitatPrefs, m.Flying)
		eastOK := g.collisionSystem.CanMoveToWithHabitat(m.ID, m.X+2, m.Y, m.HabitatPrefs, m.Flying)
		toPlayerOK := g.collisionSystem.CanMoveToWithHabitat(m.ID, m.X+(camX-m.X)*0.01, m.Y+(camY-m.Y)*0.01, m.HabitatPrefs, m.Flying)
		t.Logf("PROBE %s box=%+v type=%v ownPos=%v east2px=%v towardPlayer1%%=%v",
			m.Name, ent.BoundingBox, ent.CollisionType, ownOK, eastOK, toPlayerOK)
	}

	seconds := 30
	window := 2 * tps
	for tick := 0; tick < seconds*tps; tick++ {
		g.frameCount++
		gl.updateExploration()

		if (tick+1)%window == 0 {
			for _, wt := range watch {
				m := wt.m
				net := math.Hypot(m.X-wt.winX, m.Y-wt.winY)
				dist := math.Hypot(m.X-camX, m.Y-camY)
				stuck := ""
				if m.IsEngagingPlayer && net < 4 && m.State != monster.StateAttacking && dist > m.GetAttackRangePixels() {
					stuck = "  << STUCK (engaged, out of reach, not moving)"
					// Probe the three stepToward candidates at 1px scale.
					ddx, ddy := camX-m.X, camY-m.Y
					dd := math.Hypot(ddx, ddy)
					nx, ny := m.X+ddx/dd, m.Y+ddy/dd
					_, rDiag := g.collisionSystem.DebugCanMoveTo(m.ID, nx, ny)
					_, rX := g.collisionSystem.DebugCanMoveTo(m.ID, nx, m.Y)
					_, rY := g.collisionSystem.DebugCanMoveTo(m.ID, m.X, ny)
					stuck += fmt.Sprintf(" [diag:%s x:%s y:%s] pathTiles=%v", rDiag, rX, rY, m.PathTiles)
				}
				t.Logf("[t=%2ds] %-10s (%.0f,%.0f) tile(%d,%d) dist=%.1ft moved=%4.0fpx state=%v engaged=%v cd=%d path=%d/%d->(%d,%d) reach=%.0f%s",
					(tick+1)/tps, m.Name, m.X, m.Y, int(m.X/tile), int(m.Y/tile),
					dist/tile, net, m.State, m.IsEngagingPlayer, m.StateTimer,
					m.PathIndex, len(m.PathTiles), m.PathTargetTileX, m.PathTargetTileY,
					m.GetAttackRangePixels(), stuck)
				wt.winX, wt.winY = m.X, m.Y
			}
		}
	}

	for _, wt := range watch {
		m := wt.m
		t.Logf("FINAL %s: (%.0f,%.0f) total moved %.0fpx from start, hp=%d engaged=%v",
			m.Name, m.X, m.Y, math.Hypot(m.X-wt.startX, m.Y-wt.startY), m.HitPoints, m.IsEngagingPlayer)
	}
}
