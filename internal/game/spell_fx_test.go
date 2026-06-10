package game

import (
	"testing"

	"ugataima/internal/character"
)

// TestEveryMagicSchoolHasImpactColor: every magic school must have an explosion
// colour, so a spell hit never falls back to the gray "physical" default.
func TestEveryMagicSchoolHasImpactColor(t *testing.T) {
	for _, school := range character.AllMagicSchools {
		if _, ok := ElementColors[string(school)]; !ok {
			t.Errorf("school %q has no ElementColors entry (spell hit would render gray)", school)
		}
	}
}

// TestPerspectiveScale_NeverInflates: the collision scale must not balloon near
// the camera (the spawn-frame bug where a fireball hit/exploded several tiles
// away before being drawn). It clamps to 1 up close and shrinks far away.
func TestPerspectiveScale_NeverInflates(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	cs.game.camera.X, cs.game.camera.Y = 0, 0
	ts := float64(cs.game.config.GetTileSize())

	if near := cs.calculatePerspectiveScale(1, 1, 28, 4, 110); near > 1.0 {
		t.Errorf("point-blank scale = %v, want <= 1 (no inflation)", near)
	}
	if far := cs.calculatePerspectiveScale(ts*8, 0, 28, 4, 110); far >= 1.0 {
		t.Errorf("far scale = %v, want < 1 (shrinks with distance)", far)
	}
}

// TestFireboltParticleSize_ShrinksWithDistance: a firebolt's explosion particles
// must get smaller with distance (not pin to the max cap at every range, which
// read as "always point-blank"). Uses firebolt's real spawn size + the camera's
// real fov/screen height.
func TestFireboltParticleSize_ShrinksWithDistance(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)

	// Firebolt's actual per-particle base size (damage/radius-derived).
	cs.game.CreateSpellHitEffectFromSpell(0, 0, "firebolt")
	if len(cs.game.spellHitEffects) == 0 || len(cs.game.spellHitEffects[0].Particles) == 0 {
		t.Fatal("firebolt produced no hit particles")
	}
	ps := cs.game.spellHitEffects[0].Particles[0].Size

	fov := cs.game.camera.FOV
	sh := float64(cs.game.config.GetScreenHeight())
	ts := float64(cs.game.config.GetTileSize())
	scaleAt := func(tiles float64) float64 { return sh / (tiles * ts * fov) }

	near := spellParticleScreenSize(ps, 1.0, scaleAt(2))
	mid := spellParticleScreenSize(ps, 1.0, scaleAt(6))
	far := spellParticleScreenSize(ps, 1.0, scaleAt(12))

	if !(near > mid && mid > far) {
		t.Errorf("firebolt particle size should shrink with distance: near=%.1f mid=%.1f far=%.1f", near, mid, far)
	}
	if mid >= spellParticleMaxSize {
		t.Errorf("mid-range particles hit the max cap (%.0f) — reads as uniform/point-blank: mid=%.1f", spellParticleMaxSize, mid)
	}
	if far > near*0.5 {
		t.Errorf("far particles should be well under half the near size: near=%.1f far=%.1f", near, far)
	}
}

// TestSpellHitStyle: impact style is keyed by element/school so it generalizes
// beyond the named spells.
func TestSpellHitStyle(t *testing.T) {
	cases := map[string]string{
		"fire":     "ember",
		"water":    "shard",
		"dark":     "void",
		"light":    "flash",
		"air":      "static",
		"earth":    "rubble",
		"mind":     "spiral",
		"spirit":   "soul",
		"body":     "mend",
		"physical": "burst",
		"":         "burst",
	}
	for el, want := range cases {
		if got := spellHitStyle(el); got != want {
			t.Errorf("spellHitStyle(%q) = %q, want %q", el, got, want)
		}
	}
}

// TestCreateSpellHitEffect_StyleMotion: gravity signs encode each school's
// motion — fire embers and spirit wisps rise, ice shards and earth rubble fall,
// a plain physical burst has none. All start at the anchor with zero offset.
func TestCreateSpellHitEffect_StyleMotion(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))

	check := func(element string, wantSign int) {
		g.spellHitEffects = nil
		g.CreateSpellHitEffect(100, 100, element, 12, 4)
		if len(g.spellHitEffects) != 1 || len(g.spellHitEffects[0].Particles) == 0 {
			t.Fatalf("%s: expected one effect with particles", element)
		}
		for _, p := range g.spellHitEffects[0].Particles {
			if p.OffsetX != 0 || p.OffsetY != 0 {
				t.Errorf("%s: particle should start at anchor, got offset (%v,%v)", element, p.OffsetX, p.OffsetY)
			}
			switch {
			case wantSign < 0 && p.Gravity >= 0:
				t.Errorf("%s: particles should rise (gravity<0), got %v", element, p.Gravity)
			case wantSign > 0 && p.Gravity <= 0:
				t.Errorf("%s: particles should fall (gravity>0), got %v", element, p.Gravity)
			case wantSign == 0 && p.Gravity != 0:
				t.Errorf("%s: burst should have no gravity, got %v", element, p.Gravity)
			}
		}
	}
	check("fire", -1)
	check("water", 1)
	check("earth", 1)
	check("spirit", -1)
	check("physical", 0)
}

// Spell impacts now flash the world: CreateSpellHitEffect must leave a decaying
// impact light at the burst point, and heavy spells must rattle the screen.
func TestCreateSpellHitEffect_ImpactLightAndShake(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))

	g.CreateSpellHitEffect(100, 200, "fire", 20, 4)
	if len(g.impactLights) != 1 {
		t.Fatalf("expected one impact light, got %d", len(g.impactLights))
	}
	il := g.impactLights[0]
	if il.X != 100 || il.Y != 200 || il.Radius <= 0 || il.Intensity <= 0 || il.Life != il.MaxLife {
		t.Errorf("impact light misconfigured: %+v", il)
	}

	g.screenShake = 0
	g.CreateSpellHitEffectFromSpell(100, 200, "fireball")
	if g.screenShake <= 0 {
		t.Errorf("a fireball impact should set screen shake, got %v", g.screenShake)
	}
	if g.screenShake > screenShakeMaxAmp {
		t.Errorf("shake must be capped at %v, got %v", screenShakeMaxAmp, g.screenShake)
	}
}
