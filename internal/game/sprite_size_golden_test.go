package game

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// TestSpriteSizesGolden pins the ON-SCREEN pixel height of every billboard
// entity (monster, NPC, environment tile) at fixed distances. It is the guard
// for the size-scale unification: after values move to the wall-relative
// (tile-height) scale and the three projection paths collapse onto one
// formula, EVERY key here must still render at the exact same pixel size.
//
// The golden file is generated from the current code once and committed; it is
// NOT regenerated as part of the refactor. Regenerate deliberately with
// RAM_UPDATE_SPRITE_GOLDEN=1 only when a real size change is intended.
func TestSpriteSizesGolden(t *testing.T) {
	got := computeGoldenSpriteSizes(t)
	path := filepath.Join("testdata", "sprite_sizes_golden.json")

	if os.Getenv("RAM_UPDATE_SPRITE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		data, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %d golden sprite sizes to %s", len(got), path)
		return
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (regenerate with RAM_UPDATE_SPRITE_GOLDEN=1): %v", err)
	}
	want := map[string]int{}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}

	keys := make([]string, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		gv, ok := got[k]
		if !ok {
			t.Errorf("%s: missing from current output (golden %d)", k, want[k])
			continue
		}
		if gv != want[k] {
			t.Errorf("%s: got %d px, want %d px (size scale changed the rendered size)", k, gv, want[k])
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("%s: new entity absent from golden (regenerate if intended)", k)
		}
	}
}

// computeGoldenSpriteSizes renders every entity straight ahead of the camera
// (so perpendicular distance == the requested distance) at a couple of ranges,
// through the exact metric functions the engine uses, and returns key -> px.
func computeGoldenSpriteSizes(t *testing.T) map[string]int {
	t.Helper()
	cfg := loadTestConfig(t) // config + monsters
	if world.GlobalTileManager == nil {
		world.GlobalTileManager = world.NewTileManager()
	}
	if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	if err := character.LoadNPCConfig("../../assets/npcs.yaml"); err != nil {
		t.Fatalf("load npcs: %v", err)
	}

	g := &MMGame{
		config: cfg,
		world:  &world.World3D{},
		camera: &FirstPersonCamera{X: 320, Y: 320, Angle: 0, FOV: cfg.GetCameraFOV(), ViewDist: cfg.GetViewDistance()},
	}
	g.renderHelper = NewRenderingHelper(g)

	// Both distances clear the 5-tile environment near-cull and sit well inside
	// the 50-tile view distance.
	distances := []float64{384, 640}
	out := map[string]int{}

	for _, d := range distances {
		ex := g.camera.X + d // straight ahead: perpDist == d
		ey := g.camera.Y

		for _, key := range monster.MonsterConfig.GetAllMonsterKeys() {
			def := monster.MonsterConfig.Monsters[key]
			_, _, size, _ := g.renderHelper.CalculateMonsterSpriteMetrics(ex, ey, d, def.GetSizeGameMultiplier())
			out[fmt.Sprintf("monster/%s@%.0f", key, d)] = size
		}

		for key, data := range character.NPCConfigInstance.NPCs {
			npc := &character.NPC{
				Sprite:         data.Sprite,
				RenderCategory: data.RenderCategory,
				SizeClass:      data.SizeClass,
				SizeTiles:      data.SizeTiles,
			}
			_, _, size, _ := g.renderHelper.NPCSpriteMetrics(npc, ex, ey, d)
			out[fmt.Sprintf("npc/%s@%.0f", key, d)] = size
		}

		for _, key := range world.GlobalTileManager.GetAllTileKeys() {
			data := world.GlobalTileManager.GetTileDataByKey(key)
			if data == nil {
				continue
			}
			switch data.RenderType {
			case "tree_sprite", "environment_sprite", "flooring_object", "landmark":
			default:
				continue
			}
			tt, ok := world.GlobalTileManager.GetTileTypeFromKey(key)
			if !ok {
				continue
			}
			_, _, size, _ := g.renderHelper.CalculateEnvironmentSpriteMetrics(ex, ey, d, tt, 1.0)
			out[fmt.Sprintf("tile/%s@%.0f", key, d)] = size
		}
	}
	return out
}
