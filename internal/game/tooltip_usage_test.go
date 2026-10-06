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

// Each item kind picks its default usage wording; an authored tooltip_usage
// replaces the defaults. Authored currency hints: TestItemCurrencyUsageFollowsMerchantStock.
func TestItemUsageKindsAndAuthoredOverrides(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	u := config.GlobalItems.TooltipUsageDefaults
	authored := func(d *config.ItemDefinitionConfig) bool { return len(d.TooltipUsage) > 0 }
	for _, tc := range []struct {
		key          string
		fixture      func(*config.ItemDefinitionConfig) bool
		want, absent []string
	}{
		{"medusa_card", func(d *config.ItemDefinitionConfig) bool { return d.Type == "card" && !authored(d) },
			u.Card, []string{u.ActivateInventory, u.Trinket}},
		{"health_potion", func(d *config.ItemDefinitionConfig) bool { return d.Type == "consumable" && !authored(d) },
			[]string{u.ActivateInventory, u.ConsumedOnUse}, nil},
		{"world_map", func(d *config.ItemDefinitionConfig) bool { return d.Type == "quest" && d.OpensMap && !d.Discardable },
			[]string{u.ActivateInventory, u.CannotDrop}, []string{u.ConsumedOnUse}},
		{"lich_phylactery", func(d *config.ItemDefinitionConfig) bool { return d.Type == "quest" && d.PromotesLich && d.Discardable },
			[]string{u.ConsumedAfterPromotion}, []string{u.CannotDrop}},
		{"black_dragon_statuette", func(d *config.ItemDefinitionConfig) bool {
			return d.Type == "quest" && !d.OpensMap && !d.PromotesLich && !d.Discardable
		}, []string{u.CannotDrop}, []string{u.ActivateInventory}},
		{"ordinary_key", func(d *config.ItemDefinitionConfig) bool { return d.Type == "trinket" && authored(d) },
			nil, []string{u.Trinket}},
		{"skeleton_key", func(d *config.ItemDefinitionConfig) bool { return d.Type == "trinket" && authored(d) },
			nil, []string{u.Trinket, u.ConsumedOnUse}},
	} {
		t.Run(tc.key, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			def, ok := config.GetItemDefinition(tc.key)
			if !ok || !tc.fixture(def) {
				t.Fatalf("fixture %s no longer exercises its usage branch", tc.key)
			}
			text := GetItemTooltip(items.CreateItemFromYAML(tc.key), cs.game.party.Members[0], cs, false)
			for _, s := range append(slices.Clone(tc.want), def.TooltipUsage...) {
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
