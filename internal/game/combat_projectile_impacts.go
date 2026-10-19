package game

import (
	"math"
	"math/rand"
	"slices"

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
		owner    ProjectileOwner
		index    int
	}
	var projectiles []projectileInfo

	for i := range cs.game.arrows {
		if cs.game.arrows[i].Active && cs.game.arrows[i].LifeTime > 0 && cs.game.arrows[i].Owner != ProjectileOwnerMonster {
			snapshot := cs.game.arrows[i]
			projectiles = append(projectiles, projectileInfo{snapshot.ID, &snapshot, snapshot.Owner, i})
		}
	}
	for i := range cs.game.magicProjectiles {
		if cs.game.magicProjectiles[i].NoCollide {
			continue // mortar display bolt: the scheduled detonation is the real hit
		}
		if cs.game.magicProjectiles[i].Active && cs.game.magicProjectiles[i].LifeTime > 0 && cs.game.magicProjectiles[i].Owner != ProjectileOwnerMonster {
			snapshot := cs.game.magicProjectiles[i]
			projectiles = append(projectiles, projectileInfo{snapshot.ID, &snapshot, snapshot.Owner, i})
		}
	}
	for i := range cs.game.flasks {
		if cs.game.flasks[i].Active && cs.game.flasks[i].LifeTime > 0 {
			snapshot := cs.game.flasks[i]
			projectiles = append(projectiles, projectileInfo{snapshot.ID, &snapshot, ProjectileOwnerPlayer, i})
		}
	}
	for _, proj := range projectiles {
		var hitMonster *monsterPkg.Monster3D
		bestWorldDistance := math.MaxFloat64
		crossfire := proj.owner == ProjectileOwnerPartyAlly || proj.owner == ProjectileOwnerMonsterAtActor
		reflected := proj.owner == ProjectileOwnerReflected
		projectileX, projectileY := cs.getProjectilePosition(proj.data)

		for _, monster := range cs.game.world.Monsters {
			if ar, ok := proj.data.(*Arrow); ok && slices.Contains(ar.Signature.LegHits, monster.ID) {
				continue
			}
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
			// Crossfire faction rules: an ally's bolt skips the other allies (hits
			// enemies); a mob's bolt at another monster hits only what that mob may attack.
			if proj.owner == ProjectileOwnerPartyAlly && !cs.boundAllyCanDamageMonster(monster) {
				continue
			}
			if proj.owner == ProjectileOwnerMonsterAtActor && !projectileSourceMonster(proj.data).CanAttackActor(monster) {
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
			case *FlaskProjectile:
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
				(distSq == bestWorldDistance && hitsFirst(monster, hitMonster, launch.target)) {
				bestWorldDistance = distSq
				hitMonster = monster
			}
		}
		if hitMonster != nil {
			// Reflections preserve only the Aegis' mirrored damage contract.
			// Crossfire retains its monster-vs-monster riders, while player
			// projectiles use the full party-damage path.
			if reflected {
				cs.resolveReflectedMonsterProjectile(proj.data, hitMonster, proj.entityID)
			} else if crossfire {
				if proj.owner == ProjectileOwnerMonsterAtActor {
					cs.monsterAttackLanded(projectileSourceMonster(proj.data))
				}
				cs.resolveMonsterProjectileVsMonster(proj.data, hitMonster, proj.entityID)
			} else {
				cs.applyProjectileDamage(proj.data, hitMonster, proj.entityID)
			}
		}
		// Impact riders may append either projectile family. Resolve the detached
		// snapshot, then commit it to the live slice after any reallocation.
		switch snapshot := proj.data.(type) {
		case *Arrow:
			cs.game.arrows[proj.index] = *snapshot
		case *MagicProjectile:
			cs.game.magicProjectiles[proj.index] = *snapshot
		case *FlaskProjectile:
			cs.game.flasks[proj.index] = *snapshot
		}

	}
}

// landShots ends each aimed arrow or spell shot that missed (projectileLaunch
// landing): on the spot its fallen target stood, or where its line meets the
// floor, with the shot's own impact drawn there; a blast still goes off. A
// shot about to reach its aim point first learns whether its target is still
// ahead (reachAimPoint). Collisions resolve first, so a monster still standing
// on that spot takes the shot.
func (cs *CombatSystem) landShots() {
	g := cs.game
	anchor := func(l projectileLaunch, onFloor bool) monsterVisualAnchor {
		if onFloor {
			return floorAnchor
		}
		return visualAnchorFor(l.target)
	}
	for i := range g.arrows {
		a := &g.arrows[i]
		if !a.Active || a.LifeTime <= 0 {
			continue
		}
		a.Launch.reachAimPoint(cs, a.ID, a.X, a.Y, a.VelX, a.VelY)
		x, y, onFloor, ok := a.Launch.landing(a.X, a.Y, a.VelX, a.VelY)
		if !ok {
			continue
		}
		a.Active, a.X, a.Y = false, x, y
		fx, fy, _, _ := a.Launch.renderMotion(cs, x, y, 0, 0)
		def, _ := config.GetWeaponDefinition(a.BowKey)
		g.spawnWeaponBoltImpact(fx, fy, def, SpellParticleCount, SpellParticleSize, anchor(a.Launch, onFloor))
	}
	// A blast can kill and spawn new shots: index the live slice each time.
	for i := 0; i < len(g.magicProjectiles); i++ {
		p := &g.magicProjectiles[i]
		if !p.Active || p.LifeTime <= 0 || p.NoCollide {
			continue
		}
		p.Launch.reachAimPoint(cs, p.ID, p.X, p.Y, p.VelX, p.VelY)
		x, y, onFloor, ok := p.Launch.landing(p.X, p.Y, p.VelX, p.VelY)
		if !ok {
			continue
		}
		p.Active, p.X, p.Y = false, x, y
		shot := *p
		fx, fy, _, _ := shot.Launch.renderMotion(cs, x, y, 0, 0)
		g.CreateSpellHitEffectFromSpell(fx, fy, shot.SpellType, anchor(shot.Launch, onFloor))
		cs.burstSpellShot(&shot, x, y, nil)
	}
}

// hitsFirst breaks a tie between two bodies the same distance from a shot,
// such as a stacked band drawn fanned apart: the one the shot was aimed at
// takes it, otherwise the lower ID. A nearer body still intercepts first.
func hitsFirst(m, other, aimed *monsterPkg.Monster3D) bool {
	if m == aimed || other == aimed {
		return m == aimed
	}
	return m.ID < other.ID
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
			cs.monsterAttackLanded(mp.SourceMonster)
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
				GrazesDodge:        mp.GrazesDodge,
				DisintegrateChance: mp.DisintegrateChance,
				Spell:              true, // a MagicProjectile always flies a spells.yaml page
				Projectile:         true,
				Critical:           mp.Crit,
			}
			if mp.AoeTiles > 0 {
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
			cs.monsterAttackLanded(ar.SourceMonster)
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
				ArmorPiercePct:     weaponArmorPiercePct(weaponDef),
				GrazesDodge:        ar.GrazesDodge,
				DisintegrateChance: ar.DisintegrateChance,
				// Spell stays false: a dart flies a weapons.yaml entry, whatever
				// element it carries.
				Projectile: true,
				Critical:   ar.Crit,
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
	cs.game.AddCombatMessage(uitext.Text("combat.s_blast_engulfs_the_whole_party", sourceName))
	cs.forEachDamageablePartyMember(func(_ int, member *character.MMCharacter) {
		cs.applyMonsterProjectileDamageToChar(src, member, sourceName, hit)
	})
}

func (cs *CombatSystem) forEachDamageablePartyMember(fn func(idx int, member *character.MMCharacter)) {
	defer cs.partyDamageEvent()()
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
func (cs *CombatSystem) getProjectilePosition(projectile interface{}) (float64, float64) {
	switch p := projectile.(type) {
	case *MagicProjectile:
		return p.X, p.Y
	case *Arrow:
		return p.X, p.Y
	case *FlaskProjectile:
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
			cs.game.CreateSpellHitEffectFromSpell(fxX, fxY, mp.SpellType, visualAnchorFor(monster))
		} else {
			cs.game.CreateSpellHitEffect(fxX, fxY, damageTypeStr, SpellParticleCount, SpellParticleSize, monster)
		}
	} else if isRanged {
		cs.spawnRangedHitEffect(monster, weaponDef, damage)
	}
}

// arrowAttack is the party attack an arrow carries to a victim: the damage
// and crit stamped at release, the hero's personal enchantments, the graze.
func (cs *CombatSystem) arrowAttack(ar *Arrow, school string, weaponDef *config.WeaponDefinitionConfig, weaponName string) partyMonsterAttack {
	attack := cs.newPartyWeaponAttack(ar.Damage, ar.TrueDamage, school, weaponDef, weaponName, true, ar.Crit, ar.CritChance)
	attack.Attacker, attack.GrazesDodge = ar.Attacker, ar.GrazesDodge
	attack.addPersonalSpellPayload(ar.FireAuraPercent, ar.FireAuraSchool, ar.VampiricPct)
	return attack
}

// dodgePartyShot resolves a party shot a Perfect Dodge avoids: what gets
// through lands (landed), else the dodge is announced. dodged is false when
// the victim did not dodge and the hit goes on.
func (cs *CombatSystem) dodgePartyShot(m *monsterPkg.Monster3D, attack partyMonsterAttack, attackerName, school, weaponName string, weaponDef *config.WeaponDefinitionConfig) (dodged, landed bool) {
	if !cs.monsterPerfectDodges(m) {
		return false, false
	}
	landed = cs.applyDamageThroughDodge(m, attack, attack.Attacker, attackerName, weaponDef)
	if !landed {
		cs.game.logCombatLine(logToneNone, uitext.Text("combat.dodges_the", logSlot, logSlot), logMonsterName(m), logSchoolWord(school, weaponName))
	}
	return true, landed
}

// reportProjectileHit is the log line of a party shot's hit, its school named:
// every leg of a shot reads alike.
func (cs *CombatSystem) reportProjectileHit(m *monsterPkg.Monster3D, name, school string, hit partyMonsterHit) {
	cs.game.reportDamage(logToneGood, damageLog{
		Source:   logSchoolWord(school, name),
		Target:   monsterLogTarget(m),
		Amount:   hit.Total(),
		School:   school,
		Critical: hit.Critical,
		Kind:     damageLogProjectile,
	})
}

// applyProjectileDamage applies damage from a projectile to a monster and generates combat messages
func (cs *CombatSystem) applyProjectileDamage(projectile interface{}, monster *monsterPkg.Monster3D, entityID string) {
	// Pure summons are transparent, including both legs of returning weapons.
	if isPurePartySummon(monster) {
		return
	}
	if ar, ok := projectile.(*Arrow); ok {
		if ar.Signature.ReturnLeg {
			if !ar.Active || ar.LifeTime <= 0 || slices.Contains(ar.Signature.LegHits, monster.ID) {
				return
			}
			cs.applyReturningBladeHit(ar, monster)
			return
		}
	}
	if shot, ok := projectile.(*Arrow); ok && shot.Backwash != nil {
		cs.resolveBackwashCharge(shot, monster)
		return
	}
	var damage int
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
	var starburstFx bool
	var ricochetArrow *Arrow
	var spellShot *MagicProjectile

	switch shot := projectile.(type) {
	case *MagicProjectile:
		if !shot.Active || shot.LifeTime <= 0 {
			return
		}
		damage = shot.Damage
		disintegrateChance = shot.DisintegrateChance
		spellID := spells.SpellID(shot.SpellType)
		spellDef, _ := spells.GetSpellDefinitionByID(spellID)
		weaponName = spellDef.Name
		damageTypeStr = spellDef.DamageType()
		aoeRadiusTiles = spellDef.AoeRadiusTiles
		isBindSpell = spellDef.BindUndead
		bindSeconds = spellDef.BindDurationSeconds
		isPacifySpell = spellDef.Pacify
		pacifySeconds = spellDef.PacifyDurationSeconds
		dealsNoDamage = spellDef.DealsNoDamage
		stunChance = spellDef.StunChance
		stunSeconds = spellDef.StunDurationSeconds
		starburstFx = spellDef.StarburstFx
		shot.Active = false
		isSpell = true
		spellShot = shot

	case *Arrow:
		if !shot.Active || shot.LifeTime <= 0 {
			return
		}
		ricochetArrow = shot
		damage = shot.Damage
		disintegrateChance = shot.DisintegrateChance
		weaponName = "Arrow"
		damageTypeStr = normalizeDamageTypeStr(shot.DamageType)
		shot.Active = false
		isRanged = true
		if shot.Owner == ProjectileOwnerPlayer && shot.BowKey != "" && shot.Label == "" {
			weaponDef = lookupWeaponConfigByKey(shot.BowKey)
			if weaponDef != nil {
				aoeRadiusTiles = weaponDef.AoeRadiusTiles
				if weaponDef.Name != "" {
					weaponName = weaponDef.Name
				}
			}
		}
		// A labeled bolt (Bandit Card proc) reports under its own name - the
		// BowKey above may be the hunting-bow PHYSICS fallback of a melee wielder.
		if shot.Label != "" {
			weaponName = shot.Label
		}
		// Arena Arbalest, and a returning blade on its way out: the bolt goes
		// through the target and flies on as a continuation that can never
		// re-hit the monster it pierced. Spawned before damage resolution so
		// even a killing hit passes through.
		if shot.Owner == ProjectileOwnerPlayer && (shot.PierceLeft > 0 || shot.Signature.Returning) {
			cont := *shot
			cont.PierceLeft = max(0, shot.PierceLeft-1)
			cont.SkipMonster = monster
			cont.Launch = shot.Launch.piercing(cs, monster)
			if shot.Signature.Returning {
				cont.Signature.LegHits = append(slices.Clone(shot.Signature.LegHits), monster.ID)
				shot.Signature.Returning = false // the continuation flies on and turns
			}
			cs.spawnArrowContinuation(cont, weaponDef)
		}
	case *FlaskProjectile:
		cs.detonateFlask(shot, shot.X, shot.Y)
		return
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
	// A spent fragment is still stopped by the body, but cannot deal more
	// damage or trigger any riders after this cast's per-target hit limit.
	var budget *spellVolleyHitBudget
	if spellShot != nil {
		budget = spellShot.HitBudget
	}
	if budget != nil && budget.hits[monster] >= budget.limit {
		cs.game.collisionSystem.UnregisterEntity(entityID)
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
	attackerName := partyAttackerName(attacker)

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
	resistPierce := 0
	if ar, ok := projectile.(*Arrow); ok {
		attack = cs.arrowAttack(ar, damageTypeStr, weaponDef, weaponName)
	} else {
		attack = cs.spellProjectileAttack(spellShot)
		damage, resistPierce = attack.Packet.normalDamage(), cs.spellResistPierce(attacker, spellShot.SpellType)
	}

	// Check monster perfect dodge (applies to all attack types): the normal hit
	// is dodged, but typed TRUE damage still lands and a Grandmaster's weapon
	// strike grazes through. A blast spell still goes off around a target that
	// dodged or was bound.
	if dodged, landed := cs.dodgePartyShot(monster, attack, attackerName, damageTypeStr, weaponName, weaponDef); dodged {
		if landed && budget != nil {
			budget.hits[monster]++
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

	outcome := rollMonsterDisintegrate(monster, disintegrateChance)
	if outcome == instakillKill {
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
	// Bind/Charm are handled earlier; this is the Disintegrate case. A boss's
	// sear is real damage and takes the ordinary hit path below.
	if dealsNoDamage && outcome != instakillSear {
		cs.spawnProjectileHitFX(projectile, fxX, fxY, isSpell, isRanged, damageTypeStr, monster, weaponDef, damage)
		cs.applyMonsterDamagePacket(
			monster,
			singleMonsterDamagePacket(damagecalc.Parts{}, damageTypeStr, resistPierce),
			monsterDamageOptions{IgnoreArmor: true},
		)
		cs.markMonsterHit(monster)
		cs.game.AddCombatMessage(uitext.Text("combat.has_no_effect_on", weaponName, monster.Name))
		cs.game.collisionSystem.UnregisterEntity(entityID)
		return
	}

	if arrow, ok := projectile.(*Arrow); !ok || !arrow.Continuation {
		attack = cs.consumeDodgeCharge(attack)
	}
	if budget != nil {
		budget.hits[monster]++
	}
	// The landed hit carries the riders; a boss's sear joins its damage only;
	// the splash keeps the source attack.
	landed := attack
	landed.PrimaryHit = true
	primary := landed
	if outcome == instakillSear {
		primary = cs.searDisintegrated(monster, landed)
	}
	hit := cs.applyPartyMonsterAttack(monster, primary)
	cs.spawnProjectileHitFX(projectile, fxX, fxY, isSpell, isRanged, damageTypeStr, monster, weaponDef, hit.SourceNormal)
	cs.markMonsterHit(monster)
	cs.reportProjectileHit(monster, weaponName, damageTypeStr, hit)
	shot, _ := projectile.(*Arrow)
	executed := cs.settlePartyHit(monster, weaponDef, attacker, attackerName, landed.resolvedAs(hit), shot, func() {
		cs.applyAdvancedSpellImpact(spellShot, monster)
		// Spell stun-on-hit (Psychic Shock): chance to stun the struck monster.
		if stunChance > 0 && rand.Float64() < stunChance {
			cs.applyStun(monster, stunSeconds, true) // announces stun/resist itself
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

// partySpellAttack is the packet a party spell lands: its rolled launch
// damage and crit, the live outgoing buff, true damage and resist pierce.
func (cs *CombatSystem) partySpellAttack(caster *character.MMCharacter, spellID string, launch damagecalc.Parts, crit bool) partyMonsterAttack {
	def, _ := spells.GetSpellDefinitionByID(spells.SpellID(spellID))
	school := def.DamageType()
	parts, _ := cs.spellPartsWithOutgoingBuff(launch, school)
	attack := cs.newPartyMonsterAttack(parts.Normal, parts.True, school, cs.spellResistPierce(caster, spellID), nil, def.Name, false, true, false)
	attack.Critical = crit
	attack.Attacker = caster
	return attack
}

// spellProjectileAttack is the packet a party spell projectile delivers; the
// direct hit and every blast of the shot share it.
func (cs *CombatSystem) spellProjectileAttack(mp *MagicProjectile) partyMonsterAttack {
	attack := cs.partySpellAttack(mp.Attacker, mp.SpellType, damagecalc.Parts{Normal: mp.Damage, True: mp.TrueDamage}, mp.Crit)
	attack.Projectile = true
	attack.GrazesDodge = mp.GrazesDodge
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
		cs.game.reportDamage(logToneGood, damageLog{
			Source: logSchoolWord(primarySchool, attack.WeaponName),
			Target: monsterLogTarget(m),
			Amount: actual,
			School: primarySchool,
			Kind:   damageLogSplash,
		})
		if !m.IsAlive() {
			cs.finishWeaponKill(m, attack.WeaponDef, attack.Attacker)
		}
	})
}

// applyAoeSplashAt owns ground and target-centered party splash alike: a weapon
// or spell hit, which a Dark Elf may turn into a binding. Source-specific
// status, presentation and kill riders run after the shared hit.
func (cs *CombatSystem) applyAoeSplashAt(x, y float64, attack partyMonsterAttack, radiusTiles float64, exclude *monsterPkg.Monster3D, onHit func(*monsterPkg.Monster3D, int)) {
	cs.forEachSplashVictim(x, y, radiusTiles, exclude, func(m *monsterPkg.Monster3D) {
		if !cs.tryDarkElfBindInstead(attack.Attacker, m) {
			onHit(m, cs.applyPartyMonsterAttack(m, attack).Total())
		}
	})
}

// forEachSplashVictim visits every monster a party splash at (x, y) can hurt.
// An optional primary exclusion avoids billing a projectile's direct victim
// twice. A point blast reaches only monsters with a clear attack line from
// (x, y), so walls shield the room behind them; novas keep their own policy.
func (cs *CombatSystem) forEachSplashVictim(x, y, radiusTiles float64, exclude *monsterPkg.Monster3D, visit func(*monsterPkg.Monster3D)) {
	if radiusTiles <= 0 {
		return
	}
	cs.forEachAreaVictim(cs.pointBlast(x, y, radiusTiles), partyAreaHurts, visit, exclude)
}

// partyAreaHurts is who a party splash or blast damages: everyone it covers
// but pure party summons and the invulnerable - bound allies included.
func partyAreaHurts(m *monsterPkg.Monster3D) bool {
	return !isPurePartySummon(m) && !m.IsDamageInvulnerable()
}

func (cs *CombatSystem) spawnArrowContinuation(cont Arrow, weaponDef *config.WeaponDefinitionConfig) {
	cont.Continuation = true
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
	return cs.nearestSeenMonster(victim.X, victim.Y, nil, cs.game.world.Monsters, maxDist, func(m *monsterPkg.Monster3D) bool {
		return m != victim && !isExcludedFromPartyAutoTarget(m)
	})
}
