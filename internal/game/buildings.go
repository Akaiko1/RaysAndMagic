package game

import (
	"fmt"
	"math"
	"slices"
	"strings"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/world"
)

// Grid-span buildings (NPC grid_span_tiles >= 2, e.g. the clock tower):
// rendered as ONE grid-aligned facade slab that genuinely occupies N adjacent
// tiles - anchored to the tile grid (not centered-and-spinning on the anchor
// tile like a landmark) - with a solid collision entity per footprint tile.
// The slab's pixel height comes from the walls' own formula scaled by the
// art's aspect, so a 2:1 facade on a 2-tile span stands exactly 1 tile tall.

// buildingPose returns the slab pose for a grid-span NPC: center = midpoint
// of the footprint, yaw = the span axis (slab runs ALONG it).
func (g *MMGame) buildingPose(npc *character.NPC) (x, y, yaw float64, ok bool) {
	if npc == nil || npc.GridSpanTiles < 2 {
		return 0, 0, 0, false
	}
	ts := float64(g.config.GetTileSize())
	cx := (math.Floor(npc.X/ts) + 0.5) * ts
	cy := (math.Floor(npc.Y/ts) + 0.5) * ts
	dx, dy := g.buildingWallOffset(npc)
	cx, cy = cx+dx, cy+dy
	// w|n appear only at runtime, when the open-world stitcher rotates a
	// placed map's authored e|s span (authoring stays e|s, see validation).
	half := float64(npc.GridSpanTiles-1) / 2 * ts
	switch npc.GridSpanDir {
	case "e":
		return cx + half, cy, 0, true
	case "w":
		return cx - half, cy, 0, true
	case "s":
		return cx, cy + half, math.Pi / 2, true
	case "n":
		return cx, cy - half, math.Pi / 2, true
	}
	return 0, 0, 0, false
}

// buildingWallOffset seats a facade against a complete backing wall.
// Neighbor tests follow the runtime span, including stitched-map rotations.
func (g *MMGame) buildingWallOffset(npc *character.NPC) (float64, float64) {
	if !npc.WallBacked || g.world == nil || world.GlobalTileManager == nil {
		return 0, 0
	}
	ts := float64(g.config.GetTileSize())
	directions := [][2]float64{{0, -1}, {0, 1}}
	if npc.GridSpanDir == "s" || npc.GridSpanDir == "n" {
		directions = [][2]float64{{1, 0}, {-1, 0}}
	}
	for _, d := range directions {
		backed := true
		for _, tile := range g.buildingFootprintTiles(npc) {
			if !world.GlobalTileManager.IsSolid(g.world.GetTileAt(tile[0]+d[0]*ts, tile[1]+d[1]*ts)) {
				backed = false
				break
			}
		}
		if backed {
			return d[0] * ts / 2, d[1] * ts / 2
		}
	}
	return 0, 0
}

// buildingFootprintTiles lists the world-space tile centers the span covers.
func (g *MMGame) buildingFootprintTiles(npc *character.NPC) [][2]float64 {
	ts := float64(g.config.GetTileSize())
	cx := (math.Floor(npc.X/ts) + 0.5) * ts
	cy := (math.Floor(npc.Y/ts) + 0.5) * ts
	out := make([][2]float64, 0, npc.GridSpanTiles)
	for i := 0; i < npc.GridSpanTiles; i++ {
		switch npc.GridSpanDir {
		case "e":
			out = append(out, [2]float64{cx + float64(i)*ts, cy})
		case "w":
			out = append(out, [2]float64{cx - float64(i)*ts, cy})
		case "s":
			out = append(out, [2]float64{cx, cy + float64(i)*ts})
		case "n":
			out = append(out, [2]float64{cx, cy - float64(i)*ts})
		}
	}
	return out
}

// buildingOccupiedTiles shares authored building geometry with spatial travel
// and gathering placement. Includes rotated spans in the stitched open world.
func (g *MMGame) buildingOccupiedTiles() map[[2]int]bool {
	occupied := map[[2]int]bool{}
	if g.world == nil {
		return occupied
	}
	ts := float64(g.config.GetTileSize())
	for _, npc := range g.world.NPCs {
		if npc == nil {
			continue
		}
		if npc.GridSpanTiles >= 2 {
			for _, xy := range g.buildingFootprintTiles(npc) {
				occupied[[2]int{TileIndex(xy[0], ts), TileIndex(xy[1], ts)}] = true
			}
		} else if cat, ok := npcCatByName[npc.RenderCategory]; ok && cat == catLandmark {
			occupied[[2]int{TileIndex(npc.X, ts), TileIndex(npc.Y, ts)}] = true
		}
	}
	return occupied
}

// registerBuildingFootprints makes every grid-span building on the current map
// solid: one static entity per footprint tile. Runs on every map arrival
// (clearTransientCombatState drops the previous map's entities first).
func (g *MMGame) registerBuildingFootprints() {
	if g.collisionSystem == nil || g.world == nil {
		return
	}
	if g.buildingEntityIDs == nil {
		g.buildingEntityIDs = make(map[string]bool)
	}
	ts := float64(g.config.GetTileSize())
	for _, npc := range g.world.NPCs {
		if npc == nil {
			continue
		}
		if npc.GridSpanTiles >= 2 {
			for i, c := range g.buildingFootprintTiles(npc) {
				id := fmt.Sprintf("building_%.0f_%.0f_%d", npc.X, npc.Y, i)
				if g.buildingEntityIDs[id] {
					continue
				}
				g.buildingEntityIDs[id] = true
				g.collisionSystem.RegisterEntity(collision.NewEntity(id, c[0], c[1], ts*0.95, ts*0.95, collision.CollisionTypeNPC, true))
			}
			continue
		}
		// A landmark NPC (tower, church, gate, monument) is a building the party
		// must walk around, exactly like a landmark TILE - interaction stays a
		// Space press from the adjacent tile (InteractionDistance is two tiles).
		// Plain map lookup, not resolveNPCRenderCat: content is validated at
		// load, and a hand-built test NPC without a category is just not a
		// landmark rather than a panic.
		if cat, ok := npcCatByName[npc.RenderCategory]; !ok || cat != catLandmark {
			continue
		}
		// A save from before landmarks were solid can hold the party inside this
		// box; entity collision has no already-overlapping allowance, so
		// registering it around them would wall them in permanently. The test is
		// the EXACT box intersection the mover uses (plus a 2px float-jitter
		// margin), not a radius: a wider skip left a landmark passable for the
		// whole visit from a save 0.7-0.99 tiles away. Map SWITCHES register
		// against the old map's coordinates - refreshLandmarkCollision re-runs
		// this at the final arrival position.
		landmarkBox := collision.NewBoundingBox(npc.X, npc.Y, ts*0.95+2, ts*0.95+2)
		partyBox := collision.NewBoundingBox(g.camera.X, g.camera.Y, partyCollisionBoxSize, partyCollisionBoxSize)
		if landmarkBox.Intersects(partyBox) {
			continue
		}
		id := fmt.Sprintf("landmark_%.0f_%.0f", npc.X, npc.Y)
		if g.buildingEntityIDs[id] {
			continue
		}
		g.buildingEntityIDs[id] = true
		g.collisionSystem.RegisterEntity(collision.NewEntity(id, npc.X, npc.Y, ts*0.95, ts*0.95, collision.CollisionTypeNPC, true))
	}
}

// refreshLandmarkCollision re-derives landmark solidity from the party's FINAL
// position. Map switches run registerMapStaticCollision before finishMapArrival
// places the party, so the on-tile skip above judged the OLD map's coordinates:
// it could leave a destination landmark passable, or - worse - wall the party in
// when the arrival point lands on a landmark tile.
func (g *MMGame) refreshLandmarkCollision() {
	if g.collisionSystem == nil {
		return
	}
	for id := range g.buildingEntityIDs {
		if strings.HasPrefix(id, "landmark_") {
			g.collisionSystem.UnregisterEntity(id)
			delete(g.buildingEntityIDs, id)
		}
	}
	g.registerBuildingFootprints()
}

// clearBuildingEntities unregisters the map's building footprints (map switch).
func (g *MMGame) clearBuildingEntities() {
	if g.collisionSystem != nil {
		for id := range g.buildingEntityIDs {
			g.collisionSystem.UnregisterEntity(id)
		}
	}
	g.buildingEntityIDs = nil
}

// ValidateNPCCommerce fails fast on a bad merchant currency or grid-span
// authoring: currency must be gold (""), arena_points, or item:<known item
// key>; a grid span needs 2..4 tiles and an e|s direction. A grid-span facade
// is fixed in its grid pose, so it cannot also author spin settings. Visual
// size ownership is validated by ValidateNPCVisualSizes.
func ValidateNPCCommerce(npcs map[string]*character.NPCData) error {
	for key, npc := range npcs {
		if npc == nil {
			continue
		}
		switch npc.Currency {
		case "", character.CurrencyArenaPoints:
		default:
			itemKey, ok := character.CurrencyItemKey(npc.Currency)
			if !ok {
				return fmt.Errorf("NPC %q has unknown currency %q (valid: \"\", arena_points, item:<key>)", key, npc.Currency)
			}
			if _, found := config.GetItemDefinition(itemKey); !found {
				return fmt.Errorf("NPC %q currency names unknown item %q", key, itemKey)
			}
			// Item-currency payment is RemoveItemsByName(name, cost): cost 0
			// "succeeds" after removing nothing, i.e. a free item. Fail fast on
			// the authoring typo instead.
			for _, it := range npc.Inventory {
				if it != nil && it.Cost <= 0 {
					return fmt.Errorf("NPC %q sells %q for cost %d - item-currency stock needs cost > 0", key, it.Key, it.Cost)
				}
			}
		}
		for _, entry := range npc.Inventory {
			if entry == nil {
				continue
			}
			if err := entry.SourceError(); err != nil {
				return fmt.Errorf("NPC %q stock: %w", key, err)
			}
		}
		// Buying from the party is a gold-shop service. An item-priced shop's bag
		// shows every carried stack (hero quick slots too) as payment, so its
		// cells do not index party.Inventory and could not be sold from.
		if npc.SellAvailable {
			if npc.Currency != "" {
				return fmt.Errorf("NPC %q has sell_available but trades in %q; only gold shops buy from the party", key, npc.Currency)
			}
			for _, it := range npc.Inventory {
				if it != nil && it.CurrencyItem != "" {
					return fmt.Errorf("NPC %q has sell_available but prices %q in an item; only gold-priced shops buy from the party", key, it.Key)
				}
			}
		}
		// Per-entry item currency (Scalewright): the key must exist, the count
		// must be positive, and a gold surcharge can't be negative.
		for _, it := range npc.Inventory {
			if it == nil || it.CurrencyItem == "" {
				if it != nil && it.GoldCost != 0 {
					return fmt.Errorf("NPC %q entry %q has gold_cost without currency_item", key, it.Key)
				}
				continue
			}
			if _, found := config.GetItemDefinition(it.CurrencyItem); !found {
				return fmt.Errorf("NPC %q entry %q prices in unknown item %q", key, it.Key, it.CurrencyItem)
			}
			if it.Cost <= 0 {
				return fmt.Errorf("NPC %q entry %q needs cost > 0 for currency_item pricing", key, it.Key)
			}
			if it.GoldCost < 0 {
				return fmt.Errorf("NPC %q entry %q has negative gold_cost", key, it.Key)
			}
		}
		// A special offer is part of the sale service: only a coin shop pays it.
		if len(npc.Buys) > 0 && (!npc.SellAvailable || npc.Currency != "") {
			return fmt.Errorf("NPC %q has buys but does not buy from the party for gold", key)
		}
		for itemKey, price := range npc.Buys {
			if _, found := config.GetItemDefinition(itemKey); !found || price <= 0 {
				return fmt.Errorf("NPC %q buys %q for %d - needs a known item key and a positive price", key, itemKey, price)
			}
		}
		// Barter replaces the price outright: no currency, coin or authored cost
		// may ride along, and the goods must exist.
		for _, it := range npc.Inventory {
			if it == nil || len(it.Barter) == 0 {
				continue
			}
			if npc.Currency != "" || it.CurrencyItem != "" || it.GoldCost != 0 || it.Cost != 0 {
				return fmt.Errorf("NPC %q entry %q mixes barter with another price", key, it.Key)
			}
			if len(it.Barter) > merchantBarterGoodsMax {
				return fmt.Errorf("NPC %q entry %q barters %d goods; a price line shows at most %d", key, it.Key, len(it.Barter), merchantBarterGoodsMax)
			}
			for itemKey, n := range it.Barter {
				if _, found := config.GetItemDefinition(itemKey); !found || n <= 0 {
					return fmt.Errorf("NPC %q entry %q barters %d of %q - needs a known item key and a positive count", key, it.Key, n, itemKey)
				}
			}
		}
		// A stock tab trades one way: wholly for goods (an exchange shelf, which
		// buys nothing from the party) or wholly for its price. A shop that buys
		// from the party therefore needs a priced shelf to sell at.
		barterTabs, pricedTabs := map[string]bool{}, map[string]bool{}
		for _, it := range npc.Inventory {
			if it != nil && len(it.Barter) > 0 {
				barterTabs[it.Tab] = true
			} else if it != nil {
				pricedTabs[it.Tab] = true
			}
		}
		if npc.StockWeaponsRarity != "" {
			pricedTabs[""] = true // the generated weapon rack is untabbed coin stock
		}
		for _, tab := range sortedMapKeys(barterTabs) {
			if pricedTabs[tab] {
				return fmt.Errorf("NPC %q tab %q mixes barter and priced stock - an exchange tab trades for goods only", key, tab)
			}
		}
		if npc.SellAvailable && len(barterTabs) > 0 && len(pricedTabs) == 0 {
			return fmt.Errorf("NPC %q buys from the party but every shelf is barter, where nothing is sold - add a priced tab or drop sell_available", key)
		}
		if len(npc.Inventory) > 0 {
			tabbed, untabbed := 0, 0
			for _, it := range npc.Inventory {
				if it == nil {
					continue
				}
				if it.Tab == "" {
					untabbed++
				} else {
					tabbed++
				}
			}
			if tabbed > 0 && untabbed > 0 {
				return fmt.Errorf("NPC %q mixes tabbed and untabbed shop stock (%d/%d) - tag every entry or none", key, tabbed, untabbed)
			}
		}
		if npc.GridSpanTiles != 0 {
			if npc.GridSpanTiles < 2 || npc.GridSpanTiles > 4 {
				return fmt.Errorf("NPC %q grid_span_tiles %d out of range (2..4)", key, npc.GridSpanTiles)
			}
			if npc.GridSpanDir != "e" && npc.GridSpanDir != "s" {
				return fmt.Errorf("NPC %q grid_span_tiles needs grid_span_dir e|s, got %q", key, npc.GridSpanDir)
			}
			if npc.NoSpin {
				return fmt.Errorf("NPC %q grid_span_tiles is fixed by its grid pose; omit no_spin", key)
			}
		}
	}
	return nil
}

// validateNPCCastBuffs fails fast when a paid cast_buff service names a buff
// the party has no registry entry for, or when its top-level service catalog
// cannot fit in the dialog. Buff IDs are validated in nested choices too.
func (g *MMGame) validateNPCCastBuffs() error {
	if character.NPCConfigInstance == nil {
		return nil
	}
	maxRows := buffServiceMaxRows(0, 0, npcDialogWidth, npcDialogHeight)
	for key, npc := range character.NPCConfigInstance.NPCs {
		if npc == nil || npc.Dialogue == nil {
			continue
		}
		if count := len(buffServiceChoicesFromDialogue(npc.Dialogue)); count > maxRows {
			return fmt.Errorf("npc %q: %d cast_buff service rows exceed dialog capacity %d", key, count, maxRows)
		}
		if err := npc.Dialogue.WalkChoices(func(c *character.NPCDialogueChoice) error {
			if c.Action == "cast_buff" && !g.isTimedBuffID(c.Buff) {
				return fmt.Errorf("npc %q: cast_buff names unknown party buff %q", key, c.Buff)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// currencyItemName resolves the display name of an item-backed currency.
func currencyItemName(currency string) (string, bool) {
	key, ok := character.CurrencyItemKey(currency)
	if !ok {
		return "", false
	}
	if def, found := config.GetItemDefinition(key); found && def != nil {
		return def.Name, true
	}
	return key, true
}

// rallyAggroedAlarms is the alarm-bell pass (serial, after the parallel
// monster update). Its source must itself see the party within its own authored
// alert radius; only the bell's subsequent sound may cross walls. It wakes up
// to its authored number of calm neighbours once per life. A bell woken by
// another bell joins the fight but consumes its own ring, preventing relay
// chains. Woken monsters remember the alarm as provocation and pursue within
// the shared hit leash; changing mode cannot extend that pursuit.
func (g *MMGame) rallyAggroedAlarms() {
	if g == nil || g.world == nil || g.config == nil || g.camera == nil {
		return
	}
	ts := float64(g.config.GetTileSize())
	for _, m := range g.world.Monsters {
		if m == nil || !m.IsAlive() || m.RallyDone || m.RallyOnAggroTiles <= 0 || !m.IsEngagingPlayer ||
			!m.TargetsParty() ||
			!m.SeesPlayerWithinAlertRadius(g.collisionSystem, g.camera.X, g.camera.Y) {
			continue
		}
		m.RallyDone = true
		r := m.RallyOnAggroTiles * ts
		woken := 0
		for _, o := range g.world.Monsters {
			if m.RallyMaxTargets > 0 && woken >= m.RallyMaxTargets {
				break
			}
			// PassiveUntilAttacked keeps its authored contract: the bell never
			// force-hostiles a monster that only fights when struck (WasAttacked
			// remembers provocation, so an alarm would change future sight behavior). The shared normal
			// eligibility gate also excludes controlled, redirected, inert, and
			// already-engaged targets.
			if o == m || o == nil || o.PassiveUntilAttacked {
				continue
			}
			if math.Hypot(o.X-m.X, o.Y-m.Y) > r {
				continue
			}

			if !o.CanStartPlayerAggro() || math.Hypot(o.X-g.camera.X, o.Y-g.camera.Y) > g.config.MonsterAI.Pursuit.MaxRadiusTiles*ts {
				continue
			}
			o.WasAttacked = true
			o.BeginPlayerEngagement()
			// Reuse the persisted one-ring marker for a bell roused by this one.
			// It remains hostile, but cannot relay an alarm now or after save/load.
			if o.RallyOnAggroTiles > 0 {
				o.RallyDone = true
			}
			woken++
		}
		if woken > 0 {
			g.AddCombatMessage(uitext.Text("combat.rings_out_horrors_answer_the_bell", m.Name, woken))
		}
	}
}

// maybeRespawnMapMonsters rewinds a respawn_days map's roster on arrival: if
// enough calendar days passed since the last spawn, the authored monsters
// return in full (the clock tower is a farming zone). Runs BEFORE the new
// world's monsters register with collision.
func (g *MMGame) maybeRespawnMapMonsters() {
	if g.world == nil || world.GlobalWorldManager == nil {
		return
	}
	mc := world.GlobalWorldManager.MapConfigs[world.GlobalWorldManager.CurrentMapKey]
	if mc == nil || mc.RespawnDays <= 0 {
		return
	}
	unstamped := g.world.LastRespawnDay == 0
	if !unstamped && g.currentCalendarDay()-g.world.LastRespawnDay < mc.RespawnDays {
		return
	}
	// Unknown-age rosters adopt the current authored spawns on first entry.
	g.respawnAuthoredMonsters(g.world)
	g.world.LastRespawnDay = g.currentCalendarDay()
	if !unstamped {
		g.AddCombatMessage(uitext.Text("combat.the_tower_mechanism_grinds_its_horrors"))
	}
}

// merchantShopTabs returns the dialog's shop tab labels (authored stock
// order, tabs with nothing on offer left out), or nil for a classic
// single-grid merchant.
func (g *MMGame) merchantShopTabs() []string {
	return g.merchantStockCache().tabs
}

// merchantStockOffered reports whether an entry is on the shelf: its quest
// gate, if any, is settled the way a gated dialogue choice is.
func (g *MMGame) merchantStockOffered(m *character.MerchantStockItem) bool {
	return m != nil && g.questChainStepDone(m.RequiresQuest)
}

// merchantStockTab is the merchantShopTabs index the open dialog shows. A
// shop+talk dialog lists its stock tabs after Talk; outside a stock tab it
// clamps to the first, like a classic shop.
func (g *MMGame) merchantStockTab() int {
	tab := g.dialogTab - g.merchantStockTabOffset()
	if tab < 0 || tab >= len(g.merchantShopTabs()) {
		return 0
	}
	return tab
}

// merchantStockTabOffset is the dialogTab of the first stock tab: 1 after
// Talk on a shop+talk dialog, 0 on a classic shop.
func (g *MMGame) merchantStockTabOffset() int {
	if g.npcDialogKindFor(g.dialogNPC) == dialogKindArenaGladiator {
		return 1
	}
	return 0
}

// merchantStockCache refreshes the memo of the open merchant's stock. Stock
// only changes by reassigning the slice (restock, caravan, load) or in place
// inside an entry, which no filter reads, so the slice identity keys it; the
// gated entries' gate states key the tabs, which open and close with them.
func (g *MMGame) merchantStockCache() *merchantStockView {
	c := &g.merchantStockView
	npc := g.dialogNPC
	if npc == nil {
		return &merchantStockView{}
	}
	stock := npc.MerchantStock
	var base **character.MerchantStockItem
	if len(stock) > 0 {
		base = &stock[0]
	}
	if c.npc != npc || c.base != base || c.n != len(stock) {
		sameNPC, oldTabs := c.npc == npc, c.tabs
		*c = merchantStockView{npc: npc, base: base, n: len(stock), barterTabs: map[string]bool{}}
		for _, m := range stock {
			if m == nil {
				continue
			}
			if m.RequiresQuest != "" {
				c.gated = append(c.gated, m)
			}
			if len(m.Barter) > 0 {
				c.barterTabs[m.Tab] = true
			}
		}
		c.open = make([]bool, len(c.gated))
		g.refreshMerchantTabs(c, sameNPC, oldTabs)
		return c
	}
	for i, m := range c.gated {
		if g.merchantStockOffered(m) != c.open[i] {
			g.refreshMerchantTabs(c, true, c.tabs)
			break
		}
	}
	return c
}

// refreshMerchantTabs re-reads the gates and rebuilds the offered tabs. While
// the same dialog stays open, the selection follows its tab by name.
func (g *MMGame) refreshMerchantTabs(c *merchantStockView, sameNPC bool, oldTabs []string) {
	c.hidden = 0
	for i, m := range c.gated {
		c.open[i] = g.merchantStockOffered(m)
		if !c.open[i] {
			c.hidden++
		}
	}
	offered := c.npc.MerchantStock
	if c.hidden > 0 {
		offered = slices.DeleteFunc(slices.Clone(offered), func(m *character.MerchantStockItem) bool { return !g.merchantStockOffered(m) })
	}
	c.tabs, c.view = character.MerchantTabs(offered), nil
	if sameNPC && !slices.Equal(oldTabs, c.tabs) {
		g.keepMerchantTabSelection(oldTabs, c.tabs)
	}
}

// keepMerchantTabSelection moves dialogTab to the tab it showed before the
// stock tabs changed under the open dialog. A tab that vanished hands the
// dialog back to its first tab (Talk on a shop+talk dialog).
func (g *MMGame) keepMerchantTabSelection(oldTabs, tabs []string) {
	labels := func(shop []string) []string {
		if g.merchantStockTabOffset() == 1 {
			return serviceTabLabels(g.dialogNPC, shop)
		}
		return shop
	}
	before, after := labels(oldTabs), labels(tabs)
	if g.dialogTab < 0 || g.dialogTab >= len(before) {
		g.dialogTab = 0
		return
	}
	g.dialogTab = max(0, slices.Index(after, before[g.dialogTab]))
}

// merchantVisibleStock is the stock slice the buy grid shows RIGHT NOW: the
// active tab's offered entries for a tabbed shop, everything otherwise. The
// single source for both the draw pass and the click handler - their indices
// must agree or clicks buy the wrong item.
func (g *MMGame) merchantVisibleStock() []*character.MerchantStockItem {
	c := g.merchantStockCache()
	if c.npc == nil {
		return nil
	}
	stock := c.npc.MerchantStock
	if len(c.tabs) == 0 && c.hidden == 0 && g.merchantBuyCategory == inventoryTabAll {
		return stock
	}
	tab := g.merchantStockTab()
	if c.view == nil || c.tab != tab || c.category != g.merchantBuyCategory {
		// A fresh slice: a caller may still be ranging over the previous view.
		view := make([]*character.MerchantStockItem, 0, len(stock))
		for _, m := range stock {
			if g.merchantStockOffered(m) && (len(c.tabs) == 0 || m.Tab == c.tabs[tab]) &&
				inventoryTabShows(g.merchantBuyCategory, m.Item) {
				view = append(view, m)
			}
		}
		c.view, c.tab, c.category = view, tab, g.merchantBuyCategory
	}
	return c.view
}

// merchantStockView memoizes merchantVisibleStock for one stock slice.
type merchantStockView struct {
	npc           *character.NPC
	base          **character.MerchantStockItem
	n, hidden     int
	gated         []*character.MerchantStockItem // entries behind a quest gate
	open          []bool                         // their gate states at the last refresh
	barterTabs    map[string]bool                // tabs priced in goods (whole tabs, validated at load)
	tabs          []string
	tab, category int
	view          []*character.MerchantStockItem
}
