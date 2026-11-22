package game

import (
	"fmt"
	"image/color"
	"strconv"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/highscore"
)

// highScoresLayout is the High Scores screen: ranked runs on the left, the
// chosen run on the right. Drawing and clicks share it.
type highScoresLayout struct {
	panel, title, list, detail layoutRect
	footerY                    int
	back                       layoutRect // the title page's Back; zero in game
}

// The footer hint; in game Esc closes the screen, on the title Back does.
const (
	highScoresHint   = "Click a run to see its party | Up/Down: choose"
	highScoresEscTip = " | Esc: close"
)

const (
	highScoresW       = 820
	highScoresH       = 560
	highScoresInset   = 20
	highScoresListW   = 300
	highScoresRowH    = 40
	highScoresPitch   = 44
	highScoresHeroH   = 52
	highScoresGearSz  = 22
	highScoreGearCols = 6 // two rows hold every worn slot
)

// makeHighScoresLayout fills the panel; withBack (the title page) puts Back in
// the footer's left with the hint beside it, as on the title Load page.
func makeHighScoresLayout(screenW, screenH int, withBack bool) highScoresLayout {
	panel := centeredRect(screenW, screenH, min(screenW-24, highScoresW), min(screenH-24, highScoresH))
	c := layoutRect{panel.x + highScoresInset, panel.y + 14, panel.w - 2*highScoresInset, panel.h - 14 - highScoresInset}
	l := highScoresLayout{panel: panel, title: layoutRect{c.x, c.y, c.w, saveListTitleH}}
	top := l.title.bottom() + saveListGap
	footerTop := c.bottom() - uiTextCharHeight
	l.footerY = footerTop
	if withBack {
		l.back = layoutRect{c.x, c.bottom() - menuBackButtonH, menuBackButtonW, menuBackButtonH}
		footerTop, l.footerY = l.back.y, l.back.y+(menuBackButtonH-uiTextCharHeight)/2
	}
	bottom := footerTop - saveListGap
	l.list = layoutRect{c.x, top, highScoresListW, bottom - top}
	l.detail = layoutRect{l.list.right() + 20, top, c.right() - l.list.right() - 20, bottom - top}
	return l
}

func (l highScoresLayout) row(i int) layoutRect {
	return layoutRect{l.list.x, l.list.y + i*highScoresPitch, l.list.w, highScoresRowH}
}

// highScoreMedals tint the first three ranks; the rest go unmarked.
var highScoreMedals = [...]color.RGBA{rarityGold, raritySilver, {196, 128, 72, 255}}

// highScorePartTones colour the score parts in the bar and their swatches.
var highScorePartTones = [...]color.RGBA{
	buildsBarTones.primary[2], buildsBarTones.primary[1], buildsBarTones.primary[0], buildsBarTones.other[2],
}

// drawHighScoresOverlay is the High Scores screen, in game and on the title.
func (ui *UISystem) drawHighScoresOverlay(screen *ebiten.Image) {
	g := ui.game
	ui.discardLowerHoverText()
	w, h := g.config.GetScreenWidth(), g.config.GetScreenHeight()
	inGame := g.appScreen == AppScreenInGame
	l := makeHighScoresLayout(w, h, !inGame)
	drawFilledRect(screen, 0, 0, w, h, color.RGBA{0, 0, 0, 170})
	ui.drawThemeFrame(screen, frameGold, l.panel.x, l.panel.y, l.panel.w, l.panel.h)
	ui.drawCornerDecor(screen, frameGold, l.panel.x-8, l.panel.y-8, l.panel.w+16, l.panel.h+16, decorTopCorners)
	drawScaledMetalCenteredText(screen, "High Scores", l.title.x+l.title.w/2, l.title.y+l.title.h/2, 2, rarityGold)
	if inGame {
		drawUITextColored(screen, clipUIText(highScoresHint+highScoresEscTip, l.title.w), l.title.x, l.footerY, gameplayInk.muted)
	} else {
		drawRightUIText(screen, clipUIText(highScoresHint, l.title.right()-l.back.right()-16), l.title.right(), l.footerY, gameplayInk.muted)
		ui.drawBackButton(screen, l.back.x, l.back.y, func() { g.entryMenuMode = EntryMenuRoot })
	}

	board, err := highscore.LoadCached()
	if err != nil {
		drawCenteredTextWithShadow(screen, "The high scores could not be read.", l.list.x, l.list.y, l.panel.w-2*highScoresInset, 40, gameplayInk.muted)
		return
	}
	if len(board.Entries) == 0 {
		msg := "No victories yet. Win the game to record a run: experience, gold, party level and speed all score."
		ui.drawWrappedTextWithOverflow(screen, msg, layoutRect{l.list.x, l.list.y + 8, l.panel.w - 2*highScoresInset, 3 * (uiTextCharHeight + 4)}, 3, uiTextCharHeight+4)
		return
	}
	chosen := max(0, min(g.highScoreSelection, len(board.Entries)-1))

	mx, my := uiCursorPosition()
	for i, e := range board.Entries {
		r := l.row(i)
		if r.bottom() > l.list.bottom() {
			break
		}
		hover := isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom())
		ui.drawHighScoreRow(screen, r, i, e, i == chosen, hover)
		index := i
		ui.onClick(r, true, func() {
			g.highScoreSelection = index
		})
	}
	ui.drawHighScoreDetail(screen, l.detail, chosen, board.Entries[chosen])
}

// drawHighScoreRow is one run: its rank (a medal for the first three), the
// party's name, the score, then level, play time and date.
func (ui *UISystem) drawHighScoreRow(screen *ebiten.Image, r layoutRect, rank int, e highscore.Entry, chosen, hover bool) {
	if chosen || hover {
		ui.drawButtonFrame(screen, r.x, r.y, r.w, r.h, true)
	}
	if chosen {
		drawFilledRect(screen, r.x+3, r.y+3, r.w-6, r.h-6, gameplayInk.selected)
	}
	badge := layoutRect{r.x + 8, r.y + (r.h-22)/2, 22, 22}
	label := strconv.Itoa(rank + 1)
	if rank < len(highScoreMedals) {
		drawFilledRect(screen, badge.x, badge.y, badge.w, badge.h, highScoreMedals[rank])
		drawRectBorder(screen, badge.x, badge.y, badge.w, badge.h, 1, color.RGBA{30, 24, 16, 255})
		drawCenteredTextWithShadow(screen, label, badge.x, badge.y, badge.w, badge.h, color.RGBA{30, 24, 16, 255})
	} else {
		drawCenteredTextWithShadow(screen, label, badge.x, badge.y, badge.w, badge.h, gameplayInk.muted)
	}
	x, right := badge.right()+10, r.right()-10
	y1 := r.y + (r.h-2*uiTextCharHeight-2)/2
	score := groupedInt(int64(e.Score))
	drawRightUIText(screen, score, right, y1, rarityGold)
	nameInk := gameplayInk.text
	if chosen {
		nameInk = gameplayInk.heading
	}
	drawUITextColored(screen, clipUIText(e.PlayerName, right-uiTextWidth(score)-12-x), x, y1, nameInk)
	meta := fmt.Sprintf("Lv %d | %s | %s", e.AvgLevel, e.PlayTime, e.Date.Local().Format("2 Jan 2006"))
	drawUITextColored(screen, clipUIText(meta, right-x), x, y1+uiTextCharHeight+2, gameplayInk.muted)
}

// drawHighScoreDetail is the chosen run: who, the score and what earned it,
// and the party with everything they wore.
func (ui *UISystem) drawHighScoreDetail(screen *ebiten.Image, d layoutRect, rank int, e highscore.Entry) {
	score := groupedInt(int64(e.Score))
	scoreW := 2*uiTextWidth(score) + 4
	drawScaledMetalCenteredText(screen, score, d.right()-scoreW/2, d.y+uiTextCharHeight, 2, rarityGold)
	nameW := d.w - scoreW - 12
	drawUITextColored(screen, clipUIText(fmt.Sprintf("%s. %s", ordinal(rank+1), e.PlayerName), nameW), d.x, d.y, gameplayInk.heading)
	when := fmt.Sprintf("%s | %s played", e.Date.Local().Format("2 Jan 2006"), e.PlayTime)
	drawUITextColored(screen, clipUIText(when, nameW), d.x, d.y+uiTextCharHeight+4, gameplayInk.muted)

	// What earned the score: a proportion bar, then one line per part.
	y := d.y + 2*uiTextCharHeight + 16
	drawFilledRect(screen, d.x, y, d.w, 1, gameplayInk.edge)
	y += 8
	parts := e.Parts()
	values := [...]int{parts.Experience, parts.Gold, parts.Levels, parts.Speed}
	total := max(1, parts.Total())
	bar := layoutRect{d.x, y, d.w, 8}
	drawFilledRect(screen, bar.x, bar.y, bar.w, bar.h, buildsBarTones.track)
	x := bar.x
	for i, v := range values {
		segW := bar.w * v / total
		drawFilledRect(screen, x, bar.y, segW, bar.h, highScorePartTones[i])
		x += segW
	}
	y = bar.bottom() + 8
	speed := "none"
	if parts.Speed > 0 {
		speed = "finished in " + e.PlayTime
	} else if secs, ok := e.Seconds(); ok && secs >= highscore.SpeedBonusSeconds {
		speed = fmt.Sprintf("none: over %d hours", highscore.SpeedBonusSeconds/int(time.Hour/time.Second))
	}
	lines := [...]struct{ label, basis string }{
		{"Experience", fmt.Sprintf("%s XP x%d", groupedInt(int64(e.Experience)), highscore.PointsPerExperience)},
		{"Gold earned", fmt.Sprintf("%s x%d", groupedInt(int64(e.Gold)), highscore.PointsPerGold)},
		{"Party level", fmt.Sprintf("%d x%d", e.AvgLevel, highscore.PointsPerLevel)},
		{"Speed bonus", speed},
	}
	labelW := 0
	for _, line := range lines {
		labelW = max(labelW, uiTextWidth(line.label))
	}
	labelW += 14 + 12
	for i, line := range lines {
		drawFilledRect(screen, d.x, y+(uiTextCharHeight-8)/2, 8, 8, highScorePartTones[i])
		drawUITextColored(screen, line.label, d.x+14, y, gameplayInk.text)
		points := groupedInt(int64(values[i]))
		drawRightUIText(screen, points, d.right(), y, gameplayInk.heading)
		drawUITextColored(screen, clipUIText(line.basis, d.w-labelW-uiTextWidth(points)-12), d.x+labelW, y, gameplayInk.muted)
		y += uiTextCharHeight + 4
	}

	// The party, each hero with the gear they ended in.
	y += 6
	drawFilledRect(screen, d.x, y, d.w, 1, gameplayInk.edge)
	y += 8
	if len(e.Party) == 0 {
		drawUITextColored(screen, clipUIText("This run was recorded before parties were kept.", d.w), d.x, y, gameplayInk.muted)
		return
	}
	mx, my := uiCursorPosition()
	for _, hero := range e.Party {
		if y+highScoresHeroH > d.bottom() {
			break
		}
		portrait := layoutRect{d.x, y, 40, 48}
		if img := ui.cardPortrait(hero.Portrait, portrait.w, portrait.h, false); img != nil {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(portrait.x), float64(portrait.y))
			uiDrawImage(screen, img, op)
		}
		drawRectBorder(screen, portrait.x, portrait.y, portrait.w, portrait.h, 1, gameplayInk.edge)
		textX := portrait.right() + 10
		gearX := d.right() - highScoreGearCols*(highScoresGearSz+2) + 2
		class := hero.Class
		if c, ok := character.ClassFromKey(hero.Class); ok {
			class = c.String()
		}
		drawUITextColored(screen, clipUIText(hero.Name, gearX-8-textX), textX, y+6, gameplayInk.text)
		if hero.Level > 0 { // 0: a party restored without its final levels
			class = fmt.Sprintf("%s %d", class, hero.Level)
		}
		drawUITextColored(screen, clipUIText(class, gearX-8-textX), textX, y+8+uiTextCharHeight, gameplayInk.muted)
		gearY := y + (48-2*highScoresGearSz-2)/2
		for j, it := range hero.Gear {
			gx, gy := gearX+j%highScoreGearCols*(highScoresGearSz+2), gearY+j/highScoreGearCols*(highScoresGearSz+2)
			ui.drawItemIcon(screen, it, gx, gy, highScoresGearSz, highScoresGearSz, 1, true, false, nil)
			if isMouseHoveringBox(mx, my, gx, gy, gx+highScoresGearSz, gy+highScoresGearSz) {
				ui.queueItemTooltip(GetItemTooltipRows(it, nil, ui.game.combat, tooltipDetailHeld()), it, nil, mx+16, my+8)
			}
		}
		y += highScoresHeroH
	}
}

// stepHighScoreSelection moves the High Scores detail with Up/Down.
func (g *MMGame) stepHighScoreSelection(pressed func(ebiten.Key) bool) {
	board, err := highscore.LoadCached()
	if err != nil || len(board.Entries) == 0 {
		return
	}
	g.highScoreSelection = max(0, min(g.highScoreSelection, len(board.Entries)-1))
	if pressed(ebiten.KeyUp) {
		g.highScoreSelection--
	}
	if pressed(ebiten.KeyDown) {
		g.highScoreSelection++
	}
	g.highScoreSelection = max(0, min(g.highScoreSelection, len(board.Entries)-1))
}

// groupedInt writes n with thousands separators: 1234567 -> "1,234,567".
func groupedInt(n int64) string {
	s := strconv.FormatInt(n, 10)
	sign := ""
	if n < 0 {
		sign, s = "-", s[1:]
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return sign + s
}

// ordinal is a rank as "1st", "2nd", "3rd", "4th".
func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return strconv.Itoa(n) + suffix
}
