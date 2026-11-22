//go:build !windows

package gpupref

// PreferHighPerformance does nothing off Windows.
func PreferHighPerformance() (bool, error) { return false, nil }
