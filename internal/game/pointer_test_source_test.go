package game

import "testing"

// fakePointer drives the gesture seam with a scripted press/move/release
// sequence, so click-vs-drag arbitration can be tested as the player performs
// it rather than by hand-setting the state under test.
type fakePointer struct {
	x, y                        int
	pressed, justPress, justRel bool
	rightJust                   bool
	shift                       bool // Shift held: a drag carries one unit
}

// installFakePointer swaps the pointer seam for the duration of a test.
func installFakePointer(t *testing.T) *fakePointer {
	t.Helper()
	fp := &fakePointer{}
	prevPos, prevPressed := pointerPosition, pointerLeftPressed
	prevJustPress, prevJustRel := pointerLeftJustPressed, pointerLeftJustRelease
	prevRight, prevCancel, prevShift := pointerRightJustPress, pointerCancelJustPress, shiftModifierHeld
	shiftModifierHeld = func() bool { return fp.shift }
	pointerPosition = func() (int, int) { return fp.x, fp.y }
	pointerLeftPressed = func() bool { return fp.pressed }
	pointerLeftJustPressed = func() bool { return fp.justPress }
	pointerLeftJustRelease = func() bool { return fp.justRel }
	pointerRightJustPress = func() bool { return fp.rightJust }
	pointerCancelJustPress = func() bool { return false }
	t.Cleanup(func() {
		pointerPosition, pointerLeftPressed = prevPos, prevPressed
		pointerLeftJustPressed, pointerLeftJustRelease = prevJustPress, prevJustRel
		pointerRightJustPress, pointerCancelJustPress, shiftModifierHeld = prevRight, prevCancel, prevShift
	})
	return fp
}

func (fp *fakePointer) moveTo(x, y int) { fp.x, fp.y = x, y }

func (fp *fakePointer) press() {
	fp.pressed, fp.justPress, fp.justRel, fp.rightJust = true, true, false, false
}
func (fp *fakePointer) hold() {
	fp.pressed, fp.justPress, fp.justRel, fp.rightJust = true, false, false, false
}
func (fp *fakePointer) release() {
	fp.pressed, fp.justPress, fp.justRel, fp.rightJust = false, false, true, false
}
func (fp *fakePointer) idle() {
	fp.pressed, fp.justPress, fp.justRel, fp.rightJust = false, false, false, false
}
func (fp *fakePointer) rightPress() {
	fp.pressed, fp.justPress, fp.justRel, fp.rightJust = false, false, false, true
}

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
		"shift held":   func() { shiftModifierHeld() },
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
