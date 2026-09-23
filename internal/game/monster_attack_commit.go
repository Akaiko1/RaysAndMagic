package game

import "ugataima/internal/monster"

type monsterAttackCadence uint8

const (
	monsterAttackRealtime monsterAttackCadence = iota
	monsterAttackTurn
	monsterAttackPounce
)

// A nil foe explicitly denotes the party. Foe identity remains attached to the
// attempted action, so a dead or newly friendly foe cannot turn into a party hit.
type monsterAttackDestination struct{ foe *monster.Monster3D }

// Delivery records hostile actions even when they miss, are dodged, or deal
// zero damage. Spending an action on healing or a buff is not an attack.
type monsterActionObservation struct {
	actor       *monster.Monster3D
	partyAttack bool
}

func (cs *CombatSystem) recordMonsterPartyAttack(m *monster.Monster3D) {
	if action := cs.monsterAction; action != nil && action.actor == m {
		action.partyAttack = true
	}
}

func (cs *CombatSystem) monsterAttackStillValid(m *monster.Monster3D, target monsterAttackDestination, cadence monsterAttackCadence) bool {
	if cs == nil || cs.game == nil || m == nil || !m.IsAlive() {
		return false
	}
	behavior := m.CurrentAIBehavior()
	switch behavior {
	case monster.AIBehaviorInert, monster.AIBehaviorPacified, monster.AIBehaviorEvasive,
		monster.AIBehaviorFleeing, monster.AIBehaviorPassive, monster.AIBehaviorAmbient:
		return false
	}
	if cadence == monsterAttackTurn {
		if m.StunTurnsRemaining > 0 || cs.game.turnBasedMonsterStunned[m] {
			return false
		}
	} else if m.StunFramesRemaining > 0 {
		return false
	}
	if target.foe != nil {
		if !target.foe.IsAlive() || m.AIFoe != target.foe {
			return false
		}
		if m.IsPartyControlled() {
			if !cs.boundAllyCanDamageMonster(target.foe) {
				return false
			}
		} else if !m.CanAttackActor(target.foe) {
			return false
		}
		return cs.monsterCanAttackMonster(m, target.foe)
	}
	if !m.TargetsParty() || cs.game.party == nil || len(alivePartyIndices(cs.game.party.Members)) == 0 {
		return false
	}
	x, y := cs.logicalCameraXY()
	if !cs.monsterCanAttackParty(m, Distance(m.X, m.Y, x, y), m.GetAttackRangePixels()) {
		return false
	}
	// TB party shots retain their row/column lane rule. Crossfire retains its
	// existing radial rule; ordinary ranged mobs still use adjacent melee.
	if cadence == monsterAttackTurn && !cs.monsterUsesMeleeAgainstParty(m) {
		tile := float64(cs.game.config.GetTileSize())
		if TileIndex(m.X, tile) != TileIndex(x, tile) && TileIndex(m.Y, tile) != TileIndex(y, tile) {
			return false
		}
	}
	return true
}

// commitMonsterAttack owns normal attack delivery and action clocks in both
// modes. Planners may test reach before moving, but this boundary revalidates
// current faction, target lifetime, obstruction and logical post ownership.
// Boss specials retain their separate authored action gates.
func (cs *CombatSystem) commitMonsterAttack(m *monster.Monster3D, target monsterAttackDestination, cadence monsterAttackCadence) (spent bool) {
	if !cs.monsterAttackStillValid(m, target, cadence) {
		return false
	}
	// A normal RT contender claims a post only when its action is ready.
	// Champions retain independent main/off-hand streams.
	if cadence == monsterAttackRealtime && !(m.IsChampion() && !m.Bound && (target.foe != nil || cs.monsterUsesMeleeAgainstParty(m))) {
		if m.AttackCDFrames != 0 || (target.foe == nil && m.StateTimer != 1) {
			return false
		}
	}
	if !cs.game.tryClaimMonsterAttackPost(m) {
		return false
	}
	// Observe actual delivery across all hands/hits, including replacement
	// spells. A mixed support/attack action earns only one reaction.
	previous := cs.monsterAction
	action := monsterActionObservation{actor: m}
	cs.monsterAction = &action
	defer func() {
		cs.monsterAction = previous
		if spent && target.foe == nil && action.partyAttack {
			cs.game.observeOverwatchAttack(m)
		}
	}()
	m.State = monster.StateAttacking
	if cadence == monsterAttackTurn {
		for hit := 0; hit < m.GetTurnBasedAttackCount() && cs.monsterAttackStillValid(m, target, cadence); hit++ {
			if !spent {
				cs.game.armMonsterAttackAnimation(m)
			}
			cs.deliverMonsterAttack(m, target)
			spent = true
		}
		if spent {
			cs.armMonsterRTAttackCooldowns(m)
		}
		return spent
	}
	if cadence == monsterAttackPounce {
		cs.applyMonsterMeleeDamage(m)
		cs.armMonsterRTAttackCooldowns(m)
		return true
	}
	if target.foe != nil {
		if m.IsChampion() && !m.Bound {
			return cs.championRTCrossfireStrike(m, target.foe)
		}
		if m.AttackCDFrames != 0 {
			return false
		}
	} else {
		if m.IsChampion() && cs.monsterUsesMeleeAgainstParty(m) && cs.championRTDualStrike(m, m.StateTimer == 1) {
			return true
		}
		if m.StateTimer != 1 || m.AttackCDFrames != 0 {
			return false
		}
	}
	m.AttackCDFrames = m.AttackCooldownFrames()
	cs.game.armMonsterAttackAnimation(m)
	cs.deliverMonsterAttack(m, target)
	return true
}

func (cs *CombatSystem) deliverMonsterAttack(m *monster.Monster3D, target monsterAttackDestination) {
	if target.foe == nil {
		cs.performMonsterAttackAgainstParty(m)
		return
	}
	_, _, owner := cs.monsterAttackAim(m, target)
	cs.performMonsterAttackAgainstMonster(m, target.foe, owner)
}

func (cs *CombatSystem) monsterAttackAim(m *monster.Monster3D, target monsterAttackDestination) (float64, float64, ProjectileOwner) {
	if target.foe == nil {
		x, y := cs.logicalCameraXY()
		return x, y, ProjectileOwnerMonster
	}
	owner := ProjectileOwnerMonsterAtBound
	if m.Bound {
		owner = ProjectileOwnerBoundUndead
	}
	return target.foe.X, target.foe.Y, owner
}
