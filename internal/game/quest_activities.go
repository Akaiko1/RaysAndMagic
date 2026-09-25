package game

import (
	"fmt"
	"slices"
	"strings"
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

func (g *MMGame) canActivateQuest(id string) error {
	if g.questManager == nil {
		return fmt.Errorf("quest manager is unavailable")
	}
	if g.questManager.GetQuest(id) != nil {
		return fmt.Errorf("%s", uitext.Text("dialog.you_are_already_on_that_quest"))
	}
	d := g.questManager.Definitions()[id]
	if d == nil {
		return fmt.Errorf("unknown quest %q", id)
	}
	if !g.partyLevelUnlocked(d.MinPartyLevel) {
		return fmt.Errorf("This quest requires party level %d.", d.MinPartyLevel)
	}
	return nil
}

func (g *MMGame) activateQuest(id string) error {
	if err := g.canActivateQuest(id); err != nil {
		return err
	}
	if err := g.questManager.ActivateQuest(id); err != nil {
		return err
	}
	g.spawnQuestCompletionMonsters(false)
	g.syncQuestProps()
	if g.creditQuestIfCleared(id) {
		g.applyCompletedQuestTiles()
	}
	return nil
}

func activityProp(npc *character.NPC) (string, *character.NPCPropCopy) {
	var id string
	var prop *character.NPCPropCopy
	if npc != nil {
		_ = npc.DialogueData.WalkChoices(func(choice *character.NPCDialogueChoice) error {
			if prop == nil && choice.Prop != nil && choice.Prop.Token != "" {
				id, prop = choice.QuestID, choice.Prop
			}
			return nil
		})
	}
	return id, prop
}
func (g *MMGame) activityNPCAbsent(npc *character.NPC) bool {
	id, prop := activityProp(npc)
	if prop == nil || g.questManager == nil {
		return false
	}
	d := g.questManager.Definitions()[id]
	if d == nil || d.Activity == nil {
		return false
	}
	q := g.questManager.GetQuest(id)
	if q == nil || q.RewardsClaimed || q.Status == quests.QuestStatusFailed {
		return true
	}
	return len(d.Activity.Forage) > 0 && (!slices.Contains(q.Activity.Selected, prop.Token) || slices.Contains(q.Activity.Collected, prop.Token))
}
func (g *MMGame) activityNPCSprite(npc *character.NPC) string {
	id, prop := activityProp(npc)
	if prop != nil && prop.DormantSprite != "" && g.questManager != nil {
		if q := g.questManager.GetQuest(id); q != nil && q.Definition.Activity != nil {
			phase := q.Definition.Activity.TokenPhase(prop.Token)
			if phase != "" && (phase == "night") != g.dayNightIsNight {
				return prop.DormantSprite
			}
		}
	}
	return npcSpriteName(npc)
}
func (g *MMGame) handleQuestActivity(npc *character.NPC, id string, words *character.NPCPropCopy) bool {
	q := g.questManager.GetQuest(id)
	if q != nil && q.Completed {
		if npc != nil && npc.DialogueData != nil {
			g.AddCombatMessage(npc.DialogueData.VisitedMessage)
		}
		return true
	}
	if q == nil || q.Status != quests.QuestStatusActive {
		g.AddCombatMessage(words.NotYet)
		return true
	}
	if q.Definition.Activity == nil {
		return false
	}
	credited, completed, message := g.questManager.InteractActivity(id, words.Tag, words.Token, g.dayNightIsNight)
	g.syncQuestProps()
	if message != "" {
		g.AddCombatMessage(message)
	}
	if !credited {
		return true
	}
	g.AddCombatMessage(words.Took)
	if completed {
		g.applyCompletedQuestTiles()
		g.announceQuestCompletionWithMessage(q, words.Completed+" Claim its reward in the quest journal (J).")
	}
	return true
}

// validateQuestPropCopy is shared by general and activity-specific validation.
func validateQuestPropCopy(npcKey string, p *character.NPCPropCopy) error {
	for _, field := range []struct{ name, text string }{
		{"not_yet", p.NotYet}, {"took", p.Took}, {"completed", p.Completed},
	} {
		if strings.TrimSpace(field.text) == "" {
			return fmt.Errorf("NPC %q: prop.%s must contain nonblank text", npcKey, field.name)
		}
	}
	return nil
}

func validateQuestActivityProps(qm *quests.QuestManager, complete bool) error {
	if character.NPCConfigInstance == nil {
		return nil
	}
	producers := map[string]map[string]string{}
	for key, npc := range character.NPCConfigInstance.NPCs {
		if npc.Dialogue == nil {
			continue
		}
		if err := npc.Dialogue.WalkChoices(func(choice *character.NPCDialogueChoice) error {
			if choice == nil || choice.Prop == nil {
				return nil
			}
			p := choice.Prop
			d := qm.Definitions()[choice.QuestID]
			if d == nil {
				return nil
			}
			if d.Activity == nil {
				if p.Token != "" || p.DormantSprite != "" {
					return fmt.Errorf("NPC %q: activity token without quest activity", key)
				}
				return nil
			}
			if err := validateQuestPropCopy(key, p); err != nil {
				return err
			}
			if p.Token == "" || (!slices.Contains(d.Activity.Sequence, p.Token) && d.Activity.TokenPhase(p.Token) == "") {
				return fmt.Errorf("NPC %q: unknown activity token %q", key, p.Token)
			}
			if p.LootTable != "" {
				return fmt.Errorf("NPC %q: activity rewards belong on the quest", key)
			}
			if producers[choice.QuestID] == nil {
				producers[choice.QuestID] = map[string]string{}
			}
			if old := producers[choice.QuestID][p.Token]; old != "" {
				return fmt.Errorf("NPC %q and %q duplicate activity token %q", old, key, p.Token)
			}
			producers[choice.QuestID][p.Token] = key
			return nil
		}); err != nil {
			return err
		}
	}
	if !complete {
		return nil
	}
	placed := placedNPCKeys(world.GlobalWorldManager)
	for id, d := range qm.Definitions() {
		if d.Activity == nil {
			continue
		}
		tokens := append([]string(nil), d.Activity.Sequence...)
		for _, group := range d.Activity.Forage {
			tokens = append(tokens, group.Tokens...)
		}
		for _, token := range tokens {
			if producers[id][token] == "" {
				return fmt.Errorf("quest %q: activity token %q has no NPC", id, token)
			}
			dynamic := len(d.PropLayouts()) > 0
			for _, layout := range d.PropLayouts() {
				dynamic = dynamic && slices.ContainsFunc(layout.Props, func(p quests.QuestProp) bool { return p.NPC == producers[id][token] })
			}
			if placed != nil && !placed[producers[id][token]] && !dynamic {
				return fmt.Errorf("quest %q: activity token %q has no placed NPC", id, token)
			}
		}
	}
	return nil
}

// Exploration props can conceal their map dot without hiding their world sprite.
func (g *MMGame) npcMapMarkerVisible(npc *character.NPC) bool {
	if g.npcAbsent(npc) {
		return false
	}
	hidden := false
	if npc.DialogueData != nil {
		_ = npc.DialogueData.WalkChoices(func(c *character.NPCDialogueChoice) error {
			hidden = hidden || (c.Prop != nil && c.Prop.HideMapMarker)
			return nil
		})
	}
	return !hidden
}
