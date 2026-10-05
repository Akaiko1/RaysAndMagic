package game

import (
	"fmt"
	"image/color"
	"math"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// Shared rendering utilities to eliminate code duplication

// RenderingHelper provides common rendering operations
type RenderingHelper struct {
	game                    *MMGame
	textureCache            map[string]*ebiten.Image // Cache for procedural textures
	visibleHeightScaleCache map[visibleHeightScaleKey]float64
}

type visibleHeightScaleKey struct {
	spriteName      string
	aspectFromWidth bool
}

func visibleHeightScaleForFrame(frameWidth, frameHeight, visibleHeight int, aspectFromWidth bool) (float64, bool) {
	if frameWidth <= 0 || frameHeight <= 0 || visibleHeight <= 0 {
		return 0, false
	}
	frameSpan := frameHeight
	if aspectFromWidth {
		frameSpan = frameWidth
	}
	return float64(frameSpan) / float64(visibleHeight), true
}

// NewRenderingHelper creates a new rendering helper
func NewRenderingHelper(game *MMGame) *RenderingHelper {
	return &RenderingHelper{
		game:                    game,
		textureCache:            make(map[string]*ebiten.Image),
		visibleHeightScaleCache: make(map[visibleHeightScaleKey]float64),
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
	baseHeight := float64(rh.game.worldHeight()) / distance * rh.game.config.GetTileSize()

	// Apply height multiplier
	wallHeight = baseHeight * heightMultiplier

	// Sanity bound, reachable only inside the 1-unit epsilon above: GPU clips
	// off-screen geometry, so huge-but-finite heights cost nothing.
	if maxH := float64(rh.game.worldHeight() * 64); wallHeight > maxH {
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
	screenHeight := float64(rh.game.worldHeight())
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

// cameraSpaceXY transforms a world point into camera space: tx is the
// horizontal offset, ty the perpendicular depth. Shared by the point
// projection and the segment-span projection below.
func (rh *RenderingHelper) cameraSpaceXY(entityX, entityY float64) (tx, ty float64, ok bool) {
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
	return invDet * (dirY*dx - dirX*dy), invDet * (-planeY*dx + planeX*dy), true
}

// projectSegmentSpanX projects a world segment's on-screen column span. Unlike
// projecting the endpoints, an endpoint BEHIND the camera plane does not fail:
// it is clamped to just in front of the plane ALONG THE SEGMENT, so the span
// runs off the correct screen edge - the point-blank case where a cross's
// center or corner is behind the party while its arm is still on screen.
// ok=false only when the whole segment is behind the camera or the projection
// degenerates.
func (rh *RenderingHelper) projectSegmentSpanX(x0, y0, x1, y1 float64) (lo, hi int, ok bool) {
	const nearEps = 0.5 // world units; projection at this depth lands far off-screen
	tx0, ty0, ok0 := rh.cameraSpaceXY(x0, y0)
	tx1, ty1, ok1 := rh.cameraSpaceXY(x1, y1)
	if !ok0 || !ok1 {
		return 0, 0, false
	}
	if ty0 < nearEps && ty1 < nearEps {
		return 0, 0, false
	}
	// Camera space is linear in the world point, so the plane crossing
	// interpolates exactly.
	clamp := func(txA, tyA, txB, tyB float64) (float64, float64) {
		if tyA >= nearEps {
			return txA, tyA
		}
		s := (nearEps - tyA) / (tyB - tyA)
		return txA + s*(txB-txA), nearEps
	}
	tx0, ty0 = clamp(tx0, ty0, tx1, ty1)
	tx1, ty1 = clamp(tx1, ty1, tx0, ty0)
	halfW := float64(rh.game.worldWidth()) / 2
	xa := halfW * (1 + tx0/ty0)
	xb := halfW * (1 + tx1/ty1)
	if xa > xb {
		xa, xb = xb, xa
	}
	return int(xa), int(xb), true
}

// projectToScreenXF is projectToScreenX without the pixel truncation.
func (rh *RenderingHelper) projectToScreenXF(entityX, entityY float64) (screenXf float64, depth float64, ok bool) {
	transformX, transformY, okDet := rh.cameraSpaceXY(entityX, entityY)
	if !okDet || transformY <= 0 {
		return 0, 0, false // degenerate matrix, or behind the camera
	}
	screenW := rh.game.worldWidth()
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
		if renderType == config.TileRenderWall {
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
	case catScenery, catLandmark, catWideLandmark, catWall, catDoor:
		return size, sceneryMinSpriteSize
	default:
		return size, rh.game.config.Graphics.NPC.MinSpriteSize
	}
}

// sceneryMinSpriteSize is the pixel floor for prop standees (scenery/landmark/
// wall/door): unlike people they may recede to almost nothing at range.
const sceneryMinSpriteSize = 8

// The Calculate*SpriteMetrics trio below are the PIXEL (int) view of the float
// cores: truncating whole-pixel metrics is what made distant sprites jitter, so
// draw paths must use the F twins. These stay for hit tests and the golden
// size/near-cull tests, which reason in pixels by nature.

// CalculateMonsterSpriteMetrics sizes a monster billboard (low pixel floor so
// distant mobs shrink freely). sizeTiles is height in tiles.
func (rh *RenderingHelper) CalculateMonsterSpriteMetrics(entityX, entityY, distance, sizeTiles float64) (screenX, screenY, spriteSize int, visible bool) {
	return rh.billboardMetrics(entityX, entityY, distance, sizeTiles, rh.game.config.Graphics.Monster.MinSpriteSize)
}

// CalculateGroundContainerSpriteMetricsF sizes an interactable loot container.
// Loot bags and chests use the same float projection as every other standee so
// they do not reintroduce distant whole-pixel jitter.
func (rh *RenderingHelper) CalculateGroundContainerSpriteMetricsF(entityX, entityY, distance, sizeTiles float64) (screenXf, bottomF, sizeF float64, visible bool) {
	return rh.billboardMetricsF(entityX, entityY, distance, sizeTiles, rh.game.config.Graphics.Monster.MinSpriteSize)
}

// npcSizeTiles resolves an NPC's quantized frame span from the same config
// table used by monsters and tile standees. A real content load validates every
// class; 1.0 is only a defensive fallback for hand-built test NPCs.
func (rh *RenderingHelper) npcSizeTiles(npc *character.NPC) float64 {
	if rh != nil && rh.game != nil && rh.game.config != nil {
		if value, ok := config.ResolveSizeClassTiles(rh.game.config.Graphics.SizeClasses, npc.SizeClass); ok {
			category := npcRenderCatOf(npc)
			if category == catNPC {
				return value
			}
			aspectFromWidth := category == catLandmark && rh.game.config.Graphics.Standee.Enabled
			return value * rh.visibleHeightFrameScale(rh.game.activityNPCSprite(npc), aspectFromWidth)
		}
	}
	return 1.0
}

// visibleHeightFrameScale converts a class's target VISIBLE height into the
// full-frame span consumed by the projection code. Transparent source padding
// therefore cannot force content authors back to per-object decimal sizes.
// Crossed landmarks author projected width and derive full height from texture
// aspect, so their conversion uses frame width. Flat fallback sprites and
// ordinary props use frame height.
func (rh *RenderingHelper) visibleHeightFrameScale(spriteName string, aspectFromWidth bool) float64 {
	if rh == nil || rh.game == nil || rh.game.sprites == nil || spriteName == "" {
		return 1
	}
	key := visibleHeightScaleKey{spriteName: spriteName, aspectFromWidth: aspectFromWidth}
	if value, ok := rh.visibleHeightScaleCache[key]; ok {
		return value
	}
	resolveFraction := func(name string) (float64, bool) {
		bounds, frameWidth, frameHeight, known := rh.game.sprites.SpriteVisibleFrameBounds(name)
		visibleHeight := bounds.Dy()
		if !known {
			return 0, false
		}
		scale, ok := visibleHeightScaleForFrame(frameWidth, frameHeight, visibleHeight, aspectFromWidth)
		if !ok || scale <= 0 {
			return 0, false
		}
		return 1 / scale, true
	}
	names := rh.game.sprites.GetSpriteVariants(spriteName)
	if len(names) == 0 {
		names = []string{spriteName}
	}
	fractionSum := 0.0
	resolved := 0
	for _, name := range names {
		if fraction, ok := resolveFraction(name); ok {
			fractionSum += fraction
			resolved++
		}
	}
	value := 1.0
	if resolved > 0 && fractionSum > 0 {
		value = float64(resolved) / fractionSum
	}
	// Unknown can mean a deferred load, not just missing art. Never freeze a
	// fallback or a partially measured variant family into the final geometry.
	if resolved == len(names) {
		rh.visibleHeightScaleCache[key] = value
	}
	return value
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
	aspectFromWidth := false
	if world.GlobalTileManager != nil {
		aspectFromWidth = world.GlobalTileManager.GetRenderType(tileType) == config.TileRenderLandmarkStandee &&
			rh.game.config.Graphics.Standee.Enabled
	}
	return rh.envHeightMultiplierForMode(tileType, sizeScale, aspectFromWidth)
}

// flatEnvHeightMultiplier resolves the same authored visible-height contract
// for the legacy raycast fallback. It never uses the crossed-landmark width
// interpretation because this path draws one flat sprite.
func (rh *RenderingHelper) flatEnvHeightMultiplier(tileType world.TileType3D, sizeScale float64) float64 {
	return rh.envHeightMultiplierForMode(tileType, sizeScale, false)
}

func (rh *RenderingHelper) envHeightMultiplierForMode(tileType world.TileType3D, sizeScale float64, aspectFromWidth bool) float64 {
	if sizeScale <= 0 {
		sizeScale = 1
	}
	// 1.0 mirrors TileManager.GetSizeTiles' own defensive default: content
	// validation guarantees a class in a real load, so this only covers an
	// ad-hoc test manager with no tile database.
	heightMultiplier := 1.0
	if world.GlobalTileManager != nil {
		heightMultiplier = world.GlobalTileManager.GetSizeTiles(tileType)
		renderType := world.GlobalTileManager.GetRenderType(tileType)
		// Static swarm tiles draw their authored fixed mote layout procedurally;
		// night motes are separate moving emissions. Both paths are selected
		// by tile content rather than by a hardcoded tile key.
		// Only the natural cross authors frame WIDTH and skips the alpha
		// normalization; a crossed_prop authors visible height like the flat
		// standee it replaced.
		if renderType != config.TileRenderCrossedStandee && !isFireflySwarmTile(tileType) {
			heightMultiplier *= rh.visibleHeightFrameScale(
				world.GlobalTileManager.GetSprite(tileType),
				aspectFromWidth,
			)
		}
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

	sizeF = float64(rh.game.worldHeight()) / perpDist * rh.game.config.GetTileSize() * heightMultiplier
	if maxS := float64(rh.game.worldHeight() * 64); sizeF > maxS {
		sizeF = maxS
	}
	if minF := float64(minSize); sizeF < minF {
		sizeF = minF
	}

	screenW := float64(rh.game.worldWidth())
	if screenXf < -sizeF || screenXf > screenW+sizeF {
		return 0, 0, 0, false
	}

	return screenXf, rh.calculateFloorScreenYF(perpDist), sizeF, true
}

// calculateSpriteSizeWithHeightMultiplier returns a sprite height using the
// same scaling model as environment sprites (e.g., moss rocks).
func (rh *RenderingHelper) calculateSpriteSizeWithHeightMultiplier(perpDist, heightMultiplier float64) int {
	return int(float64(rh.game.worldHeight()) / perpDist * float64(rh.game.config.GetTileSize()) * heightMultiplier)
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
	groundOpts.GeoM.Translate(0, float64(rh.game.worldHeight()/2))
	screen.DrawImage(rh.game.groundImg, groundOpts)
}

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

	screenWidth := rh.game.worldWidth()
	skyHeight := rh.game.worldHeight() / 2
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
