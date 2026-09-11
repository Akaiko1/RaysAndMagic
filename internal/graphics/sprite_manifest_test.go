package graphics

import (
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/assetmanifest"
)

func TestSpriteManifestDuplicatePriority(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		firstRoot, secondRoot       string
		firstTracked, secondTracked bool
		wantSecond                  bool
		wantType                    string
	}{
		{"checkout_order", "environment", "environment", false, false, false, "environment"},
		{"current_after_legacy", "environment", "environment", false, true, true, "environment"},
		{"current_before_custom", "environment", "environment", true, false, false, "environment"},
		{"both_shipped", "environment", "environment", true, true, false, "environment"},
		{"later_root_shipped", "mobs", "interface", false, true, true, "interface"},
		{"earlier_root_shipped", "mobs", "interface", true, false, false, "npc_mob"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			first := filepath.Join("assets/sprites", tc.firstRoot, "a/duplicate.png")
			second := filepath.Join("assets/sprites", tc.secondRoot, "z/duplicate.png")
			manifest := assetmanifest.Manifest{}
			for _, candidate := range []struct {
				path    string
				tracked bool
			}{{first, tc.firstTracked}, {second, tc.secondTracked}} {
				if err := os.MkdirAll(filepath.Dir(candidate.path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(candidate.path, []byte("sprite"), 0644); err != nil {
					t.Fatal(err)
				}
				if candidate.tracked {
					rel, err := filepath.Rel("assets", candidate.path)
					if err != nil {
						t.Fatal(err)
					}
					manifest[filepath.ToSlash(rel)] = "shipped-hash"
				}
			}
			if len(manifest) > 0 {
				if err := manifest.Save(dir); err != nil {
					t.Fatal(err)
				}
			}
			want := first
			if tc.wantSecond {
				want = second
			}
			sm := NewSpriteManager()
			if got := sm.determineSpritePaths("duplicate"); got != tc.wantType {
				t.Errorf("placeholder type = %q, want %q", got, tc.wantType)
			}
			if got := sm.spritePaths["duplicate"]; got != want {
				t.Errorf("selected path = %q, want %q", got, want)
			}
			for _, path := range []string{first, second} {
				if _, err := os.Stat(path); err != nil {
					t.Errorf("candidate removed: %v", err)
				}
			}
		})
	}
}
