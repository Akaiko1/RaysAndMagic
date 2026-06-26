package game

import (
	"math"
	"math/rand"
	"strings"

	"ugataima/internal/config"
	"ugataima/internal/spells"
)

// spawnWeaponBoltImpact spawns the impact effect for a ranged WEAPON projectile:
// a magical school burst for a staff/book (projectile_school set), a fire/element
// burst for an AoE bow (e.g. Bow of Hellfire), and nothing for a plain arrow —
// the arrow simply vanishes on hit. Single source for the monster- and wall-hit paths.
func (g *MMGame) spawnWeaponBoltImpact(x, y float64, weaponDef *config.WeaponDefinitionConfig, count, size int) {
	if weaponDef == nil {
		return
	}
	if weaponDef.ProjectileSchool != "" {
		g.CreateSpellHitEffect(x, y, strings.ToLower(weaponDef.ProjectileSchool), count, size)
		return
	}
	// Explosive arrows (AoE bows, e.g. Bow of Hellfire) burst in their damage element.
	if weaponDef.AoeRadiusTiles > 0 {
		el := strings.ToLower(weaponDef.DamageType)
		if el == "" || el == "physical" {
			el = "fire"
		}
		g.CreateSpellHitEffect(x, y, el, count, size)
	}
	// Plain arrow: no impact effect — it just disappears.
}

const (
	SpellParticleCount = 8  // Base number of particles per spell hit
	SpellParticleLife  = 20 // ~0.33 seconds at 60fps
	SpellParticleSpeed = 2.0
	SpellParticleSize  = 4
)

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

	// Per-pixel chunkiness tracks the spell's BLAST (radius) more than a flat
	// floor, so a weak, tight bolt (radius 0.5) reads as fine sparks even at
	// range, while a wide AoE (fireball, radius 2.0) keeps fat embers. The old
	// flat base (4) pinned bolts to ~5-7px and made them look chunky on impact.
	particleSize := 2 + int(math.Round(float64(damage)/5.0)) + int(math.Round(radiusTiles*3))
	if particleSize < 2 {
		particleSize = 2
	}

	g.CreateSpellHitEffect(x, y, element, particleCount, particleSize)

	// Heavy spells rattle the view: shake amplitude follows the same damage +
	// blast levers as the particles, so a bolt barely taps and a fireball kicks.
	g.addScreenShake(0.10*float64(damage)+radiusTiles, screenShakeMaxAmp)
}

// screenShakeMaxAmp caps the camera shake in world units (~1/12 tile).
const screenShakeMaxAmp = 5.0

// addScreenShake raises the camera shake to amp (never lowers it), bounded by
// the given cap so stacked hits can't wind the view up indefinitely.
func (g *MMGame) addScreenShake(amp, maxAmp float64) {
	if amp > maxAmp {
		amp = maxAmp
	}
	if amp > g.screenShake {
		g.screenShake = amp
	}
}

// spellHitStyle maps a damage element to an impact particle behaviour, so each
// school reads distinct by MOTION, not just colour. Keyed by school so it
// generalizes beyond the named spells; unknown elements fall back to a plain
// radial burst.
func spellHitStyle(element string) string {
	switch strings.ToLower(element) {
	case "fire":
		return "ember" // rising hot embers
	case "water":
		return "shard" // sharp shards that fall and linger
	case "dark":
		return "void" // slow creeping motes that sink
	case "light":
		return "flash" // fast radiant flare, quick pop
	case "air":
		return "static" // air school is lightning/sparks: fast erratic crackle
	case "earth":
		return "rubble" // heavy chunks, strong drop
	case "mind":
		return "spiral" // tangential swirl
	case "spirit":
		return "soul" // slow rising wisps, long-lived
	case "body":
		return "mend" // gentle drifting sparkles
	default:
		return "burst"
	}
}

// ImpactLight is a short-lived point light left where a spell lands — fed into
// the floor shader and sprite brightness, so impacts visibly flash the world.
type ImpactLight struct {
	X, Y          float64
	Radius        float64
	Intensity     float64
	Life, MaxLife int
}

// impactLightFrames is how long an impact flash lasts (intensity decays with life).
const impactLightFrames = 20

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

	// Impact flash: a light pool under the burst, sized with the burst itself.
	g.impactLights = append(g.impactLights, ImpactLight{
		X: x, Y: y,
		Radius:    float64(g.config.GetTileSize()) * (1.8 + 0.02*float64(particleCount)),
		Intensity: 0.7,
		Life:      impactLightFrames, MaxLife: impactLightFrames,
	})
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
		case "void": // dark: slow, soft motes that creep outward, sink and linger
			vx *= 0.7
			vy = vy*0.7 + 0.3
			grav = 0.05
			life += 6
			tint = mixColor(baseColor, [3]int{190, 120, 255}, rand.Float64()*0.55)
		case "flash": // light: a bright, fast radiant flare that pops out and fades quickly
			vx *= 1.5
			vy *= 1.5
			life -= 4
			tint = mixColor(baseColor, [3]int{255, 255, 235}, rand.Float64()*0.6)
		case "static": // air = lightning/sparks: jagged electric crackle, gone in a snap
			vx *= 1.4 + rand.Float64()*1.4 // wildly uneven speeds → spiky, not a round star
			vy *= 1.4 + rand.Float64()*1.4
			life -= 6
			tint = mixColor(baseColor, [3]int{255, 255, 255}, rand.Float64()*0.7)
		case "rubble": // earth: heavy chunks thrown low, dropping hard
			vx *= 1.1
			vy = vy*0.5 + 0.4
			grav = 0.22
			life += 4
			tint = mixColor(baseColor, [3]int{170, 140, 90}, rand.Float64()*0.5)
		case "spiral": // mind: tangential swirl instead of a radial burst
			vx, vy = -vy*1.2, vx*1.2
			life += 4
			tint = mixColor(baseColor, [3]int{210, 230, 255}, rand.Float64()*0.5)
		case "soul": // spirit: slow wisps that float up and linger
			vx *= 0.5
			vy = vy*0.4 - (0.5 + rand.Float64()*0.8)
			grav = -0.02
			life += 12
			tint = mixColor(baseColor, [3]int{235, 225, 255}, rand.Float64()*0.6)
		case "mend": // body: gentle sparkles drifting upward, soft and brief
			vx *= 0.6
			vy = vy*0.5 - (0.3 + rand.Float64()*0.5)
			grav = -0.01
			tint = mixColor(baseColor, [3]int{220, 255, 220}, rand.Float64()*0.5)
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
			MaxLife:  life, // fade ratio uses LifeTime/MaxLife — must match the per-particle life
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

// spawnBlinkLightColumn leaves a tall, slowly-fading pillar of light where a
// monster blinked away.
func (g *MMGame) spawnBlinkLightColumn(x, y float64) {
	g.hitEffectsMu.Lock()
	defer g.hitEffectsMu.Unlock()
	const n = 22
	parts := make([]SpellHitParticle, n)
	for i := 0; i < n; i++ {
		life := 70 + rand.Intn(30)
		parts[i] = SpellHitParticle{
			X: x, Y: y,
			OffsetX:  (rand.Float64() - 0.5) * 10,
			OffsetY:  -rand.Float64() * 130,
			VelX:     (rand.Float64() - 0.5) * 0.3,
			VelY:     -(0.4 + rand.Float64()*0.7),
			Gravity:  -0.01,
			Color:    mixColor([3]int{255, 250, 210}, [3]int{255, 215, 120}, rand.Float64()),
			LifeTime: life, MaxLife: life, Size: 6, Active: true,
		}
	}
	g.spellHitEffects = append(g.spellHitEffects, SpellHitEffect{Active: true, Particles: parts})
}

// spawnImpactSparks throws a quick radial burst of bright white→gold sparks at
// a world point — the weapon-hit feedback when the party strikes a monster.
func (g *MMGame) spawnImpactSparks(x, y float64) {
	g.hitEffectsMu.Lock()
	defer g.hitEffectsMu.Unlock()

	// A short flash at the struck target (smaller and briefer than a spell's),
	// so melee hits light the world too.
	g.impactLights = append(g.impactLights, ImpactLight{
		X: x, Y: y,
		Radius:    float64(g.config.GetTileSize()) * 1.2,
		Intensity: 0.5,
		Life:      impactLightFrames * 2 / 3, MaxLife: impactLightFrames * 2 / 3,
	})
	const n = 14
	parts := make([]SpellHitParticle, n)
	for i := 0; i < n; i++ {
		ang := rand.Float64() * 2 * math.Pi
		sp := 2.8 + rand.Float64()*3.2
		life := 11 + rand.Intn(7)
		parts[i] = SpellHitParticle{
			X: x, Y: y,
			VelX:     math.Cos(ang) * sp,
			VelY:     math.Sin(ang)*sp - 0.8, // slight upward bias
			Gravity:  0.11,
			Color:    mixColor([3]int{255, 255, 210}, [3]int{255, 200, 80}, rand.Float64()),
			LifeTime: life, MaxLife: life, Size: 5, Active: true,
		}
	}
	g.spellHitEffects = append(g.spellHitEffects, SpellHitEffect{Active: true, Particles: parts})
}

// spawnStarburstFx drops a small star into every tile within `radiusTiles` of
// the impact point: each star is a cluster of bright particles that begins above
// the tile and falls into it (Starburst). Purely visual — damage is handled by
// the spell's AoE splash.
func (g *MMGame) spawnStarburstFx(cx, cy, radiusTiles float64) {
	g.hitEffectsMu.Lock()
	defer g.hitEffectsMu.Unlock()

	tile := float64(g.config.GetTileSize())
	reach := radiusTiles * tile
	r := int(radiusTiles + 0.999)
	ctx := int(cx / tile)
	cty := int(cy / tile)
	star := [3]int{235, 240, 255} // bright star-white

	for ty := cty - r; ty <= cty+r; ty++ {
		for tx := ctx - r; tx <= ctx+r; tx++ {
			wx := (float64(tx) + 0.5) * tile
			wy := (float64(ty) + 0.5) * tile
			if math.Hypot(wx-cx, wy-cy) > reach {
				continue
			}
			particles := make([]SpellHitParticle, 0, 6)
			for i := 0; i < 6; i++ {
				tint := mixColor(star, [3]int{255, 230, 140}, rand.Float64()*0.5) // white→gold sparkle
				life := SpellParticleLife + rand.Intn(8)
				particles = append(particles, SpellHitParticle{
					X:        wx,
					Y:        wy,
					OffsetX:  (rand.Float64() - 0.5) * 8,
					OffsetY:  -36 - rand.Float64()*28, // start above the tile
					VelX:     (rand.Float64() - 0.5) * 0.8,
					VelY:     2.6 + rand.Float64()*1.6, // fall down into the tile
					Gravity:  0.12,
					Color:    tint,
					LifeTime: life,
					MaxLife:  life,
					Size:     SpellParticleSize,
					Trail:    true, // leaves a slowly-evaporating streak as it falls
					Active:   true,
				})
			}
			g.spellHitEffects = append(g.spellHitEffects, SpellHitEffect{Particles: particles, Active: true})
		}
	}
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
	// Trail breadcrumbs spawned this frame (collected, then appended AFTER the
	// in-place compaction below — never mutate g.spellHitEffects mid-iteration).
	var trail []SpellHitEffect

	// Update spell hit effects
	writeIdx := 0
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
				// Falling-star trail: drop a faint, motionless breadcrumb at the
				// current position every few frames; it lingers and fades on its
				// own ("slowly evaporates"). Breadcrumbs don't trail themselves.
				if particle.Trail && particle.LifeTime%3 == 0 {
					sz := particle.Size - 1
					if sz < 1 {
						sz = 1
					}
					trail = append(trail, SpellHitEffect{Active: true, Particles: []SpellHitParticle{{
						X: particle.X, Y: particle.Y,
						OffsetX: particle.OffsetX, OffsetY: particle.OffsetY,
						Color: particle.Color, LifeTime: 14, MaxLife: 14, Size: sz, Active: true,
					}}})
				}
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
	if len(trail) > 0 {
		g.spellHitEffects = append(g.spellHitEffects, trail...)
	}
}
