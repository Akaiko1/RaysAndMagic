package config

import (
	"math"
	"strings"
	"testing"
)

func TestLoadCardMoveBurstRadius(t *testing.T) {
	for _, tc := range []struct {
		name   string
		radius any
		valid  bool
	}{
		{"authored", 5, true},
		{"fractional", 3.5, true},
		{"missing", nil, false},
		{"zero", 0, false},
		{"negative", -1, false},
		{"infinite", math.Inf(1), false},
		{"nan", math.NaN(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			preserveUsageCatalog(t)
			root := usageFixture(t)
			def := root["items"].(map[string]any)["gorilla_titan_card"].(map[string]any)
			delete(def, "card_move_aoe_radius_tiles")
			if tc.radius != nil {
				def["card_move_aoe_radius_tiles"] = tc.radius
			}
			cfg, err := loadUsageFixture(t, root)
			if tc.valid {
				if err != nil {
					t.Fatal(err)
				}
				if cfg.Items["gorilla_titan_card"].CardMoveAoeRadiusTiles <= 0 {
					t.Fatal("radius not loaded")
				}
			} else if err == nil || !strings.Contains(err.Error(), "card_move_aoe_radius_tiles") {
				t.Fatalf("expected radius validation error, got %v", err)
			}
		})
	}
}
