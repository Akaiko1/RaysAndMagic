package collision

import (
	"math"
)

// BoundingBox represents a rectangular collision boundary
type BoundingBox struct {
	X      float64 // Center X coordinate
	Y      float64 // Center Y coordinate
	Width  float64 // Total width
	Height float64 // Total height
}

// NewBoundingBox creates a new bounding box centered at the given position
func NewBoundingBox(x, y, width, height float64) *BoundingBox {
	return &BoundingBox{
		X:      x,
		Y:      y,
		Width:  width,
		Height: height,
	}
}

// GetBounds returns the min/max coordinates of the bounding box
func (bb *BoundingBox) GetBounds() (minX, minY, maxX, maxY float64) {
	halfWidth := bb.Width / 2
	halfHeight := bb.Height / 2

	minX = bb.X - halfWidth
	maxX = bb.X + halfWidth
	minY = bb.Y - halfHeight
	maxY = bb.Y + halfHeight

	return minX, minY, maxX, maxY
}

// GetCorners returns all four corners of the bounding box
func (bb *BoundingBox) GetCorners() []Point {
	minX, minY, maxX, maxY := bb.GetBounds()

	return []Point{
		{X: minX, Y: minY}, // Top-left
		{X: maxX, Y: minY}, // Top-right
		{X: minX, Y: maxY}, // Bottom-left
		{X: maxX, Y: maxY}, // Bottom-right
	}
}

// GetEdgePoints returns points along the edges of the bounding box for detailed collision
func (bb *BoundingBox) GetEdgePoints(density int) []Point {
	minX, minY, maxX, maxY := bb.GetBounds()
	var points []Point

	// Top and bottom edges
	for i := 0; i <= density; i++ {
		x := minX + (maxX-minX)*float64(i)/float64(density)
		points = append(points, Point{X: x, Y: minY}) // Top edge
		points = append(points, Point{X: x, Y: maxY}) // Bottom edge
	}

	// Left and right edges (excluding corners already added)
	for i := 1; i < density; i++ {
		y := minY + (maxY-minY)*float64(i)/float64(density)
		points = append(points, Point{X: minX, Y: y}) // Left edge
		points = append(points, Point{X: maxX, Y: y}) // Right edge
	}

	// Center point
	points = append(points, Point{X: bb.X, Y: bb.Y})

	return points
}

// Intersects checks if this bounding box intersects with another
func (bb *BoundingBox) Intersects(other *BoundingBox) bool {
	minX1, minY1, maxX1, maxY1 := bb.GetBounds()
	minX2, minY2, maxX2, maxY2 := other.GetBounds()

	return !(maxX1 < minX2 || maxX2 < minX1 || maxY1 < minY2 || maxY2 < minY1)
}

// Contains checks if a point is inside the bounding box
func (bb *BoundingBox) Contains(point Point) bool {
	minX, minY, maxX, maxY := bb.GetBounds()
	return point.X >= minX && point.X <= maxX && point.Y >= minY && point.Y <= maxY
}

// MoveTo moves the bounding box to a new center position
func (bb *BoundingBox) MoveTo(x, y float64) {
	bb.X = x
	bb.Y = y
}

// MoveBy moves the bounding box by the given offset
func (bb *BoundingBox) MoveBy(dx, dy float64) {
	bb.X += dx
	bb.Y += dy
}

// Distance returns the distance between the centers of two bounding boxes
func (bb *BoundingBox) Distance(other *BoundingBox) float64 {
	dx := bb.X - other.X
	dy := bb.Y - other.Y
	return math.Sqrt(dx*dx + dy*dy)
}

// DistanceToPoint returns the distance from the center to a point
func (bb *BoundingBox) DistanceToPoint(point Point) float64 {
	dx := bb.X - point.X
	dy := bb.Y - point.Y
	return math.Sqrt(dx*dx + dy*dy)
}

// Point represents a 2D coordinate
type Point struct {
	X, Y float64
}

// CollisionType represents different types of collision boundaries
type CollisionType int

const (
	CollisionTypePlayer CollisionType = iota
	CollisionTypeMonster
	CollisionTypeMonsterEngaged
	CollisionTypeProjectile
	CollisionTypeTile
	CollisionTypeNPC
	CollisionTypeItem
)

// Entity represents any game object that can have collisions
type Entity struct {
	BoundingBox   *BoundingBox
	CollisionType CollisionType
	ID            string
	Solid         bool // Whether this entity blocks movement
}

// NewEntity creates a new collision entity
func NewEntity(id string, x, y, width, height float64, collisionType CollisionType, solid bool) *Entity {
	return &Entity{
		BoundingBox:   NewBoundingBox(x, y, width, height),
		CollisionType: collisionType,
		ID:            id,
		Solid:         solid,
	}
}
