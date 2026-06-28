package graphics

import (
	"image"
	"image/color"
	"image/draw"
	_ "image/png"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

type SpriteManager struct {
	sprites          map[string]*ebiten.Image
	spriteTypeCache  map[string]string // Cache sprite types to avoid repeated file checks
	animations       map[string]*SpriteAnimation
	animationMissing map[string]bool

	// spritePaths maps a sprite basename (no extension) to its PNG path, built
	// once by walking the sprite roots recursively (see ensureIndex). Lets
	// sprites live in any subfolder layout — lookup is by name, not location.
	// spriteDirType records the originating root's placeholder type.
	spritePaths   map[string]string
	spriteDirType map[string]string

	// Load-time color key (configured via SetColorKey): pixels within keyTol of the
	// key color become transparent; with keyDespill, tinted fringe pixels have the
	// cast subtracted instead (kept opaque).
	keyEnabled bool
	keyR       uint8
	keyG       uint8
	keyB       uint8
	keyTol     int
	keyDespill bool
	// Sprites whose interior magenta is real art: despill only their edge fringe
	// (within keyEdgeRadius px of a transparent pixel), not the whole body.
	keyEdgeOnly   map[string]bool
	keyEdgeRadius int
}

// despillHueFloor is the magenta-excess (min(R,B)−G) below which a kept pixel is
// left alone. Deliberately low (aggressive): project art reserves any magenta
// hue for removable background/fringe, so even faint casts are subtracted.
const despillHueFloor = 8

// despillEdgeRadiusDefault is the fringe band (px from a transparent edge) used
// for edge-only despill sprites when the config leaves the radius unset.
const despillEdgeRadiusDefault = 3

// SetDespillEdgeOnly marks sprites (by name; animation sheets as
// "<name>_<animType>") whose interior magenta must be preserved — despill on
// them is restricted to within `radius` px of a transparent edge.
func (sm *SpriteManager) SetDespillEdgeOnly(names []string, radius int) {
	sm.keyEdgeOnly = make(map[string]bool, len(names))
	for _, n := range names {
		sm.keyEdgeOnly[n] = true
	}
	sm.keyEdgeRadius = radius
}

// SetColorKey enables/configures the load-time color key (see SpriteManager).
// Must be called before sprites are first loaded to take effect on them.
func (sm *SpriteManager) SetColorKey(enabled bool, r, g, b, tolerance int, despill bool) {
	sm.keyEnabled = enabled
	sm.keyR, sm.keyG, sm.keyB = uint8(r), uint8(g), uint8(b)
	sm.keyTol = tolerance
	sm.keyDespill = despill
}

// applyColorKey returns a copy of src with the key color removed: pixels within
// keyTol of the key go transparent; with keyDespill, every remaining magenta-hue
// pixel (R and B above G) has that excess subtracted and stays opaque. Despill
// assumes a magenta-style key (high R,B / low G). No-op when the key is off.
//
// For names in keyEdgeOnly (intentional magenta art), despill is restricted to
// the fringe band within keyEdgeRadius px of a transparent edge, leaving the
// interior purple/magenta untouched.
func (sm *SpriteManager) applyColorKey(name string, src image.Image) image.Image {
	if !sm.keyEnabled || src == nil {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	near := func(v, target uint8) bool {
		d := int(v) - int(target)
		if d < 0 {
			d = -d
		}
		return d <= sm.keyTol
	}
	min2 := func(a, b uint8) uint8 {
		if a < b {
			return a
		}
		return b
	}
	isKey := func(p color.NRGBA) bool {
		return near(p.R, sm.keyR) && near(p.G, sm.keyG) && near(p.B, sm.keyB)
	}

	edgeOnly := sm.keyEdgeOnly[name]
	// Transparency mask, needed only when despill is limited to the fringe band.
	var trans []bool
	if edgeOnly {
		trans = make([]bool, w*h)
	}

	// Pass 1: erase the key core (and record transparency for edge-only despill).
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := dst.NRGBAAt(b.Min.X+x, b.Min.Y+y)
			keyed := p.A != 0 && isKey(p)
			if edgeOnly {
				trans[y*w+x] = p.A == 0 || keyed
			}
			if keyed {
				dst.SetNRGBA(b.Min.X+x, b.Min.Y+y, color.NRGBA{})
			}
		}
	}
	if !sm.keyDespill {
		return dst
	}

	radius := sm.keyEdgeRadius
	if radius <= 0 {
		radius = despillEdgeRadiusDefault
	}
	nearEdge := func(x, y int) bool {
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				nx, ny := x+dx, y+dy
				if nx < 0 || ny < 0 || nx >= w || ny >= h {
					return true // image border is an edge
				}
				if trans[ny*w+nx] {
					return true
				}
			}
		}
		return false
	}

	// Pass 2: despill kept pixels. Edge-only sprites despill the fringe band only,
	// so their intentional interior magenta survives.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			px, py := b.Min.X+x, b.Min.Y+y
			p := dst.NRGBAAt(px, py)
			if p.A == 0 {
				continue
			}
			excess := int(min2(p.R, p.B)) - int(p.G)
			if excess <= despillHueFloor {
				continue
			}
			if edgeOnly && !nearEdge(x, y) {
				continue
			}
			p.R = uint8(int(p.R) - excess)
			p.B = uint8(int(p.B) - excess)
			dst.SetNRGBA(px, py, p)
		}
	}
	return dst
}

func NewSpriteManager() *SpriteManager {
	return &SpriteManager{
		sprites:          make(map[string]*ebiten.Image),
		spriteTypeCache:  make(map[string]string),
		animations:       make(map[string]*SpriteAnimation),
		animationMissing: make(map[string]bool),
	}
}

// spriteBaseDirs are the roots indexed by basename (recursively). Each maps to
// the placeholder type used when a named sprite is missing. floor/ and sky/ are
// loaded separately via resolveNamedPNG and intentionally omitted here.
var spriteBaseDirs = []struct{ dir, typ string }{
	{"assets/sprites/mobs", "npc_mob"},
	{"assets/sprites/characters", "npc_mob"},
	{"assets/sprites/environment", "environment"},
	{"assets/sprites/interface", "interface"},
}

// isIgnoredSpriteDir reports folders excluded from the sprite index: archives
// (any case) and any name starting with "_" or "." — a convention to park
// unused/duplicate art in the tree without it shadowing live sprites.
func isIgnoredSpriteDir(name string) bool {
	return strings.EqualFold(name, "archive") ||
		strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".")
}

// buildSpriteIndex walks the sprite roots recursively (skipping ignored dirs)
// and returns basename→path and basename→placeholder-type maps. Sprites may
// therefore be grouped into arbitrary subfolders; basenames must be unique
// across the whole tree (duplicates are logged and the first, by root order,
// wins). Shared by SpriteManager.ensureIndex and the package-level resolver.
func buildSpriteIndex() (paths, dirType map[string]string) {
	paths = make(map[string]string)
	dirType = make(map[string]string)
	for _, root := range spriteBaseDirs {
		_ = filepath.WalkDir(root.dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // missing root (e.g. tests run outside the repo) — skip
			}
			if d.IsDir() {
				if isIgnoredSpriteDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(path) != ".png" {
				return nil
			}
			base := strings.TrimSuffix(d.Name(), ".png")
			if existing, dup := paths[base]; dup {
				log.Printf("sprite index: duplicate basename %q (%q vs %q); keeping %q", base, existing, path, existing)
				return nil
			}
			paths[base] = path
			dirType[base] = root.typ
			return nil
		})
	}
	return paths, dirType
}

// ensureIndex lazily builds this manager's basename→path index.
func (sm *SpriteManager) ensureIndex() {
	if sm == nil || sm.spritePaths != nil {
		return
	}
	sm.spritePaths, sm.spriteDirType = buildSpriteIndex()
}

var (
	sharedIndexOnce   sync.Once
	sharedSpritePaths map[string]string
)

// ResolveSpritePath returns the on-disk PNG path for a sprite basename, found
// anywhere under the sprite roots (recursive; archive/_-prefixed dirs excluded).
// The index is built once and shared. ok=false if no such sprite exists. Lets
// external tools (e.g. the map editor) resolve sprites by name, layout-agnostic.
func ResolveSpritePath(name string) (string, bool) {
	sharedIndexOnce.Do(func() {
		sharedSpritePaths, _ = buildSpriteIndex()
	})
	p, ok := sharedSpritePaths[name]
	return p, ok
}

type SpriteAnimation struct {
	Frames      []*ebiten.Image
	FrameWidth  int
	FrameHeight int
}

func animationKey(name, animType string) string {
	return name + ":" + animType
}

func (sm *SpriteManager) createPlaceholder(name string) *ebiten.Image {
	// Create larger sprites for trees and biome obstacles to prevent transparency issues
	var img *ebiten.Image
	if name == "tree" || name == "ancient_tree" || name == "sand_dune" || name == "large_dune" || name == "coral_reef" || name == "large_coral" {
		img = ebiten.NewImage(32, 32) // Larger obstacle sprites
	} else {
		img = ebiten.NewImage(16, 16)
	}

	// Determine sprite type based on search paths (cached)
	spriteType := sm.getCachedSpriteType(name)

	switch spriteType {
	case "environment":
		img.Fill(color.RGBA{128, 0, 128, 255}) // Purple for environment
	case "npc_mob":
		img.Fill(color.RGBA{0, 128, 0, 255}) // Green for NPCs/mobs
	default:
		img.Fill(color.RGBA{128, 128, 128, 255}) // Gray for unknown
	}

	return img
}

// getCachedSpriteType determines sprite type with caching to avoid repeated file checks
func (sm *SpriteManager) getCachedSpriteType(name string) string {
	if spriteType, exists := sm.spriteTypeCache[name]; exists {
		return spriteType
	}

	spriteType := sm.determineSpritePaths(name)
	sm.spriteTypeCache[name] = spriteType
	return spriteType
}

// determineSpritePaths returns the placeholder type for a name from the index.
func (sm *SpriteManager) determineSpritePaths(name string) string {
	sm.ensureIndex()
	if t, ok := sm.spriteDirType[name]; ok {
		return t
	}
	return "unknown"
}

func (sm *SpriteManager) GetSprite(name string) *ebiten.Image {
	if sprite, exists := sm.sprites[name]; exists {
		return sprite
	}

	// Try to dynamically load the sprite if it's not already loaded
	sm.loadSpriteIfExists(name)

	// Check again after attempting to load
	if sprite, exists := sm.sprites[name]; exists {
		return sprite
	}

	// If still not found, create placeholder
	return sm.createPlaceholder(name)
}

func (sm *SpriteManager) HasSprite(name string) bool {
	if name == "" {
		return false
	}
	if _, exists := sm.sprites[name]; exists {
		return true
	}
	return sm.spriteExists(name)
}

func (sm *SpriteManager) GetSpriteVariants(baseName string) []string {
	variants := []string{}
	if sm.spriteExists(baseName) {
		variants = append(variants, baseName)
	}
	for i := 0; ; i++ {
		name := baseName + strconv.Itoa(i)
		if !sm.spriteExists(name) {
			break
		}
		if name != baseName {
			variants = append(variants, name)
		}
	}
	return variants
}

func (sm *SpriteManager) spriteExists(name string) bool {
	if sm == nil {
		return false
	}
	sm.ensureIndex()
	_, ok := sm.spritePaths[name]
	return ok
}

func (sm *SpriteManager) GetAnimation(name, animType string) *SpriteAnimation {
	key := animationKey(name, animType)
	if anim, exists := sm.animations[key]; exists {
		return anim
	}
	if sm.animationMissing[key] {
		return nil
	}
	sm.loadAnimationIfExists(name, animType)
	if anim, exists := sm.animations[key]; exists {
		return anim
	}
	sm.animationMissing[key] = true
	return nil
}

// loadSpriteIfExists attempts to load a sprite by basename from the index.
func (sm *SpriteManager) loadSpriteIfExists(name string) {
	sm.ensureIndex()
	spritePath, ok := sm.spritePaths[name]
	if ok {
		if file, err := os.Open(spritePath); err == nil {
			defer file.Close()
			if img, _, err := image.Decode(file); err == nil {
				img = sm.applyColorKey(name, img)
				sm.sprites[name] = ebiten.NewImageFromImage(img)
				sm.spriteTypeCache[name] = sm.spriteDirType[name]
				return
			}
		}
	}

	// If no sprite file found, cache as unknown to avoid future file checks
	sm.spriteTypeCache[name] = "unknown"
}

func (sm *SpriteManager) loadAnimationIfExists(name, animType string) {
	sm.ensureIndex()
	spritePath, ok := sm.spritePaths[name+"_"+animType]
	if ok {
		file, err := os.Open(spritePath)
		if err != nil {
			return
		}
		defer file.Close()

		img, _, err := image.Decode(file)
		if err != nil {
			return
		}
		img = sm.applyColorKey(name+"_"+animType, img)

		bounds := img.Bounds()
		frameHeight := bounds.Dy()
		frameWidth := bounds.Dx()
		if frameHeight <= 0 || frameWidth <= 0 {
			return
		}
		subImager, ok := img.(interface {
			SubImage(r image.Rectangle) image.Image
		})
		if !ok {
			return
		}

		// Horizontal strip (1xN)
		if frameWidth%frameHeight == 0 {
			frameCount := frameWidth / frameHeight
			if frameCount > 1 {
				frames := make([]*ebiten.Image, 0, frameCount)
				for i := 0; i < frameCount; i++ {
					rect := image.Rect(
						bounds.Min.X+i*frameHeight,
						bounds.Min.Y,
						bounds.Min.X+(i+1)*frameHeight,
						bounds.Min.Y+frameHeight,
					)
					frameImg := subImager.SubImage(rect)
					frames = append(frames, ebiten.NewImageFromImage(frameImg))
				}

				sm.animations[animationKey(name, animType)] = &SpriteAnimation{
					Frames:      frames,
					FrameWidth:  frameHeight,
					FrameHeight: frameHeight,
				}
				return
			}
		}

		// Square 2x2 grid (4 frames)
		if frameWidth == frameHeight && frameWidth%2 == 0 {
			frameSize := frameWidth / 2
			frames := make([]*ebiten.Image, 0, 4)
			for row := 0; row < 2; row++ {
				for col := 0; col < 2; col++ {
					rect := image.Rect(
						bounds.Min.X+col*frameSize,
						bounds.Min.Y+row*frameSize,
						bounds.Min.X+(col+1)*frameSize,
						bounds.Min.Y+(row+1)*frameSize,
					)
					frameImg := subImager.SubImage(rect)
					frames = append(frames, ebiten.NewImageFromImage(frameImg))
				}
			}

			sm.animations[animationKey(name, animType)] = &SpriteAnimation{
				Frames:      frames,
				FrameWidth:  frameSize,
				FrameHeight: frameSize,
			}
			return
		}

		return
	}
}
