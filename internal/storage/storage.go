// Package storage resolves the directories used to persist save files and other
// user data. For a bare binary (or "go run") it keeps the original behaviour:
// data lives next to the executable / working directory. For a macOS .app BUNDLE
// it seeds a writable per-user data directory from the read-only shipped assets
// and runs out of there - bundles can't reliably write inside themselves
// (Gatekeeper App Translocation runs them from a read-only path), and the game
// and map-editor bundles each carry a private assets copy, so a shared writable
// dir is the only way edits + saves work and are seen by both.
package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

const savesDirName = "saves"

// buildStamp optionally overrides the build-ordering stamp via `-ldflags -X`
// (unix seconds). It orders competing seeders of the shared user dir (game vs
// editor .app): a build with an older stamp never overwrites content seeded by
// a newer one. When unset, the git commit time Go embeds into every build
// (vcs.time) is used - so both the CI release package and local script builds
// order correctly with no build-script changes. 0 when neither is available.
var buildStamp string

func buildStampUnix() int64 {
	if v, err := strconv.ParseInt(strings.TrimSpace(buildStamp), 10, 64); err == nil {
		return v
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.time" {
				if t, err := time.Parse(time.RFC3339, s.Value); err == nil {
					return t.Unix()
				}
			}
		}
	}
	return 0
}

const (
	seedManifestName = ".seed_manifest"
	seedStateName    = ".seed_state" // "<build stamp> <shipped-content digest>"
)

// seedManifest maps a seeded .map's asset-relative path to the SHIPPED content
// hash as of the last seed. Author updates take priority over local edits: on a
// version bump a map is overwritten iff the shipped version changed (or was
// never tracked); while the author ships no new version, the player's copy -
// edited or not - is left alone.
type seedManifest map[string]string

func loadSeedManifest(userDir string) seedManifest {
	m := seedManifest{}
	if b, err := os.ReadFile(filepath.Join(userDir, seedManifestName)); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func (m seedManifest) save(userDir string) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(userDir, seedManifestName), b, 0644)
}

// fileSHA256 returns the hex content hash; ok=false when the file can't be read
// (typically: doesn't exist).
func fileSHA256(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", false
	}
	return hex.EncodeToString(h.Sum(nil)), true
}

// dataRoot is the writable runtime root when running as a .app bundle (empty
// otherwise). Set once by SetupBundleRuntime; AppSaveDir writes saves under it.
var dataRoot string

// AppSaveDir returns the local saves directory, creating it on demand. Errors
// during creation are logged to stderr; callers always receive a path string
// even if the directory could not be created.
func AppSaveDir() string {
	if dataRoot != "" {
		dir := filepath.Join(dataRoot, savesDirName)
		err := os.MkdirAll(dir, 0755)
		if err == nil {
			return dir
		}
		fmt.Fprintf(os.Stderr, "[storage] failed to create saves directory %q: %v\n", dir, err)
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		if !isTempExeDir(exeDir) {
			dir := filepath.Join(exeDir, savesDirName)
			if err := os.MkdirAll(dir, 0755); err == nil {
				return dir
			}
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		dir := filepath.Join(cwd, savesDirName)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "[storage] failed to create saves directory %q: %v\n", dir, err)
		}
		return dir
	}
	return savesDirName
}

// AppSavePath joins filename with the resolved save directory.
func AppSavePath(filename string) string {
	return filepath.Join(AppSaveDir(), filename)
}

// SetupBundleRuntime handles the macOS .app case ONLY: it seeds a per-user data
// dir from the bundle's read-only Resources and chdirs into it, so all relative
// asset/save paths become writable and are shared between the game and editor
// bundles. Returns true if it handled the runtime (i.e. we're in a bundle) so
// callers skip their normal (bare-binary) working-dir setup. No-op for bare
// binaries / go run / Windows exes - those work exactly as before.
func SetupBundleRuntime() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return setupBundleRuntime(exe)
}

func setupBundleRuntime(exe string) bool {
	if !insideAppBundle(exe) {
		return false
	}
	resources := filepath.Clean(filepath.Join(filepath.Dir(exe), "..", "Resources"))
	if user := UserDataDir(); user != "" && seedUserData(resources, user) == nil && os.Chdir(user) == nil {
		dataRoot = user
		migrateLegacySaves(filepath.Join(filepath.Dir(exe), savesDirName), filepath.Join(user, savesDirName))
	} else {
		// Last resort: run read-only from the bundle so the app at least launches
		// (saving/editing will fail, but that beats not starting).
		_ = os.Chdir(resources)
	}
	return true
}

// UserDataDir returns the per-user writable data root (.../RaysAndMagic), created
// on demand. "" if it can't be resolved/created.
func UserDataDir() string {
	base, err := os.UserConfigDir() // macOS: ~/Library/Application Support
	if err != nil {
		return ""
	}
	dir := filepath.Join(base, "RaysAndMagic")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ""
	}
	return dir
}

// insideAppBundle reports whether exe lives inside a macOS .app bundle.
func insideAppBundle(exe string) bool {
	return strings.Contains(filepath.ToSlash(exe), ".app/Contents/MacOS/")
}

// seedUserData copies the shipped config.yaml + assets tree from contentDir into
// userDir so the app runs from a writable copy. Reseeding is fully automatic -
// no version constant to bump: it fires whenever the shipped content digest
// differs from the last seeded one. A .map is overwritten only when the SHIPPED
// version changed since the last seed (see seedManifest) - new authored maps
// always land, untouched-by-author maps keep any player edits.
func seedUserData(contentDir, userDir string) error {
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
	if err := copyAssetsTree(filepath.Join(contentDir, "assets"), filepath.Join(userDir, "assets"), manifest); err != nil {
		return err
	}
	if err := manifest.save(userDir); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(userDir, ".seed_version")) // pre-digest scheme leftover
	return os.WriteFile(statePath, []byte(fmt.Sprintf("%d %s", buildStampUnix(), digest)), 0644)
}

// shippedContentDigest hashes everything seedUserData would copy (config.yaml +
// the assets tree, paths included), so any shipped change - content, rename,
// addition, removal - yields a new digest. WalkDir order is lexical, hence
// deterministic. ~100ms for the full tree, paid once per bundle launch.
func shippedContentDigest(contentDir string) (string, error) {
	h := sha256.New()
	addFile := func(rel, p string) error {
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := io.WriteString(h, rel+"\x00"); err != nil {
			return err
		}
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		_, err = io.WriteString(h, "\x00")
		return err
	}
	if err := addFile("config.yaml", filepath.Join(contentDir, "config.yaml")); err != nil {
		return "", err
	}
	assetsRoot := filepath.Join(contentDir, "assets")
	err := filepath.WalkDir(assetsRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(assetsRoot, p)
		if err != nil {
			return err
		}
		return addFile("assets/"+filepath.ToSlash(rel), p)
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// migrateLegacySaves copies saves/stash/highscores from the pre-user-dir
// location (inside the bundle's Contents/MacOS) into the shared saves dir, so
// data from older installs survives the switch. Existing files in the new
// location always win.
func migrateLegacySaves(oldDir, newDir string) {
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		target := filepath.Join(newDir, e.Name())
		if _, statErr := os.Stat(target); statErr == nil {
			continue
		}
		if copyErr := copyFileForce(filepath.Join(oldDir, e.Name()), target); copyErr != nil {
			fmt.Fprintf(os.Stderr, "[storage] failed to migrate legacy save %q: %v\n", e.Name(), copyErr)
		}
	}
}

// copyAssetsTree mirrors src into dst, overwriting every file EXCEPT a .map
// whose shipped version is unchanged since the last seed (manifest hash match)
// - that one keeps whatever the player has, edits included. A changed or
// never-tracked shipped map always wins and is (re)recorded in manifest.
func copyAssetsTree(src, dst string, manifest seedManifest) error {
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

// copyFileForce copies src to dst, creating parent dirs and overwriting dst.
func copyFileForce(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// isTempExeDir reports whether dir looks like a Go temp build path (used when
// the binary is launched via "go run").
func isTempExeDir(dir string) bool {
	clean := filepath.Clean(dir)
	if strings.Contains(clean, string(filepath.Separator)+"go-build") {
		return true
	}
	if strings.HasPrefix(clean, filepath.Clean(os.TempDir())+string(filepath.Separator)) {
		return true
	}
	return false
}
