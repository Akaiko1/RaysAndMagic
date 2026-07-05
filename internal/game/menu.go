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
	"ugataima/internal/highscore"
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

// Save-slot menu layout. The menus show saveRowsPerPage rows across savePageCount
// pages. Global row 0 is the shared Autosave (written automatically on map change
// and stash use; load-only - never manually overwritten). Rows 1..N are manual
// slots and map to save1.json.. unchanged, so existing saves stay reachable.
const (
	saveRowsPerPage = 7
	savePageCount   = 3
	saveRowCount    = saveRowsPerPage * savePageCount
	autosaveFile    = "autosave.json"

	// Shared geometry for the save/load menus, used by both the draw code and
	// the layout-collision test so the two never drift. Panels are sized to fit
	// saveRowsPerPage rows (the pager sits on a strip just below the in-game panel).
	saveMenuPanelW = 340
	saveMenuPanelH = 320
	// saveMenuListTopY centers the saveRowsPerPage-row block vertically between
	// the header text (ends ~py+48) and the panel bottom (py+saveMenuPanelH):
	// the highlight block is (rows-1)*pitch + 28 tall, so equal top/bottom gaps
	// put the first row at py+78. Recompute if panelH / row count / pitch change.
	saveMenuListTopY = 78
	saveMenuRowPitch = 32 // vertical pitch between rows
	entryLoadPanelW  = 460
	entryLoadPanelH  = 480
	entryLoadRowH    = 44

	// Main-menu panel + option-list layout (its own size, distinct from the
	// save/load panel). Shared by the draw code and the input hit-testing.
	mainMenuPanelW   = 360
	mainMenuPanelH   = 320
	mainMenuListTopY = 56
	mainMenuRowPitch = 32

	// menuRowHeight is the highlight/hitbox height of one vertical-menu row,
	// shared by Main-menu options and save/load slots (see menuRowRect).
	menuRowHeight = 28
)

// menuPanelSize returns the panel dimensions for a main-menu mode. Shared by the
// draw code (drawMainMenu) and the input hit-testing (handleMainMenuInput) so
// the drawn panel and its click regions can't drift.
func menuPanelSize(mode MainMenuMode) (w, h int) {
	if mode == MenuMain {
		return mainMenuPanelW, mainMenuPanelH
	}
	return saveMenuPanelW, saveMenuPanelH
}

// menuRowRect returns the highlight/click box and the text baseline for row i of
// a vertical menu list (Main-menu options, save/load slots). ONE geometry so the
// draw highlight, hover-select and click hit-tests never drift (cf.
// savePagerButtonRects). startY is the first row's baseline offset from py; pitch
// is the row spacing.
func menuRowRect(px, py, panelW, startY, pitch, i int) (box pagerRect, textX, textY int) {
	y := py + startY + i*pitch
	return pagerRect{px + 16, y - 4, px + panelW - 16, y - 4 + menuRowHeight}, px + 28, y
}

// saveRowPath maps a global save-row index to its file. Row 0 is the autosave.
func saveRowPath(row int) string {
	if row == 0 {
		return storage.AppSavePath(autosaveFile)
	}
	return storage.AppSavePath(fmt.Sprintf("save%d.json", row))
}

// saveRowIsAutosave reports whether a row is the load-only autosave slot.
func saveRowIsAutosave(row int) bool { return row == 0 }

// selectedSaveRow is the global save-row index the save/load menu cursor points
// at: the row-within-page (slotSelection) offset by the current page.
func (g *MMGame) selectedSaveRow() int {
	return g.savePage*saveRowsPerPage + g.slotSelection
}

// saveRowLabel is the slot's display name ("Autosave" or "Slot N").
func saveRowLabel(row int) string {
	if row == 0 {
		return "Autosave"
	}
	return fmt.Sprintf("Slot %d", row)
}

// GetSaveRowSummary reads minimal display info for a global save-row index.
func GetSaveRowSummary(row int) SaveSummary {
	return summaryFromPath(saveRowPath(row))
}

// Autosave writes the shared autosave slot (row 0). Best-effort and silent: a
// failure must never interrupt play. No-op outside live gameplay so it can't fire
// mid-load or before the world exists.
func (g *MMGame) Autosave() {
	_ = g.autosaveErr()
}

// autosaveErr is Autosave that reports a real write failure, so callers needing
// the bag/game state committed in step with another store (the stash) can detect
// it and roll back. A guard miss returns nil: there is no in-game state to
// autosave (the stash only opens in play, so this is the unit-test / not-in-game
// case), which is "nothing to commit", not a failure.
func (g *MMGame) autosaveErr() error {
	if g.appScreen != AppScreenInGame || world.GlobalWorldManager == nil || g.party == nil {
		return nil
	}
	return g.SaveGameToFile(saveRowPath(0))
}

// mainMenuOptions defines the visible options in the ESC menu. "Main Menu"
// returns to the title screen (not a full app quit - that's the title's "Quit").
var mainMenuOptions = []string{"Continue", "Save", "Load", "High Scores", "Main Menu"}

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
	TotalGoldEarned       int                        `json:"total_gold_earned,omitempty"`
	TotalExperienceEarned int                        `json:"total_experience_earned,omitempty"`
	VictoryAcknowledged   bool                       `json:"victory_acknowledged,omitempty"`

	// Turn-based state
	CurrentTurn           int  `json:"current_turn,omitempty"`
	PartyActionsUsed      int  `json:"party_actions_used,omitempty"`
	TurnBasedMoveCooldown int  `json:"turn_based_move_cooldown,omitempty"`
	TurnBasedRotCooldown  int  `json:"turn_based_rot_cooldown,omitempty"`
	MonsterTurnResolved   bool `json:"monster_turn_resolved,omitempty"`
	TurnBasedSpRegenCount int  `json:"turn_based_sp_regen_count,omitempty"`
	ExtraMonsterAction    bool `json:"extra_monster_action,omitempty"`

	// Utility/buff state
	TorchLightActive       bool             `json:"torch_light_active,omitempty"`
	TorchLightDuration     int              `json:"torch_light_duration,omitempty"`
	TorchLightRadius       float64          `json:"torch_light_radius,omitempty"`
	WizardEyeActive        bool             `json:"wizard_eye_active,omitempty"`
	WizardEyeDuration      int              `json:"wizard_eye_duration,omitempty"`
	WalkOnWaterActive      bool             `json:"walk_on_water_active,omitempty"`
	WalkOnWaterDuration    int              `json:"walk_on_water_duration,omitempty"`
	BlessActive            bool             `json:"bless_active,omitempty"`
	BlessDuration          int              `json:"bless_duration,omitempty"`
	BlessStatBonus         int              `json:"bless_stat_bonus,omitempty"`
	StatBuffs              []StatBuffSave   `json:"stat_buffs,omitempty"`
	BlessBonusesPerStat    map[string]int   `json:"bless_bonuses_per_stat,omitempty"`
	CombatBuffs            []CombatBuffSave `json:"combat_buffs,omitempty"`
	SteamZones             []SteamZoneSave  `json:"steam_zones,omitempty"`
	Traps                  []TrapSave       `json:"traps,omitempty"`
	WaterBreathingActive   bool             `json:"water_breathing_active,omitempty"`
	WaterBreathingDuration int              `json:"water_breathing_duration,omitempty"`
	UnderwaterReturnX      float64          `json:"underwater_return_x,omitempty"`
	UnderwaterReturnY      float64          `json:"underwater_return_y,omitempty"`
	UnderwaterReturnMap    string           `json:"underwater_return_map,omitempty"`
	StatBonus              int              `json:"stat_bonus,omitempty"`

	// MapReturnPoses remembers where the party entered each map via a gate, so a
	// return trip drops them at the doorway rather than the map's spawn tile.
	MapReturnPoses map[string]MapPose `json:"map_return_poses,omitempty"`
}

// QuestSave captures quest progress for save/load
type QuestSave struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	CurrentCount   int    `json:"current_count"`
	DynamicTarget  int    `json:"dynamic_target,omitempty"`
	RewardsClaimed bool   `json:"rewards_claimed"`
}

type PartySave struct {
	Gold                int             `json:"gold"`
	Food                int             `json:"food"`
	Inventory           []items.Item    `json:"inventory"`
	Members             []CharacterSave `json:"members"`
	Reserve             []CharacterSave `json:"reserve,omitempty"`
	Captive             []CharacterSave `json:"captive,omitempty"`
	CardCollection      []string        `json:"card_collection,omitempty"`       // legacy/card UI keys
	CardCollectionItems []items.Item    `json:"card_collection_items,omitempty"` // physical cards with InstanceID
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
	QuickSlots            []QuickSlotEntry   `json:"quick_slots,omitempty"`
	PoisonFramesRemaining int                `json:"poison_frames_remaining,omitempty"`
	BurnFramesRemaining   int                `json:"burn_frames_remaining,omitempty"`
	StunFramesRemaining   int                `json:"stun_frames_remaining,omitempty"`
	StunTurnsRemaining    int                `json:"stun_turns_remaining,omitempty"`
	// ActionsRemaining preserves mid-round turn-based state so save/reload
	// can't be used to refill action slots. Omitted from real-time saves
	// (value will simply be 0; ignored when turn-based mode is off).
	ActionsRemaining int `json:"actions_remaining,omitempty"`
	// RTCooldown preserves the real-time action cooldown - reload must not
	// reset the party's swing timers mid-fight.
	RTCooldown int `json:"rt_cooldown,omitempty"`
	// OffHandRTCooldown mirrors RTCooldown for a Dual Wielding character's
	// off-hand weapon.
	OffHandRTCooldown int `json:"off_hand_rt_cooldown,omitempty"`
	// NextTBAttackOffHand preserves which hand a Dual Wielding character prefers
	// next, so save/reload can't be used to re-pick the preferred weapon.
	NextTBAttackOffHand bool `json:"next_tb_attack_off_hand,omitempty"`
}

type SkillEntry struct {
	Type    int `json:"type"`
	Level   int `json:"level"`
	Mastery int `json:"mastery"`
}

// PendingLevelUpChoiceSave records that party member CharIndex has earned a
// level-up choice at Level but hasn't picked one yet. Options themselves are
// not stored - they're rebuilt from the character's class config on load.
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

// QuickSlotEntry is one occupied quick slot (sparse: empty slots are omitted).
type QuickSlotEntry struct {
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
	ID                      string  `json:"id,omitempty"`
	Key                     string  `json:"key"`
	Name                    string  `json:"name"`
	X                       float64 `json:"x"`
	Y                       float64 `json:"y"`
	HitPoints               int     `json:"hit_points"`
	Bound                   bool    `json:"bound,omitempty"`
	BoundFramesRemaining    int     `json:"bound_frames_remaining,omitempty"`
	Pacified                bool    `json:"pacified,omitempty"`
	PacifiedFramesRemaining int     `json:"pacified_frames_remaining,omitempty"`
	WasAttacked             bool    `json:"was_attacked,omitempty"`
	Relentless              bool    `json:"relentless,omitempty"` // patron-death revenge: relentless map-wide hunt, survives reload
	QuestProgressIgnored    bool    `json:"quest_progress_ignored,omitempty"`
	// Mid-combat cooldowns: reload must not strip a player-applied stun or
	// reset the monster's special-attack cadence.
	StunFramesRemaining     int `json:"stun_frames_remaining,omitempty"`
	StunTurnsRemaining      int `json:"stun_turns_remaining,omitempty"`
	PoisonedFramesRemaining int `json:"poisoned_frames_remaining,omitempty"` // Venom-proc cards
	// Stun diminishing-returns chain - persisted so save/reload can't reset it
	// and re-enable a full-strength perma-stun-lock (bosses included).
	StunDRStacks        int                  `json:"stun_dr_stacks,omitempty"`
	StunDRMemoryTurns   int                  `json:"stun_dr_memory_turns,omitempty"`
	StunDRMemoryFrames  int                  `json:"stun_dr_memory_frames,omitempty"`
	RootFramesRemaining int                  `json:"root_frames_remaining,omitempty"`
	RootTurnsRemaining  int                  `json:"root_turns_remaining,omitempty"`
	Pilfered            bool                 `json:"pilfered,omitempty"`
	PounceCDFrames      int                  `json:"pounce_cd_frames,omitempty"`
	PounceCDTurns       int                  `json:"pounce_cd_turns,omitempty"`
	BossCD              int                  `json:"boss_cd,omitempty"`
	BossHurtPending     bool                 `json:"boss_hurt_pending,omitempty"`
	BossLastHP          int                  `json:"boss_last_hp,omitempty"`
	SummonFirstDone     bool                 `json:"summon_first_done,omitempty"`
	SummonedBy          string               `json:"summoned_by,omitempty"`
	CrossfireCD         int                  `json:"crossfire_cd,omitempty"`
	IsEncounterMonster  bool                 `json:"is_encounter_monster,omitempty"`
	EncounterID         int                  `json:"encounter_id,omitempty"`
	EncounterRewards    *EncounterRewardSave `json:"encounter_rewards,omitempty"`
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
	LootTable         string   `json:"loot_table,omitempty"`
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
		LootTable:         reward.LootTable,
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
		LootTable:         save.LootTable,
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

// NPCSave tracks persistent NPC flags across maps. Identity is MapKey + spawn
// coordinates (deterministic from the map file) - display names can repeat on
// one map (e.g. two "City Gate" NPCs), coordinates can't. Legacy saves without
// coordinates fall back to name matching on restore.
type NPCSave struct {
	MapKey  string  `json:"map_key"`
	Name    string  `json:"name"`
	X       float64 `json:"x,omitempty"`
	Y       float64 `json:"y,omitempty"`
	Visited bool    `json:"visited"`
	// Remaining merchant quantities, aligned with the YAML stock order.
	StockQuantities []int `json:"stock_quantities,omitempty"`
}

// clearTransientCombatState drops every in-flight transient tied to the
// CURRENT world: projectiles, swings, VFX and pending deaths. Must run on any
// world swap (map switch, save load) or leftovers keep updating against the
// new map and can hit monsters there.
func (g *MMGame) clearTransientCombatState() {
	g.projectileMutex.Lock()
	if g.collisionSystem != nil {
		// applySave rebuilds the collision system anyway; switchToMap keeps it,
		// so stale projectile entities must be dropped explicitly.
		for i := range g.magicProjectiles {
			g.collisionSystem.UnregisterEntity(g.magicProjectiles[i].ID)
		}
		for i := range g.arrows {
			g.collisionSystem.UnregisterEntity(g.arrows[i].ID)
		}
	}
	g.magicProjectiles = g.magicProjectiles[:0]
	g.arrows = g.arrows[:0]
	g.projectileMutex.Unlock()
	g.slashEffects = g.slashEffects[:0]
	g.hitEffectsMu.Lock()
	g.spellHitEffects = g.spellHitEffects[:0]
	g.impactLights = g.impactLights[:0]
	g.hitEffectsMu.Unlock()
	g.deadMonsterIDs = g.deadMonsterIDs[:0]
}

// SaveSummary is lightweight info used for menu display
type SaveSummary struct {
	Exists    bool
	SavedAt   string
	MapKey    string
	TurnBased bool
	Name      string
	PlayTime  string           // formatted elapsed play time ("" if unknown)
	Party     []SavePartyBrief // active roster for the hover tooltip
}

// SavePartyBrief is the per-member info shown in the save hover tooltip.
type SavePartyBrief struct {
	Name  string
	Level int
	Class int
}

// summaryFromPath reads minimal display info from a save file path.
func summaryFromPath(path string) SaveSummary {
	f, err := os.Open(path)
	if err != nil {
		return SaveSummary{Exists: false}
	}
	defer f.Close()
	var s GameSave
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return SaveSummary{Exists: false}
	}
	sum := SaveSummary{Exists: true, SavedAt: s.SavedAt, MapKey: s.MapKey, TurnBased: s.TurnBased, Name: s.SaveName}
	if s.PlayedTimeNs > 0 {
		sum.PlayTime = highscore.FormatPlayTime(time.Duration(s.PlayedTimeNs))
	}
	for _, m := range s.Party.Members {
		sum.Party = append(sum.Party, SavePartyBrief{Name: m.Name, Level: m.Level, Class: m.Class})
	}
	return sum
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

// RenameSaveSlot updates the stored save name for an existing slot, identified
// by its global save-row index (rows 1..; the autosave row is never renamed).
func RenameSaveSlot(row int, name string) error {
	path := saveRowPath(row)
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
	g.loadNeedsResave = false
	if err := g.applySave(wm, &save); err != nil {
		return err
	}
	// One-time migration write: a load that stamped legacy items with instance
	// ids re-saves the slot so the ids stick (SaveGameToFile preserves the slot's
	// name). Without this the reloaded slot would revert to id-less items and stay
	// dupe-able. Best-effort - a write failure just defers the migration.
	if g.loadNeedsResave {
		g.loadNeedsResave = false
		_ = g.SaveGameToFile(path)
	}
	return nil
}

func normalizeItemFromConfig(item *items.Item) {
	if item == nil {
		return
	}
	// Weapons refresh from weapons.yaml (name/rarity/stat rebalances reach saved
	// slots), keyed by display name. Type-gated so a non-weapon that happens to
	// share a name can't be force-converted into a weapon.
	if item.Type == items.ItemWeapon {
		if _, key, ok := config.GetWeaponDefinitionByName(item.Name); ok && key != "" {
			if template, err := items.TryCreateWeaponFromYAML(key); err == nil {
				instanceID := item.InstanceID
				*item = template
				item.InstanceID = instanceID
			}
		}
		return
	}
	// Trap quick-slot items refresh from traps.yaml (name/cost rebalances
	// reach saved slots), keyed by SpellEffect.
	if item.Type == items.ItemTrap {
		if fresh, ok := config.TrapItem(string(item.SpellEffect)); ok {
			*item = fresh
		}
		return
	}
	switch item.Type {
	case items.ItemArmor, items.ItemAccessory, items.ItemConsumable, items.ItemQuest, items.ItemTrinket, items.ItemCard:
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
	// Adopt the def's current type, so cards saved before they became their own
	// type (was "trinket") migrate to items.ItemCard on load.
	item.Type = template.Type
	// The YAML definition is the single source of an item's attributes: ADOPT
	// it wholesale, so rebalanced (or removed) values reach items saved before
	// the change. Items carry no instance state in Attributes - merging only
	// missing keys left old saves frozen on their original balance forever.
	item.Attributes = make(map[string]int, len(template.Attributes))
	for k, v := range template.Attributes {
		item.Attributes[k] = v
	}
	item.ArmorCategory = template.ArmorCategory
	// rarity/description are definitional too (items carry no per-instance
	// override - nothing sets them outside YAML adoption), so adopt them
	// wholesale like attributes: a rebalanced rarity/desc reaches old saves,
	// matching the weapon branch above.
	item.Description = template.Description
	item.Rarity = template.Rarity
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
	for _, qs := range cs.QuickSlots {
		if qs.Slot < 0 || qs.Slot >= character.QuickSlotCount {
			continue
		}
		item := qs.Item
		normalizeItemFromConfig(&item)
		m.QuickSlots[qs.Slot] = &item
	}
	m.PoisonFramesRemaining = cs.PoisonFramesRemaining
	m.BurnFramesRemaining = cs.BurnFramesRemaining
	m.StunFramesRemaining = cs.StunFramesRemaining
	m.StunTurnsRemaining = cs.StunTurnsRemaining
	m.ActionsRemaining = cs.ActionsRemaining
	m.RTCooldown = cs.RTCooldown
	m.OffHandRTCooldown = cs.OffHandRTCooldown
	m.NextTBAttackOffHand = cs.NextTBAttackOffHand
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
	for i, item := range m.QuickSlots {
		if item != nil {
			cs.QuickSlots = append(cs.QuickSlots, QuickSlotEntry{Slot: i, Item: *item})
		}
	}
	cs.PoisonFramesRemaining = m.PoisonFramesRemaining
	cs.BurnFramesRemaining = m.BurnFramesRemaining
	cs.StunFramesRemaining = m.StunFramesRemaining
	cs.StunTurnsRemaining = m.StunTurnsRemaining
	cs.ActionsRemaining = m.ActionsRemaining
	cs.RTCooldown = m.RTCooldown
	cs.OffHandRTCooldown = m.OffHandRTCooldown
	cs.NextTBAttackOffHand = m.NextTBAttackOffHand
	return cs
}

// buildSave gathers game state into a serializable struct
func (g *MMGame) buildSave(wm *world.WorldManager) GameSave {
	// Legacy bless_* fields mirror the registry's bless entry so an older
	// binary can still read this save.
	legacyBless, _ := g.statBuffByID("bless")
	// Party
	ps := PartySave{
		Gold:                g.party.Gold,
		Food:                g.party.Food,
		Inventory:           g.party.Inventory,
		Members:             make([]CharacterSave, 0, len(g.party.Members)),
		CardCollection:      make([]string, MaxCardSlots),
		CardCollectionItems: make([]items.Item, MaxCardSlots),
	}
	// The key list is derived (legacy readers only); cardSlots is the truth.
	for i := 0; i < MaxCardSlots; i++ {
		ps.CardCollection[i] = g.cardCollectionKey(i)
		ps.CardCollectionItems[i] = g.cardCollectionItem(i)
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
			// Save the monster's own key (always set) - a name lookup is
			// ambiguous when several monsters share a Name (the elemental
			// dragons are all "Dragon") and would restore the wrong variant.
			saveEntry := MonsterSave{
				ID: mon.ID, Key: mon.Key, Name: mon.Name, X: mon.X, Y: mon.Y, HitPoints: mon.HitPoints,
				Bound: mon.Bound, BoundFramesRemaining: mon.BoundFramesRemaining,
				Pacified: mon.Pacified, PacifiedFramesRemaining: mon.PacifiedFramesRemaining,
				WasAttacked:             mon.WasAttacked,
				Relentless:              mon.Relentless,
				QuestProgressIgnored:    mon.QuestProgressIgnored,
				StunFramesRemaining:     mon.StunFramesRemaining,
				StunTurnsRemaining:      mon.StunTurnsRemaining,
				PoisonedFramesRemaining: mon.PoisonedFramesRemaining,
				StunDRStacks:            mon.StunDRStacks,
				StunDRMemoryTurns:       mon.StunDRMemoryTurns,
				StunDRMemoryFrames:      mon.StunDRMemoryFrames,
				RootFramesRemaining:     mon.RootFramesRemaining,
				RootTurnsRemaining:      mon.RootTurnsRemaining,
				Pilfered:                mon.Pilfered,
				PounceCDFrames:          mon.PounceCDFrames,
				PounceCDTurns:           mon.PounceCDTurns,
				BossCD:                  mon.BossCD,
				BossHurtPending:         mon.BossHurtPending,
				BossLastHP:              mon.BossLastHP,
				SummonFirstDone:         mon.SummonFirstDone,
				SummonedBy:              mon.SummonedBy,
				CrossfireCD:             mon.CrossfireCD,
			}
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
				ns := NPCSave{MapKey: mapKey, Name: npc.Name, X: npc.X, Y: npc.Y, Visited: npc.Visited}
				if len(npc.MerchantStock) > 0 {
					ns.StockQuantities = make([]int, len(npc.MerchantStock))
					for i, entry := range npc.MerchantStock {
						ns.StockQuantities[i] = entry.Quantity
					}
				}
				nstates = append(nstates, ns)
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
				DynamicTarget:  quest.DynamicTarget,
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
		MapKey:                wm.CurrentMapKey,
		PlayerX:               g.camera.X,
		PlayerY:               g.camera.Y,
		PlayerAngle:           g.camera.Angle,
		TurnBased:             g.turnBasedMode,
		SavedAt:               time.Now().Format(time.RFC3339),
		Party:                 ps,
		Monsters:              ms,
		MapMonsters:           mapMonsters,
		NPCStates:             nstates,
		Quests:                questSaves,
		GroundContainers:      groundContainerSaves,
		PendingLevelUpChoices: pendingChoices,
		PlayedTimeNs:          playedTime.Nanoseconds(),
		TotalGoldEarned:       g.totalGoldEarned,
		TotalExperienceEarned: g.totalExperienceEarned,
		VictoryAcknowledged:   g.victoryAcknowledged,
		CurrentTurn:           g.currentTurn,
		PartyActionsUsed:      g.partyActionsUsed,
		TurnBasedMoveCooldown: g.turnBasedMoveCooldown,
		TurnBasedRotCooldown:  g.turnBasedRotCooldown,
		MonsterTurnResolved:   g.monsterTurnResolved,
		TurnBasedSpRegenCount: g.turnBasedSpRegenCount,
		ExtraMonsterAction:    g.turnBasedExtraMonsterAction,
		TorchLightActive:      g.torchLightActive,
		TorchLightDuration:    g.torchLightDuration,
		TorchLightRadius:      g.torchLightRadius,
		WizardEyeActive:       g.wizardEyeActive,
		WizardEyeDuration:     g.wizardEyeDuration,
		WalkOnWaterActive:     g.walkOnWaterActive,
		WalkOnWaterDuration:   g.walkOnWaterDuration,
		StatBuffs:             buildStatBuffSaves(g.statBuffs),
		BlessActive:           legacyBless.Frames > 0,
		BlessDuration:         legacyBless.Frames,
		BlessStatBonus:        legacyBless.Bonuses.Might,
		BlessBonusesPerStat:   statBonusesToMap(legacyBless.Bonuses),
		// Write-only legacy: an OLD binary reads stat_bonus on load (its expiry
		// math subtracts bless_stat_bonus from it); the new binary derives the
		// aggregate from stat_buffs and never reads this back.
		StatBonus:              g.statBonuses.Might,
		CombatBuffs:            buildCombatBuffSaves(g.combatBuffs),
		SteamZones:             buildSteamZoneSaves(g.steamZones),
		Traps:                  buildTrapSaves(g.traps),
		WaterBreathingActive:   g.waterBreathingActive,
		WaterBreathingDuration: g.waterBreathingDuration,
		UnderwaterReturnX:      g.underwaterReturnX,
		UnderwaterReturnY:      g.underwaterReturnY,
		UnderwaterReturnMap:    g.underwaterReturnMap,

		MapReturnPoses: g.mapReturnPoses,
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

	g.clearTransientCombatState()

	// A loaded game starts with a clean combat log - the previous slot's history
	// must not bleed into it (clearTransientCombatState runs on every map switch
	// too, so the log reset lives here, on the load path, not in it).
	g.combatLogHistory = g.combatLogHistory[:0]
	g.combatLogScroll = 0
	g.combatLogOpen = false

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
	// Restore the monster-card collection (party-wide). New saves carry the
	// physical card item + InstanceID; the legacy key-only field is load-only
	// migration and cannot prove ownership against the shared stash.
	g.cardSlots = [MaxCardSlots]cardSlot{}
	for i := 0; i < MaxCardSlots && i < len(save.Party.CardCollectionItems); i++ {
		it := save.Party.CardCollectionItems[i]
		if it.Name == "" {
			continue
		}
		normalizeItemFromConfig(&it)
		hadID := it.InstanceID != 0
		if g.setCardCollectionSlot(i, it) && !hadID {
			g.loadNeedsResave = true
		}
	}
	for i := 0; i < MaxCardSlots && i < len(save.Party.CardCollection); i++ {
		if g.cardCollectionKey(i) != "" {
			continue
		}
		key := save.Party.CardCollection[i]
		if cardDef(key) == nil {
			continue
		}
		if g.stashOwnsCardKey(key) {
			g.loadNeedsResave = true
			continue
		}
		if g.setCardCollectionSlot(i, items.CreateItemFromYAML(key)) {
			g.loadNeedsResave = true
		}
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
	if save.TotalExperienceEarned > 0 {
		g.totalExperienceEarned = save.TotalExperienceEarned
	} else {
		g.totalExperienceEarned = earnedExperienceForParty(g.party)
	}
	if save.TotalGoldEarned > 0 {
		g.totalGoldEarned = save.TotalGoldEarned
	} else {
		g.totalGoldEarned = save.Party.Gold - g.config.Characters.StartingGold
		if g.totalGoldEarned < 0 {
			g.totalGoldEarned = 0
		}
	}
	// Instance-id dedupe: stamp any legacy (pre-id) party items, then strip from
	// the bag anything the shared chest already owns. A stamp means this slot was
	// migrated - flag it so LoadGameFromFile persists the ids once (the strip is
	// idempotent per load and needs no resave).
	if g.stampPartyInstanceIDs() {
		g.loadNeedsResave = true
	}
	g.reconcilePartyAgainstStash()
	// Benched rosters re-derive MaxHP/MaxSP under the CURRENT formula too -
	// a save written before a formula/balance change would otherwise keep
	// stale maxima until the hero is swapped in or trained. (Active members
	// get theirs via applyPartyStatBonuses below; bench carries no buffs.)
	for _, m := range g.party.Reserve {
		m.RecalculateMaxStatsKeepingCurrent(g.config)
	}
	for _, m := range g.party.Captive {
		m.RecalculateMaxStatsKeepingCurrent(g.config)
	}

	// Restore monsters (all loaded maps)
	if wm != nil {
		rewardsCache := make(map[int]*monster.EncounterRewards)
		// Self-heal sealed bosses: a dormant boss (passive-until-quest, no evade
		// radius) never legitimately moves while its quest is unfinished - it holds
		// its map spawn. Saves written before the dormant-freeze fix captured it
		// wandered off (e.g. the Samurai Warlord drifted off his throne), so on
		// restore we snap any still-sealed boss back to its map spawn. Idempotent
		// for correct saves (saved position already IS the spawn); once the quest
		// completes the boss has gone aggressive and may have moved, so it keeps
		// its saved position.
		completedQuests := make(map[string]bool)
		for _, q := range save.Quests {
			if q.Status == string(quests.QuestStatusCompleted) {
				completedQuests[q.ID] = true
			}
		}
		restoreMonsters := func(w *world.World3D, monsters []MonsterSave) {
			sealedSpawn := make(map[string][2]float64)
			for _, fresh := range w.Monsters {
				if fresh != nil && fresh.PassiveUntilQuest != "" && fresh.EvadeRadiusTiles == 0 &&
					!completedQuests[fresh.PassiveUntilQuest] {
					sealedSpawn[fresh.Key] = [2]float64{fresh.X, fresh.Y}
				}
			}
			w.Monsters = make([]*monster.Monster3D, 0, len(monsters))
			for _, ms := range monsters {
				key := ms.Key
				if key == "" {
					key = findMonsterKeyByName(ms.Name)
				}
				if key == "" {
					continue
				}
				x, y := ms.X, ms.Y
				if sp, ok := sealedSpawn[key]; ok {
					x, y = sp[0], sp[1] // sealed boss -> back to its throne
				}
				m := monster.NewMonster3DFromConfig(x, y, key, g.config)
				if ms.ID != "" {
					m.ID = ms.ID
				}
				// Seal a dormant boss immediately. refreshBoundUndeadCache recomputes
				// BossDormant every frame, but that runs AFTER input - so without this a
				// player action on the first frame after load could damage a still-sealed
				// boss before the flag is set. Uses the same completed-quest set as the
				// throne snap-back above.
				m.BossDormant = m.PassiveUntilQuest != "" && m.EvadeRadiusTiles == 0 &&
					!completedQuests[m.PassiveUntilQuest]
				m.HitPoints = ms.HitPoints
				m.Bound = ms.Bound
				m.BoundFramesRemaining = ms.BoundFramesRemaining
				m.Pacified = ms.Pacified
				m.PacifiedFramesRemaining = ms.PacifiedFramesRemaining
				m.StunFramesRemaining = ms.StunFramesRemaining
				m.StunTurnsRemaining = ms.StunTurnsRemaining
				m.PoisonedFramesRemaining = ms.PoisonedFramesRemaining
				m.StunDRStacks = ms.StunDRStacks
				m.StunDRMemoryTurns = ms.StunDRMemoryTurns
				m.StunDRMemoryFrames = ms.StunDRMemoryFrames
				m.RootFramesRemaining = ms.RootFramesRemaining
				m.RootTurnsRemaining = ms.RootTurnsRemaining
				m.Pilfered = ms.Pilfered
				m.PounceCDFrames = ms.PounceCDFrames
				m.PounceCDTurns = ms.PounceCDTurns
				m.BossCD = ms.BossCD
				m.BossHurtPending = ms.BossHurtPending
				m.BossLastHP = ms.BossLastHP
				m.SummonFirstDone = ms.SummonFirstDone
				m.SummonedBy = ms.SummonedBy
				m.CrossfireCD = ms.CrossfireCD
				m.QuestProgressIgnored = ms.QuestProgressIgnored
				// A provoked monster (struck, or spawned hostile by an encounter the
				// player opened) never stands down live - restore that hostility, or a
				// lair dragon "forgets" the fight after a reload and idles point-blank.
				// A quest-bearing encounter monster only exists because the player
				// started that fight (lair/shipwreck/statue), so it counts as provoked
				// even when the flag is absent (saves predating was_attacked).
				// Chest-bound clear-encounter mobs carry no QuestID: normal aggro.
				hostile := ms.WasAttacked ||
					(ms.IsEncounterMonster && ms.EncounterRewards != nil && ms.EncounterRewards.QuestID != "")
				m.WasAttacked = hostile
				m.IsEngagingPlayer = hostile
				// Patron-death revenge persists: a rallied human keeps hunting after reload.
				if ms.Relentless {
					m.Relentless = true
					m.IsEngagingPlayer = true
				}
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
			// Idol-ward immediately at restore. Unlike BossDormant (per-monster
			// above), it's cross-monster - it counts the live idols on THIS map - so
			// it must run after the loop. Same first-frame reason: refreshBoundUndead-
			// Cache recomputes it every frame but runs AFTER input, so without this the
			// warded boss would be hittable on the first frame after a load.
			liveIdols := 0
			for _, mm := range w.Monsters {
				if mm != nil && mm.WarlordIdol && mm.IsAlive() {
					liveIdols++
				}
			}
			if liveIdols > 0 {
				for _, mm := range w.Monsters {
					if mm != nil && mm.WardedByIdols {
						mm.BossWarded = true
					}
				}
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

	// Restore NPC state across maps (visited flags + merchant stock)
	if wm != nil {
		for _, ns := range save.NPCStates {
			w, ok := wm.LoadedMaps[ns.MapKey]
			if !ok {
				continue
			}
			for _, npc := range w.NPCs {
				// Coordinate match when the save has them; legacy saves
				// (X==Y==0) fall back to the old name match.
				if ns.X != 0 || ns.Y != 0 {
					if npc.X != ns.X || npc.Y != ns.Y || npc.Name != ns.Name {
						continue
					}
				} else if npc.Name != ns.Name {
					continue
				}
				npc.Visited = ns.Visited
				// Stock layout comes from YAML; apply saved quantities only
				// while the lengths agree (a YAML edit invalidates the save).
				if len(ns.StockQuantities) == len(npc.MerchantStock) {
					for i, q := range ns.StockQuantities {
						npc.MerchantStock[i].Quantity = q
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
	g.turnBasedExtraMonsterAction = save.ExtraMonsterAction

	// Restore utility/buff state
	g.torchLightActive = save.TorchLightActive
	g.torchLightDuration = save.TorchLightDuration
	// Radius always follows the CURRENT spells.yaml (vision_radius_tiles) -
	// old saves froze whatever value was live when they were written.
	g.torchLightRadius = save.TorchLightRadius
	if g.torchLightActive {
		if def, err := spells.GetSpellDefinitionByID("torch_light"); err == nil && def.VisionRadiusTiles > 0 {
			g.torchLightRadius = def.VisionRadiusTiles
		}
	}
	g.wizardEyeActive = save.WizardEyeActive
	g.wizardEyeDuration = save.WizardEyeDuration
	// Same anti-freeze rule as the torch: an active eye adopts the CURRENT
	// spells.yaml radius instead of a stale or missing saved value.
	if g.wizardEyeActive {
		if def, err := spells.GetSpellDefinitionByID("wizard_eye"); err == nil && def.VisionRadiusTiles > 0 {
			g.wizardEyeRadiusTiles = def.VisionRadiusTiles
		}
	}
	g.walkOnWaterActive = save.WalkOnWaterActive
	g.walkOnWaterDuration = save.WalkOnWaterDuration
	g.statBuffs = restoreStatBuffs(save.StatBuffs)
	if len(g.statBuffs) == 0 && save.BlessActive && save.BlessDuration > 0 {
		// Pre-registry save: bless lived in dedicated fields.
		g.statBuffs = []TimedStatBuff{{
			SpellID: "bless",
			Frames:  save.BlessDuration,
			Bonuses: statBonusesFromSave(save.BlessBonusesPerStat, save.BlessStatBonus),
		}}
	}
	g.combatBuffs = restoreCombatBuffs(save.CombatBuffs)
	g.steamZones = restoreSteamZones(save.SteamZones, save.MapKey)
	g.traps = restoreTraps(save.Traps, g.party)
	g.waterBreathingActive = save.WaterBreathingActive
	g.waterBreathingDuration = save.WaterBreathingDuration
	g.underwaterReturnX = save.UnderwaterReturnX
	g.underwaterReturnY = save.UnderwaterReturnY
	g.underwaterReturnMap = save.UnderwaterReturnMap
	// The aggregate is DERIVED from the restored registry (never trusted from
	// the save) - a drifted legacy save can't turn a buff expiry into a
	// permanent debuff. Also re-derives members' MaxHP/MaxSP under the buffs.
	g.recomputeStatBonuses()
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
	g.victoryAcknowledged = save.VictoryAcknowledged
	g.showHighScores = false

	if g.world != nil {
		g.world.SetWalkOnWaterActive(g.walkOnWaterActive)
		g.world.SetWaterBreathingActive(g.waterBreathingActive)
	}

	// Restore ground containers (loot bags + treasure chests).
	g.groundContainers = make([]GroundContainer, 0, len(save.GroundContainers))
	for _, c := range save.GroundContainers {
		mapKey := c.MapKey
		if mapKey == "" {
			// Legacy saves: loot bags were stored without a map and leaked onto
			// every map. Pin them to the map the save was made on - imperfect for
			// bags dropped elsewhere, but they stop following the party around.
			mapKey = save.MapKey
		}
		restored := GroundContainer{
			Kind:           ContainerKind(c.Kind),
			ID:             c.ID,
			MapKey:         mapKey,
			X:              c.X,
			Y:              c.Y,
			Gold:           c.Gold,
			Sprite:         c.Sprite,
			SizeMultiplier: c.SizeMultiplier,
		}
		// Legacy saves baked the kind's default sprite name in; blank it so the
		// live effectiveSprite() (rarity-aware for loot bags) applies to old bags.
		if restored.Sprite == groundContainerDefaults[restored.Kind].sprite {
			restored.Sprite = ""
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
	// Registry buffs (stat + combat) show their timers immediately too, not
	// only after the first tick refreshes them.
	for _, b := range g.statBuffs {
		g.updateUtilityStatus(spells.SpellID(b.SpellID), b.Frames, true)
	}
	for _, b := range g.combatBuffs {
		g.updateUtilityStatus(spells.SpellID(b.SpellID), b.Frames, true)
	}

	// Restore quest progress. Reset to the baseline (starting quests only) first
	// so quests taken AFTER this save - and therefore absent from it - don't
	// linger on the live manager; then lay the saved snapshot back on top.
	if g.questManager != nil {
		g.questManager.Reset()
		for _, qs := range save.Quests {
			g.questManager.RestoreQuestProgress(qs.ID, quests.QuestStatus(qs.Status), qs.CurrentCount, qs.DynamicTarget, qs.RewardsClaimed)
		}
		// Sync world changes to the LOADED quest state, both ways: completed
		// quests re-lay their tiles, and tiles of quests NOT completed in this
		// save revert to pristine. Loading does NOT reload maps from disk
		// (SwitchToMap flips a key on the shared instances), so a bridge laid
		// earlier this session must be actively taken back out here.
		g.syncQuestTiles()
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

// statBonusesToMap serializes a StatBonuses block for saves (nonzero entries
// only, lowercase keys - same shape spells.yaml authors).
func statBonusesToMap(b character.StatBonuses) map[string]int {
	if b.IsZero() {
		return nil
	}
	m := map[string]int{}
	for _, key := range config.StatNames {
		if v := b.ValueByName(key); v != 0 {
			m[key] = v
		}
	}
	return m
}

// statBonusesFromSave restores a bonus block: per-stat map when present,
// otherwise the legacy uniform int (pre-per-stat saves).
func statBonusesFromSave(perStat map[string]int, legacy int) character.StatBonuses {
	if len(perStat) > 0 {
		return character.StatBonusesFromMap(perStat)
	}
	return character.UniformStatBonuses(legacy)
}
