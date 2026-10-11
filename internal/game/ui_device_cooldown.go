package game

import (
	"image/color"
	"math"

	"ugataima/internal/character"
	"ugataima/internal/items"
	"ugataima/internal/spells"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// Reveal the square icon clockwise from twelve o'clock. The remaining sector
// darkens the existing art; drawing never changes the item's gameplay clock.
func drawDeviceCooldown(screen *ebiten.Image, item items.Item, x, y, size int) {
	drawIconCooldown(screen, deviceCooldownProgress(item), x, y, size)
}

func drawIconCooldown(screen *ebiten.Image, progress float64, x, y, size int) {
	if progress >= 1 || size <= 0 {
		return
	}
	shade := color.RGBA{0, 0, 0, 190}
	if progress <= 0 {
		drawFilledRect(screen, x, y, size, size, shade)
		return
	}
	cx, cy, half := float64(x)+float64(size)/2, float64(y)+float64(size)/2, float64(size)/2
	var path vector.Path
	path.MoveTo(float32(cx), float32(cy))
	edge := func(angle float64) {
		dx, dy := math.Sin(angle), -math.Cos(angle)
		scale := half / max(math.Abs(dx), math.Abs(dy))
		path.LineTo(float32(cx+dx*scale), float32(cy+dy*scale))
	}
	start := progress * 2 * math.Pi
	edge(start)
	for corner := math.Pi / 4; corner < 2*math.Pi; corner += math.Pi / 2 {
		if corner > start {
			edge(corner)
		}
	}
	edge(2 * math.Pi)
	path.Close()
	uiFillPath(screen, &path, nil, &vector.DrawPathOptions{ColorScale: statusColorScale(shade), AntiAlias: true})
}

// Spellbook and equipped spell slots read the same saved personal clock.
func (ui *UISystem) drawSpellReuse(screen *ebiten.Image, c *character.MMCharacter, id spells.SpellID, x, y, size int) {
	if c == nil {
		return
	}
	remaining := c.SpellState.ReuseFrames[string(id)]
	if remaining <= 0 {
		return
	}
	d, err := spells.GetSpellDefinitionByID(id)
	if err != nil || d.ReuseSeconds <= 0 {
		return
	}
	drawIconCooldown(screen, 1-float64(remaining)/float64(d.ReuseSeconds*ui.game.config.GetTPS()), x, y, size)
}
