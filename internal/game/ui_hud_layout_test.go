package game

import (
	"fmt"
	"math"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/items"
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

func TestMonsterSpriteClampUsesResponsiveGameplayViewport(t *testing.T) {
	g, _ := newThiefTestGame(t)
	g.config.Display.ScreenWidth = 1920
	g.config.Display.ScreenHeight = 1080
	g.showPartyStats = true
	const spriteHeight = 180.0
	partyTop := float64(gameplayViewportBottom(g))
	if got := clampMonsterSpriteTopToGameplayViewport(g, partyTop-100, spriteHeight); got != partyTop-spriteHeight {
		t.Fatalf("clamped monster top=%.1f, want %.1f", got, partyTop-spriteHeight)
	}
	const safeTop = 500.0
	if got := clampMonsterSpriteTopToGameplayViewport(g, safeTop, spriteHeight); got != safeTop {
		t.Fatalf("safe monster moved from %.1f to %.1f", safeTop, got)
	}
	g.showPartyStats = false
	if got := clampMonsterSpriteTopToGameplayViewport(g, partyTop-100, spriteHeight); got != partyTop-100 {
		t.Fatalf("hidden party HUD still clamped monster to %.1f", got)
	}
}

func TestCompassRadiusRespondsToViewportWithinReadableBounds(t *testing.T) {
	g, _ := newThiefTestGame(t)
	ui := NewUISystem(g)
	g.config.Display.ScreenWidth = 1024
	g.config.Display.ScreenHeight = 720
	small := ui.compassRadius()
	g.config.Display.ScreenWidth = 1920
	g.config.Display.ScreenHeight = 1080
	large := ui.compassRadius()
	if small < 36 || large > 64 || large <= small {
		t.Fatalf("compass radii small=%d large=%d, want responsive range 36..64", small, large)
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

func TestPartyCardPanelAndStateFramesUseReservedSymmetricGutters(t *testing.T) {
	const slotW = 256
	const slotH = partyCardPanelNativeHeight + partyCardFrameReserve*2
	panelX, panelY, panelW, panelH := partyCardPanelRect(0, 0, slotW, slotH)
	if panelX != slotW-(panelX+panelW) || panelY != slotH-(panelY+panelH) {
		t.Fatalf("panel gutters L/R/T/B = %d/%d/%d/%d", panelX, slotW-(panelX+panelW), panelY, slotH-(panelY+panelH))
	}
	outerX, outerY, outerW, outerH := expandedPartyPanelRect(panelX, panelY, panelW, panelH, partyCardOuterFrameGap)
	if outerX < 0 || outerY < 0 || outerX+outerW > slotW || outerY+outerH > slotH {
		t.Fatalf("outer frame (%d,%d %dx%d) leaves slot %dx%d", outerX, outerY, outerW, outerH, slotW, slotH)
	}
	if partyCardOuterFrameGap-partyCardInnerFrameGap != 1 {
		t.Fatal("nested selection and cooldown frames must touch without sharing one line")
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
// would paint its lower half permanently ready.
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
			if got := member.IsDualWielding(); got != tc.wantWide {
				t.Fatalf("IsDualWielding = %v, want %v - the split-frame gate reads this", got, tc.wantWide)
			}
		})
	}
}

// The compass is fully derived from the viewport: no config knob may claim to
// size it, and the readable band holds at every shipped resolution.
func TestCompassRadiusIsFullyDerivedFromViewport(t *testing.T) {
	g, _ := newThiefTestGame(t)
	ui := NewUISystem(g)
	for _, res := range []struct{ w, h int }{{800, 600}, {1024, 768}, {1920, 1080}, {3840, 2160}} {
		g.config.Display.ScreenWidth, g.config.Display.ScreenHeight = res.w, res.h
		got := ui.compassRadius()
		if got < compassMinRadius || got > compassMaxRadius {
			t.Fatalf("%dx%d compass radius = %d, want within %d..%d", res.w, res.h, got, compassMinRadius, compassMaxRadius)
		}
		want := min(max((min(res.w, res.h)*compassRadiusPercent+50)/100, compassMinRadius), compassMaxRadius)
		if got != want {
			t.Fatalf("%dx%d compass radius = %d, want %d (pure viewport derivation)", res.w, res.h, got, want)
		}
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

// HP/SP totals must stay fully readable at the shipped default resolution,
// where the compact stats column is about 60px (AGENTS.md HUD QA rule).
func TestMeterTextFitsCompactStatsColumn(t *testing.T) {
	g, _ := newThiefTestGame(t)
	g.config.Display.ScreenWidth = 1024
	g.config.Display.ScreenHeight = 768
	cardW, cardH, _, _ := partyPortraitLayout(g)
	_, _, panelW, _ := partyCardPanelRect(0, 0, cardW, cardH)
	statsW := makePartyCardContentLayout(0, 0, panelW).stats.w

	budget := statsW - 7
	for _, meter := range []struct {
		label            string
		current, maximum int
	}{
		{"HP", 33, 33}, {"HP", 100, 100}, {"SP", 100, 100}, {"HP", 1000, 1000},
	} {
		text, scale := meterText(budget, meter.label, meter.current, meter.maximum)
		if drawn := int(float64(debugTextWidth(text)) * scale); drawn > budget {
			t.Fatalf("%q draws %dpx into a %dpx box at scale %.3f - it would be clipped", text, drawn, budget, scale)
		}
		if scale < minReadableMeterScale || scale > 1 {
			t.Fatalf("%q scale = %.3f, want a legible [%.2f, 1] fit", text, scale, minReadableMeterScale)
		}
		// The default resolution must still afford the full labelled form.
		if want := fmt.Sprintf("%s %d/%d", meter.label, meter.current, meter.maximum); text != want && meter.maximum <= 100 {
			t.Fatalf("meter text = %q, want the full %q at the default resolution", text, want)
		}
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
	budget := makePartyCardContentLayout(0, 0, panelW).stats.w - 7

	text, scale := meterText(budget, "HP", 100, 100)
	if drawn := int(float64(debugTextWidth(text)) * scale); drawn > budget {
		t.Fatalf("%q draws %dpx into %dpx at %dx%d", text, drawn, budget, minW, minH)
	}
	if scale < minReadableMeterScale {
		t.Fatalf("%q scale = %.3f at %dx%d, want at least %.2f", text, scale, minW, minH, minReadableMeterScale)
	}
	if text == "" {
		t.Fatal("meter text vanished at the minimum window size")
	}
	t.Logf("minimum window %dx%d: budget=%dpx text=%q scale=%.2f", minW, minH, budget, text, scale)
}

// Utility status icons are authored 24x24 and must be drawn at native size.
func TestUtilityStatusIconUsesAuthoredSize(t *testing.T) {
	if utilityStatusIconSize != 24 {
		t.Fatalf("utility status icon size = %d, want the authored 24", utilityStatusIconSize)
	}
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
