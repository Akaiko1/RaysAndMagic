package game

import (
	"sort"
	"testing"
	"ugataima/internal/monster"
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
	for _, k := range []FxKind{FxSpell, FxWeapon, FxTrap, FxCard, FxStatus} {
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
		steps := fxRespawnTicks + 5
		if it.Kind == FxTrap {
			steps = 3 * cfg.GetTPS()
		}
		for i := 0; i < steps; i++ {
			p.Step()
		}
		if it.Kind == FxTrap && (len(p.g.traps) != 1 || len(p.g.trapBursts) != 0) {
			t.Fatal("trap preview did not complete activation and re-arm exactly once")
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
			keys := allTileKeys(tm)
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

// Every status motif is listed for each actor its catalog entry stages, is
// derived from real state on that actor, survives the respawn loop without
// piling up actors, and leaves nothing behind for the next exhibit.
func TestFxPreview_StatusExhibitsStageRealState(t *testing.T) {
	cfg := setupPreviewSandboxTest(t)
	p, err := NewFxPreview(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.g.Shutdown()
	var spell FxItem
	listed := map[string]FxItem{}
	for _, it := range p.Items() {
		switch it.Kind {
		case FxStatus:
			listed[it.Key] = it
		case FxSpell:
			if spell.Key == "" {
				spell = it
			}
		}
	}
	for _, e := range statusVisualCatalog {
		for _, onHero := range []bool{false, true} {
			if (onHero && e.hero == nil) || (!onHero && e.monster == nil) {
				continue
			}
			key := "monster_" + e.key
			if onHero {
				key = "hero_" + e.key
			}
			t.Run(key, func(t *testing.T) {
				it, ok := listed[key]
				if !ok || it.status != e.flag || it.onHero != onHero {
					t.Fatalf("catalog entry missing or mislabeled: %+v", it)
				}
				hero := p.g.party.Members[fxHeroIdx]
				check := func(stage string) {
					t.Helper()
					if onHero {
						if len(p.g.world.Monsters) != 0 || p.g.partyStatusVisuals(hero) != e.flag {
							t.Fatalf("%s: hero visual=%v, stage monsters=%d", stage, p.g.partyStatusVisuals(hero), len(p.g.world.Monsters))
						}
						return
					}
					if len(p.g.world.Monsters) != 1 || monsterStatusVisuals(p.g.world.Monsters[0], false) != e.flag ||
						p.g.partyStatusVisuals(hero) != 0 {
						t.Fatalf("%s: want one stage monster showing %v", stage, e.flag)
					}
				}
				p.Select(it)
				check("select")
				var staged *monster.Monster3D
				if !onHero {
					staged = p.g.world.Monsters[0]
				}
				for i := 0; i < fxRespawnTicks+5; i++ {
					p.Step()
				}
				check("respawn")
				// Outlive the staged state: loop-ticked timers (Charm, Bind, party
				// root) must be refreshed, never left to expire mid-selection.
				for i := 0; i < statusStageFrames+2*fxRespawnTicks; i++ {
					p.Step()
				}
				check("past the staged duration")
				// Re-staging would reseed the actor's animation phase mid-loop.
				if !onHero && p.g.world.Monsters[0] != staged {
					t.Fatal("respawn replaced the staged monster")
				}
				p.Select(spell)
				if len(p.g.world.Monsters) != 0 || p.g.partyStatusVisuals(hero) != 0 || len(hero.Conditions) != 0 {
					t.Fatal("status exhibit leaked into the next selection")
				}
			})
		}
	}
}

// Replaying while contact debris is alive hides the real attack's timing.
func TestFxPreview_AttackCompletesBeforeReplay(t *testing.T) {
	cfg := setupPreviewSandboxTest(t)
	p, err := NewFxPreview(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.g.Shutdown()
	for _, tc := range []FxItem{
		{Kind: FxSpell, Key: "fireball"},
		{Kind: FxWeapon, Key: "iron_sword"},
	} {
		t.Run(tc.Key, func(t *testing.T) {
			p.Select(tc)
			seenImpact := false
			for step := 1; step <= 600; step++ {
				p.Step()
				for _, hit := range p.g.spellHitEffects {
					if hit.Active {
						seenImpact = true
					}
				}
				if p.tick == 0 {
					if tc.Kind == FxSpell && (!seenImpact || len(p.g.spellHitEffects) != 0) {
						t.Fatal("spell replayed before its impact finished")
					}
					if tc.Kind == FxWeapon && len(p.g.slashEffects) != 1 {
						t.Fatal("weapon preview overlapped consecutive swings")
					}
					return
				}
			}
			t.Fatal("completed attack never replayed")
		})
	}
	p.Select(FxItem{Kind: FxSpell, Key: "hot_steam"})
	for i := 0; i < fxRespawnTicks*3; i++ {
		p.Step()
	}
	if len(p.g.persistentDamageZones) == 0 {
		t.Fatal("field preview has no active field")
	}
	field := p.g.persistentDamageZones[0].FieldID
	for _, zone := range p.g.persistentDamageZones {
		if zone.FieldID != field {
			t.Fatal("field preview stacked a new cast over its active field")
		}
	}
}
