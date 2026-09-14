package quests

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// QuestType represents the type of quest objective
type QuestType string

const (
	QuestTypeKill      QuestType = "kill"
	QuestTypeEncounter QuestType = "encounter" // Encounter quests auto-complete when all monsters are defeated
	// Interact quests count player interactions with a tagged object (e.g. closing
	// valves). The interaction tag is matched against the quest's TargetMonster
	// field; CurrentCount increments via OnInteract until TargetCount.
	QuestTypeInteract QuestType = "interact"
	// Future quest types can be added here:
	// QuestTypeCollect QuestType = "collect"
	// QuestTypeDeliver QuestType = "deliver"
	// QuestTypeExplore QuestType = "explore"
)

// QuestStatus represents the current status of a quest
type QuestStatus string

const (
	QuestStatusActive    QuestStatus = "active"
	QuestStatusCompleted QuestStatus = "completed"
	QuestStatusFailed    QuestStatus = "failed"
)

// QuestRewards defines the rewards for completing a quest
type QuestRewards struct {
	Gold        int `yaml:"gold"`
	ArenaPoints int `yaml:"arena_points,omitempty"`
	Experience  int `yaml:"experience"`
	// ItemPool is a set of items.yaml keys; claiming rolls ONE of them at
	// random. A repeatable errand pays a different draught each night without
	// needing a full loot table.
	ItemPool []string `yaml:"item_pool,omitempty"`
}

// QuestTileChange swaps one map tile when its quest completes (e.g. a bridge
// appearing over water). Tile is a tiles.yaml key; coordinates are tiles.
type QuestTileChange struct {
	Map  string `yaml:"map"`
	X    int    `yaml:"x"`
	Y    int    `yaml:"y"`
	Tile string `yaml:"tile"`
}

// QuestSpawn places a monster the moment its quest completes (the Brood Mother
// materializing in the emptied crater). Coordinates are map-local tiles; the
// open-world projection resolves them at runtime. Applied exactly once per
// playthrough - the spawned monster then lives and dies through the normal
// per-map monster save, never re-applied.
type QuestSpawn struct {
	// ID is the stable save identity of this spawn within its quest. It must not
	// depend on list order: reordering YAML must not re-fire a completed spawn.
	ID      string `yaml:"id"`
	Map     string `yaml:"map"`
	X       int    `yaml:"x"`
	Y       int    `yaml:"y"`
	Monster string `yaml:"monster"`
	// OnEntry defers the spawn while the party stands on the target map: it
	// fires on the next ARRIVAL there instead (the Enforcer surfaces only when
	// you come back down - the tavern rumor sends you).
	OnEntry bool `yaml:"on_entry,omitempty"`
}

// QuestDefinition is the YAML configuration for a quest
type QuestDefinition struct {
	Name          string    `yaml:"name"`
	Description   string    `yaml:"description"`
	Type          QuestType `yaml:"type"`
	TargetMonster string    `yaml:"target_monster"`
	// TargetMonsters extends TargetMonster to several normalized names when one
	// quest hunts a mixed roster (the cliff nests hold green AND gold dragons).
	TargetMonsters []string `yaml:"target_monsters,omitempty"`
	TargetCount    int      `yaml:"target_count"`
	// ProgressText is the tail of the progress line after "N/M" ("valves
	// closed", "arena duels won"). REQUIRED for interact quests: the derived
	// wording is kill-quest phrasing, and "interact" spans shutting valves,
	// lifting lamps and winning bouts - there is no verb that fits them all.
	ProgressText    string `yaml:"progress_text,omitempty"`
	Exterminate     bool   `yaml:"exterminate,omitempty"`
	IsStartingQuest bool   `yaml:"is_starting_quest"`
	// Repeatable errands are cleared again at every nightfall once claimed, so
	// their giver offers the same task the next night (see
	// refreshRepeatableQuests). Progress restarts from zero.
	Repeatable bool `yaml:"repeatable,omitempty"`
	// AutoClaim marks objective-only quests whose completion is itself the
	// reward. They finish without presenting an empty journal claim action.
	AutoClaim bool `yaml:"auto_claim,omitempty"`
	// EncounterOnly requires the kill source to name this quest. It is used for
	// authored encounter summons that share a display name with ordinary mobs.
	EncounterOnly bool `yaml:"encounter_only,omitempty"`
	// Victory marks the single quest whose completion wins the game.
	Victory bool `yaml:"victory,omitempty"`
	// TargetMap scopes the "no living targets left -> complete" check to one map,
	// for region quests whose monster type also lives elsewhere (e.g. the cliff
	// troll cull - trolls also roam the highlands). Empty = search every map,
	// which suits unique bosses (the lone Lich King).
	TargetMap string       `yaml:"target_map,omitempty"`
	Rewards   QuestRewards `yaml:"rewards"`
	// OnCompleteTiles are applied to the world the moment the quest completes
	// (and re-applied on save load), independent of turn-in.
	OnCompleteTiles []QuestTileChange `yaml:"on_complete_tiles,omitempty"`
	// OnCompleteSpawns place monsters once at the completion event (never
	// re-applied; see QuestSpawn).
	OnCompleteSpawns []QuestSpawn `yaml:"on_complete_spawns,omitempty"`
	// Optional location marker for quest objectives (tile coordinates)
	MarkerX   int    `yaml:"marker_x,omitempty"`   // X tile coordinate for quest marker
	MarkerY   int    `yaml:"marker_y,omitempty"`   // Y tile coordinate for quest marker
	MarkerMap string `yaml:"marker_map,omitempty"` // Map key where marker should appear (empty = current map)
}

// NormalizeTarget converts a display name or interaction tag to the canonical
// quest-target key used by loaded definitions.
func NormalizeTarget(target string) string {
	return strings.ToLower(strings.Join(strings.Fields(target), "_"))
}

// MatchesTarget reports whether a monster name / interaction tag is one of
// this quest's targets: the single TargetMonster or any TargetMonsters entry.
// Every target-matching site must go through here.
func (d *QuestDefinition) MatchesTarget(tag string) bool {
	if d == nil {
		return false
	}
	tag = NormalizeTarget(tag)
	if tag == "" {
		return false
	}
	if d.TargetMonster == tag {
		return true
	}
	for _, t := range d.TargetMonsters {
		if t == tag {
			return true
		}
	}
	return false
}

// Quest represents an active quest with progress tracking
type Quest struct {
	ID           string
	Definition   *QuestDefinition
	Status       QuestStatus
	CurrentCount int // Current progress towards target
	// DynamicTarget snapshots a per-instance goal count at accept time (0 = unset,
	// fall back to the static Definition.TargetCount). Exterminate quests capture
	// the live target census when accepted, so the journal counts the map's real
	// population instead of a hand-maintained number.
	DynamicTarget  int
	Completed      bool
	RewardsClaimed bool
}

func (q *Quest) complete(autoClaim bool) {
	q.Completed = true
	q.Status = QuestStatusCompleted
	if autoClaim || (q.Definition != nil && q.Definition.AutoClaim) {
		q.RewardsClaimed = true
	}
}

// Target is the effective goal count: the per-instance DynamicTarget snapshot
// when set, else the static definition count.
func (q *Quest) Target() int {
	if q.DynamicTarget > 0 {
		return q.DynamicTarget
	}
	return q.Definition.TargetCount
}

// QuestConfig holds all quest definitions loaded from YAML
type QuestConfig struct {
	Quests map[string]*QuestDefinition `yaml:"quests"`
}

// QuestManager handles all quest-related operations
type QuestManager struct {
	config       *QuestConfig
	activeQuests map[string]*Quest // Map of quest ID to active quest
	mu           sync.RWMutex
}

// Global quest manager instance
var GlobalQuestManager *QuestManager

// LoadQuestConfig loads quest definitions from YAML file
func LoadQuestConfig(filepath string) (*QuestConfig, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read quest config: %w", err)
	}

	var config QuestConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse quest config: %w", err)
	}
	if err := validateQuestConfig(&config); err != nil {
		return nil, err
	}
	return &config, nil
}

// ArenaDuelTag is the interact tag a won champion duel credits - the one tag
// produced by CODE rather than by authored content (a prop declares its own).
// It lives here so the quest catalog can be validated against it at load.
const ArenaDuelTag = "arena_duel"

func validateQuestConfig(config *QuestConfig) error {
	if config == nil || config.Quests == nil {
		return fmt.Errorf("quests config has no quests")
	}
	victoryQuest := ""
	for id, def := range config.Quests {
		if def == nil {
			return fmt.Errorf("quest %q has empty definition", id)
		}
		switch def.Type {
		case QuestTypeKill, QuestTypeEncounter, QuestTypeInteract:
		default:
			return fmt.Errorf("quest %q has unknown type %q", id, def.Type)
		}
		def.TargetMonster = NormalizeTarget(def.TargetMonster)
		seenTargets := make(map[string]bool, len(def.TargetMonsters)+1)
		if def.TargetMonster != "" {
			seenTargets[def.TargetMonster] = true
		}
		for i, target := range def.TargetMonsters {
			target = NormalizeTarget(target)
			if target == "" {
				return fmt.Errorf("quest %q target_monsters[%d] is empty", id, i)
			}
			if seenTargets[target] {
				return fmt.Errorf("quest %q repeats target %q", id, target)
			}
			seenTargets[target] = true
			def.TargetMonsters[i] = target
		}
		if def.Type != QuestTypeEncounter {
			if len(seenTargets) == 0 {
				return fmt.Errorf("quest %q has no target", id)
			}
			if def.TargetCount <= 0 {
				return fmt.Errorf("quest %q target_count must be positive", id)
			}
		}
		def.TargetMap = strings.TrimSpace(def.TargetMap)
		if def.Exterminate && (def.Type != QuestTypeKill || def.TargetMap == "") {
			return fmt.Errorf("quest %q: exterminate requires type kill and target_map", id)
		}
		if def.EncounterOnly && def.Type != QuestTypeKill {
			return fmt.Errorf("quest %q: encounter_only requires type kill", id)
		}
		def.ProgressText = strings.TrimSpace(def.ProgressText)
		if def.Type == QuestTypeInteract {
			// No default: an interact quest that inherits a verb reads as
			// nonsense ("3/3 arena duels closed" shipped for a week).
			if def.ProgressText == "" {
				return fmt.Errorf("quest %q: an interact quest must author progress_text (e.g. \"valves closed\")", id)
			}
			// The props credit through the SINGLE tag (handleQuestPropInteract
			// passes TargetMonster); a target_monsters list would consume the
			// prop and advance nothing.
			if def.TargetMonster == "" {
				return fmt.Errorf("quest %q: an interact quest must set target_monster (target_monsters alone credits nothing)", id)
			}
			// The interact hooks credit without a map (a prop is a physical object
			// on one map, a duel is fought where the pit is), so target_map would
			// be read by nothing.
			if def.TargetMap != "" {
				return fmt.Errorf("quest %q: target_map does not apply to an interact quest", id)
			}
		}
		for i, change := range def.OnCompleteTiles {
			change.Map = strings.TrimSpace(change.Map)
			change.Tile = strings.TrimSpace(change.Tile)
			if change.Map == "" || change.Tile == "" {
				return fmt.Errorf("quest %q on_complete_tiles[%d] needs map and tile", id, i)
			}
			def.OnCompleteTiles[i] = change
		}
		spawnIDs := make(map[string]bool, len(def.OnCompleteSpawns))
		for i, spawn := range def.OnCompleteSpawns {
			spawn.ID = strings.TrimSpace(spawn.ID)
			spawn.Map = strings.TrimSpace(spawn.Map)
			spawn.Monster = strings.TrimSpace(spawn.Monster)
			if spawn.ID == "" || spawn.Map == "" || spawn.Monster == "" {
				return fmt.Errorf("quest %q on_complete_spawns[%d] needs id, map and monster", id, i)
			}
			if spawnIDs[spawn.ID] {
				return fmt.Errorf("quest %q repeats on_complete_spawns id %q", id, spawn.ID)
			}
			spawnIDs[spawn.ID] = true
			def.OnCompleteSpawns[i] = spawn
		}
		if def.AutoClaim && (def.Rewards.Gold != 0 || def.Rewards.Experience != 0 || def.Rewards.ArenaPoints != 0) {
			return fmt.Errorf("quest %q: auto_claim is reserved for rewardless objective quests", id)
		}
		if !def.Victory {
			continue
		}
		if victoryQuest != "" {
			return fmt.Errorf("quests %q and %q are both marked victory", victoryQuest, id)
		}
		if !def.AutoClaim {
			return fmt.Errorf("victory quest %q must be auto_claim", id)
		}
		victoryQuest = id
	}
	return nil
}

// NewQuestManager creates a new quest manager with loaded config
func NewQuestManager(config *QuestConfig) *QuestManager {
	return &QuestManager{
		config:       config,
		activeQuests: make(map[string]*Quest),
	}
}

// InitializeStartingQuests activates all quests marked as starting quests
func (qm *QuestManager) InitializeStartingQuests() {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	for id, def := range qm.config.Quests {
		if def.IsStartingQuest {
			qm.activeQuests[id] = &Quest{
				ID:           id,
				Definition:   def,
				Status:       QuestStatusActive,
				CurrentCount: 0,
				Completed:    false,
			}
		}
	}
}

// Reset clears all quest progress and re-initializes starting quests.
func (qm *QuestManager) Reset() {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	qm.activeQuests = make(map[string]*Quest)
	for id, def := range qm.config.Quests {
		if def.IsStartingQuest {
			qm.activeQuests[id] = &Quest{
				ID:           id,
				Definition:   def,
				Status:       QuestStatusActive,
				CurrentCount: 0,
				Completed:    false,
			}
		}
	}
}

// ActivateQuest activates a quest by ID
func (qm *QuestManager) ActivateQuest(questID string) error {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	def, exists := qm.config.Quests[questID]
	if !exists {
		return fmt.Errorf("quest not found: %s", questID)
	}

	if _, active := qm.activeQuests[questID]; active {
		return fmt.Errorf("quest already active: %s", questID)
	}

	qm.activeQuests[questID] = &Quest{
		ID:           questID,
		Definition:   def,
		Status:       QuestStatusActive,
		CurrentCount: 0,
		Completed:    false,
	}

	return nil
}

// MarkCompleted forces an active quest to its completed state - used when a kill
// quest's targets are already gone (slain before the quest was taken, or fewer
// existed than TargetCount), so it can still be turned in. No-op if not active.
func (qm *QuestManager) MarkCompleted(questID string) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if quest, ok := qm.activeQuests[questID]; ok {
		quest.CurrentCount = quest.Target()
		quest.complete(false)
	}
}

// SetDynamicTarget snapshots a per-instance goal count (exterminate quests
// capture the live target census at accept). No-op if not active.
func (qm *QuestManager) SetDynamicTarget(questID string, target int) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	if quest, ok := qm.activeQuests[questID]; ok {
		quest.DynamicTarget = target
	}
}

// SetCurrentCount updates a quest counter while preserving its status. Count is
// clamped to [0, target_count] so dynamic progress displays cannot exceed 100%.
func (qm *QuestManager) SetCurrentCount(questID string, count int) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	quest, ok := qm.activeQuests[questID]
	if !ok || quest.Definition == nil {
		return
	}
	if count < 0 {
		count = 0
	}
	if max := quest.Target(); max > 0 && count > max {
		count = max
	}
	quest.CurrentCount = count
}

// OnMonsterKilled updates quest progress when a monster is killed. mapKey is
// the map the kill happened on: a quest with TargetMap set only counts kills
// there (forest wolves don't advance on a city wolf). Empty mapKey counts
// everywhere (callers without map context).
// Returns a list of quests that were completed by this kill.
func (qm *QuestManager) OnMonsterKilled(monsterType, mapKey string) []*Quest {
	return qm.OnMonsterKilledFromSource(monsterType, mapKey, "")
}

// OnMonsterKilledFromSource updates kill quests and supplies the authored
// encounter quest ID, if any. EncounterOnly quests ignore all other kills.
func (qm *QuestManager) OnMonsterKilledFromSource(monsterType, mapKey, sourceQuestID string) []*Quest {
	_, completed := qm.advanceCountedQuests(QuestTypeKill, monsterType, mapKey, sourceQuestID, "")
	return completed
}

// OnInteract advances active interact-quests whose tag (TargetMonster) matches -
// e.g. closing a valve calls OnInteract("valve"). Mirrors OnMonsterKilled: bumps
// CurrentCount and completes at TargetCount.
//
// It reports BOTH lists - what moved and what finished - because the matching
// rules (type and tag; interact quests carry neither encounter_only nor
// target_map) live here. A caller that speaks about progress must be told what
// advanced, never re-derive it: the two answers drift the moment a rule is added.
func (qm *QuestManager) OnInteract(tag string) (advanced, completed []*Quest) {
	return qm.advanceCountedQuests(QuestTypeInteract, tag, "", "", "")
}

// AdvanceInteractQuest credits ONE named interact quest with ONE tag, for a prop
// that knows both which errand it belongs to and what it is. Separate from
// OnInteract because a tag bump is a broadcast: a prop that turned out not to
// credit its own quest would still have moved every other quest sharing the tag,
// and since the prop is not consumed in that case the player could pump those
// counters by re-opening it. Same rules, same body - only the audience narrows.
func (qm *QuestManager) AdvanceInteractQuest(questID, tag string) (advanced bool, completed []*Quest) {
	moved, completed := qm.advanceCountedQuests(QuestTypeInteract, tag, "", "", questID)
	return len(moved) > 0, completed
}

// advanceCountedQuests bumps CurrentCount on every active quest of the given type
// whose TargetMonster tag matches, completing it at TargetCount. Shared by the
// kill and interact progress hooks (OnMonsterKilled / OnInteract).
// onlyQuestID, when set, narrows the bump to that one quest: every rule below
// still applies, the audience is just a single errand (a prop crediting its own).
func (qm *QuestManager) advanceCountedQuests(qType QuestType, tag, mapKey, sourceQuestID, onlyQuestID string) (advanced, completedQuests []*Quest) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	for _, quest := range qm.activeQuests {
		if quest.Status != QuestStatusActive || quest.Definition.Type != qType {
			continue
		}
		if onlyQuestID != "" && quest.ID != onlyQuestID {
			continue
		}
		if !quest.Definition.MatchesTarget(tag) {
			continue
		}
		if quest.Definition.EncounterOnly && quest.ID != sourceQuestID {
			continue
		}
		if quest.Definition.TargetMap != "" && mapKey != "" && quest.Definition.TargetMap != mapKey {
			continue
		}
		quest.CurrentCount++
		advanced = append(advanced, quest)
		// Exterminate quests never complete on the kill quota - completion is
		// owned by the living-count check (completeKillQuestIfCleared at 0 alive),
		// so killing N of M never finishes early when M != the static target.
		if !quest.Definition.Exterminate && quest.CurrentCount >= quest.Target() {
			quest.complete(false)
			completedQuests = append(completedQuests, quest)
		}
	}
	return advanced, completedQuests
}

// VictoryCompleted reports whether the data-authored victory quest is complete.
func (qm *QuestManager) VictoryCompleted() bool {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	for _, quest := range qm.activeQuests {
		if quest.Definition != nil && quest.Definition.Victory && quest.Status == QuestStatusCompleted {
			return true
		}
	}
	return false
}

// ClaimRewards marks a quest's rewards as claimed and returns the rewards
func (qm *QuestManager) ClaimRewards(questID string) (*QuestRewards, error) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	quest, exists := qm.activeQuests[questID]
	if !exists {
		return nil, fmt.Errorf("quest not found: %s", questID)
	}

	if !quest.Completed {
		return nil, fmt.Errorf("quest not completed: %s", questID)
	}

	if quest.RewardsClaimed {
		return nil, fmt.Errorf("rewards already claimed: %s", questID)
	}

	quest.RewardsClaimed = true
	return &quest.Definition.Rewards, nil
}

// GetActiveQuests returns all active quests
func (qm *QuestManager) GetActiveQuests() []*Quest {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	quests := make([]*Quest, 0, len(qm.activeQuests))
	for _, quest := range qm.activeQuests {
		if quest.Status == QuestStatusActive {
			quests = append(quests, quest)
		}
	}
	return quests
}

// GetCompletedQuests returns all completed quests (with unclaimed rewards)
func (qm *QuestManager) GetCompletedQuests() []*Quest {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	quests := make([]*Quest, 0)
	for _, quest := range qm.activeQuests {
		if quest.Status == QuestStatusCompleted && !quest.RewardsClaimed {
			quests = append(quests, quest)
		}
	}
	return quests
}

// GetAllQuests returns all quests (active and completed)
func (qm *QuestManager) GetAllQuests() []*Quest {
	qm.mu.RLock()
	defer qm.mu.RUnlock()

	quests := make([]*Quest, 0, len(qm.activeQuests))
	for _, quest := range qm.activeQuests {
		quests = append(quests, quest)
	}
	return quests
}

// EachQuest visits every quest the party holds, without building a slice - for
// the per-frame readers (the banner watcher runs 60x a second and would
// otherwise allocate and copy the whole journal on every frame that changed
// nothing). Visit order is map order: a caller that needs determinism must sort
// what it COLLECTS, not rely on this. The callback runs under the read lock, so
// it must not call back into the manager.
func (qm *QuestManager) EachQuest(visit func(*Quest)) {
	if visit == nil {
		return
	}
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	for _, quest := range qm.activeQuests {
		visit(quest)
	}
}

// Definitions returns every quest definition in the loaded config, keyed by
// quest ID - including quests not yet activated (for load-time validation).
func (qm *QuestManager) Definitions() map[string]*QuestDefinition {
	return qm.config.Quests
}

// GetQuest returns a specific quest by ID
func (qm *QuestManager) GetQuest(questID string) *Quest {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	return qm.activeQuests[questID]
}

// GetProgressString returns the formatted progress line. Authored wording wins
// (progress_text); otherwise it is derived from the target, whose TargetMonster
// is a content KEY ("elder_dragon") - player-facing text must never show
// underscores.
func (q *Quest) GetProgressString() string {
	if text := q.Definition.ProgressText; text != "" {
		return fmt.Sprintf("%d/%d %s", q.CurrentCount, q.Target(), text)
	}
	if len(q.Definition.TargetMonsters) > 0 {
		return fmt.Sprintf("%d/%d targets killed", q.CurrentCount, q.Target())
	}
	target := strings.ReplaceAll(q.Definition.TargetMonster, "_", " ")
	switch q.Definition.Type {
	case QuestTypeKill:
		return fmt.Sprintf("%d/%d %ss killed", q.CurrentCount, q.Target(), target)
	}
	// No authored line: LoadQuestConfig requires progress_text, so this is a
	// definition built in code (a fixture, a runtime errand). A bare count still
	// reads - an empty string would print "You heave the valve shut. ()".
	if target != "" {
		return fmt.Sprintf("%d/%d %s", q.CurrentCount, q.Target(), target)
	}
	return fmt.Sprintf("%d/%d", q.CurrentCount, q.Target())
}

// GetStatusString returns a human-readable status
func (q *Quest) GetStatusString() string {
	switch q.Status {
	case QuestStatusActive:
		return "In Progress"
	case QuestStatusCompleted:
		if q.RewardsClaimed {
			return "Completed"
		}
		return "Complete! (Claim Reward)"
	case QuestStatusFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

// CreateEncounterQuest creates and activates a quest for an encounter
// Returns the quest ID for linking to the encounter rewards
func (qm *QuestManager) CreateEncounterQuest(questID, name, description string, gold, experience int) string {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	// Don't create if already exists
	if _, exists := qm.activeQuests[questID]; exists {
		return questID
	}

	// Create a dynamic quest definition for the encounter
	def := &QuestDefinition{
		Name:        name,
		Description: description,
		Type:        QuestTypeEncounter,
		Rewards: QuestRewards{
			Gold:       gold,
			Experience: experience,
		},
	}

	qm.activeQuests[questID] = &Quest{
		ID:           questID,
		Definition:   def,
		Status:       QuestStatusActive,
		CurrentCount: 0,
		Completed:    false,
	}

	return questID
}

// CompleteEncounterQuest marks an encounter quest as completed and auto-claims rewards
// Returns the rewards if successful, nil if quest not found or already completed
func (qm *QuestManager) CompleteEncounterQuest(questID string) *QuestRewards {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	quest, exists := qm.activeQuests[questID]
	if !exists {
		return nil
	}

	// Only complete encounter type quests
	if quest.Definition.Type != QuestTypeEncounter {
		return nil
	}

	// Already completed
	if quest.Completed {
		return nil
	}

	// Mark as completed and auto-claim
	quest.complete(true)

	return &quest.Definition.Rewards
}

// RemoveQuest removes a quest from the active quests (for cleanup after encounter quests)
func (qm *QuestManager) RemoveQuest(questID string) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	delete(qm.activeQuests, questID)
}

// RestoreQuestProgress restores quest state from a save file
func (qm *QuestManager) RestoreQuestProgress(questID string, status QuestStatus, currentCount, dynamicTarget int, rewardsClaimed bool) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	quest, exists := qm.activeQuests[questID]
	if !exists {
		// Quest might not be activated yet, try to activate it first
		if def, ok := qm.config.Quests[questID]; ok {
			quest = &Quest{
				ID:         questID,
				Definition: def,
			}
			qm.activeQuests[questID] = quest
		} else {
			return // Quest definition not found
		}
	}

	quest.Status = status
	quest.CurrentCount = currentCount
	quest.DynamicTarget = dynamicTarget
	quest.Completed = (status == QuestStatusCompleted)
	quest.RewardsClaimed = rewardsClaimed || (quest.Completed && quest.Definition.AutoClaim)
}
