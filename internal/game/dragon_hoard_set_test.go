package game

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

// Some monster drops every Dragon's Hoard piece, and the completed exact-piece set adds its critical chance to weapon and
// spell rolls and to every card that shows them.
func TestDragonHoardSetLootAndCriticalChance(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}
	if prev := monster.MonsterConfig; prev == nil {
		t.Cleanup(func() { monster.MonsterConfig = prev })
		monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	}
	set := config.GetItemSet("dragon_hoard")
	if set == nil || set.BonusCritChance <= 0 || len(set.RequiredPieces) != 2 {
		t.Fatalf("fixture: dragon_hoard must be a two-piece crit set: %+v", set)
	}
	var armorKey, swordKey string
	for _, key := range set.RequiredPieces {
		if _, isWeapon := config.GlobalWeapons.Weapons[key]; isWeapon {
			swordKey = key
		} else {
			armorKey = key
		}
	}
	if armorKey == "" || swordKey == "" {
		t.Fatalf("fixture: dragon_hoard needs one weapon and one armor piece: %v", set.RequiredPieces)
	}

	// The set is completable from one hunt: some monster drops every piece.
	complete := false
	for _, mobKey := range slices.Sorted(maps.Keys(monster.MonsterConfig.Monsters)) {
		dropped := map[string]bool{}
		for _, entry := range config.GetLootTable(mobKey, false) {
			if entry.Chance > 0 && ((entry.Type == "item" && entry.Key == armorKey) || (entry.Type == "weapon" && entry.Key == swordKey)) {
				dropped[entry.Key] = true
			}
		}
		complete = complete || len(dropped) == 2
	}
	if !complete {
		t.Fatalf("no monster drops both %s and %s", armorKey, swordKey)
	}

	armor := items.CreateItemFromYAML(armorKey)
	sword := items.CreateWeaponFromYAML(swordKey)
	if armor.Set != "dragon_hoard" || sword.Set != "dragon_hoard" {
		t.Fatalf("set membership armor=%q sword=%q, want dragon_hoard", armor.Set, sword.Set)
	}

	ch := cs.game.party.Members[0]
	ch.Luck = 0
	delete(ch.Skills, character.SkillSword)
	delete(ch.Skills, character.SkillArmsMaster)
	ch.Equipment = map[items.EquipSlot]items.Item{}
	baseCrit := cs.CalculateWeaponCritChance(sword, ch)
	if want := config.GlobalWeapons.Weapons[swordKey].CritChance; baseCrit != want {
		t.Fatalf("%s base crit = %d, want its authored %d with no luck or skills", sword.Name, baseCrit, want)
	}
	bonus := set.BonusCritChance

	// A duplicate sword must not replace the armor in an exact-piece set.
	ch.Equipment[items.SlotMainHand] = sword
	ch.Equipment[items.SlotOffHand] = items.CreateWeaponFromYAML(swordKey)
	if got := cs.CalculateWeaponCritChance(sword, ch); got != baseCrit {
		t.Fatalf("two %s crit = %d, want %d without %s", sword.Name, got, baseCrit, armor.Name)
	}

	ch.Equipment = map[items.EquipSlot]items.Item{
		items.SlotMainHand: sword,
		items.SlotArmor:    armor,
	}
	weaponCrit := min(100, baseCrit+bonus)
	if got := cs.CalculateWeaponCritChance(sword, ch); got != weaponCrit {
		t.Fatalf("completed %s crit = %d, want %d", set.Name, got, weaponCrit)
	}
	if got := cs.CalculateCriticalChance(ch); got != bonus {
		t.Fatalf("%s spell crit bonus = %d, want %d", set.Name, got, bonus)
	}
	if _, total := cs.RollCriticalChance(0, ch); total != bonus {
		t.Fatalf("%s spell crit total = %d, want %d", set.Name, total, bonus)
	}
	setRow := fmt.Sprintf("Set: +%d%%", bonus)
	bonusRow := fmt.Sprintf("Set bonus: critical chance +%d%%", bonus)
	spellTip := GetSpellTooltip(spells.SpellID("fireball"), ch, cs, true)
	for _, want := range []string{fmt.Sprintf("Chance: %d%%", bonus), setRow} {
		if !strings.Contains(spellTip, want) {
			t.Errorf("Fireball tooltip missing %q:\n%s", want, spellTip)
		}
	}

	weaponTip := GetItemTooltip(sword, ch, cs, true)
	for _, want := range []string{
		fmt.Sprintf("Chance: %d%%", weaponCrit), fmt.Sprintf("Set: %s (2/2 equipped)", set.Name), bonusRow, setRow,
	} {
		if !strings.Contains(weaponTip, want) {
			t.Errorf("%s tooltip missing %q:\n%s", sword.Name, want, weaponTip)
		}
	}
	armorTip := GetItemTooltip(armor, ch, cs, true)
	if !strings.Contains(armorTip, bonusRow) {
		t.Errorf("%s tooltip missing set bonus:\n%s", armor.Name, armorTip)
	}
}
