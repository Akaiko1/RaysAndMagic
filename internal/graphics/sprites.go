package graphics

import (
	"image"
	"image/color"
	_ "image/png"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

type SpriteManager struct {
	sprites         map[string]*ebiten.Image
	spriteTypeCache map[string]string // Cache sprite types to avoid repeated file checks
}

func NewSpriteManager() *SpriteManager {
	return &SpriteManager{
		sprites:         make(map[string]*ebiten.Image),
		spriteTypeCache: make(map[string]string),
	}
}

func (sm *SpriteManager) createPlaceholder(name string) *ebiten.Image {
	// Create larger sprites for trees and biome obstacles to prevent transparency issues
	var img *ebiten.Image
	if name == "tree" || name == "ancient_tree" || name == "sand_dune" || name == "large_dune" || name == "coral_reef" || name == "large_coral" {
		img = ebiten.NewImage(32, 32) // Larger obstacle sprites
	} else {
		img = ebiten.NewImage(16, 16)
	}

	// Determine sprite type based on search paths (cached)
	spriteType := sm.getCachedSpriteType(name)

	switch spriteType {
	case "environment":
		img.Fill(color.RGBA{128, 0, 128, 255}) // Purple for environment
	case "npc_mob":
		img.Fill(color.RGBA{0, 128, 0, 255}) // Green for NPCs/mobs
	default:
		img.Fill(color.RGBA{128, 128, 128, 255}) // Gray for unknown
	}

	return img
}

// getCachedSpriteType determines sprite type with caching to avoid repeated file checks
func (sm *SpriteManager) getCachedSpriteType(name string) string {
	if spriteType, exists := sm.spriteTypeCache[name]; exists {
		return spriteType
	}

	spriteType := sm.determineSpritePaths(name)
	sm.spriteTypeCache[name] = spriteType
	return spriteType
}

// determineSpritePaths determines the sprite type by checking which path would be tried first
func (sm *SpriteManager) determineSpritePaths(name string) string {
	searchPaths := []string{
		"assets/sprites/mobs/" + name + ".png",        // Monsters
		"assets/sprites/characters/" + name + ".png",  // NPCs and characters
		"assets/sprites/environment/" + name + ".png", // Environment objects
	}

	// Check if any path exists (for loaded sprites) or determine most likely type
	for i, spritePath := range searchPaths {
		if _, err := os.Stat(spritePath); err == nil {
			switch i {
			case 0, 1: // mobs or characters path
				return "npc_mob"
			case 2: // environment path
				return "environment"
			}
		}
	}

	// If no file exists, default to gray
	return "unknown"
}

func (sm *SpriteManager) GetSprite(name string) *ebiten.Image {
	if sprite, exists := sm.sprites[name]; exists {
		return sprite
	}

	// Try to dynamically load the sprite if it's not already loaded
	sm.loadSpriteIfExists(name)

	// Check again after attempting to load
	if sprite, exists := sm.sprites[name]; exists {
		return sprite
	}

	// If still not found, create placeholder
	return sm.createPlaceholder(name)
}

// loadSpriteIfExists attempts to load a sprite from common locations
func (sm *SpriteManager) loadSpriteIfExists(name string) {
	// Search paths in order of priority
	searchPaths := []string{
		"assets/sprites/mobs/" + name + ".png",        // Monsters
		"assets/sprites/characters/" + name + ".png",  // NPCs and characters
		"assets/sprites/environment/" + name + ".png", // Environment objects
	}

	for i, spritePath := range searchPaths {
		file, err := os.Open(spritePath)
		if err == nil {
			defer file.Close()
			img, _, err := image.Decode(file)
			if err == nil {
				sm.sprites[name] = ebiten.NewImageFromImage(img)
				// Cache sprite type for future placeholder requests
				switch i {
				case 0, 1: // mobs or characters path
					sm.spriteTypeCache[name] = "npc_mob"
				case 2: // environment path
					sm.spriteTypeCache[name] = "environment"
				}
				return
			}
		}
	}

	// If no sprite file found, cache as unknown to avoid future file checks
	sm.spriteTypeCache[name] = "unknown"
}
