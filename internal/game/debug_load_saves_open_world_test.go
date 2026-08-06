//go:build debug

package game

// Headless load check for REAL save files against the unified open world - a
// DEBUG MODULE for verifying that every existing save (any map key, any
// pre-open-world format) still loads with world.open_world enabled.
//
// Run with:
//
//	RAM_DEBUG_SIM=1 go test -tags debug ./internal/game/ -run TestDebugSim_LoadAllSavesOpenWorld -v
import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/world"
)

func TestDebugSim_LoadAllSavesOpenWorld(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	t.Chdir("../..")

	// Enumerate the slot files by the game's own naming instead of globbing the
	// directory: highscores, the stash and arena_leaderboard.json live there too,
	// and a blacklist breaks again on the next sibling artifact.
	var saves []string
	for row := 0; row < SaveRowsTotal(); row++ {
		// Slot NAME from the one source of truth, directory hardcoded on purpose:
		// this checks the saves the BUILT game wrote next to its exe in bin/, and
		// AppSaveDir would resolve somewhere else under `go test` (and create it).
		path := filepath.Join("bin", "saves", SaveRowFileName(row))
		if _, err := os.Stat(path); err != nil {
			continue
		}
		saves = append(saves, path)
	}
	if len(saves) == 0 {
		t.Skip("no save files under bin/saves")
	}

	g, wm, cfg := bootOpenWorldGame(t, true)
	ts := cfg.GetTileSize()

	for _, path := range saves {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: read: %v", path, err)
			continue
		}
		var save GameSave
		if err := json.Unmarshal(raw, &save); err != nil {
			t.Errorf("%s: parse: %v", path, err)
			continue
		}
		if err := g.applySave(wm, &save); err != nil {
			t.Errorf("%s (map %q): applySave: %v", path, save.MapKey, err)
			continue
		}
		if wm.CurrentMapKey != save.MapKey {
			t.Errorf("%s: landed on %q, want %q", path, wm.CurrentMapKey, save.MapKey)
			continue
		}
		// The party must stand on ground the world considers reachable.
		tx, ty := int(g.camera.X/ts), int(g.camera.Y/ts)
		if g.world == nil || g.world.IsTileBlockingTerrainAt(tx, ty) {
			t.Errorf("%s (map %q): party restored into blocking tile (%d,%d)", path, save.MapKey, tx, ty)
			continue
		}
		t.Logf("%s: map=%s pos=(%.0f,%.0f) monsters=%d OK", path, save.MapKey, g.camera.X, g.camera.Y, len(g.world.Monsters))
	}
	_ = world.GlobalWorldManager
}
