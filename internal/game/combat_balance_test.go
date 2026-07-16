package game

// Diagnostic combat-balance simulator. Builds a reference L5 party (Knight,
// Sorcerer, Cleric, Archer) with hand-tuned stats and equipment, then runs N
// turn-based fights against each monster to estimate threat level.
//
// The roster values were originally seeded from a save file but are now a
// standalone balance baseline - edit them by hand if you want to retune.
//
// Output is informational (t.Logf) - no balance assertions, since "fair" is
// subjective. Run with `go test ./internal/game -run CombatBalance -v` to see
// the numbers.

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

// balanceRosterEntry is one character in the balance baseline party.
// MaxHP and MaxSP are derived (HP=End*2+Lvl*3; SP=Int+Per/3+Lvl*2)
// so we don't pin them here - CalculateDerivedStats reconstructs them after
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
	ItemKeys  []string // armor/accessory keys - auto-routed by EquipItem
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
		// Armor/accessories - EquipItem routes each item to its YAML slot.
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
					if def.HealParty {
						for _, ally := range party {
							if !ally.CanAct() || ally.HitPoints >= ally.MaxHitPoints {
								continue
							}
							ally.HitPoints += heal
							if ally.HitPoints > ally.MaxHitPoints {
								ally.HitPoints = ally.MaxHitPoints
							}
						}
					} else {
						hurt.HitPoints += heal
						if hurt.HitPoints > hurt.MaxHitPoints {
							hurt.HitPoints = hurt.MaxHitPoints
						}
					}
					char.SpellPoints -= def.SpellPointsCost
					return
				}
				// No one to heal - fall through to weapon.
			} else {
				_, _, dmg := cs.CalculateSpellDamage(spellID, char)
				target.TakeDamage(dmg, spellSchoolToDamageType(def.School))
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
	dmg = applyMonsterArmor(dmg, "physical", target.ArmorClass, isRanged)
	target.TakeDamage(dmg, monsterPkg.DamagePhysical)
}

func trySimMonsterAllyHeal(m *monsterPkg.Monster3D, monsters []*monsterPkg.Monster3D, cfg *config.Config) bool {
	if m.AllyHealChance <= 0 || m.AllyHealAmount <= 0 || rand.Float64() >= m.AllyHealChance {
		return false
	}
	radius := m.AllyHealRadiusPixels
	if radius <= 0 {
		radius = 2 * float64(cfg.GetTileSize())
	}
	var target *monsterPkg.Monster3D
	for _, candidate := range monsters {
		if candidate == nil || !candidate.IsAlive() || candidate.HitPoints >= candidate.MaxHitPoints {
			continue
		}
		if candidate.Bound != m.Bound {
			continue
		}
		if candidate != m && Distance(m.X, m.Y, candidate.X, candidate.Y) > radius {
			continue
		}
		if target == nil || candidate.HitPoints*target.MaxHitPoints < target.HitPoints*candidate.MaxHitPoints {
			target = candidate
		}
	}
	if target == nil {
		return false
	}
	target.HitPoints += m.AllyHealAmount
	if target.HitPoints > target.MaxHitPoints {
		target.HitPoints = target.MaxHitPoints
	}
	return true
}

func trySimMonsterPiercingShot(cs *CombatSystem, m *monsterPkg.Monster3D, party []*character.MMCharacter) bool {
	if m.PiercingShotChance <= 0 || rand.Float64() >= m.PiercingShotChance {
		return false
	}
	alive := make([]*character.MMCharacter, 0, len(party))
	for _, c := range party {
		if c.CanAct() {
			alive = append(alive, c)
		}
	}
	if len(alive) == 0 {
		return false
	}
	targets := m.PiercingShotTargets
	if targets <= 0 {
		targets = 2
	}
	if targets > len(alive) {
		targets = len(alive)
	}
	rand.Shuffle(len(alive), func(i, j int) { alive[i], alive[j] = alive[j], alive[i] })
	for _, target := range alive[:targets] {
		dmg := cs.mitigateCharacterDamage(m.GetAttackDamage(), "physical", target, true)
		target.HitPoints -= dmg
		if target.HitPoints < 0 {
			target.HitPoints = 0
		}
		if target.HitPoints == 0 {
			target.AddCondition(character.ConditionUnconscious)
		}
	}
	return true
}

func monsterActOnce(cs *CombatSystem, m *monsterPkg.Monster3D, party []*character.MMCharacter, monsters []*monsterPkg.Monster3D) {
	attacks := m.GetTurnBasedAttackCount()
	for i := 0; i < attacks; i++ {
		if trySimMonsterAllyHeal(m, monsters, cs.game.config) || trySimMonsterPiercingShot(cs, m, party) {
			continue
		}
		alive := make([]*character.MMCharacter, 0, len(party))
		for _, c := range party {
			if c.CanAct() {
				alive = append(alive, c)
			}
		}
		if len(alive) == 0 {
			return
		}
		// Dragon Breath: a DragonBreathChance proc (33%) that REPLACES this attack
		// with a whole-party hit of DragonBreathDamageType (runtime
		// tryMonsterDragonBreath returns before the normal attack). TrueDamage is
		// folded in like every monsterHitCharacter hit and IgnoresArmor carries
		// through. NOT tied to ProjectileSpell - green dragons (no projectile)
		// breathe too, and the element is the breath's, not the projectile's.
		if m.DragonBreathChance > 0 && rand.Float64() < m.DragonBreathChance {
			breathType := normalizeDamageTypeStr(m.DragonBreathDamageType)
			breathDmg := m.GetAttackDamage()
			for _, c := range alive {
				d := cs.mitigateCharacterDamage(breathDmg, breathType, c, m.IgnoresArmor) + m.TrueDamage
				c.HitPoints -= d
				if c.HitPoints < 0 {
					c.HitPoints = 0
				}
				if c.HitPoints == 0 {
					c.AddCondition(character.ConditionUnconscious)
				}
			}
			continue
		}
		target := alive[rand.Intn(len(alive))]
		var dmg int
		if m.HasRangedAttack() && m.ProjectileSpell != "" {
			// A ranged monster ALWAYS uses its elemental breath (combat.go dispatch
			// uses ranged whenever HasRangedAttack, even point-blank - it never
			// melees). Mitigate it by the breath's element so the target's armor
			// (elemental cap), resists, and buffs actually apply.
			school := "physical"
			if d, ok := config.GetSpellDefinition(m.ProjectileSpell); ok && d != nil && d.School != "" {
				school = d.School
			}
			dmg = cs.mitigateCharacterDamage(m.GetAttackDamage(), school, target, false)
		} else {
			dmg = cs.mitigateCharacterDamage(m.GetAttackDamage(), "physical", target, m.IgnoresArmor)
		}
		dmg += m.TrueDamage // bypasses all mitigation, folded into the hit
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

func liveSummonsForBoss(monsters []*monsterPkg.Monster3D, boss *monsterPkg.Monster3D) int {
	count := 0
	for _, m := range monsters {
		if m != nil && m.IsAlive() && m.SummonedBy == boss.ID {
			count++
		}
	}
	return count
}

func trySimBossSummon(monsters *[]*monsterPkg.Monster3D, boss *monsterPkg.Monster3D, cfg *config.Config) {
	if boss == nil || !boss.IsAlive() || len(boss.SummonMonsters) == 0 {
		return
	}
	guaranteed := boss.SummonFirstGuaranteed && !boss.SummonFirstDone
	if !guaranteed && (boss.SummonChance <= 0 || rand.Float64() >= boss.SummonChance) {
		return
	}
	live := liveSummonsForBoss(*monsters, boss)
	if boss.SummonMax > 0 && live >= boss.SummonMax {
		return
	}
	count := boss.SummonCount
	if count <= 0 {
		count = 1
	}
	if boss.SummonMax > 0 && live+count > boss.SummonMax {
		count = boss.SummonMax - live
	}
	for i := 0; i < count; i++ {
		key := boss.SummonMonsters[rand.Intn(len(boss.SummonMonsters))]
		add := monsterPkg.NewMonster3DFromConfig(0, 0, key, cfg)
		add.WasAttacked = true
		add.IsEngagingPlayer = true
		add.SummonedBy = boss.ID
		*monsters = append(*monsters, add)
	}
	boss.SummonFirstDone = true
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

func turnBasedActionSlotsForParty(party []*character.MMCharacter) map[*character.MMCharacter]int {
	slots := make(map[*character.MMCharacter]int, len(party))
	bonusActions := 0
	for _, c := range party {
		if !c.CanAct() {
			continue
		}
		slots[c] = 1
		if tier := c.SpeedBonusActionTier(); tier > bonusActions {
			bonusActions = tier
		}
	}
	for bonusActions > 0 {
		var best *character.MMCharacter
		bestSpeed := -1
		for _, c := range party {
			if !c.CanAct() || slots[c] == 0 || slots[c] > 1 {
				continue
			}
			speed := c.GetEffectiveSpeed()
			if speed > bestSpeed {
				best = c
				bestSpeed = speed
			}
		}
		if best == nil {
			return slots
		}
		slots[best]++
		bonusActions--
	}
	return slots
}

// runOneFight runs a single combat between party and monsters; both slices
// are mutated. Returns whether the monsters were all killed, whether the
// party was wiped, and how many rounds elapsed.
func runOneFight(cs *CombatSystem, party []*character.MMCharacter, monsters []*monsterPkg.Monster3D) (killed, wiped bool, rounds int) {
	const maxRounds = 30
	for ; rounds < maxRounds; rounds++ {
		// Party turn - each char uses their action slots on the
		// lowest-HP alive monster (focus-fire).
		actionSlots := turnBasedActionSlotsForParty(party)
		for _, c := range party {
			if !c.CanAct() {
				continue
			}
			slots := actionSlots[c]
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
			rounds++
			return
		}
		// Each surviving monster acts.
		for _, m := range monsters {
			if !m.IsAlive() {
				continue
			}
			trySimBossSummon(&monsters, m, cs.game.config)
			monsterActOnce(cs, m, party, monsters)
			if partyAllDown(party) {
				break
			}
		}
		if partyAllDown(party) {
			wiped = true
			return
		}
	}
	wiped = partyAllDown(party)
	return
}

// simulateFight runs one or more monsters against the party, repeated N
// times. Caller passes monsterKeys with duplicates to spawn multiples
// (e.g., ["goblin", "goblin"] for a 2-goblin encounter).
func simulateFight(t *testing.T, cs *CombatSystem, party []*character.MMCharacter, monsterKeys []string, blessActive bool, trials int) simResult {
	var totalRoundsWin, wins, wipes int
	var sumHPLeft float64

	cs.game.statBuffs = nil
	if blessActive {
		// Novice-tier Bless from spells.yaml, effectively permanent for the sim.
		cs.game.addStatBuff(TimedStatBuff{SpellID: "bless", Frames: 1 << 30, Bonuses: character.UniformStatBonuses(10)})
	} else {
		cs.game.recomputeStatBonuses()
	}
	defer func() {
		cs.game.statBuffs = nil
		cs.game.recomputeStatBonuses()
	}()

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
		killed, _, round := runOneFight(cs, party, monsters)
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
		// MaxSP includes EFFECTIVE Intellect+Personality/3 (equipment counts):
		// Lysander's magic_ring (+Int via intellect_scaling_divisor) now adds SP.
		{"Gareth", 65, 23},
		{"Lysander", 45, 39},
		{"Celestine", 45, 30},
		{"Silvelyn", 45, 28},
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
	// Per-level stat investment - applied for every level beyond 1.
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
	// L3 unlocks one mastery/spell choice from level_up.yaml - pick the
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
	t.Logf("%s - %d trials each.", header, trials)
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

func mustEquipItemKey(t *testing.T, c *character.MMCharacter, key string) {
	t.Helper()
	it := items.CreateItemFromYAML(key)
	if _, _, ok := c.EquipItem(it); !ok {
		t.Fatalf("%s: failed to equip %s", c.Name, key)
	}
}

func mustEquipWeaponKey(t *testing.T, c *character.MMCharacter, key string) {
	t.Helper()
	c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(key)
}

func mustEquipSpellID(t *testing.T, c *character.MMCharacter, spellID spells.SpellID) {
	t.Helper()
	if !characterKnowsSpellByID(c, spellID) && !addSpellByID(c, spellID) {
		t.Fatalf("%s: failed to add spell %s", c.Name, spellID)
	}
	sp, err := spells.CreateSpellItem(spellID)
	if err != nil {
		t.Fatalf("%s: create spell %s: %v", c.Name, spellID, err)
	}
	c.Equipment[items.SlotSpell] = sp
}

// buildCastleEntryL10Party models a fixed level-10 party entering the Japanese
// castle with late midgame gear, but before any castle/rack/boss loot. Keeping it
// deterministic makes the castle balance table easier to compare across changes.
func buildCastleEntryL10Party(t *testing.T, cs *CombatSystem) []*character.MMCharacter {
	t.Helper()
	cs.game.party = character.NewParty(cs.game.config)
	party := buildDefaultParty(t, cs, 10)
	cs.game.party.Members = party

	mustEquipWeaponKey(t, party[0], "silver_sword")
	mustEquipItemKey(t, party[0], "iron_armor")
	mustEquipItemKey(t, party[0], "iron_helmet")
	mustEquipItemKey(t, party[0], "iron_pants")
	mustEquipItemKey(t, party[0], "bearpaw_gauntlets")
	mustEquipItemKey(t, party[0], "belt_of_strength")
	mustEquipItemKey(t, party[0], "direpelt_cloak")

	mustEquipWeaponKey(t, party[1], "battle_staff")
	mustEquipSpellID(t, party[1], "fireball")
	mustEquipSpellID(t, party[1], "darkbolt")
	mustEquipSpellID(t, party[1], "ice_bolt")
	mustEquipItemKey(t, party[1], "wizard_robe")
	mustEquipItemKey(t, party[1], "magic_ring")
	mustEquipItemKey(t, party[1], "serpentscale_gauntlets")

	mustEquipWeaponKey(t, party[2], "steel_mace")
	mustEquipSpellID(t, party[2], "heal")
	mustEquipItemKey(t, party[2], "chain_armor")
	mustEquipItemKey(t, party[2], "chain_helmet")
	mustEquipItemKey(t, party[2], "chain_pants")
	mustEquipItemKey(t, party[2], "tideclasp_gauntlets")
	mustEquipItemKey(t, party[2], "scarab_amulet")

	mustEquipWeaponKey(t, party[3], "elven_bow")
	mustEquipItemKey(t, party[3], "leather_armor")
	mustEquipItemKey(t, party[3], "leather_helmet")
	mustEquipItemKey(t, party[3], "leather_pants")
	mustEquipItemKey(t, party[3], "belt_of_speed")
	mustEquipItemKey(t, party[3], "puma_claw")
	mustEquipItemKey(t, party[3], "batwing_cloak")

	for _, c := range party {
		c.CalculateDerivedStats(cs.game.config)
		c.HitPoints = c.MaxHitPoints
		c.SpellPoints = c.MaxSpellPoints
	}
	cs.game.party.AddItem(items.CreateItemFromYAML("health_potion"))
	cs.game.party.AddItem(items.CreateItemFromYAML("revival_potion"))
	return party
}

// buildCastleVeteranL17Party models a higher-level party returning to the castle
// with common castle gear and late-zone tools, but without Samurai boss uniques.
func buildCastleVeteranL17Party(t *testing.T, cs *CombatSystem) []*character.MMCharacter {
	t.Helper()
	cs.game.party = character.NewParty(cs.game.config)
	party := buildDefaultParty(t, cs, 17)
	cs.game.party.Members = party

	mustEquipWeaponKey(t, party[0], "katana")
	mustEquipItemKey(t, party[0], "o_yoroi")
	mustEquipItemKey(t, party[0], "kabuto")
	mustEquipItemKey(t, party[0], "silk_hakama")
	mustEquipItemKey(t, party[0], "bearpaw_gauntlets")
	mustEquipItemKey(t, party[0], "belt_of_strength")
	mustEquipItemKey(t, party[0], "haori_cloak")
	mustEquipItemKey(t, party[0], "jade_netsuke")

	mustEquipWeaponKey(t, party[1], "archmage_staff")
	mustEquipSpellID(t, party[1], "fireball")
	mustEquipSpellID(t, party[1], "darkbolt")
	mustEquipSpellID(t, party[1], "ice_bolt")
	mustEquipSpellID(t, party[1], "lightning")
	mustEquipSpellID(t, party[1], "disintegrate")
	mustEquipSpellID(t, party[1], "inferno")
	mustEquipSpellID(t, party[1], "starburst")
	mustEquipSpellID(t, party[1], "hot_steam")
	mustEquipItemKey(t, party[1], "archmage_robe")
	mustEquipItemKey(t, party[1], "serpentscale_gauntlets")
	mustEquipItemKey(t, party[1], "haori_cloak")
	mustEquipItemKey(t, party[1], "onmyoji_talisman")
	mustEquipItemKey(t, party[1], "jade_netsuke")

	mustEquipWeaponKey(t, party[2], "kanabo")
	mustEquipSpellID(t, party[2], "heal")
	mustEquipSpellID(t, party[2], "heal_other")
	mustEquipSpellID(t, party[2], "mass_heal")
	mustEquipItemKey(t, party[2], "chain_armor")
	mustEquipItemKey(t, party[2], "chain_helmet")
	mustEquipItemKey(t, party[2], "chain_pants")
	mustEquipItemKey(t, party[2], "tideclasp_gauntlets")
	mustEquipItemKey(t, party[2], "haori_cloak")
	mustEquipItemKey(t, party[2], "jade_netsuke")
	mustEquipItemKey(t, party[2], "magic_ring")

	mustEquipWeaponKey(t, party[3], "tanegashima")
	mustEquipItemKey(t, party[3], "leather_armor")
	mustEquipItemKey(t, party[3], "leather_helmet")
	mustEquipItemKey(t, party[3], "silk_hakama")
	mustEquipItemKey(t, party[3], "serpentscale_gauntlets")
	mustEquipItemKey(t, party[3], "belt_of_speed")
	mustEquipItemKey(t, party[3], "haori_cloak")
	mustEquipItemKey(t, party[3], "accuracy_talisman")

	for _, c := range party {
		c.CalculateDerivedStats(cs.game.config)
		c.HitPoints = c.MaxHitPoints
		c.SpellPoints = c.MaxSpellPoints
	}
	cs.game.party.AddItem(items.CreateItemFromYAML("health_potion"))
	cs.game.party.AddItem(items.CreateItemFromYAML("health_potion"))
	cs.game.party.AddItem(items.CreateItemFromYAML("revival_potion"))
	cs.game.party.AddItem(items.CreateItemFromYAML("revival_potion"))
	return party
}

// buildRealPlayParty reconstructs the EXACT party from save1 - the author's real
// playthrough: same classes, levels, BASE stats (gear bonuses apply on top, as
// in-game), gear and spell books. Gives the zone tables a real progression party
// alongside the synthetic entry/veteran loadouts. Silvelyn's wizard_eye is a
// vision utility, kept in her book but NOT her active spell slot, so the sim fires
// her bow each round instead of "casting" a no-op heal (how she actually fights).
// Lysander's active spell is left to equipBestOffensiveSpellVs (runFixedPartySim
// re-picks the best offensive spell he knows per fight, as in real play).
func buildRealPlayParty(t *testing.T, cs *CombatSystem) []*character.MMCharacter {
	t.Helper()
	cs.game.party = character.NewParty(cs.game.config)
	cfg := cs.game.config

	type member struct {
		name                                                            string
		class                                                           character.CharacterClass
		level                                                           int
		might, intellect, personality, endurance, accuracy, speed, luck int
		weapon                                                          string
		gear                                                            []string
		known                                                           []spells.SpellID
		equipSpell                                                      spells.SpellID
	}
	roster := []member{
		{"Gareth", character.ClassKnight, 13, 56, 10, 10, 28, 12, 16, 10,
			"tonbogiri", []string{"iron_armor", "chain_pants", "iron_helmet"}, nil, ""},
		{"Lysander", character.ClassSorcerer, 14, 8, 66, 12, 16, 10, 16, 12,
			"battle_staff", []string{"direpelt_cloak", "leather_armor", "leather_pants", "sonar_pendant", "magic_ring"},
			[]spells.SpellID{"torch_light", "firebolt", "ice_bolt", "hot_steam", "sparks", "starburst", "day_of_the_gods", "ray_of_light"}, ""},
		{"Celestine", character.ClassCleric, 13, 10, 12, 59, 22, 8, 16, 10,
			"holy_mace", []string{"chain_armor"},
			[]spells.SpellID{"heal_other", "harm", "mind_blast", "spirit_lash", "bless", "heroism"}, "mass_heal"},
		{"Silvelyn", character.ClassArcher, 13, 12, 26, 10, 18, 47, 16, 10,
			"elven_bow", []string{"ratskin_boots"}, []spells.SpellID{"wizard_eye"}, ""},
	}
	party := make([]*character.MMCharacter, 0, len(roster))
	for _, m := range roster {
		c := character.CreateCharacter(m.name, m.class, cfg)
		c.Level = m.level
		c.Might, c.Intellect, c.Personality, c.Endurance = m.might, m.intellect, m.personality, m.endurance
		c.Accuracy, c.Speed, c.Luck = m.accuracy, m.speed, m.luck
		mustEquipWeaponKey(t, c, m.weapon)
		for _, k := range m.gear {
			mustEquipItemKey(t, c, k)
		}
		for _, sid := range m.known {
			addSpellByID(c, sid)
		}
		if m.equipSpell != "" {
			mustEquipSpellID(t, c, m.equipSpell)
		}
		c.CalculateDerivedStats(cfg)
		c.HitPoints = c.MaxHitPoints
		c.SpellPoints = c.MaxSpellPoints
		party = append(party, c)
	}
	cs.game.party.Members = party
	cs.game.party.AddItem(items.CreateItemFromYAML("health_potion"))
	cs.game.party.AddItem(items.CreateItemFromYAML("health_potion"))
	cs.game.party.AddItem(items.CreateItemFromYAML("revival_potion"))
	return party
}

func runFixedPartySim(t *testing.T, cs *CombatSystem, build func(t *testing.T, cs *CombatSystem) []*character.MMCharacter, monsterKeys []string, label string, statBonus int, trials int) {
	t.Helper()
	var wins, wipes int
	var sumRoundsWin int
	var sumHPLeft, sumEnemyHPLeftOnWipe float64
	var sumLevel float64
	var enemyMaxHPSum int

	for trial := 0; trial < trials; trial++ {
		party := build(t, cs)
		monsters := make([]*monsterPkg.Monster3D, 0, len(monsterKeys))
		for _, key := range monsterKeys {
			m := monsterPkg.NewMonster3DFromConfig(0, 0, key, cs.game.config)
			m.WasAttacked = true
			m.IsEngagingPlayer = true
			monsters = append(monsters, m)
		}
		for _, c := range party {
			if c.Class == character.ClassSorcerer {
				equipBestOffensiveSpellVs(c, monsters[0])
			}
		}

		cs.game.statBuffs = nil
		cs.game.recomputeStatBonuses()
		if statBonus > 0 {
			cs.game.addStatBuff(TimedStatBuff{SpellID: "bless", Frames: 1 << 30, Bonuses: character.UniformStatBonuses(statBonus)})
		}
		resetPartyForSim(party)

		killed, wiped, rounds := runOneFightWithPotions(cs, party, monsters)
		if killed {
			wins++
			sumRoundsWin += rounds
		}
		if wiped {
			wipes++
			remHP := 0
			totalMax := 0
			for _, m := range monsters {
				remHP += m.HitPoints
				totalMax += m.MaxHitPoints
			}
			if totalMax > 0 {
				sumEnemyHPLeftOnWipe += float64(remHP) / float64(totalMax)
			}
		}
		sumHPLeft += partyHPRatio(party)
		levelSum := 0
		for _, c := range party {
			levelSum += c.Level
		}
		sumLevel += float64(levelSum) / float64(len(party))
		if trial == 0 {
			for _, m := range monsters {
				enemyMaxHPSum += m.MaxHitPoints
			}
		}
	}
	cs.game.statBuffs = nil
	cs.game.recomputeStatBonuses()

	avgRoundsWin := 0.0
	if wins > 0 {
		avgRoundsWin = float64(sumRoundsWin) / float64(wins)
	}
	t.Logf("%s - %d trials, fixed party", label, trials)
	t.Logf("  avg party level: %.1f", sumLevel/float64(trials))
	t.Logf("  wins:  %d/%d  (%.0f%%)", wins, trials, float64(wins)*100/float64(trials))
	t.Logf("  wipes: %d/%d  (%.0f%%)", wipes, trials, float64(wipes)*100/float64(trials))
	if unresolved := trials - wins - wipes; unresolved > 0 {
		t.Logf("  unresolved: %d/%d  (%.0f%%)", unresolved, trials, float64(unresolved)*100/float64(trials))
	}
	if wins > 0 {
		t.Logf("  avg rounds to kill all enemies: %.1f", avgRoundsWin)
	}
	if wipes > 0 {
		t.Logf("  avg enemy HP remaining on wipe: %.0f%% (of %d base total)",
			sumEnemyHPLeftOnWipe*100/float64(wipes), enemyMaxHPSum)
	}
	t.Logf("  avg party HP left: %.0f%%", sumHPLeft*100/float64(trials))
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
// biome. Medusa (m) and octopus (o-water) are excluded - they are biome=water.
var forestMonsters = []string{
	"goblin", "orc", "wolf", "bear", "spider", "skeleton", "troll",
	"forest_orc", "dire_wolf", "forest_spider", "treant", "pixie",
	"bandit", "alien", "dragon",
}

// countMonsterSpawns parses a .map file and returns monster_key -> count
// using the biome-aware letter resolution from monster_config. Mirrors
// map_loader's parsing rules: skip comment lines ('#'), strip the
// `  >[npc:...]`/`[stile:...]` suffix from each line (otherwise the YAML
// keys inside NPC/stile names get counted as monster spawns).
func countMonsterSpawns(mapPath, biome string) (map[string]int, error) {
	data, err := os.ReadFile(mapPath)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "  >"); i != -1 {
			line = line[:i]
		}
		for _, ch := range line {
			if ch < 'a' || ch > 'z' {
				continue
			}
			_, key, err := monsterPkg.MonsterConfig.GetMonsterByLetterForBiome(string(ch), biome)
			if err != nil {
				continue
			}
			counts[key]++
		}
	}
	return counts, nil
}

// statStrategy spends one stat point. Used at level-up to model
// different player builds (all-speed vs casters-only-speed etc).
type statStrategy func(c *character.MMCharacter)

// statSpeedFloorThenPrimary: Spd->16, End->15, then class primary.
func statSpeedFloorThenPrimary(c *character.MMCharacter) {
	primary := classPrimaryStat(c)
	switch {
	case c.Speed < 16:
		c.Speed++
	case c.Endurance < 15:
		c.Endurance++
	default:
		*primary++
	}
}

// statSpeedOnlyForCasters: Sorc + Archer follow the speed-floor path;
// Knight and Cleric skip Speed and only build End->15 -> primary.
func statSpeedOnlyForCasters(c *character.MMCharacter) {
	primary := classPrimaryStat(c)
	switch c.Class {
	case character.ClassSorcerer, character.ClassArcher:
		statSpeedFloorThenPrimary(c)
	default:
		// Knight/Cleric: End->15 floor, then primary. No Speed.
		switch {
		case c.Endurance < 15:
			c.Endurance++
		default:
			*primary++
		}
	}
}

func classPrimaryStat(c *character.MMCharacter) *int {
	switch c.Class {
	case character.ClassKnight:
		return &c.Might
	case character.ClassSorcerer:
		return &c.Intellect
	case character.ClassCleric:
		return &c.Personality
	case character.ClassArcher:
		return &c.Accuracy
	}
	return &c.Might
}

// --------------------------------------------------------------------------
// Verbose simulation tracing (opt-in). When simTrace is non-nil, the build +
// fight helpers emit a detailed lifecycle log: loot drops, level-ups, stat
// gains, level-3 picks, equips, inventory contents, and item use. Tests turn
// it on for a single trial so the rest of the suite stays quiet. tracef is a
// no-op when disabled.
var simTrace func(format string, args ...any)

func tracef(format string, args ...any) {
	if simTrace != nil {
		simTrace(format, args...)
	}
}

// l3PicksThisTrial collects the level-3 choices made during the current build
// ("Name->pick") so runEndgameSim can log, per trial, what each char picked.
// Reset at the start of every trial; appended to by applyRandomL3Choice.
var l3PicksThisTrial []string

type statSnap struct{ might, intellect, personality, endurance, accuracy, speed, luck int }

func snapStats(c *character.MMCharacter) statSnap {
	return statSnap{c.Might, c.Intellect, c.Personality, c.Endurance, c.Accuracy, c.Speed, c.Luck}
}

// statDeltaStr lists the stats that rose between two snapshots, e.g.
// "+3 Speed, +2 Endurance".
func statDeltaStr(a, b statSnap) string {
	var parts []string
	add := func(name string, before, after int) {
		if after > before {
			parts = append(parts, fmt.Sprintf("+%d %s", after-before, name))
		}
	}
	add("Might", a.might, b.might)
	add("Intellect", a.intellect, b.intellect)
	add("Personality", a.personality, b.personality)
	add("Endurance", a.endurance, b.endurance)
	add("Accuracy", a.accuracy, b.accuracy)
	add("Speed", a.speed, b.speed)
	add("Luck", a.luck, b.luck)
	if len(parts) == 0 {
		return "(no stat change)"
	}
	return strings.Join(parts, ", ")
}

// applyRandomL3Choice picks one of the character's real level-3 options from
// level_up.yaml at random (each run differs) and applies it through the same
// code the game uses (addSpellByID / upgradeSkillMastery / upgradeMagicMastery).
// Records the pick for per-trial logging and traces it.
func applyRandomL3Choice(c *character.MMCharacter) {
	choices := config.GetLevelUpChoices(c.GetClassKey(), 3)
	if len(choices) == 0 {
		return
	}
	choice := choices[rand.Intn(len(choices))]
	desc := applyLevelUpChoice(c, choice)
	l3PicksThisTrial = append(l3PicksThisTrial, fmt.Sprintf("%s->%s", c.Name, desc))
	tracef("      L3 choice: %s picks %s", c.Name, desc)
}

// applyLevelUpChoice applies a single level-up choice via the production
// appliers and returns a short description of what was gained. magic_mastery
// "any" resolves to a random known school (the game lets the player choose).
func applyLevelUpChoice(c *character.MMCharacter, choice config.LevelUpChoice) string {
	switch strings.ToLower(choice.Type) {
	case "spell":
		sid := spells.SpellID(choice.Spell)
		addSpellByID(c, sid)
		return "spell " + spellDisplayName(sid)
	case "weapon_mastery", "armor_mastery":
		if st, ok := skillTypeFromKey(choice.Skill); ok {
			upgradeSkillMastery(c, st)
			return st.String() + " mastery"
		}
	case "magic_mastery":
		school := character.MagicSchoolID(choice.School)
		if strings.ToLower(choice.School) == "any" {
			known := make([]character.MagicSchoolID, 0, len(c.MagicSchools))
			for s := range c.MagicSchools {
				known = append(known, s)
			}
			if len(known) == 0 {
				return "magic mastery (no school)"
			}
			school = known[rand.Intn(len(known))]
		}
		upgradeMagicMastery(c, school)
		return school.DisplayName() + " magic mastery"
	}
	return string(choice.Type)
}

// traceInventory logs a count-by-name summary of the party inventory.
func traceInventory(p *character.Party) {
	counts := map[string]int{}
	var order []string
	for _, it := range p.Inventory {
		if _, ok := counts[it.Name]; !ok {
			order = append(order, it.Name)
		}
		counts[it.Name]++
	}
	sort.Strings(order)
	tracef("  inventory after build (%d items):", len(p.Inventory))
	for _, name := range order {
		tracef("    %2d x %s", counts[name], name)
	}
}

// traceLoadout logs each member's final equipped gear and derived caps.
func traceLoadout(party []*character.MMCharacter) {
	tracef("  final loadout:")
	for _, c := range party {
		var eq []string
		for _, it := range c.Equipment {
			if it.Name != "" {
				eq = append(eq, it.Name)
			}
		}
		sort.Strings(eq)
		tracef("    %s (L%d %s) HP %d SP %d: %s",
			c.Name, c.Level, c.GetClassKey(), c.MaxHitPoints, c.MaxSpellPoints, strings.Join(eq, ", "))
	}
}

// TestBalanceSimUsesLiveXPCurve pins the simulator's level-up loop to the
// SHIPPING curve past the L13 quadratic crossover - a drifted sim silently
// over-levels every endgame balance scenario.
func TestBalanceSimUsesLiveXPCurve(t *testing.T) {
	cfg := loadTestConfig(t)
	c := character.CreateCharacter("SimPin", character.ClassKnight, cfg)
	applyXPAndLevelUp(c, cfg, xpSpentToReach(20)+50, statSpeedFloorThenPrimary)
	if c.Level != 20 || c.Experience != 50 {
		t.Fatalf("sim curve drifted from live xpStepCost: got L%d rem %d, want L20 rem 50", c.Level, c.Experience)
	}
}

// applyXPAndLevelUp grants experience to a character and applies as
// many level-ups as the total XP allows. Each level: spend
// StatPointsPerLevel points via the given strategy, apply L3
// mastery/spell choice, recalc derived stats.
func applyXPAndLevelUp(c *character.MMCharacter, cfg *config.Config, xpGained int, strategy statStrategy) {
	c.Experience += xpGained
	for {
		required := xpStepCost(c.Level) // the LIVE curve (quadratic from L13), not the old linear cost
		if c.Experience < required {
			break
		}
		c.Experience -= required
		c.Level++
		before := snapStats(c)
		for p := 0; p < StatPointsPerLevel; p++ {
			strategy(c)
		}
		tracef("    %s reaches L%d: %s", c.Name, c.Level, statDeltaStr(before, snapStats(c)))
		// Level 3 is the only level with a player choice in level_up.yaml.
		// Pick one of the real options at random so each run differs (e.g.
		// Sorcerer rolls fireball vs darkbolt, Knight sword vs spear).
		if c.Level == 3 {
			applyRandomL3Choice(c)
		}
		c.CalculateDerivedStats(cfg)
	}
}

// classArmorPriority returns the best-fit armor category for a class.
// Used to break ties when multiple items target the same slot.
func classArmorPriority(class character.CharacterClass) []string {
	switch class {
	case character.ClassKnight:
		return []string{"plate", "chain", "leather", "cloth"}
	case character.ClassCleric:
		return []string{"chain", "leather", "plate", "cloth"}
	case character.ClassArcher:
		return []string{"leather", "chain", "cloth", "plate"}
	case character.ClassSorcerer:
		return []string{"cloth", "leather", "chain", "plate"}
	}
	return []string{"leather", "chain", "plate", "cloth"}
}

func armorScore(item items.Item, preferOrder []string) int {
	cat := item.ArmorCategory
	for i, want := range preferOrder {
		if cat == want {
			return 100 - i*10 + item.Attributes["armor_class_base"]
		}
	}
	return item.Attributes["armor_class_base"]
}

// autoEquipPool greedily assigns loot to party members. Each char is given
// a chance in turn to grab the best-fitting item; the pool shrinks as
// items are equipped. Items the char can't equip (wrong category) are
// skipped for that char.
func autoEquipPool(party []*character.MMCharacter, pool []items.Item) []items.Item {
	leftover := make([]items.Item, 0, len(pool))
	for _, it := range pool {
		// Skip non-equippable types upfront.
		switch it.Type {
		case items.ItemConsumable, items.ItemQuest, items.ItemTrinket, items.ItemBattleSpell, items.ItemUtilitySpell:
			leftover = append(leftover, it)
			continue
		}
		// Find best candidate among party (highest armorScore or weapon damage).
		bestChar := -1
		bestScore := -1
		for i, c := range party {
			if it.Type == items.ItemArmor && !c.CanEquipArmor(it) {
				continue
			}
			if it.Type == items.ItemWeapon && !c.CanEquipWeaponByName(it.Name) {
				continue
			}
			score := 0
			switch it.Type {
			case items.ItemArmor:
				score = armorScore(it, classArmorPriority(c.Class))
			case items.ItemWeapon:
				if def := lookupWeaponConfigByName(it.Name); def != nil {
					score = def.Damage * 10
				}
			case items.ItemAccessory:
				// Accessories - heuristic by class fit
				switch c.Class {
				case character.ClassKnight:
					score = it.Attributes["bonus_might"]*5 + it.Attributes["bonus_endurance"]*4
				case character.ClassSorcerer:
					score = it.Attributes["bonus_intellect"]*5 + it.Attributes["intellect_scaling_divisor"]*3
				case character.ClassCleric:
					score = it.Attributes["bonus_personality"]*5 + it.Attributes["personality_scaling_divisor"]*3
				case character.ClassArcher:
					score = it.Attributes["bonus_accuracy"]*5 + it.Attributes["bonus_speed"]*4 + it.Attributes["bonus_luck"]*2
				}
			}
			// Compare against currently-equipped item in the target slot.
			// EquipItem auto-routes; if there's already something better there,
			// new item displaces it but we want to keep the better one.
			// Simple heuristic: score above the threshold of "interesting"
			// before we let it swap.
			if score > bestScore {
				bestScore = score
				bestChar = i
			}
		}
		if bestChar < 0 || bestScore <= 0 {
			leftover = append(leftover, it)
			continue
		}
		previous, hadPrev, ok := party[bestChar].EquipItem(it)
		if !ok {
			leftover = append(leftover, it)
			continue
		}
		// If swap displaced something of higher quality, swap back. Cheap
		// way to avoid downgrades: compare armor_class_base.
		if hadPrev && it.Type == items.ItemArmor {
			prevAC := previous.Attributes["armor_class_base"]
			newAC := it.Attributes["armor_class_base"]
			if prevAC > newAC {
				// Undo: put `it` to leftover, restore previous
				delete(party[bestChar].Equipment, items.EquipSlot(it.Attributes["equip_slot"]))
				party[bestChar].EquipItem(previous)
				leftover = append(leftover, it)
				continue
			}
		}
		// Otherwise prev item goes back to leftover for other chars or discard.
		tracef("    equip: %-18s -> %s", party[bestChar].Name, it.Name)
		if hadPrev {
			tracef("      (replaced %s, back to pool)", previous.Name)
			leftover = append(leftover, previous)
		}
	}
	return leftover
}

// dragonKeys are the 4 main-quest dragons (one per desert corner). They all
// share name "Dragon" but differ by element: base=fire, red=dark,
// green=poison/nature, gold=air. Never auto-cleared during a grind - they're
// the endgame fight, tested explicitly with the right spell per resist profile.
var dragonKeys = []string{"dragon", "dragon_red", "dragon_green", "dragon_gold"}

// elderDragonKeys are the ELITE statue-summoned dragons (the actual endgame
// fight). The VsDragon sims target these; both base and elite are kept out of
// the grind XP/loot (they're never farmed).
var elderDragonKeys = []string{"elder_dragon", "elder_dragon_red", "elder_dragon_green", "elder_dragon_gold"}

// grindMaps simulates clearing all given maps (path, biome) end-to-end:
// awards XP to each party member (split /4), rolls loot per kill, plus
// the church_skeleton_chest weapon if the church map is in the list.
// The dragon is excluded from the grind (it's the target of the
// upcoming fight). Statpoints follow `strategy`. Returns the loot pool
// that didn't fit into anyone's equipment.
func grindMaps(t *testing.T, cs *CombatSystem, party []*character.MMCharacter, maps []struct{ path, biome string }, strategy statStrategy) []items.Item {
	t.Helper()
	cfg := cs.game.config
	var allDrops []items.Item
	hasChurch := false

	for _, mapInfo := range maps {
		if mapInfo.biome == "church" {
			hasChurch = true
		}
		counts, err := countMonsterSpawns(mapInfo.path, mapInfo.biome)
		if err != nil {
			t.Fatalf("count %s: %v", mapInfo.path, err)
		}
		for _, dk := range dragonKeys { // base dragons = endgame target, never auto-cleared
			delete(counts, dk)
		}
		for _, dk := range elderDragonKeys { // elite dragons are never farmed either
			delete(counts, dk)
		}
		// Mirror the live kill path: each member gets monster.Experience/len(party)
		// floored PER kill (not summed-then-divided), matching combat.go's
		// awardExperienceAndGold.
		xpPerMember := 0
		for monsterKey, n := range counts {
			def, err := monsterPkg.MonsterConfig.GetMonsterByKey(monsterKey)
			if err != nil || def == nil {
				continue
			}
			xpPerMember += (def.Experience / len(party)) * n
			for i := 0; i < n; i++ {
				m := monsterPkg.NewMonster3DFromConfig(0, 0, monsterKey, cfg)
				drops := cs.checkMonsterLootDrop(m)
				for _, d := range drops {
					tracef("    drop: %-24s (from %s)", d.Name, monsterKey)
				}
				allDrops = append(allDrops, drops...)
			}
		}
		for _, c := range party {
			applyXPAndLevelUp(c, cfg, xpPerMember, strategy)
		}
	}

	if hasChurch {
		// Church skeleton chest: 1 random weapon.
		chest := randomWeaponRewards(1)
		for _, d := range chest {
			tracef("    drop: %-24s (church skeleton chest)", d.Name)
		}
		allDrops = append(allDrops, chest...)
	}

	// Shipwreck encounter (forest-side): 2-5 bandits + 500 XP completion.
	// Real game: each member gets per-kill XP (bandit.Experience/len(party)) for
	// the bandits PLUS the full 500 completion XP - awardEncounterRewards grants
	// the encounter XP in full to every living member, it is NOT divided.
	shipwreckBandits := 2 + rand.Intn(4)
	banditDef, _ := monsterPkg.MonsterConfig.GetMonsterByKey("bandit")
	shipwreckPerMember := 500
	if banditDef != nil {
		shipwreckPerMember += (banditDef.Experience / len(party)) * shipwreckBandits
		for i := 0; i < shipwreckBandits; i++ {
			m := monsterPkg.NewMonster3DFromConfig(0, 0, "bandit", cfg)
			drops := cs.checkMonsterLootDrop(m)
			for _, d := range drops {
				tracef("    drop: %-24s (shipwreck bandit)", d.Name)
			}
			allDrops = append(allDrops, drops...)
		}
	}
	for _, c := range party {
		applyXPAndLevelUp(c, cfg, shipwreckPerMember, strategy)
	}

	sort.Slice(allDrops, func(i, j int) bool {
		return allDrops[i].Attributes["armor_class_base"] > allDrops[j].Attributes["armor_class_base"]
	})
	return autoEquipPool(party, allDrops)
}

// equipBestOffensiveSpellVs picks the spell with the highest effective
// damage against `target`, accounting for the monster's resistances by
// school. Used to make casters "prepare the right spell" before a fight
// - e.g. ice_bolt against a fire-90-resistant dragon beats fireball
// despite higher SP cost.
func equipBestOffensiveSpellVs(c *character.MMCharacter, target *monsterPkg.Monster3D) {
	bestID := spells.SpellID("")
	bestScore := 0
	for _, school := range c.MagicSchools {
		if school == nil {
			continue
		}
		for _, sid := range school.KnownSpells {
			def, err := spells.GetSpellDefinitionByID(sid)
			if err != nil || def.IsUtility {
				continue
			}
			base := def.SpellPointsCost * 3 // SpellDamagePerSP
			// Apply monster resistance for this spell's school.
			dmgType := spellSchoolToDamageType(def.School)
			resist := 0
			if r, ok := target.Resistances[dmgType]; ok {
				resist = r
			}
			effective := base * (100 - resist) / 100
			if effective > bestScore {
				bestScore = effective
				bestID = sid
			}
		}
	}
	if bestID == "" {
		return
	}
	if sp, err := spells.CreateSpellItem(bestID); err == nil {
		c.Equipment[items.SlotSpell] = sp
	}
}

// findInventoryItem returns the inventory index of the first item with the
// given key (matched against config), or -1 if not present.
func findInventoryItem(party *character.Party, name string) int {
	for i, it := range party.Inventory {
		if it.Name == name {
			return i
		}
	}
	return -1
}

// tryUsePotions consumes at most one revival_potion and one health_potion
// per call. Revival is used first if anyone is KO'd; healing is used if
// anyone alive has HP < 50% of max.
func tryUsePotions(cs *CombatSystem) {
	party := cs.game.party
	// Revival: find first KO ally, look for revival potion in inventory.
	for i, m := range party.Members {
		if m.CanAct() {
			continue
		}
		idx := findInventoryItem(party, "Revival Potion")
		if idx < 0 {
			break
		}
		cs.game.selectedChar = i
		cs.game.UseConsumableFromInventory(idx, i)
		tracef("    item use: Revival Potion on %s", m.Name)
		break
	}
	// Healing: lowest-HP ally below 50%.
	var hurt *character.MMCharacter
	hurtIdx := -1
	bestRatio := 0.5
	for i, m := range party.Members {
		if !m.CanAct() {
			continue
		}
		r := float64(m.HitPoints) / float64(m.MaxHitPoints)
		if r < bestRatio {
			bestRatio = r
			hurt = m
			hurtIdx = i
		}
	}
	if hurt != nil {
		idx := findInventoryItem(party, "Health Potion")
		if idx >= 0 {
			cs.game.selectedChar = hurtIdx
			cs.game.UseConsumableFromInventory(idx, hurtIdx)
			tracef("    item use: Health Potion on %s (was %.0f%% HP)", hurt.Name, bestRatio*100)
		}
	}
}

// runOneFightWithPotions is runOneFight with one revival_potion + one
// health_potion automatically consumed when conditions warrant it.
// Potions are drawn from cs.game.party.Inventory.
func runOneFightWithPotions(cs *CombatSystem, party []*character.MMCharacter, monsters []*monsterPkg.Monster3D) (killed, wiped bool, rounds int) {
	const maxRounds = 30
	for ; rounds < maxRounds; rounds++ {
		tryUsePotions(cs)
		// Party turn - each char uses their action slots on the
		// lowest-HP alive monster.
		actionSlots := turnBasedActionSlotsForParty(party)
		for _, c := range party {
			if !c.CanAct() {
				continue
			}
			slots := actionSlots[c]
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
			rounds++
			return
		}
		for _, m := range monsters {
			if !m.IsAlive() {
				continue
			}
			trySimBossSummon(&monsters, m, cs.game.config)
			monsterActOnce(cs, m, party, monsters)
			if partyAllDown(party) {
				break
			}
		}
		if partyAllDown(party) {
			wiped = true
			return
		}
	}
	wiped = partyAllDown(party)
	return
}

// buildClearedForestParty installs a fresh L1 party on cs.game, grinds
// the forest+church content (XP + loot to inventory), distributes stat
// points, max-masters the Sorcerer's water school (ice_bolt prep),
// auto-equips every armor/accessory/weapon from inventory by class
// priority. Returns the party slice.
func buildClearedForestParty(t *testing.T, cs *CombatSystem) []*character.MMCharacter {
	return buildClearedMapsParty(t, cs, []struct{ path, biome string }{
		{"../../assets/forest.map", "forest"},
		{"../../assets/church.map", "church"},
	}, statSpeedFloorThenPrimary)
}

// buildClearedMapsParty grinds the given maps with the given stat strategy,
// then equips everything from inventory.
func buildClearedMapsParty(t *testing.T, cs *CombatSystem, maps []struct{ path, biome string }, strategy statStrategy) []*character.MMCharacter {
	t.Helper()
	cs.game.party = character.NewParty(cs.game.config) // starter party with inventory
	party := cs.game.party.Members

	dropsForInventory := grindMaps(t, cs, party, maps, strategy)
	for _, d := range dropsForInventory {
		cs.game.party.AddItem(d)
	}

	// Sorcerer trains water school to Grandmaster (ice_bolt mastery).
	for _, c := range party {
		if c.Class != character.ClassSorcerer {
			continue
		}
		if c.MagicSchools[character.MagicSchoolWater] == nil {
			c.MagicSchools[character.MagicSchoolWater] = &character.MagicSkill{}
		}
		c.MagicSchools[character.MagicSchoolWater].Mastery = character.MasteryGrandMaster
	}

	autoEquipPartyInventory(cs.game.party)
	return party
}

// autoEquipPartyInventory equips items from party.Inventory onto party
// members based on class fit. Walks the inventory repeatedly until no
// more upgrades can be applied.
func autoEquipPartyInventory(party *character.Party) {
	maxIterations := len(party.Inventory) * 2
	for iter := 0; iter < maxIterations; iter++ {
		bestInvIdx := -1
		bestCharIdx := -1
		bestScore := 0
		for invIdx, it := range party.Inventory {
			switch it.Type {
			case items.ItemConsumable, items.ItemQuest, items.ItemTrinket, items.ItemBattleSpell, items.ItemUtilitySpell:
				continue
			}
			for charIdx, c := range party.Members {
				if it.Type == items.ItemArmor && !c.CanEquipArmor(it) {
					continue
				}
				if it.Type == items.ItemWeapon && !c.CanEquipWeaponByName(it.Name) {
					continue
				}
				score := equipFitScore(c, it)
				// Compare against currently-equipped item in target slot.
				targetSlot := items.EquipSlot(it.Attributes["equip_slot"])
				if it.Type == items.ItemWeapon {
					targetSlot = items.SlotMainHand
				}
				if cur, exists := c.Equipment[targetSlot]; exists && targetSlot != 0 {
					curScore := equipFitScore(c, cur)
					if curScore >= score {
						continue
					}
				}
				if score > bestScore {
					bestScore = score
					bestInvIdx = invIdx
					bestCharIdx = charIdx
				}
			}
		}
		if bestInvIdx < 0 {
			break
		}
		equippedName := party.Inventory[bestInvIdx].Name
		party.EquipItemFromInventory(bestInvIdx, bestCharIdx)
		tracef("    equip: %-18s -> %s", party.Members[bestCharIdx].Name, equippedName)
	}
}

// equipFitScore: armor -> AC + class-priority bonus; weapon -> damage;
// accessory -> class-relevant stat bonuses.
func equipFitScore(c *character.MMCharacter, it items.Item) int {
	switch it.Type {
	case items.ItemArmor:
		return armorScore(it, classArmorPriority(c.Class))
	case items.ItemWeapon:
		if def := lookupWeaponConfigByName(it.Name); def != nil {
			return def.Damage * 10
		}
	case items.ItemAccessory:
		switch c.Class {
		case character.ClassKnight:
			return it.Attributes["bonus_might"]*5 + it.Attributes["bonus_endurance"]*4
		case character.ClassSorcerer:
			return it.Attributes["bonus_intellect"]*5 +
				it.Attributes["intellect_scaling_divisor"]*3 +
				it.Attributes["bonus_personality"]*2
		case character.ClassCleric:
			return it.Attributes["bonus_personality"]*5 +
				it.Attributes["personality_scaling_divisor"]*3
		case character.ClassArcher:
			return it.Attributes["bonus_accuracy"]*5 +
				it.Attributes["bonus_speed"]*4 +
				it.Attributes["bonus_luck"]*2
		}
	}
	return 0
}

// runEndgameSim runs N trials of: build a fresh party via `build`,
// fight the given monster set with potions + Bless + Sorcerer spell prep.
// Reports wins/wipes/avg-HP and average enemy HP remaining on wipe.
// When verbose: logs each trial's level-3 picks, and a full lifecycle trace
// (drops, level-ups, stat gains, equips, inventory, item use) for trial 0.
func runEndgameSim(t *testing.T, cs *CombatSystem, build func(t *testing.T, cs *CombatSystem) []*character.MMCharacter, monsterKeys []string, label string, statBonus int, verbose bool) {
	t.Helper()
	const trials = 50
	var wins, wipes int
	var sumRoundsWin int
	var sumHPLeft, sumEnemyHPLeftOnWipe float64
	var sumLevel float64
	var enemyMaxHPSum int

	for trial := 0; trial < trials; trial++ {
		l3PicksThisTrial = l3PicksThisTrial[:0]
		traceThis := verbose && trial == 0
		if traceThis {
			simTrace = t.Logf
			tracef("======== detailed trace - trial 0 - %s ========", label)
			tracef("  [grind: drops -> level-ups -> equips]")
		}

		party := build(t, cs)

		// Spawn monsters first so the caster picks a spell that targets
		// their resistance profile (water vs dragon, water vs alien
		// - both have 0% water resist).
		monsters := make([]*monsterPkg.Monster3D, 0, len(monsterKeys))
		for _, key := range monsterKeys {
			m := monsterPkg.NewMonster3DFromConfig(0, 0, key, cs.game.config)
			m.WasAttacked = true
			monsters = append(monsters, m)
		}
		// Pick spell vs the first (representative) target.
		for _, c := range party {
			if c.Class == character.ClassSorcerer {
				equipBestOffensiveSpellVs(c, monsters[0])
				tracef("  spell prep: %s readies %s vs %s", c.Name, c.Equipment[items.SlotSpell].Name, monsterKeys[0])
			}
		}

		if traceThis {
			traceInventory(cs.game.party)
			traceLoadout(party)
		}
		if verbose {
			t.Logf("  trial %2d L3 picks: %s", trial, strings.Join(l3PicksThisTrial, ", "))
		}

		// Bless (Cleric pre-cast) per caller: 10 = +10 all stats, 0 = none.
		cs.game.addStatBuff(TimedStatBuff{SpellID: "bless", Frames: 1 << 30, Bonuses: character.UniformStatBonuses(statBonus)})
		resetPartyForSim(party)

		if traceThis {
			tracef("  [fight begins - %d enemy(s), Bless +%d]", len(monsters), statBonus)
		}
		killed, wiped, rounds := runOneFightWithPotions(cs, party, monsters)
		if traceThis {
			outcome := "WIN"
			if wiped {
				outcome = "WIPE"
			}
			tracef("  [fight ends: %s in %d rounds, party HP %.0f%%]", outcome, rounds, partyHPRatio(party)*100)
			simTrace = nil
		}
		if killed {
			wins++
			sumRoundsWin += rounds
		}
		if wiped {
			wipes++
			// Sum of remaining HP across all enemies, fraction of total.
			remHP := 0
			totalMax := 0
			for _, m := range monsters {
				remHP += m.HitPoints
				totalMax += m.MaxHitPoints
			}
			if totalMax > 0 {
				sumEnemyHPLeftOnWipe += float64(remHP) / float64(totalMax)
			}
		}
		sumHPLeft += partyHPRatio(party)
		levelSum := 0
		for _, c := range party {
			levelSum += c.Level
		}
		sumLevel += float64(levelSum) / float64(len(party))
		if trial == 0 {
			for _, m := range monsters {
				enemyMaxHPSum += m.MaxHitPoints
			}
		}
	}
	cs.game.statBuffs = nil
	cs.game.recomputeStatBonuses()

	avgRoundsWin := 0.0
	if wins > 0 {
		avgRoundsWin = float64(sumRoundsWin) / float64(wins)
	}
	t.Logf("%s - %d trials, fresh loot RNG per trial", label, trials)
	t.Logf("  avg party level after grind: %.1f", sumLevel/float64(trials))
	t.Logf("  wins:  %d/%d  (%.0f%%)", wins, trials, float64(wins)*100/float64(trials))
	t.Logf("  wipes: %d/%d  (%.0f%%)", wipes, trials, float64(wipes)*100/float64(trials))
	if wins > 0 {
		t.Logf("  avg rounds to kill all enemies: %.1f", avgRoundsWin)
	}
	if wipes > 0 {
		t.Logf("  avg enemy HP remaining on wipe: %.0f%% (of %d total)",
			sumEnemyHPLeftOnWipe*100/float64(wipes), enemyMaxHPSum)
	}
	t.Logf("  avg party HP left: %.0f%%", sumHPLeft*100/float64(trials))
}

// TestCombatBalance_ClearedForestPartyVsDragon - cleared forest+church
// party (Spd/End floor + GM caster + full potions + Bless) vs EACH of the
// 4 elemental dragons (HP 1100, 30-50 dmg). The caster auto-prepares the
// best spell per dragon's resist profile (e.g. fireball vs the fire-weak
// green/nature dragon, ice vs the fire-90 base dragon).
func TestCombatBalance_ClearedForestPartyVsDragon(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}
	rand.Seed(time.Now().UnixNano())
	for _, dk := range elderDragonKeys {
		runEndgameSim(t, cs, buildClearedForestParty, []string{dk}, "Cleared-forest party vs "+dk, 10, true)
	}
}

// TestCombatBalance_ClearedForestPartyVsTwoAliens - same cleared-forest
// party against TWO L5 aliens (HP 500 each, 25-35 dmg, alien_dark_bolt
// 4-tile projectile, fire 25%/dark 30%/physical -50% vulnerability).
// Two of them double the burst damage but also double the kill load
// - focus-fire usually finishes one before the second activates.
func TestCombatBalance_ClearedForestPartyVsTwoAliens(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}
	rand.Seed(time.Now().UnixNano())
	runEndgameSim(t, cs, buildClearedForestParty, []string{"alien", "alien"}, "Cleared-forest party vs 2 Aliens", 10, false)
}

// buildFullClearCasterSpeedParty grinds forest + church + water with the
// caster-only-speed stat strategy: only Sorcerer/Archer reach Spd=16
// (and thus 2 action slots under Bless). Knight/Cleric stay at base
// Spd and invest only in End+primary.
func buildFullClearCasterSpeedParty(t *testing.T, cs *CombatSystem) []*character.MMCharacter {
	return buildClearedMapsParty(t, cs, []struct{ path, biome string }{
		{"../../assets/forest.map", "forest"},
		{"../../assets/church.map", "church"},
		{"../../assets/water.map", "water"},
	}, statSpeedOnlyForCasters)
}

// buildFullProgressionParty grinds the realistic pre-dragon path: forest,
// church, water, PLUS the culverts and highlands - the closest fixture to a
// party that has actually earned the dragon statuettes. Same caster-speed strategy.
func buildFullProgressionParty(t *testing.T, cs *CombatSystem) []*character.MMCharacter {
	return buildClearedMapsParty(t, cs, []struct{ path, biome string }{
		{"../../assets/forest.map", "forest"},
		{"../../assets/church.map", "church"},
		{"../../assets/water.map", "water"},
		{"../../assets/culverts.map", "culverts"},
		{"../../assets/highlands.map", "highlands"},
	}, statSpeedOnlyForCasters)
}

// buildHighlandsReadyParty models the party state when first entering the
// highlands: cleared forest, church, the shipwreck encounter and BOTH desert
// oases, then spent its coin on city gear. It does NOT fight the dragons (those
// are the endgame). Grinds forest+church (shipwreck XP baked into grindMaps),
// adds the two oases' 10 guarding bandits (XP + loot), buys the Seabright Arms
// class-upgrade weapons, trains the Sorcerer's water school, then auto-equips.
func buildHighlandsReadyParty(t *testing.T, cs *CombatSystem) []*character.MMCharacter {
	t.Helper()
	cs.game.party = character.NewParty(cs.game.config)
	party := cs.game.party.Members
	cfg := cs.game.config

	for _, d := range grindMaps(t, cs, party, []struct{ path, biome string }{
		{"../../assets/forest.map", "forest"},
		{"../../assets/church.map", "church"},
	}, statSpeedFloorThenPrimary) {
		cs.game.party.AddItem(d)
	}

	// Two desert oases: 10 guarding bandits (XP + loot). Dragons NOT fought.
	if banditDef, err := monsterPkg.MonsterConfig.GetMonsterByKey("bandit"); err == nil && banditDef != nil {
		const oasisBandits = 10
		// Per-kill floor split, matching the live award path (the oases grant no
		// completion XP of their own - only treasure chests).
		xpEach := (banditDef.Experience / len(party)) * oasisBandits
		for _, c := range party {
			applyXPAndLevelUp(c, cfg, xpEach, statSpeedFloorThenPrimary)
		}
		for i := 0; i < oasisBandits; i++ {
			m := monsterPkg.NewMonster3DFromConfig(0, 0, "bandit", cfg)
			for _, d := range cs.checkMonsterLootDrop(m) {
				cs.game.party.AddItem(d)
			}
		}
	}

	// Bought from Seabright Arms (city weapon shop) - one class-fit upgrade each.
	for _, name := range []string{"Silver Sword", "Steel Mace", "Battle Staff", "Elven Bow"} {
		if it, err := items.TryCreateWeaponFromYAML(items.GetWeaponKeyByName(name)); err == nil {
			cs.game.party.AddItem(it)
		}
	}

	// Sorcerer trains water to Grandmaster (matches the other cleared-party builders).
	for _, c := range party {
		if c.Class != character.ClassSorcerer {
			continue
		}
		if c.MagicSchools[character.MagicSchoolWater] == nil {
			c.MagicSchools[character.MagicSchoolWater] = &character.MagicSkill{}
		}
		c.MagicSchools[character.MagicSchoolWater].Mastery = character.MasteryGrandMaster
	}

	autoEquipPartyInventory(cs.game.party)
	return party
}

// TestCombatBalance_HighlandsPartyVsMobs - a party that cleared forest, church,
// the shipwreck and both desert oases and bought city gear, fought 1-on-1
// against every new highlands monster (50 trials each, Bless on).
func TestCombatBalance_HighlandsPartyVsMobs(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}
	rand.Seed(time.Now().UnixNano())
	for _, key := range []string{"puma", "elf_archer", "elf_swordsman", "mountain_troll", "archmage", "lich"} {
		runEndgameSim(t, cs, buildHighlandsReadyParty, []string{key}, "Highlands-ready party vs "+key+" (no Bless)", 0, false)
		runEndgameSim(t, cs, buildHighlandsReadyParty, []string{key}, "Highlands-ready party vs "+key+" (Bless +10)", 10, false)
	}
}

// TestCombatBalance_HighlandsPartyVs2Mobs - same highlands-ready party, but vs
// PAIRS of each mob (zone ambushes stack damage). 50 trials each, no-Bless and
// Bless. This is where the zone's real threat shows vs the trivial 1-on-1.
func TestCombatBalance_HighlandsPartyVs2Mobs(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}
	rand.Seed(time.Now().UnixNano())
	for _, key := range []string{"puma", "elf_archer", "elf_swordsman", "mountain_troll", "archmage", "lich"} {
		pair := []string{key, key}
		runEndgameSim(t, cs, buildHighlandsReadyParty, pair, "Highlands party vs 2x "+key+" (no Bless)", 0, false)
		runEndgameSim(t, cs, buildHighlandsReadyParty, pair, "Highlands party vs 2x "+key+" (Bless +10)", 10, false)
	}
}

// TestCombatBalance_CastleEntryL10PartyVsCastleMobs uses a deterministic level
// 10 party with late-midgame gear (no castle loot) to benchmark the Japanese
// castle's normal mobs, elite pairs, and Samurai Warlord with real summon/enrage
// fields represented by the simulator.
func TestCombatBalance_CastleEntryL10PartyVsCastleMobs(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}
	rand.Seed(42)

	for _, sc := range castleMobScenarios() {
		runFixedPartySim(t, cs, buildCastleEntryL10Party, sc.monsters, "Castle-entry L10 party vs "+sc.label+" (no Bless)", 0, 50)
		runFixedPartySim(t, cs, buildCastleEntryL10Party, sc.monsters, "Castle-entry L10 party vs "+sc.label+" (Bless +10)", 10, 50)
	}
}

func castleMobScenarios() []fightScenario {
	return []fightScenario{
		{label: "ashigaru", monsters: []string{"ashigaru_firelock"}},
		{label: "ningyo", monsters: []string{"ningyo"}},
		{label: "ronin", monsters: []string{"ronin_marksman"}},
		{label: "vengeful", monsters: []string{"vengeful_ningyo"}},
		{label: "2x ashigaru", monsters: []string{"ashigaru_firelock", "ashigaru_firelock"}},
		{label: "2x ningyo", monsters: []string{"ningyo", "ningyo"}},
		{label: "ronin+vengeful", monsters: []string{"ronin_marksman", "vengeful_ningyo"}},
		{label: "samurai", monsters: []string{"old_samurai"}},
	}
}

// TestCombatBalance_CastleVeteranL17PartyVsCastleMobs benchmarks the same
// castle scenarios against a fixed level-17 party with common castle/late-zone
// gear, without Samurai boss uniques.
func TestCombatBalance_CastleVeteranL17PartyVsCastleMobs(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}
	rand.Seed(43)

	for _, sc := range castleMobScenarios() {
		runFixedPartySim(t, cs, buildCastleVeteranL17Party, sc.monsters, "Castle-veteran L17 party vs "+sc.label+" (no Bless)", 0, 50)
		runFixedPartySim(t, cs, buildCastleVeteranL17Party, sc.monsters, "Castle-veteran L17 party vs "+sc.label+" (Bless +10)", 10, 50)
	}
}

// TestCombatBalance_CastleRealPlayPartyVsCastleMobs runs the actual save1 party
// (the author's real progression: L13-14, mixed real gear/spells) against the
// Japanese castle's mobs, elite pairs, and the Samurai Warlord.
func TestCombatBalance_CastleRealPlayPartyVsCastleMobs(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}
	rand.Seed(45)
	for _, sc := range castleMobScenarios() {
		runFixedPartySim(t, cs, buildRealPlayParty, sc.monsters, "Castle Real play party vs "+sc.label, 0, 50)
	}
}

func jungleMobScenarios() []fightScenario {
	return []fightScenario{
		{label: "ocelot", monsters: []string{"ocelot"}},
		{label: "jungle_goblin", monsters: []string{"jungle_goblin"}},
		{label: "masked_huntress", monsters: []string{"masked_huntress"}},
		{label: "serpent_dancer", monsters: []string{"masked_serpent_dancer"}},
		{label: "masked_hexer", monsters: []string{"masked_hexer_girl"}},
		{label: "gorilla_titan", monsters: []string{"gorilla_titan"}},
		{label: "2x ocelot", monsters: []string{"ocelot", "ocelot"}},
		{label: "2x goblin", monsters: []string{"jungle_goblin", "jungle_goblin"}},
		{label: "huntress+dancer", monsters: []string{"masked_huntress", "masked_serpent_dancer"}},
		{label: "warlord+4 idols", monsters: []string{"jungle_idol", "jungle_idol", "jungle_idol", "jungle_idol", "orc_hero_boss"}},
	}
}

// TestCombatBalance_JungleL8PartyVsMobs - a LOW (L8) starter party wandering into
// the L18-24 jungle. It SHOULD struggle/wipe on the zone's mobs; if it facerolls
// ocelots/goblins while they pay big XP, the trash is undertuned (farmable). Note
// the sim doesn't model the warlord's idol-ward (BossWarded is set only by the
// live per-frame pre-pass), so the boss row reflects raw HP/damage, not the gate.
func TestCombatBalance_JungleL8PartyVsMobs(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}
	rand.Seed(42)
	buildL8 := func(t *testing.T, cs *CombatSystem) []*character.MMCharacter {
		cs.game.party = character.NewParty(cs.game.config)
		party := buildDefaultParty(t, cs, 8)
		cs.game.party.Members = party
		return party
	}
	for _, sc := range jungleMobScenarios() {
		runFixedPartySim(t, cs, buildL8, sc.monsters, "Jungle L8 party vs "+sc.label, 0, 50)
	}
}

// TestCombatBalance_JungleVeteranPartyVsMobs - a well-geared L17 veteran (the
// castle-veteran loadout, the kind of party that would actually attempt the
// jungle) vs the same scenarios, as the at-level reference.
func TestCombatBalance_JungleVeteranPartyVsMobs(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}
	rand.Seed(43)
	for _, sc := range jungleMobScenarios() {
		runFixedPartySim(t, cs, buildCastleVeteranL17Party, sc.monsters, "Jungle L17-veteran party vs "+sc.label, 0, 50)
	}
}

// TestCombatBalance_JungleRealPlayPartyVsMobs runs the actual save1 party (the
// author's real progression: L13-14, mixed real gear/spells) against the jungle.
func TestCombatBalance_JungleRealPlayPartyVsMobs(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}
	rand.Seed(44)
	for _, sc := range jungleMobScenarios() {
		runFixedPartySim(t, cs, buildRealPlayParty, sc.monsters, "Jungle Real play party vs "+sc.label, 0, 50)
	}
}

// TestCombatBalance_FullClearCasterSpeedPartyVsDragon - party clears
// forest + church + water (everything except dragon), then fights the
// dragon. Stat strategy: only Sorcerer/Archer invest Spd->16; Knight
// and Cleric pour everything into End->15 then primary stat. Tests
// whether the extra water-map XP/loot offsets the lost action slots
// on the two melee chars.
func TestCombatBalance_FullClearCasterSpeedPartyVsDragon(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}
	rand.Seed(time.Now().UnixNano())
	for _, dk := range elderDragonKeys {
		runEndgameSim(t, cs, buildFullClearCasterSpeedParty, []string{dk},
			"Full-clear party (all 3 maps) vs "+dk+" (no Bless)", 0, false)
		runEndgameSim(t, cs, buildFullClearCasterSpeedParty, []string{dk},
			"Full-clear party (all 3 maps) vs "+dk+" (Bless +10)", 10, true)
	}
}

// TestCombatBalance_FullProgressionPartyVsDragon - the strongest fixture: a party
// that also cleared the culverts and highlands (the realistic pre-dragon path)
// vs each Elder Dragon, with and without Bless.
func TestCombatBalance_FullProgressionPartyVsDragon(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if _, err := config.LoadLootTables("../../assets/loots.yaml"); err != nil {
		t.Fatalf("load loots: %v", err)
	}
	rand.Seed(time.Now().UnixNano())
	for _, dk := range elderDragonKeys {
		runEndgameSim(t, cs, buildFullProgressionParty, []string{dk},
			"Full-progression party (+culverts +highlands) vs "+dk+" (no Bless)", 0, false)
		runEndgameSim(t, cs, buildFullProgressionParty, []string{dk},
			"Full-progression party (+culverts +highlands) vs "+dk+" (Bless +10)", 10, true)
	}
}

// TestCombatBalance_LowLevelPartyVsForest runs the default starter party at
// L1/L2/L3 (with class-appropriate stat investment) against every monster
// that can spawn in the forest biome - both 1v1 and 1v2 of the trainer
// mobs (goblin, spider, wolf, forest_spider, pixie) so we can see how
// pack-spawns scale difficulty. Bless is off - fresh parties typically
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

// TestMonsterDamageVsArmorTiers reports, for EVERY current monster, the physical
// melee damage it deals to a level-6 / 20-Endurance character wearing a full set
// of each armor tier - leather, chain, plate. Each monster's min..max is run
// through the real percentage armor path (armorMitigationPctFromAC: physical%
// = min(75, 100*AC/(AC+K)), floored at 1). Output is a table (run with
// -run MonsterDamageVsArmorTiers -v); it also asserts AC rises leather<chain<plate,
// heavier armor never takes MORE, and the 1-damage floor holds.
func TestMonsterDamageVsArmorTiers(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")

	armorSlots := []items.EquipSlot{
		items.SlotArmor, items.SlotHelmet, items.SlotBoots,
		items.SlotCloak, items.SlotGauntlets, items.SlotBelt,
	}
	sets := []struct {
		name   string
		pieces map[items.EquipSlot]string
	}{
		{"leather", map[items.EquipSlot]string{items.SlotArmor: "leather_armor", items.SlotHelmet: "leather_helmet", items.SlotBoots: "leather_pants"}},
		{"chain", map[items.EquipSlot]string{items.SlotArmor: "chain_armor", items.SlotHelmet: "chain_helmet", items.SlotBoots: "chain_pants"}},
		{"plate", map[items.EquipSlot]string{items.SlotArmor: "iron_armor", items.SlotHelmet: "iron_helmet", items.SlotBoots: "iron_pants"}},
	}

	// A L6/20-END tank wearing ONLY the given set (other armor slots cleared so AC
	// is purely that set).
	newTank := func(pieces map[items.EquipSlot]string) *character.MMCharacter {
		c := character.CreateCharacter("Tank", character.ClassKnight, cs.game.config)
		c.Level = 6
		c.Endurance = 20
		for _, s := range armorSlots {
			delete(c.Equipment, s)
		}
		for slot, key := range pieces {
			c.Equipment[slot] = items.CreateItemFromYAML(key)
		}
		return c
	}

	tanks := map[string]*character.MMCharacter{}
	ac := map[string]int{}
	for _, s := range sets {
		tanks[s.name] = newTank(s.pieces)
		ac[s.name] = cs.CalculateTotalArmorClass(tanks[s.name])
	}
	t.Logf("L6 Knight, 20-END (incl. class armor mastery) total AC - leather:%d  chain:%d  plate:%d (phys mitigation leather:%d%% chain:%d%% plate:%d%%)",
		ac["leather"], ac["chain"], ac["plate"],
		armorMitigationPctFromAC(ac["leather"], true), armorMitigationPctFromAC(ac["chain"], true), armorMitigationPctFromAC(ac["plate"], true))
	if !(ac["leather"] < ac["chain"] && ac["chain"] < ac["plate"]) {
		t.Errorf("AC should rise leather<chain<plate, got %d/%d/%d", ac["leather"], ac["chain"], ac["plate"])
	}

	// Build every monster, then sort by level (then name) so the table reads from
	// weakest to toughest.
	mobs := make([]*monsterPkg.Monster3D, 0, len(monsterPkg.MonsterConfig.Monsters))
	for k := range monsterPkg.MonsterConfig.Monsters {
		mobs = append(mobs, monsterPkg.NewMonster3DFromConfig(0, 0, k, cs.game.config))
	}
	sort.Slice(mobs, func(i, j int) bool {
		if mobs[i].Level != mobs[j].Level {
			return mobs[i].Level < mobs[j].Level
		}
		return mobs[i].Name < mobs[j].Name
	})

	t.Logf("%-3s %-22s %-9s %-11s %-11s %-11s", "lvl", "monster", "raw", "leather", "chain", "plate")
	rng := func(set string, m *monsterPkg.Monster3D) (int, int) {
		return cs.mitigateCharacterDamage(m.DamageMin, "physical", tanks[set], false), cs.mitigateCharacterDamage(m.DamageMax, "physical", tanks[set], false)
	}
	for _, m := range mobs {
		lMin, lMax := rng("leather", m)
		cMin, cMax := rng("chain", m)
		pMin, pMax := rng("plate", m)
		t.Logf("%-3d %-22s %-9s %-11s %-11s %-11s", m.Level, m.Name,
			fmt.Sprintf("%d-%d", m.DamageMin, m.DamageMax),
			fmt.Sprintf("%d-%d", lMin, lMax),
			fmt.Sprintf("%d-%d", cMin, cMax),
			fmt.Sprintf("%d-%d", pMin, pMax))
		// Heavier armor must never take MORE damage; floor never below 1.
		if lMax < cMax || cMax < pMax {
			t.Errorf("%s: heavier armor took more (leather %d, chain %d, plate %d)", m.Name, lMax, cMax, pMax)
		}
		// Anything that actually deals damage must chip at least 1 through armor;
		// passive 0-damage entries (e.g. the Warlord Idol) legitimately stay 0.
		if m.DamageMin >= 1 && pMin < 1 {
			t.Errorf("%s: damage floor dropped below 1 (got %d)", m.Name, pMin)
		}
	}
}

// The Golden Thief Bug Carapace grants +100% resist to all non-physical schools
// (full immunity) to its wearer, while physical damage still goes through armor.
// Exercises the resist_nonphysical attribute end-to-end (config->bridge->item->
// character) and the mitigateCharacterDamage chokepoint.
func TestGoldenCarapace_NonPhysicalImmunity(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	member := cs.game.party.Members[0]

	// Baseline: no resist gear -> non-physical passes through untouched.
	if got := cs.mitigateCharacterDamage(40, "fire", member, false); got != 40 {
		t.Errorf("no resist gear: fire 40 should pass, got %d", got)
	}

	member.Equipment[items.SlotArmor] = items.CreateItemFromYAML("golden_thiefbug_carapace")
	if got := member.GearResistPct("fire"); got != 100 {
		t.Fatalf("carapace must grant 100%% fire (per-element) resist, got %d", got)
	}
	for _, school := range []string{"fire", "water", "air", "earth", "body", "mind", "spirit", "dark", "light"} {
		if got := cs.mitigateCharacterDamage(40, school, member, false); got != 0 {
			t.Errorf("%s 40 should be fully resisted by the carapace, got %d", school, got)
		}
	}
	// Physical is NOT in the carapace's resistances -> still reduced by armor only (>=1).
	if got := cs.mitigateCharacterDamage(40, "physical", member, false); got < 1 {
		t.Errorf("physical must still apply via armor, got %d", got)
	}
}
