package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func TestTraversalBuffValidationRunsAtBoot(t *testing.T) {
	cfg := setupPreviewSandboxTest(t)
	const id = "review_unregistered_passage"
	spellConfig := config.GlobalSpells
	spellConfig.Spells[id] = &config.SpellDefinitionConfig{IsUtility: true, Duration: 10, TerrainPassage: true}
	t.Cleanup(func() { delete(spellConfig.Spells, id) })
	defer func() {
		failure := recover()
		if failure == nil || !strings.Contains(fmt.Sprint(failure), id) {
			t.Fatalf("boot did not reject unregistered traversal spell: %v", failure)
		}
	}()
	preview, err := NewFxPreview(cfg)
	if err != nil {
		t.Fatal(err)
	}
	preview.g.Shutdown()
}

func TestTraversalBuffRegistryValidationAndActivation(t *testing.T) {
	for _, tc := range []struct {
		name            string
		fly, registered bool
	}{
		{"fly_alias", true, false},
		{"registered_passage", false, true},
		{"missing_timer", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, gl, _ := tbBehaviorGame(t, 10, 10)
			g.gameLoop = gl
			cfg := config.GlobalSpells
			const id = "review_traversal"
			cfg.Spells[id] = &config.SpellDefinitionConfig{IsUtility: true, Duration: 10, Fly: tc.fly, TerrainPassage: true}
			t.Cleanup(func() { delete(cfg.Spells, id) })
			active, frames := false, 0
			gl.timedBuffRegistry = g.buildTimedBuffs()
			if tc.registered {
				gl.timedBuffRegistry = append(gl.timedBuffRegistry, timedBuff{id: id, active: &active, duration: &frames})
			}
			err := g.validateTraversalBuffs()
			if !tc.fly && !tc.registered {
				if err == nil || !strings.Contains(err.Error(), id) {
					t.Fatalf("missing timer was not rejected: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			def, err := spells.GetSpellDefinitionByID(id)
			if err != nil {
				t.Fatal(err)
			}
			if g.combat.activateUtilityTimedBuff(id, def, spells.UtilitySpellResult{}, 60) != timedBuffApplied || !g.partyHasTerrainPassage() {
				t.Fatal("validated spell did not activate terrain passage")
			}
			if tc.registered && (!active || frames != 60) {
				t.Fatal("passage did not use its registered timer")
			}
		})
	}
}

func TestTerrainPassageProviders(t *testing.T) {
	for _, provider := range []string{"fly", "spirit", "both", "non_provider"} {
		t.Run(provider, func(t *testing.T) {
			g, gl, ts := tbBehaviorGame(t, 12, 12)
			setTestWorldManager(t, nil)
			previous := world.GlobalTileManager
			t.Cleanup(func() { world.GlobalTileManager = previous })
			world.GlobalTileManager = world.NewTileManager(testTileSizeClasses())
			if err := world.GlobalTileManager.LoadTileConfig("../../assets/tiles.yaml"); err != nil {
				t.Fatal(err)
			}
			cfg := config.GlobalSpells
			cfg.Spells["spirit_walk_fixture"] = &config.SpellDefinitionConfig{TerrainPassage: provider != "non_provider"}
			t.Cleanup(func() { delete(cfg.Spells, "spirit_walk_fixture") })
			spiritActive, spiritDuration := false, 0
			g.gameLoop = gl
			gl.timedBuffRegistry = append(g.buildTimedBuffs(), timedBuff{id: "spirit_walk_fixture", active: &spiritActive, duration: &spiritDuration})
			if provider == "fly" || provider == "both" {
				g.activateTimedBuffFrames("fly", 3, false)
			}
			if provider != "fly" {
				if provider == "non_provider" {
					g.activateTimedBuffFrames("spirit_walk_fixture", 5, false)
				} else {
					def, err := spells.GetSpellDefinitionByID("spirit_walk_fixture")
					if err != nil {
						t.Fatal(err)
					}
					if g.combat.activateUtilityTimedBuff(def.ID, def, spells.UtilitySpellResult{}, 5) != timedBuffApplied {
						t.Fatal("a distinct passage spell did not activate its own buff")
					}
				}
			}
			chasm := terrainTile(t, "dragon_cliffs_chasm_floor")
			g.world.Tiles[5][5], g.world.Tiles[5][6] = chasm, chasm
			g.world.Tiles[5][7] = world.TileWall
			placePlayerAtTile(g, 5, 5, ts)
			x, y := g.camera.X, g.camera.Y
			if provider != "non_provider" {
				if sx, sy := g.safePartyDestination(x, y); sx != x || sy != y {
					t.Fatal("placement ignored active traversal provider")
				}
				// Casting and indoor restrictions remain spell-specific: removing
				// Fly must leave a separate provider's movement permission intact.
				if provider == "both" {
					g.dropFlyWithoutOpenSky()
					if g.flyActive || !g.partyHasTerrainPassage() {
						t.Fatal("indoor Fly removal revoked the other provider")
					}
					g.activateTimedBuffFrames("fly", 3, false)
				}
			}
			for tick := 1; tick <= 5; tick++ {
				gl.updateSpecialEffects()
				want := (provider == "fly" || provider == "both") && tick < 3 ||
					(provider == "spirit" || provider == "both") && tick < 5
				if g.partyHasTerrainPassage() != want {
					t.Fatalf("tick %d: wrong aggregate capability", tick)
				}
				if g.world.IsTileBlocking(5, 5) == want || g.world.IsTileBlocking(7, 5) == want {
					t.Fatalf("tick %d: movement did not follow capability", tick)
				}
				if g.canReachWorldReward(6.5*ts, 5.5*ts) != want {
					t.Fatalf("tick %d: pickup disagrees with movement capability", tick)
				}
				if want && (g.camera.X != x || g.camera.Y != y) {
					t.Fatalf("tick %d: an active provider was grounded early", tick)
				}
			}
			if provider != "non_provider" && g.world.IsTileBlockingTerrainAt(TileIndex(g.camera.X, ts), TileIndex(g.camera.Y, ts)) {
				t.Fatal("last provider expired without grounding the party")
			}
			// Reset is shared too: a future provider must not survive new game
			// or preview reset merely because its spell ID is unfamiliar.
			g.activateTimedBuffFrames("spirit_walk_fixture", 10, false)
			g.resetTimedEffects()
			if spiritActive || g.partyHasTerrainPassage() {
				t.Fatal("reset retained a provider")
			}
		})
	}
}
