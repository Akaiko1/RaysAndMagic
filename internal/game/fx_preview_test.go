package game

import (
	"testing"
)

// TestFxPreview_CatalogAndSpawnCycle smoke-tests the editor FX sandbox: it
// must build against loaded game data, enumerate a data-driven catalog, and
// survive select+step cycles for every kind without panicking.
func TestFxPreview_CatalogAndSpawnCycle(t *testing.T) {
	cfg := setupPreviewSandboxTest(t)

	p, err := NewFxPreview(cfg)
	if err != nil {
		t.Fatalf("NewFxPreview: %v", err)
	}

	items := p.Items()
	if len(items) == 0 {
		t.Fatal("FX catalog is empty")
	}
	kinds := map[FxKind]int{}
	for _, it := range items {
		kinds[it.Kind]++
	}
	for _, k := range []FxKind{FxSpell, FxWeapon, FxTrap, FxCard} {
		if kinds[k] == 0 {
			t.Errorf("FX catalog has no entries of kind %d", k)
		}
	}

	// One representative per kind: select, run a full respawn cycle of steps.
	seen := map[FxKind]bool{}
	for _, it := range items {
		if seen[it.Kind] {
			continue
		}
		seen[it.Kind] = true
		p.Select(it)
		for i := 0; i < fxRespawnTicks+5; i++ {
			p.Step()
		}
	}

	// A projectile spell must actually put a projectile or a hit burst in play.
	for _, it := range items {
		if it.Kind == FxSpell && it.Key == "fireball" {
			p.Select(it)
			if len(p.g.magicProjectiles) == 0 && len(p.g.spellHitEffects) == 0 {
				t.Errorf("fireball preview spawned neither projectile nor hit effect")
			}
			break
		}
	}
}
