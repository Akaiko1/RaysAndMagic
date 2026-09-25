package game

import "math"

// The camera basis is derived state; key it by its inputs so previews, resize,
// camera turns and repeated Draw calls all use the same rule without a timer.
type renderCameraBasis struct {
	angle, fov, halfFovTan     float64
	dirX, dirY, planeX, planeY float64
	valid                      bool
}

func (r *Renderer) cameraBasis() renderCameraBasis {
	cam := r.game.camera
	b := &r.renderBasis
	if !b.valid || b.angle != cam.Angle || b.fov != cam.FOV {
		*b = renderCameraBasis{angle: cam.Angle, fov: cam.FOV, valid: true}
		b.halfFovTan = math.Tan(cam.FOV / 2)
		b.dirX, b.dirY = math.Cos(cam.Angle), math.Sin(cam.Angle)
		b.planeX = math.Cos(cam.Angle+math.Pi/2) * b.halfFovTan
		b.planeY = math.Sin(cam.Angle+math.Pi/2) * b.halfFovTan
	}
	return *b
}

// CPU-only storage for globally interleaved cross arms. The frame boundary,
// rather than Update's tick, invalidates camera/lighting/texture dependencies.
// Release every image reference at the end of Draw so residency can evict it.
const maxFrameCrossSlabs = 1024

type frameCrossSlab struct {
	slab standeeSlab
	ok   bool
}

type crossedFrameGeometry struct {
	index  map[[3]int]int
	slabs  []frameCrossSlab
	used   int
	active bool
}

func (c *crossedFrameGeometry) begin() {
	c.active = true
	if c.index == nil {
		c.index = make(map[[3]int]int)
	}
}

func (c *crossedFrameGeometry) end() {
	for i := 0; i < c.used; i++ {
		surfaces := c.slabs[i].slab.surfaces
		clear(surfaces)
		c.slabs[i] = frameCrossSlab{slab: standeeSlab{surfaces: surfaces[:0]}}
	}
	clear(c.index)
	c.used = 0
	c.active = false
}

func (c *crossedFrameGeometry) lookup(s UnifiedSpriteRenderData) (standeeSlab, bool, bool) {
	if c.active {
		if i, ok := c.index[[3]int{s.tileX, s.tileY, s.treeArmSlab}]; ok {
			return c.slabs[i].slab, c.slabs[i].ok, true
		}
	}
	return standeeSlab{}, false, false
}

func (c *crossedFrameGeometry) buffer() []standeeSurface {
	if c.used < len(c.slabs) {
		return c.slabs[c.used].slab.surfaces[:0]
	}
	return make([]standeeSurface, 0, standeeMaxShells+2)
}

func (c *crossedFrameGeometry) store(s UnifiedSpriteRenderData, slab standeeSlab, ok bool) {
	if c.used == len(c.slabs) {
		c.slabs = append(c.slabs, frameCrossSlab{})
	}
	c.slabs[c.used] = frameCrossSlab{slab: slab, ok: ok}
	c.index[[3]int{s.tileX, s.tileY, s.treeArmSlab}] = c.used
	c.used++
}
