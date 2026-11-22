package game

import (
	"fmt"
	"math/rand"
	uitext "ugataima/assets/text"

	"ugataima/internal/character"
	"ugataima/internal/config"
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
	g.townPortalPickerOpen, g.townPortalCaster, g.townPortalSpell, g.townPortalScroll = true, caster, id, 0
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

// spellWorldRefusal is why the world turns def down now (message "" when it
// says nothing): an entombed party, a spell that needs the open sky, a portal
// with no destination. executeSpellCast refuses with it; every picker skips
// such a spell through spellCastableNow, so none picks a cast that fizzles.
func (cs *CombatSystem) spellWorldRefusal(def spells.SpellDefinition) (refused bool, message string) {
	switch {
	case cs.partyEntombed():
		return true, ""
	case def.OutdoorOnly && !cs.game.currentMapHasOpenSky():
		return true, fmt.Sprintf("%s needs the open sky.", def.Name)
	case def.TownPortal && len(cs.game.townPortalRows()) == 0:
		return true, "The portal finds no destination it knows - visit a tavern, town, or major landmark first."
	}
	return false, ""
}

// spellCastableNow: the world allows def, it is off its reuse lockout and
// caster can pay cost.
func (cs *CombatSystem) spellCastableNow(caster *character.MMCharacter, def spells.SpellDefinition, cost int) bool {
	refused, _ := cs.spellWorldRefusal(def)
	return !refused && caster.SpellState.Reusable(string(def.ID)) && caster.SpellPoints >= cost
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
		cs.game.AddCombatMessage(uitext.Text("combat.has_not_learned", caster.Name, def.Name))
		return castRejected
	}
	if !caster.SpellState.Reusable(string(req.ID)) {
		if !req.Automatic {
			cs.game.AddCombatMessage(uitext.Text("spell.reuse_not_ready", def.Name))
		}
		return castRejected
	}
	if caster.SpellPoints < req.Cost {
		cs.game.AddCombatMessage(uitext.Text("combat.s_spell_fizzles_not_enough_sp", caster.Name, caster.SpellPoints, req.Cost))
		return castRejected
	}
	if refused, message := cs.spellWorldRefusal(def); refused {
		if message != "" {
			cs.game.AddCombatMessage(message)
		}
		return castRejected
	}
	// The destination is chosen first; nothing is paid until one is confirmed.
	g := cs.game
	if def.TargetsAlly() && req.PlayerInitiated && cs.spellTarget == nil {
		g.openCastPicker(caster, req.ID)
		g.allySpellSelecting, g.menuOpen = true, false
		return castRejected
	}
	if def.Mechanic != nil && def.Mechanic.Kind == config.MechanicIdentifyMonster && req.PlayerInitiated && g.identifyConfirming == nil {
		g.openCastPicker(caster, req.ID)
		g.identifySelecting, g.menuOpen = true, false
		return castRejected
	}
	if def.Mechanic != nil && def.Mechanic.Kind == config.MechanicVvoidBeacon && req.PlayerInitiated && !g.beaconConfirming {
		g.openCastPicker(caster, req.ID)
		g.beaconPicker, g.beaconSelection = true, 0
		return castRejected
	}
	if def.TownPortal && req.PlayerInitiated && g.townPortalConfirming == nil {
		g.openCastPicker(caster, req.ID)
		return castRejected
	}
	caster.SpellPoints -= req.Cost
	previousHover := cs.castHovered
	cs.castHovered = nil
	if req.PlayerInitiated && !req.Automatic {
		cs.castHovered = cs.game.heroUnderCursor()
	}
	defer func() { cs.castHovered = previousHover }()
	previousOverflow := cs.overflow
	cs.overflow = nil
	if def.IsHeal() {
		cs.overflow = cs.beginOverflowCast(req.Cost)
	}
	defer func() { cs.overflow = previousOverflow }()
	outcome := effect()
	if outcome == castCommitted {
		cs.overflow.commit()
		if req.Cost > 0 && def.IsOffensive() {
			g.chargeSignatureWeapons(caster, 0, true)
		}
		g.armPendulum(caster, def)
	}
	if outcome != castCommitted {
		caster.SpellPoints += req.Cost
	} else if def.ReuseSeconds > 0 {
		caster.SpellState.ArmReuse(string(req.ID), def.ReuseSeconds, g.config.GetTPS())
	}
	// Penalty-free support kinds spend an action without adding the
	// weapon/cast movement penalty. Automatic casts spend neither.
	if outcome == castCommitted && req.PlayerInitiated && !req.Automatic && def.Mechanic.PenaltyFree() &&
		cs.game.turnBasedMode && cs.game.currentTurn == 0 && caster.ActionsRemaining > 0 {
		cs.game.partyPenaltyFreeActionsUsed++
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
			cs.game.AddCombatMessage(uitext.Text("combat.s_spell_echoes", req.Caster.Name))
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
	g.commitHeroAction(index, g.combat.SpellCooldownFrames(caster, id))
	return true
}
