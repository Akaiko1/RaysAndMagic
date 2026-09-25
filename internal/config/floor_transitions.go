package config

import "fmt"

// FloorTransition controls appearance only; it never changes tile collision.
type FloorTransition string

const (
	FloorTransitionHard      FloorTransition = "hard"
	FloorTransitionNatural   FloorTransition = "natural"
	FloorTransitionWater     FloorTransition = "water"
	FloorTransitionCliffEast FloorTransition = "cliff_east"
	FloorTransitionCliffWest FloorTransition = "cliff_west"
	FloorTransitionVoid      FloorTransition = "void"
)

func (b BiomeConfig) ValidateFloorTransitions() error {
	for group, profile := range b.FloorTransitions {
		if len(b.FloorTextureGroups[group]) == 0 {
			return fmt.Errorf("floor_transitions group %q has no textures", group)
		}
		switch profile {
		case FloorTransitionHard, FloorTransitionNatural, FloorTransitionWater,
			FloorTransitionCliffEast, FloorTransitionCliffWest, FloorTransitionVoid:
		default:
			return fmt.Errorf("floor_transitions group %q has unknown profile %q", group, profile)
		}
	}
	return nil
}
