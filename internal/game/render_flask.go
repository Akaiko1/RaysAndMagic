package game

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/config"
)

type flaskRenderPose struct{ x, y, size, angle float64 }

// The arc is presentation only: the same XY projectile still hits bodies,
// doors, walls and its range endpoint. Travel, not Draw time, drives its spin.
func (r *Renderer) flaskProjectilePose(p MagicProjectile) (flaskRenderPose, bool) {
	if !p.Active || p.FlaskKey == "" {
		return flaskRenderPose{}, false
	}
	p.X, p.Y, p.VelX, p.VelY = p.Launch.renderMotion(r.game.combat, p.X, p.Y, p.VelX, p.VelY)
	projected, ok := r.projectMovingEntity(p.X, p.Y, 16, 4, 96)
	if !ok {
		return flaskRenderPose{}, false
	}
	g := r.game
	depth := (p.X-g.camera.X)*math.Cos(g.camera.Angle) + (p.Y-g.camera.Y)*math.Sin(g.camera.Angle)
	if depth <= 0 {
		return flaskRenderPose{}, false
	}
	t := max(0, min(1, 1-p.FlaskRemaining/max(1, p.FlaskFlightRange)))
	tile := float64(g.config.GetTileSize())
	scale := g.viewFocal() / depth
	// Leave the throwing hand, crest above eye level, then fall to the floor.
	height := flaskArcHeight(p, tile)
	return flaskRenderPose{
		x:     float64(projected.screenX) + float64(g.worldWidth())*.13*math.Pow(1-t, 3),
		y:     g.viewHorizon() + (.5*tile-height)*scale,
		size:  max(16, min(96, .25*tile*scale)),
		angle: -.45 + t*3*math.Pi,
	}, true
}
func (r *Renderer) drawFlaskProjectile(screen *ebiten.Image, p MagicProjectile) {
	pose, ok := r.flaskProjectilePose(p)
	if !ok {
		return
	}
	d, ok := config.GetItemDefinition(p.FlaskKey)
	if !ok || d.Flask == nil {
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
	screen.DrawImage(sprite, op)
}
