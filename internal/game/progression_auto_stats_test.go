package game

import (
	"testing"

	"ugataima/internal/character"
)

// autoStatKeys is every base stat AUTO may touch, in sheet order.
var autoStatKeys = []string{"might", "intellect", "personality", "endurance", "accuracy", "speed", "luck"}

// setAutoBaseline puts every base stat at 10 so a row's role starts are the
// only authored differences.
func setAutoBaseline(m *character.MMCharacter) {
	for _, key := range autoStatKeys {
		*autoStatField(m, key) = 10
	}
}

// Every playable class walks every allocator phase with the targets read from
// its own auto_stats. Starts and gains are per role (speed, endurance,
// primary, secondary); a secondary that is Speed (Wayfarer) shares the field,
// so its gains add. Every stat outside the gains must stay put, and skill
// masteries and owed level choices are never spent.
func TestAutoDistributeStatPointsPhaseTable(t *testing.T) {
	cfg := loadTestConfig(t)
	soft := autoSecondarySoftCap
	type roles map[string]int
	for _, class := range character.PlayableClasses {
		a := autoClassStats(class, cfg)
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
				roles{"speed": spd, "endurance": end - 1}, roles{"endurance": 1, "primary": 1}},
			{"endurance pair then primary and secondary", false, 8,
				roles{"speed": spd, "endurance": end - 2},
				bySecondary(roles{"endurance": 2, "primary": 4, "secondary": 2}, roles{"endurance": 2, "primary": 6})},
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
			t.Run(class.Key()+"/"+row.name, func(t *testing.T) {
				m := character.CreateCharacter("Auto", class, cfg)
				setAutoBaseline(m)
				// Secondary last: an aliased Speed secondary takes its own start.
				for _, role := range []string{"speed", "endurance", "primary", "secondary"} {
					if v, ok := row.start[role]; ok && roleKey[role] != "" {
						*autoStatField(m, roleKey[role]) = v
					}
				}
				want := map[string]int{}
				for _, key := range autoStatKeys {
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

				if spent := autoDistributeStatPoints(m, cfg); spent != row.points || m.FreeStatPoints != 0 {
					t.Fatalf("spent/free = %d/%d, want %d/0", spent, m.FreeStatPoints, row.points)
				}
				for _, key := range autoStatKeys {
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

// With a surplus the primary, secondary and Endurance all reach the stat cap
// and the catch-all drains the rest: AUTO never strands points.
func TestAutoDistributeSaturatesPrioritiesAndNeverStrandsPoints(t *testing.T) {
	cfg := loadTestConfig(t)
	for _, class := range character.PlayableClasses {
		t.Run(class.Key(), func(t *testing.T) {
			a := autoClassStats(class, cfg)
			m := character.CreateCharacter("Auto", class, cfg)
			setAutoBaseline(m)
			m.FreeStatPoints = 400

			autoDistributeStatPoints(m, cfg)

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

func TestAutoDistributePartySpendsEveryMembersPoints(t *testing.T) {
	cfg := loadTestConfig(t)
	knight := character.CreateCharacter("Knight", character.ClassKnight, cfg)
	sorcerer := character.CreateCharacter("Sorcerer", character.ClassSorcerer, cfg)
	knight.FreeStatPoints = 5
	sorcerer.FreeStatPoints = 5
	knight.Speed, sorcerer.Speed = 10, 10

	if spent := autoDistributePartyStatPoints([]*character.MMCharacter{knight, sorcerer}, cfg); spent != 10 {
		t.Fatalf("party AUTO spent %d, want 10", spent)
	}
	if knight.FreeStatPoints != 0 || sorcerer.FreeStatPoints != 0 {
		t.Fatalf("free points remain: knight=%d sorcerer=%d", knight.FreeStatPoints, sorcerer.FreeStatPoints)
	}
}
