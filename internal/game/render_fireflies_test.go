package game

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/world"
)

func TestFireflySwarmMoteLayout(t *testing.T) {
	if got, want := len(fireflySwarmMotes), 10; got != want {
		t.Fatalf("firefly_swarm mote count = %d, want %d", got, want)
	}
	for i, mote := range fireflySwarmMotes {
		if mote.u <= 0 || mote.u >= 1 || mote.v <= 0 || mote.v >= 1 {
			t.Fatalf("firefly_swarm mote %d outside sprite-normalized bounds: u=%v v=%v", i, mote.u, mote.v)
		}
	}
}

func TestFireflySwarmFlickerRangeAndMotion(t *testing.T) {
	first := fireflySwarmFlicker(17, 0)
	changed := false
	for frame := int64(1); frame < 240; frame++ {
		got := fireflySwarmFlicker(17, frame)
		if got < 0.40 || got > 1.25 {
			t.Fatalf("fireflySwarmFlicker frame %d = %v, want in [0.40, 1.25]", frame, got)
		}
		if got != first {
			changed = true
		}
	}
	if !changed {
		t.Fatal("fireflySwarmFlicker stayed constant")
	}
}

func TestFireflySwarmDispatchesFromAuthoredEffect(t *testing.T) {
	cfg := loadTestConfig(t)
	previous := world.GlobalTileManager
	t.Cleanup(func() { world.GlobalTileManager = previous })
	tm := world.NewTileManager(cfg.Graphics.SizeClasses)
	if err := tm.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
		t.Fatalf("load tiles: %v", err)
	}
	world.GlobalTileManager = tm
	tileType, ok := tm.GetTileTypeFromKey("firefly_swarm")
	if !ok {
		t.Fatal("firefly_swarm tile is missing")
	}
	if tile := tm.GetTileData(tileType); tile == nil || tile.ProceduralEffect != config.TileEffectFireflySwarm {
		t.Fatalf("firefly_swarm procedural effect = %+v", tile)
	}
	if !isFireflySwarmTile(tileType) {
		t.Fatal("authored firefly swarm effect did not reach render dispatch")
	}
}
