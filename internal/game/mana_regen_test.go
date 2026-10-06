package game

import (
	"testing"
	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/config"
)

// turnBasedRegenSetup builds a game with a 4-member party where every member
// has Personality=10 (regen=2 per tick), SP=20/100, and is alive+conscious.
// Loads weapon/item configs because newTestGame -> NewParty -> CreateCharacter
// pulls starter equipment from those YAML files; without them setup* panics.
func turnBasedRegenSetup(t *testing.T) *MMGame {
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
	return g
}

// TB SP regen through endPartyTurn: one payout every
// TurnBasedSpRegenEveryNRounds rounds, skipped for unconscious and HP-0
// members, capped at max, sized by effective Personality. Every member starts
// at Personality 10 (regen 2) with SP 20/100.
func TestTurnBasedRegenTable(t *testing.T) {
	n := TurnBasedSpRegenEveryNRounds
	all := func(sp int) func(int) int { return func(int) int { return sp } }
	except := func(idx, sp, others int) func(int) int {
		return func(i int) int {
			if i == idx {
				return sp
			}
			return others
		}
	}
	tests := []struct {
		name   string
		setup  func(g *MMGame)
		rounds int
		wantSP func(member int) int
	}{
		{"one round short of the cadence", nil, n - 1, all(20)},
		{"first cadence pays", nil, n, all(22)},
		{"second cadence pays again", nil, 2 * n, all(24)},
		{"unconscious member is skipped", func(g *MMGame) {
			g.party.Members[0].AddCondition(character.ConditionUnconscious)
		}, 3 * n, except(0, 20, 26)},
		{"HP-0 member is skipped", func(g *MMGame) { g.party.Members[1].HitPoints = 0 }, 2 * n, except(1, 20, 24)},
		{"payout caps at max", func(g *MMGame) {
			for _, m := range g.party.Members {
				m.SpellPoints = 99
			}
		}, n, all(100)},
		{"stat buff raises effective Personality", func(g *MMGame) {
			// Personality 10 -> effective 30 -> regen 4.
			g.addStatBuff(TimedStatBuff{SpellID: "bless", Frames: 1 << 30, Bonuses: character.UniformStatBonuses(20)})
		}, n, all(24)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := turnBasedRegenSetup(t)
			if tt.setup != nil {
				tt.setup(g)
			}
			for range tt.rounds {
				g.endPartyTurn()
			}
			for i, m := range g.party.Members {
				if want := tt.wantSP(i); m.SpellPoints != want {
					t.Errorf("member %d: SP=%d after %d rounds, want %d", i, m.SpellPoints, tt.rounds, want)
				}
			}
		})
	}
}

func TestModeSwitchRegenCadencesCannotPayBackToBack(t *testing.T) {
	t.Run("TB payout resets RT progress", func(t *testing.T) {
		g := turnBasedRegenSetup(t)
		member := g.party.Members[0]
		member.MaxHitPoints, member.HitPoints = 1000, 500
		member.BonusRegenPct = 5
		g.turnBasedMode = false
		for range character.ManaRegenIntervalFrames - 1 {
			g.updatePartyClocks()
		}

		g.turnBasedMode = true
		for range TurnBasedSpRegenEveryNRounds {
			g.endPartyTurn()
		}
		if member.SpellPoints != 22 {
			t.Fatalf("TB payout SP = %d, want 22", member.SpellPoints)
		}
		if member.HitPoints != 550 {
			t.Fatalf("TB Troll payout HP = %d, want 550", member.HitPoints)
		}

		g.turnBasedMode = false
		g.updatePartyClocks()
		if member.SpellPoints != 22 {
			t.Fatalf("returning to RT paid an immediate second regen: SP=%d", member.SpellPoints)
		}
		if member.HitPoints != 550 {
			t.Fatalf("returning to RT paid an immediate second Troll regen: HP=%d", member.HitPoints)
		}
	})

	t.Run("RT payout resets TB progress", func(t *testing.T) {
		g := turnBasedRegenSetup(t)
		member := g.party.Members[0]
		member.MaxHitPoints, member.HitPoints = 1000, 500
		member.BonusRegenPct = 5
		g.turnBasedSpRegenCount = TurnBasedSpRegenEveryNRounds - 1
		g.turnBasedMode = false
		for range character.ManaRegenIntervalFrames {
			g.updatePartyClocks()
		}
		if member.SpellPoints != 22 {
			t.Fatalf("RT payout SP = %d, want 22", member.SpellPoints)
		}
		if member.HitPoints != 550 {
			t.Fatalf("RT Troll payout HP = %d, want 550", member.HitPoints)
		}
		if g.turnBasedSpRegenCount != 0 {
			t.Fatalf("RT payout left TB regen progress at %d", g.turnBasedSpRegenCount)
		}

		g.turnBasedMode = true
		g.endPartyTurn()
		if member.SpellPoints != 22 {
			t.Fatalf("returning to TB paid an immediate second regen: SP=%d", member.SpellPoints)
		}
		if member.HitPoints != 550 {
			t.Fatalf("returning to TB paid an immediate second Troll regen: HP=%d", member.HitPoints)
		}
	})
}
