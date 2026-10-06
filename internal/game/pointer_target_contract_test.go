package game

import (
	"fmt"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/game/keytracker"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/threading"
	"ugataima/internal/world"
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

func TestPointerProjectileKeepsLaunchAim(t *testing.T) {
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
					if math.Abs(p.VelX-p.VelY) > 1e-6 {
						t.Fatal("arrow lost selected world aim")
					}
					p.X, p.Y = target.X, target.Y
					g.collisionSystem.UpdateEntity(p.ID, p.X, p.Y)
				} else {
					if len(g.magicProjectiles) == 0 {
						t.Fatal("missing spell")
					}
					p := &g.magicProjectiles[0]
					if math.Abs(p.VelX-p.VelY) > 1e-6 {
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
	g.arrows = []Arrow{{Active: true}}
	g.magicProjectiles = []MagicProjectile{{Active: true}}
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
					def, err := spells.GetSpellDefinitionByID(spells.SpellID(key))
					if err != nil {
						t.Fatal(err)
					}
					if len(g.persistentDamageZones) != def.ZoneWidthTiles {
						t.Fatalf("wall spell laid %d cells, want its authored %d", len(g.persistentDamageZones), def.ZoneWidthTiles)
					}
					// The aim points south of an east-facing camera: the wall lies
					// across that axis, its authored distance ahead of the party.
					for _, cell := range g.persistentDamageZones {
						if math.Abs(cell.Y-(g.camera.Y+def.ZoneAheadTiles*ts)) > 1e-6 || cell.AxisY != 0 {
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

// The same trajectory must resolve identically through real keyboard and mouse
// inputs. Cover both clocks, weapon/spell delivery, maximum range, obstruction,
// and camera changes during flight. Projectiles are transient across save/load.
func TestProjectileInputParity(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, kind := range []string{"alien_blaster", "oak_staff", "firebolt", "fireball", "blowgun"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, kind), func(t *testing.T) {
				// Load content and allocate workers once per weapon/clock, not per
				// input permutation. Every cell restores its mutable gameplay state.
				g, ih, fp, m, tick := mouseCombatHarness(t, tb)
				g.threading = threading.NewThreadingComponents(g.config)
				t.Cleanup(g.threading.Shutdown)
				ts := float64(g.config.GetTileSize())
				g.camera.Angle = 0
				reach, volley, key := 6.0, 1, ebiten.KeyR
				if kind == "firebolt" || kind == "fireball" {
					def, _ := config.GetSpellDefinition(kind)
					reach, key = def.Physics.RangeTiles, ebiten.KeyF
				} else {
					def := lookupWeaponConfigByKey(kind)
					oldCrit, oldDisintegrate := def.CritChance, def.DisintegrateChance
					t.Cleanup(func() { def.CritChance, def.DisintegrateChance = oldCrit, oldDisintegrate })
					def.CritChance, def.DisintegrateChance = 0, 0
					reach, volley = float64(def.Range), max(1, def.Volley)
				}
				baseCamera := *g.camera
				newTarget := func(x, y float64) *monster.Monster3D {
					target := monster.NewMonster3DFromConfig(x, y, "goblin", g.config)
					target.HitPoints, target.MaxHitPoints, target.PerfectDodge = 100000, 100000, 0
					return target
				}
				floor := g.world.Tiles[10][13]
				for _, scene := range []string{"range edge", "turn away", "wall", "door", "out of range", "near", "flanks", "diagonal"} {
					for _, input := range []string{"direct key", "space", "mouse"} {
						t.Run(scene+"/"+input, func(t *testing.T) {
							// Keep loaded content, images and workers; construct fresh
							// runtime state, including locks and actor-owned maps/slices.
							for _, wrapper := range g.reusableProjectileWrappers {
								recycleProjectileWrapper(wrapper)
							}
							*g = MMGame{
								config: g.config, world: g.world, camera: g.camera,
								party:  character.NewParty(g.config),
								skyImg: g.skyImg, groundImg: g.groundImg,
								threading: g.threading, gameLoop: g.gameLoop,
								sessionStartTime: g.sessionStartTime,
								appScreen:        AppScreenInGame, turnBasedMode: tb,
							}
							*g.camera = baseCamera
							*ih = InputHandler{game: g, keys: keytracker.NewWithSource(heldOnly()), heldKeys: heldOnly()}
							*g.gameLoop.ui = UISystem{game: g}
							g.combat = NewCombatSystem(g)
							stripNewClassSkillsForLegacyFixtures(g.party)
							for _, hero := range g.party.Members {
								hero.HitPoints, hero.ActionsRemaining = hero.MaxHitPoints, 10
								delete(hero.Equipment, items.SlotSpell)
								hero.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
							}
							ch := g.party.Members[0]
							ch.Luck = 0
							ch.Skills[character.SkillBallistics] = &character.Skill{}
							if kind == "firebolt" || kind == "fireball" {
								ch.LearnSpell(spells.SpellID(kind))
								ch.SpellPoints, ch.MaxSpellPoints = 100, 100
								ch.Equipment[items.SlotSpell] = items.Item{Type: items.ItemBattleSpell, SpellEffect: items.SpellEffect(kind), SpellCost: 2}
							} else {
								ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(kind)
							}
							g.world.Tiles[10][13] = floor
							g.world.Monsters = []*monster.Monster3D{m}
							g.collisionSystem = collision.NewCollisionSystem(g.world, ts)
							g.collisionSystem.RegisterEntity(collision.NewEntity("player", g.camera.X, g.camera.Y, ts*.5, ts*.5, collision.CollisionTypePlayer, false))
							fp.idle()
							distance := reach
							if scene == "out of range" {
								distance += 2
							}
							if scene == "near" {
								distance = 2
							}
							if scene == "flanks" {
								distance = 4
							}
							if scene == "diagonal" {
								distance = 1
							}
							*m = *newTarget(g.camera.X+distance*ts, g.camera.Y)
							if scene == "diagonal" {
								m.Y += ts
							}
							if scene == "flanks" {
								for i, offset := range [][2]float64{{-.4, 0}, {0, -.7}, {0, .7}} {
									bystander := newTarget(g.camera.X+offset[0]*ts, g.camera.Y+offset[1]*ts)
									bystander.ID = fmt.Sprintf("flank-%d", i)
									g.world.Monsters = append(g.world.Monsters, bystander)
								}
							}
							g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
							if input == "mouse" {
								fp.moveTo(320, 220)
								fp.press()
							} else {
								fp.moveTo(5, 5)
								pressed := key
								if input == "space" {
									pressed = ebiten.KeySpace
								}
								ih.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == pressed })
							}
							tick()
							if len(g.arrows)+len(g.magicProjectiles) != volley {
								t.Fatal("input did not launch the authored volley")
							}
							if tb && ch.ActionsRemaining != 9 {
								t.Fatalf("attack cost %d actions, want 1", 10-ch.ActionsRemaining)
							}
							if !tb && ch.RTCooldown <= 0 {
								t.Fatal("attack omitted hero cooldown")
							}
							g.combat.CheckProjectileMonsterCollisions()
							for _, target := range g.world.Monsters {
								if target.HitPoints != target.MaxHitPoints {
									t.Fatal("shot hit before leaving the muzzle")
								}
							}
							if scene == "diagonal" && tb {
								var launch projectileLaunch
								var vx, vy float64
								if len(g.arrows) > 0 {
									launch = g.arrows[0].Launch
									vx, vy = g.arrows[0].VelX, g.arrows[0].VelY
								} else {
									launch = g.magicProjectiles[0].Launch
									vx, vy = g.magicProjectiles[0].VelX, g.magicProjectiles[0].VelY
								}
								x, y, rvx, rvy := launch.renderMotion(g.combat, m.X, m.Y, vx, vy)
								_, wantX, wantY, pulled, ok := g.combat.pulledFrontSlot(m)
								if !ok || !pulled || math.Hypot(x-wantX, y-wantY) > 1e-6 || math.Abs((wantX-g.camera.X)*rvy-(wantY-g.camera.Y)*rvx) > 1e-6 {
									t.Fatalf("flight presentation misses pulled sprite: launch=%+v end=(%g,%g) want=(%g,%g) pulled=%v ok=%v", launch, x, y, wantX, wantY, pulled, ok)
								}
								for _, heading := range []float64{math.Pi / 2, math.Pi} {
									g.camera.Angle = heading
									x, y, _, _ = launch.renderMotion(g.combat, m.X, m.Y, vx, vy)
									wantX, wantY = g.combat.monsterVisualPos(m)
									if math.Hypot(x-wantX, y-wantY) > 1e-6 {
										t.Fatal("camera turn separated flight from monster presentation")
									}
								}
								g.camera.Angle = 0
								g.turnBasedMode = false
								x, y, _, _ = launch.renderMotion(g.combat, m.X, m.Y, vx, vy)
								if math.Hypot(x-m.X, y-m.Y) > 1e-6 {
									t.Fatal("RT presentation retained a pulled TB slot")
								}
								g.turnBasedMode = tb
								for _, change := range []string{"moved", "dead"} {
									beforeY, beforeHP := m.Y, m.HitPoints
									if change == "moved" {
										m.Y += ts
									} else {
										m.HitPoints = 0
									}
									x, y, _, _ = launch.renderMotion(g.combat, launch.aimX, launch.aimY, vx, vy)
									if math.Hypot(x-launch.aimX, y-launch.aimY) > 1e-6 {
										t.Fatalf("%s target redirected flight presentation", change)
									}
									m.Y, m.HitPoints = beforeY, beforeHP
								}
							}
							if scene == "turn away" {
								g.camera.Angle = math.Pi
								g.camera.X -= ts
							}
							if scene == "wall" {
								g.world.Tiles[10][13] = world.TileWall
							}
							if scene == "door" {
								g.collisionSystem.RegisterEntity(collision.NewSightBlockingEntity("closed-door", 13.5*ts, 10.5*ts, ts*.9, ts*.9, collision.CollisionTypeNPC, true))
							}
							hits, hp := 0, m.HitPoints
							for n := 0; n < 600 && g.gameLoop.hasActiveProjectiles(); n++ {
								g.gameLoop.updateProjectilesAndImpacts()
								if m.HitPoints < hp {
									hits, hp = hits+1, m.HitPoints
								}
							}
							wantHit := scene != "wall" && scene != "door" && scene != "out of range" && !(scene == "diagonal" && !tb && input != "mouse")
							if got := m.HitPoints < m.MaxHitPoints; got != wantHit {
								t.Fatalf("hit=%v want %v; HP=%d", got, wantHit, m.HitPoints)
							}
							// Trailing volley darts share the lead dart's range plane.
							if scene == "range edge" && hits != volley {
								t.Fatalf("range-edge hits=%d want the whole volley of %d", hits, volley)
							}
							if scene == "flanks" {
								for _, bystander := range g.world.Monsters[1:] {
									if bystander.HitPoints != bystander.MaxHitPoints {
										t.Fatal("shot acquired a flank or rear target")
									}
								}
							}
							if g.gameLoop.hasActiveProjectiles() {
								t.Fatal("projectile never retired")
							}
						})
					}
				}
			})
		}
	}
}

// One gesture follows moving actors and party strafing, survives a lost frame,
// and retargets in a crowd. Assert actual launch aim and the matching highlight.
func TestMouseHoldTracksMovingProjectileTarget(t *testing.T) {
	for _, kind := range []string{"alien_blaster", "firebolt"} {
		for _, count := range []int{1, 8} {
			t.Run(fmt.Sprintf("%s/crowd=%d", kind, count), func(t *testing.T) {
				g, ih, fp, target, tick := mouseCombatHarness(t, false)
				ts := float64(g.config.GetTileSize())
				g.camera.Angle = 0
				target.X, target.Y = g.camera.X+3*ts, g.camera.Y
				for i := 1; i < count; i++ {
					m := monster.NewMonster3DFromConfig(target.X+ts, target.Y+float64(i)*ts*.15, "goblin", g.config)
					g.world.Monsters = append(g.world.Monsters, m)
				}
				g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
				for _, ch := range g.party.Members {
					ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("alien_blaster")
					if kind == "firebolt" {
						ch.LearnSpell("firebolt")
						ch.Equipment[items.SlotSpell] = items.Item{Type: items.ItemBattleSpell, SpellEffect: "firebolt", SpellCost: 2}
						ch.SpellPoints, ch.MaxSpellPoints = 1000, 1000
					}
				}
				r := g.gameLoop.renderer
				visible := func(m *monster.Monster3D, left float64) {
					r.beginMonsterPickFrame()
					r.monsterPick.hits = []monsterPickHit{{monster: m, left: left, top: 150, size: 140, depth: 3 * ts}}
				}
				shots := func() int { return len(g.arrows) + len(g.magicProjectiles) }
				awaitShot := func(want *monster.Monster3D) {
					t.Helper()
					before := shots()
					for i := 0; i < 600 && shots() == before; i++ {
						tick()
					}
					if shots() != before+1 {
						t.Fatal("held gesture did not fire")
					}
					var launch projectileLaunch
					var vx, vy float64
					if kind == "firebolt" {
						if len(g.magicProjectiles) == 0 {
							t.Fatal("smart action did not cast the equipped spell")
						}
						p := g.magicProjectiles[len(g.magicProjectiles)-1]
						launch, vx, vy = p.Launch, p.VelX, p.VelY
					} else {
						p := g.arrows[len(g.arrows)-1]
						launch, vx, vy = p.Launch, p.VelX, p.VelY
					}
					angle := math.Atan2(want.Y-g.camera.Y, want.X-g.camera.X)
					if launch.target != want || math.Abs(math.Remainder(math.Atan2(vy, vx)-angle, 2*math.Pi)) > 1e-6 {
						t.Fatal("held shot did not follow the selected actor's current position")
					}
					r.selectMonsterHover()
					if r.hoveredMonster != want {
						t.Fatal("highlight and fired target disagree")
					}
				}
				visible(target, 250)
				fp.press()
				tick()
				fp.hold()
				fp.moveTo(50, 50)
				if shots() != 1 {
					t.Fatal("initial press did not fire")
				}
				// Move the actor away from its original cursor pixel and strafe via the
				// actual input movement path. The acquired actor must remain highlighted.
				target.Y += ts * .4
				g.collisionSystem.UpdateEntity(target.ID, target.X, target.Y)
				ih.strafeRight()
				visible(target, 410)
				awaitShot(target)
				// Lost visibility pauses, without requiring another mouse-down edge.
				r.beginMonsterPickFrame()
				before := shots()
				for range 120 {
					tick()
				}
				r.selectMonsterHover()
				if shots() != before || r.hoveredMonster != nil || ih.mouseAttackWorld != g.world {
					t.Fatal("lost focus did not pause fire while preserving the gesture")
				}
				visible(target, 410)
				awaitShot(target)
				if count > 1 {
					next := g.world.Monsters[count-1]
					r.monsterPick.hits = append(r.monsterPick.hits, monsterPickHit{monster: next, left: 200, top: 150, size: 140, depth: 2 * ts})
					fp.moveTo(270, 220)
					awaitShot(next)
					fp.moveTo(480, 220)
					awaitShot(target)
				}
				fp.release()
				tick()
				if ih.mouseAttackWorld != nil {
					t.Fatal("release retained the gesture")
				}
			})
		}
	}
}
