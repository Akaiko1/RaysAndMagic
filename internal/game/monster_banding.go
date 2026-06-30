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

// updateMonsterBands runs once per tick AFTER movement + separation. It clusters
// same-key banding mobs by proximity (opportunistic — no active seeking):
//   - an all-calm cluster of >=2 STACKS onto its leader (snapped together, fanned
//   - centred in render) and so patrols as one as the leader wanders;
//   - a cluster that already holds an aggro/hit member SCATTERS its calm remainder
//     (engage + ring reposition) so the band breaks apart to fight individually.
//
// A hit propagates for free: TakeDamage makes the struck mob non-calm, so next
// tick its band is a mixed cluster and the rest scatter + aggro.
func (gl *GameLoop) updateMonsterBands() {
	if gl.game.world == nil || gl.game.collisionSystem == nil {
		return
	}
	monsters := gl.game.world.Monsters
	tile := float64(gl.game.config.GetTileSize())
	bindDistSq := (bandBindDistTiles * tile) * (bandBindDistTiles * tile)

	banders := make([]*monster.Monster3D, 0, len(monsters))
	for _, m := range monsters {
		if m != nil && m.Banding && m.IsAlive() {
			m.BandStackIndex, m.BandStackCount = 0, 0 // reset; recomputed below
			banders = append(banders, m)
		}
	}
	if len(banders) < 2 {
		return
	}

	// Union-find clusters: same Key + within bind distance (transitive).
	parent := make([]int, len(banders))
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
	for i := 0; i < len(banders); i++ {
		for j := i + 1; j < len(banders); j++ {
			if banders[i].Key != banders[j].Key {
				continue
			}
			dx := banders[i].X - banders[j].X
			dy := banders[i].Y - banders[j].Y
			if dx*dx+dy*dy <= bindDistSq {
				parent[find(i)] = find(j)
			}
		}
	}
	clusters := map[int][]*monster.Monster3D{}
	for i, m := range banders {
		r := find(i)
		clusters[r] = append(clusters[r], m)
	}

	for _, group := range clusters {
		if len(group) < 2 {
			continue
		}
		// Stable order by ID → stable leader + stack indices (no render flicker).
		sort.Slice(group, func(a, b int) bool { return group[a].ID < group[b].ID })
		var calm []*monster.Monster3D
		hasAggro := false
		for _, m := range group {
			if isCalmBander(m) {
				calm = append(calm, m)
			} else {
				hasAggro = true
			}
		}
		if hasAggro {
			if len(calm) > 0 {
				gl.scatterBand(calm, group, tile)
			}
			continue
		}
		if len(calm) < 2 {
			continue
		}
		// All calm → stack on the leader so the flock moves as one.
		leader := calm[0]
		for idx, m := range calm {
			m.BandStackIndex = idx
			m.BandStackCount = len(calm)
			if idx == 0 {
				continue // leader keeps its own wandering position
			}
			m.X, m.Y = leader.X, leader.Y
			gl.game.collisionSystem.UpdateEntity(m.ID, m.X, m.Y)
		}
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
		m.BandStackIndex, m.BandStackCount = 0, 0
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
