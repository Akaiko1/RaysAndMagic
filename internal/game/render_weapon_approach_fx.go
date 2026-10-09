package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Shellknocker sweeps its broad cutting edge through a short, weighted chop.
// The segmented wake suggests shell plates without pretending a block occurred.
func (r *Renderer) drawMeleeFxShellknocker(dst *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	p, fade, _, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	h, _ := arenaFxScale(s, screenH)
	path := func(t float64) (float64, float64) {
		a := -1.15 + 1.45*t
		return cx - h*.18 + math.Cos(a)*h*.38, cy - h*.03 + math.Sin(a)*h*.38
	}
	r.drawDissolveStroke(dst, dissolveStroke{
		path: path, width: func(t float64) float64 { return h * .042 * math.Sin(math.Pi*t) },
		color: func(t float64) [3]int { return mixColor([3]int{90, 112, 49}, s.Color, t) },
		alpha: func(float64) float64 { return .48 }, length: h * .55, seed: seedFromID(s.ID), salt: 861, blend: ebiten.BlendSourceOver,
	}, lead, p)
	// Brass-edged plate wakes are part of the swing, not on-hit particles.
	for i := 0; i < 3; i++ {
		start := .18 + float64(i)*.22
		if lead <= start {
			continue
		}
		u := math.Min(1, (lead-start)/.20)
		r.drawDissolveStroke(dst, dissolveStroke{
			path: func(t float64) (float64, float64) {
				x, y := path(start + t*.18)
				return x + h*.023*math.Sin(t*math.Pi), y
			},
			width: func(t float64) float64 { return h * .012 * math.Sin(t*math.Pi) },
			color: func(float64) [3]int { return s.Color }, alpha: func(float64) float64 { return fade * .50 },
			length: h * .12, seed: seedFromID(s.ID), salt: 862 + i, blend: additiveGlowBlend,
		}, u, p)
	}
	x, y := path(lead)
	r.drawWeaponSilhouette(dst, "chop", s.Style, x, y, tangentAt(path, lead)-math.Pi/2, h*.085, 1-smoothStep(math.Max(0, (p-.35)/.30)))
}

// One curved thrust with a split liquid wake. Steam remains exclusive to the
// real Hot Steam proc; the ordinary stab must not imply it fired every time.
func (r *Renderer) drawMeleeFxRainfang(dst *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	p, fade, _, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	h, _ := arenaFxScale(s, screenH)
	path := func(t float64) (float64, float64) {
		return cx + h*(.18-.23*t+.035*math.Sin(math.Pi*t)), cy + h*(.23-.44*t)
	}
	for _, side := range []float64{-1, 1} {
		r.drawDissolveStroke(dst, dissolveStroke{
			path:  func(t float64) (float64, float64) { x, y := path(t); return x + side*h*.018*math.Sin(t*math.Pi), y },
			width: func(t float64) float64 { return h * .012 * math.Sin(t*math.Pi) },
			color: func(t float64) [3]int { return mixColor(s.Color, [3]int{187, 246, 239}, t*.6) },
			alpha: func(float64) float64 { return .48 }, length: h * .48, seed: seedFromID(s.ID), salt: 871, blend: additiveGlowBlend,
		}, lead, p)
	}
	x, y := path(lead)
	r.drawWeaponSilhouette(dst, "stab", s.Style, x, y, tangentAt(path, lead), h*.085, 1-smoothStep(math.Max(0, (p-.35)/.30)))
	if p > .35 {
		u := (p - .35) / .65
		for i := 0; i < 4; i++ {
			t := .45 + float64(i)*.13
			x, y := path(t)
			r.drawWeaponShard(dst, x+h*.025*math.Sin(float64(i)*2.4)*u, y+h*.12*u*u, h*.005, s.Color, fade*.5, u, seedFromID(s.ID)+i*31, false)
		}
	}
}

var duskneedleWing = [3]int{144, 112, 172}
var duskneedleVein = [3]int{222, 192, 225}

// Four pointed, curved membrane lobes share the arrow's flight axis. The base
// fletched arrow is still drawn by the normal pass; this is its moth-wing wake.
func (r *Renderer) drawDuskneedleWings(dst *ebiten.Image, cx, cy, size, ax, ay, nx, ny, alpha float64, id int) {
	phase := r.weaponMaterialClock()*8 + float64(id%13)*.7
	spread := .80 + .20*math.Sin(phase)
	for _, side := range []float64{-1, 1} {
		for lobe := 0; lobe < 2; lobe++ {
			span := 1.35
			back := .25
			if lobe == 1 {
				span = .87
				back = 1.10
			}
			path := func(t float64) (float64, float64) {
				along := -.45 - back - .85*t
				cross := side * span * spread * math.Sin(t*math.Pi*.85)
				return cx + size*(ax*along+nx*cross), cy + size*(ay*along+ny*cross)
			}
			r.drawDissolveStroke(dst, dissolveStroke{
				path: path, width: func(t float64) float64 { return size * .34 * math.Sin(math.Pi*t) },
				color: func(t float64) [3]int { return mixColor(duskneedleWing, duskneedleVein, t*.55) },
				alpha: func(float64) float64 { return alpha * .64 }, length: size * 2, seed: id, salt: 881 + lobe, blend: ebiten.BlendSourceOver,
			}, 1, -1-phase)
		}
	}
	// A few drifting scales stay behind the shaft, with continuous seeded motion.
	for i := 0; i < 5; i++ {
		u := math.Mod(phase*.13+float64(i)*.2, 1)
		cross := math.Sin(float64(i)*2.4+u) * (.20 + .40*u)
		x, y := cx+size*(-ax*(1+3*u)+nx*cross), cy+size*(-ay*(1+3*u)+ny*cross)
		r.weaponFxAccent(dst, x, y, size*.07, duskneedleVein, alpha*.35*math.Sin(u*math.Pi), additiveGlowBlend)
	}
}

func (r *Renderer) drawWeaponProjectileFxDuskneedle(dst *ebiten.Image, cx, cy, size, dx, dy, alpha float64, id int) {
	nx, ny := projectilePerpendicular(dx, dy)
	r.drawDuskneedleWings(dst, cx, cy, size, dx, dy, nx, ny, alpha, id)
}
func (r *Renderer) drawWeaponProjectileFxDuskneedleHeadOn(dst *ebiten.Image, cx, cy, size, alpha float64, id int) {
	r.drawDuskneedleWings(dst, cx, cy, size, 0, -.32, 1, 0, alpha, id)
}
func (r *Renderer) drawWeaponProjectileFxDuskneedleIncoming(dst *ebiten.Image, cx, cy, size, alpha float64, id int) {
	// The approaching head retains a clear center, with foreshortened wings
	// swept behind it instead of showing the outgoing fletching face.
	r.drawDuskneedleWings(dst, cx, cy, size*.75, 0, .32, 1, 0, alpha, id)
}
