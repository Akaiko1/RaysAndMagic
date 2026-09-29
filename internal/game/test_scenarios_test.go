package game

import (
	"os"
	"path/filepath"
	"testing"
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
	} {
		path := filepath.Join(t.TempDir(), "test.yaml")
		os.WriteFile(path, []byte(body), 0600)
		if _, err := LoadTestScenarios(path); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}
