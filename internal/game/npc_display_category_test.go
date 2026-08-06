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

func TestNPCVisualSizeValidation(t *testing.T) {
	classes := map[string]float64{
		"person": 0.6, "small_prop": 0.5, "structure": 2,
	}
	tests := []struct {
		name    string
		npc     *character.NPCData
		wantErr bool
	}{
		{name: "person", npc: &character.NPCData{RenderCategory: "npc", SizeClass: "person"}},
		{name: "prop", npc: &character.NPCData{RenderCategory: "scenery", SizeClass: "small_prop"}},
		{name: "landmark", npc: &character.NPCData{RenderCategory: "landmark", SizeClass: "structure"}},
		{name: "wide landmark", npc: &character.NPCData{RenderCategory: "wide_landmark", GridSpanTiles: 4, SizeClass: "structure"}},
		{name: "invisible", npc: &character.NPCData{RenderCategory: "invisible"}},
		{name: "missing class", npc: &character.NPCData{RenderCategory: "scenery"}, wantErr: true},
		{name: "unknown class", npc: &character.NPCData{RenderCategory: "scenery", SizeClass: "typo"}, wantErr: true},
		{name: "actor class on prop", npc: &character.NPCData{RenderCategory: "scenery", SizeClass: "person"}, wantErr: true},
		{name: "wrong person class", npc: &character.NPCData{RenderCategory: "npc", SizeClass: "small_prop"}, wantErr: true},
		{name: "invisible class", npc: &character.NPCData{RenderCategory: "invisible", SizeClass: "small_prop"}, wantErr: true},
		// The facade path and the category must agree, or one silently wins.
		{name: "grid span on plain landmark", npc: &character.NPCData{RenderCategory: "landmark", GridSpanTiles: 4, SizeClass: "structure"}, wantErr: true},
		{name: "wide landmark without span", npc: &character.NPCData{RenderCategory: "wide_landmark", SizeClass: "structure"}, wantErr: true},
		{name: "wide landmark without class", npc: &character.NPCData{RenderCategory: "wide_landmark", GridSpanTiles: 4}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNPCVisualSizes(map[string]*character.NPCData{"subject": tt.npc}, classes)
			if tt.wantErr && err == nil {
				t.Fatal("invalid visual size accepted")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("valid visual size rejected: %v", err)
			}
		})
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
	all := []npcRenderCat{catNPC, catWall, catDoor, catLandmark, catWideLandmark, catScenery, catInvisible}
	for _, c := range all {
		if npcCatName[c] == "" {
			t.Errorf("category %d has no YAML name", c)
		}
	}
	if len(npcCatName) != len(all) {
		t.Errorf("npcCatName has %d entries, want %d", len(npcCatName), len(all))
	}
}
