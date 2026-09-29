package game

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/items"
)

// Case table: all 48 skills, all four tiers, fixed/no-effect skills, all nine
// schools, and loaded/unloaded data. The canonical reference feeds character,
// level-up, trainer and editor views. Existing reference-layout tests verify
// every line survives wrapping; these assertions pin the audited gameplay facts.
// Persistence is not involved: no description is stored in a save.
func TestSkillTooltipAuditAllMechanics(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	oldTechniques := config.GlobalTechniques
	t.Cleanup(func() { config.GlobalTechniques = oldTechniques })
	if err := config.LoadTechniques("../../assets/techniques.yaml"); err != nil {
		t.Fatal(err)
	}
	cases := map[character.SkillType][]string{
		character.SkillBodybuilding:        {"0/8/16/24", "10% of base max HP"},
		character.SkillMeditation:          {"0/3/6/9", "Spells, traps and techniques", "25% less SP"},
		character.SkillMerchant:            {"Gold-shop buy prices -0/5/10/15%", "sell prices +0/5/10/15%", "Best active Merchant"},
		character.SkillRepair:              {"No effect yet"},
		character.SkillIdentifyItem:        {"No effect yet"},
		character.SkillDisarmTrap:          {"40/60/80/100%", "0/1/2/3", "best active user", "true damage and DoTs bypass"},
		character.SkillLearning:            {"0/10/20/30%", "+5%", "all rosters", "does not stack"},
		character.SkillArmsMaster:          {"0/2/4/6", "+5% weapon crit"},
		character.SkillTrapper:             {"0/5/10/15", "floor((Intellect + Accuracy)/3)", "0/2/4/6s RT", "0/0/1/2 turns TB"},
		character.SkillSleightOfHand:       {"10/20/30/40%", "One successful theft per surviving enemy"},
		character.SkillDualWielding:        {"0/10/20/30%", "separate hand recovery", "at least 2 actions per round"},
		character.SkillIronBody:            {"10/20/30/40", "+10% Perfect Dodge"},
		character.SkillSpiritualTraining:   {"10/20/30/40%", "even on a miss", "no SP or extra action", "offensive spell"},
		character.SkillElementalMastery:    {"10/20/35/50%", "Light", "Dark"},
		character.SkillAnimalBonding:       {"Weapon attacks/spells:", "5/8/12/15%", "100/200/300/500%", "40/60/80/100%", "up to 2 living bears"},
		character.SkillSacrifice:           {"10/20/30/50%", "Strongest living active protector", "after defenses", "no second mitigation or DoTs"},
		character.SkillImpenetrableDefense: {"3/5/7/10", "after defenses", "Does not reduce true damage or DoTs"},
		character.SkillLockpicking:         {"20/35/50/60%", "3 failed non-key attempts", "key still works"},
		character.SkillNaturalHealer:       {"20/40/60/100%", "Spell HP healing"},
		character.SkillCelestialProvidence: {"dawn/dusk", "Living active Celestial", "Master-tier", "Does not stack"},
		character.SkillOrcishFury:          {"Normal weapon damage +3/5/7/10"},
		character.SkillHalflingGuile:       {"Halves this hero's weight", "random party target"},
		character.SkillDarkElfBinding:      {" 3% chance", "including fields", "turn the target into an ally for this map", "undead, formless, bosses"},
		character.SkillSpellAbsorption:     {"15/30/45/60%", "both HP and SP", "damage before defenses"},
		character.SkillStrongMagic:         {"damage +25/50/75/100%", "HP cost: 25/50/75/100% of paid SP", "leave 1 HP"},
		character.SkillBallistics:          {"15/25/35/50%", "0/0/1/2 tiles", "2/4/6/8%"},
		character.SkillFieldMedicine:       {"15/25/40/60%", "10/20/30/40%", "Revival unchanged"},
		character.SkillDesignateTarget:     {"6/9/12/15s", "5/8/12/15 percentage points", "One mark per user", "while the marker can act"},
		character.SkillOverwatch:           {"20/30/40/50%", "half that chance", "free bow/blaster shot", "1.0s in RT", "until next round in TB"},
		character.SkillAlchemy:             {"common: 2/3/4/6", "protective: 1/2/3/4", "revival: 1/2/3/4"},
		character.SkillPharmacology:        {"20/35/50/75%", "10/15/20/25%", "Intellect/3", "adds to Field Medicine", "Revival unchanged"},
		character.SkillBombThrowing:        {"Harm Flask: 72/90/108/126 + INT/3 physical damage", "Fire Flask: 36/48/60/72 + INT/3 fire damage", "Venom Flask: 24/36/48/60 + INT/3 body damage", "poison 6/9/12/15s", "burning 3/5/7/9s"},
		character.SkillTranslocation:       {"Fold Step: 3/4/6/8 tiles", "8/7/6/5 SP", "12/15/18/24s", "Return Step: 6/5/4/3 SP", "dodge +10/15/20/25%; 3/6/6/9s", "Purify: 12/11/10/9 SP", "RT recovery -20/25/30/35%", "TB +1/1/2/2 actions next turn"},
		character.SkillFlowingStaff:        {"1/2/3/4 staff charges", "25/50/75/100%", "guarantees a critical staff attack", "remain until spent"},
		character.SkillPathfinding:         {"5/10/15/20%", "Party RT movement speed", "Best capable active guide", "attack and cast while running"},
	}
	for _, s := range []character.SkillType{character.SkillSword, character.SkillDagger, character.SkillAxe, character.SkillSpear, character.SkillBow, character.SkillMace, character.SkillStaff, character.SkillMartialArts, character.SkillBlaster} {
		cases[s] = []string{"0/3/6/9 true damage", "resistance applies", "+7% crit", "ignore Perfect Dodge"}
	}
	for _, s := range []character.SkillType{character.SkillLeather, character.SkillChain, character.SkillPlate} {
		cases[s] = []string{"0/2/4/6 AC per equipped piece", "+5% Perfect Dodge", "once per armor type"}
	}
	cases[character.SkillShield] = []string{"Shield AC +0/2/4/6", "+5% Perfect Dodge"}
	if len(cases) != len(character.AllSkills) {
		t.Fatalf("audited %d skills, catalog has %d", len(cases), len(character.AllSkills))
	}
	grandMasterSections := map[character.SkillType]bool{
		character.SkillBodybuilding: true, character.SkillMeditation: true,
		character.SkillLearning: true, character.SkillArmsMaster: true,
		character.SkillIronBody: true, character.SkillPathfinding: true,
	}
	for _, skill := range character.AllSkills {
		grandMasterSections[skill] = grandMasterSections[skill] || skill.IsWeaponSkill() || skill.IsArmorSkill()
	}
	for _, skill := range character.AllSkills {
		t.Run(skill.String(), func(t *testing.T) {
			text := masteryTooltipTextForSkill(skill)
			assertSkillGrandMasterSection(t, text, grandMasterSections[skill])
			normalized := strings.Join(strings.Fields(text), " ")
			for _, want := range cases[skill] {
				if !strings.Contains(normalized, want) {
					t.Errorf("missing %q in %s", want, text)
				}
			}
			for _, filler := range []string{"Novice / Expert / Master / Grandmaster", "weapon hand stays free", "V:", "Shift+V", "F or SmartAttack", "techniques in the Pilgrim's book"} {
				if strings.Contains(text, filler) {
					t.Errorf("skill reference contains redundant instruction %q", filler)
				}
			}
			ui := &UISystem{game: cs.game}
			ui.queueTooltip(strings.Split(text, "\n"), 0, 0)
			assertSharedTooltipLayout(t, ui)
			layout := layoutTooltip(ui.tooltipLines, false, tooltipColumnWidth(1920, 1), 1080)
			if layout.h > 1080-2*tooltipScreenMargin {
				t.Fatal("resized tooltip escapes the screen")
			}
		})
	}
	for _, school := range character.AllMagicSchools {
		text := magicMasteryTooltipText(school)
		assertSkillGrandMasterSection(t, text, true)
		for _, want := range []string{"0/20/40/60%", "0/5/10/15", "Standard spell damage/healing"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s missing %q: %s", school, want, text)
			}
		}
	}
	flasks := masteryTooltipTextForSkill(character.SkillBombThrowing)
	if strings.Count(flasks, "range 6 tiles; radius 2 tiles") != 1 || !strings.Contains(flasks, "All flasks:") {
		t.Fatal("shared flask range/radius must appear once")
	}
}

func assertSkillGrandMasterSection(t *testing.T, text string, want bool) {
	t.Helper()
	count := strings.Count(text, "\n\nGRAND MASTER\n")
	if (want && count != 1) || (!want && count != 0) {
		t.Fatalf("Grand Master section count %d, expected section=%v: %s", count, want, text)
	}
	for _, word := range strings.Fields(text) {
		if strings.Trim(word, ":;,.()") == "GM" {
			t.Fatalf("unexplained mastery abbreviation: %s", text)
		}
	}
	if strings.Contains(text, "Grand Master:") {
		t.Fatal("Grand Master label repeated below its heading")
	}
	if want {
		if !tooltipSectionHeading("GRAND MASTER") {
			t.Fatal("Grand Master bonus lacks the shared heading style")
		}
		_, bonus, _ := strings.Cut(text, "\n\nGRAND MASTER\n")
		if strings.TrimSpace(bonus) == "" {
			t.Fatal("empty Grand Master section")
		}
	}
}

func TestSkillTooltipAuditCatalogAvailability(t *testing.T) {
	newTestCombatSystemWithConfig(t)
	oldItems, oldTechniques := config.GlobalItems, config.GlobalTechniques
	t.Cleanup(func() { config.GlobalItems, config.GlobalTechniques = oldItems, oldTechniques })
	config.GlobalItems, config.GlobalTechniques = nil, nil
	for _, skill := range character.AllSkills {
		text := skill.Description()
		if text == "" || strings.Contains(text, "%!") {
			t.Fatalf("invalid description without catalogs for %s: %s", skill, text)
		}
	}
}

func TestSkillTooltipAuditTrainerHover(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.dialogActive, g.skillTrainerPopup = true, true
	g.selectedCharIdx = 0
	g.dialogNPC = &character.NPC{Name: "Trainer", Type: character.NPCTypeSkillTrainer, Training: map[string]int{"expert": 1, "master": 2, "grandmaster": 3}}
	ui := &UISystem{game: g}
	screen := ebiten.NewImage(1024, 768)
	t.Cleanup(screen.Deallocate)
	check := func(want string) {
		t.Helper()
		ui.tooltipLines = nil
		// Put the first offered row around the current input snapshot. This
		// exercises the real hover branch without moving the user's cursor.
		mx, my := ebiten.CursorPosition()
		ui.drawSkillTrainerPopup(screen, mx-18, my-30-skillTrainerListTop-4, 420, 500)
		if strings.Join(ui.tooltipLines, "\n") != want {
			t.Fatalf("trainer did not queue the canonical hovered reference: %v", ui.tooltipLines)
		}
	}
	for _, skill := range character.AllSkills {
		if !skill.UsesMastery() {
			continue
		}
		t.Run(skill.String(), func(t *testing.T) {
			g.party.Members[0] = &character.MMCharacter{Name: "Learner", Skills: map[character.SkillType]*character.Skill{skill: {}}}
			check(masteryTooltipTextForSkill(skill))
		})
	}
	for _, school := range character.AllMagicSchools {
		t.Run(school.String(), func(t *testing.T) {
			g.party.Members[0] = &character.MMCharacter{Name: "Learner", MagicSchools: map[character.MagicSchoolID]*character.MagicSkill{school: {}}}
			check(magicMasteryTooltipText(school))
		})
	}
}

// Retuning the live catalog must change both the gameplay payload and the skill
// reference. This catches descriptions that copied today's YAML numbers once.
func TestSkillTooltipAuditRetunedFlaskWiring(t *testing.T) {
	for _, key := range []string{"harm_flask", "venom_flask", "fire_flask"} {
		for tier := 0; tier < 4; tier++ {
			t.Run(fmt.Sprintf("%s/%d", key, tier), func(t *testing.T) {
				g, c := rareClassGame(t, character.ClassAlchemist, false)
				d, _ := config.GetItemDefinition(key)
				old := *d.Flask
				t.Cleanup(func() { *d.Flask = old })
				d.Flask.Damage = [4]int{19, 27, 38, 53}
				d.Flask.PoisonSeconds = [4]int{2, 4, 7, 11}
				d.Flask.BurnSeconds = [4]int{2, 5, 8, 11}
				d.Flask.RangeTiles, d.Flask.RadiusTiles = 9, 4
				d.Flask.CooldownSeconds = 2.5
				c.Skills[character.SkillBombThrowing].Mastery = character.SkillMastery(tier)
				c.Intellect = 18
				g.party.AddItem(items.CreateItemFromYAML(key))
				if !g.throwFlask(0, key, false) {
					t.Fatal("retuned flask refused")
				}
				p := g.magicProjectiles[0]
				if p.Damage != d.Flask.Damage[tier]+c.GetEffectiveIntellect()/3 || p.FlaskPoisonFrames != d.Flask.PoisonSeconds[tier]*g.config.GetTPS() || p.FlaskBurnFrames != d.Flask.BurnSeconds[tier]*g.config.GetTPS() {
					t.Fatal("projectile did not use the live mastery table")
				}
				text := masteryTooltipTextForSkill(character.SkillBombThrowing)
				for _, want := range []string{d.Name + ": 19/27/38/53 + INT/3", "poison 2/4/7/11s", d.Name + ": range 9 tiles; radius 4 tiles"} {
					if !strings.Contains(text, want) {
						t.Errorf("retuned skill missing %q: %s", want, text)
					}
				}
				if strings.Contains(text, "All flasks:") || strings.Count(text, "range 6 tiles; radius 2 tiles") != 1 {
					t.Fatal("different flask ranges must be grouped without repeating or mislabeling shared values")
				}
			})
		}
	}
}

func TestSkillTooltipAuditRetunedTechniqueWiring(t *testing.T) {
	for _, key := range []string{"fold_step", "return_step", "phase_veil", "purify", "quickening"} {
		for tier := 0; tier < 4; tier++ {
			t.Run(fmt.Sprintf("%s/%d", key, tier), func(t *testing.T) {
				g, c := rareClassGame(t, character.ClassWayfarer, false)
				d := config.Technique(key)
				old := *d
				t.Cleanup(func() { *d = old })
				d.SPCost = [4]int{17, 14, 11, 8}
				c.Skills[character.SkillTranslocation].Mastery = character.SkillMastery(tier)
				delete(c.Skills, character.SkillMeditation)
				text := masteryTooltipTextForSkill(character.SkillTranslocation)
				if g.techniqueSPCost(c, d) != d.SPCost[tier] || !strings.Contains(text, "17/14/11/8 SP") {
					t.Fatal("technique cost and skill reference diverged")
				}
				for _, tc := range []struct {
					values *[4]int
					want   string
					used   bool
				}{
					{&d.Range, "7/8/9/10 tiles", key == "fold_step"},
					{&d.Duration, "7/8/9/10s", key == "fold_step" || key == "phase_veil" || key == "quickening"},
					{&d.Power, "7/8/9/10", key == "phase_veil" || key == "quickening"},
					{&d.TBPower, "TB +7/8/9/10 actions", key == "quickening"},
				} {
					if !tc.used {
						continue
					}
					before := *tc.values
					*tc.values = [4]int{7, 8, 9, 10}
					if !strings.Contains(masteryTooltipTextForSkill(character.SkillTranslocation), tc.want) {
						t.Errorf("retuned %s lost %q", key, tc.want)
					}
					*tc.values = before
				}
			})
		}
	}
}
