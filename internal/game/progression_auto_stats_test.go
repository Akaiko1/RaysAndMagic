package game

import (
	"testing"

	"ugataima/internal/character"
	"ugataima/internal/config"
	"ugataima/internal/stats"
)

// evenAutoTotal raises a stat outside the build's roles by one when the stat
// total is odd, so a row's pairs start with their first member.
func evenAutoTotal(t *testing.T, m *character.MMCharacter, roleKey map[string]string) {
	t.Helper()
	total := 0
	for _, key := range stats.Names {
		total += *autoStatField(m, key)
	}
	if total%2 == 0 {
		return
	}
	for _, key := range stats.Names {
		used := false
		for _, role := range roleKey {
			used = used || role == key
		}
		if !used {
			*autoStatField(m, key)++
			return
		}
	}
	t.Fatal("no stat outside the build's roles")
}

// setAutoBaseline puts every base stat at 10 so a row's role starts are the
// only authored differences.
func setAutoBaseline(m *character.MMCharacter) {
	for _, key := range stats.Names {
		*autoStatField(m, key) = 10
	}
}

// Every build of every playable class walks every allocator phase with its
// own targets. Starts and gains are per role (speed, endurance, primary,
// secondary); a role that is Speed or Endurance (Wayfarer's secondary, an
// Assassin's primary, a Guardian's secondary) shares the field, so its gains
// add. A pair alternates by the parity of the stat total, so every row starts
// on an even total (a stat outside the roles takes the odd point) and the
// pair's first member takes the first point. Every stat outside the gains must
// stay put, and skill masteries and owed level choices are never spent.
func TestAutoDistributeStatPointsPhaseTable(t *testing.T) {
	cfg := loadTestConfig(t)
	soft := autoSecondarySoftCap
	type roles map[string]int
	for _, class := range character.PlayableClasses {
		for _, build := range classAutoBuilds(class, cfg) {
			a := build.AutoStatsConfig
			spd, end := a.Speed, a.Endurance
			hasSecondary := a.Secondary != ""
			bySecondary := func(with, without roles) roles {
				if hasSecondary {
					return with
				}
				return without
			}
			roleKey := map[string]string{"speed": "speed", "endurance": "endurance", "primary": a.Primary, "secondary": a.Secondary}
			rows := []struct {
				name          string
				needSecondary bool
				points        int
				start, gain   roles
			}{
				{"speed gate comes before endurance", false, 4,
					roles{"speed": spd - 4, "endurance": end - 1}, roles{"speed": 4}},
				{"endurance takes the first point", false, 1,
					roles{"speed": spd, "endurance": end - 1}, roles{"endurance": 1}},
				{"endurance alternates with primary", false, 2,
					roles{"speed": spd, "endurance": end - 2}, roles{"endurance": 1, "primary": 1}},
				// E P E reach the target on an odd total, so the secondary leads the next pair.
				{"endurance pair then primary and secondary", false, 8,
					roles{"speed": spd, "endurance": end - 2},
					bySecondary(roles{"endurance": 2, "primary": 3, "secondary": 3}, roles{"endurance": 2, "primary": 6})},
				{"primary alternates with secondary", false, 3,
					roles{"speed": spd, "endurance": end},
					bySecondary(roles{"primary": 2, "secondary": 1}, roles{"primary": 3})},
				{"secondary stops at soft cap while primary climbs on", true, 50,
					roles{"speed": spd, "endurance": end, "secondary": soft - 5}, roles{"primary": 45, "secondary": 5}},
				{"secondary at soft cap leaves primary climbing", true, 2,
					roles{"speed": spd, "endurance": end, "secondary": soft}, roles{"primary": 2}},
				{"secondary climbs alone while primary is maxed", true, 3,
					roles{"speed": spd, "endurance": end, "primary": MaxStatValue}, roles{"secondary": 3}},
				{"maxed primary releases secondary and endurance", false, 2,
					roles{"speed": spd, "endurance": end, "primary": MaxStatValue, "secondary": soft},
					bySecondary(roles{"endurance": 1, "secondary": 1}, roles{"endurance": 2})},
			}
			for _, row := range rows {
				if row.needSecondary && !hasSecondary {
					continue
				}
				t.Run(class.Key()+"/"+build.Key+"/"+row.name, func(t *testing.T) {
					m := character.CreateCharacter("Auto", class, cfg)
					setAutoBaseline(m)
					// Secondary last: an aliased Speed secondary takes its own start.
					for _, role := range []string{"speed", "endurance", "primary", "secondary"} {
						if v, ok := row.start[role]; ok && roleKey[role] != "" {
							*autoStatField(m, roleKey[role]) = v
						}
					}
					evenAutoTotal(t, m, roleKey)
					want := map[string]int{}
					for _, key := range stats.Names {
						want[key] = *autoStatField(m, key)
					}
					for role, g := range row.gain {
						want[roleKey[role]] += g
					}
					m.FreeStatPoints = row.points
					m.OwedLevelChoices = []int{3}
					masteryBefore := map[character.SkillType]character.SkillMastery{}
					for st, s := range m.Skills {
						masteryBefore[st] = s.Mastery
					}

					if spent := autoDistributeStatPoints(m, a, cfg); spent != row.points || m.FreeStatPoints != 0 {
						t.Fatalf("spent/free = %d/%d, want %d/0", spent, m.FreeStatPoints, row.points)
					}
					for _, key := range stats.Names {
						if got := *autoStatField(m, key); got != want[key] {
							t.Errorf("%s = %d, want %d", key, got, want[key])
						}
					}
					if len(m.OwedLevelChoices) != 1 || m.OwedLevelChoices[0] != 3 {
						t.Errorf("owed choices changed: %v", m.OwedLevelChoices)
					}
					for st, before := range masteryBefore {
						if got := m.Skills[st].Mastery; got != before {
							t.Errorf("%s mastery changed from %v to %v", st, before, got)
						}
					}
				})
			}
		}
	}
}

// With a surplus the primary, secondary and Endurance all reach the stat cap
// and the catch-all drains the rest: AUTO never strands points, in any build.
func TestAutoDistributeSaturatesPrioritiesAndNeverStrandsPoints(t *testing.T) {
	cfg := loadTestConfig(t)
	for _, class := range character.PlayableClasses {
		for _, build := range classAutoBuilds(class, cfg) {
			t.Run(class.Key()+"/"+build.Key, func(t *testing.T) {
				a := build.AutoStatsConfig
				m := character.CreateCharacter("Auto", class, cfg)
				setAutoBaseline(m)
				m.FreeStatPoints = 400

				autoDistributeStatPoints(m, a, cfg)

				if m.FreeStatPoints != 0 {
					t.Errorf("free points = %d, want 0 (AUTO left points unspent)", m.FreeStatPoints)
				}
				for _, key := range []string{a.Primary, a.Secondary, "endurance"} {
					if key == "" {
						continue
					}
					if got := *autoStatField(m, key); got != MaxStatValue {
						t.Errorf("%s = %d, want %d", key, got, MaxStatValue)
					}
				}
			})
		}
	}
}

// The party AUTO button spends each hero's points by the build chosen for
// their class in Settings; a class without a choice, or with a choice that is
// no longer authored, follows its default.
func TestAutoDistributePartyFollowsEachClassesBuild(t *testing.T) {
	cfg := loadTestConfig(t)
	assassin := classAutoBuilds(character.ClassThief, cfg)[1]
	if assassin.Primary != "speed" {
		t.Fatalf("fixture: thief build %q is not Speed-first", assassin.Key)
	}
	for _, tc := range []struct {
		name      string
		choice    map[string]string
		wantThief config.AutoStatsConfig
	}{
		{"no choice", nil, autoClassStats(character.ClassThief, cfg)},
		{"chosen build", map[string]string{"thief": assassin.Key}, assassin.AutoStatsConfig},
		{"build no longer authored", map[string]string{"thief": "retired"}, autoClassStats(character.ClassThief, cfg)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := &MMGame{config: cfg, party: &character.Party{}, combatPreferences: combatPreferences{AutoBuilds: tc.choice}}
			thief := character.CreateCharacter("Thief", character.ClassThief, cfg)
			knight := character.CreateCharacter("Knight", character.ClassKnight, cfg)
			g.party.Members = []*character.MMCharacter{thief, knight}
			wantThief := character.CreateCharacter("Thief", character.ClassThief, cfg)
			wantKnight := character.CreateCharacter("Knight", character.ClassKnight, cfg)
			for _, m := range []*character.MMCharacter{thief, knight, wantThief, wantKnight} {
				m.FreeStatPoints = 60
			}
			autoDistributeStatPoints(wantThief, tc.wantThief, cfg)
			autoDistributeStatPoints(wantKnight, autoClassStats(character.ClassKnight, cfg), cfg)

			if spent := g.autoDistributePartyStatPoints(); spent != 120 {
				t.Fatalf("party AUTO spent %d, want 120", spent)
			}
			for _, pair := range [][2]*character.MMCharacter{{thief, wantThief}, {knight, wantKnight}} {
				for _, key := range stats.Names {
					if got, want := *autoStatField(pair[0], key), *autoStatField(pair[1], key); got != want {
						t.Errorf("%s %s = %d, want %d", pair[0].Name, key, got, want)
					}
				}
			}
		})
	}
}

// AUTO's result depends only on the hero and the points, never on batching:
// the Builds forecast (one batch from level 1) matches AUTO pressed after
// every level, and odd batches or single points through every phase, the
// catch-all included, end where one batch does.
func TestAutoAllocationIgnoresBatchSize(t *testing.T) {
	cfg := loadTestConfig(t)
	spendIn := func(class character.CharacterClass, a config.AutoStatsConfig, points, batch int) stats.StatBonuses {
		m := character.CreateCharacter("Auto", class, cfg)
		for points > 0 {
			step := min(batch, points)
			m.FreeStatPoints += step
			points -= step
			autoDistributeStatPoints(m, a, cfg)
		}
		return baseStatBonuses(m)
	}
	for _, class := range character.PlayableClasses {
		for _, build := range classAutoBuilds(class, cfg) {
			t.Run(class.Key()+"/"+build.Key, func(t *testing.T) {
				a := build.AutoStatsConfig
				for _, level := range autoBuildPreviewLevels {
					points := (level - 1) * StatPointsPerLevel
					forecast := baseStatBonuses(autoBuildHero(class, a, level, cfg))
					if perLevel := spendIn(class, a, points, StatPointsPerLevel); perLevel != forecast {
						t.Fatalf("L%d forecast %+v, AUTO every level %+v", level, forecast, perLevel)
					}
				}
				whole := spendIn(class, a, 400, 400)
				for _, batch := range []int{1, 7} {
					if got := spendIn(class, a, 400, batch); got != whole {
						t.Fatalf("400 points in batches of %d: %+v, in one: %+v", batch, got, whole)
					}
				}
			})
		}
	}
}
