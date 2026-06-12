package game

import (
	"strings"
	"testing"

	"ugataima/internal/spells"
)

// TestSpellTooltipMechanics_Complete asserts every spell's tooltip surfaces its
// real mechanics (the fields combat actually uses), and that no-damage spells
// (Charm/Disintegrate) don't claim damage.
func TestSpellTooltipMechanics_Complete(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	char := cs.game.party.Members[0]

	lines := func(id string) string {
		if _, err := spells.GetSpellDefinitionByID(spells.SpellID(id)); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		return GetSpellTooltip(spells.SpellID(id), char, cs)
	}

	mustContain := []struct{ id, want string }{
		{"psychic_shock", "Stun chance: 10%"},
		{"stone_skin", "Party takes -6 damage per hit"},
		{"heroism", "Party attacks deal +10 damage"},
		{"day_of_the_gods", "Party takes 50% less damage"},
		{"hour_of_power", "Party attacks deal +15 damage"},
		{"hour_of_power", "Party takes -5 damage per hit"},
		{"stun", "Stuns every monster within 3.0 tiles for 4s"},
		{"darkness", "Stuns every monster within 5.0 tiles for 5s"},
		{"disintegrate", "Disintegrate: 15%"},
		{"charm", "Pacifies"},
		{"charm", "120s"},
		{"bind_undead", "undead target for 300s"},
		{"hot_steam", "DAMAGE PER TICK"},
		{"hot_steam", "searing everything inside every 3s"},
		// Scaling is now a structured decomposition line ("Stat (value /
		// divisor): +N") instead of a prose "scales with" sentence.
		{"firebolt", "Intellect ("},
		{"psychic_shock", "Personality ("}, // self-magic school → Personality
		{"hot_steam", "Intellect ("},
		{"heal", "Personality ("},
		{"inferno", "within 7.0 tiles for 45 damage"},
		{"raise_dead", "Revives a fallen ally to 25% HP"},
		{"resurrect", "full HP"},
		{"mass_heal", "Heals the entire party"},
		{"awaken", "Wakes all unconscious allies"},
	}
	for _, c := range mustContain {
		if got := lines(c.id); !strings.Contains(got, c.want) {
			t.Errorf("%s tooltip missing %q. got:\n%s", c.id, c.want, got)
		}
	}

	// No-damage spells must NOT show damage lines.
	for _, id := range []string{"charm", "bind_undead", "disintegrate"} {
		if got := lines(id); strings.Contains(got, "Total Damage") || strings.Contains(got, "Base Damage") {
			t.Errorf("%s is deals_no_damage but tooltip shows damage:\n%s", id, got)
		}
	}
}
