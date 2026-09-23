package game

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/threading"
	"ugataima/internal/world"
)

func TestWorldClickCannotAcquireTargetAfterTurn(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, kind := range []string{"NPC", "loot"} {
			for _, hold := range []bool{false, true} {
				t.Run(fmt.Sprintf("TB=%v/%s/hold=%v", tb, kind, hold), func(t *testing.T) {
					h := newDisplayedModalHarness(t, 800, 600)
					g := h.g
					g.turnBasedMode, g.menuOpen = tb, false
					g.world.Monsters = nil
					g.world.NPCs = nil
					g.renderHelper = NewRenderingHelper(g)
					g.camera.FOV = squareProjectionFOV(800, 600)
					g.camera.ViewDist = 5000
					g.camera.X, g.camera.Y, g.camera.Angle = 320, 320, 0
					n := &character.NPC{Name: "Click target", Sprite: "missing_pick_fixture", RenderCategory: "npc", SizeClass: "full_tile", X: 320, Y: 384}
					g.cameraPresentation.presented = cameraPose{320, 320, math.Pi / 2}
					g.cameraPresentation.presentedValid = true
					x, y := 0, 0
					g.camera.Angle = math.Pi / 2
					restore := g.beginPresentedCameraSwap()
					if kind == "NPC" {
						g.world.NPCs = []*character.NPC{n}
						sx, sy, size, visible := g.renderHelper.NPCSpriteMetrics(n, n.X, n.Y, 64)
						if !visible {
							t.Fatal("NPC fixture not visible after turn")
						}
						x, y = sx, sy+size/2
					} else {
						g.groundContainers = []GroundContainer{{X: n.X, Y: n.Y, Sprite: n.Sprite, Gold: 7}}
						info := g.groundContainerRenderInfo(&g.groundContainers[0], -1)
						if !info.Visible {
							t.Fatal("loot fixture not visible after turn")
						}
						x, y = info.ScreenX, info.ScreenY+info.SpriteSize/2
					}
					restore()
					g.camera.Angle = 0
					g.cameraPresentation.presented = cameraPose{320, 320, 0}
					if target, _ := g.findNPCAtScreen(x, y); target != nil || g.findGroundContainerIndexAtScreen(x, y, g.groundContainerPickupRange()) >= 0 {
						t.Fatal("initial click must hit empty scenery")
					}
					fp := installFakePointer(t)
					fp.moveTo(x, y)
					h.ui.Draw(h.screen)
					fp.press()
					if err := h.loop.Update(); err != nil {
						t.Fatal(err)
					}
					if hold {
						fp.hold()
					} else {
						fp.release()
					}
					// The target moves under the same screen point as the view turns.
					g.camera.Angle = math.Pi / 2
					g.cameraPresentation.presented = cameraPose{320, 320, math.Pi / 2}
					h.ui.Draw(h.screen)
					for range 3 { // Updates without another Draw must not replay the press.
						if err := h.loop.Update(); err != nil {
							t.Fatal(err)
						}
						if !hold {
							fp.idle()
						}
					}
					if g.dialogActive || g.party.Gold != 0 {
						t.Fatal("empty click acquired a target after turning")
					}
					if len(g.mouseLeftClicks)+len(g.mouseRightClicks) != 0 {
						t.Fatal("unmatched world click survived Update")
					}
					fp.press()
					if err := h.loop.Update(); err != nil {
						t.Fatal(err)
					}
					if kind == "NPC" && g.dialogNPC != n || kind == "loot" && g.party.Gold != 7 {
						t.Fatal("fresh click did not act on the now-visible target")
					}
				})
			}
		}
	}
}

func TestUpdateOwnsBothClickQueueLifetimes(t *testing.T) {
	for _, state := range []string{"world", "captured", "modal", "stale", "title", "creation", "exit"} {
		for _, right := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/right=%v", state, right), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 800, 600)
				fp := installFakePointer(t)
				g := h.g
				g.world.Monsters, g.world.NPCs = nil, nil
				switch state {
				case "modal":
					g.menuOpen = true
				case "title":
					g.appScreen = AppScreenMainMenu
				case "creation":
					g.appScreen = AppScreenPartyCreate
				case "exit":
					g.exitRequested = true
				}
				h.ui.Draw(h.screen)
				g.prevWorldClickAllowed = g.worldClickAllowed()
				if state == "captured" {
					h.ui.beginDisplayedInput()
					h.ui.onDisplayedInput(uiCommandPointer, layoutRect{}, func() { h.ui.displayedInput.capturedGameplay = true })
					h.ui.endDisplayedInput()
				}
				if state == "stale" {
					g.config.Display.ScreenWidth++
				}
				click := queuedClick{x: -10, y: -10, at: time.Now().UnixMilli()}
				if right {
					g.mouseRightClicks = []queuedClick{click, click}
				} else {
					g.mouseLeftClicks = []queuedClick{click, click}
				}
				if err := h.loop.Update(); err != nil && state != "exit" {
					t.Fatal(err)
				}
				if len(g.mouseLeftClicks)+len(g.mouseRightClicks) != 0 {
					t.Fatal("Update retained unmatched click edges")
				}
				if state == "world" {
					// A new context widget must not receive the old right-click.
					fired := 0
					h.ui.beginDisplayedInput()
					h.ui.onDisplayedInput(uiCommandClick, layoutRect{-20, -20, 20, 20}, func() {
						if g.consumeRightClickIn(-20, -20, 0, 0) || g.consumeLeftClickIn(-20, -20, 0, 0) {
							fired++
						}
					})
					h.ui.endDisplayedInput()
					fp.idle()
					if err := h.loop.Update(); err != nil {
						t.Fatal(err)
					}
					if fired != 0 {
						t.Fatal("new widget acquired an earlier miss")
					}
				}
			})
		}
	}
}

func TestEmptyPressCannotArmMonsterHold(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("TB=%v", tb), func(t *testing.T) {
			g, ih, fp, m, _ := mouseCombatHarness(t, tb)
			g.threading = threading.NewThreadingComponents(g.config)
			t.Cleanup(g.threading.Shutdown)
			ui, r := g.gameLoop.ui, g.gameLoop.renderer
			ui.beginDisplayedInput()
			ui.endDisplayedInput()
			g.prevWorldClickAllowed = true
			r.monsterPick.hits = nil
			fp.press()
			if err := g.gameLoop.Update(); err != nil {
				t.Fatal(err)
			}
			fp.hold()
			r.monsterPick.hits = []monsterPickHit{{monster: m, left: 250, top: 150, size: 140, depth: 64}}
			for range rtHoldRepeatDelay + 1 {
				if err := g.gameLoop.Update(); err != nil {
					t.Fatal(err)
				}
			}
			if ih.mouseAttackTarget != nil || m.HitPoints != m.MaxHitPoints {
				t.Fatal("empty press armed an attack on a later target")
			}
			fp.release()
			if err := g.gameLoop.Update(); err != nil {
				t.Fatal(err)
			}
			fp.press()
			if err := g.gameLoop.Update(); err != nil {
				t.Fatal(err)
			}
			if ih.mouseAttackTarget != m || m.HitPoints == m.MaxHitPoints {
				t.Fatal("fresh monster press failed to acquire and attack")
			}
		})
	}
}

func TestTurnBasedClickUsesPresentedPoseDuringTurn(t *testing.T) {
	for _, kind := range []string{"NPC", "loot"} {
		t.Run(kind, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 800, 600)
			g := h.g
			g.turnBasedMode = true
			for _, ch := range g.party.Members {
				ch.ActionsRemaining = 10
			}
			g.world.Monsters, g.world.NPCs = nil, nil
			g.renderHelper = NewRenderingHelper(g)
			g.camera.FOV, g.camera.ViewDist = squareProjectionFOV(800, 600), 5000
			g.camera.X, g.camera.Y = 320, 320
			g.snapFacing(math.Pi / 2)
			n := &character.NPC{Name: "Turn target", Sprite: "missing_pick_fixture", RenderCategory: "npc", SizeClass: "full_tile", X: 320, Y: 384}
			x, y := 0, 0
			if kind == "NPC" {
				g.world.NPCs = []*character.NPC{n}
				sx, sy, size, _ := g.renderHelper.NPCSpriteMetrics(n, n.X, n.Y, 64)
				x, y = sx, sy+size/2
			} else {
				g.groundContainers = []GroundContainer{{X: n.X, Y: n.Y, Sprite: n.Sprite, Gold: 7}}
				info := g.groundContainerRenderInfo(&g.groundContainers[0], -1)
				x, y = info.ScreenX, info.ScreenY+info.SpriteSize/2
			}
			g.snapFacing(0)
			g.beginRenderCameraSwap(time.Now())() // publish the still-empty view
			h.ui.Draw(h.screen)
			fp := installFakePointer(t)
			fp.moveTo(x, y)
			rotate := true
			h.loop.inputHandler.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return rotate && k == ebiten.KeyD })
			fp.press()
			if err := h.loop.Update(); err != nil {
				t.Fatal(err)
			}
			rotate = false
			fp.release()
			if math.Abs(g.camera.Angle-math.Pi/2) > 1e-6 || g.viewTurnFramesLeft == 0 {
				t.Fatalf("fixture did not begin the real TB turn: angle=%g frames=%d turn=%d", g.camera.Angle, g.viewTurnFramesLeft, g.currentTurn)
			}
			for range 3 {
				if err := h.loop.Update(); err != nil {
					t.Fatal(err)
				}
				fp.idle()
			}
			if g.dialogActive || g.party.Gold != 0 {
				t.Fatal("simultaneous turn retargeted the click to an undisplayed object")
			}
			if !g.cameraPresentation.presentedValid || g.cameraPresentation.presented.angle != 0 {
				t.Fatal("catch-up Updates discarded the presented TB pose")
			}
			// Publish a partial turn, then finish the logical animation without Draw.
			shown := g.viewAngleRender
			g.beginRenderCameraSwap(time.Now())()
			g.viewAngleRender, g.viewTurnFramesLeft = g.camera.Angle, 0
			restore := g.beginPresentedCameraSwap()
			if g.camera.Angle != shown {
				t.Error("picking ignored the last shown partial turn")
			}
			restore()
			g.beginRenderCameraSwap(time.Now())() // now the target is actually shown
			fp.press()
			if err := h.loop.Update(); err != nil {
				t.Fatal(err)
			}
			if kind == "NPC" && g.dialogNPC != n || kind == "loot" && g.party.Gold != 7 {
				t.Fatal("fresh click did not use the completed displayed turn")
			}
		})
	}
}

func TestTurnBasedStepKeepsDisplayedClickPose(t *testing.T) {
	for _, kind := range []string{"NPC", "loot"} {
		t.Run(kind, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 800, 600)
			g := h.g
			g.turnBasedMode, g.menuOpen = true, false
			g.world.Width, g.world.Height = 20, 20
			g.world.Tiles = make([][]world.TileType3D, 20)
			for y := range g.world.Tiles {
				g.world.Tiles[y] = make([]world.TileType3D, 20)
			}
			g.collisionSystem = collision.NewCollisionSystem(g.world, 64)
			g.collisionSystem.RegisterEntity(collision.NewEntity("player", 352, 352, 32, 32, collision.CollisionTypePlayer, true))
			for _, ch := range g.party.Members {
				ch.ActionsRemaining = 10
			}
			g.world.Monsters, g.world.NPCs = nil, nil
			g.renderHelper = NewRenderingHelper(g)
			g.camera.FOV, g.camera.ViewDist = squareProjectionFOV(800, 600), 5000
			g.setPartyPosition(352, 352)
			g.snapFacing(0)
			n := &character.NPC{Name: "Step target", Sprite: "missing_pick_fixture", RenderCategory: "npc", SizeClass: "full_tile", X: 480, Y: 384}
			if kind == "NPC" {
				g.world.NPCs = []*character.NPC{n}
			} else {
				g.groundContainers = []GroundContainer{{X: 528, Y: 352, Sprite: n.Sprite, Gold: 7}}
			}
			hit := func(x, y int) bool {
				if kind == "NPC" {
					npc, _ := g.findNPCAtScreen(x, y)
					return npc != nil
				}
				return g.findGroundContainerIndexAtScreen(x, y, math.Inf(1)) >= 0
			}
			// Find a visible pixel that is empty before the step, occupied after it.
			x, y := -1, -1
			for py := 100; py < 450 && x < 0; py += 4 {
				for px := 10; px < 790; px += 4 {
					g.camera.X = 352
					old := hit(px, py)
					g.camera.X = 416
					now := hit(px, py)
					if !old && now {
						x, y = px, py
						break
					}
				}
			}
			if x < 0 {
				t.Fatal("fixture has no newly occupied screen pixel")
			}
			g.camera.X = 352
			g.beginRenderCameraSwap(time.Now())()
			h.ui.Draw(h.screen)
			fp := installFakePointer(t)
			fp.moveTo(x, y)
			moving := true
			h.loop.inputHandler.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return moving && k == ebiten.KeyW })
			fp.press()
			if err := h.loop.Update(); err != nil {
				t.Fatal(err)
			}
			moving = false
			if g.camera.X != 416 {
				t.Fatal("fixture did not take an ordinary TB step")
			}
			if g.dialogActive || g.party.Gold != 0 {
				t.Fatal("step retargeted an empty click before the new view was shown")
			}
			if !g.cameraPresentation.presentedValid || g.cameraPresentation.presented.x != 352 {
				t.Fatal("ordinary step discarded the displayed pose")
			}
			fp.release()
			if err := h.loop.Update(); err != nil {
				t.Fatal(err)
			}
			g.beginRenderCameraSwap(time.Now())()
			h.ui.Draw(h.screen)
			fp.press()
			if err := h.loop.Update(); err != nil {
				t.Fatal(err)
			}
			if kind == "NPC" && g.dialogNPC != n || kind == "loot" && g.party.Gold != 7 {
				t.Fatal("fresh click failed after presenting the new position")
			}
		})
	}
}

func TestDiscontinuousViewDiscardsPointerOwnership(t *testing.T) {
	for _, kind := range []string{"teleport", "facing", "resize"} {
		t.Run(kind, func(t *testing.T) {
			g, ih, fp, _, tick := mouseCombatHarness(t, true)
			fp.press()
			tick()
			if ih.mouseAttackTarget == nil {
				t.Fatal("fixture did not arm hold")
			}
			g.mouseLeftClicks = []queuedClick{{x: 1, y: 1}}
			g.mouseRightClicks = []queuedClick{{x: 1, y: 1}}
			switch kind {
			case "teleport":
				g.setPartyPosition(g.camera.X+64, g.camera.Y)
			case "facing":
				g.snapFacing(math.Pi)
			case "resize":
				g.handleResize(800, 600)
			}
			if ih.mouseAttackTarget != nil || len(g.mouseLeftClicks)+len(g.mouseRightClicks) > 0 {
				t.Fatal("view replacement retained an old press or hold")
			}
		})
	}
}
