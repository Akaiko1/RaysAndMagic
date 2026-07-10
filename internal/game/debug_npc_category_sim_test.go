package game

// Headless NPC-category diagnostic - a DEBUG MODULE, not a regression test.
// Loads the REAL npcs.yaml, checks each NPC's sprite resolves via stdlib image
// decode (no render context needed), and prints the display-category buckets
// the map editor's `@` palette will group by, so the real distribution can be
// eyeballed.
//
// Run with:  RAM_DEBUG_SIM=1 go test ./internal/game/ -run TestDebugSim_NPCCategories -v

import (
	"fmt"
	"image"
	_ "image/png"
	"os"
	"sort"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
)

func TestDebugSim_NPCCategories(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	t.Chdir("../..")

	if _, err := config.LoadConfig("config.yaml"); err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := config.LoadSpellConfig("assets/spells.yaml"); err != nil {
		t.Fatalf("spells: %v", err)
	}
	if err := character.LoadNPCConfig("assets/npcs.yaml"); err != nil {
		t.Fatalf("npcs: %v", err)
	}

	// Stdlib decode of just the image header (W/H) - no ebiten/GPU.
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

	byCat := map[string][]string{}
	unresolved := 0
	for key, data := range character.NPCConfigInstance.NPCs {
		sprite, rc := "", ""
		if data != nil {
			sprite, rc = data.Sprite, data.RenderCategory
		}
		w, h := dims(sprite)
		if sprite != "" && sprite != "none" && (w == 0 || h == 0) {
			unresolved++
			t.Logf("  WARN sprite %q for npc %q did not resolve", sprite, key)
		}
		cat := NPCDisplayCategory(rc)
		byCat[cat] = append(byCat[cat], key)
	}

	t.Logf("--- NPC display categories (real npcs.yaml, %d NPCs, %d unresolved sprites) ---",
		len(character.NPCConfigInstance.NPCs), unresolved)
	for _, cat := range NPCDisplayCategoryOrder {
		keys := byCat[cat]
		if len(keys) == 0 {
			continue
		}
		sort.Strings(keys)
		t.Logf("[%s] (%d): %v", cat, len(keys), keys)
	}
	// Any category returned but not in the canonical order (future branch).
	for cat, keys := range byCat {
		found := false
		for _, c := range NPCDisplayCategoryOrder {
			if c == cat {
				found = true
				break
			}
		}
		if !found {
			sort.Strings(keys)
			t.Logf("[%s] (%d, NOT in NPCDisplayCategoryOrder): %v", cat, len(keys), keys)
		}
	}
	fmt.Println()
}
