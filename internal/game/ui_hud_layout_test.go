package game

import (
	"fmt"
	"testing"

	"ugataima/internal/items"
)

func TestHUDCombatMessagesAvoidVisibleQuickBarAtStandardResolutions(t *testing.T) {
	resolutions := []struct {
		name string
		w, h int
	}{
		{"default-4x3", 1024, 768},
		{"hd-16x9", 1280, 720},
		{"wxga-16x10", 1280, 800},
		{"laptop-16x9", 1366, 768},
		{"desktop-16x10", 1440, 900},
		{"wide-hd-16x9", 1600, 900},
		{"wsxga-16x10", 1680, 1050},
		{"full-hd-16x9", 1920, 1080},
		{"wuxga-16x10", 1920, 1200},
		{"qhd-16x9", 2560, 1440},
	}

	for _, res := range resolutions {
		t.Run(fmt.Sprintf("%s-%dx%d", res.name, res.w, res.h), func(t *testing.T) {
			g, selected := newThiefTestGame(t)
			g.config.Display.ScreenWidth = res.w
			g.config.Display.ScreenHeight = res.h
			selected.QuickSlots[0] = &items.Item{Name: "Potion"}
			g.maxMessages = 4
			for i := 0; i < g.maxMessages; i++ {
				g.AddCombatMessage(fmt.Sprintf("combat message %d", i+1))
			}

			quickBar, visible := inGameQuickSlotBarLayout(g)
			if !visible {
				t.Fatal("quick bar should be visible with an occupied selected-character slot")
			}
			lines := g.hudMessageLines()
			x, y, w, h := g.hudMessageBlockRect(len(lines))
			if x < quickBar.right() && quickBar.x < x+w && y < quickBar.bottom() && quickBar.y < y+h {
				t.Fatalf("chat (%d,%d %dx%d) overlaps quick bar (%d,%d %dx%d)",
					x, y, w, h, quickBar.x, quickBar.y, quickBar.w, quickBar.h)
			}
			if y < 0 || y+h > res.h {
				t.Fatalf("chat (%d,%d %dx%d) leaves %dx%d HUD viewport", x, y, w, h, res.w, res.h)
			}
		})
	}
}
