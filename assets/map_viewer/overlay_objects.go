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
	"ugataima/internal/quests"
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
	ground := v.tileManager.GetTileKey(d.state.Data.Tiles[tile[1]][tile[0]])
	if def := d.npc(key); def != nil && def.GroundTile != "" {
		ground = "" // the definition's own ground (a portal's stream) applies
	}
	d.state.Data.NPCSpawns = append(d.state.Data.NPCSpawns, world.NPCSpawn{X: tile[0], Y: tile[1], NPCKey: key, GroundTile: ground})
	v.overlaySelect("object", len(d.state.Data.NPCSpawns)-1)
	return nil
}
func (v *viewer) overlayAddChoice(d *overlayDocument, parent []int) {
	index := v.overlay.selected
	v.overlayChoose("Choose an action", "Actions come from the game's dialogue dispatcher. tavern_rest on the first level turns the object into a tavern dialog, which shows only its tavern rows.", game.EditorDialogueActions(), func(action string) error {
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
		case "enter_schedule":
			c.Map = d.key
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
	out := []overlayField{overlayNavigate("Back to object", n.Name, "Edit the object, its conditions and other actions.", func() { v.overlaySelect("object", p.selected) }), overlayInfo("Action", c.Action, "Triggered when the player selects this dialogue option."), overlayString("Choice text", &c.Text, "Clickable text in the game's dialogue."), overlayHeading("Conditions"), overlayString("Requires completed quest", &c.RequiresQuest, "Optional: shown only after this quest is finished and its rewards claimed.", append([]string{""}, v.overlayCatalog("quests", "quests")...)...), overlayString("Show only during quest", &c.QuestStep, "Optional: shown only while this quest, offered or taken in by this object, is in progress.", overlayOwnQuests(n)...)}
	questIDs := append([]string{""}, v.overlayCatalog("quests", "quests")...)
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
		c.Map = d.key
		out = append(out, overlayInfo("Schedule", d.key, "Uses this map's entrance schedule (Map rules). Keep the action on the first level so the greeting shows today's destination."))
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
		out = append(out, overlayString("Quest", &c.QuestID, "Existing quest definition. Quest objectives remain owned by quests.yaml.", questIDs...))
	case "teach_spell":
		out = append(out, overlayString("Spell", &c.Spell, "The spell to teach.", v.overlayCatalog("spells", "spells")...), overlayInt("Gold price", &c.Cost, "Must be positive."))
	case "cast_buff":
		out = append(out, overlayString("Party buff", &c.Buff, "Choose a supported timed party buff.", game.EditorTimedBuffIDs()...), overlayInt("Gold price", &c.Cost, "Must be positive."), overlayInt("Duration (seconds)", &c.DurationSeconds, "Must be positive."))
	case "buy_food":
		out = append(out, overlayInt("Gold price", &c.Cost, "Must be positive."), overlayInt("Food amount", &c.Amount, "Must be positive."))
	case "tavern_rest", "wait_until_night", "wait_until_dawn":
		out = append(out, overlayInt("Gold price", &c.Cost, "Must be positive."), overlayString("Reply", &c.Response, "Required. Shown after resting or waiting."))
	case "start_arena_duel":
		tiers := []string{}
		if config.GlobalChampionConfig != nil {
			for k := range config.GlobalChampionConfig.Tiers {
				tiers = append(tiers, k)
			}
		}
		sort.Strings(tiers)
		out = append(out, overlayString("Champion tier", &c.Tier, "The duel runs on this map's duel arena (duel: in map_configs.yaml) against a champion of this tier.", tiers...))
	case "combat":
		out = append(out, overlayInfo("Fight source", key, "Edit monster groups and rewards under the object's Combat encounter section. Only encounter objects can fight."))
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
		defs := v.overlayQuestDefs()
		interact := []string{}
		for id, def := range defs {
			if def != nil && def.Type == quests.QuestTypeInteract && def.TargetMonster != "" {
				interact = append(interact, id)
			}
		}
		sort.Strings(interact)
		questField := overlayString("Quest", &c.QuestID, "An interact quest. Using this object counts toward it.", interact...)
		applyQuest := questField.apply
		questField.apply = func(s string) error {
			if err := applyQuest(s); err != nil {
				return err
			}
			if def := defs[s]; def != nil {
				q.Tag = def.TargetMonster
				if def.Activity == nil {
					q.Token, q.Step, q.DormantSprite = "", "", ""
				} else {
					q.LootTable, q.LootLine = "", ""
				}
			}
			return nil
		}
		out = append(out, questField, overlayInfo("Object tag", q.Tag, "Copied from the quest's target; the quest counts this tag."))
		if def := defs[c.QuestID]; def != nil && def.Activity != nil {
			out = append(out, overlayString("Activity objective", &q.Token, "Which objective of the quest's activity this object completes.", overlayActivityTokens(def.Activity)...))
			if o := def.Activity.Objective(q.Token); o != nil && len(o.Sequence) > 0 {
				out = append(out, overlayString("Alignment step", &q.Step, "This object's step in the objective's sequence.", slices.Compact(slices.Sorted(slices.Values(o.Sequence)))...))
			}
			out = append(out, overlayString("Sprite while dormant", &q.DormantSprite, "Optional art while the quest does not need this object yet.", append([]string{""}, v.overlaySprites()...)...))
		} else if def != nil {
			out = append(out, overlayString("Loot table", &q.LootTable, "Optional loot rolled on use. Needs the loot message too.", append([]string{""}, v.overlayCatalog("loots", "loot_tables")...)...), overlayString("Loot message", &q.LootLine, "Shown with the loot. Required with a loot table."))
		}
		out = append(out, overlayString("Before the quest needs it", &q.NotYet, "Shown while the quest does not need this object yet."), overlayString("When it counts", &q.Took, "Shown when using the object is credited."), overlayString("When the quest completes", &q.Completed, "Shown when this completes the quest."), overlayBool("Hide map marker", &q.HideMapMarker, "Do not mark this object on the world map."), overlayNote("A quest object must hide after use or have a Message after use (object settings)."))
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

// overlayOwnQuests lists the quests this object offers or takes in: the only
// ones a choice may follow with quest_step.
func overlayOwnQuests(n *character.NPCData) []string {
	out := []string{""}
	if n != nil && n.Dialogue != nil {
		_ = n.Dialogue.WalkChoices(func(c *character.NPCDialogueChoice) error {
			if (c.Action == "give_quest" || c.Action == "turn_in_quest") && c.QuestID != "" && !slices.Contains(out, c.QuestID) {
				out = append(out, c.QuestID)
			}
			return nil
		})
	}
	sort.Strings(out[1:])
	return out
}

// overlayActivityTokens lists every token the game accepts for an activity:
// objectives, sequence steps and forage groups.
func overlayActivityTokens(a *quests.ActivityDefinition) []string {
	out := []string{}
	addToken := func(t string) {
		if t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	for _, o := range a.Objectives {
		addToken(o.Token)
	}
	for _, t := range a.Sequence {
		addToken(t)
	}
	for _, g := range a.Forage {
		for _, t := range g.Tokens {
			addToken(t)
		}
	}
	sort.Strings(out)
	return out
}

func (v *viewer) overlayQuestDefs() map[string]*quests.QuestDefinition {
	if v.overlay.questDefs == nil {
		v.overlay.questDefs = map[string]*quests.QuestDefinition{}
		if cfg, err := quests.LoadQuestConfig("assets/quests.yaml"); err == nil {
			v.overlay.questDefs = cfg.Quests
		}
	}
	return v.overlay.questDefs
}
