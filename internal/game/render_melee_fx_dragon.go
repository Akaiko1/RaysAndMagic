package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Drakeforged weapon flourishes - the Scalewright's arsenal. The set's shared
// signature is LIVING FIRE UNDER BLACK SCALE: every stroke is a dark, almost
// unlit scale undercoat split by an ember-hot core, and every finish plays the
// weapon's own rider - the brand, the closing jaws, the hatching egg, the
// armor-ignoring puncture, the silt bloom, the venom spatter, the roar, the
// echoing eye. Same layered vocabulary as the arena/clock sets (glow ribbon +
// white-hot core, deterministic debris with ghost trails, impact beats).

// The brood's shared palette.
var (
	dragonScale = [3]int{72, 48, 44}    // black-red scale, barely lit
	dragonEmber = [3]int{255, 138, 46}  // the fire it grew in
	dragonHot   = [3]int{255, 224, 170} // white-hot core
	dragonBone  = [3]int{236, 226, 205} // fang and jawbone
	dragonBlood = [3]int{196, 60, 44}   // what the jaws keep
)

// dragonEmberRise scatters slow-rising ember flecks off a stroke's trail -
// the set's connective tissue. Each fleck is a 3-ghost streak that cools from
// hot to ember to scale as it climbs.
func (r *Renderer) dragonEmberRise(screen *ebiten.Image, seed, salt int, u, x, y, spread, h, fade float64, count int) {
	if u <= 0 || u >= 1 {
		return
	}
	for k := 0; k < count; k++ {
		born := auraHash(seed, k, salt, 0) * 0.5
		eu := (u - born) / (1 - born)
		if eu <= 0 {
			continue
		}
		ex := x + (auraHash(seed, k, salt+1, 0)-0.5)*spread
		drift := math.Sin(eu*7+float64(k)) * h * 0.02
		for g := 0; g < 3; g++ {
			gu := eu - float64(g)*0.06
			if gu < 0 {
				break
			}
			f := 1 - 0.3*float64(g)
			col := mixColor(dragonHot, dragonEmber, math.Min(1, gu*1.8))
			if gu > 0.6 {
				col = mixColor(dragonEmber, dragonScale, (gu-0.6)/0.4)
			}
			r.drawGlowRect(screen, ex+drift, y-gu*h*0.34,
				math.Max(2.5, h*0.011*f*(1-gu*0.5)), col, fade*(1-gu)*f, additiveGlowBlend)
		}
	}
}

// Drakefang - the Broodmother's tooth: a deep scale-dark crescent split by an
// ember core, shedding rising embers. The finish is the BRAND (35% ignite made
// visible): twin fang punctures flare white and linger as a slow-cooling mark
// long after the swing is gone.
func (r *Renderer) drawMeleeFxDragonFang(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	reach := h * 0.3
	pivotX, pivotY := cx, cy+reach*0.5
	R := reach * 1.35
	th0, th1 := -math.Pi/2-1.15, -math.Pi/2+0.95
	cur := th0 + (th1-th0)*(1-(1-lead)*(1-lead))
	arcAt := func(t float64) (float64, float64) {
		theta := th0 + (cur-th0)*t
		return pivotX + math.Cos(theta)*R, pivotY + math.Sin(theta)*R*0.92
	}

	// Scale undercoat: wide, dark, dense - the unlit tooth.
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   arcAt,
		width:  func(t float64) float64 { return (30 + 16*math.Sin(math.Pi*t)) * w },
		color:  func(t float64) [3]int { return mixColor(dragonScale, dragonBlood, 0.35*t) },
		alpha:  func(t float64) float64 { return 0.82 },
		length: R * 2.1, seed: seed, salt: 510, blend: ebiten.BlendSourceOver,
	}, 1, progress)
	// The fire it grew in: ember core sharpening to white toward the edge.
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   arcAt,
		width:  func(t float64) float64 { return (11 + 7*math.Sin(math.Pi*t)) * w },
		color:  func(t float64) [3]int { return mixColor(dragonEmber, dragonHot, t*t) },
		alpha:  func(t float64) float64 { return 0.66 + 0.34*t },
		length: R * 2.1, seed: seed, salt: 512, blend: additiveGlowBlend,
	}, 1, progress)

	// Embers rise off the trail while the blade is moving.
	mx, my := arcAt(math.Max(0, lead-0.25))
	r.dragonEmberRise(screen, seed, 514, math.Min(1, progress*1.4), mx, my, R*0.8, h, fade, 9)

	if sweepT < 1 {
		tx, ty := arcAt(1)
		r.drawGlowSprite(screen, tx, ty, h*0.04*w, dragonHot, fade, additiveGlowBlend)
		return
	}

	// The brand: twin fang punctures at the bite point - flare, then a mark
	// that cools from white through ember to dull scale, refusing to leave.
	u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
	bx, by := arcAt(0.96)
	gap := h * 0.045
	for _, off := range []float64{-gap, gap} {
		fx, fy := bx+off, by-math.Abs(off)*0.3
		if u < 0.22 {
			r.drawSparkStar(screen, fx, fy, h*0.05*w*(1-u/0.22), dragonHot, dragonHot, fade*(1-u/0.22), 1)
		}
		cool := math.Min(1, u*1.15)
		col := mixColor(dragonHot, dragonEmber, math.Min(1, cool*1.6))
		if cool > 0.55 {
			col = mixColor(dragonEmber, dragonScale, (cool-0.55)/0.45)
		}
		// The puncture: a hot point inside a scorched ring - the ring is laid
		// down as light first so the charred centre has an edge to sit in.
		r.drawGlowSprite(screen, fx, fy, h*0.062*w, mixColor(dragonEmber, dragonScale, 0.45), fade*0.6, additiveGlowBlend)
		r.drawGlowSprite(screen, fx, fy, h*0.036*w, dragonScale, fade*0.85, ebiten.BlendSourceOver)
		r.drawGlowRect(screen, fx, fy, math.Max(4, h*0.032*w*(1-0.25*cool)), col, fade*(1-0.3*cool), additiveGlowBlend)
	}
	r.dragonEmberRise(screen, seed, 517, u, bx, by-gap, gap*3, h, fade*0.8, 6)
}

// Wyrmcleaver - the closing jaws: TWO opposing crescents, an upper and a lower
// jaw studded with bone teeth, closing on the anchor through the sweep. The
// finish is the SNAP (the sub-15% execute): the jaws slam shut in a white-red
// flash and bone chips spray from the bite line.
func (r *Renderer) drawMeleeFxDragonJaws(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	reach := h * 0.3
	jawY := cy - reach*0.15
	span := reach * 1.5
	// gape: jaws start wide open and close with an accelerating bite.
	gape := reach * 0.62 * (1 - lead*lead)

	jaw := func(sign float64) func(t float64) (float64, float64) {
		return func(t float64) (float64, float64) {
			x := cx - span/2 + span*t
			bow := math.Sin(math.Pi*t) * reach * 0.5
			return x, jawY + sign*(gape+bow*0.35)
		}
	}
	for j, sign := range []float64{-1, 1} {
		path := jaw(sign)
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   path,
			width:  func(t float64) float64 { return (24 + 12*math.Sin(math.Pi*t)) * w },
			color:  func(t float64) [3]int { return mixColor(dragonScale, dragonBlood, 0.45) },
			alpha:  func(t float64) float64 { return 0.84 },
			length: span * 1.2, seed: seed, salt: 530 + j*4, blend: ebiten.BlendSourceOver,
		}, lead, progress)
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   path,
			width:  func(t float64) float64 { return (9 + 5*math.Sin(math.Pi*t)) * w },
			color:  func(t float64) [3]int { return mixColor(dragonBlood, dragonHot, 0.35+0.4*math.Sin(math.Pi*t)) },
			alpha:  func(t float64) float64 { return 0.7 },
			length: span * 1.2, seed: seed, salt: 532 + j*4, blend: additiveGlowBlend,
		}, lead, progress)
		// Bone teeth on the inner edge, pointing into the bite.
		const teeth = 7
		for i := 0; i < teeth; i++ {
			tt := (float64(i) + 0.5) / teeth
			if tt > lead {
				break
			}
			px, py := path(tt)
			tl := (8 + 5*math.Sin(math.Pi*tt)) * w
			r.drawGlowRect(screen, px, py-sign*tl*0.5, math.Max(2.5, tl*0.42),
				mixColor(dragonBone, dragonHot, 0.2), fade*0.9, additiveGlowBlend)
		}
	}

	if sweepT >= 1 {
		// The SNAP: a white-red flash bar along the closed bite line, bone
		// chips spraying with ghost trails, and a blood-dark afterglow.
		u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
		if u < 0.3 {
			f := 1 - u/0.3
			r.drawDissolveStroke(screen, dissolveStroke{
				path:   func(t float64) (float64, float64) { return cx - span/2 + span*t, jawY },
				width:  func(t float64) float64 { return (9 + 7*math.Sin(math.Pi*t)) * w * f },
				color:  func(t float64) [3]int { return mixColor(dragonHot, dragonBlood, t*0.3) },
				alpha:  func(t float64) float64 { return 0.9 * f },
				length: span, seed: seed, salt: 540, blend: additiveGlowBlend,
			}, 1, 0.2)
			r.drawSparkStar(screen, cx, jawY, h*0.075*w*f, dragonHot, dragonBone, fade*f, 2.2)
		}
		const chips = 10
		for k := 0; k < chips; k++ {
			ang := (auraHash(seed, k, 542, 0) - 0.5) * 2.4
			spd := 0.5 + auraHash(seed, k, 543, 0)
			for g := 0; g < 3; g++ {
				gu := u - float64(g)*0.05
				if gu < 0 {
					break
				}
				f := 1 - 0.3*float64(g)
				bx := cx + math.Sin(ang)*spd*h*0.24*gu
				by := jawY - math.Abs(math.Cos(ang))*spd*h*0.17*gu + gu*gu*h*0.26
				r.drawGlowRect(screen, bx, by, math.Max(2.5, h*0.012*f),
					mixColor(dragonBone, dragonBlood, auraHash(seed, k, 544, 0)*0.4), fade*(1-gu)*f*f, additiveGlowBlend)
			}
		}
	}
}

// Ember Egg - the mace that never cooled: an overhead smash led by a pulsing
// egg of fire. The finish HATCHES it (the death-burst rider): dark shell
// shards with hot rims blow outward and an ember ring expands to the burst
// radius while the core keeps breathing on the ground.
func (r *Renderer) drawMeleeFxDragonEmberEgg(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	reach := h * 0.34
	topY, hitY := cy-reach*1.05, cy+reach*0.12
	drop := func(t float64) (float64, float64) {
		return cx + math.Sin(t*math.Pi*0.5)*reach*0.22, topY + (hitY-topY)*t*t
	}

	r.drawDissolveStroke(screen, dissolveStroke{
		path:   drop,
		width:  func(t float64) float64 { return (22 + 12*t) * w },
		color:  func(t float64) [3]int { return mixColor(dragonScale, dragonEmber, 0.3+0.3*t) },
		alpha:  func(t float64) float64 { return 0.8 },
		length: reach * 1.35, seed: seed, salt: 550, blend: ebiten.BlendSourceOver,
	}, lead, progress)

	if sweepT < 1 {
		// The egg itself rides the drop, breathing: a hot core inside a dark
		// shell that never quite contains it. Layer order matters - a dark
		// source-over ball on a dark scene is invisible, so the RIM LIGHT is
		// laid down first and the shell is dark AGAINST it.
		ex, ey := drop(lead)
		pulse := 0.82 + 0.18*math.Sin(float64(r.game.frameCount)*0.45)
		// Oval silhouette: three stacked discs, taller than wide, so it reads
		// as an EGG and not as another glow dot at the end of the arc.
		egg := func(radius, alpha float64, col [3]int, blend ebiten.Blend) {
			for i, lobe := range []float64{-0.45, 0, 0.45} {
				sz := radius
				if i != 1 {
					sz *= 0.72
				}
				r.drawGlowSprite(screen, ex, ey+lobe*radius*1.15, sz, col, alpha, blend)
			}
		}
		egg(h*0.115*w, fade*0.45, dragonEmber, additiveGlowBlend) // rim light first
		egg(h*0.082*w, fade*0.95, dragonScale, ebiten.BlendSourceOver)
		egg(h*0.05*w*pulse, fade*0.9, dragonEmber, additiveGlowBlend)
		r.drawGlowSprite(screen, ex, ey, h*0.03*w*pulse, dragonHot, fade, additiveGlowBlend)
		return
	}

	// Hatch. Shell shards out, ember ring to the burst radius, breathing core.
	u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
	r.arenaImpactRings(screen, cx, hitY, u, reach*1.5, 0.5, h*0.013*w, 2, dragonEmber, fade)
	const shards = 9
	for k := 0; k < shards; k++ {
		ang := 2*math.Pi*float64(k)/shards + auraHash(seed, k, 552, 0)*0.5
		spd := 0.6 + auraHash(seed, k, 553, 0)*0.6
		for g := 0; g < 3; g++ {
			gu := u - float64(g)*0.05
			if gu < 0 {
				break
			}
			f := 1 - 0.3*float64(g)
			sx := cx + math.Cos(ang)*spd*h*0.3*gu
			sy := hitY + math.Sin(ang)*spd*h*0.19*gu + gu*gu*h*0.2
			// Dark shard body with a hot rim - the shell was full of fire.
			r.drawGlowRect(screen, sx, sy, math.Max(3, h*0.018*f), dragonScale, fade*(1-gu)*f, ebiten.BlendSourceOver)
			r.drawGlowRect(screen, sx, sy, math.Max(2, h*0.008*f), mixColor(dragonEmber, dragonHot, auraHash(seed, k, 554, 0)), fade*(1-gu)*f*f, additiveGlowBlend)
		}
	}
	pulse := 0.7 + 0.3*math.Sin(float64(r.game.frameCount)*0.5)
	r.drawGlowSprite(screen, cx, hitY, h*0.06*w*(1-0.4*u), dragonEmber, fade*(1-0.5*u)*pulse, additiveGlowBlend)
	r.dragonEmberRise(screen, seed, 556, u, cx, hitY, reach*0.9, h, fade, 12)
}

// Broodspike - the wing-spur lance that no armor keeps out. A cold pale
// needle thrust with a compression cone at the tip; the finish punches
// THROUGH: the line extends past the impact point and exit sparks burst on
// the far side - ten points of true damage, visibly ignoring the plate.
func (r *Renderer) drawMeleeFxDragonBroodspike(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	pale, spur := [3]int{196, 210, 218}, [3]int{240, 248, 252}
	reach := h * 0.52
	baseY := cy + reach*0.42
	// The thrust line runs up-screen; after the hit it EXTENDS past the mark.
	throughExt := 0.0
	if sweepT >= 1 {
		throughExt = math.Min(1, (progress-meleeSweepFrac)/(0.3*(1-meleeSweepFrac))) * reach * 0.34
	}
	spike := func(t float64) (float64, float64) {
		return cx + (1-t)*h*0.02, baseY - (reach+throughExt)*t
	}

	r.drawDissolveStroke(screen, dissolveStroke{
		path:   spike,
		width:  func(t float64) float64 { return (20 - 11*t) * w },
		color:  func(t float64) [3]int { return mixColor(dragonScale, pale, 0.5+0.5*t) },
		alpha:  func(t float64) float64 { return 0.72 },
		length: reach * 1.2, seed: seed, salt: 560, blend: additiveGlowBlend,
	}, 1-(1-lead)*(1-lead), progress)
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   spike,
		width:  func(t float64) float64 { return (7 - 2.5*t) * w },
		color:  func(t float64) [3]int { return spur },
		alpha:  func(t float64) float64 { return 0.6 + 0.4*t },
		length: reach * 1.2, seed: seed, salt: 562, blend: additiveGlowBlend,
	}, 1-(1-lead)*(1-lead), progress)

	// Compression cone: air shoved aside at the tip while the needle flies.
	if sweepT < 1 {
		tx, ty := spike(1 - (1-lead)*(1-lead))
		for i := 0; i < 5; i++ {
			cu := lead - float64(i)*0.05
			if cu < 0 {
				break
			}
			spreadR := h * (0.032 + 0.026*float64(i))
			a := fade * (0.62 - 0.1*float64(i))
			r.drawGlowSprite(screen, tx-h*0.01, ty+float64(i)*h*0.036, spreadR, pale, a, additiveGlowBlend)
		}
		r.drawGlowSprite(screen, tx, ty, h*0.03*w, spur, fade, additiveGlowBlend)
		return
	}

	// Punch-through: exit sparks on the FAR side of the mark, angled onward.
	u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
	markY := baseY - reach
	if u < 0.25 {
		r.drawSparkStar(screen, cx, markY, h*0.06*w*(1-u/0.25), spur, spur, fade*(1-u/0.25), 0.8)
	}
	const sparks = 8
	for k := 0; k < sparks; k++ {
		ang := -math.Pi/2 + (auraHash(seed, k, 564, 0)-0.5)*0.9
		spd := 0.6 + auraHash(seed, k, 565, 0)*0.7
		for g := 0; g < 3; g++ {
			gu := u - float64(g)*0.04
			if gu < 0 {
				break
			}
			f := 1 - 0.3*float64(g)
			sx := cx + math.Cos(ang)*spd*h*0.2*gu
			sy := markY - reach*0.2 + math.Sin(ang)*spd*h*0.26*gu
			r.drawGlowRect(screen, sx, sy, math.Max(2.5, h*0.01*f), mixColor(pale, spur, auraHash(seed, k, 566, 0)), fade*(1-gu)*f*f, additiveGlowBlend)
		}
	}
}

// Tarn Trident - drowned bronze. The thrust drags the lake with it: sagging
// water streaks fall off the tines, and the finish is the SILT BLOOM (the 30%
// slow): a heavy murk cloud that hangs at the wound and sinks, with one lazy
// weed strand trailing down.
func (r *Renderer) drawMeleeFxDragonTarn(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	bronze, tarn, murk := [3]int{150, 118, 66}, [3]int{86, 152, 142}, [3]int{42, 74, 70}
	wet := [3]int{206, 246, 240} // the sheet of water riding the tines
	reach := h * 0.48
	baseY := cy + reach*0.4

	// Two flanking tines lag the center one - a spear that remembers being a
	// trident. Each is bronze under a wet teal sheen.
	for i, off := range []float64{-0.05, 0, 0.05} {
		lag := 0.06 * math.Abs(float64(i)-1)
		p := math.Min(1, math.Max(0, (progress-lag)/(1-lag)))
		st := math.Min(1, p/meleeSweepFrac)
		l := 1 - (1-st)*(1-st)
		tine := func(t float64) (float64, float64) {
			return cx + off*h*(1-t*0.3), baseY - reach*t*(1-0.12*math.Abs(float64(i)-1))
		}
		// Two layers per tine - wet bronze glow under a bright wet core. One
		// flat stroke read half as heavy as the arena trident it stands next
		// to; the core is what gives a thrust its presence.
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   tine,
			width:  func(t float64) float64 { return (18 - 6*t) * w },
			color:  func(t float64) [3]int { return mixColor(bronze, tarn, 0.55+0.45*t) },
			alpha:  func(t float64) float64 { return 0.74 },
			length: reach, seed: seed, salt: 570 + i*3, blend: additiveGlowBlend,
		}, l, p)
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   tine,
			width:  func(t float64) float64 { return (6 - 2*t) * w },
			color:  func(t float64) [3]int { return mixColor(tarn, wet, 0.4+0.6*t) },
			alpha:  func(t float64) float64 { return 0.6 + 0.35*t },
			length: reach, seed: seed, salt: 573 + i*3, blend: additiveGlowBlend,
		}, l, p)
		if st < 1 {
			px, py := tine(l)
			r.drawGlowSprite(screen, px, py, h*0.032*w, wet, fade, additiveGlowBlend)
		}
	}

	// Water dragged with the thrust: streaks that rise with the tines then
	// sag and fall - the lake does not let go.
	if lead > 0.15 {
		const drips = 8
		for k := 0; k < drips; k++ {
			du := math.Max(0, math.Min(1, progress*1.3-auraHash(seed, k, 574, 0)*0.4))
			if du <= 0 {
				continue
			}
			dx := cx + (auraHash(seed, k, 575, 0)-0.5)*h*0.14
			rise := math.Sin(math.Min(1, du*1.6)*math.Pi) * reach * (0.3 + 0.4*auraHash(seed, k, 576, 0))
			sag := du * du * reach * 0.55
			r.drawGlowRect(screen, dx, baseY-rise+sag, math.Max(3, h*0.016),
				mixColor(wet, murk, du), fade*(1-du*0.6), additiveGlowBlend)
		}
	}

	if sweepT >= 1 {
		// Silt bloom: a murk cloud that expands a little and SINKS a lot,
		// alpha refusing to fade as fast as anything else in the set.
		u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
		bloomY := baseY - reach*0.92
		const motes = 14
		for k := 0; k < motes; k++ {
			ang := 2 * math.Pi * auraHash(seed, k, 578, 0)
			rr := reach * (0.1 + 0.32*auraHash(seed, k, 579, 0)) * math.Min(1, u*2.2)
			sink := u * u * reach * 0.5 * (0.4 + auraHash(seed, k, 580, 0))
			mx := cx + math.Cos(ang)*rr
			my := bloomY + math.Sin(ang)*rr*0.5 + sink
			col := mixColor(tarn, murk, 0.4+0.6*u)
			r.drawGlowSprite(screen, mx, my, math.Max(5, h*0.03*(1+u*0.5)), col, fade*(0.66-0.3*u), additiveGlowBlend)
		}
		// One weed strand: a lazy line sagging off the wound.
		strand := func(t float64) (float64, float64) {
			return cx + h*0.03 + math.Sin(t*3+u*2)*h*0.014, bloomY + t*reach*0.5*math.Min(1, 0.3+u)
		}
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   strand,
			width:  func(t float64) float64 { return (5 - 2*t) * w },
			color:  func(t float64) [3]int { return mixColor(murk, tarn, 0.3) },
			alpha:  func(t float64) float64 { return 0.5 },
			length: reach * 0.55, seed: seed, salt: 582, blend: additiveGlowBlend,
		}, math.Min(1, u*1.6), progress)
	}
}

// Hatchling Fang - the first venom. A small, very fast green-glass stab that
// beads venom off the tip; the finish is the SPATTER (the 35% poison): drops
// that land, cling and DRIP downward on glass-green glints, outliving the
// stab itself.
func (r *Renderer) drawMeleeFxDragonHatchling(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	venom, glass := [3]int{118, 222, 88}, [3]int{212, 255, 196}
	reach := h * 0.34
	baseX, baseY := cx+h*0.05, cy+reach*0.35
	ld := 1 - (1-lead)*(1-lead)*(1-lead) // extra-fast snap - it is a hatchling
	stab := func(t float64) (float64, float64) {
		return baseX - t*h*0.06, baseY - reach*t
	}

	r.drawDissolveStroke(screen, dissolveStroke{
		path:   stab,
		width:  func(t float64) float64 { return (15 - 7*t) * w },
		color:  func(t float64) [3]int { return mixColor(dragonScale, venom, 0.5+0.4*t) },
		alpha:  func(t float64) float64 { return 0.7 },
		length: reach, seed: seed, salt: 590, blend: additiveGlowBlend,
	}, ld, progress)
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   stab,
		width:  func(t float64) float64 { return (6 - 2*t) * w },
		color:  func(t float64) [3]int { return glass },
		alpha:  func(t float64) float64 { return 0.55 + 0.45*t },
		length: reach, seed: seed, salt: 592, blend: additiveGlowBlend,
	}, ld, progress)

	// Venom beads shaken off the moving tip.
	if sweepT < 1 {
		tx, ty := stab(ld)
		r.drawGlowSprite(screen, tx, ty, h*0.024*w, glass, fade, additiveGlowBlend)
		const beads = 5
		for k := 0; k < beads; k++ {
			bu := lead - auraHash(seed, k, 594, 0)*0.5
			if bu <= 0 {
				continue
			}
			bx, by := stab(math.Max(0, bu))
			r.drawGlowRect(screen, bx+(auraHash(seed, k, 595, 0)-0.5)*h*0.05, by+bu*h*0.05,
				math.Max(2, h*0.008), venom, fade*0.8, additiveGlowBlend)
		}
		return
	}

	// Spatter: drops land around the puncture, cling, then drip in slow runs.
	u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
	tipX, tipY := stab(1)
	if u < 0.2 {
		r.drawSparkStar(screen, tipX, tipY, h*0.04*w*(1-u/0.2), glass, venom, fade*(1-u/0.2), 1)
	}
	const drops = 7
	for k := 0; k < drops; k++ {
		ang := (auraHash(seed, k, 596, 0) - 0.5) * 2.6
		dist := h * (0.03 + 0.07*auraHash(seed, k, 597, 0))
		dx := tipX + math.Sin(ang)*dist
		dy := tipY - math.Abs(math.Cos(ang))*dist*0.5
		land := math.Min(1, u*3)
		run := math.Max(0, u-0.3) * reach * 0.45 * (0.3 + auraHash(seed, k, 598, 0))
		// The clinging drop and its downward run.
		r.drawGlowRect(screen, dx, dy+run, math.Max(2, h*0.009*(1+0.4*land)),
			mixColor(glass, venom, land), fade*(1-u*0.45), additiveGlowBlend)
		if run > h*0.02 {
			r.drawGlowRect(screen, dx, dy+run*0.55, math.Max(2, h*0.006), venom, fade*(1-u*0.6)*0.7, additiveGlowBlend)
		}
	}
}

// Scalebreaker - the sundering roar. The hammer drops and the FINISH IS SOUND:
// expanding concentric shock rings (the roar made visible) while the target's
// own scales shed and flutter off - the 25% weaken, played literally.
func (r *Renderer) drawMeleeFxDragonRoar(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	sand, roar := [3]int{214, 178, 118}, [3]int{255, 238, 200}
	reach := h * 0.34
	topY, hitY := cy-reach*1.0, cy+reach*0.14
	drop := func(t float64) (float64, float64) {
		return cx - h*0.05*(1-t), topY + (hitY-topY)*t*t
	}

	r.drawDissolveStroke(screen, dissolveStroke{
		path:   drop,
		width:  func(t float64) float64 { return (26 + 10*t) * w },
		color:  func(t float64) [3]int { return mixColor(dragonScale, sand, 0.35+0.3*t) },
		alpha:  func(t float64) float64 { return 0.82 },
		length: reach * 1.3, seed: seed, salt: 600, blend: ebiten.BlendSourceOver,
	}, lead, progress)
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   drop,
		width:  func(t float64) float64 { return (10 + 5*t) * w },
		color:  func(t float64) [3]int { return mixColor(sand, roar, t) },
		alpha:  func(t float64) float64 { return 0.6 + 0.4*t },
		length: reach * 1.3, seed: seed, salt: 602, blend: additiveGlowBlend,
	}, lead, progress)

	if sweepT < 1 {
		hx, hy := drop(lead)
		r.drawGlowSprite(screen, hx, hy, h*0.05*w, mixColor(sand, roar, 0.5), fade, additiveGlowBlend)
		return
	}

	// The roar: THREE ring salvos - sound has echoes - plus shed scales.
	u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
	r.arenaImpactRings(screen, cx, hitY, u, reach*1.7, 0.42, h*0.015*w, 3, mixColor(sand, roar, 0.6), fade*1.1)
	if u < 0.2 {
		r.drawSparkStar(screen, cx, hitY, h*0.08*w*(1-u/0.2), roar, roar, fade*(1-u/0.2), 1.6)
	}
	// Scales shed off the struck target: dark plates with sandy rims that
	// flip as they fall (width oscillates), fluttering rather than flying.
	const scales = 8
	for k := 0; k < scales; k++ {
		born := auraHash(seed, k, 604, 0) * 0.3
		su := (u - born) / (1 - born)
		if su <= 0 {
			continue
		}
		sx := cx + (auraHash(seed, k, 605, 0)-0.5)*reach*1.1
		sway := math.Sin(su*6+float64(k)*2) * h * 0.03
		sy := hitY - reach*0.5*auraHash(seed, k, 606, 0) + su*su*reach*0.75
		flip := math.Abs(math.Sin(su*9 + float64(k)))
		r.drawGlowRect(screen, sx+sway, sy, math.Max(3, h*0.016*(0.4+0.6*flip)),
			dragonScale, fade*(1-su)*0.95, ebiten.BlendSourceOver)
		r.drawGlowRect(screen, sx+sway, sy, math.Max(2, h*0.007*(0.4+0.6*flip)),
			sand, fade*(1-su)*0.8, additiveGlowBlend)
	}
}

// Verdant Eye - the scepter that watches and approves. A short green arc, and
// at the impact AN EYE OPENS: iris ring around a slit pupil, blinks once -
// then the ENTIRE flourish replays fainter a beat later. The 15% spell echo,
// played on the weapon itself.
func (r *Renderer) drawMeleeFxDragonEye(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, _, _ := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	moss, leaf, iris := [3]int{74, 130, 66}, [3]int{112, 226, 118}, [3]int{198, 255, 176}

	// The whole flourish as a function of its own local clock, so the echo is
	// literally the same drawing played again, fainter and slightly later.
	pass := func(p, gain float64, saltOff int) {
		if p <= 0 || p > 1 {
			return
		}
		st := math.Min(1, p/meleeSweepFrac)
		l := 1 - (1-st)*(1-st)
		f := gain
		if p > 0.72 {
			f *= 1 - (p-0.72)/0.28
		}
		if f <= 0 {
			return
		}
		reach := h * 0.3
		pivotX, pivotY := cx, cy+reach*0.45
		R := reach * 1.25
		th0, th1 := -math.Pi/2+0.95, -math.Pi/2-0.85 // right-to-left: the eye reads back at you
		cur := th0 + (th1-th0)*l
		arcAt := func(t float64) (float64, float64) {
			theta := th0 + (cur-th0)*t
			return pivotX + math.Cos(theta)*R, pivotY + math.Sin(theta)*R*0.9
		}
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   arcAt,
			width:  func(t float64) float64 { return (22 + 12*math.Sin(math.Pi*t)) * w },
			color:  func(t float64) [3]int { return mixColor(moss, leaf, 0.4+0.4*t) },
			alpha:  func(t float64) float64 { return 0.72 * f },
			length: R * 1.9, seed: seed, salt: 610 + saltOff, blend: additiveGlowBlend,
		}, 1, p)
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   arcAt,
			width:  func(t float64) float64 { return (8 + 5*math.Sin(math.Pi*t)) * w },
			color:  func(t float64) [3]int { return mixColor(leaf, iris, t) },
			alpha:  func(t float64) float64 { return (0.55 + 0.45*t) * f },
			length: R * 1.9, seed: seed, salt: 612 + saltOff, blend: additiveGlowBlend,
		}, 1, p)

		if st >= 1 {
			// The eye: iris ring + slit pupil; it blinks once (squash) and the
			// pupil tracks a touch sideways - it is watching the caster.
			u := (p - meleeSweepFrac) / (1 - meleeSweepFrac)
			ex, ey := arcAt(0.94)
			open := math.Min(1, u*3.5)
			blink := 1.0
			if u > 0.45 && u < 0.62 {
				blink = math.Abs(math.Cos((u - 0.45) / 0.17 * math.Pi))
			}
			// The eye is this weapon's whole signature, so it is drawn big
			// enough to read: a lit sclera to sit the dark pupil against, an
			// iris ring of bright segments, then the slit.
			irisR := h * 0.09 * open
			r.drawGlowSprite(screen, ex, ey, irisR*1.15, mixColor(moss, leaf, 0.5), f*fade*0.5, additiveGlowBlend)
			const segs = 16
			for i := 0; i < segs; i++ {
				ang := 2 * math.Pi * float64(i) / segs
				r.drawGlowRect(screen, ex+math.Cos(ang)*irisR, ey+math.Sin(ang)*irisR*0.62*blink,
					math.Max(3, h*0.017*w), mixColor(leaf, iris, 0.6), f*fade*(1-u*0.3), additiveGlowBlend)
			}
			track := math.Sin(u*2.4) * irisR * 0.3
			pupilH := irisR * 0.85 * blink
			for i := 0; i < 6; i++ {
				py := ey - pupilH/2 + pupilH*float64(i)/5
				r.drawGlowRect(screen, ex+track, py, math.Max(4, h*0.017*w), dragonScale, f*fade, ebiten.BlendSourceOver)
			}
			r.drawGlowSprite(screen, ex+track, ey, h*0.024*open*blink, iris, f*fade, additiveGlowBlend)
			// Moss motes drift off the open eye.
			const motes = 5
			for k := 0; k < motes; k++ {
				mu := math.Max(0, u-auraHash(seed, k, 616+saltOff, 0)*0.5)
				if mu <= 0 {
					continue
				}
				r.drawGlowRect(screen, ex+(auraHash(seed, k, 617+saltOff, 0)-0.5)*irisR*3, ey-mu*h*0.16,
					math.Max(2, h*0.007), leaf, f*fade*(1-mu), additiveGlowBlend)
			}
		}
	}

	pass(progress, 1, 0)
	// The echo: same flourish, delayed to land after the first finishes its
	// beat, at less than half strength - approved, repeated.
	pass((progress-0.42)/0.58, 0.42, 40)
}

// ============================ DRAKEFORGED RANGED ============================
// Overlays on top of the normal arrow silhouette (weaponProjectileFxStyleDraw).

// Wyrmspine Wing - the bow strung on wing-tendon. The bolt FLIES: membrane
// vanes fan off the shaft and beat slowly, shedding scale-flecks in the
// slipstream. This is the weapon that frees the whole party to shoot on the
// move, so the arrow itself should look airborne rather than launched.
func (r *Renderer) drawWeaponProjectileFxDragonWing(screen *ebiten.Image, cx, cy, size, dirX, dirY, critBoost float64, id int) {
	seed := id + 601
	fc := float64(r.game.frameCount)
	wing, vein := [3]int{188, 226, 176}, [3]int{232, 250, 224}
	// Wing beat: the vanes sweep between swept-back and spread.
	beat := 0.55 + 0.45*math.Sin(fc*0.34)
	nx, ny := -dirY, dirX
	for _, side := range []float64{-1, 1} {
		// Two membrane vanes per side, drawn as SOLID BARS (a chain of dots
		// reads as dots at these sizes): a leading spar plus a trailing one,
		// with the membrane suggested by a soft glow slung between them.
		var prevX, prevY float64
		for i := 0; i < 2; i++ {
			root := 0.25 + 0.65*float64(i)
			span := size * (1.15 + 0.7*float64(i)) * beat
			ax := cx - dirX*size*root
			ay := cy - dirY*size*root
			tipX := ax + nx*side*span - dirX*size*0.7*float64(i)
			tipY := ay + ny*side*span - dirY*size*0.7*float64(i)
			r.fxSegment(screen, ax, ay, tipX, tipY, math.Max(2.5, size*0.16), wing, 0.6*critBoost, additiveGlowBlend)
			r.fxSegment(screen, ax, ay, tipX, tipY, math.Max(2, size*0.07), vein, 0.75*critBoost, additiveGlowBlend)
			if i == 1 {
				// Membrane: the trailing edge closing the two spars.
				r.fxSegment(screen, prevX, prevY, tipX, tipY, math.Max(2, size*0.1), wing, 0.4*critBoost, additiveGlowBlend)
				r.drawGlowSprite(screen, (prevX+tipX)/2, (prevY+tipY)/2, size*0.5*beat, wing, 0.22*critBoost, additiveGlowBlend)
			}
			prevX, prevY = tipX, tipY
		}
	}
	// Slipstream: scale flecks peeling off behind, cooling to scale-dark.
	for k := 0; k < 6; k++ {
		t := 0.3 + 0.19*float64(k)
		wob := (auraHash(seed, k, 602, 0) - 0.5) * size * 1.1
		x := cx - dirX*size*3.6*t - dirY*wob
		y := cy - dirY*size*3.6*t + dirX*wob
		r.drawGlowRect(screen, x, y, math.Max(2.5, size*0.13*(1-0.1*float64(k))),
			mixColor(wing, dragonScale, float64(k)/6), (0.42-0.055*float64(k))*critBoost, additiveGlowBlend)
	}
}

// Nest Arbalest - the bolt that hunts the clutch. It flies like a SEEKER:
// twin hook prongs scissor ahead of the head, and a taut thread runs back
// along its flight - the line it will follow to the next target when it
// ricochets.
func (r *Renderer) drawWeaponProjectileFxDragonNest(screen *ebiten.Image, cx, cy, size, dirX, dirY, critBoost float64, id int) {
	seed := id + 611
	fc := float64(r.game.frameCount)
	hunt, bone := [3]int{178, 208, 122}, dragonBone
	nx, ny := -dirY, dirX
	// ONE connected silhouette: a harpoon head. Barbs are ATTACHED at the point
	// and sweep BACK along the shaft, so the bolt reads as a single barbed
	// object. A first pass floated scissor prongs, a leash and a ghost bolt
	// around the arrow - detached parts at projectile speed just read as
	// random blobs, however solid each one is.
	tipX, tipY := cx+dirX*size*1.9, cy+dirY*size*1.9
	flex := 0.8 + 0.2*math.Sin(fc*0.35) // the barbs work slightly, alive
	for _, side := range []float64{-1, 1} {
		// Outer barb: point -> swept back and out.
		obX := cx - dirX*size*0.5 + nx*side*size*1.05*flex
		obY := cy - dirY*size*0.5 + ny*side*size*1.05*flex
		r.fxSegment(screen, tipX, tipY, obX, obY, math.Max(2.5, size*0.19), hunt, 0.66*critBoost, additiveGlowBlend)
		r.fxSegment(screen, tipX, tipY, obX, obY, math.Max(2, size*0.09), bone, 0.85*critBoost, additiveGlowBlend)
		// Inner barb: a shorter second hook, giving the head its clutch look.
		ibX := cx - dirX*size*1.4 + nx*side*size*0.62*flex
		ibY := cy - dirY*size*1.4 + ny*side*size*0.62*flex
		r.fxSegment(screen, obX, obY, ibX, ibY, math.Max(2, size*0.12), mixColor(hunt, bone, 0.3), 0.5*critBoost, additiveGlowBlend)
	}
	// The shaft the barbs hang on, and the taut leash running back along the
	// flight path - the line it will follow to the next target.
	r.fxSegment(screen, tipX, tipY, cx-dirX*size*1.6, cy-dirY*size*1.6, math.Max(2.5, size*0.14), bone, 0.7*critBoost, additiveGlowBlend)
	r.fxSegment(screen, cx-dirX*size*1.6, cy-dirY*size*1.6, cx-dirX*size*4.8, cy-dirY*size*4.8,
		math.Max(2, size*0.1), hunt, 0.38*critBoost, additiveGlowBlend)
	r.drawGlowSprite(screen, tipX, tipY, size*0.34, bone, 0.75*critBoost, additiveGlowBlend)
	_ = seed
}
