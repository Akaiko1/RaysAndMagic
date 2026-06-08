package game

import (
	"fmt"
	"math/rand"

	"ugataima/internal/character"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/quests"
)

// Boss behaviour for the Golden Thief Bug (data-driven via monsters.yaml flags:
// PassiveUntilQuest / InfernoChance / TeleportAtHP / TeleportChance). Kept generic
// so any monster carrying those flags gets the same kit; only the sequencing lives
// here, shared by the real-time and turn-based monster loops.

// isBoss reports whether a monster carries any special boss behaviour flag.
func (cs *CombatSystem) isBoss(m *monsterPkg.Monster3D) bool {
	return m != nil && (m.PassiveUntilQuest != "" || m.InfernoChance > 0 || m.TeleportChance > 0)
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
	// Track HP loss since the boss's previous tick. Unlike the hit flash (which
	// decays in a few frames and won't survive to the boss's next turn-based turn),
	// this latches a "hurt" debt that persists until a blink actually consumes it —
	// so damage reliably triggers a teleport in both real-time and turn-based,
	// regardless of which damage path landed the hit.
	if m.BossLastHP > 0 && m.HitPoints < m.BossLastHP {
		m.BossHurtPending = true
	}
	m.BossLastHP = m.HitPoints

	if cs.bossEvasive(m) {
		// Evasive: blink to a random tile when the party closes within 3 tiles OR
		// when it owes a hurt-blink. Never attacks.
		near := Distance(cs.game.camera.X, cs.game.camera.Y, m.X, m.Y) <= 3*float64(cs.game.config.GetTileSize())
		if ready && (near || m.BossHurtPending) && cs.blinkMonsterRandom(m) {
			m.BossCD = cs.game.config.GetTPS()
			m.BossHurtPending = false
			cs.game.AddCombatMessage(fmt.Sprintf("%s skitters away into the dark!", m.Name))
		}
		return true
	}
	// Aggressive: special actions fire only at the attack moment.
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

// blinkMonsterRandom teleports the monster to a random walkable tile of the
// current map (re-registering collision). Returns false if no spot was found.
func (cs *CombatSystem) blinkMonsterRandom(m *monsterPkg.Monster3D) bool {
	w := cs.game.GetCurrentWorld()
	if w == nil || w.Width <= 0 || w.Height <= 0 {
		return false
	}
	tile := float64(cs.game.config.GetTileSize())
	for try := 0; try < 40; try++ {
		tx := rand.Intn(w.Width)
		ty := rand.Intn(w.Height)
		cx, cy := TileCenterFromTile(tx, ty, tile)
		if cs.game.collisionSystem.CanMoveToWithHabitat(m.ID, cx, cy, m.HabitatPrefs, m.Flying) {
			m.X, m.Y = cx, cy
			cs.game.collisionSystem.UpdateEntity(m.ID, cx, cy)
			m.ResetPathfinding() // drop stale waypoints from the old position
			return true
		}
	}
	return false
}

// applyMonsterInferno is the Golden Thief Bug's Inferno: a fire nova that scorches
// the whole party (flat damage, after defensive mitigation). Modeled on the
// party's tryCastInferno but cast BY the monster, against the party.
func (cs *CombatSystem) applyMonsterInferno(m *monsterPkg.Monster3D) {
	const infernoDamage = 28
	cs.game.AddCombatMessage(fmt.Sprintf("%s erupts in a wave of fire!", m.Name))
	for idx, member := range cs.game.party.Members {
		if member == nil || member.HitPoints <= 0 {
			continue
		}
		// Inferno is fire → the member's fire resistance applies (e.g. the boss's
		// own Golden Thief Bug Carapace makes its wearer immune) + party buffs.
		dmg := cs.mitigateCharacterDamage(infernoDamage, "fire", member, false)
		member.HitPoints -= dmg
		if member.HitPoints < 0 {
			member.HitPoints = 0
		}
		cs.game.AddCombatMessage(fmt.Sprintf("Inferno scorches %s for %d! (HP: %d/%d)",
			member.Name, dmg, member.HitPoints, member.MaxHitPoints))
		if member.HitPoints == 0 {
			member.AddCondition(character.ConditionUnconscious)
			cs.game.AddCombatMessage(fmt.Sprintf("%s falls unconscious!", member.Name))
		}
		cs.game.TriggerDamageBlink(idx)
		cs.game.TriggerPartyFlame(idx)
	}
}
