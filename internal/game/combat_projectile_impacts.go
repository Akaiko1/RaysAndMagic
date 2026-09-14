package game

import (
	"fmt"
	"math"
	"math/rand"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

// CheckProjectileMonsterCollisions checks for collisions between projectiles and monsters
// using perspective-scaled bounding boxes for accurate visual collision detection.
// Crossfire and reflected shots use authoritative world-space collision instead.
func (cs *CombatSystem) CheckProjectileMonsterCollisions() {
	// Collect all active projectiles. Monster-owned ones are excluded (they hit
	// the party, not other monsters); party, crossfire, and reflected owners can
	// hit monsters under their respective target filters below.
	type projectileInfo struct {
		entityID string
		data     interface{}
		pType    string
		owner    ProjectileOwner
		index    int
	}
	var projectiles []projectileInfo

	for i := range cs.game.arrows {
		if cs.game.arrows[i].Active && cs.game.arrows[i].LifeTime > 0 && cs.game.arrows[i].Owner != ProjectileOwnerMonster {
			snapshot := cs.game.arrows[i]
			projectiles = append(projectiles, projectileInfo{snapshot.ID, &snapshot, "arrow", snapshot.Owner, i})
		}
	}
	for i := range cs.game.magicProjectiles {
		if cs.game.magicProjectiles[i].NoCollide {
			continue // mortar display bolt: the scheduled detonation is the real hit
		}
		if cs.game.magicProjectiles[i].Active && cs.game.magicProjectiles[i].LifeTime > 0 && cs.game.magicProjectiles[i].Owner != ProjectileOwnerMonster {
			snapshot := cs.game.magicProjectiles[i]
			projectiles = append(projectiles, projectileInfo{snapshot.ID, &snapshot, "magic_projectile", snapshot.Owner, i})
		}
	}
	// Player shots retain the perspective-scaled first-person assist. Crossfire is
	// autonomous world combat and must not depend on where the party is looking,
	// so those projectiles use the registered world-space collision boxes.
	for _, proj := range projectiles {
		var hitMonster *monsterPkg.Monster3D
		bestDepth := 0.0
		bestLateral := 0.0
		bestWorldDistance := math.MaxFloat64
		crossfire := proj.owner == ProjectileOwnerBoundUndead || proj.owner == ProjectileOwnerMonsterAtBound
		reflected := proj.owner == ProjectileOwnerReflected
		worldSpace := crossfire || reflected
		projectileX, projectileY := cs.getProjectilePosition(proj.data, proj.pType)

		camCos := math.Cos(cs.game.camera.Angle)
		camSin := math.Sin(cs.game.camera.Angle)

		for _, monster := range cs.game.world.Monsters {
			if !monster.IsAlive() {
				continue
			}
			// A mirror-scale return belongs to its original shooter. Other mobs
			// remain transparent even when they stand across the return path.
			if reflected && projectileSourceMonster(proj.data) != monster {
				continue
			}
			if proj.owner == ProjectileOwnerPlayer && isPurePartySummon(monster) {
				continue
			}
			// A pierce continuation bolt (Arena Arbalest) flies on THROUGH the
			// monster it already hit - never collides with it again.
			if ar, ok := proj.data.(*Arrow); ok && ar.SkipMonster == monster {
				continue
			}
			// Crossfire faction rules: a bound undead's bolt skips controlled allies
			// (hits enemies); a mob's anti-undead bolt hits ONLY the bound undead.
			if proj.owner == ProjectileOwnerBoundUndead && !cs.boundAllyCanDamageMonster(monster) {
				continue
			}
			if proj.owner == ProjectileOwnerMonsterAtBound && !monster.Bound {
				continue
			}
			if worldSpace {
				if !cs.checkWorldSpaceProjectileCollision(proj.entityID, monster) {
					continue
				}
				dx, dy := monster.X-projectileX, monster.Y-projectileY
				distSq := dx*dx + dy*dy
				if hitMonster == nil || distSq < bestWorldDistance ||
					(distSq == bestWorldDistance && monster.ID < hitMonster.ID) {
					bestWorldDistance = distSq
					hitMonster = monster
				}
				continue
			}
			if cs.checkPerspectiveScaledCollision(proj.entityID, proj.data, proj.pType, monster) {
				dx := monster.X - cs.game.camera.X
				dy := monster.Y - cs.game.camera.Y
				depth := dx*camCos + dy*camSin
				if depth <= 0 {
					continue
				}
				angle := math.Atan2(dy, dx)
				angleDiff := angle - cs.game.camera.Angle
				for angleDiff > math.Pi {
					angleDiff -= 2 * math.Pi
				}
				for angleDiff < -math.Pi {
					angleDiff += 2 * math.Pi
				}
				if math.Abs(angleDiff) > cs.game.camera.FOV/2 {
					continue
				}
				lateral := math.Abs(-dx*camSin + dy*camCos)
				if hitMonster == nil || depth < bestDepth || (depth == bestDepth && lateral < bestLateral) {
					bestDepth = depth
					bestLateral = lateral
					hitMonster = monster
				}
			}
		}
		if hitMonster == nil && proj.owner == ProjectileOwnerPlayer {
			var px, py, vx, vy float64
			switch d := proj.data.(type) {
			case *Arrow:
				px, py, vx, vy = d.X, d.Y, d.VelX, d.VelY
			case *MagicProjectile:
				px, py, vx, vy = d.X, d.Y, d.VelX, d.VelY
			}
			hitMonster = cs.turnBasedProjectileAssistTarget(px, py, vx, vy)
			// A pierce continuation must not re-hit the monster it went
			// through via the TB assist either.
			if ar, ok := proj.data.(*Arrow); ok && hitMonster != nil && hitMonster == ar.SkipMonster {
				hitMonster = nil
			}
		}
		if hitMonster != nil {
			// Reflections preserve only the Aegis' mirrored damage contract.
			// Crossfire retains its monster-vs-monster riders, while player
			// projectiles use the full party-damage path.
			if reflected {
				cs.resolveReflectedMonsterProjectile(proj.data, proj.pType, hitMonster, proj.entityID)
			} else if crossfire {
				cs.resolveMonsterProjectileVsMonster(proj.data, proj.pType, hitMonster, proj.entityID)
			} else {
				cs.applyProjectileDamage(proj.data, proj.pType, hitMonster, proj.entityID)
			}
		}
		// Impact riders may append either projectile family. Resolve the detached
		// snapshot, then commit it to the live slice after any reallocation.
		switch snapshot := proj.data.(type) {
		case *Arrow:
			cs.game.arrows[proj.index] = *snapshot
		case *MagicProjectile:
			cs.game.magicProjectiles[proj.index] = *snapshot
		}

	}
}

// CheckProjectilePlayerCollisions checks for collisions between monster projectiles and the player.
func (cs *CombatSystem) CheckProjectilePlayerCollisions() {
	playerEntity := cs.game.collisionSystem.GetEntityByID("player")
	if playerEntity == nil || playerEntity.BoundingBox == nil {
		return
	}

	for i := range cs.game.magicProjectiles {
		mp := &cs.game.magicProjectiles[i]
		if !mp.Active || mp.LifeTime <= 0 || mp.Owner != ProjectileOwnerMonster {
			continue
		}
		if cs.projectileHitsPlayer(mp.ID, playerEntity) {
			damageTypeStr := spellDamageTypeStr(mp.SpellType)
			// Broodscale Aegis: this same bolt may turn and fly back at its caster.
			if cs.tryReflectMonsterProjectile(
				mp.SourceMonster,
				mp.X,
				mp.Y,
				&mp.VelX,
				&mp.VelY,
				&mp.LifeTime,
				&mp.Owner,
			) {
				continue
			}
			// The projectile carries one source-adjusted snapshot for delivery.
			parts := damagecalc.Parts{Normal: mp.Damage, True: mp.TrueDamage}
			// Champion spell riders (lightning/psychic-shock stun) resolve from
			// the spell that actually flew - a weapon swing landing mid-flight
			// may have re-stamped the mob's rider fields for a hand.
			cs.stampChampionSpellRiders(mp.SourceMonster, mp.SpellType)
			hit := monsterCharacterHit{
				Parts:              parts,
				DamageType:         damageTypeStr,
				IgnoresArmor:       mp.SourceMonster != nil && mp.SourceMonster.IgnoresArmor,
				IgnoresDodge:       mp.IgnoresDodge,
				DisintegrateChance: mp.DisintegrateChance,
				Spell:              true, // a MagicProjectile always flies a spells.yaml page
			}
			if mp.AoE {
				cs.applyMonsterProjectileDamageAoE(mp.SourceMonster, mp.SourceName, hit)
			} else {
				cs.applyMonsterProjectileDamage(mp.SourceMonster, mp.SourceName, hit)
			}
			mp.Active = false
			cs.game.collisionSystem.UnregisterEntity(mp.ID)
		}
	}

	for i := range cs.game.arrows {
		ar := &cs.game.arrows[i]
		if !ar.Active || ar.LifeTime <= 0 || ar.Owner != ProjectileOwnerMonster {
			continue
		}
		if cs.projectileHitsPlayer(ar.ID, playerEntity) {
			damageTypeStr := normalizeDamageTypeStr(ar.DamageType)
			// Broodscale Aegis: this same dart may turn and fly back at its shooter.
			if cs.tryReflectMonsterProjectile(
				ar.SourceMonster,
				ar.X,
				ar.Y,
				&ar.VelX,
				&ar.VelY,
				&ar.LifeTime,
				&ar.Owner,
			) {
				continue
			}
			// Reuse the source-adjusted snapshot recorded when the dart fired.
			parts := damagecalc.Parts{Normal: ar.Damage, True: ar.TrueDamage}
			// Riders resolve from the weapon that FIRED this dart - a swing landing
			// mid-flight may have re-armed the mob's rider fields for another hand.
			cs.stampChampionProjectileRiders(ar.SourceMonster, ar.BowKey)
			weaponDef, _ := config.GetWeaponDefinition(ar.BowKey)
			hit := monsterCharacterHit{
				Parts:              parts,
				DamageType:         damageTypeStr,
				IgnoresArmor:       ar.SourceMonster != nil && ar.SourceMonster.IgnoresArmor,
				ArmorPiercePct:     weaponArmorPiercePct(weaponDef),
				IgnoresDodge:       ar.IgnoresDodge,
				DisintegrateChance: ar.DisintegrateChance,
				// Spell stays false: a dart flies a weapons.yaml entry, whatever
				// element it carries.
			}
			// An AoE-rider weapon (bow_of_hellfire) engulfs the WHOLE party once
			// per volley - the champion rule: arc/AoE never multiply.
			if weaponDef != nil && weaponDef.AoeRadiusTiles > 0 && !ar.SuppressAoE {
				cs.applyMonsterProjectileDamageAoE(ar.SourceMonster, ar.SourceName, hit)
			} else {
				cs.applyMonsterProjectileDamage(ar.SourceMonster, ar.SourceName, hit)
			}
			ar.Active = false
			cs.game.collisionSystem.UnregisterEntity(ar.ID)
		}
	}
}

func (cs *CombatSystem) projectileHitsPlayer(projectileID string, playerEntity *collision.Entity) bool {
	projEntity := cs.game.collisionSystem.GetEntityByID(projectileID)
	if projEntity == nil || projEntity.BoundingBox == nil {
		return false
	}
	return projEntity.BoundingBox.Intersects(playerEntity.BoundingBox)
}

// applyMonsterProjectileDamage applies a single-target monster projectile/arrow.
// Both clocks use the same weighted final draw: the front slot keeps its authored
// tank bias, then race modifies each candidate's relative target weight.
func (cs *CombatSystem) applyMonsterProjectileDamage(src *monsterPkg.Monster3D, sourceName string, hit monsterCharacterHit) {
	cs.applyMonsterProjectileDamageToChar(src, cs.rangedTarget(), sourceName, hit)
}

// applyMonsterProjectileDamageAoE splashes a monster projectile across EVERY
// party member that can still take a hit (AoE spells like a monster's fireball).
func (cs *CombatSystem) applyMonsterProjectileDamageAoE(src *monsterPkg.Monster3D, sourceName string, hit monsterCharacterHit) {
	if sourceName == "" {
		sourceName = "Monster"
	}
	cs.game.AddCombatMessage(fmt.Sprintf("%s's blast engulfs the whole party!", sourceName))
	cs.forEachDamageablePartyMember(func(_ int, member *character.MMCharacter) {
		cs.applyMonsterProjectileDamageToChar(src, member, sourceName, hit)
	})
}

func (cs *CombatSystem) forEachDamageablePartyMember(fn func(idx int, member *character.MMCharacter)) {
	for idx, member := range cs.game.party.Members {
		if member == nil || member.HitPoints <= 0 {
			continue
		}
		fn(idx, member)
	}
}

func (cs *CombatSystem) applyMonsterProjectileDamageToChar(src *monsterPkg.Monster3D, currentChar *character.MMCharacter, sourceName string, hit monsterCharacterHit) {
	if currentChar == nil {
		return
	}
	// The projectile carries its immutable damage/dodge packet; src remains only
	// for authored status riders and kill attribution.
	cs.monsterHitCharacter(src, currentChar, sourceName, hit)
}

// getProjectileGraphicsInfo extracts base size, min size, and max size for a projectile
func (cs *CombatSystem) getProjectileGraphicsInfo(projectile interface{}, projectileType string) (baseSize float64, minSize, maxSize int, ok bool) {
	switch projectileType {
	case "magic_projectile":
		magicProj := projectile.(*MagicProjectile)
		cfg, err := cs.game.config.GetSpellGraphicsConfig(magicProj.SpellType)
		if err != nil {
			return 0, 0, 0, false
		}
		return float64(cfg.BaseSize), cfg.MinSize, cfg.MaxSize, true
	case "arrow":
		arrow := projectile.(*Arrow)
		weaponDef := lookupWeaponConfigByKey(arrow.BowKey)
		if weaponDef == nil || weaponDef.Graphics == nil {
			return 0, 0, 0, false
		}
		return float64(weaponDef.Graphics.BaseSize), weaponDef.Graphics.MinSize, weaponDef.Graphics.MaxSize, true
	}
	return 0, 0, 0, false
}

// getProjectilePosition returns the X, Y position of a projectile
func (cs *CombatSystem) getProjectilePosition(projectile interface{}, projectileType string) (float64, float64) {
	switch projectileType {
	case "magic_projectile":
		p := projectile.(*MagicProjectile)
		return p.X, p.Y
	case "arrow":
		p := projectile.(*Arrow)
		return p.X, p.Y
	}
	return 0, 0
}

// calculatePerspectiveScale calculates the scale factor for perspective-based collision
func (cs *CombatSystem) calculatePerspectiveScale(x, y, baseSize float64, minSize, maxSize int) float64 {
	dist := Distance(cs.game.camera.X, cs.game.camera.Y, x, y)
	if dist == 0 {
		dist = 0.001 // Avoid division by zero
	}

	visualSize := baseSize / dist * float64(cs.game.config.GetTileSize())
	if visualSize > float64(maxSize) {
		visualSize = float64(maxSize)
	}
	if visualSize < float64(minSize) {
		visualSize = float64(minSize)
	}
	scale := visualSize / baseSize
	// Never INFLATE the collision box above its true world size. Near the camera
	// (e.g. the spawn frame, dist~0) this scale would otherwise balloon - a
	// fireball's 2-tile box x ~3.9 ~ 8 tiles - so it "hit" and exploded on a
	// monster several tiles away before the projectile was even drawn. Clamping
	// to 1 keeps collision at the world box up close and only shrinks it far away.
	if scale > 1.0 {
		scale = 1.0
	}
	return scale
}

// spawnProjectileHitFX bursts the impact FX for a projectile hit at the FX
// anchor: spell-typed particles for magic projectiles (school-colored fallback
// otherwise), or the ranged-weapon effect for arrows.
func (cs *CombatSystem) spawnProjectileHitFX(projectile interface{}, fxX, fxY float64, isSpell, isRanged bool, damageTypeStr string, monster *monsterPkg.Monster3D, weaponDef *config.WeaponDefinitionConfig, damage int) {
	if isSpell {
		if mp, ok := projectile.(*MagicProjectile); ok {
			cs.game.CreateSpellHitEffectFromSpell(fxX, fxY, mp.SpellType)
		} else {
			cs.game.CreateSpellHitEffect(fxX, fxY, damageTypeStr, SpellParticleCount, SpellParticleSize)
		}
	} else if isRanged {
		cs.spawnRangedHitEffect(monster, weaponDef, damage)
	}
}

// applyProjectileDamage applies damage from a projectile to a monster and generates combat messages
func (cs *CombatSystem) applyProjectileDamage(projectile interface{}, projectileType string, monster *monsterPkg.Monster3D, entityID string) {
	// This guard is also kept at the resolver boundary for direct callers.
	// Do not consume or unregister the projectile: pure summons are transparent.
	if isPurePartySummon(monster) {
		return
	}
	var damage int
	var isCrit bool
	var weaponName string
	var damageTypeStr string
	var isSpell bool
	var isRanged bool
	var weaponDef *config.WeaponDefinitionConfig
	var disintegrateChance float64
	var aoeRadiusTiles float64
	var isBindSpell bool
	var bindSeconds int
	var isPacifySpell bool
	var pacifySeconds int
	var dealsNoDamage bool
	var stunChance float64
	var stunSeconds int
	var stunTurns int
	var starburstFx bool
	var ricochetArrow *Arrow

	switch projectileType {
	case "magic_projectile":
		mp := projectile.(*MagicProjectile)
		if !mp.Active || mp.LifeTime <= 0 {
			return
		}
		damage, isCrit = mp.Damage, mp.Crit
		disintegrateChance = mp.DisintegrateChance
		spellID := spells.SpellID(mp.SpellType)
		spellDef, _ := spells.GetSpellDefinitionByID(spellID)
		weaponName = spellDef.Name
		damageTypeStr = normalizeDamageTypeStr(spellDef.School)
		aoeRadiusTiles = spellDef.AoeRadiusTiles
		isBindSpell = spellDef.BindUndead
		bindSeconds = spellDef.BindDurationSeconds
		isPacifySpell = spellDef.Pacify
		pacifySeconds = spellDef.PacifyDurationSeconds
		dealsNoDamage = spellDef.DealsNoDamage
		stunChance = spellDef.StunChance
		stunSeconds = spellDef.StunDurationSeconds
		stunTurns = spellDef.StunDurationTurns
		starburstFx = spellDef.StarburstFx
		mp.Active = false
		isSpell = true

	case "arrow":
		ar := projectile.(*Arrow)
		if !ar.Active || ar.LifeTime <= 0 {
			return
		}
		ricochetArrow = ar
		damage, isCrit = ar.Damage, ar.Crit
		disintegrateChance = ar.DisintegrateChance
		weaponName = "Arrow"
		damageTypeStr = normalizeDamageTypeStr(ar.DamageType)
		ar.Active = false
		isRanged = true
		if ar.Owner == ProjectileOwnerPlayer && ar.BowKey != "" && ar.Label == "" {
			weaponDef = lookupWeaponConfigByKey(ar.BowKey)
			if weaponDef != nil {
				aoeRadiusTiles = weaponDef.AoeRadiusTiles
				if weaponDef.Name != "" {
					weaponName = weaponDef.Name
				}
			}
		}
		// A labeled bolt (Bandit Card proc) reports under its own name - the
		// BowKey above may be the hunting-bow PHYSICS fallback of a melee wielder.
		if ar.Label != "" {
			weaponName = ar.Label
		}
		// Arena Arbalest: the bolt goes through the first target and flies on as
		// a continuation that can never re-hit the monster it pierced. Spawned
		// before damage resolution so even a killing hit passes through.
		if ar.PierceLeft > 0 && ar.Owner == ProjectileOwnerPlayer {
			cont := *ar
			cont.PierceLeft = ar.PierceLeft - 1
			cont.SkipMonster = monster
			cs.spawnArrowContinuation(cont, weaponDef)
		}
	default:
		return
	}

	// A sealed (dormant) boss absorbs the projectile - no damage, control effect,
	// or aggro - until its quest unseals it. The projectile is already consumed by
	// the switch above; drop its collision entity and stop here.
	if cs.absorbIfSealed(monster) {
		cs.game.collisionSystem.UnregisterEntity(entityID)
		return
	}

	// Party buffs: flat bonus to party outgoing damage, filtered by damage type.
	// Spell packets use the same post-modifier step as zones, mortars, novas,
	// and tooltips; weapon arrows keep their existing direct path.
	if damage > 0 {
		if isSpell {
			parts, _ := cs.spellPartsWithOutgoingBuff(damagecalc.Parts{Normal: damage}, damageTypeStr)
			damage = parts.Normal
		} else {
			damage = weaponDamageWithBuff(damage, cs.game.combatBuffOutBonusForDamageType(damageTypeStr))
		}
	}

	// Resolve the attacker the projectile was fired by (stamped at spawn) -
	// selection may have auto-advanced (or the roster swapped) while it flew.
	var attacker *character.MMCharacter
	switch pr := projectile.(type) {
	case *MagicProjectile:
		attacker = pr.Attacker
	case *Arrow:
		attacker = pr.Attacker
	}
	attackerName := "The party"
	if attacker != nil {
		attackerName = attacker.Name
	}

	// Impact FX anchor: the projectile bursts where the monster is DRAWN. For a
	// turn-based pulled front-diagonal target that's the pulled slot (where the
	// assist connects), not its real off-to-the-side tile.
	fxX, fxY := cs.monsterVisualPos(monster)

	// Typed true damage is stamped when the projectile leaves its source.
	// Resolving weapon mastery here used to let equipment/mastery changes in
	// flight alter an arrow and made Bandit Card's generic bolt inherit the
	// Hunting Bow physics fallback.
	trueDmg, ignoreDodge := 0, false
	switch p := projectile.(type) {
	case *Arrow:
		trueDmg, ignoreDodge = p.TrueDamage, p.IgnoresDodge
	case *MagicProjectile:
		trueDmg, ignoreDodge = p.TrueDamage, p.IgnoresDodge
	}
	resistPierce := 0
	if isSpell {
		if mp, ok := projectile.(*MagicProjectile); ok {
			resistPierce = cs.spellResistPierce(attacker, mp.SpellType)
		}
	}
	attack := cs.newPartyMonsterAttack(
		damage,
		trueDmg,
		damageTypeStr,
		resistPierce,
		weaponDef,
		weaponName,
		isRanged,
		isSpell,
		false,
	)
	attack.Attacker = attacker
	attack.IgnoreDodge = ignoreDodge

	// Check monster perfect dodge (applies to all attack types). A Grandmaster
	// weapon strike ignores it; otherwise the normal hit is dodged but typed
	// TRUE damage still lands.
	if monsterPerfectDodges(monster, attack.IgnoreDodge) {
		cs.breakPacifyOnHit(monster)
		if trueDmg > 0 {
			cs.applyTrueDamageThroughDodge(monster, attack.Packet, attacker, attackerName, weaponDef)
		} else {
			cs.game.AddCombatMessage(fmt.Sprintf("%s dodges the %s!", monster.Name, weaponName))
		}
		cs.game.collisionSystem.UnregisterEntity(entityID)
		return
	}
	if cs.tryDarkElfBindInstead(attacker, monster) {
		cs.game.collisionSystem.UnregisterEntity(entityID)
		return
	}

	// Ricochet is an on-impact rider, unlike straight-line pierce. A sealed
	// target absorbs the projectile and Perfect Dodge avoids it, so neither can
	// seed a second bolt.
	cs.trySpawnArrowRicochet(ricochetArrow, monster, weaponDef)

	// Control spells deal no damage - Bind Undead takes control, Charm pacifies.
	if isBindSpell {
		cs.applyBindUndead(monster, bindSeconds, weaponName)
		cs.game.collisionSystem.UnregisterEntity(entityID)
		return
	}
	if isPacifySpell {
		cs.applyPacify(monster, pacifySeconds, weaponName)
		cs.game.collisionSystem.UnregisterEntity(entityID)
		return
	}

	if disintegrateChance > 0 && !monsterImmuneToDisintegrate(monster) && rand.Float64() < disintegrateChance {
		cs.spawnProjectileHitFX(projectile, fxX, fxY, isSpell, isRanged, damageTypeStr, monster, weaponDef, damage)

		monster.HitPoints = 0
		cs.markMonsterHit(monster)
		cs.game.collisionSystem.UnregisterEntity(entityID)
		xpAwarded := cs.finishWeaponKill(monster, weaponDef, attacker)

		cs.game.AddCombatMessage(fmt.Sprintf("%s's %s disintegrates %s!", attackerName, weaponName, monster.Name))
		cs.game.AddCombatMessage(fmt.Sprintf("Awarded %d experience.", xpAwarded))
		return
	}

	// A no-damage projectile (Disintegrate) that DIDN'T trigger its instakill - or
	// struck an immune target (undead/dragon) - deals nothing but is still a HIT:
	// run the same impact-FX and aggro/pacify/pack bookkeeping a real hit does
	// (the zero-valued packet still sets WasAttacked + engages, so passive mobs aggro)
	// and report "no effect" instead of falling into the damage path, which would
	// spam "hit for 0 damage" with a bogus "Critical!" (the spell can't crit).
	// Bind/Charm are handled earlier; this is the Disintegrate case.
	if dealsNoDamage {
		cs.spawnProjectileHitFX(projectile, fxX, fxY, isSpell, isRanged, damageTypeStr, monster, weaponDef, damage)
		cs.applyMonsterDamagePacket(
			monster,
			singleMonsterDamagePacket(damagecalc.Parts{}, damageTypeStr, resistPierce),
			monsterDamageOptions{IgnoreArmor: true},
		)
		cs.markMonsterHit(monster)
		cs.game.AddCombatMessage(fmt.Sprintf("%s has no effect on %s.", weaponName, monster.Name))
		cs.game.collisionSystem.UnregisterEntity(entityID)
		return
	}

	// Spawn hit effects at monster position (after dodge check, so only on actual hits)
	cs.spawnProjectileHitFX(projectile, fxX, fxY, isSpell, isRanged, damageTypeStr, monster, weaponDef, damage)

	actualDamage := cs.applyPartyMonsterAttack(monster, attack).Total()
	cs.markMonsterHit(monster)
	executed := false
	if monster.IsAlive() {
		cs.tryApplyWeaponHitRiders(monster, weaponDef)
		// Spell stun-on-hit (Psychic Shock): chance to stun the struck monster.
		if stunChance > 0 && rand.Float64() < stunChance {
			cs.applyStun(monster, stunSeconds, stunTurns) // announces stun/resist itself
		}
		// The Maw already credits its kill and announces itself.
		executed = cs.tryWeaponExecute(monster, weaponDef, attacker, attackerName)
	}
	xpAwarded := 0
	if !monster.IsAlive() && !executed {
		xpAwarded = cs.finishWeaponKill(monster, weaponDef, attacker)
	}
	cs.game.collisionSystem.UnregisterEntity(entityID)

	if executed {
		if aoeRadiusTiles > 0 {
			cs.applyAoeSplash(monster, attack, aoeRadiusTiles)
		}
		return
	}

	if !monster.IsAlive() {
		prefix := ""
		if isCrit {
			prefix = "Critical! "
		}
		cs.game.AddCombatMessage(fmt.Sprintf("%s%s hits %s for %d damage and kills it!",
			prefix, attackerName, monster.Name, actualDamage))
		cs.game.AddCombatMessage(fmt.Sprintf("Awarded %d experience.", xpAwarded))
	} else {
		prefix := ""
		if isCrit {
			prefix = "Critical! "
		}
		cs.game.AddCombatMessage(fmt.Sprintf("%s%s hit %s for %d %s damage! (HP: %d/%d)",
			prefix, weaponName, monster.Name, actualDamage, damageTypeStr, monster.HitPoints, monster.MaxHitPoints))
	}

	if aoeRadiusTiles > 0 {
		cs.applyAoeSplash(monster, attack, aoeRadiusTiles)
	}
	// Starburst: a star falls into every tile of the blast (purely visual).
	if starburstFx {
		r := aoeRadiusTiles
		if r <= 0 {
			r = 1
		}
		cs.game.spawnStarburstFx(monster.X, monster.Y, r)
	}
}

// applyAoeSplash deals one already-rolled party attack to every OTHER alive
// monster in radius. Crit/true/conversion are source-side and therefore shared;
// armor, target bonuses, resistance and soak resolve independently per victim.
// Splash itself cannot disintegrate, stun or trigger weapon/card on-hit riders.
func (cs *CombatSystem) applyAoeSplash(center *monsterPkg.Monster3D, attack partyMonsterAttack, radiusTiles float64) {
	if center == nil || radiusTiles <= 0 {
		return
	}
	tileSize := float64(cs.game.config.GetTileSize())
	radiusPx := radiusTiles * tileSize
	radiusSq := radiusPx * radiusPx
	cx, cy := center.X, center.Y

	for _, m := range cs.game.world.Monsters {
		// An invulnerable boss (sealed or idol-warded) takes no splash and triggers
		// no hit-flash / pack-aggro / message: skip it entirely.
		if m == nil || m == center || !m.IsAlive() || isPurePartySummon(m) || m.IsDamageInvulnerable() {
			continue
		}
		dx := m.X - cx
		dy := m.Y - cy
		if dx*dx+dy*dy > radiusSq {
			continue
		}
		if cs.tryDarkElfBindInstead(attack.Attacker, m) {
			continue
		}
		actual := cs.applyPartyMonsterAttack(m, attack).Total()
		cs.markMonsterHit(m)
		primarySchool := monsterPkg.DamagePhysical.String()
		if len(attack.Packet.Components) > 0 {
			primarySchool = attack.Packet.Components[0].School.String()
		}
		cs.spawnMonsterHitBurst(m, primarySchool)

		if !m.IsAlive() {
			xpAwarded := cs.finishWeaponKill(m, attack.WeaponDef, attack.Attacker)
			cs.game.AddCombatMessage(fmt.Sprintf("%s splash kills %s! (+%d XP)", attack.WeaponName, m.Name, xpAwarded))
		} else {
			cs.game.AddCombatMessage(fmt.Sprintf("%s splashes %s for %d damage.", attack.WeaponName, m.Name, actual))
		}
	}
}

// checkPerspectiveScaledCollision checks if a projectile collides with a monster using perspective-scaled bounding boxes
func (cs *CombatSystem) checkPerspectiveScaledCollision(entityID string, projectile interface{}, projectileType string, monster *monsterPkg.Monster3D) bool {
	// Get projectile graphics info for scaling
	baseSize, minSize, maxSize, ok := cs.getProjectileGraphicsInfo(projectile, projectileType)
	if !ok {
		return false
	}

	// Get collision entities
	projEntity := cs.game.collisionSystem.GetEntityByID(entityID)
	monsterCollisionEntity := cs.game.collisionSystem.GetEntityByID(monster.ID)
	if projEntity == nil || monsterCollisionEntity == nil {
		return false
	}

	// Calculate perspective-scaled collision boxes
	projX, projY := cs.getProjectilePosition(projectile, projectileType)
	projScale := cs.calculatePerspectiveScale(projX, projY, baseSize, minSize, maxSize)
	scaledProjW := projEntity.BoundingBox.Width * projScale
	scaledProjH := projEntity.BoundingBox.Height * projScale

	// Monster scaling
	monsterMultiplier := float64(cs.game.config.Graphics.Monster.SizeDistanceMultiplier)
	monsterScale := cs.calculatePerspectiveScale(monster.X, monster.Y, monsterMultiplier,
		cs.game.config.Graphics.Monster.MinSpriteSize, cs.game.config.Graphics.Monster.MaxSpriteSize)
	scaledMonsterW := monsterCollisionEntity.BoundingBox.Width * monsterScale
	scaledMonsterH := monsterCollisionEntity.BoundingBox.Height * monsterScale

	// Check collision with perspective-scaled boxes
	scaledProjBox := collision.NewBoundingBox(projX, projY, scaledProjW, scaledProjH)
	scaledMonsterBox := collision.NewBoundingBox(monster.X, monster.Y, scaledMonsterW, scaledMonsterH)
	return scaledProjBox.Intersects(scaledMonsterBox)
}

// checkWorldSpaceProjectileCollision is the camera-independent impact rule for
// monster-vs-monster crossfire. Both entities already own authoritative world
// boxes in the collision system; scaling them by the party camera would make a
// fight stop dealing damage when it moved behind or outside the player's FOV.
func (cs *CombatSystem) checkWorldSpaceProjectileCollision(entityID string, monster *monsterPkg.Monster3D) bool {
	if cs == nil || cs.game == nil || cs.game.collisionSystem == nil || monster == nil {
		return false
	}
	projectileEntity := cs.game.collisionSystem.GetEntityByID(entityID)
	monsterEntity := cs.game.collisionSystem.GetEntityByID(monster.ID)
	return projectileEntity != nil && projectileEntity.BoundingBox != nil &&
		monsterEntity != nil && monsterEntity.BoundingBox != nil &&
		projectileEntity.BoundingBox.Intersects(monsterEntity.BoundingBox)
}

func (cs *CombatSystem) spawnArrowContinuation(cont Arrow, weaponDef *config.WeaponDefinitionConfig) {
	if weaponDef == nil || weaponDef.Physics == nil {
		return
	}
	cont.ID = cs.game.GenerateProjectileID("arrow")
	cont.Active = true
	collisionSize := weaponDef.Physics.GetCollisionSizePixels(float64(cs.game.config.GetTileSize()))
	cs.game.arrows = append(cs.game.arrows, cont)
	entity := collision.NewEntity(
		cont.ID,
		cont.X,
		cont.Y,
		collisionSize,
		collisionSize,
		collision.CollisionTypeProjectile,
		false,
	)
	cs.game.collisionSystem.RegisterEntity(entity)
}

// trySpawnArrowRicochet creates the re-aimed Nest Arbalest leg after the
// primary projectile has passed absorption and dodge resolution.
func (cs *CombatSystem) trySpawnArrowRicochet(ar *Arrow, victim *monsterPkg.Monster3D, weaponDef *config.WeaponDefinitionConfig) {
	if ar == nil || victim == nil || ar.RicochetLeft <= 0 ||
		ar.Owner != ProjectileOwnerPlayer || weaponDef == nil || weaponDef.Physics == nil {
		return
	}
	next := cs.nearestRicochetTarget(victim, weaponDef)
	if next == nil {
		return
	}
	speed := math.Hypot(ar.VelX, ar.VelY)
	dist := math.Hypot(next.X-victim.X, next.Y-victim.Y)
	if speed <= 0 || dist <= 0 {
		return
	}
	cont := *ar
	cont.RicochetLeft--
	cont.SkipMonster = victim
	cont.X, cont.Y = victim.X, victim.Y
	// Each leg receives the YAML-authored lifetime. The parent may have spent
	// nearly all of its own lifetime reaching the first victim.
	cont.LifeTime = weaponDef.Physics.GetLifetimeFrames()
	cont.VelX = (next.X - victim.X) / dist * speed
	cont.VelY = (next.Y - victim.Y) / dist * speed
	cs.spawnArrowContinuation(cont, weaponDef)
}

// nearestRicochetTarget picks the closest other living ENEMY monster within
// seek range of the struck victim (bound allies and pure party summons are
// never ricochet food).
func (cs *CombatSystem) nearestRicochetTarget(victim *monsterPkg.Monster3D, weaponDef *config.WeaponDefinitionConfig) *monsterPkg.Monster3D {
	if victim == nil || weaponDef == nil || weaponDef.RicochetRangeTiles <= 0 || cs.game.world == nil {
		return nil
	}
	maxDist := weaponDef.RicochetRangeTiles * float64(cs.game.config.GetTileSize())
	var best *monsterPkg.Monster3D
	bestDist := maxDist
	for _, m := range cs.game.world.Monsters {
		if isExcludedFromPartyAutoTarget(m) || m == victim || !m.IsAlive() {
			continue
		}
		if d := Distance(victim.X, victim.Y, m.X, m.Y); d <= bestDist {
			best, bestDist = m, d
		}
	}
	return best
}
