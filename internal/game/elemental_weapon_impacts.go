package game

import (
	"math"
	"ugataima/internal/monster"
)

// Ability activation feedback is distinct from the weapon's ordinary attack.
// These particles use the shared presentation clock and wall-depth clipping.
func (g *MMGame) createElementalAbilityImpact(x, y float64, school string, target *monster.Monster3D) {
	fx := SpellHitEffect{Active: true, Anchor: visualAnchorFor(target)}
	life := max(36, int(math.Round(float64(g.config.GetTPS())*.8)))
	for i := 0; i < 18; i++ {
		a := float64(i) * math.Pi * 2 / 18
		p := SpellHitParticle{X: x, Y: y, LifeTime: life, MaxLife: life, Size: 4, DepthTest: true, Active: true}
		switch school {
		case "fire":
			// Six narrow pressure jets, each with three consecutive sparks.
			a = float64(i%6) * math.Pi / 3
			speed := 2 + float64(i/6)
			p.VelX, p.VelY = math.Cos(a)*speed, math.Sin(a)*speed
			p.Color = solsticeFire
		case "water":
			// A collapsing oval concentrates the travelling charge.
			p.OffsetX, p.OffsetY = math.Cos(a)*22, math.Sin(a)*12
			p.VelX, p.VelY = -p.OffsetX*.12, -p.OffsetY*.12
			p.Color = solsticeWater
		case "earth":
			p.OffsetX = float64(i%6-3) * 6
			p.VelX, p.VelY = float64(i%3-1)*.7, -1-float64(i/6)
			p.Gravity, p.Solid, p.Color = .22, true, solsticeEarth
		case "air":
			// Two displaced diagonal seams recoil in opposite directions.
			side := 1.
			if i%2 == 0 {
				side = -1
			}
			p.OffsetX, p.OffsetY = float64(i/2-4)*5, float64(4-i/2)*3+side*9
			p.VelX, p.VelY = side*2, -side
			p.Color = solsticeAir
		}
		fx.Particles = append(fx.Particles, p)
	}
	g.spellHitEffects = append(g.spellHitEffects, fx)
}
