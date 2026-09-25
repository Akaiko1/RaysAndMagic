package game

import (
	"ugataima/internal/config"
	"ugataima/internal/monster"
)

// Presentation state belongs to the live game loop, not the save or AI state.
// Only the capture->movement->face window can start walking. Combat action
// stamps, band formation snaps and teleports outside that window cannot.
type monsterWalkPlayback struct {
	startTick  int64
	sampleTick int64
	turnBased  bool
}

func (gl *GameLoop) recordMonsterWalkMotion(m *monster.Monster3D, oldX, oldY float64) {
	now, tb := gl.game.frameCount, gl.game.turnBasedMode
	moved := m.X != oldX || m.Y != oldY
	if tb {
		tile := gl.game.config.GetTileSize()
		moved = TileIndex(m.X, tile) != TileIndex(oldX, tile) || TileIndex(m.Y, tile) != TileIndex(oldY, tile)
	}
	state, exists := gl.monsterWalkPlayback[m]
	if !moved {
		if !tb || state.turnBased != tb {
			delete(gl.monsterWalkPlayback, m)
		} else if exists {
			state.sampleTick = now
			gl.monsterWalkPlayback[m] = state
		}
		return
	}
	if !exists || state.turnBased != tb || tb || state.sampleTick != now-1 {
		state.startTick = now
	}
	state.sampleTick, state.turnBased = now, tb
	if gl.monsterWalkPlayback == nil {
		gl.monsterWalkPlayback = make(map[*monster.Monster3D]monsterWalkPlayback)
	}
	gl.monsterWalkPlayback[m] = state
}

func (gl *GameLoop) inheritMonsterWalkPlayback(follower, leader *monster.Monster3D) {
	if state, ok := gl.monsterWalkPlayback[leader]; ok {
		gl.monsterWalkPlayback[follower] = state
	} else {
		delete(gl.monsterWalkPlayback, follower)
	}
}

// shouldAnimateMonster checks the current movement sample, never AI intent.
func (r *Renderer) shouldAnimateMonster(mon *monster.Monster3D) bool {
	if r.game == nil || r.game.gameLoop == nil || mon.AttackAnimFrames > 0 {
		return false
	}
	state, ok := r.game.gameLoop.monsterWalkPlayback[mon]
	return ok && state.turnBased == r.game.turnBasedMode && state.sampleTick == r.game.frameCount && state.startTick <= r.game.frameCount
}

func (r *Renderer) monsterWalkElapsed(mon *monster.Monster3D) (int64, bool) {
	if !r.shouldAnimateMonster(mon) {
		return 0, false
	}
	return r.game.frameCount - r.game.gameLoop.monsterWalkPlayback[mon].startTick, true
}

func (r *Renderer) monsterWalkTicksPerFrame() int {
	seconds := r.game.config.Graphics.Monster.WalkFrameSeconds
	if seconds <= 0 {
		seconds = config.DefaultMonsterWalkFrameSeconds
	}
	return r.game.framesForSeconds(seconds)
}
