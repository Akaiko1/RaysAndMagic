package game

import (
	"ugataima/internal/collision"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Off-screen ecology uses detached simulation contexts, never swaps the live
// game's camera/world or requests render resources. The actors themselves are
// the same saved world entities. Only wildlife and caravan encounters advance.
func (g *MMGame) simulateRemoteEcology(turn, tickStatuses bool) {
	if g.ecologyViews == nil {
		g.ecologyViews = map[*world.World3D]*MMGame{}
	}
	for _, w := range ecologyWorlds() {
		if w == g.world {
			delete(g.ecologyViews, w)
			continue
		}
		relevant := false
		for _, m := range w.Monsters {
			if m.IsAlive() && m.IsAmbient() {
				relevant = true
				break
			}
		}
		if !relevant {
			delete(g.ecologyViews, w)
			continue
		}
		v := g.ecologyViews[w]
		if v == nil {
			v = &MMGame{config: g.config, world: w, camera: &FirstPersonCamera{X: -100000, Y: -100000}, party: g.party, ecologyOwner: g, reusableDeadSet: map[string]bool{}, reusableEncounterRewardsMap: map[*monster.EncounterRewards]int{}}
			v.collisionSystem = g.ecologyCollision(w)
			v.combat = NewCombatSystem(v)
			v.gameLoop = &GameLoop{game: v}
			v.threading = g.threading
			v.questManager = g.questManager
			g.ecologyViews[w] = v
		}
		v.party = g.party
		if tickStatuses {
			v.turnBasedMonsterStunned = map[*monster.Monster3D]bool{}
		}
		v.ecology = g.ecology
		v.frameCount = g.frameCount
		v.turnBasedMode = turn
		v.calendarDay = g.calendarDay
		v.calendarWeek = g.calendarWeek
		v.calendarMonth = g.calendarMonth
		// Refresh changed spawn rosters and positions without a separate entity list.
		v.syncEcologyCollisionRoster()
		v.refreshMonsterAIState()
		v.gameLoop.reconcileMonsterAttackPosts()
		if !turn {
			v.moveRemoteEcologyRealtime()
		}
		for _, m := range w.Monsters {
			if !remoteEcologyActor(m) {
				continue
			}
			if turn {
				if v.turnBasedMonsterStunned[m] || v.tickMonsterTurnStatuses(m, tickStatuses) {
					v.turnBasedMonsterStunned[m] = true
					continue
				}
				if !m.IsAlive() {
					v.combat.finishMonsterKill(m)
					continue
				}
				if m.CurrentAIBehavior() == monster.AIBehaviorAmbient {
					m.UpdateAmbient(v.collisionSystem, m.AITargetX, m.AITargetY, true)
				} else if m.IsBoss() && v.combat.runBossSpecials(m, true, true) {
					v.combat.armMonsterRTAttackCooldowns(m)
				} else if !v.combat.commitMonsterAttack(m, monsterAttackDestination{foe: m.AIFoe}, monsterAttackTurn) {
					v.gameLoop.monsterMoveTurnBased(m)
				}
			} else {

				v.combat.handleMonsterInteraction(m)
			}
			v.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
			if !m.IsAlive() {
				v.combat.finishMonsterKill(m)
			}
		}
		g.flushRemoteEcologyDeaths(v)
	}
}

func (g *MMGame) syncEcologyCollisionRoster() {
	if g.ecologyRosterIDs == nil {
		g.ecologyRosterIDs = map[string]bool{}
	}
	for id := range g.ecologyRosterIDs {
		g.ecologyRosterIDs[id] = false
	}
	for _, m := range g.world.Monsters {
		if !m.IsAlive() {
			continue
		}
		if g.collisionSystem.GetEntityByID(m.ID) == nil {
			w, h := m.GetSize()
			g.collisionSystem.RegisterEntity(collision.NewEntity(m.ID, m.X, m.Y, w, h, desiredMonsterCollisionType(m), false))
		} else {
			g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
		}
		g.ecologyRosterIDs[m.ID] = true
	}
	for id, alive := range g.ecologyRosterIDs {
		if !alive {
			g.collisionSystem.UnregisterEntity(id)
			delete(g.ecologyRosterIDs, id)
		}
	}
}

// Rewards belong to the campaign even when the death happened off-screen.
func (g *MMGame) rewardOwner() *MMGame {
	if g.ecologyOwner != nil {
		return g.ecologyOwner
	}
	return g
}

func remoteEcologyActor(m *monster.Monster3D) bool {
	return m.IsAlive() && !m.IsPartyControlled() && (m.IsAmbient() || (m.AIFoe != nil && m.AIFoe.Disposition == "caravan"))
}

// Publish all actor motion before serial post arbitration and attack delivery,
// just like the foreground scheduler. The one snapshot is shared by the batch.
func (g *MMGame) moveRemoteEcologyRealtime() {
	snapshot := g.collisionSystem.Snapshot()
	wrappers := make([]*MonsterWrapper, 0, 12)
	for _, m := range g.world.Monsters {
		if !remoteEcologyActor(m) {
			continue
		}
		wrapper := CreateMonsterWrapper(m, g.collisionSystem, snapshot, g).(*MonsterWrapper)
		wrapper.Update()
		wrappers = append(wrappers, wrapper)
	}
	for _, wrapper := range wrappers {
		wrapper.ApplyCollisionUpdate()
	}
	g.gameLoop.reconcileMonsterAttackPosts()
	g.gameLoop.finalizeIndirectKills()
}

// Projectiles use the world-frame clock in BOTH RT and TB. Actor decisions use
// turns in TB, but an arrow already in flight must keep moving on party turns.
func (g *MMGame) advanceRemoteEcologyProjectiles() {
	for w, v := range g.ecologyViews {
		if w == g.world || v.threading == nil {
			continue
		}
		v.frameCount = g.frameCount
		v.ecology = g.ecology
		v.gameLoop.updateProjectilesAndImpacts()
		g.flushRemoteEcologyDeaths(v)
	}
}

func (g *MMGame) flushRemoteEcologyDeaths(v *MMGame) {
	if v.ecology.RespawnDay > 0 {
		g.ecology.RespawnDay = v.ecology.RespawnDay
	}
	if len(v.deadMonsterIDs) > 0 {
		v.gameLoop.removeDeadMonstersByID()
	}
}
