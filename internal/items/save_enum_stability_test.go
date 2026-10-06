package items

import "testing"

func TestItemTypeValuesAreStableForSaveCompatibility(t *testing.T) {
	for kind, want := range map[ItemType]int{
		ItemWeapon: 0, ItemArmor: 1, ItemAccessory: 2, ItemConsumable: 3, ItemQuest: 4,
		ItemBattleSpell: 5, ItemUtilitySpell: 6, ItemTrinket: 7, ItemTrap: 8, ItemCard: 9,
		ItemThrowable: 10, ItemTechnique: 11, ItemDevice: 12,
	} {
		if int(kind) != want {
			t.Errorf("%s=%d want saved value %d; append new kinds instead of inserting", kind, kind, want)
		}
	}
}
