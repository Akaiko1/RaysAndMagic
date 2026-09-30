//go:build debug

package game

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/sound"
)

func TestDebugSim_PlayerSettings(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	for _, size := range [][2]int{{800, 680}, {1024, 768}, {1920, 1080}} {
		for _, entry := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/entry=%v", size, entry), func(t *testing.T) {
				previous := config.GlobalIconFrames
				config.GlobalIconFrames = nil
				t.Cleanup(func() { config.GlobalIconFrames = previous })
				h := newDisplayedModalHarness(t, size[0], size[1])
				g := h.g
				t.Chdir("../..")
				if err := config.LoadIconFrames("assets/icon_frames.yaml"); err != nil {
					t.Fatal(err)
				}
				g.sprites = graphics.NewSpriteManager()
				g.showPartyStats = true
				g.soundManager = &sound.Manager{}
				g.soundManager.SetVolume(sound.VolumeMaster, .75)
				g.soundManager.SetVolume(sound.VolumeSFX, .50)
				g.soundManager.SetVolume(sound.VolumeMusic, .25)
				if entry {
					g.appScreen, g.entryMenuMode = AppScreenMainMenu, EntryMenuSettings
				} else {
					g.mainMenuOpen, g.mainMenuMode = true, MenuSettings
				}
				g.beginAudioSettings()
				prefs := g.config.EnsurePotionPreferences()
				prefs.Health.ThresholdPct = 40
				prefs.Mana.ThresholdPct = 25
				prefs.Health.Allowed["sake_flask"] = false
				fp := installFakePointer(t)
				fp.moveTo(0, 0)
				for i := range settingsTabLabels {
					tab := settingsTabKind(i)
					g.settingsTab = tab
					runOnDrawFrame(func(_ *ebiten.Image) {
						presentInputScreen(h)
						path := filepath.Join(os.TempDir(), fmt.Sprintf("player_settings_%d_entry%v_tab%d.png", size[0], entry, tab))
						f, err := os.Create(path)
						if err != nil {
							t.Error(err)
							return
						}
						defer f.Close()
						if err = png.Encode(f, snapshotUIImage(h.screen)); err != nil {
							t.Error(err)
						}
						t.Log(path)
					})
				}
			})
		}
	}
}
