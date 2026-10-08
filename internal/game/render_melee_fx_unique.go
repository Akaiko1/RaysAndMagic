package game

import (
	"fmt"
	"math"

	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

// Bespoke swing flourishes for legendary melee weapons, selected by
// graphics.slash_fx in weapons.yaml (SlashEffect.Style). Each stroke is a
// shaded ribbon that dissolves in draw order (drawDissolveStroke) plus
// deterministic particles in the weapon's own element and silhouette; weapons
// without a style keep the category swing in drawMeleeParticles.

// meleeFxStyledLingerFrames extends the lifetime of styled swings so their
// dissolve tails (blood droplets, debris, wisps) have room to play out.
const meleeFxStyledLingerFrames = 44 // ~0.37s at 120 TPS

// meleeFxStyleDraw maps graphics.slash_fx to its bespoke renderer
// (legendaries here, rares + naginata in render_melee_fx_rare.go).
var meleeFxStyleDraw = map[string]func(*Renderer, *ebiten.Image, SlashEffect, float64, float64, float64){
	"war_fan":           (*Renderer).drawMeleeFxWarFan,
	"katana":            (*Renderer).drawMeleeFxKatana,
	"solstice_thermal":  (*Renderer).drawMeleeFxSolsticeThermal,
	"solstice_anchor":   (*Renderer).drawMeleeFxSolsticeAnchor,
	"solstice_transfer": (*Renderer).drawMeleeFxSolsticeTransfer,
	"muramasa":          (*Renderer).drawMeleeFxMuramasa,
	"tonbogiri":         (*Renderer).drawMeleeFxTonbogiri,
	"kage_kunai":        (*Renderer).drawMeleeFxKageKunai,
	"idol_breaker":      (*Renderer).drawMeleeFxIdolBreaker,
	"silver_sword":      (*Renderer).drawMeleeFxSilverSword,
	"gold_sword":        (*Renderer).drawMeleeFxGoldSword,
	"agility_katar":     (*Renderer).drawMeleeFxAgilityKatar,
	"gorehorn":          (*Renderer).drawMeleeFxGorehorn,
	"serpent_fang":      (*Renderer).drawMeleeFxSerpentFang,
	"naginata":          (*Renderer).drawMeleeFxNaginata,
	"arena_gladius":     (*Renderer).drawMeleeFxArenaGladius,
	"arena_labrys":      (*Renderer).drawMeleeFxArenaLabrys,
	"arena_morningstar": (*Renderer).drawMeleeFxArenaMorningstar,
	"arena_hasta":       (*Renderer).drawMeleeFxArenaHasta,
	"arena_trident":     (*Renderer).drawMeleeFxArenaTrident,
	"arena_parry":       (*Renderer).drawMeleeFxArenaParry,
	"arena_lion":        (*Renderer).drawMeleeFxArenaLion,
	"arena_cesti":       (*Renderer).drawMeleeFxArenaCesti,
	"clock_cogfang":     (*Renderer).drawMeleeFxClockCogfang,
	"clock_chime":       (*Renderer).drawMeleeFxClockChime,
	"clock_minute":      (*Renderer).drawMeleeFxClockMinute,
	"clock_mainspring":  (*Renderer).drawMeleeFxClockMainspring,
	"clock_escapement":  (*Renderer).drawMeleeFxClockEscapement,
	"dragon_fang":       (*Renderer).drawMeleeFxDragonFang,
	"dragon_jaws":       (*Renderer).drawMeleeFxDragonJaws,
	"dragon_ember_egg":  (*Renderer).drawMeleeFxDragonEmberEgg,
	"dragon_broodspike": (*Renderer).drawMeleeFxDragonBroodspike,
	"dragon_tarn":       (*Renderer).drawMeleeFxDragonTarn,
	"dragon_hatchling":  (*Renderer).drawMeleeFxDragonHatchling,
	"dragon_roar":       (*Renderer).drawMeleeFxDragonRoar,
	"tech_vibro":        (*Renderer).drawMeleeFxTechVibro,
}

// validateWeaponFxStyles fails fast when a weapon effect names a style with no
// renderer - a YAML typo would otherwise silently fall back to the stock FX.
func validateWeaponFxStyles() {
	if config.GlobalWeapons == nil {
		return
	}
	for key, def := range config.GlobalWeapons.Weapons {
		if def.Graphics == nil {
			continue
		}
		if def.Graphics.SlashFx != "" {
			if _, ok := meleeFxStyleDraw[def.Graphics.SlashFx]; !ok {
				panic(fmt.Sprintf("weapon %q: unknown slash_fx style %q", key, def.Graphics.SlashFx))
			}
		}
		if def.Graphics.ProjectileFx != "" {
			fx, ok := weaponProjectileFxStyles[def.Graphics.ProjectileFx]
			if !ok || fx.side == nil || fx.headOn == nil {
				panic(fmt.Sprintf("weapon %q: unknown weapon projectile_fx style %q", key, def.Graphics.ProjectileFx))
			}
		}
	}
}

// Cross-sections approximate authored curves; erosion is evaluated per fragment.
const dissolveStepPx = 5.0

// dissolveStroke describes one material ribbon: a path with per-parameter
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

// drawDissolveStroke keeps authored geometry on the CPU and evaluates flowing
// light, edge detail and birth-ordered erosion in the shared ribbon shader.
func (r *Renderer) drawDissolveStroke(screen *ebiten.Image, st dissolveStroke, lead, progress float64) {
	if lead <= 0 || progress >= 1 || r.ensureWeaponMaterialShaders() != nil {
		return
	}
	lead = math.Min(1, lead)
	n := max(12, min(260, int(st.length/dissolveStepPx)))
	step := .5 / float64(n)
	if progress >= 0 && !r.weaponMaterialState.pose.ready {
		x, y := st.path(lead)
		r.weaponMaterialState.pose = weaponStrokePose{x, y, tangentAt(st.path, lead), true}
	}
	profile := r.weaponMaterialState.trail
	// Continuous machinery and projectile wakes retain their closed geometry.
	taper := progress >= 0 && profile.widthScale > 0
	verts := r.standeeVerts[:0]
	appendSection := func(t float64) {
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
		w := st.width(t) * 1.1
		col, a := st.color(t), st.alpha(t)
		if taper {
			// A narrow translucent wake supports the solid model instead of
			// replacing it with a broad opaque stroke.
			w = math.Min(w*profile.widthScale*.55, profile.widthLimit*.55)
			a *= .58
			tail := math.Min(1, t/math.Min(profile.tail, lead*.45))
			tip := math.Min(1, (lead-t)/math.Min(profile.tip, lead*.45))
			if profile.angular {
				// An axe cut widens into a wedge and ends in a sharp cutting edge.
				w *= (.35 + .65*t/lead) * tail * tip
			} else {
				w *= tail * tail * (3 - 2*tail) * tip * tip * (3 - 2*tip)
			}
			a *= math.Sqrt(tail * tip)
		}
		for _, side := range []float64{-1, 1} {
			v := weaponMaterialVertex(x+nx*w*side, y+ny*w*side, t*st.length/36, side, col, a)
			v.Custom0, v.Custom1 = float32(progress), float32(t)
			v.Custom2 = float32(uint(st.seed+st.salt)%997) * .013
			v.Custom3 = float32(r.weaponMaterialState.material) + .25
			verts = append(verts, v)
		}
	}
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		if t >= lead {
			break
		}
		appendSection(t)
	}
	appendSection(lead)
	idx := r.standeeIdx[:0]
	for i := 1; i < len(verts)/2; i++ {
		b := uint16(i*2 - 2)
		idx = append(idx, b, b+1, b+2, b+1, b+3, b+2)
	}
	if len(idx) > 0 {
		r.weaponMaterialOpts.Blend = weaponShaderBlend(st.blend)
		worldDrawTrianglesShader(screen, verts, idx, r.weaponRibbonShader, &r.weaponMaterialOpts)
	}
	r.standeeVerts, r.standeeIdx = verts[:0], idx[:0]
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
		color:  func(t float64) [3]int { return mixColor(blood, steel, 0.08+0.18*t) },
		alpha:  func(t float64) float64 { return 0.55 + 0.45*t },
		length: R * (thetaEnd - thetaStart),
		seed:   seed, salt: 1, blend: ebiten.BlendSourceOver,
	}, lead, progress)
	// cursed echo: a thinner crimson line trailing just inside the razor cut
	r.drawDissolveStroke(screen, dissolveStroke{
		path:   func(t float64) (float64, float64) { return arcAt(t, R-thick*0.45) },
		width:  func(t float64) float64 { return (3 + 4*math.Sin(math.Pi*t)) * widthScale },
		color:  func(t float64) [3]int { return blood },
		alpha:  func(t float64) float64 { return 0.4 * t },
		length: R * (thetaEnd - thetaStart),
		seed:   seed, salt: 2, blend: ebiten.BlendSourceOver,
	}, lead, progress)
	if sweepT < 1 {
		tx, ty := arcAt(lead, R)
		r.drawGlowSprite(screen, tx, ty, thick*2.2, steel, fade, ebiten.BlendSourceOver)
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
			r.drawGlowSprite(screen, bx, by, math.Max(2, thick*(0.5-0.25*u)*f), c, fade*(1-u)*0.95*f*f, ebiten.BlendSourceOver)
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
		r.drawGlowSprite(screen, mx, my, thick*(1.2+1.8*u), [3]int{120, 10, 22}, fade*(1-u)*0.3, ebiten.BlendSourceOver)
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

// Kage Kunai alternate two narrow stabs, each carrying its own blade model.
func (r *Renderer) drawMeleeFxKageKunai(screen *ebiten.Image, s SlashEffect, cx, cy, screenH float64) {
	r.drawIdentityStrike(screen, s, cx, cy, screenH, "kage_kunai", "stab")
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

	if progress < .65 {
		tipAngle := thetaStart + (thetaEnd-thetaStart)*lead
		r.drawHammerHead(screen, pivotX+math.Cos(tipAngle)*R, pivotY+math.Sin(tipAngle)*R, h*.075, tipAngle+math.Pi, fade)
	}

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

	// The landing pushes out a dense, broken front of powdered stone.
	r.drawImpactCloud(screen, impX, impY, h*.30, h*.12, u, mixColor(stone, amber, .3), fade*.9, seed, true)

	shardPos := func(k int, uu float64) (float64, float64) {
		ang := (auraHash(seed, k, 51, 0) - .5) * 2.4
		spd := .5 + auraHash(seed, k, 52, 0)
		return impX + math.Sin(ang)*spd*h*.22*uu, impY - math.Cos(ang)*spd*h*.3*uu + uu*uu*h*.36
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
			r.drawWeaponShard(screen, sx, sy, math.Max(2, sz*f), c, fade*(1-u*u)*0.9*f*f, ug, seed+k*37, false)
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
