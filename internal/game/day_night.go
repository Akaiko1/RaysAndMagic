package game

// Day/night cycle. A game-time clock (advances only during live gameplay
// frames) drives three things:
//   - a global OUTDOOR light scale, a smooth cosine between day_light and
//     night_light over the full cycle (multiplied into the frame ambient in
//     updateActiveLights; canopy shade composes multiplicatively so tree
//     shade deepens and fades with the daylight);
//   - the sky panorama variant: "<sky_texture>_day"/"<sky_texture>_night"
//     when the file exists, crossfaded at each phase flip. A map counts as
//     outdoor exactly when it ships such a variant - interiors keep their
//     authored panorama and static lighting;
//   - config-driven ambient packs (day_night.packs): wolves roam the forest
//     by day, spiders by night. Packs swap only ON the flip, so a fresh game
//     (clock starts at noon) begins with no pack until the first dawn.

import (
	"fmt"
	"math"
	"math/rand"
	"os"

	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// dayNightCycleFrames is the full day+night cycle length in ticks.
func (g *MMGame) dayNightCycleFrames() int {
	return 2 * g.config.DayNight.HalfCycleSecondsOrDefault() * g.config.GetTPS()
}

// dayNightFrac is the cycle position in [0,1): 0 = noon, 0.5 = midnight.
func (g *MMGame) dayNightFrac() float64 {
	cycle := g.dayNightCycleFrames()
	if cycle <= 0 {
		return 0
	}
	return float64(g.dayNightFrames%cycle) / float64(cycle)
}

// dayNightLightScale is the smooth light curve: dayLight at noon (frac 0),
// nightLight at midnight (frac 0.5), cosine in between - the whole half-cycle
// IS the transition.
func dayNightLightScale(frac, dayLight, nightLight float64) float64 {
	mid := (dayLight + nightLight) / 2
	amp := (dayLight - nightLight) / 2
	return mid + amp*math.Cos(2*math.Pi*frac)
}

// dayNightIsNightAt: night is the half-cycle centered on midnight, i.e. the
// panorama/pack flip happens where the light curve crosses its midpoint.
func dayNightIsNightAt(frac float64) bool {
	return frac > 0.25 && frac <= 0.75
}

// dayNightLightScaleNow is the current frame's outdoor ambient multiplier.
func (g *MMGame) dayNightLightScaleNow() float64 {
	return dayNightLightScale(g.dayNightFrac(), g.config.DayNight.DayLightOrDefault(), g.config.DayNight.NightLightOrDefault())
}

// updateDayNight advances the clock one tick and fires the phase flip
// (panorama crossfade + pack swap) when day turns to night or back.
func (g *MMGame) updateDayNight() {
	if g.dayNightSkipActive {
		if g.advanceSkyFadeFrame() {
			return
		}
		if len(g.dayNightSkipPhases) > 0 {
			g.beginNextDayNightSkipPhase()
			return
		}
		g.dayNightFrames = g.dayNightSkipTargetFrame
		g.dayNightSkipActive = false
		return
	}
	// A normal phase fade is visual only; it must not pause the running clock.
	g.advanceSkyFadeFrame()
	cycle := g.dayNightCycleFrames()
	if cycle <= 0 {
		return
	}
	g.dayNightFrames = (g.dayNightFrames + 1) % cycle

	night := dayNightIsNightAt(g.dayNightFrac())
	if night == g.dayNightIsNight {
		return
	}
	g.applyDayNightPhase(night)
}

func (g *MMGame) advanceSkyFadeFrame() bool {
	if g.skyFadeFrames <= 0 {
		return false
	}
	g.skyFadeFrames--
	if g.skyFadeFrames <= 0 {
		g.skyPanoramaPrev = nil
	}
	return true
}

// applyDayNightPhase performs one real dusk/dawn boundary. The arena refreshes
// at either boundary; weekly merchant stock converts those phase ticks to full
// calendar days in refreshScheduledMerchantStocks.
func (g *MMGame) applyDayNightPhase(night bool) {
	g.dayNightIsNight = night
	g.applySkyForPhase(true)
	g.syncDayNightPacks(night)
	g.dayNightDay++
	if night {
		g.AddCombatMessage("Night falls.")
		return
	}
	if weekChanged, _ := g.advanceCalendarAtDawn(); weekChanged {
		g.refreshScheduledMerchantStocks()
	}
	g.AddCombatMessage("The sun rises.")
}

// advanceDayNightToPhase queues every crossed phase start and lets the normal
// panorama fade play each one. A daytime wait to the next dawn therefore shows
// day -> night -> day while the game clock moves only forward.
func (g *MMGame) advanceDayNightToPhase(night bool) {
	if g.dayNightSkipActive {
		return
	}
	cycle := g.dayNightCycleFrames()
	if cycle <= 0 {
		return
	}
	// Cycle: frac 0 = noon, night = (0.25, 0.75] (see dayNightIsNightAt); the
	// phase flips entering the frame just past 1/4 (dusk) and 3/4 (dawn).
	duskFrame, dawnFrame := cycle/4+1, 3*cycle/4+1
	target := dawnFrame
	if night {
		target = duskFrame
	}
	delta := target - g.dayNightFrames
	if delta <= 0 {
		delta += cycle // already at/past it: advance a full cycle, never backward
	}
	phaseAt, remaining := g.dayNightFrames, delta
	for remaining > 0 {
		duskDist := dayNightForwardDistance(phaseAt, duskFrame, cycle)
		dawnDist := dayNightForwardDistance(phaseAt, dawnFrame, cycle)
		nextDist, nextNight := duskDist, true
		if dawnDist < duskDist {
			nextDist, nextNight = dawnDist, false
		}
		if nextDist > remaining {
			break
		}
		g.dayNightSkipPhases = append(g.dayNightSkipPhases, nextNight)
		phaseAt = (phaseAt + nextDist) % cycle
		remaining -= nextDist
	}
	g.dayNightSkipTargetFrame = (g.dayNightFrames + delta) % cycle
	g.dayNightSkipActive = true
	g.beginNextDayNightSkipPhase()
}

func dayNightForwardDistance(from, to, cycle int) int {
	d := to - from
	if d <= 0 {
		d += cycle
	}
	return d
}

func (g *MMGame) beginNextDayNightSkipPhase() {
	if len(g.dayNightSkipPhases) == 0 {
		return
	}
	night := g.dayNightSkipPhases[0]
	g.dayNightSkipPhases = g.dayNightSkipPhases[1:]
	cycle := g.dayNightCycleFrames()
	if night {
		g.dayNightFrames = cycle/4 + 1
	} else {
		g.dayNightFrames = 3*cycle/4 + 1
	}
	g.applyDayNightPhase(night)
}

// finishDayNightSkipImmediately commits a paid time skip before a save. The
// visual crossfades are transient, but every skipped phase has gameplay work
// (ambient packs, arena refreshes, calendar boundaries) that must be included
// in the saved world.
func (g *MMGame) finishDayNightSkipImmediately() {
	if !g.dayNightSkipActive {
		return
	}
	g.cancelSkyFade()
	for len(g.dayNightSkipPhases) > 0 {
		g.beginNextDayNightSkipPhase()
		g.cancelSkyFade()
	}
	g.dayNightFrames = g.dayNightSkipTargetFrame
	g.dayNightSkipActive = false
	g.dayNightSkipTargetFrame = 0
}

// --- Sky panorama phase variants -------------------------------------------

func skyVariantName(base string, night bool) string {
	if night {
		return base + "_night"
	}
	return base + "_day"
}

func skyTextureExists(name string) bool {
	_, err := os.Stat(resolveNamedPNG("assets/sprites/sky", name))
	return err == nil
}

// skyHasDayNightVariants reports whether a map's sky ships a phase variant -
// the definition of "outdoor" for the light cycle.
func skyHasDayNightVariants(base string) bool {
	if base == "" {
		return false
	}
	return skyTextureExists(skyVariantName(base, false)) || skyTextureExists(skyVariantName(base, true))
}

// currentMapHasOpenSky reports whether the current map is outdoor by the
// day/night convention (its sky has a phase variant). Gates Fly.
func (g *MMGame) currentMapHasOpenSky() bool {
	if world.GlobalWorldManager == nil {
		return false
	}
	mc := world.GlobalWorldManager.GetCurrentMapConfig()
	return mc != nil && skyHasDayNightVariants(mc.SkyTexture)
}

// dropFlyWithoutOpenSky ends an active Fly the moment the party stands on a
// map without an open sky (dungeon entry, loading an indoor save): wings need
// the sky the cast gate demands, and a lingering Fly indoors lets the party
// phase into walls it can then be trapped in when the buff expires.
func (g *MMGame) dropFlyWithoutOpenSky() {
	if !g.flyActive || g.currentMapHasOpenSky() {
		return
	}
	g.flyActive = false
	g.flyDuration = 0
	if g.world != nil {
		g.world.SetFlyActive(false)
	}
	g.AddCombatMessage("The close air presses down - Fly fades.")
}

// ejectFromWallAfterFly surfaces the party to the nearest walkable tile when
// Fly lapses while they hover inside solid terrain (Fly lets movement pass
// through walls). Without it the party is stuck against a wall bbox with no
// legal move out. Walkability here is terrain-only, so it works regardless of
// the world's Fly flag sync order.
func (g *MMGame) ejectFromWallAfterFly() {
	w := g.GetCurrentWorld()
	if w == nil {
		return
	}
	ts := float64(g.config.GetTileSize())
	if !w.IsTileBlockingTerrainAt(int(g.camera.X/ts), int(g.camera.Y/ts)) {
		return // already on open ground
	}
	sx, sy := g.findNearestWalkableTileWithMaxRadius(g.camera.X, g.camera.Y, 12)
	if sx < 0 || sy < 0 {
		return // no walkable tile nearby (shouldn't happen on a real map)
	}
	g.camera.X, g.camera.Y = sx, sy
	if g.collisionSystem != nil {
		g.collisionSystem.UpdateEntity("player", sx, sy)
	}
	g.AddCombatMessage("The wings fade - the party settles onto solid ground.")
}

// skyTextureForPhase resolves the phase variant when it exists on disk, else
// the base name (interiors, zones without night art).
func (g *MMGame) skyTextureForPhase(base string) string {
	if base == "" {
		return ""
	}
	if v := skyVariantName(base, g.dayNightIsNight); skyTextureExists(v) {
		return v
	}
	return base
}

func (g *MMGame) cancelSkyFade() {
	g.skyPanoramaPrev = nil
	g.skyFadeFrames = 0
	g.skyFadeTotal = 0
}

// applySkyForPhase swaps the panorama to the current phase's variant. With
// fade, the previous panorama is kept and crossfaded out over
// panorama_fade_seconds (map switches swap instantly instead).
func (g *MMGame) applySkyForPhase(fade bool) {
	if world.GlobalWorldManager == nil {
		return
	}
	mc := world.GlobalWorldManager.GetCurrentMapConfig()
	if mc == nil || mc.SkyTexture == "" {
		return
	}
	name := g.skyTextureForPhase(mc.SkyTexture)
	if name == g.currentSkyTexture {
		return
	}
	prev := g.skyPanorama
	g.updateSkyPanorama(name)
	if fade && prev != nil && g.skyPanorama != nil {
		g.skyPanoramaPrev = prev
		total := int(g.config.DayNight.PanoramaFadeSecondsOrDefault() * float64(g.config.GetTPS()))
		g.skyFadeFrames, g.skyFadeTotal = total, total
	} else {
		g.cancelSkyFade()
	}
}

// skyFadeAlpha is the crossfade progress of the incoming panorama (1 = done).
func (g *MMGame) skyFadeAlpha() float32 {
	if g.skyFadeTotal <= 0 || g.skyFadeFrames <= 0 || g.skyPanoramaPrev == nil {
		return 1
	}
	return 1 - float32(g.skyFadeFrames)/float32(g.skyFadeTotal)
}

// --- Ambient day/night packs ------------------------------------------------

// dayNightPackTag identifies a pack's members across save/load (persisted in
// MonsterSave.PackKey).
func dayNightPackTag(mapKey string, night bool) string {
	if night {
		return "daynight:" + mapKey + ":night"
	}
	return "daynight:" + mapKey + ":day"
}

// syncDayNightPacks despawns the outgoing phase's packs and spawns the
// incoming ones on every configured map (loaded maps only).
func (g *MMGame) syncDayNightPacks(night bool) {
	wm := world.GlobalWorldManager
	if wm == nil {
		return
	}
	for _, pack := range g.config.DayNight.Packs {
		w := wm.LoadedMaps[pack.Map]
		if w == nil {
			continue
		}
		// Evaluate the clear gate before removing the previous phase's pack: a
		// surviving pack member is still a living monster and must prevent a
		// fresh pack from replacing it at the next phase refresh.
		spawnAllowed := !pack.RequireMapClear || !worldHasLivingMonsters(w)
		g.despawnPackMonsters(w, dayNightPackTag(pack.Map, !night))
		g.despawnPackMonsters(w, dayNightPackTag(pack.Map, night)) // no double-stacking on odd flows (load edge cases)
		if !spawnAllowed {
			continue
		}
		tag := dayNightPackTag(pack.Map, night)
		// One tag covers every member of the phase, so a mixed pack (e.g. grunts
		// + an elite) despawns together and never self-clears mid-spawn.
		for _, mem := range pack.PhaseMembers(night) {
			if mem.Monster == "" || mem.Count <= 0 {
				continue
			}
			g.spawnPackMonsters(w, tag, mem.Monster, mem.Count, pack.MinPlayerDistTilesOrDefault())
		}
	}
}

func worldHasLivingMonsters(w *world.World3D) bool {
	if w == nil {
		return false
	}
	for _, m := range w.Monsters {
		if m != nil && m.IsAlive() {
			return true
		}
	}
	return false
}

// despawnPackMonsters removes a pack without the death path (no XP/loot). On
// the current map the removal goes through the frame-end dead-ID sweep so the
// collision entity is unregistered; queueing while ALIVE keeps the indirect-
// kill finalizer from awarding rewards. Other loaded maps run no AI and hold
// no collision entities, so a plain filter is safe there.
func (g *MMGame) despawnPackMonsters(w *world.World3D, tag string) {
	if w == g.world {
		for _, m := range w.Monsters {
			if m != nil && m.PackKey == tag {
				g.deadMonsterIDs = append(g.deadMonsterIDs, m.ID)
			}
		}
		return
	}
	kept := w.Monsters[:0]
	for _, m := range w.Monsters {
		if m != nil && m.PackKey == tag {
			continue
		}
		kept = append(kept, m)
	}
	w.Monsters = kept
}

// spawnPackMonsters scatters count monsters over random walkable tiles. On the
// current map spawns keep min_player_dist_tiles away from the party and
// register collision immediately; on other loaded maps collision registers in
// bulk on map arrival (RegisterMonstersWithCollisionSystem).
func (g *MMGame) spawnPackMonsters(w *world.World3D, tag, monsterKey string, count int, minPlayerDistTiles float64) {
	if world.GlobalTileManager == nil || w.Width <= 0 || w.Height <= 0 {
		return
	}
	if monster.MonsterConfig == nil {
		return
	}
	if _, ok := monster.MonsterConfig.Monsters[monsterKey]; !ok {
		fmt.Printf("[DayNight] unknown pack monster %q - skipping spawn\n", monsterKey)
		return
	}
	tile := float64(g.config.GetTileSize())
	minDist := minPlayerDistTiles * tile
	current := w == g.world
	spawned := 0
	for attempts := 0; spawned < count && attempts < count*60; attempts++ {
		tx, ty := rand.Intn(w.Width), rand.Intn(w.Height)
		if ty >= len(w.Tiles) || tx >= len(w.Tiles[ty]) {
			continue
		}
		if !world.GlobalTileManager.IsWalkable(w.Tiles[ty][tx]) {
			continue
		}
		x, y := TileCenterFromTile(tx, ty, tile)
		if current {
			dx, dy := x-g.camera.X, y-g.camera.Y
			if dx*dx+dy*dy < minDist*minDist {
				continue
			}
		}
		m := monster.NewMonster3DFromConfig(x, y, monsterKey, g.config)
		if m == nil {
			continue
		}
		m.PackKey = tag
		m.QuestProgressIgnored = true // ambient packs never advance kill quests
		if current {
			g.registerSpawnedMonster(m)
			g.refreshMonsterCollisionSolidity(m)
		} else {
			w.Monsters = append(w.Monsters, m)
		}
		spawned++
	}
}
