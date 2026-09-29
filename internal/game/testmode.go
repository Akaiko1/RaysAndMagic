package game

import (
	"math/rand"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// addMainDamageStat adds v to the attribute that scales a class's primary
// damage source: Intellect for arcane casters, Personality for clerics (their
// holy/light magic scales on Personality), Accuracy for ranged, Might for the
// melee fighters. Derived from class because the game stores no explicit
// "primary stat" field; kept as a single switch so the convention is obvious
// and in one place.
func addMainDamageStat(c *character.MMCharacter, v int) {
	if v <= 0 {
		return
	}
	switch c.Class {
	case character.ClassSorcerer, character.ClassDruid:
		c.Intellect += v
	case character.ClassCleric:
		c.Personality += v
	case character.ClassArcher, character.ClassThief:
		c.Accuracy += v
	default: // Knight, Paladin - melee weapon scaling on Might
		c.Might += v
	}
}

// raiseStat lifts *stat toward target spending at most budget points (1 point =
// +1 stat, matching the stat-popup), and returns the points spent.
func raiseStat(stat *int, target, budget int) int {
	if budget <= 0 || *stat >= target {
		return 0
	}
	need := target - *stat
	if need > budget {
		need = budget
	}
	*stat += need
	return need
}

// learnAllSchoolSpells fills in every available spell of each magic school the
// member already knows - a test-arena convenience so casters can exercise their
// full kit without buying/levelling into spells. Monster-only spells are already
// excluded by AvailableSpellIDs.
func learnAllSchoolSpells(m *character.MMCharacter) {
	for school, skill := range m.MagicSchools {
		if skill == nil {
			continue
		}
		ids, err := school.AvailableSpellIDs()
		if err != nil {
			continue
		}
		skill.KnownSpells = ids
	}
}

func (g *MMGame) completeTestNPCEncounter(key string) {
	if character.NPCConfigInstance == nil {
		return
	}
	data, ok := character.NPCConfigInstance.GetNPCData(key)
	if !ok || data.Encounter == nil {
		return
	}
	enc := data.Encounter

	// Defeat the encounter's bandits: spawn the same random count the live
	// encounter rolls (count_min..count_max per monster) and tally each kill -
	// loot, gold and per-kill XP - exactly like clearing a map.
	for _, em := range enc.Monsters {
		if em == nil {
			continue
		}
		for i := 0; i < rollCount(em.CountMin, em.CountMax); i++ {
			mon := monster.NewMonster3DFromConfig(0, 0, em.Type, g.config)
			xp, gold := g.tallyMonsterKill(mon)
			g.awardGold(gold)
			g.grantSharedXP(xp)
		}
	}

	if qm := quests.GlobalQuestManager; qm != nil && enc.QuestID != "" {
		gold, exp := 0, 0
		if enc.Rewards != nil {
			gold, exp = enc.Rewards.Gold, enc.Rewards.Experience
		}
		qm.CreateEncounterQuest(enc.QuestID, enc.QuestName, enc.QuestDescription, gold, exp)
		qm.CompleteEncounterQuest(enc.QuestID)
	}
	if enc.Rewards != nil {
		g.awardGold(enc.Rewards.Gold)
		// Encounter completion XP is awarded in full to each member (matching the
		// live awardEncounterRewards path), not split like per-kill XP.
		g.grantSharedXP(enc.Rewards.Experience)
		if enc.Rewards.CompletionMessage != "" {
			g.AddCombatMessage(enc.Rewards.CompletionMessage)
		}
	}
}

// rollCount returns a random count in [min, max] (inclusive), mirroring how the
// live encounter picks how many monsters to spawn. Falls back gracefully to a
// sane single value if the bounds are unset or inverted.
func rollCount(min, max int) int {
	if max < min {
		max = min
	}
	if max <= 0 {
		return 0
	}
	if min < 0 {
		min = 0
	}
	return min + rand.Intn(max-min+1)
}

func (g *MMGame) completeTestMapEncounter(key string) {
	if world.GlobalWorldManager == nil {
		return
	}
	mc, ok := world.GlobalWorldManager.MapConfigs[key]
	if !ok || mc.ClearEncounter == nil || mc.ClearEncounter.Rewards == nil {
		return
	}
	g.grantMapEncounterReward(mc.ClearEncounter.Rewards)
}

// grantMapEncounterReward gives the party the gold + items of a map-clear
// encounter reward (including its single optional chest), built from the same
// reward helpers the live chest-spawn path uses.
func (g *MMGame) grantMapEncounterReward(r *config.MapEncounterRewardsConfig) {
	g.awardGold(r.Gold)
	g.grantChestConfig(r.TreasureChest)
	for i := range r.TreasureChests {
		g.grantChestConfig(&r.TreasureChests[i])
	}
	if r.CompletionMessage != "" {
		g.AddCombatMessage(r.CompletionMessage)
	}
}

func (g *MMGame) grantChestConfig(c *config.MapTreasureChestRewardConfig) {
	if c == nil {
		return
	}
	g.awardGold(c.Gold)
	for _, it := range randomWeaponRewards(c.RandomWeaponCount) {
		g.party.AddItem(it)
	}
	for _, it := range fixedWeaponRewards(c.Weapons) {
		g.party.AddItem(it)
	}
	for _, it := range fixedItemRewards(c.Items) {
		g.party.AddItem(it)
	}
	if c.LootTable != "" {
		poolItems, poolGold := rollWeightedLootTable(c.LootTable)
		g.awardGold(poolGold)
		for _, it := range poolItems {
			g.party.AddItem(it)
		}
	}
}

// clearMapAndTally empties a loaded map's monster list and returns the
// experience the party earns for the kills (per-member share, mirroring the
// live monster.Experience/len(party) split) plus the total gold the monsters
// carried. Each monster's loot is ROLLED by its real drop chances via the same
// checkMonsterLootDrop path the live game uses - not handed out one-of-each -
// so a clear yields a believable haul. Dead monsters on the active map are
// unregistered from collision; other maps don't have theirs registered yet.
func (g *MMGame) clearMapAndTally(mapKey string) (perMemberXP, gold int) {
	wm := world.GlobalWorldManager
	if wm == nil {
		return 0, 0
	}
	w := wm.WorldByKey(mapKey)
	if w == nil {
		return 0, 0
	}
	// A merged region clears only its rect of the unified world.
	region := wm.OpenWorldRegionByKey(mapKey)
	tileSize := g.config.GetTileSize()
	isCurrent := wm.SameWorldKey(wm.CurrentMapKey, mapKey)
	kept := w.Monsters[:0]
	for _, mon := range w.Monsters {
		if mon == nil {
			continue
		}
		if region != nil && wm.OpenWorldRegionAtTile(TileIndex(mon.X, tileSize), TileIndex(mon.Y, tileSize)) != region {
			kept = append(kept, mon)
			continue
		}
		xp, gp := g.tallyMonsterKill(mon)
		perMemberXP += xp
		gold += gp
		if isCurrent && g.collisionSystem != nil {
			g.collisionSystem.UnregisterEntity(mon.ID)
		}
	}
	w.Monsters = kept
	return perMemberXP, gold
}

// tallyMonsterKill resolves one monster kill the way the live combat path does:
// rolls its loot table (by real drop chance) into the party inventory and
// returns the gold it carried plus the per-member XP share
// (monster.Experience / party size, floored - matching awardExperienceAndGold).
func (g *MMGame) tallyMonsterKill(mon *monster.Monster3D) (perMemberXP, gold int) {
	if mon == nil {
		return 0, 0
	}
	if g.combat != nil {
		for _, drop := range g.combat.checkMonsterLootDrop(mon) {
			g.party.AddItem(drop)
		}
	}
	if members := len(g.party.Members); members > 0 {
		perMemberXP = mon.Experience / members
	}
	return perMemberXP, mon.Gold
}
