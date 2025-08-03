package game

import (
	"math"
)

// Camera helper methods for the FirstPersonCamera
// These provide cleaner movement calculations

// GetForwardX returns the X component of the forward direction vector
func (c *FirstPersonCamera) GetForwardX() float64 {
	return math.Cos(c.Angle)
}

// GetForwardY returns the Y component of the forward direction vector
func (c *FirstPersonCamera) GetForwardY() float64 {
	return math.Sin(c.Angle)
}

// GetRightX returns the X component of the right direction vector
func (c *FirstPersonCamera) GetRightX() float64 {
	return math.Cos(c.Angle + math.Pi/2)
}

// GetRightY returns the Y component of the right direction vector
func (c *FirstPersonCamera) GetRightY() float64 {
	return math.Sin(c.Angle + math.Pi/2)
}

// GetPosition returns the camera's current position
func (c *FirstPersonCamera) GetPosition() (float64, float64) {
	return c.X, c.Y
}

// SetPosition sets the camera's position
func (c *FirstPersonCamera) SetPosition(x, y float64) {
	c.X = x
	c.Y = y
}

// Rotate rotates the camera by the given angle
func (c *FirstPersonCamera) Rotate(angle float64) {
	c.Angle += angle
}

// GetViewDirection returns the current view direction as a normalized vector
func (c *FirstPersonCamera) GetViewDirection() (float64, float64) {
	return c.GetForwardX(), c.GetForwardY()
}
