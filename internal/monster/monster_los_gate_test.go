package monster

import "testing"

// losGateChecker wraps the standard mock with a switchable line-of-sight
// result so detection tests can put a "wall" between monster and party.
type losGateChecker struct {
	*MockCollisionChecker
	los bool
}

func (c *losGateChecker) CheckLineOfSight(x1, y1, x2, y2 float64) bool { return c.los }

// TestPlayerDetectionRange_LOSGate pins the no-aggro-through-walls rule:
// an UNAWARE monster detects nothing without line of sight (radius 0), while
// an already-engaged one keeps its full radius so pursuit survives corners.
func TestPlayerDetectionRange_LOSGate(t *testing.T) {
	newMob := func() *Monster3D {
		sx, sy := tileToWorldCenter(2, 2)
		return &Monster3D{X: sx, Y: sy, SpawnX: sx, SpawnY: sy}
	}
	// Player two tiles away: well inside the 4-tile fallback radius.
	px, py := tileToWorldCenter(4, 2)

	t.Run("unaware+blocked: no detection", func(t *testing.T) {
		m := newMob()
		checker := &losGateChecker{NewMockCollisionChecker(defaultTileSize), false}
		if r, _ := m.PlayerDetectionRange(checker, px, py); r != 0 {
			t.Fatalf("blocked LOS must fully gate onset, got radius %.1f", r)
		}
	})
	t.Run("unaware+clear: full radius", func(t *testing.T) {
		m := newMob()
		checker := &losGateChecker{NewMockCollisionChecker(defaultTileSize), true}
		if r, _ := m.PlayerDetectionRange(checker, px, py); r <= 0 {
			t.Fatalf("clear LOS must detect, got radius %.1f", r)
		}
	})
	t.Run("engaged+blocked: pursuit keeps radius", func(t *testing.T) {
		m := newMob()
		m.IsEngagingPlayer = true
		checker := &losGateChecker{NewMockCollisionChecker(defaultTileSize), false}
		if r, _ := m.PlayerDetectionRange(checker, px, py); r <= 0 {
			t.Fatalf("engaged monster must keep its radius behind cover, got %.1f", r)
		}
	})
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
	t.Run("loot guard raises only the minimum radius", func(t *testing.T) {
		m := newMob()
		m.AlertRadius = 2 * defaultTileSize
		m.LootGuarding = true
		guardPX, guardPY := tileToWorldCenter(8, 2) // six tiles away, inside guard minimum seven
		checker := &losGateChecker{NewMockCollisionChecker(defaultTileSize), true}
		if !m.CanStartPlayerEngagement(checker, guardPX, guardPY) {
			t.Fatal("loot guard did not use its seven-tile direct-sight exception")
		}
	})
}
