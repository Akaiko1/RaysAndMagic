package graphics

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
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
		img, err := DecodeImageFile(path)
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

// IconArtBounds is where the art lies on the black backing of an unframed
// content icon (a card inside its icon): the rows and columns that are mostly
// non-black, so a glow or a stray speck never widens it. It is measured once
// from the authored PNG; known is false when the icon cannot be decoded.
// Known costs, accepted: the first lookup decodes the PNG on the calling
// (draw) thread - a binder spread's first open decodes up to 18 small icons -
// and a failed decode stays cached for the session (the caller falls back to
// the whole icon). Move it to the resource preparation pass if either shows.
func (sm *SpriteManager) IconArtBounds(name string) (bounds image.Rectangle, known bool) {
	if sm == nil || name == "" {
		return image.Rectangle{}, false
	}
	bounds, cached := sm.iconArtBounds[name]
	if !cached {
		if sm.deferMissingResource(SpriteResourceRequest{Name: name}) {
			return image.Rectangle{}, false
		}
		sm.ensureIndex()
		if img, err := DecodeImageFile(sm.spritePaths[name]); err == nil {
			bounds = iconArtBoundsFromImage(img)
		}
		if sm.iconArtBounds == nil {
			sm.iconArtBounds = make(map[string]image.Rectangle)
		}
		sm.iconArtBounds[name] = bounds
	}
	return bounds, !bounds.Empty()
}

func iconArtBoundsFromImage(img image.Image) image.Rectangle {
	// A pixel is art when a channel passes this level; a row or column is
	// art when this share of it is.
	const ink, cover = 30, .3
	b := img.Bounds()
	rows, cols := make([]int, b.Dy()), make([]int, b.Dx())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if max(r, g, bl)>>8 > ink {
				rows[y-b.Min.Y]++
				cols[x-b.Min.X]++
			}
		}
	}
	span := func(counts []int, length int) (int, int) {
		first, last := -1, -1
		for i, n := range counts {
			if float64(n) > cover*float64(length) {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		return first, last + 1
	}
	x0, x1 := span(cols, b.Dy())
	y0, y1 := span(rows, b.Dx())
	if x0 < 0 || y0 < 0 {
		return image.Rectangle{}
	}
	return image.Rect(x0, y0, x1, y1)
}
