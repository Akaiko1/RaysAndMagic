package game

import (
	"testing"
	"ugataima/internal/character"
	monsterPkg "ugataima/internal/monster"
)

func TestMonsterPoisonAppliesStatus(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)

	target := cs.findHighestEnduranceTarget()
	target.Luck = 0
	target.HitPoints = target.MaxHitPoints

	monster := &monsterPkg.Monster3D{
		Name:              "Troll",
		DamageMin:         1,
		DamageMax:         1,
		PoisonChance:      1.0,
		PoisonDurationSec: 5,
	}

	cs.applyMonsterMeleeDamage(monster, 10)

	if !target.HasCondition(character.ConditionPoisoned) {
		t.Fatalf("expected target to be poisoned")
	}
	expectedFrames := cs.game.config.GetTPS() * monster.PoisonDurationSec
	if target.PoisonFramesRemaining != expectedFrames {
		t.Fatalf("expected poison frames %d, got %d", expectedFrames, target.PoisonFramesRemaining)
	}
}

func TestMonsterFireburstDamagesParty(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)

	for _, member := range cs.game.party.Members {
		member.HitPoints = 50
	}

	monster := &monsterPkg.Monster3D{
		Name:               "Pixie",
		DamageMin:          1,
		DamageMax:          1,
		FireburstChance:    1.0,
		FireburstDamageMin: 6,
		FireburstDamageMax: 6,
	}

	cs.applyMonsterMeleeDamage(monster, 10)

	for i, member := range cs.game.party.Members {
		if member.HitPoints != 44 {
			t.Fatalf("member %d expected HP 44 after fireburst, got %d", i, member.HitPoints)
		}
	}
}
