//go:build balance

package game

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	uitext "ugataima/assets/text"
	"ugataima/internal/monster"
)

// balanceAbility is one authored monster ability and how a fight shows it:
// a line naming the monster, a line naming none (a struck hero, a field), or
// (neither set) the monster's state. name is the abilities: kind; behaviour
// rows outside that map set authored.
type balanceAbility struct {
	name     string
	authored func(d monster.MonsterDefinition) bool
	foeLine  func(line, foe string) bool
	bareLine func(line string, heroes map[string]string) bool
}

func (ab balanceAbility) authors(d monster.MonsterDefinition) bool {
	if ab.authored != nil {
		return ab.authored(d)
	}
	return slices.Contains(d.AbilityKinds(), ab.name)
}

// heroLine matches a line opening with "<hero><phrase>" for any of the
// phrases; a duration may follow ("Mara is stunned! (1 turn)").
func heroLine(phrases ...string) func(line string, heroes map[string]string) bool {
	return func(line string, heroes map[string]string) bool {
		for hero := range heroes {
			for _, phrase := range phrases {
				if strings.HasPrefix(line, hero+phrase) {
					return true
				}
			}
		}
		return false
	}
}

func foeLine(format string) func(line, foe string) bool {
	return func(line, foe string) bool { return line == strings.ReplaceAll(format, "<foe>", foe) }
}

func foePrefix(format string) func(line, foe string) bool {
	return func(line, foe string) bool { return strings.HasPrefix(line, strings.ReplaceAll(format, "<foe>", foe)) }
}

// partyAttackBalanceAbilities is one row per shipped party attack key, read
// by its own cast line.
func partyAttackBalanceAbilities() []balanceAbility {
	var out []balanceAbility
	for _, a := range shippedPartyAttacks() {
		out = append(out, balanceAbility{
			name: a.Key,
			authored: func(d monster.MonsterDefinition) bool {
				return slices.ContainsFunc(d.Abilities.PartyAttacks, func(p monster.PartyAttack) bool { return p.Key == a.Key })
			},
			foeLine: func(line, foe string) bool {
				return line == uitext.Text("combat.party_attack.cast", foe, a.Name)
			},
		})
	}
	return out
}

var balanceAbilities = append(partyAttackBalanceAbilities(), []balanceAbility{
	{name: "telegraph", foeLine: foePrefix("<foe> prepares ")},
	{name: "trap_volley", bareLine: func(l string, _ map[string]string) bool { return l == "The ground erupts in brood-fire!" }},
	{name: "pounce", foeLine: foeLine("<foe> pounces at the party!")},
	{name: "phase_shift", foeLine: foeLine("<foe> phase-shifts toward the party!")},
	{name: "poison", bareLine: heroLine(" is poisoned!")},
	{name: "ignite", bareLine: heroLine(" bursts into flames!", " catches fire!")},
	{name: "stun", bareLine: heroLine(" is stunned!")},
	{name: "melt_armor", bareLine: heroLine("'s armor melts!")},
	{name: "dispel", foeLine: func(l, f string) bool {
		return strings.HasPrefix(l, f+" rips ") && strings.HasSuffix(l, " from the party!")
	}},
	{name: "root_party", foeLine: foeLine("<foe> roots the party in place!")},
	{name: "rear_blink", foeLine: foeLine("<foe> slips behind the party!")},
	{name: "teleport", foeLine: foeLine("<foe> blinks away in a golden flash!")},
	{name: "ally_heal", foeLine: foePrefix("<foe> mends ")},
	{name: "summon", foeLine: foeLine("<foe> raises the war-banner - retainers rush to its side!")},
	{name: "push", foeLine: foePrefix("<foe> shoves the party toward ")},
	{name: "alarm_rally", authored: func(d monster.MonsterDefinition) bool { return d.RallyOnAggroTiles > 0 }, foeLine: foePrefix("<foe> rings out")},
	{name: "death_rally", authored: func(d monster.MonsterDefinition) bool { return d.DeathRalliesType != "" },
		foeLine: foePrefix("<foe> falls - its retainers turn on you")},
	{name: "enrage"},
}...)

// balanceCoverage collects, over a sweep, which authored monster abilities
// fired and what each class actually did. The log names monsters by display
// name, so a line counts for a key only when no other key in the fight could
// have written it; such fights are left out of that ability's tally.
type balanceCoverage struct {
	fights   map[string]int            // monster key -> fights it appeared in
	told     map[string]map[string]int // monster key -> ability -> fights its lines were attributable
	fired    map[string]map[string]int // monster key -> ability -> fights it fired in
	elemAtk  map[string]int            // monster key -> fights with an Elemental Attack
	actions  map[string]map[string]int // class -> action kind -> count
	phrases  map[string]map[string]int // class -> log phrase -> count
	heroSeen map[string]int            // class -> fights
}

func newBalanceCoverage() *balanceCoverage {
	return &balanceCoverage{fights: map[string]int{}, told: map[string]map[string]int{}, fired: map[string]map[string]int{}, elemAtk: map[string]int{},
		actions: map[string]map[string]int{}, phrases: map[string]map[string]int{}, heroSeen: map[string]int{}}
}

// balanceTellingNames lists the display names under which a line identifies
// key: names no other present key that also authors the line shares.
func balanceTellingNames(keysByName map[string]map[string]bool, key string, authors func(string) bool) []string {
	var names []string
	for name, keys := range keysByName {
		if !keys[key] {
			continue
		}
		sole := true
		for k := range keys {
			sole = sole && (k == key || !authors(k))
		}
		if sole {
			names = append(names, name)
		}
	}
	return names
}

var balanceNumbers = regexp.MustCompile(`[0-9]+`)

func (c *balanceCoverage) add(f balanceFight) {
	keysByName := map[string]map[string]bool{}
	for _, foe := range f.Foes {
		if keysByName[foe.Name] == nil {
			keysByName[foe.Name] = map[string]bool{}
		}
		keysByName[foe.Name][foe.Key] = true
	}
	logged := func(names []string, seen func(line, foe string) bool) bool {
		for _, line := range f.Log {
			for _, name := range names {
				if seen(line, name) {
					return true
				}
			}
		}
		return false
	}
	present := map[string]bool{}
	for _, foe := range f.Foes {
		present[foe.Key] = true
	}
	for key := range present {
		c.fights[key]++
		if c.fired[key] == nil {
			c.fired[key], c.told[key] = map[string]int{}, map[string]int{}
		}
		def := monster.MonsterConfig.Monsters[key]
		for _, ab := range balanceAbilities {
			if !ab.authors(def) {
				continue
			}
			authors := func(k string) bool { return ab.authors(monster.MonsterConfig.Monsters[k]) }
			switch {
			case ab.foeLine != nil:
				names := balanceTellingNames(keysByName, key, authors)
				if len(names) == 0 {
					continue
				}
				c.told[key][ab.name]++
				if logged(names, ab.foeLine) {
					c.fired[key][ab.name]++
				}
			case ab.bareLine != nil:
				// The line names no monster: any other present key that
				// authors the ability could have written it.
				sole := true
				for k := range present {
					sole = sole && (k == key || !authors(k))
				}
				if !sole {
					continue
				}
				c.told[key][ab.name]++
				if slices.ContainsFunc(f.Log, func(line string) bool { return ab.bareLine(line, f.Heroes) }) {
					c.fired[key][ab.name]++
				}
			default: // the monster's state, read by key
				c.told[key][ab.name]++
				if f.Enraged[key] {
					c.fired[key][ab.name]++
				}
			}
		}
		names := balanceTellingNames(keysByName, key, func(string) bool { return true })
		if logged(names, func(line, foe string) bool { return strings.HasPrefix(line, foe+" uses Elemental Attack") }) {
			c.elemAtk[key]++
		}
	}
	for hero, class := range f.Heroes {
		c.heroSeen[class]++
		if c.actions[class] == nil {
			c.actions[class] = map[string]int{}
			c.phrases[class] = map[string]int{}
		}
		for kind, n := range f.Actions[hero] {
			c.actions[class][kind] += n
		}
		for _, line := range f.Log {
			rest, ok := strings.CutPrefix(line, hero+" ")
			if !ok && !strings.HasPrefix(line, hero+"'s ") {
				continue
			}
			if !ok {
				rest = strings.TrimPrefix(line, hero)
			}
			for name := range keysByName {
				rest = strings.ReplaceAll(rest, name, "<foe>")
			}
			for other := range f.Heroes {
				rest = strings.ReplaceAll(rest, other, "<hero>")
			}
			if i := strings.Index(rest, " (HP"); i >= 0 {
				rest = rest[:i]
			}
			c.phrases[class][balanceNumbers.ReplaceAllString(rest, "#")]++
		}
	}
}

func (c *balanceCoverage) report() {
	fmt.Printf("\nMONSTER ABILITIES (fights where it fired / fights its lines could be told apart in)\n")
	for _, key := range sortedKeys(c.fights) {
		def := monster.MonsterConfig.Monsters[key]
		var parts, missing []string
		tracked := map[string]bool{}
		for _, ab := range balanceAbilities {
			tracked[ab.name] = true
			if !ab.authors(def) {
				continue
			}
			n, told := c.fired[key][ab.name], c.told[key][ab.name]
			parts = append(parts, fmt.Sprintf("%s %d/%d", ab.name, n, told))
			if n == 0 && told > 0 {
				missing = append(missing, ab.name)
			}
		}
		// An ability without a row here is invisible to the sweep.
		var untracked []string
		for _, kind := range def.AbilityKinds() {
			if !tracked[kind] {
				untracked = append(untracked, kind)
			}
		}
		if len(parts)+len(untracked) == 0 {
			continue
		}
		line := fmt.Sprintf("  %-24s %s", key, strings.Join(parts, ", "))
		if len(missing) > 0 {
			line += "   NEVER: " + strings.Join(missing, ", ")
		}
		if len(untracked) > 0 {
			line += "   UNTRACKED: " + strings.Join(untracked, ", ")
		}
		fmt.Println(line)
	}
	fmt.Printf("\nELEMENTAL ATTACK seen in fights of: ")
	var elem []string
	for _, key := range sortedKeys(c.elemAtk) {
		elem = append(elem, fmt.Sprintf("%s %d", key, c.elemAtk[key]))
	}
	fmt.Println(strings.Join(elem, ", "))

	fmt.Printf("\nPARTY ACTIONS AND PROCS by class (per fight)\n")
	for _, class := range sortedKeys(c.heroSeen) {
		n := float64(max(1, c.heroSeen[class]))
		type kv struct {
			k string
			v int
		}
		var acts []kv
		for k, v := range c.actions[class] {
			acts = append(acts, kv{k, v})
		}
		sort.Slice(acts, func(i, j int) bool { return acts[i].v > acts[j].v })
		var a []string
		for _, x := range acts {
			a = append(a, fmt.Sprintf("%s %.1f", x.k, float64(x.v)/n))
		}
		var ph []kv
		for k, v := range c.phrases[class] {
			ph = append(ph, kv{k, v})
		}
		sort.Slice(ph, func(i, j int) bool { return ph[i].v > ph[j].v })
		var p []string
		for i, x := range ph {
			if i >= 14 {
				break
			}
			p = append(p, fmt.Sprintf("%q %.1f", x.k, float64(x.v)/n))
		}
		fmt.Printf("  %-12s actions: %s\n  %-12s log: %s\n", class, strings.Join(a, ", "), "", strings.Join(p, "; "))
	}
}
