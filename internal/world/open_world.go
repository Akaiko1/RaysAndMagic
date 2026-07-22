package world

import (
	"fmt"
	"math"
	"sort"
	"ugataima/internal/config"
)

// OpenWorldKey is the world identity of the unified world in contexts that
// need one key per distinct world (EachWorld, renderer caches). It is never a
// save/config namespace: persistent state stays keyed by the source map keys.
const OpenWorldKey = "open_world"

// OpenWorldRegion is one source map's footprint inside the unified world.
// Region keys double as the logical "current map": config resolution, quest
// scoping and save state all keep working against the source map key. A
// region may be placed rotated/mirrored (Orient); Width/Height are the PLACED
// footprint (swapped for rot90/rot270), LocalWidth/LocalHeight the map's own.
type OpenWorldRegion struct {
	MapKey                  string
	OffsetX, OffsetY        int // tile offset of the placed footprint's origin
	Width, Height           int // placed footprint
	LocalWidth, LocalHeight int
	Orient                  string
	StartX, StartY          int                 // source '+' in unified tiles (-1 if none)
	InitialMonsterKeys      map[string]struct{} // this map's own authored monster kinds (region-scoped loot pools)
}

// --- Placement orientation transforms ---------------------------------------
//
// A placed map may be rotated (clockwise) or mirrored. LOCAL coordinates stay
// canonical everywhere (saves, quests, encounters); these transforms are the
// single bridge between local and placed space.

func owPlacedDims(orient string, w, h int) (int, int) {
	switch orient {
	case "rot90", "rot270":
		return h, w
	}
	return w, h
}

// owXformTile maps a local tile to the placed footprint (before offset).
func owXformTile(orient string, w, h, x, y int) (int, int) {
	switch orient {
	case "rot90":
		return h - 1 - y, x
	case "rot180":
		return w - 1 - x, h - 1 - y
	case "rot270":
		return y, w - 1 - x
	case "mirror_x":
		return w - 1 - x, y
	case "mirror_y":
		return x, h - 1 - y
	}
	return x, y
}

// owXformTileInv maps a placed-footprint tile back to local space.
func owXformTileInv(orient string, w, h, px, py int) (int, int) {
	switch orient {
	case "rot90":
		return py, h - 1 - px
	case "rot180":
		return w - 1 - px, h - 1 - py
	case "rot270":
		return w - 1 - py, px
	case "mirror_x":
		return w - 1 - px, py
	case "mirror_y":
		return px, h - 1 - py
	}
	return px, py
}

// owXformWorld / owXformWorldInv are the continuous (world-coordinate)
// counterparts; W,H are the LOCAL map size in world units.
func owXformWorld(orient string, W, H, x, y float64) (float64, float64) {
	switch orient {
	case "rot90":
		return H - y, x
	case "rot180":
		return W - x, H - y
	case "rot270":
		return y, W - x
	case "mirror_x":
		return W - x, y
	case "mirror_y":
		return x, H - y
	}
	return x, y
}

func owXformWorldInv(orient string, W, H, px, py float64) (float64, float64) {
	switch orient {
	case "rot90":
		return py, H - px
	case "rot180":
		return W - px, H - py
	case "rot270":
		return W - py, px
	case "mirror_x":
		return W - px, py
	case "mirror_y":
		return px, H - py
	}
	return px, py
}

// owXformAngle rotates/mirrors a heading into placed space; owXformAngleInv
// is the inverse (mirrors are self-inverse, rotations negate).
func owXformAngle(orient string, a float64) float64 {
	switch orient {
	case "rot90":
		return a + math.Pi/2
	case "rot180":
		return a + math.Pi
	case "rot270":
		return a - math.Pi/2
	case "mirror_x":
		return math.Pi - a
	case "mirror_y":
		return -a
	}
	return a
}

func owXformAngleInv(orient string, a float64) float64 {
	switch orient {
	case "rot90":
		return a - math.Pi/2
	case "rot270":
		return a + math.Pi/2
	}
	return owXformAngle(orient, a) // rot180 and mirrors are self-inverse
}

// owXformSide maps an opening authored on a LOCAL edge to its placed edge and
// span start, so connection authoring stays in the source map's own terms.
func owXformSide(orient string, w, h int, edge string, at, width int) (string, int) {
	switch orient {
	case "rot90":
		switch edge {
		case "north":
			return "east", at
		case "east":
			return "south", h - width - at
		case "south":
			return "west", at
		default: // west
			return "north", h - width - at
		}
	case "rot180":
		switch edge {
		case "north":
			return "south", w - width - at
		case "south":
			return "north", w - width - at
		case "east":
			return "west", h - width - at
		default:
			return "east", h - width - at
		}
	case "rot270":
		switch edge {
		case "north":
			return "west", w - width - at
		case "west":
			return "south", at
		case "south":
			return "east", w - width - at
		default: // east
			return "north", at
		}
	case "mirror_x":
		switch edge {
		case "north", "south":
			return edge, w - width - at
		case "east":
			return "west", at
		default:
			return "east", at
		}
	case "mirror_y":
		switch edge {
		case "east", "west":
			return edge, h - width - at
		case "north":
			return "south", at
		default:
			return "north", at
		}
	}
	return edge, at
}

// OpeningCells returns the unified-grid cells a connection side's opening
// occupies on this region's border (all pierced wall layers). Exported for
// the map editor's open-world views; the stitcher carves the same cells.
func (r *OpenWorldRegion) OpeningCells(side config.OpenWorldPortalSide, width int) [][2]int {
	edge, at := owXformSide(r.Orient, r.LocalWidth, r.LocalHeight, side.Edge, side.At, width)
	depth := side.Depth
	if depth == 0 {
		depth = 1
	}
	var cells [][2]int
	for i := 0; i < width; i++ {
		for d := 0; d < depth; d++ {
			var x, y int
			switch edge {
			case "north":
				x, y = r.OffsetX+at+i, r.OffsetY+d
			case "south":
				x, y = r.OffsetX+at+i, r.OffsetY+r.Height-1-d
			case "west":
				x, y = r.OffsetX+d, r.OffsetY+at+i
			default: // east
				x, y = r.OffsetX+r.Width-1-d, r.OffsetY+at+i
			}
			cells = append(cells, [2]int{x, y})
		}
	}
	return cells
}

// OpeningInteriorCells returns, per span index, the first cell BEHIND an
// opening's pierced wall - the cell the stitcher guarantees walkable. Used by
// tests to anchor reachability checks on the real (oriented) passage.
func (r *OpenWorldRegion) OpeningInteriorCells(side config.OpenWorldPortalSide, width int) [][2]int {
	edge, at := owXformSide(r.Orient, r.LocalWidth, r.LocalHeight, side.Edge, side.At, width)
	depth := side.Depth
	if depth == 0 {
		depth = 1
	}
	cells := make([][2]int, 0, width)
	for i := 0; i < width; i++ {
		var x, y int
		switch edge {
		case "north":
			x, y = r.OffsetX+at+i, r.OffsetY+depth
		case "south":
			x, y = r.OffsetX+at+i, r.OffsetY+r.Height-1-depth
		case "west":
			x, y = r.OffsetX+depth, r.OffsetY+at+i
		default: // east
			x, y = r.OffsetX+r.Width-1-depth, r.OffsetY+at+i
		}
		cells = append(cells, [2]int{x, y})
	}
	return cells
}

// owXformSpanDir rotates/mirrors a grid-span direction (e|s|w|n) into placed
// space, so multi-tile facades keep hugging the same wall after placement.
func owXformSpanDir(orient, dir string) string {
	dirs := map[string][2]int{"e": {1, 0}, "s": {0, 1}, "w": {-1, 0}, "n": {0, -1}}
	v, ok := dirs[dir]
	if !ok {
		return dir
	}
	var dx, dy int
	switch orient {
	case "rot90":
		dx, dy = -v[1], v[0]
	case "rot180":
		dx, dy = -v[0], -v[1]
	case "rot270":
		dx, dy = v[1], -v[0]
	case "mirror_x":
		dx, dy = -v[0], v[1]
	case "mirror_y":
		dx, dy = v[0], -v[1]
	default:
		return dir
	}
	for name, w := range dirs {
		if w[0] == dx && w[1] == dy {
			return name
		}
	}
	return dir
}

// SetOpenWorldConfig arms the manager to build the unified world during
// LoadAllMaps. Must be called before LoadAllMaps/Reset.
func (wm *WorldManager) SetOpenWorldConfig(owc *config.OpenWorldConfig) {
	wm.openWorldConfig = owc
}

// OpenWorldActive reports whether the unified world has been built.
func (wm *WorldManager) OpenWorldActive() bool { return wm.OpenWorld != nil }

// IsOpenWorldRegion reports whether mapKey is merged into the unified world.
func (wm *WorldManager) IsOpenWorldRegion(mapKey string) bool {
	_, ok := wm.openWorldRegionIdx[mapKey]
	return ok
}

// OpenWorldRegionByKey returns the region of a merged map, or nil.
func (wm *WorldManager) OpenWorldRegionByKey(mapKey string) *OpenWorldRegion {
	if idx, ok := wm.openWorldRegionIdx[mapKey]; ok {
		return &wm.OpenWorldRegions[idx]
	}
	return nil
}

// OpenWorldRegionAtTile resolves a unified-world tile to its region. Corridor
// cells are attributed to their nearest connected region; void returns nil.
func (wm *WorldManager) OpenWorldRegionAtTile(tx, ty int) *OpenWorldRegion {
	if wm.OpenWorld == nil || ty < 0 || ty >= len(wm.openWorldRegionGrid) || tx < 0 || tx >= len(wm.openWorldRegionGrid[ty]) {
		return nil
	}
	idx := wm.openWorldRegionGrid[ty][tx]
	if idx < 0 {
		return nil
	}
	return &wm.OpenWorldRegions[idx]
}

// WorldByKey resolves a map key to its world: a loaded split map, or the
// unified world for merged region keys.
func (wm *WorldManager) WorldByKey(mapKey string) *World3D {
	if w, ok := wm.LoadedMaps[mapKey]; ok {
		return w
	}
	if wm.OpenWorld != nil && (mapKey == OpenWorldKey || wm.IsOpenWorldRegion(mapKey)) {
		return wm.OpenWorld
	}
	return nil
}

// EachWorld visits every DISTINCT loaded world exactly once (split maps plus
// the unified world). Use for world-wide sweeps like merchant restocks.
func (wm *WorldManager) EachWorld(fn func(key string, w *World3D)) {
	for key, w := range wm.LoadedMaps {
		fn(key, w)
	}
	if wm.OpenWorld != nil {
		fn(OpenWorldKey, wm.OpenWorld)
	}
}

// SameWorldKey reports whether two map keys resolve to the same world (e.g.
// two merged region keys, or an identical split key).
func (wm *WorldManager) SameWorldKey(a, b string) bool {
	if a == b {
		return true
	}
	wa, wb := wm.WorldByKey(a), wm.WorldByKey(b)
	return wa != nil && wa == wb
}

// ProjectTile converts a map-local tile to unified coordinates (offset plus
// the region's placement orientation). Identity for non-merged maps.
func (wm *WorldManager) ProjectTile(mapKey string, tx, ty int) (int, int) {
	if r := wm.OpenWorldRegionByKey(mapKey); r != nil {
		px, py := owXformTile(r.Orient, r.LocalWidth, r.LocalHeight, tx, ty)
		return px + r.OffsetX, py + r.OffsetY
	}
	return tx, ty
}

// ProjectWorldPos converts map-local world coordinates to unified ones.
func (wm *WorldManager) ProjectWorldPos(mapKey string, x, y float64) (float64, float64) {
	if r := wm.OpenWorldRegionByKey(mapKey); r != nil {
		ts := wm.config.GetTileSize()
		px, py := owXformWorld(r.Orient, float64(r.LocalWidth)*ts, float64(r.LocalHeight)*ts, x, y)
		return px + float64(r.OffsetX)*ts, py + float64(r.OffsetY)*ts
	}
	return x, y
}

// ProjectAngle converts a map-local heading to the unified world (regions may
// be placed rotated/mirrored). Identity for non-merged maps.
func (wm *WorldManager) ProjectAngle(mapKey string, a float64) float64 {
	if r := wm.OpenWorldRegionByKey(mapKey); r != nil {
		return owXformAngle(r.Orient, a)
	}
	return a
}

// LocalizeAngle converts a unified-world heading back to a region's local one.
func (wm *WorldManager) LocalizeAngle(mapKey string, a float64) float64 {
	if r := wm.OpenWorldRegionByKey(mapKey); r != nil {
		return owXformAngleInv(r.Orient, a)
	}
	return a
}

// LocalizeTile is the inverse of ProjectTile: a unified-world tile back to
// the region's map-local tile (offset AND orientation).
func (wm *WorldManager) LocalizeTile(mapKey string, tx, ty int) (int, int) {
	if r := wm.OpenWorldRegionByKey(mapKey); r != nil {
		return owXformTileInv(r.Orient, r.LocalWidth, r.LocalHeight, tx-r.OffsetX, ty-r.OffsetY)
	}
	return tx, ty
}

// LocalizeWorldPos converts a unified-world position to (regionKey, local
// position). Positions on a region's border ring or in a corridor snap to the
// nearest interior tile, so the result is always valid inside the source map
// in split mode. ok is false when the unified world is off.
func (wm *WorldManager) LocalizeWorldPos(x, y float64) (string, float64, float64, bool) {
	if wm.OpenWorld == nil {
		return "", x, y, false
	}
	ts := wm.config.GetTileSize()
	tx, ty := int(x/ts), int(y/ts)
	r := wm.OpenWorldRegionAtTile(tx, ty)
	if r == nil {
		r = wm.nearestOpenWorldRegion(tx, ty)
		if r == nil {
			return "", x, y, false
		}
	}
	lx, ly := owXformWorldInv(r.Orient, float64(r.LocalWidth)*ts, float64(r.LocalHeight)*ts,
		x-float64(r.OffsetX)*ts, y-float64(r.OffsetY)*ts)
	// Clamp to the map's LOCAL interior (inside the border ring): a corridor
	// or carved-border position must localize to a tile that exists and is
	// inside the source map's walls.
	ltx, lty := int(lx/ts), int(ly/ts)
	ctx, cty := clampInt(ltx, 1, r.LocalWidth-2), clampInt(lty, 1, r.LocalHeight-2)
	if ctx != ltx || cty != lty {
		lx = (float64(ctx) + 0.5) * ts
		ly = (float64(cty) + 0.5) * ts
	}
	return r.MapKey, lx, ly, true
}

// nearestOpenWorldRegion picks the region whose rect is closest to a tile
// (void-cell fallback; corridors are pre-attributed in the region grid).
func (wm *WorldManager) nearestOpenWorldRegion(tx, ty int) *OpenWorldRegion {
	var best *OpenWorldRegion
	bestDist := -1
	for i := range wm.OpenWorldRegions {
		r := &wm.OpenWorldRegions[i]
		dx := rectAxisDist(tx, r.OffsetX, r.OffsetX+r.Width-1)
		dy := rectAxisDist(ty, r.OffsetY, r.OffsetY+r.Height-1)
		d := dx*dx + dy*dy
		if bestDist < 0 || d < bestDist {
			bestDist = d
			best = r
		}
	}
	return best
}

func rectAxisDist(v, lo, hi int) int {
	if v < lo {
		return lo - v
	}
	if v > hi {
		return v - hi
	}
	return 0
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// OpenWorldRegionStart returns the region's spawn tile center in unified
// world coordinates (the source map's '+').
func (wm *WorldManager) OpenWorldRegionStart(mapKey string) (float64, float64, bool) {
	r := wm.OpenWorldRegionByKey(mapKey)
	if r == nil || r.StartX < 0 || r.StartY < 0 {
		return 0, 0, false
	}
	ts := wm.config.GetTileSize()
	return (float64(r.StartX) + 0.5) * ts, (float64(r.StartY) + 0.5) * ts, true
}

// BiomeAtTile resolves the biome for a unified-world tile via its region
// (corridor cells inherit their nearest region). Empty string for void/off.
func (wm *WorldManager) BiomeAtTile(tx, ty int) string {
	r := wm.OpenWorldRegionAtTile(tx, ty)
	if r == nil {
		return ""
	}
	if mc, ok := wm.MapConfigs[r.MapKey]; ok {
		return mc.Biome
	}
	return ""
}

// MapConfigAtTile resolves the map config for a unified-world tile's region.
func (wm *WorldManager) MapConfigAtTile(tx, ty int) *config.MapConfig {
	r := wm.OpenWorldRegionAtTile(tx, ty)
	if r == nil {
		return nil
	}
	return wm.MapConfigs[r.MapKey]
}

// --- Stitcher --------------------------------------------------------------

// buildOpenWorld composes the unified world from the placed maps per
// wm.openWorldConfig. Runs at the end of LoadAllMaps (boot and Reset). All
// geometry problems are load-time errors: an overlap, a misaligned corridor
// or an opening carved into blocked terrain must never ship silently.
func (wm *WorldManager) buildOpenWorld() error {
	owc := wm.openWorldConfig
	if owc == nil {
		return nil
	}
	if GlobalTileManager == nil {
		return fmt.Errorf("open world: tile manager not loaded")
	}
	voidTile, ok := GlobalTileManager.GetTileTypeFromKey(owc.VoidTile)
	if !ok {
		return fmt.Errorf("open world: void_tile %q is not a defined tile", owc.VoidTile)
	}

	// Deterministic build order for stable region indices and monster order.
	keys := make([]string, 0, len(owc.Placements))
	for key := range owc.Placements {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	placed := make([]owPlacedMap, 0, len(keys))
	totalW, totalH := 0, 0
	for _, key := range keys {
		mc, ok := wm.MapConfigs[key]
		if !ok {
			return fmt.Errorf("open world: placement %q has no map config", key)
		}
		if mc.RespawnDays > 0 {
			return fmt.Errorf("open world: map %q uses respawn_days, which is per-map and unsupported in the unified world", key)
		}
		if mc.Duel != nil {
			return fmt.Errorf("open world: map %q has a duel config and cannot be merged", key)
		}
		loader := NewMapLoaderWithBiome(wm.config, mc.Biome)
		data, err := loader.LoadMap("assets/" + mc.File)
		if err != nil {
			return fmt.Errorf("open world: loading %q: %w", key, err)
		}
		defTile, ok := GlobalTileManager.GetTileTypeFromLetterForBiome(".", mc.Biome)
		if !ok {
			return fmt.Errorf("open world: biome %q has no '.' floor tile", mc.Biome)
		}
		off := owc.Placements[key]
		p := owPlacedMap{key: key, mc: mc, data: data, off: off, defTile: defTile}
		if err := applyOpenWorldRemovals(&p.data.NPCSpawns, &p.data.SpecialTileSpawns, p.data, owc.Removals[key], key, defTile); err != nil {
			return err
		}
		placed = append(placed, p)
		pw, ph := owPlacedDims(off.Orient, data.Width, data.Height)
		if off.X+pw > totalW {
			totalW = off.X + pw
		}
		if off.Y+ph > totalH {
			totalH = off.Y + ph
		}
	}

	// Pairwise overlap check on PLACED footprints (corridor gaps are
	// validated per connection).
	for i := range placed {
		for j := i + 1; j < len(placed); j++ {
			a, b := &placed[i], &placed[j]
			aw, ah := owPlacedDims(a.off.Orient, a.data.Width, a.data.Height)
			bw, bh := owPlacedDims(b.off.Orient, b.data.Width, b.data.Height)
			if a.off.X < b.off.X+bw && b.off.X < a.off.X+aw &&
				a.off.Y < b.off.Y+bh && b.off.Y < a.off.Y+ah {
				return fmt.Errorf("open world: placements %q and %q overlap", a.key, b.key)
			}
		}
	}

	// Compose the grid: void everywhere, then blit each map.
	merged := NewWorld3D(wm.config)
	merged.Width, merged.Height = totalW, totalH
	merged.StartX, merged.StartY = -1, -1
	merged.Tiles = make([][]TileType3D, totalH)
	regionGrid := make([][]int16, totalH)
	for y := 0; y < totalH; y++ {
		merged.Tiles[y] = make([]TileType3D, totalW)
		regionGrid[y] = make([]int16, totalW)
		for x := 0; x < totalW; x++ {
			merged.Tiles[y][x] = voidTile
			regionGrid[y][x] = -1
		}
	}

	regions := make([]OpenWorldRegion, 0, len(placed))
	regionIdx := make(map[string]int, len(placed))
	for i := range placed {
		p := &placed[i]
		pw, ph := owPlacedDims(p.off.Orient, p.data.Width, p.data.Height)
		for y := 0; y < p.data.Height; y++ {
			for x := 0; x < p.data.Width; x++ {
				px, py := owXformTile(p.off.Orient, p.data.Width, p.data.Height, x, y)
				merged.Tiles[p.off.Y+py][p.off.X+px] = p.data.Tiles[y][x]
			}
		}
		for y := 0; y < ph; y++ {
			for x := 0; x < pw; x++ {
				regionGrid[p.off.Y+y][p.off.X+x] = int16(i)
			}
		}
		startX, startY := -1, -1
		if p.data.StartX >= 0 && p.data.StartY >= 0 {
			sx, sy := owXformTile(p.off.Orient, p.data.Width, p.data.Height, p.data.StartX, p.data.StartY)
			startX, startY = p.off.X+sx, p.off.Y+sy
		}
		regions = append(regions, OpenWorldRegion{
			MapKey: p.key, OffsetX: p.off.X, OffsetY: p.off.Y,
			Width: pw, Height: ph,
			LocalWidth: p.data.Width, LocalHeight: p.data.Height,
			Orient: p.off.Orient,
			StartX: startX, StartY: startY,
		})
		regionIdx[p.key] = i
	}

	// Carve connections: openings through both border walls + the corridor.
	placedByKey := make(map[string]*owPlacedMap, len(placed))
	for i := range placed {
		placedByKey[placed[i].key] = &placed[i]
	}
	for ci := range owc.Connections {
		conn := &owc.Connections[ci]
		if err := carveOpenWorldConnection(merged, regionGrid, regionIdx, placedByKey, conn, owc); err != nil {
			return fmt.Errorf("open world: connection %d (%s->%s): %w", ci, conn.From.Map, conn.To.Map, err)
		}
	}

	// Populate entities. Monsters bind their clear-encounters on a local
	// staging world first (authored chest anchors are map-local; distances are
	// transform-invariant), then project into the unified world. Rewards keep
	// LOCAL coordinates + the source map key - the game projects them at
	// spawn time.
	ts := wm.config.GetTileSize()
	for i := range placed {
		p := &placed[i]
		pw, ph := owPlacedDims(p.off.Orient, p.data.Width, p.data.Height)
		localW, localH := float64(p.data.Width)*ts, float64(p.data.Height)*ts
		dx, dy := float64(p.off.X)*ts, float64(p.off.Y)*ts
		staging := NewWorld3D(wm.config)
		staging.Width, staging.Height = p.data.Width, p.data.Height
		staging.Tiles = p.data.Tiles
		staging.loadMonstersFromMapData(p.data.MonsterSpawns)
		wm.attachMapClearEncounter(staging, p.key, p.mc)
		for _, m := range staging.Monsters {
			m.X, m.Y = owXformWorld(p.off.Orient, localW, localH, m.X, m.Y)
			m.X += dx
			m.Y += dy
			m.SpawnX, m.SpawnY = m.X, m.Y
			merged.Monsters = append(merged.Monsters, m)
		}
		// Each region keeps its OWN authored kinds (region-scoped loot pools).
		// The merged world deliberately carries NO flattened union - nothing may
		// roll cross-biome loot.
		regions[regionIdx[p.key]].InitialMonsterKeys = staging.InitialMonsterKeys
		for _, spawn := range p.data.MonsterSpawns {
			sx, sy := owXformTile(p.off.Orient, p.data.Width, p.data.Height, spawn.X, spawn.Y)
			merged.MonsterSpawns = append(merged.MonsterSpawns, MonsterSpawn{
				X: sx + p.off.X, Y: sy + p.off.Y, MonsterKey: spawn.MonsterKey,
			})
		}

		// NPCs spawn directly at unified coordinates so ground_tile painting
		// lands on the merged grid.
		npcSpawns := make([]NPCSpawn, len(p.data.NPCSpawns))
		for si, spawn := range p.data.NPCSpawns {
			sx, sy := owXformTile(p.off.Orient, p.data.Width, p.data.Height, spawn.X, spawn.Y)
			npcSpawns[si] = NPCSpawn{X: sx + p.off.X, Y: sy + p.off.Y, NPCKey: spawn.NPCKey}
		}
		npcCountBefore := len(merged.NPCs)
		merged.loadNPCsFromMapData(npcSpawns)
		// Multi-tile facades (grid_span_dir) must keep hugging the same wall
		// when the map is placed rotated/mirrored.
		if p.off.Orient != "" && p.off.Orient != "none" {
			for _, npc := range merged.NPCs[npcCountBefore:] {
				if npc.GridSpanTiles >= 2 {
					npc.GridSpanDir = owXformSpanDir(p.off.Orient, npc.GridSpanDir)
				}
			}
		}

		// Teleporters register under the REGION key at unified coordinates, so
		// the step-on lookup (region key + global tile) finds them.
		registerTeleportersInRect(wm.GlobalTeleporterRegistry, p.key, merged.Tiles,
			p.off.X, p.off.Y, pw, ph)
	}

	// Spawn: the default starting map's '+' anchors the unified world.
	for _, r := range regions {
		if r.MapKey == wm.CurrentMapKey && r.StartX >= 0 {
			merged.StartX, merged.StartY = r.StartX, r.StartY
		}
	}
	if merged.StartX < 0 {
		for _, r := range regions {
			if r.StartX >= 0 {
				merged.StartX, merged.StartY = r.StartX, r.StartY
				break
			}
		}
	}
	if merged.StartX < 0 {
		return fmt.Errorf("open world: no placed map has a '+' start tile")
	}

	// Fly must never leave a region except through a carved passage: the void
	// filler acts as the unified world's flight boundary.
	merged.flyBoundary = func(tileX, tileY int) bool {
		if tileY < 0 || tileY >= len(regionGrid) || tileX < 0 || tileX >= len(regionGrid[tileY]) {
			return true
		}
		return regionGrid[tileY][tileX] < 0
	}

	wm.OpenWorld = merged
	wm.OpenWorldRegions = regions
	wm.openWorldRegionIdx = regionIdx
	wm.openWorldRegionGrid = regionGrid
	fmt.Printf("Open world built: %dx%d tiles, %d regions, %d monsters, %d NPCs\n",
		totalW, totalH, len(regions), len(merged.Monsters), len(merged.NPCs))
	return nil
}

// applyOpenWorldRemovals strips the listed gate NPCs and special tiles from
// one map's freshly-loaded data. A listed key that is not present is a config
// error (typo or stale entry), not a silent no-op.
func applyOpenWorldRemovals(npcSpawns *[]NPCSpawn, stileSpawns *[]SpecialTileSpawn, data *MapData, removal config.OpenWorldRemoval, mapKey string, defTile TileType3D) error {
	for _, key := range removal.NPCs {
		found := false
		kept := (*npcSpawns)[:0]
		for _, spawn := range *npcSpawns {
			if spawn.NPCKey == key {
				found = true
				continue
			}
			kept = append(kept, spawn)
		}
		*npcSpawns = kept
		if !found {
			return fmt.Errorf("open world: removals for %q list NPC %q not present on the map", mapKey, key)
		}
	}
	for _, key := range removal.SpecialTiles {
		found := false
		kept := (*stileSpawns)[:0]
		for _, spawn := range *stileSpawns {
			if spawn.TileKey == key {
				found = true
				// The loader already stamped the tile into the grid - revert it
				// to the biome floor.
				if spawn.Y >= 0 && spawn.Y < data.Height && spawn.X >= 0 && spawn.X < data.Width {
					data.Tiles[spawn.Y][spawn.X] = defTile
				}
				continue
			}
			kept = append(kept, spawn)
		}
		*stileSpawns = kept
		if !found {
			return fmt.Errorf("open world: removals for %q list special tile %q not present on the map", mapKey, key)
		}
	}
	return nil
}

// owPlacedMap is one source map staged for stitching.
type owPlacedMap struct {
	key     string
	mc      *config.MapConfig
	data    *MapData
	off     config.OpenWorldPlacement
	defTile TileType3D
}

// owEdgeIsVertical reports whether an edge runs along the map's east/west
// border (opening span varies over y) as opposed to north/south (over x).
func owEdgeIsVertical(edge string) bool { return edge == "east" || edge == "west" }

// owPlacedSide is one connection side resolved into placed space: the edge
// and span the opening occupies AFTER the map's placement orientation.
// Authors keep writing edge/at in the source map's own (local) terms.
type owPlacedSide struct {
	p     *owPlacedMap
	depth int
	edge  string // placed edge
	at    int    // placed span start
	pw    int    // placed footprint dims
	ph    int
}

func owPlaceSide(p *owPlacedMap, side *config.OpenWorldPortalSide, width int) owPlacedSide {
	edge, at := owXformSide(p.off.Orient, p.data.Width, p.data.Height, side.Edge, side.At, width)
	pw, ph := owPlacedDims(p.off.Orient, p.data.Width, p.data.Height)
	return owPlacedSide{p: p, depth: side.Depth, edge: edge, at: at, pw: pw, ph: ph}
}

// owOpeningCell returns the unified-grid cell of an opening at span index i,
// wall layer d (0 = the border itself, growing inward; -1 = just outside).
func (s owPlacedSide) owOpeningCell(i, d int) (int, int) {
	switch s.edge {
	case "north":
		return s.p.off.X + s.at + i, s.p.off.Y + d
	case "south":
		return s.p.off.X + s.at + i, s.p.off.Y + s.ph - 1 - d
	case "west":
		return s.p.off.X + d, s.p.off.Y + s.at + i
	default: // east
		return s.p.off.X + s.pw - 1 - d, s.p.off.Y + s.at + i
	}
}

// carveOpenWorldConnection cuts both border openings and paves the corridor
// between them, attributing corridor cells to their nearest region. Opposing
// edges get a straight corridor with strict geometry validation (span
// alignment, gap == corridor.length); any other edge pair is joined by an
// auto-routed bent canyon through the void (see routeBentCorridor).
func carveOpenWorldConnection(merged *World3D, regionGrid [][]int16, regionIdx map[string]int,
	placedByKey map[string]*owPlacedMap, conn *config.OpenWorldConnection, owc *config.OpenWorldConfig) error {
	from, to := placedByKey[conn.From.Map], placedByKey[conn.To.Map]
	corridorFloorKey := conn.FloorTile
	if corridorFloorKey == "" {
		corridorFloorKey = owc.Corridor.FloorTile
	}
	corridorFloor, ok := GlobalTileManager.GetTileTypeFromKey(corridorFloorKey)
	if !ok {
		return fmt.Errorf("corridor floor tile %q is not a defined tile", corridorFloorKey)
	}

	// Span bounds: keep at least one wall tile at each edge corner (checked
	// in local terms; transforms preserve span-in-edge containment).
	for _, sp := range []struct {
		p    *owPlacedMap
		side *config.OpenWorldPortalSide
	}{{from, &conn.From}, {to, &conn.To}} {
		limit := sp.p.data.Height
		if !owEdgeIsVertical(sp.side.Edge) {
			limit = sp.p.data.Width
		}
		if sp.side.At < 1 || sp.side.At+conn.Width > limit-1 {
			return fmt.Errorf("map %q: opening span [%d,%d) exceeds edge bounds (1..%d)",
				sp.p.key, sp.side.At, sp.side.At+conn.Width, limit-1)
		}
	}

	fromS := owPlaceSide(from, &conn.From, conn.Width)
	toS := owPlaceSide(to, &conn.To, conn.Width)

	// Carve the two openings (each side's biome floor), verify the terrain
	// just behind each opening is walkable so the passage cannot dead-end.
	for _, s := range []owPlacedSide{fromS, toS} {
		for i := 0; i < conn.Width; i++ {
			for d := 0; d < s.depth; d++ {
				gx, gy := s.owOpeningCell(i, d)
				merged.Tiles[gy][gx] = s.p.defTile
			}
			bx, by := s.owOpeningCell(i, s.depth)
			if !GlobalTileManager.IsWalkable(merged.Tiles[by][bx]) {
				return fmt.Errorf("map %q: opening at span index %d is blocked behind the wall (tile %q at placed %d,%d)",
					s.p.key, i, GlobalTileManager.GetTileKey(merged.Tiles[by][bx]), bx-s.p.off.X, by-s.p.off.Y)
			}
		}
	}

	fromIdx, toIdx := int16(regionIdx[conn.From.Map]), int16(regionIdx[conn.To.Map])

	// Straight corridor only for opposing placed edges with aligned spans and
	// the canonical gap; anything else is auto-routed.
	if fromS.edge == config.OpposingEdge(toS.edge) {
		fx0, fy0 := fromS.owOpeningCell(0, 0)
		tx0, ty0 := toS.owOpeningCell(0, 0)
		vertical := owEdgeIsVertical(fromS.edge)
		aligned := (vertical && fy0 == ty0) || (!vertical && fx0 == tx0)
		var gapLo, gapHi int // inclusive unified coords along the crossing axis
		if vertical {
			lo, hi := fx0, tx0
			if lo > hi {
				lo, hi = hi, lo
			}
			gapLo, gapHi = lo+1, hi-1
		} else {
			lo, hi := fy0, ty0
			if lo > hi {
				lo, hi = hi, lo
			}
			gapLo, gapHi = lo+1, hi-1
		}
		if aligned && gapHi-gapLo+1 == owc.Corridor.Length {
			// Pave the straight corridor; each cell joins its nearest side's
			// region so biome rendering and save localization stay sensible.
			fromNear := fy0
			if vertical {
				fromNear = fx0
			}
			for i := 0; i < conn.Width; i++ {
				for g := gapLo; g <= gapHi; g++ {
					var gx, gy int
					if vertical {
						gx, gy = g, fy0+i
					} else {
						gx, gy = fx0+i, g
					}
					merged.Tiles[gy][gx] = corridorFloor
					idx := toIdx
					distFrom := g - fromNear
					if distFrom < 0 {
						distFrom = -distFrom
					}
					if distFrom*2 <= owc.Corridor.Length {
						idx = fromIdx
					}
					regionGrid[gy][gx] = idx
				}
			}
			return nil
		}
	}
	return routeBentCorridor(merged, regionGrid, fromS, toS, fromIdx, toIdx, conn.Width, corridorFloor)
}

// routeBentCorridor joins two openings that cannot take a straight corridor
// with a deterministic routed canyon: an L when the exit axes are
// perpendicular, a Z when they are parallel. Every canyon cell must lie in
// unoccupied void - a route crossing any placed map is a load-time error,
// not a tunnel.
func routeBentCorridor(merged *World3D, regionGrid [][]int16,
	fromS, toS owPlacedSide, fromIdx, toIdx int16, width int, corridorFloor TileType3D) error {
	var cells [][2]int
	hline := func(x1, x2, y int) {
		for x := minInt(x1, x2); x <= maxInt(x1, x2); x++ {
			cells = append(cells, [2]int{x, y})
		}
	}
	vline := func(y1, y2, x int) {
		for y := minInt(y1, y2); y <= maxInt(y1, y2); y++ {
			cells = append(cells, [2]int{x, y})
		}
	}
	for i := 0; i < width; i++ {
		px, py := fromS.owOpeningCell(i, -1) // just outside the from border
		qx, qy := toS.owOpeningCell(i, -1)   // just outside the to border
		fromHorizontal := owEdgeIsVertical(fromS.edge)
		toHorizontal := owEdgeIsVertical(toS.edge)
		switch {
		case fromHorizontal && toHorizontal:
			// Z: out along the from row, across on the midpoint column, in
			// along the to row.
			midX := (px + qx) / 2
			hline(px, midX, py)
			vline(py, qy, midX)
			hline(midX, qx, qy)
		case !fromHorizontal && !toHorizontal:
			midY := (py + qy) / 2
			vline(py, midY, px)
			hline(px, qx, midY)
			vline(midY, qy, qx)
		case fromHorizontal:
			// L: along the from row, then the to column.
			hline(px, qx, py)
			vline(py, qy, qx)
		default:
			vline(py, qy, px)
			hline(px, qx, qy)
		}
	}
	seen := make(map[[2]int]bool, len(cells))
	for _, c := range cells {
		if seen[c] {
			continue
		}
		seen[c] = true
		x, y := c[0], c[1]
		if x < 0 || x >= merged.Width || y < 0 || y >= merged.Height {
			return fmt.Errorf("routed corridor leaves the world grid at (%d,%d) - adjust placements", x, y)
		}
		if regionGrid[y][x] != -1 {
			return fmt.Errorf("routed corridor cell (%d,%d) crosses a placed map - adjust placements", x, y)
		}
		merged.Tiles[y][x] = corridorFloor
		// Attribute to the nearer of the two connected regions.
		df := rectAxisDist(x, fromS.p.off.X, fromS.p.off.X+fromS.pw-1) + rectAxisDist(y, fromS.p.off.Y, fromS.p.off.Y+fromS.ph-1)
		dt := rectAxisDist(x, toS.p.off.X, toS.p.off.X+toS.pw-1) + rectAxisDist(y, toS.p.off.Y, toS.p.off.Y+toS.ph-1)
		if df <= dt {
			regionGrid[y][x] = fromIdx
		} else {
			regionGrid[y][x] = toIdx
		}
	}
	return nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// registerTeleportersInRect scans one region's rect of the unified grid and
// registers surviving teleporter tiles under the region's map key at unified
// coordinates (mirrors RegisterTeleportersFromMapData for merged maps).
func registerTeleportersInRect(registry *TeleporterRegistry, mapKey string, tiles [][]TileType3D, x0, y0, w, h int) {
	if registry == nil || GlobalTileManager == nil {
		return
	}
	sub := make([][]TileType3D, h)
	for y := 0; y < h; y++ {
		sub[y] = tiles[y0+y][x0 : x0+w]
	}
	// Reuse the shared scanner on the sub-grid, then shift the fresh entries
	// to unified coordinates.
	before := len(registry.Teleporters)
	RegisterTeleportersFromMapData(nil, mapKey, registry, sub)
	for i := before; i < len(registry.Teleporters); i++ {
		t := &registry.Teleporters[i]
		t.X += x0
		t.Y += y0
		t.Label = fmt.Sprintf("%s_%s_%d_%d", t.MapKey, t.Group, t.X, t.Y)
	}
}
