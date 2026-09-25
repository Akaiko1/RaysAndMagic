// Package playerprofile owns lifetime progress independently of save slots.
package playerprofile

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ugataima/internal/storage"
)

const Version = 1

type Entry struct {
	Name      string `json:"name"`
	Icon      string `json:"icon,omitempty"`
	Count     int64  `json:"count"`
	BaseValue int64  `json:"base_value,omitempty"`
}

type Run struct {
	Won  bool `json:"won,omitempty"`
	Lost bool `json:"lost,omitempty"`
}

type Data struct {
	Version  int                         `json:"version"`
	Since    time.Time                   `json:"since"`
	Counters map[string]int64            `json:"counters"`
	Rankings map[string]map[string]Entry `json:"rankings"`
	Unlocked map[string]time.Time        `json:"unlocked"`
	Runs     map[string]Run              `json:"runs"`
	// Only reset metrics need a separate progress period. Missing keys retain
	// legacy lifetime progress. Do not omit this field when a reset is saved.
	AchievementCounters map[string]int64           `json:"achievement_counters"`
	AchievementHeroes   map[string]bool            `json:"achievement_heroes,omitempty"`
	VisitedTiles        map[string]map[string]bool `json:"visited_tiles,omitempty"`
}

func New() Data {
	return Data{Version: Version, Since: time.Now().UTC(), Counters: map[string]int64{}, Rankings: map[string]map[string]Entry{}, Unlocked: map[string]time.Time{}, Runs: map[string]Run{}}
}

// Coordinates, rather than row-major indices, remain stable after a map edit.
func (d *Data) VisitTile(region string, x, y int) {
	if region == "" || x < 0 || y < 0 {
		return
	}
	if d.VisitedTiles == nil {
		d.VisitedTiles = map[string]map[string]bool{}
	}
	if d.VisitedTiles[region] == nil {
		d.VisitedTiles[region] = map[string]bool{}
	}
	d.VisitedTiles[region][strconv.Itoa(x)+","+strconv.Itoa(y)] = true
}

// VisitedTileCount intersects the historical coordinates with today's bounds.
// Coordinates outside a resized map stay stored, ready if the map grows again.
func (d *Data) VisitedTileCount(region string, width, height int) int64 {
	var count int64
	for coordinate, visited := range d.VisitedTiles[region] {
		if !visited {
			continue
		}
		xs, ys, ok := strings.Cut(coordinate, ",")
		x, xerr := strconv.Atoi(xs)
		y, yerr := strconv.Atoi(ys)
		if ok && xerr == nil && yerr == nil && x >= 0 && y >= 0 && x < width && y < height {
			count++
		}
	}
	return count
}

func (d *Data) Add(key string, count int64) {
	if count > 0 {
		d.Counters[key] += count
		if _, tracked := d.AchievementCounters[key]; tracked {
			d.AchievementCounters[key] += count
		}
	}
}
func (d *Data) Observe(key string, value int64) {
	d.ObserveHistorical(key, value)
	if current, tracked := d.AchievementCounters[key]; tracked && value > current {
		d.AchievementCounters[key] = value
	}
}

// ObserveHistorical recovers facts from an existing save. It can establish
// legacy progress, but cannot satisfy a condition in an explicitly reset period.
func (d *Data) ObserveHistorical(key string, value int64) {
	if value > d.Counters[key] {
		d.Counters[key] = value
	}
}

// ResetAchievements starts a new progress period without changing statistics,
// rankings, run history or profile age. Call while no game owns this profile.
func (d *Data) ResetAchievements(metrics []string) {
	d.Unlocked = map[string]time.Time{}
	d.AchievementHeroes = nil
	d.AchievementCounters = make(map[string]int64, len(metrics))
	for _, metric := range metrics {
		d.AchievementCounters[metric] = 0
	}
}

// AchievementProgress is shared by the unlock rules and their UI progress.
func (d *Data) AchievementProgress(metrics []string) int64 {
	progress := int64(0)
	for _, metric := range metrics {
		count, tracked := d.AchievementCounters[metric]
		if !tracked {
			count = d.Counters[metric]
		}
		progress = max(progress, count)
	}
	return progress
}
func (d *Data) Rank(group, key, name, icon string, count int64) {
	if count <= 0 || key == "" {
		return
	}
	if d.Rankings[group] == nil {
		d.Rankings[group] = map[string]Entry{}
	}
	e := d.Rankings[group][key]
	e.Name, e.Icon, e.Count = name, icon, e.Count+count
	d.Rankings[group][key] = e
}

// RankValued keeps the highest observed unit value without multiplying by
// quantity. Older records retain their counts when a value is first observed.
func (d *Data) RankValued(group, key, name, icon string, count, baseValue int64) {
	if count <= 0 || key == "" {
		return
	}
	d.Rank(group, key, name, icon, count)
	e := d.Rankings[group][key]
	e.BaseValue = max(e.BaseValue, baseValue)
	d.Rankings[group][key] = e
}

func (d *Data) MostValuableLoot() []Entry {
	result := make([]Entry, 0, len(d.Rankings["loot"]))
	for _, e := range d.Rankings["loot"] {
		if e.BaseValue > 0 {
			result = append(result, e)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.BaseValue != b.BaseValue {
			return a.BaseValue > b.BaseValue
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Icon < b.Icon
	})
	return result
}
func (d *Data) ObserveRun(id string, won, lost bool) {
	if id == "" {
		return
	}
	r, known := d.Runs[id]
	if !known {
		d.Add("adventures", 1)
	}
	if won && !r.Won {
		d.Add("victories", 1)
		r.Won = true
	}
	if lost && !r.Lost {
		d.Add("defeats", 1)
		r.Lost = true
	}
	d.Runs[id] = r
}
func (d *Data) Unlock(key string, metrics []string, target int64, now time.Time) bool {
	if _, ok := d.Unlocked[key]; ok {
		return false
	}
	if d.AchievementProgress(metrics) >= target {
		d.Unlocked[key] = now.UTC()
		return true
	}
	return false
}
func (d *Data) Top(group string) []Entry {
	result := make([]Entry, 0, len(d.Rankings[group]))
	for _, e := range d.Rankings[group] {
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].Icon < result[j].Icon
	})
	return result
}

// Store has one disk writer. Only immutable serialized snapshots cross threads.
// The game owns Data; loading a game never reloads or rolls back this profile.
type Store struct {
	Data   Data
	jobs   chan writeJob
	wg     sync.WaitGroup
	mu     sync.Mutex
	err    error
	closed bool
}
type writeJob struct {
	bytes []byte
}

func Open(path string) (*Store, error) {
	d := New()
	raw, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(raw, &d); err != nil {
			return nil, fmt.Errorf("read player profile: %w", err)
		}
		if d.Version != Version {
			return nil, fmt.Errorf("unsupported player profile version %d", d.Version)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if d.Counters == nil {
		d.Counters = map[string]int64{}
	}
	if d.Rankings == nil {
		d.Rankings = map[string]map[string]Entry{}
	}
	if d.Unlocked == nil {
		d.Unlocked = map[string]time.Time{}
	}
	if d.Runs == nil {
		d.Runs = map[string]Run{}
	}
	s := &Store{Data: d, jobs: make(chan writeJob, 1)}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for job := range s.jobs {
			err := storage.WriteFileAtomic(path, job.bytes, 0600)
			s.mu.Lock()
			s.err = err
			s.mu.Unlock()
		}
	}()
	return s, nil
}
func (s *Store) Err() error { s.mu.Lock(); defer s.mu.Unlock(); return s.err }

// Checkpoint coalesces pending snapshots without waiting for filesystem IO.
func (s *Store) Checkpoint() {
	if s == nil || s.closed {
		return
	}
	raw, err := json.Marshal(s.Data)
	if err != nil {
		s.mu.Lock()
		s.err = err
		s.mu.Unlock()
		return
	}
	job := writeJob{bytes: raw}
	select {
	case s.jobs <- job:
		return
	default:
	}
	select {
	case <-s.jobs:
	default:
	}
	s.jobs <- job
}

// Close drains the latest snapshot on normal application shutdown.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	if s.closed {
		return s.Err()
	}
	s.Checkpoint()
	s.closed = true
	close(s.jobs)
	s.wg.Wait()
	return s.Err()
}
