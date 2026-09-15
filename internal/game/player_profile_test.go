package game

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/playerprofile"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func attachTestProfile(t *testing.T, g *MMGame) {
	t.Helper()
	old := config.GlobalAchievements
	t.Cleanup(func() { config.GlobalAchievements = old })
	if _, err := config.LoadAchievementConfig("../../assets/achievements.yaml"); err != nil {
		t.Fatal(err)
	}
	s, err := playerprofile.Open(filepath.Join(t.TempDir(), "profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	g.playerProfile = s
	g.playthroughID = "profile-test-run"
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
}

func TestShippedAchievementProductionEvents(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, tc := range []struct {
			name, key string
			act       func(*MMGame)
		}{
			{"depart city", "first_steps", func(g *MMGame) { g.recordProfileTravel("city", "forest") }},
			{"first kill", "first_blood", func(g *MMGame) { g.combat.finishMonsterKill(&monster.Monster3D{Key: "goblin", ID: "victim"}) }},
			{"prison", "free_the_captives", func(g *MMGame) { g.gameLoop.freeCaptivesFromRewards(&monster.EncounterRewards{FreesCaptives: true}) }},
			{"all heroes", "full_roster", func(g *MMGame) {
				active, captives, recruits := character.StartingRoster(g.config)
				for _, group := range [][]config.RosterEntry{active, captives, recruits} {
					for _, hero := range group {
						g.party.Members = []*character.MMCharacter{character.CreateRosterCharacter(hero, g.config)}
						g.updatePlayerProfile(time.Now())
					}
				}
			}},
			{"samurai", "warlord_slayer", func(g *MMGame) {
				g.combat.finishMonsterKill(&monster.Monster3D{Key: "old_samurai", ID: "boss", Boss: true})
			}},
			{"orc", "warlord_slayer", func(g *MMGame) {
				g.combat.finishMonsterKill(&monster.Monster3D{Key: "orc_hero_boss", ID: "boss", Boss: true})
			}},
			{"archmage", "archmage", func(g *MMGame) { g.applyArchmagePromotion(0) }},
			{"lich", "lichdom", func(g *MMGame) { g.applyLichPromotion(0) }},
			{"victory", "victory", func(g *MMGame) { g.gameVictory = true; g.updatePlayerProfile(time.Now()) }},
		} {
			t.Run(fmt.Sprintf("%s/TB=%v", tc.name, tb), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g := h.g
				attachTestProfile(t, g)
				g.turnBasedMode = tb
				if len(g.party.Captive) == 0 {
					g.party.Captive = []*character.MMCharacter{character.CreateCharacter("Captive", character.ClassPaladin, g.config)}
				}
				tc.act(g)
				if _, ok := g.playerProfile.Data.Unlocked[tc.key]; !ok {
					t.Fatalf("%s not earned", tc.key)
				}
				before := len(g.pendingAchievements)
				g.evaluateAchievements()
				g.evaluateAchievements()
				if len(g.pendingAchievements) != before {
					t.Fatal("duplicate popup")
				}
			})
		}
	}
}

func TestProfileDoesNotCountAlliesOrRepeatedCorpseCleanup(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	g := h.g
	attachTestProfile(t, g)
	enemy := &monster.Monster3D{ID: "enemy", Key: "goblin"}
	for _, m := range []*monster.Monster3D{enemy, enemy, {ID: "ally", Key: "goblin", Bound: true}, {ID: "charmed", Key: "goblin", CharmedByParty: true}} {
		g.combat.finishMonsterKill(m)
	}
	if got := g.playerProfile.Data.Counters["kills"]; got != 1 {
		t.Fatalf("kills=%d", got)
	}
	if _, ok := g.playerProfile.Data.Unlocked["warlord_slayer"]; ok {
		t.Fatal("ordinary kill unlocked warlord")
	}
}

func TestProfileSpellTransactionCountsOnlySuccessfulPlayerCasts(t *testing.T) {
	for _, tc := range []struct {
		name    string
		player  bool
		outcome spellCastOutcome
		mana    int
		want    int64
	}{
		{"committed", true, castCommitted, 10, 1}, {"no effect", true, castNoEffect, 10, 0}, {"rejected", true, castRejected, 10, 0}, {"echo", false, castCommitted, 10, 0}, {"no mana", true, castCommitted, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			attachTestProfile(t, g)
			caster := g.party.Members[0]
			caster.HitPoints = 100
			caster.Conditions = nil
			caster.SpellPoints = tc.mana
			caster.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{character.MagicSchoolFire: {Mastery: character.MasteryNovice, KnownSpells: []spells.SpellID{"firebolt"}}}
			req := spellCastRequest{ID: "firebolt", Definition: spells.SpellDefinition{Name: "Firebolt"}, Caster: caster, Cost: 5, PlayerInitiated: tc.player}
			g.combat.executeSpellCast(req, func() spellCastOutcome { return tc.outcome })
			if got := g.playerProfile.Data.Counters["spells"]; got != tc.want {
				t.Fatalf("casts=%d, want %d", got, tc.want)
			}
		})
	}
}

func TestProfileWalkingAndBlockedMovesInBothModes(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprint(tb), func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			attachTestProfile(t, g)
			g.turnBasedMode = tb
			g.world = newTestWorldSized(g.config, 8, 8)
			g.collisionSystem.UpdateTileChecker(g.world)
			world.GlobalWorldManager.LoadedMaps["forest"] = g.world
			ts := g.config.GetTileSize()
			g.setPartyPosition(2.5*ts, 2.5*ts)
			if tb {
				h.loop.inputHandler.moveTurnBasedInDirection(1, 0)
			} else {
				h.loop.inputHandler.movePlayer(ts, 0)
			}
			if g.playerProfile.Data.Counters["steps"] != 1 {
				t.Fatal("successful tile step not counted")
			}
			before := g.playerProfile.Data.Counters["steps"]
			if tb {
				h.loop.inputHandler.moveTurnBasedInDirection(-1000, 0)
			} else {
				h.loop.inputHandler.movePlayer(-1000*ts, 0)
			}
			if g.playerProfile.Data.Counters["steps"] != before {
				t.Fatal("blocked step counted")
			}
		})
	}
}

func TestProfileDamageUsesActualHPLossAndSurvival(t *testing.T) {
	for _, tc := range []struct {
		name   string
		damage int
	}{
		{"hit", 10}, {"overkill", 1000}, {"zero packet", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			attachTestProfile(t, g)
			c := g.party.Members[0]
			c.HitPoints = 40
			c.MaxHitPoints = 40
			c.Luck = 0
			c.Equipment = map[items.EquipSlot]items.Item{}
			m := &monster.Monster3D{Key: "goblin", Name: "Goblin"}
			// Eradication makes the overkill case deterministic.
			hit := hitFromMonster(m, tc.damage, "physical", true, 0, false, false)
			hit.IgnoresDodge = true
			if tc.damage > 40 {
				hit.DisintegrateChance = 1
			}
			before := c.HitPoints
			g.combat.monsterHitCharacter(m, c, m.Name, hit)
			want := int64(before - max(0, c.HitPoints))
			if got := g.playerProfile.Data.Counters["monster_damage"]; got != want {
				t.Fatalf("damage=%d, HP delta=%d", got, want)
			}
			if tc.damage > 40 && want != 40 {
				t.Fatal("overkill was not capped")
			}
		})
	}
}

func TestProfileLifetimeSurvivesSaveRestoration(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	g := h.g
	attachTestProfile(t, g)
	path := filepath.Join(t.TempDir(), "old.json")
	if err := g.SaveGameToFile(path); err != nil {
		t.Fatal(err)
	}
	g.profileAdd("kills", 1)
	stamp := g.playerProfile.Data.Unlocked["first_blood"]
	if err := g.LoadGameFromFile(path); err != nil {
		t.Fatal(err)
	}
	if g.playerProfile.Data.Counters["kills"] != 1 || !g.playerProfile.Data.Unlocked["first_blood"].Equal(stamp) {
		t.Fatal("save load rolled back lifetime profile")
	}
}

func TestProfileTravelDoesNotCountFailedOrSameMapTransitions(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	g := h.g
	attachTestProfile(t, g)
	g.recordProfileTravel("city", "city")
	if err := g.transitionToMap(mapTransition{mapKey: "missing"}); err == nil {
		t.Fatal("missing map unexpectedly loaded")
	}
	if _, ok := g.playerProfile.Data.Unlocked["first_steps"]; ok {
		t.Fatal("failed travel unlocked achievement")
	}
	wm := world.GlobalWorldManager
	wm.CurrentMapKey = "city"
	wm.LoadedMaps["city"] = g.world
	if err := g.transitionToMap(mapTransition{mapKey: "forest", pose: MapPose{X: g.camera.X, Y: g.camera.Y}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := g.playerProfile.Data.Unlocked["first_steps"]; !ok {
		t.Fatal("successful city departure did not unlock")
	}
}

func TestProfileAchievementQueueSurvivesOverflowAndPauses(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	g := h.g
	attachTestProfile(t, g)
	g.profileAdd("kills", 1)
	g.updatePlayerProfile(time.Now())
	for i := 0; i < 20; i++ {
		g.queueBanner(bannerQuestProgress, fmt.Sprintf("Quest %d", i))
	}
	found := false
	for _, b := range g.screenBannerQueue {
		found = found || b.kind == bannerAchievement
	}
	if !found {
		t.Fatal("achievement dropped")
	}
	g.resetScreenBanners()
	g.menuOpen = true
	g.updatePlayerProfile(time.Now())
	if b := g.visibleScreenBanner(); b == nil || b.kind != bannerAchievement {
		t.Fatal("achievement hidden by paused menu")
	}
	before := g.currentScreenBanner().frame
	g.updateInterfacePresentation()
	if g.currentScreenBanner().frame <= before {
		t.Fatal("achievement animation frozen while paused")
	}
}

func TestProfileLootCommitBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		act  func(*MMGame, []items.Item)
		want int64
	}{
		{"monster and champion drops", func(g *MMGame, loot []items.Item) {
			g.addLootBagDrop(g.camera.X, g.camera.Y, loot, 0)
			g.pickupGroundContainerAt(0)
		}, 3},
		{"sealed chest", func(g *MMGame, loot []items.Item) {
			g.addGroundContainer(GroundContainer{Kind: ContainerKindTreasureChest, Items: loot})
		}, 0},
		{"opened chest", func(g *MMGame, loot []items.Item) {
			g.addGroundContainer(GroundContainer{Kind: ContainerKindTreasureChest, Items: loot})
			g.pickupGroundContainerAt(0)
		}, 3},
		{"crate", func(g *MMGame, loot []items.Item) { g.grantCrateLoot(&character.NPC{Name: "Crate"}, loot, 0, 0) }, 3},
		{"inventory transfer", func(g *MMGame, loot []items.Item) { g.party.AddItem(loot[0]) }, 0},
		{"restored loot bag", func(g *MMGame, loot []items.Item) {
			g.addGroundContainer(GroundContainer{Kind: ContainerKindLootBag, Items: loot})
			g.pickupGroundContainerAt(0)
		}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			attachTestProfile(t, g)
			loot := []items.Item{{Name: "Health Potion", Type: items.ItemConsumable, Quantity: 3}}
			tc.act(g, loot)
			if got := g.playerProfile.Data.Counters["loot"]; got != tc.want {
				t.Fatalf("loot=%d want=%d", got, tc.want)
			}
			if tc.want > 0 {
				top := g.playerProfile.Data.Top("loot")
				if len(top) != 1 || top[0].Count != tc.want {
					t.Fatal("ranking disagrees with total")
				}
			}
		})
	}
}

func TestProfilePlaytimePauseAndClassAccounting(t *testing.T) {
	for _, tc := range []struct {
		name       string
		screen     AppScreen
		paused, tb bool
		want       int64
	}{
		{"real time", AppScreenInGame, false, false, 100}, {"turn based", AppScreenInGame, false, true, 100}, {"inventory", AppScreenInGame, true, false, 0}, {"title", AppScreenMainMenu, false, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			attachTestProfile(t, g)
			now := time.Now()
			g.profileLastTick = now.Add(-100 * time.Millisecond)
			g.appScreen = tc.screen
			g.menuOpen = tc.paused
			g.turnBasedMode = tc.tb
			g.updatePlayerProfile(now)
			want := int64(time.Duration(tc.want) * time.Millisecond)
			if got := g.playerProfile.Data.Counters["play_ns"]; got != want {
				t.Fatalf("active time=%d want=%d", got, want)
			}
			sum := int64(0)
			for _, e := range g.playerProfile.Data.Top("classes") {
				sum += e.Count
			}
			if sum != want*int64(len(g.party.Members)) {
				t.Fatal("class time must count each active member once")
			}
		})
	}
}

func TestProfileResponsiveControlsAndScroll(t *testing.T) {
	for _, size := range [][2]int{{640, 480}, {800, 600}, {1024, 768}, {1280, 720}, {1366, 768}, {1440, 900}, {1920, 1080}, {2560, 1440}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			h := newDisplayedModalHarness(t, size[0], size[1])
			g := h.g
			attachTestProfile(t, g)
			fp := installFakePointer(t)
			g.appScreen = AppScreenMainMenu
			g.entryMenuMode = EntryMenuStatistics
			l := makeProfileStatsLayout(size[0], size[1], profilePages[0])
			if l.body.w <= 0 || l.body.h <= 0 || l.body.y+l.body.h >= l.footerY {
				t.Fatal("viewport overlaps fixed navigation")
			}
			for i := 0; i < 12; i++ {
				g.playerProfile.Data.Rank("spells", fmt.Sprint(i), "Spell", "", 1)
			}
			presentInputScreen(h)
			x := l.panel.x + menuFrameInset
			iw := l.panel.w - 2*menuFrameInset
			tab := profileTabRect(x, l.panel.y+menuFrameInset, iw, 1)
			fp.moveTo(tab.x+tab.w/2, tab.y+tab.h/2)
			fp.press()
			updateInputScreen(h)
			if g.statisticsTab != 1 {
				t.Fatal("displayed Combat tab did not respond")
			}
			fp.hold()
			for range 3 {
				updateInputScreen(h)
			}
			if g.statisticsTab != 1 {
				t.Fatal("held click escaped its tab")
			}
			fp.release()
			presentInputScreen(h)
			updateInputScreen(h)
			g.updatePlayerStatisticsKeys(func(k ebiten.Key) bool { return k == ebiten.KeyEnd })
			if g.statisticsScroll != max(0, l.contentH-l.body.h) {
				t.Fatal("End did not reach bottom")
			}
			presentInputScreen(h)
			fp.moveTo(x+40, l.footerY+15)
			fp.press()
			updateInputScreen(h)
			if g.entryMenuMode != EntryMenuRoot {
				t.Fatal("Back inaccessible after scrolling")
			}
		})
	}
}

func TestShippedAchievementCatalogAndIcons(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	attachTestProfile(t, h.g)
	if got := len(config.GetAchievements()); got != 8 {
		t.Fatalf("catalog changed: %d achievements", got)
	}
	for _, def := range config.GetAchievements() {
		f, err := os.Open(filepath.Join("../../assets/sprites/interface/achievements", def.Icon+".png"))
		if err != nil {
			t.Fatal(err)
		}
		meta, _, err := image.DecodeConfig(f)
		f.Close()
		if err != nil || meta.Width != 128 || meta.Height != 128 {
			t.Fatalf("invalid icon %s: %+v %v", def.Icon, meta, err)
		}
		for _, r := range def.Name + def.Description {
			if r > 127 {
				t.Fatalf("non-ASCII text in %s", def.Key)
			}
		}
	}
}

func TestProfileResetRequiresNewEventsAfterSaveRestoration(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, event := range []string{"kill", "departure"} {
			t.Run(fmt.Sprintf("%s/TB=%v", event, tb), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g := h.g
				attachTestProfile(t, g)
				g.turnBasedMode = tb
				d := &g.playerProfile.Data
				d.Add("kills", 9)
				d.Add("departures:city", 4)
				d.Rank("kills", "goblin", "Goblin", "goblin", 9)
				g.evaluateAchievements()
				var metrics []string
				for _, def := range config.GetAchievements() {
					metrics = append(metrics, def.AnyOf...)
				}
				d.ResetAchievements(metrics)
				g.pendingAchievements = nil
				path := filepath.Join(t.TempDir(), "game.json")
				if err := g.SaveGameToFile(path); err != nil {
					t.Fatal(err)
				}
				if err := g.LoadGameFromFile(path); err != nil {
					t.Fatal(err)
				}
				for range 3 {
					g.updatePlayerProfile(time.Now())
				}
				for _, key := range []string{"first_blood", "first_steps"} {
					if _, ok := d.Unlocked[key]; ok {
						t.Fatalf("old save re-earned %s", key)
					}
				}
				if d.Counters["kills"] != 9 || d.Counters["departures:city"] != 4 || d.Top("kills")[0].Count != 9 {
					t.Fatal("lifetime statistics changed")
				}
				key, other := "first_blood", "first_steps"
				if event == "kill" {
					g.combat.finishMonsterKill(&monster.Monster3D{Key: "goblin", ID: "new-victim"})
				} else {
					g.recordProfileTravel("city", "forest")
					key, other = other, key
				}
				if _, ok := d.Unlocked[key]; !ok {
					t.Fatal("new event did not earn achievement")
				}
				if _, ok := d.Unlocked[other]; ok {
					t.Fatal("unrelated historical condition unlocked")
				}
			})
		}
	}
}

func TestBandOfHeroesRequiresEveryActiveHero(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("TB=%v", tb), func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			attachTestProfile(t, g)
			g.party = character.NewParty(g.config)
			g.turnBasedMode = tb
			d := &g.playerProfile.Data
			d.Observe("full_roster", 1) // obsolete prison-era counter is not evidence
			g.updatePlayerProfile(time.Now())
			initial := len(d.AchievementHeroes)
			oldSave := filepath.Join(t.TempDir(), "initial.json")
			if err := g.SaveGameToFile(oldSave); err != nil {
				t.Fatal(err)
			}
			g.gameLoop.freeCaptivesFromRewards(&monster.EncounterRewards{FreesCaptives: true})
			g.updatePlayerProfile(time.Now())
			if len(d.AchievementHeroes) != initial {
				t.Fatal("prison/reserve counted as active heroes")
			}
			if _, ok := d.Unlocked["full_roster"]; ok {
				t.Fatal("prison granted Band of Heroes")
			}
			n := len(g.party.Reserve)
			last := g.party.Reserve[n-1]
			for i := 0; i < n-1; i++ {
				before := len(d.AchievementHeroes)
				if !g.swapRosterMember(0, i) {
					t.Fatal("reserve swap failed")
				}
				g.updatePlayerProfile(time.Now())
				g.updatePlayerProfile(time.Now())
				if len(d.AchievementHeroes) != before+1 {
					t.Fatal("distinct hero was missed or repeat hero counted twice")
				}
			}
			if _, ok := d.Unlocked["full_roster"]; ok {
				t.Fatal("unlocked before last hero served")
			}
			seen := len(d.AchievementHeroes)
			if err := g.LoadGameFromFile(oldSave); err != nil {
				t.Fatal(err)
			}
			g.updatePlayerProfile(time.Now())
			if len(d.AchievementHeroes) != seen {
				t.Fatal("old save lost active hero history")
			}
			g.party.Reserve = append(g.party.Reserve, last)
			if !g.swapRosterMember(0, len(g.party.Reserve)-1) {
				t.Fatal("final swap failed")
			}
			g.updatePlayerProfile(time.Now())
			if _, ok := d.Unlocked["full_roster"]; !ok {
				t.Fatal("last hero did not unlock")
			}
			beforeStats := d.Counters["highest_level"]
			var metrics []string
			for _, def := range config.GetAchievements() {
				metrics = append(metrics, def.AnyOf...)
			}
			d.ResetAchievements(metrics)
			g.pendingAchievements = nil
			g.updatePlayerProfile(time.Now())
			if _, ok := d.Unlocked["full_roster"]; ok {
				t.Fatal("reset retained earlier roster progress")
			}
			if len(d.AchievementHeroes) != len(g.party.Members) || d.Counters["highest_level"] != beforeStats {
				t.Fatal("reset must start with current active heroes and preserve statistics")
			}
		})
	}
}

func TestBandOfHeroesCountsSelectedPartyOnlyAfterStarting(t *testing.T) {
	h := newDisplayedModalHarness(t, 1024, 768)
	g := h.g
	attachTestProfile(t, g)
	_, captives, recruits := character.StartingRoster(g.config)
	g.party.Members = []*character.MMCharacter{character.CreateRosterCharacter(captives[0], g.config), character.CreateRosterCharacter(recruits[0], g.config)}
	g.appScreen = AppScreenMainMenu
	g.updatePlayerProfile(time.Now())
	if len(g.playerProfile.Data.AchievementHeroes) != 0 {
		t.Fatal("menu preview counted as playing")
	}
	g.appScreen = AppScreenInGame
	g.updatePlayerProfile(time.Now())
	for _, c := range g.party.Members {
		if !g.playerProfile.Data.AchievementHeroes[c.Name] {
			t.Fatal("selected hero not counted")
		}
	}
	if len(g.playerProfile.Data.AchievementHeroes) != 2 {
		t.Fatal("unselected heroes counted")
	}
}

func TestAchievementResetDoesNotReimportHistoricalConditions(t *testing.T) {
	for _, reset := range []bool{false, true} {
		t.Run(fmt.Sprintf("reset=%v", reset), func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			attachTestProfile(t, g)
			g.party = character.NewParty(g.config)
			g.party.FreeCaptives()
			g.party.Members[0].Promotion = character.PromotionArchmage
			g.party.Members[1].Promotion = character.PromotionLich
			if reset {
				var metrics []string
				for _, def := range config.GetAchievements() {
					metrics = append(metrics, def.AnyOf...)
				}
				g.playerProfile.Data.ResetAchievements(metrics)
			}
			g.updatePlayerProfile(time.Now())
			for _, key := range []string{"free_the_captives", "archmage", "lichdom"} {
				_, earned := g.playerProfile.Data.Unlocked[key]
				if earned == reset {
					t.Fatalf("historical %s: earned=%v after reset=%v", key, earned, reset)
				}
			}
		})
	}
}
