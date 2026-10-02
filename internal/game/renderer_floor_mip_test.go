package game

import "testing"

// boxHalve is the floor atlas mip-chain step: each 2x2 block averages into
// one texel, rounded to nearest.
func TestBoxHalve(t *testing.T) {
	// 4x2 RGBA: left block all 100s, right block 0/100 checker per channel.
	px := []byte{
		100, 100, 100, 100, 100, 100, 100, 100, 0, 0, 0, 0, 100, 100, 100, 100,
		100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 0, 0, 0, 0,
	}
	out, w, h := boxHalve(px, 4, 2)
	if w != 2 || h != 1 {
		t.Fatalf("halved dims = %dx%d, want 2x1", w, h)
	}
	if out[0] != 100 || out[1] != 100 || out[2] != 100 || out[3] != 100 {
		t.Errorf("uniform block averaged to %v, want all 100", out[:4])
	}
	if out[4] != 50 || out[5] != 50 || out[6] != 50 || out[7] != 50 {
		t.Errorf("checker block averaged to %v, want all 50", out[4:8])
	}

	// Rounding to nearest: 302/4 = 75.5 rounds up (truncation gives 75),
	// 301/4 = 75.25 rounds down (ceiling gives 76).
	px2 := []byte{
		100, 100, 0, 0, 100, 100, 0, 0,
		100, 100, 0, 0, 2, 1, 0, 0,
	}
	out2, _, _ := boxHalve(px2, 2, 2)
	if out2[0] != 76 || out2[1] != 75 {
		t.Errorf("rounded averages = %d/%d, want 76/75", out2[0], out2[1])
	}
}
