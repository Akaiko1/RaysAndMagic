package game

import (
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"os"
	"strings"
	"sync"

	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"
	"ugataima/internal/threading"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// Constants are now loaded from config.yaml
// Access via config.GlobalConfig or pass config instance

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

// SlashEffect represents a visual slash animation for melee weapons
type SlashEffect struct {
	ID             string  // Unique identifier
	X, Y           float64 // Center position
	Angle          float64 // Direction of slash
	Width, Length  int     // Dimensions of slash
	Color          [3]int  // RGB color
	AnimationFrame int     // Current animation frame
	MaxFrames      int     // Total animation frames
	Active         bool
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

	// Tabbed menu system
	menuOpen     bool
	currentTab   MenuTab
	mousePressed bool // Track mouse state for click detection

	// Double-click support for spellbook
	lastSpellClickTime int64 // Time of last spell click in milliseconds
	lastClickedSpell   int   // Index of last clicked spell
	lastClickedSchool  int   // Index of last clicked school

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
	slashEffects     []SlashEffect

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
	blessActive   bool // Whether bless is currently active
	blessDuration int  // Remaining duration in frames

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
	// Stat distribution popup UI state
	statPopupOpen    bool // Is the stat distribution popup open?
	statPopupCharIdx int  // Which character is being edited in the popup?

	// Turn-based mode state
	turnBasedMode         bool // Whether game is in turn-based mode
	currentTurn           int  // 0 = party turn, 1 = monster turn
	partyActionsUsed      int  // Actions used this turn (0-2)
	turnBasedMoveCooldown int  // Movement cooldown in frames (18 FPS = 0.3 second)
	turnBasedRotCooldown  int  // Rotation cooldown in frames (18 FPS = 0.3 second)

	// Main menu (ESC)
	mainMenuOpen      bool
	mainMenuSelection int
	mainMenuMode      MainMenuMode
	slotSelection     int
	exitRequested     bool

	// Game over state
	gameOver bool
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

		// Tabbed menu system
		menuOpen:     false,
		currentTab:   TabInventory,
		mousePressed: false,

		// Double-click support for spellbook
		lastSpellClickTime: 0,
		lastClickedSpell:   -1,
		lastClickedSchool:  -1,

		// Double-click support for dialogs (neutral)
		dialogLastClickTime:  0,
		dialogLastClickedIdx: -1,

		selectedSchool: 0,
		selectedSpell:  0,
		combatMessages: make([]string, 0),
		maxMessages:    3, // Show last 3 messages

		// Dialog system initialization
		dialogSelectedChar:  0,
		dialogSelectedSpell: 0,

		// Threading components
		threading: threadingComponents,

		// Initialize depth buffer for proper 3D rendering
		depthBuffer: make([]float64, cfg.GetScreenWidth()),
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
		dx := npc.X - g.camera.X
		dy := npc.Y - g.camera.Y
		distance := math.Sqrt(dx*dx + dy*dy)

		if distance <= nearestDistance {
			nearestNPC = npc
			nearestDistance = distance
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
				safeX := float64(x)*tileSize + tileSize/2
				safeY := float64(y)*tileSize + tileSize/2
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
				if abs(dx) != radius && abs(dy) != radius {
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
					safeX := float64(checkX)*tileSize + tileSize/2
					safeY := float64(checkY)*tileSize + tileSize/2
					return safeX, safeY
				}
			}
		}
	}

	// No walkable tile found within radius
	return -1, -1
}

// abs returns absolute value of an integer (DRY helper)
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
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
	centerX := float64(currentTileX)*tileSize + tileSize/2
	centerY := float64(currentTileY)*tileSize + tileSize/2

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
		centerX := float64(currentTileX)*tileSize + tileSize/2
		centerY := float64(currentTileY)*tileSize + tileSize/2

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
	monsterID       string
	game            *MMGame // Added to access camera position for tethering system
}

func (mw *MonsterWrapper) Update() {
	oldX, oldY := mw.Monster.X, mw.Monster.Y

	// Get player position from camera for tethering system
	playerX := mw.game.camera.X
	playerY := mw.game.camera.Y

	// Use collision-aware update with player position for tethering
	mw.Monster.Update(mw.collisionSystem, mw.monsterID, playerX, playerY)

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
					mw.monsterID,
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
						centerX := math.Floor(newX/tileSize)*tileSize + tileSize/2
						centerY := math.Floor(newY/tileSize)*tileSize + tileSize/2

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
							ok, reason := mw.collisionSystem.DebugCanMoveTo(mw.monsterID, x, y)
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
			mw.collisionSystem.UpdateEntity(mw.monsterID, newX, newY)
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
		mw.collisionSystem.UpdateEntity(mw.monsterID, x, y)
	}
}

// MagicProjectileWrapper implements entities.ProjectileUpdateInterface
type MagicProjectileWrapper struct {
	MagicProjectile *MagicProjectile
	collisionSystem *collision.CollisionSystem
	projectileID    string
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

// ArrowWrapper implements entities.ProjectileUpdateInterface
type ArrowWrapper struct {
	Arrow           *Arrow
	collisionSystem *collision.CollisionSystem
	projectileID    string
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

func (mw *MeleeAttackWrapper) GetLifetime() int {
	return mw.MeleeAttack.LifeTime
}

func (mw *MeleeAttackWrapper) SetLifetime(lifetime int) {
	mw.MeleeAttack.LifeTime = lifetime
}
