package game

import (
	"image"
	"math"
	"strings"

	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

// Material numbers match weapon_ribbon.kage. Authored geometry, colours and
// choreography still belong to each weapon's existing signature renderer.
const (
	weaponMetal = iota
	weaponHeat
	weaponLiquid
	weaponEnergy
	weaponVeil
	weaponStone
	weaponWind
)

type weaponMaterialState struct {
	weaponKey   string
	progress    float64
	hand        int
	material    int
	phase       float64
	seed        int
	trail       meleeTrailProfile
	pose        weaponStrokePose
	bodyHandled bool
}

func weaponMaterial(style string) int {
	switch style {
	case "solstice_thermal", "dragon_ember_egg", "dragon_fang", "fire":
		return weaponHeat
	case "rainfang", "solstice_flow", "dragon_tarn", "serpent_fang", "muramasa", "water":
		return weaponLiquid
	case "solstice_anchor", "idol_breaker", "dragon_roar", "earth":
		return weaponStone
	case "kage_kunai", "dragon_hatchling", "dark":
		return weaponVeil
	case "duskneedle", "solstice_transfer":
		return weaponWind
	case "tonbogiri", "agility_katar", "air", "arcane":
		return weaponEnergy
	}
	if strings.HasPrefix(style, "tech_") {
		return weaponEnergy
	}
	return weaponMetal
}

func (r *Renderer) weaponMaterialClock() float64 {
	if r.game == nil {
		return 0
	}
	tps := config.DefaultTPS
	if r.game.config != nil {
		tps = max(1, r.game.config.GetTPS())
	}
	return float64(r.game.frameCount) / float64(tps)
}

func (r *Renderer) ensureWeaponMaterialShaders() error {
	if r.fireflyShader != nil && r.spellBoltShader != nil && r.weaponRibbonShader != nil && r.impactMaterialShader != nil && r.weaponOrbShader != nil && r.spellBodyShader != nil && r.zonePlumeShader != nil && r.weaponBodyShader != nil && r.bubbleShader != nil {
		return nil
	}
	for _, entry := range []struct {
		dst **ebiten.Shader
		src string
	}{{&r.fireflyShader, fireflyShaderSrc}, {&r.spellBoltShader, spellBoltShaderSrc}, {&r.weaponRibbonShader, weaponRibbonShaderSrc}, {&r.impactMaterialShader, impactMaterialShaderSrc}, {&r.weaponOrbShader, weaponOrbShaderSrc}, {&r.spellBodyShader, spellBodyShaderSrc}, {&r.zonePlumeShader, zonePlumeShaderSrc}, {&r.weaponBodyShader, weaponBodyShaderSrc}, {&r.bubbleShader, bubbleShaderSrc}} {
		if *entry.dst != nil {
			continue
		}
		shader, err := ebiten.NewShader([]byte(entry.src))
		if err != nil {
			return err
		}
		*entry.dst = shader
	}
	return nil
}

var weaponQuadIndices = []uint16{0, 1, 2, 1, 3, 2}
var weaponTriangleIndices = []uint16{0, 1, 2}

func weaponShaderBlend(blend ebiten.Blend) ebiten.Blend {
	if blend == additiveGlowBlend {
		return ebiten.BlendLighter // shaders return premultiplied colour
	}
	return blend
}

func weaponMaterialVertex(x, y, u, v float64, rgb [3]int, alpha float64) ebiten.Vertex {
	return ebiten.Vertex{DstX: float32(x), DstY: float32(y), SrcX: float32(u), SrcY: float32(v),
		ColorR: float32(rgb[0]) / 255, ColorG: float32(rgb[1]) / 255, ColorB: float32(rgb[2]) / 255, ColorA: float32(alpha)}
}

// weaponFxSegment preserves straight shafts, gear teeth, prongs and authored
// silhouette lines while adding a bevel, flowing interior and luminous core.
func (r *Renderer) weaponFxSegment(dst *ebiten.Image, x1, y1, x2, y2, thick float64, rgb [3]int, alpha float64, blend ebiten.Blend) {
	dx, dy := x2-x1, y2-y1
	length := math.Hypot(dx, dy)
	if length <= 0 || thick <= 0 || alpha <= 0 {
		return
	}
	if err := r.ensureWeaponMaterialShaders(); err != nil {
		return
	}
	nx, ny := -dy/length*thick, dx/length*thick
	points := [4][4]float64{{x1 + nx, y1 + ny, 0, -1}, {x2 + nx, y2 + ny, length / 36, -1}, {x1 - nx, y1 - ny, 0, 1}, {x2 - nx, y2 - ny, length / 36, 1}}
	for i, p := range points {
		v := weaponMaterialVertex(p[0], p[1], p[2], p[3], rgb, alpha)
		v.Custom0 = float32(-1 - r.weaponMaterialState.phase)
		v.Custom2 = float32(uint(r.weaponMaterialState.seed)%997) * .013
		v.Custom3 = float32(r.weaponMaterialState.material) + .25
		r.weaponMaterialQuad[i] = v
	}
	r.weaponMaterialOpts.Blend = weaponShaderBlend(blend)
	worldDrawTrianglesShader(dst, r.weaponMaterialQuad[:], weaponQuadIndices, r.weaponRibbonShader, &r.weaponMaterialOpts)
}

// drawWeaponShard keeps one stable face per fragment. Rotation and apparent
// thickness change with its own age/seed; the shader erodes it without shrinking
// it back into a pixel. Matte debris keeps its source colour instead of glowing.
// The longest edge of the unit face is just under 1.67. Impact particles use
// its full diameter; weapon flourishes author the radius instead.
const (
	weaponShardRadiusScale = 1.7
	weaponShardDiameter    = weaponShardRadiusScale * 1.67
)

func (r *Renderer) drawWeaponShard(dst *ebiten.Image, x, y, size float64, rgb [3]int, alpha, age float64, seed int, matte bool) {
	r.drawWeaponShardBlended(dst, x, y, size, rgb, alpha, age, seed, matte, ebiten.BlendSourceOver)
}

func (r *Renderer) drawWeaponShardBlended(dst *ebiten.Image, x, y, size float64, rgb [3]int, alpha, age float64, seed int, matte bool, blend ebiten.Blend) {
	if alpha <= 0 || size <= 0 || age < 0 || age >= 1 {
		return
	}
	if err := r.ensureWeaponMaterialShaders(); err != nil {
		return
	}
	spin := auraHash(seed, 0, 781, 0)*math.Pi*2 + age*(5+9*auraHash(seed, 0, 782, 0))
	if seed&1 != 0 {
		spin = -spin
	}
	ca, sa := math.Cos(spin), math.Sin(spin)
	flip := .18 + .82*math.Abs(math.Cos(spin*.73+float64(seed%13)))
	points := [3][4]float64{{-.8, -.55, 0, 0}, {.85, -.35, 1, 0}, {-.12, .8, 0, 1}}
	for i, p := range points {
		lx, ly := p[0]*size*weaponShardRadiusScale*flip, p[1]*size*weaponShardRadiusScale
		v := weaponMaterialVertex(x+lx*ca-ly*sa, y+lx*sa+ly*ca, p[2], p[3], rgb, alpha)
		v.Custom0, v.Custom1 = float32(age), float32(uint(seed)%997)*.037
		v.Custom2, v.Custom3 = float32(spin), 1
		if matte {
			v.Custom3 = .15
		}
		r.weaponShardVertices[i] = v
	}
	r.weaponMaterialOpts.Blend = weaponShaderBlend(blend)
	worldDrawTrianglesShader(dst, r.weaponShardVertices[:], weaponTriangleIndices, r.impactMaterialShader, &r.weaponMaterialOpts)
}

// weaponFxAccent gives fixed motif details a bevel. These are NOT debris:
// hour marks, teeth, scales and rivets keep their authored location and shape.
func (r *Renderer) weaponFxAccent(dst *ebiten.Image, x, y, size float64, rgb [3]int, alpha float64, blend ebiten.Blend) {
	r.drawWeaponShardBlended(dst, x, y, size*.48, rgb, alpha, 0, r.weaponMaterialState.seed, false, blend)
}

func (r *Renderer) weaponFxPlate(dst *ebiten.Image, x, y, w, h, angle float64, rgb [3]int, alpha float64, blend ebiten.Blend) {
	dx, dy := math.Cos(angle)*w/2, math.Sin(angle)*w/2
	r.weaponFxSegment(dst, x-dx, y-dy, x+dx, y+dy, h*.7, rgb, alpha, blend)
}

// Closed material bands join the pressure collars and machinery that used
// to read as isolated dots. Teeth are reserved for the clockwork signatures.
func (r *Renderer) weaponFxHalo(dst *ebiten.Image, x, y, rx, ry, width float64, teeth int, rgb [3]int, alpha float64) {
	if teeth == 0 {
		// Broad, broken lobes replace the repeated wire-thin ellipse ornament.
		phase := r.weaponMaterialState.phase
		for lobe := 0; lobe < 3; lobe++ {
			offset := float64(lobe)*2.1 + phase*.3
			r.drawDissolveStroke(dst, dissolveStroke{
				path: func(t float64) (float64, float64) {
					a := offset + t*1.25
					warp := 1 + .09*math.Sin(a*3+phase)
					return x + math.Cos(a)*rx*warp, y + math.Sin(a)*ry*warp
				},
				width:  func(t float64) float64 { return math.Max(width, math.Min(rx, ry)*.18) * math.Sin(math.Pi*t) },
				color:  func(t float64) [3]int { return mixColor(rgb, [3]int{245, 248, 255}, t*.3) },
				alpha:  func(t float64) float64 { return alpha * (.45 + .45*math.Sin(math.Pi*t)) },
				length: math.Max(rx, ry) * 1.4, seed: r.weaponMaterialState.seed, salt: lobe + 815, blend: additiveGlowBlend,
			}, 1, -1-phase)
		}
		return
	}

	r.drawDissolveStroke(dst, dissolveStroke{
		path: func(t float64) (float64, float64) {
			a := t * math.Pi * 2
			rad := 1.0
			if teeth > 0 {
				rad += .14 * math.Pow(.5+.5*math.Cos(a*float64(teeth)+r.weaponMaterialState.phase*4), 6)
			}
			return x + math.Cos(a)*rx*rad, y + math.Sin(a)*ry*rad
		},
		width: func(float64) float64 { return width }, color: func(float64) [3]int { return rgb }, alpha: func(float64) float64 { return alpha },
		length: math.Pi * 2 * math.Max(rx, ry), seed: r.weaponMaterialState.seed, salt: 769, blend: additiveGlowBlend,
	}, 1, -1-r.weaponMaterialState.phase)
}

// The stock categories show a compact weapon silhouette at the moving edge.
// Bespoke weapons keep the more specific heads already authored in their FX.
func (r *Renderer) drawWeaponHead(dst *ebiten.Image, kind string, x, y, angle, size, alpha float64) {
	r.drawWeaponSilhouette(dst, kind, "", x, y, angle, size, alpha)
}

func (r *Renderer) drawWeaponCharge(dst *ebiten.Image, x, y, size, dx, dy float64, rgb [3]int, crit, alpha float64, seed int) {
	if size <= 0 || alpha < projectileAlphaThreshold || r.ensureWeaponMaterialShaders() != nil {
		return
	}
	size *= math.Sqrt(crit)
	phase := r.weaponMaterialClock()
	prev := r.weaponMaterialState
	r.weaponMaterialState = weaponMaterialState{material: weaponEnergy, phase: phase, seed: seed}
	defer func() { r.weaponMaterialState = prev }()
	// Both projections retain a round charged core. Only lateral shots trail.
	if math.Hypot(dx, dy) > .01 {
		nx, ny := projectilePerpendicular(dx, dy)
		for strand := 0; strand < 2; strand++ {
			lastX, lastY := x, y
			for k := 1; k <= 12; k++ {
				t := float64(k) / 12
				off := math.Sin(t*5-phase*4+float64(strand)*math.Pi) * size * t * .55
				px, py := x-dx*size*t*3+nx*off, y-dy*size*t*3+ny*off
				r.weaponFxSegment(dst, lastX, lastY, px, py, size*.12*(1-t)+.6, rgb, alpha*(1-t)*.65, additiveGlowBlend)
				lastX, lastY = px, py
			}
		}
	}
	for i, p := range [4][2]float64{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
		v := weaponMaterialVertex(x+p[0]*size, y+p[1]*size, p[0], p[1], rgb, alpha)
		v.Custom0, v.Custom1 = float32(phase), float32(uint(seed)%997)*.037
		r.weaponMaterialQuad[i] = v
	}
	r.weaponMaterialOpts.Blend = ebiten.BlendLighter
	worldDrawTrianglesShader(dst, r.weaponMaterialQuad[:], weaponQuadIndices, r.weaponOrbShader, &r.weaponMaterialOpts)
}

func (r *Renderer) drawWeaponMaterialWarm(dst *ebiten.Image) {
	if r.weaponMaterialWarmed || r.ensureWeaponMaterialShaders() != nil {
		return
	}
	size := dst.Bounds().Size()
	if r.game != nil && r.game.config != nil {
		size = image.Pt(r.game.worldWidth(), r.game.worldHeight())
	}
	r.resizeVolumeLayer(size)
	for i, p := range [4][2]float64{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
		r.weaponMaterialQuad[i] = weaponMaterialVertex(p[0], p[1], p[0], p[1], [3]int{}, 0)
	}
	for _, shader := range []*ebiten.Shader{r.fireflyShader, r.weaponRibbonShader, r.impactMaterialShader, r.weaponOrbShader, r.spellBodyShader, r.zonePlumeShader, r.bubbleShader, r.weaponBodyShader, r.spellBoltShader} {
		if shader == r.zonePlumeShader {
			r.weaponMaterialOpts.Images[0] = r.ensureFireNoise()
		}
		for _, blend := range []ebiten.Blend{ebiten.BlendSourceOver, ebiten.BlendLighter} {
			r.weaponMaterialOpts.Blend = blend
			worldDrawTrianglesShader(dst, r.weaponMaterialQuad[:], weaponQuadIndices, shader, &r.weaponMaterialOpts)
		}
		r.weaponMaterialOpts.Images[0] = nil
	}
	r.weaponMaterialWarmed = true
}

// Arrow triangles keep their shaft, steel head and fletching geometry. Each
// face gets its own bevel and moving reflection rather than a flat fill.
func (r *Renderer) drawWeaponFacets(dst *ebiten.Image, verts []ebiten.Vertex, indices []uint16) {
	if r.ensureWeaponMaterialShaders() != nil {
		return
	}
	uv := [3][2]float32{{0, 0}, {1, 0}, {0, 1}}
	for i := range verts {
		v := &verts[i]
		if v.ColorA > 0 {
			v.ColorR /= v.ColorA
			v.ColorG /= v.ColorA
			v.ColorB /= v.ColorA
		}
		v.SrcX, v.SrcY = uv[i%3][0], uv[i%3][1]
		v.Custom0, v.Custom1 = 0, float32(i/3)*.27
		v.Custom2, v.Custom3 = float32(r.weaponMaterialClock()*.7)+float32(i/3), .7
	}
	r.weaponMaterialOpts.Blend = ebiten.BlendSourceOver
	worldDrawTrianglesShader(dst, verts, indices, r.impactMaterialShader, &r.weaponMaterialOpts)
}

// Blasters retain their rigid energy rod and compact head-on muzzle shape.
func (r *Renderer) drawBulletTracer(dst *ebiten.Image, x, y, size float64, view projectileView, col [3]int, crit float64, id int) {
	// Project the rod's length, rather than normalizing any nonzero sideways
	// component to a full-length line. Its rear face remains visible in depth.
	r.drawWeaponCharge(dst, x, y, size*.85, 0, 0, col, crit, view.faceWeight(), id)
	dx := view.side
	if view.sideWeight() == 0 {
		return
	}
	previous := r.weaponMaterialState
	r.weaponMaterialState = weaponMaterialState{material: weaponEnergy, phase: r.weaponMaterialClock(), seed: id}
	defer func() { r.weaponMaterialState = previous }()
	length := size * 3.6 * crit
	r.weaponFxSegment(dst, x-dx*length*.75, y, x+dx*length*.25, y, size*.45, col, .85, additiveGlowBlend)
	r.weaponFxSegment(dst, x-dx*length*.7, y, x+dx*length*.25, y, size*.13, mixColor(col, [3]int{255, 255, 255}, .65), .9, additiveGlowBlend)
}
