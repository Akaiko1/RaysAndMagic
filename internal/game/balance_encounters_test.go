//go:build balance

package game

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// balanceEncounter is one group the party meets, read from the world:
//   - single: one monster alone (a boss alone fights with its own summons);
//   - pack: one kind in its usual number (map clusters, day/night packs);
//   - mixed: different kinds that stand together on a map, a boss with its
//     guards included.
type balanceEncounter struct {
	Kind  string   // single | pack | mixed
	Foes  []string // monster keys
	Level int      // strongest member
	Boss  bool
	Where string // map the group was read from ("" for singles)
}

func (e balanceEncounter) name() string {
	counts := map[string]int{}
	var keys []string
	for _, k := range e.Foes {
		if counts[k] == 0 {
			keys = append(keys, k)
		}
		counts[k]++
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k
		if counts[k] > 1 {
			parts[i] += "x" + strconv.Itoa(counts[k])
		}
	}
	return strings.Join(parts, "+")
}

// balanceGroupRadiusTiles: monsters this close to one another meet the party
// together.
const balanceGroupRadiusTiles = 6

// balanceEncounters builds every encounter the world authors.
func balanceEncounters(a *balanceArena, wm *world.WorldManager) []balanceEncounter {
	cfg := a.g.config
	fighter := map[string]bool{}
	level := map[string]int{}
	boss := map[string]bool{}
	for key, def := range monster.MonsterConfig.Monsters {
		m := monster.NewMonster3DFromConfig(0, 0, key, cfg)
		if m == nil || def.Champion != "" || m.IsAmbient() || m.CurrentAIBehavior() == monster.AIBehaviorInert || def.WarlordIdol {
			continue
		}
		fighter[key], level[key], boss[key] = true, def.Level, m.IsBoss()
	}
	var out []balanceEncounter
	seen := map[string]bool{}
	add := func(e balanceEncounter) {
		if len(e.Foes) == 0 || len(e.Foes) > len(balanceFoeTiles) {
			e.Foes = e.Foes[:min(len(e.Foes), len(balanceFoeTiles))]
		}
		for _, k := range e.Foes {
			e.Level = max(e.Level, level[k])
			e.Boss = e.Boss || boss[k]
		}
		id := e.Kind + ":" + e.name()
		if seen[id] {
			return
		}
		seen[id] = true
		out = append(out, e)
	}
	for _, key := range sortedKeys(fighter) {
		add(balanceEncounter{Kind: "single", Foes: []string{key}})
	}
	// Map groups: everything within the radius of each monster.
	ts := a.ts
	for _, mapKey := range sortedKeys(wm.LoadedMaps) {
		w := wm.LoadedMaps[mapKey]
		var placed []*monster.Monster3D
		for _, m := range w.Monsters {
			if fighter[m.Key] {
				placed = append(placed, m)
			}
		}
		for _, seed := range placed {
			var group []*monster.Monster3D
			for _, o := range placed {
				if math.Hypot(o.X-seed.X, o.Y-seed.Y) <= balanceGroupRadiusTiles*ts {
					group = append(group, o)
				}
			}
			sort.Slice(group, func(i, j int) bool {
				return math.Hypot(group[i].X-seed.X, group[i].Y-seed.Y) < math.Hypot(group[j].X-seed.X, group[j].Y-seed.Y)
			})
			if len(group) < 2 {
				continue
			}
			keys := make([]string, 0, len(group))
			kinds := map[string]bool{}
			for _, m := range group {
				keys = append(keys, m.Key)
				kinds[m.Key] = true
			}
			kind := "pack"
			if len(kinds) > 1 {
				kind = "mixed"
			}
			add(balanceEncounter{Kind: kind, Foes: keys, Where: mapKey})
		}
	}
	// Day/night packs spawn at run time, so the maps above do not hold them.
	for _, p := range cfg.DayNight.Packs {
		for _, list := range [][]string{packMembers(p.DayMonsters, p.DayMonster, p.Count), packMembers(p.NightMonsters, p.NightMonster, p.Count)} {
			var keys []string
			kinds := map[string]bool{}
			for _, k := range list {
				if fighter[k] {
					keys = append(keys, k)
					kinds[k] = true
				}
			}
			if len(keys) < 2 {
				continue
			}
			kind := "pack"
			if len(kinds) > 1 {
				kind = "mixed"
			}
			add(balanceEncounter{Kind: kind, Foes: keys, Where: p.Map + " (day/night)"})
		}
	}
	return out
}

func packMembers(list []config.PackMemberConfig, single string, count int) []string {
	var keys []string
	if len(list) > 0 {
		for _, m := range list {
			for range max(1, m.Count) {
				keys = append(keys, m.Monster)
			}
		}
		return keys
	}
	for range count {
		if single != "" {
			keys = append(keys, single)
		}
	}
	return keys
}
