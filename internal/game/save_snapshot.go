package game

import (
	"maps"
	"sort"
	"time"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/stats"
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
		FreesCaptives:     save.FreesCaptives,
		Gold:              save.Gold,
		Experience:        save.Experience,
		CompletionMessage: save.CompletionMessage,
		QuestID:           save.QuestID,
		TreasureChest:     treasureChestRewardFromSave(save.TreasureChest),
	}
	if encounter := character.NPCConfigInstance.EncounterByQuestID(save.QuestID); encounter != nil && encounter.Rewards != nil {
		rewards.Gold = encounter.Rewards.Gold
		rewards.Experience = encounter.Rewards.Experience
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
		NonKeyDoorsOpened:   g.party.NonKeyDoorsOpened,
		ThievesGuildMember:  g.party.ThievesGuildMember,
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
		saved := buildCharacterSave(m)
		g.refundShotsInFlight(m, &saved.SpellState)
		ps.Members = append(ps.Members, saved)
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
			cMapKey, cX, cY := savedPos(wm, wm != nil && wm.IsOpenWorldRegion(c.MapKey), c.MapKey, c.X, c.Y)
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
			// Leaping fish are brief visual encounters; only their drops persist.
			if mon.IsFish() {
				continue
			}
			// Save the monster's own key (always set) - a name lookup is
			// ambiguous when several monsters share a Name (the elemental
			// dragons are all "Dragon") and would restore the wrong variant.
			rootHeldThisTurn, slowPctThisTurn, weakenPctThisTurn := mon.TurnDebuffLatches()
			poisonTickTimer, burnTickTimer := mon.DoTTickTimers()
			saveEntry := MonsterSave{
				Relics:        mon.Relics.Clone(),
				AmbientThreat: mon.Threat,
				Arbor:         mon.Arbor,
				Population:    mon.Population,
				ID:            mon.ID, Key: mon.Key, Name: mon.Name, X: mon.X, Y: mon.Y, HitPoints: mon.HitPoints,
				Bound: mon.Bound, BoundFramesRemaining: mon.BoundClock.Frames,
				FearFramesRemaining:            mon.Fear.Frames,
				SilenceFramesRemaining:         mon.Silence.Frames,
				SilenceImmunityFramesRemaining: mon.SilenceImmunity.Frames,
				BerserkFramesRemaining:         mon.Berserk.Frames,
				Pacified:                       mon.Pacified, PacifiedFramesRemaining: mon.PacifiedClock.Frames,
				CharmedByParty:          mon.CharmedByParty,
				WasAttacked:             mon.WasAttacked,
				BandInstance:            mon.BandInstance,
				Aggro:                   &MonsterAggroSave{Engaged: mon.TargetsParty(), ReturningHome: mon.ReturningHome, Retaliation: mon.Retaliation, BandHitPending: mon.BandHitPending},
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
				EnrageAnnounced:         mon.Enraged,
				SoakDamage:              mon.SoakDamage,
				Soak:                    mon.Soak.Saved(),
				PackKey:                 mon.PackKey,
				QuestProgressIgnored:    mon.QuestProgressIgnored,
				HomeMap:                 mon.HomeMap,
				Stun:                    mon.Stun.Saved(),
				PoisonedFramesRemaining: mon.PoisonedFramesRemaining,
				PoisonTickTimer:         poisonTickTimer,
				StunDRStacks:            mon.StunDRStacks,
				StunDRMemory:            mon.StunDRMemory.Saved(),
				Root:                    mon.Root.Saved(),
				ArmorShredPct:           mon.ArmorShredPct,
				ElementalMarks:          maps.Clone(mon.ElementalMarks),
				ArmorShred:              mon.ArmorShred.Saved(),
				ArmorMelt:               mon.ArmorMelt.Saved(),
				BurnFramesRemaining:     mon.BurnFramesRemaining,
				BurnTickTimer:           burnTickTimer,
				TrapVolleyCD:            mon.TrapVolleyCD.Saved(),
				Telegraph:               mon.Telegraph.Clone(),
				SlowPct:                 mon.SlowPct,
				Slow:                    mon.Slow.Saved(),
				SlowSkip:                mon.SlowSkip.Saved(),
				RootHeldThisTurn:        rootHeldThisTurn,
				SlowPctThisTurn:         slowPctThisTurn,
				WeakenPct:               mon.WeakenPct,
				Weaken:                  mon.Weaken.Saved(),
				WeakenPctThisTurn:       weakenPctThisTurn,
				Pilfered:                mon.Pilfered,
				PounceCD:                mon.PounceCD.Saved(),
				PhaseShiftCD:            mon.PhaseShiftCD.Saved(),
				TurnBasedSwingCarry:     mon.TurnBasedSwingCarry,
				TurnBasedStepCarry:      mon.TurnBasedStepCarry,
				Jammed:                  mon.Jammed,
				HourMarks:               mon.HourMarks,
				BossCD:                  mon.BossCD.Saved(),
				NovaRollCD:              mon.NovaRollCD.Saved(),
				BossHurtPending:         mon.BossHurtPending,
				BossLastHP:              mon.BossLastHP,
				SummonFirstDone:         mon.SummonFirstDone,
				SummonCD:                mon.SummonCD.Saved(),
				PushCD:                  mon.PushCD.Saved(),
				LaneCD:                  mon.LaneCD.Saved(),
				SummonedBy:              mon.SummonedBy,
				SummonerName:            mon.SummonerName,
			}
			scaleLevel := mon.AdventureScaleLevel
			saveEntry.AdventureScaleLevel = &scaleLevel
			saveEntry.SpawnPosition = &[2]float64{mon.SpawnX, mon.SpawnY}
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
					FreesCaptives:     rewards.FreesCaptives,
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
			// Localize the filtered records themselves: transient actors such as
			// fish have no save entry, so live-roster indexes do not align.
			for _, entry := range saves {
				key, lx, ly, ok := wm.LocalizeWorldPos(entry.X, entry.Y)
				if !ok {
					continue
				}
				entry.X, entry.Y = lx, ly
				entry.AmbientThreat = entry.AmbientThreat.MapPosition(func(x, y float64) (float64, float64) {
					return wm.LocalizeRegionWorldPos(key, x, y)
				})
				entry.Arbor = entry.Arbor.MapPositions(func(x, y float64) (float64, float64) {
					return wm.LocalizeRegionWorldPos(key, x, y)
				})
				sx, sy := wm.LocalizeRegionWorldPos(key, entry.SpawnPosition[0], entry.SpawnPosition[1])
				entry.SpawnPosition = &[2]float64{sx, sy}
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
				if npc.QuestPropOwner != "" || npc.HarvestOwner != "" {
					continue // derived from saved activity state, not NPC visited flags
				}
				key, x, y := savedSpot(wm, localize, mapKey, npc.X, npc.Y)
				ns := NPCSave{
					MapKey: key, Key: npc.Key, Name: npc.Name, X: x, Y: y, Visited: npc.Visited,
					DoorAttempts: npc.DoorAttempts, DoorLockBroken: npc.DoorLockBroken,
					DoorOpenedBy: npc.DoorOpenedBy,
				}
				if len(npc.MerchantStock) > 0 {
					ns.Stock = make([]NPCStockSave, len(npc.MerchantStock))
					for i, entry := range npc.MerchantStock {
						ns.Stock[i] = NPCStockSave{Key: config.CatalogKey(entry.Item), Quantity: entry.Quantity}
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
				ID:               quest.ID,
				Activity:         quest.Activity.Clone(),
				Status:           string(quest.Status()),
				CurrentCount:     quest.CurrentCount,
				DynamicTarget:    quest.DynamicTarget,
				DynamicTargetSet: quest.DynamicTargetSet,
				RewardsClaimed:   quest.RewardsClaimed,
				ClaimedAtDay:     quest.ClaimedAtDay,
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
			options := make([]PendingLevelUpOptionSave, len(req.options))
			for i, option := range req.options {
				options[i] = PendingLevelUpOptionSave{Choice: option.choice, SkillType: option.skillType, School: option.school, SpellID: option.spellID}
			}
			pendingChoices = append(pendingChoices, PendingLevelUpChoiceSave{
				Options: options, MaxSelections: req.maxSelections, Selected: append([]bool(nil), req.selected...), Selection: req.selection, Title: req.title, PadToMinimum: req.padToMinimum,
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
	saveMapKey, savePX, savePY := savedPos(wm, g.openWorldActive(), wm.CurrentMapKey, g.camera.X, g.camera.Y)
	saveAngle := g.camera.Angle
	if g.openWorldActive() {
		saveAngle = wm.LocalizeAngle(saveMapKey, saveAngle)
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
			z.MapKey, z.X, z.Y = savedPos(wm, wm.IsOpenWorldRegion(z.MapKey), z.MapKey, z.X, z.Y)
		}
		for i := range trapSaves {
			t := &trapSaves[i]
			if wm.IsOpenWorldRegion(t.MapKey) {
				t.MapKey, t.X, t.Y = savedPos(wm, true, t.MapKey, t.X, t.Y)
				t.TileX, t.TileY = TileIndex(t.X, tileSize), TileIndex(t.Y, tileSize)
			}
		}
		if len(g.mapReturnPoses) > 0 {
			returnPoses = make(map[string]MapPose, len(g.mapReturnPoses))
			for key, pose := range g.mapReturnPoses {
				if wm.IsOpenWorldRegion(key) {
					// The pose keeps its key: localize into that region, as the
					// restore projects it back out of it.
					pose.X, pose.Y = wm.LocalizeKeyedWorldPos(key, pose.X, pose.Y)
					pose.Angle = wm.LocalizeAngle(key, pose.Angle)
				}
				returnPoses[key] = pose
			}
		}
		if wm.IsOpenWorldRegion(g.underwaterReturnMap) {
			uwX, uwY = wm.LocalizeKeyedWorldPos(g.underwaterReturnMap, uwX, uwY)
		}
	}

	return GameSave{
		Adventure:        cloneAdventureState(g.adventure),
		TerrainChanges:   append([]TerrainChange(nil), g.terrainChanges...),
		MapKey:           saveMapKey,
		PlayerX:          savePX,
		PlayerY:          savePY,
		PlayerAngle:      saveAngle,
		TurnBased:        g.turnBasedMode,
		SavedAt:          time.Now().Format(time.RFC3339),
		Party:            ps,
		Monsters:         ms,
		MapMonsters:      mapMonsters,
		MapRespawnDay:    mapRespawnDays,
		NPCStates:        nstates,
		Quests:           questSaves,
		QuestSpawnsDone:  questSpawnsDone,
		QuestPropLayouts: cloneQuestPropLayouts(g.questPropLayouts),
		Alchemy:          g.alchemy.Clone(), SpatialReuse: g.spatialReuse.Saved(), OpenerCooldown: g.openerCooldown.Saved(), PartyMoveClock: g.partyMoveClock.Saved(),
		BossFireTraps:               bossFireTrapSaves,
		BossFireTrapsOwner:          g.bossFireTrapsOwner,
		GroundContainers:            groundContainerSaves,
		PendingLevelUpChoices:       pendingChoices,
		PlayedTimeNs:                playedTime.Nanoseconds(),
		MaxPartyLevel:               g.unlockedPartyLevel(),
		DayNightFrames:              g.dayNightFrames,
		DayNightDay:                 g.dayNightDay,
		Ecology:                     cloneEcologyState(g.ecology),
		CalendarDay:                 g.currentCalendarDay(),
		CalendarWeek:                g.calendarWeek,
		CalendarMonth:               g.calendarMonth,
		RespawnDayVersion:           respawnDaySaveVersion,
		StackLayoutVersion:          stackLayoutSaveVersion,
		ArenaTierFoughtDay:          g.arenaTierFoughtDay,
		ArenaRunID:                  g.playthroughID,
		TotalGoldEarned:             g.totalGoldEarned,
		TotalExperienceEarned:       g.totalExperienceEarned,
		VictoryAcknowledged:         g.victoryAcknowledged,
		StashTransferID:             g.pendingStashTransferID,
		TurnBasedTurnSuspended:      g.turnBasedTurnSuspended,
		CurrentTurn:                 g.currentTurn,
		PartyRoot:                   g.partyRoot,
		PartyHinder:                 g.partyHinder,
		PartyActionsUsed:            g.partyActionsUsed,
		PartyPenaltyFreeActionsUsed: g.partyPenaltyFreeActionsUsed,
		TurnBasedMoveCooldown:       g.turnBasedMoveCooldown,
		TurnBasedRotCooldown:        g.turnBasedRotCooldown,
		MonsterTurnResolved:         g.monsterTurnResolved,
		ExtraMonsterAction:          g.turnBasedExtraMonsterAction,
		TurnBasedMonsterPassesLeft:  g.turnBasedMonsterPassesLeft,
		TurnBasedMonsterPassDelay:   g.turnBasedMonsterPassDelay,
		TurnBasedMonsterStatusTick:  g.turnBasedMonsterStatusTick,
		TurnBasedMonsterStunned:     turnBasedMonsterStunned,
		CardSummonClocks:            g.snapshotCardSummonCooldowns(),
		TorchLightActive:            g.torchLightActive,
		TorchLightDuration:          g.torchLightClock.Frames,
		TorchLightRadius:            g.torchLightRadius,
		WizardEyeActive:             g.wizardEyeActive,
		WizardEyeDuration:           g.wizardEyeClock.Frames,
		WalkOnWaterActive:           g.walkOnWaterActive,
		WalkOnWaterDuration:         g.walkOnWaterClock.Frames,
		FlyActive:                   g.flyActive,
		FlyDuration:                 g.flyClock.Frames,
		VisitedTavernMaps:           g.sortedVisitedPortalMaps(),
		StatBuffs:                   buildStatBuffSaves(g.statBuffs),
		BlessActive:                 legacyBless.Clock.Active(),
		BlessDuration:               legacyBless.Clock.Frames,
		BlessStatBonus:              legacyBless.Bonuses.Might,
		BlessBonusesPerStat:         statBonusesToMap(legacyBless.Bonuses),
		// Write-only legacy: an OLD binary reads stat_bonus on load (its expiry
		// math subtracts bless_stat_bonus from it); the new binary derives the
		// aggregate from stat_buffs and never reads this back.
		StatBonus:              g.statBonuses.Might,
		CombatBuffs:            buildCombatBuffSaves(g.combatBuffs),
		CelestialBuffSpellID:   g.celestialBuffSpellID,
		TimedBuffSourceVersion: timedBuffSourceSaveVersion,
		PersistentDamageZones:  persistentDamageZoneSaves,
		FrozenWater:            append([]FrozenWaterTile(nil), g.frozenWater...),
		Traps:                  trapSaves,
		WaterBreathingActive:   g.waterBreathingActive,
		WaterBreathingDuration: g.waterBreathingClock.Frames,
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
	for _, key := range stats.Names {
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
