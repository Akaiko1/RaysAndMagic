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
		"elf":  {RenderCategory: "npc"},
	}
	if err := ValidateNPCRenderCategories(valid); err != nil {
		t.Errorf("valid categories rejected: %v", err)
	}

	for name, npcs := range map[string]map[string]*character.NPCData{
		"missing":               {"gate": {}},
		"unknown":               {"gate": {RenderCategory: "bogus"}},
		"legacy wall value":     {"gate": {RenderCategory: "wall"}},
		"legacy standee value":  {"gate": {RenderCategory: "standee"}},
		"legacy animated value": {"gate": {RenderCategory: "animated"}},
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

// Every canonical category must have a YAML name (render_category is purely a
// render dispatch; the editor groups by the NPC `type:` field, not by this).
func TestNPCRenderCatTablesCoverAll(t *testing.T) {
	all := []npcRenderCat{catNPC, catWall, catDoor, catLandmark, catScenery, catInvisible}
	for _, c := range all {
		if npcCatName[c] == "" {
			t.Errorf("category %d has no YAML name", c)
		}
	}
	if len(npcCatName) != len(all) {
		t.Errorf("npcCatName has %d entries, want %d", len(npcCatName), len(all))
	}
}
