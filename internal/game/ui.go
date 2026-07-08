package game

import (
	"image/color"
	"time"

	"ugataima/internal/items"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

const doubleClickWindowMs = 700
const doubleClickWindow = doubleClickWindowMs * time.Millisecond

// UI Dimension constants
const (
	UIRowHeight  = 20
	UIRowSpacing = 25
)

// UISystem handles all user interface rendering and logic
type UISystem struct {
	game                *MMGame
	justOpenedStatPopup bool
	// cardPortraitCache holds portraits pre-fit to the party card's recess
	// (cover-scaled, corners bevel-cut); keyed by name|w|h so resizes rebuild.
	cardPortraitCache map[string]*ebiten.Image
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
	inventoryPage         int    // current inventory grid page (0-based)
	questPage             int    // current quest log page (0-based)
	characterPage         int    // current character-info page (0-based)
	campNotice            string // result line under the Camp button
	campNoticeOK          bool   // colors the notice green (rested) or red (refused)
	lastEquipClickTime    time.Time
	lastClickedSlot       items.EquipSlot
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
	// Cached radar dot images for wizard eye (avoid vector.DrawFilledCircle every frame)
	radarDotClose  *ebiten.Image // Red dot for close enemies
	radarDotMedium *ebiten.Image // Orange dot for medium distance
	radarDotFar    *ebiten.Image // Yellow dot for far enemies
	// Compass minimap tile-layer cache: the ~80 static tile fills only change
	// when the player crosses a tile boundary (or the world swaps), so they're
	// baked into one image and blitted per frame instead of re-emitting a
	// vector.FillRect per tile every frame.
	compassTileLayer  *ebiten.Image
	compassCacheTileX int
	compassCacheTileY int
	compassCacheWorld *world.World3D
}

// NewUISystem creates a new UI system
func NewUISystem(game *MMGame) *UISystem {
	ui := &UISystem{game: game}
	ui.initRadarDots()
	return ui
}

// initRadarDots creates cached circle images for wizard eye radar dots
func (ui *UISystem) initRadarDots() {
	dotSize := 4
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

// drawCircleToImage draws a filled circle to the given image
func drawCircleToImage(img *ebiten.Image, size int, c color.RGBA) {
	cx := float64(size-1) / 2
	cy := float64(size-1) / 2
	r2 := (float64(size) / 2) * (float64(size) / 2)
	for y := 0; y < size; y++ {
		dy := float64(y) - cy
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			if dx*dx+dy*dy <= r2 {
				img.Set(x, y, c)
			}
		}
	}
}

// Draw renders all UI elements
func (ui *UISystem) Draw(screen *ebiten.Image) {
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

	// Draw base game UI elements
	ui.drawGameplayUI(screen)

	// Draw debug/info elements
	ui.drawDebugInfo(screen)

	// Draw Game Over overlay if active
	if ui.game.gameOver {
		ui.drawGameOverOverlay(screen)
	}

	// Draw overlay interfaces (menus and dialogs)
	ui.drawOverlayInterfaces(screen)

	if ui.game.combatLogOpen {
		ui.drawCombatLogOverlay(screen)
	}

	// Draw Victory overlay if active
	if ui.game.gameVictory && !ui.game.showHighScores {
		ui.drawVictoryOverlay(screen)
	}

	// Draw High Scores overlay if active
	if ui.game.showHighScores {
		ui.drawHighScoresOverlay(screen)
	}

	// Draw stat distribution popup if open
	if ui.game.statPopupOpen {
		ui.drawStatDistributionPopup(screen)
	}

	// Draw revival picker (dead/unconscious party member chooser) if open
	if ui.game.revivalPickerOpen {
		ui.drawRevivalPickerPopup(screen)
	}
	if ui.game.healPickerOpen {
		ui.drawHealPickerPopup(screen)
	}

	// Draw promotion picker (which member becomes Archmage/Lich) if open
	if ui.game.promotionPickerOpen {
		ui.drawPromotionPickerPopup(screen)
	}

	// Draw tavern roster swap screen if open
	if ui.game.rosterScreenOpen {
		ui.drawRosterScreen(screen)
	}

	// Draw tavern stash screen if open
	if ui.game.stashScreenOpen {
		ui.drawStashScreen(screen)
	}

	// Draw level-up choice popup if pending
	if ui.game.currentLevelUpChoice() != nil {
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
	if ui.tooltipLines != nil && !ui.game.statPopupOpen && !ui.game.revivalPickerOpen && !ui.game.healPickerOpen && !ui.game.mapOverlayOpen && !ui.game.combatLogOpen {
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
