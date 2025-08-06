package items

import (
	"testing"
)

func TestCreateWeapon(t *testing.T) {
	item := CreateWeapon("Test Sword", 10, 2, "might", "A test sword")
	if item.Name != "Test Sword" || item.Damage != 10 || item.Range != 2 || item.BonusStat != "might" || item.Description != "A test sword" {
		t.Errorf("CreateWeapon did not set fields correctly: %+v", item)
	}
}

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

func TestSpellEffectToSpellIDAndBack(t *testing.T) {
	effect := SpellEffect("fire")
	id := SpellEffectToSpellID(effect)
	if id != "fire" {
		t.Errorf("SpellEffectToSpellID failed: got %s", id)
	}
	back := SpellIDToSpellEffect(id)
	if back != effect {
		t.Errorf("SpellIDToSpellEffect failed: got %s", back)
	}
}
