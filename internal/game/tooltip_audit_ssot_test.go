package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// These cover the second SSoT audit round: swing arc, RT-vs-TB cooldown,
// disintegrate immunity, monster-only cards, dual stat scaling, splash-crit and
// dodge rules, projectile hitbox and the Meditation discount - every line that
// combat (or a YAML field) actually drives.

// Weapon stun and disintegrate rows are covered for every weapon by
// TestWeaponTooltipSurfacesEveryAuthoredEffect.
func TestTooltip_WeaponArcAndCooldownLabels(t *testing.T) {
	g, thief := newThiefTestGame(t)
	mace, err := items.TryCreateWeaponFromYAML("steel_mace")
	if err != nil {
		t.Fatalf("steel_mace: %v", err)
	}
	def := lookupWeaponConfigByName(mace.Name)
	arc := character.MeleeSwingArcLine(def)
	if arc == "" {
		t.Fatal("fixture: steel_mace must be a melee weapon with a swing arc")
	}
	// Swing arc is a core melee differentiator -> visible without Shift.
	compact := GetItemTooltipRows(mace, thief, g.combat, false).String()
	if !strings.Contains(compact, arc) {
		t.Errorf("mace must show its swing arc %q:\n%s", arc, compact)
	}
	// Cooldown distinguishes real-time seconds from the turn-based action.
	for _, want := range []string{"RT Cooldown:", "TB: 1 action"} {
		if !strings.Contains(compact, want) {
			t.Errorf("cooldown must label RT/TB (%q):\n%s", want, compact)
		}
	}
}

// Spell rules that combat drives must reach the full card through the game
// entry point.
func TestTooltip_SpellCardRules(t *testing.T) {
	for _, tc := range []struct {
		name   string
		spell  spells.SpellID
		setup  func(*character.MMCharacter)
		want   []string
		absent []string
	}{
		{name: "disintegrate notes universal immunity", spell: "disintegrate", want: []string{"undead and dragons immune"}},
		{
			name: "AoE projectile splash inherits crit and meets Perfect Dodge", spell: "fireball",
			want:   []string{character.SplashCritRule(), "Perfect Dodge"},
			absent: []string{"Hitbox:"},
		},
		{
			name: "mortar uses bloom rules", spell: "stone_blossom",
			want:   []string{"A critical hit boosts the whole explosion", "The bloom cannot be evaded by Perfect Dodge"},
			absent: []string{character.SplashCritRule(), "Hitbox:"},
		},
		{
			name: "GM Meditation breaks down the cost", spell: "fireball",
			setup: func(c *character.MMCharacter) {
				c.Skills[character.SkillMeditation] = &character.Skill{Mastery: character.MasteryGrandMaster}
			},
			want: []string{"Base Cost:", fmt.Sprintf("Meditation - Grandmaster: -%d%%", MeditationGMSpellCostReductionPct)},
		},
		{name: "no Meditation keeps one cost row", spell: "fireball", absent: []string{"Base Cost:", "Meditation -"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			caster := character.CreateCharacter("Med", character.ClassSorcerer, cs.game.config)
			delete(caster.Skills, character.SkillMeditation)
			if tc.setup != nil {
				tc.setup(caster)
			}
			cs.game.party.Members[0] = caster
			full := GetSpellTooltipRows(tc.spell, caster, cs, true).String()
			for _, want := range tc.want {
				if !strings.Contains(full, want) {
					t.Errorf("missing %q:\n%s", want, full)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(full, absent) {
					t.Errorf("unexpected %q:\n%s", absent, full)
				}
			}
		})
	}
}

func TestEditorCard_RayOfLightDualScaling(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	def, ok := config.GetSpellDefinition("ray_of_light")
	if !ok || def == nil {
		t.Fatal("ray_of_light not defined")
	}
	_, err := spells.GetSpellDefinitionByID("ray_of_light")
	if err != nil {
		t.Fatalf("ray_of_light sd: %v", err)
	}
	joined := GetSpellTooltipRows(spells.SpellID("ray_of_light"), nil, nil, true).String()
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
			_, err := spells.GetSpellDefinitionByID(spells.SpellID(tc.key))
			if err != nil {
				t.Fatalf("%s spell definition: %v", tc.key, err)
			}
			card := GetSpellTooltipRows(spells.SpellID(tc.key), nil, nil, true).String()
			gotCooldown := strings.Contains(card, "Base cooldown:")
			if gotCooldown != tc.wantCooldown {
				t.Errorf("editor cooldown shown = %v, want %v:\n%s", gotCooldown, tc.wantCooldown, card)
			}
		})
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
		for i, p := range cs.game.config.Characters.Classes[class.Key()].Description {
			assertCleanTooltipText(t, fmt.Sprintf("class/%s/%d", class.Key(), i), config.PlainKeywordText(p.Text))
		}
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
		assertCleanTooltipText(t, "compact/spell/"+key, GetSpellTooltipRows(spells.SpellID(key), char, cs, false).String())
		assertCleanTooltipText(t, "spell/"+key, GetSpellTooltipRows(spells.SpellID(key), char, cs, true).String())
		editor := GetSpellTooltipRows(spells.SpellID(key), nil, nil, true).String()
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
		assertCleanTooltipText(t, "compact/weapon/"+key, GetItemTooltipRows(item, char, cs, false).String())
		assertCleanTooltipText(t, "weapon/"+key, GetItemTooltipRows(item, char, cs, true).String())
		editor := GetItemTooltipRows(items.CreateWeaponFromYAML(items.GetWeaponKeyByName(def.Name)), nil, nil, true).String()
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
		assertCleanTooltipText(t, "compact/item/"+key, GetItemTooltipRows(item, char, cs, false).String())
		assertCleanTooltipText(t, "item/"+key, GetItemTooltipRows(item, char, cs, true).String())
		editor := GetItemTooltipRows(baseTestItem(t, def.Name), nil, nil, true).String()
		// Pure collectibles have no mechanical sections; the editor's outer
		// item card still renders their authored name and description.
		if strings.TrimSpace(editor) != "" {
			assertCleanTooltipText(t, "editor/item/"+key, editor)
		}
	}
	for _, key := range config.TrapKeysOrdered() {
		_, ok := config.GetTrapDefinition(key)
		if !ok {
			t.Errorf("trap/%s: definition missing", key)
			continue
		}
		item, ok := config.TrapItem(key)
		if !ok {
			t.Errorf("trap/%s: cannot build item", key)
			continue
		}
		assertCleanTooltipText(t, "compact/trap/"+key, GetItemTooltipRows(item, char, cs, false).String())
		assertCleanTooltipText(t, "trap/"+key, GetItemTooltipRows(item, char, cs, true).String())
		editor := GetItemTooltipRows(item, nil, nil, true).String()
		assertCleanTooltipText(t, "editor/trap/"+key, editor)
	}
}
