package game

import (
	"fmt"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/quests"
)

// One arbitrary tavern's seed - rotation and retirement must hold for any.
const testTavernSeed = uint64(0x5eed7a4e12)

// The shipped rumors must load, reference real quest keys, and rotate/retire
// with quest state.
func TestRumorsYAML_LoadsAndRotates(t *testing.T) {
	prev := globalRumors
	t.Cleanup(func() { globalRumors = prev })
	questCfg, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	questManager := quests.NewQuestManager(questCfg)
	if err := LoadRumorConfig("../../assets/rumors.yaml", questManager); err != nil {
		t.Fatalf("load rumors: %v", err)
	}
	if len(globalRumors) == 0 {
		t.Fatal("no rumors shipped")
	}

	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	g.questManager = questManager

	// No prerequisites met: only after-gated rumors hide; the pool rotates by day.
	g.dayNightDay = 0
	first := g.currentRumorText(testTavernSeed)
	if first == "" {
		t.Fatal("empty rumor")
	}
	// Completing a goal quest retires its rumor from the pool.
	if err := g.questManager.ActivateQuest("culverts_valves"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	g.questManager.MarkCompleted("culverts_valves")
	for day := 0; day < len(globalRumors)*2; day++ {
		g.dayNightDay = day
		if got := g.currentRumorText(testTavernSeed); got == globalRumors[0].Text && globalRumors[0].Quest == "culverts_valves" {
			t.Fatalf("retired rumor still shown on day %d", day)
		}
	}
	// An after-gated rumor surfaces once its prerequisite completes.
	if err := g.questManager.ActivateQuest("water_purge"); err != nil {
		t.Fatalf("activate: %v", err)
	}
	g.questManager.MarkCompleted("water_purge")
	found := false
	for day := 0; day < len(globalRumors)*2 && !found; day++ {
		g.dayNightDay = day
		for _, r := range globalRumors {
			if r.After == "water_purge" && g.currentRumorText(testTavernSeed) == r.Text {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("after-gated rumor (return to the deep) never surfaced")
	}
}

// Each tavern rolls the shared pool on its own: a day's tour must not repeat
// one line everywhere, and one taproom's own deck must cover the whole pool.
// Two taverns colliding on a hint is allowed - the rolls are independent.
func TestRumors_TavernsDrawIndependently(t *testing.T) {
	prev := globalRumors
	t.Cleanup(func() { globalRumors = prev })
	globalRumors = nil
	for i := 0; i < 8; i++ {
		globalRumors = append(globalRumors, RumorDef{Text: fmt.Sprintf("rumor %d", i)})
	}
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))

	// The shipped taverns, one per map, all from the same npcs.yaml key - so the
	// seed must come from where the taproom stands, not from its key.
	taverns := []struct {
		mapKey string
		x, y   float64
	}{
		{"forest", 18, 57},
		{"desert", 29, 54},
		{"highlands", 22, 61},
		{"dragon_cliffs", 4, 82},
		{"deep_jungle", 3, 89},
	}
	seeds := make([]uint64, 0, len(taverns))
	for _, tv := range taverns {
		seed := tavernRumorSeed(&character.NPC{Key: "tavern", X: tv.x, Y: tv.y}, tv.mapKey)
		for _, prev := range seeds {
			if prev == seed {
				t.Fatalf("%s tavern shares a seed with another", tv.mapKey)
			}
		}
		seeds = append(seeds, seed)
	}

	// A day's tour: the taprooms must not all recite one line, which is the bug
	// this replaced. Collisions between two of them are fine.
	for day := 0; day < len(globalRumors)*2; day++ {
		g.dayNightDay = day
		today := map[string]bool{}
		for _, seed := range seeds {
			today[g.currentRumorText(seed)] = true
		}
		if len(today) < 2 {
			t.Fatalf("day %d: every tavern recited the same rumor", day)
		}
	}

	// Two taverns roll independently: their sequences must not run in lockstep.
	lockstep := true
	for day := 0; day < len(globalRumors) && lockstep; day++ {
		g.dayNightDay = day
		if g.currentRumorText(seeds[0]) != g.currentRumorText(seeds[1]) {
			lockstep = false
		}
	}
	if lockstep {
		t.Fatal("two taverns never diverged over a full cycle")
	}

	// One tavern's own deck is a permutation: the whole pool, once each.
	seen := map[string]int{}
	for day := 0; day < len(globalRumors); day++ {
		g.dayNightDay = day
		seen[g.currentRumorText(seeds[0])]++
	}
	if len(seen) != len(globalRumors) {
		t.Fatalf("one tavern covered %d of %d rumors in a full cycle", len(seen), len(globalRumors))
	}

	// Same tavern, same day, same line - the roll is derived, never rolled anew.
	g.dayNightDay = 5
	first := g.currentRumorText(seeds[0])
	if again := g.currentRumorText(seeds[0]); again != first {
		t.Fatalf("same tavern gave %q then %q on one day", first, again)
	}
}
