package game

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/spells"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

func TestChamberExperienceBudget(t *testing.T) {
	g, wm, _ := travelFixture(t)
	g.combat = NewCombatSystem(g)
	wm.MapConfigs["forest"] = &config.MapConfig{Adventure: &config.AdventureConfig{OpeningOwned: true, ExperienceCap: 7000}}
	g.adventureVisit("forest").Generation = 1
	for _, h := range g.party.Members {
		h.Level = 30
		h.Experience = 0
		h.HitPoints = 100
		delete(h.Skills, character.SkillLearning)
	}
	learner := g.party.Members[0]
	learner.Skills[character.SkillLearning] = &character.Skill{Mastery: character.MasteryGrandMaster}
	dead := g.party.Members[1]
	dead.HitPoints = 0
	mob := &monster.Monster3D{ID: "budget-mob", HomeMap: "forest", Experience: 20000}
	g.combat.awardExperienceAndGold(mob)
	if learner.Experience <= 5000 || learner.Experience >= 7000 || dead.Experience != 0 {
		t.Fatalf("first grant: learner=%d dead=%d", learner.Experience, dead.Experience)
	}
	// Saved character budgets do not alias the live timeline.
	saved := auditSaveJSON(t, g.buildSave(wm))
	g.combat.awardExperienceOnly(mob)
	if learner.Experience != 7000 {
		t.Fatalf("Learning or departure XP bypassed cap: %d", learner.Experience)
	}
	if saved.Party.Members[0].AdventureXP["forest"].Amount >= 7000 {
		t.Fatal("save budget aliased live hero")
	}
	if err := g.applySave(wm, &saved); err != nil {
		t.Fatal(err)
	}
	learner = g.party.Members[0]
	dead = g.party.Members[1]
	g.combat.awardExperienceAndGold(mob)
	if learner.Experience != 7000 || dead.Experience != 0 {
		t.Fatal("reload changed budget")
	}
	g.combat.awardExperienceOnly(mob)
	if learner.Experience != 7000 {
		t.Fatal("cap refilled after reload")
	}
	g.combat.announceKill(mob, mob.Experience)
	if strings.Contains(g.combatLogHistory[len(g.combatLogHistory)-1].Text, "XP") {
		t.Fatal("kill log advertised exhausted XP")
	}
	dead.HitPoints = 100
	g.combat.awardExperienceAndGold(mob)
	if dead.Experience == 0 || learner.Experience != 7000 {
		t.Fatal("dead hero spent another hero's budget")
	}
	// A roster move preserves ownership, while a genuinely new run replenishes it.
	g.party.Reserve = append(g.party.Reserve, learner)
	g.party.Members = g.party.Members[1:]
	g.combat.awardExperienceAndGold(mob)
	if learner.Experience != 7000 {
		t.Fatal("bench swap reset budget")
	}
	g.adventureVisit("forest").Generation++
	before := earnedExperienceForCharacter(learner.Level, learner.Experience)
	g.combat.awardExperienceAndGold(mob)
	if gain := earnedExperienceForCharacter(learner.Level, learner.Experience) - before; gain != 7000 {
		t.Fatalf("fresh run gain=%d", gain)
	}
	// An ordinary map and its quest rewards retain their original award policy.
	mob.HomeMap = "other"
	before = earnedExperienceForCharacter(learner.Level, learner.Experience)
	g.combat.awardExperienceAndGold(mob)
	if earnedExperienceForCharacter(learner.Level, learner.Experience) <= before {
		t.Fatal("chamber cap leaked to another map")
	}
	// Completion rewards inside a chamber are independent of its kill budget.
	for _, h := range g.party.Members {
		delete(h.Skills, character.SkillLearning)
		h.AdventureXP["forest"] = character.AdventureExperience{Generation: 2, Amount: 7000}
	}
	delete(learner.Skills, character.SkillLearning)
	hero := g.party.Members[0]
	for _, source := range []string{"quest", "encounter"} {
		before := earnedExperienceForCharacter(hero.Level, hero.Experience)
		if source == "quest" {
			g.questManager = quests.NewQuestManager(&quests.QuestConfig{Quests: map[string]*quests.QuestDefinition{
				"chamber_reward": {Name: "Chamber reward", Type: quests.QuestTypeKill, TargetCount: 1, Rewards: quests.QuestRewards{Experience: 101}},
			}})
			if err := g.questManager.ActivateQuest("chamber_reward"); err != nil {
				t.Fatal(err)
			}
			g.questManager.MarkCompleted("chamber_reward")
			if !g.claimQuestReward("chamber_reward") {
				t.Fatal("quest reward could not be claimed")
			}
		} else {
			(&GameLoop{game: g}).awardEncounterRewards(&monster.EncounterRewards{Experience: 101})
		}
		if got := earnedExperienceForCharacter(hero.Level, hero.Experience) - before; got != 101 || hero.AdventureXP["forest"].Amount != 7000 {
			t.Fatalf("%s reward inside capped chamber: gain=%d budget=%d", source, got, hero.AdventureXP["forest"].Amount)
		}
	}
	// Kills with missing legacy visit state still cap XP, without manufacturing
	// an opening, changing a generation or resetting resources.
	delete(g.adventure.Visits, "forest")
	mob.HomeMap = "forest"
	g.combat.awardExperienceAndGold(mob)
	if g.adventure.Visits["forest"] != nil || hero.AdventureXP["forest"].Generation != 0 {
		t.Fatal("XP created a visit or retained a stale generation")
	}

}

// Preview and award must agree on personal bonuses, teacher bonuses, rounding,
// living recipients and partial budgets. Save/load is covered by the budget test.
func TestChamberLearningPreviewMatchesAward(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		tier                  int
		teacher               string
		deadTeacher, deadHero bool
		amount, spent, want   int
	}{
		{name: "no skill", tier: -1, amount: 101, want: 101},
		{name: "novice", tier: 0, amount: 101, want: 101},
		{name: "expert", tier: 1, amount: 101, want: 111},
		{name: "master", tier: 2, amount: 101, want: 121},
		{name: "grandmaster", tier: 3, amount: 101, want: 136},
		{name: "reserve teacher", tier: -1, teacher: "reserve", amount: 101, want: 106},
		{name: "captive teacher", tier: 1, teacher: "captive", amount: 101, want: 116},
		{name: "dead teacher", tier: 1, teacher: "reserve", deadTeacher: true, amount: 101, want: 111},
		{name: "dead hero", tier: 1, teacher: "reserve", deadHero: true, amount: 101, want: 0},
		{name: "single rounding", tier: 1, teacher: "reserve", amount: 19, want: 21},
		{name: "partial budget", tier: 3, amount: 101, spent: 6990, want: 10},
		{name: "exhausted budget", tier: 3, amount: 101, spent: 7000, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, wm, _ := travelFixture(t)
			g.combat = NewCombatSystem(g)
			wm.MapConfigs["forest"] = &config.MapConfig{Adventure: &config.AdventureConfig{OpeningOwned: true, ExperienceCap: 7000}}
			g.adventureVisit("forest").Generation = 1
			hero, teacher := g.party.Members[0], g.party.Members[1]
			g.party.Members = []*character.MMCharacter{hero}
			g.party.Reserve, g.party.Captive = nil, nil
			hero.Level, hero.Experience, hero.HitPoints = 50, 0, 100
			delete(hero.Skills, character.SkillLearning)
			if tc.tier >= 0 {
				hero.Skills[character.SkillLearning] = &character.Skill{Mastery: character.SkillMastery(tc.tier)}
			}
			if tc.deadHero {
				hero.HitPoints = 0
			}
			hero.AdventureXP = map[string]character.AdventureExperience{"forest": {Generation: 1, Amount: tc.spent}}
			if tc.teacher != "" {
				teacher.Level, teacher.Experience, teacher.HitPoints = 50, 0, 100
				teacher.Skills[character.SkillLearning] = &character.Skill{Mastery: character.MasteryGrandMaster}
				if tc.deadTeacher {
					teacher.HitPoints = 0
				}
				if tc.teacher == "reserve" {
					g.party.Reserve = []*character.MMCharacter{teacher}
				} else {
					g.party.Captive = []*character.MMCharacter{teacher}
				}
			}
			wantTotal := tc.want
			if tc.teacher != "" && !tc.deadTeacher {
				wantTotal += tc.amount + tc.amount*35/100
			}
			mob := &monster.Monster3D{ID: "learning-budget", HomeMap: "forest", Experience: tc.amount}
			g.combat.announceKill(mob, mob.Experience)
			line := g.combatLogHistory[len(g.combatLogHistory)-1].Text
			if wantTotal > 0 && !strings.Contains(line, fmt.Sprintf("+%d XP total", wantTotal)) || wantTotal == 0 && strings.Contains(line, "XP") {
				t.Fatalf("preview=%q, want %d XP", line, wantTotal)
			}
			if hero.Experience != 0 || hero.AdventureXP["forest"].Amount != tc.spent {
				t.Fatal("preview consumed XP")
			}
			beforeTotal := g.totalExperienceEarned
			if got := g.combat.awardExperienceAndGold(mob); got != wantTotal {
				t.Fatalf("reported=%d, want %d", got, wantTotal)
			}
			if g.totalExperienceEarned-beforeTotal != wantTotal {
				t.Fatalf("credited XP=%d, logged=%d", g.totalExperienceEarned-beforeTotal, wantTotal)
			}
			if hero.Experience != tc.want || hero.AdventureXP["forest"].Amount != tc.spent+tc.want {
				t.Fatalf("award=%d budget=%d, want gain %d", hero.Experience, hero.AdventureXP["forest"].Amount, tc.want)
			}
		})
	}
}

func TestChamberArenaLifecycle(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, wm, _ := bootOpenWorldGame(t, false)
	for _, key := range []string{"solstice_fire", "solstice_water", "solstice_earth", "solstice_air"} {
		for _, tb := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/tb=%v", key, tb), func(t *testing.T) {
				g.turnBasedMode = tb
				fresh, err := wm.FreshAdventureMap(key)
				if err != nil {
					t.Fatal(err)
				}
				if err := g.transitionToMap(mapTransition{mapKey: key, arrival: mapArrivalEntrance, adventureWorld: fresh, adventureVisit: &AdventureVisit{Generation: 1}}); err != nil {
					t.Fatal(err)
				}
				a := g.adventureConfig(key)
				boss := g.adventureBoss(a)
				boss.HitPoints = boss.MaxHitPoints
				boss.EndPlayerEngagement()
				boss.WasAttacked = false
				boss.BossAggro = false
				g.adventureVisit(key).BossGranted = false
				g.adventureVisit(key).ArenaLocked = false
				g.syncAdventureArenaBounds()
				x, y := g.world.GetStartingPosition()
				g.setPartyPosition(x, y)
				g.refreshMonsterAIState()
				if g.adventureArenaBounds().Enabled {
					t.Fatal("distant idle boss pulled party")
				}
				r, ts := a.Boss.Arena, float64(g.config.GetTileSize())
				cx, cy := float64(r[0]+r[2]+1)*ts/2, float64(r[1]+r[3]+1)*ts/2
				// Every packet in one area action must resolve at the original
				// party position, even when the boss is the first victim.
				seen := 0
				g.combat.forEachAreaVictim(g.combat.partyNova(math.Inf(1)), func(m *monster.Monster3D) bool { return true }, func(m *monster.Monster3D) {
					g.combat.applyMonsterDamagePacket(m, singleMonsterDamagePacket(damagecalc.Parts{True: 1}, "physical", 0), monsterDamageOptions{IgnoreArmor: true})
					seen++
					if g.camera.X != x || g.camera.Y != y || g.adventureArenaBounds().Enabled {
						t.Fatal("area damage moved or sealed the party before the action completed")
					}
				})
				if seen < 2 {
					t.Fatal("area fixture needs multiple victims")
				}
				// An occupied arena cannot seal an outside party. A retry after
				// space opens must commit both placement and the lock together.
				g.collisionSystem.RegisterEntity(collision.NewEntity("arena_blocker", cx, cy, float64(r[2]-r[0]+1)*ts, float64(r[3]-r[1]+1)*ts, collision.CollisionTypeNPC, true))
				g.gameLoop.prepareMonsterFrame()
				if g.adventureVisit(key).ArenaLocked || g.adventureArenaBounds().Enabled || g.camera.X != x || g.camera.Y != y {
					t.Fatal("full arena sealed or moved an outside party")
				}
				g.collisionSystem.UnregisterEntity("arena_blocker")
				g.gameLoop.prepareMonsterFrame()
				if !g.adventureArenaBounds().Enabled || !g.adventureArenaAllows(g.camera.X, g.camera.Y) {
					t.Fatal("safe retry failed to pull and seal arena")
				}
				if math.Hypot(g.camera.X-cx, g.camera.Y-cy) > 2*ts {
					t.Fatal("party was not placed near center")
				}
				snap := auditSaveJSON(t, g.buildSave(wm))
				px, py := g.camera.X, g.camera.Y
				if err := g.applySave(wm, &snap); err != nil {
					t.Fatal(err)
				}
				boss = g.adventureBoss(a)
				if g.camera.X != px || g.camera.Y != py || !g.adventureArenaBounds().Enabled {
					t.Fatal("reload reset arena")
				}
				outX, outY := (float64(r[0])-.5)*ts, py
				messages := len(g.combatLogHistory)
				if g.collisionSystem.CanMoveTo("player", outX, outY) || g.canMovePartyTo(outX, outY) || g.safeForcedArrival(r[0]-1, TileIndex(py, ts), "") {
					t.Fatal("arena accepted an outside landing")
				}
				if len(g.combatLogHistory) != messages {
					t.Fatal("a movement probe announced the barrier")
				}
				// Authoritative placement cannot silently reject restore/script
				// coordinates; ordinary movement has already checked collision.
				g.setPartyPosition(outX, outY)
				if g.camera.X != outX || g.camera.Y != outY || g.collisionSystem.GetEntityByID("player").BoundingBox.X != outX {
					t.Fatal("authoritative placement silently failed")
				}
				g.setPartyPosition(px, py)
				ih := &InputHandler{game: g}
				ih.movePlayer(outX-px, 0)
				if g.camera.X != px {
					t.Fatal("RT movement escaped")
				}
				if ih.moveTurnBasedInDirection(r[0]-1-TileIndex(px, ts), 0) {
					t.Fatal("TB movement escaped")
				}
				def, _ := spells.GetSpellDefinitionByID("jump")
				g.snapFacing(math.Pi)
				def.JumpTiles = 100
				if g.combat.tryCastJump(def, g.party.Members[0]) == castCommitted {
					t.Fatal("Jump escaped")
				}
				hero := g.party.Members[0]
				hero.RareClass.Anchor = character.SpatialAnchor{MapKey: key, X: outX, Y: outY, Frames: 1000}
				if g.performSpatialStep(hero, "return_step", 3) {
					t.Fatal("Return Step escaped")
				}
				messages = len(g.combatLogHistory)
				g.arenaBarrierMessageAfter = 0
				if g.performSpatialStep(hero, "fold_step", 3) && !g.adventureArenaAllows(g.camera.X, g.camera.Y) {
					t.Fatal("Fold Step escaped")
				}
				if len(g.combatLogHistory) != messages {
					t.Fatal("Fold Step search announced a rejected candidate")
				}
				// A same-map teleporter must validate before moving or charging
				// its cooldown, even though the placement primitive is unconditional.
				px, py = g.camera.X, g.camera.Y
				sx, sy := TileIndex(px, ts), TileIndex(py, ts)
				tile, registry := g.world.Tiles[sy][sx], wm.GlobalTeleporterRegistry.Teleporters
				g.world.Tiles[sy][sx] = world.TileVioletTeleporter
				wm.GlobalTeleporterRegistry.Teleporters = []world.TeleporterLocation{
					{X: sx, Y: sy, MapKey: key, Group: "arena_probe", AutoActivate: true, ExcludeSelf: true, CooldownSeconds: 5},
					{X: r[0] - 1, Y: sy, MapKey: key, Group: "arena_probe"},
				}
				delete(wm.GlobalTeleporterRegistry.LastUsedByGroup, "arena_probe")
				ih.checkTeleporter()
				if g.camera.X != px || g.camera.Y != py || !wm.GlobalTeleporterRegistry.LastUsedByGroup["arena_probe"].IsZero() {
					t.Fatal("blocked teleporter moved the party or consumed its cooldown")
				}
				g.world.Tiles[sy][sx], wm.GlobalTeleporterRegistry.Teleporters = tile, registry
				if err := g.transitionToMap(mapTransition{mapKey: "solstice_vestibule", arrival: mapArrivalEntrance}); err == nil {
					t.Fatal("ordinary exit escaped")
				}
				if err := g.transitionToMap(mapTransition{mapKey: "solstice_vestibule", arrival: mapArrivalTownPortal}); err != nil {
					t.Fatal(err)
				}
				if g.adventure.Visits[key].ArenaLocked || g.adventureArenaBounds().Enabled {
					t.Fatal("Town Portal did not release party")
				}
				if err := g.transitionToMap(mapTransition{mapKey: key, arrival: mapArrivalEntrance}); err != nil {
					t.Fatal(err)
				}
				boss = g.adventureBoss(a)
				boss.BeginPlayerEngagement()
				g.refreshMonsterAIState()
				if !g.adventureArenaBounds().Enabled {
					t.Fatal("retry did not seal arena")
				}
				boss.HitPoints = 0
				g.combat.finishMonsterKill(boss)
				if g.adventureArenaBounds().Enabled || !g.adventure.Visits[key].BossGranted {
					t.Fatal("victory did not unlock arena")
				}
				if err := g.transitionToMap(mapTransition{mapKey: "solstice_vestibule", arrival: mapArrivalEntrance}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestChamberScheduleSkipsClearedWindows(t *testing.T) {
	t.Chdir("../..")
	g, wm, _ := bootOpenWorldGame(t, false)
	g.calendarDay = 1
	g.dayNightFrames = 0
	g.dayNightIsNight = false
	now := g.currentQuestDay()
	v := g.adventureVisit("solstice_fire")
	v.BossGranted = true
	v.VictoryAt = now
	text := g.adventureScheduleText("solstice_vestibule")
	if !strings.Contains(text, "Thermal Chamber (cleared)") || !strings.Contains(text, "Next available: Flow Chamber") || !strings.Contains(text, "game hours") {
		t.Fatal(text)
	}
	for _, key := range []string{"solstice_water", "solstice_earth", "solstice_air"} {
		v := g.adventureVisit(key)
		v.BossGranted = true
		v.VictoryAt = now
	}
	text = g.adventureScheduleText("solstice_vestibule")
	if !strings.Contains(text, "Each chamber resets 3 game days after victory.") || !strings.Contains(text, "Next available: Transfer Chamber") {
		t.Fatal(text)
	}
	if g.adventureAvailable("solstice_fire", now+3-0.000001) || !g.adventureAvailable("solstice_fire", now+3) {
		t.Fatal("cooldown boundary differs from timer")
	}
	for _, tc := range []struct {
		name     string
		data     string
		deadline float64
	}{
		{"current", fmt.Sprintf(`{"boss_granted":true,"victory_at":%g,"reset_at":%g}`, now, now+3), now + 3},
		{"legacy_deadline", fmt.Sprintf(`{"boss_granted":true,"reset_at":%g}`, now+14), now + 3},
		{"legacy_opening", `{"boss_granted":true,"opening":"1:day"}`, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var visit AdventureVisit
			if err := json.Unmarshal([]byte(tc.data), &visit); err != nil {
				t.Fatal(err)
			}
			g.adventure.Visits["solstice_fire"] = &visit
			g.adventure.Visits["solstice_water"] = &AdventureVisit{BossGranted: true, VictoryAt: now + 1}
			g.adventure.Visits["solstice_earth"] = &AdventureVisit{}
			for reload := 0; reload < 2; reload++ {
				g.adventure = cloneAdventureState(g.adventure)
				if got := g.adventureResetAt("solstice_fire", g.adventure.Visits["solstice_fire"]); math.Abs(got-tc.deadline) > 1e-9 {
					t.Fatalf("reload %d: deadline=%v want=%v", reload, got, tc.deadline)
				}
				if g.adventureAvailable("solstice_fire", tc.deadline-0.000001) || !g.adventureAvailable("solstice_fire", tc.deadline) {
					t.Fatal("cooldown boundary differs after reload")
				}
				if g.adventureAvailable("solstice_water", now+3) || !g.adventureAvailable("solstice_water", now+4) || !g.adventureAvailable("solstice_earth", now) {
					t.Fatal("chambers do not have independent cooldowns")
				}
			}
		})
	}
	// Exercise legacy expiry through the actual scheduled entrance. A deadline
	// within sub-tick rounding tolerance must both admit and replace the visit.
	g.questManager.ActivateQuest("solstice_connection")
	g.questManager.MarkCompleted("solstice_connection")
	g.questManager.ClaimRewards("solstice_connection")
	g.party.Members[0].LearnSpell("jump")
	g.party.Members[0].SpellPoints = 10000
	for _, legacy := range []bool{false, true} {
		if err := g.transitionToMap(mapTransition{mapKey: "solstice_vestibule", arrival: mapArrivalEntrance}); err != nil {
			t.Fatal(err)
		}
		var data string
		// Use a later same weekly window so victory dates are positive.
		g.calendarDay = 8
		at := g.currentQuestDay()
		if legacy {
			data = fmt.Sprintf(`{"opening":"1:day","generation":4,"boss_granted":true,"reset_at":%.12f}`, at+11+5e-10)
		} else {
			data = fmt.Sprintf(`{"opening":"1:day","generation":4,"boss_granted":true,"victory_at":%.12f}`, at-3+5e-10)
		}
		var visit AdventureVisit
		if err := json.Unmarshal([]byte(data), &visit); err != nil {
			t.Fatal(err)
		}
		g.adventure.Visits["solstice_fire"] = &visit
		if err := g.enterAdventureSchedule("solstice_vestibule"); err != nil {
			t.Fatal(err)
		}
		v := g.adventure.Visits["solstice_fire"]
		if v.Generation != 5 || v.BossGranted || len(g.world.Monsters) != 19 || v.Level != g.party.AverageLevel()+5 {
			t.Fatalf("legacy=%v: admitted cleared visit instead of fresh run: %+v", legacy, v)
		}
		encoded, err := json.Marshal(visit)
		if err != nil || strings.Contains(string(encoded), "reset_at") {
			t.Fatalf("obsolete deadline survived migration: %s (%v)", encoded, err)
		}
	}
	// Content validation must reject a cooldown that has no way to start.
	bad := filepath.Join(t.TempDir(), "map_configs.yaml")
	if err := os.WriteFile(bad, []byte("maps:\n  invalid_chamber:\n    adventure:\n      opening_owned: true\n      element: fire\n      reset_days: 3\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := wm.LoadMapConfigs(bad); err == nil || !strings.Contains(err.Error(), "reset_days requires a boss") {
		t.Fatalf("bossless reset accepted: %v", err)
	}
}
