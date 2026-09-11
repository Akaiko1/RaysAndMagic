package storage

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/graphics"
)

func TestSeedReconcilesRetiredAssets(t *testing.T) {
	for _, tc := range []struct {
		name, path           string
		edited, custom, keep bool
	}{
		{"sprite", "sprites/environment/old.png", false, false, false},
		{"edited_shipped_sprite", "sprites/environment/old.png", true, false, false},
		{"map", "old.map", false, false, false},
		{"edited_map", "old.map", true, false, true},
		{"custom_sprite", "sprites/environment/custom.png", false, true, true},
		{"custom_map", "custom.map", false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			content, user := t.TempDir(), t.TempDir()
			writeFile(t, filepath.Join(content, "config.yaml"), "v1")
			writeFile(t, filepath.Join(content, "assets/retained.yaml"), "retained")
			src, dst := filepath.Join(content, "assets", tc.path), filepath.Join(user, "assets", tc.path)
			if !tc.custom {
				writeFile(t, src, "shipped")
			}
			if err := seedUserData(content, user); err != nil {
				t.Fatal(err)
			}
			if tc.edited || tc.custom {
				writeFile(t, dst, "user content")
			}
			if !tc.custom {
				if err := os.Remove(src); err != nil {
					t.Fatal(err)
				}
			}
			writeFile(t, filepath.Join(content, "config.yaml"), "v2")
			if err := seedUserData(content, user); err != nil {
				t.Fatal(err)
			}
			_, err := os.Stat(dst)
			if (err == nil) != tc.keep {
				t.Fatalf("retired asset exists=%v, want %v (error %v)", err == nil, tc.keep, err)
			}
			if tc.keep && read(t, dst) != "user content" {
				t.Fatal("custom content overwritten")
			}
			if _, tracked := loadSeedManifest(user)[tc.path]; tracked {
				t.Fatal("retired file is still updater-owned")
			}
		})
	}
}

func TestSeedMovedSpriteResolvesReplacement(t *testing.T) {
	content, user := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(content, "config.yaml"), "test")
	oldRel, newRel := "assets/sprites/environment/a/review_moved.png", "assets/sprites/environment/z/review_moved.png"
	writeFile(t, filepath.Join(content, oldRel), "old sprite")
	if err := seedUserData(content, user); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(content, oldRel)); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(content, newRel), "new sprite")
	if err := seedUserData(content, user); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(user, oldRel)); !os.IsNotExist(err) {
		t.Fatalf("retired shipped sprite was not removed: %v", err)
	}
	t.Chdir(user)
	if resolved, ok := graphics.ResolveSpritePath("review_moved"); !ok || resolved != newRel {
		t.Fatalf("moved sprite resolves to %q (found %v), want %q", resolved, ok, newRel)
	}
}

func TestSeedLegacySpriteDoesNotShadowCurrentBundle(t *testing.T) {
	content, user := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(content, "config.yaml"), "cfg")
	writeFile(t, filepath.Join(content, "assets/forest.map"), "shipped map")
	hash, _ := fileSHA256(filepath.Join(content, "assets/forest.map"))
	if err := (seedManifest{"forest.map": hash}).Save(user); err != nil {
		t.Fatal(err)
	}
	oldRel := "assets/sprites/environment/a/legacy_moved.png"
	newRel := "assets/sprites/environment/z/legacy_moved.png"
	oldColor, newColor := color.RGBA{R: 255, A: 255}, color.RGBA{G: 255, A: 255}
	for _, fixture := range []struct {
		path  string
		pixel color.RGBA
	}{
		{filepath.Join(user, oldRel), oldColor},
		{filepath.Join(content, newRel), newColor},
	} {
		img := image.NewRGBA(image.Rect(0, 0, 2, 2))
		img.SetRGBA(0, 0, fixture.pixel)
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, img); err != nil {
			t.Fatal(err)
		}
		writeFile(t, fixture.path, encoded.String())
	}
	oldBytes := read(t, filepath.Join(user, oldRel))
	if err := seedUserData(content, user); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(user, oldRel)) != oldBytes {
		t.Fatal("untracked legacy sprite was altered")
	}
	t.Chdir(user)
	sm := graphics.NewSpriteManager()
	results := sm.PrepareResources(context.Background(), []graphics.SpriteResourceRequest{{Name: "legacy_moved"}})
	prepared := <-results
	if !prepared.Found || prepared.Image == nil {
		t.Fatal("current shipped sprite did not decode")
	}
	if got := color.RGBAModel.Convert(prepared.Image.At(0, 0)); got != newColor {
		t.Fatalf("decoded legacy pixel %v, want current shipped pixel %v", got, newColor)
	}
	for range results {
		t.Fatal("unexpected additional resource")
	}
}

func TestSeedMigratesMapsOnlyManifest(t *testing.T) {
	content, user := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(content, "config.yaml"), "cfg")
	writeFile(t, filepath.Join(content, "assets/forest.map"), "shipped")
	writeFile(t, filepath.Join(content, "assets/sprite.png"), "sprite")
	writeFile(t, filepath.Join(user, "assets/forest.map"), "edited")
	hash, _ := fileSHA256(filepath.Join(content, "assets/forest.map"))
	if err := (seedManifest{"forest.map": hash}).Save(user); err != nil {
		t.Fatal(err)
	}
	digest, err := shippedContentDigest(content)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(user, seedStateName), "0 "+digest)
	if err := seedUserData(content, user); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(user, "assets/forest.map")) != "edited" {
		t.Fatal("migration overwrote edited unchanged map")
	}
	if loadSeedManifest(user)["sprite.png"] == "" {
		t.Fatal("unchanged bundle did not migrate to a complete manifest")
	}
}
