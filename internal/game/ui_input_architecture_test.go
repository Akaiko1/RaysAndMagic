package game

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// Keep a late keyboard-handler call from quietly reintroducing a second mouse
// owner. Behavioral tests exercise controls; this pins the architectural boundary.
func TestUIKeyboardPathsCannotConsumePointerActions(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	graph := map[string][]string{}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if _, exists := graph[fn.Name.Name]; !exists {
				graph[fn.Name.Name] = nil
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := ""
				switch f := call.Fun.(type) {
				case *ast.SelectorExpr:
					// Input adapters are package-private. Exported selectors
					// include external APIs (for example draw.Draw) and
					// must not resolve to an unrelated local method by name.
					if !f.Sel.IsExported() {
						name = f.Sel.Name
					}
				case *ast.Ident:
					name = f.Name
				}
				if name != "" {
					graph[fn.Name.Name] = append(graph[fn.Name.Name], name)
				}
				return true
			})
		}
	}
	forbidden := map[string]bool{}
	for _, name := range []string{"leftClickPosition", "consumeLeftClick", "consumeLeftClickIn", "consumeRightClickIn", "consumeRightClickMatching", "pointerLeftJustPressed", "pointerLeftJustRelease", "pointerLeftPressed", "pointerRightJustPress"} {
		forbidden[name] = true
	}
	for _, root := range []string{"handleTopModalInput", "handleSaveLoadMenuInput", "handleTabbedMenuInput", "updateEntryMenu", "updatePartyCreate"} {
		t.Run(root, func(t *testing.T) {
			if _, ok := graph[root]; !ok {
				t.Fatal("missing keyboard entry point")
			}
			queue := [][]string{{root}}
			seen := map[string]bool{}
			for len(queue) > 0 {
				route := queue[0]
				queue = queue[1:]
				name := route[len(route)-1]
				if seen[name] {
					continue
				}
				seen[name] = true
				if forbidden[name] {
					t.Fatalf("keyboard path bypasses displayed input: %s", strings.Join(route, " -> "))
				}
				for _, next := range graph[name] {
					chain := append([]string(nil), route...)
					queue = append(queue, append(chain, next))
				}
			}
		})
	}
}
