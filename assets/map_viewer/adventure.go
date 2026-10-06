package main

import (
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"image/color"
)

// The editor draws the same authored rectangles used by the live hazard system.
// The preview includes dormant phases so an author can inspect every route.
func drawAdventureOverlays(screen *ebiten.Image, m mapInfo, x, y, size int) {
	if m.Config == nil || m.Config.Adventure == nil {
		return
	}
	for _, e := range m.Config.Adventure.Effects {
		c := color.RGBA{230, 170, 60, 255}
		switch e.Kind {
		case "transfer":
			c = color.RGBA{65, 190, 235, 255}
		case "lane":
			c = color.RGBA{235, 105, 95, 255}
		}
		drawRectBorder(screen, x+e.Rect[0]*size, y+e.Rect[1]*size, (e.Rect[2]-e.Rect[0]+1)*size, (e.Rect[3]-e.Rect[1]+1)*size, 1, c)
		if e.Kind == "transfer" {
			drawTileMarkerCircle(screen, x, y, size, e.Destination[0], e.Destination[1], c, false)
		}
	}
}

func adventureHoverLines(m mapInfo, x, y int) []string {
	if m.Config == nil || m.Config.Adventure == nil {
		return nil
	}
	var lines []string
	for _, e := range m.Config.Adventure.Effects {
		if !e.Contains(x, y) {
			continue
		}
		lines = append(lines, "", e.Name, "Effect: "+e.ID+" ("+e.Kind+")")
		if e.Kind == "transfer" {
			lines = append(lines, fmt.Sprintf("Receiver: %d, %d", e.Destination[0], e.Destination[1]))
		} else {
			lines = append(lines, fmt.Sprintf("Damage: %d %s", e.Damage, e.School))
		}
		if e.WarningSeconds > 0 {
			lines = append(lines, fmt.Sprintf("Warning: %.1fs / %d rounds", e.WarningSeconds, e.WarningRounds))
		}
		if e.BossBelowPercent > 0 {
			lines = append(lines, fmt.Sprintf("Active below boss HP: %d%%", e.BossBelowPercent))
		}
	}
	return lines
}
