package game

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/graphics"
)

// Every card cap/repeat/ornament offset and the portrait aperture are measured
// from party_member_panel.png. A resized sheet would leave all four cards
// unpainted, so the boot check must reject it - and the shipped art must pass.
func TestPartyCardPanelAssetContract(t *testing.T) {
	t.Chdir("../..")
	g := &MMGame{
		config:  &config.Config{Engine: config.EngineConfig{TPS: 120}},
		sprites: graphics.NewSpriteManager(),
	}

	t.Run("shipped art satisfies the measured contract", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("shipped party_member_panel.png rejected: %v", r)
			}
		}()
		g.validatePartyCardPanelAsset()
	})

	t.Run("panel really is the measured size", func(t *testing.T) {
		panel := g.sprites.GetSprite("party_member_panel")
		if panel == nil {
			t.Fatal("party_member_panel is missing")
		}
		if b := panel.Bounds(); b.Dx() != partyPanelSourceW || b.Dy() != partyPanelSourceH {
			t.Fatalf("panel is %dx%d, want %dx%d", b.Dx(), b.Dy(), partyPanelSourceW, partyPanelSourceH)
		}
	})

	t.Run("aperture fits inside the panel", func(t *testing.T) {
		if panelPortraitX+panelPortraitW > partyPanelSourceW ||
			panelPortraitY+panelPortraitH > partyPanelSourceH {
			t.Fatalf("portrait aperture (%d,%d %dx%d) leaves the %dx%d panel",
				panelPortraitX, panelPortraitY, panelPortraitW, panelPortraitH,
				partyPanelSourceW, partyPanelSourceH)
		}
	})
}
