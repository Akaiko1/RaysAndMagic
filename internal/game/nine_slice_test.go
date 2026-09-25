package game

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/graphics"
)

// Pattern detection remains covered independently of retired artwork.
func TestPatternFrame_MeasuresPeriodicStrips(t *testing.T) {
	for _, horizontal := range []bool{true, false} {
		img := image.NewNRGBA(image.Rect(0, 0, 320, 320))
		for y := 0; y < 320; y++ {
			for x := 0; x < 320; x++ {
				pos := y
				if horizontal {
					pos = x
				}
				c := color.NRGBA{R: uint8((pos - 32 + 128) % 128), A: 255}
				if pos < 32 || pos >= 288 {
					c.G = uint8(pos%32 + 1)
				}
				img.SetNRGBA(x, y, c)
			}
		}
		got, ok := analyzeStrip(img, img.Bounds(), horizontal)
		want := stripPattern{capA: 32, capB: 32, period: 128}
		if !ok || got != want {
			t.Fatalf("horizontal=%v: %+v, %v; want %+v", horizontal, got, ok, want)
		}
	}
}

// All legacy panel roles must draw current metal masters without their retired
// PNGs. Cells: seven role aliases plus retained painted art. Persistence: N/A.
func TestPatternFrame_UIFrameInventory(t *testing.T) {
	t.Chdir("../..")
	ui := &UISystem{game: &MMGame{sprites: graphics.NewSpriteManager()}}
	ui.game.validateInterfaceArt()
	screen := ebiten.NewImage(520, 360)
	defer screen.Deallocate()
	for _, tc := range []struct {
		name  string
		style interfaceFrame
	}{
		{"menu_panel_frame", frameGold},
		{"menu_panel_slot", frameGold},
		{"menu_panel_tall", frameSilver},
		{"menu_panel_wide", frameGold},
		{"menu_panel_slatted", frameSilver},
		{"menu_panel_parchment", frameBronze},
		{"character_scroll_panel", frameBronze},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ui.patternPlans = patternPlanCache{}
			ui.drawPatternFrame(screen, tc.name, 0, 0, 520, 360, generatedPatternFrameSlice)
			if len(ui.patternPlans.entries) != 1 || ui.patternPlans.entries[0].name != interfaceFrames[tc.style].name {
				t.Fatal("panel role did not resolve to its current metal master")
			}
		})
	}
	for _, name := range []string{"inventory_grid_panel", "party_member_panel"} {
		if !ui.game.sprites.HasSprite(name) {
			t.Fatalf("retained painted frame %s missing", name)
		}
		if got := analyzePatternFrame(name, 16); got != nil {
			t.Errorf("%s now analyzes as periodic %+v", name, got)
		}
	}
}

// Geometry fixture for periodic blit coverage and cache tests. Runtime masters
// have plain rails; periodic planning must remain independent of retired PNGs.
func testPeriodicFrame() *patternFrame {
	sp := stripPattern{capA: 32, capB: 32, period: 128}
	return &patternFrame{w: 512, h: 512, slice: generatedPatternFrameSlice,
		top: sp, bottom: sp, left: sp, right: sp, centreH: sp, centreV: sp}
}

// Every plan is pure 1:1 blits that cover the panel exactly once - no gaps,
// no double-drawn pixels, no source reads outside the sprite.
func TestPatternFramePlan_CoversEveryPixelExactlyOnce(t *testing.T) {
	pf := testPeriodicFrame()
	for _, size := range [][2]int{{520, 360}, {800, 120}, {130, 130}, {521, 363}, {97, 97}} {
		assertPlanCoverage(t, pf, size[0], size[1], generatedCompactFrameSourceScale, true)
	}
}

func TestGeneratedFrameInsetsClearUIContent(t *testing.T) {
	compactCorner := scaledFrameLength(generatedPatternFrameSlice, generatedCompactFrameSourceScale)
	slotCorner := scaledFrameLength(generatedPatternFrameSlice, generatedSlotFrameSourceScale)
	tallCorner := scaledFrameLength(generatedPatternFrameSlice, generatedTallPanelFrameSourceScale)

	if compactCorner != 16 || slotCorner != 12 || tallCorner > partyHeroDetailPortraitInset {
		t.Fatalf("frame geometry = compact:%d slot:%d tall:%d", compactCorner, slotCorner, tallCorner)
	}

	menu := computeTabbedMenuLayout(1280, gameplayViewportBottomWithPartyHUD(720))
	for i, tab := range menu.tabs {
		if tab.y-menu.panel.y < compactCorner {
			t.Errorf("tab %d starts inside the %dpx frame", i, compactCorner)
		}
	}
	if menu.close.y-menu.panel.y < compactCorner {
		t.Errorf("close button starts inside the %dpx frame", compactCorner)
	}

	character := computeCharacterContentLayout(menu.content)
	if character.portrait.x-character.portraitFrame.x < compactCorner ||
		character.portrait.y-character.portraitFrame.y < compactCorner {
		t.Errorf("character portrait does not clear the %dpx frame", compactCorner)
	}
	if character.portraitFrame.x < character.profile.x || character.portraitFrame.y < character.profile.y ||
		character.portraitFrame.right() > character.profile.right() || character.portraitFrame.bottom() > character.profile.bottom() {
		t.Errorf("character portrait frame %+v leaves profile %+v", character.portraitFrame, character.profile)
	}
	if character.profile.y != character.attributes.y || character.attributes.y != character.magic.y {
		t.Errorf("character dashboard top sections do not align: profile=%d attributes=%d magic=%d",
			character.profile.y, character.attributes.y, character.magic.y)
	}
	for _, card := range []rect{{10, 20, 98, 147}, {10, 20, 120, 180}, {10, 20, 140, 210}} {
		picture := heroCardPortraitRect(card)
		if picture.x <= card.x || picture.y <= card.y || picture.x+picture.w >= card.x+card.w || picture.y+picture.h >= card.y+card.h*75/100 {
			t.Fatalf("portrait %+v overlaps the trading-card rim or name area %+v", picture, card)
		}
	}
}

// The plan geometry is art-agnostic: any caps/periods/sprite size must yield
// exact coverage at any panel size, or refuse cleanly (fallback to stretch).
func TestPatternFramePlan_GeometryHoldsForArbitraryPatterns(t *testing.T) {
	for _, slice := range []int{4, 16, 23} {
		for _, spriteRun := range []int{2*slice + 9, 2*slice + 64} {
			for _, capA := range []int{0, 3} {
				for _, period := range []int{2, 7} {
					srcLen := spriteRun - 2*slice
					if capA*2+period > srcLen {
						continue
					}
					sp := stripPattern{capA: capA, capB: capA, period: period}
					m := min(capA+1, srcLen/3)
					pf := &patternFrame{
						w: spriteRun, h: spriteRun, slice: slice,
						top: sp, bottom: sp, left: sp, right: sp,
						centreH: stripPattern{capA: m, capB: m, period: period},
						centreV: stripPattern{capA: m, capB: m, period: period},
					}
					for _, size := range [][2]int{
						{spriteRun, spriteRun}, {spriteRun + 1, spriteRun + 13},
						{3 * spriteRun, spriteRun}, {257, 121}, {2*slice + capA*2 + 2, 2*slice + capA*2 + 2},
					} {
						assertPlanCoverage(t, pf, size[0], size[1], 1, false)
					}
				}
			}
		}
	}
}

// assertPlanCoverage checks the exactly-once contract; mustPlan fails the test
// when the plan refuses (a refusal is a legal stretch fallback otherwise).
func assertPlanCoverage(t *testing.T, pf *patternFrame, w, h, sourceScale int, mustPlan bool) {
	t.Helper()
	ops, ok := planPatternFrame(pf, w, h, sourceScale)
	if !ok {
		if mustPlan {
			t.Errorf("%dx%d: plan refused", w, h)
		}
		return
	}
	cover := make([]int, w*h)
	for _, op := range ops {
		if op.sw <= 0 || op.sh <= 0 || op.dw <= 0 || op.dh <= 0 {
			t.Fatalf("%dx%d: empty op %+v", w, h, op)
		}
		if op.sx < 0 || op.sy < 0 || op.sx+op.sw > pf.w || op.sy+op.sh > pf.h {
			t.Fatalf("%dx%d: op reads outside the sprite: %+v", w, h, op)
		}
		for y := op.dy; y < op.dy+op.dh; y++ {
			for x := op.dx; x < op.dx+op.dw; x++ {
				if x < 0 || y < 0 || x >= w || y >= h {
					t.Fatalf("%dx%d: op draws outside the panel: %+v", w, h, op)
				}
				cover[y*w+x]++
			}
		}
	}
	for i, c := range cover {
		if c != 1 {
			t.Fatalf("%dx%d: pixel (%d,%d) covered %d times, want exactly once", w, h, i%w, i/w, c)
		}
	}
}
