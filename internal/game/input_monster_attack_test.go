package game

import (
	"fmt"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/items"
	"ugataima/internal/monster"
)

func mouseCombatHarness(t *testing.T, tb bool) (*MMGame, *InputHandler, *fakePointer, *monster.Monster3D, func()) {
	t.Helper()
	g, ts := summonTileWorld(t)
	g.appScreen = AppScreenInGame
	g.turnBasedMode = tb
	g.camera.Angle = 0.6
	placePlayerAtTile(g, 10, 10, ts)
	g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = 640, 480
	m := monster.NewMonster3DFromConfig(11.5*ts, 10.5*ts, "goblin", g.config)
	m.HitPoints, m.MaxHitPoints = 100000, 100000
	m.PerfectDodge = 0
	g.world.Monsters = []*monster.Monster3D{m}
	for _, ch := range g.party.Members {
		ch.HitPoints = ch.MaxHitPoints
		delete(ch.Equipment, items.SlotSpell)
		ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
		ch.ActionsRemaining = 10
		ch.RTCooldown, ch.OffHandRTCooldown = 0, 0
	}
	ih := NewInputHandler(g)
	ui := &UISystem{game: g}
	r := &Renderer{game: g}
	g.gameLoop = &GameLoop{game: g, inputHandler: ih, ui: ui, renderer: r}
	r.beginMonsterPickFrame()
	r.monsterPick.hits = []monsterPickHit{{monster: m, left: 250, top: 150, size: 140, depth: ts}}
	fp := installFakePointer(t)
	fp.moveTo(320, 220)
	tick := func() {
		g.frameCount++
		g.uiFrameCount++
		if g.spellInputCooldown > 0 {
			g.spellInputCooldown--
		}
		for _, ch := range g.party.Members {
			if ch.RTCooldown > 0 {
				ch.RTCooldown--
			}
			if ch.OffHandRTCooldown > 0 {
				ch.OffHandRTCooldown--
			}
		}
		ui.updateMouseState()
		ih.HandleInput()
	}
	return g, ih, fp, m, tick
}

func TestMouseSmartAttackTapHoldAndRelease(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(map[bool]string{false: "RT", true: "TB"}[tb], func(t *testing.T) {
			g, ih, fp, m, tick := mouseCombatHarness(t, tb)
			facing := g.camera.Angle
			fp.press()
			tick()
			if len(g.slashEffects) != 1 || m.HitPoints == m.MaxHitPoints {
				t.Fatal("fresh mouse press did not aim and fire one smart attack")
			}
			if g.camera.Angle != facing {
				t.Fatal("pointer attack turned the view")
			}
			fp.hold()
			for i := 1; i < rtHoldRepeatDelay-1; i++ {
				tick()
			}
			if len(g.slashEffects) != 1 {
				t.Fatal("short hold fired twice")
			}
			for i := 0; i < 240; i++ {
				tick()
			}
			if len(g.slashEffects) <= 1 {
				t.Fatal("deliberate hold never repeated")
			}
			before := len(g.slashEffects)
			fp.release()
			tick()
			fp.idle()
			for i := 0; i < 120; i++ {
				tick()
			}
			if len(g.slashEffects) != before || ih.mouseAttackTarget != nil {
				t.Fatal("released pointer kept attacking")
			}
			if len(g.mouseLeftClicks) != 0 {
				t.Fatal("combat click leaked to world interactions")
			}
		})
	}
}

func TestMouseSmartAttackOwnershipAndGates(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, reason := range []string{"leave", "dead", "removed", "friendly", "wall", "modal", "HUD", "mode", "frame gap", "drag", "cooldown", "turn", "loading", "world"} {
			t.Run(map[bool]string{false: "RT", true: "TB"}[tb]+"/"+reason, func(t *testing.T) {
				g, _, fp, m, tick := mouseCombatHarness(t, tb)
				fp.press()
				tick()
				fp.hold()
				switch reason {
				case "leave":
					fp.moveTo(50, 50)
				case "dead":
					m.HitPoints = 0
				case "removed":
					g.world.Monsters = nil
				case "friendly":
					m.Bound = true
				case "wall":
					g.depthBuffer = make([]float64, 640)
					for i := range g.depthBuffer {
						g.depthBuffer[i] = 1
					}
				case "modal":
					g.menuOpen = true
				case "HUD":
					g.gameLoop.ui.displayedInput.commands = []uiInputCommand{{bounds: layoutRect{250, 150, 140, 140}}}
				case "mode":
					g.turnBasedMode = !tb
				case "loading":
					g.gameLoop.loading = &gameLoadingState{awaitingFrame: true}
				case "world":
					other := *g.world
					g.world = &other
				case "frame gap":
					g.uiFrameCount += 4
				case "drag":
					g.menuOpen = true
					g.dragPickedUp = true
				case "cooldown":
					g.spellInputCooldown = 1000
				case "turn":
					if tb {
						g.currentTurn = 1
					} else {
						for _, ch := range g.party.Members {
							ch.RTCooldown = 1000
							ch.OffHandRTCooldown = 1000
						}
					}
				}
				before := len(g.slashEffects)
				for i := 0; i < rtHoldRepeatDelay+30; i++ {
					tick()
				}
				if len(g.slashEffects) != before {
					t.Fatal("pointer hold bypassed " + reason)
				}
			})
		}
	}
}

func TestMonsterPointerDisplayedGeometry(t *testing.T) {
	g, _, _, m, _ := mouseCombatHarness(t, false)
	r := g.gameLoop.renderer
	for _, standee := range []bool{false, true} {
		g.camera.Angle = 0
		r.beginMonsterPickFrame()
		ts := float64(g.config.GetTileSize())
		r.monsterPick.hits = append(r.monsterPick.hits, r.makeMonsterPick(UnifiedSpriteRenderData{monster: m, sizeF: 140, bottomF: 300, depthPerp: ts}, m.X, m.Y, math.Pi/2, 250, 160, standee))
		if g.monsterAtScreen(320, 220) != m {
			t.Fatalf("standee=%v: displayed body not selectable", standee)
		}
		if g.monsterAtScreen(320, 400) != nil {
			t.Fatal("empty space below sprite selected")
		}
		// Wall occlusion is checked at the clicked column, not the sprite center.
		g.depthBuffer = make([]float64, 640)
		for i := range g.depthBuffer {
			g.depthBuffer[i] = math.Inf(1)
		}
		g.depthBuffer[310] = 1
		if g.monsterAtScreen(310, 220) != nil || g.monsterAtScreen(330, 220) != m {
			t.Fatal("partial wall coverage not respected")
		}
		g.depthBuffer = nil
		back := *m
		g.world.Monsters = append(g.world.Monsters, &back)
		r.monsterPick.hits = append(r.monsterPick.hits, monsterPickHit{monster: &back, left: 250, top: 150, size: 140, depth: 2 * ts})
		if g.monsterAtScreen(320, 220) != m {
			t.Fatal("overlapping farther monster won")
		}
	}
}

func TestMouseSmartAttackPreservesHealingPriority(t *testing.T) {
	for _, tb := range []bool{false, true} {
		g, _, fp, _, tick := mouseCombatHarness(t, tb)
		caster := g.party.Members[0]
		caster.LearnSpell("heal_other")
		caster.Equipment[items.SlotSpell] = items.Item{Name: "Heal", Type: items.ItemUtilitySpell, SpellEffect: items.SpellEffectHealOther, SpellCost: 4}
		caster.SpellPoints, caster.MaxSpellPoints = 50, 50
		hurt := g.party.Members[1]
		hurt.HitPoints = hurt.MaxHitPoints * 30 / 100
		before := hurt.HitPoints
		fp.press()
		tick()
		if hurt.HitPoints <= before || len(g.slashEffects) != 0 || caster.SpellPoints >= 50 {
			t.Fatalf("TB=%v mouse bypassed smart healing priority", tb)
		}
	}
}

func TestMouseSmartAttackSpellAndSharedInput(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, entry := range []string{"spell", "simultaneous keyboard", "initial cooldown"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, entry), func(t *testing.T) {
				g, ih, fp, _, tick := mouseCombatHarness(t, tb)
				caster := g.party.Members[0]
				switch entry {
				case "spell":
					caster.LearnSpell("fireball")
					caster.Equipment[items.SlotSpell] = items.Item{Type: items.ItemBattleSpell, SpellEffect: "fireball", SpellCost: 4}
					caster.SpellPoints, caster.MaxSpellPoints = 100, 100
				case "simultaneous keyboard":
					ih.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == ebiten.KeyR })
				case "initial cooldown":
					g.spellInputCooldown = 30
				}
				facing := g.camera.Angle
				fp.press()
				tick()
				switch entry {
				case "spell":
					if len(g.magicProjectiles) == 0 || len(g.slashEffects) != 0 || caster.SpellPoints >= 100 {
						t.Fatal("mouse did not use slotted offensive spell")
					}
					p := g.magicProjectiles[0]
					if p.VelX <= 0 || math.Abs(p.VelY) > 1e-6 || g.camera.Angle != facing {
						t.Fatal("spell not aimed at clicked monster with view preserved")
					}
				case "simultaneous keyboard":
					if len(g.slashEffects) != 1 {
						t.Fatal("keyboard and mouse bypassed shared action gate")
					}
				case "initial cooldown":
					if len(g.slashEffects) != 0 {
						t.Fatal("initial press bypassed cooldown")
					}
					fp.hold()
					for i := 0; i < rtHoldRepeatDelay+1; i++ {
						tick()
					}
					if len(g.slashEffects) != 1 {
						t.Fatal("held press did not retry after cooldown")
					}
				}
			})
		}
	}
}

func TestMonsterHoverUsesPointerAttackGates(t *testing.T) {
	for _, state := range []string{"visible", "leave", "dead", "removed", "friendly", "wall", "modal", "HUD", "drag", "world", "editor preview"} {
		t.Run(state, func(t *testing.T) {
			g, _, fp, m, _ := mouseCombatHarness(t, false)
			switch state {
			case "leave":
				fp.moveTo(-1, -1)
			case "dead":
				m.HitPoints = 0
			case "removed":
				g.world.Monsters = nil
			case "friendly":
				m.Bound = true
			case "wall":
				g.depthBuffer = make([]float64, 640)
				for i := range g.depthBuffer {
					g.depthBuffer[i] = 1
				}
			case "modal":
				g.menuOpen = true
			case "HUD":
				g.gameLoop.ui.displayedInput.commands = []uiInputCommand{{bounds: layoutRect{250, 150, 140, 140}}}
			case "drag":
				g.dragPickedUp = true
			case "world":
				other := *g.world
				g.world = &other
			case "editor preview":
				g.editorPreview = &editorPreviewState{}
			}
			r := g.gameLoop.renderer
			r.hoveredMonster = m // A previous frame's selection must clear too.
			r.selectMonsterHover()
			if (r.hoveredMonster == m) != (state == "visible") {
				t.Fatal("hover disagrees with attack targeting")
			}
		})
	}
}
