package quests

import (
	"fmt"
	"math"
	"math/rand"
	"slices"
)

// Activities are authored interactions. Tokens identify physical props, never
// slice positions, so saved selections survive content reordering.
type ActivityDefinition struct {
	Sequence       []string      `yaml:"sequence,omitempty"`
	Forage         []ForageGroup `yaml:"forage,omitempty"`
	WrongMessage   string        `yaml:"wrong_message,omitempty"`
	DormantMessage string        `yaml:"dormant_message,omitempty"`
}
type ForageGroup struct {
	Phase        string   `yaml:"phase"`
	Count        int      `yaml:"count"`
	Tokens       []string `yaml:"tokens"`
	LegacyTokens []string `yaml:"legacy_tokens,omitempty"`
}

// QuestProp is a map-local activity prop, present only between acceptance and
// turn-in. NPC keys are unique identities; activity state owns consumption.
type QuestProp struct {
	NPC string `yaml:"npc"`
	Map string `yaml:"map"`
	X   int    `yaml:"x"`
	Y   int    `yaml:"y"`
}

type QuestPropLayout struct {
	ID    string      `yaml:"id"`
	Props []QuestProp `yaml:"props"`
}

func (d *QuestDefinition) PropLayouts() []QuestPropLayout {
	if len(d.ActiveProps) > 0 {
		return []QuestPropLayout{{ID: "default", Props: d.ActiveProps}}
	}
	return d.ActivePropLayouts
}

func (d *QuestDefinition) PropsForLayout(id string) []QuestProp {
	for _, layout := range d.PropLayouts() {
		if layout.ID == id {
			return layout.Props
		}
	}
	return nil
}

type ActivityState struct {
	SequenceIndex int      `json:"sequence_index,omitempty"`
	Selected      []string `json:"selected,omitempty"`
	Collected     []string `json:"collected,omitempty"`
}

func (s ActivityState) Clone() ActivityState {
	s.Selected = slices.Clone(s.Selected)
	s.Collected = slices.Clone(s.Collected)
	return s
}

// RestoreActivityState remaps retired forage identities within their authored
// phase, preserving collected credit while keeping every remaining prop usable.
func RestoreActivityState(d *ActivityDefinition, saved ActivityState) ActivityState {
	s := saved.Clone()
	if d == nil {
		return s
	}
	for _, group := range d.Forage {
		used := map[string]bool{}
		for _, token := range s.Selected {
			if slices.Contains(group.Tokens, token) {
				used[token] = true
			}
		}
		for i, token := range s.Selected {
			if !slices.Contains(group.LegacyTokens, token) {
				continue
			}
			for _, replacement := range group.Tokens {
				if used[replacement] {
					continue
				}
				s.Selected[i] = replacement
				used[replacement] = true
				for j, collected := range s.Collected {
					if collected == token {
						s.Collected[j] = replacement
					}
				}
				break
			}
		}
	}
	return s
}
func newActivityState(d *ActivityDefinition) (s ActivityState) {
	if d == nil {
		return
	}
	for _, group := range d.Forage {
		choices := rand.Perm(len(group.Tokens))
		for _, i := range choices[:group.Count] {
			s.Selected = append(s.Selected, group.Tokens[i])
		}
	}
	return
}
func (d *ActivityDefinition) TokenPhase(token string) string {
	if d != nil {
		for _, group := range d.Forage {
			if slices.Contains(group.Tokens, token) {
				return group.Phase
			}
		}
	}
	return ""
}
func validateActivity(id string, d *QuestDefinition, cfg *QuestConfig) error {
	if d.Type == QuestTypeEncounter && (d.NextQuest != "" || len(d.Rewards.Items) > 0 || len(d.OnAcceptSpawns) > 0) {
		return fmt.Errorf("quest %q: encounter auto-rewards do not support chain chapters", id)
	}
	if d.MinPartyLevel < 0 {
		return fmt.Errorf("quest %q: negative minimum level", id)
	}
	seen := map[string]bool{id: true}
	for next := d.NextQuest; next != ""; next = cfg.Quests[next].NextQuest {
		if seen[next] || cfg.Quests[next] == nil {
			return fmt.Errorf("quest %q: cyclic or unknown next_quest %q", id, next)
		}
		seen[next] = true
	}
	for _, sp := range d.OnAcceptSpawns {
		if sp.ID == "" || sp.Map == "" || sp.Monster == "" || seen["spawn:"+sp.ID] || sp.OnEntry {
			return fmt.Errorf("quest %q: invalid on_accept_spawns entry %q", id, sp.ID)
		}
		seen["spawn:"+sp.ID] = true
	}
	a := d.Activity
	if len(d.PropLayouts()) > 0 && a == nil {
		return fmt.Errorf("quest %q: active_props requires an activity", id)
	}
	if len(d.ActiveProps) > 0 && len(d.ActivePropLayouts) > 0 {
		return fmt.Errorf("quest %q: choose active_props or active_prop_layouts", id)
	}
	if d.MinPropSpacingTiles < 0 || math.IsNaN(d.MinPropSpacingTiles) || math.IsInf(d.MinPropSpacingTiles, 0) {
		return fmt.Errorf("quest %q: invalid min_prop_spacing_tiles", id)
	}
	layoutIDs := map[string]bool{}
	var identities map[string]bool
	for _, layout := range d.PropLayouts() {
		if layout.ID == "" || layoutIDs[layout.ID] || len(layout.Props) == 0 {
			return fmt.Errorf("quest %q: empty or duplicate active prop layout", id)
		}
		layoutIDs[layout.ID] = true
		props := map[string]bool{}
		positions := map[string]bool{}
		for i, prop := range layout.Props {
			position := fmt.Sprintf("%s:%d:%d", prop.Map, prop.X, prop.Y)
			if prop.NPC == "" || prop.Map == "" || prop.X < 0 || prop.Y < 0 || props[prop.NPC] || positions[position] {
				return fmt.Errorf("quest %q: invalid or duplicate active_props entry %q", id, prop.NPC)
			}
			props[prop.NPC], positions[position] = true, true
			for _, other := range layout.Props[:i] {
				if prop.Map == other.Map && math.Hypot(float64(prop.X-other.X), float64(prop.Y-other.Y)) < d.MinPropSpacingTiles {
					return fmt.Errorf("quest %q layout %q: props violate min_prop_spacing_tiles", id, layout.ID)
				}
			}
		}
		if identities == nil {
			identities = props
		} else {
			if len(identities) != len(props) {
				return fmt.Errorf("quest %q: layouts must contain the same NPC identities", id)
			}
			for key := range identities {
				if !props[key] {
					return fmt.Errorf("quest %q: layouts must contain the same NPC identities", id)
				}
			}
		}
	}
	if a == nil {
		return nil
	}
	if d.Type != QuestTypeInteract || d.Repeatable != "" || d.IsStartingQuest {
		return fmt.Errorf("quest %q: activity needs a nonrepeatable offered interact quest", id)
	}
	if (len(a.Sequence) == 0) == (len(a.Forage) == 0) {
		return fmt.Errorf("quest %q: activity needs exactly one of sequence or forage", id)
	}
	if len(a.Sequence) > 0 {
		if d.TargetCount != 1 || a.WrongMessage == "" {
			return fmt.Errorf("quest %q: sequence needs target_count 1 and wrong_message", id)
		}
		for _, token := range a.Sequence {
			if token == "" {
				return fmt.Errorf("quest %q: empty sequence token", id)
			}
		}
	}
	count := 0
	tokens := map[string]bool{}
	phases := map[string]bool{}
	for _, group := range a.Forage {
		if phases[group.Phase] || (group.Phase != "day" && group.Phase != "night") || group.Count < 1 || group.Count > len(group.Tokens) {
			return fmt.Errorf("quest %q: invalid forage group", id)
		}
		phases[group.Phase] = true
		count += group.Count
		for _, token := range group.Tokens {
			if token == "" || tokens[token] {
				return fmt.Errorf("quest %q: empty or duplicate forage token %q", id, token)
			}
			tokens[token] = true
		}
	}
	for _, group := range a.Forage {
		for _, token := range group.LegacyTokens {
			if token == "" || tokens[token] {
				return fmt.Errorf("quest %q: invalid legacy forage token %q", id, token)
			}
			tokens[token] = true
		}
	}
	if len(a.Forage) > 0 && (count != d.TargetCount || a.DormantMessage == "") {
		return fmt.Errorf("quest %q: forage quotas must match target_count and need dormant_message", id)
	}
	return nil
}

// InteractActivity checks phase, identity and quest state before consuming a
// token. Wrong sequence choices reset progress; they never spend the prop.
func (qm *QuestManager) InteractActivity(id, tag, token string, night bool) (credited, completed bool, message string) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	q := qm.activeQuests[id]
	if q == nil || q.Status != QuestStatusActive || q.Definition.Activity == nil || !q.Definition.MatchesTarget(tag) {
		return
	}
	a := q.Definition.Activity
	if len(a.Sequence) > 0 {
		if !slices.Contains(a.Sequence, token) {
			return
		}
		if q.Activity.SequenceIndex < 0 || q.Activity.SequenceIndex >= len(a.Sequence) {
			q.Activity.SequenceIndex = 0
		}
		if a.Sequence[q.Activity.SequenceIndex] != token {
			q.Activity.SequenceIndex = 0
			return false, false, a.WrongMessage
		}
		q.Activity.SequenceIndex++
		if q.Activity.SequenceIndex < len(a.Sequence) {
			return true, false, ""
		}
	} else {
		if !slices.Contains(q.Activity.Selected, token) || slices.Contains(q.Activity.Collected, token) {
			return
		}
		phase := a.TokenPhase(token)
		if (phase == "night") != night {
			return false, false, a.DormantMessage
		}
		q.Activity.Collected = append(q.Activity.Collected, token)
	}
	q.CurrentCount++
	if q.CurrentCount >= q.Target() {
		q.complete(false)
	}
	return true, q.Completed, ""
}
