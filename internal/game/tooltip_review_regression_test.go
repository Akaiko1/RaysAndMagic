package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// Compact/full, base/live, and every source of true damage. These use the
// public dispatcher, so moving the summary back behind Shift fails the test.
func TestTooltipTrueDamageAlwaysVisible(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	for _, tc := range []struct {
		name, key                 string
		mastery, card, base, want bool
	}{
		{"normal", "iron_sword", false, false, false, false},
		{"authored base", "broodspike", false, false, true, true},
		{"authored live", "broodspike", false, false, false, true},
		{"mastery melee", "iron_sword", true, false, false, true},
		{"mastery ranged", "hunting_bow", true, false, false, true},
		{"card", "iron_sword", false, true, false, true},
		{"all sources", "broodspike", true, true, false, true},
		{"multiple strikes", "suppressor_gun", true, false, false, true},
		{"base isolation", "iron_sword", true, true, true, false},
	} {
		for _, full := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/full=%v", tc.name, full), func(t *testing.T) {
				ch := gmReferenceChar(cs.game.config)
				if !tc.mastery {
					ch.Skills = map[character.SkillType]*character.Skill{}
				}
				it := items.CreateWeaponFromYAML(tc.key)
				items.EnsureInstanceID(&it)
				ch.Equipment = map[items.EquipSlot]items.Item{items.SlotMainHand: it}
				cs.game.party.Members = []*character.MMCharacter{ch}
				cs.game.cardSlots = [MaxCardSlots]cardSlot{}
				if tc.card {
					cs.game.setCardCollectionSlot(0, items.CreateItemFromYAML("samurai_card"))
				}
				if tc.base {
					ch = nil
				}
				text := GetItemTooltip(it, ch, cs, full)
				calculator := cs
				if tc.base {
					calculator = nil
				}
				damage := calculator.calculateWeaponDamagePreview(it, ch)
				if (damage.True > 0) != tc.want {
					t.Fatalf("fixture lacks intended damage source: %+v", damage)
				}
				summary := fmt.Sprintf("Total Damage: %d", damage.Total)
				if tc.want {
					summary += fmt.Sprintf(" (%d Normal + %d True)", damage.Normal, damage.True)
				}
				if !strings.Contains(text, summary+"\n") {
					t.Fatalf("missing compact damage total %q: %s", summary, text)
				}
				if !tc.want && strings.Contains(text, " True)") {
					t.Fatalf("false true-damage summary: %s", text)
				}
				if strings.Contains(text, "True Damage ignores armor and dodge") != (tc.want && full) {
					t.Fatalf("bypass explanation must be Shift-only and require true damage: %s", text)
				}
			})
		}
	}
	for _, id := range []spells.SpellID{"fireball", "hot_steam", "firewall", "inferno"} {
		for _, tier := range []character.SkillMastery{character.MasteryNovice, character.MasteryGrandMaster} {
			for _, full := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/tier%d/full%v", id, tier, full), func(t *testing.T) {
					ch := gmReferenceChar(cs.game.config)
					for _, skill := range ch.MagicSchools {
						skill.Mastery = tier
					}
					cs.game.cardSlots = [MaxCardSlots]cardSlot{}
					text := GetSpellTooltip(id, ch, cs, full)
					want := tier == character.MasteryGrandMaster && (id == "fireball" || id == "hot_steam")
					if strings.Contains(text, " True)") != want {
						t.Fatal(text)
					}
					if want {
						def, err := spells.GetSpellDefinitionByID(id)
						if err != nil {
							t.Fatal(err)
						}
						breakdown := character.SpellDamageBreakdown(def, ch)
						parts := cs.spellDamageParts(id, ch, breakdown.Total)
						parts, _ = cs.spellPartsWithOutgoingBuff(parts, def.School)
						label := "Total Damage"
						if def.DamageFormula().Kind == spells.DamageZone {
							label = "Total per tick"
						}
						summary := fmt.Sprintf("%s: %d (%d Normal + %d True)", label, parts.Total(), parts.Normal, parts.True)
						if !strings.Contains(text, summary+"\n") {
							t.Fatalf("missing compact damage total %q: %s", summary, text)
						}
					}
					if strings.Contains(text, "True Damage ignores armor and dodge") != (want && full) {
						t.Fatalf("bypass explanation must be Shift-only and require true damage: %s", text)
					}
				})
			}
		}
	}
}

func TestItemTooltipPreservesDistinctAuthoredProse(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	for _, weapon := range []bool{false, true} {
		for _, tc := range []struct{ name, description, flavor string }{
			{"neither", "", ""}, {"description", "Authored description.", ""},
			{"flavor", "", "Authored flavor."}, {"both", "Authored description.", "Authored flavor."},
			{"identical", "Same authored text.", "Same authored text."},
		} {
			t.Run(fmt.Sprintf("weapon%v/%s", weapon, tc.name), func(t *testing.T) {
				var it items.Item
				if weapon {
					def, _ := config.GetWeaponDefinition("tonbogiri")
					original := *def
					defer func() { *def = original }()
					def.Description, def.Flavor = tc.description, tc.flavor
					it = items.CreateWeaponFromYAML("tonbogiri")
				} else {
					def, _ := config.GetItemDefinition("troll_card")
					original := *def
					defer func() { *def = original }()
					def.Description, def.Flavor = tc.description, tc.flavor
					it = items.CreateItemFromYAML("troll_card")
				}
				it.Description = "Obsolete saved prose."
				for _, full := range []bool{false, true} {
					text := GetItemTooltip(it, nil, nil, full)
					for _, want := range []string{tc.description, tc.flavor} {
						if want != "" && strings.Count(text, want) != 1 {
							t.Fatalf("missing or repeated %q: %s", want, text)
						}
					}
					if strings.Contains(text, it.Description) {
						t.Fatal("known item uses stale saved prose")
					}
				}
			})
		}
	}
	legacy := items.Item{Name: "Removed item", Type: items.ItemQuest, Description: "Only surviving description."}
	if text := GetItemTooltip(legacy, nil, nil, true); strings.Count(text, legacy.Description) != 1 {
		t.Fatal(text)
	}
}

func TestMonsterSpellPublicTooltipsDescribeMonsterCasting(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	checked := 0
	for key, def := range config.GlobalSpells.Spells {
		if !def.MonsterOnly {
			continue
		}
		checked++
		for _, full := range []bool{false, true} {
			for _, live := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/full%v/live%v", key, full, live), func(t *testing.T) {
					var ch *character.MMCharacter
					if live {
						ch = gmReferenceChar(cs.game.config)
					}
					it, err := spells.CreateSpellItem(spells.SpellID(key))
					if err != nil {
						t.Fatal(err)
					}
					for _, text := range []string{GetSpellTooltip(spells.SpellID(key), ch, cs, full), GetItemTooltip(it, ch, cs, full)} {
						if !strings.Contains(text, "Cast by monsters only") {
							t.Fatal(text)
						}
						for _, bad := range []string{"Hitbox:", "Cost:", "Intellect /", "Mastery -", "Base ("} {
							if strings.Contains(text, bad) {
								t.Fatalf("monster spell contains player/internal rule %q: %s", bad, text)
							}
						}
						sd, _ := spells.GetSpellDefinitionByID(spells.SpellID(key))
						if sd.IsProjectile && !sd.DealsNoDamage && !strings.Contains(text, "casting monster's attack damage") {
							t.Fatal(text)
						}
					}
				})
			}
		}
	}
	if checked == 0 {
		t.Fatal("no monster spell fixtures")
	}
	text := GetSpellTooltip("fireball", gmReferenceChar(cs.game.config), cs, true)
	if strings.Contains(text, "Cast by monsters only") || !strings.Contains(text, "Cost:") {
		t.Fatal(text)
	}
}
