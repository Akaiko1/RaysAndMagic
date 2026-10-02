package game

import "testing"

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
	prevRight, prevCancel := pointerRightJustPress, pointerCancelJustPress
	pointerPosition = func() (int, int) { return fp.x, fp.y }
	pointerLeftPressed = func() bool { return fp.pressed }
	pointerLeftJustPressed = func() bool { return fp.justPress }
	pointerLeftJustRelease = func() bool { return fp.justRel }
	pointerRightJustPress = func() bool { return false }
	pointerCancelJustPress = func() bool { return false }
	t.Cleanup(func() {
		pointerPosition, pointerLeftPressed = prevPos, prevPressed
		pointerLeftJustPressed, pointerLeftJustRelease = prevJustPress, prevJustRel
		pointerRightJustPress, pointerCancelJustPress = prevRight, prevCancel
	})
	return fp
}

func (fp *fakePointer) moveTo(x, y int) { fp.x, fp.y = x, y }

func (fp *fakePointer) press()   { fp.pressed, fp.justPress, fp.justRel = true, true, false }
func (fp *fakePointer) hold()    { fp.pressed, fp.justPress, fp.justRel = true, false, false }
func (fp *fakePointer) release() { fp.pressed, fp.justPress, fp.justRel = false, false, true }
func (fp *fakePointer) idle()    { fp.pressed, fp.justPress, fp.justRel = false, false, false }

// Every seam hook must default to a device read, or production input silently
// dies; reading each one in a headless test must not panic.
func TestPointerSeamDefaultsToTheDevice(t *testing.T) {
	for name, read := range map[string]func(){
		"cancel":       func() { pointerCancelJustPress() },
		"position":     func() { pointerPosition() },
		"wheel":        func() { pointerWheel() },
		"left pressed": func() { pointerLeftPressed() },
		"left press":   func() { pointerLeftJustPressed() },
		"left release": func() { pointerLeftJustRelease() },
		"right press":  func() { pointerRightJustPress() },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("the %s hook is unset or panics headless: %v", name, r)
				}
			}()
			read()
		})
	}
}
