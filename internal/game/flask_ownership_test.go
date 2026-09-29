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
					if g.actionCapable(0, rtActCast) != available || g.combat.smartActionAvailable(hero) != available {
						t.Fatal("combat selection disagrees with actor's flask supply")
					}
					actions, cooldown := hero.ActionsRemaining, hero.RTCooldown
					if g.throwFlask(0, key, false) != available {
						t.Fatal("throw disagrees with actor's flask supply")
					}
					if available {
						if len(g.magicProjectiles) != 1 || g.magicProjectiles[0].Attacker != hero || g.flaskStock(hero, key) != want-1 {
							t.Fatal("throw lost actor or failed to spend exactly one bottle")
						}
						if owner != nil && owner.Inventory[0].Count() != 2 || source == "both" && g.party.Inventory[0].Count() != 3 {
							t.Fatal("throw must spend personal stock before shared stock")
						}
					} else if len(g.magicProjectiles) != 0 || hero.ActionsRemaining != actions || hero.RTCooldown != cooldown || owner != nil && owner.Inventory[0].Count() != 3 {
						t.Fatal("unavailable throw changed stock or combat state")
					}
				})
			}
		}
	}
}
