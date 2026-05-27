package game

// Diagnostic combat-balance simulator. Builds a reference L5 party (Knight,
// Sorcerer, Cleric, Archer) with hand-tuned stats and equipment, then runs N
// turn-based fights against each monster to estimate threat level.
//
// The roster values were originally seeded from a save file but are now a
// standalone balance baseline — edit them by hand if you want to retune.
//
// Output is informational (t.Logf) — no balance assertions, since "fair" is
// subjective. Run with `go test ./internal/game -run CombatBalance -v` to see
// the numbers.

import (
	"fmt"
	"math/rand"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

// balanceRosterEntry is one character in the balance baseline party.
// MaxHP and MaxSP are derived (HP=End*2+Lvl*3; SP=Int+Per+equipPersBonus+Lvl*2)
// so we don't pin them here — CalculateDerivedStats reconstructs them after
// equipment is applied.
type balanceRosterEntry struct {
	Name      string
	Class     character.CharacterClass
	Level     int
	Might     int
	Intellect int
	Person    int
	Endur     int
	Accuracy  int
	Speed     int
	Luck      int
	WeaponKey string   // main-hand weapon (YAML key)
	SpellID   string   // equipped spell ("" = weapon-only)
	ItemKeys  []string // armor/accessory keys — auto-routed by EquipItem
}

var balanceRoster = []balanceRosterEntry{
	{
		Name: "Gareth", Class: character.ClassKnight, Level: 5,
		Might: 19, Intellect: 10, Person: 10, Endur: 25, Accuracy: 12, Speed: 16, Luck: 10,
		WeaponKey: "iron_sword",
		ItemKeys:  []string{"leather_armor", "leather_pants"},
	},
	{
		Name: "Lysander", Class: character.ClassSorcerer, Level: 5,
		Might: 8, Intellect: 22, Person: 12, Endur: 15, Accuracy: 10, Speed: 16, Luck: 12,
		WeaponKey: "magic_dagger", SpellID: "darkbolt",
		ItemKeys: []string{"magic_ring"},
	},
	{
		Name: "Celestine", Class: character.ClassCleric, Level: 5,
		Might: 10, Intellect: 12, Person: 26, Endur: 15, Accuracy: 8, Speed: 16, Luck: 10,
		WeaponKey: "steel_mace", SpellID: "heal",
	},
	{
		Name: "Silvelyn", Class: character.ClassArcher, Level: 5,
		Might: 12, Intellect: 15, Person: 10, Endur: 15, Accuracy: 21, Speed: 16, Luck: 10,
		WeaponKey: "hunting_bow", SpellID: "wizard_eye",
	},
}

func buildBalanceParty(t *testing.T, cs *CombatSystem) []*character.MMCharacter {
	t.Helper()
	cfg := cs.game.config
	party := make([]*character.MMCharacter, 0, 4)
	for _, tpl := range balanceRoster {
		c := character.CreateCharacter(tpl.Name, tpl.Class, cfg)
		c.Level = tpl.Level
		c.Might, c.Intellect, c.Personality, c.Endurance = tpl.Might, tpl.Intellect, tpl.Person, tpl.Endur
		c.Accuracy, c.Speed, c.Luck = tpl.Accuracy, tpl.Speed, tpl.Luck
		c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(tpl.WeaponKey)
		if tpl.SpellID != "" {
			sp, err := spells.CreateSpellItem(spells.SpellID(tpl.SpellID))
			if err != nil {
				t.Fatalf("create spell %s: %v", tpl.SpellID, err)
			}
			c.Equipment[items.SlotSpell] = sp
		}
		// Armor/accessories — EquipItem routes each item to its YAML slot.
		for _, key := range tpl.ItemKeys {
			it := items.CreateItemFromYAML(key)
			if _, _, ok := c.EquipItem(it); !ok {
				t.Fatalf("%s: failed to equip %s", tpl.Name, key)
			}
		}
		// Compute MaxHP/MaxSP from stats+level+equipment and fill HP/SP.
		c.CalculateDerivedStats(cfg)
		party = append(party, c)
	}
	return party
}

func resetPartyForSim(party []*character.MMCharacter) {
	for _, c := range party {
		c.HitPoints = c.MaxHitPoints
		c.SpellPoints = c.MaxSpellPoints
		c.Conditions = nil
	}
}

func partyHPRatio(party []*character.MMCharacter) float64 {
	var hp, max int
	for _, c := range party {
		hp += c.HitPoints
		max += c.MaxHitPoints
	}
	if max == 0 {
		return 0
	}
	return float64(hp) / float64(max)
}

func partyAllDown(party []*character.MMCharacter) bool {
	for _, c := range party {
		if c.CanAct() {
			return false
		}
	}
	return true
}

func spellSchoolToDamageType(school string) monsterPkg.DamageType {
	switch school {
	case "fire":
		return monsterPkg.DamageFire
	case "water":
		return monsterPkg.DamageWater
	case "air":
		return monsterPkg.DamageAir
	case "earth":
		return monsterPkg.DamageEarth
	default:
		return monsterPkg.DamagePhysical
	}
}

// lowestHurtAlly returns the alive ally with the most missing HP, or nil if
// everyone is at full HP.
func lowestHurtAlly(party []*character.MMCharacter) *character.MMCharacter {
	var pick *character.MMCharacter
	bestRatio := 1.0
	for _, c := range party {
		if !c.CanAct() || c.HitPoints >= c.MaxHitPoints {
			continue
		}
		r := float64(c.HitPoints) / float64(c.MaxHitPoints)
		if r < bestRatio {
			pick, bestRatio = c, r
		}
	}
	return pick
}

// playerActSlot performs one action slot. Casters with utility spells heal
// when an ally is hurt; otherwise everyone uses their weapon.
func playerActSlot(cs *CombatSystem, char *character.MMCharacter, target *monsterPkg.Monster3D, party []*character.MMCharacter) {
	if spItem, hasSp := char.Equipment[items.SlotSpell]; hasSp && spItem.SpellEffect != "" {
		spellID := spells.SpellID(spItem.SpellEffect)
		def, err := spells.GetSpellDefinitionByID(spellID)
		if err == nil && char.SpellPoints >= def.SpellPointsCost {
			if def.IsUtility {
				if hurt := lowestHurtAlly(party); hurt != nil {
					_, _, heal := cs.CalculateSpellHealing(spellID, char)
					hurt.HitPoints += heal
					if hurt.HitPoints > hurt.MaxHitPoints {
						hurt.HitPoints = hurt.MaxHitPoints
					}
					char.SpellPoints -= def.SpellPointsCost
					return
				}
				// No one to heal — fall through to weapon.
			} else {
				_, _, dmg := cs.CalculateSpellDamage(spellID, char)
				target.TakeDamage(dmg, spellSchoolToDamageType(def.School), 0, 0)
				char.SpellPoints -= def.SpellPointsCost
				return
			}
		}
	}
	weapon, hasW := char.Equipment[items.SlotMainHand]
	if !hasW {
		return
	}
	_, _, dmg := cs.CalculateWeaponDamage(weapon, char)
	weaponDef := lookupWeaponConfigByName(weapon.Name)
	isRanged := weaponDef != nil && weaponDef.Category == "bow"
	dmg = applyArmorReductionIfPhysical(dmg, "physical", target.ArmorClass, isRanged)
	target.TakeDamage(dmg, monsterPkg.DamagePhysical, 0, 0)
}

func monsterActOnce(cs *CombatSystem, m *monsterPkg.Monster3D, party []*character.MMCharacter) {
	attacks := m.GetTurnBasedAttackCount()
	for i := 0; i < attacks; i++ {
		alive := make([]*character.MMCharacter, 0, len(party))
		for _, c := range party {
			if c.CanAct() {
				alive = append(alive, c)
			}
		}
		if len(alive) == 0 {
			return
		}
		target := alive[rand.Intn(len(alive))]
		var dmg int
		if m.HasRangedAttack() && m.ProjectileSpell != "" {
			// Elemental projectile — no AC reduction, no resistance (party has none defined).
			dmg = m.GetAttackDamage()
		} else {
			dmg = cs.applyArmorToCharacterIfPhysical(m.GetAttackDamage(), "physical", target)
		}
		// Fireburst proc (used by dragon)
		if m.FireburstChance > 0 && rand.Float64() < m.FireburstChance {
			dmg += m.FireburstDamageMin + rand.Intn(m.FireburstDamageMax-m.FireburstDamageMin+1)
		}
		target.HitPoints -= dmg
		if target.HitPoints < 0 {
			target.HitPoints = 0
		}
		if target.HitPoints == 0 {
			target.AddCondition(character.ConditionUnconscious)
		}
	}
}

type simResult struct {
	avgRoundsWin float64
	winPct       float64
	wipePct      float64
	avgHPLeft    float64
}

// pickLowestHPTarget returns the alive monster with the lowest current HP,
// or nil if everyone is dead. Players naturally focus-fire the most-hurt
// enemy to remove a damage source, so the sim mirrors that.
func pickLowestHPTarget(monsters []*monsterPkg.Monster3D) *monsterPkg.Monster3D {
	var pick *monsterPkg.Monster3D
	for _, m := range monsters {
		if !m.IsAlive() {
			continue
		}
		if pick == nil || m.HitPoints < pick.HitPoints {
			pick = m
		}
	}
	return pick
}

func allMonstersDead(monsters []*monsterPkg.Monster3D) bool {
	for _, m := range monsters {
		if m.IsAlive() {
			return false
		}
	}
	return true
}

// simulateFight runs one or more monsters against the party, repeated N
// times. Caller passes monsterKeys with duplicates to spawn multiples
// (e.g., ["goblin", "goblin"] for a 2-goblin encounter).
func simulateFight(t *testing.T, cs *CombatSystem, party []*character.MMCharacter, monsterKeys []string, blessActive bool, trials int) simResult {
	var totalRoundsWin, wins, wipes int
	var sumHPLeft float64
	const maxRounds = 30

	if blessActive {
		cs.game.statBonus = 10 // Novice-tier Bless from spells.yaml.
	} else {
		cs.game.statBonus = 0
	}
	defer func() { cs.game.statBonus = 0 }()

	rand.Seed(42)
	for trial := 0; trial < trials; trial++ {
		resetPartyForSim(party)
		monsters := make([]*monsterPkg.Monster3D, 0, len(monsterKeys))
		for _, key := range monsterKeys {
			m := monsterPkg.NewMonster3DFromConfig(0, 0, key, cs.game.config)
			// Force engagement so PassiveUntilAttacked monsters (medusa) act.
			m.WasAttacked = true
			monsters = append(monsters, m)
		}
		round := 0
		killed := false
		for ; round < maxRounds; round++ {
			// Party turn — each char uses their action slots on the
			// lowest-HP alive monster (focus-fire).
			for _, c := range party {
				if !c.CanAct() {
					continue
				}
				slots := c.ActionSlotsForTurn(cs.game.statBonus)
				for s := 0; s < slots; s++ {
					target := pickLowestHPTarget(monsters)
					if target == nil {
						break
					}
					playerActSlot(cs, c, target, party)
				}
				if allMonstersDead(monsters) {
					killed = true
					break
				}
			}
			if killed {
				round++
				break
			}
			// Each surviving monster acts.
			for _, m := range monsters {
				if !m.IsAlive() {
					continue
				}
				monsterActOnce(cs, m, party)
				if partyAllDown(party) {
					break
				}
			}
			if partyAllDown(party) {
				break
			}
		}
		if killed {
			wins++
			totalRoundsWin += round
		}
		if partyAllDown(party) {
			wipes++
		}
		sumHPLeft += partyHPRatio(party)
	}

	winsFloat := float64(wins)
	res := simResult{
		winPct:    winsFloat * 100 / float64(trials),
		wipePct:   float64(wipes) * 100 / float64(trials),
		avgHPLeft: sumHPLeft * 100 / float64(trials),
	}
	if wins > 0 {
		res.avgRoundsWin = float64(totalRoundsWin) / winsFloat
	}
	return res
}

// TestCombatBalance_RosterDerivedStats verifies the baseline roster produces
// the expected MaxHP/MaxSP after equipment is applied. If this fails, either
// the roster numbers changed or HP/SP formulas in config.yaml drifted.
func TestCombatBalance_RosterDerivedStats(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	party := buildBalanceParty(t, cs)

	expected := []struct {
		name   string
		hp, sp int
	}{
		{"Gareth", 65, 30},
		{"Lysander", 45, 45},
		{"Celestine", 45, 48},
		{"Silvelyn", 45, 35},
	}
	for i, want := range expected {
		got := party[i]
		if got.MaxHitPoints != want.hp {
			t.Errorf("%s MaxHP: got %d, want %d", want.name, got.MaxHitPoints, want.hp)
		}
		if got.MaxSpellPoints != want.sp {
			t.Errorf("%s MaxSP: got %d, want %d", want.name, got.MaxSpellPoints, want.sp)
		}
	}
}

// applyLevelInvestment simulates per-level stat point spending and L3 choices
// using class-typical strategy. StatPointsPerLevel (5) is split across each
// class's two priority stats. L3 choice applies the more offensive option.
func applyLevelInvestment(c *character.MMCharacter, level int) {
	// Per-level stat investment — applied for every level beyond 1.
	for lvl := 2; lvl <= level; lvl++ {
		switch c.Class {
		case character.ClassKnight:
			c.Might += 3
			c.Endurance += 2
		case character.ClassSorcerer:
			c.Intellect += 3
			c.Personality += 2
		case character.ClassCleric:
			c.Personality += 3
			c.Endurance += 2
		case character.ClassArcher:
			c.Accuracy += 3
			c.Speed += 2
		}
	}
	// L3 unlocks one mastery/spell choice from level_up.yaml — pick the
	// offensive option per class.
	if level >= 3 {
		switch c.Class {
		case character.ClassKnight:
			if sk, ok := c.Skills[character.SkillSword]; ok && sk.Mastery < character.MasteryExpert {
				sk.Mastery = character.MasteryExpert
			}
		case character.ClassSorcerer:
			_ = addSpellByID(c, "fireball")
		case character.ClassCleric:
			_ = addSpellByID(c, "bless")
		case character.ClassArcher:
			if sk, ok := c.Skills[character.SkillBow]; ok && sk.Mastery < character.MasteryExpert {
				sk.Mastery = character.MasteryExpert
			}
		}
	}
}

// buildDefaultParty creates the stock starter party (Knight/Sorcerer/Cleric/Archer)
// at a given level. Beyond L1, stat points and L3 choices are auto-invested
// using a sensible per-class strategy (see applyLevelInvestment).
func buildDefaultParty(t *testing.T, cs *CombatSystem, level int) []*character.MMCharacter {
	t.Helper()
	cfg := cs.game.config
	roster := []struct {
		name  string
		class character.CharacterClass
	}{
		{"Gareth", character.ClassKnight},
		{"Lysander", character.ClassSorcerer},
		{"Celestine", character.ClassCleric},
		{"Silvelyn", character.ClassArcher},
	}
	party := make([]*character.MMCharacter, 0, 4)
	for _, ent := range roster {
		c := character.CreateCharacter(ent.name, ent.class, cfg)
		c.Level = level
		applyLevelInvestment(c, level)
		c.CalculateDerivedStats(cfg)
		party = append(party, c)
	}
	return party
}

type fightScenario struct {
	label    string   // shown in the "monster" column
	monsters []string // one entry per spawned monster; duplicates are allowed
}

func singleMonsterScenarios(keys ...string) []fightScenario {
	out := make([]fightScenario, 0, len(keys))
	for _, k := range keys {
		out = append(out, fightScenario{label: k, monsters: []string{k}})
	}
	return out
}

func logFightTable(t *testing.T, cs *CombatSystem, party []*character.MMCharacter, scenarios []fightScenario, blessStates []bool, trials int, header string) {
	t.Logf("%s — %d trials each.", header, trials)
	t.Logf("%-14s | %-9s | %-7s | %-7s | %-7s | %s",
		"encounter", "bless", "wins%", "wipes%", "rounds", "HP left avg%")
	for _, sc := range scenarios {
		for _, bless := range blessStates {
			r := simulateFight(t, cs, party, sc.monsters, bless, trials)
			blessLbl := "no"
			if bless {
				blessLbl = "yes(+10)"
			}
			rounds := "n/a"
			if r.winPct > 0 {
				rounds = fmt.Sprintf("%.1f", r.avgRoundsWin)
			}
			t.Logf("%-14s | %-9s | %6.1f  | %6.1f  | %-7s | %5.0f",
				sc.label, blessLbl, r.winPct, r.wipePct, rounds, r.avgHPLeft)
		}
	}
}

// TestCombatBalance_L5PartyVsMonsters runs the baseline L5 party against
// octopus / medusa / dragon, with and without Bless. Prints a small table.
func TestCombatBalance_L5PartyVsMonsters(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	party := buildBalanceParty(t, cs)

	logFightTable(t, cs, party,
		singleMonsterScenarios("octopus", "medusa", "dragon"),
		[]bool{false, true},
		200,
		"Reference L5 party (Knight/Sorcerer/Cleric/Archer) vs single monster",
	)
}

// forestMonsters lists every monster letter that can spawn in the forest
// biome. Medusa (m) and octopus (o-water) are excluded — they are biome=water.
var forestMonsters = []string{
	"goblin", "orc", "wolf", "bear", "spider", "skeleton", "troll",
	"forest_orc", "dire_wolf", "forest_spider", "treant", "pixie",
	"bandit", "alien", "dragon",
}

// TestCombatBalance_LowLevelPartyVsForest runs the default starter party at
// L1/L2/L3 (with class-appropriate stat investment) against every monster
// that can spawn in the forest biome — both 1v1 and 1v2 of the trainer
// mobs (goblin, spider, wolf, forest_spider, pixie) so we can see how
// pack-spawns scale difficulty. Bless is off — fresh parties typically
// don't have it up. Prints one table per level.
func TestCombatBalance_LowLevelPartyVsForest(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")

	scenarios := singleMonsterScenarios(forestMonsters...)
	for _, twin := range []string{"goblin", "spider", "wolf", "forest_spider", "pixie"} {
		scenarios = append(scenarios, fightScenario{
			label:    twin + " x2",
			monsters: []string{twin, twin},
		})
	}

	const trials = 200
	for _, level := range []int{1, 2, 3} {
		level := level
		t.Run(fmt.Sprintf("L%d", level), func(t *testing.T) {
			party := buildDefaultParty(t, cs, level)
			logFightTable(t, cs, party, scenarios,
				[]bool{false}, trials,
				fmt.Sprintf("Default starter party L%d vs forest monsters", level),
			)
		})
	}
}
