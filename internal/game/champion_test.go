package game

import (
	"testing"

	"ugataima/internal/arena"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/storage"
)

// primeTestChampions loads champions.yaml and builds the templates on top of a
// combat-system test fixture (which already loaded config/weapons/items/monsters).
func primeTestChampions(t *testing.T, g *MMGame) {
	t.Helper()
	monsterPkg.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if _, err := config.LoadChampionConfig("../../assets/champions.yaml"); err != nil {
		t.Fatalf("load champions: %v", err)
	}
	championTemplates = map[string]*character.MMCharacter{}
	if err := PrimeChampions(g.config); err != nil {
		t.Fatalf("prime champions: %v", err)
	}
}

// TestChampionBuilds verifies both champions build as auto-leveled L25 fighters:
// every level-up stat point spent by the party's own auto-distribution, authored
// gear equipped, authored skill tiers applied.
func TestChampionBuilds(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)

	arch := cs.game.championTemplate("hobbit_archer", "impossible")
	if arch == nil {
		t.Fatal("hobbit_archer template nil")
	}
	if arch.Level != 25 {
		t.Errorf("archer level = %d, want 25", arch.Level)
	}
	if arch.FreeStatPoints != 0 {
		t.Errorf("archer has %d unspent stat points, auto-distribution must spend all", arch.FreeStatPoints)
	}
	base := cs.game.config.Characters.Classes["archer"]
	baseSum := base.Might + base.Intellect + base.Personality + base.Endurance + base.Accuracy + base.Speed + base.Luck
	sum := arch.Might + arch.Intellect + arch.Personality + arch.Endurance + arch.Accuracy + arch.Speed + arch.Luck
	if want := baseSum + 24*StatPointsPerLevel; sum != want {
		t.Errorf("archer stat sum = %d, want class base %d + %d leveled points", sum, baseSum, 24*StatPointsPerLevel)
	}
	if arch.SkillTier(character.SkillBow) != int(character.MasteryGrandMaster) {
		t.Errorf("archer bow tier = %d, want grandmaster", arch.SkillTier(character.SkillBow))
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
// authored (not the character's), four TB swings, ranged weapon with weapon-
// physics range, GM dodge-pierce, character dodge chance, weapon reach.
func TestApplyChampionStatsMirror(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)

	m := monsterPkg.NewMonster3DFromConfig(0, 0, "hobbit_archer", cs.game.config)
	if !m.IsChampion() {
		t.Fatal("hobbit_archer monster not flagged IsChampion")
	}
	cs.game.mirrorChampionStats(m)

	if m.MaxHitPoints != 3000 || m.HitPoints != 3000 {
		t.Errorf("champion HP = %d/%d, want authored boss pool 3000/3000", m.HitPoints, m.MaxHitPoints)
	}
	if m.DamageMin <= 0 || m.DamageMin != m.DamageMax {
		t.Errorf("mirror damage band = [%d,%d], want equal and positive", m.DamageMin, m.DamageMax)
	}
	if m.AttacksPerRound != 4 {
		t.Errorf("champion TB swings = %d, want 4", m.AttacksPerRound)
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

// fillTestParty puts four living level-1 knights in the party - melee-arc
// targets for champion strikes.
func fillTestParty(t *testing.T, g *MMGame) {
	t.Helper()
	class, ok := character.ClassFromKey("knight")
	if !ok {
		t.Fatal("knight class missing")
	}
	g.party.Members = g.party.Members[:0]
	for i := 0; i < 4; i++ {
		ch := character.CreateCharacter("T", class, g.config)
		ch.HitPoints = ch.MaxHitPoints
		g.party.Members = append(g.party.Members, ch)
	}
}

// TestChampionMeleeArc: a champion swing catches as many members as the
// striking hand's arc authors - the mace (arc 2) hits two, Muramasa (arc 3)
// hits three; per-hand mastery true damage lands even through a dodge, so the
// count is exact. Both hands must show up across swings (random hand pick).
func TestChampionMeleeArc(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	fillTestParty(t, cs.game)

	m := monsterPkg.NewMonster3DFromConfig(0, 0, "weapon_master", cs.game.config)
	cs.game.mirrorChampionStats(m)

	// TB swings alternate hands STRICTLY (party parity): expected arc widths
	// derive from each hand's weapon def, main first.
	ch := cs.game.championTemplate("weapon_master", "impossible")
	arcOf := func(name string) int {
		wd := lookupWeaponConfigByName(name)
		if wd == nil || wd.Melee == nil {
			t.Fatalf("no melee def for %q", name)
		}
		return wd.Melee.ArcType
	}
	mainArc := arcOf(ch.Equipment[items.SlotMainHand].Name)
	offArc := arcOf(ch.Equipment[items.SlotOffHand].Name)

	for i := 0; i < 8; i++ {
		for _, mem := range cs.game.party.Members {
			mem.HitPoints = mem.MaxHitPoints
			mem.Conditions = mem.Conditions[:0] // clear last swing's KO
			mem.StunFramesRemaining, mem.StunTurnsRemaining = 0, 0
		}
		if !cs.championAlternatingStrike(m) {
			t.Fatal("champion melee strike failed")
		}
		hit := 0
		for _, mem := range cs.game.party.Members {
			if mem.HitPoints < mem.MaxHitPoints {
				hit++
			}
		}
		want := mainArc
		if i%2 == 1 {
			want = offArc
		}
		if hit != want {
			t.Fatalf("swing %d hit %d members, want the %s hand's arc %d", i, hit, map[bool]string{true: "off", false: "main"}[i%2 == 1], want)
		}
	}
}

// TestChampionRTDualStreams: real-time dual wield runs two INDEPENDENT hand
// cooldowns (party parity) - the off hand fires on its own weapon's cadence
// even when the main hand's attack tick is absent or its cooldown is running.
func TestChampionRTDualStreams(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	primeTestChampions(t, cs.game)
	fillTestParty(t, cs.game)

	m := monsterPkg.NewMonster3DFromConfig(0, 0, "weapon_master", cs.game.config)
	cs.game.mirrorChampionStats(m)
	ch := cs.game.championTemplate("weapon_master", "impossible")

	// Attack tick with both hands ready: both streams strike, both CDs arm to
	// their own weapon's formula values.
	if !cs.championRTDualStrike(m, true) {
		t.Fatal("dual strike not handled")
	}
	if m.AttackCDFrames != m.AttackCooldownFrames() && m.AttackCDFrames <= 0 {
		t.Fatalf("main CD not armed: %d", m.AttackCDFrames)
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

	// Re-arm the cached template with the AoE polearm for this test only.
	orig := championTemplates[championTemplateKey("weapon_master", "impossible")]
	aoeChar := character.CreateCharacter("AoE Test", orig.Class, cs.game.config)
	aoeChar.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("tonbogiri")
	championTemplates[championTemplateKey("weapon_master", "impossible")] = aoeChar
	defer func() { championTemplates[championTemplateKey("weapon_master", "impossible")] = orig }()

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
	// Katana 14% + luck bonus: 200 swings statistically must crit at least once.
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

// TestArenaGladiatorShop: BOTH gladiators (gatekeeper outside, duel master
// inside) carry the points shop as a dialog tab: the authored unlimited
// consumables plus EVERY uncommon weapon at the flat rack price - counts and
// prices derived from content, not restated.
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
		checkArenaShop(t, npc)
	}
}

func checkArenaShop(t *testing.T, npc *character.NPC) {
	t.Helper()
	if npc.Currency != character.CurrencyArenaPoints {
		t.Fatalf("%s currency = %q, want arena_points", npc.Name, npc.Currency)
	}
	uncommon := config.WeaponKeysByRarity("uncommon")
	unique := config.WeaponKeysByRarity("unique")
	// 5 unlimited consumables + 10 unlimited set-armor pieces + the unique
	// weapons (three copies, incl. the Parma shield item) + the uncommon rack.
	wantTotal := 5 + 10 + len(unique) + 1 + len(uncommon)
	if len(npc.MerchantStock) != wantTotal {
		t.Fatalf("stock size = %d, want %d (5 consumables + 10 armor + %d uniques + parma + %d uncommon weapons)",
			len(npc.MerchantStock), wantTotal, len(unique), len(uncommon))
	}
	weaponRack, uniqueRack := 0, 0
	for _, entry := range npc.MerchantStock {
		if !entry.InStock() {
			t.Fatalf("%s not in stock", entry.Item.Name)
		}
		rarity := entry.Item.Rarity
		if entry.Item.Type == items.ItemWeapon {
			if def, _, ok := config.GetWeaponDefinitionByName(entry.Item.Name); ok && def != nil {
				rarity = def.Rarity
			}
		}
		if rarity == "unique" {
			// Arena uniques: 5000 ap, exactly three copies - they do sell out.
			uniqueRack++
			if entry.Cost != 5000 {
				t.Fatalf("unique %s costs %d, want 5000", entry.Item.Name, entry.Cost)
			}
			for range 3 {
				entry.Take()
			}
			if entry.InStock() {
				t.Fatalf("unique %s still in stock after three purchases", entry.Item.Name)
			}
			continue
		}
		entry.Take()
		if !entry.InStock() {
			t.Fatalf("%s sold out after one purchase - stock must be unlimited", entry.Item.Name)
		}
		if entry.Item.Type == items.ItemWeapon {
			weaponRack++
			if entry.Cost != 750 {
				t.Fatalf("weapon %s costs %d, want the flat rack price 750", entry.Item.Name, entry.Cost)
			}
		}
	}
	if uniqueRack != len(unique)+1 {
		t.Fatalf("unique rack = %d, want every unique weapon + the Parma (%d)", uniqueRack, len(unique)+1)
	}
	if weaponRack != len(uncommon) {
		t.Fatalf("weapon rack = %d, want every uncommon weapon (%d)", weaponRack, len(uncommon))
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
