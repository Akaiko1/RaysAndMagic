package game

import (
	"sort"
	"ugataima/internal/collision"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// restoreSavedMonsters returns legacy encounter rewards to apply after containers.
func (g *MMGame) restoreSavedMonsters(wm *world.WorldManager, save *GameSave) *monster.EncounterRewards {
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
				// Legacy saves had no home: adopt their position once. A sealed boss
				// retains its authored throne instead of an obsolete saved anchor.
				if _, sealed := sealedSpawn[key]; !sealed && ms.SpawnPosition != nil {
					m.SpawnX, m.SpawnY = ms.SpawnPosition[0], ms.SpawnPosition[1]
				}
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

		// A load replaces the timeline. Missing stamps mean unknown age, not a
		// date inherited from whichever save happened to be loaded before it.
		for _, w := range wm.LoadedMaps {
			if w != nil {
				w.LastRespawnDay = 0
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
				hasRespawnStamp := save.MapRespawnDay[mapKey] > 0
				if len(monsters) == 0 && !hasRespawnStamp && mapConfig != nil && mapConfig.RespawnDays > 0 && len(w.MonsterSpawns) > 0 {
					// Gameplay respawns preserve party charms. A load must not
					// carry those allies over from the previous timeline.
					w.Monsters = nil
					w.RespawnAuthoredMonsters()
					w.LastRespawnDay = g.currentCalendarDay()
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
						msave = projectMonsterSave(wm, region.MapKey, msave)
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
					msave = projectMonsterSave(wm, save.MapKey, msave)
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
			if w := wm.LoadedMaps[mapKey]; w != nil && day > 0 {
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

	return migratedPyramidReliquaries
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

// Copy optional coordinates so restoring never mutates the caller's snapshot.
func projectMonsterSave(wm *world.WorldManager, mapKey string, ms MonsterSave) MonsterSave {
	ms.X, ms.Y = wm.ProjectWorldPos(mapKey, ms.X, ms.Y)
	if ms.SpawnPosition != nil {
		x, y := wm.ProjectWorldPos(mapKey, ms.SpawnPosition[0], ms.SpawnPosition[1])
		ms.SpawnPosition = &[2]float64{x, y}
	}
	return ms
}
