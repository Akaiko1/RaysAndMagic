package graphics

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestAnimationFrameCountUsesLoaderLayoutWithoutUploading(t *testing.T) {
	for _, tc := range []struct {
		name                string
		width, height, want int
	}{
		{"square", 16, 16, 4}, {"strip", 30, 10, 3}, {"invalid", 11, 10, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "sprite.png")
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, image.NewRGBA(image.Rect(0, 0, tc.width, tc.height)))
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			sm := NewSpriteManager()
			sm.spritePaths = map[string]string{"mob_dying_l": path}
			for i := 0; i < 2; i++ {
				if got := sm.AnimationFrameCount("mob", "dying_l"); got != tc.want {
					t.Fatalf("count=%d want %d", got, tc.want)
				}
				if len(sm.animations) != 0 || len(sm.sprites) != 0 {
					t.Fatal("metadata lookup uploaded a texture")
				}
			}
			if sm.AnimationFrameCount("mob", "dying_r") != 0 {
				t.Fatal("missing direction was reported present")
			}
		})
	}
}
