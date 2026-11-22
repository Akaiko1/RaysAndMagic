package world

import (
	"os"
	"testing"

	"ugataima/internal/config"
)

// Every content catalog validates its durations against the status table;
// map configs validate adventure techniques against the technique catalog.
func TestMain(m *testing.M) {
	config.MustLoadStatuses("../../assets/statuses.yaml")
	if err := config.LoadTechniques("../../assets/techniques.yaml"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
