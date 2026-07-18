package game

import (
	"testing"

	"ugataima/internal/bridge"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/threading"
	"ugataima/internal/world"
)

// TestRace_MonsterParallelUpdate exercises the EXACT production path
// (GameLoop.updateMonstersParallel: ConvertMonstersToWrappers +
// EntityUpdater.UpdateMonstersParallel) against a densely populated real map,
// for many ticks, under -race. See CollisionSystem's documented "disjoint
// entities" concurrency contract: canMoveToEntityPosition/shouldIgnoreEntityCollision
// iterate ALL entities (not just the caller's own), so if a monster's worker
// reads another monster's BoundingBox/CollisionType while that monster's own
// worker concurrently writes them (UpdateEntity / refreshMonsterCollisionSolidity),
// the race detector must catch it here.
//
// Run: go test ./internal/game/ -race -run TestRace_MonsterParallelUpdate -v
func TestRace_MonsterParallelUpdate(t *testing.T) {
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
	// Forest has the densest, most varied (ranged + melee + banding) monster
	// population of any map - the best odds of provoking cross-chunk contention.
	if err := wm.SwitchToMap("forest"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	world.GlobalWorldManager = wm
	w := wm.GetCurrentWorld()

	g := newTestGame(cfg, w)
	g.threading = threading.NewThreadingComponents(cfg)
	defer g.threading.Shutdown()
	w.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	t.Logf("forest: %d monsters", len(w.Monsters))

	// Party planted in the densest cluster so engaged/dormant collision-type
	// flips (the exact field the review flagged) happen every tick, not just
	// idle patrol drift.
	if len(w.Monsters) > 0 {
		m := w.Monsters[0]
		g.camera.X, g.camera.Y = m.X, m.Y
		g.collisionSystem.UpdateEntity("player", m.X, m.Y)
	}

	// Crossfire scenario: a bound ally in the fray hands every nearby mob an
	// AIFoe, and poison makes each foe's OWN worker write its HitPoints inside
	// the parallel phase - so any live foe.X/Y/HP read from another worker
	// (instead of the frame-start AIFoe/AITargetX/Y snapshot) races here.
	if len(w.Monsters) > 1 {
		w.Monsters[1].Bound = true
	}
	for i, m := range w.Monsters {
		if i%2 == 0 {
			m.ApplyPoison(6000) // ticks every parallel update for the whole run
		}
	}

	const ticks = 600 // 10s at 60 TPS - race detector needs sustained contention
	for tick := 0; tick < ticks; tick++ {
		g.frameCount++
		// Production per-tick order: serial foe/target snapshot, then parallel.
		g.refreshMonsterAIState()
		monsters := g.ConvertMonstersToWrappers()
		g.threading.EntityUpdater.UpdateMonstersParallel(monsters)
	}
}

// TestRace_ProjectileImpactScreenShake exercises the production path
// (GameLoop.updateProjectilesParallel: ConvertProjectilesToWrappers +
// EntityUpdater.UpdateProjectilesParallel) with several magic projectiles that
// all collide on the same tick, landing in different worker chunks.
// MagicProjectileWrapper.OnCollision -> CreateSpellHitEffectFromSpell ->
// addScreenShake reads-then-writes g.screenShake with no lock (unlike
// impactLights/spellHitEffects, which ARE guarded by hitEffectsMu) - two
// simultaneous impacts on different workers race on it.
//
// Run: go test ./internal/game/ -race -run TestRace_ProjectileImpactScreenShake -v
func TestRace_ProjectileImpactScreenShake(t *testing.T) {
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

	w := newTestWorld(cfg)
	g := newTestGame(cfg, w)
	g.threading = threading.NewThreadingComponents(cfg)
	defer g.threading.Shutdown()

	// addScreenShake's body is a couple of float comparisons - too small a
	// window for the race detector to catch at low contention. Use enough
	// projectiles that many workers land in CreateSpellHitEffectFromSpell's
	// unlocked addScreenShake call at the same instant.
	const n = 4000
	g.magicProjectiles = make([]MagicProjectile, n)
	for i := range g.magicProjectiles {
		g.magicProjectiles[i] = MagicProjectile{
			ID:        "race-bolt",
			X:         float64(i) * 64,
			Y:         64,
			VelX:      4,
			SpellType: "firebolt",
			Active:    true,
			LifeTime:  10,
		}
	}

	alwaysBlocked := func(x, y float64) bool { return false } // force OnCollision every tick

	for tick := 0; tick < 20; tick++ {
		for i := range g.magicProjectiles {
			g.magicProjectiles[i].Active = true
			g.magicProjectiles[i].LifeTime = 10
		}
		projectiles := g.ConvertProjectilesToWrappers()
		g.threading.EntityUpdater.UpdateProjectilesParallel(projectiles, alwaysBlocked)
	}
}
