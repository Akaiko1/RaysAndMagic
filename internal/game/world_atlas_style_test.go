package game

import (
	"image"
	"testing"

	"ugataima/internal/world"
)

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
