package game

import (
	"math"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

func newDoorTestGame(t *testing.T) *MMGame {
	t.Helper()
	cfg := loadTestConfig(t)
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	w := newTestWorld(cfg)
	w.Width, w.Height = 20, 20
	w.Tiles = make([][]world.TileType3D, w.Height)
	for y := range w.Tiles {
		w.Tiles[y] = make([]world.TileType3D, w.Width)
	}
	return newTestGame(cfg, w)
}

func doorNPC(x, y float64) *character.NPC {
	return &character.NPC{
		X: x, Y: y, Name: "Gate", Sprite: "arena_gate", RenderCategory: "door",
		Type: character.NPCTypeDoor, DoorBehavior: character.NPCDoorBehaviorChampionPortcullis,
	}
}

// TestDoorPoseOrientation: the slab must span the doorway - N+S flanking walls
// give a N-S slab (yaw pi/2, blocking an E-W passage), W+E give an E-W slab.
// A tile with no opposing wall pair is a mis-authored door: no pose.
func TestDoorPoseOrientation(t *testing.T) {
	g := newDoorTestGame(t)
	ts := float64(g.config.GetTileSize())
	cx, cy := 5.5*ts, 5.5*ts

	set := func(tx, ty int, tile world.TileType3D) { g.world.Tiles[ty][tx] = tile }

	set(5, 4, world.TileWall)
	set(5, 6, world.TileWall)
	_, _, yaw, ok := g.doorPose(cx, cy)
	if !ok || yaw != math.Pi/2 {
		t.Fatalf("N+S walls: yaw=%v ok=%v, want pi/2 true", yaw, ok)
	}

	set(5, 4, world.TileEmpty)
	set(5, 6, world.TileEmpty)
	set(4, 5, world.TileWall)
	set(6, 5, world.TileWall)
	_, _, yaw, ok = g.doorPose(cx, cy)
	if !ok || yaw != 0 {
		t.Fatalf("W+E walls: yaw=%v ok=%v, want 0 true", yaw, ok)
	}

	set(4, 5, world.TileEmpty)
	set(6, 5, world.TileEmpty)
	if _, _, _, ok = g.doorPose(cx, cy); ok {
		t.Fatal("no flanking walls: want ok=false")
	}
}

// TestDoorReconciler: doors close (solid entity registered) while a living
// champion is on the map, open on its death (entity gone + rise message), and
// the open door becomes invisible to focus/click.
func TestDoorReconciler(t *testing.T) {
	g := newDoorTestGame(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if _, err := config.LoadChampionConfig("../../assets/champions.yaml"); err != nil {
		t.Fatalf("load champions: %v", err)
	}
	ts := float64(g.config.GetTileSize())
	door := doorNPC(5.5*ts, 5.5*ts)
	g.world.NPCs = append(g.world.NPCs, door)

	champ := monsterPkg.NewMonster3DFromConfig(2.5*ts, 2.5*ts, "hobbit_archer", g.config)
	g.world.Monsters = append(g.world.Monsters, champ)

	g.refreshDoors()
	if !g.doorsClosed || len(g.doorEntityIDs) != 1 {
		t.Fatalf("living champion: doorsClosed=%v entities=%d, want true/1", g.doorsClosed, len(g.doorEntityIDs))
	}
	if g.npcDoorOpen(door) {
		t.Fatal("closed door reported open")
	}
	// The registered entity actually blocks movement onto the door tile.
	if g.collisionSystem.CanMoveTo("player", 5.5*ts, 5.5*ts) {
		t.Fatal("closed door does not block the player")
	}

	champ.HitPoints = 0
	g.refreshDoors()
	if g.doorsClosed || len(g.doorEntityIDs) != 0 {
		t.Fatalf("champion dead: doorsClosed=%v entities=%d, want false/0", g.doorsClosed, len(g.doorEntityIDs))
	}
	if !g.npcDoorOpen(door) {
		t.Fatal("open door not reported open")
	}
	found := false
	for _, m := range g.combatLogHistory {
		if strings.Contains(m.Text, "portcullises rise") {
			found = true
		}
	}
	if !found {
		t.Fatal("no rise message on closed->open transition")
	}
}

// TestStartArenaDuel: the dialogue action places the party on the duel ground,
// spawns the chosen champion engaged, and refuses a second duel while one lives.
func TestStartArenaDuel(t *testing.T) {
	g := newDoorTestGame(t)
	g.combat = NewCombatSystem(g) // the spawn-time mirror requires the combat system
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	primeTestChampions(t, g)

	wm := world.NewWorldManager(g.config)
	wm.LoadedMaps = map[string]*world.World3D{"arena": g.world}
	wm.CurrentMapKey = "arena"
	wm.MapConfigs = map[string]*config.MapConfig{
		"arena": {Duel: &config.MapDuelConfig{PartyTile: [2]int{11, 4}, ChampionTile: [2]int{16, 4}}},
	}
	prevWM := world.GlobalWorldManager
	world.GlobalWorldManager = wm
	defer func() { world.GlobalWorldManager = prevWM }()

	ih := &InputHandler{game: g}
	choice := &character.NPCDialogueChoice{Action: "start_arena_duel", Tier: "normal"}
	ih.startArenaDuel(choice)

	ts := float64(g.config.GetTileSize())
	if wantX := (11.0 + 0.5) * ts; g.camera.X != wantX {
		t.Fatalf("party X = %v, want %v", g.camera.X, wantX)
	}
	var champ *monsterPkg.Monster3D
	for _, m := range g.world.Monsters {
		if m != nil && m.IsChampion() {
			champ = m
		}
	}
	if champ == nil {
		t.Fatal("no champion spawned")
	}
	if !champ.IsEngagingPlayer || !champ.WasAttacked {
		t.Fatal("champion not engaged on spawn")
	}

	// The mirror must run AT SPAWN (player input precedes the monster update):
	// a first-frame hit on a placeholder-stat mob would be swallowed by the
	// later tier-HP clamp.
	tier := config.GetChampionTier("normal")
	if !champ.ChampionMirrored || champ.MaxHitPoints != tier.HP || champ.HitPoints != tier.HP {
		t.Fatalf("champion not mirrored at spawn: mirrored=%v HP=%d/%d want %d/%d",
			champ.ChampionMirrored, champ.HitPoints, champ.MaxHitPoints, tier.HP, tier.HP)
	}
	champ.HitPoints -= 500 // the first-frame hit
	g.mirrorChampionStats(champ)
	if champ.HitPoints != tier.HP-500 {
		t.Fatalf("first-frame damage healed by the mirror: HP=%d, want %d", champ.HitPoints, tier.HP-500)
	}

	before := len(g.world.Monsters)
	ih.startArenaDuel(choice)
	if len(g.world.Monsters) != before {
		t.Fatal("second duel started while a champion lives")
	}

	// Daily lockout: the tier stays spent for the rest of the day even after
	// the champion dies, and unlocks when the day advances (sunrise).
	champ.HitPoints = 0
	ih.startArenaDuel(choice)
	if len(g.world.Monsters) != before {
		t.Fatal("tier re-fought on the same day")
	}
	g.dayNightDay++
	ih.startArenaDuel(choice)
	if len(g.world.Monsters) != before+1 {
		t.Fatal("tier did not unlock on the next morning")
	}

	// A typo'd tier must refuse the duel, not silently fall back to a default.
	for _, m := range g.world.Monsters {
		m.HitPoints = 0
	}
	g.dayNightDay++
	bad := &character.NPCDialogueChoice{Action: "start_arena_duel", Tier: "normall"}
	count := len(g.world.Monsters)
	ih.startArenaDuel(bad)
	if len(g.world.Monsters) != count {
		t.Fatal("typo'd tier started a duel instead of refusing")
	}
}
