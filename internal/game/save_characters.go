package game

import (
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

func normalizeItemFromConfig(item *items.Item) {
	if item == nil {
		return
	}
	// Weapons refresh from weapons.yaml (name/rarity/stat rebalances reach saved
	// slots), keyed by display name. Type-gated so a non-weapon that happens to
	// share a name can't be force-converted into a weapon.
	if item.Type == items.ItemWeapon {
		if _, key, ok := config.GetWeaponDefinitionByName(item.Name); ok && key != "" {
			if template, err := items.TryCreateWeaponFromYAML(key); err == nil {
				instanceID := item.InstanceID
				*item = template
				item.InstanceID = instanceID
			}
		}
		return
	}
	// Trap quick-slot items refresh from traps.yaml (name/cost rebalances
	// reach saved slots), keyed by SpellEffect.
	if item.Type == items.ItemTrap {
		if fresh, ok := config.TrapItem(string(item.SpellEffect)); ok {
			*item = fresh
		}
		return
	}
	switch item.Type {
	case items.ItemArmor, items.ItemAccessory, items.ItemConsumable, items.ItemQuest, items.ItemTrinket, items.ItemCard:
	default:
		return
	}
	_, key, ok := config.GetItemDefinitionByName(item.Name)
	if !ok || key == "" {
		return
	}
	template, err := items.TryCreateItemFromYAML(key)
	if err != nil {
		return
	}
	// Adopt the def's current type, so cards saved before they became their own
	// type (was "trinket") migrate to items.ItemCard on load.
	item.Type = template.Type
	// The YAML definition is the single source of an item's attributes: ADOPT
	// it wholesale, so rebalanced (or removed) values reach items saved before
	// the change. Items carry no instance state in Attributes - merging only
	// missing keys left old saves frozen on their original balance forever.
	item.Attributes = make(map[string]int, len(template.Attributes))
	for k, v := range template.Attributes {
		item.Attributes[k] = v
	}
	item.ArmorCategory = template.ArmorCategory
	// Rarity, description, and set membership are definitional too (items carry
	// no per-instance override), so adopt them from YAML. This lets newly added
	// sets activate for equipment already present in an older save.
	item.Description = template.Description
	item.Rarity = template.Rarity
	item.Set = template.Set
}

// restoreCharacterSave reconstructs one character (active or reserve) from a save.
func restoreCharacterSave(cs CharacterSave) *character.MMCharacter {
	m := &character.MMCharacter{
		Name:             cs.Name,
		Class:            character.CharacterClass(cs.Class),
		Race:             cs.Race,
		Promotion:        character.Promotion(cs.Promotion),
		Level:            cs.Level,
		Experience:       cs.Experience,
		HitPoints:        cs.HitPoints,
		MaxHitPoints:     cs.MaxHitPoints,
		SpellPoints:      cs.SpellPoints,
		MaxSpellPoints:   cs.MaxSpellPoints,
		Might:            cs.Might,
		Intellect:        cs.Intellect,
		Personality:      cs.Personality,
		Endurance:        cs.Endurance,
		Accuracy:         cs.Accuracy,
		Speed:            cs.Speed,
		Luck:             cs.Luck,
		FreeStatPoints:   cs.FreeStatPoints,
		PermanentBonuses: character.StatBonusesFromMap(cs.PermanentBonuses),
		OwedLevelChoices: append([]int(nil), cs.OwedLevelChoices...),
		Skills:           make(map[character.SkillType]*character.Skill),
		MagicSchools:     make(map[character.MagicSchoolID]*character.MagicSkill),
		Equipment:        make(map[items.EquipSlot]items.Item),
	}
	if len(cs.Conditions) > 0 {
		m.Conditions = make([]character.Condition, len(cs.Conditions))
		for i, c := range cs.Conditions {
			m.Conditions[i] = character.Condition(c)
		}
	}
	for _, s := range cs.Skills {
		mastery := character.SkillMastery(s.Mastery)
		if migrated := character.MasteryForLevel(s.Level); migrated > mastery {
			mastery = migrated
		}
		m.Skills[character.SkillType(s.Type)] = &character.Skill{Mastery: mastery}
	}
	for _, me := range cs.MagicSchools {
		mk := character.MagicSchoolID(me.School)
		mastery := character.SkillMastery(me.Mastery)
		if migrated := character.MasteryForLevel(me.Level); migrated > mastery {
			mastery = migrated
		}
		ms := &character.MagicSkill{Mastery: mastery}
		if len(me.KnownSpells) > 0 {
			ms.KnownSpells = make([]spells.SpellID, len(me.KnownSpells))
			for i, s := range me.KnownSpells {
				ms.KnownSpells[i] = spells.SpellID(s)
			}
		}
		m.MagicSchools[mk] = ms
	}
	for _, eq := range cs.Equipment {
		item := eq.Item
		normalizeItemFromConfig(&item)
		m.Equipment[items.EquipSlot(eq.Slot)] = item
	}
	for _, qs := range cs.QuickSlots {
		if qs.Slot < 0 || qs.Slot >= character.QuickSlotCount {
			continue
		}
		item := qs.Item
		normalizeItemFromConfig(&item)
		m.QuickSlots[qs.Slot] = &item
	}
	m.PoisonFramesRemaining = cs.PoisonFramesRemaining
	m.BurnFramesRemaining = cs.BurnFramesRemaining
	m.RestoreDoTTickTimers(cs.PoisonTickTimer, cs.BurnTickTimer)
	m.StunFramesRemaining = cs.StunFramesRemaining
	m.StunTurnsRemaining = cs.StunTurnsRemaining
	m.StunRate = cs.StunRate
	m.ActionsRemaining = cs.ActionsRemaining
	m.TBRoundActionFloor = cs.TBRoundActionFloor
	if m.TBRoundActionFloor <= 0 && m.ActionsRemaining > 0 {
		// Legacy saves predate the credited-floor field. Recover the floor
		// from their restored equipment without refilling any action.
		m.TBRoundActionFloor = tbPersonalActionFloor(m)
	}
	m.RTCooldown = cs.RTCooldown
	m.OffHandRTCooldown = cs.OffHandRTCooldown
	m.NextTBAttackOffHand = cs.NextTBAttackOffHand
	return m
}

// buildCharacterSave serializes one character (active or reserve).
func buildCharacterSave(m *character.MMCharacter) CharacterSave {
	cs := CharacterSave{
		Name:             m.Name,
		Class:            int(m.Class),
		Race:             m.Race,
		Promotion:        int(m.Promotion),
		Level:            m.Level,
		Experience:       m.Experience,
		HitPoints:        m.HitPoints,
		MaxHitPoints:     m.MaxHitPoints,
		SpellPoints:      m.SpellPoints,
		MaxSpellPoints:   m.MaxSpellPoints,
		Might:            m.Might,
		Intellect:        m.Intellect,
		Personality:      m.Personality,
		Endurance:        m.Endurance,
		Accuracy:         m.Accuracy,
		Speed:            m.Speed,
		Luck:             m.Luck,
		FreeStatPoints:   m.FreeStatPoints,
		OwedLevelChoices: append([]int(nil), m.OwedLevelChoices...),
	}
	if m.PermanentBonuses != (character.StatBonuses{}) {
		cs.PermanentBonuses = m.PermanentBonuses.ToMap()
	}
	if len(m.Conditions) > 0 {
		cs.Conditions = make([]int, len(m.Conditions))
		for i, c := range m.Conditions {
			cs.Conditions[i] = int(c)
		}
	}
	for t, s := range m.Skills {
		cs.Skills = append(cs.Skills, SkillEntry{Type: int(t), Level: s.Level(), Mastery: int(s.Mastery)})
	}
	for school, ms := range m.MagicSchools {
		entry := MagicSchoolEntry{School: string(school), Level: ms.Level(), Mastery: int(ms.Mastery)}
		if len(ms.KnownSpells) > 0 {
			entry.KnownSpells = make([]string, len(ms.KnownSpells))
			for i, sp := range ms.KnownSpells {
				entry.KnownSpells[i] = string(sp)
			}
		}
		cs.MagicSchools = append(cs.MagicSchools, entry)
	}
	for slot, item := range m.Equipment {
		cs.Equipment = append(cs.Equipment, EquipmentEntry{Slot: int(slot), Item: item})
	}
	for i, item := range m.QuickSlots {
		if item != nil {
			cs.QuickSlots = append(cs.QuickSlots, QuickSlotEntry{Slot: i, Item: *item})
		}
	}
	cs.PoisonFramesRemaining = m.PoisonFramesRemaining
	cs.BurnFramesRemaining = m.BurnFramesRemaining
	cs.PoisonTickTimer, cs.BurnTickTimer = m.DoTTickTimers()
	cs.StunFramesRemaining = m.StunFramesRemaining
	cs.StunTurnsRemaining = m.StunTurnsRemaining
	cs.StunRate = m.StunRate
	cs.ActionsRemaining = m.ActionsRemaining
	cs.TBRoundActionFloor = m.TBRoundActionFloor
	cs.RTCooldown = m.RTCooldown
	cs.OffHandRTCooldown = m.OffHandRTCooldown
	cs.NextTBAttackOffHand = m.NextTBAttackOffHand
	return cs
}
