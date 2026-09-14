package main

import (
	"bytes"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ugataima/internal/boot"
	"ugataima/internal/config"
	"ugataima/internal/world"
)

func TestEditorReplacementCells(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	cfg, _ := boot.LoadGameData()
	maps, err := loadMaps(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var fixture mapInfo
	for _, m := range maps {
		if m.Key == "forest" {
			fixture = m
			break
		}
	}
	if fixture.Data == nil {
		t.Fatal("forest fixture missing")
	}
	for _, kind := range []string{"map", "open_world"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			if err := os.Mkdir("assets", 0700); err != nil {
				t.Fatal(err)
			}
			oldPage := owPage
			t.Cleanup(func() { owPage = oldPage })
			v := &viewer{maps: []mapInfo{fixture}, tileManager: world.GlobalTileManager, savePath: filepath.Join(dir, "fixture.map"), owc: &config.OpenWorldConfig{Placements: map[string]config.OpenWorldPlacement{"forest": {X: 0, Y: 0}}}}
			path := v.savePath
			save := v.saveCurrentMap
			if kind == "open_world" {
				path = openWorldConfigPath
				save = v.owSave
			}
			var first []byte
			for _, revision := range []string{"first", "replacement"} {
				v.maps[0].Header = []string{"# " + revision}
				v.maps[0].EOL = "\r\n"
				v.owc.VoidTile = revision
				if err := save(); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if revision == "first" {
					first = data
				} else if bytes.Equal(first, data) {
					t.Fatal("replacement did not publish new content")
				}
				if kind == "map" {
					if !bytes.HasPrefix(data, []byte("# "+revision+"\r\n")) {
						t.Fatal("header or line endings changed")
					}
					got, err := world.NewMapLoaderWithBiome(cfg, fixture.Config.Biome).LoadMap(path)
					if err != nil {
						t.Fatal(err)
					}
					compareMapData(t, fixture.Data, got)
				} else {
					var got config.OpenWorldConfig
					if err := yaml.Unmarshal(data, &got); err != nil {
						t.Fatal(err)
					}
					if got.VoidTile != v.owc.VoidTile || got.Corridor != v.owc.Corridor || !reflect.DeepEqual(got.Placements, v.owc.Placements) || len(got.Connections) != 0 || len(got.Removals) != 0 {
						t.Fatalf("open-world roundtrip changed: %+v", got)
					}
				}
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(path, "previous")
			if err := os.WriteFile(sentinel, first, 0600); err != nil {
				t.Fatal(err)
			}
			if err := save(); err == nil {
				t.Fatal("invalid target accepted")
			}
			if got, err := os.ReadFile(sentinel); err != nil || !bytes.Equal(got, first) {
				t.Fatal("failed replacement changed target")
			}
			leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp"))
			if err != nil || len(leftovers) != 0 {
				t.Fatalf("temporary siblings leaked: %v %v", leftovers, err)
			}
		})
	}
}
