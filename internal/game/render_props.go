package game

import (
	"math"

	"ugataima/internal/character"
	"ugataima/internal/config"
)

type propPose struct {
	model                   *propModel
	scale, co, si, progress float64
}

// Rendering, lighting and picking always switch representation together.
func (g *MMGame) usesPropModel(npc *character.NPC) bool {
	return npc.PropModel != nil && g.props3DEnabled()
}

func (g *MMGame) propPose(npc *character.NPC) propPose {
	m := cachedPropModel(*npc.PropModel)
	height, _ := config.ResolveSizeClassTiles(g.config.Graphics.SizeClasses, npc.SizeClass)
	p := g.propUseProgress(npc)
	angle := npc.PropModel.YawDegrees * math.Pi / 180
	return propPose{m, height / m.height, math.Cos(angle), math.Sin(angle), p}
}

func (g *MMGame) propUseProgress(npc *character.NPC) float64 {
	p := 0.0
	if npc.Visited {
		p = 1
		if npc.PropUseStarted > 0 {
			p = math.Max(0, math.Min(1, float64(g.frameCount-(npc.PropUseStarted-1))/float64(g.config.GetTPS())/npc.PropModel.UseSeconds))
		}
	}
	return p * p * (3 - 2*p)
}

// One posed triangle stream serves both drawing and ray picking. Cached
// topology stays immutable; only rigid lid transforms and uniform scale vary.
func (pose propPose) eachFace(visit func(worldModelTriangle)) {
	// Animation remains transient. Static endpoints reuse immutable transformed
	// geometry across both rendering and picking, independent of camera motion.
	if pose.progress != 0 && pose.progress != 1 {
		pose.eachPosedFace(visit)
		return
	}
	state := int(pose.progress)
	key := [3]float64{pose.scale, pose.co, pose.si}
	cached, ok := pose.model.staticPoses[state].Load(key)
	if !ok {
		var faces []worldModelTriangle
		pose.eachPosedFace(func(f worldModelTriangle) { faces = append(faces, f) })
		cached, _ = pose.model.staticPoses[state].LoadOrStore(key, faces)
	}
	for _, f := range cached.([]worldModelTriangle) {
		visit(f)
	}
}

func (pose propPose) eachPosedFace(visit func(worldModelTriangle)) {
	emit := func(f worldModelTriangle) {
		point := func(p worldModelPoint, normal bool) worldModelPoint {
			p.x, p.y = p.x*pose.co-p.y*pose.si, p.x*pose.si+p.y*pose.co
			if !normal {
				p.x *= pose.scale
				p.y *= pose.scale
				p.z *= pose.scale
			}
			return p
		}
		for i, p := range f.points {
			f.points[i] = point(p, false)
		}
		f.center = point(f.center, false)
		f.normal = point(f.normal, true)
		visit(f)
	}
	for _, f := range pose.model.base {
		emit(f)
	}
	smooth := func(t float64) float64 { t = math.Max(0, math.Min(1, t)); return t * t * (3 - 2*t) }
	for _, part := range pose.model.parts {
		angle, dx, dy, dz := 0.0, 0.0, 0.0, 0.0
		if part.falling {
			if pose.progress >= .9 {
				continue
			}
			t := math.Min(1, pose.progress/.82)
			fall := smooth((t - .25) / .75)
			angle = fall * .78
			dy = -.82 * smooth(t/.65)
			// Slide across the short local Y axis, then tumble around X.
			// Both lower lids move during the same opening interval.
			floorCenter := part.size*(.5*math.Abs(math.Cos(angle))+.43*math.Abs(math.Sin(angle))) + .018
			dz = -math.Min(part.pivot.z-floorCenter, .95*fall*fall)
		} else {
			t := pose.progress
			slide := smooth(t / .35)
			lean := smooth((t - .35) / .65)
			angle = part.side * lean * math.Pi / 3
			dx = part.side * (part.size*.5*slide + part.size*.5*math.Cos(angle)*lean)
			finalZ := part.size*.5*math.Abs(math.Sin(angle)) + .027
			dz = (finalZ - part.pivot.z) * lean
		}
		co, si := math.Cos(part.yaw), math.Sin(part.yaw)
		ca, sa := math.Cos(angle), math.Sin(angle)
		point := func(p worldModelPoint, normal bool) worldModelPoint {
			if !normal {
				p.x -= part.pivot.x
				p.y -= part.pivot.y
				p.z -= part.pivot.z
			}
			p.x, p.y = p.x*co+p.y*si, -p.x*si+p.y*co
			if part.falling {
				p.y, p.z = p.y*ca-p.z*sa, p.y*sa+p.z*ca
			} else {
				p.x, p.z = p.x*ca+p.z*sa, -p.x*sa+p.z*ca
			}
			if !normal {
				p.x += dx
				p.y += dy
				p.z += dz
			}
			p.x, p.y = p.x*co-p.y*si, p.x*si+p.y*co
			if !normal {
				p.x += part.pivot.x
				p.y += part.pivot.y
				p.z += part.pivot.z
			}
			return p
		}
		for _, f := range part.faces {
			for i, p := range f.points {
				f.points[i] = point(p, false)
			}
			f.center = point(f.center, false)
			f.normal = point(f.normal, true)
			emit(f)
		}
	}
}

func (r *Renderer) collectPropModels() {
	g := r.game
	if g.world == nil {
		return
	}
	ts := g.config.GetTileSize()
	for _, npc := range g.world.NPCs {
		if !g.usesPropModel(npc) || g.npcAbsent(npc) {
			continue
		}
		dist := math.Hypot(npc.X-g.camera.X, npc.Y-g.camera.Y)
		if dist > g.camera.ViewDist+ts {
			continue
		}
		ox, oz, ok := g.renderHelper.cameraSpaceXY(npc.X, npc.Y)
		if !ok {
			continue
		}
		xx, xz, _ := g.renderHelper.cameraSpaceXY(npc.X+ts, npc.Y)
		yx, yz, _ := g.renderHelper.cameraSpaceXY(npc.X, npc.Y+ts)
		pose := g.propPose(npc)
		// A whole-object extent test retains triangles crossing the near plane.
		if oz < -pose.scale*ts*1.5 {
			continue
		}
		brightness := r.calculateBrightnessWithTorchLight(npc.X, npc.Y, dist)
		brightness = math.Max(g.config.Graphics.BrightnessMin, brightness)
		if dist <= InteractionDistance && g.worldClickAllowed() {
			x, y := g.worldCursorPosition()
			if g.propModelHitTest(npc, x, y) {
				brightness *= standeeHoverBoost
			}
		}
		b := &r.worldMesh.builder
		*b = worldModelBuilder{r: r, x: npc.X, y: npc.Y, ts: ts, alpha: 1,
			origin: [2]float64{ox, oz}, axisX: [2]float64{xx - ox, xz - oz}, axisY: [2]float64{yx - ox, yz - oz}, triangles: b.triangles[:0]}
		pose.eachFace(func(f worldModelTriangle) {
			if pose.model.charredWood && f.material == worldWood {
				for i := range f.rgb {
					f.rgb[i] = int(float64(f.rgb[i])*(1-pose.progress) + 32*pose.progress)
				}
			}
			if f.material == worldEmber {
				if pose.progress >= 1 {
					f.material = worldStone
					f.rgb = [3]int{55, 49, 43}
				} else {
					f.alpha = 1
					for i := range f.rgb {
						f.rgb[i] = int(float64(f.rgb[i]) * (1 - pose.progress))
					}
				}
			}
			if f.material != worldEmber {
				for i := range f.rgb {
					f.rgb[i] = int(math.Min(255, float64(f.rgb[i])*brightness))
				}
			}
			b.projectTriangle(&f)
		})
		if len(pose.model.flame) > 0 && pose.progress < 1 {
			b.fireScale = pose.scale
			b.fireSeed = auraHash(int(npc.X), int(npc.Y), 17, 31)

			pose.eachFlameFace(func(f worldModelTriangle) { b.projectTriangle(&f) })
		}
	}
}

// Picking uses the same conservative volume as the flame shader. Clicking
// a visible tongue remains an interaction with its campfire, not empty space.
func (pose propPose) eachFlameFace(visit func(worldModelTriangle)) {
	if pose.progress >= 1 {
		return
	}

	for _, f := range pose.model.flame {
		for i, p := range f.points {
			p = worldModelPoint{p.x * pose.scale, p.y * pose.scale, p.z * pose.scale}
			f.points[i] = p
			f.uv[i] = [2]float64{p.x, p.y}
		}
		f.center = worldModelPoint{f.center.x * pose.scale, f.center.y * pose.scale, f.center.z * pose.scale}
		f.alpha = 1 - pose.progress
		visit(f)
	}
}

// The inverse of the production world projection. Intersection distance is
// perpendicular camera depth, matching wall columns, including low wall tops.
func (g *MMGame) propModelHitTest(npc *character.NPC, x, y int) bool {
	if x < 0 || x >= g.worldWidth() || y < 0 || y >= g.worldHeight() {
		return false
	}
	ts := g.config.GetTileSize()
	angle := g.camera.Angle
	co, si := math.Cos(angle), math.Sin(angle)
	lateral := (2*(float64(x)+.5)/float64(g.worldWidth()) - 1) * math.Tan(g.viewFOV()/2)
	ray := worldModelPoint{co - si*lateral, si + co*lateral, (g.viewHorizon() - float64(y) - .5) / g.viewFocal()}
	origin := worldModelPoint{(g.camera.X - npc.X) / ts, (g.camera.Y - npc.Y) / ts, .5}
	nearest := math.Inf(1)
	testFace := func(f worldModelTriangle) {
		if t, ok := worldTriangleRay(origin, ray, f.points); ok && t < nearest {
			nearest = t
		}
	}
	pose := g.propPose(npc)
	pose.eachFace(testFace)
	pose.eachFlameFace(testFace)
	if math.IsInf(nearest, 1) {
		return false
	}
	if x < len(g.depthBuffer) && nearest*ts > g.depthBuffer[x]+.07 {
		top := 0.0
		if x < len(g.wallTopBuffer) {
			top = g.wallTopBuffer[x]
		}
		if float64(y)+.5 >= top {
			return false
		}
	}
	return true
}

func worldTriangleRay(origin, ray worldModelPoint, p [3]worldModelPoint) (float64, bool) {
	sub := func(a, b worldModelPoint) worldModelPoint { return worldModelPoint{a.x - b.x, a.y - b.y, a.z - b.z} }
	cross := func(a, b worldModelPoint) worldModelPoint {
		return worldModelPoint{a.y*b.z - a.z*b.y, a.z*b.x - a.x*b.z, a.x*b.y - a.y*b.x}
	}
	dot := func(a, b worldModelPoint) float64 { return a.x*b.x + a.y*b.y + a.z*b.z }
	e1, e2 := sub(p[1], p[0]), sub(p[2], p[0])
	h := cross(ray, e2)
	det := dot(e1, h)
	if math.Abs(det) < 1e-9 {
		return 0, false
	}
	s := sub(origin, p[0])
	u := dot(s, h) / det
	if u < 0 || u > 1 {
		return 0, false
	}
	q := cross(s, e1)
	v := dot(ray, q) / det
	if v < 0 || u+v > 1 {
		return 0, false
	}
	t := dot(e2, q) / det
	return t, t > 0
}

func (r *Renderer) collectPropLights() {
	g := r.game
	if g.world == nil {
		return
	}
	for _, npc := range g.world.NPCs {
		if !g.usesPropModel(npc) || npc.PropModel.LightRadiusTiles <= 0 || g.npcAbsent(npc) {
			continue
		}
		radius := npc.PropModel.LightRadiusTiles * g.config.GetTileSize()
		if math.Hypot(npc.X-g.camera.X, npc.Y-g.camera.Y) > g.camera.ViewDist+radius {
			continue
		}
		intensity := npc.PropModel.LightIntensity * (1 - g.propUseProgress(npc))
		if intensity <= 0 {
			continue
		}
		clock := r.weaponMaterialClock() + auraHash(int(npc.X), int(npc.Y), 7, 5)*11
		intensity *= .91 + .06*math.Sin(clock*4.7) + .03*math.Sin(clock*9.1)
		r.activeLights = append(r.activeLights, LightSource{X: npc.X, Y: npc.Y, Radius: radius, Intensity: intensity})
	}
}
