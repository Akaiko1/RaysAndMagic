package spells

import (
	"os"
	"testing"
	"ugataima/internal/config"
)

func TestMain(m *testing.M) {
	_, _ = config.LoadSpellConfig("../../spells.yaml")
	os.Exit(m.Run())
}
