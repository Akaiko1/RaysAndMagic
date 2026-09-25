package game

import (
	"fmt"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
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

// partySummonKinds lists every pure-summon source the party can own: card
// allies, the druid's Animal Bonding bear and spell summons.
func partySummonKinds(g *MMGame) []struct{ kind, owner string } {
	return []struct{ kind, owner string }{
		{"card summon", cardSummonOwnerPrefix + "test_card"},
		{"druid summon", animalBondingOwner(g.party.Members[0])},
		{"spell summon", summonSpellOwner("summon_ice_elemental")},
	}
}

func markPartySummonKind(g *MMGame, m *monster.Monster3D, kind string) bool {
	for _, s := range partySummonKinds(g) {
		if s.kind == kind {
			markPurePartySummon(m, s.owner)
			return true
		}
	}
	return false
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
		for _, reason := range []string{"outside viewport", "dead", "removed", "charmed", "card summon", "druid summon", "spell summon", "not drawn", "wall", "modal", "HUD", "mode", "drag", "stash picked up", "cooldown", "turn", "loading", "world"} {
			t.Run(map[bool]string{false: "RT", true: "TB"}[tb]+"/"+reason, func(t *testing.T) {
				g, ih, fp, m, tick := mouseCombatHarness(t, tb)
				fp.press()
				tick()
				fp.hold()
				switch reason {
				case "outside viewport":
					fp.moveTo(-1, -1)
				case "dead":
					m.HitPoints = 0
				case "removed":
					g.world.Monsters = nil
				case "charmed":
					m.Pacified = true
				case "card summon", "druid summon", "spell summon":
					markPartySummonKind(g, m, reason)
				case "not drawn":
					g.gameLoop.renderer.beginMonsterPickFrame()
				case "wall":
					g.depthBuffer = make([]float64, 640)
					for x := range g.depthBuffer {
						g.depthBuffer[x] = 1
					}
					r := g.gameLoop.renderer
					r.beginMonsterPickFrame()
					sprite := ebiten.NewImage(1, 1)
					t.Cleanup(sprite.Deallocate)
					if hit, ok := r.prepareMonsterPick(UnifiedSpriteRenderData{monster: m, sprite: sprite, screenXF: 320, sizeF: 140, bottomF: 290, depthPerp: 64, monsterRenderX: m.X, monsterRenderY: m.Y}); ok {
						r.monsterPick.hits = append(r.monsterPick.hits, hit)
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
				case "drag":
					// Inventory drag states only exist while their panel is open.
					g.menuOpen = true
					g.dragPickedUp = true
				case "stash picked up":
					g.stashDragPickedUp = true
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
				if reason != "cooldown" && reason != "turn" && ih.mouseAttackTarget != nil {
					t.Fatal("pointer hold retained ownership after " + reason)
				}
				if (reason == "cooldown" || reason == "turn") && ih.mouseAttackTarget != m {
					t.Fatal("temporary action gate discarded held target")
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

// Case table: RT/TB x billboard/standee x changes to displayed pose. Every
// case starts on an opaque edge pixel which becomes empty in the next pose.
// The hold is transient; world/view replacement tests cover its reset on load.
func TestMouseSmartAttackStickyDisplayedPose(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, standee := range []bool{false, true} {
			for _, change := range []string{"hit shake", "attack frame", "mirror", "movement", "yaw"} {
				if change == "yaw" && !standee {
					continue // Billboards have no yaw.
				}
				t.Run(fmt.Sprintf("TB=%v/standee=%v/%s", tb, standee, change), func(t *testing.T) {
					g, ih, fp, m, tick := mouseCombatHarness(t, tb)
					t.Chdir("../..")
					g.camera.Angle = 0
					g.camera.FOV = squareProjectionFOV(640, 480)
					g.frameCount = 1
					g.config.Graphics.Standee.Enabled = standee
					g.sprites = graphics.NewSpriteManager()
					t.Cleanup(func() {
						g.sprites.EvictResource("goblin", "walking_r")
						g.sprites.EvictResource("goblin", "attacking_r")
					})
					walking := g.sprites.GetAnimation("goblin", "walking_r")
					attack := g.sprites.GetAnimation("goblin", "attacking_r")
					if walking == nil || attack == nil {
						t.Fatal("missing goblin animation fixtures")
					}
					g.depthBuffer = make([]float64, 640)
					for x := range g.depthBuffer {
						g.depthBuffer[x] = math.Inf(1)
					}
					r := g.gameLoop.renderer
					r.beginMonsterPickFrame()
					m.Direction = 0
					s := UnifiedSpriteRenderData{monster: m, sprite: walking.Frames[0], screenXF: 320, spriteSize: 140, sizeF: 140, bottomF: 290, depthPerp: float64(g.config.GetTileSize()), monsterRenderX: m.X, monsterRenderY: m.Y}
					before, ok := r.prepareMonsterPick(s)
					if !ok {
						t.Fatal("initial pose was culled")
					}
					switch change {
					case "hit shake":
						m.HitTintFrames = MonsterHitFlashFrames
					case "attack frame":
						s.sprite = attack.Frames[1]
					case "mirror":
						m.Direction = -math.Pi / 2
						s.monsterFlip = true
					case "movement":
						s.screenXF += 20
						s.monsterRenderY += 8
					case "yaw":
						m.Direction = math.Pi / 4
						m.StandeeYawTick = 0
					}
					after, ok := r.prepareMonsterPick(s)
					if !ok {
						t.Fatal("changed pose was culled")
					}
					if _, known := g.sprites.ImageOpaqueAt(before.sprite, 0, 0); !known {
						t.Fatal("fixture lacks the production alpha mask")
					}
					found := false
					for y := 150; y < 290 && !found; y++ {
						for x := 200; x < 440; x++ {
							r.monsterPick.hits = []monsterPickHit{before}
							if g.monsterAtScreen(x, y) != m {
								continue
							}
							r.monsterPick.hits[0] = after
							if g.monsterAtScreen(x, y) == nil {
								fp.moveTo(x, y)
								found = true
								break
							}
						}
					}
					if !found {
						t.Fatal("pose change did not expose an edge pixel")
					}
					r.monsterPick.hits = []monsterPickHit{before}
					fp.press()
					tick()
					hp := m.HitPoints
					fp.hold()
					r.monsterPick.hits[0] = after
					if tb {
						g.currentTurn = 1
						for range rtHoldRepeatDelay + 1 {
							tick()
						}
						if ih.mouseAttackTarget != m || m.HitPoints != hp {
							t.Fatal("monster turn lost ownership or allowed a party attack")
						}
						g.currentTurn = 0
					}
					for range rtHoldRepeatDelay + 120 {
						tick()
					}
					if ih.mouseAttackTarget != m || m.HitPoints >= hp {
						t.Fatal("empty edge pixel interrupted held attacks")
					}
				})
			}
		}
	}
}

func TestMouseSmartAttackStickyInput(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, change := range []string{"empty", "skipped tick", "retarget"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, change), func(t *testing.T) {
				g, ih, fp, m, tick := mouseCombatHarness(t, tb)
				fp.press()
				tick()
				fp.hold()
				want := m
				switch change {
				case "empty":
					fp.moveTo(50, 50)
				case "skipped tick":
					g.uiFrameCount += 4
				case "retarget":
					other := *m
					want = &other
					g.world.Monsters = append(g.world.Monsters, want)
					// A foreground actor crossing the cursor acquires the hold.
					g.gameLoop.renderer.monsterPick.hits = append(g.gameLoop.renderer.monsterPick.hits, monsterPickHit{monster: want, left: 250, top: 150, size: 140, depth: 32})
				}
				hp := want.HitPoints
				for range rtHoldRepeatDelay + 120 {
					tick()
				}
				if ih.mouseAttackTarget != want || want.HitPoints >= hp {
					t.Fatal("held input did not keep or switch its target")
				}
				fp.release()
				tick()
				if ih.mouseAttackTarget != nil {
					t.Fatal("release retained sticky target")
				}
			})
		}
	}
}

func TestMouseSmartAttackDynamicAcquisition(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, entry := range []string{"empty press", "kill", "kill behind", "removed", "not drawn", "switch back"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, entry), func(t *testing.T) {
				g, ih, fp, first, tick := mouseCombatHarness(t, tb)
				second := *first
				g.world.Monsters = append(g.world.Monsters, &second)
				r := g.gameLoop.renderer
				r.monsterPick.hits = append(r.monsterPick.hits, monsterPickHit{monster: &second, left: 410, top: 150, size: 140, depth: 64})
				if entry == "kill behind" {
					r.monsterPick.hits[1].left = 250
					r.monsterPick.hits[1].depth = 128
				}
				if entry == "empty press" {
					fp.moveTo(50, 50)
				}
				if entry == "kill" || entry == "kill behind" {
					first.HitPoints = 1
				}
				fp.press()
				tick()
				fp.hold()
				switch entry {
				case "empty press":
					if ih.mouseAttackTarget != nil || len(g.slashEffects) != 0 {
						t.Fatal("empty press attacked before finding a target")
					}
				case "kill", "kill behind":
					if first.IsAlive() {
						t.Fatal("initial attack did not kill the first target")
					}
				case "removed":
					g.world.Monsters = []*monster.Monster3D{&second}
				case "not drawn":
					r.monsterPick.hits = r.monsterPick.hits[1:]
				}
				// A targetless interval must not retire the held gesture.
				if entry != "kill behind" {
					fp.moveTo(50, 50)
				}
				for range rtHoldRepeatDelay {
					tick()
				}
				if entry != "kill behind" {
					fp.moveTo(480, 220)
				}
				for range 120 {
					tick()
				}
				if ih.mouseAttackTarget != &second || second.HitPoints == second.MaxHitPoints {
					t.Fatal("held gesture failed to acquire and attack the next target")
				}
				if entry == "switch back" {
					hp := first.HitPoints
					fp.moveTo(320, 220)
					for range 120 {
						tick()
					}
					if ih.mouseAttackTarget != first || first.HitPoints >= hp {
						t.Fatal("moving the held pointer back did not resume attacks on the first target")
					}
				}
			})
		}
	}
}

func TestMouseSmartAttackDynamicAcquisitionCancellation(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, reason := range []string{"release", "modal", "HUD", "world", "mode", "loading", "outside viewport", "HUD press", "outside press"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, reason), func(t *testing.T) {
				g, ih, fp, m, tick := mouseCombatHarness(t, tb)
				fp.moveTo(50, 50)
				if reason == "HUD press" {
					g.gameLoop.ui.displayedInput.commands = []uiInputCommand{{bounds: layoutRect{40, 40, 20, 20}}}
				} else if reason == "outside press" {
					fp.moveTo(-1, -1)
				}
				fp.press()
				tick()
				if reason == "HUD press" || reason == "outside press" {
					if ih.mouseAttackWorld != nil {
						t.Fatal("non-world press armed dynamic acquisition")
					}
				} else if ih.mouseAttackWorld != g.world {
					t.Fatal("empty world press did not arm the gesture")
				}
				fp.hold()
				originalWorld := g.world
				switch reason {
				case "release":
					fp.release()
				case "modal":
					g.menuOpen = true
				case "HUD":
					g.gameLoop.ui.displayedInput.commands = []uiInputCommand{{bounds: layoutRect{40, 40, 20, 20}}}
				case "world":
					other := *g.world
					g.world = &other
				case "mode":
					g.turnBasedMode = !tb
				case "loading":
					g.gameLoop.loading = &gameLoadingState{awaitingFrame: true}
				case "outside viewport":
					fp.moveTo(-1, -1)
				}
				tick()
				if ih.mouseAttackWorld != nil {
					t.Fatal("targetless hold survived cancellation")
				}
				g.menuOpen, g.turnBasedMode, g.world = false, tb, originalWorld
				g.gameLoop.loading = nil
				g.gameLoop.ui.displayedInput.commands = nil
				fp.moveTo(320, 220)
				fp.hold() // No fresh press edge after the cancelled gesture.
				for range rtHoldRepeatDelay + 120 {
					tick()
				}
				if ih.mouseAttackTarget != nil || m.HitPoints != m.MaxHitPoints {
					t.Fatal("cancelled gesture reacquired a target without a fresh press")
				}
			})
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
	for _, state := range []string{"visible", "bound", "leave", "dead", "removed", "charmed", "card summon", "druid summon", "spell summon", "wall", "modal", "HUD", "drag", "world", "editor preview"} {
		t.Run(state, func(t *testing.T) {
			g, _, fp, m, _ := mouseCombatHarness(t, false)
			switch state {
			case "leave":
				fp.moveTo(-1, -1)
			case "dead":
				m.HitPoints = 0
			case "removed":
				g.world.Monsters = nil
			case "bound":
				m.Bound = true // a former enemy stays an attack target
			case "charmed":
				m.Pacified = true
			case "card summon", "druid summon", "spell summon":
				markPartySummonKind(g, m, state)
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
			if (r.hoveredMonster == m) != (state == "visible" || state == "bound") {
				t.Fatal("hover disagrees with attack targeting")
			}
		})
	}
}

// Case table: RT/TB x monster kind x how the pointer reaches it. Summons and
// charmed monsters are never pointer targets; bound former enemies, passive
// monsters and wildlife are. Hovering also skips the caravan, while a fresh
// press on it is an explicit choice.
func TestMouseSmartAttackHoverAcquisitionPolicy(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, kind := range []string{"hostile", "passive", "wildlife", "bound", "caravan", "charmed", "card summon", "druid summon", "spell summon"} {
			for _, entry := range []string{"hover", "retarget", "press"} {
				t.Run(fmt.Sprintf("TB=%v/%s/%s", tb, kind, entry), func(t *testing.T) {
					g, ih, fp, first, tick := mouseCombatHarness(t, tb)
					victim := *first
					// Opposite the first actor, so a swing aimed at one never reaches the other.
					ts := float64(g.config.GetTileSize())
					victim.X, victim.Y = 9.5*ts, 10.5*ts
					switch kind {
					case "passive":
						victim.PassiveUntilAttacked = true
					case "wildlife":
						victim.Disposition = monster.DispositionWildlife
					case "bound":
						victim.Bound = true // Bind Undead / dark-elf binding: a former enemy
					case "caravan":
						victim.Disposition = monster.DispositionCaravan
					case "charmed":
						victim.Pacified = true
					}
					summon := markPartySummonKind(g, &victim, kind)
					if (kind == "passive") != victim.IsPassiveUntilProvoked() || (kind == "wildlife") != victim.IsWildlife() ||
						(kind == "caravan") != victim.IsCaravan() || (kind == "charmed") != victim.Pacified ||
						summon != isPurePartySummon(&victim) || (kind == "bound" || summon) != victim.Bound {
						t.Fatal("bad fixture")
					}
					g.world.Monsters = append(g.world.Monsters, &victim)
					r := g.gameLoop.renderer
					victimHit := monsterPickHit{monster: &victim, left: 250, top: 150, size: 140, depth: 32}
					switch entry {
					case "hover":
						r.monsterPick.hits = []monsterPickHit{victimHit}
						fp.moveTo(50, 50)
					case "press":
						r.monsterPick.hits = []monsterPickHit{victimHit}
					}
					fp.press()
					tick()
					fp.hold()
					switch entry {
					case "hover":
						fp.moveTo(320, 220)
					case "retarget":
						if ih.mouseAttackTarget != first {
							t.Fatal("explicit press did not acquire the first target")
						}
						r.monsterPick.hits = append(r.monsterPick.hits, victimHit)
					}
					firstHP := first.HitPoints
					for range rtHoldRepeatDelay + 120 {
						tick()
					}
					acquirable := kind != "caravan" && kind != "charmed" && !summon
					if entry == "press" {
						acquirable = kind != "charmed" && !summon
					}
					var want *monster.Monster3D
					switch {
					case acquirable:
						want = &victim
					case entry == "retarget":
						want = first
					}
					if ih.mouseAttackTarget != want {
						t.Fatalf("held target = %v, want %v", ih.mouseAttackTarget, want)
					}
					if hit := victim.HitPoints < victim.MaxHitPoints; hit != acquirable {
						t.Fatalf("victim damaged = %v, want %v", hit, acquirable)
					}
					if entry == "retarget" && !acquirable && first.HitPoints >= firstHP {
						t.Fatal("excluded foreground actor interrupted the held enemy")
					}
				})
			}
		}
	}
}

// Case table: RT/TB x action input x why the selected member cannot act. The
// request chains to the next member who can; with nobody able, nothing fires
// and the selection stays put.
func TestPartyActionChainsPastSelectedMember(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, input := range []string{"space", "weapon key", "mouse"} {
			for _, state := range []string{"spent", "dead parked", "nobody"} {
				t.Run(fmt.Sprintf("TB=%v/%s/%s", tb, input, state), func(t *testing.T) {
					g, ih, fp, m, tick := mouseCombatHarness(t, tb)
					spend := func(idx int) {
						if tb {
							g.party.Members[idx].ActionsRemaining = 0
						} else {
							g.party.Members[idx].RTCooldown = 5000
						}
					}
					g.selectedChar = 0
					switch state {
					case "spent":
						spend(0)
					case "dead parked":
						g.party.Members[0].HitPoints = 0
						g.parkSelection = true
					case "nobody":
						for i := range g.party.Members {
							spend(i)
						}
					}
					acted := func(idx int) bool {
						ch := g.party.Members[idx]
						if tb {
							return ch.ActionsRemaining < 10
						}
						return ch.RTCooldown > 0 && ch.RTCooldown != 5000
					}
					key := map[string]ebiten.Key{"space": ebiten.KeySpace, "weapon key": ebiten.KeyR}[input]
					pressed := true
					if input == "mouse" {
						fp.press()
					} else {
						fp.moveTo(5, 5)
						ih.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return pressed && k == key })
					}
					hp := m.HitPoints
					tick()
					pressed = false
					if state == "nobody" {
						if m.HitPoints != hp || g.selectedChar != 0 {
							t.Fatalf("nobody could act, yet damaged=%v selected=%d", m.HitPoints != hp, g.selectedChar)
						}
						return
					}
					if m.HitPoints == hp || !acted(1) || (state == "spent" && tb && g.party.Members[0].ActionsRemaining != 0) {
						t.Fatalf("request did not chain to member 1: damaged=%v acted1=%v selected=%d", m.HitPoints != hp, acted(1), g.selectedChar)
					}
				})
			}
		}
	}
}

// TB F and C chain by capability as well as by action slot: the selected
// member without the spell or heal hands the request to one who has it.
func TestEnsureTBActorUsesActionCapability(t *testing.T) {
	g, _, _, _, _ := mouseCombatHarness(t, true)
	caster := g.party.Members[2]
	caster.Equipment[items.SlotSpell] = items.Item{Type: items.ItemBattleSpell, SpellEffect: "fireball", SpellCost: 4}
	caster.SpellPoints = caster.MaxSpellPoints
	if !g.actionCapable(2, rtActCast) || g.actionCapable(0, rtActCast) {
		t.Fatal("bad caster fixture")
	}
	g.selectedChar, g.parkSelection = 0, true
	if !g.ensureTBActor(rtActCast) || g.selectedChar != 2 || g.parkSelection {
		t.Fatalf("cast request selected %d parked=%v, want the capable caster", g.selectedChar, g.parkSelection)
	}
	caster.ActionsRemaining = 0
	g.selectedChar = 0
	if g.ensureTBActor(rtActCast) || g.selectedChar != 0 {
		t.Fatalf("cast request with no capable actor moved selection to %d", g.selectedChar)
	}
	if !g.ensureTBActor(rtActSmart) || g.selectedChar != 0 {
		t.Fatalf("smart request left the able selected member: %d", g.selectedChar)
	}
}

// Case table: RT/TB x billboard/standee x displayed pose after the press. The
// actor stays in the render candidate list; ownership still requires some
// column inside the viewport and in front of the walls.
func TestMouseSmartAttackHeldTargetNeedsViewport(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, standee := range []bool{false, true} {
			for _, pose := range []string{"above", "below", "side", "wall", "partly visible"} {
				t.Run(fmt.Sprintf("TB=%v/standee=%v/%s", tb, standee, pose), func(t *testing.T) {
					g, ih, fp, m, tick := mouseCombatHarness(t, tb)
					g.camera.Angle = 0
					g.camera.FOV = squareProjectionFOV(640, 480)
					g.config.Graphics.Standee.Enabled = standee
					g.depthBuffer = make([]float64, 640)
					for x := range g.depthBuffer {
						g.depthBuffer[x] = math.Inf(1)
					}
					r := g.gameLoop.renderer
					r.beginMonsterPickFrame()
					s := UnifiedSpriteRenderData{monster: m, screenXF: 320, spriteSize: 140, sizeF: 140, bottomF: 290,
						depthPerp: float64(g.config.GetTileSize()), monsterRenderX: m.X, monsterRenderY: m.Y, sprite: hudWhiteImg}
					hit, ok := r.prepareMonsterPick(s)
					if !ok || hit.standee != standee {
						t.Fatal("bad pick fixture")
					}
					hit.sprite = nil // geometry only: every covered pixel is opaque
					r.monsterPick.hits = []monsterPickHit{hit}
					fp.moveTo(320, 220)
					if g.monsterAtScreen(320, 220) != m {
						t.Fatal("cursor misses the fixture")
					}
					fp.press()
					tick()
					fp.hold()
					moved := hit
					ts := float64(g.config.GetTileSize())
					switch pose {
					case "above":
						moved.top, moved.bottom = -300, -144
					case "below":
						moved.top, moved.bottom = 500, 640
					case "side":
						moved.left, moved.p0y = 700, moved.p0y+20*ts
					case "wall":
						for x := range g.depthBuffer {
							g.depthBuffer[x] = 1
						}
					case "partly visible":
						moved.top, moved.bottom = -120, 20
					}
					if moved.standee {
						// Slab columns scale by depth about the horizon: push the
						// whole oblique plane out of frame, not just its centre.
						switch pose {
						case "above":
							moved.bottom = -2000
						case "below":
							moved.bottom = 5000
						}
						moved.top = moved.bottom - moved.size
					}
					r.monsterPick.hits = []monsterPickHit{moved}
					hp := m.HitPoints
					for range rtHoldRepeatDelay + 120 {
						tick()
					}
					keep := pose == "partly visible"
					if (ih.mouseAttackTarget == m) != keep || (m.HitPoints < hp) != keep {
						t.Fatalf("target kept=%v damaged=%v, want %v", ih.mouseAttackTarget == m, m.HitPoints < hp, keep)
					}
				})
			}
		}
	}
}

// RT/TB: an injured member without a heal is selected and the cursor is off
// the portraits. C hands the cast to the healer, who heals the selected member
// rather than themselves.
func TestHealHandoffKeepsSelectedRecipient(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("TB=%v", tb), func(t *testing.T) {
			g, ih, fp, _, tick := mouseCombatHarness(t, tb)
			fp.moveTo(5, 5)
			g.showPartyStats = false
			for _, ch := range g.party.Members {
				ch.MagicSchools = nil
			}
			patient, healer := g.party.Members[0], g.party.Members[1]
			patient.HitPoints = 1
			healer.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{
				character.MagicSchoolBody: {Mastery: character.MasteryNovice, KnownSpells: []spells.SpellID{"heal_other"}},
			}
			healer.MaxSpellPoints, healer.SpellPoints = 50, 50
			g.selectedChar = 0
			pressed := true
			ih.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return pressed && k == ebiten.KeyC })
			tick()
			pressed = false
			if patient.HitPoints <= 1 || healer.SpellPoints >= 50 {
				t.Fatalf("patient hp=%d healer sp=%d: the handoff did not heal the selected member", patient.HitPoints, healer.SpellPoints)
			}
		})
	}
}
