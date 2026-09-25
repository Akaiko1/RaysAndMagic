package game

import (
	"math"
	"sort"
)

// A conservative broad phase only: existing projection and exact-distance
// tests remain authoritative. Keeping original index order preserves painter
// ties. This index contains integers, not images or duplicated sprite data.
type renderSpatialIndex struct {
	source     []TransparentSpriteData
	buckets    map[[2]int][]int
	cellSize   float64
	candidates []int
}

func (s *renderSpatialIndex) rebuild(source []TransparentSpriteData, tileSize float64) {
	s.source = source
	s.cellSize = math.Max(1, tileSize*16)
	s.buckets = make(map[[2]int][]int)
	for i, item := range source {
		key := [2]int{int(math.Floor(item.worldX / s.cellSize)), int(math.Floor(item.worldY / s.cellSize))}
		s.buckets[key] = append(s.buckets[key], i)
	}
	s.candidates = make([]int, 0, len(source))
}

func (s *renderSpatialIndex) query(source []TransparentSpriteData, x, y, radius, tileSize float64) []int {
	// Production rebuilds explicitly on tile/world changes. Identity checking
	// also supports diagnostic category swaps and lazily constructed renderers.
	if s.buckets == nil || len(s.source) != len(source) ||
		len(source) > 0 && &s.source[0] != &source[0] || s.cellSize != math.Max(1, tileSize*16) {
		s.rebuild(source, tileSize)
	}
	out := s.candidates[:0]
	// A wall-mounted standee may move from its tile centre to an adjacent wall.
	radius = math.Max(0, radius) + tileSize
	x0, x1 := int(math.Floor((x-radius)/s.cellSize)), int(math.Floor((x+radius)/s.cellSize))
	y0, y1 := int(math.Floor((y-radius)/s.cellSize)), int(math.Floor((y+radius)/s.cellSize))
	for cy := y0; cy <= y1; cy++ {
		for cx := x0; cx <= x1; cx++ {
			out = append(out, s.buckets[[2]int{cx, cy}]...)
		}
	}
	sort.Ints(out)
	s.candidates = out
	return out
}
