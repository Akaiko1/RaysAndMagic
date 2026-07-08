package game

// NPCDisplayCategory names HOW an NPC renders, mirroring the dispatch in
// drawUnifiedNPCSprite (plus the collection in drawAllSpritesSorted). The map
// editor groups the `@` palette by this label so authors see standees, wall
// tokens, landmarks and animated folk apart instead of one flat alphabetical
// list.
//
// spriteW/spriteH are the resolved sprite's pixel size (0 when the file can't
// be resolved); an animated sheet is width == height * SpriteSheetFrameCount
// (the same test selectAnimatedSpriteFrame uses).
//
// EXTENDING: when a new render branch is added to the NPC draw path, add a
// matching case here in the SAME priority order as the renderer's early-returns
// (spriteless -> wall -> landmark -> scenery -> animated -> plain). The editor
// buckets by whatever string this returns, so a brand-new category shows up as
// its own palette group automatically - no editor change needed. Keep new
// labels listed in npcDisplayCategoryOrder so they sort predictably.
func NPCDisplayCategory(sprite, renderType string, wallMounted bool, spriteW, spriteH int) string {
	switch {
	case sprite == "" || sprite == "none":
		return NPCCatInvisible
	case wallMounted:
		return NPCCatWallStandee
	case renderType == "landmark":
		return NPCCatLandmark
	case renderType == "environment_sprite":
		return NPCCatScenery
	case spriteH > 0 && spriteW == spriteH*SpriteSheetFrameCount:
		return NPCCatAnimated
	default:
		return NPCCatStandee
	}
}

// NPC display-category labels (see NPCDisplayCategory).
const (
	NPCCatInvisible   = "Invisible / anchor"
	NPCCatWallStandee = "Wall standee"
	NPCCatLandmark    = "Landmark (crossed standee)"
	NPCCatScenery     = "Scenery standee"
	NPCCatAnimated    = "Animated NPC"
	NPCCatStandee     = "Standee"
)

// NPCDisplayCategoryOrder is the canonical section order for grouped NPC
// palettes (most structural first, plain standees last). A category NOT listed
// here still renders - callers append unknown categories after these - so
// adding one to NPCDisplayCategory without touching this list is safe, it just
// sorts to the end.
var NPCDisplayCategoryOrder = []string{
	NPCCatStandee,
	NPCCatAnimated,
	NPCCatWallStandee,
	NPCCatLandmark,
	NPCCatScenery,
	NPCCatInvisible,
}
