package game

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	uitext "ugataima/assets/text"
	"ugataima/internal/config"
)

// The story screen shows one story event (story_events.go) as a two-page
// spread: the plate on the left, the dated chronicle entry on the right. A
// frame too narrow for the column stacks the entry under the plate. The plate
// arrives through the camp-rest dissolve; the world behind is paused and
// veiled. The captain's events blink his light before his words appear.

type storyPresentation struct {
	event *StoryEventDef
	day   int
	page  int
	// elapsed counts frames since the plate was first presented; pageStart is
	// the frame the current page's text began to fade in.
	elapsed, pageStart int
	presented          bool
	scene              dissolveScene
}

// storyCaptainInk is the captain's own accent: the sea-lamp teal of his light.
var storyCaptainInk = color.RGBA{92, 198, 188, 255}

const (
	storyMargin         = 24
	storyGap            = 16
	storyPad            = 16
	storyLineH          = uiTextCharHeight + 2
	storyMinColumnW     = 300
	storyMaxColumnW     = 460
	storyStackBodyLines = 5
	storyFrameInset     = 4 // the thin theme frame's rim
	storyLightGap       = 18
)

func (g *MMGame) openStoryScreen(e *StoryEventDef) {
	g.closeStoryScreen()
	day := g.currentCalendarDay()
	if d, ok := g.storySeenDay(e.ID); ok {
		day = d // 0: seen in a save from before story events, day unknown
	}
	g.storyScreen = &storyPresentation{event: e, day: day, scene: newDissolveScene(g.config.Camping)}
}

func (g *MMGame) closeStoryScreen() {
	if s := g.storyScreen; s != nil {
		s.scene.release()
	}
	g.storyScreen = nil
}

// storyTiming is the screen's clock in frames.
type storyTiming struct{ fadeIn, blinks, blink, textFade int }

// storyTiming reads the plate's clocks: the camp scene's fade-in, then the
// story_screen block (validated positive at config load).
func (g *MMGame) storyTiming() storyTiming {
	c := g.config.StoryScreen
	return storyTiming{
		fadeIn:   g.campRestFrames(g.config.Camping.FadeInSeconds, config.DefaultCampingConfig().FadeInSeconds),
		blinks:   c.LightBlinks,
		blink:    g.framesForSeconds(c.LightBlinkSeconds),
		textFade: g.framesForSeconds(c.TextFadeSeconds),
	}
}

// textStart is the frame the first page's text begins: after the plate, and
// after the captain's blinks.
func (s *storyPresentation) textStart(t storyTiming) int {
	start := t.fadeIn
	if s.event.Speaker == storySpeakerCaptain {
		start += t.blinks * t.blink
	}
	return start
}

func (s *storyPresentation) textAlpha(t storyTiming) float64 {
	start := max(s.textStart(t), s.pageStart)
	return smoothStep(float64(s.elapsed-start) / float64(t.textFade))
}

// textShown: the page's text is fully up - the next press turns the page.
func (s *storyPresentation) textShown(t storyTiming) bool {
	return s.textAlpha(t) >= 1
}

func (g *MMGame) tickStoryScreen() {
	s := g.storyScreen
	if s == nil || !s.presented {
		return
	}
	if g.loadingStall() {
		return
	}
	s.elapsed++
}

// advanceStory: the first press shows the page at once, the next turns it;
// past the last page the screen closes.
func (g *MMGame) advanceStory() {
	s := g.storyScreen
	if s == nil {
		return
	}
	t := g.storyTiming()
	if !s.textShown(t) {
		s.elapsed = max(s.elapsed, max(s.textStart(t), s.pageStart)+t.textFade)
		return
	}
	if s.page+1 < len(s.event.Pages) {
		s.page++
		s.pageStart = s.elapsed
		return
	}
	g.closeStoryScreen()
}

// storyLayout is the screen's geometry in UI units.
type storyLayout struct {
	plate, panel           layoutRect
	day, title, body, foot layoutRect
	side                   bool
}

// computeStoryLayout lays the screen out inside the gameplay viewport: the
// spread when the entry column gets at least storyMinColumnW, else stacked.
func computeStoryLayout(view layoutRect) storyLayout {
	in := layoutRect{view.x + storyMargin, view.y + storyMargin, view.w - 2*storyMargin, view.h - 2*storyMargin}
	var l storyLayout
	plateH := in.h
	plateW := plateH * 3 / 2
	if colW := min(storyMaxColumnW, in.w-plateW-storyGap); colW >= storyMinColumnW {
		x := in.x + (in.w-plateW-storyGap-colW)/2
		l.plate = layoutRect{x, in.y, plateW, plateH}
		l.panel = layoutRect{l.plate.right() + storyGap, in.y, colW, plateH}
		l.side = true
	} else {
		bandH := 2*storyPad + uiTextCharHeight + 4 + readingLineHeight + 8 + storyStackBodyLines*storyLineH + 8 + uiTextCharHeight
		plateH = max(1, in.h-bandH-storyGap)
		plateW = plateH * 3 / 2
		if plateW > in.w {
			plateW, plateH = in.w, in.w*2/3
		}
		y := in.y + (in.h-plateH-storyGap-bandH)/2
		l.plate = layoutRect{in.x + (in.w-plateW)/2, y, plateW, plateH}
		l.panel = layoutRect{l.plate.x, l.plate.bottom() + storyGap, plateW, bandH}
	}
	p := l.panel
	l.day = layoutRect{p.x + storyPad, p.y + storyPad, p.w - 2*storyPad, uiTextCharHeight}
	l.title = layoutRect{l.day.x, l.day.bottom() + 4, l.day.w, readingLineHeight}
	l.foot = layoutRect{l.day.x, p.bottom() - storyPad - uiTextCharHeight, l.day.w, uiTextCharHeight}
	l.body = layoutRect{l.day.x, l.title.bottom() + 8, l.day.w, l.foot.y - 8 - l.title.bottom() - 8}
	return l
}

// storyTitleFitsLarge: the title fits its box at reading size (2x), the
// captain's light included.
func storyTitleFitsLarge(title string, box layoutRect) bool {
	return uiTextWidth(title)*readingTextScale <= box.w-storyLightGap
}

// storyBodyLines wraps a page for the body box; the second result is whether
// it fits.
func storyBodyLines(page string, body layoutRect) ([]string, bool) {
	lines := wrapUIText(page, body.w)
	return lines, len(lines)*storyLineH <= body.h
}

func (ui *UISystem) drawStoryScreen(screen *ebiten.Image) {
	g, s := ui.game, ui.game.storyScreen
	if s == nil {
		return
	}
	t := g.storyTiming()
	view := layoutRect{0, 0, g.config.GetScreenWidth(), gameplayViewportBottom(g)}
	l := computeStoryLayout(view)
	plateIn := smoothStep(float64(s.elapsed) / float64(t.fadeIn))
	drawFilledRect(screen, view.x, view.y, view.w, view.h, color.RGBA{0, 0, 0, uint8(215 * max(0.35, plateIn))})

	ui.drawThemeFrame(screen, frameBronze, l.plate.x, l.plate.y, l.plate.w, l.plate.h)
	ui.drawThemeFrame(screen, frameBronze, l.panel.x, l.panel.y, l.panel.w, l.panel.h)
	if img := g.sprites.GetSprite(s.event.Art); img != nil {
		s.presented = true // display acknowledgement: the clock starts once the art is up
		inner := layoutRect{l.plate.x + storyFrameInset, l.plate.y + storyFrameInset, l.plate.w - 2*storyFrameInset, l.plate.h - 2*storyFrameInset}
		contain := func(w, h, iw, ih int) (float64, float64, float64, float64) {
			k := math.Min(float64(w)/float64(iw), float64(h)/float64(ih))
			dw, dh := float64(iw)*k, float64(ih)*k
			return (float64(w) - dw) / 2, (float64(h) - dh) / 2, dw, dh
		}
		s.scene.draw(screen, uiPixelRect(screen, image.Rect(inner.x, inner.y, inner.right(), inner.bottom())), img, contain, float32(plateIn), false, g.config.Camping)
	} else if !g.sprites.HasSprite(s.event.Art) {
		s.presented = true // nothing to wait for: the text still shows
	}

	ink := gameplayInk.text
	if s.event.Speaker == storySpeakerCaptain {
		ink = storyCaptainInk
		ui.drawStoryLight(screen, l, s, t)
	}
	if s.day > 0 {
		drawUITextColored(screen, uitext.Text("ui.story_day", s.day), l.day.x, l.day.y, gameplayInk.muted)
	}
	titleX := l.title.x
	if s.event.Speaker == storySpeakerCaptain {
		titleX += storyLightGap
	}
	if storyTitleFitsLarge(s.event.Title, l.title) {
		drawReadingText(screen, s.event.Title, titleX, l.title.y, gameplayInk.heading)
	} else { // a narrow column: the heading at text size, same ink
		drawUITextColored(screen, s.event.Title, titleX, l.title.y+(l.title.h-uiTextCharHeight)/2, gameplayInk.heading)
	}

	if a := s.textAlpha(t); a > 0 {
		lines, _ := storyBodyLines(s.event.Pages[s.page], l.body)
		for i, line := range lines {
			drawStoryText(screen, line, l.body.x, l.body.y+i*storyLineH, ink, a)
		}
	}
	if n := len(s.event.Pages); n > 1 {
		drawUITextColored(screen, uitext.Text("ui.story_page", s.page+1, n), l.foot.x, l.foot.y, gameplayInk.muted)
	}
	keys := uitext.Text("ui.story_continue")
	if s.page+1 >= len(s.event.Pages) {
		keys = uitext.Text("ui.story_close")
	}
	keys += "   " + uitext.Text("ui.story_skip")
	drawUITextColored(screen, keys, l.foot.right()-uiTextWidth(keys), l.foot.y, gameplayInk.muted)

	// A click anywhere over the veiled world does what Space does.
	ui.onGatedClick(view, func() bool { return ui.topModalLayer() == modalLayerStory && g.storyScreen == s }, g.advanceStory)
}

// drawStoryLight is the captain's light: three slow blinks before he speaks,
// then a steady glow while his words are up.
func (ui *UISystem) drawStoryLight(screen *ebiten.Image, l storyLayout, s *storyPresentation, t storyTiming) {
	if captainLightOn(s.elapsed-t.fadeIn, t) {
		drawCaptainLight(screen, float32(l.title.x+6), float32(l.title.y+readingLineHeight/2), 4, 1)
	}
}

// captainLightOn: the captain's light, k frames after it first appears,
// blinks t.blinks times and then stays lit.
func captainLightOn(k int, t storyTiming) bool {
	return k >= 0 && (k >= t.blinks*t.blink || k%t.blink < t.blink/2)
}

// drawCaptainLight draws his light: a solid core of radius r in a soft halo.
func drawCaptainLight(screen *ebiten.Image, cx, cy, r float32, alpha float64) {
	uiFillCircle(screen, cx, cy, 2*r, fadeVectorColor(storyCaptainInk, 0.28*alpha), true)
	uiFillCircle(screen, cx, cy, r, fadeVectorColor(storyCaptainInk, alpha), true)
}

// drawStoryText draws one body line faded by alpha through the draw options,
// so the cached label stays one image per colour.
func drawStoryText(screen *ebiten.Image, text string, x, y int, col color.RGBA, alpha float64) {
	drawUITextColoredAlphaIn(screen, activeUIFont(), text, x, y, col, alpha)
}

const (
	chronicleRowH   = 30
	chronicleDayW   = 90
	chronicleTabPad = 10
)

// drawChronicleToggle is the quiet text tab at the right end of the journal's
// title row that switches between the quests and the Chronicle.
func (ui *UISystem) drawChronicleToggle(screen *ebiten.Image, title layoutRect) {
	label := uitext.Text("ui.story_chronicle")
	if ui.questChronicle {
		label = uitext.Text("ui.story_quests")
	}
	w := uiTextWidth(label) + 2*chronicleTabPad
	r := layoutRect{title.right() - w, title.y - 4, w, uiTextCharHeight + 8}
	live := !ui.modalLayerOwnsInput()
	mx, my := uiCursorPosition()
	ink := gameplayInk.muted
	if live && isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom()) {
		ink = gameplayInk.focus
	}
	drawUITextColored(screen, label, r.x+chronicleTabPad, r.y+4, ink)
	drawFilledRect(screen, r.x+chronicleTabPad, r.bottom()-1, w-2*chronicleTabPad, 1, gameplayInk.edge)
	ui.onGatedClick(r, func() bool { return !ui.modalLayerOwnsInput() }, func() { ui.questChronicle, ui.questPage = !ui.questChronicle, 0 })
}

// drawChronicle lists the seen beats by day in the journal's card area,
// paged with the journal's pager (ui.questPage) so a long run never runs past
// it at the smaller frames; a click shows a beat again.
func (ui *UISystem) drawChronicle(screen *ebiten.Image, content layoutRect) {
	g := ui.game
	top, avail, pager := questCardListAvailable(content)
	x, w := pager.x, pager.w
	entries := g.storySeen // in the order they were seen
	if len(entries) == 0 {
		drawUITextColored(screen, uitext.Text("ui.story_chronicle_empty"), x, top, gameplayInk.muted)
		return
	}
	perPage := max(1, avail/chronicleRowH)
	pages := (len(entries) + perPage - 1) / perPage
	ui.questPage = max(0, min(ui.questPage, pages-1))
	start := ui.questPage * perPage
	live := !ui.modalLayerOwnsInput()
	mx, my := uiCursorPosition()
	for i, entry := range entries[start:min(len(entries), start+perPage)] {
		e := storyEvent(entry.ID)
		if e == nil {
			continue
		}
		r := layoutRect{x, top + i*chronicleRowH, w, chronicleRowH}
		hover := live && isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom())
		day := "-"
		if entry.Day > 0 {
			day = uitext.Text("ui.story_day", entry.Day)
		}
		ink := gameplayInk.heading
		if hover {
			ink = gameplayInk.focus
			drawFilledRect(screen, r.x, r.bottom()-2, r.w, 1, gameplayInk.edge)
		}
		drawUITextColored(screen, day, r.x, r.y+7, gameplayInk.muted)
		drawUITextColored(screen, e.Title, r.x+chronicleDayW, r.y+7, ink)
		ui.onGatedClick(r, func() bool { return !ui.modalLayerOwnsInput() }, func() { g.openStoryScreen(e) })
	}
	ui.drawPager(screen, pager.x, pager.y, pager.w, &ui.questPage, pages, live)
}
