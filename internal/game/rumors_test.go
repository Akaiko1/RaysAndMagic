package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	monsterPkg "ugataima/internal/monster"
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

func TestRumors_ShippedStoryStepsHaveExactRetirementConditions(t *testing.T) {
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

	wantSpawnedBosses := map[string]string{
		"dragon_slayer#reaper_dais":             "ancient_god_of_death",
		"broodmother_nests#brood_mother_crater": "brood_mother",
		"water_purge#enforcer_depths":           "alien_enforcer",
	}
	found := make(map[string]string)
	for i, rumor := range globalRumors {
		if rumor.Quest == "" && rumor.UntilMonster == "" {
			t.Errorf("rumor %d has no retirement condition: %q", i, rumor.Text)
		}
		if rumor.Quest == "lake_spiders" || rumor.After == "lake_spiders" ||
			strings.Contains(strings.ToLower(rumor.Text), "ilsa") {
			t.Errorf("nightly bather errand leaked into story rumors: %+v", rumor)
		}
		if rumor.Quest == "endgame_triad" {
			t.Errorf("non-specific triad rumor remains: %q", rumor.Text)
		}
		if rumor.Spawn != "" {
			found[rumor.Spawn] = rumor.UntilMonster
		}
	}
	for spawn, monster := range wantSpawnedBosses {
		if found[spawn] != monster {
			t.Errorf("spawn follow-up %q retires on %q, want %q", spawn, found[spawn], monster)
		}
	}
}

func TestRumors_SpawnedBossFollowupSurvivesUntilBossDies(t *testing.T) {
	prev := globalRumors
	t.Cleanup(func() { globalRumors = prev })
	setTestWorldManager(t, nil)

	cfg := loadTestConfig(t)
	w := newTestWorld(cfg)
	g := newTestGame(cfg, w)
	questCfg, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	g.questManager = quests.NewQuestManager(questCfg)
	if err := g.questManager.ActivateQuest("water_purge"); err != nil {
		t.Fatalf("activate water purge: %v", err)
	}
	globalRumors = []RumorDef{{
		After:        "water_purge",
		UntilMonster: "alien_enforcer",
		Spawn:        "water_purge#enforcer_depths",
		Text:         "Return to the depths.",
	}}

	if got := g.eligibleRumors(); len(got) != 0 {
		t.Fatalf("follow-up before prerequisite = %v, want hidden", got)
	}
	g.questManager.MarkCompleted("water_purge")
	if got := g.eligibleRumors(); len(got) != 1 {
		t.Fatalf("follow-up before deferred spawn = %v, want visible", got)
	}

	g.questSpawnsDone = map[string]bool{"water_purge#enforcer_depths": true}
	boss := &monsterPkg.Monster3D{Name: "Alien Enforcer", HitPoints: 100}
	g.pendingQuestSpawns = []pendingQuestSpawn{{world: w, monster: boss}}
	if got := g.eligibleRumors(); len(got) != 1 {
		t.Fatalf("follow-up while boss is pending = %v, want visible", got)
	}
	g.pendingQuestSpawns = nil
	w.Monsters = []*monsterPkg.Monster3D{boss}
	if got := g.eligibleRumors(); len(got) != 1 {
		t.Fatalf("follow-up while boss lives = %v, want visible", got)
	}

	boss.HitPoints = 0
	if got := g.eligibleRumors(); len(got) != 0 {
		t.Fatalf("follow-up after boss death = %v, want retired", got)
	}
}

func TestRumors_StaticBossFollowupRetiresOnDeath(t *testing.T) {
	prev := globalRumors
	t.Cleanup(func() { globalRumors = prev })
	setTestWorldManager(t, nil)

	cfg := loadTestConfig(t)
	w := newTestWorld(cfg)
	g := newTestGame(cfg, w)
	questCfg, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	g.questManager = quests.NewQuestManager(questCfg)
	if err := g.questManager.ActivateQuest("culverts_valves"); err != nil {
		t.Fatalf("activate culvert quest: %v", err)
	}
	g.questManager.MarkCompleted("culverts_valves")
	globalRumors = []RumorDef{{
		After:        "culverts_valves",
		UntilMonster: "golden_thief_bug",
		Text:         "Finish the bug.",
	}}

	boss := &monsterPkg.Monster3D{Name: "Golden Thief Bug", HitPoints: 100}
	w.Monsters = []*monsterPkg.Monster3D{boss}
	if got := g.eligibleRumors(); len(got) != 1 {
		t.Fatalf("follow-up while static boss lives = %v, want visible", got)
	}
	boss.HitPoints = 0
	if got := g.eligibleRumors(); len(got) != 0 {
		t.Fatalf("follow-up after static boss death = %v, want retired", got)
	}
}

func TestRumors_GeneralLeadsRemainInRotationAcrossPriorities(t *testing.T) {
	prev := globalRumors
	t.Cleanup(func() { globalRumors = prev })
	globalRumors = []RumorDef{
		{Quest: "side", Priority: 10, Text: "optional lead"},
		{Quest: "story", Priority: 50, Text: "main story"},
	}
	g := &MMGame{}
	got := g.eligibleRumors()
	if len(got) != 2 || got[0] != "optional lead" || got[1] != "main story" {
		t.Fatalf("general rumor pool = %v, want both priority tiers", got)
	}
}

func TestRumors_HighestPriorityBossFollowupLeadsThePlayer(t *testing.T) {
	prev := globalRumors
	t.Cleanup(func() { globalRumors = prev })
	questCfg, err := quests.LoadQuestConfig("../../assets/quests.yaml")
	if err != nil {
		t.Fatalf("load quests: %v", err)
	}
	questManager := quests.NewQuestManager(questCfg)
	if err := questManager.ActivateQuest("water_purge"); err != nil {
		t.Fatalf("activate water purge: %v", err)
	}
	questManager.MarkCompleted("water_purge")

	globalRumors = []RumorDef{
		{Quest: "side", Priority: 10, Text: "optional lead"},
		{
			After:        "water_purge",
			UntilMonster: "alien_enforcer",
			Spawn:        "water_purge#enforcer_depths",
			Priority:     100,
			Text:         "unlocked boss",
		},
	}
	g := &MMGame{questManager: questManager}
	got := g.eligibleRumors()
	if len(got) != 1 || got[0] != "unlocked boss" {
		t.Fatalf("urgent rumor pool = %v, want only unlocked boss", got)
	}
}
