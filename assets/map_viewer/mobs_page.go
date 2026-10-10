package main

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/items"
	"ugataima/internal/monster"
)

// Mobs page: pick a monster, see its full stat sheet, drop table and a live
// animated preview - the game's own AI + renderer via game.MobPreview, so a
// banding mob shows its whole flock patrolling.

// infoLine is one stat-sheet row with its tint (the game's line-color idiom:
// whole lines carry meaning colors - damage red, HP green, resists by school,
// drops by rarity). Header lines render on a filled band (the editor/game
// section-header convention). Shared by every detail sheet (mobs page, saves
// page); rows that show an item carry it for the hover tooltip.
type infoLine struct {
	text   string
	col    color.Color
	item   *items.Item
	header bool
	depth  int // nesting depth in the authored-source inspector
}

// appendInfoHeader appends a section-header row, inserting a blank spacer row
// first unless the sheet is empty or already ends blank - THE single place
// encoding the "headers never touch the text above" spacing convention.
func appendInfoHeader(rows []infoLine, format string, args ...any) []infoLine {
	if n := len(rows); n > 0 && rows[n-1].text != "" {
		rows = append(rows, infoLine{col: color.White}) // spacer row (never drawn, but keep col non-nil)
	}
	return append(rows, infoLine{text: fmt.Sprintf(format, args...), col: color.White, header: true})
}

var mobsPage struct {
	preview    *game.MobPreview
	keys       []string // sorted monster keys
	selIdx     int
	scroll     int
	initErr    string
	infoOffset float64    // retain fractional wheel deltas between frames
	info       []infoLine // flowing stat+drop lines for the selected mob
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
	if len(keys) > 0 {
		v.selectMob(0)
		rows := v.mobCatalogRows()
		if i := catalogSelectedRow(rows, v.browser.mobSelection, mobsPage.selIdx); i >= 0 {
			l := v.mobCatalogLayout()
			mobsPage.scroll = clampInt(i*catalogRowHeight-l.list.h/2, 0, max(0, len(rows)*catalogRowHeight-l.list.h))
		}
	}
}

func (v *viewer) selectMob(idx int) {
	if idx < 0 || idx >= len(mobsPage.keys) {
		return
	}
	mobsPage.selIdx = idx
	mobsPage.infoOffset = 0
	key := mobsPage.keys[idx]
	mobsPage.preview.Select(key)
	var runtime *monster.Monster3D
	if staged := mobsPage.preview.Monsters(); len(staged) > 0 {
		runtime = staged[0]
	}
	mobsPage.info = buildMobInfoRuntime(key, v.monsterCfg.Monsters[key], runtime, v.cfg.GetTileSize(), game.MonsterCatalogEffectContext(v.cfg))
}

// buildMobInfoRuntime uses a staged monster when available so the editor shows
// effective values after the same setup/mirroring paths the game runs. The
// definition remains the source for authored behavior and loot.
func buildMobInfoRuntime(key string, def monster.MonsterDefinition, runtime *monster.Monster3D, tileSize float64, contexts ...monster.CombatEffectContext) []infoLine {
	var out []infoLine
	addc := func(col color.Color, format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		// Retain authored nesting as layout data, not whitespace that wrapping
		// discards. Text is wrapped once, at the actual pane width.
		trimmed := strings.TrimLeft(line, " ")
		out = append(out, infoLine{text: trimmed, col: col, depth: (len(line) - len(trimmed)) / 2})
	}
	add := func(format string, args ...any) { addc(mobStatDefault, format, args...) }
	addHeader := func(format string, args ...any) {
		out = appendInfoHeader(out, format, args...)
	}

	level, xp := def.Level, def.Experience
	hp, armor, dodge := def.MaxHitPoints, def.ArmorClass, def.PerfectDodge
	damageMin, damageMax, trueDamage := def.DamageMin, def.DamageMax, def.TrueDamage
	attacks := monster.TurnBasedAttackCount(def.AttacksPerRound, def.AttackCooldownMult)
	cooldownMult := def.AttackCooldownMult
	speed, alertTiles, attackTiles := def.Speed, def.AlertRadius, def.AttackRadius
	rangedTiles := def.RangedAttackRange
	effectDef := def
	resistances := make(map[string]int, len(def.Resistances))
	for school, value := range def.Resistances {
		resistances[school] = value
	}
	if runtime != nil {
		level, xp = runtime.Level, runtime.Experience
		hp, armor, dodge = runtime.MaxHitPoints, runtime.ArmorClass, runtime.PerfectDodge
		damageMin, damageMax, trueDamage = runtime.DamageMin, runtime.DamageMax, runtime.TrueDamage
		attacks = monster.TurnBasedAttackCount(runtime.AttacksPerRound, runtime.AttackCooldownMultiplier)
		cooldownMult = runtime.AttackCooldownMultiplier
		speed = runtime.Speed
		if tileSize > 0 {
			alertTiles = runtime.AlertRadius / tileSize
			attackTiles = runtime.AttackRadius / tileSize
			if runtime.HasRangedAttack() {
				rangedTiles = runtime.GetAttackRangePixels() / tileSize
			}
		}
		effectDef.ProjectileSpell = runtime.ProjectileSpell
		effectDef.ProjectileWeapon = runtime.ProjectileWeapon
		effectDef.DamageMin, effectDef.DamageMax = runtime.DamageMin, runtime.DamageMax
		resistances = make(map[string]int, len(runtime.Resistances))
		for school, value := range runtime.Resistances {
			resistances[school.String()] = value
		}
	}

	addHeader("IDENTITY")
	add("%s  (key: %s)", def.Name, key)
	if def.Type != "" {
		add("Type: %s", def.Type)
	}
	if def.Disposition != "" {
		add("Disposition: %s", def.Disposition)
	}
	add("Level %d   XP %d", level, xp)
	if def.Champion != "" {
		addHeader("CHAMPION")
		tierName := config.ChampionDefaultTier
		if runtime != nil && runtime.ChampionTier != "" {
			tierName = runtime.ChampionTier
		}
		if champion := config.GetChampionDefinition(def.Champion); champion != nil {
			race := titleCase(strings.ReplaceAll(champion.Race, "_", " "))
			if cfg := config.GlobalConfig; cfg != nil {
				race = cfg.Characters.Races["human"].Name
				if r, ok := cfg.Characters.Races[champion.Race]; ok {
					race = r.Name
				}
			}
			add("Champion: %s   Tier: %s", champion.Name, titleCase(tierName))
			add("Class: %s   Race: %s", titleCase(strings.ReplaceAll(champion.Class, "_", " ")), race)
			if len(champion.Skills) > 0 {
				add("Skills: %s", strings.Join(champion.Skills, ", "))
			}
			if gear := champion.Equipment[tierName]; len(gear) > 0 {
				add("Tier loadout: %s", strings.Join(gear, ", "))
			}
			if champion.SpellCastChance > 0 {
				add("Spell cast: %.0f%% from {%s}", champion.SpellCastChance*100, strings.Join(champion.SpellSchools, ", "))
			}
			if champion.OpeningSpell != "" {
				add("Opening spell: %s", champion.OpeningSpell)
				if len(champion.OpeningSpellTiers) > 0 {
					add("Opening spell tiers: %s", strings.Join(champion.OpeningSpellTiers, ", "))
				}
			}
			if len(champion.ExtraSpells) > 0 {
				add("Extra spells: %s", strings.Join(champion.ExtraSpells, ", "))
			}
		}
	}

	addHeader("COMBAT")
	addc(mobStatHP, "HP %d", hp)
	add("Armor Class %d", armor)
	if dodge > 0 {
		add("Perfect dodge: %d%%", dodge)
	}
	if def.HasAttackStats() {
		dmg := fmt.Sprintf("Damage %d-%d", damageMin, damageMax)
		if trueDamage > 0 {
			dmg += fmt.Sprintf(" +%d true", trueDamage)
		}
		dmg += fmt.Sprintf("   TB attacks: %d", attacks)
		addc(mobStatDamage, "%s", dmg)
		if cooldownMult != 0 && cooldownMult != 1 {
			add("RT attack cooldown: x%.2f", cooldownMult)
		}
		// AttackRadius is not a ranged monster's melee reach. Combat selects
		// adjacent melee for ordinary ranged actors, but not ranged champions.
		ranged := effectDef.ProjectileSpell != "" || effectDef.ProjectileWeapon != ""
		switch {
		case def.Champion != "" && runtime == nil:
			add("Attack reach: tier/loadout-dependent")
		case ranged:
			if def.Champion == "" {
				add("Melee: adjacent tiles")
			}
			if rangedTiles > 0 {
				add("Ranged attack range: %.1f tiles", rangedTiles)
			} else {
				add("Ranged attack range: resolved in preview")
			}
		case def.Champion != "":
			add("Melee weapon reach: %.1f tiles", attackTiles)
		default:
			add("Melee reach: %.1f tiles", attackTiles)
		}
	} else {
		add("Does not attack")
	}
	add("Speed %.1f   Alert %.0f tiles", speed, alertTiles)
	if def.EnrageAtHP > 0 && def.EnrageCooldownMult > 0 {
		enragedAttacks := attacks * monster.TurnBasedAttacksForCooldownMultiplier(def.EnrageCooldownMult)
		add("Enraged TB attacks: %d", enragedAttacks)
	}

	effects := effectDef.CombatEffectLines(contexts...)
	if len(effects) > 0 {
		addHeader("ABILITIES")
	}
	for _, line := range effects {
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
	if def.AnimateWhenIdle {
		flags = append(flags, "animated idle")
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
	if len(flags) > 0 || def.BandGroup != "" || len(def.Prey) > 0 || def.Arboreal != nil {
		addHeader("BEHAVIOR")
	}
	if len(flags) > 0 {
		add("Flags: %s", strings.Join(flags, ", "))
	}
	if def.BandGroup != "" {
		add("Shared aggro group: %s", def.BandGroup)
	}
	if len(def.Prey) > 0 {
		add("Prey: %s (%.1f tiles)", strings.Join(def.Prey, ", "), def.PreyRadius)
	}
	if a := def.Arboreal; a != nil {
		add("Canopy height %.1f tiles, leap %.1f tiles", a.HeightTiles, a.JumpRangeTiles)
		add("Climb %gs, jump %gs, rest %gs", a.ClimbSeconds, a.JumpSeconds, a.RestSeconds)
		add("Trees: %s", strings.Join(a.TreeTiles, ", "))
	}

	// Resistances: one line per school in the school's tint, sorted for a
	// stable sheet.
	if len(resistances) > 0 {
		addHeader("RESISTS")
		resKeys := make([]string, 0, len(resistances))
		for r := range resistances {
			resKeys = append(resKeys, r)
		}
		sort.Strings(resKeys)
		for _, r := range resKeys {
			addc(game.SchoolColor(r), "  %s %d%%", titleCase(r), resistances[r])
		}
	}

	addHeader("PLACEMENT")
	if len(def.Biomes) > 0 {
		add("Biomes: %s", strings.Join(def.Biomes, ", "))
	}
	if len(def.WalkableTileOverrides) > 0 {
		add("Walkable tile overrides: %s", strings.Join(def.WalkableTileOverrides, ", "))
	}
	add("Letter '%s'   sprite %s   size %.1f", def.Letter, def.Sprite, def.GetSizeGameMultiplier())

	// Drop table: each entry tinted by its rarity (metal tiers render as the
	// game's gradient).
	addHeader("DROPS")
	if def.GoldMax > 0 {
		addc(mobStatGold, "Gold %d-%d", def.GoldMin, def.GoldMax)
	}
	entries := config.GetLootTable(key, def.Boss)
	if len(entries) == 0 {
		addc(mobStatHeader, "(no item drops)")
	}
	for _, e := range entries {
		name, rarity := e.Key, ""
		switch e.Type {
		case "item", "harvest":
			if d, ok := config.GetItemDefinition(e.Key); ok && d != nil {
				name, rarity = d.Name, d.Rarity
			}
		case "weapon":
			if d, ok := config.GetWeaponDefinition(e.Key); ok && d != nil {
				name, rarity = d.Name, d.Rarity
			}
		}
		rolls := ""
		if e.RollCount() > 1 {
			rolls = fmt.Sprintf(" x%d rolls", e.RollCount())
		}
		addc(game.RarityColor(rarity), "%4.1f%%%s  %s (%s)", e.Chance*100, rolls, name, e.Type)
	}
	return out
}

func scrollCatalogRows(offset, wheelY float64, total, capacity int) float64 {
	limit := float64(max(0, total-capacity))
	return max(0, min(limit, offset-wheelY*3))
}
