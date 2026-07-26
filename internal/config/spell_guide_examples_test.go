package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every yaml example in the spell guide must pass the same validation the game
// runs at boot - the guide has already taught removed fields once.
func TestSpellGuideExamplesPassValidation(t *testing.T) {
	raw, err := os.ReadFile("../../how_to_add_a_new_spell.md")
	if err != nil {
		t.Fatalf("read guide: %v", err)
	}
	var blocks []string
	for _, chunk := range strings.Split(string(raw), "```yaml")[1:] {
		body, _, ok := strings.Cut(chunk, "```")
		if !ok {
			t.Fatal("unterminated yaml fence in the guide")
		}
		blocks = append(blocks, body)
	}
	if len(blocks) == 0 {
		t.Fatal("the guide lost its yaml examples")
	}
	for i, block := range blocks {
		path := filepath.Join(t.TempDir(), "example.yaml")
		if err := os.WriteFile(path, []byte(block), 0o644); err != nil {
			t.Fatalf("write example: %v", err)
		}
		if _, err := LoadSpellConfig(path); err != nil {
			t.Errorf("guide yaml example %d does not pass spell validation: %v", i+1, err)
		}
	}
}
