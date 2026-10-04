package game

import (
	"fmt"
	"math"
	"testing"

	"ugataima/internal/monster"
)

// Case table: each shipped Solstice flyer x RT/TB x takeoff/midfall/landing.
// The asset must meet the runtime ground plane, and opacity stays full until
// landing. Presentation-only save settlement is covered by TestMonsterDeathFlight.
func TestSolsticeFlyingLandings(t *testing.T) {
	t.Chdir("../..")
	g, _, _ := bootOpenWorldGame(t, false)
	for _, name := range []string{"gale_elemental", "storm_kite", "transfer_automaton", "vaelith", "dusk_moth"} {
		t.Run(name, func(t *testing.T) {
			key := "solstice_" + name
			img, err := decodePNG("assets/sprites/mobs/" + key + "_dying_r.png")
			if err != nil {
				t.Fatal(err)
			}
			landed := false
			for x := 512; x < 1024; x++ {
				_, _, _, a := img.At(x, 1023).RGBA()
				landed = landed || a > 0
			}
			if !landed {
				t.Fatal("final death frame has a transparent gap above the ground plane")
			}
			for _, tb := range []bool{false, true} {
				t.Run(fmt.Sprint(tb), func(t *testing.T) {
					g.turnBasedMode = tb
					g.frameCount = 0
					g.monsterCorpses = nil
					m := monster.NewMonster3DFromConfig(224, 224, key, g.config)
					m.HitPoints = 0
					g.combat.finishMonsterKill(m)
					if len(g.monsterCorpses) != 1 || !g.monsterCorpses[0].flying {
						t.Fatal("real kill path did not retain the flying corpse")
					}
					c := &g.monsterCorpses[0]
					settings := g.monsterDeathSettings()
					const ground, size = 600.0, 120.0
					air := monsterFlyingBottom(g.worldHeight(), ground, size)
					for _, phase := range []float64{0, 0.5, 1} {
						g.frameCount = int64(math.Ceil(phase * settings.FallSeconds * float64(g.config.GetTPS())))
						age := float64(g.frameCount) / float64(g.config.GetTPS())
						progress := math.Min(1, age/settings.FallSeconds)
						want := air + (ground-air)*progress*progress
						if got := g.corpseBottom(c, ground, size); math.Abs(got-want) > 0.01 {
							t.Fatalf("phase %v: body bottom %v, want %v", phase, got, want)
						}
						if _, opacity := g.corpseFrameAndOpacity(c); opacity < 0.98 {
							t.Fatalf("phase %v: body faded before landing", phase)
						}
					}
				})
			}
		})
	}
}
