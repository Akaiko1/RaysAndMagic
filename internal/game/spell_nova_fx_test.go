package game

import (
	"math"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/world"
)

// Earthquake used to be invisible: the nova path only painted a small burst on
// each monster it hit, so a 20000-gold spell cast over empty ground showed
// nothing at all. It now authors graphics.nova_fx, which owes the cast a
// MODERATE shake (well under the spell cap) and rubble kicked off the ground
// across its reach - capped so a wide radius cannot spawn a thousand particles.
func TestEarthquakeNovaFx_ShakesAndScattersGround(t *testing.T) {
	game, _, _ := tbBehaviorGame(t, 9, 9)
	t.Chdir("../..") // outdoor_only checks the map's sky variants under assets/
	prev := world.GlobalWorldManager
	world.GlobalWorldManager = &world.WorldManager{
		MapConfigs:    map[string]*config.MapConfig{"arena": {SkyTexture: "arena_panorama"}},
		CurrentMapKey: "arena",
	}
	t.Cleanup(func() { world.GlobalWorldManager = prev })

	equipSpellAndPrepareCaster(t, game.combat, "earthquake", 200, 60)
	game.screenShake = 0
	game.spellHitEffects = nil

	if !game.combat.CastEquippedSpell() {
		t.Fatal("earthquake did not cast")
	}

	if game.screenShake <= 0 {
		t.Error("the ground heaved without shaking the view")
	}
	if game.screenShake > quakeShakeAmp || quakeShakeAmp >= screenShakeMaxAmp {
		t.Errorf("shake %.1f must be moderate: at most %.1f, itself under the %.1f spell cap",
			game.screenShake, quakeShakeAmp, screenShakeMaxAmp)
	}

	if len(game.spellHitEffects) == 0 {
		t.Fatal("no ground FX spawned")
	}
	if len(game.spellHitEffects) > tileScatterMaxTiles {
		t.Errorf("%d ground effects, budget is %d", len(game.spellHitEffects), tileScatterMaxTiles)
	}

	// The rubble must cover the reach, not pile on the party's own tile.
	tile := float64(game.config.GetTileSize())
	var reach float64
	for _, fx := range game.spellHitEffects {
		for _, p := range fx.Particles {
			if d := math.Hypot(p.X-game.camera.X, p.Y-game.camera.Y); d > reach {
				reach = d
			}
		}
	}
	if reach < 4*tile {
		t.Errorf("rubble reaches %.1f tiles, want the spell's radius", reach/tile)
	}
}
