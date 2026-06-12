package monster

import (
	"math"
	"math/rand"
	"strconv"
	"ugataima/internal/config"
	"ugataima/internal/items"
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
	CompletionMessage string   `yaml:"completion_message,omitempty"`
}

// PartyTraits holds the party's currently-active "traits" (e.g. "lich"),
// refreshed once per frame before monster updates. A passive monster turns
// hostile on sight only if one of ITS hated traits (HatesTraits, from hates.yaml)
// is present here — so a Lich enrages just archmages and elf warriors, not every
// passive monster. Mutated in place to avoid per-frame allocation.
var PartyTraits = map[string]bool{}

// HatesActiveTrait reports whether any of this monster's hated party traits is
// currently active — i.e. whether a passive monster should turn hostile on sight.
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
	StunTurnsRemaining  int     // Turn-based stun duration (monster skips turns)
	StunFramesRemaining int     // Real-time stun duration in frames
	RootTurnsRemaining  int     // TB root (bear trap): can't move, CAN attack
	RootFramesRemaining int     // RT root in frames: position pinned, attacks work
	rootHeldThisTurn    bool    // TB: rooted at the start of the current turn (runtime-only)
	Pilfered            bool    // Sleight of Hand already succeeded on this monster
	// Bind Undead and Charm are SEPARATE, mutually exclusive control states:
	Bound                    bool       // Bind Undead: under party control — hunts other monsters, ignores party
	BoundFramesRemaining     int        // Real-time bind duration in frames (0 in TB = lasts the encounter)
	CrossfireCD              int        // RT cadence between this monster's monster-vs-monster attacks (bound↔enemy crossfire)
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
	FireburstChance          float64    // Chance to cast fireburst instead of normal attack
	FireburstDamageMin       int        // Fireburst damage min
	FireburstDamageMax       int        // Fireburst damage max
	PoisonChance             float64    // Chance to apply poison on hit
	PoisonDurationSec        int        // Poison duration in seconds

	// Loot
	Gold  int
	Items []items.Item

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
	BossLastHP        int     // HP observed at the boss's previous action tick (to detect damage-since-last-tick); 0 = uninitialised
	BossHurtPending   bool    // an evasive boss took damage since its last tick and owes a blink; held until a blink consumes it (survives across turns, unlike the hit flash)

	// Encounter system
	IsEncounterMonster bool              // True if this monster is part of an encounter
	EncounterRewards   *EncounterRewards // Rewards for defeating this encounter monster

	// Configuration reference
	config *config.Config
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
	// Apply resistance (reduced by any piercing)
	if resistance, exists := m.Resistances[damageType]; exists {
		if resistPiercePct > 0 {
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

func (m *Monster3D) IsAlive() bool {
	return m.HitPoints > 0
}

func (m *Monster3D) GetAttackDamage() int {
	if m.DamageMin >= m.DamageMax {
		return m.DamageMin
	}
	return m.DamageMin + rand.Intn(m.DamageMax-m.DamageMin+1)
}

func (m *Monster3D) GetTurnBasedAttackCount() int {
	if m.AttacksPerRound < 1 {
		return 1
	}
	return m.AttacksPerRound
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
	// A rooted monster is pinned to its tile — the leap is a movement.
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
	// (Go map order) — which made dragons flicker through every color each frame.
	if MonsterConfig != nil {
		if def, err := MonsterConfig.GetMonsterByKey(m.Key); err == nil {
			return def.GetSpriteFromConfig()
		}
	}

	// Fallback if config not loaded or key unknown.
	return "goblin"
}

func (m *Monster3D) GetSize() (width, height float64) {
	// Get size from config
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
	// Get size multiplier from config
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
