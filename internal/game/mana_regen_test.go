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
// config.RegenerationRounds rounds, skipped for unconscious and HP-0
// members, capped at max, sized by effective Personality. Every member starts
// at Personality 10 (regen 2) with SP 20/100.
func TestTurnBasedRegenTable(t *testing.T) {
	n := config.RegenerationRounds
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
			g.addStatBuff(TimedStatBuff{SpellID: "bless", Clock: framesClock(1 << 30), Bonuses: character.UniformStatBonuses(20)})
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

// One period serves both modes: time spent in one mode stays spent in the
// other, so a switch can neither pay early nor pay twice.
func TestModeSwitchRegenPeriodCarriesOver(t *testing.T) {
	tps := config.GetTargetTPS()
	period := config.RegenerationSeconds * tps
	perRound := period / config.RegenerationRounds
	setup := func(t *testing.T) (*MMGame, *character.MMCharacter) {
		g := turnBasedRegenSetup(t)
		member := g.party.Members[0]
		member.MaxHitPoints, member.HitPoints = 1000, 500
		member.BonusRegenPct = 5
		return g, member
	}
	rt := func(g *MMGame, frames int) {
		g.turnBasedMode = false
		for range frames {
			g.updatePartyClocks()
		}
	}
	tb := func(g *MMGame, rounds int) {
		g.turnBasedMode = true
		for range rounds {
			g.endPartyTurn()
		}
	}
	paid := func(t *testing.T, member *character.MMCharacter, payouts int) {
		t.Helper()
		if member.SpellPoints != 20+2*payouts || member.HitPoints != 500+50*payouts {
			t.Fatalf("SP %d HP %d, want %d payouts", member.SpellPoints, member.HitPoints, payouts)
		}
	}
	t.Run("RT then TB", func(t *testing.T) {
		g, member := setup(t)
		rt(g, period-1)
		paid(t, member, 0)
		tb(g, 1) // the frame left is less than a round
		paid(t, member, 1)
		rt(g, period-1)
		paid(t, member, 1)
		rt(g, 1)
		paid(t, member, 2)
	})
	t.Run("TB then RT", func(t *testing.T) {
		g, member := setup(t)
		tb(g, config.RegenerationRounds-1)
		paid(t, member, 0)
		rt(g, perRound-1)
		paid(t, member, 0)
		rt(g, 1)
		paid(t, member, 1)
		tb(g, config.RegenerationRounds-1)
		paid(t, member, 1)
	})
}

// A fight slows SP regen only: RT every CombatSPRegenSeconds instead of the
// regen interval, TB every CombatSPRegenRounds instead of the regen rounds.
// HP regen (Troll card) keeps its cadence, and the party out of a fight keeps
// both. Every member regenerates 2 SP and 5% of 1000 HP per tick.
func TestSPRegenSlowsWhileFighting(t *testing.T) {
	tps := config.GetTargetTPS()
	for _, tc := range []struct {
		name                  string
		tb, fighting          bool
		steps, wantSP, wantHP int
	}{
		{"TB calm pays at the regen rounds", true, false, config.RegenerationRounds, 22, 550},
		{"TB fight holds SP at the regen rounds", true, true, config.RegenerationRounds, 20, 550},
		{"TB fight pays SP at the fight rounds", true, true, config.CombatSPRegenRounds, 22, 550},
		{"RT calm pays at the regen interval", false, false, (config.RegenerationSeconds * config.GetTargetTPS()), 22, 550},
		{"RT fight holds SP at the regen interval", false, true, (config.RegenerationSeconds * config.GetTargetTPS()), 20, 550},
		{"RT fight pays SP at the fight interval", false, true, config.CombatSPRegenSeconds * tps, 22, 550},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := turnBasedRegenSetup(t)
			g.turnBasedMode = tc.tb
			for _, m := range g.party.Members {
				m.MaxHitPoints, m.HitPoints, m.BonusRegenPct = 1000, 500, 5
			}
			if tc.fighting {
				ts := float64(g.config.GetTileSize())
				spawnMonsterAtTile(g, "goblin", TileIndex(g.camera.X, ts)+2, TileIndex(g.camera.Y, ts), ts)
			}
			if g.partyInCombat() != tc.fighting {
				t.Fatalf("fixture: partyInCombat=%v", g.partyInCombat())
			}
			for range tc.steps {
				if tc.tb {
					g.endPartyTurn()
				} else {
					g.updatePartyClocks()
				}
			}
			m := g.party.Members[0]
			if m.SpellPoints != tc.wantSP || m.HitPoints != tc.wantHP {
				t.Fatalf("SP=%d HP=%d after %d steps, want SP %d HP %d", m.SpellPoints, m.HitPoints, tc.steps, tc.wantSP, tc.wantHP)
			}
		})
	}
}

// The SP period follows the fight: starting or ending one swaps the running
// period's length and keeps the time already spent, counted in each mode.
func TestSPRegenPeriodFollowsTheFight(t *testing.T) {
	tps := config.GetTargetTPS()
	calm, fight := config.RegenerationSeconds*tps, config.CombatSPRegenSeconds*tps
	for _, tc := range []struct {
		name            string
		tb, startsFight bool
		before, after   int // ticks before and after the switch up to the payout
	}{
		{"RT fight starts mid-period", false, true, 100, fight - 100},
		{"RT fight ends past the calm period", false, false, calm + 100, 1},
		{"TB fight starts mid-period", true, true, 1, config.CombatSPRegenRounds - 1},
		{"TB fight ends past the calm rounds", true, false, config.RegenerationRounds, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := turnBasedRegenSetup(t)
			g.turnBasedMode = tc.tb
			member := g.party.Members[0]
			ts := float64(g.config.GetTileSize())
			setFight := func(on bool) {
				g.world.Monsters = nil
				if on {
					spawnMonsterAtTile(g, "goblin", TileIndex(g.camera.X, ts)+2, TileIndex(g.camera.Y, ts), ts)
				}
				if g.partyInCombat() != on {
					t.Fatalf("fixture: fighting=%v", g.partyInCombat())
				}
			}
			step := func(n int) {
				for range n {
					if tc.tb {
						g.endPartyTurn()
					} else {
						g.updatePartyClocks()
					}
				}
			}
			setFight(!tc.startsFight)
			step(tc.before)
			setFight(tc.startsFight)
			step(tc.after - 1)
			if member.SpellPoints != 20 {
				t.Fatalf("paid early: SP %d", member.SpellPoints)
			}
			step(1)
			if member.SpellPoints != 22 {
				t.Fatalf("no payout at the switched period: SP %d", member.SpellPoints)
			}
		})
	}
}
