package main

import (
	"strings"
	"testing"

	"ugataima/internal/boot"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/game"
	"ugataima/internal/monster"
	"ugataima/internal/world"
)

func TestEditorLabelValueSpacing(t *testing.T) {
	for _, label := range []string{"HP:", "Base RT cooldown:", "Personality / 3:", "Secondary stat contribution:"} {
		t.Run(label, func(t *testing.T) {
			const x = 17
			want := x + 6*(len(label)+1)
			if got := tooltipValueX(x, label); got != want {
				t.Fatalf("value starts at %d, want %d (one normal space after label)", got, want)
			}
			if got := game.ShadedTextWidth(label + " 123"); got != want-x+18 {
				t.Fatalf("split label and joined text use different advances: %d", got)
			}
		})
	}
}

func TestEditorConsumableSummaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		def  config.ItemDefinitionConfig
		want []string
	}{
		{"fixed HP", config.ItemDefinitionConfig{HealBase: 25}, []string{"Heals 25 HP"}},
		{"scaled HP", config.ItemDefinitionConfig{HealBase: 25, HealEnduranceDivisor: 2}, []string{"Heals 25 + Endurance/2 HP"}},
		{"fixed SP", config.ItemDefinitionConfig{ManaBase: 30}, []string{"Restores 30 SP"}},
		{"scaled SP", config.ItemDefinitionConfig{ManaBase: 30, ManaPersonalityDivisor: 3}, []string{"Restores 30 + Personality/3 SP"}},
		{"hybrid", config.ItemDefinitionConfig{HealBase: 25, ManaBase: 30}, []string{"Heals 25 HP", "Restores 30 SP"}},
		{"revive", config.ItemDefinitionConfig{Revive: true, FullHeal: true}, []string{"Revives a fallen ally at FULL health"}},
		{"cure", config.ItemDefinitionConfig{CurePoison: true}, []string{"Cures poison"}},
		{"buff", config.ItemDefinitionConfig{BuffArmorClass: 7, BuffDurationSeconds: 20}, []string{"7", "20s"}},
		{"summon", config.ItemDefinitionConfig{SummonDistanceTiles: 2}, []string{"Summons", "2 tiles away"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.def.Type = "consumable"
			c := contentCard{subtitle: itemSubtitle(&tc.def)}
			for _, want := range tc.want {
				if !strings.Contains(c.subtitle, want) {
					t.Errorf("summary %q omitted %q", c.subtitle, want)
				}
			}
			if strings.Contains(c.subtitle, "/0") {
				t.Fatalf("summary divides by zero: %s", c.subtitle)
			}
		})
	}
}

func TestEditorRenderedCatalogText(t *testing.T) {
	t.Chdir("../..")
	cfg, mobs := boot.LoadGameData()
	check := func(name, text string) {
		t.Helper()
		for _, r := range text {
			if r > 127 || (r < 32 && r != '\n') {
				t.Errorf("%s: unsupported glyph/control %U in %q", name, r, text)
				break
			}
		}
		if strings.Contains(text, "%!") {
			t.Errorf("%s: malformed formula or format: %s", name, text)
		}
	}
	for _, cards := range [][]contentCard{buildItemsCards(), buildSpellCards(), buildSkillCards()} {
		for _, c := range cards {
			t.Run(c.key, func(t *testing.T) {
				check(c.key, c.subtitle)
				if w, h := cardTooltipSize(&c); w > windowWidth-8 || h > windowHeight-pageBarHeight-8 {
					t.Errorf("tooltip too large: %dx%d", w, h)
				}
				prose := false
				for _, line := range cardTooltipLines(&c) {
					check(c.key, line.text)
					if line.kind == tooltipLineDescription || line.kind == tooltipLineFlavor {
						prose = true
					}
					if prose && (line.kind == tooltipLineBody || line.kind == tooltipLineSection) {
						t.Error("mechanics follow descriptive prose")
					}
				}
			})
		}
	}
	for _, c := range buildCharacterDetails(cfg) {
		for _, row := range c.rows {
			check(c.portrait, row.text)
		}
	}
	for key, def := range mobs.Monsters {
		for _, row := range buildMobInfoRuntime(key, def, nil, cfg.GetTileSize(), game.MonsterCatalogEffectContext(cfg)) {
			check(key, row.text)
		}
		check(key, strings.Join(mapMonsterStatLines(def), "\n"))
	}
	for _, tiles := range []map[string]*config.TileData{world.GlobalTileManager.ListTiles(), world.GlobalTileManager.ListSpecialTiles()} {
		for key, def := range tiles {
			check(key, strings.Join(appendTileTooltipLines(nil, def), "\n"))
		}
	}
	for key, def := range character.NPCConfigInstance.NPCs {
		check(key, def.Name+"\n"+def.Description)
		check(key, strings.Join(def.AvailabilityLines(), "\n"))
		check(key, strings.Join(character.TrainingOfferLines(def.Training), "\n"))
	}
	maps, err := loadMaps(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range maps {
		for _, row := range buildMapInfoLines(m, brush{kind: brushEraser}) {
			check(m.Key, row.text)
		}
	}
}

// Both editor entry points must distinguish authored combatants, noncombatants
// and champions whose final stats require a runtime tier/loadout.
func TestEditorMonsterAttackText(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		def                          monster.MonsterDefinition
		attacks, elemental, champion bool
	}{
		{"ordinary", monster.MonsterDefinition{}, true, true, false},
		{"wildlife", monster.MonsterDefinition{Disposition: "wildlife"}, true, false, false},
		{"caravan", monster.MonsterDefinition{Disposition: "caravan"}, false, false, false},
		{"fish", monster.MonsterDefinition{Disposition: monster.DispositionFish}, false, false, false},
		{"idol", monster.MonsterDefinition{WarlordIdol: true}, false, false, false},
		{"champion", monster.MonsterDefinition{Champion: "weapon_master"}, true, false, true},
		{"idle animation", monster.MonsterDefinition{AnimateWhenIdle: true}, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.def.Name = "Fixture"
			tc.def.MaxHitPoints = 321
			tc.def.DamageMin, tc.def.DamageMax = 11, 19
			ctx := monster.CombatEffectContext{ElementalAttack: config.ElementalAttackConfig{Chance: .37, DamageMultiplier: 3}}
			var sheet []string
			for _, row := range buildMobInfoRuntime("fixture", tc.def, nil, 64, ctx) {
				sheet = append(sheet, row.text)
			}
			text := strings.Join(sheet, "\n")
			if strings.Contains(text, "TB attacks:") != tc.attacks || strings.Contains(text, "Does not attack") == tc.attacks {
				t.Errorf("wrong combat role in stat sheet:\n%s", text)
			}
			if strings.Contains(text, "Elemental Attack:") != tc.elemental {
				t.Errorf("wrong elemental eligibility:\n%s", text)
			}
			if !tc.attacks && (strings.Contains(text, "Melee:") || strings.Contains(text, "Melee reach")) {
				t.Errorf("noncombatant advertises melee:\n%s", text)
			}
			if strings.Contains(text, "animated idle") != tc.def.AnimateWhenIdle {
				t.Error("idle animation flag not described")
			}
			mapText := strings.Join(mapMonsterStatLines(tc.def), "\n")
			if tc.champion {
				if strings.Contains(mapText, "HP:") || strings.Contains(mapText, "Damage:") || !strings.Contains(mapText, "Stats depend on tier and equipment") {
					t.Errorf("map tooltip exposes placeholder champion stats:\n%s", mapText)
				}
			} else if strings.Contains(mapText, "Damage:") != tc.attacks || !strings.Contains(mapText, "HP: 321") {
				t.Errorf("wrong map stat summary:\n%s", mapText)
			}
		})
	}
}

func TestEditorBossTextMatchesRules(t *testing.T) {
	for _, tc := range []struct {
		name         string
		def          monster.MonsterDefinition
		want, absent []string
	}{
		{"uncapped summons", monster.MonsterDefinition{SummonMonsters: []string{"goblin"}, SummonChance: .25}, []string{"Summons 1x {goblin}: 25% (no live summon cap)"}, []string{"max 0"}},
		{"capped guaranteed summons", monster.MonsterDefinition{SummonMonsters: []string{"goblin"}, SummonCount: 2, SummonMax: 3, SummonFirstGuaranteed: true}, []string{"first guaranteed, then 0% (max 3 alive)"}, nil},
		{"rage damage only", monster.MonsterDefinition{EnrageAtHP: 50, EnrageDamageMult: 1.5}, []string{"at/below 50 HP: dmg x1.5, cd x1.0"}, []string{"x0.0"}},
		{"rage cooldown only", monster.MonsterDefinition{EnrageAtHP: 50, EnrageCooldownMult: .5}, []string{"at/below 50 HP: dmg x1.0, cd x0.5"}, []string{"x0.0"}},
		{"blink threshold", monster.MonsterDefinition{TeleportAtHP: 50, TeleportChance: .5}, []string{"Blinks at/below 50 HP (50%)"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.def.Boss = true
			var lines []string
			for _, row := range buildMobInfoRuntime("fixture", tc.def, nil, 64) {
				lines = append(lines, row.text)
			}
			text := strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Errorf("missing %q in %s", want, text)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(text, absent) {
					t.Errorf("false rule %q in %s", absent, text)
				}
			}
		})
	}
}

func TestEditorMobSheetLayoutKeepsAllRowsReachable(t *testing.T) {
	for _, width := range []int{600, 868} {
		cols, rows, colWidth := mobInfoLayout(width, 250)
		if cols*colWidth > width || game.ShadedTextWidth(strings.Repeat("M", mobInfoCols))+16 > colWidth {
			t.Fatal("stat columns overlap")
		}
		for _, count := range []int{0, 1, rows * cols, rows*cols + 1, 150} {
			capacity := cols * rows
			end := min(count, max(0, count-capacity)+capacity)
			if end != count {
				t.Fatalf("last stat row unreachable: %d/%d", end, count)
			}
		}
	}
}
