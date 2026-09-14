package game

import (
	"math"
	"strings"
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
)

func TestElementalMonsterInspectionUsesRuntimeContext(t *testing.T) {
	for _, state := range []string{"visible", "wall", "dead", "champion", "modal", "hud"} {
		t.Run(state, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			elementalTestBiome(t, cs, "water")
			g := cs.game
			g.config.MonsterCombat.ElementalAttack = config.ElementalAttackConfig{Chance: .37, DamageMultiplier: 3}
			m := monster.NewMonster3DFromConfig(0, 0, "goblin", g.config)
			m.TrueDamage = 7
			ui := NewUISystem(g)
			r := &Renderer{game: g, unifiedSprites: []UnifiedSpriteRenderData{{monster: m, screenXF: 300, sizeF: 100, bottomF: 300, depthPerp: 10}}}
			g.gameLoop = &GameLoop{game: g, renderer: r, ui: ui}
			g.depthBuffer = make([]float64, g.config.GetScreenWidth())
			for i := range g.depthBuffer {
				g.depthBuffer[i] = math.Inf(1)
			}
			x, y := 300, 250
			switch state {
			case "wall":
				for i := range g.depthBuffer {
					g.depthBuffer[i] = 5
				}
			case "dead":
				m.HitPoints = 0
			case "champion":
				m.ChampionKey = "weapon_master"
			case "modal":
				g.mainMenuOpen = true
			case "hud":
				y = gameplayViewportBottom(g)
			}
			ui.queueMonsterInspection(x, y)
			if state != "visible" {
				if ui.tooltipLines != nil {
					t.Fatal("hidden/blocked monster exposed inspection")
				}
				return
			}
			joined := strings.Join(ui.tooltipLines, "\n")
			for _, want := range []string{"Melee: Physical", "Elemental Attack: 37%, x3 raw melee damage (water)", "True damage: 7"} {
				if !strings.Contains(joined, want) {
					t.Fatalf("missing %q in %s", want, joined)
				}
			}
			for _, c := range joined {
				if c > 127 {
					t.Fatal("non-ASCII rendered tooltip")
				}
			}
			for _, line := range g.MonsterCombatEffectLines(m) {
				if !strings.Contains(joined, line.Text) {
					t.Fatalf("game dropped shared line %s", line.Text)
				}
			}
		})
	}
}
