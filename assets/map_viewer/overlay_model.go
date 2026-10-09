package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/world"

	"gopkg.in/yaml.v3"
)

// A document is isolated from the Maps brush and runtime catalogs until Save.
// History includes created switch definitions and placements, not just effects.
type overlayState struct {
	Mechanics   []config.BossMechanic
	RespawnDays int
	Adventure   config.AdventureConfig
	Data        *world.MapData
	NPCs        map[string]*character.NPCData
}
type overlayDocument struct {
	originalNPCs                 map[string]*character.NPCData
	authoredNPCs                 map[string]*character.NPCData // npcs.yaml as written, before load-time backfill
	key                          string
	state                        overlayState
	saved                        overlayState
	undo, redo                   []overlayState
	configBase, npcBase, mapBase []byte
	questsBase                   []byte
	mechanicsBase                []byte
	status                       string
	err                          string
}

func overlayClone[T any](in T) T {
	raw, _ := json.Marshal(in)
	var out T
	_ = json.Unmarshal(raw, &out)
	return out
}
func (s overlayState) clone() overlayState {
	return overlayState{Mechanics: overlayClone(s.Mechanics), RespawnDays: s.RespawnDays, Adventure: overlayClone(s.Adventure), Data: s.Data.Clone(), NPCs: overlayClone(s.NPCs)}
}
func overlaySame(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// Compare authored state directly; drawing must not serialize whole maps.
func (d *overlayDocument) dirty() bool {
	a, b := &d.state, &d.saved
	return a.RespawnDays != b.RespawnDays || !reflect.DeepEqual(a.Mechanics, b.Mechanics) ||
		!reflect.DeepEqual(a.Adventure, b.Adventure) || !reflect.DeepEqual(a.NPCs, b.NPCs) || !overlayMapSame(a.Data, b.Data)
}
func overlayMapSame(a, b *world.MapData) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Width != b.Width || a.Height != b.Height || a.StartX != b.StartX || a.StartY != b.StartY || len(a.Tiles) != len(b.Tiles) {
		return false
	}
	for i := range a.Tiles {
		if !slices.Equal(a.Tiles[i], b.Tiles[i]) {
			return false
		}
	}
	return slices.Equal(a.NPCSpawns, b.NPCSpawns) && slices.Equal(a.MonsterSpawns, b.MonsterSpawns) && slices.Equal(a.SpecialTileSpawns, b.SpecialTileSpawns)
}
func (d *overlayDocument) checkpoint() {
	d.checkpointState(d.state.clone())
}
func (d *overlayDocument) checkpointState(before overlayState) {
	d.undo = append(d.undo, before)
	if len(d.undo) > 60 {
		d.undo = d.undo[1:]
	}
	d.redo = nil
	d.err = ""
	d.status = ""
}
func (d *overlayDocument) history(redo bool) {
	from, to := &d.undo, &d.redo
	if redo {
		from, to = &d.redo, &d.undo
	}
	if len(*from) == 0 {
		return
	}
	*to = append(*to, d.state.clone())
	d.state = (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	d.err = ""
	d.status = ""
}
func overlayID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}
func (d *overlayDocument) uniqueID(prefix string) string {
	for n := 1; ; n++ {
		id := fmt.Sprintf("%s_%d", prefix, n)
		if d.state.Adventure.Effect(id) == nil && d.state.Adventure.Control(id) == nil {
			return id
		}
	}
}
func (d *overlayDocument) addEffect(kind string, box [4]int) int {
	d.checkpoint()
	e := config.EnvironmentEffect{ID: d.uniqueID(kind), Name: map[string]string{"trap": "Pressure plate", "occupation": "Hazardous ground", "transfer": "Transfer", "lane": "Attack lane"}[kind], Kind: kind, Rect: box, School: "earth", Damage: 25}
	switch kind {
	case "trap":
		e.RearmSeconds, e.RearmRounds = 6, 2
	case "lane":
		e.WarningSeconds, e.WarningRounds, e.DurationSeconds, e.DurationRounds = 1.25, 1, 4, 2
	case "transfer":
		e.Damage = 0
		e.School = ""
		e.Destination = [2]int{min(d.state.Data.Width-1, box[2]+1), box[1]}
	}
	d.state.Adventure.Effects = append(d.state.Adventure.Effects, e)
	return len(d.state.Adventure.Effects) - 1
}
func (d *overlayDocument) npc(key string) *character.NPCData {
	if n, ok := d.state.NPCs[key]; ok {
		return n
	}
	if n := d.originalNPCs[key]; n != nil {
		return n
	}
	// Edit what the author wrote: the loader backfills derived fields (trader
	// spell names) that must never be written back into npcs.yaml.
	if d.authoredNPCs == nil {
		var cfg character.NPCConfig
		if err := yaml.Unmarshal(d.npcBase, &cfg); err == nil && cfg.NPCs != nil {
			d.authoredNPCs = cfg.NPCs
		} else {
			d.authoredNPCs = map[string]*character.NPCData{}
		}
	}
	if n, ok := d.authoredNPCs[key]; ok {
		return n
	}
	if character.NPCConfigInstance != nil {
		return character.NPCConfigInstance.NPCs[key]
	}
	return nil
}
func (d *overlayDocument) editNPC(key string) *character.NPCData {
	if n := d.state.NPCs[key]; n != nil {
		return n
	}
	n := overlayClone(d.npc(key))
	if n != nil {
		d.state.NPCs[key] = n
	}
	return n
}
func (d *overlayDocument) controlNPC(id string) (string, *character.NPCData) {
	for _, spawn := range d.state.Data.NPCSpawns {
		n := d.npc(spawn.NPCKey)
		if n == nil || n.Dialogue == nil {
			continue
		}
		found := false
		_ = n.Dialogue.WalkChoices(func(c *character.NPCDialogueChoice) error {
			if c.Action == "adventure_control" && c.Control == id {
				found = true
			}
			return nil
		})
		if found {
			return spawn.NPCKey, n
		}
	}
	return "", nil
}
func (d *overlayDocument) freeTile(x, y int, tm *world.TileManager) bool {
	data := d.state.Data
	if x < 0 || y < 0 || x >= data.Width || y >= data.Height || !tm.IsWalkable(data.Tiles[y][x]) || data.StartX == x && data.StartY == y {
		return false
	}
	for _, n := range data.NPCSpawns {
		if n.X == x && n.Y == y {
			return false
		}
	}
	for _, n := range data.MonsterSpawns {
		if n.X == x && n.Y == y {
			return false
		}
	}
	for _, n := range data.SpecialTileSpawns {
		if n.X == x && n.Y == y {
			return false
		}
	}
	return true
}
func (d *overlayDocument) addControl(x, y int, tm *world.TileManager) (int, error) {
	if !d.freeTile(x, y, tm) {
		return -1, fmt.Errorf("Place a switch on empty walkable ground")
	}
	d.checkpoint()
	id := d.uniqueID("control")
	key := d.key + "_" + id
	for d.npc(key) != nil {
		key += "_new"
	}
	d.state.Adventure.Controls = append(d.state.Adventure.Controls, config.EnvironmentControl{ID: id, Message: "The mechanism is secured."})
	d.state.NPCs[key] = &character.NPCData{EditorOwnerMap: d.key, Name: "Control switch", Type: "encounter", Sprite: "solstice_brace_control", RenderCategory: "scenery", Transparent: true, SizeClass: "small_prop", Biomes: []string{d.key}, Dialogue: &character.NPCDialogue{Greeting: "A service mechanism stands ready.", Choices: []*character.NPCDialogueChoice{{Text: "Operate the mechanism.", Action: "adventure_control", Control: id}, {Text: "Leave.", Action: "leave"}}}}
	d.state.Data.NPCSpawns = append(d.state.Data.NPCSpawns, world.NPCSpawn{X: x, Y: y, NPCKey: key, GroundTile: tm.GetTileKey(d.state.Data.Tiles[y][x])})
	return len(d.state.Adventure.Controls) - 1, nil
}
func (d *overlayDocument) renameID(kind string, index int, id string) error {
	if !overlayID(id) {
		return fmt.Errorf("ID uses lowercase letters, digits and underscores")
	}
	a := &d.state.Adventure
	old := ""
	if kind == "effect" {
		old = a.Effects[index].ID
	} else {
		old = a.Controls[index].ID
	}
	if old == id {
		return nil
	}
	if a.Effect(id) != nil || a.Control(id) != nil {
		return fmt.Errorf("ID already exists")
	}
	if kind == "effect" {
		a.Effects[index].ID = id
		for i := range a.Effects {
			if a.Effects[i].TriggerLane == old {
				a.Effects[i].TriggerLane = id
			}
		}
		for i := range a.Controls {
			for j := range a.Controls[i].Disable {
				if a.Controls[i].Disable[j] == old {
					a.Controls[i].Disable[j] = id
				}
			}
		}
		if a.Boss != nil {
			for i := range a.Boss.Lanes {
				if a.Boss.Lanes[i] == old {
					a.Boss.Lanes[i] = id
				}
			}
			for i := range a.Boss.Phases {
				for j := range a.Boss.Phases[i].Patterns {
					for k := range a.Boss.Phases[i].Patterns[j] {
						if a.Boss.Phases[i].Patterns[j][k] == old {
							a.Boss.Phases[i].Patterns[j][k] = id
						}
					}
				}
			}
		}
	} else {
		a.Controls[index].ID = id
		for i := range a.Effects {
			if a.Effects[i].RequiresControl == old {
				a.Effects[i].RequiresControl = id
			}
		}
	}
	for _, spawn := range d.state.Data.NPCSpawns {
		n := d.npc(spawn.NPCKey)
		if n == nil || n.Dialogue == nil {
			continue
		}
		action := "adventure_control"
		if kind == "effect" {
			action = "disarm_environment"
		}
		if overlayHasAction(n, action, old) {
			_ = d.editNPC(spawn.NPCKey).Dialogue.WalkChoices(func(c *character.NPCDialogueChoice) error {
				if c.Action == action && c.Control == old {
					c.Control = id
				}
				return nil
			})
		}
	}
	if kind == "control" {
		for i := range d.state.Mechanics {
			r := &d.state.Mechanics[i]
			if value, ok := r.Reductions[old]; ok {
				delete(r.Reductions, old)
				r.Reductions[id] = value
			}
		}
	}
	return nil
}

// Removing a referenced entity is blocked rather than silently deleting boss
// phases, changing transfer conditions or altering another interaction.
func (d *overlayDocument) deleteEffect(i int) error {
	id := d.state.Adventure.Effects[i].ID
	copy := d.state.clone()
	copy.Adventure.Effects = slices.Delete(copy.Adventure.Effects, i, i+1)
	// Validate references directly; unrelated incomplete edits must not block removal.
	for _, c := range copy.Adventure.Controls {
		if slices.Contains(c.Disable, id) {
			return fmt.Errorf("Used by control %s; unlink it first", c.ID)
		}
	}
	for _, e := range copy.Adventure.Effects {
		if e.TriggerLane == id {
			return fmt.Errorf("Used by transfer %s; unlink it first", e.ID)
		}
	}
	if b := copy.Adventure.Boss; b != nil {
		if slices.Contains(b.Lanes, id) {
			return fmt.Errorf("Used by boss; unlink it first")
		}
		for _, p := range b.Phases {
			for _, pattern := range p.Patterns {
				if slices.Contains(pattern, id) {
					return fmt.Errorf("Used by a boss phase; unlink it first")
				}
			}
		}
	}
	for _, spawn := range d.state.Data.NPCSpawns {
		if n := d.npc(spawn.NPCKey); overlayHasAction(n, "disarm_environment", id) {
			return fmt.Errorf("Used by %s; remove its disarm action first", n.Name)
		}
	}
	d.checkpoint()
	d.state = copy
	return nil
}
func (d *overlayDocument) deleteControl(i int) error {
	id := d.state.Adventure.Controls[i].ID
	for _, r := range d.state.Mechanics {
		if _, ok := r.Reductions[id]; ok {
			return fmt.Errorf("Used by boss mechanic %s; unlink it first", r.ID)
		}
	}
	for _, e := range d.state.Adventure.Effects {
		if e.RequiresControl == id {
			return fmt.Errorf("Required by %s; unlink it first", e.Name)
		}
	}
	d.checkpoint()
	d.state.Adventure.Controls = slices.Delete(d.state.Adventure.Controls, i, i+1)
	for _, spawn := range d.state.Data.NPCSpawns {
		n := d.npc(spawn.NPCKey)
		if n == nil || n.Dialogue == nil {
			continue
		}
		if overlayHasAction(n, "adventure_control", id) {
			n = d.editNPC(spawn.NPCKey)
			n.Dialogue.Choices = overlayRemoveActions(n.Dialogue.Choices, "adventure_control", id)
		}
	}
	return nil
}
func (v *viewer) validateOverlay(d *overlayDocument) error {
	if d.state.Data == nil {
		return fmt.Errorf("Map is unavailable")
	}
	configs := config.MapConfigs{Maps: map[string]config.MapConfig{}}
	for _, m := range v.maps {
		if m.Config != nil {
			configs.Maps[m.Key] = *m.Config
		}
	}
	mc := configs.Maps[d.key]
	mc.Adventure = &d.state.Adventure
	mc.RespawnDays = d.state.RespawnDays
	configs.Maps[d.key] = mc
	if err := configs.ValidateAdventures(); err != nil {
		return err
	}
	if d.state.RespawnDays > 0 && v.overlayInOpenWorld(d.key) {
		return fmt.Errorf("Maps merged into the open world cannot respawn; set Respawn every (days) to 0")
	}
	if s := d.state.Adventure.Schedule; s != nil {
		if !slices.Contains(v.overlayCatalog("quests", "quests"), s.RequiresQuest) {
			return fmt.Errorf("Entrance schedule: unknown required quest %q", s.RequiresQuest)
		}
		if !slices.Contains(v.overlayCatalog("spells", "spells"), s.RequiresSpell) {
			return fmt.Errorf("Entrance schedule: unknown required spell %q", s.RequiresSpell)
		}
	}
	// A clear reward binds monsters placed on this map; a missing one loses it.
	// A changed boss carries its reward along on save.
	oldBoss, newBoss := d.bossRename()
	for _, enc := range mc.ClearEncounters {
		for _, group := range enc.Monsters {
			want := group.Type
			if oldBoss != "" && want == oldBoss {
				want = newBoss
			}
			if !slices.ContainsFunc(d.state.Data.MonsterSpawns, func(s world.MonsterSpawn) bool { return s.MonsterKey == want }) {
				return fmt.Errorf("Clear reward in map_configs.yaml expects %s on this map, but none is placed", want)
			}
		}
	}
	a := &d.state.Adventure
	data := d.state.Data
	inside := func(x, y int) bool { return x >= 0 && y >= 0 && x < data.Width && y < data.Height }
	walkable := func(x, y int) bool { return inside(x, y) && v.tileManager.IsWalkable(data.Tiles[y][x]) }
	for _, e := range a.Effects {
		if !inside(e.Rect[0], e.Rect[1]) || !inside(e.Rect[2], e.Rect[3]) {
			return fmt.Errorf("%s extends outside the map", e.Name)
		}
		if e.SustainDamage < 0 || e.WarningRounds < 0 || e.DurationRounds < 0 || e.RearmRounds < 0 {
			return fmt.Errorf("%s: values cannot be negative", e.Name)
		}
		if e.Kind == "transfer" {
			x, y := e.Destination[0], e.Destination[1]
			if !walkable(x, y) {
				return fmt.Errorf("%s: receiver must be walkable", e.Name)
			}
			for _, other := range a.Effects {
				if other.Contains(x, y) && other.Kind != "lane" {
					return fmt.Errorf("%s: receiver overlaps %s", e.Name, other.Name)
				}
			}
		}
	}
	for _, c := range a.Controls {
		for _, t := range c.Tiles {
			if !inside(t.X, t.Y) || !v.tileManager.HasTileKey(t.Tile) {
				return fmt.Errorf("%s has an invalid tile change", c.ID)
			}
		}
	}
	for _, j := range a.JumpLinks {
		for _, m := range data.MonsterSpawns {
			if m.X == j[0] && m.Y == j[1] || m.X == j[2] && m.Y == j[3] {
				return fmt.Errorf("Monster on Jump endpoint")
			}
		}
		if !walkable(j[0], j[1]) || !walkable(j[2], j[3]) {
			return fmt.Errorf("Jump link endpoints must be walkable")
		}
	}
	if b := a.Boss; b != nil {
		if v.monsterCfg == nil || !v.monsterCfg.Monsters[b.Monster].Boss {
			return fmt.Errorf("Select a boss monster")
		}
		count := 0
		for _, m := range data.MonsterSpawns {
			if m.MonsterKey == b.Monster {
				count++
				if r := b.Arena; r != nil && (m.X < r[0] || m.X > r[2] || m.Y < r[1] || m.Y > r[3]) {
					return fmt.Errorf("Boss spawn is outside its arena")
				}
			}
		}
		if count != 1 {
			return fmt.Errorf("Place exactly one %s using Place or move boss", b.Monster)
		}
		if r := b.Arena; r != nil && (!inside(r[0], r[1]) || !inside(r[2], r[3])) {
			return fmt.Errorf("Boss arena is outside the map")
		}
	}
	if err := v.validateOverlayObjects(d); err != nil {
		return err
	}
	refs := map[string]*config.MapConfig{}
	for key, m := range configs.Maps {
		copy := m
		refs[key] = &copy
	}
	rules := config.BossMechanicsConfig{Maps: map[string][]config.BossMechanic{d.key: d.state.Mechanics}}
	if err := rules.Validate(refs); err != nil {
		return err
	}
	if b := a.Boss; b != nil && b.HealPercent > 0 {
		if b.HealPercent > 100 || b.HealCapPercent <= 0 {
			return fmt.Errorf("Regeneration needs a rate of 1..100 and positive total healing budget")
		}
		if !slices.ContainsFunc(a.Controls, func(c config.EnvironmentControl) bool { return c.StopHealing }) {
			return fmt.Errorf("Regeneration needs at least one linked switch")
		}
	}

	if strings.ContainsAny(d.key, "/\\") {
		return fmt.Errorf("Invalid map key")
	}
	return nil
}

func (d *overlayDocument) changeKind(index int, kind string) error {
	e := &d.state.Adventure.Effects[index]
	if e.Kind == kind {
		return nil
	}
	if !slices.Contains([]string{"trap", "occupation", "transfer", "lane"}, kind) {
		return fmt.Errorf("Unknown behavior")
	}
	// Reject dangling lane/disarm references before converting the entity.
	if e.Kind == "lane" || e.Kind == "trap" {
		probe := &overlayDocument{state: d.state.clone(), originalNPCs: d.originalNPCs}
		if err := probe.deleteEffect(index); err != nil {
			return err
		}
	}
	e.Kind = kind
	if kind != "transfer" {
		e.TriggerLane = ""
		if e.Damage <= 0 {
			e.Damage = 25
		}
		if e.School == "" {
			e.School = "earth"
		}
	}
	switch kind {
	case "trap":
		e.RearmSeconds, e.RearmRounds = 6, 2
	case "lane":
		e.WarningSeconds, e.WarningRounds, e.DurationSeconds, e.DurationRounds = 1.25, 1, 4, 2
	case "transfer":
		e.Damage, e.SustainDamage = 0, 0
		e.School, e.SecondarySchool = "", ""
		e.Destination = [2]int{min(d.state.Data.Width-1, e.Rect[2]+1), e.Rect[1]}
	}
	return nil
}

// overlayInOpenWorld reports a map stitched into the unified open world.
func (v *viewer) overlayInOpenWorld(key string) bool {
	if v.owc == nil {
		return false
	}
	_, merged := v.owc.Placements[key]
	return merged
}

// bossRename reports a boss archetype changed since the last save.
func (d *overlayDocument) bossRename() (old, next string) {
	a, b := d.saved.Adventure.Boss, d.state.Adventure.Boss
	if a == nil || b == nil || a.Monster == b.Monster {
		return "", ""
	}
	return a.Monster, b.Monster
}
