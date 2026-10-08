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

// arenaImpactCloud gives heavy hits a ragged pressure front and rolling dust.
func (r *Renderer) arenaImpactCloud(screen *ebiten.Image, x, y, u, maxR, squashY float64, col [3]int, alpha float64) {
	r.drawImpactCloud(screen, x, y, maxR, maxR*squashY, u, col, alpha, r.weaponMaterialState.seed, r.weaponMaterialState.material != weaponHeat)
}

// arenaDebrisSpray throws `count` ballistic chips from (ox,oy) with 3-step
// ghost trails and a parabolic fall - the shared "something broke off" beat
// (gladius bronze chips, cogfang plate, escapement teeth). u is the finish
// phase 0..1; spread is the fan in radians around straight up; speed/gravity/
// size scale with h; every everyN-th chip takes the accent color.
func (r *Renderer) arenaDebrisSpray(screen *ebiten.Image, ox, oy, u, fade, h, spread, spdBase, speed, gravity, size float64, count, everyN, seed, salt int, base, accent [3]int) {
	for k := 0; k < count; k++ {
		ang := -math.Pi/2 + (auraHash(seed, k, salt, 0)-0.5)*spread
		spd := spdBase + auraHash(seed, k, salt+1, 0)
		for g := 0; g < 3; g++ {
			ug := u - float64(g)*0.05
			if ug < 0 {
				break
			}
			px := ox + math.Cos(ang)*spd*h*speed*ug
			py := oy + math.Sin(ang)*spd*h*speed*ug + ug*ug*h*gravity
			f := 1 - 0.3*float64(g)
			c := base
			if k%everyN == 0 {
				c = accent
			}
			r.drawWeaponShard(screen, px, py, math.Max(3, h*size*(1-0.4*ug)*f), c, fade*(1-ug)*f*f, ug, seed+salt+k*37, false)
		}
	}
}

// Champion's Gladius sweeps its narrow blade through a single cutting arc.
func (r *Renderer) drawMeleeFxArenaGladius(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	r.drawIdentityStrike(screen, s, cx, cy, screenH, "arena_gladius", "slash")
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
				r.drawWeaponShard(screen, sx, sy, math.Max(3, h*0.013*(1-su*0.5)), mixColor(edge, white, sb), fade*(1-su), su, seed+i*211+k*37, false)
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
		r.drawWeaponShard(screen, px, py, math.Max(3, h*0.026*tum*(1-0.3*u)), bronze, fade*(1-u)*0.95, u, seed+k*37, false)
	}
	_ = lead
}

// Morningstar is a rigid spiked mace, matching its red grip and gold spikes.
func (r *Renderer) drawMeleeFxArenaMorningstar(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	r.drawIdentityStrike(screen, s, cx, cy, screenH, "arena_morningstar", "smash")
}

// Hasta drives its leaf-headed spear forward with a short trailing wake.
func (r *Renderer) drawMeleeFxArenaHasta(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	r.drawIdentityStrike(screen, s, cx, cy, screenH, "arena_hasta", "lunge")
}

// Retiarius Trident lunges with all three prongs attached to its visible shaft.
func (r *Renderer) drawMeleeFxArenaTrident(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	r.drawIdentityStrike(screen, s, cx, cy, screenH, "arena_trident", "lunge")
}

// Parrying Dagger keeps its narrow blade aligned with the forward thrust.
func (r *Renderer) drawMeleeFxArenaParry(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	r.drawIdentityStrike(screen, s, cx, cy, screenH, "arena_parry", "stab")
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
	r.arenaImpactCloud(screen, impX, impY+h*0.06, u, reach*1.2, 0.4, gold, fade)

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
			r.drawWeaponShard(screen, sx, sy, math.Max(3, h*0.018*(1-0.4*ug)*f), c, fade*(1-ug)*f*f, ug, seed+k*37, false)
		}
	}
}

// Bronze Cesti alternate two staggered punches with their plated fist models.
func (r *Renderer) drawMeleeFxArenaCesti(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	r.drawGauntletStrike(screen, s, cx, cy, screenH)
}
