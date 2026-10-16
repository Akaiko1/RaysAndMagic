package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
)

func TestFlaskOwnershipThroughCombat(t *testing.T) {
	for _, key := range []string{"harm_flask", "venom_flask", "fire_flask"} {
		for _, tb := range []bool{false, true} {
			for _, source := range []string{"own", "shared", "both", "other", "reserve", "captive", "empty", "restored"} {
				t.Run(fmt.Sprintf("%s/TB=%v/%s", key, tb, source), func(t *testing.T) {
					g, hero := rareClassGame(t, character.ClassAlchemist, tb)
					g.selectedChar = 1 // Availability and payment must use the actor.
					g.party.Inventory = nil
					for _, ch := range g.party.Members {
						ch.Inventory = nil
					}
					it := items.CreateItemFromYAML(key)
					it.Quantity = 3
					var owner *character.MMCharacter
					switch source {
					case "own", "both", "restored":
						owner = hero
					case "other":
						owner = g.party.Members[1]
					case "reserve", "captive":
						owner = character.CreateCharacter("Absent", character.ClassAlchemist, g.config)
						if source == "reserve" {
							g.party.Reserve = []*character.MMCharacter{owner}
						} else {
							g.party.Captive = []*character.MMCharacter{owner}
						}
					}
					if source != "empty" {
						if owner == nil {
							g.party.Inventory = []items.Item{it}
						} else {
							owner.Inventory = []items.Item{it}
						}
					}
					if source == "both" {
						g.party.Inventory = []items.Item{it}
					}
					if source == "restored" {
						hero = restoreCharacterSave(buildCharacterSave(hero))
						g.party.Members[0], owner = hero, hero
					}
					delete(hero.Equipment, items.SlotMainHand)
					delete(hero.Equipment, items.SlotOffHand)
					available := source == "own" || source == "shared" || source == "both" || source == "restored"
					want := 0
					if available {
						want = 3
					}
					if source == "both" {
						want = 6
					}
					if !g.equipFlask(0, key) || g.flaskStock(hero, key) != want {
						t.Fatalf("equip/stock does not describe actor: got %d want %d", g.flaskStock(hero, key), want)
					}
					messages := g.GetCombatMessages()
					if !strings.Contains(messages[len(messages)-1], fmt.Sprintf("(%d in bag)", want)) {
						t.Fatal("equip message uses another hero's stock")
					}
					if g.actionCapable(0, rtActSlotted) != available || g.combat.smartActionAvailable(hero) != available {
						t.Fatal("combat selection disagrees with actor's flask supply")
					}
					actions, cooldown := hero.ActionsRemaining, hero.RTCooldown
					if g.throwFlask(0, key, false) != available {
						t.Fatal("throw disagrees with actor's flask supply")
					}
					if available {
						if len(g.flasks) != 1 || g.flasks[0].Attacker != hero || g.flaskStock(hero, key) != want-1 {
							t.Fatal("throw lost actor or failed to spend exactly one bottle")
						}
						if owner != nil && owner.Inventory[0].Count() != 2 || source == "both" && g.party.Inventory[0].Count() != 3 {
							t.Fatal("throw must spend personal stock before shared stock")
						}
					} else if len(g.flasks) != 0 || hero.ActionsRemaining != actions || hero.RTCooldown != cooldown || owner != nil && owner.Inventory[0].Count() != 3 {
						t.Fatal("unavailable throw changed stock or combat state")
					}
				})
			}
		}
	}
}

// Space heals a wounded ally first when the hero can cast the heal, then
// throws a ready slotted flask (an always-ready reusable Retort included),
// else falls through to SmartAttack. Driven through the real Space entry
// points in real time and turn-based; a throw that is not ready (its last
// flask gone) never hides the heal behind it.
func TestSmartAvailabilityFallsThroughUnreadyThrow(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		flask                          string
		empty, wound, noSP             bool
		wantAvail, wantHeal, wantThrow bool
	}{
		{"ready", "fire_flask", false, false, false, true, false, true},
		{"ready-ally-wounded-heals", "fire_flask", false, true, false, true, true, false},
		{"retort-ally-wounded-heals", "solstice_everflow_retort", false, true, false, true, true, false},
		{"retort-ally-wounded-no-sp-throws", "solstice_everflow_retort", false, true, true, true, false, true},
		{"retort-nobody-hurt-throws", "solstice_everflow_retort", false, false, false, true, false, true},
		{"empty-ally-wounded", "fire_flask", true, true, false, true, true, false},
		{"empty-nobody-hurt", "fire_flask", true, false, false, false, false, false},
	} {
		for _, tb := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/TB=%v", tc.name, tb), func(t *testing.T) {
				g, ch := rareClassGame(t, character.ClassAlchemist, tb)
				ch.Skills[character.SkillAlchemy].Mastery = character.MasteryNovice
				ch.Skills[character.SkillBombThrowing].Mastery = character.MasteryNovice
				delete(ch.Equipment, items.SlotMainHand)
				delete(ch.Equipment, items.SlotOffHand)
				ch.LearnSpell("heal_other")
				if tc.noSP {
					ch.SpellPoints = 0
				}
				g.party.Inventory = nil
				ch.Inventory = []items.Item{items.CreateItemFromYAML(tc.flask)}
				if !g.equipFlask(0, tc.flask) {
					t.Fatal("flask not slotted")
				}
				if tc.empty { // the slotted throw is not ready: its last flask is gone
					ch.Inventory = nil
				}
				ally := g.party.Members[1]
				if tc.wound {
					ally.HitPoints = ally.MaxHitPoints / 10
				}
				if got := g.combat.smartActionAvailable(ch); got != tc.wantAvail {
					t.Fatalf("available=%v, want %v", got, tc.wantAvail)
				}
				hp, actions := ally.HitPoints, ch.ActionsRemaining
				ih := NewInputHandler(g)
				if tb {
					ih.performTurnBasedSmartAttack()
				} else {
					ih.performRTCombatAction(rtActSmart, true)
				}
				healed, thrown := ally.HitPoints > hp, len(g.flasks) > 0
				spent := ch.RTCooldown > 0
				if tb {
					spent = ch.ActionsRemaining < actions
				}
				if healed != tc.wantHeal || thrown != tc.wantThrow || spent != (tc.wantHeal || tc.wantThrow) {
					t.Fatalf("Space healed=%v thrown=%v spent=%v, want healed=%v thrown=%v", healed, thrown, spent, tc.wantHeal, tc.wantThrow)
				}
			})
		}
	}
}
