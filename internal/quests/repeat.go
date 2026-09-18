package quests

import (
	"fmt"
	"strconv"
	"strings"
)

// RepeatSchedule is empty (once), day (dawn), night (dusk), or Nd full days
// after claiming the reward. Boolean YAML is deliberately rejected.
type RepeatSchedule string

func (r RepeatSchedule) days() int {
	text := string(r)
	if !strings.HasSuffix(text, "d") {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSuffix(text, "d"))
	if err != nil || n <= 0 || strconv.Itoa(n)+"d" != text {
		return 0
	}
	return n
}

func (r RepeatSchedule) Validate() error {
	if r == "" || r == "day" || r == "night" || r.days() > 0 {
		return nil
	}
	return fmt.Errorf("repeatable must be day, night, or a positive whole-day interval such as 3d; got %q", r)
}

// Due keeps phase events separate from elapsed time, so loading at night does
// not reset a nightly errand and a half-day never counts as a full day.
func (r RepeatSchedule) Due(claimedDay, nowDay float64, event RepeatSchedule) bool {
	if r == "day" || r == "night" {
		return event == r
	}
	days := r.days()
	return days > 0 && claimedDay > 0 && nowDay-claimedDay >= float64(days)
}
