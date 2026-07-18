package game

import (
	"testing"

	"ugataima/internal/collision"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

// Gorilla Titan rallies a pair of Masked Huntress escorts via the shared boss
// summon kit (data-driven summon_* fields in monsters.yaml). Verifies the wiring
// end-to-end: config -> exactly 2 masked_huntress spawn, tagged SummonedBy, capped.
func TestGorillaTitan_SummonsTwoHuntresses(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	oldTM, oldWM := world.GlobalTileManager, world.GlobalWorldManager
	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	world.GlobalWorldManager = nil
	defer func() {
		world.GlobalTileManager = oldTM
		world.GlobalWorldManager = oldWM
	}()

	w := world.NewWorld3D(cs.game.config)
	w.Width, w.Height = 12, 12
	w.Tiles = make([][]world.TileType3D, w.Height)
	for y := range w.Tiles {
		w.Tiles[y] = make([]world.TileType3D, w.Width)
		for x := range w.Tiles[y] {
			w.Tiles[y][x] = world.TileEmpty
		}
	}
	cs.game.world = w
	tile := float64(cs.game.config.GetTileSize())
	cs.game.camera.X, cs.game.camera.Y = TileCenterFromTile(6, 6, tile)
	cs.game.collisionSystem = collision.NewCollisionSystem(w, tile)
	cs.game.collisionSystem.RegisterEntity(collision.NewEntity("player", cs.game.camera.X, cs.game.camera.Y, 16, 16, collision.CollisionTypePlayer, false))

	gx, gy := TileCenterFromTile(4, 6, tile)
	gorilla := monsterPkg.NewMonster3DFromConfig(gx, gy, "gorilla_titan", cs.game.config)
	cs.game.registerSpawnedMonster(gorilla)

	// The data wiring landed, and summon_chance>0 routes it through the boss kit.
	if len(gorilla.SummonMonsters) != 1 || gorilla.SummonMonsters[0] != "masked_huntress" {
		t.Fatalf("gorilla_titan summon_monsters = %v, want [masked_huntress]", gorilla.SummonMonsters)
	}
	if gorilla.SummonCount != 2 || gorilla.SummonMax != 2 {
		t.Fatalf("summon count/max = %d/%d, want 2/2", gorilla.SummonCount, gorilla.SummonMax)
	}
	if !gorilla.IsBoss() {
		t.Fatal("a monster with summon_chance>0 should use the boss kit")
	}

	// Rally the escort: exactly two Masked Huntresses, tagged to this gorilla.
	if !cs.summonBossAdds(gorilla) {
		t.Fatal("gorilla should summon its escort")
	}
	if got := cs.countLiveSummons(gorilla); got != 2 {
		t.Fatalf("live summons = %d, want 2 (a pair of huntresses)", got)
	}
	for _, m := range cs.game.world.Monsters {
		if m.SummonedBy == gorilla.ID && m.Key != "masked_huntress" {
			t.Errorf("summon should be masked_huntress, got %q", m.Key)
		}
	}

	// Cap holds: with the pair already live, a second rally adds nothing.
	cs.summonBossAdds(gorilla)
	if got := cs.countLiveSummons(gorilla); got != 2 {
		t.Fatalf("summon cap breached: %d live, want 2", got)
	}
}

// The Gorilla Titan (a summoning boss with NO map-wide trait) must NOT beeline
// across the map from spawn - it goes relentless only after normal aggro (in its
// alert radius / once hit). Contrast: a boss with AggroWholeMap (Golden Thief Bug)
// chases from anywhere on activation.
func TestGorillaTitan_NoMapWideAggroUntilEngaged(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 5, 5, ts)

	gorilla := monsterPkg.NewMonster3DFromConfig(30*ts, 30*ts, "gorilla_titan", game.config) // far away
	game.world.Monsters = []*monsterPkg.Monster3D{gorilla}

	game.refreshBoundAllyCache()
	if gorilla.BossAggro {
		t.Fatal("gorilla must NOT relentlessly chase from across the map before aggro")
	}
	gorilla.IsEngagingPlayer = true
	game.refreshBoundAllyCache()
	if !gorilla.BossAggro {
		t.Fatal("an engaged gorilla should relentlessly pursue")
	}
	gorilla.IsEngagingPlayer = false
	gorilla.WasAttacked = true // sticky once hit
	game.refreshBoundAllyCache()
	if !gorilla.BossAggro {
		t.Fatal("a struck gorilla should keep relentlessly pursuing")
	}
}

// AggroWholeMap is the unique opt-in (Golden Thief Bug): an active such boss
// chases from anywhere with no prior engagement. Verified on a synthetic boss to
// avoid the bug's quest-gate setup, plus the data wiring on the real monster.
func TestAggroWholeMap_RelentlessFromSpawn(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 5, 5, ts)

	m := monsterPkg.NewMonster3DFromConfig(30*ts, 30*ts, "goblin", game.config)
	m.Boss = true // synthetic boss: the static YAML classification in production
	m.AggroWholeMap = true
	game.world.Monsters = []*monsterPkg.Monster3D{m}

	game.refreshBoundAllyCache()
	if !m.BossAggro {
		t.Fatal("an AggroWholeMap boss should relentlessly pursue from spawn")
	}

	bug := monsterPkg.NewMonster3DFromConfig(0, 0, "golden_thief_bug", game.config)
	if !bug.AggroWholeMap {
		t.Fatal("golden_thief_bug should carry aggro_whole_map (data wiring)")
	}
}

// Killing the Orc Warlord sends every HUMAN on the map (the masked Amazons) into
// a relentless hunt - goblins and beasts are untouched. Also checks the Warlord's
// summon roster (Huntress + Hexer) is wired.
func TestOrcWarlord_DeathRalliesHumansOnly(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	cs := game.combat

	orc := monsterPkg.NewMonster3DFromConfig(10*ts, 10*ts, "orc_hero_boss", game.config)
	if orc.DeathRalliesType != "human" {
		t.Fatalf("orc death_rallies_type = %q, want \"human\"", orc.DeathRalliesType)
	}
	if len(orc.SummonMonsters) != 2 || orc.SummonCount != 2 || orc.SummonMax != 4 {
		t.Fatalf("orc summon wiring off: monsters=%v count=%d max=%d", orc.SummonMonsters, orc.SummonCount, orc.SummonMax)
	}

	huntress := monsterPkg.NewMonster3DFromConfig(12*ts, 12*ts, "masked_huntress", game.config)
	hexer := monsterPkg.NewMonster3DFromConfig(13*ts, 13*ts, "masked_hexer_girl", game.config)
	goblin := monsterPkg.NewMonster3DFromConfig(14*ts, 14*ts, "jungle_goblin", game.config)
	cat := monsterPkg.NewMonster3DFromConfig(15*ts, 15*ts, "ocelot", game.config)
	game.world.Monsters = []*monsterPkg.Monster3D{orc, huntress, hexer, goblin, cat}

	if huntress.MonsterType != "human" || hexer.MonsterType != "human" {
		t.Fatalf("masked mobs should be type \"human\", got %q/%q", huntress.MonsterType, hexer.MonsterType)
	}

	orc.HitPoints = 0 // slain
	cs.rallyOnPatronDeath(orc)

	if !huntress.Relentless || !hexer.Relentless {
		t.Error("Amazons (type human) must go relentless when the Warlord dies")
	}
	if !huntress.IsEngagingPlayer || !huntress.WasAttacked {
		t.Error("a rallied human should be engaged + flagged hostile (sticky/persisted)")
	}
	if goblin.Relentless || cat.Relentless {
		t.Error("goblins and beasts must NOT be swept up by the human revenge")
	}
}
