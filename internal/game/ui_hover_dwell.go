package game

import "time"

// tooltipDwellDelay is how long the pointer rests on a party-creation entry
// before its tooltip opens: the pointer crosses those on its way elsewhere.
const tooltipDwellDelay = 450 * time.Millisecond

// dwellNow is the dwell clock; tests step it.
var dwellNow = time.Now

// hoverDwell times the pointer's rest on one named target.
type hoverDwell struct {
	key   string
	since time.Time
	seen  bool // key was hovered in the frame being built
}

// dwelled reports whether the pointer has rested on key for the dwell delay.
// Call it every frame the target is hovered; a frame drawn without the call
// ends the rest.
func (ui *UISystem) dwelled(key string) bool {
	d := &ui.hoverDwell
	now := dwellNow()
	if d.key != key {
		*d = hoverDwell{key: key, since: now}
	}
	d.seen = true
	return now.Sub(d.since) >= tooltipDwellDelay
}
