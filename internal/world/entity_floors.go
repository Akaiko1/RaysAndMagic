package world

import (
	"maps"
	"slices"

	"ugataima/internal/character"
)

// entityFloor remembers derived ground without serializing it. Tile allows an
// explicit terrain edit to override automatic ground until that edit is undone.
type entityFloor struct {
	Tile, Fallback TileType3D
}

func (spawn NPCSpawn) groundTileKey() string {
	if spawn.GroundTile != "" {
		return spawn.GroundTile
	}
	if character.NPCConfigInstance != nil {
		if data, ok := character.NPCConfigInstance.GetNPCData(spawn.NPCKey); ok {
			return data.GroundTile
		}
	}
	return ""
}

// RebuildFloors is shared by the map loader and editor mutations. Overlay the
// authored objects and explicit NPC ground before resolving any automatic cell.
func (md *MapData) RebuildFloors(tm *TileManager, biome string) {
	if tm == nil {
		return
	}
	fallback, _ := tm.GetTileTypeFromLetterForBiome(".", biome)
	previous := md.entityFloors
	md.entityFloors = make(map[[2]int]entityFloor)
	add := func(x, y int) {
		if md.hasCell(x, y) {
			cell, tile := [2]int{x, y}, md.Tiles[y][x]
			old, derived := previous[cell]
			// Only parser placeholders and previously derived ground inherit.
			// An authored bridge or a later terrain edit must remain intact.
			if tile == fallback || tile == TileEmpty || derived && old.Tile == tile {
				md.entityFloors[cell] = entityFloor{Tile: tile, Fallback: fallback}
			}
		}
	}
	for _, spawn := range md.MonsterSpawns {
		if !md.hasCell(spawn.X, spawn.Y) {
			continue
		}
		if tile, ok := tm.GetTileTypeFromKey(spawn.GroundTile); ok {
			md.Tiles[spawn.Y][spawn.X] = tile
			delete(md.entityFloors, [2]int{spawn.X, spawn.Y})
		} else {
			add(spawn.X, spawn.Y)
		}
	}
	for _, spawn := range md.NPCSpawns {
		if !md.hasCell(spawn.X, spawn.Y) {
			continue
		}
		if tile, ok := tm.GetTileTypeFromKey(spawn.groundTileKey()); ok {
			md.Tiles[spawn.Y][spawn.X] = tile
			delete(md.entityFloors, [2]int{spawn.X, spawn.Y})
		} else {
			add(spawn.X, spawn.Y)
		}
	}
	for _, spawn := range md.SpecialTileSpawns {
		if md.hasCell(spawn.X, spawn.Y) {
			md.Tiles[spawn.Y][spawn.X] = spawn.TileType
			delete(md.entityFloors, [2]int{spawn.X, spawn.Y})
		}
	}
	md.Floors = resolveEntityFloors(tm, md.Tiles, md.entityFloors)
}

// ClearNPCGround removes an NPC's stamped ground when its placement is removed
// or moved. Preserve any explicit terrain edit that replaced that stamp.
// It reports whether a ground stamp was cleared.
func (md *MapData) ClearNPCGround(tm *TileManager, spawn NPCSpawn, fallback TileType3D) bool {
	return md.clearGround(tm, spawn.X, spawn.Y, spawn.groundTileKey(), fallback)
}

// ClearMonsterGround is ClearNPCGround for a keyed monster placement.
func (md *MapData) ClearMonsterGround(tm *TileManager, spawn MonsterSpawn, fallback TileType3D) bool {
	return md.clearGround(tm, spawn.X, spawn.Y, spawn.GroundTile, fallback)
}

func (md *MapData) clearGround(tm *TileManager, x, y int, ground string, fallback TileType3D) bool {
	if tm == nil || !md.hasCell(x, y) {
		return false
	}
	cleared := false
	if tile, ok := tm.GetTileTypeFromKey(ground); ok && md.Tiles[y][x] == tile {
		md.Tiles[y][x] = fallback
		cleared = true
	}
	delete(md.entityFloors, [2]int{x, y})
	md.Floors = nil
	return cleared
}

// Clone deep-copies the map with its derived ground, so an editor draft and its
// undo snapshots never share mutable state with the loaded map. Floors is
// replaced wholesale on rebuild, never edited in place, so it may be shared.
func (md *MapData) Clone() *MapData {
	if md == nil {
		return nil
	}
	out := *md
	out.Tiles = make([][]TileType3D, len(md.Tiles))
	for i := range md.Tiles {
		out.Tiles[i] = slices.Clone(md.Tiles[i])
	}
	out.MonsterSpawns = slices.Clone(md.MonsterSpawns)
	out.NPCSpawns = slices.Clone(md.NPCSpawns)
	out.SpecialTileSpawns = slices.Clone(md.SpecialTileSpawns)
	out.entityFloors = maps.Clone(md.entityFloors)
	return &out
}

func (md *MapData) hasCell(x, y int) bool {
	return y >= 0 && y < len(md.Tiles) && x >= 0 && x < len(md.Tiles[y])
}

func resolveEntityFloors(tm *TileManager, tiles [][]TileType3D, entities map[[2]int]entityFloor) FloorResolution {
	pending := make(map[[2]int]bool, len(entities))
	for cell, floor := range entities {
		x, y := cell[0], cell[1]
		if y >= 0 && y < len(tiles) && x >= 0 && x < len(tiles[y]) && tiles[y][x] == floor.Tile {
			pending[cell] = true
		}
	}
	floors := tm.ResolveFloors(tiles, pending)
	for cell := range pending {
		floor := entities[cell]
		x, y := cell[0], cell[1]
		chosen, ok := floors.At(x, y)
		if !ok {
			chosen = floor.Fallback
		}
		tiles[y][x] = chosen
		floor.Tile = chosen
		entities[cell] = floor
	}
	return floors
}

// RebuildInheritedFloors refreshes both appearance and automatic spawn ground
// after map transitions, terrain edits and save restoration. It is derived from
// authored map cells, not live monster positions, so moving actors never repaint.
func (w *World3D) RebuildInheritedFloors() {
	if GlobalTileManager == nil {
		w.floors = nil
		return
	}
	w.floors = resolveEntityFloors(GlobalTileManager, w.Tiles, w.entityFloors)
}

func (w *World3D) InheritedFloorAt(x, y int) (TileType3D, bool) {
	if w.floors == nil {
		w.RebuildInheritedFloors()
	}
	return w.floors.At(x, y)
}
