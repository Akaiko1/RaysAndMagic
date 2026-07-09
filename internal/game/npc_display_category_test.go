package game

import "testing"

func TestNPCDisplayCategory(t *testing.T) {
	anim := 64 * SpriteSheetFrameCount // animated sheet width for a 64px frame
	tests := []struct {
		name           string
		renderCategory string
		sprite         string
		renderType     string
		wallMounted    bool
		w, h           int
		want           npcRenderCat
	}{
		// Derived (render_category empty) - the legacy classification.
		{"spriteless anchor", "", "", "", false, 0, 0, catInvisible},
		{"sprite none", "", "none", "", false, 0, 0, catInvisible},
		{"wall beats everything", "", "grate", "landmark", true, anim, 64, catWall},
		{"landmark", "", "mage_tower", "landmark", false, 128, 256, catLandmark},
		{"scenery", "", "shipwreck", "environment_sprite", false, 128, 128, catScenery},
		{"animated sheet", "", "elf", "", false, anim, 64, catAnimated},
		{"plain standee single frame", "", "oldman", "", false, 64, 64, catStandee},
		{"unresolved sprite stays standee", "", "missing", "", false, 0, 0, catStandee},
		// Explicit render_category WINS over the derived signals.
		{"explicit scenery over animated sheet", "scenery", "elf", "", false, anim, 64, catScenery},
		{"explicit standee over env render_type", "standee", "shipwreck", "environment_sprite", false, 128, 128, catStandee},
		// A bogus explicit value falls back to derivation.
		{"bad explicit falls back to derived", "bogus", "elf", "", false, anim, 64, catAnimated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveNPCRenderCat(tt.renderCategory, tt.sprite, tt.renderType, tt.wallMounted, tt.w, tt.h)
			if got != tt.want {
				t.Fatalf("resolveNPCRenderCat(%q,%q,%q,%v,%d,%d) = %q, want %q",
					tt.renderCategory, tt.sprite, tt.renderType, tt.wallMounted, tt.w, tt.h, npcCatName[got], npcCatName[tt.want])
			}
		})
	}
}

// Every canonical category must have a label, a YAML name, and appear in the
// sort order, so the editor never drops a group and content can always name it.
func TestNPCRenderCatTablesCoverAll(t *testing.T) {
	all := []npcRenderCat{catStandee, catAnimated, catWall, catLandmark, catScenery, catInvisible}
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
