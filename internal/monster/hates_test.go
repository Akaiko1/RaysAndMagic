package monster

import "testing"

// TestHatesActiveTrait checks that a passive monster only wakes for traits it
// actually hates, and only while that trait is active in the party.
func TestHatesActiveTrait(t *testing.T) {
	// reset global party traits between assertions
	defer func() { PartyTraits = map[string]bool{} }()

	lichHater := &Monster3D{HatesTraits: []string{"lich"}}
	indifferent := &Monster3D{HatesTraits: nil}

	PartyTraits = map[string]bool{"lich": false}
	if lichHater.HatesActiveTrait() {
		t.Error("should not hate-aggro when no lich in party")
	}

	PartyTraits = map[string]bool{"lich": true}
	if !lichHater.HatesActiveTrait() {
		t.Error("lich-hater should aggro when a lich is in the party")
	}
	if indifferent.HatesActiveTrait() {
		t.Error("a monster with no hated traits must never hate-aggro")
	}
}
