package config

import (
	"math"
	"testing"
)

func TestMonsterDeathRenderSettings(t *testing.T) {
	valid := DefaultMonsterDeathRenderConfig()
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, value := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		for _, field := range []string{"fall", "fade", "hop_time", "hop_height"} {
			c := valid
			switch field {
			case "fall":
				c.FallSeconds = value
			case "fade":
				c.FadeSeconds = value
			case "hop_time":
				c.LootHopSeconds = value
			case "hop_height":
				c.LootHopHeightTiles = value
			}
			if c.Validate() == nil {
				t.Fatalf("accepted %s=%v", field, value)
			}
		}
	}
}
