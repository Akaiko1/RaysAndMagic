package game

import (
	"image/color"
	"strings"

	"ugataima/internal/config"
)

var equipmentBenefitColor = color.RGBA{120, 225, 135, 255}

const activeSetPrefix = "[ACTIVE] "

func highlightActiveSetLines(text, setKey string) string {
	lines := strings.Split(text, "\n")
	for _, setLine := range config.EquipmentSetLines(setKey) {
		for i, line := range lines {
			if strings.TrimSpace(line) == setLine {
				lines[i] = strings.Replace(line, setLine, activeSetPrefix+setLine, 1)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func activeSetBonusColors(lines []string, base []color.Color) []color.Color {
	var out []color.Color
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), activeSetPrefix) {
			continue
		}
		if out == nil {
			out = tooltipLineColors(len(lines), base)
		}
		out[i] = equipmentBenefitColor
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
