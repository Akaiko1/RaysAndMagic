//go:build balance

package game

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestBalanceSweep fights every bench party at every level against every
// encounter the world authors within five levels: each monster alone, each
// kind in its usual pack, and every mixed group that stands together on a
// map (bosses with their guards). It lists the cells outside their bands and
// which authored monster abilities and party actions actually happened. Run:
//
//	go test -tags balance ./internal/game -run TestBalanceSweep -v
//
// BALANCE_TRIALS (default 10), BALANCE_PARTY, BALANCE_LEVEL and BALANCE_KIND
// narrow the sweep; BALANCE_OUT writes every cell as JSON.
func TestBalanceSweep(t *testing.T) {
	a := newBalanceArena(t)
	trials := balanceEnvInt("BALANCE_TRIALS", 10)
	encounters := balanceEncounters(a, a.wm)
	coverage := newBalanceCoverage()
	var cells []balanceCell
	for _, partyKey := range sortedKeys(a.bench.Parties) {
		if f := os.Getenv("BALANCE_PARTY"); f != "" && f != partyKey {
			continue
		}
		for _, level := range a.bench.Levels {
			if f := balanceEnvInt("BALANCE_LEVEL", 0); f != 0 && f != level {
				continue
			}
			for _, e := range encounters {
				if e.Level < level-5 || e.Level > level+5 {
					continue
				}
				if f := os.Getenv("BALANCE_KIND"); f != "" && f != e.Kind {
					continue
				}
				cell := balanceCell{Party: partyKey, Level: level, Kind: e.Kind, Foe: e.name(), FoeLevel: e.Level, Boss: e.Boss, Pack: len(e.Foes), Where: e.Where, Trials: trials}
				for range trials {
					f := a.fight(partyKey, level, e.Foes)
					cell.add(f)
					coverage.add(f)
				}
				cell.finish()
				cells = append(cells, cell)
			}
		}
	}
	balanceReport(cells)
	coverage.report()
	if out := os.Getenv("BALANCE_OUT"); out != "" {
		data, _ := json.MarshalIndent(cells, "", "  ")
		if err := os.WriteFile(out, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

type balanceCell struct {
	Party     string   `json:"party"`
	Level     int      `json:"level"`
	Kind      string   `json:"kind"`
	Foe       string   `json:"foe"`
	Where     string   `json:"where,omitempty"`
	FoeLevel  int      `json:"foe_level"`
	Boss      bool     `json:"boss"`
	Pack      int      `json:"pack"`
	Trials    int      `json:"trials"`
	WinPct    float64  `json:"win_pct"`
	StallPct  float64  `json:"stall_pct"`
	Rounds    float64  `json:"rounds"` // mean over won fights
	HPLostPct float64  `json:"hp_lost_pct"`
	SPPct     float64  `json:"sp_spent_pct"`
	KOs       float64  `json:"kos"`
	MaxHitPct float64  `json:"max_hit_pct"` // mean over fights of the biggest hit, % of the hero's max HP
	OneShots  float64  `json:"one_shots"`   // per fight
	Potions   float64  `json:"potions"`
	Holes     []string `json:"holes,omitempty"`

	wins, stalls, wonRounds int
	sumHP, sumSP, sumKO     float64
	sumPotions, sumOneShots int
	sumMaxHit               float64
}

func (c *balanceCell) add(f balanceFight) {
	if f.Won {
		c.wins++
		c.wonRounds += f.Rounds
	}
	if f.Timeout {
		c.stalls++
	}
	c.sumHP += f.HPLostPct
	c.sumSP += f.SPSpentPct
	c.sumKO += float64(f.KOs)
	c.sumPotions += f.Potions
	c.sumMaxHit += f.MaxHitPct
	c.sumOneShots += f.OneShots
}

// Bands (first cut, judged against the party's level):
//   - a pack at or below the party: won at least 95%, under half a KO;
//   - a pack above the party: won at least 50%;
//   - a boss at or below the party: won at least 80%; above: at least 30%;
//   - trivial: a pack 3+ levels up or a boss at or above the party falling
//     in 2 rounds or less for under 5% (pack) / 15% (boss) of party HP;
//   - stalemate: half the fights hit the round cap.
func (c *balanceCell) finish() {
	n := float64(max(1, c.Trials))
	c.WinPct = 100 * float64(c.wins) / n
	c.StallPct = 100 * float64(c.stalls) / n
	if c.wins > 0 {
		c.Rounds = float64(c.wonRounds) / float64(c.wins)
	}
	c.HPLostPct, c.SPPct, c.KOs = c.sumHP/n, c.sumSP/n, c.sumKO/n
	c.Potions = float64(c.sumPotions) / n
	c.MaxHitPct, c.OneShots = c.sumMaxHit/n, float64(c.sumOneShots)/n
	diff := c.FoeLevel - c.Level
	switch {
	case c.StallPct >= 50:
		c.Holes = append(c.Holes, "stalemate")
	case c.Boss && diff <= 0 && c.WinPct < 80, c.Boss && diff > 0 && c.WinPct < 30,
		!c.Boss && diff <= 0 && (c.WinPct < 95 || c.KOs >= 0.5), !c.Boss && diff > 0 && c.WinPct < 50:
		c.Holes = append(c.Holes, "too hard")
	case c.Boss && diff >= 0 && c.WinPct == 100 && c.Rounds <= 2 && c.HPLostPct < 15,
		!c.Boss && diff >= 3 && c.WinPct == 100 && c.Rounds <= 2 && c.HPLostPct < 5:
		c.Holes = append(c.Holes, "trivial")
	}
}

func balanceReport(cells []balanceCell) {
	fmt.Printf("\nBALANCE %d cells\n", len(cells))
	type key struct {
		party string
		level int
	}
	byGroup := map[key][]balanceCell{}
	pw := len("party")
	for _, c := range cells {
		byGroup[key{c.Party, c.Level}] = append(byGroup[key{c.Party, c.Level}], c)
		pw = max(pw, len(c.Party))
	}
	var groups []key
	for k := range byGroup {
		groups = append(groups, k)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].party != groups[j].party {
			return groups[i].party < groups[j].party
		}
		return groups[i].level < groups[j].level
	})
	fmt.Printf("%-*s %3s %5s %6s %6s %6s %5s %5s\n", pw, "party", "lvl", "foes", "win%", "hp%", "rounds", "KOs", "holes")
	for _, k := range groups {
		var win, hp, rounds, ko float64
		holes := 0
		for _, c := range byGroup[k] {
			win += c.WinPct
			hp += c.HPLostPct
			rounds += c.Rounds
			ko += c.KOs
			if len(c.Holes) > 0 {
				holes++
			}
		}
		n := float64(len(byGroup[k]))
		fmt.Printf("%-*s %3d %5d %6.0f %6.0f %6.1f %5.2f %5d\n", pw, k.party, k.level, len(byGroup[k]), win/n, hp/n, rounds/n, ko/n, holes)
	}
	fmt.Printf("\n%-*s %3s %-7s %-44s %3s %5s %5s %6s %5s %5s  %s\n", pw, "party", "lvl", "kind", "encounter", "flv", "win%", "stall", "rounds", "hp%", "KOs", "hole")
	for _, c := range cells {
		if len(c.Holes) == 0 {
			continue
		}
		kind := c.Kind
		if c.Boss {
			kind += "*"
		}
		fmt.Printf("%-*s %3d %-7s %-44s %3d %5.0f %5.0f %6.1f %5.0f %5.2f  %s\n", pw, c.Party, c.Level, kind, c.Foe, c.FoeLevel, c.WinPct, c.StallPct, c.Rounds, c.HPLostPct, c.KOs, strings.Join(c.Holes, ","))
	}
	// Composition gaps: one party wins a cell another mostly loses.
	type foeKey struct {
		foe   string
		level int
	}
	spread := map[foeKey][]balanceCell{}
	for _, c := range cells {
		spread[foeKey{c.Foe, c.Level}] = append(spread[foeKey{c.Foe, c.Level}], c)
	}
	fmt.Printf("\nCOMPOSITION GAPS (win%% spread >= 60 between parties)\n")
	var gaps []string
	for k, cs := range spread {
		lo, hi := math.MaxFloat64, -1.0
		var loP, hiP string
		for _, c := range cs {
			if c.WinPct < lo {
				lo, loP = c.WinPct, c.Party
			}
			if c.WinPct > hi {
				hi, hiP = c.WinPct, c.Party
			}
		}
		if hi-lo >= 60 {
			gaps = append(gaps, fmt.Sprintf("L%-3d %-26s %s %.0f%% vs %s %.0f%%", k.level, k.foe, hiP, hi, loP, lo))
		}
	}
	sort.Strings(gaps)
	for _, s := range gaps {
		fmt.Println(s)
	}
}

func balanceEnvInt(name string, fallback int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return fallback
}
