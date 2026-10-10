package game

import (
	"gopkg.in/yaml.v3"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/spritecatalog"
	"ugataima/internal/world"
)

func TestAtlasStartupCatalogAndFallbacks(t *testing.T) {
	oldStyle, oldNPCs, oldTiles := loadedAtlasStyle, character.NPCConfigInstance, world.GlobalTileManager
	t.Cleanup(func() {
		loadedAtlasStyle, character.NPCConfigInstance, world.GlobalTileManager = oldStyle, oldNPCs, oldTiles
	})
	cfg := setupPreviewSandboxTest(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatal(err)
	}
	path, err := filepath.Abs("../../assets/world_atlas.yaml")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir("../..")
	paths, _ := spritecatalog.BuildIndex()
	t.Chdir(t.TempDir())
	if err := LoadWorldAtlas(path, paths); err != nil {
		t.Fatal(err)
	}
	// Game creation consumes the loaded catalog and does no atlas file/index IO.
	p, err := NewFxPreview(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.g.Shutdown)
	ui := NewUISystem(p.g)
	for _, tc := range []struct {
		name string
		npc  character.NPC
		want string
	}{
		{"prop", character.NPC{Type: character.NPCTypeLootCrate, RenderCategory: "scenery", PropModel: &character.NPCPropModel{Shape: "campfire"}}, loadedAtlasStyle.PropShapes["campfire"]},
		{"type", character.NPC{Type: character.NPCTypeLootCrate, RenderCategory: "scenery"}, loadedAtlasStyle.NPCTypes[character.NPCTypeLootCrate]},
		{"category", character.NPC{Type: character.NPCTypeQuestGiver, RenderCategory: "npc"}, loadedAtlasStyle.RenderCategories["npc"]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ui.atlasNPCIcon(&tc.npc); got == "" || got != tc.want {
				t.Fatalf("icon %q want %q", got, tc.want)
			}
		})
	}
	// Fallback icons are just as load-bearing as explicitly listed NPC icons.
	for _, icon := range []string{loadedAtlasStyle.PropShapes["campfire"], loadedAtlasStyle.NPCTypes[character.NPCTypeSpellLectern], loadedAtlasStyle.RenderCategories["scenery"], loadedAtlasStyle.PortalIcon} {
		pathValue := paths[icon]
		delete(paths, icon)
		if err := LoadWorldAtlas(path, paths); err == nil || !strings.Contains(err.Error(), icon) {
			t.Fatalf("missing fallback %s was accepted: %v", icon, err)
		}
		paths[icon] = pathValue
	}
	for _, group := range []map[string]string{loadedAtlasStyle.PropShapes, loadedAtlasStyle.NPCTypes, loadedAtlasStyle.RenderCategories, loadedAtlasStyle.LegendIcons} {
		for key, old := range group {
			group[key] = "atlas_missing_fixture"
			data, err := yaml.Marshal(loadedAtlasStyle)
			group[key] = old
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(t.TempDir(), "atlas.yaml")
			if err := os.WriteFile(file, data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := LoadWorldAtlas(file, paths); err == nil || !strings.Contains(err.Error(), "atlas_missing_fixture") {
				t.Fatalf("missing %s fallback was accepted: %v", key, err)
			}
			break
		}
	}
}

func TestAtlasRegionlessLayoutAndReopenCache(t *testing.T) {
	h := newDisplayedModalHarness(t, 1280, 960)
	g, ui := h.g, h.ui
	g.mapOverlayOpen = true
	base := computeMapOverlayLayout(1280, gameplayViewportBottom(g))
	if base.index.w == 0 {
		t.Fatal("fixture cannot expose unused index")
	}
	l := ui.mapOverlayLayout()
	if l.index.w != 0 || l.body.right() != base.index.right() {
		t.Fatalf("regionless layout wastes index: %+v", l)
	}
	wm := world.GlobalWorldManager
	wm.OpenWorld = g.world
	wm.OpenWorldRegions = []world.OpenWorldRegion{{MapKey: "forest", Width: g.world.Width, Height: g.world.Height}}
	if ui.mapOverlayLayout().index.w == 0 {
		t.Fatal("open-world index disappeared")
	}
	ui.Draw(h.screen)
	before := ui.atlas.terrain
	beforeHash := ui.atlas.hash
	if before == nil || !ui.atlas.terrainChecked {
		t.Fatal("terrain was not cached")
	}
	ui.Draw(h.screen)
	if ui.atlas.terrain != before {
		t.Fatal("unchanged modal rebuilt terrain")
	}
	g.mapOverlayOpen = false
	ui.Draw(h.screen)
	g.world.Tiles[1][1] = world.TileWater
	g.mapOverlayOpen = true
	ui.Draw(h.screen)
	if ui.atlas.hash == beforeHash {
		t.Fatal("reopening did not refresh changed terrain")
	}
	// Zoom changes bucket density; pan only projects the same retained marks.
	ui.atlas.tileMarks = []atlasTileMark{{image.Pt(1, 1), "a"}, {image.Pt(2, 1), "a"}}
	ui.atlas.bucketStride = 0
	p := atlasProjection{bounds: image.Rect(0, 0, 40, 40), scale: 1}
	ui.drawAtlasTileIcons(h.screen, p)
	if len(ui.atlas.bucketMarks) != 1 {
		t.Fatal("overview did not consolidate nearby symbols")
	}
	p.scale = 20
	ui.drawAtlasTileIcons(h.screen, p)
	if len(ui.atlas.bucketMarks) != 2 {
		t.Fatal("zoom did not reveal both authored marks")
	}
}
