package game

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

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

		// Draw background panel
		vector.DrawFilledRect(screen, float32(x), float32(startY), float32(portraitWidth-2), float32(portraitHeight), bgColor, false)

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
		isPoisoned := false
		for _, cond := range member.Conditions {
			if cond == character.ConditionUnconscious {
				isUnconscious = true
			}
			if cond == character.ConditionPoisoned {
				isPoisoned = true
			}
		}
		if isUnconscious {
			vector.DrawFilledRect(screen, float32(x), float32(startY), float32(portraitWidth-2), float32(portraitHeight), color.RGBA{0, 0, 0, 140}, false)
		} else if isPoisoned {
			pulse := 0.6 + 0.4*math.Sin(float64(ui.game.frameCount)*0.15)
			alpha := uint8((80 + 60*pulse) / 3.0)
			vector.DrawFilledRect(screen, float32(x), float32(startY), float32(portraitWidth-2), float32(portraitHeight), color.RGBA{40, 160, 80, alpha}, false)
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

		// Draw ^ indicator for pending skill/spell choice
		if ui.game.hasLevelUpChoiceForChar(i) {
			caretX := x + portraitWidth - 28
			caretY := startY + portraitHeight - 28
			caretW := 24
			caretH := 24
			mouseX, mouseY := ebiten.CursorPosition()
			isHover := mouseX >= caretX && mouseX < caretX+caretW && mouseY >= caretY && mouseY < caretY+caretH
			drawSkillPointIndicator(screen, caretX, caretY, caretW, caretH, isHover)
			if ui.game.consumeLeftClickIn(caretX, caretY, caretX+caretW, caretY+caretH) {
				ui.game.openLevelUpChoiceForChar(i)
			}
		}
	}
}

// drawStatPointPlusButton draws the + button under the portrait if stat points are available
func drawStatPointPlusButton(screen *ebiten.Image, x, y, w, h, points int, isHover bool) {
	var plusColor color.RGBA
	if isHover {
		plusColor = color.RGBA{80, 200, 80, 220}
	} else {
		plusColor = color.RGBA{60, 120, 60, 180}
	}
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), float32(h), plusColor, false)
	ebitenutil.DebugPrintAt(screen, "+", x+7, y+3)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%d", points), x+w+2, y+6)
}

// drawSkillPointIndicator draws the ^ button for pending skill/spell choices.
func drawSkillPointIndicator(screen *ebiten.Image, x, y, w, h int, isHover bool) {
	var caretColor color.RGBA
	if isHover {
		caretColor = color.RGBA{200, 180, 80, 220}
	} else {
		caretColor = color.RGBA{160, 140, 60, 200}
	}
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), float32(h), caretColor, false)
	ebitenutil.DebugPrintAt(screen, "^", x+7, y+3)
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
		vector.DrawFilledRect(screen, float32(statusBarX-5), float32(statusBarY-4), float32(barWidth), float32(barHeight), color.RGBA{0, 0, 0, 120}, false)
	}
}

// drawSpellIcon draws a single spell status icon with duration bar and returns clickable bounds
func (ui *UISystem) drawSpellIcon(screen *ebiten.Image, x, y, size int, icon, fallback string, currentDuration, maxDuration int) (int, int, int, int) {
	// Draw icon background (more transparent, with border)
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(size), float32(size), color.RGBA{80, 80, 80, 200}, false)
	vector.DrawFilledRect(screen, float32(x+1), float32(y+1), float32(size-2), float32(size-2), color.RGBA{20, 20, 20, 120}, false)

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
		vector.DrawFilledRect(screen, float32(x), float32(y+size-barHeight), float32(barWidth), float32(barHeight), color.RGBA{60, 60, 60, 200}, false)

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

				vector.DrawFilledRect(screen, float32(x), float32(y+size-barHeight), float32(fillWidth), float32(barHeight), barColor, false)
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
	vector.DrawFilledRect(screen, float32(arrowX-arrowHeadSize/2), float32(arrowY-arrowHeadSize/2), float32(arrowHeadSize), float32(arrowHeadSize), color.RGBA{255, 80, 80, 255}, false)

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
		dist := dx*dx + dy*dy // Use squared distance to avoid sqrt
		maxRangeSq := maxRadarRange * maxRadarRange

		// Only show enemies within 10 tiles
		if dist <= maxRangeSq {
			// Calculate angle from player to monster
			angle := math.Atan2(dy, dx)

			// Place dot at compass edge based on direction
			edgeRadius := float64(compassRadius - 5) // 5 pixels inside compass edge
			dotX := compassX + int(math.Cos(angle)*edgeRadius)
			dotY := compassY + int(math.Sin(angle)*edgeRadius)

			// Select cached dot image based on distance for threat assessment
			// Using squared distances to avoid sqrt
			closeDistSq := (tileSize * 3) * (tileSize * 3)
			mediumDistSq := (tileSize * 6) * (tileSize * 6)

			var dotImg *ebiten.Image
			if dist < closeDistSq {
				dotImg = ui.radarDotClose // Red for close enemies
			} else if dist < mediumDistSq {
				dotImg = ui.radarDotMedium // Orange for medium distance
			} else {
				dotImg = ui.radarDotFar // Yellow for far enemies
			}

			// Draw cached dot image (much faster than vector.DrawFilledCircle)
			opts := &ebiten.DrawImageOptions{}
			opts.GeoM.Translate(float64(dotX-2), float64(dotY-2)) // Center the 4x4 dot
			screen.DrawImage(dotImg, opts)
		}
	}
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
	vector.DrawFilledRect(screen, float32(startX-5), float32(startY-5), float32(messageWidth), float32(bgHeight), color.RGBA{0, 0, 0, 150}, false)

	// Draw messages from top to bottom (most recent at bottom)
	for i, message := range messages {
		textY := startY + (i * messageSpacing)
		ebitenutil.DebugPrintAt(screen, message, startX, textY)
	}
}

// drawTurnBasedStatus displays the current game mode and turn state
func (ui *UISystem) drawTurnBasedStatus(screen *ebiten.Image) {
	lines, barX, barY, barWidth, barHeight := ui.turnBasedStatusLayout()
	lineHeight := 16
	padding := 6

	vector.DrawFilledRect(screen, float32(barX), float32(barY), float32(barWidth), float32(barHeight), color.RGBA{0, 0, 0, 120}, false)

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

	vector.DrawFilledRect(screen, float32(barX), float32(barY), float32(barWidth), float32(barHeight), color.RGBA{0, 0, 0, 120}, false)

	for i, line := range lines {
		ebitenutil.DebugPrintAt(screen, line, barX+padding, barY+padding+i*lineHeight)
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
	vector.DrawFilledRect(screen, float32(notificationX), float32(notificationY), float32(notificationWidth), float32(notificationHeight), color.RGBA{0, 0, 0, 180}, false)

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

// drawInstructions draws the control instructions
func (ui *UISystem) drawInstructions(screen *ebiten.Image) {
	ebitenutil.DebugPrintAt(screen, "ESC: Main menu", 10, 10)
}
