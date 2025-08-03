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

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

// UISystem handles all user interface rendering and logic
type UISystem struct {
	game *MMGame
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
}

// drawGameplayUI draws core gameplay UI elements
func (ui *UISystem) drawGameplayUI(screen *ebiten.Image) {
	ui.drawPartyUI(screen)
	ui.drawSpellStatusBar(screen)
	ui.drawCompass(screen)
	ui.drawWizardEyeRadar(screen)
	ui.drawCombatMessages(screen)
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

	// Draw dialog if active
	if ui.game.dialogActive {
		ui.drawNPCDialog(screen)
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
		ui.drawSpellIcon(screen, currentX, statusBarY, iconSize, "💡", ui.game.torchLightDuration, 1800)
		currentX += iconSpacing
		hasActiveSpells = true
	}

	// Wizard Eye effect
	if ui.game.wizardEyeActive && ui.game.wizardEyeDuration > 0 {
		ui.drawSpellIcon(screen, currentX, statusBarY, iconSize, "👁", ui.game.wizardEyeDuration, 900)
		currentX += iconSpacing
		hasActiveSpells = true
	}

	// Walk on Water effect
	if ui.game.walkOnWaterActive && ui.game.walkOnWaterDuration > 0 {
		ui.drawSpellIcon(screen, currentX, statusBarY, iconSize, "🌊", ui.game.walkOnWaterDuration, 18000)
		currentX += iconSpacing
		hasActiveSpells = true
	}

	// TODO: Add other spell effects like Bless, etc.
	// when they are implemented in the game state

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

// drawSpellIcon draws a single spell status icon with duration bar
func (ui *UISystem) drawSpellIcon(screen *ebiten.Image, x, y, size int, icon string, currentDuration, maxDuration int) {
	// Draw icon background (slightly transparent)
	iconBg := ebiten.NewImage(size, size)
	iconBg.Fill(color.RGBA{40, 40, 40, 180})

	iconOpts := &ebiten.DrawImageOptions{}
	iconOpts.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(iconBg, iconOpts)

	// Draw icon (for now using text, could be replaced with actual icons later)
	ebitenutil.DebugPrintAt(screen, icon, x+4, y+4)

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

	// Reset mouse state at the end of the frame
	ui.resetMouseState()
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
	for i, slotInfo := range equipSlots {
		y := equipmentY + (i * 15)
		if item, equipped := currentChar.Equipment[slotInfo.slot]; equipped {
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%-8s: %s", slotInfo.name, item.Name), equipX, y)
		} else {
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%-8s: (empty)", slotInfo.name), equipX, y)
		}
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
		// Show items in single column for better readability
		for i, item := range ui.game.party.Inventory {
			if i >= 15 { // Show max 15 items due to space constraints
				ebitenutil.DebugPrintAt(screen, fmt.Sprintf("... and %d more items",
					len(ui.game.party.Inventory)-15), invX, itemsY+(i*15))
				break
			}
			y := itemsY + (i * 15)
			itemInfo := fmt.Sprintf("%d. %s", i+1, item.Name)
			ebitenutil.DebugPrintAt(screen, itemInfo, invX, y)
		}
	}

	// Instructions
	instructionY := contentY + contentHeight - 30
	ebitenutil.DebugPrintAt(screen, "Use 1-4 keys to select different characters and view their equipment", panelX+20, instructionY)
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

	for schoolIndex, school := range schools {
		schoolName := ui.getSchoolName(school)
		spells := currentChar.GetSpellsForSchool(school)

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
			if ui.game.selectedSpell >= len(spells) {
				ui.game.selectedSpell = 0
			}

			for spellIndex, spell := range spells {
				canCast := "✓"
				if currentChar.SpellPoints < spell.SpellPoints {
					canCast = "✗"
				}

				// Handle mouse interactions for spells
				spellY := y
				spellHeight := 15
				mouseX, mouseY := ebiten.CursorPosition()
				isHovering := mouseX >= panelX+50 && mouseX < panelX+350 && mouseY >= spellY && mouseY < spellY+spellHeight

				// Handle mouse clicks on spells
				ui.handleSpellbookSpellClick(panelX+50, spellY, 300, spellHeight, spellIndex)

				// Draw spell with hover highlight
				if isHovering {
					// Draw hover background
					hoverBg := ebiten.NewImage(300, spellHeight)
					hoverBg.Fill(color.RGBA{100, 100, 150, 100})
					hoverOpts := &ebiten.DrawImageOptions{}
					hoverOpts.GeoM.Translate(float64(panelX+50), float64(spellY))
					screen.DrawImage(hoverBg, hoverOpts)
				}

				if spellIndex == ui.game.selectedSpell {
					ebitenutil.DebugPrintAt(screen, fmt.Sprintf("  > %s %s (SP:%d)",
						canCast, spell.Name, spell.SpellPoints), panelX+50, y)
				} else {
					ebitenutil.DebugPrintAt(screen, fmt.Sprintf("    %s %s (SP:%d)",
						canCast, spell.Name, spell.SpellPoints), panelX+50, y)
				}
				y += 15
			}
		}
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
		character.ConditionNormal:     "Normal",
		character.ConditionPoisoned:   "Poisoned",
		character.ConditionDiseased:   "Diseased",
		character.ConditionCursed:     "Cursed",
		character.ConditionAsleep:     "Asleep",
		character.ConditionFear:       "Fear",
		character.ConditionParalyzed:  "Paralyzed",
		character.ConditionDead:       "Dead",
		character.ConditionStone:      "Stone",
		character.ConditionEradicated: "Eradicated",
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

// drawNPCDialog draws the NPC dialog interface for spell trading
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
	for i := 0; i < 3; i++ {
		ebitenutil.DrawRect(screen, float64(dialogX-i), float64(dialogY-i), float64(dialogWidth+2*i), float64(dialogHeight+2*i), borderColor)
	}

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
		y := dialogY + 100 + (i * 25)
		charText := fmt.Sprintf("%d. %s (Level %d %s)", i+1, member.Name, member.Level, member.GetClassName())

		// Check if character can learn the selected spell
		canLearn := selectedSpell != nil && ui.characterCanLearnSpell(member, selectedSpell)
		alreadyKnows := selectedSpell != nil && ui.characterKnowsSpell(member, selectedSpell.Name)

		// Color coding based on eligibility
		var bgColor color.RGBA
		if i == ui.game.selectedCharIdx {
			// Selected character - blue background
			bgColor = color.RGBA{0, 100, 200, 128}
		} else if alreadyKnows {
			// Already knows spell - gray background
			bgColor = color.RGBA{100, 100, 100, 64}
			charText += " (Knows Spell)"
		} else if canLearn {
			// Can learn spell - green tint
			bgColor = color.RGBA{0, 150, 0, 64}
			charText += " (Can Learn)"
		} else if selectedSpell != nil {
			// Cannot learn spell - red tint
			bgColor = color.RGBA{150, 0, 0, 64}
			charText += " (Cannot Learn)"
		} else {
			// No spell selected - no background
			bgColor = color.RGBA{0, 0, 0, 0}
		}

		// Draw background if needed
		if bgColor.A > 0 {
			selectionImg := ebiten.NewImage(300, 20)
			selectionImg.Fill(bgColor)
			selectionOpts := &ebiten.DrawImageOptions{}
			selectionOpts.GeoM.Translate(float64(dialogX+15), float64(y-2))
			screen.DrawImage(selectionImg, selectionOpts)
		}

		ebitenutil.DebugPrintAt(screen, charText, dialogX+20, y)
	}

	// Draw spells section
	spellsY := dialogY + 100 + (len(ui.game.party.Members) * 25) + 20
	ebitenutil.DebugPrintAt(screen, "Available Spells:", dialogX+20, spellsY)

	// Draw spell list using deterministic ordering
	spellKeys := ui.getAvailableSpellKeys()
	for spellIndex, spellKey := range spellKeys {
		npcSpell := ui.game.dialogNPC.SpellData[spellKey]
		y := spellsY + 20 + (spellIndex * 25)
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
			// Draw selection background
			selectionImg := ebiten.NewImage(350, 20)
			selectionImg.Fill(color.RGBA{0, 150, 0, 128})
			selectionOpts := &ebiten.DrawImageOptions{}
			selectionOpts.GeoM.Translate(float64(dialogX+15), float64(y-2))
			screen.DrawImage(selectionImg, selectionOpts)
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

// characterKnowsSpell checks if a character already knows a spell
func (ui *UISystem) characterKnowsSpell(char *character.MMCharacter, spellName string) bool {
	for _, magicSkill := range char.MagicSchools {
		for _, spell := range magicSkill.Spells {
			if spell.Name == spellName {
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
