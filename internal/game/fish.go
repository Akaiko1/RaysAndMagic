package game

import (
	"math"
	"math/rand"

	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

var fishNeighbors = [4][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}}

// Fish killed over deep water still leave a reachable scale. Prefer the nearest
// unblocked standing tile with a clear path from the hit, including shallows.
func (g *MMGame) fishLootLanding(m *monster.Monster3D) (float64, float64) {
	tile := g.config.GetTileSize()
	tx, ty := int(m.X/tile), int(m.Y/tile)
	best, bx, by := math.Inf(1), m.X, m.Y
	for y := max(0, ty-8); y < min(g.world.Height, ty+9); y++ {
		for x := max(0, tx-8); x < min(g.world.Width, tx+9); x++ {
			wx, wy := (float64(x)+.5)*tile, (float64(y)+.5)*tile
			if !g.world.CanMoveTo(wx, wy) {
				continue
			}
			if g.collisionSystem != nil && (!g.collisionSystem.CanMoveTo("player", wx, wy) || !g.collisionSystem.CheckLineOfSight(m.X, m.Y, wx, wy)) {
				continue
			}
			d := Distance(m.X, m.Y, wx, wy)
			if d < best {
				best, bx, by = d, wx, wy
			}
		}
	}
	return bx, by
}

func fishWater(w *world.World3D, x, y int) bool {
	if w == nil || world.GlobalTileManager == nil || x < 0 || y < 0 || x >= w.Width || y >= w.Height {
		return false
	}
	d := world.GlobalTileManager.GetTileData(w.Tiles[y][x])
	return d != nil && d.Type == "water"
}

func (g *MMGame) fishRegionContains(key string, x, y int) bool {
	w := g.world
	if x < 0 || y < 0 || x >= w.Width || y >= w.Height {
		return false
	}
	wm := world.GlobalWorldManager
	if wm != nil && w == wm.OpenWorld {
		r := wm.OpenWorldRegionAtTile(x, y)
		return r != nil && r.MapKey == key
	}
	return wm != nil && wm.CurrentMapKey == key
}

// Only cardinal neighbors count: two diagonal puddles are still isolated.
func (g *MMGame) fishDestinations(key string, x, y int, water bool) [][2]int {
	var out [][2]int
	tile := g.config.GetTileSize()
	for _, d := range fishNeighbors {
		nx, ny := x+d[0], y+d[1]
		if !g.fishRegionContains(key, nx, ny) || fishWater(g.world, nx, ny) != water {
			continue
		}
		wx, wy := (float64(nx)+.5)*tile, (float64(ny)+.5)*tile
		if !water && (!g.world.CanMoveTo(wx, wy) || (g.collisionSystem != nil && !g.collisionSystem.CanMoveTo("player", wx, wy))) {
			continue
		}
		out = append(out, [2]int{nx, ny})
	}
	return out
}

func (g *MMGame) fishSources(key string, radius float64) [][2]int {
	var out [][2]int
	tile := g.config.GetTileSize()
	cx, cy := int(g.camera.X/tile), int(g.camera.Y/tile)
	r := int(math.Ceil(radius))
	for y := max(0, cy-r); y < min(g.world.Height, cy+r+1); y++ {
		for x := max(0, cx-r); x < min(g.world.Width, cx+r+1); x++ {
			if !g.fishRegionContains(key, x, y) || !fishWater(g.world, x, y) || len(g.fishDestinations(key, x, y, true)) == 0 {
				continue
			}
			wx, wy := (float64(x)+.5)*tile, (float64(y)+.5)*tile
			d := Distance(wx, wy, g.camera.X, g.camera.Y) / tile
			if d < 1.5 || d > radius || (g.collisionSystem != nil && !g.collisionSystem.CheckLineOfSight(g.camera.X, g.camera.Y, wx, wy)) {
				continue
			}
			out = append(out, [2]int{x, y})
		}
	}
	return out
}

func (g *MMGame) spawnLeapingFish(key, species string, source [2]int, settings *config.FishSpawnConfig, beachRoll float64) *monster.Monster3D {
	water := g.fishDestinations(key, source[0], source[1], true)
	if !g.fishRegionContains(key, source[0], source[1]) || !fishWater(g.world, source[0], source[1]) || len(water) == 0 {
		return nil
	}
	destinations, beached := water, false
	if beachRoll < settings.BeachChance {
		if land := g.fishDestinations(key, source[0], source[1], false); len(land) > 0 {
			destinations, beached = land, true
		}
	}
	dest := destinations[rand.Intn(len(destinations))]
	tile := g.config.GetTileSize()
	x, y := (float64(source[0])+.5)*tile, (float64(source[1])+.5)*tile
	m := monster.NewMonster3DFromConfig(x, y, species, g.config)
	m.QuestProgressIgnored = true
	m.FishLeap = &monster.FishLeapState{FromX: x, FromY: y, ToX: (float64(dest[0]) + .5) * tile, ToY: (float64(dest[1]) + .5) * tile, Duration: settings.FlightSeconds, PeakHeight: settings.HeightTiles, Beached: beached}
	m.AdvanceFishLeap(0)
	g.registerSpawnedMonster(m)
	return m
}

// Called behind the normal menu/loading pause barriers, in RT and TB. Fish
// move once per elapsed frame and never spend a monster AI turn or patrol.
func (g *MMGame) updateFish() {
	c := config.GlobalEcology
	if c == nil || c.Fish == nil || g.world == nil || g.camera == nil || world.GlobalWorldManager == nil {
		return
	}
	f := c.Fish
	dt := 1 / float64(g.config.GetTPS())
	for _, w := range ecologyWorlds() {
		kept := w.Monsters[:0]
		for _, m := range w.Monsters {
			remove := false
			if m.Disposition == "fish" {
				if w != g.world {
					remove = true
				} else if m.IsAlive() {
					remove = m.AdvanceFishLeap(dt)
					if remove && m.FishLeap != nil && m.FishLeap.Beached {
						g.addMonsterLootDrop(m, g.combat.rollMonsterLoot(m), 0)
					}
					if !remove {
						g.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
					}
				}
			}
			if remove {
				if w == g.world {
					g.collisionSystem.UnregisterEntity(m.ID)
				}
				continue
			}
			kept = append(kept, m)
		}
		clear(w.Monsters[len(kept):])
		w.Monsters = kept
	}
	// A shared update cadence, not a spawn cooldown. Flights do not suppress
	// later rolls, and each eligible tile rolls independently.
	if g.ecology.FishRollFrames < 0 || g.ecology.FishRollFrames >= f.RollEveryFrames {
		g.ecology.FishRollFrames = 0
	}
	g.ecology.FishRollFrames++
	if g.ecology.FishRollFrames < f.RollEveryFrames {
		return
	}
	g.ecology.FishRollFrames = 0
	key := world.GlobalWorldManager.CurrentMapKey
	species := f.Species[key]
	if species == "" || f.SpawnChancePerTile <= 0 {
		return
	}
	for _, source := range g.fishSources(key, f.RadiusTiles) {
		if rand.Float64() < f.SpawnChancePerTile {
			g.spawnLeapingFish(key, species, source, f, rand.Float64())
		}
	}
}
