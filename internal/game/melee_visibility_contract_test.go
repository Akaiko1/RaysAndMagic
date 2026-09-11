package game

import (
	"fmt"
	"testing"

	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/world"
)

func TestMeleeDeliveryVisibility(t *testing.T) {
	for _, entry := range []string{"party", "monster_party", "monster_crossfire", "champion_party", "champion_crossfire"} {
		for _, blocked := range []bool{false, true} {
			for _, tb := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/blocked_%v/TB_%v", entry, blocked, tb), func(t *testing.T) {
					g, ts := summonTileWorld(t)
					g.turnBasedMode = tb
					placePlayerAtTile(g, 5, 5, ts)
					g.camera.Angle = 0
					if blocked {
						g.world.Tiles[5][6] = world.TileWall
					}
					m := monsterPkg.NewMonster3DFromConfig(7.5*ts, 5.5*ts, "dragon_green", g.config)
					m.DamageMin, m.DamageMax = 100, 100
					g.world.Monsters = []*monsterPkg.Monster3D{m}
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					for _, ch := range g.party.Members {
						ch.Luck = 0
						ch.HitPoints = 1000
						ch.MaxHitPoints = 1000
					}
					before := m.HitPoints
					var target *monsterPkg.Monster3D
					if entry == "monster_crossfire" || entry == "champion_crossfire" {
						target = monsterPkg.NewMonster3DFromConfig(5.5*ts, 5.5*ts, "skeleton", g.config)
						target.Bound = true
						target.HitPoints, target.MaxHitPoints = 1000, 1000
						target.PerfectDodge = 0
						g.world.Monsters = append(g.world.Monsters, target)
						g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					}
					if entry == "champion_party" || entry == "champion_crossfire" {
						primeTestChampions(t, g)
						overrideChampionMainHand(t, g, "weapon_master", "impossible", "iron_spear")
						m = monsterPkg.NewMonster3DFromConfig(7.5*ts, 5.5*ts, "weapon_master", g.config)
						g.world.Monsters[0] = m
					}
					switch entry {
					case "party":
						w, err := items.TryCreateWeaponFromYAML("iron_spear")
						if err != nil {
							t.Fatal(err)
						}
						g.party.Members[0].Equipment[items.SlotMainHand] = w
						g.combat.EquipmentMeleeAttack()
					case "monster_party":
						g.combat.applyMonsterMeleeDamage(m)
					case "monster_crossfire":
						g.combat.monsterStrikeMonster(m, target)
					case "champion_party":
						g.combat.championMeleeStrike(m, false)
					case "champion_crossfire":
						g.combat.championCrossfireStrike(m, target, false)
					}
					damaged := false
					if entry == "party" {
						damaged = m.HitPoints < before
					} else if target != nil {
						damaged = target.HitPoints < 1000
					} else {
						for _, ch := range g.party.Members {
							damaged = damaged || ch.HitPoints < 1000
						}
					}
					if damaged == blocked {
						t.Fatalf("damaged=%v, blocked=%v", damaged, blocked)
					}
				})
			}
		}
	}
}
