package game

import (
	"math"
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
