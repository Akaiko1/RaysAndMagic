package character

import (
	"fmt"

	"ugataima/internal/config"
	"ugataima/internal/items"
)

// BuildChampion constructs a full MMCharacter from a champions.yaml build at
// one difficulty tier: the class kit, every authored skill raised to the
// tier's mastery, the tier's equipment list, at the tier's level. Combat code
// mirrors this character's derived numbers onto a monster
// (game.mirrorChampionStats); it is never a party member. Returns an error on
// any unresolvable key so startup fails loud instead of shipping a gimped
// champion.
func BuildChampion(def *config.ChampionDefinition, tier *config.ChampionTier, tierName string, cfg *config.Config) (*MMCharacter, error) {
	if def == nil || tier == nil {
		return nil, fmt.Errorf("nil champion definition or tier")
	}
	class, ok := ClassFromKey(def.Class)
	if !ok {
		return nil, fmt.Errorf("champion %q: unknown class %q", def.Name, def.Class)
	}
	char := CreateCharacter(def.Name, class, cfg)

	if def.Race != "" {
		if _, ok := cfg.Characters.Races[def.Race]; !ok {
			return nil, fmt.Errorf("champion %q: unknown race %q", def.Name, def.Race)
		}
		char.ApplyRace(def.Race, cfg)
	}

	// Stats stay at the class/race base here; the game side levels the champion
	// and spends its stat points via the party's own auto-distribution
	// (buildChampionTemplate), so builds never hand-author attributes.
	char.Level = tier.Level

	// Every authored skill sits at the tier's mastery (expert/master/grandmaster).
	mastery, ok := masteryFromKey(tier.Mastery)
	if !ok {
		return nil, fmt.Errorf("champion tier %q: unknown mastery %q", tierName, tier.Mastery)
	}
	for _, skillKey := range def.Skills {
		st, ok := SkillTypeFromKey(skillKey)
		if !ok {
			return nil, fmt.Errorf("champion %q: unknown skill %q", def.Name, skillKey)
		}
		char.Skills[st] = &Skill{Mastery: mastery}
	}

	// Authored spell schools sit at the tier's mastery too - the caster dual of
	// the weapon skills above. A school the class kit already opened is raised
	// in place so its kit-known spells survive.
	for _, schoolKey := range def.SpellSchools {
		school := MagicSchoolID(schoolKey)
		if !isKnownMagicSchool(school) {
			return nil, fmt.Errorf("champion %q: unknown spell school %q", def.Name, schoolKey)
		}
		if ms, ok := char.MagicSchools[school]; ok && ms != nil {
			ms.Mastery = mastery
		} else {
			char.MagicSchools[school] = &MagicSkill{Mastery: mastery}
		}
	}

	// The tier's equipment is authoritative - drop the class-kit loadout first.
	// The first weapon lands in the main hand, a second weapon in the off hand
	// (dual wield); everything else routes by its own equip_slot. The same
	// skill gates the party obeys apply: a build cannot wield or wear what its
	// skills don't allow.
	char.Equipment = make(map[items.EquipSlot]items.Item)
	mainHandSet := false
	for _, key := range def.Equipment[tierName] {
		if weapon, err := items.TryCreateWeaponFromYAML(key); err == nil {
			if !char.CanEquipWeaponByName(weapon.Name) {
				return nil, fmt.Errorf("champion %q tier %q: lacks the skill to wield %q", def.Name, tierName, key)
			}
			slot := items.SlotMainHand
			if mainHandSet {
				slot = items.SlotOffHand
			}
			char.Equipment[slot] = weapon
			mainHandSet = true
			continue
		}
		it, err := items.TryCreateItemFromYAML(key)
		if err != nil {
			return nil, fmt.Errorf("champion %q tier %q: equipment %q is neither a weapon nor an item", def.Name, tierName, key)
		}
		if it.Type == items.ItemArmor && !char.CanEquipArmor(it) {
			return nil, fmt.Errorf("champion %q tier %q: lacks the skill to wear %q", def.Name, tierName, key)
		}
		char.Equipment[it.PreferredSlot(items.SlotArmor)] = it
	}

	// Recompute MaxHP/MaxSP from the final stats, level and gear.
	char.CalculateDerivedStats(cfg)
	char.HitPoints = char.MaxHitPoints
	char.SpellPoints = char.MaxSpellPoints
	return char, nil
}
