package monster

import (
	"math"
	"math/rand"
	"strconv"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/status"
)

// EncounterRewards represents rewards for completing an encounter
type EncounterRewards struct {
	Gold              int                   `yaml:"gold"`
	Experience        int                   `yaml:"experience"`
	CompletionMessage string                `yaml:"completion_message"`        // Configurable message shown when encounter is completed
	TreasureChest     *TreasureChestReward  `yaml:"treasure_chest,omitempty"`  // Optional chest spawned when encounter is completed
	TreasureChests    []TreasureChestReward `yaml:"treasure_chests,omitempty"` // Optional chests spawned when encounter is completed
	QuestID           string                `yaml:"-"`                         // Quest ID linked to this encounter (set at runtime, not from YAML)
	FreesCaptives     bool                  `yaml:"frees_captives,omitempty"`  // On clear, move the party's imprisoned heroes into the reserve roster
}

// TreasureChestReward describes a chest spawned after an encounter is cleared.
type TreasureChestReward struct {
	ID                string   `yaml:"id,omitempty"`
	Map               string   `yaml:"map,omitempty"`
	TileX             int      `yaml:"tile_x"`
	TileY             int      `yaml:"tile_y"`
	Sprite            string   `yaml:"sprite,omitempty"`
	SizeMultiplier    float64  `yaml:"size_multiplier,omitempty"`
	RandomWeaponCount int      `yaml:"random_weapon_count,omitempty"`
	Items             []string `yaml:"items,omitempty"`
	Weapons           []string `yaml:"weapons,omitempty"`
	Gold              int      `yaml:"gold,omitempty"`
	LootTable         string   `yaml:"loot_table,omitempty"`
	CompletionMessage string   `yaml:"completion_message,omitempty"`
}

// PartyTraits holds the party's currently-active "traits" (e.g. "lich"),
// refreshed once per frame before monster updates. A passive monster turns
// hostile on sight only if one of ITS hated traits (HatesTraits, from hates.yaml)
// is present here - so a Lich enrages just archmages and elf warriors, not every
// passive monster. Mutated in place to avoid per-frame allocation.
var PartyTraits = map[string]bool{}

// relentlessHunter reports whether this monster pursues the party MAP-WIDE: a boss
// turned aggressive (BossAggro) or a non-boss rallied for revenge (Relentless,
// e.g. Amazons after their Warlord dies). Both ignore detection/LoS AND must get
// the widened A* window + node budget, or they'd stay on a normal mob's reach and
// fail to path across a large/maze map despite being "hostile from anywhere".
func (m *Monster3D) relentlessHunter() bool { return m.BossAggro || m.Relentless }

// HatesActiveTrait reports whether any of this monster's hated party traits is
// currently active - i.e. whether a passive monster should turn hostile on sight.
func (m *Monster3D) HatesActiveTrait() bool {
	for _, t := range m.HatesTraits {
		if PartyTraits[t] {
			return true
		}
	}
	return false
}

// Global counter for unique monster IDs
var nextMonsterID int = 1

// generateUniqueMonsterID creates a unique ID for a monster
func generateUniqueMonsterID() string {
	id := "monster_" + strconv.Itoa(nextMonsterID)
	nextMonsterID++
	return id
}

type Monster3D struct {
	X, Y         float64
	Name         string
	Key          string // YAML monster key (e.g. "bandit"); used to match encounter monster requirements
	MonsterType  string // creature category from YAML, e.g. "undead" (empty = generic)
	Level        int
	HitPoints    int
	MaxHitPoints int
	ArmorClass   int
	PerfectDodge int // Chance (0-100) to completely avoid an attack
	Experience   int
	ID           string // Unique identifier for collision tracking

	// Combat stats
	DamageMin int
	DamageMax int
	// TrueDamage is added to every attack and bypasses EVERYTHING on the target -
	// armor, resists, Stone Skin/flat, dodge - landing straight on HP (folded into
	// the hit's total, no separate message). Applies to melee AND ranged.
	TrueDamage int
	// Light emission (torch-like)
	LightRadius    float64
	LightIntensity float64

	// AI behavior
	State        MonsterState
	AlertRadius  float64
	AttackRadius float64
	Speed        float64
	Direction    float64
	StateTimer   int
	AttackCount  int // Number of attacks made in current engagement

	// Pathfinding state - prevents oscillation when stuck between obstacles
	LastChosenDir float64 // Last direction chosen by pathfinding
	StuckCounter  int     // Counts consecutive frames where monster couldn't move
	LastX, LastY  float64 // Position last frame to detect stuck state
	LastMoveTick  int64   // Last game frame when the monster moved (for animations)

	// Pursuit pathfinding state (tile-based A*)
	PathTiles        []TileCoord
	PathIndex        int
	PathTargetTileX  int
	PathTargetTileY  int
	LastPathCalcTick int
	pathScratch      pathScratch
	// Pursuit stall detection: a cached path is only recomputed when the target
	// tile changes, so a path that became unwalkable (an engaged packmate now
	// blocks the corridor) is followed forever. Track net progress and drop the
	// path when pursuing goes nowhere, forcing A* against current positions.
	stallAnchorX float64
	stallAnchorY float64
	stallTimer   int

	// RT movement target selection for non-pursuit movement (patrol/flee)
	MoveTargetTileX int
	MoveTargetTileY int
	MoveTargetState MonsterState
	HasMoveTarget   bool

	// Tethering system - monsters stay within 3 tiles of spawn unless engaging player
	SpawnX, SpawnY      float64 // Original spawn position
	TetherRadius        float64 // Maximum distance from spawn point (default 4 tiles = 256 pixels)
	IsEngagingPlayer    bool    // True when actively pursuing/fighting player
	WasAttacked         bool    // True when monster was hit - prevents disengagement
	HitTintFrames       int     // Frames remaining for red hit tint
	AttackAnimFrames    int     // Frames remaining for attack animation (TB mode)
	StandeeYaw          float64 // Render-only: displayed token yaw (eases toward heading)
	StandeeYawTick      int64   // Render-only: frame the token yaw was last advanced
	StandeeMirror       bool    // Render-only: art flip so the walk faces the heading (held while heading is camera-aligned)
	FaceAccX            float64 // Render-only: accumulated per-tick WALK displacement since the last facing commit (separation shoves / band snaps excluded)
	FaceAccY            float64
	StunTurnsRemaining  int  // Turn-based stun duration (monster skips turns)
	StunFramesRemaining int  // Real-time stun duration in frames
	StunDRStacks        int  // Stun diminishing-returns chain length (0=fresh; caps -> immune)
	StunDRMemoryTurns   int  // TB: stun-free turns left before the DR chain resets
	StunDRMemoryFrames  int  // RT: stun-free frames left before the DR chain resets
	RootTurnsRemaining  int  // TB root (bear trap): can't move, CAN attack
	RootFramesRemaining int  // RT root in frames: position pinned, attacks work
	rootHeldThisTurn    bool // TB: rooted at the start of the current turn (runtime-only)
	Pilfered            bool // Sleight of Hand already succeeded on this monster
	// PoisonedFramesRemaining is a party Venom-proc card DoT (rat/spider/masked
	// serpent dancer cards) - separate from monster-inflicted PoisonChance on
	// characters. Ticks 1% of MaxHitPoints (min 1) per second of real time (RT)
	// or once per monster turn (TB).
	PoisonedFramesRemaining int
	poisonTickTimer         int
	// Bind Undead and Charm are SEPARATE, mutually exclusive control states:
	Bound                    bool       // Bind Undead: under party control - hunts other monsters, ignores party
	BoundFramesRemaining     int        // Real-time bind duration in frames (0 in TB = lasts the encounter)
	CrossfireCD              int        // RT cadence between this monster's monster-vs-monster attacks (bound<->enemy crossfire)
	Pacified                 bool       // Charm: simply stops attacking (no fighting others); breaks on any hit taken
	PacifiedFramesRemaining  int        // Real-time charm duration in frames (0 in TB = lasts the encounter)
	AITargetX                float64    // Per-frame pursuit target X (precomputed single-threaded; see refreshBoundUndeadCache)
	AITargetY                float64    // Per-frame pursuit target Y
	AIFoe                    *Monster3D // Per-frame foe to attack (nil = fight the party); precomputed with AITarget
	Flying                   bool       // Whether the monster should be rendered above ground
	RangedAttackRange        float64    // Optional ranged attack range override (pixels)
	AttacksPerRound          int        // Turn-based melee attacks per monster round
	PassiveUntilAttacked     bool       // True when the monster should not aggro until hit
	HatesTraits              []string   // party traits (from hates.yaml) that enrage this passive monster on sight
	AttackCooldownMultiplier float64    // Real-time attack cooldown multiplier (0.5 = twice as often)
	AttackCDFrames           int        // RT frames remaining until this monster may attack again; ticks down regardless of AI state, so kiting in/out of range can't reset the attack cadence
	FireburstChance          float64    // Chance to cast fireburst instead of normal attack
	FireburstDamageMin       int        // Fireburst damage min
	FireburstDamageMax       int        // Fireburst damage max
	DragonBreathChance       float64    // Chance for this attack to hit every living party member
	DragonBreathDamageType   string     // Element used by dragon breath mitigation/resists
	PiercingShotChance       float64    // Chance to fire an armor-piercing shot at multiple party members
	PiercingShotTargets      int        // Number of party members hit by Piercing Shot (default 2)
	AllyHealChance           float64    // Chance to heal self or a nearby allied monster instead of attacking
	AllyHealAmount           int        // HP restored by the ally heal special
	AllyHealRadiusPixels     float64    // Radius for ally heal target search
	PoisonChance             float64    // Chance to apply poison on hit
	PoisonDurationSec        int        // Poison duration in seconds
	IgniteChance             float64    // Chance to set the target on fire (burn DoT 3x poison; stacks with poison)
	IgniteDurationSec        int        // Burn duration in seconds
	StunCharChance           float64    // Chance to stun the struck character (skips actions)
	StunCharSeconds          int        // Stun duration in RT seconds
	StunCharTurns            int        // Stun duration in TB turns
	DispelChance             float64    // Chance to strip one random active party buff on hit

	// Loot
	Gold  int
	Items []items.Item

	// QuestProgressIgnored marks ad-hoc/runtime summons that should not advance or
	// block map-clear kill quests. Fixed map spawns leave this false.
	QuestProgressIgnored bool

	// Resistances and immunities
	Resistances map[DamageType]int

	// Habitat preferences - tiles this monster can walk on even if normally blocked
	HabitatPrefs []string

	// Ranged attack configuration
	ProjectileSpell  string
	ProjectileWeapon string

	// Pounce/leap: PounceRangePixels > 0 enables it. Runtime cooldowns are
	// tracked separately per mode (frames in real-time, turns in turn-based).
	PounceRangePixels     float64
	PounceCooldownSeconds float64
	PounceCDFrames        int // real-time cooldown countdown (frames)
	PounceCDTurns         int // turn-based cooldown countdown (turns)

	// Boss behaviour (data-driven; see the Golden Thief Bug). All zero/"" = off.
	IgnoresArmor      bool    // melee bypasses the party's armor class
	InfernoChance     float64 // 0..1 chance per action to cast a party-nova Inferno
	InfernoDamage     int     // fire damage of that nova, pre-mitigation
	TeleportAtHP      int     // when HP <= this, may blink to a random walkable tile
	TeleportChance    float64 // 0..1 chance per action to blink (only at/below TeleportAtHP)
	PassiveUntilQuest string  // while this quest is incomplete: only evades (blinks away when the party is near), never attacks
	EvadeRadiusTiles  float64 // evasive phase: blink when the party is within this many tiles
	BossCooldownSecs  float64 // RT cadence between evasive blinks (seconds)
	BossCD            int     // RT cadence (frames) between boss special actions (evasive blink)
	BossAggro         bool    // transient (per-frame): an aggressive boss that should relentlessly chase the party (set by refreshBoundUndeadCache)
	BossDormant       bool    // transient (per-frame): a sealed boss (passive-until-quest, no evade radius) that holds its spawn - no detection or wandering until its quest unseals it (set by refreshBoundUndeadCache)
	// Idol-ward (deep-jungle warlord): while any of its plaza idols live the boss is
	// invulnerable and HOLDS its plaza (frozen like a dormant boss); break every idol
	// and it activates as a normal aggressive boss. Idols are immobile, never attack.
	WardedByIdols    bool   // static: this boss is warded while any WarlordIdol lives
	WarlordIdol      bool   // static: this monster is a ward idol (immobile, vulnerable, counts toward the ward)
	AggroWholeMap    bool   // static: UNIQUE boss trait - once active, relentlessly chases from anywhere (ignores detection range). Without it a boss only goes relentless AFTER normal aggro (in alert radius / hit). Golden Thief Bug only.
	DeathRalliesType string // static: when THIS monster dies, every live monster on the map of this Type goes Relentless (revenge). "" = none. (Orc Warlord -> "human".)
	Banding          bool   // static: flocks with same-type banding mobs while calm (stack on a tile + patrol together), scatters on aggro/hit. See [[project_monster_banding]].
	BandID           int    // transient: stable runtime band membership; 0 = solo/unbanded
	BandLeaderID     string // transient: mob ID of this band's stable leader (leader marks itself); "" = none
	BandStackIndex   int    // render-only (per-tick): position in the banded stack (0 = leader/centre); set by updateMonsterBands
	BandStackCount   int    // render-only (per-tick): size of the banded stack (0/1 = not stacked)
	Relentless       bool   // persisted: relentlessly hunt the party from anywhere, like BossAggro but for non-bosses (set by a patron's DeathRalliesType). Survives reload.
	BossWarded       bool   // transient (per-frame): a WardedByIdols boss with >=1 live idol (set by refreshBoundUndeadCache)
	BossLastHP       int    // HP observed at the boss's previous action tick (to detect damage-since-last-tick); 0 = uninitialised
	BossHurtPending  bool   // an evasive boss took damage since its last tick and owes a blink; held until a blink consumes it (survives across turns, unlike the hit flash)
	// Summon (war-banner): on its action an aggressive boss may rally adds.
	SummonChance          float64  // 0..1 chance per action to summon
	SummonFirstGuaranteed bool     // first successful summon ignores SummonChance; refills use SummonChance
	SummonFirstDone       bool     // save-backed latch once the guaranteed summon has happened
	SummonMonsters        []string // monster keys to pick from when summoning
	SummonCount           int      // adds spawned per summon (default 1)
	SummonMax             int      // cap on simultaneously-live summons (0 = uncapped)
	SummonedBy            string   // ID of the boss that summoned this monster ("" = not a summon)
	PackKey               string   // ambient day/night pack tag ("" = not a pack spawn); despawned on phase flips
	// Enrage: at/below EnrageAtHP the boss hits harder and/or faster. The effect is
	// derived LIVE from current HP in GetAttackDamage/AttackCooldownFrames, so it is
	// save-safe (no mutated stats stored); Enraged is only the one-shot announce latch.
	EnrageAtHP         int
	EnrageDamageMult   float64
	EnrageCooldownMult float64
	Enraged            bool
	// Visual tint: a persistent RGB cast multiplied into the lit sprite, marking a
	// variant apart when it shares a base mob's sprite (e.g. an elite). All-zero = none.
	TintR, TintG, TintB float32

	// Encounter system
	IsEncounterMonster bool              // True if this monster is part of an encounter
	EncounterRewards   *EncounterRewards // Rewards for defeating this encounter monster

	// Configuration reference
	config *config.Config

	// Config-derived size, cached at SetupMonsterFromConfig (never changes).
	// The separation pass calls GetSize per overlapping pair every tick - a
	// by-name scan of monsters.yaml there cost ~10^5 string compares/tick.
	cachedSizeW    float64
	cachedSizeH    float64
	cachedSizeMult float64
}

// NewMonster3DFromConfig creates a monster from YAML configuration
func NewMonster3DFromConfig(x, y float64, monsterKey string, cfg *config.Config) *Monster3D {
	if MonsterConfig == nil {
		panic("Monster configuration not loaded. Call monster.MustLoadMonsterConfig() first.")
	}
	ts := defaultTileSize
	if cfg != nil && cfg.GetTileSize() > 0 {
		ts = cfg.GetTileSize()
	}

	def, err := MonsterConfig.GetMonsterByKey(monsterKey)
	if err != nil {
		panic("Monster not found in config: " + monsterKey + ". Check assets/monsters.yaml")
	}

	monster := &Monster3D{
		X:           x,
		Y:           y,
		Key:         monsterKey,
		State:       StatePatrolling, // Start patrolling immediately to avoid "vibing" at spawn
		Speed:       1.0,
		Direction:   rand.Float64() * 2 * math.Pi,
		StateTimer:  0,
		Resistances: make(map[DamageType]int),
		config:      cfg,
		ID:          generateUniqueMonsterID(), // Assign unique ID

		// Initialize tethering system
		SpawnX:           x,
		SpawnY:           y,
		TetherRadius:     4 * ts, // 4 tiles
		IsEngagingPlayer: false,
	}

	// Setup from YAML config
	monster.SetupMonsterFromConfig(def)
	monster.HitPoints = monster.MaxHitPoints

	return monster
}

func (m *Monster3D) TakeDamage(damage int, damageType DamageType, playerX, playerY float64) int {
	return m.TakeDamageResist(damage, damageType, 0, playerX, playerY)
}

// TakeDamageResist is TakeDamage with resistance piercing: resistPiercePct (0..100)
// of the target's resistance to damageType is ignored before reduction. Used by
// Grandmaster spell mastery; TakeDamage passes 0 for the normal path.
func (m *Monster3D) TakeDamageResist(damage int, damageType DamageType, resistPiercePct int, playerX, playerY float64) int {
	// An invulnerable boss absorbs all damage from every source: a sealed (dormant)
	// boss until its quest unseals it, or an idol-warded boss until its idols fall.
	// Both flags are set per-frame in the game's pre-pass; this is the backstop for
	// damage paths that don't pre-check (AoE splash, mastery true-damage, mob-vs-mob).
	if m.BossDormant || m.BossWarded {
		return 0
	}
	// Apply resistance (reduced by any piercing)
	if resistance, exists := m.Resistances[damageType]; exists {
		if resistPiercePct > 0 && resistance > 0 {
			resistance = resistance * (100 - resistPiercePct) / 100
		}
		damage = damage * (100 - resistance) / 100
		if damage < 0 {
			damage = 0
		}
	}

	// Apply damage
	m.HitPoints -= damage
	if m.HitPoints < 0 {
		m.HitPoints = 0
	}

	// Mark as attacked - prevents AI from disengaging due to distance
	m.WasAttacked = true

	// If already engaging player, just return damage (don't change AI state)
	if m.IsEngagingPlayer {
		return damage
	}

	// Being attacked always triggers engagement - chase the attacker
	m.IsEngagingPlayer = true
	m.State = StateAlert
	m.StateTimer = 0

	return damage
}

// ApplyPoison applies or refreshes a party-inflicted poison DoT (Venom-proc
// cards). Mirrors character.ApplyPoison - refreshing never shortens an
// existing, longer poison.
func (m *Monster3D) ApplyPoison(frames int) {
	if frames <= 0 {
		return
	}
	status.Refresh(&m.PoisonedFramesRemaining, frames)
}

// poisonTickDamage deals one poison tick: 1% of max HP, minimum 1.
func (m *Monster3D) poisonTickDamage() {
	if m.HitPoints <= 0 {
		return
	}
	dmg := m.MaxHitPoints / 100
	if dmg < 1 {
		dmg = 1
	}
	m.HitPoints -= dmg
	if m.HitPoints < 0 {
		m.HitPoints = 0
	}
}

// TickPoison advances the poison timer by one REAL-TIME frame (RT mode),
// dealing a tick once per second of real time.
func (m *Monster3D) TickPoison() {
	tps := 60
	if m.config != nil {
		tps = m.config.GetTPS()
	} else {
		tps = config.GetTargetTPS()
	}
	if deal, _ := status.TickDoTFrame(&m.PoisonedFramesRemaining, &m.poisonTickTimer, tps); deal {
		m.poisonTickDamage()
	}
}

// TickPoisonTurn advances the poison timer by one TURN (TB mode) - one damage
// tick per turn, duration measured in the same frame units ApplyPoison used.
func (m *Monster3D) TickPoisonTurn(framesPerTurn int) {
	if deal, _ := status.TickDoTTurn(&m.PoisonedFramesRemaining, &m.poisonTickTimer, framesPerTurn); deal {
		m.poisonTickDamage()
	}
}

func (m *Monster3D) IsAlive() bool {
	return m.HitPoints > 0
}

func (m *Monster3D) GetAttackDamage() int {
	dmg := m.DamageMin
	if m.DamageMax > m.DamageMin {
		dmg += rand.Intn(m.DamageMax - m.DamageMin + 1)
	}
	if m.IsEnraged() && m.EnrageDamageMult > 0 {
		dmg = int(float64(dmg) * m.EnrageDamageMult)
	}
	return dmg
}

// IsEnraged reports whether the boss is at/below its enrage HP threshold. Derived
// live from HP (not a stored flag) so enrage survives save/load without mutating
// stats; the EnrageDamageMult/EnrageCooldownMult are applied wherever this is true.
func (m *Monster3D) IsEnraged() bool {
	return m.EnrageAtHP > 0 && m.HitPoints > 0 && m.HitPoints <= m.EnrageAtHP
}

func (m *Monster3D) GetTurnBasedAttackCount() int {
	n := m.AttacksPerRound
	if n < 1 {
		n = tbAttacksForCooldownMult(m.AttackCooldownMultiplier)
	}
	// RT/TB parity: cooldown speedups that make the monster strike faster in real
	// time grant proportionally more turn-based swings. If AttacksPerRound is set,
	// it is the explicit base; otherwise derive the base from the static cooldown
	// multiplier. Enrage is a dynamic multiplier on top.
	if m.IsEnraged() && m.EnrageCooldownMult > 0 {
		n *= tbAttacksForCooldownMult(m.EnrageCooldownMult)
	}
	return n
}

func (m *Monster3D) HasRangedAttack() bool {
	return m.ProjectileSpell != "" || m.ProjectileWeapon != ""
}

// CanPounce reports whether the monster has the leap ability configured.
// TickRootTurn consumes one TB turn of a root effect; the monster stays
// pinned for the WHOLE turn it started rooted (RootHeld), so an adjacent
// rooted monster that only attacks still burns its root down each turn.
func (m *Monster3D) TickRootTurn() {
	m.rootHeldThisTurn = m.RootTurnsRemaining > 0
	if m.RootTurnsRemaining > 0 {
		m.RootTurnsRemaining--
	}
}

// RootHeld reports whether this turn's movement is pinned by a root.
func (m *Monster3D) RootHeld() bool { return m.rootHeldThisTurn }

func (m *Monster3D) CanPounce() bool {
	// A rooted monster is pinned to its tile - the leap is a movement.
	// rootHeldThisTurn covers the LAST rooted TB turn: TickRootTurn has
	// already decremented the counter to 0 but the pin lasts the whole turn.
	return m.PounceRangePixels > 0 && m.RootFramesRemaining <= 0 &&
		m.RootTurnsRemaining <= 0 && !m.rootHeldThisTurn
}

// GetAttackRangePixels returns the effective attack range in pixels.
// For ranged monsters, this uses the projectile range from config when available.
func (m *Monster3D) GetAttackRangePixels() float64 {
	attackRange := m.AttackRadius
	if !m.HasRangedAttack() || m.config == nil {
		return attackRange
	}

	if m.RangedAttackRange > 0 {
		return m.RangedAttackRange
	}

	tileSize := m.tileSize()

	if m.ProjectileSpell != "" {
		if physics, err := m.config.GetSpellConfig(m.ProjectileSpell); err == nil && physics != nil {
			if physics.RangeTiles > 0 {
				rangePixels := physics.RangeTiles * tileSize
				if rangePixels > attackRange {
					attackRange = rangePixels
				}
			}
		}
	}

	if m.ProjectileWeapon != "" {
		if def, exists := config.GetWeaponDefinition(m.ProjectileWeapon); exists && def.Physics != nil {
			if def.Physics.RangeTiles > 0 {
				rangePixels := def.Physics.RangeTiles * tileSize
				if rangePixels > attackRange {
					attackRange = rangePixels
				}
			}
		}
	}

	return attackRange
}

func (m *Monster3D) GetSpriteType() string {
	// Resolve by the monster's own KEY (always set by NewMonster3DFromConfig),
	// never by name: several monsters can share a display Name (the 4 elemental
	// dragons are all "Dragon"), and a name scan returns a random match per call
	// (Go map order) - which made dragons flicker through every color each frame.
	if MonsterConfig != nil {
		if def, err := MonsterConfig.GetMonsterByKey(m.Key); err == nil {
			return def.GetSpriteFromConfig()
		}
	}

	// Fallback if config not loaded or key unknown.
	return "goblin"
}

func (m *Monster3D) GetSize() (width, height float64) {
	// Cached at SetupMonsterFromConfig; the scan below only serves hand-built
	// test monsters that never went through config setup.
	if m.cachedSizeW > 0 && m.cachedSizeH > 0 {
		return m.cachedSizeW, m.cachedSizeH
	}
	if MonsterConfig != nil {
		for _, def := range MonsterConfig.Monsters {
			if def.Name == m.Name {
				return def.GetSizeFromConfig()
			}
		}
	}

	// Fallback size if config not loaded or monster not found
	return 32.0, 32.0
}

// GetSizeGameMultiplier returns the visual size multiplier from config
func (m *Monster3D) GetSizeGameMultiplier() float64 {
	if m.cachedSizeMult > 0 {
		return m.cachedSizeMult
	}
	if MonsterConfig != nil {
		for _, def := range MonsterConfig.Monsters {
			if def.Name == m.Name {
				return def.GetSizeGameMultiplier()
			}
		}
	}

	// Fallback multiplier if config not loaded or monster not found
	return 1.0
}

// distance calculates the Euclidean distance between two 2D points.
func distance(x1, y1, x2, y2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	return math.Sqrt(dx*dx + dy*dy)
}

// GetDistanceFromSpawn calculates the distance from the monster's spawn point
func (m *Monster3D) GetDistanceFromSpawn() float64 {
	return distance(m.X, m.Y, m.SpawnX, m.SpawnY)
}

// IsWithinTetherRadius checks if the monster is within its tether radius from spawn
func (m *Monster3D) IsWithinTetherRadius() bool {
	return m.GetDistanceFromSpawn() <= m.TetherRadius
}

// GetDirectionToSpawn returns the direction (in radians) from current position back to spawn
func (m *Monster3D) GetDirectionToSpawn() float64 {
	dx := m.SpawnX - m.X
	dy := m.SpawnY - m.Y
	return math.Atan2(dy, dx)
}

// CanMoveWithinTether checks if moving in a direction would keep monster within tether
func (m *Monster3D) CanMoveWithinTether(newX, newY float64) bool {
	return distance(newX, newY, m.SpawnX, m.SpawnY) <= m.TetherRadius
}
