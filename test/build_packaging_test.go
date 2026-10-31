package test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Run the real build entry points against small assets. Compilation, lipo,
// otool and signing are stand-ins here (a stand-in binary records the target it
// was built for); copying, filtering and output layouts are production code. Content ships once per install unit: the game bundle
// and the Windows and Linux folders carry it, the editor bundle only its icon,
// and the release mac folder holds nothing but the two bundles.
func TestBuildRuntimePackaging(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required for the macOS build scripts")
	}
	lib, err := os.ReadFile(filepath.Join("..", "_build_lib.sh"))
	if err != nil {
		t.Fatal(err)
	}
	floorLine := regexp.MustCompile(`(?m)^MACOS_MIN_VERSION="([0-9.]+)"$`).FindSubmatch(lib)
	if floorLine == nil {
		t.Fatal("_build_lib.sh declares no MACOS_MIN_VERSION")
	}
	floor := string(floorLine[1]) // the stand-in otool reports it unless a case overrides
	for _, tc := range []struct {
		script  string
		roots   []string            // carry the full runtime content
		exact   map[string][]string // directory -> its complete entry list
		targets map[string]string   // executable -> the GOOS/GOARCH and cgo it was built with
	}{
		{"build_bin.sh", []string{
			"bin/RaysAndMagic.app/Contents/Resources",
		}, map[string][]string{
			"bin/RaysAndMagicMapViewer.app/Contents/Resources": {"rays_and_magic_map_editor.icns"},
		}, map[string]string{
			"bin/RaysAndMagic.app/Contents/MacOS/RaysAndMagic": "host/host cgo=0",
			"bin/map_viewer":                "host/host cgo=0",
			"bin/RaysAndMagic.exe":          "windows/amd64 cgo=0",
			"bin/RaysAndMagicMapViewer.exe": "windows/amd64 cgo=0",
		}},
		{"build_mac_release.sh", []string{
			"dist/windows_amd64",
			"dist/linux_amd64",
			"dist/mac_universal/RaysAndMagic.app/Contents/Resources",
		}, map[string][]string{
			"dist":               {"linux_amd64", "mac_universal", "windows_amd64"},
			"dist/linux_amd64":   {"RaysAndMagic", "RaysAndMagicMapViewer", "assets", "config.yaml"},
			"dist/mac_universal": {"RaysAndMagic.app", "RaysAndMagicMapViewer.app"},
			"dist/mac_universal/RaysAndMagicMapViewer.app/Contents/Resources": {"rays_and_magic_map_editor.icns"},
		}, map[string]string{
			"dist/mac_universal/RaysAndMagic.app/Contents/MacOS/RaysAndMagic":                   "universal darwin/arm64 cgo=0 darwin/amd64 cgo=0",
			"dist/mac_universal/RaysAndMagicMapViewer.app/Contents/MacOS/RaysAndMagicMapViewer": "universal darwin/arm64 cgo=0 darwin/amd64 cgo=0",
			"dist/windows_amd64/RaysAndMagic.exe":                                               "windows/amd64 cgo=0",
			"dist/windows_amd64/RaysAndMagicMapViewer.exe":                                      "windows/amd64 cgo=0",
			"dist/linux_amd64/RaysAndMagic":                                                     "linux/amd64 cgo=0",
			"dist/linux_amd64/RaysAndMagicMapViewer":                                            "linux/amd64 cgo=0",
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
			for _, script := range []string{tc.script, "_build_lib.sh", "scripts/test_scenario.command", "scripts/test_arena.command", "scripts/test_pilgrimage.command"} {
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
    printf '%s/%s cgo=%s\n' "${GOOS:-host}" "${GOARCH:-host}" "${CGO_ENABLED:-default}" > "$1"
    exit 0
  fi
  shift
done
exit 1
`), 0755)
			write("tools/codesign", []byte("#!/bin/bash\nexit 0\n"), 0755)
			write("tools/otool", []byte("#!/bin/bash\nprintf '      cmd LC_BUILD_VERSION\\n    minos %s\\n' \"${FAKE_MINOS:-"+floor+"}\"\n"), 0755)
			write("tools/lipo", []byte(`#!/bin/bash
set -eu
[ "$1" = "-create" ] && [ "$2" = "-output" ]
out="$3"; shift 3
{ printf 'universal'; for slice in "$@"; do printf ' %s' "$(cat "$slice")"; done; printf '\n'; } > "$out"
`), 0755)
			// An exported GOOS/GOARCH/CGO_ENABLED in the caller's shell must not
			// leak into the scripts' host builds.
			env := append(os.Environ(), "GOOS=", "GOARCH=", "CGO_ENABLED=", "PATH="+filepath.Join(dir, "tools")+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Run("newer macOS floor fails", func(t *testing.T) {
				cmd := exec.Command(bash, tc.script)
				cmd.Dir = dir
				cmd.Env = append(env, "FAKE_MINOS=27.0")
				output, err := cmd.CombinedOutput()
				if err == nil || !strings.Contains(string(output), "requires macOS 27.0, bundles promise "+floor) {
					t.Fatalf("a binary needing macOS 27 was packaged: %v\n%s", err, output)
				}
				_ = os.Remove(filepath.Join(dir, "shader-build.log"))
			})
			for _, state := range []string{"fresh", "rebuild"} {
				t.Run(state, func(t *testing.T) {
					if state == "rebuild" {
						for _, root := range tc.roots {
							write(filepath.Join(root, "assets/retired/stale_test.go"), []byte("package stale\n"), 0644)
						}
					}
					cmd := exec.Command(bash, tc.script)
					cmd.Dir = dir
					cmd.Env = env
					if output, err := cmd.CombinedOutput(); err != nil {
						t.Fatalf("build script failed: %v\n%s", err, output)
					}
					if tc.script == "build_bin.sh" {
						for _, name := range []string{"test_scenario.command", "test_arena.command", "test_pilgrimage.command"} {
							source, _ := os.ReadFile(filepath.Join(dir, "scripts", name))
							copied, err := os.ReadFile(filepath.Join(dir, "bin", name))
							info, statErr := os.Stat(filepath.Join(dir, "bin", name))
							if err != nil || statErr != nil || string(source) != string(copied) || info.Mode()&0111 == 0 {
								t.Fatalf("launcher %s was not installed executable: %v, %v", name, err, statErr)
							}
						}
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
					for root, want := range tc.exact {
						entries, err := os.ReadDir(filepath.Join(dir, root))
						var got []string
						for _, e := range entries {
							got = append(got, e.Name())
						}
						if err != nil || strings.Join(got, ",") != strings.Join(want, ",") {
							t.Errorf("%s holds %v (%v), want exactly %v", root, got, err, want)
						}
					}
					for exe, want := range tc.targets {
						if data, err := os.ReadFile(filepath.Join(dir, exe)); err != nil || string(data) != want+"\n" {
							t.Errorf("%s built as %q (%v), want %q", exe, data, err, want)
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
