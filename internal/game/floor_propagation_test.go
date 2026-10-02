package game

import (
	"image/color"
	"slices"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func TestFloorPropagationRendering(t *testing.T) {
	cfg := loadTestConfig(t)
	wm, _ := loadRealWorldForTest(t, cfg, "dragon_cliffs")
	w := wm.GetCurrentWorld()
	g := newTestGame(cfg, w)
	r := NewRenderer(g)
	ui := &UISystem{game: g}
	tm := world.GlobalTileManager
	rock := terrainTile(t, "dragon_cliffs_boulder")
	edge := terrainTile(t, "dragon_cliffs_chasm_edge")
	basalt := terrainTile(t, "dragon_cliffs_basalt_floor")
	void := terrainTile(t, "dragon_cliffs_chasm_floor")
	for _, tc := range []struct {
		name, group string
		source      world.TileType3D
		resolved    bool
	}{
		{"connected across four rocks", "basalt", basalt, true},
		{"edge-only fallback", "default", edge, false},
		{"chasm bottom", "chasm_floor_0", void, true},
		{"other chasm bottom", "chasm_floor_1", terrainTile(t, "dragon_cliffs_chasm_floor_b"), true},
		{"water", "water", terrainTile(t, "water"), true},
		{"deep water", "water", terrainTile(t, "deep_water"), true},
		{"terrain changed back", "basalt", basalt, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for y := 4; y <= 6; y++ {
				for x := 4; x <= 11; x++ {
					w.Tiles[y][x] = edge
				}
			}
			w.Tiles[5][5] = tc.source
			for x := 6; x <= 9; x++ {
				w.Tiles[5][x] = rock
			}
			w.Tiles[5][10] = edge
			r.precomputeFloorColorCache()
			if got := r.floorTextureGroupForTile(9, 5, rock); got != tc.group {
				t.Fatalf("texture group = %s, want %s", got, tc.group)
			}
			if _, ok := w.InheritedFloorAt(9, 5); ok != tc.resolved {
				t.Fatalf("resolved = %v, want %v", ok, tc.resolved)
			}
			if tc.resolved {
				rgb := tm.GetFloorColor(tc.source)
				want := color.RGBA{uint8(rgb[0]), uint8(rgb[1]), uint8(rgb[2]), 255}
				if got := r.floorColorCache[[2]int{9, 5}]; got != want {
					t.Fatalf("3D floor color = %v, want %v", got, want)
				}
				want.A = 235
				if got := ui.compassTileAppearance(9, 5, color.RGBA{}).floor; got != want {
					t.Fatalf("minimap floor color = %v, want %v", got, want)
				}
			}
			if got := r.floorTextureGroupForTile(10, 5, edge); got != "chasm_edge_0" {
				t.Fatal("authored edge lost its own material")
			}
		})
	}
}

// A lectern authored on impassable ground (the Pinnacle's chasm bottom) is a
// Fly-only reward, split and stitched, fresh and after a save restore.
func TestDragonCliffsChasmLecternWithFly(t *testing.T) {
	t.Chdir("../..")
	for _, mode := range []string{"split", "stitched"} {
		t.Run(mode, func(t *testing.T) {
			g, wm, cfg := bootOpenWorldGame(t, mode == "stitched")
			if err := wm.SwitchToMap("dragon_cliffs"); err != nil {
				t.Fatal(err)
			}
			g.world = wm.GetCurrentWorld()
			g.collisionSystem.UpdateTileChecker(g.world)
			tm := world.GlobalTileManager
			mc := wm.MapConfigs["dragon_cliffs"]
			md, err := world.NewMapLoaderWithBiome(cfg, mc.Biome).LoadMap("assets/" + mc.File)
			if err != nil {
				t.Fatal(err)
			}
			var spawn *world.NPCSpawn
			for i, s := range md.NPCSpawns {
				if ground, ok := tm.GetTileTypeFromKey(s.GroundTile); s.NPCKey == "spell_lectern" && ok && !tm.IsWalkable(ground) {
					spawn = &md.NPCSpawns[i]
					break
				}
			}
			if spawn == nil {
				t.Fatal("no dragon_cliffs lectern is authored on impassable ground")
			}
			ground, _ := tm.GetTileTypeFromKey(spawn.GroundTile)
			x, y := wm.ProjectTile("dragon_cliffs", spawn.X, spawn.Y)
			var book *character.NPC
			for _, npc := range g.world.NPCs {
				if npc.Key == spawn.NPCKey && int(npc.X/cfg.GetTileSize()) == x && int(npc.Y/cfg.GetTileSize()) == y {
					book = npc
					break
				}
			}
			if book == nil {
				t.Fatal("authored chasm lectern is missing from the world")
			}
			r := g.gameLoop.renderer
			r.precomputeFloorColorCache()
			assertChasm := func() {
				t.Helper()
				want := tm.GetTileData(ground).FloorTextureGroup
				if got := r.floorTextureGroupForTile(x, y, g.world.Tiles[y][x]); got != want {
					t.Fatalf("lectern stands on %s, want its authored %s ground %s", got, spawn.GroundTile, want)
				}
			}
			assertChasm()
			for _, member := range g.party.Members {
				member.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{}
			}
			g.party.Members[0].MagicSchools[character.MagicSchoolAir] = &character.MagicSkill{}
			// The authored reward must not be collectible from the same nearby
			// position without Fly, either fresh or after restoring a save.
			for _, restored := range []bool{false, true} {
				if restored {
					save := g.buildSave(wm)
					if err := g.applySave(wm, &save); err != nil {
						t.Fatal(err)
					}
				}
				g.camera.X, g.camera.Y, g.camera.Angle = book.X-cfg.GetTileSize(), book.Y, 0
				g.resetCameraPresentation()
				g.updateFocusedNPC()
				if g.focusedNPC != book {
					t.Fatal("visible chasm lectern must accept an attempt without Fly")
				}
				NewInputHandler(g).openNPCInteraction(book)
				if book.Visited {
					t.Fatal("chasm lectern consumed without Fly")
				}
			}
			g.flyActive = true
			g.flyDuration = 600
			g.world.SetTerrainPassageActive(true)
			if !g.world.IsTileBlockingTerrainAt(x, y) || g.world.IsTileBlocking(x, y) {
				t.Fatal("chasm must block ordinary walking but allow Fly")
			}
			g.camera.X, g.camera.Y, g.camera.Angle = book.X-cfg.GetTileSize(), book.Y, 0
			if g.world.IsTileBlocking(x-1, y) {
				t.Fatal("Fly cannot approach the chasm lectern")
			}
			// Save and load the flying party before using the book. Its derived
			// ground must stay chasm, including saves without floor-cache fields.
			save := g.buildSave(wm)
			if err := g.applySave(wm, &save); err != nil {
				t.Fatal(err)
			}
			assertChasm()
			if !g.flyActive {
				t.Fatal("save load dropped Fly at the outdoor lectern")
			}
			for _, member := range g.party.Members {
				member.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{}
			}
			reader := g.party.Members[0]
			reader.MagicSchools[character.MagicSchoolAir] = &character.MagicSkill{}
			g.updateFocusedNPC()
			if g.focusedNPC != book {
				t.Fatal("the flying party cannot focus the nearby book")
			}
			g.flyActive = false
			NewInputHandler(g).tryFocusedNPCInteraction()
			if book.Visited {
				t.Fatal("cached focus consumed the book after Fly expired")
			}
			g.flyActive = true
			if !NewInputHandler(g).tryFocusedNPCInteraction() || !book.Visited {
				t.Fatal("normal interaction did not consume the book while flying")
			}
			learned := false
			for _, id := range book.Lectern.Pool {
				learned = learned || reader.KnowsSpell(spells.SpellID(id))
			}
			if !learned {
				t.Fatal("the flying reader learned no spell")
			}
		})
	}
}

func TestFloorPropagationSaveRestore(t *testing.T) {
	t.Chdir("../..")
	g, wm, _ := bootOpenWorldGame(t, true)
	r := g.gameLoop.renderer
	tm := world.GlobalTileManager
	// The saved terrain patch supplies a basalt source four objects away from
	// the target. Save restoration must rebuild the derived floor after replay.
	var changes []TerrainChange
	for y := 4; y <= 6; y++ {
		for x := 4; x <= 11; x++ {
			wx, wy := wm.ProjectTile("dragon_cliffs", x, y)
			after := "dragon_cliffs_chasm_edge"
			if y == 5 {
				switch {
				case x == 5:
					after = "dragon_cliffs_basalt_floor"
				case x >= 6 && x <= 9:
					after = "dragon_cliffs_boulder"
				case x == 10:
					after = "dragon_cliffs_chasm_edge"
				}
			}
			changes = append(changes, TerrainChange{Map: "dragon_cliffs", X: x, Y: y,
				Before: tm.GetTileKey(g.world.Tiles[wy][wx]), After: after})
		}
	}
	g.restoreTerrainChanges(changes)
	save := g.buildSave(wm)
	x, y := wm.ProjectTile("dragon_cliffs", 9, 5)
	for _, entry := range []string{"saved terrain", "reloaded terrain"} {
		t.Run(entry, func(t *testing.T) {
			if entry != "saved terrain" {
				// Derived ground has no save fields: both existing and new saves
				// enter through the same authored-map + terrain-replay path.
				if err := g.applySave(wm, &save); err != nil {
					t.Fatal(err)
				}
			}
			if got := r.floorTextureGroupForTile(x, y, g.world.Tiles[y][x]); got != "basalt" {
				t.Fatalf("%s lost propagated material: %s", entry, got)
			}
		})
	}
}

// The ground left under a removed travel device is derived at stitch time, so a
// save round-trip must rebuild exactly the same cell.
func TestRemovedDeviceGroundSurvivesSaveRestore(t *testing.T) {
	t.Chdir("../..")
	g, wm, cfg := bootOpenWorldGame(t, true)
	tm := world.GlobalTileManager
	r := g.gameLoop.renderer
	type gap struct {
		name, ground, group string
		x, y                int
	}
	owc, err := config.LoadOpenWorldConfig("assets/open_world.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var gaps []gap
	r.precomputeFloorColorCache()
	for region, removal := range owc.Removals {
		mc := wm.MapConfigs[region]
		md, err := world.NewMapLoaderWithBiome(cfg, mc.Biome).LoadMap("assets/" + mc.File)
		if err != nil {
			t.Fatal(err)
		}
		for _, spawn := range md.NPCSpawns {
			if !slices.Contains(removal.NPCs, spawn.NPCKey) {
				continue
			}
			x, y := wm.ProjectTile(region, spawn.X, spawn.Y)
			tile := wm.OpenWorld.Tiles[y][x]
			gaps = append(gaps, gap{region + "/" + spawn.NPCKey, tm.GetTileKey(tile), r.floorTextureGroupForTile(x, y, tile), x, y})
		}
	}
	if len(gaps) == 0 {
		t.Fatal("no removed travel device on any stitched region")
	}
	save := g.buildSave(wm)
	if err := g.applySave(wm, &save); err != nil {
		t.Fatal(err)
	}
	r.precomputeFloorColorCache()
	for _, want := range gaps {
		tile := wm.OpenWorld.Tiles[want.y][want.x]
		if got := tm.GetTileKey(tile); got != want.ground {
			t.Errorf("%s: ground after load = %q, want %q", want.name, got, want.ground)
		}
		if got := r.floorTextureGroupForTile(want.x, want.y, tile); got != want.group {
			t.Errorf("%s: texture after load = %q, want %q", want.name, got, want.group)
		}
	}
}
