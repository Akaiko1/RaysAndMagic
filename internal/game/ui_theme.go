package game

import (
	"fmt"
	"image/color"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
)

// Interface art owns only decoration. Layout and input continue to share their
// existing rectangles. Measured source cuts preserve the corners and leave
// straight rails in the stretch bands.
type interfaceFrame uint8

const (
	frameGold interfaceFrame = iota
	frameSilver
	frameBronze
)

var interfacePanelFill = color.RGBA{20, 18, 16, 255}

type interfaceFrameSpec struct {
	name    string
	pattern patternFrame
	scale   int
}

var interfaceFrames = [...]interfaceFrameSpec{
	{"theme_trim_gold", patternFrame{w: 512, h: 512, slice: 16, stretch: true}, 4},
	{"theme_trim_silver", patternFrame{w: 512, h: 512, slice: 16, stretch: true}, 4},
	{"theme_trim_bronze", patternFrame{w: 512, h: 512, slice: 16, stretch: true}, 4},
}

// Validate and preload the small shared masters once, before the first menu.
// Source-coordinate cuts must never silently address a resized replacement.
func (g *MMGame) validateInterfaceArt() {
	if g.sprites == nil {
		return
	}
	if _, err := os.Stat("assets/sprites"); err != nil {
		return
	}
	for _, spec := range interfaceFrames {
		if !g.sprites.HasSprite(spec.name) {
			panic(fmt.Sprintf("interface frame %q is missing", spec.name))
		}
		src := g.sprites.GetSprite(spec.name)
		if src == nil || src.Bounds().Dx() != spec.pattern.w || src.Bounds().Dy() != spec.pattern.h {
			panic(fmt.Sprintf("interface frame %q must be %dx%d", spec.name, spec.pattern.w, spec.pattern.h))
		}
	}
}

func interfaceFrameStyle(name string) (interfaceFrame, bool) {
	switch name {
	case "menu_panel_wide", "menu_panel_frame":
		return frameGold, true
	case "menu_panel_tall", "menu_panel_slatted":
		return frameSilver, true
	case "menu_panel_parchment", "character_scroll_panel":
		return frameBronze, true
	case "menu_panel_slot":
		return frameGold, true
	}
	return 0, false
}

func (ui *UISystem) drawThemeFrame(screen *ebiten.Image, style interfaceFrame, x, y, w, h int) {
	ui.drawThemeFrameAlpha(screen, style, x, y, w, h, 1)
}

func (ui *UISystem) drawThemeFrameAlpha(screen *ebiten.Image, style interfaceFrame, x, y, w, h int, alpha float64) {
	var tint ebiten.ColorScale
	tint.ScaleAlpha(float32(alpha))
	ui.drawThemeFrameTint(screen, style, x, y, w, h, tint)
}

func (ui *UISystem) drawThemeFrameTint(screen *ebiten.Image, style interfaceFrame, x, y, w, h int, tint ebiten.ColorScale) {
	alpha := float64(tint.A())
	if w <= 0 || h <= 0 {
		return
	}
	spec := &interfaceFrames[style]
	if ui.game.sprites == nil || !ui.game.sprites.HasSprite(spec.name) {
		drawFilledRect(screen, x, y, w, h, fadeVectorColor(interfacePanelFill, alpha))
		return
	}
	src := ui.game.sprites.GetSprite(spec.name)
	if src == nil {
		// A deferred loader can know the asset without having its texture yet.
		// Loading banners must remain drawable while that same loader is busy.
		drawFilledRect(screen, x, y, w, h, fadeVectorColor(interfacePanelFill, alpha))
		return
	}
	plan := ui.patternPlans.get(spec.name, src, &spec.pattern, w, h, spec.scale)
	op := &ebiten.DrawImageOptions{}
	op.ColorScale = tint
	// Only the perimeter comes from art. All panels share a solid interior.
	drawFilledRect(screen, x, y, w, h, fadeVectorColor(interfacePanelFill, alpha))
	center := src.Bounds().Inset(spec.pattern.slice)
	for _, part := range plan.ops {
		if part.part.Bounds() == center {
			continue
		}
		graphics.DrawImageScaled(screen, part.part, float64(x+part.dx), float64(y+part.dy), float64(part.dw), float64(part.dh), op)
	}
}

func (ui *UISystem) drawButtonFrame(screen *ebiten.Image, x, y, w, h int, active bool) {
	var tint ebiten.ColorScale
	if active {
		tint.Scale(1.35, 1.35, 1.35, 1)
	}
	// Hover changes brightness, never the authored rail or corner geometry.
	ui.drawThemeFrameTint(screen, frameBronze, x, y, w, h, tint)
}

// Portraits keep a quiet continuous trim; ornamental panel corners do not
// belong on the small image aperture.
func drawPortraitFrame(screen *ebiten.Image, x, y, w, h int) {
	drawFilledRect(screen, x, y, w, h, color.RGBA{20, 18, 16, 255})
	drawRectBorder(screen, x+1, y+1, w-2, h-2, 1, color.RGBA{154, 126, 76, 255})
}

const (
	decorTopCorners    = 0b0011
	decorBottomCorners = 0b1100
	decorAllCorners    = decorTopCorners | decorBottomCorners
)

// Decoration is opt-in at major panel call sites, never part of a frame.
// Forty-pixel corners retain their silhouette without crowding the content.
func (ui *UISystem) drawCornerDecor(screen *ebiten.Image, style interfaceFrame, x, y, w, h int, corners uint8) {
	if ui.game.sprites == nil {
		return
	}
	const size = 40
	metal := [...]string{"gold", "silver", "bronze"}[style]
	for i, suffix := range [...]string{"", "_tr", "_bl", "_br"} {
		if corners&(1<<i) == 0 {
			continue
		}
		name := "theme_corner_" + metal + suffix
		if !ui.game.sprites.HasSprite(name) {
			continue
		}
		dx, dy := x, y
		if i%2 != 0 {
			dx += w - size
		}
		if i/2 != 0 {
			dy += h - size
		}
		drawImageScaled(screen, ui.game.sprites.GetSprite(name), dx, dy, size, size)
	}
}

func (ui *UISystem) drawPanelInlay(screen *ebiten.Image, style interfaceFrame, centerX, railY int) {
	name := "theme_inlay_" + [...]string{"gold", "silver", "bronze"}[style]
	if ui.game.sprites != nil && ui.game.sprites.HasSprite(name) {
		drawImageScaled(screen, ui.game.sprites.GetSprite(name), centerX-24, railY-12, 48, 24)
	}
}
