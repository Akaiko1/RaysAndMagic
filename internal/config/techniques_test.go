package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The shipped catalog loads, autocasts in kind order (not file order), and a
// technique that breaks its kind's contract fails the boot.
func TestLoadTechniquesKindContract(t *testing.T) {
	const path = "../../assets/techniques.yaml"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	saved := GlobalTechniques
	t.Cleanup(func() { GlobalTechniques = saved })
	if err := LoadTechniques(path); err != nil {
		t.Fatal(err)
	}
	var order []string
	for d := range AutomaticTechniques() {
		order = append(order, d.Kind)
	}
	if strings.Join(order, ",") != "purify,dodge,haste" {
		t.Fatalf("autocast order = %v, want purify, dodge, haste", order)
	}
	for _, tc := range []struct{ name, old, new, want string }{
		{"unknown kind", "kind: dodge\n", "kind: dodgy\n", "unknown kind"},
		{"second fold step", "kind: return_step\n", "kind: fold_step\n", "only one fold_step"},
		{"automatic step", "kind: fold_step\n", "kind: fold_step\n    automatic: true\n", "cannot autocast"},
		{"automatic without trigger", "trigger: \"Engaged, and either two enemies within 2 tiles or a living ally below 35% HP.\"", "trigger: \"\"", "trigger text is required"},
		{"step with cooldown", "kind: return_step\n", "kind: return_step\n    cooldown_seconds: 2\n", "cooldown_seconds unsupported"},
		{"ladder the kind ignores", "kind: purify\n", "kind: purify\n    power: [5, 5, 5, 5]\n", "does not use power"},
		{"removed free_step knob", "kind: return_step\n", "kind: return_step\n    free_step: true\n", "free_step"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := string(raw)
			if strings.Count(text, tc.old) != 1 {
				t.Fatalf("fixture: %q is not unique in the catalog", tc.old)
			}
			file := filepath.Join(t.TempDir(), "techniques.yaml")
			if err := os.WriteFile(file, []byte(strings.Replace(text, tc.old, tc.new, 1)), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := LoadTechniques(file); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
