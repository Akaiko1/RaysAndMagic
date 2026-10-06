package boot

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A missing or non-movement spell must fail the same startup used by both binaries.
func TestDeviceActionsFailAtBoot(t *testing.T) {
	if os.Getenv("RAM_TEST_INVALID_DEVICE_ACTION") == "1" {
		LoadGameData()
		return
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "assets/items.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "assets"))
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"missing_device_spell", "heal"} {
		t.Run(ref, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "assets"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(root, "config.yaml"), filepath.Join(dir, "config.yaml")); err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.Name() == "items.yaml" {
					continue
				}
				if err := os.Symlink(filepath.Join(root, "assets", entry.Name()), filepath.Join(dir, "assets", entry.Name())); err != nil {
					t.Fatal(err)
				}
			}
			changed := strings.Replace(string(raw), "use_spell: jump", "use_spell: "+ref, 1)
			if changed == string(raw) {
				t.Fatal("device fixture missing")
			}
			if err := os.WriteFile(filepath.Join(dir, "assets/items.yaml"), []byte(changed), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestDeviceActionsFailAtBoot$")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "RAM_TEST_INVALID_DEVICE_ACTION=1")
			output, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(output), "Device actions:") || !strings.Contains(string(output), ref) {
				t.Fatalf("invalid device reference was not rejected at boot: %v\n%s", err, output)
			}
		})
	}
}
