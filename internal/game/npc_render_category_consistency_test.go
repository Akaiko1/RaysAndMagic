package game

import (
	"image"
	_ "image/png"
	"os"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
)

// TestNPCRenderCategorySpriteConsistency pins the invariant that a NPC's
// explicit render_category agrees with its sprite: an "animated" NPC must be a
// w==h*SpriteSheetFrameCount sheet, and a "standee" must NOT be (else it should
// be "animated"). This guards the one-time seed AND catches a future hand-edit
// that declares a category the sprite can't back - the fail-fast the explicit
// field is meant to provide.
func TestNPCRenderCategorySpriteConsistency(t *testing.T) {
	t.Chdir("../..") // ResolveSpritePath is repo-root relative
	if _, err := config.LoadConfig("config.yaml"); err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := config.LoadSpellConfig("assets/spells.yaml"); err != nil {
		t.Fatalf("spells: %v", err)
	}
	if err := character.LoadNPCConfig("assets/npcs.yaml"); err != nil {
		t.Fatalf("npcs: %v", err)
	}

	dims := func(sprite string) (int, int) {
		if sprite == "" {
			return 0, 0
		}
		path, ok := graphics.ResolveSpritePath(sprite)
		if !ok {
			return 0, 0
		}
		f, err := os.Open(path)
		if err != nil {
			return 0, 0
		}
		defer f.Close()
		cfg, _, err := image.DecodeConfig(f)
		if err != nil {
			return 0, 0
		}
		return cfg.Width, cfg.Height
	}

	for key, data := range character.NPCConfigInstance.NPCs {
		w, h := dims(data.Sprite)
		cat := resolveNPCRenderCat(data.RenderCategory, data.Sprite, data.RenderType, data.WallMounted, w, h)
		is4 := h > 0 && w == h*SpriteSheetFrameCount
		switch cat {
		case catAnimated:
			if !is4 {
				t.Errorf("NPC %q render_category=animated but sprite %q is not a %d-frame sheet (%dx%d)",
					key, data.Sprite, SpriteSheetFrameCount, w, h)
			}
		case catStandee:
			if is4 {
				t.Errorf("NPC %q render_category=standee but sprite %q IS a %d-frame sheet (%dx%d) - should be animated",
					key, data.Sprite, SpriteSheetFrameCount, w, h)
			}
		}
	}
}
