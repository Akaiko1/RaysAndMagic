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

// Case table for live tooltip relevance:
// - Equipment: shop/novice/GM x weapon/armor/shield x compact/full.
// - Recovery: HP/SP/both/poison/revive/none x shop/novice/GM x compact/full;
//   editor has no bearer and shares the rule formatter. Auto-use off is separate.
// - Spells: self/party/selected healing, movement/summon/control x compact/full
//   x game/editor. Descriptive effect lists still serve comparisons.
// - Comparisons: identical/changed weapon and spell; only changed facts appear.
// - HUD: all Overwatch tiers in RT/TB; the ready marker shows current chances.
// Persistence: N/A. Formatting is synchronous; no input or saved state changes.

func TestTooltipEquipmentShowsOnlyActiveMastery(t *testing.T) {
	g, _ := newThiefTestGame(t)
	for _, key := range []string{"hunting_bow", "leather_armor", "elven_shield"} {
		for _, tier := range []int{-1, 0, 3} {
			for _, full := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/tier=%d/full=%v", key, tier, full), func(t *testing.T) {
					var ch *character.MMCharacter
					if tier >= 0 {
						ch = character.CreateCharacter("Archer", character.ClassArcher, g.config)
						for _, sk := range []character.SkillType{character.SkillBow, character.SkillLeather, character.SkillShield} {
							ch.Skills[sk] = &character.Skill{Mastery: character.SkillMastery(tier)}
						}
						g.party.Members = []*character.MMCharacter{ch}
					}
					var item items.Item
					if key == "hunting_bow" {
						item = items.CreateWeaponFromYAML(key)
					} else {
						item = items.CreateItemFromYAML(key)
					}
					card := GetItemTooltip(item, ch, g.combat, full)
					if strings.Contains(card, "Grandmaster:") != (tier == 3 && full) {
						t.Fatalf("inactive or missing mastery rule:\n%s", card)
					}
					if key == "hunting_bow" {
						if strings.Contains(card, "Normal Damage:") != (tier == 3 && full) {
							t.Fatalf("normal damage must appear only as a real component of total:\n%s", card)
						}
						if !strings.Contains(card, "Total Damage:") {
							t.Fatal("lost damage total")
						}
					}
				})
			}
		}
	}
}

func TestTooltipRecoveryKeepsRelevantResourceAndDetails(t *testing.T) {
	g, _ := newThiefTestGame(t)
	for _, key := range []string{"health_potion", "mana_potion", "antivenom", "revival_potion", "world_map", "hybrid"} {
		for _, context := range []string{"shop", "archer", "sniper", "editor"} {
			for _, full := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/full=%v", key, context, full), func(t *testing.T) {
					itemKey := key
					if key == "hybrid" {
						itemKey = "health_potion"
					}
					def, _ := config.GetItemDefinition(itemKey)
					if key == "hybrid" {
						original := *def
						t.Cleanup(func() { *def = original })
						def.ManaBase = 10
					}
					var ch *character.MMCharacter
					if context == "archer" || context == "sniper" {
						class := character.ClassArcher
						if context == "sniper" {
							class = character.ClassSniper
						}
						ch = character.CreateCharacter("Bearer", class, g.config)
						if context == "sniper" {
							ch.Skills[character.SkillFieldMedicine].Mastery = character.MasteryGrandMaster
						}
					}
					var card string
					if context == "editor" {
						card = GetItemTooltip(baseTestItem(t, def.Name), nil, nil, full)
					} else {
						card = GetItemTooltip(items.CreateItemFromYAML(itemKey), ch, g.combat, full)
					}
					if strings.Contains(card, "Current recovery: 0 HP") || strings.Contains(card, "Current recovery: 0 SP") {
						t.Fatalf("irrelevant recovery info:\n%s", card)
					}
					wantMedicine := full && ch != nil && ch.HasSkill(character.SkillFieldMedicine) && !def.Revive && (def.HealBase > 0 || def.ManaBase > 0)
					if strings.Contains(card, "Field Medicine:") != wantMedicine {
						t.Fatalf("wrong medicine contribution: %s", card)
					}
					for _, resource := range []struct {
						base, divisor int
						mana          bool
						name          string
					}{
						{def.HealBase, def.HealEnduranceDivisor, false, "HP"},
						{def.ManaBase, def.ManaPersonalityDivisor, true, "SP"},
					} {
						if ch != nil && !def.Revive && resource.base > 0 {
							want := fmt.Sprintf("Current recovery: %d %s", character.ConsumableRestore(ch, resource.base, resource.divisor, resource.mana), resource.name)
							if !strings.Contains(card, want) {
								t.Fatalf("missing %q:\n%s", want, card)
							}
						}
					}
					automatic := !def.Revive && (def.HealBase > 0 || def.ManaBase > 0)
					if strings.Contains(card, "Auto-use when") != automatic || strings.Contains(card, "own quick slots") != (automatic && full) {
						t.Fatalf("incorrect auto-use detail tier:\n%s", card)
					}
					if key == "antivenom" && !strings.Contains(card, "While poisoned:") {
						t.Fatal("lost poison restriction")
					}
					for _, line := range def.RecoveryLines() {
						want := ch == nil || def.Revive
						if strings.Contains(card, line) != want {
							t.Fatalf("incorrect formula visibility:\n%s", card)
						}
					}
				})
			}
		}
	}
}

func TestTooltipRecoveryWithoutAutomaticUse(t *testing.T) {
	g, ch := newThiefTestGame(t)
	original := config.GlobalConfig.Characters.AutoDrink
	t.Cleanup(func() { config.GlobalConfig.Characters.AutoDrink = original })
	config.GlobalConfig.Characters.AutoDrink.ThresholdPct = 0
	for _, full := range []bool{false, true} {
		for _, key := range []string{"health_potion", "mana_potion", "antivenom"} {
			def, _ := config.GetItemDefinition(key)
			for _, card := range []string{GetItemTooltip(items.CreateItemFromYAML(key), ch, g.combat, full), GetItemTooltip(baseTestItem(t, def.Name), nil, nil, full)} {
				if strings.Contains(card, "Auto-use") || strings.Contains(card, "own quick slots") {
					t.Fatalf("disabled automatic use advertised:\n%s", card)
				}
			}
		}
	}
}

func TestTooltipSpellTargetsAndNonAttacks(t *testing.T) {
	g, ch := newThiefTestGame(t)
	for _, tc := range []struct{ key, want, absent string }{
		{"heal", "Target: Self", "Self-target only"},
		{"mass_heal", "Target: Entire Party", "Heals the entire party"},
		{"heal_other", "Can target any party member", "Target: Self"},
		{"jump", "Teleports the party", "Cannot critically hit"},
		{"summon_ice_elemental", "Summons an ally", "Deals no damage"},
		{"charm", "Pacifies", "Total Damage:"},
	} {
		for _, full := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/full=%v", tc.key, full), func(t *testing.T) {
				sd, _ := spells.GetSpellDefinitionByID(spells.SpellID(tc.key))
				_, _ = config.GetSpellDefinition(tc.key)
				for _, card := range []string{GetSpellTooltip(sd.ID, ch, g.combat, full), GetSpellTooltip(spells.SpellID(tc.key), nil, nil, full)} {
					if !strings.Contains(card, tc.want) || strings.Contains(card, tc.absent) {
						t.Fatalf("incorrect spell facts:\n%s", card)
					}
					if tc.key == "charm" && full && !strings.Contains(card, "Can be evaded by Perfect Dodge") {
						t.Fatal("lost control projectile restriction")
					}
				}
			})
		}
	}
}

func TestTooltipComparisonsOmitUnchangedFacts(t *testing.T) {
	g, _ := newThiefTestGame(t)
	ch := character.CreateCharacter("Archer", character.ClassArcher, g.config)
	g.party.Members = []*character.MMCharacter{ch}
	for _, tc := range []struct {
		key  string
		same bool
	}{{"hunting_bow", true}, {"long_bow", false}} {
		t.Run(tc.key, func(t *testing.T) {
			card := GetItemComparisonTooltip(items.CreateWeaponFromYAML(tc.key), ch, g.combat)
			if strings.Contains(card, "(+0") || strings.Contains(card, "No change") != tc.same {
				t.Fatalf("irrelevant comparison:\n%s", card)
			}
			if !tc.same && !strings.Contains(card, "Damage / hit:") {
				t.Fatal("lost changed damage")
			}
		})
	}
	for _, key := range []string{"fireball", "ice_bolt"} {
		card := strings.Join(buildSpellComparisonLinesByID(spells.SpellID(key), "fireball", ch, g.combat), "\n")
		if strings.Contains(card, "(+0") || strings.Contains(card, "No change") != (key == "fireball") {
			t.Fatalf("irrelevant spell comparison:\n%s", card)
		}
	}
}

func TestOverwatchReadyTooltipUsesCurrentTier(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for tier := 0; tier < 4; tier++ {
			t.Run(fmt.Sprintf("TB=%v/tier=%d", tb, tier), func(t *testing.T) {
				g, _, ch, _ := sniperFixture(t, tb)
				ch.Skills[character.SkillOverwatch].Mastery = character.SkillMastery(tier)
				ui := &UISystem{game: g}
				ui.queueOverwatchTooltip(ch, 0, 0)
				card := strings.Join(ui.tooltipLines, "\n")
				chance := character.OverwatchChancePct(tier)
				if len(ui.tooltipLines) != 2 || !strings.Contains(card, fmt.Sprintf("%d%% per tile", chance)) || !strings.Contains(card, fmt.Sprintf("attack: %.0f%%", float64(chance)*character.OverwatchAttackChanceScale)) || strings.Contains(card, character.SkillOverwatch.Description()) {
					t.Fatalf("ready marker is not scoped to current skill:\n%s", card)
				}
			})
		}
	}
}
