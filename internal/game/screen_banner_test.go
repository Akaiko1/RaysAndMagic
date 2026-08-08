package game

import (
	"fmt"
	"image/color"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/world"
)

// The banner drops in, hangs at rest fully opaque, and rises back out fading -
// and it retires itself when the timeline ends.
func TestScreenBannerAnimationSlidesInHoldsAndLeaves(t *testing.T) {
	g := bannerGame(t)
	for _, kind := range []screenBannerKind{bannerQuestProgress, bannerInteractPrompt} {
		hold := g.bannerHoldFrames(kind)
		life := g.bannerLifetime(kind)

		alpha0, offset0 := g.screenBannerAnim(0, hold)
		if alpha0 != 0 || offset0 != bannerTravelPx {
			t.Fatalf("kind %d frame 0 = alpha %.2f offset %.1f, want fully faded at full travel", kind, alpha0, offset0)
		}
		prevAlpha, prevOffset := alpha0, offset0
		for f := 1; f <= g.bannerInFrames(); f++ {
			a, o := g.screenBannerAnim(f, hold)
			if a < prevAlpha || o > prevOffset {
				t.Fatalf("kind %d frame %d: alpha %.2f (was %.2f), offset %.1f (was %.1f) - the entry must only fade in and descend",
					kind, f, a, prevAlpha, o, prevOffset)
			}
			prevAlpha, prevOffset = a, o
		}
		if prevAlpha != 1 || prevOffset != 0 {
			t.Fatalf("kind %d end of entry = alpha %.2f offset %.1f, want it opaque at rest", kind, prevAlpha, prevOffset)
		}
		if a, o := g.screenBannerAnim(g.bannerInFrames()+hold/2, hold); a != 1 || o != 0 {
			t.Fatalf("kind %d mid-hold = alpha %.2f offset %.1f, want it parked and opaque", kind, a, o)
		}

		prevAlpha, prevOffset = 1, 0
		for f := g.bannerInFrames() + hold; f < life; f++ {
			a, o := g.screenBannerAnim(f, hold)
			if a > prevAlpha || o < prevOffset {
				t.Fatalf("kind %d frame %d: alpha %.2f (was %.2f), offset %.1f (was %.1f) - the exit must only fade out and rise",
					kind, f, a, prevAlpha, o, prevOffset)
			}
			prevAlpha, prevOffset = a, o
		}
		if prevAlpha > 0.1 {
			t.Fatalf("kind %d last frame alpha = %.2f, want it nearly gone", kind, prevAlpha)
		}
		// It leaves the way it came in: back up toward the top edge, not in place.
		if prevOffset < bannerTravelPx*0.9 {
			t.Fatalf("kind %d last frame offset = %.1f, want it back near the full travel %d - the banner must slide out, not just fade",
				kind, prevOffset, bannerTravelPx)
		}
	}

	// A nudge hangs for less time than news does.
	if g.bannerHoldFrames(bannerInteractPrompt) >= g.bannerHoldFrames(bannerQuestDone) {
		t.Fatalf("the interact prompt hold (%d) must be shorter than a quest banner's (%d)",
			g.bannerHoldFrames(bannerInteractPrompt), g.bannerHoldFrames(bannerQuestDone))
	}

	// And the queue drains on its own, at the kind's own pace.
	if err := g.questManager.ActivateQuest("goblin_hunt"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	for i := 0; i < g.bannerLifetime(bannerQuestTaken)+1; i++ {
		g.tickScreenBanners()
	}
	if got := g.currentScreenBanner(); got != nil {
		t.Fatalf("banner %q is still on screen after its timeline", got.text)
	}
}

// Banners never pile up without bound, and the newest news always survives.
func TestScreenBannerQueueIsBounded(t *testing.T) {
	g := bannerGame(t)
	const qid = "forest_wolf_cull" // 21 wolves: plenty of counter events
	if err := g.questManager.ActivateQuest(qid); err != nil {
		t.Fatalf("activate: %v", err)
	}
	name := g.questManager.Definitions()[qid].Name
	for i := 0; i < 12; i++ {
		g.questManager.OnMonsterKilled("wolf", "")
		g.syncQuestBanners(true)
	}
	if len(g.screenBannerQueue) > bannerQueueMax {
		t.Fatalf("queue length %d exceeds the cap %d", len(g.screenBannerQueue), bannerQueueMax)
	}
	last := g.screenBannerQueue[len(g.screenBannerQueue)-1].text
	if want := name + "  12/21"; last != want {
		t.Fatalf("newest banner = %q, want %q", last, want)
	}
}

// Two sources raising the same line in one frame must not stutter it twice.
func TestScreenBannerSkipsAConsecutiveDuplicate(t *testing.T) {
	g := bannerGame(t)
	g.queueBanner(bannerQuestProgress, "Stem the Flow  1/7")
	g.queueBanner(bannerQuestProgress, "Stem the Flow  1/7")
	if len(g.screenBannerQueue) != 1 {
		t.Fatalf("queue = %d banners, want the duplicate dropped", len(g.screenBannerQueue))
	}
}

// The banner sits at the top between the corner readouts, and a long line is
// clipped to that band instead of running under them.
func TestScreenBannerLayoutStaysClearOfTheHudCorners(t *testing.T) {
	const screenW = 1280
	long := "New quest - " + strings.Repeat("Wolves of the Lakeshore ", 6)
	geo := screenBannerLayout(screenW, long, 0)
	if geo.plateX < bannerCornerReservePx {
		t.Fatalf("plate starts at x=%d, inside the reserved %dpx corner", geo.plateX, bannerCornerReservePx)
	}
	if right := geo.plateX + geo.plateW; right > screenW-bannerCornerReservePx {
		t.Fatalf("plate ends at x=%d, inside the right reserved corner (screen %d)", right, screenW)
	}
	if geo.text == long {
		t.Fatal("a line that cannot fit the band must be clipped")
	}
	if geo.plateY < 0 {
		t.Fatalf("plate top y=%d is off-screen", geo.plateY)
	}
	entering := screenBannerLayout(screenW, "New quest - Short", bannerTravelPx)
	resting := screenBannerLayout(screenW, "New quest - Short", 0)
	if entering.cy >= resting.cy {
		t.Fatalf("entering cy=%d must be above the rest position cy=%d", entering.cy, resting.cy)
	}
}

// The approach nudge is white and flat; the payoffs and the legendary drop wear
// their own metal.
func TestScreenBannerTintsPerKind(t *testing.T) {
	if got := screenBannerTint(bannerInteractPrompt); got != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("interact prompt tint = %v, want plain white", got)
	}
	if got := screenBannerTint(bannerLegendaryDrop); got != rarityFire {
		t.Fatalf("legendary tint = %v, want the legendary rarity colour %v", got, rarityFire)
	}
	if got := screenBannerTint(bannerQuestDone); got != rarityGold {
		t.Fatalf("quest-complete tint = %v, want %v", got, rarityGold)
	}
	// Every quest banner must wear the same treatment: the heading renderer only
	// gives its brushed gradient to colours in metallicColors, so a tint left out
	// would split the four quest kinds into flat and metal.
	for _, kind := range []screenBannerKind{bannerQuestTaken, bannerQuestProgress, bannerQuestDone, bannerQuestPaid} {
		if c := screenBannerTint(kind); !metallicColors[c] {
			t.Fatalf("quest banner kind %d uses %v, which is not in metallicColors - it would render flat", kind, c)
		}
	}
	// The nudge is deliberately the exception: a hint should look like a hint.
	if metallicColors[screenBannerTint(bannerInteractPrompt)] {
		t.Fatal("the interact nudge should stay flat white, not become a metal heading")
	}
}

// newPromptNPC builds a talkable prop WITHOUT placing it anywhere - for the one
// test that needs an NPC the party cannot be standing at.
func newPromptNPC(name string) *character.NPC {
	return &character.NPC{
		Name: name, Type: character.NPCTypeEncounter, RenderCategory: "npc",
		DialogueData: &character.NPCDialogue{Greeting: "hm"},
	}
}

// promptTestNPC places the prop in the party's CURRENT world, which is what the
// producer requires before it will announce anything (npcInCurrentWorld).
func promptTestNPC(g *MMGame, name string) *character.NPC {
	npc := newPromptNPC(name)
	g.world.NPCs = append(g.world.NPCs, npc)
	return npc
}

// The nudge fires ONCE when interact focus lands on something, stays quiet while
// the party keeps standing there, and fires again on a fresh approach.
func TestInteractPromptBannerFiresOncePerApproach(t *testing.T) {
	g := bannerGame(t)
	npc := promptTestNPC(g, "Barrel")
	want := g.interactionPromptText(npc)
	if want == "" {
		t.Fatal("the shared prompt builder produced nothing")
	}

	g.focusedNPC = npc
	g.tickScreenBanners()
	b := g.currentScreenBanner()
	if b == nil || b.text != want || b.kind != bannerInteractPrompt {
		t.Fatalf("approach banner = %+v, want the prompt %q", b, want)
	}
	firstFrame := b.frame

	// Standing there must not re-raise it.
	for i := 0; i < 5; i++ {
		g.tickScreenBanners()
	}
	if len(g.screenBannerQueue) != 1 {
		t.Fatalf("standing in focus queued %d banners, want the one", len(g.screenBannerQueue))
	}
	if g.currentScreenBanner().frame <= firstFrame {
		t.Fatal("the banner is not ageing while the party stands there")
	}

	// Walk away for real - long enough that it is not a combat blip - and come
	// back: news again.
	g.focusedNPC = nil
	for i := 0; i < g.framesForSeconds(bannerPromptForgetSeconds); i++ {
		g.tickScreenBanners()
	}
	g.screenBannerQueue = nil
	g.focusedNPC = npc
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b == nil || b.text != want {
		t.Fatalf("a fresh approach raised %+v, want the prompt again", b)
	}
}

// Only the newest nudge matters: walking past a row of props must not queue a
// backlog of prompts, and a prompt must never displace quest news.
func TestInteractPromptBannerKeepsOnlyTheNewestAndYieldsToNews(t *testing.T) {
	g := bannerGame(t)
	for _, name := range []string{"Barrel", "Crate", "Chest"} {
		g.focusedNPC = promptTestNPC(g, name)
		g.tickScreenBanners()
	}
	if len(g.screenBannerQueue) != 1 {
		t.Fatalf("three approaches queued %d banners, want only the newest", len(g.screenBannerQueue))
	}
	if got := g.currentScreenBanner().text; !strings.Contains(got, "Chest") {
		t.Fatalf("the banner on screen is %q, want the newest approach (Chest)", got)
	}

	// Quest news arrives: it queues behind the nudge and is NOT dropped when the
	// next nudge comes in.
	g.queueBanner(bannerQuestDone, "Quest complete - Test")
	g.focusedNPC = promptTestNPC(g, "Sack")
	g.tickScreenBanners()
	found := false
	for _, b := range g.screenBannerQueue {
		if b.kind == bannerQuestDone {
			found = true
		}
	}
	if !found {
		t.Fatalf("the quest banner was dropped by a prompt: %+v", g.screenBannerQueue)
	}
}

// A nudge must never cost the player a quest banner: with the queue full it is
// the nudge that is dropped, not the oldest waiting news.
func TestInteractPromptBannerNeverDisplacesNews(t *testing.T) {
	g := bannerGame(t)
	for i := 0; i < bannerQueueMax; i++ {
		g.queueBanner(bannerQuestProgress, "Errand  "+string(rune('1'+i))+"/9")
	}
	if len(g.screenBannerQueue) != bannerQueueMax {
		t.Fatalf("fixture queued %d banners, want a full queue of %d", len(g.screenBannerQueue), bannerQueueMax)
	}
	before := append([]screenBanner(nil), g.screenBannerQueue...)

	g.focusedNPC = promptTestNPC(g, "Barrel")
	g.tickScreenBanners()

	for i, b := range before {
		if i >= len(g.screenBannerQueue) || g.screenBannerQueue[i].text != b.text {
			t.Fatalf("the nudge evicted news: queue is now %v, was %v", g.screenBannerQueue, before)
		}
	}
	for _, b := range g.screenBannerQueue {
		if b.kind == bannerInteractPrompt {
			t.Fatal("the nudge was queued into a full queue - it must be dropped instead")
		}
	}
}

// Same rule from the other side: a nudge ALREADY on screen is what the overflow
// trim throws away, not a piece of news waiting behind it. The player can bring
// a nudge back by stepping away and returning; a quest banner never shown is
// gone for good.
func TestBannerOverflowSacrificesTheNudgeNotTheNews(t *testing.T) {
	g := bannerGame(t)
	g.focusedNPC = promptTestNPC(g, "Barrel")
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b == nil || b.kind != bannerInteractPrompt {
		t.Fatalf("fixture wanted the nudge on screen, got %+v", b)
	}

	news := []string{"Errand  1/9", "Errand  2/9", "Errand  3/9", "Errand  4/9"}
	for _, text := range news {
		g.queueBanner(bannerQuestProgress, text)
	}
	if len(g.screenBannerQueue) != bannerQueueMax {
		t.Fatalf("queue holds %d, want it trimmed to %d", len(g.screenBannerQueue), bannerQueueMax)
	}
	for i, want := range news {
		if g.screenBannerQueue[i].text != want {
			t.Fatalf("queue = %v, want every news line kept in order (%v)", g.screenBannerQueue, news)
		}
	}
}

// Walking away must take the un-shown nudge with it: queued behind a quest
// banner it would otherwise surface seconds later, pointing at nothing.
func TestInteractPromptBannerDropsWhenFocusIsLost(t *testing.T) {
	g := bannerGame(t)
	g.queueBanner(bannerQuestDone, "Quest complete - Test")
	g.focusedNPC = promptTestNPC(g, "Barrel")
	g.tickScreenBanners()
	pending := 0
	for _, b := range g.screenBannerQueue {
		if b.kind == bannerInteractPrompt {
			pending++
		}
	}
	if pending != 1 {
		t.Fatalf("fixture has %d queued nudges, want exactly one waiting behind the news", pending)
	}

	g.focusedNPC = nil
	g.tickScreenBanners()
	for _, b := range g.screenBannerQueue {
		if b.kind == bannerInteractPrompt {
			t.Fatalf("a nudge for an NPC the party left is still queued: %q", b.text)
		}
	}
}

// Chests and lecterns have NO dialog - interacting IS the effect - so their
// interaction returns early. The nudge must still be settled: pressing Space is
// the answer to it whatever the object does with the press.
func TestInteractPromptBannerIsSettledByAnImmediateUseProp(t *testing.T) {
	g := bannerGame(t)
	ih := NewInputHandler(g)
	for _, npcType := range []string{character.NPCTypeLootCrate, character.NPCTypeSpellLectern} {
		t.Run(npcType, func(t *testing.T) {
			g.screenBannerQueue = nil
			g.bannerPromptNPC = nil
			prop := promptTestNPC(g, "Iron Chest")
			prop.Type = npcType
			g.focusedNPC = prop
			g.tickScreenBanners()
			if b := g.currentScreenBanner(); b == nil || b.kind != bannerInteractPrompt {
				t.Fatalf("fixture raised %+v, want the nudge", b)
			}

			ih.openNPCInteraction(prop)
			for _, b := range g.screenBannerQueue {
				if b.kind == bannerInteractPrompt {
					t.Fatalf("using the prop left its nudge %q on screen", b.text)
				}
			}
		})
	}
}

// A NEW GAME starts clean: no heading from the old run, no journal snapshot to
// diff against, no focus identity into a world that is being rebuilt.
func TestScreenBannersResetForANewRun(t *testing.T) {
	g := bannerGame(t)
	g.queueBanner(bannerLegendaryDrop, "Legendary drop - Old Run Sword")
	g.bannerPromptNPC = promptTestNPC(g, "Ghost of the old world")
	if err := g.questManager.ActivateQuest("goblin_hunt"); err != nil {
		t.Fatalf("activate: %v", err)
	}

	// What startNewGameWithParty does after the quest manager is reset.
	g.questManager.Reset()
	g.resetScreenBanners()

	if b := g.currentScreenBanner(); b != nil {
		t.Fatalf("the new run opened with the old run's banner %q", b.text)
	}
	if g.bannerPromptNPC != nil {
		t.Fatal("the new run kept a focus identity from the old world")
	}
	// And the fresh journal is adopted silently, not announced.
	if got := bannerTexts(g); len(got) != 0 {
		t.Fatalf("the new run announced its fresh journal: %v", got)
	}
}

// Opening a conversation settles that object's nudge: the queued one goes now
// (the tick stops for the dialog, so it would otherwise surface AFTER the talk),
// and closing the dialog with the same NPC still in focus raises nothing - the
// party just spoke to them.
func TestInteractPromptBannerIsSettledByOpeningTheDialog(t *testing.T) {
	g := bannerGame(t)
	ih := NewInputHandler(g)
	npc := promptTestNPC(g, "Keeper")

	// A nudge is queued behind quest news, so it has not been shown yet.
	g.queueBanner(bannerQuestDone, "Quest complete - Test")
	g.focusedNPC = npc
	g.tickScreenBanners()
	queued := false
	for _, b := range g.screenBannerQueue {
		if b.kind == bannerInteractPrompt {
			queued = true
		}
	}
	if !queued {
		t.Fatal("fixture did not queue a nudge behind the news")
	}

	ih.openNPCInteraction(npc)
	for _, b := range g.screenBannerQueue {
		if b.kind == bannerInteractPrompt {
			t.Fatalf("opening the dialog left the nudge %q queued", b.text)
		}
	}

	// And the same when the nudge is the banner ON SCREEN, which is the common
	// case: walk up, read "Press SPACE to talk to X", press SPACE. It must not
	// hover over the conversation it announced.
	g.screenBannerQueue = nil
	g.bannerPromptNPC = nil
	g.dialogActive, g.dialogNPC = false, nil
	g.focusedNPC = npc
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b == nil || b.kind != bannerInteractPrompt {
		t.Fatalf("fixture did not put the nudge on screen: %+v", b)
	}
	ih.openNPCInteraction(npc)
	if b := g.currentScreenBanner(); b != nil && b.kind == bannerInteractPrompt {
		t.Fatalf("the nudge %q is still on screen over the open dialog", b.text)
	}
	if b := g.visibleScreenBanner(); b != nil && b.kind == bannerInteractPrompt {
		t.Fatalf("the renderer would still paint the nudge %q over the dialog", b.text)
	}

	// Close the dialog; focus never left the NPC.
	g.dialogActive = false
	g.dialogNPC = nil
	for i := 0; i < 3; i++ {
		g.tickScreenBanners()
	}
	for _, b := range g.screenBannerQueue {
		if b.kind == bannerInteractPrompt {
			t.Fatalf("a nudge appeared after the conversation: %q", b.text)
		}
	}
}

// A frozen banner must not sit over a paused overlay: the tick stops with the
// world, but the HUD keeps being drawn under the menu.
func TestScreenBannerIsNotPaintedUnderAPausedOverlay(t *testing.T) {
	g := bannerGame(t)
	g.queueBanner(bannerQuestDone, "Quest complete - Test")
	if g.gameplayPausedByOverlay() || g.visibleScreenBanner() == nil {
		t.Fatal("fixture starts paused or without a banner")
	}
	g.menuOpen = true
	if !g.gameplayPausedByOverlay() {
		t.Fatal("the character hub must pause the world")
	}
	if g.visibleScreenBanner() != nil {
		t.Fatal("a banner is painted under a paused overlay - it cannot animate there")
	}
	// It keeps its place in the queue and comes back when the overlay closes.
	if g.currentScreenBanner() == nil {
		t.Fatal("the paused overlay dropped the banner instead of hiding it")
	}
	g.menuOpen = false
	if g.visibleScreenBanner() == nil {
		t.Fatal("closing the overlay did not bring the banner back")
	}
}

// Loading a save is not news either - and because loading does NOT reload maps,
// the old run's NPC pointers survive: the focus identity has to be cleared too,
// or the NPC the party is standing at never nudges again.
func TestScreenBannersResetWhenASaveIsLoaded(t *testing.T) {
	cfg := loadTestConfig(t)
	wm := world.NewWorldManager(cfg)
	w := newTestWorld(cfg)
	wm.LoadedMaps = map[string]*world.World3D{"forest": w}
	wm.CurrentMapKey = "forest"

	game := newTestGame(cfg, w)
	save := game.buildSave(wm)

	loaded := newTestGame(cfg, w)
	loaded.questManager = loadTestQuestManager(t)
	loaded.resyncQuestBannerBaseline()
	standing := promptTestNPC(loaded, "Silverbough Gate")
	loaded.bannerPromptNPC = standing
	loaded.queueBanner(bannerLegendaryDrop, "Legendary drop - From The Session Before")

	if err := loaded.applySave(wm, &save); err != nil {
		t.Fatalf("apply save: %v", err)
	}
	if b := loaded.currentScreenBanner(); b != nil {
		t.Fatalf("the loaded run opened with a banner from before the load: %q", b.text)
	}
	if loaded.bannerPromptNPC != nil {
		t.Fatal("the loaded run kept the pre-load focus identity, so that NPC will never nudge again")
	}
}

// Interact focus flickers at the edge of its cone. Re-arriving on the SAME
// object must not restart the nudge's timeline, or it jitters in its slide-in
// forever instead of holding and retiring.
func TestInteractPromptBannerSurvivesAFocusFlicker(t *testing.T) {
	g := bannerGame(t)
	npc := promptTestNPC(g, "Barrel")
	g.focusedNPC = npc
	g.tickScreenBanners()
	for i := 0; i < 20; i++ {
		g.tickScreenBanners()
	}
	aged := g.currentScreenBanner().frame
	if aged < 20 {
		t.Fatalf("banner only reached frame %d, fixture is wrong", aged)
	}
	for i := 0; i < 6; i++ { // flicker: out of focus and back, twice a frame apart
		g.focusedNPC = nil
		g.tickScreenBanners()
		g.focusedNPC = npc
		g.tickScreenBanners()
	}
	if got := g.currentScreenBanner(); got == nil {
		t.Fatal("the flicker dropped the nudge entirely")
	} else if got.frame <= aged {
		t.Fatalf("the flicker restarted the nudge: frame %d, was %d - it would never finish", got.frame, aged)
	}
}

// A map change ends the approach: the old map's NPC must not stay pinned as the
// prompt target (it would silence the first thing focused after arrival, and it
// keeps that NPC reachable from the game struct).
func TestInteractPromptTargetIsForgottenOnMapChange(t *testing.T) {
	g := bannerGame(t)
	npc := promptTestNPC(g, "Silverbough Gate")
	g.focusedNPC = npc
	g.tickScreenBanners()
	if g.bannerPromptNPC != npc {
		t.Fatal("fixture did not record the approach")
	}
	g.forgetInteractPromptTarget()
	if g.bannerPromptNPC != nil {
		t.Fatal("the map change kept the old map's NPC pinned")
	}
	for _, b := range g.screenBannerQueue {
		if b.kind == bannerInteractPrompt {
			t.Fatalf("a nudge for the map we left is still queued: %q", b.text)
		}
	}
}

// Combat nils interact focus wholesale (updateFocusedNPC), so a monster
// wandering past a shop must not count as walking away and back.
func TestInteractPromptBannerIgnoresACombatFocusBlip(t *testing.T) {
	g := bannerGame(t)
	npc := promptTestNPC(g, "Zaira")
	g.focusedNPC = npc
	g.tickScreenBanners()
	if g.currentScreenBanner() == nil {
		t.Fatal("fixture raised no nudge")
	}
	g.screenBannerQueue = nil // the player has read it

	for blip := 0; blip < 3; blip++ {
		g.focusedNPC = nil // a mob steps into interaction range
		for i := 0; i < 20; i++ {
			g.tickScreenBanners()
		}
		g.focusedNPC = npc // and dies or wanders off
		g.tickScreenBanners()
		if b := g.currentScreenBanner(); b != nil {
			t.Fatalf("combat blip %d re-raised the nudge %q without the party moving", blip, b.text)
		}
	}
}

// Two props side by side make focus alternate. The nudge may swap its text, but
// restarting its timeline on every flip would pin it in the slide-in forever.
func TestInteractPromptBannerKeepsItsTimelineAcrossTwoProps(t *testing.T) {
	g := bannerGame(t)
	a, b := promptTestNPC(g, "Lamp"), promptTestNPC(g, "Chest")
	g.focusedNPC = a
	g.tickScreenBanners()
	for i := 0; i < 15; i++ {
		g.tickScreenBanners()
	}
	aged := g.currentScreenBanner().frame

	for flip := 0; flip < 4; flip++ {
		g.focusedNPC = b
		g.tickScreenBanners()
		g.focusedNPC = a
		g.tickScreenBanners()
	}
	got := g.currentScreenBanner()
	if got == nil {
		t.Fatal("the flicker dropped the nudge")
	}
	if got.frame <= aged {
		t.Fatalf("the flicker restarted the timeline: frame %d, was %d", got.frame, aged)
	}
	if len(g.screenBannerQueue) != 1 {
		t.Fatalf("the flicker queued %d banners, want the one", len(g.screenBannerQueue))
	}
}

// Interact focus outlives a map change by a frame (it is resolved once per
// frame), so the producer must refuse to announce an NPC that is not in the
// world the party is standing in - whichever way they arrived.
func TestInteractPromptOnlyAnnouncesSomethingOnThisMap(t *testing.T) {
	g := bannerGame(t)
	here := promptTestNPC(g, "Local Barrel")

	// Something the party can actually walk up to: announced.
	g.focusedNPC = here
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b == nil || b.kind != bannerInteractPrompt {
		t.Fatalf("an NPC on this map raised %+v, want the nudge", b)
	}
	g.screenBannerQueue = nil

	// A nudge for the local NPC is left QUEUED behind news, then the party
	// arrives somewhere else by a path that is not switchToMap (the underwater
	// return, an encounter map): the queued nudge must go with the old map.
	g.queueBanner(bannerQuestDone, "Quest complete - Test")
	g.bannerPromptNPC = nil
	g.focusedNPC = here
	g.tickScreenBanners()
	queued := false
	for _, b := range g.screenBannerQueue {
		if b.kind == bannerInteractPrompt {
			queued = true
		}
	}
	if !queued {
		t.Fatal("fixture did not queue a nudge behind the news")
	}

	// A leftover focus from the map just left: silent, and forgotten.
	departed := newPromptNPC("Pyramid Stairs") // deliberately NOT in g.world.NPCs
	g.focusedNPC = departed
	g.tickScreenBanners()
	for _, b := range g.screenBannerQueue {
		if b.kind == bannerInteractPrompt {
			t.Fatalf("a nudge for the map just left is still queued: %q", b.text)
		}
	}
	g.screenBannerQueue = nil
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b != nil {
		t.Fatalf("an NPC from another map raised %q", b.text)
	}
	if g.bannerPromptNPC != nil {
		t.Fatal("the producer kept a focus identity that is not on this map")
	}
}

// Talking is not approaching: no nudge while a dialog is open.
func TestInteractPromptBannerIsSilentDuringADialog(t *testing.T) {
	g := bannerGame(t)
	g.dialogActive = true
	g.focusedNPC = promptTestNPC(g, "Keeper")
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b != nil {
		t.Fatalf("a dialog raised the approach nudge %q", b.text)
	}
}

// Focus keeps resolving during a conversation, so it can land on a SECOND prop
// (a shifting cone, a walker crossing the aisle) while the player is talking to
// the first. Silence then is right - but the producer must not record that prop
// as prompted, or it stays silent for good once the talk ends.
func TestInteractPromptBannerStillNudgesWhatFocusFoundDuringADialog(t *testing.T) {
	g := bannerGame(t)
	ih := NewInputHandler(g)
	keeper, lectern := promptTestNPC(g, "Keeper"), promptTestNPC(g, "Lectern")

	ih.openNPCInteraction(keeper)
	if !g.dialogActive {
		t.Fatal("fixture did not open the dialog")
	}
	g.focusedNPC = lectern
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b != nil {
		t.Fatalf("focus moving during the talk raised %q", b.text)
	}

	// The talk ends with the lectern still in front of the party.
	g.dialogActive, g.dialogNPC = false, nil
	g.tickScreenBanners()
	b := g.currentScreenBanner()
	if b == nil || b.kind != bannerInteractPrompt {
		t.Fatalf("closing the dialog raised %+v, want the lectern's nudge", b)
	}
	if want := g.interactionPromptText(lectern); b.text != want {
		t.Fatalf("nudge = %q, want %q", b.text, want)
	}
}

// The text swap that absorbs a focus flicker must not hand a NEW target the tail
// of a nudge that is already sliding out - it would flash for a few frames at
// falling alpha and vanish.
func TestInteractPromptBannerRestartsWhenTheOldNudgeIsLeaving(t *testing.T) {
	g := bannerGame(t)
	a, b := promptTestNPC(g, "Lamp"), promptTestNPC(g, "Chest")
	g.focusedNPC = a
	g.tickScreenBanners()
	// Age it into the slide-out.
	for g.currentScreenBanner().frame < g.bannerInFrames()+g.bannerHoldFrames(bannerInteractPrompt) {
		g.tickScreenBanners()
	}
	leaving := g.currentScreenBanner().frame

	g.focusedNPC = b
	g.tickScreenBanners()
	got := g.currentScreenBanner()
	if got == nil || got.kind != bannerInteractPrompt {
		t.Fatalf("the new approach raised %+v, want a nudge", got)
	}
	if want := g.interactionPromptText(b); got.text != want {
		t.Fatalf("nudge = %q, want %q", got.text, want)
	}
	if got.frame >= leaving {
		t.Fatalf("the new nudge inherited the spent timeline (frame %d, the old one was at %d)", got.frame, leaving)
	}
	if alpha, _ := g.screenBannerAnim(got.frame, g.bannerHoldFrames(bannerInteractPrompt)); alpha >= 1 {
		t.Fatalf("a restarted nudge should be sliding in, got alpha %.2f", alpha)
	}
}

// A legendary drop names itself; anything less does not.
func TestLegendaryDropBannerNamesEveryLegendary(t *testing.T) {
	g := bannerGame(t)

	g.announceLegendaryDrops([]items.Item{
		{Name: "Iron Sword", Rarity: "common"},
		{Name: "Ruby", Rarity: "rare"},
	})
	g.tickScreenBanners() // the drop banner is raised by the tick, not by the drop
	if b := g.currentScreenBanner(); b != nil {
		t.Fatalf("a common/rare drop raised %q", b.text)
	}

	g.announceLegendaryDrops([]items.Item{
		{Name: "Iron Sword", Rarity: "common"},
		{Name: "Wyrmcleaver", Rarity: "Legendary"}, // rarity is authored free-case
		{Name: "Broodscale Aegis", Rarity: "legendary"},
	})
	g.tickScreenBanners()
	b := g.currentScreenBanner()
	if b == nil {
		t.Fatal("a legendary drop raised no banner")
	}
	if b.kind != bannerLegendaryDrop {
		t.Fatalf("banner kind = %d, want the legendary kind", b.kind)
	}
	for _, want := range []string{"Wyrmcleaver", "Broodscale Aegis", "Legendary drops"} {
		if !strings.Contains(b.text, want) {
			t.Fatalf("banner %q does not mention %q", b.text, want)
		}
	}
	if strings.Contains(b.text, "Iron Sword") {
		t.Fatalf("banner %q lists a common item", b.text)
	}
}

// A container refused as a duplicate puts nothing on the floor, so it announces
// nothing: ID is the generic dedup key, and a spawn that hits it must be silent.
func TestLegendaryDropBannerIgnoresADuplicateContainer(t *testing.T) {
	g := bannerGame(t)
	bag := GroundContainer{
		ID:    "pyramid_reliquary_1",
		Kind:  ContainerKindLootBag,
		X:     100,
		Y:     100,
		Items: []items.Item{{Name: "Sunspine Reliquary", Rarity: "legendary"}},
	}
	g.addGroundContainer(bag)
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b == nil || !strings.Contains(b.text, "Sunspine Reliquary") {
		t.Fatalf("the first spawn raised %+v, want the legendary banner", b)
	}
	g.screenBannerQueue = nil

	g.addGroundContainer(bag) // same ID: the spawn is refused
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b != nil {
		t.Fatalf("a duplicate container announced %q with nothing added to the floor", b.text)
	}
	if n := len(g.groundContainers); n != 1 {
		t.Fatalf("the floor holds %d containers, want the one", n)
	}
}

// A CHEST announces when its lid comes up, not when it spawns: an encounter
// clear can drop one a region away, sealed, and naming the contents then spoils
// a container the party has not opened and may never reach.
func TestLegendaryChestAnnouncesOnOpenNotOnSpawn(t *testing.T) {
	g := bannerGame(t)
	g.addGroundContainer(GroundContainer{
		ID:    "pyramid_reliquary_1",
		Kind:  ContainerKindTreasureChest,
		X:     100,
		Y:     100,
		Items: []items.Item{{Name: "Sunspine Reliquary", Rarity: "legendary"}},
	})
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b != nil {
		t.Fatalf("a sealed chest announced %q at spawn", b.text)
	}
	if n := len(g.groundContainers); n != 1 {
		t.Fatalf("the chest itself must still be spawned, got %d containers", n)
	}

	g.pickupGroundContainerAt(0)
	g.tickScreenBanners()
	b := g.currentScreenBanner()
	if b == nil || !strings.Contains(b.text, "Sunspine Reliquary") {
		t.Fatalf("opening the chest raised %+v, want the legendary banner", b)
	}
	if b.kind != bannerLegendaryDrop {
		t.Fatalf("banner kind = %d, want the legendary kind", b.kind)
	}
}

// The drop banner rides the ground-drop funnel, so every death path announces
// alike instead of each caller remembering to.
func TestLegendaryDropBannerRidesTheDropFunnel(t *testing.T) {
	g := bannerGame(t)
	g.addLootBagDrop(100, 100, []items.Item{{Name: "Wyrmspine Wing", Rarity: "legendary"}}, 0)
	g.tickScreenBanners()
	b := g.currentScreenBanner()
	if b == nil || !strings.Contains(b.text, "Wyrmspine Wing") {
		t.Fatalf("a mob drop raised %+v, want the legendary banner", b)
	}
	if b.kind != bannerLegendaryDrop {
		t.Fatalf("banner kind = %d, want the legendary kind", b.kind)
	}
}

// One event can put SEVERAL containers on the floor - a wiped pack drops a bag
// each, legendaries among them. They announce as ONE heading: four in a row
// would bury each other and fill the queue on their own (bannerQueueMax is 4,
// so the quest banner the same clear raises would evict one unseen).
func TestLegendaryDropBannersCoalesceIntoOneHeading(t *testing.T) {
	g := bannerGame(t)
	// Short names on purpose: this test is about ONE heading, not about the
	// band-fitting rule (TestLegendaryDropBannerCountsWhatItCannotName owns that).
	for i, name := range []string{"Aegis", "Crown", "Fang", "Pearl"} {
		g.addLootBagDrop(float64(100+i*10), 100, []items.Item{{Name: name, Rarity: "legendary"}}, 0)
	}
	g.queueBanner(bannerQuestDone, "Quest complete - The Sealed Dais")
	g.tickScreenBanners()

	drops := 0
	var text string
	for _, b := range g.screenBannerQueue {
		if b.kind == bannerLegendaryDrop {
			drops++
			text = b.text
		}
	}
	if drops != 1 {
		t.Fatalf("four chests raised %d drop banners, want one: %v", drops, g.screenBannerQueue)
	}
	for _, want := range []string{"Aegis", "Crown", "Fang", "Pearl"} {
		if !strings.Contains(text, want) {
			t.Errorf("the coalesced banner %q drops %q", text, want)
		}
	}
	// And the quest banner of the same clear survived.
	found := false
	for _, b := range g.screenBannerQueue {
		if b.kind == bannerQuestDone {
			found = true
		}
	}
	if !found {
		t.Fatalf("the drops evicted the quest banner: %v", g.screenBannerQueue)
	}
}

// A haul too long for the band is counted, not cut: clipping mid-name reads as
// a rendering fault.
func TestLegendaryDropBannerCountsWhatItCannotName(t *testing.T) {
	names := []string{
		"Wyrmcleaver, the Closing Jaws", "Broodscale Aegis of the Ember Crater",
		"Crown of the Lich King Unmade", "Scalewright's Drakeforged Barding",
		"Sunspine Reliquary of the Sealed Dais",
	}
	text := legendaryDropBannerText(names, 1280)
	if !screenBannerFits(text, 1280) {
		t.Fatalf("the banner does not fit its own band: %q", text)
	}
	if !strings.Contains(text, "more") {
		t.Fatalf("a haul that cannot be named should count the rest, got %q", text)
	}
	if !strings.Contains(text, names[0]) {
		t.Fatalf("banner %q names nothing at all", text)
	}
	// A haul that DOES fit is named in full - the count is a fallback, not the rule.
	short := legendaryDropBannerText([]string{"Wyrmcleaver"}, 1280)
	if short != "Legendary drop - Wyrmcleaver" {
		t.Fatalf("single drop = %q", short)
	}
	// And a LONE name too long for the band is still named (the layout clips it):
	// counting one item says less than a truncated title.
	lone := legendaryDropBannerText(names[:1], 320)
	if !strings.Contains(lone, names[0]) {
		t.Fatalf("a single long name was replaced by a count: %q", lone)
	}
}

// A monster drop is combat information, not a visibility discovery. A DoT or an
// ally can finish a monster after the party crossed a region seam; the drop is
// still announced at the event that created it.
func TestLegendaryDropBannerIgnoresVisibilityAndRegion(t *testing.T) {
	g := bannerGame(t)
	// Two real worlds, the party standing in one of them: without a world manager
	// every map key resolves to "here" and the test would prove nothing.
	prev := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = prev })
	wm := world.NewWorldManager(g.config)
	wm.LoadedMaps = map[string]*world.World3D{
		"forest":    g.world,
		"pyramid_1": newTestWorld(g.config),
		// A MERGED region: the unified world is one World3D shared by every
		// outdoor region, so "same world" spans the whole outdoors and is NOT the
		// question the banner asks.
		"highlands": g.world,
	}
	wm.CurrentMapKey = "forest"
	world.GlobalWorldManager = wm

	g.addGroundContainer(GroundContainer{
		ID:     "far_reliquary",
		Kind:   ContainerKindLootBag,
		MapKey: "pyramid_1", // the party is not there
		X:      100,
		Y:      100,
		Items:  []items.Item{{Name: "Sunspine Reliquary", Rarity: "legendary"}},
	})
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b == nil || !strings.Contains(b.text, "Sunspine Reliquary") {
		t.Fatalf("a drop on another map raised %+v, want its legendary announced", b)
	}
	if len(g.groundContainers) != 1 {
		t.Fatalf("the container itself must still be spawned, got %d containers", len(g.groundContainers))
	}

	g.screenBannerQueue = nil

	// Same again for a region merged into the party's own world.
	g.addGroundContainer(GroundContainer{
		ID:     "merged_reliquary",
		Kind:   ContainerKindLootBag,
		MapKey: "highlands",
		X:      200,
		Y:      200,
		Items:  []items.Item{{Name: "Wyrmcleaver", Rarity: "legendary"}},
	})
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b == nil || !strings.Contains(b.text, "Wyrmcleaver") {
		t.Fatalf("a drop in a merged region raised %+v, want its legendary announced", b)
	}

	g.screenBannerQueue = nil
	// Positive control: the ordinary nearby drop follows the same path.
	g.addLootBagDrop(100, 100, []items.Item{{Name: "Broodscale Aegis", Rarity: "legendary"}}, 0)
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b == nil || !strings.Contains(b.text, "Broodscale Aegis") {
		t.Fatalf("a drop at the party's feet raised %+v", b)
	}
}

// The banner paints after the overlay pass (above the dialog's dim), so it must
// stand down for the full screens drawn BEFORE it that do not pause the world.
func TestScreenBannerYieldsToTheMapAndGameOver(t *testing.T) {
	g := bannerGame(t)
	g.queueBanner(bannerQuestPaid, "Reward claimed - Test")
	if g.visibleScreenBanner() == nil {
		t.Fatal("fixture raised no banner")
	}
	for _, tc := range []struct {
		name string
		set  func(bool)
	}{
		{"fullscreen map", func(v bool) { g.mapOverlayOpen = v }},
		{"game over", func(v bool) { g.gameOver = v }},
	} {
		tc.set(true)
		if b := g.visibleScreenBanner(); b != nil {
			t.Errorf("the banner %q paints over the %s", b.text, tc.name)
		}
		tc.set(false)
		if g.visibleScreenBanner() == nil {
			t.Errorf("the banner did not come back after the %s closed", tc.name)
		}
	}
}

// Every banner line is plain ASCII (repo rule) and fits its band.
func TestScreenBannerLinesAreAsciiAndFit(t *testing.T) {
	g := bannerGame(t)
	lines := []string{
		g.interactionPromptText(promptTestNPC(g, "Sewerman Garran")),
		"Legendary drop - Wyrmcleaver, the Closing Jaws",
	}
	for _, text := range lines {
		for _, r := range text {
			if r > 126 || r < 32 {
				t.Fatalf("banner %q carries a non-ASCII rune %q", text, r)
			}
		}
		if geo := screenBannerLayout(1280, text, 0); geo.text != text {
			t.Errorf("banner %q does not fit the band: shown as %q", text, geo.text)
		}
	}
}

// A bag belongs to the region it FELL in for rendering, pickup and persistence.
// That location does not participate in the announcement rule.
func TestLootBagBelongsToTheRegionItFellIn(t *testing.T) {
	t.Chdir("../..")
	g, wm, _ := bootOpenWorldGame(t, true)

	partyX, partyY, ok := wm.OpenWorldRegionStart("forest")
	if !ok {
		t.Fatal("fixture: the forest region has no start")
	}
	g.camera.X, g.camera.Y = partyX, partyY
	g.syncOpenWorldRegion()
	if wm.CurrentMapKey != "forest" {
		t.Fatalf("region tracking: CurrentMapKey = %q, want forest", wm.CurrentMapKey)
	}

	farX, farY, ok := wm.OpenWorldRegionStart("highlands")
	if !ok {
		t.Fatal("fixture: the highlands region has no start")
	}
	g.screenBannerQueue = nil
	g.addLootBagDrop(farX, farY, []items.Item{{Name: "Wyrmcleaver", Rarity: "legendary"}}, 0)
	if n := len(g.groundContainers); n != 1 {
		t.Fatalf("the bag did not drop (%d containers)", n)
	}
	if key := g.groundContainers[0].MapKey; key != "highlands" {
		t.Fatalf("a bag dropped in the highlands is stamped %q", key)
	}
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b == nil || !strings.Contains(b.text, "Wyrmcleaver") {
		t.Fatalf("a drop a region away raised %+v, want the drop announced immediately", b)
	}

	// Picking up an already announced bag must not announce it a second time.
	g.screenBannerQueue = nil
	g.pickupGroundContainerAt(0)
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b != nil {
		t.Fatalf("picking up the far bag announced it again: %q", b.text)
	}

	// Positive control: the same drop at the party's feet does announce.
	g.addLootBagDrop(partyX, partyY, []items.Item{{Name: "Broodscale Aegis", Rarity: "legendary"}}, 0)
	if key := g.groundContainers[0].MapKey; key != "forest" {
		t.Fatalf("a bag dropped at the party's feet is stamped %q", key)
	}
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b == nil || !strings.Contains(b.text, "Broodscale Aegis") {
		t.Fatalf("a drop at the party's feet raised %+v", b)
	}
}

// An overlay ends the approach. The nudge fires once per approach, so a player
// who opens the inventory in front of a merchant and closes it again would
// otherwise be left with no affordance at all - the persistent HUD hint this
// banner replaced was always on screen, and the only way back was to walk two
// seconds away. A DIALOG is not in this set: acting on the object settles it.
func TestInteractPromptReturnsAfterAnOverlay(t *testing.T) {
	g := bannerGame(t)
	npc := promptTestNPC(g, "Merchant")
	g.focusedNPC = npc
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b == nil || b.kind != bannerInteractPrompt {
		t.Fatalf("approach raised %+v, want the nudge", b)
	}
	// It ages out while the party keeps standing there, and stays quiet.
	for i := 0; i < g.bannerLifetime(bannerInteractPrompt)+1; i++ {
		g.tickScreenBanners()
	}
	if b := g.currentScreenBanner(); b != nil {
		t.Fatalf("the nudge repeated itself while standing still: %q", b.text)
	}

	// The character hub opens (menuOpen is the same pause contract as ESC), the
	// world stops ticking, and it closes again with focus still held.
	g.menuOpen = true
	if !g.gameplayPausedByOverlay() {
		t.Fatal("fixture: the overlay does not pause gameplay")
	}
	g.forgetInteractPromptTarget() // what the paused Update path does
	g.menuOpen = false

	g.tickScreenBanners()
	b := g.currentScreenBanner()
	if b == nil || b.kind != bannerInteractPrompt {
		t.Fatalf("after the overlay closed the nudge is %+v, want it raised again", b)
	}
	if want := g.interactionPromptText(npc); b.text != want {
		t.Fatalf("nudge = %q, want %q", b.text, want)
	}
}

// The wiring, not just the rule: the re-arm lives on GameLoop's pause path, so
// this drives the REAL frame (the same updateExploration the playthrough sim
// runs) with an overlay opening and closing in front of a live NPC.
func TestInteractPromptReturnsAfterAnOverlayOnTheRealFrame(t *testing.T) {
	t.Chdir("../..")
	g, _, cfg := bootOpenWorldGame(t, true)
	gl := g.gameLoop
	if gl == nil {
		t.Fatal("fixture has no game loop")
	}
	inn := g.townPortalAnchor("forest")
	if inn == nil {
		t.Fatal("fixture: the forest inn is gone")
	}
	x, y, ok := g.townPortalArrivalPoint("forest")
	if !ok {
		t.Fatal("no walkable tile beside the inn")
	}
	g.camera.X, g.camera.Y = x, y
	g.camera.Angle = math.Atan2(inn.Y-y, inn.X-x)
	g.syncOpenWorldRegion()
	_ = cfg

	gl.updateExploration()
	if g.focusedNPC != inn {
		t.Skipf("focus resolved to %v, not the inn - this test needs the inn in interact focus", g.focusedNPC)
	}
	if b := g.currentScreenBanner(); b == nil || b.kind != bannerInteractPrompt {
		t.Fatalf("standing at the inn raised %+v, want the nudge", b)
	}
	for i := 0; i < g.bannerLifetime(bannerInteractPrompt)+1; i++ {
		gl.updateExploration()
	}
	if b := g.currentScreenBanner(); b != nil && b.kind == bannerInteractPrompt {
		t.Fatalf("the nudge repeated itself while the party stood still: %q", b.text)
	}

	// The character hub opens for one frame and closes: the pause path must have
	// ended the approach, so the next frame nudges again.
	g.menuOpen = true
	gl.updateExploration()
	g.menuOpen = false
	gl.updateExploration()
	if b := g.currentScreenBanner(); b == nil || b.kind != bannerInteractPrompt {
		t.Fatalf("after the hub closed the nudge is %+v, want it raised again", b)
	}
}

// THE WHOLE RULE, one cell per row: monster loot is announced at the drop event
// regardless of location; sealed chest loot is announced only when opened.
func TestLegendaryAnnouncementTable(t *testing.T) {
	for _, tc := range []struct {
		name             string
		kind             ContainerKind
		mapKey           string // "" -> the party's own region
		wantOnSpawn      bool
		wantOnPickupOpen bool
	}{
		{"bag at the party's feet", ContainerKindLootBag, "", true, false},
		{"bag a region away", ContainerKindLootBag, "pyramid_1", true, false},
		{"chest at the party's feet", ContainerKindTreasureChest, "", false, true},
		{"chest a region away", ContainerKindTreasureChest, "pyramid_1", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := bannerGame(t)
			prev := world.GlobalWorldManager
			t.Cleanup(func() { world.GlobalWorldManager = prev })
			wm := world.NewWorldManager(g.config)
			wm.LoadedMaps = map[string]*world.World3D{"forest": g.world, "pyramid_1": newTestWorld(g.config)}
			wm.CurrentMapKey = "forest"
			world.GlobalWorldManager = wm

			mapKey := tc.mapKey
			if mapKey == "" {
				mapKey = "forest"
			}
			g.addGroundContainer(GroundContainer{
				Kind: tc.kind, ID: "subject", MapKey: mapKey, X: 100, Y: 100,
				Items: []items.Item{{Name: "Wyrmcleaver", Rarity: "legendary"}},
			})
			g.tickScreenBanners()
			if got := g.currentScreenBanner() != nil; got != tc.wantOnSpawn {
				t.Fatalf("announced at spawn = %v, want %v", got, tc.wantOnSpawn)
			}
			g.screenBannerQueue = nil

			g.pickupGroundContainerAt(0)
			g.tickScreenBanners()
			if got := g.currentScreenBanner() != nil; got != tc.wantOnPickupOpen {
				t.Fatalf("announced on pickup/open = %v, want %v", got, tc.wantOnPickupOpen)
			}
		})
	}
}

// Drop announcements are transient event output, not container state. Saving a
// bag after its drop must neither persist a deferred banner nor reannounce it on
// load or pickup.
func TestLegendaryDropAnnouncementDoesNotPersistOnTheBag(t *testing.T) {
	t.Chdir("../..")
	g, wm, _ := bootOpenWorldGame(t, true)

	partyX, partyY, ok := wm.OpenWorldRegionStart("forest")
	if !ok {
		t.Fatal("fixture: the forest region has no start")
	}
	g.camera.X, g.camera.Y = partyX, partyY
	g.syncOpenWorldRegion()
	farX, farY, ok := wm.OpenWorldRegionStart("highlands")
	if !ok {
		t.Fatal("fixture: the highlands region has no start")
	}

	g.groundContainers = nil
	g.addLootBagDrop(farX, farY, []items.Item{{Name: "Wyrmcleaver", Rarity: "legendary"}}, 0)
	g.addLootBagDrop(partyX, partyY, []items.Item{{Name: "Broodscale Aegis", Rarity: "legendary"}}, 0)
	g.screenBannerQueue = nil
	g.pendingLegendaryDrops = nil

	saved := g.buildSave(wm)
	g.groundContainers = nil
	if err := g.applySave(wm, &saved); err != nil {
		t.Fatalf("apply save: %v", err)
	}
	g.screenBannerQueue = nil
	g.pendingLegendaryDrops = nil
	if len(g.groundContainers) != 2 {
		t.Fatalf("%d containers survived the save, want 2", len(g.groundContainers))
	}

	if got := g.groundContainers[0].Items[0].Name; got != "Wyrmcleaver" {
		t.Fatalf("fixture: container 0 holds %q, want the bag that fell out of sight", got)
	}
	// Both bags already announced when they dropped. Restoring and picking them up
	// is silent, independent of the region stored on either container.
	g.pickupGroundContainerAt(0)
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b != nil {
		t.Fatalf("a restored bag announced on pickup: %q", b.text)
	}
	g.screenBannerQueue = nil
	if got := g.groundContainers[0].Items[0].Name; got != "Broodscale Aegis" {
		t.Fatalf("fixture: container 0 now holds %q, want the bag dropped at the feet", got)
	}
	g.pickupGroundContainerAt(0)
	g.tickScreenBanners()
	if b := g.currentScreenBanner(); b != nil {
		t.Fatalf("a bag already announced when it dropped announced again: %q", b.text)
	}
}

// THE RULE, one row per authored duration: a duration is written in SECONDS and
// converted with the live tps, so the same wall-clock time elapses at 60 and at
// 120 (the shipped build runs 120 - config.yaml, and main.go forces it on macOS).
// Written as a table because these were frame counts commented "at 60fps": every
// one of them elapsed in HALF its documented time in the shipped game, and the
// prompt grace commented "two seconds is longer than any blip" was one second.
func TestTimedDurationsAreTpsIndependent(t *testing.T) {
	cases := []struct {
		name    string
		seconds float64
		frames  func(g *MMGame) int
	}{
		{"banner slide in", bannerInSeconds, func(g *MMGame) int { return g.bannerInFrames() }},
		{"banner slide out", bannerOutSeconds, func(g *MMGame) int { return g.bannerOutFrames() }},
		{"news hold", bannerDefaultHoldSeconds, func(g *MMGame) int { return g.bannerHoldFrames(bannerQuestDone) }},
		{"nudge hold", bannerPromptHoldSeconds, func(g *MMGame) int { return g.bannerHoldFrames(bannerInteractPrompt) }},
		{"approach-over grace", bannerPromptForgetSeconds, func(g *MMGame) int {
			return g.framesForSeconds(bannerPromptForgetSeconds)
		}},
		{"badge aura period", badgeAuraPeriodSeconds, func(g *MMGame) int {
			return g.framesForSeconds(badgeAuraPeriodSeconds)
		}},
	}
	// loadTestConfig returns the SHARED config.GlobalConfig pointer, so the tick
	// rate is changed on a copy: mutating it in place leaked 60 tps into every
	// later test in the run and made the suite fail on some -shuffle orders.
	atTPS := func(tps int) *MMGame {
		base := loadTestConfig(t)
		local := *base
		local.Engine.TPS = tps
		return newTestGame(&local, newTestWorldSized(&local, 4, 4))
	}
	for _, tps := range []int{60, 120} {
		g := atTPS(tps)
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s at %d tps", tc.name, tps), func(t *testing.T) {
				frames := tc.frames(g)
				if got := float64(frames) / float64(tps); math.Abs(got-tc.seconds) > 0.02 {
					t.Fatalf("%d frames at %d tps = %.2fs, want the authored %.2fs",
						frames, tps, got, tc.seconds)
				}
			})
		}
	}
	// And the whole banner timeline follows, not just its legs.
	fast, slow := atTPS(120), atTPS(60)
	for _, kind := range []screenBannerKind{bannerQuestDone, bannerInteractPrompt} {
		fastSec := float64(fast.bannerLifetime(kind)) / 120
		slowSec := float64(slow.bannerLifetime(kind)) / 60
		if math.Abs(fastSec-slowSec) > 0.03 {
			t.Errorf("kind %d lives %.2fs at 120 tps and %.2fs at 60", kind, fastSec, slowSec)
		}
	}
}

// The shipped tick rate has ONE name. A second literal is how the code came to
// disagree with itself: config.yaml said 120, main.go pinned 120, and every
// frame constant was calibrated to 60.
func TestShippedTickRateHasOneSource(t *testing.T) {
	cfg := loadTestConfig(t)
	if got := cfg.GetTPS(); got != config.DefaultTPS {
		t.Fatalf("config.yaml ticks at %d but config.DefaultTPS is %d - the shipped rate must have one value",
			got, config.DefaultTPS)
	}
	// A config with no engine block falls back to the same rate, so a config-less
	// path never silently ticks at a different speed than the game.
	var bare config.Config
	if got := bare.GetTPS(); got != config.DefaultTPS {
		t.Fatalf("the fallback rate is %d, want the shipped %d", got, config.DefaultTPS)
	}
}

// No second answer to "what rate does this tick at". Every fallback in the code
// used to pick 60 or 120 on its own, so a config-less path computed durations at
// half or twice the shipped speed depending on which file it landed in.
func TestNoHardcodedTickRateFallbacks(t *testing.T) {
	root := "../.."
	rate := regexp.MustCompile(`\btps\s*:?=\s*(60|120)\b`)
	var offenders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		for i, line := range strings.Split(string(body), "\n") {
			if rate.MatchString(line) {
				offenders = append(offenders, fmt.Sprintf("%s:%d: %s", path, i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("hardcoded tick rates (use config.DefaultTPS):\n%s", strings.Join(offenders, "\n"))
	}
}
