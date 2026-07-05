package main

import (
	"fmt"
	"image"
	"image/color"

	"ugataima/internal/config"
	"ugataima/internal/game"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// FX page: live preview of the game's special effects. The heavy lifting is
// game.FxPreview — a sandbox MMGame whose real combat/render code plays the
// selected effect — so the editor stays a thin list + viewport around it.

const (
	fxListW    = 340
	fxRowH     = 22
	fxListPadY = 8
)

var fxPage struct {
	preview *game.FxPreview
	items   []game.FxItem
	selIdx  int
	scroll  int
	initErr string
}

// ensureFXPage lazily builds the sandbox on first tab open, so editor startup
// cost is unchanged and an FX init failure degrades to an on-page message.
func (v *viewer) ensureFXPage() {
	if fxPage.preview != nil || fxPage.initErr != "" {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			fxPage.initErr = fmt.Sprintf("FX sandbox failed to start: %v", r)
		}
	}()
	p, err := game.NewFxPreview(config.GlobalConfig)
	if err != nil {
		fxPage.initErr = err.Error()
		return
	}
	fxPage.preview = p
	fxPage.items = p.Items()
	if len(fxPage.items) > 0 {
		p.Select(fxPage.items[0])
	}
}

func (v *viewer) updateFXPage() {
	v.ensureFXPage()
	if fxPage.preview == nil {
		return
	}

	moved := 0
	if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
		moved = 1
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
		moved = -1
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageDown) {
		moved = 10
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageUp) {
		moved = -10
	}
	if moved != 0 {
		fxPage.selIdx += moved
		if fxPage.selIdx < 0 {
			fxPage.selIdx = 0
		}
		if fxPage.selIdx >= len(fxPage.items) {
			fxPage.selIdx = len(fxPage.items) - 1
		}
		fxPage.preview.Select(fxPage.items[fxPage.selIdx])
		v.scrollFXSelectionIntoView()
	}

	_, wheelY := ebiten.Wheel()
	if wheelY != 0 {
		mx, _ := ebiten.CursorPosition()
		if mx < fxListW {
			fxPage.scroll -= int(wheelY * 30)
			v.clampFXScroll()
		}
	}

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		if mx < fxListW && my > pageBarHeight {
			idx := (my - pageBarHeight - fxListPadY + fxPage.scroll) / fxRowH
			if idx >= 0 && idx < len(fxPage.items) {
				fxPage.selIdx = idx
				fxPage.preview.Select(fxPage.items[idx])
			}
		}
	}

	fxPage.preview.Step()
}

func (v *viewer) fxListViewportH() int {
	return windowHeight - pageBarHeight - fxListPadY*2
}

func (v *viewer) clampFXScroll() {
	maxScroll := len(fxPage.items)*fxRowH - v.fxListViewportH()
	if maxScroll < 0 {
		maxScroll = 0
	}
	if fxPage.scroll > maxScroll {
		fxPage.scroll = maxScroll
	}
	if fxPage.scroll < 0 {
		fxPage.scroll = 0
	}
}

func (v *viewer) scrollFXSelectionIntoView() {
	top := fxPage.selIdx * fxRowH
	if top-fxPage.scroll < 0 {
		fxPage.scroll = top
	}
	if bottom := top + fxRowH; bottom-fxPage.scroll > v.fxListViewportH() {
		fxPage.scroll = bottom - v.fxListViewportH()
	}
	v.clampFXScroll()
}

func fxKindTag(k game.FxKind) string {
	switch k {
	case game.FxSpell:
		return "[spell]"
	case game.FxWeapon:
		return "[weapon]"
	case game.FxTrap:
		return "[trap]"
	case game.FxTile:
		return "[tile]"
	case game.FxCard:
		return "[card]"
	}
	return "[?]"
}

func (v *viewer) drawFXPage(screen *ebiten.Image) {
	if fxPage.initErr != "" {
		ebitenutil.DebugPrintAt(screen, fxPage.initErr, contentPad, pageBarHeight+contentPad)
		return
	}
	if fxPage.preview == nil {
		ebitenutil.DebugPrintAt(screen, "starting FX sandbox...", contentPad, pageBarHeight+contentPad)
		return
	}

	// Left: selectable effect list, clipped so scrolled rows never overlap the
	// page tab bar.
	vector.FillRect(screen, 0, float32(pageBarHeight), float32(fxListW), float32(windowHeight-pageBarHeight), color.RGBA{22, 22, 32, 255}, false)
	list := screen.SubImage(image.Rect(0, pageBarHeight, fxListW, windowHeight)).(*ebiten.Image)
	y0 := pageBarHeight + fxListPadY - fxPage.scroll
	for i, it := range fxPage.items {
		ry := y0 + i*fxRowH
		if ry < pageBarHeight-fxRowH || ry > windowHeight {
			continue
		}
		if i == fxPage.selIdx {
			vector.FillRect(list, 0, float32(ry-3), float32(fxListW), float32(fxRowH), color.RGBA{60, 90, 140, 200}, false)
		}
		ebitenutil.DebugPrintAt(list, fmt.Sprintf("%-8s %s", fxKindTag(it.Kind), it.Label), 8, ry)
	}

	// Right: the sandbox scene, aspect-fit into the remaining panel.
	scene := fxPage.preview.Scene()
	panelX := fxListW + contentPad
	panelY := pageBarHeight + contentPad
	panelW := windowWidth - panelX - contentPad
	panelH := windowHeight - panelY - contentPad - 24
	sw, sh := scene.Bounds().Dx(), scene.Bounds().Dy()
	scale := float64(panelW) / float64(sw)
	if s := float64(panelH) / float64(sh); s < scale {
		scale = s
	}
	dw, dh := int(float64(sw)*scale), int(float64(sh)*scale)
	dx := panelX + (panelW-dw)/2
	dy := panelY + (panelH-dh)/2
	vector.FillRect(screen, float32(dx-2), float32(dy-2), float32(dw+4), float32(dh+4), color.RGBA{60, 60, 80, 255}, false)
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Scale(scale, scale)
	opts.GeoM.Translate(float64(dx), float64(dy))
	screen.DrawImage(scene, opts)

	sel := fxPage.items[fxPage.selIdx]
	ebitenutil.DebugPrintAt(screen,
		fmt.Sprintf("%s %s  (key: %s)  — Up/Down select, wheel scroll", fxKindTag(sel.Kind), sel.Label, sel.Key),
		panelX, windowHeight-20)
}
