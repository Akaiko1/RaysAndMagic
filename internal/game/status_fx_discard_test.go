package game

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/items"
)

// The stun-star ring must stay on screen for a point-blank monster, whose
// sprite is raised above the HUD bar (screenY can go negative).
func TestStunStarRingStaysOnScreen(t *testing.T) {
	// Far monster: ring sits above the head, untouched by the clamp.
	cy, _, ry := stunStarRingGeometry(260, 200)
	if want := 260 - 200*0.08; cy != want {
		t.Errorf("far monster ring center = %.1f, want %.1f (no clamp expected)", cy, want)
	}
	if cy-ry < 0 {
		t.Errorf("far monster ring top %.1f is off-screen", cy-ry)
	}

	// Point-blank monster raised above the HUD bar: topY deeply negative.
	cy, _, ry = stunStarRingGeometry(-300, 800)
	if cy-ry < 0 {
		t.Errorf("point-blank ring top %.1f is off-screen; clamp failed", cy-ry)
	}
}

// Quest items stay undiscardable, except items that opt out via
// `discardable: true` in items.yaml (a refusable quest choice).
func TestQuestItemDiscardability(t *testing.T) {
	loadTestConfig(t)
	for _, tc := range []struct {
		name string
		def  config.ItemDefinitionConfig
		want bool
	}{
		{"quest_opted_out", config.ItemDefinitionConfig{Name: "Fixture Refusable Relic", Type: "quest", Discardable: true}, true},
		{"plain_quest", config.ItemDefinitionConfig{Name: "Fixture Quest Relic", Type: "quest"}, false},
		{"non_quest", config.ItemDefinitionConfig{Name: "Fixture Trinket", Type: "trinket"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := "discard_fixture_" + tc.name
			def := tc.def
			config.GlobalItems.Items[key] = &def
			t.Cleanup(func() { delete(config.GlobalItems.Items, key) })
			if got := itemDiscardable(items.CreateItemFromYAML(key)); got != tc.want {
				t.Fatalf("itemDiscardable = %v, want %v", got, tc.want)
			}
		})
	}
}
