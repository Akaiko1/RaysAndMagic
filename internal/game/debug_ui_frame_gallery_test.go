//go:build debug

package game

import (
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/quests"
	"ugataima/internal/spells"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestDebugSim_UIFrameGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	g, _ := bootFxGalleryGame(t)
	defer g.Shutdown()

	// Exercise the densest shipped character sheet and representative content
	// so the gallery catches clipping that empty/default tabs would miss.
	g.party.Members[0] = character.CreateCharacter("Auberon", character.ClassPaladin, g.config)
	auberon := g.party.Members[0]
	auberon.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("bronze_labrys")
	for slot, key := range map[items.EquipSlot]string{
		items.SlotAmulet:    "scarab_amulet",
		items.SlotHelmet:    "leather_helmet",
		items.SlotArmor:     "golden_armor",
		items.SlotOffHand:   "ringmail_vambraces",
		items.SlotGauntlets: "chainwork_gauntlets",
		items.SlotBelt:      "belt_of_speed",
		items.SlotCloak:     "chrono_cape",
		items.SlotRing1:     "magic_ring",
		items.SlotRing2:     "warlords_signet",
		items.SlotBoots:     "deathgod_greaves",
	} {
		auberon.Equipment[slot] = items.CreateItemFromYAML(key)
	}
	firstAid, err := spells.CreateSpellItem("heal")
	if err != nil {
		t.Fatal(err)
	}
	auberon.Equipment[items.SlotSpell] = firstAid
	if fire := g.party.Members[1].MagicSchools[character.MagicSchoolFire]; fire != nil {
		allFire, err := character.MagicSchoolFire.AvailableSpellIDs()
		if err != nil {
			t.Fatal(err)
		}
		fire.KnownSpells = allFire
	}
	questCfg, err := quests.LoadQuestConfig("assets/quests.yaml")
	if err != nil {
		t.Fatal(err)
	}
	g.questManager = quests.NewQuestManager(questCfg)
	for _, key := range []string{"goblin_hunt", "lake_spiders", "archmage_trial", "dragon_cliffs_troll_cull"} {
		if err := g.questManager.ActivateQuest(key); err != nil {
			t.Fatal(err)
		}
	}
	for slot, key := range []string{"medusa_card", "puma_card", "archmage_card", "lich_card", "ocelot_card", "gorilla_titan_card"} {
		if !g.setCardCollectionSlot(slot, items.CreateItemFromYAML(key)) {
			t.Fatalf("seed card %q", key)
		}
	}
	g.menuOpen = true
	g.gameLoop.ui.campNotice = "The party rests. HP and spell points fully restored."
	g.gameLoop.ui.campNoticeOK = true

	out := filepath.Join(os.Getenv("HOME"), "Downloads", "RaysAndMagic_character_hub")
	if err := os.RemoveAll(out); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}

	type resolution struct {
		w, h int
	}
	resolutions := []resolution{
		{1024, 768},
		{1280, 720},
		{1280, 800},
		{1366, 768},
		{1440, 900},
		{1600, 900},
		{1680, 1050},
		{1920, 1080},
		{1920, 1200},
		{2560, 1080},
		{2560, 1440},
		{3440, 1440},
		{3840, 2160},
	}

	render := func(name string, physical resolution, draw func(*ebiten.Image)) {
		logicalW, logicalH := g.gameLoop.Layout(physical.w, physical.h)
		logical := ebiten.NewImage(logicalW, logicalH)
		output := ebiten.NewImage(physical.w, physical.h)
		// Draw twice: the first frame populates lazy UI sprite caches, while the
		// second is the same steady-state frame a player actually sees.
		for range 2 {
			runOnDrawFrame(func(_ *ebiten.Image) {
				logical.Fill(color.RGBA{18, 18, 22, 255})
				draw(logical)
				output.Clear()
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Scale(float64(physical.w)/float64(logicalW), float64(physical.h)/float64(logicalH))
				op.Filter = ebiten.FilterNearest
				output.DrawImage(logical, op)
			})
		}
		f, err := os.Create(filepath.Join(out, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, output); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}

	tabs := []struct {
		name     string
		tab      MenuTab
		selected int
	}{
		{"inventory", TabInventory, 0},
		{"characters", TabCharacters, 0},
		{"spellbook", TabSpellbook, 1},
		{"quests", TabQuests, 0},
		{"cards", TabCards, 0},
	}
	for _, resolution := range resolutions {
		suffix := fmt.Sprintf("%dx%d", resolution.w, resolution.h)
		for _, tab := range tabs {
			g.currentTab = tab.tab
			g.selectedChar = tab.selected
			g.selectedSchool = 0
			g.selectedSpell = -1
			if tab.tab == TabSpellbook {
				g.selectedSpell = 0
			}
			g.gameLoop.ui.spellPage = 0
			render(tab.name+"_"+suffix, resolution, func(screen *ebiten.Image) {
				g.gameLoop.ui.drawPartyUI(screen)
				g.gameLoop.ui.drawTabbedMenu(screen)
			})
		}
	}

	t.Logf("UI frame gallery -> %s", out)
}
