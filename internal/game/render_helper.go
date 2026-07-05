package game

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"ugataima/internal/world"

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

// CalculateWallDimensionsWithHeight calculates wall dimensions with a height multiplier
func (rh *RenderingHelper) CalculateWallDimensionsWithHeight(distance, heightMultiplier float64) (wallHeight, wallTop int) {
	// Division guard only - collision keeps the camera farther away than this.
	// The wall's vanish-at-point-blank bug came from CAPPING the height while
	// the floor anchor kept growing (the capped top sank below the screen);
	// with the height uncapped the projection stays correct at any range, and
	// any distance clamp larger than an epsilon would flatten near columns
	// into a visible crease against the still-perspective far ones.
	if distance < 1.0 {
		distance = 1.0
	}

	// Calculate base wall height on screen
	baseHeight := float64(rh.game.config.GetScreenHeight()) / distance * rh.game.config.GetTileSize()

	// Apply height multiplier
	wallHeight = int(baseHeight * heightMultiplier)

	// Sanity bound, reachable only inside the 1-unit epsilon above: GPU clips
	// off-screen geometry, so huge-but-finite heights cost nothing.
	if maxH := rh.game.config.GetScreenHeight() * 64; wallHeight > maxH {
		wallHeight = maxH
	}
	if wallHeight < 1 {
		wallHeight = 1
	}

	// Anchor wall bottom to the floor line at this distance for consistency
	// with floor and sprite projection.
	floorScreenY := rh.calculateFloorScreenY(distance)
	wallTop = floorScreenY - wallHeight

	return wallHeight, wallTop
}

// calculateFloorScreenY calculates the screen Y position where the floor appears
// at a given perpendicular distance from the camera.
//
// This is the inverse of the floor rendering formula used in drawSimpleFloorCeiling:
//
//	rowDistance = (0.5 * screenHeight * tileSize) / p
//
// Where:
//   - rowDistance is the perpendicular distance from camera to floor point
//   - p is the vertical offset from the horizon line (screen pixels)
//   - screenHeight/2 is the horizon line position
//
// Solving for screen Y:
//
//	p = (0.5 * screenHeight * tileSize) / rowDistance
//	screenY = horizon + p
//
// This ensures sprites are anchored to the floor at their correct distance,
// preventing the "drift" effect where sprites appeared to slide toward the
// camera when viewed from medium distances (4+ tiles).
func (rh *RenderingHelper) calculateFloorScreenY(perpDist float64) int {
	screenHeight := float64(rh.game.config.GetScreenHeight())
	tileSize := rh.game.config.GetTileSize()
	horizon := screenHeight / 2

	if perpDist <= 0 {
		perpDist = 1 // Avoid division by zero
	}
	p := (0.5 * screenHeight * tileSize) / perpDist
	return int(horizon + p)
}

// projectToScreenX converts a world position into screen X using the camera plane.
// It returns the screen X position and the perpendicular distance (depth) to the entity.
//
// This uses the standard raycasting sprite projection technique:
// 1. Transform world-space offset (dx, dy) into camera-space using matrix inversion
// 2. transformY is the perpendicular distance (depth into screen)
// 3. transformX is the horizontal offset in camera space
// 4. Screen X = center + (transformX / transformY) * halfWidth
//
// The perpendicular distance (transformY) is critical for:
// - Sprite sizing: size = tileSize / perpDist (not Euclidean distance)
// - Floor anchoring: sprites bottom aligned with floor at their perpDist
// - Depth buffer: comparing depths for occlusion
//
// Using perpendicular distance instead of Euclidean distance prevents:
// - Fisheye distortion at screen edges
// - Sprite drift when viewed at angles
//
// Reference: https://lodev.org/cgtutor/raycasting3.html
func (rh *RenderingHelper) projectToScreenX(entityX, entityY float64) (screenX int, depth float64, ok bool) {
	cam := rh.game.camera
	dx := entityX - cam.X
	dy := entityY - cam.Y

	// Camera direction vector
	dirX := math.Cos(cam.Angle)
	dirY := math.Sin(cam.Angle)

	// Camera plane vector (perpendicular to direction, scaled by FOV)
	planeScale := math.Tan(cam.FOV / 2)
	planeX := -dirY * planeScale
	planeY := dirX * planeScale

	// Invert the camera matrix to transform world coords to camera space
	// | planeX  dirX |   | transformX |   | dx |
	// | planeY  dirY | * | transformY | = | dy |
	det := planeX*dirY - dirX*planeY
	if math.Abs(det) < 1e-9 {
		return 0, 0, false // Degenerate matrix
	}
	invDet := 1.0 / det
	transformX := invDet * (dirY*dx - dirX*dy)      // Horizontal offset in camera space
	transformY := invDet * (-planeY*dx + planeX*dy) // Perpendicular distance (depth)

	if transformY <= 0 {
		return 0, 0, false // Behind camera
	}

	screenW := rh.game.config.GetScreenWidth()
	screenX = int(float64(screenW) / 2 * (1 + transformX/transformY))
	return screenX, transformY, true
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

	wallImage.Fill(finalColor)

	return wallImage
}

// tintOptions returns draw options that scale a white-cached texture by the
// given per-channel factors (alpha untouched).
func tintOptions(r, g, b float32) *ebiten.DrawImageOptions {
	opts := &ebiten.DrawImageOptions{}
	opts.ColorScale.Scale(r, g, b, 1.0)
	return opts
}

// applyPatternTextureCached draws a procedural pattern onto the wall image
// with color tinting. The white-base pattern is painted once per size (the
// patterns are deterministic), cached under keyPrefix, and reused.
func (rh *RenderingHelper) applyPatternTextureCached(wallImage *ebiten.Image, finalColor color.RGBA, width, height int, keyPrefix string, paint func(base *ebiten.Image)) {
	cacheKey := fmt.Sprintf("%s_%dx%d", keyPrefix, width, height)
	tint := tintOptions(
		float32(finalColor.R)/255.0,
		float32(finalColor.G)/255.0,
		float32(finalColor.B)/255.0)

	if cachedTexture, exists := rh.textureCache[cacheKey]; exists {
		wallImage.DrawImage(cachedTexture, tint)
		return
	}

	// White base so the cached pattern can be tinted to any wall color.
	baseTexture := ebiten.NewImage(width, height)
	baseTexture.Fill(color.RGBA{255, 255, 255, 255})
	paint(baseTexture)

	rh.textureCache[cacheKey] = baseTexture
	wallImage.DrawImage(baseTexture, tint)
}

// applyBrickTextureCached applies a cached brick pattern texture to the wall image.
func (rh *RenderingHelper) applyBrickTextureCached(wallImage *ebiten.Image, finalColor color.RGBA, width, height int) {
	rh.applyPatternTextureCached(wallImage, finalColor, width, height, "brick", func(base *ebiten.Image) {
		// Horizontal mortar lines every 8 pixels on a separate layer
		mortarTexture := ebiten.NewImage(width, height)
		mortarColor := color.RGBA{179, 179, 179, 255} // Gray mortar (70% of white)
		mortarLine := ebiten.NewImage(width, 1)
		mortarLine.Fill(mortarColor)
		opts := &ebiten.DrawImageOptions{}
		for y := 8; y < height; y += 8 {
			opts.GeoM.Reset()
			opts.GeoM.Translate(0, float64(y))
			mortarTexture.DrawImage(mortarLine, opts)
		}
		base.DrawImage(mortarTexture, nil)
	})
}

// applyFoliageTextureCached applies a cached foliage pattern texture.
func (rh *RenderingHelper) applyFoliageTextureCached(wallImage *ebiten.Image, finalColor color.RGBA, width, height int) {
	rh.applyPatternTextureCached(wallImage, finalColor, width, height, "foliage", func(base *ebiten.Image) {
		// Pseudo-random shadow spots on a separate layer
		shadowTexture := ebiten.NewImage(width, height)
		shadowColor := color.RGBA{153, 153, 153, 255} // Gray shadow (60% of white)
		for y := 0; y < height; y += 3 {
			for x := 0; x < width; x += 4 {
				if (x+y)%5 < 2 {
					shadowTexture.Set(x, y, shadowColor)
				}
			}
		}
		base.DrawImage(shadowTexture, nil)
	})
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

	sourceWidth := width
	if sourceWidth < 1 {
		sourceWidth = 1
	}
	if sourceWidth > spriteWidth {
		sourceWidth = spriteWidth
	}

	// Create cache key including sprite dimensions, texture position, sampled strip width, and target size.
	cacheKey := fmt.Sprintf("sprite_slice_%dx%d_x%d_sw%d_%dx%d", spriteWidth, spriteHeight, textureX, sourceWidth, width, height)

	// Grayscale distance/side shading, red component as reference
	shading := float32(finalColor.R) / 255.0

	// Check if we have this sprite slice cached
	if cachedSlice, exists := rh.textureCache[cacheKey]; exists {
		wallImage.DrawImage(cachedSlice, tintOptions(shading, shading, shading))
		return
	}

	// Create a narrow strip from the sprite. This keeps real wall textures readable
	// for ray widths > 1 and wraps at texture edges for seamless textures.
	sliceImage := ebiten.NewImage(sourceWidth, spriteHeight)
	drawSourceStrip := func(srcX, dstX, stripWidth int) {
		if stripWidth <= 0 {
			return
		}
		src := sprite.SubImage(image.Rect(srcX, 0, srcX+stripWidth, spriteHeight)).(*ebiten.Image)
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(float64(dstX), 0)
		sliceImage.DrawImage(src, opts)
	}
	if textureX+sourceWidth <= spriteWidth {
		drawSourceStrip(textureX, 0, sourceWidth)
	} else {
		firstWidth := spriteWidth - textureX
		drawSourceStrip(textureX, 0, firstWidth)
		drawSourceStrip(0, firstWidth, sourceWidth-firstWidth)
	}

	// Create the final scaled slice for caching
	scaledSlice := ebiten.NewImage(width, height)
	drawOpts := &ebiten.DrawImageOptions{}

	// Scale the slice to fit the wall dimensions
	scaleX := float64(width) / float64(sourceWidth)
	scaleY := float64(height) / float64(spriteHeight)
	drawOpts.GeoM.Scale(scaleX, scaleY)

	// Draw the scaled slice (white/uncolored for caching)
	scaledSlice.DrawImage(sliceImage, drawOpts)

	// Cache the scaled slice for reuse
	rh.textureCache[cacheKey] = scaledSlice

	wallImage.DrawImage(scaledSlice, tintOptions(shading, shading, shading))
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

	return color.RGBA{101, 67, 33, 255}
}

// CalculateMonsterSpriteMetrics calculates sprite position and size for 3D rendering with monster-specific size multiplier
func (rh *RenderingHelper) CalculateMonsterSpriteMetrics(entityX, entityY, distance, sizeGameMultiplier float64) (screenX, screenY, spriteSize int, visible bool) {
	// Match environment sprite scaling (moss rocks, trees) using the same formula and caps.
	distanceMultiplier := float64(rh.game.config.Graphics.Monster.SizeDistanceMultiplier) * sizeGameMultiplier
	heightMultiplier := distanceMultiplier / float64(rh.game.config.GetScreenHeight())
	screenX, screenY, spriteSize, visible = rh.calculateScreenCappedSpriteMetrics(entityX, entityY, distance, heightMultiplier)

	// screenY is now correctly calculated by calculateScreenCappedSpriteMetrics to anchor
	// the sprite's bottom to the floor at its distance

	return screenX, screenY, spriteSize, visible
}

// CalculateNPCSpriteMetrics calculates sprite position and size for NPCs (larger than monsters).
// People-sized NPCs use this; buildings should set render_type: environment_sprite in YAML
// so they go through CalculateEnvironmentSpriteMetrics (same path as the shipwreck) instead.
//
// sizeMultiplier scales maxSize (how big a "size 4" NPC gets up close) and the
// distance multiplier (so it grows proportionally), but NOT minSize - the floor
// stays absolute so far-away NPCs recede to the same pixel size as small ones,
// instead of clamping at a sizeMultiplierxbigger floor and appearing oversized
// relative to the receding background.
func (rh *RenderingHelper) CalculateNPCSpriteMetrics(entityX, entityY, distance, sizeMultiplier float64) (screenX, screenY, spriteSize int, visible bool) {
	if sizeMultiplier <= 0 {
		sizeMultiplier = 1
	}
	minSize := rh.game.config.Graphics.NPC.MinSpriteSize
	effectiveMultiplier := int(float64(rh.game.config.Graphics.NPC.SizeDistanceMultiplier) * sizeMultiplier)
	return rh.calculateBoundedSpriteMetrics(entityX, entityY, distance, minSize, effectiveMultiplier)
}

// CalculateEnvironmentSpriteMetrics calculates sprite position and size for environment sprites (similar to trees)
func (rh *RenderingHelper) CalculateEnvironmentSpriteMetrics(entityX, entityY, distance float64, tileType world.TileType3D, sizeScale float64) (screenX, screenY, spriteSize int, visible bool) {
	if sizeScale <= 0 {
		sizeScale = 1
	}
	// Get visual size multiplier from tile definition (trees = 2.0, ferns = 1.0, etc.)
	heightMultiplier := rh.game.config.Graphics.Sprite.TreeHeightMultiplier
	if world.GlobalTileManager != nil {
		heightMultiplier = world.GlobalTileManager.GetSizeMultiplier(tileType)
	}
	heightMultiplier *= sizeScale

	// Fixed 5.0 near cull: environment sprites keep it even in turn-based mode
	// (only monsters/NPCs get the close-range exemption).
	return rh.projectSpriteMetrics(entityX, entityY, distance, 5.0, heightMultiplier, 8)
}

// projectSpriteMetrics is the shared projection core for floor-anchored
// billboard sprites: view-distance/near culling, camera-plane projection,
// height-multiplier sizing with the numeric sanity cap, horizontal culling,
// and floor anchoring. Callers differ only in the near-cull distance, the
// minimum sprite size, and where heightMultiplier comes from.
//
// Math notes:
//   - Culling uses the Euclidean distance parameter; sizing uses PERPENDICULAR
//     distance from projectToScreenX - Euclidean sizing would create fisheye
//     distortion at screen edges
//   - The size cap is a numeric sanity bound only: any screen-pixel cap
//     reachable at playable range makes the sprite SINK as the camera closes
//     in - the floor anchor keeps growing ~1/d while the capped size stops,
//     dragging the top below the viewport. The GPU clips oversize sprites.
//   - Screen Y anchors the sprite's BOTTOM edge to the floor at its perpDist,
//     so sprites appear grounded rather than floating
func (rh *RenderingHelper) projectSpriteMetrics(entityX, entityY, distance, minDistance, heightMultiplier float64, minSize int) (screenX, screenY, spriteSize int, visible bool) {
	if distance > rh.game.camera.ViewDist || distance < minDistance {
		return 0, 0, 0, false
	}

	screenX, perpDist, ok := rh.projectToScreenX(entityX, entityY)
	if !ok {
		return 0, 0, 0, false
	}

	spriteSize = rh.calculateSpriteSizeWithHeightMultiplier(perpDist, heightMultiplier)
	if maxS := rh.game.config.GetScreenHeight() * 64; spriteSize > maxS {
		spriteSize = maxS
	}
	if spriteSize < minSize {
		spriteSize = minSize
	}

	screenW := rh.game.config.GetScreenWidth()
	if screenX < -spriteSize || screenX > screenW+spriteSize {
		return 0, 0, 0, false
	}

	floorScreenY := rh.calculateFloorScreenY(perpDist)
	screenY = floorScreenY - spriteSize

	return screenX, screenY, spriteSize, true
}

// spriteNearCull returns the near-cull distance for monster/NPC sprites: in
// turn-based mode monsters can be very close (adjacent tiles), so allow them
// to render at close range.
func (rh *RenderingHelper) spriteNearCull() float64 {
	if rh.game.turnBasedMode {
		return 1.0
	}
	return 5.0
}

// calculateBoundedSpriteMetrics projects an entity and sizes its sprite with a
// caller-supplied minimum (far-away sprites stay readable). NPCs use this path
// because they carry a per-NPC `size_multiplier` scaling both the projection
// coefficient and the minimum together (so a "size 4" NPC reads as a tall
// building, not the same size as a "size 1" NPC).
//
// There is deliberately NO maximum at playable range: a screen-pixel cap makes
// the sprite SINK as the camera closes in - the floor anchor keeps growing
// ~1/d while the capped size stops, dragging the top below the viewport.
func (rh *RenderingHelper) calculateBoundedSpriteMetrics(entityX, entityY, distance float64, minSize, multiplier int) (screenX, screenY, spriteSize int, visible bool) {
	heightMultiplier := float64(multiplier) / float64(rh.game.config.GetScreenHeight())
	return rh.projectSpriteMetrics(entityX, entityY, distance, rh.spriteNearCull(), heightMultiplier, minSize)
}

// calculateScreenCappedSpriteMetrics projects an entity and sizes its sprite
// using a SCREEN-RELATIVE scaling model.
//
// Use this for entities that should grow freely as the player approaches
// until they fill the viewport - environment props (trees, ferns, moss),
// monsters, and ground containers (loot bags, treasure chests). There are
// no per-instance bounds; the sprite is allowed to scale up to one screen
// height (so a big monster fills the screen at point-blank range) and is
// floored at 8 px so distant sprites don't vanish to a single row.
//
// For the alternative (per-instance min/max bounds, NPCs), see
// calculateBoundedSpriteMetrics.
func (rh *RenderingHelper) calculateScreenCappedSpriteMetrics(entityX, entityY, distance, heightMultiplier float64) (screenX, screenY, spriteSize int, visible bool) {
	return rh.projectSpriteMetrics(entityX, entityY, distance, rh.spriteNearCull(), heightMultiplier, 8)
}

// calculateSpriteSizeWithHeightMultiplier returns a sprite height using the
// same scaling model as environment sprites (e.g., moss rocks).
func (rh *RenderingHelper) calculateSpriteSizeWithHeightMultiplier(perpDist, heightMultiplier float64) int {
	return int(float64(rh.game.config.GetScreenHeight()) / perpDist * float64(rh.game.config.GetTileSize()) * heightMultiplier)
}

// RenderBackgroundLayers renders sky and ground layers
func (rh *RenderingHelper) RenderBackgroundLayers(screen *ebiten.Image) {
	if !rh.drawSkyPanorama(screen) {
		// Draw cached solid-color sky fallback.
		skyOpts := &ebiten.DrawImageOptions{}
		screen.DrawImage(rh.game.skyImg, skyOpts)
	}

	// Draw cached ground
	groundOpts := &ebiten.DrawImageOptions{}
	groundOpts.GeoM.Translate(0, float64(rh.game.config.GetScreenHeight()/2))
	screen.DrawImage(rh.game.groundImg, groundOpts)
}

// floorShaderSrc is a Kage fragment shader that renders the perspective
// floor. Per-fragment logic:
//
//	samplePx = floor(px/2)-2 + 1               # 2x2 block quantization
//	rowDist  = RowDistFactor / (samplePx.y - Horizon)
//	s        = 2-samplePx.x / ScreenSize.x - 1
//	floorX   = camX + rowDist-DirCos + rowDist-PlaneCos-s
//	floorY   = camY + rowDist-DirSin + rowDist-PlaneSin-s
//	tx, ty   = floor(floor[XY] / TileSize)
//	base     = floorColorMap[tx, ty]
//	idx      = floorTextureIndexMap[tx, ty].r - 1
//	texel    = atlas[idx-TexW + int(localX-TexW), int(localY-TexH)]
//	weight   = 0.8 - (1 - smoothstep(1.5, 5.0, texelsPerPixel))
//	color    = mix(base, texel, weight) - brightness(dist, lights)
//
// Inputs:
//
//	Images[0] = floorColorMap (worldWxworldH RGBA8 base colors)
//	Images[1] = floorTexAtlas (horizontal strip of N floor textures)
//	Images[2] = floorTextureIndexMap (R = atlas index + 1, 0 = no texture)
const floorShaderSrc = `//kage:unit pixels

package main

var CamPos vec2
var DirCos float
var DirSin float
var PlaneCos float
var PlaneSin float
var ScreenSize vec2
var Horizon float
var RowDistFactor float
var TileSize float
var WorldSize vec2
var ViewDist float
var MinBrightness float
var Ambient float
var ViewerAmbient float
var TexCount float
var TexTileSize vec2
var LightCount float
var Lights [32]vec4

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	// Per-pixel sampling. (A legacy 2x2 block quantization matched the old CPU
	// floor loop; the floor is GPU-only now, so the half-resolution cost bought
	// nothing.)
	samplePx := dstPos.xy - imageDstOrigin()

	p := samplePx.y - Horizon
	if p < 0.5 {
		p = 0.5
	}
	rowDist := RowDistFactor / p

	t := samplePx.x / ScreenSize.x
	s := 2.0*t - 1.0
	floorX := CamPos.x + rowDist*DirCos + rowDist*PlaneCos*s
	floorY := CamPos.y + rowDist*DirSin + rowDist*PlaneSin*s

	tx := floor(floorX / TileSize)
	ty := floor(floorY / TileSize)

	var rgb vec3
	var atlasIndex float
	if tx < 0.0 || tx >= WorldSize.x || ty < 0.0 || ty >= WorldSize.y {
		rgb = vec3(30.0/255.0, 30.0/255.0, 30.0/255.0)
		atlasIndex = -1.0
	} else {
		raw := imageSrc0UnsafeAt(imageSrc0Origin() + vec2(tx+0.5, ty+0.5))
		rgb = raw.rgb
		idxRaw := imageSrc2UnsafeAt(imageSrc0Origin() + vec2(tx+0.5, ty+0.5))
		atlasIndex = floor(idxRaw.r*255.0 + 0.5) - 1.0
	}

	if atlasIndex >= 0.0 && atlasIndex < TexCount && TexCount > 0.5 {
		lx := fract(floorX / TileSize)
		ly := fract(floorY / TileSize)
		if lx < 0.0 {
			lx += 1.0
		}
		if ly < 0.0 {
			ly += 1.0
		}

		// Texel footprint of one screen pixel (horizontal), used both for the
		// sharp-bilinear band below and the far-field detail fade.
		planeLen := sqrt(PlaneCos*PlaneCos + PlaneSin*PlaneSin)
		worldPerPixel := rowDist * planeLen * 2.0 / ScreenSize.x
		texelsPerPixel := worldPerPixel * TexTileSize.x / TileSize

		// Sharp-bilinear sample inside this texture's atlas cell, wrapping
		// texel lookups within the cell so tiling stays seamless (4-tap mix as
		// in the sky shader; Kage samples nearest-only natively). Plain
		// bilinear smears magnified pixel art, so when one texel spans several
		// screen pixels the interpolation is squeezed into a ~1-pixel band at
		// texel seams - crisp texels, antialiased edges - and relaxes back to
		// ordinary bilinear by the 1:1 footprint.
		fx := lx*TexTileSize.x - 0.5
		fy := ly*TexTileSize.y - 0.5
		bx := floor(fx)
		by := floor(fy)
		sharp := 1.0
		if texelsPerPixel < 1.0 {
			sharp = 1.0 / texelsPerPixel
		}
		fracX := clamp((fx-bx-0.5)*sharp+0.5, 0.0, 1.0)
		fracY := clamp((fy-by-0.5)*sharp+0.5, 0.0, 1.0)
		x0 := mod(bx, TexTileSize.x)
		x1 := mod(bx+1.0, TexTileSize.x)
		y0 := mod(by, TexTileSize.y)
		y1 := mod(by+1.0, TexTileSize.y)
		cellX := atlasIndex * TexTileSize.x
		// imageSrcNUnsafeAt for N>=1 expects coordinates in source-0 texture
		// space; Ebitengine converts them to the target source internally.
		base := imageSrc0Origin()
		c00 := imageSrc1UnsafeAt(base + vec2(cellX+x0+0.5, y0+0.5))
		c10 := imageSrc1UnsafeAt(base + vec2(cellX+x1+0.5, y0+0.5))
		c01 := imageSrc1UnsafeAt(base + vec2(cellX+x0+0.5, y1+0.5))
		c11 := imageSrc1UnsafeAt(base + vec2(cellX+x1+0.5, y1+0.5))
		texColor := mix(mix(c00, c10, fracX), mix(c01, c11, fracX), fracY)

		// In the far field one screen pixel spans many source texels; without
		// mipmaps that shimmers, so fade texture detail toward the tile's flat
		// color by the horizontal texel footprint. Bilinear absorbs the first
		// ~2 texels/pixel cleanly, so detail persists further than the old
		// nearest-sample fade did.
		textureWeight := 0.8 * (1.0 - smoothstep(2.0, 6.0, texelsPerPixel))

		rgb = texColor.rgb*textureWeight + rgb*(1.0-textureWeight)
	}

	dx := floorX - CamPos.x
	dy := floorY - CamPos.y
	dist := sqrt(dx*dx + dy*dy)
	brightness := 1.0 - dist/ViewDist
	if brightness < MinBrightness {
		brightness = MinBrightness
	}
	// Map ambient level: dark maps stay dark until point lights lift them.
	localAmbient := Ambient
	if ViewerAmbient > 0.0 && ViewerAmbient < localAmbient {
		localAmbient = ViewerAmbient
	}
	brightness *= localAmbient
	for i := 0; i < 32; i++ {
		if float(i) >= LightCount {
			break
		}
		L := Lights[i]
		ldx := floorX - L.x
		ldy := floorY - L.y
		// Squared-distance early-out: most pixels are outside most lights, and
		// an unconditional sqrt per light per pixel was the frame-rate cost of
		// torch-lined maps.
		d2 := ldx*ldx + ldy*ldy
		if d2 < L.z*L.z {
			falloff := 1.0 - sqrt(d2)/L.z
			brightness += L.w * falloff
		}
	}
	if brightness > 1.0 {
		brightness = 1.0
	}

	return vec4(rgb*brightness, 1.0)
}
`

// skyShaderSrc is a Kage fragment shader that samples the sky panorama with
// manual bilinear filtering and X-axis wrap. Doing this in a custom shader
// lets us avoid the deprecated DrawTrianglesOptions.Filter / Address paths
// (which break batching and force the source out of the texture atlas).
// turnBlurShaderSrc is a horizontal directional blur - camera motion blur for a
// yaw turn (the whole scene pans sideways, so the smear is horizontal). It box-
// averages taps spread across [-BlurPx, +BlurPx] on the X axis of the source
// scene image; BlurPx (pixels) tracks the turn speed. Y is clamped per row.
const turnBlurShaderSrc = `//kage:unit pixels

package main

var BlurPx float

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	size := imageSrc0Size()
	origin := imageSrc0Origin()
	local := srcPos - origin
	sy := clamp(local.y, 0.5, size.y-0.5)

	const taps = 13
	sum := vec4(0.0)
	for i := 0; i < taps; i++ {
		t := (float(i)/float(taps-1))*2.0 - 1.0 // -1 .. +1
		sx := clamp(local.x+t*BlurPx, 0.5, size.x-0.5)
		sum += imageSrc0UnsafeAt(origin + vec2(sx, sy))
	}
	return (sum / float(taps)) * color
}
`

const skyShaderSrc = `//kage:unit pixels

package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	size := imageSrc0Size()
	origin := imageSrc0Origin()

	// Image-local coordinates (atlas offset removed).
	local := srcPos - origin

	// Bilinear: gather four texels around the sample point. Texel centers sit
	// at integer + 0.5, so shift by 0.5 before flooring.
	p := local - vec2(0.5)
	base := floor(p)
	f := p - base

	// Wrap X over the image width; clamp Y to a valid row.
	sx0 := mod(base.x, size.x)
	sx1 := mod(base.x+1.0, size.x)
	sy0 := clamp(base.y, 0.0, size.y-1.0)
	sy1 := clamp(base.y+1.0, 0.0, size.y-1.0)
	half := vec2(0.5)

	c00 := imageSrc0UnsafeAt(origin + vec2(sx0, sy0) + half)
	c10 := imageSrc0UnsafeAt(origin + vec2(sx1, sy0) + half)
	c01 := imageSrc0UnsafeAt(origin + vec2(sx0, sy1) + half)
	c11 := imageSrc0UnsafeAt(origin + vec2(sx1, sy1) + half)

	top := mix(c00, c10, f.x)
	bot := mix(c01, c11, f.x)
	return mix(top, bot, f.y) * color
}
`

// drawSkyPanorama draws the sky with an isotropic pixel scale (horizontal scale
// equals vertical scale) so panorama features don't appear stretched at any
// resolution. The visible source span auto-adapts to screen width, which means
// the texture repeats more times per 360deg turn on wider screens - classic
// Doom-style behavior, but without anisotropy.
//
// Sampling is done by skyShader, which performs bilinear filtering + X-wrap in
// a single draw call. This avoids deprecated Filter/Address paths so the
// panorama can sit in the shared texture atlas and the draw batches normally.
func (rh *RenderingHelper) drawSkyPanorama(screen *ebiten.Image) bool {
	panorama := rh.game.skyPanorama
	if panorama == nil {
		return false
	}

	shader, err := rh.game.ensureSkyShader()
	if err != nil || shader == nil {
		return false
	}

	screenWidth := rh.game.config.GetScreenWidth()
	skyHeight := rh.game.config.GetScreenHeight() / 2
	if screenWidth <= 0 || skyHeight <= 0 {
		return false
	}

	bounds := panorama.Bounds()
	srcW := float64(bounds.Dx())
	srcH := float64(bounds.Dy())
	if srcW <= 0 || srcH <= 0 {
		return false
	}

	scale := float64(skyHeight) / srcH
	srcSpan := float64(screenWidth) / scale
	if scale <= 0 || srcSpan <= 0 {
		return false
	}

	pixelsPerRadian := srcSpan / rh.game.camera.FOV
	bx := float64(bounds.Min.X)
	by := float64(bounds.Min.Y)
	sx0 := bx + rh.game.camera.Angle*pixelsPerRadian - srcSpan/2
	sx1 := sx0 + srcSpan
	sy0 := by
	sy1 := by + srcH
	dx1 := float32(screenWidth)
	dy1 := float32(skyHeight)

	vertices := [4]ebiten.Vertex{
		{DstX: 0, DstY: 0, SrcX: float32(sx0), SrcY: float32(sy0), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: dx1, DstY: 0, SrcX: float32(sx1), SrcY: float32(sy0), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: 0, DstY: dy1, SrcX: float32(sx0), SrcY: float32(sy1), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
		{DstX: dx1, DstY: dy1, SrcX: float32(sx1), SrcY: float32(sy1), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1},
	}
	indices := [6]uint16{0, 1, 2, 1, 3, 2}
	op := &ebiten.DrawTrianglesShaderOptions{}
	op.Images[0] = panorama
	screen.DrawTrianglesShader(vertices[:], indices[:], shader, op)
	return true
}
