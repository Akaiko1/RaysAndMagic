package game

import (
	"encoding/json"
	"fmt"
	"testing"

	"ugataima/internal/character"
)

func TestCelestialArmorAbsorbsPeriodicDamageInBothModes(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, restored := range []bool{false, true} {
			for _, effect := range []struct {
				name         string
				poison, burn bool
				damage       int
			}{{"poison", true, false, 3}, {"burn", false, true, 9}, {"both", true, true, 12}} {
				for _, budget := range []int{0, 2, 20} {
					t.Run(fmt.Sprintf("TB=%v/restored=%v/%s/budget=%d", tb, restored, effect.name, budget), func(t *testing.T) {
						g := advancedSupportGame(t, tb)
						castAdvancedSupport(t, g, "celestial_armor", 0, nil)
						g.advancedBuff("celestial_armor").Absorb = budget
						c := g.party.Members[1]
						tps := g.config.GetTPS()
						if effect.poison {
							c.ApplyPoison(10 * tps)
						}
						if effect.burn {
							c.ApplyBurn(10 * tps)
						}
						// Save partway through a cadence, not just at its zero boundary.
						c.RestoreDoTTickTimers(tps/3, tps/3)
						if restored {
							snapshot := struct {
								Hero  CharacterSave
								Buffs []CombatBuffSave
							}{buildCharacterSave(c), buildCombatBuffSaves(g.combatBuffs)}
							raw, err := json.Marshal(snapshot)
							if err != nil {
								t.Fatal(err)
							}
							if err = json.Unmarshal(raw, &snapshot); err != nil {
								t.Fatal(err)
							}
							c = restoreCharacterSave(snapshot.Hero)
							g.party.Members[1] = c
							g.combatBuffs = restoreCombatBuffs(snapshot.Buffs)
						}
						if tb {
							// Waiting in TB must not spend barriers or advance DoTs.
							for range tps {
								g.updatePartyClocks()
							}
							if c.HitPoints != 1000 {
								t.Fatal("TB deliberation advanced periodic damage")
							}
							g.startPartyTurn()
						} else {
							for range 3 * tps {
								g.updatePartyClocks()
								g.sweepLethalDoTVictims()
							}
						}
						if want := 1000 - max(0, effect.damage-budget); c.HitPoints != want {
							t.Fatalf("%s bypassed absorption: HP=%d, want %d", effect.name, c.HitPoints, want)
						}
						remaining := 0
						if b := g.advancedBuff("celestial_armor"); b != nil {
							remaining = b.Absorb
						}
						if remaining != max(0, budget-effect.damage) {
							t.Fatalf("shared barrier=%d after %d periodic damage, initial=%d", remaining, effect.damage, budget)
						}
						poisonPhase, burnPhase := c.DoTTickTimers()
						if effect.poison && poisonPhase != tps/3 || effect.burn && burnPhase != tps/3 {
							t.Fatalf("cadence phase changed: poison=%d burn=%d", poisonPhase, burnPhase)
						}
					})
				}
			}
		}
	}
}

func TestCelestialArmorPeriodicPoolAndLethalOrdering(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, budget := range []int{0, 4, 6} {
			t.Run(fmt.Sprintf("TB=%v/budget=%d", tb, budget), func(t *testing.T) {
				g := advancedSupportGame(t, tb)
				castAdvancedSupport(t, g, "celestial_armor", 0, nil)
				g.advancedBuff("celestial_armor").Absorb = budget
				castAdvancedSupport(t, g, "preservation", 0, nil)
				tps := g.config.GetTPS()
				for _, c := range g.party.Members[1:3] {
					c.HitPoints = 1
					c.ApplyBurn(tps)
					c.RestoreDoTTickTimers(0, 0)
				}
				// A corpse must never consume the shared pool.
				g.party.Members[3].HitPoints = 0
				g.party.Members[3].AddCondition(character.ConditionUnconscious)
				g.party.Members[3].ApplyBurn(tps)
				if tb {
					g.startPartyTurn()
				} else {
					for range tps {
						g.updatePartyClocks()
						g.sweepLethalDoTVictims()
					}
				}
				if b := g.advancedBuff("celestial_armor"); b != nil && b.Absorb != 0 {
					t.Fatalf("shared absorption was not exhausted: %+v", b)
				}
				for i, c := range g.party.Members[1:3] {
					wantHP := 1
					if budget == 0 && i == 1 {
						wantHP = 0 // The single Preservation charge saved the first hero.
					}
					if c.HitPoints != wantHP || c.HasCondition(character.ConditionUnconscious) != (wantHP == 0) {
						t.Fatalf("hero %d: HP=%d conditions=%v, want HP=%d", i+1, c.HitPoints, c.Conditions, wantHP)
					}
				}
				preservation := g.advancedBuff("preservation")
				if (preservation != nil) != (budget == 6) {
					t.Fatalf("Preservation must be spent only after barrier overflow: %+v", preservation)
				}
			})
		}
	}
}
