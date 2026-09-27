package game

import (
	"fmt"
	"math/rand"
	"slices"
	"sort"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/world"
)

func (g *MMGame) hasHarvestAlchemist() bool {
	if g.party == nil {
		return false
	}
	for _, c := range g.party.Members {
		if c != nil && c.Class == character.ClassAlchemist && c.HitPoints > 0 && !c.HasCondition(character.ConditionDead) && !c.HasCondition(character.ConditionEradicated) {
			return true
		}
	}
	return false
}
func (g *MMGame) harvestGround(x, y int) bool {
	w := g.world
	if w == nil || x <= 0 || y <= 0 || x >= w.Width-1 || y >= w.Height-1 || w.IsTileBlockingTerrainAt(x, y) {
		return false
	}
	t := w.Tiles[y][x]
	if t == world.TileWater || t == world.TileDeepWater || t == world.TileForestStream {
		return false
	}
	return true
}

// Search is bounded per simulation update; no dawn frame walks a whole region.
const harvestSearchTilesPerUpdate = 64

type harvestSearch struct {
	region    string
	day       int
	start     [2]int
	queue     [][2]int
	seen      map[[2]int]bool
	occupied  map[[2]int]bool
	buildings map[[2]int]bool
	cursor    int
	free      [][2]int
}
type harvestRuntime struct {
	world   *world.World3D
	catalog *config.AlchemySpawnConfig
	region  string
	dirty   bool
	search  *harvestSearch
}

func (g *MMGame) harvestOccupied() map[[2]int]bool {
	ts := float64(g.config.GetTileSize())
	occupied := g.buildingOccupiedTiles()
	occupied[[2]int{TileIndex(g.camera.X, ts), TileIndex(g.camera.Y, ts)}] = true
	for _, n := range g.world.NPCs {
		if n != nil {
			occupied[[2]int{TileIndex(n.X, ts), TileIndex(n.Y, ts)}] = true
		}
	}
	for _, m := range g.world.Monsters {
		if m != nil && m.IsAlive() {
			occupied[[2]int{TileIndex(m.X, ts), TileIndex(m.Y, ts)}] = true
		}
	}
	return occupied
}
func (g *MMGame) harvestPlacementAllowed(x, y int) bool {
	if !g.harvestGround(x, y) {
		return false
	}
	tm := world.GlobalTileManager
	if tm == nil {
		return false
	}
	def := tm.GetTileData(g.world.Tiles[y][x])
	policy := config.GlobalAlchemySpawns.Placement
	if def == nil || !slices.Contains(policy.TerrainTypes, def.Type) {
		return false
	}
	for _, key := range policy.ExcludeTiles {
		if key == tm.GetTileKey(g.world.Tiles[y][x]) {
			return false
		}
	}
	return true
}
func (g *MMGame) advanceHarvestSearch(job *harvestSearch) bool {
	ts := float64(g.config.GetTileSize())
	policy := spatialTerrain{World3D: g.world, g: g, region: job.region, buildings: job.buildings}
	end := job.cursor + harvestSearchTilesPerUpdate
	for job.cursor < len(job.queue) && job.cursor < end {
		p := job.queue[job.cursor]
		job.cursor++
		if g.harvestPlacementAllowed(p[0], p[1]) && !job.occupied[p] && p != job.start {
			job.free = append(job.free, p)
		}
		for _, delta := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			q := [2]int{p[0] + delta[0], p[1] + delta[1]}
			if job.seen[q] {
				continue
			}
			if !g.harvestGround(q[0], q[1]) || policy.IsTileBlocking(q[0], q[1]) {
				continue
			}
			ax, ay := (float64(p[0])+.5)*ts, (float64(p[1])+.5)*ts
			bx, by := (float64(q[0])+.5)*ts, (float64(q[1])+.5)*ts
			if g.collisionSystem != nil && !g.collisionSystem.CheckLineOfSight(ax, ay, bx, by) {
				continue
			}
			job.seen[q] = true
			job.queue = append(job.queue, q)
		}
	}
	return job.cursor == len(job.queue)
}
func chooseHarvest(weights map[string]int) string {
	keys := make([]string, 0, len(weights))
	total := 0
	for k, w := range weights {
		keys = append(keys, k)
		total += w
	}
	sort.Strings(keys)
	if total <= 0 {
		return ""
	}
	roll := rand.Intn(total)
	for _, k := range keys {
		roll -= weights[k]
		if roll < 0 {
			return k
		}
	}
	return ""
}
func (g *MMGame) updateAlchemyHarvest() {
	if config.GlobalAlchemySpawns == nil || g.world == nil || g.camera == nil || g.config == nil {
		return
	}
	region, _, _ := canonicalPosition(g.camera.X, g.camera.Y)
	r := &g.harvestRuntime
	if r.world != g.world || r.catalog != config.GlobalAlchemySpawns || r.region != region {
		*r = harvestRuntime{world: g.world, catalog: config.GlobalAlchemySpawns, region: region, dirty: true}
	}
	if r.dirty {
		g.syncHarvestProps()
		r.dirty = false
	}
	if !g.hasHarvestAlchemist() {
		r.search = nil
		return
	}
	day := g.currentCalendarDay()
	if g.alchemy.Populations == nil {
		g.alchemy.Populations = map[string]HarvestPopulationState{}
	}
	pending := false
	for _, p := range config.GlobalAlchemySpawns.Populations {
		if p.Map != region {
			continue
		}
		id := p.Map + ":" + p.Key
		saved, exists := g.alchemy.Populations[id]
		if exists && saved.Day == day {
			continue
		}
		if len(saved.Nodes) >= p.Count {
			saved.Day = day
			g.alchemy.Populations[id] = saved
			continue
		}
		pending = true
	}
	if !pending {
		r.search = nil
		return
	}
	if r.search == nil || r.search.day != day {
		ts := float64(g.config.GetTileSize())
		start := [2]int{TileIndex(g.camera.X, ts), TileIndex(g.camera.Y, ts)}
		r.search = &harvestSearch{region: region, day: day, start: start, queue: [][2]int{start}, seen: map[[2]int]bool{start: true}, occupied: g.harvestOccupied(), buildings: g.buildingOccupiedTiles()}
	}
	if !g.advanceHarvestSearch(r.search) {
		return
	}
	free := r.search.free
	r.search = nil
	rand.Shuffle(len(free), func(i, j int) { free[i], free[j] = free[j], free[i] })
	// Recheck current occupancy after the incremental search; actors may have moved.
	occupied := g.harvestOccupied()
	for _, pop := range g.alchemy.Populations {
		for _, n := range pop.Nodes {
			if n.Region != region {
				continue
			}
			x, y := n.X, n.Y
			if wm := world.GlobalWorldManager; wm != nil {
				x, y = wm.ProjectTile(region, x, y)
			}
			occupied[[2]int{x, y}] = true
		}
	}
	for _, p := range config.GlobalAlchemySpawns.Populations {
		if p.Map != region {
			continue
		}
		id := p.Map + ":" + p.Key
		saved, exists := g.alchemy.Populations[id]
		if exists && saved.Day == day {
			continue
		}
		saved.Day = day
		for len(saved.Nodes) < p.Count && len(free) > 0 {
			xy := free[len(free)-1]
			free = free[:len(free)-1]
			if occupied[xy] || !g.harvestPlacementAllowed(xy[0], xy[1]) {
				continue
			}
			lx, ly := xy[0], xy[1]
			if wm := world.GlobalWorldManager; wm != nil {
				lx, ly = wm.LocalizeTile(region, lx, ly)
			}
			key := chooseHarvest(p.Weights)
			g.alchemy.Serial++
			saved.Nodes = append(saved.Nodes, HarvestNode{Key: key, Quantity: p.Yield, Region: region, X: lx, Y: ly, ID: fmt.Sprintf("harvest:%d", g.alchemy.Serial)})
			occupied[xy] = true
		}
		g.alchemy.Populations[id] = saved
	}
	g.syncHarvestProps()
}
func (g *MMGame) syncHarvestProps() {
	existing := map[string]*character.NPC{}
	kept := g.world.NPCs[:0]
	for _, n := range g.world.NPCs {
		if n != nil && n.HarvestOwner != "" {
			existing[n.Key] = n
		} else {
			kept = append(kept, n)
		}
	}
	g.world.NPCs = kept
	ts := float64(g.config.GetTileSize())
	buildings := g.buildingOccupiedTiles()
	for _, p := range config.GlobalAlchemySpawns.Populations {
		if !mapKeyOnCurrentWorld(p.Map) {
			continue
		}
		id := p.Map + ":" + p.Key
		saved := g.alchemy.Populations[id]
		valid := make([]HarvestNode, 0, len(saved.Nodes))
		for _, node := range saved.Nodes {
			d, ok := config.GetItemDefinition(node.Key)
			if !ok {
				valid = append(valid, node)
				continue
			}
			x, y := node.X, node.Y
			if wm := world.GlobalWorldManager; wm != nil {
				x, y = wm.ProjectTile(p.Map, x, y)
			}
			if buildings[[2]int{x, y}] {
				// Repair old saves containing unreachable nodes under a building.
				// Refill the missing node once, using the normal placement search.
				saved.Day = g.currentCalendarDay() - 1
				continue
			}
			valid = append(valid, node)
			n := existing[node.ID]
			if n == nil {
				n = &character.NPC{Key: node.ID, Name: d.Name, Type: character.NPCTypeHarvest, RenderCategory: npcCatName[catScenery], Transparent: true, SizeClass: "small_prop", Sprite: d.HarvestSprite, PromptVerb: "gather", HarvestOwner: id, HarvestItem: node.Key}
			}
			n.X, n.Y = (float64(x)+.5)*ts, (float64(y)+.5)*ts
			g.world.NPCs = append(g.world.NPCs, n)
		}
		if len(valid) != len(saved.Nodes) {
			saved.Nodes = valid
			g.alchemy.Populations[id] = saved
		}
	}
}
func (g *MMGame) gatherAlchemyReagent(n *character.NPC) bool {
	if n == nil || n.HarvestOwner == "" || !g.hasHarvestAlchemist() || !g.canReachWorldReward(n.X, n.Y) {
		return false
	}
	if Distance(g.camera.X, g.camera.Y, n.X, n.Y) > 2*float64(g.config.GetTileSize()) {
		return false
	}
	p, ok := g.alchemy.Populations[n.HarvestOwner]
	if !ok {
		return false
	}
	index := -1
	for i, node := range p.Nodes {
		if node.ID == n.Key {
			index = i
			break
		}
	}
	if index < 0 {
		return false
	}
	item, err := items.TryCreateItemFromYAML(p.Nodes[index].Key)
	if err != nil {
		return false
	}
	item.Quantity = max(1, p.Nodes[index].Quantity)
	p.Nodes = append(p.Nodes[:index], p.Nodes[index+1:]...)
	g.alchemy.Populations[n.HarvestOwner] = p
	g.harvestRuntime.dirty = true
	g.party.AddItem(item)
	g.AddCombatMessage("Gathered " + item.Name + ".")
	n.Visited = true
	g.focusedNPC = nil
	return true
}
