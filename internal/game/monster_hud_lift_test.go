package game

import (
	"math"
	"testing"
)

// A small creature whose feet would sit under the party panel is lifted so
// they show above it - in Classic and in Panini, where the column's
// magnification is part of what the panel covers. Plain Wide frames the
// floor above the panel and lifts nothing; no view lifts a creature at the
// camera.
func TestMonsterHUDLiftKeepsFeetAboveThePanel(t *testing.T) {
	g, _, _, _, _ := mouseCombatHarness(t, false)
	g.renderHelper = NewRenderingHelper(g)
	g.showPartyStats = true
	ts := g.config.GetTileSize()
	viewport := float64(worldViewportBottom(g))
	if float64(g.worldHeight()) <= viewport {
		t.Fatal("fixture has no panel over the world")
	}
	// Far enough out for the full lift.
	depth := (g.config.Graphics.View.LiftFullTiles + 1) * ts
	const size = 20.0
	for _, view := range []struct {
		name         string
		wide, panini bool
		wantLift     bool
	}{
		{name: "classic", wantLift: true},
		{name: "wide", wide: true},
		{name: "panini", wide: true, panini: true, wantLift: true},
	} {
		g.combatPreferences.WideView, g.combatPreferences.PaniniDisabled = view.wide, !view.panini
		p := g.widePaniniProjection()
		if view.panini != (p.distance != 0) {
			t.Fatalf("%s: panini projection active = %v", view.name, p.distance != 0)
		}
		for _, x := range []float64{float64(g.worldWidth()) / 2, float64(g.worldWidth()) * .1} {
			// Feet that land halfway down the panel once presented.
			target := viewport + (float64(g.worldHeight())-viewport)/2
			ground := target
			if p.distance != 0 {
				ground = p.horizon + (target-p.horizon)/p.columnMagnification(x)
			}
			lift := g.monsterHUDLift(depth, ground, size, x)
			if !view.wantLift {
				if lift != 0 {
					t.Fatalf("%s x=%.0f: lift %.2f, want none", view.name, x, lift)
				}
				continue
			}
			feet := p.presentedY(x, ground-lift)
			if math.Abs(feet-viewport) > 1e-6 {
				t.Fatalf("%s x=%.0f: presented feet at %.2f, want the panel edge %.2f", view.name, x, feet, viewport)
			}
			if near := g.monsterHUDLift(g.config.Graphics.View.LiftFadeTiles*ts, ground, size, x); near != 0 {
				t.Fatalf("%s x=%.0f: lift %.2f at the camera, want none", view.name, x, near)
			}
		}
	}
}
