package items

import (
	"testing"
)

func TestTryCreateWeaponWithoutAccessorReturnsError(t *testing.T) {
	oldAccessor := GlobalWeaponAccessor
	GlobalWeaponAccessor = nil
	t.Cleanup(func() { GlobalWeaponAccessor = oldAccessor })

	if _, err := TryCreateWeaponFromYAML("missing"); err == nil {
		t.Fatal("TryCreateWeaponFromYAML succeeded without a configured accessor")
	}
}

func TestCreateWeaponFromYAML_UsesFlavorWhenPresent(t *testing.T) {
	oldAccessor := GlobalWeaponAccessor
	defer func() { GlobalWeaponAccessor = oldAccessor }()
	GlobalWeaponAccessor = func(key string) (*WeaponDefinitionFromYAML, bool) {
		if key != "tonbogiri" {
			return nil, false
		}
		return &WeaponDefinitionFromYAML{
			Name:        "Tonbogiri, the Dragonfly Spear",
			Description: "A legendary spear so keen a dragonfly landing on it was cut in two",
			Flavor:      "A dragonfly that lit upon the point fell to the ground in two.",
			Category:    "spear",
			Rarity:      "legendary",
			Value:       1700,
		}, true
	}

	item := CreateWeaponFromYAML("tonbogiri")
	want := "A dragonfly that lit upon the point fell to the ground in two."
	if item.Description != want {
		t.Fatalf("weapon item description = %q, want flavor %q", item.Description, want)
	}
}
