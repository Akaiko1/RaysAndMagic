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
	"ugataima/internal/game"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/stats"

	"github.com/hajimehoshi/ebiten/v2"
)

// contentCard is a single entry on the Items & Spells page.
type contentCard struct {
	kind     contentKind
	section  string // section header this card belongs under
	key      string // YAML key (used to resolve icon filename)
	name     string
	subtitle string // short stat line shown on the card
	rarity   string // item/weapon rarity - tints the name on the card and in the tooltip

	// Tooltip-only fields (full data).
	description string
	tooltipRows []string // complete shared tooltip, including name and category

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

// groupCardsBySection collapses each section into ONE contiguous run, keeping
// both the sections' and the cards' first-appearance order. The page draws a
// header whenever the section changes, so a section that reappears later in the
// source order (a skill appended late in the save-pinned SkillType enum lands
// after the Misc block) would otherwise print its header twice.
func groupCardsBySection(cards []contentCard) []contentCard {
	order := make([]string, 0, len(cards))
	bySection := make(map[string][]contentCard, len(cards))
	for _, card := range cards {
		if _, seen := bySection[card.section]; !seen {
			order = append(order, card.section)
		}
		bySection[card.section] = append(bySection[card.section], card)
	}
	out := make([]contentCard, 0, len(cards))
	for _, section := range order {
		out = append(out, bySection[section]...)
	}
	return out
}

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
			if a.SpellPointsCost != b.SpellPointsCost {
				return a.SpellPointsCost < b.SpellPointsCost
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

// buildTrapCards lists traps using the same base tooltips as the shop.
func buildTrapCards() []contentCard {
	var cards []contentCard
	for _, key := range config.TrapKeysOrdered() {
		def, ok := config.GetTrapDefinition(key)
		if !ok {
			continue
		}
		cards = append(cards, trapCard("Traps (Thief)", key, def))
	}
	return cards
}

func trapCard(section, key string, def *config.TrapDefinitionConfig) contentCard {
	it, ok := config.TrapItem(key)
	if !ok {
		panic("unknown catalog trap: " + key)
	}
	rows := strings.Split(game.GetItemTooltip(it, nil, nil, true), "\n")
	return contentCard{
		kind:        cardSpell,
		section:     section,
		key:         key,
		name:        def.Name,
		subtitle:    fmt.Sprintf("Lv %d  SP %d", def.Level, def.SPCost),
		tooltipRows: rows,
		icon:        def.Icon,
	}
}

func weaponCard(section, key string, def *config.WeaponDefinitionConfig) contentCard {
	formula := character.WeaponDamageFormula(def)
	subtitle := fmt.Sprintf("Base dmg %d  Range %d", formula.Base, def.Range)
	if character.WeaponStrikeCount(def) > 1 {
		subtitle = fmt.Sprintf("Pre-split dmg %d  Range %d", formula.Base, def.Range)
	}
	if def.AoeRadiusTiles > 0 {
		subtitle += fmt.Sprintf("  AoE %.0ft", def.AoeRadiusTiles)
	}
	for _, term := range formula.Terms {
		subtitle += fmt.Sprintf("  +%s/%d", term.Stat, term.Divisor)
	}
	if def.TrueDamage > 0 {
		subtitle += fmt.Sprintf("  +%d True", def.TrueDamage)
	}
	rows := strings.Split(game.GetItemTooltip(items.CreateWeaponFromYAML(key), nil, nil, true), "\n")
	return contentCard{
		kind:        cardWeapon,
		section:     section,
		key:         key,
		name:        def.Name,
		subtitle:    subtitle,
		rarity:      def.Rarity,
		tooltipRows: rows,
	}
}

func itemCard(section, key string, def *config.ItemDefinitionConfig) contentCard {
	subtitle := itemSubtitle(def)
	it := items.CreateItemFromYAML(key)
	kind := it.DisplayKind()
	if strings.ToLower(def.Type) == "accessory" {
		// Surface the slot on the card itself, not only in the tooltip.
		subtitle = strings.TrimSpace(kind + "  " + subtitle)
	}

	rows := strings.Split(game.GetItemTooltip(it, nil, nil, true), "\n")
	return contentCard{
		kind:        cardItem,
		section:     section,
		key:         key,
		name:        def.Name,
		subtitle:    subtitle,
		rarity:      def.Rarity,
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
		return strings.Join(def.CoreEffectLines(), "  ")
	case "accessory":
		return strings.Join(def.CoreEffectLines(), "  ")
	case "quest":
		return "Quest item"
	}
	return ""
}

func spellCard(section, key string, def *config.SpellDefinitionConfig) contentCard {
	sd, sdErr := spells.GetSpellDefinitionByID(spells.SpellID(key))

	baseDamage := 0
	kind := spells.DamageNone
	if sdErr == nil {
		kind = sd.DamageFormula().Kind
		baseDamage = character.SpellDamageBreakdown(sd, nil).Total
	}

	subtitle := fmt.Sprintf("SP %d", def.SpellPointsCost)
	switch {
	case kind == spells.DamageZone:
		subtitle += fmt.Sprintf("  Base tick %d", baseDamage)
	case baseDamage > 0:
		subtitle += fmt.Sprintf("  Base dmg %d", baseDamage)
		if def.AoeRadiusTiles > 0 {
			subtitle += fmt.Sprintf("  AoE %.0ft", def.AoeRadiusTiles)
		}
	case def.HealAmount > 0:
		subtitle += fmt.Sprintf("  Base heal %d", def.HealAmount)
	case def.Duration > 0:
		subtitle += fmt.Sprintf("  Base duration %ds", def.Duration)
	}

	if def.MonsterOnly {
		subtitle = "Monster only - " + titleCase(def.School)
	}
	rows := strings.Split(game.GetSpellTooltip(spells.SpellID(key), nil, nil, true), "\n")
	return contentCard{
		kind:        cardSpell,
		section:     section,
		key:         key,
		name:        def.Name,
		subtitle:    subtitle,
		tooltipRows: rows,
	}
}

// --- Characters page ---------------------------------------------------------
//
// The class list, skill list, class blurbs and skill descriptions are NOT
// duplicated here - they come from the shared `character` package
// (PlayableClasses, AllSkills, CharacterClass.Blurb, SkillType.Description/
// Category), so adding a class or skill to the game updates the editor too.

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
	// Each school gets its own mastery policy. A generic card used to mix
	// elemental and self-magic rules into one misleading description.
	for _, school := range character.AllMagicSchools {
		cards = append(cards, contentCard{
			kind:        cardSkill,
			section:     "Magic",
			key:         school.String() + "_magic",
			name:        school.DisplayName() + " Magic",
			description: character.MagicMasteryDescription(school),
		})
	}
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

func titleCase(s string) string { return config.TitleWords(s) }

// tileSpriteThumbnail loads a tile's sprite image (for legend previews),
// searching the same sprite dirs the game does. Returns nil for tiles with no
// sprite (floors) or no file on disk. Cached alongside card icons.
func (v *viewer) tileSpriteThumbnail(sprite string) *ebiten.Image {
	if sprite == "" {
		return nil
	}
	img, _ := v.iconImages.Get(sprite)
	return img
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
	img, _ := v.iconImages.Get(fileBase)
	return img
}
