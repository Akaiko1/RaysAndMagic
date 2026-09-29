package game

import (
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/game/keytracker"
)

func heldOnly(keys ...ebiten.Key) func(ebiten.Key) bool {
	return func(k ebiten.Key) bool { return slices.Contains(keys, k) }
}

// V picks Fold or Return from the held Shift and ignores a sprint, all through
// the heldKeys seam - the real keyboard must not decide which step fires.
func TestSpatialStepInputReadsHeldKeysThroughSeam(t *testing.T) {
	cases := []struct {
		name string
		held []ebiten.Key
		want string // fold, return, ignored
	}{
		{"V", nil, "fold"},
		{"ShiftLeft+V", []ebiten.Key{ebiten.KeyShiftLeft}, "return"},
		{"ShiftRight+V", []ebiten.Key{ebiten.KeyShiftRight}, "return"},
		{"W+V walking", []ebiten.Key{ebiten.KeyW}, "fold"},
		{"Shift+W+V running", []ebiten.Key{ebiten.KeyShiftLeft, ebiten.KeyW}, "ignored"},
		{"Shift+S+V running", []ebiten.Key{ebiten.KeyShiftRight, ebiten.KeyS}, "ignored"},
		{"Shift+A+V running", []ebiten.Key{ebiten.KeyShiftLeft, ebiten.KeyA}, "ignored"},
		{"Shift+D+V running", []ebiten.Key{ebiten.KeyShiftLeft, ebiten.KeyD}, "ignored"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, c := rareClassGame(t, character.ClassWayfarer, false)
			startX, startY := g.camera.X, g.camera.Y
			if !g.useTechnique(0, "fold_step", false, false) {
				t.Fatal("setup Fold rejected")
			}
			foldX, foldY := g.camera.X, g.camera.Y
			g.spatialReuseFrames = 0
			c.RTCooldown = 0
			c.ActionsRemaining = 2
			sp := c.SpellPoints

			ih := NewInputHandler(g)
			ih.heldKeys = heldOnly(tc.held...)
			ih.keys = keytracker.NewWithSource(func(k ebiten.Key) bool { return k == ebiten.KeyV })
			ih.keys.BeginFrame()
			if !ih.handleSpatialStepInput() {
				t.Fatal("V press not claimed")
			}
			switch tc.want {
			case "fold":
				if c.SpellPoints >= sp || math.Hypot(g.camera.X-foldX, g.camera.Y-foldY) < 1 {
					t.Fatal("Fold Step did not fire")
				}
			case "return":
				if c.SpellPoints >= sp || math.Hypot(g.camera.X-startX, g.camera.Y-startY) > .01 {
					t.Fatal("Return Step did not bring the party back to the anchor")
				}
			case "ignored":
				if c.SpellPoints != sp || g.camera.X != foldX || g.camera.Y != foldY {
					t.Fatal("a step fired while running")
				}
			}
		})
	}
}

// Movement, party selection and the RT hold-repeat chain read held keys through
// the seam, so a scripted hold drives them and the desktop keyboard cannot.
func TestInputHandlerHeldKeysThroughSeam(t *testing.T) {
	t.Run("movement", func(t *testing.T) {
		cases := []struct {
			name    string
			held    []ebiten.Key
			forward int // sign of travel along the facing
			right   int // sign of travel along the right vector
			turn    int // sign of the angle change
		}{
			{"none", nil, 0, 0, 0},
			{"W", []ebiten.Key{ebiten.KeyW}, 1, 0, 0},
			{"Up", []ebiten.Key{ebiten.KeyUp}, 1, 0, 0},
			{"S", []ebiten.Key{ebiten.KeyS}, -1, 0, 0},
			{"Down", []ebiten.Key{ebiten.KeyDown}, -1, 0, 0},
			{"Q", []ebiten.Key{ebiten.KeyQ}, 0, -1, 0},
			{"E", []ebiten.Key{ebiten.KeyE}, 0, 1, 0},
			{"A", []ebiten.Key{ebiten.KeyA}, 0, 0, -1},
			{"Left", []ebiten.Key{ebiten.KeyLeft}, 0, 0, -1},
			{"D", []ebiten.Key{ebiten.KeyD}, 0, 0, 1},
			{"Right", []ebiten.Key{ebiten.KeyRight}, 0, 0, 1},
		}
		sign := func(v float64) int {
			switch {
			case v > 1e-9:
				return 1
			case v < -1e-9:
				return -1
			}
			return 0
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				g, _ := rareClassGame(t, character.ClassKnight, false)
				ih := NewInputHandler(g)
				ih.heldKeys = heldOnly(tc.held...)
				x, y, angle := g.camera.X, g.camera.Y, g.camera.Angle
				fx, fy := g.camera.GetForwardX(), g.camera.GetForwardY()
				rx, ry := g.camera.GetRightX(), g.camera.GetRightY()
				ih.handleMovementInput()
				dx, dy := g.camera.X-x, g.camera.Y-y
				if sign(dx*fx+dy*fy) != tc.forward || sign(dx*rx+dy*ry) != tc.right || sign(g.camera.Angle-angle) != tc.turn {
					t.Fatalf("delta=(%.3f,%.3f) turn=%.4f", dx, dy, g.camera.Angle-angle)
				}
			})
		}
		t.Run("Shift+W runs", func(t *testing.T) {
			step := func(held ...ebiten.Key) float64 {
				g, _ := rareClassGame(t, character.ClassKnight, false)
				ih := NewInputHandler(g)
				ih.heldKeys = heldOnly(held...)
				x, y := g.camera.X, g.camera.Y
				ih.handleMovementInput()
				return math.Hypot(g.camera.X-x, g.camera.Y-y)
			}
			g, _ := rareClassGame(t, character.ClassKnight, false)
			walk, run := step(ebiten.KeyW), step(ebiten.KeyShiftLeft, ebiten.KeyW)
			if walk == 0 || math.Abs(run/walk-g.config.GetRunMultiplier()) > 1e-6 {
				t.Fatalf("walk=%.4f run=%.4f", walk, run)
			}
		})
	})

	t.Run("party selection", func(t *testing.T) {
		cases := []struct {
			name string
			held []ebiten.Key
			want int
		}{
			{"none", nil, -1},
			{"Key1", []ebiten.Key{ebiten.Key1}, 0},
			{"Key2", []ebiten.Key{ebiten.Key2}, 1},
			{"Key3", []ebiten.Key{ebiten.Key3}, 2},
			{"Key4", []ebiten.Key{ebiten.Key4}, 3},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				g, _ := rareClassGame(t, character.ClassKnight, false)
				start := (tc.want + 1) % len(g.party.Members)
				if tc.want < 0 {
					start = 1
				}
				g.selectedChar = start
				ih := NewInputHandler(g)
				ih.heldKeys = heldOnly(tc.held...)
				ih.handleCharacterSelectionInput()
				want := tc.want
				if want < 0 {
					want = start
				}
				if g.selectedChar != want {
					t.Fatalf("selected %d want %d", g.selectedChar, want)
				}
			})
		}
	})

	t.Run("RT hold repeat", func(t *testing.T) {
		cases := []struct {
			name string
			key  ebiten.Key
			want rtActionKind
		}{
			{"R", ebiten.KeyR, rtActWeapon},
			{"Space", ebiten.KeySpace, rtActSmart},
			{"F", ebiten.KeyF, rtActCast},
			{"C", ebiten.KeyC, rtActHeal},
			{"H", ebiten.KeyH, rtActHeal},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				g, _ := rareClassGame(t, character.ClassKnight, false)
				ih := NewInputHandler(g)
				ih.heldKeys = heldOnly(tc.key)
				ih.keys = keytracker.NewWithSource(func(ebiten.Key) bool { return false })
				for frame := 1; frame <= rtHoldRepeatDelay; frame++ {
					ih.keys.BeginFrame()
					ih.handleCombatInput()
					if frame < rtHoldRepeatDelay && ih.pendingRepeat != rtActNone {
						t.Fatalf("hold repeated after %d frames", frame)
					}
				}
				if ih.pendingRepeat != tc.want {
					t.Fatalf("pending repeat %d want %d", ih.pendingRepeat, tc.want)
				}
			})
		}
	})
}

// No InputHandler method may read the live keyboard except keyHeld itself,
// directly or through a package helper that does.
func TestInputHandlerHasNoRawHeldKeyReads(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	isRawRead := func(call *ast.CallExpr) bool {
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "IsKeyPressed" {
			return false
		}
		pkg, ok := sel.X.(*ast.Ident)
		return ok && pkg.Name == "ebiten"
	}
	type method struct {
		name string
		body *ast.BlockStmt
	}
	var methods []method
	rawHelpers := map[string]bool{}
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
			if fn.Recv != nil && len(fn.Recv.List) == 1 {
				if star, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
					if id, ok := star.X.(*ast.Ident); ok && id.Name == "InputHandler" {
						methods = append(methods, method{path + ":" + fn.Name.Name, fn.Body})
						continue
					}
				}
			}
			if fn.Recv == nil {
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					if call, ok := n.(*ast.CallExpr); ok && isRawRead(call) {
						rawHelpers[fn.Name.Name] = true
					}
					return true
				})
			}
		}
	}
	seamReads := 0
	for _, m := range methods {
		ast.Inspect(m.body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if strings.HasSuffix(m.name, ":keyHeld") {
				if isRawRead(call) {
					seamReads++
				}
				return true
			}
			if isRawRead(call) {
				t.Errorf("%s reads the keyboard directly; use ih.keyHeld", m.name)
			}
			if id, ok := call.Fun.(*ast.Ident); ok && rawHelpers[id.Name] {
				t.Errorf("%s reads the keyboard through %s; use ih.keyHeld", m.name, id.Name)
			}
			return true
		})
	}
	if seamReads != 1 || len(methods) < 50 {
		t.Fatalf("guard is vacuous: %d methods scanned, keyHeld live reads=%d", len(methods), seamReads)
	}
}
