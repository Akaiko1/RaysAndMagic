package game

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

// Case table: RT/TB x main/off x both melee proc weapons/ranged proc weapon
// x weapon/card/both/neither x landed/undead/dragon/warded/sealed/dodged/summon.
// Proc fixtures use 100% to prove wiring without probabilistic failures. The
// separate launch test checks the unchanged authored percentages and snapshots.
func TestWeaponDisintegrateCombatContract(t *testing.T) {
	for _, key := range []string{"kage_kunai", "serpent_fang", "alien_blaster"} {
		for _, tb := range []bool{false, true} {
			for _, off := range []bool{false, true} {
				for _, source := range []string{"weapon", "card", "both", "neither"} {
					for _, targetState := range []string{"landed", "undead", "dragon", "warded", "sealed", "dodged", "summon"} {
						t.Run(fmt.Sprintf("%s/TB=%v/off=%v/%s/%s", key, tb, off, source, targetState), func(t *testing.T) {
							cs := newTestCombatSystemWithConfig(t)
							g := cs.game
							g.turnBasedMode = tb
							def, _ := config.GetWeaponDefinition(key)
							oldChance := def.DisintegrateChance
							t.Cleanup(func() { def.DisintegrateChance = oldChance })
							def.DisintegrateChance = 0
							if source == "weapon" || source == "both" {
								def.DisintegrateChance = 1
							}
							if source == "card" || source == "both" {
								card, _ := config.GetItemDefinition("alien_card")
								old := card.CardDisintegratePct
								t.Cleanup(func() { card.CardDisintegratePct = old })
								card.CardDisintegratePct = 100
								g.cardSlots[0].key = "alien_card"
							}
							ch := g.party.Members[0]
							ch.Equipment = map[items.EquipSlot]items.Item{}
							ch.Skills = map[character.SkillType]*character.Skill{}
							for _, st := range character.AllSkills {
								ch.Skills[st] = &character.Skill{Mastery: character.MasteryNovice}
							}
							delete(ch.Skills, character.SkillSpiritualTraining)
							slot := items.SlotMainHand
							if off {
								slot = items.SlotOffHand
								ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
								ch.RTCooldown = 100
								ch.NextTBAttackOffHand = true
							}
							if _, _, ok := ch.EquipItemToSlot(items.CreateWeaponFromYAML(key), slot); !ok {
								t.Fatal("equip failed")
							}
							g.world.Width, g.world.Height = 8, 8
							g.world.Tiles = make([][]world.TileType3D, 8)
							for y := range g.world.Tiles {
								g.world.Tiles[y] = make([]world.TileType3D, 8)
							}
							g.camera.X, g.camera.Y = 96, 96
							m := mkTestMonster("Proc target", 1000000)
							m.ID = "proc-target"
							m.X, m.Y = 160, 96
							switch targetState {
							case "undead", "dragon":
								m.MonsterType = targetState
							case "warded":
								m.BossWarded = true
							case "sealed":
								m.BossDormant = true
							case "dodged":
								m.PerfectDodge = 100
							case "summon":
								m.SummonedBy = spellSummonOwnerPrefix + "test"
							}
							g.world.Monsters = []*monsterPkg.Monster3D{m}
							g.collisionSystem.RegisterEntity(collision.NewEntity(m.ID, m.X, m.Y, 32, 32, collision.CollisionTypeMonster, false))
							if !cs.EquipmentMeleeAttack() {
								t.Fatal("attack refused")
							}
							if def.Range > 3 {
								if len(g.arrows) != 1 {
									t.Fatalf("arrows=%d, want 1", len(g.arrows))
								}
								g.arrows[0].X, g.arrows[0].Y = m.X, m.Y
								g.collisionSystem.UpdateEntity(g.arrows[0].ID, m.X, m.Y)
								cs.CheckProjectileMonsterCollisions()
							}
							wantDead := targetState == "landed" && source != "neither"
							if dead := !m.IsAlive(); dead != wantDead {
								t.Fatalf("dead=%v want %v; HP=%d", dead, wantDead, m.HitPoints)
							}
						})
					}
				}
			}
		}
	}
}

func TestWeaponDisintegrateLaunchSnapshot(t *testing.T) {
	for _, key := range []string{"alien_blaster", "hunting_bow"} {
		for _, off := range []bool{false, true} {
			for _, card := range []bool{false, true} {
				for _, bonus := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/off=%v/card=%v/bonus=%v", key, off, card, bonus), func(t *testing.T) {
						cs := newTestCombatSystemWithConfig(t)
						g := cs.game
						ch := g.party.Members[0]
						slot := items.SlotMainHand
						if off {
							slot = items.SlotOffHand
						}
						ch.Equipment[slot] = items.CreateWeaponFromYAML(key)
						if card {
							g.cardSlots[0].key = "alien_card"
						}
						def, _ := config.GetWeaponDefinition(key)
						want := 0.0
						if !bonus {
							want = def.DisintegrateChance
						}
						if card {
							d, _ := config.GetItemDefinition("alien_card")
							want += float64(d.CardDisintegratePct) / 100
						}
						label := ""
						if bonus {
							label = "Bonus Bolt"
						}
						if !cs.createArrowAttack(1, slot, label) {
							t.Fatal("launch failed")
						}
						if got := g.arrows[0].DisintegrateChance; math.Abs(got-want) > 1e-9 {
							t.Fatalf("snapshot chance=%v want %v", got, want)
						}
						delete(ch.Equipment, slot)
						g.cardSlots = [MaxCardSlots]cardSlot{}
						if g.arrows[0].DisintegrateChance != want {
							t.Fatal("in-flight chance changed with equipment/cards")
						}
						g.clearTransientCombatState()
						if len(g.arrows) != 0 {
							t.Fatal("load/transition reset retained proc projectiles")
						}
					})
				}
			}
		}
	}
}

// Continuations are autonomous in either mode and at every camera angle. They
// must skip the previous victim, require a real overlap and never use TB assist.
func TestWeaponContinuationCollisionContract(t *testing.T) {
	for _, key := range []string{"nest_arbalest", "arbalest", "longlance_rifle"} {
		for _, tb := range []bool{false, true} {
			for _, angle := range []float64{0, math.Pi / 2, math.Pi} {
				t.Run(fmt.Sprintf("%s/TB=%v/angle=%.2f", key, tb, angle), func(t *testing.T) {
					cs := newTestCombatSystemWithConfig(t)
					g := cs.game
					g.turnBasedMode = tb
					g.camera.X, g.camera.Y = 96, 96
					g.camera.Angle = angle
					first := mkTestMonster("First", 1000)
					first.ID = "first"
					first.X, first.Y = 160, 96
					next := mkTestMonster("Next", 1000)
					next.ID = "next"
					next.X, next.Y = 224, 96
					g.world.Monsters = []*monsterPkg.Monster3D{first, next}
					for _, m := range g.world.Monsters {
						g.collisionSystem.RegisterEntity(collision.NewEntity(m.ID, m.X, m.Y, 32, 32, collision.CollisionTypeMonster, false))
					}
					def, _ := config.GetWeaponDefinition(key)
					ar := Arrow{ID: "origin", Active: true, LifeTime: 100, Damage: 10, Owner: ProjectileOwnerPlayer, BowKey: key, DamageType: "physical", X: first.X, Y: first.Y, VelX: 8, PierceLeft: def.PierceCount, RicochetLeft: def.RicochetTargets}
					cs.applyProjectileDamage(&ar, "arrow", first, ar.ID)
					if len(g.arrows) != 1 {
						t.Fatalf("continuations=%d want 1", len(g.arrows))
					}
					hp := first.HitPoints
					cs.CheckProjectileMonsterCollisions()
					if first.HitPoints != hp || next.HitPoints != 1000 || !g.arrows[0].Active {
						t.Fatal("continuation re-hit previous victim or hit without overlap")
					}
					g.arrows[0].X, g.arrows[0].Y = next.X, next.Y
					g.collisionSystem.UpdateEntity(g.arrows[0].ID, next.X, next.Y)
					next.SummonedBy = spellSummonOwnerPrefix + "test"
					cs.CheckProjectileMonsterCollisions()
					if next.HitPoints != 1000 || !g.arrows[0].Active {
						t.Fatal("continuation hit a pure party summon")
					}
					next.SummonedBy = ""
					cs.CheckProjectileMonsterCollisions()
					if next.HitPoints != 990 || g.arrows[0].Active {
						t.Fatalf("HP=%d active=%v, want 990 and consumed", next.HitPoints, g.arrows[0].Active)
					}
					cs.CheckProjectileMonsterCollisions()
					if next.HitPoints != 990 {
						t.Fatal("consumed continuation hit twice")
					}
				})
			}
		}
	}
}

// Name, exact key and family type are equivalent selectors for both weapons and
// cards. Repeated identity fields must not multiply the same entry repeatedly.
func TestWeaponAndCardBonusTargetContract(t *testing.T) {
	identities := []struct {
		name, key, kind string
		matches         bool
	}{
		{"Dragon", "variant", "other", true}, {"Other", "dragon", "other", true},
		{"Elder Dragon", "elder_dragon", "dragon", true}, {"Dragon", "dragon", "dragon", true},
		{"Other", "other", "DrAgOn", true}, {"Other", "other", "formless", false},
	}
	for _, source := range []string{"weapon", "card", "both"} {
		for _, path := range []string{"melee", "projectile", "splash"} {
			for i, identity := range identities {
				t.Run(fmt.Sprintf("%s/%s/identity=%d", source, path, i), func(t *testing.T) {
					cs := newTestCombatSystemWithConfig(t)
					g := cs.game
					g.party.Members[0].Skills = map[character.SkillType]*character.Skill{}
					def, _ := config.GetWeaponDefinition("elven_bow")
					old := def.BonusVs
					t.Cleanup(func() { def.BonusVs = old })
					def.BonusVs = nil
					if source != "card" {
						def.BonusVs = map[string]float64{"dragon": 1.5}
					}
					if source != "weapon" {
						g.cardSlots[0].key = "elf_archer_card"
					}
					m := mkTestMonster(identity.name, 1000)
					m.ID = "bonus-target"
					m.Key = identity.key
					m.MonsterType = identity.kind
					m.X = 64
					g.world.Monsters = []*monsterPkg.Monster3D{m}
					want := 200
					if identity.matches {
						if source != "card" {
							want = 300
						}
						if source != "weapon" {
							want = int(float64(want) * 1.25)
						}
					}
					switch path {
					case "melee":
						cs.ApplyDamageToMonster(m, 200, def.Name, false)
					case "projectile":
						ar := Arrow{ID: "bonus-arrow", Active: true, LifeTime: 100, Damage: 200, Owner: ProjectileOwnerPlayer, BowKey: "elven_bow", DamageType: "physical"}
						cs.applyProjectileDamage(&ar, "arrow", m, ar.ID)
					case "splash":
						center := mkTestMonster("Center", 1000)
						attack := cs.newPartyMonsterAttack(200, 0, "physical", 0, def, def.Name, true, false, false)
						cs.applyAoeSplash(center, attack, 2)
					}
					if got := 1000 - m.HitPoints; got != want {
						t.Fatalf("damage=%d want %d", got, want)
					}
				})
			}
		}
	}
}

func TestElvenBowAllAuthoredDragonFamilies(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	for _, key := range []string{"dragon", "dragon_red", "dragon_green", "dragon_gold", "elder_dragon", "elder_dragon_red", "elder_dragon_green", "elder_dragon_gold"} {
		t.Run(key, func(t *testing.T) {
			m := monsterPkg.NewMonster3DFromConfig(128, 0, key, cs.game.config)
			if m.MonsterType != "dragon" {
				t.Fatalf("authored type=%q, want dragon", m.MonsterType)
			}
			m.MaxHitPoints, m.HitPoints = 100000, 100000
			m.PerfectDodge = 0
			m.ArmorClass = 0 // Isolate family matching from randomized ranged armor bypass.
			cs.game.world.Monsters = []*monsterPkg.Monster3D{m}
			def, _ := config.GetWeaponDefinition("elven_bow")
			original := def.BonusVs
			defer func() { def.BonusVs = original }()
			hit := func() int {
				m.HitPoints = m.MaxHitPoints
				ar := Arrow{ID: "dragon-bonus", Active: true, LifeTime: 100, Damage: 10000, Owner: ProjectileOwnerPlayer, BowKey: "elven_bow", DamageType: "physical"}
				cs.applyProjectileDamage(&ar, "arrow", m, ar.ID)
				return m.MaxHitPoints - m.HitPoints
			}
			withBonus := hit()
			def.BonusVs = nil
			withoutBonus := hit()
			if withBonus <= withoutBonus {
				t.Fatalf("bonus damage=%d baseline=%d", withBonus, withoutBonus)
			}
		})
	}
}

func TestStaffCooldownMainHandPresentationContract(t *testing.T) {
	for _, key := range []string{"archmage_staff", "lanista_scepter", "verdant_eye_scepter"} {
		for _, where := range []string{"main", "off", "bag"} {
			t.Run(key+"/"+where, func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				ch := gmReferenceChar(cs.game.config)
				ch.Equipment = map[items.EquipSlot]items.Item{}
				it := items.CreateWeaponFromYAML(key)
				base := cs.SpellCooldownFrames(ch, "firebolt")
				if where == "main" {
					ch.Equipment[items.SlotMainHand] = it
				}
				if where == "off" {
					ch.Equipment[items.SlotOffHand] = it
				}
				got := cs.SpellCooldownFrames(ch, "firebolt")
				if where == "main" {
					if got >= base {
						t.Fatalf("main-hand cooldown=%d baseline=%d", got, base)
					}
				} else if got != base {
					t.Fatalf("inactive cooldown=%d want %d", got, base)
				}
				def, _ := config.GetWeaponDefinition(key)
				for _, full := range []bool{false, true} {
					for _, text := range []string{GetItemTooltip(it, ch, cs, full), GetItemTooltip(items.CreateWeaponFromYAML(items.GetWeaponKeyByName(def.Name)), nil, nil, full)} {
						if !strings.Contains(text, "(main hand only)") {
							t.Fatalf("card hides hand restriction (full=%v):\n%s", full, text)
						}
					}
				}
			})
		}
	}
}

func TestSpellDisintegrateLaunchSnapshot(t *testing.T) {
	for _, key := range []string{"firebolt", "disintegrate"} {
		for _, card := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/card=%v", key, card), func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				g := cs.game
				equipSpellAndPrepareCaster(t, cs, key, 200, 30)
				if card {
					g.cardSlots[0].key = "alien_card"
				}
				def, _ := config.GetSpellDefinition(key)
				want := def.DisintegrateChance
				if card {
					d, _ := config.GetItemDefinition("alien_card")
					want += float64(d.CardDisintegratePct) / 100
				}
				if !cs.CastEquippedSpell() {
					t.Fatal("cast refused")
				}
				if len(g.magicProjectiles) != 1 {
					t.Fatalf("projectiles=%d", len(g.magicProjectiles))
				}
				if got := g.magicProjectiles[0].DisintegrateChance; math.Abs(got-want) > 1e-9 {
					t.Fatalf("spell chance=%v want %v", got, want)
				}
				g.cardSlots = [MaxCardSlots]cardSlot{}
				if g.magicProjectiles[0].DisintegrateChance != want {
					t.Fatal("in-flight spell changed after card removal")
				}
				g.clearTransientCombatState()
				if len(g.magicProjectiles) != 0 {
					t.Fatal("transition retained magic projectile")
				}
			})
		}
	}
}

// Spell, crossfire and reflected projectiles preserve their distinct policies:
// only player/crossfire payloads can disintegrate, and all respect immunity.
func TestProjectileDisintegrateOwnerContract(t *testing.T) {
	for _, kind := range []string{"arrow", "magic_projectile"} {
		for _, owner := range []ProjectileOwner{ProjectileOwnerPlayer, ProjectileOwnerBoundUndead, ProjectileOwnerReflected} {
			for _, state := range []string{"landed", "undead", "dragon", "warded", "sealed", "dodged"} {
				t.Run(fmt.Sprintf("%s/owner=%v/%s", kind, owner, state), func(t *testing.T) {
					cs := newTestCombatSystemWithConfig(t)
					g := cs.game
					m := mkTestMonster("Owner target", 10000)
					m.ID = "owner-target"
					m.X = 64
					switch state {
					case "undead", "dragon":
						m.MonsterType = state
					case "warded":
						m.BossWarded = true
					case "sealed":
						m.BossDormant = true
					case "dodged":
						m.PerfectDodge = 100
					}
					g.world.Monsters = []*monsterPkg.Monster3D{m}
					g.collisionSystem.RegisterEntity(collision.NewEntity(m.ID, m.X, m.Y, 32, 32, collision.CollisionTypeMonster, false))
					g.collisionSystem.RegisterEntity(collision.NewEntity("owner-bolt", m.X, m.Y, 16, 16, collision.CollisionTypeProjectile, false))
					if kind == "arrow" {
						g.arrows = []Arrow{{ID: "owner-bolt", Active: true, LifeTime: 100, X: m.X, Y: m.Y, Damage: 1, DisintegrateChance: 1, DamageType: "physical", BowKey: "alien_blaster", Owner: owner, SourceMonster: m}}
					} else {
						g.magicProjectiles = []MagicProjectile{{ID: "owner-bolt", Active: true, LifeTime: 100, X: m.X, Y: m.Y, Damage: 1, DisintegrateChance: 1, SpellType: "firebolt", Owner: owner, SourceMonster: m}}
					}
					cs.CheckProjectileMonsterCollisions()
					wantDead := state == "landed" && owner != ProjectileOwnerReflected
					if dead := !m.IsAlive(); dead != wantDead {
						t.Fatalf("dead=%v want %v", dead, wantDead)
					}
				})
			}
		}
	}
}

func TestContinuationDoesNotBorrowTurnBasedAimAssist(t *testing.T) {
	for _, continuation := range []bool{false, true} {
		t.Run(fmt.Sprintf("continuation=%v", continuation), func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			g.turnBasedMode = true
			g.camera.X, g.camera.Y = 96, 96
			m := mkTestMonster("Front", 1000)
			m.ID = "front-assist"
			m.X, m.Y = 160, 96
			g.world.Monsters = []*monsterPkg.Monster3D{m}
			g.collisionSystem.RegisterEntity(collision.NewEntity(m.ID, m.X, m.Y, 8, 8, collision.CollisionTypeMonster, false))
			ar := Arrow{ID: "assist-bolt", Active: true, LifeTime: 100, X: 160, Y: 180, VelX: 8, Damage: 10, DamageType: "physical", Owner: ProjectileOwnerPlayer, BowKey: "hunting_bow"}
			if continuation {
				ar.SkipMonster = mkTestMonster("Previous", 1000)
			}
			g.arrows = []Arrow{ar}
			g.collisionSystem.RegisterEntity(collision.NewEntity(ar.ID, ar.X, ar.Y, 8, 8, collision.CollisionTypeProjectile, false))
			if got := cs.turnBasedProjectileAssistTarget(ar.X, ar.Y, ar.VelX, ar.VelY); got != m {
				t.Fatal("fixture is not eligible for initial-shot aim assist")
			}
			cs.CheckProjectileMonsterCollisions()
			want := 990
			if continuation {
				want = 1000
			}
			if m.HitPoints != want {
				t.Fatalf("HP=%d want %d", m.HitPoints, want)
			}
		})
	}
}
