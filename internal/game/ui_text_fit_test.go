package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/quests"
)

// Every merchant price form must FIT the box it is drawn in, and the box must
// stop short of the next column. Measured, not eyeballed: a compound price
// ("scale + gold") used to overrun its cell and collide with the neighbour.
func TestMerchantPriceLabelsFitTheirBox(t *testing.T) {
	pitch := merchantIconSize + merchantIconGapX
	if merchantPriceBoxW >= pitch {
		t.Fatalf("price box %d >= column pitch %d - neighbouring prices would touch", merchantPriceBoxW, pitch)
	}
	// The box is centred on the icon, so it may not reach either neighbour.
	x, _, w, _ := merchantPriceRect(0, 0, merchantIconSize, merchantIconSize)
	if x < -(pitch-merchantIconSize)/2-1 || x+w > merchantIconSize+(pitch-merchantIconSize)/2+1 {
		t.Fatalf("price box [%d,%d) escapes its cell gap (icon %d, pitch %d)", x, x+w, merchantIconSize, pitch)
	}

	for _, label := range []string{
		"20000 g",                         // plain gold, biggest realistic buy price
		"5000 ap",                         // arena points
		"x3",                              // item currency alone
		"x3 +20000",                       // compound BEFORE compaction - must be clamped
		"x3 +" + compactCoinAmount(20000), // the shipped Scalewright form
		"sold out",
		"no value",
	} {
		fitted := merchantPriceLabel(label)
		if got := debugTextWidth(fitted); got > merchantPriceBoxW {
			t.Errorf("price %q renders %dpx wide, box is %dpx", fitted, got, merchantPriceBoxW)
		}
	}

	// The shipped compound form must fit WITHOUT being clipped: a "20k" that
	// arrives as "20.." tells the player nothing.
	shipped := fmt.Sprintf("x%d +%s", 3, compactCoinAmount(20000))
	if merchantPriceLabel(shipped) != shipped {
		t.Fatalf("shipped compound price %q had to be clipped to %q", shipped, merchantPriceLabel(shipped))
	}
	if got, want := compactCoinAmount(20000), "20k"; got != want {
		t.Fatalf("compactCoinAmount(20000) = %q, want %q", got, want)
	}
	if got, want := compactCoinAmount(9999), "9999"; got != want {
		t.Fatalf("compactCoinAmount keeps small amounts exact: got %q, want %q", got, want)
	}
}

// Every shipped merchant's stock must fit the price box with its real currency
// wiring - the catalog is the thing that actually ships, so measure IT.
func TestShippedMerchantStockPricesFit(t *testing.T) {
	loadTestConfig(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load npcs: %v", err)
	}
	checked := 0
	for key, npc := range character.NPCConfigInstance.NPCs {
		for _, entry := range npc.Inventory {
			if entry == nil {
				continue
			}
			label := "0 g"
			switch {
			case entry.CurrencyItem != "" && entry.GoldCost > 0:
				label = fmt.Sprintf("x%d +%s", entry.Cost, compactCoinAmount(entry.GoldCost))
			case entry.CurrencyItem != "":
				label = fmt.Sprintf("x%d", entry.Cost)
			case npc.Currency == character.CurrencyArenaPoints:
				label = fmt.Sprintf("%d ap", entry.Cost)
			default:
				label = fmt.Sprintf("%d g", entry.Cost)
			}
			checked++
			if w := debugTextWidth(label); w > merchantPriceBoxW {
				t.Errorf("NPC %q sells %q at %q: %dpx wide, box is %dpx",
					key, entry.Name, label, w, merchantPriceBoxW)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no merchant stock measured - the catalog did not load")
	}
}

// A quest card is sized from its OWN copy, and pages pack by height: wordy
// quests give fewer cards per page, terse ones more. Cards must never overlap
// or leave the list area.
func TestQuestCardsSizeToCopyAndPackByHeight(t *testing.T) {
	content := layoutRect{0, 0, tabbedMenuPanelW, 620}
	listTop, avail, pager := questCardListAvailable(content)
	maxRows := questCardMaxDescRowsFor(avail)

	short := questCardCopyFor("Short.", questCardW, maxRows)
	long := questCardCopyFor(strings.Repeat("A wordy objective that wraps. ", 30), questCardW, maxRows)
	if long.height <= short.height {
		t.Fatalf("long card (%d) must be taller than short card (%d)", long.height, short.height)
	}
	if !long.descClipped() && len(long.descLines) < maxRows {
		t.Fatalf("long copy neither filled the cap nor reported clipping: %d rows", len(long.descLines))
	}

	// A page of short cards must hold strictly more entries than a page of long
	// ones - that IS the "smart pagination" contract.
	manyShort := make([]questCardCopy, 12)
	manyLong := make([]questCardCopy, 12)
	for i := range manyShort {
		manyShort[i], manyLong[i] = short, long
	}
	shortPage := computeQuestContentLayout(content, manyShort, 0)
	longPage := computeQuestContentLayout(content, manyLong, 0)
	if len(shortPage.rows) <= len(longPage.rows) {
		t.Fatalf("short cards per page %d, long %d - pagination is not height-aware",
			len(shortPage.rows), len(longPage.rows))
	}

	// Geometry: no overlap, nothing past the pager.
	for _, layout := range []questContentLayout{shortPage, longPage} {
		for i, row := range layout.rows {
			if row.y < listTop {
				t.Errorf("card %d starts above the list top (%d < %d)", i, row.y, listTop)
			}
			if row.bottom() > pager.y {
				t.Errorf("card %d bottom %d runs into the pager at %d", i, row.bottom(), pager.y)
			}
			if i > 0 && row.y < layout.rows[i-1].bottom() {
				t.Errorf("card %d overlaps card %d", i, i-1)
			}
		}
	}

	// Paging covers every card exactly once, with no gaps.
	seen := 0
	for page := 0; page < longPage.totalPages; page++ {
		l := computeQuestContentLayout(content, manyLong, page)
		if l.pageStart != seen {
			t.Fatalf("page %d starts at %d, expected %d", page, l.pageStart, seen)
		}
		seen += len(l.rows)
	}
	if seen != len(manyLong) {
		t.Fatalf("pages covered %d of %d cards", seen, len(manyLong))
	}
}

// The clipped-copy tooltip is the promise that nothing authored is lost.
// Measured against the SHIPPED greetings: a merchant greeting that overflows
// its two-line box must be detectable as clipped, with the whole text intact.
func TestShippedGreetingsThatOverflowStayRecoverable(t *testing.T) {
	loadTestConfig(t)
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load npcs: %v", err)
	}
	greetingW := min(npcDialogWidth-40, tabGreetingWrapColumns*debugTextCharWidth)

	overflowing := 0
	for key, npc := range character.NPCConfigInstance.NPCs {
		if npc.Dialogue == nil || npc.Dialogue.Greeting == "" {
			continue
		}
		full := wrapDebugText(npc.Dialogue.Greeting, greetingW)
		shown := truncateWrappedLines(full, 2, greetingW)
		for _, line := range shown {
			if w := debugTextWidth(line); w > greetingW {
				t.Errorf("NPC %q greeting line %q is %dpx wide, box is %dpx", key, line, w, greetingW)
			}
		}
		if len(full) > len(shown) {
			overflowing++
			// The clipped view keeps its ellipsis cue and the full copy its tail.
			if !strings.HasSuffix(shown[len(shown)-1], "...") {
				t.Errorf("NPC %q greeting is clipped without an ellipsis cue: %q", key, shown[len(shown)-1])
			}
			if strings.Join(full, " ") == strings.Join(shown, " ") {
				t.Errorf("NPC %q full greeting was not preserved for the hover tooltip", key)
			}
		}
	}
	if overflowing == 0 {
		t.Skip("no shipped greeting currently overflows its two-line box")
	}

	// The generic dialog body reports the same condition through the layout.
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorld(cfg))
	long := strings.Repeat("Maruna turns a scale to the light and keeps talking. ", 40)
	npc := &character.NPC{Name: "Scalewright", DialogueData: &character.NPCDialogue{Greeting: long}}
	layout := g.dialogueLayout(npc, npcDialogWidth, npcDialogHeight)
	if !layout.bodyClipped() || len(layout.bodyFullLines) <= len(layout.bodyLines) {
		t.Fatal("an over-long body must report itself clipped and keep the full copy")
	}
	_ = config.TitleWords // keeps the config import honest if assertions change
}

// Journal order is driven by what the player must DO next: hand-in first, then
// in progress, then closed - alphabetical inside each group.
func TestQuestJournalOrder(t *testing.T) {
	mk := func(name string, completed, claimed bool) *quests.Quest {
		return &quests.Quest{
			ID:             name,
			Definition:     &quests.QuestDefinition{Name: name},
			Completed:      completed,
			RewardsClaimed: claimed,
		}
	}
	doneB := mk("B done", true, true)
	activeA := mk("A active", false, false)
	turnInZ := mk("Z turn in", true, false)
	activeC := mk("C active", false, false)
	turnInA := mk("A turn in", true, false)
	doneA := mk("A done", true, true)

	list := []*quests.Quest{doneB, activeA, turnInZ, activeC, turnInA, doneA}
	sortQuestJournal(list)

	var got []string
	for _, q := range list {
		got = append(got, q.Definition.Name)
	}
	want := []string{"A turn in", "Z turn in", "A active", "C active", "A done", "B done"}
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Fatalf("journal order:\n got %v\nwant %v", got, want)
	}

	// An auto-claimed quest (the victory quest) is closed, not waiting.
	auto := mk("Victory", true, true)
	auto.Definition.AutoClaim = true
	if questJournalRank(auto) != questRankDone {
		t.Fatalf("auto-claimed quest ranked %d, want done (%d)", questJournalRank(auto), questRankDone)
	}
}
