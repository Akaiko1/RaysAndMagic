package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

type weaponProjectileFxSideDraw func(*Renderer, *ebiten.Image, float64, float64, float64, float64, float64, float64, int)
type weaponProjectileFxHeadOnDraw func(*Renderer, *ebiten.Image, float64, float64, float64, float64, int)

// weaponProjectileFxStyle keeps every projection of one authored signature in
// one registry entry. Incoming is optional: nil deliberately reuses headOn for
// radial signatures and effects that already depict the projectile head.
type weaponProjectileFxStyle struct {
	side     weaponProjectileFxSideDraw
	headOn   weaponProjectileFxHeadOnDraw
	incoming weaponProjectileFxHeadOnDraw
}

// weaponProjectileFxStyles overlays a weapon-specific signature on top of the
// normal arrow or spell orb. The base projectile remains intact, preserving
// the readable silhouette shared by all ranged attacks.
var weaponProjectileFxStyles = map[string]weaponProjectileFxStyle{
	"arena_recurve": {
		side:     (*Renderer).drawWeaponProjectileFxArenaRecurve,
		headOn:   (*Renderer).drawWeaponProjectileFxArenaRecurveHeadOn,
		incoming: (*Renderer).drawWeaponProjectileFxArenaRecurveIncoming,
	},
	"arena_arbalest": {
		side:   (*Renderer).drawWeaponProjectileFxArenaArbalest,
		headOn: (*Renderer).drawWeaponProjectileFxArenaArbalestHeadOn,
	},
	"arena_lanista": {
		side:   (*Renderer).drawWeaponProjectileFxArenaLanista,
		headOn: (*Renderer).drawWeaponProjectileFxArenaLanistaHeadOn,
	},
	"clock_pistol": {
		side:   (*Renderer).drawWeaponProjectileFxClockPistol,
		headOn: (*Renderer).drawWeaponProjectileFxClockPistolHeadOn,
	},
	"dragon_wing": {
		side:     (*Renderer).drawWeaponProjectileFxDragonWing,
		headOn:   (*Renderer).drawWeaponProjectileFxDragonWingHeadOn,
		incoming: (*Renderer).drawWeaponProjectileFxDragonWingIncoming,
	},
	"dragon_nest": {
		side:   (*Renderer).drawWeaponProjectileFxDragonNest,
		headOn: (*Renderer).drawWeaponProjectileFxDragonNestHeadOn,
	},
	"tech_suppressor": {
		side:   (*Renderer).drawWeaponProjectileFxTechSuppressor,
		headOn: (*Renderer).drawWeaponProjectileFxTechSuppressorHeadOn,
	},
	"tech_longlance": {
		side:   (*Renderer).drawWeaponProjectileFxTechLonglance,
		headOn: (*Renderer).drawWeaponProjectileFxTechLonglanceHeadOn,
	},
	"tech_compound_bow": {
		side:     (*Renderer).drawWeaponProjectileFxTechCompound,
		headOn:   (*Renderer).drawWeaponProjectileFxTechCompoundHeadOn,
		incoming: (*Renderer).drawWeaponProjectileFxTechCompoundIncoming,
	},
}

func (r *Renderer) drawWeaponProjectileFx(style string, screen *ebiten.Image, cx, cy, size, dirX, dirY, critBoost float64, id int) {
	if fx, ok := weaponProjectileFxStyles[style]; ok && fx.side != nil {
		fx.side(r, screen, cx, cy, size, dirX, dirY, critBoost, id)
	}
}

// drawBlasterWeaponProjectileFx keeps a blaster's signature aligned with the
// same projection used by drawBulletTracer. Side-on shots use the authored
// directional overlay; shots travelling along the camera axis use a compact
// radial signature instead of inventing a horizontal +X flight direction.
// The return value reports whether the head-on path was selected.
func (r *Renderer) drawBlasterWeaponProjectileFx(style string, screen *ebiten.Image, cx, cy, size, vx, vy, critBoost float64, id int) bool {
	dirX, ok := r.projectileScreenDir(vx, vy)
	if ok {
		r.drawWeaponProjectileFx(style, screen, cx, cy, size, dirX, 0, critBoost, id)
		return false
	}
	r.drawWeaponProjectileFxHeadOn(style, screen, cx, cy, size, critBoost, id)
	return true
}

func (r *Renderer) drawWeaponProjectileFxHeadOn(style string, screen *ebiten.Image, cx, cy, size, critBoost float64, id int) {
	if fx, ok := weaponProjectileFxStyles[style]; ok && fx.headOn != nil {
		fx.headOn(r, screen, cx, cy, size, critBoost, id)
	}
}

func (r *Renderer) drawBowWeaponProjectileFxHeadOn(style string, screen *ebiten.Image, cx, cy, size, critBoost float64, id int, incoming bool) {
	fx, ok := weaponProjectileFxStyles[style]
	if !ok {
		return
	}
	draw := fx.headOn
	if incoming && fx.incoming != nil {
		draw = fx.incoming
	}
	if draw != nil {
		draw(r, screen, cx, cy, size, critBoost, id)
	}
}

func (r *Renderer) drawWeaponProjectileFxArenaRecurveHeadOn(screen *ebiten.Image, cx, cy, size, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	// Rear view of the enchanted arrow: loose feathers orbit its fletching.
	gold, feather := [3]int{220, 166, 82}, [3]int{255, 238, 182}
	for k := 0; k < 8; k++ {
		angle := fc*0.08 + 2*math.Pi*float64(k)/8
		radius := size * (0.75 + 0.38*float64(k%2))
		r.drawGlowRectRotated(screen, cx+math.Cos(angle)*radius, cy+math.Sin(angle)*radius,
			size*0.38, math.Max(2, size*0.1), angle, mixColor(gold, feather, float64(k%2)*0.5),
			0.5*critBoost, additiveGlowBlend)
	}
}

func (r *Renderer) drawWeaponProjectileFxArenaArbalestHeadOn(screen *ebiten.Image, cx, cy, size, critBoost float64, id int) {
	// Its pressure cage is already radial; collapse only the depth axis.
	r.drawWeaponProjectileFxArenaArbalest(screen, cx, cy, size, 0, 0, critBoost, id)
}

func (r *Renderer) drawWeaponProjectileFxArenaLanistaHeadOn(screen *ebiten.Image, cx, cy, size, critBoost float64, id int) {
	// The scepter's solar ring is naturally readable end-on.
	r.drawWeaponProjectileFxArenaLanista(screen, cx, cy, size, 0, 0, critBoost, id)
}

func (r *Renderer) drawWeaponProjectileFxClockPistolHeadOn(screen *ebiten.Image, cx, cy, size, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	// The cog is seen face-on around the muzzle flash.
	const teeth = 8
	for i := 0; i < teeth; i++ {
		angle := fc*0.28 + 2*math.Pi*float64(i)/teeth
		radius := size * 1.15
		if i%2 == 0 {
			radius *= 1.25
		}
		r.drawGlowRect(screen, cx+math.Cos(angle)*radius, cy+math.Sin(angle)*radius,
			math.Max(2, size*0.14), mixColor(clockBrass, clockWhite, float64(i%2)*0.5),
			0.5*critBoost, additiveGlowBlend)
	}
}

func (r *Renderer) drawWeaponProjectileFxDragonWingHeadOn(screen *ebiten.Image, cx, cy, size, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	// Four membrane vanes seen around the arrow tail, beating in and out.
	beat := 0.62 + 0.28*math.Sin(fc*0.34)
	wing, vein := [3]int{188, 226, 176}, [3]int{232, 250, 224}
	for k := 0; k < 4; k++ {
		angle := fc*0.025 + 2*math.Pi*float64(k)/4
		rootX, rootY := cx+math.Cos(angle)*size*0.35, cy+math.Sin(angle)*size*0.35
		tipX, tipY := cx+math.Cos(angle)*size*1.6*beat, cy+math.Sin(angle)*size*1.6*beat
		r.fxSegment(screen, rootX, rootY, tipX, tipY, math.Max(2.5, size*0.17), wing, 0.58*critBoost, additiveGlowBlend)
		r.fxSegment(screen, rootX, rootY, tipX, tipY, math.Max(2, size*0.07), vein, 0.78*critBoost, additiveGlowBlend)
	}
}

func (r *Renderer) drawWeaponProjectileFxDragonNestHeadOn(screen *ebiten.Image, cx, cy, size, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	// The seeker head becomes a ring of inward-facing clutch hooks.
	hunt, bone := [3]int{178, 208, 122}, dragonBone
	for k := 0; k < 6; k++ {
		angle := fc*0.04 + 2*math.Pi*float64(k)/6
		outerX, outerY := cx+math.Cos(angle)*size*1.35, cy+math.Sin(angle)*size*1.35
		innerX, innerY := cx+math.Cos(angle+0.45)*size*0.62, cy+math.Sin(angle+0.45)*size*0.62
		r.fxSegment(screen, outerX, outerY, innerX, innerY, math.Max(2.5, size*0.18), hunt, 0.65*critBoost, additiveGlowBlend)
		r.fxSegment(screen, outerX, outerY, innerX, innerY, math.Max(2, size*0.08), bone, 0.82*critBoost, additiveGlowBlend)
	}
}

func (r *Renderer) drawWeaponProjectileFxTechSuppressorHeadOn(screen *ebiten.Image, cx, cy, size, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	// End-on rifling reads as two counter-rotating gas rings.
	for ring := 0; ring < 2; ring++ {
		radius := size * (0.75 + 0.55*float64(ring))
		spin := fc * 0.5 * (1 - 2*float64(ring))
		for i := 0; i < 10; i++ {
			angle := spin + 2*math.Pi*float64(i)/10
			r.drawGlowRect(screen, cx+math.Cos(angle)*radius, cy+math.Sin(angle)*radius,
				math.Max(2, size*(0.16-0.03*float64(ring))),
				mixColor(techWhite, techCyan, 0.35+0.3*float64(ring)),
				(0.5-0.12*float64(ring))*critBoost, additiveGlowBlend)
		}
	}
}

func (r *Renderer) drawWeaponProjectileFxTechLonglanceHeadOn(screen *ebiten.Image, cx, cy, size, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	// The lance collapses into its scope reticle when viewed down-axis.
	pulse := 0.75 + 0.25*math.Sin(fc*0.5)
	for ring := 0; ring < 2; ring++ {
		radius := size * (0.8 + 0.55*float64(ring))
		for i := 0; i < 12; i++ {
			angle := 2 * math.Pi * float64(i) / 12
			r.drawGlowRect(screen, cx+math.Cos(angle)*radius, cy+math.Sin(angle)*radius,
				math.Max(2, size*0.1), techCyan,
				(0.5-0.16*float64(ring))*pulse*critBoost, additiveGlowBlend)
		}
	}
	for _, offset := range []float64{-1, 1} {
		r.drawGlowRect(screen, cx+offset*size*1.8, cy, math.Max(2, size*0.11),
			techWhite, 0.55*pulse*critBoost, additiveGlowBlend)
		r.drawGlowRect(screen, cx, cy+offset*size*1.8, math.Max(2, size*0.11),
			techWhite, 0.55*pulse*critBoost, additiveGlowBlend)
	}
}

func (r *Renderer) drawWeaponProjectileFxTechCompoundHeadOn(screen *ebiten.Image, cx, cy, size, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	// The two cams remain visible as counter-rotating rings around the nock.
	alloy := [3]int{168, 196, 214}
	for ring := 0; ring < 2; ring++ {
		radius := size * (0.82 + 0.5*float64(ring))
		spin := fc * 0.22 * (1 - 2*float64(ring))
		for k := 0; k < 10; k++ {
			angle := spin + 2*math.Pi*float64(k)/10
			r.drawGlowRectRotated(screen, cx+math.Cos(angle)*radius, cy+math.Sin(angle)*radius,
				size*0.3, math.Max(2, size*0.1), angle+math.Pi/2,
				mixColor(alloy, techWhite, 0.4*float64(ring)),
				(0.5-0.12*float64(ring))*critBoost, additiveGlowBlend)
		}
	}
}

func (r *Renderer) drawWeaponProjectileFxArenaRecurveIncoming(screen *ebiten.Image, cx, cy, size, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	// A compact pressure crown surrounds the approaching arrowhead.
	gold, white := [3]int{220, 166, 82}, [3]int{255, 238, 182}
	for k := 0; k < 8; k++ {
		angle := fc*0.08 + 2*math.Pi*float64(k)/8
		inner := size * 0.68
		outer := size * (1.05 + 0.12*float64(k%2))
		r.fxSegment(screen,
			cx+math.Cos(angle)*inner, cy+math.Sin(angle)*inner,
			cx+math.Cos(angle)*outer, cy+math.Sin(angle)*outer,
			math.Max(2, size*0.12), mixColor(gold, white, float64(k%2)*0.45),
			0.55*critBoost, additiveGlowBlend)
	}
}

func (r *Renderer) drawWeaponProjectileFxDragonWingIncoming(screen *ebiten.Image, cx, cy, size, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	// Folded wing energy frames the approaching point without exposing the rear
	// membrane vanes.
	wing, vein := [3]int{188, 226, 176}, [3]int{232, 250, 224}
	pulse := 0.82 + 0.14*math.Sin(fc*0.34)
	for k := 0; k < 4; k++ {
		angle := fc*0.025 + math.Pi/4 + 2*math.Pi*float64(k)/4
		inner := size * 0.72
		outer := size * 1.25 * pulse
		r.fxSegment(screen,
			cx+math.Cos(angle)*inner, cy+math.Sin(angle)*inner,
			cx+math.Cos(angle)*outer, cy+math.Sin(angle)*outer,
			math.Max(2, size*0.12), wing, 0.55*critBoost, additiveGlowBlend)
		r.drawGlowRect(screen, cx+math.Cos(angle)*outer, cy+math.Sin(angle)*outer,
			math.Max(2, size*0.13), vein, 0.72*critBoost, additiveGlowBlend)
	}
}

func (r *Renderer) drawWeaponProjectileFxTechCompoundIncoming(screen *ebiten.Image, cx, cy, size, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	// The approaching broadhead is bracketed by a rotating targeting ring; the
	// rear nock and cams stay hidden behind it.
	alloy := [3]int{168, 196, 214}
	spin := fc * 0.22
	for k := 0; k < 12; k++ {
		angle := spin + 2*math.Pi*float64(k)/12
		radius := size * 1.02
		r.drawGlowRectRotated(screen, cx+math.Cos(angle)*radius, cy+math.Sin(angle)*radius,
			size*0.24, math.Max(2, size*0.09), angle+math.Pi/2,
			mixColor(alloy, techWhite, float64(k%2)*0.45),
			0.48*critBoost, additiveGlowBlend)
	}
}

// drawOutgoingBowFromHand renders the short screen-space convergence between
// the party's right hand and the centered line of fire. The side silhouette is
// progressively foreshortened into the rear view at one shared moving anchor.
func (r *Renderer) drawOutgoingBowFromHand(style string, screen *ebiten.Image, cx, cy, size float64, col [3]int, critBoost float64, id int, convergence float64) bool {
	if convergence <= 0 {
		return false
	}
	convergence = math.Min(1, convergence)
	anchorX := cx + float64(screen.Bounds().Dx())*0.055*convergence
	anchorY := cy + float64(screen.Bounds().Dy())*0.025*convergence
	sideAngle := -math.Pi + 0.10 + 0.08*convergence
	profileAlpha := math.Sqrt(convergence)
	rearAlpha := 1 - convergence

	if style != "" {
		r.drawWeaponProjectileFx(style, screen, anchorX, anchorY, size,
			math.Cos(sideAngle), math.Sin(sideAngle), critBoost*profileAlpha, id)
		if rearAlpha > 0 {
			r.drawBowWeaponProjectileFxHeadOn(style, screen, anchorX, anchorY, size,
				critBoost*rearAlpha, id, false)
		}
	}
	r.drawArrowQuadForeshortened(screen, anchorX, anchorY, size, sideAngle, convergence, col, profileAlpha)
	if rearAlpha > 0 {
		r.drawArrowHeadOn(screen, anchorX, anchorY, size, col, rearAlpha, false)
	}
	return true
}

func (r *Renderer) drawWeaponProjectileFxArenaRecurve(screen *ebiten.Image, cx, cy, size, dirX, dirY, critBoost float64, id int) {
	seed := id + 401
	gold, feather := [3]int{220, 166, 82}, [3]int{255, 238, 182}
	for k := 0; k < 7; k++ {
		t := 0.18 + 0.13*float64(k)
		spread := (auraHash(seed, k, 402, 0) - 0.5) * size * 1.35
		x := cx - dirX*size*3.2*t - dirY*spread
		y := cy - dirY*size*3.2*t + dirX*spread
		r.drawGlowRect(screen, x, y, math.Max(2, size*(0.10+0.03*float64(k%2))), mixColor(gold, feather, float64(k%3)/2), (0.55-0.05*float64(k))*critBoost, additiveGlowBlend)
	}
}

func (r *Renderer) drawWeaponProjectileFxArenaArbalest(screen *ebiten.Image, cx, cy, size, dirX, dirY, critBoost float64, id int) {
	steel, spark := [3]int{155, 175, 195}, [3]int{238, 246, 255}
	fc := float64(r.game.frameCount)
	for ring := 0; ring < 2; ring++ {
		radius := size * (0.65 + 0.38*float64(ring))
		for i := 0; i < 6; i++ {
			a := fc*0.16*(1+float64(ring)*0.25) + 2*math.Pi*float64(i)/6
			r.drawGlowRect(screen, cx+math.Cos(a)*radius, cy+math.Sin(a)*radius*0.72,
				math.Max(2, size*0.105), mixColor(steel, spark, float64(ring)*0.35), 0.48*critBoost, additiveGlowBlend)
		}
	}
	// A pin-straight pressure line makes the piercing bolt read even before it hits.
	for i := 1; i <= 4; i++ {
		t := float64(i) / 4
		r.drawGlowSprite(screen, cx-dirX*size*(0.45+1.55*t), cy-dirY*size*(0.45+1.55*t),
			size*(0.18-0.02*t), spark, (0.42-0.06*t)*critBoost, additiveGlowBlend)
	}
	_ = id
}

func (r *Renderer) drawWeaponProjectileFxArenaLanista(screen *ebiten.Image, cx, cy, size, dirX, dirY, critBoost float64, id int) {
	gold, white := [3]int{245, 202, 94}, [3]int{255, 251, 220}
	fc := float64(r.game.frameCount)
	for i := 0; i < 10; i++ {
		a := fc*0.09 + 2*math.Pi*float64(i)/10
		radius := size * (1.05 + 0.08*math.Sin(fc*0.13+float64(i)))
		r.drawGlowSprite(screen, cx+math.Cos(a)*radius, cy+math.Sin(a)*radius*0.72,
			size*0.20, mixColor(gold, white, float64(i%2)*0.4), 0.54*critBoost, additiveGlowBlend)
	}
	for k := 1; k <= 3; k++ {
		t := float64(k) / 3
		r.drawGlowSprite(screen, cx-dirX*size*(1.2+2.1*t), cy-dirY*size*(1.2+2.1*t),
			size*(0.18-0.03*t), gold, (0.3-0.06*t)*critBoost, additiveGlowBlend)
	}
	_ = id
}
