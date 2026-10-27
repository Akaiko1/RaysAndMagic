package game

import (
	"math"
	"math/rand"

	uitext "ugataima/assets/text"
	"ugataima/internal/collision"
	monsterPkg "ugataima/internal/monster"
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

// bossEvasive reports whether the boss is in its evasive phase: it has a
// PassiveUntilQuest gate and that quest is NOT yet completed. While evasive it
// never attacks or chases - it only blinks away when the party closes in.
func (cs *CombatSystem) bossEvasive(m *monsterPkg.Monster3D) bool {
	if m == nil || !m.IsBoss() || m.QuestProgressIgnored || m.PassiveUntilQuest == "" {
		return false
	}
	return !cs.game.questManager.ObjectiveMet(m.PassiveUntilQuest)
}

// runBossSpecials is the ONE boss rider for every combat path: RT and TB, party
// fight and summon fight alike. Reads the special cadences (BossCD, the
// at-range nova cooldown; Monster3D.TickAbilityClocks spends them), owns the
// trap field and updateBoss; true = a special consumed the action. Bosses previously lost the WHOLE kit whenever a summon out-competed
// the party for aggro.
func (cs *CombatSystem) runBossSpecials(m *monsterPkg.Monster3D, attackTick, turnBased bool) bool {
	if m == nil || !m.IsBoss() {
		return false
	}
	if m.IsAfraid() || m.IsSilenced() {
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
// blink (low HP) or cast a nova; between attacks a nova may roll AT RANGE
// once NovaRollCD has run out. Returns true when it handled the monster's action
// this tick (caller skips the normal attack); false lets the normal
// melee/ranged attack proceed (which can melt armor: abilities.melt_armor).
func (cs *CombatSystem) updateBoss(m *monsterPkg.Monster3D, attackTick bool) bool {
	if m == nil || !m.IsBoss() {
		return false
	}
	if m.IsSilenced() || m.IsAfraid() {
		return false
	}
	// Latch any HP loss since last tick so the evasive blink can fire when hit.
	if m.BossLastHP > 0 && m.HitPoints < m.BossLastHP {
		m.BossHurtPending = true
	}
	m.BossLastHP = m.HitPoints

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
	// Aggressive. Enrage is a passive HP-threshold state announced once here; its
	// mechanical effect (harder/faster hits) is applied live in
	// GetAttackDamage/AttackCooldownFrames, so it is save-safe.
	if m.Abilities.Enrage.AtHP > 0 && !m.Enraged && m.IsEnraged() {
		m.Enraged = true
		cs.game.logCombatLine(logToneBad, uitext.Text("combat.flies_into_a", logSlot, logSlot), logMonsterName(m), logKeyword("damage", "furious rage"))
	}
	// A rolled summon fires on the melee attack moment OR when the boss TAKES
	// DAMAGE while able to act. The hurt-provoke is the RT anti-kite: a boss being
	// shot from range still rallies adds instead of standing helpless, so it can't
	// be safely kited down. (In TB attackTick is already always true, so this adds
	// nothing there.) A timed summon (cooldown_seconds) never rolls here. CC is
	// filtered by the caller loops; bossDisabled is belt-and-suspenders. One roll
	// per damage event - BossHurtPending is latched from the HP delta at the top
	// of this func and consumed here. Skip the roll entirely at the SummonMax cap
	// (a passed roll would spawn nothing and fall through anyway).
	hurtProvoke := m.BossHurtPending && !cs.bossDisabled(m)
	m.BossHurtPending = false
	if cs.bossSummonDue(m, attackTick, hurtProvoke) && cs.summonBossAdds(m) {
		m.SummonFirstDone = true
		return true
	}

	// The low-HP blink still fires only at the melee attack moment. A nova
	// (party_attacks trigger nova) may also roll from its reach between attacks.
	if !attackTick {
		return cs.tryBossRangedNova(m)
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

// bossSummonDue decides this tick's rolled summon: at the action or on a
// hurt-provoke, below the live cap. A timed summon is tryBossTimedSummon's.
func (cs *CombatSystem) bossSummonDue(m *monsterPkg.Monster3D, attackTick, hurtProvoke bool) bool {
	s := m.Abilities.Summon
	if len(s.Monsters) == 0 || s.CooldownSeconds > 0 || !(attackTick || hurtProvoke) || cs.summonsFull(m) {
		return false
	}
	return (s.FirstGuaranteed && !m.SummonFirstDone) || (s.Chance > 0 && rand.Float64() < s.Chance)
}

// tryBossTimedSummon calls the adds each time summon.cooldown_seconds runs
// out, when the live cap allows; the timer restarts either way. It does not
// wait for an attack moment, so kiting cannot stall it.
func (cs *CombatSystem) tryBossTimedSummon(m *monsterPkg.Monster3D) bool {
	if m.Abilities.Summon.CooldownSeconds <= 0 || m.SummonCD.Active() || cs.bossDisabled(m) || cs.bossEvasive(m) {
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
// round of focused hits could kill the boss before it ever dodged. BossCD is
// spent on the monster turn like every TB clock. Aggressive-phase specials
// still fire only on the boss's own TB turn.
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
// current map (re-registering collision). Returns false if no spot was found.
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
