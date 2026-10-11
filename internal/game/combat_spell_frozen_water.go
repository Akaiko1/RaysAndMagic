package game

import (
	"math"
	"ugataima/internal/config"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// FrozenWaterTile stores local authored map coordinates, never stitched offsets.
// The shared combat clock expires ice on visited and unvisited maps alike.
type FrozenWaterTile struct {
	MapKey          string `json:"map_key"`
	X               int    `json:"x"`
	Y               int    `json:"y"`
	FramesRemaining int    `json:"frames_remaining"`
}

// frozenWaterLocal resolves a tile of world w to the key and local tile ice is stored under.
func (g *MMGame) frozenWaterLocal(w *world.World3D, tileX, tileY int) (string, int, int) {
	key := g.mapKeyAtWorldTile(w, tileX, tileY)
	if wm := world.GlobalWorldManager; wm != nil && wm.OpenWorld != nil && w == wm.OpenWorld {
		tileX, tileY = wm.LocalizeTile(key, tileX, tileY)
	}
	return key, tileX, tileY
}

func (g *MMGame) isFrozenWaterTile(w *world.World3D, tileX, tileY int) bool {
	if len(g.frozenWater) == 0 {
		return false // every water probe in pathfinding asks; skip the key lookup
	}
	key, x, y := g.frozenWaterLocal(w, tileX, tileY)
	for _, ice := range g.frozenWater {
		if ice.MapKey == key && ice.X == x && ice.Y == y && ice.FramesRemaining > 0 {
			return true
		}
	}
	return false
}

func (g *MMGame) isFrozenWaterAt(x, y float64) bool {
	ts := g.config.GetTileSize()
	return g.isFrozenWaterTile(g.world, TileIndex(x, ts), TileIndex(y, ts))
}

func (g *MMGame) syncFrozenWaterCollision() { g.bindFrozenWater(g.world) }

// bindFrozenWater points w's terrain probe at the shared ice list in w's own
// terms, so a beacon or portal check on a world the party is not in reads it right.
func (g *MMGame) bindFrozenWater(w *world.World3D) {
	if w != nil {
		w.FrozenWater = func(tx, ty int) bool { return g.isFrozenWaterTile(w, tx, ty) }
	}
}

func (cs *CombatSystem) freezeWaterCone(def spells.SpellDefinition) bool {
	g := cs.game
	if g.world == nil {
		return false
	}
	changed := false
	originX, originY := cs.logicalCameraXY()
	cs.forEachFrozenConeTile(def, func(tx, ty int) {
		tile := g.world.Tiles[ty][tx]
		water := world.GlobalTileManager != nil && world.GlobalTileManager.IsWater(tile)
		if !water && g.world.IsTileBlockingTerrainAt(tx, ty) {
			return
		}
		if authored, ok := config.GetSpellDefinition(string(def.ID)); ok && authored.Graphics != nil && authored.Graphics.WorldFx != "" {
			g.spellWorldVisuals = append(g.spellWorldVisuals, spellWorldVisual{kind: authored.Graphics.WorldFx, world: g.world, tx: tx, ty: ty, born: g.frameCount, originX: originX, originY: originY})
		}
		if !water {
			return
		}
		key, lx, ly := g.frozenWaterLocal(g.world, tx, ty)
		found := false
		for i := range g.frozenWater {
			ice := &g.frozenWater[i]
			if ice.MapKey == key && ice.X == lx && ice.Y == ly {
				ice.FramesRemaining = max(ice.FramesRemaining, def.Duration*g.config.GetTPS())
				found = true
				break
			}
		}
		if !found {
			g.frozenWater = append(g.frozenWater, FrozenWaterTile{key, lx, ly, def.Duration * g.config.GetTPS()})
		}
		changed = true
	})
	g.syncFrozenWaterCollision()
	if changed {
		g.invalidateFrozenWaterNavigation()
	}
	return changed
}

// The actual water crossing and its growing ice share exactly one footprint.
func (cs *CombatSystem) forEachFrozenConeTile(def spells.SpellDefinition, visit func(int, int)) {
	g := cs.game
	a := def.Mechanic
	ts := g.config.GetTileSize()
	x, y := cs.logicalCameraXY()
	reach := int(math.Ceil(a.RadiusTiles))
	cx, cy := TileIndex(x, ts), TileIndex(y, ts)
	for ty := max(0, cy-reach); ty <= min(g.world.Height-1, cy+reach); ty++ {
		for tx := max(0, cx-reach); tx <= min(g.world.Width-1, cx+reach); tx++ {
			wx, wy := TileCenterFromTile(tx, ty, ts)
			if Distance(x, y, wx, wy) > a.RadiusTiles*ts || !cs.attackLineClear(x, y, wx, wy) {
				continue
			}
			if math.Cos(math.Atan2(wy-y, wx-x)-cs.partyAttackAngle()) < math.Cos(a.ConeDegrees*math.Pi/360) {
				continue
			}
			visit(tx, ty)
		}
	}
}

// replaceFrozenWater swaps the whole ice list (load, reset) and refreshes the
// collision probe, floor material and aura that read it.
func (g *MMGame) replaceFrozenWater(ice []FrozenWaterTile) {
	g.frozenWater = ice
	g.syncFrozenWaterCollision()
	g.invalidateFrozenWaterNavigation()
}

func (g *MMGame) invalidateFrozenWaterNavigation() {
	if g.gameLoop != nil && g.gameLoop.renderer != nil {
		g.gameLoop.renderer.combatAura.valid = false
		g.gameLoop.renderer.syncFrozenFloorMaterial()
	}
}

func (g *MMGame) advanceFrozenWater(elapsed int) {
	g.syncFrozenWaterCollision()
	if elapsed <= 0 || len(g.frozenWater) == 0 {
		return
	}
	wasOnIce := g.isFrozenWaterAt(g.camera.X, g.camera.Y)
	kept := g.frozenWater[:0]
	oldCount := len(g.frozenWater)
	for _, ice := range g.frozenWater {
		ice.FramesRemaining -= elapsed
		if ice.FramesRemaining > 0 {
			kept = append(kept, ice)
		}
	}
	g.frozenWater = kept
	if len(kept) != oldCount {
		g.invalidateFrozenWaterNavigation()
	}
	if wasOnIce && !g.isFrozenWaterAt(g.camera.X, g.camera.Y) && !g.walkOnWaterEffective() && !g.waterBreathingActive && !g.partyHasTerrainPassage() && g.world != nil && g.world.IsTileBlockingTerrainAt(TileIndex(g.camera.X, g.config.GetTileSize()), TileIndex(g.camera.Y, g.config.GetTileSize())) {
		g.settleAshore("The ice melts - the party returns to shore.")
	}
}
