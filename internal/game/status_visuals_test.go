package game

import (
	"go/ast"
	"go/parser"
	"go/token"
	"math/bits"
	"strings"
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/monster"
)

func TestStatusVisualActiveStateAndHeldTurns(t *testing.T) {
	for _, tc := range statusVisualCatalog {
		if tc.monster == nil {
			continue
		}
		t.Run(tc.key, func(t *testing.T) {
			m := &monster.Monster3D{HitPoints: 100}
			if got := monsterStatusVisuals(m, true); got != 0 {
				t.Fatal("healthy monster has an overlay")
			}
			tc.monster(m)
			for _, tb := range []bool{false, true} {
				if got := monsterStatusVisuals(m, tb); got != tc.flag {
					t.Fatalf("active visual=%v want %v", got, tc.flag)
				}
			}
			m.HitPoints = 0
			if got := monsterStatusVisuals(m, true); got != 0 {
				t.Fatal("dead monster retains a living status overlay")
			}
		})
	}
	m := &monster.Monster3D{HitPoints: 100, RootFramesRemaining: 1, RootTurnsRemaining: 1, RootRate: 1}
	m.ApplySlow(30, 1, 1)
	m.ApplyWeaken(40, 1, 1)
	m.TickRootTurn()
	m.TickSlowTurn()
	m.TickWeakenTurn()
	if got := monsterStatusVisuals(m, true); got != visualRoot|visualSlow|visualWeaken {
		t.Fatal("final held turn lost its visual")
	}
	m.TickRootTurn()
	m.TickSlowTurn()
	m.TickWeakenTurn()
	if got := monsterStatusVisuals(m, true); got != 0 {
		t.Fatal("expired turn latch retained visual")
	}
}

func TestStatusVisualPartyConditionCoverage(t *testing.T) {
	g := &MMGame{}
	for cond := character.ConditionNormal; cond <= character.ConditionStunned; cond++ {
		c := &character.MMCharacter{HitPoints: 100, Conditions: []character.Condition{cond}}
		if cond == character.ConditionStunned {
			c.StunFramesRemaining = 1
		}
		v := g.partyStatusVisuals(c)
		switch cond {
		case character.ConditionNormal, character.ConditionPoisoned, character.ConditionBurning, character.ConditionStunned:
			if v != 0 {
				t.Fatal("existing specialized effect unexpectedly duplicated")
			}
		case character.ConditionUnconscious, character.ConditionDead, character.ConditionEradicated:
			// Down states darken the card; they never animate.
			if v != 0 || !partyCardDown(c) {
				t.Fatalf("%s must darken the card without a motif", cond)
			}
		default:
			if v == 0 {
				t.Fatalf("no effect for %s", cond)
			}
		}
	}
	for _, cond := range []character.Condition{character.ConditionDead, character.ConditionEradicated} {
		g.partyRoot = PartyRootState{Frames: 300}
		down := &character.MMCharacter{Conditions: []character.Condition{cond}}
		if g.partyStatusVisuals(down) != 0 {
			t.Fatalf("%s card animates the shared root", cond)
		}
		g.partyRoot = PartyRootState{}
	}
	if partyCardDown(&character.MMCharacter{HitPoints: 100, Conditions: []character.Condition{character.ConditionPoisoned}}) {
		t.Fatal("a living poisoned hero darkened")
	}
	c := &character.MMCharacter{HitPoints: 100}
	g.partyRoot = PartyRootState{Frames: 300}
	if g.partyStatusVisuals(c)&visualRoot == 0 || !strings.Contains(g.partyConditionLabel(c), "Rooted") {
		t.Fatal("party root has no visual or label")
	}
	c.Conditions = []character.Condition{character.ConditionUnconscious}
	if g.partyStatusVisuals(c) != visualRoot {
		t.Fatal("shared root disappeared from unconscious card")
	}
	g.partyRoot = PartyRootState{}
	if g.partyStatusVisuals(c) != 0 {
		t.Fatal("cleansed root retained its visual")
	}
	c.Conditions = nil
	if g.partyConditionLabel(c) != "OK" {
		t.Fatal("healthy card retains status label")
	}
}

// Every hero entry's staged state derives exactly its own motif, as on the HUD.
func TestStatusVisualCatalogHeroRoundTrip(t *testing.T) {
	for _, tc := range statusVisualCatalog {
		if tc.hero == nil {
			continue
		}
		t.Run(tc.key, func(t *testing.T) {
			g := &MMGame{}
			h := &character.MMCharacter{HitPoints: 100}
			tc.hero(g, h)
			if got := g.partyStatusVisuals(h); got != tc.flag {
				t.Fatalf("hero visual=%v want %v", got, tc.flag)
			}
		})
	}
}

// The catalog is the preview's only source of motifs: it must name each enum
// flag exactly once and stage it on at least one actor.
func TestStatusVisualCatalogCoversEnum(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "render_status_fx.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	declared := 0
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST || len(gen.Specs) == 0 {
			continue
		}
		first := gen.Specs[0].(*ast.ValueSpec)
		if id, ok := first.Type.(*ast.Ident); !ok || id.Name != "statusVisuals" {
			continue
		}
		for _, spec := range gen.Specs {
			declared += len(spec.(*ast.ValueSpec).Names)
		}
	}
	var union statusVisuals
	keys := map[string]bool{}
	for _, e := range statusVisualCatalog {
		if e.flag == 0 || e.flag&(e.flag-1) != 0 || union&e.flag != 0 {
			t.Fatalf("%s: flag must be a single unused bit", e.key)
		}
		if keys[e.key] || e.label == "" || (e.monster == nil && e.hero == nil) {
			t.Fatalf("%s: duplicate key, missing label or no stage", e.key)
		}
		union |= e.flag
		keys[e.key] = true
	}
	if declared == 0 || len(statusVisualCatalog) != declared || union != statusVisuals(1)<<declared-1 {
		t.Fatalf("catalog has %d entries for %d declared motifs", len(statusVisualCatalog), declared)
	}
}

// Badge motifs are drawn only by their head-badge renderer; every other motif
// has none and paints on the canvas instead.
func TestStatusVisualBadgeRenderers(t *testing.T) {
	for _, e := range statusVisualCatalog {
		_, drawn := statusBadgeRenderers[e.flag]
		if badge := e.flag&statusBadgeVisuals != 0; badge != drawn {
			t.Fatalf("%s: badge=%v but renderer=%v", e.key, badge, drawn)
		}
	}
	if len(statusBadgeRenderers) != bits.OnesCount32(uint32(statusBadgeVisuals)) {
		t.Fatal("badge renderer for a flag outside the badge set")
	}
}
