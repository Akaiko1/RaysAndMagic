package monster

import "testing"

// losGateChecker wraps the standard mock with a switchable line-of-sight
// result so detection tests can put a "wall" between monster and party.
type losGateChecker struct {
	*MockCollisionChecker
	los bool
}

func (c *losGateChecker) CheckLineOfSight(x1, y1, x2, y2 float64) bool { return c.los }

// TestPlayerEngagement_LOSGate pins the no-aggro-through-walls rule on the RT
// engagement update: an UNAWARE monster never engages without line of sight,
// while an already-engaged one keeps its fight behind cover (distance-only leash).
func TestPlayerEngagement_LOSGate(t *testing.T) {
	// Player two tiles away: well inside the 4-tile fallback radius.
	px, py := tileToWorldCenter(4, 2)
	for _, tc := range []struct {
		name         string
		engaged, los bool
		wantEngaged  bool
	}{
		{"unaware+blocked: no detection", false, false, false},
		{"unaware+clear: detects", false, true, true},
		{"engaged+blocked: pursuit survives cover", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sx, sy := tileToWorldCenter(2, 2)
			m := &Monster3D{X: sx, Y: sy, SpawnX: sx, SpawnY: sy, HitPoints: 1}
			m.IsEngagingPlayer = tc.engaged
			checker := &losGateChecker{NewMockCollisionChecker(defaultTileSize), tc.los}
			m.UpdatePlayerEngagement(checker, px, py, px, py)
			if m.IsEngagingPlayer != tc.wantEngaged {
				t.Fatalf("engaging = %v, want %v", m.IsEngagingPlayer, tc.wantEngaged)
			}
		})
	}
}

// TestCanStartPlayerEngagement pins the public first-sight API used by both
// real-time and turn-based scheduling. It must remain the one place where
// normal alert radius and direct LoS combine.
func TestCanStartPlayerEngagement(t *testing.T) {
	newMob := func() *Monster3D {
		sx, sy := tileToWorldCenter(2, 2)
		return &Monster3D{
			X: sx, Y: sy, SpawnX: sx, SpawnY: sy,
			HitPoints: 1, AlertRadius: 3 * defaultTileSize,
		}
	}
	px, py := tileToWorldCenter(5, 2) // exactly three tiles away

	t.Run("clear sight inside authored radius", func(t *testing.T) {
		m := newMob()
		checker := &losGateChecker{NewMockCollisionChecker(defaultTileSize), true}
		if !m.CanStartPlayerEngagement(checker, px, py) {
			t.Fatal("clear sight inside authored alert radius did not start engagement")
		}
	})
	t.Run("blocked sight", func(t *testing.T) {
		m := newMob()
		checker := &losGateChecker{NewMockCollisionChecker(defaultTileSize), false}
		if m.CanStartPlayerEngagement(checker, px, py) {
			t.Fatal("blocked sight started engagement")
		}
	})
	t.Run("already engaged is not a new sight event", func(t *testing.T) {
		m := newMob()
		m.IsEngagingPlayer = true
		checker := &losGateChecker{NewMockCollisionChecker(defaultTileSize), true}
		if m.CanStartPlayerEngagement(checker, px, py) {
			t.Fatal("an existing engagement was treated as first sight")
		}
	})
	t.Run("loot guard uses exactly seven tiles", func(t *testing.T) {
		m := newMob()
		m.AlertRadius = 12 * defaultTileSize
		m.LootGuarding = true
		checker := &losGateChecker{NewMockCollisionChecker(defaultTileSize), true}
		guardPX, guardPY := tileToWorldCenter(8, 2) // six tiles away, inside seven
		if !m.CanStartPlayerEngagement(checker, guardPX, guardPY) {
			t.Fatal("loot guard did not use its seven-tile direct-sight range")
		}
		if leash := m.PursuitLeashPixels(); leash != LootGuardAggroRadiusTiles*defaultTileSize {
			t.Fatalf("loot guard leash = %.1f, want %.1f", leash, LootGuardAggroRadiusTiles*defaultTileSize)
		}
		outsidePX, outsidePY := tileToWorldCenter(10, 2) // eight tiles away
		if m.CanStartPlayerEngagement(checker, outsidePX, outsidePY) {
			t.Fatal("loot guard used its ordinary authored radius beyond seven tiles")
		}
	})
}
