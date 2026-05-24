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
