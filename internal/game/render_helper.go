package game

import (
	"fmt"
	"image/color"
	"math"
	"ugataima/internal/character"
	"ugataima/internal/monster"
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
	wallHeightF, floorBottomF := rh.CalculateWallDimensionsWithHeightF(distance, heightMultiplier)
	wallHeight = int(wallHeightF)
	return wallHeight, int(floorBottomF) - wallHeight
}

// CalculateWallDimensionsWithHeightF is the float-precision projection shared
// by textured wall meshes. The integer wrapper remains for cache keys and the
// wall-top occlusion buffer, while visible distant sprite walls keep their
// subpixel top/bottom instead of stepping a whole pixel between frames.
func (rh *RenderingHelper) CalculateWallDimensionsWithHeightF(distance, heightMultiplier float64) (wallHeight, floorBottom float64) {
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
	wallHeight = baseHeight * heightMultiplier

	// Sanity bound, reachable only inside the 1-unit epsilon above: GPU clips
	// off-screen geometry, so huge-but-finite heights cost nothing.
	if maxH := float64(rh.game.config.GetScreenHeight() * 64); wallHeight > maxH {
		wallHeight = maxH
	}
	if wallHeight < 1 {
		wallHeight = 1
	}

	// Anchor wall bottom to the floor line at this distance for consistency
	// with floor and sprite projection.
	return wallHeight, rh.calculateFloorScreenYF(distance)
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
	return int(rh.calculateFloorScreenYF(perpDist))
}

func (rh *RenderingHelper) calculateFloorScreenYF(perpDist float64) float64 {
	screenHeight := float64(rh.game.config.GetScreenHeight())
	tileSize := rh.game.config.GetTileSize()
	horizon := screenHeight / 2

	if perpDist <= 0 {
		perpDist = 1 // Avoid division by zero
	}
	return horizon + (0.5*screenHeight*tileSize)/perpDist
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
	xf, d, ok := rh.projectToScreenXF(entityX, entityY)
	return int(xf), d, ok
}

// projectToScreenXF is projectToScreenX without the pixel truncation.
func (rh *RenderingHelper) projectToScreenXF(entityX, entityY float64) (screenXf float64, depth float64, ok bool) {
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
	return float64(screenW) / 2 * (1 + transformX/transformY), transformY, true
}

// CreateBaseTexturedWallSlice creates a procedural or color-only wall slice
// without distance shading. Sprite-textured walls are handled by the renderer's
// direct column/mesh paths and never enter this cache.
func (rh *RenderingHelper) CreateBaseTexturedWallSlice(tileType world.TileType3D, width, height, wallSide int) *ebiten.Image {
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

// billboardMetrics is THE sizing formula for every floor-anchored entity
// billboard (monsters, NPCs, props, loot containers): height in tiles
// (1.0 == a 1-tile wall), no near-cull - entities remain visible when the
// party steps into their tile. The only knob that varies by subject is the
// minimum pixel floor (how small a far sprite is allowed to recede to).
// Environment TILES (trees etc.) add a tile-type multiplier on top - see
// calculateEnvironmentSpriteMetrics; both funnel into projectSpriteMetrics.
func (rh *RenderingHelper) billboardMetrics(entityX, entityY, distance, sizeTiles float64, minSize int) (screenX, screenY, spriteSize int, visible bool) {
	if sizeTiles <= 0 {
		sizeTiles = 1
	}
	return rh.projectSpriteMetrics(entityX, entityY, distance, 0, sizeTiles, minSize)
}

// billboardMetricsF is billboardMetrics at float precision (see
// projectSpriteMetricsF) - the renderer's collection pass uses it so draw
// paths get subpixel-smooth verticals.
func (rh *RenderingHelper) billboardMetricsF(entityX, entityY, distance, sizeTiles float64, minSize int) (screenXf, bottomF, sizeF float64, visible bool) {
	if sizeTiles <= 0 {
		sizeTiles = 1
	}
	return rh.projectSpriteMetricsF(entityX, entityY, distance, 0, sizeTiles, minSize)
}

// CalculateMonsterSpriteMetricsF is the float twin of CalculateMonsterSpriteMetrics.
func (rh *RenderingHelper) CalculateMonsterSpriteMetricsF(entityX, entityY, distance, sizeTiles float64) (screenXf, bottomF, sizeF float64, visible bool) {
	return rh.billboardMetricsF(entityX, entityY, distance, sizeTiles, rh.game.config.Graphics.Monster.MinSpriteSize)
}

// NPCSpriteMetricsF is the float twin of NPCSpriteMetrics (same routing).
func (rh *RenderingHelper) NPCSpriteMetricsF(npc *character.NPC, ex, ey, distance float64) (screenXf, bottomF, sizeF float64, visible bool) {
	sizeTiles, minSize := rh.npcBillboardParams(npc)
	return rh.billboardMetricsF(ex, ey, distance, sizeTiles, minSize)
}

// npcBillboardParams resolves the (height-in-tiles, min-pixel-floor) pair an
// NPC billboard projects with - the single routing table NPCSpriteMetrics and
// its float twin share.
func (rh *RenderingHelper) npcBillboardParams(npc *character.NPC) (sizeTiles float64, minSize int) {
	if npc.GridSpanTiles >= 2 {
		return float64(npc.GridSpanTiles), sceneryMinSpriteSize
	}
	size := rh.npcSizeTiles(npc)
	if rh.game.npcIsWalkUpProp(npc) {
		return size, rh.game.config.Graphics.Monster.MinSpriteSize
	}
	switch npcRenderCatOf(npc) {
	case catScenery, catLandmark, catWall, catDoor:
		return size, sceneryMinSpriteSize
	default:
		return size, rh.game.config.Graphics.NPC.MinSpriteSize
	}
}

// sceneryMinSpriteSize is the pixel floor for prop standees (scenery/landmark/
// wall/door): unlike people they may recede to almost nothing at range.
const sceneryMinSpriteSize = 8

// CalculateMonsterSpriteMetrics sizes a monster billboard (low pixel floor so
// distant mobs shrink freely). sizeTiles is height in tiles.
func (rh *RenderingHelper) CalculateMonsterSpriteMetrics(entityX, entityY, distance, sizeTiles float64) (screenX, screenY, spriteSize int, visible bool) {
	return rh.billboardMetrics(entityX, entityY, distance, sizeTiles, rh.game.config.Graphics.Monster.MinSpriteSize)
}

// CalculateGroundContainerSpriteMetrics sizes an interactable loot container.
func (rh *RenderingHelper) CalculateGroundContainerSpriteMetrics(entityX, entityY, distance, sizeTiles float64) (screenX, screenY, spriteSize int, visible bool) {
	return rh.billboardMetrics(entityX, entityY, distance, sizeTiles, rh.game.config.Graphics.Monster.MinSpriteSize)
}

// CalculateGroundContainerSpriteMetricsF is the float twin of
// CalculateGroundContainerSpriteMetrics. Loot bags and chests use the same
// float projection as every other standee so they do not reintroduce distant
// whole-pixel jitter.
func (rh *RenderingHelper) CalculateGroundContainerSpriteMetricsF(entityX, entityY, distance, sizeTiles float64) (screenXf, bottomF, sizeF float64, visible bool) {
	return rh.billboardMetricsF(entityX, entityY, distance, sizeTiles, rh.game.config.Graphics.Monster.MinSpriteSize)
}

// CalculateNPCSpriteMetrics sizes a person-NPC billboard (higher pixel floor so
// distant NPCs stay readable). NPCs remain visible when walked up to, matching
// loot containers instead of disappearing under a near-cull.
func (rh *RenderingHelper) CalculateNPCSpriteMetrics(entityX, entityY, distance, sizeTiles float64) (screenX, screenY, spriteSize int, visible bool) {
	return rh.billboardMetrics(entityX, entityY, distance, sizeTiles, rh.game.config.Graphics.NPC.MinSpriteSize)
}

// npcSizeTiles resolves an NPC's sprite height in tiles: a shared size_class
// (same table as monsters) wins, else the raw size_tiles number.
func (rh *RenderingHelper) npcSizeTiles(npc *character.NPC) float64 {
	if npc.SizeClass != "" {
		if h, ok := monster.SizeClassTiles(npc.SizeClass); ok {
			return h
		}
	}
	return npc.SizeTiles
}

// NPCSpriteMetrics projects an NPC billboard through the correct path
// (environment/landmark props vs person NPCs; grid-span facades project the
// WHOLE span so hover/click rects cover every footprint tile). Single source
// for both the renderer and click hit-testing so drawing and hit-tests never
// diverge; the routing lives in npcBillboardParams, shared with the float twin.
func (rh *RenderingHelper) NPCSpriteMetrics(npc *character.NPC, ex, ey, distance float64) (screenX, screenY, spriteSize int, visible bool) {
	sizeTiles, minSize := rh.npcBillboardParams(npc)
	return rh.billboardMetrics(ex, ey, distance, sizeTiles, minSize)
}

// CalculateEnvironmentSpriteMetrics sizes an environment TILE sprite (trees,
// rocks): billboardMetrics' model plus the tile-type height multiplier, and a
// fixed 5.0 near-cull (env tiles keep it even in turn-based mode).
func (rh *RenderingHelper) CalculateEnvironmentSpriteMetrics(entityX, entityY, distance float64, tileType world.TileType3D, sizeScale float64) (screenX, screenY, spriteSize int, visible bool) {
	return rh.projectSpriteMetrics(entityX, entityY, distance, 5.0, rh.envHeightMultiplier(tileType, sizeScale), sceneryMinSpriteSize)
}

// CalculateEnvironmentSpriteMetricsF is the float twin of
// CalculateEnvironmentSpriteMetrics.
func (rh *RenderingHelper) CalculateEnvironmentSpriteMetricsF(entityX, entityY, distance float64, tileType world.TileType3D, sizeScale float64) (screenXf, bottomF, sizeF float64, visible bool) {
	return rh.projectSpriteMetricsF(entityX, entityY, distance, 5.0, rh.envHeightMultiplier(tileType, sizeScale), sceneryMinSpriteSize)
}

// envHeightMultiplier is the visual size multiplier from the tile definition
// (trees = 2.0, ferns = 1.0, ...), scaled by the caller's factor.
func (rh *RenderingHelper) envHeightMultiplier(tileType world.TileType3D, sizeScale float64) float64 {
	if sizeScale <= 0 {
		sizeScale = 1
	}
	heightMultiplier := rh.game.config.Graphics.Sprite.TreeHeightMultiplier
	if world.GlobalTileManager != nil {
		heightMultiplier = world.GlobalTileManager.GetSizeTiles(tileType)
	}
	return heightMultiplier * sizeScale
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
	screenXf, bottomF, sizeF, ok := rh.projectSpriteMetricsF(entityX, entityY, distance, minDistance, heightMultiplier, minSize)
	if !ok {
		return 0, 0, 0, false
	}
	spriteSize = int(sizeF)
	return int(screenXf), int(bottomF) - spriteSize, spriteSize, true
}

// projectSpriteMetricsF is projectSpriteMetrics before pixel truncation:
// float screen-X center, float floor-anchor BOTTOM, float size. The renderer
// draws from these so distant sprites move subpixel-smoothly - deriving the
// top edge from independently truncated ints (int(floor)-int(size)) makes a
// far object's edges hop +/-1px out of phase while walking, a visible shake
// once open-world sightlines reach 40+ tiles.
func (rh *RenderingHelper) projectSpriteMetricsF(entityX, entityY, distance, minDistance, heightMultiplier float64, minSize int) (screenXf, bottomF, sizeF float64, visible bool) {
	if distance > rh.game.camera.ViewDist || distance < minDistance {
		return 0, 0, 0, false
	}

	screenXf, perpDist, ok := rh.projectToScreenXF(entityX, entityY)
	if !ok {
		return 0, 0, 0, false
	}

	sizeF = float64(rh.game.config.GetScreenHeight()) / perpDist * rh.game.config.GetTileSize() * heightMultiplier
	if maxS := float64(rh.game.config.GetScreenHeight() * 64); sizeF > maxS {
		sizeF = maxS
	}
	if minF := float64(minSize); sizeF < minF {
		sizeF = minF
	}

	screenW := float64(rh.game.config.GetScreenWidth())
	if screenXf < -sizeF || screenXf > screenW+sizeF {
		return 0, 0, 0, false
	}

	return screenXf, rh.calculateFloorScreenYF(perpDist), sizeF, true
}

// calculateSpriteSizeWithHeightMultiplier returns a sprite height using the
// same scaling model as environment sprites (e.g., moss rocks).
func (rh *RenderingHelper) calculateSpriteSizeWithHeightMultiplier(perpDist, heightMultiplier float64) int {
	return int(float64(rh.game.config.GetScreenHeight()) / perpDist * float64(rh.game.config.GetTileSize()) * heightMultiplier)
}

// RenderSkyBackground draws the panorama or its solid-color fallback. The
// perspective floor is rendered separately by the floor shader.
func (rh *RenderingHelper) RenderSkyBackground(screen *ebiten.Image) {
	if !rh.drawSkyPanorama(screen) {
		// Draw cached solid-color sky fallback.
		skyOpts := &ebiten.DrawImageOptions{}
		screen.DrawImage(rh.game.skyImg, skyOpts)
	}
}

// DrawGroundFallback covers the lower half when the floor shader is unavailable.
// The normal shader path is fully opaque, so drawing this first would only add
// a redundant half-screen source draw and fill cost.
func (rh *RenderingHelper) DrawGroundFallback(screen *ebiten.Image) {
	groundOpts := &ebiten.DrawImageOptions{}
	groundOpts.GeoM.Translate(0, float64(rh.game.config.GetScreenHeight()/2))
	screen.DrawImage(rh.game.groundImg, groundOpts)
}

// floorShaderSrc is a Kage fragment shader that renders the perspective
// floor. Per-fragment logic:
//
//	rowDist  = RowDistFactor / (px.y - Horizon)
//	s        = 2-px.x / ScreenSize.x - 1
//	floorX   = camX + rowDist-DirCos + rowDist-PlaneCos-s
//	floorY   = camY + rowDist-DirSin + rowDist-PlaneSin-s
//	tx, ty   = floor(floor[XY] / TileSize)
//	base     = floorColorMap[tx, ty]
//	idx      = floorTextureIndexMap[tx, ty].r - 1
//	mip      = clamp(log2(max(texelsX, texelsY)), 0, MaxMip)
//	texel    = trilinear(atlas, idx, mip)      # manual mips - Kage has none
//	color    = mix(base, texel, 0.8) - brightness(dist, lights)
//
// Inputs:
//
//	Images[0] = floorColorMap (worldWxworldH RGBA8 base colors)
//	Images[1] = floorTexAtlas (horizontal strip of N floor textures, mip
//	            chain strips stacked below - see buildFloorTexAtlas)
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
var MaxMip float
var LightCount float
var Lights [32]vec4

// sampleFloorMip does one sharp-bilinear tap inside a texture's atlas cell at
// the given mip level. Level k's strip starts at y = TexH*2*(1-0.5^k) with
// cells scaled by 0.5^k; texel lookups wrap inside the cell so tiling stays
// seamless (Kage samples nearest-only natively). The sharpen factor squeezes
// magnification interpolation into a ~1px band at texel seams; it only
// applies at level 0 - minified levels use plain bilinear.
func sampleFloorMip(atlasIndex, lx, ly, level, texelsPerPixel float) vec4 {
	scale := pow(0.5, level)
	cw := TexTileSize.x * scale
	ch := TexTileSize.y * scale
	yOff := TexTileSize.y * 2.0 * (1.0 - scale)
	fx := lx*cw - 0.5
	fy := ly*ch - 0.5
	bx := floor(fx)
	by := floor(fy)
	sharp := 1.0
	if level < 0.5 && texelsPerPixel < 1.0 {
		sharp = 1.0 / texelsPerPixel
	}
	fracX := clamp((fx-bx-0.5)*sharp+0.5, 0.0, 1.0)
	fracY := clamp((fy-by-0.5)*sharp+0.5, 0.0, 1.0)
	x0 := mod(bx, cw)
	x1 := mod(bx+1.0, cw)
	y0 := mod(by, ch)
	y1 := mod(by+1.0, ch)
	cellX := atlasIndex * cw
	// imageSrcNUnsafeAt for N>=1 expects coordinates in source-0 texture
	// space; Ebitengine converts them to the target source internally.
	base := imageSrc0Origin()
	c00 := imageSrc1UnsafeAt(base + vec2(cellX+x0+0.5, yOff+y0+0.5))
	c10 := imageSrc1UnsafeAt(base + vec2(cellX+x1+0.5, yOff+y0+0.5))
	c01 := imageSrc1UnsafeAt(base + vec2(cellX+x0+0.5, yOff+y1+0.5))
	c11 := imageSrc1UnsafeAt(base + vec2(cellX+x1+0.5, yOff+y1+0.5))
	return mix(mix(c00, c10, fracX), mix(c01, c11, fracX), fracY)
}

// sampleFloorTrilinear blends the two mip levels bracketing mip.
func sampleFloorTrilinear(atlasIndex, lx, ly, mip, texelsPerPixel float) vec4 {
	k0 := floor(mip)
	k1 := min(k0+1.0, MaxMip)
	return mix(
		sampleFloorMip(atlasIndex, lx, ly, k0, texelsPerPixel),
		sampleFloorMip(atlasIndex, lx, ly, k1, texelsPerPixel),
		fract(mip))
}

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

		// Texel footprint of one screen pixel. Horizontal grows linearly with
		// rowDist; VERTICAL grows with rowDist^2 (one screen row near the
		// horizon spans rowDist^2/RowDistFactor world units) and dominates
		// there. Point/bilinear sampling of a footprint many texels wide is
		// the ripple-while-moving: each step lands on different texels. The
		// mip level pre-averages exactly that footprint.
		planeLen := sqrt(PlaneCos*PlaneCos + PlaneSin*PlaneSin)
		worldPerPixel := rowDist * planeLen * 2.0 / ScreenSize.x
		texelsPerPixel := worldPerPixel * TexTileSize.x / TileSize
		vertTexels := rowDist * rowDist / RowDistFactor * TexTileSize.y / TileSize

		// Anisotropic-lite mip selection. The honest vertical footprint grows
		// with rowDist^2, so an isotropic max-axis mip turns the texture flat
		// within a few tiles. Bias the vertical term 9x down - detail carries
		// ~3x farther on the quadratic term - and cover the undersampling gap
		// with 3 taps spread along the column's world step so the
		// ripple-while-moving stays gone. Raise the 9.0 for sharper/farther,
		// lower for calmer.
		foot := max(texelsPerPixel, vertTexels/9.0)
		mip := 0.0
		if foot > 1.0 {
			mip = min(log2(foot), MaxMip)
		}

		// Tap positions: this pixel's ray plus +/- a third of a screen row
		// along the same column - together they span the pixel's true
		// vertical footprint. Tile-local coords wrap; the tap keeps this
		// pixel's texture even if a neighbour tile differs (subpixel blur).
		rowDistB := RowDistFactor / (p + 0.33)
		rowDistC := RowDistFactor / (p - 0.33)
		rayX := DirCos + PlaneCos*s
		rayY := DirSin + PlaneSin*s
		lxB := fract((CamPos.x + rowDistB*rayX) / TileSize)
		lyB := fract((CamPos.y + rowDistB*rayY) / TileSize)
		lxC := fract((CamPos.x + rowDistC*rayX) / TileSize)
		lyC := fract((CamPos.y + rowDistC*rayY) / TileSize)
		texColor := (sampleFloorTrilinear(atlasIndex, lx, ly, mip, texelsPerPixel) +
			sampleFloorTrilinear(atlasIndex, lxB, lyB, mip, texelsPerPixel) +
			sampleFloorTrilinear(atlasIndex, lxC, lyC, mip, texelsPerPixel)) / 3.0

		// Keep the floor material visible across the whole view. The old
		// footprint fade replaced it with the flat tile colour in the distance,
		// which read as fog rather than a textured floor.
		rgb = texColor.rgb*0.8 + rgb*0.2
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

// wrapPanoramaOffset keeps source coordinates near the panorama's own width
// before they are converted to float32 GPU vertices. Real-time rotation leaves
// camera.Angle unbounded; after enough left turns, a large negative source X
// loses subpixel precision and mod() can expose an atlas-gap column at the wrap.
// Removing whole panorama periods is visually identical and keeps the shader's
// per-pixel wrap in a numerically stable range.
func wrapPanoramaOffset(offset, width float64) float64 {
	if width <= 0 || math.IsNaN(offset) || math.IsInf(offset, 0) {
		return 0
	}
	offset = math.Mod(offset, width)
	if offset < 0 {
		offset += width
	}
	return offset
}

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
	// Day/night phase flip: crossfade the incoming panorama over the outgoing one.
	if prev := rh.game.skyPanoramaPrev; prev != nil && rh.game.skyFadeFrames > 0 {
		drew := rh.drawSkyLayer(screen, prev, 1)
		return rh.drawSkyLayer(screen, panorama, rh.game.skyFadeAlpha()) || drew
	}
	return rh.drawSkyLayer(screen, panorama, 1)
}

// drawSkyLayer draws one panorama at the given opacity (premultiplied vertex
// colors, so layered draws source-over into a crossfade).
func (rh *RenderingHelper) drawSkyLayer(screen *ebiten.Image, panorama *ebiten.Image, alpha float32) bool {
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
	centerOffset := wrapPanoramaOffset(rh.game.camera.Angle*pixelsPerRadian, srcW)
	sx0 := bx + centerOffset - srcSpan/2
	sx1 := sx0 + srcSpan
	sy0 := by
	sy1 := by + srcH
	dx1 := float32(screenWidth)
	dy1 := float32(skyHeight)

	a := alpha
	vertices := [4]ebiten.Vertex{
		{DstX: 0, DstY: 0, SrcX: float32(sx0), SrcY: float32(sy0), ColorR: a, ColorG: a, ColorB: a, ColorA: a},
		{DstX: dx1, DstY: 0, SrcX: float32(sx1), SrcY: float32(sy0), ColorR: a, ColorG: a, ColorB: a, ColorA: a},
		{DstX: 0, DstY: dy1, SrcX: float32(sx0), SrcY: float32(sy1), ColorR: a, ColorG: a, ColorB: a, ColorA: a},
		{DstX: dx1, DstY: dy1, SrcX: float32(sx1), SrcY: float32(sy1), ColorR: a, ColorG: a, ColorB: a, ColorA: a},
	}
	indices := [6]uint16{0, 1, 2, 1, 3, 2}
	op := &ebiten.DrawTrianglesShaderOptions{}
	op.Images[0] = panorama
	screen.DrawTrianglesShader(vertices[:], indices[:], shader, op)
	return true
}
