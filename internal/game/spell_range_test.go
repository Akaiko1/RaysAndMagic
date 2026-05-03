package game

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/spells"
)

func TestCalculateSpellRangeTilesRequiresPhysics(t *testing.T) {
	oldSpells := config.GlobalSpells
	t.Cleanup(func() {
		config.GlobalSpells = oldSpells
	})

	config.GlobalSpells = &config.SpellSystemConfig{
		Spells: map[string]*config.SpellDefinitionConfig{
			"with_physics": {
				Physics: &config.ProjectilePhysicsConfig{
					RangeTiles: 7.5,
				},
			},
			"without_physics": {},
			"zero_range": {
				Physics: &config.ProjectilePhysicsConfig{
					RangeTiles: 0,
				},
			},
		},
	}

	cs := &CombatSystem{}

	rangeTiles, ok := cs.CalculateSpellRangeTiles(spells.SpellID("with_physics"))
	if !ok {
		t.Fatalf("expected spell with physics range to be available")
	}
	if rangeTiles != 7.5 {
		t.Fatalf("expected physics range 7.5, got %.1f", rangeTiles)
	}

	if _, ok := cs.CalculateSpellRangeTiles(spells.SpellID("without_physics")); ok {
		t.Fatalf("expected spell without physics range to be unavailable")
	}
	if _, ok := cs.CalculateSpellRangeTiles(spells.SpellID("zero_range")); ok {
		t.Fatalf("expected zero physics range to be unavailable")
	}
	if _, ok := cs.CalculateSpellRangeTiles(spells.SpellID("missing")); ok {
		t.Fatalf("expected missing spell range to be unavailable")
	}
}
