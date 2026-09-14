package game

import (
	"fmt"
	"math"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func TestSaveMapResolutionBeforeMutation(t *testing.T) {
	for _, tc := range []struct {
		name, key               string
		unavailable, transition bool
		wantError               bool
	}{
		{"current", "forest", false, false, false},
		{"different", "other", false, false, false},
		{"legacy", "", false, false, false},
		{"missing", "missing", true, false, true},
		{"unloaded_current", "forest", true, false, true},
		{"transition_refused", "other", false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, ts := summonTileWorld(t)
			wm := world.NewWorldManager(g.config)
			wm.LoadedMaps["forest"] = g.world
			wm.LoadedMaps["other"] = newTestWorldSized(g.config, 40, 40)
			setTestWorldManager(t, wm)
			save := g.buildSave(wm)
			save.MapKey, save.PlayerX, save.PlayerY = tc.key, 7.5*ts, 10.5*ts
			if tc.unavailable {
				delete(wm.LoadedMaps, tc.key)
			}
			wm.TransitionInProgress = tc.transition
			beforeWorld, beforeParty, x, y := g.world, g.party, g.camera.X, g.camera.Y
			g.focusedPartyMask = 1
			g.arrows = []Arrow{{ID: "retained", Active: true}}
			err := g.applySave(wm, &save)
			if (err != nil) != tc.wantError {
				t.Fatalf("restore error=%v, wantError=%v", err, tc.wantError)
			}
			if tc.wantError {
				if g.world != beforeWorld || g.party != beforeParty || g.camera.X != x || g.camera.Y != y || wm.CurrentMapKey != "forest" || g.focusedPartyMask != 1 || len(g.arrows) != 1 {
					t.Fatal("rejected save changed the current timeline")
				}
			} else if g.camera.X != save.PlayerX || g.camera.Y != save.PlayerY || len(g.arrows) != 0 {
				t.Fatal("accepted save did not restore its position and transient state")
			}
		})
	}
}

func TestEarnedChoiceRoundTrip(t *testing.T) {
	for _, level := range []int{3, 6} {
		for _, reserve := range []bool{false, true} {
			t.Run(fmt.Sprintf("level_%d/reserve_%v", level, reserve), func(t *testing.T) {
				g, _ := summonTileWorld(t)
				if _, err := config.LoadLevelUpConfig("../../assets/level_up.yaml"); err != nil {
					t.Fatal(err)
				}
				m := g.party.Members[0]
				if reserve {
					g.party.Members = g.party.Members[1:]
					g.party.Reserve = []*character.MMCharacter{m}
				}
				m.Level, m.Experience = level-1, xpStepCost(level-1)
				g.combat.checkLevelUp(m, false)
				wm := world.NewWorldManager(g.config)
				wm.LoadedMaps["forest"] = g.world
				setTestWorldManager(t, wm)
				save := g.buildSave(wm)
				if err := g.applySave(wm, &save); err != nil {
					t.Fatal(err)
				}
				if reserve && !g.swapRosterMember(0, 0) {
					t.Fatal("could not activate reserve")
				}
				if len(g.levelUpChoiceQueue) != 1 || g.levelUpChoiceQueue[0].level != level || len(g.levelUpChoiceQueue[0].options) < MinLevelUpOptions {
					t.Fatalf("earned choice not restored: %+v", g.levelUpChoiceQueue)
				}
			})
		}
	}
}

func TestRoundEligibilityPrecedesStunExpiry(t *testing.T) {
	for _, gear := range []string{"normal", "dual", "suppressor_gun"} {
		for _, stun := range []int{0, 1, 2} {
			t.Run(fmt.Sprintf("%s/stun_%d", gear, stun), func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				g, m := cs.game, cs.game.party.Members[0]
				g.party.Members = []*character.MMCharacter{m}
				g.turnBasedMode = true
				m.Speed = 100
				if gear == "dual" {
					makeDualWielder(t, m)
				}
				if gear == "suppressor_gun" {
					w, err := items.TryCreateWeaponFromYAML(gear)
					if err != nil {
						t.Fatal(err)
					}
					m.Equipment[items.SlotMainHand] = w
				}
				if stun > 0 {
					m.ApplyCharStun(120, stun)
				}
				g.startPartyTurn()
				want := tbPersonalActionFloor(m) + 1
				if stun > 0 {
					want = 0
				}
				if m.ActionsRemaining != want {
					t.Fatalf("actions=%d, want %d", m.ActionsRemaining, want)
				}
				if stun > 0 && m.TBRoundActionFloor != 0 {
					t.Fatal("stunned round credited an equipment floor")
				}
				g.applyEquipmentMutation(0, func() bool { return true })
				if m.ActionsRemaining != want {
					t.Fatal("equipment mutation changed round eligibility")
				}
			})
		}
	}
	// A just-expired stun cannot contribute the party's speed pool either.
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.turnBasedMode = true
	for _, m := range g.party.Members {
		m.Speed = 0
	}
	g.party.Members[0].Speed = 100
	g.party.Members[0].ApplyCharStun(120, 1)
	g.startPartyTurn()
	for _, m := range g.party.Members[1:] {
		if m.ActionsRemaining != m.TBRoundActionFloor {
			t.Fatal("stunned speed contributor granted another member a bonus")
		}
	}
	// A bonus supplied by ANOTHER member still cannot go to the fastest
	// character whose final stun was consumed in this round.
	g.party.Members[0].ApplyCharStun(120, 1)
	g.party.Members[1].Speed = 26
	g.startPartyTurn()
	if g.party.Members[0].ActionsRemaining != 0 || g.party.Members[1].ActionsRemaining != g.party.Members[1].TBRoundActionFloor+1 {
		t.Fatal("another member's speed bonus was awarded to the skipped actor")
	}
}

func TestSkippedRoundWithCardBonusSurvivesSave(t *testing.T) {
	for _, state := range []string{"last_stun", "ongoing_stun", "unconscious"} {
		t.Run(state, func(t *testing.T) {
			g, _ := summonTileWorld(t)
			m := g.party.Members[0]
			g.turnBasedMode = true
			g.party.Members = []*character.MMCharacter{m}
			m.Speed = 100
			card := items.CreateItemFromYAML("puma_card")
			if !g.setCardCollectionSlot(0, card) || g.cardBonusActions() < 1 {
				t.Fatal("card bonus fixture did not activate")
			}
			switch state {
			case "last_stun":
				m.ApplyCharStun(120, 1)
			case "ongoing_stun":
				m.ApplyCharStun(240, 2)
			case "unconscious":
				m.HitPoints = 0
				m.AddCondition(character.ConditionUnconscious)
			}
			g.startPartyTurn()
			if m.ActionsRemaining != 0 || m.TBRoundActionFloor != 0 {
				t.Fatal("card bonus revived skipped round")
			}
			wm := world.NewWorldManager(g.config)
			wm.LoadedMaps["forest"] = g.world
			setTestWorldManager(t, wm)
			save := g.buildSave(wm)
			if err := g.applySave(wm, &save); err != nil {
				t.Fatal(err)
			}
			m = g.party.Members[0]
			if m.ActionsRemaining != 0 || m.TBRoundActionFloor != 0 {
				t.Fatal("save restored an action in the skipped round")
			}
		})
	}
}

func TestZoneCadenceLifetimeAcrossModesAndSave(t *testing.T) {
	for _, saved := range []bool{false, true} {
		for _, tb := range []bool{false, true} {
			for _, tc := range []struct {
				name                       string
				left, phase, elapsed, want int
			}{
				{"before_tick", 1, 118, 360, 0},
				{"on_tick", 1, 119, 360, 1},
				{"two_ticks", 121, 119, 360, 2},
				{"active", 600, 0, 360, 3},
			} {
				t.Run(fmt.Sprintf("%s/TB_%v/saved_%v", tc.name, tb, saved), func(t *testing.T) {
					g, ts := summonTileWorld(t)
					setTestWorldManager(t, nil)
					m := mkTestMonster("Target", 1000)
					m.X, m.Y = 8.5*ts, 10.5*ts
					g.world.Monsters = []*monsterPkg.Monster3D{m}
					g.persistentDamageZones = []PersistentDamageZone{{SpellID: "firewall", X: m.X, Y: m.Y, Radius: ts, FramesLeft: tc.left, tickCounter: tc.phase, IntervalFrames: 120, TickDamage: 10}}
					if saved {
						wm := world.NewWorldManager(g.config)
						wm.LoadedMaps["forest"] = g.world
						setTestWorldManager(t, wm)
						save := g.buildSave(wm)
						if err := g.applySave(wm, &save); err != nil {
							t.Fatal(err)
						}
						// Saved monsters use content keys; the damage target is a synthetic probe.
						g.world.Monsters = []*monsterPkg.Monster3D{m}
					}
					gl := &GameLoop{game: g}
					if tb {
						gl.advancePersistentDamageZones(tc.elapsed)
					} else {
						for range tc.elapsed {
							gl.advancePersistentDamageZones(1)
						}
					}
					if got := (1000 - m.HitPoints) / 10; got != tc.want {
						t.Fatalf("ticks=%d, want %d", got, tc.want)
					}
				})
			}
		}
	}
}

func TestSpellNoEffectPaymentAndProcContract(t *testing.T) {
	for _, id := range []spells.SpellID{"firewall", "jump", "summon_ice_elemental", "raise_dead", "resurrect", "awaken"} {
		for _, cost := range []int{0, 7} {
			t.Run(fmt.Sprintf("%s/cost_%d", id, cost), func(t *testing.T) {
				g, ts := summonTileWorld(t)
				caster := g.party.Members[0]
				forceOrcWarlordSummonAlways(t, g)
				def, err := spells.GetSpellDefinitionByID(id)
				if err != nil {
					t.Fatal(err)
				}
				placePlayerAtTile(g, 1, 1, ts)
				g.camera.Angle = math.Pi
				if id == "summon_ice_elemental" {
					def.SummonMax = 0
				}
				caster.SpellPoints = 100
				weapon, err := items.TryCreateWeaponFromYAML("verdant_eye_scepter")
				if err != nil {
					t.Fatal(err)
				}
				caster.Equipment[items.SlotMainHand] = weapon
				wd, _ := config.GetWeaponDefinition("verdant_eye_scepter")
				old := wd.SpellEchoPct
				wd.SpellEchoPct = 100
				t.Cleanup(func() { wd.SpellEchoPct = old })
				outcome := g.combat.castSpell(spellCastRequest{ID: id, Definition: def, Caster: caster, Cost: cost, ActionProcs: true})
				if outcome != castNoEffect || caster.SpellPoints != 100 {
					t.Fatalf("outcome=%v SP=%d, want no-effect and 100", outcome, caster.SpellPoints)
				}
				if len(g.persistentDamageZones) != 0 || len(g.magicProjectiles) != 0 || len(g.world.Monsters) != 0 {
					t.Fatal("no-effect cast created a successful-cast proc")
				}
			})
		}
	}
}
