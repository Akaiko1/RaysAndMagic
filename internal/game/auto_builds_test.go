package game

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/stats"
)

// Every shipped build's gear passes the boot check, and the check rejects a
// piece that is no weapon or wear, that the class cannot use, or that serves
// none of the build's stats.
func TestAutoBuildGearValidation(t *testing.T) {
	cfg := loadTestConfig(t)
	if _, err := config.LoadWeaponConfig("../../assets/weapons.yaml"); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadItemConfig("../../assets/items.yaml"); err != nil {
		t.Fatal(err)
	}
	check := func(c *config.Config) (msg string) {
		defer func() {
			if r := recover(); r != nil {
				msg, _ = r.(string)
			}
		}()
		validateAutoBuildGear(c)
		return ""
	}
	if msg := check(cfg); msg != "" {
		t.Fatalf("shipped builds rejected: %s", msg)
	}
	for _, tc := range []struct {
		name    string
		class   character.CharacterClass
		gear    string
		want    string
		noBonus bool // the weapon scales with no stat (the knight build has no secondary)
	}{
		{"unknown key", character.ClassKnight, "no_such_thing", "is not a weapon", false},
		{"consumable", character.ClassKnight, "health_potion", "is not a weapon", false},
		{"weapon the class cannot wield", character.ClassKnight, "oak_staff", "cannot be wielded", false},
		{"weapon of another stat", character.ClassKnight, "vibro_blade", "scales with Speed", false},
		{"weapon scaling with no stat", character.ClassKnight, "tarn_trident", "scales with no stat", true},
		{"armor the class cannot wear", character.ClassSorcerer, "onryo_lamellar", "cannot be worn", false},
		{"wear raising none of the stats", character.ClassKnight, "foxfire_ofuda", "raises none", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.noBonus {
				def := config.GlobalWeapons.Weapons[tc.gear]
				saved := def.BonusStat
				def.BonusStat = ""
				t.Cleanup(func() { def.BonusStat = saved })
			}
			local := *cfg
			local.Characters.Classes = map[string]config.ClassStats{}
			for k, v := range cfg.Characters.Classes {
				local.Characters.Classes[k] = v
			}
			class := local.Characters.Classes[tc.class.Key()]
			class.AutoStats.Gear = []string{tc.gear}
			class.AutoBuilds = nil
			local.Characters.Classes[tc.class.Key()] = class
			if msg := check(&local); !strings.Contains(msg, tc.want) {
				t.Fatalf("got %q, want a rejection containing %q", msg, tc.want)
			}
		})
	}
}

// The Builds page through real gestures, in game (the party's classes) and on
// the title screen (every class): the page switch opens it, a class row shows
// that class, a build row chooses and saves its build, and on the class's row
// Left and Right step through its builds, the default clearing the choice.
func TestGameplayBuildsPageChoosesAndSaves(t *testing.T) {
	for _, entry := range []bool{false, true} {
		t.Run(fmt.Sprintf("entry=%v", entry), func(t *testing.T) {
			h, _ := audioGestureHarness(t, entry, 0)
			g, fp := h.g, installFakePointer(t)
			layout := makeAudioSettingsPanelLayout(1024, 768, entry)
			px, py := layout.px, layout.py
			click := func(r layoutRect) {
				presentInputScreen(h)
				fp.moveTo(r.x+r.w/2, r.y+r.h/2)
				fp.press()
				updateInputScreen(h)
				fp.release()
				updateInputScreen(h)
			}
			key := func(k ebiten.Key) {
				presentInputScreen(h)
				h.loop.inputHandler.keys = keytracker.NewWithSource(func(p ebiten.Key) bool { return p == k })
				updateInputScreen(h)
				h.loop.inputHandler.keys = keytracker.NewWithSource(func(ebiten.Key) bool { return false })
			}
			saved := func(class character.CharacterClass) string {
				restored := &MMGame{}
				restored.loadCombatPreferences()
				return restored.combatPreferences.AutoBuilds[class.Key()]
			}
			click(settingsTabRect(px, py, layout.panelW, settingsTabCombat))
			click(gameplayPageRect(px, py, gameplayPageBuilds))
			if g.gameplaySettingsPage != gameplayPageBuilds {
				t.Fatal("the page switch did not open Builds")
			}
			classes := g.autoBuildClasses()
			if len(classes) < 2 || entry && len(classes) != len(character.PlayableClasses) {
				t.Fatalf("classes offered: %v", classes)
			}

			// Two presses queued before one redraw act on the frame they were
			// aimed at: a second build click for the same class lands, but after
			// a class or page change the build row was drawn for another frame.
			queued := func(first, second layoutRect) {
				presentInputScreen(h)
				for _, r := range []layoutRect{first, second} {
					h.clickTime += 10
					g.mouseLeftClicks = append(g.mouseLeftClicks, queuedClick{x: r.x + r.w/2, y: r.y + r.h/2, at: h.clickTime})
				}
				updateInputScreen(h)
			}
			first := classAutoBuilds(classes[0], g.config)
			queued(buildsListRow(px, py, 1), buildsListRow(px, py, 2))
			if saved(classes[0]) != first[2].Key {
				t.Fatalf("two build clicks on one frame saved %q, want %q (positive control)", saved(classes[0]), first[2].Key)
			}
			g.setAutoBuild(classes[0], config.DefaultAutoBuildKey)
			queued(buildsRosterRow(px, py, 1, g.buildsRosterScroll), buildsListRow(px, py, 2))
			if g.selectedBuildsClass(classes) != 1 || saved(classes[0]) != "" || saved(classes[1]) != "" {
				t.Fatalf("a build click aimed at %s saved %q / %q after the class changed", classes[0].Key(), saved(classes[0]), saved(classes[1]))
			}
			queued(gameplayPageRect(px, py, gameplayPageView), buildsListRow(px, py, 2))
			if g.gameplaySettingsPage != gameplayPageView || saved(classes[1]) != "" {
				t.Fatalf("a build click survived the switch to View: saved %q", saved(classes[1]))
			}
			click(gameplayPageRect(px, py, gameplayPageBuilds))
			click(buildsRosterRow(px, py, 1, g.buildsRosterScroll))
			class := classes[1]
			builds := classAutoBuilds(class, g.config)
			if g.selectedBuildsClass(classes) != 1 || g.audioSettingsSelection != gameplayRowPage+2 || len(builds) < 3 {
				t.Fatalf("class row: shown %d, row %d, %d builds", g.selectedBuildsClass(classes), g.audioSettingsSelection, len(builds))
			}
			click(buildsListRow(px, py, 1))
			if g.autoBuildFor(class).Key != builds[1].Key || saved(class) != builds[1].Key {
				t.Fatalf("clicked build %q: runtime %q, saved %q", builds[1].Key, g.autoBuildFor(class).Key, saved(class))
			}
			key(ebiten.KeyLeft)
			if g.autoBuildFor(class).Key != config.DefaultAutoBuildKey || saved(class) != "" {
				t.Fatalf("Left kept %q (saved %q), want the default", g.autoBuildFor(class).Key, saved(class))
			}
			key(ebiten.KeyRight)
			key(ebiten.KeyRight)
			if g.autoBuildFor(class).Key != builds[2].Key || saved(class) != builds[2].Key {
				t.Fatalf("two Rights chose %q (saved %q), want %q", g.autoBuildFor(class).Key, saved(class), builds[2].Key)
			}
			key(ebiten.KeyUp)
			if g.selectedBuildsClass(classes) != 0 {
				t.Fatal("Up did not show the previous class")
			}
			key(ebiten.KeyUp)
			key(ebiten.KeyLeft)
			if g.gameplaySettingsPage != gameplayPageView || g.autoBuildFor(classes[0]).Key != config.DefaultAutoBuildKey {
				t.Fatal("Left on the page row must switch pages, not builds")
			}
		})
	}
}

// The party AUTO on the action rail and the stat popup's Auto spend points by
// the build chosen for each hero's class: the rail spends every hero's points,
// the popup only its own hero's. Under a modal, or with nothing to spend, they
// change nothing.
func TestAutoButtonsSpendByChosenBuild(t *testing.T) {
	const points = 12
	statsOf := func(m *character.MMCharacter) []int {
		var v []int
		for _, name := range stats.Names {
			v = append(v, *autoStatField(m, name))
		}
		return v
	}
	spentBy := func(m *character.MMCharacter, b config.AutoBuild, cfg *config.Config) []int {
		c := *m
		c.FreeStatPoints = points
		autoDistributeStatPoints(&c, b.AutoStatsConfig, cfg)
		return statsOf(&c)
	}
	for _, surface := range []string{"rail", "popup"} {
		for _, state := range []string{"ready", "modal", "no_points"} {
			t.Run(surface+"/"+state, func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g := h.g
				g.menuOpen, g.showPartyStats = false, true
				hero, other := g.party.Members[0], g.party.Members[1]
				builds := classAutoBuilds(hero.Class, g.config)
				chosen := builds[0]
				for _, b := range builds[1:] {
					if fmt.Sprint(spentBy(hero, b, g.config)) != fmt.Sprint(spentBy(hero, builds[0], g.config)) {
						chosen = b
						break
					}
				}
				if chosen.Key == config.DefaultAutoBuildKey {
					t.Fatalf("no %s build spends %d points unlike the default (positive control)", hero.Class.Key(), points)
				}
				g.setAutoBuild(hero.Class, chosen.Key)
				otherBuild := classAutoBuilds(other.Class, g.config)[0]
				if other.Class == hero.Class {
					otherBuild = chosen
				}
				wantHero, wantOther := spentBy(hero, chosen, g.config), spentBy(other, otherBuild, g.config)
				beforeHero, beforeOther := statsOf(hero), statsOf(other)
				if state != "no_points" {
					hero.FreeStatPoints, other.FreeStatPoints = points, points
				}
				if state == "modal" {
					g.campConfirmOpen = true // a modal above both the HUD and the stat popup
				}
				target := statPopupAutoRect(statPopupRect(1024, 768))
				if surface == "popup" {
					g.statPopupOpen, g.statPopupCharIdx = true, 0
				} else {
					rail, _ := inGameActionBarLayout(g)
					if rail.hasAuto != (state != "no_points") {
						t.Fatalf("AUTO shown=%v with %d points", rail.hasAuto, hero.FreeStatPoints)
					}
					if !rail.hasAuto {
						return
					}
					target = rail.auto
				}
				// The popup previews AUTO's split; reading it leaves the hero as is.
				preview := h.ui.autoStatPlan(hero, chosen.AutoStatsConfig)
				if fmt.Sprint(statsOf(hero)) != fmt.Sprint(beforeHero) {
					t.Fatal("previewing AUTO changed the hero")
				}
				h.clicks(false, target.x+target.w/2, target.y+target.h/2, 1)

				spent := state == "ready"
				if spent {
					for i, name := range stats.Names {
						if got := statsOf(hero)[i] - beforeHero[i]; got != preview.ValueByName(name) {
							t.Fatalf("%s gained %d, the preview promised %d", name, got, preview.ValueByName(name))
						}
					}
				}
				otherSpent := spent && surface == "rail"
				switch {
				case spent && (hero.FreeStatPoints != 0 || fmt.Sprint(statsOf(hero)) != fmt.Sprint(wantHero)):
					t.Fatalf("hero stats %v (%d left), want %v by %s", statsOf(hero), hero.FreeStatPoints, wantHero, chosen.Key)
				case !spent && fmt.Sprint(statsOf(hero)) != fmt.Sprint(beforeHero):
					t.Fatal("a blocked or empty Auto changed the hero's stats")
				case otherSpent && (other.FreeStatPoints != 0 || fmt.Sprint(statsOf(other)) != fmt.Sprint(wantOther)):
					t.Fatalf("other hero stats %v, want %v by %s", statsOf(other), wantOther, otherBuild.Key)
				case !otherSpent && fmt.Sprint(statsOf(other)) != fmt.Sprint(beforeOther):
					t.Fatal("Auto spent a hero it does not own")
				}
			})
		}
	}
}

// The Builds page keeps every block inside the panel and apart from the
// others in every shipped font: the page switch and its caption, the class
// list, the build rows (the longest class list of builds), the chosen build's
// name, order, level columns and gear, and the footer.
func TestBuildSettingsLayout(t *testing.T) {
	cfg := loadTestConfig(t)
	most := 0
	for _, class := range character.PlayableClasses {
		most = max(most, len(classAutoBuilds(class, cfg)))
	}
	forEachUIFont(t, func(t *testing.T) {
		for _, size := range [][2]int{{907, 680}, {1024, 768}, {1920, 1080}} {
			for _, entry := range []bool{false, true} {
				layout := makeAudioSettingsPanelLayout(size[0], size[1], entry)
				px, py, panelW := layout.px, layout.py, layout.panelW
				y0 := py + buildsTop
				x, w := px+buildsDetailX, buildsDetailW
				var boxes []uiBox
				for page := range gameplayPageLabels {
					boxes = append(boxes, namedLayoutBox(fmt.Sprintf("page-%d", page), gameplayPageRect(px, py, gameplayPage(page))))
				}
				boxes = append(boxes,
					textLineBox("classes heading", "Party classes", px+buildsRosterX, y0),
					namedLayoutBox("class list", buildsRosterArea(px, py)),
					textLineBox("builds heading", "Builds", px+buildsListX, y0),
					uiBox{"default name", x, y0, uiTextWidth("Auto") + 8 + uiTextWidth("(class default)"), uiTextCharHeight},
					namedLayoutBox("order", layoutRect{x, y0 + 18, w, 32}),
					textLineBox("default marker", "Auto at L45", x+buildsLabelW+6, y0+56),
					namedLayoutBox("ladder", layoutRect{x, y0 + 76, w, len(stats.Names)*buildsLadderRowH + 2*buildsLadderRowH}),
				)
				cols := [3]int{x + w - 64, x + w - 32, x + w}
				for k, level := range autoBuildPreviewLevels {
					label := fmt.Sprintf("L%d", level)
					boxes = append(boxes, uiBox{label, cols[k] - uiTextWidth(label), y0 + 56, uiTextWidth(label), uiTextCharHeight})
				}
				for j := 0; j < most; j++ {
					boxes = append(boxes, namedLayoutBox(fmt.Sprintf("build-%d", j), buildsListRow(px, py, j)))
				}
				gy := y0 + 56 + 24 + len(stats.Names)*buildsLadderRowH + 2*buildsLadderRowH + 8
				for k := 0; k < config.MaxAutoBuildGear; k++ {
					boxes = append(boxes, uiBox{fmt.Sprintf("gear-%d", k), x + k*buildsGearPitch, gy + 18, buildsGearCell, buildsGearCell})
				}
				back := audioBackRect(px, py, layout.panelH, layout.contentInset)
				hint, hx, hy := settingsHintPosition(settingsTabCombat, gameplayPageBuilds, "", px, py, panelW, layout.panelH, layout.contentInset)
				boxes = append(boxes, uiBox{"back", back.x1, back.y1, back.x2 - back.x1, back.y2 - back.y1}, textLineBox("hint", hint, hx, hy))
				assertNoCollisions(t, fmt.Sprintf("builds/%dx%d/entry=%v", size[0], size[1], entry), uiBox{"settings", px, py, panelW, layout.panelH}, boxes)
			}
		}
	})
}
