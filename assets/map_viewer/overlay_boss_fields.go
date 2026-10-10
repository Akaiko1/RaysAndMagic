package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"ugataima/internal/config"
)

func overlayMechanicName(stat string) string {
	return map[string]string{"critical_chance": "Critical chance", "perfect_dodge": "Perfect dodge"}[stat]
}
func overlayArenaSummary(b *config.AdventureBoss) string {
	if b == nil || b.Arena == nil {
		return "No movement boundary"
	}
	r := b.Arena
	return fmt.Sprintf("%d x %d tiles", r[2]-r[0]+1, r[3]-r[1]+1)
}
func overlayControlBossSummary(d *overlayDocument, id string) string {
	var parts []string
	if c := d.state.Adventure.Control(id); c != nil {
		if c.StopHealing && d.state.Adventure.Boss != nil && d.state.Adventure.Boss.HealPercent > 0 {
			parts = append(parts, "Stops regeneration")
		}
		if c.ArmorReduction > 0 {
			parts = append(parts, fmt.Sprintf("Armor -%d", c.ArmorReduction))
		}
	}
	for _, r := range d.state.Mechanics {
		if n := r.Reductions[id]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s -%d", overlayMechanicName(r.Stat), n))
		}
	}
	if len(parts) == 0 {
		return "None linked"
	}
	return strings.Join(parts, "; ")
}
func (v *viewer) overlayAddMechanic(d *overlayDocument) {
	var choices []string
	for _, stat := range []string{"critical_chance", "perfect_dodge"} {
		if !slices.ContainsFunc(d.state.Mechanics, func(r config.BossMechanic) bool { return r.Stat == stat }) {
			choices = append(choices, overlayMechanicName(stat))
		}
	}
	if len(choices) == 0 {
		return
	}
	v.overlayChoose("Add boss mechanic", "Each mechanic has its own values and linked switches.", choices, func(name string) error {
		stat := "critical_chance"
		if name == "Perfect dodge" {
			stat = "perfect_dodge"
		}
		d.checkpoint()
		d.state.Mechanics = append(d.state.Mechanics, config.BossMechanic{ID: stat, Stat: stat, Base: 25, Reductions: map[string]int{}})
		v.overlaySelect("mechanic", len(d.state.Mechanics)-1)
		return nil
	})
}
func (v *viewer) overlayBossFields(d *overlayDocument) []overlayField {
	a, p := &d.state.Adventure, &v.overlay
	b := a.Boss
	if b == nil {
		return []overlayField{overlayNote("No boss is configured on this map yet."), overlayAction("Choose and place boss", "Select a boss, then place it on the map.", func() { v.overlayBossPicker(d) })}
	}
	nav := func(label, summary, section string) overlayField {
		return overlayNavigate(label, summary, "Open "+strings.ToLower(label)+" settings.", func() { v.overlaySelect(section, 0) })
	}
	switch p.section {
	case "boss":
		location := "Not placed"
		for _, m := range d.state.Data.MonsterSpawns {
			if m.MonsterKey == b.Monster {
				location = fmt.Sprintf("Column %d, row %d", m.X, m.Y)
			}
		}
		name := b.Monster
		if v.monsterCfg != nil {
			if def, ok := v.monsterCfg.Monsters[b.Monster]; ok {
				name = def.Name
			}
		}
		return []overlayField{overlayHeading("Placement"), overlayInfo("Boss", name, "Archetype: "+b.Monster), overlayInfo("Map position", location, "Exactly one placement belongs to this boss encounter."), overlayAction("Change boss", "Keep the placement and change its archetype.", func() { v.overlayBossPicker(d) }), overlayAction("Place or move boss", "Click empty walkable ground.", func() { p.tool = "boss-place" }), overlayHeading("Encounter settings"), nav("Movement area", overlayArenaSummary(b), "arena"), nav("Mechanics", fmt.Sprintf("Regeneration, armor + %d extra", len(d.state.Mechanics)), "mechanics"), nav("Attack patterns", fmt.Sprintf("%d health phases", len(b.Phases)), "attacks")}
	case "mechanics":
		regen := "Disabled"
		if b.HealPercent > 0 {
			regen = fmt.Sprintf("%d%% HP every 6 s (2 rounds)", b.HealPercent)
		}
		out := []overlayField{overlayHeading("Core mechanics"), nav("Regeneration", regen, "regeneration"), nav("Armor reduction", fmt.Sprintf("Minimum armor: %d", b.ArmorFloor), "armor"), overlayHeading("Extra mechanics")}
		add := overlayAction("+ Add mechanic", "Add critical chance or perfect dodge.", func() { v.overlayAddMechanic(d) })
		if len(d.state.Mechanics) >= 2 {
			add.label = "All extra mechanics added"
			add.disabled = true
		}
		out = append(out, add)
		if len(d.state.Mechanics) == 0 {
			out = append(out, overlayNote("No extra mechanics yet. Add one, then link switches to reduce it."))
		}
		for i, r := range d.state.Mechanics {
			f := overlayNavigate(overlayMechanicName(r.Stat), fmt.Sprintf("Base %d%%  |  Minimum %d%%  |  %d links", r.Base, r.Floor, len(r.Reductions)), "Open this mechanic's values and switch links.", func() { v.overlaySelect("mechanic", i) })
			f.depth = 1
			out = append(out, f)
		}
		return out
	case "regeneration":
		enabled := b.HealPercent > 0
		toggle := overlayBool("Enable regeneration", &enabled, "The boss heals in combat until every linked switch has been operated.")
		toggle.apply = func(s string) error {
			if s == "Yes" {
				b.HealPercent = 5
				b.HealCapPercent = max(25, b.HealCapPercent)
			} else {
				b.HealPercent = 0
			}
			return nil
		}
		out := []overlayField{overlayNote("Heals every 6 seconds in real time, every 2 rounds in turn-based combat. Using all linked switches stops it."), toggle}
		if enabled {
			out = append(out, overlayHeading("Healing"), overlayInt("HP restored each time (%)", &b.HealPercent, "Percent of maximum HP restored every 6 s (2 rounds). Use the toggle to disable."), overlayInt("Total healing per visit (%)", &b.HealCapPercent, "Most healing the boss can gain in one visit, as a percent of maximum HP."))
		}
		active, all := []string{}, []string{}
		for _, c := range a.Controls {
			all = append(all, c.ID)
			if c.StopHealing {
				active = append(active, c.ID)
			}
		}
		link := overlayList("Switches that stop healing", &active, "All selected switches must be used; this is the single editor for regeneration links.", all)
		apply := link.apply
		link.apply = func(s string) error {
			if err := apply(s); err != nil {
				return err
			}
			for i := range a.Controls {
				a.Controls[i].StopHealing = slices.Contains(active, a.Controls[i].ID)
			}
			return nil
		}
		out = append(out, overlayHeading("Linked switches"))
		if len(all) > 0 {
			out = append(out, link)
		}
		if len(active) == 0 {
			out = append(out, overlayNote("Healing needs at least one linked switch. Select an existing switch or place one below."))
		}
		out = append(out, overlayAction("+ Place regeneration switch", "Place a new switch already linked to this mechanic.", func() { p.tool = "healing-control" }))
		return out
	case "armor":
		out := []overlayField{overlayNote("Each linked switch subtracts armor for the rest of the visit. Armor never drops below the minimum."), overlayInt("Minimum armor", &b.ArmorFloor, "Armor never drops below this value. 0 or more."), overlayHeading("Switch reductions")}
		var available []string
		for i := range a.Controls {
			c := &a.Controls[i]
			if c.ArmorReduction == 0 {
				available = append(available, c.ID)
				continue
			}
			f := overlayInt(c.ID+" - armor removed", &c.ArmorReduction, "0 unlinks this switch from armor reduction.")
			f.depth = 1
			out = append(out, f)
		}
		add := overlayAction("+ Link armor switch", "Select a placed switch. Edit its reduction after linking.", func() {
			v.overlayChoose("Link armor switch", "Use + Switch on the map to place a new switch first.", available, func(id string) error { d.checkpoint(); a.Control(id).ArmorReduction = 10; return nil })
		})
		add.disabled = len(available) == 0
		out = append(out, add)
		if len(available) == 0 {
			out = append(out, overlayNote("No unlinked switches. Place one using + Switch above the map."))
		}
		return out
	case "mechanic":
		return v.overlayMechanicFields(d)
	case "arena":
		out := []overlayField{overlayNote("The violet rectangle keeps the boss inside. When the fight starts the party is pulled into it and cannot leave until the boss falls or Town Portal is used. It deals no damage and does not limit attack range.")}
		if b.Arena == nil {
			out = append(out, overlayInfo("Movement", "Unrestricted", "No boss movement boundary is configured."))
			if a.OpeningOwned {
				out = append(out, overlayAction("Draw movement area", "Drag a rectangle around the boss's allowed movement tiles.", func() { p.tool = "arena" }))
			} else {
				out = append(out, overlayNote("A movement area needs a map that opens on a schedule."), nav("Map rules", "Configure scheduled openings", "settings"))
			}
			return out
		}
		out = append(out, overlayInfo("Area size", overlayArenaSummary(b), "Inclusive map tiles; the boss spawn must remain inside."), overlayAction("Redraw movement area", "Drag a new rectangle on the map.", func() { p.tool = "arena" }), overlayHeading("Size and position"))
		for _, field := range []string{"Width (tiles)", "Height (tiles)", "Left column", "Top row"} {
			axis := 0
			if field == "Height (tiles)" || field == "Top row" {
				axis = 1
			}
			size := strings.Contains(field, "tiles")
			value := b.Arena[axis]
			if size {
				value = b.Arena[axis+2] - b.Arena[axis] + 1
			}
			f := overlayInt(field, &value, "Edit the movement boundary in map tiles.")
			apply := f.apply
			f.apply = func(s string) error {
				if err := apply(s); err != nil {
					return err
				}
				r := *b.Arena
				limit := d.state.Data.Width
				if axis == 1 {
					limit = d.state.Data.Height
				}
				if size {
					if value < 1 {
						return fmt.Errorf("Size must be at least one tile")
					}
					r[axis+2] = r[axis] + value - 1
				} else {
					width := r[axis+2] - r[axis]
					r[axis] = value
					r[axis+2] = value + width
				}
				if r[axis] < 0 || r[axis+2] >= limit {
					return fmt.Errorf("Movement area must stay inside the map")
				}
				*b.Arena = r
				return nil
			}
			out = append(out, f)
		}
		return append(out, overlayAction("Remove movement boundary", "Let this boss use ordinary movement rules.", func() { d.checkpoint(); b.Arena = nil }))
	case "attacks":
		lanes := []string{}
		for _, e := range a.Effects {
			if e.Kind == "lane" {
				lanes = append(lanes, e.ID)
			}
		}
		out := []overlayField{overlayHeading("Default pattern"), overlayInt("Lane attack every N actions", &b.EveryActions, "At least 2. A health phase can override this."), overlayList("Attack lanes", &b.Lanes, "Fired in turn while the boss's HP is above every health phase threshold.", lanes), overlayFloat("Attack range at or below 25% HP (tiles)", &b.LowHealthRange, "0 keeps the normal attack range."), overlayHeading("Health phases"), overlayAction("+ Add health phase", "Configure its threshold and attack volleys.", func() { v.overlayAddPhase(d) })}
		for i, phase := range b.Phases {
			f := overlayNavigate(fmt.Sprintf("At or below %d%% HP", phase.BelowPercent), fmt.Sprintf("%d attack volleys", len(phase.Patterns)), "Edit this phase's conditions and attack sequence.", func() { v.overlaySelect("phase", i) })
			f.depth = 1
			out = append(out, f)
		}
		return out
	}
	return nil
}

func (v *viewer) overlayMechanicFields(d *overlayDocument) []overlayField {
	i := v.overlay.selected
	if i < 0 || i >= len(d.state.Mechanics) {
		return nil
	}
	r := &d.state.Mechanics[i]
	help := "Critical hits double normal direct attack damage. Ground damage is unchanged."
	if r.Stat == "perfect_dodge" {
		help = "Chance to evade a direct hit. Attacks that ignore dodge still hit."
	}
	out := []overlayField{overlayNote(help), overlayHeading("Chance"), overlayInt("Base chance (%)", &r.Base, "Before any linked switches are used."), overlayInt("Minimum chance (%)", &r.Floor, "Reductions cannot take the chance below this value."), overlayHeading("Switch reductions")}
	keys := []string{}
	for key := range r.Reductions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := r.Reductions[key]
		f := overlayInt(key+" - points removed", &value, "Percentage points removed once. Set 0 to unlink.")
		f.depth = 1
		apply := f.apply
		f.apply = func(s string) error {
			if err := apply(s); err != nil {
				return err
			}
			if value == 0 {
				delete(r.Reductions, key)
			} else {
				r.Reductions[key] = value
			}
			return nil
		}
		out = append(out, f)
	}
	var ids []string
	for _, c := range d.state.Adventure.Controls {
		if _, used := r.Reductions[c.ID]; !used {
			ids = append(ids, c.ID)
		}
	}
	add := overlayAction("+ Link switch", "Choose a placed switch, then set its reduction.", func() {
		v.overlayChoose("Link switch", "Use + Switch above the map to place a new one.", ids, func(id string) error {
			d.checkpoint()
			if r.Reductions == nil {
				r.Reductions = map[string]int{}
			}
			r.Reductions[id] = max(1, r.Base-r.Floor)
			return nil
		})
	})
	add.disabled = len(ids) == 0
	out = append(out, add)
	if len(ids) == 0 {
		out = append(out, overlayNote("No unlinked switches. Place one using + Switch above the map."))
	}
	return append(out, overlayHeading("Remove mechanic"), overlayAction("Remove "+overlayMechanicName(r.Stat), "Restore this monster's default for this stat.", func() {
		d.checkpoint()
		d.state.Mechanics = slices.Delete(d.state.Mechanics, i, i+1)
		v.overlaySelect("mechanics", 0)
	}))
}
