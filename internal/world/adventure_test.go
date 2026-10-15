package world

import (
	"strings"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
)

// A push shoves the party toward a lane of its boss's own chamber, so a map
// load fails for a pusher anywhere else; shipped chambers load in
// TestShippedMapsLoadWithWalkableStarts.
func TestPushNeedsItsChamberLanes(t *testing.T) {
	previous := monster.MonsterConfig
	t.Cleanup(func() { monster.MonsterConfig = previous })
	if _, err := monster.LoadMonsterConfig("../../assets/monsters.yaml"); err != nil {
		t.Fatalf("load monster config: %v", err)
	}
	chamber := func(boss string, lanes ...string) *config.AdventureConfig {
		return &config.AdventureConfig{Boss: &config.AdventureBoss{Monster: boss, Lanes: lanes, CooldownSeconds: 6}}
	}
	for _, tc := range []struct {
		name      string
		adventure *config.AdventureConfig
		ok        bool
	}{
		{"boss of a chamber with lanes", chamber("solstice_aureth", "lane"), true},
		{"chamber without lanes", chamber("solstice_aureth"), false},
		{"map without a chamber", nil, false},
		{"another monster's chamber", chamber("goblin", "lane"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := NewWorld3D(createTestWorldConfig())
			w.loadMonstersFromMapData([]MonsterSpawn{{X: 2, Y: 2, MonsterKey: "solstice_aureth"}, {X: 4, Y: 2, MonsterKey: "goblin"}}, "test_map")
			if w.Monsters[0].Abilities.Push.CooldownSeconds == 0 {
				t.Fatal("solstice_aureth no longer authors push")
			}
			err := validateAdventureWorld("test_map", w, tc.adventure)
			if tc.ok != (err == nil) || (err != nil && !strings.Contains(err.Error(), "has push")) {
				t.Fatalf("validation error %v, want ok=%v", err, tc.ok)
			}
		})
	}
}
