package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/config"
	"ugataima/internal/world"
)

// Wall torches (map flag `wall_torches`): a light hangs at every inner wall
// corner - a walkable tile with two perpendicular wall neighbours. The map's
// wall_torch_style picks a burning torch (a world mesh of corner plate, arms,
// handle and fire cup under a flame volume) or a paper lantern on a bracket.
// Each is a flickering point light. Light, detail distances and colours are
// graphics.wall_torches in config.yaml.

// wallTorchCornerInset is how far the flame hangs from the corner along each
// wall, in tiles.
const wallTorchCornerInset = .15625

// wallTorchPoint is one detected corner light: X, Y is its hanging axis.
type wallTorchPoint struct {
	X, Y   float64
	dx, dy int    // the corner's two wall directions
	style  string // config.WallTorchStyle*: its map's wall_torch_style
	seed   int
}

// wallTorchHostTile reports whether a torch may occupy this authored room
// tile. Read TileData directly so traversal buffs such as Walk on Water cannot
// turn water into permanent map dressing while the torch cache is rebuilt.
func wallTorchHostTile(w *world.World3D, x, y int) bool {
	if w == nil || world.GlobalTileManager == nil || x < 0 || y < 0 || x >= w.Width || y >= w.Height {
		return false
	}
	data := world.GlobalTileManager.GetTileData(w.GetTileAtGrid(x, y))
	return data != nil && data.Walkable
}

// wallTorchBackingWall reports whether this tile draws an actual wall face.
// Movement blockers are not interchangeable with walls here: water, scenery,
// and other impassable floor tiles cannot physically support a corner torch.
func wallTorchBackingWall(w *world.World3D, x, y int) bool {
	if w == nil || world.GlobalTileManager == nil || x < 0 || y < 0 || x >= w.Width || y >= w.Height {
		return false
	}
	data := world.GlobalTileManager.GetTileData(w.GetTileAtGrid(x, y))
	return data != nil && data.RenderType == config.TileRenderWall
}

// buildWallTorches scans the world for inner wall corners and caches their
// lights. Called from the same world-change hook that rebuilds the other
// per-map caches. Each corner follows the map it belongs to: on the unified
// world a region takes its own map's setting, wherever the party stands.
func (r *Renderer) buildWallTorches() {
	r.wallTorches = r.wallTorches[:0]
	w := r.game.world
	wm := world.GlobalWorldManager
	if w == nil || wm == nil {
		return
	}
	lit := false
	for key, mc := range wm.MapConfigs {
		lit = lit || mc.WallTorches && wm.WorldByKey(key) == w
	}
	if !lit {
		return
	}
	ts := float64(r.game.config.GetTileSize())
	for ty := 0; ty < w.Height; ty++ {
		for tx := 0; tx < w.Width; tx++ {
			if !wallTorchHostTile(w, tx, ty) {
				continue // torches hang over authored room floor, never water
			}
			// Each pair of perpendicular wall faces = one inner corner.
			for _, c := range [4]struct {
				dx, dy int // wall directions forming the corner
			}{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
				if !wallTorchBackingWall(w, tx+c.dx, ty) || !wallTorchBackingWall(w, tx, ty+c.dy) {
					continue
				}
				mc := wm.MapConfigs[wm.MapKeyAt(w, tx, ty)]
				if mc == nil || !mc.WallTorches {
					continue
				}
				px := (float64(tx) + wallTorchCornerInset) * ts
				if c.dx > 0 {
					px = (float64(tx+1) - wallTorchCornerInset) * ts
				}
				py := (float64(ty) + wallTorchCornerInset) * ts
				if c.dy > 0 {
					py = (float64(ty+1) - wallTorchCornerInset) * ts
				}
				r.wallTorches = append(r.wallTorches, wallTorchPoint{
					X: px, Y: py, dx: c.dx, dy: c.dy, style: mc.WallTorchModel(), seed: (tx*73+ty*19)*4 + (c.dx + 1) + (c.dy+1)/2,
				})
			}
		}
	}
}

// wallTorchFlicker returns the torch's brightness modulation for this frame -
// a slow breathing wave plus per-torch phase so neighbours don't pulse in sync.
func wallTorchFlicker(seed int, seconds float64) float64 {
	phase := auraHash(seed, 0, 11, 0) * 2 * math.Pi
	f := 0.85 + 0.12*math.Sin(seconds*15.6+phase) +
		0.03*math.Sin(seconds*56.4+phase*2)
	return f
}

// The torch model, in tiles: the flame's axis is the origin, the corner lies
// along +X and Z rises from the floor.
const (
	wallTorchFlameScale = .28 // the camp flame volume, shrunk to a torch
	wallTorchCupTopZ    = .575
	wallTorchLeanX      = .423 // the handle's unit axis, leaning back into the corner
	wallTorchLeanZ      = .906
	wallTorchHandle     = .30 // handle length below the cup
	wallTorchCup        = .06
	// The flame volume's root sits at .17 of its own height (world_material.kage).
	wallTorchFlameBase = wallTorchCupTopZ - .012 - .17*wallTorchFlameScale
)

// The paper lantern hangs wholly above the party's eyes (half a tile up), so
// every part a view ray meets first is the lower one.
const (
	paperLanternBottom = .545 // underside of the bottom cap
	paperLanternPaperZ = .568 // where the paper starts
	paperLanternTopZ   = .775 // where the paper ends
	paperLanternRadius = .062 // the paper's widest
	paperLanternArmZ   = .885 // underside of the bracket arm
)

// wallTorchPart is how long a layer stays drawn as the light recedes.
type wallTorchPart int

const (
	wallTorchAlways wallTorchPart = iota // the light itself: the paper
	wallTorchBody                        // handle, cup, caps: past body_tiles only the light is left
)

// wallTorchLayer is a group of convex parts. A light's layers are authored in
// the order the room sees them, back and top first, and drawn in that order:
// its parts touch and pass through one another, so sorting their faces by
// depth would flip them with the view.
type wallTorchLayer struct {
	faces []worldModelTriangle
	part  wallTorchPart
}

// wallTorchModel is one light in two builds: near has every part at full
// detail; far, drawn past detail_tiles, has a few facets per part (and a torch
// loses its bracket), since every face is projected on the CPU each frame.
type wallTorchModel struct {
	near, far []wallTorchLayer
	flame     []worldModelTriangle
	reach     float64
	// The glow the light throws into the air: its height in tiles and its halo.
	glowZ  float64
	halo   config.WallTorchHaloConfig
	sparks bool // a naked flame sheds sparks; a lantern keeps them in
}

// wallTorchModelSet is every built light of one configured style: per
// wall_torch_style and corner direction, each the same model turned about its
// hanging axis.
type wallTorchModelSet struct {
	style  config.WallTorchRenderConfig
	models map[wallTorchModelKey]*wallTorchModel
}

type wallTorchModelKey struct {
	style  string
	corner int
}

// wallTorchModelFor returns the renderer's model for a corner light, built
// from the configured style on first use.
func (r *Renderer) wallTorchModelFor(tp *wallTorchPoint) *wallTorchModel {
	style := r.game.config.Graphics.WallTorches
	if r.wallTorchModels.models == nil || r.wallTorchModels.style != style {
		r.wallTorchModels = wallTorchModelSet{style: style, models: make(map[wallTorchModelKey]*wallTorchModel)}
	}
	key := wallTorchModelKey{tp.style, (tp.dx+1)/2 + (tp.dy + 1)}
	m := r.wallTorchModels.models[key]
	if m == nil {
		if tp.style == config.WallTorchStylePaperLantern {
			m = buildPaperLanternModel(style)
		} else {
			m = buildWallTorchModel(style)
		}
		m.turnToCorner(tp.dx, tp.dy)
		r.wallTorchModels.models[key] = m
	}
	return m
}

// wallTorchLayers collects layers as they are authored.
type wallTorchLayers []wallTorchLayer

func (l *wallTorchLayers) add(part wallTorchPart, build func(b *worldModelBuilder)) {
	b := &worldModelBuilder{}
	build(b)
	*l = append(*l, wallTorchLayer{faces: b.triangles, part: part})
}

// turnToCorner turns a model authored with its corner along +X to the corner
// (dx, dy) and measures its reach.
func (m *wallTorchModel) turnToCorner(dx, dy int) {
	for _, f := range m.flame {
		for _, p := range f.points {
			m.reach = max(m.reach, math.Hypot(p.x, p.y))
		}
	}
	for _, layers := range [][]wallTorchLayer{m.near, m.far} {
		for _, l := range layers {
			rotatePropGeometry(l.faces, 0, 0, math.Atan2(float64(dy), float64(dx)))
			for _, face := range l.faces {
				for _, p := range face.points {
					m.reach = max(m.reach, math.Hypot(p.x, p.y))
				}
			}
		}
	}
}

// wallTorchPlateX is where a light's wall plate stands: it spans the corner
// behind every other part.
const wallTorchPlateX = wallTorchCornerInset*math.Sqrt2 - .034

func buildWallTorchModel(style config.WallTorchRenderConfig) *wallTorchModel {
	m := &wallTorchModel{glowZ: wallTorchCupTopZ + .07, halo: style.TorchHalo, sparks: true}
	m.near, m.far = wallTorchLayersFor(style, true), wallTorchLayersFor(style, false)
	f := &worldModelBuilder{}
	scale := wallTorchFlameScale
	f.propBox(0, 0, wallTorchFlameBase+.17*scale, .33*scale, .33*scale, .83*scale, 0, [3]int{}, worldTorchFlame)
	m.flame = f.triangles
	for i := range m.flame {
		for j, p := range m.flame[i].points {
			m.flame[i].uv[j] = [2]float64{p.x, p.y} // the volume reads its own XY
		}
	}
	return m
}

// wallTorchLayersFor builds the torch: near with its bracket, far without and
// with fewer facets.
func wallTorchLayersFor(style config.WallTorchRenderConfig, near bool) []wallTorchLayer {
	along := func(d float64) worldModelPoint { // a point on the handle's axis, d below the cup top
		return worldModelPoint{wallTorchLeanX * d, 0, wallTorchCupTopZ - wallTorchLeanZ*d}
	}
	foot := wallTorchCup + wallTorchHandle
	collar := wallTorchCup + wallTorchHandle*.42
	handleSides, cupSides, coalSides := 4, 5, 4
	if near {
		handleSides, cupSides, coalSides = 6, 8, 6
	}
	var layers wallTorchLayers
	if near {
		// Against the walls: the plate across the corner and an arm out to each band.
		layers.add(wallTorchBody, func(b *worldModelBuilder) {
			b.propBox(wallTorchPlateX, 0, along(foot).z-.045, .006, .027, .235, 0, style.Iron, worldIron)
			for _, d := range []float64{foot - .012, collar} {
				b.cylinder(along(d), worldModelPoint{wallTorchPlateX, 0, along(d).z}, .008, style.Iron, worldIron, 4)
			}
		})
	}
	layers.add(wallTorchBody, func(b *worldModelBuilder) {
		b.cylinder(along(foot), along(wallTorchCup), .017, style.Wood, worldWood, handleSides)
	})
	if near {
		// Iron bands around the handle: its foot socket and the collar.
		layers.add(wallTorchBody, func(b *worldModelBuilder) {
			b.tube(along(foot+.012), along(foot-.035), .024, style.Iron, worldIron, 6)
			b.tube(along(collar+.013), along(collar-.013), .023, style.Iron, worldIron, 6)
		})
	}
	// Fire cup: a flared iron basket, the coals heaped above its rim.
	layers.add(wallTorchBody, func(b *worldModelBuilder) {
		b.tube(along(wallTorchCup+.01), along(.022), .029, style.Iron, worldIron, cupSides)
		b.cylinder(along(.03), along(.012), .036, style.Iron, worldIron, cupSides)
		b.cylinder(along(.012), along(-.006), .027, style.Coals, worldEmber, coalSides)
	})
	return layers
}

// buildPaperLanternModel is a round paper lantern (chochin) on bamboo ribs,
// with lacquered caps, hung by a cord from a wooden arm in the corner. Seen
// from below, a higher part never hides a lower one, so the layers run top
// down: plate, arm, cord, top cap, paper, bottom cap.
func buildPaperLanternModel(style config.WallTorchRenderConfig) *wallTorchModel {
	glowZ := (paperLanternPaperZ + paperLanternTopZ) / 2
	m := &wallTorchModel{glowZ: glowZ, halo: style.LanternHalo}
	m.near, m.far = paperLanternLayersFor(style, true), paperLanternLayersFor(style, false)
	return m
}

// paperLanternLayersFor builds the lantern; far has fewer facets. The bracket
// stays in both: without its arm a lantern would hang in the air.
func paperLanternLayersFor(style config.WallTorchRenderConfig, near bool) []wallTorchLayer {
	frame := style.LanternFrame
	sides, rings := 6, 3
	if near {
		sides, rings = 10, 5
	}
	var layers wallTorchLayers
	layers.add(wallTorchBody, func(b *worldModelBuilder) {
		b.propBox(wallTorchPlateX, 0, paperLanternArmZ-.07, .006, .03, .14, 0, frame, worldWood)
	})
	layers.add(wallTorchBody, func(b *worldModelBuilder) {
		b.propBox((wallTorchPlateX-.012)/2, 0, paperLanternArmZ, (wallTorchPlateX+.012)/2, .009, .018, 0, frame, worldWood)
	})
	layers.add(wallTorchBody, func(b *worldModelBuilder) {
		b.cylinder(worldModelPoint{0, 0, paperLanternTopZ + .025}, worldModelPoint{0, 0, paperLanternArmZ}, .003, frame, worldWood, 3)
	})
	layers.add(wallTorchBody, func(b *worldModelBuilder) {
		b.cylinder(worldModelPoint{0, 0, paperLanternTopZ - .003}, worldModelPoint{0, 0, paperLanternTopZ + .025}, .036, frame, worldWood, sides)
	})
	// The paper bulges between its caps: a convex barrel.
	layers.add(wallTorchAlways, func(b *worldModelBuilder) {
		profile := make([][2]float64, rings+1)
		for k := range profile {
			t := float64(k) / float64(rings)
			profile[k] = [2]float64{paperLanternRadius * (.78 + .22*math.Sin(math.Pi*t)), paperLanternPaperZ + t*(paperLanternTopZ-paperLanternPaperZ)}
		}
		b.lathe(0, 0, profile, style.LanternPaper, worldPaper, sides)
		// The paper material reads its place on the lantern: round it, and up
		// it. A facet across the seam keeps its turn running one way.
		for i := range b.triangles {
			f := &b.triangles[i]
			last := 0.0
			for j, p := range f.points {
				f.uv[j] = [2]float64{math.Atan2(p.y, p.x)/(2*math.Pi) + .5, (p.z - paperLanternPaperZ) / (paperLanternTopZ - paperLanternPaperZ)}
				last = max(last, f.uv[j][0])
			}
			for j := range f.uv {
				if last-f.uv[j][0] > .5 {
					f.uv[j][0]++
				}
			}
		}
	})
	layers.add(wallTorchBody, func(b *worldModelBuilder) {
		b.cylinder(worldModelPoint{0, 0, paperLanternBottom}, worldModelPoint{0, 0, paperLanternPaperZ + .003}, .034, frame, worldWood, sides)
	})
	return layers
}

// collectWallTorchModels adds every torch the party can see to the world mesh
// pass, lit by the scene and by its own fire.
func (r *Renderer) collectWallTorchModels() {
	g := r.game
	if len(r.wallTorches) == 0 {
		return
	}
	ts := float64(g.config.GetTileSize())
	style := g.config.Graphics.WallTorches
	halfFovTan := g.cameraBasis().halfFovTan
	halfWidth := float64(g.worldWidth()) / 2
	t := g.worldSeconds()
	for i := range r.wallTorches {
		tp := &r.wallTorches[i]
		dist := math.Hypot(tp.X-g.camera.X, tp.Y-g.camera.Y)
		if dist > g.camera.ViewDist+ts {
			continue
		}
		ox, oz, ok := g.renderHelper.cameraSpaceXY(tp.X, tp.Y)
		if !ok || oz < -ts {
			continue
		}
		model := r.wallTorchModelFor(tp)
		reach := model.reach * ts
		if propOutsideView(ox, oz, reach, halfFovTan) || r.wallTorchBehindWalls(halfWidth*(1+ox/oz), halfWidth*reach/halfFovTan/oz, oz) {
			continue
		}
		xx, xz, _ := g.renderHelper.cameraSpaceXY(tp.X+ts, tp.Y)
		yx, yz, _ := g.renderHelper.cameraSpaceXY(tp.X, tp.Y+ts)
		flicker := wallTorchFlicker(tp.seed, t)
		// Its own fire lights the light, however dark the room.
		brightness := math.Max(style.SelfLight*flicker, r.calculateBrightnessWithTorchLight(tp.X, tp.Y, dist))
		firstFace := len(r.worldMesh.faces)
		b := &r.worldMesh.builder
		*b = worldModelBuilder{r: r, x: tp.X, y: tp.Y, ts: ts, alpha: 1,
			origin: [2]float64{ox, oz}, axisX: [2]float64{xx - ox, xz - oz}, axisY: [2]float64{yx - ox, yz - oz}, triangles: b.triangles[:0],
			fireScale: wallTorchFlameScale, fireBase: wallTorchFlameBase, fireSeed: auraHash(tp.seed, 0, 17, 31)}
		layers := model.near
		if dist > style.DetailTiles*ts {
			layers = model.far
		}
		for _, layer := range layers {
			if layer.part == wallTorchBody && dist > style.BodyTiles*ts {
				continue
			}
			for _, f := range layer.faces {
				// Coals and paper give light; the rest is lit.
				light := brightness
				if f.material == worldEmber {
					light = 1
				} else if f.material == worldPaper {
					light = flicker
				}
				for c := range f.rgb {
					f.rgb[c] = int(math.Min(255, float64(f.rgb[c])*light))
				}
				b.projectTriangle(&f)
			}
		}
		for _, f := range model.flame {
			f.alpha = math.Min(1, flicker) // the flame swells with the light it gives
			b.projectTriangle(&f)
		}
		// One depth for the whole torch: the painter keeps equal-depth faces in
		// the order they were added, which is the authored layer order.
		for f := firstFace; f < len(r.worldMesh.faces); f++ {
			r.worldMesh.faces[f].depth = oz
		}
	}
}

// wallTorchBehindWalls reports whether walls hide a torch at depth whose body
// spans half columns either side of the centre column: its middle and both
// edges are behind a wall face. Columns off the view hide nothing.
func (r *Renderer) wallTorchBehindWalls(centre, half, depth float64) bool {
	if depth <= 0 {
		return false
	}
	walls := r.game.depthBuffer
	for _, x := range [3]float64{centre - half, centre, centre + half} {
		column := int(math.Floor(x))
		if column < 0 || column >= len(walls) || depth < walls[column] {
			return false
		}
	}
	return true
}

// drawWallTorchGlow adds the light the flame throws into the air around it: a
// soft halo, and sparks from a naked flame. Called from the unified sprite pass so it
// depth-sorts against billboards and standees; walls occlude by depth here.
func (r *Renderer) drawWallTorchGlow(screen *ebiten.Image, tp wallTorchPoint) {
	horizon := r.game.viewHorizon()
	viewDist := r.game.camera.ViewDist
	depthBuf := r.game.depthBuffer
	t := r.game.worldSeconds()

	screenX, depth, ok := r.game.renderHelper.projectToScreenX(tp.X, tp.Y)
	if !ok || depth < auraMinDepth || depth > viewDist {
		return
	}
	if screenX >= 0 && screenX < len(depthBuf) && depth >= depthBuf[screenX] {
		return // behind a wall face
	}
	floorY := float64(r.game.renderHelper.calculateFloorScreenY(depth))
	halfWall := floorY - horizon
	if halfWall <= 0 {
		return
	}
	model := r.wallTorchModelFor(&tp)
	baseY := floorY - halfWall*2*model.glowZ
	size := math.Max(2, halfWall*0.10)
	flick := wallTorchFlicker(tp.seed, t)
	halo := func() {
		r.drawGlowSprite(screen, float64(screenX), baseY, size*model.halo.Size*flick, model.halo.Color, model.halo.Opacity*flick, additiveGlowBlend)
	}
	if !model.sparks {
		halo()
		return
	}
	spark := r.game.config.Graphics.WallTorches.Spark

	// Crackling sparks: a few tiny embers that FLY out smoothly and die.
	// Each spark's flight is one phase cycle; the cycle index seeds a fresh
	// direction/length every flight, so trajectories never repeat in step -
	// and the per-torch seed keeps neighbouring torches out of sync.
	for k := 0; k < 3; k++ {
		period := .15 + .0833*auraHash(tp.seed, k, 8, 0) // seconds of flight
		cyc := t/period + auraHash(tp.seed, k, 9, 0)
		ph := math.Mod(cyc, 1.0)
		flight := int(cyc) // changes once per flight -> new random trajectory
		if auraHash(tp.seed, k, flight, 1) < 0.45 {
			continue // this spark sits this cycle out
		}
		ang := (0.6 + 1.8*auraHash(tp.seed, k, flight, 2)) * math.Pi // mostly upward fan
		reach := size * (2.0 + 3.0*auraHash(tp.seed, k, flight, 3))
		sx := float64(screenX) + math.Cos(ang)*reach*ph
		sy := baseY - size*0.5 - math.Abs(math.Sin(ang))*reach*ph
		r.drawGlowSprite(screen, sx, sy, math.Max(2.5, size*0.6), spark, flick*(1-ph)*0.9, additiveGlowBlend)
	}

	// Halo last so the sparks above stay one consecutive DrawImage batch.
	halo()
}
