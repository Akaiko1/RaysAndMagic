package game

import (
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/items"
)

// itemUse is what using an item does: the ONE classification behind the bag's
// double-click and the quick bar. Each source applies it to its own place.
type itemUse uint8

const (
	itemUseNone itemUse = iota
	itemUseAtlas
	itemUsePromotion
	itemUseDevice
	itemUseConsumable
	itemUseEquip
)

func itemUseOf(it *items.Item) itemUse {
	switch {
	case it.Type == items.ItemQuest && itemOpensMap(it):
		return itemUseAtlas
	case it.Type == items.ItemQuest && it.Attributes["promotes_lich"] > 0:
		return itemUsePromotion
	case it.Type == items.ItemDevice:
		return itemUseDevice
	case it.Type == items.ItemConsumable:
		return itemUseConsumable
	case it.Type == items.ItemWeapon || it.Type == items.ItemArmor || it.Type == items.ItemAccessory:
		return itemUseEquip
	}
	return itemUseNone
}

// useBagItem uses the item at bag index for the selected hero (the bag's
// double-click). Gear is worn; spells never sit in a bag.
func (g *MMGame) useBagItem(index int, owner *character.MMCharacter) {
	bag := g.party.Bag(owner)
	if index < 0 || index >= len(bag.Items()) {
		return
	}
	item := bag.Items()[index]
	hero := g.party.Members[g.selectedChar]
	switch itemUseOf(&item) {
	case itemUseAtlas:
		g.mapOverlayOpen = true
	case itemUsePromotion:
		g.useLichPhylactery(index, bag.Owner)
	case itemUseDevice:
		g.useDeviceFromInventory(index, g.selectedChar, bag.Owner)
	case itemUseConsumable:
		g.UseConsumableFromInventory(index, g.selectedChar, bag.Owner)
	case itemUseEquip:
		if g.equipPartyItemFromInventory(index, g.selectedChar, bag.Owner) {
			g.AddCombatMessage(uitext.Text("combat.equipped", hero.Name, item.Name))
			return
		}
		verb := "use"
		if item.Type == items.ItemArmor || item.Type == items.ItemAccessory {
			verb = "wear"
		}
		g.AddCombatMessage(uitext.Text("combat.cannot", hero.Name, verb, item.Name))
	}
}
