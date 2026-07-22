package game

import (
	"testing"

	"ugataima/internal/character"
)

// One sizing formula for every floor-anchored billboard: person NPCs, prop
// standees, and loot containers must produce IDENTICAL metrics at close range
// (where no pixel floor applies) and differ at long range only by their
// authored minimum pixel floor.
func TestBillboardSizingSingleFormula(t *testing.T) {
	game, _, ts := tbBehaviorGame(t, 40, 40)
	game.renderHelper = NewRenderingHelper(game)
	game.camera.Angle = 0
	game.camera.FOV = game.config.GetCameraFOV()
	game.camera.ViewDist = game.config.GetViewDistance()

	person := &character.NPC{RenderCategory: "npc", SizeTiles: 1}
	prop := &character.NPC{RenderCategory: "scenery", SizeTiles: 1}

	// Close: both floors are irrelevant - the shared formula must agree exactly.
	nx, ny := game.camera.X+2*ts, game.camera.Y
	near := Distance(game.camera.X, game.camera.Y, nx, ny)
	_, _, personSize, pv := game.renderHelper.NPCSpriteMetrics(person, nx, ny, near)
	_, _, propSize, sv := game.renderHelper.NPCSpriteMetrics(prop, nx, ny, near)
	containerX, containerY, contSize, cv := game.renderHelper.CalculateGroundContainerSpriteMetrics(nx, ny, near, 1)
	if !pv || !sv || !cv {
		t.Fatal("close-range billboards must be visible")
	}
	if personSize != propSize || personSize != contSize {
		t.Fatalf("close-range sizes diverged: person=%d prop=%d container=%d - the single formula split", personSize, propSize, contSize)
	}
	containerXF, containerBottomF, containerSizeF, containerFloatVisible := game.renderHelper.CalculateGroundContainerSpriteMetricsF(nx, ny, near, 1)
	if !containerFloatVisible {
		t.Fatal("close-range float container metrics must be visible")
	}
	if gotX, gotY, gotSize := int(containerXF), int(containerBottomF)-int(containerSizeF), int(containerSizeF); gotX != containerX || gotY != containerY || gotSize != contSize {
		t.Fatalf("container float/int projections diverged: float=(%d,%d,%d) int=(%d,%d,%d)", gotX, gotY, gotSize, containerX, containerY, contSize)
	}

	// Far: only the per-category minimum pixel floor may differ.
	fx, fy := game.camera.X+30*ts, game.camera.Y
	far := Distance(game.camera.X, game.camera.Y, fx, fy)
	_, _, personFar, _ := game.renderHelper.NPCSpriteMetrics(person, fx, fy, far)
	_, _, propFar, _ := game.renderHelper.NPCSpriteMetrics(prop, fx, fy, far)
	if want := game.config.Graphics.NPC.MinSpriteSize; personFar < want {
		t.Fatalf("distant person = %dpx, must hold the readability floor %d", personFar, want)
	}
	if propFar < sceneryMinSpriteSize {
		t.Fatalf("distant prop = %dpx, below its own floor %d", propFar, sceneryMinSpriteSize)
	}
	if propFar > personFar {
		t.Fatalf("prop floor (%d) must not exceed the person floor (%d)", propFar, personFar)
	}
}
