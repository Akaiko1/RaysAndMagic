package config

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// StatusDefinition is one row of assets/statuses.yaml: a status's names, what
// it does to each side, and the numbers its mechanics read. Durations are not
// here: whoever applies a status authors its length (see ValidateEffectSeconds).
type StatusDefinition struct {
	Key      string `yaml:"-"`
	Name     string `yaml:"name"`
	Short    string `yaml:"short"`
	Party    string `yaml:"party"`
	Monsters string `yaml:"monsters"`
	Rule     string `yaml:"rule"`

	PartyDamagePerSecond    int   `yaml:"party_damage_per_second"`
	MonsterPercentPerSecond int   `yaml:"monster_percent_per_second"`
	BossPercentPerSecond    int   `yaml:"boss_percent_per_second"`
	MonsterMinimumDamage    int   `yaml:"monster_minimum_damage"`
	RepeatPercents          []int `yaml:"repeat_percents"`
	RepeatResetSeconds      int   `yaml:"repeat_reset_seconds"`
	HinderPercent           int   `yaml:"hinder_percent"`
	SkipEveryTurns          int   `yaml:"skip_every_turns"`
	Seconds                 int   `yaml:"seconds"`
}

// statusRule is what the code needs from one status: which sides it reaches
// and which numbers it reads. statusRules is the closed set, in display order.
type statusRule struct {
	key            string
	party, monster bool
	numbers        []string
}

var statusRules = []statusRule{
	{key: "poison", party: true, monster: true, numbers: []string{"party_damage_per_second", "monster_percent_per_second", "monster_minimum_damage"}},
	{key: "burn", party: true, monster: true, numbers: []string{"party_damage_per_second", "monster_percent_per_second", "boss_percent_per_second", "monster_minimum_damage"}},
	{key: "stun", party: true, monster: true, numbers: []string{"repeat_percents", "repeat_reset_seconds"}},
	{key: "root", party: true, monster: true},
	{key: "slow", party: true, monster: true, numbers: []string{"hinder_percent", "skip_every_turns"}},
	{key: "armor_melt", party: true, monster: true, numbers: []string{"seconds"}},
	{key: "weaken", monster: true},
	{key: "armor_shred", monster: true},
	{key: "charm", monster: true},
	{key: "bind", monster: true},
	{key: "fear", party: true, monster: true},
	{key: "silence", monster: true},
	{key: "berserk", monster: true},
	{key: "enrage", monster: true},
	{key: "diseased", party: true},
	{key: "cursed", party: true},
	{key: "asleep", party: true},
	{key: "paralyzed", party: true},
	{key: "stone", party: true},
	{key: "unconscious", party: true},
	{key: "dead", party: true},
	{key: "eradicated", party: true},
}

var globalStatuses map[string]*StatusDefinition

// StatusKeys lists every status in display order.
func StatusKeys() []string {
	keys := make([]string, len(statusRules))
	for i, r := range statusRules {
		keys[i] = r.key
	}
	return keys
}

// Status is the loaded row for key. The key set is closed and checked at load,
// so an unknown key or an unloaded table is a programming error.
func Status(key string) *StatusDefinition {
	if globalStatuses == nil {
		panic("statuses: assets/statuses.yaml is not loaded")
	}
	d := globalStatuses[key]
	if d == nil {
		panic(fmt.Sprintf("statuses: unknown status %q", key))
	}
	return d
}

// MustLoadStatuses loads the status table or panics; it loads before every
// catalog whose durations it validates.
func MustLoadStatuses(path string) {
	if err := LoadStatuses(path); err != nil {
		panic(err)
	}
}

// LoadStatuses reads and validates the status table.
func LoadStatuses(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var file struct {
		Statuses map[string]*StatusDefinition `yaml:"statuses"`
	}
	dec := yaml.NewDecoder(strings.NewReader(string(b)))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil {
		return fmt.Errorf("statuses: %w", err)
	}
	if len(file.Statuses) != len(statusRules) {
		for key := range file.Statuses {
			if !knownStatus(key) {
				return fmt.Errorf("statuses: unknown status %q", key)
			}
		}
	}
	for _, r := range statusRules {
		d := file.Statuses[r.key]
		if d == nil {
			return fmt.Errorf("statuses: %q is missing", r.key)
		}
		d.Key = r.key
		if err := d.validate(r); err != nil {
			return fmt.Errorf("statuses: %s: %w", r.key, err)
		}
	}
	globalStatuses = file.Statuses
	return nil
}

func knownStatus(key string) bool {
	for _, r := range statusRules {
		if r.key == key {
			return true
		}
	}
	return false
}

func (d *StatusDefinition) validate(r statusRule) error {
	if d.Name == "" {
		return fmt.Errorf("name is required")
	}
	if (d.Party != "") != r.party || (d.Monsters != "") != r.monster {
		return fmt.Errorf("party/monsters texts must match the sides the status reaches")
	}
	if (d.Short != "") != r.party {
		return fmt.Errorf("short is required exactly when the status reaches the party")
	}
	for _, n := range d.numberFields() {
		needed := slices.Contains(r.numbers, n.tag)
		if n.set != needed {
			if n.set {
				return fmt.Errorf("%s is not read for this status", n.tag)
			}
			return fmt.Errorf("%s is required", n.tag)
		}
		if !n.set {
			continue
		}
		if n.tag == "repeat_percents" {
			if err := validateRepeatPercents(n.ints); err != nil {
				return err
			}
			continue
		}
		bounds, ok := statusNumberBounds[n.tag]
		if !ok {
			return fmt.Errorf("%s has no bounds", n.tag)
		}
		if x := n.ints[0]; x < bounds[0] || x > bounds[1] {
			return fmt.Errorf("%s %d must be %d..%d", n.tag, x, bounds[0], bounds[1])
		}
	}
	if d.RepeatResetSeconds > 0 {
		if err := ValidateEffectSeconds("repeat_reset_seconds", d.RepeatResetSeconds); err != nil {
			return err
		}
	}
	if d.Seconds > 0 {
		if err := ValidateEffectSeconds("seconds", d.Seconds); err != nil {
			return err
		}
	}
	for _, text := range []string{d.Party, d.Monsters, d.Rule} {
		if _, err := d.render(text); err != nil {
			return err
		}
	}
	return nil
}

// validateRepeatPercents keeps a repeat chain diminishing: it opens at 100,
// never grows, and ends at 0 (immune until the reset window passes).
func validateRepeatPercents(p []int) error {
	if len(p) < 2 || p[0] != 100 || p[len(p)-1] != 0 {
		return fmt.Errorf("repeat_percents must run from 100 down to 0")
	}
	for i := 1; i < len(p); i++ {
		if p[i] < 0 || p[i] > p[i-1] {
			return fmt.Errorf("repeat_percents must never grow: %v", p)
		}
	}
	return nil
}

// statusNumberBounds bounds every number a row may carry (inclusive); seconds
// also need whole rounds and repeat_percents is checked as a whole.
var statusNumberBounds = map[string][2]int{
	"party_damage_per_second":    {1, 1000},
	"monster_percent_per_second": {1, 100},
	"boss_percent_per_second":    {1, 100},
	"monster_minimum_damage":     {1, 1000},
	"repeat_reset_seconds":       {CombatRoundSeconds, 3600},
	"hinder_percent":             {1, 99},
	"skip_every_turns":           {2, 100},
	"seconds":                    {CombatRoundSeconds, 3600},
}

// statusNumber is one numeric field of a row, in declaration order.
type statusNumber struct {
	tag      string
	ints     []int // one value for a plain number, the list for repeat_percents
	set      bool
	rendered string
}

// numberFields lists every numeric field of the row in declaration order.
func (d *StatusDefinition) numberFields() []statusNumber {
	var out []statusNumber
	v, t := reflect.ValueOf(*d), reflect.TypeOf(*d)
	for i := range t.NumField() {
		f := v.Field(i)
		n := statusNumber{tag: t.Field(i).Tag.Get("yaml")}
		switch f.Kind() {
		case reflect.Int:
			n.ints = []int{int(f.Int())}
			n.set = f.Int() != 0
		case reflect.Slice:
			for j := range f.Len() {
				n.ints = append(n.ints, int(f.Index(j).Int()))
			}
			n.set = f.Len() > 0
		default:
			continue
		}
		if n.set {
			parts := make([]string, len(n.ints))
			for j, x := range n.ints {
				parts[j] = strconv.Itoa(x)
			}
			n.rendered = strings.Join(parts, "/")
		}
		out = append(out, n)
	}
	return out
}

// numbers maps every numeric field's yaml name to its rendered value, "" when
// unset.
func (d *StatusDefinition) numbers() map[string]string {
	out := map[string]string{}
	for _, n := range d.numberFields() {
		out[n.tag] = n.rendered
	}
	return out
}

// derived are the placeholder values computed from a row's numbers.
func (d *StatusDefinition) derived() map[string]string {
	out := map[string]string{"armor_cap_percent": strconv.Itoa(ArmorElementalMitigationCap)}
	if d.PartyDamagePerSecond > 0 {
		out["party_damage_per_round"] = strconv.Itoa(d.PartyDamagePerSecond * CombatRoundSeconds)
	}
	if d.RepeatResetSeconds > 0 {
		out["repeat_reset_rounds"] = strconv.Itoa(CombatRounds(d.RepeatResetSeconds))
	}
	if p := d.HinderPercent; p > 0 {
		keep := hinderKeep(p)
		out["recovery_percent"] = strconv.Itoa((100*p + keep/2) / keep)
		out["action_percent"] = strconv.Itoa(keep)
	}
	if d.Seconds > 0 {
		out["duration"] = DurationText(d.Seconds)
	}
	return out
}

var statusPlaceholder = regexp.MustCompile(`\{([a-z_]+)\}`)

func (d *StatusDefinition) render(text string) (string, error) {
	values := d.numbers()
	for k, v := range d.derived() {
		values[k] = v
	}
	var missing string
	out := statusPlaceholder.ReplaceAllStringFunc(text, func(m string) string {
		name := m[1 : len(m)-1]
		if v := values[name]; v != "" {
			return v
		}
		missing = name
		return m
	})
	if missing != "" {
		return "", fmt.Errorf("{%s} has no value in this row", missing)
	}
	return out, nil
}

func (d *StatusDefinition) text(raw string) string {
	out, err := d.render(raw)
	if err != nil {
		panic(err) // validated at load
	}
	return out
}

// PartyText is what the status does to the party ("" when it never does).
func (d *StatusDefinition) PartyText() string { return d.text(d.Party) }

// MonsterText is what the status does to monsters ("" when it never does).
func (d *StatusDefinition) MonsterText() string { return d.text(d.Monsters) }

// RuleText is the mechanic both sides share ("" when there is none).
func (d *StatusDefinition) RuleText() string { return d.text(d.Rule) }

// SlowSkipEveryTurns is how many TB turns a slowed monster spends per skipped
// turn; slow durations come in whole steps of it.
func SlowSkipEveryTurns() int { return Status("slow").SkipEveryTurns }

// HinderPercent is the slow a slowing telegraph lands on whoever it hits: the
// party's movement, recovery and TB actions, or a monster's speed.
func HinderPercent() int { return Status("slow").HinderPercent }

// HinderKeepPercent is what a hindered party keeps: its move speed and TB
// actions, and the share recovery is divided by.
func HinderKeepPercent() int { return hinderKeep(HinderPercent()) }

func hinderKeep(hinderPercent int) int { return 100 - hinderPercent }

// BurnPercent is a burning monster's loss per second, in percent of max HP.
func BurnPercent(boss bool) int {
	if boss {
		return Status("burn").BossPercentPerSecond
	}
	return Status("burn").MonsterPercentPerSecond
}

// ArmorMeltSeconds is how long melted armor lasts.
func ArmorMeltSeconds() int { return Status("armor_melt").Seconds }

// ValidateSlowSeconds is the authoring rule for a slow's duration: whole rounds
// in whole steps of the TB skip cadence, so every slow skips a turn.
func ValidateSlowSeconds(what string, seconds int) error {
	if err := ValidateEffectSeconds(what, seconds); err != nil {
		return err
	}
	if step := SlowSkipEveryTurns(); CombatRounds(seconds)%step != 0 {
		return fmt.Errorf("%s: a slow of %ds lasts %d TB turns; slows last whole steps of %d turns", what, seconds, CombatRounds(seconds), step)
	}
	return nil
}
