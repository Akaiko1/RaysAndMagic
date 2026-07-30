package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/quests"
)

// bootQuestGiverTest loads the shipped quest and NPC content against a live
// game, so these are content contracts, not fixtures.
func bootQuestGiverTest(t *testing.T) (*MMGame, *quests.QuestManager) {
	t.Helper()
	cs := newTestCombatSystemWithConfig(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load NPCs: %v", err)
	}
	qc, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	qm := quests.NewQuestManager(qc)
	qm.InitializeStartingQuests()
	cs.game.questManager = qm
	return cs.game, qm
}

// A fresh party must not wake up holding errands nobody gave it. The three
// openers now belong to named givers; only the endgame GATES (which exist to
// spawn bosses, not to be read) may start active.
func TestOpeningQuestsComeFromGivers(t *testing.T) {
	_, qm := bootQuestGiverTest(t)
	for _, id := range []string{"goblin_hunt", "wolf_pack", "dragon_slayer", "lake_spiders"} {
		if q := qm.GetQuest(id); q != nil {
			t.Errorf("quest %q is active at boot; it must be given by an NPC", id)
		}
	}
	givers := map[string]string{
		"goblin_hunt":   "forest_peasant_wenna",
		"wolf_pack":     "forest_peasant_wenna",
		"lake_spiders":  "lake_bather_ilsa",
		"dragon_slayer": "desert_pilgrim_sylwen",
	}
	for questID, npcKey := range givers {
		npc, err := character.CreateNPCFromConfig(npcKey, 0, 0)
		if err != nil {
			t.Errorf("NPC %q: %v", npcKey, err)
			continue
		}
		found := false
		for _, c := range questChoicesOf(npc) {
			if c.QuestID == questID && c.Action == "give_quest" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("NPC %q does not offer %q", npcKey, questID)
		}
	}
}

// Wenna hands out ONE errand at a time: the wolves stay hidden until the goblin
// nest is settled and paid for, and turning the goblins in must not conclude
// her - she still has the second ask.
func TestPeasantChainsGoblinsThenWolves(t *testing.T) {
	g, qm := bootQuestGiverTest(t)
	npc, err := character.CreateNPCFromConfig("forest_peasant_wenna", 0, 0)
	if err != nil {
		t.Fatalf("Wenna: %v", err)
	}

	// What the player can actually accept right now: every give_quest reachable
	// from the root, descending into "info" branches through the ENGINE so the
	// same filtering applies at depth as at the top level.
	offers := func() (goblin, wolf bool) {
		var walk func(depth int)
		walk = func(depth int) {
			for _, c := range g.visibleNPCChoices(npc) {
				switch c.Action {
				case "give_quest":
					switch c.QuestID {
					case "goblin_hunt":
						goblin = true
					case "wolf_pack":
						wolf = true
					}
				case "info":
					if depth < 3 {
						g.dialogNodePath = append(g.dialogNodePath, c)
						walk(depth + 1)
						g.dialogNodePath = g.dialogNodePath[:len(g.dialogNodePath)-1]
					}
				}
			}
		}
		walk(0)
		return
	}

	if gob, wolf := offers(); !gob || wolf {
		t.Fatalf("fresh Wenna offers goblins=%v wolves=%v, want goblins only", gob, wolf)
	}
	if got := g.activeChainQuestID(npc); got != "goblin_hunt" {
		t.Fatalf("chain step = %q, want goblin_hunt", got)
	}

	// Goblins taken, done, and paid: only now do the wolves come up.
	if err = qm.ActivateQuest("goblin_hunt"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if _, wolf := offers(); wolf {
		t.Error("wolves offered while the goblin nest is still active")
	}
	qm.MarkCompleted("goblin_hunt")
	if g.npcDialogueState(npc) != npcStateCompleted {
		t.Error("Wenna should be awaiting the goblin turn-in")
	}
	if !g.npcHasPendingChainStep(npc, "goblin_hunt") {
		t.Error("turning in the goblins must not conclude Wenna - the wolves are still to come")
	}
	if _, err := qm.ClaimRewards("goblin_hunt"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if gob, wolf := offers(); !wolf || gob {
		t.Fatalf("after the goblins Wenna offers goblins=%v wolves=%v, want wolves only", gob, wolf)
	}
	if got := g.activeChainQuestID(npc); got != "wolf_pack" {
		t.Fatalf("chain step = %q, want wolf_pack", got)
	}

	// Both done and paid: now she is finished.
	if err = qm.ActivateQuest("wolf_pack"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	qm.MarkCompleted("wolf_pack")
	if _, err := qm.ClaimRewards("wolf_pack"); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if g.npcHasPendingChainStep(npc, "wolf_pack") {
		t.Error("Wenna has no errands left; the turn-in must conclude her")
	}
	if g.npcDialogueState(npc) != npcStateConcluded {
		t.Error("Wenna should be concluded once both errands are paid")
	}
}

// Ilsa exists only in the dark, alongside the spiders she is complaining about.
func TestBatherIsNightOnly(t *testing.T) {
	g, _ := bootQuestGiverTest(t)
	npc, err := character.CreateNPCFromConfig("lake_bather_ilsa", 0, 0)
	if err != nil {
		t.Fatalf("Ilsa: %v", err)
	}
	if !npc.NightOnly {
		t.Fatal("Ilsa must be night_only")
	}

	g.dayNightIsNight = false
	if !g.npcAbsent(npc) {
		t.Error("Ilsa must not be present by day")
	}
	g.dayNightIsNight = true
	if g.npcAbsent(npc) {
		t.Error("Ilsa must be present at night")
	}
	// Her spiders are the map's own night pack, not a private spawn.
	found := false
	for _, p := range g.config.DayNight.Packs {
		if p.Map != "forest" {
			continue
		}
		for _, member := range p.PhaseMembers(true) {
			if member.Monster == "forest_spider" && member.QuestProgress {
				found = true
			}
		}
	}
	if !found {
		t.Error("the forest night pack must contain quest-eligible forest_spiders")
	}
}

// The nightly errand resets at dusk: cleared once paid, so the same task can be
// taken again, and its giver stops being concluded.
func TestLakeSpidersRepeatEachNight(t *testing.T) {
	g, qm := bootQuestGiverTest(t)
	def := qm.Definitions()["lake_spiders"]
	if def == nil || !def.Repeatable {
		t.Fatal("lake_spiders must be repeatable")
	}
	if len(def.Rewards.ItemPool) == 0 {
		t.Fatal("lake_spiders must pay an item from a pool")
	}
	for _, key := range def.Rewards.ItemPool {
		if _, ok := config.GetItemDefinition(key); !ok {
			t.Errorf("reward pool item %q is not a real item", key)
		}
	}

	npc, err := character.CreateNPCFromConfig("lake_bather_ilsa", 0, 0)
	if err != nil {
		t.Fatalf("Ilsa: %v", err)
	}
	npc.Visited = true // as a turn-in leaves her
	if err = qm.ActivateQuest("lake_spiders"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	qm.MarkCompleted("lake_spiders")
	if _, err := qm.ClaimRewards("lake_spiders"); err != nil {
		t.Fatalf("claim: %v", err)
	}

	g.refreshRepeatableQuests()
	if q := qm.GetQuest("lake_spiders"); q != nil {
		t.Error("a claimed repeatable errand must be cleared at nightfall")
	}
	// The sweep only reaches NPCs through the loaded worlds; check the rule it
	// applies directly on a giver that is not in one.
	if g.questChainStepDone("lake_spiders") {
		t.Error("with the quest cleared, the step must read as not done again")
	}

	// An UNCLAIMED errand in progress must survive the night, not restart.
	if err = qm.ActivateQuest("lake_spiders"); err != nil {
		t.Fatalf("re-activate: %v", err)
	}
	qm.SetCurrentCount("lake_spiders", 3)
	g.refreshRepeatableQuests()
	q := qm.GetQuest("lake_spiders")
	if q == nil || q.CurrentCount != 3 {
		t.Error("an in-progress errand must not be wiped by nightfall")
	}
}

// The seals answer an oath, not curiosity: no summon option exists until the
// hunt is sworn, and the refusal explains itself instead of showing nothing.
func TestDragonStatueRequiresTheSwornHunt(t *testing.T) {
	g, qm := bootQuestGiverTest(t)
	npc, err := character.CreateNPCFromConfig("dragon_statue_black", 0, 0)
	if err != nil {
		t.Fatalf("statue: %v", err)
	}
	g.party.Inventory = append(g.party.Inventory, items.CreateItemFromYAML("black_dragon_statuette"))
	ih := &InputHandler{game: g}

	ih.buildStatueChoices(npc)
	for _, c := range npc.DialogueData.Choices {
		if c.Action == "summon_dragon" {
			t.Fatal("statue offered a summon to a party that never swore the hunt")
		}
	}
	if len(npc.DialogueData.Choices) < 2 {
		t.Error("a refusing statue must still say why - the runes line is the hint")
	}

	if err = qm.ActivateQuest("dragon_slayer"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	ih.buildStatueChoices(npc)
	summon := false
	for _, c := range npc.DialogueData.Choices {
		if c.Action == "summon_dragon" {
			summon = true
		}
	}
	if !summon {
		t.Error("with the hunt sworn and the statuette held, the seal must open")
	}
}
