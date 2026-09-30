package game

import (
	"image"
	"strings"
	"testing"
	"time"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/spells"

	"github.com/hajimehoshi/ebiten/v2"
)

// dwellHarness drives the dwell clock and the pointer by hand.
type dwellHarness struct {
	h     *displayedModalHarness
	clock time.Time
	at    image.Point
}

func newDwellHarness(t *testing.T) *dwellHarness {
	t.Helper()
	d := &dwellHarness{h: newDisplayedModalHarness(t, 1280, 720), clock: time.Unix(1000, 0)}
	prevNow, prevCursor := dwellNow, rawCursorPosition
	dwellNow = func() time.Time { return d.clock }
	rawCursorPosition = func() (int, int) { return d.at.X, d.at.Y }
	t.Cleanup(func() { dwellNow, rawCursorPosition = prevNow, prevCursor })
	d.h.g.dialogActive = false
	return d
}

// frame draws one presented frame and returns the title of the card on
// screen - a shared tooltip or the quick bar's own card ("" = none). In game
// only the interface is drawn: the harness has no renderer for the world.
func (d *dwellHarness) frame() string {
	if d.h.g.appScreen == AppScreenInGame {
		beginUIFrame(d.h.screen, d.h.g.uiPixelScale())
		d.h.ui.Draw(d.h.screen)
	} else {
		d.h.loop.Draw(d.h.screen)
	}
	if card := d.h.ui.quickCard; card.drawn.h > 0 {
		return card.item.Name
	}
	if len(d.h.ui.tooltipLines) == 0 {
		return ""
	}
	return d.h.ui.tooltipLines[0]
}

// Quick-slot cells and party-creation entries open their card only after the
// pointer rests on them for tooltipDwellDelay; leaving restarts the rest.
func TestDwellTooltipsOpenAfterTheRest(t *testing.T) {
	for _, tc := range []struct {
		name  string
		delay time.Duration
		// pose sets the screen up and returns the target point and the
		// card title it must open.
		pose func(t *testing.T, d *dwellHarness) (image.Point, string)
	}{
		{"in-game quick slot", quickSlotCardDelay, func(t *testing.T, d *dwellHarness) (image.Point, string) {
			g := d.h.g
			potion := items.CreateItemFromYAML("health_potion")
			g.party.Members[g.selectedChar].QuickSlots[1] = &potion
			bar, visible := inGameQuickSlotBarLayout(g)
			if !visible {
				t.Fatal("fixture: the in-game quick bar is hidden")
			}
			_, cells := quickSlotRects(bar.x, bar.y, bar.w)
			return centerOf(cells[1]), potion.Name
		}},
		{"spellbook quick slot", quickSlotCardDelay, func(t *testing.T, d *dwellHarness) (image.Point, string) {
			g := d.h.g
			spell, err := spells.CreateSpellItem("firebolt")
			if err != nil {
				t.Fatal(err)
			}
			g.party.Members[g.selectedChar] = character.CreateCharacter("Lysander", character.ClassSorcerer, g.config)
			g.party.Members[g.selectedChar].QuickSlots[0] = &spell
			g.menuOpen, g.currentTab = true, TabSpellbook
			bar := computeBookLayout(computeTabbedMenuLayout(g.config.GetScreenWidth(), gameplayViewportBottom(g)).content).quick
			_, cells := quickSlotRects(bar.x, bar.y, bar.w)
			return centerOf(cells[0]), spell.Name
		}},
		{"party creation skill", tooltipDwellDelay, func(t *testing.T, d *dwellHarness) (image.Point, string) {
			return d.findPartyCreateToken(t, "party_create_skill:")
		}},
		{"party creation spell", tooltipDwellDelay, func(t *testing.T, d *dwellHarness) (image.Point, string) {
			return d.findPartyCreateToken(t, "party_create_spell:")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := newDwellHarness(t)
			target, title := tc.pose(t, d)
			d.at = target
			if got := d.frame(); got != "" {
				t.Fatalf("card %q opened the moment the pointer arrived", got)
			}
			d.clock = d.clock.Add(tc.delay - time.Millisecond)
			if got := d.frame(); got != "" {
				t.Fatalf("card %q opened before the dwell delay", got)
			}
			d.clock = d.clock.Add(time.Millisecond)
			if got := d.frame(); !strings.Contains(got, title) {
				t.Fatalf("after the dwell the card title is %q, want %q", got, title)
			}
			// The quick bar's own card never covers its bar; party creation
			// uses the shared tooltip, and the shared queue stays empty for
			// the bar.
			card := d.h.ui.quickCard
			if quick := tc.delay == quickSlotCardDelay; quick != (card.drawn.h > 0) || quick && d.h.ui.tooltipLines != nil {
				t.Fatalf("quick card drawn=%+v shared tooltip=%v", card.drawn, d.h.ui.tooltipLines != nil)
			}
			if r, bar := card.drawn, card.bar; r.h > 0 && r.y < bar.bottom() && r.bottom() > bar.y && r.x < bar.right() && r.right() > bar.x {
				t.Fatalf("quick card %+v covers its bar %+v", r, bar)
			}
			// Leaving ends the rest; coming back waits again.
			d.at = image.Pt(2, 2)
			d.frame()
			d.at = target
			if got := d.frame(); got != "" {
				t.Fatalf("card %q reopened without a new rest", got)
			}
			d.clock = d.clock.Add(tc.delay)
			if got := d.frame(); !strings.Contains(got, title) {
				t.Fatalf("the second rest opened %q, want %q", got, title)
			}
		})
	}
}

// Under a modal layer or while a hero is carried the rest opens nothing.
func TestDwellTooltipsStayShutWhileBlocked(t *testing.T) {
	t.Run("quick slot under a modal", func(t *testing.T) {
		d := newDwellHarness(t)
		g := d.h.g
		potion := items.CreateItemFromYAML("health_potion")
		g.party.Members[g.selectedChar].QuickSlots[1] = &potion
		bar, _ := inGameQuickSlotBarLayout(g)
		_, cells := quickSlotRects(bar.x, bar.y, bar.w)
		d.at = centerOf(cells[1])
		g.openMainMenu()
		d.frame()
		d.clock = d.clock.Add(2 * quickSlotCardDelay)
		if got := d.frame(); got == potion.Name {
			t.Fatal("the bar under the main menu opened its card")
		}
	})
	t.Run("party creation while dragging", func(t *testing.T) {
		d := newDwellHarness(t)
		at, _ := d.findPartyCreateToken(t, "party_create_skill:")
		pc := d.h.g.partyCreate
		pc.drag = pc.slots[3]
		d.at = at
		d.frame()
		d.clock = d.clock.Add(2 * tooltipDwellDelay)
		if got := d.frame(); got != "" {
			t.Fatalf("card %q opened under a carried hero", got)
		}
	})
}

// A card that would cover a strip moves above it, or below it when the
// strip sits too high; cards clear of it stay put.
func TestQuickSlotCardYClearOf(t *testing.T) {
	strip := layoutRect{100, 600, 240, 54}
	for _, tc := range []struct {
		name     string
		y, h     int
		strip    layoutRect
		wantY    int
		screenHt int
	}{
		{"no strip", 580, 100, layoutRect{}, 580, 720},
		{"already above", 400, 150, strip, 400, 720},
		{"already below", 660, 40, strip, 660, 720},
		{"overlapping moves above", 560, 100, strip, 600 - tooltipScreenMargin - 100, 720},
		{"too tall for above moves below", 30, 700, layoutRect{100, 60, 240, 54}, 60 + 54 + tooltipScreenMargin, 1080},
	} {
		if got := cardYClearOf(tc.y, tc.h, tc.strip, tc.screenHt); got != tc.wantY {
			t.Errorf("%s: y = %d, want %d", tc.name, got, tc.wantY)
		}
		if got := cardYClearOf(tc.y, tc.h, tc.strip, tc.screenHt); tc.strip.h > 0 && got < tc.strip.bottom() && got+tc.h > tc.strip.y {
			t.Errorf("%s: card still covers the strip", tc.name)
		}
	}
}

func centerOf(r image.Rectangle) image.Point {
	return image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
}

// findPartyCreateToken opens party creation on the cleric and finds a token
// with the key prefix; it returns its point and the card title it opens.
func (d *dwellHarness) findPartyCreateToken(t *testing.T, prefix string) (image.Point, string) {
	t.Helper()
	g := d.h.g
	g.appScreen = AppScreenPartyCreate
	g.partyCreate = newPartyCreateState(g.config)
	pc := g.partyCreate
	pc.detail = pc.slots[2]
	if pc.detail.char.Class != character.ClassCleric {
		t.Fatalf("fixture: slot 3 is %s, want the cleric", pc.detail.char.Class)
	}
	lay := partyCreateLayout(pc, g.config.GetScreenWidth(), g.config.GetScreenHeight())
	// Tokens start at the list indents.
	tx := lay.detail.x + 16
	xs := []int{tx + uiTextWidth("  ") + 2, tx + uiTextWidth("      ") + 2}
	for _, id := range character.AllMagicSchools {
		xs = append(xs, tx+uiTextWidth("  "+humanizeKey(string(id))+": ")+2)
	}
	at, key, ok := scanHeroDetailTokens(d.h.ui, pc, lay, xs, func(p image.Point) { d.at = p },
		func(key string) bool { return strings.HasPrefix(key, prefix) })
	if !ok {
		t.Fatalf("no %s token found in the detail panel", prefix)
	}
	id := strings.TrimPrefix(key, prefix)
	if prefix == "party_create_spell:" {
		def, err := spells.GetSpellDefinitionByID(spells.SpellID(id))
		if err != nil {
			t.Fatal(err)
		}
		return at, def.Name
	}
	return at, id
}

// scanHeroDetailTokens rests the pointer at each x of xs down the detail
// panel, at every scroll stop the wheel reaches, until the dwell names a
// token want accepts. It draws the panel alone: the rest of the screen holds
// no tokens.
func scanHeroDetailTokens(ui *UISystem, pc *partyCreateState, lay pcLayout, xs []int, moveTo func(image.Point), want func(key string) bool) (image.Point, string, bool) {
	scratch := ebiten.NewImage(lay.detail.x+lay.detail.w, lay.detail.y+lay.detail.h)
	defer scratch.Deallocate()
	draw := func() {
		ui.beginDisplayedInput()
		ui.drawHeroDetailPanel(scratch, pc.detail, lay.detail)
		ui.endDisplayedInput()
	}
	moveTo(image.Point{})
	draw()
	for scroll := 0; scroll <= pc.detailMaxScroll+16; scroll += 16 {
		for y := lay.detail.y; y < lay.detail.y+lay.detail.h; y += 4 {
			for _, x := range xs {
				pc.detailScroll = min(scroll, pc.detailMaxScroll)
				p := image.Pt(x, y)
				moveTo(p)
				draw()
				if key := ui.hoverDwell.key; want(key) {
					return p, key, true
				}
			}
		}
	}
	return image.Point{}, "", false
}
