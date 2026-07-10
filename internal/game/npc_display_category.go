package game

import (
	"fmt"

	"ugataima/internal/character"
)

// npcRenderCat is the single enum describing HOW an NPC renders. It is the one
// source of truth shared by the engine draw dispatch (drawUnifiedNPCSprite),
// the sprite-metric selection (NPCSpriteMetrics) and the map-editor `@` palette
// grouping - so the classification can never drift between editor and game.
//
// The category is AUTHORED in YAML (NPC `render_category`, REQUIRED): every
// npcs.yaml consumer validates via ValidateNPCRenderCategories and fails fast
// on a missing or unknown value, so runtime code can trust it blindly.
type npcRenderCat int

const (
	catStandee   npcRenderCat = iota // a person/figure token (static sprite)
	catAnimated                      // a person/figure whose sheet is w == h*SpriteSheetFrameCount
	catWall                          // flush wall-mounted token (slides to the nearest solid neighbour)
	catDoor                          // doorway blocker: stands ACROSS the opening (yaw from the flanking walls); drawn + solid only while closed
	catLandmark                      // tall crossed monument standee (towers, gates, fountains)
	catScenery                       // scenery prop (environment sprite; sized like a tree)
	catInvisible                     // no sprite - a pure interaction/anchor point
)

// npcCatName is the canonical YAML value for each category (NPC render_category)
// and the reverse map used to parse it. Editing one keeps both in sync.
var npcCatName = map[npcRenderCat]string{
	catStandee:   "standee",
	catAnimated:  "animated",
	catWall:      "wall_mounted",
	catDoor:      "door",
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
	catDoor:      "Door (doorway blocker)",
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

// resolveNPCRenderCat parses an authored render_category. Content validation
// (ValidateNPCRenderCategories) rejects bad values at load, so an unknown one
// here means a code path bypassed it - fail loud, never guess a render class.
func resolveNPCRenderCat(renderCategory string) npcRenderCat {
	c, ok := npcCatByName[renderCategory]
	if !ok {
		panic(fmt.Sprintf("render_category %q was not validated at content load", renderCategory))
	}
	return c
}

// ValidateNPCRenderCategories fails on the first NPC whose render_category is
// missing or unknown. Every npcs.yaml consumer (game boot, map editor) runs it.
func ValidateNPCRenderCategories(npcs map[string]*character.NPCData) error {
	for key, npc := range npcs {
		if npc == nil || !ValidNPCRenderCategories[npc.RenderCategory] {
			got := ""
			if npc != nil {
				got = npc.RenderCategory
			}
			return fmt.Errorf("NPC %q has missing or unknown render_category %q (valid: standee|animated|wall_mounted|landmark|scenery|door|invisible)", key, got)
		}
	}
	return nil
}

// npcRenderCatOf resolves the category for a live NPC (engine paths).
func npcRenderCatOf(npc *character.NPC) npcRenderCat {
	return resolveNPCRenderCat(npc.RenderCategory)
}

// NPCDisplayCategory returns the editor grouping label for an NPC definition.
// Same classification the renderer dispatches on, so a new render branch
// surfaces as its own palette group.
func NPCDisplayCategory(renderCategory string) string {
	return npcCatLabel[resolveNPCRenderCat(renderCategory)]
}

// NPCDisplayCategoryOrder is the canonical section order for grouped NPC
// palettes (most structural first, plain standees last). Unlisted labels are
// appended after these by the caller.
var NPCDisplayCategoryOrder = []string{
	npcCatLabel[catStandee],
	npcCatLabel[catAnimated],
	npcCatLabel[catWall],
	npcCatLabel[catDoor],
	npcCatLabel[catLandmark],
	npcCatLabel[catScenery],
	npcCatLabel[catInvisible],
}
