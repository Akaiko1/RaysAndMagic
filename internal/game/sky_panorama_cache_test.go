package game

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/world"
)

// A day/night flip must not pay a mid-frame PNG decode: boot prepares the
// current map's pair, and re-selecting a texture reuses the same image.
func TestSkyPanoramaCacheServesFlipsWithoutDecode(t *testing.T) {
	previousWorldManager := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = previousWorldManager })
	world.GlobalWorldManager = &world.WorldManager{
		CurrentMapKey: "forest",
		MapConfigs: map[string]*config.MapConfig{
			"forest": {SkyTexture: "forest_panorama"},
		},
	}
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
	t.Chdir("../..") // panorama paths are repo-root-relative, like the real game's cwd

	g.prewarmSkyPanoramas()
	if len(g.skyPanoramaCache) == 0 {
		t.Fatal("boot prewarm cached no panoramas")
	}
	if _, ok := g.skyPanoramaCache["forest_panorama_night"]; !ok {
		t.Fatal("boot prewarm missed the forest night panorama")
	}
	if _, ok := g.skyPanoramaCache["desert_panorama_day"]; ok {
		t.Fatal("boot prewarm decoded an unrelated map panorama")
	}

	// Selecting a phase must serve the cached instance: same pointer each time.
	g.updateSkyPanorama("forest_panorama_night")
	first := g.skyPanorama
	if first == nil {
		t.Fatal("night panorama did not load")
	}
	if first != g.skyPanoramaCache["forest_panorama_night"] {
		t.Fatal("selected panorama is not the cached instance")
	}
	g.updateSkyPanorama("forest_panorama_day")
	g.updateSkyPanorama("forest_panorama_night")
	if g.skyPanorama != first {
		t.Fatal("re-selecting a panorama decoded a new image instead of reusing the cache")
	}
}
