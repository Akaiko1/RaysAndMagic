package game

import (
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/config"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

// newBossTestGame builds a 6x6 open world with one quest-gated evasive boss
// placed far from the party (outside its evade radius), so only the hurt-latch
// can trigger a blink.
func newBossTestGame(t *testing.T, cfg *config.Config) (*MMGame, *monsterPkg.Monster3D) {
	t.Helper()
	w := world.NewWorld3D(cfg)
	w.Width, w.Height = 6, 6
	w.Tiles = make([][]world.TileType3D, w.Height)
	for y := 0; y < w.Height; y++ {
		w.Tiles[y] = make([]world.TileType3D, w.Width)
		for x := 0; x < w.Width; x++ {
			w.Tiles[y][x] = world.TileEmpty
		}
	}
	g := newTestGame(cfg, w)
	g.turnBasedMode = true

	boss := &monsterPkg.Monster3D{
		ID: "boss_tb_1", Name: "Golden Thief Bug",
		X: 352, Y: 352, // tile (5,5) center; party camera sits at (64,64)
		HitPoints: 200, MaxHitPoints: 200,
		PassiveUntilQuest: "culverts_valves", // no quest manager in test → evasive
		EvadeRadiusTiles:  3,
		BossCooldownSecs:  1,
	}
	w.Monsters = append(w.Monsters, boss)
	g.collisionSystem.RegisterEntity(collision.NewEntity(boss.ID, boss.X, boss.Y, 32, 32, collision.CollisionTypeMonster, false))
	return g, boss
}

// The evasive boss must dodge MID-party-round in TB: the per-frame tick latches
// the HP loss and blinks immediately instead of waiting for the monster turn
// (which let a focused party round kill it before it ever dodged).
func TestEvasiveBossTB_BlinksWhenHurtMidPartyRound(t *testing.T) {
	cfg := loadTestConfig(t)
	g, boss := newBossTestGame(t, cfg)

	g.combat = NewCombatSystem(g)
	g.combat.tickEvasiveBossesTB() // initialise the HP latch
	if boss.BossCD != 0 {
		t.Fatalf("boss far from party must not blink unprovoked")
	}

	boss.HitPoints -= 50 // a party member's hit lands mid-round
	g.combat.tickEvasiveBossesTB()

	if boss.BossCD == 0 {
		t.Fatalf("hurt evasive boss must blink on the very next frame (BossCD untouched)")
	}
	if boss.BossHurtPending {
		t.Errorf("hurt latch must be consumed by the blink")
	}
}

// Crowd control suppresses the per-frame evasive blink like any other action.
func TestEvasiveBossTB_CrowdControlSuppressesBlink(t *testing.T) {
	cfg := loadTestConfig(t)

	cases := []struct {
		name string
		cc   func(m *monsterPkg.Monster3D)
	}{
		{"stun_turns", func(m *monsterPkg.Monster3D) { m.StunTurnsRemaining = 1 }},
		{"stun_frames", func(m *monsterPkg.Monster3D) { m.StunFramesRemaining = 10 }},
		{"charm", func(m *monsterPkg.Monster3D) { m.Pacified = true }},
		{"bind", func(m *monsterPkg.Monster3D) { m.Bound = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, boss := newBossTestGame(t, cfg)
			g.combat = NewCombatSystem(g)
			g.combat.tickEvasiveBossesTB() // initialise the HP latch
			tc.cc(boss)
			boss.HitPoints -= 50
			g.combat.tickEvasiveBossesTB()
			if boss.BossCD != 0 {
				t.Fatalf("%s boss must not blink while controlled", tc.name)
			}
		})
	}
}
