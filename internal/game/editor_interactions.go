package game

import (
	"slices"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/quests"
	"ugataima/internal/world"
)

// EditorDialogueActions derives authorable rows from the actual dispatcher.
// Door-unlock and summon rows are synthesized from their object's configuration.
func EditorDialogueActions() []string {
	keys := make([]string, 0, len(dialogActions))
	for key := range dialogActions {
		if key != "open_door" && key != "summon_dragon" {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

// Source configs are immutable during play. Reloads replace their pointers;
// placement changes also invalidate the projected geometry.
type projectedAdventure struct {
	source                          *config.AdventureConfig
	region                          *world.OpenWorldRegion
	offsetX, offsetY, width, height int
	orient                          string
	value                           *config.AdventureConfig
}

// Source YAML always stores local coordinates. Runtime users receive projected
// copies so drawing, entry, clocks and controlled terrain agree in stitched maps.
func (g *MMGame) projectAdventureConfig(key string, a *config.AdventureConfig) *config.AdventureConfig {
	wm := world.GlobalWorldManager
	if a == nil || !g.openWorldActive() || wm.OpenWorldRegionByKey(key) == nil {
		return a
	}
	region := wm.OpenWorldRegionByKey(key)
	cached := g.projectedAdventures[key]
	if cached.source == a && cached.region == region && cached.offsetX == region.OffsetX && cached.offsetY == region.OffsetY && cached.width == region.LocalWidth && cached.height == region.LocalHeight && cached.orient == region.Orient {
		return cached.value
	}
	result := *a
	project := func(x, y int) (int, int) { return wm.ProjectTile(key, x, y) }
	box := func(r [4]int) [4]int {
		x0, y0 := project(r[0], r[1])
		x1, y1 := project(r[2], r[3])
		return [4]int{min(x0, x1), min(y0, y1), max(x0, x1), max(y0, y1)}
	}
	result.Effects = slices.Clone(a.Effects)
	for i := range result.Effects {
		e := &result.Effects[i]
		e.Rect = box(e.Rect)
		if e.Kind == "transfer" {
			e.Destination[0], e.Destination[1] = project(e.Destination[0], e.Destination[1])
		}
	}
	result.Controls = slices.Clone(a.Controls)
	for i := range result.Controls {
		c := &result.Controls[i]
		c.Tiles = slices.Clone(c.Tiles)
		for j := range c.Tiles {
			c.Tiles[j].X, c.Tiles[j].Y = project(c.Tiles[j].X, c.Tiles[j].Y)
		}
	}
	result.JumpLinks = slices.Clone(a.JumpLinks)
	for i := range result.JumpLinks {
		r := &result.JumpLinks[i]
		r[0], r[1] = project(r[0], r[1])
		r[2], r[3] = project(r[2], r[3])
	}
	if a.Boss != nil {
		b := *a.Boss
		result.Boss = &b
		if b.Arena != nil {
			r := box(*b.Arena)
			b.Arena = &r
		}
	}
	if g.projectedAdventures == nil {
		g.projectedAdventures = map[string]projectedAdventure{}
	}
	g.projectedAdventures[key] = projectedAdventure{a, region, region.OffsetX, region.OffsetY, region.LocalWidth, region.LocalHeight, region.Orient, &result}
	return &result
}

func EditorTimedBuffIDs() []string {
	g := &MMGame{}
	var ids []string
	for _, b := range g.buildTimedBuffs() {
		ids = append(ids, string(b.id))
	}
	slices.Sort(ids)
	return ids
}

// ValidateEditorNPCs shares boot's checks for runtime dispatch and presentation.
func ValidateEditorNPCs(defs map[string]*character.NPCData, classes map[string]float64) error {
	if err := character.ValidateNPCDefinitions(defs); err != nil {
		return err
	}
	for _, check := range []func(map[string]*character.NPCData) error{ValidateNPCRenderCategories, ValidateNPCCommerce, ValidateDoorNPCs} {
		if err := check(defs); err != nil {
			return err
		}
	}
	if err := ValidateNPCVisualSizes(defs, classes); err != nil {
		return err
	}
	previous := character.NPCConfigInstance
	defer func() { character.NPCConfigInstance = previous }()
	character.NPCConfigInstance = &character.NPCConfig{NPCs: defs}
	g := &MMGame{}
	for _, check := range []func() error{g.validateDialogueActionsAreDispatched, g.validateDialogueRowsAreDrawable, g.validateNPCCastBuffs} {
		if err := check(); err != nil {
			return err
		}
	}
	return nil
}

// A mechanism is owned by its source region. Do not let a click across a seam
// operate a same-named control in the party's current region.
func (g *MMGame) sceneInteractionInCurrentRegion(n *character.NPC) bool {
	if n == nil || !g.openWorldActive() {
		return true
	}
	tile := float64(g.config.GetTileSize())
	r := world.GlobalWorldManager.OpenWorldRegionAtTile(TileIndex(n.X, tile), TileIndex(n.Y, tile))
	if r == nil || r.MapKey == currentMapKey() {
		return true
	}
	g.AddCombatMessage("Enter the mechanism's region before operating it.")
	return false
}

// Quest links use the same catalog validation as game startup. The editor owns
// source maps rather than a running world; geometry is checked separately.
func ValidateEditorQuestLinks(defs map[string]*character.NPCData, questPath string) error {
	catalog, err := quests.LoadQuestConfig(questPath)
	if err != nil {
		return err
	}
	qm := quests.NewQuestManager(catalog)
	previousNPC, previousWM := character.NPCConfigInstance, world.GlobalWorldManager
	defer func() { character.NPCConfigInstance = previousNPC; world.GlobalWorldManager = previousWM }()
	merged := map[string]*character.NPCData{}
	if previousNPC != nil {
		for key, n := range previousNPC.NPCs {
			merged[key] = n
		}
	}
	for key, n := range defs {
		merged[key] = n
	}
	character.NPCConfigInstance = &character.NPCConfig{NPCs: merged}
	world.GlobalWorldManager = nil
	g := &MMGame{questManager: qm}
	return g.validateQuestWorldReferences(qm)
}
