package game

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

// The weapon card's total is the hit, a buff on its school included: the
// buff meets the mastery's and the weapon's own true damage together, as the
// attack packet carries them, so no rounding of the parts apart drifts.
func TestWeaponCardTotalIsTheBuffedHit(t *testing.T) {
	for _, pct := range []int{0, 10, 25, 50, 75} {
		t.Run(fmt.Sprintf("buff=%d", pct), func(t *testing.T) {
			g := advancedSupportGame(t, false)
			g.cardSlots = [MaxCardSlots]cardSlot{}
			c := g.party.Members[1]
			c.Skills[character.SkillSword] = &character.Skill{Mastery: character.MasteryGrandMaster}
			c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("silver_sword")
			wd := lookupWeaponConfig(c.Equipment[items.SlotMainHand])
			oldCrit, oldTrue := wd.CritChance, wd.TrueDamagePct
			t.Cleanup(func() { wd.CritChance, wd.TrueDamagePct = oldCrit, oldTrue })
			wd.CritChance, wd.TrueDamagePct = 0, 33
			c.Luck = -100 // cancels the Grandmaster's crit bonus: the hit lands plain
			if chance := g.combat.CalculateWeaponCritChance(c.Equipment[items.SlotMainHand], c); chance != 0 {
				t.Fatalf("fixture: crit chance %d%%, want 0", chance)
			}
			school := weaponDamageTypeStr(wd)
			if school == monster.DamagePhysical.String() {
				t.Fatal("fixture: the weapon must deal an element a buff can raise")
			}
			if pct > 0 {
				g.addCombatBuff(TimedCombatBuff{ID: "test", Clock: framesClock(600), OutDamageType: school, OutPercent: pct})
			}
			card := GetItemTooltipRows(c.Equipment[items.SlotMainHand], c, g.combat, false)
			g.selectedChar = 1
			m := mkTestMonster("Plain target", 10000)
			m.ID, m.X = "plain-target", float64(g.config.GetTileSize())
			g.world.Monsters = []*monster.Monster3D{m}
			if !g.combat.EquipmentMeleeAttack() {
				t.Fatal("weapon action rejected")
			}
			dealt := 10000 - m.HitPoints
			want := fmt.Sprintf("Total Damage: %d", dealt)
			if !slices.ContainsFunc(card, func(row character.CardRow) bool { return row.Text == want || strings.HasPrefix(row.Text, want+" (") }) {
				t.Fatalf("the hit dealt %d; card:\n%s", dealt, card)
			}
		})
	}
}

// A wrapped damage split breaks only after a separator: every share keeps its
// number and name on one line and a continuation line starts with a share,
// at every width in every font. A plain row keeps wrapUIText's lines.
func TestDamageSplitWrapsBetweenShares(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardSlots = [MaxCardSlots]cardSlot{}
	for i, key := range []string{"archmage_card", "masked_hexer_girl_card", "isis_card"} {
		g.cardSlots[i].key = key
	}
	item := items.CreateWeaponFromYAML("iron_sword")
	ch := character.CreateCharacter("Hero", character.ClassKnight, g.config)
	ch.Skills[character.SkillSword] = &character.Skill{Mastery: character.MasteryGrandMaster}
	ch.Equipment = map[items.EquipSlot]items.Item{items.SlotMainHand: item}
	g.party.Members = []*character.MMCharacter{ch}
	var split, plain character.CardRow
	for _, row := range GetItemTooltipRows(item, ch, cs, true) {
		switch {
		case strings.HasPrefix(row.Text, "Total Damage:"):
			split = row
		case len(row.Runs) == 0 && len(strings.Fields(row.Text)) > len(strings.Fields(plain.Text)):
			plain = row
		}
	}
	open := strings.Index(split.Text, "(")
	shares := regexp.MustCompile(`\d+( [A-Za-z]+)?`).FindAllString(split.Text[open:], -1)
	if len(shares) < 5 {
		t.Fatalf("fixture: want three conversions and true damage, got %q", split.Text)
	}
	forEachUIFont(t, func(t *testing.T) {
		wrapped := false
		for w := uiTextWidth(split.Text) + 12; w >= uiTextWidth(split.Text[:open+1]+shares[0]+" +")+12; w-- {
			var lines []string
			for _, r := range tooltipLayoutRows(character.CardRows{split}, false, w, 16).rows {
				lines = append(lines, r.text)
			}
			wrapped = wrapped || len(lines) > 1
			for _, share := range shares {
				if !slices.ContainsFunc(lines, func(line string) bool { return strings.Contains(line, share) }) {
					t.Fatalf("width %d: share %q split across %q", w, share, lines)
				}
			}
			for i, line := range lines {
				if i > 0 && strings.HasPrefix(line, "+") || i < len(lines)-1 && len(strings.Join(lines[:i+1], " ")) > open && !strings.HasSuffix(line, "+") {
					t.Fatalf("width %d: a line break away from a separator in %q", w, lines)
				}
			}
			var plainLines []string
			for _, r := range tooltipLayoutRows(character.CardRows{plain}, false, w, 16).rows {
				plainLines = append(plainLines, r.text)
			}
			if want := wrapUIText(strings.Join(strings.Fields(plain.Text), " "), max(uiTextCharWidth, w-12)); !slices.Equal(plainLines, want) {
				t.Fatalf("width %d: plain row %q, wrapUIText gives %q", w, plainLines, want)
			}
		}
		if !wrapped {
			t.Fatal("no width wrapped the split")
		}
	})
}

// Compact/full, base/live, and every source of true damage. These use the
// public dispatcher, so moving the summary back behind Shift fails the test.
func TestTooltipTrueDamageAlwaysVisible(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	for _, tc := range []struct {
		name, key                 string
		mastery, card, base, want bool
		share, tint               string // the weapon's share name and its school's tint
		aura                      bool   // the bearer wears Fire Aura
	}{
		{"normal", "iron_sword", false, false, false, false, "Normal", "", false},
		{"authored base", "broodspike", false, false, true, true, "Normal", "", false},
		{"authored live", "broodspike", false, false, false, true, "Normal", "", false},
		{"mastery melee", "iron_sword", true, false, false, true, "Normal", "", false},
		{"mastery ranged", "hunting_bow", true, false, false, true, "Normal", "", false},
		{"mastery elemental", "silver_sword", true, false, false, true, "Light", "light", false},
		{"card", "iron_sword", false, true, false, true, "Normal", "", false},
		{"card elemental", "silver_sword", false, true, false, true, "Light", "light", false},
		{"all sources", "broodspike", true, true, false, true, "Normal", "", false},
		{"multiple strikes", "suppressor_gun", true, false, false, true, "Normal", "", false},
		{"base isolation", "iron_sword", true, true, true, false, "Normal", "", false},
		{"aura", "iron_sword", false, false, false, false, "Normal", "", true},
		{"aura ranged", "hunting_bow", true, false, false, true, "Normal", "", true},
		{"aura elemental", "silver_sword", true, false, false, true, "Light", "light", true},
		{"aura same school", "archmage_staff", true, false, false, true, "Fire", "fire", true},
		{"aura same school alone", "archmage_staff", false, false, false, false, "Fire", "fire", true},
	} {
		for _, full := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/full=%v", tc.name, full), func(t *testing.T) {
				ch := gmReferenceChar(cs.game.config)
				if !tc.mastery {
					ch.Skills = map[character.SkillType]*character.Skill{}
				}
				if tc.aura {
					ch.SpellState.Effects = []character.PersonalSpellEffect{{SpellID: "fire_aura", Charges: 1}}
				}
				it := items.CreateWeaponFromYAML(tc.key)
				items.EnsureInstanceID(&it)
				ch.Equipment = map[items.EquipSlot]items.Item{items.SlotMainHand: it}
				cs.game.party.Members = []*character.MMCharacter{ch}
				cs.game.cardSlots = [MaxCardSlots]cardSlot{}
				if tc.card {
					cs.game.setCardCollectionSlot(0, items.CreateItemFromYAML("samurai_card"))
				}
				if tc.base {
					ch = nil
				}
				rows := GetItemTooltipRows(it, ch, cs, full)
				text := rows.String()
				calculator := cs
				if tc.base {
					calculator = nil
				}
				damage := calculator.calculateWeaponDamagePreview(it, ch)
				wantComponents := 1
				if tc.aura {
					wantComponents = 2 // the weapon's, then the aura's
				}
				if (damage.True > 0) != tc.want || len(damage.Shares) != wantComponents {
					t.Fatalf("fixture lacks intended damage source: %+v", damage)
				}
				// The split names each school once, as the combat log does: an
				// aura of the weapon's own school joins the weapon's share, and
				// beside an element the physical share is a bare number.
				type share struct {
					name, tint string
					amount     int
				}
				shares := []share{{tc.share, tc.tint, damage.Shares[0].Amount}}
				if tc.aura && damage.Shares[1].School == tc.tint {
					shares[0].amount += damage.Shares[1].Amount
				} else if tc.aura {
					shares = append(shares, share{"Fire", "fire", damage.Shares[1].Amount})
				}
				named := map[string]string{}
				var pieces []string
				elemental := slices.ContainsFunc(shares, func(sh share) bool { return sh.tint != "" })
				for _, sh := range shares {
					piece := fmt.Sprintf("%d %s", sh.amount, sh.name)
					if elemental && sh.tint == "" {
						piece = fmt.Sprint(sh.amount)
					}
					named[piece], pieces = sh.tint, append(pieces, piece)
				}
				if tc.want {
					piece := fmt.Sprintf("%d True", damage.True)
					named[piece], pieces = tc.tint, append(pieces, piece)
				}
				summary := fmt.Sprintf("Total Damage: %d", damage.Total)
				if len(pieces) > 1 {
					summary += " (" + strings.Join(pieces, " + ") + ")"
				}
				if !strings.Contains(text, summary+"\n") {
					t.Fatalf("missing compact damage total %q: %s", summary, text)
				}
				typeLine := tc.share + " Damage"
				if tc.tint == "" {
					typeLine = "Physical Damage"
				}
				typeRow := typeLine // the school's line, splash radius after it
				for _, line := range rows.Lines() {
					if strings.HasPrefix(line, typeLine+" - ") {
						typeRow = line
					}
				}
				assertShareTints(t, rows, typeRow, map[string]string{typeLine: tc.tint})
				if len(pieces) > 1 {
					assertShareTints(t, rows, summary, named)
				}
				for _, sh := range shares {
					total := fmt.Sprintf("%s Damage: %d", sh.name, sh.amount)
					if shown := slices.Contains(rows.Lines(), total); shown != (full && len(pieces) > 1) {
						t.Fatalf("share total %q shown=%v:\n%s", total, shown, text)
					} else if shown {
						assertShareTints(t, rows, total, map[string]string{total: sh.tint})
					}
				}
				if tc.want && full {
					assertTruePartTints(t, rows, tc.tint)
				}
				if !tc.want && strings.Contains(text, " True)") {
					t.Fatalf("false true-damage summary: %s", text)
				}
				if strings.Contains(text, "True Damage ignores armor and dodge") != (tc.want && full) {
					t.Fatalf("bypass explanation must be Shift-only and require true damage: %s", text)
				}
			})
		}
	}
	shares := map[spells.SpellID][2]string{"fireball": {"Fire", "fire"}, "hot_steam": {"Water", "water"}}
	for _, id := range []spells.SpellID{"fireball", "hot_steam", "firewall", "inferno"} {
		for _, tier := range []character.SkillMastery{character.MasteryNovice, character.MasteryGrandMaster} {
			for _, full := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/tier%d/full%v", id, tier, full), func(t *testing.T) {
					ch := gmReferenceChar(cs.game.config)
					for _, skill := range ch.MagicSchools {
						skill.Mastery = tier
					}
					cs.game.cardSlots = [MaxCardSlots]cardSlot{}
					rows := GetSpellTooltipRows(id, ch, cs, full)
					text := rows.String()
					want := tier == character.MasteryGrandMaster && (id == "fireball" || id == "hot_steam")
					if strings.Contains(text, " True)") != want {
						t.Fatal(text)
					}
					if want {
						def, err := spells.GetSpellDefinitionByID(id)
						if err != nil {
							t.Fatal(err)
						}
						breakdown := character.SpellDamageBreakdown(def, ch)
						parts := cs.spellDamageParts(id, ch, breakdown.Total)
						parts, _, _ = cs.spellPartsWithOutgoingBuff(parts, def.School)
						label := "Total Damage"
						if def.DamageFormula().Kind == spells.DamageZone {
							label = "Total per tick"
						}
						share, tint := shares[id][0], shares[id][1]
						summary := fmt.Sprintf("%s: %d (%d %s + %d True)", label, parts.Total(), parts.Normal, share, parts.True)
						if !strings.Contains(text, summary+"\n") {
							t.Fatalf("missing compact damage total %q: %s", summary, text)
						}
						assertShareTints(t, rows, summary, map[string]string{fmt.Sprintf("%d %s", parts.Normal, share): tint, fmt.Sprintf("%d True", parts.True): tint})
						if full {
							assertTruePartTints(t, rows, tint)
						}
					}
					if strings.Contains(text, "True Damage ignores armor and dodge") != (want && full) {
						t.Fatalf("bypass explanation must be Shift-only and require true damage: %s", text)
					}
				})
			}
		}
	}
}

// assertShareTints finds the row reading text: each named share carries its
// tint ("" for physical damage), every other piece of the row none.
func assertShareTints(t *testing.T, rows character.CardRows, text string, shares map[string]string) {
	t.Helper()
	for _, row := range rows {
		if row.Text != text {
			continue
		}
		found := 0
		for _, run := range row.Runs {
			want, named := shares[run.Text]
			if named {
				found++
			}
			if run.School != want {
				t.Fatalf("%q in %q tinted %q, want %q", run.Text, text, run.School, want)
			}
		}
		if found != len(shares) {
			t.Fatalf("row %q runs %+v lack the shares %v", text, row.Runs, shares)
		}
		return
	}
	t.Fatalf("no row %q in:\n%s", text, rows)
}

// assertTruePartTints: on the Shift card each source of true damage ("+9
// True", "+15 Fire True Damage") is a run in the school's tint.
func assertTruePartTints(t *testing.T, rows character.CardRows, tint string) {
	t.Helper()
	parts := 0
	for _, row := range rows {
		for _, run := range row.Runs {
			if !strings.HasPrefix(run.Text, "+") || !strings.Contains(run.Text, " True") {
				continue
			}
			parts++
			if run.School != tint {
				t.Fatalf("%q in %q tinted %q, want %q", run.Text, row.Text, run.School, tint)
			}
		}
	}
	if parts == 0 {
		t.Fatalf("no tinted true-damage part:\n%s", rows)
	}
}

func TestItemTooltipPreservesDistinctAuthoredProse(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	for _, weapon := range []bool{false, true} {
		for _, tc := range []struct{ name, description, flavor string }{
			{"neither", "", ""}, {"description", "Authored description.", ""},
			{"flavor", "", "Authored flavor."}, {"both", "Authored description.", "Authored flavor."},
			{"identical", "Same authored text.", "Same authored text."},
		} {
			t.Run(fmt.Sprintf("weapon%v/%s", weapon, tc.name), func(t *testing.T) {
				var it items.Item
				if weapon {
					def, _ := config.GetWeaponDefinition("tonbogiri")
					original := *def
					defer func() { *def = original }()
					def.Description, def.Flavor = tc.description, tc.flavor
					it = items.CreateWeaponFromYAML("tonbogiri")
				} else {
					def, _ := config.GetItemDefinition("troll_card")
					original := *def
					defer func() { *def = original }()
					def.Description, def.Flavor = tc.description, tc.flavor
					it = items.CreateItemFromYAML("troll_card")
				}
				it.Description = "Obsolete saved prose."
				for _, full := range []bool{false, true} {
					text := GetItemTooltipRows(it, nil, nil, full).String()
					for _, want := range []string{tc.description, tc.flavor} {
						if want != "" && strings.Count(text, want) != 1 {
							t.Fatalf("missing or repeated %q: %s", want, text)
						}
					}
					if strings.Contains(text, it.Description) {
						t.Fatal("known item uses stale saved prose")
					}
				}
			})
		}
	}
	legacy := items.Item{Name: "Removed item", Type: items.ItemQuest, Description: "Only surviving description."}
	if text := GetItemTooltipRows(legacy, nil, nil, true).String(); strings.Count(text, legacy.Description) != 1 {
		t.Fatal(text)
	}
}

func TestMonsterSpellPublicTooltipsDescribeMonsterCasting(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	checked := 0
	for key, def := range config.GlobalSpells.Spells {
		if !def.MonsterOnly {
			continue
		}
		checked++
		for _, full := range []bool{false, true} {
			for _, live := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/full%v/live%v", key, full, live), func(t *testing.T) {
					var ch *character.MMCharacter
					if live {
						ch = gmReferenceChar(cs.game.config)
					}
					it, err := spells.CreateSpellItem(spells.SpellID(key))
					if err != nil {
						t.Fatal(err)
					}
					texts := []string{GetSpellTooltipRows(spells.SpellID(key), ch, cs, full).String(), GetItemTooltipRows(it, ch, cs, full).String()}
					if !live {
						texts = append(texts, GetSpellTooltipRows(spells.SpellID(key), nil, nil, full).String()) // the editor card's call
					}
					for _, text := range texts {
						if !strings.Contains(text, "Cast by monsters only") {
							t.Fatal(text)
						}
						for _, bad := range []string{"Hitbox:", "Cost:", "Intellect /", "Mastery -", "Mastery:", "Base ("} {
							if strings.Contains(text, bad) {
								t.Fatalf("monster spell contains player/internal rule %q: %s", bad, text)
							}
						}
						sd, _ := spells.GetSpellDefinitionByID(spells.SpellID(key))
						if sd.IsProjectile && !sd.DealsNoDamage && !strings.Contains(text, "casting monster's attack damage") {
							t.Fatal(text)
						}
					}
				})
			}
		}
	}
	if checked == 0 {
		t.Fatal("no monster spell fixtures")
	}
	text := GetSpellTooltipRows("fireball", gmReferenceChar(cs.game.config), cs, true).String()
	if strings.Contains(text, "Cast by monsters only") || !strings.Contains(text, "Cost:") {
		t.Fatal(text)
	}
}

// A flask's and a spell's card name the hit with the party's buff on its
// school, and what that buff added, as weapon and trap cards do: the flask's
// against a real burst, the spell's as the gain over its unbuffed card.
func TestSchoolBuffLineOnFlaskAndSpellCards(t *testing.T) {
	buff := func(g *MMGame, school string) {
		g.addCombatBuff(TimedCombatBuff{ID: "test", Clock: framesClock(600), OutDamageType: school, OutPercent: 50})
	}
	buffLine := func(school string, n int) string {
		return uitext.Text("damage.school_buff", config.TitleWords(school), n)
	}
	t.Run("flask", func(t *testing.T) {
		const key = "fire_flask"
		d, _ := config.GetItemDefinition(key)
		burst := func(buffed bool) (card string, dealt int) {
			g, c := rareClassGame(t, character.ClassAlchemist, false)
			if buffed {
				buff(g, d.Flask.Element)
			}
			g.party.Inventory = nil
			item := items.CreateItemFromYAML(key)
			g.party.AddItem(item)
			card = GetItemTooltipRows(item, c, g.combat, true).String()
			if !g.throwFlask(0, key, false) {
				t.Fatal("flask did not launch")
			}
			m := zoneVictim(t, g)
			g.combat.detonateFlask(&g.flasks[0], m.X, m.Y)
			return card, m.MaxHitPoints - m.HitPoints
		}
		card, dealt := burst(true)
		plain, unbuffed := burst(false)
		if dealt <= unbuffed {
			t.Fatalf("fixture: the buff added nothing (%d vs %d)", dealt, unbuffed)
		}
		for _, want := range []string{fmt.Sprintf("Total Damage: %d to each victim", dealt), buffLine(d.Flask.Element, dealt-unbuffed)} {
			if !strings.Contains(card, want) {
				t.Fatalf("buffed card lacks %q:\n%s", want, card)
			}
		}
		if !strings.Contains(plain, fmt.Sprintf("Total Damage: %d to each victim", unbuffed)) || strings.Contains(plain, " buff: +") {
			t.Fatalf("unbuffed card diverged from its burst (%d):\n%s", unbuffed, plain)
		}
	})
	t.Run("spell", func(t *testing.T) {
		cs := newTestCombatSystemWithConfig(t)
		caster := gmReferenceChar(cs.game.config)
		total := func() (string, int) {
			card := GetSpellTooltipRows("fireball", caster, cs, true).String()
			n := 0
			if at := strings.Index(card, "Total Damage: "); at >= 0 {
				fmt.Sscanf(card[at:], "Total Damage: %d", &n)
			}
			return card, n
		}
		plain, unbuffed := total()
		buff(cs.game, "fire")
		card, buffed := total()
		if buffed <= unbuffed || strings.Contains(plain, " buff: +") {
			t.Fatalf("fixture: buffed %d, unbuffed %d:\n%s", buffed, unbuffed, plain)
		}
		if line := buffLine("fire", buffed-unbuffed); !strings.Contains(card, line) {
			t.Fatalf("card lacks %q:\n%s", line, card)
		}
	})
}
