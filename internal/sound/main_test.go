package sound

import (
	"os"
	"testing"

	"ugataima/internal/config"
)

// The weapon catalog validates its status durations against the status table.
func TestMain(m *testing.M) {
	config.MustLoadStatuses("../../assets/statuses.yaml")
	os.Exit(m.Run())
}
