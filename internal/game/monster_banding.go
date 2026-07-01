package game

import (
	"math"
	"sort"

	"ugataima/internal/monster"
)

const (
	// bandBindDistTiles: same-key calm mobs within this distance merge into a band.
	// Kept near one tile so a band forms when a mob actually wanders into another
	// (opportunistic), not by magnetically yanking distant mobs together.
	bandBindDistTiles = 0.85
	// bandFanRadiusTiles: render fan radius — stacked followers ring the leader so
	// the pile reads as several mobs, centred on the tile.
	bandFanRadiusTiles = 0.16
	// maxBandStackCount caps one calm visual/position band. Extra nearby mobs
	// stay separate instead of being pulled into the band.
	maxBandStackCount = 3
)

// isCalmBander reports whether a banding monster is in a calm (non-aggro) state
// and so eligible to stack into a flock.
func isCalmBander(m *monster.Monster3D) bool {
	if m == nil || !m.Banding || !m.IsAlive() {
		return false
	}
	if m.IsEngagingPlayer || m.WasAttacked || m.Relentless || m.BossAggro {
		return false
	}
	return m.State == monster.StateIdle || m.State == monster.StatePatrolling
}

type monsterBandGroup struct {
	id      int
	members []*monster.Monster3D
}

// updateMonsterBands runs once per tick AFTER movement + separation. It treats
// bands as stable runtime groups:
//   - existing bands stay bands and may recruit only solo same-key calm mobs;
//   - existing band + existing band never merges;
//   - solo calm mobs may form new bands with other solo calm mobs;
//   - each band caps at maxBandStackCount.
//
// A hit propagates for free: TakeDamage makes the struck mob non-calm, so next
// tick its existing band scatters + aggros.
func (gl *GameLoop) updateMonsterBands() {
	if gl.game.world == nil || gl.game.collisionSystem == nil {
		return
	}
	monsters := gl.game.world.Monsters
	tile := float64(gl.game.config.GetTileSize())
	bindDistSq := (bandBindDistTiles * tile) * (bandBindDistTiles * tile)

	banders := make([]*monster.Monster3D, 0, len(monsters))
	existing := map[int][]*monster.Monster3D{}
	maxBandID := 0
	for _, m := range monsters {
		if m == nil || !m.Banding || !m.IsAlive() {
			continue
		}
		m.BandStackIndex, m.BandStackCount = 0, 0 // reset; recomputed below
		banders = append(banders, m)
		if m.BandID > 0 {
			existing[m.BandID] = append(existing[m.BandID], m)
			if m.BandID > maxBandID {
				maxBandID = m.BandID
			}
		}
	}
	if len(banders) == 0 {
		return
	}

	var bands []monsterBandGroup
	singles := make([]*monster.Monster3D, 0, len(banders))
	handled := map[*monster.Monster3D]bool{}

	bandIDs := make([]int, 0, len(existing))
	for id := range existing {
		bandIDs = append(bandIDs, id)
	}
	sort.Ints(bandIDs)
	for _, id := range bandIDs {
		group := existing[id]
		sortMonstersByID(group)
		var calm []*monster.Monster3D
		hasAggro := false
		for _, m := range group {
			handled[m] = true
			if isCalmBander(m) {
				calm = append(calm, m)
			} else {
				hasAggro = true
			}
		}
		if hasAggro {
			for _, m := range group {
				leaveBand(m)
			}
			if len(calm) > 0 {
				gl.scatterBand(calm, group, tile)
			}
			continue
		}
		if len(calm) < 2 {
			for _, m := range calm {
				leaveBand(m)
				singles = append(singles, m)
			}
			continue
		}
		// Keep the stable leader first (and safe from the overflow trim) so the
		// band's snap position doesn't jump when a lower-ID mob is in the group.
		hoistBandLeader(calm)
		if len(calm) > maxBandStackCount {
			for _, m := range calm[maxBandStackCount:] {
				leaveBand(m)
				singles = append(singles, m)
			}
			calm = calm[:maxBandStackCount]
		}
		bands = append(bands, monsterBandGroup{id: id, members: calm})
	}

	for _, m := range banders {
		if handled[m] {
			continue
		}
		leaveBand(m)
		if isCalmBander(m) {
			singles = append(singles, m)
		}
	}
	sortMonstersByID(singles)

	usedSingles := map[*monster.Monster3D]bool{}
	for bi := range bands {
		band := &bands[bi]
		for len(band.members) < maxBandStackCount {
			next := firstRecruitableSingle(band.members, singles, usedSingles, bindDistSq)
			if next == nil {
				break
			}
			usedSingles[next] = true
			band.members = append(band.members, next)
		}
	}

	remainingSingles := make([]*monster.Monster3D, 0, len(singles))
	for _, m := range singles {
		if !usedSingles[m] {
			remainingSingles = append(remainingSingles, m)
		}
	}
	for _, group := range soloBandClusters(remainingSingles, bindDistSq) {
		for start := 0; start < len(group); start += maxBandStackCount {
			end := start + maxBandStackCount
			if end > len(group) {
				end = len(group)
			}
			chunk := group[start:end]
			if len(chunk) < 2 {
				leaveBand(chunk[0])
				continue
			}
			maxBandID++
			bands = append(bands, monsterBandGroup{id: maxBandID, members: chunk})
		}
	}

	for _, band := range bands {
		gl.stackMonsterBand(band.id, band.members)
	}
}

func sortMonstersByID(monsters []*monster.Monster3D) {
	sort.Slice(monsters, func(a, b int) bool { return monsters[a].ID < monsters[b].ID })
}

// leaveBand clears a monster's runtime band membership (it becomes solo/unbanded).
func leaveBand(m *monster.Monster3D) {
	m.BandID, m.BandLeaderID = 0, ""
	m.BandStackIndex, m.BandStackCount = 0, 0
}

// hoistBandLeader moves the previously-marked leader (the member whose stored
// BandLeaderID points to itself) to the front, preserving the order of the rest,
// so the band's leader — and thus its snap position — stays stable across ticks
// even when a lower-ID mob joins. No-op if the old leader is gone (lowest ID leads).
func hoistBandLeader(members []*monster.Monster3D) {
	for i, m := range members {
		if m.ID == m.BandLeaderID {
			if i != 0 {
				leader := members[i]
				copy(members[1:i+1], members[0:i])
				members[0] = leader
			}
			return
		}
	}
}

// withinBindDist reports whether two mobs are close enough to band together.
func withinBindDist(a, b *monster.Monster3D, bindDistSq float64) bool {
	dx := a.X - b.X
	dy := a.Y - b.Y
	return dx*dx+dy*dy <= bindDistSq
}

func firstRecruitableSingle(band, singles []*monster.Monster3D, used map[*monster.Monster3D]bool, bindDistSq float64) *monster.Monster3D {
	for _, single := range singles {
		if used[single] || single.Key != band[0].Key {
			continue
		}
		for _, member := range band {
			if withinBindDist(single, member, bindDistSq) {
				return single
			}
		}
	}
	return nil
}

func soloBandClusters(singles []*monster.Monster3D, bindDistSq float64) [][]*monster.Monster3D {
	if len(singles) < 2 {
		if len(singles) == 1 {
			return [][]*monster.Monster3D{singles}
		}
		return nil
	}
	parent := make([]int, len(singles))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for i := 0; i < len(singles); i++ {
		for j := i + 1; j < len(singles); j++ {
			if singles[i].Key != singles[j].Key {
				continue
			}
			if withinBindDist(singles[i], singles[j], bindDistSq) {
				parent[find(i)] = find(j)
			}
		}
	}
	clustersByRoot := map[int][]*monster.Monster3D{}
	for i, m := range singles {
		clustersByRoot[find(i)] = append(clustersByRoot[find(i)], m)
	}
	// Sort each cluster once, then order clusters by their lowest ID → stable
	// leader + BandID assignment (no render flicker across ticks).
	clusters := make([][]*monster.Monster3D, 0, len(clustersByRoot))
	for _, group := range clustersByRoot {
		sortMonstersByID(group)
		clusters = append(clusters, group)
	}
	sort.Slice(clusters, func(a, b int) bool {
		return clusters[a][0].ID < clusters[b][0].ID
	})
	return clusters
}

func (gl *GameLoop) stackMonsterBand(id int, band []*monster.Monster3D) {
	if len(band) < 2 {
		return
	}
	leader := band[0]
	for idx, m := range band {
		m.BandID = id
		m.BandLeaderID = leader.ID
		m.BandStackIndex = idx
		m.BandStackCount = len(band)
		if idx == 0 {
			continue // leader keeps its own wandering position
		}
		m.X, m.Y = leader.X, leader.Y
		gl.game.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
	}
}

// bandScatterRing is the search order for scatter destinations: 8 adjacent tiles
// first, then a ring further out ("через тайл") when the near ones are taken.
var bandScatterRing = [][2]int{
	{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1},
	{2, 0}, {-2, 0}, {0, 2}, {0, -2}, {2, 1}, {-2, 1}, {1, 2}, {-1, 2},
}

// scatterBand aggros every still-calm member of a triggered band and repositions
// each onto a distinct walkable tile around the band centroid, so a stacked band
// bursts apart the moment one of them engages or is hit.
func (gl *GameLoop) scatterBand(calm, group []*monster.Monster3D, tile float64) {
	var cx, cy float64
	for _, m := range group {
		cx += m.X
		cy += m.Y
	}
	cx /= float64(len(group))
	cy /= float64(len(group))
	ctx, cty := int(cx/tile), int(cy/tile)

	used := map[[2]int]bool{}
	ri := 0
	for _, m := range calm {
		m.IsEngagingPlayer = true // engage the whole band
		m.State = monster.StateAlert
		m.WasAttacked = true
		leaveBand(m)
		for ri < len(bandScatterRing) {
			d := bandScatterRing[ri]
			ri++
			key := [2]int{ctx + d[0], cty + d[1]}
			if used[key] {
				continue
			}
			nx, ny := TileCenterFromTile(key[0], key[1], tile)
			if gl.game.collisionSystem.CanMoveToWithHabitat(m.ID, nx, ny, m.HabitatPrefs, m.Flying) {
				used[key] = true
				m.X, m.Y = nx, ny
				gl.game.collisionSystem.UpdateEntity(m.ID, nx, ny)
				m.ResetPathfinding()
				break
			}
		}
		// No free tile found → the mob still engages from where it stands.
	}
}

// bandFanOffset returns the render-only world offset for a stacked band member:
// the leader (index 0) sits at the tile centre, followers ring it evenly so the
// pile reads as several mobs without the actual (co-located) positions moving.
func bandFanOffset(idx, count int, tile float64) (float64, float64) {
	if count <= 1 || idx <= 0 {
		return 0, 0
	}
	r := bandFanRadiusTiles * tile
	ang := (float64(idx-1) / float64(count-1)) * 2 * math.Pi
	return math.Cos(ang) * r, math.Sin(ang) * r
}
