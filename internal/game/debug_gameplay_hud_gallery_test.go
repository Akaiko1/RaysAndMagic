//go:build debug

package game

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/spells"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestDebugSim_GameplayHUDGallery(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("debug module; run with RAM_DEBUG_SIM=1")
	}
	g, renderer := bootFxGalleryGame(t)
	defer g.Shutdown()

	g.gameLoop.inputHandler.switchToMap("forest")
	g.camera.X, g.camera.Y = TileCenterFromTile(13, 36, float64(g.config.GetTileSize()))
	g.camera.Angle = 45 * math.Pi / 180
	g.turnBasedMode = false
	g.focusedPartyMask = 1<<0 | 1<<2

	tps := g.config.GetTPS()
	blessID := spells.SpellID("bless")
	wizardEyeID := spells.SpellID("wizard_eye")
	waterBreathingID := spells.SpellID("water_breathing")
	g.utilitySpellStatuses = map[spells.SpellID]*UtilitySpellStatus{
		blessID: {
			SpellID: blessID, Icon: "status_bless", Label: "Bless",
			Duration: 42 * tps, MaxDuration: 60 * tps,
		},
		wizardEyeID: {
			SpellID: wizardEyeID, Icon: "status_wizard_eye", Label: "Wizard Eye",
			Duration: 70 * tps, MaxDuration: 90 * tps,
		},
		waterBreathingID: {
			SpellID: waterBreathingID, Icon: "status_water_breathing", Label: "Water Breathing",
			Duration: 35 * tps, MaxDuration: 90 * tps,
		},
	}
	g.wizardEyeActive = true
	g.wizardEyeRadiusTiles = 10
	if len(g.party.Members) > 1 {
		g.party.Members[1].Conditions = append(g.party.Members[1].Conditions, character.ConditionPoisoned)
	}
	if len(g.party.Members) > 0 {
		g.party.Members[0].Equipment[items.SlotOffHand] = items.Item{Name: "Kite Shield", Type: items.ItemArmor}
		g.party.Members[0].FreeStatPoints = 3
		g.party.Members[0].RTCooldown = tps / 2
		g.gameLoop.ui.partyCooldownState = map[*character.MMCharacter]partyCooldownVisualState{
			g.party.Members[0]: {peakRemaining: tps, lastRemaining: tps / 2},
		}
	}
	if len(g.party.Members) > 3 {
		g.party.Members[3].Conditions = append(g.party.Members[3].Conditions, character.ConditionBurning)
	}
	g.levelUpChoiceQueue = []levelUpChoiceRequest{{charIndex: 0}, {charIndex: 2}}
	g.AddCombatMessage("The forest path bends toward Silverbough.")
	g.AddCombatMessage("Wizard Eye reveals movement beyond the trees.")
	// Park a banner mid-hold so the gallery shows it at rest, fully faded in -
	// the frame the player reads.
	g.screenBannerQueue = []screenBanner{{
		text:  "Legendary drop - Wyrmcleaver, the Closing Jaws",
		kind:  bannerLegendaryDrop,
		frame: g.bannerInFrames() + g.bannerHoldFrames(bannerLegendaryDrop)/2,
	}}

	out := filepath.Join(os.Getenv("HOME"), "Downloads", "RaysAndMagic_gameplay_hud")
	if err := os.RemoveAll(out); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}

	type resolution struct{ w, h int }
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

	for _, physical := range resolutions {
		logicalW, logicalH := g.gameLoop.Layout(physical.w, physical.h)
		logical := ebiten.NewImage(logicalW, logicalH)
		output := ebiten.NewImage(physical.w, physical.h)
		runOnDrawFrame(func(_ *ebiten.Image) {
			logical.Clear()
			renderer.RenderFirstPersonView(logical)
			g.gameLoop.ui.Draw(logical)
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Scale(float64(physical.w)/float64(logicalW), float64(physical.h)/float64(logicalH))
			op.Filter = ebiten.FilterNearest
			output.DrawImage(logical, op)
		})

		name := fmt.Sprintf("gameplay_hud_%dx%d.png", physical.w, physical.h)
		f, err := os.Create(filepath.Join(out, name))
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

	t.Logf("gameplay HUD gallery -> %s", out)
}
