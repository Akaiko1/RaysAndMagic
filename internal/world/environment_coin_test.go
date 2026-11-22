package world

import "testing"

// A layout seed must decide each tile on its own: with a weak mix one seed
// only flipped every coin of the map together.
func TestEnvironmentCoinIsIndependentPerTile(t *testing.T) {
	const side = 16
	grid := func(seed uint64) (coins [side * side]bool, heads int) {
		w := &World3D{environmentSpriteSeed: seed}
		for y := 0; y < side; y++ {
			for x := 0; x < side; x++ {
				coins[y*side+x] = w.EnvironmentCoin(x, y)
				if coins[y*side+x] {
					heads++
				}
			}
		}
		return coins, heads
	}
	a, headsA := grid(1)
	b, _ := grid(2)
	if headsA < side*side/4 || headsA > side*side*3/4 {
		t.Fatalf("%d heads of %d: the coin is not close to even", headsA, side*side)
	}
	same := 0
	for i := range a {
		if a[i] == b[i] {
			same++
		}
	}
	if same < side*side/4 || same > side*side*3/4 {
		t.Fatalf("two seeds agree on %d of %d tiles: one layout is a copy or a mirror of the other", same, side*side)
	}
	if (*World3D)(nil).EnvironmentCoin(3, 4) {
		t.Fatal("a missing world must give a fixed side")
	}
}
