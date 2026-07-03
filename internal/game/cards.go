package game

import (
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
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

// cardFullArtSprite returns a card's full-art sprite name ("full_art_<key>"
// by convention) and whether that art exists — cards without one simply have
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
	for _, key := range g.cardCollection {
		if def := cardDef(key); def != nil {
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
	for _, key := range g.cardCollection {
		if def := cardDef(key); def != nil && len(def.CardStatBonuses) > 0 {
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
	for _, key := range g.cardCollection {
		if def := cardDef(key); def != nil && def.CardSummonChance > 0 && def.CardSummonMonster != "" {
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

// resetCardCollection empties the collection and recomputes party stats. Called
// when a new game starts so a fresh party never inherits the old run's card
// effects (move speed, actions, stat bonuses, walk-on-water, summons...).
func (g *MMGame) resetCardCollection() {
	g.cardCollection = [MaxCardSlots]string{}
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
	for _, key := range g.cardCollection {
		if def := cardDef(key); def != nil && def.CardWalkOnWater {
			return true
		}
	}
	return false
}

// cardEffectText derives the human-readable collection effect from a card's
// attributes (ASCII only — the bitmap font has no glyph for unicode dashes).
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
	for i, k := range g.cardCollection {
		if k == "" {
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
	g.cardCollection[slot] = key
	g.party.Inventory = append(g.party.Inventory[:inv], g.party.Inventory[inv+1:]...)
	g.recomputeStatBonuses() // fold the card's Speed bonus into the party stats
	return true
}

// removeCardToInventory returns the card in slot back to the party inventory.
// Cards carry no instance state, so it is rebuilt fresh from its key.
func (g *MMGame) removeCardToInventory(slot int) bool {
	if slot < 0 || slot >= MaxCardSlots || g.cardCollection[slot] == "" {
		return false
	}
	key := g.cardCollection[slot]
	g.cardCollection[slot] = ""
	g.party.Inventory = append(g.party.Inventory, items.CreateItemFromYAML(key))
	g.recomputeStatBonuses()
	return true
}

// inventoryCardIndices lists the inventory slots holding collectible cards, in
// order — the left panel of the collector dialog and its click mapping.
func (g *MMGame) inventoryCardIndices() []int {
	var out []int
	for i := range g.party.Inventory {
		if itemCardKey(g.party.Inventory[i]) != "" {
			out = append(out, i)
		}
	}
	return out
}
