package main

import (
	"image"
	"image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/game"
)

var overlayText = color.RGBA{226, 232, 240, 255}
var overlayMuted = color.RGBA{150, 163, 181, 255}
var overlayBorder = color.RGBA{63, 76, 94, 255}
var overlayInput = color.RGBA{19, 25, 35, 255}
var overlayButton = color.RGBA{42, 57, 70, 255}
var overlayDanger = color.RGBA{224, 147, 137, 255}
var overlayBossColor = color.RGBA{187, 144, 244, 255}

const overlayInspectorTop = 150
const overlayInspectorHeight = 554

func overlayHeading(label string) overlayField { return overlayField{label: label, kind: "heading"} }
func overlayNote(text string) overlayField     { return overlayField{value: text, kind: "note"} }
func overlayNavigate(label, summary, help string, action func()) overlayField {
	return overlayField{label: label, value: summary, help: help, kind: "navigation", action: action}
}
func (f overlayField) presentation() string {
	if f.kind != "" {
		return f.kind
	}
	if f.action != nil {
		return "button"
	}
	if f.apply != nil {
		return "input"
	}
	return "info"
}
func (f overlayField) interactive() bool { return !f.disabled && (f.action != nil || f.apply != nil) }

type overlayInspectorRow struct {
	index int
	r     rect
}

// One measured layout owns scrolling, painting, focus and mouse hit targets.
func overlayInspectorRows(fields []overlayField, scroll int) ([]overlayInspectorRow, int) {
	rows := make([]overlayInspectorRow, 0, len(fields))
	y := overlayInspectorTop
	for i, f := range fields {
		height := 60
		switch f.presentation() {
		case "heading":
			height = 36
		case "button", "toggle":
			height = 44
		case "navigation":
			height = 64
		case "note":
			height = len(wrapTooltipLines(f.value, game.ShadedTextColumns(322-f.depth*16)))*16 + 16
		case "info":
			height = 52
		}
		rows = append(rows, overlayInspectorRow{i, rect{844 + f.depth*16, y - scroll, 332 - f.depth*16, height}})
		y += height
	}
	return rows, max(0, y-overlayInspectorTop-overlayInspectorHeight)
}
func overlayInspectorHit(fields []overlayField, scroll, x, y int) int {
	if !pointInRect(x, y, 838, overlayInspectorTop, 350, overlayInspectorHeight) {
		return -1
	}
	rows, _ := overlayInspectorRows(fields, scroll)
	for _, row := range rows {
		if fields[row.index].interactive() && pointInRect(x, y, row.r.x, row.r.y, row.r.w, row.r.h-6) {
			return row.index
		}
	}
	return -1
}
func (v *viewer) overlayInspectorTitle(d *overlayDocument) (string, string) {
	crumb := "SCENE"
	titles := map[string]string{"overview": "Scene overview", "settings": "Map rules", "boss": "Boss encounter", "mechanics": "Mechanics", "regeneration": "Regeneration", "armor": "Armor reduction", "arena": "Movement area", "attacks": "Attack patterns", "phase": "Health phase", "effect": "Area trigger", "control": "Switch", "link": "Jump link", "object": "Interactive object", "choice": "Dialogue action", "special": "Special tile"}
	section := v.overlay.section
	title := titles[section]
	switch section {
	case "mechanics", "arena", "attacks":
		crumb = "SCENE / BOSS"
	case "regeneration", "armor", "mechanic":
		crumb = "BOSS / MECHANICS"
	case "phase":
		crumb = "BOSS / ATTACK PATTERNS"
	case "choice":
		crumb = "OBJECT / DIALOGUE"
	}
	if section == "mechanic" && v.overlay.selected >= 0 && v.overlay.selected < len(d.state.Mechanics) {
		title = overlayMechanicName(d.state.Mechanics[v.overlay.selected].Stat)
	}
	return crumb, title
}
func (v *viewer) drawOverlayInspector(screen *ebiten.Image, d *overlayDocument, mx, my int, help string) string {
	crumb, title := v.overlayInspectorTitle(d)
	game.DrawShadedText(screen, crumb, 844, 88, overlayMuted)
	game.DrawShadedText(screen, clipText(title, 330), 844, 111, overlayText)
	drawFilledRect(screen, 844, 138, 332, 1, overlayBorder)
	fields := v.overlayFields(d)
	_, maxScroll := overlayInspectorRows(fields, 0)
	v.overlay.fieldScroll = clampInt(v.overlay.fieldScroll, 0, maxScroll)
	rows, _ := overlayInspectorRows(fields, v.overlay.fieldScroll)
	clip := screen.SubImage(image.Rect(838, overlayInspectorTop, 1188, overlayInspectorTop+overlayInspectorHeight)).(*ebiten.Image)
	for _, row := range rows {
		f, r := fields[row.index], row.r
		if r.y+r.h <= overlayInspectorTop || r.y >= overlayInspectorTop+overlayInspectorHeight {
			continue
		}
		hover := my >= overlayInspectorTop && my < overlayInspectorTop+overlayInspectorHeight && pointInRect(mx, my, r.x, r.y, r.w, r.h-6)
		if hover && f.help != "" {
			help = f.help
		}
		fg, border := overlayText, overlayBorder
		if f.disabled {
			fg = overlayMuted
		}
		if f.interactive() && (hover || v.overlay.fieldFocus == row.index) {
			border = overlayAccent
		}
		switch f.presentation() {
		case "heading":
			game.DrawShadedText(clip, strings.ToUpper(f.label), r.x, r.y+12, overlayAccent)
		case "note":
			for i, line := range wrapTooltipLines(f.value, game.ShadedTextColumns(r.w-8)) {
				game.DrawShadedText(clip, line, r.x, r.y+4+i*16, overlayMuted)
			}
		case "button":
			drawFilledRect(clip, r.x, r.y, r.w, 36, overlayButton)
			drawRectBorder(clip, r.x, r.y, r.w, 36, 1, border)
			if strings.HasPrefix(f.label, "Remove") || strings.HasPrefix(f.label, "Delete") {
				fg = overlayDanger
			}
			game.DrawShadedText(clip, clipText(f.label, r.w-24), r.x+12, r.y+10, fg)
		case "navigation":
			drawFilledRect(clip, r.x, r.y, r.w, 56, overlayInput)
			if hover || v.overlay.fieldFocus == row.index {
				drawRectBorder(clip, r.x, r.y, r.w, 56, 1, border)
			}
			game.DrawShadedText(clip, clipText(f.label, r.w-32), r.x+12, r.y+9, fg)
			game.DrawShadedText(clip, clipText(f.value, r.w-32), r.x+12, r.y+31, overlayMuted)
			game.DrawShadedText(clip, ">", r.x+r.w-18, r.y+19, overlayAccent)
		case "toggle":
			drawFilledRect(clip, r.x, r.y+8, 20, 20, overlayInput)
			drawRectBorder(clip, r.x, r.y+8, 20, 20, 1, border)
			if f.value == "Yes" {
				game.DrawShadedText(clip, "x", r.x+5, r.y+10, overlayAccent)
			}
			game.DrawShadedText(clip, clipText(f.label, r.w-36), r.x+32, r.y+11, fg)
		case "info":
			game.DrawShadedText(clip, clipText(f.label, r.w), r.x, r.y+2, overlayMuted)
			game.DrawShadedText(clip, clipText(f.value, r.w), r.x, r.y+24, fg)
		default:
			game.DrawShadedText(clip, clipText(f.label, r.w), r.x, r.y+2, overlayMuted)
			drawFilledRect(clip, r.x, r.y+20, r.w, 32, overlayInput)
			drawRectBorder(clip, r.x, r.y+20, r.w, 32, 1, border)
			value := f.value
			if value == "" {
				value = "None"
			}
			game.DrawShadedText(clip, clipText(value, r.w-58), r.x+10, r.y+29, fg)
			mark := "Edit"
			if len(f.choices) > 0 || f.multi {
				mark = "v"
			}
			game.DrawShadedText(clip, mark, r.x+r.w-40, r.y+29, overlayMuted)
		}
	}
	if maxScroll > 0 {
		h := max(24, overlayInspectorHeight*overlayInspectorHeight/(maxScroll+overlayInspectorHeight))
		y := overlayInspectorTop + (overlayInspectorHeight-h)*v.overlay.fieldScroll/maxScroll
		drawFilledRect(screen, 1184, y, 3, h, overlayAccent)
	}
	return help
}
