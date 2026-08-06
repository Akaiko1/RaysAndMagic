package game

import (
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// These cover the second SSoT audit round: arc/mass-hit, RT-vs-TB cooldown,
// disintegrate immunity, monster-only cards, dual stat scaling, splash-crit and
// dodge rules, projectile hitbox, the active-buff bump and the Meditation
// discount - every line that combat (or a YAML field) actually drives.

func TestTooltip_WeaponArcCooldownStun(t *testing.T) {
	g, thief := newThiefTestGame(t)
	mace, err := items.TryCreateWeaponFromYAML("steel_mace")
	if err != nil {
		t.Fatalf("steel_mace: %v", err)
	}
	full := GetItemTooltip(mace, thief, g.combat, true)

	// Swing arc is a core melee differentiator -> visible without Shift. The steel
	// mace is arc type 2 (front + one flank).
	compact := GetItemTooltip(mace, thief, g.combat, false)
	if !strings.Contains(compact, "front and one flank") {
		t.Errorf("mace must show its swing arc shape:\n%s", compact)
	}
	// Cooldown distinguishes real-time seconds from the turn-based action.
	for _, want := range []string{"RT Cooldown:", "TB: 1 action"} {
		if !strings.Contains(compact, want) {
			t.Errorf("cooldown must label RT/TB (%q):\n%s", want, compact)
		}
	}
	// Stun spells out RT seconds AND TB turns (was an ambiguous "(3 turns)").
	if !strings.Contains(full, "(3s RT / 3 turns TB)") {
		t.Errorf("mace stun must read RT seconds / TB turns:\n%s", full)
	}
}

func TestTooltip_DisintegrateImmunity(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]

	def, err := spells.GetSpellDefinitionByID("disintegrate")
	if err != nil {
		t.Fatalf("disintegrate: %v", err)
	}
	spellTip := buildSpellTooltipUnified(def, char, cs, true)
	if !strings.Contains(spellTip, "undead and dragons immune") {
		t.Errorf("disintegrate spell must note universal immunity:\n%s", spellTip)
	}

	blaster, err := items.TryCreateWeaponFromYAML("alien_blaster")
	if err != nil {
		t.Skip("alien_blaster not defined")
	}
	weaponTip := GetItemTooltip(blaster, char, cs, true)
	if !strings.Contains(weaponTip, "undead and dragons immune") {
		t.Errorf("alien blaster disintegrate must note universal immunity:\n%s", weaponTip)
	}
}

func TestEditorCard_MonsterOnlySpellHidesPlayerFormula(t *testing.T) {
	newTestCombatSystemWithConfig(t) // loads spell config
	def, ok := config.GetSpellDefinition("alien_dark_bolt")
	if !ok || def == nil {
		t.Skip("alien_dark_bolt not defined")
	}
	if !def.MonsterOnly {
		t.Fatalf("alien_dark_bolt should be monster_only")
	}
	sd, err := spells.GetSpellDefinitionByID("alien_dark_bolt")
	if err != nil {
		t.Fatalf("alien_dark_bolt sd: %v", err)
	}
	joined := strings.Join(character.RenderCardLines(character.MonsterSpellCardSections(def, sd), true), "\n")

	if !strings.Contains(joined, "Cast by monsters only") ||
		!strings.Contains(joined, "casting monster's attack damage") {
		t.Errorf("monster card must describe monster mechanics:\n%s", joined)
	}
	// It must NOT borrow the player formula (SP cost, stat scaling, mastery).
	for _, leak := range []string{"Cost:", "Intellect /", "Mastery:", "Base ("} {
		if strings.Contains(joined, leak) {
			t.Errorf("monster card leaks player formula %q:\n%s", leak, joined)
		}
	}
}

func TestEditorCard_RayOfLightDualScaling(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	def, ok := config.GetSpellDefinition("ray_of_light")
	if !ok || def == nil {
		t.Skip("ray_of_light not defined")
	}
	sd, err := spells.GetSpellDefinitionByID("ray_of_light")
	if err != nil {
		t.Fatalf("ray_of_light sd: %v", err)
	}
	joined := strings.Join(character.RenderCardLines(character.SpellCardSections("ray_of_light", def, sd), true), "\n")
	// Ray of Light scales with BOTH stats (school Intellect + the personality flag).
	if !strings.Contains(joined, "Intellect / 3") || !strings.Contains(joined, "Personality / 3") {
		t.Errorf("Ray of Light editor card must scale with BOTH Intellect and Personality:\n%s", joined)
	}
}

func TestEditorCard_BuffOmitsInactiveRTCooldown(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	for _, tc := range []struct {
		key          string
		wantCooldown bool
	}{
		{key: "bless", wantCooldown: false},
		{key: "fireball", wantCooldown: true},
	} {
		t.Run(tc.key, func(t *testing.T) {
			def, ok := config.GetSpellDefinition(tc.key)
			if !ok || def == nil {
				t.Fatalf("%s definition missing", tc.key)
			}
			sd, err := spells.GetSpellDefinitionByID(spells.SpellID(tc.key))
			if err != nil {
				t.Fatalf("%s spell definition: %v", tc.key, err)
			}
			card := strings.Join(character.RenderCardLines(character.SpellCardSections(tc.key, def, sd), true), "\n")
			gotCooldown := strings.Contains(card, "Cooldown")
			if gotCooldown != tc.wantCooldown {
				t.Errorf("editor cooldown shown = %v, want %v:\n%s", gotCooldown, tc.wantCooldown, card)
			}
		})
	}
}

func TestTooltip_AoESplashCritAndDodgeRules(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]
	def, err := spells.GetSpellDefinitionByID("fireball")
	if err != nil {
		t.Fatalf("fireball: %v", err)
	}
	full := buildSpellTooltipUnified(def, char, cs, true)
	if !strings.Contains(full, character.SplashCritRule) {
		t.Errorf("AoE spell must explain splash inherits the crit:\n%s", full)
	}
	if !strings.Contains(full, "Perfect Dodge") {
		t.Errorf("projectile spell must mention Perfect Dodge:\n%s", full)
	}
	if !strings.Contains(full, "Hitbox:") {
		t.Errorf("projectile spell should show its hitbox size:\n%s", full)
	}
}

func TestTooltip_MortarUsesBloomRules(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]
	def, err := spells.GetSpellDefinitionByID("stone_blossom")
	if err != nil {
		t.Fatalf("stone_blossom: %v", err)
	}
	full := buildSpellTooltipUnified(def, char, cs, true)

	for _, want := range []string{
		"One critical roll boosts the entire bloom",
		"The bloom cannot be evaded by Perfect Dodge",
	} {
		if !strings.Contains(full, want) {
			t.Errorf("mortar tooltip missing %q:\n%s", want, full)
		}
	}
	for _, stale := range []string{character.SplashCritRule, "Hitbox:"} {
		if strings.Contains(full, stale) {
			t.Errorf("mortar tooltip contains ordinary projectile rule %q:\n%s", stale, full)
		}
	}
}

func TestTooltip_ResistanceSummaryDoesNotDoubleCountPhysical(t *testing.T) {
	for _, tc := range []struct {
		name string
		def  config.ItemDefinitionConfig
		want string
	}{
		{
			name: "non-physical only",
			def: config.ItemDefinitionConfig{Resistances: map[string]int{
				"fire": 25, "water": 25, "air": 25, "earth": 25, "spirit": 25,
				"mind": 25, "body": 25, "light": 25, "dark": 25,
			}},
			want: "+25% resistance to every non-physical school",
		},
		{
			name: "different physical value",
			def: config.ItemDefinitionConfig{Resistances: map[string]int{
				"physical": 10, "fire": 25, "water": 25, "air": 25, "earth": 25,
				"spirit": 25, "mind": 25, "body": 25, "light": 25, "dark": 25,
			}},
			want: "+25% resistance to every non-physical school; +10% Physical resistance",
		},
		{
			name: "same every school",
			def: config.ItemDefinitionConfig{Resistances: map[string]int{
				"physical": 25, "fire": 25, "water": 25, "air": 25, "earth": 25,
				"spirit": 25, "mind": 25, "body": 25, "light": 25, "dark": 25,
			}},
			want: "+25% resistance to every damage school",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.def.ResistLines()
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("ResistLines() = %v, want [%q]", got, tc.want)
			}
		})
	}
}

func TestTooltip_ActiveBuffRaisesTotalDamage(t *testing.T) {
	g, thief := newThiefTestGame(t)
	w := thief.Equipment[items.SlotMainHand]
	before := GetItemTooltip(w, thief, g.combat, true)

	// Heroism (+10 outgoing) is the OutBonus combat adds to every hit.
	g.addCombatBuff(TimedCombatBuff{SpellID: "heroism", OutBonus: 10, Frames: 600})
	after := GetItemTooltip(w, thief, g.combat, true)

	if !strings.Contains(after, "Active party buff: +10") {
		t.Errorf("buffed weapon tooltip must surface the active buff:\n%s", after)
	}
	if before == after {
		t.Errorf("an active outgoing-damage buff must change Total Damage:\n%s", after)
	}
}

func TestTooltip_MeditationCostDiscount(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	caster := character.CreateCharacter("Med", character.ClassSorcerer, cs.game.config)
	caster.Skills[character.SkillMeditation] = &character.Skill{Mastery: character.MasteryGrandMaster}
	cs.game.party.Members[0] = caster

	def, err := spells.GetSpellDefinitionByID("fireball")
	if err != nil {
		t.Fatalf("fireball: %v", err)
	}
	full := buildSpellTooltipUnified(def, caster, cs, true)
	for _, want := range []string{"Base Cost:", "GM Meditation: -25%"} {
		if !strings.Contains(full, want) {
			t.Errorf("GM meditator spell must break down the cost (%q):\n%s", want, full)
		}
	}
}

func assertCleanTooltipText(t *testing.T, label, tooltip string) {
	t.Helper()
	if strings.TrimSpace(tooltip) == "" {
		t.Errorf("%s: empty tooltip", label)
		return
	}
	for _, r := range tooltip {
		if r > 127 {
			t.Errorf("%s: tooltip contains non-ASCII character %q (U+%04X):\n%s", label, r, r, tooltip)
			break
		}
	}
	lower := strings.ToLower(tooltip)
	for _, forbidden := range []string{
		"explicit special-spell",
		"other schools",
		"regular spell",
		"magic mastery never",
		"(level default)",
		"no sp, intellect, mastery",
		"elemental up to",
		"all incoming damage",
	} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("%s: tooltip contains internal or misleading text %q:\n%s", label, forbidden, tooltip)
		}
	}

	seen := make(map[string]bool)
	for _, line := range strings.Split(tooltip, "\n") {
		line = strings.ToLower(strings.TrimSpace(line))
		if line == "" || line == "[shift] full breakdown" {
			continue
		}
		if seen[line] {
			t.Errorf("%s: duplicate tooltip line %q:\n%s", label, line, tooltip)
			return
		}
		seen[line] = true
	}
}

// TestAllTooltipCatalogsAreClean walks every player-facing tooltip catalog.
// It catches stale implementation prose and exact duplicate lines regardless
// of whether the content came from Go or YAML.
func TestAllTooltipCatalogsAreClean(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]

	for _, class := range character.PlayableClasses {
		assertCleanTooltipText(t, "class/"+class.Key(), class.Blurb())
	}
	for _, stat := range []string{"might", "intellect", "personality", "endurance", "accuracy", "speed", "luck"} {
		assertCleanTooltipText(t, "stat/"+stat, character.StatDescription(stat))
	}
	for _, skill := range character.AllSkills {
		assertCleanTooltipText(t, "skill/"+skill.String(), skill.Description())
	}
	for _, school := range character.AllMagicSchools {
		tip := character.MagicMasteryDescription(school)
		assertCleanTooltipText(t, "school/"+school.String(), tip)
		if !strings.Contains(tip, school.DisplayName()) {
			t.Errorf("school/%s: tooltip does not identify its own school: %q", school, tip)
		}
	}

	for key, def := range config.GlobalSpells.Spells {
		if def == nil {
			t.Errorf("spell/%s: nil definition", key)
			continue
		}
		sd, err := spells.GetSpellDefinitionByID(spells.SpellID(key))
		if err != nil {
			t.Errorf("spell/%s: %v", key, err)
			continue
		}
		if def.MonsterOnly {
			tip := strings.Join(character.RenderCardLines(character.MonsterSpellCardSections(def, sd), true), "\n")
			assertCleanTooltipText(t, "spell/"+key, tip)
			continue
		}
		assertCleanTooltipText(t, "compact/spell/"+key, GetSpellTooltip(spells.SpellID(key), char, cs, false))
		assertCleanTooltipText(t, "spell/"+key, GetSpellTooltip(spells.SpellID(key), char, cs, true))
		editor := strings.Join(character.RenderCardLines(character.SpellCardSections(key, def, sd), true), "\n")
		assertCleanTooltipText(t, "editor/spell/"+key, editor)
	}

	for key, def := range config.GlobalWeapons.Weapons {
		if def == nil {
			t.Errorf("weapon/%s: nil definition", key)
			continue
		}
		item, err := items.TryCreateWeaponFromYAML(key)
		if err != nil {
			t.Errorf("weapon/%s: %v", key, err)
			continue
		}
		assertCleanTooltipText(t, "compact/weapon/"+key, GetItemTooltip(item, char, cs, false))
		assertCleanTooltipText(t, "weapon/"+key, GetItemTooltip(item, char, cs, true))
		editor := strings.Join(character.RenderCardLines(character.WeaponCardSections(def), true), "\n")
		assertCleanTooltipText(t, "editor/weapon/"+key, editor)
	}
	for key, def := range config.GlobalItems.Items {
		if def == nil {
			t.Errorf("item/%s: nil definition", key)
			continue
		}
		item, err := items.TryCreateItemFromYAML(key)
		if err != nil {
			t.Errorf("item/%s: %v", key, err)
			continue
		}
		assertCleanTooltipText(t, "compact/item/"+key, GetItemTooltip(item, char, cs, false))
		assertCleanTooltipText(t, "item/"+key, GetItemTooltip(item, char, cs, true))
		editor := strings.Join(character.RenderCardLines(character.ItemCardSections(def), true), "\n")
		// Pure collectibles have no mechanical sections; the editor's outer
		// item card still renders their authored name and description.
		if strings.TrimSpace(editor) != "" {
			assertCleanTooltipText(t, "editor/item/"+key, editor)
		}
	}
	for _, key := range config.TrapKeysOrdered() {
		def, ok := config.GetTrapDefinition(key)
		if !ok {
			t.Errorf("trap/%s: definition missing", key)
			continue
		}
		item, ok := config.TrapItem(key)
		if !ok {
			t.Errorf("trap/%s: cannot build item", key)
			continue
		}
		assertCleanTooltipText(t, "compact/trap/"+key, GetItemTooltip(item, char, cs, false))
		assertCleanTooltipText(t, "trap/"+key, GetItemTooltip(item, char, cs, true))
		editor := strings.Join(character.RenderCardLines(
			character.TrapCardSections(def, config.TrapPlaceRangeTiles, config.MaxTrapsPerOwner), true), "\n")
		assertCleanTooltipText(t, "editor/trap/"+key, editor)
	}
}
