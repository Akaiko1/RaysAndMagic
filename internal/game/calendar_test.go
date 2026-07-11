package game

import (
	"testing"

	"ugataima/internal/config"
)

func TestCalendarAdvancesDaysWeeksAndMonthsAtDawn(t *testing.T) {
	g := &MMGame{config: &config.Config{}}
	g.calendarDay, g.calendarWeek, g.calendarMonth = 1, 1, 1

	for range 6 { // Day 1 -> Day 7 stays inside week 1.
		weekChanged, monthChanged := g.advanceCalendarAtDawn()
		if weekChanged || monthChanged {
			t.Fatal("calendar changed week or month before its boundary")
		}
	}
	if g.calendarDay != 7 || g.calendarWeek != 1 || g.calendarMonth != 1 {
		t.Fatalf("after six dawns = day/week/month %d/%d/%d, want 7/1/1", g.calendarDay, g.calendarWeek, g.calendarMonth)
	}

	weekChanged, monthChanged := g.advanceCalendarAtDawn() // Day 8, Week 2.
	if !weekChanged || monthChanged || g.calendarDay != 8 || g.calendarWeek != 2 || g.calendarMonth != 1 {
		t.Fatalf("week boundary = day/week/month %d/%d/%d, changed week/month=%v/%v", g.calendarDay, g.calendarWeek, g.calendarMonth, weekChanged, monthChanged)
	}

	for g.calendarDay < 29 {
		weekChanged, monthChanged = g.advanceCalendarAtDawn()
	}
	if !weekChanged || !monthChanged || g.calendarDay != 29 || g.calendarWeek != 5 || g.calendarMonth != 2 {
		t.Fatalf("month boundary = day/week/month %d/%d/%d, changed week/month=%v/%v", g.calendarDay, g.calendarWeek, g.calendarMonth, weekChanged, monthChanged)
	}
}

func TestCalendarMigratesOldPhaseCounterSaves(t *testing.T) {
	day, week, month := calendarFromSave(0, 0, 0, 14, config.DayNightConfig{})
	if day != 8 || week != 2 || month != 1 {
		t.Fatalf("old save migration = day/week/month %d/%d/%d, want 8/2/1", day, week, month)
	}
}
