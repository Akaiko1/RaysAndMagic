package game

import (
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/spells"
)

func TestScenarioArgumentValidation(t *testing.T) {
	for _, tc := range []struct {
		args []string
		key  string
		fail bool
	}{
		{nil, "", false}, {[]string{"--test-arena"}, "arena", false}, {[]string{"--test-scenario", "pilgrimage"}, "pilgrimage", false}, {[]string{"--test-scenario=pilgrimage"}, "pilgrimage", false},
		{[]string{"--test-scenario"}, "", true}, {[]string{"--test-scenario="}, "", true}, {[]string{"--test-scenario", "../save"}, "", true}, {[]string{"--test-arena", "--test-scenario", "pilgrimage"}, "", true},
	} {
		key, err := TestScenarioArg(tc.args)
		if (err != nil) != tc.fail || (!tc.fail && key != tc.key) {
			t.Fatalf("%v -> %q,%v", tc.args, key, err)
		}
	}
}
func TestScenarioCatalogFailsOnTypos(t *testing.T) {
	for _, body := range []string{
		"scenarios: {test: {levle: 15}}",
		"scenarios: {test: {level: -1}}",
		"scenarios: {test: {party: [{name: Monk, class: monster}]}}",
		"scenarios: {test: {party: [{name: Monk, class: monk, skills: {iron_boddy: expert}}]}}",
		"scenarios: {test: {party: [{name: Monk, class: monk, skills: {iron_body: legendary}}]}}",
		"scenarios: {test: {level: 15}}\n---\nscenarios: {}",
		"scenarios: {test: {bench_party: rogues}}",
		"scenarios: {test: {bench_party: rogues, level: 20, party: [{name: Monk, class: monk}]}}",
		"scenarios: {test: {bench_party: rogues, level: 20, speed_target: 20}}",
		"scenarios: {test: {bench_party: rogues, level: 20, learn_school_spells: true}}",
		"scenarios: {test: {skill_mastery: legendary}}",
		"scenarios: {test: {party: [{name: Mage, class: sorcerer, schools: {fire: legendary}}]}}",
		"scenarios: {test: {party: [{name: Mage, class: sorcerer, schools: {firre: expert}}]}}",
	} {
		path := filepath.Join(t.TempDir(), "test.yaml")
		os.WriteFile(path, []byte(body), 0600)
		if _, err := LoadTestScenarios(path); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestSpellPlaytestScenario(t *testing.T) {
	g, _, _ := tbBehaviorGame(t, 24, 24)
	t.Chdir("../..")
	loadBenchContent(t)
	if err := g.ApplyTestScenario("assets/test_scenarios.yaml", "spells"); err != nil {
		t.Fatal(err)
	}
	for _, hero := range g.party.Members {
		if hero.Level != 50 || hero.HitPoints != hero.MaxHitPoints || hero.SpellPoints != hero.MaxSpellPoints {
			t.Errorf("%s is not ready for spell testing", hero.Name)
		}
		for _, skill := range hero.Skills {
			if skill.Mastery != character.MasteryGrandMaster {
				t.Errorf("%s has an untrained skill", hero.Name)
			}
		}
	}
	for _, school := range character.AllMagicSchools {
		for _, id := range config.GetSpellsBySchool(string(school)) {
			found := false
			for _, hero := range g.party.Members {
				found = found || hero.KnowsSpell(spells.SpellID(id))
			}
			if !found {
				t.Errorf("no test caster knows %s", id)
			}
		}
	}
	if g.currentLevelUpChoice() != nil {
		t.Fatal("training left a blocking level-up picker open")
	}
}
