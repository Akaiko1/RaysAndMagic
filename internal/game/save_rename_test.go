package game

import "testing"

// TestCloseSaveRenameClearsState pins the single-source dismiss used by the
// top-level Escape handler (which cancels the modal before backing out of the
// submenu), the Enter path, and the full input reset: all rename scratch
// fields must clear together, or a stale slot/name leaks into the next open.
func TestCloseSaveRenameClearsState(t *testing.T) {
	g := &MMGame{
		menuState: menuState{
			saveRenameOpen:  true,
			saveRenameSlot:  3,
			saveRenameInput: "Old Name",
		},
	}
	g.closeSaveRename()
	if g.saveRenameOpen || g.saveRenameSlot != -1 || g.saveRenameInput != "" {
		t.Fatalf("closeSaveRename left state: open=%v slot=%d input=%q",
			g.saveRenameOpen, g.saveRenameSlot, g.saveRenameInput)
	}
}
