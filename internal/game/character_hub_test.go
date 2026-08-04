package game

import (
	"testing"
	"time"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

func TestCharacterHubUsesGameplayPauseContract(t *testing.T) {
	g, _ := newThiefTestGame(t)
	if g.gameplayPausedByOverlay() {
		t.Fatal("game starts paused without an overlay")
	}
	g.menuOpen = true
	if !g.gameplayPausedByOverlay() {
		t.Fatal("open character hub did not pause gameplay")
	}
	g.menuOpen = false
	if g.gameplayPausedByOverlay() {
		t.Fatal("closing character hub left gameplay paused")
	}
}

func TestCharacterHubWorldActionClosesBeforeDispatchAndRestoresOnFailure(t *testing.T) {
	g, _ := newThiefTestGame(t)
	g.menuOpen = true
	if g.dispatchCharacterHubWorldAction(func() bool {
		if g.menuOpen {
			t.Fatal("world action observed an open character hub")
		}
		return false
	}) {
		t.Fatal("failed action reported success")
	}
	if !g.menuOpen {
		t.Fatal("failed world action did not restore the character hub")
	}
}

func TestCharacterHubKeepsPartyCardsAsMouseSelector(t *testing.T) {
	g, _ := newThiefTestGame(t)
	g.menuOpen = true
	g.showPartyStats = true
	g.selectedChar = 0
	ih := NewInputHandler(g)
	cardW, cardH, left, top := partyPortraitLayout(g)
	g.mouseLeftClicks = []queuedClick{{
		x:  left + cardW + cardW/2,
		y:  top + cardH/2,
		at: 1,
	}}
	ih.handlePartyPortraitMouseInput(false)
	if g.selectedChar != 1 {
		t.Fatalf("party-card click selected %d, want 1 while hub is open", g.selectedChar)
	}
}

func TestSpellbookCardsStayInsideInnerPageFrames(t *testing.T) {
	for _, res := range [][2]int{{1024, 768}, {1280, 720}, {1920, 1080}, {3440, 1440}} {
		menu := computeTabbedMenuLayout(res[0], gameplayViewportBottomWithPartyHUD(res[1]))
		book := computeBookLayout(menu.content)
		for i := 0; i < book.cardsPerSpread(); i++ {
			x, y := book.cardPos(i)
			card := layoutRect{x, y, book.cardW, book.cardH}
			inner := book.pageInnerRect(i / book.cardsPerPage)
			if card.x < inner.x || card.y < inner.y || card.right() > inner.right() || card.bottom() > inner.bottom() {
				t.Fatalf("%dx%d spell card %d %+v leaves inner page %+v", res[0], res[1], i, card, inner)
			}
			if i%book.cardsPerPage < book.cols && card.y-inner.y < book.srcH(20) {
				t.Fatalf("%dx%d top spell card %d starts only %dpx below the inner frame", res[0], res[1], i, card.y-inner.y)
			}
			if i%book.cardsPerPage >= book.cols && inner.bottom()-card.bottom() < book.srcH(20) {
				t.Fatalf("%dx%d bottom spell card %d ends only %dpx above the inner frame", res[0], res[1], i, inner.bottom()-card.bottom())
			}
		}
	}
}

func TestSpellbookSchoolBookmarksUseWholeSafeDrawRect(t *testing.T) {
	for _, res := range [][2]int{{1024, 768}, {1280, 720}, {1920, 1080}, {3440, 1440}} {
		menu := computeTabbedMenuLayout(res[0], gameplayViewportBottomWithPartyHUD(res[1]))
		book := computeBookLayout(menu.content)
		tabs := book.schoolTabRects(len(character.AllMagicSchools), 2)
		if len(tabs) != len(character.AllMagicSchools) {
			t.Fatalf("%dx%d created %d school tabs, want %d", res[0], res[1], len(tabs), len(character.AllMagicSchools))
		}
		for i, tab := range tabs {
			if tab.w <= 0 || tab.h <= 0 {
				t.Fatalf("%dx%d school tab %d has invalid bounds %+v", res[0], res[1], i, tab)
			}
			if tab.bottom() > book.gridY {
				t.Fatalf("%dx%d school tab %d reaches spell cards: bottom=%d grid=%d", res[0], res[1], i, tab.bottom(), book.gridY)
			}
		}
	}

	g, _, caster, _, _ := setupSorcererFireboltSelection(t)
	schools := spellbookSchoolsWithSpells(caster)
	if len(schools) < 2 {
		t.Fatal("sorcerer fixture needs at least two school bookmarks")
	}
	menu := computeTabbedMenuLayout(g.config.GetScreenWidth(), gameplayViewportBottom(g))
	book := computeBookLayout(menu.content)
	tabs := book.schoolTabRects(len(schools), g.selectedSchool)
	target := 1
	tab := tabs[target]
	g.mouseLeftClicks = []queuedClick{{x: tab.right() - 1, y: tab.bottom() - 1, at: 1000}}
	ui := NewUISystem(g)
	ui.handleSpellbookSchoolClick(tab, target, schools[target])
	if g.selectedSchool != target {
		t.Fatalf("click at bookmark bottom-right selected school %d, want %d", g.selectedSchool, target)
	}
}

func TestCharacterHubContextChangeBreaksDoubleClickChains(t *testing.T) {
	g, _ := newThiefTestGame(t)
	ui := NewUISystem(g)
	g.menuOpen = true
	g.currentTab = TabInventory
	ui.syncCharacterHubClickContext()

	ui.lastClickedItem = 3
	ui.lastClickTime = time.UnixMilli(1000)
	ui.lastClickedSlot = items.SlotMainHand
	ui.lastEquipClickTime = time.UnixMilli(1000)
	ui.lastClickedTrap = 2
	ui.lastTrapClickTime = 1000
	g.lastClickedSpell = 1
	g.lastClickedSchool = 0
	g.lastSpellClickTime = 1000

	g.selectedChar = 1
	ui.syncCharacterHubClickContext()
	if ui.lastClickedItem != -1 || !ui.lastClickTime.IsZero() ||
		ui.lastClickedSlot != items.EquipSlot(-1) || !ui.lastEquipClickTime.IsZero() ||
		ui.lastClickedTrap != -1 || ui.lastTrapClickTime != 0 ||
		g.lastClickedSpell != -1 || g.lastClickedSchool != -1 || g.lastSpellClickTime != 0 {
		t.Fatal("character switch preserved a double-click chain from the previous character")
	}
}

func TestInventoryPanelsShareTopRailAtStandardResolutions(t *testing.T) {
	for _, res := range [][2]int{{1024, 768}, {1280, 720}, {1280, 800}, {1920, 1080}, {3440, 1440}} {
		menu := computeTabbedMenuLayout(res[0], gameplayViewportBottomWithPartyHUD(res[1]))
		inventory := computeInventoryContentLayout(menu.content)
		if inventory.paper.y != inventory.grid.y {
			t.Fatalf("%dx%d paperdoll top=%d, inventory top=%d", res[0], res[1], inventory.paper.y, inventory.grid.y)
		}
	}
}

func TestTabbedMenuLabelsFitOneLineInsideTabs(t *testing.T) {
	for _, width := range []int{1024, 1280, 1920, 3440} {
		menu := computeTabbedMenuLayout(width, gameplayViewportBottomWithPartyHUD(768))
		for i, tab := range menu.tabs {
			label := tabbedMenuTabs[i].label + " " + tabbedMenuTabs[i].key
			if debugTextWidth(label) > tab.w-16 {
				t.Fatalf("width %d tab %q is %dpx inside %dpx", width, label, debugTextWidth(label), tab.w)
			}
			if debugTextCharHeight > tab.h-12 {
				t.Fatalf("tab label height %d leaves decorative rails in %dpx tab", debugTextCharHeight, tab.h)
			}
		}
	}
}

func setupSorcererFireboltSelection(t *testing.T) (*MMGame, *InputHandler, *character.MMCharacter, int, int) {
	t.Helper()
	g, _ := newThiefTestGame(t)
	caster := character.CreateCharacter("Lysander", character.ClassSorcerer, g.config)
	caster.SpellPoints = caster.MaxSpellPoints
	g.party.Members[0] = caster
	g.selectedChar = 0
	schoolIndex, spellIndex := -1, -1
	for si, school := range spellbookSchoolsWithSpells(caster) {
		for spi, spellID := range caster.GetSpellsForSchool(school) {
			if spellID == "firebolt" {
				schoolIndex, spellIndex = si, spi
			}
		}
	}
	if schoolIndex < 0 || spellIndex < 0 {
		t.Fatal("sorcerer fixture has no Firebolt")
	}
	g.selectedSchool = schoolIndex
	g.selectedSpell = spellIndex
	g.menuOpen = true
	return g, NewInputHandler(g), caster, schoolIndex, spellIndex
}

func TestSpellbookDoubleClickEquipsFastSpellWithoutCasting(t *testing.T) {
	g, _, caster, schoolIndex, spellIndex := setupSorcererFireboltSelection(t)
	ui := NewUISystem(g)

	g.mouseLeftClicks = []queuedClick{{x: 15, y: 15, at: 1000}}
	ui.handleSpellbookSpellClick(10, 10, 40, 40, schoolIndex, spellIndex)
	if !g.menuOpen {
		t.Fatal("first click cast instead of selecting")
	}
	beforeSP := caster.SpellPoints

	g.mouseLeftClicks = []queuedClick{{x: 15, y: 15, at: 1100}}
	ui.handleSpellbookSpellClick(10, 10, 40, 40, schoolIndex, spellIndex)
	if !g.menuOpen {
		t.Fatal("double-click closed the character hub")
	}
	if caster.SpellPoints != beforeSP {
		t.Fatalf("double-click cast the spell: SP %d -> %d", beforeSP, caster.SpellPoints)
	}
	equipped, ok := caster.Equipment[items.SlotSpell]
	if !ok || equipped.SpellEffect != items.SpellEffect("firebolt") {
		t.Fatalf("double-click equipped %+v, want Firebolt", equipped)
	}
}

func TestSpellbookKeyboardActionClosesHubBeforeSuccessfulCast(t *testing.T) {
	g, ih, caster, _, _ := setupSorcererFireboltSelection(t)
	beforeSP := caster.SpellPoints
	if !ih.castSelectedSpellFromHub() {
		t.Fatal("selected Firebolt did not cast")
	}
	if g.menuOpen {
		t.Fatal("successful Enter/F cast left the character hub open")
	}
	if caster.SpellPoints >= beforeSP {
		t.Fatalf("spell points did not decrease: %d -> %d", beforeSP, caster.SpellPoints)
	}
}

func TestQuickSlotUseBarIsHiddenWhileCharacterHubIsOpen(t *testing.T) {
	g, _, caster, _, _ := setupSorcererFireboltSelection(t)
	spell, err := spells.CreateSpellItem("firebolt")
	if err != nil {
		t.Fatal(err)
	}
	caster.QuickSlots[0] = &spell
	if _, visible := inGameQuickSlotBarLayout(g); visible {
		t.Fatal("in-game quick-slot use bar is visible inside the character hub")
	}
	g.menuOpen = false
	if _, visible := inGameQuickSlotBarLayout(g); !visible {
		t.Fatal("in-game quick-slot use bar did not return after closing the hub")
	}
}
