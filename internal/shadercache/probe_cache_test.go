package shadercache

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShaderAdmissionCache(t *testing.T) {
	good := shaderArtifact{SourceID: "aaaaaaaaaaaaaaaaaaaaaaaaaa", Source: "package main\nfunc Fragment(dst vec4, src vec2, c vec4) vec4 {return c}"}
	for _, change := range []string{"unchanged", "archive", "binary_same_stat", "executable", "backend", "corrupt_marker", "failed_probe", "unwritable"} {
		t.Run(change, func(t *testing.T) {
			dir := t.TempDir()
			executable := filepath.Join(dir, "game")
			cache := filepath.Join(dir, "cache")
			if err := os.WriteFile(executable, []byte("binary-v1"), 0600); err != nil {
				t.Fatal(err)
			}
			stamp := time.Unix(10000, 0)
			if err := os.Chtimes(executable, stamp, stamp); err != nil {
				t.Fatal(err)
			}
			data := archiveForTest(t, []shaderArtifact{good})
			if change == "unwritable" {
				if err := os.WriteFile(cache, []byte("file"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			probes, publishes := 0, 0
			admit := func() error {
				return initializeArchive(data, func() error {
					probes++
					if change == "failed_probe" && probes == 1 {
						return errors.New("GPU rejected archive")
					}
					return nil
				}, func([]shaderArtifact) { publishes++ }, shaderProbeCache{executable: executable, dir: cache})
			}
			err := admit()
			if (err != nil) != (change == "failed_probe") {
				t.Fatalf("first admission: %v", err)
			}
			if probes != 1 || publishes != map[bool]int{false: 1, true: 0}[change == "failed_probe"] {
				t.Fatalf("cold probes=%d publishes=%d", probes, publishes)
			}
			switch change {
			case "archive":
				edited := good
				edited.Source += "\n"
				data = archiveForTest(t, []shaderArtifact{edited})
			case "binary_same_stat":
				if err := os.WriteFile(executable, []byte("binary-v2"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(executable, stamp, stamp); err != nil {
					t.Fatal(err)
				}
			case "executable":
				executable = filepath.Join(dir, "editor")
				if err := os.WriteFile(executable, []byte("binary-v1"), 0600); err != nil {
					t.Fatal(err)
				}
			case "backend":
				t.Setenv("EBITENGINE_GRAPHICS_LIBRARY", "opengl")
			case "corrupt_marker":
				paths, err := filepath.Glob(filepath.Join(cache, "*.ok"))
				if err != nil || len(paths) != 1 {
					t.Fatalf("markers: %v %v", paths, err)
				}
				if err := os.WriteFile(paths[0], []byte("partial"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := admit(); err != nil {
				t.Fatal(err)
			}
			want := 2
			if change == "unchanged" {
				want = 1
			}
			if probes != want {
				t.Fatalf("restart probes=%d want %d", probes, want)
			}
			if err := admit(); err != nil {
				t.Fatal(err)
			}
			if change == "unwritable" {
				want++
			}
			if probes != want {
				t.Fatalf("accepted restart re-probed: %d want %d", probes, want)
			}
			if publishes != map[bool]int{false: 3, true: 2}[change == "failed_probe"] {
				t.Fatalf("successful cache hit skipped publication: %d", publishes)
			}
		})
	}
}
