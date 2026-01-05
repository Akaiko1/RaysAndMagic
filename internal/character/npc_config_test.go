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
