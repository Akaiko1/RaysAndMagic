package game

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Kage sources compile at runtime, so a Go build can't catch shader syntax
// errors - compile them here so a broken shader fails the suite, not the game.
func TestKageShadersCompile(t *testing.T) {
	for name, src := range map[string]string{
		"floor":    floorShaderSrc,
		"sky":      skyShaderSrc,
		"turnBlur": turnBlurShaderSrc,
	} {
		if _, err := ebiten.NewShader([]byte(src)); err != nil {
			t.Errorf("%s shader failed to compile: %v", name, err)
		}
	}
}
