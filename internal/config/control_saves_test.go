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
	// The test's own rule: the curve floor (20) sits above min_percent (10)
	// so the two clamps read apart. Expected chances are hand-computed.
	rule := ControlSaveConfig{
		Mechanics:             []MechanicKind{"turn_undead", "mass_fear", "feeblemind"},
		DiminishingMechanics:  []MechanicKind{"turn_undead", "mass_fear"},
		Curve:                 []ControlSavePoint{{-4, 20}, {0, 40}, {10, 70}},
		MinPercent:            10,
		MaxPercent:            80,
		MasteryPenaltyPercent: 5,
		ResistanceWeight:      0.5,
		ResistedMessage:       "%s resists %s.",
		Summary:               "summary",
		Details:               "details",
		DiminishingSummary:    "diminishing",
	}
	for _, tc := range []struct {
		difference, deficit, resistance int
		want                            float64
	}{
		{-20, 0, 0, 20}, // below the curve: its first point
		{-4, 0, 0, 20},
		{-2, 0, 0, 30}, // halfway from 20 to 40
		{0, 0, 0, 40},
		{3, 0, 0, 49}, // 40 + 30 * 3/10
		{10, 0, 0, 70},
		{100, 0, 0, 70},   // above the curve: its last point
		{0, 2, 0, 50},     // + 2 tiers * 5
		{0, 0, 40, 60},    // + 40 resistance * 0.5
		{10, 3, 0, 80},    // 85 clamps to max_percent
		{-20, 0, -40, 10}, // 0 clamps to min_percent
	} {
		if got := rule.Chance(tc.difference, tc.deficit, tc.resistance); got != tc.want {
			t.Errorf("%+v: got %g", tc, got)
		}
	}
	for _, tc := range []struct {
		name string
		edit func(*ControlSaveConfig)
	}{
		{"valid", func(*ControlSaveConfig) {}},
		{"missing", func(c *ControlSaveConfig) { *c = ControlSaveConfig{} }},
		{"unordered", func(c *ControlSaveConfig) { c.Curve[1].LevelDifference = c.Curve[0].LevelDifference }},
		{"decreasing", func(c *ControlSaveConfig) { c.Curve[2].Percent = 30 }},
		{"nan", func(c *ControlSaveConfig) { c.ResistanceWeight = math.NaN() }},
		{"infinity", func(c *ControlSaveConfig) { c.MasteryPenaltyPercent = math.Inf(1) }},
		{"invalid_cap", func(c *ControlSaveConfig) { c.MaxPercent = 101 }},
		{"unknown_mechanic", func(c *ControlSaveConfig) { c.Mechanics = []MechanicKind{"typo"} }},
		{"duplicate_mechanic", func(c *ControlSaveConfig) { c.Mechanics = []MechanicKind{"mass_fear", "mass_fear"} }},
		{"silence_dr", func(c *ControlSaveConfig) { c.DiminishingMechanics = []MechanicKind{"feeblemind"} }},
		{"duplicate_dr", func(c *ControlSaveConfig) { c.DiminishingMechanics = []MechanicKind{"mass_fear", "mass_fear"} }},
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
			_, err = LoadConfig(path)
			if tc.name == "valid" {
				if err != nil {
					t.Fatalf("valid control config rejected: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "control_saves") {
				t.Fatalf("invalid control config accepted: %v", err)
			}
		})
	}
}
