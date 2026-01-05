package game

import (
	"fmt"
	"math"
	"strings"
	"ugataima/internal/items"

	"github.com/hajimehoshi/ebiten/v2"
)

// LootBag represents a dropped bag of items on the ground.
type LootBag struct {
	X, Y           float64
	Gold           int
	Items          []items.Item
	SizeMultiplier float64
}

type LootBagRenderInfo struct {
	ScreenX    int
	ScreenY    int
	SpriteSize int
	Distance   float64
	Visible    bool
}

func (g *MMGame) lootBagPickupRange() float64 {
	if g == nil {
		return 0
	}
	return float64(g.config.GetTileSize()) * 2.0
}

func (g *MMGame) tryPickupNearestLootBag(maxDist float64) bool {
	if g == nil {
		return false
	}
	if idx := g.findNearestLootBagIndex(maxDist); idx >= 0 {
		g.pickupLootBagAt(idx)
		return true
	}
	return false
}

func (g *MMGame) addLootBag(x, y float64, drops []items.Item, gold int, sizeMultiplier float64) {
	if len(drops) == 0 && gold <= 0 {
		return
	}
	if sizeMultiplier <= 0 {
		sizeMultiplier = 0.33
	}
	g.lootBags = append(g.lootBags, LootBag{
		X:              x,
		Y:              y,
		Gold:           gold,
		Items:          append([]items.Item{}, drops...),
		SizeMultiplier: sizeMultiplier,
	})
}

func (g *MMGame) findNearestLootBagIndex(maxDist float64) int {
	if len(g.lootBags) == 0 {
		return -1
	}
	playerX, playerY := g.camera.X, g.camera.Y
	maxDistSq := maxDist * maxDist
	bestIdx := -1
	bestDistSq := 0.0
	for i := range g.lootBags {
		bag := &g.lootBags[i]
		dx := bag.X - playerX
		dy := bag.Y - playerY
		distSq := dx*dx + dy*dy
		if distSq > maxDistSq {
			continue
		}
		if bestIdx == -1 || distSq < bestDistSq {
			bestIdx = i
			bestDistSq = distSq
		}
	}
	return bestIdx
}

func (g *MMGame) findLootBagIndexAtScreen(clickX, clickY int, maxDist float64) int {
	if len(g.lootBags) == 0 || g.renderHelper == nil {
		return -1
	}
	playerX, playerY := g.camera.X, g.camera.Y
	maxDistSq := maxDist * maxDist
	bestIdx := -1
	bestDistSq := 0.0

	for i := range g.lootBags {
		bag := &g.lootBags[i]
		dx := bag.X - playerX
		dy := bag.Y - playerY
		distSq := dx*dx + dy*dy
		if distSq > maxDistSq {
			continue
		}
		distance := math.Hypot(dx, dy)
		info := g.lootBagRenderInfo(bag, distance)
		if !g.lootBagHitTestFromInfo(info, clickX, clickY, maxDist) {
			continue
		}
		if bestIdx == -1 || distSq < bestDistSq {
			bestIdx = i
			bestDistSq = distSq
		}
	}
	return bestIdx
}

func (g *MMGame) pickupLootBagAt(index int) {
	if index < 0 || index >= len(g.lootBags) {
		return
	}
	bag := g.lootBags[index]
	if len(bag.Items) == 0 && bag.Gold <= 0 {
		g.lootBags = append(g.lootBags[:index], g.lootBags[index+1:]...)
		return
	}
	for _, it := range bag.Items {
		g.party.AddItem(it)
	}
	if bag.Gold > 0 {
		g.party.Gold += bag.Gold
	}

	if len(bag.Items) == 0 && bag.Gold > 0 {
		g.AddCombatMessage(fmt.Sprintf("Picked up %d gold.", bag.Gold))
	} else if len(bag.Items) == 1 && bag.Gold <= 0 {
		g.AddCombatMessage(fmt.Sprintf("Picked up %s.", bag.Items[0].Name))
	} else {
		parts := make([]string, 0, len(bag.Items)+1)
		if bag.Gold > 0 {
			parts = append(parts, fmt.Sprintf("%d gold", bag.Gold))
		}
		for _, it := range bag.Items {
			parts = append(parts, it.Name)
		}
		g.AddCombatMessage(fmt.Sprintf("Picked up loot bag: %s.", strings.Join(parts, ", ")))
	}

	// Remove bag
	g.lootBags = append(g.lootBags[:index], g.lootBags[index+1:]...)
}

func (g *MMGame) lootBagRenderInfo(bag *LootBag, distance float64) LootBagRenderInfo {
	info := LootBagRenderInfo{Distance: distance}
	if bag == nil || g.renderHelper == nil {
		return info
	}
	if info.Distance < 0 {
		info.Distance = math.Hypot(bag.X-g.camera.X, bag.Y-g.camera.Y)
	}
	info.ScreenX, info.ScreenY, info.SpriteSize, info.Visible = g.renderHelper.CalculateMonsterSpriteMetrics(bag.X, bag.Y, info.Distance, bag.SizeMultiplier)
	return info
}

func (g *MMGame) lootBagHitTestFromInfo(info LootBagRenderInfo, mouseX, mouseY int, maxDist float64) bool {
	if !info.Visible || info.SpriteSize <= 0 {
		return false
	}
	if info.Distance > maxDist {
		return false
	}
	sprite := g.sprites.GetSprite("bag")
	drawLeft := info.ScreenX - info.SpriteSize/2
	return spriteHitTest(sprite, mouseX, mouseY, drawLeft, info.ScreenY, info.SpriteSize)
}

func spriteHitTest(sprite *ebiten.Image, mouseX, mouseY, drawLeft, drawTop, spriteSize int) bool {
	if sprite == nil || spriteSize <= 0 {
		return false
	}
	if mouseX < drawLeft || mouseX >= drawLeft+spriteSize || mouseY < drawTop || mouseY >= drawTop+spriteSize {
		return false
	}
	spriteW := sprite.Bounds().Dx()
	spriteH := sprite.Bounds().Dy()
	if spriteW == 0 || spriteH == 0 {
		return false
	}
	scaleX := float64(spriteSize) / float64(spriteW)
	scaleY := float64(spriteSize) / float64(spriteH)
	localX := int(float64(mouseX-drawLeft) / scaleX)
	localY := int(float64(mouseY-drawTop) / scaleY)
	if localX < 0 || localX >= spriteW || localY < 0 || localY >= spriteH {
		return false
	}
	_, _, _, a := sprite.At(localX, localY).RGBA()
	return a > 0
}
