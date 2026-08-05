package game

import (
	"testing"

	"ugataima/internal/graphics"
	"ugataima/internal/items"
	"ugataima/internal/threading"

	"github.com/hajimehoshi/ebiten/v2"
)

func firstPartyStatBadgeClick(g *MMGame, at int64) queuedClick {
	pw, ph, left, top := partyPortraitLayout(g)
	panelX, panelY, _, _ := partyCardPanelRect(left, top, pw, ph)
	badges := makePartyProgressionBadgeLayout(panelX+panelPortraitX, panelY+panelPortraitY,
		panelPortraitW, panelPortraitH, true, false)
	return queuedClick{x: badges.stat.x + badges.stat.w/2, y: badges.stat.y + badges.stat.h/2, at: at}
}

// The map item opens its overlay from INSIDE the character hub, so the overlay
// floats above a hub that stays open and its close button lands on top of the
// hub's inventory grid. Clicks must resolve top-down: the overlay claims the
// click before any hub handler can eat it.
func TestMapOverlayCloseButtonClaimsItsClickAboveTheHub(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 12, 12))
	ui := NewUISystem(g)
	g.menuOpen = true // the hub stays open behind the map
	g.mapOverlayOpen = true

	layout := computeMapOverlayLayout(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	click := queuedClick{x: layout.close.x + layout.close.w/2, y: layout.close.y + layout.close.h/2, at: 1000}
	g.mouseLeftClicks = []queuedClick{click}

	ui.handleMapOverlayInput()
	if g.mapOverlayOpen {
		t.Fatal("close button did not shut the map overlay")
	}
	if len(g.mouseLeftClicks) != 0 {
		t.Fatal("the click stayed queued - a hub handler below could still consume it")
	}

	// A click elsewhere must not CLOSE the map. It stays queued here on purpose:
	// dropping what a modal did not use is the frame-end rule in UISystem.Draw
	// (covered by TestMapOverlayCloseDrainsBufferedClicks), not this handler's job.
	g.mapOverlayOpen = true
	g.mouseLeftClicks = []queuedClick{{x: layout.body.x + 4, y: layout.body.bottom() - 4, at: 1001}}
	ui.handleMapOverlayInput()
	if !g.mapOverlayOpen {
		t.Fatal("a click outside the close button closed the overlay")
	}
	if len(g.mouseLeftClicks) != 1 {
		t.Fatal("the handler consumed a click that was not on its close button")
	}
}

// Proof the ordering matters: the overlay's close button really does overlap the
// hub's inventory grid at the shipped default resolution.
func TestMapOverlayCloseOverlapsHubInventoryGrid(t *testing.T) {
	cfg := loadTestConfig(t)
	screenW, screenH := cfg.GetScreenWidth(), cfg.GetScreenHeight()
	closeRect := computeMapOverlayLayout(screenW, screenH).close
	hub := computeTabbedMenuLayout(screenW, gameplayViewportBottomWithPartyHUD(screenH))
	grid := computeInventoryContentLayout(hub.content).grid
	overlaps := closeRect.x < grid.right() && grid.x < closeRect.right() &&
		closeRect.y < grid.bottom() && grid.y < closeRect.bottom()
	if !overlaps {
		t.Skipf("close button (%+v) no longer overlaps the inventory grid (%+v) - ordering guard is now belt-and-braces", closeRect, grid)
	}
}

// The ordering must hold through the REAL Draw sequence, not just a direct call
// to the handler: the HUD pass draws (and handles clicks) before any overlay, so
// a click under an open map must not reach a party-card badge, the auto-assign
// button or a quick slot.
func TestMapOverlayClaimsClicksAheadOfTheHudPass(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 12, 12))
	g.appScreen = AppScreenInGame
	fillTestParty(t, g)
	g.sprites = graphics.NewSpriteManager() // Draw reaches real icon lookups
	ui := NewUISystem(g)
	g.mapOverlayOpen = true

	g.showPartyStats = true // the cards (and their badges) must actually draw
	member := g.party.Members[0]
	member.FreeStatPoints = 5 // makes the stat badge (a HUD click target) appear
	before := member.FreeStatPoints

	// Aim at the first card's stat badge - a HUD target that sits under the map.
	pw, ph, left, top := partyPortraitLayout(g)
	panelX, panelY, _, _ := partyCardPanelRect(left, top, pw, ph)
	badges := makePartyProgressionBadgeLayout(panelX+panelPortraitX, panelY+panelPortraitY,
		panelPortraitW, panelPortraitH, true, false)
	badgeClick := queuedClick{x: badges.stat.x + badges.stat.w/2, y: badges.stat.y + badges.stat.h/2, at: 1000}

	screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())

	// POSITIVE CONTROL: with no modal up, that very click must reach the badge.
	// Without this the test could pass simply by never hitting anything.
	g.mapOverlayOpen = false
	g.mouseLeftClicks = []queuedClick{badgeClick}
	ui.Draw(screen)
	if !g.statPopupOpen {
		t.Fatal("control failed: the badge click never reached the HUD, so the ordering assertion below proves nothing")
	}
	g.statPopupOpen = false
	ui.justOpenedStatPopup = false

	// Now the same click under an open map must be claimed by the modal layer.
	g.mapOverlayOpen = true
	g.mouseLeftClicks = []queuedClick{badgeClick}
	ui.Draw(screen)

	if g.statPopupOpen {
		t.Fatal("a click under the open map opened the stat popup - the HUD consumed a modal's click")
	}
	if member.FreeStatPoints != before {
		t.Fatalf("stat points changed under the map: %d -> %d", before, member.FreeStatPoints)
	}
	if len(g.mouseLeftClicks) != 0 {
		t.Fatal("the modal layer left the click queued for a lower pass")
	}
	if !g.mapOverlayOpen {
		t.Fatal("the map closed on a click that was not on its close button")
	}
}

// Two presses can buffer behind a dropped frame. The one that closes the map
// must not leave a sibling queued: the interface it unblocks sits directly under
// the close button, so that leftover would fire on the hub in the same frame.
func TestMapOverlayCloseDrainsBufferedClicks(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 12, 12))
	ui := NewUISystem(g)
	g.mapOverlayOpen = true
	ui.renderedModalSnapshot = ui.topModalSnapshot()
	g.showPartyStats = true
	g.sprites = graphics.NewSpriteManager()
	fillTestParty(t, g)
	member := g.party.Members[0]
	member.FreeStatPoints = 5

	layout := computeMapOverlayLayout(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	closeClick := queuedClick{x: layout.close.x + layout.close.w/2, y: layout.close.y + layout.close.h/2, at: 1000}
	// The second press landed on a real HUD target under the overlay. If cleanup
	// waits until the end of Draw, the freshly uncovered HUD opens this popup
	// before the deferred queue drain runs.
	pw, ph, left, top := partyPortraitLayout(g)
	panelX, panelY, _, _ := partyCardPanelRect(left, top, pw, ph)
	badges := makePartyProgressionBadgeLayout(panelX+panelPortraitX, panelY+panelPortraitY,
		panelPortraitW, panelPortraitH, true, false)
	strayClick := queuedClick{x: badges.stat.x + badges.stat.w/2, y: badges.stat.y + badges.stat.h/2, at: 1001}
	g.mouseLeftClicks = []queuedClick{closeClick, strayClick}
	g.mouseRightClicks = []queuedClick{strayClick}

	ui.Draw(ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight()))

	if g.mapOverlayOpen {
		t.Fatal("close button did not shut the overlay")
	}
	if len(g.mouseLeftClicks) != 0 {
		t.Fatalf("%d left click(s) survived the close - they will hit the hub this frame", len(g.mouseLeftClicks))
	}
	if len(g.mouseRightClicks) != 0 {
		t.Fatalf("%d right click(s) survived the close", len(g.mouseRightClicks))
	}
	if g.statPopupOpen {
		t.Fatal("a buffered map click reached the HUD before deferred cleanup")
	}
}

// A quick-slot bar drawn beneath a modal layer is decoration: a right click on
// it must not rebind the Space quick spell.
func TestQuickSlotBarUnderModalIsNotInteractive(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 12, 12))
	fillTestParty(t, g)
	g.sprites = graphics.NewSpriteManager()
	ui := NewUISystem(g)

	member := g.party.Members[0]
	spell := items.Item{Name: "Fire Bolt", Type: items.ItemBattleSpell}
	member.QuickSlots[0] = &spell

	screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	bar, visible := inGameQuickSlotBarLayout(g)
	if !visible {
		t.Skip("the in-game quick bar is hidden at this resolution")
	}
	_, slots := quickSlotRects(bar.x, bar.y, bar.w)
	cell := slots[0]
	rightClick := queuedClick{x: (cell.Min.X + cell.Max.X) / 2, y: (cell.Min.Y + cell.Max.Y) / 2, at: 1000}

	// Control: with nothing modal up, that right click DOES bind the quick spell.
	g.mouseRightClicks = []queuedClick{rightClick}
	ui.drawQuickSlotBar(screen, 0, bar.x, bar.y, bar.w, true)
	if len(g.mouseRightClicks) != 0 {
		t.Fatal("control failed: an interactive bar ignored the right click")
	}
	// Snapshot AFTER the control: that binding is the expected state to hold.
	boundAfterControl, hadBinding := member.Equipment[items.SlotSpell]

	// Under a modal layer the same click must be left alone.
	g.mouseRightClicks = []queuedClick{rightClick}
	ui.drawQuickSlotBar(screen, 0, bar.x, bar.y, bar.w, false)
	if len(g.mouseRightClicks) != 1 {
		t.Fatal("a decorative bar consumed a right click belonging to the modal above it")
	}
	bound, has := member.Equipment[items.SlotSpell]
	if has != hadBinding || bound.Name != boundAfterControl.Name {
		t.Fatal("the hidden bar changed the quick spell under a modal layer")
	}
}

// The buffered-click rule must hold for EVERY modal, not just the map: lower
// layers merely refuse such a press, so it would otherwise sit in the ~500ms
// buffer and fire on the HUD the moment the modal closes. Scenario from the
// report: one click on a card badge under the stat popup, one on the popup's
// Close - the badge press must never reach the HUD afterwards.
func TestModalFrameDropsUnconsumedClicksOnClose(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 12, 12))
	fillTestParty(t, g)
	g.sprites = graphics.NewSpriteManager()
	ui := NewUISystem(g)
	g.showPartyStats = true

	member := g.party.Members[0]
	member.FreeStatPoints = 5
	before := member.FreeStatPoints

	pw, ph, left, top := partyPortraitLayout(g)
	panelX, panelY, _, _ := partyCardPanelRect(left, top, pw, ph)
	badges := makePartyProgressionBadgeLayout(panelX+panelPortraitX, panelY+panelPortraitY,
		panelPortraitW, panelPortraitH, true, false)
	badgeClick := queuedClick{x: badges.stat.x + badges.stat.w/2, y: badges.stat.y + badges.stat.h/2, at: 1000}

	screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())

	// CONTROL: the badge click reaches the HUD when nothing modal is up.
	g.mouseLeftClicks = []queuedClick{badgeClick}
	ui.Draw(screen)
	if !g.statPopupOpen {
		t.Fatal("control failed: the badge click never reached the HUD")
	}
	ui.justOpenedStatPopup = false

	// The stat popup is now the modal layer. Queue the badge press (which the
	// gated HUD refuses) plus a press on the popup's own Close button.
	popupW, popupH := 340, 320 // drawStatDistributionPopup's own size
	popupX := (cfg.GetScreenWidth() - popupW) / 2
	popupY := (cfg.GetScreenHeight() - popupH) / 2
	closeClick := queuedClick{x: popupX + popupW - 40 + 14, y: popupY + 12 + 14, at: 1001}
	g.mouseLeftClicks = []queuedClick{badgeClick, closeClick}

	ui.Draw(screen)
	if len(g.mouseLeftClicks) != 0 {
		t.Fatalf("%d click(s) survived a modal frame - they will fire on the HUD next frame", len(g.mouseLeftClicks))
	}

	// Next frame with no modal: the stale press must not act.
	statPopupWasOpen := g.statPopupOpen
	ui.Draw(screen)
	if member.FreeStatPoints != before {
		t.Fatalf("a buffered click spent stat points after the modal closed: %d -> %d", before, member.FreeStatPoints)
	}
	if !statPopupWasOpen && g.statPopupOpen {
		t.Fatal("a buffered click reopened the stat popup after it closed")
	}
}

// A modal that OPENS during a frame must not inherit clicks queued before it
// existed. The stat popup has its own justOpenedStatPopup guard, so the case is
// proven on the MAP: a double click on a quick slot opens it mid-frame while a
// third press is already buffered over where its close button appears.
func TestModalOpenedMidFrameIgnoresOlderQueuedClicks(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 12, 12))
	fillTestParty(t, g)
	g.sprites = graphics.NewSpriteManager()
	ui := NewUISystem(g)
	g.showPartyStats = true

	mapItem := items.Item{Name: "Old Map", Type: items.ItemQuest, Attributes: map[string]int{"opens_map": 1}}
	g.party.Members[g.selectedChar].QuickSlots[0] = &mapItem

	bar, visible := inGameQuickSlotBarLayout(g)
	if !visible {
		t.Skip("the in-game quick bar is hidden at this resolution")
	}
	_, slots := quickSlotRects(bar.x, bar.y, bar.w)
	cell := slots[0]
	slotClick := func(at int64) queuedClick {
		return queuedClick{x: (cell.Min.X + cell.Max.X) / 2, y: (cell.Min.Y + cell.Max.Y) / 2, at: at}
	}
	closeRect := computeMapOverlayLayout(cfg.GetScreenWidth(), cfg.GetScreenHeight()).close
	stale := queuedClick{x: closeRect.x + closeRect.w/2, y: closeRect.y + closeRect.h/2, at: 1060}

	screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	// One cell consumes one click per frame, so the double click spans two frames.
	g.mouseLeftClicks = []queuedClick{slotClick(1000)}
	ui.Draw(screen)
	if g.mapOverlayOpen {
		t.Fatal("a single click on the quick slot already opened the map")
	}
	// Second press completes the double click; the third was already buffered
	// behind it, aimed at where the map's close button is about to appear.
	g.mouseLeftClicks = []queuedClick{slotClick(1050), stale}
	ui.Draw(screen)
	if !g.mapOverlayOpen {
		t.Fatal("control failed: the double click did not open the map from the quick slot")
	}
	if len(g.mouseLeftClicks) != 0 {
		t.Fatalf("%d stale click(s) survived into the freshly opened modal", len(g.mouseLeftClicks))
	}

	// Next frame: the map must still be open - the stale press was never its input.
	ui.Draw(screen)
	if !g.mapOverlayOpen {
		t.Fatal("a click queued before the map opened closed it on the next frame")
	}
}

// The other birthplace of a modal is the INVENTORY inside the character hub:
// using the map item there opens the overlay mid-frame, from a pass that runs
// after the HUD checkpoint. A press already buffered over the overlay's close
// button must not become its input (AGENTS.md: inventory mutations respect the
// modal flow).
func TestModalOpenedFromInventoryIgnoresOlderQueuedClicks(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")
	g := newTestGame(cfg, newTestWorldSized(cfg, 12, 12))
	g.appScreen = AppScreenInGame
	fillTestParty(t, g)
	g.sprites = graphics.NewSpriteManager()
	ui := NewUISystem(g)
	g.menuOpen = true // the hub is open on the inventory tab
	g.currentTab = TabInventory
	g.party.Inventory = []items.Item{
		{Name: "World Map", Type: items.ItemQuest, Attributes: map[string]int{"opens_map": 1}},
	}

	hub := computeTabbedMenuLayout(cfg.GetScreenWidth(), gameplayViewportBottomWithPartyHUD(cfg.GetScreenHeight()))
	inv := computeInventoryContentLayout(hub.content)
	x, y, w, h := scaleInventorySourceRect(inv.grid.x, inv.grid.y, inv.grid.w, inv.grid.w,
		inventoryGridLayoutSize, inventoryGridLayoutSize, inventoryGridSlots[0])
	itemClick := func(at int64) queuedClick { return queuedClick{x: x + w/2, y: y + h/2, at: at} }
	closeRect := computeMapOverlayLayout(cfg.GetScreenWidth(), cfg.GetScreenHeight()).close
	stale := queuedClick{x: closeRect.x + closeRect.w/2, y: closeRect.y + closeRect.h/2, at: 1060}

	screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	g.mouseLeftClicks = []queuedClick{itemClick(1000)}
	ui.Draw(screen)
	if g.mapOverlayOpen {
		t.Fatal("a single click on the inventory item already opened the map")
	}

	// Second press completes the double click that uses the item; the third was
	// already buffered behind it.
	g.mouseLeftClicks = []queuedClick{itemClick(1050), stale}
	ui.Draw(screen)
	if !g.mapOverlayOpen {
		t.Fatal("control failed: double-clicking the map item did not open the overlay")
	}
	if len(g.mouseLeftClicks) != 0 {
		t.Fatalf("%d stale click(s) survived into the modal opened from the inventory", len(g.mouseLeftClicks))
	}

	ui.Draw(screen)
	if !g.mapOverlayOpen {
		t.Fatal("a click queued before the map opened closed it on the next frame")
	}
}

// The map opened from the hub's inventory is painted by the SAME overlay pass
// that opened it: that frame is fresh, not stale. The post-pass re-mark must
// record it as rendered, or the next Update would arm the redraw barrier and
// eat the player's first quick click on the already-visible map.
func TestMapOpenedFromInventoryCountsAsRenderedSameFrame(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")
	g := newTestGame(cfg, newTestWorldSized(cfg, 12, 12))
	g.appScreen = AppScreenInGame
	fillTestParty(t, g)
	g.sprites = graphics.NewSpriteManager()
	ui := NewUISystem(g)
	g.menuOpen = true
	g.currentTab = TabInventory
	g.party.Inventory = []items.Item{
		{Name: "World Map", Type: items.ItemQuest, Attributes: map[string]int{"opens_map": 1}},
	}

	hub := computeTabbedMenuLayout(cfg.GetScreenWidth(), gameplayViewportBottomWithPartyHUD(cfg.GetScreenHeight()))
	inv := computeInventoryContentLayout(hub.content)
	x, y, w, h := scaleInventorySourceRect(inv.grid.x, inv.grid.y, inv.grid.w, inv.grid.w,
		inventoryGridLayoutSize, inventoryGridLayoutSize, inventoryGridSlots[0])
	itemClick := func(at int64) queuedClick { return queuedClick{x: x + w/2, y: y + h/2, at: at} }

	screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	g.mouseLeftClicks = []queuedClick{itemClick(1000)}
	ui.Draw(screen)
	g.mouseLeftClicks = []queuedClick{itemClick(1050)}
	ui.Draw(screen)
	if !g.mapOverlayOpen {
		t.Fatal("staging broke: the double click did not open the map from the inventory")
	}

	if ui.modalRedrawBarrierActive() {
		t.Fatal("the freshly painted map counts as stale - the next Update will eat the player's first click")
	}

	// End-to-end: the very next click on the close button must work.
	closeRect := computeMapOverlayLayout(cfg.GetScreenWidth(), cfg.GetScreenHeight()).close
	g.mouseLeftClicks = []queuedClick{{x: closeRect.x + closeRect.w/2, y: closeRect.y + closeRect.h/2, at: 1100}}
	ui.Draw(screen)
	if g.mapOverlayOpen {
		t.Fatal("the first quick click on the just-opened map did not close it")
	}
}

// A modal closed in Update is still the last image presented until Draw. Ebiten
// may run more Updates first, so the redraw barrier must discard their clicks
// and keep lower input paused until a clean replacement frame lands.
func TestClosedRenderedModalBlocksUpdatesUntilReplacementDraw(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")
	g := newTestGame(cfg, newTestWorldSized(cfg, 12, 12))
	g.appScreen = AppScreenInGame
	fillTestParty(t, g)
	g.showPartyStats = true
	g.sprites = graphics.NewSpriteManager()
	g.threading = threading.NewThreadingComponents(cfg)
	t.Cleanup(g.threading.Shutdown)

	ui := NewUISystem(g)
	loop := &GameLoop{game: g, inputHandler: NewInputHandler(g), ui: ui}
	g.gameLoop = loop
	ui.renderedModalSnapshot = modalLayerSnapshot{layer: modalLayerStat} // the model closed it, but Draw has not replaced it
	g.mouseLeftClicks = []queuedClick{firstPartyStatBadgeClick(g, 1000)}

	if err := loop.Update(); err != nil {
		t.Fatalf("barrier update: %v", err)
	}
	if len(g.mouseLeftClicks) != 0 {
		t.Fatal("a click survived while the previously rendered modal was still visible")
	}
	if g.statPopupOpen {
		t.Fatal("lower HUD input ran before the modal frame was replaced")
	}
	if !ui.modalRedrawBarrierActive() {
		t.Fatal("Update cleared the redraw barrier before a replacement Draw")
	}

	ui.Draw(ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight()))
	if ui.renderedModalSnapshot.layer != modalLayerNone || ui.modalRedrawBarrierActive() {
		t.Fatal("a clean Draw did not release the modal redraw barrier")
	}
}

// HandleInput can close a modal after GameLoop.Update's early redraw-barrier
// check. The second checkpoint inside updateExploration must still allow
// party-card animation while refusing the world tick until Draw replaces the
// stale modal image.
func TestModalClosedDuringInputDoesNotAdvanceWorldBeforeDraw(t *testing.T) {
	g, _ := newThiefTestGame(t)
	g.appScreen = AppScreenInGame
	ui := NewUISystem(g)
	g.combatLogOpen = true
	ui.renderedModalSnapshot = ui.topModalSnapshot()
	g.cardFxTimers[fxBlink][0] = 2
	g.cardSummonCDFrames = 2
	x, y, w, _ := combatLogPanelLayout(g)
	g.mouseLeftClicks = []queuedClick{{x: x + w - 20, y: y + 18, at: 1000}}

	loop := &GameLoop{game: g, inputHandler: NewInputHandler(g), ui: ui}
	loop.updateExploration()

	if g.combatLogOpen {
		t.Fatal("control failed: HandleInput did not close the combat log")
	}
	if got := g.cardFxTimers[fxBlink][0]; got != 1 {
		t.Fatalf("party-card visual timer = %d, want 1 behind the redraw barrier", got)
	}
	if got := g.cardSummonCDFrames; got != 2 {
		t.Fatalf("world cooldown = %d, want 2 until Draw replaces the modal", got)
	}
}

// The fullscreen character hub leaves the party strip exposed. Its stat/skill
// badges and auto button remain real controls; only HUD elements covered by the
// hub use the broader hudClicksBlocked gate.
func TestCharacterHubKeepsPartyProgressionBadgesInteractive(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 12, 12))
	fillTestParty(t, g)
	g.showPartyStats = true
	g.menuOpen = true
	g.sprites = graphics.NewSpriteManager()
	ui := NewUISystem(g)
	g.party.Members[0].FreeStatPoints = 5

	if ui.partyCardClicksBlocked() {
		t.Fatal("the character hub incorrectly blocks its exposed party-card controls")
	}
	if !ui.hudClicksBlocked() {
		t.Fatal("the character hub no longer blocks HUD controls covered by its panel")
	}

	g.mouseLeftClicks = []queuedClick{firstPartyStatBadgeClick(g, 1000)}
	NewInputHandler(g).handlePartyPortraitMouseInput(false)
	if len(g.mouseLeftClicks) != 1 {
		t.Fatal("portrait selection consumed the progression badge click")
	}
	ui.drawPartyUI(ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight()))
	if !g.statPopupOpen || g.statPopupCharIdx != 0 {
		t.Fatal("the visible stat badge did not open its popup from the character hub")
	}
}

func TestModalBlocksSpellbookContentInput(t *testing.T) {
	g, _, caster, selectedSchool, _ := setupSorcererFireboltSelection(t)
	ui := NewUISystem(g)
	schools := spellbookSchoolsWithSpells(caster)
	if len(schools) < 2 {
		t.Fatal("sorcerer fixture needs at least two spellbook schools")
	}
	targetSchool := (selectedSchool + 1) % len(schools)
	bounds := layoutRect{x: 10, y: 10, w: 40, h: 40}
	click := queuedClick{x: 20, y: 20, at: 1000}

	g.mapOverlayOpen = true
	g.mouseLeftClicks = []queuedClick{click}
	ui.handleSpellbookSchoolClick(bounds, targetSchool, schools[targetSchool])
	if g.selectedSchool != selectedSchool {
		t.Fatalf("spellbook changed school under a modal: %d -> %d", selectedSchool, g.selectedSchool)
	}
	if len(g.mouseLeftClicks) != 1 {
		t.Fatal("spellbook consumed a click owned by the modal above it")
	}

	// Positive control: the same handler and hit rectangle remain interactive
	// as soon as no modal owns the character hub.
	g.mapOverlayOpen = false
	ui.handleSpellbookSchoolClick(bounds, targetSchool, schools[targetSchool])
	if g.selectedSchool != targetSchool || len(g.mouseLeftClicks) != 0 {
		t.Fatal("spellbook did not consume the click after the modal closed")
	}
}

func TestModalBlocksInventoryContextMutation(t *testing.T) {
	g, _ := newThiefTestGame(t)
	ui := NewUISystem(g)
	g.party.Inventory = []items.Item{{Name: "Gem", Type: items.ItemTrinket}}
	ui.inventoryContextOpen = true
	ui.inventoryContextIndex = 0
	ui.inventoryContextX = 20
	ui.inventoryContextY = 20
	click := queuedClick{x: 30, y: 30, at: 1000}
	screen := ebiten.NewImage(g.config.GetScreenWidth(), g.config.GetScreenHeight())

	g.statPopupOpen = true
	g.mouseLeftClicks = []queuedClick{click}
	ui.drawInventoryContextMenu(screen)
	if len(g.party.Inventory) != 1 {
		t.Fatal("inventory context menu discarded an item under a modal")
	}
	if !ui.inventoryContextOpen || len(g.mouseLeftClicks) != 1 {
		t.Fatal("blocked inventory context menu consumed the modal's click")
	}

	// Positive control: without the modal the same click discards the item.
	g.statPopupOpen = false
	ui.drawInventoryContextMenu(screen)
	if len(g.party.Inventory) != 0 || ui.inventoryContextOpen || len(g.mouseLeftClicks) != 0 {
		t.Fatal("inventory context action stayed blocked after the modal closed")
	}
}
