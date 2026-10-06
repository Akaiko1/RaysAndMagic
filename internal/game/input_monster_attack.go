package game

import "ugataima/internal/monster"

func (ih *InputHandler) cancelMouseAttack() {
	ih.mouseAttackTarget, ih.mouseAttackWorld = nil, nil
	ih.mouseAttackHoldFrames = 0
}

func (ih *InputHandler) blockMouseAttackUntilRelease() {
	ih.cancelMouseAttack()
	// Cancellation is state-only: keyboard, load and camera paths can call it.
	// HandleInput clears the barrier on release or a fresh physical press.
	ih.mouseAttackBlocked = true
}

func (g *MMGame) mouseCombatInputAllowed() bool {
	return g.worldClickAllowed() && !g.dragArmed && !g.dragActive && !g.dragPickedUp && !g.stashDragPickedUp
}

func (ih *InputHandler) beginMouseAttack(target *monster.Monster3D) {
	if ih.mouseAttackBlocked {
		return
	}
	ih.mouseAttackTarget = target
	ih.mouseAttackWorld = ih.game.world
	ih.mouseAttackTurnBased = ih.game.turnBasedMode
	ih.mouseAttackHoldFrames = 1
	if target != nil {
		ih.performMouseSmartAttack(target)
	}
}

func (ih *InputHandler) repeatMouseAttack() {
	g := ih.game
	x, y := pointerPosition()
	if !pointerLeftPressed() || !g.mouseCombatInputAllowed() {
		ih.cancelMouseAttack()
		return
	}
	if ih.mouseAttackWorld != nil && (g.world != ih.mouseAttackWorld || g.turnBasedMode != ih.mouseAttackTurnBased) {
		ih.blockMouseAttackUntilRelease()
	}
	if ih.mouseAttackBlocked {
		return
	}
	// Holding is level-triggered: a press can start over the HUD or empty
	// scenery and acquire a monster later, without another button edge.
	// UI pixels pause combat, but do not discard the retained actor.
	if !g.monsterPointerFrameAllowed(x, y) {
		return
	}
	if ih.mouseAttackWorld == nil {
		ih.beginMouseAttack(nil)
	}
	if !g.monsterPointerPresent(ih.mouseAttackTarget) {
		ih.mouseAttackTarget = nil
	}
	// The actor owns the lock, not its last drawn silhouette. Empty pixels and
	// temporary occlusion keep it through animation, movement and camera turns.
	// Retargeting still requires the same opaque-pixel pick as a fresh press.
	if target := g.monsterAtScreen(x, y); target != nil && heldHoverAcquirable(target) {
		ih.mouseAttackTarget = target
	}
	ih.mouseAttackHoldFrames++
	if target := ih.focusedMouseAttackTarget(); target != nil && ih.mouseAttackHoldFrames >= rtHoldRepeatDelay {
		ih.performMouseSmartAttack(target)
	}
}

// Rendering and held attacks share one focus: a retained living actor with
// some part still visible. Losing visibility pauses fire without ending hold.
func (ih *InputHandler) focusedMouseAttackTarget() *monster.Monster3D {
	if ih.game.heldMonsterVisible(ih.mouseAttackTarget) {
		return ih.mouseAttackTarget
	}
	return nil
}

func (g *MMGame) monsterPointerPresent(target *monster.Monster3D) bool {
	if !pointerAttackable(target) || g.world == nil {
		return false
	}
	for _, live := range g.world.Monsters {
		if live == target {
			return true
		}
	}
	return false
}

func (g *MMGame) monsterPointerInSight(target *monster.Monster3D) bool {
	if !g.monsterPointerPresent(target) {
		return false
	}
	if g.combat == nil {
		return true
	}
	pose := g.logicalCameraPose()
	return g.combat.attackLineClear(pose.x, pose.y, target.X, target.Y)
}

// pointerAttackable follows the ally categories: summons are never party
// targets and Charm keeps a monster friendly, while bound former enemies (Bind
// Undead, dark-elf binding), passive monsters and wildlife stay hittable.
func pointerAttackable(m *monster.Monster3D) bool {
	return m != nil && m.IsAlive() && !isPurePartySummon(m) && !m.Pacified
}

// heldHoverAcquirable is automatic selection and follows the party
// auto-target policy. A fresh press on the caravan stays an explicit choice.
func heldHoverAcquirable(m *monster.Monster3D) bool {
	return pointerAttackable(m) && !isExcludedFromPartyAutoTarget(m)
}

func (ih *InputHandler) performMouseSmartAttack(target *monster.Monster3D) {
	g := ih.game
	if !pointerAttackable(target) || !g.worldClickAllowed() || g.combat == nil {
		return
	}
	if g.turnBasedMode {
		if g.currentTurn != 0 || g.viewTurnFramesLeft > 0 || g.partyAllExhausted() || g.spellInputCooldown != 0 {
			return
		}
		if !g.ensureTBActor(rtActSmart) {
			if g.passTBAttackRequest() {
				g.spellInputCooldown = ih.actionCooldown(15)
			}
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
	if _, used := ih.game.useSlottedRareAction(ih.game.selectedChar, rtActSmart, false); used {
		return
	}
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
