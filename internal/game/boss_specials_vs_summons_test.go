package game

// Bosses lured onto a party SUMMON must still use their kit. The crossfire
// branch used to strike the summon and skip the boss block entirely, so every
// boss went toothless the moment a summon out-competed the party for aggro:
// no trap field, no adds, no enrage, no blink. Both loops, every shipped boss.

import (
	"testing"

	monsterPkg "ugataima/internal/monster"
)

// spawnSummonsBossScenario puts a card-summoned ally next to the boss and the
// party farther out, so foe selection picks the summon while the party stays
// inside nova reach. prep runs BEFORE the AI refresh: a quest-gated boss is
// evasive until it does, and an evasive boss acquires no foe at all.
func spawnSummonsBossScenario(t *testing.T, g *MMGame, key string,
	prep func(*monsterPkg.Monster3D)) (*monsterPkg.Monster3D, *monsterPkg.Monster3D) {
	t.Helper()
	boss := spawnSpecialsMonster(g, key, 7, 2)
	ally := monsterPkg.NewMonster3DFromConfig(6*64+32, 2*64+32, "revenant", g.config)
	if boss == nil || ally == nil {
		t.Fatalf("%s and revenant must load from monsters.yaml", key)
	}
	markCardAlly(ally)
	g.world.Monsters = append(g.world.Monsters, ally)
	g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
	if prep != nil {
		prep(boss)
	}
	g.refreshMonsterAIState()
	if boss.AIFoe != ally {
		t.Fatalf("setup: %s targets %v, want the summon", key, boss.AIFoe)
	}
	return boss, ally
}

func TestBossSpecialsFireWhileFightingSummons(t *testing.T) {
	cases := []struct {
		name string
		key  string
		prep func(m *monsterPkg.Monster3D)
		want string
	}{
		{
			name: "brood-mother-traps", key: "dragon_brood_mother",
			prep: func(m *monsterPkg.Monster3D) { noSummons(m); m.InfernoChance = 0 },
			want: "seeds the ground with smouldering eggs",
		},
		{
			name: "brood-mother-adds", key: "dragon_brood_mother",
			prep: func(m *monsterPkg.Monster3D) { m.SummonChance = 1.0; m.InfernoChance = 0 },
			want: "raises the war-banner",
		},
		{
			name: "brood-mother-enrage", key: "dragon_brood_mother",
			prep: func(m *monsterPkg.Monster3D) {
				noSummons(m)
				m.InfernoChance = 0
				m.HitPoints = m.EnrageAtHP
			},
			want: "flies into a furious rage",
		},
		{
			name: "brood-mother-inferno", key: "dragon_brood_mother",
			prep: func(m *monsterPkg.Monster3D) { noSummons(m); m.InfernoChance = 1.0 },
			want: "Inferno scorches",
		},
		{
			name: "samurai-adds", key: "old_samurai",
			prep: func(m *monsterPkg.Monster3D) { aggro(m); m.SummonChance = 1.0 },
			want: "raises the war-banner",
		},
		{
			name: "orc-warlord-adds", key: "orc_hero_boss",
			prep: func(m *monsterPkg.Monster3D) { m.BossAggro = true; m.SummonChance = 1.0 },
			want: "raises the war-banner",
		},
		{
			name: "gorilla-titan-adds", key: "gorilla_titan",
			prep: func(m *monsterPkg.Monster3D) { m.BossAggro = true; m.SummonChance = 1.0 },
			want: "raises the war-banner",
		},
		{
			name: "ancient-god-adds", key: "ancient_god_of_death",
			prep: func(m *monsterPkg.Monster3D) { m.BossAggro = true; m.SummonChance = 1.0 },
			want: "raises the war-banner",
		},
		{
			name: "alien-enforcer-adds", key: "alien_enforcer",
			prep: func(m *monsterPkg.Monster3D) { m.BossAggro = true; m.SummonChance = 1.0 },
			want: "raises the war-banner",
		},
		{
			name: "gtb-lowhp-blink", key: "golden_thief_bug",
			prep: func(m *monsterPkg.Monster3D) {
				aggro(m)
				noSummons(m)
				m.InfernoChance = 0
				m.TeleportChance = 1.0
				m.HitPoints = m.TeleportAtHP
			},
			want: "blinks away in a golden flash",
		},
	}

	for _, tc := range cases {
		for _, mode := range []string{"rt", "tb"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				g, gl := newSpecialsTestGame(t)
				spawnSummonsBossScenario(t, g, tc.key, tc.prep)

				if mode == "rt" {
					runRTCombatSeconds(g, 12) // trap interval is 10s
				} else {
					runTBMonsterTurns(g, gl, 4) // trap interval is 3 turns
				}

				if countCombatLog(g, tc.want) == 0 {
					t.Errorf("%s/%s: no %q in combat log while the boss fought a summon",
						tc.name, mode, tc.want)
				}
			})
		}
	}
}

// The summon fight must not become a free pass either: the boss keeps striking
// the ally it is locked onto, so specials ride the fight instead of replacing it.
// Melee boss on purpose - a projectile boss would need the projectile pass.
func TestBossStillStrikesSummonAlongsideSpecials(t *testing.T) {
	g, _ := newSpecialsTestGame(t)
	_, ally := spawnSummonsBossScenario(t, g, "old_samurai", func(m *monsterPkg.Monster3D) {
		aggro(m)
		noSummons(m)
		m.HitPoints = m.EnrageAtHP // enrage announces on the same action moment
	})
	hp := ally.HitPoints

	runRTCombatSeconds(g, 4)

	if ally.HitPoints >= hp {
		t.Fatalf("summon took no damage from the boss: hp %d -> %d", hp, ally.HitPoints)
	}
	if countCombatLog(g, "flies into a furious rage") == 0 {
		t.Fatal("boss struck the summon but never announced enrage")
	}
}

// A target can die after the shared AI snapshot but before a later actor's RT
// interaction. The boss must wait for the next retarget just like an ordinary
// monster; its rider must not use that stale frame to cast at the party.
func TestBossWithDeadCachedFoeWaitsForRetargetRT(t *testing.T) {
	g, _ := newSpecialsTestGame(t)
	boss, ally := spawnSummonsBossScenario(t, g, "golden_thief_bug", func(m *monsterPkg.Monster3D) {
		aggro(m)
		noSummons(m)
		m.InfernoChance = 1
		m.InfernoCDFrames = 0
	})
	ally.HitPoints = 0 // killed by an earlier actor after target selection
	if boss.AIFoe != ally {
		t.Fatalf("setup: boss target = %v, want dead cached ally", boss.AIFoe)
	}

	hpBefore := partyHPSum(g)
	g.combat.HandleMonsterInteractions()

	if got := partyHPSum(g); got != hpBefore {
		t.Fatalf("boss cast at the party through a dead cached foe: HP %d -> %d", hpBefore, got)
	}
	if countCombatLog(g, "Inferno scorches") != 0 {
		t.Fatal("boss used Inferno before its dead crossfire target was retargeted")
	}
}
