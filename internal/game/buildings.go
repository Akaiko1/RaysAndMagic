package game

import (
	"fmt"
	"math"
	"strings"

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
					return fmt.Errorf("NPC %q sells %q for cost %d - item-currency stock needs cost > 0", key, it.Name, it.Cost)
				}
			}
		}
		// Per-entry item currency (Scalewright): the key must exist, the count
		// must be positive, and a gold surcharge can't be negative.
		for _, it := range npc.Inventory {
			if it == nil || it.CurrencyItem == "" {
				if it != nil && it.GoldCost != 0 {
					return fmt.Errorf("NPC %q entry %q has gold_cost without currency_item", key, it.Name)
				}
				continue
			}
			if _, found := config.GetItemDefinition(it.CurrencyItem); !found {
				return fmt.Errorf("NPC %q entry %q prices in unknown item %q", key, it.Name, it.CurrencyItem)
			}
			if it.Cost <= 0 {
				return fmt.Errorf("NPC %q entry %q needs cost > 0 for currency_item pricing", key, it.Name)
			}
			if it.GoldCost < 0 {
				return fmt.Errorf("NPC %q entry %q has negative gold_cost", key, it.Name)
			}
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
// chains. Woken monsters receive the STICKY WasAttacked signal so mode/AI
// transitions cannot lull them back.
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
			// is sticky, so waking it here would be permanent). The shared normal
			// eligibility gate also excludes controlled, redirected, inert, and
			// already-engaged targets.
			if o == m || o == nil || o.PassiveUntilAttacked || !o.CanStartPlayerAggro() {
				continue
			}
			if math.Hypot(o.X-m.X, o.Y-m.Y) > r {
				continue
			}
			o.BeginPlayerEngagement()
			o.WasAttacked = true
			// Reuse the persisted one-ring marker for a bell roused by this one.
			// It remains hostile, but cannot relay an alarm now or after save/load.
			if o.RallyOnAggroTiles > 0 {
				o.RallyDone = true
			}
			woken++
		}
		if woken > 0 {
			g.AddCombatMessage(fmt.Sprintf("%s rings out - %d horrors answer the bell!", m.Name, woken))
		}
	}
}

// maybeRespawnMapMonsters rewinds a respawn_days map's roster on arrival: if
// enough day/night phases passed since the last spawn, the authored monsters
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
	if g.world.LastRespawnDay == 0 {
		// Never stamped: a fresh map's first arrival, or a roster restored from a
		// pre-stamp save whose age is unknown. Rewind to the CURRENT authored
		// roster now (a fresh map rebuilds its identical spawn list - harmless),
		// so old saves pick up re-authored maps on first entry, silently.
		g.world.RespawnAuthoredMonsters()
		g.world.LastRespawnDay = g.dayNightDay + 1 // +1 keeps 0 as the "never stamped" sentinel
		return
	}
	if g.dayNightDay+1-g.world.LastRespawnDay < mc.RespawnDays {
		return
	}
	g.world.RespawnAuthoredMonsters()
	g.world.LastRespawnDay = g.dayNightDay + 1
	g.AddCombatMessage("The tower mechanism grinds - its horrors are wound anew.")
}

// merchantShopTabs returns the dialog's shop tab labels (authored stock
// order), or nil for a classic single-grid merchant.
func (g *MMGame) merchantShopTabs() []string {
	if g.dialogNPC == nil {
		return nil
	}
	return character.MerchantTabs(g.dialogNPC.MerchantStock)
}

// merchantVisibleStock is the stock slice the buy grid shows RIGHT NOW: the
// active tab's entries for a tabbed shop, everything otherwise. The single
// source for both the draw pass and the click handler - their indices must
// agree or clicks buy the wrong item.
func (g *MMGame) merchantVisibleStock() []*character.MerchantStockItem {
	if g.dialogNPC == nil {
		return nil
	}
	tabs := character.MerchantTabs(g.dialogNPC.MerchantStock)
	if len(tabs) == 0 {
		return g.dialogNPC.MerchantStock
	}
	if g.dialogTab < 0 || g.dialogTab >= len(tabs) {
		g.dialogTab = 0
	}
	want := tabs[g.dialogTab]
	out := make([]*character.MerchantStockItem, 0, len(g.dialogNPC.MerchantStock))
	for _, m := range g.dialogNPC.MerchantStock {
		if m != nil && m.Tab == want {
			out = append(out, m)
		}
	}
	return out
}
