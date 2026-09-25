package character

import uitext "ugataima/assets/text"

// AvailabilityLines describes authored world-presence gates for content tools.
func (n *NPCData) AvailabilityLines() []string {
	if n == nil || n.MinPartyLevel <= 0 {
		return nil
	}
	return []string{uitext.Text("npc.unlock_party_level", n.MinPartyLevel)}
}
