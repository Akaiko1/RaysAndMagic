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

// combatStackParticipant: a monster fighting right now (a target, and alert,
// pursuing or attacking). TB turn-boundary separation leaves such shared
// tiles alone: that overlap is transit toward an attack post. A tile holding
// one has a Combat fan (monster.TileFan).
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
