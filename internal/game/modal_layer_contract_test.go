package game

import (
	"fmt"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/graphics"
	"ugataima/internal/items"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestTopModalLayerIdentifiesEveryLayer(t *testing.T) {
	cfg := loadTestConfig(t)
	tests := []struct {
		name string
		want modalLayerID
		set  func(*MMGame, *UISystem)
		// pauses is the world-clock answer for this layer. Same table as the
		// identity on purpose: the completeness check below then forces every new
		// layer to declare BOTH, which is what the victory screen was missing -
		// it was drawn, routed input, and left monsters walking behind it.
		pauses bool
	}{
		{name: "none", want: modalLayerNone, set: func(*MMGame, *UISystem) {}, pauses: false},
		{name: "game over", want: modalLayerGameOver, set: func(g *MMGame, _ *UISystem) { g.gameOver = true }, pauses: true},
		{name: "main menu", want: modalLayerMainMenu, set: func(g *MMGame, _ *UISystem) { g.mainMenuOpen = true }, pauses: true},
		{name: "save rename", want: modalLayerSaveRename, set: func(g *MMGame, _ *UISystem) {
			g.mainMenuOpen = true
			g.saveRenameOpen = true
		}, pauses: true},
		{name: "dialog", want: modalLayerDialog, set: func(g *MMGame, _ *UISystem) { g.dialogActive = true }, pauses: false},
		{name: "skill trainer", want: modalLayerSkillTrainer, set: func(g *MMGame, _ *UISystem) {
			g.dialogActive = true
			g.skillTrainerPopup = true
		}, pauses: false},
		{name: "map", want: modalLayerMap, set: func(g *MMGame, _ *UISystem) { g.mapOverlayOpen = true }, pauses: true},
		{name: "combat log", want: modalLayerCombatLog, set: func(g *MMGame, _ *UISystem) { g.combatLogOpen = true }, pauses: true},
		{name: "victory", want: modalLayerVictory, set: func(g *MMGame, _ *UISystem) { g.gameVictory = true }, pauses: true},
		{name: "high scores", want: modalLayerHighScores, set: func(g *MMGame, _ *UISystem) { g.showHighScores = true }, pauses: true},
		{name: "stat", want: modalLayerStat, set: func(g *MMGame, _ *UISystem) { g.statPopupOpen = true }, pauses: true},
		{name: "revival", want: modalLayerRevival, set: func(g *MMGame, _ *UISystem) { g.revivalPickerOpen = true }, pauses: true},
		{name: "heal", want: modalLayerHeal, set: func(g *MMGame, _ *UISystem) { g.healPickerOpen = true }, pauses: true},
		{name: "town portal", want: modalLayerTownPortal, set: func(g *MMGame, _ *UISystem) { g.townPortalPickerOpen = true }, pauses: true},
		{name: "promotion", want: modalLayerPromotion, set: func(g *MMGame, _ *UISystem) { g.promotionPickerOpen = true }, pauses: true},
		{name: "roster", want: modalLayerRoster, set: func(g *MMGame, _ *UISystem) { g.rosterScreenOpen = true }, pauses: true},
		{name: "stash", want: modalLayerStash, set: func(g *MMGame, _ *UISystem) { g.stashScreenOpen = true }, pauses: true},
		{name: "stack split", want: modalLayerStackSplit, set: func(g *MMGame, ui *UISystem) {
			g.stashScreenOpen = true
			ui.stackSplitPicker.open = true
		}, pauses: true},
		{name: "level choice", want: modalLayerLevelChoice, set: func(g *MMGame, _ *UISystem) {
			g.levelUpChoiceQueue = []levelUpChoiceRequest{{}}
			g.levelUpChoiceOpen = true
		}, pauses: true},
	}

	seen := make(map[modalLayerID]bool, len(tests))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
			ui := NewUISystem(g)
			tt.set(g, ui)
			if got := ui.topModalLayer(); got != tt.want {
				t.Fatalf("top modal = %d, want %d", got, tt.want)
			}
			if got := tt.want.pausesWorld(); got != tt.pauses {
				t.Fatalf("layer %d pausesWorld = %v, want %v", tt.want, got, tt.pauses)
			}
			// And the pause CONTRACT the loop reads must agree - the layer table is
			// only the rule if the one predicate derives from it.
			g.gameLoop = &GameLoop{game: g, ui: ui}
			if got := g.gameplayPausedByOverlay(); got != tt.pauses {
				t.Fatalf("%s: gameplayPausedByOverlay = %v, want %v", tt.name, got, tt.pauses)
			}
		})
		if seen[tt.want] {
			t.Fatalf("modal layer %d is covered more than once", tt.want)
		}
		seen[tt.want] = true
	}
	if got, want := len(seen), int(modalLayerCount); got != want {
		t.Fatalf("contract covers %d modal identities, want all %d", got, want)
	}
}

func TestTopModalLayerUsesDrawPriority(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
	ui := NewUISystem(g)

	g.gameOver = true
	g.mainMenuOpen = true
	g.dialogActive = true
	g.mapOverlayOpen = true
	g.combatLogOpen = true
	g.gameVictory = true
	g.showHighScores = true
	g.statPopupOpen = true
	g.revivalPickerOpen = true
	g.healPickerOpen = true
	g.townPortalPickerOpen = true
	g.promotionPickerOpen = true
	g.rosterScreenOpen = true
	g.stashScreenOpen = true
	ui.stackSplitPicker.open = true
	g.levelUpChoiceQueue = []levelUpChoiceRequest{{}}
	g.levelUpChoiceOpen = true

	if got := ui.topModalLayer(); got != modalLayerLevelChoice {
		t.Fatalf("top modal = %d, want last-drawn level choice %d", got, modalLayerLevelChoice)
	}
	g.levelUpChoiceOpen = false
	if got := ui.topModalLayer(); got != modalLayerStackSplit {
		t.Fatalf("top modal after level choice = %d, want stack split %d", got, modalLayerStackSplit)
	}
}

func TestModalIdentityChangeDropsQueuedClicks(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
	ui := NewUISystem(g)
	g.dialogActive = true
	inputLayer := ui.topModalSnapshot()
	g.mouseLeftClicks = []queuedClick{{x: 1, y: 2}}
	g.mouseRightClicks = []queuedClick{{x: 3, y: 4}}

	g.dialogActive = false
	g.rosterScreenOpen = true
	if !ui.claimQueueIfModalChanged(&inputLayer) {
		t.Fatal("dialog-to-roster replacement was not detected")
	}
	if inputLayer.layer != modalLayerRoster {
		t.Fatalf("input owner = %d, want roster %d", inputLayer.layer, modalLayerRoster)
	}
	if len(g.mouseLeftClicks) != 0 || len(g.mouseRightClicks) != 0 {
		t.Fatal("clicks from the prior modal survived into its replacement")
	}
}

func TestNestedModalCloseActivatesRedrawBarrier(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
	g.appScreen = AppScreenInGame
	ui := NewUISystem(g)
	g.stashScreenOpen = true
	ui.renderedModalSnapshot = modalLayerSnapshot{layer: modalLayerStackSplit}

	if !ui.modalRedrawBarrierActive() {
		t.Fatal("closing stack split onto its stash parent did not activate the identity barrier")
	}
}

func TestMainMenuContentChangeActivatesRedrawBarrier(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
	g.appScreen = AppScreenInGame
	g.mainMenuOpen = true
	g.mainMenuMode = MenuMain
	ui := NewUISystem(g)
	ui.renderedModalSnapshot = ui.topModalSnapshot()

	g.mainMenuMode = MenuLoadSelect
	if !ui.modalRedrawBarrierActive() {
		t.Fatal("Main -> Load content replacement did not activate the redraw barrier")
	}

	ui.renderedModalSnapshot = ui.topModalSnapshot()
	g.savePage++
	if !ui.modalRedrawBarrierActive() {
		t.Fatal("save-page replacement did not activate the redraw barrier")
	}
}

func TestInputDispatchUsesDrawPriority(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
	ui := NewUISystem(g)
	g.gameLoop = &GameLoop{game: g, ui: ui}
	g.gameVictory = true
	g.levelUpChoiceQueue = []levelUpChoiceRequest{{
		charIndex: 0,
		options:   []levelUpChoiceOption{{}},
	}}
	g.levelUpChoiceOpen = true
	req := g.currentLevelUpChoice()
	popupX, _, popupW, _, startY, rowH := levelUpChoiceLayout(req, cfg.GetScreenWidth(), cfg.GetScreenHeight())
	g.mouseLeftClicks = []queuedClick{{x: popupX + popupW/2, y: startY + rowH/2}}

	ih := NewInputHandler(g)
	ih.keys.BeginFrame()
	if !ih.handleTopModalInput() {
		t.Fatal("top modal did not claim input")
	}
	if len(g.levelUpChoiceQueue) != 0 {
		t.Fatal("visible level choice did not receive input above victory")
	}
	if !g.gameVictory {
		t.Fatal("input leaked to the obscured victory layer")
	}
}

// Production selection lives in selectedCharIdx (portrait clicks, arrow keys,
// the trainer popup); the snapshot must track THAT field, or switching the
// character under a trader/trainer never arms the barrier and a later Update
// buys or trains for a selection the player has not seen drawn.
func TestDialogSnapshotTracksTraderCharacterSelection(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
	g.appScreen = AppScreenInGame
	ui := NewUISystem(g)
	g.dialogActive = true
	ui.renderedModalSnapshot = ui.topModalSnapshot()

	g.selectedCharIdx = 1
	if !ui.modalRedrawBarrierActive() {
		t.Fatal("switching the dialog's selected character did not activate the redraw barrier")
	}
}

// Any transaction a modal displays must change its snapshot, or a second
// buffered click acts on contents whose replacement has not been drawn yet.
// Gold and the bag/party/reserve lengths cover buy/sell/teach/train/hire
// without per-callsite bookkeeping; the tavern's embedded stash and roster
// sub-state ride on the dialog snapshot.
func TestModalContentMutationActivatesRedrawBarrier(t *testing.T) {
	cfg := loadTestConfig(t)
	tests := []struct {
		name   string
		mutate func(*MMGame)
	}{
		{"gold spent", func(g *MMGame) { g.party.Gold -= 100 }},
		{"arena points spent", func(g *MMGame) { g.party.ArenaPoints -= 5 }},
		{"item bought", func(g *MMGame) { g.party.Inventory = append(g.party.Inventory, items.Item{Name: "Sword"}) }},
		// Length-neutral mutations: only Party.contentRev can see these.
		{"purchase merged into stack", func(g *MMGame) {
			g.party.AddItem(items.Item{Name: "Health Potion", Type: items.ItemConsumable, Quantity: 1})
		}},
		{"partial currency drain", func(g *MMGame) {
			if !g.party.RemoveItemsByName("Health Potion", 2) {
				panic("staging: currency stack missing")
			}
		}},
		{"bench swap", func(g *MMGame) {
			if !g.party.SwapActiveReserve(0, 0) {
				panic("staging: swap rejected")
			}
		}},
		{"member hired", func(g *MMGame) { g.party.Reserve = g.party.Reserve[:len(g.party.Reserve)-1] }},
		{"embedded stash tab", func(g *MMGame) { g.stashShowCards = true }},
		{"embedded stash page", func(g *MMGame) { g.stashInvPage++ }},
		{"embedded roster selection", func(g *MMGame) { g.rosterSelectedActive = 2 }},
		{"content revision", func(g *MMGame) { g.bumpModalContentRev() }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
			g.appScreen = AppScreenInGame
			fillTestParty(t, g)
			g.party.Gold = 1000
			g.party.ArenaPoints = 50
			g.party.Reserve = []*character.MMCharacter{{}}
			g.party.Inventory = []items.Item{{Name: "Health Potion", Type: items.ItemConsumable, Quantity: 5}}
			ui := NewUISystem(g)
			g.dialogActive = true
			ui.renderedModalSnapshot = ui.topModalSnapshot()

			tt.mutate(g)
			if !ui.modalRedrawBarrierActive() {
				t.Fatal("modal content mutation did not activate the redraw barrier")
			}
		})
	}
}

// MenuSettings: Down (select channel) then Right (adjust) across two catch-up
// Updates must not change a channel whose highlight has not been drawn yet.
func TestAudioSettingsSelectionActivatesRedrawBarrier(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
	g.appScreen = AppScreenInGame
	g.mainMenuOpen = true
	g.mainMenuMode = MenuSettings
	ui := NewUISystem(g)
	ui.renderedModalSnapshot = ui.topModalSnapshot()

	g.audioSettingsSelection++
	if !ui.modalRedrawBarrierActive() {
		t.Fatal("audio channel selection change did not activate the redraw barrier")
	}
}

// A drag frozen under a higher modal misses its one-tick release edge and would
// resurrect when the layer returns: stale coordinates resolve a drop the player
// never confirmed. Losing layer ownership must cancel armed/active/drop state;
// only a deliberately picked-up split fragment survives the freeze.
func TestLayerOwnershipLossCancelsTransientDrags(t *testing.T) {
	cfg := loadTestConfig(t)
	stage := func(t *testing.T) (*MMGame, *UISystem) {
		g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
		g.appScreen = AppScreenInGame
		g.statPopupOpen = true // a modal above both the hub and any stash owner
		ui := NewUISystem(g)
		ui.renderedModalSnapshot = ui.topModalSnapshot()
		return g, ui
	}

	t.Run("stash drag cancelled", func(t *testing.T) {
		g, ui := stage(t)
		g.stashDragActive, g.stashDragDrop, g.stashDragFrom = true, true, 3
		ui.updateMouseState()
		if g.stashDragActive || g.stashDragDrop || g.stashDragFrom != -1 {
			t.Fatalf("frozen stash drag survived ownership loss: active=%v drop=%v from=%d",
				g.stashDragActive, g.stashDragDrop, g.stashDragFrom)
		}
	})
	t.Run("stash fragment survives", func(t *testing.T) {
		g, ui := stage(t)
		g.stashDragActive, g.stashDragPickedUp = true, true
		ui.updateMouseState()
		if !g.stashDragPickedUp {
			t.Fatal("picked-up stash fragment was discarded by ownership loss")
		}
	})
	t.Run("quick drag cancelled", func(t *testing.T) {
		g, ui := stage(t)
		g.menuOpen = true
		g.dragArmed, g.dragActive, g.dragSrc = true, true, dragFromInventory
		ui.updateMouseState()
		if g.dragArmed || g.dragActive || g.dragSrc != dragNone {
			t.Fatalf("frozen inventory drag survived ownership loss: armed=%v active=%v src=%d",
				g.dragArmed, g.dragActive, g.dragSrc)
		}
	})
	t.Run("inventory fragment survives", func(t *testing.T) {
		g, ui := stage(t)
		g.menuOpen = true
		g.dragActive, g.dragPickedUp = true, true
		ui.updateMouseState()
		if !g.dragPickedUp {
			t.Fatal("picked-up inventory fragment was discarded by ownership loss")
		}
	})
}

// Draw-phase widgets of the overlay group (dialog pagers, tavern cards, buff
// services) are not individually gated; with a non-overlay modal on top the
// click queues are hidden from the whole overlay pass. Proven on the spell
// trader's pager under a level-up choice.
func TestOverlayWidgetsCannotEatTopModalClicks(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")
	g := newTestGame(cfg, newTestWorldSized(cfg, 12, 12))
	g.appScreen = AppScreenInGame
	fillTestParty(t, g)
	g.sprites = graphics.NewSpriteManager()
	ui := NewUISystem(g)

	npc := aldricLikeNPC()
	npc.SpellData = map[string]*character.NPCSpell{}
	for i := 0; i < spellTraderPerPage+1; i++ { // 2 pages -> the pager renders
		key := fmt.Sprintf("spell_%02d", i)
		npc.SpellData[key] = &character.NPCSpell{Name: key, Cost: 100}
	}
	g.dialogActive = true
	g.dialogNPC = npc

	dlg := npcDialogLayout(g)
	gridW := spellTraderGridCols*spellTraderIconSize + (spellTraderGridCols-1)*spellTraderIconGap
	pagerX := dlg.x + (600-gridW)/2
	pagerY := spellTraderPagerY(dlg.y)
	nextClick := queuedClick{x: pagerX + gridW - 15, y: pagerY + 9, at: 1000}

	screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())
	ui.Draw(screen) // present the dialog as the top layer

	// CONTROL: with the dialog on top its own pager click must work.
	g.mouseLeftClicks = []queuedClick{nextClick}
	ui.Draw(screen)
	if g.spellTraderPage != 1 {
		t.Fatal("control failed: the pager click never flipped the trader page")
	}
	g.spellTraderPage = 0
	ui.Draw(screen) // resync the rendered snapshot after the reset

	// A level-up choice opens on top (quest turn-in during the dialog).
	g.levelUpChoiceQueue = []levelUpChoiceRequest{{charIndex: 0, level: 2}}
	g.levelUpChoiceOpen = true
	ui.Draw(screen) // present the new top layer

	g.mouseLeftClicks = []queuedClick{nextClick}
	ui.Draw(screen)
	if g.spellTraderPage != 0 {
		t.Fatal("the dialog's pager consumed a click owned by the level-up choice above it")
	}
	if g.currentLevelUpChoice() == nil {
		t.Fatal("staging broke: the level-up choice disappeared")
	}
}

// A chest cell-to-cell stash move changes neither the bag length nor the gold;
// the journalled transfer chokepoint must bump the explicit content revision.
func TestStashTransferBumpsModalContentRevision(t *testing.T) {
	g := stashTestGame(t)
	g.appScreen = AppScreenInGame
	g.stashScreenOpen = true
	ui := NewUISystem(g)
	ui.renderedModalSnapshot = ui.topModalSnapshot()

	if !g.commitStashTransfer(g.stashSnapshot()) {
		t.Fatal("stash transfer commit failed in the isolated test dir")
	}
	if !ui.modalRedrawBarrierActive() {
		t.Fatal("a committed stash transfer did not activate the redraw barrier")
	}
}

// The ESC edge for cancellable pickers is consumed in Update: a press-and-
// release falling entirely between two Draws must still cancel. The promotion
// picker stays non-cancellable.
func TestPickerEscapeIsConsumedInUpdate(t *testing.T) {
	cfg := loadTestConfig(t)
	tests := []struct {
		name      string
		open      func(*MMGame)
		cancelled func(*MMGame) bool
	}{
		{"revival", func(g *MMGame) { g.revivalPickerOpen = true }, func(g *MMGame) bool { return !g.revivalPickerOpen }},
		{"heal", func(g *MMGame) { g.healPickerOpen = true }, func(g *MMGame) bool { return !g.healPickerOpen }},
		{"town portal", func(g *MMGame) { g.townPortalPickerOpen = true }, func(g *MMGame) bool { return !g.townPortalPickerOpen }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
			g.pickerQuickChar, g.pickerQuickSlot = -1, -1
			tt.open(g)
			ih := NewInputHandler(g)
			ih.keys = keytracker.NewWithSource(escKeyboard())
			ih.HandleInput()
			if !tt.cancelled(g) {
				t.Fatal("ESC in Update did not cancel the picker")
			}
		})
	}

	t.Run("promotion stays non-cancellable", func(t *testing.T) {
		g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
		g.promotionPickerOpen = true
		ih := NewInputHandler(g)
		ih.keys = keytracker.NewWithSource(escKeyboard())
		ih.HandleInput()
		if !g.promotionPickerOpen {
			t.Fatal("ESC cancelled the promotion picker - it must stay committed")
		}
	})
}

func TestTopLevelScreenIgnoresGameplayModalBarrier(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
	ui := NewUISystem(g)
	g.appScreen = AppScreenMainMenu
	g.dialogActive = true

	if ui.modalRedrawBarrierActive() {
		t.Fatal("a background gameplay modal blocked the entry screen")
	}
}

// The identity contract's other half: for EVERY layer a few real Draw frames
// must converge renderedModalSnapshot to the top snapshot. A layer that draws
// without marking (or marks a state it did not draw) keeps the barrier up and
// freezes Update forever. Empty pickers legitimately close themselves during
// their own pass, hence convergence within a few frames, not exactly one.
func TestEveryModalLayerReleasesRedrawBarrier(t *testing.T) {
	cfg := loadTestConfig(t)
	t.Chdir("../..")
	stages := []struct {
		name string
		set  func(*MMGame, *UISystem)
	}{
		{"game over", func(g *MMGame, _ *UISystem) { g.gameOver = true }},
		{"main menu", func(g *MMGame, _ *UISystem) { g.mainMenuOpen = true }},
		{"save rename", func(g *MMGame, _ *UISystem) {
			g.mainMenuOpen, g.saveRenameOpen = true, true
			g.mainMenuMode = MenuSaveSelect
		}},
		{"dialog", func(g *MMGame, _ *UISystem) { g.dialogActive = true }},
		{"skill trainer", func(g *MMGame, _ *UISystem) { g.dialogActive, g.skillTrainerPopup = true, true }},
		{"map", func(g *MMGame, _ *UISystem) { g.mapOverlayOpen = true }},
		{"combat log", func(g *MMGame, _ *UISystem) { g.combatLogOpen = true }},
		{"victory", func(g *MMGame, _ *UISystem) { g.gameVictory = true }},
		{"high scores", func(g *MMGame, _ *UISystem) { g.showHighScores = true }},
		{"stat", func(g *MMGame, _ *UISystem) { g.statPopupOpen = true }},
		{"revival (self-closing empty)", func(g *MMGame, _ *UISystem) { g.revivalPickerOpen = true }},
		{"heal (self-closing empty)", func(g *MMGame, _ *UISystem) { g.healPickerOpen = true }},
		{"town portal", func(g *MMGame, _ *UISystem) { g.townPortalPickerOpen = true }},
		{"promotion (self-closing empty)", func(g *MMGame, _ *UISystem) { g.promotionPickerOpen = true }},
		{"roster", func(g *MMGame, _ *UISystem) { g.rosterScreenOpen = true }},
		{"stash", func(g *MMGame, _ *UISystem) { g.stashScreenOpen = true }},
		{"stack split", func(g *MMGame, ui *UISystem) {
			g.party.Inventory = []items.Item{{Name: "Health Potion", Type: items.ItemConsumable, Quantity: 6, InstanceID: 9}}
			ui.openStackSplitPicker(stackSplitPickerInventory, 0, g.party.Inventory[0])
		}},
		{"level choice", func(g *MMGame, _ *UISystem) {
			g.levelUpChoiceQueue = []levelUpChoiceRequest{{charIndex: 0, level: 2}}
			g.levelUpChoiceOpen = true
		}},
	}

	for _, tt := range stages {
		t.Run(tt.name, func(t *testing.T) {
			g := newTestGame(cfg, newTestWorldSized(cfg, 12, 12))
			g.appScreen = AppScreenInGame
			fillTestParty(t, g)
			g.sprites = graphics.NewSpriteManager()
			ui := NewUISystem(g)
			tt.set(g, ui)

			screen := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight())
			for i := 0; i < 3; i++ {
				ui.Draw(screen)
				if !ui.modalRedrawBarrierActive() {
					return
				}
			}
			t.Fatalf("barrier never released: rendered layer %d vs top layer %d - Update is frozen forever",
				ui.renderedModalSnapshot.layer, ui.topModalLayer())
		})
	}
}

// The wiring, on the real frame: the victory summary is read for as long as the
// player likes, and the world behind it must not walk, tick day/night, or age
// cooldowns while they read it. Driven through updateExploration (the same entry
// the playthrough sim runs) rather than the predicate alone, because a predicate
// that nothing consults is not a pause.
//
// The frame is presented between ticks (presentModalFrame) exactly as Draw would.
// Without it the modal REDRAW BARRIER returns from updateExploration first, the
// world stops for a reason that has nothing to do with pausing, and the test
// passes with the pause contract removed - which is how the first version of this
// test survived its own mutation.
func TestVictoryScreenStopsTheWorldClock(t *testing.T) {
	t.Chdir("../..")
	g, wm, _ := bootOpenWorldGame(t, true)
	gl := g.gameLoop
	x, y, ok := wm.OpenWorldRegionStart("forest")
	if !ok {
		t.Fatal("fixture: the forest region has no start")
	}
	g.camera.X, g.camera.Y = x, y
	g.syncOpenWorldRegion()

	presentModalFrame := func() { gl.ui.renderedModalSnapshot = gl.ui.topModalSnapshot() }
	runFrames := func(n int) {
		for i := 0; i < n; i++ {
			presentModalFrame()
			gl.updateExploration()
		}
	}

	const frames = 30
	g.gameVictory = true
	g.spellInputCooldown = frames + 5 // a second clock, decremented once per live frame
	beforeClock, beforeCooldown := g.dayNightFrames, g.spellInputCooldown
	runFrames(frames)
	if g.dayNightFrames != beforeClock {
		t.Errorf("the day/night clock advanced %d frames behind the victory screen",
			g.dayNightFrames-beforeClock)
	}
	if g.spellInputCooldown != beforeCooldown {
		t.Errorf("the action cooldown aged %d frames behind the victory screen",
			beforeCooldown-g.spellInputCooldown)
	}

	// Positive control: the same frames with the screen closed DO advance both, or
	// the assertions above would pass on clocks that never move.
	g.gameVictory = false
	runFrames(frames)
	if g.dayNightFrames == beforeClock {
		t.Fatal("the day/night clock did not move with no overlay either - the test proves nothing")
	}
	if g.spellInputCooldown == beforeCooldown {
		t.Fatal("the cooldown did not move with no overlay either - the test proves nothing")
	}
}

// A paused overlay presents a STILL frame. The world clock is what every part of
// the world's picture is drawn from - sprite cadence, standee yaw, torch and
// firefly flicker, and the screen-shake phase, which alternates the camera
// sideways on every other frame. While that clock kept running under the victory
// screen the scene shivered in place instead of freezing.
func TestPausedOverlayFreezesTheWorldPicture(t *testing.T) {
	t.Chdir("../..")
	g, wm, _ := bootOpenWorldGame(t, true)
	gl := g.gameLoop
	x, y, ok := wm.OpenWorldRegionStart("forest")
	if !ok {
		t.Fatal("fixture: the forest region has no start")
	}
	g.camera.X, g.camera.Y = x, y
	g.syncOpenWorldRegion()

	runFrames := func(n int) {
		for i := 0; i < n; i++ {
			gl.ui.renderedModalSnapshot = gl.ui.topModalSnapshot() // what Draw presents
			gl.updateExploration()
		}
	}

	// A boss dies, the victory summary comes up with the shake still running.
	g.screenShake = 3
	g.gameVictory = true
	worldBefore, uiBefore := g.frameCount, g.uiFrameCount
	shakeBefore := g.screenShake
	runFrames(20)

	if g.frameCount != worldBefore {
		t.Errorf("the world clock advanced %d frames behind the victory screen", g.frameCount-worldBefore)
	}
	if g.screenShake != shakeBefore {
		t.Errorf("the shake decayed from %.2f to %.2f while the world was stopped", shakeBefore, g.screenShake)
	}
	// The shake phase is what shivered: the sign flips with the world clock, so a
	// frozen clock means a frozen displacement.
	if g.frameCount%2 != worldBefore%2 {
		t.Error("the shake phase moved behind the paused overlay")
	}
	// The interface clock is deliberately the exception - a hit flash or a badge
	// aura on a visible party card must not freeze under an open panel.
	if g.uiFrameCount == uiBefore {
		t.Error("the interface clock stopped too; party-card feedback would freeze")
	}

	// Positive control: with the screen closed both clocks move and the shake decays.
	g.gameVictory = false
	runFrames(20)
	if g.frameCount == worldBefore {
		t.Fatal("the world clock did not move with no overlay either - the test proves nothing")
	}
	if g.screenShake >= shakeBefore {
		t.Fatalf("the shake did not decay with the world running (%.2f)", g.screenShake)
	}
}

// The other half of the split, as an observable: party-card animation reads the
// INTERFACE clock. Asserting the two counters alone left this free - a card layer
// switched back to the world clock passed every clock test and froze on screen.
func TestPartyCardAnimationReadsTheInterfaceClock(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
	ui := NewUISystem(g)

	g.frameCount, g.uiFrameCount = 500, 900
	if got := ui.cardAnimClock(); got != g.uiFrameCount {
		t.Fatalf("card animation clock = %d, want the interface clock %d (world clock is %d)",
			got, g.uiFrameCount, g.frameCount)
	}
	// And it keeps moving on a frame the world does not: that is the whole point.
	before := ui.cardAnimClock()
	g.uiFrameCount++
	if ui.cardAnimClock() == before {
		t.Fatal("the card clock did not follow the interface clock")
	}
	if g.frameCount != 500 {
		t.Fatal("fixture moved the world clock")
	}
}

// A wipe ends the conversation with it. The ladder ranks a dialog ABOVE Game
// Over (a shop is painted later than the death screen), so a party that dies
// mid-trade would leave a non-pausing layer on top: monsters walking, the clock
// ticking and autosaves firing behind a Game Over the player cannot dismiss,
// with the shop still eating keys.
func TestAWipeClosesTheConversationAndPauses(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
	ui := NewUISystem(g)
	g.gameLoop = &GameLoop{game: g, ui: ui}

	g.dialogActive = true
	g.dialogNPC = &character.NPC{Name: "Merchant", RenderCategory: "npc"}
	g.skillTrainerPopup = true
	g.dragActive, g.dragPickedUp = true, true
	g.stashDragActive, g.stashDragPickedUp = true, true
	for _, m := range g.party.Members {
		m.HitPoints = 0
	}
	g.checkGameOver()

	if !g.gameOver {
		t.Fatal("a party with no one standing is not game over")
	}
	if g.dialogActive || g.dialogNPC != nil || g.skillTrainerPopup {
		t.Fatalf("the conversation survived the wipe (dialog=%v npc=%v trainer=%v)",
			g.dialogActive, g.dialogNPC != nil, g.skillTrainerPopup)
	}
	if g.dragPickedUp || g.stashDragPickedUp || g.stackSplitInteractionActive() {
		t.Fatalf("a carried split fragment survived the forced close: inventory=%v stash=%v",
			g.dragPickedUp, g.stashDragPickedUp)
	}
	if got := ui.topModalLayer(); got != modalLayerGameOver {
		t.Fatalf("top layer = %d, want Game Over (%d)", got, modalLayerGameOver)
	}
	if !g.gameplayPausedByOverlay() {
		t.Fatal("the world still runs behind Game Over")
	}
}
