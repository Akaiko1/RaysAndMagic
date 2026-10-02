package game

import (
	"fmt"
	"sort"
	"testing"

	"ugataima/internal/arena"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/storage"
)

// TestChampionBuilds verifies champions build as auto-leveled fighters at the
// tier's level: every level-up stat point spent by the party's own
// auto-distribution, authored gear equipped, the tier's mastery applied.
func TestChampionBuilds(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	tier := config.GetChampionTier("impossible")
	if tier == nil {
		t.Fatal("impossible champion tier missing")
	}
	wantMastery, ok := character.MasteryFromKey(tier.Mastery)
	if !ok {
		t.Fatalf("impossible tier: bad mastery %q", tier.Mastery)
	}

	arch := cs.game.championTemplate("hobbit_archer", "impossible")
	if arch == nil {
		t.Fatal("hobbit_archer template nil")
	}
	if arch.Level != tier.Level {
		t.Errorf("archer level = %d, want tier level %d", arch.Level, tier.Level)
	}
	if arch.FreeStatPoints != 0 {
		t.Errorf("archer has %d unspent stat points, auto-distribution must spend all", arch.FreeStatPoints)
	}
	base := cs.game.config.Characters.Classes["archer"]
	baseSum := base.Might + base.Intellect + base.Personality + base.Endurance + base.Accuracy + base.Speed + base.Luck
	sum := arch.Might + arch.Intellect + arch.Personality + arch.Endurance + arch.Accuracy + arch.Speed + arch.Luck
	leveled := (tier.Level - 1) * StatPointsPerLevel
	if want := baseSum + leveled; sum != want {
		t.Errorf("archer stat sum = %d, want class base %d + %d leveled points", sum, baseSum, leveled)
	}
	if got := arch.SkillTier(character.SkillBow); got != int(wantMastery) {
		t.Errorf("archer bow tier = %d, want tier mastery %d", got, int(wantMastery))
	}
	if _, ok := arch.Equipment[items.SlotMainHand]; !ok {
		t.Error("archer has no main-hand weapon")
	}

	wm := cs.game.championTemplate("weapon_master", "impossible")
	if wm == nil {
		t.Fatal("weapon_master template nil")
	}
	if _, ok := wm.Equipment[items.SlotOffHand]; !ok {
		t.Error("weapon_master dual-wield needs an off-hand weapon")
	}
}

// TestApplyChampionStatsMirror verifies the static mirror: boss HP pool stays
// authored (not the character's), two TB swings, ranged weapon with weapon-
// physics range, GM dodge-pierce, character dodge chance, weapon reach.
func TestApplyChampionStatsMirror(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)

	m := monsterPkg.NewMonster3DFromConfig(0, 0, "hobbit_archer", cs.game.config)
	if !m.IsChampion() {
		t.Fatal("hobbit_archer monster not flagged IsChampion")
	}
	cs.game.mirrorChampionStats(m)

	tier := config.GetChampionTier(championTierOf(m))
	if tier == nil {
		t.Fatalf("champion tier %q missing", championTierOf(m))
	}
	if m.MaxHitPoints != tier.HP || m.HitPoints != tier.HP {
		t.Errorf("champion HP = %d/%d, want the tier's pool %d", m.HitPoints, m.MaxHitPoints, tier.HP)
	}
	if m.DamageMin <= 0 || m.DamageMin != m.DamageMax {
		t.Errorf("mirror damage band = [%d,%d], want equal and positive", m.DamageMin, m.DamageMax)
	}
	if m.AttacksPerRound != 2 {
		t.Errorf("champion TB swings = %d, want 2", m.AttacksPerRound)
	}
	if !m.HasRangedAttack() {
		t.Error("archer champion should be ranged (main-hand bow as projectile)")
	}
	// Range derives from the equipped weapon's own physics - re-gear the
	// champion and this expectation follows automatically.
	ts := float64(cs.game.config.GetTileSize())
	bow, _, found := config.GetWeaponDefinitionByName(cs.game.championTemplate("hobbit_archer", "impossible").Equipment[items.SlotMainHand].Name)
	if !found || bow.Physics == nil {
		t.Fatal("archer main hand has no projectile physics def")
	}
	if got, want := m.GetAttackRangePixels(), bow.Physics.RangeTiles*ts; got != want {
		t.Errorf("archer range = %.0fpx, want weapon physics %.0fpx", got, want)
	}
	if !m.IgnoresDodge {
		t.Error("GM bow mastery must pierce Perfect Dodge")
	}
	if m.PerfectDodge <= 0 {
		t.Error("champion PerfectDodge must derive from character luck")
	}
	if m.AttackCooldownMultiplier <= 0 {
		t.Error("champion RT cadence not mirrored")
	}

	// The mirror never touches current HP - a loaded champion keeps its wounds.
	m.HitPoints = 7
	cs.game.mirrorChampionStats(m)
	if m.HitPoints != 7 {
		t.Errorf("loaded champion HP = %d, want preserved 7", m.HitPoints)
	}

	// Melee champion mirrors its main-hand weapon's authored reach.
	wm := monsterPkg.NewMonster3DFromConfig(0, 0, "weapon_master", cs.game.config)
	cs.game.mirrorChampionStats(wm)
	mh, _, found := config.GetWeaponDefinitionByName(cs.game.championTemplate("weapon_master", "impossible").Equipment[items.SlotMainHand].Name)
	if !found {
		t.Fatal("weapon_master main hand missing from weapons.yaml")
	}
	if got, want := wm.AttackRadius, float64(mh.Range)*ts; got != want {
		t.Errorf("weapon_master reach = %.0fpx, want weapon range %.0fpx", got, want)
	}
	if wm.HasRangedAttack() {
		t.Error("weapon_master must stay melee")
	}
}

// overrideChampionMainHand makes a single champion attack test data-driven by
// swapping only its weapon. The cached production template is restored when
// the test completes.
func overrideChampionMainHand(t *testing.T, g *MMGame, championKey, tierName, weaponKey string) {
	t.Helper()
	key := championTemplateKey(championKey, tierName)
	original := championTemplates[key]
	if original == nil {
		t.Fatalf("missing champion template %q", key)
	}
	replacement := character.CreateCharacter("Test Champion", original.Class, g.config)
	replacement.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(weaponKey)
	championTemplates[key] = replacement
	t.Cleanup(func() { championTemplates[key] = original })
}

// meleeWeaponWithArc picks an authored single-target melee weapon (reach 1, no
// splash) of the given arc type, so arc geometry tests follow content.
func meleeWeaponWithArc(t *testing.T, arc int) string {
	t.Helper()
	keys := make([]string, 0, len(config.GlobalWeapons.Weapons))
	for key := range config.GlobalWeapons.Weapons {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		def := config.GlobalWeapons.Weapons[key]
		if def.Melee != nil && def.Melee.ArcType == arc && def.Range <= 1 && def.AoeRadiusTiles == 0 && !def.IsRanged() {
			return key
		}
	}
	t.Fatalf("no authored reach-1 melee weapon with arc %d", arc)
	return ""
}

// championHandArc is the authored arc of a champion template's striking hand.
func championHandArc(t *testing.T, ch *character.MMCharacter, offHand bool) int {
	t.Helper()
	weapon := championHandWeapon(ch, offHand)
	def, _, ok := config.GetWeaponDefinitionByName(weapon.Name)
	if !ok || def == nil || def.Melee == nil || def.AoeRadiusTiles > 0 {
		t.Fatalf("champion hand %q is not an arc melee weapon", weapon.Name)
	}
	return def.Melee.ArcType
}

// TestChampionMeleeArc checks every authored arc width against a clean party.
// The champion's formation targeting must remain independent from crossfire
// targets: arc 1/2/3/4 catch 1/2/3/all living party members respectively.
func TestChampionMeleeArc(t *testing.T) {
	for _, tc := range []struct {
		arc, want int
	}{
		{arc: 1, want: 1},
		{arc: 2, want: 2},
		{arc: 3, want: 3},
		{arc: 4, want: 4},
	} {
		t.Run(fmt.Sprintf("arc_%d", tc.arc), func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			primeTestChampions(t, cs.game)
			fillTestParty(t, cs.game)
			weapon := meleeWeaponWithArc(t, tc.arc)
			overrideChampionMainHand(t, cs.game, "weapon_master", "impossible", weapon)

			// Eliminate perfect-dodge variance so the struck formation is observable.
			for _, mem := range cs.game.party.Members {
				mem.Luck = 0
			}
			m := monsterPkg.NewMonster3DFromConfig(0, 0, "weapon_master", cs.game.config)
			if !cs.championMeleeStrike(m, false) {
				t.Fatal("champion melee strike failed")
			}

			hit := 0
			for _, mem := range cs.game.party.Members {
				if mem.HitPoints < mem.MaxHitPoints {
					hit++
				}
			}
			if hit != tc.want {
				t.Fatalf("%s hit %d party members, want %d", weapon, hit, tc.want)
			}
		})
	}
}

// TestChampionRTDualStreams: real-time dual wield runs two INDEPENDENT hand
// cooldowns (party parity) - the off hand fires on its own weapon's cadence
// even when the main hand's attack tick is absent or its cooldown is running.
func TestChampionRTDualStreams(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	fillTestParty(t, cs.game)

	m := monsterPkg.NewMonster3DFromConfig(float64(cs.game.config.GetTileSize()), 0, "weapon_master", cs.game.config)
	m.BeginPlayerEngagement()
	cs.game.mirrorChampionStats(m)
	ch := cs.game.championTemplate("weapon_master", "impossible")

	// Attack tick with both hands ready: both streams strike, both CDs arm to
	// their own weapon's formula values.
	if !cs.championRTDualStrike(m, true) {
		t.Fatal("dual strike not handled")
	}
	if want := m.AttackCooldownFrames(); want <= 0 || m.AttackCDFrames != want {
		t.Fatalf("main CD = %d, want the main weapon's own %d", m.AttackCDFrames, want)
	}
	if want := cs.OffHandWeaponCooldownFrames(ch); m.OffHandCDFrames != want {
		t.Fatalf("off CD = %d, want the off weapon's own %d", m.OffHandCDFrames, want)
	}
	hit := 0
	for _, mem := range cs.game.party.Members {
		if mem.HitPoints < mem.MaxHitPoints {
			hit++
		}
	}
	if hit == 0 {
		t.Fatal("no party member struck on the double swing")
	}

	// No attack tick, main CD still running, off CD elapsed: ONLY the off hand
	// swings - the streams are independent.
	for _, mem := range cs.game.party.Members {
		mem.HitPoints = mem.MaxHitPoints
		mem.Conditions = mem.Conditions[:0]
		mem.StunFramesRemaining, mem.StunTurnsRemaining = 0, 0
	}
	mainBefore := m.AttackCDFrames
	m.OffHandCDFrames = 0
	if !cs.championRTDualStrike(m, false) {
		t.Fatal("dual strike not handled")
	}
	if m.AttackCDFrames != mainBefore {
		t.Fatal("main CD changed without an attack tick")
	}
	if m.OffHandCDFrames <= 0 {
		t.Fatal("off CD not re-armed after its solo swing")
	}
	hit = 0
	for _, mem := range cs.game.party.Members {
		if mem.HitPoints < mem.MaxHitPoints {
			hit++
		}
	}
	if hit == 0 {
		t.Fatal("off-hand solo swing struck nobody")
	}
}

// TestChampionMeleeAoEOnce: an AoE-rider melee weapon (tonbogiri) sweeps the
// WHOLE party exactly once per swing - the champion arc+AoE rule.
func TestChampionMeleeAoEOnce(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	fillTestParty(t, cs.game)
	// Perfect Dodge is luck/5% - a lucky member would evade the sweep and
	// read as "untouched". Zero it for determinism.
	for _, mem := range cs.game.party.Members {
		mem.Luck = 0
	}

	overrideChampionMainHand(t, cs.game, "weapon_master", "impossible", "tonbogiri")

	m := monsterPkg.NewMonster3DFromConfig(0, 0, "weapon_master", cs.game.config)
	for _, mem := range cs.game.party.Members {
		mem.HitPoints = mem.MaxHitPoints
	}
	if !cs.championMeleeStrike(m, false) {
		t.Fatal("champion AoE strike failed")
	}
	for i, mem := range cs.game.party.Members {
		if mem.HitPoints >= mem.MaxHitPoints {
			t.Fatalf("member %d untouched - AoE weapon must sweep the whole party", i)
		}
	}
}

// TestChampionVolley: the archer's weapon looses its authored volley of darts
// per attack, each its own projectile (each rolls its own crit inside
// monsterAttackDamage). The expected count derives from the weapon def.
func TestChampionVolley(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)

	m := monsterPkg.NewMonster3DFromConfig(0, 0, "hobbit_archer", cs.game.config)
	cs.game.mirrorChampionStats(m)
	if !m.HasRangedAttack() {
		t.Fatal("archer champion should be ranged")
	}
	def, ok := config.GetWeaponDefinition(m.ProjectileWeapon)
	if !ok || def == nil {
		t.Fatalf("projectile weapon %q missing from weapons.yaml", m.ProjectileWeapon)
	}
	want := def.Volley
	if want < 1 {
		want = 1
	}
	before := len(cs.game.arrows)
	cs.spawnMonsterWeaponProjectile(m, m.ProjectileWeapon, 320, 0, ProjectileOwnerMonster)
	if got := len(cs.game.arrows) - before; got != want {
		t.Fatalf("volley spawned %d darts, want weapon def's %d", got, want)
	}
}

// TestChampionAttackDamage: champion hits resolve through the live character
// pipeline - always at least the weapon+stats total, at most a crit multiple.
func TestChampionAttackDamage(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)

	m := monsterPkg.NewMonster3DFromConfig(0, 0, "weapon_master", cs.game.config)
	cs.game.mirrorChampionStats(m)
	ch := cs.game.championTemplate("weapon_master", "impossible")
	_, _, base := cs.CalculateWeaponDamage(ch.Equipment[items.SlotMainHand], ch)

	crits := 0
	for i := 0; i < 200; i++ {
		dmg := cs.monsterAttackDamage(m)
		if dmg < base || dmg > base*CritDamageMultiplier {
			t.Fatalf("champion damage %d outside [%d, %d]", dmg, base, base*CritDamageMultiplier)
		}
		if dmg > base {
			crits++
		}
	}
	// Weapon crit + luck bonus: 200 swings statistically must crit at least once.
	if crits == 0 {
		t.Error("no crits in 200 swings - crit roll not wired")
	}
}

// TestChampionTiers: each difficulty builds at its own level/mastery and the
// mirror stamps the tier's HP pool and victory experience - all derived from
// champions.yaml tier config, never re-stated here.
func TestChampionTiers(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)

	for tierName, tier := range config.GlobalChampionConfig.Tiers {
		ch := cs.game.championTemplate("weapon_master", tierName)
		if ch == nil {
			t.Fatalf("no template for tier %q", tierName)
		}
		if ch.Level != tier.Level {
			t.Errorf("tier %s level = %d, want %d", tierName, ch.Level, tier.Level)
		}
		wantMastery, ok := character.MasteryFromKey(tier.Mastery)
		if !ok {
			t.Fatalf("tier %s: bad mastery %q", tierName, tier.Mastery)
		}
		if got := ch.Skills[character.SkillSword].Mastery; got != wantMastery {
			t.Errorf("tier %s sword mastery = %v, want %v", tierName, got, wantMastery)
		}

		m := monsterPkg.NewMonster3DFromConfig(0, 0, "weapon_master", cs.game.config)
		m.ChampionTier = tierName
		cs.game.mirrorChampionStats(m)
		if m.MaxHitPoints != tier.HP {
			t.Errorf("tier %s HP pool = %d, want %d", tierName, m.MaxHitPoints, tier.HP)
		}
		if m.HitPoints != tier.HP {
			t.Errorf("tier %s spawn HP = %d, want full %d", tierName, m.HitPoints, tier.HP)
		}
		if m.Experience != tier.Experience {
			t.Errorf("tier %s XP = %d, want %d", tierName, m.Experience, tier.Experience)
		}
	}
}

// killChampion is the one victory ritual the reward/board tests share: spawn a
// champion at the tier, mirror, slay through the kill choke point.
func killChampion(cs *CombatSystem, key, tier string) *monsterPkg.Monster3D {
	m := monsterPkg.NewMonster3DFromConfig(0, 0, key, cs.game.config)
	m.ChampionTier = tier
	cs.game.mirrorChampionStats(m)
	m.HitPoints = 0
	cs.finishMonsterKill(m)
	return m
}

// TestChampionVictoryRewards: a champion kill pays the tier's arena points and
// records the party on the global leaderboard (isolated to a temp dir).
func TestChampionVictoryRewards(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	fillTestParty(t, cs.game)
	storage.SetDataRootForTesting(t.TempDir())
	defer storage.SetDataRootForTesting("")

	tier := config.GetChampionTier("easy")
	before := cs.game.party.ArenaPoints
	m := killChampion(cs, "hobbit_archer", "easy")
	if got := cs.game.party.ArenaPoints - before; got != tier.ArenaPoints {
		t.Fatalf("arena points awarded = %d, want tier's %d", got, tier.ArenaPoints)
	}

	board := arena.Load()
	if len(board.Entries) != 1 {
		t.Fatalf("leaderboard entries = %d, want 1", len(board.Entries))
	}
	e := board.Entries[0]
	if e.TotalKills() != 1 || e.Kills[m.Name]["easy"] != 1 {
		t.Fatalf("leaderboard kills = %+v, want one easy %s", e.Kills, m.Name)
	}
	if len(e.Members) != 4 || e.Members[0].Class == "" || e.Members[0].Level <= 0 {
		t.Fatalf("leaderboard members malformed: %+v", e.Members)
	}
	if e.TotalPoints != tier.ArenaPoints {
		t.Fatalf("leaderboard points = %d, want %d", e.TotalPoints, tier.ArenaPoints)
	}
}

// A card summon killing the champion is still the party's arena victory. The
// crossfire death path must reach the same champion-reward choke point as a
// direct party strike.
func TestChampionVictoryRewardsFromCardSummonKill(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	fillTestParty(t, cs.game)

	champion := monsterPkg.NewMonster3DFromConfig(128, 128, "weapon_master", cs.game.config)
	champion.ChampionTier = "easy"
	champion.MaxHitPoints, champion.HitPoints = 1, 1
	summon := monsterPkg.NewMonster3DFromConfig(192, 128, "masked_huntress", cs.game.config)
	markCardAlly(summon)
	cs.game.world.Monsters = []*monsterPkg.Monster3D{champion, summon}
	cs.game.world.RegisterMonstersWithCollisionSystem(cs.game.collisionSystem)

	tier := config.GetChampionTier("easy")
	if tier == nil {
		t.Fatal("easy champion tier missing")
	}
	before := cs.game.party.ArenaPoints
	hit := hitFromMonster(summon, 9999, monsterPkg.DamagePhysical.String(), false, 0, true, false)
	cs.strikeMonsterFor(summon, champion, hit, nil, false)

	if got, want := cs.game.party.ArenaPoints-before, tier.ArenaPoints; got != want {
		t.Fatalf("arena points from card-summon champion kill = %d, want %d", got, want)
	}
}

// TestArenaGladiatorShop: BOTH gladiators (gatekeeper outside, duel master
// inside) carry the points shop as a dialog tab: every authored entry at its
// authored price and stock, plus EVERY weapon of the authored rack rarity at
// the rack price - all derived from content, not restated.
func TestArenaGladiatorShop(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load npcs: %v", err)
	}
	for _, key := range []string{"gladiator_gatekeeper", "arena_duel_master"} {
		npc, err := character.CreateNPCFromConfig(key, 0, 0)
		if err != nil {
			t.Fatalf("create %s: %v", key, err)
		}
		data, ok := character.NPCConfigInstance.GetNPCData(key)
		if !ok {
			t.Fatalf("%s: npc data missing", key)
		}
		checkArenaShop(t, npc, data)
	}
}

func checkArenaShop(t *testing.T, npc *character.NPC, data *character.NPCData) {
	t.Helper()
	if npc.Currency != character.CurrencyArenaPoints {
		t.Fatalf("%s currency = %q, want arena_points", npc.Name, npc.Currency)
	}
	if data.StockWeaponsRarity == "" || data.StockWeaponsCost <= 0 {
		t.Fatalf("%s authors no weapon rack", npc.Name)
	}
	rack := map[string]bool{}
	for _, key := range config.WeaponKeysByRarity(data.StockWeaponsRarity) {
		rack[config.GlobalWeapons.Weapons[key].Name] = true
	}
	authored := map[string]*character.NPCItem{}
	for _, entry := range data.Inventory {
		authored[entry.Name] = entry
	}
	if want := len(data.Inventory) + len(rack); len(npc.MerchantStock) != want {
		t.Fatalf("stock size = %d, want %d authored entries + %d %s weapons",
			len(npc.MerchantStock), len(data.Inventory), len(rack), data.StockWeaponsRarity)
	}
	soldUniques := map[string]bool{}
	rackSeen := 0
	for _, entry := range npc.MerchantStock {
		if !entry.InStock() {
			t.Fatalf("%s not in stock", entry.Item.Name)
		}
		if def, _, ok := config.GetWeaponDefinitionByName(entry.Item.Name); ok && def != nil && def.Rarity == "unique" {
			soldUniques[entry.Item.Name] = true
		}
		if src, ok := authored[entry.Item.Name]; ok {
			if entry.Cost != src.Cost {
				t.Fatalf("%s costs %d, want authored %d", entry.Item.Name, entry.Cost, src.Cost)
			}
			if copies := src.Quantity; copies >= 0 { // negative = unlimited, 0 = one copy
				copies = max(copies, 1)
				for range copies {
					entry.Take()
				}
				if entry.InStock() {
					t.Fatalf("%s still in stock after its %d authored copies", entry.Item.Name, copies)
				}
				continue
			}
		} else {
			if !rack[entry.Item.Name] {
				t.Fatalf("%s is neither authored nor a %s rack weapon", entry.Item.Name, data.StockWeaponsRarity)
			}
			rackSeen++
			if entry.Cost != data.StockWeaponsCost {
				t.Fatalf("weapon %s costs %d, want the rack price %d", entry.Item.Name, entry.Cost, data.StockWeaponsCost)
			}
		}
		entry.Take()
		if !entry.InStock() {
			t.Fatalf("%s sold out after one purchase - stock must be unlimited", entry.Item.Name)
		}
	}
	if rackSeen != len(rack) {
		t.Fatalf("weapon rack = %d, want every %s weapon (%d)", rackSeen, data.StockWeaponsRarity, len(rack))
	}
	// The arena is the unique tier's shop: every unique weapon is sold here.
	for _, key := range config.WeaponKeysByRarity("unique") {
		if name := config.GlobalWeapons.Weapons[key].Name; !soldUniques[name] {
			t.Fatalf("unique weapon %s is not sold by %s", name, npc.Name)
		}
	}

	// Weapons are GROUPED by category: same-category entries must be adjacent
	// (category never repeats after a different one intervenes).
	seen := map[string]bool{}
	last := ""
	for _, entry := range npc.MerchantStock {
		if entry.Item.Type != items.ItemWeapon {
			continue
		}
		def, _, ok := config.GetWeaponDefinitionByName(entry.Item.Name)
		if !ok {
			t.Fatalf("weapon %s missing def", entry.Item.Name)
		}
		if def.Category != last {
			if seen[def.Category] {
				t.Fatalf("category %q split across the rack - weapons must be grouped by type", def.Category)
			}
			seen[def.Category] = true
			last = def.Category
		}
	}
}

// TestArenaBoardFarmGuard: replaying the same in-game day (save-scumming the
// duel) must not inflate the global board - the runID+day credit token blocks
// the duplicate; a new day or a new run records again.
func TestArenaBoardFarmGuard(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	fillTestParty(t, cs.game)
	storage.SetDataRootForTesting(t.TempDir())
	defer storage.SetDataRootForTesting("")
	cs.game.playthroughID = "test-run"

	kill := func() { killChampion(cs, "hobbit_archer", "easy") }

	kill() // day 0: counts
	kill() // same run+day replay: must NOT count
	if got := arena.Load().Entries[0].TotalKills(); got != 1 {
		t.Fatalf("board kills after same-day replay = %d, want 1", got)
	}

	cs.game.dayNightDay++ // a new morning: counts again
	kill()
	if got := arena.Load().Entries[0].TotalKills(); got != 2 {
		t.Fatalf("board kills after a new day = %d, want 2", got)
	}

	cs.game.playthroughID = "another-run" // fresh playthrough, same day index
	kill()
	board := arena.Load()
	if len(board.Entries) != 2 {
		t.Fatalf("entries after a new run = %d, want a SEPARATE record per run", len(board.Entries))
	}
	total := 0
	for _, e := range board.Entries {
		total += e.TotalKills()
	}
	if total != 3 {
		t.Fatalf("total board kills = %d, want 3 (2 + the new run's 1)", total)
	}
}
