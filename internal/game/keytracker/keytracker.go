// keytracker.go - minimal input utility for Ebiten v2.8.8
// Provides IsKeyJustPressed functionality for a single key.
package keytracker

import (
	"github.com/hajimehoshi/ebiten/v2"
)

// KeyStateTracker tracks the previous state of a key.
type KeyStateTracker struct {
	prevPressed bool
}

// IsKeyJustPressed returns true if the key was not pressed last frame but is pressed this frame.
func (k *KeyStateTracker) IsKeyJustPressed(key ebiten.Key) bool {
	pressed := ebiten.IsKeyPressed(key)
	justPressed := pressed && !k.prevPressed
	k.prevPressed = pressed
	return justPressed
}
