package game

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/items"
	"ugataima/internal/quests"
	"ugataima/internal/storage"
)

// Case table: active/ready/claimed x no/fixed/pool/mixed rewards x normal/narrow
// card. Check the production renderer and hover path, including wrapped icons.
func TestJournalRewardIconsAndTooltips(t *testing.T) {
	t.Chdir("../..")
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	g, _, _ := bootOpenWorldGame(t, false)
	pointer := installFakePointer(t)
	ui := &UISystem{game: g}
	for _, width := range []int{650, 1120} {
		for _, state := range []string{"active", "ready", "claimed"} {
			for _, kind := range []string{"none", "fixed", "pool", "mixed"} {
				t.Run(fmt.Sprintf("%d/%s/%s", width, state, kind), func(t *testing.T) {
					d := *g.questManager.Definitions()["unbroken_mountain"]
					d.Rewards.Items = nil
					d.Rewards.ItemPool = nil
					if kind == "fixed" || kind == "mixed" {
						d.Rewards.Items = []string{"unbroken_headband", "unbroken_handwraps"}
					}
					if kind == "pool" || kind == "mixed" {
						d.Rewards.ItemPool = []string{"unbroken_sandals", "unbroken_sash", "unbroken_mantle", "unbroken_robe"}
					}
					q := &quests.Quest{ID: "unbroken_mountain", Definition: &d, Status: quests.QuestStatusActive, Completed: state != "active", RewardsClaimed: state == "claimed"}
					c := questCardCopyForQuest(q, width, 3)
					r := layoutRect{10, 10, width, c.height}
					dst := ebiten.NewImage(width+20, c.height+20)
					defer dst.Deallocate()
					keys := append(append([]string(nil), d.Rewards.Items...), d.Rewards.ItemPool...)
					var boxes []layoutRect
					for i, key := range keys {
						icon := questRewardIconRect(r, c, i)
						if icon.x < r.x || icon.right() > r.right()-148 || icon.y < r.y || icon.bottom() > r.bottom()-questCardBottomPad {
							t.Fatalf("icon outside card or overlaps claim button: %+v card %+v", icon, r)
						}
						for _, other := range boxes {
							if icon.x < other.right() && icon.right() > other.x && icon.y < other.bottom() && icon.bottom() > other.y {
								t.Fatal("reward icons overlap")
							}
						}
						boxes = append(boxes, icon)
						pointer.moveTo(icon.x+icon.w/2, icon.y+icon.h/2)
						ui.tooltipLines = nil
						ui.tooltipIcon = ""
						ui.drawJournalEntry(dst, q, r, c)
						item, err := items.TryCreateItemFromYAML(key)
						if err != nil {
							t.Fatal(err)
						}
						if len(ui.tooltipLines) == 0 || !strings.Contains(strings.Join(ui.tooltipLines, "\n"), item.Name) || ui.tooltipIcon != itemTooltipIconName(item) {
							t.Fatalf("missing shared item tooltip for %s: %v", key, ui.tooltipLines)
						}
						if i >= len(d.Rewards.Items) && !strings.Contains(strings.Join(ui.tooltipLines, "\n"), "One random reward") {
							t.Fatal("pool appears guaranteed")
						}
					}
					pointer.moveTo(r.x+5, r.y+5)
					ui.tooltipLines = nil
					ui.tooltipIcon = ""
					ui.drawJournalEntry(dst, q, r, c)
					if ui.tooltipIcon != "" {
						t.Fatal("item tooltip outside icon")
					}
					if len(keys) == 0 && c.height != questCardHeight(len(c.descLines)) {
						t.Fatal("empty rewards reserve icon space")
					}
				})
			}
		}
	}
}
