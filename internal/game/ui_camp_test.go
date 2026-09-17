package game

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/items"
	"ugataima/internal/monster"
)

var campHUDResolutions = [][2]int{{800, 600}, {1024, 768}, {1280, 720}, {1280, 800}, {1366, 768}, {1440, 900}, {1600, 900}, {1680, 1050}, {1920, 1080}, {1920, 1200}, {2560, 1440}, {3440, 1440}, {3840, 2160}}

func TestCampHUDLayout(t *testing.T) {
	for _, res := range campHUDResolutions {
		for _, quick := range []bool{false, true} {
			t.Run(fmt.Sprintf("%dx%d/quick=%v", res[0], res[1], quick), func(t *testing.T) {
				g, ch := newThiefTestGame(t)
				g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = res[0], res[1]
				ch.QuickSlots = [character.QuickSlotCount]*items.Item{}
				if quick {
					ch.QuickSlots[0] = &items.Item{Name: "Potion"}
				}
				l, visible := inGameActionBarLayout(g)
				if !visible || l.hasQuick != quick {
					t.Fatal("camp availability depends on quick-slot contents")
				}
				_, _, _, partyY := partyPortraitLayout(g)
				viewport := uiBox{"viewport", 0, 0, res[0], partyY}
				if !viewport.contains(namedLayoutBox("actions", l.bounds)) || l.camp.w < 48 || l.camp.w != l.camp.h {
					t.Fatalf("unreadable/out-of-bounds camp: %+v", l)
				}
				if quick && namedLayoutBox("camp", l.camp).overlaps(namedLayoutBox("slots", l.quick)) {
					t.Fatal("camp covers quick slots")
				}
				for _, lines := range []int{1, 4} {
					x, y, w, h := g.hudMessageBlockRect(lines)
					if (uiBox{"chat", x, y, w, h}).overlaps(namedLayoutBox("actions", l.bounds)) || y < 0 {
						t.Fatal("combat log overlaps the action rail or leaves viewport")
					}
				}
				g.menuOpen = true
				if _, visible := inGameActionBarLayout(g); visible {
					t.Fatal("camp remained active under the character hub")
				}
			})
		}
	}
}

func TestCampHUDDisplayedClick(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, state := range []string{"ready", "empty", "enemy", "empty_after", "enemy_after", "no", "close", "escape", "outside", "menu"} {
			t.Run(fmt.Sprintf("tb=%v/%s", tb, state), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 800, 600)
				g := h.g
				g.menuOpen, g.turnBasedMode = false, tb
				g.party.Food = 3
				g.world.Monsters = nil
				ch := g.party.Members[g.selectedChar]
				ch.QuickSlots = [character.QuickSlotCount]*items.Item{}
				ch.HitPoints, ch.SpellPoints = 1, 0
				switch state {
				case "empty":
					g.party.Food = 0
				case "enemy":
					g.world.Monsters = []*monster.Monster3D{{ID: "camp_blocker", Name: "Goblin", X: g.camera.X + g.config.GetTileSize(), Y: g.camera.Y, HitPoints: 10, MaxHitPoints: 10}}
				case "menu":
					g.mainMenuOpen = true
				}
				foodBefore := g.party.Food
				l, _ := inGameActionBarLayout(g)
				fp := installFakePointer(t)
				fp.moveTo(l.camp.x+l.camp.w/2, l.camp.y+l.camp.h/2)
				fp.press()
				h.pointerStep()
				if g.party.Food != foodBefore || ch.HitPoints != 1 {
					t.Fatal("opening confirmation rested immediately")
				}
				if g.campConfirmOpen != (state != "menu") {
					t.Fatal("camp modal opened through another layer or failed to open")
				}
				// Move the still-held opening press over Yes. Neither holding nor
				// releasing that same physical click may accept the new modal.
				modal := layoutCampConfirmation(800, 600)
				fp.moveTo(modal.yes.x+modal.yes.w/2, modal.yes.y+modal.yes.h/2)
				fp.hold()
				h.pointerStep()
				h.pointerStep()
				fp.release()
				h.pointerStep()
				fp.idle()
				if g.party.Food != foodBefore || ch.HitPoints != 1 {
					t.Fatal("opening gesture clicked through to Yes")
				}
				if state == "menu" {
					return
				}
				if !g.gameplayPausedByOverlay() || g.worldClickAllowed() {
					t.Fatal("camp modal did not own/pause gameplay")
				}
				switch state {
				case "empty_after":
					g.party.Food = 0
					foodBefore = 0
				case "enemy_after":
					g.world.Monsters = []*monster.Monster3D{{ID: "camp_blocker", Name: "Goblin", X: g.camera.X + g.config.GetTileSize(), Y: g.camera.Y, HitPoints: 10, MaxHitPoints: 10}}
				}
				switch state {
				case "no":
					h.clicks(false, modal.no.x+modal.no.w/2, modal.no.y+modal.no.h/2, 2)
				case "close":
					h.clicks(false, modal.close.x+modal.close.w/2, modal.close.y+modal.close.h/2, 2)
				case "escape":
					h.loop.inputHandler.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == ebiten.KeyEscape })
					h.pointerStep()
				case "outside":
					h.clicks(false, 10, 10, 2)
				default:
					h.clicks(false, modal.yes.x+modal.yes.w/2, modal.yes.y+modal.yes.h/2, 2)
				}
				if g.campConfirmOpen != (state == "outside") {
					t.Fatal("confirmation did not close, or outside click accepted/dismissed it")
				}
				want := foodBefore
				if state == "ready" {
					want -= CampFoodCost
					if ch.HitPoints != ch.MaxHitPoints || ch.SpellPoints != ch.MaxSpellPoints {
						t.Fatal("camp click did not reach restParty")
					}
				}
				if g.party.Food != want {
					t.Fatalf("press/hold/release food=%d want=%d", g.party.Food, want)
				}
				if state != "ready" && ch.HitPoints != 1 {
					t.Fatal("cancelled/denied rest healed party")
				}
				if state != "outside" && !h.ui.modalRedrawBarrierActive() {
					t.Fatal("closing the modal did not protect the stale displayed frame")
				}
			})
		}
	}
}

func TestAuthoredRationPrice(t *testing.T) {
	t.Chdir("../..")
	g, _, _ := bootOpenWorldGame(t, false)
	var food *character.NPCDialogueChoice
	for _, choice := range character.NPCConfigInstance.NPCs["tavern"].Dialogue.Choices {
		if choice.Action == "buy_food" {
			food = choice
		}
	}
	if food == nil || food.Amount != 5 || food.Cost != 250 {
		t.Fatalf("authored food offer: %+v", food)
	}
	g.party.Gold, g.party.Food = 250, 0
	(&InputHandler{game: g}).handleBuyFood(food)
	if g.party.Gold != 0 || g.party.Food != 5 {
		t.Fatalf("ration purchase: gold=%d food=%d", g.party.Gold, g.party.Food)
	}
}

// Case table: confirmation open / rest animation active x load / new game /
// title. Every transition must retire the transient state and reject a stale
// confirmation without spending food or starting another rest.
func TestCampPresentationRetiresWithTimeline(t *testing.T) {
	t.Chdir("../..")
	for _, state := range []string{"confirmation", "animation"} {
		for _, action := range []string{"load", "new game", "title"} {
			t.Run(state+"/"+action, func(t *testing.T) {
				g, wm, cfg := bootOpenWorldGame(t, false)
				save := g.buildSave(wm)
				if state == "confirmation" {
					g.campConfirmOpen = true
				} else {
					g.beginCampRest()
				}
				if g.campConfirmOpen != (state == "confirmation") || (g.campRest != nil) != (state == "animation") {
					t.Fatal("fixture did not establish the requested camp state")
				}
				switch action {
				case "load":
					if err := g.applySave(wm, &save); err != nil {
						t.Fatal(err)
					}
				case "new game":
					g.startNewGameWithParty(character.NewParty(cfg))
				case "title":
					g.returnToMainMenu()
				}
				if g.campConfirmOpen || g.campRest != nil {
					t.Fatal("pending camp carried into another timeline/screen")
				}
				food := g.party.Food
				g.resolveCampConfirmation(true)
				if g.party.Food != food || g.campRest != nil || g.campConfirmOpen {
					t.Fatal("stale confirmation spent food or restarted camping")
				}
			})
		}
	}
}
