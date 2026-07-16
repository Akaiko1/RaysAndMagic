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
	"ugataima/internal/game"
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
	config.MustLoadLootTables("assets/loots.yaml")

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
	monster.SetSizeClassHeights(cfg.Graphics.SizeClasses)
	if err := monster.ValidateSizeClassHeights(); err != nil {
		log.Fatalf("Size class config: %v", err)
	}
	monsterCfg := monster.MustLoadMonsterConfig("assets/monsters.yaml")
	character.MustLoadNPCConfig("assets/npcs.yaml")
	config.MustLoadChampionConfig("assets/champions.yaml")

	// Build every champion once so a bad class/skill/equipment key fails loud at
	// startup instead of mid-combat (needs the class, weapon and item catalogs
	// loaded above).
	if err := game.PrimeChampions(cfg); err != nil {
		log.Fatalf("Champion build: %v", err)
	}
	// Fail fast on champion/monster link integrity: a monster naming an unknown
	// champion, a champion no monster carries (the duel roller would dead-end on
	// it), two monsters sharing one build (ambiguous duel identity), and a tier
	// HP above the mob's authored ceiling (the champion would spawn wounded).
	links := map[string]string{}
	for key, def := range monsterCfg.Monsters {
		if def.Champion == "" {
			continue
		}
		if config.GetChampionDefinition(def.Champion) == nil {
			log.Fatalf("monster %q references unknown champion %q", key, def.Champion)
		}
		if prev, dup := links[def.Champion]; dup {
			log.Fatalf("champion %q is carried by both %q and %q - one mob per build", def.Champion, prev, key)
		}
		links[def.Champion] = key
		if config.GlobalChampionConfig != nil {
			for tierName, tier := range config.GlobalChampionConfig.Tiers {
				if tier != nil && tier.HP > def.MaxHitPoints {
					log.Fatalf("champion mob %q max_hit_points %d is below tier %q hp %d - it would spawn wounded", key, def.MaxHitPoints, tierName, tier.HP)
				}
			}
		}
	}
	for _, championKey := range config.ChampionKeys() {
		if _, ok := links[championKey]; !ok {
			log.Fatalf("champion %q has no monsters.yaml mob carrying it (champion: %s)", championKey, championKey)
		}
	}

	// Fail fast on an NPC naming a size_class the config doesn't define (a typo
	// would otherwise silently render it at fallback wall height).
	for key, npc := range character.NPCConfigInstance.NPCs {
		if npc.SizeClass != "" {
			if _, ok := monster.SizeClassTiles(npc.SizeClass); !ok {
				log.Fatalf("NPC %q has unknown size_class %q", key, npc.SizeClass)
			}
		}
		validateDuelChoices(key, npc.Dialogue, monsterCfg)
	}
	if err := game.ValidateNPCRenderCategories(character.NPCConfigInstance.NPCs); err != nil {
		log.Fatalf("%v", err)
	}
	if err := game.ValidateNPCCommerce(character.NPCConfigInstance.NPCs); err != nil {
		log.Fatalf("%v", err)
	}

	return cfg, monsterCfg
}

// validateDuelChoices fails fast on a start_arena_duel choice naming an
// unknown difficulty tier, and requires at least one champion-linked monster
// to exist (the duel rolls a random champion from the registry). Walks the
// nested choice tree.
func validateDuelChoices(npcKey string, dlg *character.NPCDialogue, monsterCfg *monster.MonsterYAMLConfig) {
	if dlg == nil {
		return
	}
	var walk func(choices []*character.NPCDialogueChoice)
	walk = func(choices []*character.NPCDialogueChoice) {
		for _, c := range choices {
			if c == nil {
				continue
			}
			if c.Action == "start_arena_duel" {
				if config.GlobalChampionConfig == nil || config.GlobalChampionConfig.Tiers[c.Tier] == nil {
					log.Fatalf("NPC %q: start_arena_duel choice %q has unknown tier %q", npcKey, c.Text, c.Tier)
				}
				found := false
				for _, def := range monsterCfg.Monsters {
					if def.Champion != "" {
						found = true
						break
					}
				}
				if !found {
					log.Fatalf("NPC %q: start_arena_duel but no monster carries a champion build", npcKey)
				}
			}
			walk(c.Choices)
		}
	}
	walk(dlg.Choices)
}
