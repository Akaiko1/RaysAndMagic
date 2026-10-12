package game

import (
	"fmt"
	"image"
	"strings"
	uitext "ugataima/assets/text"

	"ugataima/internal/character"
	"ugataima/internal/config"
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

// merchantSellsHere reports whether the open shop view buys the party's goods
// for coin: a coin shop that buys from the party, except on a stock tab priced
// wholly in goods, which trades like an exchange merchant and buys nothing.
// The one predicate behind the bag's sale prices, drags and glow, the sale
// click and the sale itself.
func (g *MMGame) merchantSellsHere() bool {
	return merchantBuysForGold(g.dialogNPC) && !g.merchantBarterShelfOpen()
}

// merchantBarterShelfOpen reports a shown stock tab (or untabbed shop) priced
// in goods. A tab is wholly barter or wholly priced (validated at load), so
// the authored tab decides, whichever of its entries a quest gate shows now.
func (g *MMGame) merchantBarterShelfOpen() bool {
	c := g.merchantStockCache()
	if c.npc == nil {
		return false
	}
	tab := ""
	if len(c.tabs) > 0 {
		tab = c.tabs[g.merchantStockTab()]
	}
	return c.barterTabs[tab]
}

// merchantEntryPrice is the party-facing unit price of a stock entry in its own
// currency, with the Merchant-skill discount applied to gold shops only.
func (g *MMGame) merchantEntryPrice(entry *character.MerchantStockItem) int {
	if entry == nil || (entry.RewardKey != "" && g.dialogNPC != nil && g.dialogNPC.FreeGoods) {
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
	if entry.RewardKey != "" && g.dialogNPC != nil && g.dialogNPC.FreeGoods {
		return max(0, min(entry.Quantity, g.ecology.Stock[entry.RewardKey]))
	}
	affordable := 0
	switch currency := entry.EffectiveCurrency(g.dialogNPC.Currency); {
	case len(entry.Barter) > 0:
		affordable = maxUnlimitedShelfUnits
		for _, c := range entry.Barter {
			affordable = min(affordable, divideBudget(g.party.CountItemsByName(c.Item.Name), c.Count))
		}
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
	if entry.RewardKey != "" {
		if !g.dialogNPC.FreeGoods || n > g.ecology.Stock[entry.RewardKey] {
			return false
		}
		g.takeMerchantUnits(entry, n)
		g.ecology.Stock[entry.RewardKey] -= n
		if g.ecology.Stock[entry.RewardKey] == 0 {
			delete(g.ecology.Stock, entry.RewardKey)
		}
		g.syncCaravanStock()
		g.AddCombatMessage(uitext.Text("caravan.collected", merchantUnitsLabel(entry, n)))
		return true
	}
	if len(entry.Barter) > 0 {
		return g.barterMerchantUnits(entry, n)
	}
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
		key, _ := character.CurrencyItemKey(currency)
		g.recordProfileItemTrade(key, name, entry.Cost*n)
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

// barterMerchantUnits pays n units of a barter price in goods - every good is
// counted before any is taken, so a short payment changes nothing.
func (g *MMGame) barterMerchantUnits(entry *character.MerchantStockItem, n int) bool {
	for _, c := range entry.Barter {
		if need := c.Count * n; g.party.CountItemsByName(c.Item.Name) < need {
			g.AddCombatMessage(fmt.Sprintf("Need %d %s to trade for %s.", need, c.Item.Name, merchantUnitsLabel(entry, n)))
			return false
		}
	}
	for _, c := range entry.Barter {
		g.party.RemoveItemsByName(c.Item.Name, c.Count*n)
		g.recordProfileItemTrade(c.Key, c.Item.Name, c.Count*n)
	}
	g.takeMerchantUnits(entry, n)
	g.AddCombatMessage(fmt.Sprintf("Traded %s for %s.", barterPriceLabel(entry, n), merchantUnitsLabel(entry, n)))
	return true
}

// barterPriceLabel names the goods n units of a barter entry cost.
func barterPriceLabel(entry *character.MerchantStockItem, n int) string {
	parts := make([]string, 0, len(entry.Barter))
	for _, c := range entry.Barter {
		parts = append(parts, fmt.Sprintf("%d %s", c.Count*n, c.Item.Name))
	}
	return strings.Join(parts, " + ")
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
	if !g.merchantSellsHere() || idx < 0 || idx >= len(g.party.Inventory) || n < 1 {
		return false
	}
	item := g.party.Inventory[idx]
	unit, _ := g.merchantUnitSellPrice(item)
	if unit <= 0 {
		g.AddCombatMessage("This item has no value.")
		return false
	}
	price := unit * n
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

// merchantBuys is the merchant's authored special offers (item key -> gold
// per unit), read from the catalog by NPC key so restored NPCs see current
// content; nil without offers.
func merchantBuys(npc *character.NPC) map[string]int {
	if npc == nil || character.NPCConfigInstance == nil {
		return nil
	}
	if def, ok := character.NPCConfigInstance.GetNPCData(npc.Key); ok && def != nil {
		return def.Buys
	}
	return nil
}

// merchantUnitSellPrice is what the open merchant pays for one unit: its own
// offer for that item (merchantBuys), else the Merchant-skill price on the
// item's value.
func (g *MMGame) merchantUnitSellPrice(item items.Item) (price int, special bool) {
	if buys := merchantBuys(g.dialogNPC); len(buys) > 0 {
		if _, key, found := config.GetItemDefinitionByName(item.Name); found && buys[key] > 0 {
			return buys[key], true
		}
	}
	if base := item.Attributes["value"]; base > 0 {
		return g.merchantSellPrice(base), false
	}
	return 0, false
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
	return g.npcDialogKindFor(g.dialogNPC).drawsShop()
}

// Item-for-item traders can show active heroes' possessions as payment stock.
// Coin-sale indices always continue to address the shared bag only.
func merchantUsesItemCurrency(npc *character.NPC) bool {
	if npc == nil {
		return false
	}
	if _, ok := currencyItemName(npc.Currency); ok {
		return true
	}
	for _, entry := range npc.MerchantStock {
		if entry != nil {
			if _, ok := currencyItemName(entry.EffectiveCurrency(npc.Currency)); ok {
				return true
			}
		}
	}
	return false
}

// A view priced in goods - an item-currency shop or a barter shelf - shows
// every carried stack it can be paid from; such a view never buys from the
// party, so its cells are never sale indices.
func (g *MMGame) merchantBagItems() []items.Item {
	if merchantUsesItemCurrency(g.dialogNPC) || g.merchantBarterShelfOpen() {
		return g.party.CarriedItems()
	}
	return g.party.Inventory
}

// merchantBagViewIndices keeps filtered cells tied to their physical bag
// entries: positions in merchantBagItems(). Sales and drags must never use a
// category-local index as a bag index. Where the party can sell (a gold shop,
// ValidateNPCCommerce) merchantBagItems() is party.Inventory itself.
func (g *MMGame) merchantBagViewIndices() []int {
	return inventoryTabView(g.merchantSellCategory, g.merchantBagItems())
}

func (g *MMGame) setMerchantCategory(buy bool, category int) {
	selected, page := &g.merchantSellCategory, &g.merchantSellPage
	if buy {
		selected, page = &g.merchantBuyCategory, &g.merchantBuyPage
	}
	if !selectCategory(selected, page, category) {
		return
	}
	g.clearStashDrag()
	g.resetDialogClickTracker()
}

// merchantBagHeaderLabel is the header over the party's bag grid. At a shop
// that pays no coin the label carries BOTH facts - the grid is a buy target and
// nothing is bought from the party - and it must fit merchantGridW whole: a
// second line would paint over the first row of icons, and a clipped one loses
// exactly the part that explains the action.
func merchantBagHeaderLabel(npc *character.NPC, sells bool) string {
	if npc != nil && npc.FreeGoods {
		return uitext.Text("caravan.bag_header")
	}
	if sells {
		return "Your Items"
	}
	return "Bag: drop to buy (no selling)"
}

// merchantTotalPriceLabel renders what n units of an entry cost, in whichever
// currency the entry trades in. Shared by the buy picker and its tooltip.
func merchantTotalPriceLabel(g *MMGame, entry *character.MerchantStockItem, n int) string {
	if entry == nil || n < 1 {
		return "-"
	}
	if entry.RewardKey != "" && g.dialogNPC.FreeGoods {
		return uitext.Text("caravan.free")
	}
	if len(entry.Barter) > 0 {
		return barterPriceLabel(entry, n)
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
	if ui.displayedInput.building {
		ui.onDisplayedInput(uiCommandDrag, layoutRect{r.Min.X, r.Min.Y, r.Dx(), r.Dy()}, func() { ui.merchantStockDragSource(idx, r) })
		return
	}
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
