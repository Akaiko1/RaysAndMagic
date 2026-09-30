//go:build debug

package game

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/quests"
	"ugataima/internal/sound"
)

// themeGalleryGame boots the preview game the theme screens are posed in,
// with one NPC of every dialog kind.
func themeGalleryGame(t *testing.T) (*MMGame, *UISystem, map[npcDialogKind]*character.NPC) {
	t.Helper()
	g, _ := bootFxGalleryGame(t)
	t.Cleanup(g.Shutdown)
	questCfg, err := quests.LoadQuestConfig("assets/quests.yaml")
	if err != nil {
		t.Fatal(err)
	}
	g.questManager = quests.NewQuestManager(questCfg)
	g.soundManager = &sound.Manager{}
	g.soundManager.SetVolume(sound.VolumeMaster, 1)
	g.soundManager.SetVolume(sound.VolumeSFX, 0.8)
	g.soundManager.SetVolume(sound.VolumeMusic, 0.6)
	ui := g.gameLoop.ui
	var npcKeys []string
	for key := range character.NPCConfigInstance.NPCs {
		npcKeys = append(npcKeys, key)
	}
	sort.Strings(npcKeys)
	representatives := map[npcDialogKind]*character.NPC{}
	for _, key := range npcKeys {
		npc, err := character.CreateNPCFromConfig(key, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		kind := g.npcDialogKindFor(npc)
		if representatives[kind] == nil {
			representatives[kind] = npc
		}
	}
	return g, ui, representatives
}

// walkThemeScreens poses every screen the theme covers and hands each to
// render, drawn at the current interface frame.
func walkThemeScreens(t *testing.T, g *MMGame, ui *UISystem, representatives map[npcDialogKind]*character.NPC, render func(name string, draw func(*ebiten.Image))) {
	t.Helper()
	g.menuOpen, g.mainMenuOpen, g.dialogActive = false, false, false
	for _, mode := range []EntryMenuMode{EntryMenuRoot, EntryMenuLoad, EntryMenuSettings} {
		g.entryMenuMode = mode
		render(fmt.Sprintf("main_menu_%d", mode), ui.drawEntryMenuScreen)
	}
	g.enterPartyCreate()
	render("party_creation", ui.drawPartyCreateScreen)
	g.appScreen = AppScreenInGame
	g.menuOpen = true
	g.currentTab = TabSpellbook
	g.selectedChar = 0
	saved := g.party.Members[0]
	g.party.Members[0] = character.CreateCharacter("Lysander", character.ClassSorcerer, g.config)
	g.selectedSchool, g.selectedSpell = 0, 0
	render("spell_book", func(dst *ebiten.Image) { ui.drawPartyUI(dst); ui.drawTabbedMenu(dst) })
	g.party.Members[0] = character.CreateCharacter("Nyra", character.ClassThief, g.config)
	render("trap_book", func(dst *ebiten.Image) { ui.drawPartyUI(dst); ui.drawTabbedMenu(dst) })
	g.party.Members[0] = saved
	g.menuOpen = false
	g.mainMenuOpen = true
	for _, mode := range []MainMenuMode{MenuMain, MenuControlTips, MenuSettings} {
		g.mainMenuMode = mode
		render(fmt.Sprintf("pause_%d", mode), ui.drawMainMenu)
	}
	g.mainMenuOpen = false
	for kind := dialogKindGeneric; kind <= dialogKindTavern; kind++ {
		npc := representatives[kind]
		if npc == nil {
			continue
		}
		g.beginConversation(npc)
		render("dialog_"+strings.ReplaceAll(kind.String(), " ", "_"), ui.drawNPCDialog)
		if kind == dialogKindSkillTrainer {
			g.skillTrainerPopup = true
			g.selectedCharIdx = 0
			render("trainer_popup", ui.drawNPCDialog)
			g.skillTrainerPopup = false
		}
		g.closeConversation()
	}
	for _, key := range []string{"clockmaker", "elf_city_archive", "nomad_city_spells", "mtrader0"} {
		npc, err := character.CreateNPCFromConfig(key, 0, 0)
		if err != nil {
			continue
		}
		if npc.RequiresQuest != "" {
			g.questManager.RestoreQuestProgress(npc.RequiresQuest, quests.QuestStatusCompleted, 0, 0, true)
			if !g.npcServiceGateOpen(npc) {
				t.Fatal("preview service gate stayed closed")
			}
		}
		g.beginConversation(npc)
		tabCount := 2
		if g.npcDialogKindFor(npc) == dialogKindSpellTrader && !g.npcDialogHasTalkTab(npc) {
			tabCount = 1
		}
		for tab := 0; tab < tabCount; tab++ {
			g.switchDialogTab(tab)
			render(fmt.Sprintf("tabs_%s_%d", key, tab), ui.drawNPCDialog)
		}
		g.closeConversation()
	}
	g.statPopupCharIdx = 0
	render("stat_popup", ui.drawStatDistributionPopup)
	render("member_picker", func(dst *ebiten.Image) {
		ui.drawMemberPickerPopup(dst, "Choose a Hero", "Select a party member.", 360, []int{0, 1, 2, 3}, func(i int) string { return g.party.Members[i].Name }, func(int) {}, func() {}, false)
	})
	for kind := bannerQuestTaken; kind <= bannerAchievement; kind++ {
		render(fmt.Sprintf("top_banner_%d", kind), func(dst *ebiten.Image) {
			icon := ""
			if kind == bannerAchievement {
				icon = "icon_achievement_first_blood"
			}
			g.screenBannerQueue = []screenBanner{{text: "First Blood", icon: icon, kind: kind, frame: g.bannerInFrames() + 1}}
			ui.drawScreenBanner(dst)
		})
	}
	render("loading_area", func(dst *ebiten.Image) { ui.drawLoadingBanner(dst, time.Second, 1) })
	render("party_hud", ui.drawPartyUI)
	hero := g.party.Members[0]
	g.appScreen, g.menuOpen, g.mainMenuOpen = AppScreenInGame, true, false
	for _, tab := range []MenuTab{TabInventory, TabCharacters, TabQuests, TabCards} {
		g.currentTab = tab
		render(fmt.Sprintf("hub_tab_%d", tab), func(dst *ebiten.Image) { ui.drawPartyUI(dst); ui.drawTabbedMenu(dst) })
	}
	g.currentTab = TabSpellbook
	for _, class := range []character.CharacterClass{character.ClassAlchemist, character.ClassWayfarer} {
		g.party.Members[0] = character.CreateCharacter("Elara", class, g.config)
		render(fmt.Sprintf("rare_book_%d", class), func(dst *ebiten.Image) { ui.drawPartyUI(dst); ui.drawTabbedMenu(dst) })
	}
	g.party.Members[0] = hero
	g.menuOpen, g.mainMenuOpen = false, true
	for _, mode := range []MainMenuMode{MenuSaveSelect, MenuLoadSelect} {
		g.mainMenuMode = mode
		render(fmt.Sprintf("pause_%d", mode), ui.drawMainMenu)
	}
	g.mainMenuMode = MenuSettings
	for tab := range len(settingsTabLabels) {
		g.settingsTab = settingsTabKind(tab)
		render(fmt.Sprintf("pause_settings_tab_%d", tab), ui.drawMainMenu)
	}
	g.settingsTab, g.mainMenuOpen = settingsTabSound, false
	for _, mode := range []EntryMenuMode{EntryMenuScores, EntryMenuAchievements, EntryMenuStatistics} {
		g.appScreen, g.entryMenuMode = AppScreenMainMenu, mode
		render(fmt.Sprintf("main_menu_%d", mode), ui.drawEntryMenuScreen)
	}
	g.appScreen, g.entryMenuMode = AppScreenInGame, EntryMenuRoot
}

func TestDebugSim_UIThemeGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	g, ui, representatives := themeGalleryGame(t)
	out := os.Getenv("RAM_THEME_QA_DIR")
	if out == "" {
		out = filepath.Join(os.TempDir(), "ram-ui-theme")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	for _, size := range withInterfaceFrames(t, [][2]int{{800, 680}, {1280, 720}, {1920, 1080}, {3440, 1440}, {3840, 2160}}) {
		pw, ph := g.gameLoop.Layout(size[0], size[1])
		w, h := g.config.GetScreenWidth(), g.config.GetScreenHeight()
		render := func(name string, draw func(*ebiten.Image)) {
			t.Helper()
			if filter := os.Getenv("RAM_THEME_QA_FILTER"); filter != "" && !strings.Contains(name, filter) {
				return
			}
			var err error
			for pass := 0; pass < 2; pass++ {
				runOnDrawFrame(func(_ *ebiten.Image) {
					dst := ebiten.NewImage(pw, ph)
					defer dst.Deallocate()
					beginUIFrame(dst, g.uiPixelScale())
					ui.drawScreenBackdrop(dst, w, h, "screen_title_bg")
					draw(dst)
					if pass == 0 {
						return
					}
					var f *os.File
					f, err = os.Create(filepath.Join(out, fmt.Sprintf("%s_%dx%d.png", name, size[0], size[1])))
					if err != nil {
						return
					}
					physical := ebiten.NewImage(size[0], size[1])
					opts := &ebiten.DrawImageOptions{}
					opts.GeoM.Scale(float64(size[0])/float64(pw), float64(size[1])/float64(ph))
					opts.Filter = ebiten.FilterNearest
					physical.DrawImage(dst, opts)
					err = png.Encode(f, snapshotUIImage(physical))
					physical.Deallocate()
					if closeErr := f.Close(); err == nil {
						err = closeErr
					}
				})
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		walkThemeScreens(t, g, ui, representatives, render)
	}
}

// Read back once; encoding an Ebiten image directly reads every pixel through
// the GPU image API and makes large preview galleries unnecessarily slow.
func snapshotUIImage(src *ebiten.Image) *image.RGBA {
	pixels := image.NewRGBA(src.Bounds())
	src.ReadPixels(pixels.Pix)
	return pixels
}

// Every theme screen at every interface frame in every shipped font: which
// labels had to be cut to fit. The classic font's cuts are the layout's own
// budgets; a cut another font adds is listed, and the classic font adds none
// by definition, so the survey fails only on a label that still overflows.
func TestDebugSim_LabelFit(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	g, ui, representatives := themeGalleryGame(t)
	if err := loadUIFonts(g.config.Display.Fonts); err != nil {
		t.Fatal(err)
	}
	defer func(prev *uiGlyphAtlas) { setActiveUIFont(prev) }(uiActiveFont)
	type place struct {
		screen, text string
		frame        [2]int
	}
	cuts := map[string]map[place]int{} // font -> cut label -> its box width
	var font, screen string
	var frame [2]int
	uiLabelClipHook = func(text string, boxW int) {
		cuts[font][place{screen, text, frame}] = boxW
	}
	defer func() { uiLabelClipHook = nil }()
	sizes := withInterfaceFrames(t, [][2]int{{800, 680}, {1280, 720}, {1920, 1080}, {3440, 1440}, {3840, 2160}})
	for _, f := range g.config.Display.Fonts {
		font = f.Key
		cuts[font] = map[place]int{}
		g.config.PlayerFont = f.Key
		g.applyUIFont()
		for _, size := range sizes {
			pw, ph := g.gameLoop.Layout(size[0], size[1])
			frame = [2]int{g.config.GetScreenWidth(), g.config.GetScreenHeight()}
			walkThemeScreens(t, g, ui, representatives, func(name string, draw func(*ebiten.Image)) {
				screen = name
				runOnDrawFrame(func(_ *ebiten.Image) {
					dst := ebiten.NewImage(pw, ph)
					defer dst.Deallocate()
					beginUIFrame(dst, g.uiPixelScale())
					ui.drawScreenBackdrop(dst, frame[0], frame[1], "screen_title_bg")
					draw(dst)
				})
			})
		}
	}
	t.Logf("classic cuts %d labels by its own layout budgets", len(cuts["classic"]))
	for _, f := range g.config.Display.Fonts {
		var extra []place
		for p := range cuts[f.Key] {
			if _, classic := cuts["classic"][p]; !classic {
				extra = append(extra, p)
			}
		}
		sort.Slice(extra, func(i, j int) bool {
			a, b := extra[i], extra[j]
			if a.frame != b.frame {
				return a.frame[0] < b.frame[0] || a.frame[0] == b.frame[0] && a.frame[1] < b.frame[1]
			}
			return a.screen+a.text < b.screen+b.text
		})
		frames := map[[2]int]bool{}
		for _, p := range extra {
			frames[p.frame] = true
		}
		t.Logf("%-12s %3d extra cuts on %d of %d frames", f.Key, len(extra), len(frames), len(sizes))
		for _, p := range extra {
			t.Logf("    %-12s %4dx%-4d %-26s %q (box %d)", f.Key, p.frame[0], p.frame[1], p.screen, p.text, cuts[f.Key][p])
		}
	}
}
