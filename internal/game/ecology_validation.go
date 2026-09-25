package game

import (
	"fmt"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// ValidateEcologyContent checks content links after maps and quests have loaded.
// Scalar YAML validation belongs to config; world/catalog links belong here.
func ValidateEcologyContent(cfg *config.Config, qm *quests.QuestManager) error {
	c, wm := config.GlobalEcology, world.GlobalWorldManager
	if c == nil || wm == nil {
		return nil
	}
	if err := c.Validate(); err != nil {
		return err
	}
	checkActor := func(key, disposition string) error {
		if monster.MonsterConfig == nil {
			return fmt.Errorf("ecology requires monster catalog")
		}
		def, err := monster.MonsterConfig.GetMonsterByKey(key)
		if err != nil || def.Disposition != disposition {
			return fmt.Errorf("ecology actor %q must have disposition %q", key, disposition)
		}
		if def.Arboreal != nil {
			for _, tree := range def.Arboreal.TreeTiles {
				if world.GlobalTileManager == nil {
					return fmt.Errorf("arboreal actor %q needs the tile catalog", key)
				}
				tile := world.GlobalTileManager.GetTileDataByKey(tree)
				if tile == nil || tile.RenderType != config.TileRenderCrossedStandee || tile.Walkable {
					return fmt.Errorf("arboreal actor %q has invalid tree %q", key, tree)
				}
			}
		}
		return nil
	}
	if c.Fish != nil {
		for mapKey, key := range c.Fish.Species {
			if err := checkActor(key, monster.DispositionFish); err != nil {
				return err
			}
			if ecologyWorld(mapKey) == nil {
				return fmt.Errorf("fish map %q is missing", mapKey)
			}
		}
	}
	for _, p := range c.Populations {
		if err := checkActor(p.Monster, monster.DispositionWildlife); err != nil {
			return err
		}
		if ecologyWorld(p.Map) == nil {
			return fmt.Errorf("ecology population map %q is missing", p.Map)
		}
	}
	if err := checkActor(c.Caravan.Monster, monster.DispositionCaravan); err != nil {
		return err
	}
	if character.NPCConfigInstance == nil || character.NPCConfigInstance.NPCs[c.Caravan.Merchant] == nil {
		return fmt.Errorf("ecology merchant %q is missing", c.Caravan.Merchant)
	}
	if qm == nil || qm.Definitions()[c.Caravan.UnlockQuest] == nil {
		return fmt.Errorf("ecology unlock quest %q is missing", c.Caravan.UnlockQuest)
	}
	first := c.Caravan.Routes[0].Points[0]
	for _, r := range c.Caravan.Routes {
		if r.Points[0] != first {
			return fmt.Errorf("caravan route %q must share the home anchor", r.ID)
		}
		for _, p := range r.Points {
			if p.Skip {
				continue
			}
			w, x, y := ecologyPoint(p, float64(cfg.GetTileSize()))
			if w == nil || w.IsTileBlockingForMonster(int(x/float64(cfg.GetTileSize())), int(y/float64(cfg.GetTileSize())), nil, false) {
				return fmt.Errorf("caravan route %q has blocked/missing anchor %+v", r.ID, p)
			}
		}
	}
	return nil
}
