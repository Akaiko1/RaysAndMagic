package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Clock tower weapon flourishes - the horologist's arsenal. The set's shared
// signature is that TIME MOVES IN TICKS: sweeps advance through discrete
// ratchet notches (clockTickLead) instead of gliding, and every finish beats
// like a mechanism - tolling rings, punched gears, released mainsprings.
// Built from the same layered vocabulary as the arena/legendary sets (glow
// undercoat + white-hot core ribbon, deterministic debris with ghost trails,
// impact beats).

// clockTickLead quantizes an eased sweep into n ratchet notches with a fast
// snap inside each notch - the escapement feel shared by the whole set.
func clockTickLead(lead float64, n int) float64 {
	if lead >= 1 {
		return 1
	}
	step := math.Floor(lead * float64(n))
	frac := lead*float64(n) - step
	snap := math.Min(1, frac*3.2)
	return (step + snap) / float64(n)
}

// The tower's shared brass-and-steel palette.
var (
	clockBrass  = [3]int{232, 180, 88}
	clockCopper = [3]int{212, 122, 52}
	clockSteel  = [3]int{206, 218, 236}
	clockWhite  = [3]int{255, 250, 236}
)

// clockCogRing draws a spinning cog: bright hub + toothed rim (alternating
// radius) with a dot between every tooth so the rim reads closed, rotating
// with the frame counter. The set's little mascot.
func (r *Renderer) clockCogRing(screen *ebiten.Image, x, y, rad, dotSize float64, col, hot [3]int, alpha, spin float64) {
	r.drawGlowSprite(screen, x, y, dotSize*1.8, hot, alpha, additiveGlowBlend)
	const teeth = 16
	for i := 0; i < teeth; i++ {
		ang := spin + 2*math.Pi*float64(i)/teeth
		rr, sz, c := rad, dotSize*0.8, col
		if i%2 == 0 {
			rr, sz, c = rad*1.35, dotSize, mixColor(col, hot, 0.5) // tooth
		}
		r.drawGlowRect(screen, x+math.Cos(ang)*rr, y+math.Sin(ang)*rr,
			math.Max(2.5, sz), c, alpha, additiveGlowBlend)
	}
}

// Cogfang Blade - the gear-bite: a deep crescent swept in six ratchet
// notches, bright steel TEETH studding its outer edge, a live cog spinning on
// the leading edge. The finish is the bite: a punched-plate chip spray and a
// toothed gear-ring stamped where it chewed through - the 25% armor pierce
// made visible.
func (r *Renderer) drawMeleeFxClockCogfang(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	reach := h * 0.24
	pivotX, pivotY := cx, cy+reach*0.55
	R := reach * 1.5
	th0, th1 := -math.Pi/2-1.05, -math.Pi/2+1.05
	tick := clockTickLead(lead, 6)
	cur := th0 + (th1-th0)*tick
	arcAt := func(t float64) (float64, float64) {
		theta := th0 + (cur-th0)*t
		return pivotX + math.Cos(theta)*R, pivotY + math.Sin(theta)*R
	}

	// Brass glow undercoat under a hot core - wide, saturated, high alpha.
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   arcAt,
		width:  func(t float64) float64 { return (24 + 16*math.Sin(math.Pi*t)) * w },
		color:  func(t float64) [3]int { return mixColor(clockCopper, clockBrass, t) },
		alpha:  func(t float64) float64 { return 0.75 + 0.25*t },
		length: R * 2.1, seed: seed, salt: 310, blend: additiveGlowBlend,
	}, 1, progress)
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   arcAt,
		width:  func(t float64) float64 { return (8 + 6*math.Sin(math.Pi*t)) * w },
		color:  func(t float64) [3]int { return mixColor(clockSteel, clockWhite, t) },
		alpha:  func(t float64) float64 { return 0.75 + 0.25*t },
		length: R * 2.1, seed: seed, salt: 312, blend: additiveGlowBlend,
	}, 1, progress)

	// Gear TEETH: bright steel studs riding the crescent's outer edge, revealed
	// with the sweep and dissolving with the trail.
	const teeth = 11
	for i := 0; i < teeth; i++ {
		tt := (float64(i) + 0.5) / teeth
		theta := th0 + (cur-th0)*tt
		outR := R + (14+8*math.Sin(math.Pi*tt))*w
		a := fade * (0.55 + 0.45*tt)
		r.drawGlowRect(screen, pivotX+math.Cos(theta)*outR, pivotY+math.Sin(theta)*outR,
			math.Max(3, h*0.014*w), mixColor(clockSteel, clockWhite, tt), a, additiveGlowBlend)
	}

	// Live cog spinning on the leading edge while the blade sweeps.
	if sweepT < 1 {
		tx, ty := arcAt(1)
		r.clockCogRing(screen, tx, ty, h*0.06*w, h*0.016*w, clockBrass, clockWhite,
			fade, float64(r.game.frameCount)*0.3)
	}

	// The bite: toothed gear-ring stamp + punched plate chips at the arc's end.
	if sweepT >= 1 {
		u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
		bx, by := arcAt(1)
		if u < 0.35 {
			r.drawSparkStar(screen, bx, by, h*0.08*w*(1-u/0.35), clockBrass, clockWhite, fade*(1-u/0.35), 1)
		}
		if u > 0 && u < 1 {
			rad := reach * 0.9 * u
			const spokes = 14
			for i := 0; i < spokes; i++ {
				ang := 2*math.Pi*float64(i)/spokes + u*1.2 // the stamp itself turns
				rr := rad
				if i%2 == 0 {
					rr *= 1.25
				}
				r.drawGlowRect(screen, bx+math.Cos(ang)*rr, by+math.Sin(ang)*rr*0.55,
					math.Max(3, h*0.015*(1-u*0.4)), clockBrass, fade*(1-u)*1.3, additiveGlowBlend)
			}
		}
		// Punched plate chips off the bite (shared debris beat).
		r.arenaDebrisSpray(screen, bx, by, u, fade, h, 2.4, 0.5, 0.22, 0.28, 0.016, 12, 3, seed, 316, clockSteel, clockWhite)
	}
}

// Chime Maul - the noon toll: a short, brutal overhead arc wound through four
// ratchet notches, and on impact the bell RINGS - three flattened resonance
// waves, twelve hour-marks flashing around the strike in dial order, and slow
// golden motes hanging in the air after the note. The stun rider, sung loud.
func (r *Renderer) drawMeleeFxClockChime(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	reach := h * 0.24
	// Steep one-sided chop (~125 degrees, all right of the pivot): up-right ->
	// impact low-right. Never closes into a ring silhouette.
	pivotX, pivotY := cx-reach*0.5, cy-reach*0.45
	R := reach * 1.35
	th0, th1 := -0.95, 1.15
	tick := clockTickLead(lead, 4)
	cur := th0 + (th1-th0)*tick
	arcAt := func(t float64) (float64, float64) {
		theta := th0 + (cur-th0)*t
		return pivotX + math.Cos(theta)*R, pivotY + math.Sin(theta)*R
	}
	impactX, impactY := pivotX+math.Cos(th1)*R, pivotY+math.Sin(th1)*R

	// Heavy golden head: wide undercoat + hot core, thickening toward the head.
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   arcAt,
		width:  func(t float64) float64 { return (26 + 22*t) * w },
		color:  func(t float64) [3]int { return mixColor(clockCopper, clockBrass, t) },
		alpha:  func(t float64) float64 { return 0.7 + 0.3*t },
		length: R * 1.6, seed: seed, salt: 330, blend: additiveGlowBlend,
	}, 1, progress)
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   arcAt,
		width:  func(t float64) float64 { return (9 + 9*t) * w },
		color:  func(t float64) [3]int { return mixColor(clockBrass, clockWhite, t) },
		alpha:  func(t float64) float64 { return 0.75 + 0.25*t },
		length: R * 1.6, seed: seed, salt: 332, blend: additiveGlowBlend,
	}, 1, progress)
	if sweepT < 1 {
		tx, ty := arcAt(1)
		r.drawGlowSprite(screen, tx, ty, h*0.07*w, clockWhite, fade*0.95, additiveGlowBlend)
	}

	// The toll.
	if sweepT >= 1 {
		u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
		if u < 0.25 {
			flash := 1 - u/0.25
			r.drawSparkStar(screen, impactX, impactY, h*0.13*w*flash, clockBrass, clockWhite, fade*flash, 1)
		}
		// Three resonance waves - the bell note spreading out of the strike.
		r.arenaImpactRings(screen, impactX, impactY, u, reach*2.0, 0.45, h*0.02, 3, clockBrass, fade*1.4)
		// Twelve hour-marks flash on around the strike, in dial order - the
		// dial IS the finish, it must outshine the fading swing.
		lit := int(u * 30)
		for i := 0; i < 12 && i < lit; i++ {
			ang := -math.Pi/2 + 2*math.Pi*float64(i)/12
			rad := reach * 0.8
			r.drawGlowRect(screen, impactX+math.Cos(ang)*rad, impactY+math.Sin(ang)*rad*0.6,
				math.Max(4, h*0.02), mixColor(clockBrass, clockWhite, 0.5), fade*(1-u*0.45)*1.3, additiveGlowBlend)
		}
		// Hanging golden motes: the note's shimmer, drifting up, slow to leave.
		const motes = 16
		for k := 0; k < motes; k++ {
			mx := impactX + (auraHash(seed, k, 336, 0)-0.5)*reach*1.8
			rise := u * (0.4 + auraHash(seed, k, 337, 0)) * h * 0.14
			my := impactY - rise + (auraHash(seed, k, 338, 0)-0.5)*reach*0.4
			tw := 0.6 + 0.4*math.Sin(float64(r.game.frameCount)*0.31+float64(k)*1.7)
			r.drawGlowSprite(screen, mx, my, math.Max(2.5, h*0.013), clockBrass, fade*(1-u*0.5)*0.85*tw, additiveGlowBlend)
		}
	}
}

// Minute Hand - sixty stabs an hour: a long needle thrust extending in five
// hard ticks with a bright notch-dash snapped at every height it passed, a
// ghost needle one beat behind (the 0.65x cooldown, visible), and at full
// extension the dial flash - twelve marks and a minute hand snapping from
// noon to the strike before the reading fades.
func (r *Renderer) drawMeleeFxClockMinute(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	reach := h * 0.36
	baseX, baseY := cx, cy+reach*0.5
	tick := clockTickLead(lead, 5)

	needle := func(off, ld, alphaMul float64) {
		bx := baseX + off
		tipLen := reach * ld
		path := func(t float64) (float64, float64) { return bx + off*0.2*t, baseY - tipLen*t }
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   path,
			width:  func(t float64) float64 { return (16 - 9*t) * w },
			color:  func(t float64) [3]int { return mixColor(clockSteel, clockWhite, t) },
			alpha:  func(t float64) float64 { return (0.7 + 0.3*t) * alphaMul },
			length: reach, seed: seed, salt: 350 + int(off), blend: additiveGlowBlend,
		}, 1, progress)
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   path,
			width:  func(t float64) float64 { return (6 - 2.5*t) * w },
			color:  func(t float64) [3]int { return clockWhite },
			alpha:  func(t float64) float64 { return (0.8 + 0.2*t) * alphaMul },
			length: reach, seed: seed, salt: 352 + int(off), blend: additiveGlowBlend,
		}, 1, progress)
		if sweepT < 1 {
			r.drawGlowSprite(screen, bx, baseY-tipLen, h*0.04*w*alphaMul, clockWhite, fade*alphaMul, additiveGlowBlend)
		}
	}
	needle(0, tick, 1)
	// Ghost needle one tick behind - the second stab already queued.
	ghostLead := clockTickLead(math.Max(0, lead-0.2), 5)
	needle(h*0.045, ghostLead, 0.5)

	// Notch dashes: a bright horizontal tick crossing the blade at each height
	// the needle has snapped past - the minute marks of the thrust.
	notches := int(tick * 5)
	for i := 1; i <= notches && i <= 5; i++ {
		ny := baseY - reach*float64(i)/5
		for d := -1; d <= 1; d++ {
			r.drawGlowRect(screen, baseX+float64(d)*h*0.016, ny, math.Max(3, h*0.011),
				mixColor(clockSteel, clockWhite, 0.5), fade*0.75, additiveGlowBlend)
		}
	}

	// The dial flash at full extension: 12 marks + the minute hand snapping over.
	if sweepT >= 1 {
		u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
		tipX, tipY := baseX, baseY-reach
		dialR := reach * 0.42
		for i := 0; i < 12; i++ {
			ang := -math.Pi/2 + 2*math.Pi*float64(i)/12
			r.drawGlowRect(screen, tipX+math.Cos(ang)*dialR, tipY+math.Sin(ang)*dialR,
				math.Max(3, h*0.012), clockSteel, fade*(1-u), additiveGlowBlend)
		}
		// Minute hand: snaps from noon around the dial as the flash fades.
		handAng := -math.Pi/2 + math.Min(1, u*1.6)*2*math.Pi*0.65
		for k := 0; k <= 8; k++ {
			t := float64(k) / 8
			r.drawGlowSprite(screen, tipX+math.Cos(handAng)*dialR*t, tipY+math.Sin(handAng)*dialR*t,
				math.Max(2.5, h*0.012*(1-t*0.35)), clockWhite, fade*(1-u)*(0.55+0.45*t), additiveGlowBlend)
		}
		if u < 0.2 {
			r.drawSparkStar(screen, tipX, tipY, h*0.07*w*(1-u/0.2), clockSteel, clockWhite, fade*(1-u/0.2), 1.8)
		}
	}
}

// Mainspring Pike - unwound fury: the thrust is a mainspring letting go. The
// stroke coils around the thrust axis and STRAIGHTENS as it extends (a helix
// with its radius wound down by the release), tension glints streak
// tangentially off the coil, and at full reach the tip overshoots and springs
// back - with a pressure ring cracking off the point.
func (r *Renderer) drawMeleeFxClockMainspring(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, _ := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	reach := h * 0.4
	baseX, baseY := cx, cy+reach*0.4

	// Spring release: violent ease-out - almost all of the reach in the first
	// beats of the sweep.
	ld := 1 - math.Pow(1-sweepT, 4)
	// Recoil after full extension: the tip bounces once and settles.
	bounce := 0.0
	if sweepT >= 1 {
		u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
		bounce = 0.05 * math.Sin(u*14) * math.Exp(-u*5)
	}
	ext := reach * (ld + bounce)
	unwind := 1 - 0.8*ld // coil radius multiplier: wound tight -> straight

	helix := func(t float64) (float64, float64) {
		coilR := h * 0.08 * (1 - t*0.7) * unwind
		phase := t*3.4*2*math.Pi + float64(seed%7)
		return baseX + math.Cos(phase)*coilR, baseY - ext*t + math.Sin(phase)*coilR*0.45
	}
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   helix,
		width:  func(t float64) float64 { return (19 - 9*t) * w },
		color:  func(t float64) [3]int { return mixColor(clockCopper, clockSteel, t) },
		alpha:  func(t float64) float64 { return 0.72 + 0.28*t },
		length: reach * 1.5, seed: seed, salt: 370, blend: additiveGlowBlend,
	}, 1, progress)
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   helix,
		width:  func(t float64) float64 { return (7 - 3*t) * w },
		color:  func(t float64) [3]int { return clockWhite },
		alpha:  func(t float64) float64 { return 0.75 + 0.25*t },
		length: reach * 1.5, seed: seed, salt: 372, blend: additiveGlowBlend,
	}, 1, progress)

	// Tension glints: short STREAKS thrown tangentially off the unwinding coil.
	if sweepT < 1 {
		const glints = 8
		for k := 0; k < glints; k++ {
			t := auraHash(seed, k, 374, 0) * ld
			gx, gy := helix(t)
			ang := auraHash(seed, k, 375, 0) * 2 * math.Pi
			for st := 0; st < 3; st++ {
				d := h * (0.015 + 0.02*float64(st)) * (0.4 + sweepT)
				r.drawGlowRect(screen, gx+math.Cos(ang)*d, gy+math.Sin(ang)*d,
					math.Max(2.5, h*0.012*(1-0.25*float64(st))), clockSteel, fade*(0.85-0.22*float64(st)), additiveGlowBlend)
			}
		}
		tx, ty := helix(ld)
		r.drawGlowSprite(screen, tx, ty, h*0.055*w, clockWhite, fade, additiveGlowBlend)
	}

	// Full extension: pressure ring + the point flash riding the recoil.
	if sweepT >= 1 {
		u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
		tipX, tipY := baseX, baseY-ext
		if u < 0.3 {
			r.drawSparkStar(screen, tipX, tipY, h*0.09*w*(1-u/0.3), clockSteel, clockWhite, fade*(1-u/0.3), 2.4)
		}
		r.arenaImpactRings(screen, tipX, tipY, u, reach*0.6, 0.6, h*0.013, 2, clockCopper, fade)
	}
}

// Escapement Mace - tick, tock: two ratcheting chops that CONVERGE on the same
// strike point, the second half a beat behind, each advancing through four
// hard notches with a click-spark at every one. The finish shears BRASS GEAR
// TEETH off the target - square flakes tumbling with ghost trails - while
// pawl-click arc segments snap in around the strike. Armor stripped, tooth by
// tooth.
func (r *Renderer) drawMeleeFxClockEscapement(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, _ := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	reach := h * 0.26
	meetX, meetY := cx, cy-reach*0.25

	for i, sign := range []float64{-1, 1} {
		lag := float64(i) * 0.3
		p := math.Min(1, math.Max(0, (progress-lag*meleeSweepFrac)/(1-lag*meleeSweepFrac)))
		st := math.Min(1, p/meleeSweepFrac)
		ld := clockTickLead(1-(1-st)*(1-st), 4)
		// A sagging cut from high-out to the meet: start -> meet with an outward
		// bulge, so both chops visibly END on the same strike point.
		startX, startY := meetX+sign*reach*1.5, meetY-reach*0.85
		pathTo := func(t float64) (float64, float64) {
			tt := t * ld
			bulge := math.Sin(math.Pi*tt) * reach * 0.34
			return startX + (meetX-startX)*tt + sign*bulge*0.35,
				startY + (meetY-startY)*tt + bulge
		}
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   pathTo,
			width:  func(t float64) float64 { return (20 + 12*math.Sin(math.Pi*t)) * w },
			color:  func(t float64) [3]int { return mixColor(clockCopper, clockBrass, t) },
			alpha:  func(t float64) float64 { return 0.72 + 0.28*t },
			length: reach * 2.0, seed: seed, salt: 390 + i, blend: additiveGlowBlend,
		}, 1, p)
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   pathTo,
			width:  func(t float64) float64 { return (7 + 5*math.Sin(math.Pi*t)) * w },
			color:  func(t float64) [3]int { return mixColor(clockBrass, clockWhite, t) },
			alpha:  func(t float64) float64 { return 0.75 + 0.25*t },
			length: reach * 2.0, seed: seed, salt: 394 + i, blend: additiveGlowBlend,
		}, 1, p)
		// Click-spark at each ratchet notch the head has passed. pathTo scales
		// its argument by ld, so notch n sits at pathTo((n/4)/ld) - one source
		// for the arc math.
		notches := int(ld * 4)
		for n := 1; n <= notches && n <= 4; n++ {
			nx, ny := pathTo(float64(n) / 4 / ld)
			r.drawGlowRect(screen, nx, ny, math.Max(3, h*0.012), clockWhite, fade*0.65, additiveGlowBlend)
		}
		if st < 1 {
			tx, ty := pathTo(1)
			r.drawGlowSprite(screen, tx, ty, h*0.055*w, clockWhite, fade*0.95, additiveGlowBlend)
		}
	}

	// The strip: sheared gear teeth tumbling + pawl-click arcs snapping in.
	if sweepT >= 1 {
		u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
		if u < 0.3 {
			r.drawSparkStar(screen, meetX, meetY, h*0.1*w*(1-u/0.3), clockBrass, clockWhite, fade*(1-u/0.3), 1)
		}
		r.arenaImpactRings(screen, meetX, meetY, u, reach*1.1, 0.5, h*0.013, 2, clockBrass, fade)
		// Pawl clicks: three short arc segments appear around the strike, stepwise.
		clicks := int(u * 6)
		for c := 0; c < 3 && c < clicks; c++ {
			base := -math.Pi/2 + float64(c)*2*math.Pi/3
			rad := reach * (0.6 + 0.2*float64(c))
			for k := 0; k <= 5; k++ {
				ang := base + (float64(k)/5-0.5)*0.7
				r.drawGlowSprite(screen, meetX+math.Cos(ang)*rad, meetY+math.Sin(ang)*rad*0.6,
					math.Max(2.5, h*0.012), clockBrass, fade*(1-u), additiveGlowBlend)
			}
		}
		// Sheared gear teeth: square brass flakes (shared debris beat).
		r.arenaDebrisSpray(screen, meetX, meetY, u, fade, h, 3.0, 0.4, 0.24, 0.32, 0.017, 14, 4, seed, 398, clockBrass, clockWhite)
	}
}

// Clockwork Pistol - the wound shot: a live cog spins around the slug as it
// flies, and spent steam puffs vent off the mechanism behind it. Reads as
// machinery even at a glance across the room.
func (r *Renderer) drawWeaponProjectileFxClockPistol(screen *ebiten.Image, cx, cy, size, dirX, dirY, critBoost float64, id int) {
	seed := id + 421
	fc := float64(r.game.frameCount)
	// Spinning cog ring around the slug.
	const teeth = 8
	for i := 0; i < teeth; i++ {
		ang := fc*0.28 + 2*math.Pi*float64(i)/teeth
		rr := size * 0.95
		if i%2 == 0 {
			rr *= 1.3
		}
		r.drawGlowRect(screen, cx+math.Cos(ang)*rr, cy+math.Sin(ang)*rr*0.8,
			math.Max(2, size*0.14), mixColor(clockBrass, clockWhite, float64(i%2)*0.5), 0.5*critBoost, additiveGlowBlend)
	}
	// Steam puffs venting behind: soft, widening, quick to thin.
	for k := 0; k < 6; k++ {
		t := 0.25 + 0.16*float64(k)
		wob := (auraHash(seed, k, 422, 0) - 0.5) * size * 0.9
		x := cx - dirX*size*3.4*t - dirY*wob
		y := cy - dirY*size*3.4*t + dirX*wob
		r.drawGlowSprite(screen, x, y, size*(0.28+0.1*float64(k)),
			[3]int{225, 230, 235}, (0.34-0.045*float64(k))*critBoost, additiveGlowBlend)
	}
	// A short brass pressure line ahead of the slug - wound, not fired.
	for i := 1; i <= 3; i++ {
		t := float64(i) / 3
		r.drawGlowSprite(screen, cx+dirX*size*(0.6+0.9*t), cy+dirY*size*(0.6+0.9*t),
			size*(0.16-0.03*t), clockBrass, (0.4-0.09*t)*critBoost, additiveGlowBlend)
	}
}
