package game

import (
	"cmp"
	"math"
	"math/rand"
	"slices"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

// advancedSpellTarget selects the explicitly aimed enemy, or the nearest eligible
// victim in the forward cone; an ineligible monster in front never shadows a
// valid target. Range and walls are checked even for explicit targeting.
func (cs *CombatSystem) advancedSpellTarget(radius, coneDegrees float64, eligible func(*monster.Monster3D) bool) *monster.Monster3D {
	x, y := cs.logicalCameraXY()
	var result *monster.Monster3D
	best := radius
	for _, m := range cs.game.world.Monsters {
		if !advancedSpellVictim(m) || !eligible(m) || !cs.attackLineClear(x, y, m.X, m.Y) {
			continue
		}
		d := Distance(x, y, m.X, m.Y)
		if d > radius {
			continue
		}
		if cs.partyAimTarget != nil {
			if m == cs.partyAimTarget {
				return m
			}
			continue
		}
		if !cs.inPartyCone(x, y, m.X, m.Y, coneDegrees) {
			continue
		}
		if result == nil || d < best {
			result, best = m, d
		}
	}
	return result
}

// inPartyCone reports whether (tx,ty) lies within the full coneDegrees arc
// centred on the party's attack heading from (x,y).
func (cs *CombatSystem) inPartyCone(x, y, tx, ty, coneDegrees float64) bool {
	return math.Cos(math.Atan2(ty-y, tx-x)-cs.partyAttackAngle()) >= math.Cos(coneDegrees*math.Pi/360)
}

func advancedSpellVictim(m *monster.Monster3D) bool {
	return m != nil && m.IsAlive() && !m.IsDamageInvulnerable() && !m.IsPartyControlled() && !m.IsFish() && !m.IsCaravan()
}

func (cs *CombatSystem) tryCastAdvancedOffense(id spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter) spellCastOutcome {
	a := def.Mechanic
	if a == nil {
		return castNotHandled
	}
	tier := casterSpellMasteryTier(caster, def)
	frames, turns := config.EffectClock(def.DurationSeconds(tier), cs.game.config.GetTPS())
	x, y := cs.logicalCameraXY()
	radius := a.RadiusTiles * cs.game.config.GetTileSize()
	area := areaReach{x: x, y: y, radius: radius}
	changed := false
	switch a.Kind {
	case "acid_burst", "flying_fist":
		return castNotHandled // Standard projectile path, with riders at impact.
	case "shrapmetal":
		return cs.castShrapmetal(id, def, caster)
	case "berserk", "destroy_undead":
		// Berserk takes the living, Destroy Undead the undead.
		undead := a.Kind == "destroy_undead"
		m := cs.advancedSpellTarget(radius, a.ConeDegrees, func(m *monster.Monster3D) bool { return (m.MonsterType == monster.TypeUndead) == undead })
		if m == nil {
			return castNoEffect
		}
		if a.Kind == "berserk" {
			m.BerserkFramesRemaining = max(m.BerserkFramesRemaining, frames)
			m.WasAttacked = true
			m.BeginPlayerEngagement()
			cs.refreshMonsterAITarget(m)
			cs.game.startMonsterSpellVisual(def, m)
		} else {
			cs.game.startMonsterSpellVisual(def, m)
			// This is an undead-only execution, not Disintegrate (which undead
			// resist). A successful roll replaces damage rather than adding to it.
			if rand.Float64() < a.InstantKillChance {
				m.HitPoints = 0
				cs.markMonsterHit(m)
				cs.game.logCombat(logToneGood, "%s destroys %s!", logSchoolWord(a.DamageSchool, def.Name), logMonsterName(m))
				cs.finishMonsterKillImmediately(m)
			} else {
				cs.advancedSpellDamage(id, def, caster, m)
			}
		}
		return castCommitted
	case "slow", "frozen_ground", "turn_undead", "mass_fear", "feeblemind":
		cs.forEachAreaVictim(area, advancedSpellVictim, func(m *monster.Monster3D) {
			if a.Kind == "frozen_ground" && !cs.inPartyCone(x, y, m.X, m.Y, a.ConeDegrees) {
				return
			}
			if (a.Kind == "turn_undead" || a.Kind == "mass_fear") && (m.MonsterType == monster.TypeUndead) != (a.Kind == "turn_undead") {
				return
			}
			changed = true // An eligible target pays for the attempt even if it saves.
			turns, frames := cs.spellControlDuration(def, caster, m, turns, frames)
			if turns <= 0 && frames <= 0 {
				return
			}
			switch a.Kind {
			case "slow":
				m.ApplySlow(a.Power(tier), frames, turns)
			case "frozen_ground":
				cs.applyMonsterRoot(m, turns)
			case "turn_undead", "mass_fear":
				m.ApplyFear(frames)
				cs.game.releaseMonsterAttackPost(m)
			case "feeblemind":
				m.SilenceFramesRemaining = frames
				m.SilenceImmunityFramesRemaining = a.ReapplyImmunitySeconds * cs.game.config.GetTPS()
			}
			if a.Kind != "frozen_ground" {
				cs.game.startMonsterSpellVisual(def, m)
			}
		})
		if a.Kind == "frozen_ground" {
			changed = cs.freezeWaterCone(def) || changed
		}
	case "prismatic_light", "souldrinker":
		drained := 0
		for victimIndex, m := range cs.advancedSpellTargets(area, a.MaxTargets) {
			cs.spawnAreaTargetFlight(def, m, victimIndex)
			drained += cs.advancedSpellDamage(id, def, caster, m)
			changed = true
		}
		if a.Kind == "souldrinker" && drained > 0 {
			cs.distributePartyHealing(drained * a.Power(tier) / 100)
		}
	default:
		return castNotHandled
	}
	if !changed {
		return castNoEffect
	}
	return castCommitted
}

// Targeted fans share eligibility, wall/range checks and nearest-first order.
// Zero means unlimited for area drains; authored capped fans require a limit.
func (cs *CombatSystem) advancedSpellTargets(area areaReach, limit int) []*monster.Monster3D {
	var targets []*monster.Monster3D
	cs.forEachAreaVictim(area, advancedSpellVictim, func(m *monster.Monster3D) { targets = append(targets, m) })
	slices.SortStableFunc(targets, func(a, b *monster.Monster3D) int {
		return cmp.Compare(Distance(area.x, area.y, a.X, a.Y), Distance(area.x, area.y, b.X, b.Y))
	})
	if limit > 0 && len(targets) > limit {
		targets = targets[:limit]
	}
	return targets
}

// spawnAreaTargetFlight uses the existing projectile material, motion and cleanup.
// Snapshot the visible contact before damage can kill or displace its target.
// No collision entity is registered: the area spell has already owned its hit.
func (cs *CombatSystem) spawnAreaTargetFlight(def spells.SpellDefinition, target *monster.Monster3D, index int) {
	authored, ok := config.GetSpellDefinition(string(def.ID))
	if !ok || authored.Graphics == nil || authored.Graphics.TargetFlights == nil {
		return
	}
	rays := authored.Graphics.TargetFlights
	x, y := cs.logicalCameraXY()
	tx, ty := cs.monsterVisualPos(target)
	distance := math.Hypot(target.X-x, target.Y-y)
	if distance <= 0 {
		return
	}
	angle := math.Atan2(target.Y-y, target.X-x)
	p, err := spells.NewCastingSystem(cs.game.config).CreateProjectile(spells.SpellID(rays.Projectile), x, y, angle)
	if err != nil {
		return // Catalog validation rejects missing source physics/render data.
	}
	frames := max(1, int(math.Ceil(distance/math.Hypot(p.VelX, p.VelY))))
	if rays.DurationSeconds > 0 {
		frames = max(1, int(math.Round(rays.DurationSeconds*float64(cs.game.config.GetTPS()))))
	}
	tint := rays.Palette[index%len(rays.Palette)]
	launch := projectileLaunch{valid: true, x: x, y: y, angle: angle,
		dx: math.Cos(angle), dy: math.Sin(angle), distance: distance,
		aimX: target.X, aimY: target.Y, aimDistance: distance,
		passed: true, passedX: tx - target.X, passedY: ty - target.Y,
		passedAnchor: visualAnchorFor(target)}
	vx, vy := (target.X-x)/float64(frames), (target.Y-y)/float64(frames)
	if rays.ReturnToParty {
		// Return from the actual sprite to the party's cast-time position.
		vx, vy = (x-target.X)/float64(frames), (y-target.Y)/float64(frames)
		x, y = target.X, target.Y
		launch = projectileLaunch{x: x, y: y, dx: -math.Cos(angle), dy: -math.Sin(angle), distance: distance, aimDistance: distance, startX: tx - x, startY: ty - y, startAnchor: visualAnchorFor(target)}
	}
	cs.game.magicProjectiles = append(cs.game.magicProjectiles, MagicProjectile{
		ID: cs.game.GenerateProjectileID(rays.Projectile), X: x, Y: y,
		VelX: vx, VelY: vy,
		LifeTime: frames + 1, Active: true, SpellType: rays.Projectile, Size: p.Size,
		NoCollide: true, Owner: ProjectileOwnerPlayer, VisualColor: &tint, VisualFx: rays.ProjectileFx, Launch: launch,
		VisualScale:  rays.SizeScale,
		VisualFrames: frames + 1,
	})
}

// advancedSpellDamage resolves and reports one spell victim before kill
// settlement. Its return value is HP lost; the log shows the resolved packet,
// which can exceed HP lost on an overkill.
func (cs *CombatSystem) advancedSpellDamage(id spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter, m *monster.Monster3D) int {
	if cs.tryDarkElfBindInstead(caster, m) {
		return 0
	}
	_, _, total := cs.CalculateSpellDamage(id, caster)
	parts := cs.spellDamageParts(id, caster, total)
	school := def.DamageType()
	parts, _ = cs.spellPartsWithOutgoingBuff(parts, school)
	attack := cs.newPartyMonsterAttack(parts.Normal, parts.True, school, cs.spellResistPierce(caster, string(id)), nil, def.Name, false, true, false)
	attack.Attacker = caster
	before := m.HitPoints
	dealt := cs.applyPartyMonsterAttack(m, attack)
	cs.markMonsterHit(m)
	cs.spawnMonsterHitBurst(m, school)
	cs.game.reportDamage(logToneGood, damageLog{
		Source: logSchoolWord(school, def.Name),
		Target: monsterLogTarget(m),
		Amount: dealt.Total(),
		School: school,
	})
	if !m.IsAlive() {
		if cs.game.collisionSystem != nil {
			cs.game.collisionSystem.UnregisterEntity(m.ID)
		}
		cs.finishMonsterKill(m)
	}
	return max(0, before-max(0, m.HitPoints))
}

// applyAdvancedSpellImpact runs only after a successful player projectile hit.
func (cs *CombatSystem) applyAdvancedSpellImpact(p *MagicProjectile, m *monster.Monster3D) {
	if p == nil || p.Owner != ProjectileOwnerPlayer || !m.IsAlive() {
		return
	}
	def, err := spells.GetSpellDefinitionByID(spells.SpellID(p.SpellType))
	if err != nil || def.Mechanic == nil {
		return
	}
	a := def.Mechanic
	tier := casterSpellMasteryTier(p.Attacker, def)
	frames, rounds := config.EffectClock(def.DurationSeconds(tier), cs.game.config.GetTPS())
	switch a.Kind {
	case "acid_burst":
		m.ApplyArmorShred(a.Power(tier), frames, rounds)
	case "flying_fist":
		cs.pushElementalTarget(m, float64(a.Power(tier)))
	}
}

// A volley owns its hit budget, so simultaneous casts and spell echoes are
// independent. Projectile copies retain the same budget until the volley dies.
type spellVolleyHitBudget struct {
	limit int
	hits  map[*monster.Monster3D]int
}

func (cs *CombatSystem) castShrapmetal(id spells.SpellID, def spells.SpellDefinition, caster *character.MMCharacter) spellCastOutcome {
	physics, err := cs.game.config.GetSpellConfig(string(id))
	if err != nil {
		return castRejected
	}
	a := def.Mechanic
	x, y := cs.logicalCameraXY()
	p, err := spells.NewCastingSystem(cs.game.config).CreateProjectile(id, x, y, cs.partyAttackAngle())
	if err != nil {
		return castRejected
	}
	count := a.Projectiles(casterSpellMasteryTier(caster, def))
	budget := &spellVolleyHitBudget{limit: a.MaxHitsPerTarget, hits: make(map[*monster.Monster3D]int)}
	speed := math.Hypot(p.VelX, p.VelY)
	distance := physics.RangeTiles * cs.game.config.GetTileSize()
	spread := a.ConeDegrees * math.Pi / 180
	size := cs.game.config.ProjectileHitboxTiles(physics) * cs.game.config.GetTileSize()
	_, _, total := cs.CalculateSpellDamage(id, caster)
	parts := cs.spellDamageParts(id, caster, total)
	for i := 0; i < count; i++ {
		shotParts, crit := cs.rollSpellCritParts(id, caster, parts)
		angle := cs.partyAttackAngle() + (rand.Float64()-.5)*spread
		// No target assistance: physical boxes decide contact along each ray.
		// The release-plane guard only excludes objects behind the fragment.
		launch := projectileLaunch{valid: true, x: x, y: y, angle: angle, dx: math.Cos(angle), dy: math.Sin(angle), distance: distance, cone: math.Pi / 2}
		shot := MagicProjectile{ID: cs.game.GenerateProjectileID(string(id)), Launch: launch, HitBudget: budget, Attacker: caster, X: x, Y: y, VelX: launch.dx * speed, VelY: launch.dy * speed, Damage: shotParts.Normal, TrueDamage: shotParts.True, Crit: crit, LifeTime: p.LifeTime, Active: true, SpellType: string(id), Size: p.Size, Owner: ProjectileOwnerPlayer, DisintegrateChance: cs.game.partyDisintegrateChance(def.DisintegrateChance)}
		cs.game.magicProjectiles = append(cs.game.magicProjectiles, shot)
		cs.game.collisionSystem.RegisterEntity(collision.NewEntity(shot.ID, shot.X, shot.Y, size, size, collision.CollisionTypeProjectile, false))
	}
	return castCommitted
}

func (cs *CombatSystem) nearestBerserkFoe(m *monster.Monster3D) *monster.Monster3D {
	best := Distance(m.X, m.Y, cs.game.camera.X, cs.game.camera.Y)
	if !cs.attackLineClear(m.X, m.Y, cs.game.camera.X, cs.game.camera.Y) {
		best = math.Inf(1)
	}
	var foe *monster.Monster3D
	for _, other := range cs.game.world.Monsters {
		if !m.CanAttackActor(other) || !cs.attackLineClear(m.X, m.Y, other.X, other.Y) {
			continue
		}
		if d := Distance(m.X, m.Y, other.X, other.Y); d < best {
			best, foe = d, other
		}
	}
	return foe
}
