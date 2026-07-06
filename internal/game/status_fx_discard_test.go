package game

import (
	"testing"

	"ugataima/internal/items"
)

// The stun-star ring must stay on screen for a point-blank monster: the sprite
// is raised above the HUD bar (screenY goes negative for a big close sprite),
// which used to push the ring past the top edge - stars "disappeared" on any
// stun of a melee-range monster while the stun itself was working.
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
// `discardable: true` in items.yaml (the Lich Phylactery is a refusable choice).
func TestQuestItemDiscardability(t *testing.T) {
	loadTestConfig(t)

	phylactery := items.CreateItemFromYAML("lich_phylactery")
	if phylactery.Type != items.ItemQuest {
		t.Fatalf("lich_phylactery type = %v, want quest", phylactery.Type)
	}
	if !itemDiscardable(phylactery) {
		t.Errorf("lich_phylactery must be discardable (discardable: true in items.yaml)")
	}

	worldMap := items.CreateItemFromYAML("world_map")
	if itemDiscardable(worldMap) {
		t.Errorf("world_map is a plain quest item and must NOT be discardable")
	}

	potion := items.CreateItemFromYAML("health_potion")
	if !itemDiscardable(potion) {
		t.Errorf("non-quest items must always be discardable")
	}
}
