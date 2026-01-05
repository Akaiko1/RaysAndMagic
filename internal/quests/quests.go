package quests

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// QuestType represents the type of quest objective
type QuestType string

const (
	QuestTypeKill      QuestType = "kill"
	QuestTypeEncounter QuestType = "encounter" // Encounter quests auto-complete when all monsters are defeated
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
	Gold       int `yaml:"gold"`
	Experience int `yaml:"experience"`
	// Future: Items []string `yaml:"items"`
}

// QuestDefinition is the YAML configuration for a quest
type QuestDefinition struct {
	Name            string       `yaml:"name"`
	Description     string       `yaml:"description"`
	Type            QuestType    `yaml:"type"`
	TargetMonster   string       `yaml:"target_monster"`
	TargetCount     int          `yaml:"target_count"`
	IsStartingQuest bool         `yaml:"is_starting_quest"`
	Rewards         QuestRewards `yaml:"rewards"`
	// Optional location marker for quest objectives (tile coordinates)
	MarkerX   int    `yaml:"marker_x,omitempty"`   // X tile coordinate for quest marker
	MarkerY   int    `yaml:"marker_y,omitempty"`   // Y tile coordinate for quest marker
	MarkerMap string `yaml:"marker_map,omitempty"` // Map key where marker should appear (empty = current map)
}

// Quest represents an active quest with progress tracking
type Quest struct {
	ID             string
	Definition     *QuestDefinition
	Status         QuestStatus
	CurrentCount   int // Current progress towards target
	Completed      bool
	RewardsClaimed bool
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

	return &config, nil
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

// OnMonsterKilled updates quest progress when a monster is killed
// Returns a list of quests that were completed by this kill
func (qm *QuestManager) OnMonsterKilled(monsterType string) []*Quest {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	var completedQuests []*Quest

	for _, quest := range qm.activeQuests {
		if quest.Status != QuestStatusActive {
			continue
		}

		if quest.Definition.Type != QuestTypeKill {
			continue
		}

		if quest.Definition.TargetMonster != monsterType {
			continue
		}

		// Increment progress
		quest.CurrentCount++

		// Check if quest is now complete
		if quest.CurrentCount >= quest.Definition.TargetCount {
			quest.Completed = true
			quest.Status = QuestStatusCompleted
			completedQuests = append(completedQuests, quest)
		}
	}

	return completedQuests
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

// GetQuest returns a specific quest by ID
func (qm *QuestManager) GetQuest(questID string) *Quest {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	return qm.activeQuests[questID]
}

// GetProgressString returns a formatted progress string for a kill quest
func (q *Quest) GetProgressString() string {
	if q.Definition.Type == QuestTypeKill {
		return fmt.Sprintf("%d/%d %ss killed", q.CurrentCount, q.Definition.TargetCount, q.Definition.TargetMonster)
	}
	return ""
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
	quest.Completed = true
	quest.Status = QuestStatusCompleted
	quest.RewardsClaimed = true

	return &quest.Definition.Rewards
}

// RemoveQuest removes a quest from the active quests (for cleanup after encounter quests)
func (qm *QuestManager) RemoveQuest(questID string) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	delete(qm.activeQuests, questID)
}

// RestoreQuestProgress restores quest state from a save file
func (qm *QuestManager) RestoreQuestProgress(questID string, status QuestStatus, currentCount int, rewardsClaimed bool) {
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
	quest.RewardsClaimed = rewardsClaimed
	quest.Completed = (status == QuestStatusCompleted)
}
