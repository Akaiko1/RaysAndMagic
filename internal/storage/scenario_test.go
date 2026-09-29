package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScenarioDirectoryNeverFallsBackToNormalSaves(t *testing.T) {
	old := scenarioRoot
	t.Cleanup(func() { scenarioRoot = old })
	scenarioRoot = filepath.Join(t.TempDir(), "scenario")
	normal := dataRoot
	got := appDataRoots()
	if len(got) != 1 || got[0] != scenarioRoot || (normal != "" && got[0] == normal) {
		t.Fatalf("scenario roots %v", got)
	}
	// Even an unusable directory must not redirect a scenario to normal saves.
	if err := os.WriteFile(scenarioRoot, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := AppSavePath("save1.json"); got != filepath.Join(scenarioRoot, "saves", "save1.json") {
		t.Fatal("fallback escaped isolation", got)
	}
	for _, key := range []string{"", "../real", "test/run", "test\\run"} {
		if err := UseTestScenarioDirectory(key); err == nil {
			t.Fatal("accepted unsafe key", key)
		}
	}
}
