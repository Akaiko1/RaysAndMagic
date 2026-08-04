package game

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Source-space measurements from party_member_panel.png. Every piece is drawn
// 1:1: the horizontal repeat extends a card without scaling its frame art, and
// the portrait sits on whole pixels inside the authored recess.
const (
	partyPanelSourceW       = partyCardPanelNativeWidth
	partyPanelSourceH       = partyCardPanelNativeHeight
	partyPanelLeftCapW      = 78
	partyPanelRightCapW     = 16
	partyPanelRepeatX       = 88
	partyPanelRepeatW       = 32
	partyPanelOrnamentX     = 120
	partyPanelOrnamentW     = 16
	panelPortraitX          = 17
	panelPortraitY          = 20
	panelPortraitW          = 47
	panelPortraitH          = 56
	partyFocusMarkerOffsetY = -11
	partyProgressBadgeSize  = 24
	partyProgressBadgeGap   = 2
	partyProgressBadgeLift  = 12
	partyAutoButtonMaxW     = 62
	partyAutoButtonH        = 16
	partyPanelContentLeft   = 83
	partyPanelContentRight  = 18
	partyPanelContentTop    = 18
	partyPanelContentBottom = 82
	// utilityStatusIconSize is the authored size of a utility status icon
	// (AGENTS.md, Icons). Drawing at native size avoids resampling.
	utilityStatusIconSize = 24
)

type partyCardTemplate struct {
	columnGap    int
	statsPercent int
}

// partyCardTemplateForWidth preserves the measured source safe area while
// giving compact, standard, and wide cards different column proportions. The
// portrait cap stays fixed; only the repeatable information field grows.
func partyCardTemplateForWidth(panelW int) partyCardTemplate {
	switch {
	case panelW < 300:
		return partyCardTemplate{columnGap: 5, statsPercent: 42}
	case panelW < 440:
		return partyCardTemplate{columnGap: 9, statsPercent: 40}
	default:
		return partyCardTemplate{columnGap: 12, statsPercent: 38}
	}
}

type partyCardContentLayout struct {
	box, stats, equipment layoutRect
}

type partyProgressionBadgeLayout struct {
	stat, skill layoutRect
}

func makePartyCardContentLayout(panelX, panelY, panelW int) partyCardContentLayout {
	template := partyCardTemplateForWidth(panelW)
	available := layoutRect{
		x: panelX + partyPanelContentLeft,
		y: panelY + partyPanelContentTop,
		w: max(1, panelW-partyPanelContentLeft-partyPanelContentRight),
		h: partyPanelContentBottom - partyPanelContentTop,
	}
	box := available
	statsW := max(1, (box.w-template.columnGap)*template.statsPercent/100)
	stats := layoutRect{x: box.x, y: box.y, w: statsW, h: box.h}
	equipmentX := stats.right() + template.columnGap
	equipment := layoutRect{x: equipmentX, y: box.y, w: max(1, box.right()-equipmentX), h: box.h}
	return partyCardContentLayout{box: box, stats: stats, equipment: equipment}
}

// makePartyProgressionBadgeLayout attaches pending-progression actions to the
// portrait that owns them. One badge centers; two straddle the lower rim with
// equal spacing and only a minimal overhang beyond the irregular aperture.
func makePartyProgressionBadgeLayout(px, py, pw, ph int, hasStat, hasSkill bool) partyProgressionBadgeLayout {
	count := 0
	if hasStat {
		count++
	}
	if hasSkill {
		count++
	}
	if count == 0 {
		return partyProgressionBadgeLayout{}
	}
	totalW := count*partyProgressBadgeSize + (count-1)*partyProgressBadgeGap
	x := px + (pw-totalW)/2
	y := py + ph - partyProgressBadgeLift
	next := func() layoutRect {
		r := layoutRect{x: x, y: y, w: partyProgressBadgeSize, h: partyProgressBadgeSize}
		x += partyProgressBadgeSize + partyProgressBadgeGap
		return r
	}
	layout := partyProgressionBadgeLayout{}
	if hasStat {
		layout.stat = next()
	}
	if hasSkill {
		layout.skill = next()
	}
	return layout
}

func makePartyAutoButtonLayout(content partyCardContentLayout) layoutRect {
	w := min(partyAutoButtonMaxW, content.equipment.w)
	return layoutRect{
		x: content.equipment.x + (content.equipment.w-w)/2,
		y: content.box.bottom() - partyAutoButtonH,
		w: w,
		h: partyAutoButtonH,
	}
}

// validatePartyCardPanelAsset fails the boot when party_member_panel.png is
// missing or no longer the authored 256x100 sheet. Every cap/repeat/ornament
// offset and the portrait aperture are measured from THAT sheet, so a resized
// or absent one would otherwise leave all four cards silently unpainted.
func (g *MMGame) validatePartyCardPanelAsset() {
	if g.sprites == nil {
		return
	}
	if _, err := os.Stat(filepath.Join("assets", "sprites")); err != nil {
		return // not running from the asset root (unit tests)
	}
	const name = "party_member_panel"
	panel := g.sprites.GetSprite(name)
	if panel == nil || !g.sprites.HasSprite(name) {
		panic(fmt.Sprintf("party HUD: %q not found under assets/sprites", name))
	}
	if b := panel.Bounds(); b.Dx() != partyPanelSourceW || b.Dy() != partyPanelSourceH {
		panic(fmt.Sprintf("party HUD: %q is %dx%d, want %dx%d - the card offsets and portrait aperture are measured from that size",
			name, b.Dx(), b.Dy(), partyPanelSourceW, partyPanelSourceH))
	}
}

// drawPartyPanel draws the authored 256x100 panel without scaling any source
// pixels. Fixed portrait and corner caps stay untouched; a neutral centre strip
// tiles horizontally and the single centre ornament is restored once. The size
// contract itself is enforced at boot by validatePartyCardPanelAsset.
func drawPartyPanel(screen, panel *ebiten.Image, x, y, w int) {
	if screen == nil || panel == nil || w <= partyPanelLeftCapW+partyPanelRightCapW {
		return
	}
	b := panel.Bounds()
	if b.Dx() != partyPanelSourceW || b.Dy() != partyPanelSourceH {
		return
	}
	if w == partyPanelSourceW {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(x), float64(y))
		screen.DrawImage(panel, op)
		return
	}
	drawPart := func(srcX, srcW, dstX int) {
		part := panel.SubImage(image.Rect(b.Min.X+srcX, b.Min.Y, b.Min.X+srcX+srcW, b.Min.Y+partyPanelSourceH)).(*ebiten.Image)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(dstX), float64(y))
		screen.DrawImage(part, op)
	}

	middleX := x + partyPanelLeftCapW
	middleEnd := x + w - partyPanelRightCapW
	for dstX := middleX; dstX < middleEnd; {
		pieceW := min(partyPanelRepeatW, middleEnd-dstX)
		drawPart(partyPanelRepeatX, pieceW, dstX)
		dstX += pieceW
	}
	drawPart(0, partyPanelLeftCapW, x)
	drawPart(partyPanelSourceW-partyPanelRightCapW, partyPanelRightCapW, x+w-partyPanelRightCapW)

	ornamentX := x + (w-partyPanelOrnamentW)/2
	if ornamentX >= middleX && ornamentX+partyPanelOrnamentW <= middleEnd {
		drawPart(partyPanelOrnamentX, partyPanelOrnamentW, ornamentX)
	}
}

// hudWhiteImg is a 1x1 opaque source for triangle fills (corner bevel cuts).
var hudWhiteImg = func() *ebiten.Image {
	img := ebiten.NewImage(1, 1)
	img.Fill(color.White)
	return img
}()

// partyCooldownMode is which hand a single-frame readout belongs to, and thus
// which tracked history measures its fill.
type partyCooldownMode uint8

const (
	partyCooldownModeMain partyCooldownMode = iota
	partyCooldownModeOffHand
)

// partyCooldownVisualState remembers each hand's cooldown peak so a frame fills
// smoothly. A hand that is not being read gets its history cleared, so switching
// readouts (a weapon unequipped mid-cooldown) can never measure one hand's
// countdown against the other's remembered peak.
type partyCooldownVisualState struct {
	peakRemaining    int
	lastRemaining    int
	offPeakRemaining int
	offLastRemaining int
}

func (ui *UISystem) partyCooldownStateFor(member *character.MMCharacter) partyCooldownVisualState {
	if ui.partyCooldownState == nil {
		ui.partyCooldownState = make(map[*character.MMCharacter]partyCooldownVisualState)
	}
	return ui.partyCooldownState[member]
}

func trackedPartyCooldownProgress(remaining int, peakRemaining, lastRemaining *int) float64 {
	if remaining <= 0 {
		*peakRemaining = 0
		*lastRemaining = 0
		return 1
	}
	// A fresh cooldown, or a re-arm mid-countdown, sets the peak the bar fills
	// from. Otherwise remaining only decays, so the peak already covers it.
	if *peakRemaining <= 0 || remaining > *lastRemaining {
		*peakRemaining = remaining
	}
	*lastRemaining = remaining
	progress := 1 - float64(remaining)/float64(*peakRemaining)
	return max(0.0, min(1.0, progress))
}

// partyCooldownProgress tracks ONE readout, filling from the history of the hand
// that readout belongs to (see partySingleHandCooldown).
func (ui *UISystem) partyCooldownProgress(member *character.MMCharacter, readout partyCooldownReadout) (remaining int, progress float64, active bool) {
	if readout.remaining <= 0 {
		delete(ui.partyCooldownState, member)
		return 0, 0, false
	}
	state := ui.partyCooldownStateFor(member)
	peak, last := &state.peakRemaining, &state.lastRemaining
	idlePeak, idleLast := &state.offPeakRemaining, &state.offLastRemaining
	if readout.mode == partyCooldownModeOffHand {
		peak, last, idlePeak, idleLast = idlePeak, idleLast, peak, last
	}
	progress = trackedPartyCooldownProgress(readout.remaining, peak, last)
	// The hand nobody reads keeps no peak, so it cannot skew a later readout.
	*idlePeak, *idleLast = 0, 0
	ui.partyCooldownState[member] = state
	return readout.remaining, progress, true
}

// partyCooldownReadout is the countdown a one-frame card shows plus the hand it
// belongs to, so the fill is always measured against that hand's own peak.
type partyCooldownReadout struct {
	remaining int
	mode      partyCooldownMode
}

// partySingleHandCooldown is the readout a one-frame card shows: the timer of
// the hand that actually attacks. Neither hand's countdown may leak into the
// other's readout - an unequipped weapon leaves its timer behind, and showing it
// would keep the frame up while the remaining hand is already free to strike.
func partySingleHandCooldown(member *character.MMCharacter) partyCooldownReadout {
	if member == nil {
		return partyCooldownReadout{}
	}
	switch {
	case member.MainHandArmed():
		// Main-hand attacks (and spells) run on RTCooldown. A real second weapon
		// is handled by the split readout instead of this one.
		return partyCooldownReadout{remaining: member.RTCooldown, mode: partyCooldownModeMain}
	case member.IsDualWielding():
		// Only the off-hand carries a weapon, so only its timer governs the card.
		return partyCooldownReadout{remaining: member.OffHandRTCooldown, mode: partyCooldownModeOffHand}
	default:
		// No weapon in either hand: RTCooldown is the spell cooldown.
		return partyCooldownReadout{remaining: member.RTCooldown, mode: partyCooldownModeMain}
	}
}

// partyArmsMasterCooldownProgress tracks the two weapon hands separately. A
// ready hand is fully colored while the other is cooling; once both are ready,
// the whole transient cooldown frame disappears.
func (ui *UISystem) partyArmsMasterCooldownProgress(member *character.MMCharacter) (mainProgress, offProgress float64, active bool) {
	if member == nil || (member.RTCooldown <= 0 && member.OffHandRTCooldown <= 0) {
		delete(ui.partyCooldownState, member)
		return 0, 0, false
	}
	state := ui.partyCooldownStateFor(member)
	mainProgress = trackedPartyCooldownProgress(member.RTCooldown, &state.peakRemaining, &state.lastRemaining)
	offProgress = trackedPartyCooldownProgress(member.OffHandRTCooldown, &state.offPeakRemaining, &state.offLastRemaining)
	ui.partyCooldownState[member] = state
	return mainProgress, offProgress, true
}

type partyCooldownEdge struct{ ax, ay, bx, by float32 }

func drawPartyCooldownProgress(screen *ebiten.Image, edges []partyCooldownEdge, progress float64, col color.RGBA) {
	totalLength := 0.0
	for _, edge := range edges {
		totalLength += math.Hypot(float64(edge.bx-edge.ax), float64(edge.by-edge.ay))
	}
	left := totalLength * max(0.0, min(1.0, progress))
	for _, edge := range edges {
		length := math.Hypot(float64(edge.bx-edge.ax), float64(edge.by-edge.ay))
		if left <= 0 {
			break
		}
		fraction := min(1.0, left/length)
		ex := edge.ax + (edge.bx-edge.ax)*float32(fraction)
		ey := edge.ay + (edge.by-edge.ay)*float32(fraction)
		vector.StrokeLine(screen, edge.ax, edge.ay, ex, ey, 2, col, false)
		left -= length
	}
}

func drawPartyCooldownFrame(screen *ebiten.Image, x, y, w, h int, progress float64) {
	x1, y1 := float32(x), float32(y)
	x2, y2 := float32(x+w-1), float32(y+h-1)
	if x2 <= x1 || y2 <= y1 {
		return
	}
	gray := color.RGBA{104, 112, 123, 235}
	green := color.RGBA{66, 218, 116, 250}
	vector.StrokeRect(screen, x1, y1, x2-x1, y2-y1, 1.5, gray, false)
	edges := [...]partyCooldownEdge{
		{x1, y1, x2, y1},
		{x2, y1, x2, y2},
		{x2, y2, x1, y2},
		{x1, y2, x1, y1},
	}
	drawPartyCooldownProgress(screen, edges[:], progress, green)
}

func drawPartyArmsMasterCooldownFrame(screen *ebiten.Image, x, y, w, h int, mainProgress, offProgress float64) {
	x1, y1 := float32(x), float32(y)
	x2, y2 := float32(x+w-1), float32(y+h-1)
	if x2 <= x1 || y2 <= y1 {
		return
	}
	midY := (y1 + y2) / 2
	gray := color.RGBA{104, 112, 123, 235}
	mainGreen := color.RGBA{66, 218, 116, 250}
	offBlue := color.RGBA{66, 154, 235, 250}
	vector.StrokeRect(screen, x1, y1, x2-x1, y2-y1, 1.5, gray, false)

	// Both paths travel from the left midpoint to the right midpoint, keeping
	// the main-hand readout wholly above the off-hand readout.
	mainEdges := [...]partyCooldownEdge{
		{x1, midY, x1, y1},
		{x1, y1, x2, y1},
		{x2, y1, x2, midY},
	}
	offEdges := [...]partyCooldownEdge{
		{x1, midY, x1, y2},
		{x1, y2, x2, y2},
		{x2, y2, x2, midY},
	}
	drawPartyCooldownProgress(screen, mainEdges[:], mainProgress, mainGreen)
	drawPartyCooldownProgress(screen, offEdges[:], offProgress, offBlue)
}

func expandedPartyPanelRect(x, y, w, h, gap int) (int, int, int, int) {
	return x - gap, y - gap, w + gap*2, h + gap*2
}

func drawPartySolidFrame(screen *ebiten.Image, x, y, w, h int, thickness float32, col color.RGBA) {
	vector.StrokeRect(screen, float32(x), float32(y), float32(w-1), float32(h-1), thickness, col, false)
}

func drawPartyFocusMarker(screen *ebiten.Image, centerX, topY int) {
	drawTriangle := func(halfWidth, topOffset, tipOffset int, topCol, tipCol color.RGBA) {
		tr := float32(topCol.R) / 255
		tg := float32(topCol.G) / 255
		tb := float32(topCol.B) / 255
		ta := float32(topCol.A) / 255
		br := float32(tipCol.R) / 255
		bg := float32(tipCol.G) / 255
		bb := float32(tipCol.B) / 255
		ba := float32(tipCol.A) / 255
		verts := []ebiten.Vertex{
			{DstX: float32(centerX - halfWidth), DstY: float32(topY + topOffset), SrcX: 0.5, SrcY: 0.5, ColorR: tr, ColorG: tg, ColorB: tb, ColorA: ta},
			{DstX: float32(centerX + halfWidth), DstY: float32(topY + topOffset), SrcX: 0.5, SrcY: 0.5, ColorR: tr, ColorG: tg, ColorB: tb, ColorA: ta},
			{DstX: float32(centerX), DstY: float32(topY + tipOffset), SrcX: 0.5, SrcY: 0.5, ColorR: br, ColorG: bg, ColorB: bb, ColorA: ba},
		}
		screen.DrawTriangles(verts, []uint16{0, 1, 2}, hudWhiteImg, nil)
	}

	// Broad dark rim, then a blue-steel body using the same highlight-to-shadow
	// ramp as rarity names. The thin top glint makes the tiny marker read as
	// beveled metal rather than a flat UI glyph.
	rim := color.RGBA{5, 18, 38, 245}
	drawTriangle(9, -2, 12, rim, rim)
	drawTriangle(7, 0, 9, metalShade(focusModeMetal, 0), metalShade(focusModeMetal, 1))
	vector.FillRect(screen, float32(centerX-5), float32(topY+1), 10, 1,
		color.RGBA{210, 240, 255, 230}, false)
}

// partyPortraitApertureSpan returns the exact opaque interval for one row of
// the authored portrait recess in party_member_panel.png. The interval is
// inclusive-exclusive in the mask's local coordinates.
func partyPortraitApertureSpan(row int) (start, end int) {
	switch {
	case row < 0 || row >= panelPortraitH:
		return 0, 0
	case row == 0:
		return 3, 43
	case row < 4:
		return 4 - row, 43 + row
	case row <= 51:
		return 0, panelPortraitW
	default:
		inset := row - 51
		return inset, panelPortraitW - inset
	}
}

func newPartyPortraitApertureMaskImage() *image.Alpha {
	mask := image.NewAlpha(image.Rect(0, 0, panelPortraitW, panelPortraitH))
	for row := 0; row < panelPortraitH; row++ {
		start, end := partyPortraitApertureSpan(row)
		for col := start; col < end; col++ {
			mask.SetAlpha(col, row, color.Alpha{A: 0xff})
		}
	}
	return mask
}

var partyPortraitApertureMask = ebiten.NewImageFromImage(newPartyPortraitApertureMaskImage())

// cardPortrait returns a cover-fitted portrait. Party-card portraits use the
// exact authored aperture mask; other callers keep an ordinary rectangular
// cover fit. Results are cached per name, size, and mask mode.
func (ui *UISystem) cardPortrait(name string, w, h int, usePartyAperture bool) *ebiten.Image {
	if w <= 0 || h <= 0 {
		return nil
	}
	key := fmt.Sprintf("%s|%dx%d|party-mask=%t", name, w, h, usePartyAperture)
	if img, ok := ui.cardPortraitCache[key]; ok {
		return img
	}
	src := ui.game.sprites.GetSprite(name)
	if src == nil {
		return nil
	}
	// HUD portraits already ship with their own one-pixel black card border.
	// The party panel supplies a second, irregular frame, so keeping that source
	// border creates a false empty seam even when the aperture mask is exact.
	// Unmasked callers outside this HUD intentionally preserve the source.
	if usePartyAperture && src.Bounds().Dx() > 2 && src.Bounds().Dy() > 2 {
		b := src.Bounds()
		src = src.SubImage(image.Rect(b.Min.X+1, b.Min.Y+1, b.Max.X-1, b.Max.Y-1)).(*ebiten.Image)
	}
	img := ebiten.NewImage(w, h)
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	scale := math.Max(float64(w)/float64(sw), float64(h)/float64(sh)) // cover-fit
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Scale(scale, scale)
	opts.GeoM.Translate((float64(w)-float64(sw)*scale)/2, (float64(h)-float64(sh)*scale)/2)
	if scale < 1 {
		opts.Filter = ebiten.FilterLinear // mipmapped shrink, no nearest mush
	}
	img.DrawImage(src, opts)

	if usePartyAperture {
		if w != panelPortraitW || h != panelPortraitH {
			return nil
		}
		maskOpts := &ebiten.DrawImageOptions{Blend: ebiten.BlendDestinationIn}
		img.DrawImage(partyPortraitApertureMask, maskOpts)
	}

	if ui.cardPortraitCache == nil {
		ui.cardPortraitCache = make(map[string]*ebiten.Image)
	}
	ui.cardPortraitCache[key] = img
	return img
}

// partyCardEffectSet is what a card would actually paint into its effect layer
// this frame. Every drawCard* call in the layer block below must be represented
// here, or the card silently loses that effect.
type partyCardEffectSet struct {
	poison, burn, stun, timed bool
}

func (s partyCardEffectSet) any() bool {
	return s.poison || s.burn || s.stun || s.timed
}

// layerCardFx are the timer-driven overlays that paint into the effect layer.
// fxBlink is absent on purpose: it tints the portrait directly.
var layerCardFx = [...]cardFx{fxFlame, fxSpark, fxHeal}

// anyTimedCardFxActive reports whether such an overlay is mid-animation.
func (g *MMGame) anyTimedCardFxActive(characterIndex int) bool {
	for _, fx := range layerCardFx {
		if g.cardFxActive(fx, characterIndex) > 0 {
			return true
		}
	}
	return false
}

// partyCardEffects returns the card's reusable particle layer, or nil when
// nothing would draw into it (needed=false) - an idle card then pays no
// render-target switch and no transparent blit.
func (ui *UISystem) partyCardEffects(index, w, h int, needed bool) *ebiten.Image {
	if !needed || index < 0 || index >= len(ui.partyCardEffectLayer) || w <= 0 || h <= 0 {
		return nil
	}
	img := ui.partyCardEffectLayer[index]
	if img == nil || img.Bounds().Dx() != w || img.Bounds().Dy() != h {
		img = ebiten.NewImage(w, h)
		ui.partyCardEffectLayer[index] = img
	}
	img.Clear()
	return img
}

func scaledDebugTextWidth(text string, scale float64) int {
	return int(math.Ceil(float64(debugTextWidth(text)) * scale))
}

func fittedDebugTextScale(maxW int, texts ...string) float64 {
	if maxW <= 0 {
		return 1
	}
	widest := 0
	for _, text := range texts {
		widest = max(widest, debugTextWidth(text))
	}
	if widest <= maxW || widest == 0 {
		return 1
	}
	return max(0.78, float64(maxW)/float64(widest))
}

func drawScaledLeftDebugText(screen *ebiten.Image, text string, x, y, maxW int, scale float64, col color.Color) {
	if text == "" || maxW <= 0 || scale <= 0 {
		return
	}
	text = clipDebugText(text, int(float64(maxW)/scale))
	img := outlinedLabelImage(text, col)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(float64(x)-scale, float64(y)-scale)
	screen.DrawImage(img, op)
}

func drawPartyMeter(screen *ebiten.Image, x, y, w, h, current, maximum int, label string, fill color.RGBA, textScale float64) {
	if w <= 0 || h <= 0 {
		return
	}
	vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), color.RGBA{3, 5, 9, 230}, false)
	innerW := max(0, w-2)
	if maximum > 0 && current > 0 {
		value := min(current, maximum)
		filled := innerW * value / maximum
		vector.FillRect(screen, float32(x+1), float32(y+1), float32(filled), float32(max(0, h-2)), fill, false)
		if filled > 0 {
			vector.FillRect(screen, float32(x+1), float32(y+1), float32(filled), 2, metalShade(fill, 0), false)
		}
	}
	vector.StrokeRect(screen, float32(x), float32(y), float32(w), float32(h), 1, color.RGBA{118, 125, 145, 220}, false)
	// An HP/SP readout must never be clipped - a truncated "HP 100/1.." hides the
	// number the player is reading. The card's shared scale is only the ceiling.
	textW := w - 7
	text, fitted := meterText(textW, label, current, maximum)
	if fitted < textScale {
		textScale = fitted
	}
	drawScaledLeftDebugText(screen, text, x+4,
		y+(h-int(float64(debugTextCharHeight)*textScale))/2, textW, textScale, color.White)
}

// minReadableMeterScale is how far a meter may shrink before dropping detail
// instead: below this the fixed-width glyphs stop being legible.
const minReadableMeterScale = 0.8

// meterText picks the most informative readout that still fits its box legibly:
// the full "HP 33/33", else "33/33", else the current value alone. The last form
// is always scaled to fit, so a meter can be terse but never clipped.
func meterText(maxW int, label string, current, maximum int) (string, float64) {
	forms := [...]string{
		fmt.Sprintf("%s %d/%d", label, current, maximum),
		fmt.Sprintf("%d/%d", current, maximum),
		fmt.Sprintf("%d", current),
	}
	for _, form := range forms {
		if scale := meterTextScale(maxW, form); scale >= minReadableMeterScale {
			return form, scale
		}
	}
	shortest := forms[len(forms)-1]
	return shortest, meterTextScale(maxW, shortest)
}

// meterTextScale fits one meter string to its box. Unlike fittedDebugTextScale it
// has no readability floor that could still clip: a meter's numbers must fit at
// the shipped 1024-wide default, where the compact stats column is ~60px.
func meterTextScale(maxW int, text string) float64 {
	width := debugTextWidth(text)
	if maxW <= 0 || width <= maxW || width == 0 {
		return 1
	}
	return float64(maxW) / float64(width)
}

func centeredIconRowX(barX, barW, iconSize, gap, count int) int {
	contentW := count*iconSize + max(0, count-1)*gap
	return barX + (barW-contentW)/2
}

// drawGameplayUI draws core gameplay UI elements
func (ui *UISystem) drawGameplayUI(screen *ebiten.Image) {
	ui.drawPartyUI(screen)
	ui.drawInGameQuickSlots(screen)
	ui.drawSpellStatusBar(screen)
	ui.drawCompass(screen)
	ui.drawWizardEyeRadar(screen)
	ui.drawCombatMessages(screen)
	ui.drawTurnBasedStatus(screen)
	ui.drawInteractionNotification(screen)
}

// drawDebugInfo draws debug and information elements
func (ui *UISystem) drawDebugInfo(screen *ebiten.Image) {
	ui.drawInstructions(screen)
	if ui.game.showFPS {
		ui.drawFPSCounter(screen)
	}
}

// drawPartyUI draws the party member portraits and stats at the bottom of the screen
func (ui *UISystem) drawPartyUI(screen *ebiten.Image) {
	if !ui.game.showPartyStats {
		return
	}

	portraitWidth, portraitHeight, baseLeft, startY := partyPortraitLayout(ui.game)
	vector.FillRect(screen, 0, float32(startY), float32(ui.game.config.GetScreenWidth()), float32(portraitHeight), color.RGBA{3, 5, 10, 246}, false)
	vector.FillRect(screen, 0, float32(startY), float32(ui.game.config.GetScreenWidth()), 1, color.RGBA{104, 114, 128, 235}, false)
	vector.FillRect(screen, 0, float32(startY+1), float32(ui.game.config.GetScreenWidth()), 1, color.RGBA{38, 44, 53, 245}, false)

	for i, member := range ui.game.party.Members {
		x := baseLeft + i*portraitWidth
		selected := i == ui.game.selectedChar
		panelX, panelY, panelW, panelH := partyCardPanelRect(x, startY, portraitWidth, portraitHeight)

		// In turn-based mode an
		// alive character that has already spent all their action slots gets
		// a gray frame so the player can see at a glance who still has a
		// move left. KO characters are skipped - they get no frame at all.
		// Dual Wielding gets an amber state: one weapon already used
		// this round but the other is still available - distinct from "fully
		// spent" gray, so the player can tell "acted once" from "acted twice".
		highlightColor := color.RGBA{0, 0, 0, 0}
		grayBusy := color.RGBA{120, 120, 120, 220}
		amberOneHandBusy := color.RGBA{225, 205, 40, 220}
		switch {
		case ui.game.turnBasedMode && member.CanAct() && member.IsDualWielding():
			switch member.ActionsRemaining {
			case 0:
				highlightColor = grayBusy
			case 1:
				highlightColor = amberOneHandBusy
			}
		case ui.game.turnBasedMode && member.CanAct() && member.ActionsRemaining == 0:
			highlightColor = grayBusy
		}

		panel := ui.game.sprites.GetSprite("party_member_panel")
		drawPartyPanel(screen, panel, panelX, panelY, panelW)

		// The promotion-aware portrait follows the measured painted recess exactly.
		// It is neither stretched nor resized with the viewport.
		portraitName := ui.game.portraitSpriteName(member)
		px := panelX + panelPortraitX
		py := panelY + panelPortraitY
		pw := panelPortraitW
		ph := panelPortraitH
		portraitColWidth := partyPanelContentLeft

		portraitOpts := &ebiten.DrawImageOptions{}
		portraitOpts.GeoM.Translate(float64(px), float64(py))
		// Apply red tint if character is blinking from damage
		if ui.game.IsCharacterBlinking(i) {
			portraitOpts.ColorScale.Scale(1.5, 0.5, 0.5, 1.0) // Red tint: more red, less green/blue
		}
		if card := ui.cardPortrait(portraitName, pw, ph, true); card != nil {
			screen.DrawImage(card, portraitOpts)
		}

		// Darken overlay if unconscious
		isUnconscious := false
		isPoisoned := false
		isBurning := false
		for _, cond := range member.Conditions {
			if cond == character.ConditionUnconscious {
				isUnconscious = true
			}
			if cond == character.ConditionPoisoned {
				isPoisoned = true
			}
			if cond == character.ConditionBurning {
				isBurning = true
			}
		}
		if isUnconscious {
			vector.FillRect(screen, float32(panelX), float32(panelY), float32(panelW), float32(panelH), color.RGBA{0, 0, 0, 140}, false)
		}
		// Status and feedback particles keep their full-card choreography, but a
		// reused layer clips them to the painted panel so they never cross the
		// reserved cooldown/selection frames. The layer costs a render-target
		// switch plus a full-card blit every frame, so an idle card skips it.
		fx := partyCardEffectSet{
			poison: isPoisoned && !isUnconscious,
			burn:   isBurning && !isUnconscious,
			stun:   member.IsStunned() && !isUnconscious,
			timed:  ui.game.anyTimedCardFxActive(i),
		}
		if effects := ui.partyCardEffects(i, panelW, panelH, fx.any()); effects != nil {
			if fx.poison {
				ui.drawCardPoisonBubbles(effects, 0, 0, panelW, panelH)
			}
			if fx.burn {
				ui.drawCardIgnite(effects, 0, 0, panelW, panelH, i)
			}
			if fx.stun {
				ui.drawCardStunStars(effects, 0, 0, portraitColWidth, panelH)
			}
			ui.drawCardFlames(effects, 0, 0, panelW, panelH, i)
			ui.drawCardSparks(effects, 0, 0, panelW, panelH, i)
			ui.drawCardHealPlus(effects, 0, 0, panelW, panelH, i)
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(panelX), float64(panelY))
			screen.DrawImage(effects, op)
		}

		content := makePartyCardContentLayout(panelX, panelY, panelW)
		contentX := content.box.x
		statsW := content.stats.w
		equipX := content.equipment.x
		equipW := content.equipment.w
		textScale := 1.0
		nameY := content.box.y
		levelText := fmt.Sprintf("L%d", member.Level)
		nameScale := fittedDebugTextScale(statsW, member.Name+" "+levelText)
		levelW := scaledDebugTextWidth(levelText, nameScale)
		nameText := clipDebugText(member.Name, max(1, int(float64(statsW)/nameScale)-debugTextWidth(levelText)-5))
		nameW := scaledDebugTextWidth(nameText, nameScale)
		nameX := contentX
		nameColor := raritySilver
		if i == ui.game.selectedChar {
			nameColor = rarityGold
		}
		drawScaledLeftDebugText(screen, nameText, nameX, nameY, nameW, nameScale, nameColor)
		drawScaledLeftDebugText(screen, levelText, nameX+nameW+5, nameY, levelW, nameScale, color.RGBA{175, 190, 215, 255})

		meterH := 14
		hpY := panelY + 35
		spY := panelY + 51
		drawPartyMeter(screen, contentX, hpY, statsW, meterH,
			member.HitPoints, member.MaxHitPoints, "HP", color.RGBA{156, 42, 48, 245}, textScale)
		drawPartyMeter(screen, contentX, spY, statsW, meterH,
			member.SpellPoints, member.MaxSpellPoints, "SP", color.RGBA{38, 88, 160, 245}, textScale)

		// Add character condition status
		statusText := "OK"
		if len(member.Conditions) > 0 {
			conds := make([]string, 0, len(member.Conditions))
			for _, cond := range member.Conditions {
				conds = append(conds, cond.String())
			}
			statusText = strings.Join(conds, ", ")
		}
		statusColor := color.RGBA{126, 220, 154, 255}
		if statusText != "OK" {
			statusColor = color.RGBA{255, 174, 88, 255}
		}
		statusY := panelY + 67
		statusScale := fittedDebugTextScale(statsW, statusText)
		statusW := scaledDebugTextWidth(statusText, statusScale)
		statusX := contentX + max(0, (statsW-statusW)/2)
		drawScaledLeftDebugText(screen, statusText, statusX, statusY, statsW, statusScale, statusColor)

		mainText, mainColor := "W None", color.Color(color.RGBA{135, 143, 158, 255})
		if weapon, ok := member.Equipment[items.SlotMainHand]; ok {
			mainText, mainColor = "W "+weapon.Name, ui.itemRarityColor(weapon)
		}
		spellText, spellColor := "S None", color.Color(color.RGBA{135, 143, 158, 255})
		if spell, ok := member.Equipment[items.SlotSpell]; ok {
			spellText, spellColor = "S "+spell.Name, color.RGBA{145, 192, 255, 255}
		}
		offText, offColor := "O None", color.Color(color.RGBA{135, 143, 158, 255})
		if offItem, ok := member.Equipment[items.SlotOffHand]; ok {
			offText, offColor = "O "+offItem.Name, ui.itemRarityColor(offItem)
		}
		equipmentScale := fittedDebugTextScale(equipW, mainText, offText, spellText)
		equipmentBlockW := max(
			scaledDebugTextWidth(mainText, equipmentScale),
			scaledDebugTextWidth(offText, equipmentScale),
			scaledDebugTextWidth(spellText, equipmentScale),
		)
		equipmentTextX := equipX + max(0, (equipW-equipmentBlockW)/2)
		drawScaledLeftDebugText(screen, mainText, equipmentTextX, nameY, equipW, equipmentScale, mainColor)
		drawScaledLeftDebugText(screen, offText, equipmentTextX, nameY+16, equipW, equipmentScale, offColor)
		drawScaledLeftDebugText(screen, spellText, equipmentTextX, nameY+32, equipW, equipmentScale, spellColor)

		hasStatBadge := member.FreeStatPoints > 0
		hasSkillBadge := ui.game.hasLevelUpChoiceForChar(i)
		badges := makePartyProgressionBadgeLayout(px, py, pw, ph, hasStatBadge, hasSkillBadge)
		mouseX, mouseY := ebiten.CursorPosition()

		if hasStatBadge {
			statHover := isMouseHoveringBox(mouseX, mouseY, badges.stat.x, badges.stat.y, badges.stat.right(), badges.stat.bottom())
			ui.drawStatPointPlusButton(screen, badges.stat.x, badges.stat.y, badges.stat.w, badges.stat.h, statHover)
			if statHover {
				ui.queueTooltip([]string{fmt.Sprintf("%d stat points ready", member.FreeStatPoints), "Click to assign"}, mouseX+12, mouseY+8)
			}
			if ui.game.consumeLeftClickIn(badges.stat.x, badges.stat.y, badges.stat.right(), badges.stat.bottom()) {
				ui.game.statPopupOpen = true
				// Open the popup for THIS character. Don't touch selectedChar:
				// in turn-based mode it tracks whose turn it is, and hijacking it
				// made the popup show the active char instead of the clicked one.
				ui.game.statPopupCharIdx = i
				ui.justOpenedStatPopup = true
			}
		}

		if hasSkillBadge {
			skillHover := isMouseHoveringBox(mouseX, mouseY, badges.skill.x, badges.skill.y, badges.skill.right(), badges.skill.bottom())
			ui.drawSkillPointIndicator(screen, badges.skill.x, badges.skill.y, badges.skill.w, badges.skill.h, skillHover)
			if skillHover {
				ui.queueTooltip([]string{"Skill choice ready", "Click to choose"}, mouseX+12, mouseY+8)
			}
			if ui.game.consumeLeftClickIn(badges.skill.x, badges.skill.y, badges.skill.right(), badges.skill.bottom()) {
				ui.game.openLevelUpChoiceForChar(i)
			}
		}

		stateFrameActive := highlightColor.A > 0
		innerX, innerY, innerW, innerH := expandedPartyPanelRect(panelX, panelY, panelW, panelH, partyCardInnerFrameGap)
		if ui.game.turnBasedMode && stateFrameActive {
			drawPartySolidFrame(screen, innerX, innerY, innerW, innerH, 1.5, highlightColor)
		} else if !ui.game.turnBasedMode {
			// The split readout represents two ATTACKING hands, so both must
			// actually hold a weapon: an empty (or shield-bearing) off-hand has no
			// off-hand attack, and an empty main hand has no main-hand attack -
			// either way one half would sit permanently "ready".
			if member.MainHandArmed() && member.IsDualWielding() {
				if mainProgress, offProgress, active := ui.partyArmsMasterCooldownProgress(member); active {
					stateFrameActive = true
					drawPartyArmsMasterCooldownFrame(screen, innerX, innerY, innerW, innerH, mainProgress, offProgress)
				}
			} else {
				if _, progress, active := ui.partyCooldownProgress(member, partySingleHandCooldown(member)); active {
					stateFrameActive = true
					drawPartyCooldownFrame(screen, innerX, innerY, innerW, innerH, progress)
				}
			}
		}
		if selected {
			selectionGap := partyCardInnerFrameGap
			if stateFrameActive {
				selectionGap = partyCardOuterFrameGap
			}
			selectionX, selectionY, selectionW, selectionH := expandedPartyPanelRect(panelX, panelY, panelW, panelH, selectionGap)
			drawPartySolidFrame(screen, selectionX, selectionY, selectionW, selectionH, 1.5, color.RGBA{232, 190, 86, 245})
		}
		if ui.game.partyMemberFocused(i) {
			// Focus belongs to the portrait, not to the whole party slot. Its tip
			// deliberately overlaps the authored top rim so the marker reads as
			// attached to this character even when selection/cooldown frames exist.
			drawPartyFocusMarker(screen, px+pw/2, py+partyFocusMarkerOffsetY)
		}
	}

	hasFreeStats := false
	for _, member := range ui.game.party.Members {
		if member != nil && member.FreeStatPoints > 0 {
			hasFreeStats = true
			break
		}
	}
	if hasFreeStats {
		panelX, panelY, panelW, _ := partyCardPanelRect(baseLeft, startY, portraitWidth, portraitHeight)
		auto := makePartyAutoButtonLayout(makePartyCardContentLayout(panelX, panelY, panelW))
		mouseX, mouseY := ebiten.CursorPosition()
		autoHover := isMouseHoveringBox(mouseX, mouseY, auto.x, auto.y, auto.right(), auto.bottom())
		ui.drawAutoStatButton(screen, auto.x, auto.y, auto.w, auto.h, autoHover)
		if autoHover {
			ui.queueTooltip([]string{"Auto-assign party stats", "Click to spend all available points"}, mouseX+12, mouseY+8)
		}
		if ui.game.consumeLeftClickIn(auto.x, auto.y, auto.right(), auto.bottom()) {
			autoDistributePartyStatPoints(ui.game.party.Members, ui.game.config)
		}
	}
}

// drawCardFlames draws rising flame-tongue particles over a party card while
// that member's Inferno scorch timer burns (set by TriggerPartyFlame). Each
// tongue rises on its own phase, flickers sideways, and shifts yellow->red and
// fades as it climbs; the whole effect dims as the timer runs out.
func (ui *UISystem) drawCardFlames(screen *ebiten.Image, x, startY, w, h, idx int) {
	t := ui.game.cardFxActive(fxFlame, idx)
	if t <= 0 {
		return
	}
	intensity := float64(t) / float64(PartyFlameFrames) // 1 -> 0 overall fade
	f := int(ui.game.frameCount)
	const n = 14
	for k := 0; k < n; k++ {
		phase := float64((f*2+k*53)%60) / 60.0 // 0..1 rising cycle, staggered per tongue
		rise := 1.0 - phase                    // brightness/heat fade as it climbs
		a := uint8(220 * rise * intensity)
		if a < 8 {
			continue
		}
		px := float64(x) + (float64(k)+0.5)/float64(n)*float64(w) + math.Sin(float64(f)*0.2+float64(k))*3
		py := float64(startY+h) - phase*float64(h)*1.05 // from card bottom up past the top
		sz := float32(3 + 3*rise)
		col := color.RGBA{255, uint8(40 + 190*rise), uint8(40 * rise), a} // yellow->orange->red
		vector.FillRect(screen, float32(px)-sz/2, float32(py)-sz/2, sz, sz, col, false)
	}
}

// drawCardSparks draws the hit feedback on a party card after the member takes a
// hit (fxSpark, set by TriggerDamageHit): the WHOLE card flashes
// red, plus a big radial spark burst flies outward - both fading over the timer.
func (ui *UISystem) drawCardSparks(screen *ebiten.Image, x, startY, w, h, idx int) {
	t := ui.game.cardFxActive(fxSpark, idx)
	if t <= 0 {
		return
	}
	intensity := float64(t) / float64(HitSparkFrames) // 1 -> 0 fade
	prog := 1.0 - intensity                           // 0 -> 1 as sparks fly out

	// Whole-card red flash (not just the portrait).
	vector.FillRect(screen, float32(x), float32(startY), float32(w-2), float32(h),
		color.RGBA{225, 40, 40, uint8(150 * intensity)}, false)

	// Big radial spark burst from the card centre.
	cx := float64(x) + float64(w)/2
	cy := float64(startY) + float64(h)/2
	maxR := float64(h) * 0.8
	const n = 16
	for k := 0; k < n; k++ {
		ang := 2*math.Pi*float64(k)/float64(n) + 0.4
		dist := prog * maxR
		px := cx + math.Cos(ang)*dist
		py := cy + math.Sin(ang)*dist*0.8
		a := uint8(245 * intensity)
		if a < 10 {
			continue
		}
		sz := float32(6*intensity + 2)
		col := color.RGBA{255, uint8(160 + 80*intensity), uint8(150 * intensity), a} // hot white-gold, fading
		vector.FillRect(screen, float32(px)-sz/2, float32(py)-sz/2, sz, sz, col, false)
	}
}

// drawCardHealPlus draws green "+" glyphs rising and evaporating up a member's
// card when they're healed (fxHeal, set by TriggerPartyHeal).
func (ui *UISystem) drawCardHealPlus(screen *ebiten.Image, x, startY, w, h, idx int) {
	t := ui.game.cardFxActive(fxHeal, idx)
	if t <= 0 {
		return
	}
	prog := 1.0 - float64(t)/float64(HealEffectFrames) // 0 -> 1 as they rise & fade
	const n = 5
	for k := 0; k < n; k++ {
		// Each "+" rises on its own staggered phase so they don't move in lockstep.
		ph := prog + float64(k)*0.13
		if ph > 1 {
			ph -= 1
		}
		px := float64(x) + (float64(k)+0.5)/float64(n)*float64(w) + math.Sin(ph*6+float64(k))*4
		py := float64(startY+h) - 6 - ph*(float64(h)-10) // rise from bottom toward top
		a := uint8(235 * (1 - ph))                       // evaporate as it climbs
		if a < 12 {
			continue
		}
		col := color.RGBA{90, 230, 110, a}
		arm := float32(4)                                               // half-length of the plus arms
		th := float32(2)                                                // arm thickness
		cx, cy := float32(px), float32(py)                              // centre
		vector.FillRect(screen, cx-arm, cy-th/2, arm*2, th, col, false) // horizontal bar
		vector.FillRect(screen, cx-th/2, cy-arm, th, arm*2, col, false) // vertical bar
	}
}

// drawCardPoisonBubbles draws green bubbles drifting up a poisoned member's card
// (replaces the old flat green tint). Runs continuously while poisoned.
func (ui *UISystem) drawCardPoisonBubbles(screen *ebiten.Image, x, startY, w, h int) {
	f := int(ui.game.frameCount)
	const n = 6
	const period = 72
	for k := 0; k < n; k++ {
		phase := float64((f+k*period/n)%period) / float64(period) // 0..1 rising loop
		bx := float64(x) + (float64(k)+0.5)/float64(n)*float64(w) + math.Sin(float64(f)*0.08+float64(k))*3
		by := float64(startY+h) - phase*float64(h)
		a := uint8(170 * (1 - phase)) // fade as it nears the top ("pops")
		if a < 12 {
			continue
		}
		r := float32(1.5 + 2.2*phase) // swells as it rises
		vector.FillCircle(screen, float32(bx), float32(by), r, color.RGBA{70, 210, 90, a}, true)
	}
}

// hashNoise is a cheap deterministic [0,1) hash - gives ignite its per-particle
// randomness without rand (so the flame is reproducible frame-to-frame, not pure
// flicker). seed blends a particle index with a per-card salt.
func hashNoise(seed float64) float64 {
	s := math.Sin(seed*127.1+311.7) * 43758.5453
	return s - math.Floor(s)
}

// drawCardIgnite draws a living, layered fire climbing a burning member's card:
// each tongue has its own randomized rise/lick cycle, three colour layers
// (dark-red glow -> orange body -> hot yellow-white core near the base) plus a few
// embers that float up and wink out. Built to read as real fire, not a recolour
// of the poison bubbles. Runs continuously while ConditionBurning.
func (ui *UISystem) drawCardIgnite(screen *ebiten.Image, x, startY, w, h, idx int) {
	f := float64(ui.game.frameCount)
	fx, fb, fw, fh := float64(x), float64(startY+h), float64(w), float64(h)
	salt := float64(idx) * 13.7

	// Flickering warm glow banked along the bottom of the card.
	glow := uint8(35 + 25*math.Sin(f*0.3+salt))
	vector.FillRect(screen, float32(x), float32(startY)+float32(fh*0.55), float32(w-2), float32(fh*0.45),
		color.RGBA{120, 40, 10, glow}, false)

	const n = 22
	for k := 0; k < n; k++ {
		seed := float64(k)*1.7 + salt
		life := hashNoise(seed)
		ph := math.Mod(f*0.02*(0.6+life*0.8)+life, 1.0) // 0..1 rising, varied speed
		rise := 1.0 - ph                                // heat fades with height
		col := hashNoise(seed * 3.3)
		wob := math.Sin(f*0.15+life*6.28+float64(k))*5*ph + (hashNoise(seed+f*0.01)-0.5)*4
		px := fx + col*fw + wob
		if px < fx || px > fx+fw {
			continue
		}
		py := fb - ph*fh*1.05 - 4
		base := float32(4 + 5*rise)
		if a := uint8(85 * rise); a > 8 { // outer red glow
			vector.FillCircle(screen, float32(px), float32(py), base*1.7, color.RGBA{200, 30, 0, a}, true)
		}
		if a := uint8(170 * rise); a > 8 { // orange body
			vector.FillCircle(screen, float32(px), float32(py), base, color.RGBA{255, uint8(40 + 120*rise), 0, a}, true)
		}
		if rise > 0.55 { // hot core, only near the base
			a := uint8(230 * (rise - 0.55) / 0.45)
			vector.FillCircle(screen, float32(px), float32(py), base*0.5, color.RGBA{255, 240, 170, a}, true)
		}
	}

	const embers = 6
	for k := 0; k < embers; k++ {
		seed := float64(k)*7.1 + salt
		ph := math.Mod(f*0.012+hashNoise(seed), 1.0)
		px := fx + hashNoise(seed*2.0)*fw + math.Sin(f*0.05+seed)*6
		py := fb - ph*fh*1.2
		a := uint8(200 * (1 - ph) * (1 - ph))
		if a < 12 {
			continue
		}
		vector.FillCircle(screen, float32(px), float32(py), float32(1+1.5*(1-ph)), color.RGBA{255, 200, 90, a}, true)
	}
}

// drawCardStunStars wheels a ring of twinkling four-point stars around a stunned
// member's portrait head - the classic "seeing stars" daze. Each star orbits,
// pulses in size/alpha on its own phase, and carries a faint diagonal sparkle.
func (ui *UISystem) drawCardStunStars(screen *ebiten.Image, x, startY, w, h int) {
	f := float64(ui.game.frameCount)
	cx := float64(x) + float64(w)*0.5
	cy := float64(startY) + float64(h)*0.30 // ring around the upper portrait (head)
	rx, ry := float64(w)*0.42, float64(h)*0.20
	const n = 5
	for k := 0; k < n; k++ {
		ang := f*0.06 + 2*math.Pi*float64(k)/float64(n)
		sx := float32(cx + math.Cos(ang)*rx)
		sy := float32(cy + math.Sin(ang)*ry)
		tw := 0.5 + 0.5*math.Sin(f*0.25+float64(k)*1.7) // twinkle
		a := uint8(120 + 135*tw)
		arm := float32(2.5 + 3.5*tw)
		col := color.RGBA{255, 240, 120, a}
		vector.StrokeLine(screen, sx-arm, sy, sx+arm, sy, 1.5, col, true)
		vector.StrokeLine(screen, sx, sy-arm, sx, sy+arm, 1.5, col, true)
		d := arm * 0.6
		spark := color.RGBA{255, 255, 200, uint8(a / 2)}
		vector.StrokeLine(screen, sx-d, sy-d, sx+d, sy+d, 1, spark, true)
		vector.StrokeLine(screen, sx-d, sy+d, sx+d, sy-d, 1, spark, true)
		vector.FillCircle(screen, sx, sy, 1.2, color.RGBA{255, 255, 230, a}, true)
	}
}

// drawPartyProgressionBadgeShadow lifts a badge off the portrait beneath it.
// It is a drop shadow only: the badge PNGs already carry their own dark button
// and gold rim, and compositing a second frame for one icon is forbidden (see
// AGENTS.md, Icons).
func drawPartyProgressionBadgeShadow(screen *ebiten.Image, x, y, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	vector.FillRect(screen, float32(x+2), float32(y+3), float32(max(0, w-3)), float32(max(0, h-3)), color.RGBA{0, 0, 0, 190}, false)
}

// drawPartyProgressionBadgeHover rings a badge OUTSIDE the authored art, after
// the icon is drawn, so the hover cue never reads as a second rim.
func drawPartyProgressionBadgeHover(screen *ebiten.Image, x, y, w, h int, base color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	vector.StrokeRect(screen, float32(x)-1, float32(y)-1, float32(w+1), float32(h+1), 1, metalShade(base, 0), false)
}

// drawStatPointPlusButton draws a portrait-attached progression badge. The
// available-point count lives in its tooltip so the icon remains one clean
// silhouette instead of looking like two adjacent badges.
func (ui *UISystem) drawStatPointPlusButton(screen *ebiten.Image, x, y, w, h int, isHover bool) {
	drawPartyProgressionBadgeShadow(screen, x, y, w, h)
	ui.drawInterfaceIcon(screen, "icon_stat_up", x, y, w, h)
	if isHover {
		drawPartyProgressionBadgeHover(screen, x, y, w, h, rarityEmerald)
	}
}

func (ui *UISystem) drawAutoStatButton(screen *ebiten.Image, x, y, w, h int, isHover bool) {
	if w <= 0 || h <= 0 {
		return
	}
	base := color.RGBA{18, 47, 78, 255}
	if isHover {
		base = color.RGBA{30, 82, 132, 255}
	}
	vector.FillRect(screen, float32(x+1), float32(y+2), float32(max(0, w-1)), float32(max(0, h-2)), color.RGBA{0, 0, 0, 190}, false)
	vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), color.RGBA{5, 10, 18, 245}, false)
	for row := 1; row < h-1; row++ {
		shade := metalShade(base, float64(row-1)/float64(max(1, h-3)))
		vector.FillRect(screen, float32(x+1), float32(y+row), float32(max(0, w-2)), 1, shade, false)
	}
	vector.StrokeRect(screen, float32(x), float32(y), float32(w-1), float32(h-1), 1, color.RGBA{26, 35, 48, 255}, false)
	vector.StrokeRect(screen, float32(x+1), float32(y+1), float32(max(0, w-3)), float32(max(0, h-3)), 1, metalShade(raritySilver, 0.6), false)
	vector.FillRect(screen, float32(x+3), float32(y+2), float32(max(0, w-6)), 1, color.RGBA{210, 235, 255, 180}, false)
	drawCenteredTextWithShadow(screen, "AUTO", x, y, w, h, raritySilver)
}

// drawSkillPointIndicator draws the ^ button for pending skill/spell choices.
func (ui *UISystem) drawSkillPointIndicator(screen *ebiten.Image, x, y, w, h int, isHover bool) {
	drawPartyProgressionBadgeShadow(screen, x, y, w, h)
	ui.drawInterfaceIcon(screen, "icon_level_choice", x, y, w, h)
	if isHover {
		drawPartyProgressionBadgeHover(screen, x, y, w, h, rarityGold)
	}
}

// drawSpellStatusBar draws active party effects on a compact rail directly
// above the responsive party deck.
func (ui *UISystem) drawSpellStatusBar(screen *ebiten.Image) {
	if !ui.game.showPartyStats {
		return
	}

	statuses := make([]*UtilitySpellStatus, 0, len(ui.game.utilitySpellStatuses))
	for _, status := range ui.game.utilitySpellStatuses {
		if status != nil && status.Duration > 0 {
			statuses = append(statuses, status)
		}
	}
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].SpellID < statuses[j].SpellID
	})
	if len(statuses) == 0 {
		return
	}

	_, _, _, partyStartY := partyPortraitLayout(ui.game)
	// Utility status icons are authored 24x24 (AGENTS.md, Icons) - drawn at their
	// native size they stay crisp and the compact rail holds the most effects.
	const iconSize = utilityStatusIconSize
	const iconGap = 5
	const barPadding = 4
	iconPitch := iconSize + iconGap
	barX := 10
	rightEdge := ui.game.config.GetScreenWidth() - 10
	if quickBar, visible := inGameQuickSlotBarLayout(ui.game); visible {
		rightEdge = quickBar.x - 10
	}
	if lines := ui.game.hudMessageLines(); len(lines) > 0 {
		messageX, _, _, _ := ui.game.hudMessageBlockRect(len(lines))
		rightEdge = min(rightEdge, messageX-10)
	}
	availableW := max(iconSize+barPadding*2, rightEdge-barX)
	iconsPerRow := max(1, (availableW-barPadding*2+iconGap)/iconPitch)
	rows := (len(statuses) + iconsPerRow - 1) / iconsPerRow
	barH := barPadding*2 + rows*iconSize + max(0, rows-1)*iconGap
	barY := partyStartY - barH - 18
	maxRowCount := min(iconsPerRow, len(statuses))
	barW := barPadding*2 + maxRowCount*iconSize + max(0, maxRowCount-1)*iconGap

	vector.FillRect(screen, float32(barX), float32(barY), float32(barW), float32(barH), color.RGBA{5, 9, 16, 222}, false)
	vector.FillRect(screen, float32(barX+2), float32(barY+2), float32(barW-4), 2, color.RGBA{88, 118, 158, 190}, false)
	vector.StrokeRect(screen, float32(barX), float32(barY), float32(barW), float32(barH), 1, color.RGBA{180, 147, 76, 235}, false)

	for i, status := range statuses {
		row := i / iconsPerRow
		col := i % iconsPerRow
		rowStart := row * iconsPerRow
		rowCount := min(iconsPerRow, len(statuses)-rowStart)
		iconX := centeredIconRowX(barX, barW, iconSize, iconGap, rowCount) + col*iconPitch
		iconY := barY + barPadding + row*iconPitch
		x, y, w, h := ui.drawSpellIcon(screen, iconX, iconY, iconSize, status.Icon, status.Fallback, status.Duration, status.MaxDuration)
		ui.handleSpellIconClick(x, y, w, h, status.SpellID)
		statusLabel := status.Label
		if statusLabel == "" {
			statusLabel = spellDisplayName(status.SpellID)
		}
		mouseX, mouseY := ebiten.CursorPosition()
		if isMouseHoveringBox(mouseX, mouseY, x, y, x+w, y+h) {
			seconds := float64(status.Duration) / float64(max(1, ui.game.config.GetTPS()))
			ui.queueTooltipIcon([]string{statusLabel, fmt.Sprintf("%.1fs remaining", seconds), "Double-click to dispel"}, status.Icon, mouseX+12, mouseY+8)
		}
	}
}

// drawSpellIcon draws a single spell status icon with duration bar and returns clickable bounds
func (ui *UISystem) drawSpellIcon(screen *ebiten.Image, x, y, size int, icon, fallback string, currentDuration, maxDuration int) (int, int, int, int) {
	vector.FillRect(screen, float32(x), float32(y), float32(size), float32(size), color.RGBA{4, 7, 12, 245}, false)
	vector.FillRect(screen, float32(x+2), float32(y+2), float32(size-4), float32(size-4), color.RGBA{22, 29, 42, 210}, false)

	if icon != "" {
		sprite := ui.game.sprites.GetSprite(icon)
		opts := &ebiten.DrawImageOptions{}
		opts.GeoM.Scale(float64(size)/float64(sprite.Bounds().Dx()), float64(size)/float64(sprite.Bounds().Dy()))
		opts.GeoM.Translate(float64(x), float64(y))
		// Linear (mipmapped) on the typical downscale keeps spell icons crisp.
		if size < sprite.Bounds().Dx() || size < sprite.Bounds().Dy() {
			opts.Filter = ebiten.FilterLinear
		}
		screen.DrawImage(sprite, opts)
	} else if fallback != "" {
		drawDebugText(screen, fallback, x+size/2-4, y+size/2-4)
	}
	vector.StrokeRect(screen, float32(x), float32(y), float32(size), float32(size), 1, color.RGBA{195, 162, 82, 245}, false)
	vector.StrokeRect(screen, float32(x+2), float32(y+2), float32(size-4), float32(size-4), 1, color.RGBA{82, 119, 164, 220}, false)

	// Draw duration bar at bottom of icon
	if maxDuration > 0 {
		barWidth := size
		barHeight := 3

		// Background bar (gray)
		vector.FillRect(screen, float32(x), float32(y+size-barHeight), float32(barWidth), float32(barHeight), color.RGBA{60, 60, 60, 200}, false)

		// Duration bar (colored based on remaining time)
		if currentDuration > 0 {
			fillWidth := int(float64(barWidth) * float64(currentDuration) / float64(maxDuration))
			if fillWidth > 0 {
				// Color changes from green to yellow to red as time runs out
				progress := float64(currentDuration) / float64(maxDuration)
				var barColor color.RGBA
				if progress > 0.6 {
					barColor = color.RGBA{0, 200, 0, 255} // Green
				} else if progress > 0.3 {
					barColor = color.RGBA{200, 200, 0, 255} // Yellow
				} else {
					barColor = color.RGBA{200, 100, 0, 255} // Orange-red
				}

				vector.FillRect(screen, float32(x), float32(y+size-barHeight), float32(fillWidth), float32(barHeight), barColor, false)
			}
		}
	}

	// Return clickable bounds (x, y, width, height)
	return x, y, size, size
}

// handleSpellIconClick handles mouse clicks on spell status icons for dispelling
func (ui *UISystem) handleSpellIconClick(x, y, width, height int, spellID spells.SpellID) {
	// Check for mouse click (only process on first press, not while held)
	if ui.game.consumeLeftClickIn(x, y, x+width, y+height) {
		currentTime := ui.game.mouseLeftClickAt

		// Check for a fast double-click on the same icon.
		doubleClick := withinDoubleClickWindow(currentTime, ui.game.lastUtilitySpellClickTime) &&
			ui.game.lastClickedUtilitySpell == string(spellID)
		if doubleClick {
			// Double-click detected - dispel the spell
			ui.dispelUtilitySpell(spellID)
			// Reset click tracking
			ui.game.lastUtilitySpellClickTime = 0
			ui.game.lastClickedUtilitySpell = ""
		} else {
			// Single click - record for potential double-click
			ui.game.lastUtilitySpellClickTime = currentTime
			ui.game.lastClickedUtilitySpell = string(spellID)
		}
	}
}

// dispelUtilitySpell removes an active utility spell effect by triggering natural expiration
func (ui *UISystem) dispelUtilitySpell(spellID spells.SpellID) {
	// Flag effects (torch / wizard eye / water): zero the duration and let the
	// next tick expire it naturally - onExpire side effects (e.g. the
	// underwater return teleport) fire exactly as on a normal timeout.
	for _, b := range ui.game.timedBuffs() {
		if b.id != spellID {
			continue
		}
		if *b.active {
			*b.duration = 0
			ui.game.AddCombatMessage(fmt.Sprintf("%s dispelled!", spellDisplayName(spellID)))
		}
		return
	}
	// Registry buffs (stat AND combat) dispel by spell id.
	if _, ok := ui.game.statBuffByID(string(spellID)); ok {
		ui.game.removeStatBuff(string(spellID))
		ui.game.AddCombatMessage(fmt.Sprintf("%s dispelled!", spellDisplayName(spellID)))
	} else if _, ok := ui.game.combatBuffByID(string(spellID)); ok {
		ui.game.removeCombatBuff(string(spellID))
		ui.game.AddCombatMessage(fmt.Sprintf("%s dispelled!", spellDisplayName(spellID)))
	}
}

// drawCompass draws the compass/direction indicator with minimap showing nearby tiles
func (ui *UISystem) drawCompass(screen *ebiten.Image) {
	compassX, compassY := ui.getCompassCenter()
	compassRadius := ui.compassRadius()

	vector.FillCircle(screen, float32(compassX+2), float32(compassY+3), float32(compassRadius+6), color.RGBA{0, 0, 0, 170}, true)
	vector.FillCircle(screen, float32(compassX), float32(compassY), float32(compassRadius+5), color.RGBA{66, 48, 24, 245}, true)
	vector.FillCircle(screen, float32(compassX), float32(compassY), float32(compassRadius+3), color.RGBA{194, 153, 66, 255}, true)
	vector.FillCircle(screen, float32(compassX), float32(compassY), float32(compassRadius), color.RGBA{8, 14, 23, 235}, true)

	ui.drawCompassMinimap(screen, compassX, compassY, compassRadius)

	vector.StrokeCircle(screen, float32(compassX), float32(compassY), float32(compassRadius), 2, color.RGBA{98, 140, 181, 245}, true)
	vector.StrokeCircle(screen, float32(compassX), float32(compassY), float32(compassRadius+4), 1, color.RGBA{255, 218, 115, 245}, true)

	// A single north-up map and a rotating player pointer avoid the ambiguity of
	// the old red line, which looked like either a heading or a target marker.
	angle := ui.game.camera.Angle
	tipRadius := float64(compassRadius - 9)
	tipX := float64(compassX) + math.Cos(angle)*tipRadius
	tipY := float64(compassY) + math.Sin(angle)*tipRadius
	rearX := float64(compassX) - math.Cos(angle)*5
	rearY := float64(compassY) - math.Sin(angle)*5
	perpX := -math.Sin(angle) * 5
	perpY := math.Cos(angle) * 5
	verts := []ebiten.Vertex{
		{DstX: float32(tipX), DstY: float32(tipY), SrcX: 0.5, SrcY: 0.5, ColorR: 0.35, ColorG: 0.85, ColorB: 1, ColorA: 1},
		{DstX: float32(rearX + perpX), DstY: float32(rearY + perpY), SrcX: 0.5, SrcY: 0.5, ColorR: 0.08, ColorG: 0.35, ColorB: 0.8, ColorA: 1},
		{DstX: float32(rearX - perpX), DstY: float32(rearY - perpY), SrcX: 0.5, SrcY: 0.5, ColorR: 0.08, ColorG: 0.35, ColorB: 0.8, ColorA: 1},
	}
	screen.DrawTriangles(verts, []uint16{0, 1, 2}, hudWhiteImg, nil)
	vector.StrokeLine(screen, float32(tipX), float32(tipY), float32(rearX+perpX), float32(rearY+perpY), 1, color.RGBA{215, 244, 255, 245}, true)
	vector.StrokeLine(screen, float32(tipX), float32(tipY), float32(rearX-perpX), float32(rearY-perpY), 1, color.RGBA{215, 244, 255, 245}, true)
	vector.FillCircle(screen, float32(compassX), float32(compassY), 4, color.RGBA{220, 245, 255, 255}, true)
	vector.FillCircle(screen, float32(compassX), float32(compassY), 2, color.RGBA{32, 124, 220, 255}, true)

	cardinalColor := color.RGBA{236, 214, 156, 255}
	drawDebugTextColored(screen, "N", compassX-3, compassY-compassRadius-17, rarityGold)
	drawDebugTextColored(screen, "E", compassX+compassRadius+8, compassY-8, cardinalColor)
	drawDebugTextColored(screen, "S", compassX-3, compassY+compassRadius+3, cardinalColor)
	drawDebugTextColored(screen, "W", compassX-compassRadius-14, compassY-8, cardinalColor)
}

// invalidateCompassTileLayer forces the next drawCompassMinimap call to
// rebuild the cached tile layer, even though the player is still on the same
// tile - needed when a quest swaps a tile out from under a standing player.
func (ui *UISystem) invalidateCompassTileLayer() {
	ui.compassCacheWorld = nil
}

// drawCompassMinimap renders the nearby tiles on the compass as a minimap.
// The tile layer is cached (see rebuildCompassTileLayer) and only rebuilt when
// the player crosses a tile boundary or the world changes; NPC dots move
// smoothly relative to the player, so they stay live (few per map).
func (ui *UISystem) drawCompassMinimap(screen *ebiten.Image, centerX, centerY, radius int) {
	if ui.game.world == nil {
		return
	}

	tileSize := ui.game.config.GetTileSize()
	playerTileX := TileIndex(ui.game.camera.X, tileSize)
	playerTileY := TileIndex(ui.game.camera.Y, tileSize)

	// Number of tiles to show in each direction from center
	viewRange := 6
	// Size of each minimap tile in pixels
	miniTileSize := float32(radius) / float32(viewRange+1)
	if miniTileSize < 3 {
		miniTileSize = 3
	}
	if miniTileSize > 8 {
		miniTileSize = 8
	}

	if ui.compassTileLayer == nil || ui.compassTileLayer.Bounds().Dx() != radius*2 ||
		ui.compassCacheWorld != ui.game.world ||
		ui.compassCacheTileX != playerTileX || ui.compassCacheTileY != playerTileY {
		ui.rebuildCompassTileLayer(playerTileX, playerTileY, viewRange, miniTileSize, radius)
	}

	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Translate(float64(centerX-radius), float64(centerY-radius))
	screen.DrawImage(ui.compassTileLayer, opts)

	// Draw NPCs on minimap
	for _, npc := range ui.game.world.NPCs {
		if ui.game.npcAbsent(npc) {
			continue
		}
		npcTileX := TileIndex(npc.X, tileSize)
		npcTileY := TileIndex(npc.Y, tileSize)
		dx := npcTileX - playerTileX
		dy := npcTileY - playerTileY

		// Only show NPCs within view range
		if dx*dx+dy*dy <= viewRange*viewRange {
			screenX := float32(centerX) + float32(dx)*miniTileSize
			screenY := float32(centerY) + float32(dy)*miniTileSize
			dotRadius := max(float32(2), miniTileSize/2)
			vector.FillCircle(screen, screenX, screenY, dotRadius+1, color.RGBA{8, 10, 14, 235}, true)
			vector.FillCircle(screen, screenX, screenY, dotRadius, color.RGBA{255, 210, 55, 255}, true)
		}
	}
}

// rebuildCompassTileLayer bakes the compass minimap's tile fills into a
// 2R x 2R layer image centered on the player's tile.
func (ui *UISystem) rebuildCompassTileLayer(playerTileX, playerTileY, viewRange int, miniTileSize float32, radius int) {
	side := 2 * radius
	if ui.compassTileLayer == nil || ui.compassTileLayer.Bounds().Dx() != side {
		ui.compassTileLayer = ebiten.NewImage(side, side)
	} else {
		ui.compassTileLayer.Clear()
	}
	ui.compassCacheWorld = ui.game.world
	ui.compassCacheTileX = playerTileX
	ui.compassCacheTileY = playerTileY

	center := float32(radius)
	for dy := -viewRange; dy <= viewRange; dy++ {
		for dx := -viewRange; dx <= viewRange; dx++ {
			tileX := playerTileX + dx
			tileY := playerTileY + dy

			// Skip tiles outside world bounds
			if tileX < 0 || tileX >= ui.game.world.Width || tileY < 0 || tileY >= ui.game.world.Height {
				continue
			}

			// Check if this tile is within the circular compass area
			if dx*dx+dy*dy > viewRange*viewRange {
				continue
			}

			// Get tile color based on type; the floor color follows the
			// tile's own region on the unified world.
			fc := ui.game.floorColorForTile(tileX, tileY, [3]int{60, 110, 60})
			floorColor := color.RGBA{uint8(fc[0]), uint8(fc[1]), uint8(fc[2]), 180}
			tile := ui.game.world.Tiles[tileY][tileX]
			tileColor := ui.getMinimapTileColor(tile, floorColor)

			// Draw the minimap tile (layer coords: player tile at the center)
			screenX := center + float32(dx)*miniTileSize
			screenY := center + float32(dy)*miniTileSize
			halfSize := miniTileSize / 2
			vector.FillRect(ui.compassTileLayer, screenX-halfSize, screenY-halfSize, miniTileSize, miniTileSize, tileColor, false)
		}
	}
}

// getMinimapTileColor returns the color for a tile type on the minimap
func (ui *UISystem) getMinimapTileColor(tile world.TileType3D, floorColor color.RGBA) color.RGBA {
	switch tile {
	case world.TileWall, world.TileTree, world.TileAncientTree, world.TileThicket, world.TileMossRock, world.TileLowWall, world.TileHighWall:
		return color.RGBA{29, 35, 44, 245} // Dark for walls/obstacles
	case world.TileWater:
		return color.RGBA{35, 102, 178, 235} // Blue for water
	case world.TileDeepWater:
		return color.RGBA{20, 54, 116, 240} // Darker blue for deep water
	case world.TileVioletTeleporter:
		return color.RGBA{181, 78, 222, 245} // Violet for teleporters
	case world.TileRedTeleporter:
		return color.RGBA{218, 68, 68, 245} // Red for teleporters
	case world.TileClearing:
		return color.RGBA{83, 151, 91, 225} // Lighter green for clearings
	default:
		floorColor.A = 220
		return floorColor
	}
}

// drawWizardEyeRadar draws enemy dots on the compass when wizard eye is active
func (ui *UISystem) drawWizardEyeRadar(screen *ebiten.Image) {
	if !ui.game.wizardEyeActive {
		return
	}

	compassX, compassY := ui.getCompassCenter()
	compassRadius := ui.compassRadius()

	// Convert tile distance to pixel distance
	tileSize := float64(ui.game.config.GetTileSize())
	radarTiles := ui.game.wizardEyeRadiusTiles
	if radarTiles <= 0 {
		radarTiles = 10 // legacy saves activated the eye before the radius was stored
	}
	maxRadarRange := radarTiles * tileSize

	// Check each monster for distance from player
	for _, monster := range ui.game.world.Monsters {
		if !monster.IsAlive() {
			continue
		}

		// Calculate distance from player
		dx := monster.X - ui.game.camera.X
		dy := monster.Y - ui.game.camera.Y
		dist := dx*dx + dy*dy // Use squared distance to avoid sqrt
		maxRangeSq := maxRadarRange * maxRadarRange

		// Only show enemies within the radar radius
		if dist <= maxRangeSq {
			// Plot at the actual relative position on the north-up minimap instead
			// of collapsing every threat onto the rim.
			radarScale := float64(compassRadius-8) / maxRadarRange
			dotX := compassX + int(dx*radarScale)
			dotY := compassY + int(dy*radarScale)

			// Select cached dot image based on distance for threat assessment
			// Using squared distances to avoid sqrt
			closeDistSq := (tileSize * 3) * (tileSize * 3)
			mediumDistSq := (tileSize * 6) * (tileSize * 6)

			var dotImg *ebiten.Image
			if dist < closeDistSq {
				dotImg = ui.radarDotClose // Red for close enemies
			} else if dist < mediumDistSq {
				dotImg = ui.radarDotMedium // Orange for medium distance
			} else {
				dotImg = ui.radarDotFar // Yellow for far enemies
			}

			// Draw cached dot image (much faster than vector.FillCircle)
			opts := &ebiten.DrawImageOptions{}
			opts.GeoM.Translate(float64(dotX-3), float64(dotY-3))
			screen.DrawImage(dotImg, opts)
		}
	}
}

const hudMessageSpacing = 18

// drawCombatMessages draws the compact recent-message log above the bottom HUD.
// Long entries word-wrap to the block width (see hudMessageLines); the block
// grows upward and reserves a visible quick bar when their spans meet.
func (ui *UISystem) drawCombatMessages(screen *ebiten.Image) {
	lines := ui.game.hudMessageLines()
	if len(lines) == 0 {
		return
	}

	bx, by, bw, bh := ui.game.hudMessageBlockRect(len(lines))
	vector.FillRect(screen, float32(bx), float32(by), float32(bw), float32(bh), color.RGBA{0, 0, 0, 150}, false)

	// Draw lines from top to bottom (most recent at bottom)
	for i, line := range lines {
		textY := by + 5 + (i * hudMessageSpacing)
		drawDebugTextColored(screen, line.Text, bx+5, textY, line.Color)
	}
}

func combatMessageArea(g *MMGame) (x, y, w, h int) {
	count := len(g.hudMessageLines())
	if count == 0 {
		return 0, 0, 0, 0
	}
	return g.hudMessageBlockRect(count)
}

func combatLogPanelLayout(g *MMGame) (x, y, w, h int) {
	w, h = 700, 640
	x = (g.config.GetScreenWidth() - w) / 2
	y = (g.config.GetScreenHeight() - h) / 2
	return
}

func (ui *UISystem) drawCombatLogOverlay(screen *ebiten.Image) {
	x, y, w, h := combatLogPanelLayout(ui.game)
	drawFilledRect(screen, 0, 0, ui.game.config.GetScreenWidth(), ui.game.config.GetScreenHeight(), color.RGBA{0, 0, 0, 150})
	ui.drawPatternFrame(screen, "menu_panel_frame", x, y, w, h, menuPanelFrameSlice)
	drawCenteredDebugText(screen, "GAME LOG", x, y+18, w, 20)

	closeX, closeY := x+w-30, y+8
	mouseX, mouseY := ebiten.CursorPosition()
	closeColor := color.RGBA{100, 100, 100, 180}
	if isMouseHoveringBox(mouseX, mouseY, closeX, closeY, closeX+20, closeY+20) {
		closeColor = color.RGBA{170, 60, 60, 220}
	}
	drawFilledRect(screen, closeX, closeY, 20, 20, closeColor)
	ui.drawInterfaceIcon(screen, "icon_close", closeX, closeY, 20, 20)

	contentX, contentY := x+28, y+54
	contentW, contentH := w-72, h-88
	drawFilledRect(screen, contentX, contentY, contentW, contentH, color.RGBA{8, 8, 18, 210})
	drawRectBorder(screen, contentX, contentY, contentW, contentH, 1, color.RGBA{100, 100, 145, 220})

	maxChars := (contentW - 24) / debugTextCharWidth
	rowY := contentY + contentH - 22
	entryIndex := len(ui.game.combatLogHistory) - 1 - ui.game.combatLogScroll
	for entryIndex >= 0 && rowY >= contentY+8 {
		entry := ui.game.combatLogHistory[entryIndex]
		lines := wrapText(entry.Text, maxChars)
		for i := len(lines) - 1; i >= 0 && rowY >= contentY+8; i-- {
			drawDebugTextColored(screen, lines[i], contentX+10, rowY, entry.Color)
			rowY -= 16
		}
		rowY -= 4
		entryIndex--
	}

	buttonX := x + w - 36
	for _, btn := range []struct {
		y     int
		label string
	}{
		{contentY + 8, "^"},
		{contentY + contentH - 30, "v"},
	} {
		drawFilledRect(screen, buttonX, btn.y, 22, 22, color.RGBA{65, 65, 95, 220})
		drawCenteredDebugText(screen, btn.label, buttonX, btn.y+2, 22, 18)
	}
	drawDebugTextColored(screen, "Mouse wheel / arrows to scroll", contentX, y+h-24, color.RGBA{180, 180, 190, 255})
}

// Translucent text-panel geometry shared by the turn-based status bar and the
// FPS/perf overlay.
const (
	textPanelLineHeight = 16
	textPanelPadding    = 6
)

// measureTextPanel returns the panel size fitting the given lines (7px-per-char
// width estimate).
func measureTextPanel(lines []string) (w, h int) {
	maxLen := 0
	for _, line := range lines {
		if len(line) > maxLen {
			maxLen = len(line)
		}
	}
	return maxLen*7 + textPanelPadding*2, len(lines)*textPanelLineHeight + textPanelPadding*2
}

// drawTurnBasedStatus displays the current game mode and turn state
func (ui *UISystem) drawTurnBasedStatus(screen *ebiten.Image) {
	lines, barX, barY, barWidth, barHeight := ui.turnBasedStatusLayout()

	vector.FillRect(screen, float32(barX), float32(barY), float32(barWidth), float32(barHeight), color.RGBA{5, 9, 16, 220}, false)
	vector.FillRect(screen, float32(barX+2), float32(barY+2), float32(barWidth-4), 2, color.RGBA{88, 125, 169, 210}, false)
	vector.StrokeRect(screen, float32(barX), float32(barY), float32(barWidth), float32(barHeight), 1, color.RGBA{188, 154, 78, 235}, false)

	for i, line := range lines {
		textColor := color.Color(color.White)
		if i == 0 {
			textColor = rarityGold
		}
		drawDebugTextColored(screen, line, barX+textPanelPadding, barY+textPanelPadding+i*textPanelLineHeight, textColor)
	}
}

func (ui *UISystem) turnBasedStatusLayout() ([]string, int, int, int, int) {
	mode := "REAL-TIME"
	if ui.game.turnBasedMode {
		mode = "TURN-BASED"
	}
	lines := []string{mode}
	if ui.game.turnBasedMode {
		turnText := "Party Turn"
		if ui.game.currentTurn == 1 {
			turnText = "Monster Turn"
		}
		lines = append(lines, turnText)
		if ui.game.currentTurn == 0 {
			lines = append(lines, fmt.Sprintf("Actions: %d/2", ui.game.partyActionsUsed))
		}
	}

	barWidth, barHeight := measureTextPanel(lines)
	barX := ui.game.config.GetScreenWidth() - barWidth - 10
	barY := 10

	return lines, barX, barY, barWidth, barHeight
}

func (ui *UISystem) getCompassCenter() (int, int) {
	_, _, barY, _, barHeight := ui.turnBasedStatusLayout()
	compassRadius := ui.compassRadius()
	spacing := 28
	compassX := ui.game.config.GetScreenWidth() - 28 - compassRadius
	compassY := barY + barHeight + spacing + compassRadius
	return compassX, compassY
}

// The compass is sized from the viewport (6% of its shorter side) and clamped
// to a readable band. It is fully derived - the old ui.compass_radius knob was
// always overridden by the lower bound, so it was removed rather than left as a
// setting that changes nothing.
const (
	compassRadiusPercent = 6
	compassMinRadius     = 36
	compassMaxRadius     = 64
)

func (ui *UISystem) compassRadius() int {
	screenMin := min(ui.game.config.GetScreenWidth(), ui.game.config.GetScreenHeight())
	radius := (screenMin*compassRadiusPercent + 50) / 100
	return min(max(radius, compassMinRadius), compassMaxRadius)
}

// drawFPSCounter draws the FPS counter in the top-right corner
func (ui *UISystem) drawFPSCounter(screen *ebiten.Image) {
	// Use Ebiten's built-in FPS counter which is more reliable
	fps := ebiten.ActualFPS()
	tps := ebiten.ActualTPS()

	// Format FPS text
	lines := []string{
		fmt.Sprintf("FPS: %.1f", fps),
		fmt.Sprintf("TPS: %.1f", tps),
	}

	// Perf diagnostics: raycast vs sprite cost split, plus the per-frame draw
	// counters that localize a frame-time spike (open-corridor rays vs tree
	// standees vs impassable-aura tiles).
	if ui.game.gameLoop != nil && ui.game.gameLoop.renderer != nil {
		r := ui.game.gameLoop.renderer
		stats := ui.game.threading.PerformanceMonitor.GetDetailedStats()
		lines = append(lines,
			fmt.Sprintf("ray: %.2fms", getPerfFloat(stats, "last_raycast_time_ms")),
			fmt.Sprintf("spr: %.2fms", getPerfFloat(stats, "last_sprite_render_time_ms")),
			fmt.Sprintf("  floor: %.2fms", r.statFloorMs),
			fmt.Sprintf("  walls: %.2fms", r.statWallsMs),
			fmt.Sprintf("  sprites: %.2fms", r.statSpritesMs),
			fmt.Sprintf("trees: %d", r.statTreesDrawn),
			fmt.Sprintf("standee dc: %d", r.statStandeeCalls),
			fmt.Sprintf("aura: %d", r.statAuraTiles),
		)
		if p50, p95, p99, ok := ui.game.threading.PerformanceMonitor.FrameTimePercentilesMs(); ok {
			lines = append(lines, fmt.Sprintf("frame p50/95/99: %.1f/%.1f/%.1f", p50, p95, p99))
		}
	}

	compassX, compassY := ui.getCompassCenter()
	compassRadius := ui.compassRadius()
	_ = compassX
	barWidth, barHeight := measureTextPanel(lines)
	screenWidth := ui.game.config.GetScreenWidth()
	barX := screenWidth - barWidth - 10
	barY := compassY + compassRadius + 10

	vector.FillRect(screen, float32(barX), float32(barY), float32(barWidth), float32(barHeight), color.RGBA{0, 0, 0, 120}, false)

	for i, line := range lines {
		drawDebugText(screen, line, barX+textPanelPadding, barY+textPanelPadding+i*textPanelLineHeight)
	}
}

// drawInteractionNotification draws a semi-transparent notification when near an interactable NPC
func (ui *UISystem) drawInteractionNotification(screen *ebiten.Image) {
	// Skip if dialog is already active or menu is open
	if ui.game.dialogActive || ui.game.menuOpen {
		return
	}

	// The Space target: the NPC in interact focus (centred + adjacent tile).
	nearestNPC := ui.game.focusedNPC
	if nearestNPC == nil {
		return
	}

	// Calculate screen dimensions for positioning
	screenWidth := ui.game.config.GetScreenWidth()

	// Create interaction message based on NPC capabilities
	var message string
	if verb := nearestNPC.PromptVerb; verb != "" {
		// Authored override (npcs.yaml prompt_verb): "enter" for the tavern etc.
		message = fmt.Sprintf("Press SPACE to %s %s", verb, nearestNPC.Name)
	} else if ui.game.npcIsWalkUpProp(nearestNPC) {
		// Chests and lecterns are immediate-use props, not conversations - never
		// fall into the "talk to" ladder (a standee-sprited lectern would).
		message = fmt.Sprintf("Press SPACE to interact with %s", nearestNPC.Name)
	} else {
		switch npcDialogKindFor(nearestNPC) {
		case dialogKindSpellTrader:
			message = fmt.Sprintf("Press SPACE to talk to %s (Spell Trader)", nearestNPC.Name)
		case dialogKindChoices:
			// A person with a choice dialog is still a conversation; only
			// props/landmarks (wrecks, bones, valves) are "investigated".
			if npcIsPerson(nearestNPC) {
				message = fmt.Sprintf("Press SPACE to talk to %s", nearestNPC.Name)
			} else {
				message = fmt.Sprintf("Press SPACE to investigate %s", nearestNPC.Name)
			}
		case dialogKindSkillTrainer:
			message = fmt.Sprintf("Press SPACE to train with %s", nearestNPC.Name)
		case dialogKindMerchant:
			message = fmt.Sprintf("Press SPACE to trade with %s", nearestNPC.Name)
		case dialogKindCardCollector:
			message = fmt.Sprintf("Press SPACE to manage cards with %s", nearestNPC.Name)
		default:
			message = fmt.Sprintf("Press SPACE to talk to %s", nearestNPC.Name)
		}
	}

	// Calculate text dimensions for background sizing
	textWidth := debugTextWidth(message)
	textHeight := debugTextCharHeight
	padding := 15

	// Position at top center of screen
	notificationWidth := textWidth + (padding * 2)
	notificationHeight := textHeight + (padding * 2)
	notificationX := (screenWidth - notificationWidth) / 2
	notificationY := 10

	// Draw semi-transparent background
	vector.FillRect(screen, float32(notificationX), float32(notificationY), float32(notificationWidth), float32(notificationHeight), color.RGBA{0, 0, 0, 180}, false)

	// Draw border for better visibility
	borderColor := color.RGBA{255, 255, 255, 200} // Semi-transparent white
	vector.StrokeRect(
		screen,
		float32(notificationX-1),
		float32(notificationY-1),
		float32(notificationWidth+2),
		float32(notificationHeight+2),
		2,
		borderColor,
		false,
	)

	// Draw the interaction message
	textX := notificationX + padding
	textY := notificationY + padding
	drawDebugText(screen, message, textX, textY)
}

// drawInstructions draws the control instructions
func (ui *UISystem) drawInstructions(screen *ebiten.Image) {
	drawDebugText(screen, "ESC: Main menu", 10, 10)
	if ui.game.focusModeActive() {
		drawDebugTextColored(screen, "Focus mode", 10, 10+textPanelLineHeight, focusModeMetal)
	}
}
