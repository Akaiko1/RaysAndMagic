package game

import (
	"image/color"

	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"

	"github.com/hajimehoshi/ebiten/v2"
)

// Editor-facing text helpers: the map editor's preview pages render labels
// through the game's own outlined/metallic label pipeline, so both UIs shade
// text identically.

// DrawShadedText draws text with the game's dark 8-direction outline;
// registered rarity metals (see rarityColor) get the vertical metal gradient.
func DrawShadedText(dst *ebiten.Image, text string, x, y int, col color.Color) {
	drawDebugTextColored(dst, text, x, y, col)
}

// RarityColor is the game's single rarity->tint mapping (metal tiers render as
// gradients through DrawShadedText).
func RarityColor(rarity string) color.Color { return rarityColor(rarity) }

// ItemRarityColor resolves an item's rarity tint, including the weapon-def
// fallback for weapons saved without a rarity field.
func ItemRarityColor(it items.Item) color.Color { return rarityColor(itemRarity(it)) }

// RefreshItemFromConfig re-adopts an item's YAML definition (rarity,
// attributes, description) - the same normalization the game applies on load -
// so a viewer displaying saved items shows current balance.
func RefreshItemFromConfig(item *items.Item) { normalizeItemFromConfig(item) }

// SchoolColor is the damage-school tint, usable wherever a school needs a
// color (editor resist sheets; free for game HUD use).
func SchoolColor(school string) color.Color {
	damageType, err := damagecalc.ParseType(school)
	if err != nil {
		return color.White
	}
	if c, ok := schoolColors[damageType]; ok {
		return c
	}
	return color.White
}

var schoolColors = map[damagecalc.Type]color.RGBA{
	damagecalc.Physical: {200, 200, 200, 255},
	damagecalc.Fire:     {255, 110, 60, 255},
	damagecalc.Water:    {80, 150, 255, 255},
	damagecalc.Air:      {160, 220, 255, 255},
	damagecalc.Earth:    {180, 140, 70, 255},
	damagecalc.Mind:     {230, 120, 255, 255},
	damagecalc.Body:     {150, 220, 90, 255},
	damagecalc.Spirit:   {235, 235, 255, 255},
	damagecalc.Light:    {255, 235, 130, 255},
	damagecalc.Dark:     {160, 70, 220, 255},
}
