package test

import (
	"os"
	"testing"
	"ugataima/internal/bridge"
	"ugataima/internal/config"
)

// TestMain sets up global config and weapon bridge for all integration tests
func TestMain(m *testing.M) {
	_ = config.MustLoadWeaponConfig("../weapons.yaml")
	bridge.SetupWeaponBridge()
	os.Exit(m.Run())
}
