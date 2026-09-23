package game

import (
	"image"
	"image/color"
	"math"

	"ugataima/internal/config"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// Index-map B packs profile (bits 0-2), boundary (6), and shoreline
// proximity (7); alpha stays opaque. Textures keep world-aligned UVs.
// Byte contract shared with the Kage floor shader. Zero is deliberately hard:
// old/untextured maps and unclassified groups retain their authored geometry.
const (
	floorBlendHard byte = iota
	floorBlendNatural
	floorBlendWater
	floorBlendCliffEast
	floorBlendCliffWest
	floorBlendVoid
)

const floorShoreReach = 2.0
const floorBlendBoundary byte = 64
const floorBlendShore byte = 128

type floorMaterial struct {
	atlas, shore int // atlas index + 1; zero means no texture
	profile      byte
	group        string
	color        color.RGBA
}

func (r *Renderer) floorBiomeKeyAt(x, y int) string {
	wm := world.GlobalWorldManager
	if wm == nil {
		return ""
	}
	biome := ""
	if r.game.openWorldActive() {
		biome = wm.BiomeAtTile(x, y)
	}
	if biome == "" {
		if mc := wm.GetCurrentMapConfig(); mc != nil {
			biome = mc.Biome
		}
	}
	return biome
}

func floorProfileByte(profile config.FloorTransition) byte {
	switch profile {
	case config.FloorTransitionNatural:
		return floorBlendNatural
	case config.FloorTransitionWater:
		return floorBlendWater
	case config.FloorTransitionCliffEast:
		return floorBlendCliffEast
	case config.FloorTransitionCliffWest:
		return floorBlendCliffWest
	case config.FloorTransitionVoid:
		return floorBlendVoid
	default:
		return floorBlendHard
	}
}

func (r *Renderer) resolvedFloorMaterial(x, y int, tile world.TileType3D) floorMaterial {
	group := r.floorTextureGroupForTile(x, y, tile)
	biomeKey := r.floorBiomeKeyAt(x, y)
	unified := r.game.openWorldActive()
	lookup := floorTextureGroupKey(biomeKey, group, unified)
	out := floorMaterial{group: lookup}
	if texture, ok := r.floorTexGroups[lookup]; ok && texture.count > 0 {
		out.atlas = texture.start + 1 + stableFloorTextureIndex(x, y, int(tile), texture.count)
	}
	// Read only the transition map rather than copying the full biome config.
	var transitions map[string]config.FloorTransition
	if wm := world.GlobalWorldManager; wm != nil {
		transitions = wm.Biomes[biomeKey].FloorTransitions
	}
	out.profile = floorProfileByte(transitions[group])
	// Authored directional banks are not reusable beach textures.
	if group == defaultFloorTextureGroup && transitions["beach"] == config.FloorTransitionNatural {
		if shore, ok := r.floorTexGroups[floorTextureGroupKey(biomeKey, "beach", unified)]; ok && shore.count > 0 {
			out.shore = shore.start + 1 + stableFloorTextureIndex(x, y, int(tile), shore.count)
		}
	}
	return out
}

// prepareFloorMaps runs after map/NPC ground overrides and inherited floor
// resolution. It does not inspect actor positions or mutate world tiles.
// Called on map load, quest terrain changes and save restoration by the same
// floor-cache lifecycle as lighting; masks are derived and never serialized.
func (r *Renderer) prepareFloorMaps(w, h int) (colors, indices, shore *image.RGBA) {
	colors = image.NewRGBA(image.Rect(0, 0, w, h))
	indices = image.NewRGBA(colors.Rect)
	materials := make([]floorMaterial, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cell := y*w + x
			c := r.floorColorCache[[2]int{x, y}]
			colors.Pix[cell*4], colors.Pix[cell*4+1], colors.Pix[cell*4+2], colors.Pix[cell*4+3] = c.R, c.G, c.B, 255
			tile := world.TileEmpty
			if r.game.world != nil && x < r.game.world.Width && y < r.game.world.Height {
				tile = r.game.world.Tiles[y][x]
			}
			m := r.resolvedFloorMaterial(x, y, tile)
			m.color = c
			materials[cell] = m
			indices.Pix[cell*4], indices.Pix[cell*4+1], indices.Pix[cell*4+2], indices.Pix[cell*4+3] = byte(m.atlas), byte(m.shore), m.profile, 255
		}
	}
	// One sample per tile VERTEX, not center, keeps the shoreline field
	// continuous across neighboring tiles, including diagonal water contacts.
	shore = image.NewRGBA(image.Rect(0, 0, w+1, h+1))
	for y := 0; y <= h; y++ {
		for x := 0; x <= w; x++ {
			nearest := floorShoreReach * floorShoreReach
			for ny := max(0, y-2); ny < min(h, y+2); ny++ {
				for nx := max(0, x-2); nx < min(w, x+2); nx++ {
					if materials[ny*w+nx].profile != floorBlendWater {
						continue
					}
					dx := float64(max(nx-x, 0, x-nx-1))
					dy := float64(max(ny-y, 0, y-ny-1))
					nearest = min(nearest, dx*dx+dy*dy)
				}
			}
			i := y*shore.Stride + x*4
			shore.Pix[i] = byte(math.Round(math.Sqrt(nearest) * 255 / floorShoreReach))
			shore.Pix[i+3] = 255
		}
	}
	// Only real material/color boundaries need the multi-material shader path.
	// Variants of the same material need transitions too. Uniform interiors
	// retain the single-material path; beach variants only count near water.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			m := materials[y*w+x]
			near := shore.RGBAAt(x, y).R < 255 || shore.RGBAAt(x+1, y).R < 255 || shore.RGBAAt(x, y+1).R < 255 || shore.RGBAAt(x+1, y+1).R < 255
			boundary := false
			if m.profile != floorBlendHard {
				for ny := max(0, y-1); ny < min(h, y+2); ny++ {
					for nx := max(0, x-1); nx < min(w, x+2); nx++ {
						neighbor := materials[ny*w+nx]
						if neighbor.group != m.group || neighbor.atlas != m.atlas || (near && neighbor.shore != m.shore) || m.color != neighbor.color {
							boundary = true
						}
					}
				}
			}
			if boundary {
				indices.Pix[i+2] |= floorBlendBoundary
			}
			if near && (m.profile == floorBlendNatural || (m.profile == floorBlendWater && boundary)) {
				indices.Pix[i+2] |= floorBlendShore
			} else {
				indices.Pix[i+1] = 0
			}
		}
	}
	return
}

func (r *Renderer) buildFloorColorMap(w, h int) {
	if w <= 0 || h <= 0 {
		for _, img := range []*ebiten.Image{r.floorColorMap, r.floorTextureIndexMap, r.floorShoreMap} {
			if img != nil {
				img.Deallocate()
			}
		}
		r.floorColorMap, r.floorTextureIndexMap, r.floorShoreMap = nil, nil, nil
		return
	}
	colors, indices, shore := r.prepareFloorMaps(w, h)
	for _, pair := range []struct {
		dst **ebiten.Image
		src *image.RGBA
	}{
		{&r.floorColorMap, colors}, {&r.floorTextureIndexMap, indices}, {&r.floorShoreMap, shore},
	} {
		if *pair.dst == nil || (*pair.dst).Bounds().Size() != pair.src.Bounds().Size() {
			if *pair.dst != nil {
				(*pair.dst).Deallocate()
			}
			*pair.dst = ebiten.NewImage(pair.src.Bounds().Dx(), pair.src.Bounds().Dy())
		}
		(*pair.dst).WritePixels(pair.src.Pix)
	}
}
