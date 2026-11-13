package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The shipped pacing keys load, and each one out of range fails the boot with
// its own key in the message.
func TestPacingConfigValidation(t *testing.T) {
	data, err := os.ReadFile("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Engine.ParallelMonsterMinimum < 1 {
		t.Fatal("shipped engine.parallel_monster_minimum did not load")
	}
	if streaming := cfg.Graphics.Streaming; streaming.BackgroundStepsPerTick < 1 || streaming.ReleaseImagesPerTick < 1 {
		t.Fatalf("shipped graphics.streaming did not load: %+v", streaming)
	}
	if n := cfg.Graphics.Streaming.LoadingScreen.WorkerCount(); n < 2 {
		t.Fatalf("loading screen resolved %d workers, want at least the play pair", n)
	}
	if n := (LoadingScreenConfig{Workers: 5}).WorkerCount(); n != 5 {
		t.Fatalf("authored workers resolved to %d, want 5", n)
	}
	for _, tt := range []struct{ key, old, replacement string }{
		{"engine.parallel_monster_minimum", "parallel_monster_minimum: 64", "parallel_monster_minimum: 0"},
		{"streaming.prefetch_lead_tiles", "prefetch_lead_tiles: 24", "prefetch_lead_tiles: -1"},
		{"streaming.background_steps_per_tick", "background_steps_per_tick: 4", "background_steps_per_tick: 0"},
		{"streaming.release_images_per_tick", "release_images_per_tick: 16", "release_images_per_tick: 0"},
		{"loading_screen.workers", "      workers: 0", "      workers: -1"},
		{"loading_screen.queue_megabytes", "queue_megabytes: 256", "queue_megabytes: 16"},
		{"loading_screen.upload_megabytes_per_frame", "upload_megabytes_per_frame: 64", "upload_megabytes_per_frame: 4"},
		{"loading_screen.upload_images_per_frame", "upload_images_per_frame: 512", "upload_images_per_frame: 8"},
	} {
		t.Run(tt.key, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			changed := strings.Replace(string(data), tt.old, tt.replacement, 1)
			if changed == string(data) {
				t.Fatal("fixture setting absent")
			}
			if err := os.WriteFile(path, []byte(changed), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), tt.key) {
				t.Fatalf("error %v, want it to name %s", err, tt.key)
			}
		})
	}
}
