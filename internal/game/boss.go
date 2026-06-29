package game

import (
	"fmt"
	"math"
	"math/rand"

	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// Boss behaviour for the Golden Thief Bug (data-driven via monsters.yaml flags:
// PassiveUntilQuest / InfernoChance / TeleportAtHP / TeleportChance). Kept generic
// so any monster carrying those flags gets the same kit; only the sequencing lives
// here, shared by the real-time and turn-based monster loops.

// isBoss reports whether a monster carries any special boss behaviour flag.
func (cs *CombatSystem) isBoss(m *monsterPkg.Monster3D) bool {
	return m != nil && (m.PassiveUntilQuest != "" || m.InfernoChance > 0 ||
		m.TeleportChance > 0 || m.SummonChance > 0 || m.EnrageAtHP > 0 || m.WardedByIdols)
}

// bossDisabled reports whether crowd control should suppress a boss action:
// stun (either mode), charm, or bind. The RT/TB monster loops already skip
// disabled monsters before reaching updateBoss; this guards the few paths that
// don't (and documents intent).
func (cs *CombatSystem) bossDisabled(m *monsterPkg.Monster3D) bool {
	return m.StunTurnsRemaining > 0 || m.StunFramesRemaining > 0 || m.Pacified || m.Bound
}

// bossEvasive reports whether the boss is in its evasive phase: it has a
// PassiveUntilQuest gate and that quest is NOT yet completed. While evasive it
// never attacks or chases — it only blinks away when the party closes in.
func (cs *CombatSystem) bossEvasive(m *monsterPkg.Monster3D) bool {
	if m == nil || m.PassiveUntilQuest == "" {
		return false
	}
	if cs.game.questManager == nil {
		return true // no quest manager → treat as not-yet-done (stay evasive)
	}
	q := cs.game.questManager.GetQuest(m.PassiveUntilQuest)
	return q == nil || q.Status != quests.QuestStatusCompleted
}

// updateBoss runs the boss's special behaviour. `ready` gates the evasive blink to
// the boss's own cadence (RT: BossCD; TB: every turn). `attackTick` marks the
// once-per-attack moment when an aggressive boss may blink (low HP) or cast
// Inferno. Returns true when it handled the monster's action this tick (caller
// skips the normal attack); false lets the normal melee/ranged attack proceed
// (which honours IgnoresArmor).
func (cs *CombatSystem) updateBoss(m *monsterPkg.Monster3D, ready, attackTick bool) bool {
	// Latch any HP loss since last tick so the evasive blink can fire when hit.
	if m.BossLastHP > 0 && m.HitPoints < m.BossLastHP {
		m.BossHurtPending = true
	}
	m.BossLastHP = m.HitPoints

	if cs.bossEvasive(m) {
		// Quest unfinished → the boss never attacks. An evasive boss
		// (evade_radius_tiles set) blinks away when crowded or hit; a dormant boss
		// (no evade radius — e.g. the sealed Samurai Warlord) just holds its ground
		// until the quest completes, then turns aggressive.
		if m.EvadeRadiusTiles > 0 {
			near := Distance(cs.game.camera.X, cs.game.camera.Y, m.X, m.Y) <= m.EvadeRadiusTiles*float64(cs.game.config.GetTileSize())
			if ready && (near || m.BossHurtPending) && cs.blinkMonsterRandom(m) {
				m.BossCD = int(m.BossCooldownSecs * float64(cs.game.config.GetTPS()))
				m.BossHurtPending = false
				cs.game.AddCombatMessage(fmt.Sprintf("%s skitters away into the dark!", m.Name))
			}
		}
		return true
	}
	// Aggressive. Enrage is a passive HP-threshold state announced once here; its
	// mechanical effect (harder/faster hits) is applied live in
	// GetAttackDamage/AttackCooldownFrames, so it is save-safe.
	if m.EnrageAtHP > 0 && !m.Enraged && m.IsEnraged() {
		m.Enraged = true
		cs.game.AddCombatMessage(fmt.Sprintf("%s flies into a furious rage!", m.Name))
	}
	// Summon fires on the melee attack moment OR when the boss TAKES DAMAGE while
	// able to act. The hurt-provoke is the RT anti-kite: a boss being shot from
	// range still rallies adds instead of standing helpless, so it can't be safely
	// kited down. (In TB attackTick is already always true, so this adds nothing
	// there.) CC is filtered by the caller loops; bossDisabled is belt-and-
	// suspenders. One roll per damage event — BossHurtPending is latched from the
	// HP delta at the top of this func and consumed here. Skip the roll entirely at
	// the SummonMax cap (a passed roll would spawn nothing and fall through anyway).
	hurtProvoke := m.BossHurtPending && !cs.bossDisabled(m)
	m.BossHurtPending = false
	if (attackTick || hurtProvoke) && len(m.SummonMonsters) > 0 &&
		(m.SummonMax <= 0 || cs.countLiveSummons(m) < m.SummonMax) {
		guaranteed := m.SummonFirstGuaranteed && !m.SummonFirstDone
		if guaranteed || (m.SummonChance > 0 && rand.Float64() < m.SummonChance) {
			if cs.summonBossAdds(m) {
				m.SummonFirstDone = true
				return true
			}
		}
	}

	// The remaining specials (low-HP blink, Inferno) still fire only at the melee
	// attack moment.
	if !attackTick {
		return false
	}
	if m.TeleportAtHP > 0 && m.HitPoints <= m.TeleportAtHP && rand.Float64() < m.TeleportChance {
		if cs.blinkMonsterRandom(m) {
			cs.game.AddCombatMessage(fmt.Sprintf("%s blinks away in a golden flash!", m.Name))
			return true
		}
	}
	if m.InfernoChance > 0 && rand.Float64() < m.InfernoChance {
		cs.applyMonsterInferno(m)
		return true
	}
	return false // proceed to the normal attack (armor-piercing if IgnoresArmor)
}

// summonBossAdds rallies the boss's adds (war-banner): up to SummonCount monsters
// from SummonMonsters spawned ~2 tiles out on walkable tiles, hostile on arrival.
// Honours SummonMax (live summons of THIS boss). Returns true if any spawned.
func (cs *CombatSystem) summonBossAdds(m *monsterPkg.Monster3D) bool {
	if len(m.SummonMonsters) == 0 {
		return false
	}
	liveSummons := cs.countLiveSummons(m)
	if m.SummonMax > 0 && liveSummons >= m.SummonMax {
		return false
	}
	n := m.SummonCount
	if n < 1 {
		n = 1
	}
	if m.SummonMax > 0 && liveSummons+n > m.SummonMax {
		n = m.SummonMax - liveSummons
	}
	tile := float64(cs.game.config.GetTileSize())
	spawned := 0
	maxAttempts := n * 12
	if maxAttempts < 12 {
		maxAttempts = 12
	}
	for attempts := 0; spawned < n && attempts < maxAttempts; attempts++ {
		key := m.SummonMonsters[rand.Intn(len(m.SummonMonsters))]
		angle := rand.Float64() * 2 * math.Pi
		tx := m.X + math.Cos(angle)*2*tile
		ty := m.Y + math.Sin(angle)*2*tile
		sx, sy, ok := cs.findNearestSummonTile(tx, ty, 10)
		if !ok {
			continue
		}
		add := monsterPkg.NewMonster3DFromConfig(sx, sy, key, cs.game.config)
		if add == nil {
			continue
		}
		add.IsEngagingPlayer = true // summons wake hostile
		add.WasAttacked = true
		add.SummonedBy = m.ID
		add.QuestProgressIgnored = true // runtime summons never count toward map-clear quests
		cs.game.registerSpawnedMonster(add)
		cs.game.updateMonsterCollisionEngagement(add, cs.game.camera.X, cs.game.camera.Y)
		spawned++
	}
	if spawned == 0 {
		return false
	}
	cs.game.AddCombatMessage(fmt.Sprintf("%s raises the war-banner — retainers rush to its side!", m.Name))
	return true
}

func (cs *CombatSystem) findNearestSummonTile(targetX, targetY float64, maxRadius int) (float64, float64, bool) {
	w := cs.game.GetCurrentWorld()
	if w == nil || world.GlobalTileManager == nil {
		return 0, 0, false
	}
	tile := float64(cs.game.config.GetTileSize())
	targetTX := int(targetX / tile)
	targetTY := int(targetY / tile)

	for radius := 0; radius < maxRadius; radius++ {
		for dx := -radius; dx <= radius; dx++ {
			for dy := -radius; dy <= radius; dy++ {
				adx, ady := dx, dy
				if adx < 0 {
					adx = -adx
				}
				if ady < 0 {
					ady = -ady
				}
				if adx != radius && ady != radius {
					continue
				}
				tx := targetTX + dx
				ty := targetTY + dy
				if tx < 0 || tx >= w.Width || ty < 0 || ty >= w.Height {
					continue
				}
				if !world.GlobalTileManager.IsWalkable(w.Tiles[ty][tx]) {
					continue
				}
				x, y := TileCenterFromTile(tx, ty, tile)
				if cs.summonSpawnOccupied(x, y) {
					continue
				}
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

func (cs *CombatSystem) summonSpawnOccupied(x, y float64) bool {
	tile := float64(cs.game.config.GetTileSize())
	tx, ty := int(x/tile), int(y/tile)
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
		if int(o.X/tile) == tx && int(o.Y/tile) == ty {
			return true
		}
	}
	return false
}

// countLiveSummons counts living monsters this boss has summoned (for SummonMax).
func (cs *CombatSystem) countLiveSummons(m *monsterPkg.Monster3D) int {
	w := cs.game.GetCurrentWorld()
	if w == nil {
		return 0
	}
	n := 0
	for _, o := range w.Monsters {
		if o != nil && o.IsAlive() && o.SummonedBy == m.ID {
			n++
		}
	}
	return n
}

// tickEvasiveBossesTB runs the evasive-phase reaction every frame in turn-based
// mode, mirroring the RT cadence. Without it the hurt-blink waits for the
// monster turn, so a full party round of focused hits could kill the boss
// before it ever dodged. Aggressive-phase specials still fire only on the
// boss's own TB turn.
func (cs *CombatSystem) tickEvasiveBossesTB() {
	w := cs.game.GetCurrentWorld()
	if w == nil {
		return
	}
	for _, m := range w.Monsters {
		if m == nil || !m.IsAlive() || !cs.isBoss(m) || !cs.bossEvasive(m) {
			continue
		}
		// Crowd control suppresses the blink like any other action.
		if cs.bossDisabled(m) {
			continue
		}
		ready := m.BossCD == 0
		if m.BossCD > 0 {
			m.BossCD--
		}
		cs.updateBoss(m, ready, false)
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
		if cs.game.collisionSystem.CanMoveToWithHabitat(m.ID, cx, cy, m.HabitatPrefs, m.Flying) {
			m.X, m.Y = cx, cy
			cs.game.collisionSystem.UpdateEntity(m.ID, cx, cy)
			m.ResetPathfinding() // drop stale waypoints from the old position
			cs.game.spawnBlinkLightColumn(fromX, fromY)
			return true
		}
	}
	return false
}

// applyMonsterInferno scorches the whole party with fire (flat, mitigated).
func (cs *CombatSystem) applyMonsterInferno(m *monsterPkg.Monster3D) {
	cs.game.AddCombatMessage(fmt.Sprintf("%s erupts in a wave of fire!", m.Name))
	for idx, member := range cs.game.party.Members {
		if member == nil || member.HitPoints <= 0 {
			continue
		}
		dmg := cs.mitigateCharacterDamage(m.InfernoDamage, "fire", member, false)
		member.HitPoints -= dmg
		if member.HitPoints < 0 {
			member.HitPoints = 0
		}
		cs.game.AddCombatMessage(fmt.Sprintf("Inferno scorches %s for %d! (HP: %d/%d)",
			member.Name, dmg, member.HitPoints, member.MaxHitPoints))
		if member.HitPoints == 0 {
			cs.knockOut(member) // shared lethal chokepoint: Lich Card cheat-death roll, else unconscious
		}
		cs.game.TriggerDamageBlink(idx)
		cs.game.TriggerPartyFlame(idx)
	}
}
