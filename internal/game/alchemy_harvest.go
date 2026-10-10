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
	world     *world.World3D
	catalog   *config.AlchemySpawnConfig
	region    string
	dirty     bool
	repairDay int
	search    *harvestSearch
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

// harvestPlacementAllowed is the ground rule for one herb tile of region, in
// the current world's tiles. Herbs are saved map-local, so a cell the stitch
// rewrote (corridor, carved opening, removed device or gate) is refused: in
// the split world it is a wall, off the map or a travel device.
func (g *MMGame) harvestPlacementAllowed(region string, x, y int) bool {
	if !g.harvestGround(x, y) {
		return false
	}
	if wm := world.GlobalWorldManager; wm != nil {
		if lx, ly := wm.LocalizeTile(region, x, y); !wm.SourceTile(region, lx, ly) {
			return false
		}
	}
	if a := g.adventureConfig(region); a != nil && a.OpeningOwned {
		for _, link := range a.JumpLinks {
			if (x == link[0] && y == link[1]) || (x == link[2] && y == link[3]) {
				return false
			}
		}
		for _, effect := range a.Effects {
			if effect.Contains(x, y) || (effect.Kind == "transfer" && effect.Destination == [2]int{x, y}) {
				return false
			}
		}
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
		if g.harvestPlacementAllowed(job.region, p[0], p[1]) && !job.occupied[p] && p != job.start {
			job.free = append(job.free, p)
		}
		// Closed interiors include their authored Jump and transport edges.
		// Population search is topology-only; it spends no mana or turn and
		// ignores temporary actor occupancy along the route.
		if a := g.adventureConfig(job.region); a != nil && a.OpeningOwned {
			var receivers [][2]int
			for _, link := range a.JumpLinks {
				if p == [2]int{link[0], link[1]} {
					receivers = append(receivers, [2]int{link[2], link[3]})
				}
				if p == [2]int{link[2], link[3]} {
					receivers = append(receivers, [2]int{link[0], link[1]})
				}
			}
			for _, e := range a.Effects {
				if e.Kind == "transfer" && e.TriggerLane == "" && e.Contains(p[0], p[1]) {
					receivers = append(receivers, e.Destination)
				}
			}
			for _, q := range receivers {
				if !job.seen[q] && g.harvestGround(q[0], q[1]) && !policy.IsTileBlocking(q[0], q[1]) {
					job.seen[q] = true
					job.queue = append(job.queue, q)
				}
			}
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
	region, _, _ := g.canonicalPosition(g.camera.X, g.camera.Y)
	r := &g.harvestRuntime
	if r.world != g.world || r.catalog != config.GlobalAlchemySpawns || r.region != region {
		*r = harvestRuntime{world: g.world, catalog: config.GlobalAlchemySpawns, region: region, dirty: true}
	}
	if r.dirty {
		r.repairDay = 0
		g.syncHarvestProps()
		r.dirty = false
	}
	if !g.hasHarvestAlchemist() {
		r.search = nil
		return
	}
	day := g.currentCalendarDay()
	if a := g.adventureConfig(region); a != nil && a.OpeningOwned {
		v := g.adventure.Visits[region]
		if v == nil {
			return
		}
		day = v.Generation
	}
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
		repair := r.repairDay != day && slices.ContainsFunc(saved.Nodes, func(n HarvestNode) bool { return n.Relocate })
		if exists && saved.Day == day && !repair {
			continue
		}
		if len(saved.Nodes) >= p.Count && !repair {
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
	// Relocation consumes a free position, not a fresh material or a day reset.
	nextPosition := func() (int, int, bool) {
		for len(free) > 0 {
			xy := free[len(free)-1]
			free = free[:len(free)-1]
			if occupied[xy] || !g.harvestPlacementAllowed(region, xy[0], xy[1]) {
				continue
			}
			occupied[xy] = true
			lx, ly := xy[0], xy[1]
			if wm := world.GlobalWorldManager; wm != nil {
				lx, ly = wm.LocalizeTile(region, lx, ly)
			}
			return lx, ly, true
		}
		return 0, 0, false
	}
	for _, p := range config.GlobalAlchemySpawns.Populations {
		if p.Map != region {
			continue
		}
		id := p.Map + ":" + p.Key
		saved, exists := g.alchemy.Populations[id]
		refill := !exists || saved.Day != day
		if r.repairDay != day {
			for i := range saved.Nodes {
				node := &saved.Nodes[i]
				if !node.Relocate {
					continue
				}
				lx, ly, ok := nextPosition()
				if !ok {
					break
				}
				node.X, node.Y, node.Relocate = lx, ly, false
			}
		}
		saved.Day = day
		for refill && len(saved.Nodes) < p.Count {
			lx, ly, ok := nextPosition()
			if !ok {
				break
			}
			key := chooseHarvest(p.Weights)
			g.alchemy.Serial++
			saved.Nodes = append(saved.Nodes, HarvestNode{Key: key, Quantity: p.Yield, Region: region, X: lx, Y: ly, ID: fmt.Sprintf("harvest:%d", g.alchemy.Serial)})
		}
		g.alchemy.Populations[id] = saved
	}
	// If no free position exists, retry after a world change, pickup or dawn,
	// instead of scanning the whole region continuously.
	r.repairDay = day
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
	npcTiles := map[[2]int]bool{}
	for _, n := range g.world.NPCs {
		if n != nil {
			npcTiles[[2]int{TileIndex(n.X, ts), TileIndex(n.Y, ts)}] = true
		}
	}
	for _, p := range config.GlobalAlchemySpawns.Populations {
		if !mapKeyOnCurrentWorld(p.Map) {
			continue
		}
		id := p.Map + ":" + p.Key
		saved := g.alchemy.Populations[id]
		for i := range saved.Nodes {
			node := &saved.Nodes[i]
			d, ok := config.GetItemDefinition(node.Key)
			if !ok {
				continue
			}
			x, y := node.X, node.Y
			if wm := world.GlobalWorldManager; wm != nil {
				x, y = wm.ProjectTile(p.Map, x, y)
			}
			if buildings[[2]int{x, y}] || npcTiles[[2]int{x, y}] || !g.harvestPlacementAllowed(p.Map, x, y) {
				// The saved tile is not harvest ground in the world just loaded:
				// an old save, a newly occupied tile, or a cell the other world
				// mode rewrites. Preserve the remaining stock and its identity
				// while repairing. Never rewind its day.
				node.Relocate = true
			}
			if node.Relocate {
				continue
			}
			n := existing[node.ID]
			if n == nil {
				n = &character.NPC{Key: node.ID, Name: d.Name, Type: character.NPCTypeHarvest, RenderCategory: npcCatName[catScenery], Transparent: true, SizeClass: "small_prop", Sprite: d.HarvestSprite, PromptVerb: "gather", HarvestOwner: id, HarvestItem: node.Key}
			}
			n.X, n.Y = (float64(x)+.5)*ts, (float64(y)+.5)*ts
			g.world.NPCs = append(g.world.NPCs, n)
		}
		if _, exists := g.alchemy.Populations[id]; exists {
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
