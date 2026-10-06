package game

import (
	"fmt"
	"slices"
	"testing"

	uitext "ugataima/assets/text"
	"ugataima/internal/config"
	"ugataima/internal/spells"
	"ugataima/internal/world"
)

// Every active magnitude belongs to the running buff, even when current YAML
// differs from the cast or saved value. Static utility mechanics stay visible.
func TestBuffHoverEffectsAppearOnce(t *testing.T) {
	cases := []struct {
		name, key string
		perStat   bool
		want      func(*MMGame, spells.SpellDefinition) []string
	}{
		{"uniform_stats", "bless", false, func(g *MMGame, _ spells.SpellDefinition) []string {
			b, _ := g.statBuffByID("bless")
			return []string{uitext.Text("spell.to_all_stats_whole_party", b.Bonuses.Might)}
		}},
		{"per_stat", "bless", true, func(_ *MMGame, _ spells.SpellDefinition) []string {
			return []string{"+9 Might (whole party)", "-3 Speed (whole party)"}
		}},
		{"outgoing_damage", "heroism", false, func(g *MMGame, _ spells.SpellDefinition) []string {
			b, _ := g.combatBuffByID("heroism")
			return []string{uitext.Text("spell.party_deal_damage", spells.OutgoingDamageTarget(b.OutDamageType), b.OutBonus)}
		}},
		{"flat_soak", "stone_skin", false, func(g *MMGame, _ spells.SpellDefinition) []string {
			b, _ := g.combatBuffByID("stone_skin")
			return []string{uitext.Text("spell.party_takes_damage_per_hit", b.InReduce)}
		}},
		{"incoming_damage", "day_of_the_gods", false, func(g *MMGame, _ spells.SpellDefinition) []string {
			b, _ := g.combatBuffByID("day_of_the_gods")
			return []string{uitext.Text("spell.party_takes_less_damage", b.ResistPct)}
		}},
		{"school_resistance", "fire_shield", false, func(g *MMGame, _ spells.SpellDefinition) []string {
			b, _ := g.combatBuffByID("fire_shield")
			return []string{uitext.Text("spell.party_resists_for_the_duration", config.TitleWords(b.ResistSchool), b.ResistSchoolPct)}
		}},
		{"light", "torch_light", false, func(_ *MMGame, d spells.SpellDefinition) []string {
			return []string{uitext.Text("spell.light_radius_tiles", d.LightRadiusTiles)}
		}},
		{"radar", "wizard_eye", false, func(_ *MMGame, d spells.SpellDefinition) []string {
			return []string{uitext.Text("spell.radar_radius_tiles", d.RadarRadiusTiles)}
		}},
		{"water_walk", "walk_on_water", false, func(_ *MMGame, _ spells.SpellDefinition) []string {
			return []string{uitext.Text("spell.allows_the_party_to_walk_on_water")}
		}},
		{"water_breathing", "water_breathing", false, func(_ *MMGame, _ spells.SpellDefinition) []string {
			return []string{uitext.Text("spell.deep_water_leads_to_the_depths")}
		}},
		{"terrain_passage", "fly", false, func(_ *MMGame, _ spells.SpellDefinition) []string {
			return []string{
				uitext.Text("spell.the_party_crosses_terrain_and_walls_but"),
				uitext.Text("spell.no_fighting_inside_solid_terrain"),
				uitext.Text("spell.ends_without_open_sky"),
				uitext.Text("spell.only_under_an_open_sky_never_in"),
			}
		}},
	}
	for _, tc := range cases {
		// The changed-definition axis matters for the two formerly overlapping
		// fields. Other magnitudes already use only live state; utility radii
		// intentionally adopt current YAML on restore rather than saved values.
		rebalances := []bool{false}
		if tc.perStat || tc.key == "fire_shield" {
			rebalances = append(rebalances, true)
		}
		for _, rebalance := range rebalances {
			for _, restored := range []bool{false, true} {
				for _, tb := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/rebalanced=%v/restored=%v/tb=%v", tc.name, rebalance, restored, tb), func(t *testing.T) {
						h := newDisplayedModalHarness(t, 1024, 768)
						g := h.g
						g.menuOpen, g.showPartyStats = false, true
						wm := world.GlobalWorldManager
						if tc.key == "fly" {
							t.Chdir("../..")
							if err := wm.LoadMapConfigs("assets/map_configs.yaml"); err != nil {
								t.Fatal(err)
							}
						}
						original := config.GlobalSpells.Spells[tc.key]
						authored := *original
						if tc.perStat {
							authored.StatBonus, authored.StatBonusGrandmaster = 0, 0
							authored.StatBonuses = map[string]int{"might": 9, "speed": -3}
						}
						config.GlobalSpells.Spells[tc.key] = &authored
						t.Cleanup(func() { config.GlobalSpells.Spells[tc.key] = original })
						equipSpellAndPrepareCaster(t, g.combat, tc.key, 500, 40)
						if !g.combat.CastEquippedSpell() {
							t.Fatal("cast failed")
						}
						g.turnBasedMode = tb
						def, err := spells.GetSpellDefinitionByID(spells.SpellID(tc.key))
						if err != nil {
							t.Fatal(err)
						}
						want := tc.want(g, def)
						save := auditSaveJSON(t, g.buildSave(wm))
						if rebalance {
							if tc.perStat {
								authored.StatBonuses = map[string]int{"might": 19, "speed": 7}
							} else {
								authored.ResistBuffSchoolPct += 17
							}
							config.GlobalSpells.Spells[tc.key] = &authored
						}
						if restored {
							g.statBuffs, g.combatBuffs = nil, nil
							g.utilitySpellStatuses = nil
							g.restoreSavedEffects(&save)
							g.restoreSavedEffectPresentation(wm, &save)
						}
						rail, ok := g.spellStatusRail()
						if !ok || len(rail.statuses) != 1 || rail.statuses[0].SpellID != def.ID {
							t.Fatalf("rail = %+v, want only %s", rail.statuses, tc.key)
						}
						icon := rail.icons[0]
						prev := rawCursorPosition
						rawCursorPosition = func() (int, int) { return icon.x + icon.w/2, icon.y + icon.h/2 }
						t.Cleanup(func() { rawCursorPosition = prev })
						h.ui.Draw(h.screen)
						card := h.ui.tooltipRows.Lines()
						start, end := slices.Index(card, "EFFECTS"), slices.Index(card, "DURATION")
						if start < 0 || end <= start {
							t.Fatalf("missing effect section: %q", card)
						}
						got := slices.DeleteFunc(slices.Clone(card[start+1:end]), func(line string) bool { return line == "" })
						if !slices.Equal(got, want) {
							t.Fatalf("hover effects = %q, want each live effect once: %q", got, want)
						}
					})
				}
			}
		}
	}
}
