package character

// Equipment slot routing tests - exercise the real MMCharacter.EquipItem /
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

func TestChronoCapeEquipsAsUniversalCloak(t *testing.T) {
	character := &MMCharacter{
		Name:      "TestMonk",
		Skills:    map[SkillType]*Skill{},
		Equipment: make(map[items.EquipSlot]items.Item),
	}

	cape := items.CreateItemFromYAML("chrono_cape")
	if cape.Type != items.ItemAccessory {
		t.Fatalf("Chrono Cape type = %v, want accessory", cape.Type)
	}
	if _, hadPrevious, ok := character.EquipItem(cape); !ok || hadPrevious {
		t.Fatalf("Chrono Cape should equip in an empty cloak slot, ok=%v hadPrevious=%v", ok, hadPrevious)
	}

	equipped, ok := character.Equipment[items.SlotCloak]
	if !ok || equipped.Name != cape.Name {
		t.Fatalf("cloak slot = %#v, want Chrono Cape", equipped)
	}
	if got := equipped.Attributes["armor_class_base"]; got != 4 {
		t.Errorf("Chrono Cape armor_class_base = %d, want 4", got)
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
	//   intellect_scaling_divisor: 6  -> +Intellect/6
	//   personality_scaling_divisor: 8 -> +Personality/8
	magicRing := items.CreateItemFromYAML("magic_ring")
	character.EquipItem(magicRing)

	_, intellect, personality, endurance, _, _, _ := character.GetEffectiveStats()

	wantInt := 30 + (30 / 6) // 35
	wantPer := 25 + (25 / 8) // 28
	wantEnd := 20            // ring has no endurance bonus

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

// TestTwoRingsUseBothSlots: a second ring goes to SlotRing2 instead of
// overwriting the first; a third ring (both fingers full) replaces SlotRing1.
func TestTwoRingsUseBothSlots(t *testing.T) {
	character := &MMCharacter{
		Name:      "TestMage",
		Equipment: make(map[items.EquipSlot]items.Item),
	}

	ring1 := items.CreateItemFromYAML("magic_ring")
	if _, hadPrev, ok := character.EquipItem(ring1); !ok || hadPrev {
		t.Fatalf("first ring should equip with no previous item")
	}
	ring2 := items.CreateItemFromYAML("magic_ring")
	if _, hadPrev, ok := character.EquipItem(ring2); !ok || hadPrev {
		t.Fatalf("second ring should equip into the free SlotRing2, not replace SlotRing1 (hadPrev=%v)", hadPrev)
	}

	if _, ok := character.Equipment[items.SlotRing1]; !ok {
		t.Errorf("SlotRing1 should be occupied")
	}
	if _, ok := character.Equipment[items.SlotRing2]; !ok {
		t.Errorf("SlotRing2 should be occupied by the second ring")
	}

	// Both fingers full -> a third ring overwrites SlotRing1 and returns it.
	ring3 := items.CreateItemFromYAML("magic_ring")
	if _, hadPrev, ok := character.EquipItem(ring3); !ok || !hadPrev {
		t.Errorf("third ring should replace SlotRing1 and return the previous item (ok=%v hadPrev=%v)", ok, hadPrev)
	}
}

// TestMoveRingBetweenFingers covers dragging an already-equipped ring from one
// finger to the other: to an empty finger it relocates; onto an occupied finger
// it swaps. Same-slot and empty-source moves are no-ops.
func TestMoveRingBetweenFingers(t *testing.T) {
	c := &MMCharacter{Name: "TestMage", Equipment: make(map[items.EquipSlot]items.Item)}
	a := items.Item{Name: "Ring A", Type: items.ItemAccessory}
	b := items.Item{Name: "Ring B", Type: items.ItemAccessory}

	// Relocate to an empty finger.
	c.Equipment[items.SlotRing1] = a
	if !c.MoveEquipmentSlot(items.SlotRing1, items.SlotRing2) {
		t.Fatal("move to an empty finger should succeed")
	}
	if _, ok := c.Equipment[items.SlotRing1]; ok {
		t.Error("SlotRing1 should be empty after relocating")
	}
	if got := c.Equipment[items.SlotRing2]; got.Name != "Ring A" {
		t.Errorf("SlotRing2 = %q, want Ring A", got.Name)
	}

	// Move onto an occupied finger swaps the two.
	c.Equipment[items.SlotRing1] = b
	if !c.MoveEquipmentSlot(items.SlotRing1, items.SlotRing2) {
		t.Fatal("move onto an occupied finger should succeed (swap)")
	}
	if c.Equipment[items.SlotRing1].Name != "Ring A" || c.Equipment[items.SlotRing2].Name != "Ring B" {
		t.Errorf("swap failed: r1=%q r2=%q, want Ring A / Ring B",
			c.Equipment[items.SlotRing1].Name, c.Equipment[items.SlotRing2].Name)
	}

	// No-ops.
	if c.MoveEquipmentSlot(items.SlotRing2, items.SlotRing2) {
		t.Error("same-slot move should be a no-op")
	}
	empty := &MMCharacter{Equipment: make(map[items.EquipSlot]items.Item)}
	if empty.MoveEquipmentSlot(items.SlotRing1, items.SlotRing2) {
		t.Error("moving from an empty slot should be a no-op")
	}
}

// TestTwoRingsStackBonuses: both worn rings contribute - magic_ring's
// intellect/personality scaling-divisor bonuses sum across SlotRing1+SlotRing2,
// so two rings double a single ring's bonus (calculateEquipmentBonuses ranges
// the whole Equipment map, not per-slot).
func TestTwoRingsStackBonuses(t *testing.T) {
	newMage := func() *MMCharacter {
		return &MMCharacter{
			Name:        "TestMage",
			Intellect:   30, // /6 -> +5 spell-power per magic_ring
			Personality: 32, // /8 -> +4 per magic_ring
			Equipment:   make(map[items.EquipSlot]items.Item),
		}
	}

	one := newMage()
	one.EquipItem(items.CreateItemFromYAML("magic_ring"))
	_, int1, per1, _, _, _, _ := one.calculateEquipmentBonuses()

	two := newMage()
	two.EquipItem(items.CreateItemFromYAML("magic_ring"))
	two.EquipItem(items.CreateItemFromYAML("magic_ring"))
	_, int2, per2, _, _, _, _ := two.calculateEquipmentBonuses()

	if int1 != 5 || per1 != 4 {
		t.Fatalf("single ring bonus: got int=%d per=%d, want int=5 per=4", int1, per1)
	}
	if int2 != 2*int1 || per2 != 2*per1 {
		t.Errorf("two rings should double the bonus: got int=%d per=%d, want int=%d per=%d",
			int2, per2, 2*int1, 2*per1)
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

func TestEquipItemToSlotRejectsMismatchedSlot(t *testing.T) {
	character := &MMCharacter{
		Name: "TestKnight",
		Skills: map[SkillType]*Skill{
			SkillLeather: {Mastery: MasteryNovice},
		},
		Equipment: make(map[items.EquipSlot]items.Item),
	}

	helmet := items.CreateItemFromYAML("leather_helmet")
	if _, _, ok := character.EquipItemToSlot(helmet, items.SlotBoots); ok {
		t.Errorf("EquipItemToSlot should reject a helmet forced into the boots slot")
	}
	if _, occupied := character.Equipment[items.SlotBoots]; occupied {
		t.Errorf("boots slot must stay empty after the rejected equip")
	}
	ring := items.CreateItemFromYAML("magic_ring")
	if _, _, ok := character.EquipItemToSlot(ring, items.SlotRing2); !ok {
		t.Errorf("EquipItemToSlot should allow a ring on either finger")
	}
}

func TestMoveEquipmentSlotRejectsMismatchedSlot(t *testing.T) {
	character := &MMCharacter{
		Name: "TestKnight",
		Skills: map[SkillType]*Skill{
			SkillLeather: {Mastery: MasteryNovice},
		},
		Equipment: make(map[items.EquipSlot]items.Item),
	}

	helmet := items.CreateItemFromYAML("leather_helmet")
	if _, _, ok := character.EquipItem(helmet); !ok {
		t.Fatalf("setup: equip helmet failed")
	}
	if character.MoveEquipmentSlot(items.SlotHelmet, items.SlotBoots) {
		t.Errorf("MoveEquipmentSlot should refuse moving a helmet onto the boots slot")
	}
	if got, ok := character.Equipment[items.SlotHelmet]; !ok || got.Name != "Leather Helmet" {
		t.Errorf("helmet must stay in its slot after the rejected move")
	}
}
