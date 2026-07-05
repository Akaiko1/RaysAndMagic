package character

// Dual Wielding / zero-weapon-skill class tests - exercise the off-hand
// equip gate, the second-weapon overflow, the "can't unequip your only
// weapon" guard, the IsDualWielding skill requirement, and the universal
// fallbacks (blaster still needs a real weapon skill; cloth stays a true
// no-skill armor category).
//
// TestMain in main_test.go has already wired items.GlobalItemAccessor via
// bridge.SetupItemBridge() before any test in this package runs.

import (
	"testing"

	"ugataima/internal/items"
)

// TestIsDualWieldingRequiresSkill: an off-hand weapon alone must NOT confer
// dual-wield perks - the Dual Wielding skill is also required. Guards the
// save-restore path, which does not revalidate equipment slots, so a crafted
// or legacy save with an off-hand weapon on a skill-less character can't grab
// the extra action / off-hand cooldown for free.
func TestIsDualWieldingRequiresSkill(t *testing.T) {
	c := &MMCharacter{
		Skills:    map[SkillType]*Skill{SkillSword: {Mastery: MasteryNovice}},
		Equipment: make(map[items.EquipSlot]items.Item),
	}
	c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
	c.Equipment[items.SlotOffHand] = items.CreateWeaponFromYAML("magic_dagger")

	if c.IsDualWielding() {
		t.Error("an off-hand weapon without the Dual Wielding skill must not count as dual-wielding")
	}
	c.Skills[SkillDualWielding] = &Skill{Mastery: MasteryNovice}
	if !c.IsDualWielding() {
		t.Error("with the skill AND an off-hand weapon, IsDualWielding should be true")
	}
}

func TestOffHandWeaponRequiresDualWielding(t *testing.T) {
	c := &MMCharacter{
		Skills:    map[SkillType]*Skill{SkillSword: {Mastery: MasteryNovice}},
		Equipment: make(map[items.EquipSlot]items.Item),
	}
	sword := items.CreateWeaponFromYAML("iron_sword")
	if c.ItemFitsSlot(sword, items.SlotOffHand) {
		t.Fatal("a weapon should not fit the off-hand without Dual Wielding")
	}

	c.Skills[SkillDualWielding] = &Skill{Mastery: MasteryNovice}
	if !c.ItemFitsSlot(sword, items.SlotOffHand) {
		t.Fatal("a weapon should fit the off-hand once Dual Wielding is known")
	}
}

// TestEquipItemOverflowsSecondWeaponToOffHand: dropping a second weapon on a
// Dual Wielding character overflows to the empty off-hand, mirroring how a
// second ring overflows to Ring2 instead of replacing Ring1.
func TestEquipItemOverflowsSecondWeaponToOffHand(t *testing.T) {
	c := &MMCharacter{
		Skills: map[SkillType]*Skill{
			SkillSword:        {Mastery: MasteryNovice},
			SkillDagger:       {Mastery: MasteryNovice},
			SkillDualWielding: {Mastery: MasteryNovice},
		},
		Equipment: make(map[items.EquipSlot]items.Item),
	}
	sword := items.CreateWeaponFromYAML("iron_sword")
	dagger := items.CreateWeaponFromYAML("magic_dagger")

	if _, _, ok := c.EquipItem(sword); !ok {
		t.Fatalf("first weapon should equip into the main hand")
	}
	if _, hadPrev, ok := c.EquipItem(dagger); !ok || hadPrev {
		t.Fatalf("second weapon should overflow to the empty off-hand, not replace the main hand (ok=%v hadPrev=%v)", ok, hadPrev)
	}
	if got := c.Equipment[items.SlotMainHand]; got.Name != sword.Name {
		t.Errorf("main hand should still hold %q, got %q", sword.Name, got.Name)
	}
	if got := c.Equipment[items.SlotOffHand]; got.Name != dagger.Name {
		t.Errorf("off hand should hold %q, got %q", dagger.Name, got.Name)
	}
	if !c.IsDualWielding() {
		t.Error("IsDualWielding should be true once a real weapon occupies the off-hand")
	}

	// A THIRD weapon has nowhere to overflow to - EquipItem's slot resolution
	// always starts from SlotMainHand, so it replaces the main hand (matches
	// how a third ring replaces Ring1 once both fingers are full).
	axe := items.CreateWeaponFromYAML("iron_sword")
	if _, hadPrev, ok := c.EquipItem(axe); !ok || !hadPrev {
		t.Errorf("third weapon should replace the main hand once both hands are full (ok=%v hadPrev=%v)", ok, hadPrev)
	}
}

// TestUnequipGuardBlocksOnlyAZeroWeaponSkillCharacter: a character whose only
// weapon skill is Martial Arts (a Monk) can never equip anything else, so
// removing their fists must be refused. A character with a real weapon skill
// (Arms Master) can always unequip freely.
func TestUnequipGuardBlocksOnlyAZeroWeaponSkillCharacter(t *testing.T) {
	monk := &MMCharacter{
		Skills:    map[SkillType]*Skill{SkillMartialArts: {Mastery: MasteryNovice}},
		Equipment: make(map[items.EquipSlot]items.Item),
	}
	fists := items.CreateWeaponFromYAML("monk_fists")
	monk.Equipment[items.SlotMainHand] = fists
	if _, ok := monk.UnequipItem(items.SlotMainHand); ok {
		t.Error("unequipping a zero-other-weapon-skill character's only weapon should be refused")
	}
	if _, still := monk.Equipment[items.SlotMainHand]; !still {
		t.Error("fists should remain equipped after the refused unequip")
	}

	armsMaster := &MMCharacter{
		Skills:    map[SkillType]*Skill{SkillSword: {Mastery: MasteryNovice}},
		Equipment: make(map[items.EquipSlot]items.Item),
	}
	sword := items.CreateWeaponFromYAML("iron_sword")
	armsMaster.Equipment[items.SlotMainHand] = sword
	if _, ok := armsMaster.UnequipItem(items.SlotMainHand); !ok {
		t.Error("a character with a real weapon skill should be able to unequip their main hand")
	}
}

// TestUniversalFallbacks: blaster still requires at least one real weapon
// skill, while cloth remains a true no-skill armor category (robes are allowed
// for Monk-like classes; leather/chain/plate/shield stay skill-gated).
func TestUniversalFallbacks(t *testing.T) {
	weaponless := &MMCharacter{Skills: map[SkillType]*Skill{}, Equipment: make(map[items.EquipSlot]items.Item)}
	if weaponless.CanEquipWeaponByName("Alien Blaster") {
		t.Error("a character with NO weapon skill at all should not get the universal blaster pass")
	}
	if !weaponless.CanEquipArmor(items.CreateItemFromYAML("wizard_robe")) {
		t.Error("cloth should remain wearable without armor skills")
	}

	trained := &MMCharacter{
		Skills: map[SkillType]*Skill{
			SkillDagger:  {Mastery: MasteryNovice},
			SkillLeather: {Mastery: MasteryNovice},
		},
		Equipment: make(map[items.EquipSlot]items.Item),
	}
	if !trained.CanEquipWeaponByName("Alien Blaster") {
		t.Error("a character with a real weapon skill should still get the universal blaster pass")
	}
	if !trained.CanEquipArmor(items.CreateItemFromYAML("wizard_robe")) {
		t.Error("a character with a real armor skill should still get the universal cloth pass")
	}
}
