package game

// Headless backwards-walker detector - a DEBUG MODULE, not a regression test.
// Runs the real forest map through the RT frame pieces in game-loop order
// (Update -> separation -> bands -> frame-motion facing) and flags every tick
// where a monster's FACING opposes its own AI walk, attributing the cause:
// a separation shove / band snap overriding the walk in the frame-motion
// displacement, or a stale Direction while gliding under the facing threshold.
//
// Run with: RAM_DEBUG_SIM=1 go test ./internal/game/ -run TestDebugSim_BackwardsWalkers -v
import (
	"fmt"
	"math"
	"os"
	"sort"
	"testing"

	"ugataima/internal/bridge"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func TestDebugSim_BackwardsWalkers(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	t.Chdir("../..")

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
	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()
	monster.MustLoadMonsterConfig("assets/monsters.yaml")

	prevTM, prevWM := world.GlobalTileManager, world.GlobalWorldManager
	defer func() { world.GlobalTileManager, world.GlobalWorldManager = prevTM, prevWM }()
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		t.Fatalf("tiles: %v", err)
	}
	wm := world.NewWorldManager(cfg)
	if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
		t.Fatalf("map configs: %v", err)
	}
	if err := wm.LoadAllMaps(); err != nil {
		t.Fatalf("load maps: %v", err)
	}
	if err := wm.SwitchToMap("forest"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	world.GlobalWorldManager = wm
	w := wm.GetCurrentWorld()

	g := newTestGame(cfg, w)
	g.combat = NewCombatSystem(g)
	gl := &GameLoop{game: g}
	w.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	tps := cfg.GetTPS()

	// Park the party by the densest cluster so crowds engage and shove.
	bestX, bestY, bestCount := 64.0, 64.0, -1
	for _, m := range w.Monsters {
		count := 0
		for _, o := range w.Monsters {
			if math.Hypot(o.X-m.X, o.Y-m.Y) < 64*6 {
				count++
			}
		}
		if count > bestCount {
			bestCount = count
			bestX, bestY = m.X+64*2, m.Y
		}
	}
	g.camera.X, g.camera.Y = bestX, bestY
	g.collisionSystem.UpdateEntity("player", bestX, bestY)
	t.Logf("forest: %d monsters, %d TPS, party at (%.0f,%.0f) near %d mobs", len(w.Monsters), tps, bestX, bestY, bestCount)

	type stats struct {
		backTicks   int // facing opposes AI walk (>120deg apart)
		shoveTicks  int // ...and the non-AI displacement dominated the frame motion
		glideTicks  int // moving (AI) but under the facing threshold with opposing stale facing
		walkTicks   int // ticks with meaningful AI walk at all
		packTicks   int // ticks spent as a stacked band member
		desyncTicks int // ...facing >60deg away from the band leader's facing
	}
	perMonster := map[string]*stats{}
	stat := func(m *monster.Monster3D) *stats {
		s := perMonster[m.Name]
		if s == nil {
			s = &stats{}
			perMonster[m.Name] = s
		}
		return s
	}

	const seconds = 120
	for tick := 0; tick < seconds*tps; tick++ {
		g.frameCount++
		start := gl.captureMonsterFramePositions()

		for _, m := range w.Monsters {
			if !m.IsAlive() {
				continue
			}
			m.Update(g.collisionSystem, g.camera.X, g.camera.Y)
			g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
			g.refreshMonsterCollisionSolidity(m, g.camera.X, g.camera.Y)
		}
		// AI-only displacement snapshot, before shoves/snaps.
		aiX := map[*monster.Monster3D]float64{}
		aiY := map[*monster.Monster3D]float64{}
		for _, p := range start {
			aiX[p.monster], aiY[p.monster] = p.monster.X, p.monster.Y
		}

		gl.faceMonstersAlongFrameMotion(start)
		gl.separateOverlappingMonsters()
		gl.updateMonsterBands()

		// Pack coherence: a stacked band member must face like its leader.
		leaders := map[string]*monster.Monster3D{}
		for _, m := range w.Monsters {
			if m.IsAlive() && m.BandStackCount > 1 && m.BandStackIndex == 0 {
				leaders[m.ID] = m
			}
		}
		for _, m := range w.Monsters {
			if !m.IsAlive() || m.BandStackCount <= 1 || m.BandStackIndex == 0 {
				continue
			}
			leader := leaders[m.BandLeaderID]
			if leader == nil {
				continue
			}
			s := stat(m)
			s.packTicks++
			if math.Abs(angleDiffRad(m.Direction, leader.Direction)) > math.Pi/3 {
				s.desyncTicks++
			}
		}

		for _, p := range start {
			m := p.monster
			if !m.IsAlive() {
				continue
			}
			if m.BandStackCount > 1 && m.BandStackIndex > 0 {
				continue // stacked member: facing is band-owned (leader-synced), its private wander is overridden
			}
			adx, ady := aiX[m]-p.x, aiY[m]-p.y
			aiLen := math.Hypot(adx, ady)
			if aiLen < 0.2 {
				continue // no meaningful AI walk this tick
			}
			s := stat(m)
			s.walkTicks++
			walkAngle := math.Atan2(ady, adx)
			diff := math.Abs(angleDiffRad(m.Direction, walkAngle))
			if diff <= 2*math.Pi/3 {
				continue // facing within 120deg of the walk: fine
			}
			s.backTicks++
			// Attribute: did the post-AI passes move it more than the AI did?
			edx, edy := m.X-aiX[m], m.Y-aiY[m]
			if math.Hypot(edx, edy) > aiLen {
				s.shoveTicks++
			}
			totLen := math.Hypot(m.X-p.x, m.Y-p.y)
			if totLen*totLen < 0.25*0.25 {
				s.glideTicks++ // frame motion under threshold: stale facing kept
			}
		}
	}

	type row struct {
		name string
		s    *stats
	}
	var rows []row
	for name, s := range perMonster {
		rows = append(rows, row{name, s})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].s.backTicks > rows[j].s.backTicks })
	total, totalWalk := 0, 0
	for _, r := range rows {
		total += r.s.backTicks
		totalWalk += r.s.walkTicks
	}
	t.Logf("TOTAL backwards-facing ticks: %d of %d walk-ticks (%.2f%%) over %ds",
		total, totalWalk, 100*float64(total)/math.Max(1, float64(totalWalk)), seconds)
	for i, r := range rows {
		if i >= 8 || r.s.backTicks == 0 {
			break
		}
		fmt.Printf("%-14s backwards=%4d of %6d walk-ticks (shove-dominated=%d glide=%d)\n",
			r.name, r.s.backTicks, r.s.walkTicks, r.s.shoveTicks, r.s.glideTicks)
	}
	packTotal, desyncTotal := 0, 0
	for _, r := range rows {
		packTotal += r.s.packTicks
		desyncTotal += r.s.desyncTicks
	}
	t.Logf("PACK coherence: %d desynced of %d stacked-member ticks (%.2f%%)",
		desyncTotal, packTotal, 100*float64(desyncTotal)/math.Max(1, float64(packTotal)))
}

func angleDiffRad(a, b float64) float64 {
	d := math.Mod(a-b+3*math.Pi, 2*math.Pi) - math.Pi
	return d
}
