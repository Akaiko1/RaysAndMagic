package main

import (
	"fmt"
	"log"
	"ugataima/internal/world"
)

func main() {
	// Test the dynamic tile system
	fmt.Println("Testing Dynamic Tile System")
	fmt.Println("===========================")

	// Create tile manager and load config
	tm := world.NewTileManager()
	err := tm.LoadTileConfig("../assets/tiles.yaml")
	if err != nil {
		log.Fatalf("Failed to load tile config: %v", err)
	}

	// Test core tiles
	fmt.Println("\nCore Tiles:")
	if data := tm.GetTileData(world.TileEmpty); data != nil {
		fmt.Printf("TileEmpty -> %s (letter: '%s')\n", data.Name, data.Letter)
	}

	if data := tm.GetTileData(world.TileSpawn); data != nil {
		fmt.Printf("TileSpawn -> %s (letter: '%s')\n", data.Name, data.Letter)
	}

	// Test dynamic tile access by key
	fmt.Println("\nDynamic Tiles:")
	if data := tm.GetTileDataByKey("crystal"); data != nil {
		fmt.Printf("crystal -> %s (letter: '%s')\n", data.Name, data.Letter)

		// Get the dynamically assigned TileType3D
		if tileType, ok := tm.GetTileTypeFromKey("crystal"); ok {
			fmt.Printf("crystal has dynamic TileType3D: %d\n", tileType)
		}
	}

	// Test letter mappings for all tiles
	fmt.Println("\nLetter Mappings:")
	for letter, tileType := range tm.GetAllLetterMappings() {
		if data := tm.GetTileData(tileType); data != nil {
			fmt.Printf("'%s' -> %s (TileType3D: %d)\n", letter, data.Name, tileType)
		}
	}

	// Show all available tile keys
	fmt.Println("\nAll Available Tiles:")
	for _, key := range tm.GetAllTileKeys() {
		if data := tm.GetTileDataByKey(key); data != nil {
			fmt.Printf("- %s: %s\n", key, data.Name)
		}
	}
}
