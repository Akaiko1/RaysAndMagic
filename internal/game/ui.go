package game

import (
	"image/color"
	"time"

	"ugataima/internal/items"

	"github.com/hajimehoshi/ebiten/v2"
)

const doubleClickWindowMs = 700
const doubleClickWindow = doubleClickWindowMs * time.Millisecond

// UI Color constants for DRY code
var (
	UIColorSelectedCharacter = color.RGBA{0, 100, 200, 128}  // Blue background for selected character
	UIColorKnowsSpell        = color.RGBA{100, 100, 100, 64} // Gray background for known spells
	UIColorCanLearn          = color.RGBA{0, 150, 0, 64}     // Green background for learnable spells
	UIColorCannotLearn       = color.RGBA{150, 0, 0, 64}     // Red background for non-learnable spells
	UIColorSpellSelection    = color.RGBA{0, 150, 0, 128}    // Green background for selected spell
)

// UI Dimension constants
const (
	UICharacterBackgroundWidth = 300
	UISpellBackgroundWidth     = 350
	UIRowHeight                = 20
	UIRowSpacing               = 25
)

// UISystem handles all user interface rendering and logic
type UISystem struct {
	game                  *MMGame
	justOpenedStatPopup   bool
	// statHoldStat is the name of the stat whose +button the user is
	// currently holding the mouse on. Empty means no hold in progress.
	// Single clicks go through consumeLeftClickIn; hold-to-repeat kicks in
	// after statHoldInitialDelay frames and fires every statHoldRepeatRate
	// frames thereafter so points pour out at a controllable rate.
	statHoldStat   string
	statHoldFrames int
	lastClickTime         time.Time
	lastClickedItem       int
	inventoryContextOpen  bool
	inventoryContextX     int
	inventoryContextY     int
	inventoryContextIndex int
	lastEquipClickTime    time.Time
	lastClickedSlot       items.EquipSlot
	tooltipLines          []string
	tooltipColors         []color.Color
	tooltipIcon           string
	tooltipX              int
	tooltipY              int
	tooltipCompareLines   []string
	tooltipCompareColors  []color.Color
	// Cached radar dot images for wizard eye (avoid vector.DrawFilledCircle every frame)
	radarDotClose  *ebiten.Image // Red dot for close enemies
	radarDotMedium *ebiten.Image // Orange dot for medium distance
	radarDotFar    *ebiten.Image // Yellow dot for far enemies
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

	// Draw level-up choice popup if pending
	if ui.game.currentLevelUpChoice() != nil {
		ui.drawLevelUpChoicePopup(screen)
	}

	// Draw tooltip last so it stays above other UI. NPC dialogs (dialogActive)
	// are no longer suppressed — the spell trader UI surfaces spell details on
	// hover and that's the only path that queues a tooltip there. Other modal
	// states (stat popup, revival picker, fullscreen map) still suppress.
	if ui.tooltipLines != nil && !ui.game.statPopupOpen && !ui.game.revivalPickerOpen && !ui.game.mapOverlayOpen {
		screenW := screen.Bounds().Dx()
		mainW, _ := tooltipBoxSizeForScreen(ui.tooltipLines, ui.tooltipColors, ui.tooltipIcon != "", ui.tooltipX, screenW)
		drawTooltip(screen, ui.tooltipLines, ui.tooltipColors, ui.tooltipIcon, ui.tooltipX, ui.tooltipY, ui.game.sprites)
		if ui.tooltipCompareLines != nil {
			compareW, _ := tooltipBoxSizeForScreen(ui.tooltipCompareLines, ui.tooltipCompareColors, false, 0, screenW)
			compareX := ui.tooltipX - tooltipCompareGap - compareW
			if compareX < 0 {
				compareX = ui.tooltipX + mainW + tooltipCompareGap
			}
			if compareX+compareW > screenW {
				compareX = screenW - compareW
			}
			if compareX < 0 {
				compareX = 0
			}
			drawTooltip(screen, ui.tooltipCompareLines, ui.tooltipCompareColors, "", compareX, ui.tooltipY, ui.game.sprites)
		}
	}
}
