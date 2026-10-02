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

func TestTestProcessSavesNeverLandInTheWorkingDirectory(t *testing.T) {
	oldData, oldScenario := dataRoot, scenarioRoot
	t.Cleanup(func() { dataRoot, scenarioRoot = oldData, oldScenario })
	cwd := t.TempDir()
	t.Chdir(cwd)
	dataRoot, scenarioRoot = "", ""
	if got := AppSaveDir(); filepath.Dir(got) != testSaveRoot() {
		t.Fatalf("default test save dir = %q, want under %q", got, testSaveRoot())
	}
	if _, err := os.Stat(filepath.Join(cwd, savesDirName)); !os.IsNotExist(err) {
		t.Fatalf("test process created saves in its working directory: %v", err)
	}
	explicit := t.TempDir()
	dataRoot = explicit
	if got := AppSaveDir(); filepath.Dir(got) != explicit {
		t.Fatalf("explicit root ignored: %q", got)
	}
}
