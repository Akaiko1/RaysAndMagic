package uitext

import (
	"fmt"
	"go/ast"
	"go/types"
	"golang.org/x/tools/go/packages"
	"gopkg.in/yaml.v3"
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
	pkgs, err := packages.Load(&packages.Config{
		Dir:  "../..",
		Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
	}, "./internal/...", "./assets/map_viewer/...")
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range pkgs {
		if len(pkg.Errors) != 0 {
			t.Fatalf("load %s: %v", pkg.PkgPath, pkg.Errors)
		}
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Text" {
					return true
				}
				fn, ok := pkg.TypesInfo.Uses[sel.Sel].(*types.Func)
				if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "ugataima/assets/text" {
					return true
				}
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok {
					t.Errorf("%s: text key must be explicit", pkg.Fset.Position(call.Pos()))
					return true
				}
				key, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Error(err)
					return true
				}
				spec, known := contract[key]
				if !known || len(spec) != len(call.Args)-1 {
					t.Errorf("%s: key %q or argument count is invalid", pkg.Fset.Position(call.Pos()), key)
				} else {
					for i, verb := range []byte(spec) {
						typ := pkg.TypesInfo.TypeOf(call.Args[i+1])
						if !textArgumentMatches(typ, verb) {
							t.Errorf("%s: %q argument %d has type %v, incompatible with %%%c", pkg.Fset.Position(call.Pos()), key, i+1, typ, verb)
						}
					}
				}
				used[key] = true
				return true
			})
		}
	}
	for key := range contract {
		if !used[key] {
			t.Errorf("unused text key %q", key)
		}
	}
}

func textArgumentMatches(typ types.Type, verb byte) bool {
	if typ == nil {
		return false
	}
	if basic, ok := typ.Underlying().(*types.Basic); ok {
		switch verb {
		case 'd':
			return basic.Info()&types.IsInteger != 0
		case 'f':
			return basic.Info()&types.IsFloat != 0
		case 's':
			if basic.Info()&types.IsString != 0 {
				return true
			}
		}
	}
	if verb == 's' {
		// fmt accepts byte slices and Stringer/error implementations for %s.
		if slice, ok := typ.Underlying().(*types.Slice); ok && types.Identical(slice.Elem(), types.Typ[types.Byte]) {
			return true
		}
		for _, name := range []string{"String", "Error"} {
			method := types.NewMethodSet(typ).Lookup(nil, name)
			if method == nil {
				continue
			}
			sig := method.Obj().Type().(*types.Signature)
			if sig.Params().Len() == 0 && sig.Results().Len() == 1 && types.Identical(sig.Results().At(0).Type(), types.Typ[types.String]) {
				return true
			}
		}
	}
	return false
}

func TestCatalogTemplates(t *testing.T) {
	for key, spec := range contract {
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
