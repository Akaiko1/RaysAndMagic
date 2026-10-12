package game

import (
	"os"
	"path/filepath"
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/monster"
)

// The champion trophy is an ordinary loot table: a first_of group rolls its
// set entries in order and keeps the first success, so a kill yields at most
// ONE set piece and the authored order decides which. A pickpocket gets the
// consolation gold only - the trophy is the victory's.
func TestChampionTrophyFirstSuccessOnePieceMax(t *testing.T) {
	g, thief := newThiefTestGame(t)
	previousLoots := config.GlobalLoots
	t.Cleanup(func() { config.GlobalLoots = previousLoots })
	load := func(t *testing.T, group string) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "loots.yaml")
		body := "loot_groups:\n  trophies: [" + group + "]\nloots:\n  hobbit_archer: [{type: first_of, key: trophies}]\n"
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := config.LoadLootTables(path); err != nil {
			t.Fatal(err)
		}
	}
	champion := monster.NewMonster3DFromConfig(g.camera.X+64, g.camera.Y, "hobbit_archer", g.config)
	if !champion.IsChampion() {
		t.Fatal("hobbit_archer must be a champion")
	}
	for _, tc := range []struct {
		name, group, wantSet string
	}{
		{"both certain: the first wins alone", "{type: set, key: padded, chance: 1}, {type: set, key: ringmail, chance: 1}", "padded"},
		{"order is data", "{type: set, key: ringmail, chance: 1}, {type: set, key: padded, chance: 1}", "ringmail"},
		{"a miss falls through", "{type: set, key: padded, chance: 0}, {type: set, key: ringmail, chance: 1}", "ringmail"},
		{"all miss: nothing", "{type: set, key: padded, chance: 0}, {type: set, key: ringmail, chance: 0}", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			load(t, tc.group)
			drops := g.combat.checkMonsterLootDrop(champion)
			if tc.wantSet == "" {
				if len(drops) != 0 {
					t.Fatalf("drops = %v, want none", drops)
				}
				return
			}
			if len(drops) != 1 || drops[0].Set != tc.wantSet {
				t.Fatalf("drops = %v, want exactly one %q piece", drops, tc.wantSet)
			}
		})
	}

	load(t, "{type: set, key: padded, chance: 1}")
	thief.Skills[character.SkillSleightOfHand].Mastery = character.MasteryGrandMaster
	startGold, startItems := g.party.Gold, len(g.party.Inventory)
	for i := 0; i < 500 && !champion.Pilfered; i++ {
		g.combat.trySleightOfHand(thief, champion)
	}
	if !champion.Pilfered {
		t.Fatal("GM sleight did not land in 500 attempts")
	}
	if len(g.party.Inventory) != startItems || g.party.Gold <= startGold {
		t.Fatalf("pickpocketing a champion: items %d -> %d, gold %d -> %d; want gold only",
			startItems, len(g.party.Inventory), startGold, g.party.Gold)
	}
}
