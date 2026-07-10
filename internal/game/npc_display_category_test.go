package game

import (
	"testing"

	"ugataima/internal/character"
)

// render_category is REQUIRED: every canonical value parses to its category,
// and a missing or unknown one is rejected by validation (no silent fallback).
func TestNPCRenderCategoryParsing(t *testing.T) {
	for cat, name := range npcCatName {
		if got := resolveNPCRenderCat(name); got != cat {
			t.Errorf("resolveNPCRenderCat(%q) = %d, want %d", name, got, cat)
		}
	}
}

func TestNPCRenderCategoryValidation(t *testing.T) {
	valid := map[string]*character.NPCData{
		"gate": {RenderCategory: "wall_mounted"},
		"elf":  {RenderCategory: "animated"},
	}
	if err := ValidateNPCRenderCategories(valid); err != nil {
		t.Errorf("valid categories rejected: %v", err)
	}

	for name, npcs := range map[string]map[string]*character.NPCData{
		"missing":           {"gate": {}},
		"unknown":           {"gate": {RenderCategory: "bogus"}},
		"legacy wall value": {"gate": {RenderCategory: "wall"}},
	} {
		if err := ValidateNPCRenderCategories(npcs); err == nil {
			t.Errorf("%s render_category passed validation", name)
		}
	}
}

// An unvalidated value reaching the render path must fail loud, not guess.
func TestResolveNPCRenderCatPanicsOnUnknown(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("resolveNPCRenderCat(\"\") did not panic")
		}
	}()
	resolveNPCRenderCat("")
}

// Every canonical category must have a label, a YAML name, and appear in the
// sort order, so the editor never drops a group and content can always name it.
func TestNPCRenderCatTablesCoverAll(t *testing.T) {
	all := []npcRenderCat{catStandee, catAnimated, catWall, catDoor, catLandmark, catScenery, catInvisible}
	inOrder := map[string]bool{}
	for _, c := range NPCDisplayCategoryOrder {
		inOrder[c] = true
	}
	for _, c := range all {
		if npcCatLabel[c] == "" {
			t.Errorf("category %d has no editor label", c)
		}
		if npcCatName[c] == "" {
			t.Errorf("category %d has no YAML name", c)
		}
		if !inOrder[npcCatLabel[c]] {
			t.Errorf("category %q missing from NPCDisplayCategoryOrder", npcCatLabel[c])
		}
	}
	if len(NPCDisplayCategoryOrder) != len(all) {
		t.Errorf("NPCDisplayCategoryOrder has %d entries, want %d", len(NPCDisplayCategoryOrder), len(all))
	}
}
