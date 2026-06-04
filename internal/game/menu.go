package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/spells"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

// ErrExit is returned from the game loop to request a clean exit
var ErrExit = errors.New("exit game")

// DefaultSavePath is the default file used for saving/loading
const DefaultSavePath = "savegame.json"

// slotPath returns a filename for a numbered save slot (0-based index)
func slotPath(slot int) string { return storage.AppSavePath(fmt.Sprintf("save%d.json", slot+1)) }

// mainMenuOptions defines the visible options in the ESC menu
var mainMenuOptions = []string{"Continue", "Save", "Load", "High Scores", "Exit"}

// GameSave captures minimal persistent state for save/load
type GameSave struct {
	MapKey           string                   `json:"map_key"`
	PlayerX          float64                  `json:"player_x"`
	PlayerY          float64                  `json:"player_y"`
	PlayerAngle      float64                  `json:"player_angle"`
	TurnBased        bool                     `json:"turn_based"`
	SaveName         string                   `json:"save_name,omitempty"`
	SavedAt          string                   `json:"saved_at"`
	Party            PartySave                `json:"party"`
	Monsters         []MonsterSave            `json:"monsters"`
	MapMonsters      map[string][]MonsterSave `json:"map_monsters,omitempty"`
	NPCStates        []NPCSave                `json:"npc_states"`
	Quests           []QuestSave              `json:"quests,omitempty"`
	GroundContainers []GroundContainerSave    `json:"ground_containers,omitempty"`
	// PendingLevelUpChoices preserves unconsumed skill/spell choices from
	// level-ups. Options are rebuilt from class+level on load, so we only
	// need to remember which character is owed a choice at which level.
	PendingLevelUpChoices []PendingLevelUpChoiceSave `json:"pending_level_up_choices,omitempty"`
	PlayedTimeNs          int64                      `json:"played_time_ns,omitempty"` // Elapsed play time in nanoseconds

	// Turn-based state
	CurrentTurn           int  `json:"current_turn,omitempty"`
	PartyActionsUsed      int  `json:"party_actions_used,omitempty"`
	TurnBasedMoveCooldown int  `json:"turn_based_move_cooldown,omitempty"`
	TurnBasedRotCooldown  int  `json:"turn_based_rot_cooldown,omitempty"`
	MonsterTurnResolved   bool `json:"monster_turn_resolved,omitempty"`
	TurnBasedSpRegenCount int  `json:"turn_based_sp_regen_count,omitempty"`

	// Utility/buff state
	TorchLightActive       bool    `json:"torch_light_active,omitempty"`
	TorchLightDuration     int     `json:"torch_light_duration,omitempty"`
	TorchLightRadius       float64 `json:"torch_light_radius,omitempty"`
	WizardEyeActive        bool    `json:"wizard_eye_active,omitempty"`
	WizardEyeDuration      int     `json:"wizard_eye_duration,omitempty"`
	WalkOnWaterActive      bool    `json:"walk_on_water_active,omitempty"`
	WalkOnWaterDuration    int     `json:"walk_on_water_duration,omitempty"`
	BlessActive            bool    `json:"bless_active,omitempty"`
	BlessDuration          int     `json:"bless_duration,omitempty"`
	BlessStatBonus         int     `json:"bless_stat_bonus,omitempty"`
	DayGodsActive          bool    `json:"day_gods_active,omitempty"`
	DayGodsDuration        int     `json:"day_gods_duration,omitempty"`
	DayGodsResistPct       int     `json:"day_gods_resist_pct,omitempty"`
	HourPowerActive        bool    `json:"hour_power_active,omitempty"`
	HourPowerDuration      int     `json:"hour_power_duration,omitempty"`
	HourPowerOutBonus      int     `json:"hour_power_out_bonus,omitempty"`
	HourPowerInReduce      int     `json:"hour_power_in_reduce,omitempty"`
	WaterBreathingActive   bool    `json:"water_breathing_active,omitempty"`
	WaterBreathingDuration int     `json:"water_breathing_duration,omitempty"`
	UnderwaterReturnX      float64 `json:"underwater_return_x,omitempty"`
	UnderwaterReturnY      float64 `json:"underwater_return_y,omitempty"`
	UnderwaterReturnMap    string  `json:"underwater_return_map,omitempty"`
	StatBonus              int     `json:"stat_bonus,omitempty"`

	// MapReturnPoses remembers where the party entered each map via a gate, so a
	// return trip drops them at the doorway rather than the map's spawn tile.
	MapReturnPoses map[string]MapPose `json:"map_return_poses,omitempty"`
}

// QuestSave captures quest progress for save/load
type QuestSave struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	CurrentCount   int    `json:"current_count"`
	RewardsClaimed bool   `json:"rewards_claimed"`
}

type PartySave struct {
	Gold      int             `json:"gold"`
	Food      int             `json:"food"`
	Inventory []items.Item    `json:"inventory"`
	Members   []CharacterSave `json:"members"`
	Reserve   []CharacterSave `json:"reserve,omitempty"`
	Captive   []CharacterSave `json:"captive,omitempty"`
}

type CharacterSave struct {
	Name                  string             `json:"name"`
	Class                 int                `json:"class"`
	Promotion             int                `json:"promotion,omitempty"`
	Level                 int                `json:"level"`
	Experience            int                `json:"experience"`
	HitPoints             int                `json:"hit_points"`
	MaxHitPoints          int                `json:"max_hit_points"`
	SpellPoints           int                `json:"spell_points"`
	MaxSpellPoints        int                `json:"max_spell_points"`
	Might                 int                `json:"might"`
	Intellect             int                `json:"intellect"`
	Personality           int                `json:"personality"`
	Endurance             int                `json:"endurance"`
	Accuracy              int                `json:"accuracy"`
	Speed                 int                `json:"speed"`
	Luck                  int                `json:"luck"`
	FreeStatPoints        int                `json:"free_stat_points"`
	OwedLevelChoices      []int              `json:"owed_level_choices,omitempty"`
	Conditions            []int              `json:"conditions"`
	Skills                []SkillEntry       `json:"skills"`
	MagicSchools          []MagicSchoolEntry `json:"magic_schools"`
	Equipment             []EquipmentEntry   `json:"equipment"`
	PoisonFramesRemaining int                `json:"poison_frames_remaining,omitempty"`
	// ActionsRemaining preserves mid-round turn-based state so save/reload
	// can't be used to refill action slots. Omitted from real-time saves
	// (value will simply be 0; ignored when turn-based mode is off).
	ActionsRemaining int `json:"actions_remaining,omitempty"`
}

type SkillEntry struct {
	Type    int `json:"type"`
	Level   int `json:"level"`
	Mastery int `json:"mastery"`
}

// PendingLevelUpChoiceSave records that party member CharIndex has earned a
// level-up choice at Level but hasn't picked one yet. Options themselves are
// not stored — they're rebuilt from the character's class config on load.
type PendingLevelUpChoiceSave struct {
	CharIndex int `json:"char_index"`
	Level     int `json:"level"`
}

type MagicSchoolEntry struct {
	School      string   `json:"school"`
	Level       int      `json:"level"`
	Mastery     int      `json:"mastery"`
	KnownSpells []string `json:"known_spells"`
}

type EquipmentEntry struct {
	Slot int        `json:"slot"`
	Item items.Item `json:"item"`
}

// GroundContainerSave captures an on-floor reward container (loot bag or
// treasure chest) for save/load. Kind drives the presentation defaults; the
// rest of the fields are the runtime state.
type GroundContainerSave struct {
	Kind           int          `json:"kind"`
	ID             string       `json:"id,omitempty"`
	MapKey         string       `json:"map_key,omitempty"`
	X              float64      `json:"x"`
	Y              float64      `json:"y"`
	Gold           int          `json:"gold"`
	Items          []items.Item `json:"items,omitempty"`
	Sprite         string       `json:"sprite,omitempty"`
	SizeMultiplier float64      `json:"size_multiplier"`
}

type MonsterSave struct {
	Key                  string               `json:"key"`
	Name                 string               `json:"name"`
	X                    float64              `json:"x"`
	Y                    float64              `json:"y"`
	HitPoints            int                  `json:"hit_points"`
	Charmed              bool                 `json:"charmed,omitempty"`
	CharmFramesRemaining int                  `json:"charm_frames_remaining,omitempty"`
	IsEncounterMonster   bool                 `json:"is_encounter_monster,omitempty"`
	EncounterID          int                  `json:"encounter_id,omitempty"`
	EncounterRewards     *EncounterRewardSave `json:"encounter_rewards,omitempty"`
}

type EncounterRewardSave struct {
	Gold              int                       `json:"gold"`
	Experience        int                       `json:"experience"`
	CompletionMessage string                    `json:"completion_message,omitempty"`
	QuestID           string                    `json:"quest_id,omitempty"`
	TreasureChest     *TreasureChestRewardSave  `json:"treasure_chest,omitempty"`
	TreasureChests    []TreasureChestRewardSave `json:"treasure_chests,omitempty"`
}

type TreasureChestRewardSave struct {
	ID                string   `json:"id,omitempty"`
	Map               string   `json:"map,omitempty"`
	TileX             int      `json:"tile_x"`
	TileY             int      `json:"tile_y"`
	Sprite            string   `json:"sprite,omitempty"`
	SizeMultiplier    float64  `json:"size_multiplier,omitempty"`
	RandomWeaponCount int      `json:"random_weapon_count,omitempty"`
	Items             []string `json:"items,omitempty"`
	Weapons           []string `json:"weapons,omitempty"`
	Gold              int      `json:"gold,omitempty"`
	CompletionMessage string   `json:"completion_message,omitempty"`
}

func treasureChestRewardToSave(reward *monster.TreasureChestReward) *TreasureChestRewardSave {
	if reward == nil {
		return nil
	}
	return &TreasureChestRewardSave{
		ID:                reward.ID,
		Map:               reward.Map,
		TileX:             reward.TileX,
		TileY:             reward.TileY,
		Sprite:            reward.Sprite,
		SizeMultiplier:    reward.SizeMultiplier,
		RandomWeaponCount: reward.RandomWeaponCount,
		Items:             append([]string(nil), reward.Items...),
		Weapons:           append([]string(nil), reward.Weapons...),
		Gold:              reward.Gold,
		CompletionMessage: reward.CompletionMessage,
	}
}

func treasureChestRewardFromSave(save *TreasureChestRewardSave) *monster.TreasureChestReward {
	if save == nil {
		return nil
	}
	return &monster.TreasureChestReward{
		ID:                save.ID,
		Map:               save.Map,
		TileX:             save.TileX,
		TileY:             save.TileY,
		Sprite:            save.Sprite,
		SizeMultiplier:    save.SizeMultiplier,
		RandomWeaponCount: save.RandomWeaponCount,
		Items:             append([]string(nil), save.Items...),
		Weapons:           append([]string(nil), save.Weapons...),
		Gold:              save.Gold,
		CompletionMessage: save.CompletionMessage,
	}
}

func encounterRewardsFromSave(save *EncounterRewardSave) *monster.EncounterRewards {
	if save == nil {
		return nil
	}
	rewards := &monster.EncounterRewards{
		Gold:              save.Gold,
		Experience:        save.Experience,
		CompletionMessage: save.CompletionMessage,
		QuestID:           save.QuestID,
		TreasureChest:     treasureChestRewardFromSave(save.TreasureChest),
	}
	for _, chestSave := range save.TreasureChests {
		if chest := treasureChestRewardFromSave(&chestSave); chest != nil {
			rewards.TreasureChests = append(rewards.TreasureChests, *chest)
		}
	}
	return rewards
}

// NPCSave tracks persistent NPC flags across maps
type NPCSave struct {
	MapKey  string `json:"map_key"`
	Name    string `json:"name"`
	Visited bool   `json:"visited"`
}

// SaveSummary is lightweight info used for menu display
type SaveSummary struct {
	Exists    bool
	SavedAt   string
	MapKey    string
	TurnBased bool
	Name      string
}

// GetSaveSlotSummary reads minimal info from a save slot for UI display
func GetSaveSlotSummary(slot int) SaveSummary {
	path := slotPath(slot)
	f, err := os.Open(path)
	if err != nil {
		return SaveSummary{Exists: false}
	}
	defer f.Close()
	var s GameSave
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return SaveSummary{Exists: false}
	}
	return SaveSummary{Exists: true, SavedAt: s.SavedAt, MapKey: s.MapKey, TurnBased: s.TurnBased, Name: s.SaveName}
}

// SaveGameToFile writes the current game state to a JSON file
func (g *MMGame) SaveGameToFile(path string) error {
	wm := world.GlobalWorldManager
	if wm == nil {
		return errors.New("world manager not available")
	}
	save := g.buildSave(wm)
	if f, err := os.Open(path); err == nil {
		var prev GameSave
		if err := json.NewDecoder(f).Decode(&prev); err == nil && prev.SaveName != "" {
			save.SaveName = prev.SaveName
		}
		_ = f.Close()
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(&save)
}

// RenameSaveSlot updates the stored save name for an existing slot.
func RenameSaveSlot(slot int, name string) error {
	path := slotPath(slot)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	var save GameSave
	if err := json.NewDecoder(f).Decode(&save); err != nil {
		_ = f.Close()
		return err
	}
	_ = f.Close()
	save.SaveName = name
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(&save)
}

// LoadGameFromFile loads state from a JSON file and applies it
func (g *MMGame) LoadGameFromFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var save GameSave
	if err := json.NewDecoder(f).Decode(&save); err != nil {
		return err
	}

	wm := world.GlobalWorldManager
	if wm == nil {
		return errors.New("world manager not available")
	}
	return g.applySave(wm, &save)
}

func normalizeItemFromConfig(item *items.Item) {
	if item == nil {
		return
	}
	switch item.Type {
	case items.ItemArmor, items.ItemAccessory, items.ItemConsumable, items.ItemQuest, items.ItemTrinket:
	default:
		return
	}
	_, key, ok := config.GetItemDefinitionByName(item.Name)
	if !ok || key == "" {
		return
	}
	template, err := items.TryCreateItemFromYAML(key)
	if err != nil {
		return
	}
	if item.Attributes == nil {
		item.Attributes = make(map[string]int)
	}
	for k, v := range template.Attributes {
		if _, exists := item.Attributes[k]; !exists {
			item.Attributes[k] = v
		}
	}
	if item.ArmorCategory == "" {
		item.ArmorCategory = template.ArmorCategory
	}
	if item.Description == "" {
		item.Description = template.Description
	}
	if item.Rarity == "" {
		item.Rarity = template.Rarity
	}
}

// restoreCharacterSave reconstructs one character (active or reserve) from a save.
func restoreCharacterSave(cs CharacterSave) *character.MMCharacter {
	m := &character.MMCharacter{
		Name:             cs.Name,
		Class:            character.CharacterClass(cs.Class),
		Promotion:        character.Promotion(cs.Promotion),
		Level:            cs.Level,
		Experience:       cs.Experience,
		HitPoints:        cs.HitPoints,
		MaxHitPoints:     cs.MaxHitPoints,
		SpellPoints:      cs.SpellPoints,
		MaxSpellPoints:   cs.MaxSpellPoints,
		Might:            cs.Might,
		Intellect:        cs.Intellect,
		Personality:      cs.Personality,
		Endurance:        cs.Endurance,
		Accuracy:         cs.Accuracy,
		Speed:            cs.Speed,
		Luck:             cs.Luck,
		FreeStatPoints:   cs.FreeStatPoints,
		OwedLevelChoices: append([]int(nil), cs.OwedLevelChoices...),
		Skills:           make(map[character.SkillType]*character.Skill),
		MagicSchools:     make(map[character.MagicSchoolID]*character.MagicSkill),
		Equipment:        make(map[items.EquipSlot]items.Item),
	}
	if len(cs.Conditions) > 0 {
		m.Conditions = make([]character.Condition, len(cs.Conditions))
		for i, c := range cs.Conditions {
			m.Conditions[i] = character.Condition(c)
		}
	}
	for _, s := range cs.Skills {
		mastery := character.SkillMastery(s.Mastery)
		if migrated := character.MasteryForLevel(s.Level); migrated > mastery {
			mastery = migrated
		}
		m.Skills[character.SkillType(s.Type)] = &character.Skill{Mastery: mastery}
	}
	for _, me := range cs.MagicSchools {
		mk := character.MagicSchoolID(me.School)
		mastery := character.SkillMastery(me.Mastery)
		if migrated := character.MasteryForLevel(me.Level); migrated > mastery {
			mastery = migrated
		}
		ms := &character.MagicSkill{Mastery: mastery}
		if len(me.KnownSpells) > 0 {
			ms.KnownSpells = make([]spells.SpellID, len(me.KnownSpells))
			for i, s := range me.KnownSpells {
				ms.KnownSpells[i] = spells.SpellID(s)
			}
		}
		m.MagicSchools[mk] = ms
	}
	for _, eq := range cs.Equipment {
		item := eq.Item
		normalizeItemFromConfig(&item)
		m.Equipment[items.EquipSlot(eq.Slot)] = item
	}
	m.PoisonFramesRemaining = cs.PoisonFramesRemaining
	m.ActionsRemaining = cs.ActionsRemaining
	return m
}

// buildCharacterSave serializes one character (active or reserve).
func buildCharacterSave(m *character.MMCharacter) CharacterSave {
	cs := CharacterSave{
		Name:             m.Name,
		Class:            int(m.Class),
		Promotion:        int(m.Promotion),
		Level:            m.Level,
		Experience:       m.Experience,
		HitPoints:        m.HitPoints,
		MaxHitPoints:     m.MaxHitPoints,
		SpellPoints:      m.SpellPoints,
		MaxSpellPoints:   m.MaxSpellPoints,
		Might:            m.Might,
		Intellect:        m.Intellect,
		Personality:      m.Personality,
		Endurance:        m.Endurance,
		Accuracy:         m.Accuracy,
		Speed:            m.Speed,
		Luck:             m.Luck,
		FreeStatPoints:   m.FreeStatPoints,
		OwedLevelChoices: append([]int(nil), m.OwedLevelChoices...),
	}
	if len(m.Conditions) > 0 {
		cs.Conditions = make([]int, len(m.Conditions))
		for i, c := range m.Conditions {
			cs.Conditions[i] = int(c)
		}
	}
	for t, s := range m.Skills {
		cs.Skills = append(cs.Skills, SkillEntry{Type: int(t), Level: s.Level(), Mastery: int(s.Mastery)})
	}
	for school, ms := range m.MagicSchools {
		entry := MagicSchoolEntry{School: string(school), Level: ms.Level(), Mastery: int(ms.Mastery)}
		if len(ms.KnownSpells) > 0 {
			entry.KnownSpells = make([]string, len(ms.KnownSpells))
			for i, sp := range ms.KnownSpells {
				entry.KnownSpells[i] = string(sp)
			}
		}
		cs.MagicSchools = append(cs.MagicSchools, entry)
	}
	for slot, item := range m.Equipment {
		cs.Equipment = append(cs.Equipment, EquipmentEntry{Slot: int(slot), Item: item})
	}
	cs.PoisonFramesRemaining = m.PoisonFramesRemaining
	cs.ActionsRemaining = m.ActionsRemaining
	return cs
}

// buildSave gathers game state into a serializable struct
func (g *MMGame) buildSave(wm *world.WorldManager) GameSave {
	// Party
	ps := PartySave{
		Gold:      g.party.Gold,
		Food:      g.party.Food,
		Inventory: g.party.Inventory,
		Members:   make([]CharacterSave, 0, len(g.party.Members)),
	}
	for _, m := range g.party.Members {
		ps.Members = append(ps.Members, buildCharacterSave(m))
	}
	for _, m := range g.party.Reserve {
		ps.Reserve = append(ps.Reserve, buildCharacterSave(m))
	}
	for _, m := range g.party.Captive {
		ps.Captive = append(ps.Captive, buildCharacterSave(m))
	}

	// Ground containers (loot bags + treasure chests) currently on the ground.
	var groundContainerSaves []GroundContainerSave
	if len(g.groundContainers) > 0 {
		groundContainerSaves = make([]GroundContainerSave, len(g.groundContainers))
		for i, c := range g.groundContainers {
			entry := GroundContainerSave{
				Kind:           int(c.Kind),
				ID:             c.ID,
				MapKey:         c.MapKey,
				X:              c.X,
				Y:              c.Y,
				Gold:           c.Gold,
				Sprite:         c.Sprite,
				SizeMultiplier: c.SizeMultiplier,
			}
			if len(c.Items) > 0 {
				entry.Items = append([]items.Item(nil), c.Items...)
			}
			groundContainerSaves[i] = entry
		}
	}

	// Monsters across all loaded maps.
	var ms []MonsterSave
	mapMonsters := make(map[string][]MonsterSave)
	encounterIDs := make(map[*monster.EncounterRewards]int)
	nextEncounterID := 1
	buildMonsterSaves := func(w *world.World3D) []MonsterSave {
		monsters := make([]MonsterSave, 0, len(w.Monsters))
		for _, mon := range w.Monsters {
			// Save the monster's own key (always set) — a name lookup is
			// ambiguous when several monsters share a Name (the elemental
			// dragons are all "Dragon") and would restore the wrong variant.
			saveEntry := MonsterSave{Key: mon.Key, Name: mon.Name, X: mon.X, Y: mon.Y, HitPoints: mon.HitPoints, Charmed: mon.Charmed, CharmFramesRemaining: mon.CharmFramesRemaining}
			if mon.IsEncounterMonster && mon.EncounterRewards != nil {
				saveEntry.IsEncounterMonster = true
				if id, ok := encounterIDs[mon.EncounterRewards]; ok {
					saveEntry.EncounterID = id
				} else {
					encounterIDs[mon.EncounterRewards] = nextEncounterID
					saveEntry.EncounterID = nextEncounterID
					nextEncounterID++
				}
				rewards := mon.EncounterRewards
				saveEntry.EncounterRewards = &EncounterRewardSave{
					Gold:              rewards.Gold,
					Experience:        rewards.Experience,
					CompletionMessage: rewards.CompletionMessage,
					QuestID:           rewards.QuestID,
				}
				if rewards.TreasureChest != nil {
					saveEntry.EncounterRewards.TreasureChest = treasureChestRewardToSave(rewards.TreasureChest)
				}
				for _, chest := range rewards.TreasureChests {
					if chestSave := treasureChestRewardToSave(&chest); chestSave != nil {
						saveEntry.EncounterRewards.TreasureChests = append(saveEntry.EncounterRewards.TreasureChests, *chestSave)
					}
				}
			}
			monsters = append(monsters, saveEntry)
		}
		return monsters
	}
	if wm != nil {
		for mapKey, w := range wm.LoadedMaps {
			monsters := buildMonsterSaves(w)
			mapMonsters[mapKey] = monsters
			if mapKey == wm.CurrentMapKey {
				ms = monsters
			}
		}
	} else if g.world != nil {
		ms = buildMonsterSaves(g.world)
	}

	// NPC states across all loaded maps
	var nstates []NPCSave
	if wm != nil {
		for mapKey, w := range wm.LoadedMaps {
			for _, npc := range w.NPCs {
				nstates = append(nstates, NPCSave{MapKey: mapKey, Name: npc.Name, Visited: npc.Visited})
			}
		}
	}

	// Quest progress (save all quests, not just active)
	var questSaves []QuestSave
	if g.questManager != nil {
		for _, quest := range g.questManager.GetAllQuests() {
			questSaves = append(questSaves, QuestSave{
				ID:             quest.ID,
				Status:         string(quest.Status),
				CurrentCount:   quest.CurrentCount,
				RewardsClaimed: quest.RewardsClaimed,
			})
		}
	}

	// Calculate played time
	playedTime := time.Since(g.sessionStartTime)

	var pendingChoices []PendingLevelUpChoiceSave
	if len(g.levelUpChoiceQueue) > 0 {
		pendingChoices = make([]PendingLevelUpChoiceSave, 0, len(g.levelUpChoiceQueue))
		for _, req := range g.levelUpChoiceQueue {
			pendingChoices = append(pendingChoices, PendingLevelUpChoiceSave{
				CharIndex: req.charIndex,
				Level:     req.level,
			})
		}
	}

	return GameSave{
		MapKey:                 wm.CurrentMapKey,
		PlayerX:                g.camera.X,
		PlayerY:                g.camera.Y,
		PlayerAngle:            g.camera.Angle,
		TurnBased:              g.turnBasedMode,
		SavedAt:                time.Now().Format(time.RFC3339),
		Party:                  ps,
		Monsters:               ms,
		MapMonsters:            mapMonsters,
		NPCStates:              nstates,
		Quests:                 questSaves,
		GroundContainers:       groundContainerSaves,
		PendingLevelUpChoices:  pendingChoices,
		PlayedTimeNs:           playedTime.Nanoseconds(),
		CurrentTurn:            g.currentTurn,
		PartyActionsUsed:       g.partyActionsUsed,
		TurnBasedMoveCooldown:  g.turnBasedMoveCooldown,
		TurnBasedRotCooldown:   g.turnBasedRotCooldown,
		MonsterTurnResolved:    g.monsterTurnResolved,
		TurnBasedSpRegenCount:  g.turnBasedSpRegenCount,
		TorchLightActive:       g.torchLightActive,
		TorchLightDuration:     g.torchLightDuration,
		TorchLightRadius:       g.torchLightRadius,
		WizardEyeActive:        g.wizardEyeActive,
		WizardEyeDuration:      g.wizardEyeDuration,
		WalkOnWaterActive:      g.walkOnWaterActive,
		WalkOnWaterDuration:    g.walkOnWaterDuration,
		BlessActive:            g.blessActive,
		BlessDuration:          g.blessDuration,
		BlessStatBonus:         g.blessStatBonus,
		DayGodsActive:          g.dayGodsActive,
		DayGodsDuration:        g.dayGodsDuration,
		DayGodsResistPct:       g.dayGodsResistPct,
		HourPowerActive:        g.hourPowerActive,
		HourPowerDuration:      g.hourPowerDuration,
		HourPowerOutBonus:      g.hourPowerOutBonus,
		HourPowerInReduce:      g.hourPowerInReduce,
		WaterBreathingActive:   g.waterBreathingActive,
		WaterBreathingDuration: g.waterBreathingDuration,
		UnderwaterReturnX:      g.underwaterReturnX,
		UnderwaterReturnY:      g.underwaterReturnY,
		UnderwaterReturnMap:    g.underwaterReturnMap,
		StatBonus:              g.statBonus,
		MapReturnPoses:         g.mapReturnPoses,
	}
}

// applySave restores game state from a save struct
func (g *MMGame) applySave(wm *world.WorldManager, save *GameSave) error {
	// Switch map if needed
	if save.MapKey != "" && save.MapKey != wm.CurrentMapKey && wm.IsValidMap(save.MapKey) {
		if err := wm.SwitchToMap(save.MapKey); err != nil {
			return err
		}
	}
	// Update world reference and visuals
	g.world = wm.GetCurrentWorld()
	g.UpdateSkyAndGroundColors()
	g.collisionSystem.UpdateTileChecker(g.world)
	if g.gameLoop != nil && g.gameLoop.renderer != nil {
		g.gameLoop.renderer.precomputeFloorColorCache()
		g.gameLoop.renderer.buildTransparentSpriteCache()
	}

	// Restore player
	g.camera.X = save.PlayerX
	g.camera.Y = save.PlayerY
	g.camera.Angle = save.PlayerAngle
	g.collisionSystem.UpdateEntity("player", save.PlayerX, save.PlayerY)

	// Restore party
	g.party = &character.Party{Members: make([]*character.MMCharacter, 0, len(save.Party.Members)), Gold: save.Party.Gold, Food: save.Party.Food, Inventory: save.Party.Inventory}
	for i := range g.party.Inventory {
		normalizeItemFromConfig(&g.party.Inventory[i])
	}
	for _, cs := range save.Party.Members {
		g.party.Members = append(g.party.Members, restoreCharacterSave(cs))
	}
	for _, cs := range save.Party.Reserve {
		g.party.Reserve = append(g.party.Reserve, restoreCharacterSave(cs))
	}
	for _, cs := range save.Party.Captive {
		g.party.Captive = append(g.party.Captive, restoreCharacterSave(cs))
	}

	// Restore monsters (all loaded maps)
	if wm != nil {
		rewardsCache := make(map[int]*monster.EncounterRewards)
		restoreMonsters := func(w *world.World3D, monsters []MonsterSave) {
			w.Monsters = make([]*monster.Monster3D, 0, len(monsters))
			for _, ms := range monsters {
				key := ms.Key
				if key == "" {
					key = findMonsterKeyByName(ms.Name)
				}
				if key == "" {
					continue
				}
				m := monster.NewMonster3DFromConfig(ms.X, ms.Y, key, g.config)
				m.HitPoints = ms.HitPoints
				m.Charmed = ms.Charmed
				m.CharmFramesRemaining = ms.CharmFramesRemaining
				if ms.IsEncounterMonster && ms.EncounterRewards != nil {
					m.IsEncounterMonster = true
					if ms.EncounterID > 0 {
						if rewards, ok := rewardsCache[ms.EncounterID]; ok {
							m.EncounterRewards = rewards
						} else {
							rewards = encounterRewardsFromSave(ms.EncounterRewards)
							rewardsCache[ms.EncounterID] = rewards
							m.EncounterRewards = rewards
						}
					} else {
						m.EncounterRewards = encounterRewardsFromSave(ms.EncounterRewards)
					}
				}
				w.Monsters = append(w.Monsters, m)
			}
		}

		if len(save.MapMonsters) > 0 {
			for mapKey, w := range wm.LoadedMaps {
				monsters, ok := save.MapMonsters[mapKey]
				if !ok {
					continue
				}
				restoreMonsters(w, monsters)
			}
		} else if g.world != nil {
			restoreMonsters(g.world, save.Monsters)
		}

		// Re-register current map monsters with collision system
		if g.world != nil {
			g.collisionSystem = collision.NewCollisionSystem(g.world, float64(g.config.World.TileSize))
			g.collisionSystem.RegisterEntity(collision.NewEntity("player", g.camera.X, g.camera.Y, 16, 16, collision.CollisionTypePlayer, false))
			g.world.RegisterMonstersWithCollisionSystem(g.collisionSystem)
		}
	}

	// Restore NPC visited flags across maps
	if wm != nil {
		for _, ns := range save.NPCStates {
			if w, ok := wm.LoadedMaps[ns.MapKey]; ok {
				for _, npc := range w.NPCs {
					if npc.Name == ns.Name {
						npc.Visited = ns.Visited
					}
				}
			}
		}
	}

	// Restore mode
	g.turnBasedMode = save.TurnBased
	g.currentTurn = save.CurrentTurn
	g.partyActionsUsed = save.PartyActionsUsed
	g.turnBasedMoveCooldown = save.TurnBasedMoveCooldown
	g.turnBasedRotCooldown = save.TurnBasedRotCooldown
	g.monsterTurnResolved = save.MonsterTurnResolved
	g.turnBasedSpRegenCount = save.TurnBasedSpRegenCount

	// Restore utility/buff state
	g.torchLightActive = save.TorchLightActive
	g.torchLightDuration = save.TorchLightDuration
	g.torchLightRadius = save.TorchLightRadius
	if g.torchLightActive && g.torchLightRadius == 0 {
		g.torchLightRadius = TorchLightRadiusTiles
	}
	g.wizardEyeActive = save.WizardEyeActive
	g.wizardEyeDuration = save.WizardEyeDuration
	g.walkOnWaterActive = save.WalkOnWaterActive
	g.walkOnWaterDuration = save.WalkOnWaterDuration
	g.blessActive = save.BlessActive
	g.blessDuration = save.BlessDuration
	g.blessStatBonus = save.BlessStatBonus
	g.dayGodsActive = save.DayGodsActive
	g.dayGodsDuration = save.DayGodsDuration
	g.dayGodsResistPct = save.DayGodsResistPct
	g.hourPowerActive = save.HourPowerActive
	g.hourPowerDuration = save.HourPowerDuration
	g.hourPowerOutBonus = save.HourPowerOutBonus
	g.hourPowerInReduce = save.HourPowerInReduce
	g.waterBreathingActive = save.WaterBreathingActive
	g.waterBreathingDuration = save.WaterBreathingDuration
	g.underwaterReturnX = save.UnderwaterReturnX
	g.underwaterReturnY = save.UnderwaterReturnY
	g.underwaterReturnMap = save.UnderwaterReturnMap
	g.statBonus = save.StatBonus
	g.mapReturnPoses = save.MapReturnPoses
	if g.mapReturnPoses == nil {
		g.mapReturnPoses = make(map[string]MapPose)
	}
	g.levelUpChoiceQueue = g.levelUpChoiceQueue[:0]
	g.levelUpChoiceOpen = false
	g.levelUpChoiceIdx = 0
	for _, pending := range save.PendingLevelUpChoices {
		if pending.CharIndex < 0 || pending.CharIndex >= len(g.party.Members) {
			continue
		}
		char := g.party.Members[pending.CharIndex]
		choices := config.GetLevelUpChoices(char.GetClassKey(), pending.Level)
		if len(choices) == 0 {
			continue
		}
		g.queueLevelUpChoices(char, pending.Level, choices)
	}
	g.gameOver = false
	g.gameVictory = false
	g.showHighScores = false

	if g.world != nil {
		g.world.SetWalkOnWaterActive(g.walkOnWaterActive)
		g.world.SetWaterBreathingActive(g.waterBreathingActive)
	}

	// Restore ground containers (loot bags + treasure chests).
	g.groundContainers = make([]GroundContainer, 0, len(save.GroundContainers))
	for _, c := range save.GroundContainers {
		restored := GroundContainer{
			Kind:           ContainerKind(c.Kind),
			ID:             c.ID,
			MapKey:         c.MapKey,
			X:              c.X,
			Y:              c.Y,
			Gold:           c.Gold,
			Sprite:         c.Sprite,
			SizeMultiplier: c.SizeMultiplier,
		}
		if len(c.Items) > 0 {
			restored.Items = make([]items.Item, len(c.Items))
			for i, it := range c.Items {
				normalizeItemFromConfig(&it)
				restored.Items[i] = it
			}
		}
		g.groundContainers = append(g.groundContainers, restored)
	}

	// Rebuild HUD buff icons from the single timed-buff registry (same source the
	// per-frame update uses), so a restored buff shows its timer immediately.
	g.utilitySpellStatuses = make(map[spells.SpellID]*UtilitySpellStatus)
	for _, b := range g.timedBuffs() {
		g.updateUtilityStatus(b.id, *b.duration, *b.active)
	}

	// Restore quest progress
	if g.questManager != nil && len(save.Quests) > 0 {
		for _, qs := range save.Quests {
			g.questManager.RestoreQuestProgress(qs.ID, quests.QuestStatus(qs.Status), qs.CurrentCount, qs.RewardsClaimed)
		}
	}

	// Restore played time by adjusting session start
	if save.PlayedTimeNs > 0 {
		g.sessionStartTime = time.Now().Add(-time.Duration(save.PlayedTimeNs))
	}

	return nil
}

func findMonsterKeyByName(name string) string {
	if monster.MonsterConfig == nil {
		return ""
	}
	for key, def := range monster.MonsterConfig.Monsters {
		if def.Name == name {
			return key
		}
	}
	return ""
}
