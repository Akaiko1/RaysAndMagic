package uitext

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"gopkg.in/yaml.v3"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// All production references must resolve at boot, including rarely reached
// service failures that a normal playthrough may never display.
func TestCatalogProductionReferences(t *testing.T) {
	used := map[string]bool{}
	err := filepath.WalkDir("../../internal", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		set := token.NewFileSet()
		file, err := parser.ParseFile(set, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Text" {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "uitext" {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				t.Errorf("%s: text key must be explicit", set.Position(call.Pos()))
				return true
			}
			key, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Error(err)
				return true
			}
			spec, known := signatures[key]
			if !known || len(spec) != len(call.Args)-1 {
				t.Errorf("%s: key %q or argument count is invalid", set.Position(call.Pos()), key)
			}
			used[key] = true
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for key := range signatures {
		if !used[key] {
			t.Errorf("unused text key %q", key)
		}
	}
}

func TestCatalogTemplates(t *testing.T) {
	for key, spec := range signatures {
		t.Run(key, func(t *testing.T) {
			var args []any
			for _, verb := range spec {
				switch verb {
				case 'd':
					args = append(args, 7)
				case 's':
					args = append(args, "Example")
				case 'f':
					args = append(args, 1.25)
				}
			}
			line := Text(key, args...)
			if strings.TrimSpace(line) == "" || strings.Contains(line, "%!") {
				t.Fatalf("invalid rendered template: %q", line)
			}
		})
	}
}

func TestCatalogLoadValidationAndPublication(t *testing.T) {
	original := active.Load()
	t.Cleanup(func() { active.Store(original) })
	for _, tc := range []struct {
		name   string
		mutate func(catalog)
	}{
		{"valid", func(c catalog) { c["dialog.back"] = "Return" }},
		{"missing", func(c catalog) { delete(c, "dialog.back") }},
		{"unknown", func(c catalog) { c["unknown"] = "Extra" }},
		{"blank", func(c catalog) { c["dialog.back"] = "  " }},
		{"non ASCII", func(c catalog) { c["dialog.back"] = "Back\u2014" }},
		{"wrong argument type", func(c catalog) { c["dialog.party_gold"] = "Gold: %s" }},
		{"missing argument", func(c catalog) { c["dialog.party_gold"] = "Gold" }},
		{"extra argument", func(c catalog) { c["dialog.back"] = "Back %d" }},
		{"unsupported format", func(c catalog) { c["dialog.party_gold"] = "Gold: %[1]d" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := readCatalog(bundled)
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(c)
			data, err := yaml.Marshal(c)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "text.yaml"), data, 0600); err != nil {
				t.Fatal(err)
			}
			before := active.Load()
			err = LoadDirectory(dir)
			if tc.name == "valid" {
				if err != nil {
					t.Fatal(err)
				}
				if Text("dialog.back") != "Return" {
					t.Fatal("external wording not used")
				}
			} else if err == nil || active.Load() != before {
				t.Fatalf("invalid catalog was published: %v", err)
			}
		})
	}
}

func TestCatalogRejectsDuplicateKeys(t *testing.T) {
	data, err := yaml.Marshal(*active.Load())
	if err != nil {
		t.Fatal(err)
	}
	for _, crossFile := range []bool{false, true} {
		t.Run(fmt.Sprint(crossFile), func(t *testing.T) {
			dir := t.TempDir()
			body := append([]byte(nil), data...)
			if crossFile {
				if err := os.WriteFile(filepath.Join(dir, "duplicate.yaml"), []byte("dialog.back: Return\n"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				body = append(body, []byte("dialog.back: Return\n")...)
			}
			if err := os.WriteFile(filepath.Join(dir, "text.yaml"), body, 0600); err != nil {
				t.Fatal(err)
			}
			before := active.Load()
			if err := LoadDirectory(dir); err == nil || before != active.Load() {
				t.Fatal("duplicate catalog published")
			}
		})
	}
}
