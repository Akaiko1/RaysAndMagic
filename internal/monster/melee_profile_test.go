package monster

import (
	"strings"
	"testing"
	"ugataima/internal/config"
	damagecalc "ugataima/internal/damage"
)

func TestElementalProfileExclusionAndDescription(t *testing.T) {
	rules := config.ElementalAttackConfig{Chance: .37, DamageMultiplier: 3}
	for _, tt := range []struct {
		name     string
		def      MonsterDefinition
		school   string
		eligible bool
	}{
		{"melee", MonsterDefinition{}, "earth", true},
		{"ranged", MonsterDefinition{ProjectileSpell: "firebolt"}, "water", true},
		{"unknown_biome", MonsterDefinition{}, "", true},
		{"idol", MonsterDefinition{WarlordIdol: true}, "earth", false},
		{"weapon_master", MonsterDefinition{Champion: "weapon_master"}, "earth", false},
		{"hobbit_archer", MonsterDefinition{Champion: "hobbit_archer"}, "earth", false},
		{"dark_elf_sorceress", MonsterDefinition{Champion: "dark_elf_sorceress"}, "earth", false},
		{"wild_druid", MonsterDefinition{Champion: "wild_druid"}, "earth", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := CombatEffectContext{ElementalAttack: rules, ElementalSchool: tt.school}
			p := tt.def.MeleeProfile(rules, tt.school)
			calls := 0
			parts, school, proc := p.Resolve(damagecalc.Parts{Normal: 7, True: 2}, func() float64 { calls++; return 0 })
			wantProc := tt.eligible && tt.school != ""
			if proc != wantProc {
				t.Fatalf("proc=%t want %t", proc, wantProc)
			}
			if wantProc {
				if calls != 1 || parts.Normal != 21 || parts.True != 6 || school != tt.school {
					t.Fatalf("hit=%+v %s, rolls=%d", parts, school, calls)
				}
			} else if calls != 0 {
				t.Fatal("excluded or context-free profile consumed RNG")
			}
			var text []string
			for _, l := range tt.def.CombatEffectLines(ctx) {
				text = append(text, l.Text)
			}
			joined := strings.Join(text, "\n")
			if strings.Contains(joined, "Elemental Attack") != tt.eligible {
				t.Fatalf("description eligibility: %s", joined)
			}
			if tt.eligible && (!strings.Contains(joined, "37%") || !strings.Contains(joined, "x3 raw melee damage") || !strings.Contains(joined, "Melee: Physical")) {
				t.Fatalf("description ignored shared tuning: %s", joined)
			}
			if tt.name == "unknown_biome" && !strings.Contains(joined, "biome-dependent") {
				t.Fatalf("catalog guessed biome: %s", joined)
			}
		})
	}
}
