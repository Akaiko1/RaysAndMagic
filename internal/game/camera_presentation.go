package game

import (
	"math"
	"time"
)

// RT presentation interpolates one simulation tick behind. No textures or
// gameplay positions are resampled; TB retains its existing turn animation.
type cameraPose struct{ x, y, angle float64 }
type cameraPresentation struct {
	older, previous, current, presented cameraPose
	tickStart                           time.Time
	epoch                               uint64
	valid, presentedValid, active       bool
	historyValid                        bool
}

func (g *MMGame) cameraPose() cameraPose {
	if g.camera == nil {
		return cameraPose{}
	}
	return cameraPose{g.camera.X, g.camera.Y, g.camera.Angle}
}
func (g *MMGame) resetCameraPresentation() {
	g.cameraPresentation = cameraPresentation{epoch: g.cameraPresentation.epoch + 1}
	// A discontinuous view change cannot transfer a press or held target to
	// the replacement scene. Ordinary movement preserves the displayed pose.
	if gl := g.gameLoop; gl != nil {
		gl.ui.dropQueuedClicks()
		if gl.inputHandler != nil {
			gl.inputHandler.cancelMouseAttack()
		}
	}
}
func (g *MMGame) cameraInterpolationAllowed() bool {
	return !g.turnBasedMode && g.cameraPresentationAllowed()
}
func (g *MMGame) cameraPresentationAllowed() bool {
	return g.camera != nil && g.config != nil && g.appScreen == AppScreenInGame && !g.gameplayPausedByOverlay() &&
		(g.gameLoop == nil || g.gameLoop.loading == nil || !g.gameLoop.loading.awaitingFrame)
}
func (g *MMGame) finishCameraTick(before cameraPose, epoch uint64, started time.Time) {
	if epoch != g.cameraPresentation.epoch || !g.cameraPresentationAllowed() {
		g.resetCameraPresentation()
		return
	}
	p := &g.cameraPresentation
	if g.turnBasedMode {
		// TB eases its turn separately, but picking still needs the last Draw's
		// pose across any number of Updates before the next frame is presented.
		p.valid, p.historyValid = false, false
		return
	}
	step := time.Second / time.Duration(g.config.GetTPS())
	// Early and catch-up Updates share a fixed timeline rather than restarting
	// interpolation at wall-clock time. Re-anchor after a substantial stall.
	if p.valid {
		next := p.tickStart.Add(step)
		if offset := started.Sub(next); offset >= -step && offset < 4*step {
			started = next
		}
	}
	p.older, p.historyValid = p.previous, p.valid
	p.previous, p.current, p.tickStart, p.valid = before, g.cameraPose(), started, true
}
func (g *MMGame) renderCameraPose(now time.Time) cameraPose {
	logical := g.cameraPose()
	p := &g.cameraPresentation
	if !g.cameraInterpolationAllowed() || !p.valid || p.current != logical {
		return logical
	}
	a := now.Sub(p.tickStart).Seconds() * float64(g.config.GetTPS())
	from, to := p.previous, p.current
	// The engine can round a tick up before its nominal time. Keep rendering
	// the preceding segment until that time instead of snapping to its end.
	if a < 0 && p.historyValid {
		from, to = p.older, p.previous
		a++
	}
	a = max(0.0, min(1.0, a))
	return cameraPose{from.x + (to.x-from.x)*a, from.y + (to.y-from.y)*a,
		from.angle + math.Remainder(to.angle-from.angle, 2*math.Pi)*a}
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
	pose := g.renderCameraPose(now)
	if g.turnBasedMode {
		pose.angle = g.viewAngleRender
	}
	g.cameraPresentation.presented, g.cameraPresentation.presentedValid = pose, true
	return g.swapCameraPose(pose)
}

// Screen picking uses the pose that produced the displayed depth buffer;
// interaction range remains a gameplay decision at the logical position.
func (g *MMGame) beginPresentedCameraSwap() func() {
	p := &g.cameraPresentation
	if !p.presentedValid || p.active || !g.cameraPresentationAllowed() {
		return func() {}
	}
	return g.swapCameraPose(p.presented)
}
