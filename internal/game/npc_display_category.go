package game

import (
	"fmt"
	"strings"

	"ugataima/internal/character"
)

// npcRenderCat is the single enum describing HOW an NPC renders - the one
// source of truth for the engine draw dispatch (drawUnifiedNPCSprite) and the
// sprite-metric selection (NPCSpriteMetrics). PURELY about rendering: the
// editor palette classifies by the authored NPC `type:` instead (see
// character.NPCTypeOrder), never by render_category.
//
// The category is AUTHORED in YAML (NPC `render_category`, REQUIRED): every
// npcs.yaml consumer validates via ValidateNPCRenderCategories and fails fast
// on a missing or unknown value, so runtime code can trust it blindly.
//
// Frame ANIMATION is never a category: any sprite whose sheet is
// w == frameHeight*SpriteSheetFrameCount animates, whatever category it is in
// (selectAnimatedSpriteFrame). A category only decides pose, sizing floor and
// interaction semantics.
type npcRenderCat int

const (
	catNPC       npcRenderCat = iota // a person/figure: turns to face the party, keeps the readability pixel floor, "talk to"
	catWall                          // flush wall-mounted token (slides to the nearest solid neighbour)
	catDoor                          // doorway blocker: stands ACROSS the opening (yaw from the flanking walls); drawn + solid only while closed
	catLandmark                      // tall crossed monument standee (towers, gates, fountains)
	catScenery                       // scenery prop: showcase-spins in place, may recede small at range, "investigate"
	catInvisible                     // no sprite - a pure interaction/anchor point
)

// npcCatName is the canonical YAML value for each category (NPC render_category)
// and the reverse map used to parse it. Editing one keeps both in sync.
var npcCatName = map[npcRenderCat]string{
	catNPC:       "npc",
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
			return fmt.Errorf("NPC %q has missing or unknown render_category %q (valid: npc|wall_mounted|landmark|scenery|door|invisible)", key, got)
		}
	}
	return nil
}

// npcRenderCatOf resolves the category for a live NPC (engine paths).
func npcRenderCatOf(npc *character.NPC) npcRenderCat {
	return resolveNPCRenderCat(npc.RenderCategory)
}

// npcIsPerson reports whether the NPC is a figure (a person you talk TO), as
// opposed to a prop/landmark/anchor you investigate. Driven by the authored
// render_category, so prompts can never disagree with what is drawn.
func npcIsPerson(npc *character.NPC) bool {
	return npcRenderCatOf(npc) == catNPC
}

// npcSpriteName resolves the sprite an NPC currently shows: the authored
// visited_sprite once Visited (an emptied barrel closes -> opens), else the
// base sprite. Drives both the draw lookup and the standee cache keys, so a
// swap can never serve a stale cached silhouette.
func npcSpriteName(npc *character.NPC) string {
	name := npc.Sprite
	if npc.Visited && npc.VisitedSprite != "" {
		name = npc.VisitedSprite
	}
	if name == "" {
		return "elf"
	}
	return strings.TrimSuffix(name, ".png")
}
