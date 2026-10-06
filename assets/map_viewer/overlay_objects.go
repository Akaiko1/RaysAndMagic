package main

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func overlayInfo(label, value, help string) overlayField {
	return overlayField{label: label, value: value, help: help}
}
func (v *viewer) overlayChoose(title, help string, choices []string, apply func(string) error) {
	v.overlay.modal = &overlayModal{title: title, help: help, choices: choices, apply: apply}
}
func (v *viewer) overlayCatalog(file, root string) []string {
	if v.overlay.catalogs == nil {
		v.overlay.catalogs = map[string][]string{}
	}
	if keys, ok := v.overlay.catalogs[file]; ok {
		return keys
	}
	var doc map[string]map[string]any
	raw, _ := os.ReadFile("assets/" + file + ".yaml")
	_ = yaml.Unmarshal(raw, &doc)
	var keys []string
	for key := range doc[root] {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	v.overlay.catalogs[file] = keys
	return keys
}
func (v *viewer) overlayBossPicker(d *overlayDocument) {
	var choices []string
	for key, def := range v.monsterCfg.Monsters {
		if def.Boss {
			choices = append(choices, key)
		}
	}
	sort.Strings(choices)
	v.overlayChoose("Choose a boss", "Select an archetype, then click its position. The map keeps one boss policy.", choices, func(key string) error {
		d.checkpoint()
		if d.state.Adventure.Boss == nil {
			d.state.Adventure.Boss = &config.AdventureBoss{Monster: key, EveryActions: 2}
		}
		old := d.state.Adventure.Boss.Monster
		d.state.Adventure.Boss.Monster = key
		for i := range d.state.Data.MonsterSpawns {
			if d.state.Data.MonsterSpawns[i].MonsterKey == old {
				d.state.Data.MonsterSpawns[i].MonsterKey = key
			}
		}
		v.overlaySelect("boss", 0)
		count := 0
		for _, s := range d.state.Data.MonsterSpawns {
			if s.MonsterKey == key {
				count++
			}
		}
		if count == 0 {
			v.overlay.tool = "boss-place"
		}
		return nil
	})
}
func (v *viewer) overlayPlaceBoss(d *overlayDocument, tile [2]int) error {
	b := d.state.Adventure.Boss
	if b == nil {
		return fmt.Errorf("Choose a boss first")
	}
	for _, s := range d.state.Data.MonsterSpawns {
		if s.MonsterKey == b.Monster && s.X == tile[0] && s.Y == tile[1] {
			v.overlay.tool = ""
			return nil
		}
	}
	if !d.freeTile(tile[0], tile[1], v.tileManager) {
		return fmt.Errorf("Place the boss on empty walkable ground")
	}
	d.checkpoint()
	for i, s := range d.state.Data.MonsterSpawns {
		if s.MonsterKey == b.Monster {
			v.overlayMovePlacement(d, true, i, tile)
			v.overlay.tool = ""
			return nil
		}
	}
	d.state.Data.MonsterSpawns = append(d.state.Data.MonsterSpawns, world.MonsterSpawn{X: tile[0], Y: tile[1], MonsterKey: b.Monster})
	v.overlay.tool = ""
	return nil
}

// A move carries the placement's own ground, as a drag in the Maps tab does: the
// vacated cell loses that stamp and the destination receives it.
func (v *viewer) overlayMovePlacement(d *overlayDocument, monster bool, i int, tile [2]int) {
	data := d.state.Data
	floor, _ := v.tileManager.GetTileTypeFromLetterForBiome(floorLetter, v.currentBiome())
	if monster {
		data.ClearMonsterGround(v.tileManager, data.MonsterSpawns[i], floor)
		data.MonsterSpawns[i].X, data.MonsterSpawns[i].Y = tile[0], tile[1]
	} else {
		data.ClearNPCGround(v.tileManager, data.NPCSpawns[i], floor)
		data.NPCSpawns[i].X, data.NPCSpawns[i].Y = tile[0], tile[1]
	}
	data.Tiles[tile[1]][tile[0]] = floor
	data.RebuildFloors(v.tileManager, v.currentBiome())
}
func (v *viewer) overlayObjectPicker(d *overlayDocument) {
	v.overlayChoose("Add interactive object", "Create an editable local object or place an existing game object with its behavior.", []string{"New interactive object", "Existing NPC or object", "Special tile"}, func(kind string) error {
		// The next picker opens on the next Update, after this modal is dismissed.
		v.overlay.pendingPicker = kind
		return nil
	})
}
func (v *viewer) overlayContinuePicker(d *overlayDocument) {
	kind := v.overlay.pendingPicker
	v.overlay.pendingPicker = ""
	switch kind {
	case "New interactive object":
		v.overlay.pendingNPC = ""
		v.overlay.tool = "object-new"
	case "Existing NPC or object":
		keys := []string{}
		for key, n := range character.NPCConfigInstance.NPCs {
			if n != nil {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		v.overlayChoose("Object library", "Places the existing definition. Use Make local copy before changing a shared object's behavior.", keys, func(key string) error { v.overlay.pendingNPC = key; v.overlay.tool = "object-new"; return nil })
	case "Special tile":
		keys := []string{}
		for key, t := range v.tileDataByKey {
			if t.Type == "teleporter" {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		v.overlayChoose("Special tile", "Existing teleporter behavior. Use + Trap for working pressure traps.", keys, func(key string) error { v.overlay.pendingNPC = key; v.overlay.tool = "special-new"; return nil })
	}
}
func (d *overlayDocument) newObjectKey() string {
	for n := 1; ; n++ {
		key := fmt.Sprintf("%s_object_%d", d.key, n)
		if d.npc(key) == nil {
			return key
		}
	}
}
func (v *viewer) overlayPlaceObject(d *overlayDocument, tile [2]int) error {
	if !d.freeTile(tile[0], tile[1], v.tileManager) {
		return fmt.Errorf("Place the object on empty walkable ground")
	}
	key := v.overlay.pendingNPC
	d.checkpoint()
	if key == "" {
		key = d.newObjectKey()
		d.state.NPCs[key] = &character.NPCData{EditorOwnerMap: d.key, Name: "Interactive object", Type: "encounter", Sprite: "solstice_brace_control", RenderCategory: "scenery", SizeClass: "small_prop", Transparent: true, Biomes: []string{v.currentBiome()}, Dialogue: &character.NPCDialogue{Greeting: "A curious mechanism waits in silence.", Choices: []*character.NPCDialogueChoice{{Text: "Examine it.", Action: "info", Response: "Its purpose is not yet clear."}}}}
	} else if d.originalNPCs[key] == nil {
		d.originalNPCs[key] = overlayClone(d.npc(key))
	}
	d.state.Data.NPCSpawns = append(d.state.Data.NPCSpawns, world.NPCSpawn{X: tile[0], Y: tile[1], NPCKey: key, GroundTile: v.tileManager.GetTileKey(d.state.Data.Tiles[tile[1]][tile[0]])})
	v.overlaySelect("object", len(d.state.Data.NPCSpawns)-1)
	return nil
}
func (v *viewer) overlayObjectFields(d *overlayDocument) []overlayField {
	p := &v.overlay
	if p.selected < 0 || p.selected >= len(d.state.Data.NPCSpawns) {
		return nil
	}
	spawn := d.state.Data.NPCSpawns[p.selected]
	key := spawn.NPCKey
	n := overlayClone(d.npc(key))
	if n == nil {
		return nil
	}
	fields := []overlayField{
		overlayString("Name", &n.Name, "Name shown during interaction."),
		overlayInfo("Position", fmt.Sprintf("Column %d, row %d", spawn.X, spawn.Y), "Click Move object to place it elsewhere."),
		overlayAction("Move object", "Click empty walkable ground.", func() { p.tool = "object-move" }),
	}
	if n.Dialogue == nil {
		n.Dialogue = &character.NPCDialogue{}
	}
	fields = append(fields, overlayHeading("Dialogue"), overlayString("Greeting", &n.Dialogue.Greeting, "Text shown when the object is examined."), overlayAction("+ Add dialogue action", "Choose what an interaction does. Each action exposes its own targets and parameters.", func() { v.overlayAddChoice(d, nil) }))
	for i, c := range n.Dialogue.Choices {
		if c == nil {
			continue
		}
		path := []int{i}
		f := overlayNavigate(c.Text, "Action: "+c.Action, "Edit this dialogue action and its parameters.", func() { v.overlaySelect("choice", p.selected); p.choicePath = path })
		f.depth = 1
		fields = append(fields, f)
	}
	fields = append(fields, overlayHeading("Identity and appearance"), overlayInfo("Object key", key, "This definition may be shared by several placements."), overlayInfo("Interaction type", n.Type, "The object library includes merchants, doors and encounters."), overlayAction("Make local copy", "Copies this object's definition for this placement. Catalog-backed crates must keep their original key.", func() {
		if n.Type == character.NPCTypeLootCrate {
			d.err = "Loot crates use their catalog key. Choose another crate from the library."
			return
		}
		d.checkpoint()
		newKey := d.newObjectKey()
		d.state.NPCs[newKey] = overlayClone(n)
		d.state.NPCs[newKey].EditorOwnerMap = d.key
		if e := d.state.NPCs[newKey].Encounter; e != nil && e.QuestID != "" {
			e.QuestID = newKey + "_encounter"
		}
		d.state.Data.NPCSpawns[p.selected].NPCKey = newKey
		v.overlayRemoveUnusedLocalNPC(d, key)
	}))
	sprites := []string{}
	for _, def := range character.NPCConfigInstance.NPCs {
		if def != nil && def.Sprite != "" && !slices.Contains(sprites, def.Sprite) {
			sprites = append(sprites, def.Sprite)
		}
	}
	sort.Strings(sprites)
	fields = append(fields, overlayString("Sprite", &n.Sprite, "Choose an existing object or NPC sprite.", sprites...), overlayString("Prompt verb", &n.PromptVerb, "Examples: examine, use, enter. Empty uses the object's default."), overlayHeading("Availability"), overlayInt("Minimum party level", &n.MinPartyLevel, "0 means no level requirement."), overlayBool("Night only", &n.NightOnly, "Visible and available only at night."), overlayBool("Hide after visit", &n.HideWhenVisited, "Use only when the object's action records a visited state."), overlayBool("Fixed facing", &n.NoSpin, "Keep the object from rotating with the party."))
	if n.Type == character.NPCTypeDoor {
		fields = append(fields, overlayString("Door behavior", &n.DoorBehavior, "Locked doors expose key and stat unlock options in the game.", "locked", "champion_portcullis"), overlayString("Lock label", &n.LockLabel, "Description of the lock."), overlayList("Accepted keys", &n.DoorKeyItemKeys, "Any listed item can unlock the door.", v.overlayCatalog("items", "items")))
	}
	if n.Lectern != nil {
		fields = append(fields, overlayString("Teaching spell", &n.Lectern.Spell, "Spell taught by this book.", append([]string{""}, v.overlayCatalog("spells", "spells")...)...), overlayList("Random spell pool", &n.Lectern.Pool, "Used instead of a fixed spell.", v.overlayCatalog("spells", "spells")))
	}

	if n.GridSpanTiles == 0 && n.Type != character.NPCTypeDoor {
		f := overlayString("Presentation", &n.RenderCategory, "Wall mounted objects attach to an adjacent wall; scenery rests on the floor.", "npc", "wall_mounted", "scenery", "landmark")
		apply := f.apply
		f.apply = func(s string) error {
			if err := apply(s); err != nil {
				return err
			}
			if s == "npc" {
				n.SizeClass = "person"
			} else if n.SizeClass == "person" {
				n.SizeClass = "full_tile"
			}
			return nil
		}
		fields = append(fields, f)
	}
	sizes := []string{}
	for size := range v.cfg.Graphics.SizeClasses {
		if config.IsPropSizeClass(size) || size == "person" {
			sizes = append(sizes, size)
		}
	}
	sort.Strings(sizes)
	fields = append(fields, overlayString("Size class", &n.SizeClass, "full_tile is exactly one tile high.", sizes...))
	fields = append(fields, overlayHeading("Combat encounter"))
	fields = append(fields, v.overlayEncounterFields(d, n)...)
	// Field reads stay pure: commit the private copy only after a successful edit.
	for i := range fields {
		if apply := fields[i].apply; apply != nil {
			fields[i].apply = func(s string) error {
				if err := apply(s); err != nil {
					return err
				}
				d.state.NPCs[key] = n
				return nil
			}
		}
	}
	return fields
}
func (v *viewer) overlayEncounterFields(d *overlayDocument, n *character.NPCData) []overlayField {
	key := d.state.Data.NPCSpawns[v.overlay.selected].NPCKey
	if n.Encounter == nil {
		return []overlayField{overlayAction("Add combat encounter", "Configure monsters and rewards, then add a combat action.", func() {
			d.checkpoint()
			d.editNPC(key).Encounter = &character.NPCEncounter{Type: "combat", QuestID: key + "_encounter", FirstVisitOnly: true, Rewards: &monster.EncounterRewards{}}
		})}
	}
	e := n.Encounter
	out := []overlayField{overlayBool("Encounter once", &e.FirstVisitOnly, "Prevents repeating the authored encounter after a win."), overlayString("Encounter quest ID", &e.QuestID, "Unique save identity for this encounter."), overlayString("Encounter title", &e.QuestName, "Name in the quest log."), overlayString("Encounter description", &e.QuestDescription, "Objective text in the quest log."), overlayString("Combat announcement", &e.StartMessage, "Message when the encounter begins.")}
	keys := []string{}
	for k, m := range v.monsterCfg.Monsters {
		if m.Disposition != "fish" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for i, m := range e.Monsters {
		if m == nil {
			continue
		}
		out = append(out, overlayString(fmt.Sprintf("Group %d monster", i+1), &m.Type, "Archetype to spawn.", keys...), overlayInt(fmt.Sprintf("Group %d minimum", i+1), &m.CountMin, "Minimum count."), overlayInt(fmt.Sprintf("Group %d maximum", i+1), &m.CountMax, "Maximum count."), overlayAction(fmt.Sprintf("Remove group %d", i+1), "Remove this encounter group.", func() { d.checkpoint(); x := d.editNPC(key).Encounter; x.Monsters = slices.Delete(x.Monsters, i, i+1) }))
	}
	out = append(out, overlayAction("Add monster group", "Adds one monster; select its archetype and count.", func() {
		d.checkpoint()
		x := d.editNPC(key).Encounter
		x.Monsters = append(x.Monsters, &character.EncounterMonster{Type: keys[0], CountMin: 1, CountMax: 1})
	}))
	if e.Rewards == nil {
		e.Rewards = &monster.EncounterRewards{}
	}
	out = append(out, overlayInt("Reward gold", &e.Rewards.Gold, "Gold awarded on victory."), overlayInt("Reward experience", &e.Rewards.Experience, "Experience awarded on victory."), overlayString("Victory message", &e.Rewards.CompletionMessage, "Message after every encounter monster is defeated."))
	return out
}
func (v *viewer) overlayAddChoice(d *overlayDocument, parent []int) {
	index := v.overlay.selected
	v.overlayChoose("Choose an action", "Actions are taken from the game's actual dialogue dispatcher. Select one to configure its target.", game.EditorDialogueActions(), func(action string) error {
		d.checkpoint()
		n := d.editNPC(d.state.Data.NPCSpawns[index].NPCKey)
		if n.Dialogue == nil {
			n.Dialogue = &character.NPCDialogue{}
		}
		list := &n.Dialogue.Choices
		for _, i := range parent {
			if i < 0 || i >= len(*list) {
				return fmt.Errorf("Dialogue branch no longer exists")
			}
			list = &(*list)[i].Choices
		}
		c := &character.NPCDialogueChoice{Action: action, Text: strings.ReplaceAll(action, "_", " ") + "."}
		switch action {
		case "tavern_rest", "wait_until_night", "wait_until_dawn":
			c.Cost = 100
		case "buy_food":
			c.Cost = 10
			c.Amount = 1
		case "cast_buff":
			c.Cost = 100
			c.DurationSeconds = 60
		case "exchange":
			c.Exchange = &character.NPCExchange{Costs: map[string]int{}}
		case "prop":
			c.Prop = &character.NPCPropCopy{}
		case "info":
			c.Response = "There is more to discover."
		}
		*list = append(*list, c)
		v.overlaySelect("choice", index)
		v.overlay.choicePath = append(slices.Clone(parent), len(*list)-1)
		return nil
	})
}
func overlayChoiceAt(n *character.NPCData, path []int) *character.NPCDialogueChoice {
	if n == nil || n.Dialogue == nil {
		return nil
	}
	list := n.Dialogue.Choices
	var c *character.NPCDialogueChoice
	for _, i := range path {
		if i < 0 || i >= len(list) {
			return nil
		}
		c = list[i]
		if c == nil {
			return nil
		}
		list = c.Choices
	}
	return c
}

func (v *viewer) overlayChoiceFields(d *overlayDocument) []overlayField {
	p := &v.overlay
	if p.selected < 0 || p.selected >= len(d.state.Data.NPCSpawns) {
		return nil
	}
	key := d.state.Data.NPCSpawns[p.selected].NPCKey
	n := overlayClone(d.npc(key))
	c := overlayChoiceAt(n, p.choicePath)
	if c == nil {
		return nil
	}
	path := slices.Clone(p.choicePath)
	out := []overlayField{overlayNavigate("Back to object", n.Name, "Edit the object, its conditions and other actions.", func() { v.overlaySelect("object", p.selected) }), overlayInfo("Action", c.Action, "Triggered when the player selects this dialogue option."), overlayString("Choice text", &c.Text, "Clickable text in the game's dialogue."), overlayHeading("Conditions"), overlayString("Requires completed quest", &c.RequiresQuest, "Optional: quest must be finished and its rewards claimed.", append([]string{""}, v.overlayCatalog("quests", "quests")...)...), overlayString("Quest step visibility", &c.QuestStep, "Optional: keep this choice with one quest step.", append([]string{""}, v.overlayCatalog("quests", "quests")...)...)}
	quests := append([]string{""}, v.overlayCatalog("quests", "quests")...)
	maps := []string{}
	for _, m := range v.maps {
		maps = append(maps, m.Key)
	}
	out = append(out, overlayHeading("Action settings"))
	switch c.Action {
	case "info":
		out = append(out, overlayString("Reply", &c.Response, "The object's response before showing child choices."), overlayHeading("Follow-up actions"), overlayAction("+ Add follow-up action", "Create a nested dialogue option.", func() { v.overlayAddChoice(d, path) }))
		for i, ch := range c.Choices {
			if ch == nil {
				continue
			}
			child := append(slices.Clone(path), i)
			f := overlayNavigate(ch.Text, "Action: "+ch.Action, "Edit this nested dialogue action.", func() { p.choicePath = child; p.fieldScroll = 0; p.fieldFocus = -1 })
			f.depth = 1
			out = append(out, f)
		}
	case "enter_map":
		out = append(out, overlayString("Destination map", &c.Map, "Travel to a map or an open-world region.", maps...))
		if c.ArrivalTile == nil {
			out = append(out, overlayAction("Set arrival position", "Without an override, use the destination's entry point or return pose.", func() { d.checkpoint(); overlayChoiceAt(d.editNPC(key), path).ArrivalTile = &[2]int{} }))
		} else {
			out = append(out, overlayInt("Arrival column", &c.ArrivalTile[0], "Coordinates local to the destination map."), overlayInt("Arrival row", &c.ArrivalTile[1], "Must be walkable and in bounds."), overlayAction("Use default arrival", "Remove the explicit arrival override.", func() { d.checkpoint(); overlayChoiceAt(d.editNPC(key), path).ArrivalTile = nil }))
		}
	case "enter_schedule":
		out = append(out, overlayString("Schedule source map", &c.Map, "Must be this object's map and have an entrance schedule.", maps...))
	case "adventure_control", "disarm_environment":
		targets := []string{}
		if c.Action == "adventure_control" {
			for _, x := range d.state.Adventure.Controls {
				targets = append(targets, x.ID)
			}
		} else {
			for _, x := range d.state.Adventure.Effects {
				if x.Kind == "trap" {
					targets = append(targets, x.ID)
				}
			}
		}
		out = append(out, overlayString("Target", &c.Control, "Choose a mechanism or trap from this map.", targets...))
	case "give_quest", "turn_in_quest":
		out = append(out, overlayString("Quest", &c.QuestID, "Existing quest definition. Quest objectives remain owned by quests.yaml.", quests...))
	case "teach_spell":
		out = append(out, overlayString("Spell", &c.Spell, "The spell to teach.", v.overlayCatalog("spells", "spells")...), overlayInt("Gold price", &c.Cost, "Must be positive."))
	case "cast_buff":
		out = append(out, overlayString("Party buff", &c.Buff, "Choose a supported timed party buff.", game.EditorTimedBuffIDs()...), overlayInt("Gold price", &c.Cost, "Must be positive."), overlayInt("Duration (seconds)", &c.DurationSeconds, "Must be positive."))
	case "buy_food":
		out = append(out, overlayInt("Gold price", &c.Cost, "Must be positive."), overlayInt("Food amount", &c.Amount, "Must be positive."))
	case "tavern_rest", "wait_until_night", "wait_until_dawn":
		out = append(out, overlayInt("Gold price", &c.Cost, "Must be positive."))
	case "start_arena_duel":
		tiers := []string{}
		if config.GlobalChampionConfig != nil {
			for k := range config.GlobalChampionConfig.Tiers {
				tiers = append(tiers, k)
			}
		}
		sort.Strings(tiers)
		out = append(out, overlayString("Champion tier", &c.Tier, "Uses the configured arena map and champion tier.", tiers...))
	case "combat":
		out = append(out, overlayInfo("Encounter source", key, "Edit monster groups and rewards under the object's Combat encounter settings."))
	case "exchange":
		if c.Exchange == nil {
			c.Exchange = &character.NPCExchange{Costs: map[string]int{}}
		}
		x := c.Exchange
		out = append(out, overlayString("Output item", &x.Output, "Choose either an output item or positive output gold.", append([]string{""}, v.overlayCatalog("items", "items")...)...), overlayInt("Output gold", &x.Gold, "0 when rewarding an item."))
		keys := []string{}
		for k := range x.Costs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			value := x.Costs[k]
			f := overlayInt("Cost: "+k, &value, "Required item count. Set 0 to remove this cost.")
			apply := f.apply
			f.apply = func(s string) error {
				if err := apply(s); err != nil {
					return err
				}
				if value <= 0 {
					delete(x.Costs, k)
				} else {
					x.Costs[k] = value
				}
				return nil
			}
			out = append(out, f)
		}
		out = append(out, overlayAction("Add item cost", "Select a required item.", func() {
			v.overlayChoose("Required item", "Each required item has a configurable count.", v.overlayCatalog("items", "items"), func(k string) error {
				d.checkpoint()
				x := overlayChoiceAt(d.editNPC(key), path).Exchange
				if x.Costs == nil {
					x.Costs = map[string]int{}
				}
				x.Costs[k] = 1
				return nil
			})
		}))
	case "prop":
		if c.Prop == nil {
			c.Prop = &character.NPCPropCopy{}
		}
		q := c.Prop
		out = append(out, overlayString("Quest", &c.QuestID, "Quest whose activity this object advances.", quests...), overlayString("Activity tag", &q.Tag, "Must match the quest's authored activity target."), overlayString("Object token", &q.Token, "Unique token used by the quest's activity definition."), overlayString("Activity step", &q.Step, "Optional authored objective step."), overlayString("Not active message", &q.NotYet, "Shown before the quest needs this object."), overlayString("Interaction message", &q.Took, "Shown when the interaction is credited."), overlayString("Completion message", &q.Completed, "Shown when the final objective completes."), overlayString("Loot table", &q.LootTable, "Optional existing loot table."), overlayString("Loot message", &q.LootLine, "Required when awarding a loot table."), overlayBool("Hide map marker", &q.HideMapMarker, "Hide the quest marker for this prop."), overlayString("Dormant sprite", &q.DormantSprite, "Optional sprite while this quest prop is dormant."))
	case "back", "leave", "open_roster", "manage_stash":
		out = append(out, overlayInfo("Parameters", "No additional parameters", "This action uses the game's shared conversation or party service."))
	}
	out = append(out, overlayAction("Remove this action", "Removes this action and its nested branches. Undo restores them.", func() {
		d.checkpoint()
		x := d.editNPC(key)
		list := &x.Dialogue.Choices
		for _, i := range path[:len(path)-1] {
			list = &(*list)[i].Choices
		}
		i := path[len(path)-1]
		*list = slices.Delete(*list, i, i+1)
		v.overlaySelect("object", p.selected)
	}))
	for i := range out {
		if apply := out[i].apply; apply != nil {
			out[i].apply = func(s string) error {
				if err := apply(s); err != nil {
					return err
				}
				d.state.NPCs[key] = n
				return nil
			}
		}
	}
	return out
}

func (v *viewer) overlayOverviewFields(d *overlayDocument) []overlayField {
	a := &d.state.Adventure
	boss := "No boss placed"
	if a.Boss != nil {
		boss = a.Boss.Monster
	}
	return []overlayField{
		overlayNote("Select an object or area on the map to edit it. Use the tools above the map to add interactions."),
		overlayHeading("Encounter"),
		overlayNavigate("Boss encounter", boss, "Placement, movement area, mechanics and attack patterns.", func() { v.overlaySelect("boss", 0) }),
		overlayHeading("Interactions"),
		overlayInfo("On this map", fmt.Sprintf("%d areas  |  %d switches  |  %d objects", len(a.Effects), len(a.Controls), len(d.state.Data.NPCSpawns)), "Area triggers activate on entry, occupation or a boss action."),
		overlayAction("+ Add object or NPC", "Place a new local interaction or reuse an object from the library.", func() { v.overlayObjectPicker(d) }),
		overlayNavigate("Map rules", "Resets and scheduled entrances", "Configure visit ownership and scheduled entrances.", func() { v.overlaySelect("settings", 0) }),
		overlayNote("Object actions can target zones, switches, maps and quests. Boss mechanics have their own switch links. One boss encounter per map."),
	}
}

// Only editor-owned definitions are disposable. Reusable library entries and
// objects referenced by another placement or draft remain in the catalog.
func (v *viewer) overlayRemoveUnusedLocalNPC(d *overlayDocument, key string) {
	n := d.npc(key)
	if n == nil || (n.EditorOwnerMap != d.key && d.originalNPCs[key] != nil) {
		return
	}
	referenced := func(data *world.MapData) bool {
		return data != nil && slices.ContainsFunc(data.NPCSpawns, func(s world.NPCSpawn) bool { return s.NPCKey == key })
	}
	if referenced(d.state.Data) {
		return
	}
	for _, m := range v.maps {
		if m.Key == d.key {
			continue
		}
		if referenced(m.Data) {
			return
		}
		if other := v.overlay.documents[m.Key]; other != nil && referenced(other.state.Data) {
			return
		}
	}
	d.state.NPCs[key] = nil
}
