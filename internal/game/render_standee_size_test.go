package game

import "testing"

func TestStandeeHeightForWidthPreservesTextureAspect(t *testing.T) {
	tests := []struct {
		name          string
		textureWidth  int
		textureHeight int
		wantHeight    float64
	}{
		{name: "square", textureWidth: 512, textureHeight: 512, wantHeight: 2},
		{name: "portrait", textureWidth: 512, textureHeight: 1024, wantHeight: 4},
		{name: "landscape", textureWidth: 1024, textureHeight: 512, wantHeight: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := standeeHeightForWidth(2, tt.textureWidth, tt.textureHeight); got != tt.wantHeight {
				t.Fatalf("standeeHeightForWidth(2, %d, %d) = %v, want %v", tt.textureWidth, tt.textureHeight, got, tt.wantHeight)
			}
		})
	}
}

func TestStandeeHeightForWidthInvalidTextureFallsBack(t *testing.T) {
	if got := standeeHeightForWidth(2, 0, 512); got != 2 {
		t.Fatalf("invalid texture width changed standee height: got %v, want 2", got)
	}
}
