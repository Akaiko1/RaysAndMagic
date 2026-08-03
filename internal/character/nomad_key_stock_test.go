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

	want := map[string]int{
		"Ordinary Key": 500,
		"Inlaid Key":   2500,
	}
	for _, entry := range merchant.Inventory {
		cost, ok := want[entry.Name]
		if !ok {
			continue
		}
		if entry.Type != "item" || entry.Cost != cost || entry.Quantity != -1 || entry.Tab != "Road" {
			t.Errorf("%s stock = type %q, cost %d, quantity %d, tab %q", entry.Name, entry.Type, entry.Cost, entry.Quantity, entry.Tab)
		}
		delete(want, entry.Name)
	}
	for name := range want {
		t.Errorf("nomad caravan does not stock %s", name)
	}
}
