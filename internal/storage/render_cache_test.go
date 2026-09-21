package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRenderCacheLocalAndBundleRoots(t *testing.T) {
	old := dataRoot
	t.Cleanup(func() { dataRoot = old })
	local, bundle := t.TempDir(), t.TempDir()
	t.Chdir(local)
	for _, tc := range []struct{ name, root, want string }{{"local", "", local}, {"bundle", bundle, bundle}} {
		t.Run(tc.name, func(t *testing.T) {
			dataRoot = tc.root
			if got := RenderCacheDir(); got != filepath.Join(tc.want, ".render-cache") {
				t.Fatalf("root=%s", got)
			}
		})
	}
}

func TestRenderCacheUsesSaveRootPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, root, executable, cwd string
		want                        []string
	}{
		{"bare_binary_from_root", "", "/Applications/Rays/game", "/", []string{"/Applications/Rays", "/"}},
		{"development", "", filepath.Join(os.TempDir(), "go-build-test", "game"), "/project", []string{"/project"}},
		{"bundle", "/user/data", "/Applications/Rays.app/Contents/MacOS/game", "/", []string{"/user/data", "/Applications/Rays.app/Contents/MacOS", "/"}},
		{"no_executable", "", "", "/project", []string{"/project"}},
		{"no_paths", "", "", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := dataDirectoryRoots(tc.root, tc.executable, tc.cwd); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("roots=%v want %v", got, tc.want)
			}
		})
	}
	for _, available := range []bool{false, true} {
		t.Run(fmt.Sprintf("fallback=%v", available), func(t *testing.T) {
			dir := t.TempDir()
			blocked := filepath.Join(dir, "file")
			if err := os.WriteFile(blocked, []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			roots := []string{blocked}
			want := ""
			if available {
				roots = append(roots, dir)
				want = filepath.Join(dir, ".render-cache")
			}
			if got := renderCacheDir(roots); got != want {
				t.Fatalf("cache=%s want %s", got, want)
			}
		})
	}
}
