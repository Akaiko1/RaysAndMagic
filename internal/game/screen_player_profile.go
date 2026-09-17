package game

import (
	"fmt"
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
	return layoutRect{(w - pw) / 2, (h - ph) / 2, pw, ph}
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
	if y+size < 0 || y >= screen.Bounds().Dy() {
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
		drawCenteredDebugText(screen, spellInitials(label), x, y, size, size)
	}
	if !strings.HasPrefix(key, "icon_") {
		drawRectBorder(screen, x, y, size, size, 1, profileGold)
	}
}

func profileText(s string, width int) string {
	return truncateName(s, max(1, width/debugTextCharWidth))
}

func (ui *UISystem) drawAchievementsScreen(screen *ebiten.Image, w, h int) {
	g := ui.game
	r := profilePanelRect(w, h)
	ui.drawPanel(screen, "menu_panel_wide", r.x, r.y, r.w, r.h)
	ui.drawPanelInlay(screen, frameGold, r.x+r.w/2, r.y)
	ui.drawCornerDecor(screen, frameGold, r.x-8, r.y-8, r.w+16, r.h+16, decorAllCorners)
	x, y, innerW := r.x+menuFrameInset, r.y+menuFrameInset, r.w-2*menuFrameInset
	defs := config.GetAchievements()
	unlocked := 0
	for _, def := range defs {
		if g.playerProfile != nil {
			if _, ok := g.playerProfile.Data.Unlocked[def.Key]; ok {
				unlocked++
			}
		}
	}
	drawScaledMetalCenteredTextAlpha(screen, "ACHIEVEMENTS", r.x+r.w/2, y+10, 2, profileGold, 1)
	drawDebugTextColored(screen, fmt.Sprintf("%d / %d earned across all adventures", unlocked, len(defs)), x, y+36, profileMuted)
	bottom := r.y + r.h - menuFrameInset - menuBackButtonH
	cols := 2
	if innerW < 820 {
		cols = 1
	}
	rowH := 100
	rows := max(1, (bottom-y-66)/rowH)
	totalRows := (len(defs) + cols - 1) / cols
	maxScroll := max(0, totalRows-rows)
	g.achievementsScroll = max(0, min(g.achievementsScroll, maxScroll))
	cw := (innerW - (cols-1)*16) / cols
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			idx := (g.achievementsScroll+row)*cols + col
			if idx >= len(defs) {
				break
			}
			def := defs[idx]
			cx, cy := x+col*(cw+16), y+60+row*rowH
			stamp := time.Time{}
			progress := int64(0)
			if g.playerProfile != nil {
				stamp = g.playerProfile.Data.Unlocked[def.Key]
				progress = g.playerProfile.Data.AchievementProgress(def.AnyOf)
			}
			unlocked := !stamp.IsZero()
			ui.drawProfileCard(screen, layoutRect{cx, cy, cw, rowH - 12}, unlocked)
			ui.profileIcon(screen, def.Icon, def.Name, cx+8, cy+10, 64)
			if !unlocked {
				drawFilledRect(screen, cx+8, cy+10, 64, 64, color.RGBA{0, 0, 0, 130})
			}
			tx, tw := cx+84, cw-96
			drawDebugTextColored(screen, profileText(def.Name, tw), tx, cy+9, profileGold)
			lines := wrapArenaBoardLine(def.Description, tw)
			for i, line := range lines {
				if i >= 2 {
					break
				}
				drawDebugTextColored(screen, line, tx, cy+28+i*14, profileMuted)
			}
			status := fmt.Sprintf("Locked  %d/%d", min(progress, def.Target), def.Target)
			clr := profileMuted
			if unlocked {
				status = "Earned " + stamp.Local().Format("02 Jan 2006")
				clr = profileGreen
			}
			drawDebugTextColored(screen, status, tx, cy+62, clr)
		}
	}
	ui.drawBackButton(screen, x, bottom, func() { g.entryMenuMode = EntryMenuRoot })
	ui.profileButton(screen, "< Prev", layoutRect{x + innerW - 190, bottom, 86, 30}, g.achievementsScroll > 0, func() { g.achievementsScroll = max(0, g.achievementsScroll-rows) })
	ui.profileButton(screen, "Next >", layoutRect{x + innerW - 94, bottom, 94, 30}, g.achievementsScroll < maxScroll, func() { g.achievementsScroll = min(maxScroll, g.achievementsScroll+rows) })
	ui.drawProfileError(screen, x, bottom-18, innerW)
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
	{"Trophies", []profileCounterSpec{{"Bosses defeated", "bosses", "icon_achievement_warlord", false}, {"Items traded", "items_traded", "icon_item_clock_hand", false}, {"Quests completed", "quest_rewards", "icon_achievement_archmage", false}}, []profileRankingSpec{{"Bosses defeated", "bosses", "kills by boss", false}, {"Trade offerings", "items_traded", "item units paid to merchants", false}, {"Quests completed", "quest_rewards", "successful turn-ins by quest", false}}},
	{"Collecting", []profileCounterSpec{{"Cards found", "cards_found", "icon_item_goblin_card", false}, {"Chest loot", "chest_loot", "chest_golden", false}, {"Legendary drops", "legendary_loot", "icon_weapon_wyrmcleaver", false}}, []profileRankingSpec{{"Cards found", "cards_found", "units found, all rarities", false}, {"Loot from chests", "chest_loot", "item units, excludes gold", false}, {"Legendary loot", "legendary_loot", "units found, excludes cards", false}}},
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

const profileRanksPerPage = 5

type profileStatsLayout struct {
	panel, body                                                     layoutRect
	columns, columnW, counterRows, rankRows, rankY, rankH, contentH int
	footerY                                                         int
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
	l.counterRows = (len(page.counters) + columns - 1) / columns
	l.rankRows = (len(page.rankings) + columns - 1) / columns
	l.rankY = l.counterRows*100 + 8
	l.rankH = 144 + (profileRanksPerPage-1)*50 + 24
	l.contentH = l.rankY + l.rankRows*(l.rankH+14) + 106
	return l
}

func (g *MMGame) updatePlayerStatisticsKeys(pressed func(ebiten.Key) bool) {
	l := makeProfileStatsLayout(g.config.GetScreenWidth(), g.config.GetScreenHeight(), profilePages[max(0, min(g.statisticsTab, len(profilePages)-1))])
	_, wheel := ebiten.Wheel()
	mx, my := pointerPosition()
	if isMouseHoveringBox(mx, my, l.body.x, l.body.y, l.body.x+l.body.w, l.body.y+l.body.h) {
		g.statisticsScroll -= int(wheel * 48)
	}
	if pressed(ebiten.KeyDown) {
		g.statisticsScroll += 48
	}
	if pressed(ebiten.KeyUp) {
		g.statisticsScroll -= 48
	}
	if pressed(ebiten.KeyPageDown) {
		g.statisticsScroll += l.body.h - 40
	}
	if pressed(ebiten.KeyPageUp) {
		g.statisticsScroll -= l.body.h - 40
	}
	if pressed(ebiten.KeyHome) {
		g.statisticsScroll = 0
	}
	if pressed(ebiten.KeyEnd) {
		g.statisticsScroll = l.contentH
	}
	g.statisticsScroll = max(0, min(g.statisticsScroll, max(0, l.contentH-l.body.h)))
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
		if i == g.statisticsTab {
			label = "[ " + label + " ]"
		}
		ui.profileButton(screen, label, profileTabRect(x, y, iw, i), true, func() { g.statisticsTab = i; g.statisticsPage = 0; g.statisticsScroll = 0 })
	}
	var d playerprofile.Data
	if g.playerProfile != nil {
		d = g.playerProfile.Data
	} else {
		d = playerprofile.New()
	}
	maxPages := 1
	for _, rs := range spec.rankings {
		maxPages = max(maxPages, (len(ui.profileRankingEntries(rs, &d))+profileRanksPerPage-1)/profileRanksPerPage)
	}
	g.statisticsPage = max(0, min(g.statisticsPage, maxPages-1))
	maxScroll := max(0, l.contentH-l.body.h)
	g.statisticsScroll = max(0, min(g.statisticsScroll, maxScroll))
	if ui.profileViewport == nil || ui.profileViewport.Bounds().Dx() != l.body.w || ui.profileViewport.Bounds().Dy() != l.body.h {
		if ui.profileViewport != nil {
			ui.profileViewport.Deallocate()
		}
		ui.profileViewport = ebiten.NewImage(l.body.w, l.body.h)
	}
	dst := ui.profileViewport
	dst.Clear()
	offset := -g.statisticsScroll
	for i, c := range spec.counters {
		cx, cy := (i%l.columns)*(l.columnW+14), offset+(i/l.columns)*100
		ui.drawProfileCard(dst, layoutRect{cx, cy, l.columnW, 86}, true)
		ui.profileIcon(dst, c.icon, c.title, cx+12, cy+13, 60)
		drawDebugTextColored(dst, profileText(c.title, l.columnW-92), cx+84, cy+14, profileMuted)
		val := ui.profileCounterValue(c, &d)
		drawScaledMetalCenteredTextAlpha(dst, val, cx+84+(l.columnW-92)/2, cy+52, 2, profileGold, 1)
	}
	for i, rs := range spec.rankings {
		rr := layoutRect{(i % l.columns) * (l.columnW + 14), offset + l.rankY + (i/l.columns)*(l.rankH+14), l.columnW, l.rankH}
		ui.drawProfileRanking(dst, rs, ui.profileRankingEntries(rs, &d), rr, g.statisticsPage*profileRanksPerPage, profileRanksPerPage)
	}
	sy := offset + l.rankY + l.rankRows*(l.rankH+14) + 4
	summary := fmt.Sprintf("Bosses %s   Steps %s   Highest level %s   Party wipes %s", profileValue(d.Counters["bosses"], false), profileValue(d.Counters["steps"], false), profileValue(d.Counters["highest_level"], false), profileValue(d.Counters["defeats"], false))
	for _, line := range wrapArenaBoardLine(summary, iw-18) {
		drawDebugTextColored(dst, line, 0, sy, profileGold)
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
	for _, text := range []string{note, detail} {
		sy += 5
		for _, line := range wrapArenaBoardLine(text, iw-18) {
			drawDebugTextColored(dst, line, 0, sy, profileMuted)
			sy += 15
		}
	}
	drawImageScaled(screen, dst, l.body.x, l.body.y, l.body.w, l.body.h)
	if maxScroll > 0 {
		trackX := l.body.x + l.body.w - 6
		drawFilledRect(screen, trackX, l.body.y, 4, l.body.h, color.RGBA{56, 46, 44, 255})
		thumbH := max(24, l.body.h*l.body.h/l.contentH)
		thumbY := l.body.y + (l.body.h-thumbH)*g.statisticsScroll/maxScroll
		drawFilledRect(screen, trackX, thumbY, 4, thumbH, profileGold)
	}
	bottom := l.footerY
	ui.drawBackButton(screen, x, bottom, func() { g.entryMenuMode = EntryMenuRoot })
	if maxScroll > 0 {
		drawDebugTextColored(screen, "Scroll / PgUp / PgDn", x+126, bottom+9, profileMuted)
	}
	if iw >= 690 {
		drawDebugTextColored(screen, fmt.Sprintf("Page %d/%d", g.statisticsPage+1, maxPages), x+iw-280, bottom+9, profileMuted)
	}
	changePage := func(delta int) {
		g.statisticsPage = (g.statisticsPage + delta + maxPages) % maxPages
		g.statisticsScroll = 0
	}
	ui.profileButton(screen, "< Prev", layoutRect{x + iw - 184, bottom, 86, 30}, maxPages > 1, func() { changePage(-1) })
	ui.profileButton(screen, "Next >", layoutRect{x + iw - 90, bottom, 90, 30}, maxPages > 1, func() { changePage(1) })
	ui.drawProfileError(screen, x, y-16, iw)
}

func (ui *UISystem) drawProfileRanking(screen *ebiten.Image, spec profileRankingSpec, entries []playerprofile.Entry, r layoutRect, start, limit int) {
	ui.drawProfileCard(screen, r, true)
	drawDebugTextColored(screen, profileText(spec.title, r.w-24), r.x+12, r.y+12, profileGold)
	drawDebugTextColored(screen, spec.unit, r.x+12, r.y+30, profileMuted)
	if start >= len(entries) {
		drawDebugTextColored(screen, "No records yet.", r.x+16, r.y+90, profileMuted)
		return
	}
	maxValue := max(int64(1), spec.score(entries[0]))
	if spec.group == "exploration" {
		maxValue = 1000
	}
	total := int64(0)
	for _, e := range entries {
		total += e.Count
	}
	for row := 0; row < limit && start+row < len(entries); row++ {
		e := entries[start+row]
		if spec.group == "classes" {
			e.Icon = ui.game.largePortraitSpriteName(e.Icon)
		}
		idx := start + row
		if row == 0 {
			size := min(76, r.w/3)
			ix, iy := r.x+14, r.y+53
			ui.profileIcon(screen, e.Icon, e.Name, ix, iy, size)
			tx, tw := ix+size+12, r.w-size-40
			drawDebugTextColored(screen, fmt.Sprintf("#%d", idx+1), tx, iy, profileMuted)
			for i, line := range wrapArenaBoardLine(e.Name, tw) {
				if i >= 2 {
					break
				}
				drawDebugTextColored(screen, line, tx, iy+18+i*14, profileGold)
			}
			drawDebugTextColored(screen, spec.entryValue(e), tx, iy+size-15, profileGreen)
			if spec.group == "valuable_loot" {
				drawDebugTextColored(screen, "Found: "+profileValue(e.Count, false), tx, iy+size+2, profileMuted)
			}
			continue
		}
		cy := r.y + 144 + (row-1)*50
		ui.profileIcon(screen, e.Icon, e.Name, r.x+12, cy, 38)
		tx, tw := r.x+60, r.w-72
		val := spec.entryValue(e)
		drawDebugTextColored(screen, profileText(e.Name, tw-debugTextWidth(val)-12), tx, cy+1, profileMuted)
		drawDebugTextColored(screen, val, r.x+r.w-12-debugTextWidth(val), cy+1, profileGold)
		if spec.group == "valuable_loot" {
			drawDebugTextColored(screen, "Found: "+profileValue(e.Count, false), tx, cy+14, profileMuted)
		}
		bw := int(float64(spec.score(e)) / float64(maxValue) * float64(tw))
		barY := cy + 23
		if spec.group == "valuable_loot" {
			barY = cy + 32
		}
		drawFilledRect(screen, tx, barY, tw, 5, color.RGBA{43, 34, 45, 255})
		drawFilledRect(screen, tx, barY, bw, 5, color.RGBA{128, 104, 62, 255})
	}
	if total > 0 || spec.group == "exploration" {
		drawDebugTextColored(screen, fmt.Sprintf("%d recorded", len(entries)), r.x+16, r.y+r.h-24, profileMuted)
	}
}

func (ui *UISystem) drawProfileError(screen *ebiten.Image, x, y, w int) {
	g := ui.game
	message := g.playerProfileError
	if g.playerProfile != nil && g.playerProfile.Err() != nil {
		message = "Profile save failed; will retry."
	}
	if message != "" {
		drawDebugTextColored(screen, profileText(message, w), x, y, color.RGBA{233, 128, 113, 255})
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
