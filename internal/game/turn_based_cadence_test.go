package game

import (
	"slices"
	"testing"

	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Turn-based swings per attack action are fractional and accumulate
// (monster.TurnBasedAttackRate). Each row drives real TB attack actions through
// commitMonsterAttack and counts the swings that land. Expected sequences are
// worked out by hand from rate = 1/cooldown (x 1/enrage cooldown), min 1.
func TestTurnBasedSwingsAccumulateThroughAttackActions(t *testing.T) {
	setup := func(t *testing.T, cooldown, enrageCooldown float64) (*CombatSystem, *monster.Monster3D, *int) {
		t.Helper()
		cs := newTestCombatSystemWithConfig(t)
		elementalTestBiome(t, cs, "earth")
		g := cs.game
		g.party.Members = g.party.Members[:1]
		ch := g.party.Members[0]
		isolateTrueDamageMember(ch, 0)
		ch.HitPoints, ch.MaxHitPoints = 100000, 100000
		m := mkTestMonster("Swing", 1000)
		m.X, m.Y = g.camera.X+g.config.GetTileSize(), g.camera.Y
		m.AttackRadius = g.config.GetTileSize()
		m.DamageMin, m.DamageMax = 1, 1
		m.AttackCooldownMultiplier = cooldown
		if enrageCooldown > 0 {
			m.Abilities.Enrage = monster.EnrageAbility{AtHP: m.HitPoints, CooldownMult: enrageCooldown}
		}
		m.State, m.StateTimer = monster.StateAttacking, 1
		m.IsEngagingPlayer, m.WasAttacked = true, true
		g.world.Monsters = []*monster.Monster3D{m}
		swings := 0 // the elemental roll runs once per landed melee swing
		cs.elementalAttackRoll = func() float64 { swings++; return 0 }
		return cs, m, &swings
	}
	act := func(t *testing.T, cs *CombatSystem, m *monster.Monster3D, swings *int, cadence monsterAttackCadence) int {
		t.Helper()
		*swings = 0
		cs.commitMonsterAttack(m, monsterAttackDestination{}, cadence)
		return *swings
	}

	for _, row := range []struct {
		name                     string
		cooldown, enrageCooldown float64
		want                     []int
	}{
		{"default speed", 0, 0, []int{1, 1, 1, 1, 1}},
		{"half cooldown", 0.5, 0, []int{2, 2, 2, 2, 2}},
		{"quarter cooldown", 0.25, 0, []int{4, 4, 4, 4, 4}},
		{"0.7 cooldown alternates (1.43)", 0.7, 0, []int{1, 2, 1, 2, 1}},
		{"0.6 cooldown (1.67)", 0.6, 0, []int{2, 1, 2, 2, 1}},
		{"slow monster still swings once", 1.8, 0, []int{1, 1, 1, 1, 1}},
		{"enrage 0.7 on default speed (1.43)", 0, 0.7, []int{1, 2, 1, 2, 1}},
		{"enrage 0.7 on half cooldown (2.86)", 0.5, 0.7, []int{3, 3, 3, 2, 3}},
		{"capped at eight", 0.05, 0, []int{8, 8, 8, 8, 8}},
	} {
		t.Run(row.name, func(t *testing.T) {
			cs, m, swings := setup(t, row.cooldown, row.enrageCooldown)
			var got []int
			for range row.want {
				got = append(got, act(t, cs, m, swings, monsterAttackTurn))
			}
			if !slices.Equal(got, row.want) {
				t.Fatalf("swings per action = %v, want %v", got, row.want)
			}
		})
	}

	t.Run("a revoked attack keeps the banked fraction", func(t *testing.T) {
		cs, m, swings := setup(t, 0.7, 0)
		if got := act(t, cs, m, swings, monsterAttackTurn); got != 1 {
			t.Fatalf("first action = %d swings, want 1", got)
		}
		m.Stun.Turns = 1
		if got := act(t, cs, m, swings, monsterAttackTurn); got != 0 {
			t.Fatalf("stunned action = %d swings, want 0", got)
		}
		m.Stun.Turns = 0
		if got := act(t, cs, m, swings, monsterAttackTurn); got != 2 {
			t.Fatalf("action after the revoked one = %d swings, want 2 (the banked fraction survives)", got)
		}
	})

	t.Run("enrage mid-fight switches the rate", func(t *testing.T) {
		cs, m, swings := setup(t, 0, 0.5)
		m.Abilities.Enrage.AtHP = m.HitPoints - 1 // not yet enraged
		if got := act(t, cs, m, swings, monsterAttackTurn); got != 1 {
			t.Fatalf("healthy action = %d swings, want 1", got)
		}
		m.HitPoints = m.Abilities.Enrage.AtHP
		if got := act(t, cs, m, swings, monsterAttackTurn); got != 2 {
			t.Fatalf("enraged action = %d swings, want 2", got)
		}
	})

	t.Run("real-time attacks neither swing extra nor touch the carry", func(t *testing.T) {
		cs, m, swings := setup(t, 0.5, 0)
		if got := act(t, cs, m, swings, monsterAttackRealtime); got != 1 {
			t.Fatalf("real-time action = %d swings, want 1", got)
		}
		if m.TurnBasedSwingCarry != 0 {
			t.Fatalf("real-time action moved the TB carry to %v", m.TurnBasedSwingCarry)
		}
	})
}

// Turn-based steps per movement action follow the same fractional,
// accumulating rule: speed / monster_ai.turn_step_speed (2.0), at least one
// tile and at most four, ending where the monster can strike. Each row runs
// real monster passes and counts the tiles crossed per pass (pathing is
// four-directional, so one step is one tile of Manhattan distance).
func TestTurnBasedStepsAccumulateThroughMonsterPasses(t *testing.T) {
	setup := func(t *testing.T, speed float64) (*MMGame, *GameLoop, *monster.Monster3D, float64) {
		t.Helper()
		game, gl, ts := tbBehaviorGame(t, 24, 20)
		placePlayerAtTile(game, 17, 10, ts)
		m := spawnMonsterAtTile(game, "goblin", 2, 10, ts)
		m.Speed = speed
		return game, gl, m, ts
	}
	tiles := func(m *monster.Monster3D, ts float64) (int, int) { return TileIndex(m.X, ts), TileIndex(m.Y, ts) }
	moved := func(x0, y0, x1, y1 int) int { return absInt(x1-x0) + absInt(y1-y0) }
	pass := func(game *MMGame, gl *GameLoop, m *monster.Monster3D, ts float64) int {
		x0, y0 := tiles(m, ts)
		runOneMonsterTurn(game, gl)
		x1, y1 := tiles(m, ts)
		return moved(x0, y0, x1, y1)
	}

	for _, row := range []struct {
		name  string
		speed float64
		want  []int
	}{
		{"stationary", 0, []int{0, 0, 0, 0, 0}},
		{"slow walks one tile", 1.0, []int{1, 1, 1, 1, 1}},
		{"step speed walks one tile", 2.0, []int{1, 1, 1, 1, 1}},
		{"2.4 banks an extra tile", 2.4, []int{1, 1, 2, 1, 1}},
		{"3.0 alternates", 3.0, []int{2, 1, 2, 1, 2}},
		{"capped at four, stops at the attack post", 20, []int{4, 4, 4, 2, 0}},
	} {
		t.Run(row.name, func(t *testing.T) {
			game, gl, m, ts := setup(t, row.speed)
			var got []int
			for range row.want {
				got = append(got, pass(game, gl, m, ts))
			}
			if !slices.Equal(got, row.want) {
				t.Fatalf("tiles per pass = %v, want %v", got, row.want)
			}
		})
	}

	t.Run("the overlay previews exactly the steps the pass takes", func(t *testing.T) {
		game, gl, m, ts := setup(t, 3.0)
		for i := 0; i < 3; i++ {
			game.currentTurn = 0
			preview := game.combatAuraMoveSteps(m)
			if got := pass(game, gl, m, ts); got != preview {
				t.Fatalf("pass %d moved %d tiles, overlay previewed %d", i, got, preview)
			}
		}
	})

	t.Run("a blocked action keeps the banked fraction", func(t *testing.T) {
		game, gl, m, ts := setup(t, 3.0)
		x, y := tiles(m, ts)
		walls := [][2]int{{x + 1, y}, {x - 1, y}, {x, y + 1}, {x, y - 1}}
		for _, w := range walls {
			game.world.Tiles[w[1]][w[0]] = world.TileWall
		}
		if got := pass(game, gl, m, ts); got != 0 {
			t.Fatalf("boxed-in pass moved %d tiles", got)
		}
		for _, w := range walls {
			game.world.Tiles[w[1]][w[0]] = world.TileEmpty
		}
		if got := pass(game, gl, m, ts); got != 2 {
			t.Fatalf("first free pass moved %d tiles, want 2 (the blocked pass spent nothing)", got)
		}
	})

	t.Run("a trap on a crossed tile springs and ends the walk", func(t *testing.T) {
		game, gl, m, ts := setup(t, 3.0)
		x, y := tiles(m, ts)
		game.traps = []PlacedTrap{{Key: "bear_trap", MapKey: currentMapKey(), TileX: x + 1, TileY: y, Owner: game.party.Members[0]}}
		if got := pass(game, gl, m, ts); got != 1 {
			t.Fatalf("pass over a bear trap moved %d tiles, want 1 (rooted on the crossed tile)", got)
		}
		if len(game.traps) != 0 || m.Root.Turns == 0 {
			t.Fatalf("crossed trap did not spring: traps=%d root=%d", len(game.traps), m.Root.Turns)
		}
	})

	t.Run("a held attack post is walked through to a free one", func(t *testing.T) {
		game, gl, ts := tbBehaviorGame(t, 24, 20)
		placePlayerAtTile(game, 17, 10, ts)
		holder := spawnMonsterAtTile(game, "goblin", 16, 10, ts)
		holder.Speed = 0
		walker := monster.NewMonster3DFromConfig(0, 0, "goblin", game.config)
		walker.X, walker.Y = TileCenterFromTile(13, 10, ts)
		walker.IsEngagingPlayer, walker.WasAttacked, walker.Speed = true, true, 20
		game.world.Monsters = append(game.world.Monsters, walker)
		game.world.RegisterMonstersWithCollisionSystem(game.collisionSystem)
		game.refreshMonsterAIState()
		if !game.tryClaimMonsterAttackPost(holder) {
			t.Fatal("setup: the holder must claim the post west of the party")
		}
		// In reach on the held post, the walk may not end there.
		walker.X, walker.Y = holder.X, holder.Y
		game.collisionSystem.UpdateEntity(walker.ID, walker.X, walker.Y)
		if gl.monsterCanStrikeTB(walker) {
			t.Fatal("a post another monster holds ended the walk")
		}
		game.releaseMonsterAttackPost(holder)
		if !gl.monsterCanStrikeTB(walker) {
			t.Fatal("positive control: the freed post must end the walk")
		}
		if !game.tryClaimMonsterAttackPost(holder) {
			t.Fatal("setup: re-claim the holder's post")
		}
		walker.X, walker.Y = TileCenterFromTile(13, 10, ts)
		game.collisionSystem.UpdateEntity(walker.ID, walker.X, walker.Y)
		runOneMonsterTurn(game, gl)
		wx, wy := tiles(walker, ts)
		if hx, hy := tiles(holder, ts); wx == hx && wy == hy {
			t.Fatalf("the fast walker stopped on the held post (%d,%d)", wx, wy)
		}
		if _, open := game.monsterAttackPostOpen(walker); !gl.monsterCanStrikeTB(walker) || !open {
			t.Fatalf("the walker ended at (%d,%d), not at a post it can claim", wx, wy)
		}
	})

	t.Run("fleeing uses the same rate and ticks its clock once", func(t *testing.T) {
		game, gl, m, ts := setup(t, 3.0)
		placePlayerAtTile(game, 10, 10, ts)
		m.X, m.Y = TileCenterFromTile(12, 10, ts)
		game.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
		m.State, m.WasAttacked, m.IsEngagingPlayer = monster.StateFleeing, true, false
		game.refreshMonsterAIState()
		if got := pass(game, gl, m, ts); got != 2 {
			t.Fatalf("fleeing pass moved %d tiles, want 2", got)
		}
		if m.StateTimer != game.config.GetTPS() {
			t.Fatalf("flee clock = %d frames after one pass, want %d", m.StateTimer, game.config.GetTPS())
		}
	})
}
