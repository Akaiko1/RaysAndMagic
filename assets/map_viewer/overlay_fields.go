package main

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
)

type overlayField struct {
	label, value, help string
	kind               string
	depth              int
	disabled           bool
	choices            []string
	multi              bool
	apply              func(string) error
	action             func()
}

func overlayString(label string, p *string, help string, choices ...string) overlayField {
	return overlayField{label: label, value: *p, help: help, choices: choices, apply: func(s string) error { *p = s; return nil }}
}
func overlayInt(label string, p *int, help string) overlayField {
	return overlayField{label: label, value: strconv.Itoa(*p), help: help, apply: func(s string) error {
		n, e := strconv.Atoi(strings.TrimSpace(s))
		if e != nil {
			return fmt.Errorf("Enter a whole number")
		}
		*p = n
		return nil
	}}
}
func overlayFloat(label string, p *float64, help string) overlayField {
	return overlayField{label: label, value: strconv.FormatFloat(*p, 'f', -1, 64), help: help, apply: func(s string) error {
		n, e := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if e != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 1e6 {
			return fmt.Errorf("Enter a number from 0 to 1000000")
		}
		*p = n
		return nil
	}}
}
func overlaySignedFloat(label string, p *float64, help string, low, high float64) overlayField {
	return overlayField{label: label, value: strconv.FormatFloat(*p, 'f', -1, 64), help: help, apply: func(s string) error {
		n, e := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if e != nil || math.IsNaN(n) || n < low || n > high {
			return fmt.Errorf("Enter a number from %g to %g", low, high)
		}
		*p = n
		return nil
	}}
}

// overlayNumbers edits a fixed-size tuple typed as "a, b, c".
func overlayNumbers(label string, values []*float64, help string, low, high float64) overlayField {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.FormatFloat(*v, 'f', -1, 64)
	}
	return overlayField{label: label, value: strings.Join(parts, ", "), help: help, apply: func(s string) error {
		fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
		if len(fields) != len(values) {
			return fmt.Errorf("Enter %d numbers separated by commas", len(values))
		}
		parsed := make([]float64, len(values))
		for i, f := range fields {
			n, e := strconv.ParseFloat(f, 64)
			if e != nil || math.IsNaN(n) || n < low || n > high {
				return fmt.Errorf("Each number must be from %g to %g", low, high)
			}
			parsed[i] = n
		}
		for i := range values {
			*values[i] = parsed[i]
		}
		return nil
	}}
}

func overlayRGB(label string, p *[3]int, help string) overlayField {
	r, g, b := float64(p[0]), float64(p[1]), float64(p[2])
	f := overlayNumbers(label, []*float64{&r, &g, &b}, help, 0, 255)
	apply := f.apply
	f.apply = func(s string) error {
		if err := apply(s); err != nil {
			return err
		}
		*p = [3]int{int(r), int(g), int(b)}
		return nil
	}
	return f
}

func overlayBool(label string, p *bool, help string) overlayField {
	value := "No"
	if *p {
		value = "Yes"
	}
	return overlayField{label: label, value: value, help: help, kind: "toggle", choices: []string{"No", "Yes"}, apply: func(s string) error { *p = s == "Yes"; return nil }}
}
func overlayList(label string, p *[]string, help string, choices []string) overlayField {
	return overlayField{label: label, value: strings.Join(*p, ", "), help: help, choices: choices, multi: true, apply: func(s string) error {
		*p = nil
		for _, x := range strings.Split(s, ",") {
			x = strings.TrimSpace(x)
			if x != "" && !slices.Contains(*p, x) {
				*p = append(*p, x)
			}
		}
		return nil
	}}
}
func overlayAction(label, help string, action func()) overlayField {
	return overlayField{label: label, help: help, action: action}
}
func (v *viewer) overlayFields(d *overlayDocument) []overlayField {
	p := &v.overlay
	a := &d.state.Adventure
	var fields []overlayField
	effects, controls, lanes := []string{}, []string{""}, []string{}
	for _, e := range a.Effects {
		effects = append(effects, e.ID)
		if e.Kind == "lane" {
			lanes = append(lanes, e.ID)
		}
	}
	for _, c := range a.Controls {
		controls = append(controls, c.ID)
	}
	box := func(r *[4]int) {
		for i, label := range []string{"Left column", "Top row", "Right column", "Bottom row"} {
			fields = append(fields, overlayInt(label, &r[i], "Inclusive tile coordinates. Draw or drag the area on the map."))
		}
	}
	switch p.section {
	case "overview":
		return v.overlayOverviewFields(d)
	case "object":
		return v.overlayObjectFields(d)
	case "choice":
		return v.overlayChoiceFields(d)
	case "special":
		if p.selected < 0 || p.selected >= len(d.state.Data.SpecialTileSpawns) {
			return nil
		}
		x := d.state.Data.SpecialTileSpawns[p.selected]
		return []overlayField{overlayInfo("Special tile", x.TileKey, "Behavior is provided by the shared tile catalog."), overlayInfo("Trigger", "Party steps on tile", "Teleporters use the portal rules. Legacy trap tiles are visual only; use + Trap for damage."), overlayInfo("Position", fmt.Sprintf("Column %d, row %d", x.X, x.Y), "Remove and place again to change its position.")}

	case "effect":
		if p.selected < 0 || p.selected >= len(a.Effects) {
			return nil
		}
		e := &a.Effects[p.selected]
		fields = append(fields, overlayField{label: "ID", value: e.ID, help: "Renaming also updates links in this map.", apply: func(s string) error { return d.renameID("effect", p.selected, s) }}, overlayString("Name", &e.Name, "Shown in combat messages."), overlayField{label: "Behavior", value: e.Kind, help: "trap: hits once on entry. occupation: damages every second while standing in it. transfer: moves the party to a receiver tile. lane: a boss attack announced before it strikes.", choices: []string{"trap", "occupation", "transfer", "lane"}, apply: func(s string) error { return d.changeKind(p.selected, s) }})
		fields = append(fields, overlayHeading("Area"))
		box(&e.Rect)
		fields = append(fields, overlayAction("Redraw area", "Drag a new rectangle on the map.", func() { p.tool = "redraw" }))
		fields = append(fields, overlayHeading("Behavior"))
		if e.Kind == "transfer" {
			fields = append(fields, overlayInt("Receiver column", &e.Destination[0], "Destination must be walkable and outside the source."), overlayInt("Receiver row", &e.Destination[1], "Destination must be safe."), overlayAction("Pick receiver on map", "Click the destination tile.", func() { p.tool = "receiver" }), overlayString("Fires with lane", &e.TriggerLane, "Optional: wait until this boss lane fires.", append([]string{""}, lanes...)...), overlayString("Tile colour", &e.School, "Colours the transfer tiles by school. No damage.", "", "physical", "fire", "water", "earth", "air"))
		} else {
			fields = append(fields, overlayInt("Damage", &e.Damage, "Per entry for traps, per strike for lanes, per second for occupation."), overlayString("Damage school", &e.School, "Resistance used against the damage.", "physical", "fire", "water", "earth", "air"), overlayString("Second school", &e.SecondarySchool, "Optional: split the damage equally between two schools.", "", "physical", "fire", "water", "earth", "air"))
		}
		if e.Kind == "lane" {
			fields = append(fields, overlayInt("Lingering damage per second", &e.SustainDamage, "Damage per second on the struck tiles afterwards. 0: only the strike hurts."), overlayFloat("Lingering time (seconds)", &e.DurationSeconds, "Real-time lifetime of the lingering field."), overlayInt("Lingering time (rounds)", &e.DurationRounds, "Turn-based lifetime of the lingering field."))
		}
		if e.Kind == "lane" || e.BossBelowPercent > 0 {
			help := "Time between the announcement and the strike, to leave the marked tiles."
			if e.Kind != "lane" {
				help = "Charge time after the boss falls below the HP threshold, before the effect starts."
			}
			fields = append(fields, overlayFloat("Warning (seconds)", &e.WarningSeconds, help), overlayInt("Warning (rounds)", &e.WarningRounds, "Turn-based warning. "+help))
		}
		if e.Kind == "trap" {
			fields = append(fields, overlayFloat("Rearm (seconds)", &e.RearmSeconds, "Delay before it can hit again on re-entry."), overlayInt("Rearm (rounds)", &e.RearmRounds, "Turn-based rearm delay."))
		}
		threshold := overlayInt("Starts at boss HP (%)", &e.BossBelowPercent, "0: always active. Otherwise it charges once the boss is at or below this HP, then starts after its warning.")
		applyThreshold := threshold.apply
		threshold.apply = func(s string) error {
			if err := applyThreshold(s); err != nil {
				return err
			}
			if e.BossBelowPercent > 0 && (e.WarningSeconds <= 0 || e.WarningRounds < 1) {
				e.WarningSeconds, e.WarningRounds = 1.25, 1 // a zero warning would never count down
			}
			return nil
		}
		fields = append(fields, overlayHeading("Activation conditions"), overlayString("Requires switch", &e.RequiresControl, "Optional: inactive until this switch is used.", controls...), threshold)
	case "control":
		if p.selected < 0 || p.selected >= len(a.Controls) {
			return nil
		}
		c := &a.Controls[p.selected]
		fields = append(fields, overlayHeading("On use"), overlayField{label: "ID", value: c.ID, help: "Renaming also updates this map's switches and conditions.", apply: func(s string) error { return d.renameID("control", p.selected, s) }}, overlayString("Success message", &c.Message, "Feedback after the player uses this mechanism."), overlayList("Disable zones", &c.Disable, "Selected zones stop working for this visit.", effects))
		if a.Boss != nil {
			fields = append(fields, overlayHeading("Boss links"), overlayInfo("Effects on boss", overlayControlBossSummary(d, c.ID), "Edit each effect under Boss > Mechanics."), overlayNavigate("Edit boss mechanics", "Regeneration, armor, critical chance, dodge", "One editor for every boss mechanic and its switch links.", func() { v.overlaySelect("mechanics", 0) }))
		} else {
			fields = append(fields, overlayAction("Set up a target boss", "Choose and place a boss before adding boss actions.", func() { v.overlayBossPicker(d) }))
		}
		key, n := d.controlNPC(c.ID)
		options := []string{}
		for _, spawn := range d.state.Data.NPCSpawns {
			options = append(options, spawn.NPCKey)
		}
		fields = append(fields, overlayHeading("Placed object"), overlayField{label: "Switch NPC", value: key, help: "Select a placed NPC to operate this control. Other dialogue choices are preserved.", choices: options, apply: func(chosen string) error {
			if chosen == key {
				return nil
			}
			target := d.editNPC(chosen)
			if target == nil {
				return fmt.Errorf("NPC is unavailable")
			}
			if key != "" {
				old := d.editNPC(key)
				old.Dialogue.Choices = overlayRemoveActions(old.Dialogue.Choices, "adventure_control", c.ID)
			}
			if target.Dialogue == nil {
				target.Dialogue = &character.NPCDialogue{Greeting: "A service mechanism stands ready."}
			}
			target.Dialogue.Choices = append(target.Dialogue.Choices, &character.NPCDialogueChoice{Text: "Operate the mechanism.", Action: "adventure_control", Control: c.ID})
			return nil
		}})
		if n != nil {
			for i, spawn := range d.state.Data.NPCSpawns {
				if spawn.NPCKey == key {
					fields = append(fields, overlayNavigate("Edit switch object", n.Name, "Edit its appearance, greeting and dialogue actions, including disarm actions.", func() { v.overlaySelect("object", i) }))
					break
				}
			}
		}
		fields = append(fields, overlayHeading("Terrain changes"), overlayAction("+ Add tile change", "Click a tile, then choose its replacement in the properties.", func() { p.tool = "tile-change" }))
		tiles := make([]string, 0, len(v.tileDataByKey))
		for key := range v.tileDataByKey {
			tiles = append(tiles, key)
		}
		sort.Strings(tiles)
		for i := range c.Tiles {
			t := &c.Tiles[i]
			label := fmt.Sprintf("Change %d", i+1)
			fields = append(fields, overlayHeading(label), overlayInt(label+" column", &t.X, "Tile changed when the mechanism is used."), overlayInt(label+" row", &t.Y, "Tile changed when the mechanism is used."), overlayString(label+" new tile", &t.Tile, "Replacement terrain.", tiles...), overlayAction("Remove "+label, "Remove only this terrain edit.", func() { d.checkpoint(); c.Tiles = slices.Delete(c.Tiles, i, i+1) }))
		}
	case "boss", "mechanics", "regeneration", "armor", "mechanic", "arena", "attacks":
		return v.overlayBossFields(d)

	case "phase":
		if a.Boss == nil || p.selected < 0 || p.selected >= len(a.Boss.Phases) {
			return nil
		}
		phase := &a.Boss.Phases[p.selected]
		fields = append(fields, overlayHeading("Phase condition"), overlayInt("At or below HP (%)", &phase.BelowPercent, "Phases must be ordered from highest to lowest threshold."), overlayInt("Special every N actions", &phase.EveryActions, "0 inherits the boss cadence. 1 attacks every action."))
		fields = append(fields, overlayHeading("Attack sequence"))
		for i := range phase.Patterns {
			fields = append(fields, overlayList(fmt.Sprintf("Volley %d", i+1), &phase.Patterns[i], "Selected lanes fire together. Volleys alternate in order.", lanes), overlayAction(fmt.Sprintf("Remove volley %d", i+1), "A phase needs at least one volley.", func() { d.checkpoint(); phase.Patterns = slices.Delete(phase.Patterns, i, i+1) }))
		}
		fields = append(fields, overlayAction("Add volley", "Choose one or more attack lanes after adding.", func() {
			d.checkpoint()
			pattern := []string{}
			if len(lanes) > 0 {
				pattern = append(pattern, lanes[0])
			}
			phase.Patterns = append(phase.Patterns, pattern)
		}))
	case "link":
		if p.selected < 0 || p.selected >= len(a.JumpLinks) {
			return nil
		}
		j := &a.JumpLinks[p.selected]
		for i, label := range []string{"Launch column", "Launch row", "Landing column", "Landing row"} {
			fields = append(fields, overlayInt(label, &j[i], "Endpoints must be walkable, two tiles apart in one straight line."))
		}
		fields = append(fields, overlayAction("Pick endpoints", "Click the launch tile and then the landing tile.", func() { p.tool = "link-redraw"; p.anchorSet = false }))
	case "settings":
		fields = append(fields, overlayHeading("Map lifetime"))
		if v.overlayInOpenWorld(d.key) {
			fields = append(fields, overlayInfo("Respawn", "Not available", "Maps merged into the open world never respawn as a whole."))
		} else {
			fields = append(fields, overlayInt("Respawn every (days)", &d.state.RespawnDays, "The map's monsters return when the party comes back after this many days. 0: never. Must be 0 on scheduled maps."))
		}
		opening := overlayBool("Opens on a schedule", &a.OpeningOwned, "Entered through scheduled openings; each visit starts fresh instead of respawning.")
		applyOpening := opening.apply
		opening.apply = func(s string) error {
			if err := applyOpening(s); err != nil {
				return err
			}
			if !a.OpeningOwned {
				a.ExperienceCap, a.ResetDays = 0, 0
			}
			return nil
		}
		fields = append(fields, overlayHeading("Scheduled openings"), opening, overlayString("Element", &a.Element, "The opening's element. Required on scheduled maps; it names the map's element and does not change combat.", "", "fire", "water", "earth", "air"))
		if a.OpeningOwned {
			fields = append(fields, overlayInt("Experience cap per hero per visit", &a.ExperienceCap, "Monster experience each hero can earn during one visit. 0: no cap."), overlayInt("Reset after the boss falls (days)", &a.ResetDays, "The map starts a fresh visit this many days after its boss is defeated. Needs a boss. 0: no timer."))
		}
		fields = append(fields, overlayHeading("Entrance schedule"))
		if a.Schedule == nil {
			fields = append(fields, overlayAction("Add entrance schedule", "Configure the destination for every day and night.", func() { d.checkpoint(); a.Schedule = &config.AdventureSchedule{Days: make([][2]string, 7)} }))
		} else {
			s := a.Schedule
			fields = append(fields, overlayString("Required quest", &s.RequiresQuest, "The entrance opens once this quest is done.", v.overlayCatalog("quests", "quests")...), overlayString("Required spell", &s.RequiresSpell, "A party member must know this spell. A Jump device also works.", v.overlayCatalog("spells", "spells")...), overlayString("Alternative technique", &s.AlternativeTechnique, "Optional technique accepted instead of the spell.", "", "fold_step"))
			maps := []string{}
			for _, m := range v.maps {
				if m.Key != d.key && m.Config != nil && m.Config.Adventure != nil && m.Config.Adventure.OpeningOwned {
					maps = append(maps, m.Key)
				}
			}
			for i := range s.Days {
				for half, label := range []string{"Day", "Night"} {
					fields = append(fields, overlayString(fmt.Sprintf("Day %d - %s", i+1, label), &s.Days[i][half], "Destination map for this opening.", maps...))
				}
			}
			fields = append(fields, overlayAction("Remove entrance schedule", "Preserves the destination maps.", func() { d.checkpoint(); a.Schedule = nil }))
		}
	}
	return fields
}
