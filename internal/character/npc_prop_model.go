package character

import (
	"fmt"
	"math"

	"gopkg.in/yaml.v3"
)

// NPCPropModel is visual authoring only. Visited still owns consumption,
// rewards and persistence; animation never advances gameplay state.
type NPCPropModel struct {
	LightRadiusTiles float64 `yaml:"light_radius_tiles,omitempty"`
	LightIntensity   float64 `yaml:"light_intensity,omitempty"`
	Shape            string  `yaml:"shape"`
	Body             [3]int  `yaml:"body"`
	Trim             [3]int  `yaml:"trim"`
	YawDegrees       float64 `yaml:"yaw_degrees"`
	UseSeconds       float64 `yaml:"use_seconds"`
}

func validateNPCPropModel(key string, npc *NPCData) error {
	if npc == nil || npc.PropModel == nil {
		return nil
	}
	p := npc.PropModel
	if npc.Type != NPCTypeLootCrate || npc.RenderCategory != "scenery" || npc.HideWhenVisited || npc.GridSpanTiles != 0 {
		return fmt.Errorf("NPC %q: prop_model requires visible scenery of type loot_crate in both visit states", key)
	}
	switch p.Shape {
	case "crates", "campfire":
	default:
		return fmt.Errorf("NPC %q: unknown prop_model.shape %q", key, p.Shape)
	}
	if math.IsNaN(p.UseSeconds) || math.IsInf(p.UseSeconds, 0) || p.UseSeconds <= 0 {
		return fmt.Errorf("NPC %q: prop_model.use_seconds must be finite and positive, got %g", key, p.UseSeconds)
	}
	if math.IsNaN(p.YawDegrees) || math.IsInf(p.YawDegrees, 0) {
		return fmt.Errorf("NPC %q: prop_model.yaw_degrees must be finite", key)
	}
	if math.IsNaN(p.LightRadiusTiles) || math.IsInf(p.LightRadiusTiles, 0) || p.LightRadiusTiles < 0 || math.IsNaN(p.LightIntensity) || math.IsInf(p.LightIntensity, 0) || p.LightIntensity < 0 || p.LightIntensity > 1 || (p.LightRadiusTiles == 0) != (p.LightIntensity == 0) {
		return fmt.Errorf("NPC %q: prop_model light requires a finite positive radius in tiles and intensity in (0,1], or both zero", key)
	}
	for _, rgb := range [][3]int{p.Body, p.Trim} {
		for _, c := range rgb {
			if c < 0 || c > 255 {
				return fmt.Errorf("NPC %q: prop_model colors must be RGB bytes", key)
			}
		}
	}
	return nil
}

// RGB zero is valid black; omission is not an authored color. Check presence
// and channel count during YAML decoding, leaving value ranges to validation.
func (p *NPCPropModel) UnmarshalYAML(node *yaml.Node) error {
	type plain NPCPropModel
	var value plain
	if err := node.Decode(&value); err != nil {
		return err
	}
	var colors struct {
		Body *[]int `yaml:"body"`
		Trim *[]int `yaml:"trim"`
	}
	if err := node.Decode(&colors); err != nil {
		return err
	}
	for name, rgb := range map[string]*[]int{"body": colors.Body, "trim": colors.Trim} {
		if rgb == nil || len(*rgb) != 3 {
			return fmt.Errorf("prop_model.%s requires exactly three RGB channels", name)
		}
	}
	*p = NPCPropModel(value)
	return nil
}
