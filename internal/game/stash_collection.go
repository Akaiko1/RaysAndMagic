package game

import (
	"maps"
	"slices"
	"strings"

	uitext "ugataima/assets/text"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/stash"
)

// The Storage Clerk keeps two cross-save collections inside the shared stash:
// a showcase with one place per legendary (weapons and gear; never quest
// items, cards or a class's starting kit) and a card binder with a pocket per
// monster card that holds up to stash.AlbumCopies of it. Both live in the same
// stash file, journal and lineage claims as the chest.

// collectionCatalog is the showcase and binder order, built once per loaded
// catalog and class config (the class kits are excluded).
type collectionCatalog struct {
	items     *config.ItemSystemConfig
	weapons   *config.WeaponSystemConfig
	classes   *config.Config
	legendary []string
	cards     []string
	isStamp   map[string]bool // showcase keys
	isCard    map[string]bool // binder keys
}

var collectionCache collectionCatalog

// collectionGearOrder groups the non-weapon showcase places after the weapons.
var collectionGearOrder = map[string]int{"armor": 0, "accessory": 1, "trinket": 2, "device": 3}

func collectionLists() collectionCatalog {
	if collectionCache.legendary != nil && collectionCache.items == config.GlobalItems && collectionCache.weapons == config.GlobalWeapons && collectionCache.classes == config.GlobalConfig {
		return collectionCache
	}
	c := collectionCatalog{items: config.GlobalItems, weapons: config.GlobalWeapons, classes: config.GlobalConfig, isStamp: map[string]bool{}, isCard: map[string]bool{}}
	// A class's starting kit is not a find (the Monk's Fists never leave his hands).
	kit := map[string]bool{}
	if config.GlobalConfig != nil {
		for _, class := range config.GlobalConfig.Characters.Classes {
			kit[class.MainHand] = true
			for _, key := range class.Equipment {
				kit[key] = true
			}
		}
	}
	type entry struct {
		key, name string
		group     int
	}
	var stamps []entry
	if config.GlobalWeapons != nil {
		for key, d := range config.GlobalWeapons.Weapons {
			if d.Rarity == "legendary" && !kit[key] {
				stamps = append(stamps, entry{key, d.Name, -1})
			}
		}
	}
	var cards []entry
	if config.GlobalItems != nil {
		for key, d := range config.GlobalItems.Items {
			switch {
			case d.Type == "card":
				cards = append(cards, entry{key, d.Name, 0})
			case d.Rarity == "legendary" && d.Type != "quest" && !kit[key]:
				group, ok := collectionGearOrder[d.Type]
				if !ok {
					group = len(collectionGearOrder)
				}
				stamps = append(stamps, entry{key, d.Name, group})
			}
		}
	}
	byGroupThenName := func(a, b entry) int {
		if a.group != b.group {
			return a.group - b.group
		}
		return strings.Compare(a.name, b.name)
	}
	slices.SortFunc(stamps, byGroupThenName)
	slices.SortFunc(cards, byGroupThenName)
	for _, e := range stamps {
		c.legendary = append(c.legendary, e.key)
		c.isStamp[e.key] = true
	}
	for _, e := range cards {
		c.cards = append(c.cards, e.key)
		c.isCard[e.key] = true
	}
	if c.legendary == nil {
		c.legendary = []string{}
	}
	collectionCache = c
	return c
}

// LegendaryCollectionKeys lists every legendary with a showcase place,
// weapons first, then armor, accessories, trinkets and devices, each by name.
func LegendaryCollectionKeys() []string { return collectionLists().legendary }

// CardAlbumKeys lists every monster card with a binder pocket, by name.
func CardAlbumKeys() []string { return collectionLists().cards }

// legendaryStampKey is the showcase place an item fills, "" when it has none.
func legendaryStampKey(it items.Item) string {
	key := ""
	if it.Type == items.ItemWeapon {
		if _, k, ok := config.GetWeaponDefinitionByName(it.Name); ok {
			key = k
		}
	} else if _, k, ok := config.GetItemDefinitionByName(it.Name); ok {
		key = k
	}
	if key != "" && collectionLists().isStamp[key] {
		return key
	}
	return ""
}

// collectionBagKey is the showcase place or binder pocket a bag item files
// into, "" when the item belongs to neither. Bag items are normalized at load.
func collectionBagKey(it items.Item, album bool) string {
	if album {
		return itemCardKey(it)
	}
	return legendaryStampKey(it)
}

// collectionPlaceFull is the one capacity rule: a showcase place holds one
// legendary, a binder pocket stash.AlbumCopies cards.
func (g *MMGame) collectionPlaceFull(key string, album bool) bool {
	if album {
		return len(g.stash.Album[key]) >= stash.AlbumCopies
	}
	_, filled := g.stash.Collection[key]
	return filled
}

// returnUnlistedCollectionItems moves showcase and binder entries whose key
// the catalog no longer lists (re-rated, made a class kit, removed) into free
// chest cells, so a stored item never becomes unreachable. Entries stay put
// while the chest is full. Reports whether anything moved.
func returnUnlistedCollectionItems(st *stash.Stash) bool {
	lists := collectionLists()
	moved := false
	place := func(it items.Item) bool {
		for i := range st.Slots {
			if stash.IsEmpty(st.Slots[i]) {
				st.Slots[i] = it
				moved = true
				return true
			}
		}
		return false
	}
	for _, key := range slices.Sorted(maps.Keys(st.Collection)) {
		if !lists.isStamp[key] && place(st.Collection[key]) {
			delete(st.Collection, key)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(st.Album)) {
		if lists.isCard[key] {
			continue
		}
		page := st.Album[key]
		for len(page) > 0 && place(page[len(page)-1]) {
			page = page[:len(page)-1]
		}
		if len(page) == 0 {
			delete(st.Album, key)
		} else {
			st.Album[key] = page
		}
	}
	return moved
}

// fileIntoCollection moves one unit of bag item idx into its showcase place
// or binder pocket. It is a stash transfer: journalled, autosaved, rolled back
// on failure.
func (g *MMGame) fileIntoCollection(idx int, album bool) bool {
	if g.stash == nil || g.party == nil || idx < 0 || idx >= len(g.party.Inventory) {
		return false
	}
	key := collectionBagKey(g.party.Inventory[idx], album)
	if key == "" {
		return false
	}
	if g.collectionPlaceFull(key, album) {
		if album {
			g.AddCombatMessage(uitext.Text("collection.page_full", stash.AlbumCopies))
		} else {
			g.AddCombatMessage(uitext.Text("collection.already_filed"))
		}
		return false
	}
	if !g.finishPendingStashTransfer() {
		return false
	}
	rollback := g.stashSnapshot()
	unit, ok := g.party.TakeStackUnits(idx, 1)
	if !ok {
		unit = g.party.Inventory[idx]
		g.party.RemoveItem(idx)
	}
	if album {
		if g.stash.Album == nil {
			g.stash.Album = map[string][]items.Item{}
		}
		g.stash.Album[key] = append(g.stash.Album[key], unit)
	} else {
		if g.stash.Collection == nil {
			g.stash.Collection = map[string]items.Item{}
		}
		g.stash.Collection[key] = unit
	}
	return g.commitStashTransfer(rollback)
}

// takeFromCollection returns a showcased legendary, or the last card of a
// binder pocket, to the party bag through the same transfer.
func (g *MMGame) takeFromCollection(key string, album bool) bool {
	if g.stash == nil || g.party == nil {
		return false
	}
	if !g.finishPendingStashTransfer() {
		return false
	}
	rollback := g.stashSnapshot()
	var unit items.Item
	if album {
		page := g.stash.Album[key]
		if len(page) == 0 {
			return false
		}
		unit = page[len(page)-1]
		if len(page) == 1 {
			delete(g.stash.Album, key)
		} else {
			g.stash.Album[key] = slices.Clone(page[:len(page)-1])
		}
	} else {
		stamp, ok := g.stash.Collection[key]
		if !ok {
			return false
		}
		unit = stamp
		delete(g.stash.Collection, key)
	}
	g.party.AddItem(unit)
	return g.commitStashTransfer(rollback)
}
