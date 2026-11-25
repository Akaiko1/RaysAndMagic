package world

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A wrong wall_torch_style stops the map configs from loading, and the error
// names the file, the map and the key.
func TestMapConfigsRejectUnknownWallTorchStyle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "map_configs.yaml")
	if err := os.WriteFile(path, []byte("maps:\n  keep:\n    wall_torches: true\n    wall_torch_style: brazier\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := NewWorldManager(nil).LoadMapConfigs(path)
	for _, want := range []string{path, `map "keep"`, "wall_torch_style"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("error %v, want it to name %s", err, want)
		}
	}
}
