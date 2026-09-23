package game

import "ugataima/internal/monster"

func (ih *InputHandler) cancelMouseAttack() {
	ih.mouseAttackTarget, ih.mouseAttackWorld = nil, nil
	ih.mouseAttackHoldFrames = 0
}

func (ih *InputHandler) beginMouseAttack(target *monster.Monster3D) {
	ih.mouseAttackTarget = target
	ih.mouseAttackWorld = ih.game.world
	ih.mouseAttackTurnBased = ih.game.turnBasedMode
	ih.mouseAttackHoldFrames = 1
	ih.mouseAttackInputTick = ih.game.uiFrameCount
	ih.performMouseSmartAttack(target)
}

func (ih *InputHandler) repeatMouseAttack() {
	g := ih.game
	if ih.mouseAttackTarget == nil {
		return
	}
	x, y := pointerPosition()
	if !pointerLeftPressed() || !g.worldClickAllowed() || g.world != ih.mouseAttackWorld || g.turnBasedMode != ih.mouseAttackTurnBased || g.uiFrameCount-ih.mouseAttackInputTick > 1 || g.monsterAtScreen(x, y) != ih.mouseAttackTarget {
		ih.cancelMouseAttack()
		return
	}
	ih.mouseAttackInputTick = g.uiFrameCount
	ih.mouseAttackHoldFrames++
	if ih.mouseAttackHoldFrames >= rtHoldRepeatDelay {
		ih.performMouseSmartAttack(ih.mouseAttackTarget)
	}
}

func (ih *InputHandler) performMouseSmartAttack(target *monster.Monster3D) {
	g := ih.game
	if !target.IsAlive() || target.IsPartyControlled() || !g.worldClickAllowed() || g.combat == nil {
		return
	}
	if !g.combat.attackLineClear(g.camera.X, g.camera.Y, target.X, target.Y) {
		return
	}
	if g.turnBasedMode {
		if g.currentTurn != 0 || g.viewTurnFramesLeft > 0 || g.partyAllExhausted() || !g.canSelectChar(g.selectedChar) || g.spellInputCooldown != 0 {
			return
		}
	} else if ih.isRunning() && !g.partyFireWhileRunning() {
		return
	}
	// Aim the action at the selected actor without turning the player's view.
	// Every weapon/spell still resolves its own range, arc, obstruction and cost.
	defer g.combat.beginPartyTargetAim(target)()
	if g.turnBasedMode {
		ih.performTurnBasedSmartAttack()
	} else {
		ih.performRTCombatAction(rtActSmart, false)
	}
}

func (ih *InputHandler) performTurnBasedSmartAttack() {
	g := ih.game
	selected := g.party.Members[g.selectedChar]
	if acted, spellID := g.combat.SmartAttack(); acted {
		if spellID == "" {
			g.consumeSelectedCharWeaponAction()
		} else {
			g.consumeSelectedCharActionWithRTCooldown(g.combat.SpellCooldownFrames(selected, spellID))
		}
	}
	g.spellInputCooldown = ih.actionCooldown(15)
}
