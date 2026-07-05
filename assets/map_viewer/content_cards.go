package main

// Content card model + builders for the Items & Spells tab. Each YAML config
// (weapons, items, spells) gets a per-kind builder that produces a single
// `contentCard` carrying both the short summary line and the full tooltip
// rows. The page renderer (content_page.go) doesn't need to know what kind
// of thing a card represents - it just lays them out by section.

import (
	"fmt"
	"sort"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/stats"

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

	// icon overrides the icon_<kind>_<key>.png naming convention (traps ship
	// their sprite name in traps.yaml).
	icon string
}

type contentKind int

const (
	cardWeapon contentKind = iota
	cardItem
	cardSpell
	cardSkill
)

// buildItemsCards assembles the Items page: weapons (by category) followed by
// items (armor/accessory/consumable/quest). Runs once at startup.
func buildItemsCards() []contentCard {
	var cards []contentCard
	cards = append(cards, buildWeaponCards()...)
	cards = append(cards, buildItemCards()...)
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
		section := "Weapons - " + titleCase(strings.ReplaceAll(cat, "_", " "))
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

// buildSpellCards groups spells BY SCHOOL (canonical order from the character
// package), and within each school sorts by spell level then name. Battle and
// utility spells are mixed together - the school is the only grouping.
func buildSpellCards() []contentCard {
	if config.GlobalSpells == nil {
		return nil
	}
	bySchool := map[string][]string{}
	for key, def := range config.GlobalSpells.Spells {
		if def == nil {
			continue
		}
		school := strings.ToLower(strings.TrimSpace(def.School))
		if school == "" {
			school = "spell"
		}
		bySchool[school] = append(bySchool[school], key)
	}

	var cards []contentCard
	seen := map[string]bool{}
	emitSchool := func(school string) {
		keys := bySchool[school]
		if len(keys) == 0 {
			return
		}
		sort.SliceStable(keys, func(i, j int) bool {
			a, b := config.GlobalSpells.Spells[keys[i]], config.GlobalSpells.Spells[keys[j]]
			if a.Level != b.Level {
				return a.Level < b.Level
			}
			return a.Name < b.Name
		})
		section := titleCase(school)
		for _, key := range keys {
			cards = append(cards, spellCard(section, key, config.GlobalSpells.Spells[key]))
		}
		seen[school] = true
	}
	// Canonical school order first, then any leftover schools alphabetically.
	for _, s := range character.AllMagicSchools {
		emitSchool(string(s))
	}
	rest := make([]string, 0)
	for s := range bySchool {
		if !seen[s] {
			rest = append(rest, s)
		}
	}
	sort.Strings(rest)
	for _, s := range rest {
		emitSchool(s)
	}
	cards = append(cards, buildTrapCards()...)
	return cards
}

// buildTrapCards lists the thief traps under their own section. Mechanics rows
// come from config.TrapDefinitionConfig.EffectLines - the SAME source as the
// in-game trap-book tooltip, so the editor can't drift.
func buildTrapCards() []contentCard {
	var cards []contentCard
	for _, key := range config.TrapKeysOrdered() {
		def, ok := config.GetTrapDefinition(key)
		if !ok {
			continue
		}
		rows := []string{"Level: " + fmt.Sprintf("%d", def.Level)}
		rows = append(rows, character.RenderCardLines(character.TrapCardSections(def, config.TrapPlaceRangeTiles, config.MaxTrapsPerOwner), true)...)
		cards = append(cards, contentCard{
			kind:        cardSpell,
			section:     "Traps (Thief)",
			key:         key,
			name:        def.Name,
			subtitle:    fmt.Sprintf("Lv %d  SP %d", def.Level, def.SPCost),
			description: def.Description,
			tooltipRows: rows,
			icon:        def.Icon,
		})
	}
	return cards
}

func weaponCard(section, key string, def *config.WeaponDefinitionConfig) contentCard {
	subtitle := fmt.Sprintf("Dmg %d  Range %d", def.Damage, def.Range)
	if def.AoeRadiusTiles > 0 {
		subtitle += fmt.Sprintf("  AoE %.0ft", def.AoeRadiusTiles)
	}
	if def.BonusStat != "" {
		subtitle += "  +" + def.BonusStat
	}
	// Unified template (shared engine in character/cardtemplate.go): the
	// editor shows the character-independent variant - formulas in place of
	// personal numbers - in the same section order as the in-game tooltip.
	rows := character.RenderCardLines(character.WeaponCardSections(def), true)
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
		flavor:      def.Flavor,
		tooltipRows: rows,
	}
}

// wearableKindLabel names a wearable by its SLOT (Belt / Amulet / Cloak / ...),
// matching the in-game tooltip's itemKindLabel - "Accessory" says nothing
// about where the piece goes.
func wearableKindLabel(def *config.ItemDefinitionConfig) string {
	t := strings.ToLower(strings.TrimSpace(def.Type))
	if t == "armor" || t == "accessory" {
		if slot, ok := items.EquipSlotFromName(def.EquipSlot); ok {
			return slot.DisplayName()
		}
		if t == "accessory" {
			return items.SlotRing1.DisplayName()
		}
	}
	return titleCase(def.Type)
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func itemCard(section, key string, def *config.ItemDefinitionConfig) contentCard {
	subtitle := itemSubtitle(def)
	kind := wearableKindLabel(def)
	if strings.ToLower(def.Type) == "accessory" {
		// Surface the slot on the card itself, not only in the tooltip.
		subtitle = strings.TrimSpace(kind + "  " + subtitle)
	}

	rows := []string{"Type: " + kind}
	// Unified template (character/cardtemplate.go) - same sections as the
	// in-game tooltip, character-independent variant.
	rows = append(rows, character.RenderCardLines(character.ItemCardSections(def), true)...)
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
		flatBonuses := []struct {
			label string
			val   int
		}{
			{"Might", def.BonusMight},
			{"Int", def.BonusIntellect},
			{"Per", def.BonusPersonality},
			{"End", def.BonusEndurance},
			{"Acc", def.BonusAccuracy},
			{"Spd", def.BonusSpeed},
			{"Luck", def.BonusLuck},
		}
		for _, b := range flatBonuses {
			if b.val > 0 {
				parts = append(parts, fmt.Sprintf("+%d %s", b.val, b.label))
			}
		}
		if def.IntellectScalingDivisor > 0 {
			parts = append(parts, fmt.Sprintf("+Int/%d Intellect", def.IntellectScalingDivisor))
		}
		if def.PersonalityScalingDivisor > 0 {
			parts = append(parts, fmt.Sprintf("+Per/%d Personality", def.PersonalityScalingDivisor))
		}
		for _, school := range sortedKeys(def.Resistances) {
			parts = append(parts, fmt.Sprintf("%d%% %s resist", def.Resistances[school], titleCase(school)))
		}
		return strings.Join(parts, "  ")
	case "quest":
		return "Quest item"
	}
	return ""
}

func spellCard(section, key string, def *config.SpellDefinitionConfig) contentCard {
	sd, sdErr := spells.GetSpellDefinitionByID(spells.SpellID(key))

	// Monster-only spells are cast with the monster's own attack damage (no SP /
	// Intellect / mastery / crit), so the player-formula card would lie - render
	// the dedicated monster card instead.
	if def.MonsterOnly {
		subtitle := fmt.Sprintf("MONSTER ONLY  %s  Lvl %d", titleCase(def.School), def.Level)
		if def.AoeRadiusTiles > 0 {
			subtitle += fmt.Sprintf("  AoE %.0ft", def.AoeRadiusTiles)
		}
		var rows []string
		rows = appendRow(rows, "School", titleCase(def.School))
		if sdErr == nil {
			rows = append(rows, character.RenderCardLines(character.MonsterSpellCardSections(def, sd), true)...)
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

	// Base damage comes from the SAME formula combat uses (cost x
	// SpellDamagePerSP x damage_cost_multiplier) - intellect 0 isolates the
	// character-independent base. A hand-rolled costxN here ignored the
	// multiplier (Ray of Light showed half its real base).
	baseDamage := 0
	if def.IsProjectile && !def.DealsNoDamage { // no-damage projectiles (Charm/Disintegrate) deal nothing
		baseDamage, _, _ = spells.CalculateSpellDamageByID(spells.SpellID(key), 0)
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

	// Unified template (character/cardtemplate.go) - same sections as the
	// in-game tooltip, character-independent variant (formulas, not numbers).
	var rows []string
	rows = appendRow(rows, "School", titleCase(def.School))
	if sdErr == nil {
		rows = append(rows, character.RenderCardLines(character.SpellCardSections(key, def, sd), true)...)
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

// --- Characters page ---------------------------------------------------------
//
// The class list, skill list, class blurbs and skill descriptions are NOT
// duplicated here - they come from the shared `character` package
// (PlayableClasses, AllSkills, CharacterClass.Blurb, SkillType.Description/
// Category), so adding a class or skill to the game updates the editor too.

// equipSlotOrder is the order starting equipment is listed on a character card.
var equipSlotOrder = []struct {
	slot  items.EquipSlot
	label string
}{
	{items.SlotMainHand, "Weapon"},
	{items.SlotSpell, "Spell"},
	{items.SlotOffHand, "Off-hand"},
	{items.SlotArmor, "Armor"},
	{items.SlotHelmet, "Helmet"},
	{items.SlotBoots, "Boots"},
	{items.SlotGauntlets, "Gauntlets"},
	{items.SlotBelt, "Belt"},
	{items.SlotCloak, "Cloak"},
	{items.SlotAmulet, "Amulet"},
	{items.SlotRing1, "Ring"},
	{items.SlotRing2, "Ring"},
}

func spellDisplayName(key string) string {
	if config.GlobalSpells != nil {
		if def, ok := config.GlobalSpells.Spells[key]; ok && def != nil && def.Name != "" {
			return def.Name
		}
	}
	return key
}

// --- Skills page -------------------------------------------------------------

// buildSkillCards renders every skill straight from the shared character
// catalog (character.AllSkills + SkillType.Category/Description), so the editor
// never maintains its own copy of the list or the descriptions.
func buildSkillCards() []contentCard {
	var cards []contentCard
	for _, st := range character.AllSkills {
		cards = append(cards, contentCard{
			kind:        cardSkill,
			section:     st.Category() + " Skills",
			key:         strings.ToLower(strings.ReplaceAll(st.String(), " ", "_")),
			name:        st.String(),
			description: st.Description(),
		})
	}
	// Magic mastery + primary stats come from the same catalog texts the
	// in-game tooltips quote (character.MagicMasteryDescription/StatDescription).
	cards = append(cards, contentCard{
		kind:        cardSkill,
		section:     "Magic",
		key:         "magic_mastery",
		name:        "Magic Mastery",
		description: character.MagicMasteryDescription(),
	})
	for _, statName := range stats.Names {
		cards = append(cards, contentCard{
			kind:        cardSkill,
			section:     "Stats",
			key:         statName,
			name:        titleCase(statName),
			description: character.StatDescription(statName),
		})
	}
	return cards
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

// tileSpriteThumbnail loads a tile's sprite image (for legend previews),
// searching the same sprite dirs the game does. Returns nil for tiles with no
// sprite (floors) or no file on disk. Cached alongside card icons.
func (v *viewer) tileSpriteThumbnail(sprite string) *ebiten.Image {
	if sprite == "" {
		return nil
	}
	cacheKey := "tilesprite:" + sprite
	if img, ok := v.iconCache[cacheKey]; ok {
		return img // may be nil - already checked, no file
	}
	if path, ok := graphics.ResolveSpritePath(sprite); ok {
		if img, _, err := ebitenutil.NewImageFromFile(path); err == nil {
			v.iconCache[cacheKey] = img
			return img
		}
	}
	v.iconCache[cacheKey] = nil
	return nil
}

// iconForCard loads the per-card sprite by naming convention
// (icon_<kind>_<key>), resolved anywhere under assets/sprites via the shared
// index. Returns nil if no file.
func (v *viewer) iconForCard(c *contentCard) *ebiten.Image {
	// Skills have no art (the card renderer draws a placeholder box).
	if c.kind == cardSkill {
		return nil
	}

	var prefix string
	switch c.kind {
	case cardWeapon:
		prefix = "weapon"
	case cardItem:
		prefix = "item"
	case cardSpell:
		prefix = "spell"
	}
	fileBase := "icon_" + prefix + "_" + c.key
	if c.icon != "" {
		fileBase = c.icon // explicit sprite name (traps)
	}
	cacheKey := prefix + ":" + c.key
	if img, ok := v.iconCache[cacheKey]; ok {
		return img // may be nil - "we already checked, no file"
	}
	path, ok := graphics.ResolveSpritePath(fileBase)
	if !ok {
		v.iconCache[cacheKey] = nil
		return nil
	}
	img, _, err := ebitenutil.NewImageFromFile(path)
	if err != nil {
		v.iconCache[cacheKey] = nil
		return nil
	}
	v.iconCache[cacheKey] = img
	return img
}
