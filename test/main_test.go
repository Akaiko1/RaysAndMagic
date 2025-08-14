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
	_ = config.MustLoadItemConfig("../assets/items.yaml")
	_ = config.MustLoadLootTables("../assets/loots.yaml")
	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()
	os.Exit(m.Run())
}
