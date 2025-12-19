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
	_ = config.MustLoadWeaponConfig("../../assets/weapons.yaml")
	_ = config.MustLoadItemConfig("../../assets/items.yaml")
	_ = config.MustLoadLootTables("../../assets/loots.yaml")
	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()
	os.Exit(m.Run())
}
