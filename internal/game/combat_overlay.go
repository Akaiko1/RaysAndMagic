package game

import (
	"cmp"
	"hash/maphash"
	"math"
	"slices"
	"ugataima/internal/monster"
)

// Only the party's battle participates; unrelated ecology and friendly summons
// do not acquire red outlines. Enemies fighting a bound ally still participate.
func combatAuraHostile(m *monster.Monster3D) bool {
	return m != nil && m.IsAlive() && !m.IsPartyControlled() &&
		(m.TargetsParty() || (m.IsInCombat() && m.AIFoe != nil && m.AIFoe.IsAlive() && m.AIFoe.Bound))
}

func (g *MMGame) combatAuraMoveSteps(m *monster.Monster3D) int {
	if g.monsterMovementHeld(m) {
		return 0
	}
	if g.currentTurn == 1 && g.turnBasedMonsterPassesLeft > 0 {
		return g.turnBasedMonsterPassesLeft
	}
	if g.currentTurn == 0 && g.movementGrantsExtraMonsterPass() {
		return 2
	}
	return g.monsterTurnState.plannedPasses()
}

func (g *MMGame) combatAuraMovement(c *combatAuraScratch, m *monster.Monster3D, steps int, ts float64) {
	clear(c.tiles)
	if steps <= 0 || g.collisionSystem == nil {
		return
	}
	start := [2]int{TileIndex(m.X, ts), TileIndex(m.Y, ts)}
	c.tiles[start] = true
	c.queue = append(c.queue[:0], start)
	begin := 0
	for step := 0; step < steps; step++ {
		end := len(c.queue)
		for _, from := range c.queue[begin:end] {
			for _, d := range auraCardinalDirections {
				next := [2]int{from[0] + d[0], from[1] + d[1]}
				if c.tiles[next] {
					continue
				}
				if !g.monsterCanEnterTileTB(m, next[0], next[1]) {
					continue
				}
				c.tiles[next] = true
				c.queue = append(c.queue, next)
			}
		}
		begin = end
	}
	if len(c.tiles) == 1 {
		clear(c.tiles)
	}
}

func (g *MMGame) combatAuraAttacks(c *combatAuraScratch, m *monster.Monster3D, ts float64) {
	clear(c.tiles)
	if !m.CurrentAIBehavior().Caps().MayAttack || m.StunTurnsRemaining > 0 || g.turnBasedMonsterStunned[m] {
		return
	}
	tx, ty := TileIndex(m.X, ts), TileIndex(m.Y, ts)
	add := func(x, y int) {
		if x < 0 || y < 0 || x >= g.world.Width || y >= g.world.Height {
			return
		}
		px, py := TileCenterFromTile(x, y, ts)
		if g.combat.monsterAttackPointInReachTB(m, px, py) || g.combat.monsterPouncePointInReachTB(m, px, py, g.combatAuraPounceCooldown(m)) {
			c.tiles[[2]int{x, y}] = true
		}
	}
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			add(tx+dx, ty+dy)
		}
	}
	if m.HasRangedAttack() {
		// Cardinal firing lanes, not a misleading filled ranged-attack disk.
		reach := min(max(g.world.Width, g.world.Height), max(1, int(m.GetAttackRangePixels()/ts)))
		for _, d := range auraCardinalDirections {
			for step := 2; step <= reach; step++ {
				add(tx+d[0]*step, ty+d[1]*step)
			}
		}
	}
	areaReach := 0
	if !m.HasRangedAttack() {
		areaReach = int(math.Ceil(m.GetAttackRangePixels() / ts))
	}
	if m.CanPounce() && g.combatAuraPounceCooldown(m) <= 0 {
		areaReach = max(areaReach, int(m.PounceRangePixels/ts))
	}
	if areaReach > 1 {
		reach := min(max(g.world.Width, g.world.Height), areaReach)
		for y := max(0, ty-reach); y <= min(g.world.Height-1, ty+reach); y++ {
			for x := max(0, tx-reach); x <= min(g.world.Width-1, tx+reach); x++ {
				add(x, y)
			}
		}
	}
	if len(c.tiles) > 0 {
		// Do not create a false inner hole around the attacker.
		c.tiles[[2]int{tx, ty}] = true
	}
}

func (g *MMGame) combatAuraPounceCooldown(m *monster.Monster3D) int {
	if g.currentTurn == 0 || !g.turnBasedMonsterStatusTick {
		return max(0, m.PounceCDTurns-1)
	}
	return m.PounceCDTurns
}

func (g *MMGame) prepareCombatAura(c *combatAuraScratch, ts float64) bool {
	px, py := g.combat.logicalCameraXY()
	radius := 0.0
	for _, m := range g.world.Monsters {
		if combatAuraHostile(m) {
			radius = max(radius, Distance(px, py, m.X, m.Y)/ts+float64(g.combatAuraMoveSteps(m)))
		}
	}
	if radius <= 0 {
		c.valid = false
		return false
	}
	key := g.combatAuraStateKey(px, py, radius, ts)
	if c.valid && c.world == g.world && c.key == key {
		return true
	}
	c.valid, c.world, c.key = true, g.world, key
	c.reset()
	// Limit work to the camera's view while retaining the full battle radius.
	ptx, pty := TileIndex(px, ts), TileIndex(py, ts)
	reach := int(math.Ceil(min(radius, g.camera.ViewDist/ts))) + 1
	for ty := max(0, pty-reach); ty <= min(g.world.Height-1, pty+reach); ty++ {
		for tx := max(0, ptx-reach); tx <= min(g.world.Width-1, ptx+reach); tx++ {
			x, y := TileCenterFromTile(tx, ty, ts)
			if g.world.IsTileBlocking(tx, ty) || Distance(px, py, x, y) > radius*ts {
				continue
			}
			for _, edge := range combatTileEdges(tx, ty) {
				c.add(edge, combatAuraGrid)
			}
		}
	}
	for _, m := range g.world.Monsters {
		if !combatAuraHostile(m) {
			continue
		}
		g.combatAuraMovement(c, m, g.combatAuraMoveSteps(m), ts)
		c.outline(combatAuraMove)
		g.combatAuraAttacks(c, m, ts)
		c.outline(combatAuraAttack)
	}
	for edge := range c.edges {
		c.ordered = append(c.ordered, edge)
	}
	// Stable submission also keeps floating-point blend order stable at corners.
	slices.SortFunc(c.ordered, func(a, b combatAuraEdge) int {
		if a.y != b.y {
			return cmp.Compare(a.y, b.y)
		}
		if a.x != b.x {
			return cmp.Compare(a.x, b.x)
		}
		if a.vertical == b.vertical {
			return 0
		}
		if a.vertical {
			return 1
		}
		return -1
	})
	return true
}

var combatAuraHashSeed = maphash.MakeSeed()

type combatAuraActorKey struct {
	id                                                                        string
	x, y, rangePixels, pounce                                                 float64
	targetX, targetY                                                          float64
	behavior                                                                  monster.AIBehaviorMode
	steps, cooldown                                                           int
	alive, hostile, hasTarget, ranged, champion, flying, pounceReady, stunned bool
	bounds                                                                    [4]int
	hasBounds                                                                 bool
}

// This small signature replaces per-frame path searches, LOS casts, cell maps
// and sorting. Tile changes invalidate through the existing floor-cache owner;
// moving/registered blockers and authored traversal limits are observed here.
func (g *MMGame) combatAuraStateKey(px, py, radius, ts float64) uint64 {
	key := maphash.Comparable(combatAuraHashSeed, [5]float64{px, py, radius, ts, g.camera.ViewDist}) ^ g.collisionSystem.NavigationStamp() ^ uint64(g.world.TraversalModeKey())
	key ^= maphash.Comparable(combatAuraHashSeed, [2]int{g.world.Width, g.world.Height})
	for _, m := range g.world.Monsters {
		if m == nil {
			continue
		}
		_, tx, ty, target := g.monsterAttackTarget(m)
		state := combatAuraActorKey{id: m.ID, x: m.X, y: m.Y, rangePixels: m.GetAttackRangePixels(), pounce: m.PounceRangePixels, targetX: tx, targetY: ty,
			behavior: m.CurrentAIBehavior(), steps: g.combatAuraMoveSteps(m), cooldown: g.combatAuraPounceCooldown(m), alive: m.IsAlive(), hostile: combatAuraHostile(m), hasTarget: target, ranged: m.HasRangedAttack(), champion: m.IsChampion(), flying: m.Flying, pounceReady: m.CanPounce(), stunned: m.StunTurnsRemaining > 0 || g.turnBasedMonsterStunned[m]}
		if m.AmbientBounds != nil {
			state.bounds = *m.AmbientBounds
			state.hasBounds = true
		}
		actor := maphash.Comparable(combatAuraHashSeed, state)
		for _, tile := range m.WalkableTileOverrides {
			actor ^= maphash.String(combatAuraHashSeed, tile)
		}
		key ^= actor
	}
	return key
}
