package game

import (
	"fmt"
	"math"

	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

// Bespoke flying-body renderers for signature spells, selected by
// graphics.projectile_fx in spells.yaml (spellFxProfile → profile.style →
// dispatch at the top of drawSpellProjectileFx). Impact FX stay school-driven.
// Animation cycles run on frameCount; per-particle constants come from
// auraHash so nothing needs state.

// spellFxStyleDraw maps graphics.projectile_fx to its bespoke renderer.
var spellFxStyleDraw = map[string]func(*Renderer, *ebiten.Image, float64, float64, float64, float64, float64, [3]int, projectileFxProfile, float64, int){
	"fireball":     (*Renderer).drawSpellFxFireball,
	"lightning":    (*Renderer).drawSpellFxLightning,
	"harm":         (*Renderer).drawSpellFxHarm,
	"psyshock":     (*Renderer).drawSpellFxPsyshock,
	"starburst":    (*Renderer).drawSpellFxStarburst,
	"disintegrate": (*Renderer).drawSpellFxDisintegrate,
	"ray_of_light": (*Renderer).drawSpellFxRayOfLight,
}

// validateProjectileFxStyles fails fast on a projectile_fx naming a style with
// no renderer — a YAML typo would silently fall back to the school default.
func validateProjectileFxStyles() {
	if config.GlobalSpells == nil {
		return
	}
	for key, def := range config.GlobalSpells.Spells {
		if def.Graphics != nil && def.Graphics.ProjectileFx != "" {
			if _, ok := spellFxStyleDraw[def.Graphics.ProjectileFx]; !ok {
				panic(fmt.Sprintf("spell %q: unknown projectile_fx style %q", key, def.Graphics.ProjectileFx))
			}
		}
	}
}

// frac returns the fractional part — the loop clock for cycling particles.
func frac(v float64) float64 { return v - math.Floor(v) }

// Fireball — a burning SPHERE built the card-ignite way: stable fire puffs
// slowly churning inside a round silhouette (three layers each: deep-red
// shell, orange body, white-hot heart), flame tongues rising off the crown on
// their own life-cycles, and a cooling puff wake behind.
func (r *Renderer) drawSpellFxFireball(screen *ebiten.Image, cx, cy, size, dirX, dirY float64, core [3]int, p projectileFxProfile, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	heart := [3]int{255, 240, 180}
	flame := [3]int{255, 150, 40}
	deep := [3]int{200, 40, 10}
	soot := [3]int{80, 25, 8}

	// Cooling wake: shed fire puffs drifting behind, orange → soot as they age.
	for k := 0; k < 7; k++ {
		ph := frac(fc*0.02*(0.7+auraHash(id, k, 61, 0)*0.6) + auraHash(id, k, 62, 0))
		px := cx - dirX*ph*size*3.4 + (auraHash(id, k, 63, 0)-0.5)*size*0.8
		py := cy - ph*ph*size*0.9 + (auraHash(id, k, 64, 0)-0.5)*size*0.4
		r.drawGlowSprite(screen, px, py, size*(0.55+0.5*ph),
			mixColor(deep, soot, ph), (1-ph)*0.5, additiveGlowBlend)
	}

	// Round silhouette first — the unifying ball the puffs churn inside.
	r.drawGlowSprite(screen, cx, cy, size*1.55, deep, 0.55, additiveGlowBlend)

	// Fire puffs: STABLE positions churning slowly around the centre (no
	// per-frame rehash), breathing radius; hotter toward the middle.
	const puffs = 12
	ballR := size * 0.62
	for k := 0; k < puffs; k++ {
		ang := auraHash(id, k, 65, 0)*2*math.Pi + fc*0.02*(1+auraHash(id, k, 66, 0))
		rad := math.Sqrt(auraHash(id, k, 67, 0)) * ballR
		rad *= 0.9 + 0.12*math.Sin(fc*0.11+float64(k))
		px := cx + math.Cos(ang)*rad
		py := cy + math.Sin(ang)*rad*0.92
		edge := rad / (ballR + 1)
		flick := 0.8 + 0.2*math.Sin(fc*0.23+float64(k)*2.1)
		r.drawGlowSprite(screen, px, py, size*0.7, deep, 0.22*flick, additiveGlowBlend)
		r.drawGlowSprite(screen, px, py, size*0.42, mixColor(flame, deep, edge), 0.5*flick*critBoost, additiveGlowBlend)
		if edge < 0.5 {
			r.drawGlowSprite(screen, px, py, size*0.24, heart, (1-edge)*0.5*flick, additiveGlowBlend)
		}
	}

	// Flame tongues off the crown, card-ignite style: each rises on its own
	// life-cycle, wobbling and cooling as it climbs.
	for k := 0; k < 7; k++ {
		life := auraHash(id, k, 68, 0)
		ph := frac(fc*0.02*(0.6+life*0.8) + life)
		rise := 1 - ph
		wob := math.Sin(fc*0.15+life*6.28+float64(k)) * size * 0.22 * ph
		px := cx + (auraHash(id, k, 69, 0)-0.5)*size*1.0 + wob
		py := cy - size*0.45 - ph*size*1.35
		bs := size * (0.2 + 0.24*rise)
		r.drawGlowSprite(screen, px, py, bs*1.7, deep, rise*0.45, additiveGlowBlend)
		r.drawGlowSprite(screen, px, py, bs, mixColor(flame, heart, rise*0.4), rise*0.8, additiveGlowBlend)
		if rise > 0.55 {
			r.drawGlowSprite(screen, px, py, bs*0.5, heart, (rise-0.55)/0.45, additiveGlowBlend)
		}
	}

	// Spat sparks.
	for k := 0; k < 4; k++ {
		sa := auraHash(id, k, 70, int(fc)) * 2 * math.Pi
		sr := size * (0.7 + auraHash(id, k, 71, int(fc))*0.8)
		r.drawGlowRect(screen, cx+math.Cos(sa)*sr, cy+math.Sin(sa)*sr*0.8,
			math.Max(2, size*0.08), heart, 0.8, additiveGlowBlend)
	}
}

// Lightning — not an orb but a crackling bolt: a jagged chain re-rolled every
// few frames, with dim forks and the previous shape lingering as an afterglow.
func (r *Renderer) drawSpellFxLightning(screen *ebiten.Image, cx, cy, size, dirX, dirY float64, core [3]int, p projectileFxProfile, critBoost float64, id int) {
	fc := int(r.game.frameCount)
	hot := [3]int{240, 250, 255}
	blue := [3]int{120, 170, 255}

	// Halo pulsing at the head.
	pulse := 0.7 + 0.3*math.Sin(float64(fc)*0.5)
	r.drawGlowSprite(screen, cx, cy, size*2.0*pulse*critBoost, blue, 0.5, additiveGlowBlend)

	span := size * 3.4
	drawBolt := func(jseed int, alpha float64) {
		const joints = 9
		var px, py float64
		for j := 0; j < joints; j++ {
			t := float64(j) / (joints - 1)
			envelope := math.Sin(math.Pi * t) // widest mid-bolt, pinned at ends
			x := cx + dirX*(size*0.9-span*t)
			y := cy + (auraHash(id, j, 71, jseed)-0.5)*size*1.5*envelope
			if j > 0 {
				for s := 1; s <= 3; s++ {
					f := float64(s) / 3
					mx, my := px+(x-px)*f, py+(y-py)*f
					r.drawGlowSprite(screen, mx, my, size*0.34, hot, alpha, additiveGlowBlend)
					r.drawGlowSprite(screen, mx, my, size*0.7, blue, alpha*0.45, additiveGlowBlend)
				}
			}
			px, py = x, y
		}
		// Two dim forks branching off random joints.
		for b := 0; b < 2; b++ {
			jf := 2 + int(auraHash(id, b, 72, jseed)*5)
			t := float64(jf) / (joints - 1)
			bx := cx + dirX*(size*0.9-span*t)
			by := cy + (auraHash(id, jf, 71, jseed)-0.5)*size*1.5*math.Sin(math.Pi*t)
			dir := 1.0
			if auraHash(id, b, 73, jseed) < 0.5 {
				dir = -1
			}
			for s := 1; s <= 3; s++ {
				bx -= dirX * size * 0.4
				by += dir * size * (0.3 + auraHash(id, b*7+s, 74, jseed)*0.3)
				r.drawGlowSprite(screen, bx, by, size*0.24, blue, alpha*(0.6-0.15*float64(s)), additiveGlowBlend)
			}
		}
	}
	drawBolt(fc/3-1, 0.25) // afterglow of the previous crackle
	drawBolt(fc/3, 0.9*critBoost)
}

// Harm — a pulsing blight: a dark membrane beating around a toxic heart,
// viscous drips sliding off it and stray life-motes spiralling in to be eaten.
func (r *Renderer) drawSpellFxHarm(screen *ebiten.Image, cx, cy, size, dirX, dirY float64, core [3]int, p projectileFxProfile, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	toxic := [3]int{150, 255, 160}
	murk := [3]int{40, 95, 45}
	beat := 0.8 + 0.25*math.Sin(fc*0.22) + 0.1*math.Sin(fc*0.44)

	// Membrane + heart beating in counterphase.
	r.drawGlowSprite(screen, cx, cy, size*1.6*beat, murk, 0.5, additiveGlowBlend)
	r.drawGlowSprite(screen, cx, cy, size*0.75*(1.7-beat), toxic, 0.85*critBoost, additiveGlowBlend)

	// Warts crawling over the membrane surface.
	for k := 0; k < 9; k++ {
		a := auraHash(id, k, 76, 0)*2*math.Pi + fc*0.03
		rad := size * 0.78 * beat
		r.drawGlowSprite(screen, cx+math.Cos(a)*rad, cy+math.Sin(a)*rad*0.85,
			size*0.2, mixColor(murk, toxic, auraHash(id, k, 77, 0)*0.5), 0.6, additiveGlowBlend)
	}

	// Viscous drips: swell at the underside, then tear off and fall.
	for k := 0; k < 5; k++ {
		u := frac(fc/45 + auraHash(id, k, 78, 0))
		dx := (auraHash(id, k, 79, 0) - 0.5) * size * 1.0
		for g := 0; g < 2; g++ {
			ug := u - float64(g)*0.07
			if ug < 0 {
				break
			}
			dy := size*0.6 + ug*ug*size*2.4
			f := 1 - 0.4*float64(g)
			r.drawGlowSprite(screen, cx+dx, cy+dy, size*0.22*(1-ug*0.5)*f,
				mixColor(toxic, murk, 0.4), (1-ug)*0.8*f, additiveGlowBlend)
		}
	}

	// Life-motes spiralling inward — the harm feeds.
	for k := 0; k < 6; k++ {
		v := frac(fc/50 + auraHash(id, k, 80, 0))
		a := auraHash(id, k, 81, 0)*2*math.Pi + v*3.2
		rad := (1.25 - v) * size * 1.7
		r.drawGlowSprite(screen, cx+math.Cos(a)*rad, cy+math.Sin(a)*rad*0.7,
			size*0.14, [3]int{210, 255, 210}, v*0.8, additiveGlowBlend)
	}
}

// Psychic Shock — a trembling mind-mote emitting flattened sonar rings, with
// three thought-orbs circling it and dragging short trails.
func (r *Renderer) drawSpellFxPsyshock(screen *ebiten.Image, cx, cy, size, dirX, dirY float64, core [3]int, p projectileFxProfile, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	lav := [3]int{190, 170, 255}
	white := [3]int{245, 240, 255}

	// Core trembles — a mind under strain.
	wx := cx + math.Sin(fc*0.31)*size*0.12
	wy := cy + math.Sin(fc*0.43+1.3)*size*0.1
	r.drawGlowSprite(screen, wx, wy, size*1.1*critBoost, lav, 0.7, additiveGlowBlend)
	r.drawGlowSprite(screen, wx, wy, size*0.5, white, 0.95, additiveGlowBlend)

	// Sonar pings: flattened rings expanding and fading, staggered thirds.
	for k := 0; k < 3; k++ {
		u := frac(fc/36 + float64(k)/3)
		rad := (0.3 + 1.9*u) * size
		alpha := (1 - u) * 0.5
		const pts = 14
		for j := 0; j < pts; j++ {
			a := float64(j) / pts * 2 * math.Pi
			r.drawGlowSprite(screen, wx+math.Cos(a)*rad, wy+math.Sin(a)*rad*0.55,
				size*0.16*(1-u*0.5), lav, alpha, additiveGlowBlend)
		}
	}

	// Thought-orbs on an elliptical orbit, each with a short fading tail.
	for k := 0; k < 3; k++ {
		base := fc*0.13 + float64(k)*2.09
		for g := 0; g < 3; g++ {
			a := base - float64(g)*0.22
			f := 1 - 0.3*float64(g)
			r.drawGlowSprite(screen, wx+math.Cos(a)*size*1.05, wy+math.Sin(a)*size*0.5,
				size*0.22*f, mixColor(white, lav, float64(g)/2), 0.8*f*f, additiveGlowBlend)
		}
	}
}

// Starburst — the projectile IS a star: a slowly spinning eight-pointed glint
// shedding twinkling stardust (the impact scatter stays the classic spray).
func (r *Renderer) drawSpellFxStarburst(screen *ebiten.Image, cx, cy, size, dirX, dirY float64, core [3]int, p projectileFxProfile, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	icy := [3]int{200, 225, 255}
	white := [3]int{255, 255, 250}
	twinkle := 0.8 + 0.2*math.Sin(fc*0.4+float64(id))

	// Stardust wake: tiny twinkling glints strewn behind the flight path.
	for k := 0; k < 10; k++ {
		t := auraHash(id, k, 85, 0)
		px := cx - dirX*t*size*3.5 + (auraHash(id, k, 86, 0)-0.5)*size*1.2
		py := cy + (auraHash(id, k, 87, 0)-0.5)*size*1.0 - t*size*0.4
		tw := math.Sin(fc*0.5 + auraHash(id, k, 88, 0)*6.28)
		r.drawGlowSprite(screen, px, py, size*0.16*(1-t*0.5), icy, (1-t)*tw*tw*0.9, additiveGlowBlend)
	}

	// The star: bright heart + 4 long arms and 4 short diagonals, all spinning.
	r.drawGlowSprite(screen, cx, cy, size*0.9*critBoost, white, twinkle, additiveGlowBlend)
	rot := fc*0.07 + float64(id)
	for k := 0; k < 8; k++ {
		a := rot + float64(k)*math.Pi/4
		armLen := size * 1.4
		if k%2 == 1 { // diagonals shorter and dimmer
			armLen *= 0.55
		}
		for s := 1; s <= 3; s++ {
			f := float64(s) / 3
			r.drawGlowSprite(screen, cx+math.Cos(a)*armLen*f, cy+math.Sin(a)*armLen*f,
				size*(0.34-0.09*float64(s)), mixColor(white, icy, f),
				twinkle*(1-0.28*float64(s))*(1-0.3*float64(k%2)), additiveGlowBlend)
		}
	}
}

// Disintegrate — a void bolt: a near-black heart in a violet rim, streaming a
// wake of matter chunks that scatter and shrink into nothing, with unmaking
// scan-flickers snapping across the core.
func (r *Renderer) drawSpellFxDisintegrate(screen *ebiten.Image, cx, cy, size, dirX, dirY float64, core [3]int, p projectileFxProfile, critBoost float64, id int) {
	fc := int(r.game.frameCount)
	violet := [3]int{190, 90, 230}
	grey := [3]int{120, 120, 130}

	// Unravelling wake: chunks of matter come apart, drift, shrink, vanish.
	for k := 0; k < 16; k++ {
		t := auraHash(id, k, 90, 0)
		jx := (auraHash(id, k, 91, fc/2) - 0.5) * size * (0.4 + t*1.6)
		jy := (auraHash(id, k, 92, fc/2) - 0.5) * size * (0.4 + t*1.4)
		px := cx - dirX*t*size*3.8 + jx
		py := cy + jy
		r.drawGlowRect(screen, px, py, math.Max(2, size*0.24*(1-t)),
			mixColor(violet, grey, t), (1-t)*0.8, additiveGlowBlend)
	}

	// Void heart: darkness with a thin violet event-horizon rim.
	r.drawGlowSprite(screen, cx, cy, size*1.15*critBoost, [3]int{12, 4, 20}, 0.8, ebiten.BlendSourceOver)
	const rim = 10
	for j := 0; j < rim; j++ {
		a := float64(j)/rim*2*math.Pi + float64(fc)*0.04
		r.drawGlowSprite(screen, cx+math.Cos(a)*size*0.55, cy+math.Sin(a)*size*0.5,
			size*0.22, violet, 0.75, additiveGlowBlend)
	}

	// Unmaking flicker: a horizontal scan-line snaps across the core.
	if (fc+id)%9 < 2 {
		for j := -3; j <= 3; j++ {
			r.drawGlowSprite(screen, cx+float64(j)*size*0.35, cy,
				size*0.2, [3]int{230, 180, 255}, 0.7-0.1*math.Abs(float64(j)), additiveGlowBlend)
		}
	}
}

// Ray of Light — lore: a plasma charge. The blaster bolt's big brother: a
// longer, wider, brighter golden energy rod with an outer radiance halo and
// star-glints crackling along the sheath.
func (r *Renderer) drawSpellFxRayOfLight(screen *ebiten.Image, cx, cy, size, dirX, dirY float64, core [3]int, p projectileFxProfile, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	hot := mixColor(core, [3]int{255, 255, 255}, 0.8)
	pulse := 0.92 + 0.08*math.Sin(fc*0.5+float64(id))

	if dirX == 0 && dirY == 0 {
		dirX = 1
	}
	length := size * 4.6 * critBoost
	n := int(length / (size * 0.28))
	if n < 8 {
		n = 8
	}
	for k := 0; k <= n; k++ {
		t := float64(k) / float64(n) // 0 head → 1 tail
		endCap := 1.0
		if t < 0.1 {
			endCap = t / 0.1
		} else if t > 0.9 {
			endCap = (1 - t) / 0.1
		}
		px := cx - dirX*length*(t-0.3)
		py := cy - dirY*length*(t-0.3)
		r.drawGlowSprite(screen, px, py, size*2.3, core, 0.2*endCap*pulse, additiveGlowBlend) // radiance halo
		r.drawGlowSprite(screen, px, py, size*1.35, core, 0.55*endCap*pulse, additiveGlowBlend)
		r.drawGlowSprite(screen, px, py, size*0.6, hot, endCap*pulse, additiveGlowBlend)
	}

	// Plasma crackle: star-glints popping along the sheath.
	for k := 0; k < 4; k++ {
		gt := frac(fc*0.05*(0.7+auraHash(id, k, 96, 0)*0.6) + auraHash(id, k, 97, 0))
		px := cx - dirX*length*(gt-0.3)
		py := cy - dirY*length*(gt-0.3)
		off := (auraHash(id, k, 98, 0) - 0.5) * size * 1.6
		tw := math.Sin(fc*0.6 + auraHash(id, k, 99, 0)*6.28)
		r.drawSparkStar(screen, px-dirY*off, py+dirX*off, size*0.3, core, hot, tw*tw*0.9, 1)
	}
}

// drawBulletTracer renders a blaster shot as a laser bolt: a uniform
// elongated energy rod along the flight line — colored sheath around a
// white-hot core, soft-capped at both ends — not a fletched arrow.
func (r *Renderer) drawBulletTracer(screen *ebiten.Image, cx, cy, size, vx, vy float64, col [3]int, critBoost float64, id int) {
	hot := mixColor(col, [3]int{255, 255, 255}, 0.75)
	pulse := 0.92 + 0.08*math.Sin(float64(r.game.frameCount)*0.6+float64(id))
	dx, dy, ok := r.projectileScreenDir(vx, vy)
	if !ok {
		// Head-on: the bolt seen down the barrel — a bright core in a halo.
		r.drawGlowSprite(screen, cx, cy, size*1.7*critBoost, col, 0.8*pulse, additiveGlowBlend)
		r.drawGlowSprite(screen, cx, cy, size*0.7, hot, pulse, additiveGlowBlend)
		return
	}
	length := size * 3.6 * critBoost
	n := int(length / (size * 0.3))
	if n < 6 {
		n = 6
	}
	for k := 0; k <= n; k++ {
		t := float64(k) / float64(n) // 0 head → 1 tail
		endCap := 1.0
		if t < 0.12 {
			endCap = t / 0.12
		} else if t > 0.88 {
			endCap = (1 - t) / 0.12
		}
		// The head sticks a quarter-length ahead of the anchor point.
		px := cx - dx*length*(t-0.25)
		py := cy - dy*length*(t-0.25)
		r.drawGlowSprite(screen, px, py, size*1.0, col, 0.4*endCap*pulse, additiveGlowBlend)
		r.drawGlowSprite(screen, px, py, size*0.45, hot, 0.9*endCap*pulse, additiveGlowBlend)
	}
}
