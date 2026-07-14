package character

import (
	"path/filepath"
	"testing"

	"ugataima/internal/config"
)

// Regression: a Lich in the party must only be turned away by the Mage Tower
// (the Light-aligned ward), not by every quest-giver. The bug gated the
// rejection on "is a quest giver", so a Lich silenced the Dragon Cliffs hermits
// too - with a tower-flavored line. rejects_lich is now a per-NPC flag.
func TestNPCRejectsLich_MageTowerOnly(t *testing.T) {
	root := func(name string) string { return filepath.Join("..", "..", "assets", name) }
	if _, err := config.LoadItemConfig(root("items.yaml")); err != nil {
		t.Fatalf("load items: %v", err)
	}
	if _, err := config.LoadWeaponConfig(root("weapons.yaml")); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	if _, err := config.LoadSpellConfig(root("spells.yaml")); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	if err := LoadNPCConfig(root("npcs.yaml")); err != nil {
		t.Fatalf("load npcs: %v", err)
	}

	tower, err := CreateNPCFromConfig("mage_tower", 0, 0)
	if err != nil {
		t.Fatalf("create mage_tower: %v", err)
	}
	if !tower.RejectsLich {
		t.Error("mage_tower must set RejectsLich - it gates the Lich rejection")
	}

	// Dragon Cliffs quest-givers must still speak to a Lich party.
	for _, key := range []string{"dragon_cliffs_ranger", "dragon_cliffs_bone_hermit"} {
		npc, err := CreateNPCFromConfig(key, 0, 0)
		if err != nil {
			t.Fatalf("create %s: %v", key, err)
		}
		if npc.RejectsLich {
			t.Errorf("%s must NOT reject Lich - only the Mage Tower does", key)
		}
	}
}
