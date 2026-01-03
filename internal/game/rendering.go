package game

import (
	"fmt"
	"image/color"
	"math"
	"ugataima/internal/world"

	"ugataima/internal/threading/rendering"

	"github.com/hajimehoshi/ebiten/v2"
)

// Shared rendering utilities to eliminate code duplication

// RenderingHelper provides common rendering operations
type RenderingHelper struct {
	game         *MMGame
	textureCache map[string]*ebiten.Image // Cache for procedural textures
}

// NewRenderingHelper creates a new rendering helper
func NewRenderingHelper(game *MMGame) *RenderingHelper {
	return &RenderingHelper{
		game:         game,
		textureCache: make(map[string]*ebiten.Image),
	}
}

// CalculateWallDimensions calculates wall height and position for rendering
func (rh *RenderingHelper) CalculateWallDimensions(distance float64) (wallHeight, wallTop int) {
	return rh.CalculateWallDimensionsWithHeight(distance, 1.0)
}

// CalculateWallDimensionsWithHeight calculates wall dimensions with a height multiplier
func (rh *RenderingHelper) CalculateWallDimensionsWithHeight(distance, heightMultiplier float64) (wallHeight, wallTop int) {
	// Prevent division by zero or very small distances
	if distance < 0.1 {
		distance = 0.1
	}

	// Calculate base wall height on screen
	baseHeight := float64(rh.game.config.GetScreenHeight()) / distance * rh.game.config.GetTileSize()

	// Apply height multiplier
	wallHeight = int(baseHeight * heightMultiplier)

	if wallHeight > rh.game.config.GetScreenHeight()*2 {
		wallHeight = rh.game.config.GetScreenHeight() * 2 // Allow tall walls to extend off-screen
	}
	if wallHeight < 1 {
		wallHeight = 1
	}

	// Center the wall vertically (bottom aligned to horizon)
	wallTop = (rh.game.config.GetScreenHeight() - wallHeight) / 2

	return wallHeight, wallTop
}

// projectToScreenX converts a world position into screen X using the camera plane.
// It returns the screen X and camera-space depth (transformY).
func (rh *RenderingHelper) projectToScreenX(entityX, entityY float64) (screenX int, depth float64, ok bool) {
	cam := rh.game.camera
	dx := entityX - cam.X
	dy := entityY - cam.Y

	dirX := math.Cos(cam.Angle)
	dirY := math.Sin(cam.Angle)
	planeScale := math.Tan(cam.FOV / 2)
	planeX := -dirY * planeScale
	planeY := dirX * planeScale

	det := planeX*dirY - dirX*planeY
	if math.Abs(det) < 1e-9 {
		return 0, 0, false
	}
	invDet := 1.0 / det
	transformX := invDet * (dirY*dx - dirX*dy)
	transformY := invDet * (-planeY*dx + planeX*dy)
	if transformY <= 0 {
		return 0, 0, false
	}

	screenW := rh.game.config.GetScreenWidth()
	screenX = int(float64(screenW) / 2 * (1 + transformX/transformY))
	return screenX, transformY, true
}

// CreateWallSlice creates a cached wall slice image with proper shading
func (rh *RenderingHelper) CreateWallSlice(tileType world.TileType3D, distance float64, width, height int) *ebiten.Image {
	return rh.CreateWallSliceWithSide(tileType, distance, width, height, 0)
}

// CreateWallSliceWithSide creates a cached wall slice image with side-based shading and basic texturing
func (rh *RenderingHelper) CreateWallSliceWithSide(tileType world.TileType3D, distance float64, width, height, side int) *ebiten.Image {
	return rh.CreateTexturedWallSlice(tileType, distance, width, height, side, 0.0)
}

// CreateTexturedWallSlice creates a wall slice with texture mapping and proper shading.
// This combines distance-based lighting, side-based shading, and procedural texture patterns.
func (rh *RenderingHelper) CreateTexturedWallSlice(tileType world.TileType3D, distance float64, width, height, wallSide int, textureCoord float64) *ebiten.Image {
	return rh.CreateBaseTexturedWallSlice(tileType, width, height, wallSide, textureCoord)
}

// CreateBaseTexturedWallSlice creates a wall slice with base colors and textures but without distance-based shading.
// Distance-based shading should be applied at draw time for better cache efficiency.
func (rh *RenderingHelper) CreateBaseTexturedWallSlice(tileType world.TileType3D, width, height, wallSide int, textureCoord float64) *ebiten.Image {
	// Get the base color for this tile type
	baseColor := rh.GetTileColor(tileType)

	// Apply side-based shading for 3D depth perception
	// East-west walls appear darker than north-south walls (classic raycasting technique)
	shadingMultiplier := 1.0
	if wallSide == 1 {
		shadingMultiplier = 0.7 // East-west walls are darker
	}

	// Apply only side-based shading (distance shading will be applied at draw time)
	finalColor := color.RGBA{
		R: uint8(float64(baseColor.R) * shadingMultiplier),
		G: uint8(float64(baseColor.G) * shadingMultiplier),
		B: uint8(float64(baseColor.B) * shadingMultiplier),
		A: baseColor.A,
	}

	// Create the wall slice image
	wallImage := ebiten.NewImage(width, height)

	// First, try to use actual sprite texture if available for ANY tile type
	var spriteName string
	if world.GlobalTileManager != nil {
		spriteName = world.GlobalTileManager.GetSprite(tileType)
	}

	if spriteName != "" {
		// Use actual sprite texture - extract vertical slice
		sprite := rh.game.sprites.GetSprite(spriteName)
		if sprite != nil {
			rh.applyWallSliceFromSprite(wallImage, sprite, finalColor, width, height, textureCoord)
			return wallImage
		}
	}

	// Fallback to procedural texture patterns when no sprite available
	// Check if this is a textured wall type that needs special procedural patterns
	if world.GlobalTileManager != nil {
		renderType := world.GlobalTileManager.GetRenderType(tileType)
		if renderType == "textured_wall" {
			// Use appropriate procedural texture based on tile type
			switch tileType {
			case world.TileThicket:
				rh.applyFoliageTextureCached(wallImage, finalColor, width, height)
			default:
				// Default to brick texture for all other textured walls
				rh.applyBrickTextureCached(wallImage, finalColor, width, height)
			}
			return wallImage
		}
	}

	// Legacy fallback for specific tile types (when GlobalTileManager is unavailable)
	switch tileType {
	case world.TileWall, world.TileLowWall, world.TileHighWall:
		rh.applyBrickTextureCached(wallImage, finalColor, width, height)
	case world.TileThicket:
		rh.applyFoliageTextureCached(wallImage, finalColor, width, height)
	default:
		// Solid color fill for tiles without specific textures
		wallImage.Fill(finalColor)
	}

	return wallImage
}

// applyBrickTextureCached applies a cached brick pattern texture to the wall image.
// This pre-renders the brick texture pattern once and reuses it for better performance.
func (rh *RenderingHelper) applyBrickTextureCached(wallImage *ebiten.Image, finalColor color.RGBA, width, height int) {
	// Create cache key based on size - pattern is deterministic
	cacheKey := fmt.Sprintf("brick_%dx%d", width, height)

	// Check if we have this texture cached
	if cachedTexture, exists := rh.textureCache[cacheKey]; exists {
		// Apply the cached texture with color tinting
		opts := &ebiten.DrawImageOptions{}

		// Calculate color scaling for shading
		opts.ColorScale.Scale(
			float32(finalColor.R)/255.0,
			float32(finalColor.G)/255.0,
			float32(finalColor.B)/255.0,
			1.0)

		wallImage.DrawImage(cachedTexture, opts)
		return
	}

	// Create the base texture pattern (white base for color tinting)
	baseTexture := ebiten.NewImage(width, height)
	baseTexture.Fill(color.RGBA{255, 255, 255, 255}) // White base

	// Create mortar lines on a separate layer
	mortarTexture := ebiten.NewImage(width, height)
	mortarColor := color.RGBA{179, 179, 179, 255} // Gray mortar (70% of white)

	// Create mortar line image
	mortarLine := ebiten.NewImage(width, 1)
	mortarLine.Fill(mortarColor)

	// Add horizontal mortar lines every 8 pixels
	opts := &ebiten.DrawImageOptions{}
	for y := 8; y < height; y += 8 {
		opts.GeoM.Reset()
		opts.GeoM.Translate(0, float64(y))
		mortarTexture.DrawImage(mortarLine, opts)
	}

	// Combine base and mortar lines
	baseTexture.DrawImage(mortarTexture, nil)

	// Cache the texture for reuse
	rh.textureCache[cacheKey] = baseTexture

	// Apply with color tinting
	finalOpts := &ebiten.DrawImageOptions{}
	finalOpts.ColorScale.Scale(
		float32(finalColor.R)/255.0,
		float32(finalColor.G)/255.0,
		float32(finalColor.B)/255.0,
		1.0)

	wallImage.DrawImage(baseTexture, finalOpts)
}

// applyFoliageTextureCached applies a cached foliage pattern texture.
// This pre-renders the texture pattern once and reuses it for better performance.
func (rh *RenderingHelper) applyFoliageTextureCached(wallImage *ebiten.Image, finalColor color.RGBA, width, height int) {
	// Create cache key based on size - pattern is deterministic
	cacheKey := fmt.Sprintf("foliage_%dx%d", width, height)

	// Check if we have this texture cached
	if cachedTexture, exists := rh.textureCache[cacheKey]; exists {
		// Apply the cached texture with color tinting
		opts := &ebiten.DrawImageOptions{}

		// Calculate color scaling for shading
		opts.ColorScale.Scale(
			float32(finalColor.R)/255.0,
			float32(finalColor.G)/255.0,
			float32(finalColor.B)/255.0,
			1.0)

		wallImage.DrawImage(cachedTexture, opts)
		return
	}

	// Create the base texture pattern (white on transparent)
	baseTexture := ebiten.NewImage(width, height)
	baseTexture.Fill(color.RGBA{255, 255, 255, 255}) // White base

	// Create shadow spots on a separate layer
	shadowTexture := ebiten.NewImage(width, height)
	shadowColor := color.RGBA{153, 153, 153, 255} // Gray shadow (60% of white)

	// Add pseudo-random shadow spots
	for y := 0; y < height; y += 3 {
		for x := 0; x < width; x += 4 {
			if (x+y)%5 < 2 {
				shadowTexture.Set(x, y, shadowColor)
			}
		}
	}

	// Combine base and shadows
	baseTexture.DrawImage(shadowTexture, nil)

	// Cache the texture for reuse
	rh.textureCache[cacheKey] = baseTexture

	// Apply with color tinting
	opts := &ebiten.DrawImageOptions{}
	opts.ColorScale.Scale(
		float32(finalColor.R)/255.0,
		float32(finalColor.G)/255.0,
		float32(finalColor.B)/255.0,
		1.0)

	wallImage.DrawImage(baseTexture, opts)
}

// applyWallSliceFromSprite extracts a vertical slice from a sprite texture for wall rendering.
// This uses caching to avoid repeated sprite slice extraction and scaling operations.
func (rh *RenderingHelper) applyWallSliceFromSprite(wallImage *ebiten.Image, sprite *ebiten.Image, finalColor color.RGBA, width, height int, textureCoord float64) {
	spriteWidth := sprite.Bounds().Dx()
	spriteHeight := sprite.Bounds().Dy()

	// Calculate which column of the sprite to use based on textureCoord
	textureX := int(textureCoord * float64(spriteWidth))
	if textureX >= spriteWidth {
		textureX = spriteWidth - 1
	}
	if textureX < 0 {
		textureX = 0
	}

	// Create cache key including sprite dimensions, texture position, and target size
	cacheKey := fmt.Sprintf("sprite_slice_%dx%d_x%d_%dx%d", spriteWidth, spriteHeight, textureX, width, height)

	// Check if we have this sprite slice cached
	if cachedSlice, exists := rh.textureCache[cacheKey]; exists {
		// Apply the cached slice with color tinting
		opts := &ebiten.DrawImageOptions{}

		// Apply color tinting for distance/side shading
		shadingFactor := float64(finalColor.R) / 255.0 // Use red component as reference
		opts.ColorScale.Scale(
			float32(shadingFactor),
			float32(shadingFactor),
			float32(shadingFactor),
			1.0)

		wallImage.DrawImage(cachedSlice, opts)
		return
	}

	// Create a 1-pixel wide slice from the sprite
	sliceImage := ebiten.NewImage(1, spriteHeight)

	// Use DrawImage with source rectangle to extract the slice - much faster than pixel operations
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(float64(-textureX), 0) // Offset to show only the desired column

	// Create a temporary 1-pixel wide image to act as a mask
	sliceImage.DrawImage(sprite, opts)

	// Create the final scaled slice for caching
	scaledSlice := ebiten.NewImage(width, height)
	drawOpts := &ebiten.DrawImageOptions{}

	// Scale the slice to fit the wall dimensions
	scaleX := float64(width)
	scaleY := float64(height) / float64(spriteHeight)
	drawOpts.GeoM.Scale(scaleX, scaleY)

	// Draw the scaled slice (white/uncolored for caching)
	scaledSlice.DrawImage(sliceImage, drawOpts)

	// Cache the scaled slice for reuse
	rh.textureCache[cacheKey] = scaledSlice

	// Apply with color tinting
	finalOpts := &ebiten.DrawImageOptions{}
	shadingFactor := float64(finalColor.R) / 255.0 // Use red component as reference
	finalOpts.ColorScale.Scale(
		float32(shadingFactor),
		float32(shadingFactor),
		float32(shadingFactor),
		1.0)

	wallImage.DrawImage(scaledSlice, finalOpts)
}

// GetTileColor returns the base color for a tile type (reads from tile configuration)
func (rh *RenderingHelper) GetTileColor(tileType world.TileType3D) color.RGBA {
	// Try to get color from tile configuration first
	if world.GlobalTileManager != nil {
		wallColor := world.GlobalTileManager.GetWallColor(tileType)
		return color.RGBA{
			R: uint8(wallColor[0]),
			G: uint8(wallColor[1]),
			B: uint8(wallColor[2]),
			A: 255,
		}
	}

	// Legacy fallback for when GlobalTileManager is unavailable
	switch tileType {
	case world.TileTree:
		return color.RGBA{101, 67, 33, 255} // Brown tree
	case world.TileAncientTree:
		return color.RGBA{69, 39, 19, 255} // Darker brown
	case world.TileWall:
		return color.RGBA{64, 64, 64, 255} // Gray stone wall
	case world.TileThicket:
		return color.RGBA{34, 80, 34, 255} // Dark green thicket
	case world.TileMossRock:
		return color.RGBA{105, 105, 105, 255} // Gray rock
	case world.TileMushroomRing:
		return color.RGBA{128, 64, 128, 255} // Purple mushrooms
	case world.TileFernPatch:
		return color.RGBA{50, 120, 50, 255} // Green ferns
	case world.TileForestStream:
		return color.RGBA{64, 128, 255, 255} // Blue water
	case world.TileFireflySwarm:
		return color.RGBA{255, 255, 150, 255} // Yellow fireflies
	case world.TileClearing:
		return color.RGBA{80, 150, 80, 255} // Light green grass
	case world.TileWater:
		return color.RGBA{30, 100, 200, 255} // Deep blue water
	case world.TileLowWall:
		return color.RGBA{120, 120, 120, 255} // Light gray low wall
	case world.TileHighWall:
		return color.RGBA{40, 40, 40, 255} // Dark gray high wall
	default:
		return color.RGBA{101, 67, 33, 255} // Default brown
	}
}

// CalculateMonsterSpriteMetrics calculates sprite position and size for 3D rendering with monster-specific size multiplier
func (rh *RenderingHelper) CalculateMonsterSpriteMetrics(entityX, entityY, distance, sizeGameMultiplier float64) (screenX, screenY, spriteSize int, visible bool) {
	maxSize := int(float64(rh.game.config.Graphics.Monster.MaxSpriteSize) * sizeGameMultiplier)
	minSize := int(float64(rh.game.config.Graphics.Monster.MinSpriteSize) * sizeGameMultiplier)
	// Apply size multiplier to distance calculation as well
	effectiveMultiplier := int(float64(rh.game.config.Graphics.Monster.SizeDistanceMultiplier) * sizeGameMultiplier)
	screenX, screenY, spriteSize, visible = rh.calculateSpriteMetricsWithConfig(entityX, entityY, distance, maxSize, minSize, effectiveMultiplier)

	// Adjust Y position to place monsters on the ground (bottom edge at horizon line)
	if visible {
		screenY = rh.game.config.GetScreenHeight() / 2
	}

	return screenX, screenY, spriteSize, visible
}

// CalculateNPCSpriteMetrics calculates sprite position and size for NPCs (larger than monsters)
func (rh *RenderingHelper) CalculateNPCSpriteMetrics(entityX, entityY, distance, sizeMultiplier float64) (screenX, screenY, spriteSize int, visible bool) {
	if sizeMultiplier <= 0 {
		sizeMultiplier = 1
	}
	maxSize := int(float64(rh.game.config.Graphics.NPC.MaxSpriteSize) * sizeMultiplier)
	minSize := int(float64(rh.game.config.Graphics.NPC.MinSpriteSize) * sizeMultiplier)
	effectiveMultiplier := int(float64(rh.game.config.Graphics.NPC.SizeDistanceMultiplier) * sizeMultiplier)
	screenX, screenY, spriteSize, visible = rh.calculateSpriteMetricsWithConfig(entityX, entityY, distance, maxSize, minSize, effectiveMultiplier)

	// Adjust Y position to place NPCs on the ground (bottom edge at horizon line)
	if visible {
		screenY = rh.game.config.GetScreenHeight() / 2
	}

	return screenX, screenY, spriteSize, visible
}

// CalculateEnvironmentSpriteMetrics calculates sprite position and size for environment sprites (similar to trees)
func (rh *RenderingHelper) CalculateEnvironmentSpriteMetrics(entityX, entityY, distance float64) (screenX, screenY, spriteSize int, visible bool) {
	// Use tree sprite configuration for environment sprites
	spriteHeight := int(float64(rh.game.config.GetScreenHeight()) / distance * rh.game.config.GetTileSize() * rh.game.config.Graphics.Sprite.TreeHeightMultiplier)
	if spriteHeight > rh.game.config.GetScreenHeight() {
		spriteHeight = rh.game.config.GetScreenHeight()
	}
	if spriteHeight < 8 {
		spriteHeight = 8
	}

	// spriteWidth := int(float64(spriteHeight) * rh.game.config.Graphics.Sprite.TreeWidthMultiplier)

	// Check if within view distance
	if distance > rh.game.camera.ViewDist || distance < 5.0 {
		return 0, 0, 0, false
	}

	screenX, _, ok := rh.projectToScreenX(entityX, entityY)
	if !ok {
		return 0, 0, 0, false
	}

	screenW := rh.game.config.GetScreenWidth()
	if screenX < -spriteHeight || screenX > screenW+spriteHeight {
		return 0, 0, 0, false
	}

	// Environment sprites are centered at horizon line (like trees)
	screenY = (rh.game.config.GetScreenHeight() - spriteHeight) / 2

	return screenX, screenY, spriteHeight, true
}

// calculateSpriteMetricsWithConfig is the shared implementation
func (rh *RenderingHelper) calculateSpriteMetricsWithConfig(entityX, entityY, distance float64, maxSize, minSize, multiplier int) (screenX, screenY, spriteSize int, visible bool) {
	// Check if within view distance
	// In turn-based mode, monsters can be very close (adjacent tiles), so allow closer distances
	minDistance := 5.0
	if rh.game.turnBasedMode {
		minDistance = 1.0 // Allow monsters to be rendered even when very close in turn-based mode
	}
	if distance > rh.game.camera.ViewDist || distance < minDistance {
		return 0, 0, 0, false
	}

	// Calculate sprite size based on distance
	spriteSize = int(rh.game.config.GetTileSize() / distance * float64(multiplier))
	if spriteSize > maxSize {
		spriteSize = maxSize
	}
	if spriteSize < minSize {
		spriteSize = minSize
	}

	screenX, _, ok := rh.projectToScreenX(entityX, entityY)
	if !ok {
		return 0, 0, 0, false
	}

	screenW := rh.game.config.GetScreenWidth()
	if screenX < -spriteSize || screenX > screenW+spriteSize {
		return 0, 0, 0, false
	}

	screenY = rh.game.config.GetScreenHeight()/2 - spriteSize/2

	return screenX, screenY, spriteSize, true
}

// CreateSpriteRenderJob creates a sprite render job for the parallel renderer
func (rh *RenderingHelper) CreateSpriteRenderJob(sprite *ebiten.Image, screenX, screenY, spriteSize int) *rendering.SpriteRenderJob {
	return &rendering.SpriteRenderJob{
		Image:      sprite,
		X:          screenX - spriteSize/2,
		Y:          screenY,
		ScaleX:     float64(spriteSize) / float64(sprite.Bounds().Dx()),
		ScaleY:     float64(spriteSize) / float64(sprite.Bounds().Dy()),
		ColorScale: struct{ R, G, B, A float32 }{1.0, 1.0, 1.0, 1.0},
	}
}

// RenderBackgroundLayers renders sky and ground layers
func (rh *RenderingHelper) RenderBackgroundLayers(screen *ebiten.Image) {
	// Draw cached sky
	skyOpts := &ebiten.DrawImageOptions{}
	screen.DrawImage(rh.game.skyImg, skyOpts)

	// Draw cached ground
	groundOpts := &ebiten.DrawImageOptions{}
	groundOpts.GeoM.Translate(0, float64(rh.game.config.GetScreenHeight()/2))
	screen.DrawImage(rh.game.groundImg, groundOpts)
}
