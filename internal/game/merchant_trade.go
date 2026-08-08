package game

import (
	"fmt"
	"image"

	"ugataima/internal/character"
	"ugataima/internal/items"
)

// Merchant trade execution: the ONE body behind every purchase and sale,
// whether it came from a double-click or from a drag-and-drop quantity picker.

// merchantBuysForGold reports whether this merchant may BUY the party's goods.
// Only coin shops do: a trader whose till is arena points or dragon scales has
// nothing to pay with, so its sell grid stays shut (validated at load too).
func merchantBuysForGold(npc *character.NPC) bool {
	return npc != nil && npc.SellAvailable && npc.Currency == ""
}

// merchantEntryPrice is the party-facing unit price of a stock entry in its own
// currency, with the Merchant-skill discount applied to gold shops only.
func (g *MMGame) merchantEntryPrice(entry *character.MerchantStockItem) int {
	if entry == nil {
		return 0
	}
	if entry.EffectiveCurrency(g.dialogNPC.Currency) == "" {
		return g.merchantBuyPrice(entry.Cost)
	}
	return entry.Cost // flat: arena points and item currencies ignore the skill
}

// merchantMaxUnits is how many units of an entry the party could buy right now:
// the smaller of the remaining stock and what the purse can pay for.
func (g *MMGame) merchantMaxUnits(entry *character.MerchantStockItem) int {
	if entry == nil || !entry.InStock() {
		return 0
	}
	affordable := 0
	switch currency := entry.EffectiveCurrency(g.dialogNPC.Currency); {
	case currency == character.CurrencyArenaPoints:
		affordable = divideBudget(g.party.ArenaPoints, entry.Cost)
	default:
		if name, ok := currencyItemName(currency); ok {
			affordable = divideBudget(g.party.CountItemsByName(name), entry.Cost)
			if entry.GoldCost > 0 {
				affordable = min(affordable, divideBudget(g.party.Gold, entry.GoldCost))
			}
			break
		}
		affordable = divideBudget(g.party.Gold, g.merchantEntryPrice(entry))
	}
	if entry.Quantity > 0 {
		affordable = min(affordable, entry.Quantity)
	} else {
		// An unlimited shelf with a rich purse must not prefill thousands -
		// the picker exists to PREVENT oversized accidents.
		affordable = min(affordable, maxUnlimitedShelfUnits)
	}
	return affordable
}

// maxUnlimitedShelfUnits bounds one purchase from an unlimited (or free)
// shelf so the picker can never prefill an unbounded quantity.
const maxUnlimitedShelfUnits = 99

// divideBudget is how many units a budget covers at unitCost. A free unit
// (cost 0) is limited by stock alone, never by the purse.
func divideBudget(budget, unitCost int) int {
	if unitCost <= 0 {
		return maxUnlimitedShelfUnits
	}
	if budget < 0 {
		return 0
	}
	return budget / unitCost
}

// buyMerchantUnits charges n units of a stock entry in its own currency and
// puts them in the bag. Reports whether the purchase went through; refusals
// explain themselves in the combat log and change nothing.
func (g *MMGame) buyMerchantUnits(entry *character.MerchantStockItem, n int) bool {
	if entry == nil || n < 1 {
		return false
	}
	if !entry.InStock() {
		g.AddCombatMessage("That item is sold out.")
		return false
	}
	if entry.Quantity > 0 && n > entry.Quantity {
		g.AddCombatMessage(fmt.Sprintf("Only %d %s left.", entry.Quantity, entry.Item.Name))
		return false
	}
	// Arena-points merchants trade at flat prices in the victory currency; gold
	// merchants keep the Merchant-skill discount. A per-entry currency_item
	// (Scalewright) overrides the shop currency and may add a gold surcharge.
	currency := entry.EffectiveCurrency(g.dialogNPC.Currency)
	if name, ok := currencyItemName(currency); ok {
		goldCost := entry.GoldCost * n
		if goldCost > g.party.Gold {
			g.AddCombatMessage(fmt.Sprintf("Need %d gold on top of the %ss for %s.", goldCost, name, entry.Item.Name))
			return false
		}
		if !g.party.RemoveItemsByName(name, entry.Cost*n) {
			g.AddCombatMessage(fmt.Sprintf("Need %d %ss to trade for %s.", entry.Cost*n, name, entry.Item.Name))
			return false
		}
		g.party.Gold -= goldCost
		g.takeMerchantUnits(entry, n)
		if goldCost > 0 {
			g.AddCombatMessage(fmt.Sprintf("Traded %d %ss and %d gold for %s.", entry.Cost*n, name, goldCost, merchantUnitsLabel(entry, n)))
		} else {
			g.AddCombatMessage(fmt.Sprintf("Traded %d %ss for %s.", entry.Cost*n, name, merchantUnitsLabel(entry, n)))
		}
		return true
	}
	if currency == character.CurrencyArenaPoints {
		cost := entry.Cost * n
		if cost > g.party.ArenaPoints {
			g.AddCombatMessage(fmt.Sprintf("Need %d arena points to buy %s.", cost, entry.Item.Name))
			return false
		}
		g.party.ArenaPoints -= cost
		g.takeMerchantUnits(entry, n)
		g.AddCombatMessage(fmt.Sprintf("Bought %s for %d arena points.", merchantUnitsLabel(entry, n), cost))
		return true
	}
	cost := g.merchantEntryPrice(entry) * n
	if cost > g.party.Gold {
		g.AddCombatMessage(fmt.Sprintf("Need %d gold to buy %s.", cost, merchantUnitsLabel(entry, n)))
		return false
	}
	g.party.Gold -= cost
	g.takeMerchantUnits(entry, n)
	g.AddCombatMessage(fmt.Sprintf("Bought %s for %d gold.", merchantUnitsLabel(entry, n), cost))
	return true
}

// takeMerchantUnits hands n copies to the party and decrements the shelf.
// Each copy leaves the shelf template with a FRESH instance id: two bought bows
// are two physical items, and stash reconciliation identifies gear by that id -
// shared ids would let one deposit delete the other.
func (g *MMGame) takeMerchantUnits(entry *character.MerchantStockItem, n int) {
	for i := 0; i < n; i++ {
		unit := entry.Item
		unit.InstanceID = items.NewInstanceID()
		g.party.AddItem(unit)
		entry.Take()
	}
}

func merchantUnitsLabel(entry *character.MerchantStockItem, n int) string {
	if n == 1 {
		return entry.Item.Name
	}
	return fmt.Sprintf("%s x%d", entry.Item.Name, n)
}

// sellInventoryUnits sells n units of bag entry idx to the open merchant at the
// merchant-skill price. The ONE sale body behind the double-click single-unit
// sale and the drag-to-sell quantity picker.
func (g *MMGame) sellInventoryUnits(idx, n int) bool {
	if !merchantBuysForGold(g.dialogNPC) || idx < 0 || idx >= len(g.party.Inventory) || n < 1 {
		return false
	}
	item := g.party.Inventory[idx]
	base := item.Attributes["value"]
	if base <= 0 {
		g.AddCombatMessage("This item has no value.")
		return false
	}
	price := g.merchantSellPrice(base) * n
	if !g.party.ConsumeUnitsAt(idx, n) {
		return false
	}
	g.awardGold(price)
	if n == 1 {
		g.AddCombatMessage(fmt.Sprintf("Sold %s for %d gold.", item.Name, price))
	} else {
		g.AddCombatMessage(fmt.Sprintf("Sold %s x%d for %d gold.", item.Name, n, price))
	}
	return true
}

// merchantDragOpen reports a merchant dialog whose grids accept a drag - the
// second working surface of the stash drag state machine. Buying by drag works
// at every shop; only the SELL half additionally needs a coin till.
func (g *MMGame) merchantDragOpen() bool {
	// No second "is this a shop" test: both kinds below are only produced for an
	// NPC that has stock, and the kind dispatch is where that is decided.
	if !g.dialogActive || g.dialogNPC == nil {
		return false
	}
	switch g.npcDialogKindFor(g.dialogNPC) {
	case dialogKindMerchant, dialogKindArenaGladiator:
		return true
	default:
		return false
	}
}

// merchantBagHeaderLabel is the header over the party's bag grid. At a shop
// that pays no coin the label carries BOTH facts - the grid is a buy target and
// nothing is bought from the party - and it must fit merchantGridW whole: a
// second line would paint over the first row of icons, and a clipped one loses
// exactly the part that explains the action.
func merchantBagHeaderLabel(npc *character.NPC) string {
	if merchantBuysForGold(npc) {
		return "Your Items"
	}
	return "Bag: drop buys here (no selling)"
}

// merchantTotalPriceLabel renders what n units of an entry cost, in whichever
// currency the entry trades in. Shared by the buy picker and its tooltip.
func merchantTotalPriceLabel(g *MMGame, entry *character.MerchantStockItem, n int) string {
	if entry == nil || n < 1 {
		return "-"
	}
	currency := entry.EffectiveCurrency(g.dialogNPC.Currency)
	if name, ok := currencyItemName(currency); ok {
		if entry.GoldCost > 0 {
			return fmt.Sprintf("%d %s + %d g", entry.Cost*n, name, entry.GoldCost*n)
		}
		return fmt.Sprintf("%d %s", entry.Cost*n, name)
	}
	if currency == character.CurrencyArenaPoints {
		return fmt.Sprintf("%d ap", entry.Cost*n)
	}
	return fmt.Sprintf("%d g", g.merchantEntryPrice(entry)*n)
}

// merchantStockDragSource captures a shelf cell as a buy-drag source, reusing
// the stash drag machine's shared state (shop bank of stashDragFrom).
func (ui *UISystem) merchantStockDragSource(idx int, r image.Rectangle) {
	g := ui.game
	if !g.stashDragArmed || g.stashDragFrom >= 0 {
		return
	}
	stock := g.merchantVisibleStock()
	if idx < 0 || idx >= len(stock) || stock[idx] == nil || !stock[idx].InStock() {
		return
	}
	if !ptInRect(g.stashDragStartX, g.stashDragStartY, r) {
		return
	}
	ui.beginStashDrag(stashShopDragBase+idx, stock[idx].Item)
}
