package game

import (
	"fmt"
	"math/rand"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// Test-arena tunables. This mode fast-forwards the party to a mid-game state so
// later content (highlands, pyramid, lich nexus) can be exercised without
// replaying the early game. Values intentionally live here, not in YAML: this
// is a developer fixture, not shipped game balance.
//
// The party LEVEL is NOT set here — it emerges from the experience actually
// earned clearing the forest + church + shipwreck (mirroring the live award
// rules), which lands the party around level 6. The stat points those level-ups
// hand out (StatPointsPerLevel each) are then SPENT level-consistently: up to
// the speed/endurance targets below, with everything left over going into the
// class's primary damage stat. So the damage stat scales with the level reached
// rather than being a fixed number.
const (
	testArenaSpeed     = 16
	testArenaEndurance = 20
)

// ApplyTestArena mutates a freshly constructed game into a pre-progressed test
// state: the forest and abandoned church cleared (loot, gold and experience
// collected), the shipwreck-bandit encounter completed, and the party levelled
// by that experience (~L6) with pumped stats and an UNSPENT skill choice.
//
// It is invoked from main.go only when the --test-arena flag is present, after
// NewMMGame has fully wired the world, party and collision system.
func (g *MMGame) ApplyTestArena() {
	if g == nil || g.party == nil {
		return
	}
	// Clear the two locations: per monster, roll its loot exactly as the live
	// kill path does, and tally the per-member XP share and the gold it carries
	// (same split the live kill path uses) as the monsters are removed.
	perMemberXP, gold := 0, 0
	for _, mapKey := range []string{"forest", "church"} {
		xp, gp := g.clearMapAndTally(mapKey)
		perMemberXP += xp
		gold += gp
	}
	g.party.Gold += gold

	g.completeTestEncounters() // shipwreck quest (gold + XP) + church chest reward
	g.grantSharedXP(perMemberXP) // forest + church kill XP → natural level-ups
	g.setupTestParty()          // pump stats / full heal on top of the earned level

	level := 0
	if len(g.party.Members) > 0 && g.party.Members[0] != nil {
		level = g.party.Members[0].Level
	}
	g.AddCombatMessage(fmt.Sprintf("[TEST ARENA] Forest & church cleared, shipwreck done, party at L%d.", level))
	fmt.Printf("[TEST ARENA] Party at level %d; +%d gold from mobs; loot + encounters collected.\n", level, gold)
}

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
	case character.ClassArcher:
		c.Accuracy += v
	default: // Knight, Paladin — melee weapon scaling on Might
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

// setupTestParty spends each member's earned level-up stat points and full-
// heals them. It runs AFTER experience has been awarded, so the level (and the
// pending level-3 skill choice queued by the level-up path) are already in
// place. The points are spent level-consistently: speed up to testArenaSpeed,
// endurance up to testArenaEndurance, and whatever remains into the primary
// damage stat — so a higher level yields a higher damage stat.
func (g *MMGame) setupTestParty() {
	for _, m := range g.party.Members {
		if m == nil {
			continue
		}
		pts := m.FreeStatPoints
		pts -= raiseStat(&m.Speed, testArenaSpeed, pts)
		pts -= raiseStat(&m.Endurance, testArenaEndurance, pts)
		addMainDamageStat(m, pts) // remainder → primary damage stat
		m.FreeStatPoints = 0

		m.CalculateDerivedStats(g.config)
		m.HitPoints = m.MaxHitPoints
		m.SpellPoints = m.MaxSpellPoints
	}
}

// completeTestEncounters finishes the two forest-side encounters the way the
// game would, reading every value from config so it can't drift:
//   - shipwreck bandits: an NPC encounter quest (npcs.yaml) — registered and
//     completed in the quest log, gold + experience granted.
//   - abandoned church: its chest reward (map_configs.yaml). The skeletons
//     themselves are cleared (and tallied for XP/gold) in clearMapAndTally.
func (g *MMGame) completeTestEncounters() {
	g.completeShipwreckEncounter()
	g.completeChurchEncounter()
}

func (g *MMGame) completeShipwreckEncounter() {
	if character.NPCConfigInstance == nil {
		return
	}
	data, ok := character.NPCConfigInstance.GetNPCData("shipwreck_bandit_camp")
	if !ok || data.Encounter == nil {
		return
	}
	enc := data.Encounter

	// Defeat the encounter's bandits: spawn the same random count the live
	// encounter rolls (count_min..count_max per monster) and tally each kill —
	// loot, gold and per-kill XP — exactly like clearing a map.
	for _, em := range enc.Monsters {
		if em == nil {
			continue
		}
		for i := 0; i < rollCount(em.CountMin, em.CountMax); i++ {
			mon := monster.NewMonster3DFromConfig(0, 0, em.Type, g.config)
			xp, gold := g.tallyMonsterKill(mon)
			g.party.Gold += gold
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
		g.party.Gold += enc.Rewards.Gold
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

func (g *MMGame) completeChurchEncounter() {
	if world.GlobalWorldManager == nil {
		return
	}
	mc, ok := world.GlobalWorldManager.MapConfigs["church"]
	if !ok || mc.ClearEncounter == nil || mc.ClearEncounter.Rewards == nil {
		return
	}
	g.grantMapEncounterReward(mc.ClearEncounter.Rewards)
}

// grantMapEncounterReward gives the party the gold + items of a map-clear
// encounter reward (including its single optional chest), built from the same
// reward helpers the live chest-spawn path uses.
func (g *MMGame) grantMapEncounterReward(r *config.MapEncounterRewardsConfig) {
	g.party.Gold += r.Gold
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
	g.party.Gold += c.Gold
	for _, it := range randomWeaponRewards(c.RandomWeaponCount) {
		g.party.AddItem(it)
	}
	for _, it := range fixedWeaponRewards(c.Weapons) {
		g.party.AddItem(it)
	}
	for _, it := range fixedItemRewards(c.Items) {
		g.party.AddItem(it)
	}
}

// clearMapAndTally empties a loaded map's monster list and returns the
// experience the party earns for the kills (per-member share, mirroring the
// live monster.Experience/len(party) split) plus the total gold the monsters
// carried. Each monster's loot is ROLLED by its real drop chances via the same
// checkMonsterLootDrop path the live game uses — not handed out one-of-each —
// so a clear yields a believable haul. Dead monsters on the active map are
// unregistered from collision; other maps don't have theirs registered yet.
func (g *MMGame) clearMapAndTally(mapKey string) (perMemberXP, gold int) {
	if world.GlobalWorldManager == nil {
		return 0, 0
	}
	w, ok := world.GlobalWorldManager.LoadedMaps[mapKey]
	if !ok || w == nil {
		return 0, 0
	}
	isCurrent := world.GlobalWorldManager.CurrentMapKey == mapKey
	for _, mon := range w.Monsters {
		if mon == nil {
			continue
		}
		xp, gp := g.tallyMonsterKill(mon)
		perMemberXP += xp
		gold += gp
		if isCurrent && g.collisionSystem != nil {
			g.collisionSystem.UnregisterEntity(mon.ID)
		}
	}
	w.Monsters = w.Monsters[:0]
	return perMemberXP, gold
}

// tallyMonsterKill resolves one monster kill the way the live combat path does:
// rolls its loot table (by real drop chance) into the party inventory and
// returns the gold it carried plus the per-member XP share
// (monster.Experience / party size, floored — matching awardExperienceAndGold).
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
