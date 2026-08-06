package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/world"
)

func TestWorldVisualSizeContentUsesSharedClasses(t *testing.T) {
	cfg := loadTestConfig(t)
	for _, class := range config.VisualSizeClassNames() {
		if got, ok := config.ResolveSizeClassTiles(cfg.Graphics.SizeClasses, class); !ok || got <= 0 {
			t.Errorf("graphics.size_classes.%s = %v (found=%v), want a positive value", class, got, ok)
		}
	}

	previousTiles := world.GlobalTileManager
	t.Cleanup(func() { world.GlobalTileManager = previousTiles })
	tm := world.NewTileManager(cfg.Graphics.SizeClasses)
	if err := tm.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	world.GlobalTileManager = tm

	for _, key := range tm.GetAllTileKeys() {
		data := tm.GetTileDataByKey(key)
		if data == nil {
			continue
		}
		if data.RemovedSizeTiles != nil {
			t.Errorf("tile %q still authors size_tiles", key)
		}
		if data.RenderType != config.TileRenderCrossedStandee {
			continue
		}
		if !config.IsCrossedStandeeSizeClass(data.SizeClass) {
			t.Errorf("crossed standee %q has incompatible size_class %q", key, data.SizeClass)
		}
		tileType, ok := tm.GetTileTypeFromKey(key)
		if !ok {
			t.Errorf("tree tile %q has no runtime type", key)
			continue
		}
		if got := tm.GetSizeTiles(tileType); got <= 0 {
			t.Errorf("crossed standee %q width = %v, want a positive resolved class", key, got)
		}
	}

	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load NPCs: %v", err)
	}
	if err := ValidateNPCRenderCategories(character.NPCConfigInstance.NPCs); err != nil {
		t.Fatal(err)
	}
	if err := ValidateNPCVisualSizes(character.NPCConfigInstance.NPCs, cfg.Graphics.SizeClasses); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"seabright_gate", "silverbough_gate", "dunehold_gate"} {
		npc := character.NPCConfigInstance.NPCs[key]
		if npc == nil {
			t.Errorf("city %q is missing", key)
			continue
		}
		if got, ok := config.ResolveSizeClassTiles(cfg.Graphics.SizeClasses, npc.SizeClass); !ok || got <= 0 || !config.IsPropSizeClass(npc.SizeClass) {
			t.Errorf("city %q class %q resolves to %v (found=%v), want a positive prop class", key, npc.SizeClass, got, ok)
		}
	}
}
