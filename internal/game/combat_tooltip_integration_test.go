package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

func tooltipNumber(t *testing.T, tooltip, label string) int {
	t.Helper()
	for _, line := range strings.Split(tooltip, "\n") {
		if strings.HasPrefix(line, label) {
			var n int
			if _, err := fmt.Sscanf(strings.TrimPrefix(line, label), "%d", &n); err != nil {
				t.Fatal(err)
			}
			return n
		}
	}
	t.Fatalf("missing %q in tooltip:\n%s", label, tooltip)
	return 0
}

func TestWeaponTooltipStrikeUnits(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		rangeTiles             int
		doubleStrike           bool
		base, volley           int
		wantBase, wantPersonal int
		wantSplit              bool
	}{
		{"single-melee", 3, false, 11, 0, 11, 14, false},
		{"double-even", 3, true, 10, 0, 5, 7, true},
		{"double-odd", 1, true, 11, 0, 6, 7, true},
		{"ranged-double-flag", 4, true, 11, 0, 11, 14, false},
		{"ranged-volley", 4, false, 10, 3, 10, 13, false},
	} {
		for _, personal := range []bool{false, true} {
			for _, full := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/personal%v/full%v", tc.name, personal, full), func(t *testing.T) {
					cs := newTestCombatSystemWithConfig(t)
					weapon, err := items.TryCreateWeaponFromYAML("bronze_cesti")
					if err != nil {
						t.Fatal(err)
					}
					def := lookupWeaponConfigByName(weapon.Name)
					original := *def
					t.Cleanup(func() { *def = original })
					def.Damage, def.Range, def.DoubleStrike, def.Volley = tc.base, tc.rangeTiles, tc.doubleStrike, tc.volley
					cs.game.cardSlots = [MaxCardSlots]cardSlot{}
					var caster *character.MMCharacter
					want := tc.wantBase
					if personal {
						caster = cs.game.party.Members[0]
						isolateTrueDamageMember(caster, 0)
						caster.Might, caster.Speed = 6, 4 // +2 and +1, summed before splitting.
						want = tc.wantPersonal
					}
					text := GetItemTooltip(weapon, caster, cs, full)
					if got := tooltipNumber(t, text, "Total Damage: "); got != want {
						t.Errorf("per-strike damage = %d, want %d:\n%s", got, want, text)
					}
					if strings.Contains(text, "Damage shown per strike") != tc.wantSplit || strings.Contains(text, "Strikes per attack: 2") != tc.wantSplit {
						t.Errorf("per-strike label does not match the attack kind:\n%s", text)
					}
					line := "Per strike: divide Normal formula total by 2, round up"
					if strings.Contains(text, line) != (full && tc.wantSplit) {
						t.Errorf("incorrect split stage in tooltip:\n%s", text)
					}
					if full {
						if got := tooltipNumber(t, text, "Base: "); got != tc.base {
							t.Errorf("source base = %d, want %d", got, tc.base)
						}
						if tc.wantSplit && (strings.Index(text, line) < strings.Index(text, "Base:") || strings.Index(text, line) > strings.Index(text, "Normal Damage:")) {
							t.Error("split must be between source formula and resolved damage")
						}
					}
					for _, r := range text {
						if r > 127 {
							t.Fatalf("non-ASCII character %q in rendered tooltip", r)
						}
					}
				})
			}
		}
	}
}

// Compare rendered numbers to real casts and hit resolution, not another call
// to the preview helper. Both gameplay clocks and all damage delivery forms
// participate; modifiers exercise integer rounding and typed true damage.
func TestSharedSpellFormulaThroughCastAndTooltip(t *testing.T) {
	for _, id := range []spells.SpellID{
		"fireball", "harm", "ray_of_light", "stone_blossom", "hot_steam", "firewall",
		"inferno", "earthquake", "charm", "disintegrate", "bind_undead", "heal_other", "mass_heal",
	} {
		for _, tier := range []character.SkillMastery{character.MasteryNovice, character.MasteryGrandMaster} {
			for _, boosted := range []bool{false, true} {
				for _, tb := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/tier%d/boost%v/tb%v", id, tier, boosted, tb), func(t *testing.T) {
						g, _, _ := tbBehaviorGame(t, 16, 16)
						cs := g.combat
						g.turnBasedMode = tb
						caster := g.party.Members[0]
						isolateTrueDamageMember(caster, 0)
						mustEquipSpellID(t, caster, id)
						for _, school := range caster.MagicSchools {
							school.Mastery = tier
						}
						caster.Intellect, caster.Personality = 37, 23
						caster.MaxHitPoints, caster.HitPoints, caster.SpellPoints = 10000, 5000, 10000
						g.cardSlots = [MaxCardSlots]cardSlot{}
						if boosted {
							caster.Luck = 100 * LuckToCritDivisor
							caster.Skills[character.SkillStrongMagic] = &character.Skill{Mastery: tier}
							caster.Skills[character.SkillNaturalHealer] = &character.Skill{Mastery: tier}
							g.addCombatBuff(TimedCombatBuff{SpellID: "test", Frames: 600, OutBonus: 7, OutDamageType: "all"})
						}
						def, err := spells.GetSpellDefinitionByID(id)
						if err != nil {
							t.Fatal(err)
						}
						if def.OutdoorOnly {
							t.Chdir("../..")
							setTestWorldManager(t, &world.WorldManager{MapConfigs: map[string]*config.MapConfig{"arena": {SkyTexture: "arena_panorama"}}, CurrentMapKey: "arena"})
						}
						target := mkTestMonster("Target", 10000)
						target.ID, target.MonsterType = "formula-target", "undead"
						target.X, target.Y = g.camera.X+32, g.camera.Y
						g.world.Monsters = []*monsterPkg.Monster3D{target}
						full := GetSpellTooltip(id, caster, cs, true)
						compact := GetSpellTooltip(id, caster, cs, false)
						if !cs.CastEquippedSpell() {
							t.Fatal("cast rejected")
						}
						label := "Total Damage: "
						actual := 0
						switch {
						case def.MortarRangeTiles > 0:
							if len(g.pendingMortars) != 1 {
								t.Fatal("mortar not created")
							}
							shot := g.pendingMortars[0]
							target.X, target.Y = shot.X, shot.Y
							cs.detonateMortar(shot)
							if shot.Crit {
								label = "Critical Damage: "
							}
						case def.IsProjectile:
							if len(g.magicProjectiles) != 1 {
								t.Fatal("projectile not created")
							}
							shot := &g.magicProjectiles[0]
							if def.DealsNoDamage {
								if shot.Damage+shot.TrueDamage != 0 || strings.Contains(full, "Total Damage:") {
									t.Fatalf("control spell gained direct damage: %+v\n%s", shot, full)
								}
								return
							}
							cs.applyProjectileDamage(shot, "magic_projectile", target, shot.ID)
							if shot.Crit {
								label = "Critical Damage: "
							}
						case def.ZoneRadiusTiles > 0:
							if len(g.persistentDamageZones) == 0 {
								t.Fatal("zone not created")
							}
							zone := &g.persistentDamageZones[0]
							target.X, target.Y = zone.X, zone.Y
							cs.damageZoneMonsters(string(id), []*PersistentDamageZone{zone}, []*PersistentDamageZone{zone})
							label = "Total per tick: "
						case def.PartyAoeRadiusTiles > 0 || def.MapWide:
							label = "Damage: "
						case def.HealAmount > 0:
							label = "Total Healing: "
							actual = caster.HitPoints - 5000
						}
						if def.HealAmount <= 0 {
							actual = 10000 - target.HitPoints
						}
						for _, tip := range []string{full, compact} {
							if shown := tooltipNumber(t, tip, label); shown != actual {
								t.Errorf("tooltip %d != actual %d (%s)", shown, actual, label)
							}
						}
					})
				}
			}
		}
	}
}

func TestNoDamageSpellComparisonsBothDirections(t *testing.T) {
	for _, id := range []spells.SpellID{"charm", "disintegrate", "bind_undead"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse%v", id, reverse), func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				caster := cs.game.party.Members[0]
				hover, equipped := id, spells.SpellID("fireball")
				if reverse {
					hover, equipped = equipped, hover
				}
				mustEquipSpellID(t, caster, equipped)
				spellItem, err := spells.CreateSpellItem(hover)
				if err != nil {
					t.Fatal(err)
				}
				for _, tip := range []string{GetSpellComparisonTooltip(hover, caster, cs), GetItemComparisonTooltip(spellItem, caster, cs)} {
					for _, line := range strings.Split(tip, "\n") {
						if !strings.HasPrefix(line, "Total Damage:") {
							continue
						}
						var a, b int
						if _, err := fmt.Sscanf(line, "Total Damage: %d vs %d", &a, &b); err != nil {
							t.Fatal(err)
						}
						if reverse {
							a, b = b, a
						}
						if a != 0 || b <= 0 {
							t.Fatalf("control vs damage = %d vs %d", a, b)
						}
					}
					if !strings.Contains(tip, "Total Damage:") {
						t.Fatal("comparison missing")
					}
				}
			})
		}
	}
}

func TestWeaponTooltipMatchesRealAttackStages(t *testing.T) {
	for _, key := range []string{"iron_sword", "hunting_bow", "bronze_cesti", "blowgun"} {
		for _, boosted := range []bool{false, true} {
			for _, crit := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/boost%v/crit%v", key, boosted, crit), func(t *testing.T) {
					cs := newTestCombatSystemWithConfig(t)
					g, caster := cs.game, cs.game.party.Members[0]
					isolateTrueDamageMember(caster, 0)
					weapon, err := items.TryCreateWeaponFromYAML(key)
					if err != nil {
						t.Fatal(err)
					}
					def := lookupWeaponConfigByName(weapon.Name)
					originalCrit := def.CritChance
					t.Cleanup(func() { def.CritChance = originalCrit })
					def.CritChance = 0
					caster.Equipment[items.SlotMainHand] = weapon
					caster.Might, caster.Accuracy = 29, 38
					if crit {
						caster.Luck = 100 * LuckToCritDivisor
					}
					g.cardSlots = [MaxCardSlots]cardSlot{}
					if boosted {
						g.cardSlots[0].key = "masked_serpent_dancer_card"
						g.cardSlots[1].key = "masked_huntress_card"
						g.cardSlots[2].key = "samurai_card"
						g.addCombatBuff(TimedCombatBuff{SpellID: "test", Frames: 600, OutBonus: 7, OutDamageType: "all"})
					}
					target := mkTestMonster("Target", 10000)
					target.ID, target.X, target.Y = "weapon-target", g.camera.X+32, g.camera.Y
					g.world.Monsters = []*monsterPkg.Monster3D{target}
					label := "Total Damage: "
					if crit {
						label = "Critical Damage: "
					}
					shown := tooltipNumber(t, GetItemTooltip(weapon, caster, cs, true), label)
					if !cs.EquipmentMeleeAttack() {
						t.Fatal("attack rejected")
					}
					hits := 1
					if def.DoubleStrike {
						hits = 2
					}
					if def.Range > 3 {
						hits = max(1, def.Volley)
						if len(g.arrows) != hits {
							t.Fatalf("arrows = %d, want %d", len(g.arrows), hits)
						}
						for i := range g.arrows {
							cs.applyProjectileDamage(&g.arrows[i], "arrow", target, g.arrows[i].ID)
						}
					}
					shown *= hits
					if actual := 10000 - target.HitPoints; shown != actual {
						t.Fatalf("tooltip %d != actual %d", shown, actual)
					}
				})
			}
		}
	}
}

func TestAuthoredSpellOverridesReachGameAndEditor(t *testing.T) {
	for _, tc := range []struct {
		name   string
		edit   func(*config.SpellDefinitionConfig)
		want   int
		absent string
	}{
		{"ladder-overrides-cost-and-stats", func(d *config.SpellDefinitionConfig) {
			d.DamageByMastery = []int{11, 23, 47, 95}
			d.DamageCostMultiplier = 2
		}, 11, "Base ("},
		{"self-magic-counts-personality-once", func(d *config.SpellDefinitionConfig) {
			d.School = "body"
			d.ScalesWithPersonality = true
		}, 19, "Intellect /"}, // Fireball's 12 base + 23 Personality / 3.
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			original := config.GlobalSpells.Spells["fireball"]
			def := *original
			tc.edit(&def)
			config.GlobalSpells.Spells["fireball"] = &def
			t.Cleanup(func() { config.GlobalSpells.Spells["fireball"] = original })
			caster := cs.game.party.Members[0]
			isolateTrueDamageMember(caster, 0)
			mustEquipSpellID(t, caster, "fireball")
			caster.SpellPoints = 1000
			caster.Intellect, caster.Personality = 37, 23
			for _, school := range caster.MagicSchools {
				school.Mastery = character.MasteryNovice
			}
			sd, err := spells.GetSpellDefinitionByID("fireball")
			if err != nil {
				t.Fatal(err)
			}
			gameTip := GetSpellTooltip("fireball", caster, cs, true)
			editorTip := strings.Join(character.RenderCardLines(character.SpellCardSections("fireball", &def, sd), true), "\n")
			if strings.Contains(gameTip, tc.absent) || strings.Contains(editorTip, tc.absent) {
				t.Fatalf("inactive formula term %q in game or editor:\n%s\n%s", tc.absent, gameTip, editorTip)
			}
			if !cs.CastEquippedSpell() {
				t.Fatal("cast rejected")
			}
			if len(cs.game.magicProjectiles) != 1 {
				t.Fatal("projectile not created")
			}
			shot := cs.game.magicProjectiles[0]
			if actual := shot.Damage + shot.TrueDamage; actual != tc.want {
				t.Fatalf("cast = %d, want %d", actual, tc.want)
			}
			if shown := tooltipNumber(t, gameTip, "Total Damage: "); shown != tc.want {
				t.Fatalf("tooltip = %d, want %d", shown, tc.want)
			}
		})
	}
}
