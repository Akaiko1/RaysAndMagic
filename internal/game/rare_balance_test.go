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

func TestRareFoldNearestLegalLanding(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, tc := range []struct {
			name string
			want int
		}{{"clear", 8}, {"endpoint", 7}, {"wall", 4}, {"door", 4}, {"teleporter", 3}, {"edge", 3}, {"region", 3}, {"all_occupied", 0}, {"too_close", 0}, {"first_wall", 0}} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, tc.name), func(t *testing.T) {
				g, c := rareClassGame(t, character.ClassWayfarer, tb)
				c.Skills[character.SkillTranslocation] = &character.Skill{Mastery: character.MasteryGrandMaster}
				c.ActionsRemaining = 0
				c.RTCooldown = 123
				ts := float64(g.config.GetTileSize())
				x, y := g.camera.X, g.camera.Y
				switch tc.name {
				case "endpoint", "all_occupied":
					first := 8
					if tc.name == "all_occupied" {
						first = 3
					}
					for n := first; n <= 8; n++ {
						m := zoneVictim(t, g)
						m.X = x + float64(n)*ts
						m.Y = y
						g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
						g.refreshMonsterCollisionState(m)
					}
				case "wall":
					g.world.Tiles[10][13] = world.TileWall
				case "too_close":
					g.world.Tiles[10][11] = world.TileWall
				case "first_wall":
					g.world.Tiles[10][9] = world.TileWall
				case "door":
					n := woodenDoorNPC()
					n.X = x + 5*ts
					n.Y = y
					g.world.NPCs = append(g.world.NPCs, n)
					g.world.Tiles[9][13] = world.TileWall
					g.world.Tiles[11][13] = world.TileWall
					g.registerLockedDoors()
				case "teleporter":
					wm := world.NewWorldManager(g.config)
					wm.CurrentMapKey = "forest"
					wm.LoadedMaps = map[string]*world.World3D{"forest": g.world}
					wm.GlobalTeleporterRegistry.Teleporters = []world.TeleporterLocation{{MapKey: "forest", X: 12, Y: 10}}
					world.GlobalWorldManager = wm
				case "region":
					wm := world.NewWorldManager(g.config)
					wm.CurrentMapKey = "forest"
					wm.OpenWorld = g.world
					wm.OpenWorldRegions = []world.OpenWorldRegion{{MapKey: "forest", Width: 12, Height: 40, LocalWidth: 12, LocalHeight: 40}, {MapKey: "highlands", OffsetX: 12, Width: 28, Height: 40, LocalWidth: 28, LocalHeight: 40}}
					world.GlobalWorldManager = wm
				case "edge":
					g.world.Width = 12
				}
				sp := c.SpellPoints
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
				} else if c.SpellPoints != sp-5 || c.FlowingStaffCharges() == 0 || c.RareClass.Anchor.X != x {
					t.Fatal("short landing did not commit exactly once")
				}
			})
		}
	}
}

func TestRareSpatialReuseAndAnchorByTier(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for tier, seconds := range []int{12, 15, 18, 24} {
			t.Run(fmt.Sprintf("TB=%v/tier=%d", tb, tier), func(t *testing.T) {
				g, c := rareClassGame(t, character.ClassWayfarer, tb)
				c.Skills[character.SkillTranslocation] = &character.Skill{Mastery: character.SkillMastery(tier)}
				g.party.Members[1] = character.CreateCharacter("Other pilgrim", character.ClassWayfarer, g.config)
				g.party.Members[1].ActionsRemaining = 2
				x, y := g.camera.X, g.camera.Y
				if !g.useTechnique(0, "fold_step", false, false) {
					t.Fatal("Fold rejected")
				}
				tps := g.config.GetTPS()
				if g.spatialReuseFrames != 3*tps || c.RareClass.Anchor.Frames != seconds*tps {
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
				if g.spatialReuseFrames != 3*tps || c.RareClass.Anchor.Frames != seconds*tps {
					t.Fatal("mode switch refreshed or consumed spatial clocks")
				}
				if g.useTechnique(1, "fold_step", false, false) || g.useTechnique(0, "return_step", false, false) {
					t.Fatal("shared lock bypassed after restore")
				}
				if tb {
					// A clock change alone must not permit a second step in the same TB turn.
					g.spatialReuseFrames = 0
					if g.useTechnique(0, "return_step", false, false) {
						t.Fatal("same-turn step bypassed")
					}
					g.startPartyTurn()
				} else {
					g.tickRareClassClocks(3*tps - 1)
					if g.useTechnique(0, "return_step", false, false) {
						t.Fatal("reuse ended early")
					}
					g.tickRareClassClocks(1)
				}
				if c.RareClass.Anchor.Frames != (seconds-3)*tps {
					t.Fatal("anchor clock diverged")
				}
				if !g.useTechnique(0, "return_step", false, false) || math.Hypot(g.camera.X-x, g.camera.Y-y) > .01 || c.RareClass.Anchor.Frames != 0 {
					t.Fatal("Return unavailable when lock ends")
				}
				if g.spatialReuseFrames != 3*tps {
					t.Fatal("Return did not share reuse")
				}
			})
		}
	}
}

func TestRareReturnRequiresExactAnchor(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, state := range []string{"clear", "occupied", "expired", "other_region"} {
			t.Run(fmt.Sprintf("TB=%v/%s", tb, state), func(t *testing.T) {
				g, c := rareClassGame(t, character.ClassWayfarer, tb)
				x, y := g.camera.X, g.camera.Y
				if !g.useTechnique(0, "fold_step", false, false) {
					t.Fatal("Fold rejected")
				}
				if tb {
					g.startPartyTurn()
				} else {
					g.tickRareClassClocks(3 * g.config.GetTPS())
				}
				switch state {
				case "occupied":
					m := zoneVictim(t, g)
					m.X = x
					m.Y = y
				case "expired":
					c.RareClass.Anchor = character.SpatialAnchor{}
				case "other_region":
					c.RareClass.Anchor.MapKey = "elsewhere"
				}
				anchor, sp, px, py := c.RareClass.Anchor, c.SpellPoints, g.camera.X, g.camera.Y
				got := g.useTechnique(0, "return_step", false, false)
				if got != (state == "clear") {
					t.Fatal("Return accepted an invalid anchor")
				}
				if !got && (sp != c.SpellPoints || anchor != c.RareClass.Anchor || px != g.camera.X || py != g.camera.Y || g.spatialReuseFrames != 0) {
					t.Fatal("Return rejection changed state")
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
						region, x, y := canonicalPosition(g.camera.X-float64(g.config.GetTileSize()), g.camera.Y)
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
					tip := buildSimpleItemTooltipWithParty(it, true, c, g.party)
					if !strings.Contains(tip, fmt.Sprintf("%d %s damage to each victim", want, d.Flask.Element)) {
						t.Fatalf("tooltip damage diverged: %s", tip)
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
				region, x, y := canonicalPosition(g.camera.X-float64(g.config.GetTileSize()), g.camera.Y)
				c.RareClass.Anchor = character.SpatialAnchor{MapKey: region, X: x, Y: y, Frames: 1000}
			}
			g.turnBasedExtraMonsterAction = true
			if !g.useTechnique(0, key, false, false) || !g.turnBasedExtraMonsterAction {
				t.Fatal("step erased an already scheduled monster pass")
			}
		})
	}
}
