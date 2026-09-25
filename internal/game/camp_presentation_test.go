package game

import (
	"fmt"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/world"
)

// One shared rest rule serves the paid camp, tavern and authored campfire.
// Death/eradication are terminal; all living conditions and timers are cured.
func TestRestCuresLivingConditions(t *testing.T) {
	for _, source := range []string{"camp", "tavern", "campfire"} {
		for _, tb := range []bool{false, true} {
			for _, terminal := range []character.Condition{character.ConditionNormal, character.ConditionDead, character.ConditionEradicated} {
				t.Run(fmt.Sprintf("%s/TB=%v/%s", source, tb, terminal), func(t *testing.T) {
					h := newDisplayedModalHarness(t, 1024, 768)
					g := h.g
					g.turnBasedMode, g.party.Food, g.party.Gold = tb, 3, 100
					g.world.Monsters = nil
					m := g.party.Members[0]
					m.HitPoints, m.SpellPoints = 1, 0
					for c := character.ConditionPoisoned; c <= character.ConditionStunned; c++ {
						if c != character.ConditionDead && c != character.ConditionEradicated {
							m.AddCondition(c)
						}
					}
					m.PoisonFramesRemaining, m.BurnFramesRemaining = 900, 900
					m.RestoreDoTTickTimers(30, 40)
					m.StunFramesRemaining, m.StunTurnsRemaining, m.StunRate = 240, 2, 120
					if terminal != character.ConditionNormal {
						m.AddCondition(terminal)
						m.HitPoints = 0
					}
					switch source {
					case "camp":
						if msg, ok := g.TryCamp(); !ok {
							t.Fatal(msg)
						}
					case "tavern":
						h.loop.inputHandler.handleTavernRest(&character.NPCDialogueChoice{Cost: 10})
					case "campfire":
						g.applyCrateEffects(&character.NPC{}, &config.CrateConfig{FreeRest: true})
					}
					if terminal != character.ConditionNormal {
						if m.HitPoints != 0 || !m.HasCondition(terminal) {
							t.Fatal("rest revived a terminal condition")
						}
						return
					}
					p, b := m.DoTTickTimers()
					if len(m.Conditions) != 0 || p != 0 || b != 0 || m.PoisonFramesRemaining != 0 || m.BurnFramesRemaining != 0 || m.StunFramesRemaining != 0 || m.StunTurnsRemaining != 0 || m.StunRate != 0 {
						t.Fatal("rest retained a condition or its hidden timer")
					}
					for i := 0; i < 240; i++ {
						m.UpdateWithMode(false)
					}
					m.TickPoisonTurn(240, 120)
					m.TickBurnTurn(240, 120)
					m.TickStunTurn()
					if m.HitPoints != m.MaxHitPoints || m.SpellPoints != m.MaxSpellPoints {
						t.Fatal("a cured effect resumed after rest or a mode switch")
					}
				})
			}
		}
	}
}

func TestCampPresentationPausesWorldAndOwnsInput(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("TB=%v", tb), func(t *testing.T) {
			h := newDisplayedModalHarness(t, 800, 600)
			g := h.g
			g.turnBasedMode, g.party.Food = tb, 3
			g.world.Monsters = nil
			g.campConfirmOpen = true
			h.ui.Draw(h.screen)
			l := layoutCampConfirmation(800, 600)
			h.clicks(false, l.yes.x+10, l.yes.y+10, 2)
			if g.campRest == nil || g.party.Food != 2 {
				t.Fatal("confirmed camp did not start exactly once")
			}
			s := g.campRest
			// No artwork has been presented yet: streaming must not consume the fade.
			for i := 0; i < 120; i++ {
				g.updateInterfacePresentation()
			}
			if s.elapsed != 0 {
				t.Fatal("fade ran before its first displayed frame")
			}
			frame, day := g.frameCount, g.dayNightFrames
			x, y := g.camera.X, g.camera.Y
			h.loop.inputHandler.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == ebiten.KeyW || k == ebiten.KeyEscape || k == ebiten.KeySpace })
			// Draw/Update the actual modal. Missing art in this unit fixture uses
			// the bounded fallback; GPU tests exercise the authored images.
			for i := 0; i < s.fadeIn+s.hold+s.fadeOut+2 && g.campRest != nil; i++ {
				h.ui.Draw(h.screen)
				if !g.gameplayPausedByOverlay() || g.worldClickAllowed() {
					t.Fatal("rest lost modal ownership")
				}
				if err := h.loop.Update(); err != nil {
					t.Fatal(err)
				}
				if g.frameCount != frame || g.dayNightFrames != day || g.camera.X != x || g.camera.Y != y || g.party.Food != 2 {
					t.Fatal("world/input advanced under camping artwork")
				}
			}
			if g.campRest != nil {
				t.Fatal("camp never finished its fade")
			}
			if !h.ui.modalRedrawBarrierActive() {
				t.Fatal("fade completion leaked input into the stale frame")
			}
		})
	}
}

func TestCampFadeEnvelope(t *testing.T) {
	s := &campRestPresentation{fadeIn: 60, hold: 120, fadeOut: 60}
	for _, tc := range []struct {
		frame int
		alpha float32
	}{{0, 0}, {30, .5}, {60, 1}, {120, 1}, {180, 1}, {210, .5}, {240, 0}} {
		s.elapsed = tc.frame
		if s.alpha() != tc.alpha {
			t.Fatalf("frame %d alpha=%v want %v", tc.frame, s.alpha(), tc.alpha)
		}
	}
}

func TestCampSceneUsesPartyBiome(t *testing.T) {
	t.Chdir("../..")
	for _, merged := range []bool{false, true} {
		g, wm, _ := bootOpenWorldGame(t, merged)
		for key, mc := range wm.MapConfigs {
			t.Run(fmt.Sprintf("%s/merged=%v", key, merged), func(t *testing.T) {
				wm.CurrentMapKey = key
				g.world = wm.WorldByKey(key)
				if g.world == nil {
					t.Fatal("authored map missing")
				}
				x, y := wm.ProjectWorldPos(key, 64, 64)
				g.camera.X, g.camera.Y = x, y
				if merged && wm.IsOpenWorldRegion(key) {
					wm.CurrentMapKey = world.OpenWorldKey
					g.world = wm.OpenWorld
				}
				want := wm.Biomes[mc.Biome].CampScene
				if want == "" {
					want = g.config.Camping.DefaultScene
				}
				if got := g.campSceneSprite(); got != want {
					t.Fatalf("scene=%s want=%s", got, want)
				}
			})
		}
	}
}

func TestCampCoverPreservesProportionsAndHeads(t *testing.T) {
	for _, res := range campHUDResolutions {
		for _, hud := range []bool{true, false} {
			h := res[1]
			if hud {
				h = gameplayViewportBottomWithPartyHUD(h)
			}
			x, y, w, height := campSceneCoverGeometry(res[0], h, 1536, 1024, .08)
			if math.Abs(w/1536-height/1024) > 1e-9 {
				t.Fatal("non-uniform artwork scaling")
			}
			if x > 0 || y > 0 || x+w < float64(res[0])-1e-9 || y+height < float64(h)-1e-9 {
				t.Fatal("artwork does not cover viewport")
			}
			// All current scenes keep the top of every head below 10% of source height.
			if y+.10*height <= 0 {
				t.Fatalf("%v crops a character's head", res)
			}
		}
	}
}

func TestCampClusterPatterns(t *testing.T) {
	for _, count := range []int{5, 6} {
		for trial := 0; trial < 20; trial++ {
			c := config.DefaultCampingConfig()
			c.DissolveClustersMin, c.DissolveClustersMax = count, count
			p := newCampDissolvePattern(c)
			unique := make(map[[2]float32]bool)
			for i := 0; i < 6; i++ {
				point := [2]float32{p.centers[i*2], p.centers[i*2+1]}
				if point[0] <= 0 || point[0] >= 1 || point[1] <= 0 || point[1] >= 1 {
					t.Fatal("cluster origin outside the picture")
				}
				unique[point] = true
			}
			if len(unique) != count {
				t.Fatalf("got %d cluster origins, want %d", len(unique), count)
			}
			if other := newCampDissolvePattern(c); other.centers == p.centers {
				t.Fatal("new transition reused the same origins")
			}
			for _, res := range campHUDResolutions {
				if r := p.coverageRadius(res[0], res[1]); r <= 0 || math.IsNaN(float64(r)) {
					t.Fatal("invalid cluster coverage after resize")
				}
			}
		}
	}
}
