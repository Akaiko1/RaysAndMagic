package game

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/world"
)

// Fold lands on the farthest legal tile from its Grandmaster range down to
// min_range_tiles; obstacles sit relative to those authored bounds.
func TestRareFoldNearestLegalLanding(t *testing.T) {
	rareClassGame(t, character.ClassWayfarer, false)
	fold := config.Technique("fold_step")
	maxR, minR := config.TierValue(fold.Range, int(character.MasteryGrandMaster)), fold.MinRange
	if maxR < minR+2 {
		t.Fatalf("fixture needs a Grandmaster range at least 2 past min_range_tiles: %d..%d", minR, maxR)
	}
	for _, tb := range []bool{false, true} {
		for _, tc := range []struct {
			name string
			want int
		}{{"clear", maxR}, {"endpoint", maxR - 1}, {"wall", minR + 1}, {"door", minR + 1}, {"teleporter", minR}, {"edge", minR}, {"region", minR}, {"all_occupied", 0}, {"too_close", 0}, {"first_wall", 0}} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, tc.name), func(t *testing.T) {
				g, c := rareClassGame(t, character.ClassWayfarer, tb)
				c.Skills[character.SkillTranslocation] = &character.Skill{Mastery: character.MasteryGrandMaster}
				c.ActionsRemaining = 0
				c.RTCooldown = 123
				ts := float64(g.config.GetTileSize())
				x, y := g.camera.X, g.camera.Y
				sx, sy := TileIndex(x, ts), TileIndex(y, ts)
				switch tc.name {
				case "endpoint", "all_occupied":
					first := maxR
					if tc.name == "all_occupied" {
						first = minR
					}
					for n := first; n <= maxR; n++ {
						m := zoneVictim(t, g)
						m.X = x + float64(n)*ts
						m.Y = y
						g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
						g.refreshMonsterCollisionState(m)
					}
				case "wall":
					g.world.Tiles[sy][sx+minR+2] = world.TileWall
				case "too_close":
					g.world.Tiles[sy][sx+minR] = world.TileWall
				case "first_wall":
					g.world.Tiles[sy][sx+1] = world.TileWall
				case "door":
					n := woodenDoorNPC()
					n.X = x + float64(minR+2)*ts
					n.Y = y
					g.world.NPCs = append(g.world.NPCs, n)
					g.world.Tiles[sy-1][sx+minR+2] = world.TileWall
					g.world.Tiles[sy+1][sx+minR+2] = world.TileWall
					g.registerLockedDoors()
				case "teleporter":
					wm := world.NewWorldManager(g.config)
					wm.CurrentMapKey = "forest"
					wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
					wm.GlobalTeleporterRegistry.Teleporters = []world.TeleporterLocation{{MapKey: "forest", X: sx + minR + 1, Y: sy}}
					world.GlobalWorldManager = wm
				case "region":
					wm := world.NewWorldManager(g.config)
					wm.CurrentMapKey = "forest"
					wm.OpenWorld = g.world
					edge := sx + minR + 1
					wm.OpenWorldRegions = []world.OpenWorldRegion{{MapKey: "forest", Width: edge, Height: g.world.Height, LocalWidth: edge, LocalHeight: g.world.Height}, {MapKey: "highlands", OffsetX: edge, Width: g.world.Width - edge, Height: g.world.Height, LocalWidth: g.world.Width - edge, LocalHeight: g.world.Height}}
					world.GlobalWorldManager = wm
				case "edge":
					g.world.Width = sx + minR + 1
				}
				sp := c.SpellPoints
				cost := g.techniqueSPCost(c, fold)
				if got := g.useTechnique(0, "fold_step", false, false); got != (tc.want > 0) {
					t.Fatalf("cast=%v want distance=%d", got, tc.want)
				}
				if math.Abs(g.camera.X-x-float64(tc.want)*ts) > .01 || g.camera.Y != y || c.ActionsRemaining != 0 || c.RTCooldown != 123 {
					t.Fatal("wrong landing or action cost")
				}
				if tc.want == 0 {
					if c.SpellPoints != sp || g.spatialReuseFrames != 0 || c.RareClass.Anchor.Frames != 0 || c.FlowingStaffCharges() != 0 {
						t.Fatal("rejection changed state")
					}
				} else if c.SpellPoints != sp-cost || c.FlowingStaffCharges() == 0 || c.RareClass.Anchor.X != x {
					t.Fatal("short landing did not commit exactly once")
				}
			})
		}
	}
}

func TestRareSpatialReuseAndAnchorByTier(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for tier := 0; tier <= int(character.MasteryGrandMaster); tier++ {
			t.Run(fmt.Sprintf("TB=%v/tier=%d", tb, tier), func(t *testing.T) {
				g, c := rareClassGame(t, character.ClassWayfarer, tb)
				fold, back := config.Technique("fold_step"), config.Technique("return_step")
				seconds := config.TierValue(fold.Duration, tier)
				c.Skills[character.SkillTranslocation] = &character.Skill{Mastery: character.SkillMastery(tier)}
				g.party.Members[1] = character.CreateCharacter("Other pilgrim", character.ClassWayfarer, g.config)
				g.party.Members[1].ActionsRemaining = 2
				x, y := g.camera.X, g.camera.Y
				if !g.useTechnique(0, "fold_step", false, false) {
					t.Fatal("Fold rejected")
				}
				tps := g.config.GetTPS()
				reuse := fold.ReuseSeconds * tps
				if g.spatialReuseFrames != reuse || c.RareClass.Anchor.Frames != seconds*tps {
					t.Fatal("wrong authored duration or reuse")
				}
				// Save through the production snapshot and restore adapter while locked.
				wm := world.NewWorldManager(g.config)
				wm.CurrentMapKey = currentMapKey()
				wm.LoadedMaps = map[string]*world.World3D{wm.CurrentMapKey: g.world}
				saved := g.buildSave(wm)
				raw, err := json.Marshal(saved)
				if err != nil {
					t.Fatal(err)
				}
				var loaded GameSave
				if err = json.Unmarshal(raw, &loaded); err != nil {
					t.Fatal(err)
				}
				g.restoreSavedTurnState(&loaded)
				g.ToggleTurnBasedMode()
				g.ToggleTurnBasedMode()
				if g.spatialReuseFrames != reuse || c.RareClass.Anchor.Frames != seconds*tps {
					t.Fatal("mode switch refreshed or consumed spatial clocks")
				}
				if g.useTechnique(1, "fold_step", false, false) || g.useTechnique(0, "return_step", false, false) {
					t.Fatal("shared lock bypassed after restore")
				}
				elapsed := reuse
				if tb {
					// A clock change alone must not permit a second step in the same TB turn.
					g.spatialReuseFrames = 0
					if g.useTechnique(0, "return_step", false, false) {
						t.Fatal("same-turn step bypassed")
					}
					g.startPartyTurn()
					elapsed = g.combatRoundFrames()
				} else {
					g.tickRareClassClocks(reuse - 1)
					if g.useTechnique(0, "return_step", false, false) {
						t.Fatal("reuse ended early")
					}
					g.tickRareClassClocks(1)
				}
				if c.RareClass.Anchor.Frames != seconds*tps-elapsed {
					t.Fatal("anchor clock diverged")
				}
				if !g.useTechnique(0, "return_step", false, false) || math.Hypot(g.camera.X-x, g.camera.Y-y) > .01 || c.RareClass.Anchor.Frames != 0 {
					t.Fatal("Return unavailable when lock ends")
				}
				if g.spatialReuseFrames != back.ReuseSeconds*tps {
					t.Fatal("Return did not share reuse")
				}
			})
		}
	}
}

func TestRareSpatialMonsterPasses(t *testing.T) {
	for _, key := range []string{"fold_step", "return_step"} {
		for _, stepFirst := range []bool{false, true} {
			for _, paidKind := range []string{"weapon", "technique"} {
				t.Run(fmt.Sprintf("%s/first=%v/%s", key, stepFirst, paidKind), func(t *testing.T) {
					g, c := rareClassGame(t, character.ClassWayfarer, true)
					if key == "return_step" {
						region, x, y := g.canonicalPosition(g.camera.X-float64(g.config.GetTileSize()), g.camera.Y)
						c.RareClass.Anchor = character.SpatialAnchor{MapKey: region, X: x, Y: y, Frames: 1000}
					}
					paid := func() {
						if paidKind == "weapon" {
							g.consumeSelectedCharWeaponAction()
						} else if !g.useTechnique(0, "phase_veil", false, false) {
							t.Fatal("paid action rejected")
						}
					}
					step := func() {
						if !g.useTechnique(0, key, false, false) {
							t.Fatal("step rejected")
						}
					}
					if stepFirst {
						step()
						paid()
					} else {
						paid()
						step()
					}
					if g.turnBasedExtraMonsterAction {
						t.Fatal("spatial step added a monster pass")
					}
					g.endPartyTurn()
					g.monsterTurnState.startPasses()
					if g.turnBasedMonsterPassesLeft != 1 {
						t.Fatal("monster scheduler received extra pass")
					}
				})
			}
		}
	}
	g, _ := rareClassGame(t, character.ClassWayfarer, true)
	g.consumeSelectedCharWeaponAction()
	g.endPartyTurnAfterMovement()
	g.monsterTurnState.startPasses()
	if g.turnBasedMonsterPassesLeft != 2 {
		t.Fatal("ordinary walking lost its extra pass")
	}
}

func TestRareFlaskDamageUsesOnlyIntellect(t *testing.T) {
	for _, key := range []string{"harm_flask", "venom_flask", "fire_flask"} {
		for tier := 0; tier < 4; tier++ {
			for _, stats := range [][2]int{{15, 13}, {60, 50}, {60, 500}} {
				t.Run(fmt.Sprintf("%s/tier=%d/stats=%v", key, tier, stats), func(t *testing.T) {
					g, c := rareClassGame(t, character.ClassAlchemist, false)
					c.Intellect, c.Accuracy = stats[0], stats[1]
					c.Skills[character.SkillBombThrowing] = &character.Skill{Mastery: character.SkillMastery(tier)}
					it, _ := items.TryCreateItemFromYAML(key)
					g.party.AddItem(it)
					d, _ := config.GetItemDefinition(key)
					want := d.Flask.Damage[tier] + c.GetEffectiveIntellect()/3
					if !g.throwFlask(0, key, false) || g.magicProjectiles[0].Damage != want {
						t.Fatalf("projectile damage must be %d", want)
					}
					tip := buildSimpleItemTooltipWithParty(it, true, c, g.party, g.combat)
					if !strings.Contains(tip, fmt.Sprintf("Total Damage: %d to each victim", want)) || !strings.Contains(tip, damageTypeAoELine(d.Flask.Element, float64(d.Flask.RadiusTiles))) {
						t.Fatalf("tooltip damage diverged: %s", tip)
					}
					// The card's recovery is the one the throw just armed.
					if line := cooldownLine(g.combat, c.RTCooldown); !g.turnBasedMode && !strings.Contains(tip, line) {
						t.Fatalf("tooltip recovery %q missing (the throw armed %d frames): %s", line, c.RTCooldown, tip)
					}
				})
			}
		}
	}
}

func TestRareSpatialPreservesExistingMonsterPass(t *testing.T) {
	for _, key := range []string{"fold_step", "return_step"} {
		t.Run(key, func(t *testing.T) {
			g, c := rareClassGame(t, character.ClassWayfarer, true)
			if key == "return_step" {
				region, x, y := g.canonicalPosition(g.camera.X-float64(g.config.GetTileSize()), g.camera.Y)
				c.RareClass.Anchor = character.SpatialAnchor{MapKey: region, X: x, Y: y, Frames: 1000}
			}
			g.turnBasedExtraMonsterAction = true
			if !g.useTechnique(0, key, false, false) || !g.turnBasedExtraMonsterAction {
				t.Fatal("step erased an already scheduled monster pass")
			}
		})
	}
}
