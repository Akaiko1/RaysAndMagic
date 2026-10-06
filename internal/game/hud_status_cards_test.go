package game

import (
	"fmt"
	"image/color"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

// Each buff family hovered on the real status rail states the magnitude it was
// cast with and both clocks, and nothing that belongs to another buff.
func TestBuffStatusCardStatesLiveEffectThroughTheHover(t *testing.T) {
	rows := []struct {
		key, marker string
		live        func(g *MMGame, def spells.SpellDefinition) string
	}{
		{"bless", "to all stats", func(g *MMGame, _ spells.SpellDefinition) string {
			b, _ := g.statBuffByID("bless")
			return fmt.Sprintf("+%d to all stats", b.Bonuses.Might)
		}},
		{"heroism", "deal +", func(g *MMGame, _ spells.SpellDefinition) string {
			b, _ := g.combatBuffByID("heroism")
			return fmt.Sprintf("deal +%d damage", b.OutBonus)
		}},
		{"stone_skin", "per hit", func(g *MMGame, _ spells.SpellDefinition) string {
			b, _ := g.combatBuffByID("stone_skin")
			return fmt.Sprintf("-%d damage per hit", b.InReduce)
		}},
		{"day_of_the_gods", "less damage", func(g *MMGame, _ spells.SpellDefinition) string {
			b, _ := g.combatBuffByID("day_of_the_gods")
			return fmt.Sprintf("%d%% less damage", b.ResistPct)
		}},
		{"torch_light", "Lights the dark", func(_ *MMGame, def spells.SpellDefinition) string {
			return fmt.Sprintf("within %.0f tiles", def.LightRadiusTiles)
		}},
	}
	for _, tb := range []bool{false, true} {
		for _, row := range rows {
			t.Run(fmt.Sprintf("%s/tb=%v", row.key, tb), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g := h.g
				g.menuOpen, g.showPartyStats = false, true
				def, err := spells.GetSpellDefinitionByID(spells.SpellID(row.key))
				if err != nil {
					t.Fatal(err)
				}
				equipSpellAndPrepareCaster(t, g.combat, row.key, 500, 40)
				if !g.combat.CastEquippedSpell() {
					t.Fatalf("%s cast failed", row.key)
				}
				g.turnBasedMode = tb
				rail, ok := g.spellStatusRail()
				if !ok || len(rail.statuses) != 1 || rail.statuses[0].SpellID != def.ID {
					t.Fatalf("rail = %+v, want only %s", rail.statuses, row.key)
				}
				icon, status := rail.icons[0], rail.statuses[0]
				prev := rawCursorPosition
				rawCursorPosition = func() (int, int) { return icon.x + icon.w/2, icon.y + icon.h/2 }
				t.Cleanup(func() { rawCursorPosition = prev })
				h.ui.Draw(h.screen)

				card := h.ui.tooltipLines
				if len(card) < 2 || card[0] != def.Name || card[1] != spellSchoolsLabel(def)+" Magic" {
					t.Fatalf("card header = %q", card)
				}
				if h.ui.tooltipTitleColor != schoolPlateColor(def.School) {
					t.Fatalf("plate = %v, want the %s school plate", h.ui.tooltipTitleColor, def.School)
				}
				text := strings.Join(card, "\n")
				if live := row.live(g, def); !strings.Contains(text, live) {
					t.Fatalf("card lacks the live effect %q:\n%s", live, text)
				}
				tps, round := g.config.GetTPS(), g.combatRoundFrames()
				remaining := fmt.Sprintf("Remaining: %ds (TB: %s)", (status.Duration+tps-1)/tps,
					pluralizeCount((status.Duration+round-1)/round, "round", "rounds"))
				if !slices.Contains(card, remaining) || !slices.Contains(card, uitext.Text("buff.double_click_dispel")) {
					t.Fatalf("card lacks %q or the dispel hint:\n%s", remaining, text)
				}
				for _, other := range rows {
					if other.key != row.key && strings.Contains(text, other.marker) {
						t.Fatalf("%s card shows %s's effect %q:\n%s", row.key, other.key, other.marker, text)
					}
				}
			})
		}
	}
}

// The camp hover predicts exactly what clicking the camp will do: the ready
// line when TryCamp succeeds, TryCamp's own refusal otherwise. Hovering spends
// nothing.
func TestCampStatusCardMatchesTryCamp(t *testing.T) {
	for _, tb := range []bool{false, true} {
		for _, state := range []string{"ready", "no_food", "enemy_near", "engaged_far"} {
			t.Run(fmt.Sprintf("%s/tb=%v", state, tb), func(t *testing.T) {
				h := newDisplayedModalHarness(t, 1024, 768)
				g := h.g
				g.menuOpen, g.turnBasedMode = false, tb
				g.party.Food = CampFoodCost + 2
				g.world.Monsters = nil
				g.party.Members[g.selectedChar].QuickSlots = [character.QuickSlotCount]*items.Item{}
				tile := g.config.GetTileSize()
				switch state {
				case "no_food":
					g.party.Food = 0
				case "enemy_near":
					g.world.Monsters = []*monster.Monster3D{{ID: "camp_near", Name: "Goblin", X: g.camera.X + tile, Y: g.camera.Y, HitPoints: 10, MaxHitPoints: 10}}
				case "engaged_far":
					g.world.Monsters = []*monster.Monster3D{{ID: "camp_engaged", Name: "Goblin", X: g.camera.X + 9*tile, Y: g.camera.Y, HitPoints: 10, MaxHitPoints: 10, IsEngagingPlayer: true}}
				}
				food := g.party.Food
				l, visible := inGameActionBarLayout(g)
				if !visible {
					t.Fatal("camp button hidden")
				}
				fp := installFakePointer(t)
				fp.moveTo(l.camp.x+l.camp.w/2, l.camp.y+l.camp.h/2)
				h.ui.Draw(h.screen)

				card, colors := h.ui.tooltipLines, h.ui.tooltipColors
				if len(card) == 0 || card[0] != uitext.Text("ui.camp") || len(colors) != len(card) {
					t.Fatalf("camp card = %q", card)
				}
				if h.ui.tooltipTitleColor != color.Color(woodPlateColor) {
					t.Fatal("camp card lost its wood plate")
				}
				if !slices.Contains(card, uitext.Text("ui.camp_cost", CampFoodCost, food)) {
					t.Fatalf("camp card lacks the cost with %d food left:\n%s", food, strings.Join(card, "\n"))
				}
				if g.party.Food != food {
					t.Fatal("hovering the camp spent food")
				}
				status, ink := card[len(card)-1], colors[len(colors)-1]
				msg, ok := g.TryCamp()
				if ok != (state == "ready") {
					t.Fatalf("TryCamp ok=%v (%s) in state %s", ok, msg, state)
				}
				if ok && (status != uitext.Text("ui.camp_click") || ink != color.Color(hudCardReadyColor)) {
					t.Fatalf("ready camp card ends %q", status)
				}
				if !ok && (status != msg || ink != color.Color(hudCardBlockedColor)) {
					t.Fatalf("blocked camp card ends %q, TryCamp refused with %q", status, msg)
				}
			})
		}
	}
}

// Draughts and techniques have no spell definition: their cards name the
// source and state the live values from the running buff.
func TestBuffStatusCardNamesNonSpellSources(t *testing.T) {
	for _, tb := range []bool{false, true} {
		t.Run(fmt.Sprintf("draught/tb=%v", tb), func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			g := cs.game
			def, ok := config.GetItemDefinition("flame_ward_draught")
			if !ok || def.ResistBuffSchoolPct <= 0 {
				t.Fatal("fixture: flame_ward_draught must be a timed resist ward")
			}
			g.party.Inventory = []items.Item{items.CreateItemFromYAML("flame_ward_draught")}
			if !g.UseConsumableFromInventory(0, 0) {
				t.Fatal("draught not consumed")
			}
			g.turnBasedMode = tb
			g.tickCombatBuffs() // the frame tick puts the draught on the rail
			status := g.utilitySpellStatuses["flame_ward_draught"]
			if status == nil {
				t.Fatal("drunk draught never reached the status rail")
			}
			card, _ := g.buffStatusCard(status)
			text := strings.Join(card, "\n")
			if card[0] != def.Name || card[1] != uitext.Text("buff.source_draught") {
				t.Fatalf("draught card header = %q", card[:2])
			}
			// The same lines as the draught's own item card.
			for _, want := range def.ItemMechanicLines() {
				if !slices.Contains(card, want) {
					t.Fatalf("draught card lacks its item line %q:\n%s", want, text)
				}
			}
		})
		t.Run(fmt.Sprintf("technique/tb=%v", tb), func(t *testing.T) {
			g, c := rareClassGame(t, character.ClassWayfarer, tb)
			if !g.useTechnique(0, "quickening", false, false) {
				t.Fatalf("quickening refused: %s", g.techniqueRefusal(0, "quickening"))
			}
			d := config.Technique("quickening")
			tier := c.SkillTier(character.SkillTranslocation)
			status := g.utilitySpellStatuses["quickening"]
			if status == nil {
				t.Fatal("quickening never reached the status rail")
			}
			card, _ := g.buffStatusCard(status)
			text := strings.Join(card, "\n")
			for _, want := range []string{
				uitext.Text("buff.rt_recovery", strconv.Itoa(config.TierValue(d.Power, tier))),
				uitext.Text("buff.tb_pool", strconv.Itoa(config.TierValue(d.TBPower, tier))),
			} {
				if !slices.Contains(card, want) {
					t.Fatalf("quickening card lacks %q:\n%s", want, text)
				}
			}
			if card[0] != d.Name || card[1] != uitext.Text("buff.source_technique") {
				t.Fatalf("technique card header = %q", card[:2])
			}
		})
	}
}

// A buff cast at Novice and at Grandmaster shows the ladder value and duration
// of the caster's own mastery, as authored in spells.yaml.
func TestBuffStatusCardFollowsTheCastersMastery(t *testing.T) {
	ladders := map[string]func(def spells.SpellDefinition, tier int) string{
		"bless": func(def spells.SpellDefinition, tier int) string {
			return uitext.Text("spell.to_all_stats_whole_party", scaledMasteryValueAt(def.StatBonus, def.StatBonusGrandmaster, tier))
		},
		"heroism": func(def spells.SpellDefinition, tier int) string {
			return uitext.Text("spell.party_deal_damage", spells.OutgoingDamageTarget(def.OutgoingDamageType),
				scaledMasteryValueAt(def.OutgoingDamageBonus, def.OutgoingDamageBonusGrandmaster, tier))
		},
		"stone_skin": func(def spells.SpellDefinition, tier int) string {
			return uitext.Text("spell.party_takes_damage_per_hit",
				scaledMasteryValueAt(def.IncomingDamageReduction, def.IncomingDamageReductionGrandmaster, tier))
		},
		"day_of_the_gods": func(def spells.SpellDefinition, tier int) string {
			return uitext.Text("spell.party_takes_less_damage", scaledMasteryValueAt(def.ResistBuffPct, def.ResistBuffPctGrandmaster, tier))
		},
	}
	newTestCombatSystemWithConfig(t) // loads spells.yaml for the ladder fixtures
	for _, key := range slices.Sorted(maps.Keys(ladders)) {
		def, err := spells.GetSpellDefinitionByID(spells.SpellID(key))
		if err != nil {
			t.Fatal(err)
		}
		novice, gm := ladders[key](def, int(character.MasteryNovice)), ladders[key](def, int(character.MasteryGrandMaster))
		if novice == gm {
			t.Fatalf("fixture: %s has no mastery ladder", key)
		}
		for _, tier := range []character.SkillMastery{character.MasteryNovice, character.MasteryGrandMaster} {
			t.Run(fmt.Sprintf("%s/tier=%d", key, tier), func(t *testing.T) {
				cs := newTestCombatSystemWithConfig(t)
				g := cs.game
				equipSpellAndPrepareCaster(t, cs, key, 500, 40)
				g.party.Members[0].SpellMasterySkill(def).Mastery = tier
				if !cs.CastEquippedSpell() {
					t.Fatalf("%s cast failed", key)
				}
				card, _ := g.buffStatusCard(g.utilitySpellStatuses[def.ID])
				seconds := character.SpellDurationAtTier(def, int(tier)).Seconds
				for _, want := range []string{ladders[key](def, int(tier)), "Remaining: " + strconv.Itoa(seconds) + "s"} {
					if !slices.ContainsFunc(card, func(line string) bool { return strings.HasPrefix(line, want) }) {
						t.Fatalf("tier %d card lacks %q:\n%s", tier, want, strings.Join(card, "\n"))
					}
				}
			})
		}
	}
}
