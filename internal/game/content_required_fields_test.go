package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// TestContentRequiredFields runs the boot-order validators over the REAL
// content files plus the presence rules no validator owns, so a required field
// cannot quietly go missing from tiles, NPCs, or monsters again. Anything that
// fails here would have failed the game boot - this just reports it at test
// time with the offending key.
func TestContentRequiredFields(t *testing.T) {
	cfg := loadTestConfig(t)

	// Tiles + special tiles: the loader validates fail-fast (type, sprite,
	// size_class, render_type, letter conflicts, and the field contracts).
	tm := world.NewTileManager(cfg.Graphics.SizeClasses)
	if err := tm.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("tiles: %v", err)
	}
	if err := tm.LoadSpecialTileConfig("../../assets/special_tiles.yaml"); err != nil {
		t.Fatalf("special tiles: %v", err)
	}

	// NPCs: the same validator chain boot.go runs.
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("npcs: %v", err)
	}
	npcs := character.NPCConfigInstance.NPCs
	if err := ValidateNPCRenderCategories(npcs); err != nil {
		t.Fatal(err)
	}
	if err := ValidateNPCVisualSizes(npcs, cfg.Graphics.SizeClasses); err != nil {
		t.Fatal(err)
	}
	if err := ValidateNPCCommerce(npcs); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDoorNPCs(npcs); err != nil {
		t.Fatal(err)
	}
	for key, npc := range npcs {
		if npc.Name == "" {
			t.Errorf("NPC %q has no name", key)
		}
		// Only a pure interaction anchor may be sprite-less.
		if npc.Sprite == "" && npc.RenderCategory != "invisible" {
			t.Errorf("NPC %q has no sprite but render_category %q is drawn", key, npc.RenderCategory)
		}
	}

	// Monsters: the loader validates letters/size classes; name and sprite
	// presence is owned here.
	monsterCfg, err := monster.LoadMonsterConfig("../../assets/monsters.yaml")
	if err != nil {
		t.Fatalf("monsters: %v", err)
	}
	for key, def := range monsterCfg.Monsters {
		if def.Name == "" {
			t.Errorf("monster %q has no name", key)
		}
		if def.Sprite == "" {
			t.Errorf("monster %q has no sprite", key)
		}
		if def.SizeClass == "" {
			t.Errorf("monster %q has no size_class", key)
		}
	}
}
