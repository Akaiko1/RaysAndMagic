package monster

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	_ = MustLoadMonsterConfig("../../assets/monsters.yaml")
	os.Exit(m.Run())
}
