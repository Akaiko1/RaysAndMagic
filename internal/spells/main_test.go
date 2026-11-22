package spells

import (
	"os"
	"testing"
	"ugataima/internal/config"
)

func TestMain(m *testing.M) {
	config.MustLoadStatuses("../../assets/statuses.yaml")
	_, _ = config.LoadSpellConfig("../../assets/spells.yaml")
	os.Exit(m.Run())
}
