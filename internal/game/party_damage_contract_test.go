package game

import (
	"fmt"
	"math"
	"sort"
	"testing"
	"ugataima/internal/status"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

func partyDamageTargets(g *MMGame, tile float64) (*monsterPkg.Monster3D, *monsterPkg.Monster3D) {
	primary, secondary := mkTestMonster("Primary", 10000), mkTestMonster("Secondary", 10000)
	primary.ID, secondary.ID = "primary", "secondary"
	primary.X, primary.Y = g.camera.X+tile, g.camera.Y
	secondary.X, secondary.Y = primary.X+tile/2, primary.Y
	primary.ArmorClass, secondary.ArmorClass = 0, 0
	primary.PerfectDodge, secondary.PerfectDodge = 0, 0
	primary.Resistances, secondary.Resistances = nil, nil
	g.world.Monsters = []*monsterPkg.Monster3D{primary, secondary}
	return primary, secondary
}

// Every authored AoE weapon x pre-impact marks x launch crit x surviving or
// lethal primary, in RT, plus one TB cell per weapon. A death burst remains
// its own, non-critical hit.
func TestAuthoredWeaponSplashDesignationContract(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	var keys []string
	for key, def := range config.GlobalWeapons.Weapons {
		if def.AoeRadiusTiles > 0 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		t.Fatal("no AoE weapons loaded")
	}
	type cell struct {
		key, marks   string
		tb, critical bool
		lethal       bool
	}
	var cells []cell
	for _, key := range keys {
		for _, critical := range []bool{false, true} {
			// A launch crit already doubles every victim: marks cannot upgrade
			// it further, so a crit needs only no marks and every mark.
			marks := []string{"none", "primary", "secondary", "both"}
			if critical {
				marks = []string{"none", "both"}
			}
			for _, m := range marks {
				for _, lethal := range []bool{false, true} {
					cells = append(cells, cell{key: key, marks: m, critical: critical, lethal: lethal})
				}
			}
		}
		// The damage contract reads no combat mode; one TB cell per weapon
		// keeps the turn-based path honest.
		cells = append(cells, cell{key: key, marks: "secondary", tb: true, lethal: true})
	}
	for _, c := range cells {
		key, tb, marks, critical, lethal := c.key, c.tb, c.marks, c.critical, c.lethal
		t.Run(fmt.Sprintf("%s/tb=%v/%s/crit=%v/lethal=%v", key, tb, marks, critical, lethal), func(t *testing.T) {
			g, _, ch, tile := sniperFixture(t, tb)
			isolateTrueDamageMember(ch, 0)
			ch.Skills[character.SkillDesignateTarget] = &character.Skill{}
			ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(key)
			g.combat.designationRoll = func(int) int { return 0 }
			g.cardSlots = [MaxCardSlots]cardSlot{}
			g.addCombatBuff(TimedCombatBuff{ID: "test", Clock: framesClock(600), OutBonus: 7, OutDamageType: "all"})
			def, _ := config.GetWeaponDefinition(key)
			primary, secondary := partyDamageTargets(g, tile)
			if marks == "primary" || marks == "both" {
				g.designateTarget(ch, primary)
			}
			if marks == "secondary" {
				g.designateTarget(ch, secondary)
			}
			if marks == "both" {
				other := character.CreateCharacter("Other", character.ClassSniper, g.config)
				g.party.Members = append(g.party.Members, other)
				g.designateTarget(other, secondary)
			}
			if lethal {
				primary.HitPoints = 1
			}
			normal := 100
			if critical {
				normal = 200
			}
			trueDamage, _ := g.combat.weaponMasteryStrike(ch, def)
			if def.Range > 3 {
				shot := &Arrow{ID: "splash-contract", Active: true, LifeTime: 60, Owner: ProjectileOwnerPlayer,
					Attacker: ch, BowKey: key, Damage: normal, TrueDamage: trueDamage, DamageType: weaponDamageTypeStr(def), Crit: critical}
				g.combat.applyProjectileDamage(shot, primary, shot.ID)
				if ch.DesignatedTargetID != primary.ID {
					t.Fatal("successful ranged hit did not replace the mark")
				}
				if shot.Damage != normal || shot.Crit != critical {
					t.Fatal("impact modified the launch payload")
				}
			} else {
				g.combat.ApplyDamageToMonster(primary, normal, testWeapon(def.Name), critical)
			}
			for i, victim := range []*monsterPkg.Monster3D{primary, secondary} {
				if i == 0 && lethal {
					if victim.IsAlive() {
						t.Fatal("lethal primary survived")
					}
					continue
				}
				wantNormal := 100
				if critical || marks == "both" || (i == 0 && marks == "primary") || (i == 1 && marks == "secondary") {
					wantNormal = 200
				}
				want := wantNormal + 7 + trueDamage
				if i == 1 && lethal && def.DeathBurstRadiusTiles >= 0.5 {
					want += primary.MaxHitPoints * def.DeathBurstMaxHPPct / 100
				}
				if got := 10000 - victim.HitPoints; got != want {
					t.Fatalf("%s took %d, want %d (crit affects only its pre-buff normal damage)", victim.Name, got, want)
				}
			}
		})
	}
}

func TestDesignationConversionAndRestoredMark(t *testing.T) {
	for _, restored := range []bool{false, true} {
		for _, ranged := range []bool{false, true} {
			t.Run(fmt.Sprintf("restored=%v/ranged=%v", restored, ranged), func(t *testing.T) {
				g, _, ch, tile := sniperFixture(t, false)
				isolateTrueDamageMember(ch, 0)
				ch.Skills[character.SkillDesignateTarget] = &character.Skill{}
				g.combat.designationRoll = func(int) int { return 0 }
				g.cardSlots = [MaxCardSlots]cardSlot{}
				g.cardSlots[0].key = "masked_hexer_girl_card"
				g.addCombatBuff(TimedCombatBuff{ID: "test", Clock: framesClock(600), OutBonus: 7, OutDamageType: "all"})
				primary, secondary := partyDamageTargets(g, tile)
				for _, m := range g.world.Monsters {
					m.Resistances = map[monsterPkg.DamageType]int{monsterPkg.DamageDark: 50}
					m.SoakDamage, m.Soak.Frames = 5, 60
				}
				g.designateTarget(ch, secondary)
				if restored {
					ch = restoreCharacterSave(buildCharacterSave(ch))
					g.party.Members[0] = ch
				}
				def, _ := config.GetWeaponDefinition("idol_breakers_maul")
				// Use a physical splash arrow to exercise conversion in the ranged path;
				// this is the same source packet as the authored melee maul.
				if ranged {
					shot := &Arrow{ID: "conversion", Active: true, LifeTime: 60, Owner: ProjectileOwnerPlayer, Attacker: ch,
						BowKey: "idol_breakers_maul", Damage: 101, TrueDamage: 12, DamageType: "physical"}
					g.combat.applyProjectileDamage(shot, primary, shot.ID)
				} else {
					ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("idol_breakers_maul")
					g.cardSlots[1].key = "orc_card" // 12 flat melee true
					g.combat.ApplyDamageToMonster(primary, 101, testWeapon(def.Name), false)
				}
				// Ordinary: 108 -> 87 physical + 21 dark -> 97, minus 5 soak, plus 12 true.
				// Marked: 209 -> 168 physical + 41 dark -> 188, minus 5 soak, plus 12 true.
				for i, want := range []int{104, 195} {
					if got := 10000 - g.world.Monsters[i].HitPoints; got != want {
						t.Fatalf("victim %d took %d, want %d", i, got, want)
					}
				}
			})
		}
	}
}

// Enumerate authored damaging spells instead of maintaining a parallel spell
// registry. Each form must apply offensive cards per victim and ignore marks.
func TestAuthoredSpellVictimBonusContract(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	var keys []string
	for key := range config.GlobalSpells.Spells {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		def, err := spells.GetSpellDefinitionByID(spells.SpellID(key))
		if err != nil {
			t.Fatal(err)
		}
		if def.DealsNoDamage || def.BindUndead || def.Pacify {
			continue
		}
		if !def.IsProjectile && def.MortarRangeTiles <= 0 && def.ZoneRadiusTiles <= 0 && def.PartyAoeRadiusTiles <= 0 && !def.MapWide {
			continue
		}
		for _, tb := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/tb=%v", key, tb), func(t *testing.T) {
				g, _, ch, tile := sniperFixture(t, tb)
				g.combat.designationRoll = func(int) int { return 0 }
				g.cardSlots = [MaxCardSlots]cardSlot{}
				g.cardSlots[0].key = "elf_archer_card"
				g.addCombatBuff(TimedCombatBuff{ID: "test", Clock: framesClock(600), OutBonus: 7, OutDamageType: "all"})
				primary, secondary := partyDamageTargets(g, tile)
				secondary.MonsterType = "dragon"
				g.designateTarget(ch, secondary)
				normal, trueDamage := 100, 13
				splash := true
				switch {
				case def.MortarRangeTiles > 0:
					g.combat.detonateMortar(pendingMortar{SpellID: key, X: primary.X, Y: primary.Y, RadiusTiles: def.AoeRadiusTiles,
						Damage: normal, TrueDamage: trueDamage, School: def.School, Caster: ch})
				case def.ZoneRadiusTiles > 0:
					z := &PersistentDamageZone{SpellID: key, FieldID: 1, X: primary.X, Y: primary.Y, Radius: 2 * tile, FramesLeft: 60, TickDamage: normal, TrueTickDamage: trueDamage}
					g.combat.damageZoneMonsters(key, []*PersistentDamageZone{z}, []*PersistentDamageZone{z})
				case def.PartyAoeRadiusTiles > 0 || def.MapWide:
					parts := g.combat.spellDamageParts(def.ID, ch, g.combat.CalculatePartyNovaDamage(def, ch))
					normal, trueDamage = parts.Normal, parts.True
					if !g.combat.tryCastPartyNova(def, ch) {
						t.Fatal("nova not handled")
					}
				default:
					shot := &MagicProjectile{ID: "spell-contract", Active: true, LifeTime: 60, Damage: normal, TrueDamage: trueDamage, SpellType: key, Attacker: ch}
					g.combat.applyProjectileDamage(shot, primary, shot.ID)
					splash = def.AoeRadiusTiles > 0
				}
				for i, victim := range []*monsterPkg.Monster3D{primary, secondary} {
					want := normal + 7 + trueDamage
					if i == 1 {
						want = int(math.Round(float64(normal+7)*1.25)) + trueDamage
						if !splash {
							want = 0
						}
					}
					if got := 10000 - victim.HitPoints; got != want {
						t.Fatalf("%s took %d, want %d; bonus must be per victim and normal only", victim.Name, got, want)
					}
				}
			})
		}
	}
}

// A spell's rolled crit reaches the victim's log line whichever way the
// spell lands: a direct projectile hit or a mortar bloom.
func TestSpellCritReachesTheLog(t *testing.T) {
	for _, delivery := range []string{"projectile", "mortar"} {
		for _, crit := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/crit=%v", delivery, crit), func(t *testing.T) {
				g, _, ch, tile := sniperFixture(t, false)
				primary, _ := partyDamageTargets(g, tile)
				switch delivery {
				case "projectile":
					shot := &MagicProjectile{ID: "crit-shot", Active: true, LifeTime: 60, Damage: 50, SpellType: "fireball", Attacker: ch, Crit: crit}
					g.combat.applyProjectileDamage(shot, primary, shot.ID)
				case "mortar":
					def, _ := spells.GetSpellDefinitionByID("stone_blossom")
					g.combat.detonateMortar(pendingMortar{SpellID: "stone_blossom", X: primary.X, Y: primary.Y, RadiusTiles: def.AoeRadiusTiles,
						Damage: 50, School: def.School, Caster: ch, Crit: crit})
				}
				if primary.HitPoints == primary.MaxHitPoints {
					t.Fatal("fixture: the spell did not land")
				}
				if got := countCombatLog(g, "Critical!") > 0; got != crit {
					t.Fatalf("log shows a crit=%v, want %v", got, crit)
				}
			})
		}
	}
}

// A monster's crit - a champion's weapon or spell roll, a boss's
// critical_chance - names itself in its hit line like a party crit, on a hero
// and on a monster, struck directly or by a bolt; a dodged blow says nothing.
func TestMonsterCritReachesTheLog(t *testing.T) {
	for _, route := range []string{"direct", "bolt", "on a monster", "dodged"} {
		for _, crit := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/crit=%v", route, crit), func(t *testing.T) {
				g, _, ch, tile := sniperFixture(t, false)
				ch.HitPoints, ch.MaxHitPoints, ch.Luck = 10000, 10000, 0
				src := mkTestMonster("Critter", 1000)
				hit := monsterCharacterHit{Parts: damagecalc.Parts{Normal: 20}, DamageType: "physical", Melee: true, Critical: crit}
				switch route {
				case "direct":
					g.combat.monsterHitCharacter(src, ch, src.Name, hit)
				case "dodged":
					ch.Luck = 100 * LuckToDodgeDivisor
					g.combat.monsterHitCharacter(src, ch, src.Name, hit)
				case "bolt":
					shot := MagicProjectile{ID: "crit-bolt", X: g.camera.X, Y: g.camera.Y, Damage: 20, LifeTime: 10, Active: true,
						SpellType: "firebolt", Owner: ProjectileOwnerMonster, SourceName: src.Name, SourceMonster: src, Crit: crit}
					g.magicProjectiles = append(g.magicProjectiles, shot)
					g.collisionSystem.RegisterEntity(collision.NewEntity(shot.ID, shot.X, shot.Y, 8, 8, collision.CollisionTypeProjectile, false))
					g.combat.CheckProjectilePlayerCollisions()
				case "on a monster":
					victim, _ := partyDamageTargets(g, tile)
					victim.Bound = true
					g.combat.strikeMonsterPacketFor(src, victim, singleMonsterDamagePacket(damagecalc.Parts{Normal: 20}, "physical", 0), nil, false, false, false, crit)
				}
				if got := countCombatLog(g, "Critical!") > 0; got != (crit && route != "dodged") {
					t.Fatalf("log shows a crit=%v: %v", got, g.combatLogHistory)
				}
			})
		}
	}
}

func TestDesignationContinuationUsesLaunchPayloadAndCurrentMark(t *testing.T) {
	for _, key := range []string{"arbalest", "nest_arbalest"} {
		for _, tb := range []bool{false, true} {
			for _, markNext := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/tb=%v/markNext=%v", key, tb, markNext), func(t *testing.T) {
					g, _, ch, tile := sniperFixture(t, tb)
					g.cardSlots = [MaxCardSlots]cardSlot{}
					primary, secondary := partyDamageTargets(g, tile)
					g.designateTarget(ch, primary)
					// The failed 95% launch roll plus a 5-point designation guarantees the
					// conditional upgrade. Re-reading the currently equipped weapon would
					// incorrectly turn this into an unreliable 5% roll.
					ch.Luck = 0
					shot := &Arrow{ID: "continuation-contract", Active: true, LifeTime: 60, Owner: ProjectileOwnerPlayer,
						Attacker: ch, BowKey: key, Damage: 100, TrueDamage: 13, DamageType: "physical", CritChance: 95, VelX: 10,
						X: primary.X, Y: primary.Y}
					if key == "arbalest" {
						shot.PierceLeft = 1
					} else {
						shot.RicochetLeft = 1
					}
					g.combat.applyProjectileDamage(shot, primary, shot.ID)
					if got := 10000 - primary.HitPoints; got != 213 {
						t.Fatalf("marked first impact took %d, want 213", got)
					}
					if len(g.arrows) != 1 {
						t.Fatalf("got %d continuations, want one", len(g.arrows))
					}
					next := &g.arrows[0]
					if next.Damage != 100 || next.TrueDamage != 13 || next.Crit || next.CritChance != 95 {
						t.Fatalf("continuation inherited target crit instead of launch payload: normal=%d true=%d crit=%v chance=%d", next.Damage, next.TrueDamage, next.Crit, next.CritChance)
					}
					if markNext {
						g.designateTarget(ch, secondary)
					}
					g.combat.applyProjectileDamage(next, secondary, next.ID)
					want := 113
					if markNext {
						want = 213
					}
					if got := 10000 - secondary.HitPoints; got != want {
						t.Fatalf("next impact took %d, want %d", got, want)
					}
				})
			}
		}
	}
}

func TestDesignationImpactEligibility(t *testing.T) {
	for _, state := range []string{"active", "expired", "stunned", "dead", "reserve", "no skill", "proc bolt", "spell"} {
		t.Run(state, func(t *testing.T) {
			g, _, ch, tile := sniperFixture(t, false)
			g.combat.designationRoll = func(int) int { return 0 }
			primary, secondary := partyDamageTargets(g, tile)
			g.designateTarget(ch, secondary)
			want := 100
			switch state {
			case "active":
				want = 200
			case "expired":
				ch.Designation = status.DualClock{}
			case "stunned":
				ch.Stun.Frames = 60
			case "dead":
				ch.HitPoints = 0
			case "reserve":
				other := character.CreateCharacter("Other", character.ClassKnight, g.config)
				g.party.Members = []*character.MMCharacter{other}
				g.party.Reserve = []*character.MMCharacter{ch}
			case "no skill":
				delete(ch.Skills, character.SkillDesignateTarget)
			}
			switch state {
			case "spell":
				shot := &MagicProjectile{ID: "spell", Active: true, LifeTime: 60, Attacker: ch, SpellType: "fireball", Damage: 100}
				g.combat.applyProjectileDamage(shot, primary, shot.ID)
			default:
				shot := &Arrow{ID: "eligibility", Active: true, LifeTime: 60, Owner: ProjectileOwnerPlayer, Attacker: ch, BowKey: "bow_of_hellfire", Damage: 100, DamageType: "dark"}
				if state == "proc bolt" {
					shot.Label = "Bandit Bolt"
					// A proc has no splash, so check its direct hit against the marked foe.
					primary = secondary
				}
				g.combat.applyProjectileDamage(shot, primary, shot.ID)
			}
			if got := 10000 - secondary.HitPoints; got != want {
				t.Fatalf("%s: marked victim took %d, want %d", state, got, want)
			}
		})
	}
}

func TestRestoredSpellZoneKeepsVictimBonusesAndBilling(t *testing.T) {
	for _, restored := range []bool{false, true} {
		t.Run(fmt.Sprint(restored), func(t *testing.T) {
			g, _, _, tile := sniperFixture(t, false)
			g.cardSlots = [MaxCardSlots]cardSlot{}
			g.cardSlots[0].key = "elf_archer_card"
			primary, secondary := partyDamageTargets(g, tile)
			secondary.MonsterType = "dragon"
			zones := []PersistentDamageZone{{SpellID: "firewall", FieldID: 1, X: primary.X, Y: primary.Y, Radius: 2 * tile, FramesLeft: 60, TickDamage: 100, TrueTickDamage: 13}}
			if restored {
				zones = restorePersistentDamageZones(buildPersistentDamageZoneSaves(zones), "")
			}
			z := &zones[0]
			for range 2 {
				g.combat.damageZoneMonsters(z.SpellID, []*PersistentDamageZone{z}, []*PersistentDamageZone{z})
			}
			for i, want := range []int{113, 138} {
				if got := 10000 - g.world.Monsters[i].HitPoints; got != want {
					t.Fatalf("victim %d took %d, want one billed hit of %d", i, got, want)
				}
			}
		})
	}
}

func TestAuthoredSingleTargetWeaponsUseDesignation(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	var keys []string
	for key, def := range config.GlobalWeapons.Weapons {
		if def.AoeRadiusTiles == 0 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			g, _, ch, tile := sniperFixture(t, false)
			isolateTrueDamageMember(ch, 0)
			ch.Skills[character.SkillDesignateTarget] = &character.Skill{}
			ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(key)
			g.combat.designationRoll = func(int) int { return 0 }
			primary, secondary := partyDamageTargets(g, tile)
			// A wounded target keeps victim-state multipliers (the Mainspring
			// Pike's fresh strike) out of this packet check; they have their own.
			primary.HitPoints = primary.MaxHitPoints * 8 / 10
			before := primary.HitPoints
			g.designateTarget(ch, primary)
			def, _ := config.GetWeaponDefinition(key)
			// Instakill chance/immunity has its own production contract table. Force
			// this test through damage to check every authored weapon's normal path.
			chance := def.DisintegrateChance
			def.DisintegrateChance = 0
			t.Cleanup(func() { def.DisintegrateChance = chance })
			// First-hit riders are separate resisted impacts, not part of the
			// designated weapon packet. In particular they must not double.
			firstHitBonus := 0
			if a := def.ElementalAbility; a != nil {
				switch a.Kind {
				case "anchor":
					firstHitBonus = a.Damage
				case "backwash":
					firstHitBonus = a.Damage + ch.GetEffectiveIntellect()/a.StatDivisor
				}
			}
			trueDamage, _ := g.combat.weaponMasteryStrike(ch, def)
			if signature := def.Signature; signature != nil && signature.Kind == "rune_discharge" && ch.SpellPoints >= signature.SPCost {
				firstHitBonus += statScaledDamage(signature.StatScaledDamage, ch)
			}
			if def.Range > 3 {
				shot := &Arrow{ID: "single", Active: true, LifeTime: 60, Owner: ProjectileOwnerPlayer, Attacker: ch, BowKey: key,
					Damage: 100, TrueDamage: trueDamage, DamageType: weaponDamageTypeStr(def), ElementalAbilityDamage: firstHitBonus}
				g.combat.applyProjectileDamage(shot, primary, shot.ID)
			} else {
				g.combat.ApplyDamageToMonster(primary, 100, testWeapon(def.Name), false)
			}
			if got := before - primary.HitPoints; got != 200+trueDamage+firstHitBonus {
				t.Fatalf("marked primary took %d, want %d", got, 200+trueDamage+firstHitBonus)
			}
			if secondary.HitPoints != 10000 {
				t.Fatal("single-target weapon gained splash")
			}
		})
	}
}
