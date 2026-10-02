package game

import (
	"testing"

	"ugataima/internal/character"
)

// sorcererIndex returns the party slot of the (first) Sorcerer.
func sorcererIndex(t *testing.T, g *MMGame) int {
	t.Helper()
	for i, m := range g.party.Members {
		if m.Class == character.ClassSorcerer {
			return i
		}
	}
	t.Fatal("no sorcerer in test party")
	return -1
}

// TestApplyLichPromotion_UnlocksDarkAndPicks2 exercises the full Lich path
// mechanics (status, school unlock, the multi-select spell picker granting
// exactly two unknown Dark spells), bypassing the asset-based eligibility check.
func TestApplyLichPromotion_UnlocksDarkAndPicks2(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	idx := sorcererIndex(t, g)

	g.applyLichPromotion(idx)
	m := g.party.Members[idx]

	if !m.IsLich() {
		t.Error("character should be a Lich after promotion")
	}
	if m.ClassDisplayName() != "Lich" {
		t.Errorf("class display = %q, want Lich", m.ClassDisplayName())
	}
	if m.MagicSchools[character.MagicSchoolDark] == nil {
		t.Fatal("Dark school should be unlocked")
	}
	req := g.currentLevelUpChoice()
	if req == nil {
		t.Fatal("a spell picker should be open after promotion")
	}
	if req.maxSelections != 2 {
		t.Errorf("picker maxSelections = %d, want 2", req.maxSelections)
	}

	// Cap enforcement: a third toggle is ignored.
	g.toggleLevelUpSelection(0)
	g.toggleLevelUpSelection(1)
	g.toggleLevelUpSelection(2)
	if got := req.selectedCount(); got != 2 {
		t.Errorf("selectedCount = %d, want 2 (cap)", got)
	}

	g.confirmLevelUpSelections()
	if g.currentLevelUpChoice() != nil {
		t.Error("picker should close after confirming")
	}
	if learned := len(m.MagicSchools[character.MagicSchoolDark].KnownSpells); learned != 2 {
		t.Errorf("learned %d Dark spells, want 2", learned)
	}
}

// TestApplyArchmagePromotion_UnlocksLight mirrors the Lich test for the Light path.
func TestApplyArchmagePromotion_UnlocksLight(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	idx := sorcererIndex(t, g)

	g.applyArchmagePromotion(idx)
	m := g.party.Members[idx]

	if !m.IsArchmage() {
		t.Error("character should be an Archmage after promotion")
	}
	if m.MagicSchools[character.MagicSchoolLight] == nil {
		t.Fatal("Light school should be unlocked")
	}
	req := g.currentLevelUpChoice()
	if req == nil || req.maxSelections != 2 {
		t.Fatalf("a 2-pick Light spell picker should be open, got %+v", req)
	}
	g.toggleLevelUpSelection(0)
	g.toggleLevelUpSelection(1)
	g.confirmLevelUpSelections()
	if learned := len(m.MagicSchools[character.MagicSchoolLight].KnownSpells); learned != 2 {
		t.Errorf("learned %d Light spells, want 2", learned)
	}
}
