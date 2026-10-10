package game

import (
	"math"
	"math/rand"

	"ugataima/internal/config"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

// spawnWeaponBoltImpact spawns the impact effect for a ranged WEAPON projectile:
// a magical school burst for a staff/book (projectile_school set), a fire/element
// burst for an AoE bow (e.g. Bow of Hellfire), and nothing for a plain arrow -
// the arrow simply vanishes on hit. Single source for the monster- and wall-hit paths.
func (g *MMGame) spawnWeaponBoltImpact(x, y float64, weaponDef *config.WeaponDefinitionConfig, count, size int, target *monsterPkg.Monster3D) {
	if weaponDef == nil {
		return
	}
	if weaponDef.ProjectileSchool != "" {
		g.createSpellHitEffectStyled(x, y, normalizeDamageTypeStr(weaponDef.ProjectileSchool), count, size, false, visualAnchorFor(target), weaponImpactColor(weaponDef))
		return
	}
	// Explosive arrows (AoE bows, e.g. Bow of Hellfire) burst in their damage element.
	if weaponDef.AoeRadiusTiles > 0 {
		element := convertToMonsterDamageType(weaponDef.DamageType)
		if element == monsterPkg.DamagePhysical {
			element = monsterPkg.DamageFire
		}
		g.createSpellHitEffectStyled(x, y, element.String(), count, size, false, visualAnchorFor(target), weaponImpactColor(weaponDef))
	}
	// Plain arrow: no impact effect - it just disappears.
}

func weaponImpactColor(def *config.WeaponDefinitionConfig) [3]int {
	if def != nil && def.Graphics != nil {
		if def.Graphics.Color != [3]int{} {
			return def.Graphics.Color
		}
		if def.Graphics.SlashColor != [3]int{} {
			return def.Graphics.SlashColor
		}
	}
	if def != nil && def.ProjectileSchool != "" {
		if c, ok := ElementColors[normalizeDamageTypeStr(def.ProjectileSchool)]; ok {
			return c
		}
		return ElementColors["arcane"]
	}
	return [3]int{200, 200, 200}
}

const (
	SpellParticleCount = 8  // Base number of particles per spell hit
	SpellParticleLife  = 20 // ~0.17s at 120 TPS
	SpellParticleSpeed = 2.0
	SpellParticleSize  = 4
)

// CreateSpellHitEffectFromSpell spawns spell hit particles scaled by base damage and hit radius.
func (g *MMGame) CreateSpellHitEffectFromSpell(x, y float64, spellID string, target *monsterPkg.Monster3D) {
	def, err := spells.GetSpellDefinitionByID(spells.SpellID(spellID))
	element := monsterPkg.DamagePhysical.String()
	damage := 1
	if err == nil {
		element = def.School
		// Use the canonical damage formula (cost x SpellDamagePerSP) so the
		// visual scales follow the same balance lever as actual damage.
		if base := def.SpellPointsCost * spells.SpellDamagePerSP; base > 0 {
			damage = base
		}
	}

	radiusTiles := 0.5
	if g.config != nil {
		if physics, err := g.config.GetSpellConfig(spellID); err == nil && physics != nil {
			radiusTiles = g.config.ProjectileHitboxTiles(physics)
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

	// impact_stars spells scatter twinkling stars instead of mirror fragments.
	stars := false
	if cfgDef, ok := config.GetSpellDefinition(spellID); ok && cfgDef != nil && cfgDef.Graphics != nil {
		stars = cfgDef.Graphics.ImpactStars
	}
	rgb := ElementColors[normalizeDamageTypeStr(element)]
	if cfgDef, ok := config.GetSpellDefinition(spellID); ok && cfgDef.Graphics != nil {
		rgb = cfgDef.Graphics.Color
	}
	g.createSpellHitEffectStyled(x, y, element, particleCount, particleSize, stars, visualAnchorFor(target), rgb)

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
	switch convertToMonsterDamageType(element) {
	case monsterPkg.DamageFire:
		return "ember" // rising hot embers
	case monsterPkg.DamageWater:
		return "shard" // sharp shards that fall and linger
	case monsterPkg.DamageDark:
		return "void" // slow creeping motes that sink
	case monsterPkg.DamageLight:
		return "flash" // fast radiant flare, quick pop
	case monsterPkg.DamageAir:
		return "static" // air school is lightning/sparks: fast erratic crackle
	case monsterPkg.DamageEarth:
		return "rubble" // heavy chunks, strong drop
	case monsterPkg.DamageMind:
		return "spiral" // tangential swirl
	case monsterPkg.DamageSpirit:
		return "soul" // slow rising wisps, long-lived
	case monsterPkg.DamageBody:
		return "mend" // gentle drifting sparkles
	default:
		return "burst"
	}
}

// ImpactLight is a short-lived point light left where a spell lands - fed into
// the floor shader and sprite brightness, so impacts visibly flash the world.
type ImpactLight struct {
	X, Y          float64
	Radius        float64
	Intensity     float64
	Life, MaxLife int
}

// impactLightFrames is how long an impact flash lasts (intensity decays with life).
const impactLightFrames = 20 // ~0.17s at 120 TPS

// CreateSpellHitEffect spawns a burst of colored particles at the impact point
func (g *MMGame) CreateSpellHitEffect(x, y float64, element string, particleCount, particleSize int, target *monsterPkg.Monster3D) {
	g.createSpellHitEffectStyled(x, y, element, particleCount, particleSize, false, visualAnchorFor(target))
}

// createSpellHitEffectStyled is CreateSpellHitEffect with the star-shape flag
// (impact_stars): star bursts twinkle; other impacts shed eroding mirror shards.
func (g *MMGame) createSpellHitEffectStyled(x, y float64, element string, particleCount, particleSize int, stars bool, anchor monsterVisualAnchor, authored ...[3]int) {
	g.hitEffectsMu.Lock()
	defer g.hitEffectsMu.Unlock()

	element = normalizeDamageTypeStr(element)
	baseColor, ok := ElementColors[element]
	if !ok {
		baseColor = ElementColors[monsterPkg.DamagePhysical.String()]
	}

	if len(authored) > 0 && authored[0] != [3]int{} {
		baseColor = authored[0]
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
	// burst WIDER, not just denser - a fireball blast dwarfs a bolt's.
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
		// VelX/VelY use impact-plane units, projected at the anchor depth.
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
			vx *= 1.4 + rand.Float64()*1.4 // wildly uneven speeds -> spiky, not a round star
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

		// Reflective faces retain the source hue; brightness varies as they turn.
		if len(authored) > 0 && authored[0] != [3]int{} {
			tint = baseColor
		}
		brightness := .85 + rand.Float64()*.15
		particleColor := [3]int{int(float64(tint[0]) * brightness), int(float64(tint[1]) * brightness), int(float64(tint[2]) * brightness)}

		// Mirror fragments need a visible tumble and erosion tail. Keep authored
		// star flashes brief; this lifetime is presentation-only.
		if !stars {
			life += max(1, int(math.Round(float64(g.config.GetTPS())*.55)))
		}

		particles[i] = SpellHitParticle{
			X:        x,
			Y:        y,
			VelX:     vx,
			VelY:     vy,
			Gravity:  grav,
			Color:    particleColor,
			LifeTime: life,
			MaxLife:  life, // fade ratio uses LifeTime/MaxLife - must match the per-particle life
			Size:     particleSize,
			Star:     stars,
			Active:   true,
		}
	}

	effect := SpellHitEffect{
		Anchor:    anchor,
		Particles: particles,
		Active:    true,
	}
	if !stars {
		effect.BurstLife = max(1, int(math.Round(float64(g.config.GetTPS())*.32)))
		effect.BurstRadius = float64(particleSize)*4 + 6
		effect.BurstColor = baseColor
		effect.BurstDust = style != "ember" && style != "flash"
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

// spawnHitSparks is the "a hit landed here" burst, anchored on the monster's
// VISUAL position so sparks land where a pulled monster is drawn, not on its
// tile. Weapon blows, traps and damage zones share it.
func (cs *CombatSystem) spawnHitSparks(m *monsterPkg.Monster3D) {
	if m == nil {
		return
	}
	vx, vy := cs.monsterVisualPos(m)
	cs.game.spawnImpactSparks(vx, vy, m)
}

// spawnWeaponHitImpactFX adds the damage-scaled view kick to the sparks. Only a
// blow the party lands kicks the camera - a field ticking every second must not.
func (cs *CombatSystem) spawnWeaponHitImpactFX(m *monsterPkg.Monster3D, damage int, weapon *config.WeaponDefinitionConfig) {
	x, y := cs.monsterVisualPos(m)
	cs.game.spawnImpactSparks(x, y, m, weaponImpactColor(weapon))
	cs.game.addScreenShake(0.05*float64(damage), 2.2)
}

// spawnImpactSparks throws a quick radial burst of bright white->gold sparks at
// a world point - the weapon-hit feedback when the party strikes a monster.
func (g *MMGame) spawnImpactSparks(x, y float64, target *monsterPkg.Monster3D, authored ...[3]int) {
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
	base := [3]int{255, 210, 110}
	if len(authored) > 0 {
		base = authored[0]
	}
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
			Color:    mixColor(base, [3]int{}, rand.Float64()*.2),
			LifeTime: life, MaxLife: life, Size: 5, Active: true,
		}
	}
	g.spellHitEffects = append(g.spellHitEffects, SpellHitEffect{Anchor: visualAnchorFor(target), Active: true, Particles: parts})
}

// tileScatterMaxTiles caps how many tiles one area FX paints. A wide spell
// (Earthquake reaches 8 tiles = ~200 tiles) would otherwise spawn a thousand
// particles in a frame; past the cap the disc is thinned evenly so the effect
// still reads out to its full radius.
const tileScatterMaxTiles = 84

// spawnTileScatterFx calls perTile once per tile centre within radiusTiles of
// (cx,cy) and files what it returns as one effect. The disc walk, the radius
// clip, the tile budget and the lock live here - a caller only authors what one
// patch of ground throws up. Shared by every "AoE that paints the ground it
// covers" (Starburst's falling stars, Earthquake's rubble).
func (g *MMGame) spawnTileScatterFx(cx, cy, radiusTiles float64, perTile func(wx, wy float64) []SpellHitParticle) {
	g.spawnTileScatterFxWhere(cx, cy, radiusTiles, nil, perTile)
}

// Filter before the tile budget so inaccessible ground cannot crowd out visible FX.
func (g *MMGame) spawnTileScatterFxWhere(cx, cy, radiusTiles float64, allowed func(wx, wy float64) bool, perTile func(wx, wy float64) []SpellHitParticle) {
	tile := float64(g.config.GetTileSize())
	reach := radiusTiles * tile
	r := int(radiusTiles + 0.999)
	ctx := TileIndex(cx, tile)
	cty := TileIndex(cy, tile)

	type tilePos struct {
		wx, wy    float64
		distTiles float64
	}
	covered := make([]tilePos, 0, (2*r+1)*(2*r+1))
	for ty := cty - r; ty <= cty+r; ty++ {
		for tx := ctx - r; tx <= ctx+r; tx++ {
			wx := (float64(tx) + 0.5) * tile
			wy := (float64(ty) + 0.5) * tile
			d := math.Hypot(wx-cx, wy-cy)
			if d > reach || (allowed != nil && !allowed(wx, wy)) {
				continue
			}
			covered = append(covered, tilePos{wx, wy, d / tile})
		}
	}

	// Over budget, thin by 1/distance rather than evenly: a disc has far more
	// far tiles than near ones, so an even cull empties the foreground - where
	// the player is looking - to spend the budget on specks at the horizon.
	// Keeping ~keepRing tiles per ring holds the near ground dense.
	keepRing := 1.0
	if len(covered) > tileScatterMaxTiles && radiusTiles > 0 {
		keepRing = float64(tileScatterMaxTiles) / (2 * math.Pi * radiusTiles)
	}

	selected := covered
	if len(covered) > tileScatterMaxTiles {
		selected = make([]tilePos, 0, len(covered))
		for _, t := range covered {
			if t.distTiles <= keepRing || rand.Float64() <= keepRing/t.distTiles {
				selected = append(selected, t)
			}
		}
		// Bernoulli thinning preserves the desired near-to-far density but only
		// controls the expected count. Shuffle any statistical overshoot before
		// truncating so iteration order cannot bias the kept patch of ground.
		if len(selected) > tileScatterMaxTiles {
			rand.Shuffle(len(selected), func(i, j int) {
				selected[i], selected[j] = selected[j], selected[i]
			})
			selected = selected[:tileScatterMaxTiles]
		}
	}

	g.hitEffectsMu.Lock()
	defer g.hitEffectsMu.Unlock()
	for _, t := range selected {
		if particles := perTile(t.wx, t.wy); len(particles) > 0 {
			g.spellHitEffects = append(g.spellHitEffects, SpellHitEffect{Particles: particles, Active: true})
		}
	}
}

// spawnStarburstFx drops a small star into every tile within `radiusTiles` of
// the impact point: each star is a cluster of bright particles that begins above
// the tile and falls into it (Starburst). Purely visual - damage is handled by
// the spell's AoE splash.
func (g *MMGame) spawnStarburstFx(cx, cy, radiusTiles float64) {
	star := [3]int{235, 240, 255} // bright star-white
	g.spawnTileScatterFx(cx, cy, radiusTiles, func(wx, wy float64) []SpellHitParticle {
		particles := make([]SpellHitParticle, 0, 6)
		for i := 0; i < 6; i++ {
			tint := mixColor(star, [3]int{255, 230, 140}, rand.Float64()*0.5) // white->gold sparkle
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
		return particles
	})
}

// Nova FX: the ground effect a non-projectile AoE paints over its reach. Same
// shape as the projectile_fx registry (renderer func + registry entry + one
// yaml line) so a nova spell gets its signature without a name check in code.
var novaFxSpawn = map[string]func(g *MMGame, cx, cy, radiusTiles float64){
	"quake": (*MMGame).spawnQuakeFx,
}

// mapWideNovaFxRadiusTiles is how far a map_wide nova paints its ground FX: the
// damage covers the region, the visuals only need to cover what the party can
// see from where they stand.
const mapWideNovaFxRadiusTiles = 10.0

// spawnNovaFx plays the style authored in the spell's graphics.nova_fx. Spells
// without one keep the plain per-monster impact bursts.
func (g *MMGame) spawnNovaFx(spellID string, cx, cy, radiusTiles float64) {
	def, ok := config.GetSpellDefinition(spellID)
	if !ok || def == nil || def.Graphics == nil {
		return
	}
	if spawn := novaFxSpawn[def.Graphics.NovaFx]; spawn != nil {
		spawn(g, cx, cy, radiusTiles)
	}
}

// quakeShakeAmp is a MODERATE rumble - roughly half the spell cap
// (screenShakeMaxAmp), so the ground heaving is felt without the view snapping
// the way a fireball hit does.
const quakeShakeAmp = 2.6

// groundOffsetY is the particle OffsetY that sits ON THE FLOOR, and
// offsetYPerTileHeight is how much OffsetY one tile of height is worth.
//
// A ground particle draws at viewHorizon + OffsetY*viewFocal/(depth*camera.FOV),
// while the floor is viewHorizon + 0.5*viewFocal*tileSize/depth. The authored
// offset units retain the logical FOV in either view mode; focal length and
// depth cancel. OffsetY 0 is the horizon when no body anchor is attached.
func (g *MMGame) groundOffsetY() float64 {
	return 0.5 * float64(g.config.GetTileSize()) * g.camera.FOV
}

func (g *MMGame) offsetYPerTileHeight() float64 {
	return float64(g.config.GetTileSize()) * g.camera.FOV
}

// spawnQuakeFx shakes the view and kicks a shower of earth off every patch of
// ground the quake reaches: each clod jumps off the floor, arcs, and drops back
// into it.
func (g *MMGame) spawnQuakeFx(cx, cy, radiusTiles float64) {
	g.addScreenShake(quakeShakeAmp, quakeShakeAmp)
	// The spell's nova is not visibility-gated; preserve its area policy.
	g.spawnQuakeGroundFxWhere(cx, cy, radiusTiles, nil)
}

// spawnQuakeGroundFx shares the rubble and dust without shaking the camera.
// Frequent movement procs use this presentation; the spell also adds rumble.
func (g *MMGame) spawnQuakeGroundFx(cx, cy, radiusTiles float64) {
	g.spawnQuakeGroundFxWhere(cx, cy, radiusTiles, func(wx, wy float64) bool {
		return g.attackLineClear(cx, cy, wx, wy)
	})
}

func (g *MMGame) spawnQuakeGroundFxWhere(cx, cy, radiusTiles float64, allowed func(wx, wy float64) bool) {
	soil := mixColor(ElementColors[monsterPkg.DamageEarth.String()], [3]int{40, 26, 14}, 0.45) // dark turned earth
	dust := [3]int{150, 126, 92}                                                               // dry dust off the same ground
	ground := g.groundOffsetY()
	perTileHeight := g.offsetYPerTileHeight()
	g.spawnTileScatterFxWhere(cx, cy, radiusTiles, allowed, func(wx, wy float64) []SpellHitParticle {
		clods := 4 + rand.Intn(3)
		particles := make([]SpellHitParticle, 0, clods+1)
		for i := 0; i < clods; i++ {
			life := SpellParticleLife + rand.Intn(16)
			// Thrown a third of a tile up at most, then gravity drops it back
			// into the floor it came off.
			hop := (0.16 + rand.Float64()*0.20) * perTileHeight
			gravity := 0.19
			particles = append(particles, SpellHitParticle{
				X:        wx,
				Y:        wy,
				OffsetX:  (rand.Float64() - 0.5) * 26,
				OffsetY:  ground - rand.Float64()*3, // sits on the floor, not at eye level
				VelX:     (rand.Float64() - 0.5) * 1.2,
				VelY:     -math.Sqrt(2 * gravity * hop), // up; OffsetY grows downward
				Gravity:  gravity,
				Color:    mixColor(soil, dust, rand.Float64()*0.5),
				LifeTime: life,
				MaxLife:  life,
				Size:     SpellParticleSize - 1 + rand.Intn(3),
				Solid:    true, // a clod of earth, not a spark
				Active:   true,
			})
		}
		// One slow dust puff per patch, hanging where the ground broke.
		life := SpellParticleLife + 18 + rand.Intn(10)
		particles = append(particles, SpellHitParticle{
			X:        wx,
			Y:        wy,
			OffsetX:  (rand.Float64() - 0.5) * 14,
			OffsetY:  ground - 0.06*perTileHeight,
			VelX:     (rand.Float64() - 0.5) * 0.4,
			VelY:     -(0.25 + rand.Float64()*0.35),
			Gravity:  0.012,
			Color:    mixColor(dust, [3]int{198, 180, 150}, rand.Float64()),
			LifeTime: life,
			MaxLife:  life,
			Size:     SpellParticleSize + 2,
			Solid:    true,
			Active:   true,
		})
		return particles
	})
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
	// in-place compaction below - never mutate g.spellHitEffects mid-iteration).
	var trail []SpellHitEffect

	// Update spell hit effects
	writeIdx := 0
	for i := range g.spellHitEffects {
		effect := &g.spellHitEffects[i]
		if !effect.Active {
			continue
		}

		effect.BurstAge++

		// Update particles
		activeParticles := 0
		for j := range effect.Particles {
			particle := &effect.Particles[j]
			if !particle.Active {
				continue
			}

			// Integrate impact-plane offsets; projection applies the anchor depth.
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
					trail = append(trail, SpellHitEffect{Anchor: effect.Anchor, Active: true, Particles: []SpellHitParticle{{
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
