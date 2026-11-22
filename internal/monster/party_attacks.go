package monster

import (
	"fmt"
	"strconv"
	"strings"

	uitext "ugataima/assets/text"
	damagecalc "ugataima/internal/damage"

	"gopkg.in/yaml.v3"
)

// PartyAttack is one authored attack on several party members at once - a
// breath, a fire nova, a piercing volley - from the abilities.party_attacks
// list. Every entry runs through one runtime: each struck member takes its own
// damage roll and its own dodge. Key is its identity, Name what the card
// and the log call it (authored, like a telegraph's name). Its damage is a
// percent of the monster's attack, so auto-level moves it with the attack.
type PartyAttack struct {
	Key           string              `yaml:"key"`
	Name          string              `yaml:"name"`
	Trigger       PartyAttackTrigger  `yaml:"trigger"`
	Delivery      PartyAttackDelivery `yaml:"delivery"`
	School        damagecalc.Type     `yaml:"school"`
	Chance        float64             `yaml:"chance"`
	DamagePercent int                 `yaml:"damage_percent"`
	Targets       PartyAttackTargets  `yaml:"targets"`
	Riders        Riders              `yaml:"riders"`
	IgnoreArmor   bool                `yaml:"ignore_armor,omitempty"`
	RangeTiles    float64             `yaml:"range_tiles,omitempty"` // a nova's reach
}

// DamageBand is the band each struck member rolls in: the monster's attack
// band at DamagePercent.
func (a PartyAttack) DamageBand(attackMin, attackMax int) (int, int) {
	return ScaleDamageBand(attackMin, attackMax, float64(a.DamagePercent)/100)
}

// PartyAttackTrigger is when a party attack rolls.
type PartyAttackTrigger string

const (
	// PartyAttackOnAttack replaces the monster's attack on the party on a roll.
	PartyAttackOnAttack PartyAttackTrigger = "attack"
	// PartyAttackNova is a boss nova: it rolls at the boss's action while the
	// party is within range_tiles, and between attacks on a fixed interval.
	PartyAttackNova PartyAttackTrigger = "nova"
)

// PartyAttackDelivery is the channel a party attack arrives on.
type PartyAttackDelivery string

const (
	// PartyAttackSpell is a spell channel: magic wards absorb it.
	PartyAttackSpell PartyAttackDelivery = "spell"
	// PartyAttackShot is a weapon shot: the ranged ward stops it.
	PartyAttackShot PartyAttackDelivery = "shot"
)

// PartyAttackTargets is `all` (every living member) or a count of members
// drawn at random.
type PartyAttackTargets struct {
	All   bool
	Count int
}

const partyAttackAllTargets = "all"

func (t *PartyAttackTargets) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode && node.Value == partyAttackAllTargets {
		*t = PartyAttackTargets{All: true}
		return nil
	}
	count, err := strconv.Atoi(node.Value)
	if node.Kind != yaml.ScalarNode || err != nil {
		return fmt.Errorf("party attack targets must be %q or a count, got %q", partyAttackAllTargets, node.Value)
	}
	*t = PartyAttackTargets{Count: count}
	return nil
}

// Spell reports a spell-channel attack.
func (a PartyAttack) Spell() bool { return a.Delivery == PartyAttackSpell }

// problems lists the entry's load problems; boss says whether its monster
// may author a nova.
func (a PartyAttack) problems(boss bool) []string {
	var out []string
	add := func(ok bool, format string, args ...any) {
		if !ok {
			out = append(out, fmt.Sprintf("party_attacks %q: "+format, append([]any{a.Key}, args...)...))
		}
	}
	add(a.Key != "", "needs a key")
	add(strings.TrimSpace(a.Name) != "", "needs a name")
	switch a.Trigger {
	case PartyAttackOnAttack:
		add(a.RangeTiles == 0, "range_tiles is a nova's reach; an attack-trigger entry reaches as far as the attack")
	case PartyAttackNova:
		add(boss, "a nova needs boss: true")
		add(a.RangeTiles > 0, "a nova needs range_tiles")
	default:
		add(false, "trigger must be %q or %q, got %q", PartyAttackOnAttack, PartyAttackNova, a.Trigger)
	}
	add(a.Delivery == PartyAttackSpell || a.Delivery == PartyAttackShot,
		"delivery must be %q or %q, got %q", PartyAttackSpell, PartyAttackShot, a.Delivery)
	_, err := damagecalc.ParseType(a.School.String())
	add(err == nil, "needs a known school, got %q", a.School)
	add(a.Chance > 0 && a.Chance <= 1, "needs a chance in (0,1]")
	add(a.DamagePercent > 0 && a.DamagePercent <= MaxAttackPercent, "needs damage_percent in 1..%d", MaxAttackPercent)
	add(a.Riders.Authored(), "needs riders: true or false (whether the monster's on-hit riders ride it)")
	add(a.Targets.All || a.Targets.Count > 0, "targets must be all or a positive count")
	return out
}

// line describes the entry on the monster card.
func (a PartyAttack) line() EffectLine {
	school := a.School.String()
	damage := uitext.Text("monster.party_attack.damage", a.DamagePercent, school)
	targets := uitext.Text("monster.party_attack.targets_all")
	if !a.Targets.All {
		targets = uitext.Text("monster.party_attack.targets_count", a.Targets.Count)
	}
	extras := ""
	if a.RangeTiles > 0 {
		extras += uitext.Text("monster.party_attack.reach", a.RangeTiles)
	}
	if a.IgnoreArmor {
		extras += uitext.Text("monster.party_attack.ignores_armor")
	}
	return EffectLine{
		Text:   uitext.Text("monster.ability.party_attack", a.Name, a.Chance*100, damage, targets, extras),
		School: school,
	}
}
