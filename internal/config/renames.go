package config

import (
	"fmt"
	"slices"
	"strings"
)

// Renamed is one catalog entry as the rename rule sees it: its key, its live
// display name, and the former names and keys saves may still hold.
type Renamed struct {
	Key, Name               string
	LegacyNames, LegacyKeys []string
}

// ValidateRenames is the one rename rule of a catalog (weapons, items,
// monsters): a former name or key is trimmed and not empty, never names a
// live entry, and belongs to one entry only. uniqueNames also forbids two
// live entries sharing a display name. It returns each former name's key.
func ValidateRenames(kind string, entries []Renamed, uniqueNames bool) (map[string]string, error) {
	slices.SortFunc(entries, func(a, b Renamed) int { return strings.Compare(a.Key, b.Key) })
	liveKeys := make(map[string]bool, len(entries))
	liveNames := make(map[string]string, len(entries))
	for _, e := range entries {
		liveKeys[e.Key] = true
		if prev, dup := liveNames[e.Name]; dup && uniqueNames {
			return nil, fmt.Errorf("%ss %q and %q share display name %q - display names must be unique", kind, prev, e.Key, e.Name)
		}
		liveNames[e.Name] = e.Key
	}
	legacyNames, legacyKeys := map[string]string{}, map[string]string{}
	for _, e := range entries {
		for _, list := range []struct {
			what    string
			aliases []string
			live    func(string) bool
			claimed map[string]string
		}{
			{"name", e.LegacyNames, func(n string) bool { _, ok := liveNames[n]; return ok }, legacyNames},
			{"key", e.LegacyKeys, func(k string) bool { return liveKeys[k] }, legacyKeys},
		} {
			for _, old := range list.aliases {
				if old == "" || strings.TrimSpace(old) != old {
					return nil, fmt.Errorf("%s %q: legacy %s %q is empty or untrimmed", kind, e.Key, list.what, old)
				}
				if list.live(old) {
					return nil, fmt.Errorf("%s %q: legacy %s %q still names a live %s", kind, e.Key, list.what, old, kind)
				}
				if prev, dup := list.claimed[old]; dup {
					return nil, fmt.Errorf("%ss %q and %q both claim legacy %s %q", kind, prev, e.Key, list.what, old)
				}
				list.claimed[old] = e.Key
			}
		}
	}
	return legacyNames, nil
}
