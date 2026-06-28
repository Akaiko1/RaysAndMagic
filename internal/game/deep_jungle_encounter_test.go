package game

import (
	"testing"

	monsterPkg "ugataima/internal/monster"
)

// TestDeepJungleWarlordBindsChestEncounter guards the bug where the lost-city
// treasury never appeared: the map used SINGULAR `clear_encounter`, which ignores
// the `monsters:` list and binds EVERY mob (chest fires only when the LAST one
// dies). With the PLURAL `clear_encounters`, exactly the warlord binds, so killing
// it spawns the chest — not after sweeping the whole zone. This asserts the warlord
// (and ONLY it) is bound to the lost_city_treasury reward on the real shipped map.
func TestDeepJungleWarlordBindsChestEncounter(t *testing.T) {
	cfg := loadTestConfig(t) // loads monster config (before the chdir below)
	wm, _ := loadRealWorldForTest(t, cfg, "deep_jungle")

	w := wm.GetCurrentWorld()
	if w == nil {
		t.Fatal("deep_jungle world did not load")
	}

	var warlord, regularMob *monsterPkg.Monster3D
	bound := 0
	for _, m := range w.Monsters {
		if m.IsEncounterMonster && m.EncounterRewards != nil {
			bound++
		}
		switch m.Key {
		case "orc_hero_boss":
			warlord = m
		case "ocelot", "jungle_goblin":
			if regularMob == nil {
				regularMob = m
			}
		}
	}

	if warlord == nil {
		t.Fatal("no orc_hero_boss on the deep_jungle map")
	}
	if !warlord.IsEncounterMonster || warlord.EncounterRewards == nil {
		t.Fatal("warlord must be bound to the clear-encounter reward (else killing it never spawns the chest)")
	}
	if tc := warlord.EncounterRewards.TreasureChest; tc == nil || tc.ID != "lost_city_treasury" {
		t.Errorf("warlord's reward must carry the lost_city_treasury chest, got %+v", warlord.EncounterRewards.TreasureChest)
	}
	// Plural binding ⇒ ONLY the warlord is bound. Singular would bind the whole map
	// (the bug), so this count catches a regression to `clear_encounter`.
	if bound != 1 {
		t.Errorf("expected exactly 1 bound encounter monster (the warlord), got %d — did clear_encounters revert to singular clear_encounter?", bound)
	}
	if regularMob != nil && regularMob.IsEncounterMonster {
		t.Errorf("a regular jungle mob (%s) must NOT be bound to the warlord's chest", regularMob.Key)
	}
}
