package game

import (
	"fmt"
	"math"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func TestMeleeContactReciprocity(t *testing.T) {
	g, ts := summonTileWorld(t)
	offsets := []float64{0.05, 0.5, 0.95}
	for _, reach := range []int{1, 2} {
		weaponKey, monsterKey := "steel_axe", "goblin"
		if reach == 2 {
			weaponKey, monsterKey = "iron_spear", "dragon_green"
		}
		weapon := items.CreateWeaponFromYAML(weaponKey)
		for _, tb := range []bool{false, true} {
			t.Run(fmt.Sprintf("range=%d/TB=%v", reach, tb), func(t *testing.T) {
				g.turnBasedMode = tb
				contacts := 0
				for dx := -reach; dx <= reach; dx++ {
					for dy := -reach; dy <= reach; dy++ {
						if dx == 0 && dy == 0 {
							continue
						}
						for _, px := range offsets {
							for _, py := range offsets {
								for _, mx := range offsets {
									for _, my := range offsets {
										for _, obstacle := range []string{"clear", "corner", "occupied target"} {
											g.world.Tiles[10][11] = world.TileEmpty
											if obstacle == "corner" {
												g.world.Tiles[10][11] = world.TileWall
											}
											if obstacle == "occupied target" {
												g.world.Tiles[10+dy][10+dx] = world.TileWall
											}
											g.camera.X, g.camera.Y = (10+px)*ts, (10+py)*ts
											m := monster.NewMonster3DFromConfig((float64(10+dx)+mx)*ts, (float64(10+dy)+my)*ts, monsterKey, g.config)
											m.AttackRadius = float64(reach) * ts
											m.HitPoints, m.MaxHitPoints = 1000, 1000
											g.world.Monsters = []*monster.Monster3D{m}
											g.camera.Angle = math.Atan2(m.Y-g.camera.Y, m.X-g.camera.X)
											canHit := g.combat.monsterCanAttackParty(m, Distance(m.X, m.Y, g.camera.X, g.camera.Y), m.AttackRadius)
											hits := g.combat.performMeleeHitDetection(weapon, 1, &config.MeleeAttackConfig{ArcType: 1}, false)
											if canHit && hits != 1 {
												t.Fatalf("delta=%d,%d party=%g,%g mob=%g,%g obstacle=%s: equal-range reply unavailable", dx, dy, px, py, mx, my, obstacle)
											}
											if obstacle == "occupied target" && (canHit || hits != 0) {
												t.Fatal("opaque endpoint allowed one-way melee")
											}
											if canHit {
												contacts++
											}
											g.world.Tiles[10+dy][10+dx] = world.TileEmpty
										}
									}
								}
							}
						}
					}
				}
				if contacts < 100 {
					t.Fatalf("positive contact coverage missing: %d", contacts)
				}
			})
		}
	}
}

func TestMeleeLongReachRemainsAnExplicitDifference(t *testing.T) {
	g, ts := summonTileWorld(t)
	placePlayerAtTile(g, 10, 10, ts)
	g.camera.Angle = 0
	for _, tb := range []bool{false, true} {
		g.turnBasedMode = tb
		m := monster.NewMonster3DFromConfig(12.5*ts, 10.5*ts, "dragon_green", g.config)
		g.world.Monsters = []*monster.Monster3D{m}
		if !g.combat.monsterCanAttackParty(m, 2*ts, m.GetAttackRangePixels()) {
			t.Fatal("authored long melee reach lost")
		}
		if hits := g.combat.performMeleeHitDetection(items.CreateWeaponFromYAML("steel_axe"), 1, &config.MeleeAttackConfig{ArcType: 1}, false); hits != 0 {
			t.Fatal("range-one weapon gained arbitrary long reach")
		}
		if hits := g.combat.performMeleeHitDetection(items.CreateWeaponFromYAML("iron_spear"), 1, &config.MeleeAttackConfig{ArcType: 1}, false); hits != 1 {
			t.Fatal("matching spear reach failed")
		}
	}
}

func TestEqualRangeMeleeCommitAndReply(t *testing.T) {
	for _, reach := range []int{1, 2} {
		for _, tb := range []bool{false, true} {
			for _, blocked := range []bool{false, true} {
				t.Run(fmt.Sprintf("range=%d/TB=%v/endpoint=%v", reach, tb, blocked), func(t *testing.T) {
					g, ts := summonTileWorld(t)
					g.turnBasedMode = tb
					placePlayerAtTile(g, 10, 10, ts)
					g.camera.Angle = 0
					m := monster.NewMonster3DFromConfig((10.5+float64(reach))*ts, 10.5*ts, "dragon_green", g.config)
					m.AttackRadius = float64(reach) * ts
					m.HitPoints, m.MaxHitPoints = 10000, 10000
					m.BeginPlayerEngagement()
					m.State, m.StateTimer = monster.StateAttacking, 1
					m.AttackCDFrames = 0
					g.world.Monsters = []*monster.Monster3D{m}
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					if blocked {
						g.world.Tiles[10][10+reach] = world.TileWall
					}
					cadence := monsterAttackRealtime
					if tb {
						cadence = monsterAttackTurn
					}
					if spent := g.combat.commitMonsterAttack(m, monsterAttackDestination{}, cadence); spent == blocked {
						t.Fatalf("commit=%v, endpoint blocked=%v", spent, blocked)
					}
					key := "steel_axe"
					if reach == 2 {
						key = "iron_spear"
					}
					hits := g.combat.performMeleeHitDetection(items.CreateWeaponFromYAML(key), 1, &config.MeleeAttackConfig{ArcType: 1}, false)
					if (hits > 0) == blocked {
						t.Fatalf("reply hits=%d, endpoint blocked=%v", hits, blocked)
					}
				})
			}
		}
	}
}

func TestRangedOccupiedTileCannotAttack(t *testing.T) {
	g, ts := summonTileWorld(t)
	placePlayerAtTile(g, 10, 10, ts)
	m := monster.NewMonster3DFromConfig(13.5*ts, 10.5*ts, "lich", g.config)
	g.world.Tiles[10][13] = world.TileWall
	if !m.HasRangedAttack() {
		t.Fatal("test requires ranged attacker")
	}
	if g.combat.monsterCanAttackParty(m, 3*ts, m.GetAttackRangePixels()) {
		t.Fatal("ranged observer attacked from inside an object")
	}
}
