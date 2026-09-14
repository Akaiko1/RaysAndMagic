package monster

import (
	"strings"
	"testing"
)

// The generator's whole contract is stateless uniqueness: no persisted counter,
// so a fresh process can never re-issue an ID adopted from a save. A constant
// or unseeded generator would resurrect the ID-twin sweep bug (kills and pack
// despawns remove monsters BY ID and take every twin with them).
func TestMonsterIDGeneratorMintsDistinctIDs(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id := generateUniqueMonsterID()
		if !strings.HasPrefix(id, "monster_") || len(id) <= len("monster_") {
			t.Fatalf("malformed monster ID %q", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("generator re-issued ID %q after %d draws", id, i)
		}
		seen[id] = struct{}{}
	}
}

// The entropy-failure fallback must hold the same uniqueness contract on its
// own: a coarse clock hands one UnixNano to a whole spawn loop, so the stamp
// alone would mint ID-twins - the exact bug class the generator exists to
// prevent.
func TestFallbackMonsterIDsDistinctWithinOneClockTick(t *testing.T) {
	const stamp = int64(1234567890)
	a := fallbackMonsterID(stamp)
	b := fallbackMonsterID(stamp)
	if !strings.HasPrefix(a, "monster_t") || !strings.HasPrefix(b, "monster_t") {
		t.Fatalf("malformed fallback IDs %q, %q", a, b)
	}
	if a == b {
		t.Fatalf("same-instant fallback IDs collide: %q", a)
	}
	if c := fallbackMonsterID(stamp + 1); c == a || c == b {
		t.Fatalf("cross-instant fallback ID collides: %q", c)
	}
}
