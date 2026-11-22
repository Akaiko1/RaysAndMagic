package game

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/stash"
)

// Filing and taking back, per kind: a showcase place takes one legendary and
// refuses a second; a stack files one unit; a binder pocket takes up to
// stash.AlbumCopies cards; anything without a place - quest items, rares,
// uniques, cards in the showcase, a class's starting kit - never leaves the bag.
func TestCollectionFilingMovesOneUnitThroughTheStash(t *testing.T) {
	loadTestConfig(t)
	weapon := ""
	for _, key := range LegendaryCollectionKeys() {
		if _, ok := config.GetWeaponDefinition(key); ok {
			weapon = key
			break
		}
	}
	for _, tc := range []struct {
		name   string
		bag    []items.Item
		album  bool
		files  int // successful filings in a row
		bagEnd int // units left in the bag
	}{
		{"legendary weapon, then its twin", []items.Item{items.CreateWeaponFromYAML(weapon), items.CreateWeaponFromYAML(weapon)}, false, 1, 1},
		{"one unit of a legendary stack", []items.Item{withQuantity(items.CreateItemFromYAML("skeleton_key"), 2)}, false, 1, 1},
		{"a card pocket fills at the cap", cardStack("rat_card", stash.AlbumCopies+1), true, stash.AlbumCopies, 1},
		{"quest statuette has no showcase place", []items.Item{items.CreateItemFromYAML("black_dragon_statuette")}, false, 0, 1},
		{"a rare valuable has no showcase place", []items.Item{items.CreateItemFromYAML("diamond")}, false, 0, 1},
		{"a card has no showcase place", []items.Item{items.CreateItemFromYAML("rat_card")}, false, 0, 1},
		{"a legendary has no binder pocket", []items.Item{items.CreateWeaponFromYAML(weapon)}, true, 0, 1},
		{"a unique has no showcase place", []items.Item{items.CreateWeaponFromYAML("arena_labrys")}, false, 0, 1},
		{"a class's starting kit has no showcase place", []items.Item{items.CreateWeaponFromYAML("monk_fists")}, false, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := stashTestGame(t)
			for _, it := range tc.bag {
				items.EnsureInstanceID(&it)
				g.party.Inventory = append(g.party.Inventory, it)
			}
			filed := 0
			for range len(tc.bag) + 1 {
				if len(g.party.Inventory) > 0 && g.fileIntoCollection(0, tc.album) {
					filed++
				}
			}
			units := 0
			for _, it := range g.party.Inventory {
				units += it.Count()
			}
			if filed != tc.files || units != tc.bagEnd {
				t.Fatalf("filed %d, bag holds %d units; want %d and %d", filed, units, tc.files, tc.bagEnd)
			}
			stored := 0
			g.stash.Each(func(it *items.Item) { stored += it.Count() })
			if stored != tc.files {
				t.Fatalf("stash holds %d units after %d filings", stored, tc.files)
			}
			if filed == 0 {
				return
			}
			// Taking back returns exactly one unit and empties the slot when it was the last.
			key := collectionBagKey(tc.bag[0], tc.album)
			if !g.takeFromCollection(key, tc.album) {
				t.Fatal("take refused")
			}
			after := 0
			g.stash.Each(func(it *items.Item) { after += it.Count() })
			if after != tc.files-1 {
				t.Fatalf("stash holds %d units after taking one of %d", after, tc.files)
			}
		})
	}
}

func withQuantity(it items.Item, n int) items.Item {
	it.Quantity = n
	return it
}

func cardStack(key string, n int) []items.Item {
	out := make([]items.Item, n)
	for i := range out {
		out[i] = items.CreateItemFromYAML(key)
	}
	return out
}

// Filing and taking back are double-clicks through the displayed screen, in
// both tabs; a single click moves nothing.
func TestCollectionDoubleClickFilesAndTakes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		album   bool
		filed   int // copies in the place before
		bag     int // units in the bag before
		onPlace bool
		clicks  int
		want    [2]int // place copies, bag units after
	}{
		{"showcase files from the bag", false, 0, 1, false, 2, [2]int{1, 0}},
		{"showcase gives it back", false, 1, 0, true, 2, [2]int{0, 1}},
		{"binder sleeves a card", true, 0, 1, false, 2, [2]int{1, 0}},
		{"binder gives one copy back", true, 2, 0, true, 2, [2]int{1, 1}},
		{"one click moves nothing", false, 1, 0, true, 1, [2]int{1, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newDisplayedModalHarness(t, 1024, 768)
			g := h.g
			key := LegendaryCollectionKeys()[0]
			newItem := func() items.Item {
				var it items.Item
				if tc.album {
					it = items.CreateItemFromYAML(key)
				} else {
					it = items.CreateWeaponFromYAML(key)
				}
				items.EnsureInstanceID(&it)
				return it
			}
			tab := collectionTabShowcase
			if tc.album {
				key, tab = CardAlbumKeys()[0], collectionTabBinder
			}
			g.stash = &stash.Stash{Collection: map[string]items.Item{}, Album: map[string][]items.Item{}}
			for range tc.filed {
				if tc.album {
					g.stash.Album[key] = append(g.stash.Album[key], newItem())
				} else {
					g.stash.Collection[key] = newItem()
				}
			}
			g.party.Inventory = nil
			for range tc.bag {
				g.party.Inventory = append(g.party.Inventory, newItem())
			}
			g.collectionScreenOpen = true
			g.setCollectionTab(tab)
			L := computeCollectionLayout(g.config.GetScreenWidth(), gameplayViewportBottom(g), tab)
			x, y := L.bagGrid.x+collectionBagCell/2, L.bagGrid.y+collectionBagCell/2
			if tc.onPlace && tc.album {
				r := L.pocketRect(0)
				x, y = r.x+r.w/2, r.y+r.h/2
			} else if tc.onPlace {
				x, y = L.grid.x+showcaseNiche/2, L.grid.y+showcaseNiche/2
			}
			h.clicks(false, x, y, tc.clicks)
			placed := len(g.stash.Album[key])
			if _, ok := g.stash.Collection[key]; ok && !tc.album {
				placed = 1
			}
			units := 0
			for _, it := range g.party.Inventory {
				units += it.Count()
			}
			if got := [2]int{placed, units}; got != tc.want {
				t.Fatalf("place holds %d, bag %d; want %v", placed, units, tc.want)
			}
		})
	}
}

// A place the catalog no longer lists (an item re-rated, made a class's kit or
// removed) gives its items back to free chest cells when the stash loads, and
// the file keeps the move; a listed place stays, and a full chest leaves the
// entry where it is rather than lose it.
func TestUnlistedCollectionItemsReturnToTheChest(t *testing.T) {
	loadTestConfig(t)
	listed := LegendaryCollectionKeys()[0]
	unit := func(key string) items.Item {
		it := items.CreateItemFromYAML(key)
		items.EnsureInstanceID(&it)
		return it
	}
	for _, tc := range []struct {
		name      string
		fullChest bool
		wantChest int // units back in the chest
		wantKept  bool
	}{
		{"unlisted entries come back", false, 3, false},
		{"a full chest keeps them in place", true, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := stashTestGame(t)
			st := &stash.Stash{
				Collection: map[string]items.Item{"diamond": unit("diamond"), listed: items.CreateWeaponFromYAML(listed)},
				Album:      map[string][]items.Item{"golden_idol": {unit("golden_idol"), unit("golden_idol")}},
			}
			if tc.fullChest {
				for i := range st.Slots {
					st.Slots[i] = unit("health_potion")
				}
			}
			if err := stash.Save(st); err != nil {
				t.Fatal(err)
			}
			g.stash = nil
			if !g.ensureStashLoaded() {
				t.Fatal("stash did not load")
			}
			saved, err := stash.Load()
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range []*stash.Stash{g.stash, saved} {
				back := 0
				for _, it := range s.Slots {
					if it.Name == "Diamond" || it.Name == "Golden Idol" {
						back++
					}
				}
				_, keptDiamond := s.Collection["diamond"]
				_, keptListed := s.Collection[listed]
				if back != tc.wantChest || keptDiamond != tc.wantKept || (len(s.Album["golden_idol"]) > 0) != tc.wantKept || !keptListed {
					t.Fatalf("chest got %d back, diamond kept=%v, idol pocket=%d, listed kept=%v", back, keptDiamond, len(s.Album["golden_idol"]), keptListed)
				}
			}
		})
	}
}
