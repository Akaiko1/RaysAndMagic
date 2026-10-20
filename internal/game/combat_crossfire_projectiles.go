package game

import (
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

func projectileSourceMonster(projectile interface{}) *monsterPkg.Monster3D {
	switch p := projectile.(type) {
	case *MagicProjectile:
		return p.SourceMonster
	case *Arrow:
		return p.SourceMonster
	default:
		return nil
	}
}

// resolveReflectedMonsterProjectile consumes an Aegis return only when it
// reaches the original shooter. It preserves the old reflection contract:
// same snapshotted payload and school, target mitigation, but no projectile
// riders, splash, ricochet, or armor-pierce inheritance.
func (cs *CombatSystem) resolveReflectedMonsterProjectile(
	projectile interface{},
	target *monsterPkg.Monster3D,
	entityID string,
) {
	if target == nil || target != projectileSourceMonster(projectile) || !target.IsAlive() {
		return
	}

	var parts damagecalc.Parts
	var damageTypeStr string
	var weaponDef *config.WeaponDefinitionConfig
	switch shot := projectile.(type) {
	case *MagicProjectile:
		if !shot.Active || shot.LifeTime <= 0 || shot.Owner != ProjectileOwnerReflected {
			return
		}
		shot.Active = false
		parts = damagecalc.Parts{Normal: shot.Damage, True: shot.TrueDamage}
		damageTypeStr = spellDamageTypeStr(shot.SpellType)
		fxX, fxY := cs.monsterVisualPos(target)
		cs.game.CreateSpellHitEffectFromSpell(fxX, fxY, shot.SpellType, visualAnchorFor(target))
	case *Arrow:
		if !shot.Active || shot.LifeTime <= 0 || shot.Owner != ProjectileOwnerReflected {
			return
		}
		shot.Active = false
		parts = damagecalc.Parts{Normal: shot.Damage, True: shot.TrueDamage}
		damageTypeStr = normalizeDamageTypeStr(shot.DamageType)
		weaponDef = lookupWeaponConfigByKey(shot.BowKey)
		cs.spawnRangedHitEffect(target, weaponDef, parts.Total())
	default:
		return
	}
	cs.game.collisionSystem.UnregisterEntity(entityID)

	hit := cs.applyMonsterDamagePacket(
		target,
		singleMonsterDamagePacket(parts, damageTypeStr, 0),
		monsterDamageOptions{Projectile: true},
	)
	dealt := hit.Total()
	if dealt > 0 {
		cs.game.playMonsterSound(soundMonsterHit, target)
		target.HitTintFrames = MonsterHitFlashFrames
	}
	cs.game.reportDamage(logToneGood, damageLog{
		Source: logSchoolWord(damageTypeStr, "The reflected bolt"),
		Target: monsterLogTarget(target),
		Amount: dealt,
		School: damageTypeStr,
		Shares: dealtShares(hit),
	})
	if !target.IsAlive() {
		cs.finishMonsterKill(target)
	}
}

// resolveMonsterProjectileVsMonster applies a monster-fired projectile's hit to
// another monster (party ally <-> enemy crossfire). Damage is the projectile's
// own; a kill is finalized like any other, so a fallen enemy or bound ally
// rewards the party and a pure party summon gives nothing.
func (cs *CombatSystem) resolveMonsterProjectileVsMonster(projectile interface{}, target *monsterPkg.Monster3D, entityID string) {
	var parts damagecalc.Parts
	var dmgTypeStr, spellFx, sourceName string
	var disintegrateChance, aoeRadiusTiles float64
	var grazesDodge, crit bool
	var weaponDef *config.WeaponDefinitionConfig
	var srcMonster *monsterPkg.Monster3D
	var owner ProjectileOwner
	switch shot := projectile.(type) {
	case *MagicProjectile:
		if !shot.Active || shot.LifeTime <= 0 {
			return
		}
		shot.Active = false
		crit = shot.Crit
		parts = damagecalc.Parts{Normal: shot.Damage, True: shot.TrueDamage}
		sourceName, spellFx = shot.SourceName, shot.SpellType
		disintegrateChance = shot.DisintegrateChance
		grazesDodge = shot.GrazesDodge
		srcMonster = shot.SourceMonster
		owner = shot.Owner
		spellDef, _ := spells.GetSpellDefinitionByID(spells.SpellID(shot.SpellType))
		dmgTypeStr = spellDef.DamageType()
		aoeRadiusTiles = spellDef.AoeRadiusTiles
	case *Arrow:
		if !shot.Active || shot.LifeTime <= 0 {
			return
		}
		shot.Active = false
		crit = shot.Crit
		parts = damagecalc.Parts{Normal: shot.Damage, True: shot.TrueDamage}
		sourceName = shot.SourceName
		disintegrateChance = shot.DisintegrateChance
		grazesDodge = shot.GrazesDodge
		dmgTypeStr = normalizeDamageTypeStr(shot.DamageType)
		srcMonster = shot.SourceMonster
		owner = shot.Owner
		// A weapon projectile carries its own AoE rider (a champion's archmage staff,
		// Bow of Hellfire): read it so the impact splashes nearby monsters, matching
		// the spell path above and the party-side splash.
		if shot.BowKey != "" {
			if wd, ok := config.GetWeaponDefinition(shot.BowKey); ok && wd != nil {
				weaponDef = wd
				if !shot.SuppressAoE {
					aoeRadiusTiles = wd.AoeRadiusTiles
				}
			}
		}
	default:
		return
	}
	cs.game.collisionSystem.UnregisterEntity(entityID)
	// Target already slain this frame (another hit landed first): consume the
	// projectile but don't re-damage or double-reward.
	if !target.IsAlive() {
		return
	}
	cs.game.notifyCaravanAttack(target)
	if sourceName == "" {
		sourceName = "A bolt"
	}
	if spellFx != "" {
		tx, ty := cs.monsterVisualPos(target) // banded/pulled: burst where the mob is DRAWN
		cs.game.CreateSpellHitEffectFromSpell(tx, ty, spellFx, visualAnchorFor(target))
	}

	// The projectile already snapshotted source-side modifiers when fired.
	// A dodged bolt cannot trigger riders or AoE, matching a party projectile
	// that misses its primary target; a boss takes the disintegrate sear on
	// this hit only, never on the splash.
	packet := singleMonsterDamagePacket(parts, dmgTypeStr, 0)
	if cs.landMonsterOnMonster(srcMonster, target, monsterOnMonsterHit{
		packet: packet, source: sourceName, opts: cs.monsterWeaponDamageOptions(weaponDef, target, true),
		canDodge: true, grazesDodge: grazesDodge, disintegrate: disintegrateChance, spellID: spellFx,
		kind: damageLogBolt, evades: uitext.Text("combat.noun.bolt"), riders: true, critical: crit,
	}) != monsterHitStruck {
		return
	}
	// AoE rider: crossfire may splash only the direct target's faction. Reusing
	// the player AoE path here would hit the firing champion and its own ordinary
	// allies, then incorrectly credit their deaths to the party.
	if aoeRadiusTiles > 0 {
		cs.applyCrossfireAoeSplash(target, srcMonster, owner, packet, weaponDef, aoeRadiusTiles)
		// A CHAMPION's AoE bolt that reaches the party strikes it too (the extra
		// action - the summon-splash's party twin). Plain mob crossfire never hits
		// the party, so this is gated to champions. Walls shield the party like
		// any other blast victim.
		if srcMonster != nil && srcMonster.IsChampion() && cs.reachesParty(cs.pointBlast(target.X, target.Y, aoeRadiusTiles)) {
			hit := monsterCharacterHit{
				SpellID:        spellFx,
				Parts:          parts, // already weakened once at packet build
				DamageType:     dmgTypeStr,
				ArmorPiercePct: weaponArmorPiercePct(weaponDef),
				GrazesDodge:    grazesDodge,
				Spell:          weaponDef == nil, // a champion splash without a weapon behind it is a spell's
				Projectile:     true,
			}
			cs.forEachDamageablePartyMember(func(_ int, member *character.MMCharacter) {
				cs.monsterHitCharacter(srcMonster, member, srcMonster.Name, hit)
			})
		}
	}
}

// applyCrossfireAoeSplash applies an AoE monster projectile to the same faction
// as its direct target. Bound allies splash enemy monsters; enemies splash bound
// allies. The firing monster is never a splash target, so a close-range AoE
// cannot self-kill a champion or damage its own pack.
func (cs *CombatSystem) applyCrossfireAoeSplash(
	center, source *monsterPkg.Monster3D,
	owner ProjectileOwner,
	packet monsterDamagePacket,
	weaponDef *config.WeaponDefinitionConfig,
	radiusTiles float64,
) {
	if center == nil || source == nil || radiusTiles <= 0 {
		return
	}
	hurts := source.CanAttackActor
	if owner == ProjectileOwnerPartyAlly {
		hurts = cs.boundAllyCanDamageMonster
	}
	// An explosion cannot be Perfect-Dodged, but still uses the victim's armor,
	// resistance and one shared soak.
	cs.forEachAreaVictim(cs.pointBlast(center.X, center.Y, radiusTiles), hurts, func(candidate *monsterPkg.Monster3D) {
		cs.strikeMonsterPacketFor(source, candidate, packet, weaponDef, true, false, false, false)
	}, center, source)
}
