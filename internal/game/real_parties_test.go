package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ugataima/internal/spells"
)

// realPartiesDir holds the player's own leveled parties, snapshotted from
// saves (heroes without inventories) as samples for balance calculations.
// Resolved from this file, so tests that change directory still find it.
var realPartiesDir = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "real_parties")
}()

// realPartySample is one snapshot as the extraction writes it.
type realPartySample struct {
	Source              string          `json:"source"`
	AverageLevel        float64         `json:"average_level"`
	Members             json.RawMessage `json:"members"`
	Reserve             json.RawMessage `json:"reserve"`
	CardCollection      json.RawMessage `json:"card_collection"`
	CardCollectionItems json.RawMessage `json:"card_collection_items"`
}

// loadRealParty restores a sample into g through the save restore path, so
// derived stats (effective stats, armor, resistances) are the game's own, and
// buffs it as it goes into a fight (castFightBuffs).
func loadRealParty(t *testing.T, g *MMGame, name string) realPartySample {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(realPartiesDir, name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var sample realPartySample
	if err := json.Unmarshal(raw, &sample); err != nil {
		t.Fatal(err)
	}
	party := map[string]json.RawMessage{"members": sample.Members, "reserve": sample.Reserve,
		"card_collection": sample.CardCollection, "card_collection_items": sample.CardCollectionItems}
	for k, v := range party {
		if len(v) == 0 {
			delete(party, k)
		}
	}
	doc, _ := json.Marshal(map[string]any{"party": party})
	var save GameSave
	if err := json.Unmarshal(doc, &save); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	g.restoreSavedParty(&save)
	castFightBuffs(g, false)
	return sample
}

// castFightBuffs casts every fight buff the party knows (stats, resistances,
// damage dealt or taken), and its summons when asked, as a player prepares a
// fight; then the party rests, so buffs that raise max HP or SP leave it full.
func castFightBuffs(g *MMGame, summons bool) {
	for _, m := range g.party.Members {
		for _, school := range m.MagicSchools {
			for _, id := range school.KnownSpells {
				def, err := spells.GetSpellDefinitionByID(id)
				fightBuff := def.IsBuff() && (def.StatBonus != 0 || len(def.StatBonuses) > 0 || def.ResistBuffPct != 0 ||
					def.ResistBuffSchoolPct != 0 || def.OutgoingDamageBonus != 0 || def.IncomingDamageReduction != 0)
				if err != nil || !(fightBuff || (def.SummonMonster != "" && summons)) {
					continue
				}
				g.combat.castPlayerSpell(id, def, m, false)
			}
		}
	}
	for _, m := range g.party.Members {
		m.HitPoints, m.SpellPoints = m.MaxHitPoints, m.MaxSpellPoints
	}
}

func realPartyNames(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(realPartiesDir, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no real party samples: %v", err)
	}
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, strings.TrimSuffix(filepath.Base(f), ".json"))
	}
	return names
}

// The samples stay loadable as the save format moves on.
func TestRealPartySamplesRestore(t *testing.T) {
	for _, name := range realPartyNames(t) {
		t.Run(name, func(t *testing.T) {
			cs := newTestCombatSystemWithConfig(t)
			loadRealParty(t, cs.game, name)
			if len(cs.game.party.Members) == 0 {
				t.Fatal("no heroes restored")
			}
			for _, m := range cs.game.party.Members {
				if m.Level < 1 || m.MaxHitPoints < 1 {
					t.Fatalf("%s restored as level %d, %d max HP", m.Name, m.Level, m.MaxHitPoints)
				}
			}
		})
	}
}
