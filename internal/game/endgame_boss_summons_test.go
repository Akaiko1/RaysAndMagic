package game

import (
	"testing"

	monsterPkg "ugataima/internal/monster"
)

// The endgame bosses rally DEDICATED adds: each authored summon is its own
// monster key at the boss's level, not the ordinary map mob it is drawn as.
func TestEndgameBossesSummonDedicatedLevelMatchedAdds(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	// The monster catalog is a GLOBAL the helper does not load; without it the
	// test only passes when a shuffled-in neighbour loaded it first.
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	for _, bossKey := range []string{"ancient_god_of_death", "alien_enforcer"} {
		t.Run(bossKey, func(t *testing.T) {
			boss := monsterPkg.NewMonster3DFromConfig(0, 0, bossKey, cs.game.config)
			if boss == nil || len(boss.SummonMonsters) == 0 {
				t.Fatalf("%s authors no summons", bossKey)
			}
			for _, summonKey := range boss.SummonMonsters {
				def, err := monsterPkg.MonsterConfig.GetMonsterByKey(summonKey)
				if err != nil {
					t.Fatal(err)
				}
				// The add borrows the art of the ordinary mob it is a variant of.
				if _, err := monsterPkg.MonsterConfig.GetMonsterByKey(def.Sprite); err != nil {
					t.Fatalf("%s: drawn as %q, which is no map mob", summonKey, def.Sprite)
				}
				if summonKey == def.Sprite {
					t.Fatalf("boss still resolves the ordinary map mob %q", summonKey)
				}
				add := monsterPkg.NewMonster3DFromConfig(0, 0, summonKey, cs.game.config)
				if add.Level != boss.Level {
					t.Fatalf("summoned %s level = %d, want boss level %d", summonKey, add.Level, boss.Level)
				}
			}
		})
	}
}
