package game

import (
	"fmt"

	"ugataima/internal/character"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// openWorldActive reports whether the party currently plays on the unified
// open world (the flag is on AND the current world IS the stitched one - a
// dungeon visit from the open world returns false).
func (g *MMGame) openWorldActive() bool {
	wm := world.GlobalWorldManager
	return wm != nil && wm.OpenWorld != nil && g.world == wm.OpenWorld
}

// syncOpenWorldRegion tracks the party's region on the unified world and
// keeps CurrentMapKey - the logical "current map" every per-map system reads
// (sky, biome config, quest scoping, saves) - pointed at it. Seamless: no
// collision or cache rebuilds, the world itself never changes.
func (g *MMGame) syncOpenWorldRegion() {
	if !g.openWorldActive() {
		return
	}
	wm := world.GlobalWorldManager
	ts := g.config.GetTileSize()
	r := wm.OpenWorldRegionAtTile(int(g.camera.X/ts), int(g.camera.Y/ts))
	if r == nil || r.MapKey == wm.CurrentMapKey {
		return
	}
	wm.CurrentMapKey = r.MapKey
	if g.gameLoop != nil && g.gameLoop.renderer != nil {
		// The stitched world does not rebuild renderer caches at a seamless
		// boundary. Warm the new logical map explicitly; renderer residency
		// retains the previous region as well so both sides of the seam render.
		g.gameLoop.renderer.scheduleMapRenderResourcePrewarm(r.MapKey)
	}
	// A region cross IS a map departure: bound undead crumble (XP granted),
	// card allies vanish - same rules as a split-map switch. Charmed monsters
	// are Pacified, not Bound, so they stay put untouched.
	g.crumbleBoundAlliesOnDeparture(g.world)
	g.updateSkyAndGroundColorsFaded()
	g.registerVisitedTownPortalDestination()
	if g.gameLoop != nil && g.gameLoop.ui != nil {
		g.gameLoop.ui.invalidateCompassTileLayer()
	}
	if mc := wm.MapConfigs[r.MapKey]; mc != nil && mc.Name != "" {
		g.AddCombatMessage(fmt.Sprintf("You enter %s.", mc.Name))
	}
}

// mapKeyOnCurrentWorld reports whether state tagged with mapKey lives on the
// world the party is on right now. Replaces raw `key == currentMapKey()`
// checks: on the unified world every merged region shares one world, so a
// chest tagged "desert" must stay visible while the party stands in "forest".
func mapKeyOnCurrentWorld(mapKey string) bool {
	if wm := world.GlobalWorldManager; wm != nil {
		return wm.SameWorldKey(mapKey, wm.CurrentMapKey)
	}
	return true
}

// questKillMapKey attributes a kill to a map for quest scoping: the region
// the monster died in on the unified world (a projectile fired across a seam
// must credit the victim's region, not the party's), the current map key
// otherwise.
func (g *MMGame) questKillMapKey(m *monster.Monster3D) string {
	wm := world.GlobalWorldManager
	if m != nil && g.openWorldActive() {
		ts := g.config.GetTileSize()
		if r := wm.OpenWorldRegionAtTile(int(m.X/ts), int(m.Y/ts)); r != nil {
			return r.MapKey
		}
	}
	return currentMapKey()
}

// floorColorForTile resolves the default floor color for one tile of the
// current world: the tile's REGION config on the unified world (a desert tile
// stays sand-colored on the world map while the party walks the forest), the
// current map config otherwise.
func (g *MMGame) floorColorForTile(tx, ty int, fallback [3]int) [3]int {
	wm := world.GlobalWorldManager
	if wm == nil {
		return fallback
	}
	if g.openWorldActive() {
		if mc := wm.MapConfigAtTile(tx, ty); mc != nil {
			return mc.DefaultFloorColor
		}
		return fallback
	}
	if mc := wm.GetCurrentMapConfig(); mc != nil {
		return mc.DefaultFloorColor
	}
	return fallback
}

// npcOnMapRegion reports whether an NPC stands on the given map key's
// territory: inside that region's rect on the unified world, anywhere on a
// split map. Keeps "this map's tavern" logic (Town Portal, destination
// registration) region-accurate when one world holds five maps' NPCs.
func (g *MMGame) npcOnMapRegion(npc *character.NPC, mapKey string) bool {
	wm := world.GlobalWorldManager
	if wm == nil || npc == nil {
		return true
	}
	r := wm.OpenWorldRegionByKey(mapKey)
	if r == nil {
		return true
	}
	ts := g.config.GetTileSize()
	return wm.OpenWorldRegionAtTile(int(npc.X/ts), int(npc.Y/ts)) == r
}

// projectTileToCurrentWorld converts an authored map-local tile position
// (quests, encounter chests) to coordinates on the world that map key
// resolves to. Identity for split maps.
func projectTileToCurrentWorld(mapKey string, tx, ty int) (int, int) {
	if wm := world.GlobalWorldManager; wm != nil {
		return wm.ProjectTile(mapKey, tx, ty)
	}
	return tx, ty
}
