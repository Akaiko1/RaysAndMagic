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
	game.refreshBoundAllyCache()
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
		game.refreshBoundAllyCache()
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
		game.refreshBoundAllyCache()
		runOneMonsterTurn(game, gl)
		assertWardEncounterHeld(t, boss, idol, start)
	}
}

// A warded warlord and its idols are scripted set pieces, not merely rooted
// monsters. The shared RT crossfire loop runs separately from Monster.Update;
// without the action gate it gave both an AIFoe and let the warlord strike a
// nearby bound revenant despite the ward still being active.
func TestDeepJungleWard_InertSetPiecesIgnoreBoundAlliesRT(t *testing.T) {
	cfg := loadTestConfig(t)
	worldTest := newTestWorldSized(cfg, 12, 12)
	game := newTestGame(cfg, worldTest)
	game.combat = NewCombatSystem(game)
	ts := float64(cfg.GetTileSize())
	boss, idol, _ := spawnWardedWarlordEncounter(t, game, ts)

	// One tile between the boss and idol: both would previously select this card
	// summon as their nearest foe. The boss can deal real melee damage here.
	ally := monster.NewMonster3DFromConfig(7*ts+ts/2, 6*ts+ts/2, "revenant", game.config)
	if ally == nil {
		t.Fatal("revenant must load from monsters.yaml")
	}
	markCardAlly(ally)
	game.world.Monsters = append(game.world.Monsters, ally)
	game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
	placePlayerAtTile(game, 0, 0, ts)

	game.refreshBoundAllyCache()
	if boss.AIFoe != nil || idol.AIFoe != nil {
		t.Fatalf("inactive ward encounter must not acquire bound allies: boss=%v idol=%v", boss.AIFoe, idol.AIFoe)
	}
	if ally.AIFoe == boss {
		t.Fatal("bound ally must not select an invulnerable warded boss")
	}

	// Defend the action loop too: it must hold even if a target from a previous
	// frame somehow survives until the ward state is refreshed again.
	boss.AIFoe, idol.AIFoe = ally, ally
	hp := ally.HitPoints
	game.combat.HandleMonsterInteractions()
	if ally.HitPoints != hp {
		t.Fatalf("warded encounter damaged bound revenant: hp=%d, want %d", ally.HitPoints, hp)
	}
	if boss.AttackAnimFrames != 0 || idol.AttackAnimFrames != 0 {
		t.Fatalf("inactive ward encounter played attack animation: boss=%d idol=%d", boss.AttackAnimFrames, idol.AttackAnimFrames)
	}
}
