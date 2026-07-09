package game

import "ugataima/internal/character"

// npcRenderCat is the single enum describing HOW an NPC renders. It is the one
// source of truth shared by the engine draw dispatch (drawUnifiedNPCSprite),
// the sprite-metric selection (NPCSpriteMetrics) and the map-editor `@` palette
// grouping - so the classification can never drift between editor and game.
//
// The category is normally AUTHORED in YAML (NPC `render_category`); when a
// definition omits it, it is derived from the legacy signals (sprite, its
// render_type/wall_mounted, and whether the sheet is animated) so old content
// keeps working. deriveNPCRenderCat is also what seeds the explicit field.
type npcRenderCat int

const (
	catStandee   npcRenderCat = iota // a person/figure token (static sprite)
	catAnimated                      // a person/figure whose sheet is w == h*SpriteSheetFrameCount
	catWall                          // flush wall-mounted token (slides to the nearest solid neighbour)
	catLandmark                      // tall crossed monument standee (towers, gates, fountains)
	catScenery                       // scenery prop (environment sprite; sized like a tree)
	catInvisible                     // no sprite - a pure interaction/anchor point
)

// npcCatName is the canonical YAML value for each category (NPC render_category)
// and the reverse map used to parse it. Editing one keeps both in sync.
var npcCatName = map[npcRenderCat]string{
	catStandee:   "standee",
	catAnimated:  "animated",
	catWall:      "wall",
	catLandmark:  "landmark",
	catScenery:   "scenery",
	catInvisible: "invisible",
}

var npcCatByName = func() map[string]npcRenderCat {
	m := make(map[string]npcRenderCat, len(npcCatName))
	for c, n := range npcCatName {
		m[n] = c
	}
	return m
}()

// npcCatLabel is the human-facing section title the map editor groups under.
var npcCatLabel = map[npcRenderCat]string{
	catStandee:   "Standee",
	catAnimated:  "Animated NPC",
	catWall:      "Wall standee",
	catLandmark:  "Landmark (crossed standee)",
	catScenery:   "Scenery standee",
	catInvisible: "Invisible / anchor",
}

// ValidNPCRenderCategories is the set of accepted render_category YAML values,
// for fail-fast content validation.
var ValidNPCRenderCategories = func() map[string]bool {
	m := make(map[string]bool, len(npcCatByName))
	for n := range npcCatByName {
		m[n] = true
	}
	return m
}()

// deriveNPCRenderCat reproduces the historical signal-based classification. It
// is the fallback when render_category is unset AND the seed for populating it.
func deriveNPCRenderCat(sprite, renderType string, wallMounted bool, spriteW, spriteH int) npcRenderCat {
	switch {
	case sprite == "" || sprite == "none":
		return catInvisible
	case wallMounted:
		return catWall
	case renderType == "landmark":
		return catLandmark
	case renderType == "environment_sprite":
		return catScenery
	case spriteH > 0 && spriteW == spriteH*SpriteSheetFrameCount:
		return catAnimated
	default:
		return catStandee
	}
}

// resolveNPCRenderCat returns the authored category when render_category is a
// known value, else derives it. spriteW/spriteH matter only for the derived
// animated case (0,0 is fine wherever the caller doesn't need that distinction).
func resolveNPCRenderCat(renderCategory, sprite, renderType string, wallMounted bool, spriteW, spriteH int) npcRenderCat {
	if renderCategory != "" {
		if c, ok := npcCatByName[renderCategory]; ok {
			return c
		}
	}
	return deriveNPCRenderCat(sprite, renderType, wallMounted, spriteW, spriteH)
}

// npcRenderCatOf resolves the category for a live NPC (engine paths).
func npcRenderCatOf(npc *character.NPC, spriteW, spriteH int) npcRenderCat {
	return resolveNPCRenderCat(npc.RenderCategory, npc.Sprite, npc.RenderType, npc.WallMounted, spriteW, spriteH)
}

// NPCDisplayCategory returns the editor grouping label for an NPC definition
// (render_category wins; otherwise derived). Same classification the renderer
// dispatches on, so a new render branch surfaces as its own palette group.
func NPCDisplayCategory(renderCategory, sprite, renderType string, wallMounted bool, spriteW, spriteH int) string {
	return npcCatLabel[resolveNPCRenderCat(renderCategory, sprite, renderType, wallMounted, spriteW, spriteH)]
}

// NPCDisplayCategoryOrder is the canonical section order for grouped NPC
// palettes (most structural first, plain standees last). Unlisted labels are
// appended after these by the caller.
var NPCDisplayCategoryOrder = []string{
	npcCatLabel[catStandee],
	npcCatLabel[catAnimated],
	npcCatLabel[catWall],
	npcCatLabel[catLandmark],
	npcCatLabel[catScenery],
	npcCatLabel[catInvisible],
}
