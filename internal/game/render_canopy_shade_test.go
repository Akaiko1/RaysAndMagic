package game

import (
	"math"
	"testing"
)

func TestCanopyShadeFactorForDensity(t *testing.T) {
	tests := []struct {
		name    string
		density int
		want    float64
	}{
		{name: "below start remains daylight", density: 1, want: 1.0},
		{name: "at start remains daylight", density: 2, want: 1.0},
		{name: "middle interpolates", density: 5, want: 0.85},
		{name: "full density reaches minimum", density: 8, want: 0.7},
		{name: "above full density clamps", density: 12, want: 0.7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canopyShadeFactorForDensity(tt.density, 0.7, 2, 8)
			if got != tt.want {
				t.Fatalf("canopyShadeFactorForDensity(%d) = %v, want %v", tt.density, got, tt.want)
			}
		})
	}
}

func TestCanopyAmbientUsesSurfaceAndViewerShade(t *testing.T) {
	tests := []struct {
		name         string
		mapAmbient   float64
		surfaceShade float64
		viewerShade  float64
		want         float64
	}{
		{name: "open map remains daylight", mapAmbient: 1, surfaceShade: 1, viewerShade: 1, want: 1},
		{name: "surface shade darkens target point", mapAmbient: 1, surfaceShade: 0.7, viewerShade: 1, want: 0.7},
		{name: "viewer shade darkens whole scene", mapAmbient: 1, surfaceShade: 1, viewerShade: 0.7, want: 0.7},
		{name: "deeper of surface and viewer shade wins, not both", mapAmbient: 1, surfaceShade: 0.8, viewerShade: 0.7, want: 0.7},
		// Shade composes multiplicatively with ambient, so canopy contrast
		// scales with the day/night light level (night forest: 0.7 * 0.7).
		{name: "night ambient deepens under-canopy shade", mapAmbient: 0.7, surfaceShade: 0.7, viewerShade: 1, want: 0.7 * 0.7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canopyAmbient(tt.mapAmbient, tt.surfaceShade, tt.viewerShade)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("canopyAmbient(%v, %v, %v) = %v, want %v",
					tt.mapAmbient, tt.surfaceShade, tt.viewerShade, got, tt.want)
			}
		})
	}
}

// localAmbientAt must apply the map ambient exactly ONCE. The viewer factor it
// feeds into canopyAmbient is the pure smoothed shade - if it were the already
// composed viewerAmbient, a night forest (ambient 0.7, shade 0.7) would light
// CPU sprites at 0.7*0.49=0.343 while the floor shader gets 0.49.
func TestLocalAmbientAppliesMapAmbientOnce(t *testing.T) {
	cfg := loadTestConfig(t)
	r := &Renderer{
		game:         &MMGame{config: cfg},
		ambientLight: 0.7,
		canopyShadeFactors: []float64{
			0.7, 0.7,
			0.7, 0.7,
		},
		canopyShadeW: 2,
		canopyShadeH: 2,
	}
	// No camera: viewer shade resolves to 1, so the only factors are the map
	// ambient and the surface shade.
	tileSize := float64(cfg.GetTileSize())
	got := r.localAmbientAt(tileSize/2, tileSize/2)
	want := 0.7 * 0.7
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("localAmbientAt = %v, want %v (ambient x surface shade, applied once)", got, want)
	}
	// And the floor-shader value composes the same way: ambient x viewer shade.
	if got := r.viewerAmbient(); math.Abs(got-0.7) > 1e-9 {
		t.Fatalf("viewerAmbient with shade 1 = %v, want map ambient 0.7", got)
	}
}

func TestCanopyShadeFactorAtInterpolatesBetweenTiles(t *testing.T) {
	cfg := loadTestConfig(t)
	r := &Renderer{
		game: &MMGame{config: cfg},
		canopyShadeFactors: []float64{
			1.0, 0.7,
			1.0, 0.7,
		},
		canopyShadeW: 2,
		canopyShadeH: 2,
	}

	tileSize := float64(cfg.GetTileSize())
	got := r.canopyShadeFactorAt(tileSize, tileSize/2)
	if got <= 0.7 || got >= 1.0 {
		t.Fatalf("interpolated canopy shade = %v, want between 0.7 and 1.0", got)
	}
}
