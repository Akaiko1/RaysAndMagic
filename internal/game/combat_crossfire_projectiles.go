package game

import (
	"fmt"
	"math/rand"

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
	projectileType string,
	target *monsterPkg.Monster3D,
	entityID string,
) {
	if target == nil || target != projectileSourceMonster(projectile) || !target.IsAlive() {
		return
	}

	var parts damagecalc.Parts
	var damageTypeStr string
	var weaponDef *config.WeaponDefinitionConfig
	switch projectileType {
	case "magic_projectile":
		mp, ok := projectile.(*MagicProjectile)
		if !ok || !mp.Active || mp.LifeTime <= 0 || mp.Owner != ProjectileOwnerReflected {
			return
		}
		mp.Active = false
		parts = damagecalc.Parts{Normal: mp.Damage, True: mp.TrueDamage}
		damageTypeStr = spellDamageTypeStr(mp.SpellType)
		fxX, fxY := cs.monsterVisualPos(target)
		cs.game.CreateSpellHitEffectFromSpell(fxX, fxY, mp.SpellType)
	case "arrow":
		ar, ok := projectile.(*Arrow)
		if !ok || !ar.Active || ar.LifeTime <= 0 || ar.Owner != ProjectileOwnerReflected {
			return
		}
		ar.Active = false
		parts = damagecalc.Parts{Normal: ar.Damage, True: ar.TrueDamage}
		damageTypeStr = normalizeDamageTypeStr(ar.DamageType)
		weaponDef = lookupWeaponConfigByKey(ar.BowKey)
		cs.spawnRangedHitEffect(target, weaponDef, parts.Total())
	default:
		return
	}
	cs.game.collisionSystem.UnregisterEntity(entityID)

	dealt := cs.applyMonsterDamagePacket(
		target,
		singleMonsterDamagePacket(parts, damageTypeStr, 0),
		monsterDamageOptions{},
	).Total()
	if dealt > 0 {
		cs.game.playMonsterSound(soundMonsterHit, target)
		target.HitTintFrames = MonsterHitFlashFrames
	}
	if !target.IsAlive() {
		xpAwarded := cs.finishMonsterKill(target)
		cs.game.AddCombatMessage(fmt.Sprintf("The reflected bolt slays %s! (+%d XP)", target.Name, xpAwarded))
		return
	}
	cs.game.AddCombatMessage(fmt.Sprintf("The reflected bolt hits %s for %d!", target.Name, dealt))
}

// resolveMonsterProjectileVsMonster applies a monster-fired projectile's hit to
// another monster (bound undead <-> enemy crossfire). Damage is the projectile's
// own; the party is rewarded ONLY when an enemy falls (never for a bound ally).
func (cs *CombatSystem) resolveMonsterProjectileVsMonster(projectile interface{}, pType string, target *monsterPkg.Monster3D, entityID string) {
	var parts damagecalc.Parts
	var dmgTypeStr, spellFx, sourceName string
	var disintegrateChance, aoeRadiusTiles, stunChance float64
	var stunSeconds, stunTurns int
	var ignoresDodge bool
	var weaponDef *config.WeaponDefinitionConfig
	var srcMonster *monsterPkg.Monster3D
	var owner ProjectileOwner
	switch pType {
	case "magic_projectile":
		mp := projectile.(*MagicProjectile)
		if !mp.Active || mp.LifeTime <= 0 {
			return
		}
		mp.Active = false
		parts = damagecalc.Parts{Normal: mp.Damage, True: mp.TrueDamage}
		sourceName, spellFx = mp.SourceName, mp.SpellType
		disintegrateChance = mp.DisintegrateChance
		ignoresDodge = mp.IgnoresDodge
		srcMonster = mp.SourceMonster
		owner = mp.Owner
		spellDef, _ := spells.GetSpellDefinitionByID(spells.SpellID(mp.SpellType))
		dmgTypeStr = normalizeDamageTypeStr(spellDef.School)
		aoeRadiusTiles = spellDef.AoeRadiusTiles
		stunChance, stunSeconds, stunTurns = spellDef.StunChance, spellDef.StunDurationSeconds, spellDef.StunDurationTurns
	case "arrow":
		ar := projectile.(*Arrow)
		if !ar.Active || ar.LifeTime <= 0 {
			return
		}
		ar.Active = false
		parts = damagecalc.Parts{Normal: ar.Damage, True: ar.TrueDamage}
		sourceName = ar.SourceName
		disintegrateChance = ar.DisintegrateChance
		ignoresDodge = ar.IgnoresDodge
		dmgTypeStr = normalizeDamageTypeStr(ar.DamageType)
		srcMonster = ar.SourceMonster
		owner = ar.Owner
		// A weapon projectile carries its own AoE rider (a champion's archmage staff,
		// Bow of Hellfire): read it so the impact splashes nearby monsters, matching
		// the spell path above and the party-side splash.
		if ar.BowKey != "" {
			if wd, ok := config.GetWeaponDefinition(ar.BowKey); ok && wd != nil {
				weaponDef = wd
				if !ar.SuppressAoE {
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
	if sourceName == "" {
		sourceName = "A bolt"
	}
	if spellFx != "" {
		tx, ty := cs.monsterVisualPos(target) // banded/pulled: burst where the mob is DRAWN
		cs.game.CreateSpellHitEffectFromSpell(tx, ty, spellFx)
	}

	// kill finalizes a slain crossfire target through the same kill choke point as
	// a party hit, so a bound summon can earn champion rewards for the party too.
	kill := func() {
		cs.game.AddCombatMessage(fmt.Sprintf("%s is destroyed!", target.Name))
		cs.finishMonsterKillImmediately(target)
	}

	// The projectile already snapshotted source-side modifiers when fired.
	packet := singleMonsterDamagePacket(parts, dmgTypeStr, 0)
	ignoreArmor := srcMonster != nil && srcMonster.IgnoresArmor

	// Crossfire uses the target's real Perfect Dodge too. Typed true damage still
	// lands through a dodge; the avoided projectile cannot trigger riders or AoE,
	// matching a party projectile that misses its primary target.
	if monsterPerfectDodges(target, ignoresDodge) {
		actual := cs.applyMonsterDamagePacket(
			target,
			packet.trueOnly(),
			cs.monsterWeaponDamageOptions(weaponDef, target, true, true),
		).Total()
		if actual > 0 {
			cs.game.playMonsterSound(soundMonsterHit, target)
			target.HitTintFrames = MonsterHitFlashFrames
			cs.game.AddCombatMessage(fmt.Sprintf("%s dodges, but %s lands %d true damage!", target.Name, sourceName, actual))
			if !target.IsAlive() {
				kill()
			}
		} else {
			cs.game.AddCombatMessage(fmt.Sprintf("%s dodges %s's bolt!", target.Name, sourceName))
		}
		return
	}

	// Disintegrate rider: the bound mob's projectile keeps its instakill chance.
	if disintegrateChance > 0 && !monsterImmuneToDisintegrate(target) && rand.Float64() < disintegrateChance {
		target.HitPoints = 0
		target.HitTintFrames = MonsterHitFlashFrames
		cs.game.AddCombatMessage(fmt.Sprintf("%s's bolt disintegrates %s!", sourceName, target.Name))
		kill()
		return
	}

	if parts.Total() > 0 {
		actual := cs.applyMonsterDamagePacket(
			target,
			packet,
			cs.monsterWeaponDamageOptions(weaponDef, target, true, ignoreArmor),
		).Total()
		if actual > 0 {
			cs.game.playMonsterSound(soundMonsterHit, target)
		}
		target.HitTintFrames = MonsterHitFlashFrames
		cs.game.AddCombatMessage(fmt.Sprintf("%s's bolt hits %s for %d!", sourceName, target.Name, actual))
	}
	// Stun rider (Psychic Shock etc.) carries over too.
	if target.IsAlive() && stunChance > 0 && rand.Float64() < stunChance {
		cs.applyStun(target, stunSeconds, stunTurns) // announces stun/resist itself
	}
	if !target.IsAlive() {
		kill()
	}
	// AoE rider: crossfire may splash only the direct target's faction. Reusing
	// the player AoE path here would hit the firing champion and its own ordinary
	// allies, then incorrectly credit their deaths to the party.
	if aoeRadiusTiles > 0 {
		cs.applyCrossfireAoeSplash(target, srcMonster, owner, packet, weaponDef, ignoreArmor, aoeRadiusTiles)
		// A CHAMPION's AoE bolt that reaches the party strikes it too (the extra
		// action - the summon-splash's party twin). Plain mob crossfire never hits
		// the party, so this is gated to champions.
		if srcMonster != nil && srcMonster.IsChampion() &&
			Distance(target.X, target.Y, cs.game.camera.X, cs.game.camera.Y) <= aoeRadiusTiles*float64(cs.game.config.GetTileSize()) {
			hit := monsterCharacterHit{
				Parts:          parts, // already weakened once at packet build
				DamageType:     dmgTypeStr,
				IgnoresArmor:   srcMonster.IgnoresArmor,
				ArmorPiercePct: weaponArmorPiercePct(weaponDef),
				IgnoresDodge:   ignoresDodge,
				Spell:          weaponDef == nil, // a champion splash without a weapon behind it is a spell's
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
	ignoreArmor bool,
	radiusTiles float64,
) {
	if center == nil || source == nil || radiusTiles <= 0 {
		return
	}
	radius := radiusTiles * float64(cs.game.config.GetTileSize())
	for _, candidate := range cs.game.world.Monsters {
		if candidate == nil || candidate == center || candidate == source || !candidate.IsAlive() {
			continue
		}
		if owner == ProjectileOwnerBoundUndead {
			if !cs.boundAllyCanDamageMonster(candidate) {
				continue
			}
		} else if !candidate.Bound {
			continue
		}
		if Distance(center.X, center.Y, candidate.X, candidate.Y) <= radius {
			// An explosion cannot be Perfect-Dodged, but still uses the victim's
			// armor, resistance and one shared soak.
			cs.strikeMonsterPacketFor(source, candidate, packet, weaponDef, true, ignoreArmor, false, false)
		}
	}
}
