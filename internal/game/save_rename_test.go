package game

import "testing"

// TestLoadMenuRightClickNeverOpensRename pins the fix for a phantom-modal bug:
// the Load menu never draws the rename dialog, so a right-click there must not
// open it. Before the fix the right-click set saveRenameOpen=true invisibly,
// stranding a modal that only surfaced (and forced a rename) on the next visit
// to the Save menu. Rename is Save-menu-only, matching the R-key gate.
func TestLoadMenuRightClickNeverOpensRename(t *testing.T) {
	const w, h = 800, 600
	px := (w - saveMenuPanelW) / 2
	py := (h - saveMenuPanelH) / 2
	// Centre of the first save row's right-click hitbox.
	rowY := py + saveMenuListTopY + 12

	g := &MMGame{config: loadTestConfig(t), savePage: 0, slotSelection: 0}
	ih := NewInputHandler(g)
	g.mouseRightClicks = []queuedClick{{x: px + saveMenuPanelW/2, y: rowY, at: 1}}

	// Load menu: allowRename=false. The gate must short-circuit before the
	// rename path runs, so no dialog opens (and no save file is touched).
	ih.handleSaveLoadMenuInput(0, 0, w, h, saveMenuPanelW, saveMenuPanelH, false, func() {})

	if g.saveRenameOpen {
		t.Fatal("right-click in the Load menu opened the rename dialog; it must be Save-menu-only")
	}
}

// TestCloseSaveRenameClearsState pins the single-source dismiss used by the
// top-level Escape handler (which cancels the modal before backing out of the
// submenu), the Enter path, and the full input reset: all rename scratch
// fields must clear together, or a stale slot/name leaks into the next open.
func TestCloseSaveRenameClearsState(t *testing.T) {
	g := &MMGame{saveRenameOpen: true, saveRenameSlot: 3, saveRenameInput: "Old Name"}
	g.closeSaveRename()
	if g.saveRenameOpen || g.saveRenameSlot != -1 || g.saveRenameInput != "" {
		t.Fatalf("closeSaveRename left state: open=%v slot=%d input=%q",
			g.saveRenameOpen, g.saveRenameSlot, g.saveRenameInput)
	}
}
