package game

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// fakePointer drives the gesture seam with a scripted press/move/release
// sequence, so click-vs-drag arbitration can be tested as the player performs
// it rather than by hand-setting the state under test.
type fakePointer struct {
	x, y                        int
	pressed, justPress, justRel bool
}

// installFakePointer swaps the pointer seam for the duration of a test.
func installFakePointer(t *testing.T) *fakePointer {
	t.Helper()
	fp := &fakePointer{}
	prevPos, prevPressed := pointerPosition, pointerLeftPressed
	prevJustPress, prevJustRel := pointerLeftJustPressed, pointerLeftJustRelease
	prevRight := pointerRightJustPress
	pointerPosition = func() (int, int) { return fp.x, fp.y }
	pointerLeftPressed = func() bool { return fp.pressed }
	pointerLeftJustPressed = func() bool { return fp.justPress }
	pointerLeftJustRelease = func() bool { return fp.justRel }
	pointerRightJustPress = func() bool { return false }
	t.Cleanup(func() {
		pointerPosition, pointerLeftPressed = prevPos, prevPressed
		pointerLeftJustPressed, pointerLeftJustRelease = prevJustPress, prevJustRel
		pointerRightJustPress = prevRight
	})
	return fp
}

func (fp *fakePointer) moveTo(x, y int) { fp.x, fp.y = x, y }

func (fp *fakePointer) press()   { fp.pressed, fp.justPress, fp.justRel = true, true, false }
func (fp *fakePointer) hold()    { fp.pressed, fp.justPress, fp.justRel = true, false, false }
func (fp *fakePointer) release() { fp.pressed, fp.justPress, fp.justRel = false, false, true }
func (fp *fakePointer) idle()    { fp.pressed, fp.justPress, fp.justRel = false, false, false }

// The seam must default to the real device, or production input silently dies.
func TestPointerSeamDefaultsToTheDevice(t *testing.T) {
	if pointerPosition == nil || pointerLeftPressed == nil ||
		pointerLeftJustPressed == nil || pointerLeftJustRelease == nil || pointerRightJustPress == nil {
		t.Fatal("a pointer seam hook is nil")
	}
	// Reading through the seam in a headless test must not panic.
	_, _ = pointerPosition()
	_ = pointerLeftPressed()
	_ = ebiten.MouseButtonLeft
}
