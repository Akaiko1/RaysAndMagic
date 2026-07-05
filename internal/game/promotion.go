package game

import (
	"fmt"
	"strings"

	"ugataima/internal/character"
	"ugataima/internal/config"
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
	var suffix string
	switch c.Promotion {
	case character.PromotionArchmage:
		suffix = "_archmage"
	case character.PromotionLich:
		suffix = "_lich"
	default:
		return g.basePortraitSpriteName(c)
	}
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
	if n := g.portraitSpriteName(c) + "_full"; g.sprites.HasSprite(n) {
		return n
	}
	return strings.ToLower(c.Name) + "_full"
}

// promotionSpriteExists reports whether a member has the asset set for the given
// promotion (checked by name, then by class key).
func (g *MMGame) promotionSpriteExists(c *character.MMCharacter, suffix string) bool {
	return g.sprites.HasSprite(strings.ToLower(c.Name)+suffix) ||
		g.sprites.HasSprite(c.GetClassKey()+suffix)
}

func (g *MMGame) canPromoteArchmage(c *character.MMCharacter) bool {
	return c != nil && c.Promotion == character.PromotionNone && g.promotionSpriteExists(c, "_archmage")
}

func (g *MMGame) canPromoteLich(c *character.MMCharacter) bool {
	return c != nil && c.Promotion == character.PromotionNone && g.promotionSpriteExists(c, "_lich")
}

func (g *MMGame) eligibleArchmageIndices() []int {
	var out []int
	for i, m := range g.party.Members {
		if g.canPromoteArchmage(m) {
			out = append(out, i)
		}
	}
	return out
}

func (g *MMGame) eligibleLichIndices() []int {
	var out []int
	for i, m := range g.party.Members {
		if g.canPromoteLich(m) {
			out = append(out, i)
		}
	}
	return out
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
	unlockSchool(c, character.MagicSchoolLight)
	g.AddCombatMessage(fmt.Sprintf("%s ascends to Archmage, master of Light!", c.Name))
	g.openPromotionSpellPicker(charIndex, character.MagicSchoolLight, "Archmage: Choose Light Spells")
}

func (g *MMGame) applyLichPromotion(charIndex int) {
	if charIndex < 0 || charIndex >= len(g.party.Members) {
		return
	}
	c := g.party.Members[charIndex]
	c.Promotion = character.PromotionLich
	unlockSchool(c, character.MagicSchoolDark)
	g.AddCombatMessage(fmt.Sprintf("%s embraces undeath as a Lich, wielder of Dark!", c.Name))
	g.openPromotionSpellPicker(charIndex, character.MagicSchoolDark, "Lich: Choose Dark Spells")
}

// openPromotionSpellPicker queues a "pick up to 2" multi-select over the school's
// spells the character doesn't already know. Status + school unlock must already
// be applied by the caller (so a save mid-picker only forfeits the free spells).
func (g *MMGame) openPromotionSpellPicker(charIndex int, school character.MagicSchoolID, title string) {
	if charIndex < 0 || charIndex >= len(g.party.Members) {
		return
	}
	char := g.party.Members[charIndex]

	schoolSpells, err := school.AvailableSpellIDs()
	if err != nil {
		g.AddCombatMessage(fmt.Sprintf("No spells available for %s.", school.DisplayName()))
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
		g.AddCombatMessage(fmt.Sprintf("%s already knows every spell of that school.", char.Name))
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
	var indices []int
	if kind == character.PromotionArchmage {
		indices = g.eligibleArchmageIndices()
	} else {
		indices = g.eligibleLichIndices()
	}
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
func (g *MMGame) useLichPhylactery(itemIdx int) {
	if !g.promoteEligibleMember(character.PromotionLich, itemIdx) {
		g.AddCombatMessage("No one in the party can bind their soul to the phylactery.")
	}
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
		if itemIdx >= 0 && itemIdx < len(g.party.Inventory) &&
			g.party.Inventory[itemIdx].Attributes["promotes_lich"] > 0 {
			g.party.RemoveItem(itemIdx)
		}
	}
}
