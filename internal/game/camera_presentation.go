package game

import (
	"math"
	"time"
)

// RT presentation interpolates one simulation tick behind. No textures or
// gameplay positions are resampled; TB retains its existing turn animation.
type cameraPose struct{ x, y, angle float64 }
type cameraPresentation struct {
	previous, current, presented  cameraPose
	tickStart                     time.Time
	epoch                         uint64
	valid, presentedValid, active bool
}

func (g *MMGame) cameraPose() cameraPose {
	if g.camera == nil {
		return cameraPose{}
	}
	return cameraPose{g.camera.X, g.camera.Y, g.camera.Angle}
}
func (g *MMGame) resetCameraPresentation() {
	g.cameraPresentation = cameraPresentation{epoch: g.cameraPresentation.epoch + 1}
}
func (g *MMGame) cameraInterpolationAllowed() bool {
	return g.camera != nil && g.config != nil && g.appScreen == AppScreenInGame && !g.turnBasedMode && !g.gameplayPausedByOverlay() &&
		(g.gameLoop == nil || g.gameLoop.loading == nil || !g.gameLoop.loading.awaitingFrame)
}
func (g *MMGame) finishCameraTick(before cameraPose, epoch uint64, started time.Time) {
	if epoch != g.cameraPresentation.epoch || !g.cameraInterpolationAllowed() {
		g.resetCameraPresentation()
		return
	}
	p := &g.cameraPresentation
	step := time.Second / time.Duration(g.config.GetTPS())
	// Catch-up Updates share a fixed timeline, rather than each restarting the
	// interpolation at wall-clock time. Re-anchor after a substantial stall.
	if p.valid {
		next := p.tickStart.Add(step)
		if !next.After(started) && started.Sub(next) < 4*step {
			started = next
		}
	}
	p.previous, p.current, p.tickStart, p.valid = before, g.cameraPose(), started, true
}
func (g *MMGame) renderCameraPose(now time.Time) cameraPose {
	logical := g.cameraPose()
	p := &g.cameraPresentation
	if !g.cameraInterpolationAllowed() || !p.valid || p.current != logical {
		return logical
	}
	a := max(0.0, min(1.0, now.Sub(p.tickStart).Seconds()*float64(g.config.GetTPS())))
	return cameraPose{p.previous.x + (p.current.x-p.previous.x)*a, p.previous.y + (p.current.y-p.previous.y)*a,
		p.previous.angle + math.Remainder(p.current.angle-p.previous.angle, 2*math.Pi)*a}
}
func (g *MMGame) swapCameraPose(pose cameraPose) func() {
	cam := g.camera
	if cam == nil {
		return func() {}
	}
	logical, epoch, wasActive := g.cameraPose(), g.cameraPresentation.epoch, g.cameraPresentation.active
	cam.X, cam.Y, cam.Angle = pose.x, pose.y, pose.angle
	g.cameraPresentation.active = true
	return func() {
		// A load or teleport performed during the pass must survive restoration.
		if g.camera == cam && g.cameraPresentation.epoch == epoch && g.cameraPose() == pose {
			cam.X, cam.Y, cam.Angle = logical.x, logical.y, logical.angle
		}
		g.cameraPresentation.active = wasActive
	}
}
func (g *MMGame) beginRenderCameraSwap(now time.Time) func() {
	if g.turnBasedMode {
		g.cameraPresentation.presentedValid = false
		return g.beginViewAngleSwap()
	}
	pose := g.renderCameraPose(now)
	g.cameraPresentation.presented, g.cameraPresentation.presentedValid = pose, true
	return g.swapCameraPose(pose)
}

// Screen picking uses the pose that produced the displayed depth buffer;
// interaction range remains a gameplay decision at the logical position.
func (g *MMGame) beginPresentedCameraSwap() func() {
	p := &g.cameraPresentation
	if !p.presentedValid || p.active || !g.cameraInterpolationAllowed() {
		return func() {}
	}
	return g.swapCameraPose(p.presented)
}
