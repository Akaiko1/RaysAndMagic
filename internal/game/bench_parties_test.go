package game

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// benchTierBudget is the mastery a hero can hold at a level: one tier per
// level-up choice (every third level) plus trainer purchases the gold of that
// stage affords.
func benchTierBudget(level int) int { return level/3 + max(0, level-10)/5 }

// benchSourceGates derives from the shipped data the earliest party level at
// which each item can be had. The judgement rules live here only:
//   - class starting kit and alchemy recipe outputs: level 1;
//   - gold shops: level 1, or the merchant's min_party_level;
//   - arena points (tiers open at L20): <=750 -> 21, <=2500 -> 23, else 25;
//   - clock hands (tower mobs L22-30): <=10 -> 23, <=13 -> 25, else 27;
//   - dragon scales: the scale's own gate + 3 for <=2 scales, + 5 above;
//   - drops: the dropper's level; boss_loot: the lowest boss level;
//   - loot tables: a crate's map (its lowest monster level), a prop's map
//     and quest (sword racks), an encounter chest's strongest monster;
//   - quest rewards: the highest min_party_level along the quest chain and
//     its giver.
//
// Random catalog pools in chests are ignored, so the gates are conservative.
func benchSourceGates(t *testing.T, g *MMGame, wm *world.WorldManager) map[string]int {
	t.Helper()
	const never = 1 << 20
	gates := map[string]int{}
	offer := func(key string, level int) {
		if key == "" {
			return
		}
		if old, ok := gates[key]; !ok || level < old {
			gates[key] = max(1, level)
		}
	}
	byName := map[string]string{}
	for k, d := range config.GlobalItems.Items {
		if d != nil {
			byName[d.Name] = k
		}
	}
	for k, d := range config.GlobalWeapons.Weapons {
		if d != nil {
			byName[d.Name] = k
		}
	}
	for _, class := range g.config.Characters.Classes {
		offer(class.MainHand, 1)
		for _, k := range class.Equipment {
			offer(k, 1)
		}
	}
	for _, r := range config.GlobalAlchemy.Recipes {
		offer(r.Output, 1)
	}
	levelOf := map[string]int{}
	bossLevel := never
	for k, d := range monster.MonsterConfig.Monsters {
		levelOf[k] = d.Level
		if d.Boss {
			bossLevel = min(bossLevel, d.Level)
		}
	}
	for k, entries := range config.GlobalLoots.Loots {
		for _, e := range entries {
			if lvl, ok := levelOf[k]; ok {
				offer(e.Key, lvl)
			}
		}
	}
	for _, e := range config.GlobalLoots.BossLoot {
		offer(e.Key, bossLevel)
	}
	mapLevel, mapTop := map[string]int{}, map[string]int{}
	placedOn := map[string][]string{}
	for mk, w := range wm.LoadedMaps {
		lvl := never
		for _, m := range w.Monsters {
			lvl = min(lvl, m.Level)
			mapTop[mk] = max(mapTop[mk], m.Level)
		}
		mapLevel[mk] = lvl
		for _, n := range w.NPCs {
			if n != nil {
				placedOn[n.Key] = append(placedOn[n.Key], mk)
			}
		}
	}
	offerTable := func(table string, level int) {
		if tbl := config.GlobalLoots.WeightedLootTables[table]; tbl != nil {
			for _, e := range tbl.Entries {
				offer(e.Key, level)
			}
		}
	}
	for crateKey, crate := range config.GlobalLoots.Crates {
		tables := []string{crate.LootTable}
		for _, src := range crate.RollSources {
			if src.Pool == "loot_table" {
				tables = append(tables, src.LootTable)
			}
		}
		for _, mk := range placedOn[crateKey] {
			for _, tk := range tables {
				offerTable(tk, mapLevel[mk])
			}
		}
	}
	for mk, mc := range wm.MapConfigs {
		encounters := mc.ClearEncounters
		if mc.ClearEncounter != nil {
			encounters = append(encounters, *mc.ClearEncounter)
		}
		for _, e := range encounters {
			if e.Rewards == nil || e.Rewards.TreasureChest == nil || e.Rewards.TreasureChest.LootTable == "" {
				continue
			}
			level := 0
			for _, req := range e.Monsters {
				level = max(level, levelOf[req.Type])
			}
			if len(e.Monsters) == 0 {
				level = mapTop[mk]
			}
			offerTable(e.Rewards.TreasureChest.LootTable, level)
		}
	}
	defs := g.questManager.Definitions()
	// A quest is offered no earlier than its giver appears.
	giverGate := map[string]int{}
	var walk func(npcLevel int, choices []*character.NPCDialogueChoice)
	walk = func(npcLevel int, choices []*character.NPCDialogueChoice) {
		for _, c := range choices {
			if c == nil {
				continue
			}
			if c.Action == "give_quest" && c.QuestID != "" {
				if old, ok := giverGate[c.QuestID]; !ok || npcLevel < old {
					giverGate[c.QuestID] = npcLevel
				}
			}
			walk(npcLevel, c.Choices)
		}
	}
	for _, d := range character.NPCConfigInstance.NPCs {
		if d != nil && d.Dialogue != nil {
			walk(d.MinPartyLevel, d.Dialogue.Choices)
		}
	}
	chainGate := map[string]int{}
	var gateOf func(id string, seen map[string]bool) int
	gateOf = func(id string, seen map[string]bool) int {
		if v, ok := chainGate[id]; ok {
			return v
		}
		gate := max(defs[id].MinPartyLevel, giverGate[id])
		seen[id] = true
		for pid, p := range defs {
			if p.NextQuest == id && !seen[pid] {
				gate = max(gate, gateOf(pid, seen))
			}
		}
		chainGate[id] = gate
		return gate
	}
	for id, q := range defs {
		for _, k := range append(append([]string(nil), q.Rewards.Items...), q.Rewards.ItemPool...) {
			offer(k, gateOf(id, map[string]bool{}))
		}
	}
	// Props that roll a table (the castle sword racks): their map and quest.
	var walkProps func(npcKey string, choices []*character.NPCDialogueChoice)
	walkProps = func(npcKey string, choices []*character.NPCDialogueChoice) {
		for _, c := range choices {
			if c == nil {
				continue
			}
			if c.Prop != nil && c.Prop.LootTable != "" {
				quest := 0
				if c.QuestID != "" && defs[c.QuestID] != nil {
					quest = gateOf(c.QuestID, map[string]bool{})
				}
				for _, mk := range placedOn[npcKey] {
					offerTable(c.Prop.LootTable, max(mapLevel[mk], quest))
				}
			}
			walkProps(npcKey, c.Choices)
		}
	}
	for key, d := range character.NPCConfigInstance.NPCs {
		if d != nil && d.Dialogue != nil {
			walkProps(key, d.Dialogue.Choices)
		}
	}
	// Currency shops last: scale prices read the scales' own drop gates.
	for key := range character.NPCConfigInstance.NPCs {
		n, err := character.CreateNPCFromConfig(key, 0, 0)
		if err != nil || n == nil {
			continue
		}
		for _, s := range n.MerchantStock {
			itemKey := byName[s.Item.Name]
			switch currency := s.EffectiveCurrency(n.Currency); {
			case currency == "" || currency == "gold":
				offer(itemKey, n.MinPartyLevel)
			case currency == "arena_points":
				offer(itemKey, priceGate(s.Cost, [][2]int{{750, 21}, {2500, 23}}, 25))
			case currency == "item:clock_hand":
				offer(itemKey, priceGate(s.Cost, [][2]int{{10, 23}, {13, 25}}, 27))
			case strings.HasSuffix(currency, "_dragon_scale"):
				if scale, ok := gates[strings.TrimPrefix(currency, "item:")]; ok {
					offer(itemKey, scale+priceGate(s.Cost, [][2]int{{2, 3}}, 5))
				}
			}
		}
	}
	return gates
}

// priceGate is the first band whose price ceiling covers cost, else top.
func priceGate(cost int, bands [][2]int, top int) int {
	for _, b := range bands {
		if cost <= b[0] {
			return b[1]
		}
	}
	return top
}

// Invariant: every bench party builds through the real scenario launcher at
// every catalog level, exactly as authored, within the mastery budget, and
// from gear, cards and items obtainable by that level.
// Case table: every party x every level (the launcher scenario of each).
// Persistence: each built party survives a save/load round trip unchanged.
func TestBenchPartiesBuildAsAuthored(t *testing.T) {
	t.Chdir("../..")
	const scenarios = "assets/test_scenarios.yaml"
	bench, err := LoadBenchCatalog("assets/bench_parties.yaml")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadTestScenarios(scenarios)
	if err != nil {
		t.Fatal(err)
	}
	g, wm, _ := bootOpenWorldGame(t, false)
	loadBenchContent(t)
	gates := benchSourceGates(t, g, wm)
	// Positive control: the gates separate early gear from the endgame.
	top, gatedAbove := slices.Max(bench.Levels), 0
	for _, gate := range gates {
		if gate > top {
			gatedAbove++
		}
	}
	if gatedAbove == 0 {
		t.Fatalf("no catalog item is gated above L%d; the availability rule checks nothing", top)
	}

	classes, races := map[string]bool{}, map[string]bool{}
	var keys []string
	for key := range bench.Parties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, partyKey := range keys {
		p := bench.Parties[partyKey]
		for _, m := range p.Members {
			classes[m.Class] = true
			races[benchRace(m)] = true
		}
		for _, level := range bench.Levels {
			scenarioKey := fmt.Sprintf("bench_%s_%d", partyKey, level)
			t.Run(scenarioKey, func(t *testing.T) {
				if s := catalog[scenarioKey]; s.BenchParty != partyKey || s.Level != level {
					t.Fatalf("launcher scenario %s missing or not bench_party %s at %d", scenarioKey, partyKey, level)
				}
				g, wm, _ := bootOpenWorldGame(t, false)
				if err := g.ApplyTestScenario(scenarios, scenarioKey); err != nil {
					t.Fatal(err)
				}
				checkBenchParty(t, g, p, level, gates)

				data, err := json.Marshal(g.buildSave(wm))
				if err != nil {
					t.Fatal(err)
				}
				var loaded GameSave
				if err := json.Unmarshal(data, &loaded); err != nil {
					t.Fatal(err)
				}
				if err := g.applySave(wm, &loaded); err != nil {
					t.Fatal(err)
				}
				checkBenchParty(t, g, p, level, nil)
			})
		}
	}
	for key := range g.config.Characters.Classes {
		if !classes[key] {
			t.Errorf("no bench party fields a %s", key)
		}
	}
	for key := range g.config.Characters.Races {
		if !races[key] {
			t.Errorf("no bench party fields a %s", key)
		}
	}
}

// loadBenchContent loads the catalogs the bench classes use that the shared
// boot helper leaves out, restoring the previous globals afterwards.
func loadBenchContent(t *testing.T) {
	t.Helper()
	alchemy, traps, techniques := config.GlobalAlchemy, config.GlobalTrapConfig, config.GlobalTechniques
	t.Cleanup(func() {
		config.GlobalAlchemy, config.GlobalTrapConfig, config.GlobalTechniques = alchemy, traps, techniques
	})
	if _, err := config.LoadAlchemyConfig("assets/alchemy_recipes.yaml"); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadTrapConfig("assets/traps.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := config.LoadTechniques("assets/techniques.yaml"); err != nil {
		t.Fatal(err)
	}
}

func benchRace(m BenchMember) string {
	if m.Race == "" {
		return "human"
	}
	return m.Race
}

// checkBenchParty compares the live party with its authored build. gates is
// nil after a reload: availability is a property of the data, not the save.
func checkBenchParty(t *testing.T, g *MMGame, p BenchParty, level int, gates map[string]int) {
	t.Helper()
	available := func(kind, key string) {
		if gates == nil {
			return
		}
		if gate, ok := gates[key]; !ok || gate > level {
			t.Errorf("L%d %s %s is not obtainable yet (earliest %d)", level, kind, key, gate)
		}
	}
	if len(g.party.Members) != len(p.Members) {
		t.Fatalf("party has %d members, want %d", len(g.party.Members), len(p.Members))
	}
	for i, want := range p.Members {
		m := g.party.Members[i]
		build := want.Builds[level]
		race := m.Race
		if race == "" {
			race = "human"
		}
		if m.Name != want.Name || m.Class.Key() != want.Class || race != benchRace(want) || m.Level != level {
			t.Fatalf("member %d is %s %s/%s L%d", i, m.Name, m.Class.Key(), m.Race, m.Level)
		}
		if m.FreeStatPoints != 0 || m.HitPoints != m.MaxHitPoints || m.SpellPoints != m.MaxSpellPoints {
			t.Errorf("%s: unspent stats or not rested", m.Name)
		}
		// Unlisted tiers stay at the fresh kit's; only tiers above it are spent.
		kit := character.CreateCharacter(want.Name, m.Class, g.config)
		if want.Race != "" && want.Race != "human" {
			kit.ApplyRace(want.Race, g.config)
		}
		tiers := 0
		checkTier := func(kind, key string, got, base character.SkillMastery, authored map[string]string) {
			wantTier := strings.ToLower(base.String())
			if authored[key] != "" {
				wantTier = authored[key]
			}
			if strings.ToLower(got.String()) != wantTier {
				t.Errorf("%s: %s %s is %s, authored %s", m.Name, kind, key, got, wantTier)
			}
			tiers += max(0, int(got-base))
		}
		for skill, s := range m.Skills {
			base := character.MasteryNovice
			if k := kit.Skills[skill]; k != nil {
				base = k.Mastery
			}
			checkTier("skill", strings.ReplaceAll(strings.ToLower(skill.String()), " ", "_"), s.Mastery, base, build.Skills)
		}
		for school, s := range m.MagicSchools {
			base := character.MasteryNovice
			if k := kit.MagicSchools[school]; k != nil {
				base = k.Mastery
			}
			checkTier("school", string(school), s.Mastery, base, build.Schools)
		}
		if tiers > benchTierBudget(level) {
			t.Errorf("%s: %d mastery tiers exceed the L%d budget of %d", m.Name, tiers, level, benchTierBudget(level))
		}
		var worn []string
		for _, slot := range items.DisplayEquipSlots {
			if it, ok := m.Equipment[slot]; ok && it.Name != "" {
				worn = append(worn, it.Name)
			}
		}
		var authored []string
		for _, key := range build.Equipment {
			it, _ := benchGear(key)
			authored = append(authored, it.Name)
			available("gear", key)
		}
		slices.Sort(worn)
		slices.Sort(authored)
		if !slices.Equal(worn, authored) {
			t.Errorf("%s wears %v, authored %v", m.Name, worn, authored)
		}
		for j, key := range build.Quick {
			want, _ := benchQuickItem(key)
			if q := m.QuickSlots[j]; q == nil || q.Name != want.Name {
				t.Errorf("%s quick slot %d holds %v, authored %s", m.Name, j, q, key)
			}
		}
		if build.Spell != "" && string(m.Equipment[items.SlotSpell].SpellEffect) != build.Spell {
			t.Errorf("%s readies %q, authored %s", m.Name, m.Equipment[items.SlotSpell].SpellEffect, build.Spell)
		}
		// Every open school knows all of its spells but the party's withheld ones.
		for school, s := range m.MagicSchools {
			want, _ := school.AvailableSpellIDs()
			want = slices.DeleteFunc(want, func(id spells.SpellID) bool { return slices.Contains(p.WithoutSpells, string(id)) })
			got := slices.Clone(s.KnownSpells)
			slices.Sort(got)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("%s knows %s %v, want %v", m.Name, school, got, want)
			}
		}
	}
	var cards []string
	for i := range g.cardSlots {
		if k := g.cardSlots[i].key; k != "" {
			cards = append(cards, k)
		}
	}
	if !slices.Equal(cards, p.Cards[level]) {
		t.Errorf("cards %v, authored %v", cards, p.Cards[level])
	}
	for _, key := range p.Cards[level] {
		available("card", key)
	}
	// The authored items are everything carried, quick slots included; only
	// key items join them.
	authored := map[string]int{}
	for key, n := range p.Items[level] {
		available("item", key)
		it, _ := items.TryCreateItemFromYAML(key)
		authored[it.Name] = n
	}
	held := map[string]int{}
	bags := [][]items.Item{g.party.Inventory}
	for _, m := range g.party.Members {
		bags = append(bags, m.Inventory)
		for _, q := range m.QuickSlots {
			if q != nil && !benchShortcut(*q) {
				bags = append(bags, []items.Item{*q})
				if q.Count() != 1 {
					t.Errorf("%s quick-slots %d %s; the stock stays in the shared bag", m.Name, q.Count(), q.Name)
				}
			}
		}
	}
	for _, bag := range bags {
		for _, it := range bag {
			if it.Type != items.ItemQuest {
				held[it.Name] += it.Count()
			}
		}
	}
	if !maps.Equal(held, authored) {
		t.Errorf("party carries %v, authored %v", held, authored)
	}
}

func TestBenchCatalogFailsOnTypos(t *testing.T) {
	member := "{name: Sora, class: monk, levels: {15: {}}}"
	for _, body := range []string{
		"levels: [15]\nparties: {p: {members: [" + member + "], cardz: {}}}",
		"levels: [15]\nparties: {p: {members: [{name: Sora, class: monster, levels: {15: {}}}]}}",
		"levels: [15, 20]\nparties: {p: {members: [" + member + "]}}",
		"levels: [20]\nparties: {p: {members: [{name: Sora, class: monk, levels: {15: {}, 20: {}}}]}}",
		"levels: [15]\nparties: {p: {members: []}}",
		"levels: [15]\nparties: {}",
		"levels: [15]\nparties: {p: {members: [" + member + "], without_spells: [fireball]}}",
		"levels: [15]\nparties: {p: {members: [" + member + "], items: {15: {health_potion: 0}}}}",
		"levels: [15]\nparties: {p: {members: [" + member + "]}, v: {base: q}}",
		"levels: [15]\nparties: {p: {members: [" + member + "]}, v: {base: p}, w: {base: v}}",
		"levels: [15]\nparties: {p: {members: [" + member + "]}, v: {base: p, members: [" + member + "]}}",
		"levels: [15]\nparties: {p: {members: [" + member + "]}, v: {base: p, spell: {Kai: sparks}}}",
		"levels: [15]\nparties: {p: {members: [" + member + "]}, s: {members: [{name: Sora, from: q}]}}",
		"levels: [15]\nparties: {p: {members: [" + member + "]}, s: {members: [{name: Kai, from: p}]}}",
		"levels: [15]\nparties: {p: {members: [" + member + "]}, s: {members: [{name: Sora, class: monk, from: p}]}}",
		"levels: [15]\nparties: {p: {members: [" + member + "]}, s: {members: [{name: Sora, from: p}]}, t: {members: [{name: Sora, from: s}]}}",
	} {
		path := filepath.Join(t.TempDir(), "bench.yaml")
		os.WriteFile(path, []byte(body), 0600)
		if _, err := LoadBenchCatalog(path); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestBenchVariantCopiesBase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bench.yaml")
	os.WriteFile(path, []byte(`levels: [15, 20]
parties:
  p: {members: [{name: Sora, class: monk, levels: {15: {spell: sparks, quick: [bless]}, 20: {}}}], gold: {15: 5}}
  v: {note: variant, base: p, without_spells: [bless], spell: {Sora: firebolt}}
`), 0600)
	cat, err := LoadBenchCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	p, v := cat.Parties["p"], cat.Parties["v"]
	for level, want := range map[int]string{15: "firebolt", 20: "firebolt"} {
		if got := v.Members[0].Builds[level].Spell; got != want {
			t.Errorf("variant L%d readies %q, want %q", level, got, want)
		}
	}
	if b := v.Members[0].Builds[15]; !slices.Equal(b.Quick, []string{"bless"}) || v.Gold[15] != 5 || v.Note != "variant" {
		t.Errorf("variant lost the base build: %+v gold %v note %q", b, v.Gold, v.Note)
	}
	if got := p.Members[0].Builds[15].Spell; got != "sparks" || p.WithoutSpells != nil {
		t.Errorf("base changed: readies %q, withholds %v", got, p.WithoutSpells)
	}
}

func TestBenchMemberFromCopiesBuild(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bench.yaml")
	os.WriteFile(path, []byte(`levels: [15]
parties:
  p: {members: [{name: Sora, class: monk, race: halfling, levels: {15: {spell: sparks}}}, {name: Kai, class: knight, levels: {15: {}}}]}
  s: {members: [{name: Sora, from: p}], gold: {15: 7}}
`), 0600)
	cat, err := LoadBenchCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	got, src := cat.Parties["s"], cat.Parties["p"]
	if len(got.Members) != 1 || got.Members[0].Class != "monk" || got.Members[0].Race != "halfling" ||
		got.Members[0].Builds[15].Spell != "sparks" || got.Gold[15] != 7 {
		t.Fatalf("copied member %+v, gold %v", got.Members, got.Gold)
	}
	if len(src.Members) != 2 || src.Members[0].From != "" {
		t.Errorf("source party changed: %+v", src.Members)
	}
}

// A build the game could not produce fails instead of quietly differing.
func TestBenchBuildFailsLoudly(t *testing.T) {
	t.Chdir("../..")
	g, _, _ := bootOpenWorldGame(t, false)
	loadBenchContent(t)
	for name, tc := range map[string]struct {
		class, race string
		build       BenchBuild
		cards       []string
		carried     map[string]int
		without     []string
		valid       bool
	}{
		"positive control": {class: "monk", race: "halfling", valid: true, cards: []string{"wolf_card"}, carried: map[string]int{"health_potion": 2},
			build: BenchBuild{Skills: map[string]string{"iron_body": "master"}, Equipment: []string{"monk_fists"}, Quick: []string{"health_potion"}}},
		"quick-slots an item not carried": {class: "monk", build: BenchBuild{Quick: []string{"health_potion"}}},
		"skill outside the kit":           {class: "cleric", build: BenchBuild{Skills: map[string]string{"sword": "expert"}}},
		"unknown mastery":                 {class: "monk", build: BenchBuild{Skills: map[string]string{"iron_body": "legend"}}},
		"unknown school":                  {class: "monk", build: BenchBuild{Schools: map[string]string{"chaos": "expert"}}},
		"unknown race":                    {class: "monk", race: "elf"},
		"armor without the skill":         {class: "sorcerer", build: BenchBuild{Equipment: []string{"iron_armor"}}},
		"unknown gear":                    {class: "monk", build: BenchBuild{Equipment: []string{"no_such_item"}}},
		"readied spell unknown":           {class: "knight", build: BenchBuild{Spell: "fireball"}},
		"quick spell unknown":             {class: "knight", build: BenchBuild{Quick: []string{"fireball"}}},
		"technique off its class":         {class: "knight", build: BenchBuild{Quick: []string{"fold_step"}}},
		"flask without the skill":         {class: "knight", build: BenchBuild{Quick: []string{"fire_flask"}}},
		"more quick keys than slots":      {class: "monk", build: BenchBuild{Quick: []string{"health_potion", "health_potion", "health_potion", "health_potion", "health_potion", "health_potion"}}},
		"not a card":                      {class: "monk", cards: []string{"health_potion"}},
		"withheld spell":                  {class: "battle_mage", without: []string{"earthquake"}, valid: true},
		"withholds an unlearned one":      {class: "knight", without: []string{"earthquake"}},
		"readies a withheld spell":        {class: "battle_mage", without: []string{"earthquake"}, build: BenchBuild{Spell: "earthquake"}},
		"quick-slots a withheld one":      {class: "battle_mage", without: []string{"firewall"}, build: BenchBuild{Quick: []string{"firewall"}}},
		"kit readies a withheld one":      {class: "battle_mage", without: []string{"rock_blast"}},
		"kit spell replaced":              {class: "battle_mage", without: []string{"rock_blast"}, build: BenchBuild{Spell: "sparks"}, valid: true},
	} {
		t.Run(name, func(t *testing.T) {
			p := BenchParty{
				Members: []BenchMember{{Name: "Probe", Class: tc.class, Race: tc.race, Builds: map[int]BenchBuild{15: tc.build}}},
				Cards:   map[int][]string{15: tc.cards},
				Items:   map[int]map[string]int{15: tc.carried},

				WithoutSpells: tc.without,
			}
			members, err := g.buildBenchMembers(p, 15)
			if err == nil {
				g.party = character.NewPartyFromGroups(g.config, members, nil, nil)
				err = g.applyBenchPartyState(p, 15)
			}
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, got %v", tc.valid, err)
			}
		})
	}
}
