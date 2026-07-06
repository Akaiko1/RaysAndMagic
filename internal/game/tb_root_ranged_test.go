package game

// A root must pin TB movement for both goal strategies: melee closing to an
// adjacent tile and ranged repositioning to a firing lane.

import "testing"

func TestTB_RootPinsRangedAndMeleeMonsters(t *testing.T) {
	cases := []struct {
		name string
		key  string
	}{
		{"ranged-gold-elder", "elder_dragon_gold"},
		{"melee-green-elder", "elder_dragon_green"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, gl := newSpecialsTestGame(t)
			// Off the party's row/column: a ranged attacker has no firing lane
			// here and must WANT to reposition; melee wants to close in.
			m := spawnSpecialsMonster(g, tc.key, 6, 5)
			m.RootTurnsRemaining = 2
			x, y := m.X, m.Y

			for turn := 1; turn <= 2; turn++ {
				runTBMonsterTurns(g, gl, 1)
				if m.X != x || m.Y != y {
					t.Fatalf("rooted %s moved on turn %d: (%.0f,%.0f) -> (%.0f,%.0f)",
						tc.key, turn, x, y, m.X, m.Y)
				}
			}

			runTBMonsterTurns(g, gl, 1) // root expired: free to act again
			if m.X == x && m.Y == y {
				t.Fatalf("%s must move again once the root expires", tc.key)
			}
		})
	}
}
