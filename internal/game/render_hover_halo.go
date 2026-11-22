package game

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// The hover halo is a soft glowing rim around the monster under the cursor.
// The mob is drawn once more into an off-screen mask through the very path
// that put it on screen, so the rim follows the token's turn, its
// foreshortening, the walls that cut it and the view projection. The mask's
// alpha is blurred and only the part outside the silhouette reaches the scene.

const (
	// hoverHaloReferenceRows is the view height hover_halo.width_px is authored at.
	hoverHaloReferenceRows = 720.0
	// hover_halo.kage: taps on each side of a pixel, and how many tap steps
	// span the authored width (sigma is four steps). The steps stay about a
	// pixel apart over config.HoverHaloMaxWidthPx.
	hoverHaloTapReach     = 12
	hoverHaloStepsInWidth = 10.0
	// A token end closer than this share of the centre depth stops growing the
	// halo's area; such a token already fills the view.
	hoverHaloNearestEndShare = 0.1
)

type hoverHaloPass struct {
	mask, blur *ebiten.Image
	shader     *ebiten.Shader
	opts       ebiten.DrawTrianglesShaderOptions
	step       [2]float32
	tint       [4]float32
	verts      []ebiten.Vertex
	idx        []uint32
}

// targets returns the mask and blur images, sized like the view.
func (p *hoverHaloPass) targets(bounds image.Rectangle) (mask, blur *ebiten.Image) {
	if p.mask == nil || p.mask.Bounds() != bounds {
		for _, old := range []*ebiten.Image{p.mask, p.blur} {
			if old != nil {
				old.Deallocate()
			}
		}
		// Each target alternates between writes and reads: dedicated backing
		// keeps it out of the shared atlases.
		unmanaged := &ebiten.NewImageOptions{Unmanaged: true}
		p.mask = ebiten.NewImageWithOptions(bounds, unmanaged)
		p.blur = ebiten.NewImageWithOptions(bounds, unmanaged)
	}
	return p.mask, p.blur
}

// mobScreenRect bounds a mob's drawn frame in unprojected view pixels.
func (r *Renderer) mobScreenRect(s UnifiedSpriteRenderData, h monsterPickHit) (left, top, right, bottom float64, ok bool) {
	left, top, right, bottom = h.left, h.top, h.left+h.size, h.bottom
	if !h.standee {
		return left, top, right, bottom, true
	}
	lo, hi, visible := r.game.renderHelper.projectSegmentSpanX(h.p0x, h.p0y, h.p0x+h.dx, h.p0y+h.dy)
	if !visible {
		return 0, 0, 0, 0, false
	}
	left, right = float64(lo), float64(hi+1)
	// The nearer end of a turned token stands taller than its centre.
	basis := r.cameraBasis()
	cam := r.game.camera
	nearest := s.depthPerp
	for _, end := range [2][2]float64{{h.p0x, h.p0y}, {h.p0x + h.dx, h.p0y + h.dy}} {
		nearest = math.Min(nearest, (end[0]-cam.X)*basis.dirX+(end[1]-cam.Y)*basis.dirY)
	}
	scale := s.depthPerp / math.Max(nearest, s.depthPerp*hoverHaloNearestEndShare)
	horizon := float64(r.game.viewHorizon())
	return left, horizon + (top-horizon)*scale, right, horizon + (bottom-horizon)*scale, true
}

// drawMobHoverHalo rims the mob frame f, already drawn on screen, with the
// configured glow.
func (r *Renderer) drawMobHoverHalo(screen *ebiten.Image, s UnifiedSpriteRenderData, f mobFrame, h monsterPickHit) {
	cfg := r.game.config.Graphics.Monster.HoverHalo
	if cfg.Strength <= 0 || cfg.WidthPx <= 0 {
		return
	}
	halo := &r.hoverHalo
	if compileShader(&halo.shader, "hover_halo.kage") != nil {
		return
	}
	left, top, right, bottom, ok := r.mobScreenRect(s, h)
	if !ok {
		return
	}
	bounds := screen.Bounds()
	step := cfg.WidthPx * float64(bounds.Dy()) / hoverHaloReferenceRows / hoverHaloStepsInWidth
	pad := math.Ceil(step*hoverHaloTapReach) + 1

	// Rim quads, one per view column, cut like the mob's own columns: behind a
	// nearer wall only what rises above the wall's top edge shows. A column
	// beside the token takes the depth of the token's nearest column.
	raster := worldRaster(screen)
	tokenFirst, tokenLast := int(left), int(right)-1
	depths, wallTops := r.game.depthBuffer, r.game.wallTopBuffer
	firstX := max(int(math.Floor(left-pad)), bounds.Min.X)
	lastX := min(int(math.Ceil(right+pad)), bounds.Max.X) - 1
	rimTop := math.Max(top-pad, float64(bounds.Min.Y))
	rimBottom := math.Min(bottom+pad, float64(bounds.Max.Y))
	verts, idx := halo.verts[:0], halo.idx[:0]
	area := image.Rectangle{}
	for x := firstX; x <= lastX; x++ {
		columnBottom := rimBottom
		depth, _, _, _, onToken := r.monsterPick.column(h, min(max(x, tokenFirst), tokenLast))
		if !onToken {
			depth = s.depthPerp
		}
		if x >= 0 && x < len(depths) && x < len(wallTops) && standeeColumnOccluded(depth, depths[x], 0) {
			columnBottom = math.Min(columnBottom, float64(wallTops[x]))
		}
		if columnBottom <= rimTop {
			continue
		}
		base := uint32(len(verts))
		for _, corner := range [4][2]float64{{float64(x), rimTop}, {float64(x + 1), rimTop}, {float64(x), columnBottom}, {float64(x + 1), columnBottom}} {
			px, py := corner[0], corner[1]
			if raster != nil {
				px, py = raster.presentedPoint(px, py)
			}
			verts = append(verts, ebiten.Vertex{DstX: float32(px), DstY: float32(py), SrcX: float32(px), SrcY: float32(py), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1})
			area = area.Union(image.Rect(int(math.Floor(px)), int(math.Floor(py)), int(math.Ceil(px))+1, int(math.Ceil(py))+1))
		}
		idx = append(idx, base, base+1, base+2, base+1, base+3, base+2)
	}
	halo.verts, halo.idx = verts[:0], idx[:0]
	if len(idx) == 0 {
		return
	}

	// The silhouette goes through the screen's own projection.
	mask, blur := halo.targets(bounds)
	mask.Clear()
	if raster != nil {
		raster.target = mask
	}
	silhouette := f
	silhouette.rr, silhouette.gg, silhouette.bb, silhouette.fade = 1, 1, 1, 0
	r.drawMob(mask, s, silhouette)
	if raster != nil {
		raster.target = screen
	}

	if halo.opts.Uniforms == nil {
		halo.opts.Uniforms = map[string]any{"Step": halo.step[:], "Tint": halo.tint[:]}
	}
	// Pass one, along X, over every row pass two reads.
	blurArea := area.Inset(-int(pad)).Intersect(bounds)
	halo.step = [2]float32{float32(step), 0}
	halo.tint = [4]float32{}
	halo.opts.Images = [4]*ebiten.Image{mask}
	halo.opts.Blend = ebiten.BlendCopy
	var quad [4]ebiten.Vertex
	for i, corner := range [4]image.Point{blurArea.Min, {X: blurArea.Max.X, Y: blurArea.Min.Y}, {X: blurArea.Min.X, Y: blurArea.Max.Y}, blurArea.Max} {
		x, y := float32(corner.X), float32(corner.Y)
		quad[i] = ebiten.Vertex{DstX: x, DstY: y, SrcX: x, SrcY: y, ColorR: 1, ColorG: 1, ColorB: 1, ColorA: 1}
	}
	blur.DrawTrianglesShader32(quad[:], []uint32{0, 1, 2, 1, 3, 2}, halo.shader, &halo.opts)

	// Pass two, along Y, leaves the rim on the scene.
	halo.step = [2]float32{0, float32(step)}
	halo.tint = [4]float32{float32(cfg.Color[0]) / 255, float32(cfg.Color[1]) / 255, float32(cfg.Color[2]) / 255, float32(cfg.Strength)}
	halo.opts.Images = [4]*ebiten.Image{blur, mask}
	halo.opts.Blend = ebiten.BlendSourceOver
	screen.DrawTrianglesShader32(verts, idx, halo.shader, &halo.opts)
}
