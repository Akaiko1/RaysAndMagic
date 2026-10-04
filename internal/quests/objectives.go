package quests

import (
	"fmt"
	"math"
	"slices"
)

// ActivityObjective is a unique, map-owned event with explicit prerequisites.
// A sequence belongs to one objective; mistakes cannot erase earlier discoveries.
type ActivityObjective struct {
	Token        string   `yaml:"token"`
	Map          string   `yaml:"map"`
	Event        string   `yaml:"event"`
	Phase        string   `yaml:"phase,omitempty"`
	Requires     []string `yaml:"requires,omitempty"`
	Sequence     []string `yaml:"sequence,omitempty"`
	RequiresBoss string   `yaml:"requires_boss,omitempty"`
	From         [2]int   `yaml:"from,omitempty"`
	To           [2]int   `yaml:"to,omitempty"`
	Item         string   `yaml:"item,omitempty"`
	Count        int      `yaml:"count,omitempty"`
}

func (a *ActivityDefinition) Objective(token string) *ActivityObjective {
	if a != nil {
		for i := range a.Objectives {
			if a.Objectives[i].Token == token {
				return &a.Objectives[i]
			}
		}
	}
	return nil
}

func validateObjectives(id string, d *QuestDefinition) error {
	a := d.Activity
	if d.Type != QuestTypeInteract || len(a.Sequence) > 0 || len(a.Forage) > 0 || d.IsStartingQuest || d.TargetCount != len(a.Objectives) {
		return fmt.Errorf("quest %q: objectives require an offered interact quest with matching quota", id)
	}
	seen := map[string]bool{}
	for _, o := range a.Objectives {
		if o.Token == "" || o.Map == "" || seen[o.Token] || (o.Event != "interact" && o.Event != "jump" && o.Event != "crossing") || (o.Phase != "" && o.Phase != "day" && o.Phase != "night") {
			return fmt.Errorf("quest %q: invalid objective %q", id, o.Token)
		}
		if (o.Event == "jump" || o.Event == "crossing") && (o.From == o.To || len(o.Sequence) > 0 || o.Item != "") {
			return fmt.Errorf("quest %q: invalid jump objective", id)
		}
		if (o.Item == "") != (o.Count == 0) || o.Count < 0 {
			return fmt.Errorf("quest %q: invalid objective item quota", id)
		}
		for _, step := range o.Sequence {
			if step == "" {
				return fmt.Errorf("quest %q: empty alignment step", id)
			}
		}
		seen[o.Token] = true
	}
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(token string) error {
		if done[token] {
			return nil
		}
		if visiting[token] || !seen[token] {
			return fmt.Errorf("quest %q: cyclic or missing prerequisite %q", id, token)
		}
		visiting[token] = true
		for _, r := range a.Objective(token).Requires {
			if err := visit(r); err != nil {
				return err
			}
		}
		visiting[token] = false
		done[token] = true
		return nil
	}
	for token := range seen {
		if err := visit(token); err != nil {
			return err
		}
	}
	return nil
}

// ObjectiveEvent is emitted by an interaction or committed movement. Callers
// check physical ownership and item/boss prerequisites before submitting it.
type ObjectiveEvent struct {
	Token, Map, Event, Step string
	Night                   bool
	From, To                [2]int
}

func (qm *QuestManager) CreditObjective(id string, event ObjectiveEvent) (credited, completed bool, message string) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	q := qm.activeQuests[id]
	if q == nil || q.Status != QuestStatusActive || q.Definition.Activity == nil {
		return
	}
	a := q.Definition.Activity
	o := a.Objective(event.Token)
	if o == nil || o.Map != event.Map || o.Event != event.Event || slices.Contains(q.Activity.Collected, o.Token) {
		return
	}
	for _, r := range o.Requires {
		if !slices.Contains(q.Activity.Collected, r) {
			return false, false, "Complete the preceding work first."
		}
	}
	if o.Phase != "" && (o.Phase == "night") != event.Night {
		return false, false, "This reading needs the other half of the day."
	}
	if o.Event == "jump" && (o.From != event.From || o.To != event.To) {
		return
	}
	if o.Event == "crossing" && !o.crossedBy(event.From, event.To) {
		return
	}
	if len(o.Sequence) > 0 {
		if q.Activity.Attempts == nil {
			q.Activity.Attempts = map[string]int{}
		}
		i := q.Activity.Attempts[o.Token]
		if i < 0 || i >= len(o.Sequence) {
			i = 0
		}
		if o.Sequence[i] != event.Step {
			q.Activity.Attempts[o.Token] = 0
			return false, false, "The disc releases its setting. Begin the alignment again."
		}
		q.Activity.Attempts[o.Token] = i + 1
		if i+1 < len(o.Sequence) {
			return false, false, "The disc clicks into position."
		}
	}
	q.Activity.Collected = append(q.Activity.Collected, o.Token)
	q.CurrentCount = len(q.Activity.Collected)
	if q.CurrentCount >= q.Target() {
		q.complete(false)
	}
	return true, q.Completed, ""
}

// A crossing spans both authored banks in the forward direction. A longer
// Fold may depart earlier or land farther away, but must pass the gap itself.
func (o ActivityObjective) crossedBy(from, to [2]int) bool {
	dx, dy := float64(o.To[0]-o.From[0]), float64(o.To[1]-o.From[1])
	length2 := dx*dx + dy*dy
	if length2 == 0 {
		return false
	}
	ax, ay := float64(from[0]-o.From[0]), float64(from[1]-o.From[1])
	bx, by := float64(to[0]-o.From[0]), float64(to[1]-o.From[1])
	start, end := ax*dx+ay*dy, bx*dx+by*dy
	if start > 0 || end < length2 {
		return false
	}
	t := (length2/2 - start) / (end - start)
	x, y := ax+(bx-ax)*t, ay+(by-ay)*t
	return math.Abs(x*dy-y*dx) < 0.5*math.Sqrt(length2)
}
