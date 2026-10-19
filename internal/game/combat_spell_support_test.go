package game

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"ugataima/internal/status"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

func TestPersonalSpellDamageSchoolReachesWeaponHits(t *testing.T) {
	for _, ranged := range []bool{false, true} {
		for _, saved := range []bool{false, true} {
			for _, immune := range []bool{false, true} {
				for _, critical := range []bool{false, true} {
					t.Run(fmt.Sprintf("ranged=%v/saved=%v/immune=%v/crit=%v", ranged, saved, immune, critical), func(t *testing.T) {
						g := advancedSupportGame(t, false)
						def, _ := config.GetSpellDefinition("fire_aura")
						previous := def.Mechanic.DamageSchool
						t.Cleanup(func() { def.Mechanic.DamageSchool = previous })
						def.Mechanic.DamageSchool = "water"
						c := g.party.Members[1]
						aura := castAdvancedSupport(t, g, "fire_aura", 0, c)
						if saved {
							c = restoreCharacterSave(buildCharacterSave(c))
							g.party.Members[1] = c
						}
						weapon := "iron_sword"
						if ranged {
							weapon = "hunting_bow"
						}
						c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(weapon)
						wd := lookupWeaponConfig(c.Equipment[items.SlotMainHand])
						oldCrit := wd.CritChance
						t.Cleanup(func() { wd.CritChance = oldCrit })
						wd.CritChance = 0
						if critical {
							wd.CritChance = 100
						}
						_, _, sourceDamage := g.combat.CalculateWeaponDamage(c.Equipment[items.SlotMainHand], c)
						if critical {
							sourceDamage *= 2
						}
						g.selectedChar = 1
						m := mkTestMonster("Authored aura target", 10000)
						m.ID, m.X = "authored-aura-target", float64(g.config.GetTileSize())
						m.Resistances = map[monsterPkg.DamageType]int{monsterPkg.DamagePhysical: 100, monsterPkg.DamageFire: 100}
						if immune {
							m.Resistances[monsterPkg.DamageWater] = 100
						}
						g.world.Monsters = []*monsterPkg.Monster3D{m}
						if !g.combat.EquipmentMeleeAttack() {
							t.Fatal("weapon action rejected")
						}
						if ranged {
							if len(g.arrows) != 1 {
								t.Fatalf("got %d arrows", len(g.arrows))
							}
							// A flying shot owns its payload even if the effect expires or
							// authored data is reloaded before impact.
							c.SpellState.Effects = nil
							def.Mechanic.DamageSchool = "fire"
							g.combat.applyProjectileDamage(&g.arrows[0], m, "")
						}
						want := sourceDamage * aura.Mechanic.Power(0) / 100
						if immune {
							want = 0
						}
						if got := 10000 - m.HitPoints; got != want {
							t.Fatalf("authored water aura damage=%d, want %d", got, want)
						}
					})
				}
			}
		}
	}
}

func TestDispelUsesAllHostileMagicalProjectileRoutes(t *testing.T) {
	for _, kind := range []string{"spell", "elemental_arrow", "physical_arrow"} {
		for _, owner := range []ProjectileOwner{ProjectileOwnerMonster, ProjectileOwnerMonsterAtActor, ProjectileOwnerPlayer, ProjectileOwnerPartyAlly, ProjectileOwnerReflected} {
			for _, state := range []string{"inside", "boundary", "outside", "expired", "inactive"} {
				t.Run(fmt.Sprintf("%s/owner=%d/%s", kind, owner, state), func(t *testing.T) {
					g := advancedSupportGame(t, false)
					def, _ := spells.GetSpellDefinitionByID("dispel_magic")
					limit := def.Mechanic.RadiusTiles * float64(g.config.GetTileSize())
					x, life, active := g.camera.X+limit/2, 60, true
					switch state {
					case "boundary":
						x = g.camera.X + limit
					case "outside":
						x = g.camera.X + limit + 1
					case "expired":
						life = 0
					case "inactive":
						active = false
					}
					const id = "dispel-test-shot"
					if kind == "spell" {
						g.magicProjectiles = []MagicProjectile{{ID: id, X: x, Y: g.camera.Y, LifeTime: life, Active: active, Owner: owner, SpellType: "flying_fist"}}
					} else {
						school := "fire"
						if kind == "physical_arrow" {
							school = "physical"
						}
						g.arrows = []Arrow{{ID: id, X: x, Y: g.camera.Y, LifeTime: life, Active: active, Owner: owner, DamageType: school}}
					}
					g.collisionSystem.RegisterEntity(collision.NewEntity(id, x, g.camera.Y, 1, 1, collision.CollisionTypeProjectile, false))
					eligible := kind != "physical_arrow" && (owner == ProjectileOwnerMonster || owner == ProjectileOwnerMonsterAtActor) && (state == "inside" || state == "boundary")
					if got := g.hostileMagicNearby(def.Mechanic.RadiusTiles); got != eligible {
						t.Fatalf("autocast usefulness=%v, want %v", got, eligible)
					}
					c := g.party.Members[0]
					c.LearnSpell(def.ID)
					sp := c.SpellPoints
					outcome := g.combat.castSpell(spellCastRequest{ID: def.ID, Definition: def, Caster: c, Cost: def.SpellPointsCost, PlayerInitiated: true})
					if eligible {
						if outcome != castCommitted || c.SpellPoints != sp-def.SpellPointsCost || g.collisionSystem.GetEntityByID(id) != nil {
							t.Fatal("eligible projectile did not commit and unregister exactly once")
						}
					} else if outcome != castRejected || c.SpellPoints != sp || g.collisionSystem.GetEntityByID(id) == nil {
						t.Fatal("ineligible projectile changed spell cost or collision registration")
					}
					if !eligible {
						// An independent cleansing reason commits Dispel, so preserved
						// shots are checked inside the removal path, not only its gate.
						g.party.Members[1].Conditions = []character.Condition{character.ConditionPoisoned}
						outcome = g.combat.castSpell(spellCastRequest{ID: def.ID, Definition: def, Caster: c, Cost: def.SpellPointsCost, PlayerInitiated: true})
						if outcome != castCommitted || g.collisionSystem.GetEntityByID(id) == nil {
							t.Fatal("committed cleanse removed an ineligible projectile")
						}
					}
					remainingActive := active
					if eligible {
						remainingActive = false
					}
					if kind == "spell" && g.magicProjectiles[0].Active != remainingActive || kind != "spell" && g.arrows[0].Active != remainingActive {
						t.Fatal("projectile activity disagrees with dispel eligibility")
					}
				})
			}
		}
	}
}

func advancedSupportGame(t *testing.T, tb bool) *MMGame {
	t.Helper()
	g := newTestCombatSystemWithConfig(t).game
	previousTechniques := config.GlobalTechniques
	t.Cleanup(func() { config.GlobalTechniques = previousTechniques })
	if err := config.LoadTechniques("../../assets/techniques.yaml"); err != nil {
		t.Fatal(err)
	}
	g.turnBasedMode = tb
	for _, c := range g.party.Members {
		isolateTrueDamageMember(c, 0)
		c.HitPoints, c.MaxHitPoints, c.SpellPoints, c.MaxSpellPoints = 1000, 1000, 1000, 1000
		c.Conditions = nil
		c.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{}
		c.ActionsRemaining, c.RTCooldown = 2, 0
	}
	return g
}

func castAdvancedSupport(t *testing.T, g *MMGame, key string, tier int, target *character.MMCharacter) spells.SpellDefinition {
	t.Helper()
	d, err := spells.GetSpellDefinitionByID(spells.SpellID(key))
	if err != nil {
		t.Fatal(err)
	}
	caster := g.party.Members[0]
	caster.LearnSpell(d.ID)
	caster.MagicSchools[caster.SpellSchoolFor(d)].Mastery = character.SkillMastery(tier)
	previous := g.combat.spellTarget
	g.combat.spellTarget = target
	defer func() { g.combat.spellTarget = previous }()
	before := caster.SpellPoints
	if outcome := g.combat.castSpell(spellCastRequest{ID: d.ID, Definition: d, Caster: caster, Cost: d.SpellPointsCost, PlayerInitiated: true}); outcome != castCommitted {
		t.Fatalf("%s outcome=%v, want committed", key, outcome)
	}
	if caster.SpellPoints != before-d.SpellPointsCost {
		t.Fatalf("%s charged SP incorrectly: %d -> %d", key, before, caster.SpellPoints)
	}
	return d
}

func TestAdvancedSharedLifeScalesAndRedistributesThroughCast(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for tier := 0; tier < 4; tier++ {
			for _, personality := range []int{0, 61} {
				t.Run(fmt.Sprintf("TB=%v/tier=%d/PER=%d", tb, tier, personality), func(t *testing.T) {
					g := advancedSupportGame(t, tb)
					c := g.party.Members[0]
					c.Intellect, c.Personality = 399, personality
					for i, hp := range []int{10, 50, 50, 0} {
						g.party.Members[i].HitPoints = hp
					}
					c.MaxHitPoints = 20
					d := castAdvancedSupport(t, g, "shared_life", tier, nil)
					bonus := d.HealAmount + personality/2 + tier*5
					pool := 110 + bonus - 20
					want := []int{20, pool / 2, pool - pool/2, 0}
					for i, hp := range want {
						if got := g.party.Members[i].HitPoints; got != hp {
							t.Fatalf("hero %d HP=%d, want %d (pool=%d)", i, got, hp, 110+bonus)
						}
					}
				})
			}
		}
	}
}

func TestAdvancedMagicWardSharesCastBudget(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, route := range []string{"fireburst", "inferno"} {
			for tier := 0; tier < 4; tier++ {
				t.Run(fmt.Sprintf("TB=%v/%s/tier=%d", tb, route, tier), func(t *testing.T) {
					g := advancedSupportGame(t, tb)
					d := castAdvancedSupport(t, g, "protection_from_magic", tier, nil)
					m := mkTestMonster("Ward tester", 10000)
					attack := func() { castTestPartyAttack(g.combat, m, catalogPartyAttack(route), 100) }
					budget := d.Mechanic.Charges(tier)
					for cast := 0; cast < budget; cast++ {
						attack()
						for slot, c := range g.party.Members {
							if c.HitPoints != 1000 {
								t.Fatalf("cast %d bypassed shared ward for hero %d: HP=%d", cast, slot, c.HitPoints)
							}
						}
						if b := g.advancedBuff(config.MechanicProtectionFromMagic); cast+1 < budget && (b == nil || b.Charges != budget-cast-1) {
							t.Fatalf("one AoE must spend exactly one shared charge, got %+v", b)
						}
						// Exercise the real saved-buff representation after partial use.
						g.combatBuffs = restoreCombatBuffs(buildCombatBuffSaves(g.combatBuffs))
					}
					attack()
					for slot, c := range g.party.Members {
						if c.HitPoints >= 1000 {
							t.Fatalf("exhausted ward still protects hero %d", slot)
						}
					}
				})
			}
		}
	}
}

func TestAdvancedPreservationSharedPoolNeverHeals(t *testing.T) {
	for tier := 0; tier < 4; tier++ {
		t.Run(fmt.Sprint(tier), func(t *testing.T) {
			g := advancedSupportGame(t, tier%2 == 1) // RT and TB alike
			for _, c := range g.party.Members {
				c.HitPoints = 13
			}
			d := castAdvancedSupport(t, g, "preservation", tier, nil)
			for _, c := range g.party.Members {
				if c.HitPoints != 13 {
					t.Fatal("Preservation healed on cast")
				}
			}
			for i := 0; i < d.Mechanic.Charges(tier); i++ {
				slot := i % len(g.party.Members)
				c := g.party.Members[slot]
				c.ApplyPoison(1200)
				c.BurnFramesRemaining = 1200
				c.AddCondition(character.ConditionBurning)
				g.combat.damagePartyMemberElement(slot, c, 9999, "physical", false)
				if c.PoisonFramesRemaining != 0 || c.BurnFramesRemaining != 0 || c.HasCondition(character.ConditionPoisoned) || c.HasCondition(character.ConditionBurning) {
					t.Fatal("Preservation left damage over time running")
				}

				if c.HitPoints != 1 || c.HasCondition(character.ConditionUnconscious) {
					t.Fatalf("shared save %d left HP=%d, conditions=%v", i, c.HitPoints, c.Conditions)
				}
				// The last charge ends the buff at once, off the status rail too.
				if spent := i == d.Mechanic.Charges(tier)-1; spent && (len(g.combatBuffs) != 0 || g.buffStatuses["preservation"] != nil) {
					t.Fatalf("spent Preservation stayed: %+v", g.combatBuffs)
				}
				g.combatBuffs = restoreCombatBuffs(buildCombatBuffSaves(g.combatBuffs))
			}
			c := g.party.Members[0]
			g.combat.damagePartyMemberElement(0, c, 9999, "physical", false)
			if c.HitPoints != 0 || !c.HasCondition(character.ConditionUnconscious) {
				t.Fatal("exhausted Preservation still prevents death")
			}
		})
	}
}

func TestAdvancedPersonalSpellsStackAndFollowWeaponAction(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprint(tb), func(t *testing.T) {
			g := advancedSupportGame(t, tb)
			c := g.party.Members[1]
			c.HitPoints = 50
			c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
			weapon := c.Equipment[items.SlotMainHand]
			baseCrit := g.combat.CalculateWeaponCritChance(weapon, c)
			aura := castAdvancedSupport(t, g, "fire_aura", 0, c)
			fate := castAdvancedSupport(t, g, "fate", 0, c)
			vamp := castAdvancedSupport(t, g, "vampiric_weapon", 0, c)
			if len(c.SpellState.Effects) != 3 || len(g.party.Members[0].SpellState.Effects) != 0 {
				t.Fatal("targeted effects overwrote a sibling or landed on the caster")
			}
			if got := g.combat.CalculateWeaponCritChance(weapon, c); got != baseCrit+fate.Mechanic.Power(0) {
				t.Fatalf("Fate crit=%d, want base %d plus %d", got, baseCrit, fate.Mechanic.Power(0))
			}
			c.Luck = 10000 // Deterministic critical, including the aura's percentage.
			_, _, sourceDamage := g.combat.CalculateWeaponDamage(weapon, c)
			wantAura := 2 * sourceDamage * aura.Mechanic.Power(0) / 100
			m := mkTestMonster("Fire-only target", 10000)
			m.ID, m.X, m.Y = "personal-spell-target", float64(g.config.GetTileSize()), 0
			m.Resistances = map[monsterPkg.DamageType]int{monsterPkg.DamagePhysical: 100}
			g.world.Monsters = []*monsterPkg.Monster3D{m}
			g.selectedChar = 1
			if !g.combat.EquipmentMeleeAttack() {
				t.Fatal("equipped weapon action rejected")
			}
			if dealt := 10000 - m.HitPoints; dealt != wantAura {
				t.Fatalf("Fire Aura must retain separate fire resistance channel: dealt=%d, want %d", dealt, wantAura)
			}
			wantHP := 50 + wantAura*vamp.Mechanic.Power(0)/100
			if c.HitPoints != wantHP {
				t.Fatalf("Vampiric Weapon HP=%d, want %d", c.HitPoints, wantHP)
			}
			for _, e := range c.SpellState.Effects {
				if (e.SpellID == "fire_aura" || e.SpellID == "fate") && e.Charges != 1 {
					t.Fatalf("one committed action must spend one charge: %+v", e)
				}
			}
			raw, err := json.Marshal(buildCharacterSave(c))
			if err != nil {
				t.Fatal(err)
			}
			var saved CharacterSave
			if err := json.Unmarshal(raw, &saved); err != nil {
				t.Fatal(err)
			}
			restored := restoreCharacterSave(saved)
			if !reflect.DeepEqual(restored.SpellState, c.SpellState) {
				t.Fatalf("save/load changed the remaining personal budgets: %+v vs %+v", restored.SpellState, c.SpellState)
			}
		})
	}
}

func TestAdvancedCleanseAutocastTransaction(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, key := range []string{"body_purify", "dispel_magic"} {
			for _, blocked := range []string{"none", "checkbox", "unknown", "SP", "AP", "overlay", "guard", "stunned", "monster_phase", "focus"} {
				t.Run(fmt.Sprintf("TB=%v/%s/%s", tb, key, blocked), func(t *testing.T) {
					g := advancedSupportGame(t, tb)
					g.config.AutocastIntervalRounds = 3
					c, patient := g.party.Members[0], g.party.Members[1]
					d, err := spells.GetSpellDefinitionByID(spells.SpellID(key))
					if err != nil {
						t.Fatal(err)
					}
					if blocked != "unknown" {
						c.LearnSpell(d.ID)
					}
					c.SpellState.Automatic = map[string]bool{key: true}
					g.selectedChar = 1
					patient.Conditions = []character.Condition{character.ConditionPoisoned}
					switch blocked {
					case "stunned":
						c.AddCondition(character.ConditionStunned)
						c.Stun.Frames = 100
					case "monster_phase":
						if !tb {
							t.Skip("RT has no monster phase")
						}
						g.currentTurn = 1
					case "focus":
						if tb {
							t.Skip("focus applies only in RT")
						}
						g.focusedPartyMask = 2
					case "checkbox":
						c.SpellState.Automatic[key] = false
					case "SP":
						c.SpellPoints = 0
					case "AP":
						c.ActionsRemaining, c.RTCooldown = 0, 100
					case "overlay":
						g.menuOpen = true
					case "guard":
						c.AutocastCD = framesClock(100)
					}
					ap, sp, otherAP, recovery := c.ActionsRemaining, c.SpellPoints, patient.ActionsRemaining, c.RTCooldown
					(&GameLoop{game: g}).updateAutomaticTechniques()
					if blocked != "none" && blocked != "AP" {
						if c.ActionsRemaining != ap || c.SpellPoints != sp || !patient.Purifiable() {
							t.Fatal("blocked spell autocast changed state")
						}
						return
					}
					if patient.Purifiable() || c.SpellPoints != sp-d.SpellPointsCost || c.AutocastCD.Frames != g.config.GetTPS()*9 {
						t.Fatal("automatic spell cleanse did not commit exactly once")
					}
					if c.ActionsRemaining != ap || c.RTCooldown != recovery || g.selectedChar != 1 || patient.ActionsRemaining != otherAP {
						t.Fatal("automatic spell charged the wrong action or stole selection")
					}
					if g.actionInputStagger != 0 {
						t.Fatal("automatic spell blocked manual input")
					}
					if g.partyPenaltyFreeActionsUsed != 0 || g.partyActionsUsed != 0 {
						t.Fatal("automatic cleanse must not count as an action")
					}
				})
			}
		}
	}
}

func TestAdvancedVampirismHealsActualDodgeDamage(t *testing.T) {
	for _, ranged := range []bool{false, true} {
		for _, lethal := range []bool{false, true} {
			t.Run(fmt.Sprintf("ranged=%v/lethal=%v", ranged, lethal), func(t *testing.T) {
				g := advancedSupportGame(t, false)
				c := g.party.Members[0]
				c.HitPoints = 10
				d := castAdvancedSupport(t, g, "vampiric_weapon", 3, c)
				g.cardSlots = [MaxCardSlots]cardSlot{}
				g.cardSlots[0].key = "samurai_card"
				trueDamage := cardItemDef(t, "samurai_card").CardMeleeTrueDmg
				if ranged {
					trueDamage = 100
				}
				if trueDamage*d.Mechanic.Power(3)/100 == 0 {
					t.Fatal("fixture must produce measurable vampiric healing")
				}
				m := mkTestMonster("Perfect dodger", 1000)
				m.PerfectDodge = 100
				if lethal {
					m.HitPoints = trueDamage / 2
				}
				before := m.HitPoints
				if ranged {
					shot := &Arrow{ID: "vampire-dodge", Active: true, Owner: ProjectileOwnerPlayer, LifeTime: 60, Attacker: c, BowKey: "hunting_bow", Damage: 100, TrueDamage: trueDamage, DamageType: "physical", VampiricPct: d.Mechanic.Power(3)}
					g.combat.applyProjectileDamage(shot, m, shot.ID)
				} else {
					g.combat.ApplyDamageToMonster(m, 100, testWeapon("Iron Sword"), false)
				}
				lost := before - m.HitPoints
				if lost != min(before, trueDamage) {
					t.Fatalf("true damage through dodge=%d, want %d", lost, min(before, trueDamage))
				}
				if got, want := c.HitPoints, 10+lost*d.Mechanic.Power(3)/100; got != want {
					t.Fatalf("vampiric HP=%d, want %d from actual lost HP", got, want)
				}
			})
		}
	}
}

func TestAdvancedTechniqueSpellRetreatAccounting(t *testing.T) {
	for _, key := range []string{"haste", "body_purify", "dispel_magic"} {
		t.Run(key, func(t *testing.T) {
			g := advancedSupportGame(t, true)
			g.party.Members[1].Conditions = []character.Condition{character.ConditionPoisoned}
			castAdvancedSupport(t, g, key, 0, nil)
			g.commitHeroAction(g.selectedChar, 0)
			if g.partyActionsUsed != 1 || g.partyPenaltyFreeActionsUsed != 1 {
				t.Fatal("manual technique alternative counted as an ordinary offensive cast")
			}
			g.endPartyTurnAfterMovement()
			if g.turnBasedExtraMonsterAction {
				t.Fatal("retreat after a technique spell granted an extra monster action")
			}
		})
	}
}

func TestAdvancedPersonalTimersSurviveModeSwitchAndSave(t *testing.T) {
	g := advancedSupportGame(t, false)
	c := g.party.Members[1]
	d := castAdvancedSupport(t, g, "vampiric_weapon", 0, c)
	castAdvancedSupport(t, g, "fire_aura", 0, c)
	remaining := d.DurationSeconds(0) * g.config.GetTPS()
	assertRemaining := func(want int) {
		t.Helper()
		found := false
		for _, e := range c.SpellState.Effects {
			if e.SpellID == string(d.ID) {
				found = true
				if e.Clock.Frames != want {
					t.Fatalf("remaining=%d, want %d", e.Clock.Frames, want)
				}
			}
		}
		if found != (want > 0) {
			t.Fatalf("timed effect presence=%v for budget %d", found, want)
		}
	}
	passFrames(g, 1)
	remaining--
	assertRemaining(remaining)
	g.turnBasedMode = true
	g.startPartyTurn(true)
	assertRemaining(remaining)
	g.startPartyTurn()
	remaining -= g.combatRoundFrames()
	assertRemaining(remaining)
	restored := restoreCharacterSave(buildCharacterSave(c))
	g.party.Members[1], c = restored, restored
	assertRemaining(remaining)
	g.turnBasedMode = false
	passFrames(g, remaining-1)
	assertRemaining(1)
	passFrames(g, 1)
	assertRemaining(0)
	if len(c.SpellState.Effects) != 1 || c.SpellState.Effects[0].SpellID != "fire_aura" {
		t.Fatal("elapsed time also expired an attack-charge effect")
	}
}

func TestAutocastDuplicateCasters(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, key := range []string{"purify", "phase_veil", "quickening", "body_purify", "dispel_magic", "haste", "celestial_armor"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, key), func(t *testing.T) {
				g, first := rareClassGame(t, character.ClassWayfarer, tb)
				if config.Technique(key) != nil {
					first.RareClass.Automatic = map[string]bool{key: true}
				} else {
					first.LearnSpell(spells.SpellID(key))
					first.SpellState.Automatic = map[string]bool{key: true}
				}
				second := restoreCharacterSave(buildCharacterSave(first))
				g.party.Members[1] = second
				patient := g.party.Members[2]
				patient.HitPoints = 1
				patient.AddCondition(character.ConditionPoisoned)
				m := zoneVictim(t, g)
				m.X = g.camera.X + 32
				m.BeginPlayerEngagement()
				gl := &GameLoop{game: g}
				sp1, sp2 := first.SpellPoints, second.SpellPoints
				gl.updateAutomaticTechniques()
				paid := first.SpellPoints
				if paid >= sp1 || first.AutocastCD.Frames != g.config.AutocastIntervalRounds*g.combatRoundFrames() {
					t.Fatal("first eligible caster did not commit with the authored autocast interval")
				}
				gl.updateAutomaticTechniques()
				if first.SpellPoints != paid || second.SpellPoints != sp2 || second.AutocastCD.Frames != 0 {
					t.Fatal("duplicate autocast spent mana or locked the standby caster")
				}
				if first.ActionsRemaining != 2 || second.ActionsRemaining != 2 || first.RTCooldown != 0 || second.RTCooldown != 0 {
					t.Fatal("automatic cast changed actions or recovery")
				}
			})
		}
	}
}

func TestAutocastBudgetAcrossRoundsModesAndSave(t *testing.T) {
	for _, clock := range []string{"TB", "RT", "mixed"} {
		t.Run(clock, func(t *testing.T) {
			g, c := rareClassGame(t, character.ClassWayfarer, clock == "TB")
			// The scenario's own two-round budget: round 2 is still blocked,
			// round 3 is ready.
			g.config.AutocastIntervalRounds = 2
			budget := 2 * g.combatRoundFrames()
			c.RareClass.Automatic = map[string]bool{"purify": true}
			patient := g.party.Members[1]
			patient.AddCondition(character.ConditionPoisoned)
			gl := &GameLoop{game: g}
			gl.updateAutomaticTechniques()
			if c.AutocastCD.Frames != budget {
				t.Fatal("commit did not start the two-round budget")
			}
			// A different automatic ability uses the same personal lock. Manual
			// spells remain usable during it and do not reset the lock.
			c.RareClass.Automatic = nil
			c.LearnSpell("celestial_armor")
			c.SpellState.Automatic = map[string]bool{"celestial_armor": true}
			m := zoneVictim(t, g)
			m.X = g.camera.X + 32
			m.BeginPlayerEngagement()
			if !g.useTechnique(0, "quickening", false, false) {
				t.Fatal("autocast budget blocked manual technique")
			}
			if g.turnBasedMode && c.ActionsRemaining != 1 {
				t.Fatal("manual technique lost its normal action cost")
			}
			g.actionInputStagger = 0
			sp := c.SpellPoints
			gl.updateAutomaticTechniques()
			if c.SpellPoints != sp || c.AutocastCD.Frames != budget {
				t.Fatal("manual use changed the automatic budget or a sibling bypassed it")
			}
			switch clock {
			case "TB":
				g.startPartyTurn() // Round 2: still blocked.
			case "RT":
				passFrames(g, g.combatRoundFrames())
			case "mixed":
				passFrames(g, g.config.GetTPS())
				g.turnBasedMode = true
				g.startPartyTurn(true) // Entering TB grants no elapsed time.
				if c.AutocastCD.Frames != budget-g.config.GetTPS() {
					t.Fatal("mode entry shortened the automatic budget")
				}
				g.startPartyTurn()
			}
			gl.updateAutomaticTechniques()
			if c.SpellPoints != sp {
				t.Fatal("autocast retriggered before round 3 / the full budget")
			}
			remaining := c.AutocastCD.Frames
			c = restoreCharacterSave(buildCharacterSave(c))
			g.party.Members[0] = c
			if c.AutocastCD.Frames != remaining {
				t.Fatal("save/load changed the remaining automatic budget")
			}
			if clock == "TB" {
				g.startPartyTurn() // Round 3: ready.
			} else {
				g.turnBasedMode = false
				passFrames(g, remaining-1)
				gl.updateAutomaticTechniques()
				if c.SpellPoints != sp {
					t.Fatal("autocast became ready a frame early")
				}
				passFrames(g, 1)
			}
			gl.updateAutomaticTechniques()
			armor, err := spells.GetSpellDefinitionByID("celestial_armor")
			if err != nil {
				t.Fatal(err)
			}
			if c.SpellPoints != sp-armor.SpellPointsCost || g.advancedBuff(config.MechanicCelestialArmor) == nil {
				t.Fatal("autocast did not resume at the exact budget boundary")
			}
		})
	}
}

// Each automatic pass commits one caster's spell, so two heroes with
// different autocasts both cast within two passes, each paying only for
// their own spell.
func TestAutocastCastersCommitOnConsecutivePasses(t *testing.T) {
	g := advancedSupportGame(t, true)
	m := zoneVictim(t, g)
	m.X = g.camera.X + 32
	m.BeginPlayerEngagement()
	keys := []string{"haste", "celestial_armor"}
	for i, key := range keys {
		c := g.party.Members[i]
		c.LearnSpell(spells.SpellID(key))
		c.SpellState.Automatic = map[string]bool{key: true}
	}
	gl := &GameLoop{game: g}
	gl.updateAutomaticTechniques()
	gl.updateAutomaticTechniques()
	for i, key := range keys {
		d, err := spells.GetSpellDefinitionByID(spells.SpellID(key))
		if err != nil {
			t.Fatal(err)
		}
		if got := g.party.Members[i].SpellPoints; got != 1000-d.SpellPointsCost {
			t.Fatalf("%s caster SP=%d, want %d", key, got, 1000-d.SpellPointsCost)
		}
	}
	if g.party.Members[2].SpellPoints != 1000 || g.advancedBuff(config.MechanicCelestialArmor) == nil {
		t.Fatal("second autocast did not apply or charged a bystander")
	}
}

// Shield wards ranged weapon hits and Pain Reflection answers attacks. Neither
// treats an authored hazard (the Brood Mother's brood-fire trap) as an attack.
func TestShieldAndPainReflectionSkipHazards(t *testing.T) {
	for _, buff := range []string{"shield", "pain_reflection"} {
		for _, route := range []string{"melee", "weapon projectile", "fireburst", "boss trap"} {
			t.Run(buff+"/"+route, func(t *testing.T) {
				strike := func(warded bool) (received, reflected, pct int) {
					g := advancedSupportGame(t, false)
					g.party.Members = g.party.Members[:1]
					ch := g.party.Members[0]
					if warded {
						castAdvancedSupport(t, g, buff, int(character.MasteryGrandMaster), nil)
						b := g.advancedBuff(config.MechanicKind(buff))
						pct = b.RangedWardPct + b.ReflectionPct
					}
					m := mkTestMonster("Attacker", 1000)
					armTestTrapVolley(m, 100)
					switch route {
					case "melee", "weapon projectile":
						hit := hitFromMonster(m, 100, "physical", false, 0, route == "melee", false)
						hit.Projectile = route == "weapon projectile"
						g.combat.monsterHitCharacter(m, ch, m.Name, hit)
					case "fireburst":
						castTestPartyAttack(g.combat, m, catalogPartyAttack(route), 100)
					case "boss trap":
						g.detonateBossFireTrap(m)
					}
					return ch.MaxHitPoints - ch.HitPoints, 1000 - m.HitPoints, pct
				}
				base, _, _ := strike(false)
				received, reflected, pct := strike(true)
				if base <= 0 || pct <= 0 {
					t.Fatalf("fixture: base hit %d, %s power %d", base, buff, pct)
				}
				wantReceived, wantReflected := base, 0
				if buff == "shield" && route == "weapon projectile" {
					wantReceived = base * (100 - pct) / 100
				}
				if buff == "pain_reflection" && route != "boss trap" {
					wantReflected = received * pct / 100
				}
				if received != wantReceived || reflected != wantReflected {
					t.Fatalf("received %d, reflected %d; want %d and %d", received, reflected, wantReceived, wantReflected)
				}
			})
		}
	}
}

// Heal triage (C key, Space) walks one chain: a heal that mends every member
// (Mass Heal), the shared pool (Shared Life), one ally (Heal), and the
// caster's First Aid only when it is the caster's one heal. A heal under its
// reuse lockout or past the purse hands over to the next; Space prefers the
// quick-slotted heal, but a slotted First Aid waits like the book's.
func TestHealTriageChain(t *testing.T) {
	for _, tc := range []struct {
		name           string
		space          bool // Space (SmartAttack) instead of the C key
		known, reusing []string
		slotted        string // Space: the quick-slotted spell
		short          bool   // SP one short of the first heal of the chain
		want           string // "" = no heal is cast
	}{
		{name: "party heal leads", known: []string{"heal_other", "mass_heal", "shared_life"}, want: "mass_heal"},
		{name: "the pool outranks one ally", known: []string{"heal_other", "shared_life"}, want: "shared_life"},
		{name: "Mass Heal resting hands to the pool", known: []string{"mass_heal", "shared_life", "heal_other"}, reusing: []string{"mass_heal"}, want: "shared_life"},
		{name: "both party heals resting hand to Heal", known: []string{"mass_heal", "shared_life", "heal_other"}, reusing: []string{"mass_heal", "shared_life"}, want: "heal_other"},
		{name: "a short purse hands to a cheaper heal", known: []string{"mass_heal", "heal_other"}, short: true, want: "heal_other"},
		{name: "Heal beats First Aid", known: []string{"heal", "heal_other"}, want: "heal_other"},
		{name: "First Aid waits while another heal rests", known: []string{"heal", "mass_heal"}, reusing: []string{"mass_heal"}, want: ""},
		{name: "First Aid serves a caster with no other heal", known: []string{"heal"}, want: "heal"},
		{name: "Space: the slotted heal leads", space: true, known: []string{"mass_heal", "heal_other"}, slotted: "heal_other", want: "heal_other"},
		{name: "Space: a slotted First Aid waits for the chain", space: true, known: []string{"heal", "heal_other"}, slotted: "heal", want: "heal_other"},
		{name: "Space: a short purse hands to a cheaper heal", space: true, known: []string{"mass_heal", "heal_other"}, short: true, want: "heal_other"},
		{name: "Space: First Aid stays out while Mass Heal rests", space: true, known: []string{"heal", "mass_heal"}, reusing: []string{"mass_heal"}, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := advancedSupportGame(t, false)
			caster := g.party.Members[0]
			for _, key := range tc.known {
				caster.LearnSpell(spells.SpellID(key))
			}
			caster.SpellState.Reuse = map[string]status.DualClock{}
			for _, key := range tc.reusing {
				caster.SpellState.Reuse[key] = framesClock(1)
			}
			if tc.short {
				first, err := spells.GetSpellDefinitionByID(spells.SpellID(tc.known[0]))
				if err != nil {
					t.Fatal(err)
				}
				caster.SpellPoints = caster.MeditatedSPCost(first.SpellPointsCost) - 1
			}
			if tc.slotted != "" {
				caster.Equipment[items.SlotQuickAction] = items.Item{Type: items.ItemUtilitySpell, SpellEffect: items.SpellEffect(tc.slotted)}
			}
			// The caster and an ally are both wounded, so a self-only heal has
			// its target too.
			for _, m := range g.party.Members[:2] {
				m.HitPoints = m.MaxHitPoints / 2
			}
			g.selectedChar = 0
			var cast bool
			var id spells.SpellID
			if tc.space {
				id = g.combat.SmartAttack().Spell
				cast = id != ""
			} else {
				cast, id = g.combat.CastBestHeal()
			}
			if cast != (tc.want != "") || string(id) != tc.want {
				t.Fatalf("triage cast=%v %q, want %q", cast, id, tc.want)
			}
		})
	}
}

// Mass Heal's own cast starts its personal reuse lockout (9s, three TB rounds);
// the very next triage heals with Heal Other instead of refusing.
func TestMassHealReuseHandsTriageOver(t *testing.T) {
	g := advancedSupportGame(t, false)
	caster := g.party.Members[0]
	caster.LearnSpell("heal_other")
	caster.LearnSpell("mass_heal")
	caster.SpellPoints = caster.MaxSpellPoints
	g.selectedChar = 0
	for _, want := range []string{"mass_heal", "heal_other"} {
		g.party.Members[1].HitPoints = g.party.Members[1].MaxHitPoints / 2
		caster.RTCooldown, caster.ActionsRemaining = 0, 1
		if cast, id := g.combat.CastBestHeal(); !cast || string(id) != want {
			t.Fatalf("triage cast=%v %q, want %q", cast, id, want)
		}
	}
	if got := caster.SpellState.Reuse["mass_heal"].Frames; got != 9*g.config.GetTPS() {
		t.Fatalf("Mass Heal reuse = %d frames, want 9s", got)
	}
}

// A charge-only party buff (Preservation, Protection from Magic) has no timer.
// After a load its HUD icon reads "until spent" exactly as after the cast.
func TestChargeOnlyBuffIconSurvivesLoad(t *testing.T) {
	for _, key := range []string{"preservation", "protection_from_magic"} {
		t.Run(key, func(t *testing.T) {
			g := advancedSupportGame(t, false)
			castAdvancedSupport(t, g, key, 1, nil)
			cast := g.buffStatuses[key]
			if cast == nil {
				t.Fatal("fixture: the cast shows no icon")
			}
			want := *cast
			raw, err := json.Marshal(GameSave{CombatBuffs: buildCombatBuffSaves(g.combatBuffs)})
			if err != nil {
				t.Fatal(err)
			}
			var save GameSave
			if err := json.Unmarshal(raw, &save); err != nil {
				t.Fatal(err)
			}
			g.combatBuffs, g.buffStatuses = nil, nil
			g.restoreSavedEffects(&save)
			g.restoreSavedEffectPresentation(nil, &save)
			got := g.buffStatuses[key]
			if got == nil || got.Duration != want.Duration {
				t.Fatalf("restored icon %+v, want %+v", got, want)
			}
		})
	}
}

// A Pixie Card swing becomes a free Fire Bolt: no weapon strike lands, so the
// hero keeps Fate and Fire Aura charges. A real swing spends one of each.
func TestPixieCardSwingKeepsPersonalCharges(t *testing.T) {
	g := advancedSupportGame(t, false)
	c := g.party.Members[1]
	c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
	castAdvancedSupport(t, g, "fire_aura", 0, c)
	castAdvancedSupport(t, g, "fate", 0, c)
	charges := func() map[string]int {
		out := map[string]int{}
		for _, e := range c.SpellState.Effects {
			out[e.SpellID] = e.Charges
		}
		return out
	}
	original := config.GlobalItems.Items["pixie_card"]
	always := *original
	always.CardSpellProcPct = 100
	config.GlobalItems.Items["pixie_card"] = &always
	t.Cleanup(func() { config.GlobalItems.Items["pixie_card"] = original })
	g.cardSlots[0].key = "pixie_card"
	g.selectedChar = 1
	before := charges()
	if !g.combat.EquipmentMeleeAttack() || len(g.magicProjectiles) == 0 {
		t.Fatal("fixture: the swing did not become a Fire Bolt")
	}
	if got := charges(); !reflect.DeepEqual(got, before) {
		t.Fatalf("a card spell spent weapon charges: %v -> %v", before, got)
	}
	g.cardSlots[0].key = ""
	c.RTCooldown = 0
	if !g.combat.EquipmentMeleeAttack() {
		t.Fatal("real swing rejected")
	}
	after := charges()
	if len(after) != len(before) || len(before) != 2 {
		t.Fatalf("fixture: charges %v -> %v, want both effects to outlive one swing", before, after)
	}
	for id, n := range after {
		if n != before[id]-1 {
			t.Fatalf("%s charges %d after a real swing, want %d", id, n, before[id]-1)
		}
	}
}

// Autocast suppression never locks manual use. Legacy cleansing timers are
// ignored on restore, and manual use cannot restart the automatic budget.
func TestAutocastManualReuseAndLegacySave(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, key := range []string{"purify", "phase_veil", "quickening", "body_purify", "dispel_magic", "haste", "celestial_armor"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, key), func(t *testing.T) {
				g, c := rareClassGame(t, character.ClassWayfarer, tb)
				c.SpellPoints, c.MaxSpellPoints, c.ActionsRemaining = 1000, 1000, 3
				technique := config.Technique(key)
				cost := 0
				if technique != nil {
					c.RareClass.Automatic = map[string]bool{key: true}
					cost = g.techniqueSPCost(c, technique)
				} else {
					id := spells.SpellID(key)
					c.LearnSpell(id)
					c.SpellState.Automatic = map[string]bool{key: true}
					d, err := spells.GetSpellDefinitionByID(id)
					if err != nil {
						t.Fatal(err)
					}
					cost = c.MeditatedSPCost(d.SpellPointsCost)
					c.Equipment[items.SlotQuickAction] = items.Item{Type: items.ItemUtilitySpell, SpellEffect: items.SpellEffect(id), SpellCost: d.SpellPointsCost}
				}
				patient := g.party.Members[1]
				patient.HitPoints = 1
				patient.AddCondition(character.ConditionPoisoned)
				m := zoneVictim(t, g)
				m.X = g.camera.X + 32
				m.BeginPlayerEngagement()
				gl := &GameLoop{game: g}
				gl.updateAutomaticTechniques()
				if c.SpellPoints != 1000-cost || c.AutocastCD.Frames <= 0 || c.ActionsRemaining != 3 || c.RTCooldown != 0 {
					t.Fatal("autocast did not pay only mana and start its automatic budget")
				}
				raw, err := json.Marshal(buildCharacterSave(c))
				if err != nil {
					t.Fatal(err)
				}
				var legacy map[string]json.RawMessage
				if err = json.Unmarshal(raw, &legacy); err != nil {
					t.Fatal(err)
				}
				for _, field := range []struct{ state, timer string }{{"rare_class", "purify_frames"}, {"spell_state", "cleanse_frames"}} {
					var state map[string]json.RawMessage
					if err = json.Unmarshal(legacy[field.state], &state); err != nil {
						t.Fatal(err)
					}
					state[field.timer] = json.RawMessage("9999")
					// Older saves kept the budget in the rare-class (technique) or spell state.
					if (field.state == "rare_class") == (technique != nil) {
						state["auto_frames"] = json.RawMessage(fmt.Sprint(c.AutocastCD.Frames))
					}
					legacy[field.state], err = json.Marshal(state)
					if err != nil {
						t.Fatal(err)
					}
				}
				delete(legacy, "autocast_frames")
				raw, err = json.Marshal(legacy)
				if err != nil {
					t.Fatal(err)
				}
				var saved CharacterSave
				if err = json.Unmarshal(raw, &saved); err != nil {
					t.Fatal(err)
				}
				budget := c.AutocastCD.Frames
				c = restoreCharacterSave(saved)
				g.party.Members[0] = c
				c.ActionsRemaining, c.RTCooldown = 3, 0
				// Remove the active effect so every manual cast has something to do.
				g.combatBuffs = nil
				count := 1
				if key == "purify" || key == "body_purify" || key == "dispel_magic" {
					count = 2
				}
				for n := 0; n < count; n++ {
					patient.AddCondition(character.ConditionPoisoned)
					g.selectedChar, g.actionInputStagger, c.RTCooldown = 0, 0, 0
					ap, sp := c.ActionsRemaining, c.SpellPoints
					if technique != nil {
						if !g.useTechnique(0, key, false, false) {
							t.Fatal("manual technique rejected during automatic suppression")
						}
					} else {
						ih := &InputHandler{game: g}
						if tb {
							fired, recovery := ih.useSlottedActionResolved(c)
							if !fired {
								t.Fatal("manual spell rejected during automatic suppression")
							}
							g.commitHeroAction(g.selectedChar, recovery)
						} else {
							ih.useSlottedAction(c)
						}
					}
					// Recovery uses the newly applied Haste effect, just like input.
					cooldown := g.combat.SpellCooldownFrames(c, spells.SpellID(key))
					if technique != nil {
						cooldown = g.techniqueCooldown(c, technique)
					}
					wantAP := ap
					if tb {
						wantAP--
					}
					if c.SpellPoints != sp-cost || c.ActionsRemaining != wantAP || c.RTCooldown != cooldown || c.AutocastCD.Frames != budget {
						t.Fatalf("manual use: SP=%d/%d AP=%d/%d recovery=%d/%d automatic=%d/%d", c.SpellPoints, sp-cost, c.ActionsRemaining, wantAP, c.RTCooldown, cooldown, c.AutocastCD.Frames, budget)
					}
					if count == 2 && patient.Purifiable() {
						t.Fatal("manual cleanse left the affliction")
					}
				}
				g.combatBuffs = nil
				patient.AddCondition(character.ConditionPoisoned)
				g.actionInputStagger = 0
				sp := c.SpellPoints
				gl.updateAutomaticTechniques()
				if c.SpellPoints != sp || !patient.Purifiable() {
					t.Fatal("manual use bypassed automatic suppression")
				}
			})
		}
	}
}

func TestPersonalSpellReuseAcrossModesAndSave(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprint(tb), func(t *testing.T) {
			g := advancedSupportGame(t, tb)
			equipSpellAndPrepareCaster(t, g.combat, "protection_from_magic", 1000, 50)
			c := g.party.Members[0]
			d, _ := spells.GetSpellDefinitionByID("protection_from_magic")
			cast := func() bool { return g.combat.castPlayerSpell(d.ID, d, c, false) }
			if !cast() {
				t.Fatal("initial cast rejected")
			}
			total := d.ReuseSeconds * g.config.GetTPS()
			if total <= 0 || c.SpellState.Reuse[string(d.ID)].Frames != total {
				t.Fatal("cast did not start authored reuse")
			}
			sp := c.SpellPoints
			if cast() || c.SpellPoints != sp {
				t.Fatal("reuse did not reject a repeat without charging mana")
			}
			other := g.party.Members[1]
			other.LearnSpell(d.ID)
			other.SpellPoints = 1000
			if !g.combat.castPlayerSpell(d.ID, d, other, false) {
				t.Fatal("personal reuse leaked to another hero")
			}
			step := 1
			if tb {
				step = g.combatRoundFrames()
			}
			passFrames(g, step)
			saved := buildCharacterSave(c)
			c = restoreCharacterSave(saved)
			g.party.Members[0] = c
			if c.SpellState.Reuse[string(d.ID)].Frames != total-step {
				t.Fatal("save/load changed the remaining budget")
			}
			g.turnBasedMode = !tb
			passFrames(g, total-step-1)
			if cast() {
				t.Fatal("mode switch or partial tick cleared reuse early")
			}
			passFrames(g, 1)
			if !cast() {
				t.Fatal("spell not ready at exact expiry")
			}
			saved.SpellState.Reuse[string(d.ID)] = framesClock(total * 2)
			saved.SpellState.Reuse["removed_spell"] = framesClock(10)
			saved.SpellState.Reuse["firebolt"] = framesClock(10)
			restored := restoreCharacterSave(saved)
			if restored.SpellState.Reuse[string(d.ID)].Frames != total || len(restored.SpellState.Reuse) != 1 {
				t.Fatal("load did not normalize reuse against current catalog")
			}
			rounds := (total + g.combatRoundFrames() - 1) / g.combatRoundFrames()
			if !strings.Contains(GetSpellTooltipRows(d.ID, c, g.combat, true).String(), fmt.Sprintf("%ds / %d round", d.ReuseSeconds, rounds)) {
				t.Fatal("tooltip omitted authored personal reuse")
			}
		})
	}
}

// Celestial Armor ends the moment its absorption is used up, in either mode.
func TestCelestialArmorEndsWhenSpent(t *testing.T) {
	for _, tb := range []bool{false, true} {
		g := advancedSupportGame(t, tb)
		castAdvancedSupport(t, g, "celestial_armor", 3, nil)
		b := g.advancedBuff(config.MechanicCelestialArmor)
		if b == nil || b.Absorb <= 0 {
			t.Fatal("fixture: no absorption")
		}
		absorb := b.Absorb
		if left := g.absorbCelestialDamage(absorb + 7); left != 7 {
			t.Fatalf("absorbed %d of %d", absorb+7-left, absorb)
		}
		if len(g.combatBuffs) != 0 || g.buffStatuses["celestial_armor"] != nil {
			t.Fatalf("tb=%v: spent Celestial Armor stayed: %+v", tb, g.combatBuffs)
		}
	}
}
