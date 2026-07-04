package highscore

import (
	"testing"
	"time"
)

func TestCalculate_UsesTwoTierTimeBonus(t *testing.T) {
	base := ScoreData{TotalExperience: 100, Gold: 10, AverageLevel: 2}

	tests := []struct {
		name string
		time time.Duration
		want int
	}{
		{name: "over two hours", time: 2*time.Hour + time.Second, want: 100*10 + 10*5 + 2*1000},
		{name: "under two hours", time: 90 * time.Minute, want: 100*10 + 10*5 + 2*1000 + 30*60*5},
		{name: "under one hour gets both tiers", time: 30 * time.Minute, want: 100*10 + 10*5 + 2*1000 + 90*60*5 + 30*60*10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Calculate(ScoreData{
				TotalExperience: base.TotalExperience,
				Gold:            base.Gold,
				AverageLevel:    base.AverageLevel,
				PlayTime:        tt.time,
			})
			if got != tt.want {
				t.Fatalf("Calculate() = %d, want %d", got, tt.want)
			}
		})
	}
}
