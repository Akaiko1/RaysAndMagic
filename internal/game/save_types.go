package game

import (
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/spells"
	"ugataima/internal/status"
)

// GameSave captures minimal persistent state for save/load
type GameSave struct {
	// TickRate is the rate its frame counts were written at, config.GetTargetTPS
	// (none: an old save, config.DefaultTPS); a load at another rate rescales
	// them (status.RescaleSaved).
	TickRate           int               `json:"tick_rate,omitempty"`
	PartyHinder        PartyHinderState  `json:"party_hinder,omitzero"`
	Adventure          AdventureState    `json:"adventure,omitempty"`
	Alchemy            AlchemyState      `json:"alchemy,omitempty"`
	SpatialReuse       *status.DualClock `json:"spatial_reuse,omitempty"`
	SpatialReuseFrames int               `json:"spatial_reuse_frames,omitempty" ticks:"frames"` // legacy, read into SpatialReuse
	OpenerCooldown     *status.DualClock `json:"opener_cooldown,omitempty"`
	PartyMoveClock     *status.DualClock `json:"party_move_clock,omitempty"`
	PartyRoot          status.DualClock  `json:"party_root,omitzero"`
	TerrainChanges     []TerrainChange   `json:"terrain_changes,omitempty"`
	Ecology            EcologyState      `json:"ecology,omitempty"`

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
	StoryVersion       int                      `json:"story_version,omitempty"`
	StorySeen          []StorySeenSave          `json:"story_seen,omitempty"`
	StoryQueue         []string                 `json:"story_queue,omitempty"`
	StoryRevealed      []string                 `json:"story_revealed,omitempty"`
	WatcherSeen        []string                 `json:"watcher_seen,omitempty"`
	WatcherLastDay     int                      `json:"watcher_last_day,omitempty"`
	WatcherQueue       []string                 `json:"watcher_queue,omitempty"`
	QuestPropLayouts   map[string]string        `json:"quest_prop_layouts,omitempty"`
	BossFireTraps      []bossFireTrap           `json:"boss_fire_traps,omitempty"`
	BossFireTrapsOwner string                   `json:"boss_fire_traps_owner,omitempty"`
	GroundContainers   []GroundContainerSave    `json:"ground_containers,omitempty"`
	// PendingLevelUpChoices preserves unconsumed skill/spell choices from
	// level-ups and promotions, including their exact options and selections.
	PendingLevelUpChoices []PendingLevelUpChoiceSave `json:"pending_level_up_choices,omitempty"`
	PlayedTimeNs          int64                      `json:"played_time_ns,omitempty"` // Elapsed play time in nanoseconds
	MaxPartyLevel         int                        `json:"max_party_level,omitempty"`
	DayNightFrames        int                        `json:"day_night_frames,omitempty" ticks:"frames"`
	DayNightDay           int                        `json:"day_night_day,omitempty"`
	CalendarDay           int                        `json:"calendar_day,omitempty"`
	CalendarWeek          int                        `json:"calendar_week,omitempty"`
	CalendarMonth         int                        `json:"calendar_month,omitempty"`
	RespawnDayVersion     int                        `json:"respawn_day_version,omitempty"`
	StackLayoutVersion    int                        `json:"stack_layout_version,omitempty"`
	ArenaTierFoughtDay    map[string]int             `json:"arena_tier_fought_day,omitempty"`
	MapRespawnDay         map[string]int             `json:"map_respawn_day,omitempty"` // One-based calendar day; zero means unstamped.
	ArenaRunID            string                     `json:"arena_run_id,omitempty"`
	TotalGoldEarned       int                        `json:"total_gold_earned,omitempty"`
	TotalExperienceEarned int                        `json:"total_experience_earned,omitempty"`
	VictoryAcknowledged   bool                       `json:"victory_acknowledged,omitempty"`
	// StashTransferID is a short-lived commit marker for the shared-stash
	// journal. It is ignored after recovery and carries no gameplay meaning.
	StashTransferID string `json:"stash_transfer_id,omitempty"`

	// Turn-based state
	TurnBasedTurnSuspended      bool `json:"turn_based_turn_suspended,omitempty"`
	CurrentTurn                 int  `json:"current_turn,omitempty"`
	PartyActionsUsed            int  `json:"party_actions_used,omitempty"`
	PartyPenaltyFreeActionsUsed int  `json:"party_technique_actions_used,omitempty"`
	TurnBasedMoveCooldown       int  `json:"turn_based_move_cooldown,omitempty" ticks:"frames"`
	TurnBasedRotCooldown        int  `json:"turn_based_rot_cooldown,omitempty" ticks:"frames"`
	MonsterTurnResolved         bool `json:"monster_turn_resolved,omitempty"`
	// Legacy saves: the party's TB rounds since the last regeneration payouts
	// (a save without the HP count paced both on the SP count).
	LegacyTBSPRegenRounds int  `json:"turn_based_sp_regen_count,omitempty"`
	LegacyTBHPRegenRounds *int `json:"turn_based_hp_regen_count,omitempty"`
	ExtraMonsterAction    bool `json:"extra_monster_action,omitempty"`
	// A save can land during the visible delay before an earned second monster
	// pass. Preserve the in-progress scheduler instead of restarting the turn.
	TurnBasedMonsterPassesLeft int      `json:"turn_based_monster_passes_left,omitempty"`
	TurnBasedMonsterPassDelay  int      `json:"turn_based_monster_pass_delay,omitempty" ticks:"frames"`
	TurnBasedMonsterStatusTick bool     `json:"turn_based_monster_status_tick,omitempty"`
	TurnBasedMonsterStunned    []string `json:"turn_based_monster_stunned,omitempty"`

	// Utility/buff state
	// CardSummonCDFrames is the legacy shared timer (load-only migration).
	CardSummonCDFrames     int                         `json:"card_summon_cd_frames,omitempty" ticks:"frames"`
	CardSummonClocks       map[string]status.DualClock `json:"card_summon_clocks,omitempty"`
	CardSummonCooldowns    map[string]int              `json:"card_summon_cooldowns,omitempty" ticks:"frames"` // legacy frames, read into CardSummonClocks
	TorchLightActive       bool                        `json:"torch_light_active,omitempty"`
	TorchLightDuration     int                         `json:"torch_light_duration,omitempty" ticks:"frames"`
	TorchLightRadius       float64                     `json:"torch_light_radius,omitempty"`
	WizardEyeActive        bool                        `json:"wizard_eye_active,omitempty"`
	WizardEyeDuration      int                         `json:"wizard_eye_duration,omitempty" ticks:"frames"`
	WalkOnWaterActive      bool                        `json:"walk_on_water_active,omitempty"`
	WalkOnWaterDuration    int                         `json:"walk_on_water_duration,omitempty" ticks:"frames"`
	FlyActive              bool                        `json:"fly_active,omitempty"`
	FlyDuration            int                         `json:"fly_duration,omitempty" ticks:"frames"`
	VisitedTavernMaps      []string                    `json:"visited_tavern_maps,omitempty"`
	BlessActive            bool                        `json:"bless_active,omitempty"`
	BlessDuration          int                         `json:"bless_duration,omitempty" ticks:"frames"`
	BlessStatBonus         int                         `json:"bless_stat_bonus,omitempty"`
	StatBuffs              []StatBuffSave              `json:"stat_buffs,omitempty"`
	BlessBonusesPerStat    map[string]int              `json:"bless_bonuses_per_stat,omitempty"`
	CombatBuffs            []CombatBuffSave            `json:"combat_buffs,omitempty"`
	CelestialBuffSpellID   string                      `json:"celestial_buff_spell_id,omitempty"`
	TimedBuffSourceVersion int                         `json:"timed_buff_source_version,omitempty"`
	PersistentDamageZones  []PersistentDamageZoneSave  `json:"steam_zones,omitempty"`
	FrozenWater            []FrozenWaterTile           `json:"frozen_water,omitempty"`
	Traps                  []TrapSave                  `json:"traps,omitempty"`
	WaterBreathingActive   bool                        `json:"water_breathing_active,omitempty"`
	WaterBreathingDuration int                         `json:"water_breathing_duration,omitempty" ticks:"frames"`
	UnderwaterReturnX      float64                     `json:"underwater_return_x,omitempty"`
	UnderwaterReturnY      float64                     `json:"underwater_return_y,omitempty"`
	UnderwaterReturnMap    string                      `json:"underwater_return_map,omitempty"`
	StatBonus              int                         `json:"stat_bonus,omitempty"`

	// MapReturnPoses remembers where the party entered each map via a gate, so a
	// return trip drops them at the doorway rather than the map's spawn tile.
	MapReturnPoses map[string]MapPose `json:"map_return_poses,omitempty"`
}

// QuestSave captures quest progress for save/load
type QuestSave struct {
	Activity         quests.ActivityState `json:"activity,omitempty"`
	ID               string               `json:"id"`
	Status           string               `json:"status"`
	CurrentCount     int                  `json:"current_count"`
	DynamicTarget    int                  `json:"dynamic_target,omitempty"`
	DynamicTargetSet bool                 `json:"dynamic_target_set,omitempty"`
	RewardsClaimed   bool                 `json:"rewards_claimed"`
	ClaimedAtDay     float64              `json:"claimed_at_day,omitempty"`
}

type PartySave struct {
	NonKeyDoorsOpened   int             `json:"non_key_doors_opened,omitempty"`
	ThievesGuildMember  bool            `json:"thieves_guild_member,omitempty"`
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
	ActorID        uint64                                   `json:"actor_id,omitempty"`
	ReactiveCombat character.ReactiveCombatState            `json:"reactive_combat,omitempty"`
	Clockwork      character.ClockworkGearState             `json:"clockwork,omitzero"`
	AdventureXP    map[string]character.AdventureExperience `json:"adventure_xp,omitempty"`
	Inventory      []items.Item                             `json:"inventory,omitempty"`
	RareClass      character.RareClassState                 `json:"rare_class,omitempty"`
	SpellState     character.SpellState                     `json:"spell_state,omitempty"`
	AutoDrinkCD    *status.DualClock                        `json:"auto_drink_cd,omitempty"`
	AutocastCD     *status.DualClock                        `json:"autocast_cd,omitempty"`
	Designation    *status.DualClock                        `json:"designation,omitempty"`
	// Legacy frame counts, read once into the clocks above.
	AutoDrinkCooldown  int    `json:"auto_drink_cooldown,omitempty" ticks:"frames"`
	AutocastFrames     int    `json:"autocast_frames,omitempty" ticks:"frames"`
	DesignatedTargetID string `json:"designated_target_id,omitempty"`
	DesignationFrames  int    `json:"designation_frames,omitempty" ticks:"frames"`
	Name               string `json:"name"`
	Class              int    `json:"class"`
	Race               string `json:"race,omitempty"`
	Promotion          int    `json:"promotion,omitempty"`
	Level              int    `json:"level"`
	Experience         int    `json:"experience"`
	HitPoints          int    `json:"hit_points"`
	MaxHitPoints       int    `json:"max_hit_points"`
	SpellPoints        int    `json:"spell_points"`
	MaxSpellPoints     int    `json:"max_spell_points"`
	Might              int    `json:"might"`
	Intellect          int    `json:"intellect"`
	Personality        int    `json:"personality"`
	Endurance          int    `json:"endurance"`
	Accuracy           int    `json:"accuracy"`
	Speed              int    `json:"speed"`
	Luck               int    `json:"luck"`
	FreeStatPoints     int    `json:"free_stat_points"`
	// PermanentBonuses are one-time permanent stat gains (stat barrels) -
	// effective-stat layer, kept apart from the base stats above.
	PermanentBonuses      map[string]int     `json:"permanent_bonuses,omitempty"`
	OwedLevelChoices      []int              `json:"owed_level_choices,omitempty"`
	Conditions            []int              `json:"conditions"`
	Skills                []SkillEntry       `json:"skills"`
	MagicSchools          []MagicSchoolEntry `json:"magic_schools"`
	Equipment             []EquipmentEntry   `json:"equipment"`
	QuickSlots            []QuickSlotEntry   `json:"quick_slots,omitempty"`
	QuickSecondRow        bool               `json:"quick_second_row,omitempty"`
	PoisonFramesRemaining int                `json:"poison_frames_remaining,omitempty" ticks:"frames"`
	PoisonTickTimer       int                `json:"poison_tick_timer,omitempty" ticks:"frames"`
	BurnFramesRemaining   int                `json:"burn_frames_remaining,omitempty" ticks:"frames"`
	BurnTickTimer         int                `json:"burn_tick_timer,omitempty" ticks:"frames"`
	SPRegen               *status.DualClock  `json:"sp_regen,omitempty"`
	HPRegen               *status.DualClock  `json:"hp_regen,omitempty"`
	SPRegenFight          bool               `json:"sp_regen_fight,omitempty"`
	SpellRegenTimer       int                `json:"spell_regen_timer,omitempty" ticks:"frames"` // legacy RT frames since the last payout
	HPRegenTimer          int                `json:"hp_regen_timer,omitempty" ticks:"frames"`    // legacy, as SpellRegenTimer
	Stun                  *status.DualClock  `json:"stun_clock,omitempty"`
	StunDRStacks          int                `json:"stun_dr_stacks,omitempty"`
	StunDRMemory          *status.DualClock  `json:"stun_dr_memory_clock,omitempty"`
	ArmorMelt             *status.DualClock  `json:"armor_melt,omitempty"`
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
	RTCooldown int `json:"rt_cooldown,omitempty" ticks:"frames"`
	// OffHandRTCooldown mirrors RTCooldown for a Dual Wielding character's
	// off-hand weapon.
	OffHandRTCooldown int `json:"off_hand_rt_cooldown,omitempty" ticks:"frames"`
	// NextTBAttackOffHand preserves which hand a Dual Wielding character prefers
	// next, so save/reload can't be used to re-pick the preferred weapon.
	NextTBAttackOffHand bool `json:"next_tb_attack_off_hand,omitempty"`
	CharacterLegacyClocks
}

// CharacterLegacyClocks are the stun pairs saves wrote before each saved one
// DualClock; read on load only.
type CharacterLegacyClocks struct {
	StunFramesRemaining int `json:"stun_frames_remaining,omitempty" ticks:"frames"`
	StunTurnsRemaining  int `json:"stun_turns_remaining,omitempty"`
	StunRate            int `json:"stun_rate,omitempty" ticks:"frames"`
	StunDRMemoryTurns   int `json:"stun_dr_memory_turns,omitempty"`
	StunDRMemoryFrames  int `json:"stun_dr_memory_frames,omitempty" ticks:"frames"`
}

// legacyRegen reports a save from before the regeneration periods.
func (cs CharacterSave) legacyRegen() bool { return cs.SPRegen == nil && cs.HPRegen == nil }

// SkillEntry persists one skill. Mastery is the truth; Level is the derived
// label (Mastery+1) written for older builds and read back ONLY through
// MasteryForLevel, which migrates pre-mastery saves where level WAS the stat.
type SkillEntry struct {
	Type    int `json:"type"`
	Level   int `json:"level"`
	Mastery int `json:"mastery"`
}

// PendingLevelUpChoiceSave records that party member CharIndex has earned a
// level-up or promotion choice. Old saves without options use class/level
// reconstruction; level zero identifies the legacy promotion picker.
type PendingLevelUpChoiceSave struct {
	CharIndex     int                        `json:"char_index"`
	Level         int                        `json:"level"`
	Options       []PendingLevelUpOptionSave `json:"options,omitempty"`
	MaxSelections int                        `json:"max_selections,omitempty"`
	Selected      []bool                     `json:"selected,omitempty"`
	Selection     int                        `json:"selection,omitempty"`
	Title         string                     `json:"title,omitempty"`
	PadToMinimum  bool                       `json:"pad_to_minimum,omitempty"`
}

// PendingLevelUpOptionSave preserves resolved random mastery options as well as spells.
type PendingLevelUpOptionSave struct {
	Choice    config.LevelUpChoice    `json:"choice"`
	SkillType character.SkillType     `json:"skill_type"`
	School    character.MagicSchoolID `json:"school,omitempty"`
	SpellID   spells.SpellID          `json:"spell_id,omitempty"`
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

type MonsterAggroSave struct {
	Retaliation    monster.RetaliationState `json:"retaliation,omitzero"`
	BandHitPending bool                     `json:"band_hit_pending,omitempty"`
	Engaged        bool                     `json:"engaged,omitempty"`
	ReturningHome  bool                     `json:"returning_home,omitempty"`
}

type MonsterSave struct {
	Relics        monster.RelicState     `json:"relics,omitzero"`
	Telegraph     monster.TelegraphState `json:"telegraph,omitzero"`
	BandInstance  string                 `json:"band_instance,omitempty"`
	AmbientThreat monster.AmbientThreat  `json:"ambient_threat,omitzero"`
	Arbor         monster.ArborealState  `json:"arboreal,omitzero"`
	Population    string                 `json:"population,omitempty"`

	ID        string  `json:"id,omitempty"`
	Key       string  `json:"key"`
	Name      string  `json:"name"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	HitPoints int     `json:"hit_points"`
	// Optional for legacy saves; coordinates use the same map space as X/Y.
	SpawnPosition *[2]float64 `json:"spawn_position,omitempty"`
	// Nil migrates legacy saves; explicit zero preserves an unscaled actor.
	AdventureScaleLevel *int `json:"adventure_scale_level,omitempty"`
	// Pure party summons can replace their YAML stats at runtime from the
	// summoner's mastery. Keep the snapshot optional so ordinary monsters still
	// pick up current balance values from monsters.yaml after a load.
	RuntimeStats                   *MonsterRuntimeStatsSave `json:"runtime_stats,omitempty"`
	Bound                          bool                     `json:"bound,omitempty"`
	BoundFramesRemaining           int                      `json:"bound_frames_remaining,omitempty" ticks:"frames"`
	FearFramesRemaining            int                      `json:"fear_frames_remaining,omitempty" ticks:"frames"`
	SilenceImmunityFramesRemaining int                      `json:"silence_immunity_frames_remaining,omitempty" ticks:"frames"`
	SilenceFramesRemaining         int                      `json:"silence_frames_remaining,omitempty" ticks:"frames"`
	BerserkFramesRemaining         int                      `json:"berserk_frames_remaining,omitempty" ticks:"frames"`
	Pacified                       bool                     `json:"pacified,omitempty"`
	PacifiedFramesRemaining        int                      `json:"pacified_frames_remaining,omitempty" ticks:"frames"`
	CharmedByParty                 bool                     `json:"charmed_by_party,omitempty"`
	WasAttacked                    bool                     `json:"was_attacked,omitempty"`
	// Aggro distinguishes active pursuit from permanent provocation. Nil means
	// a legacy save whose old flags must be reconciled against the current scope.
	Aggro                 *MonsterAggroSave `json:"aggro,omitempty"`
	TurnBasedSightEngaged bool              `json:"turn_based_sight_engaged,omitempty"` // legacy read only
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
	HomeMap              string `json:"home_map,omitempty"` // map key the monster was created in (target_map quests)
	// Mid-combat cooldowns: reload must not strip a player-applied stun or
	// reset the monster's special-attack cooldowns.
	Stun                    *status.DualClock `json:"stun_clock,omitempty"`
	PoisonedFramesRemaining int               `json:"poisoned_frames_remaining,omitempty" ticks:"frames"` // Venom-proc cards
	PoisonTickTimer         int               `json:"poison_tick_timer,omitempty" ticks:"frames"`
	// Stun diminishing-returns chain - persisted so save/reload can't reset it
	// and re-enable a full-strength perma-stun-lock (bosses included).
	StunDRStacks        int                                    `json:"stun_dr_stacks,omitempty"`
	StunDRMemory        *status.DualClock                      `json:"stun_dr_memory_clock,omitempty"`
	Root                *status.DualClock                      `json:"root_clock,omitempty"`
	ArmorShredPct       int                                    `json:"armor_shred_pct,omitempty"`
	ElementalMarks      map[string]monster.ElementalWeaponMark `json:"elemental_marks,omitempty"`
	ArmorShred          *status.DualClock                      `json:"armor_shred_clock,omitempty"`
	ArmorMelt           *status.DualClock                      `json:"armor_melt,omitempty"`
	BurnFramesRemaining int                                    `json:"burn_frames_remaining,omitempty" ticks:"frames"`
	BurnTickTimer       int                                    `json:"burn_tick_timer,omitempty" ticks:"frames"`
	TrapVolleyCD        *status.DualClock                      `json:"trap_volley_clock,omitempty"`
	SlowPct             int                                    `json:"slow_pct,omitempty"`
	Slow                *status.DualClock                      `json:"slow_clock,omitempty"`
	SlowSkip            *status.DualClock                      `json:"slow_skip,omitempty"`
	SlowTurnCount       int                                    `json:"slow_turn_count,omitempty"` // legacy, read into SlowSkip
	RootHeldThisTurn    bool                                   `json:"root_held_this_turn,omitempty"`
	SlowPctThisTurn     int                                    `json:"slow_pct_this_turn,omitempty"`
	WeakenPct           int                                    `json:"weaken_pct,omitempty"`
	Weaken              *status.DualClock                      `json:"weaken_clock,omitempty"`
	WeakenPctThisTurn   int                                    `json:"weaken_pct_this_turn,omitempty"`
	Pilfered            bool                                   `json:"pilfered,omitempty"`
	PounceCD            *status.DualClock                      `json:"pounce_clock,omitempty"`
	PhaseShiftCD        *status.DualClock                      `json:"phase_shift_clock,omitempty"`
	TurnBasedSwingCarry float64                                `json:"tb_swing_carry,omitempty"`                    // fractional TB swing banked between attack actions
	TurnBasedStepCarry  float64                                `json:"tb_step_carry,omitempty"`                     // fractional TB step banked between movement actions
	Jammed              bool                                   `json:"jammed,omitempty"`                            // a jam already landed; more hits add nothing until the delayed attack
	AttackCDFrames      int                                    `json:"attack_cd_frames,omitempty" ticks:"frames"`   // real-time attack cooldown, a jam's delay included
	OffHandCDFrames     int                                    `json:"off_hand_cd_frames,omitempty" ticks:"frames"` // a champion's off-hand cooldown
	HourMarks           int                                    `json:"hour_marks,omitempty"`                        // Minute Hand hits toward the hour strike
	BossCD              *status.DualClock                      `json:"boss_blink_cd,omitempty"`
	NovaRollCD          *status.DualClock                      `json:"inferno_roll_cd,omitempty"`           // key kept from the inferno-only days
	LegacyBossCD        int                                    `json:"boss_cd,omitempty" ticks:"frames"`    // frames, read into BossCD
	LegacyInfernoCD     int                                    `json:"inferno_cd,omitempty" ticks:"frames"` // frames, read into NovaRollCD
	BossHurtPending     bool                                   `json:"boss_hurt_pending,omitempty"`
	BossLastHP          int                                    `json:"boss_last_hp,omitempty"`
	SummonFirstDone     bool                                   `json:"summon_first_done,omitempty"`
	SummonCD            *status.DualClock                      `json:"summon_cd,omitempty"`
	PushCD              *status.DualClock                      `json:"push_cd,omitempty"`
	LaneCD              *status.DualClock                      `json:"lane_cd,omitempty"`
	SummonedBy          string                                 `json:"summoned_by,omitempty"`
	SummonerName        string                                 `json:"summoner_name,omitempty"`
	IsEncounterMonster  bool                                   `json:"is_encounter_monster,omitempty"`
	ChampionTier        string                                 `json:"champion_tier,omitempty"`
	OpeningSpellDone    bool                                   `json:"opening_spell_done,omitempty"`
	EnrageAnnounced     bool                                   `json:"enrage_announced,omitempty"`
	SoakDamage          int                                    `json:"soak_damage,omitempty"`
	Soak                *status.DualClock                      `json:"soak_clock,omitempty"`
	EncounterID         int                                    `json:"encounter_id,omitempty"`
	EncounterRewards    *EncounterRewardSave                   `json:"encounter_rewards,omitempty"`
	MonsterLegacyClocks
}

// MonsterLegacyClocks are the frames/turns/rate triples saves wrote before
// each status saved one DualClock; read on load only.
type MonsterLegacyClocks struct {
	StunFramesRemaining int `json:"stun_frames_remaining,omitempty" ticks:"frames"`
	StunTurnsRemaining  int `json:"stun_turns_remaining,omitempty"`
	StunRate            int `json:"stun_rate,omitempty" ticks:"frames"`
	StunDRMemoryTurns   int `json:"stun_dr_memory_turns,omitempty"`
	StunDRMemoryFrames  int `json:"stun_dr_memory_frames,omitempty" ticks:"frames"`
	RootFramesRemaining int `json:"root_frames_remaining,omitempty" ticks:"frames"`
	RootTurnsRemaining  int `json:"root_turns_remaining,omitempty"`
	RootRate            int `json:"root_rate,omitempty" ticks:"frames"`
	ArmorShredFrames    int `json:"armor_shred_frames,omitempty" ticks:"frames"`
	ArmorShredTurns     int `json:"armor_shred_turns,omitempty"`
	ArmorShredRate      int `json:"armor_shred_rate,omitempty" ticks:"frames"`
	TrapVolleyCDFrames  int `json:"trap_volley_cd,omitempty" ticks:"frames"`
	TrapVolleyTurnCD    int `json:"trap_volley_turn_cd,omitempty"`
	TrapVolleyCDRate    int `json:"trap_volley_cd_rate,omitempty" ticks:"frames"`
	SlowFrames          int `json:"slow_frames,omitempty" ticks:"frames"`
	SlowTurns           int `json:"slow_turns,omitempty"`
	SlowRate            int `json:"slow_rate,omitempty" ticks:"frames"`
	WeakenFrames        int `json:"weaken_frames,omitempty" ticks:"frames"`
	WeakenTurns         int `json:"weaken_turns,omitempty"`
	WeakenRate          int `json:"weaken_rate,omitempty" ticks:"frames"`
	PounceCDFrames      int `json:"pounce_cd_frames,omitempty" ticks:"frames"`
	PounceCDTurns       int `json:"pounce_cd_turns,omitempty"`
	PounceCDRate        int `json:"pounce_cd_rate,omitempty" ticks:"frames"`
	PhaseShiftCDFrames  int `json:"phase_shift_cd_frames,omitempty" ticks:"frames"`
	PhaseShiftCDTurns   int `json:"phase_shift_cd_turns,omitempty"`
	PhaseShiftCDRate    int `json:"phase_shift_cd_rate,omitempty" ticks:"frames"`
	SoakFrames          int `json:"soak_frames,omitempty" ticks:"frames"`
	SoakTurns           int `json:"soak_turns,omitempty"`
	SoakRate            int `json:"soak_rate,omitempty" ticks:"frames"`
}

type MonsterRuntimeStatsSave struct {
	MaxHitPoints int `json:"max_hit_points"`
	ArmorClass   int `json:"armor_class"`
	DamageMin    int `json:"damage_min"`
	DamageMax    int `json:"damage_max"`
}

type EncounterRewardSave struct {
	FreesCaptives     bool                      `json:"frees_captives,omitempty"`
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
	MapKey         string                      `json:"map_key"`
	Key            string                      `json:"key,omitempty"` // npcs.yaml key: the identity, with the spot
	Name           string                      `json:"name"`          // identity of saves from before Key
	X              float64                     `json:"x,omitempty"`
	Y              float64                     `json:"y,omitempty"`
	Visited        bool                        `json:"visited"`
	DoorAttempts   int                         `json:"door_attempts,omitempty"`
	DoorLockBroken bool                        `json:"door_lock_broken,omitempty"`
	DoorOpenedBy   character.DoorOpeningMethod `json:"door_opened_by,omitempty"`
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
	Key string `json:"key,omitempty"`
	// Name identifies the ware in saves from before keys.
	Name     string `json:"name,omitempty"`
	Quantity int    `json:"quantity"`
}
