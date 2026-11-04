package game

import (
	"math"
	"math/rand"

	uitext "ugataima/assets/text"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/status"
)

// Boss behaviour is explicitly authored with boss: true in monsters.yaml.
// Mechanics such as blink, Inferno, summoning, and enrage remain optional;
// this file owns only their shared RT/TB sequencing.

// bossDisabled reports whether crowd control should suppress a boss action:
// stun (either mode), charm, or bind. The RT/TB monster loops already skip
// disabled monsters before reaching updateBoss; this guards the few paths that
// don't (and documents intent).
func (cs *CombatSystem) bossDisabled(m *monsterPkg.Monster3D) bool {
	return m.Stun.Active() || m.Pacified || m.Bound
}

// bossMayAct reports a boss free to use its kit: not disabled, silenced or
// frightened. Every boss special and the answer to a blow check it.
func (cs *CombatSystem) bossMayAct(m *monsterPkg.Monster3D) bool {
	return !cs.bossDisabled(m) && !m.IsSilenced() && !m.IsAfraid()
}

// bossEvasive reports whether the boss is in its evasive phase: it has a
// PassiveUntilQuest gate and that quest is NOT yet completed. While evasive it
// never attacks or chases - it only blinks away when the party closes in.
func (cs *CombatSystem) bossEvasive(m *monsterPkg.Monster3D) bool {
	if m == nil || !m.IsBoss() || m.QuestProgressIgnored || m.PassiveUntilQuest == "" {
		return false
	}
	return !cs.game.questManager.ObjectiveMet(m.PassiveUntilQuest)
}

// runBossSpecials is the ONE boss rider for every combat action: RT and TB,
// party fight and summon fight alike. Reads the special cadences (BossCD, the
// at-range nova cooldown; Monster3D.TickAbilityClocks spends them), owns the
// trap field, the timed summon and updateBoss; true = a special consumed the
// action. Bosses previously lost the WHOLE kit whenever a summon out-competed
// the party for aggro. A party blow's summon roll is no action: answerBossHurt.
func (cs *CombatSystem) runBossSpecials(m *monsterPkg.Monster3D, attackTick, turnBased bool) bool {
	if m == nil || !m.IsBoss() || !cs.bossMayAct(m) {
		return false
	}
	cs.updateChamberBossReach(m)
	cs.tryBossTrapVolley(m)
	if cs.tryBossTimedSummon(m) {
		if !turnBased {
			cs.armMonsterRTAttackCooldowns(m) // the summon spends the boss's next attack
		}
		return true
	}
	return cs.updateBoss(m, attackTick)
}

// bossActionTick is the boss's RT once-per-attack moment against whatever it
// fights: contact with the party, or the reach tick on a summon it was lured
// onto. TB passes the tick directly (one turn is one action).
func (cs *CombatSystem) bossActionTick(m *monsterPkg.Monster3D) bool {
	if m == nil || m.AttackCDFrames != 0 {
		return false
	}
	if foe := m.AIFoe; foe != nil && foe.IsAlive() {
		return cs.monsterCanAttackMonster(m, foe)
	}
	// Same reach gate as the normal attack: an adjacent melee boss is in contact
	// at >1 tile of pixel distance.
	return m.State == monsterPkg.StateAttacking && m.StateTimer == 1 && cs.partyInStrikeReach(m, false)
}

// updateBoss runs the boss's special behaviour. BossCD gates the evasive blink.
// `attackTick` marks the once-per-attack moment when an aggressive boss may
// summon (once a round), blink (low HP) or cast a nova; between attacks a nova
// may roll AT RANGE once NovaRollCD has run out. Returns true when it handled the monster's action
// this tick (caller skips the normal attack); false lets the normal
// melee/ranged attack proceed (which can melt armor: abilities.melt_armor).
func (cs *CombatSystem) updateBoss(m *monsterPkg.Monster3D, attackTick bool) bool {
	if m == nil || !m.IsBoss() || !cs.bossMayAct(m) {
		return false
	}

	if cs.bossEvasive(m) {
		// Quest unfinished -> the boss never attacks. An evasive boss
		// (evade_radius_tiles set) blinks away when crowded or hit; a dormant boss
		// (no evade radius - e.g. the sealed Samurai Warlord) just holds its ground
		// until the quest completes, then turns aggressive.
		if m.EvadeRadiusTiles > 0 {
			near := Distance(cs.game.camera.X, cs.game.camera.Y, m.X, m.Y) <= m.EvadeRadiusTiles*float64(cs.game.config.GetTileSize())
			if !m.BossCD.Active() && (near || m.BossHurtPending) && cs.blinkMonsterRandom(m) {
				tps := cs.game.config.GetTPS()
				m.BossCD.ArmSeconds(m.BossCooldownSecs, tps)
				m.BossHurtPending = false
				cs.game.AddCombatMessage(uitext.Text("combat.skitters_away_into_the_dark", m.Name))
			}
		}
		return true
	}
	// Aggressive: it owes no blink, also none left from its evasive phase.
	m.BossHurtPending = false
	// Enrage is a passive HP-threshold state announced once here; its
	// mechanical effect (harder/faster hits) is applied live in
	// GetAttackDamage/AttackCooldownFrames, so it is save-safe.
	if m.Abilities.Enrage.AtHP > 0 && !m.Enraged && m.IsEnraged() {
		m.Enraged = true
		cs.game.logCombatLine(logToneBad, uitext.Text("combat.flies_into_a", logSlot, logSlot), logMonsterName(m), logKeyword("damage", "furious rage"))
	}
	// The low-HP blink still fires only at the melee attack moment. A nova
	// (party_attacks trigger nova) may also roll from its reach between attacks.
	if !attackTick {
		return cs.tryBossRangedNova(m)
	}
	// The attack moment's summon roll, once a round, takes the attack's place.
	if cs.claimSummonRoll(m, &m.AttackSummonCD) && cs.rollBossSummon(m) {
		return true
	}
	if m.Abilities.Teleport.AtHP > 0 && m.HitPoints <= m.Abilities.Teleport.AtHP && rand.Float64() < m.Abilities.Teleport.Chance {
		if cs.blinkMonsterRandom(m) {
			cs.game.logCombatLine(logToneBad, uitext.Text("combat.in_a_golden_flash", logSlot, logSlot), logMonsterName(m), logAbility("blinks away"))
			return true
		}
	}
	// Attack moment: a nova can replace the normal hit while the party is
	// inside its authored reach.
	if cs.tryMonsterPartyAttack(m, monsterPkg.PartyAttackNova) {
		cs.armBossNovaRoll(m)
		return true
	}
	return false // proceed to the normal attack (it may melt armor: abilities.melt_armor)
}

// rollsSummon reports a boss whose summon rolls (chance, first_guaranteed)
// and is below its live cap. A timed summon is tryBossTimedSummon's.
func (cs *CombatSystem) rollsSummon(m *monsterPkg.Monster3D) bool {
	s := m.Abilities.Summon
	return len(s.Monsters) > 0 && s.CooldownSeconds <= 0 && !cs.summonsFull(m)
}

// claimSummonRoll takes the round's summon roll on clock (the attack's
// AttackSummonCD or the blow's HurtSummonCD): none while it runs or at the
// live cap. The clock starts with the claim, whatever the roll brings.
func (cs *CombatSystem) claimSummonRoll(m *monsterPkg.Monster3D, clock *status.DualClock) bool {
	if clock.Active() || !cs.rollsSummon(m) {
		return false
	}
	clock.Arm(config.CombatRoundSeconds, cs.game.config.GetTPS())
	return true
}

// rollBossSummon is one summon roll: sure while the first guaranteed summon
// is owed, else the chance. True when adds arrived.
func (cs *CombatSystem) rollBossSummon(m *monsterPkg.Monster3D) bool {
	s := m.Abilities.Summon
	due := (s.FirstGuaranteed && !m.SummonFirstDone) || (s.Chance > 0 && rand.Float64() < s.Chance)
	if !due || !cs.summonBossAdds(m) {
		return false
	}
	m.SummonFirstDone = true
	return true
}

// noteBossBlow takes, at the moment a hero's attack takes HP off an aggressive
// boss, that round's blow roll (HurtSummonCD: from each party turn in TB, 3 s
// in RT) for the end of the frame (BossBlowRolls, answerBossHurt). A blow the
// boss cannot answer then (crowd control, silence, fear, the roll spent, the
// live cap) is not kept for later. An evasive boss's blink answers any HP
// loss instead (BossHurtPending, set where HP drops).
func (cs *CombatSystem) noteBossBlow(m *monsterPkg.Monster3D) {
	if m.IsBoss() && !cs.bossEvasive(m) && cs.bossAnswersBlows(m) && cs.claimSummonRoll(m, &m.HurtSummonCD) {
		m.BossBlowRolls++
	}
}

// bossAnswersBlows reports a boss alive and able to act on a blow.
func (cs *CombatSystem) bossAnswersBlows(m *monsterPkg.Monster3D) bool {
	return m.IsAlive() && cs.bossMayAct(m) && m.CurrentAIBehavior().Caps().MayAttack
}

// answerBossHurt rolls, once a frame in both modes, the summon each blow
// claimed (noteBossBlow), so the adds join after this frame's combat. The roll
// is a reaction: it spends no action and replaces no attack. A boss the
// frame left dead or unable to act summons nothing.
func (cs *CombatSystem) answerBossHurt() {
	w := cs.game.GetCurrentWorld()
	if w == nil {
		return
	}
	for _, m := range w.Monsters {
		if m == nil || m.BossBlowRolls == 0 {
			continue
		}
		rolls := m.BossBlowRolls
		m.BossBlowRolls = 0
		for ; rolls > 0 && cs.bossAnswersBlows(m); rolls-- {
			cs.rollBossSummon(m)
		}
	}
}

// tryBossTimedSummon calls the adds each time summon.cooldown_seconds runs
// out, when the live cap allows; the timer restarts either way. It does not
// wait for an attack moment, so kiting cannot stall it.
func (cs *CombatSystem) tryBossTimedSummon(m *monsterPkg.Monster3D) bool {
	if m.Abilities.Summon.CooldownSeconds <= 0 || m.SummonCD.Active() || cs.bossEvasive(m) {
		return false
	}
	m.SummonCD.Arm(m.Abilities.Summon.CooldownSeconds, cs.game.config.GetTPS())
	return !cs.summonsFull(m) && cs.summonBossAdds(m)
}

// summonsFull reports a boss at its live summon cap.
func (cs *CombatSystem) summonsFull(m *monsterPkg.Monster3D) bool {
	return m.Abilities.Summon.Max > 0 && cs.countLiveSummons(m) >= m.Abilities.Summon.Max
}

// summonBossAdds rallies the boss's adds (war-banner): up to SummonCount monsters
// from SummonMonsters spawned ~2 tiles out on walkable tiles inside the boss's
// movement bounds, hostile on arrival and at the boss's auto-level. Honours
// SummonMax (live summons of THIS boss). Returns true if any spawned. Adds give
// no XP but drop their own gold and loot (killExperience): intended.
func (cs *CombatSystem) summonBossAdds(m *monsterPkg.Monster3D) bool {
	if len(m.Abilities.Summon.Monsters) == 0 {
		return false
	}
	liveSummons := cs.countLiveSummons(m)
	if m.Abilities.Summon.Max > 0 && liveSummons >= m.Abilities.Summon.Max {
		return false
	}
	n := m.Abilities.Summon.Count
	if n < 1 {
		n = 1
	}
	if m.Abilities.Summon.Max > 0 && liveSummons+n > m.Abilities.Summon.Max {
		n = m.Abilities.Summon.Max - liveSummons
	}
	tile := float64(cs.game.config.GetTileSize())
	// Adds appear where the boss may move (a chamber arena).
	var bounds collision.MovementBounds
	if e := cs.game.collisionSystem.GetEntityByID(m.ID); e != nil {
		bounds = e.MovementBounds()
	}
	spawned := 0
	maxAttempts := n * 12
	if maxAttempts < 12 {
		maxAttempts = 12
	}
	for attempts := 0; spawned < n && attempts < maxAttempts; attempts++ {
		key := m.Abilities.Summon.Monsters[rand.Intn(len(m.Abilities.Summon.Monsters))]
		angle := rand.Float64() * 2 * math.Pi
		tx := m.X + math.Cos(angle)*2*tile
		ty := m.Y + math.Sin(angle)*2*tile
		sx, sy, ok := cs.findNearestSummonTileWithin(tx, ty, 10, bounds)
		if !ok {
			continue
		}
		add := monsterPkg.NewMonster3DFromConfig(sx, sy, key, cs.game.config)
		// An auto-levelled boss calls adds of its own level.
		scaleAdventureMonster(cs.game.config.AdventureScaling, add, m.AdventureScaleLevel)
		add.WasAttacked = true
		add.BeginPlayerEngagement() // summons wake hostile
		add.SummonedBy = m.ID
		add.HomeMap = m.HomeMap
		add.QuestProgressIgnored = true // runtime summons never count toward map-clear quests
		cs.game.registerSpawnedMonster(add)
		cs.game.refreshMonsterCollisionState(add)
		spawned++
	}
	if spawned == 0 {
		return false
	}
	cs.game.logCombatLine(logToneBad, uitext.Text("combat.raises_the_retainers_rush_to_its", logSlot, logSlot), logMonsterName(m), logAbility("war-banner"))
	return true
}

func (cs *CombatSystem) findNearestSummonTile(targetX, targetY float64, maxRadius int) (float64, float64, bool) {
	return cs.findNearestSummonTileWithin(targetX, targetY, maxRadius, collision.MovementBounds{})
}

// findNearestSummonTileWithin also keeps the tile centre inside bounds.
func (cs *CombatSystem) findNearestSummonTileWithin(targetX, targetY float64, maxRadius int, bounds collision.MovementBounds) (float64, float64, bool) {
	tile := float64(cs.game.config.GetTileSize())
	x, y := cs.game.findNearestWalkableTileWithMaxRadius(targetX, targetY, maxRadius, func(tx, ty int) bool {
		x, y := TileCenterFromTile(tx, ty, tile)
		return bounds.Allows(collision.NewBoundingBox(x, y, 0, 0)) && !cs.summonSpawnOccupied(x, y)
	})
	return x, y, x >= 0
}

func (cs *CombatSystem) summonSpawnOccupied(x, y float64) bool {
	tile := float64(cs.game.config.GetTileSize())
	tx, ty := TileIndex(x, tile), TileIndex(y, tile)
	ptx, pty := cs.game.GetPlayerTilePosition()
	if tx == ptx && ty == pty {
		return true
	}
	w := cs.game.GetCurrentWorld()
	if w == nil {
		return false
	}
	for _, o := range w.Monsters {
		if o == nil || !o.IsAlive() {
			continue
		}
		if TileIndex(o.X, tile) == tx && TileIndex(o.Y, tile) == ty {
			return true
		}
	}
	return false
}

// countLiveSummons counts living monsters this boss has summoned (for SummonMax).
func (cs *CombatSystem) countLiveSummons(m *monsterPkg.Monster3D) int {
	if m == nil {
		return 0
	}
	return cs.countLiveSummonsByOwner(m.ID)
}

// tickEvasiveBossesTB runs the evasive-phase reaction every frame in turn-based
// mode. Without it the hurt-blink waits for the monster turn, so a full party
// round of focused hits could kill the boss before it ever dodged. BossCD's TB
// round starts with the party turn (TickHurtReactionTurn). Aggressive-phase
// specials still fire only on the boss's own TB turn.
//
// DELIBERATELY the one direct updateBoss caller: the rider's other work is
// dead weight here (an evasive boss sows no traps and updateBoss returns
// before reading attackTick).
func (cs *CombatSystem) tickEvasiveBossesTB() {
	w := cs.game.GetCurrentWorld()
	if w == nil {
		return
	}
	for _, m := range w.Monsters {
		if m == nil || !m.IsAlive() || !m.IsBoss() || !cs.bossEvasive(m) {
			continue
		}
		// Crowd control suppresses the blink like any other action.
		if cs.bossDisabled(m) {
			continue
		}
		cs.updateBoss(m, false)
	}
}

// blinkMonsterRandom teleports the monster to a random walkable tile of the
// current map other than its own (re-registering collision). Returns false if
// no spot was found.
func (cs *CombatSystem) blinkMonsterRandom(m *monsterPkg.Monster3D) bool {
	w := cs.game.GetCurrentWorld()
	if w == nil || w.Width <= 0 || w.Height <= 0 {
		return false
	}
	tile := float64(cs.game.config.GetTileSize())
	fromX, fromY := m.X, m.Y
	for try := 0; try < 40; try++ {
		tx := rand.Intn(w.Width)
		ty := rand.Intn(w.Height)
		if tx == TileIndex(fromX, tile) && ty == TileIndex(fromY, tile) {
			continue // a blink always leaves its tile
		}
		cx, cy := TileCenterFromTile(tx, ty, tile)
		if cs.game.monsterHasAttackTarget(m) && cs.game.collisionSystem.IsMonsterAttackPostReserved(m.ID, cx, cy) {
			continue
		}
		if cs.game.collisionSystem.CanMoveToWithTileOverrides(m.ID, cx, cy, m.WalkableTileOverrides, m.Flying) {
			cs.game.relocateMonster(m, cx, cy)
			cs.game.spawnBlinkLightColumn(fromX, fromY)
			return true
		}
	}
	return false
}
