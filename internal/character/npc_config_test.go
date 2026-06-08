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

// Trader catalogs list only spell IDs; backfillTraderSpells must fill
// name/school/level/cost/requirements from spells.yaml, and the learn gate's
// min-level must equal the spell's own level (so it can't drift — this is the
// structural fix for the old Water Breathing level mismatch).
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
	if heal.Name == "" || heal.School != "body" || heal.Level == 0 || heal.Cost <= 0 || heal.Requirements == nil {
		t.Errorf("heal not backfilled: %+v", heal)
	}

	// Water Breathing's purchase gate equals its spell level (was a hardcoded 5).
	wb := get("city_spell_shop", "water_breathing")
	def, _ := config.GetSpellDefinition("water_breathing")
	if wb.Requirements == nil || wb.Requirements.MinLevel != def.Level {
		t.Errorf("water_breathing min-level should equal spell level %d, got %+v", def.Level, wb.Requirements)
	}

	// Corner trader: an explicit cost override is preserved (not replaced by the tier default).
	wow := get("mtrader0", "walk_on_water")
	if wow.Cost != 500 {
		t.Errorf("corner walk_on_water cost should be 500, got %d", wow.Cost)
	}
	if len(NPCConfigInstance.NPCs["mtrader0"].Spells) != 1 {
		t.Errorf("corner trader should sell exactly one spell, got %d", len(NPCConfigInstance.NPCs["mtrader0"].Spells))
	}

	// City sells elemental only — no Light/Dark.
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
