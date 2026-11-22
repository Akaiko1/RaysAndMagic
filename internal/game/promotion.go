package game

import (
	"fmt"
	"strings"

	uitext "ugataima/assets/text"
	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/quests"
	"ugataima/internal/spells"
)

// Class promotions (Archmage / Lich). Eligibility is data-driven by asset
// existence: a character can take a promotion only if its promoted portrait
// sprite ships under assets/sprites/characters/ (lysander_archmage, druid_lich,
// ...) - found by basename in any subfolder via the sprite index.
// That naturally limits promotions to the Sorcerer and Druid today without a
// hardcoded class list.

// portraitSpriteName returns the sprite key for a member's portrait, preferring
// the promoted variant (by name, then by class) when one exists.
func (g *MMGame) portraitSpriteName(c *character.MMCharacter) string {
	base := strings.ToLower(c.Name)
	if c.Promotion == character.PromotionNone {
		return g.basePortraitSpriteName(c)
	}
	suffix := "_" + c.Promotion.Key()
	if n := base + suffix; g.sprites.HasSprite(n) {
		return n
	}
	if n := c.GetClassKey() + suffix; g.sprites.HasSprite(n) {
		return n
	}
	return g.basePortraitSpriteName(c)
}

// basePortraitSpriteName resolves the unpromoted portrait, falling back to the
// class sprite when no per-name sprite exists (so recruits with custom names -
// e.g. a Paladin "Auberon" - render with paladin.png).
func (g *MMGame) basePortraitSpriteName(c *character.MMCharacter) string {
	base := strings.ToLower(c.Name)
	if g.sprites.HasSprite(base) {
		return base
	}
	if key := c.GetClassKey(); g.sprites.HasSprite(key) {
		return key
	}
	return base
}

// fullPortraitSpriteName returns the large "_full" portrait key, promotion-aware.
func (g *MMGame) fullPortraitSpriteName(c *character.MMCharacter) string {
	base := g.portraitSpriteName(c)
	if n := g.largePortraitSpriteName(base); n != base {
		return n
	}
	return strings.ToLower(c.Name) + "_full"
}

// largePortraitSpriteName also resolves persisted portrait keys without needing
// a live party member. Keep the original identity when detailed art is absent.
func (g *MMGame) largePortraitSpriteName(base string) string {
	if !strings.HasSuffix(base, "_full") && g.sprites.HasSprite(base+"_full") {
		return base + "_full"
	}
	return base
}

// promotionSpriteExists reports whether a member has the asset set for the given
// promotion (checked by name, then by class key).
func (g *MMGame) promotionSpriteExists(c *character.MMCharacter, suffix string) bool {
	return g.sprites.HasSprite(strings.ToLower(c.Name)+suffix) ||
		g.sprites.HasSprite(c.GetClassKey()+suffix)
}

// canPromote: an unpromoted member whose promoted art exists.
func (g *MMGame) canPromote(kind character.Promotion, c *character.MMCharacter) bool {
	return c != nil && kind != character.PromotionNone && c.Promotion == character.PromotionNone &&
		g.promotionSpriteExists(c, "_"+kind.Key())
}

func (g *MMGame) eligiblePromotionIndices(kind character.Promotion) []int {
	var out []int
	for i, m := range g.party.Members {
		if g.canPromote(kind, m) {
			out = append(out, i)
		}
	}
	return out
}

// questPromotion is the promotion a quest's claim grants, PromotionNone for a
// plain purse.
func questPromotion(rewards quests.QuestRewards) character.Promotion {
	p, _ := character.PromotionByKey(rewards.Promotion)
	return p
}

// promotionRefusal is the giver's reply when a promotion cannot be taken now,
// "" when it can; giver is nil for the journal.
func (g *MMGame) promotionRefusal(kind character.Promotion, giver *character.NPC, questID string) string {
	switch {
	case kind == character.PromotionNone:
		return ""
	case kind.RejectsUndead() && g.party.HasLich():
		return npcQuestReply(giver, questID, func(m character.NPCQuestMessages) string { return m.RejectedUndead }, uitext.Text("dialog.promotion_rejects_undead"))
	case len(g.eligiblePromotionIndices(kind)) == 0:
		return npcQuestReply(giver, questID, func(m character.NPCQuestMessages) string { return m.Ineligible }, uitext.Text("dialog.promotion_ineligible"))
	}
	return ""
}

// unlockSchool ensures the character has access to a magic school at Novice
// mastery (mirrors addSpellByID's school init).
func unlockSchool(c *character.MMCharacter, school character.MagicSchoolID) {
	if c.MagicSchools[school] == nil {
		c.MagicSchools[school] = &character.MagicSkill{
			Mastery:     character.MasteryNovice,
			KnownSpells: make([]spells.SpellID, 0),
		}
	}
}

func (g *MMGame) applyArchmagePromotion(charIndex int) {
	if charIndex < 0 || charIndex >= len(g.party.Members) {
		return
	}
	c := g.party.Members[charIndex]
	c.Promotion = character.PromotionArchmage
	g.profileAdd("promotion:archmage", 1)
	unlockSchool(c, character.MagicSchoolLight)
	g.AddCombatMessage(uitext.Text("combat.ascends_to_archmage_master_of_light", c.Name))
	g.openPromotionSpellPicker(charIndex, character.MagicSchoolLight, "Archmage: Choose Light Spells")
}

func (g *MMGame) applyLichPromotion(charIndex int) {
	if charIndex < 0 || charIndex >= len(g.party.Members) {
		return
	}
	c := g.party.Members[charIndex]
	c.Promotion = character.PromotionLich
	g.profileAdd("promotion:lich", 1)
	unlockSchool(c, character.MagicSchoolDark)
	g.AddCombatMessage(uitext.Text("combat.embraces_undeath_as_a_lich_wielder", c.Name))
	g.openPromotionSpellPicker(charIndex, character.MagicSchoolDark, "Lich: Choose Dark Spells")
}

// openPromotionSpellPicker queues a "pick up to 2" multi-select over the school's
// spells the character doesn't already know. Status + school unlock must already
// be applied by the caller. Pending choices are preserved by save/load.
func (g *MMGame) openPromotionSpellPicker(charIndex int, school character.MagicSchoolID, title string) {
	if charIndex < 0 || charIndex >= len(g.party.Members) {
		return
	}
	char := g.party.Members[charIndex]

	schoolSpells, err := school.AvailableSpellIDs()
	if err != nil {
		g.AddCombatMessage(uitext.Text("combat.no_spells_available_for", school.DisplayName()))
		return
	}
	var options []levelUpChoiceOption
	for _, id := range schoolSpells {
		if characterKnowsSpellByID(char, id) {
			continue
		}
		def, err := spells.GetSpellDefinitionByID(id)
		if err != nil {
			continue
		}
		options = append(options, levelUpChoiceOption{
			choice:  config.LevelUpChoice{Type: "spell", Spell: string(id)},
			label:   fmt.Sprintf("Learn Spell: %s", def.Name),
			spellID: id,
		})
	}
	if len(options) == 0 {
		g.AddCombatMessage(uitext.Text("combat.already_knows_every_spell_of_that", char.Name))
		return
	}
	maxSel := 2
	if len(options) < maxSel {
		maxSel = len(options)
	}
	g.levelUpChoiceQueue = append(g.levelUpChoiceQueue, levelUpChoiceRequest{
		charIndex:     charIndex,
		options:       options,
		selection:     0,
		maxSelections: maxSel,
		selected:      make([]bool, len(options)),
		title:         title,
	})
	g.openLevelUpChoiceForChar(charIndex)
}

// promoteEligibleMember promotes immediately if exactly one member is eligible,
// otherwise opens the member picker. itemIdx is the phylactery slot to consume
// on confirm for the Lich path (-1 for the Archmage/quest path). Returns true if
// a promotion was started (direct or via picker).
func (g *MMGame) promoteEligibleMember(kind character.Promotion, itemIdx int) bool {
	indices := g.eligiblePromotionIndices(kind)
	if len(indices) == 0 {
		return false
	}
	if len(indices) == 1 {
		g.applyPromotionKind(kind, indices[0], itemIdx)
		return true
	}
	g.promotionPickerOpen = true
	g.promotionPickerKind = kind
	g.promotionPickerItemIdx = itemIdx
	return true
}

// useLichPhylactery offers the Lich path to a compatible party member. If none
// is eligible the phylactery is NOT consumed (so it isn't wasted).
func (g *MMGame) useLichPhylactery(itemIdx int, owner ...*character.MMCharacter) {
	if !g.lichPhylacteryUsable() {
		return
	}
	g.promotionPickerItemOwner = g.party.Bag(owner...).Owner
	g.promoteEligibleMember(character.PromotionLich, itemIdx)
}

// lichPhylacteryUsable reports a member who can take the Lich path; when no
// one can, it says so and the phylactery stays wherever it is.
func (g *MMGame) lichPhylacteryUsable() bool {
	if len(g.eligiblePromotionIndices(character.PromotionLich)) > 0 {
		return true
	}
	g.AddCombatMessage(uitext.Text("combat.no_one_in_the_party_can"))
	return false
}

// applyPromotionKind applies the chosen promotion to charIndex and, for the Lich
// path, consumes the phylactery at itemIdx (re-validated, since the inventory
// may have shifted).
func (g *MMGame) applyPromotionKind(kind character.Promotion, charIndex, itemIdx int) {
	switch kind {
	case character.PromotionArchmage:
		g.applyArchmagePromotion(charIndex)
	case character.PromotionLich:
		g.applyLichPromotion(charIndex)
		bag := g.party.Bag(g.promotionPickerItemOwner)
		if itemIdx >= 0 && itemIdx < len(bag.Items()) &&
			bag.Items()[itemIdx].Attributes["promotes_lich"] > 0 {
			bag.Remove(itemIdx)
		}
	}
}
