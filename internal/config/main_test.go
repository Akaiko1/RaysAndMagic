package config

import (
	"os"
	"testing"
)

// Every content catalog validates its durations against the status table.
func TestMain(m *testing.M) {
	MustLoadStatuses("../../assets/statuses.yaml")
	os.Exit(m.Run())
}
