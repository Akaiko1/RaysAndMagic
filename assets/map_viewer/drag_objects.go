package main

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"

	"ugataima/internal/world"
)

// Drag-to-move for map content: hold the left button on a monster, NPC, special
// tile or a non-floor terrain tile and drop it on another cell. Painting is
// untouched - a press on EMPTY ground still paints with the current brush, so
// the two gestures never compete for the same cell.

// dragKind is what was picked up, in the order grabAt probes a cell.
type dragKind int

const (
	dragNone dragKind = iota
	dragMonster
	dragNPC
	dragSpecial
	dragTile
)

// dragState is the object in flight. Kept as a VALUE copy of the spawn, so a
// cancelled drag needs no undo bookkeeping - the source is only cleared when
// the drop is committed.
type dragState struct {
	kind    dragKind
	fromX   int
	fromY   int
	label   string
	monster world.MonsterSpawn
	npc     world.NPCSpawn
	special world.SpecialTileSpawn
	tile    world.TileType3D
}

func (d dragState) active() bool { return d.kind != dragNone }

// grabAt reports what a press on (tx, ty) would pick up. Entities win over
// terrain: they sit ON a floor tile, so grabbing the tile under an NPC would
// leave the NPC floating on nothing.
func grabAt(m *mapInfo, tx, ty int) dragState {
	if m == nil || m.Data == nil {
		return dragState{}
	}
	for _, s := range m.Data.MonsterSpawns {
		if s.X == tx && s.Y == ty {
			return dragState{kind: dragMonster, fromX: tx, fromY: ty, label: s.MonsterKey, monster: s}
		}
	}
	for _, s := range m.Data.NPCSpawns {
		if s.X == tx && s.Y == ty {
			return dragState{kind: dragNPC, fromX: tx, fromY: ty, label: s.NPCKey, npc: s}
		}
	}
	for _, s := range m.Data.SpecialTileSpawns {
		if s.X == tx && s.Y == ty {
			return dragState{kind: dragSpecial, fromX: tx, fromY: ty, label: s.TileKey, special: s}
		}
	}
	if ty < 0 || ty >= len(m.Data.Tiles) || tx < 0 || tx >= len(m.Data.Tiles[ty]) {
		return dragState{}
	}
	tile := m.Data.Tiles[ty][tx]
	if isFloorTile(tile) {
		return dragState{} // bare ground: the press belongs to the brush
	}
	return dragState{kind: dragTile, fromX: tx, fromY: ty, label: tileLabel(tile), tile: tile}
}

// dropAt commits the drag onto (tx, ty) and reports whether the map changed.
// The target cell is cleared of whatever else lives there first, so a drop can
// replace - the same rule the brush follows. With copy set (Shift held at
// release) the source is left in place and the drop is a duplicate.
func (v *viewer) dropAt(m *mapInfo, d dragState, tx, ty int, copy bool) bool {
	if m == nil || m.Data == nil || !d.active() {
		return false
	}
	if ty < 0 || ty >= len(m.Data.Tiles) || tx < 0 || tx >= len(m.Data.Tiles[ty]) {
		return false
	}
	if tx == d.fromX && ty == d.fromY {
		return false
	}

	// Clear the source, then the destination. Order matters when a drag lands on
	// a cell that holds another entity: clearing the source first keeps the
	// moved object out of the removal sweep.
	if !copy {
		switch d.kind {
		case dragMonster:
			m.Data.MonsterSpawns = removeMonsterAt(m.Data.MonsterSpawns, d.fromX, d.fromY)
		case dragNPC:
			m.Data.NPCSpawns = removeNPCAt(m.Data.NPCSpawns, d.fromX, d.fromY)
		case dragSpecial:
			m.Data.SpecialTileSpawns = removeSpecialAt(m.Data.SpecialTileSpawns, d.fromX, d.fromY)
		case dragTile:
			v.setTile(m, d.fromX, d.fromY, floorLetter)
		}
	}

	clearMapCellSpawns(m, tx, ty)

	switch d.kind {
	case dragMonster:
		s := d.monster
		s.X, s.Y = tx, ty
		m.Data.MonsterSpawns = append(m.Data.MonsterSpawns, s)
		v.setTile(m, tx, ty, floorLetter) // an entity stands on clear ground
	case dragNPC:
		s := d.npc
		s.X, s.Y = tx, ty
		m.Data.NPCSpawns = append(m.Data.NPCSpawns, s)
		v.setTile(m, tx, ty, floorLetter)
	case dragSpecial:
		s := d.special
		s.X, s.Y = tx, ty
		m.Data.SpecialTileSpawns = append(m.Data.SpecialTileSpawns, s)
		v.setTile(m, tx, ty, floorLetter)
	case dragTile:
		m.Data.Tiles[ty][tx] = d.tile
	}
	return true
}

// drawDragGhost marks the cell the grabbed object would land on and prints what
// is in flight, so a drag is never a blind gesture.
func (v *viewer) drawDragGhost(screen *ebiten.Image, lay layout) {
	if !v.grab.active() {
		return
	}
	mx, my := ebiten.CursorPosition()
	tx, ty, over := hoveredMapTile(lay, mx, my)
	copying := dragCopyHeld()
	fill := color.RGBA{110, 200, 255, 70}
	edge := color.RGBA{150, 220, 255, 255}
	if copying { // green reads as "adding a second one", blue as "moving this one"
		fill = color.RGBA{110, 235, 130, 70}
		edge = color.RGBA{150, 255, 170, 255}
	}
	if over {
		x := lay.originX + tx*lay.tileSize
		y := lay.originY + ty*lay.tileSize
		drawFilledRect(screen, x, y, lay.tileSize, lay.tileSize, fill)
		drawRectBorder(screen, x, y, lay.tileSize, lay.tileSize, 1, edge)
	}
	// Source cell stays outlined until the drop commits.
	fx := lay.originX + v.grab.fromX*lay.tileSize
	fy := lay.originY + v.grab.fromY*lay.tileSize
	drawRectBorder(screen, fx, fy, lay.tileSize, lay.tileSize, 1, color.RGBA{255, 200, 90, 255})
	ebitenutil.DebugPrintAt(screen, dragStatus(v.grab, tx, ty, over, copying), lay.mapAreaX+8, lay.mapAreaY+8)
}

// dragShouldPromote decides the gesture: an armed press becomes a real DRAG
// only once the cursor leaves the cell it started on. A single click therefore
// stays with the brush (that is how the eraser keeps working on an occupied
// cell), and hold-and-move is a move/copy.
func dragShouldPromote(pending dragState, tx, ty int, overMap bool) bool {
	if !pending.active() || !overMap {
		return false
	}
	return tx != pending.fromX || ty != pending.fromY
}

// dragCopyHeld reports whether the drop would DUPLICATE instead of move.
// Read at release, not at grab, so the choice can still be made mid-drag.
func dragCopyHeld() bool { return shiftHeld() }

// dragStatus is the line shown while an object is in flight.
func dragStatus(d dragState, tx, ty int, over, copying bool) string {
	if !d.active() {
		return ""
	}
	what := map[dragKind]string{dragMonster: "monster", dragNPC: "NPC", dragSpecial: "special", dragTile: "tile"}[d.kind]
	verb := "moving"
	if copying {
		verb = "copying"
	}
	if !over {
		return fmt.Sprintf("%s %s %s - release over the map (Shift copies, Esc cancels)", verb, what, d.label)
	}
	return fmt.Sprintf("%s %s %s -> (%d,%d) (Shift copies, Esc cancels)", verb, what, d.label, tx, ty)
}
