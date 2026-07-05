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
	icon, fallback := g.resolveStatusIconSprite(def.StatusIcon)
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

// resolveStatusIconSprite picks the HUD status-bar icon for a token. A known
// legacy token (bless, torch, ...) maps to its dedicated status_* sprite. For any
// other token (e.g. a spell key) it PREFERS a dedicated "status_<token>" sprite
// if one exists, otherwise falls back to the spellbook icon "icon_spell_<token>"
// (which drawSpellIcon shrinks to the bar), and finally to a text label.
func (g *MMGame) resolveStatusIconSprite(token string) (icon, fallback string) {
	icon, fallback = resolveStatusIcon(token)
	if icon != token {
		return icon, fallback // known legacy token, already mapped to a status_* sprite
	}
	if g.sprites != nil {
		if status := "status_" + token; g.sprites.HasSprite(status) {
			return status, fallback
		}
		if spellIcon := "icon_spell_" + token; g.sprites.HasSprite(spellIcon) {
			return spellIcon, fallback
		}
	}
	return icon, fallback
}

func resolveStatusIcon(token string) (string, string) {
	switch token {
	case "torch":
		return "status_torch_light", "TL"
	case "eye":
		return "status_wizard_eye", "WE"
	case "water_walk":
		return "status_walk_on_water", "WW"
	case "water_breathing":
		return "status_water_breathing", "WB"
	case "bless":
		return "status_bless", "BL"
	default:
		return token, token
	}
}
