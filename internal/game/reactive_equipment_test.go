package game

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

func TestApproachWeaponActions(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprint(tb), func(t *testing.T) {
			g, _, ch, tile := sniperFixture(t, tb)
			isolateTrueDamageMember(ch, 0)
			def, _ := config.GetWeaponDefinition("duskneedle")
			ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("duskneedle")
			weapon := ch.Equipment[items.SlotMainHand]
			g.dayNightIsNight = false
			_, _, day := g.combat.CalculateWeaponDamage(weapon, ch)
			g.dayNightIsNight = true
			_, _, night := g.combat.CalculateWeaponDamage(weapon, ch)
			if night-day != def.Damage*(def.NightBaseDamageMultiplier-1) {
				t.Fatalf("night changed more than base: %d -> %d", day, night)
			}
			partyDamageTargets(g, tile)
			g.world.Monsters = g.world.Monsters[:1]
			def, _ = config.GetWeaponDefinition("rainfang")
			before := *def.AttackZoneProc
			t.Cleanup(func() { *def.AttackZoneProc = before })
			def.AttackZoneProc.ChancePct = 100
			ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("rainfang")
			ch.Intellect = 900
			sp := ch.SpellPoints
			if !g.combat.EquipmentMeleeAttack() {
				t.Fatal("attack rejected")
			}
			if len(g.persistentDamageZones) != 1 {
				t.Fatal("committed attack did not cast zone")
			}
			z := g.persistentDamageZones[0]
			if z.TickDamage != 19 || z.CasterName != ch.Name || ch.SpellPoints != sp {
				t.Fatalf("fixed 50 INT spell: %+v SP %d -> %d", z, sp, ch.SpellPoints)
			}
			// The same tile refreshes one existing field; it cannot stack copies.
			g.combat.EquipmentMeleeAttack()
			if len(g.persistentDamageZones) != 1 {
				t.Fatal("proc stacked same-tile zones")
			}
			// An automatic cast cannot weaken a stronger manual field. Include
			// restoration: ownership and snapshot damage must survive loading.
			for _, restored := range []bool{false, true} {
				strong := z
				strong.TickDamage = 100
				strong.CasterName = "Mage"
				strong.FramesLeft = 2 * z.FramesLeft
				g.persistentDamageZones = []PersistentDamageZone{strong}
				if restored {
					g.persistentDamageZones = restorePersistentDamageZones(buildPersistentDamageZoneSaves(g.persistentDamageZones), strong.MapKey)
				}
				g.combat.EquipmentMeleeAttack()
				if len(g.persistentDamageZones) != 1 || g.persistentDamageZones[0].TickDamage != 100 || g.persistentDamageZones[0].CasterName != "Mage" {
					t.Fatalf("proc replaced a stronger field (restored=%v): %+v", restored, g.persistentDamageZones)
				}
			}
			g.persistentDamageZones = nil
			def.AttackZoneProc.ChancePct = 0
			g.combat.EquipmentMeleeAttack()
			if len(g.persistentDamageZones) != 0 {
				t.Fatal("zero-chance proc cast")
			}
			def.AttackZoneProc.ChancePct = 100
			ch.Skills[character.SkillDualWielding] = &character.Skill{Mastery: character.MasteryNovice}
			ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
			ch.Equipment[items.SlotOffHand] = items.CreateWeaponFromYAML("rainfang")
			ch.NextTBAttackOffHand = true
			ch.RTCooldown = 100
			g.combat.EquipmentMeleeAttack()
			if len(g.persistentDamageZones) != 1 {
				t.Fatal("offhand proc missing")
			}
			g.persistentDamageZones = nil
			ch.AddCondition(character.ConditionDead)
			ch.HitPoints = 0
			if g.combat.EquipmentMeleeAttack() || len(g.persistentDamageZones) != 0 {
				t.Fatal("rejected attack triggered a proc")
			}
		})
	}
}

func TestReactiveCardsAndShellThroughCombat(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, ranged := range []bool{false, true} {
			t.Run(fmt.Sprintf("TB=%v/ranged=%v", tb, ranged), func(t *testing.T) {
				g, _, ch, tile := sniperFixture(t, tb)
				isolateTrueDamageMember(ch, 0)
				g.cardSlots[0].key = "dusk_moth_card"
				ch.Luck = 1000
				m, _ := partyDamageTargets(g, tile)
				for i := 0; i < 2; i++ {
					g.combat.monsterHitCharacter(m, ch, m.Name, monsterCharacterHit{Parts: damagecalc.Parts{Normal: 100}, DamageType: "physical"})
				}
				if ch.ReactiveCombat.DodgeCharges != 2 {
					t.Fatal("real dodges did not charge")
				}
				data, err := json.Marshal(buildCharacterSave(ch))
				if err != nil {
					t.Fatal(err)
				}
				var saved CharacterSave
				if err = json.Unmarshal(data, &saved); err != nil {
					t.Fatal(err)
				}
				ch = restoreCharacterSave(saved)
				g.party.Members[0] = ch
				ch.Luck = 0
				key := "shellknocker"
				if ranged {
					key = "duskneedle"
				}
				def, _ := config.GetWeaponDefinition(key)
				ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(key)
				hit := func() {
					if ranged {
						shot := &Arrow{ID: "reactive", Active: true, LifeTime: 60, Owner: ProjectileOwnerPlayer, Attacker: ch, BowKey: key, Damage: 100, DamageType: "physical"}
						g.combat.applyProjectileDamage(shot, "arrow", m, shot.ID)
					} else {
						g.combat.ApplyDamageToMonster(m, 100, def.Name, false)
					}
				}
				m.PerfectDodge = 100
				hit()
				if ch.ReactiveCombat.DodgeCharges != 2 {
					t.Fatal("miss consumed charges")
				}
				m.PerfectDodge = 0
				before := m.HitPoints
				hit()
				if dealt := before - m.HitPoints; dealt != 150 {
					t.Fatalf("charged hit dealt %d want 150", dealt)
				}
				if ch.ReactiveCombat.DodgeCharges != 0 {
					t.Fatal("landed hit kept charges")
				}
				if !ranged {
					if ch.ReactiveCombat.Shell != def.HitShellAbsorption {
						t.Fatal("shell not granted")
					}
					ch.HitPoints, ch.MaxHitPoints = 1000, 1000
					g.combat.monsterHitCharacter(m, ch, m.Name, monsterCharacterHit{Parts: damagecalc.Parts{Normal: 100}, DamageType: "physical", IgnoresArmor: true, IgnoresDodge: true})
					if ch.HitPoints != 915 || ch.ReactiveCombat.Shell != 0 {
						t.Fatalf("shell failed: hp=%d state=%+v", ch.HitPoints, ch.ReactiveCombat)
					}
				}
			})
		}
	}
}

func TestRainSerpentPotionCleansing(t *testing.T) {
	for _, key := range []string{"health_potion", "brewed_health_potion"} {
		for _, quick := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/quick=%v", key, quick), func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				g := cs.game
				ch := g.party.Members[0]
				g.cardSlots[0].key = "rain_serpent_card"
				ch.HitPoints = ch.MaxHitPoints
				ch.Conditions = []character.Condition{character.ConditionNormal}
				p := items.CreateItemFromYAML(key)
				g.party.Inventory = []items.Item{p}
				if g.applyHealTo(0, 0) {
					t.Fatal("healthy full-HP hero wasted potion")
				}
				ch.ApplyPoison(100)
				ch.ApplyBurn(100)
				// Legacy saves can retain damage timers without condition icons.
				ch.Conditions = nil
				ch.StunDRStacks, ch.StunDRMemoryFrames = 2, 200
				ch.ReactiveCombat = character.ReactiveCombatState{Shell: 15, DodgeCharges: 2}
				var used bool
				if quick {
					ch.QuickSlots[0] = &p
					g.party.Inventory = nil
					used = g.useQuickConsumable(0, 0)
				} else {
					used = g.UseConsumableFromInventory(0, 0)
				}
				if !used || ch.Purifiable() || ch.PoisonFramesRemaining != 0 || ch.BurnFramesRemaining != 0 {
					t.Fatalf("failed to cleanse: used=%v conditions=%v", used, ch.Conditions)
				}
				if ch.ReactiveCombat.Shell != 15 || ch.ReactiveCombat.DodgeCharges != 2 {
					t.Fatal("cleansing removed beneficial combat state")
				}
				if ch.StunDRStacks != 2 || ch.StunDRMemoryFrames != 200 {
					t.Fatal("potion reset stun diminishing returns")
				}
				g.party.Inventory = []items.Item{p}
				ch.AddCondition(character.ConditionDead)
				if g.applyHealTo(0, 0) {
					t.Fatal("ordinary potion resurrected dead hero")
				}
				if !strings.Contains(cardDef("rain_serpent_card").CardEffectLines()[0], "harmful") {
					t.Fatal("missing effect description")
				}
			})
		}
	}
}

func TestSunscarabConsecutiveAttacker(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	ch := g.party.Members[0]
	isolateTrueDamageMember(ch, 0)
	ch.HitPoints, ch.MaxHitPoints = 10000, 10000
	g.cardSlots[0].key = "sunscarab_card"
	def := cardDef("sunscarab_card")
	m := &monster.Monster3D{ID: "one", Name: "one", HitPoints: 1000}
	hit := monsterCharacterHit{Parts: damagecalc.Parts{Normal: 100}, DamageType: "physical", IgnoresArmor: true, IgnoresDodge: true}
	for i := 1; i <= 20; i++ {
		before := ch.HitPoints
		cs.monsterHitCharacter(m, ch, m.Name, hit)
		want := 100 - min(def.CardRepeatedHitReductionCap, i*def.CardRepeatedHitReductionPct)
		if before-ch.HitPoints != want {
			t.Fatalf("hit %d dealt %d want %d", i, before-ch.HitPoints, want)
		}
	}
	m.ID = "two"
	before := ch.HitPoints
	cs.monsterHitCharacter(m, ch, m.Name, hit)
	if before-ch.HitPoints != 100-def.CardRepeatedHitReductionPct {
		t.Fatal("another attacker did not reset chain")
	}
	// Extra copies reach the same reduction cap faster, never exceed it.
	g.cardSlots[1].key = "sunscarab_card"
	for i := 0; i < 20; i++ {
		cs.monsterHitCharacter(m, ch, m.Name, hit)
	}
	before = ch.HitPoints
	cs.monsterHitCharacter(m, ch, m.Name, hit)
	if before-ch.HitPoints != 100-def.CardRepeatedHitReductionCap || g.cardCollectionAggregate().CardRepeatedHitReductionCap != def.CardRepeatedHitReductionCap {
		t.Fatal("duplicate card exceeded cap or tooltip disagrees")
	}
	g.clearCardCollectionSlot(1)
	g.clearCardCollectionSlot(0)
	if ch.ReactiveCombat.RepeatedHits != 0 {
		t.Fatal("removed card left stored chain")
	}
}

func TestReactiveProtectionAcrossHitDispatchers(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, kind := range []string{"ordinary", "telegraph", "fireburst", "inferno"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, kind), func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				g := cs.game
				g.turnBasedMode = tb
				ch := g.party.Members[0]
				isolateTrueDamageMember(ch, 0)
				ch.HitPoints, ch.MaxHitPoints = 10000, 10000
				g.cardSlots[0].key = "sunscarab_card"
				ch.ReactiveCombat.Shell = 15
				m := &monster.Monster3D{ID: "source", Name: "source", HitPoints: 1000, FireburstDamageMin: 100, FireburstDamageMax: 100, InfernoDamage: 100}
				hit := func() {
					switch kind {
					case "ordinary":
						cs.monsterHitCharacter(m, ch, m.Name, monsterCharacterHit{Parts: damagecalc.Parts{Normal: 100}, DamageType: "fire", IgnoresArmor: true, IgnoresDodge: true})
					case "telegraph":
						g.damageTelegraphParty(m, m.Name, "fire", damagecalc.Parts{Normal: 100})
					case "fireburst":
						cs.applyMonsterFireburst(m)
					case "inferno":
						cs.applyMonsterInferno(m)
					}
				}
				step := cardDef("sunscarab_card").CardRepeatedHitReductionPct
				for n := 1; n <= 2; n++ {
					before := ch.HitPoints
					hit()
					want := 100 - n*step
					if n == 1 {
						want -= 15
					}
					if before-ch.HitPoints != want || ch.ReactiveCombat.Shell != 0 || ch.ReactiveCombat.RepeatedHits != n {
						t.Fatalf("hit %d: dealt %d want %d; state %+v", n, before-ch.HitPoints, want, ch.ReactiveCombat)
					}
					if n == 1 {
						ch = restoreCharacterSave(buildCharacterSave(ch))
						g.party.Members[0] = ch
					}
				}
			})
		}
	}
}

func TestReactiveProtectionIgnoresSourcelessDamage(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, restored := range []bool{false, true} {
			for _, kind := range []string{"terrain", "crate", "self_splash"} {
				t.Run(fmt.Sprintf("TB=%v/restored=%v/%s", tb, restored, kind), func(t *testing.T) {
					cs := newTestCombatSystemWithConfig(t)
					g := cs.game
					g.turnBasedMode = tb
					ch := g.party.Members[0]
					isolateTrueDamageMember(ch, 0)
					ch.HitPoints, ch.MaxHitPoints = 10000, 10000
					g.party.Members = []*character.MMCharacter{ch}
					g.cardSlots[0].key = "sunscarab_card"
					foe := &monster.Monster3D{ID: "first", Name: "First foe", HitPoints: 1000}
					enemyHit := func() {
						cs.monsterHitCharacter(foe, ch, foe.Name, monsterCharacterHit{Parts: damagecalc.Parts{Normal: 100}, DamageType: "fire", IgnoresArmor: true, IgnoresDodge: true})
					}
					enemyHit()
					if ch.ReactiveCombat.RepeatedHits != 1 {
						t.Fatal("enemy hit did not establish the chain")
					}
					shell, _ := config.GetWeaponDefinition("shellknocker")
					ch.ReactiveCombat.Shell = shell.HitShellAbsorption
					if restored {
						data, err := json.Marshal(buildCharacterSave(ch))
						if err != nil {
							t.Fatal(err)
						}
						var save CharacterSave
						if err := json.Unmarshal(data, &save); err != nil {
							t.Fatal(err)
						}
						ch = restoreCharacterSave(save)
						g.party.Members[0] = ch
					}
					state, hp := ch.ReactiveCombat, ch.HitPoints
					switch kind {
					case "terrain":
						g.damageEnvironment(config.EnvironmentEffect{Name: "Hot ground", Damage: 100, School: "fire", SecondarySchool: "air"}, 2)
					case "crate":
						g.springCrateTrap(&character.NPC{Name: "Trapped crate"}, &config.CrateConfig{TrapDamage: 100, TrapDamageTypes: []string{"fire"}})
					case "self_splash":
						def, err := spells.GetSpellDefinitionByID("inferno")
						if err != nil {
							t.Fatal(err)
						}
						if !cs.tryCastPartyNova(def, ch) {
							t.Fatal("self-splash cast was not handled")
						}
					}
					if ch.HitPoints >= hp || ch.HitPoints <= 0 {
						t.Fatalf("expected nonlethal damage, HP %d -> %d", hp, ch.HitPoints)
					}
					if ch.ReactiveCombat != state {
						t.Fatalf("source-less damage changed reactive protection: %+v -> %+v", state, ch.ReactiveCombat)
					}
					// The gate must not bypass the rest of the shared damage path.
					protector := character.CreateCharacter("Protector", character.ClassPaladin, g.config)
					protector.HitPoints, protector.MaxHitPoints = 1000, 1000
					protector.Skills[character.SkillSacrifice].Mastery = character.MasteryGrandMaster
					g.party.Members = append(g.party.Members, protector)
					before := ch.HitPoints
					cs.damagePartyMemberElement(0, ch, 100, "fire", false)
					if before-ch.HitPoints != 50 || protector.HitPoints != 950 || ch.ReactiveCombat != state {
						t.Fatal("source-less hit bypassed sacrifice or changed reactive protection")
					}
					g.party.Members = g.party.Members[:1]
					before = ch.HitPoints
					enemyHit()
					step := cardDef("sunscarab_card").CardRepeatedHitReductionPct
					if ch.ReactiveCombat.RepeatedHits != 2 || ch.ReactiveCombat.Shell != 0 || before-ch.HitPoints != 100-2*step-state.Shell {
						t.Fatalf("same enemy did not continue chain and consume shell: %+v", ch.ReactiveCombat)
					}
					foe = &monster.Monster3D{ID: "second", Name: "Second foe", HitPoints: 1000}
					enemyHit()
					if ch.ReactiveCombat.LastAttackerID != foe.ID || ch.ReactiveCombat.RepeatedHits != 1 {
						t.Fatalf("different enemy did not reset chain: %+v", ch.ReactiveCombat)
					}
				})
			}
		}
	}
}

func TestDuskneedleUnownedPreviewAtNight(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	weapon := items.CreateWeaponFromYAML("duskneedle")
	def, _ := config.GetWeaponDefinition("duskneedle")
	for _, night := range []bool{false, true} {
		cs.game.dayNightIsNight = night
		want := def.Damage
		if night {
			want *= def.NightBaseDamageMultiplier
		}
		text := buildWeaponTooltipUnified(weapon, nil, cs, true)
		if !strings.Contains(text, fmt.Sprintf("Base: %d", want)) || !strings.Contains(text, fmt.Sprintf("Total Damage: %d", want)) {
			t.Fatalf("night=%v: inconsistent tooltip: %s", night, text)
		}
	}
}

func TestArrowContinuationDoesNotSpendFreshDodgeCharges(t *testing.T) {
	for _, ricochet := range []bool{false, true} {
		t.Run(fmt.Sprint(ricochet), func(t *testing.T) {
			g, _, ch, tile := sniperFixture(t, false)
			isolateTrueDamageMember(ch, 0)
			g.cardSlots[0].key = "dusk_moth_card"
			m, next := partyDamageTargets(g, tile)
			key := "arbalest"
			if ricochet {
				key = "nest_arbalest"
			}
			def, _ := config.GetWeaponDefinition(key)
			if def == nil {
				t.Fatal("missing arbalest fixture")
			}
			shot := &Arrow{ID: "primary", Active: true, LifeTime: 60, Owner: ProjectileOwnerPlayer, Attacker: ch, BowKey: key, Damage: 100, DamageType: "physical", VelX: 10, PierceLeft: 1}
			if ricochet {
				shot.PierceLeft = 0
				shot.RicochetLeft = 1
			}
			ch.ReactiveCombat.DodgeCharges = 2
			g.combat.applyProjectileDamage(shot, "arrow", m, shot.ID)
			if ch.ReactiveCombat.DodgeCharges != 0 {
				t.Fatal("primary failed to consume charges")
			}
			if len(g.arrows) != 1 {
				t.Fatalf("expected a real continuation, got %d", len(g.arrows))
			}
			cont := g.arrows[0]
			ch.ReactiveCombat.DodgeCharges = 2
			before := next.HitPoints
			g.combat.applyProjectileDamage(&cont, "arrow", next, cont.ID)
			if ch.ReactiveCombat.DodgeCharges != 2 || before-next.HitPoints != 100 {
				t.Fatalf("continuation spent or used fresh charges: charges=%d damage=%d", ch.ReactiveCombat.DodgeCharges, before-next.HitPoints)
			}
		})
	}
}
