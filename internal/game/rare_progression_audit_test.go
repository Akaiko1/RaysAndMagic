package game

import (
	"fmt"
	"reflect"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/world"
)

func TestRareProgressionEveryClassThroughXPAndSave(t *testing.T) {
	for _, class := range character.PlayableClasses {
		for _, level := range []int{2, 3, 4, 6, 9, 12, 15, 18, 21} {
			for _, roster := range []string{"active", "reserve", "captive"} {
				t.Run(fmt.Sprintf("%s/L%d/%s", class.Key(), level, roster), func(t *testing.T) {
					g, c := rareClassGame(t, class, false)
					if _, err := config.LoadLevelUpConfig("../../assets/level_up.yaml"); err != nil {
						t.Fatal(err)
					}
					g.party.Reserve, g.party.Captive = nil, nil
					if roster != "active" {
						g.party.Members = g.party.Members[1:]
						if roster == "reserve" {
							g.party.Reserve = []*character.MMCharacter{c}
						} else {
							g.party.Captive = []*character.MMCharacter{c}
						}
					}
					c.Level, c.Experience = level-1, xpStepCost(level-1)
					g.combat.checkLevelUp(c, false)
					if c.Level != level {
						t.Fatalf("XP reached level %d instead of %d", c.Level, level)
					}
					wm := world.NewWorldManager(g.config)
					wm.LoadedMaps["forest"] = g.world
					setTestWorldManager(t, wm)
					saved := auditSaveJSON(t, g.buildSave(wm))
					if err := g.applySave(wm, &saved); err != nil {
						t.Fatal(err)
					}
					if roster == "active" && !reflect.DeepEqual(saved.PendingLevelUpChoices, g.buildSave(wm).PendingLevelUpChoices) {
						t.Fatal("load rerolled or changed pending options")
					}
					if roster == "captive" {
						g.party.FreeCaptives()
					}
					if roster != "active" {
						// Loading also restores missing named tavern recruits; locate
						// this hero instead of assuming the reserve starts at zero.
						idx := -1
						for i, member := range g.party.Reserve {
							if member.Name == c.Name && member.Class == class {
								idx = i
								break
							}
						}
						if idx < 0 || !g.swapRosterMember(0, idx) {
							t.Fatal("cannot activate hero")
						}
					}
					c = g.party.Members[0]
					if level%LevelUpChoiceInterval != 0 {
						if len(g.levelUpChoiceQueue) != 0 || len(c.OwedLevelChoices) != 0 {
							t.Fatal("choice granted at a non-choice level")
						}
						return
					}
					g.openLevelUpChoiceForChar(0)
					req := g.currentLevelUpChoice()
					if req == nil || req.level != level || len(req.options) < MinLevelUpOptions {
						t.Fatal("earned popup is missing or incomplete")
					}
					options := append([]levelUpChoiceOption(nil), req.options...)
					before := buildCharacterSave(c)
					seen := map[string]bool{}
					for i, opt := range options {
						c = restoreCharacterSave(before)
						g.party.Members[0] = c
						id := ""
						switch opt.choice.Type {
						case "weapon_mastery", "armor_mastery":
							id = fmt.Sprintf("skill:%d", opt.skillType)
							if !opt.skillType.UsesMastery() || !c.HasSkill(opt.skillType) || !opt.hasMastery {
								t.Fatalf("invalid upgrade: %+v", opt)
							}
						case "magic_mastery":
							id = "school:" + string(opt.school)
							if c.MagicSchools[opt.school] == nil || !opt.hasMastery {
								t.Fatalf("invalid school: %+v", opt)
							}
						case "spell":
							id = "spell:" + string(opt.spellID)
							if c.KnowsSpell(opt.spellID) {
								t.Fatal("known spell still offered")
							}
						default:
							t.Fatalf("unknown choice type: %s", opt.choice.Type)
						}
						if seen[id] {
							t.Fatalf("duplicate option %s", id)
						}
						seen[id] = true
						oldTier := 0
						if opt.choice.Type == "magic_mastery" {
							oldTier = int(c.MagicSchools[opt.school].Mastery)
						} else if opt.choice.Type != "spell" {
							oldTier = c.SkillTier(opt.skillType)
						}
						g.levelUpChoiceQueue = []levelUpChoiceRequest{{charIndex: 0, level: level, options: append([]levelUpChoiceOption(nil), options...), maxSelections: 1, padToMinimum: true}}
						g.levelUpChoiceOpen, g.levelUpChoiceIdx = true, 0
						g.consumeLevelUpChoice(i)
						if len(g.levelUpChoiceQueue) != 0 || g.levelUpChoiceOpen {
							t.Fatal("choice was not consumed exactly once")
						}
						switch opt.choice.Type {
						case "weapon_mastery", "armor_mastery":
							if c.SkillTier(opt.skillType) != oldTier+1 {
								t.Fatal("skill selection failed")
							}
						case "magic_mastery":
							if int(c.MagicSchools[opt.school].Mastery) != oldTier+1 {
								t.Fatal("school selection failed")
							}
						case "spell":
							if !c.KnowsSpell(opt.spellID) {
								t.Fatal("spell selection failed")
							}
						}
						restored := restoreCharacterSave(buildCharacterSave(c))
						if !reflect.DeepEqual(c.Skills, restored.Skills) || !reflect.DeepEqual(c.MagicSchools, restored.MagicSchools) {
							t.Fatal("selected progression lost after save/load")
						}
					}
				})
			}
		}
	}
}
