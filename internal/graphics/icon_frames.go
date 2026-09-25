package graphics

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"strings"

	"ugataima/internal/config"
)

const contentIconSize = config.ContentIconSize

// HUD variants share the authored PNG and resource loader, but omit the
// decorative content frame. Both surfaces use the same destination scaler.
const hudIconPrefix = "hud_"

func (sm *SpriteManager) HUDIconName(name string) string {
	if _, ok := sm.iconFrames[name]; ok {
		return hudIconPrefix + name
	}
	return name
}

func (sm *SpriteManager) indexHUDIcons() {
	for name := range sm.iconFrames {
		if path := sm.spritePaths[name]; path != "" {
			sm.spritePaths[hudIconPrefix+name] = path
			sm.spriteDirType[hudIconPrefix+name] = sm.spriteDirType[name]
		}
	}
}

func (sm *SpriteManager) prepareSpritePixels(name string, src image.Image) image.Image {
	// Full illustrations have painted backgrounds, not a sprite chroma key.
	// Preserve intentional magenta and purple in every loading/metadata path.
	if strings.HasPrefix(name, "full_art_") {
		return src
	}
	if base, hud := strings.CutPrefix(name, hudIconPrefix); hud {
		if _, ok := sm.iconFrames[base]; ok {
			return sm.applyColorKey(base, src)
		}
	}
	return sm.composeContentIcon(name, sm.applyColorKey(name, src))
}

type iconFrameEntry struct {
	mask *image.RGBA
	tint color.RGBA
}

// Snapshot immutable content metadata before background decoding begins. Three
// CPU masks are shared by all entries; no additional GPU images are retained.
func loadIconFrameEntries() map[string]iconFrameEntry {
	cfg := config.GlobalIconFrames
	if cfg == nil || len(cfg.Icons) == 0 {
		return nil
	}
	masks := make(map[string]*image.RGBA, len(cfg.Frames))
	for style, path := range cfg.Frames {
		f, err := os.Open(path)
		if err != nil {
			panic(fmt.Sprintf("icon frame: %v", err))
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			panic(fmt.Sprintf("icon frame %s: %v", style, err))
		}
		if img.Bounds().Dx() != contentIconSize || img.Bounds().Dy() != contentIconSize {
			panic(fmt.Sprintf("icon frame %s must be %dx%d", style, contentIconSize, contentIconSize))
		}
		masks[style] = rgbaFromImage(img)
	}
	entries := make(map[string]iconFrameEntry, len(cfg.Icons))
	for name, style := range cfg.Icons {
		tint, ok := config.IconFrameColor(name)
		if !ok {
			panic(fmt.Sprintf("unknown icon content %q", name))
		}
		entries[name] = iconFrameEntry{mask: masks[style], tint: tint}
	}
	return entries
}

// Prepare one source at its authored resolution. Every consumer, including
// the editor's async cache, receives this same composed source. Destination
// scaling stays exclusively in DrawImageScaled, with its existing filters.
func (sm *SpriteManager) composeContentIcon(name string, art image.Image) image.Image {
	entry, ok := sm.iconFrames[name]
	if !ok || art == nil {
		return art
	}
	b := art.Bounds()
	if b.Dx() != contentIconSize || b.Dy() != contentIconSize {
		panic(fmt.Sprintf("unframed icon %s must be %dx%d", name, contentIconSize, contentIconSize))
	}
	out := image.NewRGBA(image.Rect(0, 0, contentIconSize, contentIconSize))
	draw.Draw(out, out.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	draw.Draw(out, out.Bounds(), art, b.Min, draw.Over)
	// White RGB with authored alpha is the tintable frame mask. Multiplying
	// alpha preserves the generated metal's shading without changing the art.
	draw.DrawMask(out, out.Bounds(), image.NewUniform(entry.tint), image.Point{}, entry.mask, entry.mask.Bounds().Min, draw.Over)
	return out
}
