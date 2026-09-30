package game

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"ugataima/internal/config"
	"ugataima/internal/playerprofile"

	"github.com/hajimehoshi/ebiten/v2"
)

var profileGold = color.RGBA{226, 197, 131, 255}
var profileMuted = color.RGBA{164, 164, 175, 255}
var profileGreen = color.RGBA{131, 194, 137, 255}

// The existing nine-sliced menu frame supplies the measured outer inset. All
// children share this rectangle; responsive layouts do not stretch frame art.
func profilePanelRect(w, h int) layoutRect {
	pw, ph := min(w-32, 1320), min(h-32, 840)
	return centeredRect(w, h, pw, ph)
}

// Profile card rectangles include their frame, unlike drawRectBorder's
// content rectangles. Inset the content once so every card edge stays inside
// the viewport when its own rectangle touches the clip boundary.
func (ui *UISystem) drawProfileCard(dst *ebiten.Image, r layoutRect, highlighted bool) {
	style := frameSilver
	if highlighted {
		style = frameGold
	}
	ui.drawThemeFrame(dst, style, r.x, r.y, r.w, r.h)
}

func (ui *UISystem) profileButton(screen *ebiten.Image, label string, r layoutRect, enabled bool, action func()) {
	mx, my := pointerPosition()
	ui.drawMenuButton(screen, label, r.x, r.y, r.w, r.h, enabled && isMouseHoveringBox(mx, my, r.x, r.y, r.x+r.w, r.y+r.h))
	if !enabled {
		drawFilledRect(screen, r.x, r.y, r.w, r.h, color.RGBA{0, 0, 0, 100})
		return
	}
	ui.onDisplayedInput(uiCommandNavigation, r, func() {
		if ui.game.consumeLeftClickIn(r.x, r.y, r.x+r.w, r.y+r.h) {
			action()
		}
	})
}

func (ui *UISystem) profileIcon(screen *ebiten.Image, key, label string, x, y, size int) {
	if y+size <= uiBounds(screen).Min.Y || y >= uiBounds(screen).Max.Y {
		return
	}
	var img *ebiten.Image
	if strings.HasPrefix(key, "sky:") {
		if ui.profileArt == nil {
			ui.profileArt = newProfileArt()
		}
		img = ui.profileArt.thumbnail(strings.TrimPrefix(key, "sky:"))
	} else if strings.HasPrefix(key, "monster:") {
		name := strings.TrimPrefix(key, "monster:")
		for _, animType := range []string{"walking_r", "walking_l"} {
			if anim := ui.game.sprites.GetAnimation(name, animType); anim != nil && len(anim.Frames) > 0 {
				img = anim.Frames[0]
				break
			}
		}
		if img == nil && ui.game.sprites.HasSprite(name) {
			img = ui.game.sprites.GetSprite(name)
		}
	} else if key != "" && ui.game.sprites.HasSprite(key) {
		img = ui.game.sprites.GetSprite(key)
	}
	drawFilledRect(screen, x, y, size, size, color.RGBA{10, 9, 14, 240})
	if img != nil {
		b := img.Bounds()
		dw, dh := size, size
		if b.Dx() > b.Dy() {
			dh = max(1, size*b.Dy()/b.Dx())
		} else {
			dw = max(1, size*b.Dx()/b.Dy())
		}
		drawImageScaled(screen, img, x+(size-dw)/2, y+(size-dh)/2, dw, dh)
	} else {
		drawCenteredUIText(screen, spellInitials(label), x, y, size, size)
	}
	if !strings.HasPrefix(key, "icon_") {
		drawRectBorder(screen, x, y, size, size, 1, profileGold)
	}
}

func profileText(s string, width int) string {
	return clipUIText(s, max(uiTextCharWidth, width))
}

type profileAchievementsLayout struct {
	panel, body                         layoutRect
	columns, columnW, contentH, footerY int
}

func makeProfileAchievementsLayout(w, h, count int) profileAchievementsLayout {
	r := profilePanelRect(w, h)
	iw := r.w - 2*menuFrameInset
	cols := 1
	if iw >= 820 {
		cols = 2
	}
	footer := r.bottom() - menuFrameInset - menuBackButtonH
	bodyY := r.y + menuFrameInset + 64
	return profileAchievementsLayout{panel: r, body: layoutRect{r.x + menuFrameInset, bodyY, iw, max(1, footer-bodyY-16)}, columns: cols, columnW: (iw - 12 - (cols-1)*16) / cols, contentH: ((count + cols - 1) / cols) * 104, footerY: footer}
}

func (g *MMGame) updateAchievementsKeys(pressed func(ebiten.Key) bool) {
	l := makeProfileAchievementsLayout(g.config.GetScreenWidth(), g.config.GetScreenHeight(), len(config.GetAchievements()))
	_, wheel := pointerWheel()
	mx, my := pointerPosition()
	if !isMouseHoveringBox(mx, my, l.body.x, l.body.y, l.body.right(), l.body.bottom()) {
		wheel = 0
	}
	g.achievementsScroll = profileScrollInput(g.achievementsScroll, max(0, l.contentH-l.body.h), l.body.h, wheel, pressed)
}

func (ui *UISystem) drawAchievementsScreen(screen *ebiten.Image, w, h int) {
	g := ui.game
	defs := config.GetAchievements()
	l := makeProfileAchievementsLayout(w, h, len(defs))
	r := l.panel
	ui.drawThemeFrame(screen, frameGold, r.x, r.y, r.w, r.h)
	ui.drawPanelInlay(screen, frameGold, r.x+r.w/2, r.y)
	ui.drawCornerDecor(screen, frameGold, r.x-8, r.y-8, r.w+16, r.h+16, decorAllCorners)
	x, y := l.body.x, r.y+menuFrameInset
	unlockedCount := 0
	for _, def := range defs {
		if g.playerProfile != nil {
			if _, ok := g.playerProfile.Data.Unlocked[def.Key]; ok {
				unlockedCount++
			}
		}
	}
	drawScaledMetalCenteredTextAlpha(screen, "ACHIEVEMENTS", r.x+r.w/2, y+10, 2, profileGold, 1)
	drawUITextColored(screen, fmt.Sprintf("%d / %d earned across all adventures", unlockedCount, len(defs)), x, y+36, profileMuted)
	drawFilledRect(screen, x, y+53, l.body.w, 3, color.RGBA{50, 41, 43, 255})
	drawFilledRect(screen, x, y+53, l.body.w*unlockedCount/max(1, len(defs)), 3, profileGold)
	g.achievementsScroll = max(0, min(g.achievementsScroll, max(0, l.contentH-l.body.h)))
	clip := image.Rect(l.body.x, l.body.y, l.body.right(), l.body.bottom()).Intersect(uiBounds(screen))
	dst := uiClip(screen, clip)
	for idx, def := range defs {
		cx, cy := x+(idx%l.columns)*(l.columnW+16), l.body.y+(idx/l.columns)*104-g.achievementsScroll
		if cy+96 <= clip.Min.Y || cy >= clip.Max.Y {
			continue
		}
		stamp := time.Time{}
		progress := int64(0)
		if g.playerProfile != nil {
			stamp = g.playerProfile.Data.Unlocked[def.Key]
			progress = g.playerProfile.Data.AchievementProgress(def.AnyOf)
		}
		unlocked := !stamp.IsZero()
		ui.drawProfileCard(dst, layoutRect{cx, cy, l.columnW, 94}, unlocked)
		ui.profileIcon(dst, def.Icon, def.Name, cx+10, cy+14, 64)
		if !unlocked {
			drawFilledRect(dst, cx+10, cy+14, 64, 64, color.RGBA{0, 0, 0, 115})
		}
		tx, tw := cx+86, l.columnW-100
		drawUITextColored(dst, profileText(def.Name, tw), tx, cy+11, profileGold)
		for i, line := range wrapArenaBoardLine(def.Description, tw) {
			if i >= 2 {
				break
			}
			drawUITextColored(dst, line, tx, cy+31+i*14, profileMuted)
		}
		status := fmt.Sprintf("%d / %d", min(progress, def.Target), def.Target)
		clr := profileMuted
		if unlocked {
			status = "Earned " + stamp.Local().Format("02 Jan 2006")
			clr = profileGreen
			progress = def.Target
		}
		drawUITextColored(dst, status, tx, cy+64, clr)
		drawFilledRect(dst, tx, cy+81, tw, 3, color.RGBA{47, 39, 45, 255})
		drawFilledRect(dst, tx, cy+81, int(int64(tw)*min(progress, def.Target)/max(int64(1), def.Target)), 3, clr)
	}
	ui.drawScrollbar(screen, "profile:achievements", layoutRect{l.body.right() - 5, l.body.y, 3, l.body.h}, g.achievementsScroll, l.contentH, true, func(v int) {
		g.achievementsScroll = v
	})
	ui.drawBackButton(screen, x, l.footerY, func() { g.entryMenuMode = EntryMenuRoot })
	drawUITextColored(screen, "Scroll / PgUp / PgDn", x+126, l.footerY+9, profileMuted)
	ui.drawProfileError(screen, x, l.footerY-16, l.body.w)
}

type profileRankingSpec struct {
	title, group, unit string
	duration           bool
}
type profileCounterSpec struct {
	title, key, icon string
	duration         bool
}
type profilePageSpec struct {
	title    string
	counters []profileCounterSpec
	rankings []profileRankingSpec
}

var profilePages = []profilePageSpec{
	{"Overview", []profileCounterSpec{{"Adventures", "adventures", "icon_achievement_first_steps", false}, {"Victories", "victories", "icon_achievement_victory", false}, {"Active play time", "play_ns", "icon_achievement_full_roster", true}}, []profileRankingSpec{{"Favorite classes", "classes", "hero time", true}, {"Favorite spells", "spells", "casts", false}, {"Favorite regions", "regions", "time spent", true}}},
	{"Combat", []profileCounterSpec{{"Monsters defeated", "kills", "icon_achievement_first_blood", false}, {"HP lost to attacks", "monster_damage", "icon_achievement_warlord", false}, {"Heroes knocked out", "knockouts", "icon_achievement_lich", false}}, []profileRankingSpec{{"Most hunted", "kills", "defeated", false}, {"Most dangerous", "danger", "HP lost", false}, {"Most knockouts caused", "knockouts", "knockouts", false}}},
	{"Discoveries", []profileCounterSpec{{"Loot found", "loot", "icon_achievement_jailbreak", false}, {"Gold earned", "gold", "icon_achievement_victory", false}, {"Regions explored", "exploration", "icon_item_world_map", false}}, []profileRankingSpec{{"Most common loot", "loot", "units found", false}, {"Regions explored", "exploration", "% of all region tiles", false}, {"Most valuable finds", "valuable_loot", "highest base gold per item", false}}},
	{"Trophies", []profileCounterSpec{{"Bosses defeated", "bosses", "icon_achievement_warlord", false}, {"Items traded", "items_traded", "icon_item_clock_hand", false}, {"Quests completed", "quest_rewards", "icon_achievement_archmage", false}}, []profileRankingSpec{{"Bosses defeated", "bosses", "kills by boss", false}, {"Trade offerings", "items_traded", "item units paid to merchants", false}, {"Quests completed", "quest_rewards", "completions by quest", false}}},
	{"Collecting", []profileCounterSpec{{"Cards found", "cards_found", "icon_item_goblin_card", false}, {"Chest loot", "chest_loot", "chest_golden", false}, {"Legendary drops", "legendary_loot", "icon_weapon_wyrmcleaver", false}}, []profileRankingSpec{{"Cards found", "cards_found", "units found, all rarities", false}, {"Loot from chests", "chest_loot", "item units, excludes gold", false}, {"Legendary loot", "legendary_loot", "units found, excludes cards", false}}},
	{"Arena", []profileCounterSpec{{"Champions defeated", "arena_wins", "icon_achievement_warlord", false}, {"Arena points earned", "arena_points", "icon_achievement_victory", false}, {"Victorious parties", "arena_parties", "icon_achievement_full_roster", false}}, []profileRankingSpec{{"Defeated champions", "arena_champions", "victories", false}, {"Difficulty tiers", "arena_tiers", "victories", false}, {"Party records", "arena_parties", "points earned", false}}},
}

func profileTabRect(x, y, width, index int) layoutRect {
	count := len(profilePages)
	gap := 12
	tabW := min(180, (width-(count-1)*gap)/count)
	left := x + (width-(count*tabW+(count-1)*gap))/2
	return layoutRect{left + index*(tabW+gap), y + 34, tabW, 30}
}

func profileValue(n int64, duration bool) string {
	if duration {
		d := time.Duration(n)
		if d < time.Hour {
			return fmt.Sprintf("%dm", int64(d/time.Minute))
		}
		return fmt.Sprintf("%dh %02dm", int64(d/time.Hour), int64(d/time.Minute)%60)
	}
	s := fmt.Sprintf("%d", n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

const profileRankingRowH = 64

type profileStatsLayout struct {
	panel, body                                                     layoutRect
	columns, columnW, counterRows, rankRows, rankY, rankH, contentH int
	footerY                                                         int
	counterColumns, counterW                                        int
}

func makeProfileStatsLayout(w, h int, page profilePageSpec) profileStatsLayout {
	r := profilePanelRect(w, h)
	x, iw := r.x+menuFrameInset, r.w-2*menuFrameInset
	columns := 1
	if iw >= 1040 {
		columns = min(3, len(page.rankings))
	} else if iw >= 640 {
		columns = 2
	}
	footer := r.y + r.h - menuFrameInset - 30
	bodyY := r.y + menuFrameInset + 76
	l := profileStatsLayout{panel: r, body: layoutRect{x, bodyY, iw, max(1, footer-bodyY-14)}, columns: columns, columnW: (iw - 12 - (columns-1)*14) / columns, footerY: footer}
	l.counterColumns = columns
	if iw >= 600 {
		l.counterColumns = min(3, len(page.counters))
	}
	l.counterW = (iw - 12 - (l.counterColumns-1)*14) / l.counterColumns
	l.counterRows = (len(page.counters) + l.counterColumns - 1) / l.counterColumns
	l.rankRows = (len(page.rankings) + columns - 1) / columns
	l.rankY = l.counterRows*100 + 8
	l.rankH = 360
	l.contentH = l.rankY + l.rankRows*(l.rankH+14) + 106
	return l
}

func (l profileStatsLayout) rankingRect(i, scroll int) layoutRect {
	return layoutRect{l.body.x + (i%l.columns)*(l.columnW+14), l.body.y - scroll + l.rankY + (i/l.columns)*(l.rankH+14), l.columnW, l.rankH}
}

func profileRankingBody(r layoutRect) layoutRect {
	return layoutRect{r.x + 8, r.y + 48, r.w - 16, max(1, r.h-76)}
}

func (g *MMGame) setStatisticsTab(tab int) {
	if g.statisticsTabScroll == nil {
		g.statisticsTabScroll = make(map[int]int)
	}
	g.statisticsTabScroll[g.statisticsTab] = g.statisticsScroll
	g.statisticsTab = tab
	g.statisticsScroll = g.statisticsTabScroll[tab]
}

func profileScrollInput(offset, limit, viewport int, wheel float64, pressed func(ebiten.Key) bool) int {
	delta := int(wheel * 48)
	if delta == 0 && wheel != 0 {
		if wheel > 0 {
			delta = 1
		} else {
			delta = -1
		}
	}
	offset -= delta
	if pressed(ebiten.KeyDown) {
		offset += 48
	}
	if pressed(ebiten.KeyUp) {
		offset -= 48
	}
	if pressed(ebiten.KeyPageDown) {
		offset += max(1, viewport-40)
	}
	if pressed(ebiten.KeyPageUp) {
		offset -= max(1, viewport-40)
	}
	if pressed(ebiten.KeyHome) {
		offset = 0
	}
	if pressed(ebiten.KeyEnd) {
		offset = limit
	}
	return max(0, min(offset, limit))
}

func (g *MMGame) updatePlayerStatisticsKeys(pressed func(ebiten.Key) bool) {
	spec := profilePages[max(0, min(g.statisticsTab, len(profilePages)-1))]
	l := makeProfileStatsLayout(g.config.GetScreenWidth(), g.config.GetScreenHeight(), spec)
	_, wheel := pointerWheel()
	mx, my := pointerPosition()
	if isMouseHoveringBox(mx, my, l.body.x, l.body.y, l.body.right(), l.body.bottom()) {
		for i, rs := range spec.rankings {
			r := profileRankingBody(l.rankingRect(i, g.statisticsScroll))
			if isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom()) {
				if g.statisticsRankingScroll == nil {
					g.statisticsRankingScroll = make(map[string]int)
				}
				before := g.statisticsRankingScroll[rs.group]
				g.statisticsRankingScroll[rs.group] = profileScrollInput(before, g.statisticsRankingLimits[rs.group], r.h, wheel, pressed)
				if before != g.statisticsRankingScroll[rs.group] {
					g.statisticsRevision++
					return
				}
				break
			}
		}
	} else {
		wheel = 0
	}
	g.statisticsScroll = profileScrollInput(g.statisticsScroll, max(0, l.contentH-l.body.h), l.body.h, wheel, pressed)
}

func (ui *UISystem) drawPlayerStatistics(screen *ebiten.Image, w, h int) {
	g := ui.game
	g.statisticsTab = max(0, min(g.statisticsTab, len(profilePages)-1))
	spec := profilePages[g.statisticsTab]
	l := makeProfileStatsLayout(w, h, spec)
	r := l.panel
	ui.drawThemeFrame(screen, frameGold, r.x, r.y, r.w, r.h)
	ui.drawCornerDecor(screen, frameGold, r.x-8, r.y-8, r.w+16, r.h+16, decorAllCorners)
	ui.drawPanelInlay(screen, frameGold, r.x+r.w/2, r.y)
	x, y, iw := r.x+menuFrameInset, r.y+menuFrameInset, r.w-2*menuFrameInset
	drawScaledMetalCenteredTextAlpha(screen, "PLAYER STATISTICS", r.x+r.w/2, y+10, 2, profileGold, 1)
	for i, page := range profilePages {
		label := page.title
		tab := profileTabRect(x, y, iw, i)
		ui.profileButton(screen, profileText(label, tab.w-20), tab, true, func() { g.setStatisticsTab(i) })
		if i == g.statisticsTab {
			drawFilledRect(screen, tab.x+10, tab.bottom()-4, tab.w-20, 2, profileGold)
		}
	}
	var d playerprofile.Data
	if g.playerProfile != nil {
		d = g.playerProfile.Data
	} else {
		d = playerprofile.New()
	}
	if spec.title == "Arena" {
		ui.ensureProfileArena()
		d = *ui.profileArena
	}
	if g.statisticsRankingLimits == nil {
		g.statisticsRankingLimits = make(map[string]int)
	}
	if g.statisticsRankingScroll == nil {
		g.statisticsRankingScroll = make(map[string]int)
	}
	maxScroll := max(0, l.contentH-l.body.h)
	g.statisticsScroll = max(0, min(g.statisticsScroll, maxScroll))
	ui.profileViewport = uiLayer(ui.profileViewport, l.body.w, l.body.h)
	dst := ui.profileViewport
	dst.Clear()
	offset := -g.statisticsScroll
	for i, c := range spec.counters {
		cx, cy := (i%l.counterColumns)*(l.counterW+14), offset+(i/l.counterColumns)*100
		ui.drawProfileCard(dst, layoutRect{cx, cy, l.counterW, 86}, true)
		iconSize := min(60, l.counterW/5)
		tx := cx + iconSize + 24
		tw := l.counterW - iconSize - 36
		ui.profileIcon(dst, c.icon, c.title, cx+12, cy+(86-iconSize)/2, iconSize)
		drawUITextColored(dst, profileText(c.title, tw), tx, cy+14, profileMuted)
		val := ui.profileCounterValue(c, &d)
		drawScaledMetalCenteredTextAlpha(dst, val, tx+tw/2, cy+52, 2, profileGold, 1)
	}
	for i, rs := range spec.rankings {
		rr := layoutRect{(i % l.columns) * (l.columnW + 14), offset + l.rankY + (i/l.columns)*(l.rankH+14), l.columnW, l.rankH}
		entries := ui.profileRankingEntries(rs, &d)
		limit := max(0, len(entries)*profileRankingRowH-profileRankingBody(rr).h)
		g.statisticsRankingLimits[rs.group] = limit
		g.statisticsRankingScroll[rs.group] = max(0, min(g.statisticsRankingScroll[rs.group], limit))
		ui.drawProfileRanking(dst, l.body, rs, entries, rr, g.statisticsRankingScroll[rs.group])
	}
	sy := offset + l.rankY + l.rankRows*(l.rankH+14) + 4
	summary := fmt.Sprintf("Bosses %s   Steps %s   Highest level %s   Party wipes %s", profileValue(d.Counters["bosses"], false), profileValue(d.Counters["steps"], false), profileValue(d.Counters["highest_level"], false), profileValue(d.Counters["defeats"], false))
	if spec.title == "Arena" {
		summary = "THE ARENA  /  Recorded victories across all adventures"
	}
	for _, line := range wrapArenaBoardLine(summary, iw-18) {
		drawUITextColored(dst, line, 0, sy, profileGold)
		sy += 15
	}
	note := "Lifetime activity since " + d.Since.Local().Format("02 Jan 2006") + ". Reloading does not erase records."
	if g.playerProfile == nil {
		note = "Profile unavailable. Historical activity cannot be reconstructed from saves."
	}
	detail := "Class time counts each active hero. Paused and loading time is excluded."
	if g.statisticsTab == 1 {
		detail = "Damage: net HP lost to direct attacks, including AoE. Ongoing poison/burn is excluded."
	}
	if g.statisticsTab == 2 {
		detail = "Loot: drops/chests/crates. Old finds had no price recorded; values start with new drops."
	}
	if g.statisticsTab == 3 {
		detail = "Quests count successful reward turn-ins. Older totals remain; quest breakdown and reward totals start with new turn-ins."
		detail += " Rewards: " + questRewardSummary(int(d.Counters["quest_gold"]), int(d.Counters["quest_arena_points"]), int(d.Counters["quest_xp"])) + "."
	}
	if g.statisticsTab == 4 {
		detail = "Cards are separate from legendary loot. Chest item counts exclude gold and roadside boxes. Chest detail starts with new finds."
	}
	if spec.title == "Arena" {
		note = "Records from the arena leaderboard, including earlier adventures."
		detail = "Only credited champion victories count. Shop spending and quest points are excluded."
		if ui.profileArenaError != "" {
			note = ui.profileArenaError
		}
	}
	for _, text := range []string{note, detail} {
		sy += 5
		for _, line := range wrapArenaBoardLine(text, iw-18) {
			drawUITextColored(dst, line, 0, sy, profileMuted)
			sy += 15
		}
	}
	drawImageScaled(screen, dst, l.body.x, l.body.y, l.body.w, l.body.h)
	ui.drawScrollbar(screen, "profile:statistics", layoutRect{l.body.right() - 6, l.body.y, 4, l.body.h}, g.statisticsScroll, l.contentH, true, func(v int) {
		g.statisticsScroll = v
	})
	bottom := l.footerY
	ui.drawBackButton(screen, x, bottom, func() { g.entryMenuMode = EntryMenuRoot })
	drawUITextColored(screen, profileText("Scroll over a list to browse it", iw-138), x+126, bottom+9, profileMuted)
	ui.drawProfileError(screen, x, y-16, iw)
}

// drawProfileRanking draws one ranking card into the statistics viewport,
// whose screen rect is view.
func (ui *UISystem) drawProfileRanking(screen *ebiten.Image, view layoutRect, spec profileRankingSpec, entries []playerprofile.Entry, r layoutRect, scroll int) {
	ui.drawProfileCard(screen, r, false)
	drawUITextColored(screen, profileText(strings.ToUpper(spec.title), r.w-28), r.x+14, r.y+12, profileGold)
	drawUITextColored(screen, profileText(spec.unit, r.w-28), r.x+14, r.y+29, profileMuted)
	body := profileRankingBody(r)
	clip := image.Rect(body.x, body.y, body.right(), body.bottom()).Intersect(uiBounds(screen))
	if !clip.Empty() {
		dst := uiClip(screen, clip)
		if len(entries) == 0 {
			drawUITextColored(dst, "No records yet", body.x+10, body.y+20, profileMuted)
		}
		var maximum int64 = 1
		if len(entries) > 0 {
			maximum = max(maximum, spec.score(entries[0]))
		}
		if spec.group == "exploration" {
			maximum = 1000
		}
		for i, e := range entries {
			if spec.group == "classes" {
				e.Icon = ui.game.largePortraitSpriteName(e.Icon)
			}
			cy := body.y + i*profileRankingRowH - scroll
			if cy+profileRankingRowH <= clip.Min.Y || cy >= clip.Max.Y {
				continue
			}
			if i%2 == 0 {
				drawFilledRect(dst, body.x, cy, body.w-8, profileRankingRowH-2, color.RGBA{35, 30, 39, 130})
			}
			ui.profileIcon(dst, e.Icon, e.Name, body.x+5, cy+7, 44)
			tx, tw := body.x+58, body.w-76
			name := fmt.Sprintf("%d. %s", i+1, e.Name)
			drawUITextColored(dst, profileText(name, tw), tx, cy+8, profileGold)
			value := spec.entryValue(e)
			if spec.group == "valuable_loot" {
				value += "  /  Found " + profileValue(e.Count, false)
			}
			drawUITextColored(dst, profileText(value, tw), tx, cy+27, profileGreen)
			drawFilledRect(dst, tx, cy+47, tw, 3, color.RGBA{53, 44, 52, 255})
			drawFilledRect(dst, tx, cy+47, max(1, int(float64(spec.score(e))/float64(maximum)*float64(tw))), 3, color.RGBA{151, 120, 66, 255})
		}
		track := layoutRect{body.right() - 4, body.y, 3, body.h}
		drawScrollbarThumb(dst, track, scroll, len(entries)*profileRankingRowH)
		// The card lives in the viewport layer; its thumb is grabbed on screen,
		// only where the viewport shows it.
		onScreen := layoutRect{view.x + track.x, view.y + track.y, track.w, track.h}
		strip := scrollbarGrabStrip(onScreen)
		seen := image.Rect(strip.x, strip.y, strip.right(), strip.bottom()).Intersect(image.Rect(view.x, view.y, view.right(), view.bottom()))
		hit := layoutRect{seen.Min.X, seen.Min.Y, seen.Dx(), seen.Dy()}
		g := ui.game
		ui.scrollbarGesture("profile:ranking:"+spec.group, onScreen, hit, scroll, len(entries)*profileRankingRowH, func(v int) {
			g.statisticsRankingScroll[spec.group] = v
			g.statisticsRevision++
		})
	}
	drawUITextColored(screen, fmt.Sprintf("%d recorded", len(entries)), r.x+14, r.bottom()-21, profileMuted)
}

func (ui *UISystem) drawProfileError(screen *ebiten.Image, x, y, w int) {
	g := ui.game
	message := g.playerProfileError
	if g.playerProfile != nil && g.playerProfile.Err() != nil {
		message = "Profile save failed; will retry."
	}
	if message != "" {
		drawUITextColored(screen, profileText(message, w), x, y, color.RGBA{233, 128, 113, 255})
	}
}

func (spec profileRankingSpec) value(n int64) string {
	if spec.group == "exploration" {
		return fmt.Sprintf("%.1f%%", float64(n)/10)
	}
	return profileValue(n, spec.duration)
}

func (spec profileRankingSpec) score(e playerprofile.Entry) int64 {
	if spec.group == "valuable_loot" {
		return e.BaseValue
	}
	return e.Count
}

func (spec profileRankingSpec) entryValue(e playerprofile.Entry) string {
	if spec.group == "valuable_loot" {
		return profileValue(e.BaseValue, false) + "g"
	}
	return spec.value(e.Count)
}

func (ui *UISystem) profileRankingEntries(spec profileRankingSpec, d *playerprofile.Data) []playerprofile.Entry {
	if spec.group == "valuable_loot" {
		return d.MostValuableLoot()
	}
	if spec.group != "exploration" {
		return d.Top(spec.group)
	}
	ui.ensureProfileExploration()
	return ui.profileExploration.entries
}

func (ui *UISystem) ensureProfileExploration() {
	if !ui.profileExplorationReady {
		ui.profileExploration = ui.game.profileExplorationStats()
		ui.profileExplorationReady = true
	}
}

func (ui *UISystem) profileCounterValue(spec profileCounterSpec, d *playerprofile.Data) string {
	if spec.key == "exploration" {
		ui.ensureProfileExploration()
		return (profileRankingSpec{group: "exploration"}).value(ui.profileExploration.percentTenths())
	}
	return profileValue(d.Counters[spec.key], spec.duration)
}
