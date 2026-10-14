package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/config"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

// Disintegrate is a NO-DAMAGE projectile: a hit either instakills (the proc) or
// does nothing. It must NOT fall into the normal damage path, which used to spam
// "hit for 0 damage" - even prefixed "Critical!" - on every non-proc hit. An
// undead is immune to disintegrate, so the proc can never apply: a deterministic
// "no effect" outcome, no RNG.
func TestDisintegrate_NoDamageSpamOnImmuneTarget(t *testing.T) {
	g, thief := newThiefTestGame(t)
	def, err := spells.GetSpellDefinitionByID("disintegrate")
	if err != nil {
		t.Fatalf("disintegrate spell: %v", err)
	}
	if !def.DealsNoDamage || def.DisintegrateChance <= 0 {
		t.Fatalf("test assumes a no-damage disintegrate with a proc chance; got DealsNoDamage=%v chance=%v",
			def.DealsNoDamage, def.DisintegrateChance)
	}

	mob := monsterPkg.NewMonster3DFromConfig(g.camera.X+1, g.camera.Y, "minotaur", g.config)
	if mob == nil {
		t.Fatal("failed to build test monster")
	}
	mob.MonsterType = "undead" // immune to disintegrate -> the instakill never applies
	mob.PerfectDodge = 0       // don't let a dodge preempt the outcome
	mob.MaxHitPoints, mob.HitPoints = 100, 100
	mob.Pacified = true // a hit must break Charm even when it deals no damage
	mob.WasAttacked = false
	mob.IsEngagingPlayer = false
	mob.HitTintFrames = 0
	g.world.Monsters = []*monsterPkg.Monster3D{mob}

	mp := &MagicProjectile{
		ID:                 "mp_disint",
		Attacker:           thief,
		Active:             true,
		LifeTime:           60,
		SpellType:          string(def.ID),
		DisintegrateChance: def.DisintegrateChance,
		Damage:             0,
	}
	g.combat.applyProjectileDamage(mp, mob, mp.ID)

	var b strings.Builder
	for _, e := range g.combatLogHistory {
		b.WriteString(e.Text)
		b.WriteByte('\n')
	}
	got := b.String()

	if !strings.Contains(got, "has no effect on") {
		t.Errorf("want a 'no effect' message, got:\n%s", got)
	}
	if strings.Contains(got, "0 damage") {
		t.Errorf("no-damage spell must not spam a 0-damage hit:\n%s", got)
	}
	if strings.Contains(got, "Critical") {
		t.Errorf("no-damage spell must not crit:\n%s", got)
	}
	if mob.HitPoints != 100 {
		t.Errorf("immune monster should be unharmed, hp=%d", mob.HitPoints)
	}

	// A no-effect hit is still a HIT: it must run the normal bookkeeping so the mob
	// reacts (the early-return bug skipped all of this).
	if !mob.WasAttacked {
		t.Error("no-effect hit must still set WasAttacked (passive mobs would never aggro)")
	}
	if !mob.IsEngagingPlayer {
		t.Error("no-effect hit must aggro the monster")
	}
	if mob.Pacified {
		t.Error("no-effect hit must break Charm (breakPacifyOnHit)")
	}
	if mob.HitTintFrames <= 0 {
		t.Error("no-effect hit must still flash the hit tint (visual feedback)")
	}
}

// A boss withstands the spell's proc: it takes the sear as a real hit - no
// "no effect" line - and lives. A missed roll stays inert.
func TestDisintegrate_SearsBoss(t *testing.T) {
	for _, proc := range []bool{true, false} {
		t.Run(fmt.Sprintf("proc=%v", proc), func(t *testing.T) {
			g, thief := newThiefTestGame(t)
			def, err := spells.GetSpellDefinitionByID("disintegrate")
			if err != nil {
				t.Fatalf("disintegrate spell: %v", err)
			}
			mob := monsterPkg.NewMonster3DFromConfig(g.camera.X+1, g.camera.Y, "minotaur", g.config)
			mob.Boss = true // a minotaur is no formless type: the boss flag decides
			mob.PerfectDodge = 0
			mob.Resistances = nil
			mob.MaxHitPoints, mob.HitPoints = 1000, 1000
			g.world.Monsters = []*monsterPkg.Monster3D{mob}
			chance := 0.0
			if proc {
				chance = 1
			}
			mp := &MagicProjectile{ID: "mp_sear", Attacker: thief, Active: true, LifeTime: 60, SpellType: string(def.ID), DisintegrateChance: chance}
			g.combat.applyProjectileDamage(mp, mob, mp.ID)

			want := 1000
			if proc {
				want -= config.DisintegrateBossTrueDamage
			}
			if !mob.IsAlive() || mob.HitPoints != want {
				t.Fatalf("HP=%d alive=%v, want %d alive", mob.HitPoints, mob.IsAlive(), want)
			}
			if noEffect := countCombatLog(g, "has no effect on") == 1; noEffect == proc {
				t.Fatalf("'no effect' line=%v with proc=%v", noEffect, proc)
			}
		})
	}
}
