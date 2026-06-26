package game

import "testing"

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
		{name: "dungeon ambient stays darker than canopy", mapAmbient: 0.35, surfaceShade: 0.7, viewerShade: 1, want: 0.35},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canopyAmbient(tt.mapAmbient, tt.surfaceShade, tt.viewerShade)
			if got != tt.want {
				t.Fatalf("canopyAmbient(%v, %v, %v) = %v, want %v",
					tt.mapAmbient, tt.surfaceShade, tt.viewerShade, got, tt.want)
			}
		})
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
