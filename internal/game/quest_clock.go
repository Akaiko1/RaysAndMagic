package game

// currentQuestDay joins the saved calendar with time since dawn. The clock
// begins at noon; dawn is the first tick after 3/4 of its cyclic frame range.
// This preserves full elapsed days even when a reward is claimed near dusk.
func (g *MMGame) currentQuestDay() float64 {
	day := float64(g.currentCalendarDay())
	if g.config == nil {
		return day
	}
	cycle := g.dayNightCycleFrames()
	if cycle <= 0 {
		return day
	}
	dawn := 3*cycle/4 + 1
	phase := (g.dayNightFrames - dawn + cycle) % cycle
	return day + float64(phase)/float64(cycle)
}
