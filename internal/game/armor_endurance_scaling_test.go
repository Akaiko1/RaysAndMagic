package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

func TestArmorEnduranceScalingDataIsCategoryWide(t *testing.T) {
	itemCfg, err := config.LoadItemConfig("../../assets/items.yaml")
	if err != nil {
		t.Fatalf("load items: %v", err)
	}

	expected := map[string]int{
		"leather": 10,
		"chain":   7,
		"plate":   5,
	}

	for key, def := range itemCfg.Items {
		if def.Type != "armor" {
			continue
		}
		if want, ok := expected[def.ArmorType]; ok {
			if def.EnduranceScalingDivisor != want {
				t.Errorf("%s (%s) endurance_scaling_divisor = %d, want %d", key, def.ArmorType, def.EnduranceScalingDivisor, want)
			}
			continue
		}
		if def.EnduranceScalingDivisor != 0 {
			t.Errorf("%s (%s) must not scale from Endurance, got divisor %d", key, def.ArmorType, def.EnduranceScalingDivisor)
		}
	}
}

func TestArmorEnduranceScalingIgnoresUnsupportedCategories(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ch := cs.game.party.Members[0]
	ch.Endurance = 100
	ch.Skills[character.SkillShield] = gmSkill()

	for _, tc := range []struct {
		name string
		item items.Item
	}{
		{
			name: "cloth",
			item: items.Item{ArmorCategory: "cloth", Attributes: map[string]int{
				"armor_class_base":          3,
				"endurance_scaling_divisor": 1,
			}},
		},
		{
			name: "shield",
			item: items.Item{ArmorCategory: "shield", Attributes: map[string]int{
				"armor_class_base":          3,
				"endurance_scaling_divisor": 1,
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := cs.CalculateArmorClassContribution(tc.item, ch)
			want := tc.item.Attributes["armor_class_base"] + cs.armorMasteryBonus(ch, tc.item)
			if got != want {
				t.Fatalf("AC = %d, want %d; unsupported category must ignore stale endurance scaling", got, want)
			}
		})
	}
}
