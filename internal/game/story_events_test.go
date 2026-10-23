package game

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/strictyaml"
	"ugataima/internal/world"
)

// storyGame boots the open world with the shipped story catalog loaded; the
// catalog is global, so it is dropped again when the test ends.
func storyGame(t *testing.T) (*MMGame, *world.WorldManager) {
	t.Helper()
	t.Chdir("../..")
	g, wm, _ := bootOpenWorldGame(t, true)
	if err := LoadStoryConfig("assets/story_events.yaml", g.questManager, wm); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { globalStoryEvents = nil })
	return g, wm
}

// journalLists: the journal shows this quest.
func journalLists(g *MMGame, id string) bool {
	return slices.ContainsFunc(g.journalQuests(), func(q *quests.Quest) bool { return q.ID == id })
}

func storyNPC(t *testing.T, g *MMGame, key string) *character.NPC {
	t.Helper()
	for _, npc := range g.world.NPCs {
		if npc != nil && npc.Key == key {
			return npc
		}
	}
	t.Fatalf("no %s in the world", key)
	return nil
}

// The main line's first steps through the real hooks: a new game queues the
// prologue and nothing else - the quests it starts with reach no beat - it
// opens on a calm frame and is recorded with
// its day, its pages turn and close; the hidden cliff quest stays out of the
// journal until the Tower's beat reveals it, and speaking to the Tower
// delivers the envoy's letter - even for a party it will not admit.
func TestStoryMainLineFirstSteps(t *testing.T) {
	g, _ := storyGame(t)
	g.startNewGameWithParty(character.NewParty(g.config))
	g.syncStoryQuestBeats() // the first frame reads the journal the run starts with
	if len(g.storyQueue) != 1 || g.storyQueue[0] != "prologue" {
		t.Fatalf("new game queued %v, want [prologue]", g.storyQueue)
	}
	g.dialogActive = true
	g.tickStoryEvents()
	if g.storyScreen != nil {
		t.Fatal("a story event opened over a dialog")
	}
	g.dialogActive = false
	g.tickStoryEvents()
	if day, seen := g.storySeenDay("prologue"); g.storyScreen == nil || g.storyScreen.event.ID != "prologue" || !seen || day != g.currentCalendarDay() {
		t.Fatalf("prologue not opened and dated: screen %v seen %v", g.storyScreen, g.storySeen)
	}
	if topModalLayerFor(g, false) != modalLayerStory || !modalLayerStory.pausesWorld() {
		t.Fatal("the story screen is not the paused top layer")
	}
	for i := 0; i < 2*len(storyEvent("prologue").Pages); i++ {
		g.advanceStory() // first press shows the page, the next turns it
	}
	if g.storyScreen != nil {
		t.Fatal("the prologue did not close after its last page")
	}

	if g.questManager.GetQuest("broodmother_nests") == nil || journalLists(g, "broodmother_nests") {
		t.Fatal("the hidden cliff quest must be active but unlisted")
	}
	if err := g.activateQuest("main_tower_letter"); err != nil {
		t.Fatal(err)
	}
	tower := storyNPC(t, g, "mage_tower")
	ih := &InputHandler{game: g}
	g.party.Members[0].Promotion = character.PromotionLich // the wards refuse a lich party
	ih.openNPCInteraction(tower)
	g.party.Members[0].Promotion = character.PromotionNone
	if q := g.questManager.GetQuest("main_tower_letter"); q == nil || !q.Completed {
		t.Fatal("speaking at the Tower's gate did not deliver the letter")
	}
	g.syncStoryQuestBeats() // the frame's read of the journal
	if len(g.storyQueue) != 1 || g.storyQueue[0] != "tower" {
		t.Fatalf("delivery queued %v, want [tower]", g.storyQueue)
	}

	// While the beat waits, no talk opens a dialog: mid-fight the log says why,
	// and a crate still answers for itself; when calm the beat simply comes first.
	said := func(from int, text string) bool {
		for _, e := range g.combatLogHistory[from:] {
			if strings.Contains(e.Text, text) {
				return true
			}
		}
		return false
	}
	ts := float64(g.config.GetTileSize())
	spawnMonsterAtTile(g, "goblin", TileIndex(g.camera.X, ts)+2, TileIndex(g.camera.Y, ts), ts)
	if !g.partyInCombat() {
		t.Fatal("fixture: no fight")
	}
	n := len(g.combatLogHistory)
	ih.openNPCInteraction(tower)
	if g.dialogActive || !said(n, uitext.Text("combat.enemies_are_upon_you_finish_the_fight")) {
		t.Fatalf("mid-fight talk: dialog %v; want none, and the log saying why", g.dialogActive)
	}
	crate := slices.IndexFunc(g.world.NPCs, func(npc *character.NPC) bool { return npc != nil && npc.Type == character.NPCTypeLootCrate })
	if crate < 0 {
		t.Fatal("fixture: no loot crate in the world")
	}
	n = len(g.combatLogHistory)
	ih.openNPCInteraction(g.world.NPCs[crate])
	if !said(n, uitext.Text("combat.no_time_to_search_the_mid", g.world.NPCs[crate].Name)) {
		t.Fatal("a crate mid-fight did not answer for itself")
	}
	g.world.Monsters = nil
	n = len(g.combatLogHistory)
	ih.openNPCInteraction(tower)
	if g.dialogActive || len(g.combatLogHistory) != n {
		t.Fatal("calm talk with a beat waiting: want no dialog and nothing logged - the beat opens next")
	}
	g.tickStoryEvents()
	if g.storyScreen == nil || !journalLists(g, "broodmother_nests") || journalLists(g, "water_purge") {
		t.Fatal("the Tower's beat must open and reveal only the cliff quest")
	}
	g.closeStoryScreen()
	ih.openNPCInteraction(tower)
	if !g.dialogActive {
		t.Fatal("once the beat has played, speaking at the gate opens the Tower's dialog")
	}
	g.closeConversation()

	// A hunt credited by the turn-in talk itself: its beats come before
	// Sylwen's dialog too.
	if err := g.questManager.ActivateQuest("dragon_slayer"); err != nil {
		t.Fatal(err)
	}
	g.syncStoryQuestBeats()
	for g.tickStoryEvents(); g.storyScreen != nil; g.tickStoryEvents() {
		g.closeStoryScreen() // the oath's own beat
	}
	for _, npc := range g.allLoadedNPCs() {
		if len(npc.Summons) > 0 && npc.Summons[0].QuestID == "dragon_slayer" {
			npc.Visited = true // every seal spent, no dragon alive
		}
	}
	ih.openNPCInteraction(storyNPC(t, g, "desert_pilgrim_sylwen"))
	if q := g.questManager.GetQuest("dragon_slayer"); !q.Completed || g.dialogActive || !slices.Contains(g.storyQueue, "dais") {
		t.Fatalf("hunt completed %v, dialog %v, queue %v; want completed, no dialog, the dais beat waiting", q.Completed, g.dialogActive, g.storyQueue)
	}
}

// Seen beats, in the order shown, and still-queued beats survive a save; a
// save from before story events replays nothing its journal has already
// passed, while kill and arrival beats stay pending for their next moment, and
// keeps every quest its journal showed - through its next save as well.
func TestStoryStateSavesAndResyncsLegacySaves(t *testing.T) {
	g, wm := storyGame(t)
	g.startNewGameWithParty(character.NewParty(g.config))
	g.tickStoryEvents() // the prologue opens and is seen
	g.closeStoryScreen()
	// One day, Seabright before the letter - against catalog order.
	g.fireStoryTrigger(storyOnRegionEntered, "city", false)
	g.tickStoryEvents()
	g.closeStoryScreen()
	if err := g.activateQuest("main_tower_letter"); err != nil {
		t.Fatal(err)
	}
	g.questManager.MarkCompleted("main_tower_letter")
	g.syncStoryQuestBeats()
	g.tickStoryEvents()
	g.closeStoryScreen()
	g.fireStoryTrigger(storyOnMonsterKilled, "golden_thief_bug", true)
	seenIDs := func(seen []StorySeenSave) []string {
		var ids []string
		for _, s := range seen {
			ids = append(ids, s.ID)
		}
		return ids
	}
	shown := []string{"prologue", "lamps", "tower"}
	save := g.buildSave(wm)
	if save.StoryVersion != storyEventsVersion || !slices.Equal(seenIDs(save.StorySeen), shown) ||
		len(save.StoryQueue) != 1 || save.StoryQueue[0] != "captain_first" {
		t.Fatalf("saved story %d %v %v", save.StoryVersion, save.StorySeen, save.StoryQueue)
	}
	g.resetStory()
	if err := g.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(seenIDs(g.storySeen), shown) || len(g.storyQueue) != 1 || g.storyQueue[0] != "captain_first" {
		t.Fatalf("restored seen %v queue %v", g.storySeen, g.storyQueue)
	}
	if journalLists(g, "endgame_triad") {
		t.Fatal("a current save revealed a hidden quest no beat has shown")
	}

	if err := g.activateQuest("dragon_slayer"); err != nil && !strings.Contains(err.Error(), "level") {
		t.Fatal(err)
	}
	if g.questManager.GetQuest("dragon_slayer") == nil {
		if err := g.questManager.ActivateQuest("dragon_slayer"); err != nil {
			t.Fatal(err)
		}
	}
	legacy := g.buildSave(wm)
	legacy.StoryVersion, legacy.StorySeen, legacy.StoryQueue, legacy.StoryRevealed = 0, nil, nil, nil
	if err := g.applySave(wm, &legacy); err != nil {
		t.Fatal(err)
	}
	for _, reload := range []bool{false, true} {
		if reload {
			next := g.buildSave(wm)
			if err := g.applySave(wm, &next); err != nil {
				t.Fatal(err)
			}
		}
		for id, def := range g.questManager.Definitions() {
			if def.Hidden && g.questManager.GetQuest(id) != nil && !journalLists(g, id) {
				t.Errorf("legacy save (reloaded %v): %s left the journal", reload, id)
			}
		}
	}
	for id, want := range map[string]bool{"prologue": true, "four_seals": true, "dais": false, "captain_first": false, "lamps": false} {
		if g.storyWasSeen(id) != want {
			t.Errorf("legacy resync: %s seen = %v, want %v", id, !want, want)
		}
	}
	if len(g.storyQueue) != 0 {
		t.Errorf("legacy resync queued %v", g.storyQueue)
	}
	// An order the old journal has carried out was lived through: the dive
	// before Seabright, with its Enforcer beat; the cliffs, not done, still wait.
	g.questManager.MarkCompleted("water_purge")
	done := g.buildSave(wm)
	done.StoryVersion, done.StorySeen, done.StoryQueue, done.StoryRevealed = 0, nil, nil, nil
	if err := g.applySave(wm, &done); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]bool{"lamps": true, "enforcer": true, "crater": false, "captain_first": false} {
		if g.storyWasSeen(id) != want {
			t.Errorf("legacy save with the depths cleared: %s seen = %v, want %v", id, !want, want)
		}
	}

	// Tools and fixtures run without a quest manager: restoring, reading the
	// journal and opening a revealing beat stay safe.
	g.questManager = nil
	g.restoreStory(&GameSave{StoryVersion: storyEventsVersion, StoryQueue: []string{"tower"},
		StoryRevealed: []string{"broodmother_nests"}})
	g.syncStoryQuestBeats()
	g.tickStoryEvents()
	if g.storyScreen == nil || g.storyScreen.event.ID != "tower" {
		t.Fatal("without a quest manager the queued beat did not open")
	}
}

// A hidden quest finished before its beat is announced once, when the beat
// reveals it; a save taken before the next frame does not lose the beat; a
// beat reached in the open journal waits for it to close; over the victory
// screen the queue plays at once, though a fight lingers, and in order - the
// ending last.
func TestStoryBeatsAnnounceAndOpenOnTime(t *testing.T) {
	g, wm := storyGame(t)
	g.startNewGameWithParty(character.NewParty(g.config))
	g.tickStoryEvents()
	g.closeStoryScreen()

	q := g.questManager.GetQuest("broodmother_nests")
	told := func() int {
		n := 0
		for _, e := range g.combatLogHistory {
			if strings.Contains(e.Text, q.Definition.Name) {
				n++
			}
		}
		return n
	}
	g.questManager.MarkCompleted(q.ID)
	g.announceQuestCompletion(q)
	if told() != 0 {
		t.Fatal("a hidden quest was announced before its reveal")
	}
	save := g.buildSave(wm) // before the frame that would queue the beat
	if err := g.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
	g.menuOpen = true
	g.updateInterfacePresentation()
	if !slices.Contains(g.storyQueue, "crater") || g.storyScreen != nil {
		t.Fatalf("after the load, in the open journal: queue %v, screen open %v; want crater queued, nothing open", g.storyQueue, g.storyScreen != nil)
	}
	g.menuOpen = false
	g.tickStoryEvents()
	if g.storyScreen == nil || g.storyScreen.event.ID != "crater" || told() != 1 {
		t.Fatalf("crater open %v, told %d times, want open and once", g.storyScreen != nil, told())
	}
	for g.storyScreen != nil {
		g.closeStoryScreen()
		g.tickStoryEvents()
	}
	if told() != 1 {
		t.Fatalf("told %d times", told())
	}

	g.fireStoryTrigger(storyOnMonsterKilled, "golden_thief_bug", true)
	g.fireStoryTrigger(storyOnVictory, "", false)
	ts := float64(g.config.GetTileSize())
	spawnMonsterAtTile(g, "goblin", TileIndex(g.camera.X, ts)+2, TileIndex(g.camera.Y, ts), ts)
	if !g.partyInCombat() {
		t.Fatal("fixture: no fight")
	}
	g.gameVictory = true
	g.tickStoryEvents()
	if g.storyScreen == nil || g.storyScreen.event.ID != "captain_first" || !slices.Equal(g.storyQueue, []string{"ending"}) {
		t.Fatalf("over the victory screen: open %v, queue %v; want the queue in order, captain_first first", g.storyScreen, g.storyQueue)
	}
	g.closeStoryScreen()
	g.tickStoryEvents()
	if g.storyScreen == nil || g.storyScreen.event.ID != "ending" || len(g.storyQueue) != 0 {
		t.Fatalf("over the victory screen: open %v, queue %v; want the ending last", g.storyScreen, g.storyQueue)
	}
}

// A load's own reconciliation can finish a quest whose targets died in the
// save's timeline: whether that is told now or at its reveal follows the
// loaded save's reveals, never the session's before it; a save from before
// story events, whose journal showed every quest, tells it now.
func TestStoryLoadReconciliationReadsTheSavesReveals(t *testing.T) {
	g, wm := storyGame(t)
	g.startNewGameWithParty(character.NewParty(g.config))
	q := g.questManager.GetQuest("broodmother_nests")
	// The cliff dragons die off the books, those still to rise included.
	target := func(key string) bool {
		md, err := monster.MonsterConfig.GetMonsterByKey(key)
		return err == nil && q.Definition.MatchesTarget(quests.NormalizeTarget(md.Name))
	}
	wm.EachWorld(func(_ string, w *world.World3D) {
		w.Monsters = slices.DeleteFunc(w.Monsters, func(m *monster.Monster3D) bool { return target(m.Key) })
	})
	for _, npc := range g.allLoadedNPCs() {
		for _, sm := range npc.Summons {
			npc.Visited = npc.Visited || sm != nil && target(sm.Monster)
		}
		if enc := npc.EncounterData; enc != nil {
			for _, sp := range enc.Monsters {
				npc.Visited = npc.Visited || sp != nil && target(sp.Type)
			}
		}
	}
	for id, def := range g.questManager.Definitions() {
		for _, sp := range def.OnAcceptSpawns {
			if target(sp.Monster) {
				g.questSpawnsDone[id+"#accept:"+sp.ID] = true
			}
		}
		for _, sp := range def.OnCompleteSpawns {
			if target(sp.Monster) {
				g.questSpawnsDone[id+"#"+sp.ID] = true
			}
		}
	}
	if q.Completed {
		t.Fatal("fixture: the hunt finished before the save")
	}
	save := g.buildSave(wm)
	told := func() int {
		n := 0
		for _, e := range g.combatLogHistory {
			if strings.Contains(e.Text, q.Definition.Name) {
				n++
			}
		}
		return n
	}
	for _, row := range []struct {
		name   string
		legacy bool
		want   int
	}{
		{"current save, revealed only before the load", false, 0},
		{"save from before story events", true, 1},
	} {
		s := save
		if row.legacy {
			s.StoryVersion, s.StorySeen, s.StoryQueue, s.StoryRevealed = 0, nil, nil, nil
		}
		g.revealQuest(q.ID) // the session before the load
		before := told()
		if err := g.applySave(wm, &s); err != nil {
			t.Fatal(err)
		}
		if !g.questManager.GetQuest(q.ID).Completed {
			t.Fatalf("%s: fixture: the load's reconciliation did not finish the hunt", row.name)
		}
		if got := told() - before; got != row.want {
			t.Errorf("%s: completion told %d times at the load, want %d", row.name, got, row.want)
		}
	}
}

// The three main quests finish in any order: whichever completes first (its
// boss rises) brings the Last Reckoning, and the others never queue it again.
func TestStoryReckoningComesWithTheFirstRisenBoss(t *testing.T) {
	g, _ := storyGame(t)
	main := []string{"dragon_slayer", "broodmother_nests", "water_purge"}
	for _, first := range main {
		g.resetStory()
		g.fireStoryTrigger(storyOnQuestCompleted, first, false)
		if !slices.Contains(g.storyQueue, "reckoning") {
			t.Fatalf("%s first: queue %v, want the reckoning", first, g.storyQueue)
		}
		for _, other := range main {
			g.fireStoryTrigger(storyOnQuestCompleted, other, false)
		}
		n := 0
		for _, id := range g.storyQueue {
			if id == "reckoning" {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("%s first: reckoning queued %d times", first, n)
		}
	}
}

// Every page of every shipped beat fits its text box at every interface frame
// and font, and the plate and the entry panel stay inside the viewport
// without overlapping.
func TestStoryScreenFitsEveryFrame(t *testing.T) {
	data, err := os.ReadFile("../../assets/story_events.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cfg storyEventConfig
	if err := strictyaml.Decode(data, &cfg); err != nil {
		t.Fatal(err)
	}
	forEachUIFont(t, func(t *testing.T) {
		for _, size := range withInterfaceFrames(t, [][2]int{{1024, 768}, {1280, 720}, {1920, 1080}}) {
			view := layoutRect{0, 0, size[0], gameplayViewportBottomWithPartyHUD(size[1])}
			l := computeStoryLayout(view)
			inside := func(r layoutRect) bool {
				return r.x >= view.x && r.y >= view.y && r.right() <= view.right() && r.bottom() <= view.bottom()
			}
			overlap := l.plate.x < l.panel.right() && l.panel.x < l.plate.right() && l.plate.y < l.panel.bottom() && l.panel.y < l.plate.bottom()
			if !inside(l.plate) || !inside(l.panel) || overlap || l.plate.h < 200 {
				t.Fatalf("%v: plate %+v panel %+v in %+v", size, l.plate, l.panel, view)
			}
			for _, e := range cfg.Events {
				if !storyTitleFitsLarge(e.Title, l.title) && uiTextWidth(e.Title) > l.title.w-storyLightGap {
					t.Errorf("%v: title %q fits no size in a %d box", size, e.Title, l.title.w-storyLightGap)
				}
				for i, page := range e.Pages {
					if lines, ok := storyBodyLines(page, l.body); !ok {
						t.Errorf("%v: %s page %d needs %d lines, box %d tall", size, e.ID, i+1, len(lines), l.body.h)
					}
				}
			}
		}
	})
}

// The loader refuses a beat that could never fire or never show.
func TestStoryEventConfigRejectsBadEvents(t *testing.T) {
	g, wm := storyGame(t)
	// Every test catalog carries one beat revealing the hidden quests, which the
	// loader demands; the rows break one thing each.
	var hidden []string
	for id, def := range g.questManager.Definitions() {
		if def.Hidden {
			hidden = append(hidden, id)
		}
	}
	slices.Sort(hidden)
	reveal := "  - id: r\n    trigger: {on: victory}\n    art: story_prologue\n    title: \"R\"\n    pages: [\"P\"]\n    reveals_quests: [" + strings.Join(hidden, ", ") + "]\n"
	load := func(catalog string) error {
		path := filepath.Join(t.TempDir(), "story.yaml")
		if err := os.WriteFile(path, []byte("events:\n"+catalog), 0o644); err != nil {
			t.Fatal(err)
		}
		return LoadStoryConfig(path, g.questManager, wm)
	}
	good := "  - id: x\n    trigger: {on: new_game}\n    art: story_prologue\n    title: \"T\"\n    pages: [\"P\"]\n"
	if err := load(good + reveal); err != nil {
		t.Fatalf("control: a good catalog was refused: %v", err)
	}
	if err := load(good); err == nil {
		t.Error("a catalog leaving hidden quests unrevealed loaded")
	}
	for name, entry := range map[string]string{
		"unknown trigger": strings.Replace(good, "new_game", "sunrise", 1),
		"unknown quest":   strings.Replace(good, "{on: new_game}", "{on: quest_completed, quests: [no_such_quest]}", 1),
		"no pages":        strings.Replace(good, "[\"P\"]", "[]", 1),
		"four pages":      strings.Replace(good, "[\"P\"]", "[\"P\", \"P\", \"P\", \"P\"]", 1),
		"reveals shown":   good + "    reveals_quests: [goblin_hunt]\n",
		"unknown map":     strings.Replace(good, "{on: new_game}", "{on: region_entered, maps: [city, nowhere]}", 1),
		"unknown speaker": good + "    speaker: wizard\n",
		"new game map":    strings.Replace(good, "{on: new_game}", "{on: new_game, maps: [city]}", 1),
		"victory quests":  strings.Replace(good, "{on: new_game}", "{on: victory, quests: [water_purge]}", 1),
		"kill with map":   strings.Replace(good, "{on: new_game}", "{on: monster_killed, boss: true, maps: [city]}", 1),
		"quest monsters":  strings.Replace(good, "{on: new_game}", "{on: quest_completed, quests: [water_purge], monsters: [alien]}", 1),
		"repeated id":     good + strings.Replace(good, "  - ", "  - ", 1),
		"starting accept": strings.Replace(good, "{on: new_game}", "{on: quest_accepted, quests: [water_purge]}", 1),
	} {
		if err := load(entry + reveal); err == nil {
			t.Errorf("%s: loaded", name)
		}
	}
	// The boot check passes the shipped plates and refuses a missing plate or
	// portrait.
	if err := LoadStoryConfig("assets/story_events.yaml", g.questManager, wm); err != nil {
		t.Fatal(err)
	}
	panics := func() (p bool) {
		defer func() { p = recover() != nil }()
		g.validateStoryArt()
		return false
	}
	if panics() {
		t.Fatal("the shipped plates failed the art check")
	}
	art := globalStoryEvents[0].Art
	globalStoryEvents[0].Art = "no_such_plate"
	if !panics() {
		t.Error("a missing plate passed the art check")
	}
	globalStoryEvents[0].Art = art
	globalWatcher = &watcherConfig{Portrait: "no_such_portrait"}
	t.Cleanup(func() { globalWatcher = nil })
	if !panics() {
		t.Error("a missing portrait passed the art check")
	}
}
