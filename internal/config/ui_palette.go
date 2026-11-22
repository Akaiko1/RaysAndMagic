package config

import (
	"image/color"
	"strings"

	"ugataima/internal/damage"
)

// Shared by content icons, names, tooltips and the editor.
var (
	RaritySilver    = color.RGBA{210, 216, 230, 255}
	RarityGold      = color.RGBA{255, 215, 0, 255}
	RarityLegendary = color.RGBA{220, 80, 20, 255}
	RarityUnique    = color.RGBA{70, 220, 130, 255}
)

func RarityRGBA(rarity string) color.RGBA {
	switch strings.ToLower(rarity) {
	case "uncommon":
		return RaritySilver
	case "rare":
		return RarityGold
	case "legendary":
		return RarityLegendary
	case "unique":
		return RarityUnique
	default:
		return color.RGBA{255, 255, 255, 255}
	}
}

func SchoolRGBA(school string) color.RGBA {
	s, err := damage.ParseType(school)
	if err != nil {
		return color.RGBA{255, 255, 255, 255}
	}
	if c, ok := schoolPalette[s]; ok {
		return c
	}
	return color.RGBA{255, 255, 255, 255}
}

// SchoolTextRGBA is a school's tint for text: its palette color, lifted where
// that reads under 4.5:1 on the card plate (30,30,60).
func SchoolTextRGBA(school string) color.RGBA {
	if s, err := damage.ParseType(school); err == nil {
		if c, ok := schoolTextPalette[s]; ok {
			return c
		}
	}
	return SchoolRGBA(school)
}

// schoolTextPalette lifts the school colors too dark for text (Dark is 3.37:1)
// while keeping them apart from their neighbours (Dark from Mind).
var schoolTextPalette = map[damage.Type]color.RGBA{
	damage.Dark: {175, 100, 250, 255},
}

var schoolPalette = map[damage.Type]color.RGBA{
	damage.Physical: {200, 200, 200, 255},
	damage.Fire:     {255, 110, 60, 255},
	damage.Water:    {80, 150, 255, 255},
	damage.Air:      {160, 220, 255, 255},
	damage.Earth:    {180, 140, 70, 255},
	damage.Mind:     {230, 120, 255, 255},
	damage.Body:     {150, 220, 90, 255},
	damage.Spirit:   {235, 235, 255, 255},
	damage.Light:    {255, 235, 130, 255},
	damage.Dark:     {160, 70, 220, 255},
}
