package game

import (
	"fmt"
	"image/color"
	"math"

	"ugataima/internal/sound"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	audioSettingsRowTop   = 152
	audioSettingsRowPitch = 88
	audioSliderTrackX     = 246
	audioSliderRightPad   = 90
	audioSliderThumbW     = 16
	audioSliderThumbH     = 20
	audioSettingsInset    = 28
	audioSettingsHint     = "Drag sliders | Arrow keys adjust | Changes saved automatically"
	audioMenuContentInset = 28
	audioPercentGap       = 12
)

type audioSettingDefinition struct {
	label   string
	channel sound.VolumeChannel
}

var audioSettingDefinitions = []audioSettingDefinition{
	{label: "Master", channel: sound.VolumeMaster},
	{label: "Effects", channel: sound.VolumeSFX},
	{label: "Music", channel: sound.VolumeMusic},
}

type audioSettingsPanelLayout struct {
	px, py       int
	panelW       int
	panelH       int
	contentInset int
}

func makeAudioSettingsPanelLayout(screenW, screenH int, ornate bool) audioSettingsPanelLayout {
	panel := centeredRect(screenW, screenH, settingsMenuPanelW, settingsMenuPanelH)
	return audioSettingsPanelLayoutAt(panel.x, panel.y, panel.w, panel.h, ornate)
}

func audioSettingsPanelLayoutAt(px, py, panelW, panelH int, ornate bool) audioSettingsPanelLayout {
	inset := audioMenuContentInset
	if ornate {
		inset = audioSettingsInset
	}
	return audioSettingsPanelLayout{
		px:           px,
		py:           py,
		panelW:       panelW,
		panelH:       panelH,
		contentInset: inset,
	}
}

func (g *MMGame) beginAudioSliderDrag(row int) {
	g.audioSettingsSelection = row
	g.audioSliderDrag = row
	// The displayed pointer gesture owns this press. Retire any buffered copy
	// so it cannot activate an option after the settings screen closes.
	g.mouseLeftClicks = g.mouseLeftClicks[:0]
}

func (g *MMGame) beginAudioSettings() {
	g.audioSettingsSelection = 0
	g.audioSliderDrag = -1
	g.fontListOpen = false
	g.settingsTab = settingsTabSound
	if g.config != nil {
		g.config.EnsurePotionPreferences()
	}
}

func (g *MMGame) saveAudioSettings() {
	if g.soundManager == nil {
		g.audioSettingsDirty = false
	} else if g.audioSettingsDirty {
		err := g.soundManager.SaveVolumes()
		g.recordSettingsSave("sound", "sound", err)
		g.audioSettingsDirty = err != nil
	}
	g.savePotionPreferences()
	g.saveDisplayPreferences()
	g.saveCombatPreferences()
}

func (g *MMGame) closeAudioSettings() {
	g.saveAudioSettings()
	g.audioSliderDrag = -1
	g.fontListOpen = false
	// Settings owns pointer gestures through the displayed dispatcher. Drop
	// every pending press at this UI boundary, including clicks in dead space,
	// so the root menu cannot replay them on its next update.
	g.mouseLeftClicks = g.mouseLeftClicks[:0]
	if g.entryMenuMode == EntryMenuSettings {
		g.entryMenuMode = EntryMenuRoot
	}
	if g.mainMenuMode == MenuSettings {
		g.mainMenuMode = MenuMain
	}
}

func audioSliderRect(px, py, panelW, row int) pagerRect {
	y := py + audioSettingsRowTop + row*audioSettingsRowPitch
	trackW := panelW - audioSliderTrackX - audioSliderRightPad
	if trackW < 1 {
		trackW = 1
	}
	return pagerRect{
		x1: px + audioSliderTrackX,
		y1: y - audioSliderThumbH/2,
		x2: px + audioSliderTrackX + trackW,
		y2: y + audioSliderThumbH/2,
	}
}

func audioSelectionRect(px, py, panelW, contentInset, row int) pagerRect {
	r := audioSliderRect(px, py, panelW, row)
	inset := contentInset
	return pagerRect{px + inset, r.y1 - 24, px + panelW - inset, r.y2 + 24}
}

func audioBackRect(px, py, panelH, contentInset int) pagerRect {
	x := px + contentInset
	y := py + panelH - contentInset - menuBackButtonH
	return pagerRect{x, y, x + menuBackButtonW, y + menuBackButtonH}
}

func audioHintPosition(px, py, panelW, panelH, contentInset int) (string, int, int) {
	y := py + panelH - contentInset - uiTextCharHeight
	return audioSettingsHint, px + panelW - contentInset - uiTextWidth(audioSettingsHint), y
}

// The live tab and layout checks share the exact footer text and width fitting.
func settingsHintPosition(tab settingsTabKind, saveError string, px, py, panelW, panelH, inset int) (string, int, int) {
	hint, _, y := audioHintPosition(px, py, panelW, panelH, inset)
	if tab == settingsTabCombat {
		hint = "Up/Down: row | Left/Right: choose | Auto-save"
	}
	if saveError != "" {
		hint = saveError
	}
	back := audioBackRect(px, py, panelH, inset)
	width := px + panelW - inset - (back.x2 + 16)
	if uiTextWidth(hint) > width && saveError == "" {
		if tab == settingsTabCombat {
			hint = "Arrows: choose | Auto-save"
		} else {
			hint = "Changes saved automatically"
		}
	}
	hint = clipUIText(hint, max(0, width))
	return hint, px + panelW - inset - uiTextWidth(hint), y
}

func audioPercentX(px, panelW, contentInset int, label string) int {
	trackRight := px + panelW - audioSliderRightPad
	columnRight := min(
		px+panelW-contentInset,
		trackRight+audioPercentGap+uiTextWidth("100%"),
	)
	return columnRight - uiTextWidth(label)
}

func (g *MMGame) setSelectedAudioVolume(delta float64) {
	switch g.settingsTab {
	case settingsTabCombat:
		if g.audioSettingsSelection == gameplayRowView {
			g.setWideView(delta > 0)
		} else {
			g.setCombatOverlay(delta > 0)
		}
		return
	case settingsTabPotions:
		mana := g.audioSettingsSelection == 1
		g.setPotionThreshold(mana, g.config.AutoPotionThreshold(mana)+int(math.Round(delta*100)))
		g.savePotionPreferences()
		return
	case settingsTabDisplay:
		dir := 1
		if delta < 0 {
			dir = -1
		}
		g.fontListOpen = false
		if g.audioSettingsSelection == displayRowFont {
			g.stepUIFont(dir)
		} else {
			g.stepInterfaceSize(dir)
		}
		return
	}
	if g.soundManager == nil || g.audioSettingsSelection < 0 || g.audioSettingsSelection >= len(audioSettingDefinitions) {
		return
	}
	channel := audioSettingDefinitions[g.audioSettingsSelection].channel
	if g.soundManager.SetVolume(channel, g.soundManager.Volume(channel)+delta) {
		g.audioSettingsDirty = true
	}
}

func (g *MMGame) updateAudioSettingsPointer(px, py, panelW int) {
	if g.soundManager == nil {
		return
	}
	mx, my := pointerPosition()
	if pointerLeftJustPressed() {
		g.audioSliderDrag = -1
		for row := range audioSettingDefinitions {
			r := audioSliderRect(px, py, panelW, row)
			if isMouseHoveringBox(mx, my, r.x1-8, r.y1, r.x2+8, r.y2) {
				g.beginAudioSliderDrag(row)
				break
			}
		}
	}
	if g.audioSliderDrag >= 0 && pointerLeftPressed() {
		r := audioSliderRect(px, py, panelW, g.audioSliderDrag)
		volume := float64(mx-r.x1) / float64(r.x2-r.x1)
		channel := audioSettingDefinitions[g.audioSliderDrag].channel
		if g.soundManager.SetVolume(channel, volume) {
			g.audioSettingsDirty = true
		}
	}
	if pointerLeftJustRelease() && g.audioSliderDrag >= 0 {
		g.saveAudioSettings()
		g.audioSliderDrag = -1
	}
}

func (ui *UISystem) audioSettingsOwnsInput() bool {
	g := ui.game
	switch g.appScreen {
	case AppScreenMainMenu:
		return g.entryMenuMode == EntryMenuSettings
	case AppScreenInGame:
		return ui.topModalLayer() == modalLayerMainMenu && g.mainMenuMode == MenuSettings
	default:
		return false
	}
}

func (g *MMGame) updateAudioSettingsKeys(pressed func(ebiten.Key) bool) {
	// Selection is presentation state, not ownership of the fixed slider
	// geometry. Gate keyboard targeting here without cancelling mouse drags.
	if gl := g.gameLoop; gl != nil && gl.ui != nil {
		ui := gl.ui
		if !ui.audioSettingsOwnsInput() || !ui.displayedInputCurrent() ||
			ui.displayedInput.audioSelection != g.audioSettingsSelection {
			return
		}
	}
	selection := g.audioSettingsSelection
	rows := settingsTabRows(g.settingsTab)
	if pressed(ebiten.KeyUp) && g.audioSettingsSelection > 0 {
		g.audioSettingsSelection--
	}
	if pressed(ebiten.KeyDown) && g.audioSettingsSelection < rows-1 {
		g.audioSettingsSelection++
	}
	if selection != g.audioSettingsSelection {
		return
	}
	if pressed(ebiten.KeyLeft) {
		g.setSelectedAudioVolume(-0.05)
	}
	if pressed(ebiten.KeyRight) {
		g.setSelectedAudioVolume(0.05)
	}
}

func (ui *UISystem) drawSettingsContent(screen *ebiten.Image, px, py, panelW, panelH, contentInset int) {
	g := ui.game
	ui.drawSettingsHeader(screen, px, py, panelW)
	if ui.audioSettingsOwnsInput() {
		ui.displayedInput.audioSelection = g.audioSettingsSelection
		ui.onDisplayedInput(uiCommandPointer, layoutRect{}, func() {
			switch g.settingsTab {
			case settingsTabSound:
				g.updateAudioSettingsPointer(px, py, panelW)
			case settingsTabPotions:
				g.updatePotionSettingsPointer(px, py, panelW)
			}
		})
	}
	if g.settingsTab == settingsTabPotions {
		ui.drawPotionSettings(screen, px, py, panelW)
	} else if g.settingsTab == settingsTabDisplay {
		ui.drawDisplaySettings(screen, px, py, panelW, contentInset)
	} else if g.settingsTab == settingsTabCombat {
		ui.drawCombatSettings(screen, px, py, panelW)
	} else if g.soundManager == nil {
		drawCenteredUIText(screen, "Audio is unavailable", px+32, py+150, panelW-64, 24)
	} else {
		drawCenteredUIText(screen, "Adjust the balance of your adventure", px+32, py+100, panelW-64, 18)
		descriptions := []string{"Overall sound level", "Combat and world sounds", "Exploration and battle"}
		for row, def := range audioSettingDefinitions {
			r := audioSliderRect(px, py, panelW, row)
			volume := g.soundManager.Volume(def.channel)
			box := audioSelectionRect(px, py, panelW, contentInset, row)
			selected := row == g.audioSettingsSelection
			drawFilledRect(screen, box.x1, box.y1, box.x2-box.x1, box.y2-box.y1, color.RGBA{24, 27, 30, 255})
			border := color.RGBA{68, 65, 55, 255}
			if selected {
				border = color.RGBA{177, 149, 89, 255}
			}
			drawRectBorder(screen, box.x1, box.y1, box.x2-box.x1, box.y2-box.y1, 1, border)
			for bar := 0; bar < 4; bar++ {
				height := 10 + bar*5
				ink := color.RGBA{65, 61, 50, 255}
				if float64(bar)/4 < volume {
					ink = color.RGBA{209, 174, 93, 255}
				}
				drawFilledRect(screen, box.x1+14+bar*6, box.y1+46-height, 4, height, ink)
			}
			drawUITextColored(screen, def.label, box.x1+50, r.y1-6, color.RGBA{230, 213, 172, 255})
			drawUITextColored(screen, descriptions[row], box.x1+50, r.y1+14, color.RGBA{156, 158, 158, 255})
			drawSettingsSlider(screen, r, volume, color.RGBA{184, 144, 66, 255}, selected)
			label := fmt.Sprintf("%d%%", int(math.Round(volume*100)))
			drawUITextColored(screen, label, audioPercentX(px, panelW, contentInset, label), r.y1+4, color.RGBA{235, 221, 180, 255})
		}
	}
	hint, hx, hy := settingsHintPosition(g.settingsTab, g.settingsSaveError, px, py, panelW, panelH, contentInset)
	drawUITextColored(screen, hint, hx, hy, color.RGBA{169, 165, 149, 255})
}

func (ui *UISystem) drawEntryAudioSettings(screen *ebiten.Image, w, h int) {
	ui.tooltipLines = nil
	ui.tooltipRows = nil
	ui.tooltipCompareLines = nil
	ui.tooltipCompareRows = nil
	layout := makeAudioSettingsPanelLayout(w, h, true)
	ui.drawPanel(screen, "menu_panel_wide", layout.px, layout.py, layout.panelW, layout.panelH)
	ui.drawSettingsContent(screen, layout.px, layout.py, layout.panelW, layout.panelH, layout.contentInset)
	back := audioBackRect(layout.px, layout.py, layout.panelH, layout.contentInset)
	ui.drawBackButton(screen, back.x1, back.y1, func() {
		ui.game.closeAudioSettings()
	})
	ui.drawQueuedTooltips(screen)
}
