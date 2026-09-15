//go:build debug

package game

import (
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/config"
	"ugataima/internal/playerprofile"
)

// Synthetic records are confined to a temporary profile; this gallery never
// reads or changes a player's lifetime progress.
func TestDebugSim_PlayerProfileGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	h := newDisplayedModalHarness(t, 1920, 1080)
	g := h.g
	attachTestProfile(t, g)
	t.Chdir("../..")
	g.appScreen = AppScreenMainMenu
	d := &g.playerProfile.Data
	d.Since = time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	for key, n := range map[string]int64{"adventures": 12, "victories": 4, "defeats": 3, "kills": 2184, "monster_damage": 56370, "knockouts": 48, "bosses": 17, "steps": 14603, "highest_level": 32, "gold": 156980, "loot": 971, "quest_rewards": 86, "play_ns": int64(36*time.Hour + 24*time.Minute)} {
		d.Add(key, n)
	}
	for i, e := range []struct{ key, name, icon string }{
		{"knight", "Knight", "gareth"}, {"sorcerer", "Sorcerer", "lysander"}, {"cleric", "Cleric", "celestine"}, {"archer", "Archer", "silvelyn"}, {"druid", "Druid", "druid"}, {"paladin", "Paladin", "paladin"}, {"thief", "Thief", "nyra"},
	} {
		d.Rank("classes", e.key, e.name, e.icon, int64([]time.Duration{40 * time.Hour, 32 * time.Hour, 25 * time.Hour, 20 * time.Hour, 16 * time.Hour, 8 * time.Hour, 4*time.Hour + 36*time.Minute}[i]))
	}
	for i, e := range []struct{ key, name string }{{"fireball", "Fireball"}, {"heal", "Heal"}, {"bless", "Bless"}, {"ice_bolt", "Ice Bolt"}, {"inferno", "Inferno"}, {"torch_light", "Torch Light"}, {"fly", "Fly"}} {
		d.Rank("spells", e.key, e.name, "icon_spell_"+e.key, int64(630-i*73))
	}
	for i, e := range []struct{ key, name string }{{"forest", "Elvish Forest"}, {"highlands", "Misty Highlands"}, {"desert", "Scorching Desert"}, {"city", "Seabright"}, {"deep_jungle", "Deep Jungle"}} {
		d.Rank("regions", e.key, e.name, "sky:"+e.key+"_panorama", int64([]time.Duration{12 * time.Hour, 9 * time.Hour, 7 * time.Hour, 5 * time.Hour, 3*time.Hour + 24*time.Minute}[i]))
	}
	for i, e := range []struct{ key, name, icon string }{{"orc_hero_boss", "Orc Warlord", "orc_warlord"}, {"masked_huntress", "Masked Huntress", "masked_huntress"}, {"goblin", "Goblin", "goblin"}, {"lich", "Lich", "lich"}, {"old_samurai", "Samurai Warlord", "old_samurai"}} {
		for _, group := range []string{"danger", "kills", "knockouts"} {
			d.Rank(group, e.key, e.name, "monster:"+e.icon, map[string][]int64{"danger": {18760, 12210, 9520, 8710, 7170}, "kills": {17, 741, 1048, 312, 66}, "knockouts": {17, 12, 8, 7, 4}}[group][i])
		}
	}
	for i, e := range []struct{ key, name string }{{"golden_idol", "Golden Idol"}, {"health_potion", "Health Potion"}, {"mana_potion", "Mana Potion"}, {"red_dragon_statuette", "Red Dragon Statuette"}, {"gold_dragon_statuette", "Gold Dragon Statuette"}} {
		d.Rank("loot", e.key, e.name, "icon_item_"+e.key, []int64{300, 243, 186, 142, 100}[i])
	}

	for i, e := range []struct{ group, key, name, icon string }{
		{"bosses", "orc_hero_boss", "Orc Warlord", "monster:orc_warlord"},
		{"bosses", "old_samurai", "Samurai Warlord", "monster:old_samurai"},
		{"legendary_loot", "wyrmcleaver", "Wyrmcleaver", "icon_weapon_wyrmcleaver"},
		{"legendary_loot", "bow_of_hellfire", "Bow of Hellfire", "icon_weapon_bow_of_hellfire"},
		{"items_traded", "clock_hand", "Clock Hand", "icon_item_clock_hand"},
		{"items_traded", "red_dragon_scale", "Red Dragon Scale", "icon_item_red_dragon_scale"},
		{"cards_found", "goblin_card", "Goblin Card", "icon_item_goblin_card"},
	} {
		n := []int64{11, 6, 5, 2, 126, 18, 24}[i]
		d.Rank(e.group, e.key, e.name, e.icon, n)
		if e.group != "bosses" {
			d.Add(e.group, n)
		}
	}
	for i, def := range config.GetAchievements() {
		if i == 0 || i == 1 || i == 4 || i == 7 {
			d.Unlocked[def.Key] = time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
		}
	}
	out := os.Getenv("RAM_PROFILE_QA_DIR")
	if out == "" {
		out = filepath.Join(os.TempDir(), "ram-profile-qa")
	}
	os.MkdirAll(out, 0755)
	sizes := [][2]int{{800, 600}, {1024, 768}, {1280, 720}, {1366, 768}, {1440, 900}, {1920, 1080}, {2560, 1440}}
	mw, mh := MinimumWindowSize()
	sizes = append(sizes, [2]int{mw, mh})
	t.Cleanup(func() { h.ui.profileArt.close(); h.ui.profileArt = nil })
	for attempt := 0; attempt < 300; attempt++ {
		ready := false
		runOnDrawFrame(func(_ *ebiten.Image) {
			if h.ui.profileArt == nil {
				h.ui.profileArt = newProfileArt()
			}
			for _, e := range d.Top("regions") {
				h.ui.profileArt.thumbnail(strings.TrimPrefix(e.Icon, "sky:"))
			}
			ready = len(h.ui.profileArt.images) == len(d.Rankings["regions"])
		})
		if ready {
			break
		}
		if attempt == 299 {
			t.Fatal("region thumbnails did not finish")
		}
	}
	for _, size := range sizes {
		for page := 0; page <= len(profilePages); page++ {
			name := fmt.Sprintf("%dx%d-%s.png", size[0], size[1], []string{"overview", "combat", "discoveries", "trophies", "achievements"}[page])
			var drawErr error
			runOnDrawFrame(func(_ *ebiten.Image) {
				g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = size[0], size[1]
				g.entryMenuMode = EntryMenuStatistics
				g.statisticsTab = page
				if page == 2 {
					h.ui.profileExplorationReady = true
					h.ui.profileExplorationEntries = []playerprofile.Entry{
						{Name: "Seabright", Icon: "sky:city_panorama", Count: 923},
						{Name: "Elvish Forest", Icon: "sky:forest_panorama", Count: 682},
						{Name: "Misty Highlands", Icon: "sky:highlands_panorama", Count: 427},
						{Name: "Scorching Desert", Icon: "sky:desert_panorama", Count: 314},
						{Name: "Deep Jungle", Icon: "sky:deep_jungle_panorama", Count: 185},
					}
				}
				g.statisticsScroll = 0
				if page == len(profilePages) {
					g.entryMenuMode = EntryMenuAchievements
				}
				dst := ebiten.NewImage(size[0], size[1])
				defer dst.Deallocate()
				h.loop.Draw(dst)
				f, err := os.Create(filepath.Join(out, name))
				if err != nil {
					drawErr = err
					return
				}
				defer f.Close()
				drawErr = png.Encode(f, dst)
			})
			if (page == 0 || page == 3) && size[0] <= 1024 {
				runOnDrawFrame(func(_ *ebiten.Image) {
					l := makeProfileStatsLayout(size[0], size[1], profilePages[g.statisticsTab])
					g.statisticsScroll = max(0, l.contentH-l.body.h)
					dst := ebiten.NewImage(size[0], size[1])
					defer dst.Deallocate()
					h.loop.Draw(dst)
					f, err := os.Create(filepath.Join(out, fmt.Sprintf("%dx%d-%s-scrolled.png", size[0], size[1], strings.ToLower(profilePages[page].title))))
					if err != nil {
						drawErr = err
						return
					}
					defer f.Close()
					drawErr = png.Encode(f, dst)
				})
			}

			if drawErr != nil {
				t.Fatal(drawErr)
			}
		}
	}
	t.Logf("gallery: %s; minimum %dx%d", out, mw, mh)
}

func TestDebugSim_ProfileFramesStayInsideViewport(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	h := newDisplayedModalHarness(t, 1024, 768)
	g := h.g
	attachTestProfile(t, g)
	t.Chdir("../..")
	g.appScreen = AppScreenMainMenu
	for _, size := range [][2]int{{640, 480}, {800, 600}, {1024, 768}, {1280, 720}, {1366, 768}, {1440, 900}, {1920, 1080}, {2560, 1440}} {
		for tab := range profilePages {
			l := makeProfileStatsLayout(size[0], size[1], profilePages[tab])
			for _, scroll := range []int{0, l.rankY, max(0, l.contentH-l.body.h)} {
				t.Run(fmt.Sprintf("%dx%d/tab%d/scroll%d", size[0], size[1], tab, scroll), func(t *testing.T) {
					var problems []string
					runOnDrawFrame(func(_ *ebiten.Image) {
						g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = size[0], size[1]
						g.entryMenuMode = EntryMenuStatistics
						g.statisticsTab = tab
						g.statisticsScroll = scroll
						dst := ebiten.NewImage(size[0], size[1])
						defer dst.Deallocate()
						h.loop.Draw(dst)
						check := func(kind string, r layoutRect, want color.RGBA) {
							points := [][2]int{{r.x + r.w/2, r.y}, {r.x + r.w/2, r.y + r.h - 1}, {r.x, r.y + r.h/2}, {r.x + r.w - 1, r.y + r.h/2}}
							for edge, p := range points {
								if p[1] < l.body.y || p[1] >= l.body.y+l.body.h {
									continue
								}
								got := color.RGBAModel.Convert(dst.At(p[0], p[1])).(color.RGBA)
								if got != want {
									problems = append(problems, fmt.Sprintf("%s edge %d at %v clipped: got %v want %v", kind, edge, p, got, want))
								}
							}
						}
						for i := range profilePages[tab].counters {
							cx := l.body.x + (i%l.columns)*(l.columnW+14)
							check("counter", layoutRect{cx, l.body.y - g.statisticsScroll + (i/l.columns)*100, l.columnW, 86}, profileGold)
							check("ranking", layoutRect{cx, l.body.y - g.statisticsScroll + l.rankY + (i/l.columns)*(l.rankH+14), l.columnW, l.rankH}, color.RGBA{109, 91, 63, 255})
						}
					})
					for _, problem := range problems {
						t.Error(problem)
					}
				})
			}
		}
		t.Run(fmt.Sprintf("%dx%d/achievements", size[0], size[1]), func(t *testing.T) {
			var got color.RGBA
			runOnDrawFrame(func(_ *ebiten.Image) {
				g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = size[0], size[1]
				g.entryMenuMode = EntryMenuAchievements
				dst := ebiten.NewImage(size[0], size[1])
				defer dst.Deallocate()
				h.loop.Draw(dst)
				r := profilePanelRect(size[0], size[1])
				got = color.RGBAModel.Convert(dst.At(r.x+menuFrameInset+100, r.y+menuFrameInset+60)).(color.RGBA)
			})
			if got != (color.RGBA{93, 82, 66, 255}) {
				t.Fatalf("achievement top edge clipped: %v", got)
			}
		})
	}
}
