package game

import (
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
)

// splitPhysToFire divides physical damage into a remaining-physical part and a
// fire part, diverting pct% to fire (Archmage Card). pct clamps to 0..100.
func splitPhysToFire(damage, pct int) (phys, fire int) {
	if pct <= 0 || damage <= 0 {
		return damage, 0
	}
	if pct > 100 {
		pct = 100
	}
	fire = damage * pct / 100
	return damage - fire, fire
}

// physConvShare is one elemental share carved out of physical damage by a
// conversion card (Archmage=fire, Hexer=dark, Isis=light).
type physConvShare struct {
	element string
	amount  int
}

// splitPhysConversions carves every conversion card's share out of a physical
// damage total, in fixed fire->dark->light order (each card converts a share of
// what the previous ones left). One rule for ALL party-dealt physical damage -
// melee, ranged and traps; each share is then mitigated as its own element.
func (g *MMGame) splitPhysConversions(damage int) (int, []physConvShare) {
	var shares []physConvShare
	for _, c := range [...]struct {
		element string
		pct     int
	}{
		{"fire", g.cardPhysToFirePct()},
		{"dark", g.cardPhysToDarkPct()},
		{"light", g.cardPhysToLightPct()},
	} {
		var amt int
		damage, amt = splitPhysToFire(damage, c.pct)
		if amt > 0 {
			shares = append(shares, physConvShare{c.element, amt})
		}
	}
	return damage, shares
}

// MaxCardSlots is the size of the party-wide monster-card collection. Cards held
// here grant passive party-wide effects; only the desert card collector can
// place or remove them (see drawCardCollectorDialog).
const MaxCardSlots = 8

// cardDef returns the item definition for a card key (nil if missing).
func cardDef(key string) *config.ItemDefinitionConfig {
	if key == "" {
		return nil
	}
	if def, ok := config.GetItemDefinition(key); ok {
		return def
	}
	return nil
}

// itemCardKey returns the card key for an inventory item, or "" if it isn't a
// collectible card. Cards are gated by Type (items.ItemCard), not by name.
func itemCardKey(it items.Item) string {
	if it.Type != items.ItemCard {
		return ""
	}
	if _, key, ok := config.GetItemDefinitionByName(it.Name); ok {
		return key
	}
	return ""
}

// cardSlot is one collection slot: the resolved card key (gameplay truth,
// read on hot combat/frame paths) plus the physical card item whose
// InstanceID survives into saves for stash reconciliation. The key is
// resolved ONCE in setCardCollectionSlot - never re-derived from the item's
// name at read time (GetItemDefinitionByName is a linear scan).
type cardSlot struct {
	key  string
	item items.Item
}

func (g *MMGame) cardCollectionKey(slot int) string {
	if slot < 0 || slot >= MaxCardSlots {
		return ""
	}
	return g.cardSlots[slot].key
}

func (g *MMGame) setCardCollectionSlot(slot int, it items.Item) bool {
	if slot < 0 || slot >= MaxCardSlots {
		return false
	}
	normalizeItemFromConfig(&it)
	key := itemCardKey(it)
	if key == "" {
		return false
	}
	items.EnsureInstanceID(&it)
	g.cardSlots[slot] = cardSlot{key: key, item: it}
	return true
}

func (g *MMGame) clearCardCollectionSlot(slot int) {
	if slot < 0 || slot >= MaxCardSlots {
		return
	}
	g.cardSlots[slot] = cardSlot{}
}

func (g *MMGame) cardCollectionItem(slot int) items.Item {
	if slot < 0 || slot >= MaxCardSlots {
		return items.Item{}
	}
	s := g.cardSlots[slot]
	if s.item.Name != "" {
		return s.item
	}
	// Key without a physical item (legacy save migrated, or a test seeding the
	// key directly): rebuild the card fresh from its definition.
	if cardDef(s.key) == nil {
		return items.Item{}
	}
	it := items.CreateItemFromYAML(s.key)
	items.EnsureInstanceID(&it)
	return it
}

func (g *MMGame) stashOwnsCardKey(key string) bool {
	if key == "" || !g.ensureStashLoaded() {
		return false
	}
	for _, it := range g.stash.Slots {
		if itemCardKey(it) == key {
			return true
		}
	}
	for _, it := range g.stash.CardSlots {
		if itemCardKey(it) == key {
			return true
		}
	}
	return false
}

// cardFullArtSprite returns a card's full-art sprite name ("full_art_<key>"
// by convention) and whether that art exists - cards without one simply have
// no SHIFT view.
func (g *MMGame) cardFullArtSprite(key string) (string, bool) {
	if key == "" || g.sprites == nil {
		return "", false
	}
	name := "full_art_" + key
	return name, g.sprites.HasSprite(name)
}

// cardCollectionBonus sums one per-card integer effect across the active
// collection (the single source for both the mechanic and the effect text).
func (g *MMGame) cardCollectionBonus(get func(*config.ItemDefinitionConfig) int) int {
	total := 0
	for slot := 0; slot < MaxCardSlots; slot++ {
		if def := cardDef(g.cardCollectionKey(slot)); def != nil {
			total += get(def)
		}
	}
	return total
}

func (g *MMGame) cardMoveSpeedPct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardMoveSpeedPct })
}

func (g *MMGame) cardBonusActions() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardBonusActions })
}

// cardStatBonuses sums the flat party-wide stat bonuses across the collection,
// reusing StatBonusesFromMap (the same converter spells use for stat_bonuses).
func (g *MMGame) cardStatBonuses() character.StatBonuses {
	var sum character.StatBonuses
	for slot := 0; slot < MaxCardSlots; slot++ {
		if def := cardDef(g.cardCollectionKey(slot)); def != nil && len(def.CardStatBonuses) > 0 {
			sum = sum.Add(character.StatBonusesFromMap(def.CardStatBonuses))
		}
	}
	return sum
}

func (g *MMGame) cardRangedDmgPct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardRangedDmgPct })
}

func (g *MMGame) cardMeleeTrueDmg() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardMeleeTrueDmg })
}

func (g *MMGame) cardPhysToFirePct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardPhysToFirePct })
}

func (g *MMGame) cardHealOnAttackPct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardHealOnAtkPct })
}

func (g *MMGame) cardHealAmount() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardHealAmount })
}

func (g *MMGame) cardSummonChance() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardSummonChance })
}

func (g *MMGame) cardSummonLimit() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardSummonLimit })
}

// cardSummonOwner tags monsters summoned by the card collection (so they count
// against the summon limit and restore as allies after a save).
const cardSummonOwner = "card_collection"

// cardSummonMonsterKey is the ally summoned by the collection's summon cards
// (the first one set), or "" if none.
func (g *MMGame) cardSummonMonsterKey() string {
	for slot := 0; slot < MaxCardSlots; slot++ {
		if def := cardDef(g.cardCollectionKey(slot)); def != nil && def.CardSummonChance > 0 && def.CardSummonMonster != "" {
			return def.CardSummonMonster
		}
	}
	return ""
}

func (g *MMGame) cardLethalSavePct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardLethalSavePct })
}

func (g *MMGame) cardMoveAoePct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardMoveAoePct })
}

func (g *MMGame) cardMoveAoeDmg() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardMoveAoeDmg })
}

func (g *MMGame) cardDisintegratePct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardDisintegratePct })
}

func (g *MMGame) cardRegenPct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardRegenPct })
}

func (g *MMGame) cardDoubleAttackPct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardDoubleAttackPct })
}

func (g *MMGame) cardSpellProcPct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardSpellProcPct })
}

func (g *MMGame) cardDodgeBonusPct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardDodgeBonusPct })
}

func (g *MMGame) cardArmorBonus() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardArmorBonus })
}

func (g *MMGame) cardThornsPct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardThornsPct })
}

func (g *MMGame) cardPhysToDarkPct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardPhysToDarkPct })
}

func (g *MMGame) cardPhysToLightPct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardPhysToLightPct })
}

// cardPoisonProc returns the summed on-hit poison chance and the longest
// duration from poison-proc cards. Chance stacks like the other card proc
// percentages; duration is not additive, so multiple copies keep the best
// duration instead of stretching poison indefinitely.
func (g *MMGame) cardPoisonProc() (chancePct int, durationSec int) {
	for slot := 0; slot < MaxCardSlots; slot++ {
		if def := cardDef(g.cardCollectionKey(slot)); def != nil && def.CardPoisonProcPct > 0 {
			chancePct += def.CardPoisonProcPct
			if def.CardPoisonDurationSec > durationSec {
				durationSec = def.CardPoisonDurationSec
			}
		}
	}
	return chancePct, durationSec
}

func (g *MMGame) cardMeleeDmgPct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardMeleeDmgPct })
}

func (g *MMGame) cardMaxHPBonus() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardMaxHPBonus })
}

// cardResistBonus sums the party's card-granted elemental resist bonuses.
func (g *MMGame) cardResistBonus() map[string]int {
	sum := make(map[string]int)
	for slot := 0; slot < MaxCardSlots; slot++ {
		if def := cardDef(g.cardCollectionKey(slot)); def != nil {
			for school, pct := range def.CardResistBonus {
				sum[school] += pct
			}
		}
	}
	return sum
}

// cardResistBonusFor returns the party's card-granted resist bonus for a single
// damage school (the hot path used by combat, avoiding a map alloc per hit).
func (g *MMGame) cardResistBonusFor(school string) int {
	total := 0
	for slot := 0; slot < MaxCardSlots; slot++ {
		if def := cardDef(g.cardCollectionKey(slot)); def != nil {
			total += def.CardResistBonus[school]
		}
	}
	return total
}

func (g *MMGame) cardGoldFindPct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardGoldFindPct })
}

func (g *MMGame) cardBonusBoltPct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardBonusBoltPct })
}

func (g *MMGame) cardVolleyBonusPct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardVolleyBonusPct })
}

func (g *MMGame) cardStunOnHitPct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardStunOnHitPct })
}

func (g *MMGame) cardPoisonResistPct() int {
	total := g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardPoisonResistPct })
	if total > 100 {
		return 100
	}
	return total
}

func (g *MMGame) cardCritBonusPct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardCritBonusPct })
}

func (g *MMGame) cardArmorPiercePct() int {
	return g.cardCollectionBonus(func(d *config.ItemDefinitionConfig) int { return d.CardArmorPiercePct })
}

// cardBonusVsMultiplier mirrors weaponBonusMultiplier but sources from the card
// collection and also matches the monster's Type (e.g. "formless") in addition
// to its Name/Key - letting a card grant "+dmg vs a whole creature category"
// the way a weapon's bonus_vs can't.
func (g *MMGame) cardBonusVsMultiplier(monster *monsterPkg.Monster3D) float64 {
	if monster == nil {
		return 1.0
	}
	candidates := []string{monster.Name}
	if monster.Key != "" {
		candidates = append(candidates, monster.Key)
	}
	if monster.MonsterType != "" {
		candidates = append(candidates, monster.MonsterType)
	}
	mult := 1.0
	for slot := 0; slot < MaxCardSlots; slot++ {
		def := cardDef(g.cardCollectionKey(slot))
		if def == nil || len(def.CardBonusVs) == 0 {
			continue
		}
		for bonusKey, m := range def.CardBonusVs {
			if m <= 0 {
				continue
			}
			// Name/Key/MonsterType often name the same identity (e.g. the Dragon
			// monster has Name="Dragon", Key="dragon", MonsterType="dragon") - one
			// matching bonus_vs entry means "this card applies to this monster",
			// not "multiply once per field that happened to match", so stop at the
			// first hit instead of checking the remaining candidates.
			for _, candidate := range candidates {
				if strings.EqualFold(bonusKey, candidate) {
					mult *= m
					break
				}
			}
		}
	}
	return mult
}

// resetCardCollection empties the collection and recomputes party stats. Called
// when a new game starts so a fresh party never inherits the old run's card
// effects (move speed, actions, stat bonuses, walk-on-water, summons...).
func (g *MMGame) resetCardCollection() {
	g.cardSlots = [MaxCardSlots]cardSlot{}
	g.cardBurstTileX, g.cardBurstTileY = 0, 0
	g.recomputeStatBonuses()
}

// maybeCardMoveBurst fires the Gorilla Titan on-move shockwave at most once per
// tile the party enters (movePlayer runs every frame; this gates it to steps).
func (g *MMGame) maybeCardMoveBurst() {
	if g.cardMoveAoePct() <= 0 || g.combat == nil {
		return
	}
	ts := float64(g.config.GetTileSize())
	if ts <= 0 {
		return
	}
	tx, ty := int(g.camera.X/ts), int(g.camera.Y/ts)
	if tx == g.cardBurstTileX && ty == g.cardBurstTileY {
		return
	}
	g.cardBurstTileX, g.cardBurstTileY = tx, ty
	g.combat.tryCardMoveBurst()
}

// hasCardWalkOnWater reports whether any collected card grants walk-on-water.
func (g *MMGame) hasCardWalkOnWater() bool {
	for slot := 0; slot < MaxCardSlots; slot++ {
		if def := cardDef(g.cardCollectionKey(slot)); def != nil && def.CardWalkOnWater {
			return true
		}
	}
	return false
}

// cardEffectText derives the human-readable collection effect from a card's
// attributes (ASCII only - the bitmap font has no glyph for unicode dashes).
func cardEffectText(def *config.ItemDefinitionConfig) string {
	if def == nil {
		return ""
	}
	parts := def.CardEffectLines() // single source (config/item_lines.go)
	if len(parts) == 0 {
		return "Currently not implemented"
	}
	return strings.Join(parts, ", ")
}

// firstFreeCardSlot returns the first empty collection slot, or -1 if full.
func (g *MMGame) firstFreeCardSlot() int {
	for i := 0; i < MaxCardSlots; i++ {
		if g.cardCollectionKey(i) == "" {
			return i
		}
	}
	return -1
}

// placeCardFromInventory moves the card at inventory index inv into the first
// free collection slot. Collector-only. Returns false if it isn't a card, the
// index is bad, or the collection is full.
func (g *MMGame) placeCardFromInventory(inv int) bool {
	if inv < 0 || inv >= len(g.party.Inventory) {
		return false
	}
	key := itemCardKey(g.party.Inventory[inv])
	if key == "" {
		return false
	}
	slot := g.firstFreeCardSlot()
	if slot < 0 {
		return false
	}
	if !g.setCardCollectionSlot(slot, g.party.Inventory[inv]) {
		return false
	}
	g.party.Inventory = append(g.party.Inventory[:inv], g.party.Inventory[inv+1:]...)
	g.recomputeStatBonuses() // fold the card's Speed bonus into the party stats
	return true
}

// removeCardToInventory returns the physical card in slot back to the party
// inventory, preserving its InstanceID for cross-save stash reconciliation.
func (g *MMGame) removeCardToInventory(slot int) bool {
	if slot < 0 || slot >= MaxCardSlots || g.cardCollectionKey(slot) == "" {
		return false
	}
	it := g.cardCollectionItem(slot)
	g.clearCardCollectionSlot(slot)
	if itemCardKey(it) != "" {
		g.party.Inventory = append(g.party.Inventory, it)
	}
	g.recomputeStatBonuses()
	return true
}

// inventoryCardIndices lists the inventory slots holding collectible cards, in
// order - the left panel of the collector dialog and its click mapping.
func (g *MMGame) inventoryCardIndices() []int {
	var out []int
	for i := range g.party.Inventory {
		if itemCardKey(g.party.Inventory[i]) != "" {
			out = append(out, i)
		}
	}
	return out
}
