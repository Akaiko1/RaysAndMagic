package game

import (
	"fmt"
	"os"
	"path/filepath"

	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

// Buff-cast overlay: a short 4-frame animation (w==h*4 sheet, white keyed to
// alpha) played once, centred in the party's view, when a buff spell resolves.
// Screen-space like the melee flourish - no camera/depth involvement.

const (
	buffFxTotalFrames  = 56  // ~0.9s at 60 TPS
	buffFxFadeInEnd    = 6   // ticks of fade-in
	buffFxFadeOutStart = 42  // fade to zero from here to the end
	buffFxHeightFrac   = 0.6 // target sprite height as a fraction of screen height
)

type buffFxAnim struct {
	sprite string
	age    int
}

// playBuffFx queues the buff overlay animation. No-op for an empty name.
func (g *MMGame) playBuffFx(sprite string) {
	if sprite == "" {
		return
	}
	g.buffFxAnims = append(g.buffFxAnims, buffFxAnim{sprite: sprite})
}

// tickBuffFx ages the overlay animations and drops the finished ones.
func (g *MMGame) tickBuffFx() {
	if len(g.buffFxAnims) == 0 {
		return
	}
	dst := g.buffFxAnims[:0]
	for _, a := range g.buffFxAnims {
		a.age++
		if a.age < buffFxTotalFrames {
			dst = append(dst, a)
		}
	}
	g.buffFxAnims = dst
}

// drawBuffFx renders the active buff overlays centred on screen: sheet frame
// by animation age, gentle fade in/out. Drawn at the end of the world pass, so
// it sits above the scene and below the HUD.
func (r *Renderer) drawBuffFx(screen *ebiten.Image) {
	if len(r.game.buffFxAnims) == 0 {
		return
	}
	sw := float64(r.game.config.GetScreenWidth())
	sh := float64(r.game.config.GetScreenHeight())
	for _, a := range r.game.buffFxAnims {
		sheet := r.game.sprites.GetSprite(a.sprite)
		if sheet == nil {
			continue
		}
		frames := r.animationFrames(sheet)
		frame := a.age * len(frames) / buffFxTotalFrames
		if frame >= len(frames) {
			frame = len(frames) - 1
		}
		img := frames[frame]

		alpha := 1.0
		if a.age < buffFxFadeInEnd {
			alpha = float64(a.age) / buffFxFadeInEnd
		} else if a.age > buffFxFadeOutStart {
			alpha = 1.0 - float64(a.age-buffFxFadeOutStart)/float64(buffFxTotalFrames-buffFxFadeOutStart)
		}

		fw, fh := img.Bounds().Dx(), img.Bounds().Dy()
		// Stay close to native size (never upscale past 1:1) and shrink with the
		// same linear filter the portraits use - nearest at fractional scales
		// visibly mushes the pixel art.
		scale := sh * buffFxHeightFrac / float64(fh)
		if scale > 1 {
			scale = 1
		}
		opts := &ebiten.DrawImageOptions{}
		opts.Filter = ebiten.FilterLinear
		opts.GeoM.Scale(scale, scale)
		opts.GeoM.Translate(sw/2-float64(fw)*scale/2, sh*0.42-float64(fh)*scale/2)
		opts.ColorScale.ScaleAlpha(float32(alpha))
		screen.DrawImage(img, opts)
	}
}

// validateBuffFxSprites fails fast on a buff_fx_sprite that points at a sheet
// missing from the sprite index - a YAML typo would otherwise show the
// placeholder box mid-screen at the first cast. Skipped when no sprite assets
// are present at the working dir (headless test runs).
func (g *MMGame) validateBuffFxSprites() {
	if config.GlobalSpells == nil {
		return
	}
	if _, err := os.Stat(filepath.Join("assets", "sprites")); err != nil {
		return
	}
	for key, def := range config.GlobalSpells.Spells {
		if def.BuffFxSprite != "" && !g.sprites.HasSprite(def.BuffFxSprite) {
			panic(fmt.Sprintf("spell %q: buff_fx_sprite %q not found under assets/sprites", key, def.BuffFxSprite))
		}
	}
}
