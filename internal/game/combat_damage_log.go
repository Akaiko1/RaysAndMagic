package game

import (
	"strings"
	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/monster"
)

// damageLogTarget snapshots the target at the reporting point. Later procs,
// reflection and death cleanup must not change the HP printed for this hit.
type damageLogTarget struct {
	name      logPart
	hp, maxHP int
}

// monsterLogTarget captures a monster's name, side and post-hit HP.
func monsterLogTarget(m *monster.Monster3D) damageLogTarget {
	return damageLogTarget{logMonsterName(m), m.HitPoints, m.MaxHitPoints}
}

// heroLogTarget captures a party member's name and post-hit HP.
func heroLogTarget(c *character.MMCharacter) damageLogTarget {
	return damageLogTarget{logHeroName(c), c.HitPoints, c.MaxHitPoints}
}

type damageLogKind string

const (
	damageLogHit         damageLogKind = "hit"
	damageLogCrush       damageLogKind = "crush"
	damageLogSplash      damageLogKind = "splash"
	damageLogIndirect    damageLogKind = "indirect"
	damageLogScorch      damageLogKind = "scorch"
	damageLogDodge       damageLogKind = "dodge"
	damageLogHeroDodge   damageLogKind = "hero_dodge"
	damageLogReflection  damageLogKind = "reflection"
	damageLogSacrifice   damageLogKind = "sacrifice"
	damageLogBurnCost    damageLogKind = "burn_cost"
	damageLogStrike      damageLogKind = "strike"
	damageLogBoundStrike damageLogKind = "bound_strike"
	damageLogBolt        damageLogKind = "bolt"
	damageLogProjectile  damageLogKind = "projectile"
	damageLogBurst       damageLogKind = "burst"
)

// damageLog describes a resolved hit, not a damage request. Amount is the
// resolved packet total (including overkill), not raw damage or an HP delta.
// For sacrifice, Source names the protected hero and Target is the HP payer.
type damageLog struct {
	Source   logPart
	Target   damageLogTarget
	Amount   int
	School   string // the number's tint and named school; "" is true damage
	Kind     damageLogKind
	Critical bool
}

// logSlot marks where a catalog line takes a styled log part: render the
// line with it for each %s, then pass it through styledFormat.
const logSlot = "\x00"

// styledFormat turns a catalog line rendered with logSlot markers into a
// logCombat format: each marker takes one styled part, and literal percent
// signs are escaped.
func styledFormat(rendered string) string {
	return strings.ReplaceAll(strings.ReplaceAll(rendered, "%", "%%"), logSlot, "%s")
}

// reportDamage is the sole numeric damage-message formatter. Call immediately
// after applying the hit and before follow-up procs or death messages. It never
// deals damage, awards kills, or decides whether an event should be announced.
// Silent DoT ticks and execution messages deliberately do not use this path.
func (g *MMGame) reportDamage(tone logTone, hit damageLog) {
	kind := hit.Kind
	if kind == "" {
		kind = damageLogHit
	}
	amount := logDamage(hit.Amount, hit.School)
	args := []any{hit.Source, hit.Target.name, amount}
	// Explicit keys let the catalog audit check every authored template and its
	// arity; styledFormat turns the logSlot markers into styled log parts.
	const slot = logSlot
	var format string
	switch kind {
	case damageLogHit:
		format = uitext.Text("combat.damage.hit", slot, slot, slot)
	case damageLogCrush:
		format = uitext.Text("combat.damage.crush", slot, slot, slot)
	case damageLogSplash:
		format = uitext.Text("combat.damage.splash", slot, slot, slot)
	case damageLogIndirect:
		format = uitext.Text("combat.damage.indirect", slot, slot, slot)
		args = []any{hit.Target.name, amount, hit.Source}
	case damageLogScorch:
		format = uitext.Text("combat.damage.scorch", slot, slot, slot)
	case damageLogDodge:
		format = uitext.Text("combat.damage.dodge", slot, slot, slot, slot)
		args = []any{hit.Target.name, hit.Source, amount, logDamageSchool(hit.School)}
	case damageLogHeroDodge:
		format = uitext.Text("combat.damage.hero_dodge", slot, slot, slot, slot)
		args = []any{hit.Target.name, hit.Source, amount, logDamageSchool(hit.School)}
	case damageLogReflection:
		format = uitext.Text("combat.damage.reflection", slot, slot)
		args = []any{hit.Target.name, amount}
	case damageLogSacrifice:
		format = uitext.Text("combat.damage.sacrifice", slot, slot, slot)
		args = []any{hit.Target.name, amount, hit.Source}
	case damageLogBurnCost:
		format = uitext.Text("combat.damage.burn_cost", slot, slot)
		args = []any{hit.Target.name, amount}
	case damageLogStrike:
		format = uitext.Text("combat.damage.strike", slot, slot, slot)
	case damageLogBoundStrike:
		format = uitext.Text("combat.damage.bound_strike", slot, slot, slot)
	case damageLogBolt:
		format = uitext.Text("combat.damage.bolt", slot, slot, slot)
	case damageLogProjectile:
		format = uitext.Text("combat.damage.projectile", slot, slot, slot, slot)
		args = append(args, logDamageSchool(hit.School))
	case damageLogBurst:
		format = uitext.Text("combat.damage.burst", slot, slot, slot)
	default:
		panic("unknown damage log kind: " + string(kind))
	}
	format = styledFormat(format)
	args = append([]any{logCrit(hit.Critical)}, args...)
	args = append(args, logHP(hit.Target.hp, hit.Target.maxHP))
	g.logCombat(tone, "%s"+format+" %s", args...)
}

// reportActorDamage applies the same visibility and party-side policy as other
// actor messages, then delegates all damage wording and HP to reportDamage.
func (g *MMGame) reportActorDamage(attacker, target *monster.Monster3D, hit damageLog) {
	if quietActorCombat(attacker, target) {
		return
	}
	g.reportDamage(actorCombatTone(attacker, target), hit)
}
