package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
)

// All monster-owned routes share card thorns; environmental/friendly damage
// has no hostile owner. Zero damage cannot trigger a counterattack.
func TestCardThornsMonsterDamageRoutes(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, card := range []bool{false, true} {
			for _, immune := range []bool{false, true} {
				for _, route := range []string{"melee", "projectile", "fireburst", "inferno", "boss trap", "environment", "friendly splash"} {
					t.Run(fmt.Sprintf("%s/TB=%v/card=%v/immune=%v", route, tb, card, immune), func(t *testing.T) {
						cs := newTestCombatSystemWithConfig(t)
						g := cs.game
						g.turnBasedMode = tb
						g.party.Members = g.party.Members[:1]
						ch := g.party.Members[0]
						isolateTrueDamageMember(ch, 0)
						ch.MaxHitPoints, ch.HitPoints = 1000, 1000
						if immune {
							g.combatBuffs = []TimedCombatBuff{{ResistSchool: "fire", ResistSchoolPct: 100}}
						}
						if card {
							g.cardSlots[0].key = "vengeful_ningyo_card"
						}
						m := mkTestMonster("Caster", 1000)
						m.FireburstDamageMin, m.FireburstDamageMax, m.InfernoDamage, m.TrapVolleyDamage = 100, 100, 100, 100
						switch route {
						case "melee", "projectile":
							cs.monsterHitCharacter(m, ch, m.Name, hitFromMonster(m, 100, "fire", route == "melee", 0, false, false))
						case "fireburst":
							cs.applyMonsterFireburst(m)
						case "inferno":
							cs.applyMonsterInferno(m)
						case "boss trap":
							g.detonateBossFireTrap(m)
						case "environment", "friendly splash":
							cs.damagePartyMemberElement(0, ch, 100, "fire", false)
						}
						received := 1000 - ch.HitPoints
						want := 0
						if card && route != "environment" && route != "friendly splash" {
							want = received * g.cardThornsPct() / 100
						}
						if got := 1000 - m.HitPoints; got != want {
							t.Fatalf("received %d, reflected %d, want %d", received, got, want)
						}
						if immune && received != 0 {
							t.Fatal("immune target took damage")
						}
					})
				}
			}
		}
	}
}

func TestCardThornsSpecialKillAndMeleeBoundaries(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, route := range []string{"fireburst", "inferno"} {
			for _, lethal := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/TB=%v/lethal=%v", route, tb, lethal), func(t *testing.T) {
					cs := newTestCombatSystemWithConfig(t)
					g := cs.game
					g.turnBasedMode = tb
					for _, ch := range g.party.Members {
						isolateTrueDamageMember(ch, 0)
						ch.MaxHitPoints, ch.HitPoints = 1000, 1000
						// Neither the weapon's melee-only thorns nor the fiery set's melee
						// riposte may answer a hostile spell.
						ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("parry_dagger")
						for key, def := range config.GlobalItems.Items {
							if set := config.GetItemSet(def.Set); set != nil && set.FieryRipostePct > 0 {
								it := items.CreateItemFromYAML(key)
								slot, _ := ch.EquipDestination(it)
								ch.Equipment[slot] = it
							}
						}
					}
					hp := 10000
					if lethal {
						hp = 1
						g.cardSlots[0].key = "vengeful_ningyo_card"
					}
					m := mkTestMonster("Caster", hp)
					m.ID = "special-reflection"
					m.FireburstDamageMin, m.FireburstDamageMax, m.InfernoDamage = 100, 100, 100
					if route == "fireburst" {
						cs.applyMonsterFireburst(m)
					} else {
						cs.applyMonsterInferno(m)
					}
					if lethal {
						if m.IsAlive() || len(g.deadMonsterIDs) != 1 {
							t.Fatalf("reflected kill must be finalized once: alive=%v kills=%v", m.IsAlive(), g.deadMonsterIDs)
						}
					} else if m.HitPoints != hp {
						t.Fatal("melee-only riposte fired on a spell")
					}
					for _, ch := range g.party.Members {
						if ch.HitPoints >= 1000 {
							t.Fatal("already launched area attack failed to hit the whole party")
						}
					}
				})
			}
		}
	}
}

func TestAttackCardTextMatchesTrigger(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("Octopus/TB=%v", tb), func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			g.turnBasedMode = tb
			ch := g.party.Members[0]
			comparisonTestHero(ch)
			delete(ch.Skills, character.SkillSpiritualTraining)
			def, _ := config.GetItemDefinition("octopus_card")
			original := def.CardDoubleAttackPct
			def.CardDoubleAttackPct = 100
			defer func() { def.CardDoubleAttackPct = original }()
			g.cardSlots[0].key = "octopus_card"
			ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
			g.world.Monsters = nil
			if !cs.EquipmentMeleeAttack() || len(g.slashEffects) != 2 {
				t.Fatal("melee action, including a whiff, must allow the extra strike")
			}
			if text := strings.Join(def.CardEffectLines(), " "); !strings.Contains(text, "on melee attack") {
				t.Fatalf("wrong trigger description: %s", text)
			}
		})
		for _, weapon := range []string{"hunting_bow", "alien_blaster"} {
			for _, label := range []string{"", "bonus bolt"} {
				t.Run(fmt.Sprintf("Ashigaru/%s/%s/TB=%v", weapon, label, tb), func(t *testing.T) {
					cs := newTestCombatSystemWithConfig(t)
					g := cs.game
					g.turnBasedMode = tb
					ch := g.party.Members[0]
					comparisonTestHero(ch)
					def, _ := config.GetItemDefinition("ashigaru_firelock_card")
					original := def.CardVolleyBonusPct
					def.CardVolleyBonusPct = 100
					defer func() { def.CardVolleyBonusPct = original }()
					g.cardSlots[0].key = "ashigaru_firelock_card"
					ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(weapon)
					if !cs.createArrowAttack(10, items.SlotMainHand, label) {
						t.Fatal("launch failed")
					}
					want := 2
					if label != "" {
						want = 1
					}
					if len(g.arrows) != want {
						t.Fatalf("got %d projectiles, want %d", len(g.arrows), want)
					}
					if text := strings.Join(def.CardEffectLines(), " "); !strings.Contains(text, "on ranged weapon attack") {
						t.Fatalf("wrong trigger description: %s", text)
					}
				})
			}
		}
	}
}

func TestSpecialReflectionUsesMitigatedDamage(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.party.Members = g.party.Members[:1]
	ch := g.party.Members[0]
	isolateTrueDamageMember(ch, 0)
	ch.MaxHitPoints, ch.HitPoints = 1000, 1000
	g.cardSlots[0].key = "vengeful_ningyo_card"
	g.combatBuffs = []TimedCombatBuff{{ResistPct: 50, InReduce: 10}}
	m := mkTestMonster("Caster", 1000)
	dealt := cs.damagePartyMemberPartsFromSource(0, ch, damagecalc.Parts{Normal: 100, True: 20}, monsterPkg.DamageFire.String(), true, m)
	if dealt != 50 || 1000-m.HitPoints != dealt*g.cardThornsPct()/100 {
		t.Fatalf("wrong mitigation/reflection: received=%d reflected=%d", dealt, 1000-m.HitPoints)
	}
}

func TestCardReflectionRestoredCollection(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, tb := range []bool{false, true} {
			t.Run(fmt.Sprintf("legacy=%v/TB=%v", legacy, tb), func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				g := cs.game
				g.turnBasedMode = tb
				ch := g.party.Members[0]
				isolateTrueDamageMember(ch, 0)
				saved := &GameSave{Party: PartySave{Members: []CharacterSave{buildCharacterSave(ch)}}}
				if legacy {
					saved.Party.CardCollection = []string{"vengeful_ningyo_card"}
				} else {
					saved.Party.CardCollectionItems = []items.Item{items.CreateItemFromYAML("vengeful_ningyo_card")}
				}
				g.restoreSavedParty(saved)
				ch = g.party.Members[0]
				ch.Luck = 0
				ch.BuffBonuses = character.StatBonuses{}
				ch.HitPoints, ch.MaxHitPoints = 1000, 1000
				g.combatBuffs = restoreCombatBuffs(buildCombatBuffSaves([]TimedCombatBuff{{SpellID: "fire_shield", Frames: 600, ResistSchool: "fire", ResistSchoolPct: 50}}))
				m := mkTestMonster("Restored target", 1000)
				m.InfernoDamage = 100
				cs.applyMonsterInferno(m)
				received := 1000 - ch.HitPoints
				if received <= 0 || m.HitPoints == 1000 || 1000-m.HitPoints != received*g.cardThornsPct()/100 {
					t.Fatalf("restore lost thorns: received=%d reflected=%d", received, 1000-m.HitPoints)
				}
			})
		}
	}
}
