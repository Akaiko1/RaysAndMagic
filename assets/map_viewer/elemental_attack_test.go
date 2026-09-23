package main

import (
	"strings"
	"testing"
	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/monster"
)

func TestElementalEditorCatalogUsesConfiguredProfile(t *testing.T) {
	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
	cfg.MonsterCombat.ElementalAttack = config.ElementalAttackConfig{Chance: .37, DamageMultiplier: 3}
	for key, def := range monster.MonsterConfig.Monsters {
		t.Run(key, func(t *testing.T) {
			ctx := game.MonsterCatalogEffectContext(cfg)
			rows := buildMobInfoRuntime(key, def, nil, 0, ctx)
			var texts []string
			for _, row := range rows {
				texts = append(texts, row.text)
			}
			text := strings.Join(strings.Fields(strings.Join(texts, " ")), " ")
			excluded := def.Champion != "" || def.WarlordIdol || def.Disposition != ""
			if excluded {
				if strings.Contains(text, "Elemental Attack:") {
					t.Fatal("excluded monster acquired elemental row")
				}
			} else {
				for _, want := range []string{"Melee: Physical", "Elemental Attack: 37%, x3 raw melee damage (biome-dependent)"} {
					if !strings.Contains(text, want) {
						t.Fatalf("missing %q in %s", want, text)
					}
				}
			}
			for _, c := range text {
				if c > 127 {
					t.Fatalf("non-ASCII editor card %q", text)
				}
			}
		})
	}
}

func TestElementalEditorMapsCarryAuthoredBiome(t *testing.T) {
	t.Chdir("../..")
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	maps, err := loadMaps(cfg)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, m := range maps {
		seen[m.Config.Biome] = true
		ctx := game.MonsterCatalogEffectContext(cfg)
		ctx.ElementalSchool = m.Biome.ElementalAttackSchool
		lines := (monster.MonsterDefinition{}).CombatEffectLines(ctx)
		found := false
		for _, line := range lines {
			if strings.HasPrefix(line.Text, "Elemental Attack:") {
				found = true
				if line.School == "" || !strings.Contains(line.Text, "("+m.Biome.ElementalAttackSchool+")") {
					t.Fatalf("map %s lost authored school: %+v", m.Key, line)
				}
			}
		}
		if !found {
			t.Fatalf("map %s lost elemental description", m.Key)
		}
	}
	if len(seen) != 19 {
		t.Fatalf("covered %d biomes", len(seen))
	}
}
