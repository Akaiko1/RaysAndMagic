package main

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"image/color"
	"slices"
	"strconv"
	"strings"

	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/monster"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// The Actionables page is the rules sheet: every status from
// assets/statuses.yaml with what it does to the party and to monsters, and the
// auto-level of the adventure maps from config.yaml adventure_scaling.
const pageActionables = 10

const (
	actionablesViewStatuses = iota
	actionablesViewAutoLevel
)

const (
	actionablesLabelW     = 84 // PARTY / MONSTERS / RULE column
	actionablesLineH      = 18
	actionablesNameH      = 26
	actionablesGroupH     = 30
	actionablesCardGap    = 10
	actionablesTabW       = 112
	actionablesLevelStep  = 5
	actionablesLevelMax   = 100
	actionablesLedgerW    = 440
	actionablesLedgerRowH = 26
	actionablesLedgerTop  = 156 // first ledger row below the panel top
	actionablesChamberRow = 18
	actionablesCardMinW   = 340
)

var actionablesPage = struct {
	view          int
	scroll        int
	dungeonScroll int
	level         int // selected visit level of the auto-level view
	rows          []actionRow
	rowsWidth     int
	rowsHeight    int
}{level: 35}

// actionablesGeometry places the page on the current canvas, below the title
// and down to the help line. Update and draw share it.
type actionablesGeometry struct {
	body, ledger, maps rect
	tabs               [2]rect
}

func (v *viewer) actionablesGeometry() actionablesGeometry {
	w, h := v.canvasSize()
	top, bottom := pageBarHeight+48, h-34
	g := actionablesGeometry{body: rect{8, top, w - 16, bottom - top}}
	g.ledger = rect{8, top, actionablesLedgerW, bottom - top}
	g.maps = rect{g.ledger.x + g.ledger.w + 8, top, w - 8 - (g.ledger.x + g.ledger.w + 8), bottom - top}
	tabX := 16 + game.ShadedTextWidth("ACTIONABLES") + 24
	for i := range g.tabs {
		g.tabs[i] = rect{tabX + i*(actionablesTabW+4), pageBarHeight + 6, actionablesTabW, 22}
	}
	return g
}

type actionRowKind int

const (
	actionGroup actionRowKind = iota
	actionName
	actionLine
)

// actionRow is one laid-out row of the status list, at its pixel offset.
type actionRow struct {
	y, h        int
	kind        actionRowKind
	label, text string
	note        string // the card's short name, beside a status name
}

// actionableRows lays the status list out for a text width, once per width.
func actionableRows(width int) ([]actionRow, int) {
	if actionablesPage.rowsWidth == width && actionablesPage.rows != nil {
		return actionablesPage.rows, actionablesPage.rowsHeight
	}
	var rows []actionRow
	y := 0
	add := func(r actionRow) {
		r.y = y
		rows = append(rows, r)
		y += r.h
	}
	groups := []struct {
		title          string
		party, monster bool
	}{{"BOTH SIDES", true, true}, {"MONSTERS ONLY", false, true}, {"PARTY ONLY", true, false}}
	for _, group := range groups {
		add(actionRow{kind: actionGroup, h: actionablesGroupH, text: group.title})
		for _, key := range config.StatusKeys() {
			s := config.Status(key)
			if (s.Party != "") != group.party || (s.Monsters != "") != group.monster {
				continue
			}
			name := actionRow{kind: actionName, h: actionablesNameH, text: s.Name}
			if s.Short != "" && s.Short != s.Name {
				name.note = "card: " + s.Short
			}
			add(name)
			for _, side := range []struct{ label, text string }{
				{"PARTY", s.PartyText()}, {"MONSTERS", s.MonsterText()}, {"RULE", s.RuleText()},
			} {
				if side.text == "" {
					continue
				}
				for i, line := range wrapCatalogInfo([]infoLine{{text: side.text}}, width-actionablesLabelW) {
					row := actionRow{kind: actionLine, h: actionablesLineH, text: line.text}
					if i == 0 {
						row.label = side.label
					}
					add(row)
				}
			}
			y += actionablesCardGap
		}
	}
	actionablesPage.rows, actionablesPage.rowsWidth, actionablesPage.rowsHeight = rows, width, y
	return rows, y
}

// statusListView is the scrolled viewport inside the status panel.
func (g actionablesGeometry) statusListView() rect {
	return rect{g.body.x + 12, g.body.y + 8, g.body.w - 24, g.body.h - 16}
}

// ledgerRow is the ledger row of a visit level; ok is false for a row below
// the panel, which is neither drawn nor clickable.
func (g actionablesGeometry) ledgerRow(i int) (rect, bool) {
	r := rect{g.ledger.x + 6, g.ledger.y + actionablesLedgerTop + i*actionablesLedgerRowH, g.ledger.w - 12, actionablesLedgerRowH}
	return r, r.y+r.h <= g.ledger.y+g.ledger.h
}

func (v *viewer) updateActionablesPage() {
	g := v.actionablesGeometry()
	in := readCatalogListInput()
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyTab):
		actionablesPage.view = 1 - actionablesPage.view
	case inpututil.IsKeyJustPressed(ebiten.Key1):
		actionablesPage.view = actionablesViewStatuses
	case inpututil.IsKeyJustPressed(ebiten.Key2):
		actionablesPage.view = actionablesViewAutoLevel
	}
	for i, tab := range g.tabs {
		if in.click && pointInRect(in.x, in.y, tab.x, tab.y, tab.w, tab.h) {
			actionablesPage.view = i
		}
	}
	if actionablesPage.view == actionablesViewAutoLevel {
		level := actionablesPage.level + in.delta*actionablesLevelStep
		for i, l := 0, actionablesLevelStep; l <= actionablesLevelMax; i, l = i+1, l+actionablesLevelStep {
			if r, ok := g.ledgerRow(i); ok && in.click && pointInRect(in.x, in.y, r.x, r.y, r.w, r.h) {
				level = l
			}
		}
		actionablesPage.level = clampInt(level, actionablesLevelStep, actionablesLevelMax)
		view := g.dungeonView()
		_, _, total := v.dungeonCards(view.w)
		scroll := actionablesPage.dungeonScroll
		if in.wheel != 0 && pointInRect(in.x, in.y, view.x, view.y, view.w, view.h) {
			scroll -= int(in.wheel * 44)
		}
		if dragged, owned := v.dragCatalogScroll("autolevel", view, scroll, total, in); owned {
			scroll = dragged
		}
		actionablesPage.dungeonScroll = clampInt(scroll, 0, max(0, total-view.h))
		return
	}
	view := g.statusListView()
	_, total := actionableRows(view.w - 12)
	scroll := actionablesPage.scroll
	if in.wheel != 0 && pointInRect(in.x, in.y, view.x, view.y, view.w, view.h) {
		scroll -= int(in.wheel * 44)
	}
	scroll += in.delta * actionablesLineH
	if dragged, owned := v.dragCatalogScroll("actionables", view, scroll, total, in); owned {
		scroll = dragged
	}
	actionablesPage.scroll = clampInt(scroll, 0, max(0, total-view.h))
}

func (v *viewer) drawActionablesPage(screen *ebiten.Image) {
	g := v.actionablesGeometry()
	w, h := v.canvasSize()
	game.DrawShadedText(screen, "ACTIONABLES", 16, pageBarHeight+12, overlayAccent)
	for i, label := range [2]string{"1  STATUSES", "2  AUTO-LEVEL"} {
		t := g.tabs[i]
		fill, text := overlayInput, overlayMuted
		if i == actionablesPage.view {
			fill, text = overlayButton, overlayText
			drawFilledRect(screen, t.x, t.y+t.h-2, t.w, 2, overlayAccent)
		}
		drawFilledRect(screen, t.x, t.y, t.w, t.h, fill)
		drawRectBorder(screen, t.x, t.y, t.w, t.h, 1, overlayBorder)
		game.DrawShadedText(screen, label, t.x+10, t.y+6, text)
	}
	if actionablesPage.view == actionablesViewAutoLevel {
		game.DrawShadedText(screen, clipText("Every map an adventure schedule opens moves its monsters from their own level to the visit level. Tune config.yaml adventure_scaling; hazard percents live in map_configs.yaml.", w-32), 16, pageBarHeight+34, overlayMuted)
		v.drawAutoLevelLedger(screen, g)
		v.drawDungeons(screen, g)
		game.DrawShadedText(screen, clipText("Click a row or Up / Down: every map at that visit level   |   Wheel: scroll the maps   |   Tab, 1, 2: switch view", w-32), 16, h-20, overlayMuted)
		return
	}
	game.DrawShadedText(screen, clipText("Statuses come from assets/statuses.yaml. Each source authors its duration in seconds of whole 3 s rounds: the same span in RT and TB.", w-32), 16, pageBarHeight+34, overlayMuted)
	v.drawActionableStatuses(screen, g)
	game.DrawShadedText(screen, clipText("Wheel or arrow keys: scroll   |   Durations per monster: Mobs > Abilities   |   Tab, 1, 2: switch view", w-32), 16, h-20, overlayMuted)
}

func (v *viewer) drawActionableStatuses(screen *ebiten.Image, g actionablesGeometry) {
	drawFilledRect(screen, g.body.x, g.body.y, g.body.w, g.body.h, overlayBG)
	drawRectBorder(screen, g.body.x, g.body.y, g.body.w, g.body.h, 1, overlayBorder)
	view := g.statusListView()
	rows, total := actionableRows(view.w - 12)
	clip := catalogClip(screen, view)
	for _, row := range rows {
		y := view.y + row.y - actionablesPage.scroll
		if y+row.h < view.y || y > view.y+view.h {
			continue
		}
		switch row.kind {
		case actionGroup:
			drawHeaderBandRect(clip, view.x, y+6, view.w-12, row.h-6)
			game.DrawShadedText(clip, row.text, view.x+10, y+11, viewerHeaderTextColor)
		case actionName:
			drawFilledRect(clip, view.x, y+row.h-3, view.w-12, 1, overlayBorder)
			game.DrawShadedText(clip, row.text, view.x, y+5, overlayAccent)
			if row.note != "" {
				game.DrawShadedText(clip, row.note, view.x+game.ShadedTextWidth(row.text)+16, y+5, overlayMuted)
			}
		case actionLine:
			game.DrawShadedText(clip, row.label, view.x, y+1, actionSideColor(row.label))
			game.DrawShadedText(clip, row.text, view.x+actionablesLabelW, y+1, overlayText)
		}
	}
	drawCatalogScroll(screen, view, actionablesPage.scroll, total)
}

// actionSideColor tells the sides apart: bosses' violet marks the monster side
// as it does in the Overlay, the accent marks the party.
func actionSideColor(label string) color.RGBA {
	switch label {
	case "PARTY":
		return overlayAccent
	case "MONSTERS":
		return overlayBossColor
	}
	return overlayMuted
}

// sideColor is a regular monster's or a boss's color on the auto-level view.
func sideColor(boss bool) color.RGBA {
	if boss {
		return overlayBossColor
	}
	return overlayText
}

// drawRightText ends text at x: numbers line up in their columns.
func drawRightText(dst *ebiten.Image, text string, x, y int, c color.RGBA) {
	game.DrawShadedText(dst, text, x-game.ShadedTextWidth(text), y, c)
}

// drawAutoLevelLedger is what a monster authored on its curve has at each
// visit level: HP, average hit and experience, regular beside boss. Each
// metric's bars share one scale, so a boss falling behind shows at a glance.
func (v *viewer) drawAutoLevelLedger(screen *ebiten.Image, g actionablesGeometry) {
	s, r := v.cfg.AdventureScaling, g.ledger
	drawFilledRect(screen, r.x, r.y, r.w, r.h, overlayBG)
	drawRectBorder(screen, r.x, r.y, r.w, r.h, 1, overlayBorder)
	x := r.x + 12
	game.DrawShadedText(screen, "CURVES", x, r.y+10, overlayAccent)
	flat := func(name string, reg, boss config.LevelCurve) string {
		return fmt.Sprintf("%s flat past L%d +%g/L, boss past L%d +%g/L.", name, reg.FlatFrom(), reg.FlatPerLevel, boss.FlatFrom(), boss.FlatPerLevel)
	}
	for i, line := range []string{
		fmt.Sprintf("Visit level = party average + %d; HP and hit x curve(L) / curve(own).", s.VisitLevelBonus),
		flat("HP", s.Regular.HP, s.Boss.HP),
		flat("Hit", s.Regular.Hit, s.Boss.Hit) + fmt.Sprintf(" AC x (L / own)^%g.", s.ArmorExponent),
		fmt.Sprintf("XP = %d%% / %d%% (boss) of a party-level hero's next level.", s.Regular.ExperienceLevelPercent, s.Boss.ExperienceLevelPercent),
		"Rows: a monster on its curve; HIT = average melee hit.",
	} {
		game.DrawShadedText(screen, clipText(line, r.w-24), x, r.y+30+i*16, overlayMuted)
	}
	head := r.y + actionablesLedgerTop - 46
	drawHeaderBandRect(screen, r.x+6, head, r.w-12, 22)
	game.DrawShadedText(screen, "LEVEL", x, head+5, viewerHeaderTextColor)
	pairW := (r.w - 12 - 52) / 3
	cell := func(metric, side int) int { return r.x + 6 + 52 + metric*pairW + side*pairW/2 }
	for m, label := range [3]string{"HP", "HIT", "XP"} {
		game.DrawShadedText(screen, label, cell(m, 0)+(pairW-game.ShadedTextWidth(label))/2, head+5, viewerHeaderTextColor)
		for side, sub := range [2]string{"reg", "boss"} {
			drawRightText(screen, sub, cell(m, side)+pairW/2-8, head+28, sideColor(side == 1))
		}
	}
	var top [3]int
	for level := actionablesLevelStep; level <= actionablesLevelMax; level += actionablesLevelStep {
		for _, boss := range [2]bool{false, true} {
			hp, hit, xp := game.AdventureCurve(s, boss, level)
			top = [3]int{max(top[0], hp), max(top[1], hit), max(top[2], xp)}
		}
	}
	mx, my := ebiten.CursorPosition()
	for i, level := 0, actionablesLevelStep; level <= actionablesLevelMax; i, level = i+1, level+actionablesLevelStep {
		row, ok := g.ledgerRow(i)
		if !ok {
			break
		}
		selected := level == actionablesPage.level
		switch {
		case selected:
			drawFilledRect(screen, row.x, row.y, row.w, row.h, overlayButton)
			drawFilledRect(screen, row.x, row.y, 3, row.h, overlayAccent)
		case pointInRect(mx, my, row.x, row.y, row.w, row.h):
			drawFilledRect(screen, row.x, row.y, row.w, row.h, overlayInput)
		}
		levelColor := overlayMuted
		if selected {
			levelColor = overlayAccent
		}
		game.DrawShadedText(screen, "L"+strconv.Itoa(level), x, row.y+5, levelColor)
		for side, boss := range [2]bool{false, true} {
			hp, hit, xp := game.AdventureCurve(s, boss, level)
			for m, value := range [3]int{hp, hit, xp} {
				left, right := cell(m, side)+6, cell(m, side)+pairW/2-8
				drawRightText(screen, strconv.Itoa(value), right, row.y+3, sideColor(boss))
				drawFilledRect(screen, left, row.y+20, max(1, (right-left)*value/max(1, top[m])), 2, sideColor(boss))
			}
		}
	}
}

// dungeonMonster is one monster kind of an auto-levelled map with its count.
type dungeonMonster struct {
	key   string
	def   monster.MonsterDefinition
	count int
}

// autoLevelDungeon is a map an adventure schedule opens, laid out as a card.
type autoLevelDungeon struct {
	m        mapInfo
	monsters []dungeonMonster
	hazards  int
	x, y, h  int // card offset in the scrolled column and its height
}

var autoLevelDungeons struct {
	key      uint64 // every map's spawns and the browser revision
	width    int
	cards    []autoLevelDungeon
	cardW    int
	contentH int
}

// dungeonCardsKey changes whenever a painted, moved or replaced monster or a
// published Overlay edit could change a card.
func (v *viewer) dungeonCardsKey() uint64 {
	h := fnv.New64a()
	var buf [8]byte
	put := func(n int) {
		binary.LittleEndian.PutUint64(buf[:], uint64(n))
		h.Write(buf[:])
	}
	put(v.browser.revision)
	for _, m := range v.maps {
		h.Write([]byte(m.Key))
		if m.Data == nil {
			continue
		}
		for _, sp := range m.Data.MonsterSpawns {
			h.Write([]byte(sp.MonsterKey))
			put(sp.X)
			put(sp.Y)
		}
	}
	return h.Sum64()
}

// dungeonCards are every map an adventure schedule opens, bosses first, laid
// out in columns by height; rebuilt when the maps or the width change.
func (v *viewer) dungeonCards(width int) ([]autoLevelDungeon, int, int) {
	key := v.dungeonCardsKey()
	if autoLevelDungeons.key == key && autoLevelDungeons.width == width && autoLevelDungeons.cards != nil {
		return autoLevelDungeons.cards, autoLevelDungeons.cardW, autoLevelDungeons.contentH
	}
	var cards []autoLevelDungeon
	for _, source := range v.maps {
		if source.Config == nil || source.Config.Adventure == nil || source.Config.Adventure.Schedule == nil {
			continue
		}
		for _, dest := range source.Config.Adventure.Schedule.Destinations() {
			i := slices.IndexFunc(v.maps, func(m mapInfo) bool { return m.Key == dest })
			if i < 0 || v.maps[i].Data == nil || slices.ContainsFunc(cards, func(c autoLevelDungeon) bool { return c.m.Key == dest }) {
				continue
			}
			d := autoLevelDungeon{m: v.maps[i], monsters: dungeonMonsters(v.maps[i])}
			if a := d.m.Config.Adventure; a != nil {
				for _, e := range a.Effects {
					if e.Damaging() {
						d.hazards++
					}
				}
			}
			d.h = 52 + len(d.monsters)*actionablesChamberRow
			if d.hazards > 0 {
				d.h += 30 + d.hazards*actionablesChamberRow
			}
			cards = append(cards, d)
		}
	}
	cols := max(1, min(2, (width+8)/(actionablesCardMinW+8)))
	cardW := (width - (cols-1)*8) / cols
	heights := make([]int, cols)
	for i := range cards {
		col := slices.Index(heights, slices.Min(heights))
		cards[i].x, cards[i].y = col*(cardW+8), heights[col]
		heights[col] += cards[i].h + 8
	}
	autoLevelDungeons.key, autoLevelDungeons.width, autoLevelDungeons.cards, autoLevelDungeons.cardW = key, width, cards, cardW
	autoLevelDungeons.contentH = max(0, slices.Max(append(heights, 0))-8)
	return cards, cardW, autoLevelDungeons.contentH
}

// dungeonMonsters lists a map's monster kinds: bosses first, then by
// authored level.
func dungeonMonsters(m mapInfo) []dungeonMonster {
	var out []dungeonMonster
	for _, spawn := range m.Data.MonsterSpawns {
		def, ok := monster.MonsterConfig.Monsters[spawn.MonsterKey]
		if !ok {
			continue
		}
		if i := slices.IndexFunc(out, func(c dungeonMonster) bool { return c.key == spawn.MonsterKey }); i >= 0 {
			out[i].count++
			continue
		}
		out = append(out, dungeonMonster{key: spawn.MonsterKey, def: def, count: 1})
	}
	slices.SortFunc(out, func(a, b dungeonMonster) int {
		if a.def.Boss != b.def.Boss {
			if a.def.Boss {
				return -1
			}
			return 1
		}
		if a.def.Level != b.def.Level {
			return b.def.Level - a.def.Level
		}
		return strings.Compare(a.def.Name, b.def.Name)
	})
	return out
}

// dungeonView is the scrolled card column below the dungeons header.
func (g actionablesGeometry) dungeonView() rect {
	return rect{g.maps.x, g.maps.y + 22, g.maps.w - 12, g.maps.h - 22}
}

// drawDungeons is every auto-levelled map at the selected visit level, through
// the game's own scaling: the numbers a party meets there.
func (v *viewer) drawDungeons(screen *ebiten.Image, g actionablesGeometry) {
	level, r := actionablesPage.level, g.maps
	header := fmt.Sprintf("AUTO-LEVEL MAPS AT VISIT LEVEL %d", level)
	game.DrawShadedText(screen, header, r.x+4, r.y+4, overlayAccent)
	game.DrawShadedText(screen, fmt.Sprintf("party average %d", level-v.cfg.AdventureScaling.VisitLevelBonus), r.x+4+game.ShadedTextWidth(header)+16, r.y+4, overlayMuted)
	view := g.dungeonView()
	cards, cardW, total := v.dungeonCards(view.w)
	if len(cards) == 0 {
		game.DrawShadedText(screen, "No adventure schedule opens a map.", r.x+4, view.y+6, overlayMuted)
		return
	}
	clip := catalogClip(screen, view)
	for _, c := range cards {
		y := view.y + c.y - actionablesPage.dungeonScroll
		if y+c.h < view.y || y > view.y+view.h {
			continue
		}
		v.drawDungeonCard(clip, rect{view.x + c.x, y, cardW, c.h}, c, level)
	}
	drawCatalogScroll(screen, view, actionablesPage.dungeonScroll, total)
}

func (v *viewer) drawDungeonCard(screen *ebiten.Image, r rect, d autoLevelDungeon, level int) {
	s, a := v.cfg.AdventureScaling, d.m.Config.Adventure
	drawFilledRect(screen, r.x, r.y, r.w, r.h, overlayBG)
	drawRectBorder(screen, r.x, r.y, r.w, r.h, 1, overlayBorder)
	tint := d.m.Config.DefaultFloorColor
	drawFilledRect(screen, r.x+1, r.y+1, r.w-2, 3, color.RGBA{uint8(tint[0]), uint8(tint[1]), uint8(tint[2]), 255})
	x, right := r.x+10, r.x+r.w-10
	game.DrawShadedText(screen, clipText(d.m.Config.Name, r.w/2), x, r.y+12, overlayText)
	if a.ExperienceCap > 0 {
		drawRightText(screen, fmt.Sprintf("XP cap %d / hero", a.ExperienceCap), right, r.y+12, overlayMuted)
	}
	// Numbers end at fixed fractions of the card, the name takes the rest.
	cols := [5]int{right - r.w*48/100, right - r.w*36/100, right - r.w*19/100, right - r.w*11/100, right}
	y := r.y + 32
	for i, label := range [5]string{"OWN", "HP", "HIT", "AC", "XP"} {
		drawRightText(screen, label, cols[i], y, overlayMuted)
	}
	game.DrawShadedText(screen, "MONSTER", x, y, overlayMuted)
	y += actionablesChamberRow + 2
	nameW := cols[0] - game.ShadedTextWidth("OWN") - 8 - x
	for _, c := range d.monsters {
		st := game.AdventureScaledStats(s, c.def, c.def.Boss, level)
		name := c.def.Name
		if c.count > 1 {
			name += " x" + strconv.Itoa(c.count)
		}
		tc := sideColor(c.def.Boss)
		game.DrawShadedText(screen, clipText(name, nameW), x, y, tc)
		for i, text := range [5]string{strconv.Itoa(c.def.Level), strconv.Itoa(st.HP), fmt.Sprintf("%d-%d", st.DamageMin, st.DamageMax), strconv.Itoa(st.ArmorClass), strconv.Itoa(st.Experience)} {
			col := tc
			if i == 0 {
				col = overlayMuted
			}
			drawRightText(screen, text, cols[i], y, col)
		}
		y += actionablesChamberRow
	}
	if d.hazards == 0 {
		return
	}
	y += 6
	drawFilledRect(screen, x, y, r.w-20, 1, overlayBorder)
	game.DrawShadedText(screen, "HAZARDS  (% of the boss hit)", x, y+6, overlayMuted)
	y += actionablesChamberRow + 6
	for _, h := range game.AdventureHazards(s, a, level) {
		game.DrawShadedText(screen, clipText(h.Name, r.w*48/100), x, y, overlayText)
		drawRightText(screen, hazardText(h), right, y, overlayDanger)
		y += actionablesChamberRow
	}
}

// hazardText is a hazard's damage: a hit for traps and lanes, per second for
// occupation and a lane's lingering field.
func hazardText(h game.AdventureHazard) string {
	part := func(percent, damage int, perSecond bool) string {
		text := strconv.Itoa(damage)
		if perSecond {
			text += "/s"
		}
		if percent > 0 {
			text = strconv.Itoa(percent) + "% = " + text
		}
		return text
	}
	var parts []string
	if h.Damage > 0 {
		parts = append(parts, part(h.Percent, h.Damage, h.Kind == config.EffectOccupation))
	}
	if h.Sustain > 0 {
		parts = append(parts, "then "+part(h.SustainPercent, h.Sustain, true))
	}
	return strings.Join(parts, ", ")
}
