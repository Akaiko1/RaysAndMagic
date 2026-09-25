package test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Run the real build entry points against small assets. Compilation and signing
// are stand-ins here; copying, filtering and output layouts are production code.
func TestBuildRuntimePackaging(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required for the macOS build scripts")
	}
	for _, tc := range []struct {
		script string
		roots  []string
	}{
		{"build_bin.sh", []string{
			"bin/RaysAndMagic.app/Contents/Resources",
			"bin/RaysAndMagicMapViewer.app/Contents/Resources",
		}},
		{"build_mac_release.sh", []string{
			"dist/mac_amd64", "dist/mac_arm64", "dist/windows_amd64",
			"dist/mac_amd64/RaysAndMagic.app/Contents/Resources",
			"dist/mac_amd64/RaysAndMagicMapViewer.app/Contents/Resources",
			"dist/mac_arm64/RaysAndMagic.app/Contents/Resources",
			"dist/mac_arm64/RaysAndMagicMapViewer.app/Contents/Resources",
		}},
	} {
		t.Run(tc.script, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "project with spaces")
			write := func(path string, data []byte, mode fs.FileMode) {
				t.Helper()
				path = filepath.Join(dir, path)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, mode); err != nil {
					t.Fatal(err)
				}
			}
			for _, script := range []string{tc.script, "_build_lib.sh"} {
				data, err := os.ReadFile(filepath.Join("..", script))
				if err != nil {
					t.Fatal(err)
				}
				write(script, data, 0755)
			}
			assets := map[string]string{
				"config.yaml":                                     "display: {}\n",
				"assets/items.yaml":                               "items: {}\n",
				"assets/text/dialogue_ui.yaml":                    "dialog.back: Back\n",
				"assets/text/tooltip_effects.yaml":                "item.collection: Collection\n",
				"assets/sprites/icon.png":                         "image bytes\x00\xff",
				"assets/sounds/theme.ogg":                         "audio bytes\x00\xff",
				"assets/app_icons/rays_and_magic.icns":            "game icon",
				"assets/app_icons/rays_and_magic_map_editor.icns": "editor icon",
			}
			for path, data := range assets {
				write(path, []byte(data), 0644)
			}
			sources := []string{"assets/text/catalog.go", "assets/text/catalog_test.go", "assets/another/package/source.go", "assets/map_viewer/main.go"}
			for _, path := range sources {
				write(path, []byte("package fixture\n"), 0644)
			}
			write("assets/map_viewer/source_only.txt", []byte("editor source tree"), 0644)
			write("tools/go", []byte(`#!/bin/bash
set -eu
if [ "${1:-}" = "run" ] && [ "${2:-}" = "./tools/shadergen" ]; then
  printf 'prepared shaders\n' >> shader-build.log
  exit 0
fi
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    shift
    mkdir -p "$(dirname "$1")"
    printf 'compiled binary\n' > "$1"
    exit 0
  fi
  shift
done
exit 1
`), 0755)
			write("tools/codesign", []byte("#!/bin/bash\nexit 0\n"), 0755)
			for _, state := range []string{"fresh", "rebuild"} {
				t.Run(state, func(t *testing.T) {
					if state == "rebuild" {
						for _, root := range tc.roots {
							write(filepath.Join(root, "assets/retired/stale_test.go"), []byte("package stale\n"), 0644)
						}
					}
					cmd := exec.Command(bash, tc.script)
					cmd.Dir = dir
					cmd.Env = append(os.Environ(), "PATH="+filepath.Join(dir, "tools")+string(os.PathListSeparator)+os.Getenv("PATH"))
					if output, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("build script failed: %v\n%s", err, output)
					}
					log, err := os.ReadFile(filepath.Join(dir, "shader-build.log"))
					wantRuns := 1
					if state == "rebuild" {
						wantRuns = 2
					}
					if err != nil || strings.Count(string(log), "prepared shaders") != wantRuns {
						t.Fatalf("shader preparation must run once per build: %q, %v", log, err)
					}
					for _, root := range tc.roots {
						for path, want := range assets {
							got, err := os.ReadFile(filepath.Join(dir, root, path))
							if err != nil || string(got) != want {
								t.Errorf("%s: runtime asset %s changed or missing: %v", root, path, err)
							}
						}
						if _, err := os.Stat(filepath.Join(dir, root, "assets/map_viewer")); !os.IsNotExist(err) {
							t.Errorf("%s: editor source tree packaged: %v", root, err)
						}
						if err := filepath.WalkDir(filepath.Join(dir, root, "assets"), func(path string, entry fs.DirEntry, err error) error {
							if err == nil && !entry.IsDir() && strings.HasSuffix(path, ".go") {
								t.Errorf("Go source packaged: %s", path)
							}
							return err
						}); err != nil {
							t.Fatal(err)
						}
					}
					for _, path := range sources {
						if data, err := os.ReadFile(filepath.Join(dir, path)); err != nil || string(data) != "package fixture\n" {
							t.Errorf("source asset modified: %s: %v", path, err)
						}
					}
				})
			}
		})
	}
}
