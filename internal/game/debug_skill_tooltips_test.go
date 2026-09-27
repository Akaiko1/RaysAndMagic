//go:build debug

package game

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
)

func TestDebugSim_SkillTooltipAudit(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	for _, size := range [][2]int{{1024, 768}, {1920, 1080}} {
		t.Run(fmt.Sprint(size[0]), func(t *testing.T) {
			oldFrames := config.GlobalIconFrames
			config.GlobalIconFrames = nil
			t.Cleanup(func() { config.GlobalIconFrames = oldFrames })
			h := newDisplayedModalHarness(t, size[0], size[1])
			t.Chdir("../..")
			if err := config.LoadIconFrames("assets/icon_frames.yaml"); err != nil {
				t.Fatal(err)
			}
			oldTechniques := config.GlobalTechniques
			t.Cleanup(func() { config.GlobalTechniques = oldTechniques })
			if err := config.LoadTechniques("assets/techniques.yaml"); err != nil {
				t.Fatal(err)
			}
			h.g.sprites = graphics.NewSpriteManager()
			h.g.menuOpen, h.g.currentTab = true, TabCharacters
			h.g.party.Members[0] = character.CreateCharacter("Elara", character.ClassAlchemist, h.g.config)
			var cards []struct{ key, text string }
			for _, skill := range []character.SkillType{character.SkillBombThrowing, character.SkillTranslocation, character.SkillTrapper, character.SkillDualWielding, character.SkillStrongMagic, character.SkillDarkElfBinding, character.SkillSword, character.SkillPlate, character.SkillLearning, character.SkillPathfinding} {
				cards = append(cards, struct{ key, text string }{fmt.Sprint(skill), masteryTooltipTextForSkill(skill)})
			}
			cards = append(cards, struct{ key, text string }{"fire_magic", magicMasteryTooltipText(character.MagicSchoolFire)})
			for _, card := range cards {
				runOnDrawFrame(func(_ *ebiten.Image) {
					h.screen.Clear()
					h.ui.drawTabbedMenu(h.screen)
					h.ui.queueTooltip(strings.Split(card.text, "\n"), size[0]-584, 100)
					h.ui.drawQueuedTooltips(h.screen)
					filename := filepath.Join(os.TempDir(), fmt.Sprintf("skill_tooltip_%d_%s.png", size[0], card.key))
					f, err := os.Create(filename)
					if err != nil {
						t.Error(err)
						return
					}
					defer f.Close()
					if err := png.Encode(f, snapshotUIImage(h.screen)); err != nil {
						t.Error(err)
					}
				})
			}
		})
	}
}
