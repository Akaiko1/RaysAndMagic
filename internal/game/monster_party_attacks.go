package game

import (
	"math/rand"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	monsterPkg "ugataima/internal/monster"
)

// tryMonsterPartyAttack rolls the monster's party attacks of one trigger in
// authored order (abilities.party_attacks); the first that fires replaces
// the monster's action. A nova rolls only while the party is in its reach.
func (cs *CombatSystem) tryMonsterPartyAttack(m *monsterPkg.Monster3D, trigger monsterPkg.PartyAttackTrigger) bool {
	for i := range m.Abilities.PartyAttacks {
		a := &m.Abilities.PartyAttacks[i]
		if a.Trigger != trigger || (a.RangeTiles > 0 && !cs.partyWithinTiles(m, a.RangeTiles)) {
			continue
		}
		if rand.Float64() < a.Chance && cs.castMonsterPartyAttack(m, a) {
			return true
		}
	}
	return false
}

// partyWithinTiles reports the party within reach of an area attack cast
// from the monster's own tile.
func (cs *CombatSystem) partyWithinTiles(m *monsterPkg.Monster3D, tiles float64) bool {
	if cs.attackOriginBlocked(m.X, m.Y) {
		return false
	}
	return Distance(cs.game.camera.X, cs.game.camera.Y, m.X, m.Y) <= tiles*float64(cs.game.config.GetTileSize())
}

// castMonsterPartyAttack lands one party attack as one monster action and
// one party damage event (one ward decision): each target takes its own
// damage roll and its own dodge. A fire spell burns the cards it strikes.
func (cs *CombatSystem) castMonsterPartyAttack(m *monsterPkg.Monster3D, a *monsterPkg.PartyAttack) bool {
	var targets []*character.MMCharacter
	if !a.Targets.All {
		targets = cs.randomLivingMembers(a.Targets.Count)
	} else {
		for _, member := range cs.game.party.Members {
			if member != nil && member.HitPoints > 0 {
				targets = append(targets, member)
			}
		}
	}
	if len(targets) == 0 {
		return false
	}
	defer cs.partyDamageEvent()()
	school, name := a.School.String(), a.Name
	cs.monsterActionScope(m, nil, func() bool {
		phrase := logAbility(name)
		if a.Spell() {
			phrase = logSchoolWord(school, name)
			cs.game.playMonsterSchoolSound(school, true, m)
		} else if weaponDef, ok := config.GetWeaponDefinition(m.ProjectileWeapon); ok {
			cs.game.playMonsterRangedWeaponAttackSound(weaponDef, m)
		}
		cs.game.logCombatLine(logToneBad, uitext.Text("combat.party_attack.cast", logSlot, logSlot), logMonsterName(m), phrase)
		source := uitext.Text("combat.party_attack.source", m.Name, name)
		flame := a.Spell() && a.School == damagecalc.Fire
		for _, member := range targets {
			hit := cs.hitFromMonster(m, partyAttackDamage(m, a), school, a.IgnoreArmor, 0, false, a.Spell())
			hit.Projectile = !a.Spell()
			hit.NoRiders = !a.Riders.Carried()
			if flame {
				hit.Flame, hit.LogKind = true, damageLogScorch
			}
			cs.monsterHitCharacter(m, member, source, hit)
		}
		return true
	})
	return true
}

// partyAttackDamage rolls one member's damage from m's current attack band.
func partyAttackDamage(m *monsterPkg.Monster3D, a *monsterPkg.PartyAttack) int {
	lo, hi := a.DamageBand(m.AttackBand())
	return lo + rand.Intn(hi-lo+1)
}

// tryBossRangedNova rolls a boss's novas from outside its attack moment once
// NovaRollCD has run out; the attempt arms the interval, so a failed roll is
// not retried every frame.
func (cs *CombatSystem) tryBossRangedNova(m *monsterPkg.Monster3D) bool {
	if m.NovaRollCD.Active() || cs.bossDisabled(m) || !cs.anyNovaInReach(m) {
		return false
	}
	cs.armBossNovaRoll(m)
	return cs.tryMonsterPartyAttack(m, monsterPkg.PartyAttackNova)
}

func (cs *CombatSystem) anyNovaInReach(m *monsterPkg.Monster3D) bool {
	for _, a := range m.Abilities.PartyAttacks {
		if a.Trigger == monsterPkg.PartyAttackNova && cs.partyWithinTiles(m, a.RangeTiles) {
			return true
		}
	}
	return false
}

// armBossNovaRoll starts the gap between at-range nova rolls. It is armed
// after every nova too, so one at the attack moment is not followed at once
// by a ranged one.
func (cs *CombatSystem) armBossNovaRoll(m *monsterPkg.Monster3D) {
	m.NovaRollCD.Arm(BossNovaRangedRollSeconds, cs.game.config.GetTPS())
}
