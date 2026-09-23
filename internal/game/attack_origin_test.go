package game

import (
	"fmt"
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

var attackOriginCases = []struct {
	tile    string
	allowed bool
}{
	{"empty", true}, {"water", true}, {"deep_water", true},
	{"dragon_cliffs_chasm_floor", true}, {"dragon_cliffs_chasm_floor_b", true},
	{"wall", false}, {"tree", false}, {"moss_rock", false}, {"desert_dune", false}, {"large_dune", false},
}

func TestFlyingPartyAttackOrigins(t *testing.T) {
	for _, tc := range attackOriginCases {
		for _, tb := range []bool{false, true} {
			for _, action := range []string{"melee", "bow", "spell", "heal", "mouse"} {
				t.Run(fmt.Sprintf("%s/TB=%v/%s", tc.tile, tb, action), func(t *testing.T) {
					g, _, fp, _, tick := mouseCombatHarness(t, tb)
					tile, ok := world.GlobalTileManager.GetTileTypeFromKey(tc.tile)
					if !ok {
						t.Fatal(tc.tile)
					}
					g.world.Tiles[10][10] = tile
					g.flyActive = true
					g.camera.Angle = 0
					caster := g.party.Members[0]
					caster.SpellPoints, caster.MaxSpellPoints = 100, 100
					switch action {
					case "bow":
						caster.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("hunting_bow")
					case "spell":
						caster.LearnSpell("fireball")
						caster.Equipment[items.SlotSpell] = items.Item{Type: items.ItemBattleSpell, SpellEffect: "fireball", SpellCost: 4}
					case "heal":
						caster.LearnSpell("heal_other")
						caster.Equipment[items.SlotSpell] = items.Item{Type: items.ItemUtilitySpell, SpellEffect: items.SpellEffectHealOther, SpellCost: 4}
						g.party.Members[1].HitPoints = g.party.Members[1].MaxHitPoints / 4
					}
					acted := false
					switch action {
					case "melee", "bow":
						acted = g.combat.EquipmentMeleeAttack()
					case "spell":
						acted = g.combat.CastEquippedSpell()
					case "heal":
						acted = g.combat.CastEquippedHealOnTarget(1)
					case "mouse":
						fp.press()
						tick()
						acted = len(g.slashEffects) > 0
					}
					if acted != tc.allowed {
						t.Fatalf("acted=%v, want %v", acted, tc.allowed)
					}
					if !tc.allowed && (caster.SpellPoints != 100 || len(g.magicProjectiles) > 0 || len(g.arrows) > 0 || len(g.slashEffects) > 0) {
						t.Fatal("blocked action paid or spawned effects")
					}
					for _, p := range g.magicProjectiles {
						if !g.world.CanProjectileMoveTo(p.X+p.VelX, p.Y+p.VelY) {
							t.Fatal("allowed spell collided immediately with its origin floor")
						}
					}
				})
			}
		}
	}
}

func TestMonsterAttackOriginsLiveAndSnapshot(t *testing.T) {
	for _, tc := range attackOriginCases {
		for _, key := range []string{"goblin", "lich"} {
			for _, tb := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/TB=%v", tc.tile, key, tb), func(t *testing.T) {
					g, ts := summonTileWorld(t)
					g.turnBasedMode = tb
					placePlayerAtTile(g, 10, 10, ts)
					tile, ok := world.GlobalTileManager.GetTileTypeFromKey(tc.tile)
					if !ok {
						t.Fatal(tc.tile)
					}
					m := spawnMonsterAtTile(g, key, 11, 10, ts)
					g.world.Tiles[10][11] = tile
					m.State, m.StateTimer, m.AttackCDFrames = monster.StateAttacking, 1, 0
					for _, checker := range []collision.SightChecker{g.collisionSystem, g.collisionSystem.Snapshot()} {
						if collision.CanAttackFrom(checker, m.X, m.Y) != tc.allowed {
							t.Fatal("live/snapshot origin rule differs")
						}
					}
					cadence := monsterAttackRealtime
					if tb {
						cadence = monsterAttackTurn
					}
					spent := g.combat.commitMonsterAttack(m, monsterAttackDestination{}, cadence)
					if spent != tc.allowed {
						t.Fatalf("committed=%v, want %v", spent, tc.allowed)
					}
					if !tc.allowed && m.AttackCDFrames != 0 {
						t.Fatal("blocked origin spent attack cooldown")
					}
				})
			}
		}
	}
}

func TestMonstersLeaveObjectsBeforeAttacking(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, snapshot := range []bool{false, true} {
			if tb && snapshot {
				continue
			} // TB uses the live collision owner.
			for _, key := range []string{"goblin", "lich"} {
				for _, origin := range []string{"desert_dune", "moss_rock"} {
					t.Run(fmt.Sprintf("TB=%v/snapshot=%v/%s/%s", tb, snapshot, key, origin), func(t *testing.T) {
						g, ts := summonTileWorld(t)
						g.turnBasedMode = tb
						placePlayerAtTile(g, 10, 10, ts)
						m := spawnMonsterAtTile(g, key, 12, 10, ts)
						m.Flying = origin == "moss_rock"
						if !m.Flying {
							m.WalkableTileOverrides = []string{origin}
						}
						m.State, m.StateTimer = monster.StateAttacking, 1
						tile, ok := world.GlobalTileManager.GetTileTypeFromKey(origin)
						if !ok {
							t.Fatal(origin)
						}
						g.world.Tiles[10][12] = tile
						if !g.collisionSystem.CanMoveToWithTileOverrides(m.ID, m.X, m.Y, m.WalkableTileOverrides, m.Flying) {
							t.Fatal("fixture must allow traversal of the object")
						}
						gl := &GameLoop{game: g}
						attacked := false
						for i := 0; i < 1200 && !attacked; i++ {
							g.frameCount++
							if tb {
								oldHP := partyHPSum(g)
								oldShots := len(g.magicProjectiles) + len(g.arrows)
								runOneMonsterTurn(g, gl)
								attacked = partyHPSum(g) < oldHP || len(g.magicProjectiles)+len(g.arrows) > oldShots
							} else {
								var checker monster.CollisionChecker = g.collisionSystem
								if snapshot {
									checker = g.collisionSystem.Snapshot()
								}
								m.Update(checker, g.camera.X, g.camera.Y)
								g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
								g.refreshMonsterCollisionState(m)
								attacked = g.combat.commitMonsterAttack(m, monsterAttackDestination{}, monsterAttackRealtime)
							}
							if attacked && !g.collisionSystem.CanAttackFrom(m.X, m.Y) {
								t.Fatal("monster attacked before clearing the object")
							}
						}
						if !attacked {
							t.Fatalf("monster never left obstacle and attacked: pos %.1f,%.1f state=%v path=%v", m.X/ts, m.Y/ts, m.State, m.PathTiles)
						}
					})
				}
			}
		}
	}
}
