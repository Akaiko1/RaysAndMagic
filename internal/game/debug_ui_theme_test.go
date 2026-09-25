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

func TestDebugSim_UIThemeGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()
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
	out := os.Getenv("RAM_THEME_QA_DIR")
	if out == "" {
		out = filepath.Join(os.TempDir(), "ram-ui-theme")
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
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
	for _, size := range [][2]int{{800, 680}, {1280, 720}, {1920, 1080}, {3440, 1440}, {3840, 2160}} {
		g.gameLoop.Layout(size[0], size[1])
		w, h := g.config.GetScreenWidth(), g.config.GetScreenHeight()
		render := func(name string, draw func(*ebiten.Image)) {
			t.Helper()
			if filter := os.Getenv("RAM_THEME_QA_FILTER"); filter != "" && !strings.Contains(name, filter) {
				return
			}
			var err error
			for pass := 0; pass < 2; pass++ {
				runOnDrawFrame(func(_ *ebiten.Image) {
					dst := ebiten.NewImage(w, h)
					defer dst.Deallocate()
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
					opts.GeoM.Scale(float64(size[0])/float64(w), float64(size[1])/float64(h))
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
	}
}

// Read back once; encoding an Ebiten image directly reads every pixel through
// the GPU image API and makes large preview galleries unnecessarily slow.
func snapshotUIImage(src *ebiten.Image) *image.RGBA {
	pixels := image.NewRGBA(src.Bounds())
	src.ReadPixels(pixels.Pix)
	return pixels
}
