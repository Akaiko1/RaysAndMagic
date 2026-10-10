package game

import (
	"image/color"
	"maps"
	"slices"

	"ugataima/internal/character"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func (cs *CombatSystem) countCardSummons() int {
	w := cs.game.GetCurrentWorld()
	if w == nil {
		return 0
	}
	n := 0
	for _, m := range w.Monsters {
		if m != nil && m.IsAlive() && isCardAlly(m) {
			n++
		}
	}
	return n
}

// markCardAlly turns a spawned monster into a party ally summoned by the card
// collection: Bound (hunts enemy monsters, ignores the party), tagged for the
// summon limit, and excluded from map-clear quest counts.
// BoundFramesRemaining 0 = never expires (the bind tick only counts down > 0).
// A card ally is a PURE summon (never was an enemy): it yields the party no
// XP/gold/loot on death and does not follow across maps - it simply crumbles
// when the party leaves (a fresh set is re-summoned there via the proc).
func markCardAlly(m *monsterPkg.Monster3D) {
	markPurePartySummon(m, cardSummonOwner)
}

// mitigateCharacterDamage is the int-shaped shorthand for a hit with no true
// component: same pipeline, same armor step, one number in and out. Balance
// tests read better through it; there is no second formula here.
func (cs *CombatSystem) mitigateCharacterDamage(damage int, damageTypeStr string, char *character.MMCharacter, ignoreArmor bool) int {
	return cs.mitigateCharacterDamageParts(
		damagecalc.Parts{Normal: damage}, damageTypeStr, char, ignoreArmor,
	).Normal
}

// tankTarget exposes the shared tank-index rule to targeting tests.
func (cs *CombatSystem) tankTarget() *character.MMCharacter {
	if i := cs.tankIndex(); i >= 0 {
		return cs.game.party.Members[i]
	}
	return nil
}

// tryCastAoeStun handles AoE-stun effect spells (e.g. Darkness): if the spell
// has StunRadiusTiles > 0, every alive monster within that radius of the caster
// is stunned (RT frames + TB turns), no damage dealt. Tests without a caster
// use this shorthand over the production caster-aware path.
func (cs *CombatSystem) tryCastAoeStun(spellID spells.SpellID, def spells.SpellDefinition) bool {
	return cs.tryCastAoeStunBy(spellID, def, nil)
}

func (gl *GameLoop) monsterAttackFoeTurnBased(attacker, foe *monsterPkg.Monster3D) {
	if foe != nil {
		gl.game.combat.commitMonsterAttack(attacker, monsterAttackDestination{foe: foe}, monsterAttackTurn)
	}
}

// GetCombatMessages returns the HUD combat-message texts (most recent last).
// The HUD itself draws from the cached line list (combatLogVersion).
func (g *MMGame) GetCombatMessages() []string {
	hud := g.hudLog()
	out := make([]string, len(hud))
	for i, e := range hud {
		out[i] = e.Text
	}
	return out
}

// GetCombatMessageColor returns the display color for HUD row index (aligned
// with GetCombatMessages).
func (g *MMGame) GetCombatMessageColor(index int) color.Color {
	hud := g.hudLog()
	if index < 0 || index >= len(hud) {
		return color.White
	}
	return hud[index].Color
}

func (reg *renderResourceRegistry) estimatedGPUBytes() int64 {
	var bytes int64
	for img := range reg.allocations {
		b := img.Bounds()
		bytes += int64(b.Dx()) * int64(b.Dy()) * 4
	}
	return bytes
}

func inventoryBagUnits(b character.InventoryBag) int {
	n := 0
	for _, it := range b.Items() {
		n += it.Count()
	}
	return n
}

// createArrowAttack fires along the party's own aim, as a plain ranged attack.
func (cs *CombatSystem) createArrowAttack(damage int, slot items.EquipSlot, label string) bool {
	return cs.createArrowAttackAimed(damage, slot, label, cs.partyAttackAngle(), false)
}

func (r *Renderer) collectMapRenderPrewarmPlan(mapKey string) mapRenderPrewarmPlan {
	return r.collectMapRenderPrewarmPlanForScope(r.mapRenderPrewarmScope(mapKey))
}

func (r *Renderer) collectMapRenderPrewarmPlanForScope(scope mapRenderPrewarmScope) mapRenderPrewarmPlan {
	plan, _ := r.collectMapRenderPrewarmPlanAndPriorities(scope)
	return plan
}

func alchemyRecipeByKey(key string) *config.AlchemyRecipe {
	if config.GlobalAlchemy != nil {
		for i := range config.GlobalAlchemy.Recipes {
			if config.GlobalAlchemy.Recipes[i].Key == key {
				return &config.GlobalAlchemy.Recipes[i]
			}
		}
	}
	return nil
}

func allTileKeys(tm *world.TileManager) []string {
	return slices.Collect(maps.Keys(tm.ListTiles()))
}

// takeDamageParts is the monster half of a hit alone: resistance and soak, no
// target armor. Game code goes through applyMonsterDamagePacket.
func takeDamageParts(m *monsterPkg.Monster3D, parts damagecalc.Parts, damageType monsterPkg.DamageType, resistPiercePct int) int {
	return m.TakeDamagePacket([]monsterPkg.DamageComponent{{Parts: parts, DamageType: damageType, ResistPiercePct: resistPiercePct}}).Total()
}
