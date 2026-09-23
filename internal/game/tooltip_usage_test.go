package game

import (
	"slices"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

func TestItemUsageGameEditorCatalog(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	for key, def := range config.GlobalItems.Items {
		t.Run(key, func(t *testing.T) {
			item := items.CreateItemFromYAML(key)
			editor := GetItemTooltip(baseTestItem(t, def.Name), nil, nil, true)
			for _, full := range []bool{false, true} {
				rendered := GetItemTooltip(item, cs.game.party.Members[0], cs, full)
				for _, line := range def.TooltipUsageLines() {
					if !strings.Contains(rendered, line) || !strings.Contains(editor, line) {
						t.Errorf("game/editor usage mismatch: %q", line)
					}
				}
				for _, r := range rendered {
					if r > 127 {
						t.Fatalf("non-ASCII rendered tooltip: %q", rendered)
					}
				}
				if item.Type == items.ItemCard {
					if !strings.Contains(rendered, "Card Collector") || !strings.Contains(rendered, "only while") || strings.Contains(rendered, "sell to merchants") {
						t.Fatalf("card usage does not explain activation: %s", rendered)
					}
				}
				if item.Type == items.ItemQuest && strings.Contains(rendered, "Cannot be dropped") == itemDiscardable(item) {
					t.Fatalf("tooltip disagrees with discard rule: %s", rendered)
				}
			}
		})
	}
}

func TestItemUsageKindsAndAuthoredOverrides(t *testing.T) {
	for _, tc := range []struct {
		key          string
		want, absent []string
	}{
		{"medusa_card", []string{"Card Collector", "active collection"}, []string{"Double-click", "sell to merchants"}},
		{"health_potion", []string{"Double-click in inventory to use", "Consumed on use"}, nil},
		{"world_map", []string{"Double-click in inventory to use", "Cannot be dropped"}, []string{"Consumed on use"}},
		{"lich_phylactery", []string{"Consumed after promotion"}, []string{"Cannot be dropped"}},
		{"black_dragon_statuette", []string{"Cannot be dropped"}, []string{"Double-click"}},
		{"ordinary_key", []string{"Choose at a locked door", "Consumed when it opens a door"}, []string{"sell to merchants"}},
		{"skeleton_key", []string{"Choose at a locked door", "Never consumed"}, []string{"Consumed on use"}},
		{"clock_hand", []string{"Exchange with the Clockmaker"}, []string{"sell to merchants"}},
		{"black_dragon_scale", []string{"Exchange with the Scalewright", "gold is also required"}, []string{"sell to merchants"}},
		{"red_dragon_scale", []string{"Exchange with the Scalewright", "gold is also required"}, []string{"sell to merchants"}},
		{"green_dragon_scale", []string{"Exchange with the Scalewright", "gold is also required"}, []string{"sell to merchants"}},
		{"gold_dragon_scale", []string{"Exchange with the Scalewright", "gold is also required"}, []string{"sell to merchants"}},
	} {
		t.Run(tc.key, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			text := GetItemTooltip(items.CreateItemFromYAML(tc.key), cs.game.party.Members[0], cs, false)
			for _, s := range tc.want {
				if !strings.Contains(text, s) {
					t.Errorf("missing %q: %s", s, text)
				}
			}
			for _, s := range tc.absent {
				if strings.Contains(text, s) {
					t.Errorf("misleading %q: %s", s, text)
				}
			}
		})
	}
	def := &config.ItemDefinitionConfig{Type: "card", TooltipUsage: []string{"Special authored use"}}
	got := def.TooltipUsageLines()
	got[0] = "changed"
	if !slices.Equal(def.TooltipUsage, []string{"Special authored use"}) {
		t.Fatal("usage formatter mutates YAML data")
	}
}

func TestItemCurrencyUsageFollowsMerchantStock(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatal(err)
	}
	checked := map[string]bool{}
	for npcKey := range character.NPCConfigInstance.NPCs {
		npc, err := character.CreateNPCFromConfig(npcKey, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, stock := range npc.MerchantStock {
			key, ok := character.CurrencyItemKey(stock.EffectiveCurrency(npc.Currency))
			if !ok || checked[key] {
				continue
			}
			checked[key] = true
			text := GetItemTooltip(items.CreateItemFromYAML(key), cs.game.party.Members[0], cs, false)
			if !strings.Contains(text, "Exchange with") || strings.Contains(text, "sell to merchants") {
				t.Errorf("%s currency %s lacks an exchange hint: %s", npcKey, key, text)
			}
		}
	}
	if len(checked) == 0 {
		t.Fatal("no authored item currencies checked")
	}
}
