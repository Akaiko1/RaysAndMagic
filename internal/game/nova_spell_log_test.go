package game

import (
	"testing"

	"ugataima/internal/config"
	"ugataima/internal/monster"
	"ugataima/internal/spells"
)

// Every nova (radius Earthquake, map-wide Inferno - ONE shared cast path) must
// report damage on SURVIVING monsters, not only kills - a quake that visibly
// rocks the screen and prints nothing reads as a no-op in the console.
func TestNovaSpellLogsSurvivorDamage(t *testing.T) {
	for _, spellID := range []string{"earthquake", "inferno"} {
		t.Run(spellID, func(t *testing.T) {
			cfg := loadTestConfig(t)
			if _, err := config.LoadSpellConfig("../../assets/spells.yaml"); err != nil {
				t.Fatalf("load spells: %v", err)
			}
			monster.MustLoadMonsterConfig("../../assets/monsters.yaml")
			game := newTestGame(cfg, newTestWorldSized(cfg, 20, 20))
			game.combat = NewCombatSystem(game)
			ts := float64(cfg.GetTileSize())
			placePlayerAtTile(game, 10, 10, ts)

			m := spawnMonsterAtTile(game, "troll", 10, 8, ts)
			m.HitPoints = 10000
			m.MaxHitPoints = 10000

			def, err := spells.GetSpellDefinitionByID(spells.SpellID(spellID))
			if err != nil {
				t.Fatalf("%s not in spell registry: %v", spellID, err)
			}
			caster := game.party.Members[0]
			if !game.combat.tryCastInferno(def, caster) {
				t.Fatalf("%s cast was not handled by the nova path", spellID)
			}

			if !m.IsAlive() {
				t.Fatal("test monster died - survivor logging not exercised")
			}
			if countCombatLog(game, "takes") == 0 {
				t.Fatal("surviving monster damaged by the nova but no damage line in the combat log")
			}
		})
	}
}
