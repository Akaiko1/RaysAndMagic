// Package testutil contains fixtures shared by tests across internal packages.
package testutil

import "ugataima/internal/config"

// UniformVisualSizeClasses returns the complete visual-size class table with
// every class mapped to value.
func UniformVisualSizeClasses(value float64) map[string]float64 {
	classes := make(map[string]float64)
	for _, name := range config.VisualSizeClassNames() {
		classes[name] = value
	}
	return classes
}
