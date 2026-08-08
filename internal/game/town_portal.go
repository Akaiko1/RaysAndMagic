package game

import (
	"fmt"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"

	"ugataima/internal/character"
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

// sortedTownPortalDestinations returns the Town Portal destination list in
// stable order.
func (g *MMGame) sortedTownPortalDestinations() []string {
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

// townPortalTeleport moves the party to the chosen map and lands them at its
// arrival point.
func (g *MMGame) townPortalTeleport(mapKey string) {
	g.townPortalPickerOpen = false
	if g.gameLoop == nil || g.gameLoop.inputHandler == nil {
		return
	}
	g.gameLoop.inputHandler.switchToMap(mapKey)
	if world.GlobalWorldManager == nil || world.GlobalWorldManager.CurrentMapKey != mapKey || g.world == nil {
		return
	}
	// Every arrival must complete through finishMapArrival - it re-registers the
	// player's collision entity and autosaves; a raw camera write would leave
	// collisions/projectiles resolving against the previous map's position.
	x, y, ok := g.townPortalArrivalPoint(mapKey)
	if !ok {
		// Nothing authored to arrive at: still finish, so collision + autosave
		// stay coherent.
		g.gameLoop.inputHandler.finishMapArrival(g.camera.X, g.camera.Y, g.camera.Angle)
		return
	}
	g.gameLoop.inputHandler.finishMapArrival(x, y, g.camera.Angle)
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

// drawTownPortalPickerPopup is the Town Portal destination chooser: one row
// per visited destination. Reuses the shared member-picker overlay with indices
// into the destination list.
func (ui *UISystem) drawTownPortalPickerPopup(screen *ebiten.Image) {
	g := ui.game
	dests := g.sortedTownPortalDestinations()
	if len(dests) == 0 {
		g.townPortalPickerOpen = false
		return
	}
	rows := make([]int, len(dests))
	for i := range dests {
		rows[i] = i
	}
	ui.drawMemberPickerPopup(screen, "Town Portal", "Choose a destination.", 360, rows,
		func(idx int) string {
			return fmt.Sprintf("%d) %s", idx+1, g.townPortalDestinationLabel(dests[idx]))
		},
		func(idx int) {
			g.townPortalTeleport(dests[idx])
		},
		g.cancelTownPortalPicker, ui.topModalLayer() == modalLayerTownPortal)
}

// townPortalDestinationLabel renders a map key as a picker row label: the map's
// own name, and the ANCHOR's name when one speaks for it ("Elvish Forest - The
// Wandering Wyvern"). The flag is generic, so the label must not assume the
// anchor is an inn.
func (g *MMGame) townPortalDestinationLabel(mapKey string) string {
	name := humanizeKey(mapKey)
	if world.GlobalWorldManager != nil {
		if mc := world.GlobalWorldManager.MapConfigs[mapKey]; mc != nil && mc.Name != "" {
			name = mc.Name
		}
	}
	if anchor := g.townPortalAnchor(mapKey); anchor != nil && anchor.Name != "" {
		return fmt.Sprintf("%s - %s", name, anchor.Name)
	}
	return name
}
