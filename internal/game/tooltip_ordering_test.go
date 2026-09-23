package game

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// Case ledger: weapon delivery/cooldown/true damage; spells across all effect
// families and mastery tiers; armor/accessory and healing/stat consumables;
// shop vs wearer and compact vs Shift; comparison direction and effective values.
// Set ownership, exact pieces, and persistence use TestEquippedSetTooltipActivation.
// Other cards are derived on hover: no tooltip state is persisted.
func TestTooltipOrderedWeaponResults(t *testing.T) {
	for _, key := range []string{"hunting_bow", "tanegashima", "broodspike", "suppressor_gun", "archmage_staff", "iron_sword"} {
		for _, speed := range []int{0, 12, 38, 150} {
			for _, dual := range []bool{false, true} {
				for _, full := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/speed%d/dual%v/full%v", key, speed, dual, full), func(t *testing.T) {
						cs := newTestCombatSystemWithConfig(t)
						ch := gmReferenceChar(cs.game.config)
						if !dual {
							delete(ch.Skills, character.SkillDualWielding)
						}
						ch.Speed = speed
						item := items.CreateWeaponFromYAML(key)
						items.EnsureInstanceID(&item)
						ch.Equipment = map[items.EquipSlot]items.Item{items.SlotMainHand: item}
						cs.game.party.Members = []*character.MMCharacter{ch}
						def, _, _ := config.GetWeaponDefinitionByName(item.Name)
						card := GetItemTooltip(item, ch, cs, full)
						for _, bad := range []string{"Current Range:", "Current Projectile Speed:", "Hitbox:", "slower than standard", "faster than standard", "base cooldown"} {
							if strings.Contains(card, bad) {
								t.Fatalf("obsolete row %q: %s", bad, card)
							}
						}
						rng, flight := character.EffectiveWeaponFlight(def, ch)
						if !strings.Contains(card, fmt.Sprintf("Range: %.0f tiles", rng)) {
							t.Fatal(card)
						}
						if full && def.Physics != nil && !strings.Contains(card, fmt.Sprintf("Projectile Speed: %.1f tiles/s", flight)) {
							t.Fatal(card)
						}
						frames := cs.WeaponCooldownFramesFor(ch, item.Name)
						var seconds float64
						for _, line := range strings.Split(card, "\n") {
							if strings.HasPrefix(line, "RT Cooldown:") {
								fmt.Sscanf(line, "RT Cooldown: %fs", &seconds)
							}
						}
						if math.Abs(seconds-float64(frames)/float64(cs.game.config.GetTPS())) > 0.0051 {
							t.Fatalf("cooldown differs from combat: %s", card)
						}
						if strings.Contains(card, "Base weapon cooldown:") != full {
							t.Fatal(card)
						}
						if full && !dual && !strings.Contains(card, "Cooldown limit:") {
							var base, delta float64
							for _, line := range strings.Split(card, "\n") {
								if strings.HasPrefix(line, "Base weapon cooldown:") {
									fmt.Sscanf(line, "Base weapon cooldown: %fs", &base)
								}
								if strings.HasPrefix(line, "Speed (") {
									var stat int
									fmt.Sscanf(line, "Speed (%d): %fs", &stat, &delta)
								}
							}
							if math.Abs(base+delta-seconds) > 0.0051 {
								t.Fatalf("displayed cooldown stages do not add up: %s", card)
							}
						}
						if full && def.TrueDamage > 0 {
							source := strings.Index(card, fmt.Sprintf("Weapon: +%d True", def.TrueDamage))
							if source < 0 || source > strings.Index(card, "Total Damage:") {
								t.Fatalf("authored damage outside breakdown: %s", card)
							}
						}
						preview := cs.calculateWeaponDamagePreview(item, ch)
						for _, want := range []string{fmt.Sprintf("Total Damage: %d", preview.Total), fmt.Sprintf("Critical Damage: %d", preview.CriticalTotal)} {
							if !strings.Contains(card, want) {
								t.Fatal(card)
							}
						}
					})
				}
			}
		}
	}
}

func TestTooltipOrderedSpellResults(t *testing.T) {
	for _, id := range []spells.SpellID{"fireball", "inferno", "earthquake", "firewall", "hot_steam", "heal_other", "heroism", "bless", "stone_skin", "hour_of_power", "day_of_the_gods", "summon_ice_elemental", "charm", "jump"} {
		for _, tier := range []character.SkillMastery{character.MasteryNovice, character.MasteryGrandMaster} {
			for _, full := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/tier%d/full%v", id, tier, full), func(t *testing.T) {
					cs := newTestCombatSystemWithConfig(t)
					ch := gmReferenceChar(cs.game.config)
					for _, school := range ch.MagicSchools {
						school.Mastery = tier
					}
					card := GetSpellTooltip(id, ch, cs, full)
					for _, bad := range []string{"by mastery", "Expert / Master / GM:", "Scales with caster Speed", "Hitbox:", "for 5 minutes", "For 10 minutes"} {
						if strings.Contains(card, bad) {
							t.Fatalf("reference text %q in live card: %s", bad, card)
						}
					}
					def, _ := spells.GetSpellDefinitionByID(id)
					if def.SummonMonster != "" {
						hp := def.SummonHPByMastery[int(tier)]
						damage := def.SummonDamageByMastery[int(tier)]
						if !strings.Contains(card, fmt.Sprintf("Summon HP: %d", hp)) || !strings.Contains(card, fmt.Sprintf("Summon Damage: %d", damage)) {
							t.Fatal(card)
						}
					}
					if def.Duration > 0 {
						want := character.SpellDurationBreakdown(def, ch).Seconds
						if !strings.Contains(card, fmt.Sprintf("Current Duration: %ds", want)) {
							t.Fatal(card)
						}
					}
					if !def.IsBuff() {
						if !strings.Contains(card, cooldownLine(cs, cs.SpellCooldownFrames(ch, id))) {
							t.Fatal(card)
						}
						if full {
							base, delta, final := strings.Index(card, "Base cooldown:"), strings.Index(card, "Speed ("), strings.Index(card, "RT Cooldown:")
							if base < 0 || delta < base || final < delta {
								t.Fatalf("cooldown out of order: %s", card)
							}
						}
					}
				})
			}
		}
	}
}

func TestTooltipComparisonUsesEffectiveValuesAndDirection(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ch := character.CreateCharacter("Mara", character.ClassSniper, cs.game.config)
	ch.Skills[character.SkillBallistics] = &character.Skill{Mastery: character.MasteryGrandMaster}
	ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("hunting_bow")
	cs.game.party.Members = []*character.MMCharacter{ch}
	card := GetItemComparisonTooltip(items.CreateWeaponFromYAML("compound_bow"), ch, cs)
	if !strings.Contains(card, "Range: 10 -> 13 (+3) tiles") {
		t.Fatal(card)
	}
	for _, pair := range [][2]spells.SpellID{{"fireball", "ice_bolt"}, {"ice_bolt", "fireball"}} {
		card = strings.Join(buildSpellComparisonLinesByID(pair[1], pair[0], ch, cs), "\n")
		if strings.Contains(card, " vs ") || !strings.Contains(card, "After equipping (current -> new)") {
			t.Fatal(card)
		}
		old, _ := spells.GetSpellDefinitionByID(pair[0])
		next, _ := spells.GetSpellDefinitionByID(pair[1])
		want := fmt.Sprintf("Spell Points: %d -> %d", cs.effectiveSpellCost(ch, old.SpellPointsCost), cs.effectiveSpellCost(ch, next.SpellPointsCost))
		if !strings.Contains(card, want) {
			t.Fatal(card)
		}
	}
}

func TestTooltipItemContributionAndRecovery(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ch := gmReferenceChar(cs.game.config)
	for _, key := range []string{"magic_ring", "onmyoji_talisman", "leather_armor", "padded_vest", "health_potion", "mana_potion", "antivenom", "revival_potion", "world_map", "ordinary_key", "troll_card"} {
		for _, full := range []bool{false, true} {
			for _, shop := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/full%v/shop%v", key, full, shop), func(t *testing.T) {
					bearer := ch
					if shop {
						bearer = nil
					}
					item := items.CreateItemFromYAML(key)
					def, _ := config.GetItemDefinition(key)
					card := GetItemTooltip(item, bearer, cs, full)
					if strings.Contains(card, "Equipped mitigation:") || strings.Contains(card, "Total Armor Class:") {
						t.Fatal(card)
					}
					if bearer != nil {
						ib, pb := bearer.ItemAttributeScalingBonuses(item)
						for _, row := range []struct {
							stat       string
							div, value int
						}{{"Intellect", def.IntellectScalingDivisor, ib}, {"Personality", def.PersonalityScalingDivisor, pb}} {
							if row.div > 0 && !strings.Contains(card, fmt.Sprintf("%s: +%d", row.stat, row.value)) {
								t.Fatal(card)
							}
						}
					}
					if def.CardRegenPct > 0 && (!strings.Contains(card, "5.0s RT / 3 rounds TB") || strings.Contains(card, "regeneration tick")) {
						t.Fatal(card)
					}
					if def.HealBase > 0 || def.ManaBase > 0 {
						if strings.Index(card, "Auto-use when") < strings.Index(card, "USAGE") {
							t.Fatal(card)
						}
						want := bearer != nil && full && bearer.HasSkill(character.SkillFieldMedicine) && !def.Revive
						if strings.Contains(card, "Field Medicine:") != want {
							t.Fatal(card)
						}
					}
				})
			}
		}
	}
}

func TestTooltipSpellComparisonDamageUnits(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ch := gmReferenceChar(cs.game.config)
	cs.game.combatBuffs = []TimedCombatBuff{{SpellID: "test", Frames: 600, OutBonus: 7, OutDamageType: "all"}}
	types := []struct {
		id    spells.SpellID
		label string
		tick  bool
	}{
		{"fireball", "Total Damage: ", false},
		{"inferno", "Damage: ", false},
		{"earthquake", "Damage: ", false},
		{"firewall", "Total per tick: ", true},
		{"hot_steam", "Total per tick: ", true},
		{"charm", "", false},
		{"heal_other", "", false},
		{"summon_ice_elemental", "", false},
	}
	for _, old := range types {
		for _, next := range types {
			t.Run(string(old.id)+"/"+string(next.id), func(t *testing.T) {
				card := strings.Join(buildSpellComparisonLinesByID(next.id, old.id, ch, cs), "\n")
				for _, tick := range []bool{false, true} {
					a, b := 0, 0
					if old.label != "" && old.tick == tick {
						a = tooltipNumber(t, GetSpellTooltip(old.id, ch, cs, true), old.label)
					}
					if next.label != "" && next.tick == tick {
						b = tooltipNumber(t, GetSpellTooltip(next.id, ch, cs, true), next.label)
					}
					label := "Total Damage"
					if tick {
						label = "Damage per tick"
					}
					if a != b {
						want := fmt.Sprintf("%s: %d -> %d (%+d)", label, a, b, b-a)
						if !strings.Contains(card, want) {
							t.Fatalf("missing %q:\n%s", want, card)
						}
					} else if strings.Contains(card, label+":") {
						t.Fatalf("unchanged or inapplicable damage row:\n%s", card)
					}
				}
				if strings.Contains(card, "Effects: ;") || strings.Contains(card, "by mastery") {
					t.Fatal(card)
				}
				if old.id != next.id && (old.id == "inferno" || next.id == "inferno") && !strings.Contains(card, "Radius: Current map") {
					t.Fatalf("missing nova geometry:\n%s", card)
				}
			})
		}
	}
}

func TestFullWeaponTooltipFitsSmallWindow(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ch := gmReferenceChar(cs.game.config)
	for _, key := range []string{"hunting_bow", "tanegashima", "broodspike", "compound_bow"} {
		card := GetItemTooltip(items.CreateWeaponFromYAML(key), ch, cs, true)
		r := singleTooltipLayout(strings.Split(card, "\n"), nil, true, 780, 590, 800, 600)
		if r.y < tooltipScreenMargin || r.bottom() > 600-tooltipScreenMargin {
			t.Errorf("%s: full card extends off screen: %+v\n%s", key, r, card)
		}
	}
}
