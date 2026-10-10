package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"image/color"
	"strings"
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
)

const guildLedgerRowHeight = 128

var guildInk = struct{ row, alternate, line, ready color.RGBA }{
	color.RGBA{27, 26, 23, 255}, color.RGBA{23, 23, 21, 255},
	color.RGBA{65, 57, 43, 255}, color.RGBA{139, 198, 158, 255},
}

type guildLedgerLayout struct{ panel, list, track, footer layoutRect }

func makeGuildLedgerLayout(g *MMGame) guildLedgerLayout {
	d := npcDialogLayout(g)
	p := layoutRect{d.x, d.y, d.w, d.h}
	list := layoutRect{p.x + 24, p.y + 132, p.w - 64, p.h - 188}
	return guildLedgerLayout{p, list, layoutRect{list.right() + 12, list.y, 4, list.h}, layoutRect{p.x + 28, p.bottom() - 40, p.w - 56, 24}}
}

func (g *MMGame) setGuildScroll(offset int) {
	l := makeGuildLedgerLayout(g)
	g.guildScroll = max(0, min(offset, len(g.guildLedger[g.dialogTab])*guildLedgerRowHeight-l.list.h))
}

func (ih *InputHandler) handleGuildInput() {
	g := ih.game
	if !g.party.ThievesGuildMember {
		if ih.keys.Consume(ebiten.KeyEnter) {
			g.joinThievesGuild()
		}
		return
	}
	if ih.keys.Consume(ebiten.KeyTab) {
		g.switchDialogTab((g.dialogTab + 1) % len(g.guildLedger))
	}
	delta := 0
	if ih.keys.Consume(ebiten.KeyArrowUp) {
		delta -= guildLedgerRowHeight / 2
	}
	if ih.keys.Consume(ebiten.KeyArrowDown) {
		delta += guildLedgerRowHeight / 2
	}
	if ih.keys.Consume(ebiten.KeyPageUp) {
		delta -= makeGuildLedgerLayout(g).list.h
	}
	if ih.keys.Consume(ebiten.KeyPageDown) {
		delta += makeGuildLedgerLayout(g).list.h
	}
	if ih.keys.Consume(ebiten.KeyHome) {
		g.setGuildScroll(0)
	}
	if ih.keys.Consume(ebiten.KeyEnd) {
		g.setGuildScroll(len(g.guildLedger[g.dialogTab]) * guildLedgerRowHeight)
	}
	if delta != 0 {
		g.setGuildScroll(g.guildScroll + delta)
	}
}

func guildMiniature(a character.GuildLedgerArt, kind guildReportArt) string {
	switch kind {
	case guildReportTown:
		return a.Town
	case guildReportDungeon:
		return a.Dungeon
	case guildReportCreature:
		return a.Creature
	case guildReportWildlife:
		return a.Wildlife
	case guildReportCaravan:
		return a.Caravan
	default:
		return a.Merchant
	}
}

func (ui *UISystem) drawGuildMiniature(dst *ebiten.Image, name string, r layoutRect) {
	if ui.game.sprites == nil || name == "" {
		return
	}
	if sprite := ui.game.sprites.GetSprite(name); sprite != nil {
		drawImageScaled(dst, sprite, r.x, r.y, r.w, r.h)
	}
}

func (ui *UISystem) drawGuildButton(dst *ebiten.Image, r layoutRect, label string, enabled bool, action func()) {
	g := ui.game
	live := ui.topModalLayer() == modalLayerDialog
	mx, my := uiCursorPosition()
	ui.drawButtonFrame(dst, r.x, r.y, r.w, r.h, live && enabled && isMouseHoveringBox(mx, my, r.x, r.y, r.right(), r.bottom()))
	drawCenteredUIText(dst, label, r.x, r.y, r.w, r.h)
	if !enabled {
		drawFilledRect(dst, r.x+1, r.y+1, r.w-2, r.h-2, color.RGBA{0, 0, 0, 100})
	}
	ui.onDisplayedInput(uiCommandClick, r, func() {
		if live && enabled && g.consumeLeftClickIn(r.x, r.y, r.right(), r.bottom()) {
			action()
		}
	})
}

func (ui *UISystem) drawThievesGuildDialog(dst *ebiten.Image, panel layoutRect) {
	g := ui.game
	n := g.dialogNPC
	if n == nil || n.Guild == nil {
		return
	}
	l := makeGuildLedgerLayout(g)
	live := ui.topModalLayer() == modalLayerDialog
	x, y := panel.x+28, panel.y+24
	ui.drawGuildMiniature(dst, n.Guild.Art.Town, layoutRect{x, y - 8, 64, 64})
	drawUITextColored(dst, n.Name, x+80, y, gameplayInk.heading)
	drawUITextColored(dst, uitext.Text("guild.subtitle"), x+80, y+24, gameplayInk.muted)
	if !g.party.ThievesGuildMember {
		ui.drawGuildMiniature(dst, n.Guild.Art.Merchant, layoutRect{x + 8, y + 104, 176, 176})
		tx, ty, width := x+208, y+100, panel.w-264
		for _, line := range wrapUIText(n.DialogueData.Greeting, width) {
			drawUITextColored(dst, line, tx, ty, gameplayInk.text)
			ty += dialogueLineHeight
		}
		ty += 24
		for _, line := range wrapUIText(uitext.Text("guild.requirement", g.party.NonKeyDoorsOpened, n.Guild.NonKeyDoorsOpened), width) {
			drawUITextColored(dst, line, tx, ty, gameplayInk.heading)
			ty += dialogueLineHeight
		}
		drawUITextColored(dst, uitext.Text("guild.balance", g.party.Gold), tx, ty+12, gameplayInk.text)
		ui.drawGuildButton(dst, layoutRect{tx, panel.bottom() - 72, width, 36}, uitext.Text("guild.join", n.Guild.MembershipGold), g.guildJoinRefusal(n) == "", g.joinThievesGuild)
		return
	}
	labels := []string{uitext.Text("guild.shops"), uitext.Text("guild.dungeons"), uitext.Text("guild.creatures")}
	ui.drawFolderTabs(dst, labels, live, func(i int) layoutRect {
		return layoutRect{panel.x + 24 + i*(panel.w-48)/3, panel.y + 84, (panel.w-48)/3 - 4, 32}
	})
	rows := g.guildLedger[g.dialogTab]
	contentH := len(rows) * guildLedgerRowHeight
	offset := max(0, min(g.guildScroll, contentH-l.list.h))
	if live {
		ui.onDisplayedInput(uiCommandPointer, l.list, func() {
			mx, my := pointerPosition()
			if !isMouseHoveringBox(mx, my, l.list.x, l.list.y, l.list.right(), l.list.bottom()) {
				return
			}
			_, wheel := pointerWheel()
			delta := int(wheel * 48)
			if delta == 0 && wheel != 0 {
				if wheel > 0 {
					delta = 1
				} else {
					delta = -1
				}
			}
			if delta != 0 {
				g.setGuildScroll(offset - delta)
			}
		})
	}
	clip := image.Rect(l.list.x, l.list.y, l.list.right(), l.list.bottom()).Intersect(uiBounds(dst))
	if !clip.Empty() {
		listDst := uiClip(dst, clip)
		if len(rows) == 0 {
			drawCenteredUIText(listDst, uitext.Text("guild.empty"), l.list.x, l.list.y, l.list.w, l.list.h)
		}
		for i := offset / guildLedgerRowHeight; i < len(rows); i++ {
			ry := l.list.y + i*guildLedgerRowHeight - offset
			if ry >= l.list.bottom() {
				break
			}
			row := rows[i]
			r := layoutRect{l.list.x, ry, l.list.w, guildLedgerRowHeight - 8}
			fill := guildInk.row
			if i%2 != 0 {
				fill = guildInk.alternate
			}
			drawFilledRect(listDst, r.x, r.y, r.w, r.h, fill)
			drawFilledRect(listDst, r.x, r.bottom()-1, r.w, 1, guildInk.line)
			ui.drawGuildMiniature(listDst, guildMiniature(n.Guild.Art, row.art), layoutRect{r.x + 8, r.y + 18, 80, 80})
			tx, tw := r.x+104, r.w-120
			statusW := min(208, tw/2)
			for j, line := range wrapUIText(row.title, tw-statusW-16) {
				if j == 2 {
					break
				}
				drawUITextColored(listDst, line, tx, r.y+12+j*20, gameplayInk.heading)
			}
			statusInk := gameplayInk.heading
			if row.ready {
				statusInk = guildInk.ready
			}
			status := clipUIText(row.status, statusW-16)
			sx := r.right() - statusW - 8
			drawFilledRect(listDst, sx, r.y+12, statusW, 28, interfacePanelFill)
			drawUITextColored(listDst, status, sx+(statusW-uiTextWidth(status))/2, r.y+18, statusInk)
			if row.location != "" {
				drawUITextColored(listDst, clipUIText(row.location, tw), tx, r.y+54, gameplayInk.muted)
			}
			for j, line := range wrapUIText(strings.Join(row.details, ". "), tw) {
				if j == 2 {
					break
				}
				drawUITextColored(listDst, line, tx, r.y+76+j*20, gameplayInk.text)
			}
			mx, my := uiCursorPosition()
			if live && isMouseHoveringBox(mx, my, r.x, max(r.y, l.list.y), r.right(), min(r.bottom(), l.list.bottom())) {
				ui.queueTooltip(append([]string{row.title, row.location, row.status}, row.details...), mx+12, my+8)
			}
		}
	}
	ui.drawScrollbar(dst, "guild-ledger", l.track, offset, contentH, live, g.setGuildScroll)
	drawUITextColored(dst, clipUIText(uitext.Text("guild.footer", len(rows)), l.footer.w-104), l.footer.x, l.footer.y+4, gameplayInk.muted)
	ui.drawGuildButton(dst, layoutRect{l.footer.right() - 96, l.footer.y, 96, 24}, uitext.Text("guild.refresh"), true, g.refreshGuildLedger)
}
