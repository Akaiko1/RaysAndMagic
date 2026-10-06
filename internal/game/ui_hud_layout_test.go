package game

import (
	"fmt"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/monster"
)

func TestPartyPortraitLayoutSpansViewportAndStaysProportional(t *testing.T) {
	resolutions := []struct {
		w, h int
	}{
		{740, 680},
		{1024, 768},
		{1280, 720},
		{1366, 768},
		{1920, 1080},
		{2580, 1080},
	}
	for _, res := range resolutions {
		t.Run(fmt.Sprintf("%dx%d", res.w, res.h), func(t *testing.T) {
			g, _ := newThiefTestGame(t)
			g.config.Display.ScreenWidth = res.w
			g.config.Display.ScreenHeight = res.h
			cardW, cardH, left, top := partyPortraitLayout(g)
			if uncovered := res.w - cardW*4; uncovered < 0 || uncovered > 3 {
				t.Fatalf("four cards cover %d of %d pixels", cardW*4, res.w)
			}
			if left < 0 || left > 1 {
				t.Fatalf("left margin = %d, want at most one pixel", left)
			}
			if top+cardH != res.h {
				t.Fatalf("party deck bottom = %d, want %d", top+cardH, res.h)
			}
			if cardH != partyCardPanelNativeHeight+partyCardFrameReserve*2 {
				t.Fatalf("party card height = %d, want native panel plus frame gutters", cardH)
			}
		})
	}
}

func TestPartyCardTemplatesKeepContentCenteredInsidePaintedFrame(t *testing.T) {
	for _, panelW := range []int{250, 314, 335, 474, 639} {
		layout := makePartyCardContentLayout(10, 20, panelW)
		leftSafe := 10 + partyPanelContentLeft
		rightSafe := 10 + panelW - partyPanelContentRight
		if layout.box.x < leftSafe || layout.box.right() > rightSafe {
			t.Fatalf("panel %d content x-range [%d,%d) leaves safe range [%d,%d)",
				panelW, layout.box.x, layout.box.right(), leftSafe, rightSafe)
		}
		leftPad := layout.box.x - leftSafe
		rightPad := rightSafe - layout.box.right()
		if abs(leftPad-rightPad) > 1 {
			t.Fatalf("panel %d content padding left=%d right=%d", panelW, leftPad, rightPad)
		}
		if layout.stats.right() >= layout.equipment.x || layout.equipment.right() != layout.box.right() {
			t.Fatalf("panel %d invalid columns: stats=%+v equipment=%+v box=%+v",
				panelW, layout.stats, layout.equipment, layout.box)
		}
	}
}

func TestPartyPortraitMatchesMeasuredSteppedAperture(t *testing.T) {
	if panelPortraitX != 17 || panelPortraitY != 20 || panelPortraitW != 47 || panelPortraitH != 56 {
		t.Fatalf("portrait aperture=(%d,%d %dx%d), want measured (17,20 47x56)",
			panelPortraitX, panelPortraitY, panelPortraitW, panelPortraitH)
	}
	want := [][3]int{
		{0, 3, 43},
		{1, 3, 44},
		{2, 2, 45},
		{3, 1, 46},
		{4, 0, 47},
		{51, 0, 47},
		{52, 1, 46},
		{53, 2, 45},
		{54, 3, 44},
		{55, 4, 43},
	}
	mask := newPartyPortraitApertureMaskImage()
	for _, row := range want {
		start, end := partyPortraitApertureSpan(row[0])
		if start != row[1] || end != row[2] {
			t.Fatalf("portrait row %d span=[%d,%d), want [%d,%d)", row[0], start, end, row[1], row[2])
		}
		for col := 0; col < panelPortraitW; col++ {
			wantAlpha := uint8(0)
			if col >= start && col < end {
				wantAlpha = 0xff
			}
			if got := mask.AlphaAt(col, row[0]).A; got != wantAlpha {
				t.Fatalf("portrait mask (%d,%d) alpha=%d, want %d", col, row[0], got, wantAlpha)
			}
		}
	}
}

func TestPartyCardTemplatesUseDistinctResponsiveColumnPlans(t *testing.T) {
	compact := partyCardTemplateForWidth(250)
	standard := partyCardTemplateForWidth(335)
	wide := partyCardTemplateForWidth(474)
	if compact == standard || standard == wide || compact == wide {
		t.Fatalf("responsive card templates collapsed: compact=%+v standard=%+v wide=%+v", compact, standard, wide)
	}
	if compact.columnGap >= standard.columnGap || standard.columnGap >= wide.columnGap {
		t.Fatalf("column gutters do not grow with available space: %d, %d, %d",
			compact.columnGap, standard.columnGap, wide.columnGap)
	}
}

func TestPartyProgressionBadgesStayAttachedToPortrait(t *testing.T) {
	const px, py = 100, 200
	one := makePartyProgressionBadgeLayout(px, py, panelPortraitW, panelPortraitH, true, false)
	if one.stat.x != px+(panelPortraitW-partyProgressBadgeSize)/2 {
		t.Fatalf("single stat badge x=%d, want centered on portrait", one.stat.x)
	}
	if one.stat.y != py+panelPortraitH-partyProgressBadgeLift {
		t.Fatalf("single stat badge y=%d, want portrait-rim anchor", one.stat.y)
	}

	two := makePartyProgressionBadgeLayout(px, py, panelPortraitW, panelPortraitH, true, true)
	if gap := two.skill.x - two.stat.right(); gap != partyProgressBadgeGap {
		t.Fatalf("two-badge gap=%d, want %d", gap, partyProgressBadgeGap)
	}
	portraitCenter := px + panelPortraitW/2
	badgesCenter := (two.stat.x + two.skill.right()) / 2
	if abs(portraitCenter-badgesCenter) > 1 {
		t.Fatalf("two badges center=%d, portrait center=%d", badgesCenter, portraitCenter)
	}
	if two.stat.y != two.skill.y {
		t.Fatalf("badge baselines differ: stat=%d skill=%d", two.stat.y, two.skill.y)
	}
}

func TestPartyAutoButtonUsesEquipmentBottomRow(t *testing.T) {
	for _, panelW := range []int{179, 250, 314, 474, 639} {
		content := makePartyCardContentLayout(10, 20, panelW)
		auto := makePartyAutoButtonLayout(content)
		if auto.x < content.equipment.x || auto.right() > content.equipment.right() {
			t.Fatalf("panel %d auto x-range [%d,%d) leaves equipment [%d,%d)",
				panelW, auto.x, auto.right(), content.equipment.x, content.equipment.right())
		}
		if auto.bottom() != content.box.bottom() {
			t.Fatalf("panel %d auto bottom=%d, want content bottom=%d", panelW, auto.bottom(), content.box.bottom())
		}
	}
}

func TestPartyCardsFillQuarterSlotsWithoutConsumingFrameGutters(t *testing.T) {
	for _, screenW := range []int{1280, 1366, 1440, 1600, 1680, 1920, 2560} {
		slotW := screenW / 4
		x, y, w, h := partyCardPanelRect(0, 0, slotW, partyCardPanelNativeHeight+partyCardFrameReserve*2)
		if w != slotW-partyCardFrameReserve*2 || h != partyCardPanelNativeHeight {
			t.Fatalf("screen %d panel=%dx%d, want slot width minus frame gutters and native height", screenW, w, h)
		}
		if left, right := x, slotW-(x+w); left != partyCardFrameReserve || right != partyCardFrameReserve || y != partyCardFrameReserve {
			t.Fatalf("screen %d card gutters left/right/top=%d/%d/%d", screenW, left, right, y)
		}
	}
}

func TestGameplayViewportBottomUsesPartyLayoutAtEveryResolution(t *testing.T) {
	for _, res := range [][2]int{{740, 680}, {1024, 768}, {1280, 720}, {1920, 1080}, {2580, 1080}} {
		g, _ := newThiefTestGame(t)
		g.config.Display.ScreenWidth = res[0]
		g.config.Display.ScreenHeight = res[1]
		g.showPartyStats = true
		_, _, _, partyTop := partyPortraitLayout(g)
		want := partyTop - partyHUDWorldClearance
		if got := gameplayViewportBottom(g); got != want {
			t.Fatalf("%dx%d viewport bottom=%d, want %d above party top", res[0], res[1], got, want)
		}
		g.showPartyStats = false
		if got := gameplayViewportBottom(g); got != res[1] {
			t.Fatalf("%dx%d hidden HUD viewport bottom=%d, want %d", res[0], res[1], got, res[1])
		}
	}
}

func TestPartyCooldownProgressTracksObservedCooldown(t *testing.T) {
	g, _ := newThiefTestGame(t)
	ui := NewUISystem(g)
	member := g.party.Members[0]
	member.RTCooldown = 80
	remaining, progress, active := ui.partyCooldownProgress(member, partySingleHandCooldown(member))
	if !active || remaining != 80 || progress != 0 {
		t.Fatalf("initial cooldown = active:%v remaining:%d progress:%.2f", active, remaining, progress)
	}
	member.RTCooldown = 40
	_, progress, _ = ui.partyCooldownProgress(member, partySingleHandCooldown(member))
	if progress != 0.5 {
		t.Fatalf("half cooldown progress = %.2f, want 0.50", progress)
	}
	member.RTCooldown = 0
	if _, _, active := ui.partyCooldownProgress(member, partySingleHandCooldown(member)); active {
		t.Fatal("completed cooldown remained active")
	}
	if _, ok := ui.partyCooldownState[member]; ok {
		t.Fatal("completed cooldown retained visual state")
	}
}

func TestArmsMasterCooldownProgressTracksHandsIndependently(t *testing.T) {
	g, _ := newThiefTestGame(t)
	ui := NewUISystem(g)
	member := g.party.Members[0]
	member.Class = character.ClassArmsMaster
	member.RTCooldown = 80
	member.OffHandRTCooldown = 0

	mainProgress, offProgress, active := ui.partyArmsMasterCooldownProgress(member)
	if !active || mainProgress != 0 || offProgress != 1 {
		t.Fatalf("initial split cooldown = active:%v main:%.2f off:%.2f, want true/0/1", active, mainProgress, offProgress)
	}

	member.RTCooldown = 40
	member.OffHandRTCooldown = 60
	mainProgress, offProgress, active = ui.partyArmsMasterCooldownProgress(member)
	if !active || mainProgress != 0.5 || offProgress != 0 {
		t.Fatalf("independent split cooldown = active:%v main:%.2f off:%.2f, want true/0.5/0", active, mainProgress, offProgress)
	}

	member.RTCooldown = 0
	member.OffHandRTCooldown = 30
	mainProgress, offProgress, active = ui.partyArmsMasterCooldownProgress(member)
	if !active || mainProgress != 1 || offProgress != 0.5 {
		t.Fatalf("main-ready split cooldown = active:%v main:%.2f off:%.2f, want true/1/0.5", active, mainProgress, offProgress)
	}

	member.OffHandRTCooldown = 0
	if _, _, active := ui.partyArmsMasterCooldownProgress(member); active {
		t.Fatal("fully recovered Arms Master retained split cooldown frame")
	}
	if _, ok := ui.partyCooldownState[member]; ok {
		t.Fatal("fully recovered Arms Master retained visual state")
	}
}

// The panel sits centred in its slot, and the state and selection bands each
// get their own pixels: they never overlap, a gap separates them, and the
// selection band stays inside the slot.
func TestPartyCardFrameBandsNeverOverlap(t *testing.T) {
	const slotW = 256
	const slotH = partyCardPanelNativeHeight + partyCardFrameReserve*2
	panelX, panelY, panelW, panelH := partyCardPanelRect(0, 0, slotW, slotH)
	if panelX != slotW-(panelX+panelW) || panelY != slotH-(panelY+panelH) {
		t.Fatalf("panel gutters L/R/T/B = %d/%d/%d/%d", panelX, slotW-(panelX+panelW), panelY, slotH-(panelY+panelH))
	}
	// Distances from the panel edge each band's pixels occupy.
	stateFrom, stateTo := partyStateBandGap, partyStateBandGap+partyFrameBand-1
	selFrom, selTo := partySelectionBandGap, partySelectionBandGap+partyFrameBand-1
	if stateFrom < 1 {
		t.Fatal("the state band touches the painted panel")
	}
	if selFrom <= stateTo+1 {
		t.Fatalf("state band %d..%d and selection band %d..%d have no gap", stateFrom, stateTo, selFrom, selTo)
	}
	if selTo > partyCardFrameReserve {
		t.Fatalf("selection band reaches distance %d, beyond the %dpx gutter", selTo, partyCardFrameReserve)
	}
	if partyFrameBand < 3 {
		t.Fatalf("bands are %dpx; the metal ramp needs 3", partyFrameBand)
	}
}

func TestCenteredIconRowHasBalancedHorizontalPadding(t *testing.T) {
	const barX, barW, iconSize, gap, count = 10, 114, 32, 5, 3
	start := centeredIconRowX(barX, barW, iconSize, gap, count)
	contentW := count*iconSize + (count-1)*gap
	left := start - barX
	right := barX + barW - (start + contentW)
	if left != right {
		t.Fatalf("effect rail padding left=%d right=%d", left, right)
	}
}

// An idle card must not allocate or blit its particle layer; each effect that
// the layer block paints must switch it back on.
func TestPartyCardEffectLayerOnlyWakesForRealEffects(t *testing.T) {
	ui := &UISystem{}
	if effects := ui.partyCardEffects(0, 200, 100, false); effects != nil {
		t.Fatal("idle card allocated an effect layer")
	}
	if (partyCardEffectSet{}).any() {
		t.Fatal("empty effect set reported work to do")
	}
	for name, set := range map[string]partyCardEffectSet{
		"poison": {poison: true},
		"burn":   {burn: true},
		"stun":   {stun: true},
		"timed":  {timed: true},
	} {
		if !set.any() {
			t.Fatalf("%s effect did not request the layer", name)
		}
		if effects := ui.partyCardEffects(1, 200, 100, set.any()); effects == nil {
			t.Fatalf("%s effect got no layer", name)
		}
	}
}

// anyTimedCardFxActive drives that gate for the timer-based overlays. fxBlink
// tints the portrait directly and must NOT wake the layer.
func TestAnyTimedCardFxActiveCoversLayerOverlaysOnly(t *testing.T) {
	g := &MMGame{}
	if g.anyTimedCardFxActive(0) {
		t.Fatal("no timers running, yet the layer was requested")
	}
	for _, fx := range []cardFx{fxFlame, fxSpark, fxHeal} {
		g.cardFxTimers = [cardFxCount][4]int{}
		g.cardFxTimers[fx][0] = 5
		if !g.anyTimedCardFxActive(0) {
			t.Fatalf("card fx %d did not request the layer", fx)
		}
	}
	g.cardFxTimers = [cardFxCount][4]int{}
	g.cardFxTimers[fxBlink][0] = 5
	if g.anyTimedCardFxActive(0) {
		t.Fatal("fxBlink woke the effect layer - it only tints the portrait")
	}
}

// The split cooldown readout represents two ATTACKING hands. An Arms Master
// with an empty off-hand, or a shield in it, has only one - the split frame
// would paint its lower half permanently ready. Asserted on the card's gate.
func TestSplitCooldownFrameRequiresRealDualWield(t *testing.T) {
	g, _ := newThiefTestGame(t)
	member := g.party.Members[0]
	member.Class = character.ClassArmsMaster
	// Arms Masters are authored with dual_wielding among their starting skills
	// (config.yaml); IsDualWielding needs BOTH that skill and a real weapon.
	member.Skills[character.SkillDualWielding] = &character.Skill{}
	if member.Equipment == nil {
		member.Equipment = map[items.EquipSlot]items.Item{}
	}
	member.Equipment[items.SlotMainHand] = items.Item{Name: "Iron Sword", Type: items.ItemWeapon}

	cases := []struct {
		name     string
		offHand  *items.Item
		wantWide bool
	}{
		{name: "empty off-hand", offHand: nil},
		{name: "shield off-hand", offHand: &items.Item{Name: "Kite Shield", Type: items.ItemArmor}},
		{name: "weapon off-hand", offHand: &items.Item{Name: "Iron Sword", Type: items.ItemWeapon}, wantWide: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			delete(member.Equipment, items.SlotOffHand)
			if tc.offHand != nil {
				member.Equipment[items.SlotOffHand] = *tc.offHand
			}
			if got := partyCardShowsSplitCooldown(member); got != tc.wantWide {
				t.Fatalf("split cooldown frame = %v, want %v", got, tc.wantWide)
			}
		})
	}
}

// The compass is derived from the viewport alone: it stays inside the readable
// band at every shipped resolution, never shrinks as the screen grows, and
// does grow between a small and a full-HD screen.
func TestCompassRadiusFollowsTheViewportWithinReadableBounds(t *testing.T) {
	g, _ := newThiefTestGame(t)
	ui := NewUISystem(g)
	radius := func(w, h int) int {
		g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = w, h
		return ui.compassRadius()
	}
	prev := 0
	for _, res := range []struct{ w, h int }{{800, 600}, {1024, 720}, {1024, 768}, {1920, 1080}, {2560, 1440}, {3840, 2160}} {
		got := radius(res.w, res.h)
		if got < compassMinRadius || got > compassMaxRadius {
			t.Fatalf("%dx%d compass radius = %d, want within %d..%d", res.w, res.h, got, compassMinRadius, compassMaxRadius)
		}
		if got < prev {
			t.Fatalf("%dx%d compass radius = %d shrank from %d on a larger screen", res.w, res.h, got, prev)
		}
		prev = got
	}
	if small, large := radius(1024, 720), radius(1920, 1080); large <= small {
		t.Fatalf("compass radii small=%d large=%d, want it to respond to the viewport", small, large)
	}
}

// A hand with no weapon has no attack, so its cooldown must not drive the card:
// the split readout needs BOTH hands armed, and a single readout must ignore a
// stale timer belonging to an empty hand.
func TestCooldownReadoutFollowsArmedHands(t *testing.T) {
	g, _ := newThiefTestGame(t)
	member := g.party.Members[0]
	member.Class = character.ClassArmsMaster
	member.Skills[character.SkillDualWielding] = &character.Skill{}
	if member.Equipment == nil {
		member.Equipment = map[items.EquipSlot]items.Item{}
	}
	sword := items.Item{Name: "Iron Sword", Type: items.ItemWeapon}

	// Stale off-hand countdown left over from a removed second weapon.
	member.RTCooldown = 0
	member.OffHandRTCooldown = 90

	t.Run("main hand only ignores the stale off-hand timer", func(t *testing.T) {
		member.Equipment[items.SlotMainHand] = sword
		delete(member.Equipment, items.SlotOffHand)
		if member.MainHandArmed() && member.IsDualWielding() {
			t.Fatal("one-weapon character reported as dual wielding")
		}
		if got := partySingleHandCooldown(member); got.remaining != 0 || got.mode != partyCooldownModeMain {
			t.Fatalf("single-hand readout = %+v, want main-hand 0 - the empty hand's timer leaked in", got)
		}
	})

	t.Run("off hand only counts its own timer", func(t *testing.T) {
		delete(member.Equipment, items.SlotMainHand)
		member.Equipment[items.SlotOffHand] = sword
		if member.MainHandArmed() {
			t.Fatal("empty main hand reported as armed")
		}
		if got := partySingleHandCooldown(member); got.remaining != 90 || got.mode != partyCooldownModeOffHand {
			t.Fatalf("single-hand readout = %+v, want off-hand 90", got)
		}
		// The mirror case: the removed main-hand weapon left a long countdown
		// while the off-hand is already free to strike.
		member.RTCooldown = 90
		member.OffHandRTCooldown = 0
		if got := partySingleHandCooldown(member); got.remaining != 0 {
			t.Fatalf("ready off-hand shows %+v, want 0 - the removed weapon's timer leaked in", got)
		}
		member.RTCooldown = 0
		member.OffHandRTCooldown = 90
	})

	t.Run("both hands armed enables the split readout", func(t *testing.T) {
		member.Equipment[items.SlotMainHand] = sword
		member.Equipment[items.SlotOffHand] = sword
		if !(member.MainHandArmed() && member.IsDualWielding()) {
			t.Fatal("two weapons did not enable the split readout")
		}
	})

	t.Run("unarmed caster still shows its spell cooldown", func(t *testing.T) {
		delete(member.Equipment, items.SlotMainHand)
		delete(member.Equipment, items.SlotOffHand)
		member.RTCooldown = 45
		member.OffHandRTCooldown = 90
		if got := partySingleHandCooldown(member); got.remaining != 45 || got.mode != partyCooldownModeMain {
			t.Fatalf("unarmed readout = %+v, want main-hand spell cooldown 45", got)
		}
	})
}

// HP/SP totals never clip at the shipped default resolution in any font, where
// the compact stats column is sized for native pixel-font readouts (AGENTS.md
// HUD QA rule); the shipped default font still affords the full labelled form.
// A wider font drops the label instead of clipping.
func TestMeterTextFitsCompactStatsColumn(t *testing.T) {
	g, _ := newThiefTestGame(t)
	g.config.Display.ScreenWidth = 1024
	g.config.Display.ScreenHeight = 768
	cardW, cardH, _, _ := partyPortraitLayout(g)
	_, _, panelW, _ := partyCardPanelRect(0, 0, cardW, cardH)
	statsW := makePartyCardContentLayout(0, 0, panelW).stats.w

	budget := statsW - 4
	for _, font := range shippedUIFonts(t) {
		t.Run("font="+font.Key, func(t *testing.T) {
			withUIFont(t, font.Key)
			full := font.Key == g.config.Display.DefaultFont
			for _, meter := range []struct {
				label            string
				current, maximum int
			}{
				{"HP", 33, 33}, {"HP", 100, 100}, {"SP", 100, 100}, {"HP", 1000, 1000},
			} {
				text := meterText(budget, meter.label, meter.current, meter.maximum)
				if drawn := uiTextWidth(text); drawn > budget {
					t.Fatalf("%q draws %dpx into a %dpx box at native scale - it would be clipped", text, drawn, budget)
				}
				// The default resolution must still afford the full labelled form.
				if want := fmt.Sprintf("%s %d/%d", meter.label, meter.current, meter.maximum); full && text != want && meter.maximum <= 100 {
					t.Fatalf("meter text = %q, want the full %q at the default resolution", text, want)
				}
			}
		})
	}
}

// The narrowest supported window drops detail rather than shrinking glyphs into
// mush - and never clips.
func TestMeterTextStaysLegibleAtMinimumWindow(t *testing.T) {
	g, _ := newThiefTestGame(t)
	minW, minH := MinimumWindowSize()
	g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = minW, minH
	cardW, cardH, _, _ := partyPortraitLayout(g)
	_, _, panelW, _ := partyCardPanelRect(0, 0, cardW, cardH)
	budget := makePartyCardContentLayout(0, 0, panelW).stats.w - 4

	text := meterText(budget, "HP", 100, 100)
	if drawn := uiTextWidth(text); drawn > budget {
		t.Fatalf("%q draws %dpx into %dpx at %dx%d", text, drawn, budget, minW, minH)
	}
	if text == "" {
		t.Fatal("meter text vanished at the minimum window size")
	}
	t.Logf("minimum window %dx%d: budget=%dpx text=%q at native scale", minW, minH, budget, text)
}

// Dropping the main-hand weapon mid-cooldown switches the card from the split
// readout to the off-hand-only one. The fill must then be measured against the
// OFF-HAND's own peak, not the main hand's remembered one, or the frame jumps.
func TestCooldownHistoryFollowsReadoutModeChange(t *testing.T) {
	g, _ := newThiefTestGame(t)
	ui := NewUISystem(g)
	member := g.party.Members[0]
	member.Class = character.ClassArmsMaster
	member.Skills[character.SkillDualWielding] = &character.Skill{}
	if member.Equipment == nil {
		member.Equipment = map[items.EquipSlot]items.Item{}
	}
	sword := items.Item{Name: "Iron Sword", Type: items.ItemWeapon}
	member.Equipment[items.SlotMainHand] = sword
	member.Equipment[items.SlotOffHand] = sword

	// Split phase: a long main-hand swing and a short off-hand one.
	member.RTCooldown, member.OffHandRTCooldown = 120, 30
	if _, _, active := ui.partyArmsMasterCooldownProgress(member); !active {
		t.Fatal("split readout not active with both hands cooling")
	}
	member.RTCooldown, member.OffHandRTCooldown = 100, 20
	ui.partyArmsMasterCooldownProgress(member)

	// The main-hand weapon is removed: only the off-hand attacks now.
	delete(member.Equipment, items.SlotMainHand)
	readout := partySingleHandCooldown(member)
	if readout.mode != partyCooldownModeOffHand || readout.remaining != 20 {
		t.Fatalf("readout after dropping the main weapon = %+v, want off-hand 20", readout)
	}
	_, progress, active := ui.partyCooldownProgress(member, readout)
	if !active {
		t.Fatal("off-hand cooldown reported inactive")
	}
	// The off-hand's own swing peaked at 30 and has 20 left: 1 - 20/30. Measured
	// against the main hand's remembered peak of 120 it would read ~0.83.
	if want := 1 - 20.0/30.0; math.Abs(progress-want) > 0.01 {
		t.Fatalf("progress after the mode switch = %.3f, want %.3f from the off-hand's own peak", progress, want)
	}
	// The main hand is no longer read, so it must keep no peak to skew a later
	// readout (re-equipping a weapon must start its fill from zero).
	if state := ui.partyCooldownState[member]; state.peakRemaining != 0 || state.lastRemaining != 0 {
		t.Fatalf("idle main-hand history retained: %+v", state)
	}
	// It must then advance monotonically as that hand's own countdown drains.
	member.OffHandRTCooldown = 10
	_, later, _ := ui.partyCooldownProgress(member, partySingleHandCooldown(member))
	if later <= progress {
		t.Fatalf("off-hand fill did not advance: %.2f then %.2f", progress, later)
	}
}

// Wizard Eye keeps enemy threat colors by distance; allies show green and
// non-hostile ambient actors blue instead of reading as nearby enemies.
func TestWizardEyeRadarDotCategories(t *testing.T) {
	g, _, _, _, _ := mouseCombatHarness(t, false)
	ui := &UISystem{game: g}
	ui.initRadarDots()
	ts := float64(g.config.GetTileSize())
	mk := func(mod func(*monster.Monster3D)) *monster.Monster3D {
		m := monster.NewMonster3DFromConfig(0, 0, "goblin", g.config)
		mod(m)
		return m
	}
	type row struct {
		name  string
		m     *monster.Monster3D
		tiles float64
		want  *ebiten.Image
	}
	hostile := mk(func(*monster.Monster3D) {})
	rows := []row{
		{"close enemy", hostile, 1, ui.radarDotClose},
		{"medium enemy", hostile, 4, ui.radarDotMedium},
		{"far enemy", hostile, 8, ui.radarDotFar},
		{"passive enemy", mk(func(m *monster.Monster3D) { m.PassiveUntilAttacked = true }), 1, ui.radarDotClose},
		{"bound", mk(func(m *monster.Monster3D) { m.Bound = true }), 1, ui.radarDotAlly},
		{"charmed", mk(func(m *monster.Monster3D) { m.Pacified = true }), 1, ui.radarDotAlly},
		{"caravan", mk(func(m *monster.Monster3D) { m.Disposition = monster.DispositionCaravan }), 1, ui.radarDotNeutral},
		{"wildlife", mk(func(m *monster.Monster3D) { m.Disposition = monster.DispositionWildlife }), 1, ui.radarDotNeutral},
		{"fish", mk(func(m *monster.Monster3D) { m.Disposition = monster.DispositionFish }), 1, ui.radarDotNeutral},
	}
	for _, s := range partySummonKinds(g) {
		rows = append(rows, row{s.kind, mk(func(m *monster.Monster3D) { markPurePartySummon(m, s.owner) }), 1, ui.radarDotAlly})
	}
	for _, r := range rows {
		distSq := (r.tiles * ts) * (r.tiles * ts)
		if got := ui.radarDot(r.m, distSq, ts); got != r.want {
			t.Errorf("%s: wrong radar dot", r.name)
		}
	}
}

// The mode panel (REAL-TIME / TURN-BASED and the turn lines) holds every line
// inside its frame in every shipped font, in both clocks and both phases.
func TestTurnModePanelFitsItsTextInEveryFont(t *testing.T) {
	g, _ := newThiefTestGame(t)
	ui := NewUISystem(g)
	forEachUIFont(t, func(t *testing.T) {
		for _, pose := range []struct {
			tb   bool
			turn int
		}{{false, 0}, {true, 0}, {true, 1}} {
			g.turnBasedMode, g.currentTurn = pose.tb, pose.turn
			g.partyActionsUsed = 2
			lines, x, _, w, h := ui.turnBasedStatusLayout()
			for i, line := range lines {
				if right := x + textPanelPadding + uiTextWidth(line); right > x+w-1 {
					t.Errorf("tb=%v turn=%d: %q ends at %d, past the frame at %d", pose.tb, pose.turn, line, right, x+w-1)
				}
				if bottom := textPanelPadding + i*textPanelLineHeight + uiTextCharHeight; bottom > h {
					t.Errorf("tb=%v turn=%d: line %d runs below the frame", pose.tb, pose.turn, i)
				}
			}
			if x+w > g.config.GetScreenWidth() {
				t.Errorf("tb=%v: the panel leaves the screen", pose.tb)
			}
		}
	})
}

// A cooldown running when the party is replaced (a load, a new game) must not
// pin the old hero: the cache keeps exactly the heroes that still have a card,
// and a current hero keeps the peak its fill is measured against.
func TestPartyCooldownCacheForgetsReplacedHeroes(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
	ui := NewUISystem(g)
	if len(g.party.Members) < 2 {
		t.Fatal("fixture party needs two heroes")
	}
	kept, gone := g.party.Members[0], g.party.Members[1]
	for _, m := range []*character.MMCharacter{kept, gone} {
		m.RTCooldown = 30
		if _, _, active := ui.partyCooldownProgress(m, partySingleHandCooldown(m)); !active {
			t.Fatal("cooldown readout inactive (positive control)")
		}
	}
	g.party.Members = []*character.MMCharacter{kept}
	ui.prunePartyCooldownState()
	if _, pinned := ui.partyCooldownState[gone]; pinned {
		t.Fatal("a replaced hero is still pinned by the cooldown cache")
	}
	if st, ok := ui.partyCooldownState[kept]; !ok || st.peakRemaining != 30 {
		t.Fatalf("current hero cooldown state = %+v, %v; want its peak of 30", st, ok)
	}
}
