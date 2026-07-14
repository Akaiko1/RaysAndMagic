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
	"ugataima/internal/stash"
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
	Attacker           *character.MMCharacter // caster (nil = monster/none) - mastery/pierce resolve from HIM at impact; a pointer survives roster swaps mid-flight
	LifeTime           int                    // Frames remaining
	Active             bool
	SpellType          string // Type of spell for visual differentiation
	Size               int    // Projectile size
	Crit               bool   // Critical hit flag
	DisintegrateChance float64
	Owner              ProjectileOwner
	SourceName         string
	SourceMonster      *monster.Monster3D // monster that fired it (nil = party/none) - carries true-damage + on-hit rider flags to impact
	AoE                bool               // monster projectile: on hit, splash damage to the whole party
	NoCollide          bool               // mortar visual (Stone Blossom): the display bolt never collides
}

// SlashEffect represents a visual melee swing (a per-weapon pixel-particle
// flourish; see drawMeleeParticles).
type SlashEffect struct {
	ID             string  // Unique identifier
	X, Y           float64 // Origin (camera position at swing) - for cleanup/debug
	Width, Length  int     // Dimensions from weapon graphics config
	Color          [3]int  // RGB color
	AnimationFrame int     // Current animation frame
	MaxFrames      int     // Total animation frames
	Active         bool
	Kind           string // per-weapon FX flavor: slash/chop/smash/stab/lunge
	Style          string // bespoke legendary flourish (graphics.slash_fx); overrides Kind
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
	Label              string // chat display name override (card-proc bolts); "" = the weapon's name
	DamageType         string // Damage element type ("physical", "dark", etc.)
	Crit               bool   // Critical hit flag
	SuppressAoE        bool   // volley darts past the first: whole-party AoE fires once per volley
	DisintegrateChance float64
	Owner              ProjectileOwner
	SourceName         string
	SourceMonster      *monster.Monster3D // monster that fired it (nil = party/none) - carries true-damage + on-hit rider flags to impact
	// Pierce-through (Arena Arbalest): a hit with PierceLeft > 0 consumes this
	// arrow and spawns a continuation bolt that skips the monster it went through.
	PierceLeft     int
	SkipMonster    *monster.Monster3D
	RenderAngle    float64 // Render-only: smoothed on-screen shaft angle
	RenderAngleSet bool    // Render-only: RenderAngle initialised
}

// SpellHitParticle represents a single particle from a spell impact
type SpellHitParticle struct {
	X, Y             float64 // World anchor (impact point) - fixed; used for projection
	OffsetX, OffsetY float64 // Screen-space offset from the anchor (a real 2D burst)
	VelX, VelY       float64 // Screen-space velocity (px/frame at the anchor's scale)
	Gravity          float64 // Added to VelY each frame (ice shards fall, embers rise)
	Color            [3]int  // RGB color based on element
	LifeTime         int     // Frames remaining
	MaxLife          int     // Initial lifetime for alpha calculation
	Size             int     // Particle size (shrinks over time)
	Trail            bool    // emits a fading breadcrumb trail each few frames (Starburst falling stars)
	Star             bool    // renders as a twinkling 4-point star, not a square (impact_stars)
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
	// entombedMsgFrame throttles the "can't fight inside stone" explanation
	// (see partyEntombed) so held attack keys don't spam the log.
	entombedMsgFrame int64

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
	// prevWorldClickAllowed tracks worldClickAllowed() across frames: the click
	// queues flush on every modal<->world flip so a buffered click never
	// outlives the UI layer it was aimed at.
	prevWorldClickAllowed bool

	// Double-click support for spellbook
	lastSpellClickTime int64 // Time of last spell click in milliseconds
	lastClickedSpell   int   // Index of last clicked spell
	lastClickedSchool  int   // Index of last clicked school
	// Double-click support for spellbook school collapse
	lastSchoolClickTime  int64 // Time of last school click in milliseconds
	lastSchoolClickedIdx int   // Index of last clicked school header

	// Quick-slot drag-and-drop (sampled in updateMouseState, resolved in Draw).
	// A drag is only armed while the menu is open; the in-game bar is double-click
	// only. dragSrc names what's being carried; see quickslots.go.
	dragArmed     bool // left button down on a draggable; awaiting move/release
	dragActive    bool // moved past threshold -> a real drag is in flight
	dragDropAt    int  // 1 = a drop must be resolved this frame (release), else 0
	dragStartX    int  // press position (for the move threshold + source hit-test)
	dragStartY    int
	dragCurX      int // live cursor position (drop target + carried-icon render)
	dragCurY      int
	dragSrc       dragSource // kind of source captured this drag
	dragItem      items.Item // the carried item (copy, for rendering)
	dragInvIndex  int        // source: party inventory index
	dragQuickChar int        // source: quick-slot owner index
	dragQuickSlot int        // source: quick-slot index
	dragSpellID   spells.SpellID
	dragTrapKey   string          // source: trap recipe key (trap book -> quick slot)
	dragEquipSlot items.EquipSlot // source: paperdoll slot (equipped item -> inventory)
	dragEquipChar int             // source: owner of the dragged equipped item (may differ from selectedChar after a 1-4 switch mid-drag)
	// Double-click support for the in-game quick-slot bar
	lastQuickClickTime int64
	lastQuickClickedCh int
	lastQuickClickedSl int

	// Double-click support for dialogs (neutral)
	dialogLastClickTime  int64  // Time of last dialog list click in milliseconds
	dialogLastClickedIdx int    // Index of last clicked dialog list entry
	dialogLastClickZone  string // Which dialog list was clicked (buy/sell/spell/...) - a double-click never spans lists

	// Double-click support for utility spell icons (dispelling)
	lastUtilitySpellClickTime int64  // Time of last utility spell icon click in milliseconds
	lastClickedUtilitySpell   string // Icon of last clicked utility spell

	// Cached images for performance
	skyImg            *ebiten.Image
	groundImg         *ebiten.Image
	skyPanorama       *ebiten.Image
	currentSkyTexture string
	skyShader         *ebiten.Shader // lazily compiled, reused across frames

	// Day/night cycle (day_night.go). skyPanoramaPrev is the outgoing panorama
	// during the phase-flip crossfade.
	dayNightFrames          int
	dayNightIsNight         bool
	dayNightSkipActive      bool
	dayNightSkipTargetFrame int
	dayNightSkipPhases      []bool
	dayNightOutdoor         bool // current map's sky ships a _day/_night variant
	skyPanoramaPrev         *ebiten.Image
	skyFadeFrames           int
	skyFadeTotal            int

	// Combat effects
	magicProjectiles []MagicProjectile
	arrows           []Arrow
	groundContainers []GroundContainer // unified loot bags + treasure chests on the ground
	// containerFanOffsets caches each container's render-only fan offset for the
	// current frame (see groundContainerRenderOffset): rebuilt in one O(n) tile
	// grouping pass, then O(1) per lookup - the render + hit-test paths query
	// every container several times a frame. Invalidated (containerFanDirty) on
	// any container add/remove; containers never move, so nothing else dirties it.
	containerFanOffsets map[*GroundContainer][2]float64
	containerFanDirty   bool

	// Spellbook UI state
	collapsedSpellSchools map[character.MagicSchoolID]bool

	// Utility spell status icons (data-driven)
	utilitySpellStatuses map[spells.SpellID]*UtilitySpellStatus
	slashEffects         []SlashEffect
	spellHitEffects      []SpellHitEffect
	buffFxAnims          []buffFxAnim  // buff-cast overlay animations (render_buff_fx.go)
	impactLights         []ImpactLight // short-lived light flashes at spell impacts (guarded by hitEffectsMu)
	screenShake          float64       // camera shake amplitude in world units, decays each tick
	screenShakeOffsetX   float64       // live Draw-time camera shake displacement (0 outside Draw); subtract for logical camera
	screenShakeOffsetY   float64
	hitEffectsMu         sync.Mutex

	// Smooth turn-based rotation: logic snaps camera.Angle 90deg instantly (no
	// gameplay change), but the RENDER uses viewAngleRender, which eases toward it
	// over viewTurnFramesLeft frames so a TB turn glides instead of popping. While
	// the ease runs, the scene (rendered to sceneBuf) is drawn through a horizontal
	// directional-blur shader whose length tracks the turn speed - camera motion
	// blur, the standard post-process for a yaw turn (the scene pans horizontally).
	viewAngleRender    float64
	viewTurnFramesLeft int
	sceneBuf           *ebiten.Image  // offscreen 3D scene (blur shader source)
	blurShader         *ebiten.Shader // lazily compiled horizontal motion blur
	turnBlurWarm       bool           // first draw prewarms shader/buffer before the first real turn

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

	// Fly effect: the party passes through any non-border tile (outdoor maps only).
	flyActive   bool
	flyDuration int // Remaining duration in frames

	// Stone Blossom mortars in flight: detonation is scheduled at cast time
	// (the arc ignores everything until it lands). Transient - not saved.
	pendingMortars []pendingMortar

	// Town Portal picker (visited destinations). Transient UI state.
	townPortalPickerOpen bool
	// visitedTavernMaps retains its legacy save-field name. It contains map keys
	// of all Town Portal destinations the party has visited: tavern maps plus
	// maps explicitly marked town_portal_destination in map_configs.yaml.
	visitedTavernMaps map[string]bool

	// Bless effect
	// statBuffs is the registry of active stat-buff spells (Bless, ...): different
	// spells stack, recasting one refreshes it. g.statBonuses is DERIVED as
	// their sum via recomputeStatBonuses - never mutate it directly.
	statBuffs []TimedStatBuff

	// Stacking timed party combat buffs (Day of the Gods, Hour of Power, Stone
	// Skin, Heroism, ...) - see combat_buffs.go. Their ResistPct/OutBonus/InReduce
	// sum across all active entries.
	combatBuffs []TimedCombatBuff

	// Persistent damage zones (Hot Steam) - see combat_zones.go.
	steamZones   []SteamZone
	traps        []PlacedTrap // armed thief traps (map-scoped, persisted)
	selectedTrap int          // trap-book browse index (selection != equipped quick trap)

	// boundAllies caches the bound undead (bind_undead) present this frame so the
	// per-monster AI-target lookup can let normal mobs turn on them without an
	// O(n^2) scan in the common (no-bind) case. Rebuilt each frame before the
	// monster update; see refreshBoundAllyCache.
	boundAllies []*monster.Monster3D

	// Door state (render_category "door"): closed iff a living champion is on
	// the current map; doorEntityIDs tracks the solid collision entities the
	// per-frame reconciler currently has registered. See doors.go.
	doorsClosed   bool
	doorEntityIDs map[string]bool

	// dayNightDay counts day/night phase changes (the arena's refresh clock).
	// arenaTierFoughtDay: difficulty tier -> dayNightDay it was last challenged;
	// a tier unlocks again when the day advances. Both persisted in saves.
	dayNightDay        int
	arenaTierFoughtDay map[string]int
	// Calendar advances only at dawn. Unlike dayNightDay it represents real
	// calendar boundaries for weekly/monthly content events and is save-backed.
	calendarDay   int
	calendarWeek  int
	calendarMonth int
	// playthroughID identifies this run (minted on new game, persisted; JSON
	// key stays arena_run_id for save compat). Consumed by the save-row glow
	// and, with the in-game day, by the leaderboard's anti-farm credit token.
	playthroughID string

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
	focusedNPC          *character.NPC // NPC in interact focus (centred + adjacent); recomputed each tick
	dialogSelectedChar  int            // Currently selected character in dialog
	dialogSelectedSpell int            // Currently selected spell in dialog
	selectedCharIdx     int            // Selected character index for spell learning
	skillTrainerPopup   bool           // Skill trainer: per-character mastery popup open
	skillTrainerPage    int            // Skill trainer: mastery-list page (0-based); shared by renderer and input
	selectedSpellKey    string         // Selected spell key for learning
	selectedChoice      int            // Selected choice in encounter dialogs
	// dialogNodePath is the chain of "info" choices the player has descended into
	// this conversation (empty = root). It drives the body text and choice list so
	// "ask about X" branches into a real reply instead of closing. Reset on open.
	dialogNodePath       []*character.NPCDialogueChoice
	dialogTab            int // Spell-trader tab: 0 = spells, 1 = quests (quest-giving traders)
	merchantBuyPage      int // Merchant buy-grid page (0-based); read by both renderer and input
	merchantSellPage     int // Merchant sell-grid page (0-based)
	spellTraderPage      int // Spell-trader icon-grid page (0-based); shared by renderer and input
	cardCollectorInvPage int // Card-collector loose-card grid page (0-based); shared by renderer and input
	// cardSlots is the party-wide monster-card collection (MaxCardSlots).
	// Cards held here grant passive effects; only the card collector mutates it,
	// through setCardCollectionSlot/clearCardCollectionSlot, which keep key and
	// item consistent - key is the gameplay truth (O(1) hot-path reads), item
	// carries the physical card's InstanceID for stash reconciliation.
	cardSlots [MaxCardSlots]cardSlot
	// cardBurstTile is the last party tile the Gorilla Titan move-burst rolled on,
	// so the on-move proc fires once per tile entered, not every frame.
	cardBurstTileX, cardBurstTileY int

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
	// hudMessageLines cache: the wrapped HUD tail is requested at least twice
	// per frame (draw + click hit-region) but only changes when the log does.
	combatLogVersion int
	hudLinesCache    []combatLogEntry
	hudLinesCacheVer int
	hudLinesCacheOK  bool

	// Per-member, per-effect card-overlay timers (frames remaining): blink/scorch/
	// spark/heal. One table instead of four parallel arrays - see cardFx and
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
	// wallTopBuffer is the screen-Y of the nearest solid wall's TOP per column
	// (parallel to depthBuffer). Lets tall sprites (tree standees) render the
	// part that rises ABOVE a shorter wall instead of being culled whole-column.
	wallTopBuffer []int

	// Systems
	gameLoop        *GameLoop
	combat          *CombatSystem
	collisionSystem *collision.CollisionSystem
	questManager    *quests.QuestManager
	// questTileOriginals: pristine tile at every quest on_complete_tiles
	// position, captured once (maps always load pristine from disk) so
	// syncQuestTiles can REVERT a change when its quest isn't completed.
	questTileOriginals map[string]world.TileType3D

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

	// healPicker mirrors the revival picker: when a heal potion is used by an
	// UNCONSCIOUS owner (who can't heal themselves), the player chooses which
	// conscious, wounded member to heal instead.
	healPickerOpen    bool
	healPickerItemIdx int

	// pickerQuickChar/Slot record the quick slot a heal/revival picker was opened
	// FROM (char<0 = opened from the inventory). The slot stays filled while the
	// picker is up; on confirm it's cleared, on cancel the temp bag copy is dropped
	// and the slot kept - so cancelling never silently moves the potion to the bag.
	pickerQuickChar int
	pickerQuickSlot int

	// parkSelection is set when the player selects a member BY HAND (portrait
	// click / number key) and cleared on any auto-advance of the selection. While
	// set, the TB/RT auto-snap that moves selection off a member who can't act is
	// suppressed, so the player can park on a downed ally to use their (free,
	// passive) quick-slot potions. Zero value (false) is the safe default.
	parkSelection bool

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

	// Tavern stash screen: a cross-save shared chest (see internal/stash). The
	// chest is lazy-loaded on first open and persisted on every transfer.
	// Drag-and-drop state mirrors the quick-slot drag but is self-contained so it
	// works while no tab/menu is open. stashDragFrom < 0 when nothing is carried;
	// 0..SlotCount-1 = a stash cell, >= stashDragInvBase = an inventory index.
	stashScreenOpen bool
	stash           *stash.Stash
	loadNeedsResave bool // a load stamped legacy items with instance ids: re-save the slot once
	stashDragArmed  bool
	stashDragActive bool
	stashDragFrom   int        // -1 none; 0..7 stash cell; >=stashDragInvBase inventory
	stashDragItem   items.Item // carried copy, for rendering
	stashDragStartX int
	stashDragStartY int
	stashDragCurX   int
	stashDragCurY   int
	stashDragDrop   bool // a drop must be resolved this frame
	stashInvPage    int
	stashShowCards  bool // top storage tab: false = general chest, true = card vault

	// Turn-based mode state
	turnBasedMode         bool // Whether game is in turn-based mode
	currentTurn           int  // 0 = party turn, 1 = monster turn
	partyActionsUsed      int  // Actions used this turn (0-2)
	turnBasedMoveCooldown int  // Movement cooldown in frames (18 FPS = 0.3 second)
	turnBasedRotCooldown  int  // Rotation cooldown in frames (18 FPS = 0.3 second)
	monsterTurnResolved   bool // Whether monster turn already processed this round
	turnBasedSpRegenCount int  // Counter for turn-based SP regeneration (every 5 turns)
	// turnBasedExtraMonsterAction grants the next monster turn one extra action
	// pass when the party attacks/casts first and then retreats in the same TB
	// round. This closes infinite shoot-and-step-back kiting without forbidding
	// tactical retreats outright.
	turnBasedExtraMonsterAction bool
	turnBasedMonsterPassesLeft  int
	turnBasedMonsterPassDelay   int
	turnBasedMonsterStatusTick  bool
	turnBasedMonsterStunned     map[*monster.Monster3D]bool

	// cardSummonCDFrames silences the card-collection summon PROC after it
	// fires (card_summon_cd_seconds); it never gates the character's actions.
	cardSummonCDFrames int

	// Main menu (ESC)
	mainMenuOpen      bool
	mainMenuSelection int
	mainMenuMode      MainMenuMode
	slotSelection     int // row within the current save page (0..saveRowsPerPage-1)
	savePage          int // current save/load menu page (0..savePageCount-1)
	saveRenameOpen    bool
	saveRenameSlot    int
	saveRenameInput   string
	exitRequested     bool

	// Game over state
	gameOver bool

	// Victory state
	gameVictory           bool
	victoryAcknowledged   bool
	victoryTime           time.Time
	sessionStartTime      time.Time
	totalGoldEarned       int
	totalExperienceEarned int

	// High scores state
	showHighScores   bool
	arenaBoardScroll int // champions' board scroll offset (lines)
	// Board tab render cache: the leaderboard file is read and flattened to
	// display lines only when stale (victory recorded / dialog opened) or when
	// the Shift-detail mode flips - never per frame.
	arenaBoardLines   []arenaBoardLine
	arenaBoardDetail  bool
	arenaBoardWidth   int
	arenaBoardStale   bool
	victoryNameInput  string
	victoryScoreSaved bool

	// Top-level screen state (entry menu / party creation / gameplay). See
	// AppScreen. The entry menu and party-creation screens live in
	// screen_entry.go and screen_party_create.go.
	appScreen          AppScreen
	entryMenuMode      EntryMenuMode
	achievementsScroll int               // achievements list scroll offset (rows)
	partyCreate        *partyCreateState // built lazily on entering AppScreenPartyCreate
}

type GameState int

const (
	GameStateExploration GameState = iota
	GameStateTurnBased
)

// AppScreen is the top-level screen the game is showing. It gates the whole
// update/draw cycle: the gameplay loop only runs in AppScreenInGame; the entry
// menu and party-creation screens replace it entirely. Its zero value is the
// entry menu, so a fresh MMGame boots to the menu.
type AppScreen int

const (
	AppScreenMainMenu    AppScreen = iota // entry/title menu (Start, Load, Scores, Achievements)
	AppScreenPartyCreate                  // pick the starting party of 4
	AppScreenInGame                       // normal gameplay (the rest of this engine)
)

// EntryMenuMode is the sub-screen shown within AppScreenMainMenu.
type EntryMenuMode int

const (
	EntryMenuRoot EntryMenuMode = iota
	EntryMenuLoad
	EntryMenuScores
	EntryMenuAchievements
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
	TabCards
)

type FirstPersonCamera struct {
	X, Y     float64 // Position in world
	Angle    float64 // Viewing angle in radians
	FOV      float64 // Field of view
	ViewDist float64 // Maximum view distance
}

// ApplySpriteColorKey wires the config's load-time color key into a sprite
// manager: stray magenta (from imperfect sprite background removal) turns
// transparent; edge-only sprites despill just their rims. Shared by the game
// and the map editor so both render sprites identically.
func ApplySpriteColorKey(sprites *graphics.SpriteManager, cfg *config.Config) {
	ck := cfg.Graphics.ColorKey
	if !ck.Enabled {
		return
	}
	r, g, b := ck.Color[0], ck.Color[1], ck.Color[2]
	if r == 0 && g == 0 && b == 0 {
		r, g, b = 255, 0, 255 // default key = magenta
	}
	sprites.SetColorKey(true, r, g, b, ck.Tolerance, ck.Despill)
	sprites.SetDespillEdgeOnly(ck.EdgeOnlyDespill, ck.EdgeDespillRadius)
}

func NewMMGame(cfg *config.Config) *MMGame {
	sprites := graphics.NewSpriteManager()
	ApplySpriteColorKey(sprites, cfg)

	// Get world from WorldManager instead of creating new one
	currentWorld := world.GlobalWorldManager.GetCurrentWorld()
	if currentWorld == nil {
		panic("No world available from WorldManager")
	}
	// Content check that needs loaded MAPS (boot sees only configs): every
	// duel-offering NPC must stand on a map with a duel: block.
	if err := ValidateDuelGrounds(world.GlobalWorldManager); err != nil {
		panic(err)
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
		pickerQuickChar:  -1,
		pickerQuickSlot:  -1,
		skyImg:           skyImg,
		groundImg:        groundImg,
		magicProjectiles: make([]MagicProjectile, 0),
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
		depthBuffer:   make([]float64, cfg.GetScreenWidth()),
		wallTopBuffer: make([]int, cfg.GetScreenWidth()),

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

	game.collisionSystem.RegisterEntity(newPlayerCollisionEntity(startX, startY))

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

	// Fail fast on buff_fx_sprite / slash_fx / projectile_fx typos (sprite
	// index is ready by now).
	game.validateBuffFxSprites()
	validateWeaponFxStyles()
	validateProjectileFxStyles()

	// Update sky and ground colors for initial map
	game.UpdateSkyAndGroundColors()

	game.registerVisitedTownPortalDestination() // the starting map may be a Town Portal destination

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
	entityType, solid := desiredMonsterCollisionState(m)
	entity := collision.NewEntity(m.ID, m.X, m.Y, width, height, entityType, solid)
	g.collisionSystem.RegisterEntity(entity)
}

// newPlayerCollisionEntity keeps the party's collision contract consistent
// across a new game, map arrival, and save load. The player blocks hostile
// monster pathfinding, while a monster's own solidity still controls whether
// the party can walk through it.
func newPlayerCollisionEntity(x, y float64) *collision.Entity {
	return collision.NewEntity("player", x, y, 16, 16, collision.CollisionTypePlayer, true)
}

// partyInCombat reports whether a live engaging monster is NEAR the party
// (TB vision range). While true, Space keeps its combat meaning and never
// opens dialog. Distance-gated on purpose: a whole-map-aggro boss or an
// AoE-clipped stray must not lock interaction (e.g. the map-exit NPC) from
// across the map.
func (g *MMGame) partyInCombat() bool {
	if g.world == nil {
		return false
	}
	radius := TurnBasedVisionRangeTiles * float64(g.config.GetTileSize())
	for _, m := range g.world.Monsters {
		if m != nil && m.IsAlive() && m.IsEngagingPlayer &&
			Distance(g.camera.X, g.camera.Y, m.X, m.Y) <= radius {
			return true
		}
	}
	return false
}

// npcEffectivePos returns where the NPC is actually rendered: wall-mounted
// standees (gates/grates) slide flush onto the nearest wall face, everyone
// else stays at their tile. Interaction focus and hit-tests MUST use this,
// not the raw tile position, or they miss the visible sprite.
func (g *MMGame) npcEffectivePos(npc *character.NPC) (float64, float64) {
	if g.npcIsWall(npc) {
		if wx, wy, _, ok := g.wallStickPose(npc.X, npc.Y); ok {
			return wx, wy
		}
	}
	return npc.X, npc.Y
}

// npcIsWall is the single test for the wall-mounted render class (the
// wall_mounted render category), and only while standees are on. Every wall
// render/position/hit-test path uses it.
func (g *MMGame) npcIsWall(npc *character.NPC) bool {
	return g.config.Graphics.Standee.Enabled && npcRenderCatOf(npc) == catWall
}

// updateFocusedNPC recomputes the Space-to-interact target once per tick:
// the nearest NPC the party stands next to (within ~1 adjacent tile, diagonals
// included) that is roughly centred on screen. Cached so input and the HUD
// hint agree, and so the hint never reads a screen-shake-displaced camera.
// Nothing takes focus mid-combat - Space stays an attack while enemies engage.
func (g *MMGame) updateFocusedNPC() {
	g.focusedNPC = nil
	currentWorld := g.GetCurrentWorld()
	if currentWorld == nil || g.renderHelper == nil || g.partyInCombat() {
		return
	}
	maxDist := float64(g.config.GetTileSize()) * 1.6 // own + adjacent tile, diagonal-safe
	halfW := float64(g.config.GetScreenWidth()) / 2
	band := halfW * 0.4 // "roughly centred": middle 40% of the screen
	bestDist := maxDist
	for _, npc := range currentWorld.NPCs {
		if npc.HideWhenVisited && npc.Visited {
			continue
		}
		if g.npcDoorOpen(npc) { // raised portcullis: nothing to interact with
			continue
		}
		ex, ey := g.npcEffectivePos(npc)
		dist := Distance(g.camera.X, g.camera.Y, ex, ey)
		if dist > bestDist {
			continue
		}
		screenX, _, ok := g.renderHelper.projectToScreenX(ex, ey)
		if !ok || math.Abs(float64(screenX)-halfW) > band {
			continue
		}
		g.focusedNPC = npc
		bestDist = dist
	}
}

// findNPCAtScreen returns the visible NPC whose rendered sprite is under the
// given screen point (nearest wins). inRange reports whether it is close
// enough to interact (InteractionDistance) - a hit beyond that only prompts.
func (g *MMGame) findNPCAtScreen(clickX, clickY int) (npc *character.NPC, inRange bool) {
	currentWorld := g.GetCurrentWorld()
	if currentWorld == nil || g.renderHelper == nil {
		return nil, false
	}
	bestDist := math.MaxFloat64
	for _, n := range currentWorld.NPCs {
		if n.HideWhenVisited && n.Visited {
			continue
		}
		if n.Sprite == "" || n.Sprite == "none" {
			continue // invisible portal NPCs aren't clickable
		}
		if g.npcDoorOpen(n) { // raised portcullis: not drawn, not clickable
			continue
		}
		ex, ey := g.npcEffectivePos(n)
		dist := Distance(g.camera.X, g.camera.Y, ex, ey)
		if dist >= bestDist {
			continue
		}
		if !g.npcScreenHitTest(n, ex, ey, dist, clickX, clickY) {
			continue
		}
		npc = n
		bestDist = dist
	}
	return npc, npc != nil && bestDist <= InteractionDistance
}

// npcScreenHitTest projects the NPC with the same metrics the renderer uses
// and tests the point against its billboard rect (inset horizontally so the
// mostly-transparent sprite margins don't catch clicks). ex/ey is the NPC's
// effective (rendered) position - see npcEffectivePos. Occlusion is checked
// against the wall depth buffer at the sprite's centre column.
func (g *MMGame) npcScreenHitTest(npc *character.NPC, ex, ey, distance float64, x, y int) bool {
	screenX, screenY, spriteSize, visible := g.renderHelper.NPCSpriteMetrics(npc, ex, ey, distance)
	if !visible || spriteSize <= 0 {
		return false
	}
	if screenX >= 0 && screenX < len(g.depthBuffer) {
		if _, depth, ok := g.renderHelper.projectToScreenX(ex, ey); ok {
			// Wall-mounted tokens sit flush ON the wall plane, so their depth ties
			// with the backing wall's and the comparison flips with tiny angle
			// changes. Bias toward the camera exactly like the renderer's
			// standeeDepthBias does when drawing them.
			if g.npcIsWall(npc) {
				depth -= float64(g.config.GetTileSize()) * 0.6
			}
			if depth >= g.depthBuffer[screenX] {
				return false // behind a wall
			}
		}
	}
	drawLeft := screenX - spriteSize/2
	inset := spriteSize / 5
	return x >= drawLeft+inset && x < drawLeft+spriteSize-inset && y >= screenY && y < screenY+spriteSize
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
	// Map switches swap the sky instantly (no crossfade) and re-derive whether
	// the day/night light cycle applies here (outdoor = phase variants exist).
	g.dayNightOutdoor = skyHasDayNightVariants(skyTexture)
	g.cancelSkyFade()
	g.updateSkyPanorama(g.skyTextureForPhase(skyTexture))

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

const (
	// A turn-based 90deg turn eases over this long (then lands exactly). This is
	// intentionally long enough to read the diagonal sector while turning, so a
	// monster can't be skipped just because the camera snaps between cardinals.
	turnViewSeconds = 0.25
	// Horizontal motion-blur length as a FRACTION of the per-frame view pan. The
	// raw per-frame pan during a fast turn is huge, so a small fraction reads as a
	// SLIGHT smear (full would be mush). Kept subtle because TB turning is a
	// look-around aid: the diagonal sector must stay readable while moving.
	turnBlurStrength = 0.06
	// Upper bound on the blur half-length in pixels - never smear into mush.
	turnBlurMaxPixels = 120.0
)

// turnBlurPixels is the horizontal motion-blur half-length (pixels) for this
// frame: ~the distance the view pans per frame during the turn, scaled down to a
// slight smear and capped. 0 outside a turn.
// screenWidth is the ACTUAL draw-target width (the scene buffer), not the config
// width - they can differ, and the pan-to-pixels mapping must match what's drawn.
func (g *MMGame) turnBlurPixels(screenWidth int) float64 {
	if g.viewTurnFramesLeft <= 0 || g.camera == nil || g.camera.FOV <= 0 || turnBlurStrength <= 0 {
		return 0
	}
	stepRad := (math.Pi / 2) / float64(g.turnViewFrames()) // per-frame yaw step
	panPx := stepRad / g.camera.FOV * float64(screenWidth)
	if blur := panPx * turnBlurStrength; blur < turnBlurMaxPixels {
		return blur
	}
	return turnBlurMaxPixels
}

// ensureBlurShader lazily compiles the horizontal motion-blur shader.
func (g *MMGame) ensureBlurShader() (*ebiten.Shader, error) {
	if g.blurShader != nil {
		return g.blurShader, nil
	}
	s, err := ebiten.NewShader([]byte(turnBlurShaderSrc))
	if err != nil {
		return nil, err
	}
	g.blurShader = s
	return s, nil
}

func (g *MMGame) ensureTurnSceneBuffer(bounds image.Rectangle) *ebiten.Image {
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return nil
	}
	if g.sceneBuf == nil || g.sceneBuf.Bounds() != bounds {
		g.sceneBuf = ebiten.NewImage(bounds.Dx(), bounds.Dy())
	}
	return g.sceneBuf
}

// turnViewFrames is how many frames a 90deg TB turn eases over at the current TPS.
func (g *MMGame) turnViewFrames() int {
	if n := int(turnViewSeconds * float64(g.config.GetTPS())); n > 1 {
		return n
	}
	return 1
}

// snapFacing is the single point for instant heading changes (loads, map
// arrivals, new-game resets, TB cardinal snaps): it moves the logical angle AND
// the rendered view together and cancels any in-flight turn glide, so the view
// can never ease from a stale heading. Gliding turns go through rotateTurnBased.
func (g *MMGame) snapFacing(angle float64) {
	g.camera.Angle = angle
	g.viewAngleRender = angle
	g.viewTurnFramesLeft = 0
}

// beginViewAngleSwap points the camera at the eased display angle for a draw
// pass and returns the restore for the logical angle. The restore only undoes
// OUR swap: a Draw-time handler (the entry menu loads saves from its draw pass)
// may re-aim the camera mid-Draw, and that write must survive the frame.
func (g *MMGame) beginViewAngleSwap() (restore func()) {
	logicalAngle := g.camera.Angle
	displayAngle := g.viewAngleRender
	g.camera.Angle = displayAngle
	return func() {
		if g.camera.Angle == displayAngle {
			g.camera.Angle = logicalAngle
		}
	}
}

// advanceViewTurn eases the rendered view angle toward the logical camera angle.
// During a TB turn (viewTurnFramesLeft > 0) it moves at most one step per frame so
// the scene glides; otherwise it tracks the angle exactly - real-time rotation,
// teleports and loads must snap, never animate. Call once per frame.
func (g *MMGame) advanceViewTurn() {
	if g.camera == nil {
		return
	}
	if g.viewTurnFramesLeft > 0 {
		step := (math.Pi / 2) / float64(g.turnViewFrames())
		g.viewAngleRender = approachAngle(g.viewAngleRender, g.camera.Angle, step)
		g.viewTurnFramesLeft--
		if g.viewTurnFramesLeft == 0 {
			g.viewAngleRender = g.camera.Angle // land exactly on the target
		}
	} else {
		g.viewAngleRender = g.camera.Angle
	}
}

func (g *MMGame) Draw(screen *ebiten.Image) {
	// Render at the eased view angle so a turn-based turn glides. Logic keeps the
	// snapped camera.Angle (set in Update); restore it right after Draw so nothing
	// observes the display angle. In real time viewAngleRender == camera.Angle, so
	// this is a no-op.
	if g.camera != nil {
		defer g.beginViewAngleSwap()()
	}

	// Screen shake: nudge the camera sideways (perpendicular to the view) for
	// this frame only - the whole raycast scene shifts coherently, and the
	// camera is restored before any game logic can observe it.
	if g.screenShake > 0 && g.camera != nil {
		ox := -math.Sin(g.camera.Angle) * g.screenShake
		oy := math.Cos(g.camera.Angle) * g.screenShake
		if g.frameCount%2 == 0 {
			ox, oy = -ox, -oy
		}
		g.camera.X += ox
		g.camera.Y += oy
		// Record the displacement so render-time geometry that must IGNORE the
		// cosmetic shake (the TB front-diagonal pull - see pulledFrontSlot) can
		// recover the logical camera. Otherwise the per-frame +/- jitter flips the
		// pull's LOS near walls and the pulled monster blinks when struck.
		g.screenShakeOffsetX, g.screenShakeOffsetY = ox, oy
		// Same only-undo-our-own-write rule as the angle swap above.
		defer func(shakenX, shakenY, x, y float64) {
			if g.camera.X == shakenX && g.camera.Y == shakenY {
				g.camera.X, g.camera.Y = x, y
			}
			g.screenShakeOffsetX, g.screenShakeOffsetY = 0, 0
		}(g.camera.X, g.camera.Y, g.camera.X-ox, g.camera.Y-oy)
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
	g.wallTopBuffer = make([]int, screenWidth)
	g.skyImg = ebiten.NewImage(screenWidth, screenHeight/2)
	g.groundImg = ebiten.NewImage(screenWidth, screenHeight/2)
	g.UpdateSkyAndGroundColors()

	if g.gameLoop != nil && g.gameLoop.renderer != nil {
		g.gameLoop.renderer.handleResize(screenWidth, screenHeight)
	}
}

// Shutdown releases threading resources. Safe to call multiple times only via
// the threading components' own idempotency - call once on game exit.
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
	if g.gameVictory || g.victoryAcknowledged || g.gameOver {
		return
	}
	if g.questManager == nil {
		return
	}
	quest := g.questManager.GetQuest("dragon_slayer")
	if quest != nil && quest.Status == quests.QuestStatusCompleted {
		if !quest.RewardsClaimed {
			g.claimQuestReward("dragon_slayer")
		}
		g.enterPostVictoryFreeMode()
		g.gameVictory = true
		g.victoryTime = time.Now()
	}
}

func (g *MMGame) enterPostVictoryFreeMode() {
	g.turnBasedMode = false
	g.currentTurn = 0
	g.partyActionsUsed = 0
	g.turnBasedMoveCooldown = 0
	g.turnBasedRotCooldown = 0
	g.monsterTurnResolved = false
	g.turnBasedExtraMonsterAction = false
	g.turnBasedMonsterPassesLeft = 0
	g.turnBasedMonsterPassDelay = 0
	g.turnBasedMonsterStatusTick = false
	g.turnBasedMonsterStunned = nil
	g.clearTransientCombatState()
}

// AddCombatMessage adds a combat message to the message queue
func (g *MMGame) AddCombatMessage(message string) {
	g.AddColoredCombatMessage(message, color.White)
}

// AddColoredCombatMessage appends a combat-log entry with an explicit display
// color. The HUD slice is derived on demand (GetCombatMessages), so text and
// color can never fall out of sync.
func (g *MMGame) AddColoredCombatMessage(message string, messageColor color.Color) {
	g.combatLogVersion++
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
	m.QuestProgressIgnored = true // Dead Branch / ad-hoc summons are not map quest targets.
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

const (
	// hudMessageWidth is the on-screen width (px) of the inline HUD message block.
	hudMessageWidth = 400
	// maxHudMessageLines caps how many wrapped lines the HUD shows (most recent
	// kept), so a burst of long messages can't grow the block up across the screen.
	maxHudMessageLines = 8
	// hudMessageBottomGap is the gap (px) between the block's bottom and the party
	// portraits: the block is bottom-anchored here and grows upward.
	hudMessageBottomGap = 4
)

// hudMessageLines wraps the HUD combat-log tail to the message-block width,
// carrying each entry's color onto every line it wraps to, and keeps only the
// most recent maxHudMessageLines lines. Shared by the HUD renderer and its click
// hit-region so the drawn block and the clickable area stay the same height.
func (g *MMGame) hudMessageLines() []combatLogEntry {
	if g.hudLinesCacheOK && g.hudLinesCacheVer == g.combatLogVersion {
		return g.hudLinesCache
	}
	maxChars := (hudMessageWidth - 10) / debugTextCharWidth
	var lines []combatLogEntry
	for _, e := range g.hudLog() {
		for _, l := range wrapText(e.Text, maxChars) {
			lines = append(lines, combatLogEntry{Text: l, Color: e.Color})
		}
	}
	if len(lines) > maxHudMessageLines {
		lines = lines[len(lines)-maxHudMessageLines:]
	}
	g.hudLinesCache = lines
	g.hudLinesCacheVer = g.combatLogVersion
	g.hudLinesCacheOK = true
	return lines
}

// hudMessageBlockRect returns the screen rect of the HUD combat-log block for the
// given wrapped-line count: bottom-anchored just above the party portraits and
// growing upward so a tall block never spills down over the party UI. Shared by
// the renderer and the click hit-region.
func (g *MMGame) hudMessageBlockRect(lineCount int) (x, y, w, h int) {
	h = lineCount*hudMessageSpacing + 10
	w = hudMessageWidth
	x = g.config.GetScreenWidth() - w - 15
	y = g.config.GetScreenHeight() - g.config.UI.PartyPortraitHeight - hudMessageBottomGap - h
	return
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
// card - fired wherever a member takes a visible hit, so impacts read clearly.
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

// refreshBoundAllyCache rebuilds the per-frame list of bound undead (bind_undead)
// so the AI-target lookup can let normal mobs retaliate against them without an
// O(n^2) scan when none exist. Called once per frame before the monster update.
func (g *MMGame) refreshBoundAllyCache() {
	g.boundAllies = g.boundAllies[:0]
	for _, m := range g.world.Monsters {
		if m != nil && m.Bound && m.IsAlive() {
			g.boundAllies = append(g.boundAllies, m)
		}
	}
	// Precompute each monster's foe + pursuit target ONCE per frame, single-threaded,
	// so the parallel real-time update never scans other monsters' positions (which
	// are being mutated concurrently) and no consumer recomputes the foe. The wrapper
	// reads AITargetX/Y; combat reads AIFoe.
	if g.combat == nil {
		return
	}
	// Count the live ward idols on this map once. While >=1 stands, every
	// WardedByIdols boss here is invulnerable and rooted (it holds its plaza);
	// enrage stays HP-driven and only applies after the ward drops. The link is
	// type-scoped (idols aren't bound to a specific boss) - fine while a map has a
	// single warded boss, which is all the content has. Recomputed per frame => no
	// save state, self-heals on reload.
	liveIdols := 0
	for _, m := range g.world.Monsters {
		if m != nil && m.WarlordIdol && m.IsAlive() {
			liveIdols++
		}
	}
	for _, m := range g.world.Monsters {
		if m == nil {
			continue
		}
		// Champion mobs mirror their character build's combat stats. Runs before
		// the monster update (both RT and TB) so damage/cadence/HP are ready by
		// the mob's first action, and covers every spawn source and save-load.
		if m.IsChampion() {
			g.mirrorChampionStats(m)
		}
		// Idol-warded boss: invulnerable AND HOLDS its plaza (its idols' power roots
		// it in place) until every idol is broken - then it activates as a normal
		// aggressive boss. Computed first so it can gate BossAggro/freeze below.
		// Without this hold an aggressive boss beelines across the whole map to the
		// party at the landing the instant the map loads.
		m.BossWarded = m.WardedByIdols && liveIdols > 0
		// Sealed boss (passive-until-quest, no evade radius) -> freeze on its spawn
		// until the quest unseals it. An evasive boss WITH an evade radius still
		// skitters and blinks, so it is excluded.
		m.BossDormant = g.combat.isBoss(m) && g.combat.bossEvasive(m) && m.EvadeRadiusTiles == 0
		if m.IsInertSetPiece() {
			// Do not hand a scripted inactive actor a crossfire foe. The RT combat
			// loop is separate from movement AI, so leaving this populated lets it
			// bypass the movement freeze and strike a nearby bound ally.
			m.AIFoe = nil
			m.AITargetX, m.AITargetY = m.X, m.Y
			m.BossAggro = false
			continue
		}
		m.AIFoe = g.combat.monsterAIFoeMonster(m)
		m.AITargetX, m.AITargetY = g.combat.monsterAITargetPoint(m)
		// Relentless chase (ignores detection range). Most bosses go relentless only
		// AFTER normal aggro - within their (larger) alert radius or once the party
		// has hit them (WasAttacked is sticky) - so they don't beeline across the
		// whole map the instant they activate. AggroWholeMap is the UNIQUE opt-in
		// (Golden Thief Bug) that DOES chase from anywhere on activation.
		m.BossAggro = g.combat.isBoss(m) && !g.combat.bossEvasive(m) && !m.BossWarded &&
			(m.AggroWholeMap || m.IsEngagingPlayer || m.WasAttacked)
	}
	g.ejectPartyTargetingMonsters()
}

// ejectPartyTargetingMonsters resolves the exceptional case where a monster
// that has selected the party is already overlapping the player (for example,
// it had been walkable while chasing a summon and its target changed). Hostile
// mobs must fight from an adjacent tile, never render on top of the party.
func (g *MMGame) ejectPartyTargetingMonsters() {
	if g == nil || g.world == nil || g.collisionSystem == nil || g.camera == nil || g.config == nil {
		return
	}
	player := g.collisionSystem.GetEntityByID("player")
	if player == nil || player.BoundingBox == nil {
		return
	}
	tileSize := float64(g.config.GetTileSize())
	ptx, pty := int(g.camera.X/tileSize), int(g.camera.Y/tileSize)
	// The cardinal tiles are preferred to keep an attacker in a readable melee
	// ring; diagonals provide a fallback around walls and other combatants.
	offsets := [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}, {-1, -1}, {1, -1}, {-1, 1}, {1, 1}}
	for _, m := range g.world.Monsters {
		if m == nil || !m.IsAlive() || !monsterTargetsParty(m) {
			continue
		}
		entity := g.collisionSystem.GetEntityByID(m.ID)
		if entity == nil || entity.BoundingBox == nil || !entity.BoundingBox.Intersects(player.BoundingBox) {
			continue
		}
		mw, mh := m.GetSize()
		for _, off := range offsets {
			x, y := TileCenterFromTile(ptx+off[0], pty+off[1], tileSize)
			candidate := collision.NewBoundingBox(x, y, mw, mh)
			if candidate.Intersects(player.BoundingBox) ||
				!g.collisionSystem.CanMoveToWithHabitat(m.ID, x, y, m.HabitatPrefs, m.Flying) {
				continue
			}
			m.X, m.Y = x, y
			m.ResetPathfinding()
			g.collisionSystem.UpdateEntity(m.ID, x, y)
			break
		}
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

// canSelectChar reports whether the party member can spend a turn-based action
// right now: alive + conscious and at least one action slot left. Manual UI
// selection is looser; exhausted living members can still be selected for stats
// and inventory.
func (g *MMGame) canSelectChar(idx int) bool {
	if idx < 0 || idx >= len(g.party.Members) {
		return false
	}
	m := g.party.Members[idx]
	return m.CanAct() && m.ActionsRemaining > 0
}

// partyAllExhausted reports whether every still-able-to-act party member has
// spent all their action slots this round. KO members are skipped - the
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
	g.parkSelection = false // auto-advance clears any manual park
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
// combat: alive, conscious, and off cooldown for SOME action. That's a ready
// weapon hand (covers a dual-wielder whose off-hand is free while the main is
// still cycling) OR the main cooldown being clear (covers a weaponless caster
// whose only move is a spell/heal - gated by RTCooldown, not a weapon). The
// real-time analogue of canSelectChar (which is turn-based, gated by action
// slots).
func (g *MMGame) rtCharReady(idx int) bool {
	if idx < 0 || idx >= len(g.party.Members) {
		return false
	}
	m := g.party.Members[idx]
	return m.CanAct() && (m.AnyWeaponHandReady() || m.RTCooldown <= 0)
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
	if m == nil || !m.CanAct() || m.IsStunned() {
		return false
	}
	switch kind {
	case rtActWeapon:
		return m.HasWeaponInEitherHand()
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

// rtActionReady = capable of the action AND off cooldown. Weapon actions accept
// either ready hand for a Dual Wielding character. Smart actions also accept an
// off-hand-only-ready dual-wielder, but input.go then forces that path to a
// weapon fallback only; heals/spells/traps remain gated by RTCooldown.
func (g *MMGame) rtActionReady(idx int, kind rtActionKind) bool {
	if !g.rtActionCapable(idx, kind) {
		return false
	}
	m := g.party.Members[idx]
	if kind == rtActWeapon {
		return m.AnyWeaponHandReady()
	}
	if kind == rtActSmart && m.RTCooldown > 0 && m.AnyWeaponHandReady() {
		return true
	}
	return m.RTCooldown <= 0
}

// nextReadyRTActor returns the next member (after the selection, wrapping) who is
// ready to do `kind`, or -1 if none. Unlike advanceRTActor it never falls back to
// an on-cooldown member - so callers can WAIT in place instead of churning the
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
	g.parkSelection = false // auto-advance clears any manual park
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
// act once their cooldown ends) - without this the party freezes on a corpse.
func (g *MMGame) ensureSelectedCanActRT() {
	if g.rtCharReady(g.selectedChar) {
		return
	}
	if g.parkedOnSelected() {
		return // player deliberately parked here (e.g. to use a downed ally's potions)
	}
	cur := g.party.Members[g.selectedChar]
	if cur != nil && cur.CanAct() {
		return // alive, just on cooldown - leave selection put
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
	// Otherwise any living member (still on cooldown - they'll fire when ready).
	for off := 1; off <= n; off++ {
		idx := (g.selectedChar + off) % n
		if m := g.party.Members[idx]; m != nil && m.CanAct() {
			g.selectedChar = idx
			return
		}
	}
}

// startPartyTurn resets ActionsRemaining for every able-bodied party member,
// then grants a small party-wide pool of Speed bonus actions to the fastest
// living members (tie-break: lower party slot). Called when entering
// turn-based mode and at the end of each monster turn. KO members get 0 slots.
func (g *MMGame) startPartyTurn() {
	g.parkSelection = false // a new round clears any manual park
	tps := g.config.GetTPS()
	for _, m := range g.party.Members {
		// Poison/ignite tick once per party turn in TB (mirrors monster
		// TickPoisonTurn) - ticks regardless of stun, same as the RT per-frame
		// updatePoison/updateBurn did before TB switched off real-time ticking.
		m.TickPoisonTurn(tps)
		m.TickBurnTurn(tps)
	}
	// Run the lethal-DoT sweep (Lich Card save / Unconscious) BEFORE handing out
	// action slots below - otherwise a member the tick just ticked to 0 HP reads
	// as unable to act this round even when the Lich Card would have saved them,
	// and the KO message/condition lag a full frame behind the tick that caused
	// it (the per-frame sweep in the main loop runs before this point, not after).
	if g.combat != nil {
		g.combat.knockOutLethalDoTVictims()
	}
	for _, m := range g.party.Members {
		m.NextTBAttackOffHand = false // fresh round: next swing starts on the main hand
		if m.IsStunned() {
			m.TickStunTurn() // consume one stunned turn
			m.ActionsRemaining = 0
		} else if m.CanAct() {
			// Dual Wielding grants a PERSONAL extra action (two weapons, two
			// swings) - separate from the party-wide Speed bonus pool assigned
			// below, so a fast dual-wielder can get both.
			if m.IsDualWielding() {
				m.ActionsRemaining = 2
			} else {
				m.ActionsRemaining = 1
			}
		} else {
			m.ActionsRemaining = 0
		}
	}
	g.assignTurnBasedSpeedBonusActions()
	if idx := g.firstEligiblePartyIndex(); idx >= 0 {
		g.selectedChar = idx
	}
	// Pull any stacked mobs onto distinct tiles before the player aims - TB fires
	// along rows/columns, so a stacked/half-offset pair would let a shot thread
	// between them. Once per turn = jitter-free (unlike a per-frame RT snap).
	if g.turnBasedMode {
		g.separateStackedMonstersTB()
	}
}

func (g *MMGame) assignTurnBasedSpeedBonusActions() {
	bonusActions := 0
	for _, m := range g.party.Members {
		if m != nil && m.CanAct() && !m.IsStunned() {
			if tier := m.SpeedBonusActionTier(); tier > bonusActions {
				bonusActions = tier
			}
		}
	}
	// Monster cards (e.g. the Puma Card) add to the party's bonus-action pool,
	// distributed to the fastest members alongside Speed bonuses.
	bonusActions += g.cardBonusActions()
	for bonusActions > 0 {
		bestIdx := -1
		bestSpeed := -1
		for i, m := range g.party.Members {
			if m == nil || !m.CanAct() || m.IsStunned() || m.ActionsRemaining > 1 {
				continue
			}
			speed := m.GetEffectiveSpeed()
			if speed > bestSpeed {
				bestIdx = i
				bestSpeed = speed
			}
		}
		if bestIdx < 0 {
			return
		}
		g.party.Members[bestIdx].ActionsRemaining++
		bonusActions--
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
			member.ApplyCardRegenTick() // Troll Card(s): RT ticks on a frame timer, TB on this round counter
		}
	}

	g.currentTurn = 1 // Monster turn
	g.monsterTurnResolved = false
}

// endPartyTurnAfterMovement spends every remaining party action and ends the
// turn. Moving after at least one attack/cast grants monsters an extra action
// pass as anti-kiting pressure; opening the round with movement remains normal.
func (g *MMGame) endPartyTurnAfterMovement() {
	if g.partyActionsUsed > 0 {
		g.turnBasedExtraMonsterAction = true
	}
	for _, m := range g.party.Members {
		m.ActionsRemaining = 0
	}
	g.endPartyTurn()
}

// ensureSelectedCharCanAct auto-advances selectedChar to the next eligible
// member when the current one can no longer act. Necessary because a party
// member can become KO mid-party-turn from sources outside the action loop:
// in-flight projectiles fired during the previous monster turn that connect
// during the party turn, poison ticks, etc. Without this guard pressing
// Space/F on a dead selectedChar would silently do nothing.
// Real-time mode no-ops - selection isn't gated on CanAct there.
// parkedOnSelected reports whether the player manually selected the current
// member (portrait/number key) and that member is still a valid park target
// (present, not Eradicated). The auto-snap that moves selection off a member
// who can't act respects this, so the player can sit on a downed ally to use
// their free, passive quick-slot potions instead of being bounced away.
func (g *MMGame) parkedOnSelected() bool {
	if !g.parkSelection {
		return false
	}
	if g.selectedChar < 0 || g.selectedChar >= len(g.party.Members) {
		return false
	}
	m := g.party.Members[g.selectedChar]
	return m != nil && !m.HasCondition(character.ConditionEradicated)
}

func (g *MMGame) ensureSelectedCharCanAct() {
	if !g.turnBasedMode {
		return
	}
	if g.parkedOnSelected() {
		return // player deliberately parked here (e.g. to use a downed ally's potions)
	}
	if g.selectedChar >= 0 && g.selectedChar < len(g.party.Members) {
		if m := g.party.Members[g.selectedChar]; m != nil && m.CanAct() {
			return
		}
	}
	if g.canSelectChar(g.selectedChar) {
		return
	}
	if idx := g.firstEligiblePartyIndex(); idx >= 0 {
		g.selectedChar = idx
	}
}

// consumeSelectedCharAction spends one action slot on the currently selected
// character (turn-based mode only - no-op otherwise). If they ran out,
// auto-advances to the next eligible character. If nobody is left with
// actions, ends the party turn so monsters can move.
func (g *MMGame) consumeSelectedCharAction() {
	if !g.turnBasedMode {
		return
	}
	selected := g.party.Members[g.selectedChar]
	if selected.ActionsRemaining > 0 {
		selected.ActionsRemaining--
		g.partyActionsUsed++
	}
	if selected.ActionsRemaining == 0 {
		if g.partyAllExhausted() {
			g.endPartyTurn()
			return
		}
		g.advanceToNextEligibleChar()
	}
}

// consumeSelectedCharWeaponAction is consumeSelectedCharAction specialized for
// a weapon swing: flips NextTBAttackOffHand on the acting character BEFORE
// spending the slot (which may advance g.selectedChar to someone else), so a
// Dual Wielding character's next swing - this turn (a Speed bonus action) or
// next - hits with the OTHER weapon. A no-op flip for anyone without a weapon
// in the off-hand, since attackSlotFor never reads the flag for them.
func (g *MMGame) consumeSelectedCharWeaponAction() {
	if idx := g.selectedChar; idx >= 0 && idx < len(g.party.Members) {
		if m := g.party.Members[idx]; m != nil {
			m.NextTBAttackOffHand = !m.NextTBAttackOffHand
		}
	}
	g.consumeSelectedCharAction()
}

func (g *MMGame) ToggleTurnBasedMode() {
	g.turnBasedMode = !g.turnBasedMode

	// RT cooldowns only tick in real-time, so clear them on every switch - otherwise
	// one frozen across a turn-based fight gates RT actions afterwards. Each mode
	// starts ready.
	for _, m := range g.party.Members {
		if m != nil {
			m.RTCooldown = 0
			m.OffHandRTCooldown = 0
			m.NextTBAttackOffHand = false
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
		g.turnBasedExtraMonsterAction = false
		g.turnBasedMonsterPassesLeft = 0
		g.turnBasedMonsterPassDelay = 0
		g.turnBasedMonsterStatusTick = false
		g.turnBasedMonsterStunned = nil
		g.startPartyTurn()
		g.AddCombatMessage("Turn-based mode activated!")
	} else {
		g.turnBasedExtraMonsterAction = false
		g.turnBasedMonsterPassesLeft = 0
		g.turnBasedMonsterPassDelay = 0
		g.turnBasedMonsterStatusTick = false
		g.turnBasedMonsterStunned = nil
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
	// Normalize current angle to 0-2pi range
	angle := g.camera.Angle
	for angle < 0 {
		angle += 2 * math.Pi
	}
	for angle >= 2*math.Pi {
		angle -= 2 * math.Pi
	}

	// Cardinal directions in radians:
	// East (right): 0
	// North (up): 3pi/2 (or -pi/2)
	// West (left): pi
	// South (down): pi/2

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

	g.snapFacing(closestDirection)
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
	collisionSystem *collision.CollisionSystem   // LIVE system - touched only by ApplyCollisionUpdate (Phase 2, serial)
	snapshot        *collision.CollisionSnapshot // frozen view for THIS tick - the only thing Update (Phase 1, parallel) may query
	game            *MMGame                      // Added to access camera position for tethering system

	pendingCollisionType collision.CollisionType // computed in Update(), written to the live system in ApplyCollisionUpdate()
	pendingSolid         bool
}

// Update is the canonical RT monster tick: AI movement + the desired
// collision-solidity decision - COMPUTED ONLY here, against the frozen
// snapshot; nothing shared is written. Code that steps monsters manually
// (including tests) must call this AND ApplyCollisionUpdate, not the bare
// Monster3D.Update - that alone leaves the collision type stale.
//
// debugMonsterFilter caches the DEBUG_MONSTER env filter once at startup: the
// parallel monster update must not pay an env lookup + alloc per monster per tick.
var debugMonsterFilter = strings.ToLower(strings.TrimSpace(os.Getenv("DEBUG_MONSTER")))

// Runs in a worker goroutine (see entities.EntityUpdater.UpdateMonstersParallel):
// every read here must come from mw.Monster's own fields or mw.snapshot (frozen
// before the parallel phase started), never mw.collisionSystem - the live
// system is being written by every OTHER monster's own worker at the same time.
func (mw *MonsterWrapper) Update() {
	oldX, oldY := mw.Monster.X, mw.Monster.Y

	// Get player position from camera for tethering system
	playerX := mw.game.camera.X
	playerY := mw.game.camera.Y

	// AI pursuit/engagement target: normally the party, but charmed monsters are
	// redirected (a bound undead seeks its enemy; a pacified charm holds position)
	// so they never chase the party. Precomputed single-threaded each frame in
	// refreshBoundAllyCache to keep this parallel update race-free.
	targetX, targetY := mw.Monster.AITargetX, mw.Monster.AITargetY

	// Use collision-aware update with the chosen AI target for tethering. Reads
	// the frozen snapshot only - never the live, concurrently-mutating system.
	mw.Monster.Update(mw.snapshot, targetX, targetY)

	newX, newY := mw.Monster.X, mw.Monster.Y

	// Compute (don't apply) the desired collision type - a pure function of this
	// monster's own state, safe from a worker. ApplyCollisionUpdate writes it.
	mw.pendingCollisionType, mw.pendingSolid = desiredMonsterCollisionState(mw.Monster)

	// Temporary movement debug (opt-in via env var).
	// Example: DEBUG_MONSTER=bandit
	if debugMonsterFilter != "" {
		name := strings.ToLower(mw.Monster.Name)
		if strings.Contains(name, debugMonsterFilter) {
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

	// The collision-system write for the new position happens in
	// ApplyCollisionUpdate (Phase 2) - oldX/oldY above were only for the debug
	// block.
}

// ApplyCollisionUpdate writes this monster's Update()-computed position and
// collision type to the LIVE collision system. Phase 2 of the two-phase RT
// tick (see entities.EntityUpdater.UpdateMonstersParallel): called serially,
// once every monster's Update() has returned - never from a worker.
func (mw *MonsterWrapper) ApplyCollisionUpdate() {
	if mw.collisionSystem == nil || mw.Monster == nil {
		return
	}
	mw.collisionSystem.UpdateEntity(mw.Monster.ID, mw.Monster.X, mw.Monster.Y)
	mw.game.applyMonsterCollisionState(mw.Monster.ID, mw.pendingCollisionType, mw.pendingSolid)
}

// monsterTargetsParty reports whether a monster is actively hostile to the
// party this frame. Bound/pacified mobs and enemies redirected to a summon are
// deliberately excluded: the party may walk through them while that fight runs.
func monsterTargetsParty(m *monster.Monster3D) bool {
	if m == nil || m.Bound || m.Pacified || m.AIFoe != nil {
		return false
	}
	return m.IsEngagingPlayer || m.WasAttacked || m.BossAggro || m.Relentless ||
		m.State == monster.StateAlert || m.State == monster.StatePursuing || m.State == monster.StateAttacking
}

// desiredMonsterCollisionState is the single party-walkability rule. Peaceful
// monsters, party allies, and enemies fighting a summon are non-solid to the
// party; only a monster currently choosing the party becomes an engaged blocker.
// It reads only the monster's frame-local state, so worker updates can compute it
// safely against the frozen AI target produced before the parallel phase.
func desiredMonsterCollisionState(m *monster.Monster3D) (collision.CollisionType, bool) {
	if monsterTargetsParty(m) {
		return collision.CollisionTypeMonsterEngaged, true
	}
	return collision.CollisionTypeMonster, false
}

// applyMonsterCollisionState writes a monster's collision type and solidity to the live
// system. Must only run single-threaded (main goroutine, or another
// non-parallel context) - never from a monster-update worker; see
// desiredMonsterCollisionState for the race-free compute half.
func (g *MMGame) applyMonsterCollisionState(monsterID string, desired collision.CollisionType, solid bool) {
	if g == nil || g.collisionSystem == nil {
		return
	}
	entity := g.collisionSystem.GetEntityByID(monsterID)
	if entity == nil {
		return
	}
	if entity.CollisionType != desired {
		entity.CollisionType = desired
	}
	entity.Solid = solid
}

// refreshMonsterCollisionSolidity computes AND immediately applies m's desired
// collision type. Used by single-threaded call sites OUTSIDE the parallel RT
// tick (turn-based monster processing, boss summons, combat triggers) where
// there is no separate apply phase to defer to.
func (g *MMGame) refreshMonsterCollisionSolidity(m *monster.Monster3D) {
	if g == nil || m == nil {
		return
	}
	desired, solid := desiredMonsterCollisionState(m)
	g.applyMonsterCollisionState(m.ID, desired, solid)
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

	// pendingImpact/impactX/impactY: OnCollision (runs in a worker) only
	// records that a hit happened here - spawning the hit effect touches shared
	// state (g.spellHitEffects, g.screenShake) that must be written serially.
	// ApplyCollisionEffects (Phase 2, main goroutine) does the actual spawn.
	pendingImpact bool
	impactX       float64
	impactY       float64
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

// OnCollision runs in a worker goroutine - it only touches this projectile's
// own fields. The actual hit effect (shared state: particle list, screen
// shake) is spawned later by ApplyCollisionEffects, serially.
// IgnoresWalls: a mortar's display bolt (NoCollide) sails its whole arc; the
// deferred detonation, not terrain, ends it.
func (mpw *MagicProjectileWrapper) IgnoresWalls() bool {
	return mpw.MagicProjectile.NoCollide
}

func (mpw *MagicProjectileWrapper) OnCollision(hitX, hitY float64) {
	if mpw.MagicProjectile == nil || !mpw.MagicProjectile.Active {
		return
	}
	mpw.MagicProjectile.Active = false
	mpw.pendingImpact = true
	mpw.impactX, mpw.impactY = hitX, hitY
}

// ApplyCollisionEffects spawns the hit effect for any impact OnCollision
// recorded this tick. Phase 2 of the two-phase projectile tick (see
// entities.EntityUpdater.UpdateProjectilesParallel): called serially, once
// every projectile's Update()/OnCollision has returned - never from a worker.
func (mpw *MagicProjectileWrapper) ApplyCollisionEffects() {
	if !mpw.pendingImpact {
		return
	}
	mpw.pendingImpact = false
	if mpw.MagicProjectile == nil || mpw.game == nil {
		return
	}
	mpw.game.CreateSpellHitEffectFromSpell(mpw.impactX, mpw.impactY, mpw.MagicProjectile.SpellType)
}

// ArrowWrapper implements entities.ProjectileUpdateInterface
type ArrowWrapper struct {
	Arrow           *Arrow
	collisionSystem *collision.CollisionSystem
	projectileID    string
	game            *MMGame

	// See MagicProjectileWrapper's pendingImpact fields for the rationale.
	pendingImpact bool
	impactX       float64
	impactY       float64
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

// IgnoresWalls: arrows always stop at solid terrain.
func (aw *ArrowWrapper) IgnoresWalls() bool { return false }

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

// OnCollision runs in a worker goroutine - it only touches this projectile's
// own fields. The actual impact burst (shared state) is spawned later by
// ApplyCollisionEffects, serially.
func (aw *ArrowWrapper) OnCollision(hitX, hitY float64) {
	if aw.Arrow == nil || !aw.Arrow.Active {
		return
	}
	aw.Arrow.Active = false
	aw.pendingImpact = true
	aw.impactX, aw.impactY = hitX, hitY
}

// ApplyCollisionEffects spawns the impact burst for any hit OnCollision
// recorded this tick. Phase 2 of the two-phase projectile tick - see
// MagicProjectileWrapper.ApplyCollisionEffects.
func (aw *ArrowWrapper) ApplyCollisionEffects() {
	if !aw.pendingImpact {
		return
	}
	aw.pendingImpact = false
	if aw.Arrow == nil || aw.game == nil {
		return
	}
	// Staff/book bolt -> magical burst on wall/terrain impact, not an arrow puff
	// (shares the monster-hit decision so the staff never "explodes like an arrow").
	def, _ := config.GetWeaponDefinition(aw.Arrow.BowKey)
	aw.game.spawnWeaponBoltImpact(aw.impactX, aw.impactY, def, SpellParticleCount, SpellParticleSize)
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
