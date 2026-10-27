package game

import (
	uitext "ugataima/assets/text"
	"ugataima/internal/monster"
)

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
	foe         *monster.Monster3D // the action's monster target; nil = the party
	partyAttack bool
	actorAttack bool
}

// monsterActionScope runs one monster action - a committed attack, a boss
// special, a telegraph release, a trap volley. Its hits record what they
// attacked, and the action is answered once when it ends: Overwatch reacts to
// an attack on the party, the caravan alarm to an attack on a caravan. Hits of
// an action already open for this actor join it.
func (cs *CombatSystem) monsterActionScope(m, foe *monster.Monster3D, run func() bool) bool {
	if a := cs.monsterAction; a != nil && a.actor == m {
		return run()
	}
	previous := cs.monsterAction
	action := monsterActionObservation{actor: m, foe: foe}
	cs.monsterAction = &action
	defer func() { cs.monsterAction = previous }()
	spent := run()
	if spent && foe != nil && action.actorAttack {
		cs.game.notifyCaravanAttack(foe)
	}
	if spent && foe == nil && action.partyAttack {
		cs.game.observeOverwatchAttack(m)
	}
	return spent
}

func (cs *CombatSystem) recordMonsterActorAttack(m *monster.Monster3D) {
	if action := cs.monsterAction; action != nil && action.actor == m {
		action.actorAttack = true
	}
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
	if !m.CurrentAIBehavior().Caps().MayAttack {
		return false
	}
	if cadence == monsterAttackTurn {
		if m.Stun.Turns > 0 || cs.game.turnBasedMonsterStunned[m] {
			return false
		}
	} else if m.Stun.Frames > 0 {
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
	if !m.CanPursueParty(cs.logicalCameraXY()) {
		return false
	}
	return cs.partyInStrikeReach(m, cadence == monsterAttackTurn)
}

// commitMonsterAttack owns normal attack delivery and action clocks in both
// modes. Planners may test reach before moving, but this boundary revalidates
// current faction, target lifetime, obstruction and logical post ownership.
// Boss specials retain their separate authored action gates.
func (cs *CombatSystem) commitMonsterAttack(m *monster.Monster3D, target monsterAttackDestination, cadence monsterAttackCadence) bool {
	if !cs.monsterAttackStillValid(m, target, cadence) {
		return false
	}
	// Champions own independently timed hands; ordinary attacks have one clock.
	independentHands := m.IsChampion() && !m.Bound && (target.foe != nil || cs.monsterUsesMeleeAgainstParty(m))
	actionReady := cadence != monsterAttackRealtime || (m.AttackCDFrames == 0 && (target.foe != nil || m.StateTimer == 1))
	if !actionReady && !independentHands {
		return false
	}
	if !cs.game.tryClaimMonsterAttackPost(m) {
		return false
	}
	// Observe actual delivery across all hands/hits, including replacement
	// spells. A mixed support/attack action earns only one reaction.
	return cs.monsterActionScope(m, target.foe, func() bool {
		return cs.deliverCommittedAttack(m, target, cadence, actionReady)
	})
}

// deliverCommittedAttack is the body of a committed attack: the jam check, the
// party abilities, then the swings of the cadence.
func (cs *CombatSystem) deliverCommittedAttack(m *monster.Monster3D, target monsterAttackDestination, cadence monsterAttackCadence, actionReady bool) (spent bool) {
	m.State = monster.StateAttacking
	if cadence == monsterAttackTurn && m.PeekTurnBasedSwings() == 0 {
		// Jams took a whole action's swings (Cogfang Blade): the action is
		// spent with no attack and no ability roll.
		m.TakeTurnBasedSwings()
		cs.armMonsterRTAttackCooldowns(m)
		cs.game.logCombatLine(logToneGood, uitext.Text("combat.s_works_jam_its_attack_is", logSlot), logMonsterName(m))
		return true
	}
	if target.foe == nil && actionReady {
		cs.tryMonsterPartyAbilities(m, cadence)
	}
	if cadence == monsterAttackTurn {
		// Swings per action are fractional and accumulate (turn_based_cadence.go).
		// They are drawn on the first swing, so an attack revoked before it lands
		// keeps the banked fraction; later swings stop if the target becomes invalid.
		swings := 1
		for hit := 0; hit < swings && cs.monsterAttackStillValid(m, target, cadence); hit++ {
			if hit == 0 {
				swings = m.TakeTurnBasedSwings()
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
	} else {
		if m.IsChampion() && cs.monsterUsesMeleeAgainstParty(m) {
			if handled, struck := cs.championRTDualStrike(m, m.StateTimer == 1); handled {
				return struck
			}
		}
		if !actionReady {
			return false
		}
	}
	m.ArmAttackCooldown(m.AttackCooldownFrames(), cs.game.turnBasedMode)
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
	owner := ProjectileOwnerMonsterAtActor
	if m.Bound {
		owner = ProjectileOwnerPartyAlly
	}
	return target.foe.X, target.foe.Y, owner
}
