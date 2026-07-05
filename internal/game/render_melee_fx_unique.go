package game

import (
	"fmt"
	"math"

	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

// Bespoke swing flourishes for legendary melee weapons, selected by
// graphics.slash_fx in weapons.yaml (SlashEffect.Style). Each stroke is a
// solid pixel-ribbon that dissolves in draw order (drawDissolveStroke) plus
// deterministic particles in the weapon's own element and silhouette; weapons
// without a style keep the category swing in drawMeleeParticles.

// meleeFxStyledLingerFrames extends the lifetime of styled swings so their
// dissolve tails (blood droplets, debris, wisps) have room to play out.
const meleeFxStyledLingerFrames = 44

// meleeFxStyleDraw maps graphics.slash_fx to its bespoke renderer
// (legendaries here, rares + naginata in render_melee_fx_rare.go).
var meleeFxStyleDraw = map[string]func(*Renderer, *ebiten.Image, SlashEffect, float64, float64, float64){
	"muramasa":      (*Renderer).drawMeleeFxMuramasa,
	"tonbogiri":     (*Renderer).drawMeleeFxTonbogiri,
	"kage_kunai":    (*Renderer).drawMeleeFxKageKunai,
	"idol_breaker":  (*Renderer).drawMeleeFxIdolBreaker,
	"silver_sword":  (*Renderer).drawMeleeFxSilverSword,
	"gold_sword":    (*Renderer).drawMeleeFxGoldSword,
	"agility_katar": (*Renderer).drawMeleeFxAgilityKatar,
	"gorehorn":      (*Renderer).drawMeleeFxGorehorn,
	"serpent_fang":  (*Renderer).drawMeleeFxSerpentFang,
	"naginata":      (*Renderer).drawMeleeFxNaginata,
}

// validateSlashFxStyles fails fast on a slash_fx naming a style with no
// renderer - a YAML typo would otherwise silently fall back to the stock swing.
func validateSlashFxStyles() {
	if config.GlobalWeapons == nil {
		return
	}
	for key, def := range config.GlobalWeapons.Weapons {
		if def.Graphics != nil && def.Graphics.SlashFx != "" {
			if _, ok := meleeFxStyleDraw[def.Graphics.SlashFx]; !ok {
				panic(fmt.Sprintf("weapon %q: unknown slash_fx style %q", key, def.Graphics.SlashFx))
			}
		}
	}
}

// Dissolve-stroke tuning: a stroke section painted at path parameter t was
// born at t-meleeSweepFrac and flakes away at a birth-ordered, per-section-
// jittered die time - the start of the stroke dissolves first, the leading
// edge last.
const (
	dissolveStepPx   = 3.5  // cross-section spacing = dissolve bite size
	dissolveDieStart = 0.42 // progress when the first-painted sections start dying
	dissolveDieEnd   = 0.98 // progress when the last-painted sections finish
	dissolveJitter   = 0.14 // per-section die-time spread (ragged, uneven fade)
)

// dissolveStroke describes one solid ribbon: a path with per-parameter
// width/color/alpha, revealed up to `lead` and dissolved by `progress`.
type dissolveStroke struct {
	path   func(t float64) (float64, float64)
	width  func(t float64) float64 // ribbon width in px
	color  func(t float64) [3]int
	alpha  func(t float64) float64
	length float64 // approximate on-screen length, sets sample density
	seed   int
	salt   int
	blend  ebiten.Blend
}

// drawDissolveStroke renders the stroke as ONE smooth triangle-strip ribbon
// textured with the soft-glow cross-section (the same falloff drawGlowSprite
// uses), so the line reads as a continuous streak rather than stacked shapes.
// Dissolve runs per cross-section: a dying section fades out and pinches
// thin, eating the line in ragged bites from the oldest end.
func (r *Renderer) drawDissolveStroke(screen *ebiten.Image, st dissolveStroke, lead, progress float64) {
	if lead <= 0 {
		return
	}
	n := int(st.length / dissolveStepPx)
	if n < 12 {
		n = 12
	} else if n > 260 {
		n = 260
	}
	// Grid sections are stable across frames (stable per-section jitter); one
	// extra section pins the strip's end exactly to the blade edge.
	ts := make([]float64, 0, n+2)
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		if t >= lead {
			break
		}
		ts = append(ts, t)
	}
	ts = append(ts, lead)

	src := r.ensureSoftGlow()
	srcMid := float32(softGlowSize) / 2
	step := 0.5 / float64(n)
	verts := make([]ebiten.Vertex, 0, len(ts)*2)
	for i, t := range ts {
		x, y := st.path(t)
		tb := math.Min(t, 1-step)
		x1, y1 := st.path(tb)
		x2, y2 := st.path(tb + step)
		dx, dy := x2-x1, y2-y1
		d := math.Hypot(dx, dy)
		if d == 0 {
			d = 1
		}
		nx, ny := -dy/d, dx/d

		baseA := st.alpha(t)
		die := dissolveDieStart + (dissolveDieEnd-dissolveDieStart)*t +
			(auraHash(st.seed, i, st.salt, 0)-0.5)*dissolveJitter
		if die > 0.985 {
			die = 0.985 // always fully dissolved before the effect expires
		}
		a := baseA
		if progress >= die {
			a = 0
		} else if die-progress < 0.08 {
			a *= (die - progress) / 0.08
		}
		pinch := 0.0
		if baseA > 0 {
			pinch = a / baseA
		}
		// half-width; x1.8 offsets the soft cross-section falloff
		w := st.width(t) * 1.8 * (0.35 + 0.65*pinch) / 2
		col := st.color(t)
		cr, cg, cb, ca := float32(col[0])/255, float32(col[1])/255, float32(col[2])/255, float32(a)
		verts = append(verts,
			ebiten.Vertex{DstX: float32(x + nx*w), DstY: float32(y + ny*w), SrcX: 0, SrcY: srcMid, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
			ebiten.Vertex{DstX: float32(x - nx*w), DstY: float32(y - ny*w), SrcX: float32(softGlowSize), SrcY: srcMid, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
		)
	}
	if len(verts) < 4 {
		return
	}
	idx := make([]uint16, 0, (len(ts)-1)*6)
	for i := 1; i < len(ts); i++ {
		a0 := uint16(2*i - 2)
		idx = append(idx, a0, a0+1, a0+2, a0+1, a0+3, a0+2)
	}
	opts := &ebiten.DrawTrianglesOptions{Blend: st.blend, Filter: ebiten.FilterLinear}
	screen.DrawTriangles(verts, idx, src, opts)
}

// drawSparkStar draws a 4-point twinkle: a bright core with four tapering
// arms; wingElong stretches the horizontal pair (beating-wing look).
func (r *Renderer) drawSparkStar(screen *ebiten.Image, x, y, size float64, col, core [3]int, alpha, wingElong float64) {
	r.drawGlowSprite(screen, x, y, size, core, alpha, additiveGlowBlend)
	arm := size * 0.9
	for i := 0; i < 4; i++ {
		dx, dy := 0.0, 0.0
		l := arm
		if i < 2 { // horizontal pair = wings
			dx = 1.0 - 2*float64(i)
			l *= wingElong
		} else {
			dy = 1.0 - 2*float64(i-2)
			l *= 0.6
		}
		for s := 1; s <= 2; s++ {
			f := float64(s) / 2
			r.drawGlowSprite(screen, x+dx*l*f, y+dy*l*f, size*(0.7-0.25*f), col, alpha*(1-0.4*f), additiveGlowBlend)
		}
	}
}

// Muramasa, the Thirsting Edge - a single razor-flat iaijutsu cut: a solid
// white-hot line over a crimson echo, dissolving from hilt to tip while blood
// teardrops fall and a crimson mist breathes off the wake.
func (r *Renderer) drawMeleeFxMuramasa(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h := screenH * meleeSizeScale
	blood := [3]int{185, 20, 32}
	steel := [3]int{255, 244, 238}
	droplets := 18
	widthScale := 1.0
	if s.Crit {
		h *= 1.25
		widthScale = 1.3
		droplets = 30
	}

	// Wider and flatter than the stock sword crescent: one horizontal draw-cut.
	reach := h * 0.22
	pivotX, pivotY := cx, cy+reach*1.35
	R := reach * 2.1
	thetaStart, thetaEnd := -math.Pi/2-0.85, -math.Pi/2+0.85
	thick := h * 0.04
	arcAt := func(t, radius float64) (float64, float64) {
		theta := thetaStart + (thetaEnd-thetaStart)*t
		return pivotX + math.Cos(theta)*radius, pivotY + math.Sin(theta)*radius
	}

	r.drawDissolveStroke(screen, dissolveStroke{
		path:   func(t float64) (float64, float64) { return arcAt(t, R) },
		width:  func(t float64) float64 { return (4 + 9*math.Sin(math.Pi*t)) * widthScale },
		color:  func(t float64) [3]int { return mixColor(blood, steel, 0.25+0.75*t) },
		alpha:  func(t float64) float64 { return 0.55 + 0.45*t },
		length: R * (thetaEnd - thetaStart),
		seed:   seed, salt: 1, blend: additiveGlowBlend,
	}, lead, progress)
	// cursed echo: a thinner crimson line trailing just inside the razor cut
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   func(t float64) (float64, float64) { return arcAt(t, R-thick*0.45) },
		width:  func(t float64) float64 { return (3 + 4*math.Sin(math.Pi*t)) * widthScale },
		color:  func(t float64) [3]int { return blood },
		alpha:  func(t float64) float64 { return 0.4 * t },
		length: R * (thetaEnd - thetaStart),
		seed:   seed, salt: 2, blend: additiveGlowBlend,
	}, lead, progress)
	if sweepT < 1 {
		tx, ty := arcAt(lead, R)
		r.drawGlowSprite(screen, tx, ty, thick*2.2, steel, fade, additiveGlowBlend)
	}

	// The thirsting edge: blood beads form where the blade passed and fall as
	// teardrops - a head with a stretched tail up the fall path - while a low
	// crimson mist swells off the wake.
	dropPos := func(k int, u float64) (float64, float64) {
		tb := auraHash(seed, k, 21, 0) // arc parameter where this bead forms
		bx, by := arcAt(tb, R)
		bx += (auraHash(seed, k, 22, 0) - 0.5) * thick * 3
		by += u*u*h*0.34 + u*thick*2
		return bx, by
	}
	for k := 0; k < droplets; k++ {
		born := auraHash(seed, k, 21, 0) * meleeSweepFrac
		if progress <= born {
			continue
		}
		u := (progress - born) / (1 - born)
		c := blood
		if auraHash(seed, k, 23, 0) > 0.7 {
			c = [3]int{235, 60, 60}
		}
		for g := 0; g < 3; g++ {
			ug := u - float64(g)*0.055
			if ug < 0 {
				break
			}
			bx, by := dropPos(k, ug)
			f := 1 - 0.32*float64(g)
			r.drawGlowSprite(screen, bx, by, math.Max(2, thick*(0.5-0.25*u)*f), c, fade*(1-u)*0.95*f*f, additiveGlowBlend)
		}
	}
	const mist = 9
	for k := 0; k < mist; k++ {
		tb := auraHash(seed, k, 24, 0)
		born := tb * meleeSweepFrac
		if progress <= born {
			continue
		}
		u := (progress - born) / (1 - born)
		mx, my := arcAt(tb, R)
		mx += (auraHash(seed, k, 25, 0) - 0.5) * thick * 4
		my -= u * h * 0.05
		r.drawGlowSprite(screen, mx, my, thick*(1.2+1.8*u), [3]int{120, 10, 22}, fade*(1-u)*0.3, additiveGlowBlend)
	}
}

// Tonbogiri, the Dragonfly Spear - an extra-long lean thrust drawn as a solid
// iridescent green<->cyan line, mirrored wing-sparks fluttering off the shaft,
// and a brief cross-flash at full extension (the dragonfly, cut in two).
func (r *Renderer) drawMeleeFxTonbogiri(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, _, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	fc := int(r.game.frameCount)
	h := screenH * meleeSizeScale
	emerald := [3]int{90, 225, 140}
	skyCyan := [3]int{130, 235, 235}
	white := [3]int{240, 255, 250}
	wings := 10
	widthScale := 1.0
	if s.Crit {
		h *= 1.25
		widthScale = 1.3
		wings = 16
	}

	reach := h * 0.42
	thick := h * 0.026
	baseX, baseY := cx, cy+reach*0.45
	tipLen := reach * lead
	tipX, tipY := baseX, baseY-tipLen

	r.drawDissolveStroke(screen, dissolveStroke{
		path:  func(t float64) (float64, float64) { return baseX, baseY - reach*t },
		width: func(t float64) float64 { return (4 + 6*(1-t)) * widthScale },
		color: func(t float64) [3]int {
			return mixColor(emerald, skyCyan, 0.5+0.5*math.Sin(t*9+float64(fc)*0.25))
		},
		alpha:  func(t float64) float64 { return 0.45 + 0.55*t },
		length: reach,
		seed:   seed, salt: 3, blend: additiveGlowBlend,
	}, lead, progress)

	// Wing-sparks: mirrored pairs beating outward from the shaft as it extends.
	for k := 0; k < wings; k++ {
		tb := auraHash(seed, k, 31, 0)
		if lead < tb {
			continue
		}
		wy := baseY - reach*tb - progress*h*0.02
		flap := math.Sin(progress*26 + auraHash(seed, k, 32, 0)*2*math.Pi)
		off := thick*2.5 + auraHash(seed, k, 33, 0)*thick*4 + progress*h*0.03
		a := fade * (0.35 + 0.45*flap*flap)
		c := mixColor(skyCyan, white, auraHash(seed, k, 34, 0))
		for _, sgn := range [2]float64{-1, 1} {
			x := baseX + sgn*off*(0.6+0.4*flap)
			r.drawGlowSprite(screen, x, wy, math.Max(2, thick*0.7), c, a, additiveGlowBlend)
			r.drawGlowSprite(screen, x+sgn*thick*0.7, wy-thick*0.3, math.Max(2, thick*0.5), c, a*0.7, additiveGlowBlend)
		}
	}

	// The cut: an X of light snaps open at the tip right as the lunge peaks.
	if progress >= meleeSweepFrac && progress < meleeSweepFrac+0.2 {
		u := (progress - meleeSweepFrac) / 0.2
		arm := thick * (2 + 7*u)
		a := fade * (1 - u)
		const m = 5
		for k := -m; k <= m; k++ {
			t := float64(k) / m
			sz := math.Max(2, thick*(0.8-0.5*math.Abs(t)))
			r.drawGlowSprite(screen, tipX+t*arm, tipY+t*arm*0.6, sz, white, a, additiveGlowBlend)
			r.drawGlowSprite(screen, tipX+t*arm, tipY-t*arm*0.6, sz, skyCyan, a*0.9, additiveGlowBlend)
		}
		r.drawGlowSprite(screen, tipX, tipY, thick*2.5, white, a, additiveGlowBlend)
	}
}

// Kage-kunai, the Twin Shadows - mirrored twin stabs, the right blade a
// heartbeat behind the left: a solid darkening shadow line under a violet
// edge (both dissolving in strike order), with wisps curling up between them.
func (r *Renderer) drawMeleeFxKageKunai(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, _, _ := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h := screenH * meleeSizeScale
	violet := [3]int{150, 80, 215}
	shadow := [3]int{16, 6, 26}
	wisps := 22
	widthScale := 1.0
	if s.Crit {
		h *= 1.25
		widthScale = 1.3
		wisps = 32
	}

	reach := h * 0.2
	thick := h * 0.04
	for i, side := range [2]float64{-1, 1} {
		// The lagged blade shares the dissolve model via a shifted local clock:
		// it strikes AND flakes away one heartbeat after its twin.
		lag := float64(i) * meleeSweepFrac * 0.45
		lp := progress - lag
		if lp <= 0 {
			continue
		}
		st := lp / meleeSweepFrac
		if st > 1 {
			st = 1
		}
		ld := 1 - (1-st)*(1-st)
		baseX := cx + side*h*0.055
		baseY := cy + reach*0.5
		tipX, tipY := baseX+side*reach*ld*0.18, baseY-reach*ld

		// Compressed dissolve clock so the late blade still finishes flaking
		// away before the effect expires.
		lpDissolve := lp / (1 - lag)
		bladePath := func(t float64) (float64, float64) {
			return baseX + side*reach*0.18*t, baseY - reach*t
		}
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   bladePath,
			width:  func(t float64) float64 { return (5 + 7*(1-t)) * widthScale },
			color:  func(t float64) [3]int { return shadow },
			alpha:  func(t float64) float64 { return 0.55 + 0.3*t },
			length: reach,
			seed:   seed, salt: 5 + i, blend: ebiten.BlendSourceOver,
		}, ld, lpDissolve)
		r.drawDissolveStroke(screen, dissolveStroke{
			path:   bladePath,
			width:  func(t float64) float64 { return (3 + 3*(1-t)) * widthScale },
			color:  func(t float64) [3]int { return violet },
			alpha:  func(t float64) float64 { return 0.35 + 0.55*t },
			length: reach,
			seed:   seed, salt: 7 + i, blend: additiveGlowBlend,
		}, ld, lpDissolve)
		if st < 1 {
			r.drawGlowSprite(screen, tipX, tipY, thick*1.6, [3]int{220, 190, 255}, fade*0.9, additiveGlowBlend)
			// pale violet embers flick radially off the tip while it extends
			for k := 0; k < 6; k++ {
				ea := auraHash(seed, k, 45+i, 0) * 2 * math.Pi
				er := st * thick * (1.5 + 2*auraHash(seed, k, 47+i, 0))
				r.drawGlowSprite(screen, tipX+math.Cos(ea)*er, tipY+math.Sin(ea)*er,
					math.Max(2, thick*0.3), [3]int{200, 150, 255}, fade*(1-st)*0.9, additiveGlowBlend)
			}
		}
	}

	// Shadow wisps (dense): dark motes with violet hearts curling up between
	// the blades, each dragging a smoky ghost trail along its own curl.
	wispPos := func(k int, u float64) (float64, float64) {
		side := 1.0
		if k%2 == 0 {
			side = -1
		}
		x := cx + side*h*(0.02+auraHash(seed, k, 42, 0)*0.09) + math.Sin(u*7+auraHash(seed, k, 43, 0)*2*math.Pi)*thick*2
		y := cy - u*h*0.16 + thick
		return x, y
	}
	for k := 0; k < wisps; k++ {
		born := auraHash(seed, k, 41, 0) * 0.6
		if progress <= born {
			continue
		}
		u := (progress - born) / (1 - born)
		for g := 0; g < 4; g++ {
			ug := u - float64(g)*0.06
			if ug < 0 {
				break
			}
			x, y := wispPos(k, ug)
			f := 1 - 0.22*float64(g)
			sz := math.Max(2, thick*(0.8-0.5*ug)) * f
			r.drawGlowSprite(screen, x, y, sz*1.5, shadow, fade*(1-ug)*0.5*f, ebiten.BlendSourceOver)
			r.drawGlowSprite(screen, x, y, sz*0.8, violet, fade*(1-ug)*0.6*f*f, additiveGlowBlend)
		}
	}
}

// Idol-Breaker, the Warlord's Maul - a ponderous overhead smash drawn as a
// heavy solid stone-to-amber line that lands in an impact flash, a flattened
// ground shockwave, lingering dust and a fountain of stone shards flecked
// with golden idol-glints.
func (r *Renderer) drawMeleeFxIdolBreaker(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	progress, fade, sweepT, lead := meleeFxTiming(s)
	if fade <= 0 {
		return
	}
	seed := seedFromID(s.ID)
	h := screenH * meleeSizeScale
	amber := [3]int{230, 180, 100}
	stone := [3]int{168, 158, 146}
	glint := [3]int{255, 218, 130}
	shards := 16
	widthScale := 1.0
	if s.Crit {
		h *= 1.25
		widthScale = 1.3
		shards = 26
	}

	reach := h * 0.26
	pivotX, pivotY := cx, cy-reach*0.25
	R := reach * 1.15
	thetaStart, thetaEnd := -math.Pi/2-0.55, math.Pi/2-0.12
	thick := h * 0.095

	r.drawDissolveStroke(screen, dissolveStroke{
		path: func(t float64) (float64, float64) {
			theta := thetaStart + (thetaEnd-thetaStart)*t
			return pivotX + math.Cos(theta)*R, pivotY + math.Sin(theta)*R
		},
		width:  func(t float64) float64 { return (8 + 10*t) * widthScale },
		color:  func(t float64) [3]int { return mixColor(stone, amber, t) },
		alpha:  func(t float64) float64 { return 0.4 + 0.6*t },
		length: R * (thetaEnd - thetaStart),
		seed:   seed, salt: 9, blend: additiveGlowBlend,
	}, lead, progress)

	if sweepT < 1 {
		return // impact fireworks only once the head lands
	}
	impX := pivotX + math.Cos(thetaEnd)*R
	impY := pivotY + math.Sin(thetaEnd)*R
	u := (progress - meleeSweepFrac) / (1 - meleeSweepFrac)

	if u < 0.25 {
		flash := 1 - u/0.25
		r.drawGlowSprite(screen, impX, impY, thick*(1+3*flash), [3]int{255, 240, 200}, fade*flash, additiveGlowBlend)
	}

	// Ground shockwave: a flattened ring racing outward from the impact.
	ringR := (0.15 + 0.85*u) * h * 0.3
	const ringN = 18
	for k := 0; k < ringN; k++ {
		ang := float64(k) / ringN * 2 * math.Pi
		r.drawGlowSprite(screen, impX+math.Cos(ang)*ringR, impY+math.Sin(ang)*ringR*0.35,
			math.Max(2, thick*0.32*(1-u)), amber, fade*(1-u)*0.8, additiveGlowBlend)
	}
	r.drawGlowSprite(screen, impX, impY, h*0.1+u*h*0.16, stone, fade*(1-u)*0.28, additiveGlowBlend)

	// Stone shards: a ballistic fountain with tumbling debris trails, every
	// third one a golden idol-glint; twinkling gold star-embers float up from
	// the shattered idol.
	shardPos := func(k int, uu float64) (float64, float64) {
		ang := (auraHash(seed, k, 51, 0) - 0.5) * 2.4 // fan, mostly upward
		spd := 0.5 + auraHash(seed, k, 52, 0)
		return impX + math.Sin(ang)*spd*h*0.22*uu,
			impY - math.Cos(ang)*spd*h*0.3*uu + uu*uu*h*0.36
	}
	for k := 0; k < shards; k++ {
		c := stone
		sz := thick * 0.3 * (1 - 0.4*u)
		if k%3 == 0 {
			c = glint
			sz *= 0.7
		}
		for g := 0; g < 3; g++ {
			ug := u - float64(g)*0.05
			if ug < 0 {
				break
			}
			sx, sy := shardPos(k, ug)
			f := 1 - 0.3*float64(g)
			r.drawGlowRect(screen, sx, sy, math.Max(2, sz*f), c, fade*(1-u*u)*0.9*f*f, additiveGlowBlend)
		}
	}
	fc := int(r.game.frameCount)
	const embers = 7
	for k := 0; k < embers; k++ {
		eb := auraHash(seed, k, 55, 0) * 0.3
		if u <= eb {
			continue
		}
		eu := (u - eb) / (1 - eb)
		tw := 0.5 + 0.5*math.Sin(float64(fc)*0.3+auraHash(seed, k, 56, 0)*2*math.Pi)
		ex := impX + (auraHash(seed, k, 57, 0)-0.5)*h*0.14 + math.Sin(eu*5)*thick*0.4
		ey := impY - eu*h*0.14
		r.drawSparkStar(screen, ex, ey, math.Max(2, thick*0.22*(1-eu*0.5)),
			glint, [3]int{255, 245, 200}, fade*(1-eu)*tw, 1)
	}
}
