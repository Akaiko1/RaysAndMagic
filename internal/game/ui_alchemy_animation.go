package game

import (
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

func brewTint(rgb [3]int, light, alpha float64) color.NRGBA {
	return color.NRGBA{uint8(min(255, float64(rgb[0])*light)), uint8(min(255, float64(rgb[1])*light)), uint8(min(255, float64(rgb[2])*light)), uint8(255 * alpha)}
}

// Logical positions beside the vessel, shared by icons and their streams.
// Pooled sources may consume several distinct materials in the same batch.
func alchemyBrewIngredientRect(index, count int) layoutRect {
	rows := max(1, (count+1)/2)
	size := min(30, 156/rows-4)
	stride := min(42, 156/rows)
	return layoutRect{54 + (index%2)*212 - size/2, 48 + (index/2)*stride, size, size}
}

func (ui *UISystem) drawAlchemyBrewAnimation(screen *ebiten.Image, area layoutRect) {
	fx := ui.game.brewAnimation
	if fx == nil {
		return
	}
	p := min(1, float64(fx.Frames)/float64(max(1, fx.Duration)))
	seconds := float64(fx.Frames) / float64(ui.game.config.GetTPS())
	rgb := fx.Color
	if rgb == [3]int{} {
		rgb = [3]int{90, 195, 165}
	}
	ui.rareBookText(screen, fx.Output.Name, layoutRect{area.x + 12, area.y + 12, area.w - 24, 32})
	stage := "Infusing ingredients"
	if p >= .32 {
		stage = "Bringing to a simmer"
	}
	if p >= .64 {
		stage = "Bottling"
	}
	if p >= .82 {
		stage = "Batch complete"
	}
	canvas := layoutRect{area.x + 12, area.y + 48, area.w - 24, area.h - 112}
	scale := min(float64(canvas.w)/320, float64(canvas.h)/210)
	ox := float64(canvas.x) + (float64(canvas.w)-320*scale)/2
	oy := float64(canvas.y) + (float64(canvas.h)-210*scale)/2
	x := func(v float64) float32 { return float32(ox + v*scale) }
	y := func(v float64) float32 { return float32(oy + v*scale) }
	circle := func(cx, cy, r float64, col color.Color) {
		vector.FillCircle(screen, x(cx), y(cy), float32(r*scale), col, true)
	}
	line := func(ax, ay, bx, by, w float64, col color.Color) {
		vector.StrokeLine(screen, x(ax), y(ay), x(bx), y(by), float32(w*scale), col, true)
	}
	// Copper burner and a low, colored pool of light under the vessel.
	for i := 5; i >= 1; i-- {
		circle(160, 163, float64(29+i*8), brewTint(rgb, .55, .012))
	}
	for i := 0; i < 3; i++ {
		flame := 8 + 3*math.Sin(seconds*12+float64(i))
		circle(148+float64(i)*12, 185-flame/2, flame, color.NRGBA{217, 99, 29, 130})
		circle(148+float64(i)*12, 186-flame/3, flame*.5, color.NRGBA{255, 220, 115, 210})
	}
	line(106, 179, 214, 179, 6, color.NRGBA{54, 38, 29, 255})
	line(106, 177, 214, 177, 2, color.NRGBA{202, 157, 80, 255})
	line(122, 179, 115, 194, 4, color.NRGBA{116, 87, 54, 255})
	line(198, 179, 205, 194, 4, color.NRGBA{116, 87, 54, 255})
	line(108, 195, 212, 195, 5, color.NRGBA{85, 64, 41, 255})

	if p < .82 {
		fill := min(.86, .12+p*1.15)
		top := 164 - 82*fill
		halfWidth := func(yy float64) float64 {
			if yy < 78 {
				return 13
			}
			if yy <= 153 {
				return 13 + (yy-78)*.64
			}
			return 61 - (yy-153)*1.25
		}
		// The liquid surface oscillates; the gradient and moving bands stay inside
		// the flask silhouette, including its rounded lower corners.
		for row := int(top); row <= 164; row++ {
			yy := float64(row)
			hw := halfWidth(yy) - 3
			wave := math.Sin(seconds*5 + yy*.12)
			light := .50 + .35*(164-yy)/max(1, 164-top) + .12*wave
			line(160-hw, yy, 160+hw, yy, 1.3, brewTint(rgb, light, .95))
		}
		surfaceWidth := halfWidth(top) - 4
		for i := 0; i < 30; i++ {
			a := float64(i) / 30
			b := float64(i+1) / 30
			ay := top + 2*math.Sin(a*math.Pi*2+seconds*5)
			by := top + 2*math.Sin(b*math.Pi*2+seconds*5)
			line(160-surfaceWidth+2*surfaceWidth*a, ay, 160-surfaceWidth+2*surfaceWidth*b, by, 2, brewTint(rgb, 1.25, .95))
		}
		for i := 0; i < 4; i++ {
			yy := top + 12 + float64(i)*11
			if yy > 160 {
				continue
			}
			hw := halfWidth(yy) - 10
			shift := math.Sin(seconds*4-float64(i)) * hw * .24
			line(160-hw*.65+shift, yy+2, 160+hw*.65+shift, yy-2, 2, brewTint(rgb, 1.25, .34))
		}
		// Bubbles rise through the liquid and pop at the moving surface.
		for i := 0; i < 12; i++ {
			phase := math.Mod(seconds*(.65+float64(i%3)*.1)+float64(i)*.173, 1)
			yy := 161 - phase*(161-top-4)
			xx := 160 + math.Sin(float64(i)*2.4)*min(halfWidth(yy)-8, 34) + math.Sin(seconds*3+float64(i))*2
			vector.StrokeCircle(screen, x(xx), y(yy), float32((1.5+float64(i%3))*scale), float32(scale), brewTint(rgb, 1.65, .65*(1-phase)), true)
		}
		// Glass silhouette and two narrow specular edges.
		var glass vector.Path
		glass.MoveTo(x(145), y(43))
		glass.LineTo(x(145), y(77))
		glass.LineTo(x(103), y(145))
		glass.QuadTo(x(94), y(164), x(116), y(170))
		glass.LineTo(x(204), y(170))
		glass.QuadTo(x(226), y(164), x(217), y(145))
		glass.LineTo(x(175), y(77))
		glass.LineTo(x(175), y(43))
		op := &vector.DrawPathOptions{AntiAlias: true}
		op.ColorScale.ScaleWithColor(color.NRGBA{7, 18, 22, 255})
		vector.StrokePath(screen, &glass, &vector.StrokeOptions{Width: float32(6 * scale)}, op)
		op.ColorScale.Reset()
		op.ColorScale.ScaleWithColor(color.NRGBA{108, 175, 184, 235})
		vector.StrokePath(screen, &glass, &vector.StrokeOptions{Width: float32(2 * scale)}, op)
		line(150, 50, 150, 76, 2, color.NRGBA{215, 243, 236, 220})
		line(144, 90, 119, 133, 3, color.NRGBA{209, 240, 231, 130})
		line(209, 147, 214, 155, 2, color.NRGBA{199, 235, 233, 195})
		line(140, 43, 180, 43, 5, color.NRGBA{81, 102, 95, 255})
		line(140, 41, 180, 41, 2, color.NRGBA{224, 221, 171, 255})
		// Coherent curling vapor from the open neck, with a gentle sideways drift.
		for i := 0; i < 16; i++ {
			phase := math.Mod(seconds*.48+float64(i)*.071, 1)
			xx := 160 + math.Sin(phase*5+seconds*1.4)*7 + phase*8
			yy := 39 - phase*36
			circle(xx, yy, 2+phase*5, color.NRGBA{185, 219, 207, uint8((1 - phase) * 38)})
		}
		if p < .64 {
			for i, it := range fx.Ingredients {
				r := alchemyBrewIngredientRect(i, len(fx.Ingredients))
				ix, iy := float64(r.x+r.w/2), float64(r.y+r.h/2)
				ui.drawInventoryItemIcon(screen, it, int(x(float64(r.x))), int(y(float64(r.y))), int(float64(r.w)*scale), int(float64(r.h)*scale), 1, true)
				for j := 0; j < 8; j++ {
					t := math.Mod(seconds*.8+float64(j)*.125, 1)
					px := ix + (160-ix)*t
					py := iy + (45-iy)*t - 45*math.Sin(t*math.Pi)
					circle(px, py, 1.3, brewTint(rgb, 1.6, .8))
				}
			}
		}
	} else {
		// The completed bottle replaces the vessel in a short, restrained starburst.
		reveal := min(1, (p-.82)/.06)
		for i := 0; i < 10; i++ {
			a := float64(i)*math.Pi/5 + seconds*.2
			radius := 48 + 18*reveal
			xx, yy := 160+math.Cos(a)*radius, 106+math.Sin(a)*radius
			line(xx-3, yy, xx+3, yy, 1, brewTint(rgb, 1.5, .7))
			line(xx, yy-3, xx, yy+3, 1, brewTint(rgb, 1.5, .7))
		}
		size := int((72 + 12*reveal) * scale)
		ui.drawInventoryItemIcon(screen, fx.Output, int(x(160))-size/2, int(y(106))-size/2, size, size, 2, true)
	}
	drawCenteredDebugText(screen, stage, area.x+12, area.bottom()-60, area.w-24, 18)
	drawCenteredDebugText(screen, fmt.Sprintf("%d items prepared", fx.Count), area.x+12, area.bottom()-38, area.w-24, 16)
	bar := layoutRect{area.x + 20, area.bottom() - 14, area.w - 40, 4}
	drawFilledRect(screen, bar.x, bar.y, bar.w, bar.h, color.NRGBA{40, 45, 37, 255})
	drawFilledRect(screen, bar.x, bar.y, int(float64(bar.w)*p), bar.h, brewTint(rgb, 1, 1))
}
