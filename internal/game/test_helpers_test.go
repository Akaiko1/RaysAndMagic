package game

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/collision"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

// Shared fixtures for internal/game tests. Keep scenario-specific setup next to
// its test; only reusable game, world, party, and champion setup belongs here.
func loadTestConfig(t *testing.T) *config.Config {
	t.Helper()

	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if _, err := config.LoadSpellConfig("../../assets/spells.yaml"); err != nil {
		t.Fatalf("load spells: %v", err)
	}
	if _, err := config.LoadWeaponConfig("../../assets/weapons.yaml"); err != nil {
		t.Fatalf("load weapons: %v", err)
	}
	if _, err := config.LoadItemConfig("../../assets/items.yaml"); err != nil {
		t.Fatalf("load items: %v", err)
	}
	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()
	if _, err := config.LoadTrapConfig("../../assets/traps.yaml"); err != nil {
		t.Fatalf("load traps: %v", err)
	}
	monster.SetSizeClassHeights(cfg.Graphics.SizeClasses)
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	return cfg
}

func newTestWorld(cfg *config.Config) *world.World3D {
	return newTestWorldSized(cfg, 2, 2)
}

func newTestWorldSized(cfg *config.Config, width, height int) *world.World3D {
	w := world.NewWorld3D(cfg)
	w.Width = width
	w.Height = height
	w.Tiles = make([][]world.TileType3D, w.Height)
	for y := range w.Tiles {
		w.Tiles[y] = make([]world.TileType3D, w.Width)
		for x := range w.Tiles[y] {
			w.Tiles[y][x] = world.TileEmpty
		}
	}
	return w
}

func newTestGame(cfg *config.Config, w *world.World3D) *MMGame {
	game := &MMGame{
		config:           cfg,
		world:            w,
		party:            character.NewParty(cfg),
		camera:           &FirstPersonCamera{X: 64, Y: 64, Angle: 1.25},
		skyImg:           ebiten.NewImage(2, 2),
		groundImg:        ebiten.NewImage(2, 2),
		collisionSystem:  collision.NewCollisionSystem(w, float64(cfg.World.TileSize)),
		sessionStartTime: time.Now(),
	}
	game.collisionSystem.RegisterEntity(newPlayerCollisionEntity(game.camera.X, game.camera.Y))
	return game
}

func tbBehaviorGame(t *testing.T, width, height int) (*MMGame, *GameLoop, float64) {
	t.Helper()
	cfg := loadTestConfig(t)
	game := newTestGame(cfg, newTestWorldSized(cfg, width, height))
	game.turnBasedMode = true
	game.combat = NewCombatSystem(game)
	for _, c := range game.party.Members {
		c.Luck = 0
	}
	return game, &GameLoop{game: game}, float64(cfg.GetTileSize())
}

func placePlayerAtTile(game *MMGame, tx, ty int, tileSize float64) {
	game.camera.X = float64(tx)*tileSize + tileSize/2
	game.camera.Y = float64(ty)*tileSize + tileSize/2
	game.collisionSystem.UpdateEntity("player", game.camera.X, game.camera.Y)
}

func runOneMonsterTurn(game *MMGame, gl *GameLoop) {
	game.currentTurn = 1
	game.monsterTurnResolved = false
	gl.updateMonstersTurnBased()
}

func primeTestChampions(t *testing.T, g *MMGame) {
	t.Helper()
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	if _, err := config.LoadChampionConfig("../../assets/champions.yaml"); err != nil {
		t.Fatalf("load champions: %v", err)
	}
	championTemplates = map[string]*character.MMCharacter{}
	if err := PrimeChampions(g.config); err != nil {
		t.Fatalf("prime champions: %v", err)
	}
}

func fillTestParty(t *testing.T, g *MMGame) {
	t.Helper()
	class, ok := character.ClassFromKey("knight")
	if !ok {
		t.Fatal("knight class missing")
	}
	g.party.Members = g.party.Members[:0]
	for i := 0; i < 4; i++ {
		ch := character.CreateCharacter("T", class, g.config)
		ch.HitPoints = ch.MaxHitPoints
		g.party.Members = append(g.party.Members, ch)
	}
}
