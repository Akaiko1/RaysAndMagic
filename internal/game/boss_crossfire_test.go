package game

import (
	"testing"

	"ugataima/internal/monster"
)

// Every inactive boss style must remain neutral to party-controlled monsters in
// real time. These states use different mechanics: the Samurai is dormant, the
// Golden Thief Bug evades, and the Orc Warlord is warded by an idol.
func TestInactiveBossesIgnoreBoundAlliesRT(t *testing.T) {
	cases := []struct {
		name           string
		bossKey        string
		addWardIdol    bool
		assertInactive func(t *testing.T, game *MMGame, boss *monster.Monster3D)
	}{
		{
			name:    "dormant_samurai",
			bossKey: "old_samurai",
			assertInactive: func(t *testing.T, _ *MMGame, boss *monster.Monster3D) {
				t.Helper()
				if !boss.BossDormant {
					t.Fatal("unfinished castle quest must leave Samurai Warlord dormant")
				}
			},
		},
		{
			name:    "evasive_golden_thief_bug",
			bossKey: "golden_thief_bug",
			assertInactive: func(t *testing.T, game *MMGame, boss *monster.Monster3D) {
				t.Helper()
				if !game.combat.bossEvasive(boss) || boss.BossDormant {
					t.Fatal("unfinished valves quest must leave Golden Thief Bug evasive, not dormant")
				}
			},
		},
		{
			name:        "idol_warded_orc_warlord",
			bossKey:     "orc_hero_boss",
			addWardIdol: true,
			assertInactive: func(t *testing.T, _ *MMGame, boss *monster.Monster3D) {
				t.Helper()
				if !boss.BossWarded {
					t.Fatal("living jungle idol must leave Orc Warlord warded")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := loadTestConfig(t)
			worldTest := newTestWorldSized(cfg, 12, 12)
			game := newTestGame(cfg, worldTest)
			game.combat = NewCombatSystem(game)
			ts := float64(cfg.GetTileSize())

			boss := monster.NewMonster3DFromConfig(8*ts+ts/2, 6*ts+ts/2, tc.bossKey, game.config)
			ally := monster.NewMonster3DFromConfig(7*ts+ts/2, 6*ts+ts/2, "revenant", game.config)
			if boss == nil || ally == nil {
				t.Fatal("boss and revenant must load from monsters.yaml")
			}
			markCardAlly(ally)
			game.world.Monsters = []*monster.Monster3D{boss, ally}
			if tc.addWardIdol {
				idol := monster.NewMonster3DFromConfig(6*ts+ts/2, 6*ts+ts/2, "jungle_idol", game.config)
				if idol == nil {
					t.Fatal("jungle idol must load from monsters.yaml")
				}
				game.world.Monsters = append(game.world.Monsters, idol)
			}
			game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
			placePlayerAtTile(game, 0, 0, ts)

			game.refreshMonsterAIState()
			tc.assertInactive(t, game, boss)
			if boss.AIFoe != nil {
				t.Fatalf("inactive boss acquired bound revenant: %v", boss.AIFoe)
			}
			if ally.AIFoe == boss {
				t.Fatal("bound revenant selected inactive boss")
			}

			// Defend against a stale crossfire target surviving a state refresh.
			boss.AIFoe = ally
			hp := ally.HitPoints
			game.combat.HandleMonsterInteractions()
			if ally.HitPoints != hp {
				t.Fatalf("inactive boss damaged bound revenant: hp=%d, want %d", ally.HitPoints, hp)
			}
			if boss.AttackAnimFrames != 0 {
				t.Fatalf("inactive boss played an attack animation: %d", boss.AttackAnimFrames)
			}
		})
	}
}

// The evasive-boss guard must stay below the normal RT control gates. Otherwise
// it prevents crossfire correctly but accidentally lets an immobilized Golden
// Thief Bug blink away from the party.
func TestEvasiveBossRTControlSuppressesBlink(t *testing.T) {
	cases := []struct {
		name  string
		apply func(*monster.Monster3D)
	}{
		{"stun", func(m *monster.Monster3D) { m.StunFramesRemaining = 1 }},
		{"charm", func(m *monster.Monster3D) { m.Pacified = true }},
		{"bind", func(m *monster.Monster3D) { m.Bound = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := loadTestConfig(t)
			worldTest := newTestWorldSized(cfg, 12, 12)
			game := newTestGame(cfg, worldTest)
			game.combat = NewCombatSystem(game)
			ts := float64(cfg.GetTileSize())
			boss := monster.NewMonster3DFromConfig(8*ts+ts/2, 6*ts+ts/2, "golden_thief_bug", game.config)
			if boss == nil {
				t.Fatal("Golden Thief Bug must load from monsters.yaml")
			}
			game.world.Monsters = []*monster.Monster3D{boss}
			game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
			placePlayerAtTile(game, 6, 6, ts) // inside its three-tile evade radius
			game.refreshMonsterAIState()
			tc.apply(boss)
			startX, startY := boss.X, boss.Y

			game.combat.HandleMonsterInteractions()
			if boss.BossCD != 0 || boss.X != startX || boss.Y != startY {
				t.Fatalf("%s evasive boss blinked under control: cd=%d pos=(%.0f,%.0f)", tc.name, boss.BossCD, boss.X, boss.Y)
			}
		})
	}
}
