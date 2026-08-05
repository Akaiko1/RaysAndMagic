package game

import (
	"image/color"
	"time"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// doubleClickWindowMs deliberately stays short: a second click after a pause
// is another selection, not an action. Every mutating double-click handler
// resets its tracker after acting so a third click cannot spill onto a row that
// moved underneath the cursor.
const doubleClickWindowMs = 500

func withinDoubleClickWindow(current, previous int64) bool {
	return previous > 0 && current >= previous && current-previous <= doubleClickWindowMs
}

// UI Dimension constants
const (
	UIRowHeight  = 20
	UIRowSpacing = 25
)

// UISystem handles all user interface rendering and logic
type UISystem struct {
	game                *MMGame
	justOpenedStatPopup bool
	// renderedModalSnapshot is the complete top-modal state in the last completed
	// Draw. Comparing it with topModalSnapshot catches layer changes and visible
	// content replacement while Ebiten runs Updates before the new frame lands.
	renderedModalSnapshot modalLayerSnapshot
	// cardPortraitCache holds cover-fitted portraits, including the exact party
	// card aperture mask where requested; keyed by name, size, and mask mode.
	cardPortraitCache    map[string]*ebiten.Image
	partyCardEffectLayer [4]*ebiten.Image
	// statHoldStat is the name of the stat whose +button the user is
	// currently holding the mouse on. Empty means no hold in progress.
	// Single clicks go through consumeLeftClickIn; hold-to-repeat kicks in
	// after statHoldInitialDelay frames and fires every statHoldRepeatRate
	// frames thereafter so points pour out at a controllable rate.
	statHoldStat          string
	statHoldFrames        int
	lastClickTime         time.Time
	lastClickedItem       int
	inventoryContextOpen  bool
	inventoryContextX     int
	inventoryContextY     int
	inventoryContextIndex int
	stackSplitPicker      stackSplitPickerState
	inventoryPage         int    // current inventory grid page (0-based)
	inventoryTab          int    // active inventory category filter (index into inventoryTabs)
	questPage             int    // current quest log page (0-based)
	spellPage             int    // current spell/trap book spread (0-based)
	campNotice            string // result line under the Camp button
	campNoticeOK          bool   // colors the notice green (rested) or red (refused)
	lastEquipClickTime    time.Time
	lastClickedSlot       items.EquipSlot
	lastTrapClickTime     int64
	lastClickedTrap       int
	hubInteractionOpen    bool
	hubInteractionChar    int
	hubInteractionTab     MenuTab
	tooltipLines          []string
	tooltipColors         []color.Color
	tooltipIcon           string
	tooltipX              int
	tooltipY              int
	tooltipCompareLines   []string
	tooltipCompareColors  []color.Color
	tooltipTitleColor     color.Color // nameplate base behind the main tooltip's first line (nil = none)
	tooltipTitleText      color.Color // name-text color over the plate (nil = plain white)
	tooltipCompareTitle   color.Color // nameplate base for the comparison card
	tooltipCompareText    color.Color // comparison name-text color (nil = plain white)
	fullArtCardKey        string      // card under the cursor this frame; SHIFT shows its full art
	// Cached radar dot images for wizard eye (avoid vector.FillCircle every frame)
	radarDotClose  *ebiten.Image // Red dot for close enemies
	radarDotMedium *ebiten.Image // Orange dot for medium distance
	radarDotFar    *ebiten.Image // Yellow dot for far enemies
	// Compass minimap tile-layer cache: the ~80 static tile visuals only change
	// when the player crosses a tile boundary (or the world swaps), so they're
	// baked into one image and blitted per frame instead of redrawing their
	// floor backgrounds and environment thumbnails every frame.
	compassTileLayer   *ebiten.Image
	compassCacheTileX  int
	compassCacheTileY  int
	compassCacheWorld  *world.World3D
	partyCooldownState map[*character.MMCharacter]partyCooldownVisualState
}

// NewUISystem creates a new UI system
func NewUISystem(game *MMGame) *UISystem {
	ui := &UISystem{
		game:               game,
		lastClickedItem:    -1,
		lastClickedSlot:    items.EquipSlot(-1),
		lastClickedTrap:    -1,
		hubInteractionChar: -1,
	}
	ui.initRadarDots()
	return ui
}

// initRadarDots creates cached circle images for wizard eye radar dots
func (ui *UISystem) initRadarDots() {
	dotSize := 6
	// Create close enemy dot (red)
	ui.radarDotClose = ebiten.NewImage(dotSize, dotSize)
	drawCircleToImage(ui.radarDotClose, dotSize, color.RGBA{255, 50, 50, 255})
	// Create medium distance dot (orange)
	ui.radarDotMedium = ebiten.NewImage(dotSize, dotSize)
	drawCircleToImage(ui.radarDotMedium, dotSize, color.RGBA{255, 150, 50, 255})
	// Create far enemy dot (yellow)
	ui.radarDotFar = ebiten.NewImage(dotSize, dotSize)
	drawCircleToImage(ui.radarDotFar, dotSize, color.RGBA{255, 255, 50, 255})
}

// drawCircleToImage draws a filled radar dot with a dark one-pixel rim.
func drawCircleToImage(img *ebiten.Image, size int, c color.RGBA) {
	cx := float64(size-1) / 2
	cy := float64(size-1) / 2
	outerR2 := (float64(size) / 2) * (float64(size) / 2)
	innerR := max(0.0, float64(size)/2-1)
	innerR2 := innerR * innerR
	for y := 0; y < size; y++ {
		dy := float64(y) - cy
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			d2 := dx*dx + dy*dy
			switch {
			case d2 <= innerR2:
				img.Set(x, y, c)
			case d2 <= outerR2:
				img.Set(x, y, color.RGBA{6, 8, 12, 245})
			}
		}
	}
}

// Draw renders all UI elements
func (ui *UISystem) Draw(screen *ebiten.Image) {
	ui.syncCharacterHubClickContext()
	ui.tooltipLines = nil
	ui.tooltipColors = nil
	ui.tooltipIcon = ""
	ui.tooltipCompareLines = nil
	ui.tooltipCompareColors = nil
	ui.tooltipTitleColor = nil
	ui.tooltipTitleText = nil
	ui.tooltipCompareTitle = nil
	ui.tooltipCompareText = nil
	ui.fullArtCardKey = ""

	// MODAL LAYERS OWN THE FRAME'S CLICKS. Three rules, all needed:
	//
	// 1. Claim first. Drawing runs bottom-up and handlers live inside the draw
	//    passes, so a click aimed at a modal would otherwise be consumed by the
	//    HUD below it (a press over the dim spending a stat point).
	// 2. Nothing survives the frame. Lower layers only REFUSE such clicks, so an
	//    unconsumed press stays in the ~500ms buffer and fires on the HUD the
	//    moment the modal closes. Whatever the modal did not take was aimed at
	//    its dim, so it is dropped at the end of the frame - measured against the
	//    state at frame START, because the modal may close during this very frame.
	// 3. A modal that OPENS or CHANGES mid-frame inherits nothing. Its own pass
	//    runs later in this same frame, so clicks queued for the prior layer must
	//    not land on a sibling or child window the player has not seen yet.
	inputLayer := ui.renderedModalSnapshot
	modalOwnedFrame := inputLayer.layer != modalLayerNone
	renderedModalThisFrame := modalLayerSnapshot{}
	claimModalChange := func() {
		if ui.claimQueueIfModalChanged(&inputLayer) {
			modalOwnedFrame = true
		}
	}
	markModalRendered := func(layer modalLayerID) {
		renderedModalThisFrame = ui.topModalSnapshot()
		renderedModalThisFrame.layer = layer
		modalOwnedFrame = true
	}

	claimModalChange()
	ui.handleModalLayerInput()
	claimModalChange()
	// A modal may have closed in Update, while its last rendered frame is still on
	// screen. Clear anything collected for that stale visual before lower Draw
	// handlers run; GameLoop.Update blocks the same interval on its side.
	if ui.modalRedrawBarrierActive() {
		ui.dropQueuedClicks()
	}
	defer func() {
		claimModalChange()
		if modalOwnedFrame || ui.topModalLayer() != modalLayerNone {
			ui.dropQueuedClicks()
		}
		// Store what was actually painted, not merely what the model says is open
		// now. A layer closed after painting itself or opened after its draw pass
		// remains behind the identity barrier until a matching frame is presented.
		ui.renderedModalSnapshot = renderedModalThisFrame
	}()

	// Draw base game UI elements
	ui.drawGameplayUI(screen)

	// Draw debug/info elements
	ui.drawDebugInfo(screen)

	// Draw Game Over overlay if active
	if ui.game.gameOver {
		markModalRendered(modalLayerGameOver)
		ui.drawGameOverOverlay(screen)
	}
	claimModalChange()

	// Draw overlay interfaces (menus and dialogs)
	if layer := ui.topModalLayer(); isOverlayModalLayer(layer) {
		markModalRendered(layer)
	}
	// A NON-overlay modal on top owns this frame's clicks, but the overlay group
	// below it (hub, dialog, map) consumes clicks inside its draw passes and its
	// dialog widgets are not individually gated (tavern cards, buff service,
	// pagers). Hide the queues for the duration of the overlay pass - the top
	// modal's own pass runs later and must still see them. Reachable: a level-up
	// choice popping from a quest turn-in over the tavern, a stack split over
	// the tavern's stash tab.
	if layer := ui.topModalLayer(); layer != modalLayerNone && !isOverlayModalLayer(layer) {
		savedLeft, savedRight := ui.game.mouseLeftClicks, ui.game.mouseRightClicks
		ui.game.mouseLeftClicks, ui.game.mouseRightClicks = nil, nil
		ui.drawOverlayInterfaces(screen)
		ui.game.mouseLeftClicks, ui.game.mouseRightClicks = savedLeft, savedRight
	} else {
		ui.drawOverlayInterfaces(screen)
	}
	// An overlay modal that OPENED inside the pass (the map from the hub's
	// inventory) was painted this frame but missed the pre-pass mark; without a
	// re-mark the next Update treats the visible frame as stale and eats a fresh
	// click. Only an identity change re-marks: sub-state mutated by the pass
	// itself (a dialog branch click) keeps the conservative pre-pass snapshot.
	if layer := ui.topModalLayer(); isOverlayModalLayer(layer) && renderedModalThisFrame.layer != layer {
		markModalRendered(layer)
	}
	claimModalChange()

	if ui.game.combatLogOpen {
		markModalRendered(modalLayerCombatLog)
		ui.drawCombatLogOverlay(screen)
	}
	claimModalChange()

	// Draw Victory overlay if active
	if ui.game.gameVictory && !ui.game.showHighScores {
		markModalRendered(modalLayerVictory)
		ui.drawVictoryOverlay(screen)
	}
	claimModalChange()

	// Draw High Scores overlay if active
	if ui.game.showHighScores {
		markModalRendered(modalLayerHighScores)
		ui.drawHighScoresOverlay(screen)
	}
	claimModalChange()

	// Draw stat distribution popup if open
	if ui.game.statPopupOpen {
		markModalRendered(modalLayerStat)
		ui.drawStatDistributionPopup(screen)
	}
	claimModalChange()

	// Draw revival picker (dead/unconscious party member chooser) if open
	if ui.game.revivalPickerOpen {
		markModalRendered(modalLayerRevival)
		ui.drawRevivalPickerPopup(screen)
	}
	claimModalChange()
	if ui.game.healPickerOpen {
		markModalRendered(modalLayerHeal)
		ui.drawHealPickerPopup(screen)
	}
	claimModalChange()
	if ui.game.townPortalPickerOpen {
		markModalRendered(modalLayerTownPortal)
		ui.drawTownPortalPickerPopup(screen)
	}
	claimModalChange()

	// Draw promotion picker (which member becomes Archmage/Lich) if open
	if ui.game.promotionPickerOpen {
		markModalRendered(modalLayerPromotion)
		ui.drawPromotionPickerPopup(screen)
	}
	claimModalChange()

	// Draw tavern roster swap screen if open
	if ui.game.rosterScreenOpen {
		markModalRendered(modalLayerRoster)
		ui.drawRosterScreen(screen)
	}
	claimModalChange()

	// Draw tavern stash screen if open
	if ui.game.stashScreenOpen {
		markModalRendered(modalLayerStash)
		ui.drawStashScreen(screen)
	}
	claimModalChange()
	if ui.stackSplitPicker.open {
		markModalRendered(modalLayerStackSplit)
		ui.drawStackSplitPicker(screen)
	}
	claimModalChange()

	// Draw level-up choice popup if pending
	if ui.game.currentLevelUpChoice() != nil {
		markModalRendered(modalLayerLevelChoice)
		ui.drawLevelUpChoicePopup(screen)
	}

	// Full card art: while SHIFT is held over a card (Cards tab, collector,
	// stash) its full art replaces the tooltip, fitted to the screen. Cards
	// without a full_art_<key> sprite simply don't respond.
	if ui.fullArtCardKey != "" && (ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)) {
		if sprite, ok := ui.game.cardFullArtSprite(ui.fullArtCardKey); ok {
			ui.drawCardFullArtOverlay(screen, sprite)
			return
		}
	}

	// Draw tooltip last so it stays above other UI. NPC dialogs (dialogActive)
	// are no longer suppressed - the spell trader UI surfaces spell details on
	// hover and that's the only path that queues a tooltip there. Other modal
	// states (stat popup, revival picker, fullscreen map) still suppress.
	if ui.tooltipLines != nil && !ui.game.statPopupOpen && !ui.game.revivalPickerOpen && !ui.game.healPickerOpen && !ui.game.mapOverlayOpen && !ui.game.combatLogOpen && !ui.stackSplitPicker.open {
		screenW := screen.Bounds().Dx()
		screenH := screen.Bounds().Dy()
		hasIcon := ui.tooltipIcon != ""

		if ui.tooltipCompareLines == nil {
			_, mainH := tooltipBoxSizeForScreen(ui.tooltipLines, ui.tooltipColors, hasIcon, ui.tooltipX, screenW)
			y := flipTooltipY(ui.tooltipY, mainH, screenH)
			drawTooltip(screen, ui.tooltipLines, ui.tooltipColors, ui.tooltipTitleColor, ui.tooltipTitleText, ui.tooltipIcon, ui.tooltipX, y, screenW, ui.game.sprites)
		} else {
			// Two cards side by side. Cap EACH to ~half the screen (word-wrapped) so
			// the pair always fits, then place the comparison flush to the right of
			// the main and shift the pair left to stay on screen. Sizing and drawing
			// use the same column width (cardCap) so the measured and painted boxes
			// match; the flip is resolved once against the taller card so they share
			// a top edge.
			gap := tooltipCompareGap
			cardCap := screenW/2 - gap
			mainW, mainH := tooltipBoxSizeForScreen(ui.tooltipLines, ui.tooltipColors, hasIcon, 0, cardCap)
			compareW, compareH := tooltipBoxSizeForScreen(ui.tooltipCompareLines, ui.tooltipCompareColors, false, 0, cardCap)
			h := mainH
			if compareH > h {
				h = compareH
			}
			y := flipTooltipY(ui.tooltipY, h, screenH)

			mainX, compareX := tooltipPairX(ui.tooltipX, mainW, compareW, gap, screenW)
			drawTooltip(screen, ui.tooltipLines, ui.tooltipColors, ui.tooltipTitleColor, ui.tooltipTitleText, ui.tooltipIcon, mainX, y, mainX+cardCap, ui.game.sprites)
			drawTooltip(screen, ui.tooltipCompareLines, ui.tooltipCompareColors, ui.tooltipCompareTitle, ui.tooltipCompareText, "", compareX, y, compareX+cardCap, ui.game.sprites)
		}
	}
}
