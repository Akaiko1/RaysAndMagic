package game

import (
	"testing"
	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/config"
)

// turnBasedRegenSetup builds a game with a 4-member party where every member
// has Personality=10 (regen=2 per tick), SP=20/100, and is alive+conscious.
// Returns the game and a fresh InputHandler so tests can drive endPartyTurn.
// Loads weapon/item configs because newTestGame → NewParty → CreateCharacter
// pulls starter equipment from those YAML files; without them setup* panics.
func turnBasedRegenSetup(t *testing.T) (*MMGame, *InputHandler) {
	t.Helper()
	cfg := loadTestConfig(t)
	if _, err := config.LoadWeaponConfig("../../assets/weapons.yaml"); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	if _, err := config.LoadItemConfig("../../assets/items.yaml"); err != nil {
		t.Fatalf("load items: %v", err)
	}
	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()
	w := newTestWorld(cfg)
	g := newTestGame(cfg, w)
	g.turnBasedMode = true
	for _, m := range g.party.Members {
		m.Personality = 10
		m.MaxSpellPoints = 100
		m.SpellPoints = 20
		m.HitPoints = m.MaxHitPoints
	}
	ih := &InputHandler{game: g}
	return g, ih
}

func TestTurnBasedRegenFiresEveryNRounds(t *testing.T) {
	g, ih := turnBasedRegenSetup(t)

	// Rounds 1 .. N-1: no regen.
	for i := 1; i < TurnBasedSpRegenEveryNRounds; i++ {
		ih.endPartyTurn()
		for _, m := range g.party.Members {
			if m.SpellPoints != 20 {
				t.Fatalf("round %d: regen fired early, SP=%d", i, m.SpellPoints)
			}
		}
	}

	// Round N: tick fires for everyone.
	ih.endPartyTurn()
	for i, m := range g.party.Members {
		if m.SpellPoints != 22 { // 20 + regen(2)
			t.Errorf("char %d: SP=%d after first regen tick, want 22", i, m.SpellPoints)
		}
	}

	// Another N rounds → another tick.
	for i := 0; i < TurnBasedSpRegenEveryNRounds; i++ {
		ih.endPartyTurn()
	}
	for i, m := range g.party.Members {
		if m.SpellPoints != 24 {
			t.Errorf("char %d: SP=%d after second regen tick, want 24", i, m.SpellPoints)
		}
	}
}

func TestTurnBasedRegenSkipsUnconscious(t *testing.T) {
	g, ih := turnBasedRegenSetup(t)
	g.party.Members[0].AddCondition(character.ConditionUnconscious)
	startSP := g.party.Members[0].SpellPoints

	// Drive enough rounds to trigger several regen ticks.
	for i := 0; i < TurnBasedSpRegenEveryNRounds*3; i++ {
		ih.endPartyTurn()
	}

	if got := g.party.Members[0].SpellPoints; got != startSP {
		t.Errorf("unconscious char regenerated SP from %d to %d in turn-based", startSP, got)
	}

	// Healthy members should have regenerated 3 times → +6.
	for i, m := range g.party.Members {
		if i == 0 {
			continue
		}
		if m.SpellPoints != 26 { // 20 + 3*2
			t.Errorf("char %d (healthy): SP=%d after 3 ticks, want 26", i, m.SpellPoints)
		}
	}
}

func TestTurnBasedRegenSkipsDeadHP(t *testing.T) {
	g, ih := turnBasedRegenSetup(t)
	g.party.Members[1].HitPoints = 0
	startSP := g.party.Members[1].SpellPoints

	for i := 0; i < TurnBasedSpRegenEveryNRounds*2; i++ {
		ih.endPartyTurn()
	}

	if got := g.party.Members[1].SpellPoints; got != startSP {
		t.Errorf("dead (HP=0) char regenerated SP from %d to %d", startSP, got)
	}
}

func TestTurnBasedRegenCapsAtMax(t *testing.T) {
	g, ih := turnBasedRegenSetup(t)
	for _, m := range g.party.Members {
		m.SpellPoints = 99 // 1 short of cap with regen=2
		m.MaxSpellPoints = 100
	}

	for i := 0; i < TurnBasedSpRegenEveryNRounds; i++ {
		ih.endPartyTurn()
	}

	for i, m := range g.party.Members {
		if m.SpellPoints != 100 {
			t.Errorf("char %d: SP=%d after regen near cap, want 100", i, m.SpellPoints)
		}
	}
}

func TestTurnBasedRegenUsesEffectivePersonalityViaStatBonus(t *testing.T) {
	g, ih := turnBasedRegenSetup(t)
	g.statBonus = 20 // Bless adds +20 to all stats → Personality 10 → effective 30 → regen 4

	for i := 0; i < TurnBasedSpRegenEveryNRounds; i++ {
		ih.endPartyTurn()
	}

	for i, m := range g.party.Members {
		if m.SpellPoints != 24 { // 20 + regen(4)
			t.Errorf("char %d: SP=%d with statBonus=20, want 24", i, m.SpellPoints)
		}
	}
}
