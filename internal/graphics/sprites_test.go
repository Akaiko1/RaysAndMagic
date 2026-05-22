package graphics

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGetSpriteVariants(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected []string
	}{
		{
			name:     "base only",
			files:    []string{"grass.png"},
			expected: []string{"grass"},
		},
		{
			name:     "stops at first missing numbered variant",
			files:    []string{"grass.png", "grass3.png", "grass128.png"},
			expected: []string{"grass"},
		},
		{
			name:     "uses continuous numbered variants",
			files:    []string{"grass.png", "grass0.png", "grass1.png", "grass2.png"},
			expected: []string{"grass", "grass0", "grass1", "grass2"},
		},
		{
			name:     "ignores variants after a gap",
			files:    []string{"grass.png", "grass0.png", "grass2.png"},
			expected: []string{"grass", "grass0"},
		},
	}

	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalWD); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			spriteDir := filepath.Join(tempDir, "assets", "sprites", "environment")
			if err := os.MkdirAll(spriteDir, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, file := range tt.files {
				if err := os.WriteFile(filepath.Join(spriteDir, file), []byte{}, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chdir(tempDir); err != nil {
				t.Fatal(err)
			}

			sm := NewSpriteManager()
			got := sm.GetSpriteVariants("grass")
			if !reflect.DeepEqual(got, tt.expected) {
				t.Fatalf("variants = %v, want %v", got, tt.expected)
			}
		})
	}
}
