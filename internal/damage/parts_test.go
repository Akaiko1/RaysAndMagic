package damage

import "testing"

func TestPartsApplyResistance(t *testing.T) {
	tests := []struct {
		name       string
		parts      Parts
		resistance int
		pierce     int
		want       Parts
	}{
		{"positive resistance", Parts{Normal: 100, True: 50}, 40, 0, Parts{Normal: 60, True: 30}},
		{"vulnerability", Parts{Normal: 100, True: 50}, -50, 0, Parts{Normal: 150, True: 75}},
		{"immunity", Parts{Normal: 100, True: 50}, 100, 0, Parts{}},
		{"pierce applies to both", Parts{Normal: 100, True: 50}, 40, 50, Parts{Normal: 80, True: 40}},
		{"pierce does not cancel vulnerability", Parts{Normal: 100, True: 50}, -50, 100, Parts{Normal: 150, True: 75}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.parts.ApplyResistance(tt.resistance, tt.pierce); got != tt.want {
				t.Fatalf("ApplyResistance(%+v, %d, %d) = %+v, want %+v",
					tt.parts, tt.resistance, tt.pierce, got, tt.want)
			}
		})
	}
}
