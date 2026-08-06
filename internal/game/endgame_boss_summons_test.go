package game

import (
	"testing"

	monsterPkg "ugataima/internal/monster"
)

func TestEndgameBossesSummonDedicatedLevelMatchedAdds(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	// The monster catalog is a GLOBAL the helper does not load; without it the
	// test only passes when a shuffled-in neighbour loaded it first.
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	tests := []struct {
		bossKey   string
		summonKey string
		baseKey   string
	}{
		{bossKey: "ancient_god_of_death", summonKey: "deathbound_mummy", baseKey: "mummy"},
		{bossKey: "alien_enforcer", summonKey: "enforcer_alien", baseKey: "alien"},
	}

	for _, tc := range tests {
		t.Run(tc.bossKey, func(t *testing.T) {
			boss := monsterPkg.NewMonster3DFromConfig(0, 0, tc.bossKey, cs.game.config)
			add := monsterPkg.NewMonster3DFromConfig(0, 0, tc.summonKey, cs.game.config)
			base := monsterPkg.NewMonster3DFromConfig(0, 0, tc.baseKey, cs.game.config)
			if boss == nil || add == nil || base == nil {
				t.Fatalf("missing boss/add/base config: boss=%v add=%v base=%v", boss, add, base)
			}
			if len(boss.SummonMonsters) != 1 || boss.SummonMonsters[0] != tc.summonKey {
				t.Fatalf("summon pool = %v, want [%s]", boss.SummonMonsters, tc.summonKey)
			}
			if add.Level != boss.Level {
				t.Fatalf("summoned add level = %d, want boss level %d", add.Level, boss.Level)
			}
			if add.Key == base.Key {
				t.Fatalf("boss still resolves the ordinary map mob %q", base.Key)
			}
		})
	}
}
