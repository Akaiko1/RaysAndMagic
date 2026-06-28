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

	// Completeness check = the FULL (Shift-held) view: the Base→Stat→Mastery
	// decomposition and the universal RULES are detail-only, hidden in the
	// default compact tooltip.
	lines := func(id string) string {
		def, err := spells.GetSpellDefinitionByID(spells.SpellID(id))
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		return buildSpellTooltipUnified(def, char, cs, true)
	}

	mustContain := []struct{ id, want string }{
		{"psychic_shock", "Stun chance: 10%"},
		{"stone_skin", "Party takes -4 to -10 damage per hit by mastery"},
		{"stone_skin", "Current reduction: -4 per hit"},
		{"heroism", "Party physical attacks deal +3 to +10 damage by mastery"},
		{"day_of_the_gods", "Party takes 10% to 30% less damage by mastery"},
		{"hour_of_power", "Party attacks deal +5 to +15 damage by mastery"},
		{"hour_of_power", "Party takes -1 to -5 damage per hit by mastery"},
		{"stun", "Stuns every monster within 3.0 tiles for 4s"},
		{"darkness", "Stuns every monster within 5.0 tiles for 5s"},
		{"disintegrate", "Disintegrate: 15%"},
		{"charm", "Pacifies"},
		{"charm", "120s"},
		{"bind_undead", "undead target for 300s"},
		{"hot_steam", "DAMAGE PER TICK"},
		// Cadence lives in the structured ZONE section; the prose line states only
		// who it hits (monsters, not the party).
		{"hot_steam", "RT: one tick every 3s"},
		{"hot_steam", "scalds any monster inside (your party is unharmed)"},
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
