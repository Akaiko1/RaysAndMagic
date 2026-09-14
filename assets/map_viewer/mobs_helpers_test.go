package main

import "ugataima/internal/monster"

// buildMobInfo renders a monster definition into the page's stat + drop
// lines. Zero-valued optional fields are skipped, so the sheet shows exactly
// what the YAML authors.
func buildMobInfo(key string, def monster.MonsterDefinition) []infoLine {
	return buildMobInfoRuntime(key, def, nil, 0)
}
