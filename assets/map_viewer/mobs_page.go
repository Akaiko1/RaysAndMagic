package main

import (
	"fmt"
	"image"
	"image/color"
	"sort"
	"strings"

	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/monster"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Mobs page: pick a monster, see its full stat sheet, drop table and a live
// animated preview - the game's own AI + renderer via game.MobPreview, so a
// banding mob shows its whole flock patrolling.

const (
	mobListW    = 300
	mobRowH     = 22
	mobListPadY = 8
	mobInfoRowH = 14
	mobInfoColW = 300
)

// infoLine is one stat-sheet row with its tint (the game's line-color idiom:
// whole lines carry meaning colors - damage red, HP green, resists by school,
// drops by rarity).
type infoLine struct {
	text string
	col  color.Color
}

var mobsPage struct {
	preview *game.MobPreview
	keys    []string // sorted monster keys
	labels  []string // "L%2d Name" per key
	selIdx  int
	scroll  int
	initErr string
	info    []infoLine // flowing stat+drop lines for the selected mob
}

// Meaning tints for the stat sheet (school/rarity tints come from the game).
var (
	mobStatDefault = color.RGBA{215, 215, 225, 255}
	mobStatHeader  = color.RGBA{150, 150, 175, 255}
	mobStatDamage  = color.RGBA{255, 95, 75, 255}
	mobStatHP      = color.RGBA{110, 230, 110, 255}
	mobStatGold    = color.RGBA{255, 210, 80, 255}
)

// ensureMobsPage lazily builds the sandbox and the monster catalog on first
// tab open, so editor startup cost is unchanged and an init failure degrades
// to an on-page message.
func (v *viewer) ensureMobsPage() {
	if mobsPage.preview != nil || mobsPage.initErr != "" {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			mobsPage.initErr = fmt.Sprintf("mob sandbox failed to start: %v", r)
		}
	}()
	p, err := game.NewMobPreview(config.GlobalConfig)
	if err != nil {
		mobsPage.initErr = err.Error()
		return
	}
	mobsPage.preview = p

	keys := make([]string, 0, len(v.monsterCfg.Monsters))
	for k := range v.monsterCfg.Monsters {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := v.monsterCfg.Monsters[keys[i]], v.monsterCfg.Monsters[keys[j]]
		if a.Level != b.Level {
			return a.Level < b.Level
		}
		return a.Name < b.Name
	})
	mobsPage.keys = keys
	mobsPage.labels = make([]string, len(keys))
	for i, k := range keys {
		def := v.monsterCfg.Monsters[k]
		band := ""
		if def.Banding {
			band = " [band]"
		}
		mobsPage.labels[i] = fmt.Sprintf("L%-3d %s%s", def.Level, def.Name, band)
	}
	if len(keys) > 0 {
		v.selectMob(0)
	}
}

func (v *viewer) selectMob(idx int) {
	if idx < 0 || idx >= len(mobsPage.keys) {
		return
	}
	mobsPage.selIdx = idx
	key := mobsPage.keys[idx]
	mobsPage.preview.Select(key)
	mobsPage.info = buildMobInfo(key, v.monsterCfg.Monsters[key])
}

func (v *viewer) updateMobsPage() {
	v.ensureMobsPage()
	if mobsPage.preview == nil {
		return
	}

	moved := 0
	if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
		moved = 1
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
		moved = -1
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageDown) {
		moved = 10
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageUp) {
		moved = -10
	}
	if moved != 0 {
		idx := mobsPage.selIdx + moved
		if idx < 0 {
			idx = 0
		}
		if idx >= len(mobsPage.keys) {
			idx = len(mobsPage.keys) - 1
		}
		v.selectMob(idx)
		v.scrollMobSelectionIntoView()
	}

	_, wheelY := ebiten.Wheel()
	if wheelY != 0 {
		mx, _ := ebiten.CursorPosition()
		if mx < mobListW {
			mobsPage.scroll -= int(wheelY * 30)
			v.clampMobScroll()
		}
	}

	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		if mx < mobListW && my > pageBarHeight {
			idx := (my - pageBarHeight - mobListPadY + mobsPage.scroll) / mobRowH
			if idx >= 0 && idx < len(mobsPage.keys) {
				v.selectMob(idx)
			}
		}
	}

	mobsPage.preview.Step()
}

func (v *viewer) mobListViewportH() int {
	return windowHeight - pageBarHeight - mobListPadY*2
}

func (v *viewer) clampMobScroll() {
	maxScroll := len(mobsPage.keys)*mobRowH - v.mobListViewportH()
	if maxScroll < 0 {
		maxScroll = 0
	}
	if mobsPage.scroll > maxScroll {
		mobsPage.scroll = maxScroll
	}
	if mobsPage.scroll < 0 {
		mobsPage.scroll = 0
	}
}

func (v *viewer) scrollMobSelectionIntoView() {
	top := mobsPage.selIdx * mobRowH
	if top-mobsPage.scroll < 0 {
		mobsPage.scroll = top
	}
	if bottom := top + mobRowH; bottom-mobsPage.scroll > v.mobListViewportH() {
		mobsPage.scroll = bottom - v.mobListViewportH()
	}
	v.clampMobScroll()
}

func (v *viewer) drawMobsPage(screen *ebiten.Image) {
	if mobsPage.initErr != "" {
		ebitenutil.DebugPrintAt(screen, mobsPage.initErr, contentPad, pageBarHeight+contentPad)
		return
	}
	if mobsPage.preview == nil {
		ebitenutil.DebugPrintAt(screen, "starting mob sandbox...", contentPad, pageBarHeight+contentPad)
		return
	}

	// Left: selectable monster list, clipped so scrolled rows never overlap
	// the page tab bar.
	vector.FillRect(screen, 0, float32(pageBarHeight), float32(mobListW), float32(windowHeight-pageBarHeight), color.RGBA{22, 22, 32, 255}, false)
	list := screen.SubImage(image.Rect(0, pageBarHeight, mobListW, windowHeight)).(*ebiten.Image)
	y0 := pageBarHeight + mobListPadY - mobsPage.scroll
	for i, label := range mobsPage.labels {
		ry := y0 + i*mobRowH
		if ry < pageBarHeight-mobRowH || ry > windowHeight {
			continue
		}
		if i == mobsPage.selIdx {
			vector.FillRect(list, 0, float32(ry-3), float32(mobListW), float32(mobRowH), color.RGBA{60, 90, 140, 200}, false)
		}
		game.DrawShadedText(list, label, 8, ry, mobStatDefault)
	}

	// Top right: the live stage, aspect-fit.
	scene := mobsPage.preview.Scene()
	panelX := mobListW + contentPad
	panelY := pageBarHeight + contentPad
	panelW := windowWidth - panelX - contentPad
	sceneH := (windowHeight - pageBarHeight) * 55 / 100
	sw, sh := scene.Bounds().Dx(), scene.Bounds().Dy()
	scale := float64(panelW) / float64(sw)
	if s := float64(sceneH) / float64(sh); s < scale {
		scale = s
	}
	dw, dh := int(float64(sw)*scale), int(float64(sh)*scale)
	dx := panelX + (panelW-dw)/2
	vector.FillRect(screen, float32(dx-2), float32(panelY-2), float32(dw+4), float32(dh+4), color.RGBA{60, 60, 80, 255}, false)
	opts := &ebiten.DrawImageOptions{}
	opts.GeoM.Scale(scale, scale)
	opts.GeoM.Translate(float64(dx), float64(panelY))
	screen.DrawImage(scene, opts)

	// Below: the stat sheet + drop table, flowing top-to-bottom into columns.
	infoY := panelY + dh + contentPad
	infoH := windowHeight - infoY - contentPad
	rowsPerCol := infoH / mobInfoRowH
	if rowsPerCol < 1 {
		rowsPerCol = 1
	}
	info := screen.SubImage(image.Rect(panelX, infoY, windowWidth, windowHeight)).(*ebiten.Image)
	for i, line := range mobsPage.info {
		col := i / rowsPerCol
		row := i % rowsPerCol
		x := panelX + col*mobInfoColW
		if x+mobInfoColW > windowWidth+mobInfoColW/2 {
			game.DrawShadedText(info, "...", x, infoY, mobStatHeader)
			break
		}
		game.DrawShadedText(info, line.text, x, infoY+row*mobInfoRowH, line.col)
	}
}

// buildMobInfo renders a monster definition into the page's stat + drop
// lines. Zero-valued optional fields are skipped, so the sheet shows exactly
// what the YAML authors.
func buildMobInfo(key string, def monster.MonsterDefinition) []infoLine {
	var out []infoLine
	addc := func(col color.Color, format string, args ...any) {
		out = append(out, infoLine{fmt.Sprintf(format, args...), col})
	}
	add := func(format string, args ...any) { addc(mobStatDefault, format, args...) }

	add("%s  (key: %s)", def.Name, key)
	if def.Type != "" {
		add("Type: %s", def.Type)
	}
	add("Level %d   XP %d", def.Level, def.Experience)
	addc(mobStatHP, "HP %d", def.MaxHitPoints)
	add("AC %d", def.ArmorClass)
	if def.PerfectDodge > 0 {
		add("Perfect dodge: %d%%", def.PerfectDodge)
	}

	dmg := fmt.Sprintf("Damage %d-%d", def.DamageMin, def.DamageMax)
	if def.TrueDamage > 0 {
		dmg += fmt.Sprintf(" +%d true", def.TrueDamage)
	}
	if def.AttacksPerRound > 1 {
		dmg += fmt.Sprintf(", %d attacks/round", def.AttacksPerRound)
	}
	addc(mobStatDamage, "%s", dmg)
	if def.AttackCooldownMult != 0 && def.AttackCooldownMult != 1 {
		add("Attack cooldown x%.2f", def.AttackCooldownMult)
	}
	add("Speed %.1f   Alert %.0f tiles   Melee reach %.1f tiles", def.Speed, def.AlertRadius, def.AttackRadius)
	if def.RangedAttackRange > 0 {
		add("Ranged range: %.0f tiles", def.RangedAttackRange)
	}

	for _, line := range def.CombatEffectLines() {
		var col color.Color = mobStatDefault
		if line.School != "" {
			col = game.SchoolColor(line.School)
		}
		addc(col, "%s", line.Text)
	}

	// Behaviour flags.
	var flags []string
	if def.Banding {
		flags = append(flags, "banding")
	}
	if def.Flying {
		flags = append(flags, "flying")
	}
	if def.PassiveUntilHit {
		flags = append(flags, "passive until hit")
	}
	if def.IgnoresArmor {
		flags = append(flags, "ignores armor")
	}
	if def.AggroWholeMap {
		flags = append(flags, "aggro whole map")
	}
	if def.WardedByIdols {
		flags = append(flags, "warded by idols")
	}
	if def.WarlordIdol {
		flags = append(flags, "ward idol")
	}
	if len(flags) > 0 {
		add("Flags: %s", strings.Join(flags, ", "))
	}

	// Boss kit.
	if def.PassiveUntilQuest != "" {
		add("Sealed until quest: %s", def.PassiveUntilQuest)
	}
	if def.EvadeRadiusTiles > 0 {
		add("Evades within %.1f tiles (cd %.0fs)", def.EvadeRadiusTiles, def.BossCooldownSecs)
	}
	if def.InfernoChance > 0 {
		add("Inferno nova: %.0f%% for %d", def.InfernoChance*100, def.InfernoDamage)
	}
	if def.TeleportAtHP > 0 {
		add("Blinks below %d HP (%.0f%%)", def.TeleportAtHP, def.TeleportChance*100)
	}
	if def.SummonChance > 0 || len(def.SummonMonsters) > 0 {
		n := def.SummonCount
		if n == 0 {
			n = 1
		}
		add("Summons %dx {%s}: %.0f%% (max %d)", n, strings.Join(def.SummonMonsters, ", "), def.SummonChance*100, def.SummonMax)
	}
	if def.EnrageAtHP > 0 {
		add("Enrages below %d HP: dmg x%.1f, cd x%.1f", def.EnrageAtHP, def.EnrageDamageMult, def.EnrageCooldownMult)
	}
	if def.DeathRalliesType != "" {
		add("Death rallies: %s", def.DeathRalliesType)
	}

	// Resistances: one line per school in the school's tint, sorted for a
	// stable sheet.
	if len(def.Resistances) > 0 {
		addc(mobStatHeader, "Resists:")
		resKeys := make([]string, 0, len(def.Resistances))
		for r := range def.Resistances {
			resKeys = append(resKeys, r)
		}
		sort.Strings(resKeys)
		for _, r := range resKeys {
			addc(game.SchoolColor(r), "  %s %d", r, def.Resistances[r])
		}
	}

	if len(def.Biomes) > 0 {
		add("Biomes: %s", strings.Join(def.Biomes, ", "))
	}
	if len(def.HabitatPrefs) > 0 {
		add("Habitat: %s", strings.Join(def.HabitatPrefs, ", "))
	}
	add("Letter '%s'   sprite %s   size %.1f", def.Letter, def.Sprite, def.GetSizeGameMultiplier())

	// Drop table: each entry tinted by its rarity (metal tiers render as the
	// game's gradient).
	addc(mobStatHeader, "")
	addc(mobStatHeader, "--- Drops ---")
	if def.GoldMax > 0 {
		addc(mobStatGold, "Gold %d-%d", def.GoldMin, def.GoldMax)
	}
	entries := config.GetLootTable(key)
	if len(entries) == 0 {
		addc(mobStatHeader, "(no item drops)")
	}
	for _, e := range entries {
		name, rarity := e.Key, ""
		switch e.Type {
		case "item":
			if d, ok := config.GetItemDefinition(e.Key); ok && d != nil {
				name, rarity = d.Name, d.Rarity
			}
		case "weapon":
			if d, ok := config.GetWeaponDefinition(e.Key); ok && d != nil {
				name, rarity = d.Name, d.Rarity
			}
		}
		addc(game.RarityColor(rarity), "%4.1f%%  %s (%s)", e.Chance*100, name, e.Type)
	}
	return out
}
