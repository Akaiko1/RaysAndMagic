package game

import (
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func (g *MMGame) restoreSavedTurnState(save *GameSave) {
	// Restore mode
	g.turnBasedMode = save.TurnBased
	g.turnBasedTurnSuspended = save.TurnBasedTurnSuspended
	g.currentTurn = save.CurrentTurn
	g.partyActionsUsed = save.PartyActionsUsed
	g.turnBasedMoveCooldown = save.TurnBasedMoveCooldown
	g.turnBasedRotCooldown = save.TurnBasedRotCooldown
	g.monsterTurnResolved = save.MonsterTurnResolved
	g.turnBasedSpRegenCount = save.TurnBasedSpRegenCount
	g.turnBasedExtraMonsterAction = save.ExtraMonsterAction
	g.turnBasedMonsterPassesLeft = save.TurnBasedMonsterPassesLeft
	g.turnBasedMonsterPassDelay = save.TurnBasedMonsterPassDelay
	g.turnBasedMonsterStatusTick = save.TurnBasedMonsterStatusTick
	g.turnBasedMonsterStunned = nil
	if save.TurnBasedMonsterStatusTick || len(save.TurnBasedMonsterStunned) > 0 {
		g.turnBasedMonsterStunned = make(map[*monster.Monster3D]bool)
		stunnedByID := make(map[string]bool, len(save.TurnBasedMonsterStunned))
		for _, id := range save.TurnBasedMonsterStunned {
			stunnedByID[id] = true
		}
		if g.world != nil {
			for _, mon := range g.world.Monsters {
				if mon != nil && stunnedByID[mon.ID] {
					g.turnBasedMonsterStunned[mon] = true
				}
			}
		}
	}
}

func (g *MMGame) restoreSavedEffects(save *GameSave) {
	// Restore utility/buff state
	g.restoreCardSummonState(save.CardSummonCooldowns, save.CardSummonCDFrames)
	g.torchLightActive = save.TorchLightActive
	g.torchLightDuration = save.TorchLightDuration
	// Radius always follows the CURRENT spells.yaml (vision_radius_tiles) -
	// old saves froze whatever value was live when they were written.
	g.torchLightRadius = save.TorchLightRadius
	if g.torchLightActive {
		if def, err := spells.GetSpellDefinitionByID("torch_light"); err == nil && def.VisionRadiusTiles > 0 {
			g.torchLightRadius = def.VisionRadiusTiles
		}
	}
	g.wizardEyeActive = save.WizardEyeActive
	g.wizardEyeDuration = save.WizardEyeDuration
	// Same anti-freeze rule as the torch: an active eye adopts the CURRENT
	// spells.yaml radius instead of a stale or missing saved value.
	if g.wizardEyeActive {
		if def, err := spells.GetSpellDefinitionByID("wizard_eye"); err == nil && def.VisionRadiusTiles > 0 {
			g.wizardEyeRadiusTiles = def.VisionRadiusTiles
		}
	}
	g.walkOnWaterActive = save.WalkOnWaterActive
	g.walkOnWaterDuration = save.WalkOnWaterDuration
	g.flyActive = save.FlyActive
	g.flyDuration = save.FlyDuration
	g.visitedTavernMaps = map[string]bool{}
	for _, k := range save.VisitedTavernMaps {
		g.visitedTavernMaps[k] = true
	}
	// Pre-registry saves carry no destination list: seed it with the loaded
	// map's current Town Portal destination, if any, without forcing a map
	// change first.
	g.registerVisitedTownPortalDestination()
	g.statBuffs = restoreStatBuffs(save.StatBuffs)
	if len(g.statBuffs) == 0 && save.BlessActive && save.BlessDuration > 0 {
		// Pre-registry save: bless lived in dedicated fields.
		g.statBuffs = []TimedStatBuff{{
			SpellID: "bless",
			Frames:  save.BlessDuration,
			Bonuses: statBonusesFromSave(save.BlessBonusesPerStat, save.BlessStatBonus),
		}}
	}
	g.combatBuffs = restoreCombatBuffs(save.CombatBuffs)
	g.celestialBuffSpellID = save.CelestialBuffSpellID
	g.restoreCelestialProvidenceOwnership(save.TimedBuffSourceVersion)
	g.persistentDamageZones = restorePersistentDamageZones(save.PersistentDamageZones, save.MapKey)
	g.reseedPersistentDamageZoneFieldIDs()
	g.traps = restoreTraps(save.Traps, g.party)
	g.waterBreathingActive = save.WaterBreathingActive
	g.waterBreathingDuration = save.WaterBreathingDuration
	g.underwaterReturnX = save.UnderwaterReturnX
	g.underwaterReturnY = save.UnderwaterReturnY
	g.underwaterReturnMap = save.UnderwaterReturnMap
	// The aggregate is DERIVED from the restored registry (never trusted from
	// the save) - a drifted legacy save can't turn a buff expiry into a
	// permanent debuff. Also re-derives members' MaxHP/MaxSP under the buffs.
	g.recomputeStatBonuses()
	g.mapReturnPoses = save.MapReturnPoses
	if g.mapReturnPoses == nil {
		g.mapReturnPoses = make(map[string]MapPose)
	}
	g.levelUpChoiceQueue = g.levelUpChoiceQueue[:0]
	g.levelUpChoiceOpen = false
	g.levelUpChoiceIdx = 0
	for _, pending := range save.PendingLevelUpChoices {
		if pending.CharIndex < 0 || pending.CharIndex >= len(g.party.Members) {
			continue
		}
		char := g.party.Members[pending.CharIndex]
		choices := config.GetLevelUpChoices(char.GetClassKey(), pending.Level)
		g.queueLevelUpChoices(char, pending.Level, choices)
	}
	g.gameOver = false
	g.gameVictory = false
	g.victoryAcknowledged = save.VictoryAcknowledged
	g.showHighScores = false

	if g.world != nil {
		g.world.SetWalkOnWaterActive(g.walkOnWaterActive)
		g.world.SetFlyActive(g.flyActive)
		g.dropFlyWithoutOpenSky() // an indoor save (or a pre-rule one) must not restore wings
		g.world.SetWaterBreathingActive(g.waterBreathingActive)
	}

	// A position saved on an older map layout can sit inside what is now a
	// wall; clamp it to walkable ground. Runs here, after the buff restore
	// above, so water/Fly saves keep their legal mid-lake or airborne spot.
	if sx, sy := g.safePartyDestination(g.camera.X, g.camera.Y); sx != g.camera.X || sy != g.camera.Y {
		g.setPartyPosition(sx, sy)
	}
}

func (g *MMGame) restoreSavedEffectPresentation(wm *world.WorldManager, save *GameSave) {
	// Rebuild HUD buff icons from the single timed-buff registry (same source the
	// per-frame update uses), so a restored buff shows its timer immediately.
	g.utilitySpellStatuses = make(map[spells.SpellID]*UtilitySpellStatus)
	for _, b := range g.timedBuffs() {
		g.updateUtilityStatus(b.id, *b.duration, *b.active)
	}
	// Registry buffs (stat + combat) show their timers immediately too, not
	// only after the first tick refreshes them.
	for _, b := range g.statBuffs {
		g.updateUtilityStatus(spells.SpellID(b.SpellID), b.Frames, true)
	}
	for _, b := range g.combatBuffs {
		g.updateUtilityStatus(spells.SpellID(b.SpellID), b.Frames, true)
	}
	g.syncPersistentDamageZoneStatuses()

	// The Brood Mother's armed field is combat state, independent of quests.
	// Positions live here; her cadence cooldowns live in MonsterSave.
	g.bossFireTraps = restoreBossFireTraps(save.BossFireTraps, save.MapKey, wm)
	g.bossFireTrapsOwner = save.BossFireTrapsOwner
}
