package main

import (
	"fmt"
	"slices"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game"
)

func overlayHasAction(n *character.NPCData, action, id string) bool {
	found := false
	if n != nil && n.Dialogue != nil {
		_ = n.Dialogue.WalkChoices(func(c *character.NPCDialogueChoice) error {
			if c.Action == action && c.Control == id {
				found = true
			}
			return nil
		})
	}
	return found
}
func overlayRemoveActions(list []*character.NPCDialogueChoice, action, id string) []*character.NPCDialogueChoice {
	list = slices.DeleteFunc(list, func(c *character.NPCDialogueChoice) bool { return c != nil && c.Action == action && c.Control == id })
	for _, c := range list {
		if c != nil {
			c.Choices = overlayRemoveActions(c.Choices, action, id)
		}
	}
	return list
}
func (v *viewer) validateOverlayObjects(d *overlayDocument) error {
	if b := d.state.Adventure.Boss; b != nil && v.owc != nil {
		if _, merged := v.owc.Placements[d.key]; merged {
			for _, m := range v.maps {
				if m.Key == d.key || m.Data == nil {
					continue
				}
				if _, yes := v.owc.Placements[m.Key]; yes {
					for _, spawn := range m.Data.MonsterSpawns {
						if spawn.MonsterKey == b.Monster {
							return fmt.Errorf("Boss archetype %s is also placed in merged region %s; use a distinct boss archetype", b.Monster, m.Key)
						}
					}
				}
			}
		}
	}

	for key, n := range d.state.NPCs {
		if overlaySame(n, d.originalNPCs[key]) {
			continue
		}
		for _, m := range v.maps {
			if m.Key != d.key && m.Data != nil {
				for _, s := range m.Data.NPCSpawns {
					if s.NPCKey == key {
						return fmt.Errorf("%s is shared with %s; select Make local copy first", key, m.Key)
					}
				}
			}
		}
	}
	defs := map[string]*character.NPCData{}
	for _, s := range d.state.Data.NPCSpawns {
		if n := d.npc(s.NPCKey); n != nil {
			defs[s.NPCKey] = overlayClone(n)
		} else {
			return fmt.Errorf("Unknown object %s", s.NPCKey)
		}
	}
	if err := game.ValidateEditorNPCs(defs, v.cfg.Graphics.SizeClasses); err != nil {
		return err
	}
	for key, n := range defs {
		if n.Encounter != nil {
			e := n.Encounter
			if len(e.Monsters) == 0 {
				return fmt.Errorf("%s: encounter needs a monster group", key)
			}
			for _, group := range e.Monsters {
				if group == nil {
					return fmt.Errorf("%s: empty monster group", key)
				}
				if !slices.Contains(v.overlayEncounterMonsters(), group.Type) || group.CountMin < 1 || group.CountMax < group.CountMin {
					return fmt.Errorf("%s: invalid encounter monster or count (fish and arena champions cannot be fight groups)", key)
				}
			}
			if e.Rewards != nil && (e.Rewards.Gold < 0 || e.Rewards.Experience < 0) {
				return fmt.Errorf("%s: rewards cannot be negative", key)
			}
		}
		if n.Dialogue == nil {
			continue
		}
		for _, root := range n.Dialogue.Choices {
			if c := overlayNestedAction(root, "enter_schedule"); c != nil {
				return fmt.Errorf("%s / %s: put the schedule entrance on the first dialogue level so the greeting can show it", key, c.Text)
			}
		}
		if err := n.Dialogue.WalkChoices(func(c *character.NPCDialogueChoice) error {
			fail := func(msg string) error { return fmt.Errorf("%s / %s: %s", key, c.Text, msg) }
			if c.Text == "" {
				return fail("choice text is required")
			}
			a := &d.state.Adventure
			switch c.Action {
			case "adventure_control":
				if a.Control(c.Control) == nil {
					return fail("select an existing control")
				}
			case "disarm_environment":
				if e := a.Effect(c.Control); e == nil || e.Kind != "trap" {
					return fail("select an existing pressure trap")
				}
			case "enter_map", "enter_schedule":
				found := false
				for _, m := range v.maps {
					if m.Key == c.Map {
						found = true
						if c.Action == "enter_schedule" && (c.Map != d.key || a.Schedule == nil) {
							return fail("schedule must belong to this map")
						}
						if p := c.ArrivalTile; p != nil && (m.Data == nil || p[0] < 0 || p[1] < 0 || p[0] >= m.Data.Width || p[1] >= m.Data.Height || !v.tileManager.IsWalkable(m.Data.Tiles[p[1]][p[0]])) {
							return fail("arrival must be a walkable destination tile")
						}
					}
				}
				if !found {
					return fail("select an existing destination map")
				}
			case "combat":
				if n.Type != character.NPCTypeEncounter {
					return fail("only objects of type encounter can start a fight")
				}
				if n.Encounter == nil || len(n.Encounter.Monsters) == 0 {
					return fail("configure this object's encounter monster groups")
				}
			case "give_quest", "turn_in_quest", "prop":
				if !slices.Contains(v.overlayCatalog("quests", "quests"), c.QuestID) {
					return fail("select an existing quest")
				}
			case "start_arena_duel":
				if config.GlobalChampionConfig == nil || config.GlobalChampionConfig.Tiers[c.Tier] == nil {
					return fail("select an existing champion tier")
				}
				if mc := v.overlayMapConfig(d.key); mc == nil || mc.Duel == nil {
					return fail("this map has no duel arena; add a duel: block to its map_configs.yaml entry first")
				}
			}
			for _, q := range []string{c.RequiresQuest, c.QuestStep} {
				if q != "" && !slices.Contains(v.overlayCatalog("quests", "quests"), q) {
					return fail("unknown quest condition")
				}
			}
			// Activity tokens, loot and the quest's tag are checked by the game's
			// own quest validation below.
			if c.Prop != nil && c.Action != "prop" || c.Action == "prop" && (c.Prop == nil || c.Prop.Tag == "") {
				return fail("a quest object needs the prop action and its quest's tag")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return game.ValidateEditorQuestLinks(defs, "assets/quests.yaml")
}

func (v *viewer) overlayMapConfig(key string) *config.MapConfig {
	for _, m := range v.maps {
		if m.Key == key {
			return m.Config
		}
	}
	return nil
}

// overlayNestedAction finds action below (not at) a first-level choice.
func overlayNestedAction(root *character.NPCDialogueChoice, action string) *character.NPCDialogueChoice {
	if root == nil {
		return nil
	}
	for _, c := range root.Choices {
		if c != nil && c.Action == action {
			return c
		}
		if found := overlayNestedAction(c, action); found != nil {
			return found
		}
	}
	return nil
}
