package main

import (
	"fmt"
	"strings"
)

func (v *viewer) overlayTree(d *overlayDocument) ([]overlayWidget, int) {
	p := &v.overlay
	var rows []overlayWidget
	track := v.overlayGeometry().treeTrack
	y := track.y
	bottom := track.y + track.h
	add := func(label, kind string, index, depth int) {
		r := rect{12 + depth*16, y - p.treeScroll, 214 - depth*16, 28}
		if r.y >= 80 && r.y+r.h <= bottom {
			rows = append(rows, overlayWidget{r: r, text: label, kind: "tree", active: p.section == kind && p.selected == index, help: "Select to edit. Indented rows belong to their parent.", action: func() { v.overlaySelect(kind, index) }})
		}
		y += 32
	}
	group := func(key, label string, count int) bool {
		closed := p.collapsed[key]
		mark := "v "
		if closed {
			mark = "> "
		}
		r := rect{12, y - p.treeScroll, 214, 32}
		if r.y >= 80 && r.y+r.h <= bottom {
			rows = append(rows, overlayWidget{r: r, kind: "heading", text: fmt.Sprintf("%s%s (%d)", mark, strings.ToUpper(label), count), help: "Expand or collapse this group.", action: func() {
				if p.collapsed == nil {
					p.collapsed = map[string]bool{}
				}
				p.collapsed[key] = !p.collapsed[key]
				p.treeScroll = 0
			}})
		}
		y += 36
		return !closed
	}
	add("Scene overview", "overview", 0, 0)
	add("Map rules", "settings", 0, 0)
	if b := d.state.Adventure.Boss; b != nil {
		if group("boss", "Boss encounter", 1) {
			name := b.Monster
			if v.monsterCfg != nil {
				if def, ok := v.monsterCfg.Monsters[b.Monster]; ok {
					name = def.Name
				}
			}
			add(name, "boss", 0, 1)
			add("Movement area", "arena", 0, 2)
			add("Mechanics", "mechanics", 0, 2)
			add("Regeneration", "regeneration", 0, 3)
			add("Armor reduction", "armor", 0, 3)
			for i, m := range d.state.Mechanics {
				add(overlayMechanicName(m.Stat), "mechanic", i, 3)
			}
			add("Attack patterns", "attacks", 0, 2)
			for i, phase := range b.Phases {
				add(fmt.Sprintf("HP <= %d%%", phase.BelowPercent), "phase", i, 3)
			}
		}
	} else {
		add("+ Set up boss", "boss", 0, 0)
	}
	if group("zones", "Area triggers", len(d.state.Adventure.Effects)) {
		for i, e := range d.state.Adventure.Effects {
			add(e.Name, "effect", i, 1)
		}
	}
	if group("switches", "Switches", len(d.state.Adventure.Controls)) {
		for i, c := range d.state.Adventure.Controls {
			add(c.ID, "control", i, 1)
		}
	}
	if group("objects", "Objects", len(d.state.Data.NPCSpawns)) {
		for i, n := range d.state.Data.NPCSpawns {
			name := n.NPCKey
			if def := d.npc(n.NPCKey); def != nil {
				name = def.Name
			}
			add(name, "object", i, 1)
		}
	}
	if len(d.state.Data.SpecialTileSpawns) > 0 && group("tiles", "Special tiles", len(d.state.Data.SpecialTileSpawns)) {
		for i, tile := range d.state.Data.SpecialTileSpawns {
			add(tile.TileKey, "special", i, 1)
		}
	}
	if len(d.state.Adventure.JumpLinks) > 0 && group("links", "Jump links", len(d.state.Adventure.JumpLinks)) {
		for i := range d.state.Adventure.JumpLinks {
			add(fmt.Sprintf("Jump link %d", i+1), "link", i, 1)
		}
	}
	return rows, max(0, y-bottom)
}

func (v *viewer) overlayFocusNext(d *overlayDocument, reverse bool) {
	fields := v.overlayFields(d)
	step := 1
	if reverse {
		step = -1
	}
	index := v.overlay.fieldFocus
	if index < 0 && reverse {
		index = 0
	}
	for range fields {
		index = (index + step + len(fields)) % len(fields)
		if !fields[index].interactive() {
			continue
		}
		v.overlay.fieldFocus = index
		rows, _ := v.overlayInspectorRows(fields, v.overlay.fieldScroll)
		r := rows[index].r
		view := v.overlayGeometry().fields
		if r.y < view.y {
			v.overlay.fieldScroll -= view.y - r.y
		}
		if end := view.y + view.h; r.y+r.h > end {
			v.overlay.fieldScroll += r.y + r.h - end
		}
		return
	}
}
