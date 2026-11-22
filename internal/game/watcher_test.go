package game

import (
	"slices"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

func watcherGame(t *testing.T) *MMGame {
	t.Helper()
	g, _ := storyGame(t)
	if err := LoadWatcherConfig("assets/watcher.yaml"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { globalWatcher = nil })
	g.startNewGameWithParty(character.NewParty(g.config))
	g.storyQueue = nil // the prologue is not what these tests watch
	w := &g.config.Watcher
	w.CampChance, w.KillChance, w.MaxRandomLines, w.MinDaysBetween = 1, 0, 99, 2
	return g
}

// shownAtCamp picks the remark a rest would carry and spends it as the strip
// does once his words appear.
func shownAtCamp(g *MMGame) *WatcherLine {
	l := g.watcherCampLine(false)
	if l != nil {
		g.markWatcherSpoken(l)
	}
	return l
}

func killFor(t *testing.T, g *MMGame, key string) *monster.Monster3D {
	t.Helper()
	m := monster.NewMonster3DFromConfig(0, 0, key, g.config)
	if m == nil {
		t.Fatalf("no monster %s", key)
	}
	return m
}

// He says nothing before his first plate; after it, a story boss's line waits
// out an open dialog, plays once as a card and in the log, and never again;
// the card's clock stands still while it is covered - a load, a dialog.
func TestWatcherKillLinePlaysOnceAfterHisIntroduction(t *testing.T) {
	g := watcherGame(t)
	enforcer := killFor(t, g, "alien_enforcer")
	g.noteWatcherKill(enforcer)
	if len(g.watcherQueue) != 0 || g.watcherCampLine(false) != nil {
		t.Fatal("he spoke before his introduction")
	}
	g.markStorySeen(storyEvent(watcherIntro()), 1)
	g.noteWatcherKill(enforcer)
	if !slices.Equal(g.watcherQueue, []string{"kill_alien_enforcer"}) {
		t.Fatalf("queued %v, want kill_alien_enforcer", g.watcherQueue)
	}
	g.dialogActive = true
	g.tickWatcher()
	if g.watcherCard != nil {
		t.Fatal("his card opened over a dialog")
	}
	g.dialogActive = false
	g.menuOpen = true // the tabbed menu is no modal layer, but it holds the screen
	g.tickWatcher()
	if g.watcherCard != nil {
		t.Fatal("his card opened over the open menu")
	}
	g.menuOpen = false
	g.tickWatcher()
	if g.watcherCard == nil || !g.watcherSeen["kill_alien_enforcer"] {
		t.Fatal("his line did not play once the screen was free")
	}
	// Loading frames are unseen: the frame step they run (tickLoadingPause)
	// leaves the card's clock alone, and it runs again after.
	prev := g.gameLoop
	g.gameLoop = &GameLoop{game: g, loading: &gameLoadingState{awaitingFrame: true}}
	for i := 0; i < 2*g.watcherCardFrames(); i++ {
		g.updateInterfacePresentation()
	}
	if g.watcherCard == nil || g.watcherCard.elapsed != 0 {
		t.Fatalf("card %+v after a long load, want it still up and unaged", g.watcherCard)
	}
	g.gameLoop.loading.awaitingFrame = false
	g.updateInterfacePresentation()
	if g.watcherCard == nil || g.watcherCard.elapsed != 1 {
		t.Fatalf("card %+v once loaded, want its clock running", g.watcherCard)
	}
	g.gameLoop = prev
	// A dialog keeps the world running but covers the card: its clock waits
	// for the dialog to close.
	ih := &InputHandler{game: g}
	ih.openNPCInteraction(storyNPC(t, g, "mage_tower"))
	if !g.dialogActive {
		t.Fatal("fixture: the Tower's dialog did not open")
	}
	for i := 0; i < 2*g.watcherCardFrames(); i++ {
		g.updateInterfacePresentation()
	}
	if g.watcherCard == nil || g.watcherCard.elapsed != 1 {
		t.Fatalf("card %+v after a long dialog, want it still up and unaged", g.watcherCard)
	}
	g.closeConversation()
	g.updateInterfacePresentation()
	if g.watcherCard == nil || g.watcherCard.elapsed != 2 {
		t.Fatalf("card %+v once the dialog closed, want its clock running", g.watcherCard)
	}
	logged := false
	for _, e := range g.combatLogHistory {
		logged = logged || strings.Contains(e.Text, watcherLine("kill_alien_enforcer").Text)
	}
	if !logged {
		t.Fatal("his line is not in the combat log")
	}
	g.watcherCard = nil
	g.noteWatcherKill(killFor(t, g, "alien_enforcer"))
	if len(g.watcherQueue) != 0 {
		t.Fatal("a spoken line was queued again")
	}
}

// Bosses die in any order: when a story boss is the first boss the party
// kills, the kill queues his introduction and his own line together, and the
// card waits for the plate.
func TestWatcherStoryLineSurvivesTheFirstBossKill(t *testing.T) {
	g := watcherGame(t)
	// An Elder Dragon (no boss flag) killed before any boss introduces him too.
	g.noteStoryKill(killFor(t, g, "elder_dragon_green"))
	if !slices.Contains(g.storyQueue, watcherIntro()) || !slices.Equal(g.watcherQueue, []string{"kill_elder_dragon"}) {
		t.Fatalf("dragon first: story queue %v, captain queue %v", g.storyQueue, g.watcherQueue)
	}
	g.storyQueue, g.watcherQueue = nil, nil
	g.noteStoryKill(killFor(t, g, "ancient_god_of_death"))
	if !slices.Contains(g.storyQueue, watcherIntro()) || !slices.Equal(g.watcherQueue, []string{"kill_god_of_death"}) {
		t.Fatalf("story queue %v, captain queue %v", g.storyQueue, g.watcherQueue)
	}
	g.tickWatcher()
	g.tickStoryEvents()
	if g.watcherCard != nil || g.storyScreen == nil || g.storyScreen.event.ID != watcherIntro() {
		t.Fatal("his introduction must open first, before his card")
	}
	g.tickWatcher()
	if g.watcherCard != nil {
		t.Fatal("his card opened over his own introduction")
	}
	g.closeStoryScreen()
	g.tickWatcher()
	if g.watcherCard == nil || g.watcherCard.line.ID != "kill_god_of_death" {
		t.Fatalf("card %+v, want the God of Death's line after the plate", g.watcherCard)
	}
}

// A death moves the story and the captain exactly when it moves quests - one
// gate, through the real death path: a Dead Branch summon of the Enforcer, a
// wildlife prey, a caravan or the party's own summon is not his Enforcer.
func TestStoryKillCountsExactlyWhenQuestsDo(t *testing.T) {
	g := watcherGame(t)
	cs := NewCombatSystem(g)
	const bounty = "enforcer_bounty"
	g.questManager.Definitions()[bounty] = &quests.QuestDefinition{Name: "Enforcer Bounty", Type: quests.QuestTypeKill, TargetMonster: "alien_enforcer", TargetCount: 5}
	t.Cleanup(func() { delete(g.questManager.Definitions(), bounty) })
	if err := g.questManager.ActivateQuest(bounty); err != nil {
		t.Fatal(err)
	}
	q := g.questManager.GetQuest(bounty)
	ts := float64(g.config.GetTileSize())
	for _, row := range []struct {
		name   string
		mark   func(*monster.Monster3D)
		counts bool
	}{
		{"runtime summon", func(m *monster.Monster3D) { m.QuestProgressIgnored = true }, false},
		{"wildlife prey", func(m *monster.Monster3D) { m.NoKillRewards = true }, false},
		{"caravan", func(m *monster.Monster3D) { m.Disposition = monster.DispositionCaravan }, false},
		{"party summon", func(m *monster.Monster3D) { m.SummonedBy = spellSummonOwnerPrefix + "hero" }, false},
		{"the party's kill", func(*monster.Monster3D) {}, true}, // last: it moves the quest
	} {
		g.storyQueue, g.watcherQueue = nil, nil
		count := q.CurrentCount
		m := spawnMonsterAtTile(g, "alien_enforcer", TileIndex(g.camera.X, ts)+2, TileIndex(g.camera.Y, ts), ts)
		row.mark(m)
		m.HitPoints = 0
		cs.finishMonsterKill(m)
		story, captain := slices.Contains(g.storyQueue, watcherIntro()), slices.Contains(g.watcherQueue, "kill_alien_enforcer")
		quest := q.CurrentCount != count || q.Completed
		if story != row.counts || captain != row.counts || quest != row.counts {
			t.Errorf("%s: story %v, captain %v, quest %v; want all %v", row.name, story, captain, quest, row.counts)
		}
	}
}

// A story boss always speaks, each one in turn when several fall in one fight;
// every other remark rolls its chance, keeps the days apart and stops at the
// run's budget, which a story line never spends and whose days it never
// restarts.
func TestWatcherRandomRemarksKeepToTheirBudget(t *testing.T) {
	g := watcherGame(t)
	g.markStorySeen(storyEvent(watcherIntro()), 1)
	w := &g.config.Watcher
	w.MaxRandomLines = 1
	speak := func() *WatcherLine {
		if len(g.watcherQueue) == 0 {
			return nil
		}
		l := watcherLine(g.watcherQueue[0])
		g.tickWatcher()
		g.watcherCard = nil
		return l
	}

	g.noteWatcherKill(killFor(t, g, "orc_hero_boss"))
	if len(g.watcherQueue) != 0 {
		t.Fatal("a side boss spoke when its chance failed")
	}
	w.KillChance = 1
	g.noteWatcherKill(killFor(t, g, "golden_thief_bug"))
	g.noteWatcherKill(killFor(t, g, "dragon_brood_mother"))
	if l := speak(); l == nil || l.ID != "kill_brood_mother" {
		t.Fatalf("spoke %v, want the story line over the waiting random one", l)
	}
	if l := speak(); l != nil {
		t.Fatalf("the dropped random remark %s still played", l.ID)
	}
	g.noteWatcherKill(killFor(t, g, "orc_hero_boss"))
	if l := speak(); l == nil || l.ID != "kill_orc_warlord" {
		t.Fatalf("spoke %v, want the warlord's own line", l)
	}
	for _, key := range []string{"elder_dragon", "ancient_god_of_death", "elder_dragon_red"} {
		g.noteWatcherKill(killFor(t, g, key)) // one fight: two story bosses, one twice
	}
	for _, want := range []string{"kill_elder_dragon", "kill_god_of_death"} {
		if l := speak(); l == nil || l.ID != want {
			t.Fatalf("spoke %v, want %s", l, want)
		}
	}
	if l := speak(); l != nil {
		t.Fatalf("%s played twice", l.ID)
	}

	g.calendarDay += 5
	g.noteWatcherKill(killFor(t, g, "golden_thief_bug"))
	if len(g.watcherQueue) != 0 || shownAtCamp(g) != nil {
		t.Fatal("he spoke past the run's budget")
	}
	g.noteWatcherKill(killFor(t, g, "alien_enforcer"))
	if l := speak(); l == nil || l.ID != "kill_alien_enforcer" {
		t.Fatalf("spoke %v, want the story line past the budget", l)
	}
	w.MaxRandomLines = 2
	if shownAtCamp(g) == nil {
		t.Fatal("the story line spent the budget or restarted the days")
	}
	w.MaxRandomLines = 3
	if shownAtCamp(g) != nil {
		t.Fatal("two random remarks within the days apart")
	}
	g.calendarDay += w.MinDaysBetween
	if shownAtCamp(g) == nil {
		t.Fatal("no remark once the days had passed")
	}
}

// Camp visits: one per rest at most, never two within the configured days,
// no line twice; a party that lies down hurt hears the hurt line first, though
// the rest heals it before the scene starts; the Archmage and
// dragon branches open and close their lines; a rest carrying a line holds
// the camp scene until it is read; all of it survives a save.
func TestWatcherCampVisits(t *testing.T) {
	g := watcherGame(t)
	g.markStorySeen(storyEvent(watcherIntro()), 1)
	first := shownAtCamp(g)
	if first == nil || shownAtCamp(g) != nil {
		t.Fatal("want one camp line on day 1 and none again that day")
	}
	g.calendarDay += 2
	second := shownAtCamp(g)
	if second == nil || second.ID == first.ID {
		t.Fatal("a later visit must bring a new line")
	}

	// Only the party that lies down counts: a wounded hero on the bench is not.
	g.calendarDay += 2
	bench := character.NewParty(g.config).Members[0]
	bench.HitPoints = 1
	g.party.Reserve = []*character.MMCharacter{bench}
	g.party.Food = CampFoodCost
	g.campConfirmOpen = true
	g.resolveCampConfirmation(true)
	if s := g.campRest; s == nil || s.watcherLine == nil || s.watcherLine.PartyHurt {
		t.Fatalf("a rest with only a benched hero wounded heard %+v, want a plain line", g.campRest)
	}
	g.cancelCampPresentation() // cut short before his words: the line stays unspent
	m := g.party.Members[0]
	m.HitPoints = 1
	g.party.Food = CampFoodCost
	if reason := g.campBlocked(); reason != "" {
		t.Fatalf("the fixture cannot camp: %s", reason)
	}
	g.campConfirmOpen = true
	g.resolveCampConfirmation(true)
	if s := g.campRest; s == nil || s.watcherLine == nil || !s.watcherLine.PartyHurt || m.HitPoints != m.MaxHitPoints {
		t.Fatalf("a party that lay down hurt rested to %d HP and heard %+v, want healed and a hurt line", m.HitPoints, g.campRest)
	}
	g.cancelCampPresentation()

	// Stage 3 starts at the Raising or the first Elder Dragon kill, whichever
	// comes first, and that branch keeps its line set.
	g.watcherSeen = nil // the gates below, not the visits above
	open := func(id string) bool { return g.watcherLineOpen(watcherLine(id)) }
	stage := func(label string, opening, dragons, raised bool) {
		t.Helper()
		if got := [3]bool{open("camp_mage_hands"), open("camp_dragons_built"), open("camp_in_you")}; got != [3]bool{opening, dragons, raised} {
			t.Fatalf("%s: opening/dragons/raised lines open %v, want %v", label, got, [3]bool{opening, dragons, raised})
		}
	}
	stage("start", true, false, false)
	g.markStorySeen(storyEvent("four_seals"), 5)
	stage("the oath alone", true, false, false)
	g.watcherSeen = map[string]bool{"kill_elder_dragon": true}
	stage("first Elder Dragon", false, true, false)
	g.markStorySeen(storyEvent("raising"), 6)
	stage("then the Raising", false, false, true)
	g.watcherSeen = nil
	stage("the Raising first", false, false, true)
	g.watcherSeen = map[string]bool{"kill_elder_dragon": true}
	stage("dragons after the Raising", false, false, true)

	g.calendarDay += 2
	g.beginCampRest(false)
	s := g.campRest
	if s == nil || s.watcherLine == nil {
		t.Fatal("the rest carried no line")
	}
	s.presented = true
	logged := func() int {
		n := 0
		for _, e := range g.combatLogHistory {
			if strings.Contains(e.Text, s.watcherLine.Text) {
				n++
			}
		}
		return n
	}
	g.tickCampRest()
	if logged() != 0 {
		t.Fatal("his words reached the log before the strip showed them")
	}
	g.releaseCampWatcher() // still fading in: shows his words, keeps the scene
	if s.watcherRead || s.elapsed < s.watcherWordsAt(g.storyTiming()) {
		t.Fatal("an early press must show his words, not dismiss them")
	}
	g.tickCampRest()
	if logged() != 1 {
		t.Fatalf("his words logged %d times once shown, want 1", logged())
	}
	for i := 0; i < s.fadeIn+s.hold+s.fadeOut+10; i++ {
		g.tickCampRest()
	}
	if g.campRest == nil {
		t.Fatal("the camp scene ended before his line was read")
	}
	g.releaseCampWatcher()
	for i := 0; i < s.fadeOut+10; i++ {
		g.tickCampRest()
	}
	if g.campRest != nil || logged() != 1 {
		t.Fatalf("scene still up %v, logged %d times; want ended and logged once", g.campRest != nil, logged())
	}

	queued := []string{"kill_god_of_death", "kill_lich_king"}
	g.watcherQueue = slices.Clone(queued)
	save := g.buildSave(world.GlobalWorldManager)
	seen := len(g.watcherSeen)
	g.resetWatcher()
	g.restoreWatcher(&save)
	if len(g.watcherSeen) != seen || g.watcherLastDay != save.WatcherLastDay || !slices.Equal(g.watcherQueue, queued) {
		t.Fatalf("restored %d seen, day %d, queue %v", len(g.watcherSeen), g.watcherLastDay, g.watcherQueue)
	}
}

// The loader refuses lines that could never play or would not fit.
func TestWatcherConfigRejectsBadLines(t *testing.T) {
	_ = watcherGame(t)
	good := WatcherLine{ID: "x", When: watcherAtCamp, Text: "Sleep."}
	for name, bad := range map[string]WatcherLine{
		"unknown when":    {ID: "x", When: "dawn", Text: "Sleep."},
		"camp monsters":   {ID: "x", When: watcherAtCamp, Monsters: []string{"alien"}, Text: "Sleep."},
		"kill no target":  {ID: "x", When: watcherOnKill, Text: "Sleep."},
		"kill context":    {ID: "x", When: watcherOnKill, Boss: true, Night: true, Text: "Sleep."},
		"story camp":      {ID: "x", When: watcherAtCamp, Story: true, Text: "Sleep."},
		"story pool":      {ID: "x", When: watcherOnKill, Boss: true, Story: true, Text: "Sleep."},
		"boss and named":  {ID: "x", When: watcherOnKill, Monsters: []string{"alien"}, Boss: true, Text: "Sleep."},
		"unknown monster": {ID: "x", When: watcherOnKill, Monsters: []string{"no_such_mob"}, Text: "Sleep."},
		"unknown gate":    {ID: "x", When: watcherAtCamp, After: "no_such_beat", Text: "Sleep."},
		"story event id":  {ID: "tower", When: watcherAtCamp, Text: "Sleep."},
		"too long":        {ID: "x", When: watcherAtCamp, Text: strings.Repeat("a", watcherMaxText+1)},
	} {
		if err := validateWatcher(&watcherConfig{Speaker: "S", Portrait: "p", IntroEvent: "captain_first", Lines: []WatcherLine{bad}}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := validateWatcher(&watcherConfig{Speaker: "S", Portrait: "p", IntroEvent: "captain_first", Lines: []WatcherLine{good}}); err != nil {
		t.Fatalf("a good line was refused: %v", err)
	}
	if err := validateWatcher(&watcherConfig{Speaker: "S", Portrait: "p", IntroEvent: "no_such_beat", Lines: []WatcherLine{good}}); err == nil {
		t.Error("an unknown intro_event was accepted")
	}
}
