package game

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"strings"
	"time"
	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
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
	lastClickTime         time.Time
	lastClickedItem       int
	inventoryContextOpen  bool
	inventoryContextX     int
	inventoryContextY     int
	inventoryContextIndex int
	lastEquipClickTime    time.Time
	lastClickedSlot       items.EquipSlot
}

// NewUISystem creates a new UI system
func NewUISystem(game *MMGame) *UISystem {
	return &UISystem{game: game}
}

// Draw renders all UI elements
func (ui *UISystem) Draw(screen *ebiten.Image) {
	// Draw base game UI elements
	ui.drawGameplayUI(screen)

	// Draw debug/info elements
	ui.drawDebugInfo(screen)

	// Draw overlay interfaces (menus and dialogs)
	ui.drawOverlayInterfaces(screen)

	// Draw Game Over overlay if active
	if ui.game.gameOver {
		ui.drawGameOverOverlay(screen)
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
}

type statMeta struct {
	Name string
	Ptr  *int
}

// MaxStatValue is the maximum base stat value a character can have
const MaxStatValue = 99

// drawStatPointRow draws a single stat row with name, value, and + button
func drawStatPointRow(screen *ebiten.Image, name string, valuePtr *int, y, plusX, plusY, btnW, btnH int, canAdd, isHover *bool, clickIn bool) bool {
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s: %d", name, *valuePtr), plusX-148, y)

	// Check if stat is already at max (99)
	atMax := *valuePtr >= MaxStatValue
	canActuallyAdd := *canAdd && !atMax

	plusImg := ebiten.NewImage(btnW, btnH)
	if canActuallyAdd && *isHover {
		plusImg.Fill(color.RGBA{80, 200, 80, 220})
	} else if atMax {
		plusImg.Fill(color.RGBA{100, 100, 100, 180}) // Gray out if at max
	} else {
		plusImg.Fill(color.RGBA{60, 120, 60, 180})
	}
	plusOpts := &ebiten.DrawImageOptions{}
	plusOpts.GeoM.Translate(float64(plusX), float64(plusY))
	screen.DrawImage(plusImg, plusOpts)
	ebitenutil.DebugPrintAt(screen, "+", plusX+8, plusY+4)
	// Handle click
	if canActuallyAdd && *isHover && clickIn {
		(*valuePtr)++
		*canAdd = false // Only allow one per click
		return true
	}
	return false
}

// drawStatPointPlusButton draws the + button under the portrait if stat points are available
func drawStatPointPlusButton(screen *ebiten.Image, x, y, w, h, points int, isHover bool) {
	plusBtnImg := ebiten.NewImage(w, h)
	if isHover {
		plusBtnImg.Fill(color.RGBA{80, 200, 80, 220})
	} else {
		plusBtnImg.Fill(color.RGBA{60, 120, 60, 180})
	}
	plusBtnOpts := &ebiten.DrawImageOptions{}
	plusBtnOpts.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(plusBtnImg, plusBtnOpts)
	ebitenutil.DebugPrintAt(screen, "+", x+7, y+3)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%d", points), x+w+2, y+6)
}

// drawStatDistributionPopup draws the stat allocation popup for the selected character
// drawStatDistributionPopup draws the stat allocation popup for the selected character
func (ui *UISystem) drawStatDistributionPopup(screen *ebiten.Image) {
	charIdx := ui.game.statPopupCharIdx
	if charIdx < 0 || charIdx >= len(ui.game.party.Members) {
		return
	}
	member := ui.game.party.Members[charIdx]

	// Popup dimensions
	popupW, popupH := 340, 320
	screenW := ui.game.config.GetScreenWidth()
	screenH := ui.game.config.GetScreenHeight()
	popupX := (screenW - popupW) / 2
	popupY := (screenH - popupH) / 2

	// Draw background
	bg := ebiten.NewImage(popupW, popupH)
	bg.Fill(color.RGBA{30, 30, 60, 240})
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(float64(popupX), float64(popupY))
	screen.DrawImage(bg, opts)

	// Draw border (replace deprecated DrawRect)
	borderCol := color.RGBA{120, 120, 180, 255}
	borderThickness := 2
	drawRectBorder(screen, popupX, popupY, popupW, popupH, borderThickness, borderCol)

	// Title
	ebitenutil.DebugPrintAt(screen, "Distribute Stat Points", popupX+16, popupY+16)
	ebitenutil.DebugPrintAt(screen, "Points left:", popupX+16, popupY+44)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%d", member.FreeStatPoints), popupX+120, popupY+44)

	// Stat list
	statList := []statMeta{
		{"Might", &member.Might},
		{"Intellect", &member.Intellect},
		{"Personality", &member.Personality},
		{"Endurance", &member.Endurance},
		{"Accuracy", &member.Accuracy},
		{"Speed", &member.Speed},
		{"Luck", &member.Luck},
	}
	yStart := popupY + 80
	btnW, btnH := 28, 28
	mouseX, mouseY := ebiten.CursorPosition()
	for i, stat := range statList {
		y := yStart + i*36
		plusX := popupX + 180
		plusY := y - 4
		canAdd := member.FreeStatPoints > 0
		isHover := mouseX >= plusX && mouseX < plusX+btnW && mouseY >= plusY && mouseY < plusY+btnH
		clickIn := ui.game.consumeLeftClickIn(plusX, plusY, plusX+btnW, plusY+btnH)
		if drawStatPointRow(screen, stat.Name, stat.Ptr, y, plusX, plusY, btnW, btnH, &canAdd, &isHover, clickIn) {
			member.FreeStatPoints--
			// Recalculate derived stats (HP, SP) when any stat is increased
			member.CalculateDerivedStats(ui.game.config)
		}
	}

	// Draw close button
	closeX := popupX + popupW - 40
	closeY := popupY + 12
	closeImg := ebiten.NewImage(28, 28)
	isCloseHover := mouseX >= closeX && mouseX < closeX+28 && mouseY >= closeY && mouseY < closeY+28
	if isCloseHover {
		closeImg.Fill(color.RGBA{200, 60, 60, 220})
	} else {
		closeImg.Fill(color.RGBA{120, 60, 60, 180})
	}
	closeOpts := &ebiten.DrawImageOptions{}
	closeOpts.GeoM.Translate(float64(closeX), float64(closeY))
	screen.DrawImage(closeImg, closeOpts)
	ebitenutil.DebugPrintAt(screen, "X", closeX+7, closeY+4)
	// Handle close click
	// Only allow closing if the mouse was released after opening the popup
	if isCloseHover && ui.game.consumeLeftClickIn(closeX, closeY, closeX+28, closeY+28) && !ui.justOpenedStatPopup {
		ui.game.statPopupOpen = false
	}

	// Handle ESC key to close popup
	if ebiten.IsKeyPressed(ebiten.KeyEscape) && !ui.justOpenedStatPopup {
		ui.game.statPopupOpen = false
	}

	// Reset justOpenedStatPopup after the first frame
	if ui.justOpenedStatPopup {
		ui.justOpenedStatPopup = false
	}
}

// drawGameplayUI draws core gameplay UI elements
func (ui *UISystem) drawGameplayUI(screen *ebiten.Image) {
	ui.drawPartyUI(screen)
	ui.drawSpellStatusBar(screen)
	ui.drawCompass(screen)
	ui.drawWizardEyeRadar(screen)
	ui.drawCombatMessages(screen)
	ui.drawTurnBasedStatus(screen)
	ui.drawInteractionNotification(screen)
}

// drawDebugInfo draws debug and information elements
func (ui *UISystem) drawDebugInfo(screen *ebiten.Image) {
	ui.drawInstructions(screen)
	if ui.game.showFPS {
		ui.drawFPSCounter(screen)
	}
}

// drawOverlayInterfaces draws overlay interfaces like menus and dialogs
func (ui *UISystem) drawOverlayInterfaces(screen *ebiten.Image) {
	if ui.game.menuOpen {
		ui.drawTabbedMenu(screen)
	}

	// Draw main menu (ESC)
	if ui.game.mainMenuOpen {
		ui.drawMainMenu(screen)
	}

	// Draw dialog if active
	if ui.game.dialogActive {
		ui.drawNPCDialog(screen)
	}

	if ui.game.mapOverlayOpen {
		ui.drawMapOverlay(screen)
	}
}

// drawMainMenu renders the ESC main menu overlay
func (ui *UISystem) drawMainMenu(screen *ebiten.Image) {
	w := ui.game.config.GetScreenWidth()
	h := ui.game.config.GetScreenHeight()

	// Dim background
	dim := ebiten.NewImage(w, h)
	dim.Fill(color.RGBA{0, 0, 0, 128})
	screen.DrawImage(dim, &ebiten.DrawImageOptions{})

	// Panel
	panelW, panelH := 300, 220
	if ui.game.mainMenuMode == MenuMain {
		panelW = 360
		panelH = 300
	}
	px := (w - panelW) / 2
	py := (h - panelH) / 2
	bg := ebiten.NewImage(panelW, panelH)
	bg.Fill(color.RGBA{20, 20, 40, 230})
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(float64(px), float64(py))
	screen.DrawImage(bg, opts)
	drawRectBorder(screen, px, py, panelW, panelH, 2, color.RGBA{100, 100, 160, 255})

	switch ui.game.mainMenuMode {
	case MenuMain:
		// Title
		ebitenutil.DebugPrintAt(screen, "Main Menu", px+16, py+14)
		// Options
		startY := py + 56
		for i, label := range mainMenuOptions {
			y := startY + i*32
			if i == ui.game.mainMenuSelection {
				hi := ebiten.NewImage(panelW-32, 28)
				hi.Fill(color.RGBA{60, 120, 180, 200})
				o := &ebiten.DrawImageOptions{}
				o.GeoM.Translate(float64(px+16), float64(y-4))
				screen.DrawImage(hi, o)
			}
			ebitenutil.DebugPrintAt(screen, label, px+28, y)
		}
		tips := []string{
			"Controls:",
			"WASD: Move  QE: Strafe",
			"Space: Attack  F: Cast  H: Heal",
			"I: Inventory  C: Characters  M: Spellbook",
			"1-4: Select",
			"Enter: Toggle Mode (TB/RT)",
		}
		tipsY := startY + len(mainMenuOptions)*32 + 10
		for i, tip := range tips {
			ebitenutil.DebugPrintAt(screen, tip, px+16, tipsY+i*14)
		}
	case MenuSaveSelect:
		ebitenutil.DebugPrintAt(screen, "Save Game - Select Slot", px+16, py+14)
		startY := py + 56
		for i := 0; i < 5; i++ {
			y := startY + i*32
			sum := GetSaveSlotSummary(i)
			label := fmt.Sprintf("Slot %d", i+1)
			if sum.Exists {
				mode := "RT"
				if sum.TurnBased {
					mode = "TB"
				}
				// show time (short) and mode
				t := sum.SavedAt
				if len(t) > 19 {
					t = t[:19]
				} // RFC3339 short
				label = fmt.Sprintf("%s  [%s %s]", label, mode, t)
			}
			if i == ui.game.slotSelection {
				hi := ebiten.NewImage(panelW-32, 28)
				hi.Fill(color.RGBA{80, 180, 80, 200})
				o := &ebiten.DrawImageOptions{}
				o.GeoM.Translate(float64(px+16), float64(y-4))
				screen.DrawImage(hi, o)
			}
			ebitenutil.DebugPrintAt(screen, label, px+28, y)
		}
	case MenuLoadSelect:
		ebitenutil.DebugPrintAt(screen, "Load Game - Select Slot", px+16, py+14)
		startY := py + 56
		for i := 0; i < 5; i++ {
			y := startY + i*32
			sum := GetSaveSlotSummary(i)
			label := fmt.Sprintf("Slot %d", i+1)
			if sum.Exists {
				mode := "RT"
				if sum.TurnBased {
					mode = "TB"
				}
				t := sum.SavedAt
				if len(t) > 19 {
					t = t[:19]
				}
				label = fmt.Sprintf("%s  [%s %s]", label, mode, t)
			}
			if i == ui.game.slotSelection {
				hi := ebiten.NewImage(panelW-32, 28)
				hi.Fill(color.RGBA{180, 120, 60, 200})
				o := &ebiten.DrawImageOptions{}
				o.GeoM.Translate(float64(px+16), float64(y-4))
				screen.DrawImage(hi, o)
			}
			ebitenutil.DebugPrintAt(screen, label, px+28, y)
		}
	}
}

// drawPartyUI draws the party member portraits and stats at the bottom of the screen
func (ui *UISystem) drawPartyUI(screen *ebiten.Image) {
	if !ui.game.showPartyStats {
		return
	}

	// Draw party member portraits and stats at bottom of screen
	portraitWidth := ui.game.config.GetScreenWidth() / 4 // 4 characters across screen
	portraitHeight := ui.game.config.UI.PartyPortraitHeight
	startY := ui.game.config.GetScreenHeight() - portraitHeight

	for i, member := range ui.game.party.Members {
		x := i * portraitWidth

		// Highlight selected character and heal target
		bgColor := color.RGBA{64, 64, 64, 200}
		if i == ui.game.selectedChar {
			bgColor = color.RGBA{100, 100, 100, 200}
		}

		// Highlight heal target when H key is pressed and current player has healing spell equipped
		if !ui.game.menuOpen && ebiten.IsKeyPressed(ebiten.KeyH) {
			// Check if current player has a healing spell equipped
			currentPlayer := ui.game.party.Members[ui.game.selectedChar]
			spell, hasSpell := currentPlayer.Equipment[items.SlotSpell]
			if hasSpell && (spell.SpellEffect == items.SpellEffectHealSelf || spell.SpellEffect == items.SpellEffectHealOther) {
				mouseX, mouseY := ebiten.CursorPosition()
				if ui.isMouseOverCharacter(mouseX, mouseY, i, portraitWidth, portraitHeight, startY) {
					// Check if this is a valid target based on spell effect
					var canTarget bool
					switch spell.SpellEffect {
					case items.SpellEffectHealSelf:
						// Only highlight the caster for self-only spells (First Aid)
						canTarget = (i == ui.game.selectedChar)
					case items.SpellEffectHealOther:
						// Highlight any party member for other-targeting spells (Heal)
						canTarget = true
					}

					if canTarget {
						bgColor = color.RGBA{0, 255, 0, 150} // Green highlight for heal target
					}
				}
			}
		}

		// Draw background panel using DrawImage
		panelImg := ebiten.NewImage(portraitWidth-2, portraitHeight)
		panelImg.Fill(bgColor)

		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(float64(x), float64(startY))
		screen.DrawImage(panelImg, opts)

		// Draw character portrait (Column 1)
		portraitName := strings.ToLower(member.Name)
		portrait := ui.game.sprites.GetSprite(portraitName)

		// Portrait dimensions - smaller to leave room for status and equipment
		portraitSize := portraitHeight - 20 // Leave 20px margin
		portraitX := x + 5
		portraitY := startY + 10
		portraitColWidth := 60 // Fixed width for portrait column

		// Scale and draw portrait
		portraitOpts := &ebiten.DrawImageOptions{}
		scaleX := float64(portraitColWidth-10) / float64(portrait.Bounds().Dx())
		scaleY := float64(portraitSize) / float64(portrait.Bounds().Dy())
		scale := math.Min(scaleX, scaleY) // Maintain aspect ratio

		portraitOpts.GeoM.Scale(scale, scale)
		portraitOpts.GeoM.Translate(float64(portraitX), float64(portraitY))

		// Apply red tint if character is blinking from damage
		if ui.game.IsCharacterBlinking(i) {
			portraitOpts.ColorScale.Scale(1.5, 0.5, 0.5, 1.0) // Red tint: more red, less green/blue
		}

		screen.DrawImage(portrait, portraitOpts)

		// Darken overlay if unconscious
		isUnconscious := false
		for _, cond := range member.Conditions {
			if cond == character.ConditionUnconscious {
				isUnconscious = true
				break
			}
		}
		if isUnconscious {
			dark := ebiten.NewImage(portraitWidth-2, portraitHeight)
			dark.Fill(color.RGBA{0, 0, 0, 140})
			darkOpts := &ebiten.DrawImageOptions{}
			darkOpts.GeoM.Translate(float64(x), float64(startY))
			screen.DrawImage(dark, darkOpts)
		}

		// Status Column (Column 2) - basic character info
		statusColX := x + portraitColWidth + 5
		statusColWidth := (portraitWidth - portraitColWidth - 15) / 2 // Half remaining space

		ebitenutil.DebugPrintAt(screen, member.Name, statusColX, startY+5)
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("HP:%d/%d", member.HitPoints, member.MaxHitPoints), statusColX, startY+20)
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("SP:%d/%d", member.SpellPoints, member.MaxSpellPoints), statusColX, startY+35)

		// Add character condition status
		statusText := "OK"
		if len(member.Conditions) > 0 {
			statusText = ui.getConditionName(member.Conditions[0])
		}
		ebitenutil.DebugPrintAt(screen, statusText, statusColX, startY+50)

		// Equipment Column (Column 3) - weapon and spell equipment (even closer to status)
		equipColX := statusColX + statusColWidth - 25 // Moved even closer (was -10, now -25)

		// Show equipped weapon
		if weapon, hasWeapon := member.Equipment[items.SlotMainHand]; hasWeapon {
			weaponText := fmt.Sprintf("W:%s", weapon.Name)
			if len(weaponText) > 12 { // Truncate if too long
				weaponText = weaponText[:9] + "..."
			}
			ebitenutil.DebugPrintAt(screen, weaponText, equipColX, startY+5)
		} else {
			ebitenutil.DebugPrintAt(screen, "W:None", equipColX, startY+5)
		}

		// Show equipped spell (unified slot)
		if spell, hasSpell := member.Equipment[items.SlotSpell]; hasSpell {
			spellText := fmt.Sprintf("S:%s", spell.Name)
			if len(spellText) > 12 { // Truncate if too long
				spellText = spellText[:9] + "..."
			}
			ebitenutil.DebugPrintAt(screen, spellText, equipColX, startY+20)
		} else {
			ebitenutil.DebugPrintAt(screen, "S:None", equipColX, startY+20)
		}

		// Draw + button for stat points if available (under portrait)
		if member.FreeStatPoints > 0 {
			plusBtnX := x + 20
			plusBtnY := startY + portraitHeight - 28
			plusBtnW := 24
			plusBtnH := 24
			mouseX, mouseY := ebiten.CursorPosition()
			isHover := mouseX >= plusBtnX && mouseX < plusBtnX+plusBtnW && mouseY >= plusBtnY && mouseY < plusBtnY+plusBtnH
			drawStatPointPlusButton(screen, plusBtnX, plusBtnY, plusBtnW, plusBtnH, member.FreeStatPoints, isHover)
			if ui.game.consumeLeftClickIn(plusBtnX, plusBtnY, plusBtnX+plusBtnW, plusBtnY+plusBtnH) {
				ui.game.statPopupOpen = true
				ui.game.statPopupCharIdx = i
				ui.justOpenedStatPopup = true
			}
		}
	}
}

// drawSpellStatusBar draws active spell effects in the top-left of the party UI area
func (ui *UISystem) drawSpellStatusBar(screen *ebiten.Image) {
	if !ui.game.showPartyStats {
		return
	}

	// Position at top-left of party UI area
	portraitHeight := ui.game.config.UI.PartyPortraitHeight
	partyStartY := ui.game.config.GetScreenHeight() - portraitHeight
	statusBarX := 10
	statusBarY := partyStartY - 40 // 40px above party UI

	iconSize := 24
	iconSpacing := 30
	currentX := statusBarX

	// Check for active spell effects using data-driven approach
	hasActiveSpells := false

	statuses := make([]*UtilitySpellStatus, 0, len(ui.game.utilitySpellStatuses))
	for _, status := range ui.game.utilitySpellStatuses {
		if status != nil && status.Duration > 0 {
			statuses = append(statuses, status)
		}
	}
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].SpellID < statuses[j].SpellID
	})

	for _, status := range statuses {
		iconX, iconY, iconW, iconH := ui.drawSpellIcon(screen, currentX, statusBarY, iconSize, status.Icon, status.Fallback, status.Duration, status.MaxDuration)
		ui.handleSpellIconClick(iconX, iconY, iconW, iconH, status.SpellID)
		currentX += iconSpacing
		hasActiveSpells = true
	}

	// Draw background bar if there are active spells
	if hasActiveSpells {
		barWidth := currentX - statusBarX + 10
		barHeight := iconSize + 8

		// Semi-transparent background
		bgImg := ebiten.NewImage(barWidth, barHeight)
		bgImg.Fill(color.RGBA{0, 0, 0, 120})

		bgOpts := &ebiten.DrawImageOptions{}
		bgOpts.GeoM.Translate(float64(statusBarX-5), float64(statusBarY-4))
		screen.DrawImage(bgImg, bgOpts)
	}
}

// drawSpellIcon draws a single spell status icon with duration bar and returns clickable bounds
func (ui *UISystem) drawSpellIcon(screen *ebiten.Image, x, y, size int, icon, fallback string, currentDuration, maxDuration int) (int, int, int, int) {
	// Draw icon background (more transparent, with border)
	iconBg := ebiten.NewImage(size, size)
	iconBg.Fill(color.RGBA{20, 20, 20, 120}) // Less opaque background

	// Draw border for visibility
	border := ebiten.NewImage(size, size)
	border.Fill(color.RGBA{80, 80, 80, 200})
	borderBg := ebiten.NewImage(size-2, size-2)
	borderBg.Fill(color.RGBA{20, 20, 20, 120})

	iconOpts := &ebiten.DrawImageOptions{}
	iconOpts.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(border, iconOpts)

	borderOpts := &ebiten.DrawImageOptions{}
	borderOpts.GeoM.Translate(float64(x+1), float64(y+1))
	screen.DrawImage(borderBg, borderOpts)

	// Draw icon - try emoji first, then fallback to ASCII
	ebitenutil.DebugPrintAt(screen, icon, x+6, y+8)

	// Draw ASCII fallback in the center for better visibility
	if fallback != "" {
		ebitenutil.DebugPrintAt(screen, fallback, x+size/2-4, y+size/2-4)
	}

	// Draw duration bar at bottom of icon
	if maxDuration > 0 {
		barWidth := size
		barHeight := 3

		// Background bar (gray)
		barBg := ebiten.NewImage(barWidth, barHeight)
		barBg.Fill(color.RGBA{60, 60, 60, 200})

		barBgOpts := &ebiten.DrawImageOptions{}
		barBgOpts.GeoM.Translate(float64(x), float64(y+size-barHeight))
		screen.DrawImage(barBg, barBgOpts)

		// Duration bar (colored based on remaining time)
		if currentDuration > 0 {
			fillWidth := int(float64(barWidth) * float64(currentDuration) / float64(maxDuration))
			if fillWidth > 0 {
				// Color changes from green to yellow to red as time runs out
				progress := float64(currentDuration) / float64(maxDuration)
				var barColor color.RGBA
				if progress > 0.6 {
					barColor = color.RGBA{0, 200, 0, 255} // Green
				} else if progress > 0.3 {
					barColor = color.RGBA{200, 200, 0, 255} // Yellow
				} else {
					barColor = color.RGBA{200, 100, 0, 255} // Orange-red
				}

				fillBar := ebiten.NewImage(fillWidth, barHeight)
				fillBar.Fill(barColor)

				fillOpts := &ebiten.DrawImageOptions{}
				fillOpts.GeoM.Translate(float64(x), float64(y+size-barHeight))
				screen.DrawImage(fillBar, fillOpts)
			}
		}
	}

	// Return clickable bounds (x, y, width, height)
	return x, y, size, size
}

// handleSpellIconClick handles mouse clicks on spell status icons for dispelling
func (ui *UISystem) handleSpellIconClick(x, y, width, height int, spellID spells.SpellID) {
	// Check for mouse click (only process on first press, not while held)
	if ui.game.consumeLeftClickIn(x, y, x+width, y+height) {
		currentTime := ui.game.mouseLeftClickAt

		// Check for double-click (within 500ms and same icon)
		delta := currentTime - ui.game.lastUtilitySpellClickTime
		doubleClick := delta < doubleClickWindowMs && ui.game.lastClickedUtilitySpell == string(spellID)
		if doubleClick {
			// Double-click detected - dispel the spell
			ui.dispelUtilitySpell(spellID)
			// Reset click tracking
			ui.game.lastUtilitySpellClickTime = 0
			ui.game.lastClickedUtilitySpell = ""
		} else {
			// Single click - record for potential double-click
			ui.game.lastUtilitySpellClickTime = currentTime
			ui.game.lastClickedUtilitySpell = string(spellID)
		}
	}
}

// dispelUtilitySpell removes an active utility spell effect by triggering natural expiration
func (ui *UISystem) dispelUtilitySpell(spellID spells.SpellID) {
	switch string(spellID) {
	case "torch_light":
		if ui.game.torchLightActive {
			ui.game.torchLightDuration = 0 // Let updateTorchLightEffect handle cleanup
			ui.game.AddCombatMessage("Torch Light dispelled!")
		}
	case "wizard_eye":
		if ui.game.wizardEyeActive {
			ui.game.wizardEyeDuration = 0 // Let updateWizardEyeEffect handle cleanup
			ui.game.AddCombatMessage("Wizard Eye dispelled!")
		}
	case "walk_on_water":
		if ui.game.walkOnWaterActive {
			ui.game.walkOnWaterDuration = 0 // Let updateWalkOnWaterEffect handle cleanup
			ui.game.AddCombatMessage("Walk on Water dispelled!")
		}
	case "water_breathing":
		if ui.game.waterBreathingActive {
			ui.game.waterBreathingDuration = 0 // Let updateWaterBreathingEffect handle cleanup (including underwater return)
			ui.game.AddCombatMessage("Water Breathing dispelled!")
		}
	case "bless":
		if ui.game.blessActive {
			ui.game.blessDuration = 0 // Let updateBlessEffect handle cleanup
			ui.game.AddCombatMessage("Bless dispelled!")
		}
	}
}

// drawCompass draws the compass/direction indicator with minimap showing nearby tiles
func (ui *UISystem) drawCompass(screen *ebiten.Image) {
	compassX, compassY := ui.getCompassCenter()
	compassRadius := ui.game.config.UI.CompassRadius

	// Draw compass background circle (dark, semi-transparent)
	vector.DrawFilledCircle(screen, float32(compassX), float32(compassY), float32(compassRadius), color.RGBA{20, 20, 30, 200}, true)

	// Draw minimap tiles within the compass
	ui.drawCompassMinimap(screen, compassX, compassY, compassRadius)

	// Draw compass border
	vector.StrokeCircle(screen, float32(compassX), float32(compassY), float32(compassRadius), 2, color.RGBA{100, 100, 140, 255}, true)

	// Draw direction arrow pointing in the camera direction
	arrowLength := float64(compassRadius - 8)
	arrowX := float64(compassX) + arrowLength*math.Cos(ui.game.camera.Angle)
	arrowY := float64(compassY) + arrowLength*math.Sin(ui.game.camera.Angle)

	// Draw arrow line from center towards the direction
	vector.StrokeLine(screen, float32(compassX), float32(compassY), float32(arrowX), float32(arrowY), 2, color.RGBA{255, 80, 80, 255}, true)

	// Draw arrow head
	arrowHeadSize := 5.0
	arrowImg := ebiten.NewImage(int(arrowHeadSize), int(arrowHeadSize))
	arrowImg.Fill(color.RGBA{255, 80, 80, 255})
	arrowOpts := &ebiten.DrawImageOptions{}
	arrowOpts.GeoM.Translate(arrowX-arrowHeadSize/2, arrowY-arrowHeadSize/2)
	screen.DrawImage(arrowImg, arrowOpts)

	// Draw player position indicator in center
	vector.DrawFilledCircle(screen, float32(compassX), float32(compassY), 3, color.RGBA{50, 200, 255, 255}, true)
}

// drawCompassMinimap renders the nearby tiles on the compass as a minimap
func (ui *UISystem) drawCompassMinimap(screen *ebiten.Image, centerX, centerY, radius int) {
	if ui.game.world == nil {
		return
	}

	tileSize := ui.game.config.GetTileSize()
	playerTileX := int(ui.game.camera.X / tileSize)
	playerTileY := int(ui.game.camera.Y / tileSize)

	// Number of tiles to show in each direction from center
	viewRange := 5
	// Size of each minimap tile in pixels
	miniTileSize := float32(radius) / float32(viewRange+1)
	if miniTileSize < 3 {
		miniTileSize = 3
	}
	if miniTileSize > 8 {
		miniTileSize = 8
	}

	// Get floor color from map config
	floorColor := color.RGBA{60, 110, 60, 180}
	if world.GlobalWorldManager != nil {
		if mapCfg := world.GlobalWorldManager.GetCurrentMapConfig(); mapCfg != nil {
			floorColor = color.RGBA{uint8(mapCfg.DefaultFloorColor[0]), uint8(mapCfg.DefaultFloorColor[1]), uint8(mapCfg.DefaultFloorColor[2]), 180}
		}
	}

	// Render tiles around the player
	for dy := -viewRange; dy <= viewRange; dy++ {
		for dx := -viewRange; dx <= viewRange; dx++ {
			tileX := playerTileX + dx
			tileY := playerTileY + dy

			// Skip tiles outside world bounds
			if tileX < 0 || tileX >= ui.game.world.Width || tileY < 0 || tileY >= ui.game.world.Height {
				continue
			}

			// Calculate screen position (offset from compass center)
			screenX := float32(centerX) + float32(dx)*miniTileSize
			screenY := float32(centerY) + float32(dy)*miniTileSize

			// Check if this tile is within the circular compass area
			distFromCenter := math.Sqrt(float64(dx*dx + dy*dy))
			if distFromCenter > float64(viewRange) {
				continue
			}

			// Get tile color based on type
			tile := ui.game.world.Tiles[tileY][tileX]
			tileColor := ui.getMinimapTileColor(tile, floorColor)

			// Draw the minimap tile
			halfSize := miniTileSize / 2
			vector.DrawFilledRect(screen, screenX-halfSize, screenY-halfSize, miniTileSize, miniTileSize, tileColor, false)
		}
	}

	// Draw NPCs on minimap
	for _, npc := range ui.game.world.NPCs {
		npcTileX := int(npc.X / tileSize)
		npcTileY := int(npc.Y / tileSize)
		dx := npcTileX - playerTileX
		dy := npcTileY - playerTileY

		// Only show NPCs within view range
		distFromCenter := math.Sqrt(float64(dx*dx + dy*dy))
		if distFromCenter <= float64(viewRange) {
			screenX := float32(centerX) + float32(dx)*miniTileSize
			screenY := float32(centerY) + float32(dy)*miniTileSize
			// Draw NPC as yellow dot
			vector.DrawFilledCircle(screen, screenX, screenY, miniTileSize/2, color.RGBA{255, 220, 0, 255}, true)
		}
	}
}

// getMinimapTileColor returns the color for a tile type on the minimap
func (ui *UISystem) getMinimapTileColor(tile world.TileType3D, floorColor color.RGBA) color.RGBA {
	switch tile {
	case world.TileWall, world.TileTree, world.TileAncientTree, world.TileThicket, world.TileMossRock, world.TileLowWall, world.TileHighWall:
		return color.RGBA{50, 50, 60, 200} // Dark for walls/obstacles
	case world.TileWater:
		return color.RGBA{40, 90, 160, 200} // Blue for water
	case world.TileDeepWater:
		return color.RGBA{25, 60, 120, 200} // Darker blue for deep water
	case world.TileVioletTeleporter:
		return color.RGBA{170, 80, 200, 200} // Violet for teleporters
	case world.TileRedTeleporter:
		return color.RGBA{200, 70, 70, 200} // Red for teleporters
	case world.TileClearing:
		return color.RGBA{80, 140, 80, 180} // Lighter green for clearings
	default:
		return floorColor
	}
}

// drawWizardEyeRadar draws enemy dots on the compass when wizard eye is active
func (ui *UISystem) drawWizardEyeRadar(screen *ebiten.Image) {
	if !ui.game.wizardEyeActive {
		return
	}

	compassX, compassY := ui.getCompassCenter()
	compassRadius := ui.game.config.UI.CompassRadius

	// Convert tile distance to pixel distance
	tileSize := float64(ui.game.config.GetTileSize())
	maxRadarRange := 10.0 * tileSize // 10 tiles range

	// Check each monster for distance from player
	for _, monster := range ui.game.world.Monsters {
		if !monster.IsAlive() {
			continue
		}

		// Calculate distance from player
		dx := monster.X - ui.game.camera.X
		dy := monster.Y - ui.game.camera.Y
		dist := Distance(ui.game.camera.X, ui.game.camera.Y, monster.X, monster.Y)

		// Only show enemies within 10 tiles
		if dist <= maxRadarRange {
			// Calculate angle from player to monster
			angle := math.Atan2(dy, dx)

			// Place dot at compass edge based on direction
			edgeRadius := float64(compassRadius - 5) // 5 pixels inside compass edge
			dotX := compassX + int(math.Cos(angle)*edgeRadius)
			dotY := compassY + int(math.Sin(angle)*edgeRadius)

			// Draw enemy dot
			dotSize := 4 // Slightly larger for better visibility
			dotImg := ebiten.NewImage(dotSize, dotSize)

			// Color based on distance for threat assessment
			if dist < tileSize*3 { // Close enemies in red
				dotImg.Fill(color.RGBA{255, 50, 50, 255}) // Bright red
			} else if dist < tileSize*6 { // Medium distance in orange
				dotImg.Fill(color.RGBA{255, 150, 50, 255}) // Orange
			} else { // Far enemies in yellow
				dotImg.Fill(color.RGBA{255, 255, 50, 255}) // Yellow
			}

			dotOpts := &ebiten.DrawImageOptions{}
			dotOpts.GeoM.Translate(float64(dotX-dotSize/2), float64(dotY-dotSize/2))
			screen.DrawImage(dotImg, dotOpts)
		}
	}
}

// drawInstructions draws the control instructions
func (ui *UISystem) drawInstructions(screen *ebiten.Image) {
	ebitenutil.DebugPrintAt(screen, "ESC: Main menu", 10, 10)
}

// drawTabbedMenu draws the tabbed menu interface with mouse click support
func (ui *UISystem) drawTabbedMenu(screen *ebiten.Image) {
	// Panel dimensions
	panelWidth := 700
	panelHeight := 500 // Increased from 441 to accommodate taller character cards
	panelX := (ui.game.config.GetScreenWidth() - panelWidth) / 2
	panelY := (ui.game.config.GetScreenHeight() - panelHeight) / 2

	// Draw main background
	bgImg := ebiten.NewImage(panelWidth, panelHeight)
	bgImg.Fill(color.RGBA{0, 0, 30, 230}) // Dark blue background

	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(float64(panelX), float64(panelY))
	screen.DrawImage(bgImg, opts)

	// Tab dimensions
	tabWidth := 120
	tabHeight := 35
	tabY := panelY + 10

	// Draw tabs
	tabs := []struct {
		tab   MenuTab
		label string
		key   string
	}{
		{TabInventory, "Inventory", "(I)"},
		{TabCharacters, "Characters", "(C)"},
		{TabSpellbook, "Spellbook", "(M)"},
		{TabQuests, "Quests", "(J)"},
	}

	for i, tabInfo := range tabs {
		tabX := panelX + 20 + (i * (tabWidth + 5)) // Reduced spacing between tabs

		// Determine tab state and colors
		isActive := ui.game.currentTab == tabInfo.tab
		var tabBgColor, tabBorderColor color.RGBA

		if isActive {
			// Active tab: lighter background, matches panel, no bottom border
			tabBgColor = color.RGBA{0, 0, 30, 230}        // Same as panel background
			tabBorderColor = color.RGBA{80, 80, 120, 255} // Light border
		} else {
			// Inactive tab: darker background, full border
			tabBgColor = color.RGBA{20, 20, 40, 200}     // Darker background
			tabBorderColor = color.RGBA{60, 60, 90, 255} // Darker border
		}

		// Draw tab background
		tabImg := ebiten.NewImage(tabWidth, tabHeight)
		tabImg.Fill(tabBgColor)

		// Draw tab borders (top, left, right)
		borderImg := ebiten.NewImage(tabWidth, 2) // Top border
		borderImg.Fill(tabBorderColor)

		leftBorderImg := ebiten.NewImage(2, tabHeight) // Left border
		leftBorderImg.Fill(tabBorderColor)

		rightBorderImg := ebiten.NewImage(2, tabHeight) // Right border
		rightBorderImg.Fill(tabBorderColor)

		// Draw bottom border only for inactive tabs
		var bottomBorderImg *ebiten.Image
		if !isActive {
			bottomBorderImg = ebiten.NewImage(tabWidth, 2)
			bottomBorderImg.Fill(tabBorderColor)
		}

		// Position and draw tab elements
		tabOpts := &ebiten.DrawImageOptions{}
		tabOpts.GeoM.Translate(float64(tabX), float64(tabY))
		screen.DrawImage(tabImg, tabOpts)

		// Draw borders
		topBorderOpts := &ebiten.DrawImageOptions{}
		topBorderOpts.GeoM.Translate(float64(tabX), float64(tabY))
		screen.DrawImage(borderImg, topBorderOpts)

		leftBorderOpts := &ebiten.DrawImageOptions{}
		leftBorderOpts.GeoM.Translate(float64(tabX), float64(tabY))
		screen.DrawImage(leftBorderImg, leftBorderOpts)

		rightBorderOpts := &ebiten.DrawImageOptions{}
		rightBorderOpts.GeoM.Translate(float64(tabX+tabWidth-2), float64(tabY))
		screen.DrawImage(rightBorderImg, rightBorderOpts)

		if !isActive && bottomBorderImg != nil {
			bottomBorderOpts := &ebiten.DrawImageOptions{}
			bottomBorderOpts.GeoM.Translate(float64(tabX), float64(tabY+tabHeight-2))
			screen.DrawImage(bottomBorderImg, bottomBorderOpts)
		}

		// Draw tab text (using standard debug print for now)
		if isActive {
			ebitenutil.DebugPrintAt(screen, tabInfo.label, tabX+10, tabY+8)
		} else {
			ebitenutil.DebugPrintAt(screen, tabInfo.label, tabX+10, tabY+8)
		}
		ebitenutil.DebugPrintAt(screen, tabInfo.key, tabX+10, tabY+20)

		// Handle mouse clicks on tabs
		ui.handleTabClick(tabX, tabY, tabWidth, tabHeight, tabInfo.tab)
	}

	// Draw main panel border that connects with active tab
	panelBorderColor := color.RGBA{80, 80, 120, 255}

	// Top border (with gap for active tab)
	activeTabIndex := 0
	for i, tabInfo := range tabs {
		if ui.game.currentTab == tabInfo.tab {
			activeTabIndex = i
			break
		}
	}

	activeTabX := panelX + 20 + (activeTabIndex * (tabWidth + 5))

	// Left part of top border (before active tab)
	if activeTabX > panelX {
		leftTopBorder := ebiten.NewImage(activeTabX-panelX, 2)
		leftTopBorder.Fill(panelBorderColor)
		leftTopOpts := &ebiten.DrawImageOptions{}
		leftTopOpts.GeoM.Translate(float64(panelX), float64(panelY))
		screen.DrawImage(leftTopBorder, leftTopOpts)
	}

	// Right part of top border (after active tab)
	rightStart := activeTabX + tabWidth
	if rightStart < panelX+panelWidth {
		rightTopBorder := ebiten.NewImage((panelX+panelWidth)-rightStart, 2)
		rightTopBorder.Fill(panelBorderColor)
		rightTopOpts := &ebiten.DrawImageOptions{}
		rightTopOpts.GeoM.Translate(float64(rightStart), float64(panelY))
		screen.DrawImage(rightTopBorder, rightTopOpts)
	}

	// Left, right, and bottom borders of main panel
	leftPanelBorder := ebiten.NewImage(2, panelHeight)
	leftPanelBorder.Fill(panelBorderColor)
	leftPanelOpts := &ebiten.DrawImageOptions{}
	leftPanelOpts.GeoM.Translate(float64(panelX), float64(panelY))
	screen.DrawImage(leftPanelBorder, leftPanelOpts)

	rightPanelBorder := ebiten.NewImage(2, panelHeight)
	rightPanelBorder.Fill(panelBorderColor)
	rightPanelOpts := &ebiten.DrawImageOptions{}
	rightPanelOpts.GeoM.Translate(float64(panelX+panelWidth-2), float64(panelY))
	screen.DrawImage(rightPanelBorder, rightPanelOpts)

	bottomPanelBorder := ebiten.NewImage(panelWidth, 2)
	bottomPanelBorder.Fill(panelBorderColor)
	bottomPanelOpts := &ebiten.DrawImageOptions{}
	bottomPanelOpts.GeoM.Translate(float64(panelX), float64(panelY+panelHeight-2))
	screen.DrawImage(bottomPanelBorder, bottomPanelOpts)

	// Draw X close button in top-right corner
	closeButtonSize := 20
	closeButtonX := panelX + panelWidth - closeButtonSize - 5
	closeButtonY := panelY + 5

	// Handle mouse clicks on close button
	ui.handleCloseButtonClick(closeButtonX, closeButtonY, closeButtonSize, closeButtonSize)

	// Draw close button background
	closeButtonBg := ebiten.NewImage(closeButtonSize, closeButtonSize)
	mouseX, mouseY := ebiten.CursorPosition()
	isCloseHovering := mouseX >= closeButtonX && mouseX < closeButtonX+closeButtonSize &&
		mouseY >= closeButtonY && mouseY < closeButtonY+closeButtonSize

	if isCloseHovering {
		closeButtonBg.Fill(color.RGBA{150, 50, 50, 200}) // Red hover
	} else {
		closeButtonBg.Fill(color.RGBA{100, 100, 100, 150}) // Gray normal
	}

	closeButtonOpts := &ebiten.DrawImageOptions{}
	closeButtonOpts.GeoM.Translate(float64(closeButtonX), float64(closeButtonY))
	screen.DrawImage(closeButtonBg, closeButtonOpts)

	// Draw X text
	ebitenutil.DebugPrintAt(screen, "X", closeButtonX+6, closeButtonY+4)

	// Draw content area
	contentY := tabY + tabHeight + 10
	contentHeight := panelHeight - tabHeight - 40

	// Draw content based on selected tab
	switch ui.game.currentTab {
	case TabInventory:
		ui.drawInventoryContent(screen, panelX, contentY, contentHeight)
	case TabCharacters:
		ui.drawCharactersContent(screen, panelX, contentY)
	case TabSpellbook:
		ui.drawSpellbookContent(screen, panelX, contentY, contentHeight)
	case TabQuests:
		ui.drawQuestsContent(screen, panelX, contentY, contentHeight)
	}
}

// handleTabClick checks if mouse clicked on a tab and switches to it
func (ui *UISystem) handleTabClick(tabX, tabY, tabWidth, tabHeight int, tab MenuTab) {
	if ui.game.consumeLeftClickIn(tabX, tabY, tabX+tabWidth, tabY+tabHeight) {
		ui.game.currentTab = tab
	}
}

// handleCharacterCardClick checks if mouse clicked on a character card and selects that character
func (ui *UISystem) handleCharacterCardClick(cardX, cardY, cardWidth, cardHeight, characterIndex int) {
	if ui.game.consumeLeftClickIn(cardX, cardY, cardX+cardWidth, cardY+cardHeight) {
		ui.game.selectedChar = characterIndex
	}
}

// handleCloseButtonClick checks if mouse clicked on the close button and closes the menu
func (ui *UISystem) handleCloseButtonClick(buttonX, buttonY, buttonWidth, buttonHeight int) {
	if ui.game.consumeLeftClickIn(buttonX, buttonY, buttonX+buttonWidth, buttonY+buttonHeight) {
		ui.game.menuOpen = false
	}
}

// handleSpellbookSchoolClick checks if mouse clicked on a magic school and selects it
func (ui *UISystem) handleSpellbookSchoolClick(schoolX, schoolY, schoolWidth, schoolHeight int, schoolIndex int, school character.MagicSchool) {
	if ui.game.consumeLeftClickIn(schoolX, schoolY, schoolX+schoolWidth, schoolY+schoolHeight) {
		currentTime := ui.game.mouseLeftClickAt
		delta := currentTime - ui.game.lastSchoolClickTime
		doubleClick := ui.game.lastSchoolClickedIdx == schoolIndex && delta < doubleClickWindowMs

		ui.game.selectedSchool = schoolIndex
		ui.game.selectedSpell = 0 // Reset spell selection when changing school

		if doubleClick {
			ui.game.collapsedSpellSchools[school] = !ui.game.collapsedSpellSchools[school]
		}

		ui.game.lastSchoolClickTime = currentTime
		ui.game.lastSchoolClickedIdx = schoolIndex
	}
}

// handleSpellbookSpellClick checks if mouse clicked on a spell and selects it
func (ui *UISystem) handleSpellbookSpellClick(spellX, spellY, spellWidth, spellHeight, schoolIndex, spellIndex int) {
	if ui.game.consumeLeftClickIn(spellX, spellY, spellX+spellWidth, spellY+spellHeight) {
		currentTime := ui.game.mouseLeftClickAt

		// Check for double-click (within 500ms of last click on same spell)
		delta := currentTime - ui.game.lastSpellClickTime
		doubleClick := ui.game.lastClickedSpell == spellIndex &&
			ui.game.lastClickedSchool == schoolIndex &&
			delta < doubleClickWindowMs

		// Update selection for highlight and keyboard navigation
		ui.game.selectedSchool = schoolIndex
		ui.game.selectedSpell = spellIndex

		if doubleClick {
			// Double-click detected - cast the spell directly
			ui.game.combat.CastSelectedSpell()
		}

		// Update click tracking
		ui.game.lastSpellClickTime = currentTime
		ui.game.lastClickedSpell = spellIndex
		ui.game.lastClickedSchool = schoolIndex
	}
}

// updateMouseState should be called once per frame before input handling.
func (ui *UISystem) updateMouseState() {
	leftJustPressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	rightJustPressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight)
	now := time.Now().UnixMilli()
	ui.game.pruneClickQueues(now)

	if leftJustPressed {
		x, y := ebiten.CursorPosition()
		ui.game.mouseLeftClicks = append(ui.game.mouseLeftClicks, queuedClick{x: x, y: y, at: now})
		ui.game.mouseLeftClickX, ui.game.mouseLeftClickY = x, y
	}
	if rightJustPressed {
		x, y := ebiten.CursorPosition()
		ui.game.mouseRightClicks = append(ui.game.mouseRightClicks, queuedClick{x: x, y: y, at: now})
		ui.game.mouseRightClickX, ui.game.mouseRightClickY = x, y
	}
}

// drawTooltip draws a tooltip with the given text lines at the specified position
func drawTooltip(screen *ebiten.Image, lines []string, x, y int) {
	bgWidth := 0
	for _, line := range lines {
		if w := len(line)*7 + 12; w > bgWidth {
			bgWidth = w
		}
	}
	bgHeight := len(lines)*16 + 8
	tooltipBg := ebiten.NewImage(bgWidth, bgHeight)
	tooltipBg.Fill(color.RGBA{30, 30, 60, 220})
	bgOpts := &ebiten.DrawImageOptions{}
	bgOpts.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(tooltipBg, bgOpts)
	for i, line := range lines {
		ebitenutil.DebugPrintAt(screen, line, x+6, y+6+i*16)
	}
}

// isMouseHoveringBox checks if the mouse is hovering over a rectangular area
func isMouseHoveringBox(mouseX, mouseY, x1, y1, x2, y2 int) bool {
	return mouseX >= x1 && mouseX < x2 && mouseY >= y1 && mouseY < y2
}

// drawInventoryContent draws the inventory tab content
func (ui *UISystem) drawInventoryContent(screen *ebiten.Image, panelX, contentY, contentHeight int) {
	// Title
	ebitenutil.DebugPrintAt(screen, "=== PARTY INVENTORY & EQUIPMENT ===", panelX+20, contentY+10)

	// Party resources
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Gold: %d  Food: %d  Total Items: %d",
		ui.game.party.Gold, ui.game.party.Food, ui.game.party.GetTotalItems()),
		panelX+20, contentY+30)

	// Split into two sections: Equipment (left) and General Inventory (right)

	// Equipment section (left side)
	equipX := panelX + 20
	equipY := contentY + 60
	ebitenutil.DebugPrintAt(screen, "=== CHARACTER EQUIPMENT ===", equipX, equipY)

	currentChar := ui.game.party.Members[ui.game.selectedChar]
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s's Equipment:", currentChar.Name), equipX, equipY+20)

	// Show equipped items for selected character
	equipSlots := []struct {
		slot items.EquipSlot
		name string
	}{
		{items.SlotMainHand, "Main Hand"},
		{items.SlotOffHand, "Off Hand"},
		{items.SlotSpell, "Spell"},
		{items.SlotArmor, "Armor"},
		{items.SlotHelmet, "Helmet"},
		{items.SlotBoots, "Boots"},
		{items.SlotGauntlets, "Gauntlets"},
		{items.SlotBelt, "Belt"},
		{items.SlotCloak, "Cloak"},
		{items.SlotAmulet, "Amulet"},
		{items.SlotRing1, "Ring 1"},
		{items.SlotRing2, "Ring 2"},
	}

	equipmentY := equipY + 40
	mouseX, mouseY := ebiten.CursorPosition()
	var equipTooltip string
	var equipTooltipX, equipTooltipY int
	for i, slotInfo := range equipSlots {
		y := equipmentY + (i * 15)
		if item, equipped := currentChar.Equipment[slotInfo.slot]; equipped {
			// Create colored background for equipped items
			equipBg := ebiten.NewImage(220, 15)
			isHovering := isMouseHoveringBox(mouseX, mouseY, equipX, y, equipX+220, y+15)

			var bgColor color.RGBA
			if isHovering {
				bgColor = color.RGBA{60, 80, 40, 80} // Green tint when hovering over equipped items
			} else {
				bgColor = color.RGBA{30, 40, 20, 40} // Subtle green background for equipped items
			}

			equipBg.Fill(bgColor)
			equipBgOpts := &ebiten.DrawImageOptions{}
			equipBgOpts.GeoM.Translate(float64(equipX), float64(y))
			screen.DrawImage(equipBg, equipBgOpts)

			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%-8s: %s", slotInfo.name, item.Name), equipX, y)

			// Handle double-click to unequip
			ui.handleEquippedItemClick(slotInfo.slot, equipX, y, equipX+220, y+15)

			// Handle hover tooltip
			if isHovering {
				equipTooltip = GetItemTooltip(item, currentChar, ui.game.combat)
				equipTooltipX = mouseX + 16
				equipTooltipY = mouseY + 8
			}
		} else {
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%-8s: (empty)", slotInfo.name), equipX, y)
		}
	}
	// Draw tooltip for equipped item if needed
	if equipTooltip != "" {
		lines := strings.Split(equipTooltip, "\n")
		drawTooltip(screen, lines, equipTooltipX, equipTooltipY)
	}

	// General inventory section (right side)
	invX := panelX + 350 // Moved further right due to larger panel
	invY := contentY + 60
	ebitenutil.DebugPrintAt(screen, "=== GENERAL INVENTORY ===", invX, invY)

	// Inventory items
	itemsY := invY + 20
	if len(ui.game.party.Inventory) == 0 {
		ebitenutil.DebugPrintAt(screen, "No items in inventory", invX, itemsY)
	} else {
		mouseX, mouseY := ebiten.CursorPosition()
		var tooltip string
		var tooltipX, tooltipY int
		for i, item := range ui.game.party.Inventory {
			if i >= 15 {
				ebitenutil.DebugPrintAt(screen, fmt.Sprintf("... and %d more items",
					len(ui.game.party.Inventory)-15), invX, itemsY+(i*15))
				break
			}
			y := itemsY + (i * 15)
			currentChar := ui.game.party.Members[ui.game.selectedChar]

			// Check if item can be equipped by current character
			canEquip := true
			if item.Type == items.ItemWeapon {
				canEquip = currentChar.CanEquipWeaponByName(item.Name)
			}

			// Create colored background for the item
			itemBg := ebiten.NewImage(200, 15)
			var bgColor color.RGBA
			isHovering := isMouseHoveringBox(mouseX, mouseY, invX, y, invX+200, y+15)

			if !canEquip {
				// Red background for unusable items
				if isHovering {
					bgColor = color.RGBA{120, 40, 40, 100}
				} else {
					bgColor = color.RGBA{80, 20, 20, 60}
				}
			} else {
				// Normal background
				if isHovering {
					bgColor = color.RGBA{40, 40, 80, 80}
				} else {
					bgColor = color.RGBA{20, 20, 40, 40}
				}
			}

			itemBg.Fill(bgColor)
			itemBgOpts := &ebiten.DrawImageOptions{}
			itemBgOpts.GeoM.Translate(float64(invX), float64(y))
			screen.DrawImage(itemBg, itemBgOpts)

			// Draw item name
			itemInfo := fmt.Sprintf("%d. %s", i+1, item.Name)
			if !canEquip {
				itemInfo += " (can't equip)"
			}
			ebitenutil.DebugPrintAt(screen, itemInfo, invX, y)

			// Handle double-click to equip
			if !ui.inventoryContextOpen {
				ui.handleInventoryItemClick(i, invX, y, invX+200, y+15)
			}

			// Handle right-click to open context menu
			if !ui.inventoryContextOpen && ui.game.consumeRightClickIn(invX, y, invX+200, y+15) {
				ui.inventoryContextOpen = true
				ui.inventoryContextX = ui.game.mouseRightClickX
				ui.inventoryContextY = ui.game.mouseRightClickY
				ui.inventoryContextIndex = i
			}

			// Handle hover tooltip
			if isHovering {
				// Show tooltip for this item
				tooltip = GetItemTooltip(item, currentChar, ui.game.combat)
				tooltipX = mouseX + 16
				tooltipY = mouseY + 8
			}
		}
		// Draw tooltip if needed
		if tooltip != "" {
			lines := strings.Split(tooltip, "\n")
			drawTooltip(screen, lines, tooltipX, tooltipY)
		}

		// Draw inventory context menu if open
		if ui.inventoryContextOpen {
			menuW := 140
			menuH := 24
			x := ui.inventoryContextX
			y := ui.inventoryContextY
			// Background
			menuBg := ebiten.NewImage(menuW, menuH)
			menuBg.Fill(color.RGBA{40, 40, 60, 230})
			opts := &ebiten.DrawImageOptions{}
			opts.GeoM.Translate(float64(x), float64(y))
			screen.DrawImage(menuBg, opts)
			// Border
			drawRectBorder(screen, x, y, menuW, menuH, 2, color.RGBA{120, 120, 160, 255})
			// Entry text
			ebitenutil.DebugPrintAt(screen, "Discard", x+8, y+5)

			// Handle clicks on context menu
			if ui.game.consumeLeftClickIn(x, y, x+menuW, y+menuH) {
				// Discard clicked
				idx := ui.inventoryContextIndex
				if idx >= 0 && idx < len(ui.game.party.Inventory) {
					item := ui.game.party.Inventory[idx]
					if item.Type == items.ItemQuest {
						ui.game.AddCombatMessage(fmt.Sprintf("Cannot discard %s.", item.Name))
					} else {
						ui.game.party.RemoveItem(idx)
						ui.game.AddCombatMessage(fmt.Sprintf("Discarded %s.", item.Name))
					}
				}
				ui.inventoryContextOpen = false
			} else if ui.game.consumeLeftClick() {
				// Close the context menu on any left click
				ui.inventoryContextOpen = false
			}

			// Mouse state is updated once per frame in updateMouseState().
		}
	}

	// Instructions
	instructionY := contentY + contentHeight - 55
	ebitenutil.DebugPrintAt(screen, "Use 1-4 keys to select different characters and view their equipment", panelX+20, instructionY)
	ebitenutil.DebugPrintAt(screen, "Double-click items in inventory to equip them (red items can't be equipped)", panelX+20, instructionY+15)
	ebitenutil.DebugPrintAt(screen, "Double-click equipped items to unequip them back to inventory", panelX+20, instructionY+30)
	ebitenutil.DebugPrintAt(screen, "Right-click an inventory item to discard it", panelX+20, instructionY+45)
}

// drawCharactersContent draws the characters tab content
func (ui *UISystem) drawCharactersContent(screen *ebiten.Image, panelX, contentY int) {
	// Title
	ebitenutil.DebugPrintAt(screen, "=== PARTY MEMBERS ===", panelX+20, contentY+10)

	// Party members (2 per row)
	memberY := contentY + 40
	cardWidth := 300
	cardHeight := 190 // Increased from 147 to fit all text properly
	for i, member := range ui.game.party.Members {
		col := i % 2
		row := i / 2
		x := panelX + 15 + (col * (cardWidth + 20))
		y := memberY + (row * (cardHeight + 15))

		// Handle mouse clicks on character cards
		ui.handleCharacterCardClick(x, y, cardWidth, cardHeight, i)

		// Member background (larger card) with hover effect
		memberBg := ebiten.NewImage(cardWidth, cardHeight)
		var bgColor color.RGBA

		// Check if mouse is hovering over this card
		mouseX, mouseY := ebiten.CursorPosition()
		isHovering := mouseX >= x && mouseX < x+cardWidth && mouseY >= y && mouseY < y+cardHeight

		if i == ui.game.selectedChar {
			bgColor = color.RGBA{40, 40, 80, 180} // Highlight selected character
		} else if isHovering {
			bgColor = color.RGBA{30, 30, 50, 140} // Hover effect
		} else {
			bgColor = color.RGBA{20, 20, 40, 120} // Normal state
		}

		memberBg.Fill(bgColor)

		memberOpts := &ebiten.DrawImageOptions{}
		memberOpts.GeoM.Translate(float64(x), float64(y))
		screen.DrawImage(memberBg, memberOpts)

		// Member header info
		memberInfo := fmt.Sprintf("%d. %s (%s) Level %d",
			i+1, member.Name, ui.getClassName(member.Class), member.Level)
		ebitenutil.DebugPrintAt(screen, memberInfo, x+8, y+8)

		// Health and spell points
		healthInfo := fmt.Sprintf("Health: %d/%d", member.HitPoints, member.MaxHitPoints)
		spellInfo := fmt.Sprintf("Spell Points: %d/%d", member.SpellPoints, member.MaxSpellPoints)
		ebitenutil.DebugPrintAt(screen, healthInfo, x+8, y+25)
		ebitenutil.DebugPrintAt(screen, spellInfo, x+8, y+40)

		// Experience
		expInfo := fmt.Sprintf("Experience: %d", member.Experience)
		ebitenutil.DebugPrintAt(screen, expInfo, x+8, y+55)

		// Primary stats (split into two columns for better readability)
		ebitenutil.DebugPrintAt(screen, "--- STATS ---", x+8, y+70)

		// Left column stats
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Might: %d", member.Might), x+8, y+85)
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Intellect: %d", member.Intellect), x+8, y+100)
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Personality: %d", member.Personality), x+8, y+115)

		// Right column stats
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Endurance: %d", member.Endurance), x+160, y+85)
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Speed: %d", member.Speed), x+160, y+100)
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Accuracy: %d", member.Accuracy), x+160, y+115)
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Luck: %d", member.Luck), x+160, y+130)

		// Equipment section
		ebitenutil.DebugPrintAt(screen, "--- EQUIPMENT ---", x+8, y+145)

		// Main hand weapon
		if weapon, hasWeapon := member.Equipment[items.SlotMainHand]; hasWeapon {
			weaponName := weapon.Name
			if len(weaponName) > 16 { // Truncate weapon name if too long
				weaponName = weaponName[:13] + "..."
			}
			weaponText := fmt.Sprintf("Weapon: %s", weaponName)
			ebitenutil.DebugPrintAt(screen, weaponText, x+8, y+160)
		} else {
			ebitenutil.DebugPrintAt(screen, "Weapon: None", x+8, y+160)
		}

		// Equipped spell (unified slot)
		if spell, hasSpell := member.Equipment[items.SlotSpell]; hasSpell {
			spellName := spell.Name
			if len(spellName) > 16 { // Truncate spell name if too long
				spellName = spellName[:13] + "..."
			}
			spellText := fmt.Sprintf("Spell: %s", spellName)
			ebitenutil.DebugPrintAt(screen, spellText, x+160, y+160)
		} else {
			ebitenutil.DebugPrintAt(screen, "Spell: None", x+160, y+160)
		}

		// Status effects (moved down to make room)
		statusText := "Status: Normal"
		if len(member.Conditions) > 0 {
			statusText = fmt.Sprintf("Status: %s", ui.getConditionName(member.Conditions[0]))
			// Additional conditions if multiple
			if len(member.Conditions) > 1 {
				statusText += fmt.Sprintf(" +%d more", len(member.Conditions)-1)
			}
		}
		ebitenutil.DebugPrintAt(screen, statusText, x+8, y+175)
	}

	// Instructions
	// instructionY := contentY + contentHeight - 30
	// ebitenutil.DebugPrintAt(screen, "Click character cards or use 1-4 keys to select • Equipment updates automatically", panelX+20, instructionY)
}

// drawSpellbookContent draws the spellbook tab content
func (ui *UISystem) drawSpellbookContent(screen *ebiten.Image, panelX, contentY, contentHeight int) {
	currentChar := ui.game.party.Members[ui.game.selectedChar]
	schools := currentChar.GetAvailableSchools()

	// Validate and fix selected school index if it's out of bounds
	if ui.game.selectedSchool >= len(schools) {
		ui.game.selectedSchool = 0
		ui.game.selectedSpell = 0
	}

	// Title
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("=== %s's SPELLBOOK ===", currentChar.Name), panelX+20, contentY+10)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Spell Points: %d/%d",
		currentChar.SpellPoints, currentChar.MaxSpellPoints), panelX+20, contentY+30)

	if len(schools) == 0 {
		ebitenutil.DebugPrintAt(screen, "No magic schools available", panelX+30, contentY+60)
		return
	}

	// Draw schools and spells
	y := contentY + 60
	var spellTooltip string
	var tooltipX, tooltipY int

	for schoolIndex, school := range schools {
		schoolName := ui.getSchoolName(school)
		schoolSpells := currentChar.GetSpellsForSchool(school)

		// Handle mouse clicks on school names
		ui.handleSpellbookSchoolClick(panelX+30, y, 300, 20, schoolIndex, school)

		// Draw school name
		if schoolIndex == ui.game.selectedSchool {
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("> %s School:", schoolName), panelX+30, y)
		} else {
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("  %s School:", schoolName), panelX+30, y)
		}
		y += 20

		// Validate and fix selected spell index if it's out of bounds
		if schoolIndex == ui.game.selectedSchool && ui.game.selectedSpell >= len(schoolSpells) {
			ui.game.selectedSpell = 0
		}

		// Draw spells for this school (unless collapsed)
		if ui.game.collapsedSpellSchools[school] {
			continue
		}

		for spellIndex, spellID := range schoolSpells {
			// Get spell definition from centralized system
			def, err := spells.GetSpellDefinitionByID(spellID)
			if err != nil {
				continue // Skip invalid spells
			}

			canCast := "✓"
			if currentChar.SpellPoints < def.SpellPointsCost {
				canCast = "✗"
			}

			// Handle mouse interactions for spells
			spellY := y
			spellHeight := 15
			mouseX, mouseY := ebiten.CursorPosition()
			isHovering := mouseX >= panelX+50 && mouseX < panelX+350 && mouseY >= spellY && mouseY < spellY+spellHeight

			// Handle mouse clicks on spells
			ui.handleSpellbookSpellClick(panelX+50, spellY, 300, spellHeight, schoolIndex, spellIndex)

			// Generate tooltip for hovering spell
			if isHovering {
				// Draw hover background
				hoverBg := ebiten.NewImage(300, spellHeight)
				hoverBg.Fill(color.RGBA{100, 100, 150, 100})
				hoverOpts := &ebiten.DrawImageOptions{}
				hoverOpts.GeoM.Translate(float64(panelX+50), float64(spellY))
				screen.DrawImage(hoverBg, hoverOpts)

				// Generate spell tooltip using SpellID
				spellTooltip = GetSpellTooltip(spellID, currentChar, ui.game.combat)
				tooltipX = mouseX + 16
				tooltipY = mouseY + 8
			}

			if schoolIndex == ui.game.selectedSchool && spellIndex == ui.game.selectedSpell {
				ebitenutil.DebugPrintAt(screen, fmt.Sprintf("  > %s %s (SP:%d)",
					canCast, def.Name, def.SpellPointsCost), panelX+50, y)
			} else {
				ebitenutil.DebugPrintAt(screen, fmt.Sprintf("    %s %s (SP:%d)",
					canCast, def.Name, def.SpellPointsCost), panelX+50, y)
			}
			y += 15
		}
	}

	// Draw spell tooltip if hovering over a spell
	if spellTooltip != "" {
		lines := strings.Split(spellTooltip, "\n")
		drawTooltip(screen, lines, tooltipX, tooltipY)
	}

	// Draw spellbook controls
	ebitenutil.DebugPrintAt(screen, "Up/Down: Navigate, Enter: Equip, Click: Select, Double-click: Cast", panelX+30, contentY+contentHeight-30)
}

// drawFPSCounter draws the FPS counter in the top-right corner
func (ui *UISystem) drawFPSCounter(screen *ebiten.Image) {
	// Use Ebiten's built-in FPS counter which is more reliable
	fps := ebiten.ActualFPS()
	tps := ebiten.ActualTPS()

	// Format FPS text
	lines := []string{
		fmt.Sprintf("FPS: %.1f", fps),
		fmt.Sprintf("TPS: %.1f", tps),
	}

	compassX, compassY := ui.getCompassCenter()
	compassRadius := ui.game.config.UI.CompassRadius
	_ = compassX
	lineHeight := 16
	padding := 6
	maxLen := 0
	for _, line := range lines {
		if len(line) > maxLen {
			maxLen = len(line)
		}
	}
	barWidth := maxLen*7 + padding*2
	barHeight := len(lines)*lineHeight + padding*2
	screenWidth := ui.game.config.GetScreenWidth()
	barX := screenWidth - barWidth - 10
	barY := compassY + compassRadius + 10

	bgImg := ebiten.NewImage(barWidth, barHeight)
	bgImg.Fill(color.RGBA{0, 0, 0, 120})
	bgOpts := &ebiten.DrawImageOptions{}
	bgOpts.GeoM.Translate(float64(barX), float64(barY))
	screen.DrawImage(bgImg, bgOpts)

	for i, line := range lines {
		ebitenutil.DebugPrintAt(screen, line, barX+padding, barY+padding+i*lineHeight)
	}
}

// Helper methods

// getAvailableSpellKeys returns the list of spell keys available from the current NPC in deterministic order
func (ui *UISystem) getAvailableSpellKeys() []string {
	if ui.game.dialogNPC == nil || ui.game.dialogNPC.SpellData == nil {
		return []string{}
	}

	keys := make([]string, 0, len(ui.game.dialogNPC.SpellData))
	for key := range ui.game.dialogNPC.SpellData {
		keys = append(keys, key)
	}

	// Sort keys to ensure deterministic ordering and prevent UI blinking
	sort.Strings(keys)

	return keys
}

func (ui *UISystem) getSchoolName(school character.MagicSchool) string {
	names := map[character.MagicSchool]string{
		character.MagicBody:   "Body",
		character.MagicMind:   "Mind",
		character.MagicSpirit: "Spirit",
		character.MagicFire:   "Fire",
		character.MagicWater:  "Water",
		character.MagicAir:    "Air",
		character.MagicEarth:  "Earth",
		character.MagicLight:  "Light",
		character.MagicDark:   "Dark",
	}
	if name, exists := names[school]; exists {
		return name
	}
	return "Unknown"
}

// isMouseOverCharacter checks if the mouse cursor is over a specific character portrait
func (ui *UISystem) isMouseOverCharacter(mouseX, mouseY, charIndex, portraitWidth, portraitHeight, startY int) bool {
	charX := charIndex * portraitWidth
	return mouseX >= charX && mouseX < charX+portraitWidth &&
		mouseY >= startY && mouseY < startY+portraitHeight
}

// getClassName returns the class name for a character class
func (ui *UISystem) getClassName(class character.CharacterClass) string {
	names := map[character.CharacterClass]string{
		character.ClassKnight:   "Knight",
		character.ClassPaladin:  "Paladin",
		character.ClassArcher:   "Archer",
		character.ClassCleric:   "Cleric",
		character.ClassSorcerer: "Sorcerer",
		character.ClassDruid:    "Druid",
	}
	if name, exists := names[class]; exists {
		return name
	}
	return "Unknown"
}

// getConditionName returns the condition name for a character condition
func (ui *UISystem) getConditionName(condition character.Condition) string {
	names := map[character.Condition]string{
		character.ConditionNormal:      "Normal",
		character.ConditionPoisoned:    "Poisoned",
		character.ConditionDiseased:    "Diseased",
		character.ConditionCursed:      "Cursed",
		character.ConditionAsleep:      "Asleep",
		character.ConditionFear:        "Fear",
		character.ConditionParalyzed:   "Paralyzed",
		character.ConditionUnconscious: "Unconscious",
		character.ConditionDead:        "Dead",
		character.ConditionStone:       "Stone",
		character.ConditionEradicated:  "Eradicated",
	}
	if name, exists := names[condition]; exists {
		return name
	}
	return "Unknown"
}

// drawCombatMessages draws the last 3 combat messages in the bottom-right corner above the party
func (ui *UISystem) drawCombatMessages(screen *ebiten.Image) {
	messages := ui.game.GetCombatMessages()
	if len(messages) == 0 {
		return
	}

	// Position messages in the bottom-right corner, above the party UI
	screenWidth := ui.game.config.GetScreenWidth()
	screenHeight := ui.game.config.GetScreenHeight()
	portraitHeight := ui.game.config.UI.PartyPortraitHeight

	// Start from just above the party UI
	startY := screenHeight - portraitHeight - 80 // 80px above party UI
	messageSpacing := 18                         // Space between messages
	messageWidth := 400                          // Width of message area
	startX := screenWidth - messageWidth - 10    // 10px from right edge

	// Draw semi-transparent background for the message area
	bgHeight := len(messages)*messageSpacing + 10
	bgImg := ebiten.NewImage(messageWidth, bgHeight)
	bgImg.Fill(color.RGBA{0, 0, 0, 150}) // Semi-transparent black background

	bgOpts := &ebiten.DrawImageOptions{}
	bgOpts.GeoM.Translate(float64(startX-5), float64(startY-5))
	screen.DrawImage(bgImg, bgOpts)

	// Draw messages from top to bottom (most recent at bottom)
	for i, message := range messages {
		textY := startY + (i * messageSpacing)
		ebitenutil.DebugPrintAt(screen, message, startX, textY)
	}
}

// drawNPCDialog draws the NPC dialog interface for different NPC types
func (ui *UISystem) drawNPCDialog(screen *ebiten.Image) {
	if ui.game.dialogNPC == nil {
		return
	}

	screenWidth := ui.game.config.GetScreenWidth()
	screenHeight := ui.game.config.GetScreenHeight()

	// Dialog dimensions
	dialogWidth := 600
	dialogHeight := 400
	dialogX := (screenWidth - dialogWidth) / 2
	dialogY := (screenHeight - dialogHeight) / 2

	// Draw semi-transparent overlay
	overlayImg := ebiten.NewImage(screenWidth, screenHeight)
	overlayImg.Fill(color.RGBA{0, 0, 0, 128})
	screen.DrawImage(overlayImg, &ebiten.DrawImageOptions{})

	// Draw dialog background
	dialogImg := ebiten.NewImage(dialogWidth, dialogHeight)
	dialogImg.Fill(color.RGBA{40, 40, 60, 255})

	dialogOpts := &ebiten.DrawImageOptions{}
	dialogOpts.GeoM.Translate(float64(dialogX), float64(dialogY))
	screen.DrawImage(dialogImg, dialogOpts)

	// Draw border
	borderColor := color.RGBA{100, 100, 120, 255}
	borderThickness := 3
	drawRectBorder(screen, dialogX, dialogY, dialogWidth, dialogHeight, borderThickness, borderColor)

	// Handle different NPC types
	switch ui.game.dialogNPC.Type {
	case "encounter":
		ui.drawEncounterDialog(screen, dialogX, dialogY, dialogWidth, dialogHeight)
	case "spell_trader":
		ui.drawSpellTraderDialog(screen, dialogX, dialogY, dialogHeight)
	case "merchant":
		ui.drawMerchantDialog(screen, dialogX, dialogY, dialogWidth, dialogHeight)
	default:
		ui.drawGenericDialog(screen, dialogX, dialogY, dialogHeight)
	}
}

// drawEncounterDialog draws dialog for encounter NPCs
func (ui *UISystem) drawEncounterDialog(screen *ebiten.Image, dialogX, dialogY, dialogWidth, _ int) {
	npc := ui.game.dialogNPC

	// Draw title
	titleText := npc.Name
	ebitenutil.DebugPrintAt(screen, titleText, dialogX+20, dialogY+20)

	// Draw encounter description
	if npc.DialogueData != nil {
		// Wrap long text
		greeting := npc.DialogueData.Greeting
		lines := ui.wrapText(greeting, 70)
		for i, line := range lines {
			ebitenutil.DebugPrintAt(screen, line, dialogX+20, dialogY+50+i*16)
		}

		// Draw choices if this is first visit or encounter is repeatable
		if !npc.Visited || (npc.EncounterData != nil && !npc.EncounterData.FirstVisitOnly) {
			choicesY := dialogY + 50 + len(lines)*16 + 20

			if npc.DialogueData.ChoicePrompt != "" {
				ebitenutil.DebugPrintAt(screen, npc.DialogueData.ChoicePrompt, dialogX+20, choicesY)
				choicesY += 20
			}

			for i, choice := range npc.DialogueData.Choices {
				choiceY := choicesY + i*25
				choiceText := fmt.Sprintf("%d. %s", i+1, choice.Text)

				// Highlight selected choice
				if i == ui.game.selectedChoice {
					choiceBg := ebiten.NewImage(dialogWidth-40, 20)
					choiceBg.Fill(color.RGBA{100, 100, 0, 128})
					choiceBgOpts := &ebiten.DrawImageOptions{}
					choiceBgOpts.GeoM.Translate(float64(dialogX+20), float64(choiceY-2))
					screen.DrawImage(choiceBg, choiceBgOpts)
				}

				ebitenutil.DebugPrintAt(screen, choiceText, dialogX+25, choiceY)
			}
		} else {
			// Already visited
			ebitenutil.DebugPrintAt(screen, "The shipwreck appears empty now.", dialogX+20, dialogY+150)
			ebitenutil.DebugPrintAt(screen, "Press ESC to leave.", dialogX+20, dialogY+180)
		}
	}
}

// drawSpellTraderDialog draws dialog for spell trader NPCs
func (ui *UISystem) drawSpellTraderDialog(screen *ebiten.Image, dialogX, dialogY, dialogHeight int) {
	// Draw title
	titleText := fmt.Sprintf("Spell Trader - %s", ui.game.dialogNPC.Name)
	ebitenutil.DebugPrintAt(screen, titleText, dialogX+20, dialogY+20)

	// Draw greeting
	greetingText := "Welcome! I can teach you powerful spells for gold."
	ebitenutil.DebugPrintAt(screen, greetingText, dialogX+20, dialogY+50)

	// Draw party gold
	goldText := fmt.Sprintf("Party Gold: %d", ui.game.party.Gold)
	ebitenutil.DebugPrintAt(screen, goldText, dialogX+400, dialogY+20)

	// Draw character selection header
	ebitenutil.DebugPrintAt(screen, "Select Character:", dialogX+20, dialogY+80)

	// Get currently selected spell for eligibility checking
	var selectedSpell *character.NPCSpell
	if ui.game.selectedSpellKey != "" {
		selectedSpell = ui.game.dialogNPC.SpellData[ui.game.selectedSpellKey]
	}

	// Draw character list
	for i, member := range ui.game.party.Members {
		y := dialogY + 100 + (i * UIRowSpacing)
		charText := fmt.Sprintf("%d. %s (Level %d %s)", i+1, member.Name, member.Level, member.GetClassName())

		// Check if character can learn the selected spell
		canLearn := selectedSpell != nil && ui.characterCanLearnSpell(member, selectedSpell)
		alreadyKnows := selectedSpell != nil && ui.characterKnowsSpell(member, selectedSpell.Name)

		// Color coding and text based on eligibility
		bgColor, statusText := ui.getCharacterSpellStatus(i, canLearn, alreadyKnows, selectedSpell != nil)
		charText += statusText

		// Draw background if needed
		ui.drawUIBackground(screen, dialogX+15, y-2, UICharacterBackgroundWidth, UIRowHeight, bgColor)

		ebitenutil.DebugPrintAt(screen, charText, dialogX+20, y)
	}

	// Draw spells section
	spellsY := dialogY + 100 + (len(ui.game.party.Members) * UIRowSpacing) + 20
	ebitenutil.DebugPrintAt(screen, "Available Spells:", dialogX+20, spellsY)

	// Draw spell list using deterministic ordering
	spellKeys := ui.getAvailableSpellKeys()
	for spellIndex, spellKey := range spellKeys {
		npcSpell := ui.game.dialogNPC.SpellData[spellKey]
		y := spellsY + 20 + (spellIndex * UIRowSpacing)
		spellText := fmt.Sprintf("%d. %s - %d gold", spellIndex+1, npcSpell.Name, npcSpell.Cost)

		// Check if character already knows this spell
		if ui.game.selectedCharIdx >= 0 && ui.game.selectedCharIdx < len(ui.game.party.Members) {
			selectedChar := ui.game.party.Members[ui.game.selectedCharIdx]
			if ui.characterKnowsSpell(selectedChar, npcSpell.Name) {
				spellText += " (Already Known)"
			}
		}

		// Highlight selected spell
		if spellIndex == ui.game.dialogSelectedSpell {
			ui.drawUIBackground(screen, dialogX+15, y-2, UISpellBackgroundWidth, UIRowHeight, UIColorSpellSelection)
		}

		ebitenutil.DebugPrintAt(screen, spellText, dialogX+20, y)
	}

	// Draw instructions
	instructionsY := dialogY + dialogHeight - 95
	ebitenutil.DebugPrintAt(screen, "Arrow Keys: Navigate  |  Mouse: Click to Select", dialogX+20, instructionsY)
	ebitenutil.DebugPrintAt(screen, "Enter or Double-Click: Purchase Spell", dialogX+20, instructionsY+15)
	ebitenutil.DebugPrintAt(screen, "Escape: Close Dialog", dialogX+20, instructionsY+30)
	ebitenutil.DebugPrintAt(screen, "Green: Can Learn  |  Red: Cannot Learn  |  Gray: Knows Spell", dialogX+20, instructionsY+45)
}

// drawMerchantDialog draws a simple seller UI to sell party items
func (ui *UISystem) drawMerchantDialog(screen *ebiten.Image, dialogX, dialogY, dialogWidth, dialogHeight int) {
	// Title and greeting
	titleText := fmt.Sprintf("Merchant - %s", ui.game.dialogNPC.Name)
	ebitenutil.DebugPrintAt(screen, titleText, dialogX+20, dialogY+20)
	greeting := "Bring your wares. I pay fair coin."
	ebitenutil.DebugPrintAt(screen, greeting, dialogX+20, dialogY+50)

	// Gold
	goldText := fmt.Sprintf("Party Gold: %d", ui.game.party.Gold)
	ebitenutil.DebugPrintAt(screen, goldText, dialogX+400, dialogY+20)

	// Header
	listY := dialogY + 90
	ebitenutil.DebugPrintAt(screen, "Click an item to sell it: (shows first 15)", dialogX+20, listY)

	// List inventory with values
	startY := listY + 20
	maxItems := 15
	for i := 0; i < len(ui.game.party.Inventory) && i < maxItems; i++ {
		item := ui.game.party.Inventory[i]
		y := startY + i*UIRowSpacing
		// Price from attributes
		price := item.Attributes["value"]
		line := fmt.Sprintf("%2d. %-24s  %4d gold", i+1, item.Name, price)

		// Hover effect
		mouseX, mouseY := ebiten.CursorPosition()
		isHover := mouseX >= dialogX+18 && mouseX <= dialogX+dialogWidth-18 && mouseY >= y-2 && mouseY <= y-2+UIRowHeight
		if isHover {
			ui.drawUIBackground(screen, dialogX+15, y-2, dialogWidth-30, UIRowHeight, color.RGBA{40, 80, 40, 120})
		}
		ebitenutil.DebugPrintAt(screen, line, dialogX+20, y)
	}

	// Instructions
	instructionsY := dialogY + dialogHeight - 60
	ebitenutil.DebugPrintAt(screen, "Double-click item: Sell  |  ESC: Close", dialogX+20, instructionsY)
}

// drawGenericDialog draws basic dialog for other NPC types
func (ui *UISystem) drawGenericDialog(screen *ebiten.Image, dialogX, dialogY, _ int) {
	npc := ui.game.dialogNPC

	// Draw title
	titleText := npc.Name
	ebitenutil.DebugPrintAt(screen, titleText, dialogX+20, dialogY+20)

	// Draw basic greeting
	if npc.DialogueData != nil && npc.DialogueData.Greeting != "" {
		lines := ui.wrapText(npc.DialogueData.Greeting, 70)
		for i, line := range lines {
			ebitenutil.DebugPrintAt(screen, line, dialogX+20, dialogY+50+i*16)
		}
	}

	ebitenutil.DebugPrintAt(screen, "Press ESC to close", dialogX+20, dialogY+200)
}

// drawGameOverOverlay draws a simple game over screen with options
func (ui *UISystem) drawGameOverOverlay(screen *ebiten.Image) {
	w := ui.game.config.GetScreenWidth()
	h := ui.game.config.GetScreenHeight()
	// Darken background
	overlay := ebiten.NewImage(w, h)
	overlay.Fill(color.RGBA{0, 0, 0, 180})
	screen.DrawImage(overlay, &ebiten.DrawImageOptions{})

	// Text
	centerX := w/2 - 160
	centerY := h/2 - 30
	ebitenutil.DebugPrintAt(screen, "GAME OVER", centerX+80, centerY-30)
	ebitenutil.DebugPrintAt(screen, "Press N: New Game", centerX, centerY)
	ebitenutil.DebugPrintAt(screen, "Press L: Load Game", centerX, centerY+20)
}

// drawVictoryOverlay draws the victory screen with score and options
func (ui *UISystem) drawVictoryOverlay(screen *ebiten.Image) {
	w := ui.game.config.GetScreenWidth()
	h := ui.game.config.GetScreenHeight()

	// Darken background with golden tint
	overlay := ebiten.NewImage(w, h)
	overlay.Fill(color.RGBA{30, 25, 0, 200})
	screen.DrawImage(overlay, &ebiten.DrawImageOptions{})

	// Get score data
	scoreData := ui.game.GetScoreData()
	finalScore := CalculateScore(scoreData)
	playTimeStr := FormatPlayTime(scoreData.PlayTime)

	centerX := w / 2
	startY := h/2 - 120

	// Victory header
	ebitenutil.DebugPrintAt(screen, "=== VICTORY! ===", centerX-70, startY)
	ebitenutil.DebugPrintAt(screen, "You have slain all four dragons!", centerX-120, startY+25)
	ebitenutil.DebugPrintAt(screen, "The realm is saved!", centerX-70, startY+45)

	// Score details
	ebitenutil.DebugPrintAt(screen, "--- Final Score ---", centerX-75, startY+80)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Score: %d", finalScore), centerX-50, startY+100)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Gold: %d", scoreData.Gold), centerX-50, startY+120)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Experience: %d", scoreData.TotalExperience), centerX-70, startY+140)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Avg Level: %d", scoreData.AverageLevel), centerX-55, startY+160)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Time: %s", playTimeStr), centerX-50, startY+180)

	// Instructions
	if !ui.game.victoryScoreSaved {
		ebitenutil.DebugPrintAt(screen, "Enter your name:", centerX-60, startY+220)
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("> %s_", ui.game.victoryNameInput), centerX-80, startY+240)
		ebitenutil.DebugPrintAt(screen, "Press ENTER to save score", centerX-90, startY+270)
		ebitenutil.DebugPrintAt(screen, "Press ESC for main menu", centerX-85, startY+290)
	} else {
		ebitenutil.DebugPrintAt(screen, "Score saved!", centerX-45, startY+220)
		ebitenutil.DebugPrintAt(screen, "Press H to view High Scores", centerX-100, startY+250)
		ebitenutil.DebugPrintAt(screen, "Press ESC for main menu", centerX-85, startY+270)
	}
}

// drawHighScoresOverlay draws the high scores table
func (ui *UISystem) drawHighScoresOverlay(screen *ebiten.Image) {
	w := ui.game.config.GetScreenWidth()
	h := ui.game.config.GetScreenHeight()

	// Darken background
	overlay := ebiten.NewImage(w, h)
	overlay.Fill(color.RGBA{0, 0, 30, 220})
	screen.DrawImage(overlay, &ebiten.DrawImageOptions{})

	scores, err := LoadHighScores()
	if err != nil {
		ebitenutil.DebugPrintAt(screen, "Error loading high scores", w/2-90, h/2)
		return
	}

	centerX := w / 2
	startY := 60

	// Header
	ebitenutil.DebugPrintAt(screen, "=== HIGH SCORES ===", centerX-75, startY)

	// Column headers
	ebitenutil.DebugPrintAt(screen, "Rank  Name           Score    Time", centerX-140, startY+40)
	ebitenutil.DebugPrintAt(screen, "----  ----           -----    ----", centerX-140, startY+55)

	// Entries
	if len(scores.Entries) == 0 {
		ebitenutil.DebugPrintAt(screen, "No scores yet!", centerX-50, startY+80)
	} else {
		for i, entry := range scores.Entries {
			line := fmt.Sprintf("%2d.   %-14s %6d   %s", i+1, truncateName(entry.PlayerName, 14), entry.Score, entry.PlayTime)
			ebitenutil.DebugPrintAt(screen, line, centerX-140, startY+80+i*20)
		}
	}

	// Instructions
	ebitenutil.DebugPrintAt(screen, "Press ESC to close", centerX-70, h-50)
}

// truncateName truncates a name to maxLen characters
func truncateName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	return name[:maxLen-2] + ".."
}

// wrapText wraps text to fit within specified width
func (ui *UISystem) wrapText(text string, maxWidth int) []string {
	if len(text) <= maxWidth {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	currentLine := ""

	for _, word := range words {
		if len(currentLine)+len(word)+1 <= maxWidth {
			if currentLine == "" {
				currentLine = word
			} else {
				currentLine += " " + word
			}
		} else {
			if currentLine != "" {
				lines = append(lines, currentLine)
			}
			currentLine = word
		}
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}

// characterKnowsSpell checks if a character already knows a spell
func (ui *UISystem) characterKnowsSpell(char *character.MMCharacter, spellName string) bool {
	for _, magicSkill := range char.MagicSchools {
		for _, spellID := range magicSkill.KnownSpells {
			if def, err := spells.GetSpellDefinitionByID(spellID); err == nil && def.Name == spellName {
				return true
			}
		}
	}
	return false
}

// characterCanLearnSpell checks if a character can learn a specific spell based on class and magic schools
func (ui *UISystem) characterCanLearnSpell(char *character.MMCharacter, spellData *character.NPCSpell) bool {
	if spellData == nil {
		return false
	}

	// Check class restrictions for magic schools
	switch spellData.School {
	case "water":
		return char.Class == character.ClassSorcerer || char.Class == character.ClassCleric || char.Class == character.ClassDruid
	case "fire":
		return char.Class == character.ClassSorcerer
	case "air":
		return char.Class == character.ClassSorcerer || char.Class == character.ClassArcher
	case "earth":
		return char.Class == character.ClassSorcerer || char.Class == character.ClassDruid
	case "body":
		return char.Class == character.ClassCleric || char.Class == character.ClassDruid
	case "mind":
		return char.Class == character.ClassCleric
	case "spirit":
		return char.Class == character.ClassCleric || char.Class == character.ClassDruid
	case "light":
		return char.Class == character.ClassCleric
	case "dark":
		return false // Dark magic typically not learnable from NPCs
	default:
		return false
	}
}

// drawRectBorder draws a rectangle border of given thickness and color
func drawRectBorder(dst *ebiten.Image, x, y, w, h, thickness int, clr color.Color) {
	// Top border
	top := ebiten.NewImage(w+2*thickness, thickness)
	top.Fill(clr)
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(float64(x-thickness), float64(y-thickness))
	dst.DrawImage(top, opts)
	// Bottom border
	bottom := ebiten.NewImage(w+2*thickness, thickness)
	bottom.Fill(clr)
	opts = &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(float64(x-thickness), float64(y+h))
	dst.DrawImage(bottom, opts)
	// Left border
	left := ebiten.NewImage(thickness, h)
	left.Fill(clr)
	opts = &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(float64(x-thickness), float64(y))
	dst.DrawImage(left, opts)
	// Right border
	right := ebiten.NewImage(thickness, h)
	right.Fill(clr)
	opts = &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(float64(x+w), float64(y))
	dst.DrawImage(right, opts)
}

// handleInventoryItemClick handles double-click to equip items from inventory
func (ui *UISystem) handleInventoryItemClick(itemIndex int, x1, y1, x2, y2 int) {
	if !ui.game.consumeLeftClickIn(x1, y1, x2, y2) {
		return
	}
	currentTime := time.UnixMilli(ui.game.mouseLeftClickAt)

	// Check for double-click (same item clicked within 500ms)
	delta := currentTime.Sub(ui.lastClickTime)
	doubleClick := itemIndex == ui.lastClickedItem && delta < doubleClickWindow
	if doubleClick {
		// Double-click detected - use or equip the item
		if itemIndex < len(ui.game.party.Inventory) {
			item := ui.game.party.Inventory[itemIndex]
			if item.Attributes != nil && item.Attributes["opens_map"] == 1 {
				ui.game.mapOverlayOpen = true
				ui.lastClickedItem = itemIndex
				ui.lastClickTime = currentTime
				return
			}
			switch item.Type {
			case items.ItemConsumable:
				// Delegate to game logic (consumable effects, inventory removal, messages)
				_ = ui.game.UseConsumableFromInventory(itemIndex, ui.game.selectedChar)
			default:
				// Try to equip non-consumables
				itemName := item.Name
				if ui.game.party.EquipItemFromInventory(itemIndex, ui.game.selectedChar) {
					ui.game.AddCombatMessage(fmt.Sprintf("%s equipped %s!",
						ui.game.party.Members[ui.game.selectedChar].Name, itemName))
				} else {
					ui.game.AddCombatMessage("Cannot equip this item!")
				}
			}
		}
	}

	ui.lastClickedItem = itemIndex
	ui.lastClickTime = currentTime
}

// drawMapOverlay renders the current map with NPCs and teleporters.
func (ui *UISystem) drawMapOverlay(screen *ebiten.Image) {
	if ui.game.world == nil {
		ui.game.mapOverlayOpen = false
		return
	}

	screenW := ui.game.config.GetScreenWidth()
	screenH := ui.game.config.GetScreenHeight()

	dim := ebiten.NewImage(screenW, screenH)
	dim.Fill(color.RGBA{0, 0, 0, 140})
	screen.DrawImage(dim, &ebiten.DrawImageOptions{})

	panelW := int(float64(screenW) * 0.75)
	panelH := int(float64(screenH) * 0.75)
	if panelW > 720 {
		panelW = 720
	}
	if panelH > 560 {
		panelH = 560
	}
	if panelW < 320 {
		panelW = 320
	}
	if panelH < 240 {
		panelH = 240
	}
	panelX := (screenW - panelW) / 2
	panelY := (screenH - panelH) / 2

	bg := ebiten.NewImage(panelW, panelH)
	bg.Fill(color.RGBA{20, 20, 40, 230})
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(float64(panelX), float64(panelY))
	screen.DrawImage(bg, opts)
	drawRectBorder(screen, panelX, panelY, panelW, panelH, 2, color.RGBA{100, 100, 160, 255})

	title := "World Map"
	if world.GlobalWorldManager != nil {
		if mapCfg := world.GlobalWorldManager.GetCurrentMapConfig(); mapCfg != nil && mapCfg.Name != "" {
			title = fmt.Sprintf("World Map - %s", mapCfg.Name)
		}
	}
	ebitenutil.DebugPrintAt(screen, title, panelX+16, panelY+12)

	closeX := panelX + panelW - 26
	closeY := panelY + 10
	closeImg := ebiten.NewImage(16, 16)
	closeImg.Fill(color.RGBA{200, 60, 60, 220})
	closeOpts := &ebiten.DrawImageOptions{}
	closeOpts.GeoM.Translate(float64(closeX), float64(closeY))
	screen.DrawImage(closeImg, closeOpts)
	ebitenutil.DebugPrintAt(screen, "X", closeX+4, closeY+2)
	if ui.game.consumeLeftClickIn(closeX, closeY, closeX+16, closeY+16) {
		ui.game.mapOverlayOpen = false
	}

	mapPadding := 18
	mapX := panelX + mapPadding
	mapY := panelY + 36
	mapW := panelW - mapPadding*2
	mapH := panelH - 54

	worldW := ui.game.world.Width
	worldH := ui.game.world.Height
	if worldW <= 0 || worldH <= 0 {
		return
	}

	tileSize := mapW / worldW
	if alt := mapH / worldH; alt < tileSize {
		tileSize = alt
	}
	if tileSize < 2 {
		tileSize = 2
	}

	originX := mapX + (mapW-worldW*tileSize)/2
	originY := mapY + (mapH-worldH*tileSize)/2

	floorColor := color.RGBA{60, 110, 60, 255}
	if world.GlobalWorldManager != nil {
		if mapCfg := world.GlobalWorldManager.GetCurrentMapConfig(); mapCfg != nil {
			floorColor = color.RGBA{uint8(mapCfg.DefaultFloorColor[0]), uint8(mapCfg.DefaultFloorColor[1]), uint8(mapCfg.DefaultFloorColor[2]), 255}
		}
	}

	for y := 0; y < worldH; y++ {
		for x := 0; x < worldW; x++ {
			tile := ui.game.world.Tiles[y][x]
			cellColor := floorColor
			switch tile {
			case world.TileWall, world.TileTree, world.TileAncientTree, world.TileThicket, world.TileMossRock, world.TileLowWall, world.TileHighWall:
				cellColor = color.RGBA{40, 40, 50, 255}
			case world.TileWater:
				cellColor = color.RGBA{40, 90, 160, 255}
			case world.TileDeepWater:
				cellColor = color.RGBA{25, 60, 120, 255}
			case world.TileVioletTeleporter:
				cellColor = color.RGBA{170, 80, 200, 255}
			case world.TileRedTeleporter:
				cellColor = color.RGBA{200, 70, 70, 255}
			}

			drawX := originX + x*tileSize
			drawY := originY + y*tileSize
			vector.DrawFilledRect(screen, float32(drawX), float32(drawY), float32(tileSize), float32(tileSize), cellColor, false)
		}
	}

	// NPCs overlay
	npcColor := color.RGBA{255, 220, 0, 255}
	for _, npc := range ui.game.world.NPCs {
		nx := int(npc.X / float64(ui.game.config.GetTileSize()))
		ny := int(npc.Y / float64(ui.game.config.GetTileSize()))
		if nx < 0 || nx >= worldW || ny < 0 || ny >= worldH {
			continue
		}
		drawX := originX + nx*tileSize
		drawY := originY + ny*tileSize
		size := tileSize
		if size < 3 {
			size = 3
		}
		vector.DrawFilledRect(screen, float32(drawX), float32(drawY), float32(size), float32(size), npcColor, false)
	}

	// Quest markers overlay (top 3 active quests with RGB colors)
	ui.drawQuestMarkersOnMap(screen, originX, originY, tileSize, worldW, worldH)

	// Player position overlay (cyan dot)
	playerTileX := int(ui.game.camera.X / float64(ui.game.config.GetTileSize()))
	playerTileY := int(ui.game.camera.Y / float64(ui.game.config.GetTileSize()))
	if playerTileX >= 0 && playerTileX < worldW && playerTileY >= 0 && playerTileY < worldH {
		drawX := originX + playerTileX*tileSize
		drawY := originY + playerTileY*tileSize
		markerSize := tileSize + 2
		if markerSize < 5 {
			markerSize = 5
		}
		// Draw player as a cyan circle with border
		vector.DrawFilledCircle(screen, float32(drawX)+float32(tileSize)/2, float32(drawY)+float32(tileSize)/2, float32(markerSize)/2, color.RGBA{50, 200, 255, 255}, true)
		vector.StrokeCircle(screen, float32(drawX)+float32(tileSize)/2, float32(drawY)+float32(tileSize)/2, float32(markerSize)/2, 1, color.RGBA{255, 255, 255, 255}, true)
	}
}

// drawQuestMarkersOnMap draws quest objective markers on the map overlay
// Shows top 3 active quests with RGB color coding (1=Red, 2=Green, 3=Blue)
func (ui *UISystem) drawQuestMarkersOnMap(screen *ebiten.Image, originX, originY, tileSize, worldW, worldH int) {
	if ui.game.questManager == nil {
		return
	}

	// Get active quests (not completed)
	activeQuests := ui.game.questManager.GetActiveQuests()
	if len(activeQuests) == 0 {
		return
	}

	// Limit to top 3 quests
	maxQuests := 3
	if len(activeQuests) < maxQuests {
		maxQuests = len(activeQuests)
	}

	// Quest marker colors: Red, Green, Blue for quests 1, 2, 3
	questColors := []color.RGBA{
		{255, 80, 80, 255},  // Red for quest 1
		{80, 255, 80, 255},  // Green for quest 2
		{80, 80, 255, 255},  // Blue for quest 3
	}

	// Get current map key to filter markers
	currentMapKey := ""
	if world.GlobalWorldManager != nil {
		currentMapKey = world.GlobalWorldManager.CurrentMapKey
	}

	for i := 0; i < maxQuests; i++ {
		quest := activeQuests[i]
		def := quest.Definition

		// Skip quests without marker coordinates
		if def.MarkerX == 0 && def.MarkerY == 0 {
			continue
		}

		// Skip quests for different maps
		if def.MarkerMap != "" && def.MarkerMap != currentMapKey {
			continue
		}

		markerX := def.MarkerX
		markerY := def.MarkerY

		// Validate coordinates are within world bounds
		if markerX < 0 || markerX >= worldW || markerY < 0 || markerY >= worldH {
			continue
		}

		// Calculate screen position
		drawX := originX + markerX*tileSize
		drawY := originY + markerY*tileSize
		markerSize := tileSize + 4
		if markerSize < 8 {
			markerSize = 8
		}

		// Draw quest marker as a diamond shape with number
		centerX := float32(drawX) + float32(tileSize)/2
		centerY := float32(drawY) + float32(tileSize)/2
		halfSize := float32(markerSize) / 2

		// Draw diamond shape using lines
		markerColor := questColors[i]
		vector.StrokeLine(screen, centerX, centerY-halfSize, centerX+halfSize, centerY, 2, markerColor, true)
		vector.StrokeLine(screen, centerX+halfSize, centerY, centerX, centerY+halfSize, 2, markerColor, true)
		vector.StrokeLine(screen, centerX, centerY+halfSize, centerX-halfSize, centerY, 2, markerColor, true)
		vector.StrokeLine(screen, centerX-halfSize, centerY, centerX, centerY-halfSize, 2, markerColor, true)

		// Draw quest number in center
		questNum := fmt.Sprintf("%d", i+1)
		ebitenutil.DebugPrintAt(screen, questNum, int(centerX)-3, int(centerY)-6)
	}
}

// handleEquippedItemClick handles double-click to unequip items from equipment slots
func (ui *UISystem) handleEquippedItemClick(slot items.EquipSlot, x1, y1, x2, y2 int) {
	// Check for mouse click
	if ui.game.consumeLeftClickIn(x1, y1, x2, y2) {
		currentTime := time.UnixMilli(ui.game.mouseLeftClickAt)

		// Check for double-click (same slot clicked within 500ms)
		delta := currentTime.Sub(ui.lastEquipClickTime)
		doubleClick := slot == ui.lastClickedSlot && delta < doubleClickWindow
		if doubleClick {
			// Double-click detected - try to unequip the item
			currentChar := ui.game.party.Members[ui.game.selectedChar]
			if item, exists := currentChar.Equipment[slot]; exists {
				itemName := item.Name
				if ui.game.party.UnequipItemToInventory(slot, ui.game.selectedChar) {
					ui.game.AddCombatMessage(fmt.Sprintf("%s unequipped %s!",
						currentChar.Name, itemName))
				} else {
					ui.game.AddCombatMessage("Cannot unequip this item!")
				}
			}
		}

		ui.lastClickedSlot = slot
		ui.lastEquipClickTime = currentTime
	}

	// Mouse state is updated once per frame in updateMouseState().
}

// drawTurnBasedStatus displays the current game mode and turn state
func (ui *UISystem) drawTurnBasedStatus(screen *ebiten.Image) {
	lines, barX, barY, barWidth, barHeight := ui.turnBasedStatusLayout()
	lineHeight := 16
	padding := 6

	bgImg := ebiten.NewImage(barWidth, barHeight)
	bgImg.Fill(color.RGBA{0, 0, 0, 120})
	bgOpts := &ebiten.DrawImageOptions{}
	bgOpts.GeoM.Translate(float64(barX), float64(barY))
	screen.DrawImage(bgImg, bgOpts)

	for i, line := range lines {
		ebitenutil.DebugPrintAt(screen, line, barX+padding, barY+padding+i*lineHeight)
	}
}

func (ui *UISystem) turnBasedStatusLayout() ([]string, int, int, int, int) {
	mode := "Real-time"
	if ui.game.turnBasedMode {
		mode = "Turn-based"
	}
	lines := []string{fmt.Sprintf("Mode: %s", mode)}
	if ui.game.turnBasedMode {
		turnText := "Party Turn"
		if ui.game.currentTurn == 1 {
			turnText = "Monster Turn"
		}
		lines = append(lines, turnText)
		if ui.game.currentTurn == 0 {
			lines = append(lines, fmt.Sprintf("Actions: %d/2", ui.game.partyActionsUsed))
		}
	}

	lineHeight := 16
	padding := 6
	maxLen := 0
	for _, line := range lines {
		if len(line) > maxLen {
			maxLen = len(line)
		}
	}
	barWidth := maxLen*7 + padding*2
	barHeight := len(lines)*lineHeight + padding*2
	barX := ui.game.config.GetScreenWidth() - barWidth - 10
	barY := 10

	return lines, barX, barY, barWidth, barHeight
}

func (ui *UISystem) getCompassCenter() (int, int) {
	_, _, barY, _, barHeight := ui.turnBasedStatusLayout()
	compassRadius := ui.game.config.UI.CompassRadius
	spacing := 10
	compassX := ui.game.config.GetScreenWidth() - 10 - compassRadius
	compassY := barY + barHeight + spacing + compassRadius
	return compassX, compassY
}

// drawUIBackground draws a colored background rectangle for UI elements (DRY helper)
func (ui *UISystem) drawUIBackground(screen *ebiten.Image, x, y, width, height int, bgColor color.RGBA) {
	if bgColor.A > 0 {
		backgroundImg := ebiten.NewImage(width, height)
		backgroundImg.Fill(bgColor)
		backgroundOpts := &ebiten.DrawImageOptions{}
		backgroundOpts.GeoM.Translate(float64(x), float64(y))
		screen.DrawImage(backgroundImg, backgroundOpts)
	}
}

// getCharacterSpellStatus returns the background color and status text for a character in spell trader dialog (DRY helper)
func (ui *UISystem) getCharacterSpellStatus(charIndex int, canLearn, alreadyKnows, spellSelected bool) (color.RGBA, string) {
	if charIndex == ui.game.selectedCharIdx {
		// Selected character - blue background
		return UIColorSelectedCharacter, ""
	} else if alreadyKnows {
		// Already knows spell - gray background
		return UIColorKnowsSpell, " (Knows Spell)"
	} else if canLearn {
		// Can learn spell - green tint
		return UIColorCanLearn, " (Can Learn)"
	} else if spellSelected {
		// Cannot learn spell - red tint
		return UIColorCannotLearn, " (Cannot Learn)"
	} else {
		// No spell selected - no background
		return color.RGBA{0, 0, 0, 0}, ""
	}
}

// drawInteractionNotification draws a semi-transparent notification when near an interactable NPC
func (ui *UISystem) drawInteractionNotification(screen *ebiten.Image) {
	// Skip if dialog is already active or menu is open
	if ui.game.dialogActive || ui.game.menuOpen {
		return
	}

	// Get the nearest interactable NPC
	nearestNPC := ui.game.GetNearestInteractableNPC()
	if nearestNPC == nil {
		return
	}

	// Calculate screen dimensions for positioning
	screenWidth := ui.game.config.GetScreenWidth()

	// Create interaction message based on NPC type
	var message string
	switch nearestNPC.Type {
	case "spell_trader":
		message = fmt.Sprintf("Press T to talk to %s (Spell Trader)", nearestNPC.Name)
	case "encounter":
		message = fmt.Sprintf("Press T to investigate %s", nearestNPC.Name)
	default:
		message = fmt.Sprintf("Press T to talk to %s", nearestNPC.Name)
	}

	// Calculate text dimensions for background sizing
	textWidth := len(message) * 8 // Approximate character width
	textHeight := 20
	padding := 15

	// Position at top center of screen
	notificationWidth := textWidth + (padding * 2)
	notificationHeight := textHeight + (padding * 2)
	notificationX := (screenWidth - notificationWidth) / 2
	notificationY := 10

	// Draw semi-transparent background
	bgImage := ebiten.NewImage(notificationWidth, notificationHeight)
	bgImage.Fill(color.RGBA{0, 0, 0, 180}) // Semi-transparent black

	bgOpts := &ebiten.DrawImageOptions{}
	bgOpts.GeoM.Translate(float64(notificationX), float64(notificationY))
	screen.DrawImage(bgImage, bgOpts)

	// Draw border for better visibility
	borderColor := color.RGBA{255, 255, 255, 200} // Semi-transparent white
	vector.StrokeRect(
		screen,
		float32(notificationX-1),
		float32(notificationY-1),
		float32(notificationWidth+2),
		float32(notificationHeight+2),
		2,
		borderColor,
		false,
	)

	// Draw the interaction message
	textX := notificationX + padding
	textY := notificationY + padding
	ebitenutil.DebugPrintAt(screen, message, textX, textY)
}

// drawQuestsContent draws the quests tab content
func (ui *UISystem) drawQuestsContent(screen *ebiten.Image, panelX, contentY, contentHeight int) {
	// Title
	ebitenutil.DebugPrintAt(screen, "=== ACTIVE QUESTS ===", panelX+20, contentY+10)

	// Check if quest manager is available
	if ui.game.questManager == nil {
		ebitenutil.DebugPrintAt(screen, "No quests available.", panelX+20, contentY+40)
		return
	}

	allQuests := ui.game.questManager.GetAllQuests()
	if len(allQuests) == 0 {
		ebitenutil.DebugPrintAt(screen, "No active quests.", panelX+20, contentY+40)
		return
	}

	// Sort quests: active first, then completed
	sort.Slice(allQuests, func(i, j int) bool {
		if allQuests[i].Completed != allQuests[j].Completed {
			return !allQuests[i].Completed // Active quests first
		}
		return allQuests[i].Definition.Name < allQuests[j].Definition.Name
	})

	mouseX, mouseY := ebiten.CursorPosition()
	questY := contentY + 40
	questHeight := 95 // Height of each quest entry (increased for wrapped text)
	questWidth := 520

	for _, quest := range allQuests {
		// Draw quest background
		questBg := ebiten.NewImage(questWidth, questHeight)

		// Different colors based on quest status
		var bgColor color.RGBA
		if quest.Completed && !quest.RewardsClaimed {
			bgColor = color.RGBA{40, 80, 40, 200} // Green for completed, reward available
		} else if quest.Completed {
			bgColor = color.RGBA{40, 40, 40, 150} // Gray for completed and claimed
		} else {
			bgColor = color.RGBA{30, 30, 60, 200} // Blue for active
		}
		questBg.Fill(bgColor)

		bgOpts := &ebiten.DrawImageOptions{}
		bgOpts.GeoM.Translate(float64(panelX+20), float64(questY))
		screen.DrawImage(questBg, bgOpts)

		// Draw quest border
		borderColor := color.RGBA{80, 80, 120, 255}
		if quest.Completed && !quest.RewardsClaimed {
			borderColor = color.RGBA{100, 200, 100, 255} // Green border for claimable
		}
		vector.StrokeRect(screen, float32(panelX+20), float32(questY), float32(questWidth), float32(questHeight), 2, borderColor, false)

		// Quest name
		namePrefix := ""
		if quest.Completed {
			namePrefix = "[DONE] "
		}
		ebitenutil.DebugPrintAt(screen, namePrefix+quest.Definition.Name, panelX+30, questY+6)

		// Quest description - wrap to fit within box (max ~70 chars per line)
		descLines := wrapText(quest.Definition.Description, 70)
		for i, line := range descLines {
			if i >= 2 { // Max 2 lines for description
				break
			}
			ebitenutil.DebugPrintAt(screen, line, panelX+30, questY+22+(i*14))
		}

		// Bottom row: Progress on left, Rewards on right
		bottomY := questY + 54

		// Progress for kill quests
		if quest.Definition.Type == "kill" {
			progressText := quest.GetProgressString()
			ebitenutil.DebugPrintAt(screen, progressText, panelX+30, bottomY)

			// Draw progress bar below text
			barX := panelX + 30
			barY := questY + 72
			barWidth := 180
			barHeight := 14

			// Background bar
			barBg := ebiten.NewImage(barWidth, barHeight)
			barBg.Fill(color.RGBA{20, 20, 20, 255})
			barBgOpts := &ebiten.DrawImageOptions{}
			barBgOpts.GeoM.Translate(float64(barX), float64(barY))
			screen.DrawImage(barBg, barBgOpts)

			// Progress fill
			progress := float64(quest.CurrentCount) / float64(quest.Definition.TargetCount)
			if progress > 1 {
				progress = 1
			}
			fillWidth := int(float64(barWidth) * progress)
			if fillWidth > 0 {
				var fillColor color.RGBA
				if quest.Completed {
					fillColor = color.RGBA{80, 200, 80, 255} // Green when complete
				} else {
					fillColor = color.RGBA{80, 150, 200, 255} // Blue while in progress
				}
				barFill := ebiten.NewImage(fillWidth, barHeight)
				barFill.Fill(fillColor)
				barFillOpts := &ebiten.DrawImageOptions{}
				barFillOpts.GeoM.Translate(float64(barX), float64(barY))
				screen.DrawImage(barFill, barFillOpts)
			}

			// Progress bar border
			vector.StrokeRect(screen, float32(barX), float32(barY), float32(barWidth), float32(barHeight), 1, color.RGBA{100, 100, 100, 255}, false)
		} else if quest.Definition.Type == "encounter" {
			// Encounter quests show objective text instead of progress bar
			var objectiveText string
			if quest.Completed {
				objectiveText = "All enemies defeated!"
			} else {
				objectiveText = "Defeat all enemies"
			}
			ebitenutil.DebugPrintAt(screen, objectiveText, panelX+30, bottomY)
		}

		// Rewards section (right side)
		rewardsX := panelX + 300
		rewardsText := fmt.Sprintf("Reward: %dg / %dxp", quest.Definition.Rewards.Gold, quest.Definition.Rewards.Experience)
		ebitenutil.DebugPrintAt(screen, rewardsText, rewardsX, bottomY)

		// Claim button for completed quests with unclaimed rewards
		if quest.Completed && !quest.RewardsClaimed {
			buttonX := rewardsX
			buttonY := questY + 72
			buttonWidth := 110
			buttonHeight := 16

			isHovering := isMouseHoveringBox(mouseX, mouseY, buttonX, buttonY, buttonX+buttonWidth, buttonY+buttonHeight)

			buttonBg := ebiten.NewImage(buttonWidth, buttonHeight)
			if isHovering {
				buttonBg.Fill(color.RGBA{100, 200, 100, 255}) // Bright green on hover
			} else {
				buttonBg.Fill(color.RGBA{60, 150, 60, 255}) // Green
			}
			buttonOpts := &ebiten.DrawImageOptions{}
			buttonOpts.GeoM.Translate(float64(buttonX), float64(buttonY))
			screen.DrawImage(buttonBg, buttonOpts)

			ebitenutil.DebugPrintAt(screen, "Claim Reward", buttonX+12, buttonY+2)

			// Handle click on claim button
			if ui.game.consumeLeftClickIn(buttonX, buttonY, buttonX+buttonWidth, buttonY+buttonHeight) {
				ui.claimQuestReward(quest.ID)
			}
		}

		questY += questHeight + 8
	}
}

// wrapText wraps text to fit within a given character width
func wrapText(text string, maxChars int) []string {
	if len(text) <= maxChars {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	currentLine := ""

	for _, word := range words {
		if currentLine == "" {
			currentLine = word
		} else if len(currentLine)+1+len(word) <= maxChars {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}

// claimQuestReward claims the reward for a completed quest
func (ui *UISystem) claimQuestReward(questID string) {
	if ui.game.questManager == nil {
		return
	}

	rewards, err := ui.game.questManager.ClaimRewards(questID)
	if err != nil {
		ui.game.AddCombatMessage(fmt.Sprintf("Cannot claim reward: %s", err.Error()))
		return
	}

	// Award gold
	if rewards.Gold > 0 {
		ui.game.party.Gold += rewards.Gold
	}

	// Award experience to all living party members
	if rewards.Experience > 0 {
		for _, member := range ui.game.party.Members {
			if member.HitPoints > 0 {
				member.Experience += rewards.Experience
				ui.game.combat.checkLevelUp(member)
			}
		}
	}

	quest := ui.game.questManager.GetQuest(questID)
	if quest != nil {
		ui.game.AddCombatMessage(fmt.Sprintf("Quest '%s' completed! Received %d gold and %d XP!",
			quest.Definition.Name, rewards.Gold, rewards.Experience))
	}
}
