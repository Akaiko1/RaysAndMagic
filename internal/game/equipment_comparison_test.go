package game

import (
	"encoding/json"
	"fmt"
	"image/color"
	"maps"
	"slices"
	"strings"
	"testing"
	damagecalc "ugataima/internal/damage"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/stats"
)

func comparisonTestHero(ch *character.MMCharacter) {
	ch.Skills = map[character.SkillType]*character.Skill{}
	for _, skill := range character.AllSkills {
		ch.Skills[skill] = &character.Skill{Mastery: character.MasteryNovice}
	}
	ch.Race = "human"
	ch.Equipment = map[items.EquipSlot]items.Item{}
	ch.Might, ch.Intellect, ch.Personality, ch.Endurance, ch.Accuracy, ch.Speed, ch.Luck = 60, 60, 60, 60, 60, 60, 0
}

// Cross all shipped wearables with every legal replacement (including mixed
// armor/accessory types), empty destinations, and restored saves. Numbers are
// checked against actual equipping, not a second implementation of the formula.
func TestEquipmentComparisonCatalog(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	var catalog []items.Item
	for _, key := range slices.Sorted(maps.Keys(config.GlobalItems.Items)) {
		def := config.GlobalItems.Items[key]
		if def.Type == "armor" || def.Type == "accessory" {
			catalog = append(catalog, items.CreateItemFromYAML(key))
		}
	}
	minW, minH := MinimumWindowSize()
	count := 0
	for _, candidate := range catalog {
		for _, old := range append([]items.Item{{}}, catalog...) {
			slot, _ := items.EquipSlotFromName(config.GlobalItems.Items[mustItemKey(t, candidate)].EquipSlot)
			if old.Name != "" && old.PreferredSlot(items.SlotArmor) != slot {
				continue
			}
			for _, restored := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/restored=%v", candidate.Name, old.Name, restored), func(t *testing.T) {
					ch := cs.game.party.Members[0]
					comparisonTestHero(ch)
					ch.BuffBonuses = stats.Uniform(7)
					ch.BonusMaxHP = 10
					ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("iron_sword")
					ch.Equipment[items.SlotOffHand] = items.CreateWeaponFromYAML("kage_kunai")
					ch.Equipment[items.SlotArmor] = items.CreateItemFromYAML("leather_armor")
					delete(ch.Equipment, slot)
					if old.Name != "" {
						ch.Equipment[slot] = old
					}
					// Full rings deliberately replace Ring 1; the free-ring path is separate.
					if slot == items.SlotRing1 {
						ch.Equipment[items.SlotRing2] = items.CreateItemFromYAML("magic_ring")
					}
					if restored {
						ch = restoreCharacterSave(buildCharacterSave(ch))
						cs.game.party.Members[0] = ch
					}
					ch.RecalculateMaxStatsKeepingCurrent(cs.game.config)
					beforeJSON, _ := json.Marshal(ch)
					oldAC, oldHP, oldSP := cs.CalculateTotalArmorClass(ch), ch.MaxHitPoints, ch.MaxSpellPoints
					oldStats := character.EffectiveCombatStats(ch)
					oldResists := map[string]int{}
					for _, school := range damagecalc.Types() {
						oldResists[school.String()] = cs.game.schoolResistPct(ch, school.String())
					}

					text := GetItemComparisonTooltip(candidate, ch, cs)
					afterJSON, _ := json.Marshal(ch)
					if string(beforeJSON) != string(afterJSON) {
						t.Fatal("preview mutated the live character")
					}
					if text == "" {
						t.Fatal("missing wearable comparison")
					}
					if _, _, ok := ch.EquipItem(candidate); !ok {
						t.Fatal("actual equip refused")
					}
					assertComparisonDelta(t, text, "Armor Class", oldAC, cs.CalculateTotalArmorClass(ch), "")
					assertComparisonDelta(t, text, "Max HP", oldHP, ch.MaxHitPoints, "")
					assertComparisonDelta(t, text, "Max SP", oldSP, ch.MaxSpellPoints, "")
					newStats := character.EffectiveCombatStats(ch)
					for school, beforeResist := range oldResists {
						afterResist := cs.game.schoolResistPct(ch, school)
						if beforeResist == afterResist {
							continue
						}
						want := fmt.Sprintf("%d%% -> %d%% (%+d%%)", beforeResist, afterResist, afterResist-beforeResist)
						found := false
						for _, line := range strings.Split(text, "\n") {
							if strings.Contains(line, want) && (strings.Contains(line, config.TitleWords(school)) || strings.HasPrefix(line, "All resist:")) {
								found = true
							}
						}
						if !found {
							t.Errorf("missing equipped %s resistance: %s\n%s", school, want, text)
						}
					}

					for _, stat := range stats.Names {
						assertComparisonDelta(t, text, config.TitleWords(stat), oldStats.ValueByName(stat), newStats.ValueByName(stat), "")
					}
					for _, size := range [][2]int{{minW, minH}, {1280, 720}, {1920, 1080}} {
						_, height := tooltipBoxSizeForScreen(strings.Split(text, "\n"), nil, false, 0, size[0]/2-tooltipCompareGap, size[1])
						if height > size[1] {
							t.Fatalf("comparison clipped at %dx%d: height %d\n%s", size[0], size[1], height, text)
						}
					}
					count++
				})
			}
		}
	}
	t.Logf("checked %d wearable transitions", count)
}

func mustItemKey(t *testing.T, item items.Item) string {
	t.Helper()
	_, key, ok := config.GetItemDefinitionByName(item.Name)
	if !ok {
		t.Fatal(item.Name)
	}
	return key
}

func assertComparisonDelta(t *testing.T, text, label string, before, after int, unit string) {
	t.Helper()
	if before == after {
		return
	}
	want := fmt.Sprintf("%s: %d%s -> %d%s (%+d%s)", label, before, unit, after, unit, after-before, unit)
	if !strings.Contains(text, want) {
		t.Errorf("missing actual equip result %q in:\n%s", want, text)
	}
}

func TestEquipmentComparisonDestinations(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		equipped  map[items.EquipSlot]string
		want      items.EquipSlot
		label     string
	}{
		{"empty ring", "magic_ring", nil, items.SlotRing1, "Ring 1: Empty"},
		{"second ring", "magic_ring", map[items.EquipSlot]string{items.SlotRing1: "magic_ring"}, items.SlotRing2, "Ring 2: Empty"},
		{"full rings", "magic_ring", map[items.EquipSlot]string{items.SlotRing1: "magic_ring", items.SlotRing2: "magic_ring"}, items.SlotRing1, "Ring 1: "},
		{"empty main", "iron_sword", nil, items.SlotMainHand, "Main Hand: Empty"},
		{"second weapon", "iron_sword", map[items.EquipSlot]string{items.SlotMainHand: "iron_sword"}, items.SlotOffHand, "Off-Hand: Empty"},
		{"shield replaces weapon", "parma_shield", map[items.EquipSlot]string{items.SlotOffHand: "iron_sword"}, items.SlotOffHand, "Off-Hand: Iron Sword"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			ch := cs.game.party.Members[0]
			comparisonTestHero(ch)
			create := func(key string) items.Item {
				if _, ok := config.GlobalWeapons.Weapons[key]; ok {
					return items.CreateWeaponFromYAML(key)
				}
				return items.CreateItemFromYAML(key)
			}
			for slot, key := range tc.equipped {
				ch.Equipment[slot] = create(key)
			}
			item := create(tc.key)
			text := GetItemComparisonTooltip(item, ch, cs)
			if !strings.HasPrefix(text, tc.label) {
				t.Fatalf("wrong destination: %s", text)
			}
			if _, _, ok := ch.EquipItem(item); !ok {
				t.Fatal("equip refused")
			}
			if ch.Equipment[tc.want].InstanceID != item.InstanceID {
				t.Fatal("preview did not follow actual equip destination")
			}
		})
	}
}

func TestEquipmentComparisonCompletedSets(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	for _, restored := range []bool{false, true} {
		for key, set := range config.GlobalItems.Sets {
			t.Run(fmt.Sprintf("%s/restored=%v", key, restored), func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				ch := cs.game.party.Members[0]
				comparisonTestHero(ch)
				// Disable off-hand overflow so the weapon completes its actual main-hand set.
				delete(ch.Skills, character.SkillDualWielding)
				for itemKey, d := range config.GlobalItems.Items {
					if d.Set == key {
						it := items.CreateItemFromYAML(itemKey)
						slot, _ := ch.EquipDestination(it)
						ch.Equipment[slot] = it
					}
				}
				for itemKey, d := range config.GlobalWeapons.Weapons {
					if d.Set == key {
						ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(itemKey)
					}
				}
				if restored {
					ch = restoreCharacterSave(buildCharacterSave(ch))
					cs.game.party.Members[0] = ch
				}
				if !ch.HasCompletedEquipmentSet(key) {
					t.Fatal("fixture failed to complete set")
				}
				full := maps.Clone(ch.Equipment)
				for slot, piece := range full {
					ch.Equipment = maps.Clone(full)
					delete(ch.Equipment, slot)
					if ch.HasCompletedEquipmentSet(key) {
						continue
					}
					text := GetItemComparisonTooltip(piece, ch, cs)
					minW, minH := MinimumWindowSize()
					_, height := tooltipBoxSizeForScreen(strings.Split(text, "\n"), nil, false, 0, minW/2-tooltipCompareGap, minH)
					if height > minH {
						t.Fatalf("set comparison clipped: height=%d max=%d\n%s", height, minH, text)
					}
					if !strings.Contains(text, "Set activated: "+set.Name) {
						t.Fatalf("missing activation: %s", text)
					}
					if _, _, ok := ch.EquipItem(piece); !ok {
						t.Fatal("equip refused")
					}
					if piece.Type == items.ItemWeapon {
						expected := fmt.Sprintf("%d%%", cs.CalculateWeaponCritChance(piece, ch))
						if !strings.Contains(text, expected) {
							t.Fatalf("missing post-equip crit %s: %s", expected, text)
						}
					}
					replacement := items.Item{Type: piece.Type, Name: "Plain test replacement", Attributes: map[string]int{"equip_slot": int(slot)}}
					text = GetItemComparisonTooltip(replacement, ch, cs)
					if !strings.Contains(text, "Set lost: "+set.Name) {
						t.Fatalf("missing loss: %s", text)
					}
				}
			})
		}
	}
}

func TestEquipmentComparisonMechanicsAndContext(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ch := cs.game.party.Members[0]
	comparisonTestHero(ch)
	cs.game.cardSlots[0].key = "treant_card"
	cs.game.cardSlots[1].key = "vengeful_ningyo_card"
	cs.game.combatBuffs = []TimedCombatBuff{{ArmorBonus: 7, OutBonus: 9, ResistSchool: "fire", ResistSchoolPct: 25}}
	for _, key := range []string{"parma_shield", "broodscale_aegis", "drakehide_gauntlets", "deathgod_aegis"} {
		item := items.CreateItemFromYAML(key)
		def, _, _ := config.GetItemDefinitionByName(item.Name)
		ch.Equipment = map[items.EquipSlot]items.Item{}
		text := GetItemComparisonTooltip(item, ch, cs)
		want := def.ItemMechanicLines()
		if line := def.PartyArmorLine(); line != "" {
			want = append(want, line)
		}
		for _, line := range want {
			if !strings.Contains(text, "Gain: "+line) {
				t.Errorf("%s missing gain %s", key, line)
			}
		}
		slot, _ := ch.EquipDestination(item)
		ch.Equipment[slot] = item
		plain := items.Item{Type: item.Type, Name: "Plain replacement", Attributes: map[string]int{"equip_slot": int(slot)}}
		text = GetItemComparisonTooltip(plain, ch, cs)
		for _, line := range want {
			if !strings.Contains(text, "Lose: "+line) {
				t.Errorf("%s missing loss %s", key, line)
			}
		}
	}
	// The snapshot must retain party-only effects, including contributions from
	// another member, but must not replace the live party member pointer.
	ch.Equipment = map[items.EquipSlot]items.Item{}
	other := *ch
	other.Equipment = map[items.EquipSlot]items.Item{items.SlotOffHand: items.CreateItemFromYAML("parma_shield")}
	cs.game.party.Members = append(cs.game.party.Members, &other)
	copy, copyCS := equipmentComparisonContext(cs, ch)
	if copyCS.CalculateTotalArmorClass(copy) != cs.CalculateTotalArmorClass(ch) {
		t.Fatal("preview lost cards or party aura")
	}
	if cs.game.party.Members[0] != ch {
		t.Fatal("preview changed party membership")
	}
	ch.Skills = nil
	if text := GetItemComparisonTooltip(items.CreateItemFromYAML("golden_armor"), ch, cs); !strings.Contains(text, "Cannot equip: requirements not met") {
		t.Fatal("missing eligibility warning")
	}
}

func TestEquipmentComparisonWeaponCatalog(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	keys := slices.Sorted(maps.Keys(config.GlobalWeapons.Weapons))
	minW, minH := MinimumWindowSize()
	for _, key := range keys {
		for _, other := range keys {
			t.Run(key+"/"+other, func(t *testing.T) {
				ch := cs.game.party.Members[0]
				comparisonTestHero(ch)
				delete(ch.Skills, character.SkillDualWielding)
				ch.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(other)
				ch.Equipment[items.SlotArmor] = items.CreateItemFromYAML("golden_armor")
				candidate := items.CreateWeaponFromYAML(key)
				before := cs.calculateWeaponDamagePreview(ch.Equipment[items.SlotMainHand], ch)
				oldCrit := cs.CalculateWeaponCritChance(ch.Equipment[items.SlotMainHand], ch)
				text := GetItemComparisonTooltip(candidate, ch, cs)
				main := GetItemTooltip(candidate, ch, cs, false)
				if _, _, ok := ch.EquipItem(candidate); !ok {
					t.Fatal("equip refused")
				}
				after := cs.calculateWeaponDamagePreview(candidate, ch)
				assertComparisonDelta(t, text, "Damage / hit", before.Total, after.Total, "")
				assertComparisonDelta(t, text, "Critical Chance", oldCrit, cs.CalculateWeaponCritChance(candidate, ch), "%")
				for _, want := range []string{fmt.Sprintf("Total Damage: %d", after.Total), fmt.Sprintf("Chance: %d%%", cs.CalculateWeaponCritChance(candidate, ch))} {
					if !strings.Contains(main, want) {
						t.Fatalf("main card differs from equipped result: missing %s\n%s", want, main)
					}
				}
				for _, size := range [][2]int{{minW, minH}, {1280, 720}, {1920, 1080}} {
					_, height := tooltipBoxSizeForScreen(strings.Split(text, "\n"), nil, false, 0, size[0]/2-tooltipCompareGap, size[1])
					if height > size[1] {
						t.Fatalf("weapon comparison clipped at %dx%d: height=%d\n%s", size[0], size[1], height, text)
					}
				}
			})
		}
	}
}

func TestItemCardUsesEquippedScaling(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	ch := cs.game.party.Members[0]
	comparisonTestHero(ch)
	ch.Equipment[items.SlotArmor] = items.CreateItemFromYAML("leather_armor")
	candidate := items.CreateItemFromYAML("onryo_lamellar")
	card := GetItemTooltip(candidate, ch, cs, false)
	if _, _, ok := ch.EquipItem(candidate); !ok {
		t.Fatal("equip refused")
	}
	want := fmt.Sprintf("Item Armor Class: %d", cs.CalculateArmorClassContribution(candidate, ch))
	if !strings.Contains(card, want) {
		t.Fatalf("candidate card missed its own END scaling: want %s\n%s", want, card)
	}
	// Already equipped Ring 2 must not duplicate itself into Ring 1 for display.
	ch.Equipment[items.SlotRing1] = items.CreateItemFromYAML("magic_ring")
	ring := items.CreateItemFromYAML("magic_ring")
	ch.Equipment[items.SlotRing2] = ring
	before, _ := json.Marshal(ch)
	_ = GetItemTooltip(ring, ch, cs, true)
	after, _ := json.Marshal(ch)
	if string(before) != string(after) {
		t.Fatal("equipped card changed the character")
	}
}

func TestEquipmentComparisonDeltaColors(t *testing.T) {
	for _, tc := range []struct {
		line string
		want color.Color
	}{
		{"Armor Class: 10 -> 15 (+5)", color.RGBA{120, 225, 135, 255}},
		{"Speed: 60 -> 50 (-10)", color.RGBA{245, 135, 120, 255}},
		{"RT recovery: 1.0s -> 0.7s", color.White},
		{"Gain: Hostile statuses on the wearer last 50% as long", color.White},
	} {
		t.Run(tc.line, func(t *testing.T) {
			if got := equipmentComparisonColors([]string{tc.line}, nil)[0]; got != tc.want {
				t.Fatalf("color=%v want %v", got, tc.want)
			}
		})
	}
}
