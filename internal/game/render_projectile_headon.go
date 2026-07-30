package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawSpellProjectileFxHeadOn renders a projectile travelling along the camera
// axis. Round or cloud-like bodies can reuse their authored renderer with a
// collapsed depth direction. Elongated bodies need a real end-on silhouette so
// they do not become a horizontal bolt when screen-space motion approaches zero.
func (r *Renderer) drawSpellProjectileFxHeadOn(screen *ebiten.Image, cx, cy, size float64, core [3]int, p projectileFxProfile, critBoost float64, id int) {
	if size < spellFxMinClusterSize {
		size = spellFxMinClusterSize
	}
	switch p.style {
	case "fireball", "harm", "psyshock", "starburst", "disintegrate",
		"rock", "stone_bud", "swarm", "sparks", "shackle", "charm_bloom":
		if draw, ok := spellFxStyleDraw[p.style]; ok {
			draw(r, screen, cx, cy, size, 0, 0, core, p, critBoost, id)
			return
		}
	case "lightning":
		r.drawSpellFxHeadOnLightning(screen, cx, cy, size, critBoost, id)
		return
	case "ray_of_light":
		r.drawSpellFxHeadOnRay(screen, cx, cy, size, core, critBoost, id)
		return
	case "ice_shard":
		r.drawSpellFxHeadOnIce(screen, cx, cy, size, critBoost)
		return
	case "fire_dart":
		r.drawSpellFxHeadOnFireDart(screen, cx, cy, size, critBoost)
		return
	case "shadow_bolt":
		r.drawSpellFxHeadOnShadow(screen, cx, cy, size, critBoost)
		return
	case "void_needle":
		r.drawSpellFxHeadOnVoidNeedle(screen, cx, cy, size, critBoost)
		return
	case "psi_lance":
		r.drawSpellFxHeadOnPsiLance(screen, cx, cy, size, critBoost)
		return
	case "lash":
		r.drawSpellFxHeadOnLash(screen, cx, cy, size, critBoost)
		return
	}
	r.drawSpellFxHeadOnGeneric(screen, cx, cy, size, core, p, critBoost, id)
}

func (r *Renderer) drawSpellFxHeadOnGeneric(screen *ebiten.Image, cx, cy, size float64, core [3]int, p projectileFxProfile, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	hot := mixColor(core, [3]int{255, 255, 255}, 0.65)
	pulse := 0.9 + 0.1*math.Sin(fc*0.28+float64(id))
	r.drawGlowSprite(screen, cx, cy, size*1.8*pulse*critBoost, core, 0.45, additiveGlowBlend)
	r.drawGlowSprite(screen, cx, cy, size*0.8*pulse, hot, 0.95, additiveGlowBlend)
	for ring := 0; ring < 2; ring++ {
		radius := size * (0.85 + 0.65*float64(ring))
		for k := 0; k < 10; k++ {
			angle := fc*0.04*(1-2*float64(ring)) + 2*math.Pi*float64(k)/10
			r.drawGlowRect(screen, cx+math.Cos(angle)*radius, cy+math.Sin(angle)*radius,
				math.Max(2, size*(0.13-0.03*float64(ring))),
				mixColor(hot, p.trailColor, float64(ring)*0.55),
				(0.42-0.12*float64(ring))*critBoost, additiveGlowBlend)
		}
	}
}

func (r *Renderer) drawSpellFxHeadOnLightning(screen *ebiten.Image, cx, cy, size, critBoost float64, id int) {
	fc := int(r.game.frameCount)
	hot := [3]int{240, 250, 255}
	blue := [3]int{120, 170, 255}
	r.drawGlowSprite(screen, cx, cy, size*1.5*critBoost, blue, 0.55, additiveGlowBlend)
	for spoke := 0; spoke < 8; spoke++ {
		angle := 2*math.Pi*float64(spoke)/8 + auraHash(id, spoke, 701, fc/3)*0.35
		px, py := cx, cy
		for step := 1; step <= 3; step++ {
			radius := size * 0.48 * float64(step)
			jitter := (auraHash(id, spoke*4+step, 702, fc/3) - 0.5) * 0.45
			nx := cx + math.Cos(angle+jitter)*radius
			ny := cy + math.Sin(angle+jitter)*radius
			r.fxSegment(screen, px, py, nx, ny, math.Max(2, size*0.12), hot,
				(0.95-0.22*float64(step))*critBoost, additiveGlowBlend)
			r.fxSegment(screen, px, py, nx, ny, math.Max(3, size*0.3), blue,
				(0.4-0.08*float64(step))*critBoost, additiveGlowBlend)
			px, py = nx, ny
		}
	}
}

func (r *Renderer) drawSpellFxHeadOnRay(screen *ebiten.Image, cx, cy, size float64, core [3]int, critBoost float64, id int) {
	fc := float64(r.game.frameCount)
	hot := mixColor(core, [3]int{255, 255, 255}, 0.82)
	pulse := 0.92 + 0.08*math.Sin(fc*0.5+float64(id))
	r.drawGlowSprite(screen, cx, cy, size*2.6*critBoost, core, 0.42*pulse, additiveGlowBlend)
	r.drawGlowSprite(screen, cx, cy, size*1.15, hot, pulse, additiveGlowBlend)
	for ring := 0; ring < 2; ring++ {
		radius := size * (0.8 + 0.72*float64(ring))
		for k := 0; k < 12; k++ {
			angle := fc*0.05*(1-2*float64(ring)) + 2*math.Pi*float64(k)/12
			r.drawGlowRect(screen, cx+math.Cos(angle)*radius, cy+math.Sin(angle)*radius,
				math.Max(2, size*0.1), mixColor(core, hot, 0.45),
				(0.5-0.16*float64(ring))*pulse, additiveGlowBlend)
		}
	}
}

func (r *Renderer) drawSpellFxHeadOnIce(screen *ebiten.Image, cx, cy, size, critBoost float64) {
	fc := float64(r.game.frameCount)
	rime := [3]int{236, 250, 255}
	ice := [3]int{130, 200, 250}
	deep := [3]int{50, 120, 200}
	r.drawGlowSprite(screen, cx, cy, size*1.8*critBoost, ice, 0.3, additiveGlowBlend)
	for k := 0; k < 6; k++ {
		angle := fc*0.035 + 2*math.Pi*float64(k)/6
		radius := size * 0.72
		x, y := cx+math.Cos(angle)*radius, cy+math.Sin(angle)*radius
		r.drawGlowRectRotated(screen, x, y, size*0.75, size*0.22, angle, deep, 0.95, ebiten.BlendSourceOver)
		r.drawGlowRectRotated(screen, x, y, size*0.56, size*0.1, angle, rime, 0.95, ebiten.BlendSourceOver)
	}
	r.drawGlowRectRotated(screen, cx, cy, size*0.62, size*0.62, math.Pi/4, ice, 1, ebiten.BlendSourceOver)
}

func (r *Renderer) drawSpellFxHeadOnFireDart(screen *ebiten.Image, cx, cy, size, critBoost float64) {
	fc := float64(r.game.frameCount)
	heart := [3]int{255, 244, 190}
	flame := [3]int{255, 152, 44}
	deep := [3]int{196, 44, 12}
	pulse := 0.9 + 0.1*math.Sin(fc*0.45)
	r.drawGlowSprite(screen, cx, cy, size*1.55*pulse*critBoost, deep, 0.9, ebiten.BlendSourceOver)
	r.drawGlowSprite(screen, cx, cy, size*1.05*pulse, flame, 1, ebiten.BlendSourceOver)
	r.drawGlowSprite(screen, cx, cy, size*0.48*pulse, heart, 1, ebiten.BlendSourceOver)
	for k := 0; k < 7; k++ {
		angle := fc*0.08 + 2*math.Pi*float64(k)/7
		radius := size * (0.9 + 0.2*math.Sin(fc*0.2+float64(k)))
		r.drawGlowSprite(screen, cx+math.Cos(angle)*radius, cy+math.Sin(angle)*radius,
			size*0.25, mixColor(flame, heart, 0.35), 0.65*critBoost, additiveGlowBlend)
	}
}

func (r *Renderer) drawSpellFxHeadOnShadow(screen *ebiten.Image, cx, cy, size, critBoost float64) {
	fc := float64(r.game.frameCount)
	void := [3]int{18, 8, 26}
	violet := [3]int{168, 88, 224}
	pale := [3]int{224, 170, 255}
	r.drawGlowSprite(screen, cx, cy, size*2.2*critBoost, violet, 0.52, additiveGlowBlend)
	for k := 0; k < 12; k++ {
		angle := fc*0.06 + 2*math.Pi*float64(k)/12
		r.drawGlowSprite(screen, cx+math.Cos(angle)*size*0.72, cy+math.Sin(angle)*size*0.72,
			size*0.25, pale, 0.62, additiveGlowBlend)
	}
	r.drawGlowSprite(screen, cx, cy, size*1.05, void, 0.98, ebiten.BlendSourceOver)
}

func (r *Renderer) drawSpellFxHeadOnVoidNeedle(screen *ebiten.Image, cx, cy, size, critBoost float64) {
	fc := float64(r.game.frameCount)
	void := [3]int{10, 6, 18}
	acid := [3]int{150, 255, 190}
	violet := [3]int{150, 70, 200}
	r.drawGlowSprite(screen, cx, cy, size*2.0*critBoost, violet, 0.32, additiveGlowBlend)
	for ring := 0; ring < 2; ring++ {
		radius := size * (0.68 + 0.5*float64(ring))
		for k := 0; k < 6; k++ {
			angle := fc*0.16*(1-2*float64(ring)) + 2*math.Pi*float64(k)/6
			r.drawGlowRectRotated(screen, cx+math.Cos(angle)*radius, cy+math.Sin(angle)*radius,
				size*0.3, size*0.1, angle, acid, 0.82, additiveGlowBlend)
		}
	}
	r.drawGlowRectRotated(screen, cx, cy, size*0.62, size*0.62, math.Pi/4, void, 1, ebiten.BlendSourceOver)
	r.drawGlowRect(screen, cx, cy, math.Max(2, size*0.16), acid, 0.95, additiveGlowBlend)
}

func (r *Renderer) drawSpellFxHeadOnPsiLance(screen *ebiten.Image, cx, cy, size, critBoost float64) {
	fc := float64(r.game.frameCount)
	psi := [3]int{186, 160, 255}
	hot := [3]int{242, 234, 255}
	deep := [3]int{92, 60, 180}
	for ring := 0; ring < 3; ring++ {
		phase := frac(fc*0.035 + float64(ring)/3)
		radius := size * (0.45 + phase*1.45)
		for k := 0; k < 12; k++ {
			angle := 2 * math.Pi * float64(k) / 12
			r.drawGlowRectRotated(screen, cx+math.Cos(angle)*radius, cy+math.Sin(angle)*radius,
				size*0.18, size*0.07, angle+math.Pi/2, psi, (1-phase)*0.45, additiveGlowBlend)
		}
	}
	r.drawGlowSprite(screen, cx, cy, size*1.6*critBoost, deep, 0.42, additiveGlowBlend)
	r.drawGlowRectRotated(screen, cx, cy, size*0.72, size*0.72, math.Pi/4, psi, 1, ebiten.BlendSourceOver)
	r.drawGlowRect(screen, cx, cy, math.Max(2, size*0.18), hot, 0.95, additiveGlowBlend)
}

func (r *Renderer) drawSpellFxHeadOnLash(screen *ebiten.Image, cx, cy, size, critBoost float64) {
	fc := float64(r.game.frameCount)
	ghost := [3]int{226, 236, 255}
	spirit := [3]int{182, 158, 255}
	deep := [3]int{104, 82, 180}
	prevX, prevY := cx, cy
	for k := 1; k <= 16; k++ {
		t := float64(k) / 16
		angle := fc*0.12 + t*math.Pi*3.4
		radius := size * 1.55 * t
		x := cx + math.Cos(angle)*radius
		y := cy + math.Sin(angle)*radius
		r.fxSegment(screen, prevX, prevY, x, y, math.Max(2, size*(0.18-0.12*t)),
			mixColor(ghost, spirit, t), (0.9-0.5*t)*critBoost, additiveGlowBlend)
		prevX, prevY = x, y
	}
	r.drawGlowSprite(screen, cx, cy, size*1.5*critBoost, deep, 0.35, additiveGlowBlend)
	r.drawGlowSprite(screen, cx, cy, size*0.5, ghost, 0.95, additiveGlowBlend)
}
