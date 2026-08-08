package game

import (
	"fmt"
	"math"
	"math/rand"

	"ugataima/internal/character"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// Terrain-facing spell effects: Jump (a short self teleport) and the ground
// toppling half of an earth nova. Both are keyed off authored spell fields, so
// no spell is recognised here by name.

// tryCastJump vaults the party JumpTiles straight ahead onto a tile they could
// legally stand on; a blocked landing refunds the SP and holds position. Like
// any no-op cast the turn and RT cooldown are still spent.
func (cs *CombatSystem) tryCastJump(def spells.SpellDefinition, caster *character.MMCharacter) bool {
	if def.JumpTiles <= 0 {
		return false
	}
	g := cs.game
	ts := float64(g.config.GetTileSize())
	dx, dy := math.Cos(g.camera.Angle), math.Sin(g.camera.Angle)
	landX := g.camera.X + dx*def.JumpTiles*ts
	landY := g.camera.Y + dy*def.JumpTiles*ts

	if g.collisionSystem == nil || !g.collisionSystem.CanMoveTo("player", landX, landY) {
		caster.SpellPoints += cs.effectiveSpellCost(caster, def.SpellPointsCost)
		g.AddCombatMessage("There is no room to land.")
		return true
	}
	g.setPartyPosition(landX, landY)
	g.AddCombatMessage(fmt.Sprintf("%s carries the party forward!", def.Name))
	// Landing on a teleporter or in deep water must resolve like any other arrival.
	if g.gameLoop != nil && g.gameLoop.inputHandler != nil {
		g.gameLoop.inputHandler.applyLandingTileEffects()
	}
	// A jump IS party movement: in TB it ends the turn like a step, so it cannot
	// be used to reposition after attacking while keeping the others' actions.
	if g.turnBasedMode {
		g.endPartyTurnAfterMovement()
	}
	return true
}

// topplePropsInRadius rolls `chance` for every crossed-standee tile:
// trees, dunes, rocks) inside the radius and replaces the ones that fail with
// plain floor - the ground shook them down. Only crossed standees are eligible:
// walls, doors and buildings do not fall over.
//
// The floor is resolved PER TILE from the region that tile belongs to: on the
// unified world an 8-tile radius can straddle a seam, and one biome for the whole
// blast would drop desert sand into a forest. Tile solidity is read live, so
// walkability follows the swap with no collision bookkeeping.
func (cs *CombatSystem) topplePropsInRadius(cx, cy, radius, chance float64) {
	g := cs.game
	if g.world == nil || world.GlobalTileManager == nil {
		return
	}
	ts := float64(g.config.GetTileSize())
	reach := int(radius/ts) + 1
	ctx, cty := TileIndex(cx, ts), TileIndex(cy, ts)
	toppled := map[[2]int]bool{}
	for ty := cty - reach; ty <= cty+reach; ty++ {
		if ty < 0 || ty >= len(g.world.Tiles) {
			continue
		}
		for tx := ctx - reach; tx <= ctx+reach; tx++ {
			if tx < 0 || tx >= len(g.world.Tiles[ty]) {
				continue
			}
			tile := g.world.Tiles[ty][tx]
			// Trees, dunes and rocks fall; a prop-class cross (boiler, crate,
			// shoji) is a built object the quake must not delete - shoji in
			// particular occlude authored secrets.
			if !tileIsNaturalCross(tile) {
				continue
			}
			wx, wy := TileCenterFromTile(tx, ty, ts)
			if Distance(cx, cy, wx, wy) > radius {
				continue
			}
			if rand.Float64() >= chance {
				continue
			}
			floor, ok := world.GlobalTileManager.GetTileTypeFromLetterForBiome(".", g.biomeAtTile(tx, ty))
			if !ok {
				continue
			}
			g.world.Tiles[ty][tx] = floor
			toppled[[2]int{tx, ty}] = true
		}
	}
	if len(toppled) == 0 {
		return
	}
	g.AddCombatMessage(fmt.Sprintf("The shaking topples %s!", pluralizeCount(len(toppled), "standing thing", "standing things")))
	// Props were only REMOVED, so the caches need filtering, not a world rescan.
	g.dropPropTilesFromRenderCaches(toppled)
}

// biomeAtTile is the biome authored for the map that OWNS this tile - the
// party's region on a split map, the region under the tile on the unified world.
func (g *MMGame) biomeAtTile(tileX, tileY int) string {
	wm := world.GlobalWorldManager
	if wm == nil {
		return ""
	}
	if g.openWorldActive() {
		if r := wm.OpenWorldRegionAtTile(tileX, tileY); r != nil {
			if mc := wm.MapConfigs[r.MapKey]; mc != nil {
				return mc.Biome
			}
			return ""
		}
	}
	if mc := wm.GetCurrentMapConfig(); mc != nil {
		return mc.Biome
	}
	return ""
}

// pluralizeCount renders "1 tree" / "3 trees" for combat-log lines.
func pluralizeCount(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// masteryLadderValue picks a per-tier authored value, or 0 when unauthored.
func masteryLadderValue(ladder []int, tier int) int {
	if len(ladder) != 4 {
		return 0
	}
	if tier < 0 {
		tier = 0
	}
	if tier > 3 {
		tier = 3
	}
	return ladder[tier]
}

// tryCastSummon brings in a party-allied monster (Summon Ice Elemental), capped
// at summon_max live copies. The spawn goes through the SAME party-summon path as
// the card allies (markPurePartySummon: no XP, no loot, crumbles on map exit);
// only its HP and damage are overwritten from the caster's mastery ladders, so
// the same monster serves every tier.
func (cs *CombatSystem) tryCastSummon(def spells.SpellDefinition, caster *character.MMCharacter) bool {
	if def.SummonMonster == "" {
		return false
	}
	owner := summonSpellOwner(def.ID)
	live := cs.countLiveSummonsByOwner(owner)
	if live >= def.SummonMax {
		caster.SpellPoints += cs.effectiveSpellCost(caster, def.SpellPointsCost)
		cs.game.AddCombatMessage(fmt.Sprintf("%s already serves you.", def.Name))
		return true
	}

	add := cs.spawnPartyAlly(def.SummonMonster, owner)
	if add == nil {
		caster.SpellPoints += cs.effectiveSpellCost(caster, def.SpellPointsCost)
		cs.game.AddCombatMessage("There is no room to summon here.")
		return true
	}
	tier := casterSpellMasteryTier(caster, def)
	if hp := masteryLadderValue(def.SummonHPByMastery, tier); hp > 0 {
		add.MaxHitPoints, add.HitPoints = hp, hp
	}
	if dmg := masteryLadderValue(def.SummonDamageByMastery, tier); dmg > 0 {
		add.DamageMin, add.DamageMax = dmg, dmg
	}
	// spawnPartyAlly already registered it in the world and collision system.
	cs.game.AddCombatMessage(fmt.Sprintf("%s answers the call! (%d HP, %d damage)", add.Name, add.MaxHitPoints, add.DamageMax))
	return true
}

// summonSpellOwner is the SummonedBy tag for a spell's summons - one namespace
// per spell so each spell counts only its own against summon_max. The shared
// "spell:" prefix is what isPurePartySummon matches on; without it the summon
// would take the party's own spell/zone/trap damage.
func summonSpellOwner(id spells.SpellID) string { return spellSummonOwnerPrefix + string(id) }
