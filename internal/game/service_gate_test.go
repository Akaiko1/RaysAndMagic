package game

import (
	"os"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// The three outland services and the errand each one withholds itself behind.
// Shipped content, not a fixture: if a gate is renamed or dropped, this list is
// where it has to be said out loud.
var shippedServiceGates = []struct {
	npcKey   string
	questID  string
	openKind npcDialogKind
}{
	{"elf_city_archive", "archive_missing_volumes", dialogKindSpellTrader},
	{"nomad_city_spells", "shrine_lamps", dialogKindSpellTrader},
	{"nomad_city_trainer", "pit_standing", dialogKindSkillTrainer},
}

// finishQuest drives a quest all the way to paid out, the state a service gate
// opens on.
func finishQuest(t *testing.T, qm *quests.QuestManager, id string) {
	t.Helper()
	if err := qm.ActivateQuest(id); err != nil {
		t.Fatalf("activate %q: %v", id, err)
	}
	qm.MarkCompleted(id)
	if _, err := qm.ClaimRewards(id); err != nil {
		t.Fatalf("claim %q: %v", id, err)
	}
}

// A gated shop is not a shop yet: while the errand is unpaid the NPC resolves to
// the plain conversation, and only the turn-in turns it into a trader/trainer.
func TestShippedServiceGatesWithholdTheShopUntilPaid(t *testing.T) {
	g, qm := bootQuestGiverTest(t)
	for _, tc := range shippedServiceGates {
		t.Run(tc.npcKey, func(t *testing.T) {
			npc, err := character.CreateNPCFromConfig(tc.npcKey, 0, 0)
			if err != nil {
				t.Fatalf("build NPC: %v", err)
			}
			if npc.RequiresQuest != tc.questID {
				t.Fatalf("requires_quest = %q, want %q", npc.RequiresQuest, tc.questID)
			}
			if qm.Definitions()[tc.questID] == nil {
				t.Fatalf("quest %q is not in the shipped catalog", tc.questID)
			}
			// The gate's own giver: the shopkeeper hands out the errand, so a
			// gated shop can always be opened by the party that found it.
			offers := false
			for _, c := range questChoicesOf(npc) {
				if c.Action == "give_quest" && c.QuestID == tc.questID {
					offers = true
				}
			}
			if !offers {
				t.Errorf("%s gates on %q but never offers it", tc.npcKey, tc.questID)
			}

			if got := g.npcDialogKindFor(npc); got != dialogKindChoices {
				t.Fatalf("untaken: dialog kind = %d, want dialogKindChoices (%d)", got, dialogKindChoices)
			}
			if err := qm.ActivateQuest(tc.questID); err != nil {
				t.Fatalf("activate: %v", err)
			}
			if got := g.npcDialogKindFor(npc); got != dialogKindChoices {
				t.Fatalf("in progress: dialog kind = %d, want the shop still shut", got)
			}
			qm.MarkCompleted(tc.questID)
			if got := g.npcDialogKindFor(npc); got != dialogKindChoices {
				t.Fatalf("done but unpaid: dialog kind = %d, want the shop still shut", got)
			}
			if _, err := qm.ClaimRewards(tc.questID); err != nil {
				t.Fatalf("claim: %v", err)
			}
			if got := g.npcDialogKindFor(npc); got != tc.openKind {
				t.Fatalf("paid out: dialog kind = %d, want %d", got, tc.openKind)
			}
		})
	}
}

// While the shop is shut the body text must be the errand, never the shop
// welcome - the trader's Greeting promises rows the party cannot buy yet. The
// two shipped spell traders carry a quest_greeting for exactly that.
func TestGatedTraderLeadsWithTheErrandNotTheShopWelcome(t *testing.T) {
	g, qm := bootQuestGiverTest(t)
	for _, key := range []string{"elf_city_archive", "nomad_city_spells"} {
		t.Run(key, func(t *testing.T) {
			npc, err := character.CreateNPCFromConfig(key, 0, 0)
			if err != nil {
				t.Fatalf("build NPC: %v", err)
			}
			d := npc.DialogueData
			if d.QuestGreeting == "" || d.ActiveMessage == "" || d.CompletedMessage == "" {
				t.Fatalf("gated trader %q is missing quest copy: greeting=%q active=%q completed=%q",
					key, d.QuestGreeting, d.ActiveMessage, d.CompletedMessage)
			}
			g.dialogTab = 0 // a gated trader has no tabs; tab 0 is all there is
			if got := g.npcDialogueText(npc); got != d.QuestGreeting {
				t.Fatalf("gated body = %q, want the quest greeting %q", got, d.QuestGreeting)
			}
			if err := qm.ActivateQuest(npc.RequiresQuest); err != nil {
				t.Fatalf("activate: %v", err)
			}
			if got := g.npcDialogueText(npc); got != d.ActiveMessage {
				t.Fatalf("active body = %q, want %q", got, d.ActiveMessage)
			}
			qm.MarkCompleted(npc.RequiresQuest)
			if got := g.npcDialogueText(npc); got != d.CompletedMessage {
				t.Fatalf("completed body = %q, want %q", got, d.CompletedMessage)
			}
			// Paid out: the shop is open and its own welcome is back on the
			// Spells tab (the trader draws Greeting there directly).
			if _, err := qm.ClaimRewards(npc.RequiresQuest); err != nil {
				t.Fatalf("claim: %v", err)
			}
			if got := g.npcDialogKindFor(npc); got != dialogKindSpellTrader {
				t.Fatalf("paid out: dialog kind = %d, want the spell trader", got)
			}
		})
	}
}

// The bug this pins (reported from play): at the gated desert trainer the
// mastery UI was reachable THROUGH the quest conversation. The dialog drew the
// choices, but the mouse handler still routed by capability, so the invisible
// portrait strip opened the popup and its rows spent gold on training.
func TestGatedTrainerHasNoLiveMasteryClickTargets(t *testing.T) {
	g, qm := bootQuestGiverTest(t)
	ih := NewInputHandler(g)
	npc, err := character.CreateNPCFromConfig("nomad_city_trainer", 0, 0)
	if err != nil {
		t.Fatalf("build NPC: %v", err)
	}
	g.dialogActive = true
	g.dialogNPC = npc
	dlg := npcDialogLayout(g)
	x, y, w, h := skillTrainerPortraitRect(dlg.x, dlg.y, dlg.w, 0)
	clickPortrait := func() {
		g.mouseLeftClicks = []queuedClick{{x: x + w/2, y: y + h/2, at: 1000}}
		ih.handleDialogMouseInput()
	}

	clickPortrait()
	if g.skillTrainerPopup {
		t.Fatal("a click on the (undrawn) portrait strip opened the mastery popup while the errand was unpaid")
	}

	// Even reached directly, the purchase refuses: this is where the gold goes.
	member := g.party.Members[0]
	g.party.Gold = 1_000_000
	g.selectedCharIdx = 0
	g.dialogSelectedSpell = 0
	before := trainerOptions(member)
	if len(before) == 0 {
		t.Fatal("fixture character has no trainable mastery")
	}
	goldBefore := g.party.Gold
	ih.purchaseSelectedTraining()
	if g.party.Gold != goldBefore {
		t.Fatalf("training charged %d gold at a shut trainer", goldBefore-g.party.Gold)
	}

	// Positive control: the same click and the same purchase work once the errand
	// is paid - otherwise this test would pass on a wrong rect or a broken fixture.
	finishQuest(t, qm, npc.RequiresQuest)
	clickPortrait()
	if !g.skillTrainerPopup {
		t.Fatal("the portrait click does not open the popup even with the gate open - the fixture is wrong")
	}
	goldBefore = g.party.Gold
	ih.purchaseSelectedTraining()
	if g.party.Gold == goldBefore {
		t.Fatal("training charged nothing with the gate open - the fixture is wrong")
	}
}

// Same class of hole on the trader side: the shop's portrait strip and icon grid
// must be dead while its errand is unpaid.
func TestGatedTraderHasNoLiveShopClickTargets(t *testing.T) {
	g, qm := bootQuestGiverTest(t)
	ih := NewInputHandler(g)
	npc, err := character.CreateNPCFromConfig("elf_city_archive", 0, 0)
	if err != nil {
		t.Fatalf("build NPC: %v", err)
	}
	g.dialogActive = true
	g.dialogNPC = npc
	if len(g.party.Members) < 3 {
		t.Skip("needs a party of three to move the selection")
	}
	dlg := npcDialogLayout(g)
	x, y, w, h := spellTraderPortraitRect(dlg.x, dlg.y, 2)
	clickThirdPortrait := func() {
		g.selectedCharIdx = 0
		g.mouseLeftClicks = []queuedClick{{x: x + w/2, y: y + h/2, at: 1000}}
		ih.handleDialogMouseInput()
	}

	clickThirdPortrait()
	if g.selectedCharIdx != 0 {
		t.Fatalf("the shop's portrait strip is live at a shut shop (selection moved to %d)", g.selectedCharIdx)
	}

	finishQuest(t, qm, npc.RequiresQuest)
	clickThirdPortrait()
	if g.selectedCharIdx != 2 {
		t.Fatalf("with the gate open the portrait click must select member 2, got %d", g.selectedCharIdx)
	}
}

// Per-step copy (quest_messages) belongs to the Quests TAB of an open shop - and
// a gated trader has no tabs, so its step copy has to reach tab 0 too. Read the
// tab rule off the dialog kind, not off "has spells", or a gated trader with a
// chain falls back to its shop welcome.
func TestGatedTraderWithQuestMessagesShowsStepCopyWithoutTabs(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.questManager = loadTestQuestManager(t)
	const qid = "dragon_cliffs_troll_cull"

	npc := &character.NPC{
		Name:           "Gated Trader",
		RenderCategory: "npc",
		RequiresQuest:  qid,
		SpellData:      map[string]*character.NPCSpell{"heal": {Name: "Heal", Cost: 100}},
		DialogueData: &character.NPCDialogue{
			Greeting:      "shop welcome",
			QuestMessages: map[string]character.NPCQuestMessages{qid: {Offer: "step offer", Active: "step active"}},
			Choices: []*character.NPCDialogueChoice{
				{Text: "Take", Action: "give_quest", QuestID: qid},
				{Text: "Claim", Action: "turn_in_quest", QuestID: qid},
			},
		},
	}

	g.dialogTab = 0
	if got := g.npcDialogueText(npc); got != "step offer" {
		t.Fatalf("gated body on tab 0 = %q, want the step offer", got)
	}
	if err := g.questManager.ActivateQuest(qid); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if got := g.npcDialogueText(npc); got != "step active" {
		t.Fatalf("gated active body = %q, want the step active line", got)
	}
	// Once paid the shop is open: tab 0 is the Spells tab again and the step copy
	// must stay off it (the trader draws its Greeting there).
	g.questManager.MarkCompleted(qid)
	if _, err := g.questManager.ClaimRewards(qid); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if got := g.npcDialogueText(npc); got == "step offer" || got == "step active" {
		t.Fatalf("open shop tab 0 shows step copy %q", got)
	}
}

// The validator's probe asks the ONE dispatch (it builds the NPC and reads
// npcDialogKindFor), so the only things left to pin are its two contracts: it
// must see through a CLOSED gate - otherwise the gates we ship would be rejected
// as gating nothing - and it must not call a plain talker a service.
func TestGatedServiceProbeSeesThroughTheGate(t *testing.T) {
	g, _ := bootQuestGiverTest(t)
	for _, tc := range []struct {
		npcKey string
		want   bool
	}{
		{"elf_city_archive", true},   // gated spell trader
		{"nomad_city_trainer", true}, // gated skill trainer
		{"nomad_city_spells", true},  // gated spell trader
		{"tavern", true},             // service by top-level action, no gate
		{"forest_peasant_wenna", false},
		{"barrel_red", false},
		{"missing_npc_key", false},
	} {
		got, err := g.npcDataHasGatedService(tc.npcKey)
		if err != nil && tc.want {
			t.Errorf("npcDataHasGatedService(%q) failed: %v", tc.npcKey, err)
			continue
		}
		if got != tc.want {
			t.Errorf("npcDataHasGatedService(%q) = %v, want %v", tc.npcKey, got, tc.want)
		}
	}
}

// installPlacedNPCs wires a world manager that SPAWNS the given NPC keys - what
// the validator reads to tell an authored giver from a reachable one. One
// stand-in world answers for every map the shipped quests name, so the map
// checks stay satisfied. Restored after the test.
func installPlacedNPCs(t *testing.T, cfg *config.Config, qm *quests.QuestManager, keys ...string) {
	t.Helper()
	previous := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = previous })

	w := newTestWorld(cfg)
	for _, key := range keys {
		w.NPCs = append(w.NPCs, &character.NPC{Key: key, Name: key})
	}
	wm := world.NewWorldManager(cfg)
	wm.LoadedMaps = map[string]*world.World3D{"forest": w}
	for _, def := range qm.Definitions() {
		mapKeys := []string{def.TargetMap, def.MarkerMap}
		for _, tc := range def.OnCompleteTiles {
			mapKeys = append(mapKeys, tc.Map)
		}
		for _, sp := range def.OnCompleteSpawns {
			mapKeys = append(mapKeys, sp.Map)
		}
		for _, key := range mapKeys {
			if key != "" {
				wm.LoadedMaps[key] = w
			}
		}
	}
	wm.CurrentMapKey = "forest"
	world.GlobalWorldManager = wm
}

// A gate must name a real quest and have something to withhold.
func TestServiceGateValidation(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })

	qc, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	qm := quests.NewQuestManager(qc)

	// The errand a gate points at has to be gettable, so the fixture catalog
	// always contains someone who hands pit_standing out - and that someone is
	// STANDING somewhere (an authored giver on no map gives nothing; see
	// TestServiceGateRejectsAGiverNoMapSpawns).
	giveQuest := func(id string) *character.NPCDialogue {
		return &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{
			{Text: "Take it", Action: "give_quest", QuestID: id},
		}}
	}
	otherGiver := &character.NPCData{
		Type: character.NPCTypeQuestGiver, Dialogue: giveQuest("pit_standing"),
	}
	installPlacedNPCs(t, cfg, qm, "gated", "giver")

	cases := []struct {
		name    string
		npc     *character.NPCData
		wantErr string
	}{
		{
			name: "unknown quest",
			npc: &character.NPCData{
				Type: character.NPCTypeSkillTrainer, RequiresQuest: "no_such_quest",
			},
			wantErr: "unknown requires_quest",
		},
		{
			name: "nothing to gate",
			npc: &character.NPCData{
				Type: character.NPCTypeQuestGiver, RequiresQuest: "pit_standing",
			},
			wantErr: "owns no service to gate",
		},
		{
			name: "trainer that hands out its own gate quest",
			npc: &character.NPCData{
				Type: character.NPCTypeSkillTrainer, RequiresQuest: "pit_standing",
				Dialogue: giveQuest("pit_standing"),
			},
		},
		{
			// A rest nested inside an "ask about lodging" branch does NOT make the
			// NPC a tavern (tavernChoice reads root actions only) - but the gate
			// still withholds it, because the choice filter strips a service at
			// any depth. So it is gateable, and the only thing missing is the
			// greeting that explains the shut door.
			name: "nested service is gateable",
			npc: &character.NPCData{
				Type:          character.NPCTypeQuestGiver,
				RequiresQuest: "pit_standing",
				Dialogue: &character.NPCDialogue{
					Choices: []*character.NPCDialogueChoice{{
						Text: "Ask about lodging", Action: "info",
						Choices: []*character.NPCDialogueChoice{
							{Text: "Rest", Action: "tavern_rest", Cost: 25},
							{Text: "Bless us", Action: "cast_buff", Buff: "bless", Cost: 10, DurationSeconds: 60},
						},
					}},
				},
			},
			wantErr: "needs a quest_greeting",
		},
		{
			// Nothing to withhold at all: plain conversation only.
			name: "nothing to gate",
			npc: &character.NPCData{
				Type:          character.NPCTypeQuestGiver,
				RequiresQuest: "pit_standing",
				Dialogue: &character.NPCDialogue{
					QuestGreeting: "Win three bouts first.",
					Choices: []*character.NPCDialogueChoice{
						{Text: "Who are you?", Action: "info", Response: "Nobody."},
					},
				},
			},
			wantErr: "owns no service to gate",
		},
		{
			name: "root-level buff service is a service",
			npc: &character.NPCData{
				Type:          character.NPCTypeQuestGiver,
				RequiresQuest: "pit_standing",
				Dialogue: &character.NPCDialogue{
					Choices: []*character.NPCDialogueChoice{
						{Text: "Bless us", Action: "cast_buff", Buff: "bless", Cost: 10, DurationSeconds: 60},
						{Text: "Take it", Action: "give_quest", QuestID: "pit_standing"},
					},
				},
			},
		},
		{
			// The typo shape: a real quest id that nobody hands out. The shop would
			// be shut for the whole run and the boot would say nothing.
			name: "gate quest nobody gives",
			npc: &character.NPCData{
				Type: character.NPCTypeSkillTrainer, RequiresQuest: "archive_missing_volumes",
			},
			wantErr: "could never open",
		},
		{
			// Gating on someone else's errand is allowed, but then the shop has to
			// explain itself - otherwise its own greeting sells a shut shelf.
			name: "foreign gate quest without a quest_greeting",
			npc: &character.NPCData{
				Type: character.NPCTypeSkillTrainer, RequiresQuest: "pit_standing",
			},
			wantErr: "needs a quest_greeting",
		},
		{
			name: "foreign gate quest that explains itself",
			npc: &character.NPCData{
				Type: character.NPCTypeSkillTrainer, RequiresQuest: "pit_standing",
				Dialogue: &character.NPCDialogue{QuestGreeting: "Win three bouts first."},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			character.NPCConfigInstance = &character.NPCConfig{
				NPCs: map[string]*character.NPCData{"gated": tc.npc, "giver": otherGiver},
			}
			err := g.validateQuestWorldReferences(qm)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("valid gate rejected: %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("bad gate accepted, want error containing %q", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("error = %v, want it to mention %q", err, tc.wantErr)
			}
		})
	}
}

// "Someone in npcs.yaml offers it" is not enough: a giver that no map spawns
// hands nothing to anybody, so a shop gated behind their errand is shut for the
// whole run. The catalog outlives the maps - an NPC pulled off every map (or
// removed by the open-world stitch) still parses - so placement is what the
// boot check has to ask about.
func TestServiceGateRejectsAGiverNoMapSpawns(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })

	qc, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	qm := quests.NewQuestManager(qc)

	character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{
		"gated": {
			Type: character.NPCTypeSkillTrainer, RequiresQuest: "pit_standing",
			Dialogue: &character.NPCDialogue{QuestGreeting: "Win three bouts first."},
		},
		"giver": {
			Type: character.NPCTypeQuestGiver,
			Dialogue: &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{
				{Text: "Take it", Action: "give_quest", QuestID: "pit_standing"},
			}},
		},
	}}

	// The giver stands on a map: the gate can open.
	installPlacedNPCs(t, cfg, qm, "gated", "giver")
	if err := g.validateQuestWorldReferences(qm); err != nil {
		t.Fatalf("a placed giver was rejected: %v", err)
	}

	// The same catalog with the giver spawned nowhere.
	installPlacedNPCs(t, cfg, qm, "gated")
	err = g.validateQuestWorldReferences(qm)
	if err == nil || !strings.Contains(err.Error(), "could never open") {
		t.Fatalf("error = %v, want the gate rejected because no placed NPC gives the quest", err)
	}

	// A trustworthy loaded world with no NPCs is still a census, and must reject
	// the gate rather than falling back to authored catalog entries.
	installPlacedNPCs(t, cfg, qm)
	err = g.validateQuestWorldReferences(qm)
	if err == nil || !strings.Contains(err.Error(), "could never open") {
		t.Fatalf("empty loaded world: error = %v, want the unplaced giver rejected", err)
	}

	// A catalog-only fixture has no placement census. It may validate authoring,
	// but must not pretend it observed an empty physical world.
	world.GlobalWorldManager = nil
	if err := g.validateQuestWorldReferences(qm); err != nil {
		t.Fatalf("unavailable placement census was treated as an empty world: %v", err)
	}
}

// A gated trader must not open with a live shop SELECTION either: every other
// consumer routes by the dialog kind, and a primed selectedSpellKey is a shop
// row pointing into a conversation that draws no shop.
func TestGatedTraderOpensWithNoSpellSelection(t *testing.T) {
	g, qm := bootQuestGiverTest(t)
	ih := NewInputHandler(g)
	npc, err := character.CreateNPCFromConfig("elf_city_archive", 0, 0)
	if err != nil {
		t.Fatalf("build NPC: %v", err)
	}
	g.selectedSpellKey = "stale"
	ih.openNPCInteraction(npc)
	if g.selectedSpellKey != "" {
		t.Fatalf("a gated trader primed the shop selection with %q", g.selectedSpellKey)
	}

	// Paid out, the shop opens and the first page IS selected.
	finishQuest(t, qm, npc.RequiresQuest)
	ih.openNPCInteraction(npc)
	if g.selectedSpellKey == "" {
		t.Fatal("an open shop selected no page - the fixture is wrong")
	}
}

// A gate withholds services that are plain CHOICES too, not only the tabbed
// ones. The tabs vanish with the dialog kind; a tavern's rest, a pit's bout and
// a priest's blessing are authored choices that kept working - the gated tavern
// still healed the party for gold.
func TestGatedServiceChoicesAreNeitherOfferedNorRun(t *testing.T) {
	g, qm := bootQuestGiverTest(t)
	ih := NewInputHandler(g)

	// The rest is this NPC's ONLY choice, so the dispatcher below has nothing else
	// it could pick: whatever happens to the party is the gate's doing.
	tavern := &character.NPC{
		Name:           "Gated Innkeeper",
		RenderCategory: "npc",
		RequiresQuest:  "pit_standing",
		DialogueData: &character.NPCDialogue{
			Greeting:      "Win three bouts first.",
			QuestGreeting: "Win three bouts first.",
			Choices: []*character.NPCDialogueChoice{
				{Text: "Rest", Action: "tavern_rest", Cost: 25},
			},
		},
	}
	g.dialogActive, g.dialogNPC = true, tavern

	offered := func() bool {
		for _, c := range g.visibleNPCChoices(tavern) {
			if c.Action == "tavern_rest" {
				return true
			}
		}
		return false
	}
	if offered() {
		t.Fatal("a gated tavern still offers its rest")
	}
	if got := g.npcDialogKindFor(tavern); got != dialogKindChoices {
		t.Fatalf("gated tavern dialog kind = %d, want the plain conversation", got)
	}

	// And no route through the dialog reaches it: run EVERY choice the shut
	// tavern does offer (it still carries its rumor branch) and the party is
	// neither charged nor healed.
	g.party.Gold = 1000
	member := g.party.Members[0]
	member.MaxHitPoints, member.HitPoints = 50, 10
	for i := range g.visibleNPCChoices(tavern) {
		g.dialogActive, g.dialogNPC, g.dialogNodePath = true, tavern, nil
		g.selectedChoice = i
		ih.executeEncounterChoice()
	}
	if g.party.Gold != 1000 || member.HitPoints != 10 {
		t.Fatalf("a shut tavern rested the party: gold %d, hp %d", g.party.Gold, member.HitPoints)
	}

	// Positive control: paid out, the rest is offered again and it does work -
	// otherwise this test would pass on a broken fixture. Back to the root of the
	// conversation first: the loop above descended into the rumor branch, and a
	// node lists only its own follow-ups.
	finishQuest(t, qm, "pit_standing")
	g.dialogActive, g.dialogNPC, g.dialogNodePath = true, tavern, nil
	if !offered() {
		t.Fatal("the rest never returns once the errand is paid - the fixture is wrong")
	}
	for i, c := range g.visibleNPCChoices(tavern) {
		if c.Action == "tavern_rest" {
			g.selectedChoice = i
		}
	}
	ih.executeEncounterChoice()
	if g.party.Gold == 1000 {
		t.Fatal("the rest charged nothing with the gate open - the fixture is wrong")
	}
}

// A roster swap can shrink the party under a stale selection while a shop is
// open. Both purchase paths carry the same bounds guard - one of them used to
// index straight into the slice.
func TestPurchasePathsSurviveAStaleCharacterSelection(t *testing.T) {
	g, qm := bootQuestGiverTest(t)
	ih := NewInputHandler(g)
	for _, tc := range []struct {
		npcKey   string
		purchase func()
	}{
		{"elf_city_archive", ih.purchaseSelectedSpell},
		{"nomad_city_trainer", ih.purchaseSelectedTraining},
	} {
		t.Run(tc.npcKey, func(t *testing.T) {
			npc, err := character.CreateNPCFromConfig(tc.npcKey, 0, 0)
			if err != nil {
				t.Fatalf("build NPC: %v", err)
			}
			finishQuest(t, qm, npc.RequiresQuest) // the shop must be OPEN, or the gate returns first
			g.dialogActive, g.dialogNPC = true, npc
			g.party.Gold = 100000
			g.selectedCharIdx = len(g.party.Members) // the seat that just left
			g.dialogSelectedSpell = 0
			for key := range npc.SpellData {
				g.selectedSpellKey = key
				break
			}
			gold := g.party.Gold
			tc.purchase() // must not panic
			if g.party.Gold != gold {
				t.Fatalf("a purchase went through for a party member that is not there (gold %d -> %d)", gold, g.party.Gold)
			}
		})
	}
}

// The validator and the runtime must agree on what a "service" is. A duel
// master's bout is a plain choice (no tabbed dialog of its own), so a probe that
// only recognised dialog KINDS refused to let it be gated - the boot panicked
// over authoring the gate would have handled correctly.
func TestAGateableServiceNeedNotBeATabbedDialog(t *testing.T) {
	g, _ := bootQuestGiverTest(t)
	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })
	character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{
		"duel_master": {
			Type:           character.NPCTypeQuestGiver,
			RenderCategory: "npc",
			Dialogue: &character.NPCDialogue{
				Greeting: "Care to test the sand?",
				Choices: []*character.NPCDialogueChoice{
					{Text: "Fight", Action: "start_arena_duel", Tier: "normal"},
				},
			},
		},
	}}
	has, err := g.npcDataHasGatedService("duel_master")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if !has {
		t.Fatal("an NPC whose only service is a duel is reported as having nothing to gate")
	}
}

// A gate exists to OPEN a service, so the service must outlive the errand that
// opens it. Turning the quest in at its own giver concludes that NPC, and a
// concluded NPC used to show no choices at all - so a shop delivered as a
// dialogue row (a bout, a rest, a blessing) vanished the moment it was unlocked.
func TestGatedServiceSurvivesItsOwnQuest(t *testing.T) {
	g, qm := bootQuestGiverTest(t)
	npc := &character.NPC{
		Name: "Duel Master", RenderCategory: "npc", RequiresQuest: "pit_standing",
		DialogueData: &character.NPCDialogue{
			Greeting: "Prove yourself first.",
			Choices: []*character.NPCDialogueChoice{
				{Text: "Take the errand", Action: "give_quest", QuestID: "pit_standing"},
				{Text: "Claim", Action: "turn_in_quest", QuestID: "pit_standing"},
				// A BOUT, not a blessing: paid casts are drawn as icon rows on the
				// buff tab, so they never travel through visibleNPCChoices. A plain
				// duel master resolves to dialogKindChoices and its row does.
				{Text: "Fight", Action: "start_arena_duel", Tier: "normal"},
			},
		},
	}
	offers := func(action string) bool {
		for _, c := range g.visibleNPCChoices(npc) {
			if c.Action == action {
				return true
			}
		}
		return false
	}
	if offers("start_arena_duel") {
		t.Fatal("the bout is on offer while the errand is unpaid")
	}

	// Finish and turn it in AT THIS NPC - the path that marks them Visited.
	finishQuest(t, qm, "pit_standing")
	npc.Visited = true // handleTurnInQuest does this once the chain has no step left
	if g.npcDialogueState(npc) != npcStateConcluded {
		t.Fatal("fixture: the giver should be concluded after the turn-in")
	}

	if !offers("start_arena_duel") {
		t.Fatal("the service the gate just opened is gone: a concluded giver kept none of its rows")
	}
	// The quest rows ARE spent, though - a concluded chain must not re-offer itself.
	if offers("give_quest") || offers("turn_in_quest") {
		t.Fatal("a concluded giver still offers its finished errand")
	}
}

// A prop credits ITS OWN errand. quest_id alone routes the bump now, so a
// copy-pasted id would let a valve advance the lamps - and consume itself doing
// it. The family's tag is the second key, checked at boot and at the counter.
func TestQuestPropRefusesAnotherErrand(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.questManager = loadTestQuestManager(t)
	ih := NewInputHandler(g)
	for _, id := range []string{"culverts_valves", "shrine_lamps"} {
		if err := g.questManager.ActivateQuest(id); err != nil {
			t.Fatalf("activate %s: %v", id, err)
		}
	}
	valve := &character.NPC{Name: "Sluice Valve", RenderCategory: "npc"}
	g.dialogNPC = valve
	ih.handleQuestPropInteract("shrine_lamps", shippedQuestProps(t)["valve"].Prop)

	if c := g.questManager.GetQuest("shrine_lamps").CurrentCount; c != 0 {
		t.Fatalf("a valve advanced the LAMP errand (%d)", c)
	}
	if valve.Visited {
		t.Fatal("the valve consumed itself crediting somebody else's errand")
	}
	// Its own errand still works.
	g.dialogNPC = valve
	ih.handleQuestPropInteract("culverts_valves", shippedQuestProps(t)["valve"].Prop)
	if c := g.questManager.GetQuest("culverts_valves").CurrentCount; c != 1 {
		t.Fatalf("the valve did not credit its own errand (%d)", c)
	}
}

// The trader's second tab follows what it would SHOW. Once the errand is turned
// in the giver is concluded and has no quest rows left, so the strip must go
// with them - otherwise "Spells | Quests" stays on screen for the rest of the
// run and opens a blank panel. Strip, Tab key and click router share the
// predicate, so they cannot disagree about whether the tab is there.
func TestTraderQuestTabFollowsItsRows(t *testing.T) {
	g, qm := bootQuestGiverTest(t)
	npc, err := character.CreateNPCFromConfig("elf_city_archive", 0, 0)
	if err != nil {
		t.Fatalf("build NPC: %v", err)
	}
	if !g.npcDialogHasTalkTab(npc) {
		t.Fatal("a trader still offering its errand must carry the Quests tab")
	}
	// Standing inside an info branch must not change the answer: the strip and the
	// Tab key gated on it would vanish mid-conversation.
	g.dialogNodePath = []*character.NPCDialogueChoice{
		{Text: "Tell me more", Action: "info", Response: "A branch with no rows of its own."},
	}
	if !g.npcDialogHasTalkTab(npc) {
		t.Fatal("descending into an info branch hid the Quests tab")
	}
	g.dialogNodePath = nil

	finishQuest(t, qm, npc.RequiresQuest)
	npc.Visited = true // what handleTurnInQuest does once the chain is spent
	if g.npcDialogHasTalkTab(npc) {
		t.Fatalf("a concluded giver still shows a Quests tab over %d rows", len(g.visibleNPCChoices(npc)))
	}
}

// A buff seller's Talk tab obeys the SAME rule as a trader's Quests tab: both
// ask the rows the tab would draw. Its paid casts are icon rows on the service
// tab, so they never count as conversation - and a talk row still waiting on a
// prerequisite is not there yet either.
func TestBuffServiceTalkTabFollowsItsRows(t *testing.T) {
	g, qm := bootQuestGiverTest(t)
	gate := "culverts_valves" // any authored quest: what matters is that it is unfinished
	npc := &character.NPC{
		Name:           "Waterwalker",
		RenderCategory: "npc",
		DialogueData: &character.NPCDialogue{
			Greeting: "The lake is cold this season.",
			Choices: []*character.NPCDialogueChoice{
				{Text: "Walk on water", Action: "cast_buff", Buff: "walk_on_water", DurationSeconds: 300, Cost: 200},
				{Text: "About the culverts", Action: "info", RequiresQuest: gate, Response: "Later."},
			},
		},
	}
	if kind := g.npcDialogKindFor(npc); kind != dialogKindBuffService {
		t.Fatalf("fixture resolves to %s, want the buff service dialog", kind)
	}
	if g.npcDialogHasTalkTab(npc) {
		t.Fatalf("the only talk row is still gated, but the Talk tab is drawn over %d rows",
			len(g.visibleNPCChoices(npc)))
	}

	finishQuest(t, qm, gate)
	if !g.npcDialogHasTalkTab(npc) {
		t.Fatal("the prerequisite is paid out, so the talk row - and its tab - must be back")
	}
	// The cast rows themselves are never conversation: strip the talk row and the
	// tab goes with it, even though the NPC still has authored choices.
	npc.DialogueData.Choices = npc.DialogueData.Choices[:1]
	if g.npcDialogHasTalkTab(npc) {
		t.Fatal("paid casts alone raised a Talk tab that opens an empty panel")
	}
}

// SHIPPED CONTENT: every interact quest's tag must be produced by something -
// an authored prop or the arena pit. A renamed constant or a typo'd
// target_monster locks the errand silently, and with it anything gated behind it
// (Nadira's mastery training waits on the arena tag).
func TestEveryInteractQuestTagHasAProducer(t *testing.T) {
	_, qm := bootQuestGiverTest(t)
	// The BOOT entry point, on the shipped catalogs.
	if err := ValidateInteractTagProducers(qm); err != nil {
		t.Fatalf("shipped catalogs: %v", err)
	}
	producers := interactTagProducers(character.NPCConfigInstance.NPCs)
	// A quest counting a tag nobody produces is refused.
	qm.Definitions()["typo_errand"] = &quests.QuestDefinition{
		Name: "Typo", Type: quests.QuestTypeInteract, TargetMonster: "shrine_lantern",
		TargetCount: 3, ProgressText: "lanterns lifted",
	}
	t.Cleanup(func() { delete(qm.Definitions(), "typo_errand") })
	if err := ValidateInteractTagProducers(qm); err == nil ||
		!strings.Contains(err.Error(), "shrine_lantern") {
		t.Fatalf("error = %v, want the uncredited tag rejected", err)
	}
	delete(qm.Definitions(), "typo_errand")
	// And the arena's Go-side constant is one of them, marked as CODE-produced:
	// the pit is the only producer that is not authored content, and that mark is
	// what exempts it from the placed-prop count.
	if source, ok := producers[quests.ArenaDuelTag]; !ok || source != interactTagFromCode {
		t.Fatalf("the arena tag %q is produced by %v (ok=%v), want a code tag",
			quests.ArenaDuelTag, source, ok)
	}
	for tag, source := range producers {
		if tag != quests.ArenaDuelTag && source != interactTagFromProp {
			t.Errorf("authored tag %q is marked code-produced, so its props would never be counted", tag)
		}
	}
	if def := qm.Definitions()["pit_standing"]; def == nil || def.TargetMonster != quests.ArenaDuelTag {
		t.Fatalf("pit_standing counts %v, want the arena tag %q", def, quests.ArenaDuelTag)
	}
}

// Spell rows only sell if the KIND dispatch resolves to the trader dialog. The
// authored type merely gets them copied, and tavern / buff service / card
// collector / arena gladiator all win first - so a trader that also rents rooms
// would show no shop and say nothing about it.
func TestSpellShopsAreReachable(t *testing.T) {
	g, _ := bootQuestGiverTest(t)
	if err := g.validateSpellShopsAreReachable(); err != nil {
		t.Fatalf("shipped spell shops: %v", err)
	}

	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })
	rows := map[string]*character.NPCSpell{"heal": {Name: "Heal", Cost: 100}}
	for _, tc := range []struct {
		name string
		npc  *character.NPCData
	}{
		{
			name: "a trader that also rents rooms",
			npc: &character.NPCData{
				Type: character.NPCTypeSpellTrader, RenderCategory: "npc", Spells: rows,
				Dialogue: &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{
					{Text: "Rest", Action: "tavern_rest", Cost: 25},
				}},
			},
		},
		{
			name: "a trader that also blesses",
			npc: &character.NPCData{
				Type: character.NPCTypeSpellTrader, RenderCategory: "npc", Spells: rows,
				Dialogue: &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{
					{Text: "Bless", Action: "cast_buff", Buff: "bless", Cost: 10, DurationSeconds: 60},
				}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{"shop": tc.npc}}
			err := g.validateSpellShopsAreReachable()
			if err == nil || !strings.Contains(err.Error(), "would never be drawn") {
				t.Fatalf("error = %v, want the unreachable shop rejected", err)
			}
		})
	}
}

// Every service the validator accepts as gateable must actually BE withheld -
// either because the gate downgrades the dialog kind (the tabbed ones) or
// because its action is in gatedServiceActions.
func TestEveryGateableServiceActionIsWithheld(t *testing.T) {
	for _, action := range []string{"tavern_rest", "buy_food", "open_roster", "manage_stash",
		"cast_buff", "start_arena_duel", "wait_until_night", "wait_until_dawn"} {
		if !gatedServiceActions[action] {
			t.Errorf("%q is a service action that no gate withholds", action)
		}
	}
	// Ordinary conversation is never withheld - a gated NPC must still talk, and
	// hand out the very errand that opens it.
	for _, action := range []string{"info", "leave", "give_quest", "turn_in_quest", "back", "open_door"} {
		if gatedServiceActions[action] {
			t.Errorf("%q is not a service - a gate must not hide it", action)
		}
	}
}

// A giver whose OFFER is itself locked behind an unreachable quest gives
// nothing. The validator's job is to reject a service that can never open, and
// an offer gate is as final as no offer at all.
func TestServiceGateRejectsAnOfferLockedBehindAnUnreachableQuest(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })

	qc, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	qm := quests.NewQuestManager(qc)
	installPlacedNPCs(t, cfg, qm, "gated", "giver")

	gated := &character.NPCData{
		Type: character.NPCTypeSkillTrainer, RequiresQuest: "pit_standing",
		Dialogue: &character.NPCDialogue{QuestGreeting: "Win three bouts first."},
	}
	offer := func(requires string) *character.NPCData {
		return &character.NPCData{
			Type: character.NPCTypeQuestGiver,
			Dialogue: &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{
				{Text: "Take it", Action: "give_quest", QuestID: "pit_standing", RequiresQuest: requires},
			}},
		}
	}

	// The offer waits on an errand nobody hands out: the pit quest can never be
	// taken, so the trainer can never open.
	character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{
		"gated": gated, "giver": offer("archive_missing_volumes"),
	}}
	err = g.validateQuestWorldReferences(qm)
	if err == nil || !strings.Contains(err.Error(), "could never open") {
		t.Fatalf("error = %v, want the locked offer rejected", err)
	}

	// An offer gated behind an errand the same giver hands out is fine.
	reachable := offer("shrine_lamps")
	reachable.Dialogue.Choices = append(reachable.Dialogue.Choices,
		&character.NPCDialogueChoice{Text: "First things first", Action: "give_quest", QuestID: "shrine_lamps"})
	character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{
		"gated": gated, "giver": reachable,
	}}
	if err := g.validateQuestWorldReferences(qm); err != nil {
		t.Fatalf("a reachable offer chain was rejected: %v", err)
	}

	// A cycle (the offer waits on the very quest it offers) reaches nothing and
	// must not spin forever either.
	character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{
		"gated": gated, "giver": offer("pit_standing"),
	}}
	err = g.validateQuestWorldReferences(qm)
	if err == nil || !strings.Contains(err.Error(), "could never open") {
		t.Fatalf("error = %v, want the self-gated offer rejected", err)
	}

	// A child offer inherits its parent's gate. A flat recursive scan used to
	// find the child and discard the unreachable ancestor.
	child := offer("").Dialogue.Choices[0]
	character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{
		"gated": gated,
		"giver": {
			Type: character.NPCTypeQuestGiver,
			Dialogue: &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{{
				Text: "Locked branch", Action: "info", RequiresQuest: "archive_missing_volumes",
				Choices: []*character.NPCDialogueChoice{child},
			}}},
		},
	}}
	err = g.validateQuestWorldReferences(qm)
	if err == nil || !strings.Contains(err.Error(), "could never open") {
		t.Fatalf("offer behind an unreachable parent gate was accepted: %v", err)
	}

	// Only info rows open children at runtime. A valid offer nested under any
	// other action is physically unreachable even without a quest gate.
	character.NPCConfigInstance.NPCs["giver"].Dialogue.Choices[0].Action = "leave"
	character.NPCConfigInstance.NPCs["giver"].Dialogue.Choices[0].RequiresQuest = ""
	err = g.validateQuestWorldReferences(qm)
	if err == nil || !strings.Contains(err.Error(), "could never open") {
		t.Fatalf("offer below a non-navigation row was accepted: %v", err)
	}
}

func TestFixedDialogServiceRowsMatchTheirRealSurface(t *testing.T) {
	g, _ := bootQuestGiverTest(t)
	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })

	for _, tc := range []struct {
		name    string
		npc     *character.NPCData
		wantErr bool
	}{
		{
			name: "root tavern rest is drawn",
			npc: &character.NPCData{Type: character.NPCTypeEncounter, Dialogue: &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{
				{Text: "Rest", Action: "tavern_rest"},
			}}},
		},
		{
			name: "nested tavern rest is not a tab",
			npc: &character.NPCData{Type: character.NPCTypeEncounter, Dialogue: &character.NPCDialogue{
				Choices: []*character.NPCDialogueChoice{
					{Text: "Rest", Action: "tavern_rest"},
					{Text: "Services", Action: "info", Choices: []*character.NPCDialogueChoice{{Text: "Food", Action: "buy_food"}}},
				},
			}},
			wantErr: true,
		},
		{
			name: "card collector wins over tavern",
			npc: &character.NPCData{Type: character.NPCTypeCardCollector, Dialogue: &character.NPCDialogue{
				Choices: []*character.NPCDialogueChoice{
					{Text: "Rest", Action: "tavern_rest"},
				},
			}},
			wantErr: true,
		},
		{
			name: "choices surface draws nested conversation",
			npc: &character.NPCData{Type: character.NPCTypeEncounter, Dialogue: &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{
				{Text: "Ask", Action: "info", Choices: []*character.NPCDialogueChoice{{Text: "Back", Action: "back"}}},
			}}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{"subject": tc.npc}}
			err := g.validateDialogueRowsAreDrawable()
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateDialogueRowsAreDrawable() error = %v, want error=%v", err, tc.wantErr)
			}
		})
	}
}

// A prop credits its quest through OnInteract, which only moves INTERACT quests
// that carry a tag. Pointed at anything else the prop still consumes itself and
// advances nothing - a permanent soft-lock, so it is a boot failure.
func TestQuestPropMustPointAtATaggedInteractQuest(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })

	prop := func(questID, tag string) *character.NPCConfig {
		return &character.NPCConfig{NPCs: map[string]*character.NPCData{
			"lamp": {
				Type:            character.NPCTypeEncounter,
				HideWhenVisited: true, // a lifted lamp leaves the world, like the shipped ones
				Dialogue: &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{
					{Text: "Lift it", Action: questPropAction, QuestID: questID,
						Prop: &character.NPCPropCopy{
							Tag: tag, NotYet: "Not yet.", Took: "Lifted.", Completed: "Done.",
						}},
				}},
			},
		}}
	}
	// The shipped catalog (the rest of the validator checks the real monsters and
	// maps against it), plus one synthetic tagless errand that content does not
	// contain and should not have to.
	qc, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	qm := quests.NewQuestManager(qc)
	qm.Definitions()["untagged"] = &quests.QuestDefinition{
		Name: "Untagged", Type: quests.QuestTypeInteract, TargetCount: 3,
	}

	character.NPCConfigInstance = prop("shrine_lamps", "shrine_lamp")
	if err := g.validateQuestWorldReferences(qm); err != nil {
		t.Fatalf("a well-formed prop was rejected: %v", err)
	}
	// Neither exit from the spent state: no hide_when_visited and no
	// visited_message, so a second try would close the dialog in silence.
	silent := prop("shrine_lamps", "shrine_lamp")
	silent.NPCs["lamp"].HideWhenVisited = false
	silent.NPCs["lamp"].Dialogue.VisitedMessage = ""
	character.NPCConfigInstance = silent
	if err := g.validateQuestWorldReferences(qm); err == nil ||
		!strings.Contains(err.Error(), "neither hides when visited nor authors a visited_message") {
		t.Errorf("error = %v, want the silent spent prop rejected", err)
	}

	for _, tc := range []struct{ questID, tag, wants string }{
		{"wolf_pack", "shrine_lamp", `want "interact"`},
		{"untagged", "shrine_lamp", "no interact tag"},
		// The prop's own tag must be the quest's - quest_id alone routes the credit.
		{"culverts_valves", "shrine_lamp", "credits tag"},
		{"shrine_lamps", "", "credits tag"},
	} {
		character.NPCConfigInstance = prop(tc.questID, tc.tag)
		err := g.validateQuestWorldReferences(qm)
		if err == nil || !strings.Contains(err.Error(), tc.wants) {
			t.Errorf("prop on quest %q: error = %v, want it to mention %q", tc.questID, err, tc.wants)
		}
	}
}

// Every interact quest says what it is COUNTING in its own words. The derived
// wording was frozen at "closed" from when valves were the only interact quest,
// so the pit read "1/3 arena duels closed" and the lamps "1/3 shrine lamps
// closed" - in the combat log and in the journal.
func TestInteractQuestsAuthorTheirOwnProgressWording(t *testing.T) {
	qc, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	qm := quests.NewQuestManager(qc)
	for id, def := range qc.Quests {
		if def.Type != quests.QuestTypeInteract {
			continue
		}
		if err := qm.ActivateQuest(id); err != nil {
			continue // already active (the endgame gates)
		}
		got := qm.GetQuest(id).GetProgressString()
		if strings.Contains(got, "closed") && !strings.Contains(def.ProgressText, "closed") {
			t.Errorf("quest %q progress reads %q - the valve verb leaked", id, got)
		}
		if !strings.HasPrefix(got, "0/") || !strings.Contains(got, def.ProgressText) {
			t.Errorf("quest %q progress = %q, want it to use the authored %q", id, got, def.ProgressText)
		}
	}
	// The shipped wording, spelled out: this is what the player reads.
	for id, want := range map[string]string{
		"pit_standing":    "0/3 arena duels won",
		"shrine_lamps":    "0/3 shrine lamps lifted",
		"culverts_valves": "0/7 valves closed",
	} {
		if q := qm.GetQuest(id); q == nil {
			t.Errorf("%s is not active in the fixture", id)
		} else if got := q.GetProgressString(); got != want {
			t.Errorf("%s reads %q, want %q", id, got, want)
		}
	}
}

// A prop is one-shot and often hide_when_visited, so it must not be spent on a
// bump the manager refused - that deletes the only way to finish the errand.
func TestQuestPropIsNotConsumedWhenNothingWasCredited(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	// An ACTIVE quest whose type the interact hook does not advance. Boot
	// validation rejects this shape; the handler must survive it anyway, since
	// it is the thing that eats the prop.
	g.questManager = quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{
		"mismatched": {
			Name: "Mismatched", Type: quests.QuestTypeKill,
			TargetMonster: "shrine_lamp", TargetCount: 3,
		},
		// A NEIGHBOUR that wants the same tag. A refused prop stays live, so if
		// the credit went out as a tag broadcast the player could re-open the prop
		// and pump this counter for free.
		"neighbour": {
			Name: "Neighbour", Type: quests.QuestTypeInteract,
			TargetMonster: "shrine_lamp", TargetCount: 3, ProgressText: "lamps lifted",
		},
	}})
	for _, id := range []string{"mismatched", "neighbour"} {
		if err := g.questManager.ActivateQuest(id); err != nil {
			t.Fatalf("activate %s: %v", id, err)
		}
	}
	lamp := &character.NPC{
		Name: "Shrine Lamp", RenderCategory: "npc",
		DialogueData: &character.NPCDialogue{Greeting: "A brass lamp."},
	}
	ih := NewInputHandler(g)
	g.dialogNPC = lamp
	before := len(g.GetCombatMessages())
	ih.handleQuestPropInteract("mismatched", shippedQuestProps(t)["shrine_lamp"].Prop)

	if lamp.Visited {
		t.Fatal("the lamp was consumed although the quest counter never moved")
	}
	if c := g.questManager.GetQuest("mismatched").CurrentCount; c != 0 {
		t.Fatalf("counter moved to %d on a type the interact hook does not advance", c)
	}
	if c := g.questManager.GetQuest("neighbour").CurrentCount; c != 0 {
		t.Fatalf("the refused prop credited a NEIGHBOUR quest (%d) - and it is still live to do it again", c)
	}
	for _, m := range g.GetCombatMessages()[before:] {
		if strings.Contains(m, "0/3") {
			t.Fatalf("the prop reported fake progress: %q", m)
		}
	}
}

// A prop row and the reserved action travel together. executeEncounterChoice
// routes on the `prop:` block BEFORE the action switch, so a prop authored onto a
// dispatched action would be classified as that action by every rule that reads
// one (a service withheld by requires_quest, a row that survives conclusion) and
// still run the prop handler instead. The reverse is just as dead: the reserved
// action with no block falls through the switch and does nothing.
func TestQuestPropOwnsItsOwnAction(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })

	row := func(action string, prop *character.NPCPropCopy) *character.NPCConfig {
		return &character.NPCConfig{NPCs: map[string]*character.NPCData{
			"thing": {
				Type: character.NPCTypeEncounter, HideWhenVisited: true,
				Dialogue: &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{
					{Text: "Do it", Action: action, QuestID: "culverts_valves", Prop: prop},
				}},
			},
		}}
	}
	good := &character.NPCPropCopy{Tag: "valve", NotYet: "Not yet.", Took: "Shut.", Completed: "Done."}

	for _, tc := range []struct {
		name   string
		config *character.NPCConfig
	}{
		{"a prop on a service action", row("tavern_rest", good)},
		{"a prop on a dispatched action", row("open_door", good)},
		{"the reserved action with no prop block", row(questPropAction, nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			qm := loadTestQuestManager(t)
			character.NPCConfigInstance = tc.config
			installPlacedNPCs(t, cfg, qm, "thing")
			err := g.validateQuestWorldReferences(qm)
			if err == nil || !strings.Contains(err.Error(), "must declare action") {
				t.Fatalf("accepted: %v", err)
			}
		})
	}

	// Positive control: the reserved action WITH a well-formed block passes.
	qm := loadTestQuestManager(t)
	character.NPCConfigInstance = row(questPropAction, good)
	installPlacedNPCs(t, cfg, qm, "thing")
	if err := g.validateQuestWorldReferences(qm); err != nil {
		t.Fatalf("a well-formed prop was rejected: %v", err)
	}
	// And the shipped props all declare it.
	for tag, prop := range shippedQuestProps(t) {
		if prop.Action != questPropAction {
			t.Errorf("shipped prop %q declares action %q", tag, prop.Action)
		}
	}
}

// shippedQuestProps collects every authored quest prop from npcs.yaml, keyed by
// its TAG - every prop row declares the same reserved action, and the tag is what
// identifies one. The props themselves are the content now, so the tests read
// them instead of a Go table.
func shippedQuestProps(t *testing.T) map[string]*character.NPCDialogueChoice {
	t.Helper()
	if character.NPCConfigInstance == nil {
		if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
			t.Fatalf("load npcs: %v", err)
		}
	}
	props := map[string]*character.NPCDialogueChoice{}
	for _, npc := range character.NPCConfigInstance.NPCs {
		if npc == nil {
			continue
		}
		_ = npc.Dialogue.WalkChoices(func(c *character.NPCDialogueChoice) error {
			if c.Prop != nil {
				props[c.Prop.Tag] = c
			}
			return nil
		})
	}
	if len(props) == 0 {
		t.Fatal("no quest props are authored in npcs.yaml")
	}
	return props
}

// Every quest prop shares one body: an active quest is required, the prop is
// one-shot, and the credit lands on the quest's own interact tag.
func TestQuestPropInteractionsAreOneShotAndGated(t *testing.T) {
	bootQuestGiverTest(t) // loads the shipped catalog
	for action, choice := range shippedQuestProps(t) {
		words := choice.Prop
		t.Run(action, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			g.questManager = loadTestQuestManager(t)
			ih := NewInputHandler(g)

			// The quest that counts THIS family's tag - a prop may only credit its
			// own errand now, so the pairing is part of what is being tested.
			qid := ""
			for id, def := range g.questManager.Definitions() {
				if def.Type == quests.QuestTypeInteract && def.TargetMonster == words.Tag {
					qid = id
				}
			}
			if qid == "" {
				t.Fatalf("no shipped interact quest counts %q - the prop credits nothing", words.Tag)
			}
			prop := &character.NPC{Name: "Prop", RenderCategory: "scenery"}

			g.dialogNPC = prop
			ih.handleQuestPropInteract(qid, words)
			if q := g.questManager.GetQuest(qid); q != nil {
				t.Fatalf("an untaken quest must not be advanced by a prop (count %d)", q.CurrentCount)
			}
			if prop.Visited {
				t.Fatal("a prop consumed itself for a quest nobody took")
			}

			if err := g.questManager.ActivateQuest(qid); err != nil {
				t.Fatalf("activate: %v", err)
			}
			g.dialogNPC = prop
			ih.handleQuestPropInteract(qid, words)
			q := g.questManager.GetQuest(qid)
			if q.CurrentCount != 1 {
				t.Fatalf("progress = %d, want 1", q.CurrentCount)
			}
			if !prop.Visited {
				t.Fatal("a used prop must be Visited so it cannot be counted twice")
			}

			// Used again: no credit, and it SAYS so from its authored
			// visited_message rather than closing the dialog in silence (a prop
			// authored as a repeatable encounter keeps offering its row).
			prop.DialogueData = &character.NPCDialogue{VisitedMessage: "It is already spent."}
			g.dialogNPC = prop
			before := len(g.GetCombatMessages())
			ih.handleQuestPropInteract(qid, words)
			if q.CurrentCount != 1 {
				t.Fatalf("progress = %d after a second try on the same prop, want 1", q.CurrentCount)
			}
			said := strings.Join(g.GetCombatMessages()[before:], " | ")
			if !strings.Contains(said, "already spent") {
				t.Fatalf("a spent prop answered %q, want its visited_message", said)
			}

			// A finished quest is no longer ACTIVE: a fresh prop must refuse
			// instead of consuming itself for credit that cannot land.
			g.questManager.MarkCompleted(qid) // completion fills the counter to target
			settled := q.CurrentCount
			fresh := &character.NPC{Name: "Prop", RenderCategory: "scenery"}
			g.dialogNPC = fresh
			ih.handleQuestPropInteract(qid, words)
			if fresh.Visited {
				t.Fatal("a prop consumed itself for a quest that is already done")
			}
			if q.CurrentCount != settled {
				t.Fatalf("progress moved from %d to %d after the quest was completed", settled, q.CurrentCount)
			}
		})
	}
}

// Every prop family that pays loot must name the thing it came out of: the
// shared body refuses to invent the noun, so a missing line means the party gets
// items and gold with no message at all.
func TestQuestPropLootTablesCarryTheirLine(t *testing.T) {
	g, qm := bootQuestGiverTest(t)
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loot tables: %v", err)
	}
	for action, choice := range shippedQuestProps(t) {
		words := choice.Prop
		if words.LootTable != "" {
			if words.LootLine == "" {
				t.Errorf("prop %q rolls loot table %q with no loot_line - the payout would be silent",
					action, words.LootTable)
			} else if !strings.Contains(words.LootLine, "%s") {
				t.Errorf("prop %q loot_line %q has no %%s for the item list", action, words.LootLine)
			}
		}
		// Every prop says SOMETHING when it advances, loot or not.
		if words.Took == "" || words.NotYet == "" || words.Completed == "" {
			t.Errorf("prop %q is missing copy: took=%q not_yet=%q completed=%q",
				action, words.Took, words.NotYet, words.Completed)
		}
	}

	// And boot rejects a loot table that does not exist or a line fmt.Sprintf
	// would garble - both reach the player as a consumed prop and no payout.
	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })
	rack := func(table, line string) *character.NPCConfig {
		return &character.NPCConfig{NPCs: map[string]*character.NPCData{
			"rack": {
				Type: character.NPCTypeEncounter,
				Dialogue: &character.NPCDialogue{
					VisitedMessage: "The rack stands empty.",
					Choices: []*character.NPCDialogueChoice{
						{Text: "Strip it", Action: questPropAction, QuestID: "castle_armory",
							Prop: &character.NPCPropCopy{
								Tag: "sword_rack", NotYet: "n", Took: "t", Completed: "c",
								LootTable: table, LootLine: line,
							}},
					},
				},
			},
		}}
	}
	for _, tc := range []struct{ table, line, wants string }{
		{"castle_armoury", "You take: %s.", "unknown loot table"}, // typo
		{"castle_armory", "You take from the rack.", "exactly one %s"},
		{"castle_armory", "You take %s and %s.", "exactly one %s"},
	} {
		character.NPCConfigInstance = rack(tc.table, tc.line)
		err := g.validateQuestWorldReferences(qm)
		if err == nil || !strings.Contains(err.Error(), tc.wants) {
			t.Errorf("loot table %q line %q: error = %v, want it to mention %q", tc.table, tc.line, err, tc.wants)
		}
	}
	character.NPCConfigInstance = rack("castle_armory", "You take from the rack: %s.")
	if err := g.validateQuestWorldReferences(qm); err != nil {
		t.Fatalf("a well-formed loot prop was rejected: %v", err)
	}
}

// Shipped content: the Archive's Town Portal page must be sellable to an Air
// caster. The spell is authored earth AND air, and no starting class opens Earth
// alongside Air - so gating the row on the primary school alone locked every
// sorcerer party out of town portals for the whole game.
func TestArchiveSellsTownPortalToAnAirCaster(t *testing.T) {
	g, qm := bootQuestGiverTest(t)
	npc, err := character.CreateNPCFromConfig("elf_city_archive", 0, 0)
	if err != nil {
		t.Fatalf("build NPC: %v", err)
	}
	row := npc.SpellData["town_portal"]
	if row == nil {
		t.Fatal("the Archive no longer stocks town_portal")
	}

	airOnly := &character.MMCharacter{
		Name: "Sorcerer", Level: 1,
		MagicSchools: map[character.MagicSchoolID]*character.MagicSkill{
			character.MagicSchoolAir: {Mastery: character.MasteryNovice},
		},
	}
	if !canCharacterLearnNPCSpell(airOnly, "town_portal") {
		t.Fatal("the Archive refuses Town Portal to an Air caster")
	}

	// And the sale actually files it under Air, without opening Earth behind the
	// player's back.
	finishQuest(t, qm, npc.RequiresQuest) // the shop only opens once its errand is paid
	g.dialogActive, g.dialogNPC = true, npc
	g.party.Members = []*character.MMCharacter{airOnly}
	g.party.Gold = row.Cost
	g.selectedCharIdx = 0
	g.selectedSpellKey = "town_portal"
	NewInputHandler(g).purchaseSelectedSpell()

	if !airOnly.KnowsSpell(spells.SpellID("town_portal")) {
		t.Fatal("the purchase did not teach Town Portal")
	}
	if airOnly.MagicSchools[character.MagicSchoolEarth] != nil {
		t.Fatal("buying through Air silently opened the Earth school")
	}
	if g.party.Gold != 0 {
		t.Fatalf("gold left = %d, want the page to have been paid for", g.party.Gold)
	}

	// Buying it AGAIN is refused before the gold moves - and the refusal reads the
	// catalog key, so an authored display name cannot slip past it into a sale
	// that LearnSpell would then reject.
	npc.SpellData["town_portal"].Name = "Wayfarer's Gate" // a flavour rename
	g.party.Gold = row.Cost
	before := len(g.GetCombatMessages())
	NewInputHandler(g).purchaseSelectedSpell()
	if g.party.Gold != row.Cost {
		t.Fatalf("a spell the caster already knows was sold again (gold %d -> %d)", row.Cost, g.party.Gold)
	}
	// It must be turned away AS ALREADY KNOWN - the shop's own line. Missing that,
	// the row draws green (can learn), the sale runs, and LearnSpell refuses at
	// the very end with "cannot be taught right now": the same key-vs-name split,
	// one step later and in worse words.
	said := strings.Join(g.GetCombatMessages()[before:], " | ")
	if said == "" {
		t.Fatal("re-buying a known page said nothing at all")
	}
	if strings.Contains(said, "cannot be taught right now") {
		t.Fatalf("the known page went all the way to the teach call before failing: %q", said)
	}
}

// Yusra's lamps: the shipped props carry the take action wired to her gate quest.
func TestShrineLampPropsFeedYusrasGate(t *testing.T) {
	_, qm := bootQuestGiverTest(t)
	def := qm.Definitions()["shrine_lamps"]
	if def == nil {
		t.Fatal("shrine_lamps is not in the shipped catalog")
	}
	if def.Type != quests.QuestTypeInteract || def.TargetCount != 3 {
		t.Fatalf("shrine_lamps = %s x%d, want an interact quest for 3", def.Type, def.TargetCount)
	}
	for _, key := range []string{"shrine_lamp_1", "shrine_lamp_2", "shrine_lamp_3"} {
		npc, err := character.CreateNPCFromConfig(key, 0, 0)
		if err != nil {
			t.Fatalf("build %q: %v", key, err)
		}
		wired := false
		for _, c := range npc.DialogueData.Choices {
			if c.Action == questPropAction && c.QuestID == "shrine_lamps" {
				wired = true
			}
		}
		if !wired {
			t.Errorf("%s does not offer its lamp prop row for shrine_lamps", key)
		}
		// A lifted lamp is in the pack: it must leave the bracket, not stand
		// there looking takeable. hide_when_visited + the saved Visited flag is
		// what keeps it gone across a save.
		if !npc.HideWhenVisited {
			t.Errorf("%s stays on the wall after it is taken (hide_when_visited unset)", key)
		}
	}
}

// Taking a lamp removes it from the world through the ONE absence predicate the
// renderer, the click targeting and the interact focus all read.
func TestTakenShrineLampLeavesTheBracket(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.questManager = loadTestQuestManager(t)
	ih := NewInputHandler(g)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load NPCs: %v", err)
	}
	lamp, err := character.CreateNPCFromConfig("shrine_lamp_1", 0, 0)
	if err != nil {
		t.Fatalf("build lamp: %v", err)
	}
	if g.npcAbsent(lamp) {
		t.Fatal("an untouched lamp must be present")
	}
	if err := g.questManager.ActivateQuest("shrine_lamps"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	g.dialogNPC = lamp
	ih.handleQuestPropInteract("shrine_lamps", shippedQuestProps(t)["shrine_lamp"].Prop)
	if !lamp.Visited {
		t.Fatal("the lamp was not consumed")
	}
	if !g.npcAbsent(lamp) {
		t.Fatal("the lamp is still in the world after being taken")
	}
}

// standInTheDuelPit puts the party on a map that hosts champion duels - the
// authored `duel:` block creditArenaDuelWin now requires, so a bout only counts
// where it was fought.
func standInTheDuelPit(t *testing.T, cfg *config.Config) {
	t.Helper()
	previous := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = previous })
	wm := world.NewWorldManager(cfg)
	wm.CurrentMapKey = "arena"
	wm.MapConfigs = map[string]*config.MapConfig{
		"arena":  {Duel: &config.MapDuelConfig{}},
		"forest": {},
	}
	world.GlobalWorldManager = wm
}

// Nadira's gate counts BOUTS: a challenged champion's death advances it, and an
// ordinary kill - even of the same champion breed in the wild - does not.
func TestArenaDuelWinsAdvanceThePitQuest(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.questManager = loadTestQuestManager(t)
	standInTheDuelPit(t, g.config)
	const qid = "pit_standing"
	if err := g.questManager.ActivateQuest(qid); err != nil {
		t.Fatalf("activate: %v", err)
	}
	q := g.questManager.GetQuest(qid)

	wild := &monster.Monster3D{Name: "Weapon Master"}
	g.creditArenaDuelWin(wild)
	if q.CurrentCount != 0 {
		t.Fatalf("a wild champion kill counted as a bout (%d)", q.CurrentCount)
	}

	for i := 1; i <= 3; i++ {
		g.creditArenaDuelWin(&monster.Monster3D{Name: "Weapon Master", ChampionTier: "normal"})
		if want := i; q.CurrentCount != want {
			t.Fatalf("after %d bouts progress = %d, want %d", i, q.CurrentCount, want)
		}
	}
	if !q.Completed {
		t.Fatal("three bouts must finish pit_standing")
	}

	// Off the sand it does not count: the tier survives a save, so a champion the
	// party walked away from could otherwise post a bout when a DoT finishes it.
	if err := g.questManager.ActivateQuest("shrine_lamps"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	world.GlobalWorldManager.CurrentMapKey = "forest"
	before := g.questManager.GetQuest(qid).CurrentCount
	g.creditArenaDuelWin(&monster.Monster3D{Name: "Weapon Master", ChampionTier: "normal"})
	if got := g.questManager.GetQuest(qid).CurrentCount; got != before {
		t.Fatalf("a champion dying off the duel map counted as a bout (%d -> %d)", before, got)
	}
}

// The bout line reports what the MANAGER advanced. An errand that matches the
// duel tag but is bumped by nothing except its own encounter source must not be
// spoken for - re-deriving the match here would print progress for a counter
// that never moved.
func TestArenaDuelWinSpeaksOnlyForQuestsItActuallyAdvanced(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	standInTheDuelPit(t, g.config)
	g.questManager = quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{
		"pit_open": {
			Name: "Open Bouts", Type: quests.QuestTypeInteract,
			TargetMonster: quests.ArenaDuelTag, TargetCount: 3,
		},
		"pit_sealed": {
			Name: "Sealed Bouts", Type: quests.QuestTypeInteract,
			TargetMonster: quests.ArenaDuelTag, TargetCount: 3, EncounterOnly: true,
		},
	}})
	for _, id := range []string{"pit_open", "pit_sealed"} {
		if err := g.questManager.ActivateQuest(id); err != nil {
			t.Fatalf("activate %s: %v", id, err)
		}
	}

	before := len(g.GetCombatMessages())
	g.creditArenaDuelWin(&monster.Monster3D{Name: "Weapon Master", ChampionTier: "normal"})
	said := g.GetCombatMessages()[before:]

	if got := g.questManager.GetQuest("pit_sealed").CurrentCount; got != 0 {
		t.Fatalf("the encounter-only errand advanced to %d on an ordinary duel win", got)
	}
	if got := g.questManager.GetQuest("pit_open").CurrentCount; got != 1 {
		t.Fatalf("the open errand is at %d after one bout, want 1", got)
	}
	if len(said) != 1 {
		t.Fatalf("one bout wrote %d lines (%v), want the one quest that moved", len(said), said)
	}
	if strings.Contains(said[0], "0/3") {
		t.Fatalf("the bout line reports a counter that never moved: %q", said[0])
	}
}

// The bout is credited where a champion's DEATH is handled, not from the
// kill-quest hook: that path carries guards about kill credit (ignored progress,
// pure summons) which have nothing to do with duels.
func TestArenaDuelIsCreditedOnTheChampionDeathPath(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.questManager = loadTestQuestManager(t)
	standInTheDuelPit(t, g.config)
	const qid = "pit_standing"
	if err := g.questManager.ActivateQuest(qid); err != nil {
		t.Fatalf("activate: %v", err)
	}

	if _, err := config.LoadChampionConfig("../../assets/champions.yaml"); err != nil {
		t.Fatalf("load champions: %v", err) // the tier table gates the whole path
	}
	champ := &monster.Monster3D{Name: "Weapon Master", ChampionKey: "weapon_master", ChampionTier: "normal"}
	cs.recordChampionVictory(champ)
	if got := g.questManager.GetQuest(qid).CurrentCount; got != 1 {
		t.Fatalf("a champion death on the sand credited %d bouts, want 1", got)
	}
}

// A failed map load takes its NPCs with it (LoadAllMaps only warns), so the
// placement judgement stands down: one broken map must not turn into an
// unbootable game blaming the content.
func TestGatePlacementIsNotJudgedOnAPartialWorld(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })

	qc, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	qm := quests.NewQuestManager(qc)
	character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{
		"gated": {
			Type: character.NPCTypeSkillTrainer, RequiresQuest: "pit_standing",
			Dialogue: &character.NPCDialogue{QuestGreeting: "Win three bouts first."},
		},
		"giver": {
			Type: character.NPCTypeQuestGiver,
			Dialogue: &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{
				{Text: "Take it", Action: "give_quest", QuestID: "pit_standing"},
			}},
		},
	}}

	// The giver is NOT in the spawn set, exactly as if its map failed to load.
	installPlacedNPCs(t, cfg, qm, "gated")
	if err := g.validateQuestWorldReferences(qm); err == nil {
		t.Fatal("fixture: an intact world must still reject an unplaced giver")
	}
	world.GlobalWorldManager.FailedMaps = []string{"dunehold"}
	if err := g.validateQuestWorldReferences(qm); err != nil {
		t.Fatalf("a broken map turned into a boot failure about the gate: %v", err)
	}
}

// Same rule for the PROP COUNT check: the valves live on culverts.map, so a
// map that fails to parse leaves zero of them standing. Counting off that
// partial world would kill the boot with "quest asks for 7 valves, 0 placed" -
// a quest-data error for a map-file problem.
func TestPropCountIsNotJudgedOnAPartialWorld(t *testing.T) {
	cfg := loadTestConfig(t)
	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load npcs: %v", err)
	}
	qc, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	qm := quests.NewQuestManager(qc)

	// A world holding ONE prop-bearing NPC and none of the valves: the shape a
	// half-loaded run has.
	prev := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = prev })
	w := newTestWorld(cfg)
	lamp, err := character.CreateNPCFromConfig("shrine_lamp_1", 0, 0)
	if err != nil {
		t.Fatalf("build prop: %v", err)
	}
	w.NPCs = append(w.NPCs, lamp)
	wm := world.NewWorldManager(cfg)
	wm.LoadedMaps = map[string]*world.World3D{"forest": w}
	wm.CurrentMapKey = "forest"
	world.GlobalWorldManager = wm

	if err := ValidateInteractTagProducers(qm); err == nil {
		t.Fatal("fixture: an intact world missing its valves must be rejected")
	}
	wm.FailedMaps = []string{"culverts"}
	if err := ValidateInteractTagProducers(qm); err != nil {
		t.Fatalf("a broken map turned into a boot failure about the quest: %v", err)
	}
}

// A prop census has three distinct outcomes: unavailable, trustworthy zero,
// and a positive count of distinct physical NPCs. Dialogue routes are not
// objects, so repeating one NPC pointer or adding a second prop route must not
// manufacture another usable quest prop.
func TestInteractPropCensusCases(t *testing.T) {
	cfg := loadTestConfig(t)
	previousNPCs := character.NPCConfigInstance
	previousWorld := world.GlobalWorldManager
	t.Cleanup(func() {
		character.NPCConfigInstance = previousNPCs
		world.GlobalWorldManager = previousWorld
	})

	tests := []struct {
		name          string
		targetCount   int
		propRows      int
		physicalNPCs  int
		repeatPointer bool
		noManager     bool
		noLoadedWorld bool
		failedMap     bool
		wantError     string
	}{
		{name: "no manager is unavailable", targetCount: 1, propRows: 1, noManager: true},
		{name: "unloaded fixture is unavailable", targetCount: 1, propRows: 1, noLoadedWorld: true},
		{name: "partial world is unavailable", targetCount: 1, propRows: 1, failedMap: true},
		{name: "trustworthy zero is rejected", targetCount: 1, propRows: 1, wantError: "only 0"},
		{name: "one is below two", targetCount: 2, propRows: 1, physicalNPCs: 1, wantError: "only 1"},
		{name: "one repeated pointer still counts once", targetCount: 2, propRows: 1, physicalNPCs: 1, repeatPointer: true, wantError: "only 1"},
		{name: "two physical NPCs satisfy two", targetCount: 2, propRows: 1, physicalNPCs: 2},
		{name: "two routes on one NPC are ambiguous", targetCount: 1, propRows: 2, physicalNPCs: 1, wantError: "multiple one-shot prop actions"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			choices := make([]*character.NPCDialogueChoice, tc.propRows)
			for i := range choices {
				choices[i] = &character.NPCDialogueChoice{
					Text: "Use prop",
					Prop: &character.NPCPropCopy{Tag: "test_prop"},
				}
			}
			dialogue := &character.NPCDialogue{Choices: choices}
			character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{
				"test_prop": {Type: character.NPCTypeQuestGiver, Dialogue: dialogue},
			}}
			qm := quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{
				"test_errand": {
					Name: "Test errand", Type: quests.QuestTypeInteract,
					TargetMonster: "test_prop", TargetCount: tc.targetCount,
				},
			}})

			if tc.noManager {
				world.GlobalWorldManager = nil
			} else {
				wm := world.NewWorldManager(cfg)
				if !tc.noLoadedWorld {
					w := newTestWorld(cfg)
					for i := 0; i < tc.physicalNPCs; i++ {
						w.NPCs = append(w.NPCs, &character.NPC{
							Key: "test_prop", DialogueData: dialogue,
						})
					}
					if tc.repeatPointer && len(w.NPCs) > 0 {
						w.NPCs = append(w.NPCs, w.NPCs[0])
					}
					wm.LoadedMaps = map[string]*world.World3D{"test": w}
				}
				if tc.failedMap {
					wm.FailedMaps = []string{"missing"}
				}
				world.GlobalWorldManager = wm
			}

			err := ValidateInteractTagProducers(qm)
			if tc.wantError == "" {
				if err != nil {
					t.Fatalf("ValidateInteractTagProducers() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("ValidateInteractTagProducers() error = %v, want %q", err, tc.wantError)
			}
		})
	}
}

// The pit quest must never be a kill quest: the arena stands empty between
// bouts, so the map-cleared census would finish it for free.
func TestPitQuestIsNotACensusKillQuest(t *testing.T) {
	_, qm := bootQuestGiverTest(t)
	def := qm.Definitions()["pit_standing"]
	if def == nil {
		t.Fatal("pit_standing is not in the shipped catalog")
	}
	if def.Type != quests.QuestTypeInteract {
		t.Fatalf("pit_standing type = %s, want %s (a kill quest on an empty arena self-completes)",
			def.Type, quests.QuestTypeInteract)
	}
	if def.TargetMonster != quests.ArenaDuelTag {
		t.Fatalf("pit_standing tag = %q, want %q", def.TargetMonster, quests.ArenaDuelTag)
	}
}

// A concluded giver keeps no dialogue rows, so its conversation tab is gone -
// and with it the only surface that ever showed visited_message. Both shipped
// gated traders author one; the shop header is what has to deliver it, or the
// line is content the player can never reach.
func TestConcludedShopHeaderShowsTheAuthoredVisitedLine(t *testing.T) {
	g, qm := bootQuestGiverTest(t)
	for _, key := range []string{"elf_city_archive", "nomad_city_spells"} {
		npc, err := character.CreateNPCFromConfig(key, 0, 0)
		if err != nil {
			t.Fatalf("build %s: %v", key, err)
		}
		visited := npc.DialogueData.VisitedMessage
		if visited == "" {
			t.Fatalf("%s authors no visited_message any more", key)
		}
		// Before the errand is settled the header is the shop greeting.
		if got := g.npcShopHeaderLine(npc, "stock"); got != npc.DialogueData.Greeting {
			t.Errorf("%s header before the quest = %q, want the greeting", key, got)
		}
		finishQuest(t, qm, npc.RequiresQuest)
		npc.Visited = true // what handleTurnInQuest does once the chain is spent
		if g.npcDialogueState(npc) != npcStateConcluded {
			t.Fatalf("%s is not concluded after its chain was paid", key)
		}
		if g.npcDialogHasTalkTab(npc) {
			t.Fatalf("%s still has a conversation tab; this test is about the case where it is gone", key)
		}
		if got := g.npcShopHeaderLine(npc, "stock"); got != visited {
			t.Errorf("%s header after the quest = %q, want the authored %q", key, got, visited)
		}
	}
}

// A fixed-layout dialog (mastery grid, card grid, shop grid) draws no authored
// rows. The shipped catalog uses that on purpose - Nadira's errand IS her whole
// conversation while her gate is shut - so the rule is not "no rows", it is "no
// row that outlives the gate". A second chained errand is the mistake to catch:
// it opens once the gate is paid, which is exactly when the surface disappears.
func TestDialogueRowsOnAFixedLayoutDialogMustLiveBehindItsGate(t *testing.T) {
	g, _ := bootQuestGiverTest(t)
	if err := g.validateDialogueRowsAreDrawable(); err != nil {
		t.Fatalf("shipped catalog: %v", err)
	}
	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })

	trainer := func(gate string, rows ...*character.NPCDialogueChoice) *character.NPCConfig {
		return &character.NPCConfig{NPCs: map[string]*character.NPCData{
			"drillmaster": {
				Type: character.NPCTypeSkillTrainer, RequiresQuest: gate,
				Dialogue: &character.NPCDialogue{QuestGreeting: "Stand on the sand first.",
					Choices: rows},
			},
		}}
	}
	// The shipped shape: the gate's own errand, offered while the grid is shut.
	character.NPCConfigInstance = trainer("pit_standing",
		&character.NPCDialogueChoice{Text: "Why the arena?", Action: "info", Response: "It hits back.",
			Choices: []*character.NPCDialogueChoice{
				{Text: "We will win three", Action: "give_quest", QuestID: "pit_standing"},
				{Text: "Back", Action: "back"},
			}},
	)
	if err := g.validateDialogueRowsAreDrawable(); err != nil {
		t.Fatalf("the gate's own errand was rejected: %v", err)
	}
	// A SECOND errand behind the first: reachable only after the gate is paid, so
	// the mastery grid has replaced the conversation and nothing draws it.
	character.NPCConfigInstance = trainer("pit_standing",
		&character.NPCDialogueChoice{Text: "We will win three", Action: "give_quest", QuestID: "pit_standing"},
		&character.NPCDialogueChoice{Text: "And now?", Action: "give_quest", QuestID: "culverts_valves",
			RequiresQuest: "pit_standing"},
	)
	err := g.validateDialogueRowsAreDrawable()
	if err == nil || !strings.Contains(err.Error(), "culverts_valves") {
		t.Fatalf("a follow-up errand on a grid dialog was accepted: %v", err)
	}
	// And a grid dialog with NO gate at all cannot host a row of any kind - a
	// quest row, a chat row or an encounter row are equally undrawable there.
	for _, row := range []*character.NPCDialogueChoice{
		{Text: "Take it", Action: "give_quest", QuestID: "pit_standing"},
		{Text: "Tell me about the sand", Action: "info", Response: "It is hot."},
		{Text: "Draw steel", Action: "combat"},
		{Text: "Through here", Action: "enter_map", Map: "arena"},
	} {
		character.NPCConfigInstance = trainer("", row)
		if err := g.validateDialogueRowsAreDrawable(); err == nil {
			t.Errorf("an ungated grid dialog accepted a %q row that nothing draws", row.Action)
		}
	}
	// "Leave" is the exception: the conventional close row, authored on the tavern
	// and harmless where ESC does the same thing.
	character.NPCConfigInstance = trainer("", &character.NPCDialogueChoice{Text: "Leave", Action: "leave"})
	if err := g.validateDialogueRowsAreDrawable(); err != nil {
		t.Fatalf("the conventional close row was rejected: %v", err)
	}
	// Behind a gate, ordinary conversation is fine: it IS the pre-gate dialog.
	character.NPCConfigInstance = trainer("pit_standing",
		&character.NPCDialogueChoice{Text: "Why the arena?", Action: "info", Response: "It hits back."},
	)
	if err := g.validateDialogueRowsAreDrawable(); err != nil {
		t.Fatalf("a gated grid dialog rejected its own conversation: %v", err)
	}
}

// An action name is the key every row rule reads - the service gate, the
// conclusion rule, the prop route. A typo currently draws a row that does
// nothing when pressed and reports nothing anywhere, which is how close_valve /
// take_swords lived as labels no code dispatched.
func TestDialogueActionsMustBeDispatched(t *testing.T) {
	g, _ := bootQuestGiverTest(t)
	if err := g.validateDialogueActionsAreDispatched(); err != nil {
		t.Fatalf("shipped catalog: %v", err)
	}
	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })
	character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{
		"villager": {Type: character.NPCTypeEncounter, Dialogue: &character.NPCDialogue{
			Choices: []*character.NPCDialogueChoice{
				{Text: "Rest here", Action: "tavern_resr"}, // one letter off
			}}},
	}}
	err := g.validateDialogueActionsAreDispatched()
	if err == nil || !strings.Contains(err.Error(), "tavern_resr") {
		t.Fatalf("a typo'd action was accepted: %v", err)
	}
	// Every action the gate withholds must be dispatched too - a paid row the
	// switch does not know takes the money nowhere.
	character.NPCConfigInstance = &character.NPCConfig{NPCs: map[string]*character.NPCData{}}
	gatedServiceActions["polish_boots"] = true
	t.Cleanup(func() { delete(gatedServiceActions, "polish_boots") })
	if err := g.validateDialogueActionsAreDispatched(); err == nil ||
		!strings.Contains(err.Error(), "polish_boots") {
		t.Fatalf("an undispatched service action was accepted: %v", err)
	}
}

// A CODE tag is not credited by assertion. The arena bout advances pit_standing
// only while some map authors a `duel:` block, so deleting or renaming that block
// locks the errand - and Nadira's mastery training behind it - with the validator
// still reporting the tag as produced.
func TestArenaTagNeedsADuelArenaInTheWorld(t *testing.T) {
	_, _ = bootQuestGiverTest(t)
	qm := quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{
		"pit_standing": {
			Name: "Pit Standing", Type: quests.QuestTypeInteract,
			TargetMonster: quests.ArenaDuelTag, TargetCount: 3,
		},
	}})
	cfg := loadTestConfig(t)
	prev := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = prev })

	duelStarter := func(name string) *character.NPC {
		return &character.NPC{Key: name, Name: name, DialogueData: &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{
			{Text: "Fight", Action: "start_arena_duel", Tier: "normal"},
		}}}
	}

	// A world that authors a duel arena and places its starter on that map.
	wm := world.NewWorldManager(cfg)
	wm.MapConfigs = map[string]*config.MapConfig{
		"arena":  {Duel: &config.MapDuelConfig{}},
		"forest": {},
	}
	wm.LoadedMaps = map[string]*world.World3D{
		"arena":  newTestWorld(cfg),
		"forest": newTestWorld(cfg),
	}
	wm.LoadedMaps["arena"].NPCs = append(wm.LoadedMaps["arena"].NPCs, duelStarter("pit_master"))
	world.GlobalWorldManager = wm
	if err := ValidateInteractTagProducers(qm); err != nil {
		t.Fatalf("a world with a duel arena was rejected: %v", err)
	}

	// The same placed starter with the duel block gone: its button cannot run.
	wm.MapConfigs["arena"] = &config.MapConfig{}
	err := ValidateInteractTagProducers(qm)
	if err == nil || !strings.Contains(err.Error(), quests.ArenaDuelTag) {
		t.Fatalf("a world with no duel arena was accepted: %v", err)
	}

	// The inverse is also dead: a duel block is not a producer without a starter.
	wm.MapConfigs["arena"] = &config.MapConfig{Duel: &config.MapDuelConfig{}}
	wm.LoadedMaps["arena"].NPCs = nil
	err = ValidateInteractTagProducers(qm)
	if err == nil || !strings.Contains(err.Error(), quests.ArenaDuelTag) {
		t.Fatalf("a duel block with no placed starter was accepted: %v", err)
	}

	// A starter on another map cannot activate this arena.
	wm.LoadedMaps["forest"].NPCs = append(wm.LoadedMaps["forest"].NPCs, duelStarter("forest_master"))
	err = ValidateInteractTagProducers(qm)
	if err == nil || !strings.Contains(err.Error(), quests.ArenaDuelTag) {
		t.Fatalf("a starter from another map activated the arena: %v", err)
	}

	// With configs but no loaded worlds the placement census is unavailable and
	// validation stands down, like the other boot checks used by small fixtures.
	wm.LoadedMaps = nil
	if err := ValidateInteractTagProducers(qm); err != nil {
		t.Fatalf("catalog-only world was treated as a trustworthy empty placement census: %v", err)
	}

	// And the shipped catalogs still hold up.
	if _, err := os.Stat("../../assets/map_configs.yaml"); err == nil {
		shipped := world.NewWorldManager(cfg)
		if err := shipped.LoadMapConfigs("../../assets/map_configs.yaml"); err != nil {
			t.Fatalf("load shipped map configs: %v", err)
		}
		world.GlobalWorldManager = shipped
		if err := ValidateInteractTagProducers(qm); err != nil {
			t.Fatalf("shipped map configs: %v", err)
		}
	}
}
