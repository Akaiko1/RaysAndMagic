package main

import (
	"gopkg.in/yaml.v3"
	"os"
	"testing"
	"ugataima/internal/character"
)

func TestCulvertGatePaletteScope(t *testing.T) {
	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })
	data, err := os.ReadFile("../npcs.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cfg character.NPCConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	character.NPCConfigInstance = &cfg
	for _, biome := range []string{"culverts", "forest", "desert", ""} {
		for _, collapsed := range []bool{false, true} {
			t.Run(biome+map[bool]string{false: "/open", true: "/collapsed"}[collapsed], func(t *testing.T) {
				entries := buildLegendEntries(nil, nil, biome, map[string]bool{"group:npc:door": collapsed})
				found, reinforced := false, false
				for _, e := range entries {
					found = found || e.NPCKey == "culvert_gate"
					reinforced = reinforced || e.NPCKey == "reinforced_door"
				}
				if found != (biome == "culverts" && !collapsed) {
					t.Fatalf("culvert gate visible=%v", found)
				}
				if reinforced == collapsed {
					t.Fatal("universal reinforced door scope changed")
				}
			})
		}
	}
}
