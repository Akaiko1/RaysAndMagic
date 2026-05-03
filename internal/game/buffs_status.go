package game

import (
	"ugataima/internal/spells"
)

// UtilitySpellStatus tracks active utility spell icons and durations for the UI.
type UtilitySpellStatus struct {
	SpellID     spells.SpellID
	Icon        string
	Fallback    string
	Label       string
	Duration    int
	MaxDuration int
}

func (g *MMGame) setUtilityStatus(spellID spells.SpellID, duration int) {
	if duration <= 0 {
		return
	}
	def, err := spells.GetSpellDefinitionByID(spellID)
	if err != nil || def.StatusIcon == "" {
		return
	}
	if g.utilitySpellStatuses == nil {
		g.utilitySpellStatuses = make(map[spells.SpellID]*UtilitySpellStatus)
	}
	icon, fallback := resolveStatusIcon(def.StatusIcon)
	status, exists := g.utilitySpellStatuses[spellID]
	if !exists {
		status = &UtilitySpellStatus{SpellID: spellID}
		g.utilitySpellStatuses[spellID] = status
	}
	status.Icon = icon
	status.Fallback = fallback
	status.Label = def.Name
	status.Duration = duration
	status.MaxDuration = duration
}

func (g *MMGame) updateUtilityStatus(spellID spells.SpellID, duration int, active bool) {
	if !active || duration <= 0 {
		if g.utilitySpellStatuses != nil {
			delete(g.utilitySpellStatuses, spellID)
		}
		return
	}
	if g.utilitySpellStatuses == nil {
		g.setUtilityStatus(spellID, duration)
		return
	}
	status, exists := g.utilitySpellStatuses[spellID]
	if !exists {
		g.setUtilityStatus(spellID, duration)
		return
	}
	status.Duration = duration
	if duration > status.MaxDuration {
		status.MaxDuration = duration
	}
}

func resolveStatusIcon(token string) (string, string) {
	switch token {
	case "torch":
		return "💡", "TL"
	case "eye":
		return "👁", "WE"
	case "water_walk":
		return "🌊", "WW"
	case "water_breathing":
		return "🫧", "WB"
	case "bless":
		return "✨", "BL"
	default:
		return token, token
	}
}
