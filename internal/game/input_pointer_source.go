package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// The pointer-gesture seam. Press/move/release arbitration (click vs drag) is
// real input logic, so tests must be able to drive it with a genuine gesture
// rather than by hand-setting the state it is supposed to compute. These vars
// are the ONE device boundary the gesture pipeline reads through - mirroring
// keytracker's injectable key source. Hover checks inside draw passes keep
// calling ebiten directly: they read a position, they don't resolve a gesture.
var (
	pointerCancelJustPress = func() bool { return inpututil.IsKeyJustPressed(ebiten.KeyEscape) }
	pointerPosition        = uiCursorPosition
	pointerWheel           = ebiten.Wheel
	pointerLeftPressed     = func() bool { return ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) }
	pointerLeftJustPressed = func() bool { return inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) }
	pointerLeftJustRelease = func() bool { return inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) }
	pointerRightJustPress  = func() bool { return inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) }
)

// typedInputChars is the keyboard text seam for the game's text fields.
var typedInputChars = func() []rune { return ebiten.AppendInputChars(nil) }
