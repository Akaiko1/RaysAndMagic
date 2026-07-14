package game

import (
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/items"
)

// TestJungleIconsResolve verifies every Deep Jungle weapon/item derives the icon
// name the game looks up (itemTooltipIconName, via the name->key bridge) AND that
// the matching 64x64 PNG is on disk. Guards the flavor-named uniques ("Idol-
// Breaker...", "Mantle of the Idol-King") whose display name != key, and catches any
// future name/key drift that would silently blank an icon.
func TestJungleIconsResolve(t *testing.T) {
	newTestCombatSystemWithConfig(t) // loads weapons+items configs, sets up both bridges

	iconDir := filepath.Join("..", "..", "assets", "sprites", "interface")
	// Icons live in subfolders (items/, weapons/, ...); resolve by basename
	// anywhere under interface/, mirroring the sprite loader's recursive index.
	iconOnDisk := func(name string) bool {
		found := false
		_ = filepath.WalkDir(iconDir, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && d.Name() == name+".png" {
				found = true
			}
			return nil
		})
		return found
	}
	weapons := []string{"jungle_machete", "obsidian_spear", "bone_warclub", "blowgun", "serpent_fang", "idol_breakers_maul"}
	itemKeys := []string{
		"jaguar_pelt_jerkin", "feathered_headdress", "vinewoven_cloak", "serpent_idol_amulet",
		"warlords_signet", "mantle_of_the_idol_king", "ocelot_pelt", "serpent_skin", "tribal_mask",
		"gorilla_heart", "golden_idol", "antivenom", "ocelot_card", "gorilla_titan_card",
		"masked_huntress_card", "orc_warlord_card",
	}

	check := func(item items.Item, want string) {
		got := itemTooltipIconName(item)
		if got != want {
			t.Errorf("%q: itemTooltipIconName = %q, want %q (name->key drift)", item.Name, got, want)
			return
		}
		if !iconOnDisk(want) {
			t.Errorf("%q: icon %s.png not found under %s", item.Name, want, iconDir)
		}
	}

	for _, k := range weapons {
		check(items.CreateWeaponFromYAML(k), "icon_weapon_"+k)
	}
	for _, k := range itemKeys {
		check(items.CreateItemFromYAML(k), "icon_item_"+k)
	}
}
