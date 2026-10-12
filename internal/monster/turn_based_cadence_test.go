package monster

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The rate is 1/cooldown (x 1/enrage cooldown while enraged), never below one
// swing and never above the cap; champions keep their fixed rule.
func TestTurnBasedAttackRate(t *testing.T) {
	for _, c := range []struct {
		name                     string
		champion, enraged        bool
		cooldown, enrageCooldown float64
		want                     float64
	}{
		{"unset speed", false, false, 0, 0, 1},
		{"default speed", false, false, 1, 0, 1},
		{"faster", false, false, 0.7, 0, 1 / 0.7},
		{"slower stays at one swing", false, false, 1.8, 0, 1},
		{"capped", false, false, 0.05, 0, MaxTurnBasedSwings},
		{"enrage authored but not enraged", false, false, 0.5, 0.7, 2},
		{"enraged", false, true, 0.5, 0.7, 2 / 0.7},
		{"enraged without an enrage speed-up", false, true, 0.5, 0, 2},
		{"champion ignores cooldown and enrage", true, true, 0.1, 0.5, 2},
	} {
		if got := TurnBasedAttackRate(c.champion, c.cooldown, c.enrageCooldown, c.enraged); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: rate = %v, want %v", c.name, got, c.want)
		}
	}
}

// Over many actions the delivered swings match the rate exactly (the carry
// never drifts), and the editor's preview is the sequence combat deals.
func TestTurnBasedSwingsMatchRateAndEditorPreview(t *testing.T) {
	const actions = 1000
	for _, cooldown := range []float64{1, 0.85, 0.75, 0.7, 0.65, 0.6, 0.55, 0.5, 0.3, 0.25} {
		m := &Monster3D{AttackCooldownMultiplier: cooldown}
		rate := m.TurnBasedAttackRate()
		var dealt []int
		total := 0
		for i := 0; i < actions; i++ {
			n := m.TakeTurnBasedSwings()
			if n < 1 || n > MaxTurnBasedSwings {
				t.Fatalf("cooldown %.2f: action %d swung %d times", cooldown, i, n)
			}
			if m.TurnBasedSwingCarry < -0.5 || m.TurnBasedSwingCarry >= 0.5 {
				t.Fatalf("cooldown %.2f: carry %v left [-0.5, 0.5)", cooldown, m.TurnBasedSwingCarry)
			}
			dealt = append(dealt, n)
			total += n
		}
		if math.Abs(float64(total)-rate*actions) > 0.5 {
			t.Errorf("cooldown %.2f: %d swings over %d actions, want %.1f", cooldown, total, actions, rate*actions)
		}
		if preview := TurnBasedSequence(rate, 20); !slices.Equal(preview, dealt[:20]) {
			t.Errorf("cooldown %.2f: editor preview %v differs from combat %v", cooldown, preview, dealt[:20])
		}
	}
}

// attacks_per_round is gone: content that still authors it fails the load
// instead of being silently ignored.
func TestRemovedAttacksPerRoundFailsTheLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monsters.yaml")
	yaml := "monsters:\n  probe:\n    name: Probe\n    attacks_per_round: 2\n"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMonsterConfig(path); err == nil || !strings.Contains(err.Error(), "attacks_per_round") {
		t.Fatalf("load error = %v, want one naming attacks_per_round", err)
	}
}

// Steps are speed / step speed: at least one for a walking monster, none for
// a stationary one, capped at four. Ambient monsters have no floor.
func TestTurnBasedStepRate(t *testing.T) {
	for _, c := range []struct {
		name    string
		speed   float64
		ambient bool
		want    float64
	}{
		{"stationary", 0, false, 0},
		{"slow walks one", 1, false, 1},
		{"step speed walks one", 2, false, 1},
		{"faster banks a fraction", 2.4, false, 1.2},
		{"capped", 20, false, MaxTurnBasedSteps},
		{"ambient stroller may idle", 0.7, true, 0.35},
		{"ambient fast walker", 3.2, true, 1.6},
		{"ambient stationary", 0, true, 0},
	} {
		if got := TurnBasedStepRate(c.speed, 2, c.ambient); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: rate = %v, want %v", c.name, got, c.want)
		}
	}
}

// WalkTurn draws the action's steps on the first landed step: a blocked first
// step spends nothing, an idle ambient turn still banks its fraction, and a
// false between ends the walk.
func TestWalkTurnSpendsOnTheFirstLandedStep(t *testing.T) {
	walk := func(m *Monster3D, moves bool, between func() bool) int {
		taken := 0
		m.WalkTurn(func(bool) bool {
			if moves {
				taken++
			}
			return moves
		}, between)
		return taken
	}
	m := &Monster3D{Speed: 3}
	if walk(m, false, nil) != 0 || m.TurnBasedStepCarry != 0 {
		t.Fatalf("blocked action spent the carry: %v", m.TurnBasedStepCarry)
	}
	if got := walk(m, true, nil); got != 2 {
		t.Fatalf("free action took %d steps, want 2 at rate 1.5", got)
	}
	if got := walk(m, true, func() bool { return false }); got != 1 {
		t.Fatalf("a stopping hazard let %d steps through, want 1", got)
	}
	stroller := &Monster3D{Speed: 0.7, Disposition: DispositionCaravan}
	var steps []int
	for i := 0; i < 5; i++ {
		steps = append(steps, walk(stroller, true, nil))
	}
	if want := []int{0, 1, 0, 0, 1}; !slices.Equal(steps, want) {
		t.Fatalf("ambient 0.35 rate steps = %v, want %v", steps, want)
	}
}
