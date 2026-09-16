//go:build debug

package game

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/playerprofile"
)

func TestDebugSim_ProfilePortraitSources(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires live Draw frames")
	}
	h := newDisplayedModalHarness(t, 1920, 1080)
	t.Chdir("../..")
	paths, err := filepath.Glob("assets/sprites/characters/heroes/*_full.png")
	if err != nil || len(paths) == 0 {
		t.Fatalf("portrait sources: %v", err)
	}
	type sourceCase struct{ key, detailed string }
	var sources []sourceCase
	for _, path := range paths {
		full := strings.TrimSuffix(filepath.Base(path), ".png")
		sources = append(sources, sourceCase{strings.TrimSuffix(full, "_full"), full}, sourceCase{full, full})
	}
	sources = append(sources, sourceCase{"icon_spell_heal", "icon_spell_heal"}, sourceCase{"missing_profile_portrait", "missing_profile_portrait"})
	// Exercise old HUD keys and explicit full keys through actual disk persistence.
	path := filepath.Join(t.TempDir(), "player_profile.json")
	store, err := playerprofile.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range sources {
		store.Data.Rank("classes", src.key, src.key, src.key, 1)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = playerprofile.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	specs := []profileRankingSpec{profilePages[0].rankings[0], profilePages[2].rankings[2], {title: "Other icons", group: "spells"}}
	positions := []struct {
		name              string
		start, x, y, size int
	}{{"leader", 0, 14, 53, 76}, {"small-row", 0, 12, 144, 38}, {"paged-leader", 1, 14, 53, 76}}
	for _, src := range sources {
		for _, spec := range specs {
			for _, pos := range positions {
				t.Run(fmt.Sprintf("%s/%s/%s", src.key, spec.title, pos.name), func(t *testing.T) {
					entry := store.Data.Rankings["classes"][src.key]
					expectedKey := src.key
					if spec.group == "classes" {
						expectedKey = src.detailed
					}
					var problem string
					runOnDrawFrame(func(_ *ebiten.Image) {
						got := ebiten.NewImageWithOptions(image.Rect(0, 0, 340, 400), &ebiten.NewImageOptions{Unmanaged: true})
						want := ebiten.NewImageWithOptions(image.Rect(0, 0, 340, 400), &ebiten.NewImageOptions{Unmanaged: true})
						defer got.Deallocate()
						defer want.Deallocate()
						h.ui.drawProfileRanking(got, spec, []playerprofile.Entry{entry, entry, entry}, layoutRect{0, 0, 340, 400}, pos.start, 2)
						h.ui.drawProfileCard(want, layoutRect{0, 0, 340, 400}, false)
						h.ui.profileIcon(want, expectedKey, entry.Name, pos.x, pos.y, pos.size)
						a, b := make([]byte, 340*400*4), make([]byte, 340*400*4)
						got.ReadPixels(a)
						want.ReadPixels(b)
						for y := pos.y; y < pos.y+pos.size; y++ {
							for x := pos.x; x < pos.x+pos.size; x++ {
								for channel := 0; channel < 4; channel++ {
									i := (y*340+x)*4 + channel
									delta := int(a[i]) - int(b[i])
									if delta < -2 || delta > 2 {
										problem = fmt.Sprintf("ranking did not draw %s directly at %dpx (pixel %d,%d)", expectedKey, pos.size, x, y)
										return
									}
								}
							}
						}
					})
					if problem != "" {
						t.Fatal(problem)
					}
					if store.Data.Rankings["classes"][src.key].Icon != src.key {
						t.Fatal("rendering changed persisted identity")
					}
				})
			}
		}
	}
}
