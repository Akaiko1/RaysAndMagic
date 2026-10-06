package character

import (
	"path/filepath"
	"testing"

	"ugataima/internal/config"
)

func TestNomadCaravanStocksDoorKeysOnRoadTab(t *testing.T) {
	assets := filepath.Join("..", "..", "assets")
	if _, err := config.LoadItemConfig(filepath.Join(assets, "items.yaml")); err != nil {
		t.Fatalf("load items: %v", err)
	}
	if _, err := config.LoadWeaponConfig(filepath.Join(assets, "weapons.yaml")); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	if _, err := config.LoadSpellConfig(filepath.Join(assets, "spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	if err := LoadNPCConfig(filepath.Join(assets, "npcs.yaml")); err != nil {
		t.Fatalf("load NPCs: %v", err)
	}
	merchant := NPCConfigInstance.NPCs["nomad_city_caravan"]
	if merchant == nil {
		t.Fatal("nomad_city_caravan is missing")
	}

	want := map[string]bool{"Ordinary Key": true, "Inlaid Key": true}
	for _, entry := range merchant.Inventory {
		if !want[entry.Name] {
			continue
		}
		// Unlimited (-1) and priced, on the Road tab.
		if entry.Type != "item" || entry.Cost <= 0 || entry.Quantity != -1 || entry.Tab != "Road" {
			t.Errorf("%s stock = type %q, cost %d, quantity %d, tab %q", entry.Name, entry.Type, entry.Cost, entry.Quantity, entry.Tab)
		}
		delete(want, entry.Name)
	}
	for name := range want {
		t.Errorf("nomad caravan does not stock %s", name)
	}
}
