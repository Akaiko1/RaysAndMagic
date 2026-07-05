package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRuntimeCWD(t *testing.T) {
	tests := []struct {
		name         string
		execDir      string
		existingPath string
		want         string
		wantOK       bool
	}{
		{
			name:         "binary directory",
			execDir:      filepath.Join("game", "bin"),
			existingPath: filepath.Join("game", "bin", "config.yaml"),
			want:         filepath.Join("game", "bin"),
			wantOK:       true,
		},
		{
			name:         "parent directory",
			execDir:      filepath.Join("game", "bin"),
			existingPath: filepath.Join("game", "config.yaml"),
			want:         "game",
			wantOK:       true,
		},
		{
			name:         "mac app resources",
			execDir:      filepath.Join("Game.app", "Contents", "MacOS"),
			existingPath: filepath.Join("Game.app", "Contents", "Resources", "config.yaml"),
			want:         filepath.Join("Game.app", "Contents", "Resources"),
			wantOK:       true,
		},
		{
			name:    "missing config",
			execDir: filepath.Join("game", "bin"),
			wantOK:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := findRuntimeCWD(tc.execDir, fakeConfigStat(tc.existingPath))
			if ok != tc.wantOK {
				t.Fatalf("expected ok=%v, got %v", tc.wantOK, ok)
			}
			if filepath.Clean(got) != filepath.Clean(tc.want) {
				t.Fatalf("expected %s, got %s", filepath.Clean(tc.want), filepath.Clean(got))
			}
		})
	}
}

func fakeConfigStat(existingPath string) func(string) (os.FileInfo, error) {
	return func(path string) (os.FileInfo, error) {
		if existingPath != "" && filepath.Clean(path) == filepath.Clean(existingPath) {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}
}
