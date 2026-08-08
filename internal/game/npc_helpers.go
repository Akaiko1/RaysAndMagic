package game

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/spells"
)

type npcDialogVars struct {
	Name  string
	Spell string
	Cost  int
}

type trainerOption struct {
	Label     string
	Current   character.SkillMastery
	Next      character.SkillMastery
	Cost      int
	IsMagic   bool
	SkillType character.SkillType
	School    character.MagicSchoolID
}

func npcHasSpellTrading(npc *character.NPC) bool {
	return npc != nil && len(npc.SpellData) > 0
}

func npcHasMerchant(npc *character.NPC) bool {
	return npc != nil && (npc.SellAvailable || len(npc.MerchantStock) > 0)
}

func npcHasSkillTraining(npc *character.NPC) bool {
	return npc != nil && npc.Type == character.NPCTypeSkillTrainer
}

// probeUngatedNPC builds an authored NPC for a BOOT CHECK, with its gate cleared:
// the question these checks ask is what the NPC would be with its service open.
// The real dispatch is production code and may read party or world state, so the
// probe hands it a real NPC rather than a zero value.
func probeUngatedNPC(npcKey string) (*character.NPC, error) {
	npc, err := character.CreateNPCFromConfig(npcKey, 0, 0)
	if err != nil {
		return nil, err
	}
	if npc == nil {
		return nil, fmt.Errorf("NPC %q could not be built", npcKey)
	}
	npc.RequiresQuest = ""
	return npc, nil
}

// npcDataHasGatedService reports whether an authored NPC owns a service
// requires_quest can withhold. It asks the two things the gate itself acts on -
// isGatedService (the kind dispatch's own downgrade rule) and
// gatedServiceActions - so anything the runtime withholds is accepted here. The
// probe clears the gate (the question is what the NPC would be with the shop
// open) and returns a build failure rather than reporting it as "owns no
// service".
func (g *MMGame) npcDataHasGatedService(npcKey string) (bool, error) {
	npc, err := probeUngatedNPC(npcKey)
	if err != nil {
		return false, err
	}
	if g.npcDialogKindFor(npc).isGatedService() {
		return true, nil
	}
	// A service is not always a tabbed dialog: a duel master offers its bout as a
	// plain choice and resolves to dialogKindChoices. The gate withholds those by
	// ACTION, so the probe asks the same set - two definitions of "service" would
	// mean authoring that the runtime withholds and the validator calls unguarded.
	// At any depth - that is where the choice filter strips them.
	gated := false
	_ = npc.DialogueData.WalkChoices(func(c *character.NPCDialogueChoice) error {
		if gatedServiceActions[c.Action] {
			gated = true
		}
		return nil
	})
	return gated, nil
}

// npcDialogHasTalkTab reports whether a tabbed dialog carries its conversation
// tab RIGHT NOW - "Quests" on a spell trader, "Talk" on a buff service. ONE
// predicate for both: the question is the same (does the root row list still
// draw anything), and it asks the list the tab would draw rather than the raw
// authoring. A giver whose chain is concluded has no rows left, and the tab
// strip, the Tab key and the click router must agree - a strip that opens a
// blank panel for the rest of the run is worse than no strip.
func (g *MMGame) npcDialogHasTalkTab(npc *character.NPC) bool {
	// From the ROOT rows, not from whatever info branch the player stands in: a
	// node whose rows all filter out would make the strip - and the Tab key gated
	// on it - vanish mid-conversation. Asked from the draw pass, so it reads state
	// and never writes it.
	if npc == nil || npc.DialogueData == nil {
		return false
	}
	return len(g.npcChoiceRows(npc, npc.DialogueData.Choices, true)) > 0
}

// validateSpellShopsAreReachable fails the boot when an NPC carries spell rows
// that no dialog will ever show. The authored type only gets the rows COPIED;
// what draws the shop is the kind dispatch, and tavern / buff service / card
// collector / arena gladiator all win before dialogKindSpellTrader - so a trader
// that also rents rooms sells nothing and says nothing about it.
func (g *MMGame) validateSpellShopsAreReachable() error {
	if character.NPCConfigInstance == nil {
		return nil
	}
	for _, npcKey := range sortedMapKeys(character.NPCConfigInstance.NPCs) {
		data := character.NPCConfigInstance.NPCs[npcKey]
		if data == nil || len(data.Spells) == 0 {
			continue
		}
		npc, err := probeUngatedNPC(npcKey)
		if err != nil {
			return fmt.Errorf("NPC %q sells spells but cannot be built: %w", npcKey, err)
		}
		if kind := g.npcDialogKindFor(npc); kind != dialogKindSpellTrader {
			return fmt.Errorf("NPC %q authors %d spell rows but resolves to the %s dialog - its shop would never be drawn",
				npcKey, len(data.Spells), kind)
		}
	}
	return nil
}

// sortedMapKeys orders any keyed catalog for VALIDATION: a boot check that
// returns on the first bad entry must name the same one every run, or a fixed
// error looks unfixed when the next one takes its place.
func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// validateDialogueActionsAreDispatched fails the boot on an action name nothing
// runs. Every gate that classifies rows - the service gate, the conclusion rule,
// the prop route - keys off this string, and a typo currently draws a row that
// does nothing at all. Every service action must also be dispatched: a paid row
// the switch does not know about takes the money nowhere.
func (g *MMGame) validateDialogueActionsAreDispatched() error {
	if character.NPCConfigInstance == nil {
		return nil
	}
	for _, npcKey := range sortedMapKeys(character.NPCConfigInstance.NPCs) {
		data := character.NPCConfigInstance.NPCs[npcKey]
		if data == nil || data.Dialogue == nil {
			continue
		}
		var offender error
		_ = data.Dialogue.WalkChoices(func(c *character.NPCDialogueChoice) error {
			if offender != nil || c == nil {
				return nil
			}
			if len(c.Choices) > 0 && c.Action != "info" {
				offender = fmt.Errorf("NPC %q dialogue row %q carries nested choices under action %q; only info opens child rows",
					npcKey, c.Text, c.Action)
				return nil
			}
			if dialogActions[c.Action] == nil {
				offender = fmt.Errorf("NPC %q dialogue row %q declares action %q, which nothing dispatches - the row would draw and do nothing",
					npcKey, c.Text, c.Action)
			}
			return nil
		})
		if offender != nil {
			return offender
		}
	}
	for action := range gatedServiceActions {
		if dialogActions[action] == nil {
			return fmt.Errorf("service action %q is withheld by the requires_quest gate but nothing dispatches it", action)
		}
	}
	return nil
}

// validateDialogueRowsAreDrawable fails the boot when an NPC authors dialogue
// rows that no surface will ever draw. The kind dispatch decides the surface, and
// a fixed-layout kind (mastery grid, card grid, shop grid) has none - a row on one
// of those is silently dead.
//
// Two shapes are legitimate and must keep passing:
//   - a SERVICE row the kind draws itself: the tavern reads its rest/food/roster/
//     stash rows by action (gatedServiceActions is that same set);
//   - a row that lives only BEHIND the gate: while requires_quest is unpaid the
//     dispatch downgrades to dialogKindChoices, which draws everything. Nadira's
//     errand is authored exactly this way, and her rows stop being reachable at
//     the same moment they stop being kept (conclusion).
//
// So for a fixed-layout kind, every quest row must name the GATE's own quest. A
// second chained errand - the pattern used freely on other givers - would open
// behind the paid gate and be unreachable forever, with nothing at boot to say so.
func (g *MMGame) validateDialogueRowsAreDrawable() error {
	if character.NPCConfigInstance == nil {
		return nil
	}
	for _, npcKey := range sortedMapKeys(character.NPCConfigInstance.NPCs) {
		data := character.NPCConfigInstance.NPCs[npcKey]
		if data == nil || data.Dialogue == nil || len(data.Dialogue.Choices) == 0 {
			continue
		}
		npc, err := probeUngatedNPC(npcKey)
		if err != nil {
			return fmt.Errorf("NPC %q authors dialogue rows but cannot be built: %w", npcKey, err)
		}
		kind := g.npcDialogKindFor(npc)
		if kind.drawsDialogueRows() {
			continue
		}
		var offender error
		var walk func([]*character.NPCDialogueChoice, bool)
		walk = func(choices []*character.NPCDialogueChoice, atRoot bool) {
			for _, c := range choices {
				if offender != nil || c == nil {
					continue
				}
				if gatedServiceActions[c.Action] {
					// Fixed taverns consume these four ROOT rows directly. No other
					// fixed-layout kind, and no nested row, has such a surface.
					if !(kind == dialogKindTavern && atRoot && tavernDrawsAction(c.Action)) {
						offender = fmt.Errorf("NPC %q resolves to the %s dialog, which never draws its %q service row %q at this dialogue depth",
							npcKey, kind, c.Action, c.Text)
					}
				} else if c.Action != "leave" {
					// Everything else needs a gate to be reachable at all: while
					// requires_quest is unpaid the dispatch downgrades to choices.
					if data.RequiresQuest == "" {
						offender = fmt.Errorf("NPC %q resolves to the %s dialog, which draws no authored rows, so its %q row %q is never drawn anywhere",
							npcKey, kind, c.Action, c.Text)
					} else if (c.Action == "give_quest" || c.Action == "turn_in_quest") && c.QuestID != data.RequiresQuest {
						offender = fmt.Errorf("NPC %q resolves to the %s dialog, which draws no authored rows, so its %q row for quest %q could only ever be reached while requires_quest %q is unpaid - it would be unreachable for the rest of the run",
							npcKey, kind, c.Action, c.QuestID, data.RequiresQuest)
					}
				}
				walk(c.Choices, false)
			}
		}
		walk(data.Dialogue.Choices, true)
		if offender != nil {
			return offender
		}
	}
	return nil
}

// npcHasChoiceDialog reports whether the NPC presents a choice prompt - either
// an encounter (combat / quest pickup), a locked door with its derived unlock
// options, or a pure dialogue with authored selectable options. Both flow
// through the same encounter-style UI and input handler.
func npcHasChoiceDialog(npc *character.NPC) bool {
	return npc != nil && (npc.EncounterData != nil || lockedDoorClosed(npc) ||
		(npc.DialogueData != nil && len(npc.DialogueData.Choices) > 0))
}

func npcSpellKeys(npc *character.NPC) []string {
	if npc == nil || npc.SpellData == nil {
		return []string{}
	}
	keys := make([]string, 0, len(npc.SpellData))
	for key := range npc.SpellData {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func trainerOptions(char *character.MMCharacter) []trainerOption {
	if char == nil {
		return nil
	}
	options := make([]trainerOption, 0, len(char.Skills)+len(char.MagicSchools))
	for skillType, skill := range char.Skills {
		if skill == nil || !skillType.UsesMastery() || skill.Mastery >= character.MasteryGrandMaster {
			continue
		}
		next := skill.Mastery + 1
		options = append(options, trainerOption{
			Label:     skillType.String(),
			Current:   skill.Mastery,
			Next:      next,
			Cost:      character.TrainingCostForMastery(next),
			SkillType: skillType,
		})
	}
	for school, skill := range char.MagicSchools {
		if skill == nil || skill.Mastery >= character.MasteryGrandMaster {
			continue
		}
		next := skill.Mastery + 1
		options = append(options, trainerOption{
			Label:   school.DisplayName() + " Magic",
			Current: skill.Mastery,
			Next:    next,
			Cost:    character.TrainingCostForMastery(next),
			IsMagic: true,
			School:  school,
		})
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].IsMagic != options[j].IsMagic {
			return !options[i].IsMagic
		}
		return options[i].Label < options[j].Label
	})
	return options
}

// canCharacterLearnNPCSpell gates a shop row by its CATALOG KEY (the spell id,
// validated at load) and asks the CHARACTER - never the display name, and never
// one school string: a dual-school page sells to a holder of either school.
func canCharacterLearnNPCSpell(char *character.MMCharacter, spellKey string) bool {
	if char == nil || spellKey == "" {
		return false
	}
	return char.HasSchoolOpenFor(spells.SpellID(spellKey))
}

func formatNPCDialogue(template string, vars npcDialogVars) string {
	if template == "" {
		return ""
	}

	replaced := strings.ReplaceAll(template, "{name}", vars.Name)
	replaced = strings.ReplaceAll(replaced, "{spell}", vars.Spell)
	replaced = strings.ReplaceAll(replaced, "{cost}", strconv.Itoa(vars.Cost))

	if !strings.Contains(replaced, "%") {
		return replaced
	}

	args := make([]interface{}, 0)
	usedName := false
	usedSpell := false
	for i := 0; i < len(replaced); i++ {
		if replaced[i] != '%' || i+1 >= len(replaced) {
			continue
		}
		verb := replaced[i+1]
		if verb == '%' {
			i++
			continue
		}
		switch verb {
		case 's':
			if !usedName {
				args = append(args, vars.Name)
				usedName = true
			} else if !usedSpell {
				args = append(args, vars.Spell)
				usedSpell = true
			} else {
				args = append(args, "")
			}
		case 'd':
			args = append(args, vars.Cost)
		}
		i++
	}
	if len(args) == 0 {
		return replaced
	}
	return fmt.Sprintf(replaced, args...)
}
