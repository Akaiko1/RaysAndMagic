package game

import (
	"fmt"
	"ugataima/internal/world"
)

// Poses are live world coordinates. Save restoration projects map-local values
// separately and never runs travel-only departure effects or autosaves.
type mapArrivalKind uint8

const (
	mapArrivalPose mapArrivalKind = iota
	mapArrivalEntrance
	mapArrivalTownPortal
	mapArrivalUnderwater
)

type mapTransition struct {
	mapKey  string
	arrival mapArrivalKind
	pose    MapPose
}

// transitionToMap owns one complete travel operation. No arrival, quest entry
// spawn or autosave runs unless the destination world was selected successfully.
func (g *MMGame) transitionToMap(request mapTransition) error {
	if request.arrival > mapArrivalUnderwater {
		return fmt.Errorf("unknown map arrival kind: %d", request.arrival)
	}
	wm := world.GlobalWorldManager
	if wm == nil {
		return fmt.Errorf("world manager not available")
	}
	if g.worldByKey(request.mapKey) == nil {
		return fmt.Errorf("destination map is not loaded: %s", request.mapKey)
	}
	originKey := wm.CurrentMapKey
	origin := MapPose{X: g.camera.X, Y: g.camera.Y, Angle: g.camera.Angle}
	if request.arrival == mapArrivalUnderwater {
		origin.X, origin.Y = g.FindNearestWalkableTileMustSucceed(origin.X, origin.Y)
	}
	if err := g.switchToMap(request.mapKey); err != nil {
		return err
	}
	pose := request.pose
	switch request.arrival {
	case mapArrivalUnderwater:
		g.underwaterReturnX, g.underwaterReturnY = origin.X, origin.Y
		g.underwaterReturnMap = originKey
	case mapArrivalEntrance:
		if g.mapReturnPoses == nil {
			g.mapReturnPoses = make(map[string]MapPose)
		}
		if originKey != "" {
			g.mapReturnPoses[originKey] = origin
		}
		if saved, ok := g.mapReturnPoses[request.mapKey]; ok {
			pose = saved
		} else if x, y, ok := wm.OpenWorldRegionStart(request.mapKey); ok {
			pose = MapPose{X: x, Y: y, Angle: AngleNorth}
		} else {
			pose.X, pose.Y = g.world.GetStartingPosition()
			pose.Angle = AngleNorth
		}
	case mapArrivalTownPortal:
		pose = MapPose{X: g.camera.X, Y: g.camera.Y, Angle: g.camera.Angle}
		if x, y, ok := g.townPortalArrivalPoint(request.mapKey); ok {
			pose.X, pose.Y = x, y
		}
	}
	g.finishMapArrival(pose.X, pose.Y, pose.Angle)
	return nil
}

// switchToMap handles common map switching logic for teleporters and spell effects
func (g *MMGame) switchToMap(targetMapKey string) error {
	if world.GlobalWorldManager == nil {
		return fmt.Errorf("world manager not available")
	}
	oldWorld := g.world
	err := world.GlobalWorldManager.SwitchToMap(targetMapKey)
	if err != nil {
		return err
	}

	// Update world reference and collision system
	g.world = g.GetCurrentWorld()
	if oldWorld != g.world {
		g.crumbleBoundAlliesOnDeparture(oldWorld)
	}
	g.registerVisitedTownPortalDestination() // Town Portal learns this map's destination
	g.dropFlyWithoutOpenSky()                // wings fade indoors (dungeons have no sky)
	// Sync the new world's Fly flag to the party NOW (not next frame): it may
	// carry a stale flyActive from a previous visit, which would make walls read
	// as passable to anything querying it before the frame's buff sync runs.
	if g.world != nil {
		g.world.SetFlyActive(g.flyActive)
	}
	g.clearTransientCombatState()
	// A map change ends every approach: drop the focus identity and any nudge
	// queued for it. (The nudge producer additionally refuses to announce an NPC
	// that is not in the CURRENT world, which is what covers the other arrival
	// paths - focus is only recomputed on the next frame.)
	g.forgetInteractPromptTarget()
	if g.collisionSystem != nil {
		g.collisionSystem.UpdateTileChecker(g.world)
		// Unregister old world monsters
		if oldWorld != nil {
			for _, monster := range oldWorld.Monsters {
				g.collisionSystem.UnregisterEntity(monster.ID)
			}
		}
		// Farming maps (respawn_days) rewind their roster BEFORE registration.
		g.maybeRespawnMapMonsters()
		// Register new world monsters
		g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
		g.registerMapStaticCollision()
	}

	// Update visual systems
	g.UpdateSkyAndGroundColors()
	if g.gameLoop != nil && g.gameLoop.renderer != nil {
		// Refresh renderer caches that depend on world tiles
		g.gameLoop.renderer.precomputeFloorColorCache()
		g.gameLoop.renderer.buildTransparentSpriteCache()
	}

	return nil
}

// finishMapArrival is the single "arrived on a new map" path: it places the
// party at (x,y,angle), re-registers collision, snaps to a cardinal heading in
// turn-based mode, and autosaves. Keeping position + autosave together here is
// what guarantees the autosave can't capture stale pre-switch coordinates - the
// ordering invariant lives in one place instead of being copy-pasted per caller.
func (g *MMGame) finishMapArrival(x, y, angle float64) {
	// Arrival targets can be stale (saved return poses, positions recorded on an
	// older map layout); never place the party inside terrain.
	x, y = g.safePartyDestination(x, y)
	// A fresh arrival is the moment deferred (on_entry) quest spawns may fire
	// on this map - the Enforcer surfaces on the return trip. Flush the queue
	// immediately: the arrival is a frame boundary (no attack in flight), and
	// the Autosave below must snapshot the boss ALREADY in the roster.
	g.spawnQuestCompletionMonsters(true)
	g.flushPendingQuestSpawns()
	g.setPartyPosition(x, y)
	// Landmark solidity was registered against the OLD map's coordinates during
	// the switch; re-derive it now that the arrival position is final.
	g.refreshLandmarkCollision()
	g.snapFacing(angle)
	// Turn-based facing must be cardinal; a restored return-pose / free RT heading
	// would otherwise leave the party at 45deg on the new map.
	if g.turnBasedMode {
		g.snapToCardinalDirection()
	}
	g.Autosave()
}
