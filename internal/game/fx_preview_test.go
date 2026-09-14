package game

import (
	"sort"
	"testing"
	"ugataima/internal/world"
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

// A nova spell's ground FX (Earthquake) has to reach the editor's FX tab like
// any other effect: listed in the catalog, and actually painting something on
// the stage - the sandbox has no open sky, so the real cast refunds itself and
// the preview must fall back to playing the effect directly.
func TestFxPreview_NovaSpellPreviews(t *testing.T) {
	cfg := setupPreviewSandboxTest(t)
	p, err := NewFxPreview(cfg)
	if err != nil {
		t.Fatalf("NewFxPreview: %v", err)
	}

	var quake FxItem
	for _, it := range p.Items() {
		if it.Kind == FxSpell && it.Key == "earthquake" {
			quake = it
			break
		}
	}
	if quake.Key == "" {
		t.Fatal("earthquake is missing from the FX catalog")
	}
	p.Select(quake)
	if len(p.g.spellHitEffects) == 0 {
		t.Error("earthquake preview painted no ground FX")
	}
}

// Every catalog kind shares the editor sandbox clock, just as gameplay shares
// one presentation clock across all effects and panels.
func TestFxPreview_PresentationClockAdvancesOncePerStep(t *testing.T) {
	cfg := setupPreviewSandboxTest(t)
	p, err := NewFxPreview(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.g.Shutdown()
	seen := map[FxKind]bool{}
	for _, it := range p.Items() {
		if it.Kind != FxCard && seen[it.Kind] {
			continue
		}
		seen[it.Kind] = true
		t.Run(it.Key, func(t *testing.T) {
			p.Select(it)
			for _, steps := range []int{1, fxRespawnTicks, 1} {
				before := p.g.gameLoop.ui.cardAnimClock()
				for i := 0; i < steps; i++ {
					p.Step()
				}
				if got := p.g.gameLoop.ui.cardAnimClock() - before; got != int64(steps) {
					t.Fatalf("presentation advanced %d ticks, want %d", got, steps)
				}
			}
		})
	}
}

func TestFxPreview_AuraExhibitUsesEligibleAuthoredTile(t *testing.T) {
	for _, mode := range []string{"shipped", "alternate", "none"} {
		t.Run(mode, func(t *testing.T) {
			cfg := setupPreviewSandboxTest(t)
			previous := world.GlobalTileManager
			tm := world.NewTileManager(testTileSizeClasses())
			if err := tm.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
				t.Fatal(err)
			}
			world.GlobalTileManager = tm
			t.Cleanup(func() { world.GlobalTileManager = previous })
			keys := tm.GetAllTileKeys()
			sort.Strings(keys)
			var eligible []string
			for _, key := range keys {
				td := tm.GetTileDataByKey(key)
				tt, _ := tm.GetTileTypeFromKey(key)
				if tileShowsImpassableAura(td) && !tm.IsWalkable(tt) {
					eligible = append(eligible, key)
				}
			}
			if len(eligible) < 2 {
				t.Fatal("fixture needs multiple authored aura tiles")
			}
			if mode == "alternate" {
				tm.GetTileDataByKey(eligible[0]).ImpassableAura = false
				eligible = eligible[1:]
			}
			if mode == "none" {
				for _, key := range eligible {
					tm.GetTileDataByKey(key).ImpassableAura = false
				}
				eligible = nil
			}
			p, err := NewFxPreview(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer p.g.Shutdown()
			found := false
			for _, it := range p.Items() {
				if it.Kind != FxTile || it.Label != "Impassable aura" {
					continue
				}
				found = true
				p.g.flyActive, p.g.walkOnWaterActive, p.g.waterBreathingActive = true, true, true
				p.g.flyDuration, p.g.walkOnWaterDuration, p.g.waterBreathingDuration = 1000, 1000, 1000
				p.Step()
				p.Select(it)
				if p.g.flyActive || p.g.walkOnWaterActive || p.g.waterBreathingActive || !p.arena.IsTileBlocking(1, 4) {
					t.Error("previous utility effect hid the selected aura")
				}
				if len(eligible) == 0 || it.Key != eligible[0] {
					t.Errorf("aura exhibit %s does not match eligible content %v", it.Key, eligible)
				}
			}
			if found != (len(eligible) > 0) {
				t.Fatalf("aura exhibit present=%v for %v", found, eligible)
			}
		})
	}
}
