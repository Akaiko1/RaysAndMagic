package game

import "ugataima/internal/config"

// calendarFromSave restores the persisted calendar. Older saves only have the
// arena phase counter; two phase changes make one calendar day, so it provides
// a stable migration starting at Day 1 / Week 1 / Month 1.
func calendarFromSave(day, week, month, phaseChanges int, cfg config.DayNightConfig) (int, int, int) {
	if day < 1 {
		day = phaseChanges/2 + 1
	}
	if week < 1 {
		week = (day-1)/cfg.DaysPerWeekOrDefault() + 1
	}
	if month < 1 {
		month = (day-1)/cfg.DaysPerMonthOrDefault() + 1
	}
	return day, week, month
}

// advanceCalendarAtDawn moves the real calendar forward one day and returns
// which larger boundaries changed. Game-time systems use these boundaries,
// while dayNightDay remains the arena's per-phase refresh counter.
func (g *MMGame) advanceCalendarAtDawn() (weekChanged, monthChanged bool) {
	if g.calendarDay < 1 {
		g.calendarDay = 1
	}
	if g.calendarWeek < 1 {
		g.calendarWeek = (g.calendarDay-1)/g.config.DayNight.DaysPerWeekOrDefault() + 1
	}
	if g.calendarMonth < 1 {
		g.calendarMonth = (g.calendarDay-1)/g.config.DayNight.DaysPerMonthOrDefault() + 1
	}
	oldWeek, oldMonth := g.calendarWeek, g.calendarMonth
	g.calendarDay++
	g.calendarWeek = (g.calendarDay-1)/g.config.DayNight.DaysPerWeekOrDefault() + 1
	g.calendarMonth = (g.calendarDay-1)/g.config.DayNight.DaysPerMonthOrDefault() + 1
	return g.calendarWeek != oldWeek, g.calendarMonth != oldMonth
}
