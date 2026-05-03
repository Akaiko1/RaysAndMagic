package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRuntimeCWD(t *testing.T) {
	t.Run("binary directory", func(t *testing.T) {
		execDir := filepath.Join("game", "bin")
		runtimeDir, ok := findRuntimeCWD(execDir, fakeConfigStat(filepath.Join(execDir, "config.yaml")))
		if !ok {
			t.Fatalf("expected runtime directory")
		}
		if runtimeDir != filepath.Clean(execDir) {
			t.Fatalf("expected %s, got %s", filepath.Clean(execDir), runtimeDir)
		}
	})

	t.Run("parent directory", func(t *testing.T) {
		execDir := filepath.Join("game", "bin")
		expected := filepath.Clean(filepath.Join(execDir, ".."))
		runtimeDir, ok := findRuntimeCWD(execDir, fakeConfigStat(filepath.Join(expected, "config.yaml")))
		if !ok {
			t.Fatalf("expected runtime directory")
		}
		if runtimeDir != expected {
			t.Fatalf("expected %s, got %s", expected, runtimeDir)
		}
	})

	t.Run("mac app resources", func(t *testing.T) {
		execDir := filepath.Join("Game.app", "Contents", "MacOS")
		expected := filepath.Clean(filepath.Join(execDir, "..", "Resources"))
		runtimeDir, ok := findRuntimeCWD(execDir, fakeConfigStat(filepath.Join(expected, "config.yaml")))
		if !ok {
			t.Fatalf("expected runtime directory")
		}
		if runtimeDir != expected {
			t.Fatalf("expected %s, got %s", expected, runtimeDir)
		}
	})

	t.Run("missing config", func(t *testing.T) {
		if runtimeDir, ok := findRuntimeCWD(filepath.Join("game", "bin"), fakeConfigStat("")); ok {
			t.Fatalf("expected no runtime directory, got %s", runtimeDir)
		}
	})
}

func fakeConfigStat(existingPath string) func(string) (os.FileInfo, error) {
	return func(path string) (os.FileInfo, error) {
		if filepath.Clean(path) == filepath.Clean(existingPath) {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}
}
