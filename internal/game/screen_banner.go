package game

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"ugataima/internal/character"
	"ugataima/internal/items"
)

// SCREEN BANNERS: the one top-of-screen heading. This file owns the channel -
// queue, timeline, geometry, draw. Producers push through queueBanner and pick a
// kind (tint + hold): quest events (quest_banner.go), the approach prompt
// (syncInteractPromptBanner), legendary loot (announceLegendaryDrops, off
// addGroundContainer). A new source is a kind plus one call, never an overlay.

type screenBannerKind int

const (
	bannerQuestTaken screenBannerKind = iota
	bannerQuestProgress
	bannerQuestDone
	bannerQuestPaid
	bannerInteractPrompt
	bannerLegendaryDrop
)

// Timeline, in SECONDS: slide in, hold, slide out. Converted with the live tps
// (framesForSeconds) - written as frames these all halved on the shipped 120-tps
// build, which is how "hangs about a second" became half of one.
const (
	bannerInSeconds  = 0.37
	bannerOutSeconds = 0.43

	// A prompt is a nudge, not news: it hangs about a second so walking a street
	// of NPCs does not queue a minute of "Press SPACE".
	bannerDefaultHoldSeconds = 1.8
	bannerPromptHoldSeconds  = 0.9

	bannerTravelPx  = 30  // how far above its rest position it enters from
	bannerRestY     = 58  // rest position of the banner's centre line
	bannerScale     = 2.0 // smaller than the victory heading, same metal body
	bannerPlatePadX = 28
	bannerPlatePadY = 10

	// A backlog means the player is mid-slaughter; keep the newest few and drop
	// the rest rather than replaying a minute of counters.
	bannerQueueMax = 4

	// How long interact focus must stay lost before the approach counts as over.
	// Combat drops focus entirely, and a wandering monster is not the party
	// walking away; two seconds is longer than any such blip and far shorter
	// than crossing a room.
	bannerPromptForgetSeconds = 2.0
)

type screenBanner struct {
	text  string
	kind  screenBannerKind
	frame int
}

// bannerHoldFrames is how long this kind hangs at rest.
func (g *MMGame) bannerHoldFrames(kind screenBannerKind) int {
	if kind == bannerInteractPrompt {
		return g.framesForSeconds(bannerPromptHoldSeconds)
	}
	return g.framesForSeconds(bannerDefaultHoldSeconds)
}

// bannerLifetime is the full timeline of a kind.
func (g *MMGame) bannerLifetime(kind screenBannerKind) int {
	return g.bannerInFrames() + g.bannerHoldFrames(kind) + g.bannerOutFrames()
}

// bannerInFrames / bannerOutFrames are the slide legs at the live tick rate.
func (g *MMGame) bannerInFrames() int  { return g.framesForSeconds(bannerInSeconds) }
func (g *MMGame) bannerOutFrames() int { return g.framesForSeconds(bannerOutSeconds) }

// screenBannerTint is the colour per kind. The metal tints pick up the heading
// renderer's brushed gradient; the nudge's white stays flat.
func screenBannerTint(kind screenBannerKind) color.RGBA {
	switch kind {
	case bannerQuestDone, bannerQuestPaid:
		return rarityGold
	case bannerInteractPrompt:
		return color.RGBA{255, 255, 255, 255}
	case bannerLegendaryDrop:
		// Literally the tint item names wear at this rarity - one palette.
		return rarityRGBA(bannerLegendaryRarity)
	default:
		return bannerWorkTint
	}
}

// bannerWorkTint is the pale gold of an errand in progress. Must stay in
// metallicColors or it renders flat while the other quest banners do not.
var bannerWorkTint = color.RGBA{238, 219, 164, 255}

// dropPrompts removes approach nudges; keepOnScreen spares the one on screen.
// A queued nudge must never surface later - the party has walked away by then.
func (g *MMGame) dropPrompts(keepOnScreen bool) {
	kept := g.screenBannerQueue[:0]
	for i, b := range g.screenBannerQueue {
		if b.kind == bannerInteractPrompt && !(keepOnScreen && i == 0) {
			continue
		}
		kept = append(kept, b)
	}
	g.screenBannerQueue = kept
}

// queueBanner is the ONE entry point into the channel. Reports whether the line
// actually went in: a nudge is refused by a full queue or by a duplicate, and the
// prompt producer must not mark an NPC as prompted for a banner nobody saw.
func (g *MMGame) queueBanner(kind screenBannerKind, text string) bool {
	if text == "" {
		return false
	}
	banner := screenBanner{text: text, kind: kind}
	if kind == bannerInteractPrompt {
		// Only the newest nudge is worth showing: drop the prompts still waiting
		// their turn, and restart the one on screen instead of following it with a
		// second.
		g.dropPrompts(true)
		if n := len(g.screenBannerQueue); n > 0 && g.screenBannerQueue[0].kind == bannerInteractPrompt {
			// Swap the TEXT, keep the timeline: focus flickers between adjacent
			// props, and restarting per flip pins the nudge in its slide-in.
			on := &g.screenBannerQueue[0]
			on.text = banner.text
			// Past the hold the old nudge is spent: restart, or the new target
			// flashes for a few frames at falling alpha.
			if on.frame >= g.bannerInFrames()+g.bannerHoldFrames(bannerInteractPrompt) {
				on.frame = 0
			}
			return true // the nudge on screen was restarted: the player sees it
		}
		// A nudge never displaces news: with the queue full it is dropped. It is
		// the one banner the player can recreate by stepping back.
		if len(g.screenBannerQueue) >= bannerQueueMax {
			return false
		}
	}
	// Never queue the same line twice in a row (two sources crediting one kill).
	if n := len(g.screenBannerQueue); n > 0 && g.screenBannerQueue[n-1].text == banner.text {
		return false
	}
	g.screenBannerQueue = append(g.screenBannerQueue, banner)
	g.trimBannerOverflow()
	return true
}

// trimBannerOverflow cuts the queue back to bannerQueueMax. A nudge goes first
// wherever it sits - INCLUDING the one mid-animation at index 0: it pops instead
// of sliding out, and that is the deliberate trade. A nudge is recoverable (the
// player steps away, or returns from an overlay); a quest banner never shown is
// gone for good. Only news is protected by taking index 0 out of the fallback.
func (g *MMGame) trimBannerOverflow() {
	for len(g.screenBannerQueue) > bannerQueueMax {
		drop := 1 // the oldest entry still waiting its turn
		for i, b := range g.screenBannerQueue {
			if b.kind == bannerInteractPrompt {
				drop = i
				break
			}
		}
		g.screenBannerQueue = append(g.screenBannerQueue[:drop], g.screenBannerQueue[drop+1:]...)
	}
}

// forgetInteractPromptTarget clears the focus identity and any queued nudge: a
// map change ends every approach, and a held pointer keeps the old NPC alive.
func (g *MMGame) forgetInteractPromptTarget() {
	g.bannerPromptNPC = nil
	g.bannerPromptLostFrames = 0
	g.dropPrompts(false)
}

// resetScreenBanners clears the channel and every producer's memory: queue,
// focus identity, pending drops, and a fresh silent quest baseline.
func (g *MMGame) resetScreenBanners() {
	g.screenBannerQueue = nil
	g.bannerPromptNPC = nil
	g.bannerPromptLostFrames = 0
	g.pendingLegendaryDrops = nil // loot collected but not yet announced dies with the run
	g.resyncQuestBannerBaseline()
}

// tickScreenBanners runs the producers and ages the banner on screen. One call
// per simulation frame, so banners freeze while a menu pauses the world.
func (g *MMGame) tickScreenBanners() {
	g.flushLegendaryDropBanner()
	g.syncQuestBanners(true)
	g.syncInteractPromptBanner()
	if len(g.screenBannerQueue) == 0 {
		return
	}
	g.screenBannerQueue[0].frame++
	if g.screenBannerQueue[0].frame >= g.bannerLifetime(g.screenBannerQueue[0].kind) {
		g.screenBannerQueue = g.screenBannerQueue[1:]
	}
}

// currentScreenBanner is the banner at the head of the queue, or nil. This is
// the QUEUE's view - it keeps its place while the world is paused.
func (g *MMGame) currentScreenBanner() *screenBanner {
	if len(g.screenBannerQueue) == 0 {
		return nil
	}
	return &g.screenBannerQueue[0]
}

// visibleScreenBanner is the RENDERER's view: nothing while an overlay paused
// the world (the tick stops, the HUD keeps drawing). The map and Game Over used
// to be named here on top of that check because they did not pause; they do now,
// so the pause contract covers them.
func (g *MMGame) visibleScreenBanner() *screenBanner {
	if g.gameplayPausedByOverlay() {
		return nil
	}
	banner := g.currentScreenBanner()
	// Quest news and drops DO belong over an open dialog (a turn-in banners while
	// you are still talking). An approach nudge does not: the party is already
	// talking to the thing it points at.
	if banner != nil && banner.kind == bannerInteractPrompt && g.dialogActive {
		return nil
	}
	return banner
}

// npcInCurrentWorld reports whether this NPC belongs to the world the party is
// standing in right now.
func (g *MMGame) npcInCurrentWorld(npc *character.NPC) bool {
	w := g.GetCurrentWorld()
	if w == nil {
		return false
	}
	for _, other := range w.NPCs {
		if other == npc {
			return true
		}
	}
	return false
}

// noteInteractPromptEngaged settles this object's nudge when the party acts on
// it: the queued one goes, and the object counts as prompted.
func (g *MMGame) noteInteractPromptEngaged(npc *character.NPC) {
	g.bannerPromptLostFrames = 0
	// Including the one on screen: the tick runs during a dialog, so it would
	// hover over the conversation it announced.
	g.dropPrompts(false)
	g.bannerPromptNPC = npc
}

// syncInteractPromptBanner raises the "Press SPACE to ..." nudge once per
// approach, on the focus edge. Wording comes from interactionPromptText.
func (g *MMGame) syncInteractPromptBanner() {
	npc := g.focusedNPC
	if npc == nil {
		if g.bannerPromptNPC == nil {
			return
		}
		// A queued nudge now points at something the party left.
		g.dropPrompts(true)
		// The identity survives a grace period: combat nils focus wholesale, and
		// that is not the party walking away.
		g.bannerPromptLostFrames++
		if g.bannerPromptLostFrames >= g.framesForSeconds(bannerPromptForgetSeconds) {
			g.bannerPromptNPC = nil
		}
		return
	}
	g.bannerPromptLostFrames = 0
	if npc == g.bannerPromptNPC {
		return
	}
	// Focus keeps resolving during a dialog; recording a new target as prompted
	// here would silence it once the talk ends.
	if g.dialogActive {
		return
	}
	// Focus survives a map change until the next resolve, so it can point into
	// the map just left. Never announce something that is not HERE.
	if !g.npcInCurrentWorld(npc) {
		g.bannerPromptNPC = nil
		// And its queued nudge: the underwater return and encounter-map entry do
		// not go through switchToMap.
		g.dropPrompts(false)
		return
	}
	// Record the target only if the nudge actually went up. A queue full of quest
	// news refuses it, and marking the NPC as prompted then would swallow the
	// approach for good: the identity short-circuits every later frame, and only
	// bannerPromptForgetSeconds of fully lost focus would clear it. Refused, the
	// producer simply tries again next frame, when the queue has drained.
	// (Anything queued for the previous target is stale; queueBanner drops it.)
	if g.queueBanner(bannerInteractPrompt, g.interactionPromptText(npc)) {
		g.bannerPromptNPC = npc
	}
}

// bannerLegendaryRarity is the rarity that earns its own banner.
const bannerLegendaryRarity = "legendary"

// announceLegendaryDrops COLLECTS the legendaries in a drop; the next tick turns
// everything collected into ONE heading. It hangs off addGroundContainer, the
// funnel every floor container passes, so every death path announces alike -
// and one event can drop several containers (the pyramid dais opens four).
func (g *MMGame) announceLegendaryDrops(drops []items.Item) {
	for _, it := range drops {
		if strings.EqualFold(it.Rarity, bannerLegendaryRarity) {
			g.pendingLegendaryDrops = append(g.pendingLegendaryDrops, it.Name)
		}
	}
}

// flushLegendaryDropBanner raises one heading for everything collected.
func (g *MMGame) flushLegendaryDropBanner() {
	if len(g.pendingLegendaryDrops) == 0 {
		return
	}
	// Hand the slice over; [:0] would share the array with a later append.
	names := g.pendingLegendaryDrops
	g.pendingLegendaryDrops = nil
	g.queueBanner(bannerLegendaryDrop, legendaryDropBannerText(names, g.config.GetScreenWidth()))
}

// legendaryDropBannerText is the wording; names past the band are counted, not
// clipped mid-word.
func legendaryDropBannerText(names []string, screenW int) string {
	label := "Legendary drop - "
	if len(names) > 1 {
		label = "Legendary drops - "
	}
	full := label + strings.Join(names, ", ")
	// A lone name is always shown: "1 items" says less than a clipped title.
	if len(names) == 1 || screenBannerFits(full, screenW) {
		return full
	}
	// Drop names off the tail until the line fits, then say how many are unnamed.
	for shown := len(names) - 1; shown >= 1; shown-- {
		text := fmt.Sprintf("%s%s +%d more", label, strings.Join(names[:shown], ", "), len(names)-shown)
		if screenBannerFits(text, screenW) {
			return text
		}
	}
	return fmt.Sprintf("%s%d items", label, len(names))
}

// screenBannerFits asks the renderer's own layout whether a line survives uncut.
func screenBannerFits(text string, screenW int) bool {
	return screenBannerLayout(screenW, text, 0).text == text
}

// screenBannerAnim maps a banner's age to its fade and its offset ABOVE the rest
// position: it drops in, hangs, and rises back out the way it came.
func (g *MMGame) screenBannerAnim(frame, hold int) (alpha, offsetY float64) {
	bannerInFrames, bannerOutFrames := g.bannerInFrames(), g.bannerOutFrames()
	switch {
	case frame < bannerInFrames:
		p := smoothStep(float64(frame) / float64(bannerInFrames))
		return p, (1 - p) * bannerTravelPx
	case frame < bannerInFrames+hold:
		return 1, 0
	default:
		p := smoothStep(float64(frame-bannerInFrames-hold) / float64(bannerOutFrames))
		return 1 - p, p * bannerTravelPx
	}
}

// smoothStep eases both ends of a 0..1 ramp so the slide has no hard start.
func smoothStep(t float64) float64 {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return t * t * (3 - 2*t)
}

// screenBannerGeometry is the banner's placement for one frame - the single
// geometry source the renderer draws from and the tests measure.
type screenBannerGeometry struct {
	text           string // clipped to the band between the corner readouts
	cx, cy         int    // heading centre
	plateX, plateY int
	plateW, plateH int
	textW          int
}

// The top corners are taken: "ESC: Main menu" on the left, the mode readout and
// the compass column on the right. The banner keeps clear of both.
const bannerCornerReservePx = 130

func screenBannerLayout(screenW int, text string, offset float64) screenBannerGeometry {
	maxTextPx := screenW - 2*(bannerPlatePadX+bannerCornerReservePx)
	if maxTextPx < 120 {
		maxTextPx = 120
	}
	scale := float64(bannerScale)
	clipped := clipDebugText(text, int(float64(maxTextPx)/scale))
	g := screenBannerGeometry{
		text:  clipped,
		textW: int(float64(debugTextWidth(clipped)) * scale),
		cx:    screenW / 2,
		cy:    bannerRestY - int(offset),
	}
	g.plateW = g.textW + 2*bannerPlatePadX
	g.plateH = int(float64(debugTextCharHeight)*scale) + 2*bannerPlatePadY
	g.plateX = g.cx - g.plateW/2
	g.plateY = g.cy - g.plateH/2
	return g
}

// drawScreenBanner paints the current banner: a fading plate under a scaled
// metal heading, centred at the top of the screen between the corner readouts.
func (ui *UISystem) drawScreenBanner(screen *ebiten.Image) {
	banner := ui.game.visibleScreenBanner()
	if banner == nil {
		return
	}
	alpha, offset := ui.game.screenBannerAnim(banner.frame, ui.game.bannerHoldFrames(banner.kind))
	if alpha <= 0 {
		return
	}
	tint := screenBannerTint(banner.kind)
	geo := screenBannerLayout(ui.game.config.GetScreenWidth(), banner.text, offset)
	// Same furniture as the victory title: a dark panel, brushed-metal rules, and
	// the scaled metal heading itself.
	drawFilledRect(screen, geo.plateX, geo.plateY, geo.plateW, geo.plateH, fadeVectorColor(color.RGBA{8, 7, 3, 210}, alpha))
	rule := fadeVectorColor(metalPlateBase(tint), alpha)
	drawMetalPlate(screen, geo.plateX, geo.plateY, geo.plateW, 2, rule)
	drawMetalPlate(screen, geo.plateX, geo.plateY+geo.plateH-2, geo.plateW, 2, rule)

	drawScaledMetalCenteredTextAlpha(screen, geo.text, geo.cx, geo.cy, bannerScale, tint, alpha)
}

// fadeVectorColor fades a colour for an animating vector fill. ebiten's vector
// helpers treat colour as PREMULTIPLIED: scale RGB with A, or a faint glow
// renders as a solid disc.
func fadeVectorColor(c color.RGBA, alpha float64) color.RGBA {
	if alpha >= 1 {
		return c
	}
	if alpha < 0 {
		alpha = 0
	}
	return color.RGBA{
		R: uint8(float64(c.R) * alpha),
		G: uint8(float64(c.G) * alpha),
		B: uint8(float64(c.B) * alpha),
		A: uint8(float64(c.A) * alpha),
	}
}
