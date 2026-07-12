package game

import (
	"testing"

	"ugataima/internal/character"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/quests"
)

// TestDragonRoster verifies the dragon split:
//   - dragon_cliffs wild dragons + the two cliff lair caves spawn ORDINARY
//     dragons (name "Dragon"),
//   - the four desert statues summon the ELITE "Elder Dragon"s (no biome/letter,
//     so they never spawn wild),
//   - and the dragon_slayer win quest credits ONLY a flagged Elder Dragon -
//     never an ordinary dragon, never an unflagged one.
func TestDragonRoster_BaseWildElitesStatueQuestGating(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load NPCs: %v", err)
	}
	qc, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	qm := quests.NewQuestManager(qc)
	qm.InitializeStartingQuests() // dragon_slayer is a starting quest
	cs.game.questManager = qm

	monName := func(key string) string {
		d, err := monsterPkg.MonsterConfig.GetMonsterByKey(key)
		if err != nil || d == nil {
			t.Fatalf("monster %q missing", key)
		}
		return d.Name
	}

	// 1) Ordinary dragons are named "Dragon".
	for _, k := range []string{"dragon", "dragon_red", "dragon_green", "dragon_gold"} {
		if got := monName(k); got != "Dragon" {
			t.Errorf("base %s name = %q, want \"Dragon\"", k, got)
		}
	}
	// 2) Elite dragons are named "Elder Dragon" and CANNOT spawn wild (no biomes).
	for _, k := range []string{"elder_dragon", "elder_dragon_red", "elder_dragon_green", "elder_dragon_gold"} {
		d, err := monsterPkg.MonsterConfig.GetMonsterByKey(k)
		if err != nil || d == nil {
			t.Fatalf("elite %q missing", k)
		}
		if d.Name != "Elder Dragon" {
			t.Errorf("elite %s name = %q, want \"Elder Dragon\"", k, d.Name)
		}
		if len(d.Biomes) != 0 {
			t.Errorf("elite %s must not be wild-spawnable, has biomes %v", k, d.Biomes)
		}
	}

	npcs := character.NPCConfigInstance.NPCs
	// 3) The two cliff lair caves spawn ORDINARY dragons.
	for cave, want := range map[string]string{
		"dragon_cliffs_bone_lair":  "dragon_green",
		"dragon_cliffs_ember_lair": "dragon_red",
	} {
		n := npcs[cave]
		if n == nil || n.Encounter == nil || len(n.Encounter.Monsters) == 0 {
			t.Fatalf("%s has no encounter monsters", cave)
		}
		got := n.Encounter.Monsters[0].Type
		if got != want {
			t.Errorf("%s spawns %q, want ordinary %q", cave, got, want)
		}
		if monName(got) != "Dragon" {
			t.Errorf("%s should spawn an ordinary Dragon, got %q (%s)", cave, monName(got), got)
		}
	}
	// 4) The four desert statues summon ELITE Elder Dragons.
	for statue, want := range map[string]string{
		"dragon_statue_black": "elder_dragon",
		"dragon_statue_red":   "elder_dragon_red",
		"dragon_statue_green": "elder_dragon_green",
		"dragon_statue_gold":  "elder_dragon_gold",
	} {
		n := npcs[statue]
		if n == nil || len(n.Summons) == 0 {
			t.Fatalf("%s has no summons", statue)
		}
		got := n.Summons[0].Monster
		if got != want {
			t.Errorf("%s summons %q, want %q", statue, got, want)
		}
		if monName(got) != "Elder Dragon" {
			t.Errorf("%s should summon an Elder Dragon, got %q (%s)", statue, monName(got), got)
		}
	}

	// 5) dragon_slayer credit gating.
	prog := func() int { return qm.GetQuest("dragon_slayer").CurrentCount }
	flag := func(m *monsterPkg.Monster3D) {
		m.IsEncounterMonster = true
		m.EncounterRewards = &monsterPkg.EncounterRewards{QuestID: "dragon_slayer"}
	}

	// An ordinary dragon must NEVER count - even if (somehow) flagged.
	base := monsterPkg.NewMonster3DFromConfig(0, 0, "dragon", cs.game.config)
	flag(base)
	cs.updateQuestProgress(base)
	if prog() != 0 {
		t.Errorf("ordinary dragon must not credit dragon_slayer, got %d", prog())
	}
	// An unflagged Elder Dragon (e.g. some future non-statue source) must not count.
	unflagged := monsterPkg.NewMonster3DFromConfig(0, 0, "elder_dragon", cs.game.config)
	cs.updateQuestProgress(unflagged)
	if prog() != 0 {
		t.Errorf("unflagged Elder Dragon must not credit, got %d", prog())
	}
	// A statue-summoned (flagged) Elder Dragon counts.
	summoned := monsterPkg.NewMonster3DFromConfig(0, 0, "elder_dragon_gold", cs.game.config)
	flag(summoned)
	cs.updateQuestProgress(summoned)
	if prog() != 1 {
		t.Errorf("flagged Elder Dragon must credit dragon_slayer, got %d", prog())
	}
}

func TestDragonSlayerQuestAwardsArenaPoints(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	qc, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	qm := quests.NewQuestManager(qc)
	qm.InitializeStartingQuests()
	cs.game.questManager = qm

	quest := qm.GetQuest("dragon_slayer")
	if quest == nil {
		t.Fatal("dragon_slayer quest missing")
	}
	if quest.Definition.Rewards.Gold != 0 || quest.Definition.Rewards.Experience != 0 || quest.Definition.Rewards.ArenaPoints != 5000 {
		t.Fatalf("dragon slayer reward = %+v, want 0 gold, 0 experience, and 5000 arena points", quest.Definition.Rewards)
	}
	qm.MarkCompleted("dragon_slayer")

	before := cs.game.party.ArenaPoints
	if !cs.game.claimQuestReward("dragon_slayer") {
		t.Fatal("claim dragon slayer reward")
	}
	if got := cs.game.party.ArenaPoints - before; got != 5000 {
		t.Fatalf("arena points awarded = %d, want 5000", got)
	}
}
