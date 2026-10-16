package game

import (
	"image/color"
	"testing"
)

// ebiten's vector fills treat their colour as premultiplied: fading has to scale
// RGB as well as alpha, or a faint glow renders as a solid saturated block.
func TestFadeVectorColorPremultiplies(t *testing.T) {
	gold := color.RGBA{255, 215, 0, 255}
	faded := fadeVectorColor(gold, 0.2)
	if faded.A != 51 {
		t.Fatalf("alpha = %d, want 51 (0.2 of 255)", faded.A)
	}
	if faded.R > faded.A || faded.G > faded.A {
		t.Fatalf("faded %v is not premultiplied (R/G must not exceed A=%d) - this is what made the aura opaque",
			faded, faded.A)
	}
	if got := fadeVectorColor(gold, 1); got != gold {
		t.Fatalf("full strength changed the colour: %v", got)
	}
	if got := fadeVectorColor(gold, -1); got.A != 0 {
		t.Fatalf("negative fade = %v, want fully transparent", got)
	}
}

// The halo is a per-pixel falloff image, cached per (size, reach, tint) - both
// the hero card and the badges pull from the same generator.
func TestSoftGlowIsCachedAndFadesOutward(t *testing.T) {
	glow := softGlowImage(24, 24, badgeAuraSpreadPx, rarityEmerald, badgeAuraPeak)
	if glow == nil {
		t.Fatal("no glow image produced")
	}
	if again := softGlowImage(24, 24, badgeAuraSpreadPx, rarityEmerald, badgeAuraPeak); again != glow {
		t.Fatal("the glow is re-rasterized per call - it must be cached")
	}
	if other := softGlowImage(24, 24, badgeAuraSpreadPx, rarityGold, badgeAuraPeak); other == glow {
		t.Fatal("two tints share one cached image")
	}
	wantW := 24 + 2*badgeAuraSpreadPx
	if b := glow.Bounds(); b.Dx() != wantW || b.Dy() != wantW {
		t.Fatalf("glow is %dx%d, want %dx%d (box plus its reach on every side)", b.Dx(), b.Dy(), wantW, wantW)
	}
	if softGlowImage(0, 24, 4, rarityGold, 100) != nil || softGlowImage(24, 24, 0, rarityGold, 100) != nil {
		t.Fatal("a degenerate box or zero reach must produce no glow")
	}
}
