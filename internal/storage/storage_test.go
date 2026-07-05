package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInsideAppBundle(t *testing.T) {
	cases := map[string]bool{
		"/Applications/RaysAndMagic.app/Contents/MacOS/RaysAndMagic": true,
		"/Users/x/Downloads/Foo.app/Contents/MacOS/Foo":              true,
		"/usr/local/bin/raysandmagic":                                false,
		"/Users/x/game/bin/raysandmagic":                             false,
	}
	for path, want := range cases {
		if got := insideAppBundle(path); got != want {
			t.Errorf("insideAppBundle(%q) = %v, want %v", path, got, want)
		}
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSetupBundleRuntime_SeedsAndChdirs(t *testing.T) {
	// Isolate the per-user config dir under a temp home (cross-platform).
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "cfg"))
	t.Setenv("AppData", filepath.Join(home, "appdata")) // os.UserConfigDir on Windows

	// Fake .app bundle with read-only Resources content.
	root := t.TempDir()
	exe := filepath.Join(root, "RaysAndMagic.app", "Contents", "MacOS", "RaysAndMagic")
	res := filepath.Join(root, "RaysAndMagic.app", "Contents", "Resources")
	writeFile(t, exe, "binary")
	writeFile(t, filepath.Join(res, "config.yaml"), "cfg")
	writeFile(t, filepath.Join(res, "assets", "forest.map"), "map")
	writeFile(t, filepath.Join(res, "assets", "items.yaml"), "yaml")

	origCwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(origCwd); dataRoot = "" })

	if !setupBundleRuntime(exe) {
		t.Fatal("setupBundleRuntime returned false for a bundle path")
	}
	if dataRoot == "" || !strings.HasPrefix(dataRoot, home) {
		t.Fatalf("dataRoot = %q, want an isolated dir under %q", dataRoot, home)
	}
	cwd, _ := os.Getwd()
	// EvalSymlinks: on macOS the temp dir is /var → /private/var symlinked.
	evalCwd, _ := filepath.EvalSymlinks(cwd)
	evalRoot, _ := filepath.EvalSymlinks(dataRoot)
	if evalCwd != evalRoot {
		t.Fatalf("cwd = %q, want dataRoot %q", cwd, dataRoot)
	}
	for _, rel := range []string{"config.yaml", "assets/forest.map", "assets/items.yaml"} {
		if _, err := os.Stat(filepath.Join(dataRoot, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("%s not seeded into user dir: %v", rel, err)
		}
	}
	if got := AppSaveDir(); filepath.Clean(filepath.Dir(got)) != filepath.Clean(dataRoot) {
		t.Fatalf("AppSaveDir = %q, want <dataRoot>/saves under %q", got, dataRoot)
	}
}

func TestSeedUserData_CopiesAndPreservesMaps(t *testing.T) {
	content := t.TempDir()
	user := t.TempDir()
	writeFile(t, filepath.Join(content, "config.yaml"), "cfg-v1")
	writeFile(t, filepath.Join(content, "assets", "forest.map"), "map-shipped")
	writeFile(t, filepath.Join(content, "assets", "sprites", "x.png"), "png-v1")

	if err := seedUserData(content, user); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if got := read(t, filepath.Join(user, "config.yaml")); got != "cfg-v1" {
		t.Fatalf("config not seeded: %q", got)
	}
	if got := read(t, filepath.Join(user, "assets", "sprites", "x.png")); got != "png-v1" {
		t.Fatalf("sprite not seeded: %q", got)
	}

	// Player edits a map; a second seed at the SAME version must not touch it.
	writeFile(t, filepath.Join(user, "assets", "forest.map"), "map-edited")
	if err := seedUserData(content, user); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	if got := read(t, filepath.Join(user, "assets", "forest.map")); got != "map-edited" {
		t.Fatalf("edited map was clobbered: %q", got)
	}
}

func TestCopyAssetsTree_UntrackedMapAuthorWins(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "forest.map"), "shipped-map")
	writeFile(t, filepath.Join(src, "items.yaml"), "shipped-yaml-v2")
	// Pre-existing edited map with NO manifest entry (legacy install) + stale yaml.
	writeFile(t, filepath.Join(dst, "forest.map"), "edited-map")
	writeFile(t, filepath.Join(dst, "items.yaml"), "old-yaml")

	manifest := seedManifest{}
	if err := copyAssetsTree(src, dst, manifest); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if got := read(t, filepath.Join(dst, "forest.map")); got != "shipped-map" {
		t.Errorf("untracked map: shipped version must win, got %q", got)
	}
	if got := read(t, filepath.Join(dst, "items.yaml")); got != "shipped-yaml-v2" {
		t.Errorf("non-map should be overwritten, got %q", got)
	}
	if _, tracked := manifest["forest.map"]; !tracked {
		t.Errorf("seeded map should be recorded in the manifest")
	}
	// A brand-new shipped map gets copied in too.
	writeFile(t, filepath.Join(src, "new.map"), "new-map")
	if err := copyAssetsTree(src, dst, manifest); err != nil {
		t.Fatalf("copy2: %v", err)
	}
	if got := read(t, filepath.Join(dst, "new.map")); got != "new-map" {
		t.Errorf("new map not copied, got %q", got)
	}
}

// TestSeedUserData_AuthorUpdateWinsEditSurvivesOtherwise: an authored map
// update always lands (even over player edits); a map the author did NOT
// change keeps the player's edit across version bumps.
func TestSeedUserData_AuthorUpdateWinsEditSurvivesOtherwise(t *testing.T) {
	content := t.TempDir()
	user := t.TempDir()
	writeFile(t, filepath.Join(content, "config.yaml"), "cfg-v1")
	writeFile(t, filepath.Join(content, "assets", "forest.map"), "forest-v1")
	writeFile(t, filepath.Join(content, "assets", "castle.map"), "castle-v1")

	if err := seedUserData(content, user); err != nil {
		t.Fatalf("initial seed: %v", err)
	}

	// Player edits castle.map. Update A ships a new forest but does NOT touch
	// castle — the digest change alone must trigger the reseed.
	writeFile(t, filepath.Join(user, "assets", "castle.map"), "castle-edited")
	writeFile(t, filepath.Join(content, "assets", "forest.map"), "forest-v2")

	if err := seedUserData(content, user); err != nil {
		t.Fatalf("reseed A: %v", err)
	}
	if got := read(t, filepath.Join(user, "assets", "forest.map")); got != "forest-v2" {
		t.Errorf("shipped forest update should land, got %q", got)
	}
	if got := read(t, filepath.Join(user, "assets", "castle.map")); got != "castle-edited" {
		t.Errorf("author shipped no castle change: player edit must survive, got %q", got)
	}

	// Update B ships a new castle: the authored version overrides the edit.
	writeFile(t, filepath.Join(content, "assets", "castle.map"), "castle-v2")
	if err := seedUserData(content, user); err != nil {
		t.Fatalf("reseed B: %v", err)
	}
	if got := read(t, filepath.Join(user, "assets", "castle.map")); got != "castle-v2" {
		t.Errorf("authored castle update must override the player edit, got %q", got)
	}
}

// TestSeedUserData_StaleBuildDoesNotStompNewerSeed: the game and editor bundles
// share the user dir; an older build (smaller buildStamp) with different
// content must leave a newer build's seed alone.
func TestSeedUserData_StaleBuildDoesNotStompNewerSeed(t *testing.T) {
	content := t.TempDir()
	user := t.TempDir()
	writeFile(t, filepath.Join(content, "config.yaml"), "cfg-new")
	writeFile(t, filepath.Join(content, "assets", "forest.map"), "forest-new")

	t.Cleanup(func() { buildStamp = "" })
	buildStamp = "200" // the newer build seeds first
	if err := seedUserData(content, user); err != nil {
		t.Fatalf("newer seed: %v", err)
	}

	// A stale build (older stamp) carrying older content launches afterwards.
	stale := t.TempDir()
	writeFile(t, filepath.Join(stale, "config.yaml"), "cfg-old")
	writeFile(t, filepath.Join(stale, "assets", "forest.map"), "forest-old")
	buildStamp = "100"
	if err := seedUserData(stale, user); err != nil {
		t.Fatalf("stale seed: %v", err)
	}
	if got := read(t, filepath.Join(user, "config.yaml")); got != "cfg-new" {
		t.Errorf("stale build stomped config: got %q, want cfg-new", got)
	}
	if got := read(t, filepath.Join(user, "assets", "forest.map")); got != "forest-new" {
		t.Errorf("stale build stomped map: got %q, want forest-new", got)
	}

	// The same old content under an EQUAL-or-newer stamp is a legitimate
	// rollback/update and must apply.
	buildStamp = "300"
	if err := seedUserData(stale, user); err != nil {
		t.Fatalf("newer stale-content seed: %v", err)
	}
	if got := read(t, filepath.Join(user, "config.yaml")); got != "cfg-old" {
		t.Errorf("newer build's content should apply, got %q", got)
	}
}
