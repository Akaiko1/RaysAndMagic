package config

import "testing"

func TestItemSetsParsedAndLinesRender(t *testing.T) {
	if _, err := LoadItemConfig("../../assets/items.yaml"); err != nil {
		t.Fatalf("load items: %v", err)
	}
	for _, key := range []string{"padded", "ringmail"} {
		set := GetItemSet(key)
		if set == nil {
			t.Fatalf("item set %q not parsed", key)
		}
		if set.PiecesRequired != 4 {
			t.Fatalf("%s pieces_required = %d, want 4", key, set.PiecesRequired)
		}
	}
	def, ok := GetItemDefinition("padded_cap")
	if !ok || def.Set != "padded" {
		t.Fatalf("padded_cap set field = %+v", def)
	}
	lines := def.SetLines()
	if len(lines) != 2 {
		t.Fatalf("padded_cap SetLines = %v, want set name + bonus", lines)
	}
}
