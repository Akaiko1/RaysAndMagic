package game

import (
	"bytes"
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/threading/entities"
)

func TestSolsticeWeaponAbilitiesThroughDirectHits(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, key := range []string{"solstice_thermal_lance", "solstice_anchor_hammer", "solstice_transfer_blade"} {
			t.Run(fmt.Sprintf("%s/TB=%v", key, tb), func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				g := cs.game
				g.turnBasedMode = tb
				def, _ := config.GetWeaponDefinition(key)
				a := def.ElementalAbility
				hero := g.party.Members[0]
				hero.BuffBonuses.Speed = 20
				hero.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(key)
				m, n := partyDamageTargets(g, 64)
				hit := func(target *monster.Monster3D) int {
					before := target.HitPoints
					cs.ApplyDamageToMonster(target, 100, def.Name, false)
					return before - target.HitPoints
				}
				base := hit(m)
				if base <= 0 || m.ElementalMarks[a.Kind].Count != 1 {
					t.Fatalf("first hit=%d marks=%v", base, m.ElementalMarks)
				}
				// Another target owns a separate ledger; returning to m resumes it.
				if got := hit(n); got != base || m.ElementalMarks[a.Kind].Count != 1 {
					t.Fatalf("target switch changed buildup: hit=%d marks=%v", got, m.ElementalMarks)
				}
				if a.Kind == "anchor" {
					normal := base - a.Damage
					for hitNumber := 2; hitNumber <= a.MaxStacks+2; hitNumber++ {
						want := normal + a.Damage*min(hitNumber, a.MaxStacks)
						if got := hit(m); got != want {
							t.Fatalf("stack %d: damage=%d want=%d", hitNumber, got, want)
						}
					}
				} else {
					if got := hit(m); got != base {
						t.Fatalf("premature release %d", got)
					}
					bonus := a.Damage
					if a.Kind == "pressure" {
						_, _, _, _, _, speed, _ := hero.GetEffectiveStats()
						bonus += speed / a.StatDivisor
					}
					if got := hit(m); got != base+bonus || len(m.ElementalMarks) != 0 {
						t.Fatalf("release=%d want=%d marks=%v", got, base+bonus, m.ElementalMarks)
					}
				}
				// Both clocks expire the same stack before the next hit.
				for len(m.ElementalMarks) > 0 {
					m.TickElementalMarks(tb)
				}
				if got := hit(m); got != base || m.ElementalMarks[a.Kind].Count != 1 {
					t.Fatalf("expired buildup persisted: hit=%d marks=%v", got, m.ElementalMarks)
				}
				for _, gate := range []string{"dodge", "seal"} {
					m.ElementalMarks = nil
					m.PerfectDodge, m.BossDormant = 0, false
					if gate == "dodge" {
						m.PerfectDodge = 100
					} else {
						m.BossDormant = true
					}
					before := m.HitPoints
					hit(m)
					if len(m.ElementalMarks) != 0 || m.HitPoints != before {
						t.Fatalf("%s charged ability or dealt damage", gate)
					}
				}
			})
		}
	}
}

type blockedBackwashTiles struct{ testTileChecker }

func (*blockedBackwashTiles) IsTileOpaque(x, y int) bool { return x == 2 && y == 1 }

// Fire through the production launch path, then deterministically isolate the
// ordinary damage from random criticals. The ability snapshot remains untouched.
func fireFlowstaff(t *testing.T, cs *CombatSystem, target *monster.Monster3D) int {
	t.Helper()
	hero := cs.game.party.Members[0]
	hero.BuffBonuses.Intellect = 20
	hero.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("solstice_flowstaff")
	if !cs.createArrowAttack(100, items.SlotMainHand, "") {
		t.Fatal("flowstaff failed to launch")
	}
	index := len(cs.game.arrows) - 1
	shot := cs.game.arrows[index]
	shot.Damage, shot.Crit = 100, false
	_, intellect, _, _, _, _, _ := hero.GetEffectiveStats()
	def, _ := config.GetWeaponDefinition("solstice_flowstaff")
	want := def.ElementalAbility.Damage + intellect/def.ElementalAbility.StatDivisor
	if shot.ElementalAbilityDamage != want {
		t.Fatalf("launch scaling=%d want=%d", shot.ElementalAbilityDamage, want)
	}
	// A launch snapshot cannot read the new stat at impact.
	old := hero.Intellect
	hero.Intellect += 300
	cs.applyProjectileDamage(&shot, "arrow", target, shot.ID)
	hero.Intellect = old
	cs.game.arrows[index] = shot
	return want
}

func TestBackwashChainThroughProjectileCollisions(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, scenario := range []string{"pair", "pair_twice", "three", "alone", "exact_range", "wall", "bound", "summon", "charmed", "sealed", "dead", "dodge_primary", "sealed_primary", "lost_range", "killed_in_flight", "lethal_primary", "lethal_secondary"} {
			t.Run(fmt.Sprintf("%s/TB=%v", scenario, tb), func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				g := cs.game
				g.turnBasedMode = tb
				m, n := partyDamageTargets(g, 64)
				m.X, m.Y, n.X, n.Y = 112, 96, 208, 96
				switch scenario {
				case "three":
					third := mkTestMonster("Third", 10000)
					third.ID, third.X, third.Y = "third", 304, 96
					third.ArmorClass, third.PerfectDodge, third.Resistances = 0, 0, nil
					g.world.Monsters = append(g.world.Monsters, third)
				case "alone":
					g.world.Monsters = g.world.Monsters[:1]
				case "exact_range":
					n.X = m.X + 128
				case "wall":
					g.collisionSystem = collision.NewCollisionSystem(&blockedBackwashTiles{testTileChecker{100, 100}}, 64)
				case "bound":
					n.Bound = true
				case "summon":
					n.SummonedBy = spellSummonOwnerPrefix + "test"
				case "charmed":
					n.Pacified = true
				case "sealed":
					n.BossDormant = true
				case "dead":
					n.HitPoints = 0
				case "dodge_primary":
					m.PerfectDodge = 100
				case "sealed_primary":
					m.BossDormant = true
				case "lethal_primary":
					m.HitPoints = 1
				case "lethal_secondary":
					n.HitPoints = 1
				}
				before := make(map[string]int)
				for _, target := range g.world.Monsters {
					before[target.ID] = target.HitPoints
					g.collisionSystem.RegisterEntity(collision.NewEntity(target.ID, target.X, target.Y, 24, 24, collision.CollisionTypeMonster, false))
				}
				bonus := fireFlowstaff(t, cs, m)
				if scenario == "pair_twice" {
					fireFlowstaff(t, cs, m)
				}
				// Continuations retain the shooter's launch payload too.
				g.party.Members[0].Intellect += 300
				delete(g.party.Members[0].Equipment, items.SlotMainHand)
				g.selectedChar = 1
				if scenario == "lost_range" {
					n.X += 500
				}
				if scenario == "killed_in_flight" {
					n.HitPoints = 0
					before[n.ID] = 0
				}
				updater := entities.NewEntityUpdaterWithWorkers(2)
				t.Cleanup(updater.Stop)
				// Run actual movement, homing and collision phases, including the
				// append-safe snapshots used when a hit spawns the next charge.
				for tick := 0; ; tick++ {
					active := false
					for i := range g.arrows {
						if g.arrows[i].Active && g.arrows[i].LifeTime > 0 && g.arrows[i].Backwash != nil {
							active = true
						}
					}
					if !active {
						break
					}
					if tick > 5*g.config.GetTPS() {
						t.Fatal("chain did not terminate")
					}
					cs.CheckProjectileMonsterCollisions()
					updater.UpdateProjectilesParallel(g.ConvertProjectilesToWrappers(), func(float64, float64) bool { return true })
				}
				legs := len(g.arrows) - 1
				for _, target := range g.world.Monsters {
					want := 0
					if target == m {
						want = 100 + bonus
						if scenario == "pair" || scenario == "three" || scenario == "lethal_secondary" {
							want += bonus
						}
						if scenario == "dodge_primary" || scenario == "sealed_primary" {
							want = 0
						}
						if scenario == "lethal_primary" {
							want = 1
						}
					} else if scenario == "pair" || scenario == "three" {
						want = 2 * bonus
						if target.ID == "third" {
							want = bonus // Both possible return targets have exhausted their cap.
						}
					} else if scenario == "lethal_primary" {
						want = bonus
					} else if scenario == "lethal_secondary" {
						want = 1
					}
					if scenario == "pair_twice" {
						want = 4 * bonus
						if target == m {
							want += 200
						}
					}
					if got := before[target.ID] - target.HitPoints; got != want {
						t.Fatalf("%s damage=%d want=%d legs=%d", target.ID, got, want, legs)
					}
					if len(target.ElementalMarks) != 0 {
						t.Fatal("backwash left a persistent mark")
					}
				}
			})
		}
	}
}

func TestSolsticeWeaponsReachEditorPreview(t *testing.T) {
	cfg := setupPreviewSandboxTest(t)
	p, err := NewFxPreview(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.g.Shutdown()
	for _, key := range []string{"solstice_thermal_lance", "solstice_anchor_hammer", "solstice_transfer_blade", "solstice_flowstaff"} {
		found := false
		for _, entry := range p.Items() {
			if entry.Kind == FxWeapon && entry.Key == key {
				found = true
				p.Select(entry)
				def, _ := config.GetWeaponDefinition(key)
				if def.Melee != nil {
					if len(p.g.slashEffects) != 1 || p.g.slashEffects[0].Style != def.Graphics.SlashFx {
						t.Fatalf("%s missing styled swing", key)
					}
				} else if len(p.g.arrows) != 1 || p.g.arrows[0].BowKey != key {
					t.Fatal("missing flowstaff projectile")
				}
				break
			}
		}
		if !found {
			t.Fatal(key)
		}
	}
}

func TestSolsticeWeaponEffectsAnimate(t *testing.T) {
	if os.Getenv("RAM_DEBUG_SIM") == "" {
		t.Skip("live GPU")
	}
	g := &MMGame{config: &config.Config{}}
	r := &Renderer{game: g, whiteImg: ebiten.NewImage(1, 1)}
	r.whiteImg.Fill(color.White)
	defer r.whiteImg.Deallocate()
	for _, style := range []string{"solstice_thermal", "solstice_anchor", "solstice_transfer", "solstice_flow_side", "solstice_flow_head"} {
		for _, crit := range []bool{false, true} {
			var previous []byte
			for _, frame := range []int{4, 16, 30, 44} {
				runOnDrawFrame(func(_ *ebiten.Image) {
					dst := ebiten.NewImage(640, 360)
					defer dst.Deallocate()
					g.frameCount = int64(frame)
					if style == "solstice_flow_side" || style == "solstice_flow_head" {
						boost := 1.
						if crit {
							boost = 1.3
						}
						if style == "solstice_flow_side" {
							r.drawWeaponProjectileFx("solstice_flow", dst, 320, 180, 20, 1, 0, boost, 1)
						} else {
							r.drawWeaponProjectileFxHeadOn("solstice_flow", dst, 320, 180, 20, boost, 1)
						}
					} else {
						r.drawMeleeParticles(dst, SlashEffect{ID: "check", Style: style, AnimationFrame: frame, MaxFrames: 44, Crit: crit}, 320, 180, 360)
					}
					pixels := make([]byte, 640*360*4)
					dst.ReadPixels(pixels)
					if dir := os.Getenv("RAM_SOLSTICE_FX_GALLERY"); dir != "" {
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Error(err)
							return
						}
						f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-crit%v-%02d.png", style, crit, frame)))
						if err != nil {
							t.Error(err)
							return
						}
						img := image.NewRGBA(image.Rect(0, 0, 640, 360))
						capture := ebiten.NewImage(640, 360)
						capture.Fill(color.RGBA{16, 20, 24, 255})
						capture.DrawImage(dst, nil)
						capture.ReadPixels(img.Pix)
						capture.Deallocate()
						if err := png.Encode(f, img); err != nil {
							t.Error(err)
						}
						if err := f.Close(); err != nil {
							t.Error(err)
						}
					}
					lit := 0
					for i := 3; i < len(pixels); i += 4 {
						if pixels[i] > 0 {
							lit++
						}
					}
					expired := frame == 44 && style != "solstice_flow_side" && style != "solstice_flow_head"
					if expired && lit != 0 || !expired && lit < 20 {
						t.Errorf("%s crit=%v frame=%d pixels=%d", style, crit, frame, lit)
					}
					if previous != nil && bytes.Equal(previous, pixels) {
						t.Errorf("%s does not animate", style)
					}
					previous = pixels
				})
			}
		}
	}
}

func TestElementalMarksShareModeClock(t *testing.T) {
	for _, tps := range []int{30, 60, 120} {
		m := &monster.Monster3D{}
		m.SetElementalMark("pressure", monster.ElementalWeaponMark{Count: 2}, 6*tps, 3)
		for i := 0; i < tps; i++ {
			m.TickElementalMarks(false)
		}
		m.TickElementalMarks(true)
		mark := m.ElementalMarks["pressure"]
		if mark.Frames != 3*tps || mark.Turns != 2 || mark.Count != 2 {
			t.Fatalf("spent time refunded: %+v", mark)
		}
		restored := &monster.Monster3D{}
		restored.RestoreElementalMarks(m.ElementalMarks)
		restored.TickElementalMarks(true)
		if m.ElementalMarks["pressure"].Frames != 3*tps {
			t.Fatal("restore aliases source")
		}
		for i := 0; i < tps; i++ {
			restored.TickElementalMarks(false)
		}
		if len(restored.ElementalMarks) != 0 {
			t.Fatal("expired mark remains")
		}
	}
}
