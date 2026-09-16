// Package uitext owns the shared player-facing text templates.
package uitext

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

// The same authored files are embedded for standalone formatters and loaded
// from the asset directory at startup so installed content remains editable.
//
//go:embed *.yaml
var bundled embed.FS

type catalog map[string]string

var active atomic.Pointer[catalog]

func init() {
	c, err := readCatalog(bundled)
	if err != nil {
		panic(err)
	}
	active.Store(&c)
}

// LoadDirectory validates the complete catalog before publishing a new one.
func LoadDirectory(path string) error {
	c, err := readCatalog(os.DirFS(path))
	if err != nil {
		return err
	}
	active.Store(&c)
	return nil
}

// Text formats a known template. Wording is never used as a content key.
func Text(key string, args ...any) string {
	spec, known := signatures[key]
	if !known || len(args) != len(spec) {
		panic(fmt.Sprintf("UI text %q: unexpected argument count %d", key, len(args)))
	}
	line := (*active.Load())[key]
	if len(args) == 0 && !strings.Contains(line, "%") {
		return line
	}
	return fmt.Sprintf(line, args...)
}

var placeholder = regexp.MustCompile(`^%[-+# 0]*[0-9]*(\.[0-9]+)?[dsf]`)

func formatSignature(line string) (string, error) {
	var signature strings.Builder
	for i := 0; i < len(line); i++ {
		if line[i] != '%' {
			continue
		}
		if i+1 < len(line) && line[i+1] == '%' {
			i++
			continue
		}
		match := placeholder.FindString(line[i:])
		if match == "" {
			return "", fmt.Errorf("unsupported placeholder near %q", line[i:])
		}
		signature.WriteByte(match[len(match)-1])
		i += len(match) - 1
	}
	return signature.String(), nil
}

func readCatalog(root fs.FS) (catalog, error) {
	result := catalog{}
	paths, err := fs.Glob(root, "*.yaml")
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		data, err := fs.ReadFile(root, path)
		if err != nil {
			return nil, err
		}
		var entries map[string]string
		if err := yaml.Unmarshal(data, &entries); err != nil {
			return nil, fmt.Errorf("UI text %s: %w", path, err)
		}
		for key, line := range entries {
			spec, known := signatures[key]
			if !known {
				return nil, fmt.Errorf("UI text %s: unknown key %q", path, key)
			}
			if _, duplicate := result[key]; duplicate {
				return nil, fmt.Errorf("UI text %s: duplicate key %q", path, key)
			}
			if strings.TrimSpace(line) == "" {
				return nil, fmt.Errorf("UI text %q: text must not be blank", key)
			}
			for _, r := range line {
				if r > 127 {
					return nil, fmt.Errorf("UI text %q: text must be ASCII", key)
				}
			}
			got, err := formatSignature(line)
			if err != nil || got != spec {
				return nil, fmt.Errorf("UI text %q: expected format arguments %q, got %q (%v)", key, spec, got, err)
			}
			result[key] = line
		}
	}
	for key := range signatures {
		if _, ok := result[key]; !ok {
			return nil, fmt.Errorf("UI text: missing key %q", key)
		}
	}
	return result, nil
}
