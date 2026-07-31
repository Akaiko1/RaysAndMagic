package game

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/graphics"
	"ugataima/internal/monster"
)

func TestAnimationTimingAtDefaultTPS(t *testing.T) {
	const tps = 120

	if got, want := animationTicksPerFrame(tps, NPCIdleAnimationFPS), 30; got != want {
		t.Fatalf("NPC idle ticks per frame = %d, want %d", got, want)
	}
	if got, want := animationDurationFrames(tps, AuthoredMonsterAttackFPS, 4), 48; got != want {
		t.Fatalf("four-frame authored attack duration = %d, want %d", got, want)
	}
}

func TestMonsterAttackTimingOnlySlowsAuthoredSheets(t *testing.T) {
	t.Chdir("../..")
	monster.MustLoadMonsterConfig("assets/monsters.yaml")

	g := &MMGame{
		config:  &config.Config{Engine: config.EngineConfig{TPS: 120}},
		sprites: graphics.NewSpriteManager(),
	}

	authored := &monster.Monster3D{Key: "weapon_master"}
	if got, want := g.authoredMonsterAttackFrameCount(authored), 4; got != want {
		t.Fatalf("weapon master attack frame count = %d, want %d", got, want)
	}
	g.armMonsterAttackAnimation(authored)
	if got, want := authored.AttackAnimFrames, 48; got != want {
		t.Fatalf("authored attack duration = %d, want %d", got, want)
	}

	fallback := &monster.Monster3D{Key: "dire_wolf"}
	if got := g.authoredMonsterAttackFrameCount(fallback); got != 0 {
		t.Fatalf("dire wolf unexpectedly has %d authored attack frames", got)
	}
	g.armMonsterAttackAnimation(fallback)
	if got, want := fallback.AttackAnimFrames, MonsterAttackAnimFrames; got != want {
		t.Fatalf("walking-sheet fallback duration = %d, want unchanged %d", got, want)
	}
}
