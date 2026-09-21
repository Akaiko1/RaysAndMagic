package monster

import (
	"fmt"
	"strings"
	"testing"
)

func TestFishCannotDisappearWithoutLeapState(t *testing.T) {
	m := &Monster3D{Key: "test_fish", Disposition: DispositionFish}
	defer func() {
		failure := recover()
		if failure == nil || !strings.Contains(fmt.Sprint(failure), "test_fish") {
			t.Fatalf("missing contextual invariant failure: %v", failure)
		}
	}()
	m.AdvanceFishLeap(.1)
}

func TestFishRelationships(t *testing.T) {
	for _, attackerKind := range []string{"fish", "bound", "hostile", "wildlife"} {
		for _, targetKind := range []string{"fish", "hostile"} {
			t.Run(attackerKind+"/"+targetKind, func(t *testing.T) {
				attacker := &Monster3D{HitPoints: 10}
				target := &Monster3D{Key: "prey", HitPoints: 10}
				switch attackerKind {
				case "fish":
					attacker.Disposition = DispositionFish
				case "bound":
					attacker.Bound = true
				case "wildlife":
					attacker.Disposition = "wildlife"
					attacker.Prey = []string{"prey"}
				}
				if targetKind == "fish" {
					target.Disposition = DispositionFish
				}
				want := attackerKind == "bound" && targetKind == "hostile"
				if got := attacker.CanAttackActor(target); got != want {
					t.Fatalf("CanAttackActor=%v, want %v", got, want)
				}
			})
		}
	}
}
