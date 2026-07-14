package game

import (
	"math"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/world"
)

// TestUpdateFocusedNPC_CenterAndRangeRules pins the Space-to-interact focus
// rule: the NPC must be within ~1 adjacent tile AND roughly centred on screen.
func TestUpdateFocusedNPC_CenterAndRangeRules(t *testing.T) {
	cfg := loadTestConfig(t)
	prevWM := world.GlobalWorldManager
	world.GlobalWorldManager = nil
	t.Cleanup(func() { world.GlobalWorldManager = prevWM })

	w := &world.World3D{}
	g := &MMGame{
		config: cfg,
		world:  w,
		camera: &FirstPersonCamera{X: 320, Y: 320, Angle: 0, FOV: cfg.GetCameraFOV(), ViewDist: cfg.GetViewDistance()},
	}
	g.renderHelper = NewRenderingHelper(g)

	tile := float64(cfg.GetTileSize())
	ahead := &character.NPC{RenderCategory: "npc", Name: "Ahead", X: g.camera.X + tile, Y: g.camera.Y}
	behind := &character.NPC{RenderCategory: "npc", Name: "Behind", X: g.camera.X - tile, Y: g.camera.Y}
	far := &character.NPC{RenderCategory: "npc", Name: "Far", X: g.camera.X + 3*tile, Y: g.camera.Y}
	offCenter := &character.NPC{RenderCategory: "npc", Name: "OffCenter", X: g.camera.X + tile, Y: g.camera.Y + tile}
	w.NPCs = []*character.NPC{behind, far, offCenter, ahead}

	name := func() string {
		if g.focusedNPC == nil {
			return "nil"
		}
		return g.focusedNPC.Name
	}

	g.updateFocusedNPC()
	if g.focusedNPC != ahead {
		t.Fatalf("facing +X: focused = %s, want Ahead (adjacent + centred; Far is out of range, OffCenter is outside the centre band, Behind is behind)", name())
	}

	// Turn around: Behind is now the centred adjacent NPC.
	g.camera.Angle = math.Pi
	g.updateFocusedNPC()
	if g.focusedNPC != behind {
		t.Fatalf("facing -X: focused = %s, want Behind", name())
	}

	// Face +Y: only OffCenter is vaguely that way, but at 45 degrees off the
	// view axis it must not count as centred.
	g.camera.Angle = math.Pi / 2
	g.updateFocusedNPC()
	if g.focusedNPC != nil {
		t.Fatalf("facing +Y: focused = %s, want nil (45-degree NPC is not centred)", name())
	}

	// Spent statue: hidden-when-visited NPCs never take focus.
	g.camera.Angle = 0
	ahead.HideWhenVisited, ahead.Visited = true, true
	g.updateFocusedNPC()
	if g.focusedNPC != nil {
		t.Fatalf("visited statue: focused = %s, want nil", name())
	}
}
