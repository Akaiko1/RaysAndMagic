package game

import (
	"fmt"
	"strings"
	"testing"
)

// Four readable entries fit each supported viewport, even with long copy.
// Both short and clipped descriptions retain their full text. Persistence N/A.
func TestQuestJournalFourEntriesPerPage(t *testing.T) {
	for _, physical := range [][2]int{{800, 680}, {1024, 768}, {1280, 720}, {1920, 1080}, {3440, 1440}, {3840, 2160}} {
		for _, description := range []string{"A short objective.", strings.Repeat("A long objective with directions and a named enemy. ", 40)} {
			t.Run(fmt.Sprintf("%v/%d", physical, len(description)), func(t *testing.T) {
				w, h := logicalScreenSize(physical[0], physical[1])
				content := computeTabbedMenuLayout(w, gameplayViewportBottomWithPartyHUD(h)).content
				layout := computeQuestContentLayout(content, nil, 0)
				copies := make([]questCardCopy, 12)
				for i := range copies {
					copies[i] = questCardCopyFor(description, layout.cardW, layout.maxDescRows)
				}
				layout = computeQuestContentLayout(content, copies, 0)
				if len(layout.rows) < 4 {
					t.Fatalf("only %d entries fit", len(layout.rows))
				}
				seen := 0
				for page := 0; page < layout.totalPages; page++ {
					l := computeQuestContentLayout(content, copies, page)
					if l.pageStart != seen {
						t.Fatal("pagination skipped or duplicated an entry")
					}
					for i, r := range l.rows {
						if r.bottom() > l.pager.y || (i > 0 && r.y < l.rows[i-1].bottom()) {
							t.Fatal("entry overlaps the next entry or pager")
						}
					}
					seen += len(l.rows)
				}
				if seen != len(copies) {
					t.Fatal("lost entries")
				}
			})
		}
	}
}
