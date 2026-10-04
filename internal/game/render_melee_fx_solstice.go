package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"math"
)

var solsticeFire = [3]int{255, 117, 40}
var solsticeWater = [3]int{57, 193, 226}
var solsticeEarth = [3]int{99, 197, 124}
var solsticeAir = [3]int{188, 224, 255}

// The lance compresses three heat collars around a rigid needle, then sheds
// short perpendicular exhaust strokes. Its silhouette never becomes a slash.
func (r *Renderer) drawMeleeFxSolsticeThermal(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	p, fade, _, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	h, w := arenaFxScale(s, screenH)
	path := func(t float64) (float64, float64) { return cx + h*(.17-.24*t), cy + h*(.30-.55*t) }
	r.drawDissolveStroke(screen, dissolveStroke{path: path, width: func(t float64) float64 { return (9 - 7*t) * w }, color: func(t float64) [3]int { return mixColor(solsticeFire, [3]int{255, 249, 201}, t) }, alpha: func(float64) float64 { return .95 }, length: h * .6, seed: seedFromID(s.ID), salt: 701, blend: additiveGlowBlend}, lead, p)
	for k := 0; k < 3; k++ {
		t := lead * (.42 + .18*float64(k))
		x, y := path(t)
		span := h * (.035 + .025*(1-lead))
		r.fxSegment(screen, x-span, y+span*.4, x+span, y-span*.4, math.Max(1.5, 2*w), solsticeFire, fade, additiveGlowBlend)
	}
	if p > .30 {
		u := (p - .30) / .7
		x, y := path(1)
		for _, side := range []float64{-1, 1} {
			for k := 0; k < 3; k++ {
				d := h * (.03 + u*.18)
				dy := float64(k-1) * h * .035
				r.fxSegment(screen, x+side*d*.55, y+dy, x+side*d, y+dy+h*.04*u, math.Max(1, 3*w*(1-u)), mixColor(solsticeFire, clockCopper, u), fade*(1-u), additiveGlowBlend)
			}
		}
	}
}

// A squared head descends between two load rails. The landing folds jade
// braces inward; four angular cracks spread under it, with no circular nova.
func (r *Renderer) drawMeleeFxSolsticeAnchor(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	p, fade, _, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	h, w := arenaFxScale(s, screenH)
	y := cy - h*.28 + h*.44*lead*lead
	for _, side := range []float64{-1, 1} {
		x := cx + side*h*(.14-.08*lead)
		r.fxSegment(screen, x, cy-h*.22, x, y, math.Max(1, 3*w), clockBrass, fade*.65, additiveGlowBlend)
	}
	r.drawGlowRectRotated(screen, cx, y, h*.17, h*.085, 0, clockCopper, fade, ebiten.BlendSourceOver)
	r.drawGlowRectRotated(screen, cx, y, h*.13, h*.068, 0, solsticeEarth, fade, ebiten.BlendSourceOver)
	r.fxSegment(screen, cx-h*.075, y-h*.038, cx+h*.075, y-h*.038, math.Max(1, 2*w), [3]int{222, 255, 208}, fade, additiveGlowBlend)
	if p > .28 {
		u := (p - .28) / .72
		by := cy + h*.16
		for k := 0; k < 4; k++ {
			a := math.Pi*.25 + float64(k)*math.Pi*.5
			x1, y1 := cx+math.Cos(a)*h*.08*u, by+math.Sin(a)*h*.035*u
			x2, y2 := cx+math.Cos(a+.35)*h*.18*u, by+math.Sin(a+.35)*h*.09*u
			x3, y3 := cx+math.Cos(a)*h*.28*u, by+math.Sin(a)*h*.13*u
			r.fxSegment(screen, cx, by, x1, y1, 2*w, solsticeEarth, fade, additiveGlowBlend)
			r.fxSegment(screen, x1, y1, x2, y2, 2*w, solsticeEarth, fade, additiveGlowBlend)
			r.fxSegment(screen, x2, y2, x3, y3, w, solsticeEarth, fade, additiveGlowBlend)
		}
	}
}

// The vector blade makes one oblique cut. Its wake folds through right-angle
// elbows on opposing sides, then slides apart into thin offset afterimages.
func (r *Renderer) drawMeleeFxSolsticeTransfer(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	p, fade, _, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	h, w := arenaFxScale(s, screenH)
	path := func(t float64) (float64, float64) { return cx + h*(t-.5)*.52, cy + h*(.5-t)*.28 }
	r.drawDissolveStroke(screen, dissolveStroke{path: path, width: func(float64) float64 { return 4 * w }, color: func(float64) [3]int { return solsticeAir }, alpha: func(float64) float64 { return .9 }, length: h * .60, seed: seedFromID(s.ID), salt: 731, blend: additiveGlowBlend}, lead, p)
	for _, side := range []float64{-1, 1} {
		u := math.Min(1, p*2.5)
		x := cx + side*h*(.08+.13*u)
		y := cy - side*h*.08
		r.fxSegment(screen, x-side*h*.06, y, x, y, 2*w, solsticeAir, fade, additiveGlowBlend)
		r.fxSegment(screen, x, y, x, y+side*h*.09, 2*w, solsticeAir, fade, additiveGlowBlend)
		for k := 0; k < 3; k++ {
			off := side * h * (.025*float64(k+1) + .06*p)
			x0, y0 := path(.2)
			x1, y1 := path(.8 * lead)
			r.fxSegment(screen, x0, y0+off, x1, y1+off, w, solsticeAir, fade*.30, additiveGlowBlend)
		}
	}
}

// Water travels as two counterflowing ribbons enclosing a clear reservoir.
// A side projection reveals the wake; head-on shows its double spiral mouth.
func (r *Renderer) drawWeaponProjectileFxSolsticeFlow(screen *ebiten.Image, cx, cy, size, dx, dy, crit float64, id int) {
	angle := float64(r.game.frameCount)*.12 + float64(id%11)
	for strand := 0; strand < 2; strand++ {
		phase := angle + float64(strand)*math.Pi
		lastX, lastY := 0., 0.
		for k := 0; k < 18; k++ {
			t := float64(k) / 17
			wave := math.Sin(t*math.Pi*2+phase) * size * (.6 + .35*t)
			x, y := cx-dx*size*t*3-dy*wave, cy-dy*size*t*3+dx*wave
			if k > 0 {
				r.fxSegment(screen, lastX, lastY, x, y, math.Max(1.5, size*.16), mixColor(solsticeWater, [3]int{220, 255, 246}, float64(strand)*.55), crit*(.85-.5*t), additiveGlowBlend)
			}
			lastX, lastY = x, y
		}
	}
}

func (r *Renderer) drawWeaponProjectileFxSolsticeFlowHeadOn(screen *ebiten.Image, cx, cy, size, crit float64, id int) {
	phase := float64(r.game.frameCount)*.12 + float64(id%11)
	for strand := 0; strand < 2; strand++ {
		lastX, lastY := 0., 0.
		for k := 0; k < 20; k++ {
			t := float64(k) / 19
			a := phase + float64(strand)*math.Pi + t*math.Pi*1.4
			rad := size * (.35 + .85*t)
			x, y := cx+math.Cos(a)*rad, cy+math.Sin(a)*rad
			if k > 0 {
				r.fxSegment(screen, lastX, lastY, x, y, math.Max(1.5, size*.14), mixColor(solsticeWater, [3]int{220, 255, 246}, t), crit*(.55+.4*t), additiveGlowBlend)
			}
			lastX, lastY = x, y
		}
	}
}

func (r *Renderer) drawElementalWeaponMarks(screen *ebiten.Image, s UnifiedSpriteRenderData, top int) {
	m := s.monster
	x, y := float64(s.screenX), float64(top)-5
	size := math.Max(3, math.Min(9, float64(s.spriteSize)*.04))
	// Stable slots avoid jitter when another mark expires.
	for slot, kind := range []string{"pressure", "anchor", "crosswind"} {
		mark, ok := m.ElementalMarks[kind]
		if !ok || mark.Frames <= 0 || mark.Turns <= 0 {
			continue
		}
		cx := x + (float64(slot)-1)*size*3.5
		switch kind {
		case "pressure":
			for k := 0; k < mark.Count; k++ {
				r.drawGlowRect(screen, cx+float64(k)*size*.7, y, size*.6, solsticeFire, .9, additiveGlowBlend)
			}
		case "anchor":
			for k := 0; k < mark.Count; k++ {
				r.drawGlowRect(screen, cx+float64(k%5-2)*size*.5, y-float64(k/5)*size*.6, size*.35, solsticeEarth, .9, additiveGlowBlend)
			}
		case "crosswind":
			for k := 0; k < mark.Count; k++ {
				ax := cx + float64(k)*size*.8
				r.fxSegment(screen, ax-size*.5, y+size*.5, ax, y, 1.5, solsticeAir, .9, additiveGlowBlend)
				r.fxSegment(screen, ax, y, ax-size*.5, y-size*.5, 1.5, solsticeAir, .9, additiveGlowBlend)
			}
		}
	}
}
