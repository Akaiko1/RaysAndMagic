package main

import (
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/boot"
	"ugataima/internal/world"
)

// dragTestViewer boots the real configs (the drag path resolves tiles through
// the tile manager, exactly like the brush) and builds a small map to move
// things around on.
func dragTestViewer(t *testing.T) (*viewer, *mapInfo) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(filepath.Join("..", "..")); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	cfg, _ := boot.LoadGameData()
	maps, err := loadMaps(cfg)
	if err != nil || len(maps) == 0 {
		t.Fatalf("load maps: %v", err)
	}
	v := &viewer{cfg: cfg, tileManager: world.GlobalTileManager}

	floor, ok := world.GlobalTileManager.GetTileTypeFromLetterForBiome(".", "")
	if !ok {
		t.Fatal("no floor tile for the empty biome")
	}
	tree, ok := world.GlobalTileManager.GetTileTypeFromKey("tree")
	if !ok {
		t.Fatal("no tree tile")
	}
	tiles := make([][]world.TileType3D, 6)
	for y := range tiles {
		tiles[y] = make([]world.TileType3D, 6)
		for x := range tiles[y] {
			tiles[y][x] = floor
		}
	}
	tiles[1][1] = tree
	m := &mapInfo{
		Key: "dragtest",
		Data: &world.MapData{
			Tiles:         tiles,
			StartX:        0,
			StartY:        0,
			MonsterSpawns: []world.MonsterSpawn{{X: 2, Y: 2, MonsterKey: "orc"}},
			NPCSpawns:     []world.NPCSpawn{{X: 3, Y: 3, NPCKey: "card_collector"}},
		},
	}
	v.maps = []mapInfo{*m}
	return v, &v.maps[0]
}

// A press on bare ground grabs nothing: that gesture belongs to the brush, and
// stealing it would make painting impossible.
func TestGrabAt_OnlyPicksUpContent(t *testing.T) {
	_, m := dragTestViewer(t)
	for _, tc := range []struct {
		name string
		x, y int
		want dragKind
	}{
		{"bare floor", 4, 4, dragNone},
		{"terrain prop", 1, 1, dragTile},
		{"monster spawn", 2, 2, dragMonster},
		{"npc spawn", 3, 3, dragNPC},
	} {
		if got := grabAt(m, tc.x, tc.y); got.kind != tc.want {
			t.Errorf("%s: grab kind = %v, want %v", tc.name, got.kind, tc.want)
		}
	}
}

func TestDropAt_MovesEachKindAndClearsTheSource(t *testing.T) {
	v, m := dragTestViewer(t)

	// Monster: the spawn moves and its old cell holds nothing.
	if !v.dropAt(m, grabAt(m, 2, 2), 5, 5, false) {
		t.Fatal("monster drop was refused")
	}
	if len(m.Data.MonsterSpawns) != 1 || m.Data.MonsterSpawns[0].X != 5 || m.Data.MonsterSpawns[0].Y != 5 {
		t.Fatalf("monster spawns after drop: %+v", m.Data.MonsterSpawns)
	}
	if grabAt(m, 2, 2).active() {
		t.Error("the monster's source cell still holds something")
	}
	if m.Data.MonsterSpawns[0].MonsterKey != "orc" {
		t.Errorf("monster key changed to %q", m.Data.MonsterSpawns[0].MonsterKey)
	}

	// NPC.
	if !v.dropAt(m, grabAt(m, 3, 3), 0, 5, false) {
		t.Fatal("npc drop was refused")
	}
	if m.Data.NPCSpawns[0].X != 0 || m.Data.NPCSpawns[0].Y != 5 || m.Data.NPCSpawns[0].NPCKey != "card_collector" {
		t.Fatalf("npc spawns after drop: %+v", m.Data.NPCSpawns)
	}

	// Terrain prop: the tile travels and the source becomes plain floor.
	tree := m.Data.Tiles[1][1]
	if !v.dropAt(m, grabAt(m, 1, 1), 4, 1, false) {
		t.Fatal("tile drop was refused")
	}
	if m.Data.Tiles[1][4] != tree {
		t.Error("the prop tile did not land on the target cell")
	}
	if !isFloorTile(m.Data.Tiles[1][1]) {
		t.Error("the prop's source cell is not floor after the move")
	}
}

// Dropping where it was picked up must change nothing - that is a plain click on
// an object, not an edit.
func TestDropAt_SameCellIsANoOp(t *testing.T) {
	v, m := dragTestViewer(t)
	before := len(m.Data.MonsterSpawns)
	if v.dropAt(m, grabAt(m, 2, 2), 2, 2, false) {
		t.Error("dropping on the source cell reported a change")
	}
	if len(m.Data.MonsterSpawns) != before {
		t.Errorf("monster count changed from %d to %d", before, len(m.Data.MonsterSpawns))
	}
	if !grabAt(m, 2, 2).active() {
		t.Error("the monster vanished on a no-op drop")
	}
}

// A drop onto an occupied cell replaces what was there (the brush rule), and the
// moved object must survive that sweep - it is cleared from its own cell first.
func TestDropAt_ReplacesTheTargetAndKeepsTheMovedObject(t *testing.T) {
	v, m := dragTestViewer(t)
	m.Data.MonsterSpawns = append(m.Data.MonsterSpawns, world.MonsterSpawn{X: 4, Y: 4, MonsterKey: "goblin"})

	if !v.dropAt(m, grabAt(m, 2, 2), 4, 4, false) {
		t.Fatal("drop onto an occupied cell was refused")
	}
	if len(m.Data.MonsterSpawns) != 1 {
		t.Fatalf("expected one monster left, got %+v", m.Data.MonsterSpawns)
	}
	if got := m.Data.MonsterSpawns[0]; got.MonsterKey != "orc" || got.X != 4 || got.Y != 4 {
		t.Errorf("survivor = %+v, want the dragged orc at (4,4)", got)
	}
}

// Out-of-bounds drops are refused instead of writing outside the grid.
func TestDropAt_RefusesOffMap(t *testing.T) {
	v, m := dragTestViewer(t)
	for _, p := range [][2]int{{-1, 2}, {2, -1}, {6, 2}, {2, 6}} {
		if v.dropAt(m, grabAt(m, 2, 2), p[0], p[1], false) {
			t.Errorf("drop at %v was accepted", p)
		}
	}
	if !grabAt(m, 2, 2).active() {
		t.Error("a refused drop lost the object")
	}
}

// Shift at release DUPLICATES: the source keeps its object and a second one
// lands on the target, with the same authored key.
func TestDropAt_ShiftCopiesInsteadOfMoving(t *testing.T) {
	v, m := dragTestViewer(t)

	if !v.dropAt(m, grabAt(m, 2, 2), 5, 5, true) {
		t.Fatal("copy drop was refused")
	}
	if len(m.Data.MonsterSpawns) != 2 {
		t.Fatalf("monster spawns after copy: %+v", m.Data.MonsterSpawns)
	}
	src, dst := grabAt(m, 2, 2), grabAt(m, 5, 5)
	if src.kind != dragMonster || dst.kind != dragMonster {
		t.Fatalf("copy did not leave a monster on both cells: src=%v dst=%v", src.kind, dst.kind)
	}
	if src.monster.MonsterKey != "orc" || dst.monster.MonsterKey != "orc" {
		t.Errorf("keys after copy: src=%q dst=%q", src.monster.MonsterKey, dst.monster.MonsterKey)
	}

	// A copied prop leaves the original tile standing.
	tree := m.Data.Tiles[1][1]
	if !v.dropAt(m, grabAt(m, 1, 1), 4, 1, true) {
		t.Fatal("tile copy was refused")
	}
	if m.Data.Tiles[1][1] != tree || m.Data.Tiles[1][4] != tree {
		t.Error("a copied prop must exist on both cells")
	}

	// Copying onto the source cell would stack two spawns on one tile.
	before := len(m.Data.MonsterSpawns)
	if v.dropAt(m, grabAt(m, 2, 2), 2, 2, true) {
		t.Error("copy onto the source cell reported a change")
	}
	if len(m.Data.MonsterSpawns) != before {
		t.Errorf("same-cell copy duplicated the spawn: %d -> %d", before, len(m.Data.MonsterSpawns))
	}
}

// The gesture split the editor lives by: click = brush (so the eraser works on
// an occupied cell), hold + move = drag.
func TestDragShouldPromote_OnlyAfterLeavingTheCell(t *testing.T) {
	armed := dragState{kind: dragMonster, fromX: 3, fromY: 4}
	for _, tc := range []struct {
		name    string
		pending dragState
		x, y    int
		over    bool
		want    bool
	}{
		{"still on the press cell", armed, 3, 4, true, false},
		{"moved one cell right", armed, 4, 4, true, true},
		{"moved one cell up", armed, 3, 3, true, true},
		{"cursor left the map", armed, 9, 9, false, false},
		{"nothing armed", dragState{}, 4, 4, true, false},
	} {
		if got := dragShouldPromote(tc.pending, tc.x, tc.y, tc.over); got != tc.want {
			t.Errorf("%s: promote = %v, want %v", tc.name, got, tc.want)
		}
	}
}
