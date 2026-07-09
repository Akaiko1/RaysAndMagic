package game

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/world"

	"github.com/hajimehoshi/ebiten/v2"
)

// ContainerKind tags a GroundContainer with how it was spawned, controlling a
// handful of presentation defaults (sprite, size, pickup verb).
type ContainerKind int

const (
	ContainerKindLootBag       ContainerKind = iota // dropped by a monster on death
	ContainerKindTreasureChest                      // spawned by an encounter clear
)

// containerKindDefaults bundles the look-and-feel that a kind chooses when the
// caller doesn't override Sprite explicitly. Default size is config-driven
// (graphics.container_size_tiles), resolved via containerDefaultSizeTiles.
type containerKindDefaults struct {
	sprite       string
	sizeKey      string // key into graphics.container_size_tiles
	openMessage  string // multi-part "Picked up loot bag:" / "Opened chest:" prefix
	emptyMessage string // shown if container is empty when opened ("" -> silent)
}

var groundContainerDefaults = map[ContainerKind]containerKindDefaults{
	ContainerKindLootBag: {
		sprite:       "bag",
		sizeKey:      "loot_bag",
		openMessage:  "Picked up loot bag",
		emptyMessage: "", // empty loot bags are silently removed (legacy behavior)
	},
	ContainerKindTreasureChest: {
		sprite:       "chest",
		sizeKey:      "treasure_chest",
		openMessage:  "Opened chest",
		emptyMessage: "The chest is empty.",
	},
}

// containerDefaultSizeTiles resolves a kind's default sprite height (tiles) from
// config, with a small fallback if the map is absent (e.g. a config-less test).
func (g *MMGame) containerDefaultSizeTiles(kind ContainerKind) float64 {
	if h, ok := g.config.Graphics.ContainerSizeTiles[groundContainerDefaults[kind].sizeKey]; ok && h > 0 {
		return h
	}
	return 0.3
}

// GroundContainer is the unified on-floor reward container - replaces both
// the loot bag (monster drop) and treasure chest (encounter reward) systems
// that previously had near-identical parallel implementations.
type GroundContainer struct {
	Kind      ContainerKind
	ID        string // optional dedup key; "" disables dedup
	MapKey    string // "" -> current map only; set for cross-map containers
	X, Y      float64
	Gold      int
	Items     []items.Item
	Sprite    string  // "" -> default per Kind
	SizeTiles float64 // 0 -> default per Kind
}

// GroundContainerRenderInfo holds the projected screen geometry used by both
// the renderer and click-hit-tests.
type GroundContainerRenderInfo struct {
	ScreenX    int
	ScreenY    int
	SpriteSize int
	Distance   float64
	Visible    bool
}

// effectiveSprite returns the sprite name to draw / hit-test for this container.
// A loot bag holding an item shows a rarity-specific sack (bag_common/.../legendary);
// an item-less (gold-only) bag keeps the default sack. Chests are unaffected.
func (c *GroundContainer) effectiveSprite() string {
	if c != nil && c.Sprite != "" {
		return c.Sprite
	}
	if c.Kind == ContainerKindLootBag && len(c.Items) > 0 {
		return "bag_" + containerHighestRarity(c)
	}
	return groundContainerDefaults[c.Kind].sprite
}

// containerRenderSizeTiles returns the visual scale for this container: an
// explicit per-instance size wins, else the kind's config default.
func (g *MMGame) containerRenderSizeTiles(c *GroundContainer) float64 {
	if c != nil && c.SizeTiles > 0 {
		return c.SizeTiles
	}
	return g.containerDefaultSizeTiles(c.Kind)
}

// groundContainerPickupRange is shared across all ground containers - both
// "press Space to pick up" and "click to open" use the same reach.
func (g *MMGame) groundContainerPickupRange() float64 {
	if g == nil {
		return 0
	}
	return float64(g.config.GetTileSize()) * 2.0
}

// addGroundContainer is the single spawn entry point. Callers pass a partially
// filled container; this function fills in defaults (sprite, size, current
// map) and skips the spawn if the container would be empty.
func (g *MMGame) addGroundContainer(c GroundContainer) {
	if g == nil {
		return
	}
	if c.ID != "" {
		for i := range g.groundContainers {
			if g.groundContainers[i].ID == c.ID {
				return
			}
		}
	}
	if len(c.Items) == 0 && c.Gold <= 0 {
		return
	}
	// Sprite is left empty for kind defaults so effectiveSprite() resolves it live
	// (rarity-aware for loot bags); only an explicit override is stored.
	if c.SizeTiles <= 0 {
		c.SizeTiles = g.containerDefaultSizeTiles(c.Kind)
	}
	g.groundContainers = append(g.groundContainers, c)
}

// addLootBagDrop creates a loot-bag-kind container at the given world coords
// from a monster's drops and gold. Size comes from the kind's config default
// (addGroundContainer fills it) - bags are not scaled by the dropping monster.
func (g *MMGame) addLootBagDrop(x, y float64, drops []items.Item, gold int) {
	if len(drops) == 0 && gold <= 0 {
		return
	}
	g.addGroundContainer(GroundContainer{
		Kind:   ContainerKindLootBag,
		MapKey: currentMapKey(), // bags belong to the map they dropped on
		X:      x,
		Y:      y,
		Gold:   gold,
		Items:  append([]items.Item{}, drops...),
	})
}

// addTreasureChestFromReward spawns an encounter-clear chest from its YAML
// reward definition, rolling random weapons into it at spawn time.
func (g *MMGame) addTreasureChestFromReward(reward *monster.TreasureChestReward) {
	if g == nil || reward == nil {
		return
	}
	chestMap := reward.Map
	if chestMap == "" {
		chestMap = currentMapKey()
	}
	if !groundContainerTileIsValid(chestMap, reward.TileX, reward.TileY) {
		fmt.Printf("[WARN] treasure chest '%s' spawn tile (%d, %d) on map '%s' is blocking or out of bounds; chest will not be reachable\n",
			reward.ID, reward.TileX, reward.TileY, chestMap)
	}
	tileSize := float64(g.config.GetTileSize())
	x, y := TileCenterFromTile(reward.TileX, reward.TileY, tileSize)

	chestItems := randomWeaponRewards(reward.RandomWeaponCount)
	chestItems = append(chestItems, fixedWeaponRewards(reward.Weapons)...)
	chestItems = append(chestItems, fixedItemRewards(reward.Items)...)
	chestGold := reward.Gold
	if reward.LootTable != "" {
		poolItems, poolGold := rollWeightedLootTable(reward.LootTable)
		chestItems = append(chestItems, poolItems...)
		chestGold += poolGold
	}
	if len(chestItems) == 0 && chestGold <= 0 {
		return
	}

	g.addGroundContainer(GroundContainer{
		Kind:      ContainerKindTreasureChest,
		ID:        reward.ID,
		MapKey:    chestMap,
		X:         x,
		Y:         y,
		Gold:      chestGold,
		Items:     chestItems,
		Sprite:    reward.Sprite,
		SizeTiles: reward.SizeTiles,
	})
	if reward.CompletionMessage != "" {
		g.AddCombatMessage(reward.CompletionMessage)
	}
}

func (g *MMGame) addTreasureChestsFromRewards(rewards *monster.EncounterRewards) {
	if g == nil || rewards == nil {
		return
	}
	g.addTreasureChestFromReward(rewards.TreasureChest)
	for i := range rewards.TreasureChests {
		g.addTreasureChestFromReward(&rewards.TreasureChests[i])
	}
}

// randomWeaponRewards rolls `count` random weapons uniformly from weapons.yaml.
// Used by encounter-chest spawn - balance filtering (rarity tier, loot tables)
// is a separate concern tracked elsewhere.
func randomWeaponRewards(count int) []items.Item {
	if count <= 0 {
		return nil
	}
	if config.GlobalWeapons == nil || len(config.GlobalWeapons.Weapons) == 0 {
		fmt.Printf("[WARN] randomWeaponRewards: config.GlobalWeapons is empty or unloaded; chest will receive no weapons\n")
		return nil
	}
	keys := make([]string, 0, len(config.GlobalWeapons.Weapons))
	for key := range config.GlobalWeapons.Weapons {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	weapons := make([]items.Item, 0, count)
	for i := 0; i < count && len(keys) > 0; i++ {
		idx := rand.Intn(len(keys))
		weapon, err := items.TryCreateWeaponFromYAML(keys[idx])
		if err == nil {
			weapons = append(weapons, weapon)
		}
		keys = append(keys[:idx], keys[idx+1:]...)
	}
	return weapons
}

// fixedRewards turns an explicit list of YAML keys into items via the given
// constructor, skipping (with a warning) any key that fails to resolve. Shared
// by the weapon and item chest-reward paths.
func fixedRewards(keys []string, create func(string) (items.Item, error), label string) []items.Item {
	if len(keys) == 0 {
		return nil
	}
	rewards := make([]items.Item, 0, len(keys))
	for _, key := range keys {
		item, err := create(key)
		if err != nil {
			fmt.Printf("[WARN] %s: %v\n", label, err)
			continue
		}
		rewards = append(rewards, item)
	}
	return rewards
}

func fixedWeaponRewards(keys []string) []items.Item {
	return fixedRewards(keys, items.TryCreateWeaponFromYAML, "fixedWeaponRewards")
}

func fixedItemRewards(keys []string) []items.Item {
	return fixedRewards(keys, items.TryCreateItemFromYAML, "fixedItemRewards")
}

// rollWeightedLootTable rolls a named weighted pool: `Rolls` weighted picks (with
// replacement) plus gold in [GoldMin,GoldMax]. Zone containers (sword racks) use
// this for "one random zone item, never a unique" - uniques aren't in the pool,
// unlike randomWeaponRewards which draws from ALL weapons. Keys are validated at
// load (config.validateWeightedLootTables), so creation failures here are unexpected.
func rollWeightedLootTable(name string) ([]items.Item, int) {
	t, ok := config.GetWeightedLootTable(name)
	if !ok {
		fmt.Printf("[WARN] rollWeightedLootTable: unknown table %q\n", name)
		return nil, 0
	}
	total := 0
	for _, e := range t.Entries {
		total += e.Weight
	}
	out := make([]items.Item, 0, t.Rolls)
	for r := 0; r < t.Rolls && total > 0; r++ {
		pick := rand.Intn(total)
		var chosen *config.WeightedLootEntry
		for i := range t.Entries {
			if pick -= t.Entries[i].Weight; pick < 0 {
				chosen = &t.Entries[i]
				break
			}
		}
		if chosen == nil {
			continue
		}
		var it items.Item
		var err error
		switch chosen.Type {
		case "weapon":
			it, err = items.TryCreateWeaponFromYAML(chosen.Key)
		case "item":
			it, err = items.TryCreateItemFromYAML(chosen.Key)
		default:
			continue
		}
		if err != nil {
			fmt.Printf("[WARN] rollWeightedLootTable %q: %v\n", name, err)
			continue
		}
		out = append(out, it)
	}
	gold := t.GoldMin
	if t.GoldMax > t.GoldMin {
		gold += rand.Intn(t.GoldMax - t.GoldMin + 1)
	}
	return out, gold
}

// tryPickupNearestGroundContainer triggers pickup on the closest container in
// range. Returns true if a pickup occurred. Replaces the two separate
// tryPickupNearestLootBag / tryOpenNearestTreasureChest helpers.
func (g *MMGame) tryPickupNearestGroundContainer(maxDist float64) bool {
	if g == nil {
		return false
	}
	if idx := g.findGroundContainerIndex(maxDist, nil); idx >= 0 {
		g.pickupGroundContainerAt(idx)
		return true
	}
	return false
}

// findGroundContainerIndexAtScreen finds the closest in-range container whose
// rendered sprite is under the given screen coordinates.
func (g *MMGame) findGroundContainerIndexAtScreen(clickX, clickY int, maxDist float64) int {
	if g.renderHelper == nil {
		return -1
	}
	return g.findGroundContainerIndex(maxDist, func(c *GroundContainer, distance float64) bool {
		info := g.groundContainerRenderInfo(c, distance)
		return g.groundContainerHitTestFromInfo(info, c.effectiveSprite(), clickX, clickY, maxDist)
	})
}

// findGroundContainerIndex scans containers on the current map within maxDist.
// Returns the index of the closest match; if accept is non-nil, only
// containers for which accept(c, distance) returns true are considered.
func (g *MMGame) findGroundContainerIndex(maxDist float64, accept func(c *GroundContainer, distance float64) bool) int {
	if len(g.groundContainers) == 0 {
		return -1
	}
	currentMap := currentMapKey()
	playerX, playerY := g.camera.X, g.camera.Y
	maxDistSq := maxDist * maxDist
	bestIdx := -1
	bestDistSq := 0.0
	for i := range g.groundContainers {
		c := &g.groundContainers[i]
		if c.MapKey != "" && c.MapKey != currentMap {
			continue
		}
		dx := c.X - playerX
		dy := c.Y - playerY
		distSq := dx*dx + dy*dy
		if distSq > maxDistSq {
			continue
		}
		if accept != nil && !accept(c, math.Sqrt(distSq)) {
			continue
		}
		if bestIdx == -1 || distSq < bestDistSq {
			bestIdx = i
			bestDistSq = distSq
		}
	}
	return bestIdx
}

// pickupGroundContainerAt removes the container at the given index, transfers
// its gold and items to the party, and emits the appropriate combat message
// for its Kind.
func (g *MMGame) pickupGroundContainerAt(index int) {
	if index < 0 || index >= len(g.groundContainers) {
		return
	}
	c := g.groundContainers[index]
	defaults := groundContainerDefaults[c.Kind]

	if len(c.Items) == 0 && c.Gold <= 0 {
		if defaults.emptyMessage != "" {
			g.AddCombatMessage(defaults.emptyMessage)
		}
		g.groundContainers = append(g.groundContainers[:index], g.groundContainers[index+1:]...)
		return
	}

	for _, it := range c.Items {
		g.party.AddItem(it)
	}
	if c.Gold > 0 {
		g.awardGold(c.Gold)
	}

	// Compose a kind-appropriate combat message. Loot bags get three text
	// variants for terseness when there's a single thing; chests always use
	// the multi-part "Opened chest: ..." form.
	switch {
	case c.Kind == ContainerKindLootBag && len(c.Items) == 0 && c.Gold > 0:
		g.AddCombatMessage(fmt.Sprintf("Picked up %d gold.", c.Gold))
	case c.Kind == ContainerKindLootBag && len(c.Items) == 1 && c.Gold <= 0:
		g.AddColoredCombatMessage(fmt.Sprintf("Picked up %s.", c.Items[0].Name), lootMessageColor(c.Items))
	default:
		parts := make([]string, 0, len(c.Items)+1)
		if c.Gold > 0 {
			parts = append(parts, fmt.Sprintf("%d gold", c.Gold))
		}
		for _, it := range c.Items {
			parts = append(parts, it.Name)
		}
		message := fmt.Sprintf("%s: %s.", defaults.openMessage, strings.Join(parts, ", "))
		if len(c.Items) > 0 {
			g.AddColoredCombatMessage(message, lootMessageColor(c.Items))
		} else {
			g.AddCombatMessage(message)
		}
	}

	g.groundContainers = append(g.groundContainers[:index], g.groundContainers[index+1:]...)
}

// groundContainerRenderInfo projects a container's world position to screen. The
// render-only band fan offset is folded in so the projection and click hit-test
// both track the fanned sprite when several containers share a tile.
func (g *MMGame) groundContainerRenderInfo(c *GroundContainer, distance float64) GroundContainerRenderInfo {
	info := GroundContainerRenderInfo{Distance: distance}
	if c == nil || g.renderHelper == nil {
		return info
	}
	if info.Distance < 0 {
		info.Distance = math.Hypot(c.X-g.camera.X, c.Y-g.camera.Y)
	}
	ox, oy := g.groundContainerRenderOffset(c)
	info.ScreenX, info.ScreenY, info.SpriteSize, info.Visible = g.renderHelper.CalculateMonsterSpriteMetrics(c.X+ox, c.Y+oy, info.Distance, g.containerRenderSizeTiles(c))
	return info
}

// groundContainerRenderOffset returns the render-only fan offset for a container
// sharing its tile with others, so a pile of loot bags reads as a band - the same
// visual (and formula) as monster banding. Solo containers get (0,0).
func (g *MMGame) groundContainerRenderOffset(c *GroundContainer) (float64, float64) {
	if g == nil || c == nil {
		return 0, 0
	}
	tile := float64(g.config.GetTileSize())
	ctx, cty := int(c.X/tile), int(c.Y/tile)
	idx, count := 0, 0
	for i := range g.groundContainers {
		o := &g.groundContainers[i]
		if o.MapKey != c.MapKey || int(o.X/tile) != ctx || int(o.Y/tile) != cty {
			continue
		}
		if o == c {
			idx = count
		}
		count++
	}
	if count <= 1 {
		return 0, 0
	}
	// Arrange same-tile containers evenly on a ring around the tile CENTRE so a
	// pile from several kills reads as several bags side by side - regardless of
	// where each mob actually died (they don't snap together like monster bands).
	// Offset = ring slot - drop position.
	ox, oy := containerFanOffset(idx, count, tile)
	cx := (float64(ctx) + 0.5) * tile
	cy := (float64(cty) + 0.5) * tile
	return cx + ox - c.X, cy + oy - c.Y
}

const containerFanRadiusTiles = 0.32

// containerFanOffset spreads count containers evenly on a ring of radius
// containerFanRadiusTiles. Every member sits ON the ring (so two bags land
// opposite each other - side by side, not one behind the other). The radius grows
// slightly with the count so a bigger pile stays legible.
func containerFanOffset(idx, count int, tile float64) (float64, float64) {
	if count <= 1 {
		return 0, 0
	}
	r := containerFanRadiusTiles * tile
	if count > 3 {
		r *= 1.0 + 0.12*float64(count-3)
	}
	ang := (2 * math.Pi * float64(idx)) / float64(count)
	return math.Cos(ang) * r, math.Sin(ang) * r
}

// containerHighestRarity returns the rarest item rarity in a container ("common"
// when empty), used to pick the metallic loot-bag tint.
func containerHighestRarity(c *GroundContainer) string {
	best, bestTier := "common", 0
	for i := range c.Items {
		if r := itemRarity(c.Items[i]); rarityTier(r) > bestTier {
			best, bestTier = r, rarityTier(r)
		}
	}
	return best
}

// groundContainerHitTestFromInfo returns true if (mouseX, mouseY) is over the
// container's rendered sprite and the container is within maxDist.
func (g *MMGame) groundContainerHitTestFromInfo(info GroundContainerRenderInfo, spriteName string, mouseX, mouseY int, maxDist float64) bool {
	if !info.Visible || info.SpriteSize <= 0 {
		return false
	}
	if info.Distance > maxDist {
		return false
	}
	if spriteName == "" {
		spriteName = "bag"
	}
	sprite := g.sprites.GetSprite(spriteName)
	drawLeft := info.ScreenX - info.SpriteSize/2
	return spriteHitTest(sprite, mouseX, mouseY, drawLeft, info.ScreenY, info.SpriteSize)
}

// currentMapKey returns the active map key, with nil-safety for early-init or
// shutdown paths where GlobalWorldManager may not be set yet.
func currentMapKey() string {
	if world.GlobalWorldManager == nil {
		return ""
	}
	return world.GlobalWorldManager.CurrentMapKey
}

// groundContainerTileIsValid returns false if (tileX, tileY) on the named map
// is blocking or out of bounds - both cases would leave the container
// unreachable. Used for fail-fast YAML validation at spawn time.
func groundContainerTileIsValid(mapKey string, tileX, tileY int) bool {
	if world.GlobalWorldManager == nil {
		return true
	}
	w, ok := world.GlobalWorldManager.LoadedMaps[mapKey]
	if !ok || w == nil {
		return true
	}
	return !w.IsTileBlocking(tileX, tileY)
}

// spriteHitTest is a pixel-perfect hit test against an image-backed sprite.
// Used by all ground-container interaction (click-to-pick-up/open).
func spriteHitTest(sprite *ebiten.Image, mouseX, mouseY, drawLeft, drawTop, spriteSize int) bool {
	if sprite == nil || spriteSize <= 0 {
		return false
	}
	if mouseX < drawLeft || mouseX >= drawLeft+spriteSize || mouseY < drawTop || mouseY >= drawTop+spriteSize {
		return false
	}
	spriteW := sprite.Bounds().Dx()
	spriteH := sprite.Bounds().Dy()
	if spriteW == 0 || spriteH == 0 {
		return false
	}
	scaleX := float64(spriteSize) / float64(spriteW)
	scaleY := float64(spriteSize) / float64(spriteH)
	localX := int(float64(mouseX-drawLeft) / scaleX)
	localY := int(float64(mouseY-drawTop) / scaleY)
	if localX < 0 || localX >= spriteW || localY < 0 || localY >= spriteH {
		return false
	}
	_, _, _, a := sprite.At(localX, localY).RGBA()
	return a > 0
}
