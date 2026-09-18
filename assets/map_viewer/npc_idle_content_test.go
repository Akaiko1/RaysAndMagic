package main

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
)

// Editor map and palette thumbnails share firstFrame; keep the new idle sheet
// readable as one square instead of shrinking the whole four-pose strip.
func TestSafiyaIdleThumbnail(t *testing.T) {
	t.Chdir("../..")
	if _, err := config.LoadSpellConfig("assets/spells.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := character.LoadNPCConfig("assets/npcs.yaml"); err != nil {
		t.Fatal(err)
	}
	sprite := npcSpriteForKey("nomad_city_safiya")
	if sprite == "" {
		t.Fatal("Safiya has no configured sprite")
	}
	sheet := graphics.NewSpriteManager().GetSprite(sprite)
	if sheet.Bounds().Dx() != 4*sheet.Bounds().Dy() {
		t.Fatalf("Safiya must use the NPC idle sheet layout: %v", sheet.Bounds())
	}
	frame := firstFrame(sheet)
	if frame.Bounds().Min != sheet.Bounds().Min || frame.Bounds().Dx() != 512 || frame.Bounds().Dy() != 512 {
		t.Fatalf("editor thumbnail did not select the first square frame: %v", frame.Bounds())
	}
}
