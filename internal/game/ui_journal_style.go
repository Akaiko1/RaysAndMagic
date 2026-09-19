package game

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"image"
	"image/color"
	uitext "ugataima/assets/text"
	"ugataima/internal/graphics"
	"ugataima/internal/quests"
)

const readingTextScale = 2
const readingLineHeight = 32

func drawReadingText(dst *ebiten.Image, text string, x, y int, col color.Color) {
	label := outlinedLabelImage(text, col)
	b := label.Bounds()
	// Integer enlargement of the same cached labels used by the top banners.
	graphics.DrawImageScaled(dst, label, float64(x), float64(y), float64(b.Dx())*readingTextScale, float64(b.Dy())*readingTextScale, nil)
}

func (ui *UISystem) drawDialogTab(dst *ebiten.Image, r layoutRect, active bool) {
	fill, rim := color.RGBA{20, 19, 17, 255}, color.RGBA{117, 95, 62, 255}
	if active {
		fill, rim = color.RGBA{31, 27, 20, 255}, color.RGBA{205, 175, 107, 255}
	}
	// The fill follows the chamfer too; a rectangular fill leaves dark triangles.
	drawFilledRect(dst, r.x, r.y+4, r.w, r.h-4, fill)
	for row := 0; row < 4; row++ {
		inset := 4 - row
		drawFilledRect(dst, r.x+inset, r.y+row, r.w-2*inset, 1, fill)
	}
	// Three rails form a folder tab. The active bottom opens into the panel.
	drawFilledRect(dst, r.x, r.y+4, 1, r.h-4, rim)
	drawFilledRect(dst, r.x+r.w-1, r.y+4, 1, r.h-4, rim)
	drawFilledRect(dst, r.x+4, r.y, r.w-8, 1, rim)
	vector.StrokeLine(dst, float32(r.x), float32(r.y+4), float32(r.x+4), float32(r.y), 1, rim, false)
	vector.StrokeLine(dst, float32(r.right()-4), float32(r.y), float32(r.right()-1), float32(r.y+4), 1, rim, false)
	if active {
		drawFilledRect(dst, r.x+1, r.bottom()-2, r.w-2, 4, fill)
	} else {
		drawFilledRect(dst, r.x, r.bottom()-1, r.w, 1, rim)
	}
}

func (ui *UISystem) drawCardEffectsList(dst *ebiten.Image, r layoutRect) {
	ui.drawThemeFrame(dst, frameBronze, r.x, r.y, r.w, r.h)
	drawReadingText(dst, uitext.Text("ui.party_effects"), r.x+12, r.y+10, rarityGold)
	var lines []string
	for _, effect := range ui.game.cardCollectionEffectLines() {
		wrapped := wrapDebugText(effect, int(float64(r.w-56)/readingTextScale))
		for i, line := range wrapped {
			prefix := "  "
			if i == 0 {
				prefix = "- "
			}
			lines = append(lines, prefix+line)
		}
	}
	if len(lines) == 0 {
		lines = []string{uitext.Text("ui.no_active_effects")}
	}
	perPage := max(1, (r.h-100)/readingLineHeight)
	pages := (len(lines) + perPage - 1) / perPage
	ui.cardEffectsPage = min(max(0, ui.cardEffectsPage), pages-1)
	start := ui.cardEffectsPage * perPage
	for i, line := range lines[start:min(len(lines), start+perPage)] {
		drawReadingText(dst, line, r.x+12, r.y+56+i*readingLineHeight, color.RGBA{224, 214, 190, 255})
	}
	if pages > 1 {
		ui.drawPager(dst, r.x+12, r.bottom()-28, r.w-24, &ui.cardEffectsPage, pages, !ui.modalLayerOwnsInput())
	}
}

// Sorting, status and visual treatment use the same quest state classifier.
type questEntryStyle struct {
	status                       string
	title, body, rim, fill, wash color.RGBA
}

func journalEntryStyle(q *quests.Quest) questEntryStyle {
	switch questJournalRank(q) {
	case questRankTurnIn:
		return questEntryStyle{uitext.Text("ui.quest_reward_ready"), rarityGold, color.RGBA{238, 224, 189, 255}, color.RGBA{228, 184, 84, 255}, color.RGBA{41, 32, 17, 255}, color.RGBA{38, 26, 5, 140}}
	case questRankActive:
		return questEntryStyle{uitext.Text("ui.quest_in_progress"), raritySilver, color.RGBA{207, 214, 221, 255}, color.RGBA{125, 153, 180, 255}, color.RGBA{21, 27, 34, 255}, color.RGBA{10, 21, 35, 195}}
	default:
		return questEntryStyle{uitext.Text("ui.quest_concluded"), color.RGBA{168, 145, 113, 255}, color.RGBA{148, 142, 131, 255}, color.RGBA{108, 88, 64, 255}, color.RGBA{23, 21, 19, 255}, color.RGBA{16, 15, 14, 220}}
	}
}

func (ui *UISystem) drawJournalEntry(dst *ebiten.Image, quest *quests.Quest, r layoutRect, copy questCardCopy) {
	style := journalEntryStyle(quest)
	drawFilledRect(dst, r.x, r.y, r.w, r.h, style.fill)
	if ui.game.sprites.HasSprite("theme_quest_paper") {
		drawJournalMaterial(dst, ui.game.sprites.GetSprite("theme_quest_paper"), r)
		drawFilledRect(dst, r.x, r.y, r.w, r.h, style.wash)
	}
	drawMetalPlate(dst, r.x, r.y, r.w, 1, style.rim)
	drawMetalPlate(dst, r.x, r.bottom()-1, r.w, 1, style.rim)
	drawFilledRect(dst, r.x, r.y+1, 3, r.h-2, style.rim)
	drawFilledRect(dst, r.right()-1, r.y+1, 1, r.h-2, style.rim)
	status := style.status
	titleW := r.w - 200
	drawDebugTextColored(dst, clipDebugText(quest.Definition.Name, titleW), r.x+16, r.y+8, style.title)
	drawDebugTextColored(dst, status, r.right()-debugTextWidth(status)-16, r.y+8, style.rim)
	for i, line := range copy.descLines {
		drawDebugTextColored(dst, line, r.x+16, r.y+questCardDescTop+i*questCardLineHeight, style.body)
	}
	ui.offerClippedTextTooltip(copy.fullLines, copy.descClipped(), r.x+16, r.y+questCardDescTop, r.w-32, len(copy.descLines)*questCardLineHeight)
	bottom := r.y + questCardDescTop + len(copy.descLines)*questCardLineHeight
	progressText := quest.GetProgressString()
	if quest.Definition.Type == "encounter" {
		progressText = uitext.Text("ui.defeat_all_enemies")
		if quest.Completed {
			progressText = uitext.Text("ui.all_enemies_defeated")
		}
	}
	half := r.w/2 - 28
	drawDebugTextColored(dst, clipDebugText(progressText, half), r.x+16, bottom, style.body)
	rewardsX := r.x + r.w/2 + 10
	reward := uitext.Text("ui.reward", questRewardSummary(quest.Definition.Rewards.Gold, quest.Definition.Rewards.ArenaPoints, quest.Definition.Rewards.Experience))
	drawDebugTextColored(dst, clipDebugText(reward, half), rewardsX, bottom, style.body)
	y := bottom + questCardProgressGap
	if quest.Definition.Type == "kill" || quest.Definition.Type == "interact" {
		width := min(260, half)
		drawFilledRect(dst, r.x+16, y+8, width, 4, color.RGBA{65, 53, 35, 255})
		progress := 0.0
		if target := quest.Target(); target > 0 {
			progress = min(1, max(0, float64(quest.CurrentCount)/float64(target)))
		}
		drawMetalPlate(dst, r.x+16, y+8, int(float64(width)*progress), 4, style.rim)
	}
	if quest.Completed && !quest.RewardsClaimed {
		button := layoutRect{rewardsX, y, min(132, half), questCardBarH}
		mx, my := pointerPosition()
		hover := isMouseHoveringBox(mx, my, button.x, button.y, button.right(), button.bottom())
		tint := style.rim
		if hover {
			tint = rarityGold
		}
		drawFilledRect(dst, button.x, button.y, button.w, button.h, color.RGBA{54, 41, 21, 255})
		drawMetalPlate(dst, button.x, button.bottom()-1, button.w, 1, tint)
		drawDebugTextColored(dst, uitext.Text("ui.claim_reward"), button.x+10, button.y+4, tint)
		ui.onDisplayedInput(uiCommandClick, button, func() {
			if !ui.modalLayerOwnsInput() && ui.game.consumeLeftClickIn(button.x, button.y, button.right(), button.bottom()) {
				ui.claimQuestReward(quest.ID)
			}
		})
	}
}

// Tile the material uniformly so a wide journal row never turns fibres into
// stretched horizontal bands. Every tile still uses the shared image filter.
func drawJournalMaterial(dst, src *ebiten.Image, r layoutRect) {
	if src == nil {
		return
	}
	const tile = 256
	b := src.Bounds()
	for y := 0; y < r.h; y += tile {
		for x := 0; x < r.w; x += tile {
			w, h := min(tile, r.w-x), min(tile, r.h-y)
			cut := image.Rect(b.Min.X, b.Min.Y, b.Min.X+w*b.Dx()/tile, b.Min.Y+h*b.Dy()/tile)
			part := src.RecyclableSubImage(cut)
			drawImageScaled(dst, part, r.x+x, r.y+y, w, h)
			part.Recycle()
		}
	}
}
