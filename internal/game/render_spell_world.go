package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/shaders"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// Cast flourishes do not own timers or hitboxes. The affected tiles/actors are
// captured by the spell, then use the scene's ordinary painter and pause clock.
type spellWorldVisual struct {
	kind             string
	world            *world.World3D
	target           *monster.Monster3D
	tx, ty           int
	born             int64
	originX, originY float64
	anchor           monsterVisualAnchor
}

const spellWorldVisualSeconds = 2.8
const worldGlacier = 13
const worldFrost = 14
const worldSunPyre = 15

// The registry owns both validation and actor-relative placement. World meshes
// have no actor material; their geometry joins the ordinary scene painter.
var spellWorldStyles = map[string]struct {
	mechanic  string
	material  int
	y, rx, ry float64
}{
	"glacier":      {mechanic: "frozen_ground", material: -1},
	"sun_pyre":     {mechanic: "destroy_undead", material: -1},
	"mind_web":     {"feeblemind", 0, .13, .28, .23},
	"earth_drag":   {"slow", 1, .85, .55, .24},
	"blood_frenzy": {"berserk", 2, .15, .34, .34},
	"holy_dread":   {"turn_undead", 3, .43, .58, .65},
	"dread_shroud": {"mass_fear", 4, .40, .55, .63},
}

func (g *MMGame) expireSpellWorldVisuals() {
	dst := g.spellWorldVisuals[:0]
	for _, v := range g.spellWorldVisuals {
		if v.world == g.world && g.frameCount >= v.born && float64(g.frameCount-v.born)/float64(g.config.GetTPS()) < spellWorldVisualSeconds && (v.target == nil || v.target.IsAlive() || v.kind == "sun_pyre") {
			dst = append(dst, v)
		}
	}
	clear(g.spellWorldVisuals[len(dst):])
	g.spellWorldVisuals = dst
}

func (g *MMGame) startMonsterSpellVisual(def spells.SpellDefinition, m *monster.Monster3D) {
	a, ok := config.GetSpellDefinition(string(def.ID))
	if !ok || a.Graphics == nil || a.Graphics.WorldFx == "" {
		return
	}
	// Refresh the same cast material on a target rather than overdraw copies.
	for i := range g.spellWorldVisuals {
		v := &g.spellWorldVisuals[i]
		if v.target == m && v.kind == a.Graphics.WorldFx {
			v.born = g.frameCount
			return
		}
	}
	x, y := g.combat.monsterVisualPos(m)
	g.spellWorldVisuals = append(g.spellWorldVisuals, spellWorldVisual{kind: a.Graphics.WorldFx, world: g.world, target: m, born: g.frameCount, originX: x, originY: y, anchor: visualAnchorFor(m)})
}

func (r *Renderer) spellWorldBuilder(x, y, alpha float64) *worldModelBuilder {
	ts := r.game.config.GetTileSize()
	ox, oz, ok := r.game.renderHelper.cameraSpaceXY(x, y)
	if !ok {
		return nil
	}
	xx, xz, _ := r.game.renderHelper.cameraSpaceXY(x+ts, y)
	yx, yz, _ := r.game.renderHelper.cameraSpaceXY(x, y+ts)
	b := &r.worldMesh.builder
	*b = worldModelBuilder{r: r, x: x, y: y, ts: ts, alpha: alpha, triangles: b.triangles[:0], origin: [2]float64{ox, oz}, axisX: [2]float64{xx - ox, xz - oz}, axisY: [2]float64{yx - ox, yz - oz}}
	return b
}

func (r *Renderer) collectSunPyres() {
	for _, v := range r.game.spellWorldVisuals {
		if v.kind != "sun_pyre" || v.world != r.game.world {
			continue
		}
		t := float64(r.game.frameCount-v.born) / float64(r.game.config.GetTPS()) / spellWorldVisualSeconds
		b := r.spellWorldBuilder(v.originX, v.originY, smoothStep(t/.05)*(1-smoothStep((t-.55)/.45)))
		if b == nil {
			continue
		}
		// A stationary strike remains at the cast contact after a lethal hit.
		// It is a world plane, so walls, nearer actors and props occlude it.
		width, height := max(.65, v.anchor.sizeTiles*.55), max(2.5, v.anchor.sizeTiles*2.4)
		a := r.game.camera.Angle
		dx, dy := -math.Sin(a)*width, math.Cos(a)*width
		z := v.anchor.heightTiles
		b.quad(worldModelPoint{dx, dy, z}, worldModelPoint{-dx, -dy, z}, worldModelPoint{-dx, -dy, z + height}, worldModelPoint{dx, dy, z + height}, [3]int{255, 230, 153}, worldSunPyre, 1)
		for i := range b.triangles {
			f := &b.triangles[i]
			for j, p := range f.points {
				f.uv[j] = [2]float64{(p.x*(-math.Sin(a)) + p.y*math.Cos(a)) / width, (p.z - z) / height}
			}
			b.progress = t
			b.projectTriangle(f)
		}
	}
}

func (r *Renderer) collectGlacierModels() {
	g := r.game
	ts := g.config.GetTileSize()
	for _, v := range g.spellWorldVisuals {
		if v.kind != "glacier" || v.world != g.world {
			continue
		}
		x, y := TileCenterFromTile(v.tx, v.ty, ts)
		if math.Hypot(x-v.originX, y-v.originY) < ts*.65 {
			continue
		}
		age := float64(g.frameCount-v.born) / float64(g.config.GetTPS())
		// The freeze front advances through the actual cone, near tiles first.
		age -= math.Hypot(x-v.originX, y-v.originY) / ts * .065
		if age <= 0 || age >= 2.25 {
			continue
		}
		growth := smoothStep(age / .50)
		fade := 1 - smoothStep((age-1.5)/.75)
		b := r.spellWorldBuilder(x, y, fade)
		if b == nil {
			continue
		}
		// A frost skin fits the tile exactly; the growing outcrops sit on it.
		rgb := [3]int{139, 221, 249}
		b.quad(worldModelPoint{-.5, -.5, .009}, worldModelPoint{.5, -.5, .009}, worldModelPoint{.5, .5, .009}, worldModelPoint{-.5, .5, .009}, rgb, worldFrost, growth*.78)
		for i := 0; i < 5; i++ {
			seed := auraHash(v.tx, v.ty, 510, i)
			a := float64(i)*2.399 + seed*.6
			rad := .26
			if i == 0 {
				rad = 0
			}
			cx, cy := math.Cos(a)*rad, math.Sin(a)*rad
			height := (.08 + .14*seed) * smoothStep((age-float64(i)*.045)/.43)
			b.iceOutcrop(cx, cy, .19+.035*seed, height, a, seed)
		}
		for i := range b.triangles {
			b.projectTriangle(&b.triangles[i])
		}
	}
}

// Unequal hexagonal facets make a grounded, leaning ice outcrop. Growth moves
// its cap upward; X/Y roots never slide or change the spell's footprint.
func (b *worldModelBuilder) iceOutcrop(x, y, radius, height, angle, seed float64) {
	if height < .003 {
		return
	}
	var bottom, shoulder [6]worldModelPoint
	for i := range bottom {
		a := angle + float64(i)*math.Pi/3
		r := radius * (.88 + .12*math.Sin(float64(i)*2.1+seed*5))
		bottom[i] = worldModelPoint{x + math.Cos(a)*r, y + math.Sin(a)*r, .013}
		shoulder[i] = worldModelPoint{x + math.Cos(a)*r*.66 + height*.15, y + math.Sin(a)*r*.66 - height*.09, height * .67}
	}
	tip := worldModelPoint{x + height*.26, y - height*.14, height}
	for i := range bottom {
		j := (i + 1) % 6
		tint := [3]int{102 + i*7, 183 + i*7, 233 + i*3}
		b.quad(bottom[i], bottom[j], shoulder[j], shoulder[i], tint, worldGlacier, .90)
		b.triangle(shoulder[i], shoulder[j], tip, [3]int{155, 223, 250}, worldGlacier, .95)
	}
}

type actorSpellRenderer struct {
	shader   *ebiten.Shader
	options  ebiten.DrawTrianglesShaderOptions
	vertices []ebiten.Vertex
	indices  []uint16
}

func (r *Renderer) drawMonsterSpellVisuals(dst *ebiten.Image, s UnifiedSpriteRenderData, screenY int) {
	for _, v := range r.game.spellWorldVisuals {
		style, ok := spellWorldStyles[v.kind]
		if !ok || style.material < 0 || v.world != r.game.world || v.target != s.monster {
			continue
		}
		age := float64(r.game.frameCount-v.born) / float64(r.game.config.GetTPS())
		if age < 0 || age >= spellWorldVisualSeconds {
			continue
		}
		m := &r.actorSpell
		if m.shader == nil {
			var err error
			m.shader, err = ebiten.NewShader([]byte(shaders.Source("actor_spell.kage")))
			if err != nil {
				panic(err)
			}
		}
		// Follow the same resolved head used by all actor status indicators,
		// including the pulled-forward TB pose and flying creature height.
		cx, cy := float64(s.screenX), float64(screenY)+float64(s.spriteSize)*style.y
		rx, ry := float64(s.spriteSize)*style.rx, float64(s.spriteSize)*style.ry
		if rx <= 0 {
			continue
		}
		m.vertices, m.indices = m.vertices[:0], m.indices[:0]
		appendSpan := func(left, right int) {
			base := uint16(len(m.vertices))
			for _, p := range [4][2]float64{{float64(left), -1}, {float64(right), -1}, {float64(left), 1}, {float64(right), 1}} {
				vertex := weaponMaterialVertex(p[0], cy+p[1]*ry, (p[0]-cx)/rx, p[1], [3]int{202, 139, 255}, 1)
				vertex.Custom0 = float32(age / spellWorldVisualSeconds)
				vertex.Custom1 = float32(monsterBurnSalt(s.monster.ID)) * .013
				vertex.Custom2 = float32(style.material)
				m.vertices = append(m.vertices, vertex)
			}
			m.indices = append(m.indices, base, base+1, base+2, base+1, base+3, base+2)
		}
		lo, hi := max(0, int(math.Floor(cx-rx))), min(r.game.worldWidth(), int(math.Ceil(cx+rx)))
		start := -1
		for x := lo; x <= hi; x++ {
			visible := x < hi && (x >= len(r.game.depthBuffer) || s.depthPerp < r.game.depthBuffer[x])
			if start >= 0 && !visible {
				appendSpan(start, x)
				start = -1
			}
			if start < 0 && visible {
				start = x
			}
		}
		if len(m.indices) > 0 {
			m.options.Images[0] = r.ensureFireNoise()
			worldDrawTrianglesShader(dst, m.vertices, m.indices, m.shader, &m.options)
		}
	}
}
