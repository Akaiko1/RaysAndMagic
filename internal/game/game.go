package game

import (
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/items"
	"ugataima/internal/mathutil"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/spells"
	"ugataima/internal/threading"
	"ugataima/internal/threading/entities"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// Constants are now loaded from config.yaml
// Access via config.GlobalConfig or pass config instance

type ProjectileOwner int

type combatLogEntry struct {
	Text  string
	Color color.Color
}

const maxCombatLogHistory = 500

const (
	ProjectileOwnerPlayer ProjectileOwner = iota
	ProjectileOwnerMonster
	// ProjectileOwnerBoundUndead is a bound undead's projectile: it damages enemy
	// (non-controlled) monsters, but never the party or other controlled allies.
	ProjectileOwnerBoundUndead
	// ProjectileOwnerMonsterAtBound is an enemy mob's projectile aimed at a
	// bound undead: it damages ONLY bound monsters (the undead), never the party.
	ProjectileOwnerMonsterAtBound
)

// InteractionDistance is the max range (in world units, ~2 tiles) for the
// player to talk to NPCs and trigger interactables.
const InteractionDistance = 128.0

type MagicProjectile struct {
	ID                 string  // Unique identifier
	X, Y               float64 // Current position
	VelX, VelY         float64 // Velocity
	Damage             int
	Attacker           *character.MMCharacter // caster (nil = monster/none) — mastery/pierce resolve from HIM at impact; a pointer survives roster swaps mid-flight
	LifeTime           int                    // Frames remaining
	Active             bool
	SpellType          string // Type of spell for visual differentiation
	Size               int    // Projectile size
	Crit               bool   // Critical hit flag
	DisintegrateChance float64
	Owner              ProjectileOwner
	SourceName         string
	AoE                bool // monster projectile: on hit, splash damage to the whole party
}

type MeleeAttack struct {
	ID         string  // Unique identifier
	X, Y       float64 // Current position
	VelX, VelY float64 // Velocity
	Damage     int
	LifeTime   int // Frames remaining
	Active     bool
	WeaponName string // Name of weapon used for combat messages
	Crit       bool   // Critical hit flag
}

// SlashEffect represents a visual melee swing (a per-weapon pixel-particle
// flourish; see drawMeleeParticles).
type SlashEffect struct {
	ID             string  // Unique identifier
	X, Y           float64 // Origin (camera position at swing) — for cleanup/debug
	Width, Length  int     // Dimensions from weapon graphics config
	Color          [3]int  // RGB color
	AnimationFrame int     // Current animation frame
	MaxFrames      int     // Total animation frames
	Active         bool
	Kind           string // per-weapon FX flavor: slash/chop/smash/stab/lunge
	Crit           bool   // critical swing: bigger, brighter, golden-edged
}

type Arrow struct {
	ID                 string  // Unique identifier
	X, Y               float64 // Current position
	VelX, VelY         float64 // Velocity
	Damage             int
	Attacker           *character.MMCharacter // shooter (nil = monster/none)
	LifeTime           int                    // Frames remaining
	Active             bool
	BowKey             string // YAML key of the bow used to fire this arrow
	DamageType         string // Damage element type ("physical", "dark", etc.)
	Crit               bool   // Critical hit flag
	DisintegrateChance float64
	Owner              ProjectileOwner
	SourceName         string
	RenderAngle        float64 // Render-only: smoothed on-screen shaft angle
	RenderAngleSet     bool    // Render-only: RenderAngle initialised
}

// SpellHitParticle represents a single particle from a spell impact
type SpellHitParticle struct {
	X, Y             float64 // World anchor (impact point) — fixed; used for projection
	OffsetX, OffsetY float64 // Screen-space offset from the anchor (a real 2D burst)
	VelX, VelY       float64 // Screen-space velocity (px/frame at the anchor's scale)
	Gravity          float64 // Added to VelY each frame (ice shards fall, embers rise)
	Color            [3]int  // RGB color based on element
	LifeTime         int     // Frames remaining
	MaxLife          int     // Initial lifetime for alpha calculation
	Size             int     // Particle size (shrinks over time)
	Trail            bool    // emits a fading breadcrumb trail each few frames (Starburst falling stars)
	Active           bool
}

// SpellHitEffect represents a burst of particles from a spell impact
type SpellHitEffect struct {
	Particles []SpellHitParticle
	Active    bool
}

// ElementColors maps spell elements to RGB colors
var ElementColors = map[string][3]int{
	"fire":     {255, 100, 0},   // Orange-red
	"water":    {0, 150, 255},   // Blue
	"air":      {200, 200, 255}, // Light blue-white
	"earth":    {139, 90, 43},   // Brown
	"light":    {255, 255, 200}, // Warm white
	"dark":     {80, 0, 120},    // Purple
	"arcane":   {150, 190, 255}, // Arcane blue-white (staff/book bolts)
	"body":     {120, 230, 150}, // Healing green
	"mind":     {170, 190, 255}, // Pale blue
	"spirit":   {210, 185, 255}, // Pale violet
	"physical": {200, 200, 200}, // Gray
}

// MapPose captures the player's position and facing on a specific map so
// two-way transitions can drop them back where they came in.
type MapPose struct {
	X, Y, Angle float64
}

type MMGame struct {
	world     *world.World3D
	camera    *FirstPersonCamera
	party     *character.Party
	sprites   *graphics.SpriteManager
	gameState GameState
	config    *config.Config

	// UI state
	showPartyStats     bool
	showFPS            bool
	showCollisionBoxes bool // Toggle for collision box rendering
	perfDebugEnabled   bool
	perfLowFpsSince    time.Time
	perfLastPerfLog    time.Time

	// Unique ID generation
	nextProjectileID int64
	selectedChar     int
	frameCount       int64

	// Tabbed menu system
	menuOpen          bool
	currentTab        MenuTab
	mouseLeftClickX   int
	mouseLeftClickY   int
	mouseRightClickX  int
	mouseRightClickY  int
	mouseLeftClickAt  int64
	mouseRightClickAt int64
	mouseLeftClicks   []queuedClick
	mouseRightClicks  []queuedClick

	// Double-click support for spellbook
	lastSpellClickTime int64 // Time of last spell click in milliseconds
	lastClickedSpell   int   // Index of last clicked spell
	lastClickedSchool  int   // Index of last clicked school
	// Double-click support for spellbook school collapse
	lastSchoolClickTime  int64 // Time of last school click in milliseconds
	lastSchoolClickedIdx int   // Index of last clicked school header

	// Double-click support for dialogs (neutral)
	dialogLastClickTime  int64  // Time of last dialog list click in milliseconds
	dialogLastClickedIdx int    // Index of last clicked dialog list entry
	dialogLastClickZone  string // Which dialog list was clicked (buy/sell/spell/...) — a double-click never spans lists

	// Double-click support for utility spell icons (dispelling)
	lastUtilitySpellClickTime int64  // Time of last utility spell icon click in milliseconds
	lastClickedUtilitySpell   string // Icon of last clicked utility spell

	// Cached images for performance
	skyImg            *ebiten.Image
	groundImg         *ebiten.Image
	skyPanorama       *ebiten.Image
	currentSkyTexture string
	skyShader         *ebiten.Shader // lazily compiled, reused across frames

	// Combat effects
	magicProjectiles []MagicProjectile
	meleeAttacks     []MeleeAttack
	arrows           []Arrow
	groundContainers []GroundContainer // unified loot bags + treasure chests on the ground

	// Spellbook UI state
	collapsedSpellSchools map[character.MagicSchoolID]bool

	// Utility spell status icons (data-driven)
	utilitySpellStatuses map[spells.SpellID]*UtilitySpellStatus
	slashEffects         []SlashEffect
	spellHitEffects      []SpellHitEffect
	impactLights         []ImpactLight // short-lived light flashes at spell impacts (guarded by hitEffectsMu)
	screenShake          float64       // camera shake amplitude in world units, decays each tick
	hitEffectsMu         sync.Mutex

	// Map overlay UI state
	mapOverlayOpen bool

	// Lighting effects
	torchLightActive   bool    // Whether torch light is currently active
	torchLightDuration int     // Remaining duration in frames
	torchLightRadius   float64 // Radius of the light effect

	// Wizard Eye effect
	wizardEyeActive      bool    // Whether wizard eye is currently active
	wizardEyeRadiusTiles float64 // radar reach from spells.yaml (vision_radius_tiles)
	wizardEyeDuration    int     // Remaining duration in frames

	// Walk on Water effect
	walkOnWaterActive   bool // Whether walk on water is currently active
	walkOnWaterDuration int  // Remaining duration in frames

	// Bless effect
	// statBuffs is the registry of active stat-buff spells (Bless, …): different
	// spells stack, recasting one refreshes it. g.statBonuses is DERIVED as
	// their sum via recomputeStatBonuses — never mutate it directly.
	statBuffs []TimedStatBuff

	// Stacking timed party combat buffs (Day of the Gods, Hour of Power, Stone
	// Skin, Heroism, …) — see combat_buffs.go. Their ResistPct/OutBonus/InReduce
	// sum across all active entries.
	combatBuffs []TimedCombatBuff

	// Persistent damage zones (Hot Steam) — see combat_zones.go.
	steamZones   []SteamZone
	traps        []PlacedTrap // armed thief traps (map-scoped, persisted)
	selectedTrap int          // trap-book browse index (selection ≠ equipped quick trap)

	// boundUndead caches the bound undead (bind_undead) present this frame so the
	// per-monster AI-target lookup can let normal mobs turn on them without an
	// O(n²) scan in the common (no-bind) case. Rebuilt each frame before the
	// monster update; see refreshBoundUndeadCache.
	boundUndead []*monster.Monster3D

	// Water Breathing effect
	waterBreathingActive   bool    // Whether water breathing is currently active
	waterBreathingDuration int     // Remaining duration in frames
	underwaterReturnX      float64 // X position to return to when spell expires
	underwaterReturnY      float64 // Y position to return to when spell expires
	underwaterReturnMap    string  // Map to return to when spell expires

	// Per-map last-known player pose so two-way map transitions (e.g. enter /
	// leave a church) drop the player back where they left, not at the map's
	// spawn point.
	mapReturnPoses map[string]MapPose

	// Generic stat bonus system (for Bless, Day of Gods, Hour of Power, etc.)
	// statBonuses aggregates every active stat-buff spell per stat (today only
	// Bless contributes, uniformly). Any change MUST go through
	// applyPartyStatBonuses so members' BuffBonuses and MaxHP/MaxSP follow.
	// NOTE: saves persist the legacy uniform int (bless-only); a future
	// per-stat buff spell needs a per-stat save field.
	statBonuses character.StatBonuses

	// Dialog system
	dialogActive        bool           // Whether a dialog is currently open
	dialogNPC           *character.NPC // Current NPC being talked to
	dialogSelectedChar  int            // Currently selected character in dialog
	dialogSelectedSpell int            // Currently selected spell in dialog
	selectedCharIdx     int            // Selected character index for spell learning
	skillTrainerPopup   bool           // Skill trainer: per-character mastery popup open
	selectedSpellKey    string         // Selected spell key for learning
	selectedChoice      int            // Selected choice in encounter dialogs
	dialogTab           int            // Spell-trader tab: 0 = spells, 1 = quests (quest-giving traders)

	// Spellbook UI
	selectedSchool     int
	selectedSpell      int
	spellInputCooldown int

	// Combat log: one ordered list of (text, color) entries. The HUD shows the
	// last maxMessages of them; the scrollable overlay shows up to
	// maxCombatLogHistory. (Replaces the old combatMessages/combatMessageColors
	// parallel arrays that had to be kept in lockstep.)
	combatLogHistory   []combatLogEntry
	combatLogOpen      bool
	combatLogScroll    int
	lastCombatLogClick int64
	maxMessages        int

	// Per-member, per-effect card-overlay timers (frames remaining): blink/scorch/
	// spark/heal. One table instead of four parallel arrays — see cardFx and
	// triggerCardFx. Indexed [effect][memberIndex].
	cardFxTimers [cardFxCount][4]int

	// Multi-threading components
	threading *threading.ThreadingComponents

	// Thread safety
	projectileMutex sync.RWMutex

	// Rendering helper
	renderHelper *RenderingHelper

	// Depth buffer for proper 3D rendering (distance per screen column)
	depthBuffer []float64

	// Systems
	gameLoop        *GameLoop
	combat          *CombatSystem
	collisionSystem *collision.CollisionSystem
	questManager    *quests.QuestManager

	// Reusable slices to reduce GC pressure (allocated once, reused each frame)
	reusableMonsterWrappers     []entities.MonsterUpdateInterface
	reusableProjectileWrappers  []entities.ProjectileUpdateInterface
	reusableEncounterRewardsMap map[*monster.EncounterRewards]int
	reusableDeadSet             map[string]bool

	// Dead monster IDs to remove - populated when monster dies, processed once per frame
	deadMonsterIDs []string
	// Stat distribution popup UI state
	statPopupOpen    bool // Is the stat distribution popup open?
	statPopupCharIdx int  // Which character is being edited in the popup?
	// Level-up choice queue
	levelUpChoiceQueue []levelUpChoiceRequest
	levelUpChoiceOpen  bool
	levelUpChoiceIdx   int

	// Revival potion target picker. Dead/unconscious party members can't be
	// selected via the portrait click, so when the player uses a revive
	// consumable we hand them a small modal listing eligible targets.
	// revivalPickerOpen means the picker is visible; revivalPickerItemIdx
	// is the inventory slot of the consumable to spend on confirm.
	revivalPickerOpen    bool
	revivalPickerItemIdx int

	// Promotion picker: when more than one party member is eligible for a
	// promotion (Archmage/Lich), this modal lists them. promotionPickerKind is
	// the target promotion; promotionPickerItemIdx is the phylactery slot to
	// consume on confirm (-1 for the quest-driven Archmage path).
	promotionPickerOpen    bool
	promotionPickerKind    character.Promotion
	promotionPickerItemIdx int

	// Tavern roster screen: swap active party members with the reserve roster.
	// rosterSelectedActive is the active slot the player picked first (-1 = none).
	rosterScreenOpen     bool
	rosterSelectedActive int

	// Turn-based mode state
	turnBasedMode         bool // Whether game is in turn-based mode
	currentTurn           int  // 0 = party turn, 1 = monster turn
	partyActionsUsed      int  // Actions used this turn (0-2)
	turnBasedMoveCooldown int  // Movement cooldown in frames (18 FPS = 0.3 second)
	turnBasedRotCooldown  int  // Rotation cooldown in frames (18 FPS = 0.3 second)
	monsterTurnResolved   bool // Whether monster turn already processed this round
	turnBasedSpRegenCount int  // Counter for turn-based SP regeneration (every 5 turns)

	// Main menu (ESC)
	mainMenuOpen      bool
	mainMenuSelection int
	mainMenuMode      MainMenuMode
	slotSelection     int
	saveRenameOpen    bool
	saveRenameSlot    int
	saveRenameInput   string
	exitRequested     bool

	// Game over state
	gameOver bool

	// Victory state
	gameVictory      bool
	victoryTime      time.Time
	sessionStartTime time.Time

	// High scores state
	showHighScores    bool
	victoryNameInput  string
	victoryScoreSaved bool
}

type GameState int

const (
	GameStateExploration GameState = iota
	GameStateTurnBased
)

// MainMenuMode represents sub-modes of the ESC menu
type MainMenuMode int

const (
	MenuMain MainMenuMode = iota
	MenuSaveSelect
	MenuLoadSelect
)

// MenuTab represents the different tabs in the main menu
type MenuTab int

const (
	TabInventory MenuTab = iota
	TabCharacters
	TabSpellbook
	TabQuests
)

type FirstPersonCamera struct {
	X, Y     float64 // Position in world
	Angle    float64 // Viewing angle in radians
	FOV      float64 // Field of view
	ViewDist float64 // Maximum view distance
}

func NewMMGame(cfg *config.Config) *MMGame {
	sprites := graphics.NewSpriteManager()
	// Load-time color key: make stray magenta (from imperfect sprite background
	// removal) transparent. Data-driven via config; [0,0,0]/absent key → magenta.
	if ck := cfg.Graphics.ColorKey; ck.Enabled {
		r, g, b := ck.Color[0], ck.Color[1], ck.Color[2]
		if r == 0 && g == 0 && b == 0 {
			r, g, b = 255, 0, 255 // default key = magenta
		}
		sprites.SetColorKey(true, r, g, b, ck.Tolerance, ck.Despill)
	}

	// Get world from WorldManager instead of creating new one
	currentWorld := world.GlobalWorldManager.GetCurrentWorld()
	if currentWorld == nil {
		panic("No world available from WorldManager")
	}

	// Create a 4-character party
	party := character.NewParty(cfg)

	// Start in current map - get starting position from loaded map
	startX, startY := currentWorld.GetStartingPosition()

	camera := &FirstPersonCamera{
		X:        startX,
		Y:        startY,
		Angle:    0,
		FOV:      cfg.GetCameraFOV(),
		ViewDist: cfg.GetViewDistance(),
	}

	// Initialize sky and ground images (will be updated dynamically based on current map)
	skyImg := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight()/2)
	groundImg := ebiten.NewImage(cfg.GetScreenWidth(), cfg.GetScreenHeight()/2)

	// Initialize threading components
	threadingComponents := threading.NewThreadingComponents(cfg)

	game := &MMGame{
		world:            currentWorld,
		camera:           camera,
		party:            party,
		sprites:          sprites,
		gameState:        GameStateExploration,
		config:           cfg,
		showPartyStats:   true,
		showFPS:          false, // FPS counter starts hidden
		perfDebugEnabled: strings.TrimSpace(os.Getenv("DEBUG_PERF")) != "",
		selectedChar:     0,
		skyImg:           skyImg,
		groundImg:        groundImg,
		magicProjectiles: make([]MagicProjectile, 0),
		meleeAttacks:     make([]MeleeAttack, 0),
		arrows:           make([]Arrow, 0),
		slashEffects:     make([]SlashEffect, 0),
		spellHitEffects:  make([]SpellHitEffect, 0),

		// Tabbed menu system
		menuOpen:   false,
		currentTab: TabInventory,

		// Double-click support for spellbook
		lastSpellClickTime:   0,
		lastClickedSpell:     -1,
		lastClickedSchool:    -1,
		lastSchoolClickTime:  0,
		lastSchoolClickedIdx: -1,

		// Double-click support for dialogs (neutral)
		dialogLastClickTime:  0,
		dialogLastClickedIdx: -1,

		selectedSchool:        0,
		selectedSpell:         0,
		collapsedSpellSchools: make(map[character.MagicSchoolID]bool),
		utilitySpellStatuses:  make(map[spells.SpellID]*UtilitySpellStatus),
		combatLogHistory:      make([]combatLogEntry, 0),
		maxMessages:           4, // Show last 4 messages

		// Dialog system initialization
		dialogSelectedChar:  0,
		dialogSelectedSpell: 0,

		// Threading components
		threading: threadingComponents,

		// Initialize depth buffer for proper 3D rendering
		depthBuffer: make([]float64, cfg.GetScreenWidth()),

		// Pre-allocate reusable slices to reduce GC pressure
		reusableMonsterWrappers:     make([]entities.MonsterUpdateInterface, 0, 64),
		reusableProjectileWrappers:  make([]entities.ProjectileUpdateInterface, 0, 64),
		reusableEncounterRewardsMap: make(map[*monster.EncounterRewards]int),
		reusableDeadSet:             make(map[string]bool, 16),
		deadMonsterIDs:              make([]string, 0, 16),

		// Session timer for score calculation
		sessionStartTime: time.Now(),

		saveRenameSlot: -1,
	}

	// Initialize rendering helper
	game.renderHelper = NewRenderingHelper(game)

	// Initialize collision system
	game.collisionSystem = collision.NewCollisionSystem(currentWorld, float64(cfg.World.TileSize))

	// Register player entity in collision system (small collision box for good movement freedom)
	playerEntity := collision.NewEntity("player", startX, startY, 16, 16, collision.CollisionTypePlayer, false)
	game.collisionSystem.RegisterEntity(playerEntity)

	// Register all monsters with collision system
	currentWorld.RegisterMonstersWithCollisionSystem(game.collisionSystem)

	// Initialize systems
	game.combat = NewCombatSystem(game)
	game.gameLoop = NewGameLoop(game)

	// Connect global quest manager
	game.questManager = quests.GlobalQuestManager
	if err := validateQuestTileChanges(game.questManager); err != nil {
		panic(err)
	}

	// Update sky and ground colors for initial map
	game.UpdateSkyAndGroundColors()

	return game
}

// GetCurrentWorld returns the current world from WorldManager
func (g *MMGame) GetCurrentWorld() *world.World3D {
	if world.GlobalWorldManager != nil {
		return world.GlobalWorldManager.GetCurrentWorld()
	}
	// Fallback to the original world reference
	return g.world
}

// GetPlayerTilePosition returns the tile coordinates the player is currently in
func (g *MMGame) GetPlayerTilePosition() (tileX, tileY int) {
	tileSize := float64(g.config.GetTileSize())
	return int(g.camera.X / tileSize), int(g.camera.Y / tileSize)
}

// registerSpawnedMonster appends a freshly-created monster to the world and
// registers its collision entity. Shared by every spawn pathway so we can't
// accidentally forget the collision registration step.
func (g *MMGame) registerSpawnedMonster(m *monster.Monster3D) {
	if m == nil {
		return
	}
	g.world.Monsters = append(g.world.Monsters, m)
	width, height := m.GetSize()
	entity := collision.NewEntity(m.ID, m.X, m.Y, width, height, collision.CollisionTypeMonster, true)
	g.collisionSystem.RegisterEntity(entity)
}

// GetNearestInteractableNPC returns the nearest NPC within interaction range, or nil if none
func (g *MMGame) GetNearestInteractableNPC() *character.NPC {
	currentWorld := g.GetCurrentWorld()
	if currentWorld == nil {
		return nil
	}

	var nearestNPC *character.NPC
	nearestDistance := float64(InteractionDistance)

	for _, npc := range currentWorld.NPCs {
		// Spent statues (hide_when_visited) are gone for all purposes once used.
		if npc.HideWhenVisited && npc.Visited {
			continue
		}
		dist := Distance(g.camera.X, g.camera.Y, npc.X, npc.Y)

		if dist <= nearestDistance {
			nearestNPC = npc
			nearestDistance = dist
		}
	}

	return nearestNPC
}

// FindNearestWalkableTile finds the closest walkable tile to the given position (DRY helper)
func (g *MMGame) FindNearestWalkableTile(targetX, targetY float64) (float64, float64) {
	return g.findNearestWalkableTileWithMaxRadius(targetX, targetY, 10)
}

// FindNearestWalkableTileMustSucceed finds walkable tile with expanding search - MUST find one
func (g *MMGame) FindNearestWalkableTileMustSucceed(targetX, targetY float64) (float64, float64) {
	worldInst := g.GetCurrentWorld()
	if worldInst == nil {
		fmt.Printf("Error: No world available for walkable tile search\n")
		return targetX, targetY // Emergency fallback
	}

	// Start with normal search radius, then expand until we find something
	for maxRadius := 10; maxRadius <= worldInst.Width && maxRadius <= worldInst.Height; maxRadius += 10 {
		x, y := g.findNearestWalkableTileWithMaxRadius(targetX, targetY, maxRadius)
		if x != -1 && y != -1 {
			fmt.Printf("Found walkable tile at radius %d: (%.1f, %.1f)\n", maxRadius, x, y)
			return x, y
		}
	}

	// Last resort: scan entire map for ANY walkable tile
	fmt.Printf("Emergency: Scanning entire map for walkable tile\n")
	tileSize := float64(g.config.GetTileSize())

	for y := 0; y < worldInst.Height; y++ {
		for x := 0; x < worldInst.Width; x++ {
			tile := worldInst.Tiles[y][x]
			if world.GlobalTileManager != nil && world.GlobalTileManager.IsWalkable(tile) {
				safeX, safeY := TileCenterFromTile(x, y, tileSize)
				fmt.Printf("Emergency fallback: Found walkable tile at (%.1f, %.1f)\n", safeX, safeY)
				return safeX, safeY
			}
		}
	}

	// This should NEVER happen in a properly designed map
	fmt.Printf("CRITICAL ERROR: No walkable tiles found in entire map!\n")
	return targetX, targetY // Return original position as absolute last resort
}

// findNearestWalkableTileWithMaxRadius internal helper with configurable search radius
func (g *MMGame) findNearestWalkableTileWithMaxRadius(targetX, targetY float64, maxRadius int) (float64, float64) {
	worldInst := g.GetCurrentWorld()
	if worldInst == nil {
		return -1, -1
	}

	tileSize := float64(g.config.GetTileSize())
	targetTX := int(targetX / tileSize)
	targetTY := int(targetY / tileSize)

	// Search in expanding spiral from target position
	for radius := 0; radius < maxRadius; radius++ {
		for dx := -radius; dx <= radius; dx++ {
			for dy := -radius; dy <= radius; dy++ {
				// Only check perimeter of current radius
				if mathutil.IntAbs(dx) != radius && mathutil.IntAbs(dy) != radius {
					continue
				}

				checkX := targetTX + dx
				checkY := targetTY + dy

				// Check bounds
				if checkX < 0 || checkX >= worldInst.Width || checkY < 0 || checkY >= worldInst.Height {
					continue
				}

				tile := worldInst.Tiles[checkY][checkX]

				// Check if tile is walkable using the global tile manager
				if world.GlobalTileManager != nil && world.GlobalTileManager.IsWalkable(tile) {
					// Convert back to world coordinates
					safeX, safeY := TileCenterFromTile(checkX, checkY, tileSize)
					return safeX, safeY
				}
			}
		}
	}

	// No walkable tile found within radius
	return -1, -1
}

// UpdateSkyAndGroundColors updates the cached sky and ground images based on current map
func (g *MMGame) UpdateSkyAndGroundColors() {
	// Get current map configuration
	var skyColor, groundColor [3]int
	skyTexture := ""

	if world.GlobalWorldManager != nil {
		mapConfig := world.GlobalWorldManager.GetCurrentMapConfig()
		if mapConfig != nil {
			skyColor = mapConfig.SkyColor
			groundColor = mapConfig.DefaultFloorColor
			skyTexture = mapConfig.SkyTexture
		} else {
			// Fallback to config defaults
			skyColor = g.config.Graphics.Colors.Sky
			groundColor = g.config.Graphics.Colors.Ground
		}
	} else {
		// Fallback to config defaults
		skyColor = g.config.Graphics.Colors.Sky
		groundColor = g.config.Graphics.Colors.Ground
	}

	// Update sky image
	g.skyImg.Fill(color.RGBA{uint8(skyColor[0]), uint8(skyColor[1]), uint8(skyColor[2]), 255})
	g.updateSkyPanorama(skyTexture)

	// Update ground image
	g.groundImg.Fill(color.RGBA{uint8(groundColor[0]), uint8(groundColor[1]), uint8(groundColor[2]), 255})
}

func (g *MMGame) updateSkyPanorama(textureName string) {
	if textureName == g.currentSkyTexture {
		return
	}
	g.currentSkyTexture = textureName
	g.skyPanorama = nil
	if textureName == "" {
		return
	}

	img, err := loadPNGAsEbiten(resolveNamedPNG("assets/sprites/sky", textureName))
	if err != nil {
		fmt.Printf("[Sky] failed to load %q: %v\n", textureName, err)
		return
	}
	g.skyPanorama = img
}

func resolveNamedPNG(baseDir, name string) string {
	if filepath.IsAbs(name) {
		return name
	}
	if filepath.Ext(name) == "" {
		name += ".png"
	}
	if filepath.Dir(name) != "." {
		return name
	}
	return filepath.Join(baseDir, name)
}

func decodePNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

func loadPNGAsEbiten(path string) (*ebiten.Image, error) {
	img, err := decodePNG(path)
	if err != nil {
		return nil, err
	}
	return ebiten.NewImageFromImage(img), nil
}

func (g *MMGame) ensureSkyShader() (*ebiten.Shader, error) {
	if g.skyShader != nil {
		return g.skyShader, nil
	}
	s, err := ebiten.NewShader([]byte(skyShaderSrc))
	if err != nil {
		return nil, err
	}
	g.skyShader = s
	return s, nil
}

func (g *MMGame) Update() error {
	if err := g.gameLoop.Update(); err != nil {
		return err
	}
	g.checkGameOver()
	g.checkVictory()
	return nil
}

func (g *MMGame) Draw(screen *ebiten.Image) {
	// Screen shake: nudge the camera sideways (perpendicular to the view) for
	// this frame only — the whole raycast scene shifts coherently, and the
	// camera is restored before any game logic can observe it.
	if g.screenShake > 0 && g.camera != nil {
		ox := -math.Sin(g.camera.Angle) * g.screenShake
		oy := math.Cos(g.camera.Angle) * g.screenShake
		if g.frameCount%2 == 0 {
			ox, oy = -ox, -oy
		}
		g.camera.X += ox
		g.camera.Y += oy
		defer func(x, y float64) { g.camera.X, g.camera.Y = x, y }(g.camera.X-ox, g.camera.Y-oy)
	}
	g.gameLoop.Draw(screen)
}

// GenerateProjectileID generates a unique ID for projectiles
func (g *MMGame) GenerateProjectileID(projectileType string) string {
	g.nextProjectileID++
	return fmt.Sprintf("%s_%d", projectileType, g.nextProjectileID)
}

func (g *MMGame) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return g.gameLoop.Layout(outsideWidth, outsideHeight)
}

// handleResize updates the runtime screen dimensions used by rendering and
// UI anchoring. All 80-odd call sites read config.GetScreenWidth/Height each
// frame, so mutating those values here propagates the new size transparently
// without an audit. Pre-allocated screen-sized buffers (depth buffer,
// sky/ground images, floor cache, ray caches) are reallocated to match;
// otherwise width-indexed pixel writes in the renderer would overrun.
func (g *MMGame) handleResize(screenWidth, screenHeight int) {
	if screenWidth <= 0 || screenHeight <= 0 {
		return
	}
	if screenWidth == g.config.Display.ScreenWidth && screenHeight == g.config.Display.ScreenHeight && len(g.depthBuffer) == screenWidth {
		return
	}
	g.config.Display.ScreenWidth = screenWidth
	g.config.Display.ScreenHeight = screenHeight

	g.depthBuffer = make([]float64, screenWidth)
	g.skyImg = ebiten.NewImage(screenWidth, screenHeight/2)
	g.groundImg = ebiten.NewImage(screenWidth, screenHeight/2)
	g.UpdateSkyAndGroundColors()

	if g.gameLoop != nil && g.gameLoop.renderer != nil {
		g.gameLoop.renderer.handleResize(screenWidth, screenHeight)
	}
}

// Shutdown releases threading resources. Safe to call multiple times only via
// the threading components' own idempotency — call once on game exit.
func (g *MMGame) Shutdown() {
	if g.threading != nil {
		g.threading.Shutdown()
	}
}

// checkGameOver sets gameOver when all party members are unconscious (HP <= 0)
func (g *MMGame) checkGameOver() {
	if g.gameOver {
		return
	}
	allDown := true
	for _, m := range g.party.Members {
		if m.HitPoints > 0 {
			allDown = false
			break
		}
	}
	if allDown {
		g.gameOver = true
	}
}

// checkVictory checks if the dragon_slayer quest is completed
func (g *MMGame) checkVictory() {
	if g.gameVictory || g.gameOver {
		return
	}
	if g.questManager == nil {
		return
	}
	quest := g.questManager.GetQuest("dragon_slayer")
	if quest != nil && quest.Status == quests.QuestStatusCompleted {
		g.gameVictory = true
		g.victoryTime = time.Now()
	}
}

// AddCombatMessage adds a combat message to the message queue
func (g *MMGame) AddCombatMessage(message string) {
	g.AddColoredCombatMessage(message, color.White)
}

// AddColoredCombatMessage appends a combat-log entry with an explicit display
// color. The HUD slice is derived on demand (GetCombatMessages), so text and
// color can never fall out of sync.
func (g *MMGame) AddColoredCombatMessage(message string, messageColor color.Color) {
	g.combatLogHistory = append(g.combatLogHistory, combatLogEntry{Text: message, Color: messageColor})
	if len(g.combatLogHistory) > maxCombatLogHistory {
		g.combatLogHistory = g.combatLogHistory[len(g.combatLogHistory)-maxCombatLogHistory:]
	}
}

// SummonRandomMonsterNearPlayer spawns a random monster roughly distanceTiles away if possible
func (g *MMGame) SummonRandomMonsterNearPlayer(distanceTiles float64) bool {
	if world.GlobalWorldManager == nil || monster.MonsterConfig == nil {
		return false
	}
	px, py := g.camera.X, g.camera.Y
	tileSize := float64(g.config.GetTileSize())
	// Choose a random angle and target exactly distanceTiles away, then find nearest walkable
	angle := rand.Float64() * 2 * math.Pi
	targetX := px + math.Cos(angle)*distanceTiles*tileSize
	targetY := py + math.Sin(angle)*distanceTiles*tileSize
	sx, sy := g.FindNearestWalkableTile(targetX, targetY)
	if sx == -1 && sy == -1 {
		return false
	}

	// Pick a random monster key from config
	keys := monster.MonsterConfig.GetAllMonsterKeys()
	if len(keys) == 0 {
		return false
	}
	key := keys[rand.Intn(len(keys))]

	// Create and register the monster
	m := monster.NewMonster3DFromConfig(sx, sy, key, g.config)
	if m == nil {
		return false
	}
	g.registerSpawnedMonster(m)
	g.AddCombatMessage(fmt.Sprintf("A %s appears!", m.Name))
	return true
}

// hudLog returns the tail of the combat log shown on the HUD (last maxMessages
// entries). GetCombatMessages and GetCombatMessageColor both index into it, so
// the row text and its color always come from the same entry.
func (g *MMGame) hudLog() []combatLogEntry {
	n := g.maxMessages
	if n <= 0 || n > len(g.combatLogHistory) {
		n = len(g.combatLogHistory)
	}
	return g.combatLogHistory[len(g.combatLogHistory)-n:]
}

// GetCombatMessages returns the HUD combat-message texts (most recent last).
func (g *MMGame) GetCombatMessages() []string {
	hud := g.hudLog()
	out := make([]string, len(hud))
	for i, e := range hud {
		out[i] = e.Text
	}
	return out
}

// GetCombatMessageColor returns the display color for HUD row index (aligned with
// GetCombatMessages).
func (g *MMGame) GetCombatMessageColor(index int) color.Color {
	hud := g.hudLog()
	if index < 0 || index >= len(hud) {
		return color.White
	}
	return hud[index].Color
}

// cardFx identifies a party-card overlay effect tracked per member in
// cardFxTimers. Durations: blink uses the config value, the rest the consts.
type cardFx int

const (
	fxBlink cardFx = iota // red damage flash (portrait tint)
	fxFlame               // Inferno scorch flames
	fxSpark               // whole-card red flash + spark burst on a hit
	fxHeal                // rising green "+" on a heal
	cardFxCount
)

// HitSparkFrames is how long the hit feedback (whole-card red flash + spark
// burst) plays on a party card after the member takes a hit.
const HitSparkFrames = 18

// PartyFlameFrames is how long the Inferno flame overlay burns on a card.
const PartyFlameFrames = 45

// HealEffectFrames is how long the rising green "+" overlay plays on a card.
const HealEffectFrames = 48

// triggerCardFx lights one card overlay on one member for `frames` frames.
func (g *MMGame) triggerCardFx(fx cardFx, characterIndex, frames int) {
	if characterIndex >= 0 && characterIndex < len(g.cardFxTimers[fx]) {
		g.cardFxTimers[fx][characterIndex] = frames
	}
}

// cardFxActive returns the remaining frames of a card overlay (0 = inactive).
func (g *MMGame) cardFxActive(fx cardFx, characterIndex int) int {
	if characterIndex >= 0 && characterIndex < len(g.cardFxTimers[fx]) {
		return g.cardFxTimers[fx][characterIndex]
	}
	return 0
}

// TriggerDamageBlink triggers the red blink AND a spark burst on a character's
// card — fired wherever a member takes a visible hit, so impacts read clearly.
func (g *MMGame) TriggerDamageBlink(characterIndex int) {
	g.triggerCardFx(fxBlink, characterIndex, g.config.UI.DamageBlinkFrames)
	g.triggerCardFx(fxSpark, characterIndex, HitSparkFrames)
}

// TriggerPartyFlame lights the Inferno flame-particle overlay on a party card.
func (g *MMGame) TriggerPartyFlame(characterIndex int) {
	g.triggerCardFx(fxFlame, characterIndex, PartyFlameFrames)
}

// TriggerPartyHeal lights the rising green "+" overlay on a healed member's card.
func (g *MMGame) TriggerPartyHeal(characterIndex int) {
	g.triggerCardFx(fxHeal, characterIndex, HealEffectFrames)
}

// UpdateDamageBlinkTimers decrements every card-overlay timer each frame.
func (g *MMGame) UpdateDamageBlinkTimers() {
	for fx := range g.cardFxTimers {
		for i := range g.cardFxTimers[fx] {
			if g.cardFxTimers[fx][i] > 0 {
				g.cardFxTimers[fx][i]--
			}
		}
	}
}

// UpdateMonsterHitTintTimers decrements hit tint timers for monsters each frame
func (g *MMGame) UpdateMonsterHitTintTimers() {
	for _, m := range g.world.Monsters {
		if m.HitTintFrames > 0 {
			m.HitTintFrames--
		}
		if m.AttackAnimFrames > 0 {
			m.AttackAnimFrames--
		}
	}
}

// refreshBoundUndeadCache rebuilds the per-frame list of bound undead (bind_undead)
// so the AI-target lookup can let normal mobs retaliate against them without an
// O(n²) scan when none exist. Called once per frame before the monster update.
func (g *MMGame) refreshBoundUndeadCache() {
	g.boundUndead = g.boundUndead[:0]
	for _, m := range g.world.Monsters {
		if m != nil && m.Bound && m.IsAlive() {
			g.boundUndead = append(g.boundUndead, m)
		}
	}
	// Precompute each monster's foe + pursuit target ONCE per frame, single-threaded,
	// so the parallel real-time update never scans other monsters' positions (which
	// are being mutated concurrently) and no consumer recomputes the foe. The wrapper
	// reads AITargetX/Y; combat reads AIFoe.
	if g.combat == nil {
		return
	}
	for _, m := range g.world.Monsters {
		if m == nil {
			continue
		}
		m.AIFoe = g.combat.monsterAIFoeMonster(m)
		m.AITargetX, m.AITargetY = g.combat.monsterAITargetPoint(m)
		// Aggressive boss → relentless chase; evasive boss holds (handled by boss hook).
		m.BossAggro = g.combat.isBoss(m) && !g.combat.bossEvasive(m)
		// Sealed boss (passive-until-quest, no evade radius) → freeze on its spawn
		// until the quest unseals it. An evasive boss WITH an evade radius still
		// skitters and blinks, so it is excluded.
		m.BossDormant = g.combat.isBoss(m) && g.combat.bossEvasive(m) && m.EvadeRadiusTiles == 0
	}
}

// IsCharacterBlinking returns true if a character should be rendered with red tint
func (g *MMGame) IsCharacterBlinking(characterIndex int) bool {
	return g.cardFxActive(fxBlink, characterIndex) > 0
}

// Getter methods for turn-based mode testing
func (g *MMGame) IsTurnBasedMode() bool {
	return g.turnBasedMode
}

func (g *MMGame) GetCurrentTurn() int {
	return g.currentTurn
}

func (g *MMGame) GetPartyActionsUsed() int {
	return g.partyActionsUsed
}

// canSelectChar reports whether the player can switch selection to the given
// party index in turn-based mode: the character must still be able to act
// (alive + conscious) AND have at least one action slot left. In real-time
// mode every index is selectable — callers gate this with turnBasedMode.
func (g *MMGame) canSelectChar(idx int) bool {
	if idx < 0 || idx >= len(g.party.Members) {
		return false
	}
	m := g.party.Members[idx]
	return m.CanAct() && m.ActionsRemaining > 0
}

// partyAllExhausted reports whether every still-able-to-act party member has
// spent all their action slots this round. KO members are skipped — the
// round ends when the remaining able-bodied ones are done.
func (g *MMGame) partyAllExhausted() bool {
	for i := range g.party.Members {
		if g.canSelectChar(i) {
			return false
		}
	}
	return true
}

// firstEligiblePartyIndex returns the lowest party index of a character who
// can still act this round. Returns -1 if none.
func (g *MMGame) firstEligiblePartyIndex() int {
	for i := range g.party.Members {
		if g.canSelectChar(i) {
			return i
		}
	}
	return -1
}

// advanceToNextEligibleChar moves selectedChar forward to the next party
// member that can still act this round, wrapping from the end back to the
// start. No-op if none are eligible.
func (g *MMGame) advanceToNextEligibleChar() {
	n := len(g.party.Members)
	for off := 1; off <= n; off++ {
		idx := (g.selectedChar + off) % n
		if g.canSelectChar(idx) {
			g.selectedChar = idx
			return
		}
	}
}

// rtCharReady reports whether a party member can act right now in real-time
// combat: alive, conscious, and off cooldown. The real-time analogue of
// canSelectChar (which is turn-based, gated by action slots).
func (g *MMGame) rtCharReady(idx int) bool {
	if idx < 0 || idx >= len(g.party.Members) {
		return false
	}
	m := g.party.Members[idx]
	return m.CanAct() && m.RTCooldown <= 0
}

// rtActionKind is the real-time action a held key performs. The party cycle is
// capability-aware per action: holding F only visits members who can cast,
// holding C only members who can heal, holding R only members with a weapon.
type rtActionKind int

const (
	rtActNone   rtActionKind = iota
	rtActWeapon              // R: melee/ranged weapon
	rtActSmart               // Space: slotted combat spell else weapon
	rtActCast                // F: cast the slotted spell
	rtActHeal                // C/H: cast the best known heal
)

// rtActionCapable reports whether a party member CAN perform a real-time action
// right now (ignoring cooldown): alive, plus has the weapon / slotted spell /
// known heal AND enough SP for it. Smart-attack always falls back to a weapon
// swing, so everyone is "capable" of it.
func (g *MMGame) rtActionCapable(idx int, kind rtActionKind) bool {
	if idx < 0 || idx >= len(g.party.Members) {
		return false
	}
	m := g.party.Members[idx]
	if m == nil || !m.CanAct() {
		return false
	}
	switch kind {
	case rtActWeapon:
		_, ok := m.Equipment[items.SlotMainHand]
		return ok
	case rtActCast:
		spell, ok := m.Equipment[items.SlotSpell]
		if !ok {
			return false
		}
		cost := spell.SpellCost
		// Traps: the live traps.yaml cost is the truth (a rebalance must not
		// be fooled by the SpellCost frozen into an old save's slot item).
		if spell.Type == items.ItemTrap {
			if def, defOk := config.GetTrapDefinition(string(spell.SpellEffect)); defOk {
				cost = def.SPCost
			}
		}
		return g.combat == nil || m.SpellPoints >= g.combat.effectiveSpellCost(m, cost)
	case rtActHeal:
		if g.combat == nil {
			return false
		}
		id, ok := g.combat.bestKnownHealSpell(m)
		if !ok {
			return false
		}
		def, err := spells.GetSpellDefinitionByID(id)
		return err == nil && m.SpellPoints >= g.combat.effectiveSpellCost(m, def.SpellPointsCost)
	default: // rtActSmart
		return true
	}
}

// rtActionReady = capable of the action AND off cooldown.
func (g *MMGame) rtActionReady(idx int, kind rtActionKind) bool {
	return g.rtActionCapable(idx, kind) && g.party.Members[idx].RTCooldown <= 0
}

// nextReadyRTActor returns the next member (after the selection, wrapping) who is
// ready to do `kind`, or -1 if none. Unlike advanceRTActor it never falls back to
// an on-cooldown member — so callers can WAIT in place instead of churning the
// selection frame while everyone capable is on cooldown.
func (g *MMGame) nextReadyRTActor(kind rtActionKind) int {
	n := len(g.party.Members)
	for off := 1; off <= n; off++ {
		if i := (g.selectedChar + off) % n; g.rtActionReady(i, kind) {
			return i
		}
	}
	return -1
}

// advanceRTActor hands real-time selection to the next member who can do `kind`:
// first a ready (off-cooldown) one, else any capable one still on cooldown (so a
// held key WAITS on a capable member instead of skipping to one who can't, or
// sticking on the current incapable one). Leaves selection put if none capable.
func (g *MMGame) advanceRTActor(kind rtActionKind) {
	n := len(g.party.Members)
	for off := 1; off <= n; off++ {
		if i := (g.selectedChar + off) % n; g.rtActionReady(i, kind) {
			g.selectedChar = i
			return
		}
	}
	for off := 1; off <= n; off++ {
		if i := (g.selectedChar + off) % n; g.rtActionCapable(i, kind) {
			g.selectedChar = i
			return
		}
	}
}

// ensureSelectedCanActRT guarantees the real-time selection points at a member
// who can still act (alive + conscious). If the current one was just killed/KO'd
// it moves to the next ready member, or failing that any living member (who'll
// act once their cooldown ends) — without this the party freezes on a corpse.
func (g *MMGame) ensureSelectedCanActRT() {
	if g.rtCharReady(g.selectedChar) {
		return
	}
	cur := g.party.Members[g.selectedChar]
	if cur != nil && cur.CanAct() {
		return // alive, just on cooldown — leave selection put
	}
	n := len(g.party.Members)
	// Prefer a member ready right now.
	for off := 1; off <= n; off++ {
		idx := (g.selectedChar + off) % n
		if g.rtCharReady(idx) {
			g.selectedChar = idx
			return
		}
	}
	// Otherwise any living member (still on cooldown — they'll fire when ready).
	for off := 1; off <= n; off++ {
		idx := (g.selectedChar + off) % n
		if m := g.party.Members[idx]; m != nil && m.CanAct() {
			g.selectedChar = idx
			return
		}
	}
}

// startPartyTurn resets ActionsRemaining for every able-bodied party member
// based on their effective Speed and snaps selectedChar to the first one.
// Called when entering turn-based mode and at the end of each monster turn.
// KO members get 0 slots — they skip the round entirely.
func (g *MMGame) startPartyTurn() {
	for _, m := range g.party.Members {
		if m.CanAct() {
			m.ActionsRemaining = m.ActionSlotsForTurn()
		} else {
			m.ActionsRemaining = 0
		}
	}
	if idx := g.firstEligiblePartyIndex(); idx >= 0 {
		g.selectedChar = idx
	}
}

// endPartyTurn ends the party's turn and starts the monster turn. Every
// TurnBasedSpRegenEveryNRounds rounds, every able-bodied party member gets
// one SP regen tick (Personality-derived); KO members are skipped via
// RegenerateSpellPoints. Lives on MMGame so non-input callers (spellbook
// double-click, future UI dialogs) can drive a turn end too.
func (g *MMGame) endPartyTurn() {
	g.turnBasedSpRegenCount++
	if g.turnBasedSpRegenCount >= TurnBasedSpRegenEveryNRounds {
		g.turnBasedSpRegenCount = 0
		for _, member := range g.party.Members {
			member.RegenerateSpellPoints()
		}
	}

	g.currentTurn = 1 // Monster turn
	g.monsterTurnResolved = false
}

// ensureSelectedCharCanAct auto-advances selectedChar to the next eligible
// member when the current one can no longer act. Necessary because a party
// member can become KO mid-party-turn from sources outside the action loop:
// in-flight projectiles fired during the previous monster turn that connect
// during the party turn, poison ticks, etc. Without this guard pressing
// Space/F on a dead selectedChar would silently do nothing.
// Real-time mode no-ops — selection isn't gated on CanAct there.
func (g *MMGame) ensureSelectedCharCanAct() {
	if !g.turnBasedMode {
		return
	}
	if g.canSelectChar(g.selectedChar) {
		return
	}
	if idx := g.firstEligiblePartyIndex(); idx >= 0 {
		g.selectedChar = idx
	}
}

// consumeSelectedCharAction spends one action slot on the currently selected
// character (turn-based mode only — no-op otherwise). If they ran out,
// auto-advances to the next eligible character. If nobody is left with
// actions, ends the party turn so monsters can move.
func (g *MMGame) consumeSelectedCharAction() {
	if !g.turnBasedMode {
		return
	}
	selected := g.party.Members[g.selectedChar]
	if selected.ActionsRemaining > 0 {
		selected.ActionsRemaining--
	}
	if selected.ActionsRemaining == 0 {
		if g.partyAllExhausted() {
			g.endPartyTurn()
			return
		}
		g.advanceToNextEligibleChar()
	}
}

func (g *MMGame) ToggleTurnBasedMode() {
	g.turnBasedMode = !g.turnBasedMode

	// RT cooldowns only tick in real-time, so clear them on every switch — otherwise
	// one frozen across a turn-based fight gates RT actions afterwards. Each mode
	// starts ready.
	for _, m := range g.party.Members {
		if m != nil {
			m.RTCooldown = 0
		}
	}
	g.spellInputCooldown = 0

	if g.turnBasedMode {
		// Snap to tile center immediately
		g.snapToTileCenter()

		// Snap all monsters to tile centers
		g.snapMonstersToTileCenters()

		// Reset all monster AI states for turn-based combat
		g.resetMonsterStatesForTurnBased()

		// Initialize turn-based state
		g.currentTurn = 0 // Start with party turn
		g.partyActionsUsed = 0
		g.turnBasedMoveCooldown = 0
		g.turnBasedRotCooldown = 0
		g.monsterTurnResolved = false
		g.startPartyTurn()
		g.AddCombatMessage("Turn-based mode activated!")
	} else {
		g.AddCombatMessage("Real-time mode activated!")
	}
}

// snapToTileCenter moves the player to the center of their current tile and snaps direction to nearest cardinal
func (g *MMGame) snapToTileCenter() {
	tileSize := float64(g.config.GetTileSize())

	// Get current tile coordinates
	currentTileX := int(g.camera.X / tileSize)
	currentTileY := int(g.camera.Y / tileSize)

	// Calculate exact center of current tile
	centerX, centerY := TileCenterFromTile(currentTileX, currentTileY, tileSize)

	// Snap to center
	g.camera.X = centerX
	g.camera.Y = centerY
	g.collisionSystem.UpdateEntity("player", centerX, centerY)

	// Snap direction to nearest cardinal direction (N, E, S, W)
	g.snapToCardinalDirection()
}

// snapToCardinalDirection snaps the camera angle to the nearest cardinal direction
func (g *MMGame) snapToCardinalDirection() {
	// Normalize current angle to 0-2π range
	angle := g.camera.Angle
	for angle < 0 {
		angle += 2 * math.Pi
	}
	for angle >= 2*math.Pi {
		angle -= 2 * math.Pi
	}

	// Cardinal directions in radians:
	// East (right): 0
	// North (up): 3π/2 (or -π/2)
	// West (left): π
	// South (down): π/2

	cardinalDirections := []float64{
		0,               // East
		math.Pi / 2,     // South
		math.Pi,         // West
		3 * math.Pi / 2, // North
	}

	// Find the closest cardinal direction
	minDiff := math.Pi // Maximum possible difference
	closestDirection := 0.0

	for _, cardinal := range cardinalDirections {
		// Calculate angular difference (handle wrap-around)
		diff := math.Abs(angle - cardinal)
		if diff > math.Pi {
			diff = 2*math.Pi - diff
		}

		if diff < minDiff {
			minDiff = diff
			closestDirection = cardinal
		}
	}

	g.camera.Angle = closestDirection
}

// snapMonstersToTileCenters moves all monsters to the center of their current tiles
func (g *MMGame) snapMonstersToTileCenters() {
	tileSize := float64(g.config.GetTileSize())

	for _, monster := range g.world.Monsters {
		if !monster.IsAlive() {
			continue
		}

		// Get current tile coordinates for this monster
		currentTileX := int(monster.X / tileSize)
		currentTileY := int(monster.Y / tileSize)

		// Calculate exact center of current tile
		centerX, centerY := TileCenterFromTile(currentTileX, currentTileY, tileSize)

		// Snap monster to center
		monster.X = centerX
		monster.Y = centerY

		// Update collision system
		g.collisionSystem.UpdateEntity(monster.ID, centerX, centerY)
	}
}

// resetMonsterStatesForTurnBased resets all monster AI states to neutral for turn-based combat
func (g *MMGame) resetMonsterStatesForTurnBased() {
	for _, currentMonster := range g.world.Monsters {
		if !currentMonster.IsAlive() {
			continue
		}

		// Reset to idle state - monsters will be controlled explicitly by turn-based system
		currentMonster.State = monster.StateIdle
		currentMonster.StateTimer = 0
		currentMonster.IsEngagingPlayer = false
		currentMonster.AttackCount = 0

		// Reset movement direction to face player (for visual consistency)
		dx := g.camera.X - currentMonster.X
		dy := g.camera.Y - currentMonster.Y
		if dx != 0 || dy != 0 {
			currentMonster.Direction = math.Atan2(dy, dx)
		}
	}
}

// Wrapper types for threading system integration

// MonsterWrapper implements entities.MonsterUpdateInterface
type MonsterWrapper struct {
	Monster         *monster.Monster3D
	collisionSystem *collision.CollisionSystem
	game            *MMGame // Added to access camera position for tethering system
}

func (mw *MonsterWrapper) Update() {
	oldX, oldY := mw.Monster.X, mw.Monster.Y

	// Get player position from camera for tethering system
	playerX := mw.game.camera.X
	playerY := mw.game.camera.Y

	// AI pursuit/engagement target: normally the party, but charmed monsters are
	// redirected (a bound undead seeks its enemy; a pacified charm holds position)
	// so they never chase the party. Precomputed single-threaded each frame in
	// refreshBoundUndeadCache to keep this parallel update race-free.
	targetX, targetY := mw.Monster.AITargetX, mw.Monster.AITargetY

	// Use collision-aware update with the chosen AI target for tethering
	mw.Monster.Update(mw.collisionSystem, targetX, targetY)

	newX, newY := mw.Monster.X, mw.Monster.Y

	mw.updateCollisionEngagement(playerX, playerY)

	// Temporary movement debug (opt-in via env var).
	// Example: DEBUG_MONSTER=bandit
	if filter := strings.TrimSpace(os.Getenv("DEBUG_MONSTER")); filter != "" {
		name := strings.ToLower(mw.Monster.Name)
		needle := strings.ToLower(filter)
		if strings.Contains(name, needle) {
			// Throttle logs to avoid spamming.
			if mw.Monster.StateTimer%60 == 0 {
				withinTether := mw.Monster.IsWithinTetherRadius()
				fmt.Printf(
					"[MONDBG] name=%q id=%s state=%d timer=%d engaging=%v withinTether=%v pos=(%.1f,%.1f) old=(%.1f,%.1f) spawn=(%.1f,%.1f) tether=%.1f player=(%.1f,%.1f)\n",
					mw.Monster.Name,
					mw.Monster.ID,
					mw.Monster.State,
					mw.Monster.StateTimer,
					mw.Monster.IsEngagingPlayer,
					withinTether,
					newX,
					newY,
					oldX,
					oldY,
					mw.Monster.SpawnX,
					mw.Monster.SpawnY,
					mw.Monster.TetherRadius,
					playerX,
					playerY,
				)

				// If not moving while supposed to wander, probe cardinal target tile centers.
				if mw.collisionSystem != nil && (mw.Monster.State == monster.StateIdle || mw.Monster.State == monster.StatePatrolling) {
					if oldX == newX && oldY == newY {
						const tileSize = 64.0
						centerX := TileCenter(newX, tileSize)
						centerY := TileCenter(newY, tileSize)

						fmt.Printf("[MONDBG] center=(%.1f,%.1f) last=(%.1f,%.1f) stuck=%d lastChosenDir=%.3f\n",
							centerX, centerY,
							mw.Monster.LastX, mw.Monster.LastY,
							mw.Monster.StuckCounter,
							mw.Monster.LastChosenDir,
						)

						targets := []struct {
							label string
							dx    float64
							dy    float64
						}{
							{label: "E", dx: tileSize, dy: 0},
							{label: "S", dx: 0, dy: tileSize},
							{label: "W", dx: -tileSize, dy: 0},
							{label: "N", dx: 0, dy: -tileSize},
						}

						for _, t := range targets {
							x := centerX + t.dx
							y := centerY + t.dy
							ok, reason := mw.collisionSystem.DebugCanMoveTo(mw.Monster.ID, x, y)
							fmt.Printf("[MONDBG] step %s -> (%.1f,%.1f) ok=%v reason=%s withinTether=%v\n",
								t.label,
								x,
								y,
								ok,
								reason,
								mw.Monster.CanMoveWithinTether(x, y),
							)
						}
					}
				}
			}
		}
	}

	// Update collision system if position changed
	if oldX != newX || oldY != newY {
		if mw.collisionSystem != nil {
			mw.collisionSystem.UpdateEntity(mw.Monster.ID, newX, newY)
		}
	}
}

func (mw *MonsterWrapper) updateCollisionEngagement(playerX, playerY float64) {
	if mw.game == nil || mw.Monster == nil {
		return
	}
	mw.game.updateMonsterCollisionEngagement(mw.Monster, playerX, playerY)
}

func (g *MMGame) updateMonsterCollisionEngagement(m *monster.Monster3D, playerX, playerY float64) {
	if g == nil || g.collisionSystem == nil || m == nil {
		return
	}
	entity := g.collisionSystem.GetEntityByID(m.ID)
	if entity == nil {
		return
	}
	engaged := m.IsEngagingPlayer || m.State == monster.StateAttacking
	if !engaged {
		attackRange := m.GetAttackRangePixels()
		if attackRange > 0 {
			distSq := DistanceSquared(m.X, m.Y, playerX, playerY)
			if distSq <= attackRange*attackRange {
				engaged = true
			}
		}
	}
	desired := collision.CollisionTypeMonster
	if engaged {
		desired = collision.CollisionTypeMonsterEngaged
	}
	if entity.CollisionType != desired {
		entity.CollisionType = desired
	}
}

func (mw *MonsterWrapper) IsAlive() bool {
	return mw.Monster.IsAlive()
}

func (mw *MonsterWrapper) GetPosition() (float64, float64) {
	return mw.Monster.X, mw.Monster.Y
}

func (mw *MonsterWrapper) SetPosition(x, y float64) {
	mw.Monster.X = x
	mw.Monster.Y = y
	// Update collision system position
	if mw.collisionSystem != nil {
		mw.collisionSystem.UpdateEntity(mw.Monster.ID, x, y)
	}
}

// MagicProjectileWrapper implements entities.ProjectileUpdateInterface
type MagicProjectileWrapper struct {
	MagicProjectile *MagicProjectile
	collisionSystem *collision.CollisionSystem
	projectileID    string
	game            *MMGame
}

func (mpw *MagicProjectileWrapper) Update() {
	mpw.MagicProjectile.LifeTime--
	if mpw.MagicProjectile.LifeTime <= 0 {
		mpw.MagicProjectile.Active = false
	}
}

func (mpw *MagicProjectileWrapper) IsActive() bool {
	return mpw.MagicProjectile.Active && mpw.MagicProjectile.LifeTime > 0
}

func (mpw *MagicProjectileWrapper) GetPosition() (float64, float64) {
	return mpw.MagicProjectile.X, mpw.MagicProjectile.Y
}

func (mpw *MagicProjectileWrapper) SetPosition(x, y float64) {
	mpw.MagicProjectile.X = x
	mpw.MagicProjectile.Y = y
	// Update collision system position
	if mpw.collisionSystem != nil {
		mpw.collisionSystem.UpdateEntity(mpw.projectileID, x, y)
	}
}

func (mpw *MagicProjectileWrapper) GetVelocity() (float64, float64) {
	return mpw.MagicProjectile.VelX, mpw.MagicProjectile.VelY
}

func (mpw *MagicProjectileWrapper) SetVelocity(vx, vy float64) {
	mpw.MagicProjectile.VelX = vx
	mpw.MagicProjectile.VelY = vy
}

func (mpw *MagicProjectileWrapper) OnCollision(hitX, hitY float64) {
	if mpw.MagicProjectile == nil || !mpw.MagicProjectile.Active {
		return
	}
	mpw.MagicProjectile.Active = false
	if mpw.game == nil {
		return
	}

	mpw.game.CreateSpellHitEffectFromSpell(hitX, hitY, mpw.MagicProjectile.SpellType)
}

// ArrowWrapper implements entities.ProjectileUpdateInterface
type ArrowWrapper struct {
	Arrow           *Arrow
	collisionSystem *collision.CollisionSystem
	projectileID    string
	game            *MMGame
}

func (aw *ArrowWrapper) Update() {
	aw.Arrow.LifeTime--
	if aw.Arrow.LifeTime <= 0 {
		aw.Arrow.Active = false
	}
}

func (aw *ArrowWrapper) IsActive() bool {
	return aw.Arrow.Active && aw.Arrow.LifeTime > 0
}

func (aw *ArrowWrapper) GetPosition() (float64, float64) {
	return aw.Arrow.X, aw.Arrow.Y
}

func (aw *ArrowWrapper) SetPosition(x, y float64) {
	aw.Arrow.X = x
	aw.Arrow.Y = y
	// Update collision system position
	if aw.collisionSystem != nil {
		aw.collisionSystem.UpdateEntity(aw.projectileID, x, y)
	}
}

func (aw *ArrowWrapper) GetVelocity() (float64, float64) {
	return aw.Arrow.VelX, aw.Arrow.VelY
}

func (aw *ArrowWrapper) SetVelocity(vx, vy float64) {
	aw.Arrow.VelX = vx
	aw.Arrow.VelY = vy
}

func (aw *ArrowWrapper) OnCollision(hitX, hitY float64) {
	if aw.Arrow == nil || !aw.Arrow.Active {
		return
	}
	aw.Arrow.Active = false
	if aw.game == nil {
		return
	}
	// Staff/book bolt → magical burst on wall/terrain impact, not an arrow puff
	// (shares the monster-hit decision so the staff never "explodes like an arrow").
	def, _ := config.GetWeaponDefinition(aw.Arrow.BowKey)
	aw.game.spawnWeaponBoltImpact(hitX, hitY, def, SpellParticleCount, SpellParticleSize)
}

func (aw *ArrowWrapper) GetLifetime() int {
	return aw.Arrow.LifeTime
}

func (aw *ArrowWrapper) SetLifetime(lifetime int) {
	aw.Arrow.LifeTime = lifetime
}

func (mpw *MagicProjectileWrapper) GetLifetime() int {
	return mpw.MagicProjectile.LifeTime
}

func (mpw *MagicProjectileWrapper) SetLifetime(lifetime int) {
	mpw.MagicProjectile.LifeTime = lifetime
}

// MeleeAttackWrapper implements entities.ProjectileUpdateInterface
type MeleeAttackWrapper struct {
	MeleeAttack     *MeleeAttack
	collisionSystem *collision.CollisionSystem
	projectileID    string
	game            *MMGame
}

func (mw *MeleeAttackWrapper) Update() {
	mw.MeleeAttack.LifeTime--
	if mw.MeleeAttack.LifeTime <= 0 {
		mw.MeleeAttack.Active = false
	}
}

func (mw *MeleeAttackWrapper) IsActive() bool {
	return mw.MeleeAttack.Active && mw.MeleeAttack.LifeTime > 0
}

func (mw *MeleeAttackWrapper) GetPosition() (float64, float64) {
	return mw.MeleeAttack.X, mw.MeleeAttack.Y
}

func (mw *MeleeAttackWrapper) SetPosition(x, y float64) {
	mw.MeleeAttack.X = x
	mw.MeleeAttack.Y = y
	// Update collision system position
	if mw.collisionSystem != nil {
		mw.collisionSystem.UpdateEntity(mw.projectileID, x, y)
	}
}

func (mw *MeleeAttackWrapper) GetVelocity() (float64, float64) {
	return mw.MeleeAttack.VelX, mw.MeleeAttack.VelY
}

func (mw *MeleeAttackWrapper) SetVelocity(vx, vy float64) {
	mw.MeleeAttack.VelX = vx
	mw.MeleeAttack.VelY = vy
}

func (mw *MeleeAttackWrapper) OnCollision(hitX, hitY float64) {
	if mw.MeleeAttack == nil {
		return
	}
	mw.MeleeAttack.Active = false
}

func (mw *MeleeAttackWrapper) GetLifetime() int {
	return mw.MeleeAttack.LifeTime
}

func (mw *MeleeAttackWrapper) SetLifetime(lifetime int) {
	mw.MeleeAttack.LifeTime = lifetime
}
