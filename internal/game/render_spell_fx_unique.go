package game

import (
	"fmt"

	"ugataima/internal/config"
)

// Spell materials are selected by graphics.projectile_fx in spells.yaml.
// Every entry owns its silhouette; side and end-on views share one shader.

// spellFxStyleKinds maps graphics.projectile_fx to its material silhouette.
var spellFxStyleKinds = map[string]int{
	"fireball":      spellFireball,
	"lightning":     spellLightning,
	"harm":          spellHarm,
	"psyshock":      spellPsyshock,
	"starburst":     spellStarburst,
	"disintegrate":  spellDisintegrate,
	"ray_of_light":  spellLight,
	"rock":          spellRock,
	"stone_bud":     spellStoneBud,
	"swarm":         spellSwarm,
	"ice_shard":     spellIce,
	"fire_dart":     spellFireDart,
	"sparks":        spellSparks,
	"shadow_bolt":   spellShadow,
	"void_needle":   spellVoidNeedle,
	"shackle":       spellShackle,
	"charm_bloom":   spellCharm,
	"psi_lance":     spellPsiLance,
	"lash":          spellLash,
	"acid_burst":    spellAcid,
	"flying_fist":   spellFlyingFist,
	"shrapmetal":    spellShrapmetal,
	"soul_wisp":     spellSoulWisp,
	"prismatic_ray": spellPrismaticRay,
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
		if style := def.Graphics.WorldFx; style != "" {
			var kind config.MechanicKind
			if def.Mechanic != nil {
				kind = def.Mechanic.Kind
			}
			if entry, ok := spellWorldStyles[style]; !ok || entry.mechanic != string(kind) {
				panic(fmt.Sprintf("spell %q: world_fx %q is incompatible with mechanic %q", key, style, kind))
			}
		}
		if flights := def.Graphics.TargetFlights; flights != nil && flights.ProjectileFx != "" {
			if _, ok := spellFxStyleKinds[flights.ProjectileFx]; !ok {
				panic(fmt.Sprintf("spell %q: unknown target_flights.projectile_fx %q", key, flights.ProjectileFx))
			}
		}
		if style := def.Graphics.CastOverlay; style != "" {
			entry, ok := castOverlayStyles[style]
			if !ok {
				panic(fmt.Sprintf("spell %q: unknown cast_overlay style %q", key, style))
			}
			if def.IsProjectile {
				panic(fmt.Sprintf("spell %q: cast_overlay requires a non-projectile spell", key))
			}
			if entry.weapon != (def.Graphics.CastOverlayWeapon != "") {
				panic(fmt.Sprintf("spell %q: cast_overlay_weapon is required exactly for weapon enchantment overlays", key))
			}
		} else if def.Graphics.CastOverlayWeapon != "" {
			panic(fmt.Sprintf("spell %q: cast_overlay_weapon needs a weapon enchantment overlay", key))
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
