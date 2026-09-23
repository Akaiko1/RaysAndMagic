package game

import (
	"fmt"
	"math"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

func TestPointerMeleeKeepsSelectedTarget(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for arc := 1; arc <= 4; arc++ {
			for _, side := range []float64{-1, 1} {
				t.Run(fmt.Sprintf("TB=%v/arc=%d/side=%g", tb, arc, side), func(t *testing.T) {
					g, _, fp, target, tick := mouseCombatHarness(t, tb)
					ts := float64(g.config.GetTileSize())
					g.camera.Angle = 0
					target.Y += side * ts
					front := monster.NewMonster3DFromConfig(target.X, g.camera.Y, "goblin", g.config)
					front.HitPoints, front.MaxHitPoints, front.PerfectDodge = 100000, 100000, 0
					g.world.Monsters = append(g.world.Monsters, front)
					def := lookupWeaponConfigByName(items.CreateWeaponFromYAML("iron_spear").Name)
					before := def.Melee.ArcType
					t.Cleanup(func() { def.Melee.ArcType = before })
					def.Melee.ArcType = arc
					for _, ch := range g.party.Members {
						ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_spear")
					}
					fp.press()
					tick()
					if target.HitPoints == target.MaxHitPoints {
						t.Fatal("explicit diagonal target was displaced by front-slot assistance")
					}
					if arc == 1 && front.HitPoints != front.MaxHitPoints {
						t.Fatal("narrow pointer attack hit the unselected front slot")
					}
					if g.camera.Angle != 0 || g.combat.partyAimTarget != nil {
						t.Fatal("aim escaped its action scope")
					}
					beforeHP := target.HitPoints
					fp.hold()
					for range rtHoldRepeatDelay + 120 {
						tick()
					}
					if target.HitPoints >= beforeHP {
						t.Fatal("held action lost its explicit target")
					}
				})
			}
		}
	}
}

func TestPointerProjectileKeepsWorldAim(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, kind := range []string{"bow", "spell"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, kind), func(t *testing.T) {
				g, _, fp, target, tick := mouseCombatHarness(t, tb)
				ts := float64(g.config.GetTileSize())
				g.camera.Angle = 0
				target.Y += ts
				front := monster.NewMonster3DFromConfig(target.X, g.camera.Y, "goblin", g.config)
				front.HitPoints, front.MaxHitPoints, front.PerfectDodge = 100000, 100000, 0
				g.world.Monsters = append(g.world.Monsters, front)
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				ch := g.party.Members[0]
				ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("hunting_bow")
				if kind == "spell" {
					ch.LearnSpell("fireball")
					ch.Equipment[items.SlotSpell] = items.Item{Type: items.ItemBattleSpell, SpellEffect: "fireball", SpellCost: 4}
					ch.SpellPoints, ch.MaxSpellPoints = 100, 100
				}
				fp.press()
				tick()
				// Advance the real published collision entity to impact, after turning
				// away. Camera-dependent front-slot assistance must not retarget it.
				if kind == "bow" {
					if len(g.arrows) == 0 {
						t.Fatal("missing arrow")
					}
					p := &g.arrows[0]
					if !p.WorldAim || math.Abs(p.VelX-p.VelY) > 1e-6 {
						t.Fatal("arrow lost selected world aim")
					}
					p.X, p.Y = target.X, target.Y
					g.collisionSystem.UpdateEntity(p.ID, p.X, p.Y)
				} else {
					if len(g.magicProjectiles) == 0 {
						t.Fatal("missing spell")
					}
					p := &g.magicProjectiles[0]
					if !p.WorldAim || math.Abs(p.VelX-p.VelY) > 1e-6 {
						t.Fatal("spell lost selected world aim")
					}
					p.X, p.Y = target.X, target.Y
					g.collisionSystem.UpdateEntity(p.ID, p.X, p.Y)
				}
				g.camera.Angle = math.Pi
				g.combat.CheckProjectileMonsterCollisions()
				if target.HitPoints == target.MaxHitPoints {
					t.Fatal("camera turn prevented explicit projectile impact")
				}
				if kind == "bow" && front.HitPoints != front.MaxHitPoints {
					t.Fatal("projectile acquired another front slot")
				}
				if g.combat.partyAimTarget != nil {
					t.Fatal("launch retained input context")
				}
			})
		}
	}
}

func TestMonsterAndNPCPointerDepthPriority(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, where := range []string{"far behind", "near behind", "in front", "no monster"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, where), func(t *testing.T) {
				g, ih, fp, m, tick := mouseCombatHarness(t, tb)
				g.camera.Angle = 0
				g.camera.FOV = squareProjectionFOV(640, 480)
				g.camera.ViewDist = 5000
				g.renderHelper = NewRenderingHelper(g)
				ts := float64(g.config.GetTileSize())
				distance := 3 * ts
				if where == "near behind" {
					distance = 1.5 * ts
				}
				if where == "in front" {
					distance = 0.5 * ts
				}
				npc := &character.NPC{Name: "Door", Sprite: "missing_pick_fixture", RenderCategory: "npc", SizeClass: "full_tile", X: g.camera.X + distance, Y: g.camera.Y}
				g.world.NPCs = []*character.NPC{npc}
				sx, sy, size, visible := g.renderHelper.NPCSpriteMetrics(npc, npc.X, npc.Y, distance)
				if !visible {
					t.Fatal("fixture door not visible")
				}
				x, y := sx, sy+size/2
				r := g.gameLoop.renderer
				r.beginMonsterPickFrame()
				r.monsterPick.hits = []monsterPickHit{{monster: m, left: float64(x - 20), top: float64(y - 20), size: 40, depth: ts}}
				back := monster.NewMonster3DFromConfig(g.camera.X+2*ts, g.camera.Y, "goblin", g.config)
				g.world.Monsters = append(g.world.Monsters, back)
				r.monsterPick.hits = append(r.monsterPick.hits, monsterPickHit{monster: back, left: float64(x - 20), top: float64(y - 20), size: 40, depth: 2 * ts})
				if where == "no monster" {
					r.monsterPick.hits = nil
				}
				fp.moveTo(x, y)
				wantMonster := where == "far behind" || where == "near behind"
				if (g.monsterAtScreen(x, y) == m) != wantMonster {
					t.Fatal("hover disagrees with visible object depth")
				}
				fp.press()
				tick()
				if (m.HitPoints < m.MaxHitPoints) != wantMonster || (ih.mouseAttackTarget == m) != wantMonster {
					t.Fatal("door stole a foreground monster click, or monster stole a foreground door click")
				}
				if where == "in front" && g.dialogNPC != npc {
					t.Fatal("foreground reachable NPC lost interaction")
				}
				if where == "no monster" && (g.dialogActive || len(g.combatLogHistory) == 0) {
					t.Fatal("distant NPC no longer reports interaction range")
				}
			})
		}
	}
}

func TestPointerAdaptiveTrapKeepsTarget(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprint(tb), func(t *testing.T) {
			g, _, fp, target, tick := mouseCombatHarness(t, tb)
			ts := float64(g.config.GetTileSize())
			target.Y += ts
			ch := character.CreateCharacter("Trapper", character.ClassThief, g.config)
			ch.Level, ch.SpellPoints, ch.MaxSpellPoints, ch.ActionsRemaining = 10, 100, 100, 10
			g.party.Members[0] = ch
			if !equipTrap(ch, "bear_trap") {
				t.Fatal("trap fixture not equipped")
			}
			front := monster.NewMonster3DFromConfig(target.X, g.camera.Y, "goblin", g.config)
			g.world.Monsters = append(g.world.Monsters, front)
			fp.press()
			tick()
			if target.RootFramesRemaining == 0 && target.RootTurnsRemaining == 0 {
				t.Fatal("adaptive trap did not land on selected diagonal")
			}
			if front.RootFramesRemaining > 0 || front.RootTurnsRemaining > 0 || len(g.slashEffects) > 0 || ch.SpellPoints >= 100 {
				t.Fatal("trap priority, target, or cost changed")
			}
		})
	}
}

func TestSaveLoadDiscardsPointerFlightAndHold(t *testing.T) {
	g, wm, _ := travelFixture(t)
	save := g.buildSave(wm)
	ih := NewInputHandler(g)
	g.gameLoop = &GameLoop{game: g, inputHandler: ih, ui: &UISystem{game: g}}
	ih.mouseAttackTarget = &monster.Monster3D{HitPoints: 1}
	ih.mouseAttackWorld = g.world
	g.mouseLeftClicks = []queuedClick{{x: 1, y: 1}}
	g.mouseRightClicks = []queuedClick{{x: 1, y: 1}}
	g.arrows = []Arrow{{Active: true, WorldAim: true}}
	g.magicProjectiles = []MagicProjectile{{Active: true, WorldAim: true}}
	if err := g.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
	if ih.mouseAttackTarget != nil || len(g.mouseLeftClicks)+len(g.mouseRightClicks)+len(g.arrows)+len(g.magicProjectiles) > 0 {
		t.Fatal("restored timeline retained pointer ownership or shots")
	}
}

func TestPointerGroundSpellsKeepAim(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, key := range []string{"stone_blossom", "firewall"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, key), func(t *testing.T) {
				g, _, fp, target, tick := mouseCombatHarness(t, tb)
				ts := float64(g.config.GetTileSize())
				g.camera.Angle = 0
				target.X, target.Y = g.camera.X, g.camera.Y+ts
				g.collisionSystem.UpdateEntity(target.ID, target.X, target.Y)
				ch := g.party.Members[0]
				ch.LearnSpell(spells.SpellID(key))
				ch.SpellPoints, ch.MaxSpellPoints = 500, 500
				ch.Equipment[items.SlotSpell] = items.Item{Type: items.ItemBattleSpell, SpellEffect: items.SpellEffect(key), SpellCost: 25}
				fp.press()
				tick()
				if key == "stone_blossom" {
					if len(g.pendingMortars) != 1 {
						t.Fatal("mortar was not launched")
					}
					m := g.pendingMortars[0]
					if math.Abs(m.X-g.camera.X) > 1e-6 || m.Y <= g.camera.Y {
						t.Fatal("mortar followed camera instead of aim")
					}
				} else {
					if len(g.persistentDamageZones) != 3 {
						t.Fatal("wall spell was not cast")
					}
					for _, cell := range g.persistentDamageZones {
						if cell.Y != g.camera.Y+2*ts || cell.AxisY != 0 {
							t.Fatal("wall spell followed camera instead of aim")
						}
					}
				}
				if g.camera.Angle != 0 || g.combat.partyAimTarget != nil {
					t.Fatal("ground spell mutated view or retained aim")
				}
			})
		}
	}
}
