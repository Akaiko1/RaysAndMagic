package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"image"
)

// Stages of weapon_body.kage's DepthStage uniform, named in its const block.
const (
	depthStageOff   = iota // ordinary material shading
	depthStageHigh         // min-reduce the high depth byte
	depthStageLow          // min-reduce the low byte within the winning high byte
	depthStageShade        // shade only the nearest surface
)

// Two eight-bit min reductions resolve sixteen-bit depth using Ebitengine's
// normal colour targets. The low byte only competes within the winning high
// byte. This avoids intersecting-face artifacts without subdividing the mesh.
var rangedDepthBlend = ebiten.Blend{
	BlendOperationRGB:   ebiten.BlendOperationMin,
	BlendOperationAlpha: ebiten.BlendOperationMin,
}

// One camera-facing triangle that drives every depth-resolve stage at prewarm.
var rangedDepthWarmMesh = compileVolumeMesh([]modelFace{{a: [3]float64{-.5, -.5, 0}, b: [3]float64{.5, -.5, 0}, c: [3]float64{0, .5, 0}, origin: [3]float64{0, 0, 1}, rgb: [3]int{255, 255, 255}, material: meshMetal}})

func (r *Renderer) drawRangedDepthResolved(dst *ebiten.Image, view image.Point, verts []ebiten.Vertex, indices []uint16, bounds image.Rectangle, alpha float64) {
	layer := r.beginEquipmentLayer(view, bounds, verts)
	size := layer.Bounds().Size()
	for i, depth := range r.rangedDepthLayers {
		if depth != nil && depth.Bounds().Size() == size {
			continue
		}
		if depth != nil {
			depth.Deallocate()
		}
		r.rangedDepthLayers[i] = ebiten.NewImageWithOptions(image.Rectangle{Max: size}, &ebiten.NewImageOptions{Unmanaged: true})
	}
	for _, depth := range r.rangedDepthLayers {
		r.fillEquipmentRect(depth, bounds, ebiten.BlendCopy)
	}
	op := &r.rangedMaterialOpts
	op.Blend = rangedDepthBlend
	r.rangedDepthStage[0] = depthStageHigh
	op.Images[0], op.Images[1] = nil, nil
	r.rangedDepthLayers[0].DrawTrianglesShader(verts, indices, r.weaponBodyShader, op)
	r.rangedDepthStage[0] = depthStageLow
	op.Images[0] = r.rangedDepthLayers[0]
	r.rangedDepthLayers[1].DrawTrianglesShader(verts, indices, r.weaponBodyShader, op)
	r.rangedDepthStage[0] = depthStageShade
	op.Images[1] = r.rangedDepthLayers[1]
	op.Blend = ebiten.BlendSourceOver
	layer.DrawTrianglesShader(verts, indices, r.weaponBodyShader, op)
	// Keep the material options self-contained; no depth state leaks to melee.
	r.rangedDepthStage[0] = depthStageOff
	op.Images[0], op.Images[1] = nil, nil
	r.compositeEquipment(dst, layer, bounds, alpha)
}
