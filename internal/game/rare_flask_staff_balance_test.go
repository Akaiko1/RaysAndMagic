package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
)

func TestRareBalanceFlaskMasteryDamageAndBurn(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for key, base := range map[string][4]int{"harm_flask": {72, 90, 108, 126}, "venom_flask": {24, 36, 48, 60}, "fire_flask": {36, 48, 60, 72}} {
			for tier := 0; tier < 4; tier++ {
				t.Run(fmt.Sprintf("TB=%v/%s/tier=%d", tb, key, tier), func(t *testing.T) {
					g, c := rareClassGame(t, character.ClassAlchemist, tb)
					c.Skills[character.SkillBombThrowing].Mastery = character.SkillMastery(tier)
					c.Intellect, c.Accuracy = 19, 99
					g.party.Inventory = nil
					item := items.CreateItemFromYAML(key)
					g.party.AddItem(item)
					if !g.throwFlask(0, key, false) {
						t.Fatal("flask did not launch")
					}
					p := &g.magicProjectiles[0]
					wantDamage := base[tier] + c.GetEffectiveIntellect()/character.BombThrowingIntellectDivisor
					burnSeconds := 0
					if key == "fire_flask" {
						burnSeconds = [4]int{3, 5, 7, 9}[tier]
					}
					if p.Damage != wantDamage || p.FlaskBurnFrames != burnSeconds*g.config.GetTPS() {
						t.Fatalf("launch damage/burn=%d/%d, expected %d/%d", p.Damage, p.FlaskBurnFrames, wantDamage, burnSeconds*g.config.GetTPS())
					}
					m := zoneVictim(t, g)
					m.ArmorClass = 0
					g.combat.detonateFlask(p, m.X, m.Y)
					if m.MaxHitPoints-m.HitPoints != wantDamage || m.BurnFramesRemaining != burnSeconds*g.config.GetTPS() {
						t.Fatal("impact disagrees with the launched mastery values")
					}
					text := GetItemTooltip(item, c, g.combat, true)
					if !strings.Contains(text, fmt.Sprintf("%d ", wantDamage)) || burnSeconds > 0 && !strings.Contains(text, fmt.Sprintf("Burning: %d seconds", burnSeconds)) {
						t.Fatalf("item tooltip disagrees with impact: %s", text)
					}
				})
			}
		}
	}
}

func TestRareBalanceFlowingStaffGuaranteedCritical(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for tier := 0; tier < 4; tier++ {
			for _, route := range []string{"ranged", "melee", "empty", "other weapon", "rejected", "restored"} {
				t.Run(fmt.Sprintf("TB=%v/tier=%d/%s", tb, tier, route), func(t *testing.T) {
					g, c := rareClassGame(t, character.ClassWayfarer, tb)
					c.Skills[character.SkillFlowingStaff] = &character.Skill{Mastery: character.SkillMastery(tier)}
					c.RareClass.FlowCharges = tier + 1
					c.Luck = 0
					weapon := items.CreateWeaponFromYAML("oak_staff")
					if route == "other weapon" {
						weapon = items.CreateWeaponFromYAML("iron_sword")
					}
					c.Equipment[items.SlotMainHand] = weapon
					def := lookupWeaponConfigByName(weapon.Name)
					original := *def
					t.Cleanup(func() { *def = original })
					def.CritChance = 0
					if route == "melee" {
						def.Range = 1
						def.Melee = lookupWeaponConfigByName("Iron Sword").Melee
					}
					if route == "empty" {
						c.RareClass.FlowCharges = 0
					}
					if route == "restored" {
						c = restoreCharacterSave(buildCharacterSave(c))
						g.party.Members[0] = c
					}
					charged := route != "empty" && route != "other weapon"
					if chance := g.combat.CalculateWeaponCritChance(weapon, c); (chance == 100) != charged {
						t.Fatalf("weapon critical chance=%d, charged=%v", chance, charged)
					}
					if g.combat.CalculateCriticalChance(c) == 100 {
						t.Fatal("staff charge leaked into spell critical chance")
					}
					text := GetItemTooltip(weapon, c, g.combat, true)
					if strings.Contains(text, "Flowing Staff: guaranteed critical hit") != charged {
						t.Fatal("tooltip does not explain the active guarantee")
					}
					before := c.FlowingStaffCharges()
					_, _, damage := g.combat.CalculateWeaponDamage(weapon, c)
					if route == "rejected" {
						def.MaxProjectiles = 1
						g.arrows = []Arrow{{Active: true, BowKey: "oak_staff", LifeTime: 100, Owner: ProjectileOwnerPlayer, Attacker: c}}
					}
					acted := g.combat.equipmentAttackAtAngle(g.camera.Angle, false)
					if route == "rejected" {
						if acted || c.FlowingStaffCharges() != before {
							t.Fatal("rejected launch spent its charge")
						}
						return
					}
					if !acted || c.FlowingStaffCharges() != before-boolInt(charged) {
						t.Fatal("real weapon action did not spend exactly one eligible charge")
					}
					if route == "melee" {
						if len(g.slashEffects) != 1 || !g.slashEffects[0].Crit {
							t.Fatal("melee swing lost its guaranteed critical")
						}
					} else if route != "other weapon" {
						if len(g.arrows) != 1 || g.arrows[0].Crit != charged || g.arrows[0].Damage != weaponCriticalDamage(damage, charged) {
							t.Fatal("projectile did not snapshot the charged critical before spending it")
						}
					}
				})
			}
		}
	}
}
