// Package gpupref asks Windows to run the game on the high-performance GPU of
// a hybrid laptop. Drivers honour exported NvOptimusEnablement and
// AmdPowerXpressRequestHighPerformance symbols, which a cgo-free build cannot
// export; the per-application entry of Settings > Display > Graphics is the
// remaining switch, and Windows reads it when the process creates its device.
package gpupref

// highPerformance is the Graphics settings value for "High performance".
const highPerformance = "GpuPreference=2;"

// choice is what to store for the executable, given the entry Graphics
// settings already hold. The player's own choice, whatever it is, stays.
func choice(found bool) (value string, store bool) {
	if found {
		return "", false
	}
	return highPerformance, true
}
