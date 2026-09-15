package game

// updateInterfacePresentation is the single interface tick for gameplay,
// paused/loading frames and the FX editor. Keep every Card lifetime beside the
// clock its drawing code reads; render-only frames never call this method.
func (g *MMGame) updateInterfacePresentation() {
	g.uiFrameCount++
	if g.gameplayPausedByOverlay() {
		g.tickPausedAchievementBanner()
	}
	for fx := range g.cardFxTimers {
		for i := range g.cardFxTimers[fx] {
			if g.cardFxTimers[fx][i] > 0 {
				g.cardFxTimers[fx][i]--
			}
		}
	}
}

// updateWorldPresentation advances simulation-bound visuals before actors and
// projectiles. Its caller has already passed the world's pause/loading gates.
// Card feedback belongs to updateInterfacePresentation, not this clock.
func (gl *GameLoop) updateWorldPresentation() {
	// Renderer-owned ambient motes still advance in Update, never Draw: their
	// lifecycle and RNG therefore follow simulation ticks even on dropped frames.
	if gl.renderer != nil {
		gl.renderer.updateNightMotes()
	}

	// Screen shake decays exponentially toward rest.
	if gl.game.screenShake > 0 {
		gl.game.screenShake *= 0.88
		if gl.game.screenShake < 0.05 {
			gl.game.screenShake = 0
		}
	}

	// Buff-cast overlay animations age out.
	gl.game.tickElementalAttackFX()
	gl.game.tickBuffFx()

	// Quest banners: pick up whatever the journal did this frame and age the one
	// on screen.
	gl.game.tickScreenBanners()

	// Impact light flashes burn down and expire.
	if len(gl.game.impactLights) > 0 {
		gl.game.hitEffectsMu.Lock()
		dst := gl.game.impactLights[:0]
		for _, il := range gl.game.impactLights {
			il.Life--
			if il.Life > 0 {
				dst = append(dst, il)
			}
		}
		gl.game.impactLights = dst
		gl.game.hitEffectsMu.Unlock()
	}
}
