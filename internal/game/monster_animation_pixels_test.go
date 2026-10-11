//go:build !race

package game

import (
	"slices"
	"testing"

	"ugataima/internal/graphics"
	"ugataima/internal/monster"
)

// Decode the real sheets as well as their headers: corrupt pixels and wrong
// frame sizes must fail CI. PNG decompression is serial asset validation, not
// a concurrency test; CI runs this sweep without -race, while the streaming
// loader and TestMonsterAnimationAssets still run with the detector enabled.
func TestMonsterAnimationPixels(t *testing.T) {
	g := deathTestGame(t)
	keys := catalogMonsterKeys()
	slices.Sort(keys)
	seen := make(map[graphics.SpriteResourceRequest]bool)
	for _, key := range keys {
		m := monster.NewMonster3DFromConfig(224, 224, key, g.config)
		plan := mapRenderPrewarmPlan{monsterSprites: []mapMonsterPrewarmResource{{key: key, spriteName: m.GetSpriteType()}}}
		for _, req := range mapRenderSourceRequests(plan) {
			if req.AnimationType == "" || seen[req] {
				continue
			}
			seen[req] = true
			if g.sprites.AnimationFrameCount(req.Name, req.AnimationType) == 0 {
				continue // Unauthored facing; presence is checked by the wiring test.
			}
			t.Run(req.Name+"/"+req.AnimationType, func(t *testing.T) {
				a := g.sprites.GetAnimation(req.Name, req.AnimationType)
				if a == nil || len(a.Frames) != 4 {
					t.Fatal("cannot decode four-frame animation")
				}
				for _, frame := range a.Frames {
					if frame.Bounds().Dx() != 512 || frame.Bounds().Dy() != 512 {
						t.Fatal("changed logical frame size")
					}
				}
			})
		}
		if !m.IsFish() && (m.DamageMax > 0 || m.HasRangedAttack()) {
			g.armMonsterAttackAnimation(m)
			if m.AttackAnimFrames != animationDurationFrames(g.config.GetTPS(), AuthoredMonsterAttackFPS, 4) {
				t.Fatalf("%s: authored attack did not receive the full animation window", key)
			}
		}
	}
}
