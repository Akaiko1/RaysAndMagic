package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"math"
)

var solsticeFire = [3]int{255, 117, 40}
var solsticeWater = [3]int{57, 193, 226}
var solsticeEarth = [3]int{99, 197, 124}
var solsticeAir = [3]int{188, 224, 255}

// A bronze lance point carries a turbulent flame sheath and sheds hot exhaust.
func (r *Renderer) drawMeleeFxSolsticeThermal(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	p, fade, _, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	h, _ := arenaFxScale(s, screenH)
	path := func(t float64) (float64, float64) { return cx + h*(.17-.24*t), cy + h*(.30-.55*t) }
	r.drawDissolveStroke(screen, dissolveStroke{path: path, width: func(t float64) float64 { return h * (.026 - .016*t) }, color: func(t float64) [3]int { return mixColor(solsticeFire, [3]int{255, 249, 201}, t) }, alpha: func(float64) float64 { return .95 }, length: h * .6, seed: seedFromID(s.ID), salt: 701, blend: additiveGlowBlend}, lead, p)
	// One coherent flame sheath follows the bronze point, with a ragged wake.
	tipX, tipY := path(lead)
	r.drawSpellMaterialFade(screen, tipX, tipY, h*.065, -.4, -.9165, solsticeFire, 1, seedFromID(s.ID), spellFireDart, fade)
	if p < .65 {
		r.drawWeaponHead(screen, "lunge", tipX, tipY, -1.98, h*.036, clockBrass, fade)
	}

	if p > .30 {
		u := (p - .30) / .7
		x, y := path(1)
		r.drawImpactCloud(screen, x, y, h*.19, h*.14, u, solsticeFire, fade*.85, seedFromID(s.ID), false)
	}
}

// A jade and bronze hammer falls through a heavy tapered wake. Its impact
// sends a broad dusty pressure front carrying jade and brass chips.
func (r *Renderer) drawMeleeFxSolsticeAnchor(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	p, fade, _, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	h, _ := arenaFxScale(s, screenH)
	y := cy - h*.28 + h*.44*lead*lead
	r.drawDissolveStroke(screen, dissolveStroke{
		path:  func(t float64) (float64, float64) { return cx + h*.025*math.Sin(t*math.Pi), cy - h*.28 + h*.44*t*t },
		width: func(t float64) float64 { return h * (.018 + .048*t) },
		color: func(t float64) [3]int { return mixColor(clockCopper, solsticeEarth, t) },
		alpha: func(t float64) float64 { return .25 + .45*t }, length: h * .46, seed: seedFromID(s.ID), salt: 715, blend: ebiten.BlendSourceOver,
	}, lead, p)
	r.drawHammerHead(screen, cx, y, h*.085, math.Pi-.35+lead*.5, mixColor(solsticeEarth, [3]int{36, 147, 116}, .6), fade)

	if p > .28 {
		u := (p - .28) / .72
		by := cy + h*.16
		r.drawImpactCloud(screen, cx, by, h*.31, h*.15, u, mixColor(solsticeEarth, clockBrass, .5), fade*.8, seedFromID(s.ID), true)
		for k := 0; k < 10; k++ {
			seed := seedFromID(s.ID) + k*37
			a := -math.Pi/2 + (auraHash(seed, k, 718, 0)-.5)*2.6
			speed := h * (.1 + .14*auraHash(seed, k, 719, 0))
			r.drawWeaponShard(screen, cx+math.Cos(a)*speed*u, by+math.Sin(a)*speed*u+h*.15*u*u, h*.014,
				mixColor(solsticeEarth, clockBrass, auraHash(seed, k, 720, 0)), math.Sqrt(fade)*.8, u, seed, false)
		}
	}
}

// The vector blade opens an oblique cut, then its broad split wake curls
// outward with the shove. Three bright facets retain its stored-strike motif.
func (r *Renderer) drawMeleeFxSolsticeTransfer(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	p, fade, _, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	h, _ := arenaFxScale(s, screenH)
	seed := seedFromID(s.ID)
	path := func(t float64) (float64, float64) { return cx + h*(t-.5)*.52, cy + h*(.5-t)*.28 }
	r.drawDissolveStroke(screen, dissolveStroke{
		path: path, width: func(t float64) float64 { return h * (.014 + .02*math.Sin(math.Pi*t)) },
		color: func(t float64) [3]int { return mixColor(solsticeAir, [3]int{238, 253, 255}, t*.6) },
		alpha: func(float64) float64 { return .9 }, length: h * .6, seed: seed, salt: 731, blend: additiveGlowBlend,
	}, lead, p)
	for _, side := range []float64{-1, 1} {
		r.drawDissolveStroke(screen, dissolveStroke{
			path: func(t float64) (float64, float64) {
				x, y := path(t)
				curl := side * h * (.028 + .14*p) * math.Sin(t*math.Pi)
				return x + curl*.45, y + curl
			},
			width: func(t float64) float64 { return h * .026 * math.Sin(t*math.Pi) },
			color: func(t float64) [3]int { return mixColor(solsticeAir, [3]int{95, 139, 165}, t) },
			alpha: func(t float64) float64 { return fade * .55 * math.Sin(t*math.Pi) }, length: h * .65, seed: seed, salt: 736, blend: additiveGlowBlend,
		}, lead, p)
	}
	if p > .3 {
		u := (p - .3) / .7
		for k := 0; k < 3; k++ {
			angle := -.48 + float64(k-1)*.42
			r.drawWeaponShard(screen, cx+math.Cos(angle)*h*.26*u, cy+math.Sin(angle)*h*.26*u, h*.025, solsticeAir, fade, u, seed+k*37, false)
		}
	}
}

// Water travels as two counterflowing ribbons enclosing a clear reservoir.
// A side projection reveals the wake; head-on shows its double spiral mouth.
func (r *Renderer) drawWeaponProjectileFxSolsticeFlow(screen *ebiten.Image, cx, cy, size, dx, dy, alpha float64, id int) {
	phase := r.weaponMaterialClock()*4 + float64(id%11)
	nx, ny := projectilePerpendicular(dx, dy)
	for strand := 0; strand < 2; strand++ {
		offset := phase + float64(strand)*math.Pi
		r.drawDissolveStroke(screen, dissolveStroke{
			path: func(t float64) (float64, float64) {
				wave := math.Sin(t*math.Pi*2+offset) * size * (.6 + .35*t)
				return cx - dx*size*t*3 + nx*wave, cy - dy*size*t*3 + ny*wave
			},
			width:  func(t float64) float64 { return math.Max(1.5, size*(.24-.09*t)) },
			color:  func(t float64) [3]int { return mixColor(solsticeWater, [3]int{220, 255, 246}, .2+.4*t) },
			alpha:  func(t float64) float64 { return alpha * (.9 - .55*t) },
			length: size * 5, seed: id, salt: 740 + strand, blend: additiveGlowBlend,
		}, 1, -1-phase)
	}
}

func (r *Renderer) drawWeaponProjectileFxSolsticeFlowHeadOn(screen *ebiten.Image, cx, cy, size, alpha float64, id int) {
	phase := r.weaponMaterialClock()*4 + float64(id%11)
	for strand := 0; strand < 2; strand++ {
		offset := phase + float64(strand)*math.Pi
		r.drawDissolveStroke(screen, dissolveStroke{
			path: func(t float64) (float64, float64) {
				a := offset + t*math.Pi*1.4
				rad := size * (.35 + .85*t)
				return cx + math.Cos(a)*rad, cy + math.Sin(a)*rad
			},
			width:  func(t float64) float64 { return math.Max(1.5, size*(.24-.06*t)) },
			color:  func(t float64) [3]int { return mixColor(solsticeWater, [3]int{220, 255, 246}, t) },
			alpha:  func(t float64) float64 { return alpha * (.55 + .4*t) },
			length: size * 6, seed: id, salt: 745 + strand, blend: additiveGlowBlend,
		}, 1, -1-phase)
	}
}
