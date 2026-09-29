package game

import (
	"slices"
	"sort"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// Confirmation belongs to this pair of heroes and these important item names,
// not to roster indices that may point at different heroes after another change.
type rosterSwapRequest struct {
	active, reserve    int
	outgoing, incoming *character.MMCharacter
	items              []string
}

func rosterImportantItems(hero *character.MMCharacter) []string {
	if hero == nil {
		return nil
	}
	currencies := character.ItemCurrencyMerchants(character.NPCConfigInstance)
	statuettes := map[string]bool{}
	if catalog := character.NPCConfigInstance; catalog != nil {
		for _, npc := range catalog.NPCs {
			if npc == nil {
				continue
			}
			for _, summon := range npc.Summons {
				if summon != nil {
					statuettes[summon.Statuette] = true
				}
			}
		}
	}
	var names []string
	for it := range hero.PersonalItems() {
		_, key, _ := config.GetItemDefinitionByName(it.Name)
		_, currency := currencies[key]
		if it.Type == items.ItemQuest || it.Attributes["door_key"] > 0 || it.Attributes["master_key"] > 0 || currency || statuettes[it.Name] {
			names = append(names, it.Name)
		}
	}
	sort.Strings(names)
	return slices.Compact(names)
}

func (g *MMGame) requestRosterSwap(active, reserve int) {
	g.pendingRosterSwap = nil
	if g.party == nil || active < 0 || active >= len(g.party.Members) || reserve < 0 || reserve >= len(g.party.Reserve) {
		return
	}
	outgoing, incoming := g.party.Members[active], g.party.Reserve[reserve]
	if outgoing == nil || incoming == nil {
		return
	}
	if names := rosterImportantItems(outgoing); len(names) > 0 {
		g.pendingRosterSwap = &rosterSwapRequest{active, reserve, outgoing, incoming, names}
		return
	}
	g.swapRosterMember(active, reserve)
}

func (g *MMGame) resolveRosterSwap(confirm bool) {
	r := g.pendingRosterSwap
	g.pendingRosterSwap = nil
	if !confirm || r == nil || g.party == nil || r.active >= len(g.party.Members) || r.reserve >= len(g.party.Reserve) ||
		g.party.Members[r.active] != r.outgoing || g.party.Reserve[r.reserve] != r.incoming {
		return
	}
	// New important goods require a freshly displayed warning. Removing goods
	// does not invalidate permission to bench the same hero with fewer items.
	for _, name := range rosterImportantItems(r.outgoing) {
		if !slices.Contains(r.items, name) {
			g.requestRosterSwap(r.active, r.reserve)
			return
		}
	}
	g.swapRosterMember(r.active, r.reserve)
}
