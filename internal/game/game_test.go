package game

import (
	"testing"
)

// Projectile IDs key per-shot state, so no two shots may share one, whether
// they repeat a type or not.
func TestGenerateProjectileID(t *testing.T) {
	g := &MMGame{}
	seen := map[string]bool{}
	for _, kind := range []string{"arrow", "arrow", "fireball", "arrow", "fireball"} {
		id := g.GenerateProjectileID(kind)
		if id == "" || seen[id] {
			t.Fatalf("projectile ID %q is empty or reused (issued: %v)", id, seen)
		}
		seen[id] = true
	}
}
