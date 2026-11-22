package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/world"
)

func TestTownPortalRegistersConfiguredDestination(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorldSized(cfg, 8, 8)
	w.StartX, w.StartY = 3, 5
	game := newTestGame(cfg, w)

	previousManager := world.GlobalWorldManager
	world.GlobalWorldManager = &world.WorldManager{
		CurrentMapKey: "japanese_castle",
		MapConfigs: map[string]*config.MapConfig{
			"japanese_castle": {
				Name:                  "Eastern Isle Castle",
				TownPortalDestination: true,
			},
		},
	}
	t.Cleanup(func() { world.GlobalWorldManager = previousManager })

	game.registerVisitedTownPortalDestination()
	got := game.townPortalRows()
	if len(got) != 1 || got[0].id != "japanese_castle" {
		t.Fatalf("destinations = %v, want japanese_castle", got)
	}
	if got[0].place != "Eastern Isle Castle" || got[0].anchor != "" {
		t.Fatalf("destination = %q / %q, want the map name and no anchor", got[0].place, got[0].anchor)
	}

	x, y := w.GetStartingPosition()
	tileSize := float64(cfg.GetTileSize())
	if x != 3.5*tileSize || y != 5.5*tileSize {
		t.Fatalf("start position = (%v, %v), want tile (3, 5) center", x, y)
	}
}

// A map is a Town Portal destination by exactly one source: its own
// town_portal_destination flag (a town without an inn) or an authored anchor
// NPC standing on it. An anchored map that also carries the flag has two
// sources of truth for one contract; registration happens on map switch
// (registerVisitedTownPortalDestination), so flag or anchor is the whole of it.
func TestTownPortalMapFlagOnlyWhereNoAnchor(t *testing.T) {
	cfg := loadTestConfig(t)
	restoreNPCCatalog(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load npcs: %v", err)
	}
	wm, _ := loadRealWorldForTest(t, cfg, "")
	if len(wm.FailedMaps) > 0 {
		t.Fatalf("maps failed to load: %v", wm.FailedMaps)
	}
	g := newTestGame(cfg, wm.GetCurrentWorld())
	anchored, flagged := 0, 0
	for key, mc := range wm.MapConfigs {
		anchor := g.townPortalAnchor(key)
		if anchor != nil {
			anchored++
		}
		if mc.TownPortalDestination {
			flagged++
		}
		if anchor != nil && mc.TownPortalDestination {
			t.Errorf("%s carries the map flag although its %s is a town_portal anchor", key, anchor.Name)
		}
	}
	// Positive controls: both kinds of destination ship.
	if anchored == 0 || flagged == 0 {
		t.Fatalf("destinations: %d anchored, %d flagged; want both kinds", anchored, flagged)
	}
}

// The shipped inn is an anchor: standing on its map registers the map with no
// map flag at all, and the party lands at its door.
func TestTownPortalAnchorNPCMakesItsMapADestination(t *testing.T) {
	cfg := loadTestConfig(t) // spells first: the npcs.yaml backfill validates against it
	restoreNPCCatalog(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load npcs: %v", err)
	}
	inn, err := character.CreateNPCFromConfig("tavern", 0, 0)
	if err != nil {
		t.Fatalf("build tavern: %v", err)
	}
	if !inn.TownPortal {
		t.Fatal("the shipped tavern is not authored as a Town Portal anchor")
	}

	w := newTestWorldSized(cfg, 8, 8)
	w.NPCs = append(w.NPCs, inn)
	game := newTestGame(cfg, w)
	previous := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = previous })
	world.GlobalWorldManager = &world.WorldManager{
		CurrentMapKey: "forest",
		LoadedMaps:    map[string]*world.World3D{"forest": w},                          // the world the inn stands in
		MapConfigs:    map[string]*config.MapConfig{"forest": {Name: "Elvish Forest"}}, // no map flag
	}

	game.registerVisitedTownPortalDestination()
	if got := game.townPortalRows(); len(got) != 1 || got[0].id != "forest" {
		t.Fatalf("a map with an anchor NPC registered as %v", got)
	}

	// The picker names the ANCHOR, not a guessed "Tavern": the flag is generic,
	// so the next anchor may be a shrine or a gate house.
	place, anchor := game.townPortalPlace("forest")
	if anchor != inn.Name {
		t.Fatalf("anchor = %q, want it to name the anchor %q", anchor, inn.Name)
	}
	if strings.Contains(place, "Tavern") {
		t.Fatalf("place = %q invents a tavern", place)
	}
}

// The picker is cast from WHEREVER the party stands - usually a dungeon, which
// is the whole point of the spell. Every row must still name its anchor, so the
// lookup asks the destination's own world and not the loaded one.
func TestTownPortalLabelsAnchorsFromAnotherMap(t *testing.T) {
	cfg := loadTestConfig(t)
	restoreNPCCatalog(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load npcs: %v", err)
	}
	inn, err := character.CreateNPCFromConfig("tavern", 0, 0)
	if err != nil {
		t.Fatalf("build tavern: %v", err)
	}
	forest := newTestWorldSized(cfg, 8, 8)
	forest.NPCs = append(forest.NPCs, inn)
	dungeon := newTestWorldSized(cfg, 8, 8) // no anchor down here

	g := newTestGame(cfg, dungeon) // the party is IN the dungeon
	previous := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = previous })
	world.GlobalWorldManager = &world.WorldManager{
		CurrentMapKey: "pyramid_1",
		LoadedMaps:    map[string]*world.World3D{"forest": forest, "pyramid_1": dungeon},
		MapConfigs:    map[string]*config.MapConfig{"forest": {Name: "Elvish Forest"}},
	}

	if place, anchor := g.townPortalPlace("forest"); place != "Elvish Forest" || anchor != inn.Name {
		t.Fatalf("forest = %q / %q, want the map name and the anchor %q from the dungeon", place, anchor, inn.Name)
	}
	// And the destination the party is standing on keeps its plain name.
	if _, anchor := g.townPortalPlace("pyramid_1"); anchor != "" {
		t.Fatalf("the dungeon row names anchor %q, it has none", anchor)
	}
}

// ARRIVAL is the tavern's door whenever the map has one - a map flag only says
// the map is recallable, it never decides where the party lands.
func TestTownPortalArrivesAtTheTavernNotTheStartTile(t *testing.T) {
	cfg := loadTestConfig(t) // spells first: the npcs.yaml backfill validates against it
	restoreNPCCatalog(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load npcs: %v", err)
	}
	ts := float64(cfg.GetTileSize())
	w := newTestWorldSized(cfg, 12, 12)
	w.StartX, w.StartY = 1, 1 // the '+' tile, deliberately far from the inn
	inn, err := character.CreateNPCFromConfig("tavern", 0, 0)
	if err != nil {
		t.Fatalf("build tavern: %v", err)
	}
	inn.X, inn.Y = TileCenterFromTile(8, 8, ts)
	w.NPCs = append(w.NPCs, inn)

	g := newTestGame(cfg, w)
	previous := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = previous })
	// The map ALSO carries the flag: the tavern must still win the arrival.
	world.GlobalWorldManager = &world.WorldManager{
		CurrentMapKey: "forest",
		LoadedMaps:    map[string]*world.World3D{"forest": w},
		MapConfigs:    map[string]*config.MapConfig{"forest": {Name: "Elvish Forest", TownPortalDestination: true}},
	}

	x, y, ok := g.townPortalArrivalPoint("forest")
	if !ok {
		t.Fatal("no arrival point on a map with an inn")
	}
	if dx, dy := x-inn.X, y-inn.Y; dx*dx+dy*dy > (2*ts)*(2*ts) {
		startX, startY := w.GetStartingPosition()
		t.Fatalf("arrive at (%.0f,%.0f); the inn is at (%.0f,%.0f) and the '+' at (%.0f,%.0f)",
			x, y, inn.X, inn.Y, startX, startY)
	}

	// Take the inn away and the same map lands the party on its '+'.
	w.NPCs = nil
	sx, sy, ok := g.townPortalArrivalPoint("forest")
	wantX, wantY := w.GetStartingPosition()
	if !ok || sx != wantX || sy != wantY {
		t.Fatalf("without an inn the arrival is (%.0f,%.0f), want the '+' (%.0f,%.0f)", sx, sy, wantX, wantY)
	}
}

// Destinations are AUTHORED, one way or the other: an unflagged map whose
// innkeeper carries no town_portal flag is not a destination. The old rule read
// dialogue shape (any NPC renting a room at any depth), which tied travel to how
// a conversation happened to be nested.
func TestTownPortalIgnoresAnUnflaggedMapWithAnInn(t *testing.T) {
	cfg := loadTestConfig(t)
	w := newTestWorldSized(cfg, 8, 8)
	// Rents rooms but is NOT authored as an anchor: renting is a service, being a
	// recall point is a flag.
	w.NPCs = append(w.NPCs, &character.NPC{
		Name: "Innkeeper", RenderCategory: "npc",
		DialogueData: &character.NPCDialogue{Choices: []*character.NPCDialogueChoice{
			{Text: "Rest the night", Action: "tavern_rest", Cost: 25},
		}},
	})
	game := newTestGame(cfg, w)

	previous := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = previous })
	world.GlobalWorldManager = &world.WorldManager{
		CurrentMapKey: "unflagged_inn",
		MapConfigs:    map[string]*config.MapConfig{"unflagged_inn": {Name: "Roadside Inn"}},
	}

	game.registerVisitedTownPortalDestination()
	if got := game.townPortalRows(); len(got) != 0 {
		t.Fatalf("an unflagged map with an inn registered as %v", got)
	}

	// Flag it and the same visit registers.
	world.GlobalWorldManager.MapConfigs["unflagged_inn"].TownPortalDestination = true
	game.registerVisitedTownPortalDestination()
	if got := game.townPortalRows(); len(got) != 1 || got[0].id != "unflagged_inn" {
		t.Fatalf("a flagged map registered as %v", got)
	}
}

// Asked about a map the party is NOT on, the arrival point must still answer
// about that map. Every branch resolves by key; the last one used to read the
// loaded world, which only looked right because the one caller switches maps
// first - any preview or label use would have landed on the wrong '+'.
func TestTownPortalArrivalAnswersAboutTheAskedMap(t *testing.T) {
	t.Chdir("../..")
	g, wm, cfg := bootOpenWorldGame(t, true)
	ts := float64(cfg.GetTileSize())

	// The party stands on the unified world; the question is about a dungeon.
	const dungeon = "pyramid_1"
	dw := wm.LoadedMaps[dungeon]
	if dw == nil {
		t.Fatalf("fixture: %s did not load", dungeon)
	}
	wantX, wantY := dw.GetStartingPosition()
	x, y, ok := g.townPortalArrivalPoint(dungeon)
	if !ok {
		t.Fatal("no arrival point for a loaded dungeon")
	}
	if x != wantX || y != wantY {
		sx, sy := g.world.GetStartingPosition()
		t.Fatalf("arrival for %s = (%.0f,%.0f), want its own '+' (%.0f,%.0f); the loaded world's is (%.0f,%.0f)",
			dungeon, x, y, wantX, wantY, sx, sy)
	}

	// And a region WITH an anchor lands at the anchor, not at the region '+'.
	anchor := g.townPortalAnchor("forest")
	if anchor == nil {
		t.Fatal("fixture: the forest inn is not an anchor any more")
	}
	fx, fy, ok := g.townPortalArrivalPoint("forest")
	if !ok {
		t.Fatal("no arrival point for the forest")
	}
	if dx, dy := fx-anchor.X, fy-anchor.Y; dx*dx+dy*dy > (2*ts)*(2*ts) {
		t.Fatalf("forest arrival (%.0f,%.0f) is not at the inn (%.0f,%.0f)", fx, fy, anchor.X, anchor.Y)
	}
}

// restoreNPCCatalog puts the package-global NPC catalog back after a test loads
// the shipped one. The package convention, and -shuffle=on makes it load-bearing:
// a later test that expects its own fixture catalog - or expects none - would
// otherwise read whatever ran before it.
func restoreNPCCatalog(t *testing.T) {
	t.Helper()
	previous := character.NPCConfigInstance
	t.Cleanup(func() { character.NPCConfigInstance = previous })
}

// A long destination list scrolls, and a click queued against the frame
// before a scroll is dropped rather than landing on whichever row the scroll
// moved under it; without the scroll the same click picks its row.
func TestTownPortalScrollKeepsClicksOnTheirRow(t *testing.T) {
	for _, scrolled := range []bool{false, true} {
		t.Run(fmt.Sprintf("scrolled=%v", scrolled), func(t *testing.T) {
			h := newDisplayedModalHarness(t, 800, 600)
			g := h.g
			g.dialogActive = false
			g.visitedTavernMaps = map[string]bool{}
			for i := 0; i < 14; i++ {
				g.visitedTavernMaps[fmt.Sprintf("town_%02d", i)] = true
			}
			g.openCastPicker(g.party.Members[0], "town_portal")
			l := makeTownPortalLayout(len(g.townPortalRows()), 800, 600)
			if !l.scrolls() {
				t.Fatal("14 destinations do not scroll at 800x600 (fixture)")
			}
			r := l.row(0, 0)
			h.ui.Draw(h.screen)
			if scrolled {
				g.townPortalScroll = l.scroll(townPortalPitch * 3) // a wheel step between frames
			}
			g.mouseLeftClicks = []queuedClick{{x: r.x + r.w/2, y: r.y + r.h/2, at: 1000}}
			h.ui.dispatchDisplayedInput()
			if g.townPortalPickerOpen == !scrolled {
				t.Fatalf("picker open = %v after a click aimed before the scroll=%v", g.townPortalPickerOpen, scrolled)
			}
		})
	}
}
