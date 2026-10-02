//go:build balance

package game

import (
	"slices"
	"testing"

	"ugataima/internal/character"
	damagecalc "ugataima/internal/damage"
)

// A one-shot is a hit that takes a hero from 80%+ HP to zero; the biggest hit
// is the largest damage share of a hero's max HP.
func TestBalanceNoteHit(t *testing.T) {
	for name, tc := range map[string]struct {
		damage, before int
		oneShot        bool
	}{
		"full health, one blow":            {damage: 120, before: 100, oneShot: true},
		"finishing blow on a wounded hero": {damage: 90, before: 10},
		"from exactly 80%":                 {damage: 85, before: 80, oneShot: true},
		"from 79%":                         {damage: 85, before: 79},
		"a big hit that leaves HP":         {damage: 95, before: 100},
		"exactly lethal":                   {damage: 100, before: 100, oneShot: true},
	} {
		t.Run(name, func(t *testing.T) {
			var r balanceFight
			r.noteHit("Ocelot", tc.damage, tc.before, 100)
			if (r.OneShots == 1) != tc.oneShot || r.MaxHitPct != float64(tc.damage) || r.BigHitBy != "Ocelot" {
				t.Errorf("one-shots %d, max hit %.0f%% by %q", r.OneShots, r.MaxHitPct, r.BigHitBy)
			}
		})
	}
	var r balanceFight
	r.noteHit("Ocelot", 30, 100, 100)
	r.noteHit("Dragon", 60, 70, 100)
	r.noteHit("Ocelot", 40, 10, 100)
	if r.MaxHitPct != 60 || r.BigHitBy != "Dragon" || r.OneShots != 0 {
		t.Errorf("over a fight: max hit %.0f%% by %q, one-shots %d", r.MaxHitPct, r.BigHitBy, r.OneShots)
	}
}

// The observer sees each hostile hit with the hero's HP just before it, so HP
// moved by Sacrifice or a heal in the same step is never stale.
func TestBalanceHitsSeePreHitHealth(t *testing.T) {
	a := newBalanceArena(t)
	g := a.g
	a.stage("classic", 15, []string{"goblin"})
	foe := g.world.Monsters[0]
	type seen struct {
		hero           string
		before, damage int
	}
	var hits []seen
	g.heroHitObserver = func(h *character.MMCharacter, _ string, damage int) {
		hits = append(hits, seen{h.Name, h.HitPoints, damage})
	}
	defer func() { g.heroHitObserver = nil }()
	auberon, celestine, silvelyn := g.party.Members[0], g.party.Members[2], g.party.Members[3]
	hit := func(target *character.MMCharacter, dmg int) seen {
		g.combat.monsterHitCharacter(foe, target, foe.Name, monsterCharacterHit{Parts: damagecalc.Parts{True: dmg}, DamageType: "physical", IgnoresDodge: true})
		return hits[len(hits)-1]
	}

	// Blows on Celestine: Sacrifice moves a share onto Auberon, which is no hit.
	for i := 0; auberon.HitPoints*10 >= auberon.MaxHitPoints*8 && i < 20; i++ {
		celestine.HitPoints = celestine.MaxHitPoints
		hit(celestine, 60)
	}
	for _, h := range hits {
		if h.hero == auberon.Name {
			t.Fatalf("a Sacrifice transfer was observed as a hit on %s", h.hero)
		}
	}
	if auberon.HitPoints*10 >= auberon.MaxHitPoints*8 {
		t.Fatalf("Sacrifice left %s at %d/%d; the case checks nothing", auberon.Name, auberon.HitPoints, auberon.MaxHitPoints)
	}
	wounded := auberon.HitPoints
	finisher := hit(auberon, 1000)
	var r balanceFight
	r.noteHit(foe.Name, finisher.damage, finisher.before, auberon.MaxHitPoints)
	if finisher.hero != auberon.Name || finisher.before != wounded || r.OneShots != 0 {
		t.Errorf("finishing blow on %s seen at %d HP (was %d), one-shots %d", finisher.hero, finisher.before, wounded, r.OneShots)
	}

	// Wounded, then healed back to full in the same step: the next lethal blow
	// is a one-shot.
	hit(silvelyn, silvelyn.MaxHitPoints/2)
	silvelyn.HitPoints = silvelyn.MaxHitPoints
	blow := hit(silvelyn, 1000)
	r = balanceFight{}
	r.noteHit(foe.Name, blow.damage, blow.before, silvelyn.MaxHitPoints)
	if blow.before != silvelyn.MaxHitPoints || r.OneShots != 1 {
		t.Errorf("blow after a heal seen at %d/%d HP, one-shots %d", blow.before, silvelyn.MaxHitPoints, r.OneShots)
	}

	// A hostile packet (Fireburst, Inferno, brood-fire) is a hit; the party's
	// own splash is not.
	mirelle := g.party.Members[1]
	n := len(hits)
	g.combat.damagePartyMemberElement(1, mirelle, 10, "fire", false)
	if len(hits) != n {
		t.Errorf("the party's own splash was observed as a hit")
	}
	before := mirelle.HitPoints
	g.combat.damagePartyMemberPartsFromSource(1, mirelle, damagecalc.Parts{Normal: 10}, "fire", true, foe)
	if len(hits) != n+1 || hits[n].before != before {
		t.Errorf("a hostile packet was not observed at %d HP: %v", before, hits[n:])
	}
}

// An ability line counts for a monster only when no other monster in the
// fight could have written it.
func TestBalanceCoverageTellsMonstersApart(t *testing.T) {
	newBalanceArena(t) // loads the monster catalog
	gold := balanceFoeRef{Key: "dragon_gold", Name: "Dragon"}
	green := balanceFoeRef{Key: "dragon_green", Name: "Dragon"}
	clock := balanceFoeRef{Key: "grandfather_clock", Name: "Grandfather Clock"}
	warlord := balanceFoeRef{Key: "orc_hero_boss", Name: "Orc Warlord"}
	type tally struct{ told, fired int }
	for name, tc := range map[string]struct {
		foes    []balanceFoeRef
		line    string
		enraged string
		want    map[string]map[string]tally // key -> ability -> tally
		elem    map[string]int
	}{
		"shared name, both breathe": {foes: []balanceFoeRef{gold, green}, line: "Dragon breathes fire!",
			want: map[string]map[string]tally{"dragon_gold": {"dragon_breath": {}}, "dragon_green": {"dragon_breath": {}}}},
		"shared name, only one stuns": {foes: []balanceFoeRef{gold, green}, line: "Mara is stunned!",
			want: map[string]map[string]tally{"dragon_gold": {"stun": {1, 1}}, "dragon_green": {"poison": {1, 0}}}},
		"shared name, only one poisons": {foes: []balanceFoeRef{gold, green}, line: "Mara is poisoned!",
			want: map[string]map[string]tally{"dragon_green": {"poison": {1, 1}}, "dragon_gold": {"stun": {1, 0}}}},
		"alone, its breath counts": {foes: []balanceFoeRef{gold}, line: "Dragon breathes fire!",
			want: map[string]map[string]tally{"dragon_gold": {"dragon_breath": {1, 1}}}},
		"two stunners by different names": {foes: []balanceFoeRef{gold, clock}, line: "Mara is stunned!",
			want: map[string]map[string]tally{"dragon_gold": {"stun": {}, "dragon_breath": {1, 0}}, "grandfather_clock": {"stun": {}}}},
		"enrage read by key": {foes: []balanceFoeRef{warlord}, enraged: "orc_hero_boss",
			want: map[string]map[string]tally{"orc_hero_boss": {"enrage": {1, 1}}}},
		"elemental attack under a shared name": {foes: []balanceFoeRef{gold, green}, line: "Dragon uses Elemental Attack!",
			elem: map[string]int{"dragon_gold": 0, "dragon_green": 0}},
		"elemental attack alone": {foes: []balanceFoeRef{green}, line: "Dragon uses Elemental Attack!",
			elem: map[string]int{"dragon_green": 1}},
	} {
		t.Run(name, func(t *testing.T) {
			f := balanceFight{Foes: map[string]balanceFoeRef{}, Heroes: map[string]string{"Mara": "sniper"}, Enraged: map[string]bool{}}
			for i, foe := range tc.foes {
				f.Foes[string(rune('a'+i))] = foe
			}
			if tc.line != "" {
				f.Log = []string{tc.line}
			}
			if tc.enraged != "" {
				f.Enraged[tc.enraged] = true
			}
			c := newBalanceCoverage()
			c.add(f)
			for _, foe := range tc.foes {
				if c.fights[foe.Key] != 1 {
					t.Errorf("%s in %d fights, want 1", foe.Key, c.fights[foe.Key])
				}
			}
			for key, abilities := range tc.want {
				for ability, want := range abilities {
					if got := (tally{c.told[key][ability], c.fired[key][ability]}); got != want {
						t.Errorf("%s %s told/fired %v, want %v", key, ability, got, want)
					}
				}
			}
			for key, want := range tc.elem {
				if c.elemAtk[key] != want {
					t.Errorf("%s elemental attacks %d, want %d", key, c.elemAtk[key], want)
				}
			}
		})
	}
}

// A fight starts from a fresh turn with nothing left from the previous one,
// and a party that meets nobody loses nothing.
func TestBalanceArenaStartsClean(t *testing.T) {
	a := newBalanceArena(t)
	g := a.g
	a.fight("rogues", 15, []string{"goblin"})
	if !g.turnBasedMode {
		t.Fatal("a fight should leave turn-based mode on")
	}
	g.traps = append(g.traps, PlacedTrap{Key: "probe"})
	g.persistentDamageZones = append(g.persistentDamageZones, PersistentDamageZone{SpellID: "probe"})
	for _, b := range g.timedBuffs() {
		*b.active, *b.duration = true, 99
	}
	g.currentTurn, g.partyActionsUsed, g.partyTechniqueActionsUsed, g.monsterTurnResolved = 1, 3, 1, true
	g.spellInputCooldown, g.viewTurnFramesLeft = 30, 10

	a.reset()
	for _, b := range g.timedBuffs() {
		if *b.active || *b.duration != 0 {
			t.Errorf("a timed buff survived the reset")
			break
		}
	}
	if len(g.world.Monsters) != 0 || g.combatBuffs != nil || g.statBuffs != nil || g.spellInputCooldown != 0 || g.viewTurnFramesLeft != 0 {
		t.Errorf("reset left monsters %d, combat buffs %v, stat buffs %v, cooldowns %d/%d",
			len(g.world.Monsters), g.combatBuffs, g.statBuffs, g.spellInputCooldown, g.viewTurnFramesLeft)
	}
	a.stage("classic", 15, []string{"goblin"})
	if len(g.traps) != 0 || len(g.persistentDamageZones) != 0 {
		t.Errorf("staged fight inherits %d traps, %d damage zones", len(g.traps), len(g.persistentDamageZones))
	}
	if !g.turnBasedMode || g.currentTurn != 0 || g.partyActionsUsed != 0 || g.partyTechniqueActionsUsed != 0 || g.monsterTurnResolved {
		t.Errorf("staged fight opens turn %d with %d/%d actions used, monster turn resolved %v",
			g.currentTurn, g.partyActionsUsed, g.partyTechniqueActionsUsed, g.monsterTurnResolved)
	}
	for _, m := range g.party.Members {
		if m.ActionsRemaining == 0 {
			t.Errorf("%s starts the fight with no action", m.Name)
		}
	}

	keys := make([]string, 0, len(a.bench.Parties))
	for key := range a.bench.Parties {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	raised := 0
	for _, key := range keys {
		for _, level := range a.bench.Levels {
			a.stage(key, level, nil)
			buffed := 0
			for _, m := range g.party.Members {
				buffed += m.MaxHitPoints
			}
			g.resetTimedEffects()
			plain := 0
			for _, m := range g.party.Members {
				m.CalculateDerivedStats(g.config)
			}
			g.applyPartyStatBonuses()
			for _, m := range g.party.Members {
				plain += m.MaxHitPoints
			}
			if buffed > plain {
				raised++
			}
			if f := a.fight(key, level, nil); !f.Won || f.HPLostPct != 0 || f.SPSpentPct != 0 || f.KOs != 0 || f.Rounds != 0 {
				t.Errorf("%s L%d meets nobody: won %v, HP lost %.2f%%, SP spent %.2f%%, KOs %d, rounds %d",
					key, level, f.Won, f.HPLostPct, f.SPSpentPct, f.KOs, f.Rounds)
			}
		}
	}
	if raised == 0 {
		t.Fatal("no party's preparation raised its max HP; the rested-start rule checks nothing")
	}
}
