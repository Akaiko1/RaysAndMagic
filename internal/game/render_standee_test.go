package game

import (
	"image"
	"math"
	"reflect"
	"testing"
)

// Ray-segment math behind standee tokens: t is the perpendicular depth (ray
// built as dir + plane-s with |dir| = 1), u the position along the token.
func TestStandeeColumnHit(t *testing.T) {
	// Camera at origin looking +X; token crosses the view axis at x=100,
	// spanning y in [-10, +10] (P0 at the bottom, D pointing +Y).
	p0x, p0y, dx, dy := 100.0, -10.0, 0.0, 20.0

	// Center column: straight-ahead ray hits the middle of the token.
	tt, u, ok := standeeColumnHit(0, 0, 1, 0, p0x, p0y, dx, dy)
	if !ok || math.Abs(tt-100) > 1e-9 || math.Abs(u-0.5) > 1e-9 {
		t.Errorf("center: got t=%.3f u=%.3f ok=%v, want t=100 u=0.5", tt, u, ok)
	}

	// Slanted ray r=(1, 0.1): crosses y=10 at x=100 -> the token's far end (u=1).
	tt, u, ok = standeeColumnHit(0, 0, 1, 0.1, p0x, p0y, dx, dy)
	if !ok || math.Abs(tt-100) > 1e-9 || math.Abs(u-1.0) > 1e-9 {
		t.Errorf("edge: got t=%.3f u=%.3f ok=%v, want t=100 u=1", tt, u, ok)
	}

	// Ray missing the segment (crosses beyond its end) -> no hit.
	if _, _, ok = standeeColumnHit(0, 0, 1, 0.2, p0x, p0y, dx, dy); ok {
		t.Errorf("ray past the token end should miss")
	}

	// Ray parallel to the token plane -> no hit (degenerate determinant).
	if _, _, ok = standeeColumnHit(0, 0, 0, 1, p0x, p0y, dx, dy); ok {
		t.Errorf("parallel ray should miss")
	}

	// Token behind the camera -> rejected by the near clip.
	if _, _, ok = standeeColumnHit(200, 0, 1, 0, p0x, p0y, dx, dy); ok {
		t.Errorf("token behind the camera should be rejected")
	}
}

func TestStandeeColumnIntersectionKeepsOffSegmentBoundary(t *testing.T) {
	// A visible edge pixel has a centre ray that hits the segment, while one
	// pixel boundary can intersect its infinite line just outside the segment.
	// The renderer needs that unbounded u to give the quad a non-zero horizontal
	// texture footprint, allowing Ebiten to select a minified mip level.
	p0x, p0y, dx, dy := 100.0, -10.0, 0.0, 20.0
	tt, u, ok := standeeColumnIntersection(0, 0, 1, 0.11, p0x, p0y, dx, dy)
	if !ok || math.Abs(tt-100) > 1e-9 || u <= 1 {
		t.Fatalf("unbounded edge: got t=%.3f u=%.3f ok=%v; want t=100 and u>1", tt, u, ok)
	}
	if _, _, ok := standeeColumnHit(0, 0, 1, 0.11, p0x, p0y, dx, dy); ok {
		t.Fatal("finite segment hit must reject the off-segment boundary")
	}
}

func TestStandeeUsesMinificationSampling(t *testing.T) {
	if standeeUsesMinificationSampling(512, 512, 512, 512) {
		t.Fatal("1:1 standee must retain the crisp path")
	}
	if !standeeUsesMinificationSampling(512, 511.9, 512, 512) {
		t.Fatal("any vertical shrink must use stable linear sampling")
	}
	if !standeeUsesMinificationSampling(511.9, 512, 512, 512) {
		t.Fatal("any horizontal shrink must use stable linear sampling")
	}
}

func TestStandeeTextureFootprint(t *testing.T) {
	if got := standeeTextureFootprint(0.5, 1); got != 1 {
		t.Fatalf("magnified source footprint = %.2f, want 1", got)
	}
	if got := standeeTextureFootprint(2, -1); got != 2 {
		t.Fatalf("reversed source footprint = %.2f, want 2", got)
	}
	if got := standeeTextureFootprint(2, 1e-7); got != 1 {
		t.Fatalf("degenerate destination footprint = %.2f, want 1", got)
	}
	if got := standeeTextureFootprint(1, 1); got != 1 {
		t.Fatalf("1:1 source footprint = %.2f, want 1", got)
	}
	if got := standeeTextureFootprint(2, 1); got != 2 {
		t.Fatalf("2:1 source footprint = %.2f, want 2", got)
	}
	if got := standeeTextureFootprint(8, 2); got != 4 {
		t.Fatalf("4:1 source footprint = %.2f, want 4", got)
	}
}

func TestStandeeMipBlendIsContinuousAcrossLevels(t *testing.T) {
	tests := []struct {
		name      string
		footprint float32
		wantLevel int
		wantBlend float32
	}{
		{name: "magnified", footprint: 0.5, wantLevel: 0, wantBlend: 0},
		{name: "one to one", footprint: 1, wantLevel: 0, wantBlend: 0},
		{name: "pure lower mip", footprint: float32(math.Pow(2, 0.25)), wantLevel: 0, wantBlend: 0},
		{name: "halfway to level one", footprint: float32(math.Sqrt2), wantLevel: 0, wantBlend: 0.5},
		{name: "pure upper mip", footprint: float32(math.Pow(2, 0.75)), wantLevel: 1, wantBlend: 0},
		{name: "level one", footprint: 2, wantLevel: 1, wantBlend: 0},
		{name: "halfway to level two", footprint: float32(2 * math.Sqrt2), wantLevel: 1, wantBlend: 0.5},
		{name: "clamped", footprint: 256, wantLevel: standeeMaxMipLevel, wantBlend: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, blend := standeeMipBlend(tt.footprint, standeeMaxMipLevel)
			if level != tt.wantLevel || math.Abs(float64(blend-tt.wantBlend)) > 1e-5 {
				t.Fatalf("standeeMipBlend(%g) = (%d, %.4f), want (%d, %.4f)", tt.footprint, level, blend, tt.wantLevel, tt.wantBlend)
			}
		})
	}
}

func TestStandeeMipSizesMatchEngineDepthCap(t *testing.T) {
	got := standeeMipSizes(128, 64)
	want := []image.Point{
		image.Pt(128, 64), image.Pt(64, 32), image.Pt(32, 16),
		image.Pt(16, 8), image.Pt(8, 4), image.Pt(4, 2), image.Pt(2, 1),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("standeeMipSizes(128, 64) = %v, want %v", got, want)
	}
}

func TestWallMountedDepthAllowanceOnlyCoversBackingWall(t *testing.T) {
	allowance := wallMountedDepthAllowanceWorld(64, 0.02)
	if got, want := allowance, 2.64; math.Abs(got-want) > 1e-9 {
		t.Fatalf("wall-mounted allowance = %.2f, want %.2f", got, want)
	}
	if standeeColumnOccluded(100, 100, allowance) {
		t.Fatal("the backing wall must not occlude its own mounted standee")
	}
	if !standeeColumnOccluded(100, 95, allowance) {
		t.Fatal("a foreground wall 5px closer must occlude the mounted standee")
	}
}

func TestWallMountedBackingWallMatchIsPlaneSpecific(t *testing.T) {
	// A north-south backing wall at X=100: its standee runs along Y, so the
	// wall-stick yaw is pi/2. The centre ray sees that plane at depth 100.
	occlusion := standeeWallOcclusion{
		backingX:       100,
		backingY:       0,
		backingYaw:     math.Pi / 2,
		hasBackingWall: true,
	}
	if !occlusion.matchesBackingWall(0, 0, 1, 0, 100) {
		t.Fatal("the exact backing wall plane must be recognized")
	}
	if occlusion.matchesBackingWall(0, 0, 1, 0, 95) {
		t.Fatal("a different foreground wall plane must not inherit backing-wall immunity")
	}
}

// clampYawFromEdgeOn keeps monster tokens readable: the slab plane never
// comes closer than minRad to the camera's sight line (slabs have period pi).
func TestClampYawFromEdgeOn(t *testing.T) {
	const min = 0.35
	// Dead-on edge view (yaw aligned with the sight line) -> pushed out to +min.
	if got := clampYawFromEdgeOn(1.0, 1.0, min); math.Abs(got-(1.0+min)) > 1e-9 {
		t.Errorf("edge-on: got %.4f want %.4f", got, 1.0+min)
	}
	// Slightly negative side -> pushed to -min, preserving the side.
	if got := clampYawFromEdgeOn(1.0-0.1, 1.0, min); math.Abs(got-(1.0-min)) > 1e-9 {
		t.Errorf("neg side: got %.4f want %.4f", got, 1.0-min)
	}
	// Outside the dead zone -> untouched.
	if got := clampYawFromEdgeOn(2.0, 1.0, min); got != 2.0 {
		t.Errorf("outside: got %.4f want 2.0", got)
	}
	// Slab period pi: a yaw offset by ~pi from the sight line is still edge-on.
	if got := clampYawFromEdgeOn(1.0+math.Pi+0.05, 1.0, min); math.Abs(math.Mod(got-1.0, math.Pi)-min) > 1e-9 {
		t.Errorf("period: got %.4f, deviation should clamp to %.2f", got, min)
	}
}

// approachAngle drives the smooth token swivel: shortest arc, capped step.
func TestApproachAngle(t *testing.T) {
	// Within the step cap -> lands on the target exactly.
	if got := approachAngle(0, 0.1, 0.5); math.Abs(got-0.1) > 1e-9 {
		t.Errorf("small diff: got %.4f want 0.1", got)
	}
	// Beyond the cap -> advances by exactly the cap.
	if got := approachAngle(0, 2.0, 0.5); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("capped: got %.4f want 0.5", got)
	}
	// Shortest arc across the +/-pi seam: from 3.0 to -3.0 is +0.28 rad forward,
	// not -6.0 backward.
	got := approachAngle(3.0, -3.0, 10)
	want := 3.0 + (2*math.Pi - 6.0)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("seam: got %.4f want %.4f", got, want)
	}
	// And it must move in the negative direction when that's shorter.
	if got := approachAngle(-3.0, 3.0, 0.1); math.Abs(got-(-3.1)) > 1e-9 {
		t.Errorf("negative arc: got %.4f want -3.1", got)
	}
}
