package character

import (
	"os"
	"testing"
	"ugataima/internal/bridge"
	"ugataima/internal/config"
)

// TestMain sets up global config and weapon bridge for all tests in this package
func TestMain(m *testing.M) {
	// Load weapon config and set up bridge for items
	_ = config.MustLoadWeaponConfig("../../weapons.yaml")
	bridge.SetupWeaponBridge()
	os.Exit(m.Run())
}
