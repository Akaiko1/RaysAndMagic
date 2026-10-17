package game

import (
	"fmt"
	"image"
	"image/color"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// townPortalAnchor returns the NPC on this map region that is authored as a
// recall anchor (town_portal), or nil. THE one question behind both halves of
// the mechanic: whether the map is a destination, and where the party lands on
// it. Authored, never inferred from renting rooms - the old rule read dialogue
// shape, so a rest tucked inside an info branch quietly made a map recallable.
func (g *MMGame) townPortalAnchor(mapKey string) *character.NPC {
	// The map's OWN world, not the one the party stands in: the picker labels
	// every destination from wherever it is cast, so asking g.world would drop
	// the anchor's name from every row while the party is in a dungeon.
	w := g.worldByKey(mapKey)
	if w == nil {
		return nil
	}
	for _, npc := range w.NPCs {
		// The unified world holds every merged map's NPCs - only an anchor
		// standing in THIS region speaks for it.
		if npc != nil && npc.TownPortal && g.npcOnMapRegion(npc, mapKey) {
			return npc
		}
	}
	return nil
}

// registerVisitedTownPortalDestination records the current map as a Town Portal
// destination: either the map itself is authored as one (a town with no inn) or
// an anchor NPC stands on it. Called on every map entry (including game start).
func (g *MMGame) registerVisitedTownPortalDestination() {
	if g.world == nil || world.GlobalWorldManager == nil {
		return
	}
	mapKey := world.GlobalWorldManager.CurrentMapKey
	mapConfig := world.GlobalWorldManager.MapConfigs[mapKey]
	flagged := mapConfig != nil && mapConfig.TownPortalDestination
	if !flagged && g.townPortalAnchor(mapKey) == nil {
		return
	}
	if g.visitedTavernMaps == nil {
		g.visitedTavernMaps = map[string]bool{}
	}
	g.visitedTavernMaps[mapKey] = true
}

// townPortalRow is one picker row: its stable ID, the map it lands on, the
// place's name and the anchor that speaks for it ("" when none does).
type townPortalRow struct{ id, mapKey, place, anchor string }

// townPortalRows lists the Town Portal destinations in stable order. A row ID
// is a saved map unlock or a guild entrance from guildPortalDestinations.
func (g *MMGame) townPortalRows() []townPortalRow {
	guilds := g.guildPortalDestinations()
	ids := g.sortedVisitedPortalMaps()
	for id := range guilds {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	rows := make([]townPortalRow, len(ids))
	for i, id := range ids {
		rows[i].id, rows[i].mapKey = id, id
		rows[i].place, rows[i].anchor = g.townPortalPlace(id)
		if dest, ok := guilds[id]; ok {
			rows[i].mapKey, rows[i].place, rows[i].anchor = dest.mapKey, mapDisplayName(dest.mapKey), dest.npc.Name
		}
	}
	return rows
}

// townPortalRequest resolves a row to its transition before anything is paid:
// a guild entrance lands beside its guild, a map at its arrival point.
func (g *MMGame) townPortalRequest(id string) (mapTransition, error) {
	if dest, ok := g.guildPortalDestinations()[id]; ok {
		return g.guildPortalRequest(dest)
	}
	if g.worldByKey(id) == nil {
		return mapTransition{}, fmt.Errorf("%s", uitext.Text("portal.unavailable"))
	}
	return mapTransition{mapKey: id, arrival: mapArrivalTownPortal}, nil
}

// Only ordinary map unlocks are saved. Guild entrances derive from membership
// and current placements, so removed entrances cannot linger in a saved list.
func (g *MMGame) sortedVisitedPortalMaps() []string {
	keys := make([]string, 0, len(g.visitedTavernMaps))
	for k, ok := range g.visitedTavernMaps {
		if ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// townPortalArrivalPoint decides WHERE the party lands on a destination: at the
// anchor NPC's door if that region has one, otherwise on the region's own '+'
// start tile. A map flag only says the map is recallable - it never decides the
// landing spot.
func (g *MMGame) townPortalArrivalPoint(mapKey string) (float64, float64, bool) {
	// The DESTINATION's world throughout - the answer must not depend on where
	// the party happens to be standing when it is asked.
	w := g.worldByKey(mapKey)
	if npc := g.townPortalAnchor(mapKey); npc != nil {
		if x, y, ok := nearestWalkableNeighbor(w, float64(g.config.GetTileSize()), npc.X, npc.Y); ok {
			return x, y, true
		}
		return npc.X, npc.Y, true // anchor boxed in (content bug): its own tile
	}
	if world.GlobalWorldManager != nil {
		// A merged region arrives at ITS '+', not the unified world's anchor.
		if x, y, ok := world.GlobalWorldManager.OpenWorldRegionStart(mapKey); ok {
			return x, y, true
		}
	}
	if w != nil && w.StartX >= 0 && w.StartY >= 0 {
		x, y := w.GetStartingPosition()
		return x, y, true
	}
	return 0, 0, false
}

// confirmTownPortal casts the portal the picker was opened for: the caster
// pays its SP, action and cooldown as for any cast, then the party goes.
func (g *MMGame) confirmTownPortal(dest string) {
	caster, id := g.townPortalCaster, g.townPortalSpell
	g.cancelTownPortalPicker()
	idx := g.combat.findCharacterIndex(caster)
	if caster == nil || !g.canSpendCombatAction(idx) {
		return
	}
	if _, err := g.townPortalRequest(dest); err != nil {
		g.AddCombatMessage(err.Error())
		return
	}
	def, err := spells.GetSpellDefinitionByID(id)
	if err != nil {
		return
	}
	g.townPortalConfirming = caster
	cast := g.commitPickedSpell(caster, id, def)
	g.townPortalConfirming = nil
	if !cast {
		return
	}
	g.menuOpen = false
	g.townPortalTeleport(dest)
}

// townPortalTeleport moves the party to the chosen destination and lands them
// at its arrival point.
func (g *MMGame) townPortalTeleport(dest string) {
	g.townPortalPickerOpen = false
	request, err := g.townPortalRequest(dest)
	if err != nil {
		g.AddCombatMessage(err.Error())
		return
	}
	if err := g.transitionToMap(request); err != nil {
		g.AddCombatMessage("Town Portal failed: " + err.Error())
		return
	}
	g.AddCombatMessage("The portal closes behind the party.")
}

// nearestWalkableNeighbor finds the closest walkable tile center adjacent to
// the given position ON THE GIVEN WORLD (4-neighborhood first, then diagonals).
// The world is a parameter, not g.world: the caller asks about a destination
// map, which is usually not the one the party is standing on.
func nearestWalkableNeighbor(w *world.World3D, tileSize, px, py float64) (float64, float64, bool) {
	if w == nil {
		return 0, 0, false
	}
	tx, ty := TileIndex(px, tileSize), TileIndex(py, tileSize)
	offsets := [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
	for _, o := range offsets {
		nx, ny := tx+o[0], ty+o[1]
		// Terrain-only walkability: a placement must never trust the world's
		// transient Fly flag (a just-switched-to map may still carry a stale
		// flyActive from a previous visit, making walls read as passable and
		// seating the party inside one).
		if !w.IsTileBlockingTerrainAt(nx, ny) {
			return (float64(nx) + 0.5) * tileSize, (float64(ny) + 0.5) * tileSize, true
		}
	}
	return 0, 0, false
}

// townPortalLayout is the Town Portal chooser, shared by drawing and clicks.
// Rows name the place over its anchor; a list taller than the screen allows
// scrolls, by wheel or by dragging its scrollbar.
type townPortalLayout struct {
	panel, icon, list layoutRect
	contentH          int
}

const (
	townPortalW      = 440
	townPortalTop    = 64
	townPortalRowH   = 38
	townPortalPitch  = 42
	townPortalFooter = 40
	townPortalBarGap = 6 // between the rows and the scrollbar track
	townPortalTrackW = 3

	townPortalHint       = "Click a destination | Esc: cancel"
	townPortalScrollHint = "Click a destination | Wheel: scroll | Esc: cancel"
)

func makeTownPortalLayout(rows, screenW, screenH int) townPortalLayout {
	l := townPortalLayout{contentH: rows*townPortalPitch - (townPortalPitch - townPortalRowH)}
	listH := min(l.contentH, screenH-32-townPortalTop-townPortalFooter)
	l.panel = centeredRect(screenW, screenH, townPortalW, townPortalTop+listH+townPortalFooter)
	l.icon = layoutRect{l.panel.x + 16, l.panel.y + 14, 28, 28}
	l.list = layoutRect{l.panel.x + 16, l.panel.y + townPortalTop, l.panel.w - 32, listH}
	if l.scrolls() {
		l.list.w -= townPortalBarGap + townPortalTrackW + scrollbarGrabRight
	}
	return l
}

func (l townPortalLayout) scrolls() bool { return l.contentH > l.list.h }

// track is the scrollbar beside the rows.
func (l townPortalLayout) track() layoutRect {
	return layoutRect{l.list.right() + townPortalBarGap, l.list.y, townPortalTrackW, l.list.h}
}

// scroll clamps an offset to the list.
func (l townPortalLayout) scroll(offset int) int {
	return max(0, min(offset, l.contentH-l.list.h))
}

func (l townPortalLayout) row(i, offset int) layoutRect {
	return layoutRect{l.list.x, l.list.y + i*townPortalPitch - offset, l.list.w, townPortalRowH}
}

// drawTownPortalPickerPopup is the Town Portal destination chooser: the
// spell, who pays for it, and one row per known destination.
func (ui *UISystem) drawTownPortalPickerPopup(screen *ebiten.Image) {
	g := ui.game
	if g.allySpellSelecting {
		ui.drawAllySpellTargeting(screen)
		return
	}
	if g.identifySelecting {
		ui.drawIdentifyTargeting(screen)
		return
	}
	if g.identifiedMonster != nil {
		ui.drawIdentifyMonster(screen)
		return
	}
	if g.beaconPicker {
		ui.drawBeaconPicker(screen)
		return
	}
	dests := g.townPortalRows()
	if len(dests) == 0 {
		g.cancelTownPortalPicker()
		return
	}
	live := ui.topModalLayer() == modalLayerTownPortal
	sw, sh := g.config.GetScreenWidth(), g.config.GetScreenHeight()
	l := makeTownPortalLayout(len(dests), sw, sh)
	p := l.panel
	drawFilledRect(screen, 0, 0, sw, sh, color.RGBA{0, 0, 0, 140})
	ui.drawThemeFrame(screen, frameGold, p.x, p.y, p.w, p.h)

	ui.drawInterfaceIcon(screen, spellTooltipIconName(g.townPortalSpell), l.icon.x, l.icon.y, l.icon.w, l.icon.h)
	textX := l.icon.right() + 10
	close := statPopupCloseRect(p)
	drawUITextColored(screen, "Town Portal", textX, p.y+12, gameplayInk.heading)
	if c := g.townPortalCaster; c != nil {
		paid := c.Name
		if def, err := spells.GetSpellDefinitionByID(g.townPortalSpell); err == nil {
			paid = fmt.Sprintf("%s pays %d SP", c.Name, c.MeditatedSPCost(def.SpellPointsCost))
		}
		drawUITextColored(screen, clipUIText(paid, close.x-8-textX), textX, p.y+30, gameplayInk.muted)
	}
	drawFilledRect(screen, p.x+16, p.y+townPortalTop-10, p.w-32, 1, gameplayInk.edge)

	here := ""
	if world.GlobalWorldManager != nil {
		here = world.GlobalWorldManager.CurrentMapKey
	}
	offset := l.scroll(g.townPortalScroll)
	if l.scrolls() {
		ui.onWheelOver(l.list, live, func(dir int) {
			g.townPortalScroll = l.scroll(offset + dir*townPortalPitch)
		})
		ui.drawScrollbar(screen, "town_portal", l.track(), offset, l.contentH, live, func(v int) {
			g.townPortalScroll = v
		})
	}
	mx, my := uiCursorPosition()
	if dst, view, ok := uiListClip(screen, l.list); ok {
		for i, d := range dests {
			r := l.row(i, offset)
			hit := image.Rect(r.x, r.y, r.right(), r.bottom()).Intersect(view)
			if hit.Empty() {
				continue
			}
			hover := live && image.Pt(mx, my).In(hit)
			ui.drawTownPortalRow(dst, r, d, d.mapKey == here, hover)
			id := d.id
			ui.onDisplayedInput(uiCommandClick, layoutRect{hit.Min.X, hit.Min.Y, hit.Dx(), hit.Dy()}, func() {
				if live && g.consumeLeftClickIn(hit.Min.X, hit.Min.Y, hit.Max.X, hit.Max.Y) {
					g.confirmTownPortal(id)
				}
			})
		}
	}
	hint := townPortalHint
	if l.scrolls() {
		hint = townPortalScrollHint
	}
	drawUITextColored(screen, clipUIText(hint, p.w-32), p.x+16, p.bottom()-townPortalFooter+14, gameplayInk.muted)
	ui.drawPopupCloseButton(screen, close.x, close.y, close.w, live, g.cancelTownPortalPicker)
}

// drawTownPortalRow is one destination: the place over its anchor, and where
// the party stands now.
func (ui *UISystem) drawTownPortalRow(dst *ebiten.Image, r layoutRect, d townPortalRow, here, hover bool) {
	if hover {
		ui.drawButtonFrame(dst, r.x, r.y, r.w, r.h, true)
	}
	right := r.right() - 10
	if here {
		const tag = "You are here"
		drawRightUIText(dst, tag, right, r.y+(r.h-uiTextCharHeight)/2, gameplayInk.muted)
		right -= uiTextWidth(tag) + 12
	}
	ink := gameplayInk.text
	if hover {
		ink = gameplayInk.heading
	}
	x := r.x + 10
	if d.anchor == "" {
		drawUITextColored(dst, clipUIText(d.place, right-x), x, r.y+(r.h-uiTextCharHeight)/2, ink)
		return
	}
	y := r.y + (r.h-2*uiTextCharHeight-2)/2
	drawUITextColored(dst, clipUIText(d.place, right-x), x, y, ink)
	drawUITextColored(dst, clipUIText(d.anchor, right-x), x, y+uiTextCharHeight+2, gameplayInk.muted)
}

// townPortalPlace names a destination map and the ANCHOR that speaks for it
// ("Elvish Forest", "The Wandering Wyvern"). The flag is generic, so the
// anchor may be an inn, a shrine or a gate house; "" when the map has none.
func (g *MMGame) townPortalPlace(mapKey string) (place, anchor string) {
	if npc := g.townPortalAnchor(mapKey); npc != nil {
		anchor = npc.Name
	}
	return mapDisplayName(mapKey), anchor
}
