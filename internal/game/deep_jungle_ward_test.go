package game

import (
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/threading/entities"
)

type wardMobPositions struct {
	bossX, bossY float64
	idolX, idolY float64
}

func spawnWardedWarlordEncounter(t *testing.T, game *MMGame, ts float64) (*monster.Monster3D, *monster.Monster3D, wardMobPositions) {
	t.Helper()
	boss := monster.NewMonster3DFromConfig(float64(8)*ts+ts/2, float64(6)*ts+ts/2, "orc_hero_boss", game.config)
	idol := monster.NewMonster3DFromConfig(float64(6)*ts+ts/2, float64(6)*ts+ts/2, "jungle_idol", game.config)
	if boss == nil || idol == nil {
		t.Fatal("deep jungle ward monsters must load from monsters.yaml")
	}
	game.world.Monsters = []*monster.Monster3D{boss, idol}
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	game.refreshBoundUndeadCache()
	if !boss.BossWarded {
		t.Fatal("warlord must start warded while an idol lives")
	}
	return boss, idol, wardMobPositions{
		bossX: boss.X, bossY: boss.Y,
		idolX: idol.X, idolY: idol.Y,
	}
}

func assertWardEncounterHeld(t *testing.T, boss, idol *monster.Monster3D, start wardMobPositions) {
	t.Helper()
	if boss.X != start.bossX || boss.Y != start.bossY {
		t.Fatalf("warded warlord moved from (%.0f,%.0f) to (%.0f,%.0f)",
			start.bossX, start.bossY, boss.X, boss.Y)
	}
	if idol.X != start.idolX || idol.Y != start.idolY {
		t.Fatalf("warlord idol moved from (%.0f,%.0f) to (%.0f,%.0f)",
			start.idolX, start.idolY, idol.X, idol.Y)
	}
}

func TestDeepJungleWard_RTApproachRetreatDoesNotMoveBossOrIdol(t *testing.T) {
	cfg := loadTestConfig(t)
	worldTest := newTestWorldSized(cfg, 12, 12)
	game := newTestGame(cfg, worldTest)
	game.combat = NewCombatSystem(game)
	ts := float64(cfg.GetTileSize())
	boss, idol, start := spawnWardedWarlordEncounter(t, game, ts)

	for _, pos := range [][2]int{
		{4, 6}, // approach: close enough to wake normal monsters
		{3, 6}, // retreat one step
		{2, 6}, // retreat again
	} {
		placePlayerAtTile(game, pos[0], pos[1], ts)
		game.refreshBoundUndeadCache()
		// Mirror the real two-phase RT tick: one snapshot for every monster's
		// Update() (Phase 1), then apply all the resulting writes (Phase 2).
		snapshot := game.collisionSystem.Snapshot()
		wrappers := make([]entities.MonsterUpdateInterface, 0, len(game.world.Monsters))
		for _, m := range game.world.Monsters {
			w := CreateMonsterWrapper(m, game.collisionSystem, snapshot, game)
			w.Update()
			wrappers = append(wrappers, w)
		}
		for _, w := range wrappers {
			w.ApplyCollisionUpdate()
		}
		game.combat.HandleMonsterInteractions()
		assertWardEncounterHeld(t, boss, idol, start)
	}
}

func TestDeepJungleWard_TBApproachRetreatDoesNotMoveBossOrIdol(t *testing.T) {
	game, gl, ts := tbBehaviorGame(t, 12, 12)
	boss, idol, start := spawnWardedWarlordEncounter(t, game, ts)

	for _, pos := range [][2]int{
		{4, 6}, // approach within TB vision, but not adjacent to the idol
		{3, 6}, // retreat one step
		{2, 6}, // retreat again
	} {
		placePlayerAtTile(game, pos[0], pos[1], ts)
		game.refreshBoundUndeadCache()
		runOneMonsterTurn(game, gl)
		assertWardEncounterHeld(t, boss, idol, start)
	}
}
