package game

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"sort"
	"strings"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"
	"ugataima/internal/threading/rendering"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// TransparentSpriteData holds cached data for transparent environment sprites
type TransparentSpriteData struct {
	tileX      int
	tileY      int
	worldX     float64
	worldY     float64
	tileType   world.TileType3D
	spriteName string
}

type LightSource struct {
	X         float64
	Y         float64
	Radius    float64
	Intensity float64
}

type floorTexture struct {
	name   string
	width  int
	height int
	pixels []byte
}

type floorTextureGroup struct {
	start int
	count int
}

// Renderer handles all 3D rendering functionality
type Renderer struct {
	game                     *MMGame
	floorColorCache          map[[2]int]color.RGBA // Now world-level, static after init
	whiteImg                 *ebiten.Image   // 1x1 white image for untextured polygons
	renderedSpritesThisFrame map[[2]int]bool // Track which environment sprites have been rendered this frame
	// GPU floor rendering — a Kage shader replaces the per-pixel CPU loop.
	// floorColorMap is a worldW×worldH RGBA8 image with base tile colors.
	// floorTextureIndexMap is a worldW×worldH RGBA8 image; R encodes
	// atlas-index+1, 0 means no texture overlay. floorTexAtlas is a horizontal
	// strip of all configured floor material variants.
	floorShader          *ebiten.Shader
	floorColorMap        *ebiten.Image
	floorTextureIndexMap *ebiten.Image
	floorTexAtlas        *ebiten.Image
	floorTexGroups       map[string]floorTextureGroup
	floorTexCount        int
	floorTexTileW        int
	floorTexTileH        int
	floorTexturesKey     string // biome the floor textures were loaded for (cache key)
	// Per-frame reusable uniform buffer for floor shader light data, avoids
	// a 64-float allocation each draw call.
	floorLightsBuf [maxFloorShaderLights * 4]float32
	// Transparent environment sprite cache for performance
	transparentSpritesCache []TransparentSpriteData // Cached list of transparent sprites
	// Cached tile light sources (world-space)
	tileLightCache []LightSource
	// Active light sources for current frame (world-space)
	activeLights []LightSource
	// Precomputed ray direction cache for performance
	rayDirectionsX []float64 // Cached cos values for rays
	rayDirectionsY []float64 // Cached sin values for rays
	// Per-ray RaycastHit buffer, pre-allocated to avoid per-frame slice
	// allocations during raycasting. Each ray writes into its own index,
	// so disjoint cells are safe across parallel workers. Capacity grows
	// once per ray then stabilizes.
	rayHitBuffers [][]RaycastHit
	// Sprite cache for brightness-adjusted alpha variants. The composite key
	// avoids a per-frame fmt.Sprintf allocation that showed up in the hot draw
	// path (one call per visible transparent sprite per frame).
	processedSpriteCache map[processedSpriteKey]*ebiten.Image
	// Reusable buffer for tree hits to avoid allocation per frame
	treeHits []treeHitData
	// Unified sprite buffer for sorted rendering of all sprite types
	unifiedSprites []UnifiedSpriteRenderData
	// Cached average texture colour per tile type, used to tint the impassable
	// aura bubbles to match the rock/cliff sprite they rise from. Computed lazily.
	auraTileColorCache map[world.TileType3D][3]int
	// Reused draw options for glow quads (bubbles, projectile/arrow/slash glows).
	// Thousands of glow draws per frame would otherwise allocate one options
	// struct each; reset-and-reuse keeps the hot path allocation-free. Safe
	// because rendering is single-threaded and DrawImage reads it synchronously.
	glowOpts ebiten.DrawImageOptions
	// softGlowImg is a radial-gradient (opaque centre → transparent edge) white
	// texture for soft ROUND glows — used for spell projectile bodies/halos so a
	// big fireball reads as a fuzzy ball, not a hard square. Built lazily.
	softGlowImg *ebiten.Image
}

// NewRenderer creates a new renderer
func NewRenderer(game *MMGame) *Renderer {
	r := &Renderer{
		game:                     game,
		renderedSpritesThisFrame: make(map[[2]int]bool),
		processedSpriteCache:     make(map[processedSpriteKey]*ebiten.Image),
	}
	r.floorColorCache = make(map[[2]int]color.RGBA)
	r.precomputeFloorColorCache()
	// Create a 1x1 white image for DrawTriangles
	r.whiteImg = ebiten.NewImage(1, 1)
	r.whiteImg.Fill(color.White)

	screenWidth := game.config.GetScreenWidth()

	// Initialize transparent sprite cache
	r.buildTransparentSpriteCache()

	// Initialize ray direction cache
	rayWidth := game.config.Graphics.RaysPerScreenWidth
	numRays := (screenWidth + rayWidth - 1) / rayWidth // Round up to cover entire screen
	r.rayDirectionsX = make([]float64, numRays)
	r.rayDirectionsY = make([]float64, numRays)
	r.ensureRayHitBuffers(numRays)

	return r
}

// ensureRayHitBuffers (re)allocates the per-ray hit buffer array so each ray
// has its own backing slice. Workers write to disjoint indices, so no locks
// are needed. Initial capacity 8 covers typical hit counts; capacity grows
// once per ray if needed and is reused on subsequent frames.
func (r *Renderer) ensureRayHitBuffers(numRays int) {
	if len(r.rayHitBuffers) == numRays {
		return
	}
	r.rayHitBuffers = make([][]RaycastHit, numRays)
	for i := range r.rayHitBuffers {
		r.rayHitBuffers[i] = make([]RaycastHit, 0, 8)
	}
}

// handleResize reallocates fixed-size rendering buffers when the viewport
// size changes (e.g. fullscreen toggle, window resize). Callers must also
// update the depth buffer + sky/ground images on MMGame — see
// MMGame.handleResize.
func (r *Renderer) handleResize(screenWidth, screenHeight int) {
	if screenWidth <= 0 || screenHeight <= 0 {
		return
	}

	rayWidth := r.game.config.Graphics.RaysPerScreenWidth
	if rayWidth <= 0 {
		rayWidth = 1
	}
	numRays := (screenWidth + rayWidth - 1) / rayWidth
	if numRays <= 0 {
		numRays = 1
	}
	r.rayDirectionsX = make([]float64, numRays)
	r.rayDirectionsY = make([]float64, numRays)
	r.ensureRayHitBuffers(numRays)
}

// buildTransparentSpriteCache scans the world once to cache all transparent environment sprites
func (r *Renderer) buildTransparentSpriteCache() {
	r.processedSpriteCache = make(map[processedSpriteKey]*ebiten.Image)

	if world.GlobalTileManager == nil || r.game.GetCurrentWorld() == nil {
		r.transparentSpritesCache = nil
		r.tileLightCache = nil
		return
	}

	var cache []TransparentSpriteData
	var lights []LightSource
	worldWidth := r.game.GetCurrentWorld().Width
	worldHeight := r.game.GetCurrentWorld().Height
	tileSize := float64(r.game.config.GetTileSize())

	// Scan world once to find all transparent environment sprites
	for tileY := 0; tileY < worldHeight; tileY++ {
		for tileX := 0; tileX < worldWidth; tileX++ {
			// Calculate tile center world coordinates
			worldX, worldY := TileCenterFromTile(tileX, tileY, tileSize)

			// Get tile type at this position
			tileType := r.game.GetCurrentWorld().GetTileAt(worldX, worldY)

			if tileData := world.GlobalTileManager.GetTileData(tileType); tileData != nil && tileData.Light != nil && tileData.Light.Enabled {
				radius := tileData.Light.RadiusTiles * tileSize
				intensity := tileData.Light.Intensity
				if radius > 0 && intensity > 0 {
					lights = append(lights, LightSource{
						X:         worldX,
						Y:         worldY,
						Radius:    radius,
						Intensity: intensity,
					})
				}
			}

			// Check if it's a transparent environment sprite (trees are rendered separately via raycasting)
			if world.GlobalTileManager.GetRenderType(tileType) == "environment_sprite" &&
				world.GlobalTileManager.IsTransparent(tileType) {

				// Pick a stable variant now; load/process the image lazily in Draw.
				spriteName := r.selectEnvironmentSpriteName(tileType, tileX, tileY)
				if spriteName == "" {
					continue // Skip tiles without sprites
				}

				cache = append(cache, TransparentSpriteData{
					tileX:      tileX,
					tileY:      tileY,
					worldX:     worldX,
					worldY:     worldY,
					tileType:   tileType,
					spriteName: spriteName,
				})
			}
		}
	}

	r.transparentSpritesCache = cache
	r.tileLightCache = lights
}

func (r *Renderer) selectEnvironmentSpriteName(tileType world.TileType3D, tileX, tileY int) string {
	if world.GlobalTileManager == nil {
		return ""
	}
	baseName := world.GlobalTileManager.GetSprite(tileType)
	variants := r.game.sprites.GetSpriteVariants(baseName)
	if len(variants) == 0 {
		return baseName
	}
	index := tileX + tileY*31 + int(tileType)
	if index < 0 {
		index = -index
	}
	return variants[index%len(variants)]
}

// precomputeRayDirections calculates ray directions once per frame for performance
func (r *Renderer) precomputeRayDirections() {
	// Safety check: ensure ray direction cache is allocated
	if len(r.rayDirectionsX) == 0 || len(r.rayDirectionsY) == 0 {
		// Reallocate if needed
		rayWidth := r.game.config.Graphics.RaysPerScreenWidth
		if rayWidth <= 0 {
			rayWidth = 1
		}
		screenWidth := r.game.config.GetScreenWidth()
		if screenWidth <= 0 {
			screenWidth = 800
		}
		numRays := (screenWidth + rayWidth - 1) / rayWidth
		if numRays <= 0 {
			numRays = 1
		}
		r.rayDirectionsX = make([]float64, numRays)
		r.rayDirectionsY = make([]float64, numRays)
	}

	numRays := len(r.rayDirectionsX)
	if numRays <= 0 {
		return // Safety guard against zero-length cache
	}

	camAngle := r.game.camera.Angle
	fov := r.game.camera.FOV

	dirX := math.Cos(camAngle)
	dirY := math.Sin(camAngle)
	planeScale := math.Tan(fov / 2)
	planeX := -dirY * planeScale
	planeY := dirX * planeScale

	for i := 0; i < numRays; i++ {
		// Use the camera plane for ray directions so walls/floor/sprites align.
		cameraX := 2*(float64(i)+0.5)/float64(numRays) - 1
		r.rayDirectionsX[i] = dirX + planeX*cameraX
		r.rayDirectionsY[i] = dirY + planeY*cameraX
	}
}

func (r *Renderer) updateActiveLights() {
	r.activeLights = r.activeLights[:0]

	camX := r.game.camera.X
	camY := r.game.camera.Y
	viewDist := r.game.camera.ViewDist

	for _, light := range r.tileLightCache {
		radius := light.Radius
		if radius <= 0 || light.Intensity <= 0 {
			continue
		}
		maxDist := viewDist + radius
		dx := light.X - camX
		dy := light.Y - camY
		if dx*dx+dy*dy <= maxDist*maxDist {
			r.activeLights = append(r.activeLights, light)
		}
	}

	if world := r.game.GetCurrentWorld(); world != nil {
		for _, mon := range world.Monsters {
			if !mon.IsAlive() {
				continue
			}
			if mon.LightRadius <= 0 || mon.LightIntensity <= 0 {
				continue
			}
			maxDist := viewDist + mon.LightRadius
			dx := mon.X - camX
			dy := mon.Y - camY
			if dx*dx+dy*dy <= maxDist*maxDist {
				r.activeLights = append(r.activeLights, LightSource{
					X:         mon.X,
					Y:         mon.Y,
					Radius:    mon.LightRadius,
					Intensity: mon.LightIntensity,
				})
			}
		}
	}

	if r.game.torchLightActive && r.game.torchLightRadius > 0 {
		r.activeLights = append(r.activeLights, LightSource{
			X:         camX,
			Y:         camY,
			Radius:    r.game.torchLightRadius,
			Intensity: 0.25,
		})
	}
}

func (r *Renderer) applyLocalLight(brightness float64, sourceX, sourceY, worldX, worldY, radius, intensity float64) float64 {
	if radius <= 0 || intensity <= 0 {
		return brightness
	}
	distanceFromLight := Distance(sourceX, sourceY, worldX, worldY)
	if distanceFromLight > radius {
		return brightness
	}
	falloff := 1.0 - (distanceFromLight / radius)
	brightness += intensity * falloff
	if brightness > 1.0 {
		brightness = 1.0
	}
	return brightness
}

// calculateBrightnessWithTorchLight calculates brightness with torch light effects
func (r *Renderer) calculateBrightnessWithTorchLight(worldX, worldY, distance float64) float64 {
	// Base brightness calculation
	brightness := 1.0 - (distance / r.game.camera.ViewDist)
	if brightness < r.game.config.Graphics.BrightnessMin {
		brightness = r.game.config.Graphics.BrightnessMin
	}

	for _, light := range r.activeLights {
		brightness = r.applyLocalLight(brightness, light.X, light.Y, worldX, worldY, light.Radius, light.Intensity)
	}

	return brightness
}

// applyTreeDepthShading adds extra depth contrast for tree-like sprites.
func (r *Renderer) applyTreeDepthShading(brightness, distance float64) float64 {
	viewDist := r.game.camera.ViewDist
	if viewDist <= 0 {
		return brightness
	}

	depth := 1.0 - (distance / viewDist)
	if depth < 0 {
		depth = 0
	} else if depth > 1.0 {
		depth = 1.0
	}

	// Slightly brighten near trees and darken distant ones for added depth.
	brightness += (depth - 0.5) * 0.2 // ±0.1

	if brightness < r.game.config.Graphics.BrightnessMin {
		brightness = r.game.config.Graphics.BrightnessMin
	}
	if brightness > 1.0 {
		brightness = 1.0
	}
	return brightness
}

// precomputeFloorColorCache precalculates the floor color for every tile in the world
func (r *Renderer) precomputeFloorColorCache() {
	r.loadCurrentMapFloorTextures()

	// Get map-specific default floor color
	var defaultFloorColor [3]int
	if world.GlobalWorldManager != nil {
		mapConfig := world.GlobalWorldManager.GetCurrentMapConfig()
		currentMapKey := world.GlobalWorldManager.CurrentMapKey
		fmt.Printf("[FloorCache] CurrentMapKey: %s\n", currentMapKey)

		if mapConfig != nil {
			defaultFloorColor = mapConfig.DefaultFloorColor
			fmt.Printf("[FloorCache] Using map config - Biome: %s, FloorColor: %v\n", mapConfig.Biome, defaultFloorColor)
		} else {
			defaultFloorColor = [3]int{60, 180, 60} // Fallback green
			fmt.Printf("[FloorCache] No map config found, using fallback green\n")
		}
	} else {
		defaultFloorColor = [3]int{60, 180, 60} // Fallback green
		fmt.Printf("[FloorCache] No WorldManager, using fallback green\n")
	}

	defaultMapFloor := color.RGBA{uint8(defaultFloorColor[0]), uint8(defaultFloorColor[1]), uint8(defaultFloorColor[2]), 255}
	defaultDarkGreen := color.RGBA{20, 90, 20, 255} // Keep dark green for tree effects
	cache := make(map[[2]int]color.RGBA)

	// Estimate world bounds
	worldWidth, worldHeight := r.game.GetCurrentWorld().Width, r.game.GetCurrentWorld().Height

	// Precompute all tiles that affect nearby floor colors
	tilesWithFloorEffect := make(map[[2]int]world.TileType3D)
	for tileY := 0; tileY < worldHeight; tileY++ {
		for tileX := 0; tileX < worldWidth; tileX++ {
			checkX := float64(tileX)*float64(r.game.config.GetTileSize()) + float64(r.game.config.GetTileSize())/2
			checkY := float64(tileY)*float64(r.game.config.GetTileSize()) + float64(r.game.config.GetTileSize())/2
			t := r.game.GetCurrentWorld().GetTileAt(checkX, checkY)

			// Check if this tile affects nearby floor colors
			hasEffect := false
			if world.GlobalTileManager != nil {
				hasEffect = world.GlobalTileManager.HasFloorNearColor(t)
			}
			// Note: If tile manager not available, no tiles affect floor color

			if hasEffect {
				tilesWithFloorEffect[[2]int{tileX, tileY}] = t
			}
		}
	}

	// For each floor tile, check neighbors and determine color
	for tileY := 0; tileY < worldHeight; tileY++ {
		for tileX := 0; tileX < worldWidth; tileX++ {
			// Get base floor color for this tile
			checkX := float64(tileX)*float64(r.game.config.GetTileSize()) + float64(r.game.config.GetTileSize())/2
			checkY := float64(tileY)*float64(r.game.config.GetTileSize()) + float64(r.game.config.GetTileSize())/2
			currentTile := r.game.GetCurrentWorld().GetTileAt(checkX, checkY)

			baseColor := defaultMapFloor
			if world.GlobalTileManager != nil {
				// Only use tile-specific floor colors for non-empty tiles
				// Empty tiles should use the map's default floor color
				if currentTile != world.TileEmpty {
					if colorConfig := world.GlobalTileManager.GetFloorColor(currentTile); colorConfig != [3]int{0, 0, 0} {
						baseColor = color.RGBA{uint8(colorConfig[0]), uint8(colorConfig[1]), uint8(colorConfig[2]), 255}
					}
				}
				// For TileEmpty, keep using defaultMapFloor (map-specific color)
			}

			// Check if any nearby tiles affect this floor color
			nearSpecialTile := false
			var nearTileColor color.RGBA
			for dy := -1; dy <= 1 && !nearSpecialTile; dy++ {
				for dx := -1; dx <= 1 && !nearSpecialTile; dx++ {
					if neighborTile, ok := tilesWithFloorEffect[[2]int{tileX + dx, tileY + dy}]; ok {
						nearSpecialTile = true
						if world.GlobalTileManager != nil {
							if colorConfig := world.GlobalTileManager.GetFloorNearColor(neighborTile); colorConfig != [3]int{0, 0, 0} {
								nearTileColor = color.RGBA{uint8(colorConfig[0]), uint8(colorConfig[1]), uint8(colorConfig[2]), 255}
							} else {
								nearTileColor = defaultDarkGreen
							}
						} else {
							nearTileColor = defaultDarkGreen
						}
					}
				}
			}

			clr := baseColor
			// Apply nearby tile effect ONLY to empty '.' tiles
			if nearSpecialTile && currentTile == world.TileEmpty {
				clr = nearTileColor
			}
			cache[[2]int{tileX, tileY}] = clr
		}
	}

	// Highlight the player's spawn point tile using spawn tile configuration
	if r.game.GetCurrentWorld() != nil {
		spawnTileX := r.game.GetCurrentWorld().StartX
		spawnTileY := r.game.GetCurrentWorld().StartY

		// Get spawn color from tile configuration
		if world.GlobalTileManager != nil {
			spawnColor := world.GlobalTileManager.GetFloorColor(world.TileSpawn)
			cache[[2]int{spawnTileX, spawnTileY}] = color.RGBA{
				R: uint8(spawnColor[0]),
				G: uint8(spawnColor[1]),
				B: uint8(spawnColor[2]),
				A: 255,
			}
		}
	}

	r.floorColorCache = cache
	r.buildFloorColorMap(worldWidth, worldHeight)
}

// buildFloorColorMap encodes floorColorCache as a worldW×worldH RGBA8 image,
// one pixel per tile. A second one-pixel-per-tile image stores the floor
// texture atlas index selected for each tile.
func (r *Renderer) buildFloorColorMap(worldWidth, worldHeight int) {
	if worldWidth <= 0 || worldHeight <= 0 {
		r.floorColorMap = nil
		r.floorTextureIndexMap = nil
		return
	}
	if r.floorColorMap == nil ||
		r.floorColorMap.Bounds().Dx() != worldWidth ||
		r.floorColorMap.Bounds().Dy() != worldHeight {
		r.floorColorMap = ebiten.NewImage(worldWidth, worldHeight)
	}
	if r.floorTextureIndexMap == nil ||
		r.floorTextureIndexMap.Bounds().Dx() != worldWidth ||
		r.floorTextureIndexMap.Bounds().Dy() != worldHeight {
		r.floorTextureIndexMap = ebiten.NewImage(worldWidth, worldHeight)
	}

	colorPixels := make([]byte, worldWidth*worldHeight*4)
	indexPixels := make([]byte, worldWidth*worldHeight*4)
	hasTM := world.GlobalTileManager != nil
	for ty := 0; ty < worldHeight; ty++ {
		for tx := 0; tx < worldWidth; tx++ {
			clr := r.floorColorCache[[2]int{tx, ty}]
			tileType := world.TileEmpty
			if hasTM && r.game.world != nil &&
				tx >= 0 && tx < r.game.world.Width &&
				ty >= 0 && ty < r.game.world.Height {
				tileType = r.game.world.Tiles[ty][tx]
			}
			idx := (ty*worldWidth + tx) * 4
			colorPixels[idx] = clr.R
			colorPixels[idx+1] = clr.G
			colorPixels[idx+2] = clr.B
			colorPixels[idx+3] = 255

			// Shader reads only the R channel; G/B left zero, alpha 255 keeps
			// the image fully opaque so premultiplication is a no-op.
			indexPixels[idx+3] = 255
			if atlasIndex, ok := r.floorTextureIndexForTile(tx, ty, tileType); ok {
				indexPixels[idx] = uint8(atlasIndex + 1)
			}
		}
	}
	r.floorColorMap.WritePixels(colorPixels)
	r.floorTextureIndexMap.WritePixels(indexPixels)
}

func (r *Renderer) floorTextureIndexForTile(tileX, tileY int, tileType world.TileType3D) (int, bool) {
	groupName := r.floorTextureGroupForTile(tileX, tileY, tileType)
	group, ok := r.floorTexGroups[groupName]
	if !ok || group.count <= 0 {
		return 0, false
	}
	offset := stableFloorTextureIndex(tileX, tileY, int(tileType), group.count)
	return group.start + offset, true
}

// defaultFloorTextureGroup is the biome floor group used for any tile that
// doesn't name its own group — see floorTextureGroupForTile.
const defaultFloorTextureGroup = "default"

// floorTextureGroupForTile returns the floor-texture group name for a tile.
// Mapping is data-driven from tiles.yaml (TileData.FloorTextureGroup) with two
// fallbacks resolved here, not in the data:
//   - "beach": an "empty" tile bordering any water-group tile uses "beach"
//     instead of its own group, so shorelines transition into sand.
//   - "default": a tile that names no group AND paints no floor_color of its
//     own falls back to the biome's "default" floor (grass in forest, sand in
//     desert, …). This lets decorative objects (ferns, moss rocks, trees) sit
//     on the biome ground without hardcoding a group. Tiles that DO set a
//     floor_color — teleporters, traps, spawn — are coloured squares whose
//     color is their whole look, so they stay untextured. If the biome has no
//     "default" group, floorTextureIndexForTile falls back to the base color.
func (r *Renderer) floorTextureGroupForTile(tileX, tileY int, tileType world.TileType3D) string {
	if world.GlobalTileManager == nil {
		return ""
	}
	tileData := world.GlobalTileManager.GetTileData(tileType)
	if tileData == nil {
		return ""
	}
	group := tileData.FloorTextureGroup
	if group == "" {
		// No group: borrow the biome default only for tiles that paint no
		// floor of their own. A set floor_color means the color IS the look
		// (teleporter/trap/spawn) — leave it untextured.
		if tileData.FloorColor != ([3]int{0, 0, 0}) {
			return ""
		}
		group = defaultFloorTextureGroup
	}
	// Beach shoreline: any tile sitting on the biome's default ground that
	// borders water uses "beach" instead, so the sand transition is
	// continuous — including the ground under objects like palms, not just
	// bare empty tiles. Only when the current biome actually defines a
	// "beach" group (forest/desert), so city/church floors are unaffected.
	if group == defaultFloorTextureGroup && r.tileBordersWater(tileX, tileY) {
		if _, ok := r.floorTexGroups["beach"]; ok {
			return "beach"
		}
	}
	return group
}

func (r *Renderer) tileBordersWater(tileX, tileY int) bool {
	if r.game == nil || r.game.world == nil || world.GlobalTileManager == nil {
		return false
	}
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			nx, ny := tileX+dx, tileY+dy
			if nx < 0 || ny < 0 || nx >= r.game.world.Width || ny >= r.game.world.Height {
				continue
			}
			key := world.GlobalTileManager.GetTileKey(r.game.world.Tiles[ny][nx])
			if key == "water" || key == "deep_water" || key == "forest_stream" {
				return true
			}
		}
	}
	return false
}

func stableFloorTextureIndex(tileX, tileY, tileType, count int) int {
	if count <= 1 {
		return 0
	}
	hash := uint32(tileX)*73856093 ^ uint32(tileY)*19349663 ^ uint32(tileType)*83492791
	return int(hash % uint32(count))
}

func (r *Renderer) loadCurrentMapFloorTextures() {
	if world.GlobalWorldManager == nil {
		r.clearFloorAtlas()
		return
	}
	mapConfig := world.GlobalWorldManager.GetCurrentMapConfig()
	if mapConfig == nil {
		r.clearFloorAtlas()
		return
	}
	// Floor textures are biome-driven: every map of a biome shares the same
	// groups, so the atlas is cached per biome rather than per map file.
	groupSources := world.GlobalWorldManager.GetCurrentBiomeFloorTextureGroups()
	if len(groupSources) == 0 {
		r.clearFloorAtlas()
		return
	}
	if mapConfig.Biome == r.floorTexturesKey && r.floorTexAtlas != nil {
		return // same biome, atlas already built
	}
	groupNames := floorTextureGroupLoadOrder(groupSources)
	rawGroups := make(map[string][]floorTexture, len(groupNames))
	for _, name := range groupNames {
		texs := make([]floorTexture, 0, len(groupSources[name]))
		for _, texName := range groupSources[name] {
			tex, err := loadFloorTexture(texName)
			if err != nil {
				fmt.Printf("[FloorTextures] failed to load %q: %v\n", texName, err)
				continue
			}
			texs = append(texs, tex)
		}
		if len(texs) > 0 {
			rawGroups[name] = texs
		}
	}

	// Pick canonical dimensions from the first non-empty group; drop any
	// group containing a mismatched texture so we never leave black slots in
	// the atlas (every group is either fully present or fully absent).
	canonicalW, canonicalH := 0, 0
	for _, name := range groupNames {
		if texs := rawGroups[name]; len(texs) > 0 {
			canonicalW, canonicalH = texs[0].width, texs[0].height
			break
		}
	}
	textures := make([]floorTexture, 0)
	groups := make(map[string]floorTextureGroup)
	for _, name := range groupNames {
		texs := rawGroups[name]
		if len(texs) == 0 {
			continue
		}
		valid := true
		for _, t := range texs {
			if t.width != canonicalW || t.height != canonicalH {
				fmt.Printf("[FloorTextures] dropping group %q: %q is %dx%d, expected %dx%d\n",
					name, t.name, t.width, t.height, canonicalW, canonicalH)
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		start := len(textures)
		textures = append(textures, texs...)
		groups[name] = floorTextureGroup{start: start, count: len(texs)}
	}

	r.buildFloorTexAtlas(textures)
	r.floorTexGroups = groups
	r.floorTexturesKey = mapConfig.Biome
}

func (r *Renderer) clearFloorAtlas() {
	r.floorTexAtlas = nil
	r.floorTexGroups = nil
	r.floorTexCount = 0
	r.floorTexTileW = 0
	r.floorTexTileH = 0
	r.floorTexturesKey = ""
}

// floorTextureGroupLoadOrder returns group names sorted alphabetically. Order
// only affects atlas layout (start offset per group), not visuals — sorting is
// purely for deterministic atlas placement across runs.
func floorTextureGroupLoadOrder(groups map[string][]string) []string {
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// buildFloorTexAtlas packs the given source textures into a horizontal strip
// (tex[0] occupies x in [0, tileW), tex[1] in [tileW, 2*tileW), …). All
// textures must share dimensions — the caller pre-validates this so we never
// leave black slots in the atlas. The source slice is consumed here and not
// retained; the pixel data lives on the GPU once the atlas is built.
func (r *Renderer) buildFloorTexAtlas(textures []floorTexture) {
	if len(textures) == 0 {
		r.clearFloorAtlas()
		return
	}
	tileW := textures[0].width
	tileH := textures[0].height
	atlas := image.NewRGBA(image.Rect(0, 0, tileW*len(textures), tileH))
	for i, tex := range textures {
		for y := 0; y < tileH; y++ {
			srcRow := tex.pixels[y*tileW*4 : (y+1)*tileW*4]
			dstStart := y*atlas.Stride + i*tileW*4
			copy(atlas.Pix[dstStart:dstStart+tileW*4], srcRow)
		}
	}
	r.floorTexAtlas = ebiten.NewImageFromImage(atlas)
	r.floorTexCount = len(textures)
	r.floorTexTileW = tileW
	r.floorTexTileH = tileH
}

func loadFloorTexture(name string) (floorTexture, error) {
	src, err := decodePNG(resolveNamedPNG("assets/sprites/floor", name))
	if err != nil {
		return floorTexture{}, err
	}
	bounds := src.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(rgba, rgba.Bounds(), src, bounds.Min, draw.Src)

	return floorTexture{
		name:   name,
		width:  rgba.Bounds().Dx(),
		height: rgba.Bounds().Dy(),
		pixels: rgba.Pix,
	}, nil
}

// RenderFirstPersonView renders the complete first-person 3D view
func (r *Renderer) RenderFirstPersonView(screen *ebiten.Image) {
	r.renderFirstPerson3D(screen)
}

// renderFirstPerson3D performs the main 3D rendering using raycasting
func (r *Renderer) renderFirstPerson3D(screen *ebiten.Image) {
	// Clear environment sprite tracking for this frame
	for k := range r.renderedSpritesThisFrame {
		delete(r.renderedSpritesThisFrame, k)
	}

	r.updateActiveLights()

	// Draw background layers using helper
	r.game.renderHelper.RenderBackgroundLayers(screen)

	// Clear depth buffer for this frame - optimized with slice header manipulation
	viewDist := r.game.camera.ViewDist
	depthBuf := r.game.depthBuffer
	for i := range depthBuf {
		depthBuf[i] = viewDist
	}

	// Calculate ray parameters first
	rayWidth := r.game.config.Graphics.RaysPerScreenWidth
	if rayWidth <= 0 {
		rayWidth = 1 // Safety guard against zero/negative ray width
	}
	screenWidth := r.game.config.GetScreenWidth()
	if screenWidth <= 0 {
		screenWidth = 800 // Safety fallback
	}
	// Use ceil-division consistently to ensure all pixels are covered
	numRays := (screenWidth + rayWidth - 1) / rayWidth // Round up to cover entire screen
	if numRays <= 0 {
		numRays = 1 // Safety guard against zero rays
	}

	// Ensure precomputed ray directions match numRays BEFORE precomputing
	if len(r.rayDirectionsX) != numRays {
		r.rayDirectionsX = make([]float64, numRays)
		r.rayDirectionsY = make([]float64, numRays)
	}
	r.ensureRayHitBuffers(numRays)

	// Precompute ray directions AFTER ensuring correct array size
	r.precomputeRayDirections()

	// Perform raycasting in parallel with performance monitoring using precomputed directions
	raycastTimer := r.game.threading.PerformanceMonitor.StartRaycast()
	results := r.game.threading.ParallelRenderer.RenderRaycast(
		numRays,
		r.castRayWithPrecomputedDirection,
	)
	raycastTimer.EndRaycast()

	// Draw simple floor and ceiling before walls/trees so trees are visible above floor
	r.game.threading.PerformanceMonitor.ProfiledFunction("sprite_render", func() {
		r.drawSimpleFloorCeiling(screen)

		// Render the results and update depth buffer
		r.renderRaycastResults(screen, results)

		// Draw all sprites (trees, ferns, monsters, NPCs) sorted by depth
		r.drawAllSpritesSorted(screen)

		// Highlight impassable billboard tiles with rising ground bubbles
		// (after walls/sprites so the depth buffer is populated for occlusion).
		r.drawImpassableTileAura(screen)

		// Draw fireballs and sword attacks
		r.drawProjectiles(screen)

		// Draw slash effects
		r.drawSlashEffects(screen)

		// Draw hit effects (spell particles, arrow bursts)
		r.drawHitEffects(screen)
	})
}

// RaycastHit contains the result of a DDA raycast operation.
// This follows the Digital Differential Analysis algorithm for efficient grid traversal.
type RaycastHit struct {
	Distance      float64          // Perpendicular distance to the wall (prevents fisheye effect)
	TileType      world.TileType3D // Type of tile that was hit
	WallSide      int              // 0 for north-south walls, 1 for east-west walls (used for shading)
	TextureCoord  float64          // Wall hit position for texture mapping (0.0 to 1.0)
	IsTransparent bool             // Whether this hit should be rendered transparently
}

// MultiRaycastHit contains multiple hits for a single ray (for transparency support)
type MultiRaycastHit struct {
	Hits []RaycastHit
}

// castRayWithType casts a single ray and returns distance and hit information.
// Cold fallback path — passes nil so the raycast allocates its own slice; the
// per-ray buffer reuse path goes through castRayWithPrecomputedDirection.
func (r *Renderer) castRayWithType(angle float64) (float64, interface{}) {
	hits := r.performMultiHitRaycast(angle, nil)
	// If there are no hits, it means the ray went into the void.
	if len(hits.Hits) == 0 {
		return r.game.camera.ViewDist, hits
	}

	// The primary distance for depth sorting should be the first solid object hit.
	// If no solid object is hit, we use the distance of the furthest transparent object,
	// but this is less ideal. A better approach is to ensure a "skybox" or max distance wall is always hit.
	// For now, we find the first solid hit from the front.
	for _, hit := range hits.Hits {
		if !hit.IsTransparent {
			return hit.Distance, hits
		}
	}

	// If no solid wall was hit, return the distance of the closest transparent object.
	// This prevents sprites from appearing behind transparent things when there's no wall.
	return hits.Hits[0].Distance, hits
}

// castRayWithPrecomputedDirection casts a single ray using precomputed sin/cos values
func (r *Renderer) castRayWithPrecomputedDirection(rayIndex int) (float64, interface{}) {
	// Safety guard: check ray index bounds
	if rayIndex < 0 || rayIndex >= len(r.rayDirectionsX) || rayIndex >= len(r.rayDirectionsY) {
		// Fallback to angle-based calculation
		camAngle := r.game.camera.Angle
		fov := r.game.camera.FOV
		totalRays := len(r.rayDirectionsX)
		if totalRays <= 0 {
			totalRays = 1
		}
		angle := camAngle - fov/2 + (float64(rayIndex)/float64(totalRays))*fov
		return r.castRayWithType(angle)
	}

	// Use precomputed ray directions instead of recalculating sin/cos
	rayDirectionX := r.rayDirectionsX[rayIndex]
	rayDirectionY := r.rayDirectionsY[rayIndex]

	// Reuse this ray's pre-allocated hit buffer (different rayIndex per worker
	// → no race). Capacity is retained across frames; only first few frames
	// may grow the slice's backing.
	buf := r.rayHitBuffers[rayIndex][:0]
	hits := r.performMultiHitRaycastWithDirection(rayDirectionX, rayDirectionY, buf)
	r.rayHitBuffers[rayIndex] = hits.Hits
	// If there are no hits, it means the ray went into the void.
	if len(hits.Hits) == 0 {
		return r.game.camera.ViewDist, hits
	}

	// The primary distance for depth sorting should be the first solid object hit.
	for _, hit := range hits.Hits {
		if !hit.IsTransparent {
			return hit.Distance, hits
		}
	}

	// If no solid wall was hit, return the distance of the closest transparent object.
	return hits.Hits[0].Distance, hits
}

// performMultiHitRaycast performs DDA raycasting that can return multiple hits for transparency
func (r *Renderer) performMultiHitRaycast(angle float64, hits []RaycastHit) MultiRaycastHit {
	// Calculate ray direction vector from the given angle
	rayDirectionX := math.Cos(angle)
	rayDirectionY := math.Sin(angle)

	return r.performMultiHitRaycastWithDirection(rayDirectionX, rayDirectionY, hits)
}

// performMultiHitRaycastWithDirection performs DDA raycasting using precomputed
// ray directions. The hits parameter is a pre-allocated slice to append into;
// pass nil to allocate fresh (cold path only).
func (r *Renderer) performMultiHitRaycastWithDirection(rayDirectionX, rayDirectionY float64, hits []RaycastHit) MultiRaycastHit {
	// Get starting position in world coordinates
	startWorldX := r.game.camera.X
	startWorldY := r.game.camera.Y

	// Convert world coordinates to tile/grid coordinates for DDA algorithm
	tileSize := r.game.config.GetTileSize()
	currentTileX := int(startWorldX / tileSize)
	currentTileY := int(startWorldY / tileSize)

	// Calculate position within the current tile (normalized to 0.0-1.0 range)
	positionInTileX := (startWorldX / tileSize) - float64(currentTileX)
	positionInTileY := (startWorldY / tileSize) - float64(currentTileY)

	// Calculate delta distances - how far the ray travels to cross one grid line
	var deltaDistanceX, deltaDistanceY float64
	if rayDirectionX == 0 {
		deltaDistanceX = 1e30
	} else {
		deltaDistanceX = math.Abs(1 / rayDirectionX)
	}
	if rayDirectionY == 0 {
		deltaDistanceY = 1e30
	} else {
		deltaDistanceY = math.Abs(1 / rayDirectionY)
	}

	// Calculate step directions and initial distances to next grid lines
	var stepDirectionX, stepDirectionY int
	var distanceToNextGridLineX, distanceToNextGridLineY float64

	if rayDirectionX < 0 {
		stepDirectionX = -1
		distanceToNextGridLineX = positionInTileX * deltaDistanceX
	} else {
		stepDirectionX = 1
		distanceToNextGridLineX = (1.0 - positionInTileX) * deltaDistanceX
	}

	if rayDirectionY < 0 {
		stepDirectionY = -1
		distanceToNextGridLineY = positionInTileY * deltaDistanceY
	} else {
		stepDirectionY = 1
		distanceToNextGridLineY = (1.0 - positionInTileY) * deltaDistanceY
	}

	// Execute the DDA algorithm. `hits` was passed in (possibly nil for the
	// cold path or a reused per-ray buffer for hot path).
	maxSteps := int(r.game.camera.ViewDist/tileSize) + 2 // A little margin
	wallSide := 0

	for steps := 0; steps < maxSteps; steps++ {
		// Choose which direction to step
		if distanceToNextGridLineX < distanceToNextGridLineY {
			distanceToNextGridLineX += deltaDistanceX
			currentTileX += stepDirectionX
			wallSide = 0
		} else {
			distanceToNextGridLineY += deltaDistanceY
			currentTileY += stepDirectionY
			wallSide = 1
		}

		// Check what tile we've stepped into
		worldX, worldY := TileCenterFromTile(currentTileX, currentTileY, tileSize)
		tileType := r.game.GetCurrentWorld().GetTileAt(worldX, worldY)

		// If we hit an empty tile, we can just continue
		if tileType == world.TileEmpty {
			continue
		}

		// Calculate distance
		var perpendicularDistance float64
		if wallSide == 0 {
			perpendicularDistance = (float64(currentTileX) - startWorldX/tileSize + (1-float64(stepDirectionX))/2) / rayDirectionX
		} else {
			perpendicularDistance = (float64(currentTileY) - startWorldY/tileSize + (1-float64(stepDirectionY))/2) / rayDirectionY
		}

		// If distance is too far, stop here.
		if perpendicularDistance*tileSize > r.game.camera.ViewDist {
			return MultiRaycastHit{Hits: hits}
		}

		// Calculate texture coordinate
		var textureCoordinate float64
		if wallSide == 0 {
			textureCoordinate = startWorldY/tileSize + perpendicularDistance*rayDirectionY
		} else {
			textureCoordinate = startWorldX/tileSize + perpendicularDistance*rayDirectionX
		}
		textureCoordinate -= math.Floor(textureCoordinate)

		// Fix texture mirroring on wall faces based on ray direction
		if wallSide == 0 && rayDirectionX > 0 {
			textureCoordinate = 1 - textureCoordinate
		}
		if wallSide == 1 && rayDirectionY < 0 {
			textureCoordinate = 1 - textureCoordinate
		}

		// Check what type of tile this is and use tile manager for properties
		isTransparent := false
		if world.GlobalTileManager != nil {
			isTransparent = world.GlobalTileManager.IsTransparent(tileType)
		}
		// Note: If tile manager is not available, default to solid (non-transparent)

		if isTransparent {
			// Skip transparent tiles that are floor-only (they never render in the ray pass).
			if world.GlobalTileManager != nil {
				renderType := world.GlobalTileManager.GetRenderType(tileType)
				if renderType == "floor_only" {
					continue
				}
			}
			// Transparent tiles: add as transparent hit but continue ray
			hits = append(hits, RaycastHit{
				Distance:      perpendicularDistance * tileSize,
				TileType:      tileType,
				WallSide:      wallSide,
				TextureCoord:  textureCoordinate,
				IsTransparent: true,
			})
		} else {
			// Solid tile: add hit and stop ray
			hits = append(hits, RaycastHit{
				Distance:      perpendicularDistance * tileSize,
				TileType:      tileType,
				WallSide:      wallSide,
				TextureCoord:  textureCoordinate,
				IsTransparent: false,
			})
			return MultiRaycastHit{Hits: hits}
		}
	}

	return MultiRaycastHit{Hits: hits}
}

// treeHitData stores tree hit information for sorted rendering
type treeHitData struct {
	screenX  int
	distance float64
	tileType world.TileType3D
}

// renderRaycastResults processes and renders the results from parallel raycasting.
// Each result contains distance and hit information for one vertical screen column.
// Tree sprites are collected and rendered in the unified sprite pass for proper transparency.
func (r *Renderer) renderRaycastResults(screen *ebiten.Image, results []rendering.RaycastResult) {
	rayWidth := r.game.config.Graphics.RaysPerScreenWidth
	screenWidth := r.game.config.GetScreenWidth()

	for columnIndex, rayResult := range results {
		screenX := columnIndex * rayWidth

		// Clamp the last slice width to remaining screen pixels
		currentRayWidth := rayWidth
		if screenX+rayWidth > screenWidth {
			currentRayWidth = screenWidth - screenX
			if currentRayWidth <= 0 {
				break // No more screen space to draw
			}
		}

		// Handle both single hits and multi-hits for transparency
		switch hitData := rayResult.TileType.(type) {
		case MultiRaycastHit:
			if len(hitData.Hits) == 0 {
				continue
			}

			// Render all hits from back to front for proper transparency
			for i := len(hitData.Hits) - 1; i >= 0; i-- {
				hit := hitData.Hits[i]

				// Update depth buffer only with solid objects
				if !hit.IsTransparent {
					for dx := 0; dx < currentRayWidth; dx++ {
						if screenX+dx < len(r.game.depthBuffer) {
							r.game.depthBuffer[screenX+dx] = hit.Distance
						}
					}
				}

				// Collect tree hits for later sorted rendering
				if world.GlobalTileManager != nil && world.GlobalTileManager.GetRenderType(hit.TileType) == "tree_sprite" {
					r.treeHits = append(r.treeHits, treeHitData{
						screenX:  screenX,
						distance: hit.Distance,
						tileType: hit.TileType,
					})
					continue
				}

				// Render this hit
				r.renderSingleHit(screen, screenX, hit, currentRayWidth)
			}
		case RaycastHit:
			// This case should ideally not be hit with the new system, but as a fallback:
			hitInfo := hitData

			// Update depth buffer for proper sprite occlusion
			for dx := 0; dx < currentRayWidth; dx++ {
				if screenX+dx < len(r.game.depthBuffer) {
					r.game.depthBuffer[screenX+dx] = rayResult.Distance
				}
			}

			// Collect tree hits for later sorted rendering
			if world.GlobalTileManager != nil && world.GlobalTileManager.GetRenderType(hitInfo.TileType) == "tree_sprite" {
				r.treeHits = append(r.treeHits, treeHitData{
					screenX:  screenX,
					distance: hitInfo.Distance,
					tileType: hitInfo.TileType,
				})
				continue
			}

			// Render this hit
			r.renderSingleHit(screen, screenX, hitInfo, currentRayWidth)
		}
	}
}

// renderSingleHit renders a single raycast hit
func (r *Renderer) renderSingleHit(screen *ebiten.Image, screenX int, hit RaycastHit, rayWidth int) {
	tileType := hit.TileType

	// Do not render empty tiles
	if tileType == world.TileEmpty {
		return
	}

	// Render different tile types using tile manager configuration
	if world.GlobalTileManager != nil {
		renderType := world.GlobalTileManager.GetRenderType(tileType)
		switch renderType {
		case "tree_sprite":
			r.drawTreeSprite(screen, screenX, hit.Distance, tileType)
		case "environment_sprite":
			// Skip transparent environment sprites in raycasting - they'll be rendered in sprite phase
			// Use both hit.IsTransparent flag and tile manager check for safety
			if hit.IsTransparent {
				return // Skip transparent environment sprites - rendered in unified sprite pass
			}
			r.drawEnvironmentSpriteOnce(screen, screenX, hit.Distance, tileType)
		case "flooring_object":
			r.drawEnvironmentSprite(screen, screenX, hit.Distance, tileType)
		case "textured_wall":
			r.drawTexturedWallSlice(screen, screenX, hit.Distance, tileType, rayWidth,
				hit.WallSide, hit.TextureCoord)
		case "floor_only":
			// Floor-only tiles don't render anything here, just floor
			// These should be transparent so rays continue through them
			return
		}
	} else {
		// If tile manager not available, render as textured wall by default
		r.drawTexturedWallSlice(screen, screenX, hit.Distance, tileType, rayWidth,
			hit.WallSide, hit.TextureCoord)
	}
}

const maxFloorShaderLights = 16

// drawSimpleFloorCeiling renders the perspective floor entirely on the GPU
// via a Kage shader (see floorShaderSrc). Per-fragment work: reverse-project
// screen → world → tile, look up base color, optionally blend a hash-selected
// floor texture, then apply distance shading plus up to maxFloorShaderLights
// point lights.
//
// The shader does NOT exactly match the previous CPU loop:
//   - hash uses smaller multipliers (73 / 19) due to int32 overflow in Kage
//     where CPU used 73856093 / 19349663
//   - texture contribution fades out by distance via smoothstep on the texel
//     footprint per screen pixel, to avoid far-field stripes from nearest
//     sampling
//
// Per-tile variation pattern is similar; absolute texture index per tile
// will differ from the old CPU rendering.
func (r *Renderer) drawSimpleFloorCeiling(screen *ebiten.Image) {
	shader, err := r.ensureFloorShader()
	if err != nil || shader == nil || r.floorColorMap == nil || r.floorTextureIndexMap == nil || r.game.world == nil {
		return
	}

	screenWidth := r.game.config.GetScreenWidth()
	screenHeight := r.game.config.GetScreenHeight()
	tileSize := r.game.config.GetTileSize()
	camX := r.game.camera.X
	camY := r.game.camera.Y
	camAngle := r.game.camera.Angle
	fov := r.game.camera.FOV
	horizon := float64(screenHeight) / 2

	cosA := math.Cos(camAngle)
	sinA := math.Sin(camAngle)
	planeX := math.Cos(camAngle+math.Pi/2) * math.Tan(fov/2)
	planeY := math.Sin(camAngle+math.Pi/2) * math.Tan(fov/2)

	worldW := r.floorColorMap.Bounds().Dx()
	worldH := r.floorColorMap.Bounds().Dy()

	texAtlas := r.floorTexAtlas
	if texAtlas == nil {
		texAtlas = r.whiteImg // dummy; shader skips sampling when TexCount == 0
	}

	lights := r.floorLightsBuf[:]
	lightCount := len(r.activeLights)
	if lightCount > maxFloorShaderLights {
		lightCount = maxFloorShaderLights
	}
	for i := 0; i < lightCount; i++ {
		l := r.activeLights[i]
		lights[i*4] = float32(l.X)
		lights[i*4+1] = float32(l.Y)
		lights[i*4+2] = float32(l.Radius)
		lights[i*4+3] = float32(l.Intensity)
	}
	// Zero out unused slots so previous frame's data doesn't bleed in.
	for i := lightCount * 4; i < len(lights); i++ {
		lights[i] = 0
	}

	uniforms := map[string]any{
		"CamPos":        []float32{float32(camX), float32(camY)},
		"DirCos":        float32(cosA),
		"DirSin":        float32(sinA),
		"PlaneCos":      float32(planeX),
		"PlaneSin":      float32(planeY),
		"ScreenSize":    []float32{float32(screenWidth), float32(screenHeight)},
		"Horizon":       float32(horizon),
		"RowDistFactor": float32(0.5 * float64(screenHeight) * float64(tileSize)),
		"TileSize":      float32(tileSize),
		"WorldSize":     []float32{float32(worldW), float32(worldH)},
		"ViewDist":      float32(r.game.camera.ViewDist),
		"MinBrightness": float32(r.game.config.Graphics.BrightnessMin),
		"TexCount":      float32(r.floorTexCount),
		"TexTileSize":   []float32{float32(r.floorTexTileW), float32(r.floorTexTileH)},
		"LightCount":    float32(lightCount),
		"Lights":        lights,
	}

	x0 := float32(0)
	x1 := float32(screenWidth)
	y0 := float32(horizon)
	y1 := float32(screenHeight)
	vertices := [4]ebiten.Vertex{
		{DstX: x0, DstY: y0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: x1, DstY: y0, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: x0, DstY: y1, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: x1, DstY: y1, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
	indices := [6]uint16{0, 1, 2, 1, 3, 2}
	op := &ebiten.DrawTrianglesShaderOptions{Uniforms: uniforms}
	op.Images[0] = r.floorColorMap
	op.Images[1] = texAtlas
	op.Images[2] = r.floorTextureIndexMap
	screen.DrawTrianglesShader(vertices[:], indices[:], shader, op)
}

func (r *Renderer) ensureFloorShader() (*ebiten.Shader, error) {
	if r.floorShader != nil {
		return r.floorShader, nil
	}
	s, err := ebiten.NewShader([]byte(floorShaderSrc))
	if err != nil {
		return nil, err
	}
	r.floorShader = s
	return s, nil
}

// drawTreeSprite draws tree sprites in the 3D world
func (r *Renderer) drawTreeSprite(screen *ebiten.Image, x int, distance float64, tileType world.TileType3D) {
	screenHeight := r.game.config.GetScreenHeight()

	// Minimum distance to prevent extreme scaling and floor projection going off-screen.
	// At tileSize/2 distance, the tree fills most of the screen properly.
	// Below this, the floor projection formula breaks down.
	minDist := float64(r.game.config.GetTileSize()) / 2
	if distance < minDist {
		distance = minDist
	}

	// Calculate tree height and position
	// distance is already perpendicular distance from the raycast
	spriteHeight := r.game.renderHelper.calculateSpriteSizeWithHeightMultiplier(distance, r.game.config.Graphics.Sprite.TreeHeightMultiplier)
	if spriteHeight < 8 {
		spriteHeight = 8
	}

	// Cap sprite height to prevent extreme values at very close distances
	// (4x screen height allows tree to extend well off-screen while staying reasonable)
	if spriteHeight > screenHeight*4 {
		spriteHeight = screenHeight * 4
	}

	spriteWidth := int(float64(spriteHeight) * r.game.config.Graphics.Sprite.TreeWidthMultiplier)

	// Anchor tree's bottom to the floor at its distance
	// Use the same floor projection formula as other sprites for consistency
	floorScreenY := r.game.renderHelper.calculateFloorScreenY(distance)
	spriteTop := floorScreenY - spriteHeight
	spriteLeft := x - spriteWidth/2

	// Get appropriate tree sprite using tile manager
	var spriteName string
	if world.GlobalTileManager != nil {
		spriteName = world.GlobalTileManager.GetSprite(tileType)
	}

	// Fallback to default sprite if not configured
	if spriteName == "" {
		spriteName = "tree"
	}

	sprite := r.game.sprites.GetSprite(spriteName)

	// Scale and draw the tree sprite
	opts := &ebiten.DrawImageOptions{}
	scaleX := float64(spriteWidth) / float64(sprite.Bounds().Dx())
	scaleY := float64(spriteHeight) / float64(sprite.Bounds().Dy())
	opts.GeoM.Scale(scaleX, scaleY)
	opts.GeoM.Translate(float64(spriteLeft), float64(spriteTop))

	// Apply distance shading with torch light effects
	// For tree sprites, use camera position as approximation
	brightness := r.calculateBrightnessWithTorchLight(r.game.camera.X, r.game.camera.Y, distance)
	brightness = r.applyTreeDepthShading(brightness, distance)
	opts.ColorScale.Scale(float32(brightness), float32(brightness), float32(brightness), 1.0)

	// Use composite mode to ensure opaque rendering (no blending with background)
	opts.Blend = ebiten.BlendSourceOver

	screen.DrawImage(sprite, opts)
}

// processedSpriteKey identifies a (tileType, spriteName) pair without the
// allocation that fmt.Sprintf would impose on every cache lookup.
type processedSpriteKey struct {
	tileType   world.TileType3D
	spriteName string
}

func (r *Renderer) getProcessedSpriteByName(tileType world.TileType3D, spriteName string) *ebiten.Image {
	if spriteName == "" {
		return nil
	}
	sprite := r.game.sprites.GetSprite(spriteName)
	if world.GlobalTileManager == nil {
		return sprite
	}
	tileData := world.GlobalTileManager.GetTileData(tileType)
	if tileData == nil || tileData.AlphaFromBrightness <= 0 {
		return sprite
	}

	cacheKey := processedSpriteKey{tileType: tileType, spriteName: spriteName}
	if cached, ok := r.processedSpriteCache[cacheKey]; ok {
		return cached
	}
	processed := applyBrightnessToAlpha(sprite, tileData.AlphaFromBrightness)
	r.processedSpriteCache[cacheKey] = processed
	return processed
}

func applyBrightnessToAlpha(sprite *ebiten.Image, strength float64) *ebiten.Image {
	if sprite == nil || strength <= 0 {
		return sprite
	}
	if strength > 1 {
		strength = 1
	}
	w := sprite.Bounds().Dx()
	h := sprite.Bounds().Dy()
	if w <= 0 || h <= 0 {
		return sprite
	}

	pixels := make([]byte, 4*w*h)
	sprite.ReadPixels(pixels)
	for i := 0; i < len(pixels); i += 4 {
		a := pixels[i+3]
		if a == 0 {
			continue
		}
		rv := float64(pixels[i])
		gv := float64(pixels[i+1])
		bv := float64(pixels[i+2])
		maxv := math.Max(rv, math.Max(gv, bv))
		minv := math.Min(rv, math.Min(gv, bv))
		brightness := (rv + gv + bv) / (3.0 * 255.0)
		saturation := 0.0
		if maxv > 0 {
			saturation = (maxv - minv) / maxv
		}
		whiteness := brightness * (1.0 - saturation)
		whiteness = math.Min(1.0, whiteness*1.15)
		alphaScale := 0.0
		if rv >= 230 && gv >= 230 && bv >= 230 {
			alphaScale = 0
		} else {
			alphaScale = 1.0 - whiteness*strength
		}
		if alphaScale < 0 {
			alphaScale = 0
		}
		pixels[i] = uint8(rv*alphaScale + 0.5)
		pixels[i+1] = uint8(gv*alphaScale + 0.5)
		pixels[i+2] = uint8(bv*alphaScale + 0.5)
		pixels[i+3] = uint8(float64(a)*alphaScale + 0.5)
	}

	img := ebiten.NewImage(w, h)
	img.WritePixels(pixels)
	return img
}

// drawEnvironmentSprite draws environment sprites in the 3D world
func (r *Renderer) drawEnvironmentSprite(screen *ebiten.Image, x int, distance float64, tileType world.TileType3D) {
	spriteHeight := r.game.renderHelper.calculateSpriteSizeWithHeightMultiplier(distance, r.game.config.Graphics.Sprite.TreeHeightMultiplier)
	if spriteHeight > r.game.config.GetScreenHeight() {
		spriteHeight = r.game.config.GetScreenHeight()
	}
	if spriteHeight < 8 {
		spriteHeight = 8
	}

	spriteWidth := int(float64(spriteHeight) * r.game.config.Graphics.Sprite.TreeWidthMultiplier)
	spriteTop := (r.game.config.GetScreenHeight() - spriteHeight) / 2

	// Update depth buffer for central 85% of sprite width only if this tile is opaque
	// This prevents transparent edges from occluding objects behind them
	// Transparent floor objects like clearings should not occlude monsters/NPCs at all
	spriteLeft := x - spriteWidth/2
	spriteRight := x + spriteWidth/2
	isOpaque := true
	if world.GlobalTileManager != nil {
		isOpaque = !world.GlobalTileManager.IsTransparent(tileType)
	}
	if isOpaque {
		depthMargin := spriteWidth * 7 / 100 // 7% margin on each side
		depthLeft := spriteLeft + depthMargin
		depthRight := spriteRight - depthMargin
		for px := depthLeft; px <= depthRight && px >= 0 && px < len(r.game.depthBuffer); px++ {
			if distance < r.game.depthBuffer[px] {
				r.game.depthBuffer[px] = distance
			}
		}
	}

	var spriteName string
	if world.GlobalTileManager != nil {
		spriteName = world.GlobalTileManager.GetSprite(tileType)
	}
	if spriteName == "" {
		return // No sprite defined for this tile type
	}
	sprite := r.game.sprites.GetSprite(spriteName)

	// Scale and draw the sprite
	opts := &ebiten.DrawImageOptions{}
	scaleX := float64(spriteWidth) / float64(sprite.Bounds().Dx())
	scaleY := float64(spriteHeight) / float64(sprite.Bounds().Dy())
	opts.GeoM.Scale(scaleX, scaleY)
	opts.GeoM.Translate(float64(spriteLeft), float64(spriteTop))

	// Apply distance shading
	brightness := 1.0 - (distance / r.game.camera.ViewDist)
	if brightness < r.game.config.Graphics.BrightnessMin {
		brightness = r.game.config.Graphics.BrightnessMin
	}
	opts.ColorScale.Scale(float32(brightness), float32(brightness), float32(brightness), 1.0)

	// Use composite mode to ensure opaque rendering
	opts.Blend = ebiten.BlendSourceOver

	screen.DrawImage(sprite, opts)
}

// drawEnvironmentSpriteOnce draws environment sprites only once per frame per tile location
func (r *Renderer) drawEnvironmentSpriteOnce(screen *ebiten.Image, x int, distance float64, tileType world.TileType3D) {
	// Calculate tile coordinates for tracking
	tileSize := r.game.config.GetTileSize()
	camX := r.game.camera.X
	camY := r.game.camera.Y
	// FIXED: Use ray index instead of pixel x to compute angle correctly when rayWidth > 1
	rayWidth := r.game.config.Graphics.RaysPerScreenWidth
	if rayWidth <= 0 {
		rayWidth = 1
	}
	screenWidth := r.game.config.GetScreenWidth()
	rayIndex := x / rayWidth
	numRays := (screenWidth + rayWidth - 1) / rayWidth // Use ceil-division consistently
	angle := r.game.camera.Angle - r.game.camera.FOV/2 + (float64(rayIndex)/float64(numRays))*r.game.camera.FOV

	// Calculate the world position where this ray hits
	// FIXED: Convert perpendicular distance to ray length for correct world coordinates
	rayDirX := math.Cos(angle)
	rayDirY := math.Sin(angle)
	// Convert perpendicular distance to actual ray length
	rayLength := distance / math.Cos(angle-r.game.camera.Angle)
	hitWorldX := camX + rayDirX*rayLength
	hitWorldY := camY + rayDirY*rayLength

	// Convert to tile coordinates
	tileX := int(hitWorldX / float64(tileSize))
	tileY := int(hitWorldY / float64(tileSize))
	tileKey := [2]int{tileX, tileY}

	// Check if we've already rendered this sprite this frame
	if r.renderedSpritesThisFrame[tileKey] {
		return // Skip rendering - already done this frame
	}

	// Mark this sprite as rendered for this frame
	r.renderedSpritesThisFrame[tileKey] = true

	// Render the sprite using the existing function
	r.drawEnvironmentSprite(screen, x, distance, tileType)
}

// drawTexturedWallSlice renders a single vertical wall slice with texturing and proper shading.
// This is optimized with caching to avoid recreating similar wall slices every frame.
func (r *Renderer) drawTexturedWallSlice(screen *ebiten.Image, screenX int, distance float64, tileType world.TileType3D, width, wallSide int, textureCoord float64) {
	// Calculate wall dimensions based on distance and tile-specific height
	heightMultiplier := world.GetTileHeight(tileType)
	wallHeight, wallTop := r.game.renderHelper.CalculateWallDimensionsWithHeight(distance, heightMultiplier)

	// Sprite-textured walls bypass the WallSliceCache: every ray has its own
	// continuous textureCoord (float), so cache keys never collide and caching
	// would just allocate one new image per ray. Draw the strip directly.
	if world.GlobalTileManager != nil {
		if spriteName := world.GlobalTileManager.GetSprite(tileType); spriteName != "" {
			sprite := r.game.sprites.GetSprite(spriteName)
			if sprite != nil {
				r.drawSpriteTexturedWallSlice(screen, sprite, screenX, wallTop, wallHeight, width, wallSide, textureCoord, distance)
				return
			}
		}
	}

	// Cached path for procedural / color-only walls. Discrete TileType + integer
	// width/height/side/wallX make cache hits useful here.
	cacheKey := rendering.WallSliceKey{
		Height:   wallHeight,
		Width:    width,
		TileType: int(tileType),
		Side:     wallSide,
		WallX:    textureCoord,
	}

	wallSliceImage := r.game.threading.WallSliceCache.GetOrCreate(cacheKey, func(quantizedHeight int) *ebiten.Image {
		return r.game.renderHelper.CreateBaseTexturedWallSlice(tileType, width, quantizedHeight, wallSide, textureCoord)
	})

	drawOptions := &ebiten.DrawImageOptions{}
	cachedHeight := wallSliceImage.Bounds().Dy()
	if cachedHeight > 0 && wallHeight != cachedHeight {
		scaleY := float64(wallHeight) / float64(cachedHeight)
		drawOptions.GeoM.Scale(1.0, scaleY)
	}
	drawOptions.GeoM.Translate(float64(screenX), float64(wallTop))

	// Apply distance-based color scaling at draw time for better cache efficiency
	brightness := 1.0 - (distance / r.game.camera.ViewDist)
	if brightness < r.game.config.Graphics.BrightnessMin {
		brightness = r.game.config.Graphics.BrightnessMin
	}
	drawOptions.ColorScale.Scale(float32(brightness), float32(brightness), float32(brightness), 1.0)

	screen.DrawImage(wallSliceImage, drawOptions)
}

// drawSpriteTexturedWallSlice draws a single ray's slice of a sprite-textured
// wall straight to the screen. The textureCoord is continuous per ray, so this
// deliberately skips the WallSliceCache (which keys on it as a float and would
// just churn one entry per ray).
//
// Classic raycasting: one ray corresponds to ONE column of the wall texture,
// stretched across `width` screen pixels (rayWidth). Sampling rayWidth source
// columns per ray instead of one used to make the texture shimmer as the camera
// panned — adjacent rays' integer-truncated textureX values jumped 1..N source
// pixels at a time and showed disjoint strips. Stick with a single column and
// let the horizontal stretch handle the screen width.
func (r *Renderer) drawSpriteTexturedWallSlice(screen *ebiten.Image, sprite *ebiten.Image, screenX, wallTop, wallHeight, width, wallSide int, textureCoord, distance float64) {
	spriteBounds := sprite.Bounds()
	spriteWidth := spriteBounds.Dx()
	spriteHeight := spriteBounds.Dy()
	if spriteWidth <= 0 || spriteHeight <= 0 || width <= 0 || wallHeight <= 0 {
		return
	}

	textureX := int(textureCoord * float64(spriteWidth))
	if textureX < 0 {
		textureX = 0
	}
	if textureX >= spriteWidth {
		textureX = spriteWidth - 1
	}

	xScale := float64(width)
	yScale := float64(wallHeight) / float64(spriteHeight)

	brightness := 1.0 - (distance / r.game.camera.ViewDist)
	if brightness < r.game.config.Graphics.BrightnessMin {
		brightness = r.game.config.Graphics.BrightnessMin
	}
	if wallSide == 1 {
		brightness *= 0.7
	}

	src := sprite.SubImage(image.Rect(textureX, 0, textureX+1, spriteHeight)).(*ebiten.Image)
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Scale(xScale, yScale)
	opts.GeoM.Translate(float64(screenX), float64(wallTop))
	opts.ColorScale.Scale(float32(brightness), float32(brightness), float32(brightness), 1.0)
	screen.DrawImage(src, opts)
}

type projectileFxProfile struct {
	glowColor        [3]int
	trailColor       [3]int
	glowScale        float64
	trailLengthScale float64
	trailWidthScale  float64
	pulseSpeed       float64
	spark            bool
	sparkColor       [3]int
	// style selects the procedural pixel-particle body/trail: "ember" (fire —
	// rising flame motes), "shard" (water/ice — crisp falling crystals), or ""
	// (legacy solid-core + line trail). Set by school in spellFxProfile.
	style string
}

func mixColor(a, b [3]int, t float64) [3]int {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	out := [3]int{}
	for i := 0; i < 3; i++ {
		out[i] = int(float64(a[i])*(1-t) + float64(b[i])*t)
		if out[i] < 0 {
			out[i] = 0
		}
		if out[i] > 255 {
			out[i] = 255
		}
	}
	return out
}

func (r *Renderer) spellFxProfile(spellKey string, base [3]int) projectileFxProfile {
	profile := projectileFxProfile{
		glowColor:        mixColor(base, [3]int{255, 255, 255}, 0.35),
		trailColor:       mixColor(base, [3]int{255, 255, 255}, 0.2),
		glowScale:        1.4,
		trailLengthScale: 1.2,
		trailWidthScale:  0.35,
		pulseSpeed:       1.0,
		spark:            false,
		sparkColor:       [3]int{255, 255, 255},
	}

	if def, ok := config.GetSpellDefinition(spellKey); ok {
		switch strings.ToLower(def.School) {
		case "fire":
			profile.glowColor = [3]int{255, 140, 60}
			profile.trailColor = [3]int{255, 210, 120}
			profile.glowScale = 1.8
			profile.trailLengthScale = 1.6
			profile.trailWidthScale = 0.4
			profile.pulseSpeed = 1.4
			profile.spark = true
			profile.sparkColor = [3]int{255, 220, 160}
			profile.style = "ember"
		case "water":
			profile.style = "shard"
			profile.glowColor = [3]int{90, 170, 255}
			profile.trailColor = [3]int{150, 220, 255}
			profile.glowScale = 1.5
			profile.trailLengthScale = 1.2
			profile.trailWidthScale = 0.4
			profile.pulseSpeed = 1.0
		case "air":
			profile.glowColor = [3]int{210, 240, 255}
			profile.trailColor = [3]int{230, 255, 255}
			profile.glowScale = 1.6
			profile.trailLengthScale = 1.4
			profile.trailWidthScale = 0.3
			profile.pulseSpeed = 1.3
			profile.spark = true
			profile.sparkColor = [3]int{240, 255, 255}
		case "earth":
			profile.glowColor = [3]int{140, 200, 120}
			profile.trailColor = [3]int{190, 220, 140}
			profile.glowScale = 1.4
			profile.trailLengthScale = 1.1
			profile.trailWidthScale = 0.45
			profile.pulseSpeed = 0.9
		case "dark":
			profile.glowColor = [3]int{170, 90, 220}
			profile.trailColor = [3]int{210, 140, 255}
			profile.glowScale = 1.7
			profile.trailLengthScale = 1.3
			profile.trailWidthScale = 0.35
			profile.pulseSpeed = 1.5
			profile.spark = true
			profile.sparkColor = [3]int{210, 160, 255}
			profile.style = "dark" // sinking violet motes (not the legacy square)
		case "light":
			profile.glowColor = [3]int{255, 240, 150}
			profile.trailColor = [3]int{255, 255, 210}
			profile.glowScale = 1.7
			profile.trailLengthScale = 1.3
			profile.trailWidthScale = 0.35
			profile.pulseSpeed = 1.2
			profile.spark = true
			profile.sparkColor = [3]int{255, 255, 220}
		case "body":
			profile.glowColor = [3]int{160, 255, 180}
			profile.trailColor = [3]int{210, 255, 220}
			profile.glowScale = 1.4
			profile.trailLengthScale = 1.1
			profile.trailWidthScale = 0.4
			profile.pulseSpeed = 1.0
		case "mind":
			profile.glowColor = [3]int{180, 200, 255}
			profile.trailColor = [3]int{210, 230, 255}
			profile.glowScale = 1.5
			profile.trailLengthScale = 1.2
			profile.trailWidthScale = 0.35
			profile.pulseSpeed = 1.1
		case "spirit":
			profile.glowColor = [3]int{220, 190, 255}
			profile.trailColor = [3]int{235, 210, 255}
			profile.glowScale = 1.6
			profile.trailLengthScale = 1.2
			profile.trailWidthScale = 0.35
			profile.pulseSpeed = 1.2
			profile.spark = true
			profile.sparkColor = [3]int{240, 220, 255}
		}
	}
	return profile
}

func (r *Renderer) weaponFxProfile(weaponDef *config.WeaponDefinitionConfig) projectileFxProfile {
	base := [3]int{200, 200, 200}
	if weaponDef != nil && weaponDef.Graphics != nil {
		base = weaponDef.Graphics.Color
	}
	profile := projectileFxProfile{
		glowColor:        mixColor(base, [3]int{255, 255, 255}, 0.3),
		trailColor:       mixColor(base, [3]int{255, 255, 255}, 0.15),
		glowScale:        1.2,
		trailLengthScale: 1.3,
		trailWidthScale:  0.3,
		pulseSpeed:       0.9,
		spark:            false,
		sparkColor:       [3]int{255, 255, 255},
	}

	if weaponDef != nil {
		switch strings.ToLower(weaponDef.Category) {
		case "bow":
			profile.trailLengthScale = 1.8
			profile.trailWidthScale = 0.25
			profile.glowScale = 1.1
		case "throwing", "dagger", "knife":
			profile.trailLengthScale = 1.0
			profile.trailWidthScale = 0.4
			profile.glowScale = 1.3
			profile.spark = true
		case "staff":
			// Staves/books fling a glowing spell-style orb, not an arrow streak.
			profile.glowScale = 1.8
			profile.trailLengthScale = 1.3
			profile.trailWidthScale = 0.45
			profile.pulseSpeed = 1.3
			profile.spark = true
		}
		switch strings.ToLower(weaponDef.BonusStat) {
		case "might":
			profile.glowColor = mixColor(profile.glowColor, [3]int{255, 100, 80}, 0.35)
			profile.trailColor = mixColor(profile.trailColor, [3]int{255, 150, 120}, 0.25)
		case "accuracy":
			profile.glowColor = mixColor(profile.glowColor, [3]int{220, 240, 255}, 0.35)
			profile.trailColor = mixColor(profile.trailColor, [3]int{230, 245, 255}, 0.25)
		case "intellect":
			profile.glowColor = mixColor(profile.glowColor, [3]int{180, 140, 255}, 0.35)
			profile.trailColor = mixColor(profile.trailColor, [3]int{200, 170, 255}, 0.25)
		case "personality":
			profile.glowColor = mixColor(profile.glowColor, [3]int{255, 180, 220}, 0.35)
			profile.trailColor = mixColor(profile.trailColor, [3]int{255, 200, 230}, 0.25)
		}
		// Projectile school wins over the stat tint, giving staves/books a
		// distinct magical hue.
		switch strings.ToLower(weaponDef.ProjectileSchool) {
		case "dark":
			profile.glowColor = [3]int{170, 90, 220}
			profile.trailColor = [3]int{210, 140, 255}
			profile.sparkColor = [3]int{210, 160, 255}
			profile.spark = true
		case "arcane":
			profile.glowColor = [3]int{150, 190, 255}
			profile.trailColor = [3]int{210, 230, 255}
			profile.sparkColor = [3]int{220, 235, 255}
			profile.spark = true
			profile.style = "arcane" // pixel-particle body + trail, mirrored (R→L)
		}
	}
	return profile
}

// additiveGlowBlend is the standard additive blend used for all glow/particle
// effects (projectiles, arrows, slashes, spell hits, the impassable-tile aura):
// src·srcAlpha + dst, so overlapping glows accumulate into brighter light.
var additiveGlowBlend = ebiten.Blend{
	BlendFactorSourceRGB:        ebiten.BlendFactorSourceAlpha,
	BlendFactorSourceAlpha:      ebiten.BlendFactorSourceAlpha,
	BlendFactorDestinationRGB:   ebiten.BlendFactorOne,
	BlendFactorDestinationAlpha: ebiten.BlendFactorOne,
	BlendOperationRGB:           ebiten.BlendOperationAdd,
	BlendOperationAlpha:         ebiten.BlendOperationAdd,
}

// softGlowSize is the resolution of the radial-gradient glow texture.
const softGlowSize = 64

// ensureSoftGlow lazily builds the radial-gradient white texture (premultiplied
// alpha: opaque centre fading smoothly to transparent at the edge).
func (r *Renderer) ensureSoftGlow() *ebiten.Image {
	if r.softGlowImg != nil {
		return r.softGlowImg
	}
	const n = softGlowSize
	buf := make([]byte, 4*n*n)
	c := float64(n-1) / 2
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			dx := (float64(x) - c) / c
			dy := (float64(y) - c) / c
			d := math.Hypot(dx, dy)
			f := 1.0 - d
			if f < 0 {
				f = 0
			}
			f = f * f // soft falloff
			v := byte(f * 255)
			i := (y*n + x) * 4
			buf[i], buf[i+1], buf[i+2], buf[i+3] = v, v, v, v // premultiplied white
		}
	}
	img := ebiten.NewImage(n, n)
	img.WritePixels(buf)
	r.softGlowImg = img
	return img
}

// drawGlowSprite draws a soft ROUND glow of diameter `size` centred at (x,y),
// tinted by rgb at the given alpha. Same additive convention as drawGlowRect but
// with the radial-gradient texture so it isn't a hard square.
func (r *Renderer) drawGlowSprite(screen *ebiten.Image, x, y, size float64, rgb [3]int, alpha float64, blend ebiten.Blend) {
	if size <= 0 || alpha <= 0 {
		return
	}
	src := r.ensureSoftGlow()
	s := size / float64(softGlowSize)
	opts := &r.glowOpts
	opts.GeoM.Reset()
	opts.GeoM.Scale(s, s)
	opts.GeoM.Translate(x-size/2, y-size/2)
	opts.ColorScale.Reset()
	opts.ColorScale.Scale(
		float32(rgb[0])/255,
		float32(rgb[1])/255,
		float32(rgb[2])/255,
		float32(alpha),
	)
	opts.Blend = blend
	screen.DrawImage(src, opts)
}

func (r *Renderer) drawGlowRect(screen *ebiten.Image, x, y, size float64, rgb [3]int, alpha float64, blend ebiten.Blend) {
	if size <= 0 || alpha <= 0 {
		return
	}
	opts := &r.glowOpts // reused; reset the fields we set below
	opts.GeoM.Reset()
	opts.GeoM.Scale(size, size)
	opts.GeoM.Translate(x-size/2, y-size/2)
	opts.ColorScale.Reset()
	opts.ColorScale.Scale(
		float32(rgb[0])/255,
		float32(rgb[1])/255,
		float32(rgb[2])/255,
		float32(alpha),
	)
	opts.Blend = blend
	screen.DrawImage(r.whiteImg, opts)
}

func (r *Renderer) projectileScreenDir(vx, vy float64) (float64, float64, bool) {
	if vx == 0 && vy == 0 {
		return 0, 0, false
	}
	camRightX := -math.Sin(r.game.camera.Angle)
	camRightY := math.Cos(r.game.camera.Angle)
	right := vx*camRightX + vy*camRightY
	if math.Abs(right) < 0.01 {
		return 0, 0, false
	}
	dirX := math.Copysign(1, right)
	return dirX, 0, true
}

func (r *Renderer) shouldAnimateMonster(mon *monster.Monster3D) bool {
	switch mon.State {
	case monster.StatePatrolling, monster.StatePursuing, monster.StateFleeing:
		return true
	default:
		return false
	}
}

func (r *Renderer) getMonsterSprite(mon *monster.Monster3D) (*ebiten.Image, bool) {
	spriteName := mon.GetSpriteType()
	anim, flip := r.getMonsterWalkAnimation(spriteName, mon)
	if anim != nil && len(anim.Frames) > 0 {
		tps := r.game.config.GetTPS()
		if tps <= 0 {
			tps = 60
		}
		const animFPS = 8
		ticksPerFrame := tps / animFPS
		if ticksPerFrame < 1 {
			ticksPerFrame = 1
		}
		animWindow := int64(ticksPerFrame * len(anim.Frames))
		if animWindow < 1 {
			animWindow = 1
		}
		if r.shouldAnimateMonster(mon) {
			frame := int((r.game.frameCount / int64(ticksPerFrame)) % int64(len(anim.Frames)))
			return anim.Frames[frame], flip
		}
		if r.game.turnBasedMode && mon.LastMoveTick > 0 {
			if r.game.frameCount-mon.LastMoveTick <= animWindow {
				frame := int((r.game.frameCount / int64(ticksPerFrame)) % int64(len(anim.Frames)))
				return anim.Frames[frame], flip
			}
		}
		return anim.Frames[0], flip
	}
	return r.game.sprites.GetSprite(spriteName), false
}

func (r *Renderer) monsterScreenDir(mon *monster.Monster3D) (int, bool) {
	moveX := math.Cos(mon.Direction)
	moveY := math.Sin(mon.Direction)
	if moveX == 0 && moveY == 0 {
		return 0, false
	}
	camRightX := -math.Sin(r.game.camera.Angle)
	camRightY := math.Cos(r.game.camera.Angle)
	right := moveX*camRightX + moveY*camRightY
	if math.Abs(right) < 0.01 {
		return 0, false
	}
	if right > 0 {
		return 1, true
	}
	return -1, true
}

func (r *Renderer) getMonsterWalkAnimation(spriteName string, mon *monster.Monster3D) (*graphics.SpriteAnimation, bool) {
	if dir, ok := r.monsterScreenDir(mon); ok {
		if dir > 0 {
			if anim := r.game.sprites.GetAnimation(spriteName, "walking_r"); anim != nil && len(anim.Frames) > 0 {
				return anim, false
			}
			if anim := r.game.sprites.GetAnimation(spriteName, "walking_l"); anim != nil && len(anim.Frames) > 0 {
				return anim, true
			}
		} else {
			if anim := r.game.sprites.GetAnimation(spriteName, "walking_l"); anim != nil && len(anim.Frames) > 0 {
				return anim, false
			}
			if anim := r.game.sprites.GetAnimation(spriteName, "walking_r"); anim != nil && len(anim.Frames) > 0 {
				return anim, true
			}
		}
	}
	// No clear left/right direction: fall back to any available directional animation.
	if anim := r.game.sprites.GetAnimation(spriteName, "walking_r"); anim != nil && len(anim.Frames) > 0 {
		return anim, false
	}
	if anim := r.game.sprites.GetAnimation(spriteName, "walking_l"); anim != nil && len(anim.Frames) > 0 {
		return anim, false
	}
	return nil, false
}

// SpriteType identifies the type of sprite for unified rendering
type SpriteType int

const (
	SpriteTypeEnvironment SpriteType = iota
	SpriteTypeTree
	SpriteTypeMonster
	SpriteTypeNPC
	SpriteTypeGroundContainer
)

// UnifiedSpriteRenderData holds data for rendering any sprite type in a unified sorted pass
type UnifiedSpriteRenderData struct {
	spriteType SpriteType
	screenX    int
	screenY    int
	spriteSize int
	depthPerp  float64 // Camera-space perpendicular depth (for z-buffer comparison)
	distance   float64
	sprite     *ebiten.Image
	// Environment/Tree specific
	tileX    int
	tileY    int
	tileType world.TileType3D
	// Monster specific
	monster     *monster.Monster3D
	monsterFlip bool
	// NPC specific
	npc *character.NPC
	// Ground container (loot bag / treasure chest) specific
	groundContainer *GroundContainer
}

// drawAllSpritesSorted collects all visible sprites (trees, ferns, monsters, NPCs)
// and renders them sorted by depth for proper transparency and occlusion.
func (r *Renderer) drawAllSpritesSorted(screen *ebiten.Image) {
	// Reuse pre-allocated buffer
	sprites := r.unifiedSprites[:0]

	// Camera properties for frustum culling
	camX := r.game.camera.X
	camY := r.game.camera.Y
	camAngle := r.game.camera.Angle
	fov := r.game.camera.FOV
	viewDistSq := r.game.camera.ViewDist * r.game.camera.ViewDist

	// Precompute camera direction for camera-space depth calculations
	camDirX := math.Cos(camAngle)
	camDirY := math.Sin(camAngle)

	// Precompute frustum culling values
	halfFOV := fov / 2
	fovMargin := halfFOV + 0.1

	// Get player's current tile for sprite culling
	playerTileX, playerTileY := r.game.GetPlayerTilePosition()
	tileSize := float64(r.game.config.GetTileSize())
	minDistSq := tileSize * tileSize

	// 1. Collect transparent environment sprites (ferns, mushrooms)
	if world.GlobalTileManager != nil {
		for i := range r.transparentSpritesCache {
			spriteData := &r.transparentSpritesCache[i]

			if spriteData.tileX == playerTileX && spriteData.tileY == playerTileY {
				continue
			}

			dx := spriteData.worldX - camX
			dy := spriteData.worldY - camY
			distanceSq := dx*dx + dy*dy

			if distanceSq < minDistSq || distanceSq > viewDistSq {
				continue
			}

			depthPerp := dx*camDirX + dy*camDirY
			if depthPerp <= 0 {
				continue
			}

			entityAngle := math.Atan2(dy, dx)
			angleDiff := entityAngle - camAngle
			for angleDiff > math.Pi {
				angleDiff -= 2 * math.Pi
			}
			for angleDiff < -math.Pi {
				angleDiff += 2 * math.Pi
			}
			if math.Abs(angleDiff) > fovMargin {
				continue
			}

			distance := math.Sqrt(distanceSq)
			screenX, screenY, spriteSize, visible := r.game.renderHelper.CalculateEnvironmentSpriteMetrics(spriteData.worldX, spriteData.worldY, distance, spriteData.tileType)
			if !visible {
				continue
			}

			sprite := r.getProcessedSpriteByName(spriteData.tileType, spriteData.spriteName)
			if sprite == nil {
				continue
			}

			sprites = append(sprites, UnifiedSpriteRenderData{
				spriteType: SpriteTypeEnvironment,
				screenX:    screenX,
				screenY:    screenY,
				spriteSize: spriteSize,
				depthPerp:  depthPerp,
				sprite:     sprite,
				tileX:      spriteData.tileX,
				tileY:      spriteData.tileY,
				tileType:   spriteData.tileType,
			})
		}
	}

	// 2. Add tree hits collected during raycasting
	for _, tree := range r.treeHits {
		sprites = append(sprites, UnifiedSpriteRenderData{
			spriteType: SpriteTypeTree,
			screenX:    tree.screenX,
			depthPerp:  tree.distance,
			tileType:   tree.tileType,
		})
	}
	r.treeHits = r.treeHits[:0]

	// 3. Collect monsters
	for _, mon := range r.game.GetCurrentWorld().Monsters {
		if !mon.IsAlive() {
			continue
		}

		dx := mon.X - camX
		dy := mon.Y - camY
		distanceSq := dx*dx + dy*dy

		if distanceSq > viewDistSq {
			continue
		}

		depthPerp := dx*camDirX + dy*camDirY
		if depthPerp <= 0 {
			continue
		}

		distance := math.Sqrt(distanceSq)
		sizeMultiplier := mon.GetSizeGameMultiplier()
		screenX, screenY, spriteSize, visible := r.game.renderHelper.CalculateMonsterSpriteMetrics(mon.X, mon.Y, distance, sizeMultiplier)
		if visible && mon.Flying {
			screenY = r.game.config.GetScreenHeight()/2 - spriteSize/2
		}
		if !visible {
			continue
		}

		sprite, flip := r.getMonsterSprite(mon)

		sprites = append(sprites, UnifiedSpriteRenderData{
			spriteType:  SpriteTypeMonster,
			screenX:     screenX,
			screenY:     screenY,
			spriteSize:  spriteSize,
			depthPerp:   depthPerp,
			sprite:      sprite,
			monster:     mon,
			monsterFlip: flip,
		})
	}

	// 4. Collect NPCs
	for _, npc := range r.game.GetCurrentWorld().NPCs {
		// Spriteless NPCs (e.g. invisible portal gates) render nothing — they
		// exist only as an interaction anchor; their tile shows through instead.
		if npc.Sprite == "" || npc.Sprite == "none" {
			continue
		}
		dx := npc.X - camX
		dy := npc.Y - camY
		distanceSq := dx*dx + dy*dy

		// Same cull thresholds as environment sprites so NPCs (including the
		// shipwreck / church / exit) disappear cleanly when the player walks
		// into them, instead of glitching as their floor anchor slides below
		// the screen at very small perpDist.
		if distanceSq < minDistSq || distanceSq > viewDistSq {
			continue
		}

		depthPerp := dx*camDirX + dy*camDirY
		if depthPerp <= 0 {
			continue
		}

		distance := math.Sqrt(distanceSq)

		var screenX, screenY, spriteSize int
		var visible bool

		if npc.RenderType == "environment_sprite" {
			screenX, screenY, spriteSize, visible = r.game.renderHelper.CalculateEnvironmentSpriteMetrics(npc.X, npc.Y, distance, world.TileEmpty)
		} else {
			screenX, screenY, spriteSize, visible = r.game.renderHelper.CalculateNPCSpriteMetrics(npc.X, npc.Y, distance, npc.SizeMultiplier)
		}
		if !visible {
			continue
		}

		spriteName := "elf"
		if npc.Sprite != "" {
			spriteName = strings.TrimSuffix(npc.Sprite, ".png")
		}
		sprite := r.game.sprites.GetSprite(spriteName)

		sprites = append(sprites, UnifiedSpriteRenderData{
			spriteType: SpriteTypeNPC,
			screenX:    screenX,
			screenY:    screenY,
			spriteSize: spriteSize,
			depthPerp:  depthPerp,
			sprite:     sprite,
			npc:        npc,
		})
	}

	// 5. Collect ground containers (loot bags + treasure chests)
	activeMapKey := currentMapKey()
	for i := range r.game.groundContainers {
		c := &r.game.groundContainers[i]
		if c.MapKey != "" && c.MapKey != activeMapKey {
			continue
		}
		dx := c.X - camX
		dy := c.Y - camY
		distanceSq := dx*dx + dy*dy
		// Same one-tile cull as env sprites / NPCs so a container at the
		// player's feet disappears cleanly instead of sliding under the camera.
		if distanceSq < minDistSq || distanceSq > viewDistSq {
			continue
		}
		depthPerp := dx*camDirX + dy*camDirY
		if depthPerp <= 0 {
			continue
		}
		distance := math.Sqrt(distanceSq)
		info := r.game.groundContainerRenderInfo(c, distance)
		if !info.Visible {
			continue
		}
		sprite := r.game.sprites.GetSprite(c.effectiveSprite())
		sprites = append(sprites, UnifiedSpriteRenderData{
			spriteType:      SpriteTypeGroundContainer,
			screenX:         info.ScreenX,
			screenY:         info.ScreenY,
			spriteSize:      info.SpriteSize,
			depthPerp:       depthPerp,
			distance:        info.Distance,
			sprite:          sprite,
			groundContainer: c,
		})
	}

	// Sort all sprites by depth (back to front)
	sort.Slice(sprites, func(i, j int) bool {
		return sprites[i].depthPerp > sprites[j].depthPerp
	})

	// Update buffer for next frame
	r.unifiedSprites = sprites

	// Render all sprites in sorted order
	for _, s := range sprites {
		switch s.spriteType {
		case SpriteTypeEnvironment:
			r.drawUnifiedEnvironmentSprite(screen, s)
		case SpriteTypeTree:
			r.drawTreeSprite(screen, s.screenX, s.depthPerp, s.tileType)
		case SpriteTypeMonster:
			r.drawUnifiedMonsterSprite(screen, s)
		case SpriteTypeNPC:
			r.drawUnifiedNPCSprite(screen, s)
		case SpriteTypeGroundContainer:
			r.drawUnifiedGroundContainerSprite(screen, s)
		}
	}
}

// spriteDepthBufferVisible returns true if the sprite's screen-X span has at
// least one pixel where the sprite is in front of the wall depth buffer.
// Shared by all the floor-anchored sprite drawers (env / loot bag / chest).
func (r *Renderer) spriteDepthBufferVisible(s UnifiedSpriteRenderData) bool {
	left := s.screenX - s.spriteSize/2
	right := s.screenX + s.spriteSize/2
	if left < 0 {
		left = 0
	}
	if right >= len(r.game.depthBuffer) {
		right = len(r.game.depthBuffer) - 1
	}
	for x := left; x <= right; x++ {
		if s.depthPerp < r.game.depthBuffer[x] {
			return true
		}
	}
	return false
}

// drawTintedSprite draws a sprite scaled to spriteSize at (drawLeft, screenY)
// with the given RGBA tint applied via ColorScale. Used for both the
// brightness pass and the hover-highlight overlay.
func (r *Renderer) drawTintedSprite(screen *ebiten.Image, sprite *ebiten.Image, drawLeft, screenY, spriteSize int, tintR, tintG, tintB, tintA float32) {
	if sprite == nil {
		return
	}
	scaleX := float64(spriteSize) / float64(sprite.Bounds().Dx())
	scaleY := float64(spriteSize) / float64(sprite.Bounds().Dy())
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Scale(scaleX, scaleY)
	opts.GeoM.Translate(float64(drawLeft), float64(screenY))
	opts.ColorScale.Scale(tintR, tintG, tintB, tintA)
	opts.Blend = ebiten.BlendSourceOver
	screen.DrawImage(sprite, opts)
}

// hoverHighlightTint is the soft yellow overlay drawn on pickup-range
// sprites (ground containers) when the cursor is over them.
var hoverHighlightTint = [4]float32{1.0, 0.95, 0.6, 0.6}

// drawUnifiedGroundContainerSprite draws a ground container (loot bag or
// treasure chest) from unified data with brightness and optional hover.
func (r *Renderer) drawUnifiedGroundContainerSprite(screen *ebiten.Image, s UnifiedSpriteRenderData) {
	if !r.spriteDepthBufferVisible(s) || s.groundContainer == nil {
		return
	}

	pickupRange := r.game.groundContainerPickupRange()
	hovered := false
	if s.distance <= pickupRange {
		mouseX, mouseY := ebiten.CursorPosition()
		info := GroundContainerRenderInfo{
			ScreenX:    s.screenX,
			ScreenY:    s.screenY,
			SpriteSize: s.spriteSize,
			Distance:   s.distance,
			Visible:    true,
		}
		hovered = r.game.groundContainerHitTestFromInfo(info, s.groundContainer.effectiveSprite(), mouseX, mouseY, pickupRange)
	}

	drawLeft := s.screenX - s.spriteSize/2
	brightness := r.calculateBrightnessWithTorchLight(s.groundContainer.X, s.groundContainer.Y, s.distance)
	b := float32(brightness)
	r.drawTintedSprite(screen, s.sprite, drawLeft, s.screenY, s.spriteSize, b, b, b, 1.0)

	if hovered {
		r.drawTintedSprite(screen, s.sprite, drawLeft, s.screenY, s.spriteSize,
			hoverHighlightTint[0], hoverHighlightTint[1], hoverHighlightTint[2], hoverHighlightTint[3])
	}
}

// drawUnifiedEnvironmentSprite draws an environment sprite from unified data
func (r *Renderer) drawUnifiedEnvironmentSprite(screen *ebiten.Image, s UnifiedSpriteRenderData) {
	if !r.spriteDepthBufferVisible(s) {
		return
	}

	tileSize := float64(r.game.config.GetTileSize())
	worldX, worldY := TileCenterFromTile(s.tileX, s.tileY, tileSize)
	distance := math.Sqrt(math.Pow(worldX-r.game.camera.X, 2) + math.Pow(worldY-r.game.camera.Y, 2))
	brightness := r.calculateBrightnessWithTorchLight(worldX, worldY, distance)
	b := float32(brightness)

	drawLeft := s.screenX - s.spriteSize/2
	r.drawTintedSprite(screen, s.sprite, drawLeft, s.screenY, s.spriteSize, b, b, b, 1.0)
}

// drawUnifiedMonsterSprite draws a monster sprite from unified data
func (r *Renderer) drawUnifiedMonsterSprite(screen *ebiten.Image, s UnifiedSpriteRenderData) {
	if !r.spriteDepthBufferVisible(s) {
		return
	}

	drawLeft := s.screenX - s.spriteSize/2
	// Keep mobs above the party HUD bar: a big sprite at point-blank range would
	// otherwise sink its lower body behind the bar. If its feet would cross the
	// bar's top edge, raise the whole sprite so its bottom rests on the bar.
	screenY := s.screenY
	if r.game.showPartyStats {
		barTop := r.game.config.GetScreenHeight() - r.game.config.UI.PartyPortraitHeight
		if screenY+s.spriteSize > barTop {
			screenY = barTop - s.spriteSize
		}
	}

	opts := &ebiten.DrawImageOptions{}
	scaleX := float64(s.spriteSize) / float64(s.sprite.Bounds().Dx())
	scaleY := float64(s.spriteSize) / float64(s.sprite.Bounds().Dy())

	if s.monsterFlip {
		opts.GeoM.Scale(-scaleX, scaleY)
		opts.GeoM.Translate(float64(drawLeft+s.spriteSize), float64(screenY))
	} else {
		opts.GeoM.Scale(scaleX, scaleY)
		opts.GeoM.Translate(float64(drawLeft), float64(screenY))
	}

	distance := math.Sqrt(math.Pow(s.monster.X-r.game.camera.X, 2) + math.Pow(s.monster.Y-r.game.camera.Y, 2))
	brightness := r.calculateBrightnessWithTorchLight(s.monster.X, s.monster.Y, distance)
	opts.ColorScale.Scale(float32(brightness), float32(brightness), float32(brightness), 1.0)
	opts.Blend = ebiten.BlendSourceOver

	screen.DrawImage(s.sprite, opts)
}

// drawUnifiedNPCSprite draws an NPC sprite from unified data
func (r *Renderer) drawUnifiedNPCSprite(screen *ebiten.Image, s UnifiedSpriteRenderData) {
	if !r.spriteDepthBufferVisible(s) {
		return
	}

	drawLeft := s.screenX - s.spriteSize/2
	sprite, frameW, frameH := selectAnimatedSpriteFrame(s.sprite, r.game.frameCount)

	opts := &ebiten.DrawImageOptions{}
	scaleX := float64(s.spriteSize) / float64(frameW)
	scaleY := float64(s.spriteSize) / float64(frameH)
	opts.GeoM.Scale(scaleX, scaleY)
	opts.GeoM.Translate(float64(drawLeft), float64(s.screenY))

	distance := math.Sqrt(math.Pow(s.npc.X-r.game.camera.X, 2) + math.Pow(s.npc.Y-r.game.camera.Y, 2))
	brightness := r.calculateBrightnessWithTorchLight(s.npc.X, s.npc.Y, distance)
	if brightness < r.game.config.Graphics.BrightnessMin {
		brightness = r.game.config.Graphics.BrightnessMin
	}
	opts.ColorScale.Scale(float32(brightness), float32(brightness), float32(brightness), 1.0)
	opts.Blend = ebiten.BlendSourceOver

	screen.DrawImage(sprite, opts)
}

// selectAnimatedSpriteFrame picks an animation frame from a horizontal sprite
// sheet. If the sprite's width equals frameHeight × SpriteSheetFrameCount, the
// sheet is treated as animated and the frame is selected by frameCount; the
// returned image is a SubImage and the returned width/height are the per-frame
// dimensions. Otherwise the sprite is returned unchanged.
func selectAnimatedSpriteFrame(sprite *ebiten.Image, frameCount int64) (*ebiten.Image, int, int) {
	bounds := sprite.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if h <= 0 || w != h*SpriteSheetFrameCount {
		return sprite, w, h
	}
	frame := int((frameCount / SpriteFrameStride) % SpriteSheetFrameCount)
	rect := image.Rect(
		bounds.Min.X+frame*h, bounds.Min.Y,
		bounds.Min.X+(frame+1)*h, bounds.Min.Y+h,
	)
	return sprite.SubImage(rect).(*ebiten.Image), h, h
}

// drawProjectiles draws magic projectiles, sword attacks, and arrows
func (r *Renderer) drawProjectiles(screen *ebiten.Image) {
	r.drawMagicProjectiles(screen)
	r.drawMeleeAttacks(screen)
	r.drawArrows(screen)
}

// projectileProjection bundles the camera-space projection of a point-like
// moving entity (magic projectile, melee swing, arrow). Returned by
// projectMovingEntity when the entity passes range / FOV / depth-buffer culls.
type projectileProjection struct {
	screenX int
	screenY int
	size    int
}

// projectMovingEntity culls and projects a point-like entity at world (x, y)
// against the camera frustum and depth buffer. baseSize/minSize/maxSize come
// from the entity's graphics config (spell or weapon). Returns ok=false if the
// entity should be skipped this frame.
func (r *Renderer) projectMovingEntity(x, y float64, baseSize, minSize, maxSize int) (projectileProjection, bool) {
	cam := r.game.camera
	dx := x - cam.X
	dy := y - cam.Y
	distSq := dx*dx + dy*dy
	viewDistSq := cam.ViewDist * cam.ViewDist
	if distSq > viewDistSq || distSq < 100 { // 10 unit near clip, squared
		return projectileProjection{}, false
	}

	angleDiff := math.Atan2(dy, dx) - cam.Angle
	for angleDiff > math.Pi {
		angleDiff -= 2 * math.Pi
	}
	for angleDiff < -math.Pi {
		angleDiff += 2 * math.Pi
	}
	halfFOV := cam.FOV / 2
	if math.Abs(angleDiff) > halfFOV {
		return projectileProjection{}, false
	}

	halfW := float64(r.game.config.GetScreenWidth()) / 2
	screenX := int(halfW + (angleDiff/halfFOV)*halfW)

	depthPerp := dx*math.Cos(cam.Angle) + dy*math.Sin(cam.Angle)
	if screenX >= 0 && screenX < len(r.game.depthBuffer) {
		if depthPerp >= r.game.depthBuffer[screenX] {
			return projectileProjection{}, false
		}
	}

	dist := math.Sqrt(distSq)
	size := int(float64(baseSize) / dist * float64(r.game.config.GetTileSize()))
	if size > maxSize {
		size = maxSize
	}
	if size < minSize {
		size = minSize
	}

	return projectileProjection{
		screenX: screenX,
		screenY: r.game.config.GetScreenHeight()/2 - size/2,
		size:    size,
	}, true
}

// Spell-hit particle sizing. `scale` (= screenHeight/(relY·fov)) is the same
// perspective factor used for screen position, so size falls off linearly with
// distance (true perspective). spellParticleSizeFactor < 1 keeps the max cap a
// genuine point-blank-only ceiling: a fresh particle only hits it within ~1 tile,
// so across normal combat range the size visibly shrinks with distance instead
// of pinning to the cap everywhere.
const (
	spellParticleSizeFactor = 0.55
	spellParticleMinSize    = 0.75
	spellParticleMaxSize    = 40.0
)

// spellParticleScreenSize projects a spell-hit particle to its on-screen size:
// bigger up close, shrinking with distance (perspective via `scale`), faded by
// remaining life, clamped to [min, max]. Pure so it can be unit-tested.
func spellParticleScreenSize(particleSize int, lifeRatio, scale float64) float64 {
	size := float64(particleSize) * (0.18 + 0.82*lifeRatio) * scale * spellParticleSizeFactor
	if size < spellParticleMinSize {
		size = spellParticleMinSize
	}
	if size > spellParticleMaxSize {
		size = spellParticleMaxSize
	}
	return size
}

// drawMagicProjectiles draws all active magic projectiles
func (r *Renderer) drawMagicProjectiles(screen *ebiten.Image) {
	glowBlend := additiveGlowBlend

	for idx, magicProjectile := range r.game.magicProjectiles {
		if !magicProjectile.Active {
			continue
		}

		// The SpellType string is actually the SpellID (e.g., "firebolt", "fireball").
		spellConfigName := magicProjectile.SpellType
		spellGraphicsConfig, err := r.game.config.GetSpellGraphicsConfig(spellConfigName)
		if err != nil {
			continue // Skip rendering if no graphics config
		}

		proj, ok := r.projectMovingEntity(magicProjectile.X, magicProjectile.Y,
			spellGraphicsConfig.BaseSize, spellGraphicsConfig.MinSize, spellGraphicsConfig.MaxSize)
		if !ok {
			continue
		}
		screenX := proj.screenX
		screenY := proj.screenY
		projectileSize := proj.size

		// Draw collision box if enabled (draw first, so it's behind the projectile)
		if r.game.showCollisionBoxes {
			// Get world-space collision box dimensions
			var worldColW, worldColH float64
			if r.game.collisionSystem != nil {
				// Use unique ID for direct lookup
				if entity := r.game.collisionSystem.GetEntityByID(magicProjectile.ID); entity != nil && entity.BoundingBox != nil {
					worldColW = entity.BoundingBox.Width
					worldColH = entity.BoundingBox.Height
				} else {
					worldColW, worldColH = float64(spellGraphicsConfig.BaseSize), float64(spellGraphicsConfig.BaseSize)
				}
			} else {
				worldColW, worldColH = float64(spellGraphicsConfig.BaseSize), float64(spellGraphicsConfig.BaseSize)
			}

			// Apply the same distance-based scaling as the projectile visual
			scaleFactor := float64(projectileSize) / float64(spellGraphicsConfig.BaseSize)
			screenColW := int(worldColW * scaleFactor)
			screenColH := int(worldColH * scaleFactor)

			// Only draw collision box if we have valid dimensions
			if screenColW > 0 && screenColH > 0 {
				boxX := screenX - screenColW/2
				boxY := screenY + (projectileSize-screenColH)/2
				boxColor := color.RGBA{0, 255, 0, 120} // Green, semi-transparent
				boxOpts := &ebiten.DrawImageOptions{}
				boxOpts.GeoM.Scale(float64(screenColW), float64(screenColH))
				boxOpts.GeoM.Translate(float64(boxX), float64(boxY))
				boxOpts.ColorScale.Scale(
					float32(boxColor.R)/255,
					float32(boxColor.G)/255,
					float32(boxColor.B)/255,
					float32(boxColor.A)/255*0.5,
				)
				screen.DrawImage(r.whiteImg, boxOpts)
			}
		}

		// Use spell-specific color from config (no more hardcoded colors!)
		projectileColor := spellGraphicsConfig.Color

		centerX := float64(screenX)
		centerY := float64(screenY) + float64(projectileSize)/2
		fxProfile := r.spellFxProfile(spellConfigName, projectileColor)
		pulse := 0.85 + 0.15*math.Sin(float64(r.game.frameCount)*fxProfile.pulseSpeed*0.15)
		critBoost := 1.0
		if magicProjectile.Crit {
			critBoost = 1.2
		}
		// Soft ambient glow under the projectile (all styles).
		glowSize := float64(projectileSize) * fxProfile.glowScale * pulse * critBoost
		r.drawGlowSprite(screen, centerX, centerY, glowSize, fxProfile.glowColor, 0.6*critBoost, glowBlend)

		dirX, _, hasDir := r.projectileScreenDir(magicProjectile.VelX, magicProjectile.VelY)
		if !hasDir {
			dirX = 1 // default trail direction when motion is head-on
		}

		// Spells are always magical → particle body + evaporating trail (never the
		// old solid square). Drift/mirror come from the school's style; colour comes
		// from the projectile colour, so every school looks distinct.
		r.drawSpellProjectileFx(screen, centerX, centerY, float64(projectileSize), dirX,
			projectileColor, fxProfile, critBoost, idx)
	}
}

// drawSpellProjectileFx renders a flying spell as a cluster of pixel quads with
// an evaporating trail, instead of a single solid square. "ember" (fire) motes
// flicker hot and rise as they trail; "shard" (ice) bits stay crisp and sink.
// Density/length scale with `size`, so a fireball reads far bigger than a bolt.
func (r *Renderer) drawSpellProjectileFx(screen *ebiten.Image, cx, cy, size, dirX float64, core [3]int, p projectileFxProfile, critBoost float64, id int) {
	// sink = heavy/cold/void motes fall; others rise like embers/wisps.
	sink := p.style == "shard" || p.style == "dark"
	mirror := p.style == "arcane" // staff/book bolt: trail sweeps the other way (R→L)
	// Hot core = the spell's own colour lightened, so every school reads distinct
	// (fire → light orange, dark → light violet, ice → light blue, …).
	hot := mixColor(core, [3]int{255, 255, 255}, 0.6)
	// Mirror the trail's horizontal direction for arcane bolts.
	if mirror {
		dirX = -dirX
	}
	fc := float64(r.game.frameCount)

	// Evaporating trail: quads behind the head (opposite screen-motion), drifting
	// up (embers) or down (shards) and fading toward the tail. Wide perpendicular
	// scatter so it reads as smoke/sparks, not a straight line.
	trailLen := size * p.trailLengthScale * 4.0 * critBoost
	nTrail := int(size*1.4) + 8
	if nTrail > 70 {
		nTrail = 70
	}
	for k := 0; k < nTrail; k++ {
		j1 := auraHash(id, k, 1, int(fc)/3)
		j2 := auraHash(id, k, 2, int(fc)/3)
		t := (float64(k) + j1) / float64(nTrail) // 0 head → 1 tail
		back := t * trailLen
		drift := t * size * 1.1
		// scatter widens along the tail (cone), keyed to the seed
		spread := size * (0.25 + 0.8*t)
		px := cx - dirX*back + (j2-0.5)*spread
		py := cy + (j1-0.5)*spread*0.8
		if sink {
			py += drift // ice shards / dark motes sink
		} else {
			py -= drift // embers / wisps rise
		}
		qs := size*0.22*(1.0-t) + 1.5
		alpha := (1.0 - t) * 0.5 * (0.6 + 0.4*j2)
		if alpha <= 0.02 {
			continue
		}
		r.drawGlowSprite(screen, px, py, qs, mixColor(core, p.trailColor, t), alpha, additiveGlowBlend)
	}

	// Body: a fluffy flickering cluster at the head — many small motes spread
	// wide (radius ~0.7×size), hot/white core fading to the spell colour at the
	// edges. Smaller per-mote size keeps a big fireball round, not a blocky square.
	nBody := int(size*1.0) + 6
	if nBody > 60 {
		nBody = 60
	}
	bodyR := size * 0.7
	for k := 0; k < nBody; k++ {
		a := auraHash(id, k, 3, int(fc)/2) * 2 * math.Pi
		// sqrt distribution → denser core, soft round falloff
		rad := math.Sqrt(auraHash(id, k, 4, int(fc)/2)) * bodyR
		px := cx + math.Cos(a)*rad
		py := cy + math.Sin(a)*rad*0.9
		edge := rad / (bodyR + 1) // 0 center → 1 edge
		qs := size*(0.28-0.13*edge) + 1.5
		col := mixColor(hot, core, edge)
		flick := 0.65 + 0.35*auraHash(id, k, 5, int(fc))
		r.drawGlowSprite(screen, px, py, qs, col, (0.85-0.4*edge)*flick*critBoost, additiveGlowBlend)
	}
}

// drawMeleeAttacks draws all active melee attacks
func (r *Renderer) drawMeleeAttacks(screen *ebiten.Image) {
	for _, attack := range r.game.meleeAttacks {
		if !attack.Active {
			continue
		}

		weaponDef := lookupWeaponConfigByName(attack.WeaponName)
		if weaponDef == nil || weaponDef.Graphics == nil {
			continue // Skip rendering if weapon config missing
		}

		proj, ok := r.projectMovingEntity(attack.X, attack.Y,
			weaponDef.Graphics.BaseSize, weaponDef.Graphics.MinSize, weaponDef.Graphics.MaxSize)
		if !ok {
			continue
		}
		screenX := proj.screenX
		screenY := proj.screenY
		attackSize := proj.size

		// Draw collision box if enabled (draw first, so it's behind the attack)
		if r.game.showCollisionBoxes {
			// Get world-space collision box dimensions
			var worldColW, worldColH float64
			if r.game.collisionSystem != nil {
				// Use unique ID for direct lookup
				if entity := r.game.collisionSystem.GetEntityByID(attack.ID); entity != nil && entity.BoundingBox != nil {
					worldColW = entity.BoundingBox.Width
					worldColH = entity.BoundingBox.Height
				} else {
					worldColW, worldColH = float64(weaponDef.Graphics.BaseSize), float64(weaponDef.Graphics.BaseSize)
				}
			} else {
				worldColW, worldColH = float64(weaponDef.Graphics.BaseSize), float64(weaponDef.Graphics.BaseSize)
			}
			// Apply the same distance-based scaling as the attack visual
			scaleFactor := float64(attackSize) / float64(weaponDef.Graphics.BaseSize)
			screenColW := int(worldColW * scaleFactor)
			screenColH := int(worldColH * scaleFactor)
			// Only draw collision box if we have valid dimensions
			if screenColW > 0 && screenColH > 0 {
				boxX := screenX - screenColW/2
				boxY := screenY + (attackSize-screenColH)/2
				boxColor := color.RGBA{255, 255, 0, 120} // Yellow, semi-transparent
				boxOpts := &ebiten.DrawImageOptions{}
				boxOpts.GeoM.Scale(float64(screenColW), float64(screenColH))
				boxOpts.GeoM.Translate(float64(boxX), float64(boxY))
				boxOpts.ColorScale.Scale(
					float32(boxColor.R)/255,
					float32(boxColor.G)/255,
					float32(boxColor.B)/255,
					float32(boxColor.A)/255*0.5,
				)
				screen.DrawImage(r.whiteImg, boxOpts)
			}
		}

		// Draw attack using weapon-specific color from config
		attackColor := weaponDef.Graphics.Color

		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Scale(float64(attackSize), float64(attackSize))
		opts.GeoM.Translate(float64(screenX-attackSize/2), float64(screenY))
		opts.ColorScale.Scale(
			float32(attackColor[0])/255,
			float32(attackColor[1])/255,
			float32(attackColor[2])/255,
			1,
		)
		screen.DrawImage(r.whiteImg, opts)
	}
}

// drawArrows draws all active arrows
func (r *Renderer) drawArrows(screen *ebiten.Image) {
	glowBlend := additiveGlowBlend

	for idx, arrow := range r.game.arrows {
		if !arrow.Active {
			continue
		}

		bowDef := lookupWeaponConfigByKey(arrow.BowKey)
		if bowDef == nil || bowDef.Graphics == nil {
			continue // Skip rendering if weapon config missing
		}

		proj, ok := r.projectMovingEntity(arrow.X, arrow.Y,
			bowDef.Graphics.BaseSize, bowDef.Graphics.MinSize, bowDef.Graphics.MaxSize)
		if !ok {
			continue
		}
		screenX := proj.screenX
		screenY := proj.screenY
		arrowSize := proj.size

		// Draw collision box if enabled (draw first, so it's behind the arrow)
		if r.game.showCollisionBoxes {
			// Get world-space collision box dimensions
			var worldColW, worldColH float64
			if r.game.collisionSystem != nil {
				// Use unique ID for direct lookup
				if entity := r.game.collisionSystem.GetEntityByID(arrow.ID); entity != nil && entity.BoundingBox != nil {
					worldColW = entity.BoundingBox.Width
					worldColH = entity.BoundingBox.Height
				} else {
					worldColW, worldColH = float64(bowDef.Graphics.BaseSize), float64(bowDef.Graphics.BaseSize)
				}
			} else {
				worldColW, worldColH = float64(bowDef.Graphics.BaseSize), float64(bowDef.Graphics.BaseSize)
			}
			// Apply the same distance-based scaling as the arrow visual
			scaleFactor := float64(arrowSize) / float64(bowDef.Graphics.BaseSize)
			screenColW := int(worldColW * scaleFactor)
			screenColH := int(worldColH * scaleFactor)
			// Only draw collision box if we have valid dimensions
			if screenColW > 0 && screenColH > 0 {
				boxX := screenX - screenColW/2
				boxY := screenY + (arrowSize-screenColH)/2
				boxColor := color.RGBA{0, 255, 255, 120} // Cyan, semi-transparent
				boxOpts := &ebiten.DrawImageOptions{}
				boxOpts.GeoM.Scale(float64(screenColW), float64(screenColH))
				boxOpts.GeoM.Translate(float64(boxX), float64(boxY))
				boxOpts.ColorScale.Scale(
					float32(boxColor.R)/255,
					float32(boxColor.G)/255,
					float32(boxColor.B)/255,
					float32(boxColor.A)/255*0.5,
				)
				screen.DrawImage(r.whiteImg, boxOpts)
			}
		}

		// Draw arrow using bow-specific color from config
		arrowColor := bowDef.Graphics.Color

		centerX := float64(screenX)
		centerY := float64(screenY) + float64(arrowSize)/2
		fxProfile := r.weaponFxProfile(bowDef)
		critBoost := 1.0
		if arrow.Crit {
			critBoost = 1.2
		}
		if fxProfile.style != "" {
			// Staff/book bolt: glowing pixel-particle body + evaporating trail as
			// spells, mirrored (R→L) for arcane. Reuses the spell FX renderer.
			glowSize := float64(arrowSize) * fxProfile.glowScale * critBoost
			r.drawGlowSprite(screen, centerX, centerY, glowSize, fxProfile.glowColor, 0.6*critBoost, glowBlend)
			dirX, _, ok := r.projectileScreenDir(arrow.VelX, arrow.VelY)
			if !ok {
				dirX = 1
			}
			r.drawSpellProjectileFx(screen, centerX, centerY, float64(arrowSize), dirX,
				arrowColor, fxProfile, critBoost, idx)
			continue
		}

		// Plain arrow: crisp element-coloured discs in a touching line, no glow.
		r.drawArrowBolt(screen, centerX, centerY, float64(arrowSize), arrowScreenAngle, arrowColor, 1.0)
	}
}

// arrowScreenAngle is the fixed screen tilt arrows fly/stick at (up-left, R→L) —
// same diagonal as the staff bolt — so the projectile reads side-on.
const arrowScreenAngle = -2.7

// arrowTrailCircles is the max number of trail circles behind the head (older
// ones drop off), per the arrow spec.
const arrowTrailCircles = 5

// drawArrowBolt draws an arrow as a short line of touching round circles in the
// bow's element colour, along `angle`. Perspective: the leading tip is smallest
// and each circle toward the tail (nearer the camera) is slightly bigger. Drawn
// source-over (no additive bloom) so the colour stays vivid, not a bright glow.
// `head` sizes the tail (largest) circle; caller scales it by distance. Used in
// flight (alpha 1) and frozen-on-hit (fading).
func (r *Renderer) drawArrowBolt(screen *ebiten.Image, cx, cy, head, angle float64, color [3]int, alpha float64) {
	if head < 2 || alpha <= 0 {
		return
	}
	ca, sa := math.Cos(angle), math.Sin(angle)
	const n = arrowTrailCircles + 1 // tip + trail
	var px, py, sz [n]float64
	pos, prevR := 0.0, 0.0
	for k := 0; k < n; k++ {
		t := float64(k) / float64(n-1)  // 0 tip → 1 tail
		s := head * (0.25 + 0.25*t)     // small circles; tip smallest, tail biggest
		rr := s * 0.5
		if k > 0 {
			pos += (prevR + rr) * 0.5 // strong overlap — a near-continuous line
		}
		px[k], py[k], sz[k] = cx-ca*pos, cy-sa*pos, s
		prevR = rr
	}
	// Soft additive glow circles (same look as the bolt, just smaller) with a
	// faint bloom. Back-to-front so the leading tip is on top.
	for k := n - 1; k >= 0; k-- {
		r.drawGlowSprite(screen, px[k], py[k], sz[k], color, alpha, additiveGlowBlend)
	}
}

// drawSlashEffects draws slash animations for melee weapons
func (r *Renderer) drawSlashEffects(screen *ebiten.Image) {
	if len(r.game.slashEffects) == 0 {
		return
	}
	cx := float64(r.game.config.GetScreenWidth()) / 2
	screenH := float64(r.game.config.GetScreenHeight())
	cy := screenH * meleeAnchorYFrac // lower on screen — it's the party's own weapon
	// Melee swings are now pure pixel-particle FX (see drawMeleeParticles):
	// a sweeping crescent for slashes, a stab streak for thrusts. The old flat
	// stroke/square renderer was removed.
	for _, slash := range r.game.slashEffects {
		if !slash.Active || slash.MaxFrames <= 0 {
			continue
		}
		r.drawMeleeParticles(screen, slash, cx, cy, screenH)
	}
}

// drawHitEffects draws spell impact particles.
func (r *Renderer) drawHitEffects(screen *ebiten.Image) {
	screenWidth := r.game.config.GetScreenWidth()
	screenHeight := r.game.config.GetScreenHeight()
	centerX := float64(screenWidth) / 2
	centerY := float64(screenHeight) / 2

	// Draw spell hit particles
	for i := range r.game.spellHitEffects {
		effect := &r.game.spellHitEffects[i]
		if !effect.Active {
			continue
		}

		for j := range effect.Particles {
			particle := &effect.Particles[j]
			if !particle.Active {
				continue
			}

			// Project the anchor (impact point), then add the particle's screen-space
			// offset so the burst spreads in 2D (up/down/sideways), not a ground line.
			dx := particle.X - r.game.camera.X
			dy := particle.Y - r.game.camera.Y

			cosAngle := math.Cos(r.game.camera.Angle)
			sinAngle := math.Sin(r.game.camera.Angle)
			relY := dx*cosAngle + dy*sinAngle
			relX := -dx*sinAngle + dy*cosAngle

			if relY <= 0.1 {
				continue
			}

			fov := r.game.camera.FOV
			scale := float64(screenHeight) / (relY * fov)
			screenX := centerX + relX*scale + particle.OffsetX*scale
			screenY := centerY + particle.OffsetY*scale

			if screenX < -20 || screenX > float64(screenWidth)+20 {
				continue
			}

			// Alpha/size from remaining lifetime; particles shrink as they fade
			// but keep some body so they read as pixels, not dust.
			lifeRatio := float64(particle.LifeTime) / float64(particle.MaxLife)
			if lifeRatio < 0 {
				lifeRatio = 0
			}
			size := spellParticleScreenSize(particle.Size, lifeRatio, scale)
			// Square pixel particle (matches the impassable-aura / projectile look).
			r.drawGlowRect(screen, screenX, screenY, size, particle.Color, lifeRatio, additiveGlowBlend)
		}
	}
}
