package items

import (
	"testing"
)

func TestCreateBattleSpell(t *testing.T) {
	item := CreateBattleSpell("Fireball", "fire", "Fire", 5, "A fireball spell")
	if item.Name != "Fireball" || item.SpellEffect != "fire" || item.SpellSchool != "Fire" || item.SpellCost != 5 || item.Description != "A fireball spell" {
		t.Errorf("CreateBattleSpell did not set fields correctly: %+v", item)
	}
}

func TestCreateUtilitySpell(t *testing.T) {
	item := CreateUtilitySpell("Heal", "heal", "Body", 3, "A healing spell")
	if item.Name != "Heal" || item.SpellEffect != "heal" || item.SpellSchool != "Body" || item.SpellCost != 3 || item.Description != "A healing spell" {
		t.Errorf("CreateUtilitySpell did not set fields correctly: %+v", item)
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
