package character

import (
	"path/filepath"
	"testing"
	"ugataima/internal/config"
)

func TestCreateNPCFromConfig_MerchantStock(t *testing.T) {
	if _, err := config.LoadItemConfig(filepath.Join("..", "..", "assets", "items.yaml")); err != nil {
		t.Fatalf("load items: %v", err)
	}
	if _, err := config.LoadWeaponConfig(filepath.Join("..", "..", "assets", "weapons.yaml")); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	if err := LoadNPCConfig(filepath.Join("..", "..", "assets", "npcs.yaml")); err != nil {
		t.Fatalf("load npcs: %v", err)
	}

	npc, err := CreateNPCFromConfig("merchant_general", 0, 0)
	if err != nil {
		t.Fatalf("create npc: %v", err)
	}
	if len(npc.MerchantStock) == 0 {
		t.Fatalf("expected merchant stock to be populated")
	}

	foundPotion := false
	for _, entry := range npc.MerchantStock {
		if entry.Item.Name == "Health Potion" {
			foundPotion = true
			if entry.Cost != 50 {
				t.Fatalf("expected Health Potion cost 50, got %d", entry.Cost)
			}
			if entry.Quantity != 10 {
				t.Fatalf("expected Health Potion quantity 10, got %d", entry.Quantity)
			}
		}
	}
	if !foundPotion {
		t.Fatalf("expected Health Potion in merchant stock")
	}
}

func TestCreateNPCFromConfig_MerchantSellAvailable(t *testing.T) {
	if _, err := config.LoadItemConfig(filepath.Join("..", "..", "assets", "items.yaml")); err != nil {
		t.Fatalf("load items: %v", err)
	}
	if _, err := config.LoadWeaponConfig(filepath.Join("..", "..", "assets", "weapons.yaml")); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	if err := LoadNPCConfig(filepath.Join("..", "..", "assets", "npcs.yaml")); err != nil {
		t.Fatalf("load npcs: %v", err)
	}

	npc, err := CreateNPCFromConfig("desert_merchant", 0, 0)
	if err != nil {
		t.Fatalf("create npc: %v", err)
	}
	if !npc.SellAvailable {
		t.Fatalf("expected desert_merchant to allow selling")
	}
	if len(npc.MerchantStock) != 0 {
		t.Fatalf("expected desert_merchant to have no stock")
	}
}

func TestMerchantStockItemEffectiveCurrency(t *testing.T) {
	plain := &MerchantStockItem{}
	if got := plain.EffectiveCurrency(CurrencyArenaPoints); got != CurrencyArenaPoints {
		t.Fatalf("shop currency = %q, want %q", got, CurrencyArenaPoints)
	}

	override := &MerchantStockItem{CurrencyItem: "black_dragon_scale"}
	want := CurrencyItemPrefix + "black_dragon_scale"
	if got := override.EffectiveCurrency(""); got != want {
		t.Fatalf("entry currency = %q, want %q", got, want)
	}
}

// Trader catalogs author spell IDs and costs; backfillTraderSpells must fill
// name, school and description from spells.yaml without inventing a purchase
// level or mastery gate.
func TestBackfillTraderSpells(t *testing.T) {
	if _, err := config.LoadSpellConfig(filepath.Join("..", "..", "assets", "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	if err := LoadNPCConfig(filepath.Join("..", "..", "assets", "npcs.yaml")); err != nil {
		t.Fatalf("load npcs: %v", err)
	}

	get := func(npcKey, spellID string) *NPCSpell {
		t.Helper()
		npc, ok := NPCConfigInstance.NPCs[npcKey]
		if !ok || npc.Spells == nil {
			t.Fatalf("%s has no spells", npcKey)
		}
		sp := npc.Spells[spellID]
		if sp == nil {
			t.Fatalf("%s should sell %s", npcKey, spellID)
		}
		return sp
	}

	// Lake trader: a Body spell entry given as just an ID is fully backfilled.
	heal := get("spell_trader_mage", "heal")
	if heal.Name == "" || heal.School != "body" || heal.Cost <= 0 {
		t.Errorf("heal not backfilled: %+v", heal)
	}

	// Catalog entries carry price + identity only - there is no purchase gate to
	// backfill, so a bare ID must still resolve its school and cost.
	wb := get("city_spell_shop", "water_breathing")
	if wb.Name == "" || wb.School != "water" || wb.Cost <= 0 {
		t.Errorf("water_breathing not backfilled: %+v", wb)
	}

	// Corner trader: an explicit cost override is preserved (not replaced by the tier default).
	wow := get("mtrader0", "walk_on_water")
	if wow.Cost != 500 {
		t.Errorf("corner walk_on_water cost should be 500, got %d", wow.Cost)
	}
	if len(NPCConfigInstance.NPCs["mtrader0"].Spells) != 1 {
		t.Errorf("corner trader should sell exactly one spell, got %d", len(NPCConfigInstance.NPCs["mtrader0"].Spells))
	}

	// City sells elemental only - no Light/Dark.
	for _, sp := range NPCConfigInstance.NPCs["city_spell_shop"].Spells {
		if sp.School == "light" || sp.School == "dark" {
			t.Errorf("city shop must not sell light/dark, found %q (%s)", sp.Name, sp.School)
		}
	}
}

func TestCreateNPCFromConfig_EncounterMessages(t *testing.T) {
	if err := LoadNPCConfig(filepath.Join("..", "..", "assets", "npcs.yaml")); err != nil {
		t.Fatalf("load npcs: %v", err)
	}

	npc, err := CreateNPCFromConfig("shipwreck_bandit_camp", 0, 0)
	if err != nil {
		t.Fatalf("create npc: %v", err)
	}
	if npc.DialogueData == nil || npc.DialogueData.VisitedMessage == "" {
		t.Fatalf("expected visited_message to be set for shipwreck encounter")
	}
	if npc.EncounterData == nil || npc.EncounterData.StartMessage == "" {
		t.Fatalf("expected start_message to be set for shipwreck encounter")
	}
}
