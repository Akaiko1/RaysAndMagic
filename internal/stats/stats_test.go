package stats

import "testing"

func TestStatBonusesSummary(t *testing.T) {
	tests := []struct {
		name string
		in   StatBonuses
		want string
	}{
		{"zero block", StatBonuses{}, ""},
		{"uniform positive (bless)", Uniform(5), "+5 to all stats"},
		{"uniform grandmaster", Uniform(10), "+10 to all stats"},
		{"uniform negative", Uniform(-3), "-3 to all stats"},
		{"single stat", FromMap(map[string]int{"might": 3}), "+3 Might"},
		{"multi stat ordered", FromMap(map[string]int{"luck": 2, "might": 3}), "+3 Might, +2 Luck"},
		{"mixed signs", FromMap(map[string]int{"might": 4, "speed": -2}), "+4 Might, -2 Speed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.Summary(); got != tt.want {
				t.Fatalf("Summary() = %q, want %q", got, tt.want)
			}
		})
	}
}
