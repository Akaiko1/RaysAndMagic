package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func overwatchShots(g *MMGame) int {
	n := 0
	for _, a := range g.arrows {
		if a.Overwatch {
			n++
		}
	}
	return n
}

func TestOverwatchAttackActionWiring(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, kind := range []string{"melee", "ranged", "pounce", "champion", "champion stun", "inferno", "volley", "cooldown", "wall", "stunned", "crossfire"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, kind), func(t *testing.T) {
				g, _, ch, ts := sniperFixture(t, tb)
				ch.HitPoints, ch.MaxHitPoints = 10000, 10000
				ch.RTCooldown, ch.OffHandRTCooldown, ch.ActionsRemaining = 99, 81, 0
				rolls := 0
				g.combat.reactionRoll = func() float64 { rolls++; return 0 }
				key, x := "goblin", 6
				if kind == "ranged" {
					key, x = "lich", 8
				}
				if kind == "champion" || kind == "champion stun" {
					primeTestChampions(t, g)
					key = "weapon_master"
					// This case owns weapon delivery; deterministic spell and
					// support replacement cases live in the delivery table.
					def := config.GetChampionDefinition(key)
					original := *def
					t.Cleanup(func() { *def = original })
					def.SpellCastChance = 0
					def.OpeningSpell, def.OpeningSpellTiers = "", nil
				}
				m := spawnMonsterAtTile(g, key, x, 10, ts)
				if kind == "champion" || kind == "champion stun" {
					// Hand riders are re-stamped from the weapon on every
					// swing; zeroing only m.StunCharChance cannot pin them.
					template := g.championTemplateFor(m)
					for _, slot := range []items.EquipSlot{items.SlotMainHand, items.SlotOffHand} {
						weapon := lookupWeaponConfigByName(template.Equipment[slot].Name)
						original := *weapon
						t.Cleanup(func() { *weapon = original })
						weapon.StunChance = 0
						if kind == "champion stun" && slot == items.SlotMainHand {
							weapon.StunChance, weapon.StunTurns = 1, 3
						}
					}
					ch.Luck = 0
					if g.combat.PerfectDodgeChance(ch) != 0 {
						t.Fatal("champion fixture must not dodge its controlled hit")
					}
				}
				m.State, m.StateTimer, m.AttackCDFrames, m.OffHandCDFrames = monster.StateAttacking, 1, 0, 0
				m.AttacksPerRound = 3
				m.DamageMin, m.DamageMax = 1, 1
				m.StunCharChance = 0
				cadence := monsterAttackRealtime
				if tb {
					cadence = monsterAttackTurn
				}
				target := monsterAttackDestination{}
				want := 1
				switch kind {
				case "champion stun":
					want = 0
				case "pounce":
					cadence = monsterAttackPounce
				case "cooldown":
					if tb {
						m.StunTurnsRemaining = 2
					} else {
						m.AttackCDFrames = 100
					}
					want = 0
				case "wall":
					g.world.Tiles[10][x] = world.TileWall
					want = 0
				case "stunned":
					ch.StunFramesRemaining, ch.StunTurnsRemaining = 100, 5
					want = 0
				case "crossfire":
					foe := monster.NewMonster3DFromConfig(g.camera.X, g.camera.Y, "skeleton", g.config)
					foe.Bound = true
					foe.HitPoints = 10000
					m.AIFoe = foe
					target.foe = foe
					g.world.Monsters = append(g.world.Monsters, foe)
					g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
					want = 0
				}
				switch kind {
				case "inferno":
					m.InfernoDamage = 1
					g.combat.applyMonsterInferno(m)
				case "volley":
					m.TrapVolleyCount = 4
					m.TrapVolleyRadiusTiles = 2
					g.combat.tryBossTrapVolley(m, tb)
				default:
					g.combat.commitMonsterAttack(m, target, cadence)
				}
				if kind == "champion stun" && !ch.IsStunned() {
					t.Fatal("champion did not apply guaranteed weapon stun")
				}
				if kind == "champion" && (ch.IsStunned() || ch.HitPoints == ch.MaxHitPoints) {
					t.Fatal("weapon-only champion fixture did not deliver an unstunning hit")
				}
				if shots := overwatchShots(g); shots != want || rolls != want {
					t.Fatalf("shots=%d rolls=%d want %d per action", shots, rolls, want)
				}
				if ch.RTCooldown != 99 || ch.OffHandRTCooldown != 81 || ch.ActionsRemaining != 0 {
					t.Fatal("reaction spent shooter action/cooldowns")
				}
				if kind == "champion" && !tb {
					m.StateTimer = 2
					g.combat.commitMonsterAttack(m, target, cadence)
					if overwatchShots(g) != want {
						t.Fatal("cooling champion ownership produced a phantom attack reaction")
					}
				}
			})
		}
	}
}

func TestOverwatchAttackUsesHalfMovementChance(t *testing.T) {
	for tier := 0; tier < 4; tier++ {
		for _, attack := range []bool{false, true} {
			for _, below := range []bool{false, true} {
				g, _, ch, ts := sniperFixture(t, false)
				ch.Skills[character.SkillOverwatch].Mastery = character.SkillMastery(tier)
				threshold := float64(20+10*tier) / 100
				if attack {
					threshold /= 2
				}
				roll := threshold
				if below {
					roll -= 0.00001
				}
				g.combat.reactionRoll = func() float64 { return roll }
				m := spawnMonsterAtTile(g, "wolf", 7, 10, ts)
				if attack {
					g.observeOverwatchAttack(m)
				} else {
					g.observeOverwatchMovement(m, m.X+ts, m.Y)
				}
				if (overwatchShots(g) > 0) != below {
					t.Fatalf("tier=%d attack=%v roll=%g boundary=%g", tier, attack, roll, threshold)
				}
			}
		}
	}
}

func TestBallisticsCriticalChanceAndTooltip(t *testing.T) {
	for _, key := range []string{"hunting_bow", "alien_blaster", "iron_sword"} {
		for tier := 0; tier < 4; tier++ {
			t.Run(fmt.Sprintf("%s/%d", key, tier), func(t *testing.T) {
				g, _, ch, _ := sniperFixture(t, false)
				weapon, err := items.TryCreateWeaponFromYAML(key)
				if err != nil {
					t.Fatal(err)
				}
				skill := ch.Skills[character.SkillBallistics]
				delete(ch.Skills, character.SkillBallistics)
				before := g.combat.CalculateWeaponCritChance(weapon, ch)
				spellBefore := g.combat.CalculateCriticalChance(ch)
				ch.Skills[character.SkillBallistics] = skill
				skill.Mastery = character.SkillMastery(tier)
				want := 0
				if character.BallisticsWeapon(lookupWeaponConfigByName(weapon.Name)) {
					want = character.BallisticsCritPct(tier)
				}
				if want != 0 && want != 2*(tier+1) {
					t.Fatal("authored progression missing")
				}
				if got := g.combat.CalculateWeaponCritChance(weapon, ch); got != min(100, before+want) {
					t.Fatalf("crit=%d before=%d bonus=%d", got, before, want)
				}
				if g.combat.CalculateCriticalChance(ch) != spellBefore {
					t.Fatal("Ballistics leaked into spell crit")
				}
				text := GetItemTooltip(weapon, ch, g.combat, true)
				if strings.Contains(text, "Ballistics: +") != (want > 0) {
					t.Fatal("tooltip breakdown disagrees with runtime")
				}
				ch.Luck = 10000
				if g.combat.CalculateWeaponCritChance(weapon, ch) != 100 {
					t.Fatal("crit cap lost")
				}
			})
		}
	}
}
