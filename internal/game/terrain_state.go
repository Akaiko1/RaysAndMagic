package game

import "ugataima/internal/world"

// TerrainChange uses logical map coordinates and tile keys so saved changes
// survive switching between split and stitched world layouts.
type TerrainChange struct {
	Map    string `json:"map"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Before string `json:"before"`
	After  string `json:"after"`
}

func (g *MMGame) recordTerrainChange(tx, ty int, before, after world.TileType3D) {
	wm := world.GlobalWorldManager
	if wm == nil || world.GlobalTileManager == nil {
		return
	}
	key := g.mapKeyAtTile(tx, ty)
	if g.openWorldActive() {
		tx, ty = wm.LocalizeTile(key, tx, ty)
	}
	g.terrainChanges = append(g.terrainChanges, TerrainChange{Map: key, X: tx, Y: ty, Before: world.GlobalTileManager.GetTileKey(before), After: world.GlobalTileManager.GetTileKey(after)})
}

func (g *MMGame) applyTerrainChanges(changes []TerrainChange, undo bool) {
	wm := world.GlobalWorldManager
	if wm == nil || world.GlobalTileManager == nil {
		return
	}
	for i := range changes {
		index := i
		if undo {
			index = len(changes) - 1 - i
		}
		change := changes[index]
		w := wm.WorldByKey(change.Map)
		x, y := wm.ProjectTile(change.Map, change.X, change.Y)
		if w == nil || y < 0 || y >= len(w.Tiles) || x < 0 || x >= len(w.Tiles[y]) {
			continue
		}
		key := change.After
		if undo {
			key = change.Before
		}
		tile, ok := world.GlobalTileManager.GetTileTypeFromKey(key)
		if ok {
			w.Tiles[y][x] = tile
		}
	}
}

func (g *MMGame) restoreTerrainChanges(changes []TerrainChange) {
	g.terrainChanges = append([]TerrainChange(nil), changes...)
	g.applyTerrainChanges(g.terrainChanges, false)
	if g.gameLoop != nil && g.gameLoop.renderer != nil {
		g.gameLoop.renderer.precomputeFloorColorCache()
		g.gameLoop.renderer.buildTransparentSpriteCache()
	}
}
