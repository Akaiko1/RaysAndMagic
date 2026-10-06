package game

import (
	"fmt"
	"math"

	"ugataima/internal/config"

	"github.com/hajimehoshi/ebiten/v2"
)

// Spell materials are selected by graphics.projectile_fx in spells.yaml.
// Every entry owns its silhouette; side and end-on views share one shader.

// spellFxStyleDraw maps graphics.projectile_fx to its bespoke renderer.
var spellFxStyleDraw = map[string]func(*Renderer, *ebiten.Image, float64, float64, float64, float64, float64, [3]int, projectileFxProfile, float64, int){
	"fireball":     spellMaterialRenderer(spellFireball),
	"lightning":    spellMaterialRenderer(spellLightning),
	"harm":         spellMaterialRenderer(spellHarm),
	"psyshock":     spellMaterialRenderer(spellPsyshock),
	"starburst":    spellMaterialRenderer(spellStarburst),
	"disintegrate": spellMaterialRenderer(spellDisintegrate),
	"ray_of_light": spellMaterialRenderer(spellLight),
	"rock":         spellMaterialRenderer(spellRock),
	"stone_bud":    spellMaterialRenderer(spellStoneBud),
	"swarm":        spellMaterialRenderer(spellSwarm),
	"ice_shard":    spellMaterialRenderer(spellIce),
	"fire_dart":    spellMaterialRenderer(spellFireDart),
	"sparks":       spellMaterialRenderer(spellSparks),
	"shadow_bolt":  spellMaterialRenderer(spellShadow),
	"void_needle":  spellMaterialRenderer(spellVoidNeedle),
	"shackle":      spellMaterialRenderer(spellShackle),
	"charm_bloom":  spellMaterialRenderer(spellCharm),
	"psi_lance":    spellMaterialRenderer(spellPsiLance),
	"lash":         spellMaterialRenderer(spellLash),
}

// validateProjectileFxStyles fails fast on a projectile_fx naming a style with
// no renderer - a YAML typo would silently fall back to the school default.
func validateProjectileFxStyles() {
	if config.GlobalSpells == nil {
		return
	}
	for key, def := range config.GlobalSpells.Spells {
		if def.Graphics == nil {
			continue
		}
		if def.Graphics.ProjectileFx != "" {
			if _, ok := spellFxStyleDraw[def.Graphics.ProjectileFx]; !ok {
				panic(fmt.Sprintf("spell %q: unknown projectile_fx style %q", key, def.Graphics.ProjectileFx))
			}
		}
		if def.Graphics.NovaFx != "" {
			if _, ok := novaFxSpawn[def.Graphics.NovaFx]; !ok {
				panic(fmt.Sprintf("spell %q: unknown nova_fx style %q", key, def.Graphics.NovaFx))
			}
		}
	}
}

// frac returns the fractional part - the loop clock for cycling particles.
func frac(v float64) float64 { return v - math.Floor(v) }
