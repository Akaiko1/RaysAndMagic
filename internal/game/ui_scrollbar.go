package game

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// List scrollbars are drawn as a thin track and thumb and grabbed through a
// wider strip along the track: press the thumb and drag it, or press the
// track to bring the thumb under the pointer, then drag.

// The grab strip reaches this far left and right of the drawn track; every
// list keeps its content clear of it.
const (
	scrollbarGrabLeft  = 3
	scrollbarGrabRight = 4
)

// scrollbarDrag is the list whose thumb the pointer holds. The grab lasts
// while the button is held and the list is on the presented frame.
type scrollbarDrag struct {
	key  string // empty while no thumb is held
	grab int    // pointer offset from the thumb's top at the press
}

// scrollbarThumb is the thumb's top and height for a list scrolled by offset
// out of limit.
func scrollbarThumb(track layoutRect, offset, contentH int) (y, h, limit int) {
	limit = max(0, contentH-track.h)
	if limit == 0 {
		return track.y, track.h, 0
	}
	h = max(16, track.h*track.h/contentH)
	return track.y + (track.h-h)*min(offset, limit)/limit, h, limit
}

func drawScrollbarThumb(dst *ebiten.Image, track layoutRect, offset, contentH int) {
	y, h, limit := scrollbarThumb(track, offset, contentH)
	if limit == 0 {
		return
	}
	drawFilledRect(dst, track.x, track.y, track.w, track.h, color.RGBA{43, 37, 43, 255})
	drawFilledRect(dst, track.x, y, track.w, h, profileGold)
}

// scrollbarGrabStrip is the pointer target along a track.
func scrollbarGrabStrip(track layoutRect) layoutRect {
	return layoutRect{track.x - scrollbarGrabLeft, track.y, track.w + scrollbarGrabLeft + scrollbarGrabRight, track.h}
}

// drawScrollbar draws a list's scrollbar and, when enabled, lets the pointer
// drag it. key names the list across frames; set stores a new offset.
func (ui *UISystem) drawScrollbar(dst *ebiten.Image, key string, track layoutRect, offset, contentH int, enabled bool, set func(int)) {
	drawScrollbarThumb(dst, track, offset, contentH)
	if enabled {
		ui.scrollbarGesture(key, track, scrollbarGrabStrip(track), offset, contentH, set)
	}
}

// scrollbarGesture registers the drag for a track on screen. hit is the grab
// strip, clipped by the caller to the part of the list the player sees.
func (ui *UISystem) scrollbarGesture(key string, track, hit layoutRect, offset, contentH int, set func(int)) {
	thumbY, thumbH, limit := scrollbarThumb(track, offset, contentH)
	if limit == 0 || hit.w <= 0 || hit.h <= 0 {
		return
	}
	if ui.scrollDrag.key == key {
		ui.scrollDragShown = true
	}
	ui.onDisplayedInput(uiCommandPointer, hit, func() {
		d := &ui.scrollDrag
		mx, my := pointerPosition()
		if pointerLeftJustPressed() && isMouseHoveringBox(mx, my, hit.x, hit.y, hit.right(), hit.bottom()) {
			grab := my - thumbY
			if grab < 0 || grab >= thumbH {
				grab = thumbH / 2
			}
			*d = scrollbarDrag{key: key, grab: grab}
		}
		if d.key != key || !pointerLeftPressed() {
			return
		}
		travel := max(1, track.h-thumbH)
		next := max(0, min(limit, ((my-d.grab-track.y)*limit+travel/2)/travel))
		if next != offset {
			set(next)
		}
	})
}
