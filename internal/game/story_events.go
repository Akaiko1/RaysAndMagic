package game

import (
	"fmt"
	"os"
	"slices"
	"sort"

	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/strictyaml"
	"ugataima/internal/world"
)

// Story events are the main storyline's full-screen beats
// (assets/story_events.yaml). Each fires once per run at one of a closed set
// of moments, waits for a calm moment (no fight, no other modal) and is shown
// by story_screen.go. Seen and still-queued events persist in the save.

type storyTriggerKind string

const (
	storyOnNewGame        storyTriggerKind = "new_game"
	storyOnQuestAccepted  storyTriggerKind = "quest_accepted"
	storyOnQuestCompleted storyTriggerKind = "quest_completed"
	storyOnQuestClaimed   storyTriggerKind = "quest_claimed"
	storyOnMonsterKilled  storyTriggerKind = "monster_killed"
	storyOnRegionEntered  storyTriggerKind = "region_entered"
	storyOnVictory        storyTriggerKind = "victory"
)

// storySpeakerCaptain marks an event told by the Pursuers' captain.
const storySpeakerCaptain = "captain"

// storyEventsVersion stamps saves made with story events; an older save gets
// its seen set derived from its quest state instead (resyncLegacyStory).
const storyEventsVersion = 1

// StoryTrigger is the moment an event fires.
type StoryTrigger struct {
	On storyTriggerKind `yaml:"on"`
	// Quests: the quest triggers fire on a change of any of these quests.
	Quests []string `yaml:"quests,omitempty"`
	// Monsters (keys) or Boss (any boss) select the monster_killed kills.
	Monsters []string `yaml:"monsters,omitempty"`
	Boss     bool     `yaml:"boss,omitempty"`
	// Maps: region_entered fires on arriving in any of these maps.
	Maps []string `yaml:"maps,omitempty"`
}

// StoryEventDef is one authored story beat.
type StoryEventDef struct {
	ID      string       `yaml:"id"`
	Trigger StoryTrigger `yaml:"trigger"`
	// Art is the plate's sprite key (sprites/interface/story/).
	Art     string `yaml:"art"`
	Title   string `yaml:"title"`
	Speaker string `yaml:"speaker,omitempty"`
	// Pages are shown one at a time.
	Pages []string `yaml:"pages"`
	// RevealsQuests lists hidden quests the journal shows once this is seen.
	RevealsQuests []string `yaml:"reveals_quests,omitempty"`
}

type storyEventConfig struct {
	Events []StoryEventDef `yaml:"events"`
}

// globalStoryEvents is the loaded catalog; nil (tests, tools) means no story.
var globalStoryEvents []StoryEventDef

const storyMaxPages = 3

// LoadStoryConfig loads assets/story_events.yaml and validates every
// reference against the loaded quests, monsters and maps.
func LoadStoryConfig(path string, questManager *quests.QuestManager, wm *world.WorldManager) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("story events: %w", err)
	}
	var cfg storyEventConfig
	if err := strictyaml.Decode(data, &cfg); err != nil {
		return fmt.Errorf("story events: %w", err)
	}
	if err := validateStoryEvents(cfg.Events, questManager, wm); err != nil {
		return fmt.Errorf("story events: %w", err)
	}
	globalStoryEvents = cfg.Events
	return nil
}

func validateStoryEvents(events []StoryEventDef, questManager *quests.QuestManager, wm *world.WorldManager) error {
	seen, revealed := map[string]bool{}, map[string]bool{}
	quest := func(id string) *quests.QuestDefinition {
		if questManager == nil {
			return nil
		}
		return questManager.Definitions()[id]
	}
	for _, e := range events {
		if e.ID == "" || seen[e.ID] {
			return fmt.Errorf("event id %q is empty or repeated", e.ID)
		}
		seen[e.ID] = true
		if e.Art == "" || e.Title == "" || len(e.Pages) == 0 || len(e.Pages) > storyMaxPages {
			return fmt.Errorf("%s needs art, a title and 1-%d pages", e.ID, storyMaxPages)
		}
		for _, text := range append([]string{e.Title}, e.Pages...) {
			if text == "" || !config.IsASCII(text) {
				return fmt.Errorf("%s has an empty or non-ASCII line", e.ID)
			}
		}
		if e.Speaker != "" && e.Speaker != storySpeakerCaptain {
			return fmt.Errorf("%s has unknown speaker %q", e.ID, e.Speaker)
		}
		t := e.Trigger
		// A field the trigger kind never reads would be silently ignored.
		quests, monsters, region := len(t.Quests) > 0, len(t.Monsters) > 0 || t.Boss, len(t.Maps) > 0
		switch t.On {
		case storyOnNewGame, storyOnVictory:
			if quests || monsters || region {
				return fmt.Errorf("%s: %s takes no quests, monsters or map", e.ID, t.On)
			}
		case storyOnQuestAccepted, storyOnQuestCompleted, storyOnQuestClaimed:
			if monsters || region {
				return fmt.Errorf("%s: %s takes only quests", e.ID, t.On)
			}
			if len(t.Quests) == 0 {
				return fmt.Errorf("%s: %s needs quests", e.ID, t.On)
			}
			for _, id := range t.Quests {
				def := quest(id)
				if def == nil {
					return fmt.Errorf("%s: unknown quest %q", e.ID, id)
				}
				// The party starts with these: their taking is the new game.
				if t.On == storyOnQuestAccepted && def.IsStartingQuest {
					return fmt.Errorf("%s: %q is a starting quest; its acceptance is new_game", e.ID, id)
				}
			}
		case storyOnMonsterKilled:
			if quests || region {
				return fmt.Errorf("%s: monster_killed takes only monsters or boss", e.ID)
			}
			if len(t.Monsters) == 0 && !t.Boss {
				return fmt.Errorf("%s: monster_killed needs monsters or boss", e.ID)
			}
			if err := validateMonsterKeys(e.ID, t.Monsters); err != nil {
				return err
			}
		case storyOnRegionEntered:
			if quests || monsters {
				return fmt.Errorf("%s: region_entered takes only a map", e.ID)
			}
			if !region {
				return fmt.Errorf("%s: region_entered needs maps", e.ID)
			}
			for _, key := range t.Maps {
				if wm == nil || wm.MapConfigs[key] == nil {
					return fmt.Errorf("%s: unknown map %q", e.ID, key)
				}
			}
		default:
			return fmt.Errorf("%s: unknown trigger %q", e.ID, t.On)
		}
		for _, id := range e.RevealsQuests {
			if def := quest(id); def == nil || !def.Hidden {
				return fmt.Errorf("%s reveals %q, which is not a hidden quest", e.ID, id)
			}
			revealed[id] = true
		}
	}
	// A hidden quest no beat reveals would stay out of the journal all run.
	if questManager != nil {
		for id, def := range questManager.Definitions() {
			if def.Hidden && !revealed[id] {
				return fmt.Errorf("hidden quest %q is revealed by no story event", id)
			}
		}
	}
	return nil
}

// validateStoryArt fails fast on a story plate or the captain's portrait that
// is missing from the sprite index; skipped without an asset tree (headless
// fixtures), like the other sprite checks.
func (g *MMGame) validateStoryArt() {
	if _, err := os.Stat("assets/sprites"); err != nil {
		return
	}
	for _, e := range globalStoryEvents {
		if !g.sprites.HasSprite(e.Art) {
			panic(fmt.Sprintf("story event %q: art %q not found under assets/sprites", e.ID, e.Art))
		}
	}
	if globalWatcher != nil && !g.sprites.HasSprite(globalWatcher.Portrait) {
		panic(fmt.Sprintf("watcher: portrait %q not found under assets/sprites", globalWatcher.Portrait))
	}
}

// validateMonsterKeys fails on a key the monster catalog does not hold.
func validateMonsterKeys(owner string, keys []string) error {
	for _, key := range keys {
		if monster.MonsterConfig == nil {
			return fmt.Errorf("%s: monsters are not loaded", owner)
		}
		if _, ok := monster.MonsterConfig.Monsters[key]; !ok {
			return fmt.Errorf("%s: unknown monster %q", owner, key)
		}
	}
	return nil
}

func storyEvent(id string) *StoryEventDef {
	for i := range globalStoryEvents {
		if globalStoryEvents[i].ID == id {
			return &globalStoryEvents[i]
		}
	}
	return nil
}

// fireStoryTrigger queues every unseen event a moment fires: a kill (key: the
// monster key), an arrival (the map key), the new game or the victory. Quest
// beats are read from the journal instead (syncStoryQuestBeats).
func (g *MMGame) fireStoryTrigger(kind storyTriggerKind, key string, boss bool) {
	for i := range globalStoryEvents {
		e := &globalStoryEvents[i]
		if e.Trigger.On != kind || g.storyWasSeen(e.ID) || slices.Contains(g.storyQueue, e.ID) {
			continue
		}
		t := e.Trigger
		switch kind {
		case storyOnMonsterKilled:
			if !slices.Contains(t.Monsters, key) && !(t.Boss && boss) {
				continue
			}
		case storyOnRegionEntered:
			if !slices.Contains(t.Maps, key) {
				continue
			}
		}
		g.storyQueue = append(g.storyQueue, e.ID)
	}
}

// syncStoryQuestBeats queues every unseen quest beat whose quest the journal
// has brought to its trigger state. It reads the state, not a change of it, so
// no path that moves a quest - play, a paused journal, a talk, a load's
// reconciliation - can slip past it.
func (g *MMGame) syncStoryQuestBeats() {
	for i := range globalStoryEvents {
		e := &globalStoryEvents[i]
		if !e.Trigger.On.questKind() || g.storyWasSeen(e.ID) || slices.Contains(g.storyQueue, e.ID) {
			continue
		}
		if slices.ContainsFunc(e.Trigger.Quests, func(id string) bool { return g.storyQuestReached(e.Trigger.On, id) }) {
			g.storyQueue = append(g.storyQueue, e.ID)
		}
	}
}

func (k storyTriggerKind) questKind() bool {
	return k == storyOnQuestAccepted || k == storyOnQuestCompleted || k == storyOnQuestClaimed
}

// noteStoryKill fires the monster_killed events of a kill. A kill is a plot
// beat exactly when it advances quests (killCountsForProgress): never a
// runtime summon (a Dead Branch Enforcer is not his Enforcer), an ecology
// actor, prey, a caravan or the party's own ally.
func (g *MMGame) noteStoryKill(m *monster.Monster3D) {
	if !killCountsForProgress(m) {
		return
	}
	g.fireStoryTrigger(storyOnMonsterKilled, m.Key, m.IsBoss())
	g.noteWatcherKill(m)
}

// viewUncovered: the game view is on screen with nothing over it - in game,
// no loading, no open tabbed menu (it is not a modal layer) and no modal
// layer, a dialog included: it keeps the world running but covers the view.
func (g *MMGame) viewUncovered() bool {
	if g.appScreen != AppScreenInGame || g.menuOpen || g.loadingStall() {
		return false
	}
	gl := g.gameLoop
	stackSplit := gl != nil && gl.ui != nil && gl.ui.stackSplitPicker.open
	return topModalLayerFor(g, stackSplit) == modalLayerNone
}

// storyCalm: the view is uncovered and no fight is on. Story plates and the
// captain's cards wait for it.
func (g *MMGame) storyCalm() bool {
	return g.viewUncovered() && !g.partyInCombat()
}

// nextStoryEvent is the queue index of the event that may take the screen
// now, or -1. The victory screen freezes the world, so over it the queue plays
// at once and in order, whatever still fights - the ending, queued at victory,
// last; anywhere else the head waits for a calm frame.
func (g *MMGame) nextStoryEvent() int {
	if len(g.storyQueue) == 0 || g.storyScreen != nil || g.appScreen != AppScreenInGame {
		return -1
	}
	if topModalLayerFor(g, false) == modalLayerVictory {
		if g.loadingStall() {
			return -1
		}
		return 0
	}
	if g.storyCalm() {
		return 0
	}
	return -1
}

// tickStoryEvents opens the next queued event when it may take the screen.
func (g *MMGame) tickStoryEvents() {
	g.storyQueue = slices.DeleteFunc(g.storyQueue, func(id string) bool {
		return storyEvent(id) == nil // dropped from the catalog since the save
	})
	i := g.nextStoryEvent()
	if i < 0 {
		return
	}
	e := storyEvent(g.storyQueue[i])
	g.storyQueue = slices.Delete(g.storyQueue, i, i+1)
	// A quest finished while hidden was never announced: it is, now that the
	// journal shows it.
	var finished []*quests.Quest
	if g.questManager != nil {
		for _, id := range e.RevealsQuests {
			if q := g.questManager.GetQuest(id); q != nil && q.Completed && !g.questVisible(q) {
				finished = append(finished, q)
			}
		}
	}
	g.markStorySeen(e, g.currentCalendarDay())
	g.openStoryScreen(e)
	for _, q := range finished {
		g.presentQuestCompletion(q, "")
	}
}

func (g *MMGame) storyWasSeen(id string) bool {
	_, ok := g.storySeenDay(id)
	return ok
}

// storySeenDay is the calendar day an event was shown (0: unknown, an older
// save).
func (g *MMGame) storySeenDay(id string) (int, bool) {
	for _, s := range g.storySeen {
		if s.ID == id {
			return s.Day, true
		}
	}
	return 0, false
}

// markStorySeen records an event as shown on a calendar day (0: unknown, an
// older save), after every event shown before it, and applies its reveals.
func (g *MMGame) markStorySeen(e *StoryEventDef, day int) {
	if !g.storyWasSeen(e.ID) {
		g.storySeen = append(g.storySeen, StorySeenSave{ID: e.ID, Day: day})
	}
	for _, id := range e.RevealsQuests {
		g.revealQuest(id)
	}
}

func (g *MMGame) revealQuest(id string) {
	if g.storyRevealed == nil {
		g.storyRevealed = map[string]bool{}
	}
	g.storyRevealed[id] = true
}

// questVisible: whether the player is shown a quest - journal, banners,
// atlas markers and completion announcements. A hidden quest shows once a
// story event has revealed it.
func (g *MMGame) questVisible(q *quests.Quest) bool {
	return q != nil && q.Definition != nil && g.questDefVisible(q.ID, q.Definition)
}

// questIDVisible is questVisible by quest ID, for what names a quest before it
// is taken (tavern rumors): a hidden one stays unnamed until revealed.
func (g *MMGame) questIDVisible(id string) bool {
	if g.questManager == nil {
		return true
	}
	def := g.questManager.Definitions()[id]
	return def == nil || g.questDefVisible(id, def)
}

// questDefVisible is THE visibility rule: a hidden quest shows once revealed.
func (g *MMGame) questDefVisible(id string, def *quests.QuestDefinition) bool {
	return !def.Hidden || g.storyRevealed[id]
}

// resetStory clears the story state and the captain for a fresh run.
func (g *MMGame) resetStory() {
	g.closeStoryScreen()
	g.clearStoryState()
	g.resetWatcher()
}

func (g *MMGame) clearStoryState() {
	g.storySeen, g.storyQueue, g.storyRevealed = nil, nil, nil
}

// StorySeenSave is one shown story event and the calendar day it was shown.
type StorySeenSave struct {
	ID  string `json:"id"`
	Day int    `json:"day,omitempty"`
}

// storySaveState writes the persisted story fields into a save. A quest beat
// reached but not yet queued needs no copy: the journal state it is read from
// is saved, and the first frame after the load queues it.
func (g *MMGame) storySaveState(save *GameSave) {
	for id, ok := range g.storyRevealed {
		if ok {
			save.StoryRevealed = append(save.StoryRevealed, id)
		}
	}
	sort.Strings(save.StoryRevealed)
	save.StoryVersion = storyEventsVersion
	save.StorySeen = slices.Clone(g.storySeen)
	save.StoryQueue = slices.Clone(g.storyQueue)
}

// restoreStory restores the story state of a save. It runs before the journal
// is restored, so whether a completion the journal's reconciliation finds is
// told now or at its reveal follows this save's reveals. A save from before story events showed every
// quest: all hidden ones are revealed, and the beats it lived through are read
// from its journal once restored (resyncLegacyStory).
func (g *MMGame) restoreStory(save *GameSave) {
	g.clearStoryState() // the load's transient clear closed the screen; restoreWatcher resets him
	if save.StoryVersion < storyEventsVersion {
		if g.questManager != nil {
			for id, def := range g.questManager.Definitions() {
				if def.Hidden {
					g.revealQuest(id)
				}
			}
		}
		return
	}
	// Entries for a quest no longer hidden, or gone from the catalog, drop out.
	for _, id := range save.StoryRevealed {
		if !g.questIDVisible(id) {
			g.revealQuest(id)
		}
	}
	for _, s := range save.StorySeen {
		if e := storyEvent(s.ID); e != nil {
			g.markStorySeen(e, s.Day)
		}
	}
	for _, id := range save.StoryQueue {
		if storyEvent(id) != nil && !g.storyWasSeen(id) {
			g.storyQueue = append(g.storyQueue, id)
		}
	}
}

// resyncLegacyStory marks as seen what a save from before story events has
// lived through, read from its restored journal: the prologue, every quest
// beat it has passed and every beat whose revealed quests it has already
// completed (the order is carried out). Other kill and arrival beats stay
// pending; they play at the next such moment.
func (g *MMGame) resyncLegacyStory(save *GameSave) {
	if save.StoryVersion >= storyEventsVersion {
		return
	}
	completed := func(id string) bool { return g.storyQuestReached(storyOnQuestCompleted, id) }
	for i := range globalStoryEvents {
		e := &globalStoryEvents[i]
		passed := e.Trigger.On == storyOnNewGame ||
			e.Trigger.On.questKind() && slices.ContainsFunc(e.Trigger.Quests, func(id string) bool { return g.storyQuestReached(e.Trigger.On, id) }) ||
			len(e.RevealsQuests) > 0 && !slices.ContainsFunc(e.RevealsQuests, func(id string) bool { return !completed(id) })
		if passed {
			g.markStorySeen(e, 0)
		}
	}
}

// storyQuestReached: the journal has brought a quest to a quest trigger's
// state - THE test for live beats (syncStoryQuestBeats) and for what a legacy
// save has lived through.
func (g *MMGame) storyQuestReached(kind storyTriggerKind, id string) bool {
	if g.questManager == nil {
		return false
	}
	q := g.questManager.GetQuest(id)
	switch {
	case q == nil:
		return g.questCompleted(id) // gone from the journal: taken and finished
	case kind == storyOnQuestAccepted:
		return true
	case kind == storyOnQuestCompleted:
		return q.Completed
	default:
		return q.RewardsClaimed
	}
}
