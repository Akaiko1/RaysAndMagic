package storage

import (
	"path/filepath"
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
