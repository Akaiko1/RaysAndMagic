package game

import (
	"ugataima/internal/items"
)

// GameSave captures minimal persistent state for save/load
type GameSave struct {
	MapKey             string                   `json:"map_key"`
	PlayerX            float64                  `json:"player_x"`
	PlayerY            float64                  `json:"player_y"`
	PlayerAngle        float64                  `json:"player_angle"`
	TurnBased          bool                     `json:"turn_based"`
	SaveName           string                   `json:"save_name,omitempty"`
	SavedAt            string                   `json:"saved_at"`
	Party              PartySave                `json:"party"`
	Monsters           []MonsterSave            `json:"monsters"`
	MapMonsters        map[string][]MonsterSave `json:"map_monsters,omitempty"`
	NPCStates          []NPCSave                `json:"npc_states"`
	Quests             []QuestSave              `json:"quests,omitempty"`
	QuestSpawnsDone    []string                 `json:"quest_spawns_done,omitempty"`
	BossFireTraps      []bossFireTrap           `json:"boss_fire_traps,omitempty"`
	BossFireTrapsOwner string                   `json:"boss_fire_traps_owner,omitempty"`
	GroundContainers   []GroundContainerSave    `json:"ground_containers,omitempty"`
	// PendingLevelUpChoices preserves unconsumed skill/spell choices from
	// level-ups. Options are rebuilt from class+level on load, so we only
	// need to remember which character is owed a choice at which level.
	PendingLevelUpChoices []PendingLevelUpChoiceSave `json:"pending_level_up_choices,omitempty"`
	PlayedTimeNs          int64                      `json:"played_time_ns,omitempty"` // Elapsed play time in nanoseconds
	DayNightFrames        int                        `json:"day_night_frames,omitempty"`
	DayNightDay           int                        `json:"day_night_day,omitempty"`
	CalendarDay           int                        `json:"calendar_day,omitempty"`
	CalendarWeek          int                        `json:"calendar_week,omitempty"`
	CalendarMonth         int                        `json:"calendar_month,omitempty"`
	ArenaTierFoughtDay    map[string]int             `json:"arena_tier_fought_day,omitempty"`
	MapRespawnDay         map[string]int             `json:"map_respawn_day,omitempty"` // respawn_days maps: day the roster was last spawned (+1 sentinel form)
	ArenaRunID            string                     `json:"arena_run_id,omitempty"`
	TotalGoldEarned       int                        `json:"total_gold_earned,omitempty"`
	TotalExperienceEarned int                        `json:"total_experience_earned,omitempty"`
	VictoryAcknowledged   bool                       `json:"victory_acknowledged,omitempty"`
	// StashTransferID is a short-lived commit marker for the shared-stash
	// journal. It is ignored after recovery and carries no gameplay meaning.
	StashTransferID string `json:"stash_transfer_id,omitempty"`

	// Turn-based state
	TurnBasedTurnSuspended bool `json:"turn_based_turn_suspended,omitempty"`
	CurrentTurn            int  `json:"current_turn,omitempty"`
	PartyActionsUsed       int  `json:"party_actions_used,omitempty"`
	TurnBasedMoveCooldown  int  `json:"turn_based_move_cooldown,omitempty"`
	TurnBasedRotCooldown   int  `json:"turn_based_rot_cooldown,omitempty"`
	MonsterTurnResolved    bool `json:"monster_turn_resolved,omitempty"`
	TurnBasedSpRegenCount  int  `json:"turn_based_sp_regen_count,omitempty"`
	ExtraMonsterAction     bool `json:"extra_monster_action,omitempty"`
	// A save can land during the visible delay before an earned second monster
	// pass. Preserve the in-progress scheduler instead of restarting the turn.
	TurnBasedMonsterPassesLeft int      `json:"turn_based_monster_passes_left,omitempty"`
	TurnBasedMonsterPassDelay  int      `json:"turn_based_monster_pass_delay,omitempty"`
	TurnBasedMonsterStatusTick bool     `json:"turn_based_monster_status_tick,omitempty"`
	TurnBasedMonsterStunned    []string `json:"turn_based_monster_stunned,omitempty"`

	// Utility/buff state
	// CardSummonCDFrames is the legacy shared timer (load-only migration).
	CardSummonCDFrames     int                        `json:"card_summon_cd_frames,omitempty"`
	CardSummonCooldowns    map[string]int             `json:"card_summon_cooldowns,omitempty"`
	TorchLightActive       bool                       `json:"torch_light_active,omitempty"`
	TorchLightDuration     int                        `json:"torch_light_duration,omitempty"`
	TorchLightRadius       float64                    `json:"torch_light_radius,omitempty"`
	WizardEyeActive        bool                       `json:"wizard_eye_active,omitempty"`
	WizardEyeDuration      int                        `json:"wizard_eye_duration,omitempty"`
	WalkOnWaterActive      bool                       `json:"walk_on_water_active,omitempty"`
	WalkOnWaterDuration    int                        `json:"walk_on_water_duration,omitempty"`
	FlyActive              bool                       `json:"fly_active,omitempty"`
	FlyDuration            int                        `json:"fly_duration,omitempty"`
	VisitedTavernMaps      []string                   `json:"visited_tavern_maps,omitempty"`
	BlessActive            bool                       `json:"bless_active,omitempty"`
	BlessDuration          int                        `json:"bless_duration,omitempty"`
	BlessStatBonus         int                        `json:"bless_stat_bonus,omitempty"`
	StatBuffs              []StatBuffSave             `json:"stat_buffs,omitempty"`
	BlessBonusesPerStat    map[string]int             `json:"bless_bonuses_per_stat,omitempty"`
	CombatBuffs            []CombatBuffSave           `json:"combat_buffs,omitempty"`
	CelestialBuffSpellID   string                     `json:"celestial_buff_spell_id,omitempty"`
	TimedBuffSourceVersion int                        `json:"timed_buff_source_version,omitempty"`
	PersistentDamageZones  []PersistentDamageZoneSave `json:"steam_zones,omitempty"`
	Traps                  []TrapSave                 `json:"traps,omitempty"`
	WaterBreathingActive   bool                       `json:"water_breathing_active,omitempty"`
	WaterBreathingDuration int                        `json:"water_breathing_duration,omitempty"`
	UnderwaterReturnX      float64                    `json:"underwater_return_x,omitempty"`
	UnderwaterReturnY      float64                    `json:"underwater_return_y,omitempty"`
	UnderwaterReturnMap    string                     `json:"underwater_return_map,omitempty"`
	StatBonus              int                        `json:"stat_bonus,omitempty"`

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
	ArenaPoints         int             `json:"arena_points,omitempty"`
	Inventory           []items.Item    `json:"inventory"`
	Members             []CharacterSave `json:"members"`
	Reserve             []CharacterSave `json:"reserve,omitempty"`
	Captive             []CharacterSave `json:"captive,omitempty"`
	CardCollection      []string        `json:"card_collection,omitempty"`       // legacy/card UI keys
	CardCollectionItems []items.Item    `json:"card_collection_items,omitempty"` // physical cards with InstanceID
}

type CharacterSave struct {
	Name           string `json:"name"`
	Class          int    `json:"class"`
	Race           string `json:"race,omitempty"`
	Promotion      int    `json:"promotion,omitempty"`
	Level          int    `json:"level"`
	Experience     int    `json:"experience"`
	HitPoints      int    `json:"hit_points"`
	MaxHitPoints   int    `json:"max_hit_points"`
	SpellPoints    int    `json:"spell_points"`
	MaxSpellPoints int    `json:"max_spell_points"`
	Might          int    `json:"might"`
	Intellect      int    `json:"intellect"`
	Personality    int    `json:"personality"`
	Endurance      int    `json:"endurance"`
	Accuracy       int    `json:"accuracy"`
	Speed          int    `json:"speed"`
	Luck           int    `json:"luck"`
	FreeStatPoints int    `json:"free_stat_points"`
	// PermanentBonuses are one-time permanent stat gains (stat barrels) -
	// effective-stat layer, kept apart from the base stats above.
	PermanentBonuses      map[string]int     `json:"permanent_bonuses,omitempty"`
	OwedLevelChoices      []int              `json:"owed_level_choices,omitempty"`
	Conditions            []int              `json:"conditions"`
	Skills                []SkillEntry       `json:"skills"`
	MagicSchools          []MagicSchoolEntry `json:"magic_schools"`
	Equipment             []EquipmentEntry   `json:"equipment"`
	QuickSlots            []QuickSlotEntry   `json:"quick_slots,omitempty"`
	PoisonFramesRemaining int                `json:"poison_frames_remaining,omitempty"`
	PoisonTickTimer       int                `json:"poison_tick_timer,omitempty"`
	BurnFramesRemaining   int                `json:"burn_frames_remaining,omitempty"`
	BurnTickTimer         int                `json:"burn_tick_timer,omitempty"`
	StunFramesRemaining   int                `json:"stun_frames_remaining,omitempty"`
	StunTurnsRemaining    int                `json:"stun_turns_remaining,omitempty"`
	StunRate              int                `json:"stun_rate,omitempty"`
	// ActionsRemaining preserves mid-round turn-based state so save/reload
	// can't be used to refill action slots. It also survives an RT save made
	// while a Tab-suspended TB turn is waiting to resume.
	ActionsRemaining int `json:"actions_remaining,omitempty"`
	// TBRoundActionFloor is the equipment-derived floor credited when this
	// round began. It prevents save/load plus a gear swap from transferring
	// Autofire actions to another weapon.
	TBRoundActionFloor int `json:"tb_round_action_floor,omitempty"`
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

// SkillEntry persists one skill. Mastery is the truth; Level is the derived
// label (Mastery+1) written for older builds and read back ONLY through
// MasteryForLevel, which migrates pre-mastery saves where level WAS the stat.
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

// MagicSchoolEntry persists one magic school. Level is the same derived label as
// SkillEntry.Level - NOT a spell level and not a stat (spells have no level at
// all; a school's power is Mastery). Known spells are stored by ID.
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
	Kind      int          `json:"kind"`
	ID        string       `json:"id,omitempty"`
	MapKey    string       `json:"map_key,omitempty"`
	X         float64      `json:"x"`
	Y         float64      `json:"y"`
	Gold      int          `json:"gold"`
	Items     []items.Item `json:"items,omitempty"`
	Sprite    string       `json:"sprite,omitempty"`
	SizeTiles float64      `json:"size_tiles"`
}

type MonsterSave struct {
	ID        string  `json:"id,omitempty"`
	Key       string  `json:"key"`
	Name      string  `json:"name"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	HitPoints int     `json:"hit_points"`
	// Pure party summons can replace their YAML stats at runtime from the
	// summoner's mastery. Keep the snapshot optional so ordinary monsters still
	// pick up current balance values from monsters.yaml after a load.
	RuntimeStats            *MonsterRuntimeStatsSave `json:"runtime_stats,omitempty"`
	Bound                   bool                     `json:"bound,omitempty"`
	BoundFramesRemaining    int                      `json:"bound_frames_remaining,omitempty"`
	Pacified                bool                     `json:"pacified,omitempty"`
	PacifiedFramesRemaining int                      `json:"pacified_frames_remaining,omitempty"`
	CharmedByParty          bool                     `json:"charmed_by_party,omitempty"`
	WasAttacked             bool                     `json:"was_attacked,omitempty"`
	// Normal sight engagement is sticky in TB but non-sticky in RT. Only the TB
	// semantic case is saved, never the raw runtime flag.
	TurnBasedSightEngaged bool `json:"turn_based_sight_engaged,omitempty"`
	// A calm guard reservation is gameplay state: without it, a reload can make
	// a patrolling mob forget the crate/lectern it was already posted at.
	LootGuarding         bool   `json:"loot_guarding,omitempty"`
	LootGuardTargetKey   string `json:"loot_guard_target_key,omitempty"`
	LootGuardTargetTileX int    `json:"loot_guard_target_tile_x,omitempty"`
	LootGuardTargetTileY int    `json:"loot_guard_target_tile_y,omitempty"`
	LootGuardSide        int    `json:"loot_guard_side,omitempty"`
	LootGuardPatrolAlt   bool   `json:"loot_guard_patrol_alt,omitempty"`
	LootGuardAlerted     bool   `json:"loot_guard_alerted,omitempty"`
	RallyDone            bool   `json:"rally_done,omitempty"`
	Relentless           bool   `json:"relentless,omitempty"` // patron-death revenge: relentless map-wide hunt, survives reload
	PackKey              string `json:"pack_key,omitempty"`   // ambient day/night pack tag
	QuestProgressIgnored bool   `json:"quest_progress_ignored,omitempty"`
	// Mid-combat cooldowns: reload must not strip a player-applied stun or
	// reset the monster's special-attack cooldowns.
	StunFramesRemaining     int `json:"stun_frames_remaining,omitempty"`
	StunTurnsRemaining      int `json:"stun_turns_remaining,omitempty"`
	StunRate                int `json:"stun_rate,omitempty"`
	PoisonedFramesRemaining int `json:"poisoned_frames_remaining,omitempty"` // Venom-proc cards
	PoisonTickTimer         int `json:"poison_tick_timer,omitempty"`
	// Stun diminishing-returns chain - persisted so save/reload can't reset it
	// and re-enable a full-strength perma-stun-lock (bosses included).
	StunDRStacks        int                  `json:"stun_dr_stacks,omitempty"`
	StunDRMemoryTurns   int                  `json:"stun_dr_memory_turns,omitempty"`
	StunDRMemoryFrames  int                  `json:"stun_dr_memory_frames,omitempty"`
	RootFramesRemaining int                  `json:"root_frames_remaining,omitempty"`
	RootTurnsRemaining  int                  `json:"root_turns_remaining,omitempty"`
	RootRate            int                  `json:"root_rate,omitempty"`
	ArmorShredPct       int                  `json:"armor_shred_pct,omitempty"`
	ArmorShredFrames    int                  `json:"armor_shred_frames,omitempty"`
	ArmorShredTurns     int                  `json:"armor_shred_turns,omitempty"`
	ArmorShredRate      int                  `json:"armor_shred_rate,omitempty"`
	BurnFramesRemaining int                  `json:"burn_frames_remaining,omitempty"`
	BurnTickTimer       int                  `json:"burn_tick_timer,omitempty"`
	TrapVolleyCD        int                  `json:"trap_volley_cd,omitempty"`
	TrapVolleyTurnCD    int                  `json:"trap_volley_turn_cd,omitempty"`
	TrapVolleyCDRate    int                  `json:"trap_volley_cd_rate,omitempty"`
	SlowPct             int                  `json:"slow_pct,omitempty"`
	SlowFrames          int                  `json:"slow_frames,omitempty"`
	SlowTurns           int                  `json:"slow_turns,omitempty"`
	SlowRate            int                  `json:"slow_rate,omitempty"`
	SlowPctThisTurn     int                  `json:"slow_pct_this_turn,omitempty"`
	WeakenPct           int                  `json:"weaken_pct,omitempty"`
	WeakenFrames        int                  `json:"weaken_frames,omitempty"`
	WeakenTurns         int                  `json:"weaken_turns,omitempty"`
	WeakenRate          int                  `json:"weaken_rate,omitempty"`
	WeakenPctThisTurn   int                  `json:"weaken_pct_this_turn,omitempty"`
	Pilfered            bool                 `json:"pilfered,omitempty"`
	PounceCDFrames      int                  `json:"pounce_cd_frames,omitempty"`
	PounceCDTurns       int                  `json:"pounce_cd_turns,omitempty"`
	PounceCDRate        int                  `json:"pounce_cd_rate,omitempty"`
	BossCD              int                  `json:"boss_cd,omitempty"`
	InfernoCD           int                  `json:"inferno_cd,omitempty"`
	BossHurtPending     bool                 `json:"boss_hurt_pending,omitempty"`
	BossLastHP          int                  `json:"boss_last_hp,omitempty"`
	SummonFirstDone     bool                 `json:"summon_first_done,omitempty"`
	SummonedBy          string               `json:"summoned_by,omitempty"`
	IsEncounterMonster  bool                 `json:"is_encounter_monster,omitempty"`
	ChampionTier        string               `json:"champion_tier,omitempty"`
	OpeningSpellDone    bool                 `json:"opening_spell_done,omitempty"`
	SoakDamage          int                  `json:"soak_damage,omitempty"`
	SoakFrames          int                  `json:"soak_frames,omitempty"`
	SoakTurns           int                  `json:"soak_turns,omitempty"`
	SoakRate            int                  `json:"soak_rate,omitempty"`
	EncounterID         int                  `json:"encounter_id,omitempty"`
	EncounterRewards    *EncounterRewardSave `json:"encounter_rewards,omitempty"`
}

type MonsterRuntimeStatsSave struct {
	MaxHitPoints int `json:"max_hit_points"`
	ArmorClass   int `json:"armor_class"`
	DamageMin    int `json:"damage_min"`
	DamageMax    int `json:"damage_max"`
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
	SizeTiles         float64  `json:"size_tiles,omitempty"`
	RandomWeaponCount int      `json:"random_weapon_count,omitempty"`
	Items             []string `json:"items,omitempty"`
	Weapons           []string `json:"weapons,omitempty"`
	Gold              int      `json:"gold,omitempty"`
	LootTable         string   `json:"loot_table,omitempty"`
	CompletionMessage string   `json:"completion_message,omitempty"`
}

// NPCSave tracks persistent NPC flags across maps. Identity is MapKey + spawn
// coordinates (deterministic from the map file) - display names can repeat on
// one map (e.g. two "City Gate" NPCs), coordinates can't. Legacy saves without
// coordinates fall back to name matching on restore.
type NPCSave struct {
	MapKey         string  `json:"map_key"`
	Name           string  `json:"name"`
	X              float64 `json:"x,omitempty"`
	Y              float64 `json:"y,omitempty"`
	Visited        bool    `json:"visited"`
	DoorAttempts   int     `json:"door_attempts,omitempty"`
	DoorLockBroken bool    `json:"door_lock_broken,omitempty"`
	// Remaining merchant stock, keyed by item NAME in stock order (duplicate
	// names consume sequentially). Index-aligned restore was abandoned: stock
	// ORDER is a presentation detail (grouping can reorder it between versions)
	// and an index-keyed save would stamp quantities onto the wrong items.
	Stock []NPCStockSave `json:"stock,omitempty"`
	// StockQuantities is the retired index-aligned format; old saves carrying
	// it reset to full stock rather than risk mis-assignment.
	StockQuantities []int `json:"stock_quantities,omitempty"`
}

// NPCStockSave is one merchant stock line in a save.
type NPCStockSave struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}
