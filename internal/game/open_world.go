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
	r := wm.OpenWorldRegionAtTile(TileIndex(g.camera.X, ts), TileIndex(g.camera.Y, ts))
	if r == nil || r.MapKey == wm.CurrentMapKey {
		return
	}
	g.recordProfileTravel(wm.CurrentMapKey, r.MapKey)
	wm.CurrentMapKey = r.MapKey
	if g.gameLoop != nil && g.gameLoop.renderer != nil {
		// The stitched world does not rebuild renderer caches at a seamless
		// boundary. Warm the new logical map explicitly; renderer residency
		// retains the previous region as well so both sides of the seam render.
		g.gameLoop.renderer.scheduleMapRenderResourcePrewarm(r.MapKey)
	}
	// A region cross is NOT a departure: the world never changes, so every ally
	// (summons and bound undead alike) simply walks on with the party. Allies
	// crumble only on a real world switch - switchToMap, e.g. into a dungeon.
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

// mapKeyAtTile resolves a runtime tile to its logical map. On split maps this
// is the current map; on the unified world the tile's region is authoritative.
func (g *MMGame) mapKeyAtTile(tx, ty int) string {
	return g.mapKeyAtWorldTile(g.world, tx, ty)
}

// mapKeyAtWorldTile is mapKeyAtTile for any loaded world.
func (g *MMGame) mapKeyAtWorldTile(w *world.World3D, tx, ty int) string {
	if wm := world.GlobalWorldManager; wm != nil {
		if key := wm.MapKeyAt(w, tx, ty); key != "" {
			return key
		}
	}
	return currentMapKey()
}

// monsterHomeMap is the map a monster of world w belongs to for target_map
// quests: where it was created, wherever it has wandered since. A monster
// with no recorded home belongs where it stands.
func (g *MMGame) monsterHomeMap(w *world.World3D, m *monster.Monster3D) string {
	if m.HomeMap != "" {
		return m.HomeMap
	}
	ts := g.config.GetTileSize()
	return g.mapKeyAtWorldTile(w, TileIndex(m.X, ts), TileIndex(m.Y, ts))
}

// monsterIsFrom reports whether a monster of world w counts for mapKey's
// target_map quests: its home is mapKey, or, with no recorded home, it stands
// on mapKey.
func (g *MMGame) monsterIsFrom(w *world.World3D, m *monster.Monster3D, mapKey string) bool {
	if m.HomeMap != "" {
		return m.HomeMap == mapKey
	}
	wm := world.GlobalWorldManager
	if wm == nil {
		return true
	}
	if wm.WorldByKey(mapKey) != w {
		return false
	}
	if r := wm.OpenWorldRegionByKey(mapKey); r != nil {
		ts := g.config.GetTileSize()
		return wm.OpenWorldRegionAtTile(TileIndex(m.X, ts), TileIndex(m.Y, ts)) == r
	}
	return true
}

// stampMonsterHome records a new monster's home from the tile it appears on,
// unless its spawner already named one.
func (g *MMGame) stampMonsterHome(w *world.World3D, m *monster.Monster3D) {
	if m.HomeMap == "" {
		m.HomeMap = g.monsterHomeMap(w, m)
	}
}

// questKillMapKey attributes a kill to a map for quest scoping: the victim's
// home, so a forest spider that chased the party into the desert still counts
// for the forest lake.
func (g *MMGame) questKillMapKey(m *monster.Monster3D) string {
	if m != nil {
		return g.monsterHomeMap(g.world, m)
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
	if g == nil || g.config == nil {
		return true
	}
	return npcOnMapRegionWith(wm, npc, mapKey, g.config.GetTileSize())
}

// npcOnMapRegionWith is the world-explicit form used by boot validators before
// an MMGame is necessarily available. This is the one region-membership rule
// for placed NPC capabilities in a stitched world.
func npcOnMapRegionWith(wm *world.WorldManager, npc *character.NPC, mapKey string, tileSize float64) bool {
	if wm == nil || npc == nil {
		return true
	}
	r := wm.OpenWorldRegionByKey(mapKey)
	if r == nil {
		return true
	}
	return wm.OpenWorldRegionAtTile(TileIndex(npc.X, tileSize), TileIndex(npc.Y, tileSize)) == r
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
