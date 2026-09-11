package game

import (
	"fmt"
	"os"
	"strings"

	"ugataima/internal/collision"
	"ugataima/internal/monster"
)

// monsterFrameContext contains the only game-level values a movement worker
// may observe. The collision snapshot and actor-owned AI target are prepared
// before dispatch. No worker retains an MMGame back-pointer.
type monsterFrameContext struct {
	partyX, partyY float64
	hasParty       bool
	tick           int64
}

func (g *MMGame) monsterFrameContext() monsterFrameContext {
	if g == nil {
		return monsterFrameContext{}
	}
	frame := monsterFrameContext{tick: g.frameCount}
	if g.camera != nil {
		frame.partyX, frame.partyY, frame.hasParty = g.camera.X, g.camera.Y, true
	}
	return frame
}

type monsterAttackPostState struct {
	held, transit bool
	targetID      string
	since         int64
}

func monsterAttackPostFor(m *monster.Monster3D, targetID string, hasTarget bool, tick int64) monsterAttackPostState {
	if m == nil || !hasTarget {
		return monsterAttackPostState{}
	}
	post := monsterAttackPostState{m.AttackPost, m.AttackTransit, m.AttackPostTargetID, m.AttackPostSince}
	if m.State == monster.StateAttacking {
		if !post.held || post.targetID != targetID {
			post.held, post.targetID, post.since = true, targetID, tick
		}
		post.transit = false
		return post
	}
	if m.State != monster.StateAlert || !post.held || post.targetID != targetID {
		return monsterAttackPostState{}
	}
	return post
}

func (post monsterAttackPostState) apply(m *monster.Monster3D) {
	if m == nil {
		return
	}
	m.AttackPost, m.AttackTransit = post.held, post.transit
	m.AttackPostTargetID, m.AttackPostSince = post.targetID, post.since
}

type monsterFrameResult struct {
	ready            bool
	oldX, oldY, x, y float64
	post             monsterAttackPostState
	collisionType    collision.CollisionType
}

// MonsterWrapper implements the two-phase movement contract. Update changes
// actor-local AI/path/status state and stages movement/post publication.
// ApplyCollisionUpdate publishes the result once, after all workers finish.
type MonsterWrapper struct {
	Monster         *monster.Monster3D
	collisionSystem *collision.CollisionSystem
	snapshot        *collision.CollisionSnapshot
	frame           monsterFrameContext
	result          monsterFrameResult
}

var debugMonsterFilter = strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG_MONSTER")))

func (mw *MonsterWrapper) Update() {
	mw.result = monsterFrameResult{}
	if !mw.IsAlive() {
		return
	}
	m := mw.Monster
	oldX, oldY := m.X, m.Y
	targetX, targetY := m.AITargetX, m.AITargetY
	if m.LootGuarding {
		targetX, targetY = mw.frame.partyX, mw.frame.partyY
	}
	m.UpdateWithTarget(mw.snapshot, mw.frame.partyX, mw.frame.partyY, targetX, targetY)
	targetID, _, _, hasTarget := monsterAttackTargetAt(m, mw.frame.partyX, mw.frame.partyY, mw.frame.hasParty)
	post := monsterAttackPostFor(m, targetID, hasTarget, mw.frame.tick)
	marker := collision.CollisionTypeMonster
	if post.held && post.targetID != "" {
		marker = collision.CollisionTypeMonsterEngaged
	}
	mw.result = monsterFrameResult{true, oldX, oldY, m.X, m.Y, post, marker}
	// No other actor observes movement until serial publication. AI scratch is
	// still owned by this actor, so the next tick can reuse its path unchanged.
	m.X, m.Y = oldX, oldY
}

func (mw *MonsterWrapper) ApplyCollisionUpdate() {
	if !mw.result.ready || mw.Monster == nil {
		return
	}
	mw.result.ready = false
	mw.Monster.X, mw.Monster.Y = mw.result.x, mw.result.y
	mw.result.post.apply(mw.Monster)
	if mw.collisionSystem != nil {
		mw.collisionSystem.UpdateEntity(mw.Monster.ID, mw.result.x, mw.result.y)
		applyMonsterCollisionTypeTo(mw.collisionSystem, mw.Monster.ID, mw.result.collisionType)
	}
	mw.traceMovement()
}

func (mw *MonsterWrapper) IsAlive() bool                   { return mw.Monster != nil && mw.Monster.IsAlive() }
func (mw *MonsterWrapper) GetPosition() (float64, float64) { return mw.Monster.X, mw.Monster.Y }
func (mw *MonsterWrapper) SetPosition(x, y float64)        { mw.Monster.X, mw.Monster.Y = x, y }

// Debug probes read the live system only during serial publication.
func (mw *MonsterWrapper) traceMovement() {
	oldX, oldY, newX, newY := mw.result.oldX, mw.result.oldY, mw.result.x, mw.result.y
	playerX, playerY := mw.frame.partyX, mw.frame.partyY
	// Temporary movement debug (opt-in via env var).
	// Example: DEBUG_MONSTER=bandit
	if debugMonsterFilter != "" {
		name := strings.ToLower(mw.Monster.Name)
		if strings.Contains(name, debugMonsterFilter) {
			// Throttle logs to avoid spamming.
			if mw.Monster.StateTimer%60 == 0 {
				withinTether := mw.Monster.IsWithinTetherRadius()
				fmt.Printf(
					"[MONDBG] name=%q id=%s state=%d timer=%d engaging=%v withinTether=%v pos=(%.1f,%.1f) old=(%.1f,%.1f) spawn=(%.1f,%.1f) tether=%.1f player=(%.1f,%.1f)\n",
					mw.Monster.Name,
					mw.Monster.ID,
					mw.Monster.State,
					mw.Monster.StateTimer,
					mw.Monster.IsEngagingPlayer,
					withinTether,
					newX,
					newY,
					oldX,
					oldY,
					mw.Monster.SpawnX,
					mw.Monster.SpawnY,
					mw.Monster.TetherRadius,
					playerX,
					playerY,
				)

				// If not moving while supposed to wander, probe cardinal target tile centers.
				if mw.collisionSystem != nil && (mw.Monster.State == monster.StateIdle || mw.Monster.State == monster.StatePatrolling) {
					if oldX == newX && oldY == newY {
						const tileSize = 64.0
						centerX := TileCenter(newX, tileSize)
						centerY := TileCenter(newY, tileSize)

						fmt.Printf("[MONDBG] center=(%.1f,%.1f) last=(%.1f,%.1f) stuck=%d lastChosenDir=%.3f\n",
							centerX, centerY,
							mw.Monster.LastX, mw.Monster.LastY,
							mw.Monster.StuckCounter,
							mw.Monster.LastChosenDir,
						)

						targets := []struct {
							label string
							dx    float64
							dy    float64
						}{
							{label: "E", dx: tileSize, dy: 0},
							{label: "S", dx: 0, dy: tileSize},
							{label: "W", dx: -tileSize, dy: 0},
							{label: "N", dx: 0, dy: -tileSize},
						}

						for _, t := range targets {
							x := centerX + t.dx
							y := centerY + t.dy
							ok, reason := mw.collisionSystem.DebugCanMoveTo(mw.Monster.ID, x, y)
							fmt.Printf("[MONDBG] step %s -> (%.1f,%.1f) ok=%v reason=%s withinTether=%v\n",
								t.label,
								x,
								y,
								ok,
								reason,
								mw.Monster.CanMoveWithinTether(x, y),
							)
						}
					}
				}
			}
		}
	}

}
