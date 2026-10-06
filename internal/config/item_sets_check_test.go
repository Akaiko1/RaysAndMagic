package config

import (
	"strings"
	"testing"
)

// Every equipment set parses with a positive size and every member renders
// the shared set lines: the set's name and size, then its completed bonus.
func TestItemSetsParsedAndLinesRender(t *testing.T) {
	cfg, err := LoadItemConfig("../../assets/items.yaml")
	if err != nil {
		t.Fatalf("load items: %v", err)
	}
	weapons, err := LoadWeaponConfig("../../assets/weapons.yaml")
	if err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	members := map[string][]string{}
	for key, def := range cfg.Items {
		if def.Set != "" {
			members[def.Set] = append(members[def.Set], key)
		}
	}
	for key, def := range weapons.Weapons {
		if def.Set != "" {
			members[def.Set] = append(members[def.Set], key)
		}
	}
	if len(cfg.Sets) == 0 {
		t.Fatal("no item sets parsed")
	}
	for key := range cfg.Sets {
		set := GetItemSet(key)
		if set == nil || set.RequiredPieceCount() <= 0 {
			t.Errorf("set %q parsed as %+v, want a positive piece count", key, set)
			continue
		}
		if len(members[key]) == 0 {
			t.Errorf("set %q has no member items", key)
		}
		for _, member := range members[key] {
			var lines []string
			if def, ok := GetItemDefinition(member); ok {
				lines = def.SetLines()
			} else {
				lines = weapons.Weapons[member].SetLines()
			}
			if len(lines) != 2 || !strings.Contains(lines[0], set.Name) {
				t.Errorf("%s SetLines = %v, want set %q + its bonus", member, lines, set.Name)
			}
		}
	}
}

func TestExactItemSetRejectsDuplicateRequiredPiece(t *testing.T) {
	cfg := &ItemSystemConfig{
		Items: map[string]*ItemDefinitionConfig{
			"armor": {Name: "Armor", Set: "pair"},
		},
		Sets: map[string]*ItemSetConfig{
			"pair": {Name: "Pair", PiecesRequired: 2, RequiredPieces: []string{"armor", "armor"}},
		},
	}
	if err := validateEquipmentSetReferences(cfg, nil); err == nil {
		t.Fatal("duplicate exact set piece must fail validation")
	}
}
