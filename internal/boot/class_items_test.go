package boot

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassSuppliesFailAtBoot(t *testing.T) {
	if os.Getenv("RAM_TEST_INVALID_CLASS_SUPPLIES") == "1" {
		LoadGameData()
		return
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"missing_supply", "dawnleaf"} {
		t.Run(key, func(t *testing.T) {
			dir := t.TempDir()
			text := strings.Replace(string(raw), "key: brewed_health_potion", "key: "+key, 1)
			if text == string(raw) {
				t.Fatal("starting supply fixture missing")
			}
			if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(root, "assets"), filepath.Join(dir, "assets")); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestClassSuppliesFailAtBoot$")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "RAM_TEST_INVALID_CLASS_SUPPLIES=1")
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(output), "Class supplies:") || !strings.Contains(string(output), key) {
				t.Fatalf("invalid supply was not rejected at boot: %v\n%s", err, output)
			}
		})
	}
}
