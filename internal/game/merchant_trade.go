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

// merchantPriceKind is how a stock entry is paid for.
type merchantPriceKind uint8

const (
	priceGold   merchantPriceKind = iota // coin, Merchant-skill discount
	priceArena                           // arena points, flat
	priceItem                            // an item currency, flat, maybe a gold surcharge
	priceBarter                          // goods for goods
	priceReward                          // a caravan reward, free once the caravan is unlocked
)

// merchantQuote is what a stock entry costs at the open shop: the ONE
// dispatch on how it is paid for, behind the purse limit, the purchase and
// every price label.
type merchantQuote struct {
	entry *character.MerchantStockItem
	kind  merchantPriceKind
	unit  int    // per-unit price in the entry's own currency (gold after the discount)
	item  string // currency item name (priceItem)
}

func (g *MMGame) quoteMerchantEntry(entry *character.MerchantStockItem) merchantQuote {
	q := merchantQuote{entry: entry, unit: entry.Cost}
	currency := entry.EffectiveCurrency(g.dialogNPC.Currency)
	name, isItem := currencyItemName(currency)
	switch {
	case entry.RewardKey != "":
		q.kind, q.unit = priceReward, 0
	case len(entry.Barter) > 0:
		q.kind = priceBarter
	case isItem:
		q.kind, q.item = priceItem, name
	case currency == character.CurrencyArenaPoints:
		q.kind = priceArena
	default:
		q.kind, q.unit = priceGold, g.merchantBuyPrice(entry.Cost)
	}
	return q
}

// affordable is how many units the purse covers, stock aside.
func (q merchantQuote) affordable(g *MMGame) int {
	switch q.kind {
	case priceReward:
		return g.ecology.Stock[q.entry.RewardKey]
	case priceBarter:
		n := maxUnlimitedShelfUnits
		for _, c := range q.entry.Barter {
			n = min(n, divideBudget(g.party.CountItemsByName(c.Item.Name), c.Count))
		}
		return n
	case priceItem:
		n := divideBudget(g.party.CountItemsByName(q.item), q.unit)
		if q.entry.GoldCost > 0 {
			n = min(n, divideBudget(g.party.Gold, q.entry.GoldCost))
		}
		return n
	case priceArena:
		return divideBudget(g.party.ArenaPoints, q.unit)
	}
	return divideBudget(g.party.Gold, q.unit)
}

// label is what n units cost, in the entry's own currency.
func (q merchantQuote) label(n int) string {
	switch q.kind {
	case priceReward:
		return uitext.Text("caravan.free")
	case priceBarter:
		return barterPriceLabel(q.entry, n)
	case priceItem:
		if q.entry.GoldCost > 0 {
			return fmt.Sprintf("%d %s + %d g", q.unit*n, q.item, q.entry.GoldCost*n)
		}
		return fmt.Sprintf("%d %s", q.unit*n, q.item)
	case priceArena:
		return fmt.Sprintf("%d ap", q.unit*n)
	}
	return fmt.Sprintf("%d g", q.unit*n)
}

// shelfLabel is the price line under a stock cell: haggled gold, an item
// count (plus compacted coins), flat arena points, or the caravan's take.
// A barter cell draws its goods instead (drawBarterPriceLine).
func (q merchantQuote) shelfLabel() string {
	switch q.kind {
	case priceReward:
		return uitext.Text("caravan.take")
	case priceItem:
		if q.entry.GoldCost > 0 {
			// Two currencies on one narrow line: compact the coins.
			return uitext.Text("dialog.stack_additional_cost", q.unit, compactCoinAmount(q.entry.GoldCost))
		}
		return fmt.Sprintf("x%d", q.unit)
	case priceArena:
		return uitext.Text("dialog.arena_price_short", q.unit)
	}
	return uitext.Text("dialog.gold_price_short", q.unit)
}

// merchantMaxUnits is how many units of an entry the party could buy right now:
// the smaller of the remaining stock and what the purse can pay for.
func (g *MMGame) merchantMaxUnits(entry *character.MerchantStockItem) int {
	if entry == nil || !entry.InStock() {
		return 0
	}
	affordable := g.quoteMerchantEntry(entry).affordable(g)
	if entry.Quantity > 0 {
		affordable = min(affordable, entry.Quantity)
	} else {
		// An unlimited shelf with a rich purse must not prefill thousands -
		// the picker exists to PREVENT oversized accidents.
		affordable = min(affordable, maxUnlimitedShelfUnits)
	}
	return max(0, affordable)
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
		g.AddCombatMessage(uitext.Text("combat.that_item_is_sold_out"))
		return false
	}
	if entry.Quantity > 0 && n > entry.Quantity {
		g.AddCombatMessage(uitext.Text("combat.only_left", entry.Quantity, entry.Item.Name))
		return false
	}
	// Arena-points merchants trade at flat prices in the victory currency; gold
	// merchants keep the Merchant-skill discount. A per-entry currency_item
	// (Scalewright) overrides the shop currency and may add a gold surcharge.
	q := g.quoteMerchantEntry(entry)
	switch q.kind {
	case priceReward:
		if n > g.ecology.Stock[entry.RewardKey] {
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
	case priceBarter:
		return g.barterMerchantUnits(entry, n)
	case priceItem:
		goldCost := entry.GoldCost * n
		if !g.canAfford(goldCost) {
			g.AddCombatMessage(uitext.Text("combat.need_gold_on_top_of_the", goldCost, q.item, entry.Item.Name))
			return false
		}
		if !g.party.RemoveItemsByName(q.item, q.unit*n) {
			g.AddCombatMessage(uitext.Text("combat.need_s_to_trade_for", q.unit*n, q.item, entry.Item.Name))
			return false
		}
		g.spendGold(goldCost)
		g.takeMerchantUnits(entry, n)
		key, _ := character.CurrencyItemKey(entry.EffectiveCurrency(g.dialogNPC.Currency))
		g.recordProfileItemTrade(key, q.item, q.unit*n)
		if goldCost > 0 {
			g.AddCombatMessage(uitext.Text("combat.traded_s_and_gold_for", q.unit*n, q.item, goldCost, merchantUnitsLabel(entry, n)))
		} else {
			g.AddCombatMessage(uitext.Text("combat.traded_s_for", q.unit*n, q.item, merchantUnitsLabel(entry, n)))
		}
		return true
	case priceArena:
		cost := q.unit * n
		if cost > g.party.ArenaPoints {
			g.AddCombatMessage(uitext.Text("combat.need_arena_points_to_buy", cost, entry.Item.Name))
			return false
		}
		g.party.ArenaPoints -= cost
		g.takeMerchantUnits(entry, n)
		g.AddCombatMessage(uitext.Text("combat.bought_for_arena_points", merchantUnitsLabel(entry, n), cost))
		return true
	}
	cost := q.unit * n
	if !g.spendGold(cost) {
		g.AddCombatMessage(uitext.Text("combat.need_gold_to_buy", cost, merchantUnitsLabel(entry, n)))
		return false
	}
	g.takeMerchantUnits(entry, n)
	g.AddCombatMessage(uitext.Text("combat.bought_for_gold", merchantUnitsLabel(entry, n), cost))
	return true
}

// barterMerchantUnits pays n units of a barter price in goods - every good is
// counted before any is taken, so a short payment changes nothing.
func (g *MMGame) barterMerchantUnits(entry *character.MerchantStockItem, n int) bool {
	for _, c := range entry.Barter {
		if need := c.Count * n; g.party.CountItemsByName(c.Item.Name) < need {
			g.AddCombatMessage(uitext.Text("combat.need_to_trade_for", need, c.Item.Name, merchantUnitsLabel(entry, n)))
			return false
		}
	}
	for _, c := range entry.Barter {
		g.party.RemoveItemsByName(c.Item.Name, c.Count*n)
		g.recordProfileItemTrade(c.Key, c.Item.Name, c.Count*n)
	}
	g.takeMerchantUnits(entry, n)
	g.AddCombatMessage(uitext.Text("combat.traded_for", barterPriceLabel(entry, n), merchantUnitsLabel(entry, n)))
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
		g.AddCombatMessage(uitext.Text("combat.this_item_has_no_value"))
		return false
	}
	price := unit * n
	if !g.party.ConsumeUnitsAt(idx, n) {
		return false
	}
	g.awardGold(price)
	if n == 1 {
		g.AddCombatMessage(uitext.Text("combat.sold_for_gold", item.Name, price))
	} else {
		g.AddCombatMessage(uitext.Text("combat.sold_x_for_gold", item.Name, n, price))
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
		if _, key, found := config.ItemDefOf(item); found && buys[key] > 0 {
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
	return g.quoteMerchantEntry(entry).label(n)
}

// merchantStockDragSource captures a shelf cell as a buy-drag source, reusing
// the stash drag machine's shared state (shop bank of stashDragFrom).
func (ui *UISystem) merchantStockDragSource(idx int, r image.Rectangle) {
	if ui.displayedInput.building {
		ui.onDisplayedInput(uiCommandDrag, layoutRectOf(r), func() { ui.merchantStockDragSource(idx, r) })
		return
	}
	g := ui.game
	if !g.stashDrag.armed || g.stashDragFrom >= 0 {
		return
	}
	stock := g.merchantVisibleStock()
	if idx < 0 || idx >= len(stock) || stock[idx] == nil || !stock[idx].InStock() {
		return
	}
	if !ptInRect(g.stashDrag.startX, g.stashDrag.startY, r) {
		return
	}
	ui.beginStashDrag(stashShopDragBase+idx, stock[idx].Item)
}
