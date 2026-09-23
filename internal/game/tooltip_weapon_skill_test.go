package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
)

// Weapon cards show weapon facts, not the Ballistics skill description.
// Case table: bow/blaster/melee x compact/full x editor/shop/archer/sniper.
// Sniper covers all four mastery tiers; inventory and shop share GetItemTooltip.
// Persistence: N/A, these cards are formatted from the current bearer on demand.
func TestWeaponCardsExcludeBallisticsDescription(t *testing.T) {
	g, _ := newThiefTestGame(t)
	description := character.SkillBallistics.Description()
	if description == "" {
		t.Fatal("Ballistics must retain its own skill description")
	}
	for _, key := range []string{"hunting_bow", "surveyors_rifle", "iron_sword"} {
		weapon := items.CreateWeaponFromYAML(key)
		def := lookupWeaponConfigByName(weapon.Name)
		for _, full := range []bool{false, true} {
			for _, context := range []string{"editor", "shop", "archer", "sniper"} {
				tiers := 1
				if context == "sniper" {
					tiers = 4
				}
				for tier := 0; tier < tiers; tier++ {
					t.Run(fmt.Sprintf("%s/full=%v/%s/tier=%d", key, full, context, tier), func(t *testing.T) {
						var bearer *character.MMCharacter
						if context == "archer" || context == "sniper" {
							class := character.ClassArcher
							if context == "sniper" {
								class = character.ClassSniper
							}
							bearer = character.CreateCharacter("Bearer", class, g.config)
							bearer.Equipment[items.SlotMainHand] = weapon
							if context == "sniper" {
								bearer.Skills[character.SkillBallistics].Mastery = character.SkillMastery(tier)
							}
							g.party.Members = []*character.MMCharacter{bearer}
						}
						var card string
						if context == "editor" {
							card = GetItemTooltip(items.CreateWeaponFromYAML(items.GetWeaponKeyByName(def.Name)), nil, nil, full)
						} else {
							card = GetItemTooltip(weapon, bearer, g.combat, full)
						}
						if strings.Contains(card, description) {
							t.Fatal("weapon card includes the Ballistics skill description")
						}
						if !strings.Contains(card, "Range:") || !strings.Contains(card, "DAMAGE") {
							t.Fatal("weapon facts are missing")
						}
					})
				}
			}
		}
	}
}
