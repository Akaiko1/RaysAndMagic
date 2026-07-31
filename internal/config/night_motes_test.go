package config

import "testing"

func TestValidateNightMoteRenderConfig(t *testing.T) {
	valid := NightMoteRenderConfig{
		EmissionRadiusTiles:     10,
		EmissionIntervalSeconds: 2,
		EmissionChance:          0.20,
		MaxPerTree:              3,
		LifetimeSeconds:         7,
		MaxActive:               96,
	}
	if err := validateNightMoteRenderConfig(valid); err != nil {
		t.Fatalf("valid night mote config: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*NightMoteRenderConfig)
	}{
		{name: "radius", mutate: func(c *NightMoteRenderConfig) { c.EmissionRadiusTiles = 0 }},
		{name: "interval", mutate: func(c *NightMoteRenderConfig) { c.EmissionIntervalSeconds = 0 }},
		{name: "chance below zero", mutate: func(c *NightMoteRenderConfig) { c.EmissionChance = -0.01 }},
		{name: "chance above one", mutate: func(c *NightMoteRenderConfig) { c.EmissionChance = 1.01 }},
		{name: "per tree cap", mutate: func(c *NightMoteRenderConfig) { c.MaxPerTree = 0 }},
		{name: "lifetime", mutate: func(c *NightMoteRenderConfig) { c.LifetimeSeconds = 0 }},
		{name: "global cap", mutate: func(c *NightMoteRenderConfig) { c.MaxActive = 2 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := valid
			test.mutate(&got)
			if err := validateNightMoteRenderConfig(got); err == nil {
				t.Fatalf("invalid night mote config passed validation: %+v", got)
			}
		})
	}
}
