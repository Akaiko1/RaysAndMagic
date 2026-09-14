package game

import (
	"testing"
	"time"
)

// Drive the real game/preview entry points. Inspect several independent effect
// families so a clock-only fix cannot leave a forgotten lifetime frozen.
func TestPresentationEntryPoints(t *testing.T) {
	for _, entry := range []string{"game_rt", "game_tb", "overlay", "loading", "focus_loading", "redraw", "after_input", "preview", "entry_menu"} {
		t.Run(entry, func(t *testing.T) {
			cfg := setupPreviewSandboxTest(t)
			p, err := NewFxPreview(cfg)
			if err != nil {
				t.Fatal(err)
			}
			g, gl := p.g, p.g.gameLoop
			defer g.Shutdown()
			t.Chdir("../..")
			g.appScreen = AppScreenInGame
			g.audioSliderDrag = -1
			p.Select(FxItem{Kind: FxCard, Key: "heal"})
			g.frameCount, g.uiFrameCount = 100, 30
			for fx := range g.cardFxTimers {
				g.cardFxTimers[fx][0] = 8
			}
			g.screenShake = 2
			g.buffFxAnims = []buffFxAnim{{sprite: "test", age: 0}}
			g.elementalAttackEffects = []elementalAttackEffect{{Frames: 10}}
			g.impactLights = []ImpactLight{{Life: 20}}
			g.slashEffects = []SlashEffect{{Active: true, MaxFrames: 20}}
			g.spellHitEffects = []SpellHitEffect{{Active: true, Particles: []SpellHitParticle{{Active: true, LifeTime: 20, MaxLife: 20}}}}
			g.persistentDamageZones = []PersistentDamageZone{{SpellID: "hot_steam", MapKey: fxStageMapKey, FramesLeft: 50, IntervalFrames: 30}}
			g.spellInputCooldown = 10
			projectileX := 5.5 * cfg.GetTileSize()
			g.magicProjectiles = []MagicProjectile{{ID: "presentation-test", X: projectileX, Y: 8.5 * cfg.GetTileSize(), VelX: 1, LifeTime: 20, Active: true, SpellType: "firebolt", Owner: ProjectileOwnerPlayer}}
			step := gl.updateExploration
			running := entry == "game_rt" || entry == "game_tb" || entry == "preview"
			switch entry {
			case "game_tb":
				g.ToggleTurnBasedMode()
			case "overlay":
				g.menuOpen = true
				gl.ui.renderedModalSnapshot = gl.ui.topModalSnapshot()
			case "loading", "focus_loading":
				gl.ensureResourceLoading()
				gl.loading.begin(time.Now())
				gl.loading.pattern = make(chan struct{})
				if entry == "loading" {
					step = func() {
						if err := gl.Update(); err != nil {
							t.Fatal(err)
						}
					}
				}
			case "redraw":
				gl.ui.renderedModalSnapshot = modalLayerSnapshot{layer: modalLayerStat}
				step = func() {
					if err := gl.Update(); err != nil {
						t.Fatal(err)
					}
				}
			case "after_input":
				g.combatLogOpen = true
				gl.ui.renderedModalSnapshot = gl.ui.topModalSnapshot()
				x, y, w, _ := combatLogPanelLayout(g)
				g.mouseLeftClicks = []queuedClick{{x: x + w - 20, y: y + 18, at: 1000}}
			case "preview":
				step = p.Step
			case "entry_menu":
				g.appScreen = AppScreenMainMenu
				gl.ui.beginDisplayedInput()
				gl.ui.endDisplayedInput()
				gl.ui.renderedModalSnapshot = gl.ui.topModalSnapshot()
				step = func() {
					if err := gl.Update(); err != nil {
						t.Fatal(err)
					}
				}
			}
			step()
			uiTicks := int64(1)
			if entry == "entry_menu" {
				uiTicks = 0
			}
			if g.uiFrameCount != 30+uiTicks {
				t.Errorf("UI clock %d, want %d", g.uiFrameCount, 30+uiTicks)
			}
			for fx := range g.cardFxTimers {
				if got := g.cardFxTimers[fx][0]; got != 8-int(uiTicks) {
					t.Errorf("Card family %d timer=%d", fx, got)
				}
			}
			worldTicks := 0
			if running {
				worldTicks = 1
			}
			if len(g.magicProjectiles) != 1 || g.magicProjectiles[0].LifeTime != 20-worldTicks || g.magicProjectiles[0].X != projectileX+float64(worldTicks) {
				t.Error("projectile update disagrees with world progress")
			}
			if g.frameCount != 100+int64(worldTicks) {
				t.Errorf("world clock=%d", g.frameCount)
			}
			if g.buffFxAnims[0].age != worldTicks || g.elementalAttackEffects[0].Age != worldTicks || g.impactLights[0].Life != 20-worldTicks {
				t.Error("ambient lifetimes disagree with world progress")
			}
			if g.slashEffects[0].AnimationFrame != worldTicks || g.spellHitEffects[0].Particles[0].LifeTime != 20-worldTicks {
				t.Error("slash/impact lifetimes disagree with world progress")
			}
			if (g.screenShake < 2) != running {
				t.Error("screen shake violates world pause")
			}
			zoneTicks := worldTicks
			if entry == "game_tb" {
				zoneTicks = 0
			}
			if got := g.persistentDamageZones[0].FramesLeft; got != 50-zoneTicks {
				t.Errorf("zone lifetime=%d, want %d (one RT update)", got, 50-zoneTicks)
			}
			if g.spellInputCooldown != 10-worldTicks {
				t.Error("presentation changed gameplay cooldown pause rules")
			}
			if entry == "after_input" && g.combatLogOpen {
				t.Error("fixture did not close modal during input")
			}
		})
	}
}
