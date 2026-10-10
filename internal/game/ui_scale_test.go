package game

import (
	"fmt"
	"image"
	"math"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
)

// keepUIScreenScale restores the package's screen scale and frame surfaces
// after a test that lays out or draws a scaled frame.
func keepUIScreenScale(t testing.TB) {
	t.Helper()
	prev := uiScreenScale
	t.Cleanup(func() {
		uiScreenScale = prev
		clear(uiFrameSurfaces)
	})
}

// clearWorldDepth leaves no wall in front of any column of the view.
func clearWorldDepth(g *MMGame) {
	for i := range g.depthBuffer {
		g.depthBuffer[i] = math.Inf(1)
	}
}

// On every test display and preset, every surface the interface draws on -
// the screen Layout sizes, a clip cut from it, an offscreen layer - is
// addressed in the frame's UI units, the pointer is read in the same units,
// and a UI point maps onto the 3D view pixel drawn under it.
func TestUIUnitsOnEveryInterfaceFrame(t *testing.T) {
	keepUIScreenScale(t)
	prevCursor := rawCursorPosition
	t.Cleanup(func() { rawCursorPosition = prevCursor })
	for _, d := range interfaceTestDisplays {
		t.Run(d.name, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			prev := displayDeviceScale
			t.Cleanup(func() { displayDeviceScale = prev })
			displayDeviceScale = func() float64 { return d.scale }
			for _, size := range h.g.config.Display.InterfaceSizes {
				h.g.config.PlayerInterfaceSize = size.Key
				pw, ph := h.loop.Layout(d.outW, d.outH)
				g := h.g
				ui := image.Rect(0, 0, g.config.GetScreenWidth(), g.config.GetScreenHeight())
				screen := ebiten.NewImage(pw, ph)
				beginUIFrame(screen, g.uiPixelScale())
				if got := uiBounds(screen); got != ui {
					t.Fatalf("%s: screen is %v UI units, layouts use %v", size.Key, got, ui)
				}
				clip := image.Rect(7, 11, ui.Dx()/2, ui.Dy()/2)
				if got := uiBounds(uiClip(screen, clip)); got != clip {
					t.Fatalf("%s: clip %v reads back as %v", size.Key, clip, got)
				}
				layer := uiLayer(nil, 123, 45)
				if got := uiBounds(layer); got != image.Rect(0, 0, 123, 45) || !uiLayerFits(layer, 123, 45) {
					t.Fatalf("%s: a 123x45 layer is %v UI units", size.Key, got)
				}
				uiReleaseLayer(layer)
				for _, p := range []image.Point{{0, 0}, {pw / 2, ph / 2}, {pw - 1, ph - 1}} {
					rawCursorPosition = func() (int, int) { return p.X, p.Y }
					x, y := uiCursorPosition()
					s := g.uiPixelScale()
					if x != int(float64(p.X)/s) || y != int(float64(p.Y)/s) || !image.Pt(x, y).In(ui) {
						t.Fatalf("%s: pixel %v reads as unit (%d,%d) at x%.4f", size.Key, p, x, y, s)
					}
				}
				ww, wh := g.worldWidth(), g.worldHeight()
				for _, p := range []image.Point{{0, 0}, {ui.Dx() / 3, ui.Dy() / 2}, {ui.Dx() - 1, ui.Dy() - 1}} {
					wx, wy := g.uiToWorldPoint(p.X, p.Y)
					// The world pixel overlaps the screen area of the UI unit.
					if float64(wx+1) <= float64(p.X*ww)/float64(ui.Dx()) || float64(wx) >= float64((p.X+1)*ww)/float64(ui.Dx()) ||
						float64(wy+1) <= float64(p.Y*wh)/float64(ui.Dy()) || float64(wy) >= float64((p.Y+1)*wh)/float64(ui.Dy()) {
						t.Fatalf("%s: unit %v maps to world pixel (%d,%d) of %dx%d", size.Key, p, wx, wy, ww, wh)
					}
					if ww == ui.Dx() && (wx != p.X || wy != p.Y) {
						t.Fatalf("%s: equal frames moved unit %v to (%d,%d)", size.Key, p, wx, wy)
					}
				}
			}
		})
	}
}

// uiDrawImage's transform: art is resampled once to the destination, a layer
// lands pixel for pixel on a surface of its own scale, and the nearest filter
// is swapped only where a pixel could not double evenly.
func TestUIImageOptions(t *testing.T) {
	translate := &ebiten.DrawImageOptions{}
	translate.GeoM.Translate(10, 20)
	linear := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	linear.GeoM.Translate(10, 20)
	for _, tc := range []struct {
		name               string
		dstScale, srcScale float64
		op                 *ebiten.DrawImageOptions
		origin, nextPixel  [2]float64 // where source pixels (0,0) and (1,0) land
		filter             ebiten.Filter
	}{
		{"art on a x3 screen", 3, 1, translate, [2]float64{30, 60}, [2]float64{33, 60}, ebiten.FilterNearest},
		{"x3 layer on a x3 screen", 3, 3, translate, [2]float64{30, 60}, [2]float64{31, 60}, ebiten.FilterNearest},
		{"art on a x1.25 screen", 1.25, 1, translate, [2]float64{12.5, 25}, [2]float64{13.75, 25}, ebiten.FilterPixelated},
		{"art below scale 1", 0.75, 1, translate, [2]float64{7.5, 15}, [2]float64{8.25, 15}, ebiten.FilterLinear},
		{"x3 layer on a plain image", 1, 3, translate, [2]float64{10, 20}, [2]float64{10 + 1.0/3, 20}, ebiten.FilterLinear},
		{"authored linear stays", 1.25, 1, linear, [2]float64{12.5, 25}, [2]float64{13.75, 25}, ebiten.FilterLinear},
		{"no options", 2, 1, nil, [2]float64{0, 0}, [2]float64{2, 0}, ebiten.FilterNearest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := uiImageOptions(tc.dstScale, tc.srcScale, tc.op)
			near := func(a, b [2]float64) bool { return math.Abs(a[0]-b[0]) < 1e-9 && math.Abs(a[1]-b[1]) < 1e-9 }
			ox, oy := got.GeoM.Apply(0, 0)
			nx, ny := got.GeoM.Apply(1, 0)
			if !near([2]float64{ox, oy}, tc.origin) || !near([2]float64{nx, ny}, tc.nextPixel) {
				t.Fatalf("pixels land at (%.3f,%.3f) and (%.3f,%.3f), want %v and %v", ox, oy, nx, ny, tc.origin, tc.nextPixel)
			}
			if got.Filter != tc.filter {
				t.Fatalf("filter %v, want %v", got.Filter, tc.filter)
			}
		})
	}
	if translate.GeoM.Element(0, 2) != 10 {
		t.Fatal("the caller's options were modified")
	}
}

// The 3D scene covers the whole screen: exact whole steps, and Ebitengine's
// own screen filters for anything else.
func TestSceneToScreen(t *testing.T) {
	for _, tc := range []struct {
		name          string
		scene, screen image.Point
		sx, sy        float64
		filter        ebiten.Filter
	}{
		{"same size", image.Pt(1920, 1080), image.Pt(1920, 1080), 1, 1, ebiten.FilterNearest},
		{"x2 HiDPI", image.Pt(1920, 1080), image.Pt(3840, 2160), 2, 2, ebiten.FilterNearest},
		{"one column short", image.Pt(1366, 768), image.Pt(1365, 768), 1, 1, ebiten.FilterNearest},
		{"fractional up", image.Pt(1728, 1080), image.Pt(1920, 1200), 1920.0 / 1728, 1200.0 / 1080, ebiten.FilterPixelated},
		{"down", image.Pt(1920, 1080), image.Pt(1280, 720), 1280.0 / 1920, 720.0 / 1080, ebiten.FilterLinear},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, filter := sceneToScreen(image.Rectangle{Max: tc.scene}, image.Rectangle{Max: tc.screen})
			if m.Element(0, 0) != tc.sx || m.Element(1, 1) != tc.sy || filter != tc.filter {
				t.Fatalf("scale %.4f x %.4f filter %v, want %.4f x %.4f filter %v", m.Element(0, 0), m.Element(1, 1), filter, tc.sx, tc.sy, tc.filter)
			}
		})
	}
}

// A click is made in UI units and the world is drawn in its own pixels; above
// Normal the two frames differ, and each world pick - NPC, loot, monster -
// lands on the object drawn under the pointer, through the real input path.
func TestWorldClicksHitWhatIsDrawnAtEveryInterfaceSize(t *testing.T) {
	ui := image.Pt(640, 480)
	for _, frame := range []struct {
		name  string
		world image.Point
	}{
		{"normal", ui},
		{"large", image.Pt(960, 720)},
		{"fractional", image.Pt(874, 656)},
	} {
		for _, view := range []struct {
			name     string
			wide     bool
			distance float64
		}{{"classic", false, 0}, {"wide perspective", true, 1}, {"panini", true, 1}, {"partial panini", true, 0.5}} {
			for _, kind := range []string{"NPC", "loot", "monster"} {
				t.Run(fmt.Sprintf("%s/%s/%s", frame.name, view.name, kind), func(t *testing.T) {
					if kind == "monster" {
						g, _, fp, m, tick := mouseCombatHarness(t, false)
						g.handleResize(ui, frame.world)
						g.combatPreferences.WideView = view.wide
						g.combatPreferences.PaniniDisabled = view.name == "wide perspective"
						g.config.Graphics.View.WidePaniniDistance = view.distance
						g.gameLoop.worldProjection = g.widePaniniProjection()
						clearWorldDepth(g)
						r := g.gameLoop.renderer
						r.beginMonsterPickFrame()
						wx, wy := frame.world.X*3/4, frame.world.Y*2/5
						ux, uy := presentedPickPoint(g, wx, wy)
						fp.moveTo(ux, uy)
						r.monsterPick.hits = []monsterPickHit{{monster: m, left: float64(wx - 20), top: float64(wy - 20), size: 40, depth: 64}}
						fp.press()
						tick()
						if m.HitPoints == m.MaxHitPoints {
							t.Fatalf("press over the drawn monster (world %d,%d) did not attack", wx, wy)
						}
						return
					}
					h := newDisplayedModalHarness(t, ui.X, ui.Y)
					g := h.g
					g.handleResize(ui, frame.world)
					g.combatPreferences.WideView = view.wide
					g.combatPreferences.PaniniDisabled = view.name == "wide perspective"
					g.config.Graphics.View.WidePaniniDistance = view.distance
					g.gameLoop.worldProjection = g.widePaniniProjection()
					clearWorldDepth(g)
					g.menuOpen = false
					g.world.Monsters, g.world.NPCs = nil, nil
					g.renderHelper = NewRenderingHelper(g)
					g.camera.ViewDist = 5000
					g.camera.X, g.camera.Y = 320, 320
					g.snapFacing(0)
					n := &character.NPC{Name: "Pick target", Sprite: "missing_pick_fixture", RenderCategory: "npc", SizeClass: "full_tile", X: 430, Y: 365}
					var wx, wy int
					hit := func(x, y int) bool { return false }
					if kind == "NPC" {
						g.world.NPCs = []*character.NPC{n}
						ex, ey := g.npcEffectivePos(n)
						sx, sy, size, visible := g.renderHelper.NPCSpriteMetrics(n, ex, ey, Distance(g.camera.X, g.camera.Y, ex, ey))
						if !visible {
							t.Fatal("NPC fixture not visible")
						}
						wx, wy = sx, sy+size/2
						hit = func(x, y int) bool { return g.npcScreenHitTest(n, ex, ey, x, y) }
					} else {
						g.groundContainers = []GroundContainer{{X: 420, Y: 370, Sprite: n.Sprite, Gold: 7}}
						info := g.groundContainerRenderInfo(&g.groundContainers[0], -1)
						if !info.Visible {
							t.Fatal("loot fixture not visible")
						}
						wx, wy = info.ScreenX, info.ScreenY+info.SpriteSize/2
						hit = func(x, y int) bool {
							return g.groundContainerHitTestFromInfo(info, n.Sprite, x, y, g.groundContainerPickupRange())
						}
					}
					ux, uy := presentedPickPoint(g, wx, wy)
					if !view.wide && frame.world != ui && hit(ux, uy) {
						t.Fatal("fixture: the unit's own coordinates also hit, so it cannot tell the frames apart")
					}
					g.beginRenderCameraSwap(time.Now())()
					h.ui.Draw(h.screen)
					fp := installFakePointer(t)
					fp.moveTo(ux, uy)
					fp.press()
					if err := h.loop.Update(); err != nil {
						t.Fatal(err)
					}
					if kind == "NPC" && g.dialogNPC != n || kind == "loot" && g.party.Gold != 7 {
						t.Fatalf("click at unit (%d,%d) missed the %s drawn at world (%d,%d)", ux, uy, kind, wx, wy)
					}
				})
			}
		}
	}
}

// Independently project a known source pixel by its angular ray. Input must
// undo this when NPC, loot and monster dispatchers consume a displayed click.
func presentedPickPoint(g *MMGame, x, y int) (int, int) {
	sx, sy := float64(x)+0.5, float64(y)+0.5
	d := g.config.Graphics.View.WidePaniniDistance
	if g.combatPreferences.WideView && !g.combatPreferences.PaniniDisabled && d > 0 {
		f, cx, cy := g.viewFocal(), float64(g.worldWidth())/2, g.viewHorizon()
		edge := math.Atan(cx / f)
		a := cx * (d + math.Cos(edge)) / math.Sin(edge)
		theta := math.Atan((sx - cx) / f)
		sx = cx + a*math.Sin(theta)/(d+math.Cos(theta))
		sy = cy + (sy-cy)*a*math.Cos(theta)/(f*(d+math.Cos(theta)))
	}
	return int(sx * float64(g.config.GetScreenWidth()) / float64(g.worldWidth())),
		int(sy * float64(g.config.GetScreenHeight()) / float64(g.worldHeight()))
}

// The HUD combat log keeps Normal's physical size at every interface size, so
// a larger preset cannot have it cover the view. It still sits above the
// party cards and inside the screen, its click region is the block it draws,
// and at Normal it keeps the layout it always had.
func TestHUDCombatLogKeepsNormalSize(t *testing.T) {
	keepUIScreenScale(t)
	for _, d := range interfaceTestDisplays {
		t.Run(d.name, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			for i := 0; i < 12; i++ {
				g.AddCombatMessage(fmt.Sprintf("Goblin %d strikes Gareth for %d damage and staggers back into the brush.", i, 10+i))
			}
			prev := displayDeviceScale
			t.Cleanup(func() { displayDeviceScale = prev })
			displayDeviceScale = func() float64 { return d.scale }
			var normal [2]float64
			for i, size := range g.config.Display.InterfaceSizes {
				g.config.PlayerInterfaceSize = size.Key
				h.loop.Layout(d.outW, d.outH)
				lines := len(g.hudMessageLines())
				lx, ly, lw, lh := g.hudMessageLogRect(lines)
				s0 := g.interfaceFrames[0].pixelScale()
				physical := [2]float64{float64(lw) * s0, float64(lh) * s0}
				if i == 0 {
					normal = physical
					if bx, by, bw, bh := g.hudMessageBlockRect(lines); bx != g.config.GetScreenWidth()-hudMessageWidth-15 || bw != hudMessageWidth || by+bh > ly+lh || bw != lw || bh != lh {
						t.Fatalf("normal: block %d,%d %dx%d moved from the Normal layout (log %d,%d %dx%d)", bx, by, bw, bh, lx, ly, lw, lh)
					}
				} else if physical != normal {
					t.Fatalf("%s: log %.0fx%.0f px, Normal draws it %.0fx%.0f px", size.Key, physical[0], physical[1], normal[0], normal[1])
				}
				unit := g.hudLogUnit()
				bx, by, bw, bh := g.hudMessageBlockRect(lines)
				_, _, _, partyTop := partyPortraitLayout(g)
				if bx < 0 || by < 0 || by+bh > partyTop || float64(bx) > float64(lx)*unit || float64(bx+bw) < float64(lx+lw)*unit || float64(by+bh) < float64(ly+lh)*unit {
					t.Fatalf("%s: click block %d,%d %dx%d vs drawn log %d,%d %dx%d at x%.3f, party cards from %d", size.Key, bx, by, bw, bh, lx, ly, lw, lh, unit, partyTop)
				}
				if bx+bw > g.config.GetScreenWidth() {
					t.Fatalf("%s: log leaves the screen: right %d of %d", size.Key, bx+bw, g.config.GetScreenWidth())
				}
			}
		})
	}
}
