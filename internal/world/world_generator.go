package world

import (
	"math/rand"
)

// generateElvishForest creates a procedural elvish forest
func (w *World3D) generateElvishForest() {
	// Start with base forest coverage
	w.generateBaseForest()

	// Add special features in layers
	w.addAncientTrees()
	w.addMagicalFeatures()
	w.addNaturalFormations()
	w.createClearings()
	w.addWaterFeatures()
	w.addUndergrowth()
	w.createPaths()
}

// generateBaseForest creates the basic forest structure
func (w *World3D) generateBaseForest() {
	// Fill with trees using clustered placement
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			// Use noise-like clustering for natural tree placement
			clusterValue := float64(x+y*133) * 0.123 // Simple pseudo-noise
			clusterValue -= float64(int(clusterValue))

			if clusterValue > 0.6 {
				w.Tiles[y][x] = TileTree
			} else {
				w.Tiles[y][x] = TileEmpty
			}
		}
	}
}

// addAncientTrees places large ancient trees as landmarks
func (w *World3D) addAncientTrees() {
	ancientTreeCount := 8 + rand.Intn(5)

	for i := 0; i < ancientTreeCount; i++ {
		x := rand.Intn(w.Width)
		y := rand.Intn(w.Height)

		// Ensure spacing between ancient trees
		if w.isAreaClear(x, y, 3, TileAncientTree) {
			w.Tiles[y][x] = TileAncientTree

			// Add protective ring of regular trees
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx, ny := x+dx, y+dy
					if w.isValidCoordinate(nx, ny) && w.Tiles[ny][nx] == TileEmpty {
						if rand.Float64() < 0.7 {
							w.Tiles[ny][nx] = TileTree
						}
					}
				}
			}
		}
	}
}

// addMagicalFeatures places mushroom rings and firefly swarms
func (w *World3D) addMagicalFeatures() {
	// Mushroom rings
	mushroomRings := 5 + rand.Intn(4)
	for i := 0; i < mushroomRings; i++ {
		x := rand.Intn(w.Width)
		y := rand.Intn(w.Height)
		if w.Tiles[y][x] == TileEmpty {
			w.Tiles[y][x] = TileMushroomRing
		}
	}

	// Firefly swarms
	fireflySwarms := 6 + rand.Intn(5)
	for i := 0; i < fireflySwarms; i++ {
		x := rand.Intn(w.Width)
		y := rand.Intn(w.Height)
		if w.Tiles[y][x] == TileEmpty {
			w.Tiles[y][x] = TileFireflySwarm
		}
	}
}

// addNaturalFormations places moss rocks and other natural features
func (w *World3D) addNaturalFormations() {
	mossRocks := 15 + rand.Intn(10)
	for i := 0; i < mossRocks; i++ {
		x := rand.Intn(w.Width)
		y := rand.Intn(w.Height)
		if w.Tiles[y][x] == TileEmpty {
			w.Tiles[y][x] = TileMossRock
		}
	}
}

// createClearings makes open areas in the forest
func (w *World3D) createClearings() {
	clearingCount := 3 + rand.Intn(3)

	for i := 0; i < clearingCount; i++ {
		centerX := 5 + rand.Intn(w.Width-10)
		centerY := 5 + rand.Intn(w.Height-10)
		radius := 2 + rand.Intn(3)

		// Clear circular area
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				if dx*dx+dy*dy <= radius*radius {
					x, y := centerX+dx, centerY+dy
					if w.isValidCoordinate(x, y) {
						w.Tiles[y][x] = TileClearing
					}
				}
			}
		}
	}
}

// addWaterFeatures creates streams and water bodies
func (w *World3D) addWaterFeatures() {
	// Create 1-2 streams
	streamCount := 1 + rand.Intn(2)

	for i := 0; i < streamCount; i++ {
		// Random starting point on edge
		var startX, startY, endX, endY int

		if rand.Float64() < 0.5 {
			// Horizontal stream
			startX = 0
			startY = rand.Intn(w.Height)
			endX = w.Width - 1
			endY = rand.Intn(w.Height)
		} else {
			// Vertical stream
			startX = rand.Intn(w.Width)
			startY = 0
			endX = rand.Intn(w.Width)
			endY = w.Height - 1
		}

		// Create meandering path
		steps := 20 + rand.Intn(20)
		for step := 0; step <= steps; step++ {
			t := float64(step) / float64(steps)
			x := int(float64(startX)*(1-t) + float64(endX)*t)
			y := int(float64(startY)*(1-t) + float64(endY)*t)

			// Add some randomness
			x += rand.Intn(3) - 1
			y += rand.Intn(3) - 1

			if w.isValidCoordinate(x, y) {
				w.Tiles[y][x] = TileForestStream
			}
		}
	}
}

// addUndergrowth places fern patches and thickets
func (w *World3D) addUndergrowth() {
	fernPatches := 20 + rand.Intn(15)
	for i := 0; i < fernPatches; i++ {
		x := rand.Intn(w.Width)
		y := rand.Intn(w.Height)
		if w.Tiles[y][x] == TileEmpty {
			w.Tiles[y][x] = TileFernPatch
		}
	}

	thickets := 8 + rand.Intn(6)
	for i := 0; i < thickets; i++ {
		x := rand.Intn(w.Width)
		y := rand.Intn(w.Height)
		if w.Tiles[y][x] == TileEmpty {
			w.Tiles[y][x] = TileThicket
		}
	}
}

// createPaths makes walkable routes through the forest
func (w *World3D) createPaths() {
	// Create a few winding paths
	pathCount := 2 + rand.Intn(2)

	for i := 0; i < pathCount; i++ {
		startX := rand.Intn(w.Width)
		startY := rand.Intn(w.Height)

		// Create winding path
		currentX, currentY := startX, startY
		pathLength := 15 + rand.Intn(25)

		for j := 0; j < pathLength; j++ {
			if w.isValidCoordinate(currentX, currentY) {
				if w.Tiles[currentY][currentX] == TileTree || w.Tiles[currentY][currentX] == TileThicket {
					w.Tiles[currentY][currentX] = TileEmpty
				}
			}

			// Random walk
			direction := rand.Intn(4)
			switch direction {
			case 0:
				currentX++
			case 1:
				currentX--
			case 2:
				currentY++
			case 3:
				currentY--
			}
		}
	}
}

// Helper methods
func (w *World3D) isValidCoordinate(x, y int) bool {
	return x >= 0 && x < w.Width && y >= 0 && y < w.Height
}

func (w *World3D) isAreaClear(centerX, centerY, radius int, tileType TileType3D) bool {
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			x, y := centerX+dx, centerY+dy
			if w.isValidCoordinate(x, y) {
				if w.Tiles[y][x] == tileType {
					return false
				}
			}
		}
	}
	return true
}
