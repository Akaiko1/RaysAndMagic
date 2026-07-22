package game

import (
	"math"
	"testing"

	"ugataima/internal/world"
)

func TestWallRayHitAtGridLineMatchesWallPlane(t *testing.T) {
	// Camera at (.5, .25), looking along +X. It meets the x=2 wall plane
	// after 1.5 grid units; its Y hit coordinate is .625 and the +X face is
	// mirrored, giving U=.375.
	distance, u, ok := wallRayHitAtGridLine(0.5, 0.25, 1, 0.25, 0, 2)
	if !ok {
		t.Fatal("expected forward vertical wall-plane hit")
	}
	if math.Abs(distance-1.5) > 1e-9 {
		t.Fatalf("distance = %.9f, want 1.5", distance)
	}
	if math.Abs(u-0.375) > 1e-9 {
		t.Fatalf("mirrored U = %.9f, want .375", u)
	}

	// A horizontal wall approached from negative Y uses the matching mirror
	// rule. The exact line coordinate belongs to the final source texel, not a
	// nonexistent x=textureWidth sample.
	_, u, ok = wallRayHitAtGridLine(0.25, 0.5, -0.5, -1, 1, 0)
	if !ok {
		t.Fatal("expected forward horizontal wall-plane hit")
	}
	if u <= 0.999999 || u >= 1 {
		t.Fatalf("exact mirrored seam U = %.12f, want final in-range texel", u)
	}
}

func TestWallTextureIntervalFromSurfacePreservesRealFootprint(t *testing.T) {
	left, right := wallTextureIntervalFromSurface(10.99, 11.01, false)
	if math.Abs(left-0.99) > 1e-9 || math.Abs(right-1.01) > 1e-9 {
		t.Fatalf("seam interval = (%.3f, %.3f), want (.99, 1.01)", left, right)
	}
	left, right = wallTextureIntervalFromSurface(10.2, 10.8, false)
	if math.Abs(left-0.2) > 1e-9 || math.Abs(right-0.8) > 1e-9 {
		t.Fatalf("wide interval = (%.3f, %.3f), want (.2, .8)", left, right)
	}
	left, right = wallTextureIntervalFromSurface(10.01, 9.99, true)
	if math.Abs(left-0.99) > 1e-9 || math.Abs(right-1.01) > 1e-9 {
		t.Fatalf("mirrored seam interval = (%.3f, %.3f), want (.99, 1.01)", left, right)
	}
}

func TestWallTextureUsesMipmappedSliceOnlyWhenMinified(t *testing.T) {
	if wallTextureUsesMipmappedSlice(256, 256, 1, 0.25, 0.25+1.0/256.0, 128) {
		t.Fatal("one source texel per pixel at half height must retain nearest wall art")
	}
	if !wallTextureUsesMipmappedSlice(256, 256, 1, 0.25, 0.25+2.0/256.0, 128) {
		t.Fatal("two horizontal source texels per pixel must use the mipmapped path")
	}
	if !wallTextureUsesMipmappedSlice(256, 256, 1, 0.25, 0.25+1.0/256.0, 127.9) {
		t.Fatal("vertically minified wall must use the mipmapped path")
	}
	if wallTextureUsesMipmappedSlice(256, 256, 2, 0.25, 0.25+2.0/256.0, 128) {
		t.Fatal("two source texels over a two-pixel ray must retain nearest wall art")
	}
}

func TestDDARaycastCarriesWallGridLineForDistantTextureSampling(t *testing.T) {
	game, _, tileSize := tbBehaviorGame(t, 6, 4)
	previousWorldManager := world.GlobalWorldManager
	world.GlobalWorldManager = nil
	t.Cleanup(func() { world.GlobalWorldManager = previousWorldManager })

	game.camera.X = 1.25 * tileSize
	game.camera.Y = 1.25 * tileSize
	game.camera.ViewDist = game.config.GetViewDistance()
	game.world.Tiles[1][3] = world.TileWall
	r := &Renderer{game: game}

	hits := r.performMultiHitRaycastWithDirection(1, 0, nil)
	if len(hits.Hits) != 1 {
		t.Fatalf("got %d DDA hits, want one wall", len(hits.Hits))
	}
	hit := hits.Hits[0]
	if !hit.HasWallGridLine || hit.WallGridLine != 3 {
		t.Fatalf("wall plane = (%v, %.3f), want (true, 3)", hit.HasWallGridLine, hit.WallGridLine)
	}
	if hit.WallSide != 0 {
		t.Fatalf("wall side = %d, want vertical side 0", hit.WallSide)
	}
	wantDistance, wantU, ok := wallRayHitAtGridLine(1.25, 1.25, 1, 0, 0, 3)
	if !ok {
		t.Fatal("reference wall-plane ray unexpectedly missed")
	}
	if math.Abs(hit.Distance-wantDistance*tileSize) > 1e-9 || math.Abs(hit.TextureCoord-wantU) > 1e-9 {
		t.Fatalf("DDA hit = (distance %.9f, U %.9f), want (%.9f, %.9f)", hit.Distance, hit.TextureCoord, wantDistance*tileSize, wantU)
	}
}
