package game

import (
	"math"
	"math/rand"
	"strings"

	"ugataima/internal/config"
	"ugataima/internal/spells"
)

// spawnWeaponBoltImpact spawns the impact burst for a ranged WEAPON projectile:
// a magical school burst for a staff/book (projectile_school set), otherwise the
// plain arrow puff. Single source for both the monster-hit and wall-hit paths so
// a staff never "explodes like an arrow".
func (g *MMGame) spawnWeaponBoltImpact(x, y float64, weaponDef *config.WeaponDefinitionConfig, count, size int, velX, velY float64) {
	if weaponDef != nil && weaponDef.ProjectileSchool != "" {
		g.CreateSpellHitEffect(x, y, strings.ToLower(weaponDef.ProjectileSchool), count, size)
		return
	}
	g.CreateArrowHitEffect(x, y, velX, velY)
}

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
		// Use the canonical damage formula (cost × SpellDamagePerSP) so the
		// visual scales follow the same balance lever as actual damage.
		if base := def.SpellPointsCost * spells.SpellDamagePerSP; base > 0 {
			damage = base
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

// spellHitStyle maps a damage element to an impact particle behaviour:
// fire → rising embers, water → falling ice shards, everything else → a plain
// radial burst. Keyed by school so it generalizes beyond the named spells.
func spellHitStyle(element string) string {
	switch strings.ToLower(element) {
	case "fire":
		return "ember"
	case "water":
		return "shard"
	default:
		return "burst"
	}
}

// CreateSpellHitEffect spawns a burst of colored particles at the impact point
func (g *MMGame) CreateSpellHitEffect(x, y float64, element string, particleCount, particleSize int) {
	g.hitEffectsMu.Lock()
	defer g.hitEffectsMu.Unlock()

	baseColor, ok := ElementColors[element]
	if !ok {
		baseColor = ElementColors["physical"]
	}

	if particleCount <= 0 {
		particleCount = SpellParticleCount
	}
	if particleSize <= 0 {
		particleSize = SpellParticleSize
	}
	style := spellHitStyle(element)
	// Bigger spells (larger particleSize, set from damage+radius) throw their
	// burst WIDER, not just denser — a fireball blast dwarfs a bolt's.
	spread := 1.0 + float64(particleSize-SpellParticleSize)*0.14
	if spread < 1 {
		spread = 1
	}
	if spread > 3.5 {
		spread = 3.5
	}
	particles := make([]SpellHitParticle, particleCount)

	for i := 0; i < particleCount; i++ {
		// Burst in ALL screen directions (a real 2D star, not a ground line):
		// VelX/VelY are screen-space, integrated into OffsetX/OffsetY each frame.
		angle := (float64(i)/float64(particleCount))*2*math.Pi + (rand.Float64()-0.5)*0.6
		speed := SpellParticleSpeed * (0.6 + rand.Float64()*0.8) * spread
		vx := math.Cos(angle) * speed
		vy := math.Sin(angle) * speed
		life := SpellParticleLife + rand.Intn(10) - 5
		grav := 0.0
		tint := baseColor

		switch style {
		case "ember": // fire: bias upward, drift up, hot tint, fade fast
			vy = vy*0.55 - (0.6 + rand.Float64()*1.0)
			grav = -0.05
			tint = mixColor(baseColor, [3]int{255, 240, 180}, rand.Float64()*0.55)
		case "shard": // ice: sharp outward shards that fall and linger
			vx *= 1.3
			vy *= 1.3
			grav = 0.14
			life += 8
			tint = mixColor(baseColor, [3]int{235, 245, 255}, rand.Float64()*0.5)
		}

		particleColor := [3]int{
			clampColor(tint[0] + rand.Intn(30) - 15),
			clampColor(tint[1] + rand.Intn(30) - 15),
			clampColor(tint[2] + rand.Intn(30) - 15),
		}

		particles[i] = SpellHitParticle{
			X:        x,
			Y:        y,
			VelX:     vx,
			VelY:     vy,
			Gravity:  grav,
			Color:    particleColor,
			LifeTime: life,
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

			// Integrate screen-space offsets; gravity pulls embers up / shards down.
			particle.OffsetX += particle.VelX
			particle.OffsetY += particle.VelY
			particle.VelX *= 0.94
			particle.VelY = particle.VelY*0.96 + particle.Gravity
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
