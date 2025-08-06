package world

import (
	"math/rand"
	"sync"
	"ugataima/internal/threading/core"
)

// ParallelWorldGenerator handles multi-threaded world generation
type ParallelWorldGenerator struct {
	workerPool *core.WorkerPool
	config     interface{} // Will be *config.Config from config package
}

// NewParallelWorldGenerator creates a new parallel world generator
func NewParallelWorldGenerator(config interface{}) *ParallelWorldGenerator {
	return &ParallelWorldGenerator{
		workerPool: core.CreateDefaultWorkerPool(),
		config:     config,
	}
}

// GenerateTerrainParallel generates terrain in parallel chunks
func (pwg *ParallelWorldGenerator) GenerateTerrainParallel(width, height int,
	chunkSize int, generateChunk func(startX, startY, chunkWidth, chunkHeight int) [][]interface{}) [][]interface{} {

	// Initialize result grid
	tiles := make([][]interface{}, height)
	for y := 0; y < height; y++ {
		tiles[y] = make([]interface{}, width)
	}

	var resultMutex sync.RWMutex

	// Generate chunks in parallel
	for y := 0; y < height; y += chunkSize {
		for x := 0; x < width; x += chunkSize {
			chunkWidth := min(chunkSize, width-x)
			chunkHeight := min(chunkSize, height-y)

			pwg.workerPool.Submit(func() {
				chunkResult := generateChunk(x, y, chunkWidth, chunkHeight)

				// Copy chunk result to main grid
				resultMutex.Lock()
				for cy := 0; cy < chunkHeight; cy++ {
					for cx := 0; cx < chunkWidth; cx++ {
						if cy < len(chunkResult) && cx < len(chunkResult[cy]) {
							tiles[y+cy][x+cx] = chunkResult[cy][cx]
						}
					}
				}
				resultMutex.Unlock()
			})
		}
	}

	pwg.workerPool.Wait()
	return tiles
}

// GenerateTreeClustersParallel generates tree clusters in parallel
func (pwg *ParallelWorldGenerator) GenerateTreeClustersParallel(tiles [][]interface{},
	numClusters int, clusterSize func() int, placementFunc func(x, y int) bool) {

	// var tilesMutex sync.RWMutex // Uncomment when tile modifications are implemented
	height := len(tiles)
	width := len(tiles[0])

	// Generate clusters in parallel
	for i := 0; i < numClusters; i++ {
		pwg.workerPool.Submit(func() {
			centerX := rand.Intn(width-10) + 5
			centerY := rand.Intn(height-10) + 5
			size := clusterSize()

			// Don't place too close to starting position
			startX, startY := width/2, height/2
			if abs(centerX-startX) < 5 && abs(centerY-startY) < 5 {
				return
			}

			// Generate cluster
			for j := 0; j < size; j++ {
				x := centerX + rand.Intn(6) - 3
				y := centerY + rand.Intn(6) - 3

				if x > 0 && x < width-1 && y > 0 && y < height-1 {
					if placementFunc(x, y) {
						// TODO: Implement tile placement logic
						// tilesMutex.Lock()
						// Place tree (specific tile type would be set here)
						// tiles[y][x] = TileTree
						// tilesMutex.Unlock()
					}
				}
			}
		})
	}

	pwg.workerPool.Wait()
}

// FeaturePlacementJob represents a feature placement task
type FeaturePlacementJob struct {
	X, Y        int
	FeatureType string
	Placed      bool
}

// PlaceFeaturesParallel places features like mushroom rings, rocks, etc. in parallel
func (pwg *ParallelWorldGenerator) PlaceFeaturesParallel(tiles [][]interface{},
	features []FeaturePlacementJob, validationFunc func(x, y int, featureType string) bool) {

	// var tilesMutex sync.RWMutex // Uncomment when tile modifications are implemented

	// Process features in parallel batches
	core.ParallelForEach(features, func(feature FeaturePlacementJob) {
		if validationFunc(feature.X, feature.Y, feature.FeatureType) {
			// TODO: Implement actual feature placement
			// tilesMutex.Lock()
			// Place the feature (specific tile type would be set based on feature type)
			// tiles[feature.Y][feature.X] = getFeatureTileType(feature.FeatureType)
			// tilesMutex.Unlock()
		}
	})
}

// PathGenerationJob represents a path generation task
type PathGenerationJob struct {
	StartX, StartY int
	EndX, EndY     int
	Width          int
	PathType       string
}

// GeneratePathsParallel generates paths in parallel
func (pwg *ParallelWorldGenerator) GeneratePathsParallel(tiles [][]interface{},
	paths []PathGenerationJob) {

	// var tilesMutex sync.RWMutex // Uncomment when tile modifications are implemented

	// Generate each path in parallel
	core.ParallelForEach(paths, func(path PathGenerationJob) {
		// Simple line drawing algorithm
		dx := abs(path.EndX - path.StartX)
		dy := abs(path.EndY - path.StartY)
		sx := -1
		if path.StartX < path.EndX {
			sx = 1
		}
		sy := -1
		if path.StartY < path.EndY {
			sy = 1
		}
		err := dx - dy

		x, y := path.StartX, path.StartY

		for {
			// Clear area around path
			for py := y - path.Width/2; py <= y+path.Width/2; py++ {
				for px := x - path.Width/2; px <= x+path.Width/2; px++ {
					if px >= 0 && px < len(tiles[0]) && py >= 0 && py < len(tiles) {
						// TODO: Implement path clearing logic
						// tilesMutex.Lock()
						// tiles[py][px] = TileEmpty
						// tilesMutex.Unlock()
					}
				}
			}

			if x == path.EndX && y == path.EndY {
				break
			}

			e2 := 2 * err
			if e2 > -dy {
				err -= dy
				x += sx
			}
			if e2 < dx {
				err += dx
				y += sy
			}
		}
	})
}

// WaterFeatureJob represents a water feature generation task
type WaterFeatureJob struct {
	StartX, StartY int
	Type           string // "stream", "pond", "river"
	Size           int
	Direction      float64 // For streams
}

// GenerateWaterFeaturesParallel generates water features in parallel
func (pwg *ParallelWorldGenerator) GenerateWaterFeaturesParallel(tiles [][]interface{},
	waterFeatures []WaterFeatureJob) {

	// var tilesMutex sync.RWMutex // Uncomment when tile modifications are implemented
	height := len(tiles)
	width := len(tiles[0])

	core.ParallelForEach(waterFeatures, func(feature WaterFeatureJob) {
		switch feature.Type {
		case "stream":
			// Generate winding stream
			streamY := feature.StartY
			for x := feature.StartX; x < width-2; x++ {
				// Create winding pattern
				streamY += rand.Intn(3) - 1
				if streamY < 2 {
					streamY = 2
				}
				if streamY > height-3 {
					streamY = height - 3
				}

				// tilesMutex.Lock()
				// tiles[streamY][x] = TileWater
				// Clear banks
				// if tiles[streamY-1][x] == TileTree { tiles[streamY-1][x] = TileEmpty }
				// if tiles[streamY+1][x] == TileTree { tiles[streamY+1][x] = TileEmpty }
				// tilesMutex.Unlock()
			}

		case "pond":
			// Generate circular pond
			for py := feature.StartY; py < feature.StartY+feature.Size && py < height; py++ {
				for px := feature.StartX; px < feature.StartX+feature.Size && px < width; px++ {
					// tilesMutex.Lock()
					// tiles[py][px] = TileWater
					// tilesMutex.Unlock()
				}
			}
		}
	})
}

// BiomeGenerationJob represents a biome generation task
type BiomeGenerationJob struct {
	StartX, StartY int
	Width, Height  int
	BiomeType      string
	Density        float64
}

// GenerateBiomeFeaturesParallel generates biome-specific features in parallel
func (pwg *ParallelWorldGenerator) GenerateBiomeFeaturesParallel(tiles [][]interface{},
	biomes []BiomeGenerationJob) {

	// var tilesMutex sync.RWMutex // Uncomment when tile modifications are implemented

	core.ParallelForEach(biomes, func(biome BiomeGenerationJob) {
		for y := biome.StartY; y < biome.StartY+biome.Height; y++ {
			for x := biome.StartX; x < biome.StartX+biome.Width; x++ {
				if x >= 0 && x < len(tiles[0]) && y >= 0 && y < len(tiles) {
					if rand.Float64() < biome.Density {
						// tilesMutex.Lock()
						// Place biome-specific features
						switch biome.BiomeType {
						case "forest":
							// tiles[y][x] = TileTree
						case "desert":
							// tiles[y][x] = TileSand
						case "swamp":
							// tiles[y][x] = TileSwamp
						}
						// tilesMutex.Unlock()
					}
				}
			}
		}
	})
}

// Stop shuts down the parallel world generator
func (pwg *ParallelWorldGenerator) Stop() {
	pwg.workerPool.Stop()
}
