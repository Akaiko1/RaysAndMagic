package world

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMapDefinitionContract(t *testing.T) {
	tm := installTestTileManager(t)
	if err := tm.LoadSpecialTileConfig("../../assets/special_tiles.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, line, wantError string
	}{
		{"mixed", "+@.$.@  >[stile:spike_trap], [tile:omikuji], [npc:merchant]", ""},
		{"npc_first", "+@.@  >[npc:merchant],[stile:spike_trap]", ""},
		{"unknown_stile", "+@.@  >[stile:missing], [npc:merchant]", "unknown special tile"},
		{"unknown_tile", "+$.$  >[tile:missing], [tile:omikuji]", "unknown general tile"},
		{"missing_interactive_def", "+@", "unbound entity"},
		{"missing_general_def", "+$", "unbound entity"},
		{"excess_interactive_def", "+@  >[npc:merchant], [stile:spike_trap]", "no @ marker"},
		{"excess_general_def", "+$  >[tile:omikuji], [tile:omikuji]", "no $ marker"},
		{"wrong_marker", "+@  >[tile:omikuji]", "no $ marker"},
		{"malformed", "+@  >[npc:merchant", "malformed entity"},
		{"unknown_tag", "+@  >[thing:merchant]", "unknown entity tag"},
		{"empty_npc", "+@  >[npc:]", "malformed npc"},
		{"no_entities", "+....", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "contract.map")
			if err := os.WriteFile(path, []byte(tc.line+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			md, err := NewMapLoaderWithBiome(nil, "forest").LoadMap(path)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) || md != nil {
					t.Fatalf("map=%v error=%v, want rejected map with %q", md, err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.name == "mixed" && (len(md.NPCSpawns) != 1 || md.NPCSpawns[0].X != 5 || md.Tiles[0][3] == TileEmpty) {
				t.Fatalf("mixed definitions shifted: %+v", md)
			}
		})
	}
}
