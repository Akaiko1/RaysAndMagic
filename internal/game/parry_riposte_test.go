package game

import (
	"fmt"
	"strings"
	"testing"

	"ugataima/internal/items"
)

// TestParryingDaggerRiposteMeleeOnly: the Parrying Dagger's thorns answer a
// melee BLOW only (its tooltip says "melee damage"). An arrow / bolt / breath
// hit (melee=false) must NOT reflect, while a melee hit (melee=true) does.
func TestParryingDaggerRiposteMeleeOnly(t *testing.T) {
	cs := newTestCombatSystemWithConfig(t)
	g := cs.game
	g.cardSlots = [MaxCardSlots]cardSlot{} // no card thorns - isolate the weapon riposte

	member := g.party.Members[0]
	member.Equipment[items.SlotMainHand] = items.CreateWeaponFromYAML("parry_dagger")
	member.HitPoints, member.MaxHitPoints = 500, 500
	member.Luck = 0 // deterministic: no Perfect Dodge, the hit always lands

	// Ranged hit: no riposte.
	ranged := mkTestMonster("Archer", 1000)
	cs.monsterHitCharacter(ranged, member, "Archer", 100, "physical", false, 0, false)
	if ranged.HitPoints != 1000 {
		t.Errorf("ranged attacker took %d riposte damage, want 0 (dagger answers melee only)", 1000-ranged.HitPoints)
	}

	// Melee hit: riposte lands.
	melee := mkTestMonster("Brawler", 1000)
	cs.monsterHitCharacter(melee, member, "Brawler", 100, "physical", false, 0, true)
	if melee.HitPoints >= 1000 {
		t.Errorf("melee attacker HP = %d, should have taken the dagger riposte", melee.HitPoints)
	}
	wantLog := fmt.Sprintf("Brawler takes %d reflected damage!", 1000-melee.HitPoints)
	for _, msg := range g.GetCombatMessages() {
		if strings.Contains(msg, wantLog) {
			return
		}
	}
	t.Error("nonlethal riposte must report the actual reflected damage in the combat log")
}
