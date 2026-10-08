package game

import (
	"math"
	"sort"

	"ugataima/internal/monster"
)

type attackPostTile struct {
	x int
	y int
}

// reconcileMonsterAttackPosts serializes the one part of combat positioning
// that cannot be decided independently by parallel RT AI: only one combatant
// may settle on a given logical attack tile. The target may be the party or a
// monster foe. Physical positions remain unchanged; a losing combatant becomes
// transit only while it still shares the winning post.
func (gl *GameLoop) reconcileMonsterAttackPosts() {
	if gl == nil || gl.game == nil || gl.game.world == nil || gl.game.config == nil {
		return
	}
	tileSize := float64(gl.game.config.GetTileSize())
	if tileSize <= 0 {
		return
	}

	posts := gl.attackPostBuf[:0]
	for _, m := range gl.game.world.Monsters {
		if m == nil || !m.IsAlive() {
			continue
		}
		wasPost, wasTarget := m.AttackPost, m.AttackPostTargetID
		gl.game.syncMonsterAttackPost(m)
		if wasPost != m.AttackPost || wasTarget != m.AttackPostTargetID {
			gl.game.applyMonsterCollisionType(m.ID, desiredMonsterCollisionType(m))
		}
		if monsterHoldsAttackPost(m) {
			posts = append(posts, m)
		}
	}

	sort.Slice(posts, func(i, j int) bool {
		a, b := posts[i], posts[j]
		atx, aty := TileIndex(a.X, tileSize), TileIndex(a.Y, tileSize)
		btx, bty := TileIndex(b.X, tileSize), TileIndex(b.Y, tileSize)
		if aty != bty {
			return aty < bty
		}
		if atx != btx {
			return atx < btx
		}
		// A settled attacker keeps its post when a newcomer arrives on the
		// same frame. A stable ID breaks simultaneous-entry ties deterministically.
		if a.AttackPostSince != b.AttackPostSince {
			return a.AttackPostSince < b.AttackPostSince
		}
		return a.ID < b.ID
	})

	for i := 0; i < len(posts); {
		winner := posts[i]
		key := attackPostTile{x: TileIndex(winner.X, tileSize), y: TileIndex(winner.Y, tileSize)}
		gl.game.applyMonsterCollisionType(winner.ID, desiredMonsterCollisionType(winner))
		i++
		for i < len(posts) {
			candidate := posts[i]
			candidateKey := attackPostTile{x: TileIndex(candidate.X, tileSize), y: TileIndex(candidate.Y, tileSize)}
			if candidateKey != key {
				break
			}
			gl.game.releaseMonsterAttackPost(candidate)
			i++
		}
	}

	// Recompute transit after winners/losers have updated the live markers. A
	// regular pursuer stays targetable by party arcs; only actual overlap with a
	// claimed combat tile gets the transit exception.
	for _, m := range gl.game.world.Monsters {
		if m == nil || !m.IsAlive() || !gl.game.monsterHasAttackTarget(m) || m.AttackPost ||
			(m.State != monster.StateAlert && m.State != monster.StatePursuing) {
			if m != nil {
				m.AttackTransit = false
			}
			continue
		}
		m.AttackTransit = gl.game.collisionSystem != nil &&
			gl.game.collisionSystem.IsMonsterAttackPostReserved(m.ID, m.X, m.Y)
	}
	gl.attackPostBuf = posts
}

// combatStackParticipant reports whether a tile needs temporary combat-stack
// visuals. Once one active pursuer enters a tile, every live monster already
// there joins the render-only fan; otherwise a calm occupant would be hidden
// inside the passing combatant. Tiles containing only calm monsters retain
// normal banding visuals.
func combatStackParticipant(g *MMGame, m *monster.Monster3D) bool {
	if g == nil || !g.monsterHasAttackTarget(m) {
		return false
	}
	switch m.State {
	case monster.StateAlert, monster.StatePursuing, monster.StateAttacking:
		return true
	default:
		return false
	}
}

// updateCombatTransitVisualStacks assigns the same compact stack slots as calm
// bands to occupants of a combat tile, without inventing a BandID. It never
// recentres actors on a moving group centroid. Only the compact fan eases in
// and out, always clipped to the current tile. Combat uses physical positions.
func (gl *GameLoop) updateCombatTransitVisualStacks() {
	if gl == nil || gl.game == nil || gl.game.world == nil || gl.game.config == nil {
		return
	}
	tileSize := float64(gl.game.config.GetTileSize())
	if tileSize <= 0 {
		return
	}

	if gl.combatTransitTileBuf == nil {
		gl.combatTransitTileBuf = make(map[attackPostTile]struct{})
	} else {
		clear(gl.combatTransitTileBuf)
	}
	stacks := gl.combatTransitStackBuf[:0]
	for _, m := range gl.game.world.Monsters {
		if m == nil {
			continue
		}
		m.TransitStackIndex = 0
		m.TransitStackCount = 0
		if m.IsAlive() && combatStackParticipant(gl.game, m) {
			gl.combatTransitTileBuf[attackPostTile{x: TileIndex(m.X, tileSize), y: TileIndex(m.Y, tileSize)}] = struct{}{}
		}
	}
	for _, m := range gl.game.world.Monsters {
		if m != nil && m.IsAlive() {
			key := attackPostTile{x: TileIndex(m.X, tileSize), y: TileIndex(m.Y, tileSize)}
			if _, active := gl.combatTransitTileBuf[key]; !active {
				continue
			}
			stacks = append(stacks, m)
		}
	}
	sort.Slice(stacks, func(i, j int) bool {
		a, b := stacks[i], stacks[j]
		atx, aty := TileIndex(a.X, tileSize), TileIndex(a.Y, tileSize)
		btx, bty := TileIndex(b.X, tileSize), TileIndex(b.Y, tileSize)
		if aty != bty {
			return aty < bty
		}
		if atx != btx {
			return atx < btx
		}
		if a.AttackPost != b.AttackPost {
			return a.AttackPost
		}
		return a.ID < b.ID
	})

	for first := 0; first < len(stacks); {
		key := attackPostTile{x: TileIndex(stacks[first].X, tileSize), y: TileIndex(stacks[first].Y, tileSize)}
		last := first + 1
		for last < len(stacks) && TileIndex(stacks[last].X, tileSize) == key.x && TileIndex(stacks[last].Y, tileSize) == key.y {
			last++
		}
		if count := last - first; count > 1 {
			for index, m := range stacks[first:last] {
				m.TransitStackIndex = index
				m.TransitStackCount = count
			}
		}
		first = last
	}
	for _, m := range gl.game.world.Monsters {
		if m == nil {
			continue
		}
		if !m.IsAlive() {
			m.TransitStackActive = false
			m.TransitStackOffsetX, m.TransitStackOffsetY = 0, 0
			continue
		}
		var targetX, targetY float64
		if m.TransitStackCount > 1 {
			if !m.TransitStackActive {
				// Continue from the calm band's fan when it becomes a combat stack.
				m.TransitStackOffsetX, m.TransitStackOffsetY = bandFanOffset(m.BandStackIndex, m.BandStackCount, tileSize)
				m.TransitStackActive = true
			}
			targetX, targetY = bandFanOffset(m.TransitStackIndex, m.TransitStackCount, tileSize)
		} else {
			targetX, targetY = bandFanOffset(m.BandStackIndex, m.BandStackCount, tileSize)
		}
		if m.TransitStackActive {
			// Exponential response is independent of TPS and settles in about
			// 0.15 seconds. This is presentation interpolation, not a move.
			blend := 1 - math.Exp(-20/float64(gl.game.config.GetTPS()))
			m.TransitStackOffsetX += (targetX - m.TransitStackOffsetX) * blend
			m.TransitStackOffsetY += (targetY - m.TransitStackOffsetY) * blend
			if math.Hypot(targetX-m.TransitStackOffsetX, targetY-m.TransitStackOffsetY) < .01 {
				m.TransitStackOffsetX, m.TransitStackOffsetY = targetX, targetY
				if m.TransitStackCount <= 1 {
					m.TransitStackActive = false
				}
			}
		}
	}
	gl.combatTransitStackBuf = stacks
}

// monsterStackFanOffset returns the one render offset used for a normal calm
// band or a temporary combat transit stack. Transit wins so a mob never gets
// two offsets when a just-scattered band shares a tile for one frame.
func monsterStackFanOffset(m *monster.Monster3D, tileSize float64) (float64, float64) {
	if m == nil {
		return 0, 0
	}
	if m.TransitStackActive {
		return m.TransitStackOffsetX, m.TransitStackOffsetY
	}
	if m.BandStackCount > 1 {
		return bandFanOffset(m.BandStackIndex, m.BandStackCount, tileSize)
	}
	return 0, 0
}

// monsterVisualStackOffset preserves lateral separation without moving the
// stack toward the camera. Corpse capture and render fallbacks share this rule.
func (g *MMGame) monsterVisualStackOffset(m *monster.Monster3D, x, y float64) (float64, float64) {
	ox, oy := monsterStackFanOffset(m, g.config.GetTileSize())
	camX, camY := g.camera.X, g.camera.Y
	if g.combat != nil {
		camX, camY = g.combat.logicalCameraXY()
	}
	dx, dy := x-camX, y-camY
	if lengthSq := dx*dx + dy*dy; lengthSq > 0 {
		if inward := (ox*dx + oy*dy) / lengthSq; inward < 0 {
			ox, oy = ox-inward*dx, oy-inward*dy
		}
	}
	if m != nil && m.TransitStackActive {
		// A walking actor can be at a tile edge in RT. Shorten the compact fan
		// along its existing direction so it cannot suggest a neighbouring tile.
		// Use the presented base point to preserve the shared TB diagonal slot.
		tile := g.config.GetTileSize()
		limit := func(position, offset float64) float64 {
			lo := float64(TileIndex(position, tile)) * tile
			if offset > 0 {
				return (math.Nextafter(lo+tile, lo) - position) / offset
			}
			if offset < 0 {
				return (lo - position) / offset
			}
			return 1
		}
		scale := max(0, min(1, limit(x, ox), limit(y, oy)))
		ox, oy = ox*scale, oy*scale
	}
	return ox, oy
}
