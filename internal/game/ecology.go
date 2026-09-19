package game

import (
	"maps"
	"math"
	"math/rand"
	"sort"
	uitext "ugataima/assets/text"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// EcologyState belongs to the campaign, never the account-wide profile.
type EcologyState struct {
	PopulationPhases map[string]int `json:"population_phases,omitempty"`
	Unlocked         bool           `json:"unlocked,omitempty"`
	ActorID          string         `json:"actor_id,omitempty"`
	Route            string         `json:"route,omitempty"`
	Checkpoint       int            `json:"checkpoint,omitempty"`
	Returning        bool           `json:"returning,omitempty"`
	RespawnDay       int            `json:"respawn_day,omitempty"`
	StopFrames       int            `json:"stop_frames,omitempty"`
	Deliveries       int            `json:"deliveries,omitempty"`
	Stock            map[string]int `json:"stock,omitempty"`
}

func ecologyWorld(key string) *world.World3D {
	wm := world.GlobalWorldManager
	if wm == nil {
		return nil
	}
	if wm.IsOpenWorldRegion(key) {
		return wm.OpenWorld
	}
	return wm.LoadedMaps[key]
}
func ecologyWorlds() []*world.World3D {
	wm := world.GlobalWorldManager
	if wm == nil {
		return nil
	}
	keys := make([]string, 0, len(wm.LoadedMaps))
	for k := range wm.LoadedMaps {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []*world.World3D
	if wm.OpenWorld != nil {
		out = append(out, wm.OpenWorld)
	}
	seen := map[*world.World3D]bool{}
	if wm.OpenWorld != nil {
		seen[wm.OpenWorld] = true
	}
	for _, k := range keys {
		if w := wm.LoadedMaps[k]; w != nil && !seen[w] {
			out = append(out, w)
			seen[w] = true
		}
	}
	return out
}
func ecologyPoint(p config.RoutePoint, tile float64) (*world.World3D, float64, float64) {
	x, y := world.GlobalWorldManager.ProjectTile(p.Map, p.X, p.Y)
	return ecologyWorld(p.Map), (float64(x) + .5) * tile, (float64(y) + .5) * tile
}
func (g *MMGame) ecologyActor() (*world.World3D, *monster.Monster3D) {
	for _, w := range ecologyWorlds() {
		for _, m := range w.Monsters {
			if m.ID == g.ecology.ActorID {
				return w, m
			}
		}
	}
	return nil, nil
}
func (g *MMGame) ecologyCollision(w *world.World3D) *collision.CollisionSystem {
	if w == nil {
		return nil
	}
	if w == g.world {
		return g.collisionSystem
	}
	c := collision.NewCollisionSystem(w, float64(g.config.GetTileSize()))
	for _, m := range w.Monsters {
		if m.IsAlive() {
			bw, bh := m.GetSize()
			c.RegisterEntity(collision.NewEntity(m.ID, m.X, m.Y, bw, bh, collision.CollisionTypeMonster, false))
		}
	}
	return c
}
func (g *MMGame) addEcologyActor(w *world.World3D, m *monster.Monster3D) {
	m.QuestProgressIgnored = true
	if w == g.world {
		g.registerSpawnedMonster(m)
	} else {
		w.Monsters = append(w.Monsters, m)
	}
}

// Replenishment is edge-triggered and persisted even when every animal dies.
// Entering a map or reloading must never refill a depleted same-phase population.
func (g *MMGame) replenishWildlife() {
	c := config.GlobalEcology
	if c == nil || g.config == nil {
		return
	}
	if g.ecology.PopulationPhases == nil {
		g.ecology.PopulationPhases = map[string]int{}
	}
	tile := float64(g.config.GetTileSize())
	for _, p := range c.Populations {
		if (p.Phase == "night") != g.dayNightIsNight {
			continue
		}
		key := p.Map + ":" + p.Monster
		if phase, ok := g.ecology.PopulationPhases[key]; ok && phase == g.dayNightDay {
			continue
		}
		w := ecologyWorld(p.Map)
		if w == nil {
			continue
		}
		g.ecology.PopulationPhases[key] = g.dayNightDay
		n := 0
		occupied := map[[2]int]bool{}
		for _, m := range w.Monsters {
			if !m.IsAlive() {
				continue
			}
			occupied[[2]int{int(m.X / tile), int(m.Y / tile)}] = true
			if m.Population == key {
				n++
			}
		}
		for _, npc := range w.NPCs {
			occupied[[2]int{int(npc.X / tile), int(npc.Y / tile)}] = true
		}
		minX, minY, maxX, maxY := 0, 0, w.Width, w.Height
		wm := world.GlobalWorldManager
		if wm.IsOpenWorldRegion(p.Map) {
			for _, r := range wm.OpenWorldRegions {
				if r.MapKey == p.Map {
					minX, minY, maxX, maxY = r.OffsetX, r.OffsetY, r.OffsetX+r.Width, r.OffsetY+r.Height
					break
				}
			}
		}
		var free [][2]int
		for y := minY; y < maxY; y++ {
			for x := minX; x < maxX; x++ {
				wx, wy := (float64(x)+.5)*tile, (float64(y)+.5)*tile
				if occupied[[2]int{x, y}] || w.IsTileBlockingForMonster(x, y, nil, false) {
					continue
				}
				if w == g.world && g.camera != nil && Distance(wx, wy, g.camera.X, g.camera.Y) < 4*tile {
					continue
				}
				free = append(free, [2]int{x, y})
			}
		}
		rand.Shuffle(len(free), func(i, j int) { free[i], free[j] = free[j], free[i] })
		for i := 0; n < p.Count && i < len(free); i++ {
			xy := free[i]
			m := monster.NewMonster3DFromConfig((float64(xy[0])+.5)*tile, (float64(xy[1])+.5)*tile, p.Monster, g.config)
			m.Population = key
			g.addEcologyActor(w, m)
			n++
		}
	}
}

func (g *MMGame) prepareAmbientTarget(m *monster.Monster3D) bool {
	if !m.IsAmbient() || m.IsPartyControlled() {
		return false
	}
	m.AIFoe = nil
	m.AmbientFlee = false
	g.setWildlifeBounds(m)
	if m.Disposition == "caravan" {
		g.setCaravanTarget(m)
		return true
	}
	tx, ty := m.X, m.Y
	best := m.AmbientAwarenessRadius()
	if g.camera != nil && Distance(m.X, m.Y, g.camera.X, g.camera.Y) < best && (m.Arbor.Height > 0 || g.collisionSystem.CheckLineOfSight(m.X, m.Y, g.camera.X, g.camera.Y)) {
		tx, ty = g.camera.X, g.camera.Y
		best = Distance(m.X, m.Y, tx, ty)
		m.AmbientFlee = true
	}
	for _, other := range g.world.Monsters {
		if !other.IsAlive() || !other.Hunts(m) {
			continue
		}
		d := Distance(m.X, m.Y, other.X, other.Y)
		if d < best && g.collisionSystem.CheckLineOfSight(m.X, m.Y, other.X, other.Y) {
			best, tx, ty = d, other.X, other.Y
			m.AmbientFlee = true
		}
	}
	if m.AmbientFlee {
		m.RememberAmbientThreat(tx, ty)
	} else if m.Threat.Seconds > 0 {
		m.AmbientFlee = true
		tx, ty = m.Threat.X, m.Threat.Y
	}
	if !m.AmbientFlee {
		best = m.PreyRadius
		for _, other := range g.world.Monsters {
			if !m.Hunts(other) || !other.IsAlive() {
				continue
			}
			d := Distance(m.X, m.Y, other.X, other.Y)
			if d < best && g.collisionSystem.CheckLineOfSight(m.X, m.Y, other.X, other.Y) {
				best = d
				m.AIFoe = other
				tx, ty = other.X, other.Y
			}
		}
	}
	m.AITargetX, m.AITargetY = tx, ty
	return true
}
func (g *MMGame) caravanFoe(m *monster.Monster3D) *monster.Monster3D {
	if m.IsAmbient() || m.IsPartyControlled() || m.IsInertSetPiece() || m.BossEvasive {
		return nil
	}
	other := g.ecologyCaravan
	if other == nil || !m.CanAttackActor(other) {
		return nil
	}
	d := Distance(m.X, m.Y, other.X, other.Y)
	if d > m.AlertRadius || !g.collisionSystem.CheckLineOfSight(m.X, m.Y, other.X, other.Y) {
		return nil
	}
	if m.TargetsParty() && g.camera != nil && Distance(m.X, m.Y, g.camera.X, g.camera.Y) <= d {
		return nil
	}
	return other
}
func (cs *CombatSystem) finishActorKill(attacker, target *monster.Monster3D) {
	if attacker != nil && attacker.Disposition == "wildlife" && !attacker.IsPartyControlled() {
		target.NoKillRewards = true
	}
	cs.finishMonsterKillImmediately(target)
}

func (g *MMGame) caravanRoute() *config.CaravanRoute {
	if config.GlobalEcology == nil {
		return nil
	}
	for i := range config.GlobalEcology.Caravan.Routes {
		r := &config.GlobalEcology.Caravan.Routes[i]
		if r.ID == g.ecology.Route {
			return r
		}
	}
	return nil
}
func (g *MMGame) chooseCaravanRoute() {
	routes := config.GlobalEcology.Caravan.Routes
	r := routes[rand.Intn(len(routes))]
	g.ecology.Route = r.ID
	g.ecology.Checkpoint = 0
	g.ecology.Returning = false
}
func (g *MMGame) setCaravanTarget(m *monster.Monster3D) {
	m.AITargetX, m.AITargetY = m.X, m.Y
	r := g.caravanRoute()
	if r == nil || g.ecology.StopFrames > 0 || g.ecology.Checkpoint < 0 || g.ecology.Checkpoint >= len(r.Points) {
		return
	}
	next, x, y := ecologyPoint(r.Points[g.ecology.Checkpoint], float64(g.config.GetTileSize()))
	if next != g.world {
		return
	} // wait at the exit if the paired entrance is occupied
	m.AITargetX, m.AITargetY = x, y
}
func (g *MMGame) spawnCaravan() {
	g.chooseCaravanRoute()
	r := g.caravanRoute()
	w, x, y := ecologyPoint(r.Points[0], float64(g.config.GetTileSize()))
	if w == nil {
		return
	}
	m := monster.NewMonster3DFromConfig(x, y, config.GlobalEcology.Caravan.Monster, g.config)
	checker := g.ecologyCollision(w)
	bw, bh := m.GetSize()
	checker.RegisterEntity(collision.NewEntity(m.ID, x, y, bw, bh, collision.CollisionTypeMonster, false))
	free := checker.CanMoveToWithTileOverrides(m.ID, x, y, nil, false)
	checker.UnregisterEntity(m.ID)
	if !free || g.ecologyPointOccupied(w, x, y, m.ID) {
		return
	}
	g.ecology.ActorID = m.ID
	g.ecology.RespawnDay = 0
	g.ecology.Checkpoint = 1
	g.addEcologyActor(w, m)
}
func (g *MMGame) unlockCaravan() {
	c := config.GlobalEcology
	if c == nil || g.ecology.Unlocked || g.questManager == nil {
		return
	}
	q := g.questManager.GetQuest(c.Caravan.UnlockQuest)
	if q != nil && (q.RewardsClaimed || q.ClaimedAtDay > 0) {
		g.ecology.Unlocked = true
		g.syncCaravanStock()
	}
}
func (g *MMGame) updateEcology() {
	c := config.GlobalEcology
	if c == nil || world.GlobalWorldManager == nil || g.world == nil {
		return
	}
	g.replenishWildlife()
	g.unlockCaravan()
	if !g.ecology.Unlocked {
		return
	}
	w, m := g.ecologyActor()
	if m == nil || !m.IsAlive() {
		if g.ecology.ActorID != "" && g.ecology.RespawnDay == 0 {
			g.ecology.RespawnDay = g.currentCalendarDay() + 1
		}
		if g.ecology.RespawnDay == 0 || g.currentCalendarDay() >= g.ecology.RespawnDay {
			g.spawnCaravan()
		}
		return
	}
	if g.ecology.StopFrames > 0 {
		g.ecology.StopFrames--
		return
	}
	if g.turnBasedMode && (g.currentTurn != 1 || g.monsterTurnResolved) {
		return
	}
	r := g.caravanRoute()
	if r == nil {
		return
	}
	tile := float64(g.config.GetTileSize())
	g.ecology.Checkpoint = max(0, min(g.ecology.Checkpoint, len(r.Points)-1))
	point := r.Points[g.ecology.Checkpoint]
	next, x, y := ecologyPoint(point, tile)
	if next != w {
		// Consecutive map points are paired entrance/exit anchors. Only cross after
		// the previous anchor was reached; never move the party or change its map.
		checker := g.ecologyCollision(next)
		if checker == nil {
			return
		}
		bw, bh := m.GetSize()
		checker.RegisterEntity(collision.NewEntity(m.ID, x, y, bw, bh, collision.CollisionTypeMonster, false))
		canEnter := checker.CanMoveToWithTileOverrides(m.ID, x, y, m.WalkableTileOverrides, false)
		checker.UnregisterEntity(m.ID)
		if !canEnter || g.ecologyPointOccupied(next, x, y, m.ID) {
			return
		}
		for i, a := range w.Monsters {
			if a == m {
				w.Monsters = append(w.Monsters[:i], w.Monsters[i+1:]...)
				break
			}
		}
		if w == g.world {
			g.collisionSystem.UnregisterEntity(m.ID)
		}
		m.X, m.Y = x, y
		m.ResetPathfinding()
		g.addEcologyActor(next, m)
		w = next
	}
	if math.Hypot(m.X-x, m.Y-y) > tile*.12 {
		return
	}
	m.X, m.Y = x, y
	if g.ecology.Returning {
		if g.ecology.Checkpoint == 0 {
			g.depositCaravanGoods()
			g.ecology.Deliveries++
			g.chooseCaravanRoute()
			g.ecology.Checkpoint = 1
			g.ecology.StopFrames = c.Caravan.StopSeconds * g.config.GetTPS()
		} else {
			g.ecology.Checkpoint--
		}
	} else if g.ecology.Checkpoint == len(r.Points)-1 {
		g.ecology.Returning = true
		g.ecology.Checkpoint--
		g.ecology.StopFrames = c.Caravan.StopSeconds * g.config.GetTPS()
	} else {
		g.ecology.Checkpoint++
	}
}
func (g *MMGame) depositCaravanGoods() {
	c := config.GlobalEcology.Caravan
	pool := config.CaravanTradePool()
	if len(pool) == 0 {
		return
	}
	if g.ecology.Stock == nil {
		g.ecology.Stock = map[string]int{}
	}
	for i := 0; i < c.RewardUnits; i++ {
		candidates := pool
		if len(g.ecology.Stock) >= c.StockSlots {
			candidates = nil
			for _, k := range pool {
				if g.ecology.Stock[k] > 0 {
					candidates = append(candidates, k)
				}
			}
		}
		if len(candidates) == 0 {
			break
		}
		key := candidates[rand.Intn(len(candidates))]
		g.ecology.Stock[key]++
	}
	g.syncCaravanStock()
}
func (g *MMGame) syncCaravanStock() {
	if config.GlobalEcology == nil {
		return
	}
	keys := make([]string, 0, len(g.ecology.Stock))
	for k, n := range g.ecology.Stock {
		if n > 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, w := range ecologyWorlds() {
		for _, npc := range w.NPCs {
			if npc.Key != config.GlobalEcology.Caravan.Merchant {
				continue
			}
			npc.FreeGoods = g.ecology.Unlocked
			npc.MerchantStock = nil
			if g.dialogNPC == npc {
				g.resetDialogClickTracker()
			}
			for _, key := range keys {
				npc.MerchantStock = append(npc.MerchantStock, &character.MerchantStockItem{Item: items.CreateItemFromYAML(key), Quantity: g.ecology.Stock[key], RewardKey: key})
			}
		}
	}
}

func cloneEcologyState(s EcologyState) EcologyState {
	s.Stock = maps.Clone(s.Stock)
	s.PopulationPhases = maps.Clone(s.PopulationPhases)
	return s
}

func (g *MMGame) caravanStatusText() string {
	if g.ecology.RespawnDay > 0 {
		return uitext.Text("caravan.lost")
	}
	if g.ecology.Returning {
		return uitext.Text("caravan.returning")
	}
	return uitext.Text("caravan.outbound")
}

func (g *MMGame) setWildlifeBounds(m *monster.Monster3D) {
	if m.Disposition != "wildlife" || config.GlobalEcology == nil {
		return
	}
	for _, p := range config.GlobalEcology.Populations {
		if m.Population != p.Map+":"+p.Monster {
			continue
		}
		w := ecologyWorld(p.Map)
		if w == nil {
			return
		}
		b := [4]int{0, 0, w.Width, w.Height}
		wm := world.GlobalWorldManager
		if wm.IsOpenWorldRegion(p.Map) {
			for _, r := range wm.OpenWorldRegions {
				if r.MapKey == p.Map {
					b = [4]int{r.OffsetX, r.OffsetY, r.OffsetX + r.Width, r.OffsetY + r.Height}
					break
				}
			}
		}
		if m.AmbientBounds == nil {
			m.AmbientBounds = new([4]int)
		}
		*m.AmbientBounds = b
		return
	}
}

// Calm monsters may share transit tiles in the ordinary collision policy, but
// an inter-map arrival must never materialize inside an existing actor.
func (g *MMGame) ecologyPointOccupied(w *world.World3D, x, y float64, except string) bool {
	tile := g.config.GetTileSize()
	tx, ty := TileIndex(x, tile), TileIndex(y, tile)
	if w == g.world && g.camera != nil && TileIndex(g.camera.X, tile) == tx && TileIndex(g.camera.Y, tile) == ty {
		return true
	}
	for _, m := range w.Monsters {
		if m.IsAlive() && m.ID != except && TileIndex(m.X, tile) == tx && TileIndex(m.Y, tile) == ty {
			return true
		}
	}
	return false
}
