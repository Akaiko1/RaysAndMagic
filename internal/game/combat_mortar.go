package game

import (
	"fmt"
	"math"

	"ugataima/internal/character"
	"ugataima/internal/spells"
)

// pendingMortar is a Stone Blossom shot in flight: the arc ignores everything,
// so the detonation point and payload are fixed at cast time and simply fire
// when the flight timer runs out. Transient (not saved) - a mid-flight save
// drops the bloom, like other in-flight projectiles.
type pendingMortar struct {
	X, Y        float64 // landing point (pixels)
	FramesLeft  int
	SpellID     string
	Damage      int
	Crit        bool
	Caster      *character.MMCharacter
	RadiusTiles float64
	StunSeconds int
	StunTurns   int
	School      string
}

// castMortarSpell lobs a mortar projectile (Stone Blossom): a purely visual
// bolt flies the arc while the real detonation is scheduled for the landing
// tile, exactly MortarRangeTiles out. Nothing in flight can intercept it.
func (cs *CombatSystem) castMortarSpell(spellID spells.SpellID, spellDef spells.SpellDefinition, caster *character.MMCharacter, announce bool) bool {
	tileSize := float64(cs.game.config.GetTileSize())
	dist := spellDef.MortarRangeTiles * tileSize
	dirX, dirY := math.Cos(cs.game.camera.Angle), math.Sin(cs.game.camera.Angle)
	landX := cs.game.camera.X + dirX*dist
	landY := cs.game.camera.Y + dirY*dist

	_, _, totalDamage := cs.CalculateSpellDamage(spellID, caster)
	totalDamage, isCrit := cs.rollSpellCritDamage(spellID, caster, totalDamage)

	// Flight time from the spell's authored projectile speed (validateSpellAuthoring
	// guarantees a mortar spell has physics.speed_tiles > 0); a wall may clip the
	// visual bolt early, but the arc abstraction lands the bloom regardless.
	speed := spellDef.MortarRangeTiles // 1 tile/frame fallback if physics is ever absent
	if physics, err := cs.game.config.GetSpellConfig(string(spellID)); err == nil && physics.SpeedTiles > 0 {
		speed = physics.SpeedTiles
	}
	frames := int(spellDef.MortarRangeTiles / speed * float64(cs.game.config.GetTPS()))
	if frames < 1 {
		frames = 1
	}

	cs.spawnMortarVisual(spellID, frames)
	cs.game.pendingMortars = append(cs.game.pendingMortars, pendingMortar{
		X: landX, Y: landY,
		FramesLeft:  frames,
		SpellID:     string(spellID),
		Damage:      totalDamage,
		Crit:        isCrit,
		Caster:      caster,
		RadiusTiles: spellDef.AoeRadiusTiles,
		StunSeconds: spellDef.StunDurationSeconds,
		StunTurns:   spellDef.StunDurationTurns,
		School:      spellDef.School,
	})
	if announce {
		cs.game.AddCombatMessage(fmt.Sprintf("Casting %s!", spellDef.Name))
	}
	return true
}

// spawnMortarVisual launches the non-colliding display bolt that traces the
// mortar's path (no collision entity: nothing may intercept the arc).
func (cs *CombatSystem) spawnMortarVisual(spellID spells.SpellID, frames int) {
	castingSystem := spells.NewCastingSystem(cs.game.config)
	projectile, err := castingSystem.CreateProjectile(spellID, cs.game.camera.X, cs.game.camera.Y, cs.game.camera.Angle)
	if err != nil {
		return
	}
	mp := MagicProjectile{
		ID:        cs.game.GenerateProjectileID(string(spellID)),
		X:         projectile.X,
		Y:         projectile.Y,
		VelX:      projectile.VelX,
		VelY:      projectile.VelY,
		LifeTime:  frames,
		Active:    true,
		SpellType: string(spellID),
		Size:      projectile.Size,
		NoCollide: true,
		Owner:     ProjectileOwnerPlayer,
	}
	cs.game.magicProjectiles = append(cs.game.magicProjectiles, mp)
}

// tickPendingMortars advances every scheduled bloom and detonates the ripe
// ones. Runs once per frame in both RT and TB (projectiles fly in both).
func (g *MMGame) tickPendingMortars() {
	if len(g.pendingMortars) == 0 {
		return
	}
	kept := g.pendingMortars[:0]
	for i := range g.pendingMortars {
		m := &g.pendingMortars[i]
		m.FramesLeft--
		if m.FramesLeft > 0 {
			kept = append(kept, *m)
			continue
		}
		g.combat.detonateMortar(*m)
	}
	g.pendingMortars = kept
}

// detonateMortar blooms at the landing point: every monster within the radius
// takes the payload (its own armor + resist apply) and is stunned like
// Darkness stuns. The party is never caught - the caster aimed it away.
func (cs *CombatSystem) detonateMortar(m pendingMortar) {
	def, err := spells.GetSpellDefinitionByID(spells.SpellID(m.SpellID))
	name := m.SpellID
	if err == nil {
		name = def.Name
	}
	damageTypeStr := normalizeDamageTypeStr(m.School)
	damageType := convertToMonsterDamageType(damageTypeStr)
	dmg := m.Damage + cs.game.combatBuffOutBonusForDamageType(damageTypeStr)
	radius := m.RadiusTiles * float64(cs.game.config.GetTileSize())
	resistPierce := cs.spellResistPierce(m.Caster, m.SpellID)

	cs.game.spawnStarburstFx(m.X, m.Y, m.RadiusTiles)
	cs.game.AddCombatMessage(fmt.Sprintf("%s blooms!", name))
	for _, target := range cs.game.world.Monsters {
		if target == nil || !target.IsAlive() || bossInvulnerable(target) {
			continue
		}
		if Distance(m.X, m.Y, target.X, target.Y) > radius {
			continue
		}
		reduced := applyMonsterArmor(dmg, damageTypeStr, target.EffectiveArmorClass(), false)
		actual := target.TakeDamageResist(reduced, damageType, resistPierce)
		cs.markMonsterHit(target)
		cs.spawnMonsterHitBurst(target, damageTypeStr)
		if !target.IsAlive() {
			xpAwarded := cs.finishMonsterKill(target)
			cs.game.AddCombatMessage(fmt.Sprintf("%s crushes %s! (+%d XP)", name, target.Name, xpAwarded))
			continue
		}
		cs.game.AddCombatMessage(fmt.Sprintf("%s crushes %s for %d damage.", name, target.Name, actual))
		cs.applyStun(target, m.StunSeconds, m.StunTurns)
	}
}
