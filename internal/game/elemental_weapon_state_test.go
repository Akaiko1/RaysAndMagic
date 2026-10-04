package game

import (
	"fmt"
	"math"
	"testing"

	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
)

type transferObstacleTiles struct {
	testTileChecker
	water bool
}

func (c *transferObstacleTiles) IsTileBlockingForMonster(x, y int, overrides []string, flying bool) bool {
	return x == 4 && !(c.water && flying)
}

func TestTransferStrikeDisplacement(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, obstacle := range []string{"clear", "wall", "water", "flight", "arena", "post", "solid_prop", "idol", "canopy", "root", "stun", "latched_stun", "immobile", "full_slow"} {
			t.Run(fmt.Sprintf("%s/TB=%v", obstacle, tb), func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				g := cs.game
				g.turnBasedMode = tb
				g.camera.X, g.camera.Y = 96, 96
				m, _ := partyDamageTargets(g, 64)
				g.world.Monsters = g.world.Monsters[:1]
				m.Speed = 1
				held := true
				switch obstacle {
				case "idol":
					m.WarlordIdol = true
				case "canopy":
					m.Arbor.Phase = "perched"
				case "root":
					m.RootFramesRemaining, m.RootTurnsRemaining = 60, 2
				case "stun":
					m.StunFramesRemaining, m.StunTurnsRemaining = 60, 2
				case "latched_stun":
					held = tb
					g.turnBasedMonsterStunned = map[*monster.Monster3D]bool{m: true}
				case "immobile":
					m.Speed = 0
				case "full_slow":
					m.SlowPct, m.SlowFramesRemaining, m.SlowTurnsRemaining = 100, 60, 2
				default:
					held = false
				}
				def, _ := config.GetWeaponDefinition("solstice_transfer_blade")
				g.party.Members[0].Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("solstice_transfer_blade")
				g.party.Members[0].BuffBonuses.Accuracy = 300 // The proc must stay fixed.
				if obstacle == "wall" || obstacle == "water" || obstacle == "flight" {
					g.collisionSystem = collision.NewCollisionSystem(&transferObstacleTiles{testTileChecker{100, 100}, obstacle != "wall"}, 64)
					m.Flying = obstacle == "flight"
				}
				entity := collision.NewEntity(m.ID, m.X, m.Y, 24, 24, collision.CollisionTypeMonster, false)
				if obstacle == "arena" {
					entity.WithMovementBounds(collision.MovementBounds{Enabled: true, MinX: 64, MinY: 64, MaxX: 256, MaxY: 256})
				}
				g.collisionSystem.RegisterEntity(entity)
				if obstacle == "post" || obstacle == "solid_prop" {
					typ := collision.CollisionTypeMonsterEngaged
					if obstacle == "solid_prop" {
						typ = collision.CollisionTypeNPC
					}
					g.collisionSystem.RegisterEntity(collision.NewEntity("blocker", 288, 96, 64, 64, typ, obstacle == "solid_prop"))
				}
				x := m.X
				for i := 1; i <= def.ElementalAbility.Hits; i++ {
					before := m.HitPoints
					cs.ApplyDamageToMonster(m, 100, def.Name, false)
					want := 100
					if i == def.ElementalAbility.Hits {
						want += def.ElementalAbility.Damage
					}
					if got := before - m.HitPoints; got != want {
						t.Fatalf("hit %d damage=%d want=%d", i, got, want)
					}
					if i < def.ElementalAbility.Hits && m.X != x {
						t.Fatal("push before third hit")
					}
				}
				if held {
					if m.X != x {
						t.Fatalf("immovable target pushed from %g to %g", x, m.X)
					}
				} else if obstacle == "clear" || obstacle == "flight" || obstacle == "latched_stun" {
					want := x + def.ElementalAbility.RangeTiles*64
					if math.Abs(m.X-want) > 1e-6 {
						t.Fatalf("push=%g want=%g", m.X, want)
					}
				} else if m.X <= x || m.X >= 256 {
					t.Fatalf("push crossed obstacle or failed to move safely: %g", m.X)
				}
				if entity.BoundingBox.X != m.X || entity.BoundingBox.Y != m.Y {
					t.Fatal("collision position was not updated")
				}
				if _, ok := m.ElementalMarks["crosswind"]; ok {
					t.Fatal("third strike did not consume buildup")
				}
			})
		}
	}
}

func TestElementalStacksSaveAndResume(t *testing.T) {
	for _, key := range []string{"solstice_thermal_lance", "solstice_anchor_hammer", "solstice_transfer_blade"} {
		for _, tb := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/TB=%v", key, tb), func(t *testing.T) {
				g, _, wm := clockSaveFixture(t)
				g.turnBasedMode = tb
				def, _ := config.GetWeaponDefinition(key)
				hero := g.party.Members[0]
				hero.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(key)
				m := monster.NewMonster3DFromConfig(g.camera.X+64, g.camera.Y, "wolf", g.config)
				m.HitPoints, m.MaxHitPoints, m.ArmorClass, m.PerfectDodge, m.Resistances = 10000, 10000, 0, 0, nil
				g.world.Monsters = []*monster.Monster3D{m}
				for range 2 {
					g.combat.ApplyDamageToMonster(m, 100, def.Name, false)
				}
				for range g.config.GetTPS() {
					m.TickElementalMarks(false)
				}
				before := m.ElementalMarks[def.ElementalAbility.Kind]
				if before.Count != 2 {
					t.Fatalf("fixture did not charge: %+v", before)
				}
				// Old return marks must not revive the replaced Backwash rule.
				m.SetElementalMark("backwash", monster.ElementalWeaponMark{}, 600, 3)
				save := auditSaveJSON(t, g.buildSave(wm))
				g.restoreSavedMonsters(wm, &save)
				m = g.world.Monsters[0]
				if _, exists := m.ElementalMarks["backwash"]; exists {
					t.Fatal("obsolete backwash mark restored")
				}
				if got := m.ElementalMarks[def.ElementalAbility.Kind]; got != before {
					t.Fatalf("saved remainder=%+v want=%+v", got, before)
				}
				m.ArmorClass, m.PerfectDodge, m.Resistances = 0, 0, nil
				hp := m.HitPoints
				g.combat.ApplyDamageToMonster(m, 100, def.Name, false)
				want := 100 + def.ElementalAbility.Damage
				switch def.ElementalAbility.Kind {
				case "anchor":
					want = 100 + 3*def.ElementalAbility.Damage
				case "pressure":
					_, _, _, _, _, speed, _ := hero.GetEffectiveStats()
					want += speed / def.ElementalAbility.StatDivisor
				}
				if got := hp - m.HitPoints; got != want {
					t.Fatalf("resumed hit=%d want=%d", got, want)
				}
			})
		}
	}
}
