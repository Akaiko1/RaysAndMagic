package config

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestControlSaveCurveAndValidation(t *testing.T) {
	original, err := os.ReadFile("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err = yaml.Unmarshal(original, &cfg); err != nil {
		t.Fatal(err)
	}
	rule := cfg.StatusEffects.ControlSaves
	for _, tc := range []struct {
		difference, deficit, resistance int
		want                            float64
	}{
		{-50, 0, 0, 25}, {-5, 0, 0, 25}, {-2, 0, 0, 34}, {0, 0, 0, 40},
		{3, 0, 0, 46}, {5, 0, 0, 50}, {10, 0, 0, 60}, {12, 0, 0, 66},
		{15, 0, 0, 75}, {100, 0, 0, 75}, {0, 3, 0, 55}, {0, 2, 0, 50},
		{0, 1, 0, 45}, {0, 0, 40, 60}, {0, 3, 100, 75}, {-5, 0, -50, 25},
	} {
		if got := rule.Chance(tc.difference, tc.deficit, tc.resistance); got != tc.want {
			t.Errorf("%+v: got %g", tc, got)
		}
	}
	for _, tc := range []struct {
		name string
		edit func(*ControlSaveConfig)
	}{
		{"missing", func(c *ControlSaveConfig) { *c = ControlSaveConfig{} }},
		{"unordered", func(c *ControlSaveConfig) { c.Curve[1].LevelDifference = -5 }},
		{"decreasing", func(c *ControlSaveConfig) { c.Curve[2].Percent = 30 }},
		{"nan", func(c *ControlSaveConfig) { c.ResistanceWeight = math.NaN() }},
		{"infinity", func(c *ControlSaveConfig) { c.MasteryPenaltyPercent = math.Inf(1) }},
		{"invalid_cap", func(c *ControlSaveConfig) { c.MaxPercent = 101 }},
		{"unknown_mechanic", func(c *ControlSaveConfig) { c.Mechanics = []string{"typo"} }},
		{"duplicate_mechanic", func(c *ControlSaveConfig) { c.Mechanics = []string{"mass_fear", "mass_fear"} }},
		{"silence_dr", func(c *ControlSaveConfig) { c.DiminishingMechanics = []string{"feeblemind"} }},
		{"duplicate_dr", func(c *ControlSaveConfig) { c.DiminishingMechanics = []string{"mass_fear", "mass_fear"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var data map[string]any
			if err := yaml.Unmarshal(original, &data); err != nil {
				t.Fatal(err)
			}
			c := rule
			c.Curve = append([]ControlSavePoint(nil), rule.Curve...)
			tc.edit(&c)
			data["status_effects"].(map[string]any)["control_saves"] = c
			encoded, err := yaml.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err = os.WriteFile(path, encoded, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = LoadConfig(path); err == nil || !strings.Contains(err.Error(), "control_saves") {
				t.Fatalf("invalid control config accepted: %v", err)
			}
		})
	}
}
