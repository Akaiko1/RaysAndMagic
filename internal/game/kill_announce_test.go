package game

import (
	"strings"
	"testing"

	damagecalc "ugataima/internal/damage"
	monsterPkg "ugataima/internal/monster"
)

// Every death the party hears of is announced exactly once, after the blow
// that caused it, with the experience it gives: whatever killed it (a weapon,
// a trap, a damage-over-time tick, an ally). Wildlife hunts and caravans stay
// out of the log; a kill worth nothing shows no experience.
func TestEveryKillIsAnnouncedOnceWithItsExperience(t *testing.T) {
	type row struct {
		kill    func(g *MMGame, m *monsterPkg.Monster3D)
		setup   func(g *MMGame, m *monsterPkg.Monster3D)
		blow    string // a line that must come before the kill line
		want    string // the kill line; "" = no kill line at all
		tone    logTone
		killed  bool // the monster ends dead (positive control)
		victimF func(g *MMGame) *monsterPkg.Monster3D
	}
	strike := func(attacker *monsterPkg.Monster3D, dmg int) func(g *MMGame, m *monsterPkg.Monster3D) {
		return func(g *MMGame, m *monsterPkg.Monster3D) {
			g.combat.strikeMonsterPacketFor(attacker, m, singleMonsterDamagePacket(damagecalc.Parts{True: dmg}, "physical", 0), nil, false, true, true, false)
		}
	}
	summon := &monsterPkg.Monster3D{ID: "ally", Name: "Wolf", SummonedBy: spellSummonOwnerPrefix + "summon_wolf", HitPoints: 50, MaxHitPoints: 50}
	hunter := &monsterPkg.Monster3D{ID: "hunter", Name: "Puma", Disposition: monsterPkg.DispositionWildlife, HitPoints: 50, MaxHitPoints: 50}
	for name, tc := range map[string]row{
		"weapon blow": {kill: func(g *MMGame, m *monsterPkg.Monster3D) { g.combat.ApplyDamageToMonster(m, 500, "", false) },
			blow: "hits Goblin for", want: "Goblin is slain! +480 XP (120 each)", tone: logToneGood, killed: true},
		"blow it survives": {kill: func(g *MMGame, m *monsterPkg.Monster3D) {
			m.HitPoints, m.MaxHitPoints = 10000, 10000
			g.combat.ApplyDamageToMonster(m, 5, "", false)
		}, blow: "hits Goblin for", want: ""},
		"trap": {kill: func(g *MMGame, m *monsterPkg.Monster3D) {
			g.combat.applyTrapDamage(m, 500, monsterPkg.DamagePhysical.String(), "Test Trap")
		}, blow: "takes", want: "Goblin is slain! +480 XP (120 each)", tone: logToneGood, killed: true},
		"damage over time": {kill: func(g *MMGame, m *monsterPkg.Monster3D) {
			m.HitPoints = 0 // a poison tick left it here
			(&GameLoop{game: g}).finalizeIndirectKills()
		}, want: "Goblin is slain! +480 XP (120 each)", tone: logToneGood, killed: true},
		"party ally's blow":    {kill: strike(summon, 500), blow: "Wolf strikes Goblin", want: "Goblin is slain! +480 XP (120 each)", tone: logToneGood, killed: true},
		"wildlife kills a foe": {kill: strike(hunter, 500), blow: "Puma strikes Goblin", want: "Goblin is slain!", tone: logToneGood, killed: true},
		"wildlife hunt": {setup: func(g *MMGame, m *monsterPkg.Monster3D) { m.Disposition = monsterPkg.DispositionWildlife },
			kill: strike(hunter, 500), want: "", killed: true},
		"caravan": {setup: func(g *MMGame, m *monsterPkg.Monster3D) { m.Disposition = monsterPkg.DispositionCaravan },
			kill: func(g *MMGame, m *monsterPkg.Monster3D) { m.HitPoints = 0; g.combat.finishMonsterKill(m) }, want: "", killed: true},
		"boss add": {setup: func(g *MMGame, m *monsterPkg.Monster3D) { m.SummonedBy = "boss" },
			kill: func(g *MMGame, m *monsterPkg.Monster3D) { g.combat.applyTrapDamage(m, 500, "physical", "Test Trap") },
			want: "Goblin is slain!", tone: logToneGood, killed: true},
		"the party's summon falls": {victimF: func(g *MMGame) *monsterPkg.Monster3D {
			s := *summon
			s.ID = "fallen"
			g.world.Monsters = append(g.world.Monsters, &s)
			return &s
		}, kill: func(g *MMGame, m *monsterPkg.Monster3D) {
			g.combat.strikeMonsterPacketFor(g.world.Monsters[0], m, singleMonsterDamagePacket(damagecalc.Parts{True: 500}, "physical", 0), nil, false, true, true, false)
		}, blow: "Goblin strikes Wolf", want: "Wolf is slain!", tone: logToneBad, killed: true},
		"finished twice": {kill: func(g *MMGame, m *monsterPkg.Monster3D) {
			m.HitPoints = 0
			g.combat.finishMonsterKill(m)
			g.combat.finishMonsterKill(m)
		}, want: "Goblin is slain! +480 XP (120 each)", tone: logToneGood, killed: true},
		"a lone hero": {setup: func(g *MMGame, m *monsterPkg.Monster3D) { g.party.Members = g.party.Members[:1] },
			kill: func(g *MMGame, m *monsterPkg.Monster3D) { g.combat.applyTrapDamage(m, 500, "physical", "Test Trap") },
			want: "Goblin is slain! +480 XP", tone: logToneGood, killed: true},
	} {
		t.Run(name, func(t *testing.T) {
			g, _ := newThiefTestGame(t)
			m := spawnTestMonsterAt(g, 3, 1)
			m.Experience = 480
			if tc.victimF != nil {
				m = tc.victimF(g)
			}
			if tc.setup != nil {
				tc.setup(g, m)
			}
			if len(g.party.Members) > 1 && len(g.party.Members) != 4 {
				t.Fatalf("party of %d; the rows expect 4", len(g.party.Members))
			}
			start := len(g.combatLogHistory)
			tc.kill(g, m)
			if tc.killed == m.IsAlive() {
				t.Fatalf("victim alive=%v after the kill (HP %d)", m.IsAlive(), m.HitPoints)
			}
			var lines []combatLogEntry
			lines = append(lines, g.combatLogHistory[start:]...)
			slain, blowAt, killAt := 0, -1, -1
			for i, e := range lines {
				if strings.Contains(e.Text, " is slain!") {
					slain++
					killAt = i
					if e.Text != tc.want || e.Tone != tc.tone {
						t.Errorf("kill line %q tone %d, want %q tone %d", e.Text, e.Tone, tc.want, tc.tone)
					}
				}
				if tc.blow != "" && blowAt < 0 && strings.Contains(e.Text, tc.blow) {
					blowAt = i
				}
			}
			switch {
			case tc.want == "" && slain != 0:
				t.Errorf("%d kill lines, want none: %v", slain, texts(lines))
			case tc.want != "" && slain != 1:
				t.Errorf("%d kill lines, want one: %v", slain, texts(lines))
			case tc.blow != "" && blowAt < 0:
				t.Errorf("no %q line: %v", tc.blow, texts(lines))
			case tc.want != "" && tc.blow != "" && blowAt > killAt:
				t.Errorf("the kill is announced before its blow: %v", texts(lines))
			}
		})
	}
}

func texts(lines []combatLogEntry) []string {
	out := make([]string, len(lines))
	for i, e := range lines {
		out[i] = e.Text
	}
	return out
}
