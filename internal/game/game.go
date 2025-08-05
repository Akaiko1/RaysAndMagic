package game

import (
	"image/color"
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
	X, Y       float64 // Current position
	VelX, VelY float64 // Velocity
	Damage     int
	LifeTime   int // Frames remaining
	Active     bool
	SpellType  string // Type of spell for visual differentiation
	Size       int    // Projectile size
}

// Legacy alias for backward compatibility
type Fireball = MagicProjectile

type SwordAttack struct {
	X, Y       float64 // Current position
	VelX, VelY float64 // Velocity
	Damage     int
	LifeTime   int // Frames remaining
	Active     bool
	WeaponName string // Name of weapon used for combat messages
}

type Arrow struct {
	X, Y       float64 // Current position
	VelX, VelY float64 // Velocity
	Damage     int
	LifeTime   int // Frames remaining
	Active     bool
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
	selectedChar       int

	// Tabbed menu system
	menuOpen     bool
	currentTab   MenuTab
	mousePressed bool // Track mouse state for click detection

	// Double-click support for spellbook
	lastSpellClickTime int64 // Time of last spell click in milliseconds
	lastClickedSpell   int   // Index of last clicked spell
	lastClickedSchool  int   // Index of last clicked school

	// Double-click support for dialog spells
	lastDialogSpellClickTime int64 // Time of last dialog spell click in milliseconds
	lastClickedDialogSpell   int   // Index of last clicked dialog spell

	// Cached images for performance
	skyImg    *ebiten.Image
	groundImg *ebiten.Image

	// Combat effects
	fireballs    []Fireball
	swordAttacks []SwordAttack
	arrows       []Arrow

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

	// Generic stat bonus system (for Bless, Day of Gods, Hour of Power, etc.)
	statBonus int // Total stat bonus from all active effects

	// Dialog system
	dialogActive        bool           // Whether a dialog is currently open
	dialogNPC           *character.NPC // Current NPC being talked to
	dialogSelectedChar  int            // Currently selected character in dialog
	dialogSelectedSpell int            // Currently selected spell in dialog
	selectedCharIdx     int            // Selected character index for spell learning
	selectedSpellKey    string         // Selected spell key for learning

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
}

type GameState int

const (
	GameStateExploration GameState = iota
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
		world:          currentWorld,
		camera:         camera,
		party:          party,
		sprites:        sprites,
		gameState:      GameStateExploration,
		config:         cfg,
		showPartyStats: true,
		showFPS:        false, // FPS counter starts hidden
		selectedChar:   0,
		skyImg:         skyImg,
		groundImg:      groundImg,
		fireballs:      make([]Fireball, 0),
		swordAttacks:   make([]SwordAttack, 0),
		arrows:         make([]Arrow, 0),

		// Tabbed menu system
		menuOpen:     false,
		currentTab:   TabInventory,
		mousePressed: false,

		// Double-click support for spellbook
		lastSpellClickTime: 0,
		lastClickedSpell:   -1,
		lastClickedSchool:  -1,

		// Double-click support for dialog spells
		lastDialogSpellClickTime: 0,
		lastClickedDialogSpell:   -1,

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
	return g.gameLoop.Update()
}

func (g *MMGame) Draw(screen *ebiten.Image) {
	g.gameLoop.Draw(screen)
}

func (g *MMGame) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return g.gameLoop.Layout(outsideWidth, outsideHeight)
}

// AddCombatMessage adds a combat message to the message queue
func (g *MMGame) AddCombatMessage(message string) {
	g.combatMessages = append(g.combatMessages, message)

	// Keep only the last maxMessages
	if len(g.combatMessages) > g.maxMessages {
		g.combatMessages = g.combatMessages[len(g.combatMessages)-g.maxMessages:]
	}
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

// Wrapper types for threading system integration

// MonsterWrapper implements entities.MonsterUpdateInterface
type MonsterWrapper struct {
	Monster         *monster.Monster3D
	collisionSystem *collision.CollisionSystem
	monsterID       string
}

func (mw *MonsterWrapper) Update() {
	oldX, oldY := mw.Monster.X, mw.Monster.Y

	// Always use collision-aware update
	mw.Monster.Update(mw.collisionSystem, mw.monsterID)

	newX, newY := mw.Monster.X, mw.Monster.Y

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

// FireballWrapper implements entities.ProjectileUpdateInterface
type FireballWrapper struct {
	Fireball        *Fireball
	collisionSystem *collision.CollisionSystem
	projectileID    string
}

func (fw *FireballWrapper) Update() {
	fw.Fireball.LifeTime--
	if fw.Fireball.LifeTime <= 0 {
		fw.Fireball.Active = false
	}
}

func (fw *FireballWrapper) IsActive() bool {
	return fw.Fireball.Active && fw.Fireball.LifeTime > 0
}

func (fw *FireballWrapper) GetPosition() (float64, float64) {
	return fw.Fireball.X, fw.Fireball.Y
}

func (fw *FireballWrapper) SetPosition(x, y float64) {
	fw.Fireball.X = x
	fw.Fireball.Y = y
	// Update collision system position
	if fw.collisionSystem != nil {
		fw.collisionSystem.UpdateEntity(fw.projectileID, x, y)
	}
}

func (fw *FireballWrapper) GetVelocity() (float64, float64) {
	return fw.Fireball.VelX, fw.Fireball.VelY
}

func (fw *FireballWrapper) SetVelocity(vx, vy float64) {
	fw.Fireball.VelX = vx
	fw.Fireball.VelY = vy
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

func (fw *FireballWrapper) GetLifetime() int {
	return fw.Fireball.LifeTime
}

func (fw *FireballWrapper) SetLifetime(lifetime int) {
	fw.Fireball.LifeTime = lifetime
}

// SwordAttackWrapper implements entities.ProjectileUpdateInterface
type SwordAttackWrapper struct {
	SwordAttack     *SwordAttack
	collisionSystem *collision.CollisionSystem
	projectileID    string
}

func (sw *SwordAttackWrapper) Update() {
	sw.SwordAttack.LifeTime--
	if sw.SwordAttack.LifeTime <= 0 {
		sw.SwordAttack.Active = false
	}
}

func (sw *SwordAttackWrapper) IsActive() bool {
	return sw.SwordAttack.Active && sw.SwordAttack.LifeTime > 0
}

func (sw *SwordAttackWrapper) GetPosition() (float64, float64) {
	return sw.SwordAttack.X, sw.SwordAttack.Y
}

func (sw *SwordAttackWrapper) SetPosition(x, y float64) {
	sw.SwordAttack.X = x
	sw.SwordAttack.Y = y
	// Update collision system position
	if sw.collisionSystem != nil {
		sw.collisionSystem.UpdateEntity(sw.projectileID, x, y)
	}
}

func (sw *SwordAttackWrapper) GetVelocity() (float64, float64) {
	return sw.SwordAttack.VelX, sw.SwordAttack.VelY
}

func (sw *SwordAttackWrapper) SetVelocity(vx, vy float64) {
	sw.SwordAttack.VelX = vx
	sw.SwordAttack.VelY = vy
}

func (sw *SwordAttackWrapper) GetLifetime() int {
	return sw.SwordAttack.LifeTime
}

func (sw *SwordAttackWrapper) SetLifetime(lifetime int) {
	sw.SwordAttack.LifeTime = lifetime
}
