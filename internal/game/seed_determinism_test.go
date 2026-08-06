package game

//lint:file-ignore SA1019 this test exists to PROVE top-level rand.Seed still works

import (
	"math/rand"
	"testing"
)

// Guards the //go:debug randseednop=0 directive in combat_balance_test.go: the
// balance sims are only reproducible while top-level seeding actually works.
func TestPackageLevelSeedingIsFunctional(t *testing.T) {
	rand.Seed(7)
	a := [3]int{rand.Intn(1000), rand.Intn(1000), rand.Intn(1000)}
	rand.Seed(7)
	b := [3]int{rand.Intn(1000), rand.Intn(1000), rand.Intn(1000)}
	if a != b {
		t.Fatalf("top-level rand.Seed is a no-op (%v vs %v) - the balance sims are not reproducible; check the //go:debug randseednop=0 directive", a, b)
	}
}
