package game

import (
	"image/color"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

const equipmentSetSectionTitle = character.CardSectionSet

var equipmentBenefitColor = color.RGBA{120, 225, 135, 255}

func equipmentSetTooltipLines(key string, bearer *character.MMCharacter) []string {
	lines := config.EquipmentSetLines(key)
	if bearer != nil && len(lines) > 0 {
		count, required := bearer.EquipmentSetProgress(key)
		lines[0] = uitext.Text("item.set_equipped", config.GetItemSet(key).Name, count, required)
	}
	return lines
}

// queueItemTooltip keeps set activation in presentation colors, never in text.
func (ui *UISystem) queueItemTooltip(rows character.CardRows, item items.Item, bearer *character.MMCharacter, x, y int) {
	plate, titleText := ui.itemTitleColors(item)
	var colors []color.Color
	colors = activeSetBonusRowColors(rows, colors, item, bearer)
	ui.queueCardTooltip(rows, colors, plate, titleText, itemTooltipIconName(item), x, y)
}

func activeSetBonusRowColors(rows character.CardRows, base []color.Color, item items.Item, bearer *character.MMCharacter) []color.Color {
	if bearer == nil || item.InstanceID == 0 || item.Set == "" || !bearer.HasCompletedEquipmentSet(item.Set) {
		return base
	}
	equipped := false
	for _, worn := range bearer.Equipment {
		if worn.InstanceID == item.InstanceID {
			equipped = true
			break
		}
	}
	if !equipped {
		return base
	}
	var out []color.Color
	for i, row := range rows {
		if row.Section == equipmentSetSectionTitle && row.Kind == character.CardRowResult {
			if out == nil {
				out = tooltipColorOverrides(len(rows), base, nil)
			}
			out[i] = equipmentBenefitColor
		}
	}

	if out == nil {
		return base
	}
	return out
}

// tooltipColorOverrides preserves existing overrides. A nil default lets
// typed row styles supply the ink; comparison cards retain their neutral ink.
func tooltipColorOverrides(count int, base []color.Color, defaultInk color.Color) []color.Color {
	out := make([]color.Color, count)
	if defaultInk != nil {
		for i := range out {
			out[i] = defaultInk
		}
	}
	copy(out, base)
	return out
}
