package main

// Content card model + builders for the Items & Spells tab. Each YAML config
// (weapons, items, spells) gets a per-kind builder that produces a single
// `contentCard` carrying both the short summary line and the full tooltip
// rows. The page renderer (content_page.go) doesn't need to know what kind
// of thing a card represents — it just lays them out by section.

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"ugataima/internal/config"
	"ugataima/internal/spells"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

// contentCard is a single entry on the Items & Spells page.
type contentCard struct {
	kind     contentKind
	section  string // section header this card belongs under
	key      string // YAML key (used to resolve icon filename)
	name     string
	subtitle string // short stat line shown on the card

	// Tooltip-only fields (full data).
	description string
	flavor      string
	tooltipRows []string // pre-formatted "Label: value" rows
}

type contentKind int

const (
	cardWeapon contentKind = iota
	cardItem
	cardSpell
)

// buildContentCards walks the loaded YAML configs and assembles a flat list
// of cards grouped by section. Order of sections is fixed; within a section
// cards are sorted by name. Runs once at startup.
func buildContentCards() []contentCard {
	var cards []contentCard
	cards = append(cards, buildWeaponCards()...)
	cards = append(cards, buildItemCards()...)
	cards = append(cards, buildSpellCards()...)
	return cards
}

func buildWeaponCards() []contentCard {
	if config.GlobalWeapons == nil {
		return nil
	}
	byCategory := map[string][]string{}
	for key, def := range config.GlobalWeapons.Weapons {
		if def == nil {
			continue
		}
		cat := strings.ToLower(strings.TrimSpace(def.Category))
		if cat == "" {
			cat = "weapon"
		}
		byCategory[cat] = append(byCategory[cat], key)
	}
	categories := make([]string, 0, len(byCategory))
	for cat := range byCategory {
		categories = append(categories, cat)
	}
	sort.Strings(categories)
	var cards []contentCard
	for _, cat := range categories {
		keys := byCategory[cat]
		sort.SliceStable(keys, func(i, j int) bool {
			return config.GlobalWeapons.Weapons[keys[i]].Name < config.GlobalWeapons.Weapons[keys[j]].Name
		})
		section := "Weapons — " + titleCase(strings.ReplaceAll(cat, "_", " "))
		for _, key := range keys {
			cards = append(cards, weaponCard(section, key, config.GlobalWeapons.Weapons[key]))
		}
	}
	return cards
}

func buildItemCards() []contentCard {
	if config.GlobalItems == nil {
		return nil
	}
	// Stable section order for known item types; unknown types go last.
	typeOrder := []string{"armor", "accessory", "consumable", "quest"}
	typeLabel := map[string]string{
		"armor":      "Armor",
		"accessory":  "Accessories",
		"consumable": "Consumables",
		"quest":      "Quest Items",
	}
	byType := map[string][]string{}
	for key, def := range config.GlobalItems.Items {
		if def == nil {
			continue
		}
		t := strings.ToLower(strings.TrimSpace(def.Type))
		if t == "" {
			t = "item"
		}
		byType[t] = append(byType[t], key)
	}
	var cards []contentCard
	emit := func(t string) {
		keys := byType[t]
		if len(keys) == 0 {
			return
		}
		sort.SliceStable(keys, func(i, j int) bool {
			return config.GlobalItems.Items[keys[i]].Name < config.GlobalItems.Items[keys[j]].Name
		})
		label, ok := typeLabel[t]
		if !ok {
			label = titleCase(t)
		}
		for _, key := range keys {
			cards = append(cards, itemCard(label, key, config.GlobalItems.Items[key]))
		}
		delete(byType, t)
	}
	for _, t := range typeOrder {
		emit(t)
	}
	leftover := make([]string, 0, len(byType))
	for t := range byType {
		leftover = append(leftover, t)
	}
	sort.Strings(leftover)
	for _, t := range leftover {
		emit(t)
	}
	return cards
}

func buildSpellCards() []contentCard {
	if config.GlobalSpells == nil {
		return nil
	}
	battleBySchool := map[string][]string{}
	utilityBySchool := map[string][]string{}
	for key, def := range config.GlobalSpells.Spells {
		if def == nil {
			continue
		}
		school := strings.ToLower(strings.TrimSpace(def.School))
		if school == "" {
			school = "spell"
		}
		if def.IsUtility {
			utilityBySchool[school] = append(utilityBySchool[school], key)
		} else {
			battleBySchool[school] = append(battleBySchool[school], key)
		}
	}
	emit := func(cards []contentCard, prefix string, group map[string][]string) []contentCard {
		schools := make([]string, 0, len(group))
		for s := range group {
			schools = append(schools, s)
		}
		sort.Strings(schools)
		for _, school := range schools {
			keys := group[school]
			sort.SliceStable(keys, func(i, j int) bool {
				return config.GlobalSpells.Spells[keys[i]].Name < config.GlobalSpells.Spells[keys[j]].Name
			})
			section := prefix + " — " + titleCase(school)
			for _, key := range keys {
				cards = append(cards, spellCard(section, key, config.GlobalSpells.Spells[key]))
			}
		}
		return cards
	}
	var cards []contentCard
	cards = emit(cards, "Battle Spells", battleBySchool)
	cards = emit(cards, "Utility Spells", utilityBySchool)
	return cards
}

func weaponCard(section, key string, def *config.WeaponDefinitionConfig) contentCard {
	subtitle := fmt.Sprintf("Dmg %d  Range %d", def.Damage, def.Range)
	if def.BonusStat != "" {
		subtitle += "  +" + def.BonusStat
	}
	var rows []string
	rows = appendRow(rows, "Damage", fmt.Sprintf("%d", def.Damage))
	rows = appendRow(rows, "Range", fmt.Sprintf("%d tiles", def.Range))
	rows = appendRow(rows, "Bonus stat", def.BonusStat)
	rows = appendRow(rows, "Bonus stat (2nd)", def.BonusStatSecondary)
	if def.DamageType != "" && def.DamageType != "physical" {
		rows = appendRow(rows, "Damage type", titleCase(def.DamageType))
	}
	if def.CritChance > 0 {
		rows = appendRow(rows, "Crit chance", fmt.Sprintf("%d%%", def.CritChance))
	}
	if def.StunChance > 0 {
		turns := def.StunTurns
		if turns <= 0 {
			turns = 1
		}
		rows = appendRow(rows, "Stun chance", fmt.Sprintf("%.0f%% (%d turns)", def.StunChance*100, turns))
	}
	if def.DisintegrateChance > 0 {
		rows = appendRow(rows, "Disintegrate chance", fmt.Sprintf("%.0f%%", def.DisintegrateChance*100))
	}
	if def.MaxProjectiles > 0 {
		rows = appendRow(rows, "Max airborne", fmt.Sprintf("%d", def.MaxProjectiles))
	}
	if def.Rarity != "" {
		rows = appendRow(rows, "Rarity", titleCase(def.Rarity))
	}
	if def.Value > 0 {
		rows = appendRow(rows, "Value", fmt.Sprintf("%d gold", def.Value))
	}
	return contentCard{
		kind:        cardWeapon,
		section:     section,
		key:         key,
		name:        def.Name,
		subtitle:    subtitle,
		description: def.Description,
		tooltipRows: rows,
	}
}

func itemCard(section, key string, def *config.ItemDefinitionConfig) contentCard {
	subtitle := itemSubtitle(def)

	var rows []string
	rows = appendRow(rows, "Type", titleCase(def.Type))
	if def.ArmorType != "" {
		rows = appendRow(rows, "Armor category", titleCase(def.ArmorType))
	}
	if def.EquipSlot != "" {
		rows = appendRow(rows, "Equip slot", titleCase(def.EquipSlot))
	}
	if def.ArmorClassBase > 0 {
		rows = appendRow(rows, "Armor class base", fmt.Sprintf("%d", def.ArmorClassBase))
	}
	if def.EnduranceScalingDivisor > 0 {
		rows = appendRow(rows, "Endurance scaling", fmt.Sprintf("+1 per %d Endurance", def.EnduranceScalingDivisor))
	}
	if def.IntellectScalingDivisor > 0 {
		rows = appendRow(rows, "Spell power", fmt.Sprintf("+Intellect/%d", def.IntellectScalingDivisor))
	}
	if def.PersonalityScalingDivisor > 0 {
		rows = appendRow(rows, "Spell points", fmt.Sprintf("+Personality/%d", def.PersonalityScalingDivisor))
	}
	bonusStats := []struct {
		label string
		val   int
	}{
		{"Might bonus", def.BonusMight},
		{"Intellect bonus", def.BonusIntellect},
		{"Personality bonus", def.BonusPersonality},
		{"Endurance bonus", def.BonusEndurance},
		{"Accuracy bonus", def.BonusAccuracy},
		{"Speed bonus", def.BonusSpeed},
		{"Luck bonus", def.BonusLuck},
	}
	for _, b := range bonusStats {
		if b.val > 0 {
			rows = appendRow(rows, b.label, fmt.Sprintf("+%d", b.val))
		}
	}
	if def.HealBase > 0 {
		rows = appendRow(rows, "Heal base", fmt.Sprintf("%d HP", def.HealBase))
	}
	if def.HealEnduranceDivisor > 0 {
		rows = appendRow(rows, "Heal scaling", fmt.Sprintf("+Endurance/%d", def.HealEnduranceDivisor))
	}
	if def.SummonDistanceTiles > 0 {
		rows = appendRow(rows, "Summon distance", fmt.Sprintf("%d tiles", def.SummonDistanceTiles))
	}
	if def.Revive {
		rows = appendRow(rows, "Effect", "Revive from Dead/Unconscious")
	}
	if def.FullHeal {
		rows = appendRow(rows, "Effect", "Full HP restore")
	}
	if def.Rarity != "" {
		rows = appendRow(rows, "Rarity", titleCase(def.Rarity))
	}
	if def.Value > 0 {
		rows = appendRow(rows, "Value", fmt.Sprintf("%d gold", def.Value))
	}

	return contentCard{
		kind:        cardItem,
		section:     section,
		key:         key,
		name:        def.Name,
		subtitle:    subtitle,
		description: def.Description,
		flavor:      def.Flavor,
		tooltipRows: rows,
	}
}

// itemSubtitle picks the most informative one-line summary per item type.
func itemSubtitle(def *config.ItemDefinitionConfig) string {
	switch strings.ToLower(def.Type) {
	case "armor":
		s := fmt.Sprintf("AC %d", def.ArmorClassBase)
		if def.ArmorType != "" {
			s += "  " + titleCase(def.ArmorType)
		}
		return s
	case "consumable":
		switch {
		case def.HealBase > 0:
			return fmt.Sprintf("Heal %d+End/%d", def.HealBase, def.HealEnduranceDivisor)
		case def.Revive:
			return "Revive"
		case def.SummonDistanceTiles > 0:
			return fmt.Sprintf("Summons ~%d tiles", def.SummonDistanceTiles)
		}
	case "accessory":
		parts := []string{}
		if def.BonusMight > 0 {
			parts = append(parts, fmt.Sprintf("+%d Might", def.BonusMight))
		}
		if def.BonusIntellect > 0 {
			parts = append(parts, fmt.Sprintf("+%d Int", def.BonusIntellect))
		}
		if def.BonusLuck > 0 {
			parts = append(parts, fmt.Sprintf("+%d Luck", def.BonusLuck))
		}
		if def.IntellectScalingDivisor > 0 {
			parts = append(parts, fmt.Sprintf("SpellPwr +Int/%d", def.IntellectScalingDivisor))
		}
		if def.PersonalityScalingDivisor > 0 {
			parts = append(parts, fmt.Sprintf("SP +Per/%d", def.PersonalityScalingDivisor))
		}
		return strings.Join(parts, "  ")
	case "quest":
		return "Quest item"
	}
	return ""
}

func spellCard(section, key string, def *config.SpellDefinitionConfig) contentCard {
	// Base damage is derived from cost (cost × SpellDamagePerSP), not from a
	// YAML field — so display the formula's output here, not whatever the
	// designer wrote in a stale `damage:` line.
	baseDamage := 0
	if def.IsProjectile {
		baseDamage = def.SpellPointsCost * spells.SpellDamagePerSP
	}

	subtitle := fmt.Sprintf("SP %d  Lvl %d", def.SpellPointsCost, def.Level)
	switch {
	case baseDamage > 0:
		subtitle += fmt.Sprintf("  Dmg %d", baseDamage)
		if def.AoeRadiusTiles > 0 {
			subtitle += fmt.Sprintf("  AoE %.0ft", def.AoeRadiusTiles)
		}
	case def.HealAmount > 0:
		subtitle += fmt.Sprintf("  Heal %d", def.HealAmount)
	case def.Duration > 0:
		subtitle += fmt.Sprintf("  %ds", def.Duration)
	}

	var rows []string
	rows = appendRow(rows, "School", titleCase(def.School))
	rows = appendRow(rows, "Level", fmt.Sprintf("%d", def.Level))
	rows = appendRow(rows, "Spell points", fmt.Sprintf("%d", def.SpellPointsCost))
	if baseDamage > 0 {
		rows = appendRow(rows, "Base damage", fmt.Sprintf("%d  (cost × %d)", baseDamage, spells.SpellDamagePerSP))
	}
	if def.AoeRadiusTiles > 0 {
		rows = appendRow(rows, "AoE radius", fmt.Sprintf("%.1f tiles", def.AoeRadiusTiles))
	}
	if def.HealAmount > 0 {
		rows = appendRow(rows, "Base healing", fmt.Sprintf("%d", def.HealAmount))
	}
	if def.Duration > 0 {
		rows = appendRow(rows, "Duration", fmt.Sprintf("%d seconds", def.Duration))
	}
	if def.StatBonus > 0 {
		rows = appendRow(rows, "Stat bonus", fmt.Sprintf("+%d to all stats", def.StatBonus))
	}
	if def.DisintegrateChance > 0 {
		rows = appendRow(rows, "Disintegrate chance", fmt.Sprintf("%.0f%%", def.DisintegrateChance*100))
	}
	if def.IsProjectile {
		rows = appendRow(rows, "Type", "Projectile (offensive)")
	} else if def.IsUtility {
		rows = appendRow(rows, "Type", "Utility")
	}
	if def.TargetSelf {
		rows = appendRow(rows, "Target", "Self only")
	}
	if def.WaterWalk {
		rows = appendRow(rows, "Effect", "Walk on water")
	}
	if def.WaterBreathing {
		rows = appendRow(rows, "Effect", "Underwater breathing")
	}
	if def.Awaken {
		rows = appendRow(rows, "Effect", "Awaken party")
	}

	return contentCard{
		kind:        cardSpell,
		section:     section,
		key:         key,
		name:        def.Name,
		subtitle:    subtitle,
		description: def.Description,
		tooltipRows: rows,
	}
}

func appendRow(rows []string, label, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return rows
	}
	return append(rows, label+": "+value)
}

func titleCase(s string) string {
	if s == "" {
		return ""
	}
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) == 0 {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// iconForCard loads the per-card sprite by naming convention
// (assets/sprites/interface/icon_<kind>_<key>.png). Returns nil if no file.
func (v *viewer) iconForCard(c *contentCard) *ebiten.Image {
	var prefix string
	switch c.kind {
	case cardWeapon:
		prefix = "weapon"
	case cardItem:
		prefix = "item"
	case cardSpell:
		prefix = "spell"
	}
	cacheKey := prefix + ":" + c.key
	if img, ok := v.iconCache[cacheKey]; ok {
		return img // may be nil — "we already checked, no file"
	}
	path := filepath.Join("assets", "sprites", "interface", "icon_"+prefix+"_"+c.key+".png")
	img, _, err := ebitenutil.NewImageFromFile(path)
	if err != nil {
		v.iconCache[cacheKey] = nil
		return nil
	}
	v.iconCache[cacheKey] = img
	return img
}
