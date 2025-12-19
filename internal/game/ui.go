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

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

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
	game                       *MMGame
	justOpenedStatPopup        bool
	lastClickTime              time.Time
	lastClickedItem            int
	inventoryMousePressed      bool
	inventoryRightMousePressed bool
	inventoryContextOpen       bool
	inventoryContextX          int
	inventoryContextY          int
	inventoryContextIndex      int
	lastEquipClickTime         time.Time
	lastClickedSlot            items.EquipSlot
	equipMousePressed          bool
	utilitySpellMousePressed   bool
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

	// Draw stat distribution popup if open
	if ui.game.statPopupOpen {
		ui.drawStatDistributionPopup(screen)
	}
}

type statMeta struct {
	Name string
	Ptr  *int
}

// drawStatPointRow draws a single stat row with name, value, and + button
func drawStatPointRow(screen *ebiten.Image, name string, valuePtr *int, y, plusX, plusY, btnW, btnH int, canAdd, isHover, mousePressed *bool) bool {
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s: %d", name, *valuePtr), plusX-148, y)
	plusImg := ebiten.NewImage(btnW, btnH)
	if *canAdd && *isHover {
		plusImg.Fill(color.RGBA{80, 200, 80, 220})
	} else {
		plusImg.Fill(color.RGBA{60, 120, 60, 180})
	}
	plusOpts := &ebiten.DrawImageOptions{}
	plusOpts.GeoM.Translate(float64(plusX), float64(plusY))
	screen.DrawImage(plusImg, plusOpts)
	ebitenutil.DebugPrintAt(screen, "+", plusX+8, plusY+4)
	// Handle click
	if *canAdd && *isHover && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && !*mousePressed {
		(*valuePtr)++
		*canAdd = false // Only allow one per click
		*mousePressed = true
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
		if drawStatPointRow(screen, stat.Name, stat.Ptr, y, plusX, plusY, btnW, btnH, &canAdd, &isHover, &ui.game.mousePressed) {
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
	if isCloseHover && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && !ui.game.mousePressed && !ui.justOpenedStatPopup {
		ui.game.statPopupOpen = false
		ui.game.mousePressed = true
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
		ebitenutil.DebugPrintAt(screen, "Enter: Select  Esc: Close", px+16, py+panelH-24)
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
		ebitenutil.DebugPrintAt(screen, "Enter: Save  Esc: Close", px+16, py+panelH-24)
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
		ebitenutil.DebugPrintAt(screen, "Enter: Load  Esc: Close", px+16, py+panelH-24)
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
			if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && isHover && !ui.game.mousePressed {
				ui.game.statPopupOpen = true
				ui.game.statPopupCharIdx = i
				ui.game.mousePressed = true
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

	// Check for active spell effects
	hasActiveSpells := false

	// Torch Light effect
	if ui.game.torchLightActive && ui.game.torchLightDuration > 0 {
		// Torch Light: 300 seconds * 60 frames = 18000 frames max
		iconX, iconY, iconW, iconH := ui.drawSpellIcon(screen, currentX, statusBarY, iconSize, "💡", ui.game.torchLightDuration, 18000)
		ui.handleSpellIconClick(iconX, iconY, iconW, iconH, "💡")
		currentX += iconSpacing
		hasActiveSpells = true
	}

	// Wizard Eye effect
	if ui.game.wizardEyeActive && ui.game.wizardEyeDuration > 0 {
		// Wizard Eye: 300 seconds * 60 frames = 18000 frames max
		iconX, iconY, iconW, iconH := ui.drawSpellIcon(screen, currentX, statusBarY, iconSize, "👁", ui.game.wizardEyeDuration, 18000)
		ui.handleSpellIconClick(iconX, iconY, iconW, iconH, "👁")
		currentX += iconSpacing
		hasActiveSpells = true
	}

	// Walk on Water effect
	if ui.game.walkOnWaterActive && ui.game.walkOnWaterDuration > 0 {
		// Walk on Water: 180 seconds * 60 frames = 10800 frames max
		iconX, iconY, iconW, iconH := ui.drawSpellIcon(screen, currentX, statusBarY, iconSize, "🌊", ui.game.walkOnWaterDuration, 10800)
		ui.handleSpellIconClick(iconX, iconY, iconW, iconH, "🌊")
		currentX += iconSpacing
		hasActiveSpells = true
	}

	// Water Breathing effect
	if ui.game.waterBreathingActive && ui.game.waterBreathingDuration > 0 {
		// Water Breathing: 600 seconds * 60 frames = 36000 frames max
		iconX, iconY, iconW, iconH := ui.drawSpellIcon(screen, currentX, statusBarY, iconSize, "🫧", ui.game.waterBreathingDuration, 36000)
		ui.handleSpellIconClick(iconX, iconY, iconW, iconH, "🫧")
		currentX += iconSpacing
		hasActiveSpells = true
	}

	// Bless effect
	if ui.game.blessActive && ui.game.blessDuration > 0 {
		// Bless: 300 seconds * 60 frames = 18000 frames max
		iconX, iconY, iconW, iconH := ui.drawSpellIcon(screen, currentX, statusBarY, iconSize, "✨", ui.game.blessDuration, 18000)
		ui.handleSpellIconClick(iconX, iconY, iconW, iconH, "✨")
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
func (ui *UISystem) drawSpellIcon(screen *ebiten.Image, x, y, size int, icon string, currentDuration, maxDuration int) (int, int, int, int) {
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

	// Draw larger ASCII fallback in the center for better visibility
	asciiIcon := ui.getASCIIIcon(icon)
	if asciiIcon != "" {
		ebitenutil.DebugPrintAt(screen, asciiIcon, x+size/2-4, y+size/2-4)
	}

	// Add spell name below (small text)
	spellName := ui.getSpellNameFromIcon(icon)
	if spellName != "" {
		ebitenutil.DebugPrintAt(screen, spellName, x, y+size+2)
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
func (ui *UISystem) handleSpellIconClick(x, y, width, height int, icon string) {
	mouseX, mouseY := ebiten.CursorPosition()

	// Check if click is within icon bounds
	if mouseX >= x && mouseX <= x+width && mouseY >= y && mouseY <= y+height {
		// Check for mouse click (only process on first press, not while held)
		if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && !ui.utilitySpellMousePressed {
			ui.utilitySpellMousePressed = true
			currentTime := time.Now().UnixMilli()

			// Check for double-click (within 500ms and same icon)
			if currentTime-ui.game.lastUtilitySpellClickTime < 500 && ui.game.lastClickedUtilitySpell == icon {
				// Double-click detected - dispel the spell
				ui.dispelUtilitySpell(icon)
				// Reset click tracking
				ui.game.lastUtilitySpellClickTime = 0
				ui.game.lastClickedUtilitySpell = ""
			} else {
				// Single click - record for potential double-click
				ui.game.lastUtilitySpellClickTime = currentTime
				ui.game.lastClickedUtilitySpell = icon
			}
		}
	}

	// Reset mouse pressed state when button is released
	if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		ui.utilitySpellMousePressed = false
	}
}

// dispelUtilitySpell removes an active utility spell effect by triggering natural expiration
func (ui *UISystem) dispelUtilitySpell(icon string) {
	switch icon {
	case "💡": // Torch Light
		if ui.game.torchLightActive {
			ui.game.torchLightDuration = 0 // Let updateTorchLightEffect handle cleanup
			ui.game.AddCombatMessage("Torch Light dispelled!")
		}
	case "👁": // Wizard Eye
		if ui.game.wizardEyeActive {
			ui.game.wizardEyeDuration = 0 // Let updateWizardEyeEffect handle cleanup
			ui.game.AddCombatMessage("Wizard Eye dispelled!")
		}
	case "🌊": // Walk on Water
		if ui.game.walkOnWaterActive {
			ui.game.walkOnWaterDuration = 0 // Let updateWalkOnWaterEffect handle cleanup
			ui.game.AddCombatMessage("Walk on Water dispelled!")
		}
	case "🫧": // Water Breathing
		if ui.game.waterBreathingActive {
			ui.game.waterBreathingDuration = 0 // Let updateWaterBreathingEffect handle cleanup (including underwater return)
			ui.game.AddCombatMessage("Water Breathing dispelled!")
		}
	case "✨": // Bless
		if ui.game.blessActive {
			ui.game.blessDuration = 0 // Let updateBlessEffect handle cleanup
			ui.game.AddCombatMessage("Bless dispelled!")
		}
	}
}

// getSpellNameFromIcon returns the spell name for debugging based on the icon
func (ui *UISystem) getSpellNameFromIcon(icon string) string {
	switch icon {
	case "💡":
		return "Light"
	case "👁":
		return "Eye"
	case "🌊":
		return "Water"
	case "🫧":
		return "Breathing"
	case "✨":
		return "Bless"
	default:
		return ""
	}
}

// getASCIIIcon returns Egyptian/mystical symbols for thematic spell icons
func (ui *UISystem) getASCIIIcon(icon string) string {
	switch icon {
	case "💡":
		return "☀" // Sun symbol for Light
	case "👁":
		return "𓂀" // Eye of Horus (if this doesn't work, fallback to ◉)
	case "🌊":
		return "≋" // Water waves
	case "🫧":
		return "○" // Circle for bubbles/breathing
	case "✨":
		return "☩" // Cross/blessing symbol
	default:
		return ""
	}
}

// drawCompass draws the compass/direction indicator
func (ui *UISystem) drawCompass(screen *ebiten.Image) {
	compassX := ui.game.config.GetScreenWidth() - 100
	compassY := 50

	// Draw compass circle using image
	compassRadius := ui.game.config.UI.CompassRadius
	compassImg := ebiten.NewImage(compassRadius*2, compassRadius*2)
	compassImg.Fill(color.RGBA{255, 255, 255, 100}) // Semi-transparent white circle

	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(float64(compassX-compassRadius), float64(compassY-compassRadius))
	screen.DrawImage(compassImg, opts)

	// Draw direction arrow
	arrowSize := 4
	arrowImg := ebiten.NewImage(arrowSize, arrowSize)
	arrowImg.Fill(color.RGBA{255, 0, 0, 255}) // Red arrow

	arrowOpts := &ebiten.DrawImageOptions{}
	arrowX := compassX + int(float64(compassRadius-8)*math.Cos(ui.game.camera.Angle))
	arrowY := compassY + int(float64(compassRadius-8)*math.Sin(ui.game.camera.Angle))
	arrowOpts.GeoM.Translate(float64(arrowX-arrowSize/2), float64(arrowY-arrowSize/2))
	screen.DrawImage(arrowImg, arrowOpts)
}

// drawWizardEyeRadar draws enemy dots on the compass when wizard eye is active
func (ui *UISystem) drawWizardEyeRadar(screen *ebiten.Image) {
	if !ui.game.wizardEyeActive {
		return
	}

	compassX := ui.game.config.GetScreenWidth() - 100
	compassY := 50
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
		distance := math.Sqrt(dx*dx + dy*dy)

		// Only show enemies within 10 tiles
		if distance <= maxRadarRange {
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
			if distance < tileSize*3 { // Close enemies in red
				dotImg.Fill(color.RGBA{255, 50, 50, 255}) // Bright red
			} else if distance < tileSize*6 { // Medium distance in orange
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
	ebitenutil.DebugPrintAt(screen, "WASD:Move, QE:Strafe, Space:Sword, F:Fireball, H:Heal, I:Inventory, C:Characters, M:Spellbook, 1-4:Select", 10, 10)
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
	}
}

// handleTabClick checks if mouse clicked on a tab and switches to it
func (ui *UISystem) handleTabClick(tabX, tabY, tabWidth, tabHeight int, tab MenuTab) {
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		if !ui.game.mousePressed { // Only trigger on initial press
			mouseX, mouseY := ebiten.CursorPosition()
			if mouseX >= tabX && mouseX < tabX+tabWidth &&
				mouseY >= tabY && mouseY < tabY+tabHeight {
				ui.game.currentTab = tab
				ui.game.mousePressed = true
			}
		}
	}
}

// handleCharacterCardClick checks if mouse clicked on a character card and selects that character
func (ui *UISystem) handleCharacterCardClick(cardX, cardY, cardWidth, cardHeight, characterIndex int) {
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		if !ui.game.mousePressed { // Only trigger on initial press
			mouseX, mouseY := ebiten.CursorPosition()
			if mouseX >= cardX && mouseX < cardX+cardWidth &&
				mouseY >= cardY && mouseY < cardY+cardHeight {
				ui.game.selectedChar = characterIndex
				ui.game.mousePressed = true
			}
		}
	}
}

// handleCloseButtonClick checks if mouse clicked on the close button and closes the menu
func (ui *UISystem) handleCloseButtonClick(buttonX, buttonY, buttonWidth, buttonHeight int) {
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		if !ui.game.mousePressed { // Only trigger on initial press
			mouseX, mouseY := ebiten.CursorPosition()
			if mouseX >= buttonX && mouseX < buttonX+buttonWidth &&
				mouseY >= buttonY && mouseY < buttonY+buttonHeight {
				ui.game.menuOpen = false
				ui.game.mousePressed = true
			}
		}
	}
}

// handleSpellbookSchoolClick checks if mouse clicked on a magic school and selects it
func (ui *UISystem) handleSpellbookSchoolClick(schoolX, schoolY, schoolWidth, schoolHeight, schoolIndex int) {
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		if !ui.game.mousePressed { // Only trigger on initial press
			mouseX, mouseY := ebiten.CursorPosition()
			if mouseX >= schoolX && mouseX < schoolX+schoolWidth &&
				mouseY >= schoolY && mouseY < schoolY+schoolHeight {
				ui.game.selectedSchool = schoolIndex
				ui.game.selectedSpell = 0 // Reset spell selection when changing school
				ui.game.mousePressed = true
			}
		}
	}
}

// handleSpellbookSpellClick checks if mouse clicked on a spell and selects it
func (ui *UISystem) handleSpellbookSpellClick(spellX, spellY, spellWidth, spellHeight, spellIndex int) {
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		if !ui.game.mousePressed { // Only trigger on initial press
			mouseX, mouseY := ebiten.CursorPosition()
			if mouseX >= spellX && mouseX < spellX+spellWidth &&
				mouseY >= spellY && mouseY < spellY+spellHeight {

				currentTime := time.Now().UnixMilli()

				// Check for double-click (within 500ms of last click on same spell)
				if ui.game.lastClickedSpell == spellIndex &&
					ui.game.lastClickedSchool == ui.game.selectedSchool &&
					currentTime-ui.game.lastSpellClickTime < 500 {
					// Double-click detected - equip the spell directly
					ui.game.combat.EquipSelectedSpell()
					if ui.game.menuOpen {
						ui.game.menuOpen = false
					}
				} else {
					// Single click - just select the spell
					ui.game.selectedSpell = spellIndex
				}

				// Update click tracking
				ui.game.lastSpellClickTime = currentTime
				ui.game.lastClickedSpell = spellIndex
				ui.game.lastClickedSchool = ui.game.selectedSchool
				ui.game.mousePressed = true
			}
		}
	}
}

// resetMouseState should be called once per frame to reset mouse state
func (ui *UISystem) resetMouseState() {
	if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		ui.game.mousePressed = false
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

			// Handle mouse interactions
			if isHovering {
				equipTooltip = GetItemTooltip(item, currentChar, ui.game.combat)
				equipTooltipX = mouseX + 16
				equipTooltipY = mouseY + 8

				// Handle double-click to unequip
				ui.handleEquippedItemClick(slotInfo.slot)
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

			// Handle mouse interactions
			if isHovering {
				// Show tooltip for this item
				tooltip = GetItemTooltip(item, currentChar, ui.game.combat)
				tooltipX = mouseX + 16
				tooltipY = mouseY + 8

				// Handle double-click to equip
				if !ui.inventoryContextOpen {
					ui.handleInventoryItemClick(i)
				}

				// Handle right-click to open context menu
				if ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) && !ui.inventoryRightMousePressed {
					ui.inventoryRightMousePressed = true
					ui.inventoryContextOpen = true
					ui.inventoryContextX = mouseX
					ui.inventoryContextY = mouseY
					ui.inventoryContextIndex = i
				}
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
			mouseX, mouseY := ebiten.CursorPosition()
			inside := isMouseHoveringBox(mouseX, mouseY, x, y, x+menuW, y+menuH)
			if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
				if inside {
					// Discard clicked
					idx := ui.inventoryContextIndex
					if idx >= 0 && idx < len(ui.game.party.Inventory) {
						name := ui.game.party.Inventory[idx].Name
						ui.game.party.RemoveItem(idx)
						ui.game.AddCombatMessage(fmt.Sprintf("Discarded %s.", name))
					}
				}
				// Close the context menu on any left click
				ui.inventoryContextOpen = false
			}

			// Close menu if right button released
			if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
				ui.inventoryRightMousePressed = false
			}
		} else {
			// Reset right-click pressed when not open and not pressed
			if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
				ui.inventoryRightMousePressed = false
			}
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
		ui.handleSpellbookSchoolClick(panelX+30, y, 300, 20, schoolIndex)

		// Draw school name
		if schoolIndex == ui.game.selectedSchool {
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("> %s School:", schoolName), panelX+30, y)
		} else {
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("  %s School:", schoolName), panelX+30, y)
		}
		y += 20

		// Draw spells in current school
		if schoolIndex == ui.game.selectedSchool {
			// Validate and fix selected spell index if it's out of bounds
			if ui.game.selectedSpell >= len(schoolSpells) {
				ui.game.selectedSpell = 0
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
				ui.handleSpellbookSpellClick(panelX+50, spellY, 300, spellHeight, spellIndex)

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

				if spellIndex == ui.game.selectedSpell {
					ebitenutil.DebugPrintAt(screen, fmt.Sprintf("  > %s %s (SP:%d)",
						canCast, def.Name, def.SpellPointsCost), panelX+50, y)
				} else {
					ebitenutil.DebugPrintAt(screen, fmt.Sprintf("    %s %s (SP:%d)",
						canCast, def.Name, def.SpellPointsCost), panelX+50, y)
				}
				y += 15
			}
		}
	}

	// Draw spell tooltip if hovering over a spell
	if spellTooltip != "" {
		lines := strings.Split(spellTooltip, "\n")
		drawTooltip(screen, lines, tooltipX, tooltipY)
	}

	// Draw spellbook controls
	ebitenutil.DebugPrintAt(screen, "Up/Down: Navigate, Enter: Equip, Click: Select, Double-click: Equip", panelX+30, contentY+contentHeight-30)
}

// drawFPSCounter draws the FPS counter in the top-right corner
func (ui *UISystem) drawFPSCounter(screen *ebiten.Image) {
	// Use Ebiten's built-in FPS counter which is more reliable
	fps := ebiten.ActualFPS()
	tps := ebiten.ActualTPS()

	// Format FPS text
	fpsText := fmt.Sprintf("FPS: %.1f\nTPS: %.1f", fps, tps)

	// Position in top-right corner
	screenWidth := ui.game.config.GetScreenWidth()
	textX := screenWidth - 90 // 90px from right edge (wider for TPS)
	textY := 30               // 30px from top

	// Draw semi-transparent background
	bgWidth := 80
	bgHeight := 35 // Taller for two lines
	bgImg := ebiten.NewImage(bgWidth, bgHeight)
	bgImg.Fill(color.RGBA{0, 0, 0, 100}) // Semi-transparent black background

	bgOpts := &ebiten.DrawImageOptions{}
	bgOpts.GeoM.Translate(float64(textX-5), float64(textY-5))
	screen.DrawImage(bgImg, bgOpts)

	// Draw FPS text
	ebitenutil.DebugPrintAt(screen, fpsText, textX, textY)
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
func (ui *UISystem) handleInventoryItemClick(itemIndex int) {
	// Check for mouse click
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && !ui.inventoryMousePressed {
		ui.inventoryMousePressed = true
		currentTime := time.Now()

		// Check for double-click (same item clicked within 500ms)
		if itemIndex == ui.lastClickedItem && currentTime.Sub(ui.lastClickTime) < 500*time.Millisecond {
			// Double-click detected - use or equip the item
			if itemIndex < len(ui.game.party.Inventory) {
				item := ui.game.party.Inventory[itemIndex]
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

	// Reset mouse state when button is released
	if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		ui.inventoryMousePressed = false
	}
}

// handleEquippedItemClick handles double-click to unequip items from equipment slots
func (ui *UISystem) handleEquippedItemClick(slot items.EquipSlot) {
	// Check for mouse click
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && !ui.equipMousePressed {
		ui.equipMousePressed = true
		currentTime := time.Now()

		// Check for double-click (same slot clicked within 500ms)
		if slot == ui.lastClickedSlot && currentTime.Sub(ui.lastEquipClickTime) < 500*time.Millisecond {
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

	// Reset mouse state when button is released
	if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		ui.equipMousePressed = false
	}
}

// drawTurnBasedStatus displays the current game mode and turn state
func (ui *UISystem) drawTurnBasedStatus(screen *ebiten.Image) {
	x := ui.game.config.GetScreenWidth() - 200
	y := 10

	// Display current mode
	mode := "Real-time"
	if ui.game.turnBasedMode {
		mode = "Turn-based"
	}
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Mode: %s", mode), x, y)

	// Display turn information if in turn-based mode
	if ui.game.turnBasedMode {
		y += 20
		turnText := "Party Turn"
		if ui.game.currentTurn == 1 {
			turnText = "Monster Turn"
		}
		ebitenutil.DebugPrintAt(screen, turnText, x, y)

		// Show party actions used
		if ui.game.currentTurn == 0 {
			y += 20
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Actions: %d/2", ui.game.partyActionsUsed), x, y)
		}
	}

	// Display controls
	y += 20
	ebitenutil.DebugPrintAt(screen, "Enter: Toggle Mode", x, y)
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
	screenHeight := ui.game.config.GetScreenHeight()

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

	// Position at bottom center of screen
	notificationWidth := textWidth + (padding * 2)
	notificationHeight := textHeight + (padding * 2)
	notificationX := (screenWidth - notificationWidth) / 2
	notificationY := screenHeight - 100

	// Draw semi-transparent background
	bgImage := ebiten.NewImage(notificationWidth, notificationHeight)
	bgImage.Fill(color.RGBA{0, 0, 0, 180}) // Semi-transparent black

	bgOpts := &ebiten.DrawImageOptions{}
	bgOpts.GeoM.Translate(float64(notificationX), float64(notificationY))
	screen.DrawImage(bgImage, bgOpts)

	// Draw border for better visibility
	borderColor := color.RGBA{255, 255, 255, 200} // Semi-transparent white
	for i := 0; i < 2; i++ {
		ebitenutil.DrawRect(screen, float64(notificationX-i), float64(notificationY-i),
			float64(notificationWidth+2*i), float64(notificationHeight+2*i), borderColor)
	}

	// Draw the interaction message
	textX := notificationX + padding
	textY := notificationY + padding
	ebitenutil.DebugPrintAt(screen, message, textX, textY)
}
