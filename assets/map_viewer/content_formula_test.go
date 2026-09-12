package main

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/config"
)

func TestEditorWeaponStrikeUnits(t *testing.T) {
	for _, tc := range []struct {
		name         string
		rangeTiles   int
		doubleStrike bool
		base, volley int
		wantSplit    bool
	}{
		{"single-melee", 3, false, 11, 0, false},
		{"double-even", 3, true, 10, 0, true},
		{"double-odd", 1, true, 11, 0, true},
		{"ranged-double-flag", 4, true, 11, 0, false},
		{"ranged-volley", 4, false, 10, 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := &config.WeaponDefinitionConfig{
				Name: "Fixture", Damage: tc.base, Range: tc.rangeTiles,
				DoubleStrike: tc.doubleStrike, Volley: tc.volley,
				BonusStat: "Might", BonusStatSecondary: "Speed",
			}
			card := weaponCard("test", "fixture", def)
			text := strings.Join(card.tooltipRows, "\n")
			prefix := fmt.Sprintf("Dmg %d  Range %d", tc.base, tc.rangeTiles)
			if tc.wantSplit {
				prefix = fmt.Sprintf("Pre-split dmg %d  Range %d", tc.base, tc.rangeTiles)
			}
			if !strings.HasPrefix(card.subtitle, prefix) {
				t.Errorf("subtitle = %q, want prefix %q", card.subtitle, prefix)
			}
			for _, line := range []string{
				"Strikes per attack: 2", "Normal damage formula before strike split:",
				"Per strike: divide Normal formula total by 2, round up",
			} {
				if strings.Contains(text, line) != tc.wantSplit {
					t.Errorf("split=%v, unexpected presence/absence of %q:\n%s", tc.wantSplit, line, text)
				}
			}
			if strings.Contains(text, "Damage shown per strike") {
				t.Errorf("editor source formula mislabeled as per-strike damage:\n%s", text)
			}
			for _, line := range []string{fmt.Sprintf("Base: %d", tc.base), "Might / 3: scales", "Speed / 4: scales"} {
				if !strings.Contains(text, line) {
					t.Errorf("source term lost or divided separately: missing %q", line)
				}
			}
			if tc.wantSplit && strings.Index(text, "Per strike: divide") < strings.Index(text, "Arms Master:") {
				t.Error("strike split must follow the complete Normal formula")
			}
			for _, r := range card.subtitle + text {
				if r > 127 {
					t.Fatalf("non-ASCII character %q in rendered card", r)
				}
			}
		})
	}
}

// These authored variants exercise the editor's public card path, including
// subtitles. Expected terms come from the mechanic, not another card builder.
func TestEditorSpellCardsUseCompleteFormula(t *testing.T) {
	for _, tc := range []struct {
		name   string
		edit   func(*config.SpellDefinitionConfig)
		want   []string
		absent []string
	}{
		{"projectile", func(d *config.SpellDefinitionConfig) {}, []string{"Dmg 42", "Intellect / 3: scales"}, nil},
		{"dual-stat", func(d *config.SpellDefinitionConfig) { d.ScalesWithPersonality = true }, []string{"Intellect / 3: scales", "Personality / 3: scales"}, nil},
		{"self-magic", func(d *config.SpellDefinitionConfig) { d.School = "body"; d.ScalesWithPersonality = true }, []string{"Personality / 3: scales"}, []string{"Intellect /"}},
		{"control", func(d *config.SpellDefinitionConfig) { d.DealsNoDamage = true }, []string{"SP 7"}, []string{"Dmg ", "Base (", "/ 3: scales"}},
		{"projectile-ladder", func(d *config.SpellDefinitionConfig) { d.DamageByMastery = []int{11, 23, 47, 95} }, []string{"Dmg 11", "Novice: 11", "23 / 47 / 95"}, []string{"Intellect /", "Base ("}},
		{"zone", func(d *config.SpellDefinitionConfig) {
			d.IsProjectile = false
			d.ZoneRadiusTiles = 2
			d.ZoneTickSeconds = 1
			d.ZoneTickDamage = 9
		}, []string{"Tick 9", "DAMAGE PER TICK", "Base: 9", "Intellect / 3: scales"}, []string{"Base ("}},
		{"nova-step", func(d *config.SpellDefinitionConfig) {
			d.IsProjectile = false
			d.PartyAoeRadiusTiles = 2
			d.MasteryDamagePerTier = 5
		}, []string{"Dmg 21", "Damage: 21-36", "Targets: Monsters and Party"}, []string{"Intellect /"}},
		{"map-ladder-spares-party", func(d *config.SpellDefinitionConfig) {
			d.IsProjectile = false
			d.MapWide = true
			d.SparesParty = true
			d.DamageByMastery = []int{11, 23, 47, 95}
		}, []string{"Dmg 11", "Novice: 11", "23 / 47 / 95", "Radius: Current map", "Targets: Monsters only"}, []string{"Intellect /", "Targets: Monsters and Party"}},
		{"heal", func(d *config.SpellDefinitionConfig) { d.IsProjectile = false; d.IsUtility = true; d.HealAmount = 19 }, []string{"Heal 19", "Base: 19", "Personality / 2: scales"}, []string{"Dmg ", "Intellect /"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def := &config.SpellDefinitionConfig{Name: "Fixture", School: "light", SpellPointsCost: 7, CooldownSeconds: 2, IsProjectile: true, DamageCostMultiplier: 2}
			tc.edit(def)
			previous := config.GlobalSpells
			config.GlobalSpells = &config.SpellSystemConfig{Spells: map[string]*config.SpellDefinitionConfig{"fixture": def}}
			t.Cleanup(func() { config.GlobalSpells = previous })
			card := spellCard("test", "fixture", def)
			text := card.subtitle + "\n" + strings.Join(card.tooltipRows, "\n")
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Errorf("missing %q:\n%s", want, text)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(text, absent) {
					t.Errorf("unexpected %q:\n%s", absent, text)
				}
			}
			if n := strings.Count(text, "Personality / 3: scales"); n > 1 {
				t.Errorf("Personality counted %d times", n)
			}
		})
	}
}

func TestEditorWeaponCardUsesFormulaTerms(t *testing.T) {
	for _, stat := range []string{"", "Might", "Intellect", "Personality", "Endurance", "Accuracy", "Speed", "Luck"} {
		t.Run("primary-"+stat, func(t *testing.T) {
			def := &config.WeaponDefinitionConfig{Name: "Fixture", Damage: 17, BonusStat: stat, BonusStatSecondary: "Personality"}
			card := weaponCard("test", "fixture", def)
			primary := stat
			if primary == "" {
				primary = "Might"
			}
			text := strings.Join(card.tooltipRows, "\n")
			for _, want := range []string{"Base: 17", primary + " / 3: scales", "Personality / 4: scales"} {
				if !strings.Contains(text, want) {
					t.Errorf("missing %q:\n%s", want, text)
				}
			}
			if !strings.Contains(card.subtitle, "+"+primary) {
				t.Errorf("subtitle omitted primary stat: %s", card.subtitle)
			}
		})
	}
}
