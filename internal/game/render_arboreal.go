package game

import (
	"math"

	"ugataima/internal/monster"

	"github.com/hajimehoshi/ebiten/v2"
)

var arborealAnimations = []string{"climbing", "jumping", "descending", "perched"}

func monsterSpecialAnimations(key string) []string {
	if monster.MonsterConfig != nil {
		if def, ok := monster.MonsterConfig.Monsters[key]; ok {
			if def.Disposition == "fish" {
				return []string{"leaping"}
			}
			if def.Arboreal != nil {
				return arborealAnimations
			}
		}
	}
	return nil
}

func (r *Renderer) specialMotionSprite(m *monster.Monster3D, standee bool) (*ebiten.Image, bool) {
	kind, progress := m.SpecialMotionAnimation()
	if kind == "" {
		return nil, false
	}
	name := m.GetSpriteType()
	anim, flip := r.getMonsterDirectionalAnimation(name, m, kind)
	if standee {
		anim, flip = r.game.sprites.GetAnimation(name, kind+"_r"), false
		if anim == nil {
			anim, flip = r.game.sprites.GetAnimation(name, kind+"_l"), true
		}
	}
	if anim == nil || len(anim.Frames) == 0 {
		return nil, false
	}
	i := min(len(anim.Frames)-1, int(math.Max(0, progress)*float64(len(anim.Frames))))
	return anim.Frames[i], flip
}

func arborealBottom(ground, pixelsPerTile, height float64) float64 {
	return ground - pixelsPerTile*math.Max(0, height)
}
