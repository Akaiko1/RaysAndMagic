package game

import (
	"testing"

	monsterPkg "ugataima/internal/monster"
	"ugataima/internal/spells"
)

// A party stun nova picks victims like every party splash (partyAreaHurts): a
// sealed or idol-warded boss is immune to its control as to its damage, an
// enemy beside it is stunned.
func TestPartyStunNovaSparesSealedBosses(t *testing.T) {
	cs := newTestCombatFactory(t)(t)
	def, err := spells.GetSpellDefinitionByID("stun")
	if err != nil || def.StunRadiusTiles <= 0 {
		t.Fatal("the stun spell lost its radius")
	}
	tile := float64(cs.game.config.GetTileSize())
	enemy, dormant, warded := mkTestMonster("Enemy", 1000), mkTestMonster("Dormant", 1000), mkTestMonster("Warded", 1000)
	dormant.BossDormant, warded.BossWarded = true, true
	for i, m := range []*monsterPkg.Monster3D{enemy, dormant, warded} {
		m.ID, m.X, m.Y = m.Name, cs.game.camera.X+tile, cs.game.camera.Y+float64(i-1)*tile/2
	}
	cs.game.world.Monsters = []*monsterPkg.Monster3D{enemy, dormant, warded}
	cs.tryCastAoeStunBy("stun", def, cs.game.party.Members[0])
	stunned := func(m *monsterPkg.Monster3D) bool { return m.Stun.Active() }
	if !stunned(enemy) || stunned(dormant) || stunned(warded) {
		t.Fatalf("stunned: enemy=%v dormant=%v warded=%v; want only the enemy", stunned(enemy), stunned(dormant), stunned(warded))
	}
}
