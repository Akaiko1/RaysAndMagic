package game

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
	"ugataima/internal/spells"
)

// helper: a character with a fixed effective Speed (no stat bonus, no gear).
func charWithSpeed(speed int) *character.MMCharacter {
	c := &character.MMCharacter{Speed: speed, Equipment: map[items.EquipSlot]items.Item{}}
	return c
}

// newSpellCooldownTestSystem loads only the two configuration files this
// calculation needs. It deliberately avoids the mutable item catalog, so a
// work-in-progress item set cannot hide a spell-cooldown regression.
func newSpellCooldownTestSystem(t *testing.T) *CombatSystem {
	t.Helper()

	oldConfig, oldSpells := config.GlobalConfig, config.GlobalSpells
	t.Cleanup(func() {
		config.GlobalConfig = oldConfig
		config.GlobalSpells = oldSpells
	})

	cfg, err := config.LoadConfig("../../config.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if _, err := config.LoadSpellConfig("../../assets/spells.yaml"); err != nil {
		t.Fatalf("load spells: %v", err)
	}

	game := &MMGame{config: cfg}
	game.combat = NewCombatSystem(game)
	return game.combat
}

// sortedSpellDefs returns every authored spell definition in key order.
func sortedSpellDefs(t *testing.T) []spells.SpellDefinition {
	t.Helper()
	keys := make([]string, 0, len(config.GlobalSpells.Spells))
	for key := range config.GlobalSpells.Spells {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	defs := make([]spells.SpellDefinition, 0, len(keys))
	for _, key := range keys {
		def, err := spells.GetSpellDefinitionByID(spells.SpellID(key))
		if err != nil {
			t.Fatalf("load %s: %v", key, err)
		}
		defs = append(defs, def)
	}
	return defs
}

// TestSpellCooldownFrames_MatchesAuthoredSeconds checks that at the reference
// Speed every cooldown-bearing spell's RT cooldown equals its authored
// cooldown_seconds (xTPS), proving the safety clamp no longer crushes the long
// spells. Persistent buffs are tested separately because they intentionally
// have no personal RT cooldown.
func TestSpellCooldownFrames_MatchesAuthoredSeconds(t *testing.T) {
	cs := newSpellCooldownTestSystem(t)
	tps := cs.game.config.GetTPS()
	caster := charWithSpeed(ActionCooldownSpeedRefSpeed)

	checked := 0
	for _, def := range sortedSpellDefs(t) {
		if def.IsBuff() || def.CooldownSeconds <= 0 {
			continue
		}
		checked++
		want := int(math.Round(def.CooldownSeconds * float64(tps))) // factor == 1.0 at reference speed
		if got := cs.SpellCooldownFrames(caster, def.ID); got != want {
			t.Errorf("%s: cooldown = %d frames (%.2fs), want %d (%.2fs)",
				def.ID, got, float64(got)/float64(tps), want, def.CooldownSeconds)
		}
	}
	if checked == 0 {
		t.Fatal("no cooldown-bearing spells loaded")
	}
}

// Exercise the real book and quick-slot input paths, not just a classifier or
// commit helper. Ally-picker payment/cancellation is covered in
// TestAllySpellTargetingOwnsPointerAndPayment; reuse and save/load in
// TestPersonalSpellReuseAcrossModesAndSave. Categories themselves are not saved.
func TestSpellInputRecoveryByEffect(t *testing.T) {
	newCombat := newTestCombatFactory(t)
	previous := config.GlobalTechniques
	t.Cleanup(func() { config.GlobalTechniques = previous })
	if err := config.LoadTechniques("../../assets/techniques.yaml"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id         string
		noRecovery bool // category: buff; combat_buff and ordinary casts recover
	}{
		{"bless", true}, // Legacy timed buff.
		{"shield", true}, {"regeneration", true}, {"pain_reflection", true},
		{"haste", false}, {"preservation", false}, {"protection_from_magic", false}, {"celestial_armor", false},
		{"body_purify", false}, {"dispel_magic", false}, {"shared_life", false}, {"firebolt", false},
	} {
		for _, tb := range []bool{false, true} {
			for _, book := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/TB=%v/book=%v", tc.id, tb, book), func(t *testing.T) {
					g := newCombat(t).game
					installFakePointer(t)
					g.appScreen, g.turnBasedMode = AppScreenInGame, tb
					g.menuOpen, g.currentTab, g.selectedSpell = book, TabSpellbook, 0
					for _, c := range g.party.Members {
						isolateTrueDamageMember(c, 0)
						c.MagicSchools = map[character.MagicSchoolID]*character.MagicSkill{}
						c.HitPoints, c.MaxHitPoints, c.ActionsRemaining, c.RTCooldown = 50, 100, 3, 0
					}
					g.party.Members[1].AddCondition(character.ConditionPoisoned)
					equipSpellAndPrepareCaster(t, g.combat, tc.id, 1000, 30)
					c := g.party.Members[0]
					c.RecalculateMaxStatsKeepingCurrent(g.config)
					d, _ := spells.GetSpellDefinitionByID(spells.SpellID(tc.id))
					cost := c.MeditatedSPCost(d.SpellPointsCost)
					sp, actions := c.SpellPoints, c.ActionsRemaining
					pressKeys(NewInputHandler(g), ebiten.KeyF)
					if c.SpellPoints != sp-cost || g.menuOpen {
						t.Fatalf("cast did not commit once and close book: SP=%d, want %d, menu=%v", c.SpellPoints, sp-cost, g.menuOpen)
					}
					used := 0
					if tb {
						used = 1
					}
					if c.ActionsRemaining != actions-used || g.partyActionsUsed != used {
						t.Fatalf("wrong action cost: remaining=%d used=%d", c.ActionsRemaining, g.partyActionsUsed)
					}
					if (c.RTCooldown == 0) != tc.noRecovery {
						t.Fatalf("personal recovery=%d; want none=%v", c.RTCooldown, tc.noRecovery)
					}
					if !tb && !book && g.actionInputStagger != rtActionStagger {
						t.Fatalf("input stagger=%d, want %d", g.actionInputStagger, rtActionStagger)
					}
					for _, viewer := range []*character.MMCharacter{nil, c} {
						card := GetSpellTooltipRows(d.ID, viewer, g.combat, true).String()
						if strings.Contains(card, "No RT cooldown") != tc.noRecovery {
							t.Fatalf("tooltip recovery disagrees with buff behavior: %s", card)
						}
					}
				})
			}
		}
	}
}

func TestCombatActionGateUsesActiveModeEconomy(t *testing.T) {
	cs := newSpellCooldownTestSystem(t)
	game := cs.game
	caster := &character.MMCharacter{HitPoints: 1, ActionsRemaining: 1, Equipment: map[items.EquipSlot]items.Item{}}
	game.party = &character.Party{Members: []*character.MMCharacter{caster}}
	game.selectedChar = 0

	game.turnBasedMode = false
	caster.RTCooldown = 1
	if game.canSpendCombatAction(0) {
		t.Fatal("RT UI action gate ignored the character cooldown")
	}
	caster.RTCooldown = 0
	if !game.canSpendCombatAction(0) {
		t.Fatal("ready RT character was rejected by the UI action gate")
	}

	game.turnBasedMode = true
	game.currentTurn = 0
	if !game.canSpendCombatAction(0) {
		t.Fatal("ready TB character was rejected by the UI action gate")
	}
	game.currentTurn = 1
	if game.canSpendCombatAction(0) {
		t.Fatal("TB UI action gate allowed an action during the monster phase")
	}
}

func TestRTSpellBookCommitArmsPersonalCooldown(t *testing.T) {
	cs := newSpellCooldownTestSystem(t)
	game := cs.game
	caster := &character.MMCharacter{HitPoints: 1, ActionsRemaining: 2, Equipment: map[items.EquipSlot]items.Item{}}
	game.party = &character.Party{Members: []*character.MMCharacter{caster}}
	game.selectedChar = 0
	game.turnBasedMode = false

	game.consumeSelectedCharActionWithRTCooldown(37)

	if caster.RTCooldown != 37 {
		t.Fatalf("RT UI cast cooldown = %d, want 37", caster.RTCooldown)
	}
	if caster.ActionsRemaining != 2 {
		t.Fatalf("RT UI cast spent TB slots: remaining=%d, want 2", caster.ActionsRemaining)
	}
}

// TestSpellCooldownFrames_SpeedScales confirms Speed still influences spell
// cooldown: a faster caster casts the same spell sooner.
func TestSpellCooldownFrames_SpeedScales(t *testing.T) {
	cs := newSpellCooldownTestSystem(t)
	slow := cs.SpellCooldownFrames(charWithSpeed(5), "fireball")
	ref := cs.SpellCooldownFrames(charWithSpeed(ActionCooldownSpeedRefSpeed), "fireball")
	fast := cs.SpellCooldownFrames(charWithSpeed(50), "fireball")
	if !(slow > ref && ref > fast) {
		t.Errorf("expected slow(%d) > ref(%d) > fast(%d)", slow, ref, fast)
	}
}

// TestWeaponCooldown_TypeMultipliers checks the weapon-type ordering (dagger
// fast ... axe slow) and that the Bow of Hellfire legendary override makes it
// SLOWER than an ordinary bow despite both being bows.
func TestWeaponCooldown_TypeMultipliers(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	speed := 25

	frames := func(weaponKey string) int {
		c := charWithSpeed(speed)
		c.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML(weaponKey)
		return cs.WeaponCooldownFrames(c)
	}

	dagger := frames("magic_dagger")
	sword := frames("iron_sword")
	bow := frames("hunting_bow")
	axe := frames("bronze_labrys") // category-baseline axe (steel_axe/gorehorn carry per-weapon overrides)
	hellfire := frames("bow_of_hellfire")

	if !(dagger < sword && sword < bow && bow < axe) {
		t.Errorf("expected dagger(%d) < sword(%d) < bow(%d) < axe(%d)", dagger, sword, bow, axe)
	}
	if hellfire <= bow {
		t.Errorf("Bow of Hellfire (%d) should be SLOWER than a plain bow (%d)", hellfire, bow)
	}

	// "throwing" is not a real weapon type - it resolves to the dagger SKILL, so
	// the throwing knife must share the dagger cooldown.
	if throwingKnife := frames("throwing_knife"); throwingKnife != dagger {
		t.Errorf("throwing_knife cooldown (%d) should equal dagger (%d) - throwing maps to the dagger skill", throwingKnife, dagger)
	}
	// Blaster is a real skill but has no authored category multiplier, so it
	// keeps the neutral 1.0 cadence (same as the sword baseline).
	if blaster := frames("alien_blaster"); blaster != sword {
		t.Errorf("alien_blaster cooldown (%d) should be neutral 1.0 == sword (%d)", blaster, sword)
	}
}

// TestArchmageStaff_ReducesSpellCooldown verifies the legendary staff's
// spell_cooldown_multiplier (-20%) lowers the caster's spell cooldown.
func TestArchmageStaff_ReducesSpellCooldown(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	bare := charWithSpeed(25)
	staffed := charWithSpeed(25)
	staffed.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("archmage_staff")

	base := cs.SpellCooldownFrames(bare, "fireball")
	withStaff := cs.SpellCooldownFrames(staffed, "fireball")
	if withStaff >= base {
		t.Errorf("Archmage Staff should reduce spell cooldown: with=%d, base=%d", withStaff, base)
	}
}

// TestSpellClassification_OffensiveVsSupport guards the smart-attack autocast
// rule: offensive spells (incl. AoE-stun and zones that are flagged utility)
// count as combat; heals/buffs/pure-utility do not.
func TestSpellClassification_OffensiveVsSupport(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	offensive := []string{"firebolt", "fireball", "stun", "darkness", "hot_steam", "inferno", "charm", "disintegrate", "psychic_shock"}
	support := []string{"heal", "heal_other", "mass_heal", "bless", "heroism", "stone_skin", "resurrect", "torch_light", "wizard_eye", "awaken"}
	for _, id := range offensive {
		def, err := spells.GetSpellDefinitionByID(spells.SpellID(id))
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if !def.IsOffensive() {
			t.Errorf("%s should be offensive (autocast on Space)", id)
		}
	}
	for _, id := range support {
		def, err := spells.GetSpellDefinitionByID(spells.SpellID(id))
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if def.IsOffensive() {
			t.Errorf("%s should NOT be offensive (Space must skip it)", id)
		}
	}
}
