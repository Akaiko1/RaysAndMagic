package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"math"
	"ugataima/internal/config"
)

// Resolve presentation once, before selecting a renderer. Visual-only flights
// inherit the source projectile's sizes, with their authored overrides applied.
func (g *MMGame) magicProjectileGraphics(shot MagicProjectile) (config.ProjectileRenderConfig, bool) {
	authored, err := g.config.GetSpellGraphicsConfig(shot.SpellType)
	if err != nil {
		return config.ProjectileRenderConfig{}, false
	}
	graphics := *authored
	if shot.VisualScale > 0 {
		graphics.BaseSize = int(math.Round(float64(graphics.BaseSize) * shot.VisualScale))
		graphics.MinSize = int(math.Round(float64(graphics.MinSize) * shot.VisualScale))
		graphics.MaxSize = int(math.Round(float64(graphics.MaxSize) * shot.VisualScale))
	}
	if shot.VisualColor != nil {
		graphics.Color = *shot.VisualColor
	}
	if shot.VisualFx != "" {
		graphics.ProjectileFx = shot.VisualFx
	}
	return graphics, true
}

func (r *Renderer) spellFanOrigin() (float64, float64) {
	return float64(r.game.worldWidth()) * .5, float64(r.game.worldHeight()) * .82
}

func (r *Renderer) drawPrismaticTargetRay(dst *ebiten.Image, shot MagicProjectile, graphics config.ProjectileRenderConfig) {
	if shot.VisualFrames <= 0 || shot.VisualColor == nil || r.ensureWeaponMaterialShaders() != nil {
		return
	}
	l := shot.Launch
	end, _, ok := r.projectLaunchedEntity(l, l.aimX, l.aimY, shot.VelX, shot.VelY, graphics.BaseSize, graphics.MinSize, graphics.MaxSize)
	if !ok {
		return
	}
	h := float64(r.game.worldHeight())
	x0, y0 := r.spellFanOrigin()
	x1, y1 := float64(end.screenX), float64(end.screenY)+float64(end.size)*.5
	dx, dy := x1-x0, y1-y0
	length := math.Hypot(dx, dy)
	if length < 1 {
		return
	}
	t := 1 - float64(shot.LifeTime)/float64(shot.VisualFrames)
	alpha := smoothStep(t/.07) * (1 - smoothStep((t-.62)/.38))
	nx, ny := -dy/length, dx/length
	// Broad radiance, with a perspective taper. The authored body size owns
	// width; shader coefficients shape the luminous cross-section, not balance.
	width := h * float64(graphics.BaseSize) / r.game.config.GetTileSize() * .14
	for i, p := range [4][4]float64{{x0 - dx*.03 + nx*width, y0 - dy*.03 + ny*width, -.03, 1}, {x1 + dx*.045 + nx*width*.40, y1 + dy*.045 + ny*width*.40, 1.045, 1}, {x0 - dx*.03 - nx*width, y0 - dy*.03 - ny*width, -.03, -1}, {x1 + dx*.045 - nx*width*.40, y1 + dy*.045 - ny*width*.40, 1.045, -1}} {
		v := weaponMaterialVertex(p[0], p[1], p[2], p[3], graphics.Color, alpha)
		v.Custom0, v.Custom1, v.Custom2, v.Custom3 = float32(t), float32(l.angle), float32(math.Min(1.04, t/.20)), float32(spellPrismaticRay)+.25
		r.weaponMaterialQuad[i] = v
	}
	r.weaponMaterialOpts.Blend = ebiten.BlendSourceOver
	worldDrawTrianglesShader(dst, r.weaponMaterialQuad[:], weaponQuadIndices, r.spellBodyShader, &r.weaponMaterialOpts)
}

// One tumbling metal shard per physical projectile. Random direction belongs
// to the cast; the renderer only rotates the solid and converges from the hand.
func (r *Renderer) drawShrapmetalFragment(dst *ebiten.Image, shot MagicProjectile, graphics config.ProjectileRenderConfig) {
	if r.ensureWeaponMaterialShaders() != nil {
		return
	}
	mesh := directionalVolumeMeshes.get(spellShrapmetal, buildDirectionalVolumeMesh)
	l := shot.Launch
	distance := math.Max(1, l.distance)
	travel := (shot.X-l.x)*l.dx + (shot.Y-l.y)*l.dy
	progress := math.Max(0, math.Min(1, travel/distance))
	x0, y0 := r.spellFanOrigin()
	p, _, ok := r.projectLaunchedEntity(l, shot.X, shot.Y, shot.VelX, shot.VelY, graphics.BaseSize, graphics.MinSize, graphics.MaxSize)
	if !ok {
		return
	}
	depth := math.Min(1, progress/.75)
	cx, cy := x0+(float64(p.screenX)-x0)*depth, y0+(float64(p.screenY)+float64(p.size)*.5-y0)*depth
	seed := seedFromID(shot.ID)
	size := float64(p.size) * (.65 + .35*auraHash(seed, 0, 811, 0))
	spin := auraHash(seed, 0, 812, 0)*math.Pi*2 + progress*(7+5*auraHash(seed, 0, 813, 0))
	pitch := spin * .73
	ca, sa, cp, sp := math.Cos(spin), math.Sin(spin), math.Cos(pitch), math.Sin(pitch)
	pose := volumeTransform{rotation: [3][3]float64{{ca, -sa * cp, sa * sp}, {sa, ca * cp, -ca * sp}, {0, sp, cp}}, uvY: 1}
	r.drawCameraVolume(dst, cx, cy, size, 1, pose, r.weaponBodyShader, &r.weaponMaterialOpts, mesh, graphics.Color, meshMetal, false)
}

// Flight presentation per material silhouette. A pointed body foreshortens
// along its flight axis; a fromHand body leaves the caster's hand.
var spellFlightShapes = map[int]struct{ pointed, fromHand bool }{
	spellIce:        {pointed: true, fromHand: true},
	spellVoidNeedle: {pointed: true, fromHand: true},
	spellFireDart:   {pointed: true, fromHand: true},
	spellShadow:     {pointed: true, fromHand: true},
	spellHarm:       {pointed: true, fromHand: true},
	spellLight:      {pointed: true, fromHand: true},
	spellFlyingFist: {pointed: true, fromHand: true},
	spellSoulWisp:   {pointed: true}, // flies from the target back to the party
}

func directionalSpellKind(kind int) bool { return spellFlightShapes[kind].pointed }

// Like outgoing arrows, pointed spells leave the hand in profile, converge on
// the target sightline and expose their end face. Only authored styles qualify.
func (r *Renderer) spellHandOffset(shot MagicProjectile, p projectileFxProfile, view projectileView, width, height int) (float64, float64, float64) {
	kind, ok := spellFxStyleKinds[p.style]
	if !ok || !spellFlightShapes[kind].fromHand || shot.Owner != ProjectileOwnerPlayer || view.incoming() {
		return 0, 0, 0
	}
	convergence := bowHandConvergence(view.distance, float64(r.game.config.GetTileSize())) * view.faceWeight()
	return float64(width) * .055 * convergence, float64(height) * .025 * convergence, convergence
}

func (v projectileView) spellProjection(convergence float64) spellProjection {
	p := spellProjection{dx: 1, head: v.faceWeight(), axial: math.Max(.08, math.Abs(v.side)), axis: v.cameraAxis([3]float64{v.side, 0, v.depth})}
	p.axial += (1 - p.axial) * p.head
	if dir, ok := v.screenDir(); ok {
		p.dx = dir
	}
	if convergence > 0 {
		angle := -math.Pi + .10 + .08*convergence
		p.dx, p.dy, p.head, p.axial = math.Cos(angle), math.Sin(angle), 0, math.Max(.07, convergence)
		p.axis = v.cameraAxis([3]float64{v.side - .45*convergence, -.12 * convergence, v.depth})
	}
	return p
}
