package character

// Equipment slot routing tests — exercise the real MMCharacter.EquipItem /
// UnequipItem path against items loaded from items.yaml. Damage-reduction
// formulas are NOT tested here (they live in internal/game/CombatSystem and
// are covered by internal/game/armor_reduction_test.go).
//
// TestMain in main_test.go has already wired items.GlobalItemAccessor via
// bridge.SetupItemBridge() before any test in this package runs, so
// items.CreateItemFromYAML works in every test below without extra setup.

import (
	"testing"

	"ugataima/internal/items"
)

func TestEquipmentSlotAssignment(t *testing.T) {
	character := &MMCharacter{
		Name:      "TestKnight",
		Endurance: 20,
		Skills: map[SkillType]*Skill{
			SkillLeather: {Mastery: MasteryNovice},
			SkillPlate:   {Mastery: MasteryNovice},
		},
		Equipment: make(map[items.EquipSlot]items.Item),
	}

	cases := []struct {
		itemKey      string
		expectedSlot items.EquipSlot
	}{
		{"leather_armor", items.SlotArmor},
		{"leather_helmet", items.SlotHelmet},
		{"leather_pants", items.SlotBoots},
		{"magic_ring", items.SlotRing1},
		{"iron_armor", items.SlotArmor},
	}

	for _, tc := range cases {
		item := items.CreateItemFromYAML(tc.itemKey)
		if _, _, ok := character.EquipItem(item); !ok {
			t.Errorf("EquipItem(%s) failed", item.Name)
			continue
		}
		got, ok := character.Equipment[tc.expectedSlot]
		if !ok {
			t.Errorf("%s not in expected slot %d", item.Name, tc.expectedSlot)
			continue
		}
		if got.Name != item.Name {
			t.Errorf("slot %d holds %q, expected %q", tc.expectedSlot, got.Name, item.Name)
		}
		// Clean up for next iteration so we don't accidentally hit slot conflicts.
		character.UnequipItem(tc.expectedSlot)
	}
}

func TestEquipmentStatBonusesFromYAML(t *testing.T) {
	character := &MMCharacter{
		Name:        "TestMage",
		Intellect:   30,
		Personality: 25,
		Endurance:   20,
		Equipment:   make(map[items.EquipSlot]items.Item),
	}

	// magic_ring (items.yaml) provides:
	//   intellect_scaling_divisor: 6  → +Intellect/6
	//   personality_scaling_divisor: 8 → +Personality/8
	magicRing := items.CreateItemFromYAML("magic_ring")
	character.EquipItem(magicRing)

	_, intellect, personality, endurance, _, _, _ := character.GetEffectiveStats(0)

	wantInt := 30 + (30 / 6)   // 35
	wantPer := 25 + (25 / 8)   // 28
	wantEnd := 20              // ring has no endurance bonus

	if intellect != wantInt {
		t.Errorf("intellect: got %d, want %d", intellect, wantInt)
	}
	if personality != wantPer {
		t.Errorf("personality: got %d, want %d", personality, wantPer)
	}
	if endurance != wantEnd {
		t.Errorf("endurance: got %d, want %d", endurance, wantEnd)
	}
}

func TestEquipReplacementReturnsPrevious(t *testing.T) {
	character := &MMCharacter{
		Name: "TestKnight",
		Skills: map[SkillType]*Skill{
			SkillLeather: {Mastery: MasteryNovice},
			SkillPlate:   {Mastery: MasteryNovice},
		},
		Equipment: make(map[items.EquipSlot]items.Item),
	}

	leather := items.CreateItemFromYAML("leather_armor")
	if _, hadPrev, ok := character.EquipItem(leather); !ok || hadPrev {
		t.Fatalf("first equip should succeed with no previous item")
	}

	iron := items.CreateItemFromYAML("iron_armor")
	prev, hadPrev, ok := character.EquipItem(iron)
	if !ok {
		t.Fatalf("second equip failed")
	}
	if !hadPrev {
		t.Errorf("expected previous item flag when replacing leather with iron")
	}
	if prev.Name != "Leather Armor" {
		t.Errorf("previous item name: got %q, want Leather Armor", prev.Name)
	}
	if got := character.Equipment[items.SlotArmor]; got.Name != "Iron Armor" {
		t.Errorf("armor slot should now hold Iron Armor, got %q", got.Name)
	}
}

func TestUnequipReturnsItemAndClearsSlot(t *testing.T) {
	character := &MMCharacter{
		Name: "TestKnight",
		Skills: map[SkillType]*Skill{
			SkillLeather: {Mastery: MasteryNovice},
		},
		Equipment: make(map[items.EquipSlot]items.Item),
	}

	helmet := items.CreateItemFromYAML("leather_helmet")
	ring := items.CreateItemFromYAML("magic_ring")
	character.EquipItem(helmet)
	character.EquipItem(ring)

	got, ok := character.UnequipItem(items.SlotHelmet)
	if !ok {
		t.Fatalf("unequip helmet failed")
	}
	if got.Name != "Leather Helmet" {
		t.Errorf("unequipped item: got %q, want Leather Helmet", got.Name)
	}
	if _, stillThere := character.Equipment[items.SlotHelmet]; stillThere {
		t.Errorf("helmet slot should be empty after unequip")
	}
	if _, present := character.Equipment[items.SlotRing1]; !present {
		t.Errorf("ring should remain equipped after unrelated unequip")
	}
}

func TestEquipRejectsConsumableAndEmptyUnequip(t *testing.T) {
	character := &MMCharacter{
		Name:      "TestKnight",
		Equipment: make(map[items.EquipSlot]items.Item),
	}

	potion := items.CreateItemFromYAML("health_potion")
	if _, _, ok := character.EquipItem(potion); ok {
		t.Errorf("EquipItem should reject consumable (health_potion)")
	}
	if _, ok := character.UnequipItem(items.SlotHelmet); ok {
		t.Errorf("UnequipItem should fail on an empty slot")
	}
}
