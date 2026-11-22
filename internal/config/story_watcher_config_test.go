package config

import "testing"

// The story screen and the captain's pacing refuse values that would hide his
// words or silently break the pacing, and the camp scene must hold long enough
// for his light and words.
func TestStoryAndWatcherConfigRejectBadValues(t *testing.T) {
	camp := DefaultCampingConfig()
	story := StoryScreenConfig{TextFadeSeconds: 0.3, LightBlinks: 3, LightBlinkSeconds: 0.4}
	watcher := WatcherConfig{CampChance: 0.2, KillChance: 0.35, MaxRandomLines: 5, MinDaysBetween: 2, CardSeconds: 6}
	if err := story.validate(camp, 120); err != nil {
		t.Fatalf("good story screen refused: %v", err)
	}
	if err := watcher.validate(); err != nil {
		t.Fatalf("good watcher refused: %v", err)
	}
	short := camp
	short.HoldSeconds = 1
	for name, err := range map[string]error{
		"no text fade":    StoryScreenConfig{LightBlinks: 3, LightBlinkSeconds: 0.4}.validate(camp, 120),
		"no blinks":       StoryScreenConfig{TextFadeSeconds: 0.3, LightBlinkSeconds: 0.4}.validate(camp, 120),
		"no blink time":   StoryScreenConfig{TextFadeSeconds: 0.3, LightBlinks: 3}.validate(camp, 120),
		"short camp hold": story.validate(short, 120),
		// 1.5 s against 3x0.41+0.27 = 1.5 s passes in seconds, yet at 60 TPS
		// the rounded frames are 90 < 3*25+16 = 91: the hold would cut his words.
		"rounded frames":  StoryScreenConfig{TextFadeSeconds: 0.27, LightBlinks: 3, LightBlinkSeconds: 0.41}.validate(CampingConfig{HoldSeconds: 1.5}, 60),
		"chance above 1":  WatcherConfig{CampChance: 2, KillChance: 0.35, MaxRandomLines: 5, CardSeconds: 6}.validate(),
		"negative chance": WatcherConfig{CampChance: 0.2, KillChance: -0.1, MaxRandomLines: 5, CardSeconds: 6}.validate(),
		"negative budget": WatcherConfig{CampChance: 0.2, KillChance: 0.35, MaxRandomLines: -1, CardSeconds: 6}.validate(),
		"no card time":    WatcherConfig{CampChance: 0.2, KillChance: 0.35, MaxRandomLines: 5}.validate(),
	} {
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
