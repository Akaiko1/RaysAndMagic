package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func TestBeaconBindingRecallAndRefusal(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(map[bool]string{false: "RT", true: "TB"}[tb], func(t *testing.T) {
			g, _ := summonTileWorld(t)
			g.turnBasedMode = tb
			old := world.GlobalWorldManager
			t.Cleanup(func() { world.GlobalWorldManager = old })
			world.GlobalWorldManager = &world.WorldManager{CurrentMapKey: "test", LoadedMaps: map[string]*world.World3D{"test": g.world}, MapConfigs: map[string]*config.MapConfig{"test": {Name: "Test"}}}
			ts := float64(g.config.GetTileSize())
			placePlayerAtTile(g, 4, 4, ts)
			c := g.party.Members[0]
			c.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{character.MagicSchoolWater: {}}
			c.SpellPoints = 500
			c.ActionsRemaining = 2
			g.currentTurn = 0
			c.LearnSpell("vvoid_beacon")
			d, _ := spells.GetSpellDefinitionByID("vvoid_beacon")
			open := func() { g.combat.castPlayerSpell("vvoid_beacon", d, c, true) }
			before := c.SpellPoints
			open()
			if !g.beaconPicker || c.SpellPoints != before {
				t.Fatal("opening beacon picker paid or failed")
			}
			g.cancelTownPortalPicker()
			if c.SpellPoints != before || g.beaconPicker {
				t.Fatal("cancel paid or retained beacon mode")
			}
			open()
			g.confirmBeacon(0, true)
			cost := c.MeditatedSPCost(d.SpellPointsCost)
			if c.SpellPoints != before-cost || len(c.SpellState.Beacons) != 1 {
				t.Fatalf("bind paid %d, marks=%d", before-c.SpellPoints, len(c.SpellState.Beacons))
			}
			mark := c.SpellState.Beacons[0]
			restored := restoreCharacterSave(buildCharacterSave(c))
			if len(restored.SpellState.Beacons) != 1 || restored.SpellState.Beacons[0] != mark {
				t.Fatal("beacon was lost on character save/load")
			}
			g.world.Monsters = append(g.world.Monsters, &monster.Monster3D{X: g.camera.X, Y: g.camera.Y, HitPoints: 10})
			c.ActionsRemaining, c.RTCooldown = 2, 0
			g.currentTurn = 0
			g.townPortalCaster, g.townPortalSpell, g.beaconPicker, g.townPortalPickerOpen = c, "vvoid_beacon", true, true
			before = c.SpellPoints
			g.confirmBeacon(0, false)
			if c.SpellPoints != before || !g.beaconPicker {
				t.Fatal("occupied recall consumed SP")
			}
			g.world.Monsters = nil
			if _, err := g.beaconRequest(mark); err != nil {
				t.Fatalf("legal saved destination rejected: %v", err)
			}
			c.ActionsRemaining, c.RTCooldown = 2, 0
			g.currentTurn = 0
			placePlayerAtTile(g, 6, 6, ts)
			open()
			g.confirmBeacon(0, false)
			if c.SpellPoints != before-cost || g.camera.X != mark.X || g.camera.Y != mark.Y {
				t.Fatalf("recall: SP %d, pose (%v,%v); wanted SP %d, pose (%v,%v)", c.SpellPoints, g.camera.X, g.camera.Y, before-cost, mark.X, mark.Y)
			}
			mark.X = -10
			if _, err := g.beaconRequest(mark); err == nil {
				t.Fatal("out of bounds beacon accepted")
			}
		})
	}
}

func TestBeaconRejectsPreviousDungeonGeneration(t *testing.T) {
	g, _ := summonTileWorld(t)
	old := world.GlobalWorldManager
	t.Cleanup(func() { world.GlobalWorldManager = old })
	world.GlobalWorldManager = &world.WorldManager{CurrentMapKey: "test", LoadedMaps: map[string]*world.World3D{"test": g.world}, MapConfigs: map[string]*config.MapConfig{"test": {Adventure: &config.AdventureConfig{OpeningOwned: true}}}}
	g.adventure.Visits = map[string]*AdventureVisit{"test": {Generation: 2, Opening: g.openingID()}}
	ts := float64(g.config.GetTileSize())
	b := character.SpellBeacon{MapKey: "test", X: 4.5 * ts, Y: 4.5 * ts, Generation: 1}
	if _, err := g.beaconRequest(b); err == nil {
		t.Fatal("previous dungeon generation accepted")
	}
	b.Generation = 2
	if _, err := g.beaconRequest(b); err != nil {
		t.Fatalf("current generation refused: %v", err)
	}
}

func TestBeaconDepthsRequiresBreathingAndReturnsToCastPoint(t *testing.T) {
	for _, tb := range []bool{false, true} {
		g, wm, ts := travelFixture(t)
		g.turnBasedMode = tb
		mark := character.SpellBeacon{MapKey: underwaterMapKey, X: 6.5 * ts, Y: 7.5 * ts}
		if _, err := g.beaconRequest(mark); err == nil {
			t.Fatal("depths accepted without Water Breathing")
		}
		g.waterBreathingActive = true
		req, err := g.beaconRequest(mark)
		if err != nil {
			t.Fatal(err)
		}
		x, y := g.camera.X, g.camera.Y
		g.underwaterReturnMap, g.underwaterReturnX, g.underwaterReturnY = "old", 1, 2
		if err = g.transitionToMap(req); err != nil {
			t.Fatal(err)
		}
		if wm.CurrentMapKey != underwaterMapKey || g.underwaterReturnMap != "forest" || g.underwaterReturnX != x || g.underwaterReturnY != y {
			t.Fatal("beacon retained an old surface return")
		}
		// Another recall within the depths must not replace the surface with water.
		if err = g.transitionToMap(req); err != nil {
			t.Fatal(err)
		}
		if g.underwaterReturnMap != "forest" {
			t.Fatal("underwater recall lost surface anchor")
		}
		(&GameLoop{game: g}).returnFromUnderwater()
		if wm.CurrentMapKey != "forest" || g.camera.X != x || g.camera.Y != y {
			t.Fatal("surfacing missed the beacon cast point")
		}
	}
}
