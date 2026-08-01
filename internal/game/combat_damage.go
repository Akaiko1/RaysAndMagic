package game

import (
	"math"
	"math/rand"

	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
)

// monsterDamageComponent is one school inside a single hit. Components remain
// together through target mitigation so resistance is per school while flat
// soak is paid only once for the whole hit.
type monsterDamageComponent struct {
	Parts           damagecalc.Parts
	School          monsterPkg.DamageType
	ResistPiercePct int
}

type monsterDamagePacket struct {
	Components []monsterDamageComponent
}

func singleMonsterDamagePacket(parts damagecalc.Parts, school string, resistPiercePct int) monsterDamagePacket {
	damageType := convertToMonsterDamageType(school)
	return monsterDamagePacket{Components: []monsterDamageComponent{{
		Parts:           parts,
		School:          damageType,
		ResistPiercePct: resistPiercePct,
	}}}
}

func (p monsterDamagePacket) trueOnly() monsterDamagePacket {
	out := monsterDamagePacket{Components: make([]monsterDamageComponent, 0, len(p.Components))}
	for _, component := range p.Components {
		if component.Parts.True <= 0 {
			continue
		}
		component.Parts.Normal = 0
		out.Components = append(out.Components, component)
	}
	return out
}

type monsterDamageOptions struct {
	IsRanged       bool
	IgnoreArmor    bool
	ArmorPiercePct int
	// PostArmorNormal preserves the established combat order: armor first,
	// source/target multipliers second, resistance and one shared soak last.
	PostArmorNormal func(int) int
}

// applyMonsterDamagePacket is the only game-side monster mitigation boundary.
// It applies target armor to every normal component, then hands all schools to
// Monster3D together for per-school resistance and one shared flat soak.
func (cs *CombatSystem) applyMonsterDamagePacket(target *monsterPkg.Monster3D, packet monsterDamagePacket, options monsterDamageOptions) damagecalc.Parts {
	if target == nil || len(packet.Components) == 0 {
		return damagecalc.Parts{}
	}

	armorClass := target.EffectiveArmorClass()
	armorClass = armorAfterPierce(armorClass, options.ArmorPiercePct)

	components := make([]monsterPkg.DamageComponent, 0, len(packet.Components))
	for _, component := range packet.Components {
		parts := component.Parts
		if parts.Normal > 0 {
			if !options.IgnoreArmor {
				parts.Normal = applyMonsterArmor(parts.Normal, component.School.String(), armorClass, options.IsRanged)
			}
			if options.PostArmorNormal != nil {
				parts.Normal = options.PostArmorNormal(parts.Normal)
			}
		}
		components = append(components, monsterPkg.DamageComponent{
			Parts:           parts,
			DamageType:      component.School,
			ResistPiercePct: component.ResistPiercePct,
		})
	}
	return target.TakeDamagePacket(components)
}

func armorAfterPierce(armorClass, piercePct int) int {
	if armorClass <= 0 || piercePct <= 0 {
		return armorClass
	}
	if piercePct > 100 {
		piercePct = 100
	}
	return armorClass * (100 - piercePct) / 100
}

func monsterPerfectDodges(target *monsterPkg.Monster3D, ignoresDodge bool) bool {
	return target != nil && target.PerfectDodge > 0 && !ignoresDodge &&
		rand.Intn(100) < target.PerfectDodge
}

// partyMonsterAttack is the immutable source-side result shared by a primary
// target and every AoE victim. Target-specific bonuses and defenses resolve
// separately for each monster.
type partyMonsterAttack struct {
	Packet               monsterDamagePacket
	WeaponDef            *config.WeaponDefinitionConfig
	WeaponName           string
	IsRanged             bool
	IsSpell              bool
	IsMelee              bool
	IgnoreDodge          bool
	IgnoreArmor          bool
	ArmorIgnoreChancePct int
}

func (cs *CombatSystem) newPartyMonsterAttack(
	normal, trueDamage int,
	school string,
	resistPiercePct int,
	weaponDef *config.WeaponDefinitionConfig,
	weaponName string,
	isRanged, isSpell, isMelee bool,
) partyMonsterAttack {
	packet := cs.newPartyMonsterDamagePacket(normal, trueDamage, school, resistPiercePct, !isSpell)
	return partyMonsterAttack{
		Packet:     packet,
		WeaponDef:  weaponDef,
		WeaponName: weaponName,
		IsRanged:   isRanged,
		IsSpell:    isSpell,
		IsMelee:    isMelee,
	}
}

func (cs *CombatSystem) newPartyMonsterDamagePacket(
	normal, trueDamage int,
	school string,
	resistPiercePct int,
	convertPhysical bool,
) monsterDamagePacket {
	school = normalizeDamageTypeStr(school)
	packet := singleMonsterDamagePacket(
		damagecalc.Parts{Normal: normal, True: trueDamage},
		school,
		resistPiercePct,
	)
	if convertPhysical && school == monsterPkg.DamagePhysical.String() {
		remainder, shares := cs.game.splitPhysConversions(normal)
		packet.Components[0].Parts.Normal = remainder
		for _, share := range shares {
			packet.Components = append(packet.Components, monsterDamageComponent{
				Parts:  damagecalc.Parts{Normal: share.amount},
				School: convertToMonsterDamageType(share.element),
			})
		}
	}
	return packet
}

func applyRoundedMultiplier(damage int, multiplier float64, floorOne bool) int {
	if multiplier == 1 {
		return damage
	}
	damage = int(math.Round(float64(damage) * multiplier))
	if floorOne && damage < 1 {
		return 1
	}
	return damage
}

func (cs *CombatSystem) partyMonsterDamageOptions(attack partyMonsterAttack, target *monsterPkg.Monster3D) monsterDamageOptions {
	ignoreArmor := attack.IgnoreArmor
	if !ignoreArmor && attack.ArmorIgnoreChancePct > 0 &&
		rand.Intn(100) < attack.ArmorIgnoreChancePct {
		ignoreArmor = true
	}

	return monsterDamageOptions{
		IsRanged:       attack.IsRanged,
		IgnoreArmor:    ignoreArmor,
		ArmorPiercePct: weaponArmorPiercePct(attack.WeaponDef),
		PostArmorNormal: func(damage int) int {
			if !attack.IsSpell {
				damage = applyRoundedMultiplier(damage, cs.weaponBonusMultiplier(attack.WeaponDef, target), true)
				damage = applyRoundedMultiplier(damage, weaponStunnedBonusMultiplier(attack.WeaponDef, target), false)
			}
			damage = applyRoundedMultiplier(damage, cs.game.cardBonusVsMultiplier(target), true)
			if attack.IsMelee {
				damage = damage * (100 + cs.game.cardMeleeDmgPct()) / 100
			}
			return damage
		},
	}
}

func weaponArmorPiercePct(def *config.WeaponDefinitionConfig) int {
	if def == nil || def.ArmorPiercePct <= 0 {
		return 0
	}
	return def.ArmorPiercePct
}

func (cs *CombatSystem) monsterWeaponDamageOptions(
	def *config.WeaponDefinitionConfig,
	target *monsterPkg.Monster3D,
	isRanged, ignoreArmor bool,
) monsterDamageOptions {
	return monsterDamageOptions{
		IsRanged:       isRanged,
		IgnoreArmor:    ignoreArmor,
		ArmorPiercePct: weaponArmorPiercePct(def),
		PostArmorNormal: func(damage int) int {
			damage = applyRoundedMultiplier(damage, cs.weaponBonusMultiplier(def, target), true)
			return applyRoundedMultiplier(damage, weaponStunnedBonusMultiplier(def, target), false)
		},
	}
}

func (cs *CombatSystem) applyPartyMonsterAttack(target *monsterPkg.Monster3D, attack partyMonsterAttack) damagecalc.Parts {
	if isPurePartySummon(target) {
		return damagecalc.Parts{}
	}
	parts := cs.applyMonsterDamagePacket(target, attack.Packet, cs.partyMonsterDamageOptions(attack, target))
	if parts.Total() > 0 && (attack.IsMelee || attack.IsRanged) {
		cs.game.playMonsterSound(soundMonsterHit, target)
	}
	return parts
}

// weaponDamagePreview is the source-side, unmitigated result shown by tooltips
// and item comparisons. It mirrors the real ranged/melee ordering, including
// party-only cards, typed true damage and active outgoing buffs.
type weaponDamagePreview struct {
	FormulaNormal int
	Normal        int
	True          int
	AuthoredTrue  int
	Total         int
	CriticalTotal int
	CardDamagePct int
	CardTrue      int
	OutgoingBuff  int
}

func (cs *CombatSystem) calculateWeaponDamagePreview(item items.Item, char *character.MMCharacter) weaponDamagePreview {
	def := lookupWeaponConfigByName(item.Name)
	if def == nil {
		return weaponDamagePreview{}
	}
	normal := def.Damage
	if char != nil && cs != nil && cs.game != nil {
		_, _, normal = cs.CalculateWeaponDamage(item, char)
	}

	preview := weaponDamagePreview{
		FormulaNormal: normal,
		True:          def.TrueDamage,
		AuthoredTrue:  def.TrueDamage,
	}
	if char != nil && cs != nil && cs.game != nil {
		preview.True, _ = cs.weaponMasteryStrike(char, def)
		preview.OutgoingBuff = cs.game.combatBuffOutBonusForDamageType(weaponDamageTypeStr(def))
	}
	isRanged := def.Range > 3
	if char != nil && cs != nil && cs.game != nil && cs.game.isPartyMember(char) {
		if isRanged {
			preview.CardDamagePct = cs.game.cardRangedDmgPct()
		} else {
			preview.CardDamagePct = cs.game.cardMeleeDmgPct()
			preview.CardTrue = cs.game.cardMeleeTrueDmg()
			preview.True += preview.CardTrue
		}
	}

	if isRanged {
		preview.Normal = normal * (100 + preview.CardDamagePct) / 100
		preview.Normal += preview.OutgoingBuff
		critNormal := normal * (100 + preview.CardDamagePct) / 100
		preview.CriticalTotal = critNormal*CritDamageMultiplier + preview.OutgoingBuff + preview.True
	} else {
		preview.Normal = (normal + preview.OutgoingBuff) * (100 + preview.CardDamagePct) / 100
		critNormal := (normal*CritDamageMultiplier + preview.OutgoingBuff) * (100 + preview.CardDamagePct) / 100
		preview.CriticalTotal = critNormal + preview.True
	}
	preview.Total = preview.Normal + preview.True
	return preview
}
