package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Pursuer arsenal flourishes - the ocean strangers' field kit. The set's
// shared signature is MACHINE PRECISION: dead-straight geometry, cold
// cyan-on-graphite, oscillation instead of organic wobble. Nothing here
// flutters; it resonates.

var (
	techGraphite = [3]int{74, 86, 96}    // alloy body, barely lit
	techCyan     = [3]int{118, 232, 232} // energy sheath
	techWhite    = [3]int{232, 255, 255} // the core line
)

// Vibroblade - the humming edge. A dead-straight diagonal cut whose core is
// TRIPLED by high-frequency vibration blur; resonance ticks slide along the
// edge while it swings. The finish is the melt-slot (the 35% armor pierce):
// a clean straight seam left glowing where plate stopped mattering, with
// spark jets venting perpendicular from both sides.
func (r *Renderer) drawMeleeFxTechVibro(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h, w := arenaFxScale(s, screenH)
	fc := float64(r.game.frameCount)
	reach := h * 0.46
	// Diagonal cut, upper-left to lower-right - a machine does not arc.
	x0, y0 := cx-reach*0.62, cy-reach*0.58
	x1, y1 := cx+reach*0.55, cy+reach*0.28
	dirX, dirY := x1-x0, y1-y0
	dl := math.Hypot(dirX, dirY)
	nx, ny := -dirY/dl, dirX/dl // perpendicular, for vibration offsets
	ld := 1 - (1-lead)*(1-lead)
	line := func(off float64) func(t float64) (float64, float64) {
		return func(t float64) (float64, float64) {
			return x0 + dirX*t + nx*off, y0 + dirY*t + ny*off
		}
	}

	// Graphite undercoat: the blade itself, dark and exact.
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   line(0),
		width:  func(t float64) float64 { return 20 * w },
		color:  func(t float64) [3]int { return techGraphite },
		alpha:  func(t float64) float64 { return 0.85 },
		length: dl, seed: seed, salt: 630, blend: ebiten.BlendSourceOver,
	}, ld, progress)
	// The hum: the energy core drawn THREE times with an oscillating
	// perpendicular offset - vibration rendered as blur bands, not wobble.
	for i, gain := range []float64{1, 0.55, 0.55} {
		off := 0.0
		if i > 0 {
			sign := float64(2*i - 3) // -1, +1
			off = sign * (2.6 + 1.8*math.Abs(math.Sin(fc*0.9+float64(i)))) * w
		}
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   line(off),
			width:  func(t float64) float64 { return (7 - 2*t) * w },
			color:  func(t float64) [3]int { return mixColor(techCyan, techWhite, t) },
			alpha:  func(t float64) float64 { return (0.55 + 0.4*t) * gain },
			length: dl, seed: seed, salt: 632 + i, blend: additiveGlowBlend,
		}, ld, progress)
	}
	// Resonance ticks: short perpendicular marks sliding along the lit edge.
	const ticks = 6
	for i := 0; i < ticks; i++ {
		tt := math.Mod(float64(i)/ticks+fc*0.02, 1.0)
		if tt > ld {
			continue
		}
		px, py := x0+dirX*tt, y0+dirY*tt
		tl := h * 0.018 * w
		r.drawGlowRect(screen, px+nx*tl, py+ny*tl, math.Max(2, h*0.006*w), techCyan, fade*0.7, additiveGlowBlend)
		r.drawGlowRect(screen, px-nx*tl, py-ny*tl, math.Max(2, h*0.006*w), techCyan, fade*0.7, additiveGlowBlend)
	}

	if sweepT < 1 {
		tx, ty := x0+dirX*ld, y0+dirY*ld
		r.drawGlowSprite(screen, tx, ty, h*0.032*w, techWhite, fade, additiveGlowBlend)
		return
	}

	// Melt-slot: the seam stays, white cooling to cyan, while spark jets vent
	// perpendicular from both faces of the cut - pressure escaping the plate.
	u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)
	seam := func(t float64) (float64, float64) {
		return x0 + dirX*(0.35+0.5*t), y0 + dirY*(0.35+0.5*t)
	}
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   seam,
		width:  func(t float64) float64 { return (5.5 - 2*t) * w * (1 - 0.4*u) },
		color:  func(t float64) [3]int { return mixColor(techWhite, techCyan, math.Min(1, u*1.5)) },
		alpha:  func(t float64) float64 { return 0.85 * (1 - 0.45*u) },
		length: dl * 0.5, seed: seed, salt: 636, blend: additiveGlowBlend,
	}, 1, 0.25)
	const jets = 8
	for k := 0; k < jets; k++ {
		side := 1.0
		if k%2 == 0 {
			side = -1
		}
		born := auraHash(seed, k, 638, 0) * 0.4
		ju := (u - born) / (1 - born)
		if ju <= 0 {
			continue
		}
		st := 0.4 + 0.55*auraHash(seed, k, 639, 0)
		jx, jy := seam(st)
		for g := 0; g < 3; g++ {
			gu := ju - float64(g)*0.05
			if gu < 0 {
				break
			}
			f := 1 - 0.3*float64(g)
			r.drawGlowRect(screen, jx+nx*side*gu*h*0.14, jy+ny*side*gu*h*0.14,
				math.Max(2, h*0.008*f), mixColor(techWhite, techCyan, gu), fade*(1-gu)*f*f, additiveGlowBlend)
		}
	}
}

// ============================= PURSUER RANGED ==============================
// Overlays on top of the normal bolt/arrow (weaponProjectileFxStyleDraw).

// Suppressor - drum-fed slug thrower. ONE coherent shape, not a scatter of
// parts: the slug drags a RIFLING CORKSCREW, a single continuous helix wound
// around its flight line that spins with the frame clock. A first pass hung
// separate echo slugs and tumbling casings around the bolt, and a handful of
// detached elements moving at projectile speed just reads as random blobs -
// the silhouette has to be one connected thing.
func (r *Renderer) drawWeaponProjectileFxTechSuppressor(screen *ebiten.Image, cx, cy, size, dirX, dirY, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	nx, ny := -dirY, dirX
	const turns, steps = 3.4, 26
	reach := size * 3.4
	spin := fc * 0.5 // the whole helix rotates: rifling, seen from the side
	var px, py float64
	for k := 0; k <= steps; k++ {
		t := float64(k) / steps // 0 at the slug, 1 at the tail
		ang := spin + t*turns*2*math.Pi
		// Radius swells just behind the slug and tapers into the tail, so the
		// corkscrew reads as gas twisting off the bore rather than a spring.
		rad := size * 0.62 * math.Sin(math.Pi*math.Min(1, t*1.15)) * (1 - 0.25*t)
		// Flat wave, deliberately: leaning the loops along the flight axis to
		// fake a 3D helix made them self-intersect and read as a lumpy hook.
		// A tight in-plane oscillation reads as rifling twist and stays clean.
		x := cx - dirX*reach*t + nx*math.Cos(ang)*rad
		y := cy - dirY*reach*t + ny*math.Cos(ang)*rad
		if k > 0 {
			a := (0.62 - 0.42*t) * critBoost
			r.fxSegment(screen, px, py, x, y, math.Max(2.5, size*0.2*(1-0.4*t)),
				mixColor(techWhite, techCyan, 0.35+0.5*t), a, additiveGlowBlend)
		}
		px, py = x, y
	}
	// Bore glow at the slug itself, tying the helix to its head.
	r.drawGlowSprite(screen, cx, cy, size*0.62, techCyan, 0.5*critBoost, additiveGlowBlend)
	r.drawGlowSprite(screen, cx, cy, size*0.3, techWhite, 0.85*critBoost, additiveGlowBlend)
	_ = id
}

// Longlance - the scoped rifle that ends discussions two streets away. The
// signature is REACH: a hairline lance of light stretches far ahead of the
// bolt (the shot outruns its own report) with a scope reticle pulsing at the
// head. Thin and exact - nothing here blooms.
func (r *Renderer) drawWeaponProjectileFxTechLonglance(screen *ebiten.Image, cx, cy, size, dirX, dirY, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	nx, ny := -dirY, dirX
	// The lance: ONE long solid bar reaching far ahead, laid down in three
	// overlapping runs so it dims with distance without breaking into dots.
	for i, run := range []struct{ from, to, thick, alpha float64 }{
		{0, 3.5, 0.16, 0.55}, {3.0, 6.5, 0.11, 0.34}, {6.0, 9.5, 0.07, 0.2},
	} {
		r.fxSegment(screen,
			cx+dirX*size*run.from, cy+dirY*size*run.from,
			cx+dirX*size*run.to, cy+dirY*size*run.to,
			math.Max(2, size*run.thick), mixColor(techWhite, techCyan, 0.3+0.25*float64(i)),
			run.alpha*critBoost, additiveGlowBlend)
	}
	// A shorter, brighter tail behind: where it has already been.
	r.fxSegment(screen, cx, cy, cx-dirX*size*2.6, cy-dirY*size*2.6,
		math.Max(2, size*0.13), techCyan, 0.45*critBoost, additiveGlowBlend)
	// Scope reticle at the head: four ticks off the axis, pulsing.
	pulse := 0.6 + 0.4*math.Sin(fc*0.5)
	rr := size * (1.0 + 0.25*pulse)
	for _, s := range []float64{-1, 1} {
		r.drawGlowRect(screen, cx+nx*s*rr, cy+ny*s*rr, math.Max(2, size*0.09), techCyan, 0.55*pulse*critBoost, additiveGlowBlend)
		r.drawGlowRect(screen, cx+dirX*s*rr, cy+dirY*s*rr, math.Max(2, size*0.09), techCyan, 0.4*pulse*critBoost, additiveGlowBlend)
	}
	r.drawGlowSprite(screen, cx, cy, size*0.3, techWhite, 0.5*critBoost, additiveGlowBlend)
}

// Tidehunter Compound Bow - cam-drawn alloy from another sky. The signature is
// STORED TENSION RELEASING: two cam rings counter-rotate around the shaft and
// a wound string-trace behind the arrow visibly unwinds from a zigzag into a
// straight line as it flies.
func (r *Renderer) drawWeaponProjectileFxTechCompound(screen *ebiten.Image, cx, cy, size, dirX, dirY, critBoost float64, id int) {
	seed := id + 671
	fc := float64(r.game.frameCount)
	alloy := [3]int{168, 196, 214}
	nx, ny := -dirY, dirX
	// Counter-rotating cam rings, flattened along the flight axis.
	for ring := 0; ring < 2; ring++ {
		spin := fc * 0.22 * (1 - 2*float64(ring))
		rad := size * (0.85 + 0.4*float64(ring))
		// Chord bars around the ring: consecutive points JOINED, so the cam
		// reads as a wheel rim instead of a scatter of dots.
		const seg = 8
		var px, py float64
		for i := 0; i <= seg; i++ {
			a := spin + 2*math.Pi*float64(i)/seg
			ox, oy := math.Cos(a)*rad, math.Sin(a)*rad*0.55
			x := cx + nx*ox + dirX*oy
			y := cy + ny*ox + dirY*oy
			if i > 0 {
				r.fxSegment(screen, px, py, x, y, math.Max(2, size*0.12),
					mixColor(alloy, techWhite, float64(ring)*0.4), (0.5-0.12*float64(ring))*critBoost, additiveGlowBlend)
			}
			px, py = x, y
		}
	}
	// The unwinding string: a zigzag behind the arrow whose amplitude decays
	// to zero, so the trace straightens the further back you look.
	var px, py float64
	for k := 0; k <= 9; k++ {
		u := float64(k) / 9
		amp := size * 0.9 * (1 - u) * math.Sin(u*7+fc*0.3)
		x := cx - dirX*size*3.8*u + nx*amp
		y := cy - dirY*size*3.8*u + ny*amp
		if k > 0 {
			r.fxSegment(screen, px, py, x, y, math.Max(2, size*0.11),
				mixColor(alloy, techCyan, u*0.5), (0.5-0.045*float64(k))*critBoost, additiveGlowBlend)
		}
		px, py = x, y
	}
	_ = seed
}
