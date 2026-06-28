package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
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

// TestStavesAndBooksAreRangedMagic locks in Feature 3: staves/books are ranged
// (Range > 3) projectile weapons carrying a magic projectile_school.
func TestStavesAndBooksAreRangedMagic(t *testing.T) {
	if _, err := config.LoadWeaponConfig("../../assets/weapons.yaml"); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	// projectile_school is cosmetic; it must name the weapon's REAL magic element
	// (no fake "arcane" school, which would mislead — staves deal these types).
	cases := map[string]string{
		"oak_staff":        "air",
		"battle_staff":     "air",
		"archmage_staff":   "fire",
		"book_of_darkness": "dark",
	}
	for key, wantSchool := range cases {
		def, ok := config.GlobalWeapons.Weapons[key]
		if !ok {
			t.Errorf("%s missing from weapons.yaml", key)
			continue
		}
		if def.Range <= 3 {
			t.Errorf("%s range = %d, want > 3 (ranged)", key, def.Range)
		}
		if def.ProjectileSchool != wantSchool {
			t.Errorf("%s projectile_school = %q, want %q", key, def.ProjectileSchool, wantSchool)
		}
		if def.Physics == nil || def.Graphics == nil {
			t.Errorf("%s must define physics + graphics to be a valid projectile weapon", key)
		}
	}
}
