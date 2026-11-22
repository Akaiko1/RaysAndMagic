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

// The cooldown dial has two entry points, one per clock that an icon shows:
// drawDeviceCooldown (a device's recharge, use_cooldown_seconds on the item)
// and drawSpellReuse (a spell's personal lockout, reuse_seconds). Both draw
// through drawIconCooldown. A hero's action cooldown is no dial: it is the
// band around the party card.

// drawDeviceCooldown is the recharge dial on a device's icon, wherever the
// item is drawn: bag, quick slots, stash.
func drawDeviceCooldown(screen *ebiten.Image, item items.Item, x, y, size int) {
	drawIconCooldown(screen, deviceCooldownProgress(item), x, y, size)
}

// drawIconCooldown reveals the square icon clockwise from twelve o'clock as
// progress (0..1) grows; the remaining sector darkens the art. Drawing never
// changes a gameplay clock.
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

// drawSpellReuse is the lockout dial on a spell's icon: the spellbook and
// spell items in bags and slots read the same saved personal clock.
func (ui *UISystem) drawSpellReuse(screen *ebiten.Image, c *character.MMCharacter, id spells.SpellID, x, y, size int) {
	if c == nil {
		return
	}
	remaining := c.SpellState.Reuse[string(id)].Frames
	if remaining <= 0 {
		return
	}
	d, err := spells.GetSpellDefinitionByID(id)
	if err != nil || d.ReuseSeconds <= 0 {
		return
	}
	drawIconCooldown(screen, 1-float64(remaining)/float64(d.ReuseSeconds*ui.game.config.GetTPS()), x, y, size)
}
