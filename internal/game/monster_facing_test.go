package game

import (
	"math"
	"testing"

	monsterPkg "ugataima/internal/monster"
)

func TestFaceMonstersAlongFrameMotionUsesActualDisplacement(t *testing.T) {
	m := &monsterPkg.Monster3D{
		X:         10,
		Y:         5,
		Direction: math.Pi,
		HitPoints: 1,
	}
	start := []monsterFramePosition{{
		monster: m,
		x:       0,
		y:       5,
	}}

	(&GameLoop{}).faceMonstersAlongFrameMotion(start)

	if math.Abs(m.Direction) > 0.0001 {
		t.Fatalf("direction = %.4f, want 0 for eastward movement", m.Direction)
	}
}

func TestFaceMonstersAlongFrameMotionIgnoresTinyJitter(t *testing.T) {
	const original = math.Pi / 2
	m := &monsterPkg.Monster3D{
		X:         0.1,
		Y:         0,
		Direction: original,
		HitPoints: 1,
	}
	start := []monsterFramePosition{{
		monster: m,
		x:       0,
		y:       0,
	}}

	(&GameLoop{}).faceMonstersAlongFrameMotion(start)

	if m.Direction != original {
		t.Fatalf("direction = %.4f, want unchanged %.4f", m.Direction, original)
	}
}
