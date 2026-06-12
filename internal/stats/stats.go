// Package stats holds the ONE definition of the seven character stats: the
// StatBonuses struct. The canonical ordered name list, the name→field mapping
// and the name validator are all DERIVED from its fields via reflection —
// adding a stat means adding exactly one struct field, nothing to sync.
package stats

import (
	"reflect"
	"strings"
)

// StatBonuses is a per-stat additive bonus block — the unit of every temporary
// stat buff. Spells author either a uniform `stat_bonus: N` (all seven stats,
// e.g. Bless) or a per-stat `stat_bonuses:` map; the game aggregates active
// buffs into one StatBonuses and pushes it onto each member's BuffBonuses, so
// combat formulas AND derived stats (MaxHP/MaxSP) see the same numbers.
type StatBonuses struct {
	Might       int
	Intellect   int
	Personality int
	Endurance   int
	Accuracy    int
	Speed       int
	Luck        int
}

// Names is THE canonical, ordered, lowercase stat-name list (the YAML key
// convention), derived from the StatBonuses fields in declaration order.
// Validators, save serialization and tooltip ordering all range over it.
var Names = func() []string {
	t := reflect.TypeOf(StatBonuses{})
	names := make([]string, t.NumField())
	for i := range names {
		names[i] = strings.ToLower(t.Field(i).Name)
	}
	return names
}()

var fieldIndexByName = func() map[string]int {
	m := make(map[string]int, len(Names))
	for i, n := range Names {
		m[n] = i
	}
	return m
}()

// IsName reports whether s is a canonical lowercase stat name.
func IsName(s string) bool {
	_, ok := fieldIndexByName[s]
	return ok
}

// Uniform returns +n to all seven stats (the classic Bless shape).
func Uniform(n int) StatBonuses {
	var b StatBonuses
	v := reflect.ValueOf(&b).Elem()
	for i := 0; i < v.NumField(); i++ {
		v.Field(i).SetInt(int64(n))
	}
	return b
}

// fieldByName returns a pointer to the field for a canonical lowercase stat
// name; FromMap and ValueByName both derive from it. Nil for unknown names.
func (b *StatBonuses) fieldByName(name string) *int {
	i, ok := fieldIndexByName[name]
	if !ok {
		return nil
	}
	return reflect.ValueOf(b).Elem().Field(i).Addr().Interface().(*int)
}

// FromMap builds a StatBonuses from a lowercase stat-name map (the spells.yaml
// `stat_bonuses:` authoring shape). Unknown keys are rejected at spell-config
// load; here they are simply ignored.
func FromMap(m map[string]int) StatBonuses {
	var b StatBonuses
	for name, v := range m {
		if p := b.fieldByName(name); p != nil {
			*p = v
		}
	}
	return b
}

// Add returns the per-stat sum of two bonus blocks.
func (b StatBonuses) Add(o StatBonuses) StatBonuses {
	out := b
	vo := reflect.ValueOf(&out).Elem()
	va := reflect.ValueOf(o)
	for i := 0; i < vo.NumField(); i++ {
		vo.Field(i).SetInt(vo.Field(i).Int() + va.Field(i).Int())
	}
	return out
}

// IsZero reports whether the block carries no bonus at all.
func (b StatBonuses) IsZero() bool {
	return b == StatBonuses{}
}

// ValueByName returns the bonus for a canonical lowercase stat name. Unknown
// names yield 0.
func (b StatBonuses) ValueByName(name string) int {
	if p := b.fieldByName(name); p != nil {
		return *p
	}
	return 0
}
