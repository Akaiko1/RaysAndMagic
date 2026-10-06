package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/storage"
	"ugataima/internal/world"

	"gopkg.in/yaml.v3"
)

const overlayConfigPath = "assets/map_configs.yaml"
const overlayNPCPath = "assets/npcs.yaml"

// Locate one mapping entry in the original text. Saving an overlay preserves
// all bytes outside the edited entries, including other maps and their comments.
func overlayEntry(data []byte, path []string) (start, end, indent int, found bool, err error) {
	var root yaml.Node
	if err = yaml.Unmarshal(data, &root); err != nil {
		return
	}
	if len(root.Content) == 0 {
		err = fmt.Errorf("Empty YAML document")
		return
	}
	lines := strings.SplitAfter(string(data), "\n")
	offsets := make([]int, len(lines)+1)
	for i, line := range lines {
		offsets[i+1] = offsets[i] + len(line)
	}
	node := root.Content[0]
	parentEnd := len(data)
	level := 0
	for depth, key := range path {
		if node.Kind != yaml.MappingNode || node.Style&yaml.FlowStyle != 0 {
			err = fmt.Errorf("%s is not a mapping", strings.Join(path[:depth], "."))
			return
		}
		var k, value *yaml.Node
		nextLine := len(lines) + 1
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Value == key {
				k, value = node.Content[i], node.Content[i+1]
				if i+2 < len(node.Content) {
					nextLine = node.Content[i+2].Line
				}
				break
			}
		}
		if k == nil {
			if depth != len(path)-1 {
				err = fmt.Errorf("Missing %s", key)
				return
			}
			return parentEnd, parentEnd, level, false, nil
		}
		start = offsets[k.Line-1]
		indent = k.Column - 1
		end = parentEnd
		if nextLine <= len(lines) {
			end = min(end, offsets[nextLine-1])
		}
		// AST siblings delimit the entry. Indentation alone cannot distinguish
		// a sibling from YAML's valid indentless sequence values.
		for i := min(nextLine-2, len(lines)-1); i >= k.Line; i-- {
			if offsets[i] >= end {
				continue
			}
			line := strings.TrimRight(lines[i], "\r\n")
			trim := strings.TrimSpace(line)
			spaces := len(line) - len(strings.TrimLeft(line, " "))
			if trim != "" && !(strings.HasPrefix(trim, "#") && spaces <= indent) {
				break
			}
			end = offsets[i]
		}
		if depth == len(path)-1 {
			return start, end, indent, true, nil
		}
		node = value
		parentEnd = end
		level = indent + 2
	}
	err = fmt.Errorf("Empty YAML path")
	return
}
func overlayPatch(data []byte, path []string, value any) ([]byte, error) {
	start, end, indent, _, err := overlayEntry(data, path)
	if err != nil {
		return nil, err
	}
	node := &yaml.Node{Kind: yaml.MappingNode}
	val := &yaml.Node{}
	if err = val.Encode(value); err != nil {
		return nil, err
	}
	node.Content = []*yaml.Node{{Kind: yaml.ScalarNode, Value: path[len(path)-1]}, val}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err = enc.Encode(node); err != nil {
		return nil, err
	}
	_ = enc.Close()
	eol := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		eol = "\r\n"
	}
	var out bytes.Buffer
	out.Write(data[:start])
	if start > 0 && data[start-1] != '\n' {
		out.WriteString(eol)
	}
	for _, line := range strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n") {
		out.WriteString(strings.Repeat(" ", indent))
		out.WriteString(line)
		out.WriteString(eol)
	}
	out.Write(data[end:])
	// Validate the actual splice before any caller can publish it.
	var checked any
	if err := yaml.Unmarshal(out.Bytes(), &checked); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
func overlayEntryUnchanged(before, now []byte, path []string) bool {
	a, b, _, ok, e := overlayEntry(before, path)
	if e != nil {
		return false
	}
	x, y, _, other, f := overlayEntry(now, path)
	return f == nil && ok == other && bytes.Equal(before[a:b], now[x:y])
}
func (v *viewer) overlayDoc() *overlayDocument {
	if len(v.maps) == 0 {
		return nil
	}
	if v.overlay.documents == nil {
		v.overlay.documents = map[string]*overlayDocument{}
	}
	m := v.maps[v.mapIndex]
	if d := v.overlay.documents[m.Key]; d != nil {
		return d
	}
	d := &overlayDocument{key: m.Key, state: overlayState{Data: m.Data.Clone(), NPCs: map[string]*character.NPCData{}}}
	if m.Config != nil {
		d.state.RespawnDays = m.Config.RespawnDays
	}
	rules, rulesErr := config.ReadBossMechanics(config.BossMechanicsPath)
	if rulesErr != nil {
		d.err = rulesErr.Error()
	} else {
		d.state.Mechanics = overlayClone(rules.Maps[m.Key])
	}
	d.mechanicsBase, _ = os.ReadFile(config.BossMechanicsPath)
	if m.Config != nil && m.Config.Adventure != nil {
		d.state.Adventure = overlayClone(*m.Config.Adventure)
	}
	d.originalNPCs = map[string]*character.NPCData{}
	if d.state.Data != nil {
		for _, spawn := range d.state.Data.NPCSpawns {
			if n := d.npc(spawn.NPCKey); n != nil {
				d.originalNPCs[spawn.NPCKey] = overlayClone(n)
			}
		}
	}
	var err error
	d.configBase, err = os.ReadFile(overlayConfigPath)
	if err != nil {
		d.err = err.Error()
	}
	d.npcBase, _ = os.ReadFile(overlayNPCPath)
	if m.Config != nil {
		d.mapBase, _ = os.ReadFile(filepath.Join("assets", m.Config.File))
	}
	d.saved = d.state.clone()
	v.overlay.documents[m.Key] = d
	return d
}

type overlayWrite struct {
	path          string
	before, after []byte
}

func overlayPublish(files []overlayWrite, write func(string, []byte, os.FileMode) error) error {
	for _, f := range files {
		now, err := os.ReadFile(f.path)
		if err != nil && !(os.IsNotExist(err) && f.before == nil) {
			return err
		}
		if !bytes.Equal(now, f.before) {
			return fmt.Errorf("%s changed on disk; reload before saving", f.path)
		}
	}
	for i, f := range files {
		if err := write(f.path, f.after, 0644); err != nil {
			var rollback []string
			for j := i - 1; j >= 0; j-- {
				restore := func() error {
					if files[j].before == nil {
						return os.Remove(files[j].path)
					}
					return write(files[j].path, files[j].before, 0644)
				}
				if e := restore(); e != nil {
					rollback = append(rollback, files[j].path+": "+e.Error())
				}
			}
			if len(rollback) > 0 {
				return fmt.Errorf("Save failed: %v; restoring earlier files failed: %s", err, strings.Join(rollback, "; "))
			}
			return fmt.Errorf("Save failed; earlier writes restored: %w", err)
		}
	}
	return nil
}
func (v *viewer) saveOverlay(d *overlayDocument) error {
	if err := v.validateOverlay(d); err != nil {
		return err
	}
	m := &v.maps[v.mapIndex]
	if m.Key != d.key {
		return fmt.Errorf("Select the document's map before saving")
	}
	// The Maps tab can independently edit the same geometry. Never overwrite it.
	if !overlaySame(m.Data, d.saved.Data) {
		return fmt.Errorf("Map geometry changed in Maps; reload this overlay first")
	}
	mapPath := filepath.Join("assets", m.Config.File)
	mapNow, err := os.ReadFile(mapPath)
	if err != nil {
		return err
	}
	if !bytes.Equal(mapNow, d.mapBase) {
		return fmt.Errorf("Map changed on disk; reload before saving")
	}
	current, err := os.ReadFile(overlayConfigPath)
	if err != nil {
		return err
	}
	path := []string{"maps", d.key, "adventure"}
	if !overlayEntryUnchanged(d.configBase, current, path) {
		return fmt.Errorf("This overlay changed on disk; reload before saving")
	}
	next, err := overlayPatch(current, path, d.state.Adventure)
	if err != nil {
		return err
	}
	if d.state.RespawnDays != d.saved.RespawnDays {
		rp := []string{"maps", d.key, "respawn_days"}
		if !overlayEntryUnchanged(d.configBase, current, rp) {
			return fmt.Errorf("Map lifetime changed on disk; reload before saving")
		}
		next, err = overlayPatch(next, rp, d.state.RespawnDays)
		if err != nil {
			return err
		}
	}
	// Validate the actual merged document, including references from other maps.
	var cfg config.MapConfigs
	if err = yaml.Unmarshal(next, &cfg); err != nil {
		return err
	}
	if err = cfg.ValidateAdventures(); err != nil {
		return err
	}
	files := []overlayWrite{{overlayConfigPath, current, next}}
	var mechanicsNext []byte
	if !overlaySame(d.state.Mechanics, d.saved.Mechanics) {
		w, e := overlayMechanicsWrite(d, &cfg)
		if e != nil {
			return e
		}
		files = append(files, w)
		mechanicsNext = w.after
	}
	npcNow, npcNext := []byte(nil), []byte(nil)
	npcChanges := map[string]*character.NPCData{}
	for key := range d.state.NPCs {
		npcChanges[key] = d.state.NPCs[key]
	}
	for key := range d.saved.NPCs {
		if _, ok := npcChanges[key]; !ok {
			npcChanges[key] = d.originalNPCs[key]
		}
	}
	for key, next := range npcChanges {
		previous, savedOverride := d.saved.NPCs[key]
		if !savedOverride {
			previous = d.originalNPCs[key]
		}
		if overlaySame(previous, next) {
			delete(npcChanges, key)
		}
	}
	if len(npcChanges) > 0 {
		npcNow, err = os.ReadFile(overlayNPCPath)
		if err != nil {
			return err
		}
		npcNext = npcNow
		keys := make([]string, 0, len(npcChanges))
		for key := range npcChanges {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			p := []string{"npcs", key}
			if !overlayEntryUnchanged(d.npcBase, npcNow, p) {
				return fmt.Errorf("Switch %s changed on disk; reload before saving", key)
			}
			if npcChanges[key] == nil {
				var start, end int
				start, end, _, _, err = overlayEntry(npcNext, p)
				if err == nil {
					npcNext = append(append([]byte(nil), npcNext[:start]...), npcNext[end:]...)
				}
			} else {
				npcNext, err = overlayPatch(npcNext, p, npcChanges[key])
			}
			if err != nil {
				return err
			}
		}
		var merged character.NPCConfig
		if err = yaml.Unmarshal(npcNext, &merged); err != nil {
			return fmt.Errorf("Merged NPC catalog: %w", err)
		}
		if err = game.ValidateEditorNPCs(merged.NPCs, v.cfg.Graphics.SizeClasses); err != nil {
			return fmt.Errorf("Merged NPC catalog: %w", err)
		}
		files = append(files, overlayWrite{overlayNPCPath, npcNow, npcNext})
	}
	var mapNext []byte
	if !overlaySame(d.state.Data, d.saved.Data) {

		draft := *m
		draft.Data = d.state.Data.Clone()
		// Encode NPC placeholders without letting letterless ground overwrite them.
		// Placement-specific ground travels in the NPC definition token.
		for _, n := range draft.Data.NPCSpawns {
			draft.Data.Tiles[n.Y][n.X] = world.TileEmpty
		}
		lines, encodeErr := encodeMapLines(&draft, v.tileManager)
		if encodeErr != nil {
			return encodeErr
		}
		eol := m.EOL
		if eol == "" {
			eol = "\n"
		}
		mapNext = []byte(strings.Join(append(slicesCopy(m.Header), lines...), eol) + eol)
		files = append(files, overlayWrite{mapPath, mapNow, mapNext})
	}
	if err = overlayPublish(files, storage.WriteFileAtomic); err != nil {
		return err
	}
	d.configBase = next
	if npcNext != nil {
		d.npcBase = npcNext
	}
	if mapNext != nil {
		d.mapBase = mapNext
	}
	if mechanicsNext != nil {
		d.mechanicsBase = mechanicsNext
		var rules config.BossMechanicsConfig
		if err := yaml.Unmarshal(mechanicsNext, &rules); err == nil {
			config.GlobalBossMechanics = &rules
		}
	}
	m.Config.RespawnDays = d.state.RespawnDays
	m.Config.Adventure = overlayClone(&d.state.Adventure)
	m.Data = d.state.Data.Clone()
	if character.NPCConfigInstance != nil {
		for key, n := range npcChanges {
			if n == nil {
				delete(character.NPCConfigInstance.NPCs, key)
			} else {
				character.NPCConfigInstance.NPCs[key] = overlayClone(n)
			}
		}
	}
	rebuildMapFloors(m, v.tileManager)
	d.state.Data = m.Data.Clone()
	d.saved = d.state.clone()
	d.status = "Saved scene, objects and boss mechanics."
	d.err = ""
	v.refreshLegend()
	return nil
}
func slicesCopy[T any](s []T) []T { return append([]T(nil), s...) }
