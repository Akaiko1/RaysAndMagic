package main

import (
	"image"
	"image/color"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/game"
	"ugataima/internal/quests"
)

// Reservations are derived editor annotations, never permanent map spawns.
// Show every possible layout: runtime selects one only when accepting a quest.
type editorQuestProp struct {
	NPC, Quest, Name, Layout string
}

var questPropColor = color.RGBA{201, 157, 255, 255}

// Definitions and their derived spatial index always change together.
func (v *viewer) setEditorQuestDefs(defs map[string]*quests.QuestDefinition) {
	v.overlay.questDefs = defs
	v.overlay.questProps = nil
}

func (v *viewer) drawQuestPropLegend(screen *ebiten.Image, key string, x, y, width int) {
	if len(v.questPropsForMap(key)) == 0 {
		return
	}
	game.DrawShadedText(screen, clipText("Purple Q: quest NPC reservations (all routes)", width), x, y, questPropColor)
}

func (v *viewer) questPropsForMap(key string) map[[2]int][]editorQuestProp {
	defs := v.overlayQuestDefs()
	if v.overlay.questProps == nil {
		v.overlay.questProps = make(map[string]map[[2]int][]editorQuestProp)
		ids := make([]string, 0, len(defs))
		for id := range defs {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		for _, id := range ids {
			def := defs[id]
			if def == nil {
				continue
			}
			for _, layout := range def.PropLayouts() {
				for _, prop := range layout.Props {
					cells := v.overlay.questProps[prop.Map]
					if cells == nil {
						cells = make(map[[2]int][]editorQuestProp)
						v.overlay.questProps[prop.Map] = cells
					}
					xy := [2]int{prop.X, prop.Y}
					cells[xy] = append(cells[xy], editorQuestProp{prop.NPC, id, def.Name, layout.ID})
				}
			}
		}
	}
	return v.overlay.questProps[key]
}

func (v *viewer) questPropHoverLines(key string, x, y int) []string {
	props := v.questPropsForMap(key)[[2]int{x, y}]
	if len(props) == 0 {
		return nil
	}
	lines := []string{"", "QUEST NPC - RESERVED", "Keep this cell clear of permanent NPCs.", "Read-only placement from assets/quests.yaml."}
	for _, prop := range props {
		lines = append(lines, "", "NPC: "+prop.NPC, "Quest: "+prop.Name, "Key: "+prop.Quest, "Route: "+prop.Layout)
	}
	return lines
}

// Sprites go beneath authored entities; reservation borders stay above them so
// an overlapping permanent NPC cannot conceal the reserved cell.
func (v *viewer) drawQuestPropReservations(screen *ebiten.Image, m mapInfo, l layout, sprites bool) {
	bounds := image.Rect(l.mapAreaX+2, l.mapAreaY+2, l.mapAreaX+l.mapAreaW-2, l.mapAreaY+l.mapAreaH-2).Intersect(screen.Bounds())
	if bounds.Empty() {
		return
	}
	screen = screen.SubImage(bounds).(*ebiten.Image)
	for xy, props := range v.questPropsForMap(m.Key) {
		x, y := l.originX+xy[0]*l.tileSize, l.originY+xy[1]*l.tileSize
		if !image.Rect(x, y, x+l.tileSize, y+l.tileSize).Overlaps(screen.Bounds()) {
			continue
		}
		if sprites {
			if img := v.tileSpriteThumbnail(npcSpriteForKey(props[0].NPC)); img != nil {
				drawImageInBox(screen, img, x, y, l.tileSize, l.tileSize)
			} else {
				drawTileMarkerCircle(screen, l.originX, l.originY, l.tileSize, xy[0], xy[1], questPropColor, false)
			}
			continue
		}
		drawRectBorder(screen, x, y, l.tileSize, l.tileSize, 1, questPropColor)
		if l.tileSize >= 18 {
			drawFilledRect(screen, x+1, y+1, 10, 14, color.RGBA{38, 25, 56, 255})
			game.DrawShadedText(screen, "Q", x+2, y+1, questPropColor)
		}
	}
}
