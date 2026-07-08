package game

import "testing"

func TestNPCDisplayCategory(t *testing.T) {
	anim := 64 * SpriteSheetFrameCount // animated sheet width for a 64px frame
	tests := []struct {
		name        string
		sprite      string
		renderType  string
		wallMounted bool
		w, h        int
		want        string
	}{
		{"spriteless anchor", "", "", false, 0, 0, NPCCatInvisible},
		{"sprite none", "none", "", false, 0, 0, NPCCatInvisible},
		{"wall beats everything", "grate", "landmark", true, anim, 64, NPCCatWallStandee},
		{"landmark", "mage_tower", "landmark", false, 128, 256, NPCCatLandmark},
		{"scenery", "shipwreck", "environment_sprite", false, 128, 128, NPCCatScenery},
		{"animated sheet", "elf", "", false, anim, 64, NPCCatAnimated},
		{"plain standee single frame", "oldman", "", false, 64, 64, NPCCatStandee},
		{"unresolved sprite stays standee", "missing", "", false, 0, 0, NPCCatStandee},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NPCDisplayCategory(tt.sprite, tt.renderType, tt.wallMounted, tt.w, tt.h)
			if got != tt.want {
				t.Fatalf("NPCDisplayCategory(%q,%q,%v,%d,%d) = %q, want %q",
					tt.sprite, tt.renderType, tt.wallMounted, tt.w, tt.h, got, tt.want)
			}
		})
	}
}

// Every canonical category must be reachable AND listed in the sort order, so
// the editor never drops a group (guards against adding a label to the switch
// but forgetting NPCDisplayCategoryOrder).
func TestNPCDisplayCategoryOrderCoversAll(t *testing.T) {
	all := []string{NPCCatInvisible, NPCCatWallStandee, NPCCatLandmark, NPCCatScenery, NPCCatAnimated, NPCCatStandee}
	inOrder := map[string]bool{}
	for _, c := range NPCDisplayCategoryOrder {
		inOrder[c] = true
	}
	for _, c := range all {
		if !inOrder[c] {
			t.Errorf("category %q missing from NPCDisplayCategoryOrder", c)
		}
	}
	if len(NPCDisplayCategoryOrder) != len(all) {
		t.Errorf("NPCDisplayCategoryOrder has %d entries, want %d", len(NPCDisplayCategoryOrder), len(all))
	}
}
