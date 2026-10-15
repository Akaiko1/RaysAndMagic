package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/config"
)

type flaskRenderPose struct{ x, y, size, angle float64 }

// The arc is presentation only: the same XY projectile still hits bodies,
// doors, walls and its range endpoint. Travel, not Draw time, drives its spin.
func (r *Renderer) flaskProjectilePose(p FlaskProjectile) (flaskRenderPose, bool) {
	if !p.Active {
		return flaskRenderPose{}, false
	}
	g := r.game
	// Leave the throwing hand, crest above eye level, then fall to the floor.
	drop := p.Launch.eyeDrop(g, p.X, p.Y)
	p.X, p.Y, p.VelX, p.VelY = p.Launch.renderMotion(g.combat, p.X, p.Y, p.VelX, p.VelY)
	projected, ok := r.projectMovingEntity(p.X, p.Y, drop, 16, 4, 96)
	if !ok {
		return flaskRenderPose{}, false
	}
	t := max(0, min(1, 1-p.Remaining/max(1, p.Launch.distance)))
	pose := flaskRenderPose{
		x:     float64(projected.screenX) + float64(g.worldWidth())*.13*math.Pow(1-t, 3),
		y:     projected.centerY,
		size:  max(16, min(96, .25*g.config.GetTileSize()*g.viewFocal()/projected.depth)),
		angle: -.45 + t*3*math.Pi,
	}
	// A nearer wall hides a bottle whose centre is below that wall's top.
	if g.wallHides(int(pose.x), projected.depth, 0, pose.y) {
		return flaskRenderPose{}, false
	}
	return pose, true
}
func (r *Renderer) drawFlaskProjectile(screen *ebiten.Image, idx int) {
	if idx < 0 || idx >= len(r.game.flasks) {
		return
	}
	p := r.game.flasks[idx]
	pose, ok := r.flaskProjectilePose(p)
	if !ok {
		return
	}
	d, ok := config.GetItemDefinition(p.Key)
	if !ok || d.Flask == nil {
		return
	}
	if d.Flask.ProjectileFx != "" && r.drawRelicFlask(screen, pose, d.Flask.ProjectileFx) {
		return
	}
	sprite := r.game.sprites.GetSprite(d.Flask.Sprite)
	if sprite == nil {
		return
	}
	b := sprite.Bounds()
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM.Translate(-float64(b.Dx())/2, -float64(b.Dy())/2)
	op.GeoM.Scale(pose.size/float64(b.Dx()), pose.size/float64(b.Dy()))
	op.GeoM.Rotate(pose.angle)
	op.GeoM.Translate(pose.x, pose.y)
	worldDrawImage(screen, sprite, op)
}
