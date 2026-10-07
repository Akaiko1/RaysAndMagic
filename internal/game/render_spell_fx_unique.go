package game

import (
	"fmt"

	"ugataima/internal/config"
)

// Spell materials are selected by graphics.projectile_fx in spells.yaml.
// Every entry owns its silhouette; side and end-on views share one shader.

// spellFxStyleKinds maps graphics.projectile_fx to its material silhouette.
var spellFxStyleKinds = map[string]int{
	"fireball":     spellFireball,
	"lightning":    spellLightning,
	"harm":         spellHarm,
	"psyshock":     spellPsyshock,
	"starburst":    spellStarburst,
	"disintegrate": spellDisintegrate,
	"ray_of_light": spellLight,
	"rock":         spellRock,
	"stone_bud":    spellStoneBud,
	"swarm":        spellSwarm,
	"ice_shard":    spellIce,
	"fire_dart":    spellFireDart,
	"sparks":       spellSparks,
	"shadow_bolt":  spellShadow,
	"void_needle":  spellVoidNeedle,
	"shackle":      spellShackle,
	"charm_bloom":  spellCharm,
	"psi_lance":    spellPsiLance,
	"lash":         spellLash,
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
			if _, ok := spellFxStyleKinds[def.Graphics.ProjectileFx]; !ok {
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
