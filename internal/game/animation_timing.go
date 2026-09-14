package game

import (
	"ugataima/internal/config"
	"ugataima/internal/monster"
)

func animationTicksPerFrame(tps, fps int) int {
	if tps <= 0 {
		tps = config.DefaultTPS
	}
	if fps <= 0 {
		return 1
	}
	ticks := tps / fps
	if ticks < 1 {
		return 1
	}
	return ticks
}

func animationDurationFrames(tps, fps, frameCount int) int {
	if frameCount <= 0 {
		return 0
	}
	return animationTicksPerFrame(tps, fps) * frameCount
}

func (g *MMGame) authoredMonsterAttackFrameCount(mon *monster.Monster3D) int {
	if g == nil || g.sprites == nil || mon == nil {
		return 0
	}
	name := mon.GetSpriteType()
	maxFrames := 0
	for _, animType := range []string{"attacking_r", "attacking_l"} {
		if anim := g.sprites.GetAnimation(name, animType); anim != nil && len(anim.Frames) > maxFrames {
			maxFrames = len(anim.Frames)
		}
	}
	return maxFrames
}

func (g *MMGame) monsterAttackAnimationDuration(mon *monster.Monster3D) int {
	frameCount := g.authoredMonsterAttackFrameCount(mon)
	if frameCount == 0 {
		return MonsterAttackAnimFrames
	}
	tps := config.DefaultTPS
	if g != nil && g.config != nil {
		tps = g.config.GetTPS()
	}
	return animationDurationFrames(tps, AuthoredMonsterAttackFPS, frameCount)
}

func (g *MMGame) armMonsterAttackAnimation(mon *monster.Monster3D) {
	if mon == nil {
		return
	}
	mon.AttackAnimFrames = g.monsterAttackAnimationDuration(mon)
}
