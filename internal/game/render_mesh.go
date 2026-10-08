package game

import (
	"image"
	"image/color"
	"math"
	"slices"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

// Cached object-space triangles. The component origin gives outward normals
// even for mirrored hands; material selects a surface in the supplied shader.
type modelFace struct {
	a, b, c, origin [3]float64
	rgb             [3]int
	material        int
}

// Compiled geometry keeps only unique vertices and outward unit normals.
// Construction and degenerate-face rejection happen once, on first use.
type volumeMesh struct {
	points [][3]float64
	faces  []volumeMeshFace
}
type volumeMeshFace struct {
	vertices [3]int
	normal   [3]float64
	rgb      [3]int
	material int
}
type volumeMeshCache struct {
	mu     sync.Mutex
	meshes map[int]*volumeMesh
}

func (c *volumeMeshCache) get(key int, build func(int) []modelFace) *volumeMesh {
	c.mu.Lock()
	defer c.mu.Unlock()
	if m := c.meshes[key]; m != nil {
		return m
	}
	m := compileVolumeMesh(build(key))
	if c.meshes == nil {
		c.meshes = make(map[int]*volumeMesh)
	}
	c.meshes[key] = m
	return m
}
func compileVolumeMesh(raw []modelFace) *volumeMesh {
	m := &volumeMesh{}
	points := make(map[[3]float64]int)
	for _, f := range raw {
		a, b, c := f.a, f.b, f.c
		ux, uy, uz := b[0]-a[0], b[1]-a[1], b[2]-a[2]
		vx, vy, vz := c[0]-a[0], c[1]-a[1], c[2]-a[2]
		nx, ny, nz := uy*vz-uz*vy, uz*vx-ux*vz, ux*vy-uy*vx
		length := math.Sqrt(nx*nx + ny*ny + nz*nz)
		if length < 1e-8 {
			continue
		}
		mx, my, mz := (a[0]+b[0]+c[0])/3, (a[1]+b[1]+c[1])/3, (a[2]+b[2]+c[2])/3
		if nx*(mx-f.origin[0])+ny*(my-f.origin[1])+nz*(mz-f.origin[2]) < 0 {
			nx, ny, nz = -nx, -ny, -nz
		}
		face := volumeMeshFace{normal: [3]float64{nx / length, ny / length, nz / length}, rgb: f.rgb, material: f.material}
		for i, p := range [3][3]float64{a, b, c} {
			index, ok := points[p]
			if !ok {
				index = len(m.points)
				points[p] = index
				m.points = append(m.points, p)
			}
			face.vertices[i] = index
		}
		m.faces = append(m.faces, face)
	}
	return m
}

type litVolumePoint struct{ x, y, z, u, v float64 }
type projectedVolumePoint struct {
	litVolumePoint
	screenX, screenY float64
	visible          bool
}

// Rows of a rigid model-to-camera transform. UV reflection is independent of
// geometry reflection: metal and directional spell materials use different UVs.
type volumeTransform struct {
	rotation [3][3]float64
	uvY      float64
}
type volumeFaceOrder struct {
	depth float64
	index int
}
type litVolumeFace struct {
	vertices [3]int
	normal   [3]float64
	rgb      [3]int
	material int
}

// Camera-local solids share perspective, face culling, ordering and submission.
// The projected local origin stays fixed at (x,y), so its authored tip path is
// also the exact ribbon/contact path. No combat position is changed here.
func (r *Renderer) prepareCameraVolume(view image.Rectangle, x, y, size, alpha float64, transform volumeTransform, mesh *volumeMesh, tint [3]int, materialBase int, equipment bool) ([]ebiten.Vertex, []uint16, image.Rectangle) {
	halfW, halfH := float64(view.Dx())*.5, float64(view.Dy())*.5
	focal := halfW
	if r.game != nil && r.game.camera != nil {
		focal = r.game.viewFocal()
		halfH = r.game.viewHorizon()
		if equipment && r.game.combatPreferences.WideView {
			// Equipment lives in a stable camera-local frame. The world's HUD
			// clearance changes with UI scale; it must not rescale held weapons.
			v := r.game.config.Graphics.View
			viewport := float64(view.Dy()) * v.WideWeaponViewportRatio
			focal, halfH = viewport*v.WideFocalRatio, viewport*v.WideHorizonRatio
		}
	}

	halfW += float64(view.Min.X)
	halfH += float64(view.Min.Y)

	rotation := transform.rotation
	points := r.litVolumePoints[:0]
	for _, p := range mesh.points {
		v := litVolumePoint{
			rotation[0][0]*p[0] + rotation[0][1]*p[1] + rotation[0][2]*p[2],
			rotation[1][0]*p[0] + rotation[1][1]*p[1] + rotation[1][2]*p[2],
			rotation[2][0]*p[0] + rotation[2][1]*p[1] + rotation[2][2]*p[2],
			p[0], p[1] * transform.uvY,
		}
		perspective := 1 / (1 + v.z*size/focal)
		points = append(points, projectedVolumePoint{v, halfW + (x-halfW+v.x*size)*perspective, halfH + (y-halfH+v.y*size)*perspective, false})
	}
	faces := r.litVolumeFaces[:0]
	order := r.litVolumeOrder[:0]
	anchorX, anchorY := (x-halfW)/focal, (y-halfH)/focal
	for _, f := range mesh.faces {
		a, b, c := points[f.vertices[0]], points[f.vertices[1]], points[f.vertices[2]]
		// Rigid transforms rotate cached normals directly, without a callback,
		// a temporary UV payload, or per-face normalization.
		n := f.normal
		nx := rotation[0][0]*n[0] + rotation[0][1]*n[1] + rotation[0][2]*n[2]
		ny := rotation[1][0]*n[0] + rotation[1][1]*n[1] + rotation[1][2]*n[2]
		nz := rotation[2][0]*n[0] + rotation[2][1]*n[1] + rotation[2][2]*n[2]
		mx, my, mz := (a.x+b.x+c.x)/3, (a.y+b.y+c.y)/3, (a.z+b.z+c.z)/3
		if nx*(anchorX+mx*size/focal)+ny*(anchorY+my*size/focal)+nz*(1+mz*size/focal) >= 0 {
			continue
		}
		if 1+math.Min(a.z, math.Min(b.z, c.z))*size/focal <= .02 {
			continue
		}
		for _, vertex := range f.vertices {
			points[vertex].visible = true
		}
		order = append(order, volumeFaceOrder{mz, len(faces)})
		faces = append(faces, litVolumeFace{f.vertices, [3]float64{nx, ny, nz}, [3]int{f.rgb[0] * tint[0] / 255, f.rgb[1] * tint[1] / 255, f.rgb[2] * tint[2] / 255}, materialBase + f.material})
	}

	// Ebitengine's 2D target has no depth buffer. Sorting the visible triangles
	// keeps the thumb and finger joints in front of the palm at every angle.
	// Sort lightweight references instead of repeatedly copying face records.
	slices.SortFunc(order, func(a, b volumeFaceOrder) int {
		if a.depth > b.depth {
			return -1
		}
		if a.depth < b.depth {
			return 1
		}
		return 0
	})
	verts, indices := r.standeeVerts[:0], r.standeeIdx[:0]
	faceAlpha := alpha
	if equipment {
		faceAlpha = 1
	}
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	// Bounds visit each visible vertex once, not once for every incident face.
	for _, p := range points {
		if !p.visible {
			continue
		}
		if p.screenX < minX {
			minX = p.screenX
		}
		if p.screenY < minY {
			minY = p.screenY
		}
		if p.screenX > maxX {
			maxX = p.screenX
		}
		if p.screenY > maxY {
			maxY = p.screenY
		}
	}
	for _, entry := range order {
		f := &faces[entry.index]
		base := uint16(len(verts))
		for _, vertex := range f.vertices {
			p := points[vertex]
			px, py := p.screenX, p.screenY
			v := weaponMaterialVertex(px, py, p.u, p.v, f.rgb, faceAlpha)
			v.Custom0, v.Custom1, v.Custom2, v.Custom3 = float32(f.normal[0]), float32(f.normal[1]), float32(f.normal[2]), float32(f.material)+.25
			verts = append(verts, v)
		}
		indices = append(indices, base, base+1, base+2)
	}
	r.standeeVerts, r.standeeIdx, r.litVolumeFaces = verts[:0], indices[:0], faces[:0]
	r.litVolumeOrder, r.litVolumePoints = order[:0], points[:0]
	if len(indices) == 0 {
		return verts, indices, image.Rectangle{}
	}
	bounds := image.Rect(int(math.Floor(minX)), int(math.Floor(minY)), int(math.Ceil(maxX)), int(math.Ceil(maxY))).Inset(-1).Intersect(view)
	return verts, indices, bounds
}

// Supersample opaque equipment and fade the resolved solid once. The same
// pass handles edge coverage and opacity, without transparent inner surfaces.
const volumeSampleScale = 2

func (r *Renderer) drawCameraVolume(dst *ebiten.Image, x, y, size, alpha float64, transform volumeTransform, shader *ebiten.Shader, opts *ebiten.DrawTrianglesShaderOptions, mesh *volumeMesh, tint [3]int, materialBase int, equipment bool) {
	verts, indices, bounds := r.prepareCameraVolume(dst.Bounds(), x, y, size, alpha, transform, mesh, tint, materialBase, equipment)
	if len(indices) == 0 || bounds.Empty() {
		return
	}
	opts.Blend = ebiten.BlendSourceOver
	if !equipment {
		worldDrawTrianglesShader(dst, verts, indices, shader, opts)
		return
	}
	drawResolvedEquipment(r, dst, verts, indices, bounds, alpha, shader, opts)
}

// Resolve camera-local equipment once, preserving supersampled edges and
// applying its fade after opaque faces. World Panini does not change this frame.
func drawResolvedEquipment(r *Renderer, dst *ebiten.Image, verts []ebiten.Vertex, indices []uint16, bounds image.Rectangle, alpha float64, shader *ebiten.Shader, opts *ebiten.DrawTrianglesShaderOptions) {
	r.resizeVolumeLayer(dst.Bounds().Size())
	layer := r.ensureVolumeLayer(bounds.Size())
	w, h := float64(bounds.Dx()*volumeSampleScale), float64(bounds.Dy()*volumeSampleScale)
	// Reuse one allocation, clear only the rectangle we will sample, and never
	// create per-frame SubImages. Padding keeps the linear resolve off stale pixels.
	r.volumeClearOpts.GeoM.Reset()
	r.volumeClearOpts.GeoM.Scale(w, h)
	r.volumeClearOpts.Blend = ebiten.BlendClear
	layer.DrawImage(r.whiteImg, &r.volumeClearOpts)
	for i := range verts {
		verts[i].DstX = (verts[i].DstX - float32(bounds.Min.X)) * volumeSampleScale
		verts[i].DstY = (verts[i].DstY - float32(bounds.Min.Y)) * volumeSampleScale
	}
	o := worldShaderOptions(nil, opts)
	layer.DrawTrianglesShader(verts, indices, shader, &o)
	for i, p := range [4][2]float64{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
		r.volumeQuad[i] = ebiten.Vertex{DstX: float32(float64(bounds.Min.X) + p[0]*float64(bounds.Dx())), DstY: float32(float64(bounds.Min.Y) + p[1]*float64(bounds.Dy())), SrcX: float32(p[0] * w), SrcY: float32(p[1] * h), ColorR: 1, ColorG: 1, ColorB: 1, ColorA: float32(alpha)}
	}
	r.volumeCompositeOpts.Filter = ebiten.FilterLinear
	r.volumeCompositeOpts.DisableMipmaps = true
	dst.DrawTriangles(r.volumeQuad[:], weaponQuadIndices, layer, &r.volumeCompositeOpts)
}

// Capacity follows the largest observed body footprint in viewport-height
// units. Resizing both grows and shrinks the allocation, without changing model
// scale or sample density. A quarter viewport height is only the initial cache
// reserve: larger bodies grow it from their actual projected bounds.
func (r *Renderer) resizeVolumeLayer(view image.Point) {
	if view == r.volumeLayerView && r.litVolumeLayer != nil {
		return
	}
	r.volumeLayerView = view
	if r.volumeLayerExtent == [2]float64{} {
		r.volumeLayerExtent = [2]float64{.25, .25}
	}
	size := image.Pt(
		min(view.X, int(math.Ceil(r.volumeLayerExtent[0]*float64(view.Y)))),
		min(view.Y, int(math.Ceil(r.volumeLayerExtent[1]*float64(view.Y)))),
	)
	r.allocateVolumeLayer(r.volumeLayerCapacity(size))
}

// Round capacity, not vertices or UVs. Small pose changes reuse the same target.
func (r *Renderer) volumeLayerCapacity(size image.Point) image.Point {
	const bucket = 64
	return image.Pt(
		min(r.volumeLayerView.X, (size.X+bucket-1)/bucket*bucket)*volumeSampleScale,
		min(r.volumeLayerView.Y, (size.Y+bucket-1)/bucket*bucket)*volumeSampleScale,
	)
}

func (r *Renderer) ensureVolumeLayer(size image.Point) *ebiten.Image {
	r.volumeLayerExtent[0] = max(r.volumeLayerExtent[0], float64(size.X)/float64(r.volumeLayerView.Y))
	r.volumeLayerExtent[1] = max(r.volumeLayerExtent[1], float64(size.Y)/float64(r.volumeLayerView.Y))
	capacity := r.volumeLayerCapacity(size)
	if r.litVolumeLayer != nil {
		old := r.litVolumeLayer.Bounds().Size()
		capacity.X, capacity.Y = max(capacity.X, old.X), max(capacity.Y, old.Y)
	}
	r.allocateVolumeLayer(capacity)
	return r.litVolumeLayer
}

func (r *Renderer) allocateVolumeLayer(size image.Point) {
	if r.litVolumeLayer != nil && r.litVolumeLayer.Bounds().Size() == size {
		return
	}
	if r.litVolumeLayer != nil {
		r.litVolumeLayer.Deallocate()
	}
	// This target alternates between writes and reads. Keep dedicated backing
	// instead of migrating it between Ebitengine's destination/source atlases.
	r.litVolumeLayer = ebiten.NewImageWithOptions(image.Rectangle{Max: size}, &ebiten.NewImageOptions{Unmanaged: true})
	if r.weaponBodyShader != nil {
		// Warm the allocation and render-target pipeline before the first swing.
		var quad [4]ebiten.Vertex
		for i, p := range [4][2]float32{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
			quad[i] = ebiten.Vertex{DstX: p[0], DstY: p[1], Custom2: -1, Custom3: .25}
		}
		r.litVolumeLayer.DrawTrianglesShader(quad[:], weaponQuadIndices, r.weaponBodyShader, nil)
	}
	if r.whiteImg == nil {
		r.whiteImg = ebiten.NewImage(1, 1)
		r.whiteImg.Fill(color.White)
	}
}

// Six inset faces, twelve bevel strips and eight corner triangles. The
// same topology is used for each joint; only its proportions and pose differ.
func beveledBoxFaces(center, half [3]float64, bevel, roll float64, face func([3]float64, [3]float64, [3]float64, [3]float64)) {
	transform := func(p [3]float64) [3]float64 {
		return [3]float64{center[0] + p[0], center[1] + p[1]*math.Cos(roll) - p[2]*math.Sin(roll), center[2] + p[1]*math.Sin(roll) + p[2]*math.Cos(roll)}
	}
	emit := func(a, b, c [3]float64) { face(transform(a), transform(b), transform(c), center) }
	for axis := 0; axis < 3; axis++ {
		u, v := (axis+1)%3, (axis+2)%3
		for side := -1; side <= 1; side += 2 {
			var pts [4][3]float64
			for k, p := range [4][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
				pts[k][axis] = float64(side) * half[axis]
				pts[k][u], pts[k][v] = p[0]*(half[u]-bevel), p[1]*(half[v]-bevel)
			}
			emit(pts[0], pts[1], pts[2])
			emit(pts[0], pts[2], pts[3])
		}
	}
	// Bevel strips join the inset faces. Corner gaps close with triangles.
	for edge := 0; edge < 3; edge++ {
		u, v := (edge+1)%3, (edge+2)%3
		for su := -1; su <= 1; su += 2 {
			for sv := -1; sv <= 1; sv += 2 {
				var q [4][3]float64
				for i := range q {
					se := 1.0
					if i == 0 || i == 3 {
						se = -1
					}
					q[i][edge] = se * (half[edge] - bevel)
					if i < 2 {
						q[i][u] = float64(su) * half[u]
						q[i][v] = float64(sv) * (half[v] - bevel)
					} else {
						q[i][u] = float64(su) * (half[u] - bevel)
						q[i][v] = float64(sv) * half[v]
					}
				}
				emit(q[0], q[1], q[2])
				emit(q[0], q[2], q[3])
			}
		}
	}
	for sx := -1; sx <= 1; sx += 2 {
		for sy := -1; sy <= 1; sy += 2 {
			for sz := -1; sz <= 1; sz += 2 {
				var q [3][3]float64
				signs := [3]float64{float64(sx), float64(sy), float64(sz)}
				for i := range q {
					for j := 0; j < 3; j++ {
						h := half[j] - bevel
						if i == j {
							h = half[j]
						}
						q[i][j] = signs[j] * h
					}
				}
				emit(q[0], q[1], q[2])
			}
		}
	}
}
