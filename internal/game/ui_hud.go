package game

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/spells"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Portrait recess of party_member_panel.png in panel-source pixels (256x100),
// measured off the art: the inner dark box of the left frame, whose corners are
// trimmed at 45deg by panelRecessCut pixels.
const (
	panelRecessX0  = 16.0
	panelRecessY0  = 19.0
	panelRecessX1  = 65.0
	panelRecessY1  = 77.0
	panelRecessCut = 5.0
)

// hudWhiteImg is a 1x1 opaque source for triangle fills (corner bevel cuts).
var hudWhiteImg = func() *ebiten.Image {
	img := ebiten.NewImage(1, 1)
	img.Fill(color.White)
	return img
}()

// cardPortrait returns the portrait pre-fit to the party card's recess: cover-
// scaled (fills the frame, overflow cropped), linearly filtered, with the four
// corners bevel-cut to match the frame's trims. Cached per name and size.
func (ui *UISystem) cardPortrait(name string, w, h, cut int) *ebiten.Image {
	if w <= 0 || h <= 0 {
		return nil
	}
	key := fmt.Sprintf("%s|%dx%d", name, w, h)
	if img, ok := ui.cardPortraitCache[key]; ok {
		return img
	}
	src := ui.game.sprites.GetSprite(name)
	if src == nil {
		return nil
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

	// Bevel the corners: erase a 45deg triangle in each.
	if cut > 0 {
		c := float32(cut)
		fw, fh := float32(w), float32(h)
		corners := [4][6]float32{
			{0, 0, c, 0, 0, c},               // top-left
			{fw, 0, fw - c, 0, fw, c},        // top-right
			{0, fh, c, fh, 0, fh - c},        // bottom-left
			{fw, fh, fw - c, fh, fw, fh - c}, // bottom-right
		}
		for _, t := range corners {
			verts := []ebiten.Vertex{
				{DstX: t[0], DstY: t[1], SrcX: 0.5, SrcY: 0.5, ColorA: 1},
				{DstX: t[2], DstY: t[3], SrcX: 0.5, SrcY: 0.5, ColorA: 1},
				{DstX: t[4], DstY: t[5], SrcX: 0.5, SrcY: 0.5, ColorA: 1},
			}
			img.DrawTriangles(verts, []uint16{0, 1, 2}, hudWhiteImg,
				&ebiten.DrawTrianglesOptions{Blend: ebiten.BlendClear})
		}
	}

	if ui.cardPortraitCache == nil {
		ui.cardPortraitCache = make(map[string]*ebiten.Image)
	}
	ui.cardPortraitCache[key] = img
	return img
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

	// Draw party member portraits and stats at bottom of screen.
	// Fixed portrait width, centered horizontally - does not stretch in fullscreen.
	portraitWidth, portraitHeight, baseLeft, startY := partyPortraitLayout(ui.game)

	for i, member := range ui.game.party.Members {
		x := baseLeft + i*portraitWidth

		// Highlight selected character and heal target. In turn-based mode an
		// alive character that has already spent all their action slots gets
		// a gray frame so the player can see at a glance who still has a
		// move left. KO characters are skipped - they get no frame at all.
		// Dual Wielding gets a third, amber state: one weapon already used
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
		case !ui.game.turnBasedMode && member.CanAct() && member.IsDualWielding():
			// An empty main hand isn't a "busy hand" - it just isn't a hand at
			// all, so don't let its stale cooldown paint a false amber "one hand
			// busy". Only hands that actually hold a weapon count toward the
			// two-tone ring. (IsDualWielding already guarantees an off-hand weapon.)
			_, mainHasWeapon := member.Equipment[items.SlotMainHand]
			mainReady := mainHasWeapon && member.RTCooldown <= 0
			offReady := member.OffHandRTCooldown <= 0
			switch {
			case !mainHasWeapon:
				if !offReady { // only the off-hand weapon remains - treat as single-weapon
					highlightColor = grayBusy
				}
			case !mainReady && !offReady:
				highlightColor = grayBusy
			case mainReady != offReady:
				highlightColor = amberOneHandBusy
			}
		case !ui.game.turnBasedMode && member.CanAct() && member.RTCooldown > 0:
			highlightColor = grayBusy
		}
		if i == ui.game.selectedChar {
			highlightColor = color.RGBA{210, 170, 80, 220}
		}

		// Draw background panel
		panel := ui.game.sprites.GetSprite("party_member_panel")
		panelOpts := &ebiten.DrawImageOptions{}
		panelOpts.GeoM.Scale(
			float64(portraitWidth)/float64(panel.Bounds().Dx()),
			float64(portraitHeight)/float64(panel.Bounds().Dy()),
		)
		panelOpts.GeoM.Translate(float64(x), float64(startY))
		screen.DrawImage(panel, panelOpts)
		if highlightColor.A > 0 {
			vector.StrokeRect(screen, float32(x+2), float32(startY+2), float32(portraitWidth-5), float32(portraitHeight-5), 2, highlightColor, false)
		}

		// Draw character portrait (Column 1) - promotion-aware (Archmage/Lich
		// variant). Fitted exactly into the panel's left recess (measured off
		// party_member_panel.png) and bevel-cut at the corners to match the
		// frame's 45deg corner trims.
		portraitName := ui.game.portraitSpriteName(member)
		portraitColWidth := 82 // text columns start after the portrait recess

		panelScaleX := float64(portraitWidth) / float64(panel.Bounds().Dx())
		panelScaleY := float64(portraitHeight) / float64(panel.Bounds().Dy())
		px := x + int(panelRecessX0*panelScaleX+0.5)
		py := startY + int(panelRecessY0*panelScaleY+0.5)
		pw := int((panelRecessX1-panelRecessX0+1)*panelScaleX + 0.5)
		ph := int((panelRecessY1-panelRecessY0+1)*panelScaleY + 0.5)
		cut := int(panelRecessCut*math.Min(panelScaleX, panelScaleY) + 0.5)

		portraitOpts := &ebiten.DrawImageOptions{}
		portraitOpts.GeoM.Translate(float64(px), float64(py))
		// Apply red tint if character is blinking from damage
		if ui.game.IsCharacterBlinking(i) {
			portraitOpts.ColorScale.Scale(1.5, 0.5, 0.5, 1.0) // Red tint: more red, less green/blue
		}
		if card := ui.cardPortrait(portraitName, pw, ph, cut); card != nil {
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
			vector.FillRect(screen, float32(x), float32(startY), float32(portraitWidth-2), float32(portraitHeight), color.RGBA{0, 0, 0, 140}, false)
		}
		// Poison: green bubbles drift up the card (replaces the old green tint).
		if isPoisoned && !isUnconscious {
			ui.drawCardPoisonBubbles(screen, x, startY, portraitWidth, portraitHeight)
		}
		// Ignite: living flames lick up the card (stacks visually with poison).
		if isBurning && !isUnconscious {
			ui.drawCardIgnite(screen, x, startY, portraitWidth, portraitHeight, i)
		}
		// Stun: a ring of dazed stars wheels around the portrait head.
		if member.IsStunned() && !isUnconscious {
			ui.drawCardStunStars(screen, x, startY, portraitColWidth, portraitHeight)
		}

		// Particle overlays: Inferno scorch flames, hit sparks, heal "+" glyphs.
		ui.drawCardFlames(screen, x, startY, portraitWidth, portraitHeight, i)
		ui.drawCardSparks(screen, x, startY, portraitWidth, portraitHeight, i)
		ui.drawCardHealPlus(screen, x, startY, portraitWidth, portraitHeight, i)

		// Status Column (Column 2) - basic character info
		statusColX := x + portraitColWidth + 4
		statusColWidth := (portraitWidth - portraitColWidth - 12) / 2

		drawDebugText(screen, member.Name, statusColX, startY+15)
		drawDebugText(screen, fmt.Sprintf("HP:%d/%d", member.HitPoints, member.MaxHitPoints), statusColX, startY+30)
		drawDebugText(screen, fmt.Sprintf("SP:%d/%d", member.SpellPoints, member.MaxSpellPoints), statusColX, startY+45)

		// Add character condition status
		statusText := "OK"
		if len(member.Conditions) > 0 {
			conds := make([]string, 0, len(member.Conditions))
			for _, cond := range member.Conditions {
				conds = append(conds, cond.String())
			}
			statusText = strings.Join(conds, ", ")
		}
		drawDebugText(screen, statusText, statusColX, startY+60)

		// Equipment Column (Column 3) - weapon and spell equipment (even closer to status)
		equipColX := statusColX + statusColWidth - 12

		// Show equipped weapon
		if weapon, hasWeapon := member.Equipment[items.SlotMainHand]; hasWeapon {
			weaponText := fmt.Sprintf("W:%s", weapon.Name)
			if len(weaponText) > 12 { // Truncate if too long
				weaponText = weaponText[:9] + "..."
			}
			drawDebugText(screen, weaponText, equipColX, startY+15)
		} else {
			drawDebugText(screen, "W:None", equipColX, startY+15)
		}

		// Show equipped spell (unified slot)
		if spell, hasSpell := member.Equipment[items.SlotSpell]; hasSpell {
			spellText := fmt.Sprintf("S:%s", spell.Name)
			if len(spellText) > 12 { // Truncate if too long
				spellText = spellText[:9] + "..."
			}
			drawDebugText(screen, spellText, equipColX, startY+30)
		} else {
			drawDebugText(screen, "S:None", equipColX, startY+30)
		}

		// Dual Wielding: show the off-hand weapon on the row below (empty for
		// everyone else, who only ever has one weapon to show).
		if member.IsDualWielding() {
			offWeapon := member.Equipment[items.SlotOffHand]
			offText := fmt.Sprintf("O:%s", offWeapon.Name)
			if len(offText) > 12 {
				offText = offText[:9] + "..."
			}
			drawDebugText(screen, offText, equipColX, startY+45)
		}

		// Draw + button for stat points if available (under portrait)
		if member.FreeStatPoints > 0 {
			plusBtnX := x + 20
			plusBtnY := startY + portraitHeight - 28
			plusBtnW := 24
			plusBtnH := 24
			mouseX, mouseY := ebiten.CursorPosition()
			isHover := mouseX >= plusBtnX && mouseX < plusBtnX+plusBtnW && mouseY >= plusBtnY && mouseY < plusBtnY+plusBtnH
			ui.drawStatPointPlusButton(screen, plusBtnX, plusBtnY, plusBtnW, plusBtnH, member.FreeStatPoints, isHover)
			if ui.game.consumeLeftClickIn(plusBtnX, plusBtnY, plusBtnX+plusBtnW, plusBtnY+plusBtnH) {
				ui.game.statPopupOpen = true
				// Open the popup for THIS character. Don't touch selectedChar:
				// in turn-based mode it tracks whose turn it is, and hijacking it
				// made the popup show the active char instead of the clicked one.
				ui.game.statPopupCharIdx = i
				ui.justOpenedStatPopup = true
			}

		}

		// Draw ^ indicator for pending skill/spell choice
		if ui.game.hasLevelUpChoiceForChar(i) {
			caretX := x + portraitWidth - 28
			caretY := startY + portraitHeight - 28
			caretW := 24
			caretH := 24
			mouseX, mouseY := ebiten.CursorPosition()
			isHover := mouseX >= caretX && mouseX < caretX+caretW && mouseY >= caretY && mouseY < caretY+caretH
			ui.drawSkillPointIndicator(screen, caretX, caretY, caretW, caretH, isHover)
			if ui.game.consumeLeftClickIn(caretX, caretY, caretX+caretW, caretY+caretH) {
				ui.game.openLevelUpChoiceForChar(i)
			}
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
		autoBtnX := baseLeft + 72
		autoBtnY := startY + portraitHeight - 28
		autoBtnW, autoBtnH := 58, 24
		mouseX, mouseY := ebiten.CursorPosition()
		autoHover := isMouseHoveringBox(mouseX, mouseY, autoBtnX, autoBtnY, autoBtnX+autoBtnW, autoBtnY+autoBtnH)
		ui.drawAutoStatButton(screen, autoBtnX, autoBtnY, autoBtnW, autoBtnH, autoHover)
		if ui.game.consumeLeftClickIn(autoBtnX, autoBtnY, autoBtnX+autoBtnW, autoBtnY+autoBtnH) {
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
// hit (fxSpark, set by TriggerDamageBlink): the WHOLE card flashes
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
		vector.DrawFilledCircle(screen, float32(bx), float32(by), r, color.RGBA{70, 210, 90, a}, true)
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
			vector.DrawFilledCircle(screen, float32(px), float32(py), base*1.7, color.RGBA{200, 30, 0, a}, true)
		}
		if a := uint8(170 * rise); a > 8 { // orange body
			vector.DrawFilledCircle(screen, float32(px), float32(py), base, color.RGBA{255, uint8(40 + 120*rise), 0, a}, true)
		}
		if rise > 0.55 { // hot core, only near the base
			a := uint8(230 * (rise - 0.55) / 0.45)
			vector.DrawFilledCircle(screen, float32(px), float32(py), base*0.5, color.RGBA{255, 240, 170, a}, true)
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
		vector.DrawFilledCircle(screen, float32(px), float32(py), float32(1+1.5*(1-ph)), color.RGBA{255, 200, 90, a}, true)
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
		vector.DrawFilledCircle(screen, sx, sy, 1.2, color.RGBA{255, 255, 230, a}, true)
	}
}

// drawStatPointPlusButton draws the + button under the portrait if stat points are available
func (ui *UISystem) drawStatPointPlusButton(screen *ebiten.Image, x, y, w, h, points int, isHover bool) {
	var plusColor color.RGBA
	if isHover {
		plusColor = color.RGBA{80, 200, 80, 220}
	} else {
		plusColor = color.RGBA{60, 120, 60, 180}
	}
	vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), plusColor, false)
	ui.drawInterfaceIcon(screen, "icon_stat_up", x+2, y+2, w-4, h-4)
	drawDebugText(screen, fmt.Sprintf("%d", points), x+w+2, y+6)
}

func (ui *UISystem) drawAutoStatButton(screen *ebiten.Image, x, y, w, h int, isHover bool) {
	bg := color.RGBA{55, 95, 135, 210}
	if isHover {
		bg = color.RGBA{75, 135, 185, 230}
	}
	vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), bg, false)
	drawRectBorder(screen, x, y, w, h, 1, color.RGBA{130, 180, 220, 230})
	ui.drawInterfaceIcon(screen, "icon_stat_up", x+3, y+4, 16, 16)
	drawDebugTextColored(screen, "AUTO", x+22, y+7, color.White)
}

// drawSkillPointIndicator draws the ^ button for pending skill/spell choices.
func (ui *UISystem) drawSkillPointIndicator(screen *ebiten.Image, x, y, w, h int, isHover bool) {
	var caretColor color.RGBA
	if isHover {
		caretColor = color.RGBA{200, 180, 80, 220}
	} else {
		caretColor = color.RGBA{160, 140, 60, 200}
	}
	vector.FillRect(screen, float32(x), float32(y), float32(w), float32(h), caretColor, false)
	ui.drawInterfaceIcon(screen, "icon_level_choice", x+2, y+2, w-4, h-4)
}

// drawSpellStatusBar draws active spell effects in the top-left of the party UI area
func (ui *UISystem) drawSpellStatusBar(screen *ebiten.Image) {
	if !ui.game.showPartyStats {
		return
	}

	// Position at top-left of party UI area
	portraitHeight := ui.game.config.UI.PartyPortraitHeight
	partyStartY := ui.game.config.GetScreenHeight() - portraitHeight
	statusBarX := 10
	statusBarY := partyStartY - 40 // 40px above party UI

	iconSize := 24
	iconSpacing := 30
	currentX := statusBarX

	statuses := make([]*UtilitySpellStatus, 0, len(ui.game.utilitySpellStatuses))
	for _, status := range ui.game.utilitySpellStatuses {
		if status != nil && status.Duration > 0 {
			statuses = append(statuses, status)
		}
	}
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].SpellID < statuses[j].SpellID
	})

	if len(statuses) > 0 {
		barWidth := len(statuses)*iconSpacing - (iconSpacing - iconSize) + 10
		barHeight := iconSize + 8
		vector.FillRect(screen, float32(statusBarX-5), float32(statusBarY-4), float32(barWidth), float32(barHeight), color.RGBA{0, 0, 0, 120}, false)
	}

	for _, status := range statuses {
		iconX, iconY, iconW, iconH := ui.drawSpellIcon(screen, currentX, statusBarY, iconSize, status.Icon, status.Fallback, status.Duration, status.MaxDuration)
		ui.handleSpellIconClick(iconX, iconY, iconW, iconH, status.SpellID)
		currentX += iconSpacing
	}
}

// drawSpellIcon draws a single spell status icon with duration bar and returns clickable bounds
func (ui *UISystem) drawSpellIcon(screen *ebiten.Image, x, y, size int, icon, fallback string, currentDuration, maxDuration int) (int, int, int, int) {
	// Draw icon background (more transparent, with border)
	vector.FillRect(screen, float32(x), float32(y), float32(size), float32(size), color.RGBA{80, 80, 80, 200}, false)
	vector.FillRect(screen, float32(x+1), float32(y+1), float32(size-2), float32(size-2), color.RGBA{20, 20, 20, 120}, false)

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

		// Check for double-click (within 500ms and same icon)
		delta := currentTime - ui.game.lastUtilitySpellClickTime
		doubleClick := delta < doubleClickWindowMs && ui.game.lastClickedUtilitySpell == string(spellID)
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
	compassRadius := ui.game.config.UI.CompassRadius

	// Draw compass background circle (dark, semi-transparent)
	vector.DrawFilledCircle(screen, float32(compassX), float32(compassY), float32(compassRadius), color.RGBA{20, 20, 30, 200}, true)

	// Draw minimap tiles within the compass
	ui.drawCompassMinimap(screen, compassX, compassY, compassRadius)

	// Draw compass border
	vector.StrokeCircle(screen, float32(compassX), float32(compassY), float32(compassRadius), 2, color.RGBA{100, 100, 140, 255}, true)

	// Draw direction arrow pointing in the camera direction
	arrowLength := float64(compassRadius - 8)
	arrowX := float64(compassX) + arrowLength*math.Cos(ui.game.camera.Angle)
	arrowY := float64(compassY) + arrowLength*math.Sin(ui.game.camera.Angle)

	// Draw arrow line from center towards the direction
	vector.StrokeLine(screen, float32(compassX), float32(compassY), float32(arrowX), float32(arrowY), 2, color.RGBA{255, 80, 80, 255}, true)

	// Draw arrow head
	arrowHeadSize := 5.0
	vector.FillRect(screen, float32(arrowX-arrowHeadSize/2), float32(arrowY-arrowHeadSize/2), float32(arrowHeadSize), float32(arrowHeadSize), color.RGBA{255, 80, 80, 255}, false)

	// Draw player position indicator in center
	vector.DrawFilledCircle(screen, float32(compassX), float32(compassY), 3, color.RGBA{50, 200, 255, 255}, true)
}

// drawCompassMinimap renders the nearby tiles on the compass as a minimap
func (ui *UISystem) drawCompassMinimap(screen *ebiten.Image, centerX, centerY, radius int) {
	if ui.game.world == nil {
		return
	}

	tileSize := ui.game.config.GetTileSize()
	playerTileX := int(ui.game.camera.X / tileSize)
	playerTileY := int(ui.game.camera.Y / tileSize)

	// Number of tiles to show in each direction from center
	viewRange := 5
	// Size of each minimap tile in pixels
	miniTileSize := float32(radius) / float32(viewRange+1)
	if miniTileSize < 3 {
		miniTileSize = 3
	}
	if miniTileSize > 8 {
		miniTileSize = 8
	}

	// Get floor color from map config
	floorColor := color.RGBA{60, 110, 60, 180}
	if world.GlobalWorldManager != nil {
		if mapCfg := world.GlobalWorldManager.GetCurrentMapConfig(); mapCfg != nil {
			floorColor = color.RGBA{uint8(mapCfg.DefaultFloorColor[0]), uint8(mapCfg.DefaultFloorColor[1]), uint8(mapCfg.DefaultFloorColor[2]), 180}
		}
	}

	// Render tiles around the player
	for dy := -viewRange; dy <= viewRange; dy++ {
		for dx := -viewRange; dx <= viewRange; dx++ {
			tileX := playerTileX + dx
			tileY := playerTileY + dy

			// Skip tiles outside world bounds
			if tileX < 0 || tileX >= ui.game.world.Width || tileY < 0 || tileY >= ui.game.world.Height {
				continue
			}

			// Calculate screen position (offset from compass center)
			screenX := float32(centerX) + float32(dx)*miniTileSize
			screenY := float32(centerY) + float32(dy)*miniTileSize

			// Check if this tile is within the circular compass area
			distFromCenter := math.Sqrt(float64(dx*dx + dy*dy))
			if distFromCenter > float64(viewRange) {
				continue
			}

			// Get tile color based on type
			tile := ui.game.world.Tiles[tileY][tileX]
			tileColor := ui.getMinimapTileColor(tile, floorColor)

			// Draw the minimap tile
			halfSize := miniTileSize / 2
			vector.FillRect(screen, screenX-halfSize, screenY-halfSize, miniTileSize, miniTileSize, tileColor, false)
		}
	}

	// Draw NPCs on minimap
	for _, npc := range ui.game.world.NPCs {
		npcTileX := int(npc.X / tileSize)
		npcTileY := int(npc.Y / tileSize)
		dx := npcTileX - playerTileX
		dy := npcTileY - playerTileY

		// Only show NPCs within view range
		distFromCenter := math.Sqrt(float64(dx*dx + dy*dy))
		if distFromCenter <= float64(viewRange) {
			screenX := float32(centerX) + float32(dx)*miniTileSize
			screenY := float32(centerY) + float32(dy)*miniTileSize
			// Draw NPC as yellow dot
			vector.DrawFilledCircle(screen, screenX, screenY, miniTileSize/2, color.RGBA{255, 220, 0, 255}, true)
		}
	}
}

// getMinimapTileColor returns the color for a tile type on the minimap
func (ui *UISystem) getMinimapTileColor(tile world.TileType3D, floorColor color.RGBA) color.RGBA {
	switch tile {
	case world.TileWall, world.TileTree, world.TileAncientTree, world.TileThicket, world.TileMossRock, world.TileLowWall, world.TileHighWall:
		return color.RGBA{50, 50, 60, 200} // Dark for walls/obstacles
	case world.TileWater:
		return color.RGBA{40, 90, 160, 200} // Blue for water
	case world.TileDeepWater:
		return color.RGBA{25, 60, 120, 200} // Darker blue for deep water
	case world.TileVioletTeleporter:
		return color.RGBA{170, 80, 200, 200} // Violet for teleporters
	case world.TileRedTeleporter:
		return color.RGBA{200, 70, 70, 200} // Red for teleporters
	case world.TileClearing:
		return color.RGBA{80, 140, 80, 180} // Lighter green for clearings
	default:
		return floorColor
	}
}

// drawWizardEyeRadar draws enemy dots on the compass when wizard eye is active
func (ui *UISystem) drawWizardEyeRadar(screen *ebiten.Image) {
	if !ui.game.wizardEyeActive {
		return
	}

	compassX, compassY := ui.getCompassCenter()
	compassRadius := ui.game.config.UI.CompassRadius

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
			// Calculate angle from player to monster
			angle := math.Atan2(dy, dx)

			// Place dot at compass edge based on direction
			edgeRadius := float64(compassRadius - 5) // 5 pixels inside compass edge
			dotX := compassX + int(math.Cos(angle)*edgeRadius)
			dotY := compassY + int(math.Sin(angle)*edgeRadius)

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

			// Draw cached dot image (much faster than vector.DrawFilledCircle)
			opts := &ebiten.DrawImageOptions{}
			opts.GeoM.Translate(float64(dotX-2), float64(dotY-2)) // Center the 4x4 dot
			screen.DrawImage(dotImg, opts)
		}
	}
}

const hudMessageSpacing = 18

// drawCombatMessages draws the compact recent-message log just above the party
// portraits. Long entries word-wrap to the block width (see hudMessageLines); the
// block is bottom-anchored and grows upward so it never spills over the party UI.
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
	drawNineSlice(screen, ui.game.sprites.GetSprite("menu_panel_frame"), x, y, w, h, menuPanelFrameSlice)
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

	vector.FillRect(screen, float32(barX), float32(barY), float32(barWidth), float32(barHeight), color.RGBA{0, 0, 0, 120}, false)

	for i, line := range lines {
		drawDebugText(screen, line, barX+textPanelPadding, barY+textPanelPadding+i*textPanelLineHeight)
	}
}

func (ui *UISystem) turnBasedStatusLayout() ([]string, int, int, int, int) {
	mode := "Real-time"
	if ui.game.turnBasedMode {
		mode = "Turn-based"
	}
	lines := []string{fmt.Sprintf("Mode: %s", mode)}
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
	compassRadius := ui.game.config.UI.CompassRadius
	spacing := 10
	compassX := ui.game.config.GetScreenWidth() - 10 - compassRadius
	compassY := barY + barHeight + spacing + compassRadius
	return compassX, compassY
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
	}

	compassX, compassY := ui.getCompassCenter()
	compassRadius := ui.game.config.UI.CompassRadius
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
	switch npcDialogKindFor(nearestNPC) {
	case dialogKindSpellTrader:
		message = fmt.Sprintf("Press SPACE to talk to %s (Spell Trader)", nearestNPC.Name)
	case dialogKindChoices:
		message = fmt.Sprintf("Press SPACE to investigate %s", nearestNPC.Name)
	case dialogKindSkillTrainer:
		message = fmt.Sprintf("Press SPACE to train with %s", nearestNPC.Name)
	case dialogKindMerchant:
		message = fmt.Sprintf("Press SPACE to trade with %s", nearestNPC.Name)
	case dialogKindCardCollector:
		message = fmt.Sprintf("Press SPACE to manage cards with %s", nearestNPC.Name)
	default:
		message = fmt.Sprintf("Press SPACE to talk to %s", nearestNPC.Name)
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
}
