package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestOpenWorldConfigSaveRoundTrip guards the editor's save path: marshaling
// the loaded config and re-loading it must reproduce the exact same rules
// (placements with orient, connections with depth defaults, removals).
func TestOpenWorldConfigSaveRoundTrip(t *testing.T) {
	owc, err := LoadOpenWorldConfig(filepath.Join("..", "..", "assets", "open_world.yaml"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	data, err := yaml.Marshal(owc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	tmp := filepath.Join(t.TempDir(), "open_world.yaml")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	reloaded, err := LoadOpenWorldConfig(tmp)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reflect.DeepEqual(owc, reloaded) {
		t.Fatalf("round-trip drift:\n loaded: %+v\n reloaded: %+v", owc, reloaded)
	}
}
