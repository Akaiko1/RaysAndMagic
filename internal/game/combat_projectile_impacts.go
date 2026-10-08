package game

import (
	"fmt"
	"math"
	"math/rand"
	uitext "ugataima/assets/text"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

// CheckProjectileMonsterCollisions resolves every shot against physical world
// boxes. Input, camera direction and sprite scale never change the hit rule.
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
	for _, proj := range projectiles {
		var hitMonster *monsterPkg.Monster3D
		bestWorldDistance := math.MaxFloat64
		crossfire := proj.owner == ProjectileOwnerBoundUndead || proj.owner == ProjectileOwnerMonsterAtBound
		reflected := proj.owner == ProjectileOwnerReflected
		projectileX, projectileY := cs.getProjectilePosition(proj.data, proj.pType)

		for _, monster := range cs.game.world.Monsters {
			if ar, ok := proj.data.(*Arrow); ok && ar.Backwash != nil && ar.Backwash.Target != monster {
				continue
			}
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
			if proj.owner == ProjectileOwnerMonsterAtBound && !projectileSourceMonster(proj.data).CanAttackActor(monster) {
				continue
			}
			targetEntity := cs.game.collisionSystem.GetEntityByID(monster.ID)
			if targetEntity == nil || targetEntity.BoundingBox == nil {
				continue
			}
			var launch projectileLaunch
			switch p := proj.data.(type) {
			case *Arrow:
				launch = p.Launch
			case *MagicProjectile:
				launch = p.Launch
			}
			if proj.owner == ProjectileOwnerPlayer && !launch.reaches(projectileX, projectileY, targetEntity.BoundingBox) {
				continue
			}
			if !cs.projectileHitsEntity(proj.entityID, targetEntity) {
				continue
			}
			distSq := DistanceSquared(monster.X, monster.Y, projectileX, projectileY)
			if hitMonster == nil || distSq < bestWorldDistance ||
				(distSq == bestWorldDistance && monster.ID < hitMonster.ID) {
				bestWorldDistance = distSq
				hitMonster = monster
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
		if cs.projectileHitsEntity(mp.ID, playerEntity) {
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
			hit := monsterCharacterHit{
				SpellID:            mp.SpellType,
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
		if cs.projectileHitsEntity(ar.ID, playerEntity) {
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

// projectileHitsEntity owns physical overlap for every projectile and faction.
func (cs *CombatSystem) projectileHitsEntity(projectileID string, target *collision.Entity) bool {
	if target == nil || target.BoundingBox == nil {
		return false
	}
	projEntity := cs.game.collisionSystem.GetEntityByID(projectileID)
	if projEntity == nil || projEntity.BoundingBox == nil {
		return false
	}
	from, to := projEntity.BoundingBox, target.BoundingBox
	// Wide projectile boxes must not touch a target through a wall or door.
	return from.Intersects(to) && cs.attackLineClear(from.X, from.Y, to.X, to.Y)
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

// spawnProjectileHitFX bursts the impact FX for a projectile hit at the FX
// anchor: spell-typed particles for magic projectiles (school-colored fallback
// otherwise), or the ranged-weapon effect for arrows.
func (cs *CombatSystem) spawnProjectileHitFX(projectile interface{}, fxX, fxY float64, isSpell, isRanged bool, damageTypeStr string, monster *monsterPkg.Monster3D, weaponDef *config.WeaponDefinitionConfig, damage int) {
	if isSpell {
		if mp, ok := projectile.(*MagicProjectile); ok {
			cs.game.CreateSpellHitEffectFromSpell(fxX, fxY, mp.SpellType, monster)
		} else {
			cs.game.CreateSpellHitEffect(fxX, fxY, damageTypeStr, SpellParticleCount, SpellParticleSize, monster)
		}
	} else if isRanged {
		cs.spawnRangedHitEffect(monster, weaponDef, damage)
	}
}

// applyProjectileDamage applies damage from a projectile to a monster and generates combat messages
func (cs *CombatSystem) applyProjectileDamage(projectile interface{}, projectileType string, monster *monsterPkg.Monster3D, entityID string) {
	if shot, ok := projectile.(*Arrow); ok && shot.Backwash != nil {
		cs.resolveBackwashCharge(shot, monster)
		return
	}
	// This guard is also kept at the resolver boundary for direct callers.
	// Do not consume or unregister the projectile: pure summons are transparent.
	if isPurePartySummon(monster) {
		return
	}
	if p, ok := projectile.(*MagicProjectile); ok && p.FlaskKey != "" {
		cs.detonateFlask(p, p.X, p.Y)
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
	var spellShot *MagicProjectile

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
		spellShot = mp

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
			cont.Launch = ar.Launch.piercing(cs, monster)
			cs.spawnArrowContinuation(cont, weaponDef)
		}
	default:
		return
	}

	// A sealed (dormant) boss absorbs the projectile - no damage, control effect,
	// or aggro - until its quest unseals it. The projectile is already consumed by
	// the switch above; drop its collision entity and stop here.
	// A blast spell still goes off around it.
	if cs.absorbIfSealed(monster) {
		cs.game.collisionSystem.UnregisterEntity(entityID)
		cs.burstSpellShot(spellShot, monster.X, monster.Y, monster)
		return
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

	if ar, ok := projectile.(*Arrow); ok && ar.Overwatch {
		weaponName = uitext.Text("combat.overwatch_source", weaponName)
		attackerName = uitext.Text("combat.overwatch_source", attackerName)
	}

	// Impact FX follow the monster's drawn position, including a TB pulled slot.
	// Contact itself has already resolved against the actor's physical box.
	fxX, fxY := cs.monsterVisualPos(monster)

	// Typed true damage is stamped when the projectile leaves its source.
	// Resolving weapon mastery here used to let equipment/mastery changes in
	// flight alter an arrow and made Bandit Card's generic bolt inherit the
	// Hunting Bow physics fallback.
	var attack partyMonsterAttack
	trueDmg, resistPierce := 0, 0
	if ar, ok := projectile.(*Arrow); ok {
		trueDmg = ar.TrueDamage
		attack = cs.newPartyWeaponAttack(damage, trueDmg, damageTypeStr, weaponDef, weaponName, true, isCrit, ar.CritChance)
		attack.Attacker = attacker
		attack.IgnoreDodge = ar.IgnoresDodge
	} else {
		trueDmg = spellShot.TrueDamage
		attack = cs.spellProjectileAttack(spellShot)
		damage, resistPierce = attack.Packet.normalDamage(), cs.spellResistPierce(attacker, spellShot.SpellType)
	}

	// Check monster perfect dodge (applies to all attack types). A Grandmaster
	// weapon strike ignores it; otherwise the normal hit is dodged but typed
	// TRUE damage still lands.
	// A blast spell still goes off around a target that dodged or was bound.
	if cs.monsterPerfectDodges(monster, attack.IgnoreDodge) {
		cs.breakPacifyOnHit(monster)
		if trueDmg > 0 {
			cs.applyTrueDamageThroughDodge(monster, attack.Packet, attacker, attackerName, weaponDef)
		} else {
			cs.game.logCombat(logToneNone, "%s dodges the %s!", logMonsterName(monster), logSchoolWord(damageTypeStr, weaponName))
		}
		cs.game.collisionSystem.UnregisterEntity(entityID)
		cs.burstSpellShot(spellShot, monster.X, monster.Y, monster)
		return
	}
	if cs.tryDarkElfBindInstead(attacker, monster) {
		cs.game.collisionSystem.UnregisterEntity(entityID)
		cs.burstSpellShot(spellShot, monster.X, monster.Y, monster)
		return
	}

	// Ricochet is an on-impact rider, unlike straight-line pierce. A sealed
	// target absorbs the projectile and Perfect Dodge avoids it, so neither can
	// seed a second bolt.
	cs.trySpawnArrowRicochet(ricochetArrow, monster, weaponDef)
	if isRanged && weaponDef != nil {
		cs.game.designateTarget(attacker, monster)
	}

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

	if rollMonsterDisintegrate(monster, disintegrateChance) {
		cs.spawnProjectileHitFX(projectile, fxX, fxY, isSpell, isRanged, damageTypeStr, monster, weaponDef, attack.Packet.normalDamage())
		cs.game.collisionSystem.UnregisterEntity(entityID)
		cs.disintegratePartyTarget(monster, weaponDef, attacker, attack, aoeRadiusTiles,
			logParts(logHeroText(attackerName+"'s "), logSchoolWord(damageTypeStr, weaponName)))
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

	hit := cs.applyPartyMonsterAttack(monster, attack)
	actualDamage, isCrit := hit.Total(), hit.Critical
	cs.spawnProjectileHitFX(projectile, fxX, fxY, isSpell, isRanged, damageTypeStr, monster, weaponDef, hit.SourceNormal)
	cs.markMonsterHit(monster)
	cs.game.logCombat(logToneGood, "%s%s hit %s for %s %s damage! %s", logCrit(isCrit), logSchoolWord(damageTypeStr, weaponName),
		logMonsterName(monster), logDamage(actualDamage, damageTypeStr), logSchoolWord(damageTypeStr, damageTypeStr),
		logHP(monster.HitPoints, monster.MaxHitPoints))
	shot, _ := projectile.(*Arrow)
	executed := cs.settlePartyHit(monster, weaponDef, attacker, attackerName, shot, func() {
		// Spell stun-on-hit (Psychic Shock): chance to stun the struck monster.
		if stunChance > 0 && rand.Float64() < stunChance {
			cs.applyStun(monster, stunSeconds, stunTurns, true) // announces stun/resist itself
		}
	})
	cs.game.collisionSystem.UnregisterEntity(entityID)

	if executed {
		if aoeRadiusTiles > 0 {
			cs.applyAoeSplash(monster, attack, aoeRadiusTiles)
		}
		return
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

// spellProjectileAttack is the packet a party spell projectile delivers: its
// launch damage and crit, the live outgoing buff, true damage and pierce. The
// direct hit and every blast of the shot share it.
func (cs *CombatSystem) spellProjectileAttack(mp *MagicProjectile) partyMonsterAttack {
	def, _ := spells.GetSpellDefinitionByID(spells.SpellID(mp.SpellType))
	school := normalizeDamageTypeStr(def.School)
	parts, _ := cs.spellPartsWithOutgoingBuff(damagecalc.Parts{Normal: mp.Damage, True: mp.TrueDamage}, school)
	attack := cs.newPartyMonsterAttack(parts.Normal, parts.True, school, cs.spellResistPierce(mp.Attacker, mp.SpellType), nil, def.Name, false, true, false)
	attack.Critical = mp.Crit
	attack.Attacker = mp.Attacker
	attack.IgnoreDodge = mp.IgnoresDodge
	return attack
}

// burstSpellShot sets off a party blast spell where its shot ended without a
// damaging hit: against a wall, at the end of its range, or on a target that
// dodged, absorbed or was bound instead (spared, which the blast skips).
func (cs *CombatSystem) burstSpellShot(mp *MagicProjectile, x, y float64, spared *monsterPkg.Monster3D) {
	if mp == nil || mp.AoeTiles <= 0 || mp.Owner != ProjectileOwnerPlayer {
		return
	}
	cs.applyPartySplashFrom(x, y, cs.spellProjectileAttack(mp), mp.AoeTiles, spared)
	if def, err := spells.GetSpellDefinitionByID(spells.SpellID(mp.SpellType)); err == nil && def.StarburstFx {
		cs.game.spawnStarburstFx(x, y, mp.AoeTiles)
	}
}

// applyAoeSplash deals one already-rolled party attack to every OTHER alive
// monster in radius. The launch crit and true damage are shared; designation,
// armor, target bonuses, resistance and soak resolve independently per victim.
// Splash itself cannot disintegrate, stun or trigger weapon/card on-hit riders.
func (cs *CombatSystem) applyAoeSplash(center *monsterPkg.Monster3D, attack partyMonsterAttack, radiusTiles float64) {
	if center == nil || radiusTiles <= 0 {
		return
	}
	cs.applyPartySplashFrom(center.X, center.Y, attack, radiusTiles, center)
}

// applyPartySplashFrom is applyAoeSplash from any point; exclude (may be nil)
// is the direct victim the splash must not bill twice.
func (cs *CombatSystem) applyPartySplashFrom(x, y float64, attack partyMonsterAttack, radiusTiles float64, exclude *monsterPkg.Monster3D) {
	cs.applyAoeSplashAt(x, y, attack, radiusTiles, exclude, func(m *monsterPkg.Monster3D, actual int) {
		cs.markMonsterHit(m)
		primarySchool := attack.Packet.primarySchool()
		cs.spawnMonsterHitBurst(m, primarySchool)
		cs.game.logCombat(logToneGood, "%s splashes %s for %s damage.", logSchoolWord(primarySchool, attack.WeaponName), logMonsterName(m), logDamage(actual, primarySchool))
		if !m.IsAlive() {
			cs.finishWeaponKill(m, attack.WeaponDef, attack.Attacker)
		}
	})
}

// applyAoeSplashAt owns ground and target-centered party splash alike. An
// optional primary exclusion avoids billing a projectile's direct victim twice.
// A point blast reaches only monsters with a clear attack line from (x, y), so
// walls shield the room behind them; novas keep their own area policy.
// Source-specific status, presentation and kill riders run after the shared hit.
func (cs *CombatSystem) applyAoeSplashAt(x, y float64, attack partyMonsterAttack, radiusTiles float64, exclude *monsterPkg.Monster3D, onHit func(*monsterPkg.Monster3D, int)) {
	if radiusTiles <= 0 {
		return
	}
	hurts := func(m *monsterPkg.Monster3D) bool { return !isPurePartySummon(m) && !m.IsDamageInvulnerable() }
	cs.forEachAreaVictim(cs.pointBlast(x, y, radiusTiles), hurts, func(m *monsterPkg.Monster3D) {
		if !cs.tryDarkElfBindInstead(attack.Attacker, m) {
			onHit(m, cs.applyPartyMonsterAttack(m, attack).Total())
		}
	}, exclude)
}

func (cs *CombatSystem) spawnArrowContinuation(cont Arrow, weaponDef *config.WeaponDefinitionConfig) {
	if weaponDef == nil || weaponDef.Physics == nil {
		return
	}
	cont.ID = cs.game.GenerateProjectileID("arrow")
	cont.Active = true
	collisionSize := cs.game.config.ProjectileHitboxTiles(weaponDef.Physics) * cs.game.config.GetTileSize()
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
	cont.Launch = cs.continuationLaunch(victim, next) // a new ray from the struck actor
	cont.SkipMonster = victim
	cont.X, cont.Y = victim.X, victim.Y
	// Each leg receives the YAML-authored lifetime. The parent may have spent
	// nearly all of its own lifetime reaching the first victim.
	cont.LifeTime = weaponDef.Physics.GetLifetimeFrames()
	cont.VelX = (next.X - victim.X) / dist * speed
	cont.VelY = (next.Y - victim.Y) / dist * speed
	cs.spawnArrowContinuation(cont, weaponDef)
}

// nearestRicochetTarget picks the closest other living monster within seek
// range of the struck victim that the party auto-target policy allows.
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
