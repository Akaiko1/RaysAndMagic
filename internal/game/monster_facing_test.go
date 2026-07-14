package game

import (
	"math"
	"testing"

	"ugataima/internal/graphics"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

// stepFacing runs one movement tick through the real capture->move->face
// window: capture, apply the move, face.
func stepFacing(gl *GameLoop, move func()) {
	start := gl.captureMonsterFramePositions()
	move()
	gl.faceMonstersAlongFrameMotion(start)
}

func facingTestLoop(monsters ...*monsterPkg.Monster3D) *GameLoop {
	return &GameLoop{game: &MMGame{world: &world.World3D{Monsters: monsters}}}
}

func TestFaceMonstersAlongFrameMotionUsesActualDisplacement(t *testing.T) {
	m := &monsterPkg.Monster3D{
		X:         1,
		Y:         5,
		Direction: math.Pi,
		HitPoints: 1,
	}

	stepFacing(facingTestLoop(m), func() { m.X = 10 })

	if math.Abs(m.Direction) > 0.0001 {
		t.Fatalf("direction = %.4f, want 0 for eastward movement", m.Direction)
	}
}

func TestFaceMonstersAlongFrameMotionIgnoresTinyJitter(t *testing.T) {
	const original = math.Pi / 2
	m := &monsterPkg.Monster3D{
		X:         5,
		Y:         5,
		Direction: original,
		HitPoints: 1,
	}
	gl := facingTestLoop(m)

	// Back-and-forth jitter cancels in the accumulator and never commits.
	for i := 0; i < 20; i++ {
		delta := 1.0
		if i%2 == 1 {
			delta = -1.0
		}
		stepFacing(gl, func() { m.X += delta })
	}

	if m.Direction != original {
		t.Fatalf("direction = %.4f, want unchanged %.4f", m.Direction, original)
	}
}

// A slow walker whose per-tick step is far under the facing threshold must
// still turn once its walk accumulates past it.
func TestFaceMonstersAlongFrameMotionAccumulatesSlowWalk(t *testing.T) {
	m := &monsterPkg.Monster3D{
		X:         5,
		Y:         5,
		Direction: math.Pi, // stale facing: west
		HitPoints: 1,
	}
	gl := facingTestLoop(m)

	for i := 0; i < 10; i++ { // 0.2px/tick east; commits once 1.5px accumulate
		stepFacing(gl, func() { m.X += 0.2 })
	}

	if math.Abs(m.Direction) > 0.0001 {
		t.Fatalf("direction = %.4f, want 0 after accumulating 2px eastward", m.Direction)
	}
}

// A shove landing outside the capture->face window (separation, band snap,
// blink) must not touch facing.
func TestFaceMonstersAlongFrameMotionIgnoresOutOfWindowShoves(t *testing.T) {
	m := &monsterPkg.Monster3D{
		X:         5,
		Y:         5,
		Direction: 0, // walking east
		HitPoints: 1,
	}
	gl := facingTestLoop(m)

	for i := 0; i < 10; i++ {
		stepFacing(gl, func() { m.X += 0.4 }) // walk east
		m.X -= 2.0                            // post-window shove west (separation/band snap)
	}

	if math.Abs(m.Direction) > 0.0001 {
		t.Fatalf("direction = %.4f, want 0: out-of-window shoves must not flip the walker", m.Direction)
	}
}

// TestMonsterCrowd_BillboardSpriteFacesWalkDirection sweeps a crowd of mobs
// with REAL walking_l/walking_r sheets across many walk headings and camera
// angles, and asserts the billboard path always shows the creature facing its
// on-screen movement direction. Covers both sheet conventions: mobs shipped
// with only a _r sheet (mirrored for leftward motion) and mobs shipped with
// only a _l sheet (mirrored for rightward motion). Direction-from-displacement
// is pinned separately above; this pins Direction -> sprite choice.
func TestMonsterCrowd_BillboardSpriteFacesWalkDirection(t *testing.T) {
	t.Chdir("../..") // real sprite sheets load from assets/

	sm := graphics.NewSpriteManager()
	g := &MMGame{camera: &FirstPersonCamera{}, sprites: sm}
	r := &Renderer{game: g}

	crowd := []string{
		"dire_wolf", "goblin", // shipped with walking_r only
		"dragon", "elf_swordsman", // shipped with walking_l only
	}
	cameraAngles := []float64{0, math.Pi / 3, math.Pi / 2, math.Pi, -math.Pi / 4, -2.1}

	checked := 0
	for _, name := range crowd {
		rightSheet := sm.GetAnimation(name, "walking_r")
		leftSheet := sm.GetAnimation(name, "walking_l")
		if rightSheet == nil && leftSheet == nil {
			t.Fatalf("%s: no walking sheets found (asset moved/renamed?)", name)
		}
		for _, camAngle := range cameraAngles {
			g.camera.Angle = camAngle
			for step := 0; step < 12; step++ {
				mon := &monsterPkg.Monster3D{Direction: 2 * math.Pi * float64(step) / 12}
				screenDir, decisive := r.monsterScreenDir(mon)
				if !decisive {
					continue // moving at/away from the camera: either sheet is fine
				}
				anim, flip := r.getMonsterDirectionalAnimation(name, mon, "walking")
				if anim == nil {
					t.Fatalf("%s: no walk animation resolved", name)
				}
				renderedRight := (anim == rightSheet) != flip
				if renderedRight != (screenDir > 0) {
					t.Errorf("%s cam=%.2f dir=%.2f: sprite faces %s while moving screen-%s (sheet _r=%v flip=%v)",
						name, camAngle, mon.Direction,
						faceWord(renderedRight), faceWord(screenDir > 0), anim == rightSheet, flip)
				}
				checked++
			}
		}
	}
	if checked == 0 {
		t.Fatal("no decisive facing cases checked; sweep is broken")
	}
}

func faceWord(right bool) string {
	if right {
		return "right"
	}
	return "left"
}

// TestMonsterCrowd_StandeeMirrorFacesWalkDirection sweeps token yaw x walk
// heading x camera angle x art convention and asserts the standee mirror
// decision always leaves the creature facing its on-screen movement direction:
// projected art facing (plane U direction, XOR mirror, XOR native art side)
// must equal the heading's on-screen side whenever the decision is decisive.
func TestMonsterCrowd_StandeeMirrorFacesWalkDirection(t *testing.T) {
	checked := 0
	for camStep := 0; camStep < 8; camStep++ {
		camAngle := 2 * math.Pi * float64(camStep) / 8
		camRightX, camRightY := -math.Sin(camAngle), math.Cos(camAngle)
		for yawStep := 0; yawStep < 12; yawStep++ {
			yaw := 2 * math.Pi * float64(yawStep) / 12
			for dirStep := 0; dirStep < 12; dirStep++ {
				dir := 2 * math.Pi * float64(dirStep) / 12
				for _, artFacesLeft := range []bool{false, true} {
					mirror, decisive := standeeMirrorFor(camAngle, yaw, dir, artFacesLeft)
					sDot := math.Cos(yaw)*camRightX + math.Sin(yaw)*camRightY
					dDot := math.Cos(dir)*camRightX + math.Sin(dir)*camRightY
					if !decisive {
						if math.Abs(dDot) > 0.1 {
							t.Fatalf("cam=%.2f yaw=%.2f dir=%.2f: indecisive despite clear heading (dDot=%.2f)", camAngle, yaw, dir, dDot)
						}
						continue
					}
					renderedRight := ((sDot > 0) != mirror) != artFacesLeft
					if renderedRight != (dDot > 0) {
						t.Errorf("cam=%.2f yaw=%.2f dir=%.2f artL=%v: token faces %s while moving screen-%s",
							camAngle, yaw, dir, artFacesLeft, faceWord(renderedRight), faceWord(dDot > 0))
					}
					checked++
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no decisive mirror cases checked; sweep is broken")
	}
}
