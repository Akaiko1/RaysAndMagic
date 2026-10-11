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
	buffFxTotalFrames  = 56  // ~0.47s at 120 TPS
	buffFxFadeInEnd    = 6   // ticks of fade-in (~0.05s at 120 TPS)
	buffFxFadeOutStart = 42  // fade to zero from here to the end (~0.35s at 120 TPS)
	buffFxHeightFrac   = 0.6 // target sprite height as a fraction of screen height
)

type buffFxAnim struct {
	sprite   string
	weapon   string
	overlay  string
	lifetime int
	age      int
	delay    int // Presentation-only queue; gameplay has already committed.
	scale    float64
}

func (a buffFxAnim) screenScale() float64 {
	if a.scale > 0 {
		return a.scale
	}
	return 1
}

func (g *MMGame) regenerationCast() (buffFxAnim, bool) {
	for i := len(g.buffFxAnims) - 1; i >= 0; i-- {
		if a := g.buffFxAnims[i]; a.overlay == "regeneration" && a.delay == 0 {
			return a, true
		}
	}
	return buffFxAnim{}, false
}

func (a buffFxAnim) totalFrames() int {
	if a.lifetime > 0 {
		return a.lifetime
	}
	return buffFxTotalFrames
}

func (g *MMGame) playSpellCastFx(def *config.SpellDefinitionConfig) {
	g.playBuffFx(def.BuffFxSprite)
	if def.Graphics != nil && def.Graphics.CastOverlay != "" {
		g.buffFxAnims = append(g.buffFxAnims, buffFxAnim{overlay: def.Graphics.CastOverlay, weapon: def.Graphics.CastOverlayWeapon, scale: def.Graphics.CastOverlayScale, lifetime: max(1, g.config.GetTPS()*6/5)})
	}
}

// playBuffFx queues the buff overlay animation. No-op for an empty name.
func (g *MMGame) playBuffFx(sprite string) {
	if sprite == "" {
		return
	}
	g.buffFxAnims = append(g.buffFxAnims, buffFxAnim{sprite: sprite})
}

// queueAutocastFx keeps each committed autocast's visual group together and
// schedules it after existing cast overlays. Effects and mana are never delayed.
func (g *MMGame) queueAutocastFx(first int) {
	wait := 0
	for _, a := range g.buffFxAnims[:first] {
		wait = max(wait, a.delay+a.totalFrames()-a.age)
	}
	for i := first; i < len(g.buffFxAnims); i++ {
		g.buffFxAnims[i].delay = wait
	}
}

// tickBuffFx ages the overlay animations and drops the finished ones.
func (g *MMGame) tickBuffFx() {
	g.expireSpellWorldVisuals()
	if len(g.buffFxAnims) == 0 {
		return
	}
	dst := g.buffFxAnims[:0]
	for _, a := range g.buffFxAnims {
		if a.delay > 0 {
			a.delay--
		} else {
			a.age++
		}
		if a.age < a.totalFrames() {
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
	sw := float64(r.game.worldWidth())
	sh := float64(r.game.worldHeight())
	for _, a := range r.game.buffFxAnims {
		if a.delay > 0 {
			continue
		}
		if a.overlay == "regeneration" {
			continue // A party effect is drawn on each living hero's card.
		}
		if a.overlay != "" {
			r.drawCastOverlay(screen, a)
			continue
		}
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
		// This overlay is authored in the camera's frame, like the complete
		// melee swing. Submit it directly so world Panini cannot resize it.
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
	if config.GlobalLoots != nil {
		for key, crate := range config.GlobalLoots.Crates {
			if crate.Buff != nil {
				icon, _ := g.resolveStatusIconSprite(crate.Buff.StatusIcon)
				if !g.sprites.HasSprite(icon) {
					panic(fmt.Sprintf("crate %q: status_icon %q not found under assets/sprites", key, crate.Buff.StatusIcon))
				}
			}
		}
	}
	for key, def := range config.GlobalSpells.Spells {
		if def.Graphics != nil && def.Graphics.CastOverlayWeapon != "" {
			if _, ok := weaponBodyModels[def.Graphics.CastOverlayWeapon]; !ok {
				panic(fmt.Sprintf("spell %q: cast_overlay_weapon %q has no weapon model", key, def.Graphics.CastOverlayWeapon))
			}
		}
		if def.Graphics != nil && def.Graphics.TargetCursor != "" && !g.sprites.HasSprite(def.Graphics.TargetCursor) {
			panic(fmt.Sprintf("spell %q: target_cursor %q not found under assets/sprites", key, def.Graphics.TargetCursor))
		}
		if def.BuffFxSprite != "" && !g.sprites.HasSprite(def.BuffFxSprite) {
			panic(fmt.Sprintf("spell %q: buff_fx_sprite %q not found under assets/sprites", key, def.BuffFxSprite))
		}
	}
}
