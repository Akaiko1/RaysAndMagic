package game

import (
	"fmt"
	"testing"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

func elementalTestBiome(t *testing.T, cs *CombatSystem, school string) {
	t.Helper()
	wm := world.NewWorldManager(cs.game.config)
	wm.CurrentMapKey = "test"
	wm.MapConfigs["test"] = &config.MapConfig{Biome: "test"}
	wm.Biomes["test"] = config.BiomeConfig{ElementalAttackSchool: school}
	setTestWorldManager(t, wm)
}

func TestElementalMeleeDeliveryCells(t *testing.T) {
	for _, school := range monsterPkg.DamageTypes() {
		if school == monsterPkg.DamagePhysical {
			continue
		}
		for _, target := range []string{"party", "enemy", "bound"} {
			for _, ranged := range []bool{false, true} {
				for _, proc := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/ranged=%t/proc=%t", school, target, ranged, proc), func(t *testing.T) {
						cs := newTestCombatSystemWithConfig(t)
						elementalTestBiome(t, cs, school.String())
						cs.game.config.MonsterCombat.ElementalAttack = config.ElementalAttackConfig{Chance: .2, DamageMultiplier: 2}
						rolls := 0
						cs.elementalAttackRoll = func() float64 {
							rolls++
							if proc {
								return .199999
							}
							return .2
						}
						m := mkTestMonster("Attacker", 1000)
						m.DamageMin, m.DamageMax, m.TrueDamage = 40, 40, 3
						m.X, m.Y = 0, 0
						if ranged {
							m.ProjectileWeapon = "throwing_knife"
						}
						cs.game.camera.X, cs.game.camera.Y = cs.game.config.GetTileSize(), 0
						want := 43
						if proc {
							want *= 2
						}
						if target == "party" {
							cs.game.party.Members = cs.game.party.Members[:1]
							member := cs.game.party.Members[0]
							isolateTrueDamageMember(member, 0)
							member.HitPoints, member.MaxHitPoints = 1000, 1000
							cs.performMonsterAttackAgainstParty(m)
							if got := 1000 - member.HitPoints; got != want {
								t.Fatalf("party damage=%d, want %d", got, want)
							}
						} else {
							foe := mkTestMonster("Victim", 1000)
							foe.ArmorClass = 0
							foe.X = cs.game.config.GetTileSize()
							foe.Y = 0
							m.Bound = target == "enemy"
							foe.Bound = target == "bound"
							cs.performMonsterAttackAgainstMonster(m, foe, ProjectileOwnerMonsterAtBound)
							if got := 1000 - foe.HitPoints; got != want {
								t.Fatalf("monster damage=%d, want %d", got, want)
							}
						}
						if rolls != 1 {
							t.Fatalf("rolls=%d, want one per swing", rolls)
						}
						if len(cs.game.arrows) != 0 {
							t.Fatal("adjacent ranged monster fired instead of melee")
						}
						wantFX := 0
						if proc {
							wantFX = 2
						}
						if len(cs.game.elementalAttackEffects) != wantFX {
							t.Fatalf("FX=%d, want %d", len(cs.game.elementalAttackEffects), wantFX)
						}
					})
				}
			}
		}
	}
}

func TestElementalMeleeResistanceAndRangedExclusion(t *testing.T) {
	for _, resistance := range []int{-50, 0, 50, 100} {
		t.Run(fmt.Sprint(resistance), func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			elementalTestBiome(t, cs, "earth")
			cs.elementalAttackRoll = func() float64 { return 0 }
			cs.game.party.Members = cs.game.party.Members[:1]
			member := cs.game.party.Members[0]
			isolateTrueDamageMember(member, 0)
			member.HitPoints, member.MaxHitPoints = 1000, 1000
			member.Equipment[items.SlotRing1] = items.Item{Attributes: map[string]int{"resist_earth": resistance}}
			m := mkTestMonster("Fist", 1000)
			m.DamageMin, m.DamageMax = 50, 50
			cs.applyMonsterMeleeDamage(m)
			if got, want := 1000-member.HitPoints, 100*(100-resistance)/100; got != want {
				t.Fatalf("elemental resistance damage=%d, want %d", got, want)
			}
			m.ProjectileSpell = "firebolt"
			m.X, m.Y = 0, 0
			cs.game.camera.X = 2 * cs.game.config.GetTileSize()
			cs.game.camera.Y = 0
			cs.elementalAttackRoll = func() float64 { t.Fatal("ranged attack rolled melee proc"); return 0 }
			before := member.HitPoints
			cs.performMonsterAttackAgainstParty(m)
			if member.HitPoints != before || len(cs.game.magicProjectiles) != 1 {
				t.Fatal("ranged delivery changed")
			}
			if p := cs.game.magicProjectiles[0]; p.SpellType != "firebolt" || p.Damage != 50 {
				t.Fatalf("ranged payload changed: %+v", p)
			}
		})
	}
}

func TestMonstersYAML_MeleeProfilesLoad(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	eligible, excluded := 0, 0
	for key, def := range monsterPkg.MonsterConfig.Monsters {
		profile := def.MeleeProfile(cs.game.config.MonsterCombat.ElementalAttack, "earth")
		if def.Champion != "" || def.WarlordIdol {
			excluded++
			if profile.School != "" || profile.ElementalAttack.Chance != 0 {
				t.Fatalf("%s must be excluded", key)
			}
		} else {
			eligible++
			if profile.School != "physical" || profile.ElementalAttack.Chance != .2 {
				t.Fatalf("%s profile=%+v", key, profile)
			}
		}
	}
	if eligible != 70 || excluded != 5 {
		t.Fatalf("roster: %d eligible, %d excluded", eligible, excluded)
	}
}
