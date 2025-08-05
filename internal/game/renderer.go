package game

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"strings"
	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/threading/rendering"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// Renderer handles all 3D rendering functionality
type Renderer struct {
	game                     *MMGame
	floorColorCache          map[[2]int]color.RGBA // Now world-level, static after init
	whiteImg                 *ebiten.Image         // 1x1 white image for untextured polygons
	renderedSpritesThisFrame map[[2]int]bool       // Track which environment sprites have been rendered this frame
}

// NewRenderer creates a new renderer
func NewRenderer(game *MMGame) *Renderer {
	r := &Renderer{
		game:                     game,
		renderedSpritesThisFrame: make(map[[2]int]bool),
	}
	r.floorColorCache = make(map[[2]int]color.RGBA)
	r.precomputeFloorColorCache()
	// Create a 1x1 white image for DrawTriangles
	r.whiteImg = ebiten.NewImage(1, 1)
	r.whiteImg.Fill(color.White)
	return r
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
		dx := worldX - r.game.camera.X
		dy := worldY - r.game.camera.Y
		distanceFromTorch := math.Sqrt(dx*dx + dy*dy)

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
			// Only apply nearby tile effects if current tile is not a special colored tile
			if nearSpecialTile {
				// Check if current tile is a special tile that should preserve its color
				shouldPreserveColor := false
				switch currentTile {
				case world.TileVioletTeleporter, world.TileRedTeleporter:
					// Teleporters should keep their colors
					shouldPreserveColor = true
				default:
					// Regular tiles (including empty) can be affected by nearby tiles
					shouldPreserveColor = false
				}

				// Only override with nearby tile color if current tile allows it
				if !shouldPreserveColor {
					clr = nearTileColor
				}
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

	// Parallel raycasting for better performance
	numRays := r.game.config.GetScreenWidth() / r.game.config.Graphics.RaysPerScreenWidth

	// Perform raycasting in parallel with performance monitoring using proper camera plane
	raycastTimer := r.game.threading.PerformanceMonitor.StartRaycast()
	results := r.game.threading.ParallelRenderer.RenderRaycast(
		numRays,
		func(rayIndex int, angle float64) (float64, interface{}) {
			return r.castRayWithType(angle)
		},
		func(rayIndex, totalRays int) float64 {
			// Simple angle calculation (revert to original method)
			return r.game.camera.Angle - r.game.camera.FOV/2 + (float64(rayIndex)/float64(totalRays))*r.game.camera.FOV
		},
	)
	raycastTimer.EndRaycast()

	// Draw simple floor and ceiling before walls/trees so trees are visible above floor
	r.drawSimpleFloorCeiling(screen)

	// Render the results and update depth buffer
	r.renderRaycastResults(screen, results)

	// Draw monsters as sprites using parallel processing with depth testing
	r.drawMonstersParallel(screen)

	// Draw NPCs as sprites using depth testing
	r.drawNPCs(screen)

	// Draw fireballs and sword attacks
	r.drawProjectiles(screen)
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

// performMultiHitRaycast performs DDA raycasting that can return multiple hits for transparency
func (r *Renderer) performMultiHitRaycast(angle float64) MultiRaycastHit {
	// Calculate ray direction vector from the given angle
	rayDirectionX := math.Cos(angle)
	rayDirectionY := math.Sin(angle)

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
		worldX := float64(currentTileX)*tileSize + tileSize/2
		worldY := float64(currentTileY)*tileSize + tileSize/2
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

		// Check what type of tile this is and use tile manager for properties
		isTransparent := false
		if world.GlobalTileManager != nil {
			isTransparent = world.GlobalTileManager.IsTransparent(tileType)
		}
		// Note: If tile manager is not available, default to solid (non-transparent)

		if isTransparent {
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

// renderRaycastResults processes and renders the results from parallel raycasting.
// Each result contains distance and hit information for one vertical screen column.
func (r *Renderer) renderRaycastResults(screen *ebiten.Image, results []rendering.RaycastResult) {
	rayWidth := r.game.config.Graphics.RaysPerScreenWidth

	for columnIndex, rayResult := range results {
		screenX := columnIndex * rayWidth

		// Handle both single hits and multi-hits for transparency
		switch hitData := rayResult.TileType.(type) {
		case MultiRaycastHit:
			if len(hitData.Hits) == 0 {
				continue
			}
			// Render all hits from back to front for proper transparency
			for i := len(hitData.Hits) - 1; i >= 0; i-- {
				hit := hitData.Hits[i]

				// Update depth buffer with the solid objects
				if !hit.IsTransparent {
					for dx := 0; dx < rayWidth; dx++ {
						if screenX+dx < len(r.game.depthBuffer) {
							r.game.depthBuffer[screenX+dx] = hit.Distance
						}
					}
				}

				// Render this hit
				r.renderSingleHit(screen, screenX, hit, rayWidth)
			}
		case RaycastHit:
			// This case should ideally not be hit with the new system, but as a fallback:
			hitInfo := hitData

			// Update depth buffer for proper sprite occlusion
			for dx := 0; dx < rayWidth; dx++ {
				if screenX+dx < len(r.game.depthBuffer) {
					r.game.depthBuffer[screenX+dx] = rayResult.Distance
				}
			}

			// Render this hit
			r.renderSingleHit(screen, screenX, hitInfo, rayWidth)
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
			r.drawEnvironmentSpriteOnce(screen, screenX, hit.Distance, tileType)
		case "flooring_object":
			r.drawEnvironmentSprite(screen, screenX, hit.Distance, tileType)
		case "textured_wall":
			r.drawTexturedWallSlice(screen, screenX, hit.Distance, tileType, rayWidth,
				hit.WallSide, hit.TextureCoord)
		case "floor_only":
			// Floor-only tiles don't render anything here, just floor
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

	// Create an image to draw the floor on
	floorImage := ebiten.NewImage(screenWidth, screenHeight)
	pixels := make([]byte, screenWidth*screenHeight*4)

	// OPTIMIZATION: Skip every other row and column for 4x performance boost
	rowStep := 2
	colStep := 2

	// Draw floor and ceiling
	for y := horizon; y < screenHeight; y += rowStep {
		// Relative position of the floor pixel from the center of the screen
		// This determines the distance from the camera to the floor point
		p := float64(y - horizon)
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
	// Calculate tree height and position
	spriteHeight := int(float64(r.game.config.GetScreenHeight()) / distance * r.game.config.GetTileSize() * r.game.config.Graphics.Sprite.TreeHeightMultiplier)
	if spriteHeight > r.game.config.GetScreenHeight() {
		spriteHeight = r.game.config.GetScreenHeight()
	}
	if spriteHeight < 8 {
		spriteHeight = 8
	}

	spriteWidth := int(float64(spriteHeight) * r.game.config.Graphics.Sprite.TreeWidthMultiplier)
	spriteTop := (r.game.config.GetScreenHeight() - spriteHeight) / 2

	// Update depth buffer for the full width of the tree sprite to properly block monsters
	spriteLeft := x - spriteWidth/2
	spriteRight := x + spriteWidth/2
	for px := spriteLeft; px <= spriteRight && px >= 0 && px < len(r.game.depthBuffer); px++ {
		if distance < r.game.depthBuffer[px] {
			r.game.depthBuffer[px] = distance
		}
	}

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
	opts.ColorScale.Scale(float32(brightness), float32(brightness), float32(brightness), 1.0)

	// Use composite mode to ensure opaque rendering (no blending with background)
	opts.Blend = ebiten.BlendSourceOver

	screen.DrawImage(sprite, opts)
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

	// Update depth buffer for the sprite
	spriteLeft := x - spriteWidth/2
	spriteRight := x + spriteWidth/2
	for px := spriteLeft; px <= spriteRight && px >= 0 && px < len(r.game.depthBuffer); px++ {
		if distance < r.game.depthBuffer[px] {
			r.game.depthBuffer[px] = distance
		}
	}

	// Get appropriate sprite based on tile type using tile manager
	var spriteName string
	if world.GlobalTileManager != nil {
		spriteName = world.GlobalTileManager.GetSprite(tileType)
	}

	// Fallback to default sprite if not configured
	if spriteName == "" {
		spriteName = "grass"
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
	angle := r.game.camera.Angle - r.game.camera.FOV/2 + (float64(x)/float64(r.game.config.GetScreenWidth()))*r.game.camera.FOV

	// Calculate the world position where this ray hits
	rayDirX := math.Cos(angle)
	rayDirY := math.Sin(angle)
	hitWorldX := camX + rayDirX*distance
	hitWorldY := camY + rayDirY*distance

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

	// Create cache key that includes all factors that affect wall appearance
	cacheKey := rendering.WallSliceKey{
		Height:   wallHeight,
		Width:    width,
		TileType: tileType,
		Distance: distance,
		Side:     wallSide,
		WallX:    textureCoord,
	}

	// Get cached wall slice or create new one if not in cache
	wallSliceImage := r.game.threading.WallSliceCache.GetOrCreate(cacheKey, func() *ebiten.Image {
		return r.game.renderHelper.CreateTexturedWallSlice(tileType, distance, width, wallHeight, wallSide, textureCoord)
	})

	// Render the wall slice to the screen
	drawOptions := &ebiten.DrawImageOptions{}
	drawOptions.GeoM.Translate(float64(screenX), float64(wallTop))
	screen.DrawImage(wallSliceImage, drawOptions)
}

// MonsterRenderData holds data for rendering a monster sprite
type MonsterRenderData struct {
	monster    *monster.Monster3D
	screenX    int
	screenY    int
	spriteSize int
	distance   float64
	sprite     *ebiten.Image
}

// drawMonstersParallel draws all monsters using parallel sprite processing with depth testing
func (r *Renderer) drawMonstersParallel(screen *ebiten.Image) {
	var visibleMonsters []MonsterRenderData

	// Prepare sprite render data
	camX := r.game.camera.X
	camY := r.game.camera.Y
	viewDistSq := r.game.camera.ViewDist * r.game.camera.ViewDist // Pre-compute squared view distance

	for _, monster := range r.game.GetCurrentWorld().Monsters {
		if !monster.IsAlive() {
			continue
		}

		// Calculate distance - use squared distance for initial culling to avoid sqrt
		dx := monster.X - camX
		dy := monster.Y - camY
		distanceSq := dx*dx + dy*dy

		// Early cull monsters outside view distance without expensive sqrt
		if distanceSq > viewDistSq {
			continue
		}

		// Now calculate actual distance only for visible monsters
		distance := math.Sqrt(distanceSq)

		// Calculate sprite metrics using helper
		screenX, screenY, spriteSize, visible := r.game.renderHelper.CalculateSpriteMetrics(monster.X, monster.Y, distance)
		if !visible {
			continue
		}

		// Get sprite from monster's YAML config
		spriteName := monster.GetSpriteType()
		sprite := r.game.sprites.GetSprite(spriteName)

		visibleMonsters = append(visibleMonsters, MonsterRenderData{
			monster:    monster,
			screenX:    screenX,
			screenY:    screenY,
			spriteSize: spriteSize,
			distance:   distance,
			sprite:     sprite,
		})
	}

	// Sort monsters by distance (back to front for proper alpha blending)
	// Use Go's built-in sort for O(n log n) performance instead of O(n²) bubble sort
	sort.Slice(visibleMonsters, func(i, j int) bool {
		return visibleMonsters[i].distance > visibleMonsters[j].distance
	})

	// Render monsters in order, using depth testing
	for _, monsterData := range visibleMonsters {
		r.drawMonsterWithDepthTest(screen, monsterData)
	}
}

// drawMonsterWithDepthTest draws a monster sprite with depth buffer testing
func (r *Renderer) drawMonsterWithDepthTest(screen *ebiten.Image, monsterData MonsterRenderData) {
	// Check if monster should be visible based on depth buffer
	spriteLeft := monsterData.screenX - monsterData.spriteSize/2
	spriteRight := monsterData.screenX + monsterData.spriteSize/2

	// Clamp to screen bounds
	if spriteLeft < 0 {
		spriteLeft = 0
	}
	if spriteRight >= len(r.game.depthBuffer) {
		spriteRight = len(r.game.depthBuffer) - 1
	}

	// Check if any part of the sprite is in front of walls
	visible := false
	for x := spriteLeft; x <= spriteRight; x++ {
		if monsterData.distance < r.game.depthBuffer[x] {
			visible = true
			break
		}
	}

	if !visible {
		return // Monster is completely behind walls
	}

	// Draw collision box if enabled (draw first, so it's behind the sprite)
	if r.game.showCollisionBoxes {
		// Get collision box from collision system using monster ID
		var colW, colH int
		if r.game.collisionSystem != nil {
			if entity := r.game.collisionSystem.GetEntityByID(monsterData.monster.ID); entity != nil && entity.BoundingBox != nil {
				colW = int(entity.BoundingBox.Width)
				colH = int(entity.BoundingBox.Height)
			} else {
				colW, colH = 32, 32 // fallback
			}
		} else {
			colW, colH = 32, 32 // fallback
		}
		// Center the collision box within the sprite
		boxX := monsterData.screenX - colW/2
		boxY := monsterData.screenY + (monsterData.spriteSize-colH)/2
		boxColor := color.RGBA{255, 0, 0, 120} // Red, semi-transparent
		boxImg := ebiten.NewImage(colW, colH)
		boxImg.Fill(boxColor)
		boxOpts := &ebiten.DrawImageOptions{}
		boxOpts.GeoM.Translate(float64(boxX), float64(boxY))
		boxOpts.ColorM.Scale(1, 1, 1, 0.5)
		screen.DrawImage(boxImg, boxOpts)
	}

	// Draw the monster sprite
	opts := &ebiten.DrawImageOptions{}
	scaleX := float64(monsterData.spriteSize) / float64(monsterData.sprite.Bounds().Dx())
	scaleY := float64(monsterData.spriteSize) / float64(monsterData.sprite.Bounds().Dy())
	opts.GeoM.Scale(scaleX, scaleY)
	opts.GeoM.Translate(float64(spriteLeft), float64(monsterData.screenY))

	// Apply distance shading
	brightness := 1.0 - (monsterData.distance / r.game.camera.ViewDist)
	if brightness < r.game.config.Graphics.BrightnessMin {
		brightness = r.game.config.Graphics.BrightnessMin
	}
	opts.ColorScale.Scale(float32(brightness), float32(brightness), float32(brightness), 1.0)

	// Use opaque blending to prevent transparency issues
	opts.Blend = ebiten.BlendSourceOver

	screen.DrawImage(monsterData.sprite, opts)
}

// NPCRenderData contains data needed to render an NPC
type NPCRenderData struct {
	npc        *character.NPC
	screenX    int
	screenY    int
	spriteSize int
	distance   float64
	sprite     *ebiten.Image
}

// drawNPCs renders all visible NPCs as sprites with depth testing
func (r *Renderer) drawNPCs(screen *ebiten.Image) {
	var visibleNPCs []NPCRenderData

	// Prepare sprite render data
	camX := r.game.camera.X
	camY := r.game.camera.Y
	viewDistSq := r.game.camera.ViewDist * r.game.camera.ViewDist

	for _, npc := range r.game.GetCurrentWorld().NPCs {
		// Calculate distance - use squared distance for initial culling to avoid sqrt
		dx := npc.X - camX
		dy := npc.Y - camY
		distanceSq := dx*dx + dy*dy

		// Early cull NPCs outside view distance without expensive sqrt
		if distanceSq > viewDistSq {
			continue
		}

		// Now calculate actual distance only for visible NPCs
		distance := math.Sqrt(distanceSq)

		// Calculate sprite metrics using NPC-specific helper (larger than monsters)
		screenX, screenY, spriteSize, visible := r.game.renderHelper.CalculateNPCSpriteMetrics(npc.X, npc.Y, distance)
		if !visible {
			continue
		}

		// Use configured sprite for NPCs, fallback to "elf" if not specified
		spriteName := "elf" // default fallback
		if npc.Sprite != "" {
			// Remove .png extension if present to get sprite name
			spriteName = strings.TrimSuffix(npc.Sprite, ".png")
		}
		sprite := r.game.sprites.GetSprite(spriteName)

		visibleNPCs = append(visibleNPCs, NPCRenderData{
			npc:        npc,
			screenX:    screenX,
			screenY:    screenY,
			spriteSize: spriteSize,
			distance:   distance,
			sprite:     sprite,
		})
	}

	// Sort NPCs by distance (back to front for proper alpha blending)
	sort.Slice(visibleNPCs, func(i, j int) bool {
		return visibleNPCs[i].distance > visibleNPCs[j].distance
	})

	// Render NPCs in order, using depth testing
	for _, npcData := range visibleNPCs {
		r.drawNPCWithDepthTest(screen, npcData)
	}
}

// drawNPCWithDepthTest draws an NPC sprite with depth buffer testing
func (r *Renderer) drawNPCWithDepthTest(screen *ebiten.Image, npcData NPCRenderData) {
	// Check if NPC should be visible based on depth buffer
	spriteLeft := npcData.screenX - npcData.spriteSize/2
	spriteRight := npcData.screenX + npcData.spriteSize/2

	// Clamp to screen bounds
	if spriteLeft < 0 {
		spriteLeft = 0
	}
	if spriteRight >= len(r.game.depthBuffer) {
		spriteRight = len(r.game.depthBuffer) - 1
	}

	// Check if any part of the sprite is in front of walls
	visible := false
	for x := spriteLeft; x <= spriteRight; x++ {
		if npcData.distance < r.game.depthBuffer[x] {
			visible = true
			break
		}
	}

	if !visible {
		return // NPC is completely behind walls
	}

	// Draw the NPC sprite
	opts := &ebiten.DrawImageOptions{}
	scaleX := float64(npcData.spriteSize) / float64(npcData.sprite.Bounds().Dx())
	scaleY := float64(npcData.spriteSize) / float64(npcData.sprite.Bounds().Dy())
	opts.GeoM.Scale(scaleX, scaleY)
	opts.GeoM.Translate(float64(spriteLeft), float64(npcData.screenY))

	// Apply distance shading
	brightness := 1.0 - (npcData.distance / r.game.camera.ViewDist)
	if brightness < r.game.config.Graphics.BrightnessMin {
		brightness = r.game.config.Graphics.BrightnessMin
	}
	opts.ColorScale.Scale(float32(brightness), float32(brightness), float32(brightness), 1.0)

	// Use opaque blending to prevent transparency issues
	opts.Blend = ebiten.BlendSourceOver

	screen.DrawImage(npcData.sprite, opts)
}

// drawProjectiles draws magic projectiles, sword attacks, and arrows
func (r *Renderer) drawProjectiles(screen *ebiten.Image) {
	r.drawMagicProjectiles(screen)
	r.drawSwordAttacks(screen)
	r.drawArrows(screen)
}

// drawMagicProjectiles draws all active magic projectiles
func (r *Renderer) drawMagicProjectiles(screen *ebiten.Image) {
	for _, fireball := range r.game.fireballs {
		if !fireball.Active {
			continue
		}

		// Calculate fireball position relative to camera
		dx := fireball.X - r.game.camera.X
		dy := fireball.Y - r.game.camera.Y
		distance := math.Sqrt(dx*dx + dy*dy)

		if distance > r.game.camera.ViewDist || distance < 10 {
			continue
		}

		// Calculate angle to fireball
		fireballAngle := math.Atan2(dy, dx)
		angleDiff := fireballAngle - r.game.camera.Angle

		// Normalize angle difference
		for angleDiff > math.Pi {
			angleDiff -= 2 * math.Pi
		}
		for angleDiff < -math.Pi {
			angleDiff += 2 * math.Pi
		}

		// Check if fireball is in view
		if math.Abs(angleDiff) > r.game.camera.FOV/2 {
			continue
		}

		// Calculate screen position
		screenX := int(float64(r.game.config.GetScreenWidth())/2 + (angleDiff/(r.game.camera.FOV/2))*float64(r.game.config.GetScreenWidth()/2))

		// Get spell-specific graphics config based on spell type
		// The SpellType string is actually the SpellID (e.g., "firebolt", "fireball")
		spellConfigName := fireball.SpellType
		spellGraphicsConfig := r.game.config.GetSpellGraphicsConfig(spellConfigName)

		// Calculate projectile size based on distance using spell-specific config
		baseSize := float64(spellGraphicsConfig.BaseSize)
		projectileSize := int(baseSize / distance * r.game.config.GetTileSize())
		if projectileSize > spellGraphicsConfig.MaxSize {
			projectileSize = spellGraphicsConfig.MaxSize
		}
		if projectileSize < spellGraphicsConfig.MinSize {
			projectileSize = spellGraphicsConfig.MinSize
		}

		screenY := r.game.config.GetScreenHeight()/2 - projectileSize/2

		// Draw collision box if enabled (draw first, so it's behind the projectile)
		if r.game.showCollisionBoxes {
			// Use collision box from collision system if available
			var colW, colH int
			if r.game.collisionSystem != nil {
				// Try to find by position if no ID system for projectiles
				for _, entityID := range []string{"fireball", fireball.SpellType} {
					if entity := r.game.collisionSystem.GetEntityByID(entityID); entity != nil && math.Abs(entity.BoundingBox.X-fireball.X) < 1 && math.Abs(entity.BoundingBox.Y-fireball.Y) < 1 {
						colW = int(entity.BoundingBox.Width)
						colH = int(entity.BoundingBox.Height)
						break
					}
				}
				if colW == 0 || colH == 0 {
					// fallback: search all entities for matching position
					for _, entity := range r.game.collisionSystem.GetAllEntities() {
						if math.Abs(entity.BoundingBox.X-fireball.X) < 1 && math.Abs(entity.BoundingBox.Y-fireball.Y) < 1 {
							colW = int(entity.BoundingBox.Width)
							colH = int(entity.BoundingBox.Height)
							break
						}
					}
				}
				if colW == 0 || colH == 0 {
					colW, colH = projectileSize, projectileSize // fallback
				}
			} else {
				colW, colH = projectileSize, projectileSize // fallback
			}
			boxX := screenX - colW/2
			boxY := screenY + (projectileSize-colH)/2
			boxColor := color.RGBA{0, 255, 0, 120} // Green, semi-transparent
			boxImg := ebiten.NewImage(colW, colH)
			boxImg.Fill(boxColor)
			boxOpts := &ebiten.DrawImageOptions{}
			boxOpts.GeoM.Translate(float64(boxX), float64(boxY))
			boxOpts.ColorM.Scale(1, 1, 1, 0.5)
			screen.DrawImage(boxImg, boxOpts)
		}

		// Use spell-specific color from config (no more hardcoded colors!)
		projectileColor := spellGraphicsConfig.Color

		fireballImg := ebiten.NewImage(projectileSize, projectileSize)
		fireballImg.Fill(color.RGBA{uint8(projectileColor[0]), uint8(projectileColor[1]), uint8(projectileColor[2]), 255})

		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(float64(screenX-projectileSize/2), float64(screenY))
		screen.DrawImage(fireballImg, opts)
	}
}

// drawSwordAttacks draws all active sword attacks
func (r *Renderer) drawSwordAttacks(screen *ebiten.Image) {
	for _, attack := range r.game.swordAttacks {
		if !attack.Active {
			continue
		}

		// Calculate attack position relative to camera
		dx := attack.X - r.game.camera.X
		dy := attack.Y - r.game.camera.Y
		distance := math.Sqrt(dx*dx + dy*dy)

		if distance > r.game.camera.ViewDist || distance < 10 {
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

		// Get weapon-specific graphics config based on weapon name
		weaponType := items.GetWeaponTypeByName(attack.WeaponName)
		weaponDef := items.GetWeaponDefinition(weaponType)
		weaponGraphicsConfig := r.game.config.GetWeaponGraphicsConfig(weaponDef.Category)

		// Calculate attack size based on distance using weapon-specific config
		baseSize := float64(weaponGraphicsConfig.BaseSize)
		attackSize := int(baseSize / distance * r.game.config.GetTileSize())
		if attackSize > weaponGraphicsConfig.MaxSize {
			attackSize = weaponGraphicsConfig.MaxSize
		}
		if attackSize < weaponGraphicsConfig.MinSize {
			attackSize = weaponGraphicsConfig.MinSize
		}

		screenY := r.game.config.GetScreenHeight()/2 - attackSize/2

		// Draw collision box if enabled (draw first, so it's behind the attack)
		if r.game.showCollisionBoxes {
			// Use collision box from collision system if available
			var colW, colH int
			if r.game.collisionSystem != nil {
				// Try to find by position for melee attacks
				for _, entity := range r.game.collisionSystem.GetAllEntities() {
					if math.Abs(entity.BoundingBox.X-attack.X) < 1 && math.Abs(entity.BoundingBox.Y-attack.Y) < 1 {
						colW = int(entity.BoundingBox.Width)
						colH = int(entity.BoundingBox.Height)
						break
					}
				}
				if colW == 0 || colH == 0 {
					colW, colH = attackSize, attackSize // fallback
				}
			} else {
				colW, colH = attackSize, attackSize // fallback
			}
			boxX := screenX - colW/2
			boxY := screenY + (attackSize-colH)/2
			boxColor := color.RGBA{255, 255, 0, 120} // Yellow, semi-transparent
			boxImg := ebiten.NewImage(colW, colH)
			boxImg.Fill(boxColor)
			boxOpts := &ebiten.DrawImageOptions{}
			boxOpts.GeoM.Translate(float64(boxX), float64(boxY))
			boxOpts.ColorM.Scale(1, 1, 1, 0.5)
			screen.DrawImage(boxImg, boxOpts)
		}

		// Draw attack using weapon-specific color from config
		attackImg := ebiten.NewImage(attackSize, attackSize)
		attackColor := weaponGraphicsConfig.Color
		attackImg.Fill(color.RGBA{uint8(attackColor[0]), uint8(attackColor[1]), uint8(attackColor[2]), 255})

		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(float64(screenX-attackSize/2), float64(screenY))
		screen.DrawImage(attackImg, opts)
	}
}

// drawArrows draws all active arrows
func (r *Renderer) drawArrows(screen *ebiten.Image) {
	for _, arrow := range r.game.arrows {
		if !arrow.Active {
			continue
		}

		// Calculate arrow position relative to camera
		dx := arrow.X - r.game.camera.X
		dy := arrow.Y - r.game.camera.Y
		distance := math.Sqrt(dx*dx + dy*dy)

		if distance > r.game.camera.ViewDist || distance < 10 {
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

		// Calculate arrow size based on distance using bow-specific config
		bowGraphicsConfig := r.game.config.GetWeaponGraphicsConfig("bow")
		baseSize := float64(bowGraphicsConfig.BaseSize)
		arrowSize := int(baseSize / distance * r.game.config.GetTileSize())
		if arrowSize > bowGraphicsConfig.MaxSize {
			arrowSize = bowGraphicsConfig.MaxSize
		}
		if arrowSize < bowGraphicsConfig.MinSize {
			arrowSize = bowGraphicsConfig.MinSize
		}

		screenY := r.game.config.GetScreenHeight()/2 - arrowSize/2

		// Draw collision box if enabled (draw first, so it's behind the arrow)
		if r.game.showCollisionBoxes {
			// Use collision box from collision system if available
			var colW, colH int
			if r.game.collisionSystem != nil {
				// Try to find by position for arrows
				for _, entity := range r.game.collisionSystem.GetAllEntities() {
					if math.Abs(entity.BoundingBox.X-arrow.X) < 1 && math.Abs(entity.BoundingBox.Y-arrow.Y) < 1 {
						colW = int(entity.BoundingBox.Width)
						colH = int(entity.BoundingBox.Height)
						break
					}
				}
				if colW == 0 || colH == 0 {
					colW, colH = arrowSize, arrowSize // fallback
				}
			} else {
				colW, colH = arrowSize, arrowSize // fallback
			}
			boxX := screenX - colW/2
			boxY := screenY + (arrowSize-colH)/2
			boxColor := color.RGBA{0, 255, 255, 120} // Cyan, semi-transparent
			boxImg := ebiten.NewImage(colW, colH)
			boxImg.Fill(boxColor)
			boxOpts := &ebiten.DrawImageOptions{}
			boxOpts.GeoM.Translate(float64(boxX), float64(boxY))
			boxOpts.ColorM.Scale(1, 1, 1, 0.5)
			screen.DrawImage(boxImg, boxOpts)
		}

		// Draw arrow using bow-specific color from config
		arrowImg := ebiten.NewImage(arrowSize, arrowSize)
		arrowColor := bowGraphicsConfig.Color
		arrowImg.Fill(color.RGBA{uint8(arrowColor[0]), uint8(arrowColor[1]), uint8(arrowColor[2]), 255})

		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(float64(screenX-arrowSize/2), float64(screenY))
		screen.DrawImage(arrowImg, opts)
	}
}
