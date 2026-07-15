package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Arena weapon flourishes - each grounded in the weapon's fighting style and
// its signature rider, built from the same layered vocabulary as the
// legendary effects (glow undercoat + white-hot core ribbon, tip flashes,
// deterministic debris with ghost trails, impact beats), so the arena set
// reads as show-fighting: crowd-pleasing, big, and personal per weapon.

func arenaFxScale(s SlashEffect, screenH float64) (h, width float64) {
	h, width = screenH*meleeSizeScale, 1
	if s.Crit {
		h *= 1.25
		width = 1.3
	}
	return h, width
}

// arenaImpactRings draws n expanding rings (flattened by squashY) from an
// impact point - the shared "the crowd felt that" beat.
func (r *Renderer) arenaImpactRings(screen *ebiten.Image, x, y, u, maxR, squashY, dotSize float64, n int, col [3]int, alpha float64) {
	if u <= 0 || u >= 1 {
		return
	}
	for ring := 0; ring < n; ring++ {
		ru := u - float64(ring)*0.12
		if ru <= 0 {
			continue
		}
		rad := maxR * ru
		a := alpha * (1 - ru) / float64(ring+1)
		const seg = 16
		for i := 0; i < seg; i++ {
			ang := 2 * math.Pi * float64(i) / seg
			r.drawGlowSprite(screen, x+math.Cos(ang)*rad, y+math.Sin(ang)*rad*squashY,
				math.Max(3, dotSize*1.5*(1-ru*0.5)), col, a*1.3, additiveGlowBlend)
		}
	}
}

// Champion's Gladius - the executioner's X: two razor draw-cuts crossing at
// the kill point, a snap-flash where they meet, and molten bronze chips
// sprayed from the crossing. Short, ugly, and the last word of the bout.
func (r *Renderer) drawMeleeFxArenaGladius(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, _ := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	bronze, hot, white := [3]int{216, 170, 96}, [3]int{255, 232, 190}, [3]int{255, 252, 240}
	reach := h * 0.26
	crossX, crossY := cx, cy-reach*0.55

	// Two crossing draw-cuts, the second a heartbeat behind - each is a wide
	// bronze glow ribbon under a thin white-hot core.
	for i, sign := range []float64{-1, 1} {
		lag := float64(i) * 0.09
		p := math.Min(1, math.Max(0, (progress-lag)/(1-lag)))
		st := math.Min(1, p/meleeSweepFrac)
		ld := 1 - (1-st)*(1-st)
		path := func(t float64) (float64, float64) {
			return cx + sign*reach*(1.05-t*2.1), cy + h*0.09 - reach*(t*1.28)
		}
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   path,
			width:  func(t float64) float64 { return (16 + 14*math.Sin(math.Pi*t)) * w },
			color:  func(t float64) [3]int { return mixColor(bronze, hot, t) },
			alpha:  func(t float64) float64 { return 0.7 + 0.3*t },
			length: reach * 2.4, seed: seed, salt: 210 + i, blend: additiveGlowBlend,
		}, ld, p)
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   path,
			width:  func(t float64) float64 { return (5 + 5*math.Sin(math.Pi*t)) * w },
			color:  func(t float64) [3]int { return white },
			alpha:  func(t float64) float64 { return 0.65 + 0.35*t },
			length: reach * 2.4, seed: seed, salt: 214 + i, blend: additiveGlowBlend,
		}, ld, p)
		if st < 1 { // hot leading edge while the cut is being drawn
			tx, ty := path(ld)
			r.drawGlowSprite(screen, tx, ty, h*0.05*w, white, fade, additiveGlowBlend)
		}
	}

	// The cross-flash: the instant both cuts have passed the center, the X
	// snaps bright and a ring cracks off it.
	if sweepT >= 1 {
		u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
		if u < 0.3 {
			flash := 1 - u/0.3
			r.drawSparkStar(screen, crossX, crossY, h*0.1*w*flash, hot, white, fade*flash, 1)
		}
		r.arenaImpactRings(screen, crossX, crossY, u, reach*0.9, 0.5, h*0.012, 2, bronze, fade*0.9)

		// Molten bronze chips: ballistic spray from the crossing, ghost trails.
		const chips = 14
		for k := 0; k < chips; k++ {
			ang := -math.Pi/2 + (auraHash(seed, k, 216, 0)-0.5)*2.6
			spd := 0.5 + auraHash(seed, k, 217, 0)
			for g := 0; g < 3; g++ {
				ug := u - float64(g)*0.05
				if ug < 0 {
					break
				}
				bx := crossX + math.Cos(ang)*spd*h*0.24*ug
				by := crossY + math.Sin(ang)*spd*h*0.24*ug + ug*ug*h*0.3
				f := 1 - 0.3*float64(g)
				c := hot
				if k%3 == 0 {
					c = white
				}
				r.drawGlowRect(screen, bx, by, math.Max(3, h*0.016*(1-0.4*ug)*f), c, fade*(1-ug)*f*f, additiveGlowBlend)
			}
		}
	}
}

// Pit Labrys - the armor ripper: two heavy mirrored crescents with ragged
// torn-metal edges, a grinding spark fountain off the leading edge, and
// sheared armor plates tumbling away. Every swing peels the fight more naked.
func (r *Renderer) drawMeleeFxArenaLabrys(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, _, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	bronze, edge, white := [3]int{196, 140, 60}, [3]int{255, 220, 130}, [3]int{255, 248, 225}
	reach := h * 0.3

	for i, sign := range []float64{-1, 1} {
		lag := float64(i) * 0.12 // one-two chop rhythm
		p := math.Min(1, math.Max(0, (progress-lag)/(1-lag)))
		st := math.Min(1, p/meleeSweepFrac)
		ld := 1 - (1-st)*(1-st)
		pivotX, pivotY := cx+sign*reach*0.18, cy+reach*0.05
		start, end := -math.Pi/2-sign*1.05, -math.Pi/2+sign*0.8
		arc := func(t float64) (float64, float64) {
			a := start + (end-start)*t
			return pivotX + math.Cos(a)*reach, pivotY + math.Sin(a)*reach
		}
		// Ragged bite: the width saws along the arc - torn metal, not a clean cut.
		r.drawDissolveStroke(screen, dissolveStroke{
			path: arc,
			width: func(t float64) float64 {
				saw := 1 + 0.5*math.Sin(t*34+float64(seed%7))
				return (14 + 16*math.Sin(math.Pi*t)) * saw * w
			},
			color:  func(t float64) [3]int { return mixColor(bronze, edge, 0.2+0.8*t) },
			alpha:  func(t float64) float64 { return 0.6 + 0.4*t },
			length: reach * math.Abs(end-start), seed: seed, salt: 220 + i, blend: additiveGlowBlend,
		}, ld, p)
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   arc,
			width:  func(t float64) float64 { return (4.5 + 4.5*math.Sin(math.Pi*t)) * w },
			color:  func(t float64) [3]int { return white },
			alpha:  func(t float64) float64 { return 0.7 * t },
			length: reach * math.Abs(end-start), seed: seed, salt: 224 + i, blend: additiveGlowBlend,
		}, ld, p)

		// Grinding sparks: a fountain streaming off the moving bit-edge,
		// kicked outward and falling under gravity with ghost trails.
		if st > 0.1 {
			ex, ey := arc(ld)
			const sparks = 8
			for k := 0; k < sparks; k++ {
				sb := auraHash(seed, k, 226+i, 0)
				su := math.Mod(p*2.4+sb, 1.0)
				ang := (auraHash(seed, k, 228+i, 0) - 0.5) * 1.8
				sx := ex + math.Sin(ang)*su*h*0.12*w
				sy := ey - math.Cos(ang)*su*h*0.1 + su*su*h*0.16
				r.drawGlowRect(screen, sx, sy, math.Max(3, h*0.013*(1-su*0.5)), mixColor(edge, white, sb), fade*(1-su), additiveGlowBlend)
			}
		}
	}

	// Sheared plates: chunky bronze slabs tumbling off the arcs late in the
	// swing - the 20% armor gone, visibly.
	const plates = 8
	for k := 0; k < plates; k++ {
		born := 0.2 + auraHash(seed, k, 232, 0)*0.25
		if progress <= born {
			continue
		}
		u := (progress - born) / (1 - born)
		side := 1.0
		if k%2 == 0 {
			side = -1
		}
		px := cx + side*(0.3+auraHash(seed, k, 233, 0))*reach*0.7*(1+u*0.6)
		py := cy - reach*(0.5-0.35*auraHash(seed, k, 234, 0)) + u*u*h*0.34
		tum := 0.6 + 0.8*math.Sin(u*9+auraHash(seed, k, 235, 0)*6)
		r.drawGlowRect(screen, px, py, math.Max(3, h*0.026*tum*(1-0.3*u)), bronze, fade*(1-u)*0.95, additiveGlowBlend)
	}
	_ = lead
}

// Morningstar - the bell-ringer: a spiked head whips over on a sagging chain
// (motion ghosts along the links), lands in a star-flash, and the hit RINGS -
// three concentric chime-waves with sparks ricocheting off. The crowd counts.
func (r *Renderer) drawMeleeFxArenaMorningstar(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, _ := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	iron, flash, spark := [3]int{195, 205, 228}, [3]int{248, 252, 255}, [3]int{255, 240, 200}
	reach := h * 0.27
	pivotX, pivotY := cx-reach*0.55, cy+reach*0.5
	angle := -2.6 + math.Min(1, sweepT)*2.15 // lands up-right of the grip, at face height
	headX := pivotX + math.Cos(angle)*reach
	headY := pivotY + math.Sin(angle)*reach

	// Chain: links along a sagging curve, with two ghost copies trailing the
	// swing to sell the whip. Sag eases out as centrifugal force wins.
	sag := reach * 0.2 * (1 - sweepT*0.8)
	for ghost := 2; ghost >= 0; ghost-- {
		ga := angle - float64(ghost)*0.22*(1-sweepT*0.6)
		gx := pivotX + math.Cos(ga)*reach
		gy := pivotY + math.Sin(ga)*reach
		alpha := fade * (0.9 - 0.3*float64(ghost))
		if ghost > 0 {
			alpha *= 0.35
		}
		for i := 1; i <= 10; i++ {
			t := float64(i) / 10
			x := pivotX + (gx-pivotX)*t
			y := pivotY + (gy-pivotY)*t + math.Sin(t*math.Pi)*sag
			r.drawGlowSprite(screen, x, y, h*(0.03+0.013*t)*w, iron, alpha*(0.6+0.4*t), additiveGlowBlend)
		}
		if ghost > 0 {
			r.drawGlowSprite(screen, gx, gy, h*0.07*w, iron, alpha, additiveGlowBlend)
		}
	}

	// The head: an iron core with radiating spikes.
	r.drawGlowSprite(screen, headX, headY, h*0.13*w, iron, fade, additiveGlowBlend)
	r.drawGlowSprite(screen, headX, headY, h*0.07*w, flash, fade, additiveGlowBlend)
	const spikes = 8
	for i := 0; i < spikes; i++ {
		a := 2*math.Pi*float64(i)/spikes + angle*0.5
		sl := h * 0.085 * w
		r.drawGlowSprite(screen, headX+math.Cos(a)*sl, headY+math.Sin(a)*sl, h*0.028*w, flash, fade, additiveGlowBlend)
	}

	// The chime: impact star + three rings + ricochet sparks, all after landing.
	if sweepT >= 1 {
		u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
		if u < 0.35 {
			f := 1 - u/0.35
			r.drawSparkStar(screen, headX, headY, h*0.16*w*f, spark, flash, fade*f, 1.2)
		}
		r.arenaImpactRings(screen, headX, headY, u, reach*1.35, 0.85, h*0.02, 3, iron, fade)
		const ricochets = 10
		for k := 0; k < ricochets; k++ {
			ang := auraHash(seed, k, 242, 0) * 2 * math.Pi
			spd := 0.6 + auraHash(seed, k, 243, 0)
			for g := 0; g < 3; g++ {
				ug := u - float64(g)*0.045
				if ug < 0 {
					break
				}
				sx := headX + math.Cos(ang)*spd*h*0.2*ug
				sy := headY + math.Sin(ang)*spd*h*0.16*ug + ug*ug*h*0.1
				f := 1 - 0.3*float64(g)
				r.drawGlowRect(screen, sx, sy, math.Max(3, h*0.016*f), spark, fade*(1-ug)*f*f, additiveGlowBlend)
			}
		}
	}
}

// Hasta - the legion thrust: a leaf-headed spear rams diagonally up the
// screen behind converging speed lines, air rings slide back down the shaft,
// and the point cracks a shock-cone open at full extension while the phalanx
// shield-line glints below. The line holds because the point holds.
func (r *Renderer) drawMeleeFxArenaHasta(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	steel, hot, amber := [3]int{205, 210, 220}, [3]int{255, 252, 240}, [3]int{245, 200, 100}
	reach := h * 0.46

	// Thrust axis: from low-right (the braced grip) up-left toward the foe.
	dirX, dirY := -0.34, -0.94
	baseX, baseY := cx+h*0.13, cy+reach*0.42
	along := func(t float64) (float64, float64) {
		return baseX + dirX*reach*t, baseY + dirY*reach*t
	}
	tipX, tipY := along(lead)

	// Shaft: a steel ribbon that thickens toward the grip, with a white core.
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   along,
		width:  func(t float64) float64 { return (13 - 6*t) * w },
		color:  func(t float64) [3]int { return mixColor(steel, hot, t*0.6) },
		alpha:  func(t float64) float64 { return 0.7 + 0.3*t },
		length: reach, seed: seed, salt: 230, blend: additiveGlowBlend,
	}, lead, progress)
	// Leaf head: a short wide-to-point blade riding the leading 12% of the shaft.
	headLen := 0.14
	r.drawDissolveStroke(screen, dissolveStroke{
		path: func(t float64) (float64, float64) {
			return along(lead - headLen + headLen*t)
		},
		width:  func(t float64) float64 { return (20 * (1 - t*t) * math.Min(1, lead*3)) * w },
		color:  func(t float64) [3]int { return hot },
		alpha:  func(t float64) float64 { return 0.95 },
		length: reach * headLen, seed: seed, salt: 231, blend: additiveGlowBlend,
	}, 1, progress*0.8)
	r.drawGlowSprite(screen, tipX, tipY, h*0.045*w, hot, fade, additiveGlowBlend)

	// Converging speed lines: three thin streaks racing beside the shaft and
	// pinching toward the tip - pure thrust language.
	for i, off := range []float64{-1, 1, -1.9} {
		p := math.Max(0, math.Min(1, (progress-0.03*float64(i))/(1-0.03*float64(i))))
		sl := math.Min(1, p/meleeSweepFrac)
		sl = 1 - (1-sl)*(1-sl)
		perpX, perpY := -dirY, dirX
		r.drawDissolveStroke(screen, dissolveStroke{
			path: func(t float64) (float64, float64) {
				x, y := along(t * sl)
				pinch := (1 - t) * h * 0.045 * off
				return x + perpX*pinch, y + perpY*pinch
			},
			width:  func(t float64) float64 { return (3.5 + 2.5*t) * w },
			color:  func(t float64) [3]int { return steel },
			alpha:  func(t float64) float64 { return 0.55 * t },
			length: reach, seed: seed, salt: 233 + i, blend: additiveGlowBlend,
		}, sl, p)
	}

	// Air-shear rings sliding back down the shaft while the thrust extends.
	if sweepT < 1 {
		const rings = 3
		for k := 0; k < rings; k++ {
			rt := math.Mod(sweepT*1.6+float64(k)/rings, 1.0)
			rx, ry := along(lead * (1 - rt*0.8))
			rr := h * (0.02 + 0.05*rt)
			const seg = 8
			for i := 0; i < seg; i++ {
				a := 2 * math.Pi * float64(i) / seg
				perpX, perpY := -dirY, dirX
				ox := perpX*math.Cos(a)*rr + dirX*math.Sin(a)*rr*0.25
				oy := perpY*math.Cos(a)*rr + dirY*math.Sin(a)*rr*0.25
				r.drawGlowSprite(screen, rx+ox, ry+oy, math.Max(3, h*0.011), steel, fade*(1-rt)*0.85, additiveGlowBlend)
			}
		}
	}

	// Full extension: the point punches a shock-cone open; the phalanx glint -
	// a brief amber shield-wall line - flashes at the guard.
	if sweepT >= 1 {
		u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
		if u < 0.35 {
			f := 1 - u/0.35
			r.drawSparkStar(screen, tipX, tipY, h*0.09*w*f, hot, hot, fade*f, 1)
			// shock-cone: sparks thrown forward in a tight fan past the tip
			const cone = 9
			for k := 0; k < cone; k++ {
				ca := math.Atan2(dirY, dirX) + (auraHash(seed, k, 236, 0)-0.5)*0.9
				cd := (0.3 + auraHash(seed, k, 237, 0)) * h * 0.14 * (u / 0.35)
				r.drawGlowRect(screen, tipX+math.Cos(ca)*cd, tipY+math.Sin(ca)*cd,
					math.Max(3, h*0.014), hot, fade*f, additiveGlowBlend)
			}
		}
		if u < 0.5 {
			f := 1 - u/0.5
			gy := baseY + h*0.02
			for i := -3; i <= 3; i++ {
				gx := baseX + float64(i)*h*0.045 + dirX*h*0.06
				r.drawGlowRect(screen, gx, gy, math.Max(3, h*0.02*f), amber, fade*f*0.75, additiveGlowBlend)
			}
		}
	}
}

// Retiarius Trident - net, then fork, then verdict: a diamond-mesh net blooms
// outward and tightens, the three points ram through it in foam-white
// streaks, and sea-spray rains off the strike while bubbles drift up.
func (r *Renderer) drawMeleeFxArenaTrident(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, _ := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	sea, foam, deep := [3]int{95, 195, 215}, [3]int{230, 252, 255}, [3]int{40, 110 + seed%3, 150}
	reach := h * 0.42

	// The net: a rhombus mesh cast up-screen ahead of the points - it blooms
	// (scales out) then visibly tightens (nodes pull toward its heart).
	netU := math.Min(1, progress/(meleeSweepFrac*1.3))
	netX, netY := cx, cy-reach*0.62
	bloom := math.Sin(netU*math.Pi*0.62) // out then slightly back = the cinch
	netR := reach * (0.16 + 0.42*bloom)
	if netU > 0.02 && netU < 1 {
		aN := fade * 0.55 * math.Sin(netU*math.Pi)
		const rows = 3
		for ring := 1; ring <= rows; ring++ {
			rr := netR * float64(ring) / rows
			const seg = 10
			for i := 0; i < seg; i++ {
				ang := 2*math.Pi*float64(i)/seg + float64(ring)*0.3 + netU*0.7
				nx := netX + math.Cos(ang)*rr
				ny := netY + math.Sin(ang)*rr*0.7
				r.drawGlowSprite(screen, nx, ny, math.Max(3, h*0.014), sea, aN*1.4, additiveGlowBlend)
				// mesh threads: a dim link to the next node in the same ring
				ang2 := 2*math.Pi*float64(i+1)/seg + float64(ring)*0.3 + netU*0.7
				mx := netX + math.Cos((ang+ang2)/2)*rr
				my := netY + math.Sin((ang+ang2)/2)*rr*0.7
				r.drawGlowSprite(screen, mx, my, math.Max(3, h*0.01), deep, aN*1.1, additiveGlowBlend)
			}
		}
	}

	// The fork: three prongs, the middle one a touch ahead, each a sea-glow
	// ribbon under a foam core with a bright tip.
	for i, offset := range []float64{-0.085, 0, 0.085} {
		off := offset * h
		lag := 0.05 * math.Abs(float64(i)-1)
		p := math.Min(1, math.Max(0, (progress-lag)/(1-lag)))
		st := math.Min(1, p/meleeSweepFrac)
		ld := 1 - (1-st)*(1-st)
		prong := func(t float64) (float64, float64) {
			return cx + off*(1-t*0.2), cy + reach*0.42 - reach*t
		}
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   prong,
			width:  func(t float64) float64 { return (10 + 5*(1-t)) * w },
			color:  func(t float64) [3]int { return mixColor(sea, foam, t) },
			alpha:  func(t float64) float64 { return 0.65 + 0.35*t },
			length: reach, seed: seed, salt: 240 + i, blend: additiveGlowBlend,
		}, ld, p)
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   prong,
			width:  func(t float64) float64 { return (3.5 + 2.5*(1-t)) * w },
			color:  func(t float64) [3]int { return foam },
			alpha:  func(t float64) float64 { return 0.75 * t },
			length: reach, seed: seed, salt: 244 + i, blend: additiveGlowBlend,
		}, ld, p)
		if st < 1 {
			px, py := prong(ld)
			r.drawGlowSprite(screen, px, py, h*0.038*w, foam, fade, additiveGlowBlend)
		}
	}

	// Sea-spray: droplets flung up-and-out from the strike, falling as arcs;
	// slow bubbles wobble upward after the hit.
	if sweepT > 0.6 {
		u := (progress - meleeSweepFrac*0.6) / (1 - meleeSweepFrac*0.6)
		const drops = 12
		for k := 0; k < drops; k++ {
			ang := -math.Pi/2 + (auraHash(seed, k, 246, 0)-0.5)*1.7
			spd := 0.5 + auraHash(seed, k, 247, 0)
			for g := 0; g < 3; g++ {
				ug := u - float64(g)*0.05
				if ug < 0 {
					break
				}
				dx := cx + math.Cos(ang)*spd*h*0.2*ug
				dy := cy - reach*0.55 + math.Sin(ang)*spd*h*0.2*ug + ug*ug*h*0.3
				f := 1 - 0.3*float64(g)
				r.drawGlowSprite(screen, dx, dy, math.Max(3, h*0.013*f), mixColor(sea, foam, auraHash(seed, k, 248, 0)), fade*(1-ug)*f*f, additiveGlowBlend)
			}
		}
		const bubbles = 6
		for k := 0; k < bubbles; k++ {
			bu := math.Mod(u*0.8+auraHash(seed, k, 249, 0), 1.0)
			bx := cx + (auraHash(seed, k, 250, 0)-0.5)*reach*0.7 + math.Sin(bu*6)*h*0.012
			by := cy - reach*0.3 - bu*h*0.2
			r.drawGlowSprite(screen, bx, by, math.Max(2, h*0.007+bu*h*0.004), foam, fade*(1-bu)*0.5, additiveGlowBlend)
		}
	}
}

// Parrying Dagger - it listens, then talks back: a dim enemy blade sweeps in,
// the silver off-hand catches it in a CLANG of fanned sparks, and the answer
// is a needle riposte with a hot ember core. Thorns, staged.
func (r *Renderer) drawMeleeFxArenaParry(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, _, _ := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	silver, white, ember := [3]int{215, 224, 240}, [3]int{255, 255, 255}, [3]int{255, 150, 95}
	reach := h * 0.24
	clangX, clangY := cx-h*0.02, cy-reach*0.35

	// Phase clocks: enemy blade sweeps in [0..0.4], parry catches it [0.15..0.5],
	// the riposte answers [0.45..1].
	inP := math.Min(1, progress/0.4)
	// The other blade: a dim broad ghost cutting toward the party until caught.
	if progress < 0.5 {
		gp := 1 - (1-inP)*(1-inP)
		r.drawDissolveStroke(screen, dissolveStroke{
			path: func(t float64) (float64, float64) {
				return cx - reach*1.15 + (clangX-(cx-reach*1.15))*t, cy - reach*0.95 + (clangY-(cy-reach*0.95))*t
			},
			width:  func(t float64) float64 { return (11 - 4*t) * w },
			color:  func(t float64) [3]int { return [3]int{120, 120, 135} },
			alpha:  func(t float64) float64 { return 0.5 },
			length: reach * 1.4, seed: seed, salt: 250, blend: ebiten.BlendSourceOver,
		}, gp, math.Min(1, progress*1.6))
	}

	// The catch: a compact silver arc snapping across the ghost blade's path.
	if progress > 0.12 {
		pp := math.Min(1, (progress-0.12)/0.3)
		ld := 1 - (1-pp)*(1-pp)
		r.drawDissolveStroke(screen, dissolveStroke{
			path: func(t float64) (float64, float64) {
				a := math.Pi*1.25 - math.Pi*0.75*t
				return clangX + math.Cos(a)*reach*0.7, clangY + math.Sin(a)*reach*0.55
			},
			width:  func(t float64) float64 { return (11 + 9*math.Sin(math.Pi*t)) * w },
			color:  func(t float64) [3]int { return mixColor(silver, white, t) },
			alpha:  func(t float64) float64 { return 0.75 + 0.25*t },
			length: reach * 1.4, seed: seed, salt: 251, blend: additiveGlowBlend,
		}, ld, math.Max(0, (progress-0.12)/(1-0.12)))
	}

	// CLANG: spark fan + a tight ring right at the contact instant.
	if progress > 0.3 && progress < 0.75 {
		u := (progress - 0.3) / 0.45
		f := 1 - u
		r.drawSparkStar(screen, clangX, clangY, h*0.11*w*f, white, white, fade*f, 1)
		r.arenaImpactRings(screen, clangX, clangY, u, reach*0.7, 0.9, h*0.014, 2, silver, fade)
		const sparks = 9
		for k := 0; k < sparks; k++ {
			ang := -math.Pi*0.75 + (auraHash(seed, k, 252, 0)-0.5)*2.2
			sd := (0.3 + auraHash(seed, k, 253, 0)) * h * 0.16 * u
			r.drawGlowRect(screen, clangX+math.Cos(ang)*sd, clangY+math.Sin(ang)*sd+u*u*h*0.06,
				math.Max(3, h*0.012), white, fade*f, additiveGlowBlend)
		}
	}

	// The answer: a needle-thin lunge with an ember core, faster than the eye.
	if progress > 0.42 {
		rp := math.Min(1, (progress-0.42)/0.3)
		ld := 1 - (1-rp)*(1-rp)
		nPath := func(t float64) (float64, float64) {
			return cx - h*0.04 + h*0.1*t, cy + h*0.16 - reach*1.5*t
		}
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   nPath,
			width:  func(t float64) float64 { return (9 - 3*t) * w },
			color:  func(t float64) [3]int { return mixColor(ember, white, t*0.7) },
			alpha:  func(t float64) float64 { return 0.8 + 0.2*t },
			length: reach * 1.5, seed: seed, salt: 254, blend: additiveGlowBlend,
		}, ld, math.Max(0, (progress-0.42)/(1-0.42)))
		if rp < 1 {
			tx, ty := nPath(ld)
			r.drawGlowSprite(screen, tx, ty, h*0.04*w, white, fade, additiveGlowBlend)
			r.drawGlowSprite(screen, tx, ty, h*0.075*w, ember, fade*0.6, additiveGlowBlend)
		}
	}
}

// Lion-Crest Warhammer - the roar: a ponderous overhead crescent lands in a
// gold star-flash, a MANE of tapered golden rays snaps open around the
// impact, twin ground shockwaves race out, and breastplate shards spin away.
func (r *Renderer) drawMeleeFxArenaLion(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	gold, white, iron := [3]int{235, 185, 70}, [3]int{255, 248, 205}, [3]int{150, 150, 160}
	reach := h * 0.3
	impX, impY := cx, cy-h*0.1

	// The overhead crescent: heavy gold ribbon + white core, thickening to the head.
	arc := func(t float64) (float64, float64) {
		a := -math.Pi/2 - 1.0 + 2.0*t
		return cx + math.Cos(a)*reach, cy + h*0.14 + math.Sin(a)*reach
	}
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   arc,
		width:  func(t float64) float64 { return (15 + 17*t) * w },
		color:  func(t float64) [3]int { return mixColor(gold, white, t*0.7) },
		alpha:  func(t float64) float64 { return 0.65 + 0.35*t },
		length: reach * 2, seed: seed, salt: 260, blend: additiveGlowBlend,
	}, lead, progress)
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   arc,
		width:  func(t float64) float64 { return (5 + 6*t) * w },
		color:  func(t float64) [3]int { return white },
		alpha:  func(t float64) float64 { return 0.75 * t },
		length: reach * 2, seed: seed, salt: 261, blend: additiveGlowBlend,
	}, lead, progress)
	if sweepT < 1 {
		hx, hy := arc(lead)
		r.drawGlowSprite(screen, hx, hy, h*0.07*w, white, fade, additiveGlowBlend)
		return
	}

	// The ROAR - everything after the landing.
	u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
	if u < 0.3 {
		f := 1 - u/0.3
		r.drawSparkStar(screen, impX, impY, h*0.14*w*f, gold, white, fade*f, 1.3)
	}
	// The mane: 14 tapered golden rays snapping open, longest upward - each a
	// solid dissolve ribbon so the burst reads as a crest, not a dot cloud.
	const rays = 14
	mane := math.Min(1, u/0.45)
	maneA := fade * (1 - u*0.6)
	for k := 0; k < rays; k++ {
		a := -math.Pi/2 + (float64(k)/(rays-1)-0.5)*2.5
		l := reach * (0.6 + 0.5*math.Cos((float64(k)/(rays-1)-0.5)*math.Pi)) * mane
		jit := 0.85 + 0.3*auraHash(seed, k, 262, 0)
		kk := k
		r.drawDissolveStroke(screen, dissolveStroke{
			path: func(t float64) (float64, float64) {
				return impX + math.Cos(a)*l*t*jit, impY + math.Sin(a)*l*t*jit
			},
			width:  func(t float64) float64 { return (13 * (1 - t*0.75)) * w },
			color:  func(t float64) [3]int { return mixColor(white, gold, t) },
			alpha:  func(t float64) float64 { return math.Min(1, maneA*1.5) * (1 - t*0.35) },
			length: l, seed: seed, salt: 300 + kk, blend: additiveGlowBlend,
		}, mane, u*0.8)
	}
	r.arenaImpactRings(screen, impX, impY+h*0.06, u, reach*1.2, 0.4, h*0.014, 2, gold, fade)

	// Breastplate shards (armor pierce): iron chips with gold glints tumbling.
	const shards = 10
	for k := 0; k < shards; k++ {
		ang := -math.Pi/2 + (auraHash(seed, k, 264, 0)-0.5)*2.2
		spd := 0.5 + auraHash(seed, k, 265, 0)
		for g := 0; g < 3; g++ {
			ug := u - float64(g)*0.05
			if ug < 0 {
				break
			}
			sx := impX + math.Cos(ang)*spd*h*0.2*ug
			sy := impY + math.Sin(ang)*spd*h*0.24*ug + ug*ug*h*0.3
			c := iron
			if k%3 == 0 {
				c = gold
			}
			f := 1 - 0.3*float64(g)
			r.drawGlowRect(screen, sx, sy, math.Max(3, h*0.018*(1-0.4*ug)*f), c, fade*(1-ug)*f*f, additiveGlowBlend)
		}
	}
}

// Bronze Cesti - the one-two: two piston jabs, each landing in a knuckle
// flash with a compressed-air ring and a radial burst of speed ticks; bronze
// glint dust hangs between the blows. Two fists, four plates, one rhythm.
func (r *Renderer) drawMeleeFxArenaCesti(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, _, _ := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	bronze, hot, white := [3]int{205, 140, 70}, [3]int{255, 225, 165}, [3]int{255, 250, 235}
	reach := h * 0.24

	for i, side := range []float64{-1, 1} {
		lag := float64(i) * 0.16 // the "two" lands while the "one" still glows
		p := math.Min(1, math.Max(0, (progress-lag)/(1-lag)))
		if p <= 0 {
			continue
		}
		st := math.Min(1, p/meleeSweepFrac)
		ld := 1 - (1-st)*(1-st)
		jab := func(t float64) (float64, float64) {
			return cx + side*h*0.11*(1-t*0.75), cy + h*0.15 - reach*t
		}
		hitX, hitY := jab(1)

		// The jab: a thick piston streak + white core - reads as a straight punch.
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   jab,
			width:  func(t float64) float64 { return (14 + 8*(1-t)) * w },
			color:  func(t float64) [3]int { return mixColor(bronze, hot, t) },
			alpha:  func(t float64) float64 { return 0.7 + 0.3*t },
			length: reach, seed: seed, salt: 270 + i, blend: additiveGlowBlend,
		}, ld, p)
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   jab,
			width:  func(t float64) float64 { return (5 + 3.5*(1-t)) * w },
			color:  func(t float64) [3]int { return white },
			alpha:  func(t float64) float64 { return 0.7 + 0.3*t },
			length: reach, seed: seed, salt: 274 + i, blend: additiveGlowBlend,
		}, ld, p)

		// Knuckle flash + compressed-air ring + radial speed ticks on landing.
		if st >= 1 {
			u := (p - meleeSweepFrac) / (1 - meleeSweepFrac)
			if u < 0.3 {
				f := 1 - u/0.3
				r.drawSparkStar(screen, hitX, hitY, h*0.12*w*f, hot, white, fade*f, 0.8)
			}
			r.arenaImpactRings(screen, hitX, hitY, u, reach*0.8, 0.9, h*0.015, 2, hot, fade)
			const ticks = 8
			for k := 0; k < ticks; k++ {
				ang := auraHash(seed, k, 276+i, 0) * 2 * math.Pi
				td := (0.35 + auraHash(seed, k, 278+i, 0)*0.5) * h * 0.11 * math.Min(1, u/0.5)
				r.drawGlowRect(screen, hitX+math.Cos(ang)*td, hitY+math.Sin(ang)*td*0.8,
					math.Max(3, h*0.017), hot, fade*(1-u), additiveGlowBlend)
			}
		} else {
			// wrist glint while the arm extends
			gx, gy := jab(ld)
			r.drawGlowSprite(screen, gx, gy, h*0.042*w, white, fade, additiveGlowBlend)
		}
	}

	// Bronze dust: a faint sparkling haze hanging between the two blows.
	const dust = 8
	for k := 0; k < dust; k++ {
		born := auraHash(seed, k, 280, 0) * 0.5
		if progress <= born {
			continue
		}
		u := (progress - born) / (1 - born)
		dx := cx + (auraHash(seed, k, 281, 0)-0.5)*h*0.2
		dy := cy - reach*0.45 - u*h*0.05 + math.Sin(u*7+float64(k))*h*0.008
		r.drawGlowSprite(screen, dx, dy, math.Max(2, h*0.007), bronze, fade*(1-u)*0.5, additiveGlowBlend)
	}
}
