package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

// save1MemberSpec rebuilds (in code, NOT from the save file) one member of the
// real save-1 endgame party: exact base stats, the actual (mostly non-GM) skill
// masteries, magic schools, gear, and the quick-slot combat action. Faithful so
// the damage profile matches what that party would deal/take in the jungle.
type save1MemberSpec struct {
	name                               string
	class                              character.CharacterClass
	might, intel, pers, end, acc, luck int
	skills                             map[character.SkillType]character.SkillMastery
	schools                            map[character.MagicSchoolID]character.SkillMastery
	knownSpells                        []spells.SpellID
	weaponKey                          string
	itemKeys                           []string
	slotSpell                          spells.SpellID // "" → weapon-attacks (no offensive spell modelled)
}

func save1PartySpecs() []save1MemberSpec {
	return []save1MemberSpec{
		{ // Auberon — Paladin tank. GM Axe but wields a MACE → no weapon-mastery true damage.
			name: "Auberon", class: character.ClassPaladin,
			might: 69, intel: 10, pers: 45, end: 24, acc: 10, luck: 11,
			skills: map[character.SkillType]character.SkillMastery{
				character.SkillAxe: character.MasteryGrandMaster, character.SkillMace: character.MasteryNovice,
				character.SkillPlate: character.MasteryExpert, character.SkillBodybuilding: character.MasteryExpert,
			},
			schools:     map[character.MagicSchoolID]character.SkillMastery{"body": character.MasteryExpert},
			knownSpells: []spells.SpellID{"heal", "heal_other"},
			weaponKey:   "idol_breakers_maul",
			itemKeys:    []string{"mantle_of_the_idol_king", "warlords_signet", "warlords_signet", "grave_knight_sigil", "kabuto", "batwing_cloak"},
			slotSpell:   "heal",
		},
		{ // Mirelle — Druid caster. Offensive quick = Ray of Light (Light Expert).
			name: "Mirelle", class: character.ClassDruid,
			might: 10, intel: 70, pers: 46, end: 20, acc: 10, luck: 11,
			skills: map[character.SkillType]character.SkillMastery{
				character.SkillStaff: character.MasteryExpert, character.SkillBodybuilding: character.MasteryExpert,
			},
			schools: map[character.MagicSchoolID]character.SkillMastery{
				"earth": character.MasteryGrandMaster, "light": character.MasteryExpert,
			},
			knownSpells: []spells.SpellID{"ray_of_light", "day_of_the_gods", "stone_skin", "deadly_swarm"},
			weaponKey:   "oak_staff",
			itemKeys:    []string{"jaguar_pelt_jerkin", "direpelt_cloak", "ratskin_boots", "leather_helmet"},
			slotSpell:   "ray_of_light",
		},
		{ // Celestine — Cleric healer. No offensive quick (Mass Heal) → weapon-attacks with a Mace (Novice).
			name: "Celestine", class: character.ClassCleric,
			might: 10, intel: 12, pers: 99, end: 27, acc: 8, luck: 10,
			skills: map[character.SkillType]character.SkillMastery{character.SkillMace: character.MasteryNovice},
			schools: map[character.MagicSchoolID]character.SkillMastery{
				"spirit": character.MasteryExpert, "body": character.MasteryExpert,
			},
			knownSpells: []spells.SpellID{"bless", "mass_heal", "heal_other"},
			weaponKey:   "bone_warclub",
			itemKeys:    []string{"silk_hakama", "chain_helmet", "batwing_cloak"},
			slotSpell:   "", // Mass Heal is utility; with a full party it falls through to the weapon anyway
		},
		{ // Nyra — Thief. Dagger Expert (+3 true). Blast Trap quick slot is not modelled (weapon-attacks).
			name: "Nyra", class: character.ClassThief,
			might: 8, intel: 48, pers: 8, end: 18, acc: 76, luck: 11,
			skills: map[character.SkillType]character.SkillMastery{
				character.SkillDagger: character.MasteryExpert, character.SkillTrapper: character.MasteryGrandMaster,
				character.SkillDisarmTrap: character.MasteryExpert,
			},
			weaponKey: "agility_katar",
			itemKeys:  []string{"leather_armor", "ratskin_boots", "magic_ring", "batwing_cloak"},
			slotSpell: "",
		},
	}
}

func buildSave1Party(t *testing.T, cs *CombatSystem) []*character.MMCharacter {
	t.Helper()
	cfg := cs.game.config
	cs.game.party = character.NewParty(cfg)
	party := make([]*character.MMCharacter, 0, 4)
	for _, s := range save1PartySpecs() {
		c := character.CreateCharacter(s.name, s.class, cfg)
		c.Level = 22
		c.Might, c.Intellect, c.Personality = s.might, s.intel, s.pers
		c.Endurance, c.Accuracy, c.Speed, c.Luck = s.end, s.acc, 16, s.luck
		for st, m := range s.skills {
			c.Skills[st] = &character.Skill{Mastery: m}
		}
		for sch, m := range s.schools {
			c.MagicSchools[sch] = &character.MagicSkill{Mastery: m}
		}
		for _, sp := range s.knownSpells {
			c.LearnSpell(sp) // adds to its school (keeps the mastery set above)
		}
		mustEquipWeaponKey(t, c, s.weaponKey)
		for _, k := range s.itemKeys {
			mustEquipItemKey(t, c, k)
		}
		if s.slotSpell != "" {
			mustEquipSpellID(t, c, s.slotSpell)
		}
		party = append(party, c)
	}
	cs.game.party.Members = party
	return party
}

// applySave1Buffs reproduces the three party-wide buffs active in save 1: Bless
// +6 to every stat, Stone Skin (−10 flat incoming), Day of the Gods (+16% resist
// to all damage). Values are taken straight from the save (already mastery-scaled),
// so the caster's mastery doesn't need modelling. Recomputes derived stats so the
// Bless Endurance folds into MaxHP, then tops everyone off.
func applySave1Buffs(cs *CombatSystem, party []*character.MMCharacter) {
	cs.game.statBuffs = nil
	cs.game.addStatBuff(TimedStatBuff{SpellID: "bless", Frames: 1 << 30, Bonuses: character.UniformStatBonuses(6)})
	cs.game.recomputeStatBonuses()
	cs.game.combatBuffs = nil
	cs.game.addCombatBuff(TimedCombatBuff{SpellID: "stone_skin", Frames: 1 << 30, InReduce: 10})
	cs.game.addCombatBuff(TimedCombatBuff{SpellID: "day_of_the_gods", Frames: 1 << 30, ResistPct: 16})
	for _, c := range party {
		c.CalculateDerivedStats(cs.game.config)
		c.HitPoints, c.SpellPoints = c.MaxHitPoints, c.MaxSpellPoints
	}
}

// avgOutgoingPerHit measures one attacker's per-action damage to a fresh mob (the
// simplified player path: offensive spell if equipped, else weapon → monster
// armor + resist). Averaged over trials. The party is full-HP so a healer's quick
// slot falls through to its weapon, matching real behaviour.
func avgOutgoingPerHit(cs *CombatSystem, attacker *character.MMCharacter, mobKey string, party []*character.MMCharacter, trials int) int {
	sum := 0
	for i := 0; i < trials; i++ {
		mob := monsterPkg.NewMonster3DFromConfig(0, 0, mobKey, cs.game.config)
		attacker.SpellPoints = attacker.MaxSpellPoints
		before := mob.HitPoints
		playerActSlot(cs, attacker, mob, party)
		sum += before - mob.HitPoints
	}
	return sum / trials
}

// avgIncomingPerHit measures the post-mitigation damage one melee hit from the mob
// lands on the target (armor% → resist% → floor → flat Stone Skin/DisarmTrap),
// averaged over the mob's damage roll. Pure: it doesn't actually wound the target.
func avgIncomingPerHit(cs *CombatSystem, mobKey string, target *character.MMCharacter, trials int) int {
	mob := monsterPkg.NewMonster3DFromConfig(0, 0, mobKey, cs.game.config)
	sum := 0
	for i := 0; i < trials; i++ {
		// Honour the attacker's armor-pierce (Orc Warlord's melee bypasses AC) and
		// add its true damage (bypasses all mitigation).
		sum += cs.mitigateCharacterDamage(mob.GetAttackDamage(), "physical", target, mob.IgnoresArmor) + mob.TrueDamage
	}
	return sum / trials
}

// avgMitigated averages, over trials, the post-mitigation damage of `roll()`-rolled
// hits of the given type against the target (armor honoured per ignoreArmor).
func avgMitigated(cs *CombatSystem, target *character.MMCharacter, dmgType string, ignoreArmor bool, trials int, roll func() int) int {
	sum := 0
	for i := 0; i < trials; i++ {
		sum += cs.mitigateCharacterDamage(roll(), dmgType, target, ignoreArmor)
	}
	return sum / trials
}

// TestCombatBalance_Save1PartyJungleAndCliffs runs the real save-1 endgame party
// (L22 Paladin/Druid/Cleric/Thief, with Bless + Stone Skin + Day of the Gods)
// against every Deep Jungle and Dragon Cliffs monster, reporting per-member
// OUTGOING and INCOMING per-hit damage, plus solo and paired fight outcomes.
// Diagnostic (run with -run Save1 -v); the only assertion is a sanity floor.
func TestCombatBalance_Save1PartyJungleAndCliffs(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	party := buildSave1Party(t, cs)
	applySave1Buffs(cs, party)

	t.Log("=== Save-1 party (rebuilt in code) — buffs: Bless +6 all stats, Stone Skin −10 flat, Day of the Gods +16% resist ===")
	for _, c := range party {
		mgt, intl, pers, end, acc, spd, lck := c.GetEffectiveStats()
		wpn := c.Equipment[items.SlotMainHand].Name
		ac := cs.CalculateTotalArmorClass(c)
		t.Logf("  %-10s %-8s L%d  HP %d  SP %d  AC %d  eff[Mgt%d Int%d Per%d End%d Acc%d Spd%d Lck%d]  %s",
			c.Name, c.Class.String(), c.Level, c.MaxHitPoints, c.MaxSpellPoints, ac, mgt, intl, pers, end, acc, spd, lck, wpn)
	}

	const trials = 60
	zones := []struct {
		name string
		mobs []string
	}{
		{"DEEP JUNGLE", []string{"jungle_goblin", "ocelot", "masked_huntress", "masked_serpent_dancer", "masked_hexer_girl", "gorilla_titan", "orc_hero_boss"}},
		{"DRAGON CLIFFS", []string{"mountain_troll", "archmage", "dragon_green", "dragon_gold"}},
	}

	for _, z := range zones {
		t.Logf("\n======== %s — per-hit damage (avg of %d) ========", z.name, trials)
		t.Logf("%-22s %-12s | OUT  Aub  Mir  Cel  Nyr | IN   Aub  Mir  Cel  Nyr", "monster", "L/HP/AC")
		for _, mk := range z.mobs {
			ref := monsterPkg.NewMonster3DFromConfig(0, 0, mk, cs.game.config)
			out := make([]int, 4)
			in := make([]int, 4)
			for i, c := range party {
				out[i] = avgOutgoingPerHit(cs, c, mk, party, trials)
				in[i] = avgIncomingPerHit(cs, mk, c, trials)
			}
			t.Logf("%-22s L%-2d %4d/%-3d | OUT %4d %4d %4d %4d | IN  %4d %4d %4d %4d",
				ref.Name, ref.Level, ref.MaxHitPoints, ref.ArmorClass,
				out[0], out[1], out[2], out[3], in[0], in[1], in[2], in[3])
		}

		// Solo + paired fight outcomes (Bless re-applied by the runner; the Stone
		// Skin / Day of the Gods combat buffs set above persist through it).
		// NOTE: summoners bring their escort here — the sim's trySimBossSummon is
		// generic, so "1x gorilla_titan" actually fights the gorilla + 2 Masked
		// Huntress, and "1x orc_hero_boss" fights the Warlord + Huntress/Hexer adds.
		// (The per-hit table above is per single mob and excludes summons.)
		t.Logf("---- %s fight outcomes (solo & pairs; bosses include summoned escort) ----", z.name)
		for _, mk := range z.mobs {
			runFixedPartySim(t, cs, buildSave1Party, []string{mk}, "  1x "+mk, 6, 40)
			runFixedPartySim(t, cs, buildSave1Party, []string{mk, mk}, "  2x "+mk, 6, 40)
		}
	}

	// Precise INCOMING from the dragons / Gorilla / Warlord, broken out by attack
	// mode (melee, elemental breath, fireburst AoE, enraged melee) — the per-hit
	// table above only covers basic physical melee. Numbers are post-mitigation
	// (armor% → resist% → floor → Stone Skin/DisarmTrap flat), avg of 200.
	t.Logf("\n======== PRECISE INCOMING from dragons / gorilla / warlord (avg 200) ========")
	t.Logf("%-26s | Aub  Mir  Cel  Nyr", "attack")
	const dn = 200
	for _, mk := range []string{"dragon", "dragon_red", "dragon_green", "dragon_gold", "gorilla_titan", "orc_hero_boss"} {
		ref := monsterPkg.NewMonster3DFromConfig(0, 0, mk, cs.game.config)
		t.Logf("--- %s (dmg %d-%d, AC %d, ignoresArmor=%v) ---", ref.Name, ref.DamageMin, ref.DamageMax, ref.ArmorClass, ref.IgnoresArmor)
		row4 := func(label string, dmgType string, ignoreArmor bool, roll func() int) []int {
			out := make([]int, 4)
			for i, c := range party {
				out[i] = avgMitigated(cs, c, dmgType, ignoreArmor, dn, roll) + ref.TrueDamage // true damage bypasses mitigation
			}
			t.Logf("%-26s | %3d  %3d  %3d  %3d", label, out[0], out[1], out[2], out[3])
			return out
		}
		attackRoll := func() int { return monsterPkg.NewMonster3DFromConfig(0, 0, mk, cs.game.config).GetAttackDamage() }
		if ref.ProjectileSpell != "" {
			// Ranged attacker: it ALWAYS breathes (never melees), so the breath is
			// the real incoming. Same damage roll as melee, but the spell's element.
			school := "?"
			if d, ok := config.GetSpellDefinition(ref.ProjectileSpell); ok && d != nil {
				school = d.School
			}
			row4("  breath:"+ref.ProjectileSpell+" ("+school+")", school, false, attackRoll)
		} else {
			meleeLbl := "  melee (physical)"
			if ref.IgnoresArmor {
				meleeLbl = "  melee (IGNORES armor)"
			}
			row4(meleeLbl, "physical", ref.IgnoresArmor, attackRoll)
		}
		if ref.FireburstChance > 0 {
			fb := make([]int, 4)
			for i, c := range party {
				lo, hi := ref.FireburstDamageMin, ref.FireburstDamageMax
				fb[i] = avgMitigated(cs, c, "fire", false, dn, func() int {
					if hi > lo {
						return lo + (hi-lo)/2
					}
					return lo
				})
			}
			t.Logf("%-26s | %3d  %3d  %3d  %3d", "  fireburst AoE (fire)", fb[0], fb[1], fb[2], fb[3])
		}
		if ref.EnrageAtHP > 0 {
			en := make([]int, 4)
			for i, c := range party {
				en[i] = avgMitigated(cs, c, "physical", ref.IgnoresArmor, dn, func() int {
					m := monsterPkg.NewMonster3DFromConfig(0, 0, mk, cs.game.config)
					m.HitPoints = 1 // below enrage threshold → GetAttackDamage applies x1.5
					return m.GetAttackDamage()
				})
			}
			t.Logf("%-26s | %3d  %3d  %3d  %3d", "  melee ENRAGED (x1.5)", en[0], en[1], en[2], en[3])
		}
	}

	// Sanity floor: this fully-geared, buffed L22 party must at least clear a lone
	// trash jungle goblin every time — a regression that fails this means the
	// party build or damage pipeline broke, not balance.
	applySave1Buffs(cs, party)
	if dmg := avgOutgoingPerHit(cs, party[0], "jungle_goblin", party, 20); dmg <= 0 {
		t.Fatalf("Auberon dealt no damage to a goblin (%d) — party build/pipeline broken", dmg)
	}
}
