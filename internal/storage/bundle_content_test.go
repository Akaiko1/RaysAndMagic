package storage

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Only the game bundle ships content. Any bundle runs from its own content
// when it has some, else from the game bundle beside it or in an install
// folder, else from a data dir the game already seeded; with none of those it
// stops with a visible message instead of launching empty.
func TestBundleContentSource(t *testing.T) {
	type layout struct {
		editorOwn, gameSibling, gameInstalled, seeded, gameConfigOnly bool
	}
	cases := []struct {
		name   string
		layout layout
		// want is the marker the seeded config.yaml must carry; "" means the
		// launch fails with errNoGameContent.
		want string
	}{
		{"editor ships its own content", layout{editorOwn: true, gameSibling: true}, "editor"},
		{"editor next to the game", layout{gameSibling: true}, "sibling"},
		{"editor alone, game installed", layout{gameInstalled: true}, "installed"},
		{"sibling wins over installed", layout{gameSibling: true, gameInstalled: true}, "sibling"},
		{"sibling without assets is skipped", layout{gameConfigOnly: true, gameInstalled: true}, "installed"},
		{"editor alone, data already seeded", layout{seeded: true}, "seeded"},
		{"editor alone, nothing reachable", layout{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "cfg"))
			t.Setenv("AppData", filepath.Join(home, "appdata"))
			origCwd, _ := os.Getwd()
			t.Cleanup(func() { _ = os.Chdir(origCwd); dataRoot = "" })
			installed := t.TempDir()
			prevDirs, prevAlert := gameBundleSearchDirs, alertFailure
			t.Cleanup(func() { gameBundleSearchDirs, alertFailure = prevDirs, prevAlert })
			gameBundleSearchDirs = func() []string { return []string{installed} }
			var alerts []string
			alertFailure = func(msg string) { alerts = append(alerts, msg) }

			ship := func(resources, marker string, withAssets bool) {
				writeFile(t, filepath.Join(resources, "config.yaml"), marker)
				if withAssets {
					writeFile(t, filepath.Join(resources, "assets", "items.yaml"), "items")
				}
			}
			folder := t.TempDir()
			exe := filepath.Join(folder, "RaysAndMagicMapViewer.app", "Contents", "MacOS", "RaysAndMagicMapViewer")
			writeFile(t, exe, "binary")
			writeFile(t, filepath.Join(folder, "RaysAndMagicMapViewer.app", "Contents", "Resources", "editor.icns"), "icon")
			if tc.layout.editorOwn {
				ship(filepath.Join(folder, "RaysAndMagicMapViewer.app", "Contents", "Resources"), "editor", true)
			}
			if tc.layout.gameSibling || tc.layout.gameConfigOnly {
				ship(filepath.Join(folder, gameBundleName, "Contents", "Resources"), "sibling", tc.layout.gameSibling)
			}
			if tc.layout.gameInstalled {
				ship(filepath.Join(installed, gameBundleName, "Contents", "Resources"), "installed", true)
			}
			if tc.layout.seeded {
				user := UserDataDir()
				if user == "" {
					t.Fatal("setup: no user data dir")
				}
				ship(user, "seeded", true)
			}

			handled, err := setupBundleRuntime(exe)
			if !handled {
				t.Fatal("bundle launch not handled")
			}
			if tc.want == "" {
				if !errors.Is(err, errNoGameContent) || len(alerts) != 1 || alerts[0] != errNoGameContent.Error() {
					t.Fatalf("err=%v alerts=%q, want errNoGameContent shown once", err, alerts)
				}
				return
			}
			if err != nil || len(alerts) != 0 {
				t.Fatalf("err=%v alerts=%q, want a clean launch", err, alerts)
			}
			if got := read(t, filepath.Join(dataRoot, "config.yaml")); got != tc.want {
				t.Fatalf("running from %q content, want %q", got, tc.want)
			}
			cwd, _ := os.Getwd()
			evalCwd, _ := filepath.EvalSymlinks(cwd)
			evalRoot, _ := filepath.EvalSymlinks(dataRoot)
			if evalCwd != evalRoot {
				t.Fatalf("cwd %q, want the shared data dir %q", cwd, dataRoot)
			}
		})
	}
}

// gameBundleName must be the bundle both build scripts produce.
func TestGameBundleNameMatchesBuildScripts(t *testing.T) {
	for _, script := range []string{"../../build_bin.sh", "../../build_mac_release.sh"} {
		data, err := os.ReadFile(script)
		if err != nil {
			t.Fatal(err)
		}
		m := regexp.MustCompile(`(?m)^APP_NAME="([^"]+)"`).FindSubmatch(data)
		if m == nil || string(m[1])+".app" != gameBundleName {
			t.Fatalf("%s builds %q, storage expects %q", script, m, gameBundleName)
		}
	}
}
