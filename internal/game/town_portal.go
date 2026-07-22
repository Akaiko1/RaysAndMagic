package game

import (
	"fmt"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"

	"ugataima/internal/character"
	"ugataima/internal/world"
)

// npcOffersTavernRest reports whether an NPC's dialogue tree contains a
// tavern_rest choice - the capability that makes it "a tavern" for Town
// Portal, with no name/key matching.
func npcOffersTavernRest(npc *character.NPC) bool {
	return npcDialogueHasAction(npc, "tavern_rest")
}

// registerVisitedTownPortalDestination records the current map as a Town
// Portal destination if it hosts a tavern or is explicitly marked in
// map_configs.yaml. Called on every map entry (including game start).
func (g *MMGame) registerVisitedTownPortalDestination() {
	if g.world == nil || world.GlobalWorldManager == nil {
		return
	}
	mapKey := world.GlobalWorldManager.CurrentMapKey
	if mapConfig := world.GlobalWorldManager.MapConfigs[mapKey]; mapConfig != nil && mapConfig.TownPortalDestination {
		if g.visitedTavernMaps == nil {
			g.visitedTavernMaps = map[string]bool{}
		}
		g.visitedTavernMaps[mapKey] = true
		return
	}
	for _, npc := range g.world.NPCs {
		// The unified world holds every merged map's NPCs - only a tavern
		// standing in THIS region makes the region a destination.
		if npcOffersTavernRest(npc) && g.npcOnMapRegion(npc, mapKey) {
			if g.visitedTavernMaps == nil {
				g.visitedTavernMaps = map[string]bool{}
			}
			g.visitedTavernMaps[mapKey] = true
			return
		}
	}
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

// townPortalTeleport moves the party to the chosen map. Explicit map
// destinations arrive at the '+' start tile; tavern maps arrive one tile in
// front of their tavern.
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
	if mapConfig := world.GlobalWorldManager.MapConfigs[mapKey]; mapConfig != nil && mapConfig.TownPortalDestination {
		// A merged region arrives at ITS '+', not the unified world's anchor.
		if x, y, ok := world.GlobalWorldManager.OpenWorldRegionStart(mapKey); ok {
			g.gameLoop.inputHandler.finishMapArrival(x, y, g.camera.Angle)
			g.AddCombatMessage("The portal closes behind the party.")
			return
		}
		if g.world.StartX >= 0 && g.world.StartY >= 0 {
			x, y := g.world.GetStartingPosition()
			g.gameLoop.inputHandler.finishMapArrival(x, y, g.camera.Angle)
			g.AddCombatMessage("The portal closes behind the party.")
			return
		}
	}
	for _, npc := range g.world.NPCs {
		// Arrive at the TARGET map's tavern, not the first tavern of the
		// unified world's combined NPC list.
		if !npcOffersTavernRest(npc) || !g.npcOnMapRegion(npc, mapKey) {
			continue
		}
		x, y, ok := g.nearestWalkableNeighbor(npc.X, npc.Y)
		if !ok {
			x, y = npc.X, npc.Y // tavern boxed in (content bug): arrive on its tile
		}
		g.gameLoop.inputHandler.finishMapArrival(x, y, g.camera.Angle)
		g.AddCombatMessage("The portal closes behind the party.")
		return
	}
	// Destination list only holds tavern maps; if the tavern vanished (content
	// edit), still finish the arrival so collision + autosave stay coherent.
	g.gameLoop.inputHandler.finishMapArrival(g.camera.X, g.camera.Y, g.camera.Angle)
}

// nearestWalkableNeighbor finds the closest walkable tile center adjacent to
// the given world position (4-neighborhood first, then diagonals).
func (g *MMGame) nearestWalkableNeighbor(px, py float64) (float64, float64, bool) {
	tileSize := float64(g.config.GetTileSize())
	tx, ty := int(px/tileSize), int(py/tileSize)
	offsets := [][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
	for _, o := range offsets {
		nx, ny := tx+o[0], ty+o[1]
		// Terrain-only walkability: a placement must never trust the world's
		// transient Fly flag (a just-switched-to map may still carry a stale
		// flyActive from a previous visit, making walls read as passable and
		// seating the party inside one).
		if g.world != nil && !g.world.IsTileBlockingTerrainAt(nx, ny) {
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
			return fmt.Sprintf("%d) %s", idx+1, townPortalDestinationLabel(dests[idx]))
		},
		func(idx int) {
			g.townPortalTeleport(dests[idx])
		},
		func() {
			g.townPortalPickerOpen = false
		})
}

// townPortalDestinationLabel renders a map key as a picker row label.
func townPortalDestinationLabel(mapKey string) string {
	if world.GlobalWorldManager != nil {
		if mc := world.GlobalWorldManager.MapConfigs[mapKey]; mc != nil && mc.Name != "" {
			if mc.TownPortalDestination {
				return mc.Name
			}
			return fmt.Sprintf("%s Tavern", mc.Name)
		}
	}
	return fmt.Sprintf("%s Tavern", humanizeKey(mapKey))
}
