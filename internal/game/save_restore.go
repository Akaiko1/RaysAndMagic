package game

import (
	"fmt"
	"sort"
	"time"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// applySave restores game state from a save struct
func (g *MMGame) applySave(wm *world.WorldManager, save *GameSave) error {
	if wm == nil || save == nil {
		return fmt.Errorf("cannot restore a save without a world manager and save data")
	}
	// Resolve map identity before touching either timeline. Old saves without a
	// key deliberately inherit the current map, including its coordinate frame.
	restored := *save
	if restored.MapKey == "" {
		restored.MapKey = wm.CurrentMapKey
	}
	save = &restored
	targetWorld := wm.LoadedMaps[save.MapKey]
	if targetWorld == nil && wm.IsOpenWorldRegion(save.MapKey) {
		targetWorld = wm.OpenWorld
	}
	if targetWorld == nil {
		return fmt.Errorf("saved map is not loaded: %s", save.MapKey)
	}
	if save.MapKey != wm.CurrentMapKey {
		if err := wm.SwitchToMap(save.MapKey); err != nil {
			return err
		}
	}
	// Loading replaces the timeline, including any carried split and picker.
	g.cancelStackSplitInteraction()
	g.clearFocusMode()
	// Update world reference and visuals
	g.world = targetWorld

	// Deferred quest spawns belong to the timeline being replaced. Unlike a
	// normal map switch, loading another slot must discard them before the
	// loaded quest snapshot can enqueue its own completion spawns.
	g.pendingQuestSpawns = nil
	g.clearTransientCombatState()

	// Restore the day/night clock BEFORE the sky refresh below so the panorama
	// resolves to the saved phase. Recomputed silently (no flip side effects):
	// the save's pack monsters are restored as part of MapMonsters.
	g.dayNightFrames = save.DayNightFrames
	g.dayNightDay = save.DayNightDay
	g.calendarDay, g.calendarWeek, g.calendarMonth = calendarFromSave(save.CalendarDay, save.CalendarWeek, save.CalendarMonth, save.DayNightDay, g.config.DayNight)
	g.arenaTierFoughtDay = save.ArenaTierFoughtDay
	{
		names := make([]string, 0, len(save.Party.Members))
		for _, m := range save.Party.Members {
			names = append(names, m.Name)
		}
		g.playthroughID = adoptPlaythroughID(save.ArenaRunID, names)
	}
	g.dayNightIsNight = dayNightIsNightAt(g.dayNightFrac())

	// A loaded game starts with a clean combat log - the previous slot's history
	// must not bleed into it (clearTransientCombatState runs on every map switch
	// too, so the log reset lives here, on the load path, not in it).
	g.combatLogHistory = g.combatLogHistory[:0]
	g.combatLogVersion++
	g.combatLogScroll = 0
	g.combatLogOpen = false

	g.UpdateSkyAndGroundColors()
	g.collisionSystem.UpdateTileChecker(g.world)
	if g.gameLoop != nil && g.gameLoop.renderer != nil {
		g.gameLoop.renderer.precomputeFloorColorCache()
		g.gameLoop.renderer.buildTransparentSpriteCache()
	}

	// Restore player. Saves hold map-local coordinates and heading (local
	// canon): a merged region key projects into the unified grid via the
	// current layout and placement orientation.
	playerX, playerY := wm.ProjectWorldPos(save.MapKey, save.PlayerX, save.PlayerY)
	g.camera.X = playerX
	g.camera.Y = playerY
	g.snapFacing(wm.ProjectAngle(save.MapKey, save.PlayerAngle))
	g.collisionSystem.UpdateEntity("player", playerX, playerY)

	// Restore party
	g.party = &character.Party{Members: make([]*character.MMCharacter, 0, len(save.Party.Members)), Gold: save.Party.Gold, Food: save.Party.Food, ArenaPoints: save.Party.ArenaPoints, Inventory: save.Party.Inventory}
	for i := range g.party.Inventory {
		normalizeItemFromConfig(&g.party.Inventory[i])
	}
	// Restore the monster-card collection (party-wide). New saves carry the
	// physical card item + InstanceID; the legacy key-only field is load-only
	// migration and cannot prove ownership against the shared stash.
	g.cardSlots = [MaxCardSlots]cardSlot{}
	for i := 0; i < MaxCardSlots && i < len(save.Party.CardCollectionItems); i++ {
		it := save.Party.CardCollectionItems[i]
		if it.Name == "" {
			continue
		}
		normalizeItemFromConfig(&it)
		hadID := it.InstanceID != 0
		if g.setCardCollectionSlot(i, it) && !hadID {
			g.loadNeedsResave = true
		}
	}
	for i := 0; i < MaxCardSlots && i < len(save.Party.CardCollection); i++ {
		if g.cardCollectionKey(i) != "" {
			continue
		}
		key := save.Party.CardCollection[i]
		if cardDef(key) == nil {
			continue
		}
		if g.stashOwnsCardKey(key) {
			g.loadNeedsResave = true
			continue
		}
		if g.setCardCollectionSlot(i, items.CreateItemFromYAML(key)) {
			g.loadNeedsResave = true
		}
	}
	restoreRoster := func(dst *[]*character.MMCharacter, saves []CharacterSave) {
		for _, cs := range saves {
			member := restoreCharacterSave(cs)
			if member.EnsureClassKitSkills(g.config) {
				g.loadNeedsResave = true
			}
			if member.EnsureRacialTraits(g.config) {
				g.loadNeedsResave = true
			}
			*dst = append(*dst, member)
		}
	}
	restoreRoster(&g.party.Members, save.Party.Members)
	restoreRoster(&g.party.Reserve, save.Party.Reserve)
	restoreRoster(&g.party.Captive, save.Party.Captive)
	if save.TotalExperienceEarned > 0 {
		g.totalExperienceEarned = save.TotalExperienceEarned
	} else {
		g.totalExperienceEarned = earnedExperienceForParty(g.party)
	}
	if save.TotalGoldEarned > 0 {
		g.totalGoldEarned = save.TotalGoldEarned
	} else {
		g.totalGoldEarned = save.Party.Gold - g.config.Characters.StartingGold
		if g.totalGoldEarned < 0 {
			g.totalGoldEarned = 0
		}
	}
	// Instance-id dedupe: stamp any legacy (pre-id) party items, then strip from
	// the bag anything the shared chest already owns. A stamp means this slot was
	// migrated - flag it so LoadGameFromFile persists the ids once (the strip is
	// idempotent per load and needs no resave).
	if g.stampPartyInstanceIDs() {
		g.loadNeedsResave = true
	}
	g.reconcilePartyAgainstStash()
	// Fold duplicate stackables (pre-stacking saves) into stacks AFTER the
	// stash strip, so a chest-owned copy is removed before it can merge.
	g.party.MergeStacks()
	// Benched rosters re-derive MaxHP/MaxSP under the CURRENT formula too -
	// a save written before a formula/balance change would otherwise keep
	// stale maxima until the hero is swapped in or trained. (Active members
	// get theirs via applyPartyStatBonuses below; bench carries no buffs.)
	for _, m := range g.party.Reserve {
		m.RecalculateMaxStatsKeepingCurrent(g.config)
	}
	for _, m := range g.party.Captive {
		m.RecalculateMaxStatsKeepingCurrent(g.config)
	}

	// Restore monsters (all loaded maps)
	var migratedPyramidReliquaries *monster.EncounterRewards
	if wm != nil {
		rewardsCache := make(map[int]*monster.EncounterRewards)
		// Self-heal sealed bosses: a dormant boss (passive-until-quest, no evade
		// radius) never legitimately moves while its quest is unfinished - it holds
		// its map spawn. Saves written before the dormant-freeze fix captured it
		// wandered off (e.g. the Samurai Warlord drifted off his throne), so on
		// restore we snap any still-sealed boss back to its map spawn. Idempotent
		// for correct saves (saved position already IS the spawn); once the quest
		// completes the boss has gone aggressive and may have moved, so it keeps
		// its saved position.
		completedQuests := make(map[string]bool)
		for _, q := range save.Quests {
			if q.Status == string(quests.QuestStatusCompleted) {
				completedQuests[q.ID] = true
			}
		}
		// One taken-set for the WHOLE load: adopted IDs must be unique across
		// every map this save restores, or the dead-ID sweep (kills and
		// day/night pack despawns remove monsters BY ID) would delete ID-twins
		// on unrelated maps. Saves written before monster IDs were random can
		// carry such duplicates; the first occurrence keeps the saved identity
		// (boss adds reference their summoner via SummonedBy == that string),
		// later ones keep their fresh random ID - losing at most a summon
		// link, never a monster.
		takenMonsterIDs := make(map[string]struct{})
		adoptSavedMonsterID := func(m *monster.Monster3D, savedID string) {
			if savedID != "" {
				if _, taken := takenMonsterIDs[savedID]; !taken {
					m.ID = savedID
				} else {
					// Fossil duplicate: this monster keeps its fresh ID. Persist
					// the healing right away - LoadedMaps iterates in random map
					// order, so without a resave every future load of the same
					// slot could crown a DIFFERENT owner of the duplicated ID,
					// re-breaking SummonedBy links each time.
					g.loadNeedsResave = true
				}
			}
			takenMonsterIDs[m.ID] = struct{}{}
		}
		restoreMonsters := func(w *world.World3D, monsters []MonsterSave) {
			sealedSpawn := make(map[string][2]float64)
			for _, fresh := range w.Monsters {
				if fresh != nil && fresh.IsBoss() && fresh.PassiveUntilQuest != "" && fresh.EvadeRadiusTiles == 0 &&
					!completedQuests[fresh.PassiveUntilQuest] {
					sealedSpawn[fresh.Key] = [2]float64{fresh.X, fresh.Y}
				}
			}
			w.Monsters = make([]*monster.Monster3D, 0, len(monsters))
			for _, ms := range monsters {
				key := ms.Key
				if key == "" {
					key = findMonsterKeyByName(ms.Name)
				}
				if key == "" {
					continue
				}
				x, y := ms.X, ms.Y
				if sp, ok := sealedSpawn[key]; ok {
					x, y = sp[0], sp[1] // sealed boss -> back to its throne
				}
				m := monster.NewMonster3DFromConfig(x, y, key, g.config)
				adoptSavedMonsterID(m, ms.ID)
				// Seal a dormant boss immediately. refreshMonsterAIState recomputes
				// BossDormant every frame, but that runs AFTER input - so without this a
				// player action on the first frame after load could damage a still-sealed
				// boss before the flag is set. Uses the same completed-quest set as the
				// throne snap-back above.
				m.BossDormant = m.IsBoss() && m.PassiveUntilQuest != "" && m.EvadeRadiusTiles == 0 &&
					!completedQuests[m.PassiveUntilQuest]
				m.HitPoints = ms.HitPoints
				m.ChampionTier = ms.ChampionTier
				// Duel-cast state survives the reload: the opener fires once per
				// DUEL (config contract), and an active Stone Skin keeps soaking.
				m.OpeningSpellDone = ms.OpeningSpellDone
				m.SoakDamage = ms.SoakDamage
				m.SoakFrames = ms.SoakFrames
				m.SoakTurns = ms.SoakTurns
				m.SoakRate = ms.SoakRate
				if m.IsChampion() {
					// Mirror at restore (not next frame): the first post-load
					// input tick must already see tier HP pool and real armor.
					g.mirrorChampionStats(m)
				}
				if ms.RuntimeStats != nil {
					m.MaxHitPoints = ms.RuntimeStats.MaxHitPoints
					m.ArmorClass = ms.RuntimeStats.ArmorClass
					m.DamageMin = ms.RuntimeStats.DamageMin
					m.DamageMax = ms.RuntimeStats.DamageMax
				}
				m.Bound = ms.Bound
				m.BoundFramesRemaining = ms.BoundFramesRemaining
				// Bind and Charm are mutually exclusive. New saves cannot contain
				// both, but a defensive migration makes any older malformed state a
				// bound ally rather than silently turning a card summon neutral.
				m.Pacified = ms.Pacified && !m.Bound
				if m.Pacified {
					m.PacifiedFramesRemaining = ms.PacifiedFramesRemaining
				} else {
					m.PacifiedFramesRemaining = 0
				}
				// Old saves have no provenance bit, but an actively pacified monster
				// was necessarily charmed by the party.
				m.CharmedByParty = ms.CharmedByParty || ms.Pacified
				m.StunFramesRemaining = ms.StunFramesRemaining
				m.StunTurnsRemaining = ms.StunTurnsRemaining
				m.StunRate = ms.StunRate
				m.PoisonedFramesRemaining = ms.PoisonedFramesRemaining
				m.StunDRStacks = ms.StunDRStacks
				m.StunDRMemoryTurns = ms.StunDRMemoryTurns
				m.StunDRMemoryFrames = ms.StunDRMemoryFrames
				m.RootFramesRemaining = ms.RootFramesRemaining
				m.RootTurnsRemaining = ms.RootTurnsRemaining
				m.RootRate = ms.RootRate
				m.ArmorShredPct = ms.ArmorShredPct
				m.ArmorShredFramesRemaining = ms.ArmorShredFrames
				m.ArmorShredTurnsRemaining = ms.ArmorShredTurns
				m.ArmorShredRate = ms.ArmorShredRate
				// Pre-rated saves could preserve the inactive mode's stale clock
				// after shred had already expired in the mode they were saved in.
				// Treat that as expired rather than reviving it after load.
				if (!save.TurnBased && m.ArmorShredFramesRemaining <= 0) ||
					(save.TurnBased && m.ArmorShredTurnsRemaining <= 0) {
					m.ArmorShredPct = 0
					m.ArmorShredFramesRemaining = 0
					m.ArmorShredTurnsRemaining = 0
					m.ArmorShredRate = 0
				}
				m.BurnFramesRemaining = ms.BurnFramesRemaining
				m.RestoreDoTTickTimers(ms.PoisonTickTimer, ms.BurnTickTimer)
				m.TrapVolleyCDFrames = ms.TrapVolleyCD
				m.TrapVolleyTurnCD = ms.TrapVolleyTurnCD
				m.TrapVolleyCDRate = ms.TrapVolleyCDRate
				m.SlowPct = ms.SlowPct
				m.SlowFramesRemaining = ms.SlowFrames
				m.SlowTurnsRemaining = ms.SlowTurns
				m.SlowRate = ms.SlowRate
				if (!save.TurnBased && m.SlowFramesRemaining <= 0) ||
					(save.TurnBased && m.SlowTurnsRemaining <= 0) {
					m.SlowPct, m.SlowFramesRemaining, m.SlowTurnsRemaining, m.SlowRate = 0, 0, 0, 0
				}
				m.WeakenPct = ms.WeakenPct
				m.WeakenFramesRemaining = ms.WeakenFrames
				m.WeakenTurnsRemaining = ms.WeakenTurns
				m.WeakenRate = ms.WeakenRate
				if (!save.TurnBased && m.WeakenFramesRemaining <= 0) ||
					(save.TurnBased && m.WeakenTurnsRemaining <= 0) {
					m.WeakenPct, m.WeakenFramesRemaining, m.WeakenTurnsRemaining, m.WeakenRate = 0, 0, 0, 0
				}
				if save.TurnBased {
					m.RestoreTurnDebuffLatches(ms.SlowPctThisTurn, ms.WeakenPctThisTurn)
				} else {
					m.RestoreTurnDebuffLatches(0, 0)
				}
				m.Pilfered = ms.Pilfered
				m.PounceCDFrames = ms.PounceCDFrames
				m.PounceCDTurns = ms.PounceCDTurns
				m.PounceCDRate = ms.PounceCDRate
				m.BossCD = ms.BossCD
				m.InfernoCDFrames = ms.InfernoCD
				m.BossHurtPending = ms.BossHurtPending
				m.BossLastHP = ms.BossLastHP
				m.SummonFirstDone = ms.SummonFirstDone
				m.SummonedBy = ms.SummonedBy
				m.LootGuarding = ms.LootGuarding
				m.LootGuardTargetKey = ms.LootGuardTargetKey
				m.LootGuardTargetTileX, m.LootGuardTargetTileY = ms.LootGuardTargetTileX, ms.LootGuardTargetTileY
				m.LootGuardSide = ms.LootGuardSide
				m.LootGuardPatrolAlt = ms.LootGuardPatrolAlt
				m.LootGuardAlerted = ms.LootGuardAlerted
				m.RallyDone = ms.RallyDone
				m.PackKey = ms.PackKey
				m.QuestProgressIgnored = ms.QuestProgressIgnored
				// A provoked monster (struck, or spawned hostile by an encounter the
				// player opened) never stands down live - restore that hostility, or a
				// lair dragon "forgets" the fight after a reload and idles point-blank.
				// A quest-bearing encounter monster only exists because the player
				// started that fight (lair/shipwreck/statue), so it counts as provoked
				// even when the flag is absent (saves predating was_attacked).
				// Chest-bound clear-encounter mobs carry no QuestID: normal aggro.
				hostile := ms.WasAttacked ||
					(ms.IsEncounterMonster && ms.EncounterRewards != nil && ms.EncounterRewards.QuestID != "")
				m.WasAttacked = hostile
				// A sighted loot guard is non-sticky by design, so WasAttacked is
				// deliberately false. Preserve that active objective encounter across
				// save/load without turning it into a permanent normal aggro state.
				m.IsEngagingPlayer = hostile || m.LootGuardAlerted || (save.TurnBased && ms.TurnBasedSightEngaged)
				// Patron-death revenge persists: a rallied human keeps hunting after reload.
				if ms.Relentless {
					m.Relentless = true
					m.IsEngagingPlayer = true
				}
				if ms.IsEncounterMonster && ms.EncounterRewards != nil {
					m.IsEncounterMonster = true
					if ms.EncounterID > 0 {
						if rewards, ok := rewardsCache[ms.EncounterID]; ok {
							m.EncounterRewards = rewards
						} else {
							rewards = encounterRewardsFromSave(ms.EncounterRewards)
							rewardsCache[ms.EncounterID] = rewards
							m.EncounterRewards = rewards
						}
					} else {
						m.EncounterRewards = encounterRewardsFromSave(ms.EncounterRewards)
					}
				}
				w.Monsters = append(w.Monsters, m)
			}
			// Idol-ward immediately at restore. Unlike BossDormant (per-monster
			// above), it's cross-monster - it counts the live idols on THIS map - so
			// it must run after the loop. Same first-frame reason: refreshBoundUndead-
			// Cache recomputes it every frame but runs AFTER input, so without this the
			// warded boss would be hittable on the first frame after a load.
			liveIdols := 0
			for _, mm := range w.Monsters {
				if mm != nil && mm.WarlordIdol && mm.IsAlive() {
					liveIdols++
				}
			}
			if liveIdols > 0 {
				for _, mm := range w.Monsters {
					if mm != nil && mm.IsBoss() && mm.WardedByIdols {
						mm.BossWarded = true
					}
				}
			}
		}

		if len(save.MapMonsters) > 0 {
			for mapKey, w := range wm.LoadedMaps {
				monsters, ok := save.MapMonsters[mapKey]
				if !ok {
					continue
				}
				// A respawn_days map must stamp its roster on its first valid
				// arrival. Older saves made while a newly-authored map had no
				// resolvable spawns can contain an empty slice but no stamp; that
				// snapshot is ambiguous, and restoring it would erase the current
				// authored roster forever. A genuinely cleared farming map always
				// has its first-arrival stamp, so preserve only this legacy case.
				mapConfig := wm.MapConfigs[mapKey]
				_, hasRespawnStamp := save.MapRespawnDay[mapKey]
				if len(monsters) == 0 && !hasRespawnStamp && mapConfig != nil && mapConfig.RespawnDays > 0 && len(w.MonsterSpawns) > 0 {
					if len(w.Monsters) == 0 {
						w.RespawnAuthoredMonsters()
					}
					w.LastRespawnDay = g.dayNightDay + 1
					g.loadNeedsResave = true
					continue
				}
				restoreMonsters(w, monsters)
				if mapKey == "pyramid_3" {
					migratedPyramidReliquaries = g.migrateLegacyPyramidSanctumEncounter(w)
				}
			}
			// Unified world: gather every merged region's saved roster (projected
			// to unified coordinates) into ONE restore pass - restoreMonsters
			// resets the world's slice, so per-region calls would erase each
			// other. Regions absent from the save keep their fresh authored
			// monsters, same as split maps missing from MapMonsters.
			if wm.OpenWorld != nil {
				var combined []MonsterSave
				restoredRegions := make(map[string]bool)
				for _, region := range wm.OpenWorldRegions {
					monsters, ok := save.MapMonsters[region.MapKey]
					if !ok {
						continue
					}
					restoredRegions[region.MapKey] = true
					for _, msave := range monsters {
						msave.X, msave.Y = wm.ProjectWorldPos(region.MapKey, msave.X, msave.Y)
						if msave.LootGuardTargetTileX != 0 || msave.LootGuardTargetTileY != 0 {
							msave.LootGuardTargetTileX, msave.LootGuardTargetTileY =
								wm.ProjectTile(region.MapKey, msave.LootGuardTargetTileX, msave.LootGuardTargetTileY)
						}
						combined = append(combined, msave)
					}
				}
				if len(restoredRegions) > 0 {
					var keepFresh []*monster.Monster3D
					tileSize := g.config.GetTileSize()
					for _, mon := range wm.OpenWorld.Monsters {
						if mon == nil {
							continue
						}
						r := wm.OpenWorldRegionAtTile(TileIndex(mon.X, tileSize), TileIndex(mon.Y, tileSize))
						if r != nil && !restoredRegions[r.MapKey] {
							keepFresh = append(keepFresh, mon)
						}
					}
					restoreMonsters(wm.OpenWorld, combined)
					wm.OpenWorld.Monsters = append(wm.OpenWorld.Monsters, keepFresh...)
				}
			}
		} else if g.world != nil {
			if wm != nil && wm.OpenWorld != nil && g.world == wm.OpenWorld {
				// Legacy save (pre-MapMonsters): its roster covers the ACTIVE
				// map only, in that map's local coordinates. Restore it into
				// the save's region; every other region keeps its fresh
				// authored monsters instead of being wiped.
				projected := make([]MonsterSave, 0, len(save.Monsters))
				for _, msave := range save.Monsters {
					msave.X, msave.Y = wm.ProjectWorldPos(save.MapKey, msave.X, msave.Y)
					if msave.LootGuardTargetTileX != 0 || msave.LootGuardTargetTileY != 0 {
						msave.LootGuardTargetTileX, msave.LootGuardTargetTileY =
							wm.ProjectTile(save.MapKey, msave.LootGuardTargetTileX, msave.LootGuardTargetTileY)
					}
					projected = append(projected, msave)
				}
				var keepFresh []*monster.Monster3D
				tileSize := g.config.GetTileSize()
				saveRegion := wm.OpenWorldRegionByKey(save.MapKey)
				for _, mon := range wm.OpenWorld.Monsters {
					if mon == nil {
						continue
					}
					if r := wm.OpenWorldRegionAtTile(TileIndex(mon.X, tileSize), TileIndex(mon.Y, tileSize)); r != nil && r != saveRegion {
						keepFresh = append(keepFresh, mon)
					}
				}
				restoreMonsters(wm.OpenWorld, projected)
				wm.OpenWorld.Monsters = append(wm.OpenWorld.Monsters, keepFresh...)
			} else {
				restoreMonsters(g.world, save.Monsters)
			}
		}
		for mapKey, day := range save.MapRespawnDay {
			if w := wm.LoadedMaps[mapKey]; w != nil {
				w.LastRespawnDay = day
			}
		}

		// Re-register current map monsters with collision system
		if g.world != nil {
			g.collisionSystem = collision.NewCollisionSystem(g.world, float64(g.config.World.TileSize))
			g.collisionSystem.RegisterEntity(newPlayerCollisionEntity(g.camera.X, g.camera.Y))
			g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
		}
	}

	// Restore NPC state across maps (visited flags + merchant stock)
	if wm != nil {
		for _, ns := range save.NPCStates {
			w, ok := wm.LoadedMaps[ns.MapKey]
			nsX, nsY := ns.X, ns.Y
			if !ok {
				if w = wm.WorldByKey(ns.MapKey); w == nil || w != wm.OpenWorld {
					continue
				}
				// Merged region: saved coords are map-local, live NPCs sit at
				// unified coordinates.
				nsX, nsY = wm.ProjectWorldPos(ns.MapKey, ns.X, ns.Y)
			}
			for _, npc := range w.NPCs {
				// Coordinate match when the save has them; legacy saves
				// (X==Y==0) fall back to the old name match.
				if ns.X != 0 || ns.Y != 0 {
					if npc.X != nsX || npc.Y != nsY || npc.Name != ns.Name {
						continue
					}
				} else if npc.Name != ns.Name {
					continue
				}
				npc.Visited = ns.Visited
				npc.DoorAttempts = ns.DoorAttempts
				npc.DoorLockBroken = ns.DoorLockBroken
				// Stock restores by item NAME (order is presentation-only and can
				// change between versions); duplicate names consume sequentially.
				// A saved name missing from the current YAML is simply dropped.
				if len(ns.Stock) > 0 {
					cursor := make(map[string]int, len(ns.Stock))
					for _, saved := range ns.Stock {
						from := cursor[saved.Name]
						for i := from; i < len(npc.MerchantStock); i++ {
							if npc.MerchantStock[i].Item.Name == saved.Name {
								npc.MerchantStock[i].Quantity = saved.Quantity
								cursor[saved.Name] = i + 1
								break
							}
						}
					}
				}
			}
		}
	}
	// Static authored blocks must come after NPC Visited restoration: a saved
	// open door should not register a collision entity for one frame (or until a
	// later map switch) while its sprite is already invisible.
	if g.world != nil {
		g.registerMapStaticCollision()
	}

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

	// Restore ground containers (loot bags + treasure chests).
	g.groundContainers = make([]GroundContainer, 0, len(save.GroundContainers))
	for _, c := range save.GroundContainers {
		mapKey := c.MapKey
		if mapKey == "" {
			// Legacy saves: loot bags were stored without a map and leaked onto
			// every map. Pin them to the map the save was made on - imperfect for
			// bags dropped elsewhere, but they stop following the party around.
			mapKey = save.MapKey
		}
		restored := GroundContainer{
			Kind:      ContainerKind(c.Kind),
			ID:        c.ID,
			MapKey:    mapKey,
			X:         c.X,
			Y:         c.Y,
			Gold:      c.Gold,
			Sprite:    c.Sprite,
			SizeTiles: c.SizeTiles,
		}
		// Legacy saves baked the kind's default sprite name in; blank it so the
		// live effectiveSprite() (rarity-aware for loot bags) applies to old bags.
		if restored.Sprite == groundContainerDefaults[restored.Kind].sprite {
			restored.Sprite = ""
		}
		if len(c.Items) > 0 {
			restored.Items = make([]items.Item, len(c.Items))
			for i, it := range c.Items {
				normalizeItemFromConfig(&it)
				restored.Items[i] = it
			}
		}
		g.groundContainers = append(g.groundContainers, restored)
	}
	g.invalidateContainerFanCache()
	// Unified world: saved state is map-local (local canon) - project every
	// region-tagged coordinate into the stitched grid.
	if wm != nil && wm.OpenWorld != nil {
		tileSize := g.config.GetTileSize()
		for i := range g.groundContainers {
			c := &g.groundContainers[i]
			if wm.IsOpenWorldRegion(c.MapKey) {
				c.X, c.Y = wm.ProjectWorldPos(c.MapKey, c.X, c.Y)
			}
		}
		for i := range g.persistentDamageZones {
			z := &g.persistentDamageZones[i]
			if wm.IsOpenWorldRegion(z.MapKey) {
				z.X, z.Y = wm.ProjectWorldPos(z.MapKey, z.X, z.Y)
			}
		}
		for i := range g.traps {
			t := &g.traps[i]
			if wm.IsOpenWorldRegion(t.MapKey) {
				t.X, t.Y = wm.ProjectWorldPos(t.MapKey, t.X, t.Y)
				t.TileX, t.TileY = TileIndex(t.X, tileSize), TileIndex(t.Y, tileSize)
			}
		}
		for key, pose := range g.mapReturnPoses {
			if wm.IsOpenWorldRegion(key) {
				pose.X, pose.Y = wm.ProjectWorldPos(key, pose.X, pose.Y)
				pose.Angle = wm.ProjectAngle(key, pose.Angle)
				g.mapReturnPoses[key] = pose
			}
		}
		if wm.IsOpenWorldRegion(g.underwaterReturnMap) {
			g.underwaterReturnX, g.underwaterReturnY = wm.ProjectWorldPos(g.underwaterReturnMap, g.underwaterReturnX, g.underwaterReturnY)
		}
	}
	// A pre-fix pyramid save bound the reliquaries to every static mob on the
	// map. Ground containers restore first so their IDs can suppress duplicates;
	// then a fully cleared dais receives the reliquaries it was already owed.
	if migratedPyramidReliquaries != nil {
		g.addTreasureChestsFromRewards(migratedPyramidReliquaries)
	}

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

	// Completion-spawn history is save state even when quest content is
	// temporarily unavailable; never retain it from the replaced timeline.
	g.questSpawnsDone = make(map[string]bool, len(save.QuestSpawnsDone))
	for _, id := range save.QuestSpawnsDone {
		g.questSpawnsDone[id] = true
	}

	// Restore quest progress. Reset to the baseline (starting quests only) first
	// so quests taken AFTER this save - and therefore absent from it - don't
	// linger on the live manager; then lay the saved snapshot back on top.
	if g.questManager != nil {
		g.questManager.Reset()
		for _, qs := range save.Quests {
			g.questManager.RestoreQuestProgress(qs.ID, quests.QuestStatus(qs.Status), qs.CurrentCount, qs.DynamicTarget, qs.RewardsClaimed)
		}
		// Completion spawns already fired in this save's timeline must not fire
		// again (the spawned boss returns through the per-map monster restore).
		// Sync world changes to the LOADED quest state, both ways: completed
		// quests re-lay their tiles, and tiles of quests NOT completed in this
		// save revert to pristine. Loading does NOT reload maps from disk
		// (SwitchToMap flips a key on the shared instances), so a bridge laid
		// earlier this session must be actively taken back out here.
		g.syncQuestTiles()
		g.spawnQuestCompletionMonsters(false) // self-heal: a completed-but-unspawned quest fires now
		// Starting exterminate quests never pass through handleGiveQuest, so
		// anchor them to the restored rosters here - a save whose targets are
		// already all dead completes (and spawns its boss) right now.
		g.reconcileExterminationQuests()
	}
	// A restored journal is not news, and the loaded run must not inherit the old
	// one's heading or focus identity (loading does NOT reload maps, so NPC
	// pointers survive). One reset, after the journal has settled.
	g.resetScreenBanners()

	// Restore played time by adjusting session start
	if save.PlayedTimeNs > 0 {
		g.sessionStartTime = time.Now().Add(-time.Duration(save.PlayedTimeNs))
	}

	return nil
}

// migrateLegacyPyramidSanctumEncounter repairs saves written while pyramid_3
// used a map-wide clear encounter. Current-format saves persist the four
// encounter flags directly and never enter this path. A legacy member outside
// the authored dais catchment is the migration marker; within that format the
// same catchment keeps moved dais Isis without promoting the three lower Isis.
func (g *MMGame) migrateLegacyPyramidSanctumEncounter(w *world.World3D) *monster.EncounterRewards {
	if g == nil || w == nil || g.config == nil {
		return nil
	}
	const daisCatchmentTiles = 8.0 // 2 tiles from a chest + 4-tile tether + margin; lower Isis start 14 tiles away
	tileSize := float64(g.config.GetTileSize())
	isReliquaryReward := func(rewards *monster.EncounterRewards) bool {
		if rewards == nil || len(rewards.TreasureChests) != 4 {
			return false
		}
		for _, chest := range rewards.TreasureChests {
			if chest.ID == "pyramid_black_dragon_statuette_chest" {
				return true
			}
		}
		return false
	}
	daisDistanceSq := func(m *monster.Monster3D, rewards *monster.EncounterRewards) float64 {
		best := -1.0
		for _, chest := range rewards.TreasureChests {
			cx := (float64(chest.TileX) + 0.5) * tileSize
			cy := (float64(chest.TileY) + 0.5) * tileSize
			dx, dy := (m.X-cx)/tileSize, (m.Y-cy)/tileSize
			distSq := dx*dx + dy*dy
			if best < 0 || distSq < best {
				best = distSq
			}
		}
		return best
	}
	isDaisIsis := func(m *monster.Monster3D, rewards *monster.EncounterRewards) bool {
		if m == nil || m.Key != "isis" || rewards == nil {
			return false
		}
		limitSq := daisCatchmentTiles * daisCatchmentTiles
		return daisDistanceSq(m, rewards) <= limitSq
	}

	var legacy *monster.EncounterRewards
	for _, m := range w.Monsters {
		if m != nil && m.IsEncounterMonster && isReliquaryReward(m.EncounterRewards) &&
			!isDaisIsis(m, m.EncounterRewards) {
			legacy = m.EncounterRewards
			break
		}
	}
	if legacy == nil {
		return nil // new-format save, or an already-completed old-format save
	}
	g.loadNeedsResave = true

	type daisCandidate struct {
		monster    *monster.Monster3D
		distanceSq float64
	}
	var candidates []daisCandidate
	for _, m := range w.Monsters {
		if m == nil || m.EncounterRewards != legacy {
			continue
		}
		m.IsEncounterMonster = false
		m.EncounterRewards = nil
		if isDaisIsis(m, legacy) {
			candidates = append(candidates, daisCandidate{monster: m, distanceSq: daisDistanceSq(m, legacy)})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].distanceSq != candidates[j].distanceSq {
			return candidates[i].distanceSq < candidates[j].distanceSq
		}
		return candidates[i].monster.ID < candidates[j].monster.ID
	})
	if len(candidates) > len(legacy.TreasureChests) {
		candidates = candidates[:len(legacy.TreasureChests)]
	}
	for _, candidate := range candidates {
		candidate.monster.IsEncounterMonster = true
		candidate.monster.EncounterRewards = legacy
	}
	if len(candidates) == 0 {
		return legacy
	}
	return nil
}

func findMonsterKeyByName(name string) string {
	if monster.MonsterConfig == nil {
		return ""
	}
	for key, def := range monster.MonsterConfig.Monsters {
		if def.Name == name {
			return key
		}
	}
	return ""
}
