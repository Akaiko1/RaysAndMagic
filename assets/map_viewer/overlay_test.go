package main

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ugataima/internal/boot"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/storage"
	"ugataima/internal/world"

	"gopkg.in/yaml.v3"
)

func overlayTestViewer(t *testing.T) *viewer {
	t.Helper()
	t.Chdir("../..")
	cfg, mobs := boot.LoadGameData()
	maps, err := loadMaps(cfg)
	if err != nil {
		t.Fatal(err)
	}
	v := &viewer{cfg: cfg, monsterCfg: mobs, maps: maps, tileManager: world.GlobalTileManager, tileDataByKey: world.GlobalTileManager.ListTiles(), page: pageOverlay, legendCollapsed: map[string]bool{}}
	for i, m := range maps {
		if m.Key == "solstice_earth" {
			v.mapIndex = i
			return v
		}
	}
	t.Fatal("missing chamber fixture")
	return nil
}
func overlaySandbox(t *testing.T, v *viewer) {
	t.Helper()
	dir := t.TempDir()
	paths := []string{"assets/quests.yaml", "assets/spells.yaml", "assets/items.yaml", overlayConfigPath, overlayNPCPath, filepath.Join("assets", v.maps[v.mapIndex].Config.File)}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
}
func overlayEmptyCell(t *testing.T, v *viewer, d *overlayDocument) [2]int {
	t.Helper()
	for y := 1; y < d.state.Data.Height-1; y++ {
		for x := 1; x < d.state.Data.Width-1; x++ {
			if d.freeTile(x, y, v.tileManager) {
				return [2]int{x, y}
			}
		}
	}
	t.Fatal("no empty tile")
	return [2]int{}
}
func TestOverlayYAMLPatchesPreserveNeighbors(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(strings.ReplaceAll(eol, "\n", "LF"), func(t *testing.T) {
			raw := strings.ReplaceAll("# heading\nmaps:\n  first:\n    name: Keep\n    adventure:\n      element: earth\n\n  # second map documentation\n  second:\n    name: Other\n# footer\n", "\n", eol)
			next, err := overlayPatch([]byte(raw), []string{"maps", "first", "adventure"}, config.AdventureConfig{Element: "air"})
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Replace(raw, "element: earth", "element: air", 1)
			if string(next) != want {
				t.Fatalf("neighbor bytes changed:\n%s", next)
			}
			added, err := overlayPatch(next, []string{"maps", "second", "adventure"}, config.AdventureConfig{Element: "fire"})
			if err != nil {
				t.Fatal(err)
			}
			want = strings.Replace(want, "    name: Other"+eol, "    name: Other"+eol+"    adventure:"+eol+"      element: fire"+eol, 1)
			if string(added) != want {
				t.Fatalf("insertion changed neighbors:\n%s", added)
			}
			if !overlayEntryUnchanged([]byte(raw), added, []string{"maps", "second", "name"}) {
				t.Fatal("unrelated entry reported changed")
			}
			if overlayEntryUnchanged([]byte(raw), added, []string{"maps", "first", "adventure"}) {
				t.Fatal("same-entry change missed")
			}
		})
	}
	// An empty mapping is valid YAML but cannot be safely spliced by line.
	if _, err := overlayPatch([]byte("npcs: {}\n"), []string{"npcs", "new"}, "value"); err == nil {
		t.Fatal("flow mapping must not be corrupted")
	}
}
func TestOverlayPublishRollsBackAndRejectsConflicts(t *testing.T) {
	dir := t.TempDir()
	var files []overlayWrite
	for _, key := range []string{"config", "npc", "map"} {
		p := filepath.Join(dir, key)
		os.WriteFile(p, []byte("before"), 0600)
		files = append(files, overlayWrite{p, []byte("before"), []byte("after")})
	}
	count := 0
	err := overlayPublish(files, func(p string, b []byte, m os.FileMode) error {
		count++
		if count == 3 {
			return errors.New("disk full")
		}
		return storage.WriteFileAtomic(p, b, m)
	})
	if err == nil {
		t.Fatal("write failure lost")
	}
	for _, f := range files {
		b, _ := os.ReadFile(f.path)
		if string(b) != "before" {
			t.Fatal("partial save remained")
		}
	}
	os.WriteFile(files[1].path, []byte("concurrent"), 0600)
	if err := overlayPublish(files, func(string, []byte, os.FileMode) error { t.Fatal("write before conflict check"); return nil }); err == nil {
		t.Fatal("missed conflict")
	}
}
func TestOverlayAuthorSaveReload(t *testing.T) {
	v := overlayTestViewer(t)
	overlaySandbox(t, v)
	d := v.overlayDoc()
	a := &d.state.Adventure
	// Exercise the same gesture handlers and field application as the UI.
	v.overlay.tool = "trap"
	v.overlayMapPress(d, [2]int{2, 2})
	v.overlay.end = [2]int{3, 3}
	v.overlayFinishDrag(d)
	trap := a.Effects[v.overlay.selected].ID
	for i, f := range v.overlayFields(d) {
		if f.label == "Damage" {
			v.overlayOpenField(d, i)
			if err := v.overlay.modal.apply("bad"); err == nil {
				t.Fatal("invalid number accepted")
			}
			if err := v.overlay.modal.apply("73"); err != nil {
				t.Fatal(err)
			}
			v.overlay.modal = nil
			break
		}
	}
	a = &d.state.Adventure
	if a.Effect(trap).Damage != 73 {
		t.Fatal("retry after invalid field edited stale state")
	}
	cell := overlayEmptyCell(t, v, d)
	v.overlay.tool = "new-control"
	v.overlayMapPress(d, cell)
	a = &d.state.Adventure
	c := &a.Controls[v.overlay.selected]
	c.Disable = []string{trap}
	key, _ := d.controlNPC(c.ID)
	if key == "" {
		t.Fatal("switch has no runtime action")
	}
	if err := v.validateOverlay(d); err != nil {
		t.Fatal(err)
	}
	// A full undo step restores both placement and NPC definition.
	d.history(false)
	if d.state.NPCs[key] != nil {
		t.Fatal("undo left orphan NPC")
	}
	d.history(true)
	beforeNPCs, _ := os.ReadFile(overlayNPCPath)
	beforeConfig, _ := os.ReadFile(overlayConfigPath)
	if err := v.saveOverlay(d); err != nil {
		t.Fatal(err)
	}
	if d.dirty() {
		t.Fatal("saved draft still dirty")
	}
	if err := character.LoadNPCConfig(overlayNPCPath); err != nil {
		t.Fatalf("saved NPC rejected by game: %v", err)
	}
	gotNPC := character.NPCConfigInstance.NPCs[key]
	if gotNPC == nil || gotNPC.Dialogue.Choices[0].Action != "adventure_control" {
		t.Fatal("NPC action not persisted")
	}
	wm := world.NewWorldManager(v.cfg)
	if err := wm.LoadMapConfigs(overlayConfigPath); err != nil {
		t.Fatal(err)
	}
	saved := wm.MapConfigs[d.key].Adventure
	if saved.Effect(trap) == nil || saved.Effect(trap).Damage != 73 || saved.Control(gotNPC.Dialogue.Choices[0].Control) == nil {
		t.Fatal("saved runtime references broken")
	}
	reloaded, err := world.NewMapLoaderWithBiome(v.cfg, v.currentBiome()).LoadMap(filepath.Join("assets", v.maps[v.mapIndex].Config.File))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range reloaded.NPCSpawns {
		if n.NPCKey == key {
			found = n.X == cell[0] && n.Y == cell[1]
		}
	}
	if !found {
		t.Fatal("new switch lost from map")
	}
	afterNPCs, _ := os.ReadFile(overlayNPCPath)
	afterConfig, _ := os.ReadFile(overlayConfigPath)
	var original character.NPCConfig
	if err := yaml.Unmarshal(beforeNPCs, &original); err != nil {
		t.Fatal(err)
	}
	for k := range original.NPCs {
		if !overlayEntryUnchanged(beforeNPCs, afterNPCs, []string{"npcs", k}) {
			t.Fatalf("unrelated NPC %s rewritten", k)
		}
	}
	var originalMaps config.MapConfigs
	yaml.Unmarshal(beforeConfig, &originalMaps)
	for k := range originalMaps.Maps {
		if k != d.key && !overlayEntryUnchanged(beforeConfig, afterConfig, []string{"maps", k}) {
			t.Fatalf("unrelated map %s rewritten", k)
		}
	}
	// Unrelated disk edits merge; editing our own adventure rejects all writes.
	current, _ := overlayPatch(afterConfig, []string{"maps", "forest", "name"}, "Concurrent forest name")
	os.WriteFile(overlayConfigPath, current, 0600)
	d.checkpoint()
	d.state.Adventure.Effect(trap).Damage++
	if err := v.saveOverlay(d); err != nil {
		t.Fatal(err)
	}
	latest, _ := os.ReadFile(overlayConfigPath)
	if !bytes.Contains(latest, []byte("Concurrent forest name")) {
		t.Fatal("parallel map edit lost")
	}
	changed := overlayClone(d.state.Adventure)
	changed.Effect(trap).Damage = 99
	concurrent, _ := overlayPatch(latest, []string{"maps", d.key, "adventure"}, changed)
	os.WriteFile(overlayConfigPath, concurrent, 0600)
	d.state.Adventure.Effect(trap).Damage = 101
	if err := v.saveOverlay(d); err == nil {
		t.Fatal("same overlay conflict accepted")
	}
	untouched, _ := os.ReadFile(overlayConfigPath)
	if !bytes.Equal(untouched, concurrent) {
		t.Fatal("conflicting file overwritten")
	}
}

func TestOverlayInspectorNavigationAndPersistence(t *testing.T) {
	v := overlayTestViewer(t)
	overlaySandbox(t, v)
	d := v.overlayDoc()
	// Use the production row hit-test and activation path, including scrolled rows.
	activate := func(label string) {
		t.Helper()
		fields := v.overlayFields(d)
		rows, maxScroll := overlayInspectorRows(fields, 0)
		for i, f := range fields {
			if f.label != label {
				continue
			}
			v.overlay.fieldScroll = clampInt(rows[i].r.y-overlayInspectorTop, 0, maxScroll)
			rows, _ = overlayInspectorRows(fields, v.overlay.fieldScroll)
			r := rows[i].r
			hit := overlayInspectorHit(fields, v.overlay.fieldScroll, r.x+12, r.y+12)
			if hit != i {
				t.Fatalf("%s: hit %d, want %d", label, hit, i)
			}
			v.overlayOpenField(d, hit)
			return
		}
		t.Fatalf("missing control %q on %s", label, v.overlay.section)
	}
	edit := func(label, value string) {
		t.Helper()
		activate(label)
		if v.overlay.modal == nil {
			t.Fatalf("%s did not open an editor", label)
		}
		if err := v.overlay.modal.apply(value); err != nil {
			t.Fatal(err)
		}
		v.overlay.modal = nil
	}
	v.overlaySelect("boss", 0)
	activate("Mechanics")
	activate("Regeneration")
	activate("Enable regeneration")
	if d.state.Adventure.Boss.HealPercent == 0 || v.overlay.modal != nil {
		t.Fatal("inline toggle did not enable healing")
	}
	edit("HP restored per tick (%)", "9")
	edit("Total healing budget (%)", "40")
	id := d.state.Adventure.Controls[0].ID
	edit("Switches that stop healing", id)
	if !d.state.Adventure.Controls[0].StopHealing {
		t.Fatal("regeneration link was not applied")
	}
	key, _ := d.controlNPC(id)
	for _, n := range d.state.Data.NPCSpawns {
		if n.NPCKey == key {
			v.overlayMapPress(d, [2]int{n.X, n.Y})
			break
		}
	}
	if v.overlay.section != "control" {
		t.Fatal("placed switch did not select its behavior")
	}
	activate("Edit boss mechanics")
	activate("+ Add mechanic")
	if err := v.overlay.modal.apply("Critical chance"); err != nil {
		t.Fatal(err)
	}
	v.overlay.modal = nil
	edit("Base chance (%)", "65")
	activate("+ Link switch")
	if err := v.overlay.modal.apply(id); err != nil {
		t.Fatal(err)
	}
	v.overlay.modal = nil
	edit(id+" - points removed", "25")
	v.overlaySelect("arena", 0)
	before := *d.state.Adventure.Boss.Arena
	activate("Width (tiles)")
	if err := v.overlay.modal.apply("1000"); err == nil {
		t.Fatal("out of map area accepted")
	}
	if err := v.overlay.modal.apply("6"); err != nil {
		t.Fatal(err)
	}
	v.overlay.modal = nil
	if d.state.Adventure.Boss.Arena[2] != before[0]+5 {
		t.Fatal("area resize edited stale state after invalid input")
	}
	v.overlayHistory(d, false)
	if *d.state.Adventure.Boss.Arena != before {
		t.Fatal("undo did not restore area")
	}
	v.overlayHistory(d, true)
	// Focus and hit testing skip informational rows and section headings.
	v.overlaySelect("mechanics", 0)
	fields := v.overlayFields(d)
	rows, _ := overlayInspectorRows(fields, 0)
	for i, f := range fields {
		if f.interactive() {
			continue
		}
		r := rows[i].r
		if overlayInspectorHit(fields, 0, r.x+12, r.y+12) != -1 {
			t.Fatal("read-only row is clickable")
		}
	}
	v.overlayFocusNext(d, false)
	if !fields[v.overlay.fieldFocus].interactive() {
		t.Fatal("keyboard focus stopped on information")
	}
	// Paid replies are editable at either dialogue depth. Keep root rest on
	// its own tavern NPC, since that fixed surface cannot draw other actions.
	var paidKeys []string
	for _, nested := range []bool{false, true} {
		for _, action := range []string{"tavern_rest", "wait_until_dawn", "wait_until_night"} {
			v.overlay.pendingNPC = ""
			if err := v.overlayPlaceObject(d, overlayEmptyCell(t, v, d)); err != nil {
				t.Fatal(err)
			}
			objectKey := d.state.Data.NPCSpawns[v.overlay.selected].NPCKey
			paidKeys = append(paidKeys, objectKey)
			var parent []int
			if nested {
				parent = []int{0} // The new object's existing info branch.
			} else {
				d.editNPC(objectKey).Dialogue.Choices = nil
			}
			v.overlayAddChoice(d, parent)
			if err := v.overlay.modal.apply(action); err != nil {
				t.Fatal(err)
			}
			v.overlay.modal = nil
			edit("Reply", "Finished "+action+".")
		}
	}
	if err := v.saveOverlay(d); err != nil {
		t.Fatal(err)
	}
	want := d.state.clone()
	v.overlay.documents = nil
	reloaded := v.overlayDoc()
	if !overlaySame(reloaded.state.Adventure, want.Adventure) || !overlaySame(reloaded.state.Mechanics, want.Mechanics) {
		t.Fatal("inspector edits did not survive save and reload")
	}
	count := 0
	for _, objectKey := range paidKeys {
		if err := reloaded.npc(objectKey).Dialogue.WalkChoices(func(c *character.NPCDialogueChoice) error {
			switch c.Action {
			case "tavern_rest", "wait_until_dawn", "wait_until_night":
				count++
				if c.Response != "Finished "+c.Action+"." || c.Cost <= 0 {
					t.Fatalf("paid reply lost after save/reload: %+v", c)
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if count != 6 {
		t.Fatalf("saved %d paid choices, want 6", count)
	}

}
func TestOverlayReferencesAndValidation(t *testing.T) {
	v := overlayTestViewer(t)
	d := v.overlayDoc()
	i := d.addEffect("lane", [4]int{1, 1, 2, 2})
	lane := d.state.Adventure.Effects[i].ID
	d.state.Adventure.Controls = append(d.state.Adventure.Controls, config.EnvironmentControl{ID: "test_control", Disable: []string{lane}})
	d.state.Adventure.Boss.Lanes = []string{lane}
	d.state.Adventure.Boss.Phases = []config.AdventureBossPhase{{BelowPercent: 100, Patterns: [][]string{{lane}}}}
	j := d.addEffect("transfer", [4]int{5, 5, 5, 5})
	d.state.Adventure.Effects[j].TriggerLane = lane
	if err := d.renameID("effect", i, "renamed_lane"); err != nil {
		t.Fatal(err)
	}
	a := &d.state.Adventure
	if a.Effects[j].TriggerLane != "renamed_lane" || a.Boss.Lanes[0] != "renamed_lane" || a.Boss.Phases[0].Patterns[0][0] != "renamed_lane" || a.Controls[len(a.Controls)-1].Disable[0] != "renamed_lane" {
		t.Fatal("rename broke references")
	}
	if err := d.deleteEffect(i); err == nil {
		t.Fatal("referenced effect deleted")
	}
	if err := d.changeKind(i, "occupation"); err == nil {
		t.Fatal("referenced lane converted")
	}
	v.overlaySelect("effect", i)
	v.overlay.tool = "move"
	v.overlay.dragging = true
	v.overlay.beforeDrag = d.state.clone()
	d.state.Adventure.Effects[i].Rect = [4]int{8, 8, 9, 9}
	v.overlayCancelGesture(d)
	if d.state.Adventure.Effects[i].Rect != [4]int{1, 1, 2, 2} || v.overlay.dragging {
		t.Fatal("cancel did not restore gesture")
	}
	var f float64
	if err := overlayFloat("x", &f, "").apply("NaN"); err == nil {
		t.Fatal("NaN accepted")
	}
	for _, kind := range []string{"trap", "occupation", "lane"} {
		d := v.overlayDoc()
		v.overlay.documents = nil
		d.state.Adventure = config.AdventureConfig{OpeningOwned: true, Element: "earth"}
		d.state.Data.NPCSpawns = nil
		d.addEffect(kind, [4]int{1, 1, 1, 1})
		if err := v.validateOverlay(d); err != nil {
			t.Fatalf("%s defaults invalid: %v", kind, err)
		}
	}
}
func TestOverlayTabFitsAndTilePicking(t *testing.T) {
	tabs := pageTabLayout()
	last := tabs[len(tabs)-1]
	if last.page != pageOverlay || last.x+last.w > windowWidth {
		t.Fatalf("Overlay tab not reachable: %+v", last)
	}
	v := &viewer{}
	d := &overlayDocument{state: overlayState{Data: &world.MapData{Width: 20, Height: 20}}}
	l := v.overlayLayout(d)
	if _, ok := overlayTileAt(l, l.originX-1, l.originY+5); ok {
		t.Fatal("left padding picks tile zero")
	}
	tile, ok := overlayTileAt(l, l.originX+2*l.tileSize+1, l.originY+3*l.tileSize+1)
	if !ok || tile != [2]int{2, 3} {
		t.Fatal("wrong cell")
	}
	for _, tc := range []struct {
		name               string
		width, height      int
		zoom, wheel        float64
		panX, panY, mx, my int
	}{
		{"fit zoom in", 40, 40, 1, 1, 0, 0, 401, 371},
		{"panned zoom in", 40, 40, 3, 1, 180, 240, 511, 333},
		{"panned zoom out", 40, 40, 3, -1, 180, 240, 511, 333},
		{"narrow fitting axis", 10, 80, 1, 2, 0, 0, 520, 388},
		{"wide fitting axis", 80, 10, 1, 1, 0, 0, 623, 429},
		{"back to minimum", 40, 40, 1.1, -20, 40, 30, 450, 400},
		{"up to maximum", 40, 40, 7.9, 20, 40, 30, 450, 400},
		{"at minimum", 40, 40, 1, -1, 0, 0, 401, 371},
		{"at maximum", 40, 40, 8, 1, 180, 240, 511, 333},
		{"rounded same size", 40, 40, 1, .01, 0, 0, 401, 371},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := &viewer{overlay: overlayPage{zoom: tc.zoom, panX: tc.panX, panY: tc.panY}}
			d := &overlayDocument{state: overlayState{Data: &world.MapData{Width: tc.width, Height: tc.height}}}
			before := v.overlayLayout(d)
			picked, inside := overlayTileAt(before, tc.mx, tc.my)
			if !inside {
				t.Fatal("fixture cursor is outside the map")
			}
			fx := float64(tc.mx-before.originX) / float64(before.tileSize)
			fy := float64(tc.my-before.originY) / float64(before.tileSize)
			v.scrollOverlayAt(d, tc.mx, tc.my, tc.wheel)
			after := v.overlayLayout(d)
			x := float64(after.originX) + fx*float64(after.tileSize)
			y := float64(after.originY) + fy*float64(after.tileSize)
			if math.Abs(x-float64(tc.mx)) > .500001 || math.Abs(y-float64(tc.my)) > .500001 {
				t.Fatalf("cursor map point moved to (%.3f,%.3f), want (%d,%d)", x, y, tc.mx, tc.my)
			}
			if got, ok := overlayTileAt(after, tc.mx, tc.my); !ok || got != picked {
				t.Fatalf("zoom changed selected tile from %v to %v (inside=%v)", picked, got, ok)
			}
			if v.overlay.zoom < 1 || v.overlay.zoom > 8 {
				t.Fatal("zoom limit escaped")
			}
			if (tc.zoom == 1 && tc.wheel < 0 || tc.zoom == 8 && tc.wheel > 0) && before != after {
				t.Fatal("scrolling against a zoom limit moved the map")
			}
		})
	}
	t.Run("outside canvas", func(t *testing.T) {
		v.overlay = overlayPage{zoom: 3, panX: 180, panY: 240}
		before := v.overlayLayout(d)
		for _, p := range [][2]int{{20, 100}, {850, 200}, {400, 165}, {400, 110}, {400, 720}, {235, 400}, {820, 300}} {
			v.scrollOverlayAt(d, p[0], p[1], -1)
			if after := v.overlayLayout(d); after != before || v.overlay.zoom != 3 {
				t.Fatalf("wheel at %v outside canvas changed map view", p)
			}
		}
	})
}
func TestOverlayUndoAcrossSaveRestoresNPCWiring(t *testing.T) {
	v := overlayTestViewer(t)
	overlaySandbox(t, v)
	d := v.overlayDoc()
	oldID := d.state.Adventure.Controls[0].ID
	key, original := d.controlNPC(oldID)
	if original == nil {
		t.Fatal("missing fixture switch")
	}
	before := overlayClone(original)
	d.checkpoint()
	if err := d.renameID("control", 0, "changed_control"); err != nil {
		t.Fatal(err)
	}
	if err := v.saveOverlay(d); err != nil {
		t.Fatal(err)
	}
	d.history(false)
	if err := v.saveOverlay(d); err != nil {
		t.Fatal(err)
	}
	if !overlaySame(before, character.NPCConfigInstance.NPCs[key]) {
		t.Fatal("undo across save did not restore original NPC actions")
	}
	if err := character.LoadNPCConfig(overlayNPCPath); err != nil {
		t.Fatal(err)
	}
	if !overlaySame(before, character.NPCConfigInstance.NPCs[key]) {
		t.Fatal("restored actions were not saved")
	}
	cell := overlayEmptyCell(t, v, d)
	index, err := d.addControl(cell[0], cell[1], v.tileManager)
	if err != nil {
		t.Fatal(err)
	}
	newKey, _ := d.controlNPC(d.state.Adventure.Controls[index].ID)
	if err := v.saveOverlay(d); err != nil {
		t.Fatal(err)
	}
	d.history(false)
	if err := v.saveOverlay(d); err != nil {
		t.Fatal(err)
	}
	if err := character.LoadNPCConfig(overlayNPCPath); err != nil {
		t.Fatal(err)
	}
	if character.NPCConfigInstance.NPCs[newKey] != nil {
		t.Fatal("undo creation left a saved orphan definition")
	}
	d.history(true)
	if err := v.saveOverlay(d); err != nil {
		t.Fatal(err)
	}
	if err := character.LoadNPCConfig(overlayNPCPath); err != nil {
		t.Fatal(err)
	}
	if character.NPCConfigInstance.NPCs[newKey] == nil {
		t.Fatal("redo did not restore the saved definition")
	}
}

func TestOverlayIndentlessSequenceSplices(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		for _, suffix := range []string{"", "  other: keep\n# footer\n"} {
			raw := strings.ReplaceAll("# header\nmaps:\n  target:\n  - id: old\n    children:\n    - nested\n  - id: second\n"+suffix, "\n", eol)
			got, err := overlayPatch([]byte(raw), []string{"maps", "target"}, []string{"replacement"})
			if err != nil {
				t.Fatal(err)
			}
			var decoded struct {
				Maps map[string]any `yaml:"maps"`
			}
			if err := yaml.Unmarshal(got, &decoded); err != nil {
				t.Fatal(err)
			}
			if !overlaySame(decoded.Maps["target"], []string{"replacement"}) || strings.Contains(string(got), "second") {
				t.Fatalf("sequence tail survived: %s", got)
			}
			if suffix != "" && (!overlayEntryUnchanged([]byte(raw), got, []string{"maps", "other"}) || !bytes.HasSuffix(got, []byte(strings.ReplaceAll("# footer\n", "\n", eol)))) {
				t.Fatalf("neighbor changed: %s", got)
			}
		}
	}
}

func TestOverlayMergedNPCValidationPrecedesWrites(t *testing.T) {
	v := overlayTestViewer(t)
	overlaySandbox(t, v)
	d := v.overlayDoc()
	if err := v.overlayPlaceObject(d, overlayEmptyCell(t, v, d)); err != nil {
		t.Fatal(err)
	}
	// This unrelated disk edit is outside the draft's per-object validation.
	raw, err := os.ReadFile(overlayNPCPath)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = overlayPatch(raw, []string{"npcs", "broken_external"}, map[string]string{"type": "invalid"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overlayNPCPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	before := map[string][]byte{}
	for _, path := range []string{overlayNPCPath, overlayConfigPath, filepath.Join("assets", v.maps[v.mapIndex].Config.File)} {
		before[path], _ = os.ReadFile(path)
	}
	if err := v.saveOverlay(d); err == nil {
		t.Fatal("invalid merged NPC catalog was published")
	}
	for path, want := range before {
		got, _ := os.ReadFile(path)
		if !bytes.Equal(got, want) {
			t.Fatalf("failed validation wrote %s", path)
		}
	}
	if !d.dirty() {
		t.Fatal("failed save discarded the draft")
	}
}

func TestOverlayObjectDeletionSaveUndoReload(t *testing.T) {
	v := overlayTestViewer(t)
	overlaySandbox(t, v)
	d := v.overlayDoc()
	if err := v.overlayPlaceObject(d, overlayEmptyCell(t, v, d)); err != nil {
		t.Fatal(err)
	}
	key := d.state.Data.NPCSpawns[v.overlay.selected].NPCKey
	d.state.Data.NPCSpawns[v.overlay.selected].GroundTile = "water"
	if err := v.saveOverlay(d); err != nil {
		t.Fatal(err)
	}
	// Another placement of the same local definition keeps it alive.
	v.overlay.pendingNPC = key
	if err := v.overlayPlaceObject(d, overlayEmptyCell(t, v, d)); err != nil {
		t.Fatal(err)
	}
	v.overlay.pendingNPC = ""
	v.overlayDelete(d)
	if err := v.saveOverlay(d); err != nil {
		t.Fatal(err)
	}
	if character.NPCConfigInstance.NPCs[key] == nil {
		t.Fatal("deleting one placement removed a shared definition")
	}
	// Reopen the document so ownership must survive serialization.
	v.overlay.documents = nil
	d = v.overlayDoc()
	for i, spawn := range d.state.Data.NPCSpawns {
		if spawn.NPCKey == key {
			v.overlaySelect("object", i)
		}
	}
	v.overlayDelete(d)
	if err := v.saveOverlay(d); err != nil {
		t.Fatal(err)
	}
	if err := character.LoadNPCConfig(overlayNPCPath); err != nil {
		t.Fatal(err)
	}
	if character.NPCConfigInstance.NPCs[key] != nil {
		t.Fatal("deleted local object left its definition")
	}
	d.history(false)
	if err := v.saveOverlay(d); err != nil {
		t.Fatal(err)
	}
	if err := character.LoadNPCConfig(overlayNPCPath); err != nil {
		t.Fatal(err)
	}
	if character.NPCConfigInstance.NPCs[key] == nil {
		t.Fatal("undo did not restore the local definition")
	}
	if d.dirty() {
		t.Fatal("saved scene is still dirty after floor rebuild")
	}
	if err := v.saveOverlay(d); err != nil {
		t.Fatalf("second save conflicts with its own floor rebuild: %v", err)
	}
	// Library entries remain reusable after deleting a placement.
	for i, spawn := range d.state.Data.NPCSpawns {
		if spawn.NPCKey != key {
			v.overlaySelect("object", i)
			key = spawn.NPCKey
			break
		}
	}
	v.overlayDelete(d)
	if err := v.saveOverlay(d); err != nil {
		t.Fatal(err)
	}
	if character.NPCConfigInstance.NPCs[key] == nil {
		t.Fatal("deleted a reusable library definition")
	}
}

func TestOverlayDragHistoryBoundAndDirtyState(t *testing.T) {
	v := overlayTestViewer(t)
	d := v.overlayDoc()
	index := d.addEffect("trap", [4]int{2, 2, 3, 3})
	for i := 0; i < 65; i++ {
		v.overlay.beforeDrag = d.state.clone()
		v.overlay.tool, v.overlay.selected = "move", index
		d.err, d.status = "old error", "Saved"
		d.state.Adventure.Effects[index].Damage++
		v.overlayFinishDrag(d)
	}
	if len(d.undo) != 60 || d.err != "" || d.status != "" || !d.dirty() {
		t.Fatal("drag bypassed shared history bookkeeping")
	}
	want := d.state.Adventure.Effects[index].Damage
	d.history(false)
	if d.state.Adventure.Effects[index].Damage != want-1 {
		t.Fatal("drag undo lost previous state")
	}
	d.history(true)
	if d.state.Adventure.Effects[index].Damage != want {
		t.Fatal("drag redo lost final state")
	}
	d.saved = d.state.clone()
	if d.dirty() {
		t.Fatal("identical authored state is dirty")
	}
	d.state.Data.Floors = nil
	if d.dirty() {
		t.Fatal("derived floors count as authored edits")
	}
	d.state.Data.Tiles[0][0]++
	if !d.dirty() {
		t.Fatal("terrain edit was not detected")
	}
}

func TestOverlayReloadStagesBeforeDiscard(t *testing.T) {
	v := overlayTestViewer(t)
	overlaySandbox(t, v)
	d := v.overlayDoc()
	d.addEffect("trap", [4]int{2, 2, 3, 3})
	before := d.state.clone()
	for _, widget := range v.overlayWidgets(d) {
		if widget.text == "Reload" {
			widget.action()
			break
		}
	}
	if v.overlay.modal == nil || !v.overlay.modal.confirm || !overlaySame(d.state, before) {
		t.Fatal("reload discarded without confirmation")
	}
	if err := os.WriteFile(config.BossMechanicsPath, []byte("maps: [broken"), 0600); err != nil {
		t.Fatal(err)
	}
	v.overlay.modal.apply("")
	if v.overlay.documents[d.key] != d || !overlaySame(d.state, before) || len(d.undo) == 0 || d.err == "" {
		t.Fatal("failed reload discarded the draft")
	}
}

// Overlay moves follow the Maps-tab drag: a placement's own ground travels with
// it, so the saved map has plain floor where it stood and the ground under it.
func TestOverlayMoveCarriesPlacementGround(t *testing.T) {
	for _, boss := range []bool{true, false} {
		t.Run(map[bool]string{true: "boss", false: "object"}[boss], func(t *testing.T) {
			v := overlayTestViewer(t)
			overlaySandbox(t, v)
			d := v.overlayDoc()
			biome := v.currentBiome()
			lava, _ := v.tileManager.GetTileTypeFromKey("solstice_lava")
			floor, _ := v.tileManager.GetTileTypeFromLetterForBiome(floorLetter, biome)
			// A boss must stay inside its arena; an object may go anywhere.
			cell := func() [2]int {
				if a := d.state.Adventure.Boss.Arena; boss && a != nil {
					for y := a[1]; y <= a[3]; y++ {
						for x := a[0]; x <= a[2]; x++ {
							if d.freeTile(x, y, v.tileManager) {
								return [2]int{x, y}
							}
						}
					}
					t.Fatal("no free arena tile")
				}
				return overlayEmptyCell(t, v, d)
			}
			from := cell()
			var ground *string
			if boss {
				if err := v.overlayPlaceBoss(d, from); err != nil {
					t.Fatal(err)
				}
				for i, s := range d.state.Data.MonsterSpawns {
					if s.MonsterKey == d.state.Adventure.Boss.Monster {
						ground = &d.state.Data.MonsterSpawns[i].GroundTile
					}
				}
			} else {
				if err := v.overlayPlaceObject(d, from); err != nil {
					t.Fatal(err)
				}
				ground = &d.state.Data.NPCSpawns[v.overlay.selected].GroundTile
			}
			*ground = "solstice_lava"
			d.state.Data.RebuildFloors(v.tileManager, biome)
			if err := v.saveOverlay(d); err != nil {
				t.Fatal(err)
			}
			to := cell()
			v.overlay.tool = "boss-place"
			if !boss {
				v.overlay.tool = "object-move"
			}
			v.overlayMapPress(d, to)
			if err := v.saveOverlay(d); err != nil {
				t.Fatal(err)
			}

			loaded, err := world.NewMapLoaderWithBiome(v.cfg, biome).LoadMap(filepath.Join("assets", v.maps[v.mapIndex].Config.File))
			if err != nil {
				t.Fatal(err)
			}
			loaded.RebuildFloors(v.tileManager, biome)
			if got := loaded.Tiles[from[1]][from[0]]; got != floor {
				t.Fatalf("vacated cell saved as %v, want plain floor %v", got, floor)
			}
			if loaded.Tiles[to[1]][to[0]] != lava {
				t.Fatal("moved placement left its ground behind")
			}
			carried := false
			for _, s := range loaded.MonsterSpawns {
				carried = carried || boss && s.X == to[0] && s.Y == to[1] && s.GroundTile == "solstice_lava"
			}
			for _, s := range loaded.NPCSpawns {
				carried = carried || !boss && s.X == to[0] && s.Y == to[1] && s.GroundTile == "solstice_lava"
			}
			if !carried {
				t.Fatal("saved placement lost its explicit ground")
			}

			d.history(false)
			if d.state.Data.Tiles[from[1]][from[0]] != lava || d.state.Data.Tiles[to[1]][to[0]] == lava {
				t.Fatal("undo did not put the ground back")
			}
			if v.maps[v.mapIndex].Data.Tiles[from[1]][from[0]] != floor {
				t.Fatal("undoing the draft changed the saved map")
			}
		})
	}
}
