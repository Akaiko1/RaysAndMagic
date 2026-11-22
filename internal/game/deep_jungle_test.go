package game

import (
	"slices"
	"testing"

	"ugataima/internal/collision"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

// Gorilla Titan rallies its escort via the shared boss summon kit (data-driven
// summon_* fields in monsters.yaml). Verifies the wiring end-to-end: each rally
// adds abilities.summon.count of its own summon monsters, tagged SummonedBy, up
// to summon max and never past it.
func TestGorillaTitan_SummonsEscortUpToCap(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	oldTM, oldWM := world.GlobalTileManager, world.GlobalWorldManager
	world.GlobalTileManager = world.NewTileManager(testTileSizeClasses())
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

	// The data wiring landed, and a summon chance > 0 routes it through the boss kit.
	def := monsterPkg.MonsterConfig.Monsters["gorilla_titan"]
	if len(def.Abilities.Summon.Monsters) == 0 || def.Abilities.Summon.Count <= 0 || def.Abilities.Summon.Max <= 0 {
		t.Fatalf("gorilla_titan must author a capped escort: monsters=%v count=%d max=%d", def.Abilities.Summon.Monsters, def.Abilities.Summon.Count, def.Abilities.Summon.Max)
	}
	if !slices.Equal(gorilla.Abilities.Summon.Monsters, def.Abilities.Summon.Monsters) || gorilla.Abilities.Summon.Count != def.Abilities.Summon.Count || gorilla.Abilities.Summon.Max != def.Abilities.Summon.Max {
		t.Fatalf("summon wiring = %v %d/%d, want %v %d/%d", gorilla.Abilities.Summon.Monsters, gorilla.Abilities.Summon.Count, gorilla.Abilities.Summon.Max,
			def.Abilities.Summon.Monsters, def.Abilities.Summon.Count, def.Abilities.Summon.Max)
	}
	if !gorilla.IsBoss() {
		t.Fatal("a monster with a summon chance > 0 should use the boss kit")
	}

	// Rally until the cap: each rally adds summon count, tagged to this gorilla.
	for live := 0; live < def.Abilities.Summon.Max; {
		if !cs.summonBossAdds(gorilla) {
			t.Fatalf("gorilla should summon its escort at %d live", live)
		}
		live = min(def.Abilities.Summon.Max, live+def.Abilities.Summon.Count)
		if got := cs.countLiveSummons(gorilla); got != live {
			t.Fatalf("live summons = %d, want %d", got, live)
		}
	}
	for _, m := range cs.game.world.Monsters {
		if m.SummonedBy == gorilla.ID && !slices.Contains(def.Abilities.Summon.Monsters, m.Key) {
			t.Errorf("summon %q is not in abilities.summon.monsters %v", m.Key, def.Abilities.Summon.Monsters)
		}
	}

	// Cap holds: with the escort full, another rally adds nothing.
	cs.summonBossAdds(gorilla)
	if got := cs.countLiveSummons(gorilla); got != def.Abilities.Summon.Max {
		t.Fatalf("summon cap breached: %d live, want %d", got, def.Abilities.Summon.Max)
	}
}

// The Gorilla Titan (a summoning boss with NO map-wide trait) must NOT beeline
// across the map from spawn - it goes relentless only after normal aggro (in its
// alert radius / once hit). Contrast: a boss with AggroWholeMap (Golden Thief Bug)
// chases from anywhere on activation.
func TestGorillaTitan_AlwaysUsesFinitePursuit(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	placePlayerAtTile(game, 5, 5, ts)

	gorilla := monsterPkg.NewMonster3DFromConfig(30*ts, 30*ts, "gorilla_titan", game.config) // far away
	game.world.Monsters = []*monsterPkg.Monster3D{gorilla}

	game.refreshMonsterAIState()
	if gorilla.BossAggro {
		t.Fatal("gorilla must NOT relentlessly chase from across the map before aggro")
	}
	gorilla.IsEngagingPlayer = true
	game.refreshMonsterAIState()
	if gorilla.BossAggro || gorilla.IsEngagingPlayer {
		t.Fatal("an ordinary boss must disengage beyond its leash")
	}
	gorilla.IsEngagingPlayer = false
	gorilla.WasAttacked = true // Remember prior provocation.
	game.refreshMonsterAIState()
	if gorilla.BossAggro || gorilla.TargetsParty() {
		t.Fatal("remembering a hit must not restart distant pursuit")
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

	game.refreshMonsterAIState()
	if !m.BossAggro {
		t.Fatal("an AggroWholeMap boss should relentlessly pursue from spawn")
	}

	bug := monsterPkg.NewMonster3DFromConfig(0, 0, "golden_thief_bug", game.config)
	if !bug.AggroWholeMap {
		t.Fatal("golden_thief_bug should carry aggro_whole_map (data wiring)")
	}
}

// Killing the Orc Warlord sends every HUMAN on the map (the masked Amazons, his
// own summon roster) into a relentless hunt - goblins and beasts are untouched.
func TestOrcWarlord_DeathRalliesHumansOnly(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	cs := game.combat

	orc := monsterPkg.NewMonster3DFromConfig(10*ts, 10*ts, "orc_hero_boss", game.config)
	if orc.DeathRalliesType != "human" {
		t.Fatalf("orc death_rallies_type = %q, want \"human\"", orc.DeathRalliesType)
	}
	if len(orc.Abilities.Summon.Monsters) == 0 {
		t.Fatal("orc_hero_boss lost its summon roster")
	}

	// The Warlord's own retainers are the humans he rallies.
	game.world.Monsters = []*monsterPkg.Monster3D{orc}
	var humans []*monsterPkg.Monster3D
	for i, key := range orc.Abilities.Summon.Monsters {
		m := monsterPkg.NewMonster3DFromConfig(float64(12+i)*ts, 12*ts, key, game.config)
		if m.MonsterType != orc.DeathRalliesType {
			t.Fatalf("summon %s is type %q, want the rallied %q", key, m.MonsterType, orc.DeathRalliesType)
		}
		humans = append(humans, m)
		game.world.Monsters = append(game.world.Monsters, m)
	}
	goblin := monsterPkg.NewMonster3DFromConfig(14*ts, 14*ts, "jungle_goblin", game.config)
	cat := monsterPkg.NewMonster3DFromConfig(15*ts, 15*ts, "ocelot", game.config)
	if goblin.MonsterType == orc.DeathRalliesType || cat.MonsterType == orc.DeathRalliesType {
		t.Fatalf("negative controls must not be %q", orc.DeathRalliesType)
	}
	game.world.Monsters = append(game.world.Monsters, goblin, cat)

	orc.HitPoints = 0 // slain
	cs.rallyOnPatronDeath(orc)

	for _, m := range humans {
		if !m.Relentless {
			t.Errorf("%s (type human) must go relentless when the Warlord dies", m.Key)
		}
		if !m.IsEngagingPlayer || !m.WasAttacked {
			t.Errorf("rallied %s should be engaged + flagged hostile (sticky/persisted)", m.Key)
		}
	}
	if goblin.Relentless || cat.Relentless {
		t.Error("goblins and beasts must NOT be swept up by the human revenge")
	}
}
