package main

import (
	"fmt"
	"image/color"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/game"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

func overlayKey(k ebiten.Key) bool {
	n := inpututil.KeyPressDuration(k)
	return n == 1 || n > 24 && n%3 == 0
}
func (m *overlayModal) options() []string {
	var out []string
	for _, choice := range m.choices {
		if strings.Contains(strings.ToLower(choice), strings.ToLower(m.text)) {
			out = append(out, choice)
		}
	}
	return out
}
func (v *viewer) updateOverlayModal() {
	m := v.overlay.modal
	if m == nil {
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		v.overlay.modal = nil
		return
	}
	apply := func(value string) {
		if err := m.apply(value); err != nil {
			m.help = err.Error()
			return
		}
		v.overlay.modal = nil
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) && !m.confirm {
		value := m.text
		if len(m.choices) > 0 {
			if m.multi {
				value = strings.Join(m.selected, ", ")
			} else {
				opts := m.options()
				if len(opts) != 1 {
					m.help = "Select an option from the list."
					return
				}
				value = opts[0]
			}
		}
		apply(value)
		return
	}
	if !m.confirm {
		ctrl := ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta)
		if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyA) {
			m.all = true
		}
		if overlayKey(ebiten.KeyBackspace) {
			if m.all {
				m.text = ""
				m.cursor = 0
				m.all = false
			} else if m.cursor > 0 {
				m.text = m.text[:m.cursor-1] + m.text[m.cursor:]
				m.cursor--
			}
		}
		if overlayKey(ebiten.KeyDelete) {
			if m.all {
				m.text = ""
				m.cursor = 0
				m.all = false
			} else if m.cursor < len(m.text) {
				m.text = m.text[:m.cursor] + m.text[m.cursor+1:]
			}
		}
		if overlayKey(ebiten.KeyLeft) {
			m.cursor = max(0, m.cursor-1)
			m.all = false
		}
		if overlayKey(ebiten.KeyRight) {
			m.cursor = min(len(m.text), m.cursor+1)
			m.all = false
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyHome) {
			m.cursor = 0
			m.all = false
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEnd) {
			m.cursor = len(m.text)
			m.all = false
		}
		if !ctrl {
			for _, r := range ebiten.AppendInputChars(nil) {
				if r >= 32 && r <= 126 {
					if m.all {
						m.text = ""
						m.cursor = 0
						m.all = false
					}
					m.text = m.text[:m.cursor] + string(r) + m.text[m.cursor:]
					m.cursor++
					m.scroll = 0
				}
			}
		}
	}
	mx, my := ebiten.CursorPosition()
	ox, oy := v.overlayModalOffset()
	mx, my = mx-ox, my-oy // hit tests below use the authored modal rects
	_, wheel := ebiten.Wheel()
	opts := m.options()
	if len(m.choices) > 0 {
		m.scroll = clampInt(m.scroll-int(wheel*3), 0, max(0, len(opts)-12))
	}
	if !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		return
	}
	if pointInRect(mx, my, 680, 655, 120, 32) {
		v.overlay.modal = nil
		return
	}
	if pointInRect(mx, my, 400, 655, 260, 32) {
		value := m.text
		if m.multi {
			value = strings.Join(m.selected, ", ")
		}
		if len(m.choices) > 0 && !m.multi && !m.confirm {
			m.help = "Select an option from the list."
			return
		}
		apply(value)
		return
	}
	if len(m.choices) > 0 && pointInRect(mx, my, 340, 272, 520, 360) {
		i := (my-272)/30 + m.scroll
		if i >= len(opts) {
			return
		}
		value := opts[i]
		if !m.multi {
			apply(value)
			return
		}
		idx := -1
		for j, s := range m.selected {
			if s == value {
				idx = j
				break
			}
		}
		if idx >= 0 {
			m.selected = append(m.selected[:idx], m.selected[idx+1:]...)
		} else {
			m.selected = append(m.selected, value)
		}
	}
}

// overlayModalOffset centers the modal, authored for the 1200x800 canvas.
func (v *viewer) overlayModalOffset() (int, int) {
	w, h := v.canvasSize()
	return (w - windowWidth) / 2, (h - windowHeight) / 2
}
func (v *viewer) drawOverlayModal(screen *ebiten.Image) {
	m := v.overlay.modal
	if m == nil {
		return
	}
	ox, oy := v.overlayModalOffset()
	drawFilledRect(screen, 0, pageBarHeight, screen.Bounds().Dx(), screen.Bounds().Dy()-pageBarHeight, color.RGBA{5, 8, 16, 230})
	drawFilledRect(screen, ox+320, oy+140, 560, 570, overlayBG)
	drawRectBorder(screen, ox+320, oy+140, 560, 570, 2, overlayAccent)
	game.DrawShadedText(screen, clipText(m.title, 520), ox+340, oy+160, overlayAccent)
	for i, line := range wrapTooltipLines(m.help, game.ShadedTextColumns(520)) {
		if i >= 3 {
			break
		}
		game.DrawPlainText(screen, line, ox+340, oy+186+i*17)
	}
	if !m.confirm {
		drawFilledRect(screen, ox+340, oy+240, 520, 28, color.RGBA{48, 59, 77, 255})
		text := m.text
		start := 0
		for start < m.cursor && game.ShadedTextWidth(m.text[start:m.cursor]) > 490 {
			start++
		}
		text = text[start:]
		if m.all && text != "" {
			drawFilledRect(screen, ox+346, oy+243, min(508, game.ShadedTextWidth(text)+8), 22, color.RGBA{57, 93, 117, 255})
		}
		if text == "" && len(m.choices) > 0 {
			text = "Type to filter..."
		}
		game.DrawPlainText(screen, clipText(text, 500), ox+350, oy+247)
		if len(m.choices) == 0 && m.cursor <= len(m.text) {
			cx := 350 + game.ShadedTextWidth(m.text[start:m.cursor])
			if cx < 850 {
				drawFilledRect(screen, ox+cx, oy+244, 1, 18, overlayAccent)
			}
		}
		opts := m.options()
		if len(m.choices) > 0 {
			for i := m.scroll; i < min(len(opts), m.scroll+12); i++ {
				y := 272 + (i-m.scroll)*30
				label := opts[i]
				if label == "" {
					label = "(none)"
				}
				if m.multi {
					selected := false
					for _, s := range m.selected {
						selected = selected || s == opts[i]
					}
					if selected {
						label = "[x] " + label
					} else {
						label = "[ ] " + label
					}
				}
				drawFilledRect(screen, ox+340, oy+y, 520, 28, color.RGBA{35, 43, 57, 255})
				game.DrawPlainText(screen, clipText(label, 504), ox+348, oy+y+7)
			}
			if len(opts) == 0 {
				game.DrawPlainText(screen, "No matching options.", ox+348, oy+285)
			}
		} else {
			game.DrawPlainText(screen, "Enter: apply   Escape: cancel   Ctrl+A: select all", ox+340, oy+284)
		}
	}
	applyText := "Apply"
	if m.confirm {
		applyText = "Confirm"
	}
	for _, button := range []struct {
		x, w int
		text string
	}{{400, 260, applyText}, {680, 120, "Cancel"}} {
		drawFilledRect(screen, ox+button.x, oy+655, button.w, 32, color.RGBA{43, 88, 82, 255})
		game.DrawPlainText(screen, button.text, ox+button.x+12, oy+664)
	}
}
func (v *viewer) reloadOverlay(key string) {
	oldCatalog := character.NPCConfigInstance
	published := false
	defer func() {
		if !published {
			character.NPCConfigInstance = oldCatalog
		}
	}()
	fail := func(err error) {
		if d := v.overlay.documents[key]; d != nil {
			d.err = err.Error()
		}
	}
	if err := character.LoadNPCConfig(overlayNPCPath); err != nil {
		fail(err)
		return
	}
	maps, err := loadMaps(v.cfg)
	if err != nil {
		fail(err)
		return
	}
	for _, m := range maps {
		if m.Key != key {
			continue
		}
		if m.Err != nil {
			fail(m.Err)
			return
		}
		// Stage all files before replacing either editor geometry or history.
		staged := *v
		staged.maps = []mapInfo{m}
		staged.mapIndex = 0
		staged.overlay.documents = nil
		next := staged.overlayDoc()
		if next.err != "" {
			fail(fmt.Errorf("Reload failed: %s", next.err))
			return
		}
		v.maps[v.mapIndex] = m
		v.overlay.documents[key] = next
		v.overlaySelect("settings", 0)
		v.refreshLegend()
		published = true
		return
	}
	fail(fmt.Errorf("Map %s no longer exists", key))
}
