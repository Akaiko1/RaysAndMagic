package monster

import (
	"os"
	"testing"

	"ugataima/internal/config"
)

func TestMain(m *testing.M) {
	config.MustLoadStatuses("../../assets/statuses.yaml")
	_ = MustLoadMonsterConfig("../../assets/monsters.yaml")
	os.Exit(m.Run())
}
