package game

import (
	"reflect"
	"testing"
)

// Wall torches are static map dressing. Entering a torch-lit map while Fly is
// active must produce the same corner cache as entering it on foot.
func TestJapaneseCastleWallTorchesIgnoreFly(t *testing.T) {
	cfg := loadTestConfig(t)
	wm, _ := loadRealWorldForTest(t, cfg, "japanese_castle")
	castle := wm.GetCurrentWorld()
	if castle == nil {
		t.Fatal("japanese castle world missing")
	}

	g := newTestGame(cfg, castle)
	r := NewRenderer(g)
	walking := append([]wallTorchPoint(nil), r.wallTorches...)
	if len(walking) == 0 {
		t.Fatal("japanese castle should have authored wall torches")
	}

	castle.SetFlyActive(true)
	t.Cleanup(func() { castle.SetFlyActive(false) })
	r.buildWallTorches()
	if !reflect.DeepEqual(r.wallTorches, walking) {
		t.Fatalf("torch cache changed under Fly: got %d points, want %d", len(r.wallTorches), len(walking))
	}
}
