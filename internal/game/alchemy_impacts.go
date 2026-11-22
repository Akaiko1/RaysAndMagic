package game

import "math/rand"

// createFlaskImpact bursts a flask where it is drawn breaking.
func (g *MMGame) createFlaskImpact(p *FlaskProjectile, x, y float64, tint [3]int) {
	lx, ly, _, _ := p.Launch.renderMotion(g.combat, x, y, p.VelX, p.VelY)
	g.createSplashImpact(x, y, lx, ly, p.Radius, p.BurnFrames > 0, p.PoisonFrames > 0, tint)
}

// createSplashImpact paints a splash footprint of radius (px) at (x, y) with
// the same tile scatter as Earthquake and Starburst: burning flecks, poison
// drops or a harm spray, and a flash at (lightX, lightY). Damage already
// resolved once; every particle here is visual only.
func (g *MMGame) createSplashImpact(x, y, lightX, lightY, radius float64, burning, poisoned bool, tint [3]int) {
	tile := float64(g.config.GetTileSize())
	ground, height := g.groundOffsetY(), g.offsetYPerTileHeight()
	g.spawnTileScatterFx(x, y, radius/tile, func(wx, wy float64) []SpellHitParticle {
		parts := make([]SpellHitParticle, 0, 12)
		for i := 0; i < 12; i++ {
			// Each patch throws a small asymmetric spray off its own floor.
			// Keep dense colored matter legible against bright outdoor ground.
			life := g.framesBetween(.217, .342)
			particle := SpellHitParticle{
				X: wx, Y: wy,
				OffsetX: (rand.Float64() - .5) * tile * .55,
				OffsetY: ground - rand.Float64()*.05*height,
				VelX:    (rand.Float64() - .5) * 288,
				VelY:    -(192 + rand.Float64()*216),
				Gravity: 2016,
				Color:   mixColor(tint, [3]int{245, 228, 255}, rand.Float64()*.22),
				Size:    3 + rand.Intn(3),
				Solid:   true, DepthTest: true, Active: true,
			}
			switch {
			case burning:
				// Hot flecks rise from a short low flare on every affected tile.
				particle.VelX *= .4
				particle.VelY = -(84 + rand.Float64()*144)
				particle.Gravity = -360
				particle.Color = mixColor(tint, [3]int{255, 224, 102}, rand.Float64()*.8)
				life += g.framesForSeconds(.1)
				if i < 3 {
					particle.Size = 8
					particle.VelY *= .45
					particle.Color = tint
					life = g.framesBetween(.15, .233)
				}
			case poisoned:
				// Liquid drops fall; a few slower colored motes linger above them.
				particle.VelY *= .65
				particle.Gravity = 1440
				if i < 4 {
					particle.VelX *= .2
					particle.VelY = -(21.6 + rand.Float64()*36)
					particle.Gravity = -86.4
					particle.Size = 6 + rand.Intn(3)
					particle.Color = mixColor(tint, [3]int{33, 77, 20}, .35)
					life = g.framesBetween(.483, .633)
				}
			default:
				// Harm is a brisk concussive spray, with a few bright fragments.
				if i < 3 {
					particle.Size = 2
					particle.Color = mixColor(tint, [3]int{235, 240, 255}, .75)
					particle.VelX *= 1.4
				}
			}
			particle.LifeTime, particle.MaxLife = life, life
			parts = append(parts, particle)
		}
		return parts
	})
	g.addScreenShake(1.1, 1.1)
	// The flash bursts where the source is drawn breaking; the spray above
	// marks the physical damage footprint.
	g.hitEffectsMu.Lock()
	g.impactLights = append(g.impactLights, ImpactLight{X: lightX, Y: lightY, Radius: radius,
		Intensity: .6, Life: g.framesForSeconds(impactLightSeconds), MaxLife: g.framesForSeconds(impactLightSeconds)})
	g.hitEffectsMu.Unlock()
}
