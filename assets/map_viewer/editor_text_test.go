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
	for _, c := range buildCharacterDetails(cfg, charTextCols(windowWidth)) {
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

// Range labels describe attack delivery, not the misleadingly generic
// AttackRadius field. Saves are irrelevant: this is a derived catalog view.
func TestEditorMonsterRangeText(t *testing.T) {
	for _, tc := range []struct {
		name         string
		def          monster.MonsterDefinition
		runtime      *monster.Monster3D
		want, absent []string
	}{
		{"melee", monster.MonsterDefinition{AttackRadius: 1}, nil,
			[]string{"Melee reach: 1.0 tiles"}, []string{"Ranged attack range:"}},
		{"long melee", monster.MonsterDefinition{AttackRadius: 2.5}, nil,
			[]string{"Melee reach: 2.5 tiles"}, []string{"Ranged attack range:"}},
		{"ranged spell", monster.MonsterDefinition{AttackRadius: 5, RangedAttackRange: 5, ProjectileSpell: "fixture"}, nil,
			[]string{"Melee: adjacent tiles", "Ranged attack range: 5.0 tiles"}, []string{"Melee reach:"}},
		{"ranged weapon boss", monster.MonsterDefinition{Boss: true, AttackRadius: 2, RangedAttackRange: 7, ProjectileWeapon: "fixture"}, nil,
			[]string{"Melee: adjacent tiles", "Ranged attack range: 7.0 tiles"}, []string{"Melee reach:"}},
		{"unresolved projectile", monster.MonsterDefinition{AttackRadius: 5, ProjectileSpell: "fixture"}, nil,
			[]string{"Melee: adjacent tiles", "Ranged attack range: resolved in preview"}, []string{"Melee reach:", "Ranged attack range: 5.0"}},
		{"staged projectile", monster.MonsterDefinition{AttackRadius: 5, RangedAttackRange: 5},
			&monster.Monster3D{AttackRadius: 7 * 64, ProjectileWeapon: "fixture"},
			[]string{"Melee: adjacent tiles", "Ranged attack range: 7.0 tiles"}, []string{"Melee reach:", "Ranged attack range: 5.0"}},
		{"staged melee", monster.MonsterDefinition{AttackRadius: 1, ProjectileWeapon: "fixture", RangedAttackRange: 5},
			&monster.Monster3D{AttackRadius: 2 * 64},
			[]string{"Melee reach: 2.0 tiles"}, []string{"Ranged attack range:"}},
		{"unresolved champion", monster.MonsterDefinition{Champion: "fixture", AttackRadius: 5}, nil,
			[]string{"Attack reach: tier/loadout-dependent"}, []string{"Melee reach:", "Ranged attack range:"}},
		{"ranged champion", monster.MonsterDefinition{Champion: "fixture", AttackRadius: 5},
			&monster.Monster3D{ChampionKey: "fixture", AttackRadius: 6 * 64, ProjectileWeapon: "fixture"},
			[]string{"Ranged attack range: 6.0 tiles"}, []string{"Melee:", "Melee reach:", "Melee weapon reach:", "Adjacent tiles"}},
		{"melee champion", monster.MonsterDefinition{Champion: "fixture", AttackRadius: 5},
			&monster.Monster3D{ChampionKey: "fixture", AttackRadius: 2 * 64},
			[]string{"Melee weapon reach: 2.0 tiles"}, []string{"Ranged attack range:", "Adjacent tiles"}},
		{"noncombatant", monster.MonsterDefinition{WarlordIdol: true, AttackRadius: 5, RangedAttackRange: 5, ProjectileSpell: "fixture"}, nil,
			[]string{"Does not attack"}, []string{"Melee", "Ranged attack range:", "TB shots:", "Attacks require"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var lines []string
			for _, row := range buildMobInfoRuntime("fixture", tc.def, tc.runtime, 64) {
				lines = append(lines, row.text)
			}
			text := strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Errorf("missing %q in %s", want, text)
				}
			}
			for _, absent := range append(tc.absent, "Effective ranged range", "Attacks require clear line of sight", "TB shots:", "Adjacent tiles also reachable") {
				if strings.Contains(text, absent) {
					t.Errorf("misleading range %q in %s", absent, text)
				}
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

// The catalog reflows at the actual logical viewport, not a fixed canvas.
func TestEditorMobSheetLayoutKeepsAllRowsReachable(t *testing.T) {
	for _, size := range [][2]int{{1200, 800}, {1600, 900}, {1920, 1080}, {800, 1200}} {
		v := &viewer{page: pageMobs}
		w, h := v.Layout(size[0], size[1])
		l := v.mobCatalogLayout()
		if l.list.x+l.list.w >= l.stage.x || l.stage.x+l.stage.w >= l.details.x || l.details.x+l.details.w > w || l.details.y+l.details.h > h {
			t.Fatal("catalog panes overlap or exceed viewport")
		}
	}
}

func TestEditorTelegraphDurationAndShieldText(t *testing.T) {
	for _, tc := range []struct {
		name, kind   string
		sustain      int
		runtime      *monster.Monster3D
		want, absent string
	}{
		{"shield", "shield", 0, nil, "Ally shield: absorbs 15 damage per hit", "sustained damage"},
		{"staged shield", "shield", 0, &monster.Monster3D{DamageMin: 60, DamageMax: 80}, "Ally shield: absorbs 35 damage per hit", "sustained damage"},
		{"timed field", "root", 7, nil, "sustained damage: 7", "Ally shield"},
		{"timed control", "root", 0, nil, "Duration: 4s / 2 rounds", "sustained damage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := monster.MonsterDefinition{DamageMin: 20, DamageMax: 40, Telegraph: &monster.TelegraphedAttack{Kind: tc.kind, Name: "Fixture", DurationSeconds: 4, DurationRounds: 2, SustainDamage: tc.sustain}}
			var b strings.Builder
			for _, row := range buildMobInfoRuntime("fixture", d, tc.runtime, 64) {
				b.WriteString(row.text)
				b.WriteByte('\n')
			}
			if !strings.Contains(b.String(), tc.want) || strings.Contains(b.String(), tc.absent) {
				t.Fatalf("incorrect telegraph text: %s", b.String())
			}
		})
	}
}
