package game

import (
	"fmt"
	"image"
	"math/rand/v2"
	"os"
	"slices"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"

	uitext "ugataima/assets/text"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/strictyaml"
)

// The watcher is the Pursuers' captain (assets/watcher.yaml): he speaks at
// camp and after a boss falls, once per line, from his first story plate on.
// A story boss's line always plays; the rest are rare random remarks under a
// per-run budget. Camp lines play over the camp scene and hold it until read;
// kill lines wait for the fight to end and show as a card that never pauses.

type watcherWhen string

const (
	watcherAtCamp watcherWhen = "camp"
	watcherOnKill watcherWhen = "monster_killed"
)

// WatcherLine is one authored remark.
type WatcherLine struct {
	ID   string      `yaml:"id"`
	When watcherWhen `yaml:"when"`
	// Monsters (keys) select the kills an authored kill line answers; Boss
	// puts a line in the pool for any other boss.
	Monsters []string `yaml:"monsters,omitempty"`
	Boss     bool     `yaml:"boss,omitempty"`
	// Story marks a story boss's line: it always plays, outside the budget.
	Story bool `yaml:"story,omitempty"`
	// After names a story event or another line that must be seen; Until
	// lists those of which any one closes the line.
	After string   `yaml:"after,omitempty"`
	Until []string `yaml:"until,omitempty"`
	// Context lines play only while it holds, and take precedence at camp.
	PartyHurt bool   `yaml:"party_hurt,omitempty"`
	Night     bool   `yaml:"night,omitempty"`
	Text      string `yaml:"text"`
}

type watcherConfig struct {
	Speaker  string `yaml:"speaker"`
	Portrait string `yaml:"portrait"`
	// IntroEvent is the story beat that introduces him; no line plays before it.
	IntroEvent string        `yaml:"intro_event"`
	Lines      []WatcherLine `yaml:"lines"`
}

var globalWatcher *watcherConfig

// watcherIntro is the story event that introduces the captain.
func watcherIntro() string {
	if globalWatcher == nil {
		return ""
	}
	return globalWatcher.IntroEvent
}

// watcherMaxText keeps a line to the card's three lines at its narrowest.
const watcherMaxText = 150

// LoadWatcherConfig loads assets/watcher.yaml; story events load first, since
// lines gate on them.
func LoadWatcherConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("watcher: %w", err)
	}
	var cfg watcherConfig
	if err := strictyaml.Decode(data, &cfg); err != nil {
		return fmt.Errorf("watcher: %w", err)
	}
	if err := validateWatcher(&cfg); err != nil {
		return fmt.Errorf("watcher: %w", err)
	}
	globalWatcher = &cfg
	return nil
}

func validateWatcher(cfg *watcherConfig) error {
	if cfg.Speaker == "" || cfg.Portrait == "" {
		return fmt.Errorf("needs a speaker and a portrait")
	}
	if cfg.IntroEvent == "" || storyEvent(cfg.IntroEvent) == nil {
		return fmt.Errorf("intro_event %q is not a loaded story event", cfg.IntroEvent)
	}
	ids := map[string]bool{}
	for _, l := range cfg.Lines {
		if l.ID == "" || ids[l.ID] {
			return fmt.Errorf("line id %q is empty or repeated", l.ID)
		}
		// after/until resolve an ID against both catalogs: it must mean one thing.
		if storyEvent(l.ID) != nil {
			return fmt.Errorf("line id %q is also a story event id", l.ID)
		}
		ids[l.ID] = true
	}
	known := func(id string) bool { return id == "" || ids[id] || storyEvent(id) != nil }
	for _, l := range cfg.Lines {
		if l.Text == "" || !config.IsASCII(l.Text) || len(l.Text) > watcherMaxText {
			return fmt.Errorf("%s: text must be ASCII, 1-%d characters", l.ID, watcherMaxText)
		}
		if !known(l.After) || slices.ContainsFunc(l.Until, func(id string) bool { return id == "" || !known(id) }) {
			return fmt.Errorf("%s: after/until must name a story event or a line", l.ID)
		}
		switch l.When {
		case watcherAtCamp:
			if len(l.Monsters) > 0 || l.Boss || l.Story {
				return fmt.Errorf("%s: a camp line takes no monsters and is never story", l.ID)
			}
		case watcherOnKill:
			if len(l.Monsters) == 0 && !l.Boss {
				return fmt.Errorf("%s: a kill line needs monsters or boss", l.ID)
			}
			if l.PartyHurt || l.Night {
				return fmt.Errorf("%s: context belongs to camp lines", l.ID)
			}
			if l.Boss && len(l.Monsters) > 0 {
				return fmt.Errorf("%s: a kill line answers its monsters or joins the boss pool, not both", l.ID)
			}
			if l.Story && len(l.Monsters) == 0 {
				return fmt.Errorf("%s: a story line names its bosses and stays out of the pool", l.ID)
			}
			if err := validateMonsterKeys(l.ID, l.Monsters); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s: unknown when %q", l.ID, l.When)
		}
	}
	return nil
}

// watcherHasSeen answers a gate: a story event shown, or a line spoken.
func (g *MMGame) watcherHasSeen(id string) bool {
	return g.storyWasSeen(id) || g.watcherSeen[id]
}

// watcherIntroduced: his introduction has been shown. A story boss's line
// also counts it when the introduction is still queued - the first boss the
// party kills may be a story boss, whose line plays after the plate.
func (g *MMGame) watcherIntroduced(story bool) bool {
	intro := watcherIntro()
	return g.storyWasSeen(intro) || story && slices.Contains(g.storyQueue, intro)
}

// watcherLineOpen: the line is unspoken and its gates hold now.
func (g *MMGame) watcherLineOpen(l *WatcherLine) bool {
	if g.watcherSeen[l.ID] || !g.watcherIntroduced(l.Story) {
		return false
	}
	if l.After != "" && !g.watcherHasSeen(l.After) {
		return false
	}
	return !slices.ContainsFunc(l.Until, g.watcherHasSeen)
}

// watcherPartyHurt: a living member of the camping party - the active one;
// the reserve and captives do not lie down here - is below half health.
func (g *MMGame) watcherPartyHurt() bool {
	for _, m := range g.party.Members {
		if m.HitPoints > 0 && m.HitPoints*2 < m.MaxHitPoints {
			return true
		}
	}
	return false
}

// pickWatcherLine draws one open line of a channel; at camp a line whose
// context holds wins over the plain ones. hurt is the party's state as it lay
// down: the rest heals it before the camp scene starts.
func (g *MMGame) pickWatcherLine(when watcherWhen, hurt bool, accept func(*WatcherLine) bool) *WatcherLine {
	if globalWatcher == nil {
		return nil
	}
	var context, plain []*WatcherLine
	for i := range globalWatcher.Lines {
		l := &globalWatcher.Lines[i]
		if l.When != when || !g.watcherLineOpen(l) || !accept(l) {
			continue
		}
		if l.PartyHurt || l.Night {
			if (!l.PartyHurt || hurt) && (!l.Night || g.dayNightIsNight) {
				context = append(context, l)
			}
			continue
		}
		plain = append(plain, l)
	}
	if len(context) > 0 {
		return context[rand.IntN(len(context))]
	}
	if len(plain) > 0 {
		return plain[rand.IntN(len(plain))]
	}
	return nil
}

// speakWatcherLine records a line as spoken and writes it to the log.
func (g *MMGame) speakWatcherLine(l *WatcherLine) {
	g.markWatcherSpoken(l)
	g.logWatcherLine(l)
}

// markWatcherSpoken records a line as spoken; a random remark also restarts
// the days between them.
func (g *MMGame) markWatcherSpoken(l *WatcherLine) {
	if g.watcherSeen == nil {
		g.watcherSeen = map[string]bool{}
	}
	g.watcherSeen[l.ID] = true
	if !l.Story {
		g.watcherLastDay = g.currentCalendarDay()
	}
}

// logWatcherLine writes his words to the combat log, where they can be reread.
func (g *MMGame) logWatcherLine(l *WatcherLine) {
	g.logCombatLine(logToneWatcher, uitext.Text("combat.watcher_says", logSlot, logSlot),
		logColored(globalWatcher.Speaker, storyCaptainInk), logColored(l.Text, storyCaptainInk))
}

// watcherRandomReady: a random remark may roll now - he has been introduced,
// none is waiting, the run's budget is not spent and the days have passed.
func (g *MMGame) watcherRandomReady() bool {
	if globalWatcher == nil || !g.watcherIntroduced(false) || len(g.watcherQueue) > 0 {
		return false
	}
	c := g.config.Watcher
	spent := 0
	for id := range g.watcherSeen {
		if l := watcherLine(id); l != nil && !l.Story {
			spent++
		}
	}
	return spent < c.MaxRandomLines &&
		(g.watcherLastDay == 0 || g.currentCalendarDay()-g.watcherLastDay >= c.MinDaysBetween)
}

// watcherCampLine picks the random remark a rest may carry; hurt is whether
// the party lay down wounded.
func (g *MMGame) watcherCampLine(hurt bool) *WatcherLine {
	if !g.watcherRandomReady() || rand.Float64() >= g.config.Watcher.CampChance {
		return nil
	}
	// Spent and logged when the strip shows his words (tickCampRest): a rest
	// cut short before that keeps the line for later.
	return g.pickWatcherLine(watcherAtCamp, hurt, func(*WatcherLine) bool { return true })
}

// noteWatcherKill queues his remark on a kill: a story boss's line always,
// each one in turn, and a waiting random remark gives way to it; otherwise, by
// chance, the monster's own line or - for a boss - one from the pool.
// Its one caller, noteStoryKill, has already passed the kill gate.
func (g *MMGame) noteWatcherKill(m *monster.Monster3D) {
	if globalWatcher == nil {
		return
	}
	own := func(story bool) func(*WatcherLine) bool {
		return func(l *WatcherLine) bool { return l.Story == story && slices.Contains(l.Monsters, m.Key) }
	}
	if l := g.pickWatcherLine(watcherOnKill, false, own(true)); l != nil {
		g.watcherQueue = slices.DeleteFunc(g.watcherQueue, func(id string) bool {
			q := watcherLine(id)
			return q == nil || !q.Story
		})
		if !slices.Contains(g.watcherQueue, l.ID) {
			g.watcherQueue = append(g.watcherQueue, l.ID)
		}
		return
	}
	if !g.watcherRandomReady() || rand.Float64() >= g.config.Watcher.KillChance {
		return
	}
	l := g.pickWatcherLine(watcherOnKill, false, own(false))
	if l == nil && m.IsBoss() {
		l = g.pickWatcherLine(watcherOnKill, false, func(l *WatcherLine) bool { return l.Boss && len(l.Monsters) == 0 })
	}
	if l != nil {
		g.watcherQueue = append(g.watcherQueue, l.ID)
	}
}

func watcherLine(id string) *WatcherLine {
	if globalWatcher == nil {
		return nil
	}
	for i := range globalWatcher.Lines {
		if globalWatcher.Lines[i].ID == id {
			return &globalWatcher.Lines[i]
		}
	}
	return nil
}

// watcherCard is his remark after a kill, shown over the view.
type watcherCard struct {
	line    *WatcherLine
	elapsed int
}

// tickWatcher shows the queued kill remarks one card at a time, once the
// fight is over and nothing else holds the screen, and runs the card's clock.
func (g *MMGame) tickWatcher() {
	if c := g.watcherCard; c != nil {
		if g.viewUncovered() { // his clock runs only while the card can be seen
			c.elapsed++
		}
		if c.elapsed >= g.watcherCardFrames() {
			g.watcherCard = nil
		}
		return
	}
	if len(g.watcherQueue) == 0 || !g.watcherIntroduced(false) || g.storyScreen != nil || len(g.storyQueue) > 0 || !g.storyCalm() {
		return
	}
	l := watcherLine(g.watcherQueue[0])
	g.watcherQueue = g.watcherQueue[1:]
	if l == nil || g.watcherSeen[l.ID] {
		return
	}
	g.speakWatcherLine(l)
	g.watcherCard = &watcherCard{line: l}
}

func (g *MMGame) watcherCardFrames() int {
	return g.framesForSeconds(g.config.Watcher.CardSeconds)
}

// resetWatcher clears his state for a fresh run.
func (g *MMGame) resetWatcher() {
	g.watcherSeen, g.watcherLastDay, g.watcherQueue, g.watcherCard = nil, 0, nil, nil
}

func (g *MMGame) watcherSaveState() (seen []string, lastDay int, queue []string) {
	for id, ok := range g.watcherSeen {
		if ok {
			seen = append(seen, id)
		}
	}
	sort.Strings(seen)
	return seen, g.watcherLastDay, slices.Clone(g.watcherQueue)
}

func (g *MMGame) restoreWatcher(save *GameSave) {
	g.resetWatcher()
	for _, id := range save.WatcherSeen {
		if watcherLine(id) != nil {
			if g.watcherSeen == nil {
				g.watcherSeen = map[string]bool{}
			}
			g.watcherSeen[id] = true
		}
	}
	g.watcherLastDay = save.WatcherLastDay
	for _, id := range save.WatcherQueue {
		if watcherLine(id) != nil {
			g.watcherQueue = append(g.watcherQueue, id)
		}
	}
}

const (
	watcherCardW    = 420
	watcherCardPad  = 12
	watcherPortrait = 64
	watcherCardTop  = 96
	watcherStripMax = 760
)

// watcherCardLayout places the card at the top centre of the view.
func watcherCardLayout(viewW int, lines int) (card, portrait, text layoutRect) {
	h := max(watcherPortrait, uiTextCharHeight+4+lines*storyLineH) + 2*watcherCardPad
	card = layoutRect{(viewW - watcherCardW) / 2, watcherCardTop, watcherCardW, h}
	portrait = layoutRect{card.x + watcherCardPad, card.y + watcherCardPad, watcherPortrait, watcherPortrait}
	x := portrait.right() + watcherCardPad
	text = layoutRect{x, card.y + watcherCardPad, card.right() - watcherCardPad - x, h - 2*watcherCardPad}
	return
}

func (ui *UISystem) drawWatcherCard(screen *ebiten.Image) {
	g := ui.game
	c := g.watcherCard
	// Under the open menu the card neither shows nor takes the menu's clicks;
	// its clock stands still while anything covers it (tickWatcher).
	if c == nil || globalWatcher == nil || g.menuOpen {
		return
	}
	_, _, probe := watcherCardLayout(g.config.GetScreenWidth(), 1)
	lines := wrapUIText(c.line.Text, probe.w)
	card, portrait, text := watcherCardLayout(g.config.GetScreenWidth(), len(lines))
	total := g.watcherCardFrames()
	fade := float64(g.framesForSeconds(0.2))
	alpha := min(1, float64(c.elapsed)/fade, float64(total-c.elapsed)/fade)
	if alpha <= 0 {
		return
	}
	drawFilledRect(screen, card.x, card.y, card.w, card.h, fadeVectorColor(interfacePanelFill, 0.92*alpha))
	ui.drawThemeFrameAlpha(screen, frameBronze, card.x, card.y, card.w, card.h, alpha)
	if img := g.sprites.GetSprite(globalWatcher.Portrait); img != nil {
		op := &ebiten.DrawImageOptions{}
		op.ColorScale.ScaleAlpha(float32(alpha))
		uiDrawImageScaled(screen, img, float64(portrait.x), float64(portrait.y), float64(portrait.w), float64(portrait.h), op)
	}
	drawStoryText(screen, globalWatcher.Speaker, text.x, text.y, gameplayInk.heading, alpha)
	for i, line := range lines {
		drawStoryText(screen, line, text.x, text.y+uiTextCharHeight+4+i*storyLineH, storyCaptainInk, alpha)
	}
	if captainLightOn(c.elapsed, g.storyTiming()) {
		drawCaptainLight(screen, float32(card.right()-watcherCardPad-4), float32(card.y+watcherCardPad+4), 3.5, alpha)
	}
	ui.onGatedClick(card, func() bool { return !ui.modalLayerOwnsInput() && g.watcherCard == c }, func() { g.watcherCard = nil })
}

// drawCampWatcherLine draws his camp remark over the camp scene: the light
// blinks first, then the words, in a strip above the viewport's foot.
func (ui *UISystem) drawCampWatcherLine(screen *ebiten.Image, view image.Rectangle) {
	g, s := ui.game, ui.game.campRest
	if s == nil || s.watcherLine == nil || s.elapsed < s.fadeIn || s.elapsed > s.fadeIn+s.hold {
		return
	}
	t := g.storyTiming()
	k := s.elapsed - s.fadeIn
	w := min(watcherStripMax, view.Dx()-2*storyMargin)
	textW := w - 2*storyPad - storyLightGap
	lines := wrapUIText(s.watcherLine.Text, textW)
	// The scene waits for the player: the hint shares the last row when it fits.
	hint := uitext.Text("ui.story_continue")
	rows := len(lines)
	if uiTextWidth(lines[len(lines)-1])+2*storyPad+uiTextWidth(hint) > textW {
		rows++
	}
	h := rows*storyLineH + 2*storyPad
	strip := layoutRect{view.Min.X + (view.Dx()-w)/2, view.Max.Y - storyMargin - h, w, h}
	drawFilledRect(screen, strip.x, strip.y, strip.w, strip.h, fadeVectorColor(interfacePanelFill, 0.82))
	if captainLightOn(k, t) {
		drawCaptainLight(screen, float32(strip.x+storyPad+6), float32(strip.y+storyPad+storyLineH/2), 3.5, 1)
	}
	if k < t.blinks*t.blink {
		return
	}
	a := smoothStep(float64(k-t.blinks*t.blink) / float64(t.textFade))
	for i, line := range lines {
		drawStoryText(screen, line, strip.x+storyPad+storyLightGap, strip.y+storyPad+i*storyLineH, storyCaptainInk, a)
	}
	drawStoryText(screen, hint, strip.right()-storyPad-uiTextWidth(hint), strip.y+storyPad+(rows-1)*storyLineH, gameplayInk.muted, a)
}

// campWatcherHolding: the camp scene waits on his words until they are read.
func (s *campRestPresentation) campWatcherHolding() bool {
	return s.watcherLine != nil && !s.watcherRead && s.elapsed >= s.fadeIn+s.hold
}

// releaseCampWatcher lets a held camp scene fade out.
// A press while the light still blinks shows his words at once; the next
// press, once they are up, lets the scene go.
func (g *MMGame) releaseCampWatcher() {
	s := g.campRest
	if s == nil || s.watcherLine == nil {
		return
	}
	if t := g.storyTiming(); !s.watcherWordsUp(t) {
		s.elapsed = min(s.watcherWordsAt(t)+t.textFade, s.fadeIn+s.hold) // the strip draws only within the hold
		return
	}
	s.watcherRead = true
}

// watcherWordsUp: his words have fully faded in - the next press lets the
// scene go.
func (s *campRestPresentation) watcherWordsUp(t storyTiming) bool {
	return s.elapsed >= s.watcherWordsAt(t)+t.textFade
}

// watcherWordsAt is the scene frame his words start to appear: after the
// fade-in and the light's blinks.
func (s *campRestPresentation) watcherWordsAt(t storyTiming) int {
	return s.fadeIn + t.blinks*t.blink
}
