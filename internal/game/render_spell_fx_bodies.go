package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Flying bodies for the remaining projectile spells: each reads as the thing its
// name promises (stone, swarm, ice, whip) instead of a tinted orb. Registered in
// spellFxStyleDraw via graphics.projectile_fx. Shared wake/debris primitives
// live at the top; animation runs off frameCount, per-particle constants off
// auraHash, so nothing needs state.

// fxWake sheds n puffs behind the head, colour ageing near -> far.
func (r *Renderer) fxWake(screen *ebiten.Image, id, n int, cx, cy, size, dirX float64, near, far [3]int, spread, reach, alpha float64, salt int) {
	fc := float64(r.game.frameCount)
	for k := 0; k < n; k++ {
		ph := frac(fc*0.02*(0.7+auraHash(id, k, salt, 0)*0.7) + auraHash(id, k, salt+1, 0))
		px := cx - dirX*ph*size*reach + (auraHash(id, k, salt+2, 0)-0.5)*size*spread
		py := cy + (auraHash(id, k, salt+3, 0)-0.5)*size*spread*0.7
		r.drawGlowSprite(screen, px, py, size*(0.35+0.6*ph), mixColor(near, far, ph), (1-ph)*alpha, additiveGlowBlend)
	}
}

// fxSegment draws a straight solid bar between two points - what turns a row of
// dots into a filament, a thong or a shaft.
func (r *Renderer) fxSegment(screen *ebiten.Image, x1, y1, x2, y2, thick float64, rgb [3]int, alpha float64, blend ebiten.Blend) {
	dx, dy := x2-x1, y2-y1
	length := math.Hypot(dx, dy)
	if length <= 0 {
		return
	}
	r.drawGlowRectRotated(screen, (x1+x2)/2, (y1+y2)/2, length, thick, math.Atan2(dy, dx), rgb, alpha, blend)
}

// fxTongue draws a flame tongue: a fat root narrowing to a tip, painted as
// overlapping blobs STRETCHED along the tongue so they fuse into one body.
// Keep the lean gentle (lean, not sway): a per-segment wobble curls the tongue
// into a spiral whisker instead of a flame.
func (r *Renderer) fxTongue(screen *ebiten.Image, x, y, length, width, angle, wave float64, outer, mid, hot [3]int, alpha float64) {
	dx, dy := math.Cos(angle), math.Sin(angle)
	nx, ny := -dy, dx // across the tongue
	lean := math.Sin(wave) * width * 0.5
	const steps = 8
	seg := length / steps * 2.6 // heavy overlap: neighbours must fuse, not bead
	for k := 0; k < steps; k++ {
		t := float64(k) / (steps - 1) // 0 root -> 1 tip
		w := width * (1 - 0.8*t*t)    // stays fat, then pinches at the tip
		// One smooth bend over the whole tongue, strongest at the tip.
		off := lean * t * t
		px := x + dx*length*t + nx*off
		py := y + dy*length*t + ny*off
		// SOURCE-OVER: opaque blobs merge into one silhouette. Additive ones only
		// brighten where they overlap, which is what turns a tongue into beads.
		r.drawGlowSpriteStretched(screen, px, py, seg*1.15, w*1.5, outer, alpha*0.9, ebiten.BlendSourceOver)
		r.drawGlowSpriteStretched(screen, px, py, seg, w, mid, alpha, ebiten.BlendSourceOver)
		if t < 0.45 {
			r.drawGlowSpriteStretched(screen, px, py, seg*0.8, w*0.45, hot, alpha*(1-t/0.45), ebiten.BlendSourceOver)
		}
	}
	// One soft bloom over the whole tongue.
	r.drawGlowSpriteStretched(screen, x+dx*length*0.4, y+dy*length*0.4, length*1.2, width*1.6, mid, alpha*0.3, additiveGlowBlend)
}

// fxEmbers throws sparks off a burning body: small embers streaking outward,
// stretched along their own flight and cooling white-hot -> orange -> ash. Never
// axis-aligned white squares - those read as UI pixels, not fire.
func (r *Renderer) fxEmbers(screen *ebiten.Image, id, n int, cx, cy, size, dirX float64, hot, warm, cool [3]int, salt int) {
	fc := float64(r.game.frameCount)
	for k := 0; k < n; k++ {
		life := auraHash(id, k, salt, 0)
		ph := frac(fc*0.035*(0.7+life*0.9) + life)
		a := auraHash(id, k, salt+1, 0)*2*math.Pi - 0.4 // biased upward/backward
		reach := size * (0.5 + 1.4*ph)
		px := cx + math.Cos(a)*reach - dirX*ph*size*0.8
		py := cy + math.Sin(a)*reach*0.8 - ph*size*0.5 // embers rise as they cool
		es := math.Max(1.5, size*0.06*(1-ph*0.5))      // tiny: a big bar reads as a white stick
		col := mixColor(hot, warm, math.Min(1, ph*2))
		if ph > 0.5 {
			col = mixColor(warm, cool, (ph-0.5)/0.5)
		}
		r.drawGlowSprite(screen, px, py, es*2.4, col, (1-ph)*0.5, additiveGlowBlend) // ember bloom
		r.drawGlowRectRotated(screen, px, py, es*2.2, es, a, col, (1-ph)*0.9, additiveGlowBlend)
	}
}

// fxRing draws a ring as n solid ticks - readable at any scale, unlike a soft
// glow circle. rx/ry let it flatten into a shock ring seen edge-on.
func (r *Renderer) fxRing(screen *ebiten.Image, cx, cy, rx, ry, thick, spin float64, n int, rgb [3]int, alpha float64) {
	for k := 0; k < n; k++ {
		a := spin + float64(k)*2*math.Pi/float64(n)
		r.drawGlowRectRotated(screen, cx+math.Cos(a)*rx, cy+math.Sin(a)*ry,
			thick*2.2, thick, a+math.Pi/2, rgb, alpha, additiveGlowBlend)
	}
}

// fxChunk draws one angular fragment: a dark rim, a stone body and a small lit
// facet, all turned together. The rim is what stops it reading as flat paper.
func (r *Renderer) fxChunk(screen *ebiten.Image, cx, cy, s, angle float64, body, lit [3]int, alpha float64) {
	rim := mixColor(body, [3]int{0, 0, 0}, 0.55)
	r.drawGlowRectRotated(screen, cx, cy, s*1.1, s*0.9, angle, rim, alpha, ebiten.BlendSourceOver)
	r.drawGlowRectRotated(screen, cx, cy, s*0.9, s*0.7, angle, body, alpha, ebiten.BlendSourceOver)
	r.drawGlowRectRotated(screen, cx-s*0.18, cy-s*0.18, s*0.34, s*0.22, angle+0.4, lit, alpha*0.9, ebiten.BlendSourceOver)
}

// Rock Blast - one jagged chunk of stone tumbling nose-first, chipping smaller
// splinters, with a gritty dust wake.
func (r *Renderer) drawSpellFxRock(screen *ebiten.Image, cx, cy, size, dirX float64, _ float64, _ [3]int, _ projectileFxProfile, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	stone := [3]int{92, 78, 64}
	lit := [3]int{168, 146, 116}
	dust := [3]int{120, 104, 84}

	r.fxWake(screen, id, 8, cx, cy, size, dirX, dust, [3]int{60, 52, 44}, 0.9, 3.0, 0.35, 41)
	spin := fc * 0.09
	// Main chunk: three overlapping quads at spread angles read as a jagged mass.
	for k := 0; k < 3; k++ {
		a := spin + float64(k)*2.1
		off := size * 0.18 * float64(k)
		r.fxChunk(screen, cx+math.Cos(a)*off, cy+math.Sin(a)*off*0.8, size*(0.95-0.18*float64(k)), a, stone, lit, 1)
	}
	// Splinters thrown off the tumble.
	for k := 0; k < 5; k++ {
		ph := frac(fc*0.03*(0.8+auraHash(id, k, 45, 0)*0.7) + auraHash(id, k, 46, 0))
		px := cx - dirX*ph*size*2.2 + (auraHash(id, k, 47, 0)-0.5)*size*1.2
		py := cy + ph*size*0.8*(auraHash(id, k, 48, 0)-0.2) // heavy: they fall
		r.fxChunk(screen, px, py, size*0.22*(1-ph*0.5), spin*2+float64(k), stone, lit, 1-ph)
	}
	r.drawGlowSprite(screen, cx, cy, size*1.5*critBoost, dust, 0.16, additiveGlowBlend) // grit haze
}

// Stone Blossom - a granite bud: a closed stone core inside four petal slabs
// that shiver and part slightly as it arcs, trailing pollen grit.
func (r *Renderer) drawSpellFxStoneBud(screen *ebiten.Image, cx, cy, size, dirX float64, _ float64, _ [3]int, _ projectileFxProfile, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	granite := [3]int{104, 116, 92}
	lit := [3]int{176, 196, 148}
	bloom := [3]int{206, 226, 170}

	r.fxWake(screen, id, 6, cx, cy, size, dirX, granite, [3]int{70, 78, 62}, 0.8, 2.4, 0.3, 51)
	spin := fc * 0.05
	open := 0.42 + 0.12*math.Sin(fc*0.14) // petals breathe, promising the bloom
	// Petals: elongated slabs pointing OUTWARD with gaps, so the bud reads as a
	// closing flower rather than one block.
	for k := 0; k < 5; k++ {
		a := spin + float64(k)*2*math.Pi/5
		px := cx + math.Cos(a)*size*open
		py := cy + math.Sin(a)*size*open*0.85
		rim := mixColor(granite, [3]int{0, 0, 0}, 0.5)
		r.drawGlowRectRotated(screen, px, py, size*0.8, size*0.34, a, rim, 1, ebiten.BlendSourceOver)
		r.drawGlowRectRotated(screen, px, py, size*0.66, size*0.22, a, granite, 1, ebiten.BlendSourceOver)
		r.drawGlowRectRotated(screen, px+math.Cos(a)*size*0.12, py+math.Sin(a)*size*0.1,
			size*0.24, size*0.1, a, lit, 0.9, ebiten.BlendSourceOver)
	}
	// Seed core glowing between the petals.
	r.drawGlowSprite(screen, cx, cy, size*0.75, bloom, 0.35+0.2*math.Sin(fc*0.2), additiveGlowBlend)
	r.fxChunk(screen, cx, cy, size*0.44, -spin*1.6, granite, lit, 1)
	r.drawGlowSprite(screen, cx, cy, size*1.7*critBoost, bloom, 0.12, additiveGlowBlend)
}

// Deadly Swarm - no core at all: a loose cloud of insects, each on its own
// erratic figure-eight, wings flickering, a few stragglers lagging behind.
func (r *Renderer) drawSpellFxSwarm(screen *ebiten.Image, cx, cy, size, dirX float64, _ float64, _ [3]int, _ projectileFxProfile, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	chitin := [3]int{58, 44, 18}
	stripe := [3]int{226, 186, 54}
	haze := [3]int{140, 150, 70}

	r.drawGlowSprite(screen, cx, cy, size*2.1*critBoost, haze, 0.1, additiveGlowBlend) // dust the swarm kicks up
	const bees = 16
	for k := 0; k < bees; k++ {
		// Lissajous orbit per bee: different speeds/phases keep the cloud alive.
		sp := 0.10 + auraHash(id, k, 55, 0)*0.13
		ph := auraHash(id, k, 56, 0) * 2 * math.Pi
		rx := size * (0.5 + auraHash(id, k, 57, 0)*0.95)
		ry := size * (0.35 + auraHash(id, k, 58, 0)*0.75)
		lag := auraHash(id, k, 59, 0) // stragglers trail the pack
		px := cx - dirX*lag*size*1.3 + math.Cos(fc*sp+ph)*rx
		py := cy + math.Sin(fc*sp*1.7+ph*1.3)*ry
		buzz := math.Sin(fc*0.9 + float64(k)) // wingbeat
		bs := size * 0.15
		r.drawGlowRectRotated(screen, px, py, bs*1.5, bs*0.7, buzz*0.4, chitin, 1, ebiten.BlendSourceOver)
		r.drawGlowRectRotated(screen, px, py, bs*0.6, bs*0.5, buzz*0.4, stripe, 0.9, ebiten.BlendSourceOver)
		if buzz > 0.3 { // wings catch the light on the upbeat
			r.drawGlowSprite(screen, px, py-bs*0.4, bs*1.1, [3]int{235, 235, 210}, 0.35, additiveGlowBlend)
		}
	}
}

// Ice Bolt - a frozen spike flying point-first: a faceted crystal body with a
// white-hot rime edge, shedding frost sparkle and cold vapour.
func (r *Renderer) drawSpellFxIceShard(screen *ebiten.Image, cx, cy, size, dirX, dirY float64, _ [3]int, _ projectileFxProfile, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	rime := [3]int{236, 250, 255}
	ice := [3]int{130, 200, 250}
	deep := [3]int{50, 120, 200}

	ang := math.Atan2(dirY, dirX)
	r.fxWake(screen, id, 7, cx, cy, size, dirX, ice, deep, 0.7, 2.6, 0.3, 61)
	// Body: a long facet plus two shorter ones, all along the flight line.
	r.drawGlowSpriteStretched(screen, cx, cy, size*2.4*critBoost, size*1.0, ice, 0.32, additiveGlowBlend)
	r.drawGlowRectRotated(screen, cx, cy, size*1.5, size*0.34, ang, deep, 0.95, ebiten.BlendSourceOver)
	r.drawGlowRectRotated(screen, cx, cy, size*1.3, size*0.22, ang, ice, 1, ebiten.BlendSourceOver)
	r.drawGlowRectRotated(screen, cx+dirX*size*0.3, cy+dirY*size*0.3, size*0.6, size*0.12, ang, rime, 1, ebiten.BlendSourceOver)
	// Point: two crossed facets ahead of the body, so it ends in a spike.
	pX, pY := cx+dirX*size*0.9, cy+dirY*size*0.9
	for k := 0; k < 2; k++ {
		side := 1.0 - 2*float64(k)
		r.drawGlowRectRotated(screen, pX, pY, size*0.42, size*0.11, ang+side*0.55, deep, 0.95, ebiten.BlendSourceOver)
		r.drawGlowRectRotated(screen, pX, pY, size*0.36, size*0.06, ang+side*0.55, rime, 1, ebiten.BlendSourceOver)
	}
	for k := 0; k < 2; k++ { // side facets, slightly off-axis
		side := 1.0 - 2*float64(k)
		r.drawGlowRectRotated(screen, cx-dirX*size*0.2, cy-dirY*size*0.2+side*size*0.16,
			size*0.8, size*0.1, ang+side*0.35, ice, 0.85, ebiten.BlendSourceOver)
	}
	// Frost sparkle: tiny crossed glints, turned off-axis so they read as ice
	// catching light rather than stray white pixels.
	for k := 0; k < 6; k++ {
		sa := auraHash(id, k, 65, int(fc)/4) * 2 * math.Pi
		sr := size * (0.5 + auraHash(id, k, 66, int(fc)/4)*0.9)
		gx, gy := cx+math.Cos(sa)*sr, cy+math.Sin(sa)*sr*0.8
		gs := math.Max(2, size*0.11)
		r.drawGlowRectRotated(screen, gx, gy, gs, gs*0.3, sa, rime, 0.9, additiveGlowBlend)
		r.drawGlowRectRotated(screen, gx, gy, gs*0.3, gs, sa, rime, 0.7, additiveGlowBlend)
	}
}

// Fire Bolt - the lean cousin of Fireball: a single darting flame tongue,
// stretched along the flight line with a guttering tail.
func (r *Renderer) drawSpellFxFireDart(screen *ebiten.Image, cx, cy, size, dirX float64, _ float64, _ [3]int, _ projectileFxProfile, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	heart := [3]int{255, 244, 190}
	flame := [3]int{255, 152, 44}
	deep := [3]int{196, 44, 12}

	flick := 0.9 + 0.1*math.Sin(fc*0.45)
	// The dart IS one tongue: it points BACKWARD from the nose, so the flame
	// streams behind the head like a blown candle flame.
	back := math.Atan2(0, -dirX)
	r.fxTongue(screen, cx+dirX*size*0.2, cy, size*1.7, size*0.62*flick*critBoost, back, fc*0.3, deep, flame, heart, 1)
	// Nose: an opaque hot head, so the dart has a body and not just a glow.
	r.drawGlowSprite(screen, cx+dirX*size*0.2, cy, size*0.9*flick, deep, 0.9, ebiten.BlendSourceOver)
	r.drawGlowSprite(screen, cx+dirX*size*0.25, cy, size*0.62*flick, flame, 1, ebiten.BlendSourceOver)
	r.drawGlowSprite(screen, cx+dirX*size*0.3, cy, size*0.3*flick, heart, 1, ebiten.BlendSourceOver)
	r.drawGlowSprite(screen, cx+dirX*size*0.25, cy, size*1.3*flick, flame, 0.3, additiveGlowBlend) // bloom
	// Two shorter tongues splitting off, so the tail flickers instead of tapering
	// in one straight line.
	for k := 0; k < 2; k++ {
		side := 1.0 - 2*float64(k)
		life := auraHash(id, k, 73, 0)
		lp := frac(fc*0.04 + life)
		r.fxTongue(screen, cx-dirX*size*0.5, cy+side*size*0.18, size*(0.6+0.5*(1-lp)), size*0.28*(1-lp),
			back+side*0.45, fc*0.25+life*6.28, deep, flame, heart, (1-lp)*0.8)
	}
	r.fxEmbers(screen, id, 5, cx, cy, size, dirX, [3]int{255, 205, 95}, flame, [3]int{80, 25, 8}, 71)
}

// Sparks - a cheap crackle: no body, just a burst of short electric filaments
// re-rolled every couple of frames around a small hot point.
func (r *Renderer) drawSpellFxSparks(screen *ebiten.Image, cx, cy, size, dirX float64, _ float64, _ [3]int, _ projectileFxProfile, critBoost float64, id int) {
	fc := int(r.game.frameCount)
	hot := [3]int{245, 252, 255}
	blue := [3]int{140, 200, 255}

	r.drawGlowSprite(screen, cx, cy, size*0.9*critBoost, blue, 0.45, additiveGlowBlend)
	r.drawGlowSprite(screen, cx, cy, size*0.35, hot, 0.95, additiveGlowBlend)
	// Filaments: each a kinked chain of 3 drawn LINES, re-rolled every 2 frames.
	for k := 0; k < 7; k++ {
		seed := fc / 2
		a := auraHash(id, k, 75, seed) * 2 * math.Pi
		px, py := cx, cy
		for s := 1; s <= 3; s++ {
			a += (auraHash(id, k*5+s, 76, seed) - 0.5) * 1.4 // kink per segment
			step := size * 0.42
			nx := px + math.Cos(a)*step
			ny := py + math.Sin(a)*step*0.8
			al := 0.95 - 0.25*float64(s)
			r.fxSegment(screen, px, py, nx, ny, math.Max(2, size*0.09), hot, al, additiveGlowBlend)
			r.fxSegment(screen, px, py, nx, ny, math.Max(3, size*0.2), blue, al*0.4, additiveGlowBlend)
			px, py = nx, ny
		}
	}
	// Trailing stray sparks so it still reads as travelling.
	for k := 0; k < 4; k++ {
		ph := frac(float64(fc)*0.05 + auraHash(id, k, 77, 0))
		r.drawGlowRect(screen, cx-dirX*ph*size*1.8, cy+(auraHash(id, k, 78, 0)-0.5)*size*0.7,
			math.Max(1.5, size*0.07), mixColor(hot, blue, ph), (1-ph)*0.8, additiveGlowBlend)
	}
}

// Dark Bolt - a shadow bolt: a light-swallowing core wrapped in violet rim
// light, streaming torn smoke ribbons.
func (r *Renderer) drawSpellFxShadowBolt(screen *ebiten.Image, cx, cy, size, dirX float64, _ float64, _ [3]int, _ projectileFxProfile, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	void := [3]int{18, 8, 26}
	violet := [3]int{168, 88, 224}
	pale := [3]int{224, 170, 255}

	// Ribbons: each a wavy chain of motes peeling off the core.
	for k := 0; k < 5; k++ {
		ph := auraHash(id, k, 81, 0)
		for s := 0; s < 6; s++ {
			t := float64(s) / 5
			sway := math.Sin(fc*0.12+ph*6.28+t*3.4) * size * 0.5 * t
			px := cx - dirX*t*size*2.6
			py := cy + sway + (ph-0.5)*size*0.6
			r.drawGlowSprite(screen, px, py, size*(0.55-0.3*t), mixColor(violet, void, t), (1-t)*0.45, additiveGlowBlend)
		}
	}
	// Rim light before the hole, so the silhouette is defined against ANY
	// backdrop - a black core on a dark wall would otherwise vanish.
	r.drawGlowSprite(screen, cx, cy, size*2.1*critBoost, violet, 0.55, additiveGlowBlend)
	r.drawGlowSprite(screen, cx, cy, size*1.35, mixColor(violet, pale, 0.4), 0.75, additiveGlowBlend)
	r.drawGlowSprite(screen, cx, cy, size*1.0, void, 0.95, ebiten.BlendSourceOver) // the hole itself
	// Crescent of rim light hugging the leading edge, plus crawling arc motes.
	for k := 0; k < 7; k++ {
		a := -0.9 + float64(k)*0.3
		r.drawGlowSprite(screen, cx+math.Cos(a)*size*0.56, cy+math.Sin(a)*size*0.5, size*0.28, pale, 0.5, additiveGlowBlend)
	}
	for k := 0; k < 3; k++ {
		a := fc*0.06 + float64(k)*2.09
		r.drawGlowSprite(screen, cx+math.Cos(a)*size*0.62, cy+math.Sin(a)*size*0.56, size*0.3, pale, 0.7, additiveGlowBlend)
	}
}

// Alien Dark Bolt - void tech, not sorcery: a dense black needle inside a
// spinning containment ring, with scan-glitches snapping across it.
func (r *Renderer) drawSpellFxVoidNeedle(screen *ebiten.Image, cx, cy, size, dirX, dirY float64, _ [3]int, _ projectileFxProfile, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	void := [3]int{10, 6, 18}
	acid := [3]int{150, 255, 190}
	violet := [3]int{150, 70, 200}

	ang := math.Atan2(dirY, dirX)
	r.fxWake(screen, id, 5, cx, cy, size, dirX, violet, void, 0.5, 2.2, 0.3, 85)
	r.drawGlowSprite(screen, cx, cy, size*2.0*critBoost, violet, 0.3, additiveGlowBlend)
	// Containment ring: four arc nodes spinning around the axis.
	for k := 0; k < 4; k++ {
		a := fc*0.16 + float64(k)*math.Pi/2
		px := cx + math.Cos(a)*size*0.8
		py := cy + math.Sin(a)*size*0.7
		r.drawGlowRectRotated(screen, px, py, size*0.3, size*0.1, a, acid, 0.85, additiveGlowBlend)
	}
	r.drawGlowRectRotated(screen, cx, cy, size*1.6, size*0.3, ang, void, 1, ebiten.BlendSourceOver)
	r.drawGlowRectRotated(screen, cx, cy, size*1.7, size*0.08, ang, acid, 0.9, additiveGlowBlend)
	// Glitch: a bright slab jumps across the needle every few frames.
	if int(fc)%7 < 2 {
		off := (auraHash(id, int(fc)/7, 87, 0) - 0.5) * size
		r.drawGlowRectRotated(screen, cx+off*dirX*0.4, cy+off*0.6, size*0.5, size*0.16, ang+1.2, acid, 0.7, additiveGlowBlend)
	}
}

// Bind Undead - a thrown shackle: a spinning bone-grey chain of links with a
// spectral wisp streaming behind, reading as bondage rather than damage.
func (r *Renderer) drawSpellFxShackle(screen *ebiten.Image, cx, cy, size, dirX float64, _ float64, _ [3]int, _ projectileFxProfile, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	iron := [3]int{158, 152, 170}
	bone := [3]int{234, 228, 202}
	spectral := [3]int{164, 206, 255}

	// Spectral wisp: the will that drags the chain.
	for k := 0; k < 6; k++ {
		t := float64(k) / 5
		sway := math.Sin(fc*0.14+t*4.0) * size * 0.45 * t
		r.drawGlowSprite(screen, cx-dirX*t*size*2.4, cy+sway-t*size*0.3,
			size*(0.6-0.32*t), spectral, (1-t)*0.4, additiveGlowBlend)
	}
	r.drawGlowSprite(screen, cx, cy, size*1.8*critBoost, spectral, 0.3, additiveGlowBlend)
	// Chain: links alternate orientation, the whole run tumbling.
	spin := fc * 0.11
	for k := 0; k < 4; k++ {
		t := float64(k) / 3
		a := spin + float64(k)*1.1
		px := cx - dirX*t*size*0.95 + math.Cos(a)*size*0.16
		py := cy + math.Sin(a)*size*0.3
		ls := size * (0.62 - 0.12*t)
		r.drawGlowRectRotated(screen, px, py, ls, ls*0.42, a, iron, 1, ebiten.BlendSourceOver)
		r.drawGlowRectRotated(screen, px, py, ls*0.62, ls*0.16, a, bone, 0.95, ebiten.BlendSourceOver) // link highlight
	}
}

// Charm - a beguiling bloom: soft rose petals opening around a warm heart,
// breathing to a heartbeat and shedding sweet motes.
func (r *Renderer) drawSpellFxCharm(screen *ebiten.Image, cx, cy, size, dirX float64, _ float64, _ [3]int, _ projectileFxProfile, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	rose := [3]int{255, 128, 190}
	blush := [3]int{255, 208, 232}
	deep := [3]int{196, 60, 140}

	beat := 0.85 + 0.15*math.Sin(fc*0.24) + 0.06*math.Sin(fc*0.48) // two-thump heartbeat
	r.fxWake(screen, id, 6, cx, cy, size, dirX, blush, rose, 1.0, 2.4, 0.3, 91)
	r.drawGlowSprite(screen, cx, cy, size*2.1*beat*critBoost, rose, 0.28, additiveGlowBlend)
	// Petals: six soft lobes turning slowly, opening with the beat.
	for k := 0; k < 6; k++ {
		a := fc*0.04 + float64(k)*math.Pi/3
		rad := size * 0.5 * beat
		r.drawGlowSpriteStretched(screen, cx+math.Cos(a)*rad, cy+math.Sin(a)*rad*0.85,
			size*0.72, size*0.5, mixColor(rose, blush, 0.35), 0.5, additiveGlowBlend)
	}
	// Opaque heart: additive rose stayed a pale smudge at projectile scale.
	r.drawGlowSprite(screen, cx, cy, size*0.9*beat, deep, 0.9, ebiten.BlendSourceOver)
	r.drawGlowSprite(screen, cx, cy, size*0.6*beat, rose, 1, ebiten.BlendSourceOver)
	r.drawGlowSprite(screen, cx, cy, size*0.32*beat, blush, 1, ebiten.BlendSourceOver)
	r.drawGlowSprite(screen, cx, cy, size*1.4*beat, rose, 0.3, additiveGlowBlend)
	// Sweet motes spiralling outward.
	for k := 0; k < 5; k++ {
		ph := frac(fc*0.018 + auraHash(id, k, 93, 0))
		a := auraHash(id, k, 94, 0)*2*math.Pi + ph*2.2
		rad := size * (0.6 + ph*1.1)
		r.drawGlowSprite(screen, cx+math.Cos(a)*rad, cy+math.Sin(a)*rad*0.8, size*0.16*(1-ph), blush, (1-ph)*0.8, additiveGlowBlend)
	}
}

// Mind Blast - a lance of psychic force: a sharp spike driving forward through
// its own shock rings, with thought-static fizzing along the shaft.
func (r *Renderer) drawSpellFxPsiLance(screen *ebiten.Image, cx, cy, size, dirX, dirY float64, _ [3]int, _ projectileFxProfile, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	psi := [3]int{186, 160, 255}
	hot := [3]int{242, 234, 255}
	deep := [3]int{92, 60, 180}

	ang := math.Atan2(dirY, dirX)
	// Shock rings shed backwards, flattened across the flight line.
	for k := 0; k < 4; k++ {
		ph := frac(fc*0.035 + float64(k)*0.25)
		rw := size * (0.4 + ph*1.5)
		r.drawGlowSpriteStretched(screen, cx-dirX*ph*size*1.9, cy, rw*0.5, rw*1.5, psi, (1-ph)*0.35, additiveGlowBlend)
	}
	r.drawGlowSpriteStretched(screen, cx, cy, size*2.2*critBoost, size*0.9, deep, 0.4, additiveGlowBlend)
	// Lance: stacked segments thinning toward the point, so it tapers instead of
	// reading as one flat bar.
	const seg = 5
	for k := 0; k < seg; k++ {
		t := float64(k) / (seg - 1) // 0 butt -> 1 point
		px := cx + dirX*(t-0.35)*size*1.7
		py := cy + dirY*(t-0.35)*size*1.7
		th := size * (0.3 - 0.24*t)
		r.drawGlowRectRotated(screen, px, py, size*0.5, th, ang, psi, 0.95, ebiten.BlendSourceOver)
		r.drawGlowRectRotated(screen, px, py, size*0.5, th*0.45, ang, hot, 0.9, ebiten.BlendSourceOver)
	}
	// Point: a small diamond ahead of the shaft.
	tipX, tipY := cx+dirX*size*1.15, cy+dirY*size*1.15
	r.drawGlowRectRotated(screen, tipX, tipY, size*0.3, size*0.09, ang+0.7, hot, 1, ebiten.BlendSourceOver)
	r.drawGlowRectRotated(screen, tipX, tipY, size*0.3, size*0.09, ang-0.7, hot, 1, ebiten.BlendSourceOver)
	r.drawGlowSprite(screen, tipX, tipY, size*0.36, hot, 0.9, additiveGlowBlend)
	// Thought-static: tiny ticks fizzing ON the shaft (kept short, or they read as
	// bars stuck through the lance).
	for k := 0; k < 5; k++ {
		t := auraHash(id, k, 97, int(fc)/3)
		px := cx + dirX*(t-0.5)*size*1.6
		py := cy + dirY*(t-0.5)*size*1.6
		r.drawGlowRectRotated(screen, px, py, size*0.14, size*0.05, ang+1.57, hot, 0.7, additiveGlowBlend)
	}
}

// Spirit Lash - a whip, not a bolt: a curved ghost-light thong cracking behind
// the head, thinning to a frayed tip.
func (r *Renderer) drawSpellFxLash(screen *ebiten.Image, cx, cy, size, dirX float64, _ float64, _ [3]int, _ projectileFxProfile, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	ghost := [3]int{226, 236, 255}
	spirit := [3]int{182, 158, 255}
	deep := [3]int{104, 82, 180}

	crack := math.Sin(fc * 0.28) // the whip's own snap drives the curve
	// Thong: a CONTINUOUS S-curve, drawn as joined bars narrowing backwards.
	prevX, prevY := cx, cy
	var lastX, lastY float64
	for k := 1; k <= 12; k++ {
		t := float64(k) / 12
		bend := math.Sin(t*3.1+crack*1.2) * size * 0.75 * t
		px := cx - dirX*t*size*2.8
		py := cy + bend
		w := size * (0.2 - 0.17*t) // a thong, not a ribbon: thin and strongly tapered
		r.fxSegment(screen, prevX, prevY, px, py, w*2.6, deep, (1-t)*0.22, additiveGlowBlend)
		r.fxSegment(screen, prevX, prevY, px, py, w, mixColor(ghost, spirit, t), 0.9-0.4*t, additiveGlowBlend)
		prevX, prevY = px, py
		lastX, lastY = px, py
	}
	// Frayed tip: two split strands off the end.
	for k := 0; k < 2; k++ {
		side := 1.0 - 2*float64(k)
		for s := 1; s <= 3; s++ {
			f := float64(s) / 3
			r.drawGlowSprite(screen, lastX-dirX*f*size*0.6, lastY+side*f*size*0.45,
				size*0.14*(1-f*0.5), spirit, (1-f)*0.6, additiveGlowBlend)
		}
	}
	r.drawGlowSprite(screen, cx, cy, size*1.6*critBoost, spirit, 0.35, additiveGlowBlend)
	r.drawGlowSprite(screen, cx, cy, size*0.5, ghost, 0.95, additiveGlowBlend) // lit knot at the head
}
