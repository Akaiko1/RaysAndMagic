package game

import (
	"fmt"
	"image/color"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

const equipmentSetSectionTitle = "SET"

var equipmentBenefitColor = color.RGBA{120, 225, 135, 255}

func equipmentSetTooltipLines(key string, bearer *character.MMCharacter) []string {
	lines := config.EquipmentSetLines(key)
	if bearer != nil && len(lines) > 0 {
		count, required := bearer.EquipmentSetProgress(key)
		lines[0] = fmt.Sprintf("Set: %s (%d/%d equipped)", config.GetItemSet(key).Name, count, required)
	}
	return lines
}

// queueItemTooltip keeps set activation in presentation colors, never in text.
func (ui *UISystem) queueItemTooltip(lines []string, item items.Item, bearer *character.MMCharacter, x, y int) {
	plate, titleText := ui.itemTitleColors(item)
	var colors []color.Color
	colors = activeSetBonusColors(lines, colors, item, bearer)
	ui.queueTitledTooltipIcon(lines, colors, plate, titleText, itemTooltipIconName(item), x, y)
}

func activeSetBonusColors(lines []string, base []color.Color, item items.Item, bearer *character.MMCharacter) []color.Color {
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
	inSet := false
	for i, line := range lines {
		text := strings.TrimSpace(line)
		if text == equipmentSetSectionTitle {
			inSet = true
			continue
		}
		if text == "" {
			inSet = false
		}
		if inSet {
			if out == nil {
				out = tooltipLineColors(len(lines), base)
			}
			out[i] = equipmentBenefitColor
		}
	}

	if out == nil {
		return base
	}
	return out
}

// tooltipLineColors preserves existing line styles and supplies the default
// only for new lines. Callers can override their own semantic highlights.
func tooltipLineColors(count int, base []color.Color) []color.Color {
	out := make([]color.Color, count)
	for i := range out {
		out[i] = color.White
	}
	copy(out, base)
	return out
}
