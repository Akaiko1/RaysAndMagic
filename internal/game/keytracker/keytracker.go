// Package keytracker hands out keyboard press edges with consume semantics:
// within one frame the FIRST caller asking for a key gets true, every later
// caller gets false - two handlers can never act on the same press. Edges come
// from inpututil (valid for exactly one frame), so a press that nobody consumed
// simply expires; it can never go stale and re-fire on a later frame.
package keytracker

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Consumer is the per-frame edge dispenser. Zero value is ready to use.
// BeginFrame must run once per frame before any Consume call.
type Consumer struct {
	used map[ebiten.Key]bool
	// justPressed is the edge source; nil means inpututil.IsKeyJustPressed
	// (tests inject their own).
	justPressed func(ebiten.Key) bool
}

// BeginFrame forgets the previous frame's consumptions.
func (c *Consumer) BeginFrame() { clear(c.used) }

// Consume reports whether key was just pressed this frame and its edge is
// still unclaimed, then claims it.
func (c *Consumer) Consume(key ebiten.Key) bool {
	if c.used[key] {
		return false
	}
	just := c.justPressed
	if just == nil {
		just = inpututil.IsKeyJustPressed
	}
	if !just(key) {
		return false
	}
	if c.used == nil {
		c.used = make(map[ebiten.Key]bool, 8)
	}
	c.used[key] = true
	return true
}
