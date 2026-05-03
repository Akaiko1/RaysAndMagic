package game

import (
	"math"
	"math/rand"

	"ugataima/internal/spells"
)

const (
	ArrowHitLifetime      = 30 // 0.5 seconds at 60fps
	ArrowHitParticleLife  = 12
	ArrowHitParticleSize  = 3
	ArrowHitParticleSpeed = 1.2

	SpellParticleCount = 8  // Base number of particles per spell hit
	SpellParticleLife  = 20 // ~0.33 seconds at 60fps
	SpellParticleSpeed = 2.0
	SpellParticleSize  = 4
)

// CreateArrowHitEffect spawns a short perpendicular particle burst at the impact point
func (g *MMGame) CreateArrowHitEffect(x, y, velX, velY float64) {
	particles := make([]ArrowHitParticle, 4)
	for i := 0; i < 4; i++ {
		speed := ArrowHitParticleSpeed * (0.85 + rand.Float64()*0.3)
		velX, velY := 0.0, 0.0
		switch i {
		case 0: // up
			velY = -speed
		case 1: // down
			velY = speed
		case 2: // left
			velX = -speed
		case 3: // right
			velX = speed
		}
		particles[i] = ArrowHitParticle{
			X:        x,
			Y:        y,
			OffsetX:  0,
			OffsetY:  0,
			VelX:     velX,
			VelY:     velY,
			LifeTime: ArrowHitParticleLife,
			MaxLife:  ArrowHitParticleLife,
			Size:     ArrowHitParticleSize,
			Active:   true,
			Color:    [3]int{140, 90, 50},
		}
	}

	effect := ArrowHitEffect{
		Particles: particles,
		Active:    true,
	}

	g.hitEffectsMu.Lock()
	g.arrowHitEffects = append(g.arrowHitEffects, effect)
	g.hitEffectsMu.Unlock()
}

// CreateSpellHitEffectFromSpell spawns spell hit particles scaled by base damage and hit radius.
func (g *MMGame) CreateSpellHitEffectFromSpell(x, y float64, spellID string) {
	def, err := spells.GetSpellDefinitionByID(spells.SpellID(spellID))
	element := "physical"
	damage := 1
	if err == nil {
		element = def.School
		if def.Damage > 0 {
			damage = def.Damage
		}
	}

	radiusTiles := 0.5
	if g.config != nil {
		if physics, err := g.config.GetSpellConfig(spellID); err == nil && physics != nil {
			radiusTiles = physics.CollisionSizeTiles
			if radiusTiles < 0.5 {
				radiusTiles = 0.5
			}
		}
	}

	particleCount := SpellParticleCount + damage + int(math.Round(radiusTiles*4))
	if particleCount < SpellParticleCount {
		particleCount = SpellParticleCount
	}
	if particleCount > 48 {
		particleCount = 48
	}

	particleSize := SpellParticleSize + int(math.Round(float64(damage)/4.0)) + int(math.Round(radiusTiles*2))
	if particleSize < 2 {
		particleSize = 2
	}

	g.CreateSpellHitEffect(x, y, element, particleCount, particleSize)
}

// CreateSpellHitEffect spawns a burst of colored particles at the impact point
func (g *MMGame) CreateSpellHitEffect(x, y float64, element string, particleCount, particleSize int) {
	g.hitEffectsMu.Lock()
	defer g.hitEffectsMu.Unlock()

	color, ok := ElementColors[element]
	if !ok {
		color = ElementColors["physical"]
	}

	if particleCount <= 0 {
		particleCount = SpellParticleCount
	}
	if particleSize <= 0 {
		particleSize = SpellParticleSize
	}
	particles := make([]SpellHitParticle, particleCount)

	for i := 0; i < particleCount; i++ {
		// Random angle for particle direction
		angle := (float64(i) / float64(particleCount)) * 2 * math.Pi
		angle += (rand.Float64() - 0.5) * 0.5 // Add some randomness

		speed := SpellParticleSpeed * (0.5 + rand.Float64()*0.5)

		// Slight color variation per particle
		particleColor := [3]int{
			clampColor(color[0] + rand.Intn(40) - 20),
			clampColor(color[1] + rand.Intn(40) - 20),
			clampColor(color[2] + rand.Intn(40) - 20),
		}

		particles[i] = SpellHitParticle{
			X:        x,
			Y:        y,
			VelX:     math.Cos(angle) * speed,
			VelY:     math.Sin(angle) * speed,
			Color:    particleColor,
			LifeTime: SpellParticleLife + rand.Intn(10) - 5,
			MaxLife:  SpellParticleLife,
			Size:     particleSize,
			Active:   true,
		}
	}

	effect := SpellHitEffect{
		Particles: particles,
		Active:    true,
	}

	g.spellHitEffects = append(g.spellHitEffects, effect)
}

// clampColor clamps a color value to 0-255
func clampColor(c int) int {
	if c < 0 {
		return 0
	}
	if c > 255 {
		return 255
	}
	return c
}

// UpdateHitEffects updates all hit effects (called from game loop)
func (g *MMGame) UpdateHitEffects() {
	// Update arrow hit effects (particle bursts)
	writeIdx := 0
	for i := range g.arrowHitEffects {
		effect := &g.arrowHitEffects[i]
		if !effect.Active {
			continue
		}

		activeParticles := 0
		for j := range effect.Particles {
			particle := &effect.Particles[j]
			if !particle.Active {
				continue
			}
			particle.OffsetX += particle.VelX
			particle.OffsetY += particle.VelY
			particle.LifeTime--

			if particle.LifeTime <= 0 {
				particle.Active = false
			} else {
				activeParticles++
			}
		}

		if activeParticles == 0 {
			effect.Active = false
			continue
		}

		g.arrowHitEffects[writeIdx] = *effect
		writeIdx++
	}
	g.arrowHitEffects = g.arrowHitEffects[:writeIdx]

	// Update spell hit effects
	writeIdx = 0
	for i := range g.spellHitEffects {
		effect := &g.spellHitEffects[i]
		if !effect.Active {
			continue
		}

		// Update particles
		activeParticles := 0
		for j := range effect.Particles {
			particle := &effect.Particles[j]
			if !particle.Active {
				continue
			}

			particle.X += particle.VelX
			particle.Y += particle.VelY
			particle.VelX *= 0.95 // Slow down
			particle.VelY *= 0.95
			particle.LifeTime--

			if particle.LifeTime <= 0 {
				particle.Active = false
			} else {
				activeParticles++
			}
		}

		if activeParticles == 0 {
			effect.Active = false
			continue
		}

		g.spellHitEffects[writeIdx] = *effect
		writeIdx++
	}
	g.spellHitEffects = g.spellHitEffects[:writeIdx]
}
