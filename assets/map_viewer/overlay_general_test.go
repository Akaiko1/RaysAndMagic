package main

import (
	"bytes"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/world"
)

func TestOverlayGeneralSceneRoundTrip(t *testing.T) {
	v := overlayTestViewer(t)
	for i, m := range v.maps {
		if m.Key == "forest" {
			v.mapIndex = i
		}
	}
	overlaySandbox(t, v)
	d := v.overlayDoc()
	beforeConfig, _ := os.ReadFile(overlayConfigPath)
	beforeNPC, _ := os.ReadFile(overlayNPCPath)
	original := d.state.clone()
	v.overlayBossPicker(d)
	if err := v.overlay.modal.apply("solstice_neressa"); err != nil {
		t.Fatal(err)
	}
	v.overlay.modal = nil
	if err := v.overlayPlaceBoss(d, overlayEmptyCell(t, v, d)); err != nil {
		t.Fatal(err)
	}
	tile := overlayEmptyCell(t, v, d)
	index, err := d.addControl(tile[0], tile[1], v.tileManager)
	if err != nil {
		t.Fatal(err)
	}
	id := d.state.Adventure.Controls[index].ID
	d.state.Adventure.Controls[index].StopHealing = true
	d.state.Adventure.Boss.HealPercent = 5
	d.state.Adventure.Boss.HealCapPercent = 25
	d.state.Mechanics = []config.BossMechanic{{ID: "crit", Stat: "critical_chance", Base: 70, Floor: 10, Reductions: map[string]int{id: 30}}, {ID: "dodge", Stat: "perfect_dodge", Base: 45, Reductions: map[string]int{id: 45}}}
	v.overlay.pendingNPC = ""
	if err := v.overlayPlaceObject(d, overlayEmptyCell(t, v, d)); err != nil {
		t.Fatal(err)
	}
	object := d.state.Data.NPCSpawns[v.overlay.selected].NPCKey
	d.state.NPCs[object].Dialogue.Choices[0].Choices = []*character.NPCDialogueChoice{{Text: "Use switch", Action: "adventure_control", Control: id}}
	if err := d.renameID("control", index, "linked_switch"); err != nil {
		t.Fatal(err)
	}
	if !overlayHasAction(d.npc(object), "adventure_control", "linked_switch") || d.state.Mechanics[0].Reductions["linked_switch"] != 30 {
		t.Fatal("nested or mechanic link not renamed")
	}
	if err := d.deleteControl(index); err == nil {
		t.Fatal("referenced boss control deleted")
	}
	if err := v.saveOverlay(d); err != nil {
		t.Fatal(err)
	}
	configAfter, _ := os.ReadFile(overlayConfigPath)
	if !overlayEntryUnchanged(beforeConfig, configAfter, []string{"maps", "solstice_water"}) {
		t.Fatal("another map was rewritten")
	}
	npcAfter, _ := os.ReadFile(overlayNPCPath)
	if !bytes.HasPrefix(npcAfter, beforeNPC) {
		t.Fatal("existing NPC catalog bytes changed while adding objects")
	}
	mapPath := filepath.Join("assets", v.maps[v.mapIndex].Config.File)
	raw, _ := os.ReadFile(mapPath)
	if !bytes.Contains(raw, []byte("[monster:solstice_neressa")) {
		t.Fatal("foreign biome boss was not saved by key")
	}
	loaded, err := world.NewMapLoaderWithBiome(v.cfg, "forest").LoadMap(mapPath)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(loaded.MonsterSpawns, func(m world.MonsterSpawn) bool { return m.MonsterKey == "solstice_neressa" }) {
		t.Fatal("boss key lost on reload")
	}
	rules, err := config.ReadBossMechanics(config.BossMechanicsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !overlaySame(rules.Maps[d.key], d.state.Mechanics) {
		t.Fatal("mechanics lost on disk")
	}
	if err := character.LoadNPCConfig(overlayNPCPath); err != nil {
		t.Fatal(err)
	}
	if !overlayHasAction(character.NPCConfigInstance.NPCs[object], "adventure_control", "linked_switch") {
		t.Fatal("nested action lost on disk")
	}
	saved := d.state.clone()
	// Undo past creation after Save, save it, then restore the exact scene by Redo.
	d.undo = []overlayState{original}
	d.history(false)
	if err := v.saveOverlay(d); err != nil {
		t.Fatal(err)
	}
	if character.NPCConfigInstance.NPCs[object] != nil {
		t.Fatal("undo left an orphaned new definition")
	}
	d.history(true)
	if !overlaySame(d.state, saved) {
		t.Fatal("redo lost scene data")
	}
	if err := v.saveOverlay(d); err != nil {
		t.Fatal(err)
	}
	v.overlay.documents = nil
	reloaded := v.overlayDoc()
	if !overlaySame(reloaded.state.Mechanics, saved.Mechanics) || !overlaySame(reloaded.state.Adventure, saved.Adventure) {
		t.Fatal("document reload lost scene rules")
	}
	// An external edit to the same map's mechanics must be rejected before any write.
	changed, _ := overlayPatch(d.mechanicsBase, []string{"maps", d.key}, []config.BossMechanic{{ID: "external", Stat: "critical_chance", Base: 10}})
	if err := os.WriteFile(config.BossMechanicsPath, changed, 0600); err != nil {
		t.Fatal(err)
	}
	d.state.Mechanics[0].Base = 90
	snapshot, _ := os.ReadFile(overlayConfigPath)
	if err := v.saveOverlay(d); err == nil {
		t.Fatal("mechanic disk conflict overwritten")
	}
	actual, _ := os.ReadFile(overlayConfigPath)
	if !bytes.Equal(snapshot, actual) {
		t.Fatal("config changed on failed save")
	}
}
func TestOverlayKeyedMonstersPreserveTerrainAndRejectInvalidTokens(t *testing.T) {
	v := overlayTestViewer(t)
	d := v.overlayDoc()
	ground, _ := v.tileManager.GetTileTypeFromKey("dragon_cliffs_bridge")
	m := v.maps[v.mapIndex]
	m.Data = &world.MapData{Width: 4, Height: 1, StartX: 0, StartY: 0, Tiles: [][]world.TileType3D{{world.TileSpawn, ground, world.TileEmpty, world.TileEmpty}}, MonsterSpawns: []world.MonsterSpawn{{X: 1, MonsterKey: "solstice_neressa"}}, NPCSpawns: []world.NPCSpawn{{X: 2, NPCKey: d.state.Data.NPCSpawns[0].NPCKey}}, SpecialTileSpawns: []world.SpecialTileSpawn{{X: 3, TileKey: "vteleporter"}}}
	lines, err := encodeMapLines(&m, v.tileManager)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "mixed.map")
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(strings.Join(lines, "\n"))
	loader := world.NewMapLoaderWithBiome(v.cfg, m.Config.Biome)
	got, err := loader.LoadMap(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.MonsterSpawns[0].MonsterKey != "solstice_neressa" || got.Tiles[0][1] != ground || got.NPCSpawns[0].X != 2 || got.SpecialTileSpawns[0].X != 3 {
		t.Fatalf("mixed tokens shifted or ground lost: %+v", got)
	}
	for _, text := range []string{"+@ >[monster:unknown]", "+@ >[monster:solstice_neressa@missing_ground]", "+@ >[monster:solstice_neressa@]", "+@ >[monster:solstice_neressa], [monster:solstice_neressa]", "+@@ >[monster:solstice_neressa]"} {
		write(text)
		if _, err := loader.LoadMap(path); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
}
func TestOverlayMechanicsPreserveNeighborBytes(t *testing.T) {
	raw := []byte("# authored mechanics\nmaps:\n  other:\n    - id: crit\n      stat: critical_chance\n      base: 20\n# footer\n")
	dir := t.TempDir()
	t.Chdir(dir)
	os.Mkdir("assets", 0700)
	os.WriteFile(config.BossMechanicsPath, raw, 0600)
	d := &overlayDocument{key: "target", mechanicsBase: raw, state: overlayState{Mechanics: []config.BossMechanic{{ID: "dodge", Stat: "perfect_dodge", Base: 20}}}}
	maps := &config.MapConfigs{Maps: map[string]config.MapConfig{}}
	for _, key := range []string{"target", "other"} {
		maps.Maps[key] = config.MapConfig{Adventure: &config.AdventureConfig{Boss: &config.AdventureBoss{Monster: "boss"}}}
	}
	w, err := overlayMechanicsWrite(d, maps)
	if err != nil {
		t.Fatal(err)
	}
	if !overlayEntryUnchanged(raw, w.after, []string{"maps", "other"}) || !bytes.HasPrefix(w.after, []byte("# authored mechanics\n")) || !bytes.HasSuffix(w.after, []byte("# footer\n")) {
		t.Fatalf("neighbor bytes changed: %s", w.after)
	}
	var decoded config.BossMechanicsConfig
	if err := yaml.Unmarshal(w.after, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Maps) != 2 {
		t.Fatal("lost a map")
	}
}

func TestOverlayPublishRemovesNewFileOnRollback(t *testing.T) {
	dir := t.TempDir()
	created := filepath.Join(dir, "new.yaml")
	existing := filepath.Join(dir, "existing.yaml")
	os.WriteFile(existing, []byte("unchanged"), 0600)
	files := []overlayWrite{{created, nil, []byte("new")}, {existing, []byte("unchanged"), []byte("changed")}}
	err := overlayPublish(files, func(path string, data []byte, mode os.FileMode) error {
		if path == existing {
			return fmt.Errorf("disk full")
		}
		return os.WriteFile(path, data, mode)
	})
	if err == nil {
		t.Fatal("failed transaction reported success")
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatal("rollback left newly created mechanics file")
	}
	actual, _ := os.ReadFile(existing)
	if string(actual) != "unchanged" {
		t.Fatal("rollback changed existing file")
	}
}

func TestOverlayMechanicsIndependentDrafts(t *testing.T) {
	for _, initial := range []string{"", "maps: {}\n", "maps:\n"} {
		t.Run(fmt.Sprintf("initial=%q", initial), func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.Mkdir("assets", 0700); err != nil {
				t.Fatal(err)
			}
			if initial != "" {
				if err := os.WriteFile(config.BossMechanicsPath, []byte(initial), 0600); err != nil {
					t.Fatal(err)
				}
			}
			maps := &config.MapConfigs{Maps: map[string]config.MapConfig{}}
			var drafts []*overlayDocument
			for _, key := range []string{"first", "second"} {
				maps.Maps[key] = config.MapConfig{Adventure: &config.AdventureConfig{Boss: &config.AdventureBoss{Monster: "boss"}}}
				drafts = append(drafts, &overlayDocument{key: key, mechanicsBase: []byte(initial), state: overlayState{Mechanics: []config.BossMechanic{{ID: "crit", Stat: "critical_chance", Base: 20}}}})
			}
			for _, d := range drafts {
				w, err := overlayMechanicsWrite(d, maps)
				if err != nil {
					t.Fatal(err)
				}
				if err := overlayPublish([]overlayWrite{w}, os.WriteFile); err != nil {
					t.Fatal(err)
				}
			}
			rules, err := config.ReadBossMechanics(config.BossMechanicsPath)
			if err != nil || len(rules.Maps) != 2 {
				t.Fatalf("second draft lost mechanics: %v, %v", rules, err)
			}
			if _, err := overlayMechanicsWrite(drafts[0], maps); err == nil {
				t.Fatal("same-entry conflict was not detected")
			}
		})
	}
}

// A type switch to or from a guild must leave content the boot accepts: the
// guild model, landmark, membership terms and reserved services follow the type,
// and nothing guild-only stays behind where the inspector cannot remove it.
func TestOverlayGuildTypeSwitchStaysValid(t *testing.T) {
	for _, tc := range []struct {
		name  string
		start func(n *character.NPCData)
	}{
		{"object with dialogue choices", func(*character.NPCData) {}},
		{"crate drawn as a 3D model", func(n *character.NPCData) {
			n.Type, n.Dialogue = character.NPCTypeLootCrate, nil
			n.PropModel = &character.NPCPropModel{Shape: "crates", UseSeconds: 1.2}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := overlayTestViewer(t)
			overlaySandbox(t, v)
			d := v.overlayDoc()
			if err := v.overlayPlaceObject(d, overlayEmptyCell(t, v, d)); err != nil {
				t.Fatal(err)
			}
			key := d.state.Data.NPCSpawns[v.overlay.selected].NPCKey
			// The start need not be valid itself: a bare crate has no loot table.
			tc.start(d.state.NPCs[key])
			setType := func(s string) {
				t.Helper()
				for _, f := range v.overlayFields(d) {
					if f.label == "Object type" {
						if err := f.apply(s); err != nil {
							t.Fatal(err)
						}
						return
					}
				}
				t.Fatal("missing Object type field")
			}
			setType(character.NPCTypeThievesGuild)
			n := d.state.NPCs[key]
			if err := v.validateOverlayObjects(d); err != nil {
				t.Fatalf("switched guild is invalid: %v", err)
			}
			if n.Guild == nil || n.Guild.MembershipGold <= 0 || n.Dialogue == nil || len(n.Dialogue.Choices) != 0 {
				t.Fatal("guild did not take authored terms or kept ordinary choices")
			}
			setType(character.NPCTypeEncounter)
			n = d.state.NPCs[key]
			if n.Guild != nil || n.PropModel != nil && n.PropModel.Shape == "thieves_guild" {
				t.Fatal("guild-only parts survived the switch away")
			}
		})
	}
}
