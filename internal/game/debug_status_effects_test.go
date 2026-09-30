//go:build debug

package game

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/monster"
)

func statusEffectPixels(img *ebiten.Image) []byte {
	b := img.Bounds()
	pixels := make([]byte, b.Dx()*b.Dy()*4)
	img.ReadPixels(pixels)
	return pixels
}
func statusEffectInk(pixels []byte) int {
	n := 0
	for i := 3; i < len(pixels); i += 4 {
		if pixels[i] > 0 {
			n++
		}
	}
	return n
}
func saveStatusEffectImage(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, img)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("save image: %v/%v", err, closeErr)
	}
}

// Drives the shared monster dispatcher and the real party-card compositor,
// including its allocation gate, at two UI sizes. Geometry tests alone would
// miss an unconnected renderer or an idle-card gate that drops the new effects.
func TestDebugSim_StatusEffects(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("requires real Draw frames")
	}
	loadTestConfig(t)
	g, r := bootFxGalleryGame(t)
	defer g.Shutdown()
	g.menuOpen = false
	g.showPartyStats = true
	ui := g.gameLoop.ui
	out := os.Getenv("RAM_STATUS_FX_DIR")
	if out != "" {
		if err := os.MkdirAll(out, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, res := range [][2]int{{800, 600}, {1920, 1080}} {
		w, h := g.gameLoop.Layout(res[0], res[1])
		g.frameCount = 40
		runOnDrawFrame(func(_ *ebiten.Image) {
			dst := ebiten.NewImage(w, h)
			defer dst.Deallocate()
			for _, tc := range statusVisualCatalog {
				if tc.monster == nil {
					continue
				}
				for _, size := range []int{48, 240, 800} {
					m := monster.NewMonster3DFromConfig(g.camera.X+192, g.camera.Y, "wolf", g.config)
					tc.monster(m)
					g.depthBuffer = make([]float64, w)
					g.actorDepthBuffer = make([]float64, w)
					for i := range g.depthBuffer {
						g.depthBuffer[i] = math.Inf(1)
						g.actorDepthBuffer[i] = math.Inf(1)
					}
					s := UnifiedSpriteRenderData{monster: m, screenX: w / 2, spriteSize: size, depthPerp: 192}
					top := h/2 - size/2
					dst.Clear()
					r.drawMonsterStatusFX(dst, s, top)
					a := statusEffectPixels(dst)
					if statusEffectInk(a) == 0 {
						t.Errorf("%s/%d/%d: missing wired monster overlay", tc.key, w, size)
					}
					g.frameCount += 13
					dst.Clear()
					r.drawMonsterStatusFX(dst, s, top)
					b := statusEffectPixels(dst)
					badge := tc.flag&statusBadgeVisuals != 0
					if tc.key != "shred" && !badge && bytes.Equal(a, b) {
						t.Errorf("%s/%d: frozen animation", tc.key, size)
					}
					for i := range g.depthBuffer {
						g.depthBuffer[i] = 100
					}
					dst.Clear()
					r.drawMonsterStatusFX(dst, s, top)
					if statusEffectInk(statusEffectPixels(dst)) != 0 {
						t.Errorf("%s: effect painted through wall", tc.key)
					}
					for i := range g.depthBuffer {
						g.depthBuffer[i] = math.Inf(1)
						g.actorDepthBuffer[i] = 100
					}
					dst.Clear()
					r.drawMonsterStatusFX(dst, s, top)
					// Head badges follow the designation marker: terrain hides
					// them, a closer actor does not.
					if !badge && statusEffectInk(statusEffectPixels(dst)) != 0 {
						t.Errorf("%s: effect painted through closer actor", tc.key)
					}
				}
			}
			checkStatusReviewRendering(t, g, r, dst)
			for _, c := range g.party.Members {
				c.Conditions = nil
				c.StunFramesRemaining = 0
				c.StunTurnsRemaining = 0
				c.HitPoints = c.MaxHitPoints
			}
			conditions := []character.Condition{character.ConditionDiseased, character.ConditionCursed, character.ConditionAsleep, character.ConditionFear, character.ConditionParalyzed, character.ConditionStone}
			for _, cond := range conditions {
				g.party.Members[0].Conditions = []character.Condition{cond}
				g.frameCount += 17
				dst.Clear()
				ui.drawPartyUI(dst)
				layer := ui.partyCardEffectLayer[0]
				if layer == nil || statusEffectInk(statusEffectPixels(layer)) == 0 {
					t.Errorf("%s/%d: party compositor omitted effect", cond, w)
				}
			}
			g.party.Members[0].Conditions = nil
			g.partyRoot = PartyRootState{Frames: 1200, Turns: 5}
			dst.Clear()
			ui.drawPartyUI(dst)
			for i, layer := range ui.partyCardEffectLayer {
				if i < len(g.party.Members) && (layer == nil || statusEffectInk(statusEffectPixels(layer)) == 0) {
					t.Errorf("root/%d: party member %d lacks effect", w, i)
				}
			}
			// The new overlays are portrait-local: they must not paint into the text.
			for _, layer := range ui.partyCardEffectLayer {
				if layer == nil {
					continue
				}
				b := layer.Bounds()
				p := statusEffectPixels(layer)
				for y := 0; y < b.Dy(); y++ {
					for x := panelPortraitX + panelPortraitW + 3; x < b.Dx(); x++ {
						if p[(y*b.Dx()+x)*4+3] != 0 {
							t.Errorf("portrait status overlaps card text at %d,%d", x, y)
							break
						}
					}
				}
			}
			g.partyRoot = PartyRootState{}
		})
		// Capture real gameplay and HUD, rather than a mock-up of the effects.
		g.world.Monsters = nil
		ts := float64(g.config.GetTileSize())
		g.camera.Angle = 0
		m := monster.NewMonster3DFromConfig(g.camera.X+3*ts, g.camera.Y, "wolf", g.config)
		m.RootFramesRemaining = 1200
		m.RootTurnsRemaining = 5
		m.ApplySlow(35, 1200, 5)
		m.Pacified = true
		g.world.Monsters = []*monster.Monster3D{m}
		g.partyRoot = PartyRootState{Frames: 1200, Turns: 5}
		for i, c := range g.party.Members {
			c.Conditions = nil
			switch i {
			case 0:
				c.AddCondition(character.ConditionCursed)
			case 1:
				c.AddCondition(character.ConditionParalyzed)
			case 2:
				c.AddCondition(character.ConditionDiseased)
			case 3:
				c.AddCondition(character.ConditionAsleep)
			}
		}
		if out != "" {
			shot := captureGameplayPreviewFrame(t, g, w, h)
			saveStatusEffectImage(t, filepath.Join(out, fmt.Sprintf("gameplay-%dx%d.png", w, h)), shot)
		}
		g.partyRoot = PartyRootState{}
	}
	if out == "" {
		return
	}
	// Contact sheets show every motif at several phases over shipped mob/portrait
	// art. Actual gameplay screenshots above verify its real placement and scale.
	runOnDrawFrame(func(_ *ebiten.Image) {
		sheet := image.NewRGBA(image.Rect(0, 0, 960, (len(statusVisualCatalog)+3)/4*240))
		for k, e := range statusVisualCatalog {
			tile := ebiten.NewImage(240, 240)
			tile.Fill(color.RGBA{19, 24, 31, 255})
			portrait := ui.cardPortrait(g.portraitSpriteName(g.party.Members[0]), 112, 112, false)
			if portrait != nil {
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Translate(64, 55)
				tile.DrawImage(portrait, op)
			}
			drawUITextColored(tile, e.label, 12, 12, color.RGBA{230, 225, 205, 255})
			// Badges ride above the head with their own renderer, as in play.
			if paint, badge := statusBadgeRenderers[e.flag]; badge {
				paint(tile, 120, 40, 12)
				for j := 0; j < 3; j++ {
					paint(tile, float32(44+j*75), 210, float32(9+j*2))
				}
			} else {
				statusCanvas{dst: tile, x: 64, y: 55, w: 112, h: 112, clock: 63}.draw(e.flag)
				for j := 0; j < 3; j++ {
					statusCanvas{dst: tile, x: float64(22 + j*75), y: 188, w: 45, h: 45, clock: float64(13 + j*19)}.draw(e.flag)
				}
			}
			b := tile.Bounds()
			rgba := image.NewRGBA(b)
			copy(rgba.Pix, statusEffectPixels(tile))
			draw.Draw(sheet, image.Rect(k%4*240, k/4*240, k%4*240+240, k/4*240+240), rgba, image.Point{}, draw.Src)
			tile.Deallocate()
		}
		saveStatusEffectImage(t, filepath.Join(out, "status-contact-sheet.png"), sheet)
	})
}

// Exercise the production dispatcher with the overloaded Bound flag and stale
// final-turn latches. Testing only the mask helper would miss phase wiring.
func checkStatusReviewRendering(t *testing.T, g *MMGame, r *Renderer, dst *ebiten.Image) {
	t.Helper()
	oldTB, oldTurn, oldHUD := g.turnBasedMode, g.currentTurn, g.showPartyStats
	defer func() { g.turnBasedMode, g.currentTurn, g.showPartyStats = oldTB, oldTurn, oldHUD }()
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	for i := range g.depthBuffer {
		g.depthBuffer[i] = math.Inf(1)
		g.actorDepthBuffer[i] = math.Inf(1)
	}
	for _, hud := range []bool{false, true} {
		g.showPartyStats = hud
		for _, phase := range []int{0, 1, 2} {
			g.turnBasedMode, g.currentTurn = phase != 2, phase%2
			for _, name := range []string{"card", "animal", "spell", "animal_of_member", "spell_of_member", "permanent_bind", "timed_bind", "root", "slow", "weaken"} {
				m := monster.NewMonster3DFromConfig(400, 400, "wolf", g.config)
				want := true
				switch name {
				case "card":
					markPurePartySummon(m, cardSummonOwnerPrefix+"test")
					want = false
				case "animal":
					markPurePartySummon(m, animalBondingOwnerPrefix+"Druid")
					want = false
				case "spell":
					markPurePartySummon(m, spellSummonOwnerPrefix+"ice_elemental")
					want = false
				case "animal_of_member":
					// A summoner in the party puts a face over the ally.
					markPurePartySummon(m, animalBondingOwner(g.party.Members[3]))
				case "spell_of_member":
					markPurePartySummon(m, spellSummonOwnerPrefix+"ice_elemental")
					m.SummonerName = g.party.Members[1].Name
				case "permanent_bind":
					m.Bound = true
				case "timed_bind":
					m.Bound = true
					m.BoundFramesRemaining = 100
				case "root":
					m.RootFramesRemaining = 1
					m.RootTurnsRemaining = 1
					m.RootRate = 1
					m.TickRootTurn()
					want = phase == 1
				case "slow":
					m.ApplySlow(50, 1, 1)
					m.TickSlowTurn()
					want = phase == 1
				case "weaken":
					m.ApplyWeaken(50, 1, 1)
					m.TickWeakenTurn()
					want = phase == 1
				}
				dst.Clear()
				r.drawMonsterStatusFX(dst, UnifiedSpriteRenderData{monster: m, screenX: w / 2, spriteSize: 800, depthPerp: 192}, 0)
				pixels := statusEffectPixels(dst)
				if got := statusEffectInk(pixels) > 0; got != want {
					t.Errorf("%s/phase=%d/hud=%v: wired overlay=%v, want %v", name, phase, hud, got, want)
				}
				for y := gameplayViewportBottom(g); y < h; y++ {
					for x := 0; x < w; x++ {
						if pixels[(y*w+x)*4+3] != 0 {
							t.Fatalf("%s crossed gameplay viewport", name)
						}
					}
				}
			}
		}
	}
}
