package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Bespoke swing flourishes for rare melee weapons + the naginata (whose wide
// reaping sweep earns one despite its tier). Same building blocks as the
// legendaries: dissolve-stroke ribbons + deterministic particles.

// Silver Sword — a slim, holy-bright crescent of moonlit silver; tiny white
// star-glints kindle along the wake and sink softly as they gutter out.
func (r *Renderer) drawMeleeFxSilverSword(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	fc := float64(r.game.frameCount)
	h := screenH * meleeSizeScale
	silver := [3]int{225, 235, 250}
	white := [3]int{255, 255, 255}
	glints := 6
	widthScale := 1.0
	if s.Crit {
		h *= 1.25
		widthScale = 1.3
		glints = 10
	}

	reach := h * 0.22
	pivotX, pivotY := cx, cy+reach*0.95
	R := reach * 1.7
	thetaStart, thetaEnd := -math.Pi/2-0.6, -math.Pi/2+0.6
	thick := h * 0.05
	arcAt := func(t float64) (float64, float64) {
		theta := thetaStart + (thetaEnd-thetaStart)*t
		return pivotX + math.Cos(theta)*R, pivotY + math.Sin(theta)*R
	}

	r.drawDissolveStroke(screen, dissolveStroke{
		path:   arcAt,
		width:  func(t float64) float64 { return (3.5 + 6*math.Sin(math.Pi*t)) * widthScale },
		color:  func(t float64) [3]int { return mixColor(silver, white, t) },
		alpha:  func(t float64) float64 { return 0.5 + 0.5*t },
		length: R * (thetaEnd - thetaStart),
		seed:   seed, salt: 11, blend: additiveGlowBlend,
	}, lead, progress)
	if sweepT < 1 {
		tx, ty := arcAt(lead)
		r.drawGlowSprite(screen, tx, ty, thick*1.8, white, fade, additiveGlowBlend)
	}

	// Star-glints kindling along the wake.
	for k := 0; k < glints; k++ {
		tb := auraHash(seed, k, 12, 0)
		born := tb * meleeSweepFrac
		if progress <= born {
			continue
		}
		u := (progress - born) / (1 - born)
		gx, gy := arcAt(tb)
		gy += u * h * 0.05
		tw := 0.5 + 0.5*math.Sin(fc*0.4+auraHash(seed, k, 13, 0)*2*math.Pi)
		r.drawSparkStar(screen, gx, gy, math.Max(2, thick*0.5*(1-u*0.4)),
			silver, white, fade*(1-u)*tw, 1)
	}
}

// Gold Sword — a warm gilded crescent shedding a slow rain of twinkling gold
// dust, wealth spilling off the edge.
func (r *Renderer) drawMeleeFxGoldSword(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	fc := float64(r.game.frameCount)
	h := screenH * meleeSizeScale
	gold := [3]int{255, 205, 90}
	hot := [3]int{255, 245, 200}
	dust := 14
	widthScale := 1.0
	if s.Crit {
		h *= 1.25
		widthScale = 1.3
		dust = 22
	}

	reach := h * 0.22
	pivotX, pivotY := cx, cy+reach*0.95
	R := reach * 1.7
	thetaStart, thetaEnd := -math.Pi/2-0.6, -math.Pi/2+0.6
	thick := h * 0.055
	arcAt := func(t float64) (float64, float64) {
		theta := thetaStart + (thetaEnd-thetaStart)*t
		return pivotX + math.Cos(theta)*R, pivotY + math.Sin(theta)*R
	}

	r.drawDissolveStroke(screen, dissolveStroke{
		path:   arcAt,
		width:  func(t float64) float64 { return (4 + 7*math.Sin(math.Pi*t)) * widthScale },
		color:  func(t float64) [3]int { return mixColor(gold, hot, t) },
		alpha:  func(t float64) float64 { return 0.5 + 0.5*t },
		length: R * (thetaEnd - thetaStart),
		seed:   seed, salt: 14, blend: additiveGlowBlend,
	}, lead, progress)
	if sweepT < 1 {
		tx, ty := arcAt(lead)
		r.drawGlowSprite(screen, tx, ty, thick*1.8, hot, fade, additiveGlowBlend)
	}

	// Gold dust: heavy little motes sifting straight down, twinkling.
	for k := 0; k < dust; k++ {
		tb := auraHash(seed, k, 15, 0)
		born := tb * meleeSweepFrac
		if progress <= born {
			continue
		}
		u := (progress - born) / (1 - born)
		gx, gy := arcAt(tb)
		gx += (auraHash(seed, k, 16, 0) - 0.5) * thick * 2.5
		gy += u * h * 0.18 * (0.6 + auraHash(seed, k, 17, 0))
		tw := 0.5 + 0.5*math.Sin(fc*0.5+auraHash(seed, k, 18, 0)*2*math.Pi)
		r.drawGlowRect(screen, gx, gy, math.Max(2, thick*0.24*(1-u*0.5)),
			mixColor(gold, hot, tw), fade*(1-u)*(0.4+0.6*tw), additiveGlowBlend)
	}
}

// Agility Katar — three staggered lightning-quick punches, each a thin green
// streak in a narrow fan, with speed-lines whipping past the fist.
func (r *Renderer) drawMeleeFxAgilityKatar(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, _, _ := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h := screenH * meleeSizeScale
	green := [3]int{120, 225, 150}
	flash := [3]int{225, 255, 235}
	lines := 6
	widthScale := 1.0
	if s.Crit {
		h *= 1.25
		widthScale = 1.3
		lines = 10
	}

	reach := h * 0.2
	thick := h * 0.035
	baseY := cy + reach*0.5
	for i := 0; i < 3; i++ {
		lag := float64(i) * meleeSweepFrac * 0.3
		lp := progress - lag
		if lp <= 0 {
			continue
		}
		st := lp / meleeSweepFrac
		if st > 1 {
			st = 1
		}
		ld := 1 - (1-st)*(1-st)
		fan := (float64(i) - 1) * h * 0.045 // left / centre / right jab
		baseX := cx + fan
		lpDissolve := lp / (1 - lag)

		r.drawDissolveStroke(screen, dissolveStroke{
			path:   func(t float64) (float64, float64) { return baseX + fan*0.6*t, baseY - reach*t },
			width:  func(t float64) float64 { return (3 + 4*(1-t)) * widthScale },
			color:  func(t float64) [3]int { return mixColor(green, flash, t) },
			alpha:  func(t float64) float64 { return 0.45 + 0.55*t },
			length: reach,
			seed:   seed, salt: 19 + i, blend: additiveGlowBlend,
		}, ld, lpDissolve)
		if st < 1 {
			r.drawGlowSprite(screen, baseX+fan*0.6*ld, baseY-reach*ld, thick*1.4, flash, fade*0.9, additiveGlowBlend)
		}
	}

	// Speed-lines whipping backward past the strikes.
	for k := 0; k < lines; k++ {
		born := auraHash(seed, k, 23, 0) * 0.4
		if progress <= born {
			continue
		}
		u := (progress - born) / (1 - born)
		side := 1.0
		if k%2 == 0 {
			side = -1
		}
		lx := cx + side*h*(0.06+auraHash(seed, k, 24, 0)*0.06)
		ly := baseY - reach*auraHash(seed, k, 25, 0) + u*h*0.06
		for g := 0; g < 3; g++ {
			r.drawGlowSprite(screen, lx+side*float64(g)*thick*0.9, ly,
				math.Max(2, thick*0.3*(1-0.25*float64(g))), green,
				fade*(1-u)*(0.7-0.2*float64(g)), additiveGlowBlend)
		}
	}
}

// Gorehorn Greataxe — a brutal full chop trailing heavy gore: fat dark-red
// chunks tumble off the edge with wet trails, and the landing bursts red.
func (r *Renderer) drawMeleeFxGorehorn(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h := screenH * meleeSizeScale
	gore := [3]int{150, 25, 25}
	raw := [3]int{215, 60, 50}
	chunks := 8
	widthScale := 1.0
	if s.Crit {
		h *= 1.25
		widthScale = 1.3
		chunks = 13
	}

	reach := h * 0.24
	pivotX, pivotY := cx-reach*0.15, cy-reach*0.1
	R := reach * 1.25
	thetaStart, thetaEnd := -1.35, 0.85
	thick := h * 0.085
	arcAt := func(t float64) (float64, float64) {
		theta := thetaStart + (thetaEnd-thetaStart)*t
		return pivotX + math.Cos(theta)*R, pivotY + math.Sin(theta)*R
	}

	r.drawDissolveStroke(screen, dissolveStroke{
		path:   arcAt,
		width:  func(t float64) float64 { return (7 + 9*t) * widthScale },
		color:  func(t float64) [3]int { return mixColor(gore, raw, t) },
		alpha:  func(t float64) float64 { return 0.45 + 0.55*t },
		length: R * (thetaEnd - thetaStart),
		seed:   seed, salt: 26, blend: additiveGlowBlend,
	}, lead, progress)

	// Heavy gore chunks: fat, fast-falling, dragging wet trails.
	gorePos := func(k int, u float64) (float64, float64) {
		tb := auraHash(seed, k, 27, 0)
		gx, gy := arcAt(tb)
		gx += (auraHash(seed, k, 28, 0) - 0.5) * thick * 2
		gy += u*u*h*0.5 + u*thick
		return gx, gy
	}
	for k := 0; k < chunks; k++ {
		born := auraHash(seed, k, 27, 0) * meleeSweepFrac
		if progress <= born {
			continue
		}
		u := (progress - born) / (1 - born)
		for g := 0; g < 2; g++ {
			ug := u - float64(g)*0.05
			if ug < 0 {
				break
			}
			gx, gy := gorePos(k, ug)
			f := 1 - 0.35*float64(g)
			r.drawGlowSprite(screen, gx, gy, math.Max(2, thick*(0.55-0.2*u)*f),
				mixColor(gore, raw, auraHash(seed, k, 29, 0)*0.6), fade*(1-u)*0.9*f*f, additiveGlowBlend)
		}
	}

	// The chop lands: a short red burst at the arc's end.
	if sweepT >= 1 {
		u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
		if u < 0.3 {
			ex, ey := arcAt(1)
			flash := 1 - u/0.3
			r.drawGlowSprite(screen, ex, ey, thick*(1.5+2.5*flash), raw, fade*flash*0.9, additiveGlowBlend)
		}
	}
}

// Serpent-Fang — the stab itself snakes: a sinuous venom-green line weaving to
// the target, twin fang-prongs flashing at full extension, venom beads
// dripping off the wave.
func (r *Renderer) drawMeleeFxSerpentFang(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h := screenH * meleeSizeScale
	venom := [3]int{140, 220, 60}
	bile := [3]int{215, 245, 130}
	drips := 5
	widthScale := 1.0
	if s.Crit {
		h *= 1.25
		widthScale = 1.3
		drips = 9
	}

	reach := h * 0.24
	thick := h * 0.035
	baseX, baseY := cx, cy+reach*0.5
	wavePath := func(t float64) (float64, float64) {
		return baseX + math.Sin(t*3*math.Pi)*thick*2.2*(1-t*0.5), baseY - reach*t
	}

	r.drawDissolveStroke(screen, dissolveStroke{
		path:   wavePath,
		width:  func(t float64) float64 { return (3.5 + 4.5*(1-t)) * widthScale },
		color:  func(t float64) [3]int { return mixColor(venom, bile, t) },
		alpha:  func(t float64) float64 { return 0.5 + 0.5*t },
		length: reach * 1.25,
		seed:   seed, salt: 31, blend: additiveGlowBlend,
	}, lead, progress)

	// Twin fangs snap open at full extension.
	if progress >= meleeSweepFrac && progress < meleeSweepFrac+0.18 {
		u := (progress - meleeSweepFrac) / 0.18
		tipX, tipY := wavePath(1)
		fang := thick * (1.5 + 3*u)
		a := fade * (1 - u)
		for _, sgn := range [2]float64{-1, 1} {
			for st := 1; st <= 3; st++ {
				f := float64(st) / 3
				r.drawGlowSprite(screen, tipX+sgn*fang*f*0.5, tipY-fang*f,
					math.Max(2, thick*(0.6-0.14*float64(st))), bile, a*(1-0.2*float64(st)), additiveGlowBlend)
			}
		}
	} else if sweepT < 1 {
		tx, ty := wavePath(lead)
		r.drawGlowSprite(screen, tx, ty, thick*1.5, bile, fade*0.9, additiveGlowBlend)
	}

	// Venom beads dripping off the wave.
	for k := 0; k < drips; k++ {
		tb := auraHash(seed, k, 32, 0)
		born := tb * meleeSweepFrac
		if progress <= born {
			continue
		}
		u := (progress - born) / (1 - born)
		dx, dy := wavePath(tb)
		dy += u * u * h * 0.28
		r.drawGlowSprite(screen, dx, dy, math.Max(2, thick*0.45*(1-u*0.4)),
			venom, fade*(1-u)*0.85, additiveGlowBlend)
	}
}

// Naginata — "a moon's edge on a pole": the widest, flattest reaping crescent
// of all, pale moonlight with a faint outer halo, thin slivers spinning off
// the edge like cut stalks.
func (r *Renderer) drawMeleeFxNaginata(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h := screenH * meleeSizeScale
	moon := [3]int{215, 230, 220}
	pale := [3]int{250, 255, 250}
	slivers := 8
	widthScale := 1.0
	if s.Crit {
		h *= 1.25
		widthScale = 1.3
		slivers = 13
	}

	reach := h * 0.24
	pivotX, pivotY := cx, cy+reach*0.85
	R := reach * 1.9
	thetaStart, thetaEnd := -math.Pi/2-1.15, -math.Pi/2+1.15
	thick := h * 0.05
	arcAt := func(t, radius float64) (float64, float64) {
		theta := thetaStart + (thetaEnd-thetaStart)*t
		return pivotX + math.Cos(theta)*radius, pivotY + math.Sin(theta)*radius
	}

	r.drawDissolveStroke(screen, dissolveStroke{
		path:   func(t float64) (float64, float64) { return arcAt(t, R) },
		width:  func(t float64) float64 { return (4 + 8*math.Sin(math.Pi*t)) * widthScale },
		color:  func(t float64) [3]int { return mixColor(moon, pale, t) },
		alpha:  func(t float64) float64 { return 0.5 + 0.5*t },
		length: R * (thetaEnd - thetaStart),
		seed:   seed, salt: 34, blend: additiveGlowBlend,
	}, lead, progress)
	// Faint outer halo arc: the moon's second edge.
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   func(t float64) (float64, float64) { return arcAt(t, R+thick*1.2) },
		width:  func(t float64) float64 { return (2.5 + 3*math.Sin(math.Pi*t)) * widthScale },
		color:  func(t float64) [3]int { return moon },
		alpha:  func(t float64) float64 { return 0.3 * t },
		length: R * (thetaEnd - thetaStart),
		seed:   seed, salt: 35, blend: additiveGlowBlend,
	}, lead, progress)
	if sweepT < 1 {
		tx, ty := arcAt(lead, R)
		r.drawGlowSprite(screen, tx, ty, thick*2, pale, fade, additiveGlowBlend)
	}

	// Reaped slivers: thin stalks spun off the edge, drifting out and down.
	for k := 0; k < slivers; k++ {
		tb := auraHash(seed, k, 36, 0)
		born := tb * meleeSweepFrac
		if progress <= born {
			continue
		}
		u := (progress - born) / (1 - born)
		theta := thetaStart + (thetaEnd-thetaStart)*tb
		sx, sy := arcAt(tb, R+u*h*0.08)
		sy += u * u * h * 0.14
		spin := auraHash(seed, k, 37, 0)*2*math.Pi + u*7
		for g := -1; g <= 1; g++ {
			r.drawGlowSprite(screen, sx+math.Cos(spin)*float64(g)*thick*0.6+math.Cos(theta)*u*thick,
				sy+math.Sin(spin)*float64(g)*thick*0.6,
				math.Max(2, thick*0.28), moon, fade*(1-u)*0.7, additiveGlowBlend)
		}
	}
}
