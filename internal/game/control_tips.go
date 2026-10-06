package game

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// The Control Tips page of the ESC menu: every binding, grouped, with the
// pointer combos. Keep it in step with input.go when a binding changes.

type controlTip struct {
	keys, effect string
}

type controlTipGroup struct {
	title string
	tips  []controlTip
}

var controlTipGroups = []controlTipGroup{
	{"Movement", []controlTip{
		{"W / S, Up / Down", "Walk forward / back"},
		{"A / D, Left / Right", "Turn"},
		{"Q / E", "Strafe left / right"},
		{"Shift (hold)", "Run. No attacks or casts while running, unless a hero has Grandmaster Pathfinding or Wyrmspine Wing"},
		{"Tab", "Switch real-time / turn-based"},
	}},
	{"Combat", []controlTip{
		{"R", "Weapon attack"},
		{"Space", "Smart action: heal an ally below 60% HP if the hero knows a heal, else the slotted attack spell, a slotted trap, then the weapon"},
		{"F", "Use the slotted spell, trap, technique or flask"},
		{"C (or H)", "Cast the hero's best heal"},
		{"Hold R / Space / F / C", "The party acts in turn: each ready hero who can do it"},
		{"Click a monster", "Smart action on it; hold to keep attacking"},
		{"1 - 4", "Select a hero"},
		{"Shift + right-click card", "Focus that hero: only focused heroes act (real-time)"},
	}},
	{"Healing a chosen hero", []controlTip{
		{"Point at card + F", "The slotted Heal lands on that hero (First Aid always heals the caster)"},
		{"Point at card + C", "The hero's best heal lands on that hero"},
		{"No card pointed at", "F and C heal the selected hero"},
		{"From the spellbook", "Heal Other lands on the most hurt ally"},
		{"Space", "Heals the most hurt ally by itself"},
	}},
	{"Turn-based", []controlTip{
		{"W / S / Q / E", "Step one tile - ends the party's turn"},
		{"A / D", "Turn 90 degrees - free"},
		{"R / Space / F / C", "Spend an action of the next hero who has one"},
	}},
	{"Exploring", []controlTip{
		{"Walk", "Loot bags on and next to your tile are picked up"},
		{"Space", "Pick up loot or talk to the NPC in front; hold to gather every bag nearby"},
		{"Click", "Open loot, doors and chests, talk to NPCs, use objects within 2 tiles"},
		{"V / Shift + V", "Wayfarer: Fold Step / Return Step"},
		{"Camp icon", "Rest for 1 food; not with enemies near"},
	}},
	{"Panels", []controlTip{
		{"I / P / M / J / K", "Inventory, Characters, Spellbook, Quests, Cards; the same key closes"},
		{"Esc", "Close the panel, or open this menu"},
		{"Double-click item", "Equip or use it on the selected hero; on the paperdoll, unequip"},
		{"Right-click item", "Discard, split a stack, move between bags"},
		{"Drag", "Equip, reorder, trade bags, fill a quick slot; Shift-drag takes one from a stack"},
		{"Quick slot", "Double-click to use; right-click makes it the F / Space action"},
		{"Shift over a tooltip", "Full breakdown; over a card, its art"},
		{"Double-click log", "Open the combat log"},
		{"Double-click buff", "Dispel it"},
	}},
	{"Saving", []controlTip{
		{"F5", "Quicksave"},
		{"Shift + F4", "Load the quicksave"},
		{"Esc > Save / Load", "Save slots; the Autosave is written on every map change"},
	}},
}

const (
	controlTipsMaxW      = 1120
	controlTipsMaxH      = 760
	controlTipsMargin    = 20
	controlTipsInset     = 24
	controlTipsTitleH    = 56 // title band above the list
	controlTipsFooterH   = 56 // back button band below it
	controlTipsLineH     = 16
	controlTipsGroupGap  = 10
	controlTipsColumnGap = 28
	controlTipsMinColumn = 460
	controlTipsKeyGap    = 12
)

// controlTipsPanelSize grows the page with the screen, up to a size where two
// columns hold every group.
func controlTipsPanelSize(screenW, screenH int) (int, int) {
	return min(controlTipsMaxW, screenW-2*controlTipsMargin), min(controlTipsMaxH, screenH-2*controlTipsMargin)
}

// controlTipRow is one laid-out tip: its key text and wrapped effect lines.
type controlTipRow struct {
	keys    string
	effect  []string
	x, y, h int // content coordinates (y before scrolling)
}

type controlTipHeading struct {
	title string
	x, y  int
}

type controlTipsLayout struct {
	body     layoutRect // the scrolled viewport
	track    layoutRect // scrollbar
	keyW     int
	contentH int
	headings []controlTipHeading
	rows     []controlTipRow
}

func (l controlTipsLayout) maxScroll() int { return max(0, l.contentH-l.body.h) }

// controlTipsBackRect is the page's Back button in the footer band.
func controlTipsBackRect(panel layoutRect) layoutRect {
	return layoutRect{panel.x + controlTipsInset, panel.bottom() - 46, menuBackButtonW, menuBackButtonH}
}

// computeControlTipsLayout flows the groups into as many columns as fit, each
// group kept whole, filling a column before starting the next.
func computeControlTipsLayout(panel layoutRect) controlTipsLayout {
	body := layoutRect{panel.x + controlTipsInset, panel.y + controlTipsTitleH, panel.w - 2*controlTipsInset - scrollbarGrabRight - 4, panel.h - controlTipsTitleH - controlTipsFooterH}
	l := controlTipsLayout{body: body, track: layoutRect{body.right() + scrollbarGrabLeft + 1, body.y, 3, body.h}}
	cols := max(1, (body.w+controlTipsColumnGap)/(controlTipsMinColumn+controlTipsColumnGap))
	colW := (body.w - (cols-1)*controlTipsColumnGap) / cols
	for _, g := range controlTipGroups {
		for _, tip := range g.tips {
			l.keyW = max(l.keyW, uiTextWidth(tip.keys))
		}
	}
	l.keyW = min(l.keyW, colW*2/5)
	effectW := colW - l.keyW - controlTipsKeyGap

	heights := make([]int, len(controlTipGroups))
	total := 0
	for i, g := range controlTipGroups {
		heights[i] = controlTipsLineH + 4
		for _, tip := range g.tips {
			heights[i] += len(wrapUIText(tip.effect, effectW)) * controlTipsLineH
		}
		heights[i] += controlTipsGroupGap
		total += heights[i]
	}
	// Columns hold an equal share, and never less than the tallest group or
	// the viewport, so a short page stays in the first column.
	target := max((total+cols-1)/cols, body.h)
	col, y := 0, 0
	for i, g := range controlTipGroups {
		if y > 0 && y+heights[i] > target && col < cols-1 {
			col, y = col+1, 0
		}
		x := body.x + col*(colW+controlTipsColumnGap)
		l.headings = append(l.headings, controlTipHeading{g.title, x, y})
		y += controlTipsLineH + 4
		for _, tip := range g.tips {
			lines := wrapUIText(tip.effect, effectW)
			l.rows = append(l.rows, controlTipRow{tip.keys, lines, x, y, len(lines) * controlTipsLineH})
			y += len(lines) * controlTipsLineH
		}
		y += controlTipsGroupGap
		l.contentH = max(l.contentH, y)
	}
	return l
}

var (
	controlTipHeadingColor = color.RGBA{220, 190, 110, 255}
	controlTipKeyColor     = color.RGBA{140, 200, 255, 255}
	controlTipEffectColor  = color.RGBA{222, 222, 230, 255}
)

// drawControlTips draws the grouped tips scrolled by the menu's offset, with a
// wheel and a draggable scrollbar when they overflow the page.
func (ui *UISystem) drawControlTips(screen *ebiten.Image, panel layoutRect) {
	g := ui.game
	l := computeControlTipsLayout(panel)
	g.controlTipsScroll = min(max(0, g.controlTipsScroll), l.maxScroll())
	scroll := g.controlTipsScroll
	dst := uiClip(screen, image.Rect(l.body.x, l.body.y, l.body.right(), l.body.bottom()))
	top := l.body.y - scroll
	for _, h := range l.headings {
		drawUITextColored(dst, h.title, h.x, top+h.y, controlTipHeadingColor)
	}
	for _, r := range l.rows {
		if top+r.y+r.h < l.body.y || top+r.y > l.body.bottom() {
			continue
		}
		drawUITextColored(dst, clipUIText(r.keys, l.keyW), r.x, top+r.y, controlTipKeyColor)
		for i, line := range r.effect {
			drawUITextColored(dst, line, r.x+l.keyW+controlTipsKeyGap, top+r.y+i*controlTipsLineH, controlTipEffectColor)
		}
	}
	if l.maxScroll() == 0 {
		return
	}
	ui.drawScrollbar(screen, "control_tips", l.track, scroll, l.contentH, true, func(v int) { g.controlTipsScroll = v })
	ui.onDisplayedInput(uiCommandPointer, l.body, func() {
		_, wheel := pointerWheel()
		mx, my := pointerPosition()
		if wheel == 0 || !isMouseHoveringBox(mx, my, l.body.x, l.body.y, l.body.right(), l.body.bottom()) {
			return
		}
		g.controlTipsScroll = min(l.maxScroll(), max(0, g.controlTipsScroll-int(wheel*3*controlTipsLineH)))
	})
}
