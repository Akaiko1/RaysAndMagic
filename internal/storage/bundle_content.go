package storage

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// gameBundleName is the only .app that ships config.yaml + assets (APP_NAME in
// the build scripts). The editor bundle carries no copy and reads this one.
const gameBundleName = "RaysAndMagic.app"

var errNoGameContent = fmt.Errorf("the map editor reads the game's files: keep %s in the same folder or in Applications, or start the game once", gameBundleName)

// gameBundleSearchDirs lists the install folders checked after the bundle's own
// folder. A variable so tests can point it at temp dirs.
var gameBundleSearchDirs = func() []string {
	dirs := []string{"/Applications"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "Applications"))
	}
	return dirs
}

// alertFailure surfaces a startup failure of a Finder-launched bundle, whose
// stderr nobody sees. A variable so tests stay silent.
var alertFailure = func(msg string) {
	_ = exec.Command("osascript", "-e", "display alert \"Rays and Magic\" message "+strconv.Quote(msg)).Run()
}

// bundleContentDir returns the Resources dir holding the shipped content for
// the bundle that owns exe: its own when it ships content, otherwise the game
// bundle's next to it, then in an install folder.
func bundleContentDir(exe string) (string, bool) {
	appDir := filepath.Clean(filepath.Join(filepath.Dir(exe), "..", ".."))
	candidates := []string{filepath.Join(appDir, "Contents", "Resources")}
	for _, dir := range append([]string{filepath.Dir(appDir)}, gameBundleSearchDirs()...) {
		candidates = append(candidates, filepath.Join(dir, gameBundleName, "Contents", "Resources"))
	}
	for _, dir := range candidates {
		if hasShippedContent(dir) {
			return dir, true
		}
	}
	return "", false
}

func hasShippedContent(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, "assets"))
	return err == nil && info.IsDir()
}
