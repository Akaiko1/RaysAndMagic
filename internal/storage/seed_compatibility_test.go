package storage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func seedCompatibilityBundle(t *testing.T, root string, changed bool) {
	t.Helper()
	files := map[string]string{
		"config.yaml":                 "original config",
		"assets/forest.map":           "original forest",
		"assets/castle.map":           "original castle",
		"assets/retired.map":          "retired map",
		"assets/retired_edited.map":   "retired edited map",
		"assets/sprites/retired.png":  "retired sprite",
		"assets/sprites/retained.png": "original sprite",
	}
	if changed {
		files["config.yaml"] = "changed config"
		files["assets/forest.map"] = "changed forest"
		files["assets/sprites/retained.png"] = "changed sprite"
		delete(files, "assets/retired.map")
		delete(files, "assets/retired_edited.map")
		delete(files, "assets/sprites/retired.png")
	}
	for path, body := range files {
		writeFile(t, filepath.Join(root, path), body)
	}
}

func seedCompatibilityEdits(t *testing.T, user string) map[string]string {
	t.Helper()
	files := map[string]string{
		"config.yaml":               "player config",
		"assets/forest.map":         "player forest",
		"assets/castle.map":         "player castle",
		"assets/retired_edited.map": "player retired map",
		"assets/custom.map":         "custom map",
		"assets/sprites/custom.png": "custom sprite",
		"saves/player.yaml":         "player save",
	}
	for path, body := range files {
		writeFile(t, filepath.Join(user, path), body)
	}
	files["assets/retired.map"] = "retired map"
	files["assets/sprites/retired.png"] = "retired sprite"
	files["assets/sprites/retained.png"] = "original sprite"
	return files
}

func assertSeedCompatibilityFiles(t *testing.T, user string, want map[string]string) {
	t.Helper()
	for path, body := range want {
		b, err := os.ReadFile(filepath.Join(user, path))
		if body == "" {
			if !os.IsNotExist(err) {
				t.Errorf("retired %s still exists (error %v)", path, err)
			}
		} else if err != nil || string(b) != body {
			t.Errorf("%s = %q (error %v), want %q", path, b, err, body)
		}
	}
}

func TestSeedMixedVersionCompatibility(t *testing.T) {
	previousStamp := buildStamp
	t.Cleanup(func() { buildStamp = previousStamp })
	seeders := []struct {
		name string
		run  func(string, string) error
	}{
		{"legacy", legacySeedUserDataForCompatibility},
		{"current", seedUserData},
	}
	for _, writer := range seeders {
		for _, reader := range seeders {
			for _, stamp := range []int{100, 200, 300} {
				for _, changed := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s_to_%s/stamp_%d/changed_%t", writer.name, reader.name, stamp, changed), func(t *testing.T) {
						content, next, user := t.TempDir(), t.TempDir(), t.TempDir()
						seedCompatibilityBundle(t, content, false)
						seedCompatibilityBundle(t, next, changed)
						buildStamp = "200"
						if err := writer.run(content, user); err != nil {
							t.Fatal(err)
						}
						want := seedCompatibilityEdits(t, user)
						update := changed && stamp >= 200
						migration := !changed && stamp >= 200 && writer.name == "legacy" && reader.name == "current"
						if update {
							want["config.yaml"] = "changed config"
							want["assets/forest.map"] = "changed forest"
							want["assets/sprites/retained.png"] = "changed sprite"
							if reader.name == "current" {
								want["assets/retired.map"] = ""
								if writer.name == "current" {
									want["assets/sprites/retired.png"] = ""
								}
							}
						} else if migration {
							want["config.yaml"] = "original config"
						}
						buildStamp = strconv.Itoa(stamp)
						if err := reader.run(next, user); err != nil {
							t.Fatal(err)
						}
						assertSeedCompatibilityFiles(t, user, want)
						if fields := strings.Fields(read(t, filepath.Join(user, seedStateName))); len(fields) != 2 {
							t.Errorf("state has %d fields: older executables require exactly two", len(fields))
						}
						if reader.name == "current" && (update || migration) {
							manifest := loadSeedManifest(user)
							if manifest["sprites/retained.png"] == "" {
								t.Error("eligible current launch did not record non-map ownership")
							}
							if update && (manifest["retired.map"] != "" || manifest["retired_edited.map"] != "") {
								t.Error("retired maps remain updater-owned")
							}
						}
						// A second launch must not repeat the migration or change edits.
						writeFile(t, filepath.Join(user, "config.yaml"), "config after launch")
						want["config.yaml"] = "config after launch"
						if err := reader.run(next, user); err != nil {
							t.Fatal(err)
						}
						assertSeedCompatibilityFiles(t, user, want)
					})
				}
			}
		}
	}
}

func TestSeedRepairsInterimState(t *testing.T) {
	previousStamp := buildStamp
	t.Cleanup(func() { buildStamp = previousStamp })
	for _, stamp := range []int{100, 200, 300} {
		for _, changed := range []bool{false, true} {
			t.Run(fmt.Sprintf("stamp_%d/changed_%t", stamp, changed), func(t *testing.T) {
				content, next, stale, user := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
				seedCompatibilityBundle(t, content, false)
				seedCompatibilityBundle(t, next, changed)
				seedCompatibilityBundle(t, stale, true)
				buildStamp = "200"
				if err := seedUserData(content, user); err != nil {
					t.Fatal(err)
				}
				fields := strings.Fields(read(t, filepath.Join(user, seedStateName)))
				originalState := strings.Join(fields[:2], " ")
				writeFile(t, filepath.Join(user, seedStateName), originalState+" all-assets-v1")
				if err := os.Remove(filepath.Join(user, ".seed_manifest_state")); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				want := seedCompatibilityEdits(t, user)
				if changed && stamp >= 200 {
					want["config.yaml"] = "changed config"
					want["assets/forest.map"] = "changed forest"
					want["assets/sprites/retained.png"] = "changed sprite"
					want["assets/retired.map"] = ""
					want["assets/sprites/retired.png"] = ""
				}
				buildStamp = strconv.Itoa(stamp)
				if err := seedUserData(next, user); err != nil {
					t.Fatal(err)
				}
				assertSeedCompatibilityFiles(t, user, want)
				state := read(t, filepath.Join(user, seedStateName))
				if len(strings.Fields(state)) != 2 {
					t.Error("interim state was not repaired for older readers")
				}
				if (!changed || stamp < 200) && state != originalState {
					t.Error("metadata repair changed the installed build stamp or digest")
				}
				buildStamp = "50"
				if err := legacySeedUserDataForCompatibility(stale, user); err != nil {
					t.Fatal(err)
				}
				assertSeedCompatibilityFiles(t, user, want)
			})
		}
	}
}

func TestSeedManifestMarkerRecovery(t *testing.T) {
	previousStamp := buildStamp
	t.Cleanup(func() { buildStamp = previousStamp })
	for _, mode := range []string{"missing", "corrupt", "wrong_content", "legacy_manifest_write"} {
		t.Run(mode, func(t *testing.T) {
			content, other, user := t.TempDir(), t.TempDir(), t.TempDir()
			seedCompatibilityBundle(t, content, false)
			seedCompatibilityBundle(t, other, true)
			buildStamp = "200"
			if err := seedUserData(content, user); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(user, ".seed_manifest_state")
			switch mode {
			case "missing":
				if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
			case "corrupt":
				writeFile(t, marker, "incomplete")
			case "wrong_content":
				otherUser := t.TempDir()
				if err := seedUserData(other, otherUser); err != nil {
					t.Fatal(err)
				}
				writeFile(t, marker, read(t, filepath.Join(otherUser, ".seed_manifest_state")))
			case "legacy_manifest_write":
				// Legacy A -> B -> A leaves B-only entries in the manifest while
				// restoring A's digest. Its unchanged sidecar must not certify them.
				writeFile(t, filepath.Join(other, "assets/legacy_only.map"), "legacy-only map")
				buildStamp = "300"
				if err := legacySeedUserDataForCompatibility(other, user); err != nil {
					t.Fatal(err)
				}
				buildStamp = "400"
				if err := legacySeedUserDataForCompatibility(content, user); err != nil {
					t.Fatal(err)
				}
			}
			want := seedCompatibilityEdits(t, user)
			if mode == "legacy_manifest_write" {
				want["assets/legacy_only.map"] = ""
			}
			if err := seedUserData(content, user); err != nil {
				t.Fatal(err)
			}
			want["config.yaml"] = "original config"
			assertSeedCompatibilityFiles(t, user, want)
			if loadSeedManifest(user)["sprites/retained.png"] == "" {
				t.Error("manifest recovery skipped non-map ownership")
			}
			writeFile(t, filepath.Join(user, "config.yaml"), "post-migration config")
			want["config.yaml"] = "post-migration config"
			if err := seedUserData(content, user); err != nil {
				t.Fatal(err)
			}
			assertSeedCompatibilityFiles(t, user, want)
		})
	}
}

func TestSeedManifestMarkerWriteRetry(t *testing.T) {
	previousStamp := buildStamp
	t.Cleanup(func() { buildStamp = previousStamp })
	content, stale, user := t.TempDir(), t.TempDir(), t.TempDir()
	seedCompatibilityBundle(t, content, false)
	seedCompatibilityBundle(t, stale, true)
	marker := filepath.Join(user, ".seed_manifest_state")
	if err := os.Mkdir(marker, 0755); err != nil {
		t.Fatal(err)
	}
	buildStamp = "200"
	if err := seedUserData(content, user); err == nil {
		t.Fatal("sidecar write failure was ignored")
	}
	want := seedCompatibilityEdits(t, user)
	buildStamp = "100"
	if err := legacySeedUserDataForCompatibility(stale, user); err != nil {
		t.Fatal(err)
	}
	assertSeedCompatibilityFiles(t, user, want)
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	buildStamp = "200"
	if err := seedUserData(content, user); err != nil {
		t.Fatal(err)
	}
	want["config.yaml"] = "original config"
	assertSeedCompatibilityFiles(t, user, want)
}

// Frozen seeder/copy implementations from 4efbe4ab04854647720dbaf72c92ee426fd6c14d.
// Only names and the manifest Save method name are adapted. Keep the original
// two-field guard and copy policy: using today's seeder twice misses old readers.
// seedUserData copies the shipped config.yaml + assets tree from contentDir into
// userDir so the app runs from a writable copy. Reseeding is fully automatic -
// no version constant to bump: it fires whenever the shipped content digest
// differs from the last seeded one. A .map is overwritten only when the SHIPPED
// version changed since the last seed (see seedManifest) - new authored maps
// always land, untouched-by-author maps keep any player edits.
func legacySeedUserDataForCompatibility(contentDir, userDir string) error {
	digest, err := shippedContentDigest(contentDir)
	if err != nil {
		return err
	}
	statePath := filepath.Join(userDir, seedStateName)
	if b, err := os.ReadFile(statePath); err == nil {
		if fields := strings.Fields(string(b)); len(fields) == 2 {
			if fields[1] == digest {
				return nil // shipped content unchanged since the last seed
			}
			// The game and editor bundles share this dir: a stale build (older
			// stamp) must not stomp content seeded by a newer one.
			if stamp, convErr := strconv.ParseInt(fields[0], 10, 64); convErr == nil && stamp > buildStampUnix() {
				return nil
			}
		}
	}
	if err := copyFileForce(filepath.Join(contentDir, "config.yaml"), filepath.Join(userDir, "config.yaml")); err != nil {
		return err
	}
	manifest := loadSeedManifest(userDir)
	if err := legacyCopyAssetsTreeForCompatibility(filepath.Join(contentDir, "assets"), filepath.Join(userDir, "assets"), manifest); err != nil {
		return err
	}
	if err := manifest.Save(userDir); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(userDir, ".seed_version")) // pre-digest scheme leftover
	return os.WriteFile(statePath, []byte(fmt.Sprintf("%d %s", buildStampUnix(), digest)), 0644)
}

// copyAssetsTree mirrors src into dst, overwriting every file EXCEPT a .map
// whose shipped version is unchanged since the last seed (manifest hash match)
// - that one keeps whatever the player has, edits included. A changed or
// never-tracked shipped map always wins and is (re)recorded in manifest.
func legacyCopyAssetsTreeForCompatibility(src, dst string, manifest seedManifest) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if strings.HasSuffix(p, ".map") {
			key := filepath.ToSlash(rel)
			srcHash, ok := fileSHA256(p)
			if !ok {
				return fmt.Errorf("hash shipped map %q", p)
			}
			if _, statErr := os.Stat(target); statErr == nil {
				if shipped, tracked := manifest[key]; tracked && shipped == srcHash {
					return nil // author shipped no new version: keep the player's copy
				}
			}
			if err := copyFileForce(p, target); err != nil {
				return err
			}
			manifest[key] = srcHash
			return nil
		}
		return copyFileForce(p, target)
	})
}
