package game

import (
	"fmt"
	"image/color"
	"math"

	"ugataima/internal/sound"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	audioSettingsRowTop   = 80
	audioSettingsRowPitch = 54
	audioSliderTrackX     = 150
	audioSliderRightPad   = 80
	audioSliderTrackH     = 8
	audioSliderThumbW     = 10
	audioSliderThumbH     = 20
	audioSettingsInset    = 18
	audioSettingsHint     = "Mouse drag or Left/Right"
	audioMenuContentInset = 16
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
	return audioSettingsPanelLayoutAt(
		(screenW-settingsMenuPanelW)/2,
		(screenH-settingsMenuPanelH)/2,
		settingsMenuPanelW,
		settingsMenuPanelH,
		ornate,
	)
}

func audioSettingsPanelLayoutAt(px, py, panelW, panelH int, ornate bool) audioSettingsPanelLayout {
	inset := audioMenuContentInset
	if ornate {
		inset = menuFrameInset
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
	// Slider presses are handled directly through inpututil instead of the
	// buffered click queue. Consume that press here so it cannot activate an
	// option after the settings screen closes.
	g.mouseLeftClicks = g.mouseLeftClicks[:0]
}

func (g *MMGame) beginAudioSettings() {
	g.audioSettingsSelection = 0
	g.audioSliderDrag = -1
	g.audioSettingsDirty = false
}

func (g *MMGame) saveAudioSettings() {
	if !g.audioSettingsDirty {
		return
	}
	if g.soundManager != nil {
		g.soundManager.SaveVolumes()
	}
	g.audioSettingsDirty = false
}

func (g *MMGame) closeAudioSettings() {
	g.saveAudioSettings()
	g.audioSliderDrag = -1
	// Settings handles pointer presses outside the buffered-click system. Drop
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
	inset := max(audioSettingsInset, contentInset)
	return pagerRect{px + inset, r.y1 - 6, px + panelW - inset, r.y2 + 6}
}

func audioBackRect(px, py, panelH, contentInset int) pagerRect {
	x := px + contentInset
	y := py + panelH - contentInset - menuBackButtonH
	return pagerRect{x, y, x + menuBackButtonW, y + menuBackButtonH}
}

func audioHintPosition(px, py, panelW, panelH, contentInset int) (string, int, int) {
	y := py + panelH - contentInset - debugTextCharHeight
	return audioSettingsHint, px + (panelW-debugTextWidth(audioSettingsHint))/2, y
}

func audioPercentX(px, panelW, contentInset int, label string) int {
	trackRight := px + panelW - audioSliderRightPad
	columnRight := min(
		px+panelW-contentInset,
		trackRight+audioPercentGap+debugTextWidth("100%"),
	)
	return columnRight - debugTextWidth(label)
}

func (g *MMGame) setSelectedAudioVolume(delta float64) {
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

func (g *MMGame) updateAudioSettingsKeys(pressed func(ebiten.Key) bool) {
	if pressed(ebiten.KeyUp) && g.audioSettingsSelection > 0 {
		g.audioSettingsSelection--
	}
	if pressed(ebiten.KeyDown) && g.audioSettingsSelection < len(audioSettingDefinitions)-1 {
		g.audioSettingsSelection++
	}
	if pressed(ebiten.KeyLeft) {
		g.setSelectedAudioVolume(-0.05)
	}
	if pressed(ebiten.KeyRight) {
		g.setSelectedAudioVolume(0.05)
	}
}

func (g *MMGame) updateEntryAudioSettings(pressed func(ebiten.Key) bool) {
	layout := makeAudioSettingsPanelLayout(g.config.GetScreenWidth(), g.config.GetScreenHeight(), true)
	g.updateAudioSettingsKeys(pressed)
	g.updateAudioSettingsPointer(layout.px, layout.py, layout.panelW)
}

func (ih *InputHandler) handleAudioSettingsInput(layout audioSettingsPanelLayout) {
	g := ih.game
	g.updateAudioSettingsKeys(ih.keys.Consume)
	g.updateAudioSettingsPointer(layout.px, layout.py, layout.panelW)
}

func (ui *UISystem) drawAudioSettingsContent(screen *ebiten.Image, px, py, panelW, panelH, contentInset int, title string) {
	g := ui.game
	drawDebugText(screen, title, px+contentInset, py+contentInset-2)
	if g.soundManager == nil {
		drawDebugText(screen, "Audio is unavailable", px+contentInset, py+64)
		return
	}
	for row, def := range audioSettingDefinitions {
		r := audioSliderRect(px, py, panelW, row)
		volume := g.soundManager.Volume(def.channel)
		if row == g.audioSettingsSelection {
			highlight := audioSelectionRect(px, py, panelW, contentInset, row)
			drawFilledRect(screen, highlight.x1, highlight.y1, highlight.x2-highlight.x1, highlight.y2-highlight.y1, color.RGBA{45, 70, 105, 190})
		}
		drawDebugText(screen, def.label, px+contentInset, r.y1+3)
		trackY := (r.y1 + r.y2 - audioSliderTrackH) / 2
		drawFilledRect(screen, r.x1, trackY, r.x2-r.x1, audioSliderTrackH, color.RGBA{35, 35, 55, 255})
		filledW := int(math.Round(volume * float64(r.x2-r.x1)))
		drawFilledRect(screen, r.x1, trackY, filledW, audioSliderTrackH, color.RGBA{170, 125, 45, 255})
		thumbX := r.x1 + filledW - audioSliderThumbW/2
		if thumbX < r.x1-audioSliderThumbW/2 {
			thumbX = r.x1 - audioSliderThumbW/2
		}
		if thumbX > r.x2-audioSliderThumbW/2 {
			thumbX = r.x2 - audioSliderThumbW/2
		}
		drawFilledRect(screen, thumbX, (r.y1+r.y2-audioSliderThumbH)/2, audioSliderThumbW, audioSliderThumbH, color.RGBA{225, 205, 145, 255})
		percentLabel := fmt.Sprintf("%d%%", int(math.Round(volume*100)))
		drawDebugText(screen, percentLabel, audioPercentX(px, panelW, contentInset, percentLabel), r.y1+3)
	}
	hint, hintX, hintY := audioHintPosition(px, py, panelW, panelH, contentInset)
	drawDebugText(screen, hint, hintX, hintY)
}

func (ui *UISystem) drawEntryAudioSettings(screen *ebiten.Image, w, h int) {
	layout := makeAudioSettingsPanelLayout(w, h, true)
	ui.drawPanel(screen, "menu_panel_wide", layout.px, layout.py, layout.panelW, layout.panelH)
	ui.drawAudioSettingsContent(screen, layout.px, layout.py, layout.panelW, layout.panelH, layout.contentInset, "Audio Settings")
	back := audioBackRect(layout.px, layout.py, layout.panelH, layout.contentInset)
	ui.drawBackButton(screen, back.x1, back.y1, func() {
		ui.game.closeAudioSettings()
	})
}
