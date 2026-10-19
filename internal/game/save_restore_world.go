package game

import (
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

func (g *MMGame) restoreSavedNPCs(wm *world.WorldManager, save *GameSave) {
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
				if !npcSaveMatches(npc, ns, nsX, nsY) {
					continue
				}
				npc.Visited = ns.Visited
				npc.PropUseStarted = 0 // restore the settled prop pose, never replay consumption
				npc.DoorAttempts = ns.DoorAttempts
				npc.DoorLockBroken = ns.DoorLockBroken
				npc.DoorOpenedBy = ns.DoorOpenedBy
				// Old saves cannot distinguish a key from force or lockpicking.
				// Grandfather one opened lock, using the existing identity-aware
				// restore across split maps and merged regions. Modern key-opened
				// doors have a method and never receive this migration credit.
				if g.party != nil && g.party.NonKeyDoorsOpened == 0 && npc.Visited && character.IsLockedDoor(npc) && ns.DoorOpenedBy == "" {
					g.party.NonKeyDoorsOpened = 1
					g.loadNeedsResave = true
				}
				// Stock restores by ware key (order is presentation-only and can
				// change between versions); duplicate keys consume sequentially.
				// A ware missing from the current YAML is simply dropped.
				if len(ns.Stock) > 0 {
					cursor := make(map[string]int, len(ns.Stock))
					for _, saved := range ns.Stock {
						key := savedStockKey(saved)
						from := cursor[key]
						for i := from; i < len(npc.MerchantStock); i++ {
							if config.CatalogKey(npc.MerchantStock[i].Item) == key {
								npc.MerchantStock[i].Quantity = saved.Quantity
								cursor[key] = i + 1
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
}

// savedPos is the one save-side position rule: a position in the merged world
// persists as its region and map-local coordinates (corridors snap to the
// nearest region interior); any other position is kept as it is. A keyed
// record (a return pose) uses LocalizeKeyedWorldPos instead.
func savedPos(wm *world.WorldManager, merged bool, mapKey string, x, y float64) (string, float64, float64) {
	if merged && wm != nil {
		if key, lx, ly, ok := wm.LocalizeWorldPos(x, y); ok {
			return key, lx, ly
		}
	}
	return mapKey, x, y
}

// loadedPos is savedPos's inverse: a region-tagged position projected into
// the merged world.
func loadedPos(wm *world.WorldManager, mapKey string, x, y float64) (float64, float64) {
	if wm != nil && wm.OpenWorld != nil && wm.IsOpenWorldRegion(mapKey) {
		return wm.ProjectWorldPos(mapKey, x, y)
	}
	return x, y
}

// savedStockKey is a saved ware's catalog key: its key, or - in a save from
// before keys - the weapon or item its (possibly former) name names.
func savedStockKey(saved NPCStockSave) string {
	if saved.Key != "" {
		return config.CanonicalItemKey(saved.Key)
	}
	if _, key, ok := config.GetWeaponDefinitionByName(saved.Name); ok {
		return key
	}
	if _, key, ok := config.GetItemDefinitionByName(saved.Name); ok {
		return key
	}
	return ""
}

// npcSaveMatches is a saved NPC record's identity: its npcs.yaml key - its
// name in saves from before keys were saved, so a later rename keeps the
// state - and its spot (x, y projected to the live world) when saved.
func npcSaveMatches(npc *character.NPC, ns NPCSave, x, y float64) bool {
	if ns.Key != "" {
		if npc.Key != ns.Key {
			return false
		}
	} else if npc.Name != ns.Name {
		return false
	}
	return (ns.X == 0 && ns.Y == 0) || (npc.X == x && npc.Y == y)
}

func (g *MMGame) restoreSavedContainers(wm *world.WorldManager, save *GameSave) {
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
				if items.EnsureInstanceID(&it) {
					g.loadNeedsResave = true
				}
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
			c.X, c.Y = loadedPos(wm, c.MapKey, c.X, c.Y)
		}
		for i := range g.persistentDamageZones {
			z := &g.persistentDamageZones[i]
			z.X, z.Y = loadedPos(wm, z.MapKey, z.X, z.Y)
		}
		for i := range g.traps {
			t := &g.traps[i]
			if wm.IsOpenWorldRegion(t.MapKey) {
				t.X, t.Y = loadedPos(wm, t.MapKey, t.X, t.Y)
				t.TileX, t.TileY = TileIndex(t.X, tileSize), TileIndex(t.Y, tileSize)
			}
		}
		for key, pose := range g.mapReturnPoses {
			if wm.IsOpenWorldRegion(key) {
				pose.X, pose.Y = loadedPos(wm, key, pose.X, pose.Y)
				pose.Angle = wm.ProjectAngle(key, pose.Angle)
				g.mapReturnPoses[key] = pose
			}
		}
		g.underwaterReturnX, g.underwaterReturnY = loadedPos(wm, g.underwaterReturnMap, g.underwaterReturnX, g.underwaterReturnY)
	}
}

func (g *MMGame) restoreSavedQuests(save *GameSave) {
	g.resetQuestPropLayouts(save.QuestPropLayouts)
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
			if encounter := character.NPCConfigInstance.EncounterByQuestID(qs.ID); encounter != nil {
				gold, xp := 0, 0
				if encounter.Rewards != nil {
					gold, xp = encounter.Rewards.Gold, encounter.Rewards.Experience
				}
				g.questManager.CreateEncounterQuest(qs.ID, encounter.QuestName, encounter.QuestDescription, gold, xp)
			}
			g.questManager.RestoreQuestProgress(qs.ID, quests.QuestStatus(qs.Status), qs.CurrentCount, qs.DynamicTarget, qs.RewardsClaimed)
			if q := g.questManager.GetQuest(qs.ID); q != nil {
				q.ClaimedAtDay = qs.ClaimedAtDay
				if q.RewardsClaimed && q.ClaimedAtDay <= 0 {
					q.ClaimedAtDay = g.currentQuestDay()
				}
			}
			if q := g.questManager.GetQuest(qs.ID); q != nil {
				q.Activity = quests.RestoreActivityState(q.Definition.Activity, qs.Activity)
			}
			if qs.DynamicTargetSet {
				g.questManager.SetDynamicTarget(qs.ID, qs.DynamicTarget)
			}
		}
		// Completion spawns already fired in this save's timeline must not fire
		// again (the spawned boss returns through the per-map monster restore).
		// Sync world changes to the LOADED quest state, both ways: completed
		// quests re-lay their tiles, and tiles of quests NOT completed in this
		// save revert to pristine. Loading does NOT reload maps from disk
		// (SwitchToMap flips a key on the shared instances), so a bridge laid
		// earlier this session must be actively taken back out here.
		g.syncQuestTiles()
		g.syncQuestProps()
		g.spawnQuestCompletionMonsters(false) // self-heal: a completed-but-unspawned quest fires now
		// Reconcile active quotas against the restored roster and future spawns.
		g.reconcileKillQuests()
	}
}
