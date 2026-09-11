package game

import (
	"fmt"
	"math/rand"

	"ugataima/internal/character"
	"ugataima/internal/spells"
)

type spellCastOutcome uint8

const (
	castNotHandled spellCastOutcome = iota
	castRejected
	castNoEffect
	castCommitted
)

// A handled no-op retains the existing turn/cooldown cost, but pays no mana
// and cannot trigger effects that require a successful spell.
func (o spellCastOutcome) handled() bool { return o == castNoEffect || o == castCommitted }

type spellCastRequest struct {
	ID              spells.SpellID
	Definition      spells.SpellDefinition
	Caster          *character.MMCharacter
	Cost            int
	Announce        bool
	PlayerInitiated bool
	ActionProcs     bool
}

// executeSpellCast owns eligibility and payment for both generic and targeted
// healing casts. Effects never infer payment from a definition or an SP delta.
func (cs *CombatSystem) executeSpellCast(req spellCastRequest, effect func() spellCastOutcome) spellCastOutcome {
	caster, def := req.Caster, req.Definition
	if caster == nil || req.Cost < 0 {
		return castRejected
	}
	if (req.PlayerInitiated || req.ActionProcs) && !caster.CanUseCombatAction() {
		return castRejected
	}
	if req.PlayerInitiated && !characterKnowsSpellByID(caster, req.ID) {
		cs.game.AddCombatMessage(fmt.Sprintf("%s has not learned %s.", caster.Name, def.Name))
		return castRejected
	}
	if caster.SpellPoints < req.Cost {
		cs.game.AddCombatMessage(fmt.Sprintf("%s's spell fizzles! (Not enough SP: %d/%d)", caster.Name, caster.SpellPoints, req.Cost))
		return castRejected
	}
	if cs.partyEntombed() {
		return castRejected
	}
	if def.OutdoorOnly && !cs.game.currentMapHasOpenSky() {
		cs.game.AddCombatMessage(fmt.Sprintf("%s needs the open sky.", def.Name))
		return castRejected
	}
	if def.TownPortal && len(cs.game.sortedTownPortalDestinations()) == 0 {
		cs.game.AddCombatMessage("The portal finds no destination it knows - visit a tavern, town, or major landmark first.")
		return castRejected
	}
	caster.SpellPoints -= req.Cost
	outcome := effect()
	if outcome != castCommitted {
		caster.SpellPoints += req.Cost
	}
	return outcome
}

func (cs *CombatSystem) castSpell(req spellCastRequest) spellCastOutcome {
	outcome := cs.executeSpellCast(req, func() spellCastOutcome {
		return cs.applySpellEffect(req.ID, req.Definition, req.Caster, req.Announce)
	})
	if outcome != castCommitted {
		return outcome
	}
	if req.ActionProcs {
		cs.tryPartyActionSummons(req.Caster)
	}
	if req.Cost > 0 {
		cs.applyStrongMagicBurn(req.Caster, req.Definition, req.Cost)
	}
	cs.game.playSpellSound(req.Definition)
	if req.Definition.IsOffensive() {
		if pct := weaponSpellEchoPct(req.Caster); pct > 0 && rand.Intn(100) < pct {
			cs.game.AddCombatMessage(fmt.Sprintf("%s's spell echoes!", req.Caster.Name))
			echo := req
			echo.Cost, echo.Announce, echo.PlayerInitiated, echo.ActionProcs = 0, false, false, false
			// The transaction is shared; bypass only this recursive proc stage.
			cs.executeSpellCast(echo, func() spellCastOutcome {
				return cs.applySpellEffect(echo.ID, echo.Definition, echo.Caster, false)
			})
		}
	}
	return outcome
}
