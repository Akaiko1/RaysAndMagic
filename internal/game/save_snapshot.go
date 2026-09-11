package game

import (
	"sort"
	"time"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func treasureChestRewardToSave(reward *monster.TreasureChestReward) *TreasureChestRewardSave {
	if reward == nil {
		return nil
	}
	return &TreasureChestRewardSave{
		ID:                reward.ID,
		Map:               reward.Map,
		TileX:             reward.TileX,
		TileY:             reward.TileY,
		Sprite:            reward.Sprite,
		SizeTiles:         reward.SizeTiles,
		RandomWeaponCount: reward.RandomWeaponCount,
		Items:             append([]string(nil), reward.Items...),
		Weapons:           append([]string(nil), reward.Weapons...),
		Gold:              reward.Gold,
		LootTable:         reward.LootTable,
		CompletionMessage: reward.CompletionMessage,
	}
}

func treasureChestRewardFromSave(save *TreasureChestRewardSave) *monster.TreasureChestReward {
	if save == nil {
		return nil
	}
	return &monster.TreasureChestReward{
		ID:                save.ID,
		Map:               save.Map,
		TileX:             save.TileX,
		TileY:             save.TileY,
		Sprite:            save.Sprite,
		SizeTiles:         save.SizeTiles,
		RandomWeaponCount: save.RandomWeaponCount,
		Items:             append([]string(nil), save.Items...),
		Weapons:           append([]string(nil), save.Weapons...),
		Gold:              save.Gold,
		LootTable:         save.LootTable,
		CompletionMessage: save.CompletionMessage,
	}
}

func encounterRewardsFromSave(save *EncounterRewardSave) *monster.EncounterRewards {
	if save == nil {
		return nil
	}
	rewards := &monster.EncounterRewards{
		Gold:              save.Gold,
		Experience:        save.Experience,
		CompletionMessage: save.CompletionMessage,
		QuestID:           save.QuestID,
		TreasureChest:     treasureChestRewardFromSave(save.TreasureChest),
	}
	for _, chestSave := range save.TreasureChests {
		if chest := treasureChestRewardFromSave(&chestSave); chest != nil {
			rewards.TreasureChests = append(rewards.TreasureChests, *chest)
		}
	}
	return rewards
}

// buildSave gathers game state into a serializable struct
func (g *MMGame) buildSave(wm *world.WorldManager) GameSave {
	// A paid arena doze may be midway through a visual sky fade. Commit its
	// remaining gameplay phase transitions before taking the snapshot rather
	// than persisting a transient renderer queue with the save.
	g.finishDayNightSkipImmediately()
	// A queued quest spawn is already marked done in questSpawnsDone - land it
	// NOW or the snapshot records "spawned" with no monster in the roster and
	// a later load loses the boss forever (arrival autosave raced the flush).
	g.flushPendingQuestSpawns()
	// Legacy bless_* fields mirror the registry's bless entry so an older
	// binary can still read this save.
	legacyBless, _ := g.statBuffByID("bless")
	// Party
	ps := PartySave{
		Gold:                g.party.Gold,
		Food:                g.party.Food,
		ArenaPoints:         g.party.ArenaPoints,
		Inventory:           g.party.Inventory,
		Members:             make([]CharacterSave, 0, len(g.party.Members)),
		CardCollection:      make([]string, MaxCardSlots),
		CardCollectionItems: make([]items.Item, MaxCardSlots),
	}
	// The key list is derived (legacy readers only); cardSlots is the truth.
	for i := 0; i < MaxCardSlots; i++ {
		ps.CardCollection[i] = g.cardCollectionKey(i)
		ps.CardCollectionItems[i] = g.cardCollectionItem(i)
	}
	for _, m := range g.party.Members {
		ps.Members = append(ps.Members, buildCharacterSave(m))
	}
	for _, m := range g.party.Reserve {
		ps.Reserve = append(ps.Reserve, buildCharacterSave(m))
	}
	for _, m := range g.party.Captive {
		ps.Captive = append(ps.Captive, buildCharacterSave(m))
	}

	// Ground containers (loot bags + treasure chests) currently on the ground.
	var groundContainerSaves []GroundContainerSave
	if len(g.groundContainers) > 0 {
		groundContainerSaves = make([]GroundContainerSave, len(g.groundContainers))
		for i, c := range g.groundContainers {
			// Local canon: unified-world containers persist as (region key +
			// map-local coords), resolved by POSITION (a bag can drop across a
			// region seam from the key it was tagged with).
			cMapKey, cX, cY := c.MapKey, c.X, c.Y
			if wm != nil && wm.IsOpenWorldRegion(cMapKey) {
				if key, lx, ly, ok := wm.LocalizeWorldPos(cX, cY); ok {
					cMapKey, cX, cY = key, lx, ly
				}
			}
			entry := GroundContainerSave{
				Kind:      int(c.Kind),
				ID:        c.ID,
				MapKey:    cMapKey,
				X:         cX,
				Y:         cY,
				Gold:      c.Gold,
				Sprite:    c.Sprite,
				SizeTiles: c.SizeTiles,
			}
			if len(c.Items) > 0 {
				entry.Items = append([]items.Item(nil), c.Items...)
			}
			groundContainerSaves[i] = entry
		}
	}

	// Monsters across all loaded maps.
	var ms []MonsterSave
	mapMonsters := make(map[string][]MonsterSave)
	// Respawn stamps come from the SAME manager as the rosters below - a stamp
	// must pair with the roster snapshot it was minted for.
	mapRespawnDays := map[string]int{}
	if wm != nil {
		for key, w := range wm.LoadedMaps {
			if w != nil && w.LastRespawnDay != 0 {
				mapRespawnDays[key] = w.LastRespawnDay
			}
		}
	}
	encounterIDs := make(map[*monster.EncounterRewards]int)
	nextEncounterID := 1
	buildMonsterSaves := func(w *world.World3D) []MonsterSave {
		monsters := make([]MonsterSave, 0, len(w.Monsters))
		for _, mon := range w.Monsters {
			// Save the monster's own key (always set) - a name lookup is
			// ambiguous when several monsters share a Name (the elemental
			// dragons are all "Dragon") and would restore the wrong variant.
			slowPctThisTurn, weakenPctThisTurn := mon.TurnDebuffLatches()
			poisonTickTimer, burnTickTimer := mon.DoTTickTimers()
			saveEntry := MonsterSave{
				ID: mon.ID, Key: mon.Key, Name: mon.Name, X: mon.X, Y: mon.Y, HitPoints: mon.HitPoints,
				Bound: mon.Bound, BoundFramesRemaining: mon.BoundFramesRemaining,
				Pacified: mon.Pacified, PacifiedFramesRemaining: mon.PacifiedFramesRemaining,
				CharmedByParty:          mon.CharmedByParty,
				WasAttacked:             mon.WasAttacked,
				TurnBasedSightEngaged:   g.turnBasedMode && w == g.world && mon.IsEngagingPlayer && !mon.WasAttacked && !mon.LootGuardAlerted && mon.CurrentAIBehavior() == monster.AIBehaviorSeekParty,
				LootGuarding:            mon.LootGuarding,
				LootGuardTargetKey:      mon.LootGuardTargetKey,
				LootGuardTargetTileX:    mon.LootGuardTargetTileX,
				LootGuardTargetTileY:    mon.LootGuardTargetTileY,
				LootGuardSide:           mon.LootGuardSide,
				LootGuardPatrolAlt:      mon.LootGuardPatrolAlt,
				LootGuardAlerted:        mon.LootGuardAlerted,
				RallyDone:               mon.RallyDone,
				Relentless:              mon.Relentless,
				ChampionTier:            mon.ChampionTier,
				OpeningSpellDone:        mon.OpeningSpellDone,
				SoakDamage:              mon.SoakDamage,
				SoakFrames:              mon.SoakFrames,
				SoakTurns:               mon.SoakTurns,
				SoakRate:                mon.SoakRate,
				PackKey:                 mon.PackKey,
				QuestProgressIgnored:    mon.QuestProgressIgnored,
				StunFramesRemaining:     mon.StunFramesRemaining,
				StunTurnsRemaining:      mon.StunTurnsRemaining,
				StunRate:                mon.StunRate,
				PoisonedFramesRemaining: mon.PoisonedFramesRemaining,
				PoisonTickTimer:         poisonTickTimer,
				StunDRStacks:            mon.StunDRStacks,
				StunDRMemoryTurns:       mon.StunDRMemoryTurns,
				StunDRMemoryFrames:      mon.StunDRMemoryFrames,
				RootFramesRemaining:     mon.RootFramesRemaining,
				RootTurnsRemaining:      mon.RootTurnsRemaining,
				RootRate:                mon.RootRate,
				ArmorShredPct:           mon.ArmorShredPct,
				ArmorShredFrames:        mon.ArmorShredFramesRemaining,
				ArmorShredTurns:         mon.ArmorShredTurnsRemaining,
				ArmorShredRate:          mon.ArmorShredRate,
				BurnFramesRemaining:     mon.BurnFramesRemaining,
				BurnTickTimer:           burnTickTimer,
				TrapVolleyCD:            mon.TrapVolleyCDFrames,
				TrapVolleyTurnCD:        mon.TrapVolleyTurnCD,
				TrapVolleyCDRate:        mon.TrapVolleyCDRate,
				SlowPct:                 mon.SlowPct,
				SlowFrames:              mon.SlowFramesRemaining,
				SlowTurns:               mon.SlowTurnsRemaining,
				SlowRate:                mon.SlowRate,
				SlowPctThisTurn:         slowPctThisTurn,
				WeakenPct:               mon.WeakenPct,
				WeakenFrames:            mon.WeakenFramesRemaining,
				WeakenTurns:             mon.WeakenTurnsRemaining,
				WeakenRate:              mon.WeakenRate,
				WeakenPctThisTurn:       weakenPctThisTurn,
				Pilfered:                mon.Pilfered,
				PounceCDFrames:          mon.PounceCDFrames,
				PounceCDTurns:           mon.PounceCDTurns,
				PounceCDRate:            mon.PounceCDRate,
				BossCD:                  mon.BossCD,
				InfernoCD:               mon.InfernoCDFrames,
				BossHurtPending:         mon.BossHurtPending,
				BossLastHP:              mon.BossLastHP,
				SummonFirstDone:         mon.SummonFirstDone,
				SummonedBy:              mon.SummonedBy,
			}
			if isPurePartySummon(mon) {
				saveEntry.RuntimeStats = &MonsterRuntimeStatsSave{
					MaxHitPoints: mon.MaxHitPoints,
					ArmorClass:   mon.ArmorClass,
					DamageMin:    mon.DamageMin,
					DamageMax:    mon.DamageMax,
				}
			}
			if mon.IsEncounterMonster && mon.EncounterRewards != nil {
				saveEntry.IsEncounterMonster = true
				if id, ok := encounterIDs[mon.EncounterRewards]; ok {
					saveEntry.EncounterID = id
				} else {
					encounterIDs[mon.EncounterRewards] = nextEncounterID
					saveEntry.EncounterID = nextEncounterID
					nextEncounterID++
				}
				rewards := mon.EncounterRewards
				saveEntry.EncounterRewards = &EncounterRewardSave{
					Gold:              rewards.Gold,
					Experience:        rewards.Experience,
					CompletionMessage: rewards.CompletionMessage,
					QuestID:           rewards.QuestID,
				}
				if rewards.TreasureChest != nil {
					saveEntry.EncounterRewards.TreasureChest = treasureChestRewardToSave(rewards.TreasureChest)
				}
				for _, chest := range rewards.TreasureChests {
					if chestSave := treasureChestRewardToSave(&chest); chestSave != nil {
						saveEntry.EncounterRewards.TreasureChests = append(saveEntry.EncounterRewards.TreasureChests, *chestSave)
					}
				}
			}
			monsters = append(monsters, saveEntry)
		}
		return monsters
	}
	if wm != nil {
		for mapKey, w := range wm.LoadedMaps {
			monsters := buildMonsterSaves(w)
			mapMonsters[mapKey] = monsters
			if mapKey == wm.CurrentMapKey {
				ms = monsters
			}
		}
		// Unified world: bucket its monsters into their REGIONS with map-local
		// coordinates. The save format never learns about the merge, so the
		// same save loads in split mode (and vice versa). Loot-guard target
		// tiles localize with the monster's bucket region.
		if wm.OpenWorld != nil {
			saves := buildMonsterSaves(wm.OpenWorld)
			// Every region gets a bucket even when empty: a missing key means
			// "legacy save, keep fresh roster" to the loader, and a fully
			// cleared region must NOT read as that - its kills are permanent.
			for i := range wm.OpenWorldRegions {
				key := wm.OpenWorldRegions[i].MapKey
				if _, ok := mapMonsters[key]; !ok {
					mapMonsters[key] = []MonsterSave{}
				}
			}
			for i, mon := range wm.OpenWorld.Monsters {
				key, lx, ly, ok := wm.LocalizeWorldPos(mon.X, mon.Y)
				if !ok {
					continue
				}
				entry := saves[i]
				entry.X, entry.Y = lx, ly
				if entry.LootGuardTargetTileX != 0 || entry.LootGuardTargetTileY != 0 {
					entry.LootGuardTargetTileX, entry.LootGuardTargetTileY =
						wm.LocalizeTile(key, entry.LootGuardTargetTileX, entry.LootGuardTargetTileY)
				}
				mapMonsters[key] = append(mapMonsters[key], entry)
			}
			if regionMonsters, ok := mapMonsters[wm.CurrentMapKey]; ok && g.openWorldActive() {
				ms = regionMonsters
			}
		}
	} else if g.world != nil {
		ms = buildMonsterSaves(g.world)
	}

	// NPC states across all loaded maps
	var nstates []NPCSave
	if wm != nil {
		appendNPCStates := func(mapKey string, npcs []*character.NPC, localize bool) {
			for _, npc := range npcs {
				key, x, y := mapKey, npc.X, npc.Y
				if localize {
					if k, lx, ly, ok := wm.LocalizeWorldPos(npc.X, npc.Y); ok {
						key, x, y = k, lx, ly
					}
				}
				ns := NPCSave{
					MapKey: key, Name: npc.Name, X: x, Y: y, Visited: npc.Visited,
					DoorAttempts: npc.DoorAttempts, DoorLockBroken: npc.DoorLockBroken,
				}
				if len(npc.MerchantStock) > 0 {
					ns.Stock = make([]NPCStockSave, len(npc.MerchantStock))
					for i, entry := range npc.MerchantStock {
						ns.Stock[i] = NPCStockSave{Name: entry.Item.Name, Quantity: entry.Quantity}
					}
				}
				nstates = append(nstates, ns)
			}
		}
		for mapKey, w := range wm.LoadedMaps {
			appendNPCStates(mapKey, w.NPCs, false)
		}
		if wm.OpenWorld != nil {
			appendNPCStates("", wm.OpenWorld.NPCs, true)
		}
	}

	// Quest progress (save all quests, not just active)
	var questSaves []QuestSave
	if g.questManager != nil {
		for _, quest := range g.questManager.GetAllQuests() {
			questSaves = append(questSaves, QuestSave{
				ID:             quest.ID,
				Status:         string(quest.Status),
				CurrentCount:   quest.CurrentCount,
				DynamicTarget:  quest.DynamicTarget,
				RewardsClaimed: quest.RewardsClaimed,
			})
		}
	}
	var questSpawnsDone []string
	for id, done := range g.questSpawnsDone {
		if done {
			questSpawnsDone = append(questSpawnsDone, id)
		}
	}
	sort.Strings(questSpawnsDone) // deterministic save bytes

	// Calculate played time
	playedTime := time.Since(g.sessionStartTime)

	var pendingChoices []PendingLevelUpChoiceSave
	if len(g.levelUpChoiceQueue) > 0 {
		pendingChoices = make([]PendingLevelUpChoiceSave, 0, len(g.levelUpChoiceQueue))
		for _, req := range g.levelUpChoiceQueue {
			pendingChoices = append(pendingChoices, PendingLevelUpChoiceSave{
				CharIndex: req.charIndex,
				Level:     req.level,
			})
		}
	}
	var turnBasedMonsterStunned []string
	for mon, stunned := range g.turnBasedMonsterStunned {
		if stunned && mon != nil && mon.ID != "" {
			turnBasedMonsterStunned = append(turnBasedMonsterStunned, mon.ID)
		}
	}
	sort.Strings(turnBasedMonsterStunned)

	// Local canon: every unified-world position persists as (region key +
	// map-local coords). The save format never records the merged grid, so
	// saves survive layout changes, new maps, and flag flips in both
	// directions. Corridor positions snap to the nearest region interior.
	saveMapKey, savePX, savePY, saveAngle := wm.CurrentMapKey, g.camera.X, g.camera.Y, g.camera.Angle
	if g.openWorldActive() {
		if key, lx, ly, ok := wm.LocalizeWorldPos(savePX, savePY); ok {
			saveMapKey, savePX, savePY = key, lx, ly
			saveAngle = wm.LocalizeAngle(key, saveAngle)
		}
	}
	g.ensurePersistentDamageZoneFieldIDs()
	persistentDamageZoneSaves := buildPersistentDamageZoneSaves(g.persistentDamageZones)
	trapSaves := buildTrapSaves(g.traps)
	bossFireTrapSaves := buildBossFireTrapSaves(g.bossFireTraps, saveMapKey, wm)
	returnPoses := g.mapReturnPoses
	uwX, uwY := g.underwaterReturnX, g.underwaterReturnY
	if wm != nil && wm.OpenWorld != nil {
		tileSize := g.config.GetTileSize()
		for i := range persistentDamageZoneSaves {
			z := &persistentDamageZoneSaves[i]
			if wm.IsOpenWorldRegion(z.MapKey) {
				if key, lx, ly, ok := wm.LocalizeWorldPos(z.X, z.Y); ok {
					z.MapKey, z.X, z.Y = key, lx, ly
				}
			}
		}
		for i := range trapSaves {
			t := &trapSaves[i]
			if wm.IsOpenWorldRegion(t.MapKey) {
				if key, lx, ly, ok := wm.LocalizeWorldPos(t.X, t.Y); ok {
					t.MapKey, t.X, t.Y = key, lx, ly
					t.TileX, t.TileY = TileIndex(lx, tileSize), TileIndex(ly, tileSize)
				}
			}
		}
		if len(g.mapReturnPoses) > 0 {
			returnPoses = make(map[string]MapPose, len(g.mapReturnPoses))
			for key, pose := range g.mapReturnPoses {
				if wm.IsOpenWorldRegion(key) {
					if _, lx, ly, ok := wm.LocalizeWorldPos(pose.X, pose.Y); ok {
						pose.X, pose.Y = lx, ly
						pose.Angle = wm.LocalizeAngle(key, pose.Angle)
					}
				}
				returnPoses[key] = pose
			}
		}
		if wm.IsOpenWorldRegion(g.underwaterReturnMap) {
			if _, lx, ly, ok := wm.LocalizeWorldPos(uwX, uwY); ok {
				uwX, uwY = lx, ly
			}
		}
	}

	return GameSave{
		MapKey:                     saveMapKey,
		PlayerX:                    savePX,
		PlayerY:                    savePY,
		PlayerAngle:                saveAngle,
		TurnBased:                  g.turnBasedMode,
		SavedAt:                    time.Now().Format(time.RFC3339),
		Party:                      ps,
		Monsters:                   ms,
		MapMonsters:                mapMonsters,
		MapRespawnDay:              mapRespawnDays,
		NPCStates:                  nstates,
		Quests:                     questSaves,
		QuestSpawnsDone:            questSpawnsDone,
		BossFireTraps:              bossFireTrapSaves,
		BossFireTrapsOwner:         g.bossFireTrapsOwner,
		GroundContainers:           groundContainerSaves,
		PendingLevelUpChoices:      pendingChoices,
		PlayedTimeNs:               playedTime.Nanoseconds(),
		DayNightFrames:             g.dayNightFrames,
		DayNightDay:                g.dayNightDay,
		CalendarDay:                g.calendarDay,
		CalendarWeek:               g.calendarWeek,
		CalendarMonth:              g.calendarMonth,
		ArenaTierFoughtDay:         g.arenaTierFoughtDay,
		ArenaRunID:                 g.playthroughID,
		TotalGoldEarned:            g.totalGoldEarned,
		TotalExperienceEarned:      g.totalExperienceEarned,
		VictoryAcknowledged:        g.victoryAcknowledged,
		StashTransferID:            g.pendingStashTransferID,
		TurnBasedTurnSuspended:     g.turnBasedTurnSuspended,
		CurrentTurn:                g.currentTurn,
		PartyActionsUsed:           g.partyActionsUsed,
		TurnBasedMoveCooldown:      g.turnBasedMoveCooldown,
		TurnBasedRotCooldown:       g.turnBasedRotCooldown,
		MonsterTurnResolved:        g.monsterTurnResolved,
		TurnBasedSpRegenCount:      g.turnBasedSpRegenCount,
		ExtraMonsterAction:         g.turnBasedExtraMonsterAction,
		TurnBasedMonsterPassesLeft: g.turnBasedMonsterPassesLeft,
		TurnBasedMonsterPassDelay:  g.turnBasedMonsterPassDelay,
		TurnBasedMonsterStatusTick: g.turnBasedMonsterStatusTick,
		TurnBasedMonsterStunned:    turnBasedMonsterStunned,
		CardSummonCooldowns:        g.snapshotCardSummonCooldowns(),
		TorchLightActive:           g.torchLightActive,
		TorchLightDuration:         g.torchLightDuration,
		TorchLightRadius:           g.torchLightRadius,
		WizardEyeActive:            g.wizardEyeActive,
		WizardEyeDuration:          g.wizardEyeDuration,
		WalkOnWaterActive:          g.walkOnWaterActive,
		WalkOnWaterDuration:        g.walkOnWaterDuration,
		FlyActive:                  g.flyActive,
		FlyDuration:                g.flyDuration,
		VisitedTavernMaps:          g.sortedTownPortalDestinations(),
		StatBuffs:                  buildStatBuffSaves(g.statBuffs),
		BlessActive:                legacyBless.Frames > 0,
		BlessDuration:              legacyBless.Frames,
		BlessStatBonus:             legacyBless.Bonuses.Might,
		BlessBonusesPerStat:        statBonusesToMap(legacyBless.Bonuses),
		// Write-only legacy: an OLD binary reads stat_bonus on load (its expiry
		// math subtracts bless_stat_bonus from it); the new binary derives the
		// aggregate from stat_buffs and never reads this back.
		StatBonus:              g.statBonuses.Might,
		CombatBuffs:            buildCombatBuffSaves(g.combatBuffs),
		CelestialBuffSpellID:   g.celestialBuffSpellID,
		TimedBuffSourceVersion: timedBuffSourceSaveVersion,
		PersistentDamageZones:  persistentDamageZoneSaves,
		Traps:                  trapSaves,
		WaterBreathingActive:   g.waterBreathingActive,
		WaterBreathingDuration: g.waterBreathingDuration,
		UnderwaterReturnX:      uwX,
		UnderwaterReturnY:      uwY,
		UnderwaterReturnMap:    g.underwaterReturnMap,

		MapReturnPoses: returnPoses,
	}
}

// statBonusesToMap serializes a StatBonuses block for saves (nonzero entries
// only, lowercase keys - same shape spells.yaml authors).
func statBonusesToMap(b character.StatBonuses) map[string]int {
	if b.IsZero() {
		return nil
	}
	m := map[string]int{}
	for _, key := range config.StatNames {
		if v := b.ValueByName(key); v != 0 {
			m[key] = v
		}
	}
	return m
}

// statBonusesFromSave restores a bonus block: per-stat map when present,
// otherwise the legacy uniform int (pre-per-stat saves).
func statBonusesFromSave(perStat map[string]int, legacy int) character.StatBonuses {
	if len(perStat) > 0 {
		return character.StatBonusesFromMap(perStat)
	}
	return character.UniformStatBonuses(legacy)
}
