package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// weaponProjectileFxStyleDraw overlays a weapon-specific signature on top of
// the normal arrow or spell orb. The base projectile remains intact, preserving
// the readable silhouette shared by all ranged attacks.
var weaponProjectileFxStyleDraw = map[string]func(*Renderer, *ebiten.Image, float64, float64, float64, float64, float64, float64, int){
	"arena_recurve":  (*Renderer).drawWeaponProjectileFxArenaRecurve,
	"arena_arbalest": (*Renderer).drawWeaponProjectileFxArenaArbalest,
	"arena_lanista":  (*Renderer).drawWeaponProjectileFxArenaLanista,
	"clock_pistol":   (*Renderer).drawWeaponProjectileFxClockPistol,
}

func (r *Renderer) drawWeaponProjectileFx(style string, screen *ebiten.Image, cx, cy, size, dirX, dirY, critBoost float64, id int) {
	if draw, ok := weaponProjectileFxStyleDraw[style]; ok {
		draw(r, screen, cx, cy, size, dirX, dirY, critBoost, id)
	}
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
