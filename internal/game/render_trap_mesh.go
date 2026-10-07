package game

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/config"
)

// Trap geometry lives in tile units: X/Y follow the map and Z rises from the
// floor. Individual faces join the scene painter, so actors and foliage retain
// their silhouettes. The material clips each fragment against the wall depth.
type trapPoint struct{ x, y, z float64 }
type trapFace struct {
	vertices [3]ebiten.Vertex
	depth    float64
}
type trapMaterialRenderer struct {
	shader   *ebiten.Shader
	opts     ebiten.DrawTrianglesShaderOptions
	clock    [1]float32
	viewport [3]float32
	walls    *ebiten.Image
	pixels   []byte
	faces    []trapFace
	order    []int
	nextFace int
	vertices []ebiten.Vertex
	indices  []uint16
	warmed   bool
	models   map[string]trapCachedModel
	builder  trapModelBuilder
}

type trapBurst struct {
	key, mapKey string
	tx, ty      int
	born        int64
}

const trapBurstSeconds = .85

func (g *MMGame) startTrapBurst(t PlacedTrap) {
	g.trapBursts = append(g.trapBursts, trapBurst{key: t.Key, mapKey: t.MapKey, tx: t.TileX, ty: t.TileY, born: g.frameCount})
}

var trapModels = map[string]func(*trapModelBuilder){
	"cleave_blades": (*trapModelBuilder).cleave,
	"bear_jaws":     (*trapModelBuilder).bear,
	"stasis_rings":  (*trapModelBuilder).stasis,
	"blast_coals":   (*trapModelBuilder).blast,
}

func validateTrapFxStyles() {
	if config.GlobalTrapConfig == nil {
		return
	}
	for key, def := range config.GlobalTrapConfig.Traps {
		if trapModels[def.ArmedFx] == nil {
			panic(fmt.Sprintf("trap %q: unknown armed_fx %q", key, def.ArmedFx))
		}
	}
}

func (r *Renderer) ensureTrapMaterial() error {
	m := &r.trapMaterial
	if m.shader != nil {
		return nil
	}
	shader, err := ebiten.NewShader([]byte(trapMaterialShaderSrc))
	if err != nil {
		return err
	}
	m.shader = shader
	m.opts.Blend = ebiten.BlendSourceOver
	m.opts.Uniforms = map[string]any{"Time": m.clock[:], "Viewport": m.viewport[:]}
	return nil
}

func (r *Renderer) prepareTrapMaterial() {
	m := &r.trapMaterial
	w, h := r.game.worldWidth(), r.game.worldHeight()
	if m.walls == nil || m.walls.Bounds().Dx() != w {
		if m.walls != nil {
			m.walls.Deallocate()
		}
		m.walls = ebiten.NewImage(w, 2)
		m.pixels = make([]byte, 8*w)
	}
	scale := math.Max(r.game.camera.ViewDist, 64) * 2
	m.clock[0] = float32(r.weaponMaterialClock())
	m.viewport = [3]float32{float32(w), float32(h), float32(scale)}
	for x := 0; x < w; x++ {
		d := scale
		if x < len(r.game.depthBuffer) {
			d = r.game.depthBuffer[x]
		}
		code := uint16(math.Round(math.Min(1, math.Max(0, d/scale)) * 65535))
		top := 0.0
		if x < len(r.game.wallTopBuffer) {
			top = float64(r.game.wallTopBuffer[x])
		}
		m.pixels[x*4], m.pixels[x*4+1] = byte(code>>8), byte(code)
		m.pixels[x*4+2], m.pixels[x*4+3] = 0, 255
		// Keep both depth and wall top at 16-bit precision in opaque texels.
		// Encoding top in alpha would corrupt RGB through premultiplication.
		topCode := uint16(math.Round(math.Max(0, math.Min(1, top/float64(h))) * 65535))
		i := (w + x) * 4
		m.pixels[i], m.pixels[i+1], m.pixels[i+2], m.pixels[i+3] = byte(topCode>>8), byte(topCode), 0, 255
	}
	m.walls.WritePixels(m.pixels)
	m.opts.Images[0] = m.walls
}

func (r *Renderer) drawTrapMaterialWarm(dst *ebiten.Image) {
	m := &r.trapMaterial
	if m.warmed || r.ensureTrapMaterial() != nil {
		return
	}
	// Pipeline warmup must also work before a world or camera is available.
	if m.walls == nil {
		m.walls = ebiten.NewImage(1, 2)
		m.pixels = []byte{255, 255, 0, 255, 0, 0, 0, 255}
		m.walls.WritePixels(m.pixels)
	}
	m.viewport = [3]float32{1, 1, 1}
	m.opts.Images[0] = m.walls
	v := [3]ebiten.Vertex{{DstX: 0, DstY: 0, Custom2: 1}, {DstX: 1, DstY: 0, Custom2: 1}, {DstX: 0, DstY: 1, Custom2: 1}}
	dst.DrawTrianglesShader(v[:], weaponTriangleIndices, m.shader, &m.opts)
	m.warmed = true
}

func (r *Renderer) collectTrapModels() {
	m := &r.trapMaterial
	m.faces = m.faces[:0]
	m.order, m.nextFace = m.order[:0], 0
	g := r.game
	if g.world == nil || len(g.traps)+len(g.trapBursts) == 0 {
		return
	}
	ts := float64(g.config.GetTileSize())
	collect := func(key, mapKey string, tx, ty int, progress float64) {
		if !mapKeyOnCurrentWorld(mapKey) {
			return
		}
		def, ok := config.GetTrapDefinition(key)
		if !ok {
			return
		}
		draw := trapModels[def.ArmedFx]
		if draw == nil {
			return
		}
		x, y := (float64(tx)+.5)*ts, (float64(ty)+.5)*ts
		if math.Hypot(x-g.camera.X, y-g.camera.Y) > g.camera.ViewDist+2*ts {
			return
		}
		b := &m.builder
		*b = trapModelBuilder{r: r, x: x, y: y, ts: ts, progress: progress, alpha: 1, accent: def.BorderColor, triangles: b.triangles[:0]}
		// Reuse the common projection rule, then apply its affine basis to the
		// mesh. Camera trigonometry runs once per model, not for every vertex.
		ox, oz, ok := g.renderHelper.cameraSpaceXY(x, y)
		if !ok {
			return
		}
		xx, xz, _ := g.renderHelper.cameraSpaceXY(x+ts, y)
		yx, yz, _ := g.renderHelper.cameraSpaceXY(x, y+ts)
		b.origin, b.axisX, b.axisY = [2]float64{ox, oz}, [2]float64{xx - ox, xz - oz}, [2]float64{yx - ox, yz - oz}
		if progress >= 0 {
			b.alpha = math.Min(1, (1-progress)*4)
		}
		var triangles []trapModelTriangle
		if progress < 0 {
			if m.models == nil {
				m.models = make(map[string]trapCachedModel, len(trapModels))
			}
			cached, ready := m.models[def.ArmedFx]
			if !ready || cached.accent != def.BorderColor {
				draw(b)
				cached = trapCachedModel{accent: def.BorderColor, triangles: append(cached.triangles[:0], b.triangles...)}
				m.models[def.ArmedFx] = cached
			}
			triangles = cached.triangles
		} else {
			// Activation really deforms the model; reuse one scratch buffer.
			draw(b)
			triangles = b.triangles
		}
		for i := range triangles {
			b.projectTriangle(&triangles[i])
		}
	}
	for _, t := range g.traps {
		collect(t.Key, t.MapKey, t.TileX, t.TileY, -1)
	}
	for _, t := range g.trapBursts {
		p := float64(g.frameCount-t.born) / float64(g.config.GetTPS()) / trapBurstSeconds
		if p >= 0 && p < 1 {
			collect(t.key, t.mapKey, t.tx, t.ty, p)
		}
	}
	if len(m.faces) > 0 {
		r.prepareTrapMaterial()
	}
	for i := range m.faces {
		m.order = append(m.order, i)
	}
	// Sort small indices instead of thousands of full scene sprite records.
	// Equal-depth faces retain their authored order deterministically.
	slices.SortFunc(m.order, func(a, b int) int {
		if c := cmp.Compare(m.faces[b].depth, m.faces[a].depth); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
}

// Merge the face stream with the sprite painter, flushing only at an actual
// scenery/actor boundary. Transparent openings retain the correct background.
func (r *Renderer) drawTrapFacesBefore(dst *ebiten.Image, depth float64) {
	m := &r.trapMaterial
	if m.nextFace >= len(m.order) || m.faces[m.order[m.nextFace]].depth < depth {
		return
	}
	r.flushAuraCurtains(dst)
	for m.nextFace < len(m.order) && m.faces[m.order[m.nextFace]].depth >= depth {
		if len(m.vertices) > 60000 {
			r.flushTrapFaces(dst)
		}
		r.appendTrapFace(m.order[m.nextFace])
		m.nextFace++
	}
	r.flushTrapFaces(dst)
}

// Adjacent faces share one material submission without changing painter order.
func (r *Renderer) appendTrapFace(index int) {
	m := &r.trapMaterial
	base := uint16(len(m.vertices))
	m.vertices = append(m.vertices, m.faces[index].vertices[:]...)
	m.indices = append(m.indices, base, base+1, base+2)
}
func (r *Renderer) flushTrapFaces(dst *ebiten.Image) {
	m := &r.trapMaterial
	if len(m.indices) == 0 {
		return
	}
	if r.ensureTrapMaterial() == nil {
		dst.DrawTrianglesShader(m.vertices, m.indices, m.shader, &m.opts)
	}
	m.vertices, m.indices = m.vertices[:0], m.indices[:0]
}

// Static armed models cache tile-space topology, normals and material UVs.
// Only camera-dependent projection, visibility and lighting run every frame.
type trapModelTriangle struct {
	points         [3]trapPoint
	normal, center trapPoint
	uv             [3][2]float64
	rgb            [3]int
	material       int
	alpha          float64
}
type trapCachedModel struct {
	accent    [3]int
	triangles []trapModelTriangle
}
type trapModelBuilder struct {
	triangles                 []trapModelTriangle
	origin, axisX, axisY      [2]float64
	r                         *Renderer
	x, y, ts, progress, alpha float64
	accent                    [3]int
}

type trapClipPoint struct{ x, z, height, u, v float64 }

func (b *trapModelBuilder) triangle(a, c, d trapPoint, rgb [3]int, material int, alpha float64) {
	ux, uy, uz := c.x-a.x, c.y-a.y, c.z-a.z
	vx, vy, vz := d.x-a.x, d.y-a.y, d.z-a.z
	nx, ny, nz := uy*vz-uz*vy, uz*vx-ux*vz, ux*vy-uy*vx
	length := math.Sqrt(nx*nx + ny*ny + nz*nz)
	if length < 1e-10 {
		return
	}
	nx, ny, nz = nx/length, ny/length, nz/length
	flameWidth, flameBottom, flameHeight := 0.0, 0.0, 0.0
	if material == 7 {
		flameBottom = math.Min(a.z, math.Min(c.z, d.z))
		flameHeight = math.Max(a.z, math.Max(c.z, d.z)) - flameBottom
		for _, p := range [3]trapPoint{a, c, d} {
			flameWidth = math.Max(flameWidth, math.Abs(-ny*p.x+nx*p.y))
		}
	}
	face := trapModelTriangle{points: [3]trapPoint{a, c, d}, normal: trapPoint{nx, ny, nz}, center: trapPoint{(a.x + c.x + d.x) / 3, (a.y + c.y + d.y) / 3, (a.z + c.z + d.z) / 3}, rgb: rgb, material: material, alpha: alpha}
	for i, p := range face.points {
		u, v := p.x, p.y
		if math.Abs(nz) < .6 {
			if math.Abs(nx) > math.Abs(ny) {
				u, v = p.y, p.z
			} else {
				u, v = p.x, p.z
			}
		}
		if material == 7 {
			u, v = (-ny*p.x+nx*p.y)/flameWidth*.5, (p.z-flameBottom)/flameHeight
		}
		face.uv[i] = [2]float64{u, v}
	}
	b.triangles = append(b.triangles, face)
}

func (b *trapModelBuilder) projectTriangle(face *trapModelTriangle) {
	nx, ny, nz := face.normal.x, face.normal.y, face.normal.z
	mx, my, mz := face.center.x, face.center.y, face.center.z
	cam := b.r.game.camera
	ex, ey, ez := (cam.X-b.x)/b.ts-mx, (cam.Y-b.y)/b.ts-my, .5-mz
	facing := nx*ex + ny*ey + nz*ez
	if facing < 0 && face.material < 3 {
		return
	}
	if facing < 0 {
		nx, ny, nz = -nx, -ny, -nz
	}
	shade := .3 + .7*math.Max(0, nx*(-.35)+ny*(-.45)+nz*.82)
	eyeLength := math.Max(1e-6, math.Sqrt(ex*ex+ey*ey+ez*ez))
	spec := math.Pow(math.Max(0, (nx*(-.35+ex/eyeLength)+ny*(-.45+ey/eyeLength)+nz*(.82+ez/eyeLength))*.5), 12)
	var source [4]trapClipPoint
	for i, p := range face.points {
		xx := b.origin[0] + p.x*b.axisX[0] + p.y*b.axisY[0]
		zz := b.origin[1] + p.x*b.axisX[1] + p.y*b.axisY[1]
		source[i] = trapClipPoint{xx, zz, p.z * b.ts, face.uv[i][0], face.uv[i][1]}
	}
	// Clip triangles that cross the camera plane, retaining their full tile pose.
	var clipped [4]trapClipPoint
	n := 0
	for i := 0; i < 3; i++ {
		p, q := source[i], source[(i+1)%3]
		inside, next := p.z >= 1, q.z >= 1
		if inside {
			clipped[n] = p
			n++
		}
		if inside != next {
			t := (1 - p.z) / (q.z - p.z)
			clipped[n] = trapClipPoint{p.x + (q.x-p.x)*t, 1, p.height + (q.height-p.height)*t, p.u + (q.u-p.u)*t, p.v + (q.v-p.v)*t}
			n++
		}
	}
	w, h := float64(b.r.game.worldWidth()), float64(b.r.game.worldHeight())
	for k := 1; k+1 < n; k++ {
		f := trapFace{}
		for j, p := range [3]trapClipPoint{clipped[0], clipped[k], clipped[k+1]} {
			q := 1 / p.z
			sx, sy := w*.5*(1+p.x*q), h*.5+h*(.5*b.ts-p.height)*q
			// Pixel-unit shaders add the source atlas origin. Keep coordinates
			// large enough to retain precision before the shader subtracts it.
			vertex := weaponMaterialVertex(sx, sy, p.u*q*1024, p.v*q*1024, face.rgb, face.alpha*b.alpha)
			vertex.Custom0, vertex.Custom1, vertex.Custom2, vertex.Custom3 = float32(shade), float32(spec), float32(q), float32(face.material)+.25
			f.vertices[j] = vertex
			f.depth += p.z / 3
		}
		v := f.vertices
		if math.Max(float64(v[0].DstX), math.Max(float64(v[1].DstX), float64(v[2].DstX))) < 0 || math.Min(float64(v[0].DstX), math.Min(float64(v[1].DstX), float64(v[2].DstX))) > w {
			continue
		}
		if math.Max(float64(v[0].DstY), math.Max(float64(v[1].DstY), float64(v[2].DstY))) < 0 || math.Min(float64(v[0].DstY), math.Min(float64(v[1].DstY), float64(v[2].DstY))) > h {
			continue
		}
		b.r.trapMaterial.faces = append(b.r.trapMaterial.faces, f)
	}
}
func (b *trapModelBuilder) quad(a, c, d, e trapPoint, rgb [3]int, mat int, alpha float64) {
	b.triangle(a, c, d, rgb, mat, alpha)
	b.triangle(a, d, e, rgb, mat, alpha)
}
