package game

import (
	"ugataima/internal/character"
)

func (g *MMGame) restoreSavedParty(save *GameSave) {
	// Restore party
	g.party = &character.Party{Members: make([]*character.MMCharacter, 0, len(save.Party.Members)), Gold: save.Party.Gold, Food: save.Party.Food, ArenaPoints: save.Party.ArenaPoints, Inventory: save.Party.Inventory}
	for i := range g.party.Inventory {
		normalizeItemFromConfig(&g.party.Inventory[i])
	}
	// Restore the monster-card collection (party-wide). New saves carry the
	// physical card item + InstanceID; the legacy key-only field is load-only
	// migration and cannot prove ownership against the shared stash.
	g.ensureStashLoaded()
	var migrated bool
	g.cardSlots, migrated = resolveSavedCardSlots(save.Party, g.stash)
	g.loadNeedsResave = g.loadNeedsResave || migrated
	restoreRoster := func(dst *[]*character.MMCharacter, saves []CharacterSave) {
		for _, cs := range saves {
			member := restoreCharacterSave(cs)
			if member.EnsureClassKitSkills(g.config) {
				g.loadNeedsResave = true
			}
			if member.EnsureRacialTraits(g.config) {
				g.loadNeedsResave = true
			}
			*dst = append(*dst, member)
		}
	}
	restoreRoster(&g.party.Members, save.Party.Members)
	restoreRoster(&g.party.Reserve, save.Party.Reserve)
	restoreRoster(&g.party.Captive, save.Party.Captive)
	g.ensureAdditionalRecruits()
	if save.TotalExperienceEarned > 0 {
		g.totalExperienceEarned = save.TotalExperienceEarned
	} else {
		g.totalExperienceEarned = earnedExperienceForParty(g.party)
	}
	if save.TotalGoldEarned > 0 {
		g.totalGoldEarned = save.TotalGoldEarned
	} else {
		g.totalGoldEarned = save.Party.Gold - g.config.Characters.StartingGold
		if g.totalGoldEarned < 0 {
			g.totalGoldEarned = 0
		}
	}
	// Instance-id dedupe: stamp any legacy (pre-id) party items, then strip from
	// the bag anything the shared chest already owns. A stamp means this slot was
	// migrated - flag it so LoadGameFromFile persists the ids once (the strip is
	// idempotent per load and needs no resave).
	if g.stampPartyInstanceIDs() {
		g.loadNeedsResave = true
	}
	g.reconcilePartyAgainstStash()
	// Fold duplicate stackables (pre-stacking saves) into stacks AFTER the
	// stash strip, so a chest-owned copy is removed before it can merge.
	g.party.MergeStacks()
	// Benched rosters re-derive MaxHP/MaxSP under the CURRENT formula too -
	// a save written before a formula/balance change would otherwise keep
	// stale maxima until the hero is swapped in or trained. (Active members
	// get theirs via applyPartyStatBonuses below; bench carries no buffs.)
	for _, m := range g.party.Reserve {
		m.RecalculateMaxStatsKeepingCurrent(g.config)
	}
	for _, m := range g.party.Captive {
		m.RecalculateMaxStatsKeepingCurrent(g.config)
	}
}

// Authored opt-in brings newly released recruits into older saves once.
func (g *MMGame) ensureAdditionalRecruits() {
	for _, entry := range g.config.Characters.TavernRecruits {
		if !entry.AvailableInExistingSaves {
			continue
		}
		found := false
		experience := 0
		for _, roster := range [][]*character.MMCharacter{g.party.Members, g.party.Reserve, g.party.Captive} {
			for _, ch := range roster {
				if ch.Name == entry.Name {
					found = true
				}
				if xp := earnedExperienceForCharacter(ch.Level, ch.Experience); xp > experience {
					experience = xp
				}
			}
		}
		if found {
			continue
		}
		ch := character.CreateRosterCharacter(entry, g.config)
		if ch == nil {
			continue
		}
		g.party.Recruit(ch)
		ch.Experience = experience
		NewCombatSystem(g).checkLevelUp(ch, false)
		ch.HitPoints, ch.SpellPoints = ch.MaxHitPoints, ch.MaxSpellPoints
		g.loadNeedsResave = true
	}
}
