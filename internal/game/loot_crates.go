package game

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// npcIsWalkUpProp: interactables the party walks right up to (chests,
// lecterns). They share the loot-crate render treatment: no on-tile skip and
// no near-cull, so they can't vanish just as the party reaches them.
// Membership is the character package's single source of truth.
func (g *MMGame) npcIsWalkUpProp(npc *character.NPC) bool {
	return npc != nil && character.IsWalkUpPropType(npc.Type)
}

// partyTrapAvoidChancePct is the Disarm Trap mastery table for avoiding a
// chest/door trap ENTIRELY (constants beside the skill's other numbers in
// character/catalog.go). The party's best hand does the work; the flat damage
// reduction stays separate.
func (g *MMGame) partyTrapAvoidChancePct() (int, *character.MMCharacter) {
	best, chance := (*character.MMCharacter)(nil), 0
	for _, m := range g.party.Members {
		if m == nil || m.IsIncapacitated() || !m.HasSkill(character.SkillDisarmTrap) {
			continue
		}
		if c := character.DisarmTrapAvoidBasePct + m.SkillTier(character.SkillDisarmTrap)*character.DisarmTrapAvoidPerTierPct; c > chance {
			chance, best = c, m
		}
	}
	return chance, best
}

// useLootCrate opens a chest: LOS-gated, one-shot, trap first, then loot.
// Tier behavior is fully data-driven from loots.yaml `crates:` (by NPC key).
func (g *MMGame) useLootCrate(npc *character.NPC) {
	if npc.Visited {
		g.AddCombatMessage(fmt.Sprintf("The %s is empty.", npc.Name))
		return
	}
	crate := config.GetCrateConfig(npc.Key)
	if crate == nil {
		return
	}
	// Combat lockout: the Space focus is already combat-blocked, but a mouse
	// click reaches here directly - without this a campfire is a free mid-fight
	// full rest. Checked BEFORE Visited so the attempt doesn't waste the crate.
	if g.partyInCombat() {
		g.AddCombatMessage(fmt.Sprintf("No time to search the %s mid-fight!", npc.Name))
		return
	}
	// Direct line of sight required: no opening through walls, and the
	// axis-stepping ray already refuses diagonal gaps between trees.
	if g.collisionSystem != nil && !g.collisionSystem.CheckLineOfSight(g.camera.X, g.camera.Y, npc.X, npc.Y) {
		g.AddCombatMessage(fmt.Sprintf("You can't reach the %s from here.", npc.Name))
		return
	}
	npc.Visited = true // consumed even if the trap fires - the lid is open

	if crate.TrapDamage > 0 || crate.TrapIgnite {
		g.springCrateTrap(npc, crate)
	}
	g.applyCrateEffects(npc, crate)
	if crate.LootTable != "" {
		loot, gold := rollWeightedLootTable(crate.LootTable)
		g.grantCrateLoot(npc, loot, gold, 0)
		return
	}
	if crate.Rolls > 0 {
		loot, gold, arenaPoints := g.rollCratePool(crate)
		g.grantCrateLoot(npc, loot, gold, arenaPoints)
	}
}

// applyCrateEffects applies a crate's non-loot payload: a campfire's free rest
// and a stat barrel's permanent bonus for the SELECTED character (a green
// effective-stat gain, never a base-stat write; MaxHP/MaxSP re-derive with the
// gain granted to the current pools too).
func (g *MMGame) applyCrateEffects(npc *character.NPC, crate *config.CrateConfig) {
	if crate.FreeRest {
		g.restParty()
		g.AddCombatMessage("The party rests by the warm fire - fully recovered!")
	}
	if crate.BonusStat != "" && crate.BonusAmount != 0 {
		if crate.BonusChancePct > 0 && rand.Intn(100) >= crate.BonusChancePct {
			g.AddCombatMessage(fmt.Sprintf("The %s turns out to be empty.", npc.Name))
			return
		}
		// The draught goes to the SELECTED character - whoever the player has
		// active drinks from the barrel, MM-style.
		if g.selectedChar < 0 || g.selectedChar >= len(g.party.Members) {
			return
		}
		m := g.party.Members[g.selectedChar]
		bonus := character.StatBonusesFromMap(map[string]int{crate.BonusStat: crate.BonusAmount})
		m.PermanentBonuses = m.PermanentBonuses.Add(bonus)
		// Irreversible gain: grant the max-pool delta to CURRENT HP/SP too,
		// same as spending a stat point (not the reversible keep-current path).
		m.RecalculateMaxStatsGrantingGain(g.config)
		g.AddCombatMessage(fmt.Sprintf("%s feels changed forever: %s, permanently!", m.Name, bonus.Summary()))
	}
}

// springCrateTrap fires the chest's trap unless the party's best Disarm Trap
// hand defuses it (40/60/80/100% by mastery tier).
func (g *MMGame) springCrateTrap(npc *character.NPC, crate *config.CrateConfig) {
	if chance, hand := g.partyTrapAvoidChancePct(); chance > 0 && rand.Intn(100) < chance {
		g.AddCombatMessage(fmt.Sprintf("%s disarms the trap on the %s.", hand.Name, npc.Name))
		return
	}
	if crate.TrapIgnite {
		g.AddColoredCombatMessage(fmt.Sprintf("The %s spews a gout of flame!", npc.Name), combatMessageOrange)
		igniteSeconds := crate.TrapIgniteSeconds
		if igniteSeconds <= 0 {
			igniteSeconds = config.DefaultTrapIgniteSeconds
		}
		burnFrames := g.config.GetTPS() * igniteSeconds
		g.combat.forEachDamageablePartyMember(func(idx int, member *character.MMCharacter) {
			member.ApplyBurn(burnFrames)
			g.TriggerPartyFlame(idx)
		})
		return
	}
	damageType := "physical"
	if len(crate.TrapDamageTypes) > 0 {
		damageType = crate.TrapDamageTypes[rand.Intn(len(crate.TrapDamageTypes))]
	}
	damageLabel := strings.ToUpper(damageType[:1]) + damageType[1:]
	g.AddColoredCombatMessage(fmt.Sprintf("The %s detonates a hidden %s charge!", npc.Name, damageLabel), combatMessageOrange)
	g.combat.forEachDamageablePartyMember(func(idx int, member *character.MMCharacter) {
		dealt := g.combat.damagePartyMemberElement(idx, member, crate.TrapDamage, damageType)
		g.AddCombatMessage(fmt.Sprintf("%s takes %d damage! (HP: %d/%d)",
			member.Name, dealt, member.HitPoints, member.MaxHitPoints))
	})
}

// rollCratePool rolls the crate's non-authored loot: the "map" pool draws from
// the drop tables of monster kinds present when the current map was created
// (weighted by their drop chance, filtered by min_rarity); the "rare" pool
// draws catalog rares with a legendary_pct upgrade chance. If roll_sources is
// authored, each roll first picks one weighted source from that list.
func (g *MMGame) rollCratePool(crate *config.CrateConfig) ([]items.Item, int, int) {
	rolls := make([]crateRoll, crate.Rolls)
	for i := 0; i < crate.Rolls; i++ {
		rolls[i] = g.rollCrateItem(crate)
	}

	// A special source is a per-CHEST chance and replaces one normal slot. This
	// keeps rare/legendary odds readable in YAML instead of encoding fractional
	// per-slot probabilities as opaque large weights.
	available := make([]int, crate.Rolls)
	for i := range available {
		available[i] = i
	}
	for _, src := range crate.SpecialRolls {
		if len(available) == 0 || rand.Intn(100) >= src.ChancePct {
			continue
		}
		roll := g.rollCrateSource(src)
		if !roll.ok {
			continue // A map without this rarity keeps its normal slot.
		}
		choice := rand.Intn(len(available))
		rolls[available[choice]] = roll
		available = append(available[:choice], available[choice+1:]...)
	}

	var out []items.Item
	gold, arenaPoints := 0, 0
	for _, roll := range rolls {
		if !roll.ok {
			continue
		}
		if roll.item.Name != "" {
			out = append(out, roll.item)
		}
		gold += roll.gold
		arenaPoints += roll.arenaPoints
	}
	return out, gold, arenaPoints
}

type crateRoll struct {
	item        items.Item
	gold        int
	arenaPoints int
	ok          bool
}

func (g *MMGame) rollCrateItem(crate *config.CrateConfig) crateRoll {
	// roll_sources is the sole per-roll model (load validation guarantees a
	// non-empty list for any non-loot_table crate); a no-mix crate is a
	// one-element list.
	src, ok := pickCrateRollSource(crate.RollSources)
	if !ok {
		return crateRoll{}
	}
	return g.rollCrateSource(src)
}

// weightedPick returns the index of one of n entries chosen in proportion to
// weight(i), or -1 when every weight is <= 0. The single weighted-selection
// primitive behind every loot roll (crate sources, map drops, loot tables).
func weightedPick(n int, weight func(i int) int) int {
	total := 0
	for i := 0; i < n; i++ {
		if w := weight(i); w > 0 {
			total += w
		}
	}
	if total <= 0 {
		return -1
	}
	pick := rand.Intn(total)
	for i := 0; i < n; i++ {
		w := weight(i)
		if w <= 0 {
			continue
		}
		if pick -= w; pick < 0 {
			return i
		}
	}
	return -1
}

func pickCrateRollSource(sources []config.CrateRollSource) (config.CrateRollSource, bool) {
	if i := weightedPick(len(sources), func(i int) int { return sources[i].Weight }); i >= 0 {
		return sources[i], true
	}
	return config.CrateRollSource{}, false
}

func (g *MMGame) rollCrateSource(src config.CrateRollSource) crateRoll {
	switch src.Pool {
	case "nothing":
		return crateRoll{} // weighted empty slot - the crate held nothing this roll
	case "map":
		it, ok := g.rollMapLootEntry(src.Rarity, src.MinRarity, src.MaxRarity)
		return crateRoll{item: it, ok: ok}
	case "rare":
		rarity := "rare"
		if src.LegendaryPct > 0 && rand.Intn(100) < src.LegendaryPct {
			rarity = "legendary"
		}
		it, ok := rollCatalogItemByRarity(rarity)
		return crateRoll{item: it, ok: ok}
	case "catalog":
		it, ok := rollCatalogItem(src.ItemType, src.Rarity, src.MinRarity, src.MaxRarity)
		return crateRoll{item: it, ok: ok}
	case "gold":
		return crateRoll{gold: src.Amount, ok: src.Amount > 0}
	case "arena_points":
		return crateRoll{arenaPoints: src.Amount, ok: src.Amount > 0}
	}
	return crateRoll{}
}

// rollMapLootEntry picks one entry from the union of the per-monster loot
// tables of every fixed monster KIND present when the current map was created,
// weighted by each entry's drop chance. This keeps a chest's loot valid after
// the party clears the map; summons do not alter its pool. exactRarity,
// minRarity and maxRarity provide the tier gates used by crate roll sources.
func (g *MMGame) rollMapLootEntry(exactRarity, minRarity, maxRarity string) (items.Item, bool) {
	if g.world == nil {
		return items.Item{}, false
	}
	type poolEntry struct {
		entry  config.LootEntry
		weight int
	}
	var pool []poolEntry
	minTier := rarityTier(minRarity)
	maxTier := rarityTier(maxRarity)
	keys := g.world.InitialMonsterKeys
	if len(keys) == 0 {
		// Small programmatic worlds in unit tests predate the initial-map snapshot.
		// Their live mob list is the closest available definition of the map pool.
		keys = make(map[string]struct{})
		for _, m := range g.world.Monsters {
			if m != nil {
				keys[m.Key] = struct{}{}
			}
		}
	}
	keyList := make([]string, 0, len(keys))
	for key := range keys {
		keyList = append(keyList, key)
	}
	sort.Strings(keyList)
	for _, key := range keyList {
		for _, e := range config.GetLootTable(key) {
			rarity := lootEntryRarity(e)
			tier := rarityTier(rarity)
			if exactRarity != "" && rarity != exactRarity {
				continue
			}
			if minTier > 0 && tier < minTier {
				continue
			}
			if maxRarity != "" && tier > maxTier {
				continue
			}
			w := int(e.Chance * 1000)
			if w <= 0 {
				continue
			}
			pool = append(pool, poolEntry{e, w})
		}
	}
	i := weightedPick(len(pool), func(i int) int { return pool[i].weight })
	if i < 0 {
		return items.Item{}, false
	}
	it, err := createLootItem(pool[i].entry.Type, pool[i].entry.Key)
	return it, err == nil
}

// lootEntryRarity resolves a loot entry's rarity from its definition.
func lootEntryRarity(e config.LootEntry) string {
	if e.Type == "weapon" {
		if def, ok := config.GetWeaponDefinition(e.Key); ok && def != nil {
			return def.Rarity
		}
		return "common"
	}
	if def, ok := config.GetItemDefinition(e.Key); ok && def != nil {
		return def.Rarity
	}
	return "common"
}

// rollCatalogItemByRarity picks a uniform random weapon or item of the given
// rarity from the whole catalog. Cards are normal collectible loot; quest
// items and arena uniques never drop from chests.
func rollCatalogItemByRarity(rarity string) (items.Item, bool) {
	type candidate struct {
		key    string
		weapon bool
	}
	var pool []candidate
	if config.GlobalItems != nil {
		for key, def := range config.GlobalItems.Items {
			if def == nil || def.Rarity != rarity {
				continue
			}
			if def.Type == "quest" {
				continue
			}
			pool = append(pool, candidate{key, false})
		}
	}
	for _, key := range config.WeaponKeysByRarity(rarity) {
		pool = append(pool, candidate{key, true})
	}
	if len(pool) == 0 {
		return items.Item{}, false
	}
	// Deterministic pool order (map iteration is random) so equal seeds roll
	// equal loot in tests.
	sort.Slice(pool, func(i, j int) bool {
		if pool[i].weapon != pool[j].weapon {
			return !pool[i].weapon
		}
		return pool[i].key < pool[j].key
	})
	pick := pool[rand.Intn(len(pool))]
	if pick.weapon {
		it, err := items.TryCreateWeaponFromYAML(pick.key)
		return it, err == nil
	}
	it, err := items.TryCreateItemFromYAML(pick.key)
	return it, err == nil
}

// rollCatalogItem picks a uniform random non-weapon item from the whole catalog
// by item type and optional rarity gates. This is for crate category rolls such
// as "any consumable", "common armor", or "uncommon accessory".
func rollCatalogItem(itemType, rarity, minRarity, maxRarity string) (items.Item, bool) {
	type candidate struct {
		key string
	}
	var pool []candidate
	minTier := rarityTier(minRarity)
	maxTier := rarityTier(maxRarity)
	if config.GlobalItems != nil {
		for key, def := range config.GlobalItems.Items {
			if def == nil || def.Type != itemType {
				continue
			}
			if rarity != "" && def.Rarity != rarity {
				continue
			}
			if minTier > 0 && rarityTier(def.Rarity) < minTier {
				continue
			}
			if maxRarity != "" && rarityTier(def.Rarity) > maxTier {
				continue
			}
			pool = append(pool, candidate{key: key})
		}
	}
	if len(pool) == 0 {
		return items.Item{}, false
	}
	sort.Slice(pool, func(i, j int) bool { return pool[i].key < pool[j].key })
	pick := pool[rand.Intn(len(pool))]
	it, err := items.TryCreateItemFromYAML(pick.key)
	return it, err == nil
}

// grantCrateLoot hands the rolled loot straight to the party.
func (g *MMGame) grantCrateLoot(npc *character.NPC, loot []items.Item, gold, arenaPoints int) {
	if len(loot) == 0 && gold <= 0 && arenaPoints <= 0 {
		g.AddCombatMessage(fmt.Sprintf("The %s holds nothing but dust.", npc.Name))
		return
	}
	for _, it := range loot {
		g.party.AddItem(it)
		g.AddColoredCombatMessage(fmt.Sprintf("Found %s!", it.Name), lootMessageColor([]items.Item{it}))
	}
	if gold > 0 {
		g.awardGold(gold)
		g.AddCombatMessage(fmt.Sprintf("Found %d gold!", gold))
	}
	if arenaPoints > 0 {
		g.awardArenaPoints(arenaPoints)
		g.AddCombatMessage(fmt.Sprintf("Found %d arena points!", arenaPoints))
	}
}

// useSpellLectern teaches the book's spell to the first party member with the
// school open who doesn't know it. NOT consumed when nobody can learn.
func (g *MMGame) useSpellLectern(npc *character.NPC) {
	if npc.Visited {
		g.AddCombatMessage("The lectern's pages are blank - its spell has been claimed.")
		return
	}
	lectern := npc.Lectern
	if lectern == nil {
		return
	}
	candidates := lectern.Pool
	if lectern.Spell != "" {
		candidates = []string{lectern.Spell}
	}
	// Random pool order so a pool lectern doesn't always teach its first entry.
	shuffled := append([]string(nil), candidates...)
	rand.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	for _, id := range shuffled {
		spellID := spells.SpellID(id)
		def, err := spells.GetSpellDefinitionByID(spellID)
		if err != nil {
			continue
		}
		for _, member := range g.party.Members {
			if member == nil || !member.HasSchoolOpenFor(spellID) || member.KnowsSpell(spellID) {
				continue
			}
			member.LearnSpell(spellID)
			npc.Visited = true
			g.AddColoredCombatMessage(fmt.Sprintf("%s learns %s from the ancient tome!", member.Name, def.Name), combatMessageYellow)
			return
		}
	}
	g.AddCombatMessage("The script swims before every eye - no one here can grasp this spell.")
}
