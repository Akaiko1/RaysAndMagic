package game

import (
	"fmt"
	"math/rand"
	uitext "ugataima/assets/text"

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

// openCastPicker parks a player cast on the shared picker (ally target,
// identify, beacon, Town Portal destination); nothing is paid until it confirms.
func (g *MMGame) openCastPicker(caster *character.MMCharacter, id spells.SpellID) {
	g.cancelTownPortalPicker()
	g.townPortalPickerOpen, g.townPortalCaster, g.townPortalSpell = true, caster, id
}

type spellCastRequest struct {
	ID              spells.SpellID
	Definition      spells.SpellDefinition
	Caster          *character.MMCharacter
	Cost            int
	Announce        bool
	PlayerInitiated bool
	ActionProcs     bool
	Automatic       bool
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
	if caster.SpellState.ReuseFrames[string(req.ID)] > 0 {
		if !req.Automatic {
			cs.game.AddCombatMessage(uitext.Text("spell.reuse_not_ready", def.Name))
		}
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
	if def.TownPortal && len(cs.game.townPortalRows()) == 0 {
		cs.game.AddCombatMessage("The portal finds no destination it knows - visit a tavern, town, or major landmark first.")
		return castRejected
	}
	// The destination is chosen first; nothing is paid until one is confirmed.
	g := cs.game
	if def.TargetsAlly() && req.PlayerInitiated && cs.spellTarget == nil {
		g.openCastPicker(caster, req.ID)
		g.allySpellSelecting, g.menuOpen = true, false
		return castRejected
	}
	if def.Mechanic != nil && def.Mechanic.Kind == "identify_monster" && req.PlayerInitiated && g.identifyConfirming == nil {
		g.openCastPicker(caster, req.ID)
		g.identifySelecting, g.menuOpen = true, false
		return castRejected
	}
	if def.Mechanic != nil && def.Mechanic.Kind == "vvoid_beacon" && req.PlayerInitiated && !g.beaconConfirming {
		g.openCastPicker(caster, req.ID)
		g.beaconPicker, g.beaconSelection = true, 0
		return castRejected
	}
	if def.TownPortal && req.PlayerInitiated && g.townPortalConfirming == nil {
		g.openCastPicker(caster, req.ID)
		return castRejected
	}
	caster.SpellPoints -= req.Cost
	outcome := effect()
	if outcome != castCommitted {
		caster.SpellPoints += req.Cost
	} else if def.ReuseSeconds > 0 {
		if caster.SpellState.ReuseFrames == nil {
			caster.SpellState.ReuseFrames = make(map[string]int)
		}
		caster.SpellState.ReuseFrames[string(req.ID)] = def.ReuseSeconds * g.config.GetTPS()
	}
	// Manual spell alternatives to Pilgrim techniques spend an action without
	// adding the weapon/cast movement penalty. Automatic casts spend neither.
	if outcome == castCommitted && req.PlayerInitiated && !req.Automatic && def.Mechanic != nil && def.Mechanic.Technique != "" &&
		cs.game.turnBasedMode && cs.game.currentTurn == 0 && caster.ActionsRemaining > 0 {
		cs.game.partyTechniqueActionsUsed++
	}
	if outcome == castCommitted && req.PlayerInitiated && cs.game.playerProfile != nil {
		d := &cs.game.playerProfile.Data
		d.Add("spells", 1)
		d.Rank("spells", string(req.ID), def.Name, spellTooltipIconName(req.ID), 1)
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

// commitPickedSpell is the payment boundary for every deferred picker. Each
// picker validates its own destination and sets its effect target first.
func (g *MMGame) commitPickedSpell(caster *character.MMCharacter, id spells.SpellID, def spells.SpellDefinition) bool {
	index := g.combat.findCharacterIndex(caster)
	if caster == nil || !g.canSpendCombatAction(index) || !g.combat.castPlayerSpell(id, def, caster, true) {
		return false
	}
	g.consumeCharacterActionWithRTCooldown(index, g.combat.SpellCooldownFrames(caster, id))
	return true
}
