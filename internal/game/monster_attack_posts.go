package game

import (
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
			desired, solid := desiredMonsterCollisionState(m)
			gl.game.applyMonsterCollisionState(m.ID, desired, solid)
		}
		if monsterHoldsAttackPost(m) {
			posts = append(posts, m)
		}
	}

	sort.Slice(posts, func(i, j int) bool {
		a, b := posts[i], posts[j]
		atx, aty := int(a.X/tileSize), int(a.Y/tileSize)
		btx, bty := int(b.X/tileSize), int(b.Y/tileSize)
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
		key := attackPostTile{x: int(winner.X / tileSize), y: int(winner.Y / tileSize)}
		desired, solid := desiredMonsterCollisionState(winner)
		gl.game.applyMonsterCollisionState(winner.ID, desired, solid)
		i++
		for i < len(posts) {
			candidate := posts[i]
			candidateKey := attackPostTile{x: int(candidate.X / tileSize), y: int(candidate.Y / tileSize)}
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

// combatStackParticipant is intentionally narrower than generic movement: only
// combatants with an active attack target share transit-stack visuals. Calm
// monsters retain normal banding, while unrelated monster-vs-monster fights do
// not visually merge merely because their paths happen to cross.
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

// updateCombatTransitVisualStacks fans co-located combatants without inventing
// a BandID. It is strictly render state: combat reads logical attack posts, and
// AoE continues to read actual positions.
func (gl *GameLoop) updateCombatTransitVisualStacks() {
	if gl == nil || gl.game == nil || gl.game.world == nil || gl.game.config == nil {
		return
	}
	tileSize := float64(gl.game.config.GetTileSize())
	if tileSize <= 0 {
		return
	}

	stacks := gl.combatTransitStackBuf[:0]
	for _, m := range gl.game.world.Monsters {
		if m == nil {
			continue
		}
		m.TransitStackIndex = 0
		m.TransitStackCount = 0
		if m.IsAlive() && combatStackParticipant(gl.game, m) {
			stacks = append(stacks, m)
		}
	}
	sort.Slice(stacks, func(i, j int) bool {
		a, b := stacks[i], stacks[j]
		atx, aty := int(a.X/tileSize), int(a.Y/tileSize)
		btx, bty := int(b.X/tileSize), int(b.Y/tileSize)
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
		key := attackPostTile{x: int(stacks[first].X / tileSize), y: int(stacks[first].Y / tileSize)}
		last := first + 1
		for last < len(stacks) && int(stacks[last].X/tileSize) == key.x && int(stacks[last].Y/tileSize) == key.y {
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
	gl.combatTransitStackBuf = stacks
}

// monsterStackFanOffset returns the one render offset used for a normal calm
// band or a temporary combat transit stack. Transit wins so a mob never gets
// two offsets when a just-scattered band shares a tile for one frame.
func monsterStackFanOffset(m *monster.Monster3D, tileSize float64) (float64, float64) {
	if m == nil {
		return 0, 0
	}
	if m.TransitStackCount > 1 {
		return bandFanOffset(m.TransitStackIndex, m.TransitStackCount, tileSize)
	}
	if m.BandStackCount > 1 {
		return bandFanOffset(m.BandStackIndex, m.BandStackCount, tileSize)
	}
	return 0, 0
}
