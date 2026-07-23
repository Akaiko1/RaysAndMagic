package game

import (
	"cmp"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"slices"
	"sort"
	"strings"
	"time"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"
	"ugataima/internal/threading/rendering"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
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
	Seed      int
	Firefly   bool
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

type standeeKeyNameParts struct {
	prefix string
	name   string
}

// Renderer handles all 3D rendering functionality
type Renderer struct {
	game                     *MMGame
	floorColorCache          map[[2]int]color.RGBA // Now world-level, static after init
	whiteImg                 *ebiten.Image         // 1x1 white image for untextured polygons
	renderedSpritesThisFrame map[[2]int]bool       // Track which environment sprites have been rendered this frame
	// GPU floor rendering - a Kage shader replaces the per-pixel CPU loop.
	// floorColorMap is a worldWxworldH RGBA8 image with base tile colors.
	// floorTextureIndexMap is a worldWxworldH RGBA8 image; R encodes
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
	floorTexMaxMip       int    // deepest mip level packed into floorTexAtlas
	floorTexturesKey     string // biome the floor textures were loaded for (cache key)
	canopyShadeFactors   []float64
	canopyShadeW         int
	canopyShadeH         int
	canopyViewerShade    float64 // smoothed PURE viewer shade factor (no ambient - see viewerShadeSmoothed)
	canopyViewerFrame    int64
	canopyViewerReady    bool
	// Per-frame reusable uniform buffer for floor shader light data, avoids
	// a 64-float allocation each draw call.
	floorLightsBuf [maxFloorShaderLights * 4]float32
	// Reusable uniforms map for the floor shader (values mutated in place).
	floorUniforms map[string]any
	// Shared DrawImageOptions for per-column/per-sprite draws (see sharedDrawOpts).
	spriteOpts ebiten.DrawImageOptions
	// Current map's ambient light level (1.0 = daylight), cached per frame.
	ambientLight float64
	// Wood-silhouette cache for standee token cores, keyed per sprite frame.
	standeeCoreCache map[standeeCoreKey]*ebiten.Image
	// Stable prefixed names used by standeeCoreKey. Constructing "mob:"+key,
	// "npc:"+key, etc. for every visible object every frame showed up as
	// allocator churn; the identity set is tiny and immutable after load.
	standeeKeyNames map[standeeKeyNameParts]string
	// Standee mip chains are immutable, normalized copies of immutable sprite
	// frames. Adjacent levels are blended by the trilinear shader so Ebitengine's
	// integer mip selection cannot make a whole token flash sharp/soft at range.
	standeeMipCache        map[standeeMipKey]*standeeMipChain
	standeeTrilinearShader *ebiten.Shader
	standeeTrilinearOpts   ebiten.DrawTrianglesShaderOptions
	standeeVolumeShader    *ebiten.Shader
	standeeVolumeOpts      ebiten.DrawTrianglesShaderOptions
	// Reusable vertex/index buffers for batched standee surface draws.
	standeeVerts []ebiten.Vertex
	standeeIdx   []uint16
	// Thick standee slabs can exceed uint16 when a close full-screen face has
	// many core shells. The material shader batches the whole slab with 32-bit
	// indices instead of splitting every surface into a separate draw command.
	standeeMaterialIdx []uint32
	// Reusable slab-surface buffers (A/B keep both crossed-tree slabs live for
	// the interleaved arm draw).
	standeeSurfaces  []standeeSurface
	standeeSurfacesB []standeeSurface

	// Per-frame draw counters surfaced in the FPS overlay (perf diagnostics).
	statTreesDrawn   int
	statStandeeCalls int
	statAuraTiles    int
	// Per-frame sprite-pass sub-phase timings (ms) for the perf overlay.
	statFloorMs   float64
	statWallsMs   float64
	statSpritesMs float64
	// Wall-torch corners for the current map (flag wall_torches), rebuilt on
	// world change alongside the other per-map caches.
	wallTorches []wallTorchPoint
	// teleporterTiles caches every teleporter tile on the current map (+ its glow
	// colour), collected during precomputeFloorColorCache's per-map tile scan so
	// drawTeleporterTileFx doesn't rescan the world each frame.
	teleporterTiles []teleporterTileFx
	// Eased facing of scenery tokens (they slowly turn toward the camera),
	// keyed by tile - environment sprites have no entity struct to carry it.
	standeeEnvYaw map[[2]int]standeeEnvYawState
	// Same for animated NPC tokens (people turn to face the party; static
	// objects keep the showcase spin instead). NPC pointers are stable.
	standeeNPCYaw map[*character.NPC]standeeEnvYawState
	// standeeWallOcclusion is scoped to the current standee draw. Wall-mounted
	// tokens identify their exact backing wall plane, while doors use only a
	// small seam allowance. Normal standees keep its zero value.
	standeeWallOcclusion standeeWallOcclusion
	// Transparent environment sprite cache for performance
	transparentSpritesCache []TransparentSpriteData // Cached list of transparent sprites
	// treeTilesCache lists every tree tile (one entry per tile) for the
	// crossed-standee billboard mode (config.Graphics.TreesAsBillboards). Built
	// alongside transparentSpritesCache; unused in the per-column tree mode.
	treeTilesCache []TransparentSpriteData
	// mapRenderTileTypes is the unique authored tile inventory discovered by the
	// same map scan that builds the sprite caches. The map-resource prewarmer uses
	// it instead of rescanning the world or maintaining a parallel asset list.
	mapRenderTileTypes              []world.TileType3D
	mapRenderResourcePrewarmPending bool
	mapRenderResourcePrewarmMapKey  string
	mapRenderResidentMapKeys        []string
	mapRenderStandeeKeysByMap       map[string]map[standeeCoreKey]struct{}
	mapRenderSharedResourcesReady   bool
	mapRenderSharedStandeeKeys      map[standeeCoreKey]struct{}
	// Cached tile light sources (world-space)
	tileLightCache []LightSource
	// Active light sources for current frame (world-space)
	activeLights []LightSource
	// Precomputed ray direction cache for performance
	rayDirectionsX []float64 // Cached cos values for rays
	rayDirectionsY []float64 // Cached sin values for rays
	// Camera-plane basis used to build ray-boundary directions for minified
	// textured walls. Set alongside rayDirections every frame; keeping it here
	// avoids recalculating sin/cos/tan once per rendered wall column.
	rayDirX, rayDirY     float64
	rayPlaneX, rayPlaneY float64
	// Per-ray result storage, pre-allocated to avoid both hit-slice allocation
	// and boxing MultiRaycastHit into an interface on every ray. Each worker
	// writes to a disjoint slot; RenderRaycastInto keeps only a pointer to it.
	rayHitResults []MultiRaycastHit
	// Sprite cache for brightness-adjusted alpha variants. The composite key
	// avoids a per-frame fmt.Sprintf allocation that showed up in the hot draw
	// path (one call per visible transparent sprite per frame).
	processedSpriteCache map[processedSpriteKey]*ebiten.Image
	// Per-sprite 1px column SubImages for near sprite-textured wall slices -
	// SubImage allocates a new *ebiten.Image per call (one per wall column per
	// frame). Sprites come from the SpriteManager and live for the whole game.
	wallSliceColumns map[*ebiten.Image][]*ebiten.Image
	// Tileable source copies for the minified wall path. Ebiten builds mipmaps
	// before address-repeat is applied, so the source itself must contain the
	// neighbouring tiles to keep a seam-free mip chain.
	wallTextureRepeats map[*ebiten.Image]*ebiten.Image
	wallSliceVerts     [4]ebiten.Vertex
	wallSliceTriOpts   ebiten.DrawTrianglesOptions
	// Minified opaque wall slices are independent screen columns, so slices
	// sharing a source can be submitted in one draw. This preserves the exact
	// per-column rectangle geometry while avoiding one DrawTriangles call per
	// ray on long distant walls.
	wallMipBatchSource  *ebiten.Image
	wallMipBatchVerts   []ebiten.Vertex
	wallMipBatchIndices []uint16
	// Per-sprite animation-frame SubImages (see selectAnimatedSpriteFrame),
	// same per-frame SubImage churn for animated NPC sheets.
	animFrameCache map[*ebiten.Image][]*ebiten.Image
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
	// Unique melee ribbons reuse the standee vertex/index scratch buffers and
	// this options value. Those effects can emit several ribbons per frame, so
	// keeping all three temporaries on the renderer avoids transient GC churn.
	meleeTriOpts ebiten.DrawTrianglesOptions
	// softGlowImg is a radial-gradient (opaque centre -> transparent edge) white
	// texture for soft ROUND glows - used for spell projectile bodies/halos so a
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
	if len(r.rayHitResults) == numRays {
		return
	}
	r.rayHitResults = make([]MultiRaycastHit, numRays)
	for i := range r.rayHitResults {
		r.rayHitResults[i].Hits = make([]RaycastHit, 0, 8)
	}
}

// handleResize reallocates fixed-size rendering buffers when the viewport
// size changes (e.g. fullscreen toggle, window resize). Callers must also
// update the depth buffer + sky/ground images on MMGame - see
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
	// A physical world switch is a real render-resource boundary. Generated
	// standee cores/mips from the old world cannot become visible again until a
	// later map load, so release that residency before inventorying the new map.
	r.resetMapRenderResourceResidency()
	r.processedSpriteCache = make(map[processedSpriteKey]*ebiten.Image)

	if world.GlobalTileManager == nil || r.game.GetCurrentWorld() == nil {
		r.transparentSpritesCache = nil
		r.treeTilesCache = nil
		r.mapRenderTileTypes = nil
		r.mapRenderResourcePrewarmPending = false
		r.mapRenderResourcePrewarmMapKey = ""
		r.tileLightCache = nil
		r.clearCanopyShadeCache()
		return
	}

	var cache []TransparentSpriteData
	var treeCache []TransparentSpriteData
	var lights []LightSource
	tileTypes := make([]world.TileType3D, 0, 32)
	seenTileTypes := make(map[world.TileType3D]struct{}, 32)
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
			if _, seen := seenTileTypes[tileType]; !seen {
				seenTileTypes[tileType] = struct{}{}
				tileTypes = append(tileTypes, tileType)
			}

			if tileData := world.GlobalTileManager.GetTileData(tileType); tileData != nil && tileData.Light != nil && tileData.Light.Enabled {
				radius := tileData.Light.RadiusTiles * tileSize
				intensity := tileData.Light.Intensity
				if radius > 0 && intensity > 0 {
					light := LightSource{
						X:         worldX,
						Y:         worldY,
						Radius:    radius,
						Intensity: intensity,
					}
					if isFireflySwarmTile(tileType) {
						light.Seed = fireflySwarmSeed(tileX, tileY)
						light.Firefly = true
					}
					lights = append(lights, light)
				}
			}

			// Tree tiles: cache one entry per tile for the crossed-standee mode.
			if world.GlobalTileManager.GetRenderType(tileType) == "tree_sprite" {
				spriteName := world.GlobalTileManager.GetSprite(tileType)
				treeCache = append(treeCache, TransparentSpriteData{
					tileX: tileX, tileY: tileY, worldX: worldX, worldY: worldY,
					tileType: tileType, spriteName: spriteName,
				})
			}

			// Check if it's a transparent environment sprite (trees are rendered separately via raycasting).
			// Landmark tiles (e.g. the city fountain) share this sprite pass - they're
			// drawn as a tall crossed standee in drawEnvironmentSprite.
			if rt := world.GlobalTileManager.GetRenderType(tileType); (rt == "environment_sprite" || rt == "landmark") &&
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
	r.treeTilesCache = treeCache
	r.mapRenderTileTypes = tileTypes
	r.tileLightCache = lights
	// Defer heavyweight PNG decode/mip/upload work until the game has actually
	// entered play. GameLoop.Update consumes this once after map load/switch.
	r.scheduleMapRenderResourcePrewarm(currentMapKey())
	r.buildCanopyShadeCache()
	r.buildWallTorches()
	r.reserveUnifiedSpriteCapacity()
}

// reserveUnifiedSpriteCapacity sizes the per-frame collector from the current
// map's authored/runtime population. Without this, the first walk into a dense
// tree corridor repeatedly grows the slice while later passes reuse it.
func (r *Renderer) reserveUnifiedSpriteCapacity() {
	currentWorld := r.game.GetCurrentWorld()
	if currentWorld == nil {
		r.unifiedSprites = nil
		return
	}
	// A crossed tree normally contributes one entry. Near another standee it
	// can expand to four arm entries so painter order can interleave them.
	needed := len(r.transparentSpritesCache) + 4*len(r.treeTilesCache) +
		len(currentWorld.Monsters) + len(r.game.groundContainers) + len(r.wallTorches)
	for _, npc := range currentWorld.NPCs {
		if npc == nil {
			continue
		}
		if npc.GridSpanTiles >= 2 && r.game.config.Graphics.Standee.Enabled {
			needed += npc.GridSpanTiles
		} else {
			needed++
		}
	}
	if cap(r.unifiedSprites) < needed {
		r.unifiedSprites = make([]UnifiedSpriteRenderData, 0, needed)
		return
	}
	r.unifiedSprites = r.unifiedSprites[:0]
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

func (r *Renderer) prefixedStandeeKeyName(prefix, name string) string {
	parts := standeeKeyNameParts{prefix: prefix, name: name}
	if cached := r.standeeKeyNames[parts]; cached != "" {
		return cached
	}
	if r.standeeKeyNames == nil {
		r.standeeKeyNames = make(map[standeeKeyNameParts]string)
	}
	key := prefix + ":" + name
	r.standeeKeyNames[parts] = key
	return key
}

// computeNumRays derives the per-frame ray count from the configured screen
// width and ray width, ceil-divided so all pixels are covered, with safety
// guards against zero/negative config values.
func (r *Renderer) computeNumRays() int {
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
	return numRays
}

// precomputeRayDirections calculates ray directions once per frame for performance
func (r *Renderer) precomputeRayDirections() {
	// Safety check: ensure ray direction cache is allocated
	if len(r.rayDirectionsX) == 0 || len(r.rayDirectionsY) == 0 {
		numRays := r.computeNumRays()
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
	r.rayDirX, r.rayDirY = dirX, dirY
	r.rayPlaneX, r.rayPlaneY = planeX, planeY

	for i := 0; i < numRays; i++ {
		// Use the camera plane for ray directions so walls/floor/sprites align.
		cameraX := 2*(float64(i)+0.5)/float64(numRays) - 1
		r.rayDirectionsX[i] = dirX + planeX*cameraX
		r.rayDirectionsY[i] = dirY + planeY*cameraX
	}
}

// sharedDrawOpts returns the renderer's reusable DrawImageOptions, fully reset.
// Wall slices, trees and billboards draw hundreds of times per frame; a fresh
// options struct per draw was pure allocator churn. Callers must finish their
// DrawImage before any other code path grabs the shared struct.
func (r *Renderer) sharedDrawOpts() *ebiten.DrawImageOptions {
	o := &r.spriteOpts
	o.GeoM.Reset()
	o.ColorScale.Reset()
	o.Blend = ebiten.Blend{}        // zero value = source-over
	o.Filter = ebiten.FilterNearest // sites opt into linear for minification
	return o
}

func (r *Renderer) updateActiveLights() {
	r.activeLights = r.activeLights[:0]

	// Cache the current map's ambient level for this frame: every brightness
	// path multiplies by it, making dark maps (low ambient_light) genuinely
	// dark until a torch / spell glow lifts them. Flows into the floor shader's
	// Ambient uniform and every CPU brightness path via mapAmbient().
	r.ambientLight = 1.0
	if r.game.dayNightOutdoor {
		// Day/night maps light STRICTLY by the day/night clock (day_light noon
		// -> night_light midnight); their authored ambient_light (an interior
		// darkening knob) is ignored so the cycle alone sets brightness.
		r.ambientLight = r.game.dayNightLightScaleNow()
	} else if world.GlobalWorldManager != nil {
		if mc := world.GlobalWorldManager.GetCurrentMapConfig(); mc != nil && mc.AmbientLight > 0 {
			r.ambientLight = mc.AmbientLight
		}
	}

	camX := r.game.camera.X
	camY := r.game.camera.Y
	viewDist := r.game.camera.ViewDist

	for _, light := range r.tileLightCache {
		radius := light.Radius
		if radius <= 0 || light.Intensity <= 0 {
			continue
		}
		if light.Firefly {
			light.Intensity *= fireflySwarmFlicker(light.Seed, r.game.frameCount)
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
		// torchLightRadius is stored in TILES (TorchLightRadiusTiles); light
		// sources work in world units - without the conversion the torch lit a
		// 4-PIXEL circle (invisible; unnoticed until dark maps existed).
		r.activeLights = append(r.activeLights, LightSource{
			X:         camX,
			Y:         camY,
			Radius:    r.game.torchLightRadius * float64(r.game.config.GetTileSize()),
			Intensity: 1.0, // restores full daylight at the player; capped at 1 everywhere
		})
	}

	// Spell projectiles glow in flight: the floor shader pools light under
	// them (and calculateBrightnessWithTorchLight lifts nearby sprites), so a
	// fireball lights its way down a dark corridor.
	tile := float64(r.game.config.GetTileSize())
	for i := range r.game.magicProjectiles {
		p := &r.game.magicProjectiles[i]
		if !p.Active {
			continue
		}
		radius := tile * 2.2
		dx := p.X - camX
		dy := p.Y - camY
		if maxDist := viewDist + radius; dx*dx+dy*dy > maxDist*maxDist {
			continue
		}
		r.activeLights = append(r.activeLights, LightSource{
			X: p.X, Y: p.Y, Radius: radius, Intensity: 0.45,
		})
	}

	// Impact flashes fade out with their remaining life.
	for _, il := range r.game.impactLights {
		if il.MaxLife <= 0 {
			continue
		}
		f := float64(il.Life) / float64(il.MaxLife)
		r.activeLights = append(r.activeLights, LightSource{
			X: il.X, Y: il.Y, Radius: il.Radius, Intensity: il.Intensity * f,
		})
	}

	// Wall torches: flickering corner lights (map flag wall_torches).
	torchRadius := wallTorchLightRadiusTiles * tile
	for _, tp := range r.wallTorches {
		dx := tp.X - camX
		dy := tp.Y - camY
		if maxDist := viewDist + torchRadius; dx*dx+dy*dy > maxDist*maxDist {
			continue
		}
		r.activeLights = append(r.activeLights, LightSource{
			X: tp.X, Y: tp.Y, Radius: torchRadius,
			Intensity: wallTorchLightIntensity * wallTorchFlicker(tp.seed, r.game.frameCount),
		})
	}

	// The floor shader takes at most maxFloorShaderLights; with torch-lined
	// halls the list can exceed that, and blind truncation would drop the
	// NEAREST lights as easily as the farthest. Keep the closest ones first.
	if len(r.activeLights) > maxFloorShaderLights {
		slices.SortFunc(r.activeLights, func(a, b LightSource) int {
			da := (a.X-camX)*(a.X-camX) + (a.Y-camY)*(a.Y-camY)
			db := (b.X-camX)*(b.X-camX) + (b.Y-camY)*(b.Y-camY)
			return cmp.Compare(da, db)
		})
	}
}

func (r *Renderer) applyLocalLight(brightness float64, sourceX, sourceY, worldX, worldY, radius, intensity float64) float64 {
	if radius <= 0 || intensity <= 0 {
		return brightness
	}
	// Squared-distance reject before the sqrt: most lights are out of range.
	dx := worldX - sourceX
	dy := worldY - sourceY
	distSq := dx*dx + dy*dy
	if distSq > radius*radius {
		return brightness
	}
	falloff := 1.0 - (math.Sqrt(distSq) / radius)
	brightness += intensity * falloff
	if brightness > 1.0 {
		brightness = 1.0
	}
	return brightness
}

// calculateBrightnessWithTorchLight calculates brightness with torch light effects
func (r *Renderer) calculateBrightnessWithTorchLight(worldX, worldY, distance float64) float64 {
	// Base brightness: distance falloff scaled by the map's ambient level
	// (dark maps stay dark until a light source lifts them).
	brightness := 1.0 - (distance / r.game.camera.ViewDist)
	if brightness < r.game.config.Graphics.BrightnessMin {
		brightness = r.game.config.Graphics.BrightnessMin
	}
	brightness *= r.localAmbientAt(worldX, worldY)

	for _, light := range r.activeLights {
		brightness = r.applyLocalLight(brightness, light.X, light.Y, worldX, worldY, light.Radius, light.Intensity)
		if brightness == 1.0 {
			break // clamp ceiling reached: every further light returns 1.0 unchanged
		}
	}

	return brightness
}

// wallPointBrightness lights one raycast-column surface (wall slice, tree,
// raycast env sprite): it reconstructs the column's world point from the
// cached ray direction (point = cam + ray-distance, the rays are built as
// dir + plane-s so the ray parameter IS the perpendicular distance) and runs
// the full light-aware brightness. Without this, dark maps would have lit
// floors and pitch-black walls - the torch must land on walls too.
func (r *Renderer) wallPointBrightness(screenX int, distance float64) float64 {
	n := len(r.rayDirectionsX)
	if n == 0 {
		return r.calculateBrightnessWithTorchLight(r.game.camera.X, r.game.camera.Y, distance)
	}
	idx := screenX * n / r.game.config.GetScreenWidth()
	if idx < 0 {
		idx = 0
	} else if idx >= n {
		idx = n - 1
	}
	wx := r.game.camera.X + r.rayDirectionsX[idx]*distance
	wy := r.game.camera.Y + r.rayDirectionsY[idx]*distance
	return r.calculateBrightnessWithTorchLight(wx, wy, distance)
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
	brightness += (depth - 0.5) * 0.2 // +/-0.1

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
		if mapConfig := world.GlobalWorldManager.GetCurrentMapConfig(); mapConfig != nil {
			defaultFloorColor = mapConfig.DefaultFloorColor
		} else {
			defaultFloorColor = [3]int{60, 180, 60} // Fallback green
		}
	} else {
		defaultFloorColor = [3]int{60, 180, 60} // Fallback green
	}

	defaultMapFloor := color.RGBA{uint8(defaultFloorColor[0]), uint8(defaultFloorColor[1]), uint8(defaultFloorColor[2]), 255}
	// The unified world blends several maps: each tile's default floor color
	// comes from ITS region's config, not the party's current one.
	defaultFloorAt := func(tx, ty int) color.RGBA {
		if r.game.openWorldActive() {
			if mc := world.GlobalWorldManager.MapConfigAtTile(tx, ty); mc != nil {
				return color.RGBA{uint8(mc.DefaultFloorColor[0]), uint8(mc.DefaultFloorColor[1]), uint8(mc.DefaultFloorColor[2]), 255}
			}
		}
		return defaultMapFloor
	}
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
	var teleporterTiles []teleporterTileFx
	for tileY := 0; tileY < worldHeight; tileY++ {
		for tileX := 0; tileX < worldWidth; tileX++ {
			// Get base floor color for this tile
			checkX := float64(tileX)*float64(r.game.config.GetTileSize()) + float64(r.game.config.GetTileSize())/2
			checkY := float64(tileY)*float64(r.game.config.GetTileSize()) + float64(r.game.config.GetTileSize())/2
			currentTile := r.game.GetCurrentWorld().GetTileAt(checkX, checkY)

			baseColor := defaultFloorAt(tileX, tileY)
			var tileData *config.TileData
			if world.GlobalTileManager != nil {
				tileData = world.GlobalTileManager.GetTileData(currentTile)
			}
			// Markers and objects that inherit a floor use the dominant surrounding
			// floor as their base. Only explicit marker inheritance also receives
			// floor-near effects below; ordinary props should match that floor exactly.
			inheritsFloor := tileData.InheritsNeighbourFloor()
			markerInherit := tileData != nil && tileData.InheritFloor
			if inheritsFloor {
				if inherited := r.inheritedFloorColor(tileX, tileY, currentTile); inherited != ([3]int{0, 0, 0}) {
					baseColor = color.RGBA{uint8(inherited[0]), uint8(inherited[1]), uint8(inherited[2]), 255}
				}
			}
			if tileData != nil && strings.EqualFold(tileData.Type, "teleporter") {
				teleporterTiles = append(teleporterTiles, teleporterTileFx{tx: tileX, ty: tileY, color: tileData.FloorColor})
			}
			// Only use tile-specific floor colors for non-empty, non-inheriting tiles.
			if currentTile != world.TileEmpty && !inheritsFloor && tileData != nil {
				if colorConfig := tileData.FloorColor; colorConfig != [3]int{0, 0, 0} {
					baseColor = color.RGBA{uint8(colorConfig[0]), uint8(colorConfig[1]), uint8(colorConfig[2]), 255}
				}
			}
			// For TileEmpty, keep using defaultMapFloor (map-specific color).

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
			// Apply nearby tile effect to empty '.' tiles and explicit marker tiles.
			// Generic objects inherit the real surrounding floor without its tint.
			if nearSpecialTile && (currentTile == world.TileEmpty || markerInherit) {
				clr = nearTileColor
			}
			cache[[2]int{tileX, tileY}] = clr
		}
	}

	// The spawn tile and teleporters now inherit the biome floor (inherit_floor)
	// - no coloured-square override here. Their colour shows as a decoration
	// drawn over the floor (spawn border / teleporter glow). Cache the teleporter
	// tiles found during this per-map scan so the glow pass doesn't rescan.
	r.teleporterTiles = teleporterTiles

	r.floorColorCache = cache
	r.buildFloorColorMap(worldWidth, worldHeight)
}

// buildFloorColorMap encodes floorColorCache as a worldWxworldH RGBA8 image,
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
	group, ok := r.floorTexGroups[r.floorGroupLookupKey(tileX, tileY, groupName)]
	if !ok || group.count <= 0 {
		return 0, false
	}
	offset := stableFloorTextureIndex(tileX, tileY, int(tileType), group.count)
	return group.start + offset, true
}

// defaultFloorTextureGroup is the biome floor group used for any tile that
// doesn't name its own group - see floorTextureGroupForTile.
const defaultFloorTextureGroup = "default"

// floorTextureGroupForTile returns the floor-texture group name for a tile.
// Mapping is data-driven from tiles.yaml (TileData.FloorTextureGroup) with two
// fallbacks resolved here, not in the data:
//   - "beach": an "empty" tile bordering any water-group tile uses "beach"
//     instead of its own group, so shorelines transition into sand.
//   - objects without an authored floor inherit the dominant neighbouring
//     floor. Floor-only tiles without one use the biome's "default" group.
//     A floor-only marker can opt into inheritance with inherit_floor.
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
		if world.GlobalTileManager.InheritsFloor(tileType) {
			group = r.inheritedFloorTextureGroup(tileX, tileY, tileType)
			if group == "" {
				group = defaultFloorTextureGroup
			}
		} else if tileData.FloorColor != ([3]int{0, 0, 0}) {
			// No group + a set floor_color means the color IS the look
			// (trap, etc.) - leave it untextured.
			return ""
		} else {
			group = defaultFloorTextureGroup
		}
	}
	// Beach shoreline: any tile sitting on the biome's default ground that
	// borders water uses "beach" instead, so the sand transition is
	// continuous - including the ground under objects like palms, not just
	// bare empty tiles. Only when the current biome actually defines a
	// "beach" group (forest/desert), so city/church floors are unaffected.
	if group == defaultFloorTextureGroup && r.tileBordersWater(tileX, tileY) {
		if _, ok := r.floorTexGroups[r.floorGroupLookupKey(tileX, tileY, "beach")]; ok {
			return "beach"
		}
	}
	return group
}

func (r *Renderer) inheritedFloorTextureGroup(tileX, tileY int, tileType world.TileType3D) string {
	data := r.inheritedFloorTileData(tileX, tileY, tileType)
	if data == nil {
		return ""
	}
	return data.FloorTextureGroup
}

func (r *Renderer) inheritedFloorColor(tileX, tileY int, tileType world.TileType3D) [3]int {
	data := r.inheritedFloorTileData(tileX, tileY, tileType)
	if data == nil {
		return [3]int{0, 0, 0}
	}
	return data.FloorColor
}

// inheritedFloorTileData picks the floor an inherited marker or object blends
// into, using the same weighted dominant-neighbour vote as under-entity floors.
// This makes a prop in a multi-floor room take that room's dominant floor, not
// an arbitrary first neighbour.
func (r *Renderer) inheritedFloorTileData(tileX, tileY int, tileType world.TileType3D) *config.TileData {
	if r.game == nil || r.game.world == nil || world.GlobalTileManager == nil {
		return nil
	}
	t, ok := world.GlobalTileManager.DominantNeighbourFloorForTile(
		tileType, r.game.world.Tiles, r.game.world.Width, r.game.world.Height, tileX, tileY, nil)
	if !ok {
		return nil
	}
	return world.GlobalTileManager.GetTileData(t)
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
	// groups, so the atlas is cached per biome rather than per map file. The
	// unified world spans several biomes at once - its atlas combines them all
	// under "biome/group" keys (see floorGroupLookupKey).
	cacheKey := mapConfig.Biome
	groupSources := world.GlobalWorldManager.GetCurrentBiomeFloorTextureGroups()
	if r.game.openWorldActive() {
		cacheKey = world.OpenWorldKey
		groupSources = openWorldFloorTextureGroups()
	}
	if len(groupSources) == 0 {
		r.clearFloorAtlas()
		return
	}
	if cacheKey == r.floorTexturesKey && r.floorTexAtlas != nil {
		return // same biome (or same combined set), atlas already built
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
	r.floorTexturesKey = cacheKey
}

// openWorldFloorTextureGroups combines every merged region's biome floor
// groups under namespaced "biome/group" keys for the unified world's atlas.
func openWorldFloorTextureGroups() map[string][]string {
	wm := world.GlobalWorldManager
	out := make(map[string][]string)
	seen := make(map[string]bool)
	for _, region := range wm.OpenWorldRegions {
		mc, ok := wm.MapConfigs[region.MapKey]
		if !ok || seen[mc.Biome] {
			continue
		}
		seen[mc.Biome] = true
		biome, ok := wm.Biomes[mc.Biome]
		if !ok {
			continue
		}
		for group, texs := range biome.FloorTextureGroups {
			out[mc.Biome+"/"+group] = texs
		}
	}
	return out
}

// floorGroupLookupKey namespaces a floor group with the tile's region biome
// on the unified world; identity for split maps (single-biome atlas).
func (r *Renderer) floorGroupLookupKey(tileX, tileY int, group string) string {
	if group == "" || !r.game.openWorldActive() {
		return group
	}
	wm := world.GlobalWorldManager
	biome := wm.BiomeAtTile(tileX, tileY)
	if biome == "" {
		if mc := wm.GetCurrentMapConfig(); mc != nil {
			biome = mc.Biome
		}
	}
	return biome + "/" + group
}

func (r *Renderer) clearFloorAtlas() {
	r.floorTexAtlas = nil
	r.floorTexGroups = nil
	r.floorTexCount = 0
	r.floorTexTileW = 0
	r.floorTexTileH = 0
	r.floorTexMaxMip = 0
	r.floorTexturesKey = ""
}

// floorTextureGroupLoadOrder returns group names sorted alphabetically. Order
// only affects atlas layout (start offset per group), not visuals - sorting is
// purely for deterministic atlas placement across runs.
func floorTextureGroupLoadOrder(groups map[string][]string) []string {
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// maxFloorMipLevels caps the floor atlas mip chain; level 6 turns a 64px tile
// into 1px, far past where the per-pixel footprint stops growing usefully.
const maxFloorMipLevels = 6

// buildFloorTexAtlas packs the given source textures into a horizontal strip
// (tex[0] occupies x in [0, tileW), tex[1] in [tileW, 2*tileW), ...) plus a
// manual mip chain stacked below it: level k sits at y = tileH*2*(1-0.5^k)
// with tileW>>k wide cells. Kage samples nearest-only with no automatic mip
// selection, so minified floor rows shimmer while the camera moves unless the
// shader can blend pre-averaged levels chosen from the pixel's texel
// footprint. All textures must share dimensions - the caller pre-validates
// this so we never leave black slots in the atlas. The source slice is
// consumed here and not retained; the pixel data lives on the GPU once the
// atlas is built.
func (r *Renderer) buildFloorTexAtlas(textures []floorTexture) {
	if len(textures) == 0 {
		r.clearFloorAtlas()
		return
	}
	tileW := textures[0].width
	tileH := textures[0].height
	// Levels halve cleanly only while both dimensions stay even.
	maxMip := 0
	for w, h := tileW, tileH; w%2 == 0 && h%2 == 0 && maxMip < maxFloorMipLevels; w, h = w/2, h/2 {
		maxMip++
	}
	atlasH := tileH
	if maxMip > 0 {
		atlasH = tileH * 2
	}
	atlas := image.NewRGBA(image.Rect(0, 0, tileW*len(textures), atlasH))
	for i, tex := range textures {
		for y := 0; y < tileH; y++ {
			srcRow := tex.pixels[y*tileW*4 : (y+1)*tileW*4]
			dstStart := y*atlas.Stride + i*tileW*4
			copy(atlas.Pix[dstStart:dstStart+tileW*4], srcRow)
		}
		prev, pw, ph := tex.pixels, tileW, tileH
		yOff := tileH
		for level := 1; level <= maxMip; level++ {
			cur, cw, ch := boxHalve(prev, pw, ph)
			for y := 0; y < ch; y++ {
				srcRow := cur[y*cw*4 : (y+1)*cw*4]
				dstStart := (yOff+y)*atlas.Stride + i*cw*4
				copy(atlas.Pix[dstStart:dstStart+cw*4], srcRow)
			}
			prev, pw, ph = cur, cw, ch
			yOff += ch
		}
	}
	r.floorTexAtlas = ebiten.NewImageFromImage(atlas)
	r.floorTexCount = len(textures)
	r.floorTexTileW = tileW
	r.floorTexTileH = tileH
	r.floorTexMaxMip = maxMip
}

// boxHalve downsamples an RGBA buffer to half size by averaging each 2x2
// block - one mip level step. Averaging stays inside the tile, so every level
// tiles as seamlessly as the source.
func boxHalve(pix []byte, w, h int) ([]byte, int, int) {
	hw, hh := w/2, h/2
	out := make([]byte, hw*hh*4)
	for y := 0; y < hh; y++ {
		r0 := (y * 2) * w * 4
		r1 := (y*2 + 1) * w * 4
		for x := 0; x < hw; x++ {
			c0 := r0 + x*8
			c1 := r1 + x*8
			o := (y*hw + x) * 4
			for ch := 0; ch < 4; ch++ {
				sum := int(pix[c0+ch]) + int(pix[c0+4+ch]) + int(pix[c1+ch]) + int(pix[c1+4+ch])
				out[o+ch] = byte((sum + 2) / 4)
			}
		}
	}
	return out, hw, hh
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
	clear(r.renderedSpritesThisFrame)

	r.updateActiveLights()

	// Sky is independent of the opaque perspective-floor shader below.
	r.game.renderHelper.RenderSkyBackground(screen)

	// Clear depth buffer for this frame - optimized with slice header manipulation
	viewDist := r.game.camera.ViewDist
	depthBuf := r.game.depthBuffer
	wallTopBuf := r.game.wallTopBuffer
	for i := range depthBuf {
		depthBuf[i] = viewDist
		// Default wall top = 0 (screen top) = "occlude fully". This is the
		// fail-SAFE default: any opaque depth-writer that doesn't record a real
		// wall top leaves a sprite behind it fully clipped (the old behavior),
		// never drawn through. Only a floor-anchored wall lowers it to reveal a
		// taller sprite's canopy above the wall.
		if i < len(wallTopBuf) {
			wallTopBuf[i] = 0
		}
	}

	// Calculate ray parameters first
	numRays := r.computeNumRays()

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
	results := r.game.threading.ParallelRenderer.RenderRaycastInto(
		numRays,
		r.castRayWithPrecomputedDirectionInto,
	)
	raycastTimer.EndRaycast()

	// Draw simple floor and ceiling before walls/trees so trees are visible above floor
	r.statTreesDrawn, r.statStandeeCalls, r.statAuraTiles = 0, 0, 0
	r.game.threading.PerformanceMonitor.ProfiledFunction("sprite_render", func() {
		tf := time.Now()
		r.drawSimpleFloorCeiling(screen)
		r.statFloorMs = float64(time.Since(tf).Microseconds()) / 1000.0

		// Render the results and update depth buffer
		tw := time.Now()
		r.renderRaycastResults(screen, results)
		r.statWallsMs = float64(time.Since(tw).Microseconds()) / 1000.0

		// Draw all sprites (trees, ferns, monsters, NPCs) sorted by depth
		ts := time.Now()
		r.drawAllSpritesSorted(screen)
		r.statSpritesMs = float64(time.Since(ts).Microseconds()) / 1000.0

		// Highlight impassable billboard tiles with rising ground bubbles
		// (after walls/sprites so the depth buffer is populated for occlusion).
		r.drawImpassableTileAura(screen)
		// Grey smoke wreath around a sealed (dormant) boss - invulnerable until
		// its quest unseals it.
		r.drawSealedBossAura(screen)
		r.drawTrapTileBorders(screen)
		// Red bubble border around the player's start tile (floor inherited).
		r.drawSpawnTileBorder(screen)
		// Coloured glow filling every teleporter tile (floor inherited).
		r.drawTeleporterTileFx(screen)
		// Steam bubbles across every tile of an active Hot Steam zone.
		r.drawSteamZoneBubbles(screen)
		// Steam rising from every shut culvert valve's tile.
		r.drawClosedValveSteam(screen)

		// Draw fireballs and sword attacks
		r.drawProjectiles(screen)

		// Draw slash effects
		r.drawSlashEffects(screen)

		// Draw hit effects (spell particles, arrow bursts)
		r.drawHitEffects(screen)

		// Buff-cast overlay animation, centred in the party's view.
		r.drawBuffFx(screen)
	})
}

// RaycastHit contains the result of a DDA raycast operation.
// This follows the Digital Differential Analysis algorithm for efficient grid traversal.
type RaycastHit struct {
	Distance        float64          // Perpendicular distance to the wall (prevents fisheye effect)
	TileType        world.TileType3D // Type of tile that was hit
	WallSide        int              // 0 for north-south walls, 1 for east-west walls (used for shading)
	TextureCoord    float64          // Wall hit position for texture mapping (0.0 to 1.0)
	WallGridLine    float64          // X (side 0) or Y (side 1) grid line of the hit plane
	HasWallGridLine bool             // False only for synthetic/legacy hits without a DDA plane
	IsTransparent   bool             // Whether this hit should be rendered transparently
}

// wallRaySurfaceHitAtGridLine is the single wall-plane intersection used by
// DDA and by the distant wall rasterizer. surfaceCoord is deliberately NOT
// wrapped: pixel-boundary sampling needs its continuous texture interval, not
// two unrelated fractional coordinates at a tile seam. Coordinates are in
// tile/grid units, not world pixels.
func wallRaySurfaceHitAtGridLine(cameraX, cameraY, rayX, rayY float64, wallSide int, wallGridLine float64) (distance, surfaceCoord float64, mirrored, ok bool) {
	const epsilon = 1e-9
	switch wallSide {
	case 0:
		if math.Abs(rayX) < epsilon {
			return 0, 0, false, false
		}
		distance = (wallGridLine - cameraX) / rayX
		surfaceCoord = cameraY + distance*rayY
		mirrored = rayX > 0
	case 1:
		if math.Abs(rayY) < epsilon {
			return 0, 0, false, false
		}
		distance = (wallGridLine - cameraY) / rayY
		surfaceCoord = cameraX + distance*rayX
		mirrored = rayY < 0
	default:
		return 0, 0, false, false
	}
	if distance <= epsilon {
		return 0, 0, false, false
	}
	return distance, surfaceCoord, mirrored, true
}

// wallTextureCoordFromSurface reproduces the legacy facing/mirroring rule for
// a single centre ray. It is separate from the continuous helper below so
// regular DDA hits preserve their existing source-column choice exactly.
func wallTextureCoordFromSurface(surfaceCoord float64, mirrored bool) float64 {
	textureCoord := surfaceCoord - math.Floor(surfaceCoord)
	if mirrored {
		textureCoord = 1 - textureCoord
	}
	// The old integer-column path clamped an exact 1 to its final texel. Keep
	// that convention while making a seam-safe mesh coordinate for the far path.
	if textureCoord >= 1 {
		textureCoord = math.Nextafter(1, 0)
	}
	return textureCoord
}

// wallRayHitAtGridLine returns the legacy wrapped U used by DDA hits.
func wallRayHitAtGridLine(cameraX, cameraY, rayX, rayY float64, wallSide int, wallGridLine float64) (distance, textureCoord float64, ok bool) {
	distance, surfaceCoord, mirrored, ok := wallRaySurfaceHitAtGridLine(cameraX, cameraY, rayX, rayY, wallSide, wallGridLine)
	if !ok {
		return 0, 0, false
	}
	return distance, wallTextureCoordFromSurface(surfaceCoord, mirrored), true
}

// wallTextureIntervalFromSurface keeps a ray column's source U continuous
// relative to its left edge. Unlike a shortest-distance seam heuristic, this
// also remains correct when an oblique one-pixel column truly spans a large
// section of the wall texture.
func wallTextureIntervalFromSurface(left, right float64, mirrored bool) (float64, float64) {
	if mirrored {
		left, right = -left, -right
	}
	base := math.Floor(left)
	return left - base, right - base
}

// wallTextureUsesMipmappedSlice leaves close pixel art on the legacy nearest
// path. Once either source axis is minified, a quad with true source bounds
// lets Ebiten select a mip level instead of hopping between full-res columns.
func wallTextureUsesMipmappedSlice(textureWidth, textureHeight, screenWidth int, leftU, rightU, wallHeight float64) bool {
	if screenWidth <= 0 {
		screenWidth = 1
	}
	return math.Abs(rightU-leftU)*float64(textureWidth) > float64(screenWidth) || wallHeight < float64(textureHeight)*0.5
}

// MultiRaycastHit contains multiple hits for a single ray (for transparency support)
type MultiRaycastHit struct {
	Hits []RaycastHit
}

// castRayWithType casts a single ray and returns distance and hit information.
// Cold fallback path - passes nil so the raycast allocates its own slice; the
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

// castRayWithPrecomputedDirectionInto casts a single ray using precomputed
// sin/cos values and writes into reused result storage.
func (r *Renderer) castRayWithPrecomputedDirectionInto(rayIndex int, result *rendering.RaycastResult) {
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
		result.Distance, result.TileType = r.castRayWithType(angle)
		return
	}

	// Use precomputed ray directions instead of recalculating sin/cos
	rayDirectionX := r.rayDirectionsX[rayIndex]
	rayDirectionY := r.rayDirectionsY[rayIndex]

	// Reuse this ray's pre-allocated hit buffer (different rayIndex per worker
	// -> no race). Capacity is retained across frames; only first few frames
	// may grow the slice's backing.
	slot := &r.rayHitResults[rayIndex]
	buf := slot.Hits[:0]
	hits := r.performMultiHitRaycastWithDirection(rayDirectionX, rayDirectionY, buf)
	slot.Hits = hits.Hits
	result.TileType = slot
	// If there are no hits, it means the ray went into the void.
	if len(hits.Hits) == 0 {
		result.Distance = r.game.camera.ViewDist
		return
	}

	// The primary distance for depth sorting should be the first solid object hit.
	for _, hit := range hits.Hits {
		if !hit.IsTransparent {
			result.Distance = hit.Distance
			return
		}
	}

	// If no solid wall was hit, return the distance of the closest transparent object.
	result.Distance = hits.Hits[0].Distance
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
	// One world resolve per ray, not per step: GetCurrentWorld is a string-map
	// lookup and the world cannot change mid-frame.
	currentWorld := r.game.GetCurrentWorld()

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
		tileType := currentWorld.GetTileAtGrid(currentTileX, currentTileY)

		// If we hit an empty tile, we can just continue
		if tileType == world.TileEmpty {
			continue
		}

		// Crossed-standee tree mode: tree tiles don't block the ray and aren't
		// drawn per-column here - they render as two crossed standees in the
		// sprite pass (drawCrossedTreeStandees), so the forest shows through the
		// gaps between the planes. Skip the tile entirely.
		if r.game.config.Graphics.TreesAsBillboards && world.GlobalTileManager != nil &&
			world.GlobalTileManager.GetRenderType(tileType) == "tree_sprite" {
			continue
		}

		// The DDA tells us the exact grid line we crossed. Preserve it in the
		// hit so the renderer can reconstruct the two pixel-boundary rays from
		// this same plane for mipmapped wall texture sampling.
		wallGridLine := float64(currentTileX)
		if wallSide == 0 {
			if stepDirectionX < 0 {
				wallGridLine++
			}
		} else {
			wallGridLine = float64(currentTileY)
			if stepDirectionY < 0 {
				wallGridLine++
			}
		}
		perpendicularDistance, textureCoordinate, ok := wallRayHitAtGridLine(
			startWorldX/tileSize, startWorldY/tileSize,
			rayDirectionX, rayDirectionY, wallSide, wallGridLine)
		if !ok {
			continue
		}

		// If distance is too far, stop here.
		if perpendicularDistance*tileSize > r.game.camera.ViewDist {
			return MultiRaycastHit{Hits: hits}
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
				Distance:        perpendicularDistance * tileSize,
				TileType:        tileType,
				WallSide:        wallSide,
				TextureCoord:    textureCoordinate,
				WallGridLine:    wallGridLine,
				HasWallGridLine: true,
				IsTransparent:   true,
			})
		} else {
			// Solid tile: add hit and stop ray
			hits = append(hits, RaycastHit{
				Distance:        perpendicularDistance * tileSize,
				TileType:        tileType,
				WallSide:        wallSide,
				TextureCoord:    textureCoordinate,
				WallGridLine:    wallGridLine,
				HasWallGridLine: true,
				IsTransparent:   false,
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
// writeWallColumns records an opaque wall hit across its screen columns into
// BOTH the depth buffer and the wall-top buffer (screen-Y of the wall's top), so
// a tall sprite (tree standee) behind this wall clips at the wall's top edge and
// still renders the canopy that rises above it. This is the single place an
// opaque wall's occlusion is recorded - keeping both writes together prevents
// the buffers from drifting out of sync.
func (r *Renderer) writeWallColumns(screenX, width int, distance float64, tileType world.TileType3D) {
	_, wallTop := r.game.renderHelper.CalculateWallDimensionsWithHeight(distance, world.GetTileHeight(tileType))
	for dx := 0; dx < width; dx++ {
		x := screenX + dx
		if x >= 0 && x < len(r.game.depthBuffer) {
			r.game.depthBuffer[x] = distance
			r.game.wallTopBuffer[x] = wallTop
		}
	}
}

func (r *Renderer) renderRaycastResults(screen *ebiten.Image, results []rendering.RaycastResult) {
	rayWidth := r.game.config.Graphics.RaysPerScreenWidth
	screenWidth := r.game.config.GetScreenWidth()
	r.wallMipBatchSource = nil
	r.wallMipBatchVerts = r.wallMipBatchVerts[:0]
	r.wallMipBatchIndices = r.wallMipBatchIndices[:0]

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
		case *MultiRaycastHit:
			if hitData != nil {
				r.renderRaycastHitStack(screen, screenX, currentRayWidth, hitData.Hits)
			}
		case MultiRaycastHit:
			r.renderRaycastHitStack(screen, screenX, currentRayWidth, hitData.Hits)
		case RaycastHit:
			// This case should ideally not be hit with the new system, but as a fallback:
			hitInfo := hitData

			// Record depth + wall-top for proper sprite occlusion.
			r.writeWallColumns(screenX, currentRayWidth, rayResult.Distance, hitInfo.TileType)

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
	r.flushMipmappedWallBatch(screen)
}

func (r *Renderer) renderRaycastHitStack(screen *ebiten.Image, screenX, width int, hits []RaycastHit) {
	// Render all hits from back to front for proper transparency.
	for i := len(hits) - 1; i >= 0; i-- {
		hit := hits[i]

		// Record depth + wall-top for solid objects (tall sprites clip on the
		// wall's top edge so their canopy shows above shorter walls).
		if !hit.IsTransparent {
			r.writeWallColumns(screenX, width, hit.Distance, hit.TileType)
		}

		// Collect tree hits for later sorted rendering.
		if world.GlobalTileManager != nil && world.GlobalTileManager.GetRenderType(hit.TileType) == "tree_sprite" {
			r.treeHits = append(r.treeHits, treeHitData{
				screenX:  screenX,
				distance: hit.Distance,
				tileType: hit.TileType,
			})
			continue
		}

		r.renderSingleHit(screen, screenX, hit, width)
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
			r.flushMipmappedWallBatch(screen)
			r.drawTreeSprite(screen, screenX, hit.Distance, tileType)
		case "environment_sprite", "landmark":
			// Skip transparent environment sprites in raycasting - they'll be rendered in sprite phase
			// Use both hit.IsTransparent flag and tile manager check for safety
			if hit.IsTransparent {
				return // Skip transparent environment sprites - rendered in unified sprite pass
			}
			r.flushMipmappedWallBatch(screen)
			r.drawEnvironmentSpriteOnce(screen, screenX, hit.Distance, tileType)
		case "textured_wall":
			r.drawTexturedWallSlice(screen, screenX, hit.Distance, tileType, rayWidth,
				hit.WallSide, hit.TextureCoord, hit.WallGridLine, hit.HasWallGridLine, !hit.IsTransparent)
		case "floor_only":
			// Floor-only tiles don't render anything here, just floor
			// These should be transparent so rays continue through them
			return
		}
	} else {
		// If tile manager not available, render as textured wall by default
		r.drawTexturedWallSlice(screen, screenX, hit.Distance, tileType, rayWidth,
			hit.WallSide, hit.TextureCoord, hit.WallGridLine, hit.HasWallGridLine, !hit.IsTransparent)
	}
}

// maxFloorShaderLights must match the Lights array length in floorShaderSrc.
// 16 starved torch-lined maps (distant pools popped in only at point-blank);
// the shader's squared-distance early-out makes 32 affordable.
const maxFloorShaderLights = 32

// drawSimpleFloorCeiling renders the perspective floor entirely on the GPU
// via a Kage shader (see floorShaderSrc). Per-fragment work: reverse-project
// screen -> world -> tile, look up base color, optionally blend a hash-selected
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
		r.game.renderHelper.DrawGroundFallback(screen)
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
	if len(r.canopyShadeFactors) == 0 ||
		r.canopyShadeW != worldW ||
		r.canopyShadeH != worldH {
		r.buildCanopyShadeCache()
	}

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

	// Reuse the uniforms map and its nested slices across frames - rebuilding
	// a ~20-key map[string]any (plus five small slices) every frame was pure
	// allocator churn in the hottest draw call.
	if r.floorUniforms == nil {
		r.floorUniforms = map[string]any{
			"CamPos":        make([]float32, 2),
			"ScreenSize":    make([]float32, 2),
			"WorldSize":     make([]float32, 2),
			"TexTileSize":   make([]float32, 2),
			"DirCos":        make([]float32, 1),
			"DirSin":        make([]float32, 1),
			"PlaneCos":      make([]float32, 1),
			"PlaneSin":      make([]float32, 1),
			"Horizon":       make([]float32, 1),
			"RowDistFactor": make([]float32, 1),
			"TileSize":      make([]float32, 1),
			"ViewDist":      make([]float32, 1),
			"MinBrightness": make([]float32, 1),
			"Ambient":       make([]float32, 1),
			"ViewerAmbient": make([]float32, 1),
			"TexCount":      make([]float32, 1),
			"MaxMip":        make([]float32, 1),
			"LightCount":    make([]float32, 1),
			"Lights":        r.floorLightsBuf[:],
		}
	}
	uniforms := r.floorUniforms
	setVec2 := func(key string, a, b float32) {
		v := uniforms[key].([]float32)
		v[0], v[1] = a, b
	}
	setFloat := func(key string, value float32) {
		uniforms[key].([]float32)[0] = value
	}
	setVec2("CamPos", float32(camX), float32(camY))
	setVec2("ScreenSize", float32(screenWidth), float32(screenHeight))
	setVec2("WorldSize", float32(worldW), float32(worldH))
	setVec2("TexTileSize", float32(r.floorTexTileW), float32(r.floorTexTileH))
	setFloat("DirCos", float32(cosA))
	setFloat("DirSin", float32(sinA))
	setFloat("PlaneCos", float32(planeX))
	setFloat("PlaneSin", float32(planeY))
	setFloat("Horizon", float32(horizon))
	setFloat("RowDistFactor", float32(0.5*float64(screenHeight)*float64(tileSize)))
	setFloat("TileSize", float32(tileSize))
	setFloat("ViewDist", float32(r.game.camera.ViewDist))
	setFloat("MinBrightness", float32(r.game.config.Graphics.BrightnessMin))
	ambient := r.ambientLight
	if ambient <= 0 {
		ambient = 1
	}
	setFloat("Ambient", float32(ambient))
	setFloat("ViewerAmbient", float32(r.viewerAmbient()))
	setFloat("TexCount", float32(r.floorTexCount))
	setFloat("MaxMip", float32(r.floorTexMaxMip))
	setFloat("LightCount", float32(lightCount))

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

	// Division guard only (collision keeps the camera farther out). A larger
	// clamp freezes the projection for near rays and creases against the
	// still-perspective far ones - same fix as walls.
	if distance < 1.0 {
		distance = 1.0
	}

	// Calculate tree height and position
	// distance is already perpendicular distance from the raycast
	sizeTiles := r.game.config.Graphics.Sprite.TreeHeightMultiplier
	if world.GlobalTileManager != nil {
		sizeTiles = world.GlobalTileManager.GetSizeTiles(tileType)
	}
	spriteHeight := r.game.renderHelper.calculateSpriteSizeWithHeightMultiplier(distance, sizeTiles)
	if spriteHeight < 8 {
		spriteHeight = 8
	}

	// Sanity bound, reachable only inside the epsilon above; the GPU clips
	// off-screen geometry, so huge heights cost nothing.
	if spriteHeight > screenHeight*64 {
		spriteHeight = screenHeight * 64
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

	scaleX := float64(spriteWidth) / float64(sprite.Bounds().Dx())
	scaleY := float64(spriteHeight) / float64(sprite.Bounds().Dy())
	opts := r.scaledWorldSpriteOpts(scaleX, scaleY)
	opts.GeoM.Translate(float64(spriteLeft), float64(spriteTop))

	// Light the tree at its own column world point (not the camera), so torches
	// and spell glows reach it like any other raycast surface.
	brightness := r.wallPointBrightness(x, distance)
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
	sizeTiles := r.game.config.Graphics.Sprite.TreeHeightMultiplier
	if world.GlobalTileManager != nil {
		sizeTiles = world.GlobalTileManager.GetSizeTiles(tileType)
	}
	spriteHeight := r.game.renderHelper.calculateSpriteSizeWithHeightMultiplier(distance, sizeTiles)
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
				// Opaque billboard: fully occlude a standee behind it (wall top 0).
				// These sprites are screen-CENTERED, not floor-anchored, so their
				// top is not a valid clip line; full-occlude matches the prior
				// behavior and overrides any stale wall top from this column.
				if px < len(r.game.wallTopBuffer) {
					r.game.wallTopBuffer[px] = 0
				}
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

	scaleX := float64(spriteWidth) / float64(sprite.Bounds().Dx())
	scaleY := float64(spriteHeight) / float64(sprite.Bounds().Dy())
	opts := r.scaledWorldSpriteOpts(scaleX, scaleY)
	opts.GeoM.Translate(float64(spriteLeft), float64(spriteTop))

	// Distance shading, light-aware: torch / spell glow reaches raycast sprites
	// (and the map's ambient_light darkens them).
	brightness := r.wallPointBrightness(x, distance)
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
func (r *Renderer) drawTexturedWallSlice(screen *ebiten.Image, screenX int, distance float64, tileType world.TileType3D, width, wallSide int, textureCoord, wallGridLine float64, hasWallGridLine, queueMipmapped bool) {
	heightMultiplier := world.GetTileHeight(tileType)

	// Sprite-textured walls bypass the WallSliceCache: every ray has its own
	// continuous textureCoord (float), so cache keys never collide and caching
	// would just allocate one new image per ray. Draw the strip directly.
	if world.GlobalTileManager != nil {
		if spriteName := world.GlobalTileManager.GetSprite(tileType); spriteName != "" {
			sprite := r.game.sprites.GetSprite(spriteName)
			if sprite != nil {
				r.drawSpriteTexturedWallSlice(screen, sprite, screenX, width, wallSide, textureCoord,
					distance, heightMultiplier, wallGridLine, hasWallGridLine, queueMipmapped)
				return
			}
		}
	}

	// Cached path for procedural / color-only walls. Discrete TileType + integer
	// width/height/side/wallX make cache hits useful here.
	wallHeight, wallTop := r.game.renderHelper.CalculateWallDimensionsWithHeight(distance, heightMultiplier)
	cacheKey := rendering.WallSliceKey{
		Height:   wallHeight,
		Width:    width,
		TileType: int(tileType),
		Side:     wallSide,
	}

	wallSliceImage := r.game.threading.WallSliceCache.GetOrCreate(cacheKey, func(quantizedHeight int) *ebiten.Image {
		return r.game.renderHelper.CreateBaseTexturedWallSlice(tileType, width, quantizedHeight, wallSide)
	})

	drawOptions := r.sharedDrawOpts()
	cachedHeight := wallSliceImage.Bounds().Dy()
	if cachedHeight > 0 && wallHeight != cachedHeight {
		scaleY := float64(wallHeight) / float64(cachedHeight)
		drawOptions.GeoM.Scale(1.0, scaleY)
	}
	drawOptions.GeoM.Translate(float64(screenX), float64(wallTop))

	// Distance-based shading at draw time (cache stays brightness-agnostic),
	// light-aware so torches land on walls - vital on dark (ambient_light) maps.
	brightness := r.wallPointBrightness(screenX, distance)
	drawOptions.ColorScale.Scale(float32(brightness), float32(brightness), float32(brightness), 1.0)

	r.flushMipmappedWallBatch(screen)
	screen.DrawImage(wallSliceImage, drawOptions)
}

// drawSpriteTexturedWallSlice keeps close pixel-art walls on the original
// one-source-column path. At range, one screen column covers several texture
// texels: then it instead maps the two ray boundaries to a mesh quad, exposing
// the source footprint required for linear mipmap filtering.
func (r *Renderer) drawSpriteTexturedWallSlice(screen *ebiten.Image, sprite *ebiten.Image, screenX, width, wallSide int, textureCoord, distance, heightMultiplier, wallGridLine float64, hasWallGridLine, queueMipmapped bool) {
	spriteBounds := sprite.Bounds()
	spriteWidth := spriteBounds.Dx()
	spriteHeight := spriteBounds.Dy()
	if spriteWidth <= 0 || spriteHeight <= 0 || width <= 0 {
		return
	}
	wallHeightF, floorBottomF := r.game.renderHelper.CalculateWallDimensionsWithHeightF(distance, heightMultiplier)
	if wallHeightF <= 0 {
		return
	}

	if hasWallGridLine {
		leftU, rightU, ok := r.wallTextureCoordsAtSliceBoundaries(screenX, wallSide, wallGridLine)
		if ok {
			if wallTextureUsesMipmappedSlice(spriteWidth, spriteHeight, width, leftU, rightU, wallHeightF) {
				if queueMipmapped && r.queueMipmappedSpriteWallSlice(screen, sprite, screenX, width, wallSide, distance,
					floorBottomF-wallHeightF, wallHeightF, leftU, rightU) {
					return
				}
				r.flushMipmappedWallBatch(screen)
				r.drawMipmappedSpriteWallSlice(screen, sprite, screenX, width, wallSide, distance,
					floorBottomF-wallHeightF, wallHeightF, leftU, rightU)
				return
			}
		}
	}

	// Close walls deliberately retain the former nearest-column behavior: it
	// keeps their authored pixel art sharp and avoids changing their look.
	wallHeight := int(wallHeightF)
	wallTop := int(floorBottomF) - wallHeight
	r.flushMipmappedWallBatch(screen)
	r.drawNearestSpriteWallSlice(screen, sprite, screenX, wallTop, wallHeight, width, wallSide, textureCoord, distance)
}

// wallTextureCoordsAtSliceBoundaries reconstructs the two ray directions for
// the logical ray column at screenX. The final physical screen slice can be
// narrower after a resize, but its ray still spans one full logical interval.
func (r *Renderer) wallTextureCoordsAtSliceBoundaries(screenX, wallSide int, wallGridLine float64) (leftU, rightU float64, ok bool) {
	if r.game == nil || r.game.camera == nil {
		return 0, 0, false
	}
	rayWidth := r.game.config.Graphics.RaysPerScreenWidth
	if rayWidth <= 0 {
		rayWidth = 1
	}
	numRays := len(r.rayDirectionsX)
	if numRays <= 0 {
		numRays = r.computeNumRays()
	}
	if numRays <= 0 {
		return 0, 0, false
	}
	rayIndex := screenX / rayWidth
	rayAtEdge := func(edge float64) (float64, float64) {
		cameraX := 2*edge/float64(numRays) - 1
		return r.rayDirX + r.rayPlaneX*cameraX, r.rayDirY + r.rayPlaneY*cameraX
	}
	tileSize := r.game.config.GetTileSize()
	camX := r.game.camera.X / tileSize
	camY := r.game.camera.Y / tileSize
	leftX, leftY := rayAtEdge(float64(rayIndex))
	rightX, rightY := rayAtEdge(float64(rayIndex + 1))
	_, leftSurface, leftMirrored, leftOK := wallRaySurfaceHitAtGridLine(camX, camY, leftX, leftY, wallSide, wallGridLine)
	_, rightSurface, rightMirrored, rightOK := wallRaySurfaceHitAtGridLine(camX, camY, rightX, rightY, wallSide, wallGridLine)
	if !leftOK || !rightOK || leftMirrored != rightMirrored {
		return 0, 0, false
	}
	leftU, rightU = wallTextureIntervalFromSurface(leftSurface, rightSurface, leftMirrored)
	return leftU, rightU, true
}

// drawNearestSpriteWallSlice is the original close-range path: one source
// column per ray, stretched across that ray's logical screen width.
func (r *Renderer) drawNearestSpriteWallSlice(screen *ebiten.Image, sprite *ebiten.Image, screenX, wallTop, wallHeight, width, wallSide int, textureCoord, distance float64) {
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

	brightness := r.wallPointBrightness(screenX, distance)
	if wallSide == 1 {
		brightness *= 0.7
	}

	src := r.spriteColumn(sprite, textureX, spriteWidth, spriteHeight)
	opts := r.sharedDrawOpts()
	opts.GeoM.Scale(xScale, yScale)
	opts.GeoM.Translate(float64(screenX), float64(wallTop))
	opts.ColorScale.Scale(float32(brightness), float32(brightness), float32(brightness), 1.0)
	screen.DrawImage(src, opts)
}

// drawMipmappedSpriteWallSlice is the direct fallback for a transparent wall:
// those hits must remain in the ray's back-to-front order. Opaque minified
// walls use queueMipmappedSpriteWallSlice instead.
func (r *Renderer) drawMipmappedSpriteWallSlice(screen *ebiten.Image, sprite *ebiten.Image, screenX, width, wallSide int, distance, wallTop, wallHeight, leftU, rightU float64) {
	repeated, texW, texH, ok := r.mipmappedWallTexture(sprite)
	if !ok {
		return
	}
	brightness := r.wallPointBrightness(screenX, distance)
	if wallSide == 1 {
		brightness *= 0.7
	}
	vertices := appendMipmappedWallSliceVertices(r.wallSliceVerts[:0], texW, texH, screenX, width, wallTop, wallHeight, leftU, rightU, brightness)
	r.drawMipmappedWallTriangles(screen, vertices, wallSliceTriangleIndices[:], repeated)
}

var wallSliceTriangleIndices = [...]uint16{0, 1, 2, 1, 3, 2}

const wallMipBatchVertexLimit = 1 << 16

// queueMipmappedSpriteWallSlice gathers independent opaque ray slices that
// share the same repeated source. Each rectangle retains its original four
// vertices, so batching changes submission cost only, not wall geometry.
func (r *Renderer) queueMipmappedSpriteWallSlice(screen *ebiten.Image, sprite *ebiten.Image, screenX, width, wallSide int, distance, wallTop, wallHeight, leftU, rightU float64) bool {
	repeated, texW, texH, ok := r.mipmappedWallTexture(sprite)
	if !ok {
		return false
	}
	if r.wallMipBatchSource != nil && (r.wallMipBatchSource != repeated || len(r.wallMipBatchVerts)+4 > wallMipBatchVertexLimit) {
		r.flushMipmappedWallBatch(screen)
	}
	if r.wallMipBatchSource == nil {
		r.wallMipBatchSource = repeated
	}
	brightness := r.wallPointBrightness(screenX, distance)
	if wallSide == 1 {
		brightness *= 0.7
	}
	base := uint16(len(r.wallMipBatchVerts))
	r.wallMipBatchVerts = appendMipmappedWallSliceVertices(r.wallMipBatchVerts, texW, texH, screenX, width, wallTop, wallHeight, leftU, rightU, brightness)
	r.wallMipBatchIndices = append(r.wallMipBatchIndices, base, base+1, base+2, base+1, base+3, base+2)
	return true
}

func (r *Renderer) flushMipmappedWallBatch(screen *ebiten.Image) {
	if len(r.wallMipBatchVerts) == 0 {
		r.wallMipBatchSource = nil
		return
	}
	r.drawMipmappedWallTriangles(screen, r.wallMipBatchVerts, r.wallMipBatchIndices, r.wallMipBatchSource)
	r.wallMipBatchSource = nil
	r.wallMipBatchVerts = r.wallMipBatchVerts[:0]
	r.wallMipBatchIndices = r.wallMipBatchIndices[:0]
}

// mipmappedWallTexture returns a tileable source and its original tile bounds.
// Mipmaps are built from the repeated source, while U coordinates remain in
// original-tile units so every wall slice uses the same projection contract.
func (r *Renderer) mipmappedWallTexture(sprite *ebiten.Image) (repeated *ebiten.Image, textureWidth, textureHeight float64, ok bool) {
	repeated = r.repeatedWallTexture(sprite)
	if repeated == nil {
		return nil, 0, 0, false
	}
	bounds := sprite.Bounds()
	textureWidth = float64(bounds.Dx())
	textureHeight = float64(bounds.Dy())
	if textureWidth <= 0 || textureHeight <= 0 {
		return nil, 0, 0, false
	}
	return repeated, textureWidth, textureHeight, true
}

// appendMipmappedWallSliceVertices emits the same axis-aligned source-mapped
// rectangle used by the direct path. Keeping this shared makes batch and
// direct rendering pixel-equivalent.
func appendMipmappedWallSliceVertices(vertices []ebiten.Vertex, textureWidth, textureHeight float64, screenX, width int, wallTop, wallHeight, leftU, rightU, brightness float64) []ebiten.Vertex {
	leftSourceX := float32(textureWidth + leftU*textureWidth + 0.5)
	rightSourceX := float32(textureWidth + rightU*textureWidth + 0.5)
	bottomSourceY := float32(textureHeight - 0.5)
	color := float32(brightness)
	return append(vertices,
		ebiten.Vertex{DstX: float32(screenX), DstY: float32(wallTop), SrcX: leftSourceX, SrcY: 0.5, ColorR: color, ColorG: color, ColorB: color, ColorA: 1},
		ebiten.Vertex{DstX: float32(screenX + width), DstY: float32(wallTop), SrcX: rightSourceX, SrcY: 0.5, ColorR: color, ColorG: color, ColorB: color, ColorA: 1},
		ebiten.Vertex{DstX: float32(screenX), DstY: float32(wallTop + wallHeight), SrcX: leftSourceX, SrcY: bottomSourceY, ColorR: color, ColorG: color, ColorB: color, ColorA: 1},
		ebiten.Vertex{DstX: float32(screenX + width), DstY: float32(wallTop + wallHeight), SrcX: rightSourceX, SrcY: bottomSourceY, ColorR: color, ColorG: color, ColorB: color, ColorA: 1},
	)
}

func (r *Renderer) drawMipmappedWallTriangles(screen *ebiten.Image, vertices []ebiten.Vertex, indices []uint16, source *ebiten.Image) {
	r.wallSliceTriOpts = ebiten.DrawTrianglesOptions{
		Blend:  ebiten.BlendSourceOver,
		Filter: ebiten.FilterLinear,
	}
	screen.DrawTriangles(vertices, indices, source, &r.wallSliceTriOpts)
}

func (r *Renderer) repeatedWallTexture(sprite *ebiten.Image) *ebiten.Image {
	if cached := r.wallTextureRepeats[sprite]; cached != nil {
		return cached
	}
	bounds := sprite.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil
	}
	repeated := ebiten.NewImage(width*3, height)
	for copyIndex := 0; copyIndex < 3; copyIndex++ {
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Translate(float64(copyIndex*width), 0)
		repeated.DrawImage(sprite, opts)
	}
	if r.wallTextureRepeats == nil {
		r.wallTextureRepeats = make(map[*ebiten.Image]*ebiten.Image)
	}
	r.wallTextureRepeats[sprite] = repeated
	return repeated
}

// spriteColumn returns the cached 1px-wide column SubImage of a wall sprite.
// textureX is one of spriteWidth discrete values, so columns are built lazily
// once per sprite instead of allocating a SubImage per wall column per frame.
func (r *Renderer) spriteColumn(sprite *ebiten.Image, textureX, spriteWidth, spriteHeight int) *ebiten.Image {
	cols := r.wallSliceColumns[sprite]
	if cols == nil {
		if r.wallSliceColumns == nil {
			r.wallSliceColumns = make(map[*ebiten.Image][]*ebiten.Image)
		}
		cols = make([]*ebiten.Image, spriteWidth)
		r.wallSliceColumns[sprite] = cols
	}
	if cols[textureX] == nil {
		cols[textureX] = sprite.SubImage(image.Rect(textureX, 0, textureX+1, spriteHeight)).(*ebiten.Image)
	}
	return cols[textureX]
}

// spellFxMinClusterSize is the floor (in screen px) for a spell projectile's
// particle-cluster size, so distant/small bolts still render as a recognizable
// puff instead of a single dot. Close bolts are far larger and unaffected.
const spellFxMinClusterSize = 10.0

type projectileFxProfile struct {
	glowColor        [3]int
	trailColor       [3]int
	glowScale        float64
	trailLengthScale float64
	trailWidthScale  float64
	pulseSpeed       float64
	spark            bool
	sparkColor       [3]int
	// style selects the procedural pixel-particle body/trail: "ember" (fire -
	// rising flame motes), "shard" (water/ice - crisp falling crystals), or ""
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
		// Signature spells override the school default with a bespoke body
		// renderer (graphics.projectile_fx -> spellFxStyleDraw).
		if def.Graphics != nil && def.Graphics.ProjectileFx != "" {
			profile.style = def.Graphics.ProjectileFx
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
		// A projectile_school turns the shot into a glowing spell-ORB (not a plain
		// arrow), tinted to its magic element - staves/books fire magic charges,
		// not arrows. The "arcane" style name is just the orb body renderer
		// (pixel-particle, mirrored R->L), independent of the element.
		if school := strings.ToLower(weaponDef.ProjectileSchool); school != "" {
			c, ok := ElementColors[school]
			if !ok {
				c = ElementColors["arcane"]
			}
			profile.glowColor = mixColor(c, [3]int{255, 255, 255}, 0.25)
			profile.trailColor = mixColor(c, [3]int{255, 255, 255}, 0.45)
			profile.sparkColor = mixColor(c, [3]int{255, 255, 255}, 0.55)
			profile.spark = true
			profile.style = "arcane"
		}
	}
	return profile
}

// additiveGlowBlend is the standard additive blend used for all glow/particle
// effects (projectiles, arrows, slashes, spell hits, the impassable-tile aura):
// src-srcAlpha + dst, so overlapping glows accumulate into brighter light.
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

func (r *Renderer) projectileScreenDir(vx, vy float64) (float64, bool) {
	if vx == 0 && vy == 0 {
		return 0, false
	}
	camRightX := -math.Sin(r.game.camera.Angle)
	camRightY := math.Cos(r.game.camera.Angle)
	right := vx*camRightX + vy*camRightY
	if math.Abs(right) < 0.01 {
		return 0, false
	}
	dirX := math.Copysign(1, right)
	return dirX, true
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
	// A striking monster with a dedicated attack sheet plays it as a one-shot
	// over the strike window; monsters without one fall through to the walk
	// cycle (its AttackAnimFrames branch still reads as a brief lunge).
	if mon.AttackAnimFrames > 0 {
		if anim, flip := r.getMonsterDirectionalAnimation(spriteName, mon, "attacking"); anim != nil && len(anim.Frames) > 0 {
			return r.attackAnimFrameImage(anim, mon), flip
		}
	}
	anim, flip := r.getMonsterDirectionalAnimation(spriteName, mon, "walking")
	if anim != nil && len(anim.Frames) > 0 {
		return r.monsterAnimFrameImage(anim, mon), flip
	}
	return r.game.sprites.GetSprite(spriteName), false
}

// attackAnimFrameImage sweeps an attack animation ONCE across the strike window:
// AttackAnimFrames counts down from MonsterAttackAnimFrames to 0, mapped to
// frames 0..n-1. Unlike the free-running walk cycle, the strike plays start to
// finish so a wind-up/release reads correctly.
func (r *Renderer) attackAnimFrameImage(anim *graphics.SpriteAnimation, mon *monster.Monster3D) *ebiten.Image {
	n := len(anim.Frames)
	if n <= 1 {
		return anim.Frames[0]
	}
	total := int64(MonsterAttackAnimFrames)
	if total < 1 {
		total = 1
	}
	elapsed := total - int64(mon.AttackAnimFrames)
	if elapsed < 0 {
		elapsed = 0
	}
	idx := int(elapsed * int64(n) / total)
	if idx >= n {
		idx = n - 1
	}
	return anim.Frames[idx]
}

// monsterAnimFrameImage picks the animation frame for the monster's current
// motion state: cycling while it moves (and briefly after a TB step), the rest
// pose otherwise.
func (r *Renderer) monsterAnimFrameImage(anim *graphics.SpriteAnimation, mon *monster.Monster3D) *ebiten.Image {
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
	// Cycle while moving, while striking (both modes set AttackAnimFrames at
	// the attack moment - otherwise attackers froze on the rest pose), or
	// briefly after a TB step.
	cycling := r.shouldAnimateMonster(mon) ||
		mon.AttackAnimFrames > 0 ||
		(r.game.turnBasedMode && mon.LastMoveTick > 0 && r.game.frameCount-mon.LastMoveTick <= animWindow)
	if cycling {
		return anim.Frames[int((r.game.frameCount/int64(ticksPerFrame))%int64(len(anim.Frames)))]
	}
	return anim.Frames[0]
}

// getMonsterStandeeSprite returns the current walk frame from a fixed,
// world-deterministic animation set plus whether that art is drawn facing
// left. Picking walking_l vs walking_r by screen slide is the billboard
// path's trick; a standee uses ONE art set and mirrors by world heading,
// otherwise the two independent flips combine into backwards walking.
func (r *Renderer) getMonsterStandeeSprite(mon *monster.Monster3D) (*ebiten.Image, bool) {
	name := mon.GetSpriteType()
	// A striking monster with an attack sheet sweeps it once over the strike;
	// otherwise the walk set (mirrored by world heading upstream).
	if mon.AttackAnimFrames > 0 {
		if anim := r.game.sprites.GetAnimation(name, "attacking_r"); anim != nil && len(anim.Frames) > 0 {
			return r.attackAnimFrameImage(anim, mon), false
		}
		if anim := r.game.sprites.GetAnimation(name, "attacking_l"); anim != nil && len(anim.Frames) > 0 {
			return r.attackAnimFrameImage(anim, mon), true
		}
	}
	if anim := r.game.sprites.GetAnimation(name, "walking_r"); anim != nil && len(anim.Frames) > 0 {
		return r.monsterAnimFrameImage(anim, mon), false
	}
	if anim := r.game.sprites.GetAnimation(name, "walking_l"); anim != nil && len(anim.Frames) > 0 {
		return r.monsterAnimFrameImage(anim, mon), true
	}
	return r.game.sprites.GetSprite(name), false
}

// standeeMirrorFor decides whether a standee's texture must be mirrored so the
// depicted creature faces its walk direction on screen: it compares the token
// plane's on-screen U direction (sDot) with the heading's on-screen direction
// (dDot). decisive=false while the heading points at/away from the camera
// (|dDot| small) - the caller keeps the previous mirror so it can't flicker
// mid-charge.
func standeeMirrorFor(camAngle, standeeYaw, direction float64, artFacesLeft bool) (mirror, decisive bool) {
	camRightX := -math.Sin(camAngle)
	camRightY := math.Cos(camAngle)
	sDot := math.Cos(standeeYaw)*camRightX + math.Sin(standeeYaw)*camRightY
	dDot := math.Cos(direction)*camRightX + math.Sin(direction)*camRightY
	if math.Abs(dDot) <= 0.1 {
		return false, false
	}
	return ((sDot > 0) != (dDot > 0)) != artFacesLeft, true
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

// getMonsterDirectionalAnimation returns the "<animType>_r"/"<animType>_l"
// animation (walking, attacking, ...) picked by which way the monster slides on
// screen, plus whether the art must be mirrored. Falls back to either direction
// when the monster has no clear left/right on-screen motion.
func (r *Renderer) getMonsterDirectionalAnimation(spriteName string, mon *monster.Monster3D, animType string) (*graphics.SpriteAnimation, bool) {
	rKey, lKey := animType+"_r", animType+"_l"
	if dir, ok := r.monsterScreenDir(mon); ok {
		if dir > 0 {
			if anim := r.game.sprites.GetAnimation(spriteName, rKey); anim != nil && len(anim.Frames) > 0 {
				return anim, false
			}
			if anim := r.game.sprites.GetAnimation(spriteName, lKey); anim != nil && len(anim.Frames) > 0 {
				return anim, true
			}
		} else {
			if anim := r.game.sprites.GetAnimation(spriteName, lKey); anim != nil && len(anim.Frames) > 0 {
				return anim, false
			}
			if anim := r.game.sprites.GetAnimation(spriteName, rKey); anim != nil && len(anim.Frames) > 0 {
				return anim, true
			}
		}
	}
	// No clear left/right direction: fall back to any available directional animation.
	if anim := r.game.sprites.GetAnimation(spriteName, rKey); anim != nil && len(anim.Frames) > 0 {
		return anim, false
	}
	if anim := r.game.sprites.GetAnimation(spriteName, lKey); anim != nil && len(anim.Frames) > 0 {
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
	SpriteTypeWallTorch
)

// UnifiedSpriteRenderData holds data for rendering any sprite type in a unified sorted pass
type UnifiedSpriteRenderData struct {
	spriteType SpriteType
	screenX    int
	screenY    int
	spriteSize int
	// Float-precision metrics (screen-X center, size, floor-anchor BOTTOM).
	// Draw paths use these so distant sprites move subpixel-smoothly; the
	// truncated ints above remain for hit spans and the depth-buffer test
	// (independently truncated edges hop +/-1px out of phase - a visible
	// shake on 40-tile open-world sightlines).
	screenXF  float64
	sizeF     float64
	bottomF   float64
	depthPerp float64 // Camera-space perpendicular depth (for z-buffer comparison)
	distance  float64
	sprite    *ebiten.Image
	// Resolved authored variant from the map cache. Keeping this beside sprite
	// prevents the draw path from rediscovering the same variant every frame.
	spriteName string
	// Environment/Tree specific
	tileX    int
	tileY    int
	tileType world.TileType3D
	// A crossed tree is normally one unified entry. If another nearby standee
	// overlaps its depth interval, it expands into four arm entries so the
	// global painter pass can place that standee between the far/near arms.
	// treeCenterDepth remains the projection depth used to build every arm;
	// depthPerp becomes only that arm's global sort key.
	treeArmOnly     bool
	treeArmIndex    int
	treeArmSlab     int
	treeArmLo       int
	treeArmHi       int
	treeCenterDepth float64
	// Monster specific
	monster     *monster.Monster3D
	monsterFlip bool // billboard fallback: mirror the chosen directional sheet
	// Standee art has one deterministic authored facing; world heading supplies
	// the runtime mirror. Keeping it beside the selected frame prevents collect
	// and draw from resolving two different animation sheets.
	monsterArtFacesLeft bool
	monsterRenderX      float64
	monsterRenderY      float64
	// NPC specific
	npc *character.NPC
	// buildingSegment indexes the footprint tile this entry draws for a
	// grid-span facade. A long slab breaks the whole-sprite painter sort (a
	// nearer dune behind the facade's center still stands in front of its far
	// end), so the collector emits one entry PER footprint tile, each sorted at
	// its own tile depth and drawn column-clipped to that tile.
	buildingSegment int
	// Ground container (loot bag / treasure chest) specific
	groundContainer *GroundContainer
}

// Near-tree LOD. A tree is collected once PER screen column it covers (treeHits),
// and each of those redraws the ENTIRE tree sprite (drawTreeSprite isn't a 1px
// slice). At 1 ray/pixel a point-blank tree spans the whole screen -> that many
// full-sprite redraws -> the FPS cliff when pressed into a forest tree. For trees
// within nearTreeLODTiles we keep only 1 of every `stride` columns. A FIXED stride
// isn't enough at point-blank (1-in-4 of ~1920 columns is still ~480 full-screen
// draws), so the stride GROWS as distance shrinks (~ nearTreeDist/distance): the
// columns a tree covers scale ~1/distance, so this keeps the kept-column count -
// and thus the draw count - roughly flat all the way in. Each kept draw is still
// the whole sprite, so the tree looks the same. Far trees stay at full density.
const (
	nearTreeLODTiles     = 3  // apply the thinning only within this many tiles
	nearTreeLODStride    = 4  // base stride at the near edge (nearTreeLODTiles out)
	nearTreeLODMaxStride = 32 // clamp: point-blank keeps ~screenWidth/this columns
)

const (
	tbFrontDiagonalMonsterForwardTiles = 1.05
	tbFrontDiagonalMonsterLateralTiles = 0.28
)

// monsterVisualPosition is where a monster is DRAWN: usually its true spot, but a
// turn-based front-diagonal melee neighbour is "pulled" to read as in-front-of-you
// rather than sliding to the screen edge. The pull geometry lives in ONE place -
// CombatSystem.pulledFrontSlot - shared with the combat resolver so the sprite and
// the hit can't drift apart. Only a genuinely PULLED slot moves the sprite.
func (r *Renderer) monsterVisualPosition(mon *monster.Monster3D) (float64, float64) {
	if mon == nil {
		return 0, 0
	}
	// The on-screen anchor (pulled slot + banded-stack fan) is owned by
	// combat.monsterVisualPos so impact/splash FX land on the same spot.
	if r != nil && r.game != nil && r.game.combat != nil {
		return r.game.combat.monsterVisualPos(mon)
	}
	if r != nil && r.game != nil && r.game.config != nil {
		ox, oy := monsterStackFanOffset(mon, float64(r.game.config.GetTileSize()))
		return mon.X + ox, mon.Y + oy
	}
	return mon.X, mon.Y
}

// monsterHitShakeSizePx clamps the sprite size that scales the on-hit shudder so
// huge sprites don't jolt unboundedly every frame (see MonsterHitShakeMaxRefPx).
func monsterHitShakeSizePx(spriteSize int) float64 {
	if s := float64(spriteSize); s < MonsterHitShakeMaxRefPx {
		return s
	}
	return MonsterHitShakeMaxRefPx
}

func cardinalForwardFromAngle(angle float64) (int, int) {
	c, s := math.Cos(angle), math.Sin(angle)
	if math.Abs(c) >= math.Abs(s) {
		if c < 0 {
			return -1, 0
		}
		return 1, 0
	}
	if s < 0 {
		return 0, -1
	}
	return 0, 1
}

// cullAndProject applies the shared sprite-collection cull: near/far distance
// against squared thresholds (minDistSq 0 disables the near cull) and
// behind-camera rejection via camera-space depth. Returns the euclidean
// distance and perpendicular depth on success.
func cullAndProject(x, y, camX, camY, camDirX, camDirY, minDistSq, viewDistSq float64) (distance, depthPerp float64, ok bool) {
	dx := x - camX
	dy := y - camY
	distanceSq := dx*dx + dy*dy
	if distanceSq < minDistSq || distanceSq > viewDistSq {
		return 0, 0, false
	}
	depthPerp = dx*camDirX + dy*camDirY
	if depthPerp <= 0 {
		return 0, 0, false
	}
	return math.Sqrt(distanceSq), depthPerp, true
}

// crossedTreeRenderData is the shared visibility/projection path for both the
// real unified-sprite collector and the one-time map-load standee warmup. The
// camera basis is supplied by the caller so a frame computes sin/cos only once.
func (r *Renderer) crossedTreeRenderData(td *TransparentSpriteData, camX, camY, camDirX, camDirY, viewDistSq float64) (UnifiedSpriteRenderData, bool) {
	if td == nil || r.game == nil {
		return UnifiedSpriteRenderData{}, false
	}
	distance, depthPerp, ok := cullAndProject(
		td.worldX, td.worldY,
		camX, camY, camDirX, camDirY,
		0, viewDistSq,
	)
	if !ok {
		return UnifiedSpriteRenderData{}, false
	}
	screenXf, bottomF, sizeF, visible := r.game.renderHelper.CalculateEnvironmentSpriteMetricsF(
		td.worldX, td.worldY, distance, td.tileType, 1.0,
	)
	if !visible {
		return UnifiedSpriteRenderData{}, false
	}
	return UnifiedSpriteRenderData{
		spriteType: SpriteTypeTree,
		screenX:    int(screenXf),
		screenY:    int(bottomF) - int(sizeF),
		spriteSize: int(sizeF),
		screenXF:   screenXf,
		sizeF:      sizeF,
		bottomF:    bottomF,
		depthPerp:  depthPerp,
		distance:   distance,
		spriteName: td.spriteName,
		tileX:      td.tileX,
		tileY:      td.tileY,
		tileType:   td.tileType,
	}, true
}

func unifiedSpriteHorizontalSpan(s UnifiedSpriteRenderData) (left, right float64, ok bool) {
	size := s.sizeF
	center := s.screenXF
	if size <= 0 {
		size = float64(s.spriteSize)
		center = float64(s.screenX)
	}
	if size <= 0 {
		return 0, 0, false
	}
	return center - size/2, center + size/2, true
}

// crossedTreeNeedsArmSort keeps the usual one-entry/two-preparation fast path
// unless a non-tree standee actually overlaps the cross in both screen space
// and depth. Dense forests therefore retain their established cost; only a
// local tree/dune beside an NPC, monster, or container expands to four painter
// entries.
func (r *Renderer) crossedTreeNeedsArmSort(tree UnifiedSpriteRenderData, arms [4]treeArm, sprites []UnifiedSpriteRenderData) bool {
	minArmDepth, maxArmDepth := arms[0].depth, arms[0].depth
	treeLeft, treeRight := float64(arms[0].lo), float64(arms[0].hi)
	for _, arm := range arms[1:] {
		minArmDepth = math.Min(minArmDepth, arm.depth)
		maxArmDepth = math.Max(maxArmDepth, arm.depth)
		treeLeft = math.Min(treeLeft, float64(arm.lo))
		treeRight = math.Max(treeRight, float64(arm.hi))
	}
	// Midpoint keys cover half of each arm's depth excursion. Extend the
	// interval by the other half so an object near a corner still triggers the
	// precise arm path.
	depthMargin := math.Max(
		math.Abs(tree.depthPerp-minArmDepth),
		math.Abs(maxArmDepth-tree.depthPerp),
	)
	minTreeDepth := minArmDepth - depthMargin
	maxTreeDepth := maxArmDepth + depthMargin

	for _, candidate := range sprites {
		if candidate.spriteType == SpriteTypeTree || candidate.depthPerp <= 0 {
			continue
		}
		left, right, ok := unifiedSpriteHorizontalSpan(candidate)
		if !ok || right < treeLeft || left > treeRight {
			continue
		}
		// A rotating standee's exact yaw is draw-state, so use its projected
		// footprint as a conservative depth radius. This can expand one extra
		// nearby dune, but cannot miss the tavern-at-the-side case.
		halfDepth := r.spriteFootprintWorld(candidate.sizeF, candidate.depthPerp) / 2
		if candidate.depthPerp+halfDepth < minTreeDepth ||
			candidate.depthPerp-halfDepth > maxTreeDepth {
			continue
		}
		return true
	}
	return false
}

func (r *Renderer) splitCrossedTreesForPainterOrder(sprites []UnifiedSpriteRenderData, start, end int) []UnifiedSpriteRenderData {
	if start < 0 {
		start = 0
	}
	if end > len(sprites) {
		end = len(sprites)
	}
	if start >= end {
		return sprites
	}

	const yawA, yawB = math.Pi / 4, 3 * math.Pi / 4
	tileSize := float64(r.game.config.GetTileSize())
	for i := start; i < end; i++ {
		tree := sprites[i]
		if tree.spriteType != SpriteTypeTree ||
			treeIsBillboardLOD(tree.distance, tileSize, r.game.config.Graphics.TreeStandeeLODTiles) {
			continue
		}
		worldX, worldY := TileCenterFromTile(tree.tileX, tree.tileY, tileSize)
		footprint := r.spriteFootprintWorld(tree.sizeF, tree.depthPerp)
		arms, ok := r.crossedStandeeArms(worldX, worldY, yawA, yawB, footprint)
		if !ok || !r.crossedTreeNeedsArmSort(tree, arms, sprites) {
			continue
		}

		tree.treeCenterDepth = tree.depthPerp
		for armIndex, arm := range arms {
			part := tree
			part.treeArmOnly = true
			part.treeArmIndex = armIndex
			part.treeArmSlab = arm.slabIdx
			part.treeArmLo, part.treeArmHi = arm.lo, arm.hi
			part.depthPerp = arm.depth
			if armIndex == 0 {
				sprites[i] = part
			} else {
				sprites = append(sprites, part)
			}
		}
	}
	return sprites
}

func compareUnifiedSprites(a, b UnifiedSpriteRenderData) int {
	if c := cmp.Compare(b.depthPerp, a.depthPerp); c != 0 {
		return c
	}
	if c := cmp.Compare(a.tileY, b.tileY); c != 0 {
		return c
	}
	if c := cmp.Compare(a.tileX, b.tileX); c != 0 {
		return c
	}
	if c := cmp.Compare(a.screenX, b.screenX); c != 0 {
		return c
	}
	return cmp.Compare(a.treeArmIndex, b.treeArmIndex)
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
	viewDistSq := r.game.camera.ViewDist * r.game.camera.ViewDist

	// Precompute camera direction for camera-space depth calculations
	camDirX := math.Cos(camAngle)
	camDirY := math.Sin(camAngle)

	// Get player's current tile for sprite culling
	playerTileX, playerTileY := r.game.GetPlayerTilePosition()
	tileSize := float64(r.game.config.GetTileSize())
	minDistSq := tileSize * tileSize

	// 1. Collect transparent environment sprites (ferns, mushrooms)
	if world.GlobalTileManager != nil {
		for i := range r.transparentSpritesCache {
			spriteData := &r.transparentSpritesCache[i]

			// Wall-mounted decorations render on the ADJACENT wall, so the party
			// standing in their own tile is within one tile of the anchor - same
			// exemption wall-mounted NPCs get: no on-tile skip, no near-cull, or the
			// decoration vanishes the moment you walk up to it.
			wallMounted := world.GlobalTileManager != nil && world.GlobalTileManager.IsWallMounted(spriteData.tileType)
			if !wallMounted && spriteData.tileX == playerTileX && spriteData.tileY == playerTileY {
				continue
			}
			near := minDistSq
			// SOLID props never near-cull: the party cannot stand ON them, only
			// right against them - and an impassable object that vanishes at
			// point-blank reads as an invisible wall (the planks pile). Walkable
			// greenery keeps the cull so it doesn't smear across the screen.
			if world.GlobalTileManager != nil && world.GlobalTileManager.IsSolid(spriteData.tileType) {
				near = 0
			}
			// Wall-mounted decorations are drawn on the adjacent wall (wallStickPose),
			// so cull/size from there - the tile centre culls when you stand on it.
			// Same effective-position rule as npcEffectivePos.
			ex, ey := spriteData.worldX, spriteData.worldY
			if wallMounted {
				near = 0
				if r.game.config.Graphics.Standee.Enabled {
					if wx, wy, _, ok := r.game.wallStickPose(ex, ey); ok {
						ex, ey = wx, wy
					}
				}
			}
			distance, depthPerp, ok := cullAndProject(ex, ey, camX, camY, camDirX, camDirY, near, viewDistSq)
			if !ok {
				continue
			}

			screenXf, bottomF, sizeF, visible := r.game.renderHelper.CalculateEnvironmentSpriteMetricsF(ex, ey, distance, spriteData.tileType, 1.0)
			if !visible {
				continue
			}

			var sprite *ebiten.Image
			if !isFireflySwarmTile(spriteData.tileType) {
				sprite = r.getProcessedSpriteByName(spriteData.tileType, spriteData.spriteName)
				if sprite == nil {
					continue
				}
			}

			spriteSize := int(sizeF)
			sprites = append(sprites, UnifiedSpriteRenderData{
				spriteType: SpriteTypeEnvironment,
				screenX:    int(screenXf),
				screenY:    int(bottomF) - spriteSize,
				spriteSize: spriteSize,
				screenXF:   screenXf,
				sizeF:      sizeF,
				bottomF:    bottomF,
				depthPerp:  depthPerp,
				sprite:     sprite,
				spriteName: spriteData.spriteName,
				tileX:      spriteData.tileX,
				tileY:      spriteData.tileY,
				tileType:   spriteData.tileType,
			})
		}
	}

	// 2. Add tree hits collected during raycasting. Near trees are thinned with a
	// distance-adaptive stride (see the nearTreeLOD note above): the closer the
	// tree, the more columns it covers and the harder we thin, so the draw count
	// stays bounded even pressed point-blank into it. Far trees keep every column.
	nearTreeDist := float64(nearTreeLODTiles) * float64(r.game.config.GetTileSize())
	for _, tree := range r.treeHits {
		if tree.distance <= nearTreeDist {
			stride := nearTreeLODStride
			if tree.distance > 1 {
				// ~ 1/distance; == nearTreeLODStride at the near edge, grows as you close in.
				if s := int(float64(nearTreeLODStride) * nearTreeDist / tree.distance); s > stride {
					stride = s
				}
			} else {
				stride = nearTreeLODMaxStride // point-blank (distance ~0): max thinning
			}
			if stride > nearTreeLODMaxStride {
				stride = nearTreeLODMaxStride
			}
			if tree.screenX%stride != 0 {
				continue
			}
		}
		sprites = append(sprites, UnifiedSpriteRenderData{
			spriteType: SpriteTypeTree,
			screenX:    tree.screenX,
			depthPerp:  tree.distance,
			tileType:   tree.tileType,
		})
	}
	r.treeHits = r.treeHits[:0]

	// 2b. Crossed-standee trees (one entry per tree TILE). In this mode the DDA
	// skipped tree tiles, so treeHits is empty; trees are drawn as two crossed
	// standees, depth-sorted with everything else.
	crossedTreeStart := len(sprites)
	if r.game.config.Graphics.TreesAsBillboards {
		for i := range r.treeTilesCache {
			td := &r.treeTilesCache[i]
			// No NEAR cull (unlike other standees): a tree must stay visible when
			// the player walks right up to it. Only the far view-distance cull
			// applies; depthPerp<=0 drops trees behind the camera.
			tree, ok := r.crossedTreeRenderData(td, camX, camY, camDirX, camDirY, viewDistSq)
			if !ok {
				continue
			}
			sprites = append(sprites, tree)
		}
	}
	crossedTreeEnd := len(sprites)

	// 3. Collect monsters
	for _, mon := range r.game.GetCurrentWorld().Monsters {
		if !mon.IsAlive() {
			continue
		}

		renderX, renderY := r.monsterVisualPosition(mon)

		distance, depthPerp, ok := cullAndProject(renderX, renderY, camX, camY, camDirX, camDirY, 0, viewDistSq)
		if !ok {
			continue
		}

		sizeTiles := mon.GetSizeGameMultiplier()
		screenXf, bottomF, sizeF, visible := r.game.renderHelper.CalculateMonsterSpriteMetricsF(renderX, renderY, distance, sizeTiles)
		if !visible {
			continue
		}
		if mon.Flying {
			// Centered on the horizon: bottom = mid-screen + half height.
			bottomF = float64(r.game.config.GetScreenHeight())/2 + sizeF/2
		}

		var sprite *ebiten.Image
		var flip, artFacesLeft bool
		if r.game.config.Graphics.Standee.Enabled {
			sprite, artFacesLeft = r.getMonsterStandeeSprite(mon)
		} else {
			sprite, flip = r.getMonsterSprite(mon)
		}

		spriteSize := int(sizeF)
		sprites = append(sprites, UnifiedSpriteRenderData{
			spriteType:          SpriteTypeMonster,
			screenX:             int(screenXf),
			screenY:             int(bottomF) - spriteSize,
			spriteSize:          spriteSize,
			screenXF:            screenXf,
			sizeF:               sizeF,
			bottomF:             bottomF,
			depthPerp:           depthPerp,
			sprite:              sprite,
			monster:             mon,
			monsterFlip:         flip,
			monsterArtFacesLeft: artFacesLeft,
			monsterRenderX:      renderX,
			monsterRenderY:      renderY,
		})
	}

	// 4. Collect NPCs
	for _, npc := range r.game.GetCurrentWorld().NPCs {
		// Spriteless NPCs (e.g. invisible portal gates) render nothing - they
		// exist only as an interaction anchor; their tile shows through instead.
		if npc.Sprite == "" || npc.Sprite == "none" {
			continue
		}
		// Spent statues (hide_when_visited) vanish once used; kept in the world
		// only so their Visited state persists across saves.
		if npc.HideWhenVisited && npc.Visited {
			continue
		}
		// An open door is invisible (portcullis raised) - and non-interactive,
		// the focus/click resolvers share this same test.
		if r.game.npcDoorOpen(npc) {
			continue
		}
		// A grid-span facade (clock tower, pyramid) enters the painter sort PER
		// FOOTPRINT TILE (see UnifiedSpriteRenderData.buildingSegment) and is
		// NEVER gated on its anchor: standing on the footprint and turning the
		// anchor out of view must not vanish the building - like walls, it
		// culls per segment. A segment stays while ANY part of its slice lies
		// in front of the camera plane; its tile CENTER can sit at zero depth
		// with half the slice still visible, so the gate tests the slice's two
		// EDGE points. ONLY in standee mode - the billboard fallback draws the
		// whole facade per entry, so segmenting there would stack N copies.
		if npc.GridSpanTiles >= 2 && r.game.config.Graphics.Standee.Enabled {
			if _, _, byaw, okPose := r.game.buildingPose(npc); okPose {
				// Anchor-projected fields feed only sort tie-breaks; zero is
				// fine when the anchor sits behind the camera.
				var screenXf, bottomF, sizeF float64
				var screenX, screenY, spriteSize int
				ex, ey := r.game.npcEffectivePos(npc)
				if distance, _, okA := cullAndProject(ex, ey, camX, camY, camDirX, camDirY, 0, viewDistSq); okA {
					if sxf, bf, szf, vis := r.game.renderHelper.NPCSpriteMetricsF(npc, ex, ey, distance); vis {
						screenXf, bottomF, sizeF = sxf, bf, szf
						screenX, spriteSize = int(sxf), int(szf)
						screenY = int(bf) - spriteSize
					}
				}
				sprite := r.game.sprites.GetSprite(npcSpriteName(npc))
				ts := float64(r.game.config.GetTileSize())
				dirX, dirY := math.Cos(byaw), math.Sin(byaw)
				for i, c := range r.game.buildingFootprintTiles(npc) {
					ddx, ddy := c[0]-camX, c[1]-camY
					if ddx*ddx+ddy*ddy > viewDistSq {
						continue
					}
					d1 := (ddx-dirX*ts/2)*camDirX + (ddy-dirY*ts/2)*camDirY
					d2 := (ddx+dirX*ts/2)*camDirX + (ddy+dirY*ts/2)*camDirY
					if d1 < 1.0 && d2 < 1.0 {
						continue // whole slice behind the camera plane
					}
					sprites = append(sprites, UnifiedSpriteRenderData{
						spriteType:      SpriteTypeNPC,
						screenX:         screenX,
						screenY:         screenY,
						spriteSize:      spriteSize,
						screenXF:        screenXf,
						sizeF:           sizeF,
						bottomF:         bottomF,
						depthPerp:       math.Max(1.0, ddx*camDirX+ddy*camDirY),
						sprite:          sprite,
						npc:             npc,
						buildingSegment: i,
					})
				}
				continue
			}
		}

		// Cull/project from where the NPC is drawn (wall face for wall tokens).
		// NPCs never use a near-cull: like loot containers, they must remain
		// visible when the party walks into their tile.
		ex, ey := r.game.npcEffectivePos(npc)
		distance, depthPerp, ok := cullAndProject(ex, ey, camX, camY, camDirX, camDirY, 0, viewDistSq)
		if !ok {
			continue
		}

		screenXf, bottomF, sizeF, visible := r.game.renderHelper.NPCSpriteMetricsF(npc, ex, ey, distance)
		if !visible {
			continue
		}
		screenX, spriteSize := int(screenXf), int(sizeF)
		screenY := int(bottomF) - spriteSize

		sprite := r.game.sprites.GetSprite(npcSpriteName(npc))

		sprites = append(sprites, UnifiedSpriteRenderData{
			spriteType: SpriteTypeNPC,
			screenX:    screenX,
			screenY:    screenY,
			spriteSize: spriteSize,
			screenXF:   screenXf,
			sizeF:      sizeF,
			bottomF:    bottomF,
			depthPerp:  depthPerp,
			sprite:     sprite,
			npc:        npc,
		})
	}

	// 5. Collect ground containers (loot bags + treasure chests)
	for i := range r.game.groundContainers {
		c := &r.game.groundContainers[i]
		if c.MapKey != "" && !mapKeyOnCurrentWorld(c.MapKey) {
			continue
		}
		// Loot containers are interactable, so they do NOT use the one-tile
		// near-cull that scenery does. Keep them visible when the party steps
		// into their tile; project from the same fanned position the draw/hit
		// test uses for stacked containers.
		ox, oy := r.game.groundContainerRenderOffset(c)
		distance, depthPerp, ok := cullAndProject(c.X+ox, c.Y+oy, camX, camY, camDirX, camDirY, 0, viewDistSq)
		if !ok {
			continue
		}
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
			screenXF:        info.ScreenXF,
			sizeF:           info.SizeF,
			bottomF:         info.BottomF,
			depthPerp:       depthPerp,
			distance:        info.Distance,
			sprite:          sprite,
			groundContainer: c,
		})
	}

	// 6. Collect wall torches so their flames depth-sort against billboards
	// and standees (the effects pass only tests the WALL depth buffer, which
	// let flames shine through trees and tokens).
	for ti := range r.wallTorches {
		tp := &r.wallTorches[ti]
		_, depthPerp, ok := cullAndProject(tp.X, tp.Y, camX, camY, camDirX, camDirY, 0, viewDistSq)
		if !ok {
			continue
		}
		sprites = append(sprites, UnifiedSpriteRenderData{
			spriteType: SpriteTypeWallTorch,
			depthPerp:  depthPerp,
			tileX:      ti, // index into r.wallTorches
		})
	}

	// A crossed dune/tree normally stays one painter entry. Expand only those
	// whose depth volume overlaps another visible standee, allowing the global
	// sort to interleave that object between the cross's individual arms.
	sprites = r.splitCrossedTreesForPainterOrder(sprites, crossedTreeStart, crossedTreeEnd)

	// Sort all sprites by depth (back to front). slices.SortStableFunc: no
	// reflect swaps and no closure alloc, unlike sort.Slice - this runs every
	// frame. Depth ties are REAL and common: a row of trees seen head-on at
	// range collapses to identical depthPerp (the per-tile difference
	// underflows float64), and an unstable order there reshuffles every
	// frame - overlapping crossed standees flicker and a distant treeline
	// visibly shudders as silhouettes alternate. Tie-break by stable identity,
	// with the stable sort covering identity-less entries (monsters, NPCs).
	slices.SortStableFunc(sprites, compareUnifiedSprites)

	// Update buffer for next frame
	r.unifiedSprites = sprites

	// Render all sprites in sorted order
	for _, s := range sprites {
		switch s.spriteType {
		case SpriteTypeEnvironment:
			r.drawUnifiedEnvironmentSprite(screen, s)
		case SpriteTypeTree:
			if !s.treeArmOnly || s.treeArmIndex == 0 {
				r.statTreesDrawn++
			}
			if r.game.config.Graphics.TreesAsBillboards {
				r.drawCrossedTreeStandees(screen, s)
			} else {
				r.drawTreeSprite(screen, s.screenX, s.depthPerp, s.tileType)
			}
		case SpriteTypeMonster:
			r.drawUnifiedMonsterSprite(screen, s)
		case SpriteTypeNPC:
			r.drawUnifiedNPCSprite(screen, s)
		case SpriteTypeWallTorch:
			if s.tileX >= 0 && s.tileX < len(r.wallTorches) {
				r.drawWallTorchFlame(screen, r.wallTorches[s.tileX])
			}
		case SpriteTypeGroundContainer:
			r.drawUnifiedGroundContainerSprite(screen, s)
		}
	}
}

// spriteDepthBufferVisible returns true if the sprite's screen-X span has at
// least one pixel where the sprite is in front of the wall depth buffer.
// Shared by all the floor-anchored sprite drawers (env / loot bag / chest).
func (r *Renderer) spriteDepthBufferVisible(s UnifiedSpriteRenderData) bool {
	screenXF, sizeF := s.screenXF, s.sizeF
	if sizeF <= 0 {
		screenXF, sizeF = float64(s.screenX), float64(s.spriteSize)
	}
	left := int(math.Floor(screenXF - sizeF/2))
	right := int(math.Ceil(screenXF + sizeF/2))
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

func (r *Renderer) scaledWorldSpriteOpts(scaleX, scaleY float64) *ebiten.DrawImageOptions {
	opts := r.sharedDrawOpts()
	if math.Abs(scaleX) < 0.5 || math.Abs(scaleY) < 0.5 {
		// Mipmapped shrink only when well below source resolution: it kills the
		// nearest-sample mush at range, while the 0.5-1.0 band stays nearest.
		opts.Filter = ebiten.FilterLinear
	}
	opts.GeoM.Scale(scaleX, scaleY)
	return opts
}

// drawTintedSprite draws a sprite scaled to spriteSize at (drawLeft, screenY)
// with the given RGBA tint applied via ColorScale. Used for both the
// brightness pass and the hover-highlight overlay.
func (r *Renderer) drawTintedSprite(screen *ebiten.Image, sprite *ebiten.Image, drawLeft, screenY, spriteSize int, tintR, tintG, tintB, tintA float32) {
	r.drawTintedSpriteF(screen, sprite, float64(drawLeft), float64(screenY), float64(spriteSize), tintR, tintG, tintB, tintA)
}

// drawTintedSpriteF is drawTintedSprite with float metrics: the GPU rasterizes
// the float rect, so a distant sprite glides subpixel-smoothly instead of
// hopping whole pixels.
func (r *Renderer) drawTintedSpriteF(screen *ebiten.Image, sprite *ebiten.Image, drawLeft, screenY, spriteSize float64, tintR, tintG, tintB, tintA float32) {
	if sprite == nil || spriteSize <= 0 {
		return
	}
	scaleX := spriteSize / float64(sprite.Bounds().Dx())
	scaleY := spriteSize / float64(sprite.Bounds().Dy())
	opts := r.scaledWorldSpriteOpts(scaleX, scaleY)
	opts.GeoM.Translate(drawLeft, screenY)
	opts.ColorScale.Scale(tintR, tintG, tintB, tintA)
	opts.Blend = ebiten.BlendSourceOver
	screen.DrawImage(sprite, opts)
}

// hoverHighlightTint is the soft yellow overlay drawn on pickup-range
// sprites (ground containers) when the cursor is over them.
var hoverHighlightTint = [4]float32{1.0, 0.95, 0.6, 0.6}

// standeeHoverBoost brightens a standee token (container or NPC) while the
// cursor is over it. The billboard overlay/edge-glow trick doesn't map onto
// the token mesh: a flat silhouette halo neither turns with the token's yaw
// nor matches its foreshortened outline, so on tokens the hover cue is the
// token itself lighting up.
const standeeHoverBoost = 1.4

// drawUnifiedGroundContainerSprite draws a ground container (loot bag or
// treasure chest) as a slowly spinning standee token (falling back to a
// billboard when standee mode is off). A loot bag holding items shows its
// rarity-specific sack SPRITE (bag_<rarity>.png via effectiveSprite - no
// runtime recolor); a gold-only bag shows the plain gold pile, chests their
// chest art.
func (r *Renderer) drawUnifiedGroundContainerSprite(screen *ebiten.Image, s UnifiedSpriteRenderData) {
	if !r.spriteDepthBufferVisible(s) || s.groundContainer == nil {
		return
	}
	c := s.groundContainer
	spriteName := c.effectiveSprite()

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
		hovered = r.game.groundContainerHitTestFromInfo(info, spriteName, mouseX, mouseY, pickupRange)
	}

	brightness := r.calculateBrightnessWithTorchLight(c.X, c.Y, s.distance)
	b := float32(brightness)

	// Standee: draw as a wooden token spinning idly in place (same path as
	// scenery/NPC tokens), positioned by the band fan so a pile of bags on one
	// tile reads as several. A brightness bump is the hover cue in this path.
	if r.game.config.Graphics.Standee.Enabled {
		sb := b
		if hovered {
			sb = b * standeeHoverBoost
		}
		ox, oy := r.game.groundContainerRenderOffset(c)
		phase := auraHash(int(c.X), int(c.Y), 0, 0) * 2 * math.Pi
		yaw := standeeStaticYaw + phase + containerSpinDegSec*math.Pi/180*float64(r.game.frameCount)/float64(r.game.config.GetTPS())
		key := makeStandeeCoreKey(r.prefixedStandeeKeyName("container", spriteName), s.sprite, false)
		if r.drawStandeeSprite(screen, s.sprite, key, c.X+ox, c.Y+oy, yaw,
			s.depthPerp, s.sizeF, s.bottomF, sb, sb, sb, true, false, 0) {
			return
		}
	}

	// Billboard fallback (standee off or off-screen).
	drawLeftF := s.screenXF - s.sizeF/2
	drawTopF := s.bottomF - s.sizeF
	r.drawTintedSpriteF(screen, s.sprite, drawLeftF, drawTopF, s.sizeF, b, b, b, 1.0)
	if hovered {
		r.drawTintedSpriteF(screen, s.sprite, drawLeftF, drawTopF, s.sizeF,
			hoverHighlightTint[0], hoverHighlightTint[1], hoverHighlightTint[2], hoverHighlightTint[3])
	}
}

// smoothYaw eases a standee's stored yaw toward target at speed (deg/sec).
// First sighting (or long unseen, > 1s) snaps to the target at once.
func (r *Renderer) smoothYaw(st standeeEnvYawState, seen bool, target, speed float64) standeeEnvYawState {
	tps := r.game.config.GetTPS()
	dt := r.game.frameCount - st.tick
	if !seen || dt > int64(tps) {
		st.yaw = target
	} else if dt > 0 {
		maxStep := speed * math.Pi / 180 * float64(dt) / float64(tps)
		st.yaw = approachAngle(st.yaw, target, maxStep)
	}
	st.tick = r.game.frameCount
	return st
}

// drawUnifiedEnvironmentSprite draws an environment sprite from unified data
func (r *Renderer) drawUnifiedEnvironmentSprite(screen *ebiten.Image, s UnifiedSpriteRenderData) {
	tileSize := float64(r.game.config.GetTileSize())
	worldX, worldY := TileCenterFromTile(s.tileX, s.tileY, tileSize)
	distance := Distance(worldX, worldY, r.game.camera.X, r.game.camera.Y)
	if isFireflySwarmTile(s.tileType) {
		if !r.spriteDepthBufferVisible(s) {
			return
		}
		r.drawFireflySwarmEffect(screen, s, distance)
		return
	}
	if s.sprite == nil {
		return
	}
	brightness := r.calculateBrightnessWithTorchLight(worldX, worldY, distance)
	b := float32(brightness)

	// Animate ordinary standee tiles: a sprite whose sheet is w == h*4 cycles
	// frames like an NPC idle; single-frame sprites come back unchanged. Every
	// draw path below uses the current frame.
	frame, _, _ := r.selectAnimatedSpriteFrame(s.sprite, r.game.frameCount)

	// Standee mode: scenery tokens slowly turn to face the camera (a lazy
	// billboard - the turn rate makes them feel like propped-up cutouts being
	// nudged, not glued to the view). Rate 0 = fixed diagonal.
	if r.game.config.Graphics.Standee.Enabled {
		name := s.spriteName
		if name == "" {
			name = r.selectEnvironmentSpriteName(s.tileType, s.tileX, s.tileY)
		}
		// Wall-mounted decoration tile: stick flush to the nearest solid neighbour
		// and orient along that wall (the tile twin of NPC wall_mounted). Falls
		// through to a centred standee when no wall is adjacent.
		if world.GlobalTileManager != nil && world.GlobalTileManager.IsWallMounted(s.tileType) {
			if wx, wy, wyaw, ok := r.game.wallStickPose(worldX, worldY); ok {
				wkey := makeStandeeCoreKey(r.prefixedStandeeKeyName("wallprop", name), frame, false)
				// Centre on the wall, not floor-anchored: bottom = horizon + half
				// height puts the sprite centre on the horizon (the wall's mid-line).
				centeredBottom := float64(r.game.config.GetScreenHeight())/2 + s.sizeF/2
				if r.drawWallStandee(screen, frame, wkey, wx, wy, wyaw, s.depthPerp, s.sizeF, centeredBottom, b, 0, wallMountedDepthAllowanceWorld(r.game.config.GetTileSize(), r.game.config.Graphics.Standee.ThicknessTiles), true) {
					return
				}
			}
		}
		// Normal props use the cheap span prefilter. Wall-mounted props must
		// bypass it just like wall-mounted NPCs: their anchor is coplanar with
		// the backing wall, so only drawWallStandee's per-column test can
		// distinguish that wall from a true foreground occluder.
		if !r.spriteDepthBufferVisible(s) {
			return
		}
		yaw := standeeStaticYaw
		// Landmark tiles (e.g. the city fountain) render as a TALL crossed standee
		// spinning in place - same monument treatment as the landmark NPCs (they
		// spin, unlike ordinary scenery which only faces the camera).
		if world.GlobalTileManager != nil && world.GlobalTileManager.GetRenderType(s.tileType) == "landmark" {
			td := world.GlobalTileManager.GetTileData(s.tileType)
			if spin := r.game.config.Graphics.Standee.NPCSpinDegPerSec; spin != 0 && (td == nil || !td.NoSpin) {
				phase := auraHash(s.tileX, s.tileY, 0, 0) * 2 * math.Pi
				yaw += phase + spin*math.Pi/180*float64(r.game.frameCount)/float64(r.game.config.GetTPS())
			}
			if r.drawLandmarkStandee(screen, frame, r.prefixedStandeeKeyName("landmark", name), worldX, worldY, yaw, s.depthPerp, s.sizeF, s.bottomF, b) {
				return
			}
		}
		if speed := r.game.config.Graphics.Standee.EnvFaceDegPerSec; speed > 0 {
			target := math.Atan2(r.game.camera.Y-worldY, r.game.camera.X-worldX) + math.Pi/2
			tileKey := [2]int{s.tileX, s.tileY}
			if r.standeeEnvYaw == nil {
				r.standeeEnvYaw = make(map[[2]int]standeeEnvYawState)
			}
			st, seen := r.standeeEnvYaw[tileKey]
			st = r.smoothYaw(st, seen, target, speed)
			r.standeeEnvYaw[tileKey] = st
			yaw = st.yaw
		}
		// Key the silhouette by the tile's RESOLVED sprite variant (grass0/1/...),
		// not the base name: variants share dimensions, and a base-name key let
		// whichever variant rendered first stamp its wood slab onto all of them
		// (short grass tuft wearing the tall variant's silhouette).
		key := makeStandeeCoreKey(r.prefixedStandeeKeyName("tile", name), frame, false)
		if r.drawStandeeSprite(screen, frame, key, worldX, worldY, yaw,
			s.depthPerp, s.sizeF, s.bottomF, b, b, b, true, false, 0) {
			return
		}
	}
	if !r.spriteDepthBufferVisible(s) {
		return
	}

	r.drawTintedSpriteF(screen, frame, s.screenXF-s.sizeF/2, s.bottomF-s.sizeF, s.sizeF, b, b, b, 1.0)
}

// drawUnifiedMonsterSprite draws a monster sprite from unified data
func (r *Renderer) drawUnifiedMonsterSprite(screen *ebiten.Image, s UnifiedSpriteRenderData) {
	if !r.spriteDepthBufferVisible(s) {
		return
	}
	// drawAllSpritesSorted always stamps the visual position (true or pulled).
	renderX, renderY := s.monsterRenderX, s.monsterRenderY

	drawLeftF := s.screenXF - s.sizeF/2
	// Hit shake: while the red flash timer runs, rattle the sprite left-right in
	// place (amplitude decays with the timer). Reads as a struck shudder without
	// moving the monster's actual position - replaces the old knockback.
	if s.monster != nil && s.monster.HitTintFrames > 0 {
		f := float64(s.monster.HitTintFrames) / float64(MonsterHitFlashFrames)
		if f > 1 {
			f = 1
		}
		dir := 1.0
		if s.monster.HitTintFrames%2 == 0 {
			dir = -1.0
		}
		drawLeftF += dir * f * MonsterHitShakeAmplitudeFrac * monsterHitShakeSizePx(s.spriteSize)
	}
	// Keep mobs above the party HUD bar: a big sprite at point-blank range would
	// otherwise sink its lower body behind the bar. If its feet would cross the
	// bar's top edge, raise the whole sprite so its bottom rests on the bar.
	screenYF := s.bottomF - s.sizeF
	if r.game.showPartyStats {
		barTop := float64(r.game.config.GetScreenHeight() - r.game.config.UI.PartyPortraitHeight)
		if screenYF+s.sizeF > barTop {
			screenYF = barTop - s.sizeF
		}
	}
	screenY := int(screenYF)

	distance := Distance(renderX, renderY, r.game.camera.X, r.game.camera.Y)
	brightness := r.calculateBrightnessWithTorchLight(renderX, renderY, distance)
	br := float32(brightness)
	rr, gg, bb := br, br, br
	// Hit flash: when just struck, flash red (boost red, cut green/blue), fading
	// over MonsterHitFlashFrames so the impact reads clearly.
	if s.monster != nil && s.monster.HitTintFrames > 0 {
		f := float32(s.monster.HitTintFrames) / float32(MonsterHitFlashFrames)
		if f > 1 {
			f = 1
		}
		rr = br + (1.7-br)*f
		gg = br * (1 - 0.75*f)
		bb = br * (1 - 0.75*f)
	}
	// Elite/variant tint: a persistent colour cast distinguishes a champion from
	// the base mob it shares a sprite with. Multiplies the lit colour (applied to
	// both standee and billboard paths below), so it reads at a glance - no new art.
	if s.monster != nil && (s.monster.TintR != 0 || s.monster.TintG != 0 || s.monster.TintB != 0) {
		rr *= s.monster.TintR
		gg *= s.monster.TintG
		bb *= s.monster.TintB
	}

	// Standee mode: the monster is a wooden token whose face turns with its
	// travel direction (yaw = Direction + 90deg puts the slab across it). The
	// displayed yaw eases toward the heading at the configured turn speed so
	// the token swivels instead of snapping.
	if r.game.config.Graphics.Standee.Enabled {
		m := s.monster
		target := m.Direction + math.Pi/2
		// A monster fighting the PARTY squares up to the camera (plane across
		// the sight line - the art reads in full, never a sliver mid-brawl).
		// Bound allies fight other monsters, so they keep their travel facing.
		// The logical camera keeps the target free of Draw-time shake jitter.
		fightingParty := m.TargetsParty()
		if fightingParty && r.game.combat != nil {
			camX, camY := r.game.combat.logicalCameraXY()
			target = math.Atan2(renderY-camY, renderX-camX) + math.Pi/2
		}
		// Readability clamp: a monster crossing the view would turn its slab
		// edge-on to the camera and vanish into a sliver - keep the plane a
		// minimum angle away from the sight line. Clamping the TARGET (before
		// easing) keeps the correction itself smooth. (Facing the camera is
		// never edge-on, so the clamp is a no-op for fighters.)
		if minDeg := r.game.config.Graphics.Standee.MinViewAngleDeg; minDeg > 0 {
			viewAngle := math.Atan2(renderY-r.game.camera.Y, renderX-r.game.camera.X)
			target = clampYawFromEdgeOn(target, viewAngle, minDeg*math.Pi/180)
		}
		if m.StandeeYawTick == 0 {
			m.StandeeYaw = target // first sighting: face the heading immediately
		} else if dt := r.game.frameCount - m.StandeeYawTick; dt > 0 {
			turn := r.game.config.Graphics.Standee.TurnSpeedDegPerSec
			if turn <= 0 {
				turn = standeeTurnDefault
			}
			maxStep := turn * math.Pi / 180 * float64(dt) / float64(r.game.config.GetTPS())
			m.StandeeYaw = approachAngle(m.StandeeYaw, target, maxStep)
		}
		m.StandeeYawTick = r.game.frameCount

		// World-deterministic art: one animation set, flipped so the figure
		// faces its heading as it slides across the screen. The flip compares
		// the token's on-screen U direction with the heading's on-screen
		// direction; held while the heading points at/away from the camera
		// (|dDot| small) so it can't flicker mid-charge.
		sprite, artFacesLeft := s.sprite, s.monsterArtFacesLeft
		if sprite == nil {
			return
		}
		if mirror, decisive := standeeMirrorFor(r.game.camera.Angle, m.StandeeYaw, m.Direction, artFacesLeft); decisive {
			m.StandeeMirror = mirror
		}

		// Hit shake, standee edition: rattle the token along its own axis in
		// world space (same amplitude/phase as the billboard's screen shake).
		entX, entY := renderX, renderY
		if m.HitTintFrames > 0 {
			f := float64(m.HitTintFrames) / float64(MonsterHitFlashFrames)
			if f > 1 {
				f = 1
			}
			dir := 1.0
			if m.HitTintFrames%2 == 0 {
				dir = -1.0
			}
			worldLen := r.spriteFootprintWorld(monsterHitShakeSizePx(s.spriteSize), s.depthPerp)
			off := dir * f * MonsterHitShakeAmplitudeFrac * worldLen
			entX += math.Cos(m.StandeeYaw) * off
			entY += math.Sin(m.StandeeYaw) * off
		}

		// Monster animation frames are load-time images with identical bounds;
		// the pointer (stable for them) is what tells frames apart in the cache.
		key := makeStandeeCoreKey(r.prefixedStandeeKeyName("mob", m.Key), sprite, true)
		if r.drawStandeeSprite(screen, sprite, key, entX, entY, m.StandeeYaw,
			s.depthPerp, s.sizeF, screenYF+s.sizeF, rr, gg, bb, false, m.StandeeMirror, 0) {
			r.drawMonsterStatusFX(screen, s, screenY)
			return
		}
	}

	// DEPRECATED: flat camera-facing billboard, superseded by the standee token
	// above (graphics.standee.enabled=true is the shipped default). Kept only as
	// the fallback when standee is turned off; not a maintained visual target -
	// new per-monster overlays belong in drawMonsterStatusFX, called from BOTH
	// paths, not appended here alone.
	billboardSprite, billboardFlip := s.sprite, s.monsterFlip
	if r.game.config.Graphics.Standee.Enabled {
		// Resolve the billboard-specific directional sheet only when standee
		// geometry genuinely rejected the token and this fallback will draw.
		billboardSprite, billboardFlip = r.getMonsterSprite(s.monster)
	}
	if billboardSprite == nil {
		return
	}
	scaleX := s.sizeF / float64(billboardSprite.Bounds().Dx())
	scaleY := s.sizeF / float64(billboardSprite.Bounds().Dy())
	if billboardFlip {
		opts := r.scaledWorldSpriteOpts(-scaleX, scaleY)
		opts.GeoM.Translate(drawLeftF+s.sizeF, screenYF)
		opts.ColorScale.Scale(rr, gg, bb, 1.0)
		opts.Blend = ebiten.BlendSourceOver
		screen.DrawImage(billboardSprite, opts)
	} else {
		opts := r.scaledWorldSpriteOpts(scaleX, scaleY)
		opts.GeoM.Translate(drawLeftF, screenYF)
		opts.ColorScale.Scale(rr, gg, bb, 1.0)
		opts.Blend = ebiten.BlendSourceOver
		screen.DrawImage(billboardSprite, opts)
	}
	r.drawMonsterStatusFX(screen, s, screenY)
}

// drawMonsterStatusFX overlays a monster's status indicators (stun stars,
// poison bubbles). Mandatory in the standee path (the shipped default) and
// shared with the deprecated billboard fallback so the two can't drift -
// call this for any new per-monster overlay instead of adding it to one path.
func (r *Renderer) drawMonsterStatusFX(screen *ebiten.Image, s UnifiedSpriteRenderData, screenY int) {
	if s.monster == nil {
		return
	}
	if s.monster.StunFramesRemaining > 0 || s.monster.StunTurnsRemaining > 0 {
		r.drawMonsterStunStars(screen, float64(s.screenX), float64(screenY), float64(s.spriteSize))
	}
	if s.monster.PoisonedFramesRemaining > 0 {
		r.drawMonsterPoisonBubbles(screen, float64(s.screenX), float64(screenY), float64(s.spriteSize))
	}
}

// drawMonsterPoisonBubbles rises a column of small green bubbles past a
// poisoned monster - the world-space sibling of the character HUD's
// drawCardPoisonBubbles (ui_hud.go).
func (r *Renderer) drawMonsterPoisonBubbles(screen *ebiten.Image, centerX, topY, spriteSize float64) {
	f := int(r.game.frameCount)
	const n = 6
	const period = 72
	w := spriteSize * 0.5
	left := centerX - w/2
	for k := 0; k < n; k++ {
		phase := float64((f+k*period/n)%period) / float64(period) // 0..1 rising loop
		bx := left + (float64(k)+0.5)/float64(n)*w + math.Sin(float64(f)*0.08+float64(k))*spriteSize*0.02
		by := topY + spriteSize*(1-phase)
		a := uint8(170 * (1 - phase)) // fade as it nears the top ("pops")
		if a < 12 {
			continue
		}
		rad := float32(spriteSize * (0.015 + 0.02*phase)) // swells as it rises
		vector.FillCircle(screen, float32(bx), float32(by), rad, color.RGBA{70, 210, 90, a}, true)
	}
}

// stunStarRingGeometry places the stun ring above the monster's head. A
// point-blank monster is raised above the HUD bar, pushing its head (and this
// ring) past the top of the screen - the clamp keeps the ring in view so a
// stunned melee-range monster still shows its stars.
func stunStarRingGeometry(topY, spriteSize float64) (cy, rx, ry float64) {
	rx, ry = spriteSize*0.30, spriteSize*0.12
	cy = topY - spriteSize*0.08
	if minCy := ry + spriteSize*0.05; cy < minCy {
		cy = minCy
	}
	return cy, rx, ry
}

// drawMonsterStunStars wheels a ring of twinkling four-point stars above a
// stunned monster - the world-space sibling of the character HUD's
// drawCardStunStars (ui_hud.go), same visual, anchored over a monster sprite
// instead of a portrait card.
func (r *Renderer) drawMonsterStunStars(screen *ebiten.Image, centerX, topY, spriteSize float64) {
	f := float64(r.game.frameCount)
	cx := centerX
	cy, rx, ry := stunStarRingGeometry(topY, spriteSize)
	const n = 5
	for k := 0; k < n; k++ {
		ang := f*0.06 + 2*math.Pi*float64(k)/float64(n)
		sx := float32(cx + math.Cos(ang)*rx)
		sy := float32(cy + math.Sin(ang)*ry)
		tw := 0.5 + 0.5*math.Sin(f*0.25+float64(k)*1.7) // twinkle
		a := uint8(120 + 135*tw)
		arm := float32(spriteSize*0.02 + spriteSize*0.03*tw)
		col := color.RGBA{255, 240, 120, a}
		vector.StrokeLine(screen, sx-arm, sy, sx+arm, sy, 1.5, col, true)
		vector.StrokeLine(screen, sx, sy-arm, sx, sy+arm, 1.5, col, true)
		d := arm * 0.6
		spark := color.RGBA{255, 255, 200, uint8(a / 2)}
		vector.StrokeLine(screen, sx-d, sy-d, sx+d, sy+d, 1, spark, true)
		vector.StrokeLine(screen, sx-d, sy+d, sx+d, sy-d, 1, spark, true)
		vector.FillCircle(screen, sx, sy, 1.2, color.RGBA{255, 255, 230, a}, true)
	}
}

// drawUnifiedNPCSprite draws an NPC sprite from unified data
func (r *Renderer) drawUnifiedNPCSprite(screen *ebiten.Image, s UnifiedSpriteRenderData) {
	drawLeft := s.screenX - s.spriteSize/2
	sprite, frameW, frameH := r.selectAnimatedSpriteFrame(s.sprite, r.game.frameCount)
	// One source of truth for how this NPC renders (shared with the map editor).
	cat := npcRenderCatOf(s.npc)
	npcName := npcSpriteName(s.npc)
	npcKeyName := r.prefixedStandeeKeyName("npc", npcName)

	distance := Distance(s.npc.X, s.npc.Y, r.game.camera.X, r.game.camera.Y)
	brightness := r.calculateBrightnessWithTorchLight(s.npc.X, s.npc.Y, distance)
	if brightness < r.game.config.Graphics.BrightnessMin {
		brightness = r.game.config.Graphics.BrightnessMin
	}
	br := float32(brightness)

	// Hover cue on clickable NPCs. Same hit-test as the click path, so
	// wall-mounted standees highlight where they are actually drawn, not at
	// their tile centre. HOW the cue renders depends on the draw path: standee
	// tokens light up (standeeHoverBoost - a flat silhouette halo would neither
	// turn with the token's yaw nor match its foreshortened outline); the
	// billboard fallback keeps the silhouette edge glow drawn behind the sprite.
	hovered := false
	if r.game.worldClickAllowed() {
		ex, ey := r.game.npcEffectivePos(s.npc)
		if dist := Distance(ex, ey, r.game.camera.X, r.game.camera.Y); dist <= InteractionDistance {
			mouseX, mouseY := ebiten.CursorPosition()
			hovered = r.game.npcScreenHitTest(s.npc, ex, ey, dist, mouseX, mouseY)
		}
	}
	sb := br
	if hovered {
		sb = br * standeeHoverBoost
	}

	// Standee mode: animated NPCs (people) slowly turn to face the party, like
	// figures attending to a visitor; static objects (statues, valves,
	// buildings) spin slowly in place, showcase-style, with the spin phase
	// hashed from position so neighbours don't rotate in lockstep.
	if r.game.config.Graphics.Standee.Enabled {
		// Wall-mounted: slide the token to the nearest solid (wall) neighbour and
		// face along that wall, so gates/grates sit flush ON the wall instead of
		// floating (and spinning) mid-tile. Falls through to the normal standee if
		// no wall is adjacent.
		//
		// This must run BEFORE the generic spriteDepthBufferVisible prefilter:
		// a wall-mounted anchor sits on the backing wall, so the coarse
		// sprite-vs-wall test can see the NPC at the same depth as that wall and
		// reject it before the wall-mounted draw path gets to apply its backing
		// wall bias.
		if r.game.npcIsWall(s.npc) {
			if wx, wy, wyaw, ok := r.game.wallStickPose(s.npc.X, s.npc.Y); ok {
				wkey := makeStandeeCoreKey(npcKeyName, sprite, false)
				// Full NPC gates stay floor-anchored (bottom at the tile's floor).
				r.drawWallStandee(screen, sprite, wkey, wx, wy, wyaw, s.depthPerp, s.sizeF, s.bottomF, sb, 0, wallMountedDepthAllowanceWorld(r.game.config.GetTileSize(), r.game.config.Graphics.Standee.ThicknessTiles), true)
				return
			}
		}
		// Closed door: a slab centered on the doorway tile, spanning the two
		// flanking walls (doorPose) - perpendicular to a wall standee. Same
		// backing-bias draw as walls so the slab ends aren't depth-rejected
		// against the walls they touch. Falls through when mis-authored (no
		// flanking wall pair).
		// Grid-span building (clock tower, pyramid): ONE grid-aligned facade slab that
		// owns its whole footprint - length forced to the span, pixel height
		// from the walls' formula scaled by the art's aspect, bottom on the floor
		// line. It is free-standing, so it must NOT borrow a wall's depth bias.
		if s.npc.GridSpanTiles >= 2 {
			if bx, by, byaw, ok := r.game.buildingPose(s.npc); ok {
				wkey := makeStandeeCoreKey(npcKeyName, sprite, false)
				ts := float64(r.game.config.GetTileSize())
				span := float64(s.npc.GridSpanTiles) * ts
				aspect := float64(sprite.Bounds().Dy()) / math.Max(1, float64(sprite.Bounds().Dx()))
				// The facade's geometry comes from the FOOTPRINT CENTER depth for
				// every segment - a per-segment depth would step the height at
				// each tile boundary. The entry's own depthPerp is the sort key.
				// Camera-plane depth directly: a projection would FAIL with the
				// center behind the camera (standing on the footprint, looking
				// away from it) and drop still-visible segments. The clamp is
				// safe - the column formula uses only the size*depth product,
				// which is depth-invariant - but its floor must clear the
				// height-sanity cap in CalculateWallDimensionsWithHeight: below
				// ~span*aspect world units the capped height squashes the whole
				// facade by that factor. One tile is comfortably above it (and
				// keeps the volumetric shell count sane).
				cam := r.game.camera
				camDX, camDY := math.Cos(cam.Angle), math.Sin(cam.Angle)
				centerDepth := (bx-cam.X)*camDX + (by-cam.Y)*camDY
				if centerDepth < ts {
					centerDepth = ts
				}
				bh, btop := r.game.renderHelper.CalculateWallDimensionsWithHeight(centerDepth, float64(s.npc.GridSpanTiles)*aspect)
				slab, okSlab := r.prepareStandeeSlab(sprite, wkey, bx, by, byaw, centerDepth, float64(bh), float64(btop+bh), sb, sb, sb, true, false, span, r.standeeSurfaces[:0])
				if okSlab {
					// Column-clip the shared slab to THIS entry's footprint tile
					// (the painter sort placed the segment at its own tile depth).
					// A boundary behind the camera plane is CLAMPED to just in
					// front of it along the segment edge - never widened to the
					// whole facade, which would re-break the per-segment sort.
					tiles := r.game.buildingFootprintTiles(s.npc)
					if s.buildingSegment >= 0 && s.buildingSegment < len(tiles) {
						c := tiles[s.buildingSegment]
						dirX, dirY := math.Cos(byaw), math.Sin(byaw)
						e1x, e1y := c[0]-dirX*ts/2, c[1]-dirY*ts/2
						e2x, e2y := c[0]+dirX*ts/2, c[1]+dirY*ts/2
						depthOf := func(x, y float64) float64 { return (x-cam.X)*camDX + (y-cam.Y)*camDY }
						const nearEps = 1.0 // world px in front of the camera plane
						d1, d2 := depthOf(e1x, e1y), depthOf(e2x, e2y)
						if d1 >= nearEps || d2 >= nearEps { // both behind = segment invisible
							if d1 < nearEps {
								t := (nearEps - d1) / (d2 - d1)
								e1x, e1y = e1x+(e2x-e1x)*t, e1y+(e2y-e1y)*t
							} else if d2 < nearEps {
								t := (nearEps - d2) / (d1 - d2)
								e2x, e2y = e2x+(e1x-e2x)*t, e2y+(e1y-e2y)*t
							}
							x1, _, ok1 := r.game.renderHelper.projectToScreenX(e1x, e1y)
							x2, _, ok2 := r.game.renderHelper.projectToScreenX(e2x, e2y)
							if ok1 && ok2 {
								if x1 > x2 {
									x1, x2 = x2, x1
								}
								r.drawStandeeSlabColumns(screen, slab, x1, x2)
							}
						}
					}
				}
				r.standeeSurfaces = slab.surfaces[:0]
				return
			}
		}
		if cat == catDoor {
			if wx, wy, wyaw, ok := r.game.doorPose(s.npc.X, s.npc.Y); ok {
				wkey := makeStandeeCoreKey(npcKeyName, sprite, false)
				// The gate art is square and the doorway is exactly one tile:
				// force the slab to 1 tile of world span and measure its pixel
				// height with the WALLS' own formula at the same depth, so the
				// door meets the flanking walls and the lintel line precisely -
				// no billboard rounding, no overscan, no art stretch.
				doorSpan := float64(r.game.config.GetTileSize())
				doorH, doorTop := r.game.renderHelper.CalculateWallDimensionsWithHeight(s.depthPerp, 1.0)
				r.drawWallStandee(screen, sprite, wkey, wx, wy, wyaw, s.depthPerp, float64(doorH), float64(doorTop+doorH), sb, doorSpan, doorDepthAllowanceWorld(r.game.config.GetTileSize()), false)
				return
			}
		}
	}

	if !r.spriteDepthBufferVisible(s) {
		return
	}

	if r.game.config.Graphics.Standee.Enabled {
		yaw := standeeStaticYaw
		facesParty := cat == catNPC // people attend to the visitor; props spin showcase-style
		if speed := r.game.config.Graphics.Standee.EnvFaceDegPerSec; facesParty && speed > 0 {
			target := math.Atan2(r.game.camera.Y-s.npc.Y, r.game.camera.X-s.npc.X) + math.Pi/2
			if r.standeeNPCYaw == nil {
				r.standeeNPCYaw = make(map[*character.NPC]standeeEnvYawState)
			}
			st, seen := r.standeeNPCYaw[s.npc]
			st = r.smoothYaw(st, seen, target, speed)
			r.standeeNPCYaw[s.npc] = st
			yaw = st.yaw
		} else if spin := r.game.config.Graphics.Standee.NPCSpinDegPerSec; !facesParty && spin != 0 && !s.npc.NoSpin {
			phase := auraHash(int(s.npc.X), int(s.npc.Y), 0, 0) * 2 * math.Pi
			yaw += phase + spin*math.Pi/180*float64(r.game.frameCount)/float64(r.game.config.GetTPS())
		}
		// Landmarks (towers, churches, the city gate, the lich nexus) render as a
		// TALL crossed standee spinning with the same showcase yaw - a 3D monument
		// instead of a flat token.
		if cat == catLandmark {
			if r.drawLandmarkStandee(screen, sprite, r.prefixedStandeeKeyName("landmark", npcName), s.npc.X, s.npc.Y, yaw, s.depthPerp, s.sizeF, s.bottomF, sb) {
				return
			}
		}
		key := makeStandeeCoreKey(npcKeyName, sprite, false)
		if r.drawStandeeSprite(screen, sprite, key, s.npc.X, s.npc.Y, yaw,
			s.depthPerp, s.sizeF, s.bottomF, sb, sb, sb, true, false, 0) {
			return
		}
	}

	scaleX := float64(s.spriteSize) / float64(frameW)
	scaleY := float64(s.spriteSize) / float64(frameH)

	// Billboard: the flat silhouette edge glow matches the flat draw exactly.
	// Drawn FIRST, so the opaque sprite covers the interior and only the rim
	// protruding past the silhouette stays visible.
	if hovered {
		r.drawSpriteEdgeGlow(screen, sprite, drawLeft, s.screenY, scaleX, scaleY, s.spriteSize)
	}

	opts := r.scaledWorldSpriteOpts(scaleX, scaleY)
	opts.GeoM.Translate(float64(drawLeft), float64(s.screenY))

	opts.ColorScale.Scale(br, br, br, 1.0)
	opts.Blend = ebiten.BlendSourceOver

	screen.DrawImage(sprite, opts)
}

// drawSpriteEdgeGlow outlines a billboard sprite with a soft warm halo: the
// sprite's own silhouette re-drawn additively at eight small offsets, so the
// glow hugs the edge instead of washing over the body.
func (r *Renderer) drawSpriteEdgeGlow(screen, sprite *ebiten.Image, drawLeft, drawTop int, scaleX, scaleY float64, spriteSize int) {
	off := spriteSize / 40
	if off < 2 {
		off = 2
	}
	for _, d := range [8][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
		opts := r.scaledWorldSpriteOpts(scaleX, scaleY)
		opts.GeoM.Translate(float64(drawLeft+d[0]*off), float64(drawTop+d[1]*off))
		opts.ColorScale.Scale(1.0, 0.85, 0.45, 0.10)
		opts.Blend = additiveGlowBlend
		screen.DrawImage(sprite, opts)
	}
}

// animationFrames returns the cached per-frame SubImages of a w==h*4 sheet;
// a non-sheet image comes back as a single frame. Shared by the looping NPC
// animation and one-shot players (buff overlay), which index frames themselves.
func (r *Renderer) animationFrames(sprite *ebiten.Image) []*ebiten.Image {
	if frames := r.animFrameCache[sprite]; frames != nil {
		return frames
	}
	bounds := sprite.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if r.animFrameCache == nil {
		r.animFrameCache = make(map[*ebiten.Image][]*ebiten.Image)
	}
	if h <= 0 || w != h*SpriteSheetFrameCount {
		frames := []*ebiten.Image{sprite}
		r.animFrameCache[sprite] = frames
		return frames
	}

	frames := make([]*ebiten.Image, SpriteSheetFrameCount)
	for i := range frames {
		rect := image.Rect(
			bounds.Min.X+i*h, bounds.Min.Y,
			bounds.Min.X+(i+1)*h, bounds.Min.Y+h,
		)
		frames[i] = sprite.SubImage(rect).(*ebiten.Image)
	}
	r.animFrameCache[sprite] = frames
	return frames
}

// selectAnimatedSpriteFrame picks an animation frame from a horizontal sprite
// sheet. If the sprite's width equals frameHeight x SpriteSheetFrameCount, the
// sheet is treated as animated and the frame is selected by frameCount; the
// returned image is a cached SubImage and the returned width/height are the
// per-frame dimensions. Otherwise the sprite is returned unchanged.
func (r *Renderer) selectAnimatedSpriteFrame(sprite *ebiten.Image, frameCount int64) (*ebiten.Image, int, int) {
	bounds := sprite.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if h <= 0 || w != h*SpriteSheetFrameCount {
		return sprite, w, h
	}
	frame := int((frameCount / SpriteFrameStride) % SpriteSheetFrameCount)
	return r.animationFrames(sprite)[frame], h, h
}

// drawProjectiles draws moving spell and weapon projectiles. Melee swings render
// through slashEffects, not this projectile pass.
func (r *Renderer) drawProjectiles(screen *ebiten.Image) {
	r.drawMagicProjectiles(screen)
	r.drawArrows(screen)
}

// projectileProjection bundles the camera-space projection of a point-like
// moving projectile. Returned by projectMovingEntity when the entity passes
// range / FOV / depth-buffer culls.
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

// Spell-hit particle sizing. `scale` (= screenHeight/(relY-fov)) is the same
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
// drawProjectileCollisionBox draws the debug collision box for a flying
// projectile (magic bolt or arrow), scaled and centered on its on-screen sprite.
// No-op for non-positive dimensions. Shared by the projectile and arrow renderers.
func (r *Renderer) drawProjectileCollisionBox(screen *ebiten.Image, screenX, screenY, spriteSize, screenColW, screenColH int, boxColor color.RGBA) {
	if screenColW <= 0 || screenColH <= 0 {
		return
	}
	boxX := screenX - screenColW/2
	boxY := screenY + (spriteSize-screenColH)/2
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

			// Green box for magic projectiles.
			r.drawProjectileCollisionBox(screen, screenX, screenY, projectileSize, screenColW, screenColH, color.RGBA{0, 255, 0, 120})
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
		// Ghost trail: fading copies along the recent flight path. Projectiles
		// fly straight, so past positions are just position - velocity-k - no
		// per-projectile history needed. Drawn before the body so they read as
		// a wake behind it.
		for gi := 1; gi <= 3; gi++ {
			k := float64(gi) * 4
			gproj, gok := r.projectMovingEntity(
				magicProjectile.X-magicProjectile.VelX*k,
				magicProjectile.Y-magicProjectile.VelY*k,
				spellGraphicsConfig.BaseSize, spellGraphicsConfig.MinSize, spellGraphicsConfig.MaxSize)
			if !gok {
				continue
			}
			fade := 1.0 - float64(gi)*0.28
			r.drawGlowSprite(screen,
				float64(gproj.screenX), float64(gproj.screenY)+float64(gproj.size)/2,
				float64(gproj.size)*fxProfile.glowScale*0.8*fade,
				fxProfile.glowColor, 0.35*fade, glowBlend)
		}

		// Soft ambient glow under the projectile (all styles).
		glowSize := float64(projectileSize) * fxProfile.glowScale * pulse * critBoost
		r.drawGlowSprite(screen, centerX, centerY, glowSize, fxProfile.glowColor, 0.6*critBoost, glowBlend)

		dirX, hasDir := r.projectileScreenDir(magicProjectile.VelX, magicProjectile.VelY)
		dirY := 0.0
		if !hasDir {
			dirX = 1 // default trail direction when motion is head-on
		}

		// Spells are always magical -> particle body + evaporating trail (never the
		// old solid square). Drift/mirror come from the school's style; colour comes
		// from the projectile colour, so every school looks distinct.
		r.drawSpellProjectileFx(screen, centerX, centerY, float64(projectileSize), dirX, dirY,
			projectileColor, fxProfile, critBoost, idx)
	}
}

// drawSpellProjectileFx renders a flying spell as a cluster of pixel quads with
// an evaporating trail, instead of a single solid square. "ember" (fire) motes
// flicker hot and rise as they trail; "shard" (ice) bits stay crisp and sink.
// Density/length scale with `size`, so a fireball reads far bigger than a bolt.
func (r *Renderer) drawSpellProjectileFx(screen *ebiten.Image, cx, cy, size, dirX, dirY float64, core [3]int, p projectileFxProfile, critBoost float64, id int) {
	// Floor the cluster size so a bolt launched far from the camera (e.g. a bound
	// lich shooting across the room) still reads as a particle puff rather than a
	// lone dot. Party bolts spawn at the camera (size ~ MaxSize) so they're well
	// above this and unaffected; only distant/small projectiles get the lift.
	if size < spellFxMinClusterSize {
		size = spellFxMinClusterSize
	}
	if draw, ok := spellFxStyleDraw[p.style]; ok {
		draw(r, screen, cx, cy, size, dirX, dirY, core, p, critBoost, id)
		return
	}
	// sink = heavy/cold/void motes fall; others rise like embers/wisps.
	sink := p.style == "shard" || p.style == "dark"
	mirror := p.style == "arcane" // staff/book bolt: trail sweeps the other way (R->L)
	// Hot core = the spell's own colour lightened, so every school reads distinct
	// (fire -> light orange, dark -> light violet, ice -> light blue, ...).
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
		t := (float64(k) + j1) / float64(nTrail) // 0 head -> 1 tail
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

	// Body: a fluffy flickering cluster at the head - many small motes spread
	// wide (radius ~0.7xsize), hot/white core fading to the spell colour at the
	// edges. Smaller per-mote size keeps a big fireball round, not a blocky square.
	nBody := int(size*1.0) + 6
	if nBody > 60 {
		nBody = 60
	}
	bodyR := size * 0.7
	for k := 0; k < nBody; k++ {
		a := auraHash(id, k, 3, int(fc)/2) * 2 * math.Pi
		// sqrt distribution -> denser core, soft round falloff
		rad := math.Sqrt(auraHash(id, k, 4, int(fc)/2)) * bodyR
		px := cx + math.Cos(a)*rad
		py := cy + math.Sin(a)*rad*0.9
		edge := rad / (bodyR + 1) // 0 center -> 1 edge
		qs := size*(0.28-0.13*edge) + 1.5
		col := mixColor(hot, core, edge)
		flick := 0.65 + 0.35*auraHash(id, k, 5, int(fc))
		r.drawGlowSprite(screen, px, py, qs, col, (0.85-0.4*edge)*flick*critBoost, additiveGlowBlend)
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
			// Cyan box for arrows.
			r.drawProjectileCollisionBox(screen, screenX, screenY, arrowSize, screenColW, screenColH, color.RGBA{0, 255, 255, 120})
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
		if strings.EqualFold(bowDef.Category, "blaster") {
			// Blasters fire slugs with a tracer streak, not fletched arrows.
			r.drawBulletTracer(screen, centerX, centerY, float64(arrowSize),
				arrow.VelX, arrow.VelY, arrowColor, critBoost, idx)
			continue
		}
		if fxProfile.style != "" {
			// Staff/book bolt: glowing pixel-particle body + evaporating trail as
			// spells, mirrored (R->L) for arcane. Reuses the spell FX renderer.
			glowSize := float64(arrowSize) * fxProfile.glowScale * critBoost
			r.drawGlowSprite(screen, centerX, centerY, glowSize, fxProfile.glowColor, 0.6*critBoost, glowBlend)
			dirX, ok := r.projectileScreenDir(arrow.VelX, arrow.VelY)
			dirY := 0.0
			if !ok {
				dirX = 1
			}
			if style := bowDef.Graphics.ProjectileFx; style != "" {
				r.drawWeaponProjectileFx(style, screen, centerX, centerY, float64(arrowSize), dirX, dirY, critBoost, idx)
			}
			r.drawSpellProjectileFx(screen, centerX, centerY, float64(arrowSize), dirX, dirY,
				arrowColor, fxProfile, critBoost, idx)
			continue
		}

		// Plain arrow: a fletched shaft with a triangular head, rotated along
		// its on-screen flight direction, lobbed on a shallow arc so its
		// profile shows in flight (a dead-straight arrow shot forward reads as
		// just its tail). The arc is applied to both the current and the
		// one-step-back sample, so the shaft angle follows the arc's tangent -
		// the arrow noses up on the rise and tips down on the fall.
		arcAmp := float64(arrowSize) * 1.3
		maxLife := 1.0
		if bowDef.Physics != nil {
			maxLife = float64(bowDef.Physics.GetLifetimeFrames()) // arrows spawn with exactly this
		}
		flightT := func(lifeLeft float64) float64 {
			t := 1 - lifeLeft/maxLife
			return math.Min(math.Max(t, 0), 1)
		}
		tNow := flightT(float64(arrow.LifeTime))
		tPrev := flightT(float64(arrow.LifeTime) + 3)
		arcNow := arcAmp * 4 * tNow * (1 - tNow)
		centerY -= arcNow

		// Shaft angle from the projected flight delta - but only once the arrow
		// is clear of the camera: right after launch the one-step-back sample
		// sits at/behind the camera plane, where projections swing wildly and
		// made the arrow tumble. The displayed angle is additionally eased
		// per-arrow so a noisy frame can't flip the shaft.
		target := arrowScreenAngle
		camDx := arrow.X - r.game.camera.X
		camDy := arrow.Y - r.game.camera.Y
		if camDx*camDx+camDy*camDy > arrowAngleMinDist*arrowAngleMinDist {
			if prev, pok := r.projectMovingEntity(arrow.X-arrow.VelX*3, arrow.Y-arrow.VelY*3,
				bowDef.Graphics.BaseSize, bowDef.Graphics.MinSize, bowDef.Graphics.MaxSize); pok {
				arcPrev := arcAmp * 4 * tPrev * (1 - tPrev)
				pdx := centerX - float64(prev.screenX)
				pdy := centerY - (float64(prev.screenY) + float64(prev.size)/2 - arcPrev)
				if pdx*pdx+pdy*pdy > 4 {
					target = math.Atan2(pdy, pdx)
				}
			}
		}
		ar := &r.game.arrows[idx]
		if !ar.RenderAngleSet {
			ar.RenderAngle = target
			ar.RenderAngleSet = true
		} else {
			ar.RenderAngle = approachAngle(ar.RenderAngle, target, arrowAngleMaxStep)
		}
		angle := ar.RenderAngle
		if style := bowDef.Graphics.ProjectileFx; style != "" {
			r.drawWeaponProjectileFx(style, screen, centerX, centerY, float64(arrowSize), math.Cos(angle), math.Sin(angle), critBoost, idx)
		}
		shaftLen := float64(arrowSize) * 1.7
		for g := 2; g >= 1; g-- {
			off := shaftLen * 0.5 * float64(g)
			r.drawArrowQuad(screen,
				centerX-math.Cos(angle)*off, centerY-math.Sin(angle)*off,
				float64(arrowSize), angle, arrowColor, 0.4-0.16*float64(g))
		}
		r.drawArrowQuad(screen, centerX, centerY, float64(arrowSize), angle, arrowColor, 1.0)
	}
}

// arrowScreenAngle is the fixed screen tilt arrows fly/stick at (up-left, R->L) -
// same diagonal as the staff bolt - used when the flight is head-on and has no
// usable on-screen direction.
const arrowScreenAngle = -2.7

// arrowAngleMinDist gates the projection-derived shaft angle: closer to the
// camera than this (world units), projections of the one-step-back sample swing
// wildly and the fixed tilt is used instead.
const arrowAngleMinDist = 48.0

// arrowAngleMaxStep caps how fast the displayed shaft angle may turn per frame
// (radians) - the easing that keeps one noisy frame from flipping the arrow.
const arrowAngleMaxStep = 0.12

// drawArrowQuad draws an arrow the shape of a real one - shaft, triangular
// steel head, two swept-back fletching triangles - rotated along `angle` (its
// on-screen flight direction), in the bow's element colour. All five triangles
// go out as ONE DrawTriangles on the white pixel, coloured per vertex; drawn
// source-over (no bloom) so the colour stays vivid. `size` is the
// distance-scaled base size; the arrow is ~1.7x as long.
func (r *Renderer) drawArrowQuad(screen *ebiten.Image, cx, cy, size, angle float64, col [3]int, alpha float64) {
	if size < 2 || alpha <= 0 {
		return
	}
	ca, sa := math.Cos(angle), math.Sin(angle)
	half := size * 0.85               // half length of the whole arrow
	w := math.Max(0.8, size*0.10)     // half width of the shaft
	headLen := size * 0.45            // arrowhead length
	headW := math.Max(1.5, size*0.22) // arrowhead half width
	flLen := size * 0.45              // fletching length along the shaft
	flW := math.Max(1.2, size*0.26)   // fletching height off the shaft

	steel := mixColor(col, [3]int{235, 235, 235}, 0.55)
	feather := mixColor(col, [3]int{255, 255, 255}, 0.35)

	a := float32(alpha)
	verts := r.standeeVerts[:0]
	idx := r.standeeIdx[:0]
	vert := func(lx, ly float64, c [3]int) {
		verts = append(verts, ebiten.Vertex{
			DstX: float32(cx + lx*ca - ly*sa), DstY: float32(cy + lx*sa + ly*ca),
			SrcX: 0.5, SrcY: 0.5,
			ColorR: float32(c[0]) / 255 * a, ColorG: float32(c[1]) / 255 * a,
			ColorB: float32(c[2]) / 255 * a, ColorA: a,
		})
	}
	tri := func(x0, y0, x1, y1, x2, y2 float64, c [3]int) {
		base := uint16(len(verts))
		vert(x0, y0, c)
		vert(x1, y1, c)
		vert(x2, y2, c)
		idx = append(idx, base, base+1, base+2)
	}

	// Shaft: from the tail to the head's base.
	shaftEnd := half - headLen
	tri(-half, -w, shaftEnd, -w, shaftEnd, w, col)
	tri(-half, -w, shaftEnd, w, -half, w, col)
	// Triangular head.
	tri(shaftEnd, -headW, half, 0, shaftEnd, headW, steel)
	// Two fletching triangles swept back off the tail.
	tri(-half+flLen, -w, -half, -w-flW, -half, -w, feather)
	tri(-half+flLen, w, -half, w+flW, -half, w, feather)

	screen.DrawTriangles(verts, idx, r.whiteImg, &ebiten.DrawTrianglesOptions{Blend: ebiten.BlendSourceOver})
	r.standeeVerts = verts[:0]
	r.standeeIdx = idx[:0]
}

// drawSlashEffects draws slash animations for melee weapons
func (r *Renderer) drawSlashEffects(screen *ebiten.Image) {
	if len(r.game.slashEffects) == 0 {
		return
	}
	cx := float64(r.game.config.GetScreenWidth()) / 2
	screenH := float64(r.game.config.GetScreenHeight())
	cy := screenH * meleeAnchorYFrac // lower on screen - it's the party's own weapon
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

			// Anchor the burst on the SAME horizontal projection the sprite uses
			// (camera-plane, via projectToScreenX) - the old centerX+relX*scale
			// used a screenHeight/fov lateral coefficient that disagreed with the
			// sprite's screenW/(2*tan(fov/2)), so off-axis impacts (e.g. an AoE
			// splash on a mob to the side) drew pulled toward screen center.
			anchorX, depth, ok := r.game.renderHelper.projectToScreenX(particle.X, particle.Y)
			if !ok {
				continue
			}

			fov := r.game.camera.FOV
			// Screen-space burst spread scales with perspective; the anchor is
			// already correct, so only the OFFSET rides this scale.
			scale := float64(screenHeight) / (depth * fov)
			screenX := float64(anchorX) + particle.OffsetX*scale
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
			if particle.Star {
				// Twinkling 4-point star (impact_stars - plasma/energy bursts).
				tw := 0.7 + 0.3*math.Sin(float64(r.game.frameCount)*0.45+float64(i*13+j))
				r.drawSparkStar(screen, screenX, screenY, size*0.9,
					particle.Color, mixColor(particle.Color, [3]int{255, 255, 255}, 0.6),
					lifeRatio*tw, 1)
				continue
			}
			// Square pixel particle (matches the impassable-aura / projectile look).
			r.drawGlowRect(screen, screenX, screenY, size, particle.Color, lifeRatio, additiveGlowBlend)
		}
	}
}
