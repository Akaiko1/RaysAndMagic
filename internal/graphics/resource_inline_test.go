package graphics

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestResourcePixelBudget(t *testing.T) {
	for _, tc := range []struct {
		name                        string
		width, height, limit, want  int
		corrupt, missing, animation bool
	}{
		{name: "exact", width: 256, height: 256, limit: 256 << 10, want: 256 << 10},
		{name: "over", width: 257, height: 256, limit: 256 << 10},
		{name: "frame_remaining", width: 64, height: 64, limit: 64*64*4 - 1},
		{name: "exhausted", width: 64, height: 64},
		{name: "animation", width: 64, height: 16, limit: 64 << 10, want: 4096, animation: true},
		{name: "corrupt", limit: 256 << 10, corrupt: true},
		{name: "missing", limit: 256 << 10, missing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sm := NewSpriteManager()
			path := filepath.Join(t.TempDir(), "source.png")
			if !tc.missing {
				f, err := os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				if tc.corrupt {
					_, err = f.WriteString("not a PNG")
				} else {
					err = png.Encode(f, image.NewRGBA(image.Rect(0, 0, tc.width, tc.height)))
				}
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			request := SpriteResourceRequest{Name: "source"}
			key := "source"
			if tc.animation {
				request.AnimationType = "walking_r"
				key += "_walking_r"
			}
			sm.spritePaths = map[string]string{key: path}
			if got := sm.ResourcePixelBytesWithin(request, tc.limit); got != tc.want {
				t.Fatalf("pixel bytes=%d want=%d", got, tc.want)
			}
			if len(sm.ResourceImages(request)) != 0 {
				t.Fatal("header inspection published a GPU source")
			}
		})
	}
}
