package game

import (
	"fmt"
	"image/color"

	"ugataima/internal/character"
	"ugataima/internal/spells"

	"github.com/hajimehoshi/ebiten/v2"
)

// Buff-service dialog: an NPC who CASTS party buffs for gold (Mira's water
// charms) instead of teaching them. Service entries are the NPC's top-level
// cast_buff choices, shown as icon rows with price and span; a second folder
// tab carries the NPC's ordinary quest dialogue, reusing the spell trader's
// tab machinery so both dialogs behave identically.

const (
	buffServiceRowH     = 56
	buffServiceIconSize = 44
	buffServiceIconGap  = 12
	buffServiceRowGap   = 10
	buffServiceTopGap   = 30 // below the greeting/balance block
)

// buffServiceChoicesFromDialogue lists paid casts in authored order. Top level
// only: a service is a shop shelf, not something buried in a conversation.
func buffServiceChoicesFromDialogue(dialogue *character.NPCDialogue) []*character.NPCDialogueChoice {
	if dialogue == nil {
		return nil
	}
	var out []*character.NPCDialogueChoice
	for _, c := range dialogue.Choices {
		if c != nil && c.Action == "cast_buff" {
			out = append(out, c)
		}
	}
	return out
}

func buffServiceChoices(npc *character.NPC) []*character.NPCDialogueChoice {
	if npc == nil {
		return nil
	}
	return buffServiceChoicesFromDialogue(npc.DialogueData)
}

// npcHasBuffService reports whether the NPC sells timed party buffs as a
// service - the capability behind dialogKindBuffService.
func npcHasBuffService(npc *character.NPC) bool {
	return len(buffServiceChoices(npc)) > 0
}

// buffServiceHasQuestTab reports whether the NPC also has ordinary dialogue
// worth a second tab (every choice that is NOT a service row).
func buffServiceHasQuestTab(npc *character.NPC) bool {
	if npc == nil || npc.DialogueData == nil {
		return false
	}
	for _, c := range npc.DialogueData.Choices {
		if c != nil && c.Action != "cast_buff" {
			return true
		}
	}
	return false
}

// buffServiceRowRect is the clickable row for service entry i. Shared by the
// renderer and its click handling so the drawn row IS the hitbox.
func buffServiceRowRect(dialogX, dialogY, dialogWidth, i int) (x, y, w, h int) {
	layout := computeNPCDialogSectionLayout(layoutRect{dialogX, dialogY, dialogWidth, npcDialogHeight}, true)
	top := layout.greeting.bottom() + buffServiceTopGap
	return dialogX + 24, top + i*(buffServiceRowH+buffServiceRowGap), dialogWidth - 48, buffServiceRowH
}

// buffServiceMaxRows is how many service rows fit between the greeting and the
// footer line. Measured rather than assumed: rows are tall, and a catalog that
// grows would otherwise walk straight over the footer text.
func buffServiceMaxRows(dialogX, dialogY, dialogWidth, dialogHeight int) int {
	layout := computeNPCDialogSectionLayout(layoutRect{dialogX, dialogY, dialogWidth, dialogHeight}, true)
	top := layout.greeting.bottom() + buffServiceTopGap
	avail := layout.footer[0].y - top - 8
	rows := (avail + buffServiceRowGap) / (buffServiceRowH + buffServiceRowGap)
	if rows < 1 {
		rows = 1
	}
	return rows
}

// buffIconName resolves the icon for a party buff: its spell icon when one
// exists (both water charms have one), else the HUD status sprite.
func (g *MMGame) buffIconName(buff string) string {
	if g.sprites == nil {
		return ""
	}
	if icon := "icon_spell_" + buff; g.sprites.HasSprite(icon) {
		return icon
	}
	if icon, _ := g.resolveStatusIconSprite(buff); icon != "" && g.sprites.HasSprite(icon) {
		return icon
	}
	return ""
}

// drawBuffServiceDialog renders the service tab (icon rows: buff, span, price)
// and delegates the Quests tab to the shared dialogue body.
func (ui *UISystem) drawBuffServiceDialog(screen *ebiten.Image, dialogX, dialogY, dialogWidth, dialogHeight int) {
	npc := ui.game.dialogNPC
	layout := computeNPCDialogSectionLayout(layoutRect{dialogX, dialogY, dialogWidth, dialogHeight}, true)
	drawDebugText(screen, clipDebugText(npc.Name, layout.title.w), layout.title.x, layout.title.y)

	if buffServiceHasQuestTab(npc) {
		ui.drawDialogFolderTabs(screen, dialogX, dialogY, []string{"Service", "Talk"})
		if ui.game.dialogTab == 1 {
			ui.drawDialogueChoicesBody(screen, npc, dialogX, dialogY+50, dialogWidth)
			return
		}
	}

	greeting := ""
	if npc.DialogueData != nil {
		greeting = npc.DialogueData.Greeting
	}
	ui.drawWrappedTextWithOverflow(screen, greeting, layout.greeting, 2, dialogueLineHeight)
	drawDebugText(screen, clipDebugText(fmt.Sprintf("Party Gold: %d", ui.game.party.Gold), layout.balance.w),
		layout.balance.x, layout.balance.y)

	mouseX, mouseY := ebiten.CursorPosition()
	services := buffServiceChoices(npc)
	if maxRows := buffServiceMaxRows(dialogX, dialogY, dialogWidth, dialogHeight); len(services) > maxRows {
		services = services[:maxRows] // never draw over the footer
	}
	for i, choice := range services {
		x, y, w, h := buffServiceRowRect(dialogX, dialogY, dialogWidth, i)
		affordable := ui.game.party.Gold >= choice.Cost
		hovered := isMouseHoveringBox(mouseX, mouseY, x, y, x+w, y+h)

		bg := color.RGBA{30, 30, 50, 220}
		if !affordable {
			bg = color.RGBA{40, 28, 28, 200}
		} else if hovered {
			bg = color.RGBA{50, 55, 85, 240}
		}
		drawFilledRect(screen, x, y, w, h, bg)
		border := color.RGBA{100, 100, 130, 255}
		if affordable && hovered {
			border = color.RGBA{210, 170, 80, 240}
		}
		drawRectBorder(screen, x, y, w, h, 2, border)

		iconX := x + buffServiceIconGap
		iconY := y + (h-buffServiceIconSize)/2
		ui.drawSpellIcon(screen, iconX, iconY, buffServiceIconSize, ui.game.buffIconName(choice.Buff), "", 0, 0)

		textX := iconX + buffServiceIconSize + buffServiceIconGap
		textW := x + w - textX - 12
		drawDebugText(screen, clipDebugText(choice.Text, textW), textX, y+10)
		detail := fmt.Sprintf("%s for %s - %d gold",
			buffServiceLabel(choice.Buff), buffServiceDurationLabel(choice.DurationSeconds), choice.Cost)
		if !affordable {
			detail += " (too costly)"
		}
		drawDebugText(screen, clipDebugText(detail, textW), textX, y+10+debugTextCharHeight+4)

		if hovered {
			ui.queueTooltip([]string{
				buffServiceLabel(choice.Buff),
				fmt.Sprintf("Cast on the whole party for %s.", buffServiceDurationLabel(choice.DurationSeconds)),
				fmt.Sprintf("Cost: %d gold", choice.Cost),
				"A service - the party does not learn the spell.",
			}, mouseX+12, mouseY+8)
		}
		if ui.game.consumeLeftClickIn(x, y, x+w, y+h) {
			ui.game.pendingBuffService = choice
		}
	}

	drawDebugText(screen, "Click a charm to have it cast. ESC to leave.",
		layout.footer[0].x, layout.footer[0].y)
}

// buffServiceLabel is the player-facing name of a party buff, taken from
// spells.yaml so the service and the status bar agree.
func buffServiceLabel(buff string) string {
	if def, err := spells.GetSpellDefinitionByID(spells.SpellID(buff)); err == nil && def.Name != "" {
		return def.Name
	}
	return buff
}

func buffServiceDurationLabel(seconds int) string {
	if seconds < 60 {
		return fmt.Sprintf("%d sec", seconds)
	}
	minutes, remainder := seconds/60, seconds%60
	if remainder == 0 {
		return fmt.Sprintf("%d min", minutes)
	}
	return fmt.Sprintf("%d min %d sec", minutes, remainder)
}
