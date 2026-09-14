package game

import (
	"strings"
	"testing"

	"ugataima/internal/quests"
)

// bannerGame is a live game with the shipped quest catalog and the banner
// watcher already anchored to it (as boot does).
func bannerGame(t *testing.T) *MMGame {
	t.Helper()
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.questManager = loadTestQuestManager(t)
	g.resyncQuestBannerBaseline()
	return g
}

// bannerTexts ticks one frame and returns what the queue now holds, then clears
// it - so each step of a test reads only its own news.
func bannerTexts(g *MMGame) []string {
	g.tickScreenBanners()
	out := make([]string, 0, len(g.screenBannerQueue))
	for _, b := range g.screenBannerQueue {
		out = append(out, b.text)
	}
	g.screenBannerQueue = nil
	return out
}

// Boot has the endgame gate quests active already; adopting that state must not
// throw a banner for each of them.
func TestQuestBannerBaselineIsSilent(t *testing.T) {
	g := bannerGame(t)
	if got := bannerTexts(g); len(got) != 0 {
		t.Fatalf("boot raised banners: %v", got)
	}
}

// The whole lifecycle, driven ONLY through the quest manager: the watcher is
// what turns state changes into banners, so nothing calls a notifier by hand.
func TestQuestBannerFollowsTheQuestLifecycle(t *testing.T) {
	g := bannerGame(t)
	const qid = "dragon_cliffs_troll_cull" // kill 3 mountain_troll
	name := g.questManager.Definitions()[qid].Name

	if err := g.questManager.ActivateQuest(qid); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if got := bannerTexts(g); len(got) != 1 || got[0] != "New quest - "+name {
		t.Fatalf("taking the quest = %v, want one \"New quest\" banner", got)
	}

	g.questManager.OnMonsterKilled("mountain_troll", "")
	if got := bannerTexts(g); len(got) != 1 || got[0] != name+"  1/3" {
		t.Fatalf("first kill = %v, want the counter banner %q", got, name+"  1/3")
	}
	g.questManager.OnMonsterKilled("mountain_troll", "")
	if got := bannerTexts(g); len(got) != 1 || got[0] != name+"  2/3" {
		t.Fatalf("second kill = %v, want 2/3", got)
	}

	// The third kill completes it: the finish is the news, not the counter.
	g.questManager.OnMonsterKilled("mountain_troll", "")
	got := bannerTexts(g)
	if len(got) != 1 || got[0] != "Quest complete - "+name {
		t.Fatalf("completing kill = %v, want one \"Quest complete\" banner", got)
	}

	if _, err := g.questManager.ClaimRewards(qid); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if got := bannerTexts(g); len(got) != 1 || got[0] != "Reward claimed - "+name {
		t.Fatalf("turn-in = %v, want one \"Reward claimed\" banner", got)
	}
}

// An interact quest banners the same way - the watcher never learns what kind of
// progress source moved the counter.
func TestQuestBannerCountsInteractProgress(t *testing.T) {
	g := bannerGame(t)
	const qid = "shrine_lamps"
	name := g.questManager.Definitions()[qid].Name
	if err := g.questManager.ActivateQuest(qid); err != nil {
		t.Fatalf("activate: %v", err)
	}
	bannerTexts(g) // the "New quest" banner

	g.questManager.OnInteract("shrine_lamp")
	if got := bannerTexts(g); len(got) != 1 || got[0] != name+"  1/3" {
		t.Fatalf("first lamp = %v, want %q", got, name+"  1/3")
	}
}

// A quest that completes and pays out in one pass (auto_claim) gets ONE banner,
// the payout - not a complete/claim pair on the same frame.
func TestQuestBannerCollapsesCompletionAndPayout(t *testing.T) {
	g := bannerGame(t)
	const qid = "culverts_valves"
	if err := g.questManager.ActivateQuest(qid); err != nil {
		t.Fatalf("activate: %v", err)
	}
	bannerTexts(g)

	g.questManager.MarkCompleted(qid)
	if _, err := g.questManager.ClaimRewards(qid); err != nil {
		t.Fatalf("claim: %v", err)
	}
	got := bannerTexts(g)
	if len(got) != 1 || !strings.HasPrefix(got[0], "Reward claimed - ") {
		t.Fatalf("same-frame completion+payout = %v, want a single payout banner", got)
	}
}

// A repeatable errand is dropped from the manager at nightfall; taking it again
// is news again, so the watcher must forget the quests it can no longer see.
func TestQuestBannerRepeatsForARetakenRepeatable(t *testing.T) {
	g := bannerGame(t)
	const qid = "lake_spiders"
	name := g.questManager.Definitions()[qid].Name
	if err := g.questManager.ActivateQuest(qid); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if got := bannerTexts(g); len(got) != 1 || got[0] != "New quest - "+name {
		t.Fatalf("first take = %v, want a New quest banner", got)
	}
	g.questManager.RemoveQuest(qid) // what the nightly refresh does
	bannerTexts(g)
	if err := g.questManager.ActivateQuest(qid); err != nil {
		t.Fatalf("re-activate: %v", err)
	}
	if got := bannerTexts(g); len(got) != 1 || got[0] != "New quest - "+name {
		t.Fatalf("re-take = %v, want a New quest banner again", got)
	}
}

// A kill quest whose targets are ALREADY dead is completed inside
// handleGiveQuest, so the watcher meets it completed on its very first pass.
// Accepting it must still say something - silence would read as a broken giver.
func TestQuestBannerAnnouncesAQuestThatArrivesFinished(t *testing.T) {
	g := bannerGame(t)
	const qid = "wolf_pack"
	name := g.questManager.Definitions()[qid].Name

	// Activate + complete before the watcher ever sees it (what a cleared map
	// does through creditQuestIfCleared).
	if err := g.questManager.ActivateQuest(qid); err != nil {
		t.Fatalf("activate: %v", err)
	}
	g.questManager.MarkCompleted(qid)
	// No tick in between: the watcher meets this quest for the first time
	// already finished, exactly as it does after handleGiveQuest credits a
	// cleared map.

	got := bannerTexts(g)
	if len(got) != 1 || got[0] != "Quest complete - "+name {
		t.Fatalf("a quest that arrives finished raised %v, want one \"Quest complete\" banner", got)
	}

	// Same for one that arrives already paid out.
	if _, err := g.questManager.ClaimRewards(qid); err != nil {
		t.Fatalf("claim: %v", err)
	}
	delete(g.questBannerSeen, qid) // meet it fresh again, this time paid out
	if got := bannerTexts(g); len(got) != 1 || got[0] != "Reward claimed - "+name {
		t.Fatalf("a quest that arrives paid raised %v, want one payout banner", got)
	}
}

// An exterminate counter is derived from a live census, so it moves BACKWARDS
// when the map repopulates. Re-killing must not replay counters the player has
// already watched go past.
func TestQuestBannerDoesNotReplayAnExterminateCounter(t *testing.T) {
	g := bannerGame(t)
	const qid = "broodmother_nests" // starting exterminate quest
	q := g.questManager.GetQuest(qid)
	if q == nil {
		t.Fatalf("%s is not active at boot", qid)
	}
	g.questManager.SetCurrentCount(qid, 3)
	if got := bannerTexts(g); len(got) != 1 {
		t.Fatalf("the advance to 3 raised %v, want one banner", got)
	}
	// The cliffs repopulate: the census pushes the counter back down.
	g.questManager.SetCurrentCount(qid, 1)
	if got := bannerTexts(g); len(got) != 0 {
		t.Fatalf("going backwards raised %v, want silence", got)
	}
	// Re-killing what respawned walks back over 2 and 3 - already seen.
	for _, n := range []int{2, 3} {
		g.questManager.SetCurrentCount(qid, n)
		if got := bannerTexts(g); len(got) != 0 {
			t.Fatalf("re-reaching %d replayed %v", n, got)
		}
	}
	// Past the high-water mark it is news again.
	g.questManager.SetCurrentCount(qid, 4)
	if got := bannerTexts(g); len(got) != 1 {
		t.Fatalf("passing the previous best raised %v, want one banner", got)
	}
}

// One event can advance several quests at once. The journal is a Go map, so
// without the shared order (questIDLess) the two banners would queue in a
// different sequence run to run - and the combat log the champion pit writes
// would drift with them.
func TestQuestBannersQueueInQuestIDOrder(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.questManager = quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{
		"zulu_valves": {
			Name: "Zulu Valves", Type: quests.QuestTypeInteract,
			TargetMonster: "valve", TargetCount: 9,
		},
		"alpha_valves": {
			Name: "Alpha Valves", Type: quests.QuestTypeInteract,
			TargetMonster: "valve", TargetCount: 9,
		},
	}})
	for _, id := range []string{"zulu_valves", "alpha_valves"} {
		if err := g.questManager.ActivateQuest(id); err != nil {
			t.Fatalf("activate %s: %v", id, err)
		}
	}
	g.resyncQuestBannerBaseline()

	g.questManager.OnInteract("valve")
	got := bannerTexts(g)
	want := []string{"Alpha Valves  1/9", "Zulu Valves  1/9"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("banners queued as %v, want %v (quest ID order)", got, want)
	}
}

// A restored save is not news: loading adopts the journal silently, even one
// full of finished quests.
func TestQuestBannerSaveLoadBaselineIsSilent(t *testing.T) {
	g := bannerGame(t)
	if err := g.questManager.ActivateQuest("wolf_pack"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	g.questManager.MarkCompleted("wolf_pack")
	// What loadGame does after laying a save's journal back on the manager.
	g.screenBannerQueue = nil
	g.resyncQuestBannerBaseline()
	if got := bannerTexts(g); len(got) != 0 {
		t.Fatalf("a restored journal raised banners: %v", got)
	}
}

// Every banner line is plain ASCII (repo rule) and short enough to read.
func TestQuestBannerTextIsAsciiForEveryShippedQuest(t *testing.T) {
	g := bannerGame(t)
	kinds := []screenBannerKind{bannerQuestTaken, bannerQuestProgress, bannerQuestDone, bannerQuestPaid}
	for id := range g.questManager.Definitions() {
		if err := g.questManager.ActivateQuest(id); err != nil {
			continue // already active (the endgame gates)
		}
		q := g.questManager.GetQuest(id)
		for _, kind := range kinds {
			text := questBannerText(kind, q)
			for _, r := range text {
				if r > 126 || r < 32 {
					t.Fatalf("quest %q banner %q carries a non-ASCII rune %q", id, text, r)
				}
			}
			if geo := screenBannerLayout(1280, text, 0); geo.text != text {
				t.Errorf("quest %q banner %q does not fit the band: shown as %q", id, text, geo.text)
			}
		}
	}
}

// The watcher must be wired into the simulation tick - a banner system nothing
// calls is invisible.
func TestQuestBannerIsTickedByTheGameLoop(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.questManager = loadTestQuestManager(t)
	g.resyncQuestBannerBaseline()
	gl := NewGameLoop(g)
	if err := g.questManager.ActivateQuest("goblin_hunt"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	gl.updateSpecialEffects()
	if g.currentScreenBanner() == nil {
		t.Fatal("updateSpecialEffects did not pick up the new quest")
	}
	if got := g.currentScreenBanner(); got.frame != 1 {
		t.Fatalf("banner frame = %d after one simulation frame, want 1", got.frame)
	}
}
