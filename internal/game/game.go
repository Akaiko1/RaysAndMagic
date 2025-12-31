package game

import (
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
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

const (
	ProjectileOwnerPlayer ProjectileOwner = iota
	ProjectileOwnerMonster
)

type MagicProjectile struct {
	ID         string  // Unique identifier
	X, Y       float64 // Current position
	VelX, VelY float64 // Velocity
	Damage     int
	LifeTime   int // Frames remaining
	Active     bool
	SpellType  string // Type of spell for visual differentiation
	Size       int    // Projectile size
	Crit       bool   // Critical hit flag
	Owner      ProjectileOwner
	SourceName string
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

type SlashEffectStyle int

const (
	SlashEffectStyleSlash SlashEffectStyle = iota
	SlashEffectStyleThrust
)

// SlashEffect represents a visual slash animation for melee weapons
type SlashEffect struct {
	ID             string  // Unique identifier
	X, Y           float64 // Center position
	Angle          float64 // Base direction of the effect
	SweepAngle     float64 // Total sweep in radians for slashes
	Width, Length  int     // Dimensions of slash
	Color          [3]int  // RGB color
	AnimationFrame int     // Current animation frame
	MaxFrames      int     // Total animation frames
	Active         bool
	Style          SlashEffectStyle
}

type Arrow struct {
	ID         string  // Unique identifier
	X, Y       float64 // Current position
	VelX, VelY float64 // Velocity
	Damage     int
	LifeTime   int // Frames remaining
	Active     bool
	BowKey     string // YAML key of the bow used to fire this arrow
	DamageType string // Damage element type ("physical", "dark", etc.)
	Crit       bool   // Critical hit flag
	Owner      ProjectileOwner
	SourceName string
}

// ArrowHitParticle represents a small particle burst for arrow impacts
type ArrowHitParticle struct {
	X, Y       float64
	OffsetX    float64
	OffsetY    float64
	VelX, VelY float64
	LifeTime   int
	MaxLife    int
	Size       int
	Active     bool
	Color      [3]int
}

// ArrowHitEffect represents a short particle burst on arrow impact
type ArrowHitEffect struct {
	Particles []ArrowHitParticle
	Active    bool
}

// SpellHitParticle represents a single particle from a spell impact
type SpellHitParticle struct {
	X, Y       float64 // Current position
	VelX, VelY float64 // Velocity (outward from impact)
	Color      [3]int  // RGB color based on element
	LifeTime   int     // Frames remaining
	MaxLife    int     // Initial lifetime for alpha calculation
	Size       int     // Particle size (shrinks over time)
	Active     bool
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
	"physical": {200, 200, 200}, // Gray
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
	dialogLastClickTime  int64 // Time of last dialog list click in milliseconds
	dialogLastClickedIdx int   // Index of last clicked dialog list entry

	// Double-click support for utility spell icons (dispelling)
	lastUtilitySpellClickTime int64  // Time of last utility spell icon click in milliseconds
	lastClickedUtilitySpell   string // Icon of last clicked utility spell

	// Cached images for performance
	skyImg    *ebiten.Image
	groundImg *ebiten.Image

	// Combat effects
	magicProjectiles []MagicProjectile
	meleeAttacks     []MeleeAttack
	arrows           []Arrow

	// Spellbook UI state
	collapsedSpellSchools map[character.MagicSchool]bool

	// Utility spell status icons (data-driven)
	utilitySpellStatuses map[spells.SpellID]*UtilitySpellStatus
	slashEffects         []SlashEffect
	arrowHitEffects      []ArrowHitEffect
	spellHitEffects      []SpellHitEffect
	hitEffectsMu         sync.Mutex

	// Map overlay UI state
	mapOverlayOpen bool

	// Lighting effects
	torchLightActive   bool    // Whether torch light is currently active
	torchLightDuration int     // Remaining duration in frames
	torchLightRadius   float64 // Radius of the light effect

	// Wizard Eye effect
	wizardEyeActive   bool // Whether wizard eye is currently active
	wizardEyeDuration int  // Remaining duration in frames

	// Walk on Water effect
	walkOnWaterActive   bool // Whether walk on water is currently active
	walkOnWaterDuration int  // Remaining duration in frames

	// Bless effect
	blessActive    bool // Whether bless is currently active
	blessDuration  int  // Remaining duration in frames
	blessStatBonus int  // The stat bonus applied by this Bless cast (for proper removal)

	// Water Breathing effect
	waterBreathingActive   bool    // Whether water breathing is currently active
	waterBreathingDuration int     // Remaining duration in frames
	underwaterReturnX      float64 // X position to return to when spell expires
	underwaterReturnY      float64 // Y position to return to when spell expires
	underwaterReturnMap    string  // Map to return to when spell expires

	// Generic stat bonus system (for Bless, Day of Gods, Hour of Power, etc.)
	statBonus int // Total stat bonus from all active effects

	// Dialog system
	dialogActive        bool           // Whether a dialog is currently open
	dialogNPC           *character.NPC // Current NPC being talked to
	dialogSelectedChar  int            // Currently selected character in dialog
	dialogSelectedSpell int            // Currently selected spell in dialog
	selectedCharIdx     int            // Selected character index for spell learning
	selectedSpellKey    string         // Selected spell key for learning
	selectedChoice      int            // Selected choice in encounter dialogs

	// Spellbook UI
	selectedSchool     int
	selectedSpell      int
	spellInputCooldown int

	// Combat messages
	combatMessages []string
	maxMessages    int

	// Damage blink effects
	damageBlinkTimers [4]int // One timer per party member (0-3)

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

	// Dead monster IDs to remove - populated when monster dies, processed once per frame
	deadMonsterIDs []string
	// Stat distribution popup UI state
	statPopupOpen    bool // Is the stat distribution popup open?
	statPopupCharIdx int  // Which character is being edited in the popup?

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
		selectedChar:     0,
		skyImg:           skyImg,
		groundImg:        groundImg,
		magicProjectiles: make([]MagicProjectile, 0),
		meleeAttacks:     make([]MeleeAttack, 0),
		arrows:           make([]Arrow, 0),
		slashEffects:     make([]SlashEffect, 0),
		arrowHitEffects:  make([]ArrowHitEffect, 0),
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
		collapsedSpellSchools: make(map[character.MagicSchool]bool),
		utilitySpellStatuses:  make(map[spells.SpellID]*UtilitySpellStatus),
		combatMessages:        make([]string, 0),
		maxMessages:           3, // Show last 3 messages

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
		deadMonsterIDs:              make([]string, 0, 16),

		// Session timer for score calculation
		sessionStartTime: time.Now(),
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

// GetNearestInteractableNPC returns the nearest NPC within interaction range, or nil if none
func (g *MMGame) GetNearestInteractableNPC() *character.NPC {
	const interactionDistance = 128.0 // 2 tiles, same as input handler

	currentWorld := g.GetCurrentWorld()
	if currentWorld == nil {
		return nil
	}

	var nearestNPC *character.NPC
	nearestDistance := interactionDistance

	for _, npc := range currentWorld.NPCs {
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

	if world.GlobalWorldManager != nil {
		mapConfig := world.GlobalWorldManager.GetCurrentMapConfig()
		if mapConfig != nil {
			skyColor = mapConfig.SkyColor
			groundColor = mapConfig.DefaultFloorColor
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

	// Update ground image
	g.groundImg.Fill(color.RGBA{uint8(groundColor[0]), uint8(groundColor[1]), uint8(groundColor[2]), 255})
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
	g.combatMessages = append(g.combatMessages, message)

	// Keep only the last maxMessages
	if len(g.combatMessages) > g.maxMessages {
		g.combatMessages = g.combatMessages[len(g.combatMessages)-g.maxMessages:]
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
	g.world.Monsters = append(g.world.Monsters, m)
	// Register with collision system
	width, height := m.GetSize()
	entity := collision.NewEntity(m.ID, m.X, m.Y, width, height, collision.CollisionTypeMonster, true)
	g.collisionSystem.RegisterEntity(entity)

	g.AddCombatMessage(fmt.Sprintf("A %s appears!", m.Name))
	return true
}

// GetCombatMessages returns the current combat messages
func (g *MMGame) GetCombatMessages() []string {
	return g.combatMessages
}

// TriggerDamageBlink triggers the red blink effect for a specific character
func (g *MMGame) TriggerDamageBlink(characterIndex int) {
	if characterIndex >= 0 && characterIndex < len(g.damageBlinkTimers) {
		g.damageBlinkTimers[characterIndex] = g.config.UI.DamageBlinkFrames
	}
}

// UpdateDamageBlinkTimers decrements damage blink timers each frame
func (g *MMGame) UpdateDamageBlinkTimers() {
	for i := range g.damageBlinkTimers {
		if g.damageBlinkTimers[i] > 0 {
			g.damageBlinkTimers[i]--
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

// IsCharacterBlinking returns true if a character should be rendered with red tint
func (g *MMGame) IsCharacterBlinking(characterIndex int) bool {
	if characterIndex >= 0 && characterIndex < len(g.damageBlinkTimers) {
		return g.damageBlinkTimers[characterIndex] > 0
	}
	return false
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

func (g *MMGame) ToggleTurnBasedMode() {
	g.turnBasedMode = !g.turnBasedMode

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

	// Use collision-aware update with player position for tethering
	mw.Monster.Update(mw.collisionSystem, playerX, playerY)

	newX, newY := mw.Monster.X, mw.Monster.Y

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
	aw.game.CreateArrowHitEffect(hitX, hitY, aw.Arrow.VelX, aw.Arrow.VelY)
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
