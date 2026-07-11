package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Arena weapon flourishes are deliberately grounded in the weapon's fighting
// style: short cuts, drilled thrusts, parries, and impact marks. They use the
// same dissolve-ribbon vocabulary as the legendary effects, so the arena set
// reads as one family without looking like recoloured category swings.

func arenaFxScale(s SlashEffect, screenH float64) (h, width float64) {
	h, width = screenH*meleeSizeScale, 1
	if s.Crit {
		h *= 1.25
		width = 1.3
	}
	return h, width
}

func (r *Renderer) drawMeleeFxArenaGladius(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, _, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	bronze, hot := [3]int{216, 180, 120}, [3]int{255, 248, 225}
	reach := h * 0.22
	// Two compact crossing cuts: the close-in finishing style of a gladius.
	for i, sign := range []float64{-1, 1} {
		lag := float64(i) * 0.10
		p := (progress - lag) / (1 - lag)
		if p <= 0 {
			continue
		}
		p = math.Min(p, 1)
		r.drawDissolveStroke(screen, dissolveStroke{
			path: func(t float64) (float64, float64) {
				return cx + sign*reach*(0.9-t*1.8), cy + h*0.075 - reach*(0.2+t*1.15)
			},
			width:  func(t float64) float64 { return (3.5 + 4*(1-t)) * w },
			color:  func(t float64) [3]int { return mixColor(bronze, hot, t) },
			alpha:  func(t float64) float64 { return 0.45 + 0.5*t },
			length: reach * 2.2, seed: seed, salt: 210 + i, blend: additiveGlowBlend,
		}, lead, p)
	}
	for k := 0; k < 7; k++ {
		t := auraHash(seed, k, 212, 0)
		if progress < t*meleeSweepFrac {
			continue
		}
		x := cx + (auraHash(seed, k, 213, 0)-0.5)*reach*1.4
		y := cy - reach*(0.2+t) + (auraHash(seed, k, 214, 0)-0.5)*reach*0.5
		r.drawGlowRect(screen, x, y, math.Max(2, h*0.012), hot, fade*(1-t)*0.8, additiveGlowBlend)
	}
}

func (r *Renderer) drawMeleeFxArenaLabrys(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, _, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	bronze, edge := [3]int{190, 145, 72}, [3]int{255, 231, 160}
	reach := h * 0.28
	for i, sign := range []float64{-1, 1} {
		pivotX, pivotY := cx+sign*reach*0.20, cy+reach*0.05
		start, end := -math.Pi/2-sign*0.95, -math.Pi/2+sign*0.72
		r.drawDissolveStroke(screen, dissolveStroke{
			path: func(t float64) (float64, float64) {
				a := start + (end-start)*t
				return pivotX + math.Cos(a)*reach, pivotY + math.Sin(a)*reach
			},
			width:  func(t float64) float64 { return (5 + 9*math.Sin(math.Pi*t)) * w },
			color:  func(t float64) [3]int { return mixColor(bronze, edge, 0.25+0.75*t) },
			alpha:  func(t float64) float64 { return 0.35 + 0.55*t },
			length: reach * math.Abs(end-start), seed: seed, salt: 220 + i, blend: additiveGlowBlend,
		}, lead, progress)
	}
	for k := 0; k < 10; k++ {
		u := auraHash(seed, k, 222, 0)
		if progress < meleeSweepFrac*u {
			continue
		}
		x := cx + (auraHash(seed, k, 223, 0)-0.5)*reach*2.1
		y := cy - reach*0.15 + u*h*0.16
		r.drawGlowRect(screen, x, y, math.Max(2, h*0.014), bronze, fade*(1-u)*0.65, additiveGlowBlend)
	}
}

func (r *Renderer) drawMeleeFxArenaMorningstar(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	_, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	iron, flash := [3]int{148, 160, 180}, [3]int{240, 248, 255}
	reach := h * 0.23
	pivotX, pivotY := cx-reach*0.45, cy+reach*0.45
	angle := -2.35 + sweepT*2.6
	headX, headY := pivotX+math.Cos(angle)*reach, pivotY+math.Sin(angle)*reach
	// The links reveal the swinging chain, then linger as the bell-like hit rings.
	for i := 1; i <= 9; i++ {
		t := float64(i) / 10
		if t > lead {
			break
		}
		x := pivotX + (headX-pivotX)*t
		y := pivotY + (headY-pivotY)*t + math.Sin(t*math.Pi)*reach*0.12
		r.drawGlowSprite(screen, x, y, h*(0.018+0.006*t)*w, iron, fade*(0.35+0.45*t), additiveGlowBlend)
	}
	r.drawGlowSprite(screen, headX, headY, h*0.075*w, iron, fade*0.9, additiveGlowBlend)
	r.drawGlowSprite(screen, headX, headY, h*0.035*w, flash, fade*0.95, additiveGlowBlend)
	if sweepT > 0.55 {
		ring := reach * (0.28 + 0.4*(sweepT-0.55)/0.45)
		for i := 0; i < 10; i++ {
			a := 2 * math.Pi * float64(i) / 10
			r.drawGlowRect(screen, headX+math.Cos(a)*ring, headY+math.Sin(a)*ring, math.Max(2, h*0.012), flash, fade*0.45, additiveGlowBlend)
		}
	}
	_ = seed // keeps deterministic IDs available when this effect gets more debris.
}

func (r *Renderer) drawMeleeFxArenaHasta(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	steel, amber := [3]int{190, 180, 170}, [3]int{245, 205, 105}
	reach := h * 0.39
	for i, offset := range []float64{-0.045, 0, 0.045} {
		off := offset * h
		lag := math.Abs(offset) * 0.85
		p := math.Max(0, (progress-lag)/(1-lag))
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   func(t float64) (float64, float64) { return cx + off*(1+0.25*t), cy + reach*0.45 - reach*t },
			width:  func(t float64) float64 { return (2.5 + 2.5*(1-t)) * w },
			color:  func(t float64) [3]int { return mixColor(steel, [3]int{255, 255, 245}, t) },
			alpha:  func(t float64) float64 { return 0.45 + 0.45*t },
			length: reach, seed: seed, salt: 230 + i, blend: additiveGlowBlend,
		}, lead, p)
	}
	if sweepT < 1 {
		for i := 0; i < 5; i++ {
			a := math.Pi + math.Pi*float64(i)/4
			r.drawGlowSprite(screen, cx+math.Cos(a)*h*0.14, cy+h*0.20+math.Sin(a)*h*0.08,
				h*0.025*w, amber, fade*0.55, additiveGlowBlend)
		}
	}
}

func (r *Renderer) drawMeleeFxArenaTrident(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, _, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	sea, point := [3]int{100, 190, 210}, [3]int{225, 250, 255}
	reach := h * 0.38
	for i, offset := range []float64{-0.075, 0, 0.075} {
		off := offset * h
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   func(t float64) (float64, float64) { return cx + off*(1-t*0.25), cy + reach*0.42 - reach*t },
			width:  func(t float64) float64 { return (2.8 + 2*(1-t)) * w },
			color:  func(t float64) [3]int { return mixColor(sea, point, t) },
			alpha:  func(t float64) float64 { return 0.4 + 0.5*t },
			length: reach, seed: seed, salt: 240 + i, blend: additiveGlowBlend,
		}, lead, progress)
	}
	// A small net closes after the three points have passed, tying the root rider to the visual.
	if progress > meleeSweepFrac*0.45 {
		for row := 0; row < 3; row++ {
			y := cy - reach*(0.25+0.13*float64(row))
			for col := 0; col < 4; col++ {
				x := cx + (float64(col)-1.5)*h*0.055 + float64(row%2)*h*0.02
				r.drawGlowRect(screen, x, y, math.Max(2, h*0.011*w), sea, fade*0.42, additiveGlowBlend)
			}
		}
	}
}

func (r *Renderer) drawMeleeFxArenaParry(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, _, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	silver, reply := [3]int{220, 228, 242}, [3]int{255, 160, 125}
	reach := h * 0.19
	// The broad first curve catches an opponent blade; the narrow delayed line is the riposte.
	r.drawDissolveStroke(screen, dissolveStroke{
		path: func(t float64) (float64, float64) {
			a := math.Pi*0.18 + math.Pi*0.82*t
			return cx + math.Cos(a)*reach, cy + h*0.07 + math.Sin(a)*reach*0.65
		},
		width:  func(t float64) float64 { return (3 + 4*math.Sin(math.Pi*t)) * w },
		color:  func(t float64) [3]int { return mixColor(silver, [3]int{255, 255, 255}, t) },
		alpha:  func(t float64) float64 { return 0.45 + 0.45*t },
		length: reach * 2.5, seed: seed, salt: 250, blend: additiveGlowBlend,
	}, lead, progress)
	lag := 0.10
	if p := (progress - lag) / (1 - lag); p > 0 {
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   func(t float64) (float64, float64) { return cx - h*0.035 + h*0.07*t, cy + h*0.13 - reach*1.25*t },
			width:  func(t float64) float64 { return (2.4 + 2*(1-t)) * w },
			color:  func(t float64) [3]int { return mixColor(reply, [3]int{255, 245, 235}, t) },
			alpha:  func(t float64) float64 { return 0.45 + 0.5*t },
			length: reach * 1.3, seed: seed, salt: 251, blend: additiveGlowBlend,
		}, math.Min(1, lead*1.2), math.Min(1, p))
	}
}

func (r *Renderer) drawMeleeFxArenaLion(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	gold, white := [3]int{230, 185, 75}, [3]int{255, 245, 195}
	reach := h * 0.28
	r.drawDissolveStroke(screen, dissolveStroke{
		path: func(t float64) (float64, float64) {
			a := -math.Pi/2 - 0.95 + 1.9*t
			return cx + math.Cos(a)*reach, cy + h*0.12 + math.Sin(a)*reach
		},
		width:  func(t float64) float64 { return (6 + 10*math.Sin(math.Pi*t)) * w },
		color:  func(t float64) [3]int { return mixColor(gold, white, t) },
		alpha:  func(t float64) float64 { return 0.45 + 0.5*t },
		length: reach * 1.9, seed: seed, salt: 260, blend: additiveGlowBlend,
	}, lead, progress)
	if sweepT > 0.55 {
		impact := (sweepT - 0.55) / 0.45
		for i := 0; i < 8; i++ {
			a := 2 * math.Pi * float64(i) / 8
			radius := reach * (0.25 + 0.7*impact)
			r.drawGlowSprite(screen, cx+math.Cos(a)*radius, cy-h*0.12+math.Sin(a)*radius*0.58,
				h*0.026*w, gold, fade*(1-impact*0.35), additiveGlowBlend)
		}
		r.drawSparkStar(screen, cx, cy-h*0.12, h*0.052*w, gold, white, fade, 1.35)
	}
}

func (r *Renderer) drawMeleeFxArenaCesti(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, _, _ := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	bronze, hot := [3]int{200, 138, 76}, [3]int{255, 225, 170}
	reach := h * 0.20
	for i, side := range []float64{-1, 1} {
		lag := float64(i) * 0.12
		p := (progress - lag) / (1 - lag)
		if p <= 0 {
			continue
		}
		p = math.Min(1, p)
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   func(t float64) (float64, float64) { return cx + side*h*0.10*(1-t), cy + h*0.13 - reach*t },
			width:  func(t float64) float64 { return (4.5 + 4*(1-t)) * w },
			color:  func(t float64) [3]int { return mixColor(bronze, hot, t) },
			alpha:  func(t float64) float64 { return 0.45 + 0.5*t },
			length: reach, seed: seed, salt: 270 + i, blend: additiveGlowBlend,
		}, 1-(1-math.Min(1, p/meleeSweepFrac))*(1-math.Min(1, p/meleeSweepFrac)), p)
		if p < meleeSweepFrac {
			r.drawSparkStar(screen, cx+side*h*0.02, cy-reach*0.72, h*0.027*w, bronze, hot, fade, 0.7)
		}
	}
}
