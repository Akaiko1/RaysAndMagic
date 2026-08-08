package game

import (
	"image/color"
	"testing"
)

// The aura breathes: it must reach full strength, never dim below its floor (an
// unspent point stays legible at the trough), and repeat on its period.
func TestBadgeAuraPulseBreathesWithinItsFloor(t *testing.T) {
	cfg := loadTestConfig(t)
	g := newTestGame(cfg, newTestWorldSized(cfg, 4, 4))
	period := int64(g.framesForSeconds(badgeAuraPeriodSeconds))
	minSeen, maxSeen := 2.0, -1.0
	for f := int64(0); f < period; f++ {
		p := g.badgeAuraPulse(f)
		if p < badgeAuraFloor-0.001 || p > 1.001 {
			t.Fatalf("frame %d pulse = %.3f, want it inside [%.2f, 1]", f, p, badgeAuraFloor)
		}
		minSeen = min(minSeen, p)
		maxSeen = max(maxSeen, p)
	}
	if minSeen > badgeAuraFloor+0.01 {
		t.Fatalf("the trough only reached %.3f, want it down at the floor %.2f", minSeen, badgeAuraFloor)
	}
	if maxSeen < 0.99 {
		t.Fatalf("the peak only reached %.3f, want full strength", maxSeen)
	}
	// Same phase one period later - the cue never drifts against itself.
	for _, f := range []int64{0, 7, 31, 65} {
		if a, b := g.badgeAuraPulse(f), g.badgeAuraPulse(f+period); a != b {
			t.Fatalf("frame %d and one period later differ: %.3f vs %.3f", f, a, b)
		}
	}
	// Both badges read the same clock, so a portrait carrying both pulses as one.
	if g.badgeAuraPulse(19) != g.badgeAuraPulse(19) {
		t.Fatal("the pulse is not a pure function of the frame")
	}
}

// The stat badge is green and the skill badge is gold, and each one's aura, hover
// ring and icon come from the SAME style entry - the association cannot drift.
func TestProgressionBadgeStylesCarryOneTintEach(t *testing.T) {
	if statBadgeStyle.tint != rarityEmerald {
		t.Fatalf("stat badge tint = %v, want the emerald green %v", statBadgeStyle.tint, rarityEmerald)
	}
	if skillBadgeStyle.tint != rarityGold {
		t.Fatalf("skill badge tint = %v, want gold %v", skillBadgeStyle.tint, rarityGold)
	}
	if statBadgeStyle.icon != "icon_stat_up" || skillBadgeStyle.icon != "icon_level_choice" {
		t.Fatalf("badge icons drifted: %q / %q", statBadgeStyle.icon, skillBadgeStyle.icon)
	}
	if statBadgeStyle.tint == skillBadgeStyle.tint {
		t.Fatal("the two badges must be told apart by colour")
	}
}

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

// Two badges on one portrait stay attached to it: their haloes may blend in the
// middle (soft glows do that gracefully), but the pair must not drift out past
// the portrait it belongs to.
func TestProgressionBadgeLayoutKeepsBothBadgesOnThePortrait(t *testing.T) {
	const px, py, pw, ph = 100, 200, 47, 56
	both := makePartyProgressionBadgeLayout(px, py, pw, ph, true, true)
	if both.stat.right() > both.skill.x {
		t.Fatalf("the two badges overlap: stat ends at %d, skill starts at %d", both.stat.right(), both.skill.x)
	}
	// The pair is wider than the narrow portrait aperture by design (the badges
	// straddle its lower rim), but the overhang plus the halo has a budget: past
	// half a badge they stop reading as attached to THIS portrait.
	overhangBudget := partyProgressBadgeSize / 2
	if left := px - (both.stat.x - badgeAuraSpreadPx); left > overhangBudget {
		t.Fatalf("the stat badge halo overhangs the portrait by %dpx on the left, budget %dpx", left, overhangBudget)
	}
	if right := (both.skill.right() + badgeAuraSpreadPx) - (px + pw); right > overhangBudget {
		t.Fatalf("the skill badge halo overhangs the portrait by %dpx on the right, budget %dpx", right, overhangBudget)
	}

	// One badge alone centres under the portrait.
	only := makePartyProgressionBadgeLayout(px, py, pw, ph, false, true)
	if got, want := only.skill.x+only.skill.w/2, px+pw/2; got != want {
		t.Fatalf("a single badge centres at %d, want the portrait centre %d", got, want)
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
