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

// Space availability mirrors its dispatch: a ready slotted throw, else
// SmartAttack's heal. A recharging retort never hides the heal behind it.
func TestSmartAvailabilityFallsThroughRechargingThrow(t *testing.T) {
	for _, tc := range []struct {
		name               string
		recharging, wound  bool
		wantAvail, wantAct bool
	}{
		{"ready", false, false, true, true},
		{"recharging-ally-wounded", true, true, true, true},
		{"recharging-nobody-hurt", true, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, ch := rareClassGame(t, character.ClassAlchemist, false)
			ch.Skills[character.SkillAlchemy].Mastery = character.MasteryNovice
			ch.Skills[character.SkillBombThrowing].Mastery = character.MasteryNovice
			delete(ch.Equipment, items.SlotMainHand)
			delete(ch.Equipment, items.SlotOffHand)
			ch.LearnSpell("heal_other")
			g.party.Inventory = nil
			ch.Inventory = []items.Item{items.CreateItemFromYAML("solstice_everflow_retort")}
			if !g.equipFlask(0, "solstice_everflow_retort") {
				t.Fatal("retort not slotted")
			}
			if tc.recharging {
				ch.Inventory[0].DeviceCooldownFrames = 400
			}
			ally := g.party.Members[1]
			if tc.wound {
				ally.HitPoints = ally.MaxHitPoints / 10
			}
			if got := g.combat.smartActionAvailable(ch); got != tc.wantAvail {
				t.Fatalf("available=%v, want %v", got, tc.wantAvail)
			}
			acted := false
			if _, thrown := g.useSlottedRareAction(0, rtActSmart, false); thrown {
				acted = true
			} else {
				acted = g.combat.SmartAttack().Acted
			}
			if acted != tc.wantAct {
				t.Fatalf("Space acted=%v, want %v", acted, tc.wantAct)
			}
		})
	}
}
