package game

import (
	"math"
	uitext "ugataima/assets/text"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/status"
	"ugataima/internal/world"
)

// FrozenWaterTile stores local authored map coordinates, never stitched offsets.
// The shared combat clock expires ice on visited and unvisited maps alike.
type FrozenWaterTile struct {
	MapKey string           `json:"map_key"`
	X      int              `json:"x"`
	Y      int              `json:"y"`
	Clock  status.DualClock `json:"clock,omitzero"`
	// LegacyFrames is an older save's time left, moved into Clock on load.
	LegacyFrames int `json:"frames_remaining,omitempty"`
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
		if ice.MapKey == key && ice.X == x && ice.Y == y && ice.Clock.Active() {
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
				ice.Clock.Arm(def.Mechanic.WaterFreezeSeconds, g.config.GetTPS())
				found = true
				break
			}
		}
		if !found {
			ice := FrozenWaterTile{MapKey: key, X: lx, Y: ly}
			ice.Clock.Arm(def.Mechanic.WaterFreezeSeconds, g.config.GetTPS())
			g.frozenWater = append(g.frozenWater, ice)
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
			if !cs.inPartyCone(x, y, wx, wy, a.ConeDegrees) {
				continue
			}
			visit(tx, ty)
		}
	}
}

// settleStrandedMonsters moves every monster and ally whose terrain no longer
// holds it (thawed Frozen Ground) to the nearest tile it can stand on, as
// settleAshore does for the party. Runs on thaw and when a world becomes current.
func (g *MMGame) settleStrandedMonsters() {
	if g.world == nil || g.collisionSystem == nil || g.config == nil {
		return
	}
	g.syncFrozenWaterCollision()
	ts := float64(g.config.GetTileSize())
	// Stranded is judged on terrain alone; the landing must also be clear of
	// solid entities (NPCs, props), so nobody is set down inside one.
	stands := func(m *monster.Monster3D, x, y float64) bool {
		return g.collisionSystem.CanOccupyTilesWithTileOverrides(m.ID, x, y, m.WalkableTileOverrides, m.Flying)
	}
	lands := func(m *monster.Monster3D, x, y float64) bool {
		return g.collisionSystem.CanMoveToWithTileOverrides(m.ID, x, y, m.WalkableTileOverrides, m.Flying)
	}
	// Only water strands an actor this way; authored or arboreal footing is left alone.
	inWater := func(m *monster.Monster3D) bool {
		e := g.collisionSystem.GetEntityByID(m.ID)
		if e == nil || e.BoundingBox == nil || world.GlobalTileManager == nil {
			return false
		}
		minX, minY, maxX, maxY := e.BoundingBox.GetBounds()
		for ty := TileIndex(minY, ts); ty <= TileIndex(maxY-0.001, ts); ty++ {
			for tx := TileIndex(minX, ts); tx <= TileIndex(maxX-0.001, ts); tx++ {
				if tx >= 0 && ty >= 0 && tx < g.world.Width && ty < g.world.Height && world.GlobalTileManager.IsWater(g.world.Tiles[ty][tx]) {
					return true
				}
			}
		}
		return false
	}
	for _, m := range g.world.Monsters {
		if m == nil || !m.IsAlive() || m.IsFish() || m.Arbor.Phase != "" || !inWater(m) || stands(m, m.X, m.Y) {
			continue
		}
		cx, cy := TileIndex(m.X, ts), TileIndex(m.Y, ts)
		for r := 1; r < max(g.world.Width, g.world.Height); r++ {
			bestX, bestY, best := 0.0, 0.0, math.Inf(1)
			for ty := cy - r; ty <= cy+r; ty++ {
				for tx := cx - r; tx <= cx+r; tx++ {
					if max(absInt(tx-cx), absInt(ty-cy)) != r {
						continue
					}
					x, y := TileCenterFromTile(tx, ty, ts)
					if d := Distance(m.X, m.Y, x, y); d < best && lands(m, x, y) {
						bestX, bestY, best = x, y, d
					}
				}
			}
			if !math.IsInf(best, 1) {
				g.releaseMonsterAttackPost(m)
				m.X, m.Y = bestX, bestY
				m.ResetPathfinding()
				g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
				break
			}
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

// advanceFrozenWater spends one tick of every frozen tile: a frame or a turn.
func (g *MMGame) advanceFrozenWater(turn bool) {
	g.syncFrozenWaterCollision()
	if len(g.frozenWater) == 0 {
		return
	}
	wasOnIce := g.isFrozenWaterAt(g.camera.X, g.camera.Y)
	kept := g.frozenWater[:0]
	oldCount := len(g.frozenWater)
	for _, ice := range g.frozenWater {
		ice.Clock.Tick(turn)
		if ice.Clock.Active() {
			kept = append(kept, ice)
		}
	}
	g.frozenWater = kept
	if len(kept) != oldCount {
		g.invalidateFrozenWaterNavigation()
		g.settleStrandedMonsters()
	}
	if wasOnIce && !g.isFrozenWaterAt(g.camera.X, g.camera.Y) && !g.walkOnWaterEffective() && !g.waterBreathingActive && !g.partyHasTerrainPassage() && g.world != nil && g.world.IsTileBlockingTerrainAt(TileIndex(g.camera.X, g.config.GetTileSize()), TileIndex(g.camera.Y, g.config.GetTileSize())) {
		g.settleAshore(uitext.Text("advanced.ice_melts"))
	}
}
