package game

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"reflect"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

func travelFixture(t *testing.T) (*MMGame, *world.WorldManager, float64) {
	t.Helper()
	g, ts := summonTileWorld(t)
	g.appScreen = AppScreenInGame
	storage.SetDataRootForTesting(t.TempDir())
	t.Cleanup(func() { storage.SetDataRootForTesting("") })
	wm := world.NewWorldManager(g.config)
	wm.CurrentMapKey = "forest"
	wm.LoadedMaps["forest"] = g.world
	for _, key := range []string{"other", "water"} {
		w := newTestWorldSized(g.config, 40, 40)
		w.StartX, w.StartY = 6, 7
		wm.LoadedMaps[key] = w
	}
	setTestWorldManager(t, wm)
	return g, wm, ts
}

func readTravelAutosave(t *testing.T) GameSave {
	t.Helper()
	data, err := os.ReadFile(saveRowPath(0))
	if err != nil {
		t.Fatal(err)
	}
	var save GameSave
	if err := json.Unmarshal(data, &save); err != nil {
		t.Fatal(err)
	}
	return save
}

func TestTravelEntryPointsCommitFinalArrival(t *testing.T) {
	for _, entry := range []string{"entrance_first", "entrance_return", "town_portal_spawn", "town_portal_anchor", "dive", "surface", "pose"} {
		for _, turnBased := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/tb=%v", entry, turnBased), func(t *testing.T) {
				g, wm, ts := travelFixture(t)
				g.turnBasedMode = turnBased
				g.camera.Angle = 0.37
				origin := MapPose{X: g.camera.X, Y: g.camera.Y, Angle: g.camera.Angle}
				want := MapPose{X: 6.5 * ts, Y: 7.5 * ts, Angle: 0.37}
				key := "other"
				ih := NewInputHandler(g)
				// Town Portal and expiration run without a GameLoop input handler.
				switch entry {
				case "entrance_first", "entrance_return":
					want.Angle = AngleNorth
					if entry == "entrance_return" {
						want = MapPose{X: 12.5 * ts, Y: 14.5 * ts, Angle: 0.37}
						g.mapReturnPoses = map[string]MapPose{key: want}
					}
					ih.enterEncounterMap(key)
					if g.mapReturnPoses["forest"] != origin {
						t.Fatal("entrance did not retain its departure pose")
					}
				case "town_portal_spawn", "town_portal_anchor":
					if entry == "town_portal_anchor" {
						wm.LoadedMaps[key].NPCs = []*character.NPC{{Name: "Anchor", TownPortal: true, X: 12.5 * ts, Y: 14.5 * ts}}
						want.X, want.Y = 12.5*ts, 15.5*ts
					}
					g.townPortalTeleport(key)
				case "dive":
					key = "water"
					g.world.Tiles[10][8] = world.TileDeepWater
					g.waterBreathingActive = true
					want.X, want.Y = 25.5*ts, 25.5*ts
					ih.checkDeepWater()
					if g.underwaterReturnMap != "forest" {
						t.Fatal("dive lost the surface map")
					}
				case "surface":
					g.underwaterReturnMap = key
					g.underwaterReturnX, g.underwaterReturnY = want.X, want.Y
					(&GameLoop{game: g}).returnFromUnderwater()
				case "pose":
					if err := g.transitionToMap(mapTransition{mapKey: key, pose: want}); err != nil {
						t.Fatal(err)
					}
				}
				if turnBased && want.Angle == 0.37 {
					want.Angle = 0
				}
				if wm.CurrentMapKey != key || g.world != wm.LoadedMaps[key] || g.camera.X != want.X || g.camera.Y != want.Y || math.Abs(g.camera.Angle-want.Angle) > 1e-9 {
					t.Fatalf("wrong arrival: map=%s camera=%+v want=%+v", wm.CurrentMapKey, g.camera, want)
				}
				save := readTravelAutosave(t)
				if save.MapKey != key || save.PlayerX != want.X || save.PlayerY != want.Y || math.Abs(save.PlayerAngle-want.Angle) > 1e-9 {
					t.Fatalf("autosave captured an intermediate arrival: %s (%v,%v,%v)", save.MapKey, save.PlayerX, save.PlayerY, save.PlayerAngle)
				}
				entity := g.collisionSystem.GetEntityByID("player")
				if entity == nil || entity.BoundingBox.X != want.X || entity.BoundingBox.Y != want.Y {
					t.Fatal("arrival and player collision position differ")
				}
				if entry == "dive" && (save.UnderwaterReturnMap != "forest" || save.UnderwaterReturnX != g.underwaterReturnX || save.UnderwaterReturnY != g.underwaterReturnY) {
					t.Fatal("dive autosaved before recording its surface return")
				}
			})
		}
	}
}

func TestFailedTravelPreservesTimeline(t *testing.T) {
	for _, failure := range []string{"manager", "missing_map", "nil_world", "busy"} {
		for _, arrival := range []mapArrivalKind{mapArrivalPose, mapArrivalEntrance, mapArrivalTownPortal, mapArrivalUnderwater} {
			t.Run(fmt.Sprintf("%s/arrival=%d", failure, arrival), func(t *testing.T) {
				g, wm, ts := travelFixture(t)
				origin := g.world
				pose := MapPose{X: g.camera.X, Y: g.camera.Y, Angle: g.camera.Angle}
				g.mapReturnPoses = map[string]MapPose{"forest": pose}
				g.underwaterReturnMap, g.underwaterReturnX, g.underwaterReturnY = "old", 1, 2
				g.arrows = []Arrow{{ID: "ongoing", Active: true}}
				path := saveRowPath(0)
				if err := os.WriteFile(path, []byte("previous autosave"), 0600); err != nil {
					t.Fatal(err)
				}
				switch failure {
				case "manager":
					world.GlobalWorldManager = nil
				case "missing_map":
					delete(wm.LoadedMaps, "other")
				case "nil_world":
					wm.LoadedMaps["other"] = nil
				case "busy":
					wm.TransitionInProgress = true
				}
				err := g.transitionToMap(mapTransition{mapKey: "other", arrival: arrival, pose: MapPose{X: 3.5 * ts, Y: 4.5 * ts}})
				if err == nil {
					t.Fatal("unavailable transition succeeded")
				}
				if g.world != origin || wm.CurrentMapKey != "forest" || g.camera.X != pose.X || g.camera.Y != pose.Y || g.camera.Angle != pose.Angle || len(g.arrows) != 1 {
					t.Fatal("failed transition changed the current world, pose or combat")
				}
				if !reflect.DeepEqual(g.mapReturnPoses, map[string]MapPose{"forest": pose}) || g.underwaterReturnMap != "old" || g.underwaterReturnX != 1 || g.underwaterReturnY != 2 {
					t.Fatal("failed transition changed return locations")
				}
				data, err := os.ReadFile(path)
				if err != nil || string(data) != "previous autosave" {
					t.Fatal("failed transition replaced the autosave")
				}
			})
		}
	}
}

func TestTeleporterTravelLocality(t *testing.T) {
	for _, locality := range []string{"same_map", "separate_map", "stitched_region", "refused_map"} {
		for _, turnBased := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/tb=%v", locality, turnBased), func(t *testing.T) {
				g, wm, ts := travelFixture(t)
				targetKey := "other"
				sx, sy, dx, dy := 8, 10, 14, 15
				if locality == "stitched_region" {
					t.Chdir("../..")
					g, wm, _ = bootOpenWorldGame(t, true)
					fx, fy, _ := wm.OpenWorldRegionStart("forest")
					hx, hy, _ := wm.OpenWorldRegionStart("highlands")
					sx, sy, dx, dy = int(fx/ts), int(fy/ts), int(hx/ts), int(hy/ts)
					targetKey = "highlands"
				} else if err := world.GlobalTileManager.LoadSpecialTileConfig("../../assets/special_tiles.yaml"); err != nil {
					t.Fatal(err)
				}
				if locality == "same_map" {
					targetKey = "forest"
				}
				g.turnBasedMode = turnBased
				placePlayerAtTile(g, sx, sy, ts)
				g.camera.Angle = 0.37
				g.world.Tiles[sy][sx] = world.TileVioletTeleporter
				wm.GlobalTeleporterRegistry.Teleporters = []world.TeleporterLocation{
					{X: sx, Y: sy, MapKey: "forest", Group: "test", AutoActivate: true, ExcludeSelf: true, CrossMap: true},
					{X: dx, Y: dy, MapKey: targetKey, Group: "test"},
				}
				wm.TransitionInProgress = locality == "refused_map"
				path := saveRowPath(0)
				if err := os.WriteFile(path, []byte("previous autosave"), 0600); err != nil {
					t.Fatal(err)
				}
				g.arrows = []Arrow{{ID: "ongoing", Active: true}}
				NewInputHandler(g).checkTeleporter()
				wantX, wantY := (float64(dx)+0.5)*ts, (float64(dy)+0.5)*ts
				wantAngle := 0.37
				if turnBased {
					wantAngle = 0
				}
				if locality == "refused_map" {
					wantX, wantY, wantAngle = (float64(sx)+0.5)*ts, (float64(sy)+0.5)*ts, 0.37
				} else if locality == "separate_map" {
					wantAngle = AngleNorth
				}
				if g.camera.X != wantX || g.camera.Y != wantY || math.Abs(g.camera.Angle-wantAngle) > 1e-9 {
					t.Fatalf("teleporter locality changed position/heading: %+v, want (%v,%v,%v)", g.camera, wantX, wantY, wantAngle)
				}
				if locality == "separate_map" {
					save := readTravelAutosave(t)
					if save.MapKey != targetKey || save.PlayerX != wantX || save.PlayerY != wantY || len(g.arrows) != 0 {
						t.Fatal("cross-map teleporter did not commit the arrival")
					}
				} else {
					data, err := os.ReadFile(path)
					if err != nil || string(data) != "previous autosave" || len(g.arrows) != 1 {
						t.Fatal("same-world/refused teleport ran map departure or autosave")
					}
				}
			})
		}
	}
}

func TestStitchedRegionArrivalAutosavesLocalCoordinates(t *testing.T) {
	for _, entry := range []string{"entrance", "town_portal"} {
		t.Run(entry, func(t *testing.T) {
			t.Chdir("../..")
			storage.SetDataRootForTesting(t.TempDir())
			t.Cleanup(func() { storage.SetDataRootForTesting("") })
			g, wm, _ := bootOpenWorldGame(t, true)
			g.turnBasedMode = false
			g.camera.Angle = 0.37
			target := "highlands"
			x, y, ok := wm.OpenWorldRegionStart(target)
			if !ok {
				t.Fatal("fixture region has no start")
			}
			if entry == "town_portal" {
				x, y, _ = g.townPortalArrivalPoint(target)
				g.townPortalTeleport(target)
			} else {
				NewInputHandler(g).enterEncounterMap(target)
			}
			if g.world != wm.OpenWorld || wm.CurrentMapKey != target || g.camera.X != x || g.camera.Y != y {
				t.Fatal("arrival selected the unified world's start instead of the requested region")
			}
			save := readTravelAutosave(t)
			px, py := wm.ProjectWorldPos(save.MapKey, save.PlayerX, save.PlayerY)
			if save.MapKey != target || px != x || py != y {
				t.Fatal("region arrival did not autosave in map-local coordinates")
			}
		})
	}
}
