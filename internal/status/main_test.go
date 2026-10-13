package status

import (
	"os"
	"testing"

	"ugataima/internal/config"
)

// Every content catalog validates its durations against the status table.
func TestMain(m *testing.M) {
	config.MustLoadStatuses("../../assets/statuses.yaml")
	os.Exit(m.Run())
}
