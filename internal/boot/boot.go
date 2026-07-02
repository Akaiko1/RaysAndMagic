// Package boot is the single startup sequence for every binary that consumes
// the game's content configs (game, map editor). Load order is dependency
// order: item/weapon defs before bridges, tile manager before monsters and
// map loading, spells before NPCs.
package boot

import (
	"log"

	"ugataima/internal/bridge"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/storage"
	"ugataima/internal/world"
)

// LoadGameData resolves the runtime working directory and fail-fast loads the
// shared content configs. Binary-specific configs (loot tables, quests,
// level-up choices, maps) stay with their binary's main.
func LoadGameData() (*config.Config, *monster.MonsterYAMLConfig) {
	storage.EnsureRuntimeCWD()

	cfg := config.MustLoadConfig("config.yaml")
	config.MustLoadSpellConfig("assets/spells.yaml")
	config.MustLoadWeaponConfig("assets/weapons.yaml")
	config.MustLoadItemConfig("assets/items.yaml")

	bridge.SetupWeaponBridge()
	bridge.SetupItemBridge()

	world.GlobalTileManager = world.NewTileManager()
	if err := world.GlobalTileManager.LoadTileConfig("assets/tiles.yaml"); err != nil {
		log.Fatalf("Failed to load tile config: %v", err)
	}
	if err := world.GlobalTileManager.LoadSpecialTileConfig("assets/special_tiles.yaml"); err != nil {
		log.Fatalf("Failed to load special tile config: %v", err)
	}

	config.MustLoadTrapConfig("assets/traps.yaml")
	monsterCfg := monster.MustLoadMonsterConfig("assets/monsters.yaml")
	character.MustLoadNPCConfig("assets/npcs.yaml")

	return cfg, monsterCfg
}
