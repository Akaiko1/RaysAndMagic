package graphics

import (
	"image"
	"image/color"
	_ "image/png"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

type SpriteManager struct {
	sprites map[string]*ebiten.Image
}

func NewSpriteManager() *SpriteManager {
	sm := &SpriteManager{
		sprites: make(map[string]*ebiten.Image),
	}

	// Load character sprites (these are still hardcoded as they're not in YAML)
	sm.loadSprite("human", "assets/sprites/characters/human.png")
	sm.loadSprite("elf", "assets/sprites/characters/elf.png")
	sm.loadSprite("elf_warrior", "assets/sprites/characters/elf_warrior.png")

	// Load party member portraits
	sm.loadSprite("gareth", "assets/sprites/characters/gareth.png")
	sm.loadSprite("lysander", "assets/sprites/characters/lysander.png")
	sm.loadSprite("celestine", "assets/sprites/characters/celestine.png")
	sm.loadSprite("silvelyn", "assets/sprites/characters/sylvelyn.png") // Note: file is sylvelyn.png

	// Load environment sprites (these could also be made dynamic in the future)
	sm.loadSprite("tree", "assets/sprites/environment/tree.png")
	sm.loadSprite("grass", "assets/sprites/environment/grass.png")
	sm.loadSprite("ancient_tree", "assets/sprites/environment/ancient_tree.png")
	sm.loadSprite("mushroom_ring", "assets/sprites/environment/mushroom_ring.png")
	sm.loadSprite("forest_stream", "assets/sprites/environment/forest_stream.png")
	sm.loadSprite("fern_patch", "assets/sprites/environment/fern_patch.png")
	sm.loadSprite("moss_rock", "assets/sprites/environment/moss_rock.png")
	sm.loadSprite("firefly_swarm", "assets/sprites/environment/firefly_swarm.png")
	sm.loadSprite("water", "assets/sprites/environment/water.png")

	// Monster sprites are now loaded dynamically when requested
	// This eliminates the need to hardcode every monster sprite

	return sm
}

func (sm *SpriteManager) loadSprite(name, path string) {
	file, err := os.Open(path)
	if err != nil {
		// If sprite doesn't exist, create a placeholder
		sm.sprites[name] = sm.createPlaceholder(name)
		return
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		sm.sprites[name] = sm.createPlaceholder(name)
		return
	}

	sm.sprites[name] = ebiten.NewImageFromImage(img)
}

func (sm *SpriteManager) createPlaceholder(name string) *ebiten.Image {
	// Create larger sprites for trees to prevent transparency issues
	var img *ebiten.Image
	if name == "tree" || name == "ancient_tree" {
		img = ebiten.NewImage(32, 32) // Larger tree sprites
	} else {
		img = ebiten.NewImage(16, 16)
	}

	// Create simple pixel art patterns for better visual distinction
	switch name {
	case "human":
		sm.drawHumanPlaceholder(img)
	case "elf":
		sm.drawElfPlaceholder(img)
	case "orc", "forest_orc":
		sm.drawOrcPlaceholder(img)
	case "goblin":
		sm.drawGoblinPlaceholder(img)
	case "dire_wolf":
		sm.drawWolfPlaceholder(img)
	case "spider", "forest_spider":
		sm.drawSpiderPlaceholder(img)
	case "treant":
		sm.drawTreeantPlaceholder(img)
	case "pixie":
		sm.drawPixiePlaceholder(img)
	case "dragon":
		sm.drawDragonPlaceholder(img)
	case "tree", "ancient_tree":
		sm.drawTreePlaceholder(img)
	case "grass":
		img.Fill(color.RGBA{34, 139, 34, 255})
	case "mushroom_ring":
		sm.drawMushroomRingPlaceholder(img)
	case "moss_rock":
		sm.drawMossRockPlaceholder(img)
	case "fern_patch":
		sm.drawFernPatchPlaceholder(img)
	case "forest_stream":
		sm.drawForestStreamPlaceholder(img)
	case "firefly_swarm":
		sm.drawFireflySwarmPlaceholder(img)
	case "water":
		sm.drawWaterPlaceholder(img)
	default:
		img.Fill(color.RGBA{255, 0, 255, 255}) // Magenta for unknown
	}

	return img
}

func (sm *SpriteManager) drawHumanPlaceholder(img *ebiten.Image) {
	img.Fill(color.RGBA{255, 220, 177, 255}) // Skin color
	// Add simple armor details
	for y := 6; y < 12; y++ {
		for x := 4; x < 12; x++ {
			img.Set(x, y, color.RGBA{100, 100, 100, 255}) // Gray armor
		}
	}
}

func (sm *SpriteManager) drawElfPlaceholder(img *ebiten.Image) {
	img.Fill(color.RGBA{34, 139, 34, 255}) // Green
	// Add bow
	for y := 4; y < 12; y++ {
		img.Set(2, y, color.RGBA{101, 67, 33, 255}) // Brown bow
	}
}

func (sm *SpriteManager) drawOrcPlaceholder(img *ebiten.Image) {
	img.Fill(color.RGBA{85, 107, 47, 255}) // Olive green
	// Add tusks
	img.Set(6, 4, color.RGBA{255, 255, 255, 255})
	img.Set(9, 4, color.RGBA{255, 255, 255, 255})
}

func (sm *SpriteManager) drawGoblinPlaceholder(img *ebiten.Image) {
	img.Fill(color.RGBA{60, 80, 30, 255}) // Darker green
	// Smaller than orc, add pointy ears
	img.Set(2, 3, color.RGBA{60, 80, 30, 255})
	img.Set(13, 3, color.RGBA{60, 80, 30, 255})
}

func (sm *SpriteManager) drawWolfPlaceholder(img *ebiten.Image) {
	img.Fill(color.RGBA{64, 64, 64, 255}) // Dark gray
	// Add lighter belly
	for y := 8; y < 12; y++ {
		for x := 4; x < 12; x++ {
			img.Set(x, y, color.RGBA{96, 96, 96, 255})
		}
	}
}

func (sm *SpriteManager) drawSpiderPlaceholder(img *ebiten.Image) {
	img.Fill(color.RGBA{32, 32, 32, 255}) // Black
	// Add red markings
	img.Set(7, 5, color.RGBA{200, 0, 0, 255})
	img.Set(8, 5, color.RGBA{200, 0, 0, 255})
}

func (sm *SpriteManager) drawTreeantPlaceholder(img *ebiten.Image) {
	img.Fill(color.RGBA{101, 67, 33, 255}) // Brown
	// Add green leaves
	for y := 0; y < 4; y++ {
		for x := 4; x < 12; x++ {
			img.Set(x, y, color.RGBA{34, 139, 34, 255})
		}
	}
}

func (sm *SpriteManager) drawPixiePlaceholder(img *ebiten.Image) {
	img.Fill(color.RGBA{255, 192, 203, 255}) // Light pink
	// Add sparkles
	img.Set(3, 2, color.RGBA{255, 255, 0, 255})
	img.Set(12, 6, color.RGBA{255, 255, 0, 255})
	img.Set(6, 13, color.RGBA{255, 255, 0, 255})
}

func (sm *SpriteManager) drawDragonPlaceholder(img *ebiten.Image) {
	img.Fill(color.RGBA{139, 0, 0, 255}) // Dark red base
	// Add golden scales
	for y := 2; y < 14; y += 2 {
		for x := 2; x < 14; x += 2 {
			if (x+y)%4 == 0 {
				img.Set(x, y, color.RGBA{255, 215, 0, 255}) // Gold
			}
		}
	}
	// Add fierce eyes
	img.Set(5, 4, color.RGBA{255, 255, 0, 255}) // Yellow eyes
	img.Set(10, 4, color.RGBA{255, 255, 0, 255})
}

func (sm *SpriteManager) drawTreePlaceholder(img *ebiten.Image) {
	// Fill with opaque forest background color to prevent transparency issues
	img.Fill(color.RGBA{34, 139, 34, 255}) // Forest green background - opaque

	// Brown trunk (larger and more centered for 32x32 sprite)
	for y := 16; y < 32; y++ {
		for x := 12; x < 20; x++ {
			img.Set(x, y, color.RGBA{101, 67, 33, 255})
		}
	}
	// Darker green leaves for contrast (larger area)
	for y := 4; y < 20; y++ {
		for x := 6; x < 26; x++ {
			// Create a more tree-like circular shape
			centerX, centerY := 16, 12
			dx, dy := x-centerX, y-centerY
			if dx*dx+dy*dy < 100 { // Circular leaves
				img.Set(x, y, color.RGBA{20, 100, 20, 255}) // Darker green leaves
			}
		}
	}
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
	// Try loading from mobs folder first (most common case)
	mobPath := "assets/sprites/mobs/" + name + ".png"
	file, err := os.Open(mobPath)
	if err == nil {
		defer file.Close()
		img, _, err := image.Decode(file)
		if err == nil {
			sm.sprites[name] = ebiten.NewImageFromImage(img)
			return
		}
	}

	// You could add more search paths here if needed:
	// characterPath := "assets/sprites/characters/" + name + ".png"
	// environmentPath := "assets/sprites/environment/" + name + ".png"
}

func (sm *SpriteManager) drawMushroomRingPlaceholder(img *ebiten.Image) {
	img.Fill(color.RGBA{101, 67, 33, 255}) // Brown dirt base
	// Add mushroom caps
	for i := 0; i < 4; i++ {
		x := 3 + i*3
		y := 6 + i%2
		img.Set(x, y, color.RGBA{139, 69, 19, 255})     // Brown cap
		img.Set(x, y+1, color.RGBA{255, 255, 255, 255}) // White stem
	}
}

func (sm *SpriteManager) drawMossRockPlaceholder(img *ebiten.Image) {
	img.Fill(color.RGBA{105, 105, 105, 255}) // Gray rock base
	// Add moss patches
	for y := 2; y < 6; y++ {
		for x := 2; x < 6; x++ {
			if (x+y)%3 == 0 {
				img.Set(x, y, color.RGBA{34, 139, 34, 255}) // Green moss
			}
		}
	}
}

func (sm *SpriteManager) drawFernPatchPlaceholder(img *ebiten.Image) {
	img.Fill(color.RGBA{34, 139, 34, 255}) // Green base
	// Add darker fern fronds
	for x := 2; x < 14; x += 2 {
		for y := 4; y < 12; y++ {
			img.Set(x, y, color.RGBA{0, 100, 0, 255}) // Dark green ferns
		}
	}
}

func (sm *SpriteManager) drawForestStreamPlaceholder(img *ebiten.Image) {
	img.Fill(color.RGBA{135, 206, 235, 255}) // Light blue water
	// Add ripple effects
	for x := 1; x < 15; x += 3 {
		img.Set(x, 6, color.RGBA{100, 149, 237, 255}) // Darker blue ripples
		img.Set(x, 9, color.RGBA{100, 149, 237, 255})
	}
}

func (sm *SpriteManager) drawFireflySwarmPlaceholder(img *ebiten.Image) {
	img.Fill(color.RGBA{25, 25, 112, 255}) // Dark blue night sky
	// Add glowing fireflies
	for i := 0; i < 8; i++ {
		x := 2 + (i*3)%12
		y := 2 + (i*2)%12
		img.Set(x, y, color.RGBA{255, 255, 0, 255}) // Yellow fireflies
	}
}

func (sm *SpriteManager) drawWaterPlaceholder(img *ebiten.Image) {
	img.Fill(color.RGBA{30, 144, 255, 255}) // Bright blue water
	// Add subtle water ripple pattern
	for y := 0; y < 16; y += 3 {
		for x := 0; x < 16; x += 4 {
			if (x+y)%6 == 0 {
				img.Set(x, y, color.RGBA{65, 169, 255, 255}) // Lighter blue ripples
			}
		}
	}
}
