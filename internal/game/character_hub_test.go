package game

import (
	"image/png"
	"math"
	"os"
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

func TestCharacterHubPauseKeepsPartyCardVisualTimersMoving(t *testing.T) {
	g, _ := newThiefTestGame(t)
	g.menuOpen = true
	g.cardFxTimers[fxBlink][0] = 2
	g.cardSummonCooldowns = map[string]int{"test-card": 2}
	g.spellInputCooldown = 2
	g.tabbedMenuInputCooldown = 2
	gl := &GameLoop{game: g, inputHandler: NewInputHandler(g)}

	gl.updateExploration()

	if got := g.cardFxTimers[fxBlink][0]; got != 1 {
		t.Fatalf("party-card visual timer = %d, want 1 while character hub is open", got)
	}
	if got := g.cardSummonCooldowns["test-card"]; got != 2 {
		t.Fatalf("gameplay cooldown = %d, want 2 while character hub is open", got)
	}
	if got := g.spellInputCooldown; got != 2 {
		t.Fatalf("gameplay input cooldown = %d, want 2 while hub is open", got)
	}
	if got := g.tabbedMenuInputCooldown; got != 1 {
		t.Fatalf("character-hub input cooldown = %d, want 1 while hub is open", got)
	}
	gl.updateExploration()
	if got := g.spellInputCooldown; got != 2 {
		t.Fatalf("gameplay input cooldown = %d, want 2 after two paused ticks", got)
	}
	if got := g.tabbedMenuInputCooldown; got != 0 {
		t.Fatalf("character-hub input cooldown = %d, want 0 after two paused ticks", got)
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
		if inventory.quickSlots.bottom() != inventory.paper.bottom() {
			t.Fatalf("%dx%d quick slots bottom=%d, paperdoll bottom=%d",
				res[0], res[1], inventory.quickSlots.bottom(), inventory.paper.bottom())
		}
		noticeBottom := inventory.camp.bottom()
		quickLabelTop := inventory.quickSlots.y - quickSlotTabLabelSpace
		if gap := quickLabelTop - noticeBottom; gap < inventoryCampToQuickLabelGap {
			t.Fatalf("%dx%d camp notice to quick-slot label gap=%d, want >=%d",
				res[0], res[1], gap, inventoryCampToQuickLabelGap)
		}
	}
}

func TestPaperdollIconSquaresCenterOnAuthoredDarkRecesses(t *testing.T) {
	file, err := os.Open("../../assets/sprites/interface/ui/inventory_paperdoll_panel.png")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	panel, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}

	// Each sample is the dark interior of one independently measured slot. The
	// production image repeats one master slot, so every opening and outer frame
	// must retain the same geometry.
	darkRecesses := []inventorySourceRect{
		{116, 142, 110, 110}, {456, 142, 110, 110}, {286, 140, 110, 110},
		{96, 406, 110, 110}, {286, 364, 110, 110}, {474, 406, 110, 110},
		{96, 576, 110, 110}, {286, 518, 110, 110}, {474, 576, 110, 110},
		{114, 746, 110, 110}, {456, 746, 110, 110}, {286, 834, 110, 110},
	}
	if len(darkRecesses) != len(inventoryPaperdollSlots) {
		t.Fatal("paperdoll recess fixture does not match slot count")
	}
	for i, slot := range inventoryPaperdollSlots {
		recess := darkRecesses[i]
		if slot.rect.w != slot.rect.h {
			t.Fatalf("slot %v icon rect = %dx%d, want a complete square", slot.slot, slot.rect.w, slot.rect.h)
		}
		if recess.w != recess.h {
			t.Fatalf("slot %v dark recess = %dx%d, want a square", slot.slot, recess.w, recess.h)
		}
		if slot.rect.w != inventoryPaperdollSlots[0].rect.w || recess.w != darkRecesses[0].w {
			t.Fatalf("slot %v geometry differs from the repeated master slot", slot.slot)
		}
		if recess.x-slot.rect.x != 4 || recess.y-slot.rect.y != 4 ||
			slot.rect.x+slot.rect.w-(recess.x+recess.w) != 4 ||
			slot.rect.y+slot.rect.h-(recess.y+recess.h) != 4 {
			t.Fatalf("slot %v dark recess is not inset 4px inside its outer frame", slot.slot)
		}
		var xSum, ySum, darkCount float64
		for y := recess.y; y < recess.y+recess.h; y++ {
			for x := recess.x; x < recess.x+recess.w; x++ {
				r, g, b, _ := panel.At(x, y).RGBA()
				luma := (3*int(r>>8) + 6*int(g>>8) + int(b>>8)) / 10
				if luma >= 40 {
					continue
				}
				xSum += float64(x) + 0.5
				ySum += float64(y) + 0.5
				darkCount++
			}
		}
		if darkCount == 0 {
			t.Fatalf("slot %v recess contains no dark pixels", slot.slot)
		}
		darkCenterX, darkCenterY := xSum/darkCount, ySum/darkCount
		iconCenterX := float64(slot.rect.x) + float64(slot.rect.w)/2
		iconCenterY := float64(slot.rect.y) + float64(slot.rect.h)/2
		if math.Abs(iconCenterX-darkCenterX) > 0.5 || math.Abs(iconCenterY-darkCenterY) > 0.5 {
			t.Fatalf("slot %v icon center (%.2f,%.2f) misses dark-pixel center (%.2f,%.2f)",
				slot.slot, iconCenterX, iconCenterY, darkCenterX, darkCenterY)
		}
	}
}

func TestPaperdollIconSquaresStayCenteredAfterScaling(t *testing.T) {
	for _, dst := range [][4]int{{17, 23, 300, 450}, {41, 67, 320, 480}, {9, 11, 405, 607}, {101, 37, 287, 430}} {
		scaleX := float64(dst[2]) / inventoryPaperdollSourceW
		scaleY := float64(dst[3]) / inventoryPaperdollSourceH
		for _, slot := range inventoryPaperdollSlots {
			x, y, size := scaleInventorySourceSquare(
				dst[0], dst[1], dst[2], dst[3],
				inventoryPaperdollSourceW, inventoryPaperdollSourceH,
				slot.rect,
			)
			wantCenterX := float64(dst[0]) + (float64(slot.rect.x)+float64(slot.rect.w)/2)*scaleX
			wantCenterY := float64(dst[1]) + (float64(slot.rect.y)+float64(slot.rect.h)/2)*scaleY
			gotCenterX := float64(x) + float64(size)/2
			gotCenterY := float64(y) + float64(size)/2
			if math.Abs(gotCenterX-wantCenterX) > 0.5 || math.Abs(gotCenterY-wantCenterY) > 0.5 {
				t.Fatalf("slot %v at %dx%d center (%.2f,%.2f), want (%.2f,%.2f)",
					slot.slot, dst[2], dst[3], gotCenterX, gotCenterY, wantCenterX, wantCenterY)
			}
		}
	}
}

func TestInventoryGridSlotsCenterOnHighResolutionRecesses(t *testing.T) {
	file, err := os.Open("../../assets/sprites/interface/ui/inventory_grid_panel.png")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	panel, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}

	bounds := panel.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		t.Fatal("inventory grid asset has empty bounds")
	}
	scaleX := float64(bounds.Dx()) / inventoryGridLayoutSize
	scaleY := float64(bounds.Dy()) / inventoryGridLayoutSize
	for i, slot := range inventoryGridSlots {
		if slot.w != slot.h {
			t.Fatalf("grid slot %d = %dx%d, want square", i, slot.w, slot.h)
		}
		x0 := bounds.Min.X + int(math.Floor(float64(slot.x)*scaleX))
		y0 := bounds.Min.Y + int(math.Floor(float64(slot.y)*scaleY))
		x1 := bounds.Min.X + int(math.Ceil(float64(slot.x+slot.w)*scaleX))
		y1 := bounds.Min.Y + int(math.Ceil(float64(slot.y+slot.h)*scaleY))

		var xSum, ySum, darkCount float64
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				r, g, b, _ := panel.At(x, y).RGBA()
				luma := (3*int(r>>8) + 6*int(g>>8) + int(b>>8)) / 10
				if luma >= 20 {
					continue
				}
				xSum += float64(x-bounds.Min.X) + 0.5
				ySum += float64(y-bounds.Min.Y) + 0.5
				darkCount++
			}
		}
		if darkCount == 0 {
			t.Fatalf("grid slot %d contains no dark recess pixels", i)
		}
		darkCenterX := xSum / darkCount / scaleX
		darkCenterY := ySum / darkCount / scaleY
		slotCenterX := float64(slot.x) + float64(slot.w)/2
		slotCenterY := float64(slot.y) + float64(slot.h)/2
		if math.Abs(slotCenterX-darkCenterX) > 0.5 || math.Abs(slotCenterY-darkCenterY) > 0.5 {
			t.Fatalf("grid slot %d center (%.2f,%.2f) misses source recess center (%.2f,%.2f)",
				i, slotCenterX, slotCenterY, darkCenterX, darkCenterY)
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
