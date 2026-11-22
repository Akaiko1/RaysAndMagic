package character

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/stats"
)

// Each canonical stat reaches the hero field of the same name through every
// accessor: by name, as a block, on write, and through gear into the
// effective sum.
func TestBaseStatFieldsFollowCanonicalNames(t *testing.T) {
	c := &MMCharacter{Might: 1, Intellect: 2, Personality: 3, Endurance: 4, Accuracy: 5, Speed: 6, Luck: 7, Equipment: map[items.EquipSlot]items.Item{}}
	byField := map[string]int{"might": c.Might, "intellect": c.Intellect, "personality": c.Personality,
		"endurance": c.Endurance, "accuracy": c.Accuracy, "speed": c.Speed, "luck": c.Luck}
	if len(byField) != len(stats.Names) {
		t.Fatalf("hero has %d stat fields, canonical list has %d", len(byField), len(stats.Names))
	}
	base := c.BaseStats()
	for _, n := range stats.Names {
		if got := *c.BaseStatField(n); got != byField[n] {
			t.Errorf("BaseStatField(%s) = %d, want %d", n, got, byField[n])
		}
		if got := base.ValueByName(n); got != byField[n] {
			t.Errorf("BaseStats().%s = %d, want %d", n, got, byField[n])
		}
	}

	var shifted stats.StatBonuses
	for i := range stats.Names {
		*shifted.At(i) = 10 * (i + 1)
	}
	c.SetBaseStats(shifted)
	if c.Might != 10 || c.Luck != 70 || c.BaseStats() != shifted {
		t.Fatalf("SetBaseStats wrote %+v, want %+v", c.BaseStats(), shifted)
	}

	ring := items.Item{Attributes: map[string]int{}}
	for i, key := range items.StatBonusAttributes {
		ring.Attributes[key] = i + 1
	}
	c.Equipment[items.SlotRing1] = ring
	eff := c.EffectiveStatBlock()
	for i, n := range stats.Names {
		if got, want := eff.ValueByName(n), 10*(i+1)+i+1; got != want {
			t.Errorf("effective %s = %d, want base %d + gear %d", n, got, 10*(i+1), i+1)
		}
	}
	// Authored bonus_<stat> keys land on the stat of the same name.
	def := config.ItemStatBonuses{Might: 1, Luck: -2}
	if got := def.Stats(); got.Might != 1 || got.Luck != -2 || got.Speed != 0 {
		t.Fatalf("ItemStatBonuses.Stats() = %+v", got)
	}
}
