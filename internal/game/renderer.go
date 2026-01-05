package game

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"strings"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/threading/rendering"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// TransparentSpriteData holds cached data for transparent environment sprites
type TransparentSpriteData struct {
	tileX    int
	tileY    int
	worldX   float64
	worldY   float64
	tileType world.TileType3D
	sprite   *ebiten.Image // Cached sprite image to avoid lookups every frame
}

// Renderer handles all 3D rendering functionality
type Renderer struct {
	game                     *MMGame
	floorColorCache          map[[2]int]color.RGBA // Now world-level, static after init
	whiteImg                 *ebiten.Image         // 1x1 white image for untextured polygons
	circleCache              map[int]*ebiten.Image // Cached circle masks by diameter
	circleCacheOrder         []int                 // LRU order tracking for circle cache eviction
	renderedSpritesThisFrame map[[2]int]bool       // Track which environment sprites have been rendered this frame
	// Floor rendering optimization buffers
	floorImage  *ebiten.Image // Persistent floor image buffer
	floorPixels []byte        // Persistent pixel buffer for floor rendering
	// Transparent environment sprite cache for performance
	transparentSpritesCache []TransparentSpriteData // Cached list of transparent sprites
	// Precomputed ray direction cache for performance
	rayDirectionsX []float64 // Cached cos values for rays
	rayDirectionsY []float64 // Cached sin values for rays
	// Sprite cache by tile type to avoid repeated GetSprite lookups
	tileTypeSpriteCache map[world.TileType3D]*ebiten.Image
	// Reusable buffer for tree hits to avoid allocation per frame
	treeHits []treeHitData
	// Unified sprite buffer for sorted rendering of all sprite types
	unifiedSprites []UnifiedSpriteRenderData
}

// getWeaponConfig safely retrieves weapon definition without panicking.
func (r *Renderer) getWeaponConfig(weaponName string) *config.WeaponDefinitionConfig {
	weaponKey := items.GetWeaponKeyByName(weaponName)
	weaponDef, exists := config.GetWeaponDefinition(weaponKey)
	if !exists {
		return nil
	}
	return weaponDef
}

// getWeaponConfigByKey safely retrieves weapon definition by key without panicking.
func (r *Renderer) getWeaponConfigByKey(weaponKey string) *config.WeaponDefinitionConfig {
	weaponDef, exists := config.GetWeaponDefinition(weaponKey)
	if !exists {
		return nil
	}
	return weaponDef
}

// NewRenderer creates a new renderer
func NewRenderer(game *MMGame) *Renderer {
	r := &Renderer{
		game:                     game,
		renderedSpritesThisFrame: make(map[[2]int]bool),
		circleCache:              make(map[int]*ebiten.Image),
		tileTypeSpriteCache:      make(map[world.TileType3D]*ebiten.Image),
	}
	r.floorColorCache = make(map[[2]int]color.RGBA)
	r.precomputeFloorColorCache()
	// Create a 1x1 white image for DrawTriangles
	r.whiteImg = ebiten.NewImage(1, 1)
	r.whiteImg.Fill(color.White)

	// Initialize persistent floor rendering buffers
	screenWidth := game.config.GetScreenWidth()
	screenHeight := game.config.GetScreenHeight()
	r.floorImage = ebiten.NewImage(screenWidth, screenHeight)
	r.floorPixels = make([]byte, screenWidth*screenHeight*4)

	// Initialize transparent sprite cache
	r.buildTransparentSpriteCache()

	// Initialize ray direction cache
	rayWidth := game.config.Graphics.RaysPerScreenWidth
	numRays := (screenWidth + rayWidth - 1) / rayWidth // Round up to cover entire screen
	r.rayDirectionsX = make([]float64, numRays)
	r.rayDirectionsY = make([]float64, numRays)

	return r
}

// buildTransparentSpriteCache scans the world once to cache all transparent environment sprites
func (r *Renderer) buildTransparentSpriteCache() {
	if world.GlobalTileManager == nil || r.game.GetCurrentWorld() == nil {
		r.transparentSpritesCache = nil
		return
	}

	var cache []TransparentSpriteData
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

			// Check if it's a transparent environment sprite (trees are rendered separately via raycasting)
			if world.GlobalTileManager.GetRenderType(tileType) == "environment_sprite" &&
				world.GlobalTileManager.IsTransparent(tileType) {

				// Cache the sprite image to avoid lookups every frame
				spriteName := world.GlobalTileManager.GetSprite(tileType)
				if spriteName == "" {
					continue // Skip tiles without sprites
				}
				sprite := r.game.sprites.GetSprite(spriteName)

				cache = append(cache, TransparentSpriteData{
					tileX:    tileX,
					tileY:    tileY,
					worldX:   worldX,
					worldY:   worldY,
					tileType: tileType,
					sprite:   sprite,
				})
			}
		}
	}

	r.transparentSpritesCache = cache
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

// calculateBrightnessWithTorchLight calculates brightness with torch light effects
func (r *Renderer) calculateBrightnessWithTorchLight(worldX, worldY, distance float64) float64 {
	// Base brightness calculation
	brightness := 1.0 - (distance / r.game.camera.ViewDist)
	if brightness < r.game.config.Graphics.BrightnessMin {
		brightness = r.game.config.Graphics.BrightnessMin
	}

	// Apply torch light effect if active
	if r.game.torchLightActive {
		// Calculate distance from camera (torch light source) to the point
		distanceFromTorch := Distance(r.game.camera.X, r.game.camera.Y, worldX, worldY)

		// Check if within torch light radius
		if distanceFromTorch <= r.game.torchLightRadius {
			// Apply 25% brightness increase within 4 tile radius
			torchBrightness := 0.25
			// Fade the effect toward the edge of the radius
			falloff := 1.0 - (distanceFromTorch / r.game.torchLightRadius)
			torchBrightness *= falloff

			brightness += torchBrightness
			if brightness > 1.0 {
				brightness = 1.0
			}
		}
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

	// Precompute ray directions AFTER ensuring correct array size
	r.precomputeRayDirections()

	// Perform raycasting in parallel with performance monitoring using precomputed directions
	raycastTimer := r.game.threading.PerformanceMonitor.StartRaycast()
	results := r.game.threading.ParallelRenderer.RenderRaycast(
		numRays,
		func(rayIndex int, angle float64) (float64, interface{}) {
			// Use precomputed ray directions instead of recomputing sin/cos
			return r.castRayWithPrecomputedDirection(rayIndex)
		},
		func(rayIndex, totalRays int) float64 {
			// Angle calculation not needed for precomputed directions, but kept for compatibility
			return r.game.camera.Angle - r.game.camera.FOV/2 + (float64(rayIndex)/float64(totalRays))*r.game.camera.FOV
		},
	)
	raycastTimer.EndRaycast()

	// Draw simple floor and ceiling before walls/trees so trees are visible above floor
	r.game.threading.PerformanceMonitor.ProfiledFunction("sprite_render", func() {
		r.drawSimpleFloorCeiling(screen)

		// Render the results and update depth buffer
		r.renderRaycastResults(screen, results)

		// Draw all sprites (trees, ferns, monsters, NPCs) sorted by depth
		r.drawAllSpritesSorted(screen)

		// Draw fireballs and sword attacks
		r.drawProjectiles(screen)

		// Draw slash effects
		r.drawSlashEffects(screen)

		// Draw hit effects (stuck arrows, spell particles)
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
// This is the interface method used by the parallel rendering system.
func (r *Renderer) castRayWithType(angle float64) (float64, interface{}) {
	hits := r.performMultiHitRaycast(angle)
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

	hits := r.performMultiHitRaycastWithDirection(rayDirectionX, rayDirectionY)
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
func (r *Renderer) performMultiHitRaycast(angle float64) MultiRaycastHit {
	// Calculate ray direction vector from the given angle
	rayDirectionX := math.Cos(angle)
	rayDirectionY := math.Sin(angle)

	return r.performMultiHitRaycastWithDirection(rayDirectionX, rayDirectionY)
}

// performMultiHitRaycastWithDirection performs DDA raycasting using precomputed ray directions
func (r *Renderer) performMultiHitRaycastWithDirection(rayDirectionX, rayDirectionY float64) MultiRaycastHit {
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

	// Execute the DDA algorithm
	var hits []RaycastHit
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

// drawSimpleFloorCeiling draws a simple 3D perspective floor (and optionally ceiling)
func (r *Renderer) drawSimpleFloorCeiling(screen *ebiten.Image) {
	screenWidth := r.game.config.GetScreenWidth()
	screenHeight := r.game.config.GetScreenHeight()
	tileSize := r.game.config.GetTileSize()
	camX := r.game.camera.X
	camY := r.game.camera.Y
	camAngle := r.game.camera.Angle
	fov := r.game.camera.FOV

	horizon := screenHeight / 2
	tileColorCache := r.floorColorCache

	// Pre-calculate cosine and sine of camera angle
	cosAngle := math.Cos(camAngle)
	sinAngle := math.Sin(camAngle)

	// Pre-calculate camera plane vectors
	// The camera plane is perpendicular to the direction vector
	planeX := math.Cos(camAngle + math.Pi/2)
	planeY := math.Sin(camAngle + math.Pi/2)

	// Adjust plane vectors by FOV
	// The half-width of the camera plane is tan(FOV/2)
	fovFactor := math.Tan(fov / 2)
	planeX *= fovFactor
	planeY *= fovFactor

	// Reuse persistent buffers for floor rendering (optimization)
	floorImage := r.floorImage
	pixels := r.floorPixels

	// OPTIMIZATION: Skip every other row and column for 4x performance boost
	rowStep := 2
	colStep := 2

	// Draw floor and ceiling
	for y := horizon; y < screenHeight; y += rowStep {
		// Relative position of the floor pixel from the center of the screen
		// This determines the distance from the camera to the floor point
		sampleY := float64(y) + float64(rowStep)/2
		p := sampleY - float64(horizon)
		if p == 0 {
			p = 1 // Avoid division by zero
		}

		// Vertical position of the floor, corrected for perspective.
		// This is the distance from the camera to the floor point in camera space.
		// The '0.5 * screenHeight' is a projection plane constant.
		rowDistance := (0.5 * float64(screenHeight) * float64(tileSize)) / p

		// Calculate the world coordinates for the leftmost and rightmost pixels of this scanline
		// Start of the scanline (leftmost pixel)
		floorX := camX + rowDistance*cosAngle - rowDistance*planeX
		floorY := camY + rowDistance*sinAngle - rowDistance*planeY

		// End of the scanline (rightmost pixel)
		endFloorX := camX + rowDistance*cosAngle + rowDistance*planeX
		endFloorY := camY + rowDistance*sinAngle + rowDistance*planeY

		// Calculate the step to increment world coordinates for each pixel in this scanline
		stepX := (endFloorX - floorX) / float64(screenWidth)
		stepY := (endFloorY - floorY) / float64(screenWidth)

		// Sample from the center of each block to reduce shimmer at distance.
		floorX += stepX * (float64(colStep) * 0.5)
		floorY += stepY * (float64(colStep) * 0.5)

		for x := 0; x < screenWidth; x += colStep {
			// Get the tile coordinates from world coordinates
			tileX := int(math.Floor(floorX / float64(tileSize)))
			tileY := int(math.Floor(floorY / float64(tileSize)))

			// Get color from tile cache
			clr, ok := tileColorCache[[2]int{tileX, tileY}]
			if !ok {
				clr = color.RGBA{30, 30, 30, 255} // Fallback color (dark gray)
			}

			// Apply distance shading with torch light effects
			dist := math.Sqrt(math.Pow(floorX-camX, 2) + math.Pow(floorY-camY, 2))
			brightness := r.calculateBrightnessWithTorchLight(floorX, floorY, dist)

			// Set pixel color for 2x2 block to fill in the gaps
			for dx := 0; dx < colStep && x+dx < screenWidth; dx++ {
				for dy := 0; dy < rowStep && y+dy < screenHeight; dy++ {
					idx := ((y+dy)*screenWidth + (x + dx)) * 4
					pixels[idx] = uint8(float64(clr.R) * brightness)
					pixels[idx+1] = uint8(float64(clr.G) * brightness)
					pixels[idx+2] = uint8(float64(clr.B) * brightness)
					pixels[idx+3] = clr.A
				}
			}

			// Move to the next world coordinate (skip by colStep)
			floorX += stepX * float64(colStep)
			floorY += stepY * float64(colStep)
		}
	}

	floorImage.WritePixels(pixels)
	screen.DrawImage(floorImage, nil)
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
	spriteHeight := int(float64(screenHeight) / distance * r.game.config.GetTileSize() * r.game.config.Graphics.Sprite.TreeHeightMultiplier)
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

// getCachedSprite returns a cached sprite for the given tile type, loading it if necessary
func (r *Renderer) getCachedSprite(tileType world.TileType3D) *ebiten.Image {
	// Check cache first
	if sprite, ok := r.tileTypeSpriteCache[tileType]; ok {
		return sprite
	}

	// Load and cache the sprite
	var spriteName string
	if world.GlobalTileManager != nil {
		spriteName = world.GlobalTileManager.GetSprite(tileType)
	}
	if spriteName == "" {
		return nil // No sprite defined for this tile type
	}

	sprite := r.game.sprites.GetSprite(spriteName)
	r.tileTypeSpriteCache[tileType] = sprite
	return sprite
}

// drawEnvironmentSprite draws environment sprites in the 3D world
func (r *Renderer) drawEnvironmentSprite(screen *ebiten.Image, x int, distance float64, tileType world.TileType3D) {
	// Calculate sprite height and position
	spriteHeight := int(float64(r.game.config.GetScreenHeight()) / distance * r.game.config.GetTileSize() * r.game.config.Graphics.Sprite.TreeHeightMultiplier)
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

	// Get cached sprite for this tile type
	sprite := r.getCachedSprite(tileType)
	if sprite == nil {
		return // No sprite defined for this tile type
	}

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

	// Create cache key - pass original height, cache will quantize internally
	cacheKey := rendering.WallSliceKey{
		Height:   wallHeight,
		Width:    width,
		TileType: tileType,
		Side:     wallSide,
		WallX:    textureCoord,
	}

	// Get cached base wall slice or create new one if not in cache
	// The cache quantizes height and passes the quantized value to createFunc
	wallSliceImage := r.game.threading.WallSliceCache.GetOrCreate(cacheKey, func(quantizedHeight int) *ebiten.Image {
		return r.game.renderHelper.CreateBaseTexturedWallSlice(tileType, width, quantizedHeight, wallSide, textureCoord)
	})

	// Render the wall slice to the screen with distance-based shading applied at draw time
	drawOptions := &ebiten.DrawImageOptions{}

	// Scale the cached image to exact wall height
	// The cached image may be quantized to a different height for cache efficiency
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

type projectileFxProfile struct {
	glowColor        [3]int
	trailColor       [3]int
	glowScale        float64
	trailLengthScale float64
	trailWidthScale  float64
	pulseSpeed       float64
	spark            bool
	sparkColor       [3]int
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
		case "water":
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
	}
	return profile
}

func (r *Renderer) drawGlowRect(screen *ebiten.Image, x, y, size float64, rgb [3]int, alpha float64, blend ebiten.Blend) {
	if size <= 0 || alpha <= 0 {
		return
	}
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Scale(size, size)
	opts.GeoM.Translate(x-size/2, y-size/2)
	opts.ColorScale.Scale(
		float32(rgb[0])/255,
		float32(rgb[1])/255,
		float32(rgb[2])/255,
		float32(alpha),
	)
	opts.Blend = blend
	screen.DrawImage(r.whiteImg, opts)
}

func (r *Renderer) drawTrail(screen *ebiten.Image, x, y, dirX, dirY, length, width float64, rgb [3]int, alpha float64, blend ebiten.Blend) {
	if length <= 0 || width <= 0 || alpha <= 0 {
		return
	}
	dirLen := math.Hypot(dirX, dirY)
	if dirLen <= 0 {
		return
	}
	dirX /= dirLen
	dirY /= dirLen
	segments := int(length / 4)
	if segments < 4 {
		segments = 4
	}
	for i := 0; i < segments; i++ {
		t := float64(i) / float64(segments)
		segX := x - dirX*length*t
		segY := y - dirY*length*t
		segW := width * (1.0 - 0.75*t)
		if segW < 1 {
			segW = 1
		}
		r.drawGlowRect(screen, segX, segY, segW, rgb, alpha*(1.0-t), blend)
	}
}

const circleCacheMaxSize = 64 // Maximum cached circle images to prevent memory bloat

func (r *Renderer) getCircleImage(diameter int) *ebiten.Image {
	if diameter <= 1 {
		return r.whiteImg
	}
	if img, ok := r.circleCache[diameter]; ok {
		// Move to end of LRU order (most recently used)
		r.circleCacheMoveToEnd(diameter)
		return img
	}

	// Evict oldest entries if cache is full (before adding new entry)
	for len(r.circleCache) >= circleCacheMaxSize {
		if len(r.circleCacheOrder) > 0 {
			oldest := r.circleCacheOrder[0]
			delete(r.circleCache, oldest)
			r.circleCacheOrder = r.circleCacheOrder[1:]
		} else {
			break
		}
	}

	img := ebiten.NewImage(diameter, diameter)
	cx := float64(diameter-1) / 2
	cy := float64(diameter-1) / 2
	radius := float64(diameter) / 2
	r2 := radius * radius
	for y := 0; y < diameter; y++ {
		dy := float64(y) - cy
		for x := 0; x < diameter; x++ {
			dx := float64(x) - cx
			if dx*dx+dy*dy <= r2 {
				img.Set(x, y, color.White)
			}
		}
	}
	r.circleCache[diameter] = img
	r.circleCacheOrder = append(r.circleCacheOrder, diameter)
	return img
}

// circleCacheMoveToEnd moves a diameter to the end of LRU order
func (r *Renderer) circleCacheMoveToEnd(diameter int) {
	for i, d := range r.circleCacheOrder {
		if d == diameter {
			r.circleCacheOrder = append(r.circleCacheOrder[:i], r.circleCacheOrder[i+1:]...)
			r.circleCacheOrder = append(r.circleCacheOrder, diameter)
			return
		}
	}
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
)

// UnifiedSpriteRenderData holds data for rendering any sprite type in a unified sorted pass
type UnifiedSpriteRenderData struct {
	spriteType SpriteType
	screenX    int
	screenY    int
	spriteSize int
	depthPerp  float64 // Camera-space perpendicular depth (for z-buffer comparison)
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

			sprites = append(sprites, UnifiedSpriteRenderData{
				spriteType: SpriteTypeEnvironment,
				screenX:    screenX,
				screenY:    screenY,
				spriteSize: spriteSize,
				depthPerp:  depthPerp,
				sprite:     spriteData.sprite,
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
		dx := npc.X - camX
		dy := npc.Y - camY
		distanceSq := dx*dx + dy*dy

		if distanceSq > viewDistSq {
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
		}
	}
}

// drawUnifiedEnvironmentSprite draws an environment sprite from unified data
func (r *Renderer) drawUnifiedEnvironmentSprite(screen *ebiten.Image, s UnifiedSpriteRenderData) {
	drawLeft := s.screenX - s.spriteSize/2
	drawRight := s.screenX + s.spriteSize/2
	depthLeft := drawLeft
	depthRight := drawRight

	if depthLeft < 0 {
		depthLeft = 0
	}
	if depthRight >= len(r.game.depthBuffer) {
		depthRight = len(r.game.depthBuffer) - 1
	}

	visible := false
	for x := depthLeft; x <= depthRight; x++ {
		if s.depthPerp < r.game.depthBuffer[x] {
			visible = true
			break
		}
	}
	if !visible {
		return
	}

	opts := &ebiten.DrawImageOptions{}
	scaleX := float64(s.spriteSize) / float64(s.sprite.Bounds().Dx())
	scaleY := float64(s.spriteSize) / float64(s.sprite.Bounds().Dy())
	opts.GeoM.Scale(scaleX, scaleY)
	opts.GeoM.Translate(float64(drawLeft), float64(s.screenY))

	tileSize := float64(r.game.config.GetTileSize())
	worldX, worldY := TileCenterFromTile(s.tileX, s.tileY, tileSize)
	distance := math.Sqrt(math.Pow(worldX-r.game.camera.X, 2) + math.Pow(worldY-r.game.camera.Y, 2))
	brightness := r.calculateBrightnessWithTorchLight(worldX, worldY, distance)
	opts.ColorScale.Scale(float32(brightness), float32(brightness), float32(brightness), 1.0)
	opts.Blend = ebiten.BlendSourceOver

	screen.DrawImage(s.sprite, opts)
}

// drawUnifiedMonsterSprite draws a monster sprite from unified data
func (r *Renderer) drawUnifiedMonsterSprite(screen *ebiten.Image, s UnifiedSpriteRenderData) {
	drawLeft := s.screenX - s.spriteSize/2
	drawRight := s.screenX + s.spriteSize/2
	depthLeft := drawLeft
	depthRight := drawRight

	if depthLeft < 0 {
		depthLeft = 0
	}
	if depthRight >= len(r.game.depthBuffer) {
		depthRight = len(r.game.depthBuffer) - 1
	}

	visible := false
	for x := depthLeft; x <= depthRight; x++ {
		if s.depthPerp < r.game.depthBuffer[x] {
			visible = true
			break
		}
	}
	if !visible {
		return
	}

	opts := &ebiten.DrawImageOptions{}
	scaleX := float64(s.spriteSize) / float64(s.sprite.Bounds().Dx())
	scaleY := float64(s.spriteSize) / float64(s.sprite.Bounds().Dy())

	if s.monsterFlip {
		opts.GeoM.Scale(-scaleX, scaleY)
		opts.GeoM.Translate(float64(drawLeft+s.spriteSize), float64(s.screenY))
	} else {
		opts.GeoM.Scale(scaleX, scaleY)
		opts.GeoM.Translate(float64(drawLeft), float64(s.screenY))
	}

	distance := math.Sqrt(math.Pow(s.monster.X-r.game.camera.X, 2) + math.Pow(s.monster.Y-r.game.camera.Y, 2))
	brightness := r.calculateBrightnessWithTorchLight(s.monster.X, s.monster.Y, distance)
	opts.ColorScale.Scale(float32(brightness), float32(brightness), float32(brightness), 1.0)
	opts.Blend = ebiten.BlendSourceOver

	screen.DrawImage(s.sprite, opts)
}

// drawUnifiedNPCSprite draws an NPC sprite from unified data
func (r *Renderer) drawUnifiedNPCSprite(screen *ebiten.Image, s UnifiedSpriteRenderData) {
	drawLeft := s.screenX - s.spriteSize/2
	drawRight := s.screenX + s.spriteSize/2
	depthLeft := drawLeft
	depthRight := drawRight

	if depthLeft < 0 {
		depthLeft = 0
	}
	if depthRight >= len(r.game.depthBuffer) {
		depthRight = len(r.game.depthBuffer) - 1
	}

	visible := false
	for x := depthLeft; x <= depthRight; x++ {
		if s.depthPerp < r.game.depthBuffer[x] {
			visible = true
			break
		}
	}
	if !visible {
		return
	}

	opts := &ebiten.DrawImageOptions{}
	scaleX := float64(s.spriteSize) / float64(s.sprite.Bounds().Dx())
	scaleY := float64(s.spriteSize) / float64(s.sprite.Bounds().Dy())
	opts.GeoM.Scale(scaleX, scaleY)
	opts.GeoM.Translate(float64(drawLeft), float64(s.screenY))

	distance := math.Sqrt(math.Pow(s.npc.X-r.game.camera.X, 2) + math.Pow(s.npc.Y-r.game.camera.Y, 2))
	brightness := r.calculateBrightnessWithTorchLight(s.npc.X, s.npc.Y, distance)
	if brightness < r.game.config.Graphics.BrightnessMin {
		brightness = r.game.config.Graphics.BrightnessMin
	}
	opts.ColorScale.Scale(float32(brightness), float32(brightness), float32(brightness), 1.0)
	opts.Blend = ebiten.BlendSourceOver

	screen.DrawImage(s.sprite, opts)
}

// drawProjectiles draws magic projectiles, sword attacks, and arrows
func (r *Renderer) drawProjectiles(screen *ebiten.Image) {
	r.drawMagicProjectiles(screen)
	r.drawMeleeAttacks(screen)
	r.drawArrows(screen)
}

// drawMagicProjectiles draws all active magic projectiles
func (r *Renderer) drawMagicProjectiles(screen *ebiten.Image) {
	glowBlend := ebiten.Blend{
		BlendFactorSourceRGB:        ebiten.BlendFactorSourceAlpha,
		BlendFactorSourceAlpha:      ebiten.BlendFactorSourceAlpha,
		BlendFactorDestinationRGB:   ebiten.BlendFactorOne,
		BlendFactorDestinationAlpha: ebiten.BlendFactorOne,
		BlendOperationRGB:           ebiten.BlendOperationAdd,
		BlendOperationAlpha:         ebiten.BlendOperationAdd,
	}

	for _, magicProjectile := range r.game.magicProjectiles {
		if !magicProjectile.Active {
			continue
		}

		// Calculate magic projectile position relative to camera
		dx := magicProjectile.X - r.game.camera.X
		dy := magicProjectile.Y - r.game.camera.Y
		dist := Distance(r.game.camera.X, r.game.camera.Y, magicProjectile.X, magicProjectile.Y)

		if dist > r.game.camera.ViewDist || dist < 10 {
			continue
		}

		// Calculate angle to magic projectile
		projectileAngle := math.Atan2(dy, dx)
		angleDiff := projectileAngle - r.game.camera.Angle

		// Normalize angle difference
		for angleDiff > math.Pi {
			angleDiff -= 2 * math.Pi
		}
		for angleDiff < -math.Pi {
			angleDiff += 2 * math.Pi
		}

		// Check if magic projectile is in view
		if math.Abs(angleDiff) > r.game.camera.FOV/2 {
			continue
		}

		// Calculate screen position
		screenX := int(float64(r.game.config.GetScreenWidth())/2 + (angleDiff/(r.game.camera.FOV/2))*float64(r.game.config.GetScreenWidth()/2))

		// Calculate camera-space perpendicular depth for depth buffer comparison
		camDirX := math.Cos(r.game.camera.Angle)
		camDirY := math.Sin(r.game.camera.Angle)
		depthPerp := dx*camDirX + dy*camDirY

		// Depth test: check if projectile is behind walls
		if screenX >= 0 && screenX < len(r.game.depthBuffer) {
			if depthPerp >= r.game.depthBuffer[screenX] {
				continue // Projectile is behind a wall
			}
		}

		// Get spell-specific graphics config based on spell type
		// The SpellType string is actually the SpellID (e.g., "firebolt", "fireball")
		spellConfigName := magicProjectile.SpellType
		spellGraphicsConfig, err := r.game.config.GetSpellGraphicsConfig(spellConfigName)
		if err != nil {
			continue // Skip rendering if no graphics config
		}

		// Calculate projectile size based on distance using spell-specific config
		baseSize := float64(spellGraphicsConfig.BaseSize)
		projectileSize := int(baseSize / dist * r.game.config.GetTileSize())
		if projectileSize > spellGraphicsConfig.MaxSize {
			projectileSize = spellGraphicsConfig.MaxSize
		}
		if projectileSize < spellGraphicsConfig.MinSize {
			projectileSize = spellGraphicsConfig.MinSize
		}

		screenY := r.game.config.GetScreenHeight()/2 - projectileSize/2

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
		glowSize := float64(projectileSize) * fxProfile.glowScale * pulse * critBoost
		r.drawGlowRect(screen, centerX, centerY, glowSize, fxProfile.glowColor, 0.7*critBoost, glowBlend)
		trailLen := float64(projectileSize) * fxProfile.trailLengthScale * critBoost
		trailWidth := float64(projectileSize) * fxProfile.trailWidthScale
		if dirX, dirY, ok := r.projectileScreenDir(magicProjectile.VelX, magicProjectile.VelY); ok {
			r.drawTrail(screen, centerX, centerY, dirX, dirY, trailLen, trailWidth, fxProfile.trailColor, 0.75*critBoost, glowBlend)
		}
		if fxProfile.spark {
			angle := (float64(r.game.frameCount) * fxProfile.pulseSpeed * 0.12)
			orbital := float64(projectileSize) * 0.55
			sparkX := centerX + math.Cos(angle)*orbital
			sparkY := centerY + math.Sin(angle)*orbital*0.4
			r.drawGlowRect(screen, sparkX, sparkY, math.Max(2, float64(projectileSize)*0.25), fxProfile.sparkColor, 0.8*critBoost, glowBlend)
		}

		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Scale(float64(projectileSize), float64(projectileSize))
		opts.GeoM.Translate(float64(screenX-projectileSize/2), float64(screenY))
		opts.ColorScale.Scale(
			float32(projectileColor[0])/255,
			float32(projectileColor[1])/255,
			float32(projectileColor[2])/255,
			1,
		)
		screen.DrawImage(r.whiteImg, opts)
	}
}

// drawMeleeAttacks draws all active melee attacks
func (r *Renderer) drawMeleeAttacks(screen *ebiten.Image) {
	for _, attack := range r.game.meleeAttacks {
		if !attack.Active {
			continue
		}

		// Calculate attack position relative to camera
		dx := attack.X - r.game.camera.X
		dy := attack.Y - r.game.camera.Y
		dist := Distance(r.game.camera.X, r.game.camera.Y, attack.X, attack.Y)

		if dist > r.game.camera.ViewDist || dist < 10 {
			continue
		}

		// Calculate angle to attack
		attackAngle := math.Atan2(dy, dx)
		angleDiff := attackAngle - r.game.camera.Angle

		// Normalize angle difference
		for angleDiff > math.Pi {
			angleDiff -= 2 * math.Pi
		}
		for angleDiff < -math.Pi {
			angleDiff += 2 * math.Pi
		}

		// Check if attack is in view
		if math.Abs(angleDiff) > r.game.camera.FOV/2 {
			continue
		}

		// Calculate screen position
		screenX := int(float64(r.game.config.GetScreenWidth())/2 + (angleDiff/(r.game.camera.FOV/2))*float64(r.game.config.GetScreenWidth()/2))

		// Calculate camera-space perpendicular depth for depth buffer comparison
		camDirX := math.Cos(r.game.camera.Angle)
		camDirY := math.Sin(r.game.camera.Angle)
		depthPerp := dx*camDirX + dy*camDirY

		// Depth test: check if melee attack is behind walls
		if screenX >= 0 && screenX < len(r.game.depthBuffer) {
			if depthPerp >= r.game.depthBuffer[screenX] {
				continue // Melee attack is behind a wall
			}
		}

		// Get weapon-specific graphics config from YAML
		weaponDef := r.getWeaponConfig(attack.WeaponName)
		if weaponDef == nil || weaponDef.Graphics == nil {
			continue // Skip rendering if weapon config missing
		}

		// Calculate attack size based on distance using weapon-specific config
		baseSize := float64(weaponDef.Graphics.BaseSize)
		attackSize := int(baseSize / dist * r.game.config.GetTileSize())
		if attackSize > weaponDef.Graphics.MaxSize {
			attackSize = weaponDef.Graphics.MaxSize
		}
		if attackSize < weaponDef.Graphics.MinSize {
			attackSize = weaponDef.Graphics.MinSize
		}

		screenY := r.game.config.GetScreenHeight()/2 - attackSize/2

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
	glowBlend := ebiten.Blend{
		BlendFactorSourceRGB:        ebiten.BlendFactorSourceAlpha,
		BlendFactorSourceAlpha:      ebiten.BlendFactorSourceAlpha,
		BlendFactorDestinationRGB:   ebiten.BlendFactorOne,
		BlendFactorDestinationAlpha: ebiten.BlendFactorOne,
		BlendOperationRGB:           ebiten.BlendOperationAdd,
		BlendOperationAlpha:         ebiten.BlendOperationAdd,
	}

	for _, arrow := range r.game.arrows {
		if !arrow.Active {
			continue
		}

		// Calculate arrow position relative to camera
		dx := arrow.X - r.game.camera.X
		dy := arrow.Y - r.game.camera.Y
		dist := Distance(r.game.camera.X, r.game.camera.Y, arrow.X, arrow.Y)

		if dist > r.game.camera.ViewDist || dist < 10 {
			continue
		}

		// Calculate angle to arrow
		arrowAngle := math.Atan2(dy, dx)
		angleDiff := arrowAngle - r.game.camera.Angle

		// Normalize angle difference
		for angleDiff > math.Pi {
			angleDiff -= 2 * math.Pi
		}
		for angleDiff < -math.Pi {
			angleDiff += 2 * math.Pi
		}

		// Check if arrow is in view
		if math.Abs(angleDiff) > r.game.camera.FOV/2 {
			continue
		}

		// Calculate screen position
		screenX := int(float64(r.game.config.GetScreenWidth())/2 + (angleDiff/(r.game.camera.FOV/2))*float64(r.game.config.GetScreenWidth()/2))

		// Calculate camera-space perpendicular depth for depth buffer comparison
		camDirX := math.Cos(r.game.camera.Angle)
		camDirY := math.Sin(r.game.camera.Angle)
		depthPerp := dx*camDirX + dy*camDirY

		// Depth test: check if arrow is behind walls
		if screenX >= 0 && screenX < len(r.game.depthBuffer) {
			if depthPerp >= r.game.depthBuffer[screenX] {
				continue // Arrow is behind a wall
			}
		}

		// Calculate arrow size based on distance using bow-specific config from YAML
		bowDef := r.getWeaponConfigByKey(arrow.BowKey)
		if bowDef == nil || bowDef.Graphics == nil {
			continue // Skip rendering if weapon config missing
		}
		baseSize := float64(bowDef.Graphics.BaseSize)
		arrowSize := int(baseSize / dist * r.game.config.GetTileSize())
		if arrowSize > bowDef.Graphics.MaxSize {
			arrowSize = bowDef.Graphics.MaxSize
		}
		if arrowSize < bowDef.Graphics.MinSize {
			arrowSize = bowDef.Graphics.MinSize
		}

		screenY := r.game.config.GetScreenHeight()/2 - arrowSize/2

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
		glowSize := float64(arrowSize) * fxProfile.glowScale * critBoost
		r.drawGlowRect(screen, centerX, centerY, glowSize, fxProfile.glowColor, 0.6*critBoost, glowBlend)
		if dirX, dirY, ok := r.projectileScreenDir(arrow.VelX, arrow.VelY); ok {
			trailLen := float64(arrowSize) * fxProfile.trailLengthScale * critBoost
			trailWidth := float64(arrowSize) * fxProfile.trailWidthScale
			r.drawTrail(screen, centerX, centerY, dirX, dirY, trailLen, trailWidth, fxProfile.trailColor, 0.7*critBoost, glowBlend)

			tipX := centerX + dirX*float64(arrowSize)*0.35
			tipY := centerY + dirY*float64(arrowSize)*0.35
			r.drawGlowRect(screen, tipX, tipY, math.Max(2, float64(arrowSize)*0.25), fxProfile.glowColor, 0.9*critBoost, glowBlend)
		}

		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Scale(float64(arrowSize), float64(arrowSize))
		opts.GeoM.Translate(float64(screenX-arrowSize/2), float64(screenY))
		opts.ColorScale.Scale(
			float32(arrowColor[0])/255,
			float32(arrowColor[1])/255,
			float32(arrowColor[2])/255,
			1,
		)
		screen.DrawImage(r.whiteImg, opts)
	}
}

// drawSlashEffects draws slash animations for melee weapons
func (r *Renderer) drawSlashEffects(screen *ebiten.Image) {
	screenWidth := r.game.config.GetScreenWidth()
	screenHeight := r.game.config.GetScreenHeight()
	centerX := screenWidth / 2
	centerY := screenHeight / 2

	glowBlend := ebiten.Blend{
		BlendFactorSourceRGB:        ebiten.BlendFactorSourceAlpha,
		BlendFactorSourceAlpha:      ebiten.BlendFactorSourceAlpha,
		BlendFactorDestinationRGB:   ebiten.BlendFactorOne,
		BlendFactorDestinationAlpha: ebiten.BlendFactorOne,
		BlendOperationRGB:           ebiten.BlendOperationAdd,
		BlendOperationAlpha:         ebiten.BlendOperationAdd,
	}

	type strokePass struct {
		widthMul float64
		curveMul float64
		alphaMul float64
		blend    ebiten.Blend
	}

	type trailSpec struct {
		count    int
		spacing  float64
		widthMul float64
		curveMul float64
		alphaMul float64
		blend    ebiten.Blend
	}

	clamp01 := func(t float64) float64 {
		if t < 0 {
			return 0
		}
		if t > 1 {
			return 1
		}
		return t
	}

	easeInOut := func(t float64) float64 {
		t = clamp01(t)
		if t < 0.5 {
			return 4 * t * t * t
		}
		return 1 - math.Pow(-2*t+2, 3)/2
	}

	easeOut := func(t float64) float64 {
		t = clamp01(t)
		return 1 - math.Pow(1-t, 2)
	}

	drawSegment := func(x, y, width, height float64, clr color.RGBA, blend ebiten.Blend) {
		if width <= 0 || height <= 0 {
			return
		}
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Scale(width, height)
		opts.GeoM.Translate(x-width/2, y-height/2)
		opts.ColorScale.Scale(
			float32(clr.R)/255,
			float32(clr.G)/255,
			float32(clr.B)/255,
			float32(clr.A)/255,
		)
		opts.Blend = blend
		screen.DrawImage(r.whiteImg, opts)
	}

	drawCurvedStroke := func(startX, startY, endX, endY, width, curve, alpha float64, rgb [3]int, blend ebiten.Blend) {
		totalLen := math.Hypot(endX-startX, endY-startY)
		segments := int(totalLen)
		if segments < 1 {
			segments = 1
		}
		perpX := 0.0
		perpY := 0.0
		if totalLen > 0 {
			perpX = -(endY - startY) / totalLen
			perpY = (endX - startX) / totalLen
		}
		for i := 0; i < segments; i++ {
			t := float64(i) / float64(segments)
			x := startX + t*(endX-startX)
			y := startY + t*(endY-startY)
			arcOffset := (t - 0.5) * curve
			x += perpX * arcOffset
			y += perpY * arcOffset
			drawSegment(x, y, width, 2, color.RGBA{
				uint8(rgb[0]),
				uint8(rgb[1]),
				uint8(rgb[2]),
				uint8(255 * alpha),
			}, blend)
		}
	}

	drawStrokePasses := func(startX, startY, endX, endY, width, curve, alpha float64, rgb [3]int, passes []strokePass) {
		for _, pass := range passes {
			drawCurvedStroke(
				startX,
				startY,
				endX,
				endY,
				width*pass.widthMul,
				curve*pass.curveMul,
				alpha*pass.alphaMul,
				rgb,
				pass.blend,
			)
		}
	}

	drawTaperedStroke := func(startX, startY, endX, endY, baseWidth, alpha float64, rgb [3]int) {
		totalLen := math.Hypot(endX-startX, endY-startY)
		segments := int(totalLen)
		if segments < 1 {
			segments = 1
		}
		for i := 0; i < segments; i++ {
			t := float64(i) / float64(segments)
			x := startX + t*(endX-startX)
			y := startY + t*(endY-startY)
			width := baseWidth * (1.0 - t*0.7)
			if width < 1 {
				width = 1
			}
			drawSegment(x, y, width*1.6, 4, color.RGBA{
				uint8(rgb[0]),
				uint8(rgb[1]),
				uint8(rgb[2]),
				uint8(120 * alpha),
			}, glowBlend)
			drawSegment(x, y, width, 2, color.RGBA{
				uint8(rgb[0]),
				uint8(rgb[1]),
				uint8(rgb[2]),
				uint8(255 * alpha),
			}, ebiten.Blend{})
		}
	}

	drawSweepTrails := func(baseAngle, halfLength, width, curve, alpha float64, sweepAngle float64, progress float64, trail trailSpec, rgb [3]int) {
		for i := 1; i <= trail.count; i++ {
			ghostProgress := progress - float64(i)*trail.spacing
			if ghostProgress <= 0 {
				continue
			}
			ghostProgress = easeOut(ghostProgress)
			ghostAngle := baseAngle
			if sweepAngle != 0 {
				windup := sweepAngle * 0.35
				overshoot := sweepAngle * 0.15
				start := baseAngle - sweepAngle/2.0 - windup
				end := baseAngle + sweepAngle/2.0 + overshoot
				ghostAngle = start + (end-start)*ghostProgress
			}
			ghostHalf := halfLength * (0.9 + 0.1*ghostProgress)
			ghostStartX := float64(centerX) - math.Cos(ghostAngle)*ghostHalf
			ghostStartY := float64(centerY) - math.Sin(ghostAngle)*ghostHalf
			ghostEndX := float64(centerX) + math.Cos(ghostAngle)*ghostHalf
			ghostEndY := float64(centerY) + math.Sin(ghostAngle)*ghostHalf
			ghostAlpha := alpha * (trail.alphaMul / float64(i+1))
			drawCurvedStroke(ghostStartX, ghostStartY, ghostEndX, ghostEndY, width*trail.widthMul, curve*trail.curveMul, ghostAlpha, rgb, trail.blend)
		}
	}

	for _, slash := range r.game.slashEffects {
		if !slash.Active {
			continue
		}

		if slash.MaxFrames <= 0 {
			continue
		}

		// Calculate animation progress (0.0 to 1.0)
		progress := float64(slash.AnimationFrame) / float64(slash.MaxFrames)
		progress = clamp01(progress)

		// Fade out the slash effect over time
		alpha := 1.0 - progress
		if alpha < 0 {
			alpha = 0
		}

		switch slash.Style {
		case SlashEffectStyleThrust:
			thrust := 0.5 - 0.5*math.Cos(progress*math.Pi) // Ease in/out
			if thrust < 0 {
				thrust = 0
			}
			angle := slash.Angle
			length := float64(slash.Length) * (0.4 + 0.7*thrust)
			offset := float64(slash.Length) * 0.18 * thrust

			startX := float64(centerX) + math.Cos(angle)*offset
			startY := float64(centerY) + math.Sin(angle)*offset
			endX := startX + math.Cos(angle)*length
			endY := startY + math.Sin(angle)*length

			baseWidth := float64(slash.Width)
			drawTaperedStroke(startX, startY, endX, endY, baseWidth, alpha, slash.Color)

			// Add a brighter tip for the thrust
			tipSize := math.Max(3, float64(slash.Width)/2.4)
			drawSegment(endX, endY, tipSize*1.4, tipSize*1.4, color.RGBA{255, 255, 255, uint8(200 * alpha)}, glowBlend)
			drawSegment(endX, endY, tipSize, tipSize, color.RGBA{255, 255, 255, uint8(255 * alpha)}, ebiten.Blend{})

			// Quick side streaks for extra punch
			if progress > 0.45 && progress < 0.9 {
				streakAlpha := alpha * 0.6
				sideAngle := angle + math.Pi/2
				streakLen := float64(slash.Width) * 1.6
				streakX := endX + math.Cos(sideAngle)*4
				streakY := endY + math.Sin(sideAngle)*4
				drawCurvedStroke(
					streakX-math.Cos(sideAngle)*streakLen/2,
					streakY-math.Sin(sideAngle)*streakLen/2,
					streakX+math.Cos(sideAngle)*streakLen/2,
					streakY+math.Sin(sideAngle)*streakLen/2,
					float64(slash.Width)*0.35,
					0,
					streakAlpha,
					slash.Color,
					glowBlend,
				)
			}
		default:
			// Slash style: sweep angle and slight curvature
			angle := slash.Angle
			if slash.SweepAngle != 0 {
				windup := slash.SweepAngle * 0.35
				overshoot := slash.SweepAngle * 0.15
				start := slash.Angle - slash.SweepAngle/2.0 - windup
				end := slash.Angle + slash.SweepAngle/2.0 + overshoot
				angle = start + (end-start)*easeInOut(progress)
			}
			pulse := math.Sin(progress * math.Pi)
			halfLength := float64(slash.Length) * (0.75 + 0.35*pulse) / 2.0
			startX := float64(centerX) - math.Cos(angle)*halfLength
			startY := float64(centerY) - math.Sin(angle)*halfLength
			endX := float64(centerX) + math.Cos(angle)*halfLength
			endY := float64(centerY) + math.Sin(angle)*halfLength

			width := float64(slash.Width) * (0.7 + 0.35*pulse)
			curve := width * 0.55 * (1.0 - math.Abs(2*progress-1))

			slashPasses := []strokePass{
				{widthMul: 1.8, curveMul: 1.2, alphaMul: 0.25, blend: glowBlend},
				{widthMul: 1.25, curveMul: 0.9, alphaMul: 0.5, blend: glowBlend},
				{widthMul: 1.0, curveMul: 1.0, alphaMul: 1.0, blend: ebiten.Blend{}},
			}
			drawStrokePasses(startX, startY, endX, endY, width, curve, alpha, slash.Color, slashPasses)

			// Afterimage trails for follow-through
			drawSweepTrails(
				slash.Angle,
				halfLength,
				width,
				curve,
				alpha,
				slash.SweepAngle,
				progress,
				trailSpec{
					count:    2,
					spacing:  0.12,
					widthMul: 0.9,
					curveMul: 0.6,
					alphaMul: 0.35,
					blend:    glowBlend,
				},
				slash.Color,
			)

			// Small spark near the end of the swing
			if progress > 0.7 {
				sparkAlpha := (progress - 0.7) / 0.3
				sparkAlpha = clamp01(sparkAlpha)
				sparkAngle := angle + math.Pi/2
				sparkLen := float64(slash.Width) * 1.4
				sparkCenterX := endX
				sparkCenterY := endY
				drawCurvedStroke(
					sparkCenterX-math.Cos(sparkAngle)*sparkLen/2,
					sparkCenterY-math.Sin(sparkAngle)*sparkLen/2,
					sparkCenterX+math.Cos(sparkAngle)*sparkLen/2,
					sparkCenterY+math.Sin(sparkAngle)*sparkLen/2,
					float64(slash.Width)*0.4,
					0,
					sparkAlpha*alpha,
					slash.Color,
					glowBlend,
				)
			}
		}
	}
}

// drawHitEffects draws stuck arrows and spell impact particles
func (r *Renderer) drawHitEffects(screen *ebiten.Image) {
	screenWidth := r.game.config.GetScreenWidth()
	screenHeight := r.game.config.GetScreenHeight()
	centerX := float64(screenWidth) / 2
	centerY := float64(screenHeight) / 2

	// Draw arrow hit particles
	for i := range r.game.arrowHitEffects {
		effect := &r.game.arrowHitEffects[i]
		if !effect.Active {
			continue
		}

		for j := range effect.Particles {
			particle := &effect.Particles[j]
			if !particle.Active {
				continue
			}

			// Calculate screen position using perspective
			dx := particle.X - r.game.camera.X
			dy := particle.Y - r.game.camera.Y

			// Rotate to camera space (relY is forward depth, relX is lateral offset)
			cosAngle := math.Cos(r.game.camera.Angle)
			sinAngle := math.Sin(r.game.camera.Angle)
			relY := dx*cosAngle + dy*sinAngle
			relX := -dx*sinAngle + dy*cosAngle

			// Skip if behind camera
			if relY <= 0.1 {
				continue
			}

			// Project to screen
			fov := r.game.camera.FOV
			scale := float64(screenHeight) / (relY * fov)
			screenX := centerX + relX*scale + particle.OffsetX*scale
			screenY := centerY + particle.OffsetY*scale

			// Skip if off screen
			if screenX < -20 || screenX > float64(screenWidth)+20 {
				continue
			}

			lifeRatio := float64(particle.LifeTime) / float64(particle.MaxLife)
			alpha := lifeRatio
			size := float64(particle.Size) * scale
			if size < 1 {
				size = 1
			}
			if size > 6 {
				size = 6
			}

			opts := &ebiten.DrawImageOptions{}
			opts.GeoM.Scale(size, size)
			opts.GeoM.Translate(screenX-size/2, screenY-size/2)
			opts.ColorScale.Scale(
				float32(particle.Color[0])/255,
				float32(particle.Color[1])/255,
				float32(particle.Color[2])/255,
				float32(alpha),
			)
			screen.DrawImage(r.whiteImg, opts)
		}
	}

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

			// Calculate screen position
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
			screenX := centerX + relX*scale
			screenY := centerY

			if screenX < -20 || screenX > float64(screenWidth)+20 {
				continue
			}

			// Calculate alpha and size based on lifetime
			lifeRatio := float64(particle.LifeTime) / float64(particle.MaxLife)
			alpha := lifeRatio
			size := float64(particle.Size) * lifeRatio * scale
			if size < 1 {
				size = 1
			}
			if size > 12 {
				size = 12
			}

			clr := color.RGBA{
				uint8(particle.Color[0]),
				uint8(particle.Color[1]),
				uint8(particle.Color[2]),
				uint8(255 * alpha),
			}
			diameter := int(size) + 2
			if diameter < 2 {
				diameter = 2
			}
			particleImg := r.getCircleImage(diameter)

			opts := &ebiten.DrawImageOptions{}
			opts.GeoM.Translate(screenX-float64(diameter)/2, screenY-float64(diameter)/2)
			// Use additive blending for glow effect
			opts.Blend = ebiten.Blend{
				BlendFactorSourceRGB:        ebiten.BlendFactorSourceAlpha,
				BlendFactorSourceAlpha:      ebiten.BlendFactorSourceAlpha,
				BlendFactorDestinationRGB:   ebiten.BlendFactorOne,
				BlendFactorDestinationAlpha: ebiten.BlendFactorOne,
				BlendOperationRGB:           ebiten.BlendOperationAdd,
				BlendOperationAlpha:         ebiten.BlendOperationAdd,
			}
			opts.ColorScale.Scale(
				float32(clr.R)/255,
				float32(clr.G)/255,
				float32(clr.B)/255,
				float32(clr.A)/255,
			)
			screen.DrawImage(particleImg, opts)
		}
	}
}
